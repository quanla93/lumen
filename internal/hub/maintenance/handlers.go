// handlers.go — HTTP surface for the maintenance feature.
//
// Endpoints (session-required):
//   POST   /api/maintenance         — create
//   GET    /api/maintenance         — list (?state=active|upcoming|past|all)
//   PUT    /api/maintenance/{id}    — edit (start_at locked once active)
//   DELETE /api/maintenance/{id}    — cancel
//
// The cacher in maintenance.go is shared between this handler and
// the alerts engine (passed by pointer through server.go's
// constructor).

package maintenance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// Handlers is wired in server.go next to the other session-gated
// routes. Logger is optional; nil falls back to slog.Default.
type Handlers struct {
	DB       *sql.DB
	Cacher   *Cacher
	Logger   *slog.Logger
	// UserIDFn is set by the server to extract the caller user.id
	// from the session cookie — we stamp it on created_by for
	// audit. nil = don't stamp (operator rows stay NULL).
	UserIDFn func(r *http.Request) int64
}

// createReq is the wire shape for POST /api/maintenance. Times
// are RFC3339; the server interprets them as UTC (matches the
// on-disk form).
type createReq struct {
	StartAt   string            `json:"start_at"`
	EndAt     string            `json:"end_at"`
	Reason    string            `json:"reason"`
	ScopeTags map[string]string `json:"scope_tags"`
}

// Create — POST /api/maintenance
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	var in createReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	startAt, err := time.Parse(time.RFC3339, in.StartAt)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "start_at: "+err.Error())
		return
	}
	endAt, err := time.Parse(time.RFC3339, in.EndAt)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "end_at: "+err.Error())
		return
	}
	if !endAt.After(startAt) {
		writeJSONError(w, http.StatusBadRequest, "end_at must be after start_at")
		return
	}
	win := Window{
		StartAt:   startAt,
		EndAt:     endAt,
		Reason:    in.Reason,
		ScopeTags: in.ScopeTags,
	}
	if h.UserIDFn != nil {
		uid := h.UserIDFn(r)
		if uid > 0 {
			win.CreatedBy = &uid
		}
	}
	id, err := Create(r.Context(), h.DB, win)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "create: "+err.Error())
		return
	}
	// Refresh the cache so the alerts engine sees the new window
	// on the next tick (≤30 s heartbeat, or immediately if the
	// handler caller awaits the refresh below).
	if h.Cacher != nil {
		_ = h.Cacher.Refresh(r.Context())
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// List — GET /api/maintenance?state=active|upcoming|past|all
//
// "active" and "upcoming" come from the cacher (≤30 s fresh).
// "past" comes from a direct DB query because the cacher's
// Refresh prunes ended windows. "all" returns both: cached for
// future+active, DB for past.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		state = "all"
	}
	now := time.Now().UTC()

	var fromCache []Window
	if h.Cacher != nil {
		switch state {
		case "active", "upcoming", "all":
			fromCache = h.Cacher.List(state, now)
		}
	}

	// "past" + the all-union needs the DB.
	needDB := state == "past" || state == "all"
	var fromDB []Window
	if needDB {
		rows, err := h.DB.QueryContext(r.Context(),
			`SELECT id, start_at, end_at, reason, scope_tags, created_by, created_at
			 FROM maintenance_windows
			 WHERE end_at <= ?
			 ORDER BY start_at DESC
			 LIMIT 200`,
			now,
		)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "list past: "+err.Error())
			return
		}
		defer rows.Close()
		for rows.Next() {
			var win Window
			var scopeText string
			var createdBy sql.NullInt64
			if err := rows.Scan(&win.ID, &win.StartAt, &win.EndAt, &win.Reason, &scopeText, &createdBy, &win.CreatedAt); err != nil {
				writeJSONError(w, http.StatusInternalServerError, "scan: "+err.Error())
				return
			}
			win.ScopeTags = map[string]string{}
			if scopeText != "" {
				_ = json.Unmarshal([]byte(scopeText), &win.ScopeTags)
			}
			if createdBy.Valid {
				uid := createdBy.Int64
				win.CreatedBy = &uid
			}
			fromDB = append(fromDB, win)
		}
	}

	merged := append(fromCache, fromDB...)
	writeJSON(w, http.StatusOK, map[string]any{"windows": merged})
}

// Update — PUT /api/maintenance/{id}
func (h *Handlers) Update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "id: "+err.Error())
		return
	}
	var in createReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	startAt, err := time.Parse(time.RFC3339, in.StartAt)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "start_at: "+err.Error())
		return
	}
	endAt, err := time.Parse(time.RFC3339, in.EndAt)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "end_at: "+err.Error())
		return
	}
	win := Window{
		ID:        id,
		StartAt:   startAt,
		EndAt:     endAt,
		Reason:    in.Reason,
		ScopeTags: in.ScopeTags,
	}
	if err := Update(r.Context(), h.DB, win); err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			writeJSONError(w, http.StatusNotFound, "not found")
		case errors.Is(err, ErrStartAtLocked):
			writeJSONError(w, http.StatusConflict, err.Error())
		default:
			writeJSONError(w, http.StatusInternalServerError, "update: "+err.Error())
		}
		return
	}
	if h.Cacher != nil {
		_ = h.Cacher.Refresh(r.Context())
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Delete — DELETE /api/maintenance/{id}
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "id: "+err.Error())
		return
	}
	if err := Delete(r.Context(), h.DB, id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "delete: "+err.Error())
		return
	}
	if h.Cacher != nil {
		_ = h.Cacher.Refresh(r.Context())
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Loop runs the cacher's heartbeat. Called as a goroutine from
// server.go next to the retention + backup loops.
func (c *Cacher) Loop(ctx context.Context) {
	c.logger().Info("maintenance cacher starting", "heartbeat", Heartbeat)
	tick := time.NewTicker(Heartbeat)
	defer tick.Stop()

	// Eager first refresh so the alerts engine sees whatever's
	// already in the table when the hub boots.
	if err := c.Refresh(ctx); err != nil {
		c.logger().Warn("maintenance: initial refresh failed", "err", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if err := c.Refresh(ctx); err != nil {
				c.logger().Warn("maintenance: refresh failed", "err", err)
			}
		}
	}
}

func (c *Cacher) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// _ = fmt.Sprintf is here to keep go vet happy on build configs
// that might trim the import (none today, but cheap insurance).
var _ = fmt.Sprintf
var _ context.Context
