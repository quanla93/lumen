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
	Tags              map[string]string   `json:"tags"`
}

func toView(h Host, tags []Tag) hostView {
	v := hostView{
		ID:        h.ID,
		Name:      h.Name,
		CreatedAt: h.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		Tags:      map[string]string{},
	}
	for _, t := range tags {
		v.Tags[t.Key] = t.Value
	}
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
		tags, terr := ListTags(r.Context(), h.DB, x.ID)
		if terr != nil {
			h.Logger.Error("list host tags failed", "err", terr, "host_id", x.ID)
			writeJSONError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		views = append(views, toView(x, tags))
	}
	writeJSON(w, http.StatusOK, views)
}

// GET /api/host-tags
//
// Returns every distinct (key, value) tag currently in use across the
// fleet, with the host count. Used by the alerts rule form to render a
// clickable tag picker — operators see what tags actually exist instead
// of typing key=value from memory.
func (h *Handlers) ListTagFacets(w http.ResponseWriter, r *http.Request) {
	facets, err := ListTagFacets(r.Context(), h.DB)
	if err != nil {
		h.Logger.Error("list tag facets failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	out := make([]map[string]any, 0, len(facets))
	for _, f := range facets {
		out = append(out, map[string]any{
			"key":        f.Key,
			"value":      f.Value,
			"host_count": f.HostCount,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// PUT /api/hosts/{id}/tags
//
// Body: {"tags": {"tier":"critical", "env":"prod"}}. Replaces the host's
// tag set wholesale (no patch semantics). Empty map clears all tags.
func (h *Handlers) SetTags(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Tags map[string]string `json:"tags"`
	}
	if err := decode(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	tags := make([]Tag, 0, len(req.Tags))
	for k, v := range req.Tags {
		tags = append(tags, Tag{Key: k, Value: v})
	}
	out, err := SetTags(r.Context(), h.DB, id, tags)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			writeJSONError(w, http.StatusNotFound, "host not found")
		case errors.Is(err, ErrTagKeyRequired),
			errors.Is(err, ErrTagKeyInvalid),
			errors.Is(err, ErrTagValueInvalid),
			errors.Is(err, ErrTagKeyTooLong),
			errors.Is(err, ErrTagValueTooLong),
			errors.Is(err, ErrTooManyTags):
			writeJSONError(w, http.StatusBadRequest, err.Error())
		default:
			h.Logger.Error("set host tags failed", "err", err, "host_id", id)
			writeJSONError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	m := map[string]string{}
	for _, t := range out {
		m[t.Key] = t.Value
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": m})
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
		"host":  toView(host, nil),
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
