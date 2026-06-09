// share_test.go — RFC 0004 §"Per-host share link" failing tests.
//
// The share-link feature mints a 32-byte random token bound to one
// host with a TTL. The public route at /h/{token} renders a read-
// only host detail. This test file pins the contract: token shape,
// mint/fetch/expire/revoke, and the background sweep.
//
// All tests are expected to FAIL on v0.7.3 (no host_share_tokens
// table, no MintShare / FetchByShareToken / SweepExpired functions,
// no public host endpoint).
package hosts

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// openShareTestDB spins up a fresh sqlite with the minimum schema:
// the real `hosts` table (so MintShare can FK to it) + the new
// `host_share_tokens` table (so the new code under test can scan
// rows). Mirrors the openTestDB pattern from maintenance_test.go and
// backup/snapshot_test.go.
func openShareTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "share.db")+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Bare-minimum hosts table (just the columns the share code
	// touches via getByID, which reads hostColumns — the production
	// hosts package reads ~16 columns, but the share path only
	// scans id + name. We provide a wider schema for compatibility
	// with the production SELECT shape, but the test's mustCreateHost
	// helper only inserts id + name + token_hash).
	if _, err := db.Exec(`CREATE TABLE hosts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		token_hash TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_seen_at DATETIME,
		system_os TEXT,
		system_hostname TEXT,
		system_primary_ip TEXT,
		system_kernel TEXT,
		system_arch TEXT,
		system_cpu_model TEXT,
		system_uptime_seconds INTEGER,
		system_virt_type TEXT,
		agent_version TEXT,
		metadata_updated_at DATETIME,
		silenced_until INTEGER,
		public_visible INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		t.Fatalf("create hosts: %v", err)
	}
	// The new table Sprint 4 has to add.
	if _, err := db.Exec(`CREATE TABLE host_share_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		token TEXT NOT NULL UNIQUE,
		host_id INTEGER NOT NULL,
		expires_at DATETIME NOT NULL,
		label TEXT NOT NULL DEFAULT '',
		created_by INTEGER,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE
	)`); err != nil {
		t.Fatalf("create host_share_tokens: %v", err)
	}
	if _, err := db.Exec(`CREATE INDEX idx_share_token_expires ON host_share_tokens(expires_at)`); err != nil {
		t.Fatalf("create share index: %v", err)
	}
	return db
}

// mustRawToken returns a fresh 32-byte base64url token. The tests
// that need to insert an already-expired share row (MintShare
// rejects ttl < MinShareTTL) use this to bypass the validator
// while still producing tokens of the same shape MintShare
// generates.
func mustRawToken(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// mustCreateHost inserts a host row directly so the share tests
// don't have to drive the production Create() helper (which mints a
// token we don't need). Returns the host id.
func mustCreateHost(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO hosts (name, token_hash) VALUES (?, 'unused-hash')`, name)
	if err != nil {
		t.Fatalf("insert host: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// TestMintShare_TokenShape asserts the token is 32 random bytes
// base64url-encoded (no padding). The plaintext is what the operator
// hands to a teammate; the hash is what we store. The shape must
// stay stable so existing share links keep working across upgrades.
func TestMintShare_TokenShape(t *testing.T) {
	db := openShareTestDB(t)
	hostID := mustCreateHost(t, db, "h1")
	ctx := context.Background()

	share, err := MintShare(ctx, db, hostID, 24*time.Hour, "ops handoff", 0)
	if err != nil {
		t.Fatalf("MintShare: %v", err)
	}
	if share.Token == "" {
		t.Fatal("MintShare returned empty token")
	}
	// 32 random bytes → 43 base64url chars (no padding).
	raw, err := base64.RawURLEncoding.DecodeString(share.Token)
	if err != nil {
		t.Fatalf("token is not base64url: %v", err)
	}
	if len(raw) != 32 {
		t.Errorf("token decodes to %d bytes, want 32", len(raw))
	}
	if share.ExpiresAt.IsZero() {
		t.Error("ExpiresAt not set")
	}
	if share.Label != "ops handoff" {
		t.Errorf("Label = %q, want \"ops handoff\"", share.Label)
	}
}

// TestFetchByShareToken_HappyPath covers the read path: a freshly
// minted token returns the host name + a snapshot of read-only
// fields. The fetch must NOT return the bearer token_hash — a
// leaked share URL would otherwise leak the agent's ingest token.
func TestFetchByShareToken_HappyPath(t *testing.T) {
	db := openShareTestDB(t)
	hostID := mustCreateHost(t, db, "db-prod")
	ctx := context.Background()

	share, err := MintShare(ctx, db, hostID, 24*time.Hour, "", 0)
	if err != nil {
		t.Fatalf("MintShare: %v", err)
	}

	payload, err := FetchByShareToken(ctx, db, share.Token)
	if err != nil {
		t.Fatalf("FetchByShareToken: %v", err)
	}
	if payload.Name != "db-prod" {
		t.Errorf("payload.Name = %q, want db-prod", payload.Name)
	}
	// PublicHostPayload must NOT include any agent-bearer surface
	// (the share URL is the bearer; we don't echo it back, and the
	// host's token_hash never appears in the unauth payload).
	if payload.ExpiresAt.IsZero() {
		t.Error("ExpiresAt not set in public payload")
	}
}

// TestFetchByShareToken_ExpiredReturns404 covers expiry: a token
// whose expires_at is in the past must return ErrShareExpired (or
// map to 404 in the handler). The current RFC pins expiry as
// MANDATORY — there is no permanent share knob. We insert the
// expired row directly because MintShare validates ttl > MinShareTTL
// (1h), so we can't go through the public API to make an
// already-expired token.
func TestFetchByShareToken_ExpiredReturns404(t *testing.T) {
	db := openShareTestDB(t)
	hostID := mustCreateHost(t, db, "h1")
	ctx := context.Background()

	// Insert an already-expired share row directly.
	tok := mustRawToken(t)
	_, err := db.ExecContext(ctx,
		`INSERT INTO host_share_tokens (token, host_id, expires_at, label) VALUES (?, ?, ?, ?)`,
		tok, hostID, time.Now().UTC().Add(-1*time.Hour), "already gone")
	if err != nil {
		t.Fatalf("insert expired: %v", err)
	}
	_, err = FetchByShareToken(ctx, db, tok)
	if err == nil {
		t.Fatal("expected error fetching expired token, got nil")
	}
	if !errors.Is(err, ErrShareExpired) && !errors.Is(err, ErrShareNotFound) {
		t.Errorf("expected ErrShareExpired or ErrShareNotFound, got %v", err)
	}
}

// TestFetchByShareToken_UnknownReturns404 covers the typo / brute
// force case: a token that doesn't exist returns ErrShareNotFound.
// The handler maps this to 404 to avoid leaking which hosts have
// shares.
func TestFetchByShareToken_UnknownReturns404(t *testing.T) {
	db := openShareTestDB(t)
	_, err := FetchByShareToken(context.Background(), db, "never-minted-token-xxxxxxxxxxxxxxxx")
	if !errors.Is(err, ErrShareNotFound) {
		t.Errorf("expected ErrShareNotFound, got %v", err)
	}
}

// TestRevokeShare covers the DELETE /api/share/{token} path. After
// revocation the token must NOT resolve; the next fetch returns
// ErrShareNotFound (or ErrShareRevoked — either is acceptable as
// long as the handler maps it to 404).
func TestRevokeShare(t *testing.T) {
	db := openShareTestDB(t)
	hostID := mustCreateHost(t, db, "h1")
	ctx := context.Background()

	share, err := MintShare(ctx, db, hostID, 24*time.Hour, "", 0)
	if err != nil {
		t.Fatalf("MintShare: %v", err)
	}
	if err := RevokeShare(ctx, db, share.Token); err != nil {
		t.Fatalf("RevokeShare: %v", err)
	}
	_, err = FetchByShareToken(ctx, db, share.Token)
	if !errors.Is(err, ErrShareNotFound) && !errors.Is(err, ErrShareRevoked) && !errors.Is(err, ErrShareExpired) {
		t.Errorf("expected post-revoke error, got %v", err)
	}
}

// TestSweepExpiredShares covers the background goroutine. The
// SweepExpired function deletes every row whose expires_at is in
// the past. Returns the number of rows deleted so a heartbeat
// logger can surface the count without re-querying.
func TestSweepExpiredShares(t *testing.T) {
	db := openShareTestDB(t)
	hostID := mustCreateHost(t, db, "h1")
	ctx := context.Background()

	// 2 expired (inserted directly — MintShare rejects ttl < 1h),
	// 1 valid via MintShare.
	for i := 0; i < 2; i++ {
		tok := mustRawToken(t)
		if _, err := db.ExecContext(ctx,
			`INSERT INTO host_share_tokens (token, host_id, expires_at, label) VALUES (?, ?, ?, ?)`,
			tok, hostID, time.Now().UTC().Add(-1*time.Hour), "old"); err != nil {
			t.Fatalf("insert expired: %v", err)
		}
	}
	if _, err := MintShare(ctx, db, hostID, 24*time.Hour, "valid", 0); err != nil {
		t.Fatalf("MintShare valid: %v", err)
	}
	deleted, err := SweepExpiredShares(ctx, db, time.Now().UTC())
	if err != nil {
		t.Fatalf("SweepExpiredShares: %v", err)
	}
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2 (only the expired rows)", deleted)
	}
	// The valid row must still be there.
	var remaining int
	if err := db.QueryRow(`SELECT COUNT(*) FROM host_share_tokens`).Scan(&remaining); err != nil {
		t.Fatalf("count: %v", err)
	}
	if remaining != 1 {
		t.Errorf("remaining = %d, want 1", remaining)
	}
}

// TestListHostShares covers GET /api/hosts/{id}/shares: the admin
// needs to see "what share links are currently valid" so they can
// revoke on demand.
func TestListHostShares(t *testing.T) {
	db := openShareTestDB(t)
	hostID := mustCreateHost(t, db, "h1")
	ctx := context.Background()

	if _, err := MintShare(ctx, db, hostID, 24*time.Hour, "ops", 0); err != nil {
		t.Fatalf("MintShare: %v", err)
	}
	if _, err := MintShare(ctx, db, hostID, 1*time.Hour, "view-only", 0); err != nil {
		t.Fatalf("MintShare: %v", err)
	}
	shares, err := ListHostShares(ctx, db, hostID)
	if err != nil {
		t.Fatalf("ListHostShares: %v", err)
	}
	if len(shares) != 2 {
		t.Errorf("len(shares) = %d, want 2", len(shares))
	}
	// The plaintext token MUST NOT be in the list response — only
	// the metadata the operator needs to decide whether to revoke.
	for _, s := range shares {
		// ShareView intentionally omits the Token field; we assert
		// HostName + ExpiresAt + Label populated instead.
		if s.HostName != "h1" {
			t.Errorf("share view HostName = %q, want h1", s.HostName)
		}
		if s.ExpiresAt.IsZero() {
			t.Errorf("share %q missing ExpiresAt", s.Label)
		}
	}
}
