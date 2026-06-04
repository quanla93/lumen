package auth

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"github.com/quanla93/lumen/internal/hub/settings"
)

// oidcSettingsView is the wire shape for GET/PUT /api/settings/oidc.
// `HasClientSecret` lets the UI render "•••••• (saved)" without reading
// the plaintext secret back, and lets the operator know whether an empty
// submission means "leave as-is" or "really empty".
type oidcSettingsView struct {
	Enabled         bool   `json:"enabled"`
	Issuer          string `json:"issuer"`
	ClientID        string `json:"client_id"`
	ClientSecret    string `json:"client_secret,omitempty"` // write-only; never echoed back
	HasClientSecret bool   `json:"has_client_secret"`
	Scopes          string `json:"scopes"`
	ExpectedEmail   string `json:"expected_email"`
}

// GET /api/settings/oidc — session-protected.
func (h *Handlers) OIDCSettingsGet(w http.ResponseWriter, r *http.Request) {
	cfg, err := LoadOIDCConfig(r.Context(), h.DB, h.Secret, false)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "read failed")
		return
	}
	writeJSON(w, http.StatusOK, oidcSettingsView{
		Enabled:         cfg.Enabled,
		Issuer:          cfg.Issuer,
		ClientID:        cfg.ClientID,
		HasClientSecret: hasStoredClientSecret(r.Context(), h.DB),
		Scopes:          cfg.Scopes,
		ExpectedEmail:   cfg.ExpectedEmail,
	})
}

// PUT /api/settings/oidc — session-protected. Empty ClientSecret leaves
// the existing one untouched. Enabling without the required fields
// returns 400 so the UI can surface the exact gap.
func (h *Handlers) OIDCSettingsPut(w http.ResponseWriter, r *http.Request) {
	var req oidcSettingsView
	if err := decode(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Issuer = strings.TrimSpace(req.Issuer)
	req.ClientID = strings.TrimSpace(req.ClientID)
	req.ExpectedEmail = strings.TrimSpace(req.ExpectedEmail)
	req.Scopes = strings.TrimSpace(req.Scopes)
	if req.Scopes == "" {
		req.Scopes = defaultOIDCScopes
	}

	if req.Enabled {
		if req.Issuer == "" {
			writeJSONError(w, http.StatusBadRequest, "issuer is required to enable OIDC")
			return
		}
		if req.ClientID == "" {
			writeJSONError(w, http.StatusBadRequest, "client_id is required to enable OIDC")
			return
		}
		if !hasStoredClientSecret(r.Context(), h.DB) && req.ClientSecret == "" {
			writeJSONError(w, http.StatusBadRequest, "client_secret is required the first time OIDC is enabled")
			return
		}
		if req.ExpectedEmail == "" {
			writeJSONError(w, http.StatusBadRequest, "expected_email is required so OIDC can only sign the admin in")
			return
		}
	}

	if err := SaveOIDCConfig(r.Context(), h.DB, h.Secret, OIDCConfig{
		Enabled:       req.Enabled,
		Issuer:        req.Issuer,
		ClientID:      req.ClientID,
		ClientSecret:  req.ClientSecret,
		Scopes:        req.Scopes,
		ExpectedEmail: req.ExpectedEmail,
	}); err != nil {
		h.Logger.Error("save oidc config failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "save failed")
		return
	}
	if h.OIDC != nil {
		h.OIDC.Invalidate()
	}
	h.OIDCSettingsGet(w, r)
}

// POST /api/settings/oidc/test — fetches the issuer's discovery document
// so the operator sees "OK" (or the underlying error) before saving and
// trying to log in. Uses the in-flight Issuer value from the request, not
// whatever is saved, so the test reflects what the operator just typed.
func (h *Handlers) OIDCTestDiscovery(w http.ResponseWriter, r *http.Request) {
	if h.OIDC == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "OIDC handler not wired")
		return
	}
	var req struct {
		Issuer string `json:"issuer"`
	}
	if err := decode(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Issuer = strings.TrimSpace(req.Issuer)
	if req.Issuer == "" {
		writeJSONError(w, http.StatusBadRequest, "issuer is required")
		return
	}
	if err := h.OIDC.TestDiscovery(r.Context(), req.Issuer); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func hasStoredClientSecret(ctx context.Context, db *sql.DB) bool {
	v, _ := settings.Get(ctx, db, OIDCKeyClientSecretEnc)
	return v != ""
}
