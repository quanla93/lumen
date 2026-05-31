// Package hosts owns the hosts table: CRUD plus per-host bearer-token
// minting, hashing, lookup, and last-seen tracking.
//
// Tokens are 256 bits of urandom prefixed with "lum_" and base64-URL
// encoded. The hub stores only their SHA-256 hash; the plaintext is
// returned to the operator exactly once (at create / rotate time) and
// never persisted. SHA-256 is appropriate here because tokens have full
// machine-generated entropy — Argon2id would be wasted work per ingest.
package hosts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/quanla93/lumen/internal/shared/api"
)

const (
	TokenPrefix = "lum_"
	tokenBytes  = 32
)

var (
	ErrNotFound     = errors.New("host not found")
	ErrNameRequired = errors.New("name required")
	ErrInvalidToken = errors.New("invalid token")
	ErrNameTaken    = errors.New("host name already taken")
)

type Host struct {
	ID                  int64
	Name                string
	CreatedAt           time.Time
	LastSeenAt          sql.NullTime
	SystemOS            sql.NullString
	SystemHostname      sql.NullString
	SystemPrimaryIP     sql.NullString
	SystemKernel        sql.NullString
	SystemArch          sql.NullString
	SystemCPUModel      sql.NullString
	SystemUptimeSeconds sql.NullInt64
	AgentVersion        sql.NullString
	MetadataUpdatedAt   sql.NullTime
	// SilencedUntil is a unix timestamp (seconds). NULL or past = not
	// silenced; future = alerts engine skips event+notify for this host
	// until the time passes. Set via /api/hosts/{id}/silence.
	SilencedUntil sql.NullInt64
}

// newToken produces a fresh plaintext token + its SHA-256 hex hash.
func newToken() (plain, hashHex string, err error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("rand: %w", err)
	}
	plain = TokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	hashHex = hashToken(plain)
	return plain, hashHex, nil
}

func hashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func validateName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrNameRequired
	}
	if len(name) > 64 {
		return errors.New("name too long (max 64 chars)")
	}
	for _, r := range name {
		if !(r == '-' || r == '_' || r == '.' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9')) {
			return errors.New("name may only contain letters, digits, '-', '_', '.'")
		}
	}
	return nil
}

const hostColumns = `id, name, created_at, last_seen_at,
	system_os, system_hostname, system_primary_ip, system_kernel,
	system_arch, system_cpu_model, system_uptime_seconds,
	agent_version, metadata_updated_at, silenced_until`

func scanHost(scanner interface{ Scan(dest ...any) error }) (Host, error) {
	var h Host
	err := scanner.Scan(
		&h.ID, &h.Name, &h.CreatedAt, &h.LastSeenAt,
		&h.SystemOS, &h.SystemHostname, &h.SystemPrimaryIP, &h.SystemKernel,
		&h.SystemArch, &h.SystemCPUModel, &h.SystemUptimeSeconds,
		&h.AgentVersion, &h.MetadataUpdatedAt, &h.SilencedUntil,
	)
	return h, err
}

// SetSilence stores silenced_until on the host. Pass a zero time to
// clear the silence (UPDATE ... = NULL). Returns ErrNotFound if no row
// matches the id.
func SetSilence(ctx context.Context, db *sql.DB, id int64, until time.Time) error {
	var v sql.NullInt64
	if !until.IsZero() {
		v = sql.NullInt64{Int64: until.Unix(), Valid: true}
	}
	res, err := db.ExecContext(ctx,
		`UPDATE hosts SET silenced_until = ? WHERE id = ?`, v, id)
	if err != nil {
		return fmt.Errorf("update silence: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// List returns every host, ordered by name.
func List(ctx context.Context, db *sql.DB) ([]Host, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+hostColumns+` FROM hosts ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Host
	for rows.Next() {
		h, err := scanHost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// Create mints a token, stores its hash, and returns the new Host plus
// the PLAINTEXT token (caller shows it to the operator exactly once).
func Create(ctx context.Context, db *sql.DB, name string) (Host, string, error) {
	name = strings.TrimSpace(name)
	if err := validateName(name); err != nil {
		return Host{}, "", err
	}
	plain, hashHex, err := newToken()
	if err != nil {
		return Host{}, "", err
	}
	res, err := db.ExecContext(ctx,
		`INSERT INTO hosts (name, token_hash) VALUES (?, ?)`,
		name, hashHex,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Host{}, "", ErrNameTaken
		}
		return Host{}, "", fmt.Errorf("insert host: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Host{}, "", err
	}
	h, err := getByID(ctx, db, id)
	if err != nil {
		return Host{}, "", err
	}
	return h, plain, nil
}

// Delete removes a host. Past snapshots stay (audit trail).
func Delete(ctx context.Context, db *sql.DB, id int64) error {
	res, err := db.ExecContext(ctx, `DELETE FROM hosts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Rotate generates a new token for the host, replaces the stored hash,
// and returns the new plaintext.
func Rotate(ctx context.Context, db *sql.DB, id int64) (string, error) {
	plain, hashHex, err := newToken()
	if err != nil {
		return "", err
	}
	res, err := db.ExecContext(ctx,
		`UPDATE hosts SET token_hash = ? WHERE id = ?`,
		hashHex, id,
	)
	if err != nil {
		return "", err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", ErrNotFound
	}
	return plain, nil
}

// VerifyToken returns the host owning the given plaintext token, or
// ErrInvalidToken if the prefix is wrong / nothing matches.
func VerifyToken(ctx context.Context, db *sql.DB, plain string) (Host, error) {
	if !strings.HasPrefix(plain, TokenPrefix) {
		return Host{}, ErrInvalidToken
	}
	hashHex := hashToken(plain)
	h, err := scanHost(db.QueryRowContext(ctx,
		`SELECT `+hostColumns+` FROM hosts WHERE token_hash = ?`,
		hashHex,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return Host{}, ErrInvalidToken
	}
	if err != nil {
		return Host{}, err
	}
	return h, nil
}

// TouchLastSeen sets last_seen_at = now for the host. Best-effort —
// ingest doesn't fail if this update errors.
func TouchLastSeen(ctx context.Context, db *sql.DB, id int64) error {
	_, err := db.ExecContext(ctx,
		`UPDATE hosts SET last_seen_at = CURRENT_TIMESTAMP WHERE id = ?`,
		id,
	)
	return err
}

func UpdateSystemMetadata(ctx context.Context, db *sql.DB, id int64, meta api.SystemMetadata) error {
	_, err := db.ExecContext(ctx, `
		UPDATE hosts SET
			system_os = ?,
			system_hostname = ?,
			system_primary_ip = ?,
			system_kernel = ?,
			system_arch = ?,
			system_cpu_model = ?,
			system_uptime_seconds = ?,
			agent_version = ?,
			metadata_updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		nullString(meta.OS),
		nullString(meta.Hostname),
		nullString(meta.PrimaryIP),
		nullString(meta.Kernel),
		nullString(meta.Arch),
		nullString(meta.CPUModel),
		nullInt64(meta.UptimeSeconds),
		nullString(meta.AgentVersion),
		id,
	)
	return err
}

func getByID(ctx context.Context, db *sql.DB, id int64) (Host, error) {
	h, err := scanHost(db.QueryRowContext(ctx, `SELECT `+hostColumns+` FROM hosts WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Host{}, ErrNotFound
	}
	return h, err
}

// isUniqueViolation matches modernc.org/sqlite's UNIQUE constraint error.
// Kept as a string check to avoid importing the driver package here.
func nullString(s string) sql.NullString {
	s = strings.TrimSpace(s)
	return sql.NullString{String: s, Valid: s != ""}
}

func nullInt64(v uint64) sql.NullInt64 {
	if v == 0 || v > uint64(^uint64(0)>>1) {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(v), Valid: true}
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
