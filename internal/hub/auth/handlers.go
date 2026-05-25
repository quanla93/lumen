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
}

func NewHandlers(db *sql.DB, secret []byte, logger *slog.Logger) *Handlers {
	return &Handlers{DB: db, Secret: secret, Logger: logger}
}

// GET /api/setup-status
func (h *Handlers) SetupStatus(w http.ResponseWriter, r *http.Request) {
	exists, err := HasAny(r.Context(), h.DB)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"admin_exists": exists})
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
