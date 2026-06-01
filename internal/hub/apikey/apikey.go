// Package apikey backs the Public Read API's bearer keys (Phase 7 /
// v0.5.0). Keys are 32 random bytes prefixed with "lumk_", stored as a
// SHA-256 hex hash. Plaintext is returned exactly once at create time
// and never persisted — same scheme as host tokens.
//
// pr1 ships CRUD only (admin mint/list/revoke under a session). The
// public verify path that touches last_used_at on every incoming
// /api/v1/* request lands in pr2.
package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// TokenPrefix marks every key so log scanners can fingerprint a leak
	// across stdout/journald/CI artifacts. Pre-1.0 we use a distinct
	// prefix from host tokens ("lum_") so the two are never confused.
	TokenPrefix = "lumk_"
	tokenBytes  = 32

	// previewChars is how much of the plaintext we persist for display
	// in the list view ("lumk_AbCdEfGh…"). Short enough that even if a
	// preview leaked it doesn't grant access; long enough that an
	// operator can recognise which key is which.
	previewChars = 12
)

// Valid scopes for v0.5.0. Read-only — write scopes are deferred.
const (
	ScopeReadHosts   = "read:hosts"
	ScopeReadMetrics = "read:metrics"
	ScopeReadAlerts  = "read:alerts"
)

var validScopes = map[string]struct{}{
	ScopeReadHosts:   {},
	ScopeReadMetrics: {},
	ScopeReadAlerts:  {},
}

var (
	ErrNotFound        = errors.New("api key not found")
	ErrNameRequired    = errors.New("name required")
	ErrNameTooLong     = errors.New("name too long (max 64 chars)")
	ErrScopesRequired  = errors.New("at least one scope required")
	ErrScopeUnknown    = errors.New("unknown scope")
	ErrFilterTooLong   = errors.New("host_filter too long (max 256 chars)")
)

// Key is the metadata shape returned to the admin UI. Hash is never
// included — only the preview ("lumk_AbCdEfGh") is safe to display.
type Key struct {
	ID         string
	Name       string
	Preview    string
	Scopes     []string
	HostFilter *string
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

// Created bundles the metadata with the plaintext token — returned by
// Create exactly once. The caller is expected to display it then drop
// it on the floor.
type Created struct {
	Key
	Plaintext string
}

// Create mints a fresh key + writes a row. The returned *Created has
// the plaintext token; everywhere else only the Key (metadata) is
// available. Caller-supplied scopes are validated; an unknown scope
// fails the whole call.
func Create(ctx context.Context, db *sql.DB, name string, scopes []string, hostFilter *string) (*Created, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	if err := validateScopes(scopes); err != nil {
		return nil, err
	}
	if hostFilter != nil {
		if err := validateHostFilter(*hostFilter); err != nil {
			return nil, err
		}
		if strings.TrimSpace(*hostFilter) == "" {
			hostFilter = nil
		}
	}

	plain, hashHex, preview, err := mintToken()
	if err != nil {
		return nil, err
	}
	id := newID()
	scopesJSON, _ := json.Marshal(scopes)

	now := time.Now().Unix()
	_, err = db.ExecContext(ctx,
		`INSERT INTO api_keys (id, name, hash, preview, scopes, host_filter, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, strings.TrimSpace(name), hashHex, preview, string(scopesJSON), hostFilter, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert api_key: %w", err)
	}

	return &Created{
		Key: Key{
			ID:         id,
			Name:       strings.TrimSpace(name),
			Preview:    preview,
			Scopes:     scopes,
			HostFilter: hostFilter,
			CreatedAt:  time.Unix(now, 0).UTC(),
		},
		Plaintext: plain,
	}, nil
}

// List returns every key with metadata-only fields. Ordered most-recent
// first so newly minted keys appear at the top of the admin tab.
func List(ctx context.Context, db *sql.DB) ([]Key, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, name, preview, scopes, host_filter, last_used_at, created_at
		 FROM api_keys ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Key
	for rows.Next() {
		var (
			k          Key
			scopesJSON string
			filter     sql.NullString
			lastUsed   sql.NullInt64
			created    int64
		)
		if err := rows.Scan(&k.ID, &k.Name, &k.Preview, &scopesJSON, &filter, &lastUsed, &created); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(scopesJSON), &k.Scopes)
		if filter.Valid {
			s := filter.String
			k.HostFilter = &s
		}
		if lastUsed.Valid {
			t := time.Unix(lastUsed.Int64, 0).UTC()
			k.LastUsedAt = &t
		}
		k.CreatedAt = time.Unix(created, 0).UTC()
		out = append(out, k)
	}
	return out, rows.Err()
}

// VerifyAndTouch is the public-API verify hot path: hash the incoming
// plaintext bearer token, look it up, and update last_used_at so the
// admin UI can show "last used 2m ago". Returns ErrNotFound if no row
// matches — the caller maps that to 401.
//
// The update is fire-and-forget (we don't fail the request if the
// touch fails, since the auth itself succeeded).
func VerifyAndTouch(ctx context.Context, db *sql.DB, plaintext string) (*Key, error) {
	sum := sha256.Sum256([]byte(plaintext))
	hashHex := hex.EncodeToString(sum[:])

	var (
		k          Key
		scopesJSON string
		filter     sql.NullString
		lastUsed   sql.NullInt64
		created    int64
	)
	row := db.QueryRowContext(ctx,
		`SELECT id, name, preview, scopes, host_filter, last_used_at, created_at
		 FROM api_keys WHERE hash = ?`, hashHex)
	if err := row.Scan(&k.ID, &k.Name, &k.Preview, &scopesJSON, &filter, &lastUsed, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(scopesJSON), &k.Scopes)
	if filter.Valid {
		s := filter.String
		k.HostFilter = &s
	}
	if lastUsed.Valid {
		t := time.Unix(lastUsed.Int64, 0).UTC()
		k.LastUsedAt = &t
	}
	k.CreatedAt = time.Unix(created, 0).UTC()

	// fire-and-forget touch
	_, _ = db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = ? WHERE id = ?`,
		time.Now().Unix(), k.ID)

	return &k, nil
}

// HasScope is a small helper so middleware/handlers don't reach into
// the Scopes slice directly.
func (k *Key) HasScope(scope string) bool {
	for _, s := range k.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// Delete revokes a key. Returns ErrNotFound if no row was deleted so
// the handler can return a clean 404.
func Delete(ctx context.Context, db *sql.DB, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func mintToken() (plain, hashHex, preview string, err error) {
	b := make([]byte, tokenBytes)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("rand: %w", err)
	}
	plain = TokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(plain))
	hashHex = hex.EncodeToString(sum[:])
	if len(plain) >= previewChars {
		preview = plain[:previewChars]
	} else {
		preview = plain
	}
	return plain, hashHex, preview, nil
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func validateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrNameRequired
	}
	if len(name) > 64 {
		return ErrNameTooLong
	}
	return nil
}

func validateScopes(scopes []string) error {
	if len(scopes) == 0 {
		return ErrScopesRequired
	}
	for _, s := range scopes {
		if _, ok := validScopes[s]; !ok {
			return fmt.Errorf("%w: %q", ErrScopeUnknown, s)
		}
	}
	return nil
}

func validateHostFilter(f string) error {
	if len(f) > 256 {
		return ErrFilterTooLong
	}
	return nil
}
