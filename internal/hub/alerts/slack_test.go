// slack_test.go — RFC 0004 §"Slack-native channel" tests.
//
// The Slack-native channel uses Block Kit (color-coded severity,
// host/metric/value fields, "View in Lumen" action button) instead
// of the bare webhook format. Tests pin the wire shape and the
// action button URL derivation. The HubURLThreading test is the
// regression guard for the audit finding that produced C3.
package alerts

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestValidateChannel_SlackRequiresHooksURL covers the URL-shape
// gate. Slack Incoming Webhook URLs all start with
// `https://hooks.slack.com/services/...`. We accept only that
// shape to prevent an operator from pasting a Discord/n8n URL into
// the Slack field by mistake.
func TestValidateChannel_SlackRequiresHooksURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid hooks URL", "https://hooks.slack.com/services/T0/B0/XXX", false},
		{"missing services path", "https://hooks.slack.com/T0/B0/XXX", true},
		{"http not https", "http://hooks.slack.com/services/T0/B0/XXX", true},
		{"discord URL", "https://discord.com/api/webhooks/123/abc", true},
		{"empty", "", true},
		{"arbitrary https", "https://example.com/foo", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := `{"url":"` + c.url + `"}`
			ch := &Channel{Name: "ops", Type: "slack", Config: cfg}
			err := validateChannel(ch)
			if (err != nil) != c.wantErr {
				t.Errorf("validateChannel(slack, %q) err = %v, wantErr = %v", c.url, err, c.wantErr)
			}
		})
	}
}

// TestAllowedChannelTypes_IncludesSlack asserts the new entry is
// added. Without it, the validateChannel check (which ranges over
// AllowedChannelTypes) rejects every Slack channel before we ever
// look at the URL.
func TestAllowedChannelTypes_IncludesSlack(t *testing.T) {
	found := false
	for _, t1 := range AllowedChannelTypes {
		if t1 == "slack" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("AllowedChannelTypes = %v, missing \"slack\"", AllowedChannelTypes)
	}
}

// TestBuildSlackPayload_BlockKitShape pins the wire format. The
// payload must be a top-level object with `blocks` and
// `attachments[0].color`. The header block text encodes the state
// + severity + rule name so the operator can scan the message in
// their phone notification.
func TestBuildSlackPayload_BlockKitShape(t *testing.T) {
	n := Notification{
		RuleID:    7,
		RuleName:  "cpu high",
		Host:      "db-prod",
		Metric:    "cpu_pct",
		Severity:  "critical",
		State:     "firing",
		Value:     92.5,
		Threshold: 80,
	}
	payload := buildSlackPayload(n, "https://lumen.example.com")

	if len(payload.Attachments) == 0 {
		t.Fatal("payload missing attachments (color side bar)")
	}
	if payload.Attachments[0].Color != "#dc3545" { // red for critical
		t.Errorf("attachments[0].Color = %q, want #dc3545 (critical red)", payload.Attachments[0].Color)
	}
	if len(payload.Blocks) < 3 {
		t.Fatalf("payload.Blocks has %d entries, want ≥3 (header + section + context)",
			len(payload.Blocks))
	}
	// Header must encode state + severity + rule name.
	header := payload.Blocks[0]
	headerText := headerTextOf(header)
	for _, want := range []string{"FIRING", "CRITICAL", "cpu high"} {
		if !strings.Contains(headerText, want) {
			t.Errorf("header text missing %q: %q", want, headerText)
		}
	}
}

// TestBuildSlackPayload_SeverityColors covers the colour table.
// Resolved = green, warning = orange, critical = red, info = blue.
func TestBuildSlackPayload_SeverityColors(t *testing.T) {
	cases := []struct {
		severity, state, wantColor string
	}{
		{"critical", "firing", "#dc3545"},
		{"warning", "firing", "#fd7e14"},
		{"info", "firing", "#0d6efd"},
		{"warning", "resolved", "#198754"},
		{"critical", "resolved", "#198754"},
	}
	for _, c := range cases {
		t.Run(c.severity+"_"+c.state, func(t *testing.T) {
			n := Notification{RuleName: "r", Host: "h", Metric: "m", Severity: c.severity, State: c.state, Value: 1, Threshold: 1}
			p := buildSlackPayload(n, "")
			if p.Attachments[0].Color != c.wantColor {
				t.Errorf("color for %s/%s = %q, want %q",
					c.severity, c.state, p.Attachments[0].Color, c.wantColor)
			}
		})
	}
}

// TestBuildSlackPayload_ActionButtonURL covers the action button:
// "View in Lumen" → `${hub.public_url}/hosts/<host>` (or
// `${hub.public_url}/h/<host>` for the public share path). The
// fallback when the hub hasn't configured a public URL is
// `https://localhost` so the link is always present in the JSON.
func TestBuildSlackPayload_ActionButtonURL(t *testing.T) {
	n := Notification{RuleName: "r", Host: "db-prod", Metric: "m", Severity: "warning", State: "firing"}
	p := buildSlackPayload(n, "https://lumen.example.com")

	// Find the actions block.
	var actions *slackBlock
	for i := range p.Blocks {
		if p.Blocks[i].Type == "actions" {
			actions = &p.Blocks[i]
			break
		}
	}
	if actions == nil {
		t.Fatal("payload missing actions block (View in Lumen button)")
	}
	btn, ok := findButton(actions)
	if !ok {
		t.Fatal("actions block missing button element")
	}
	url := btn.URL
	if !strings.Contains(url, "db-prod") {
		t.Errorf("button URL %q should contain host name", url)
	}
	if !strings.HasPrefix(url, "https://lumen.example.com/") {
		t.Errorf("button URL %q should be rooted at hub public URL", url)
	}
}

// TestDispatchSlack_HappyPath runs dispatchSlack against an
// httptest.NewServer that captures the request body. The body must
// be valid JSON with `blocks` + `attachments` keys.
func TestDispatchSlack_HappyPath(t *testing.T) {
	var got int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&got, 1)
		// Echo the body so the test can assert shape.
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("body decode: %v", err)
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		if _, ok := body["blocks"]; !ok {
			t.Error("Slack body missing blocks")
		}
		if _, ok := body["attachments"]; !ok {
			t.Error("Slack body missing attachments")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := ChannelConfig{URL: srv.URL} // dispatcher will use this; the
	// validator normally checks the prefix but we can bypass it for
	// this test by constructing the config directly.
	n := Notification{RuleName: "r", Host: "h", Metric: "m", Severity: "warning", State: "firing", Value: 1, Threshold: 1}
	if err := dispatchSlack(context.Background(), cfg, n, ""); err != nil {
		t.Fatalf("dispatchSlack: %v", err)
	}
	if atomic.LoadInt32(&got) != 1 {
		t.Errorf("expected exactly 1 request to the mock, got %d", got)
	}
}

// --- helpers (types live in slack.go) ---

// headerTextOf extracts the text from a header block, or returns ""
// if the block is a different type / has no text.
func headerTextOf(b slackBlock) string {
	if b.Text == nil {
		return ""
	}
	return b.Text.Text
}

// findButton locates the first button element in an actions block.
// Returns the element + true if found. Mirrors the production type
// in slack.go (slackElement slice, not a map).
func findButton(b *slackBlock) (slackElement, bool) {
	if b == nil {
		return slackElement{}, false
	}
	for _, e := range b.Elements {
		if e.Type == "button" {
			return e, true
		}
	}
	return slackElement{}, false
}

// TestDispatchSlack_HubURLThreading is the regression guard for
// audit finding C3 — DispatchDeps.HubURL must be threaded into the
// Slack action button URL, NOT fall back to "https://localhost".
// This test fails if the slack button URL contains "localhost" when
// a real hub URL is provided.
func TestDispatchSlack_HubURLThreading(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := ChannelConfig{URL: srv.URL}
	n := Notification{RuleName: "cpu high", Host: "db-prod", Metric: "cpu_pct", Severity: "warning", State: "firing", Value: 90, Threshold: 50}
	if err := dispatchSlack(context.Background(), cfg, n, "https://lumen.example.com"); err != nil {
		t.Fatalf("dispatchSlack: %v", err)
	}
	if strings.Contains(string(capturedBody), "localhost") {
		t.Errorf("Slack payload still contains 'localhost' — HubURL not threaded through (C3 regression): %s", capturedBody)
	}
	if !strings.Contains(string(capturedBody), "lumen.example.com") {
		t.Errorf("Slack payload missing real hub URL: %s", capturedBody)
	}
}
