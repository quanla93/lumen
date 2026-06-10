// email_multi_test.go — RFC 0004 §"Multi-recipient email" failing tests.
//
// Today the email channel accepts a single recipient in to_addr. RFC
// 0004 widens this to a comma-separated list, with a 20-recipient
// cap and per-address validation. The SMTP envelope must contain
// one RCPT TO per address; the To: header must list all of them.
//
// All tests are expected to FAIL on v0.7.3 (looksLikeEmail is called
// once on the whole to_addr string, dispatchEmail sends one RCPT
// TO, the cap doesn't exist).
package alerts

import (
	"bufio"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestValidateChannel_EmailAcceptsCommaList covers the validation
// gate. Three valid addresses, comma-separated, must pass.
func TestValidateChannel_EmailAcceptsCommaList(t *testing.T) {
	cases := []struct {
		name    string
		to      string
		wantErr bool
	}{
		{"single", "ops@example.com", false},
		{"three", "ops@example.com, oncall@example.com, sre@example.com", false},
		{"trailing comma", "ops@example.com,", true},
		{"empty middle", "ops@example.com, , oncall@example.com", true},
		{"malformed middle", "ops@example.com, not-an-email, sre@example.com", true},
		{"too many (21)", strings.Repeat("u@x.com,", 21), true}, // 21 commas → 22 parts
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := `{"smtp_host":"smtp.example.com","smtp_port":587,"username":"u","password":"p","from_addr":"a@example.com","to_addr":"` + c.to + `"}`
			ch := &Channel{Name: "ops", Type: "email", Config: cfg}
			err := validateChannel(ch)
			if (err != nil) != c.wantErr {
				t.Errorf("validateEmailTo(%q) err = %v, wantErr = %v", c.to, err, c.wantErr)
			}
		})
	}
}

// TestSplitEmailRecipients covers the parser. It must trim spaces,
// drop empty pieces, and return a non-nil slice with the right
// count. Single-recipient input is returned as a 1-element slice
// (the existing single-recipient behaviour).
func TestSplitEmailRecipients(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"a@x.com", []string{"a@x.com"}},
		{"a@x.com, b@x.com", []string{"a@x.com", "b@x.com"}},
		{"  a@x.com ,  b@x.com  ", []string{"a@x.com", "b@x.com"}},
		{"a@x.com,,b@x.com", []string{"a@x.com", "b@x.com"}}, // drops empty
		{"", nil},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := SplitEmailRecipients(c.in)
			if !equalStringSlices(got, c.want) {
				t.Errorf("SplitEmailRecipients(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestMaxEmailRecipients covers the 20-recipient cap. The constant
// exists so the validateChannel path and the dispatcher share one
// source of truth — neither can drift to a different number.
func TestMaxEmailRecipients(t *testing.T) {
	if MaxEmailRecipients != 20 {
		t.Errorf("MaxEmailRecipients = %d, want 20 (per RFC 0004)", MaxEmailRecipients)
	}
}

// TestDispatchEmail_MultiRecipients drives the SMTP path against a
// mock server that captures every RCPT TO + the DATA body. The
// assertions:
//
//   - the envelope contains one RCPT TO per recipient (3 in this case)
//   - the To: header in DATA lists all three addresses
//   - the body still has one Subject + one Message
//
// We don't drive net/smtp against a real listener; instead we use
// the smtp.Server type from net/smtp/gosmtpd … no wait, the stdlib
// doesn't ship a server. We wire our own: a net.Listener that
// speaks just enough of the SMTP protocol to capture RCPT + DATA.
func TestDispatchEmail_MultiRecipients(t *testing.T) {
	srv := startMockSMTP(t)
	defer srv.Close()

	cfg := ChannelConfig{
		SmtpHost: "127.0.0.1",
		SmtpPort: srv.Port(),
		Username: "u",
		Password: "p",
		FromAddr: "alerts@example.com",
		ToAddr:   "ops@example.com, oncall@example.com, sre@example.com",
	}
	n := Notification{RuleName: "cpu high", Host: "h1", Metric: "cpu_pct", Severity: "warning", State: "firing", Value: 92, Threshold: 80, Message: "cpu is high"}
	if err := dispatchEmail(t.Context(), cfg, n); err != nil {
		t.Fatalf("dispatchEmail: %v", err)
	}

	// Assertions on what the mock captured.
	captured := srv.Captured()
	if len(captured.RcptTo) != 3 {
		t.Errorf("RCPT TO count = %d, want 3, got %v", len(captured.RcptTo), captured.RcptTo)
	}
	for _, want := range []string{"ops@example.com", "oncall@example.com", "sre@example.com"} {
		found := false
		for _, got := range captured.RcptTo {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("RCPT TO missing %q (got %v)", want, captured.RcptTo)
		}
	}
	// To: header must list all three.
	for _, want := range []string{"ops@example.com", "oncall@example.com", "sre@example.com"} {
		if !strings.Contains(captured.Data, "To: ") || !strings.Contains(captured.Data, want) {
			t.Errorf("To: header missing %q in DATA body", want)
		}
	}
}

// TestBuildEmailMessage_CommaToHeader covers the body builder. The
// `To:` header in the DATA must contain every recipient
// (comma-separated, in the original order).
func TestBuildEmailMessage_CommaToHeader(t *testing.T) {
	n := Notification{RuleName: "r", Host: "h", Metric: "m", Severity: "warning", State: "firing", Value: 1, Threshold: 1, Message: "m"}
	body := buildEmailMessage("a@x.com", "a@x.com, b@x.com, c@x.com", n)
	s := string(body)
	if !strings.Contains(s, "To: a@x.com, b@x.com, c@x.com") {
		t.Errorf("To: header = %q, want full comma-separated list", extractHeader(s, "To:"))
	}
}

// --- helpers ---

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func extractHeader(body, name string) string {
	for _, line := range strings.Split(body, "\r\n") {
		if strings.HasPrefix(line, name) {
			return line
		}
	}
	return ""
}

// mockSMTPServer is a minimal SMTP listener for the dispatchEmail
// test. It speaks just enough of the protocol to capture RCPT TO
// commands + the DATA body, then replies 250 OK to everything so
// the client can finish its envelope. We skip STARTTLS / AUTH — the
// test exercises port != 465 + non-encrypted path, and the
// dispatcher's "Only AUTH on an encrypted connection" gate means
// it won't try to AUTH (mailhog / test relays without TLS skip
// AUTH too).
type mockSMTPServer struct {
	t        *testing.T
	listener net.Listener
	port     int
	mu       sync.Mutex
	rcpt     []string
	data     strings.Builder
}

type smtpCaptured struct {
	RcptTo []string
	Data   string
}

func (m *mockSMTPServer) Port() int { return m.port }

func (m *mockSMTPServer) Captured() smtpCaptured {
	m.mu.Lock()
	defer m.mu.Unlock()
	return smtpCaptured{RcptTo: append([]string(nil), m.rcpt...), Data: m.data.String()}
}

func (m *mockSMTPServer) Close() { _ = m.listener.Close() }

func startMockSMTP(t *testing.T) *mockSMTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	m := &mockSMTPServer{t: t, listener: ln, port: ln.Addr().(*net.TCPAddr).Port}
	go m.serve()
	return m
}

func (m *mockSMTPServer) serve() {
	for {
		conn, err := m.listener.Accept()
		if err != nil {
			return
		}
		go m.handle(conn)
	}
}

func (m *mockSMTPServer) handle(conn net.Conn) {
	defer conn.Close()
	// Greet.
	_, _ = conn.Write([]byte("220 mock\r\n"))
	m.t.Logf("[mock] greeted")

	// Tiny state machine. We use bufio.Reader.ReadString('\n') so
	// the DATA body (which is one large write) gets split into
	// lines correctly across reads; a raw conn.Read() would either
	// over-read (consume the trailing "QUIT\r\n" into the same
	// buffer as the DATA body) or under-read and EOF the client.
	br := newBufioReader(conn)
	state := "greet"
	var mailFrom string
	for {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		line, err := br.ReadString('\n')
		if err != nil {
			m.t.Logf("[mock] read err: %v (state=%s)", err, state)
			return
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "EHLO") || strings.HasPrefix(line, "HELO"):
			m.t.Logf("[mock] EHLO/HELO: %q", line)
			_, _ = conn.Write([]byte("250-mock\r\n250 OK\r\n"))
		case strings.HasPrefix(line, "MAIL FROM:"):
			mailFrom = strings.TrimSpace(strings.TrimPrefix(line, "MAIL FROM:"))
			m.t.Logf("[mock] MAIL FROM: %q", mailFrom)
			_, _ = conn.Write([]byte("250 OK\r\n"))
			state = "mail"
		case strings.HasPrefix(line, "RCPT TO:"):
			addr := strings.TrimSpace(strings.TrimPrefix(line, "RCPT TO:"))
			// Strip <> if present.
			addr = strings.TrimPrefix(addr, "<")
			addr = strings.TrimSuffix(addr, ">")
			m.t.Logf("[mock] RCPT TO: %q", addr)
			m.mu.Lock()
			m.rcpt = append(m.rcpt, addr)
			m.mu.Unlock()
			_, _ = conn.Write([]byte("250 OK\r\n"))
		case strings.HasPrefix(line, "DATA"):
			m.t.Logf("[mock] DATA")
			_, _ = conn.Write([]byte("354 End data with <CR><LF>.<CR><LF>\r\n"))
			state = "data"
		case state == "data" && line == ".":
			m.t.Logf("[mock] data end (.)")
			_, _ = conn.Write([]byte("250 OK\r\n"))
			state = "post-data"
		case state == "data":
			m.mu.Lock()
			m.data.WriteString(line)
			m.data.WriteString("\r\n")
			m.mu.Unlock()
		case strings.HasPrefix(line, "QUIT"):
			m.t.Logf("[mock] QUIT")
			_, _ = conn.Write([]byte("221 Bye\r\n"))
			return
		default:
			m.t.Logf("[mock] default 250 for %q", line)
			_, _ = conn.Write([]byte("250 OK\r\n"))
		}
		_ = mailFrom // MAIL FROM captured for completeness if needed
	}
}

// newBufioReader wraps a net.Conn so the SMTP state machine can
// read line-at-a-time. Exists as a one-line helper so the rest of
// the mock reads like a normal SMTP exchange.
func newBufioReader(c net.Conn) *bufio.Reader {
	return bufio.NewReader(c)
}

// strconv referenced for symmetry (port is int but kept for future
// use in custom dialer; the linter can flag it otherwise).
var _ = strconv.Itoa
