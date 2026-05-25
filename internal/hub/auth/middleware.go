package auth

import (
	"context"
	"net/http"
)

type ctxKey int

const userIDKey ctxKey = 1

// WithUserID returns a context carrying userID. Tests and handlers can use
// UserIDFrom to read it back. Only RequireSession sets this in production.
func WithUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

// UserIDFrom returns the authenticated user id, or 0 if absent.
func UserIDFrom(ctx context.Context) int64 {
	v, _ := ctx.Value(userIDKey).(int64)
	return v
}

// RequireSession is middleware that 401s requests without a valid session
// cookie, and otherwise injects the userID into the request context.
func RequireSession(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(CookieName)
			if err != nil || c.Value == "" {
				writeJSONError(w, http.StatusUnauthorized, "session required")
				return
			}
			uid, err := ParseToken(c.Value, secret)
			if err != nil {
				writeJSONError(w, http.StatusUnauthorized, "invalid session")
				return
			}
			next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), uid)))
		})
	}
}
