package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
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
}

// dispatchClient is the single shared HTTP client. 8s timeout is the RFC
// budget — channels are best-effort, the loop must not block the engine.
var dispatchClient = &http.Client{Timeout: 8 * time.Second}

// Dispatch is the single entry point the engine uses. Returns an error
// the caller logs at Warn; never panics on a misconfigured channel.
// The logger parameter is retained for API compatibility with callers
// that pass d.cfg.Logger / h.Logger / e.cfg.Logger, but Dispatch itself
// has no log statements — errors propagate to the caller, which decides
// at what level (and with what context fields) to log them.
func Dispatch(ctx context.Context, ch Channel, n Notification, _ *slog.Logger) error {
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
	default:
		return fmt.Errorf("%w: %q", ErrInvalidChannelType, ch.Type)
	}
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
