package publicapi

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/quanla93/lumen/internal/hub/apikey"
)

// ctxKey is unexported so other packages can't accidentally collide
// with our context.WithValue key.
type ctxKey int

const (
	keyAPIKey ctxKey = iota + 1
)

// KeyFromContext returns the verified API key attached by Authn, or nil
// if the request didn't go through the Authn middleware. Handlers can
// rely on it being non-nil because we always wire Authn first.
func KeyFromContext(ctx context.Context) *apikey.Key {
	v, _ := ctx.Value(keyAPIKey).(*apikey.Key)
	return v
}

// Authn extracts a Bearer token from Authorization, verifies it against
// the api_keys table, and attaches the verified Key to the request
// context. On any failure mode it short-circuits with a 401 envelope.
func Authn(db *sql.DB, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			plain, ok := bearerToken(r)
			if !ok {
				WriteError(w, r, http.StatusUnauthorized, CodeMissingAuth,
					"missing Bearer token in Authorization header")
				return
			}
			if !strings.HasPrefix(plain, apikey.TokenPrefix) {
				WriteError(w, r, http.StatusUnauthorized, CodeInvalidAuth,
					"token does not look like a Lumen API key")
				return
			}
			key, err := apikey.VerifyAndTouch(r.Context(), db, plain)
			if err != nil {
				if errors.Is(err, apikey.ErrNotFound) {
					WriteError(w, r, http.StatusUnauthorized, CodeInvalidAuth,
						"unknown or revoked key")
					return
				}
				logger.Error("apikey verify", "err", err)
				WriteError(w, r, http.StatusInternalServerError, CodeInternalError,
					"verify failed")
				return
			}
			ctx := context.WithValue(r.Context(), keyAPIKey, key)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireScope wraps a handler and rejects requests whose verified key
// is missing the named scope. Pair after Authn.
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := KeyFromContext(r.Context())
			if key == nil || !key.HasScope(scope) {
				WriteError(w, r, http.StatusForbidden, CodeInsufficient,
					"key is missing required scope: "+scope)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	tok := strings.TrimSpace(h[len(prefix):])
	if tok == "" {
		return "", false
	}
	return tok, true
}

// ─── Rate limit ──────────────────────────────────────────────────────────

// Limiter is a per-key in-memory token bucket. Single-binary discipline
// — no Redis. Tokens are floats so we can refill at sub-token rates
// over fractional seconds without snapping to integer ticks.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	burst   float64
	refill  float64 // tokens per second
}

type bucket struct {
	tokens     float64
	lastRefill time.Time
}

// NewLimiter constructs a limiter with the given burst capacity and
// refill rate (per minute). 100/min default in v0.5.0 — that's enough
// for ~20 Grafana panels on a 15s scrape interval.
func NewLimiter(burst int, perMinute int) *Limiter {
	return &Limiter{
		buckets: map[string]*bucket{},
		burst:   float64(burst),
		refill:  float64(perMinute) / 60.0,
	}
}

// Allow either lets the request through (consuming one token) or
// returns the wall-clock duration the caller should wait before
// retrying.
func (l *Limiter) Allow(keyID string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok2 := l.buckets[keyID]
	if !ok2 {
		b = &bucket{tokens: l.burst, lastRefill: now}
		l.buckets[keyID] = b
	} else {
		elapsed := now.Sub(b.lastRefill).Seconds()
		b.tokens += elapsed * l.refill
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
		b.lastRefill = now
	}

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / l.refill * float64(time.Second))
	return false, wait
}

// Forget removes a key's bucket. Wire into apikey delete so revoked
// keys don't leave bucket state lying around. Optional — bounded by
// the small number of keys an operator creates.
func (l *Limiter) Forget(keyID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, keyID)
}

// Middleware enforces the limit. Must be wired AFTER Authn so the
// key context is set; without that we have nothing to bucket by.
// Sets X-RateLimit-* response headers on every request so integrators
// can see how much budget they have without throwing a probe.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := KeyFromContext(r.Context())
		if key == nil {
			// Defensive — should not happen since Authn runs first.
			WriteError(w, r, http.StatusUnauthorized, CodeMissingAuth, "no key in context")
			return
		}
		ok, retry := l.Allow(key.ID)
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(int(l.burst)))
		remaining := l.remaining(key.ID)
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		if !ok {
			retrySec := int(retry.Seconds())
			if retrySec < 1 {
				retrySec = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retrySec))
			WriteError(w, r, http.StatusTooManyRequests, CodeRateLimit,
				"rate limit exceeded; retry in "+strconv.Itoa(retrySec)+"s")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// remaining is a non-mutating read of how many tokens the bucket holds
// right now. Used only for the X-RateLimit-Remaining response header,
// so a slightly-stale read is fine; we still hold the same mutex.
func (l *Limiter) remaining(keyID string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[keyID]
	if !ok {
		return int(l.burst)
	}
	return int(b.tokens)
}
