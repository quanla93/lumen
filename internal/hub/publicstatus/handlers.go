// Package publicstatus owns the unauthenticated /status page handler.
//
// Design choices that gave the v0.7 cut a small, predictable surface:
//
//   - Per-host opt-in. Default is hidden. Operators tick the checkbox
//     per host in Settings → Status; the public page only ever lists
//     rows where hosts.public_visible = 1. There is no global "all
//     hosts" toggle, so a misconfiguration can leak at most one host.
//   - State + live metrics. The page shows up/stale/down + CPU/RAM/disk
//     pulled from the in-memory store.Store snapshot so the page reflects
//     the same data the dashboard renders, without a separate query path.
//   - Always 200 OK. Even when the page is disabled, the handler returns
//     {enabled:false} (instead of 404) so the frontend can render a
//     short "this status page isn't published" notice deterministically
//     instead of branching on HTTP status.
//   - No rate limiting yet. The expected audience is the operator's own
//     team or a small Discord/Slack channel; if abuse appears, drop in
//     the same publicapi rate limiter — interface matches.
package publicstatus

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/quanla93/lumen/internal/hub/hosts"
	"github.com/quanla93/lumen/internal/hub/settings"
	"github.com/quanla93/lumen/internal/hub/store"
	"github.com/quanla93/lumen/internal/shared/api"
)

// Settings keys backing the admin config form.
const (
	SettingsKeyEnabled     = "public_status.enabled"
	SettingsKeyTitle       = "public_status.title"
	SettingsKeyDescription = "public_status.description"
)

const (
	defaultTitle = "Status"
	// staleAfter mirrors the dashboard's threshold so the public page and
	// the operator's own view agree on host state. 30s is well above any
	// realistic ingest cadence and well below the alerts engine's eval.
	staleAfter = 30 * time.Second
)

type Handlers struct {
	DB     *sql.DB
	Store  *store.Store
	Logger *slog.Logger
}

func NewHandlers(db *sql.DB, st *store.Store, logger *slog.Logger) *Handlers {
	return &Handlers{DB: db, Store: st, Logger: logger}
}

type publicHostItem struct {
	Name       string  `json:"name"`
	State      string  `json:"state"` // up | stale | down | unknown
	CPUPct     float64 `json:"cpu_pct"`
	RAMPct     float64 `json:"ram_pct"`
	DiskPct    float64 `json:"disk_pct"`
	LastSeenAt string  `json:"last_seen_at,omitempty"`
}

type publicStatusResp struct {
	Enabled     bool             `json:"enabled"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	GeneratedAt string           `json:"generated_at"`
	Hosts       []publicHostItem `json:"hosts"`
}

// GET /api/public/status — unauthenticated public endpoint.
func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	cfg, err := LoadConfig(r.Context(), h.DB)
	if err != nil {
		h.Logger.Error("public status load config", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lookup failed"})
		return
	}
	resp := publicStatusResp{
		Enabled:     cfg.Enabled,
		Title:       cfg.Title,
		Description: cfg.Description,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Hosts:       []publicHostItem{},
	}
	if !cfg.Enabled {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	visibles, err := hosts.ListPublicVisible(r.Context(), h.DB)
	if err != nil {
		h.Logger.Error("public status list visible", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lookup failed"})
		return
	}
	snapMap := snapshotsByName(h.Store.Snapshot())
	for _, host := range visibles {
		item := publicHostItem{Name: host.Name, State: "unknown"}
		if host.LastSeenAt.Valid {
			item.LastSeenAt = host.LastSeenAt.Time.UTC().Format(time.RFC3339)
		}
		if snap, ok := snapMap[host.Name]; ok {
			item.CPUPct = snap.CpuPct
			item.RAMPct = snap.RamPct
			item.DiskPct = snap.DiskPct
			if time.Since(snap.ReceivedAt) < staleAfter {
				item.State = "up"
			} else {
				item.State = "stale"
			}
		} else if host.LastSeenAt.Valid {
			item.State = "down"
		}
		resp.Hosts = append(resp.Hosts, item)
	}
	writeJSON(w, http.StatusOK, resp)
}

// ConfigView is the wire shape for GET/PUT /api/settings/public-status.
type ConfigView struct {
	Enabled     bool   `json:"enabled"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// GET /api/settings/public-status — session-protected.
func (h *Handlers) ConfigGet(w http.ResponseWriter, r *http.Request) {
	cfg, err := LoadConfig(r.Context(), h.DB)
	if err != nil {
		h.Logger.Error("public status config get", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lookup failed"})
		return
	}
	writeJSON(w, http.StatusOK, ConfigView{Enabled: cfg.Enabled, Title: cfg.Title, Description: cfg.Description})
}

// PUT /api/settings/public-status — session-protected.
func (h *Handlers) ConfigPut(w http.ResponseWriter, r *http.Request) {
	var req ConfigView
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		req.Title = defaultTitle
	}
	req.Description = strings.TrimSpace(req.Description)
	if err := SaveConfig(r.Context(), h.DB, Config(req)); err != nil {
		h.Logger.Error("public status config save", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save failed"})
		return
	}
	h.ConfigGet(w, r)
}

// Config is the in-process shape, kept narrow on purpose: the public
// page does not need to know which hosts are visible; that's a separate
// query.
type Config struct {
	Enabled     bool
	Title       string
	Description string
}

func LoadConfig(ctx context.Context, db *sql.DB) (Config, error) {
	cfg := Config{Title: defaultTitle}
	if v, _ := settings.Get(ctx, db, SettingsKeyEnabled); v == "true" {
		cfg.Enabled = true
	}
	if v, _ := settings.Get(ctx, db, SettingsKeyTitle); v != "" {
		cfg.Title = v
	}
	if v, _ := settings.Get(ctx, db, SettingsKeyDescription); v != "" {
		cfg.Description = v
	}
	return cfg, nil
}

func SaveConfig(ctx context.Context, db *sql.DB, cfg Config) error {
	v := "false"
	if cfg.Enabled {
		v = "true"
	}
	if err := settings.Set(ctx, db, SettingsKeyEnabled, v); err != nil {
		return err
	}
	if err := settings.Set(ctx, db, SettingsKeyTitle, strings.TrimSpace(cfg.Title)); err != nil {
		return err
	}
	return settings.Set(ctx, db, SettingsKeyDescription, strings.TrimSpace(cfg.Description))
}

func snapshotsByName(list []api.HostSnapshot) map[string]api.HostSnapshot {
	m := make(map[string]api.HostSnapshot, len(list))
	for _, s := range list {
		m[s.Host] = s
	}
	return m
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
