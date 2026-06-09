// share.go — RFC 0004 §"Per-host share link" implementation.
//
// The share-link feature mints a 32-byte random token bound to one
// host with a TTL. The public route at /h/{token} (wired in
// server.go) renders a read-only host detail. This file owns the DB
// layer; the HTTP handlers live in handlers.go (the
// /api/hosts/{id}/share + /api/hosts/{id}/shares + /api/share/{token}
// surface) and a new public handler in package publicstatus-style
// (GET /api/public/host/{token}, unauthenticated).
//
// Security model: the plaintext token is the bearer secret. It
// never round-trips through logs or responses after the operator
// first sees it (List returns metadata only). Revocation is
// immediate (the row is deleted; we don't soft-delete so a leaked
// URL stops working the instant the admin revokes). Sweep runs
// hourly to garbage-collect expired rows.

package hosts

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

const (
	// shareTokenBytes is the entropy per minted token. 32 bytes =
	// 256 bits, identical to the host ingest token. 32 random bytes
	// base64url-encoded (no padding) = 43 chars.
	shareTokenBytes = 32
	// MinShareTTL / MaxShareTTL clamp the operator's TTL input so
	// a typo can't mint a 1-second or 100-year share.
	MinShareTTL = 1 * time.Hour
	MaxShareTTL = 30 * 24 * time.Hour
)

// Sentinel errors. ErrShareNotFound is the catch-all "we won't say
// which case you hit" return — both unknown-token and revoked-token
// return this so the operator can't tell the difference from the
// 404 response. ErrShareExpired is separate because the handler
// maps it to a different 404 message ("link expired, ask the admin
// for a new one") that's more helpful than "not found".
var (
	ErrShareNotFound  = errors.New("host share token not found")
	ErrShareExpired   = errors.New("host share token expired")
	ErrShareRevoked   = errors.New("host share token revoked")
	ErrShareInvalid   = errors.New("invalid share token")
	ErrShareTTLBounds = errors.New("share ttl out of bounds (1h..720h)")
)

// Share is the row the operator sees in the "active shares" list.
// The plaintext Token is set ONLY at mint time and is never
// returned by List / Fetch — keeping it out of the read path means
// a leaked list response can't itself leak the bearer.
type Share struct {
	Token     string    `json:"-"` // populated at mint, omitted from JSON
	ID        int64     `json:"id"`
	HostID    int64     `json:"host_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
}

// ShareView is what List returns to the operator — metadata only,
// no plaintext token. Same as Share with Token stripped + HostName
// added so the UI can render "share for db-prod" without an extra
// join.
type ShareView struct {
	ID        int64     `json:"id"`
	HostID    int64     `json:"host_id"`
	HostName  string    `json:"host_name"`
	ExpiresAt time.Time `json:"expires_at"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"created_at"`
}

// MintShare creates a new share row. Returns the populated Share
// (Token is plaintext — show it to the operator exactly once) +
// the new id. ttl must be within MinShareTTL..MaxShareTTL; label
// is optional ("" ok). createdBy is the user id (0 = operator
// row, stays NULL in the DB).
func MintShare(ctx context.Context, db *sql.DB, hostID int64, ttl time.Duration, label string, createdBy int64) (Share, error) {
	if ttl < MinShareTTL || ttl > MaxShareTTL {
		return Share{}, ErrShareTTLBounds
	}
	// Validate the host exists. Use getByID's ErrNotFound path so
	// the handler can map 404 cleanly.
	if _, err := getByID(ctx, db, hostID); err != nil {
		return Share{}, err
	}
	tok, err := newShareToken()
	if err != nil {
		return Share{}, err
	}
	now := time.Now().UTC()
	expires := now.Add(ttl)
	var createdByNull sql.NullInt64
	if createdBy > 0 {
		createdByNull = sql.NullInt64{Int64: createdBy, Valid: true}
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO host_share_tokens (token, host_id, expires_at, label, created_by)
		VALUES (?, ?, ?, ?, ?)`,
		tok, hostID, expires, label, createdByNull,
	)
	if err != nil {
		return Share{}, fmt.Errorf("share: insert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Share{}, err
	}
	return Share{
		Token:     tok,
		ID:        id,
		HostID:    hostID,
		ExpiresAt: expires,
		Label:     label,
		CreatedAt: now,
	}, nil
}

// newShareToken returns a fresh 32-byte random base64url-encoded
// token. The plaintext is the bearer; we store it verbatim in the
// DB (no hash) because the table is already time-bounded +
// revocable and a SHA-256 lookup would force a linear scan on every
// public hit. The risk model: anyone with read access to the
// hosts.db file can mint their own shares — but anyone with that
// access can already exfiltrate the agent ingest tokens too.
func newShareToken() (string, error) {
	b := make([]byte, shareTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("share: rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// PublicHostPayload is the shape the unauthenticated
// /api/public/host/{token} endpoint returns. Tags + system
// metadata are deliberately stripped (RFC 0004 §"Risks") so a
// leaked URL doesn't leak the host's full profile.
type PublicHostPayload struct {
	HostID   int64     `json:"host_id"`
	Name     string    `json:"name"`
	ExpiresAt time.Time `json:"expires_at"`
	// We don't include the share token in the response — the
	// operator saw it once at mint time and the URL is the bearer.
}

// FetchByShareToken looks up a token and returns the host payload
// + the share's expiry. Returns ErrShareNotFound for unknown,
// ErrShareExpired for past-expiry. The token is the bearer
// credential, so we don't hash-compare — direct equality is fine
// because the DB only ever has 1s of operator-issued tokens to
// brute-force at a time (TTL is hours).
func FetchByShareToken(ctx context.Context, db *sql.DB, token string) (PublicHostPayload, error) {
	if token == "" {
		return PublicHostPayload{}, ErrShareInvalid
	}
	var (
		hostID    int64
		expiresAt time.Time
	)
	err := db.QueryRowContext(ctx, `
		SELECT host_id, expires_at FROM host_share_tokens WHERE token = ?`,
		token,
	).Scan(&hostID, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return PublicHostPayload{}, ErrShareNotFound
	}
	if err != nil {
		return PublicHostPayload{}, fmt.Errorf("share: lookup: %w", err)
	}
	if !time.Now().UTC().Before(expiresAt) {
		return PublicHostPayload{}, ErrShareExpired
	}
	h, err := getByID(ctx, db, hostID)
	if err != nil {
		// Host was deleted between mint and fetch. The share
		// logically dies with the host — return 404 (ErrShareNotFound)
		// to avoid leaking host-ids.
		return PublicHostPayload{}, ErrShareNotFound
	}
	return PublicHostPayload{
		HostID:    h.ID,
		Name:      h.Name,
		ExpiresAt: expiresAt,
	}, nil
}

// RevokeShare deletes the row by token. The dispatcher / public
// route will see ErrShareNotFound on the next fetch. Idempotent:
// revoking a non-existent token is not an error.
func RevokeShare(ctx context.Context, db *sql.DB, token string) error {
	if token == "" {
		return ErrShareInvalid
	}
	_, err := db.ExecContext(ctx, `DELETE FROM host_share_tokens WHERE token = ?`, token)
	if err != nil {
		return fmt.Errorf("share: revoke: %w", err)
	}
	return nil
}

// SweepExpiredShares deletes every row whose expires_at is in the
// past. Returns the count so a heartbeat logger can surface it
// without re-querying. Idempotent + cheap on a small table.
func SweepExpiredShares(ctx context.Context, db *sql.DB, now time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM host_share_tokens WHERE expires_at <= ?`, now.UTC())
	if err != nil {
		return 0, fmt.Errorf("share: sweep: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}

// ListHostShares returns the active + recently-expired shares for
// one host. The plaintext Token is never returned — the operator
// who minted the link already has it (and it was shown once at
// mint time); listing it here would create a new exfil vector.
func ListHostShares(ctx context.Context, db *sql.DB, hostID int64) ([]ShareView, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.id, s.host_id, h.name, s.expires_at, s.label, s.created_at
		FROM host_share_tokens s
		JOIN hosts h ON h.id = s.host_id
		WHERE s.host_id = ?
		ORDER BY s.created_at DESC`,
		hostID,
	)
	if err != nil {
		return nil, fmt.Errorf("share: list: %w", err)
	}
	defer rows.Close()
	var out []ShareView
	for rows.Next() {
		var v ShareView
		if err := rows.Scan(&v.ID, &v.HostID, &v.HostName, &v.ExpiresAt, &v.Label, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
