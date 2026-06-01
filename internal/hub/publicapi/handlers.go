package publicapi

import (
	"database/sql"
	"log/slog"
	"net/http"
	"path"

	"github.com/quanla93/lumen/internal/hub/apikey"
	"github.com/quanla93/lumen/internal/hub/hosts"
)

// Handlers groups the v1 endpoint handlers. Pr2 ships /version and
// /hosts only; metrics + alerts land in pr3.
type Handlers struct {
	DB      *sql.DB
	Logger  *slog.Logger
	Version string
}

func NewHandlers(db *sql.DB, version string, logger *slog.Logger) *Handlers {
	return &Handlers{DB: db, Logger: logger, Version: version}
}

// ─── GET /api/v1/version ─────────────────────────────────────────────────

type versionResp struct {
	Version string `json:"version"`
}

// Version is the public ping. Auth required (so revoked keys can't poll
// it), but no specific scope — any authenticated caller succeeds.
func (h *Handlers) Version(w http.ResponseWriter, r *http.Request) {
	WriteSuccess(w, r, http.StatusOK, versionResp{Version: h.Version})
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
