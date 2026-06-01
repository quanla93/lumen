package userprefs

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/quanla93/lumen/internal/hub/auth"
)

type Handlers struct {
	DB     *sql.DB
	Logger *slog.Logger
}

func NewHandlers(db *sql.DB, logger *slog.Logger) *Handlers {
	return &Handlers{DB: db, Logger: logger}
}

// getResponse is GET /api/me/prefs. Returns either the stored blob or
// null per key so the client can fall back to defaults. We don't
// pre-fill server-side defaults here — defaults belong client-side so
// they're versionable per-frontend without a backend roll.
type getResponse struct {
	Dashboard json.RawMessage `json:"dashboard"`
	Display   json.RawMessage `json:"display"`
}

// Get GET /api/me/prefs
func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserIDFrom(r.Context())
	if uid == 0 {
		writeJSONError(w, http.StatusUnauthorized, "session required")
		return
	}
	out := getResponse{
		Dashboard: json.RawMessage("null"),
		Display:   json.RawMessage("null"),
	}
	if v, err := Get(r.Context(), h.DB, uid, KeyDashboard); err == nil {
		out.Dashboard = json.RawMessage(v)
	} else if !errors.Is(err, ErrNotFound) {
		h.Logger.Error("userprefs get dashboard", "err", err, "user_id", uid)
		writeJSONError(w, http.StatusInternalServerError, "read failed")
		return
	}
	if v, err := Get(r.Context(), h.DB, uid, KeyDisplay); err == nil {
		out.Display = json.RawMessage(v)
	} else if !errors.Is(err, ErrNotFound) {
		h.Logger.Error("userprefs get display", "err", err, "user_id", uid)
		writeJSONError(w, http.StatusInternalServerError, "read failed")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// PutDashboard PUT /api/me/prefs/dashboard
func (h *Handlers) PutDashboard(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserIDFrom(r.Context())
	if uid == 0 {
		writeJSONError(w, http.StatusUnauthorized, "session required")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxJSONBytes+1))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "read body failed")
		return
	}
	if len(body) > maxJSONBytes {
		writeJSONError(w, http.StatusRequestEntityTooLarge, ErrTooLarge.Error())
		return
	}
	parsed, err := ValidateDashboard(string(body))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Re-encode canonical form so subsequent GETs return stable bytes.
	canon, _ := json.Marshal(parsed)
	if err := Set(r.Context(), h.DB, uid, KeyDashboard, string(canon)); err != nil {
		h.Logger.Error("userprefs set dashboard", "err", err, "user_id", uid)
		writeJSONError(w, http.StatusInternalServerError, "write failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PutDisplay PUT /api/me/prefs/display
func (h *Handlers) PutDisplay(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserIDFrom(r.Context())
	if uid == 0 {
		writeJSONError(w, http.StatusUnauthorized, "session required")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxJSONBytes+1))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "read body failed")
		return
	}
	if len(body) > maxJSONBytes {
		writeJSONError(w, http.StatusRequestEntityTooLarge, ErrTooLarge.Error())
		return
	}
	parsed, err := ValidateDisplay(string(body))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	canon, _ := json.Marshal(parsed)
	if err := Set(r.Context(), h.DB, uid, KeyDisplay, string(canon)); err != nil {
		h.Logger.Error("userprefs set display", "err", err, "user_id", uid)
		writeJSONError(w, http.StatusInternalServerError, "write failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
