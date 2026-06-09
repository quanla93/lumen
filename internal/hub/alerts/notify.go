package alerts

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/quanla93/lumen/internal/hub/webpush"
)

// Notification is the structured event the engine emits on a state
// transition. Channels format it for their own wire protocol; the
// generic webhook channel sends this struct as JSON verbatim.
type Notification struct {
	RuleID    int64     `json:"rule_id"`
	RuleName  string    `json:"rule_name"`
	Host      string    `json:"host"`
	Metric    string    `json:"metric"`
	Severity  string    `json:"severity"`
	State     string    `json:"state"` // firing|resolved
	Value     float64   `json:"value"`
	Threshold float64   `json:"threshold"`
	Message   string    `json:"message"`
	Time      time.Time `json:"time"`
	// DigestWindow is the channel's digest_window at the moment the
	// dispatcher enqueued the row. FormatDigestBody reads it for the
	// header line ("N alerts in last 5m:"). Empty = single-shot.
	DigestWindow string `json:"digest_window,omitempty"`
}

// dispatchClient is the single shared HTTP client. 8s timeout is the RFC
// budget — channels are best-effort, the loop must not block the engine.
var dispatchClient = &http.Client{Timeout: 8 * time.Second}

// DispatchDeps carries the extra resources channel handlers may need
// that aren't carried on Channel itself — today, the web_push handler
// reaches into the database to read subscriptions and decrypt the
// VAPID private key. Email/webhook/etc handlers ignore it. Keeping the
// struct optional (zero value works for everything except web_push)
// lets test code stay terse and isolates web_push's DB dependency from
// the other dispatch paths.
type DispatchDeps struct {
	DB        *sql.DB
	HubSecret []byte
	// HubURL is the hub's public base URL (e.g.
	// "https://lumen.example.com"). Used by the Slack channel to
	// build the "View in Lumen" action button. Empty = falls back
	// to "https://localhost" inside dispatchSlack.
	HubURL string
}

// Dispatch is the single entry point the engine uses. Returns an error
// the caller logs at Warn; never panics on a misconfigured channel.
// The logger parameter is retained for API compatibility with callers
// that pass d.cfg.Logger / h.Logger / e.cfg.Logger, but Dispatch itself
// has no log statements — errors propagate to the caller, which decides
// at what level (and with what context fields) to log them.
func Dispatch(ctx context.Context, ch Channel, n Notification, deps DispatchDeps, _ *slog.Logger) error {
	cfg, err := ch.ParsedConfig()
	if err != nil {
		return err
	}
	switch ch.Type {
	case "ntfy":
		if cfg.URL == "" {
			return ErrChannelURLRequired
		}
		return dispatchNtfy(ctx, cfg, n)
	case "discord":
		if cfg.URL == "" {
			return ErrChannelURLRequired
		}
		return dispatchDiscord(ctx, cfg, n)
	case "webhook":
		if cfg.URL == "" {
			return ErrChannelURLRequired
		}
		return dispatchWebhook(ctx, cfg, n)
	case "telegram":
		if cfg.BotToken == "" {
			return ErrTelegramBotRequired
		}
		if cfg.ChatID == "" {
			return ErrTelegramChatRequired
		}
		return dispatchTelegram(ctx, cfg, n)
	case "email":
		if cfg.SmtpHost == "" {
			return ErrEmailHostRequired
		}
		if cfg.SmtpPort < 1 || cfg.SmtpPort > 65535 {
			return ErrEmailPortInvalid
		}
		if cfg.Username == "" || cfg.Password == "" {
			return ErrEmailCredsRequired
		}
		if cfg.FromAddr == "" {
			return ErrEmailFromRequired
		}
		if cfg.ToAddr == "" {
			return ErrEmailToRequired
		}
		return dispatchEmail(ctx, cfg, n)
	case "web_push":
		if deps.DB == nil {
			return errors.New("web_push dispatch needs DB; caller did not populate DispatchDeps.DB")
		}
		return dispatchWebPush(ctx, deps, ch.ID, n)
	case "slack":
		if cfg.URL == "" {
			return ErrSlackURLRequired
		}
		return dispatchSlack(ctx, cfg, n, deps.HubURL)
	default:
		return fmt.Errorf("%w: %q", ErrInvalidChannelType, ch.Type)
	}
}

// dispatchWebPush fans out the notification to every browser subscription
// registered against the channel. Per-subscription delivery is best-effort:
// a `Gone` (404/410) response means the browser unsubscribed and the row
// is deleted so it stops costing dispatcher time. Any other non-2xx error
// short-circuits the fan-out and bubbles up so the dispatcher records a
// failed delivery (the retry loop will reattempt).
func dispatchWebPush(ctx context.Context, deps DispatchDeps, channelID int64, n Notification) error {
	keys, ok, err := webpush.LoadKeys(ctx, deps.DB, deps.HubSecret)
	if err != nil {
		return fmt.Errorf("load VAPID keys: %w", err)
	}
	if !ok {
		return errors.New("VAPID keys not generated yet; configure web push in Settings")
	}
	subs, err := webpush.ListSubscriptions(ctx, deps.DB, channelID)
	if err != nil {
		return fmt.Errorf("list web_push subscriptions: %w", err)
	}
	if len(subs) == 0 {
		// No subscribers is not a delivery failure — let the dispatcher
		// mark this as sent so it stops retrying. The Settings UI lists
		// "0 subscriptions" so the operator can correct it explicitly.
		return nil
	}
	payload, err := webPushPayload(n)
	if err != nil {
		return err
	}
	for _, sub := range subs {
		sendErr := webpush.SendOne(ctx, keys, sub, payload)
		if sendErr == nil {
			continue
		}
		if webpush.IsGone(sendErr) {
			// Browser unsubscribed; drop the row so future fan-outs skip it.
			_ = webpush.DeleteSubscription(ctx, deps.DB, sub.ID)
			continue
		}
		return sendErr
	}
	return nil
}

// webPushPayload is the JSON the service worker decodes on the browser
// side to construct the visible notification. Keep keys terse and
// stable; the service worker has to ship with the hub binary so a
// schema change here = service worker bump.
func webPushPayload(n Notification) ([]byte, error) {
	body := strings.TrimSpace(n.Message)
	if body == "" {
		body = fmt.Sprintf("%s on %s: value=%.2f threshold=%.2f", n.Metric, n.Host, n.Value, n.Threshold)
	}
	state := "FIRING"
	if strings.EqualFold(n.State, "resolved") {
		state = "RESOLVED"
	}
	title := fmt.Sprintf("[%s] %s — %s", state, n.Severity, n.RuleName)
	return json.Marshal(map[string]any{
		"title": title,
		"body":  body,
		"tag":   fmt.Sprintf("rule:%d", n.RuleID),
		"url":   "/",
	})
}

// ntfy: POST <url>, body = plaintext message. Title/Priority/Tags as
// headers. URL holds the full topic endpoint (e.g. https://ntfy.sh/lumen).
func dispatchNtfy(ctx context.Context, cfg ChannelConfig, n Notification) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cfg.URL, strings.NewReader(n.Message))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("Title", ntfyTitle(n))
	req.Header.Set("Priority", ntfyPriority(cfg.Priority, n))
	req.Header.Set("Tags", ntfyTags(n))
	if cfg.Topic != "" {
		// Some ntfy deployments accept a separate topic header; harmless if
		// already encoded in the URL path.
		req.Header.Set("X-Topic", cfg.Topic)
	}
	return doRequest(req, "ntfy")
}

func ntfyTitle(n Notification) string {
	if n.State == "resolved" {
		return fmt.Sprintf("RESOLVED · %s · %s", n.RuleName, n.Host)
	}
	return fmt.Sprintf("[%s] %s · %s", strings.ToUpper(n.Severity), n.RuleName, n.Host)
}

func ntfyPriority(operatorOverride string, n Notification) string {
	if operatorOverride != "" {
		return operatorOverride
	}
	if n.State == "resolved" {
		return "default"
	}
	switch n.Severity {
	case "critical":
		return "urgent"
	case "warning":
		return "high"
	default:
		return "default"
	}
}

func ntfyTags(n Notification) string {
	if n.State == "resolved" {
		return "white_check_mark"
	}
	switch n.Severity {
	case "critical":
		return "rotating_light"
	case "warning":
		return "warning"
	default:
		return "information_source"
	}
}

// discord: POST <webhook URL> with {"content": ...}. Plain text is fine
// for Milestone A; embeds land later.
func dispatchDiscord(ctx context.Context, cfg ChannelConfig, n Notification) error {
	prefix := ""
	if n.State == "resolved" {
		prefix = ":white_check_mark: [RESOLVED] "
	} else if n.Severity == "critical" {
		prefix = ":rotating_light: [CRITICAL] "
	} else if n.Severity == "warning" {
		prefix = ":warning: [WARNING] "
	}
	payload := map[string]string{"content": prefix + n.Message}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cfg.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doRequest(req, "discord")
}

// webhook: POST <url> with the Notification struct as JSON. HMAC signing
// is deferred to the Public-API webhook unification (RFC notes).
func dispatchWebhook(ctx context.Context, cfg ChannelConfig, n Notification) error {
	body, err := json.Marshal(n)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cfg.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "lumen-hub/alerts")
	return doRequest(req, "webhook")
}

// telegram: POST https://api.telegram.org/bot<token>/sendMessage. We
// build the URL from the secret token so it never has to round-trip
// through the operator's clipboard / logs. ParseMode defaults to HTML
// because the message body is plain text — HTML is the safest mode for
// unescaped text (only <, >, & are special).
func dispatchTelegram(ctx context.Context, cfg ChannelConfig, n Notification) error {
	parseMode := cfg.ParseMode
	if parseMode == "" {
		parseMode = "HTML"
	}
	text := telegramMessage(n)
	payload := map[string]string{
		"chat_id":    cfg.ChatID,
		"text":       text,
		"parse_mode": parseMode,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := "https://api.telegram.org/bot" + cfg.BotToken + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doRequest(req, "telegram")
}

func telegramMessage(n Notification) string {
	prefix := ""
	switch {
	case n.State == "resolved":
		prefix = "✅ <b>RESOLVED</b> "
	case n.Severity == "critical":
		prefix = "🚨 <b>CRITICAL</b> "
	case n.Severity == "warning":
		prefix = "⚠️ <b>WARNING</b> "
	default:
		prefix = "ℹ️ "
	}
	return prefix + escapeHTML(n.Message)
}

// escapeHTML keeps Telegram's HTML parse_mode happy. Only three characters
// are special; do not escape '<b>' the prefix produces itself.
func escapeHTML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

// doRequest sends, checks status, and reads a bounded error body so we
// can attribute failures without leaking large response payloads.
func doRequest(req *http.Request, kind string) error {
	resp, err := dispatchClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", kind, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("%s: HTTP %d: %s", kind, resp.StatusCode, bytes.TrimSpace(snippet))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// email: send via SMTP. Port 465 uses implicit TLS (legacy "SMTPS"); any
// other port goes through STARTTLS via the stdlib smtp.SendMail helper,
// which negotiates TLS automatically when the server advertises it and
// requires it before AUTH. We dial through net.Dialer.DialContext so the
// engine's dispatch timeout / cancellation propagates — net/smtp's
// blocking calls otherwise ignore ctx entirely.
//
// The library deliberately stays at PLAIN auth: every modern SMTP relay
// (Gmail, SendGrid, Mailgun, SES, custom Postfix) accepts it as long as
// the connection is encrypted, which we enforce above. CRAM-MD5 and the
// no-auth case would only matter on internal relays — out of scope for
// homelab alerts.
func dispatchEmail(ctx context.Context, cfg ChannelConfig, n Notification) error {
	addr := net.JoinHostPort(cfg.SmtpHost, strconv.Itoa(cfg.SmtpPort))
	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.SmtpHost)
	msg := buildEmailMessage(cfg.FromAddr, cfg.ToAddr, n)

	dialer := &net.Dialer{Timeout: 8 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("email: dial %s: %w", addr, err)
	}
	defer conn.Close()

	encrypted := false
	if cfg.SmtpPort == 465 {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: cfg.SmtpHost})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("email: tls handshake: %w", err)
		}
		conn = tlsConn
		encrypted = true
	}

	c, err := smtp.NewClient(conn, cfg.SmtpHost)
	if err != nil {
		return fmt.Errorf("email: smtp client: %w", err)
	}
	defer c.Close()

	// STARTTLS for non-465 ports. Skip silently if the server doesn't
	// advertise it — local mailhog / unencrypted internal relays
	// typically don't.
	if cfg.SmtpPort != 465 {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: cfg.SmtpHost}); err != nil {
				return fmt.Errorf("email: starttls: %w", err)
			}
			encrypted = true
		}
	}
	// Only AUTH on an encrypted connection. net/smtp.PlainAuth refuses
	// to send credentials over plaintext anyway (returns "unencrypted
	// connection") — gating here gives a cleaner UX for MailHog and
	// other test relays that ADVERTISE AUTH but don't actually need it.
	// The narrow loss is "internal SMTP relay that requires AUTH but
	// doesn't offer TLS" — rare and arguably misconfigured; the server
	// will reject MAIL FROM with 530 5.7.0 in that case, which surfaces
	// as a clear dispatcher error to the operator.
	if encrypted {
		if ok, _ := c.Extension("AUTH"); ok {
			if err := c.Auth(auth); err != nil {
				return fmt.Errorf("email: auth: %w", err)
			}
		}
	}
	if err := c.Mail(cfg.FromAddr); err != nil {
		return fmt.Errorf("email: MAIL FROM: %w", err)
	}
	// RFC 0004: fan out to N recipients via N×RCPT TO. The DATA
	// body's `To:` header still lists all addresses (single
	// header, comma-separated) so RFC 5322 clients render the
	// full list. We bail on the first RCPT error — partial
	// fan-out leaves the operator guessing which addresses got
	// the message.
	addrs := SplitEmailRecipients(cfg.ToAddr)
	if len(addrs) == 0 {
		// Should be caught by validateChannel, but defend in depth.
		return ErrEmailToRequired
	}
	for _, a := range addrs {
		if err := c.Rcpt(a); err != nil {
			return fmt.Errorf("email: RCPT TO %s: %w", a, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("email: DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("email: write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("email: close body: %w", err)
	}
	return c.Quit()
}

func buildEmailMessage(from, to string, n Notification) []byte {
	var b bytes.Buffer
	b.WriteString("From: ")
	b.WriteString(from)
	b.WriteString("\r\n")
	b.WriteString("To: ")
	b.WriteString(to)
	b.WriteString("\r\n")
	b.WriteString("Subject: ")
	b.WriteString(emailSubject(n))
	b.WriteString("\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(n.Message)
	b.WriteString("\r\n")
	return b.Bytes()
}

func emailSubject(n Notification) string {
	if n.State == "resolved" {
		return fmt.Sprintf("[RESOLVED] %s · %s", n.RuleName, n.Host)
	}
	return fmt.Sprintf("[%s] %s · %s", strings.ToUpper(n.Severity), n.RuleName, n.Host)
}

// FormatMessage builds the human-facing message used by every channel.
// Centralised so the wording stays consistent (Test button reuses it).
func FormatMessage(n Notification) string {
	switch n.State {
	case "resolved":
		if n.Metric == "offline" {
			return fmt.Sprintf("%s · host %s is online again",
				n.RuleName, n.Host)
		}
		return fmt.Sprintf("%s · %s on %s back below threshold (%.2f)",
			n.RuleName, n.Metric, n.Host, n.Value)
	default:
		if n.Metric == "offline" {
			return fmt.Sprintf("%s · host %s appears OFFLINE",
				n.RuleName, n.Host)
		}
		return fmt.Sprintf("%s · %s on %s is %.2f (threshold %.2f)",
			n.RuleName, n.Metric, n.Host, n.Value, n.Threshold)
	}
}
