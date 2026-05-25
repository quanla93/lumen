package hosts

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type Handlers struct {
	DB     *sql.DB
	Logger *slog.Logger
}

func NewHandlers(db *sql.DB, logger *slog.Logger) *Handlers {
	return &Handlers{DB: db, Logger: logger}
}

type hostView struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	CreatedAt  string  `json:"created_at"`
	LastSeenAt *string `json:"last_seen_at"`
}

func toView(h Host) hostView {
	v := hostView{ID: h.ID, Name: h.Name, CreatedAt: h.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")}
	if h.LastSeenAt.Valid {
		s := h.LastSeenAt.Time.UTC().Format("2006-01-02T15:04:05Z")
		v.LastSeenAt = &s
	}
	return v
}

// GET /api/hosts
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	all, err := List(r.Context(), h.DB)
	if err != nil {
		h.Logger.Error("list hosts failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	views := make([]hostView, 0, len(all))
	for _, x := range all {
		views = append(views, toView(x))
	}
	writeJSON(w, http.StatusOK, views)
}

// POST /api/hosts {name}
//
// Returns { host: HostView, token: "lum_..." }. The plaintext token is
// shown exactly once — clients must surface it immediately to the user.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	var req struct{ Name string }
	if err := decode(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	host, token, err := Create(r.Context(), h.DB, req.Name)
	if err != nil {
		switch {
		case errors.Is(err, ErrNameRequired), errors.Is(err, ErrNameTaken):
			writeJSONError(w, http.StatusBadRequest, err.Error())
		default:
			h.Logger.Error("create host failed", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"host":  toView(host),
		"token": token,
	})
}

// DELETE /api/hosts/{id}
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := Delete(r.Context(), h.DB, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "host not found")
			return
		}
		h.Logger.Error("delete host failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/hosts/{id}/rotate
func (h *Handlers) Rotate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	token, err := Rotate(r.Context(), h.DB, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "host not found")
			return
		}
		h.Logger.Error("rotate token failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// helpers (duplicated minimally with auth.handlers — kept package-local
// so callers don't need to import a third package just for write helpers)

func decode(r *http.Request, dst any) error {
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
