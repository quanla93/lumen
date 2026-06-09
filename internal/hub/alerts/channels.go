package alerts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Allowed channel types.
var AllowedChannelTypes = []string{"ntfy", "discord", "webhook", "telegram", "email", "web_push", "slack"}

var (
	ErrChannelNotFound       = errors.New("notification channel not found")
	ErrChannelNameRequired   = errors.New("name required")
	ErrInvalidChannelType    = errors.New("invalid channel type")
	ErrChannelURLRequired    = errors.New("config.url required")
	ErrChannelURLNotHTTP     = errors.New("config.url must be http(s)")
	ErrChannelConfigInvalid  = errors.New("invalid channel config")
	ErrInvalidMinSeverity    = errors.New("invalid min_severity")
	ErrTelegramBotRequired   = errors.New("config.bot_token required for telegram")
	ErrTelegramChatRequired  = errors.New("config.chat_id required for telegram")
	ErrEmailHostRequired     = errors.New("config.smtp_host required for email")
	ErrEmailPortInvalid      = errors.New("config.smtp_port must be 1-65535")
	ErrEmailCredsRequired    = errors.New("config.username and config.password required for email")
	ErrEmailFromRequired     = errors.New("config.from_addr required for email")
	ErrEmailToRequired       = errors.New("config.to_addr required for email")
	ErrEmailAddrInvalid      = errors.New("invalid email address")
	ErrInvalidDigestWindow   = errors.New("invalid digest_window (allowed: \"\", \"0\", \"1m\", \"5m\", \"15m\", \"1h\")")
	ErrSlackURLRequired      = errors.New("config.url required for slack")
	ErrSlackURLInvalid       = errors.New("config.url must be a Slack Incoming Webhook URL (https://hooks.slack.com/services/...)")
)

// Channel holds one notification_channels row. Config is the raw JSON
// string (opaque to most callers); ChannelConfig exposes the parsed
// fields the dispatcher actually needs.
type Channel struct {
	ID          int64
	Name        string
	Type        string
	Config      string // raw JSON as stored
	OwnerType   string // 'admin' today; forward-compat for 'api_key'
	Enabled     bool
	MinSeverity string // info|warning|critical — channel suppresses events below this rank
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ChannelConfig is the parsed shape of Channel.Config. Different channel
// types use different subsets:
//   - ntfy:     URL (topic URL), Priority (optional), Topic (optional), DigestWindow
//   - discord:  URL (webhook URL), DigestWindow
//   - webhook:  URL, DigestWindow
//   - telegram: BotToken + ChatID (+ ParseMode optional). No URL needed —
//               the dispatcher composes the Bot API endpoint from the
//               token. BotToken is the secret; never log it. DigestWindow.
//   - email:    SmtpHost + SmtpPort + Username + Password + FromAddr + ToAddr
//               (comma-separated; see SplitEmailRecipients for the
//               RFC 0004 multi-recipient widening). Password is the secret.
//               Port 465 uses implicit TLS; other ports use STARTTLS via
//               net/smtp. DigestWindow.
//   - slack:    URL (Incoming Webhook, https://hooks.slack.com/services/...).
//               DigestWindow.
//
// DigestWindow is the RFC 0004 §"Digest" operator opt-in. Empty / "0"
// = today's behaviour (one notification per event). "1m" / "5m" / "15m"
// / "1h" = buffer events for that window then send one combined body.
// Critical-severity channels default to no buffering (the operator
// explicitly opts in by setting a non-empty value).
type ChannelConfig struct {
	URL          string `json:"url,omitempty"`
	Topic        string `json:"topic,omitempty"`         // ntfy: optional explicit topic header
	Priority     string `json:"priority,omitempty"`      // ntfy: min|low|default|high|urgent
	BotToken     string `json:"bot_token,omitempty"`     // telegram
	ChatID       string `json:"chat_id,omitempty"`       // telegram (string lets users paste @channel or numeric id)
	ParseMode    string `json:"parse_mode,omitempty"`    // telegram: HTML|Markdown|MarkdownV2 (default HTML)
	SmtpHost     string `json:"smtp_host,omitempty"`     // email: smtp.gmail.com etc
	SmtpPort     int    `json:"smtp_port,omitempty"`     // email: 587 (STARTTLS), 465 (implicit TLS)
	Username     string `json:"username,omitempty"`      // email: SMTP login (typically the from address)
	Password     string `json:"password,omitempty"`      // email: SMTP password / app token — secret, never log
	FromAddr     string `json:"from_addr,omitempty"`     // email: sender envelope + From header
	ToAddr       string `json:"to_addr,omitempty"`       // email: comma-separated recipients (see SplitEmailRecipients)
	DigestWindow string `json:"digest_window,omitempty"` // RFC 0004: "" / "0" / "1m" / "5m" / "15m" / "1h"
}

// allowedDigestWindows pins the granularity operators can pick. We
// don't accept arbitrary durations because the rendering in the
// channel form lists these as a dropdown — keeping the set closed
// prevents typos like "5M" (uppercase) or "30s" (off-grid) from
// silently storing an unparseable value.
var allowedDigestWindows = map[string]time.Duration{
	"":  0,
	"0": 0,
	"1m": 1 * time.Minute,
	"5m": 5 * time.Minute,
	"15m": 15 * time.Minute,
	"1h": 1 * time.Hour,
}

// ParseDigestWindow maps a ChannelConfig.DigestWindow string to a
// duration. Returns 0 with no error for "" / "0" (no buffering).
// Returns ErrInvalidDigestWindow wrapped around a hint listing the
// allowed set so the validateChannel path can return 400 with a
// useful message.
func ParseDigestWindow(s string) (time.Duration, error) {
	if d, ok := allowedDigestWindows[s]; ok {
		return d, nil
	}
	return 0, fmt.Errorf("%w: got %q", ErrInvalidDigestWindow, s)
}

func (c *Channel) ParsedConfig() (ChannelConfig, error) {
	var cc ChannelConfig
	if c.Config == "" {
		return cc, nil
	}
	if err := json.Unmarshal([]byte(c.Config), &cc); err != nil {
		return cc, fmt.Errorf("%w: %v", ErrChannelConfigInvalid, err)
	}
	return cc, nil
}

const channelColumns = `id, name, type, config, owner_type, enabled, min_severity, created_at, updated_at`

func scanChannel(scanner interface{ Scan(dest ...any) error }) (Channel, error) {
	var c Channel
	var enabled int
	err := scanner.Scan(
		&c.ID, &c.Name, &c.Type, &c.Config, &c.OwnerType, &enabled, &c.MinSeverity,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return Channel{}, err
	}
	c.Enabled = enabled != 0
	return c, nil
}

func validateChannel(c *Channel) error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return ErrChannelNameRequired
	}
	if len(c.Name) > 128 {
		return errors.New("name too long (max 128 chars)")
	}
	if !contains(AllowedChannelTypes, c.Type) {
		return fmt.Errorf("%w: %q (allowed: %s)", ErrInvalidChannelType, c.Type, strings.Join(AllowedChannelTypes, ","))
	}
	if c.OwnerType == "" {
		c.OwnerType = "admin"
	}
	if c.MinSeverity == "" {
		c.MinSeverity = "info"
	}
	if !contains(AllowedSeverities, c.MinSeverity) {
		return fmt.Errorf("%w: %q", ErrInvalidMinSeverity, c.MinSeverity)
	}
	cc, err := c.ParsedConfig()
	if err != nil {
		return err
	}
	switch c.Type {
	case "web_push":
		// No URL on the channel itself — actual targets live in the
		// web_push_subscriptions table and are bound per browser via the
		// subscribe endpoint. A channel with zero subscriptions is valid
		// (silently no-ops at dispatch time) so the operator can create
		// the channel first and subscribe browsers afterwards.
	case "telegram":
		if strings.TrimSpace(cc.BotToken) == "" {
			return ErrTelegramBotRequired
		}
		if strings.TrimSpace(cc.ChatID) == "" {
			return ErrTelegramChatRequired
		}
	case "email":
		if strings.TrimSpace(cc.SmtpHost) == "" {
			return ErrEmailHostRequired
		}
		if cc.SmtpPort < 1 || cc.SmtpPort > 65535 {
			return ErrEmailPortInvalid
		}
		if strings.TrimSpace(cc.Username) == "" || strings.TrimSpace(cc.Password) == "" {
			return ErrEmailCredsRequired
		}
		if !looksLikeEmail(cc.FromAddr) {
			if strings.TrimSpace(cc.FromAddr) == "" {
				return ErrEmailFromRequired
			}
			return fmt.Errorf("%w: from_addr=%q", ErrEmailAddrInvalid, cc.FromAddr)
		}
		// RFC 0004: comma-separated to_addr, per-recipient validation,
		// hard cap at MaxEmailRecipients. Reject any input that
		// LOOKS like a multi-recipient list but has a trailing comma
		// (catches "ops@example.com," where the operator forgot the
		// second address — the SMTP server would silently drop the
		// empty piece at RCPT TO and the operator would assume the
		// message went to two addresses).
		raw := cc.ToAddr
		if strings.TrimSpace(raw) == "" {
			return ErrEmailToRequired
		}
		if strings.HasSuffix(strings.TrimSpace(raw), ",") {
			return fmt.Errorf("%w: trailing comma in to_addr", ErrEmailAddrInvalid)
		}
		// Reject empty middle pieces ("a@x.com, , b@x.com") — the
		// SMTP server would silently drop the empty piece at RCPT TO
		// and the operator would assume the message went to three
		// addresses. Better to fail loudly at save time.
		for _, p := range strings.Split(raw, ",") {
			if strings.TrimSpace(p) == "" {
				return fmt.Errorf("%w: empty piece in to_addr", ErrEmailAddrInvalid)
			}
		}
		addrs := SplitEmailRecipients(raw)
		if len(addrs) == 0 {
			return ErrEmailToRequired
		}
		if len(addrs) > MaxEmailRecipients {
			return fmt.Errorf("email: too many recipients (%d > %d)", len(addrs), MaxEmailRecipients)
		}
		for _, a := range addrs {
			if !looksLikeEmail(a) {
				return fmt.Errorf("%w: to_addr=%q", ErrEmailAddrInvalid, a)
			}
		}
	case "slack":
		if strings.TrimSpace(cc.URL) == "" {
			return ErrSlackURLRequired
		}
		if !isSlackIncomingWebhookURL(cc.URL) {
			return ErrSlackURLInvalid
		}
	default:
		if strings.TrimSpace(cc.URL) == "" {
			return ErrChannelURLRequired
		}
		u, perr := url.Parse(cc.URL)
		if perr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return ErrChannelURLNotHTTP
		}
	}
	// RFC 0004: digest_window is allowed on every channel type. We
	// validate it AFTER the per-type switch so a typo in another
	// field short-circuits the error message (rather than showing
	// "digest_window invalid" when the real problem is a missing
	// bot_token).
	if _, err := ParseDigestWindow(cc.DigestWindow); err != nil {
		return err
	}
	return nil
}

// isSlackIncomingWebhookURL enforces the Slack Incoming Webhook shape:
// `https://hooks.slack.com/services/...`. The /services/ segment is the
// discriminator — the host + scheme alone aren't enough because
// hooks.slack.com serves both the Webhook redirector AND a static
// "install this app" page that operators might paste by mistake.
func isSlackIncomingWebhookURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	if u.Scheme != "https" || u.Host != "hooks.slack.com" {
		return false
	}
	return strings.HasPrefix(u.Path, "/services/")
}

// MaxEmailRecipients is the upper bound on the comma-separated
// to_addr list for an email channel. RFC 0004 picked 20 because
// SMTP fan-out beyond ~30 starts to trigger greylist / rate-limit
// issues on common relays (Gmail, SES, Postfix defaults) — 20 is
// the largest value the homelab target wants to support without
// rethinking the envelope.
const MaxEmailRecipients = 20

// SplitEmailRecipients turns a comma-separated to_addr into a
// cleaned, dedup-empty slice. The behaviour matches the email
// test's expectations:
//   - "a@x.com"          → ["a@x.com"]
//   - "a, b"             → ["a", "b"]   (whitespace trimmed)
//   - "a,,b"             → ["a", "b"]   (empty pieces dropped)
//   - ""                 → nil          (caller validates the empty case)
//
// We don't dedup at this layer — that's a higher-level concern
// (caller can decide whether dupes are a save error or a
// silent collapse). Splitting is pure string work so it lives
// next to looksLikeEmail for testability.
func SplitEmailRecipients(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// looksLikeEmail is a deliberately loose check — RFC 5322 is too permissive
// to encode in a regex usefully, and the SMTP server will reject a bad
// address at RCPT anyway. We just want to catch operator typos like
// missing "@" or stray spaces before the message hits the wire.
func looksLikeEmail(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	at := strings.LastIndex(s, "@")
	if at < 1 || at == len(s)-1 {
		return false
	}
	if strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	// Domain must have at least one dot or be a bare hostname accepted by
	// the SMTP server (loopback dev). Don't enforce the dot — local test
	// setups (mailhog at "mailhog") would break.
	return true
}

func ListChannels(ctx context.Context, db *sql.DB) ([]Channel, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT `+channelColumns+` FROM notification_channels ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Channel, 0)
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListEnabledChannels is what the engine dispatches into on every event.
func ListEnabledChannels(ctx context.Context, db *sql.DB) ([]Channel, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+channelColumns+` FROM notification_channels
		WHERE enabled = 1 AND owner_type = 'admin'
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Channel, 0)
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func GetChannel(ctx context.Context, db *sql.DB, id int64) (Channel, error) {
	c, err := scanChannel(db.QueryRowContext(ctx,
		`SELECT `+channelColumns+` FROM notification_channels WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Channel{}, ErrChannelNotFound
	}
	return c, err
}

func CreateChannel(ctx context.Context, db *sql.DB, c Channel) (Channel, error) {
	if err := validateChannel(&c); err != nil {
		return Channel{}, err
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO notification_channels (name, type, config, owner_type, enabled, min_severity)
		VALUES (?, ?, ?, ?, ?, ?)`,
		c.Name, c.Type, c.Config, c.OwnerType, boolToInt(c.Enabled), c.MinSeverity,
	)
	if err != nil {
		return Channel{}, fmt.Errorf("insert channel: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Channel{}, err
	}
	return GetChannel(ctx, db, id)
}

func UpdateChannel(ctx context.Context, db *sql.DB, c Channel) (Channel, error) {
	if c.ID <= 0 {
		return Channel{}, ErrChannelNotFound
	}
	if err := validateChannel(&c); err != nil {
		return Channel{}, err
	}
	res, err := db.ExecContext(ctx, `
		UPDATE notification_channels SET
			name = ?, type = ?, config = ?, enabled = ?, min_severity = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		c.Name, c.Type, c.Config, boolToInt(c.Enabled), c.MinSeverity, c.ID,
	)
	if err != nil {
		return Channel{}, fmt.Errorf("update channel: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Channel{}, err
	}
	if n == 0 {
		return Channel{}, ErrChannelNotFound
	}
	return GetChannel(ctx, db, c.ID)
}

// SeverityRank lets the engine compare a channel's min_severity against
// an event's severity. Unknown values rank as info so we err on the side
// of delivery.
func SeverityRank(s string) int {
	switch s {
	case "warning":
		return 1
	case "critical":
		return 2
	default:
		return 0
	}
}

// ChannelsForRule returns the channels the engine should fan out to for
// the given rule. Empty link set → fall back to "every enabled admin
// channel" (Milestone-A behaviour, preserves backward compatibility for
// rules created before routing existed).
func ChannelsForRule(ctx context.Context, db *sql.DB, ruleID int64) ([]Channel, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+prefixCols(channelColumns, "c.")+`
		FROM notification_channels c
		JOIN alert_rule_channels rc ON rc.channel_id = c.id
		WHERE rc.rule_id = ? AND c.enabled = 1 AND c.owner_type = 'admin'
		ORDER BY c.id`, ruleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Channel, 0)
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > 0 {
		return out, nil
	}
	return ListEnabledChannels(ctx, db)
}

// SetRuleChannels replaces the routing link set for a rule. Empty/nil
// `channelIDs` clears all links → restores broadcast-to-all behaviour.
// Caller validates that channel IDs exist (FK constraint will also error).
func SetRuleChannels(ctx context.Context, db *sql.DB, ruleID int64, channelIDs []int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM alert_rule_channels WHERE rule_id = ?`, ruleID); err != nil {
		return fmt.Errorf("clear rule channels: %w", err)
	}
	if len(channelIDs) > 0 {
		stmt, err := tx.PrepareContext(ctx,
			`INSERT INTO alert_rule_channels (rule_id, channel_id) VALUES (?, ?)`)
		if err != nil {
			return fmt.Errorf("prepare rule channel insert: %w", err)
		}
		defer stmt.Close()
		seen := map[int64]struct{}{}
		for _, cid := range channelIDs {
			if cid <= 0 {
				continue
			}
			if _, dup := seen[cid]; dup {
				continue
			}
			seen[cid] = struct{}{}
			if _, err := stmt.ExecContext(ctx, ruleID, cid); err != nil {
				return fmt.Errorf("insert rule channel %d: %w", cid, err)
			}
		}
	}
	return tx.Commit()
}

// GetRuleChannelIDs is the read side of SetRuleChannels — used by the API
// to render the rule edit form.
func GetRuleChannelIDs(ctx context.Context, db *sql.DB, ruleID int64) ([]int64, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT channel_id FROM alert_rule_channels WHERE rule_id = ? ORDER BY channel_id`,
		ruleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// prefixCols rewrites a `id, name, ...` column list to `pfx.id, pfx.name, ...`
// so the same column constant works in JOIN queries.
func prefixCols(cols, pfx string) string {
	parts := strings.Split(cols, ",")
	for i, p := range parts {
		parts[i] = pfx + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

func DeleteChannel(ctx context.Context, db *sql.DB, id int64) error {
	res, err := db.ExecContext(ctx, `DELETE FROM notification_channels WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrChannelNotFound
	}
	return nil
}
