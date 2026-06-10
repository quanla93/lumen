// slack.go — RFC 0004 §"Slack-native channel" implementation.
//
// Uses Slack's Block Kit for the message body (color side bar via
// the legacy `attachments` field — still rendered in modern Slack
// clients; works around the "no header colour" Block Kit gap).
// The "View in Lumen" action button URL is rooted at the hub's
// public URL; the caller passes it down via the optional hubURL
// argument so tests can avoid touching env / settings.

package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// slackPayload mirrors the JSON shape we POST to a Slack Incoming
// Webhook. The Block Kit body lives under `blocks`; the color side
// bar lives under `attachments[0].color` (Slack legacy but still
// rendered, and Block Kit's `header` block can't carry a color).
type slackPayload struct {
	Blocks      []slackBlock  `json:"blocks"`
	Attachments []slackAttach `json:"attachments"`
}

type slackBlock struct {
	Type     string         `json:"type"`
	Text     *slackText     `json:"text,omitempty"`
	Fields   []slackText    `json:"fields,omitempty"`
	Elements []slackElement `json:"elements,omitempty"`
}

type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// slackElement is the polymorphic element shape. We model it as a
// typed struct for the button + plain-text cases we actually emit;
// unknown shapes would need a custom MarshalJSON, but RFC 0004 only
// requires a button + a context-line text.
type slackElement struct {
	Type     string         `json:"type"`
	Text     *slackText     `json:"text,omitempty"`
	ActionID string         `json:"action_id,omitempty"`
	URL      string         `json:"url,omitempty"`
	Style    string         `json:"style,omitempty"`
	Elements []slackElement `json:"elements,omitempty"`
}

type slackAttach struct {
	Color string `json:"color"`
}

// severityColor returns the hex code for the legacy attachment
// color bar. Resolved always wins (green) so a "fix" alert never
// shows red. Otherwise severity maps:
//   critical → #dc3545 (red)
//   warning  → #fd7e14 (orange)
//   info     → #0d6efd (blue)
func severityColor(n Notification) string {
	if n.State == "resolved" {
		return "#198754"
	}
	switch n.Severity {
	case "critical":
		return "#dc3545"
	case "warning":
		return "#fd7e14"
	default:
		return "#0d6efd"
	}
}

// buildSlackPayload assembles the JSON we ship to the operator's
// Slack Incoming Webhook. The hubURL parameter is the hub's public
// base URL ("https://lumen.example.com"); an empty string falls
// back to "https://localhost" so the button link is always present
// in the JSON, even when the operator hasn't configured a public
// URL yet.
func buildSlackPayload(n Notification, hubURL string) slackPayload {
	state := "FIRING"
	if n.State == "resolved" {
		state = "RESOLVED"
	}
	headerText := fmt.Sprintf("[%s] %s — %s", state, strings.ToUpper(n.Severity), n.RuleName)
	header := slackBlock{
		Type: "header",
		Text: &slackText{Type: "plain_text", Text: headerText},
	}
	section := slackBlock{
		Type: "section",
		Fields: []slackText{
			{Type: "mrkdwn", Text: "*Host:*\n" + n.Host},
			{Type: "mrkdwn", Text: "*Metric:*\n" + n.Metric},
			{Type: "mrkdwn", Text: fmt.Sprintf("*Value:*\n%.2f", n.Value)},
			{Type: "mrkdwn", Text: fmt.Sprintf("*Threshold:*\n%.2f", n.Threshold)},
		},
	}
	context := slackBlock{
		Type: "context",
		Elements: []slackElement{
			{Type: "mrkdwn", Text: &slackText{Type: "mrkdwn", Text: "Color: " + severityColor(n)}},
		},
	}
	// Action button — "View in Lumen". Falls back to a localhost
	// URL so the link is never empty (Slack rejects empty URL
	// fields).
	if hubURL == "" {
		hubURL = "https://localhost"
	}
	actions := slackBlock{
		Type: "actions",
		Elements: []slackElement{
			{
				Type:     "button",
				Text:     &slackText{Type: "plain_text", Text: "View in Lumen"},
				ActionID: "view-in-lumen",
				URL:      fmt.Sprintf("%s/hosts/%s", hubURL, n.Host),
				Style:    "primary",
			},
		},
	}
	return slackPayload{
		Blocks:      []slackBlock{header, section, context, actions},
		Attachments: []slackAttach{{Color: severityColor(n)}},
	}
}

// dispatchSlack POSTs the Block Kit payload to the Incoming
// Webhook URL. The hubURL argument is forwarded to
// buildSlackPayload so the action button links somewhere useful.
// We accept it as a string (not a Settings key lookup) to keep
// the dispatcher free of env / settings reads — server.go does
// the lookup once and threads the value through.
func dispatchSlack(ctx context.Context, cfg ChannelConfig, n Notification, hubURL string) error {
	if cfg.URL == "" {
		return ErrSlackURLRequired
	}
	payload := buildSlackPayload(n, hubURL)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("slack: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		cfg.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doRequest(req, "slack")
}
