package alerts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type Handlers struct {
	DB        *sql.DB
	HubSecret []byte // optional; required only when the Test endpoint dispatches a web_push channel
	Logger *slog.Logger
}

func NewHandlers(db *sql.DB, logger *slog.Logger) *Handlers {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handlers{DB: db, Logger: logger}
}

type ruleView struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Metric          string  `json:"metric"`
	Comparator      string  `json:"comparator"`
	Threshold       float64 `json:"threshold"`
	ForSeconds      int     `json:"for_seconds"`
	CooldownSeconds int     `json:"cooldown_seconds"`
	Host            string  `json:"host"`
	HostSelector    string  `json:"host_selector"`
	Severity        string  `json:"severity"`
	Enabled         bool    `json:"enabled"`
	ChannelIDs      []int64 `json:"channel_ids"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type ruleWrite struct {
	Name            string  `json:"name"`
	Metric          string  `json:"metric"`
	Comparator      string  `json:"comparator"`
	Threshold       float64 `json:"threshold"`
	ForSeconds      int     `json:"for_seconds"`
	CooldownSeconds int     `json:"cooldown_seconds"`
	Host            string  `json:"host"`
	HostSelector    string  `json:"host_selector"`
	Severity        string  `json:"severity"`
	Enabled         *bool   `json:"enabled"`
	// ChannelIDs selects which notification_channels this rule fans out
	// to. nil = unchanged on UPDATE; empty array = clear all links →
	// fall back to broadcast (every enabled channel).
	ChannelIDs *[]int64 `json:"channel_ids"`
}

func ruleToView(r Rule, channelIDs []int64) ruleView {
	if channelIDs == nil {
		channelIDs = []int64{}
	}
	return ruleView{
		ID:              r.ID,
		Name:            r.Name,
		Metric:          r.Metric,
		Comparator:      r.Comparator,
		Threshold:       r.Threshold,
		ForSeconds:      r.ForSeconds,
		CooldownSeconds: r.CooldownSeconds,
		Host:            r.Host,
		HostSelector:    r.HostSelector,
		Severity:        r.Severity,
		Enabled:         r.Enabled,
		ChannelIDs:      channelIDs,
		CreatedAt:       r.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:       r.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (w ruleWrite) toRule(id int64) Rule {
	r := Rule{
		ID:              id,
		Name:            w.Name,
		Metric:          w.Metric,
		Comparator:      w.Comparator,
		Threshold:       w.Threshold,
		ForSeconds:      w.ForSeconds,
		CooldownSeconds: w.CooldownSeconds,
		Host:            w.Host,
		HostSelector:    w.HostSelector,
		Severity:        w.Severity,
		Enabled:         true,
	}
	if w.Enabled != nil {
		r.Enabled = *w.Enabled
	}
	if r.Comparator == "" {
		r.Comparator = "gt"
	}
	if r.Severity == "" {
		r.Severity = "warning"
	}
	return r
}

// GET /api/alerts/rules
func (h *Handlers) ListRules(w http.ResponseWriter, r *http.Request) {
	rules, err := ListRules(r.Context(), h.DB)
	if err != nil {
		h.Logger.Error("alerts: list rules failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	out := make([]ruleView, 0, len(rules))
	for _, x := range rules {
		ids, err := GetRuleChannelIDs(r.Context(), h.DB, x.ID)
		if err != nil {
			h.Logger.Error("alerts: read rule channels failed", "err", err, "rule_id", x.ID)
			writeJSONError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		out = append(out, ruleToView(x, ids))
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/alerts/rules
func (h *Handlers) CreateRule(w http.ResponseWriter, r *http.Request) {
	var req ruleWrite
	if err := decodeStrict(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	rule, err := CreateRule(r.Context(), h.DB, req.toRule(0))
	if err != nil {
		if isValidationErr(err) {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.Logger.Error("alerts: create rule failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if req.ChannelIDs != nil {
		if err := SetRuleChannels(r.Context(), h.DB, rule.ID, *req.ChannelIDs); err != nil {
			h.Logger.Error("alerts: set rule channels failed", "err", err, "rule_id", rule.ID)
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	ids, _ := GetRuleChannelIDs(r.Context(), h.DB, rule.ID)
	writeJSON(w, http.StatusCreated, ruleToView(rule, ids))
}

// PUT /api/alerts/rules/{id}
func (h *Handlers) UpdateRule(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req ruleWrite
	if err := decodeStrict(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	rule, err := UpdateRule(r.Context(), h.DB, req.toRule(id))
	if err != nil {
		if errors.Is(err, ErrRuleNotFound) {
			writeJSONError(w, http.StatusNotFound, "rule not found")
			return
		}
		if isValidationErr(err) {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.Logger.Error("alerts: update rule failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if req.ChannelIDs != nil {
		if err := SetRuleChannels(r.Context(), h.DB, rule.ID, *req.ChannelIDs); err != nil {
			h.Logger.Error("alerts: set rule channels failed", "err", err, "rule_id", rule.ID)
			writeJSONError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	ids, _ := GetRuleChannelIDs(r.Context(), h.DB, rule.ID)
	writeJSON(w, http.StatusOK, ruleToView(rule, ids))
}

// DELETE /api/alerts/rules/{id}
func (h *Handlers) DeleteRule(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := DeleteRule(r.Context(), h.DB, id); err != nil {
		if errors.Is(err, ErrRuleNotFound) {
			writeJSONError(w, http.StatusNotFound, "rule not found")
			return
		}
		h.Logger.Error("alerts: delete rule failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- channels ---

type channelView struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Config      json.RawMessage `json:"config"`
	OwnerType   string          `json:"owner_type"`
	Enabled     bool            `json:"enabled"`
	MinSeverity string          `json:"min_severity"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

type channelWrite struct {
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Config      json.RawMessage `json:"config"`
	Enabled     *bool           `json:"enabled"`
	MinSeverity string          `json:"min_severity"`
}

// maskedConfig redacts channel-secret fields before sending the channel
// back to the UI. Operators paste secrets once; the UI shows a fixed-
// length mask afterwards. Channels that embed the secret in the URL
// itself (ntfy topic, Discord webhook ID) can't be cleanly masked
// without losing edit-form usefulness — treat those as the operator-
// managed channel-creation contract.
//
// Currently masks:
//   - telegram.bot_token
//   - email.password
func maskedConfig(c Channel) json.RawMessage {
	if c.Config == "" {
		return json.RawMessage("{}")
	}
	if c.Type != "telegram" && c.Type != "email" {
		return json.RawMessage(c.Config)
	}
	cfg, err := c.ParsedConfig()
	if err != nil {
		return json.RawMessage(c.Config)
	}
	switch c.Type {
	case "telegram":
		if cfg.BotToken != "" {
			cfg.BotToken = secretMask
		}
	case "email":
		if cfg.Password != "" {
			cfg.Password = secretMask
		}
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return json.RawMessage(c.Config)
	}
	return b
}

// secretMask is the placeholder the UI sees in place of any stored
// secret field (telegram bot_token, email password). The PUT handler
// treats this exact value as "keep existing" so the operator can edit
// other config fields without re-typing the secret. The FE references
// the same literal via its own `TELEGRAM_TOKEN_MASK` constant.
const secretMask = "**********"

// preserveSecrets merges existing stored secrets into the incoming
// config when an incoming field equals the mask. Returns the original
// incoming bytes unchanged on any error or when no field was masked.
//
// Covers telegram.bot_token and email.password.
func preserveSecrets(incoming json.RawMessage, existing Channel) json.RawMessage {
	if len(incoming) == 0 {
		return incoming
	}
	var cfg ChannelConfig
	if err := json.Unmarshal(incoming, &cfg); err != nil {
		return incoming
	}
	prev, err := existing.ParsedConfig()
	if err != nil {
		return incoming
	}
	changed := false
	if cfg.BotToken == secretMask && prev.BotToken != "" {
		cfg.BotToken = prev.BotToken
		changed = true
	}
	if cfg.Password == secretMask && prev.Password != "" {
		cfg.Password = prev.Password
		changed = true
	}
	if !changed {
		return incoming
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return incoming
	}
	return b
}

func channelToView(c Channel) channelView {
	return channelView{
		ID:          c.ID,
		Name:        c.Name,
		Type:        c.Type,
		Config:      maskedConfig(c),
		OwnerType:   c.OwnerType,
		Enabled:     c.Enabled,
		MinSeverity: c.MinSeverity,
		CreatedAt:   c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   c.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (wr channelWrite) toChannel(id int64) Channel {
	c := Channel{
		ID:          id,
		Name:        wr.Name,
		Type:        wr.Type,
		Config:      string(wr.Config),
		OwnerType:   "admin",
		Enabled:     true,
		MinSeverity: wr.MinSeverity,
	}
	if wr.Enabled != nil {
		c.Enabled = *wr.Enabled
	}
	if len(wr.Config) == 0 {
		c.Config = "{}"
	}
	if c.MinSeverity == "" {
		c.MinSeverity = "info"
	}
	return c
}

// GET /api/alerts/channels
func (h *Handlers) ListChannels(w http.ResponseWriter, r *http.Request) {
	channels, err := ListChannels(r.Context(), h.DB)
	if err != nil {
		h.Logger.Error("alerts: list channels failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	out := make([]channelView, 0, len(channels))
	for _, x := range channels {
		out = append(out, channelToView(x))
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/alerts/channels
func (h *Handlers) CreateChannel(w http.ResponseWriter, r *http.Request) {
	var req channelWrite
	if err := decodeStrict(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	ch, err := CreateChannel(r.Context(), h.DB, req.toChannel(0))
	if err != nil {
		if isValidationErr(err) {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.Logger.Error("alerts: create channel failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, channelToView(ch))
}

// PUT /api/alerts/channels/{id}
func (h *Handlers) UpdateChannel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req channelWrite
	if err := decodeStrict(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	// If the client sent the secret-mask placeholder for a masked field
	// (telegram bot_token, email password), splice in the stored value
	// so the operator can edit other fields without re-pasting the
	// secret. Type must match the existing row — switching channel type
	// shouldn't carry secrets across kinds.
	if req.Type == "telegram" || req.Type == "email" {
		existing, exErr := GetChannel(r.Context(), h.DB, id)
		if exErr == nil && existing.Type == req.Type {
			req.Config = preserveSecrets(req.Config, existing)
		}
	}
	ch, err := UpdateChannel(r.Context(), h.DB, req.toChannel(id))
	if err != nil {
		if errors.Is(err, ErrChannelNotFound) {
			writeJSONError(w, http.StatusNotFound, "channel not found")
			return
		}
		if isValidationErr(err) {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.Logger.Error("alerts: update channel failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, channelToView(ch))
}

// DELETE /api/alerts/channels/{id}
func (h *Handlers) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := DeleteChannel(r.Context(), h.DB, id); err != nil {
		if errors.Is(err, ErrChannelNotFound) {
			writeJSONError(w, http.StatusNotFound, "channel not found")
			return
		}
		h.Logger.Error("alerts: delete channel failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/alerts/channels/{id}/test — dispatches a synthetic
// notification synchronously and reports success/failure.
func (h *Handlers) TestChannel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ch, err := GetChannel(r.Context(), h.DB, id)
	if err != nil {
		if errors.Is(err, ErrChannelNotFound) {
			writeJSONError(w, http.StatusNotFound, "channel not found")
			return
		}
		h.Logger.Error("alerts: load channel failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	notif := Notification{
		RuleID:    0,
		RuleName:  "Lumen test",
		Host:      "test-host",
		Metric:    "cpu_pct",
		Severity:  "info",
		State:     "firing",
		Value:     42,
		Threshold: 40,
		Time:      time.Now().UTC(),
	}
	notif.Message = FormatMessage(notif) + " (test ping from Lumen)"
	if err := Dispatch(ctx, ch, notif, DispatchDeps{DB: h.DB, HubSecret: h.HubSecret}, h.Logger); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"ok":    "false",
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- events ---

type eventView struct {
	ID         int64   `json:"id"`
	RuleID     int64   `json:"rule_id"`
	RuleName   string  `json:"rule_name"`
	Host       string  `json:"host"`
	Metric     string  `json:"metric"`
	Severity   string  `json:"severity"`
	State      string  `json:"state"`
	Value      float64 `json:"value"`
	Message    string  `json:"message"`
	StartedAt  string  `json:"started_at"`
	ResolvedAt *string `json:"resolved_at"`
}

// GET /api/alerts/events?state=firing|resolved|all&limit=N
//
// state=firing   → currently breaching, no resolved_at
// state=resolved → past incidents that already cleared
// state=all      → both (default; useful for raw audit/API consumers)
//
// The UI keeps firing on the "Active" tab and resolved on the "History"
// tab so the same event never appears in both — the old default of
// `all` for History was confusing once a long-firing incident showed up
// in both lists.
func (h *Handlers) ListEvents(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		state = "all"
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	// Cap at 2000 — the UI's "Load more" button steps in 200-row pages and
	// settles around a few hundred rows in practice. 2000 is the ceiling
	// before a SELECT on a busy 100k-row table starts to feel slow to a
	// homelab operator; cold-tier reads (Phase 7) take over above that.
	if limit <= 0 || limit > 2000 {
		limit = 100
	}
	var (
		rows *sql.Rows
		err  error
	)
	switch state {
	case "firing":
		rows, err = h.DB.QueryContext(r.Context(), `
			SELECT id, rule_id, rule_name, host, metric, severity, state,
				value, message, started_at, resolved_at
			FROM alert_events
			WHERE state = 'firing'
			ORDER BY started_at DESC, id DESC
			LIMIT ?`, limit)
	case "resolved":
		rows, err = h.DB.QueryContext(r.Context(), `
			SELECT id, rule_id, rule_name, host, metric, severity, state,
				value, message, started_at, resolved_at
			FROM alert_events
			WHERE state = 'resolved'
			ORDER BY resolved_at DESC, id DESC
			LIMIT ?`, limit)
	default:
		rows, err = h.DB.QueryContext(r.Context(), `
			SELECT id, rule_id, rule_name, host, metric, severity, state,
				value, message, started_at, resolved_at
			FROM alert_events
			ORDER BY started_at DESC, id DESC
			LIMIT ?`, limit)
	}
	if err != nil {
		h.Logger.Error("alerts: list events failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	defer rows.Close()
	out := make([]eventView, 0)
	for rows.Next() {
		var (
			v          eventView
			value      sql.NullFloat64
			resolved   sql.NullTime
			startedRaw time.Time
		)
		if err := rows.Scan(&v.ID, &v.RuleID, &v.RuleName, &v.Host, &v.Metric,
			&v.Severity, &v.State, &value, &v.Message, &startedRaw, &resolved); err != nil {
			h.Logger.Error("alerts: scan event failed", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		if value.Valid {
			v.Value = value.Float64
		}
		v.StartedAt = startedRaw.UTC().Format(time.RFC3339)
		if resolved.Valid {
			s := resolved.Time.UTC().Format(time.RFC3339)
			v.ResolvedAt = &s
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		h.Logger.Error("alerts: rows error", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "scan failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// --- deliveries ---

type deliveryView struct {
	ID           int64           `json:"id"`
	EventID      int64           `json:"event_id"`
	ChannelID    int64           `json:"channel_id"`
	ChannelName  string          `json:"channel_name"`
	ChannelType  string          `json:"channel_type"`
	Severity     string          `json:"severity"`
	Status       string          `json:"status"`
	Attempts     int             `json:"attempts"`
	HTTPStatus   *int            `json:"http_status"`
	Error        *string         `json:"error"`
	NextRetryAt  *string         `json:"next_retry_at"`
	Payload      json.RawMessage `json:"payload"`
	CreatedAt    string          `json:"created_at"`
	SentAt       *string         `json:"sent_at"`
}

// GET /api/alerts/deliveries?status=&channel_id=&severity=&limit=
//
// Filter combinations: anything blank = "no filter". Newest-first by
// created_at. Limit defaults to 100, max 2000 — same ceiling as
// ListEvents so the Deliveries tab's "Load more" can scroll back
// through the burst that produced a flood of rows.
func (h *Handlers) ListDeliveries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 2000 {
		limit = 100
	}

	conds := []string{"1=1"}
	args := []any{}
	if s := q.Get("status"); s != "" {
		conds = append(conds, "status = ?")
		args = append(args, s)
	}
	if cid := q.Get("channel_id"); cid != "" {
		id, err := strconv.ParseInt(cid, 10, 64)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid channel_id")
			return
		}
		conds = append(conds, "channel_id = ?")
		args = append(args, id)
	}
	if s := q.Get("severity"); s != "" {
		conds = append(conds, "severity = ?")
		args = append(args, s)
	}
	args = append(args, limit)

	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT id, event_id, channel_id, channel_name, channel_type,
			severity, status, attempts, http_status, error,
			next_retry_at, payload, created_at, sent_at
		FROM notification_deliveries
		WHERE `+strings.Join(conds, " AND ")+`
		ORDER BY created_at DESC, id DESC
		LIMIT ?`, args...)
	if err != nil {
		h.Logger.Error("alerts: list deliveries failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	defer rows.Close()

	out := make([]deliveryView, 0)
	for rows.Next() {
		var (
			v          deliveryView
			httpStatus sql.NullInt64
			errStr     sql.NullString
			nextRetry  sql.NullTime
			payload    string
			created    time.Time
			sent       sql.NullTime
		)
		if err := rows.Scan(&v.ID, &v.EventID, &v.ChannelID, &v.ChannelName, &v.ChannelType,
			&v.Severity, &v.Status, &v.Attempts, &httpStatus, &errStr,
			&nextRetry, &payload, &created, &sent); err != nil {
			h.Logger.Error("alerts: scan delivery failed", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		if httpStatus.Valid {
			n := int(httpStatus.Int64)
			v.HTTPStatus = &n
		}
		if errStr.Valid {
			s := errStr.String
			v.Error = &s
		}
		if nextRetry.Valid {
			s := nextRetry.Time.UTC().Format(time.RFC3339)
			v.NextRetryAt = &s
		}
		if payload == "" {
			payload = "{}"
		}
		v.Payload = json.RawMessage(payload)
		v.CreatedAt = created.UTC().Format(time.RFC3339)
		if sent.Valid {
			s := sent.Time.UTC().Format(time.RFC3339)
			v.SentAt = &s
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		h.Logger.Error("alerts: rows error", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "scan failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/alerts/deliveries/{id}/retry
//
// Resets a failed/dropped row back to pending with no delay, so the
// next dispatcher tick picks it up. Useful after a channel URL fix.
func (h *Handlers) RetryDelivery(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	res, err := h.DB.ExecContext(r.Context(), `
		UPDATE notification_deliveries
		SET status = 'pending', next_retry_at = NULL,
			-- reset the attempt counter so the operator gets the full
			-- backoff schedule again on this manual retry
			attempts = 0,
			error = NULL
		WHERE id = ? AND status IN ('failed', 'dropped')`, id)
	if err != nil {
		h.Logger.Error("alerts: retry delivery failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		writeJSONError(w, http.StatusNotFound, "no failed/dropped delivery with that id")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "pending"})
}

// --- helpers ---

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func decodeStrict(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// isValidationErr is true for any validation sentinel from rules.go or
// channels.go — the handler maps these to 400 instead of 500.
func isValidationErr(err error) bool {
	switch {
	case errors.Is(err, ErrRuleNameRequired),
		errors.Is(err, ErrInvalidMetric),
		errors.Is(err, ErrInvalidComparator),
		errors.Is(err, ErrInvalidSeverity),
		errors.Is(err, ErrNegativeForSeconds),
		errors.Is(err, ErrNegativeCooldown),
		errors.Is(err, ErrChannelNameRequired),
		errors.Is(err, ErrInvalidChannelType),
		errors.Is(err, ErrChannelURLRequired),
		errors.Is(err, ErrChannelURLNotHTTP),
		errors.Is(err, ErrChannelConfigInvalid),
		errors.Is(err, ErrInvalidMinSeverity),
		errors.Is(err, ErrTelegramBotRequired),
		errors.Is(err, ErrTelegramChatRequired),
		errors.Is(err, ErrEmailHostRequired),
		errors.Is(err, ErrEmailPortInvalid),
		errors.Is(err, ErrEmailCredsRequired),
		errors.Is(err, ErrEmailFromRequired),
		errors.Is(err, ErrEmailToRequired),
		errors.Is(err, ErrEmailAddrInvalid):
		return true
	}
	return false
}
