package alerts

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/quanla93/lumen/internal/hub/tagutil"
)

// GET /api/tags — inventory list with usage counts.
func (h *Handlers) ListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := ListTags(r.Context(), h.DB)
	if err != nil {
		h.Logger.Error("tags: list failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

type tagCreateReq struct {
	Key         string   `json:"key"`
	Description string   `json:"description"`
	Values      []string `json:"values"`
}

// POST /api/tags — create one inventory entry. Values may be empty;
// caller is expected to add values later via POST .../values.
func (h *Handlers) CreateTag(w http.ResponseWriter, r *http.Request) {
	var req tagCreateReq
	if err := decodeStrict(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	err := CreateTag(r.Context(), h.DB, req.Key, req.Description, req.Values)
	switch {
	case err == nil:
	case errors.Is(err, ErrTagKeyExists):
		writeJSONError(w, http.StatusConflict, err.Error())
		return
	case isTagValidationErr(err):
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	default:
		h.Logger.Error("tags: create failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out, err := GetTag(r.Context(), h.DB, tagutil.NormalizeKey(req.Key))
	if err != nil {
		h.Logger.Error("tags: read-back failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

type tagUpdateReq struct {
	Description string `json:"description"`
}

// PUT /api/tags/{key} — description only. Rename of key is intentionally
// not supported in v1.
func (h *Handlers) UpdateTag(w http.ResponseWriter, r *http.Request) {
	key, ok := pathKey(r, "key")
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "invalid key")
		return
	}
	var req tagUpdateReq
	if err := decodeStrict(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := UpdateTag(r.Context(), h.DB, key, req.Description); err != nil {
		if errors.Is(err, ErrTagNotFound) {
			writeJSONError(w, http.StatusNotFound, "tag not found")
			return
		}
		h.Logger.Error("tags: update failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out, err := GetTag(r.Context(), h.DB, key)
	if err != nil {
		h.Logger.Error("tags: read-back failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// DELETE /api/tags/{key} — cascade host_tags + rule selectors. Response
// returns the impact so the UI can show "removed from N hosts, M rules".
func (h *Handlers) DeleteTag(w http.ResponseWriter, r *http.Request) {
	key, ok := pathKey(r, "key")
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "invalid key")
		return
	}
	impact, err := DeleteTag(r.Context(), h.DB, key)
	if err != nil {
		if errors.Is(err, ErrTagNotFound) {
			writeJSONError(w, http.StatusNotFound, "tag not found")
			return
		}
		h.Logger.Error("tags: delete failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, impact)
}

// GET /api/tags/{key}/impact — dry-run for the confirm dialog.
func (h *Handlers) TagImpact(w http.ResponseWriter, r *http.Request) {
	key, ok := pathKey(r, "key")
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "invalid key")
		return
	}
	impact, err := TagImpactPreview(r.Context(), h.DB, key)
	if err != nil {
		h.Logger.Error("tags: impact failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, impact)
}

type addValueReq struct {
	Value string `json:"value"`
}

// POST /api/tags/{key}/values — append one value.
func (h *Handlers) AddTagValue(w http.ResponseWriter, r *http.Request) {
	key, ok := pathKey(r, "key")
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "invalid key")
		return
	}
	var req addValueReq
	if err := decodeStrict(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	err := AddValue(r.Context(), h.DB, key, req.Value)
	switch {
	case err == nil:
	case errors.Is(err, ErrTagNotFound):
		writeJSONError(w, http.StatusNotFound, "tag not found")
		return
	case errors.Is(err, ErrTagValueExists):
		writeJSONError(w, http.StatusConflict, err.Error())
		return
	case isTagValidationErr(err):
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	default:
		h.Logger.Error("tags: add value failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out, err := GetTag(r.Context(), h.DB, key)
	if err != nil {
		h.Logger.Error("tags: read-back failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// DELETE /api/tags/{key}/values/{value} — cascade host_tags + rule
// selectors that pin this exact pair.
func (h *Handlers) DeleteTagValue(w http.ResponseWriter, r *http.Request) {
	key, ok := pathKey(r, "key")
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "invalid key")
		return
	}
	value, ok := pathKey(r, "value")
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "invalid value")
		return
	}
	impact, err := DeleteValue(r.Context(), h.DB, key, value)
	if err != nil {
		if errors.Is(err, ErrTagValueNotFound) {
			writeJSONError(w, http.StatusNotFound, "value not found")
			return
		}
		h.Logger.Error("tags: delete value failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, impact)
}

// GET /api/tags/{key}/values/{value}/impact — dry-run for confirm.
func (h *Handlers) TagValueImpact(w http.ResponseWriter, r *http.Request) {
	key, ok := pathKey(r, "key")
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "invalid key")
		return
	}
	value, ok := pathKey(r, "value")
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "invalid value")
		return
	}
	impact, err := ValueImpactPreview(r.Context(), h.DB, key, value)
	if err != nil {
		h.Logger.Error("tags: value impact failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, impact)
}

// --- helpers -----------------------------------------------------------

// pathKey reads a chi URL param and URL-decodes it. Tag values can
// contain characters like '.' and '-'; we don't expect '/' in keys or
// values (validator blocks ',' '=' but lets the rest through), but
// `=` and friends arriving as "%3D" still get unescaped here.
func pathKey(r *http.Request, name string) (string, bool) {
	raw := chi.URLParam(r, name)
	if raw == "" {
		return "", false
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return "", false
	}
	decoded = strings.TrimSpace(decoded)
	if decoded == "" {
		return "", false
	}
	return decoded, true
}

// isTagValidationErr is true for any validate sentinel from tagutil.
func isTagValidationErr(err error) bool {
	return errors.Is(err, tagutil.ErrKeyRequired) ||
		errors.Is(err, tagutil.ErrKeyTooLong) ||
		errors.Is(err, tagutil.ErrKeyInvalid) ||
		errors.Is(err, tagutil.ErrValueTooLong) ||
		errors.Is(err, tagutil.ErrValueInvalid)
}
