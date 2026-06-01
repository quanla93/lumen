package publicapi

import (
	"database/sql"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/quanla93/lumen/internal/hub/apikey"
	"github.com/quanla93/lumen/internal/hub/hosts"
	"github.com/quanla93/lumen/internal/hub/storage"
)

// Handlers groups the v1 endpoint handlers. BuildVersion is the hub's
// build-time version string (passed through to /api/v1/version) — kept
// distinct from the Version method below so the two don't collide at
// compile time.
type Handlers struct {
	DB           *sql.DB
	Logger       *slog.Logger
	BuildVersion string
}

func NewHandlers(db *sql.DB, buildVersion string, logger *slog.Logger) *Handlers {
	return &Handlers{DB: db, Logger: logger, BuildVersion: buildVersion}
}

// ─── GET /api/v1/version ─────────────────────────────────────────────────

type versionResp struct {
	Version string `json:"version"`
}

// Version is the public ping. Auth required (so revoked keys can't poll
// it), but no specific scope — any authenticated caller succeeds.
func (h *Handlers) Version(w http.ResponseWriter, r *http.Request) {
	WriteSuccess(w, r, http.StatusOK, versionResp{Version: h.BuildVersion})
}

// ─── GET /api/v1/hosts ───────────────────────────────────────────────────

type hostItem struct {
	Name       string  `json:"name"`
	CreatedAt  string  `json:"created_at"`
	LastSeenAt *string `json:"last_seen_at"`
}

type hostsResp struct {
	Hosts []hostItem `json:"hosts"`
}

// Hosts returns the host list filtered by the key's host_filter glob
// (if set). Requires read:hosts scope.
func (h *Handlers) Hosts(w http.ResponseWriter, r *http.Request) {
	key := KeyFromContext(r.Context())
	all, err := hosts.List(r.Context(), h.DB)
	if err != nil {
		h.Logger.Error("v1 hosts list", "err", err)
		WriteError(w, r, http.StatusInternalServerError, CodeInternalError, "list failed")
		return
	}
	out := make([]hostItem, 0, len(all))
	for _, host := range all {
		if !matchesFilter(host.Name, key) {
			continue
		}
		item := hostItem{
			Name:      host.Name,
			CreatedAt: host.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		}
		if host.LastSeenAt.Valid {
			s := host.LastSeenAt.Time.UTC().Format("2006-01-02T15:04:05Z")
			item.LastSeenAt = &s
		}
		out = append(out, item)
	}
	WriteSuccess(w, r, http.StatusOK, hostsResp{Hosts: out})
}

// matchesFilter returns true if a host name passes the key's
// host_filter glob. Empty/nil filter = everything passes. path.Match
// errors (e.g. malformed glob) currently fail open — operator-supplied
// pattern was validated at write time, so a runtime error means we
// shipped a regression; logging would belong here but a misbehaving
// glob shouldn't 500 the whole list.
func matchesFilter(name string, key *apikey.Key) bool {
	if key == nil || key.HostFilter == nil || *key.HostFilter == "" {
		return true
	}
	ok, err := path.Match(*key.HostFilter, name)
	if err != nil {
		return true
	}
	return ok
}

// ─── GET /api/v1/hosts/{name} ────────────────────────────────────────────

// HostDetail returns a single host with last-seen + tags. Requires
// read:hosts. 404 if the host is unknown OR the key's host_filter glob
// excludes it (same response either way so a key can't probe for the
// existence of hosts outside its filter).
func (h *Handlers) HostDetail(w http.ResponseWriter, r *http.Request) {
	key := KeyFromContext(r.Context())
	name := chi.URLParam(r, "name")
	if name == "" {
		WriteError(w, r, http.StatusBadRequest, CodeBadRequest, "missing host name")
		return
	}
	if !matchesFilter(name, key) {
		WriteError(w, r, http.StatusNotFound, CodeNotFound, "host not found")
		return
	}
	all, err := hosts.List(r.Context(), h.DB)
	if err != nil {
		h.Logger.Error("v1 host detail list", "err", err)
		WriteError(w, r, http.StatusInternalServerError, CodeInternalError, "lookup failed")
		return
	}
	for _, host := range all {
		if host.Name != name {
			continue
		}
		item := hostItem{
			Name:      host.Name,
			CreatedAt: host.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		}
		if host.LastSeenAt.Valid {
			s := host.LastSeenAt.Time.UTC().Format("2006-01-02T15:04:05Z")
			item.LastSeenAt = &s
		}
		WriteSuccess(w, r, http.StatusOK, item)
		return
	}
	WriteError(w, r, http.StatusNotFound, CodeNotFound, "host not found")
}

// ─── GET /api/v1/hosts/{name}/metrics ────────────────────────────────────

const (
	// metricsMaxRange caps how far back any single query can ask for.
	// Cold tier would relax this; until then, 7d is enough for any UI
	// chart Lumen ships and small enough to scan SQLite quickly.
	metricsMaxRange = 7 * 24 * time.Hour
	// metricsMinBucket enforces a downsampled response — raw 5s points
	// for a week would be 120k rows and saturate any chart.
	metricsMinBucket = 30 * time.Second
	// metricsMaxPoints is the hard cap on returned points per query.
	// Combined with a 7d range it implies bucket ≥ ~10m on the long
	// end; bucket=30s on shorter ranges is fine.
	metricsMaxPoints = 1000
)

type metricPointOut struct {
	Ts       string  `json:"ts"`
	CpuPct   float64 `json:"cpu_pct"`
	RamPct   float64 `json:"ram_pct"`
	SwapPct  float64 `json:"swap_pct"`
	DiskPct  float64 `json:"disk_pct"`
	Load1    float64 `json:"load1"`
	Load5    float64 `json:"load5"`
	Load15   float64 `json:"load15"`
	NetRxBps float64 `json:"net_rx_bps"`
	NetTxBps float64 `json:"net_tx_bps"`
	DiskRBps float64 `json:"disk_r_bps"`
	DiskWBps float64 `json:"disk_w_bps"`
	TempC    float64 `json:"temp_c"`
}

type metricsResp struct {
	Host         string           `json:"host"`
	From         string           `json:"from"`
	To           string           `json:"to"`
	BucketSec    int64            `json:"bucket_seconds"`
	Points       []metricPointOut `json:"points"`
}

// HostMetrics returns downsampled time-series for one host between
// [from, to). Required query params: from (RFC3339), to (RFC3339),
// bucket (Go duration like "1m", "5m"). Requires read:metrics.
//
// Out-of-band: the metricsMaxRange / metricsMinBucket / metricsMaxPoints
// caps above are the only knobs separating this from a denial-of-wallet
// vector once the API key gets shared widely. They're conservative on
// purpose — a real homelab dashboard never needs more than 1000 points
// at a 30s+ resolution.
func (h *Handlers) HostMetrics(w http.ResponseWriter, r *http.Request) {
	key := KeyFromContext(r.Context())
	name := chi.URLParam(r, "name")
	if name == "" {
		WriteError(w, r, http.StatusBadRequest, CodeBadRequest, "missing host name")
		return
	}
	if !matchesFilter(name, key) {
		WriteError(w, r, http.StatusNotFound, CodeNotFound, "host not found")
		return
	}
	q := r.URL.Query()
	from, err := time.Parse(time.RFC3339, q.Get("from"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, CodeBadRequest, "from must be RFC3339")
		return
	}
	to, err := time.Parse(time.RFC3339, q.Get("to"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, CodeBadRequest, "to must be RFC3339")
		return
	}
	if !from.Before(to) {
		WriteError(w, r, http.StatusBadRequest, CodeBadRequest, "from must be before to")
		return
	}
	if to.Sub(from) > metricsMaxRange {
		WriteError(w, r, http.StatusBadRequest, CodeBadRequest, "range exceeds 7 days")
		return
	}
	bucket, err := time.ParseDuration(q.Get("bucket"))
	if err != nil {
		WriteError(w, r, http.StatusBadRequest, CodeBadRequest, "bucket must be a Go duration (e.g. 1m, 5m)")
		return
	}
	if bucket < metricsMinBucket {
		WriteError(w, r, http.StatusBadRequest, CodeBadRequest, "bucket must be ≥ 30s")
		return
	}
	if int64(to.Sub(from).Seconds()/bucket.Seconds()) > metricsMaxPoints {
		WriteError(w, r, http.StatusBadRequest, CodeBadRequest, "(to-from)/bucket would exceed 1000 points; increase bucket")
		return
	}

	pts, err := storage.QueryMetrics(r.Context(), h.DB, name, from, to, int64(bucket.Seconds()))
	if err != nil {
		h.Logger.Error("v1 metrics query", "err", err, "host", name)
		WriteError(w, r, http.StatusInternalServerError, CodeInternalError, "query failed")
		return
	}
	out := make([]metricPointOut, 0, len(pts))
	for _, p := range pts {
		out = append(out, metricPointOut{
			Ts:       p.Ts.Format(time.RFC3339),
			CpuPct:   p.CpuPct,
			RamPct:   p.RamPct,
			SwapPct:  p.SwapPct,
			DiskPct:  p.DiskPct,
			Load1:    p.Load1,
			Load5:    p.Load5,
			Load15:   p.Load15,
			NetRxBps: p.NetRxBps,
			NetTxBps: p.NetTxBps,
			DiskRBps: p.DiskRBps,
			DiskWBps: p.DiskWBps,
			TempC:    p.TempC,
		})
	}
	WriteSuccess(w, r, http.StatusOK, metricsResp{
		Host:      name,
		From:      from.UTC().Format(time.RFC3339),
		To:        to.UTC().Format(time.RFC3339),
		BucketSec: int64(bucket.Seconds()),
		Points:    out,
	})
}

// ─── GET /api/v1/alerts/events ───────────────────────────────────────────

type alertEvent struct {
	ID         int64    `json:"id"`
	RuleID     int64    `json:"rule_id"`
	RuleName   string   `json:"rule_name"`
	Host       string   `json:"host"`
	Metric     string   `json:"metric"`
	Severity   string   `json:"severity"`
	State      string   `json:"state"`
	Value      *float64 `json:"value"`
	Message    string   `json:"message"`
	StartedAt  string   `json:"started_at"`
	ResolvedAt *string  `json:"resolved_at"`
}

type alertEventsResp struct {
	Events []alertEvent `json:"events"`
}

// AlertEvents returns recent alert events, optionally filtered by
// state ("firing" / "resolved" / "all", default "all") and limit
// (default 100, max 500). Host filter glob is enforced — events for
// hosts the key can't see are dropped post-query (the alert table
// doesn't index by glob; over-fetch + filter is fine at homelab
// scale). Requires read:alerts.
func (h *Handlers) AlertEvents(w http.ResponseWriter, r *http.Request) {
	key := KeyFromContext(r.Context())
	q := r.URL.Query()
	state := q.Get("state")
	if state != "firing" && state != "resolved" && state != "all" && state != "" {
		WriteError(w, r, http.StatusBadRequest, CodeBadRequest, "state must be firing|resolved|all")
		return
	}
	if state == "" {
		state = "all"
	}
	limit := 100
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 500 {
			WriteError(w, r, http.StatusBadRequest, CodeBadRequest, "limit must be 1..500")
			return
		}
		limit = n
	}

	var sqlStr string
	switch state {
	case "firing":
		sqlStr = `SELECT id, rule_id, rule_name, host, metric, severity, state,
			value, message, started_at, resolved_at
			FROM alert_events WHERE state = 'firing'
			ORDER BY started_at DESC, id DESC LIMIT ?`
	case "resolved":
		sqlStr = `SELECT id, rule_id, rule_name, host, metric, severity, state,
			value, message, started_at, resolved_at
			FROM alert_events WHERE state = 'resolved'
			ORDER BY resolved_at DESC, id DESC LIMIT ?`
	default:
		sqlStr = `SELECT id, rule_id, rule_name, host, metric, severity, state,
			value, message, started_at, resolved_at
			FROM alert_events ORDER BY started_at DESC, id DESC LIMIT ?`
	}
	rows, err := h.DB.QueryContext(r.Context(), sqlStr, limit)
	if err != nil {
		h.Logger.Error("v1 alert events", "err", err)
		WriteError(w, r, http.StatusInternalServerError, CodeInternalError, "query failed")
		return
	}
	defer rows.Close()

	out := make([]alertEvent, 0, limit)
	for rows.Next() {
		var (
			e          alertEvent
			value      sql.NullFloat64
			resolved   sql.NullTime
			startedRaw time.Time
		)
		if err := rows.Scan(&e.ID, &e.RuleID, &e.RuleName, &e.Host, &e.Metric,
			&e.Severity, &e.State, &value, &e.Message, &startedRaw, &resolved); err != nil {
			h.Logger.Error("v1 alert events scan", "err", err)
			WriteError(w, r, http.StatusInternalServerError, CodeInternalError, "scan failed")
			return
		}
		if !matchesFilter(e.Host, key) {
			continue
		}
		if value.Valid {
			v := value.Float64
			e.Value = &v
		}
		e.StartedAt = startedRaw.UTC().Format(time.RFC3339)
		if resolved.Valid {
			s := resolved.Time.UTC().Format(time.RFC3339)
			e.ResolvedAt = &s
		}
		out = append(out, e)
	}
	WriteSuccess(w, r, http.StatusOK, alertEventsResp{Events: out})
}

// ─── GET /api/v1/alerts/rules ────────────────────────────────────────────

type alertRuleOut struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Enabled         bool    `json:"enabled"`
	Metric          string  `json:"metric"`
	Comparator      string  `json:"comparator"`
	Threshold       float64 `json:"threshold"`
	ForSeconds      int     `json:"for_seconds"`
	CooldownSeconds int     `json:"cooldown_seconds"`
	Severity        string  `json:"severity"`
	HostSelector    string  `json:"host_selector"`
	Host            string  `json:"host"`
}

type alertRulesResp struct {
	Rules []alertRuleOut `json:"rules"`
}

// AlertRules returns the read-only rule inventory. Channels + routing
// are not exposed on the public API — they're operator-internal config.
// Requires read:alerts. No host filtering: rules describe rule-level
// state (selector, threshold), not per-host fact.
func (h *Handlers) AlertRules(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT id, name, metric, comparator, threshold, for_seconds,
			cooldown_seconds, host, host_selector, severity, enabled
		FROM alert_rules ORDER BY id ASC`)
	if err != nil {
		h.Logger.Error("v1 alert rules", "err", err)
		WriteError(w, r, http.StatusInternalServerError, CodeInternalError, "query failed")
		return
	}
	defer rows.Close()

	out := make([]alertRuleOut, 0)
	for rows.Next() {
		var (
			ar      alertRuleOut
			host    sql.NullString
			enabled int
		)
		if err := rows.Scan(&ar.ID, &ar.Name, &ar.Metric, &ar.Comparator, &ar.Threshold,
			&ar.ForSeconds, &ar.CooldownSeconds, &host, &ar.HostSelector, &ar.Severity, &enabled,
		); err != nil {
			h.Logger.Error("v1 alert rules scan", "err", err)
			WriteError(w, r, http.StatusInternalServerError, CodeInternalError, "scan failed")
			return
		}
		if host.Valid {
			ar.Host = host.String
		}
		ar.Enabled = enabled != 0
		out = append(out, ar)
	}
	WriteSuccess(w, r, http.StatusOK, alertRulesResp{Rules: out})
}
