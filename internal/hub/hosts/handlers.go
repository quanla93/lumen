package hosts

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/quanla93/lumen/internal/hub/store"
	"github.com/quanla93/lumen/internal/shared/api"
)

type Handlers struct {
	DB     *sql.DB
	Store  *store.Store
	Logger *slog.Logger
}

func NewHandlers(db *sql.DB, st *store.Store, logger *slog.Logger) *Handlers {
	return &Handlers{DB: db, Store: st, Logger: logger}
}

type hostView struct {
	ID                int64               `json:"id"`
	Name              string              `json:"name"`
	CreatedAt         string              `json:"created_at"`
	LastSeenAt        *string             `json:"last_seen_at"`
	System            *api.SystemMetadata `json:"system,omitempty"`
	MetadataUpdatedAt *string             `json:"metadata_updated_at,omitempty"`
}

func toView(h Host) hostView {
	v := hostView{ID: h.ID, Name: h.Name, CreatedAt: h.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")}
	if h.LastSeenAt.Valid {
		s := h.LastSeenAt.Time.UTC().Format("2006-01-02T15:04:05Z")
		v.LastSeenAt = &s
	}
	if meta, ok := systemMetadata(h); ok {
		v.System = &meta
	}
	if h.MetadataUpdatedAt.Valid {
		s := h.MetadataUpdatedAt.Time.UTC().Format("2006-01-02T15:04:05Z")
		v.MetadataUpdatedAt = &s
	}
	return v
}

func systemMetadata(h Host) (api.SystemMetadata, bool) {
	meta := api.SystemMetadata{}
	if h.SystemOS.Valid {
		meta.OS = h.SystemOS.String
	}
	if h.SystemHostname.Valid {
		meta.Hostname = h.SystemHostname.String
	}
	if h.SystemPrimaryIP.Valid {
		meta.PrimaryIP = h.SystemPrimaryIP.String
	}
	if h.SystemKernel.Valid {
		meta.Kernel = h.SystemKernel.String
	}
	if h.SystemArch.Valid {
		meta.Arch = h.SystemArch.String
	}
	if h.SystemCPUModel.Valid {
		meta.CPUModel = h.SystemCPUModel.String
	}
	if h.SystemUptimeSeconds.Valid && h.SystemUptimeSeconds.Int64 > 0 {
		meta.UptimeSeconds = uint64(h.SystemUptimeSeconds.Int64)
	}
	if h.AgentVersion.Valid {
		meta.AgentVersion = h.AgentVersion.String
	}
	return meta, meta != (api.SystemMetadata{})
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
	// Resolve name BEFORE deleting so we can evict the in-memory snapshot
	// (which is keyed by host name, not id) after the DB row is gone.
	host, err := getByID(r.Context(), h.DB, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "host not found")
			return
		}
		h.Logger.Error("lookup host failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
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
	if h.Store != nil {
		h.Store.Delete(host.Name)
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
