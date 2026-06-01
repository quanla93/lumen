package apikey

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Handlers binds the admin-facing CRUD endpoints. All routes are
// session-protected at the router layer.
type Handlers struct {
	DB     *sql.DB
	Logger *slog.Logger
}

func NewHandlers(db *sql.DB, logger *slog.Logger) *Handlers {
	return &Handlers{DB: db, Logger: logger}
}

// JSON wire shapes — kept terse to match the rest of the internal API.
// The public /api/v1/* envelope (success/data/error/request_id) lands
// in pr2 and only applies to that surface.

type listItem struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Preview    string   `json:"preview"`
	Scopes     []string `json:"scopes"`
	HostFilter *string  `json:"host_filter"`
	LastUsedAt *string  `json:"last_used_at"` // RFC3339 or null
	CreatedAt  string   `json:"created_at"`
}

type createRequest struct {
	Name       string   `json:"name"`
	Scopes     []string `json:"scopes"`
	HostFilter *string  `json:"host_filter"`
}

type createResponse struct {
	listItem
	Plaintext string `json:"plaintext"`
}

// GET /api/apikeys
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	keys, err := List(r.Context(), h.DB)
	if err != nil {
		h.Logger.Error("api_keys list", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "list failed")
		return
	}
	items := make([]listItem, 0, len(keys))
	for _, k := range keys {
		items = append(items, toListItem(k))
	}
	writeJSON(w, http.StatusOK, items)
}

// POST /api/apikeys
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.HostFilter != nil {
		trimmed := strings.TrimSpace(*req.HostFilter)
		if trimmed == "" {
			req.HostFilter = nil
		} else {
			req.HostFilter = &trimmed
		}
	}

	created, err := Create(r.Context(), h.DB, req.Name, req.Scopes, req.HostFilter)
	if err != nil {
		switch {
		case errors.Is(err, ErrNameRequired),
			errors.Is(err, ErrNameTooLong),
			errors.Is(err, ErrScopesRequired),
			errors.Is(err, ErrScopeUnknown),
			errors.Is(err, ErrFilterTooLong):
			writeJSONError(w, http.StatusBadRequest, err.Error())
		default:
			h.Logger.Error("api_keys create", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "create failed")
		}
		return
	}

	resp := createResponse{
		listItem:  toListItem(created.Key),
		Plaintext: created.Plaintext,
	}
	writeJSON(w, http.StatusCreated, resp)
}

// DELETE /api/apikeys/{id}
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "missing id")
		return
	}
	if err := Delete(r.Context(), h.DB, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "not found")
			return
		}
		h.Logger.Error("api_keys delete", "err", err, "id", id)
		writeJSONError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toListItem(k Key) listItem {
	item := listItem{
		ID:         k.ID,
		Name:       k.Name,
		Preview:    k.Preview,
		Scopes:     k.Scopes,
		HostFilter: k.HostFilter,
		CreatedAt:  k.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if k.LastUsedAt != nil {
		s := k.LastUsedAt.Format("2006-01-02T15:04:05Z")
		item.LastUsedAt = &s
	}
	return item
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
