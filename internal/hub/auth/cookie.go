package auth

import (
	"net/http"
	"time"
)

// CookieName is the HTTP cookie that carries the session JWT.
const CookieName = "lumen_session"

// SetSessionCookie attaches an HttpOnly session cookie carrying `token`.
// Secure is set only when the request arrived over TLS — works in HTTP
// dev (localhost) and HTTPS prod alike without a separate code path.
func SetSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
		Expires:  time.Now().Add(SessionTTL),
		MaxAge:   int(SessionTTL.Seconds()),
	})
}

// ClearSessionCookie writes an immediately-expired cookie so the browser
// drops the session.
func ClearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}
