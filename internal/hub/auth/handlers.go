package auth

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
)

const (
	minUsernameLen = 3
	maxUsernameLen = 32
	minPasswordLen = 8
)

type Handlers struct {
	DB     *sql.DB
	Secret []byte
	Logger *slog.Logger
	OIDC   *OIDCFlow // nil when OIDC compiled-out / not wired; SetupStatus exposes enabled=false
	SAML   *SAMLFlow // nil when SAML compiled-out / not wired; SetupStatus exposes enabled=false

	// HubPublicURL is the externally-reachable URL the SAML SP uses
	// to derive its entity ID, metadata URL, and ACS URL. Set via
	// LUMEN_HUB_PUBLIC_URL. When empty the SAML flow returns 503
	// because the SP can't mint a valid AuthnRequest without a
	// known public URL.
	HubPublicURL func() string
}

func NewHandlers(db *sql.DB, secret []byte, logger *slog.Logger) *Handlers {
	return &Handlers{DB: db, Secret: secret, Logger: logger}
}

// GET /api/setup-status — also reports whether OIDC is configured so the
// login screen knows to show the "Sign in with SSO" button before any
// session exists.
func (h *Handlers) SetupStatus(w http.ResponseWriter, r *http.Request) {
	exists, err := HasAny(r.Context(), h.DB)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	oidcEnabled := false
	if h.OIDC != nil {
		cfg, _ := LoadOIDCConfig(r.Context(), h.DB, h.Secret, false)
		oidcEnabled = cfg.Enabled && cfg.Issuer != "" && cfg.ClientID != "" && cfg.ExpectedEmail != ""
	}
	samlEnabled := false
	if h.SAML != nil {
		cfg, _ := LoadSAMLConfig(r.Context(), h.DB, h.Secret)
		samlEnabled = cfg.Enabled && cfg.IdPMetadataXML != "" && len(cfg.ExpectedNameIDList) > 0 && cfg.SPPrivateKeyPEM != ""
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"admin_exists": exists,
		"oidc_enabled": oidcEnabled,
		"saml_enabled": samlEnabled,
	})
}

// GET /api/auth/oidc/login — redirects to the IdP's authorization URL,
// setting an encrypted state cookie the browser will round-trip back to
// /api/auth/oidc/callback.
func (h *Handlers) LoginOIDC(w http.ResponseWriter, r *http.Request) {
	if h.OIDC == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "OIDC not configured")
		return
	}
	url, err := h.OIDC.LoginRedirect(r.Context(), w, r)
	if err != nil {
		h.Logger.Warn("oidc login redirect failed", "err", err)
		http.Redirect(w, r, "/login?sso_error="+errToQuery(err), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}

// GET /api/auth/oidc/callback — the IdP redirects the browser here with
// ?code=...&state=.... We verify the code+state+ID-token, then mint the
// same session cookie that password login uses and redirect to the app.
func (h *Handlers) CallbackOIDC(w http.ResponseWriter, r *http.Request) {
	if h.OIDC == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "OIDC not configured")
		return
	}
	if errQ := r.URL.Query().Get("error"); errQ != "" {
		desc := r.URL.Query().Get("error_description")
		h.Logger.Info("oidc callback: provider returned error", "error", errQ, "desc", desc)
		http.Redirect(w, r, "/login?sso_error="+errToQuery(errors.New(errQ+": "+desc)), http.StatusSeeOther)
		return
	}
	_, err := h.OIDC.HandleCallback(r.Context(), w, r)
	if err != nil {
		h.Logger.Warn("oidc callback failed", "err", err)
		http.Redirect(w, r, "/login?sso_error="+errToQuery(err), http.StatusSeeOther)
		return
	}
	u, err := GetSingleAdmin(r.Context(), h.DB)
	if err != nil {
		h.Logger.Error("oidc callback: no local admin to bind session to", "err", err)
		http.Redirect(w, r, "/login?sso_error=no-admin", http.StatusSeeOther)
		return
	}
	if err := h.issueAndSetCookie(w, r, u.ID); err != nil {
		h.Logger.Error("oidc callback: issue session failed", "err", err)
		http.Redirect(w, r, "/login?sso_error=session", http.StatusSeeOther)
		return
	}
	h.Logger.Info("oidc login ok", "uid", u.ID, "user", u.Username)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func errToQuery(err error) string {
	// 1-line, URL-safe enough — frontend treats it as a hint only and renders
	// its own message. Long IdP errors get truncated.
	s := err.Error()
	if len(s) > 200 {
		s = s[:200]
	}
	return strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "&", "%26")
}

// POST /api/register — first-admin only. 403 once any user exists.
func (h *Handlers) Register(w http.ResponseWriter, r *http.Request) {
	exists, err := HasAny(r.Context(), h.DB)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if exists {
		writeJSONError(w, http.StatusForbidden, "setup already complete")
		return
	}

	var req struct{ Username, Password string }
	if err := decode(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if err := validateUsername(req.Username); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validatePassword(req.Password); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		h.Logger.Error("password hash failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	u, err := CreateUser(r.Context(), h.DB, req.Username, hash)
	if err != nil {
		h.Logger.Error("create user failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := h.issueAndSetCookie(w, r, u.ID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "session issue failed")
		return
	}
	writeJSON(w, http.StatusCreated, userView(u))
}

// POST /api/login
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req struct{ Username, Password string }
	if err := decode(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Username = strings.TrimSpace(req.Username)

	u, hash, err := GetUserByUsername(r.Context(), h.DB, req.Username)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			writeJSONError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		h.Logger.Error("user lookup failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !VerifyPassword(req.Password, hash) {
		writeJSONError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err := h.issueAndSetCookie(w, r, u.ID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "session issue failed")
		return
	}
	writeJSON(w, http.StatusOK, userView(u))
}

// POST /api/logout
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	ClearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/account/password — change own password.
//
// Body: { current, new }. Returns 204 on success; 401 if `current`
// doesn't match (constant-time, so brute-forcing `current` doesn't leak
// anything beyond what login already does); 400 on length validation.
// The existing session cookie stays valid — the user doesn't have to
// log back in, since the session token isn't password-derived.
func (h *Handlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	uid := UserIDFrom(r.Context())
	if uid == 0 {
		writeJSONError(w, http.StatusUnauthorized, "session required")
		return
	}
	var req struct{ Current, New string }
	if err := decode(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validatePassword(req.New); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Current == req.New {
		writeJSONError(w, http.StatusBadRequest, "new password must differ from current")
		return
	}

	u, oldHash, err := getUserByIDWithHash(r.Context(), h.DB, uid)
	if err != nil {
		h.Logger.Error("user lookup failed", "err", err, "uid", uid)
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if !VerifyPassword(req.Current, oldHash) {
		writeJSONError(w, http.StatusUnauthorized, "current password is wrong")
		return
	}
	newHash, err := HashPassword(req.New)
	if err != nil {
		h.Logger.Error("password hash failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := UpdatePasswordHash(r.Context(), h.DB, u.ID, newHash); err != nil {
		h.Logger.Error("update password failed", "err", err, "uid", u.ID)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.Logger.Info("password changed", "uid", u.ID, "user", u.Username)
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/me — protected; returns 401 if no valid session.
func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	uid := UserIDFrom(r.Context())
	if uid == 0 {
		writeJSONError(w, http.StatusUnauthorized, "session required")
		return
	}
	u, err := GetUserByID(r.Context(), h.DB, uid)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, userView(u))
}

func (h *Handlers) issueAndSetCookie(w http.ResponseWriter, r *http.Request, uid int64) error {
	tok, err := IssueToken(uid, h.Secret)
	if err != nil {
		return err
	}
	SetSessionCookie(w, r, tok)
	return nil
}

// helpers ---------------------------------------------------------------

func userView(u User) map[string]any {
	return map[string]any{
		"id":         u.ID,
		"username":   u.Username,
		"created_at": u.CreatedAt,
	}
}

func decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func validateUsername(u string) error {
	if len(u) < minUsernameLen || len(u) > maxUsernameLen {
		return errors.New("username must be 3–32 chars")
	}
	for _, r := range u {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.') {
			return errors.New("username may only contain letters, digits, '_', '-', '.'")
		}
	}
	return nil
}

func validatePassword(p string) error {
	if len(p) < minPasswordLen {
		return errors.New("password must be at least 8 characters")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
