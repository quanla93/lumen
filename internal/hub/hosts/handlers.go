package hosts

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/quanla93/lumen/internal/hub/store"
	"github.com/quanla93/lumen/internal/hub/tagutil"
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
	// SilencedUntil is RFC3339 in UTC when set; null = not silenced.
	// FE compares with current time to render the silence chip — a past
	// value (lazy-expired silence) is still surfaced as null since the
	// alert engine treats it as expired too.
	SilencedUntil *string `json:"silenced_until,omitempty"`
	// PublicVisible is true when the operator opted this host into the
	// unauthenticated /status page. Default false on new hosts.
	PublicVisible bool `json:"public_visible"`
}

func toView(h Host, tags []Tag) hostView {
	v := hostView{
		ID:            h.ID,
		Name:          h.Name,
		CreatedAt:     h.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		Tags:          map[string]string{},
		PublicVisible: h.PublicVisible,
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
	if h.SilencedUntil.Valid {
		t := time.Unix(h.SilencedUntil.Int64, 0).UTC()
		if t.After(time.Now()) {
			s := t.Format("2006-01-02T15:04:05Z")
			v.SilencedUntil = &s
		}
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
	if h.SystemVirtType.Valid {
		meta.VirtType = h.SystemVirtType.String
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
		case errors.Is(err, tagutil.ErrKeyRequired),
			errors.Is(err, tagutil.ErrKeyInvalid),
			errors.Is(err, tagutil.ErrValueInvalid),
			errors.Is(err, tagutil.ErrKeyTooLong),
			errors.Is(err, tagutil.ErrValueTooLong),
			errors.Is(err, ErrTooManyTags),
			errors.Is(err, ErrTagNotInInventory):
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

// POST /api/hosts/{id}/silence
//
// Body: {"seconds": 3600}. Sets silenced_until = now + seconds; the
// alert engine then skips firing/resolved events + notifications for
// this host until the timestamp passes. Use to suppress alerts during
// planned maintenance (e.g. before running `docker compose pull && up`
// on the agent's machine — without this the offline rule fires the
// moment the container restarts).
//
// Pass seconds=0 (or call DELETE) to clear the silence immediately.
// Maximum window is 1 year — long enough to act as "until I lift it"
// for homelab maintenance, short enough that an abandoned silence
// auto-expires before drifting into a "why didn't we get paged?"
// post-mortem.
func (h *Handlers) Silence(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Seconds int64 `json:"seconds"`
	}
	if err := decode(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Seconds < 0 {
		writeJSONError(w, http.StatusBadRequest, "seconds must be >= 0")
		return
	}
	const maxSilenceSeconds = int64(365 * 24 * 60 * 60)
	if req.Seconds > maxSilenceSeconds {
		writeJSONError(w, http.StatusBadRequest, "silence window too long (max 1 year)")
		return
	}
	var until time.Time
	if req.Seconds > 0 {
		until = time.Now().Add(time.Duration(req.Seconds) * time.Second)
	}
	if err := SetSilence(r.Context(), h.DB, id, until); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "host not found")
			return
		}
		h.Logger.Error("set silence failed", "err", err, "host_id", id)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	resp := map[string]any{}
	if !until.IsZero() {
		resp["silenced_until"] = until.UTC().Format("2006-01-02T15:04:05Z")
	} else {
		resp["silenced_until"] = nil
	}
	writeJSON(w, http.StatusOK, resp)
}

// DELETE /api/hosts/{id}/silence — clears any active silence.
func (h *Handlers) Unsilence(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := SetSilence(r.Context(), h.DB, id, time.Time{}); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "host not found")
			return
		}
		h.Logger.Error("clear silence failed", "err", err, "host_id", id)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Share tokens (RFC 0004) ---

// POST /api/hosts/{id}/share — mints a new share token. Returns the
// plaintext token + expires_at. Token is shown ONCE.
func (h *Handlers) MintShare(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		TTLSeconds int64  `json:"ttl_seconds"`
		Label      string `json:"label"`
	}
	if err := decode(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	if ttl < MinShareTTL || ttl > MaxShareTTL {
		writeJSONError(w, http.StatusBadRequest, ErrShareTTLBounds.Error())
		return
	}
	share, err := MintShare(r.Context(), h.DB, id, ttl, req.Label, 0)
	if err != nil {
		if errors.Is(err, ErrShareTTLBounds) {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "host not found")
			return
		}
		h.Logger.Error("mint share failed", "err", err, "host_id", id)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, share)
}

// GET /api/hosts/{id}/shares — lists active shares for the host
// (metadata only, no plaintext token).
func (h *Handlers) ListShares(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	shares, err := ListHostShares(r.Context(), h.DB, id)
	if err != nil {
		h.Logger.Error("list shares failed", "err", err, "host_id", id)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, shares)
}

// DELETE /api/hosts/{id}/share/{token} — revokes a share token.
// Filtered by (host_id, token) so the operator can only revoke
// tokens that belong to the host named in the URL.
func (h *Handlers) RevokeShare(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	token := chi.URLParam(r, "token")
	if token == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid token")
		return
	}
	res, err := h.DB.ExecContext(r.Context(),
		`DELETE FROM host_share_tokens WHERE host_id = ? AND token = ?`, id, token)
	if err != nil {
		h.Logger.Error("revoke share failed", "err", err, "host_id", id)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSONError(w, http.StatusNotFound, "share not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/public/host/{token} — unauthenticated public view of a
// shared host. Returns a slim payload (id + name + expires_at) —
// no system metadata, no tags, no metrics (per RFC 0004 §"Risks").
func (h *Handlers) PublicHostByToken(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	payload, err := FetchByShareToken(r.Context(), h.DB, token)
	if err != nil {
		if errors.Is(err, ErrShareNotFound) || errors.Is(err, ErrShareInvalid) {
			writeJSONError(w, http.StatusNotFound, "not found")
			return
		}
		if errors.Is(err, ErrShareExpired) {
			writeJSONError(w, http.StatusNotFound, "link expired")
			return
		}
		h.Logger.Error("public share fetch failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

// PUT /api/hosts/{id}/public-visible — toggles whether the host appears
// on the unauthenticated /status page. Body: {"public_visible": true|false}.
func (h *Handlers) SetPublicVisibility(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		PublicVisible bool `json:"public_visible"`
	}
	if err := decode(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := SetPublicVisible(r.Context(), h.DB, id, req.PublicVisible); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "host not found")
			return
		}
		h.Logger.Error("set public_visible failed", "err", err, "host_id", id)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
