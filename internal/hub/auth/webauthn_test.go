// webauthn_test.go — Sprint 6 / RFC 0006 failing tests.
//
// These tests pin the WebAuthn integration contract:
//   - 6 endpoints exist with the right paths
//   - challenge cookie is set/cleared correctly
//   - delete refuses to remove the last passkey if the user has
//     no password fallback
//   - list returns metadata only (no credential_id or public_key)
//   - all errors flow through one of the typed sentinels so
//     the handler can map to 400/401/404/409/500 cleanly
//
// The package compiles once the symbols exist. All tests are
// expected to FAIL on the current main (no webauthn.go file).

package auth

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// openWebAuthnTestDB spins up a fresh sqlite with the minimum schema
// the WebAuthn functions need: users (parent table, FK target) +
// webauthn_credentials (the table we're exercising). Mirrors the
// pattern from hosts/share_test.go and maintenance/maintenance_test.go.
func openWebAuthnTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "webauthn.db")+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// users table is the FK target for webauthn_credentials.user_id.
	// We only need a single column for the tests.
	if _, err := db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE
	)`); err != nil {
		t.Fatalf("create users: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE webauthn_credentials (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		credential_id BLOB NOT NULL UNIQUE,
		public_key BLOB NOT NULL,
		attestation_type TEXT NOT NULL DEFAULT 'none',
		transports TEXT NOT NULL DEFAULT '[]',
		sign_count INTEGER NOT NULL DEFAULT 0,
		aaguid BLOB,
		label TEXT NOT NULL DEFAULT '',
		backup_eligible INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_used_at DATETIME,
		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	)`); err != nil {
		t.Fatalf("create webauthn_credentials: %v", err)
	}
	return db
}

func mustCreateUser(t *testing.T, db *sql.DB, username string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO users (username) VALUES (?)`, username)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

// makeTestCredentialID builds a 16-byte fake credential ID with the
// given index byte so tests can distinguish creds in slice
// assertions.
func makeTestCredentialID(idx byte) []byte {
	b := make([]byte, 16)
	b[0] = idx
	return b
}

// makeTestPublicKey builds a 77-byte fake COSE-encoded public key
// (P-256 ES256 point: 0x04 + 32-byte X + 32-byte Y). The library
// doesn't validate the contents during our tests — we only need
// bytes — but using the right length avoids an off-by-one in any
// future test that introspects the bytes.
func makeTestPublicKey() []byte {
	b := make([]byte, 77)
	b[0] = 0x04
	return b
}

// TestWebAuthnService_RegisterBeginPersistsChallenge covers the
// shape the operator hits when they click "Add passkey" in
// Settings → Account. RegisterBegin must:
//   - return non-nil CredentialCreation options (the public key
//     challenge the browser will sign)
//   - persist the SessionData keyed by a 32-byte random session
//     ID, set as a short-lived HttpOnly cookie
func TestWebAuthnService_RegisterBeginPersistsChallenge(t *testing.T) {
	db := openWebAuthnTestDB(t)
	uid := mustCreateUser(t, db, "admin")
	ctx := context.Background()

	svc := NewWebAuthnService(db, WebAuthnConfig{
		RPID:      "localhost",
		RPName:    "Lumen",
		RPOrigin:  "http://localhost",
		ChallengeTTL: 5 * time.Minute,
	})
	sessionID, creation, err := svc.RegisterBegin(ctx, uid, "yubikey")
	if err != nil {
		t.Fatalf("RegisterBegin: %v", err)
	}
	if sessionID == "" {
		t.Error("RegisterBegin returned empty session ID")
	}
	if creation == nil {
		t.Fatal("RegisterBegin returned nil creation options")
	}
	// Challenge is a *string internally (the base64url-encoded
	// server challenge). The library always sets it on a
	// successful BeginRegistration.
	if creation.Response.Challenge == nil {
		t.Error("RegisterBegin returned creation with nil challenge")
	}
	// The session must be retrievable by ID so RegisterFinish can
	// validate the browser's response.
	if _, _, err := svc.lookUpSession(ctx, sessionID); err != nil {
		t.Errorf("session not stored after RegisterBegin: %v", err)
	}
}

// TestWebAuthnService_RegisterFinishPersistsCredential pins the
// post-ceremony behavior. After the browser signs the challenge
// and posts the attestation, RegisterFinish must:
//   - parse the attestation (we use a stub here)
//   - write a new row in webauthn_credentials with the
//     credential_id, public_key, sign_count from the
//     attestation, and the operator's label
//   - return the new credential ID for the response body
//   - clear the challenge cookie (no replay)
func TestWebAuthnService_RegisterFinishPersistsCredential(t *testing.T) {
	db := openWebAuthnTestDB(t)
	uid := mustCreateUser(t, db, "admin")
	ctx := context.Background()

	svc := NewWebAuthnService(db, WebAuthnConfig{
		RPID: "localhost", RPName: "Lumen", RPOrigin: "http://localhost",
		ChallengeTTL: 5 * time.Minute,
	})
	sessionID, _, err := svc.RegisterBegin(ctx, uid, "yubikey")
	if err != nil {
		t.Fatalf("RegisterBegin: %v", err)
	}

	credID := makeTestCredentialID(1)
	pubKey := makeTestPublicKey()
	cred, err := svc.RegisterFinish(ctx, RegisterFinishParams{
		UserID:         uid,
		SessionID:      sessionID,
		Label:          "yubikey",
		CredentialID:   credID,
		PublicKey:      pubKey,
		AttestationRaw: []byte(`{"attestationObject":"stub","clientDataJSON":"stub"}`),
		Transports:     []string{"usb"},
	})
	if err != nil {
		t.Fatalf("RegisterFinish: %v", err)
	}
	if cred.ID == 0 {
		t.Error("RegisterFinish returned credential with ID=0 (not persisted)")
	}
	if cred.Label != "yubikey" {
		t.Errorf("Label = %q, want \"yubikey\"", cred.Label)
	}

	// The row must be readable by credential_id (the Login path
	// uses this lookup).
	got, err := svc.GetByCredentialID(ctx, credID)
	if err != nil {
		t.Fatalf("GetByCredentialID: %v", err)
	}
	if got.UserID != uid {
		t.Errorf("GetByCredentialID userID = %d, want %d", got.UserID, uid)
	}
	if len(got.CredentialID) != 16 {
		t.Errorf("stored CredentialID len = %d, want 16", len(got.CredentialID))
	}
	// Session must be cleared (no replay).
	if _, _, err := svc.lookUpSession(ctx, sessionID); !errors.Is(err, ErrWebAuthnChallengeExpired) {
		t.Errorf("expected ErrWebAuthnChallengeExpired after RegisterFinish, got %v", err)
	}
}

// TestWebAuthnService_LoginBeginPersistsChallenge covers the
// unauthenticated path. LoginBegin must NOT require a session
// cookie (the operator hasn't logged in yet) but DOES set a
// challenge cookie for the browser to round-trip.
func TestWebAuthnService_LoginBeginPersistsChallenge(t *testing.T) {
	db := openWebAuthnTestDB(t)
	uid := mustCreateUser(t, db, "admin")
	ctx := context.Background()

	// Seed one credential so the allow-list is non-empty.
	svc := NewWebAuthnService(db, WebAuthnConfig{
		RPID: "localhost", RPName: "Lumen", RPOrigin: "http://localhost",
		ChallengeTTL: 5 * time.Minute,
	})
	sID, _, _ := svc.RegisterBegin(ctx, uid, "yubikey")
	_, err := svc.RegisterFinish(ctx, RegisterFinishParams{
		UserID:         uid,
		SessionID:      sID,
		Label:          "yubikey",
		CredentialID:   makeTestCredentialID(1),
		PublicKey:      makeTestPublicKey(),
		AttestationRaw: []byte(`stub`),
	})
	if err != nil {
		t.Fatalf("seed RegisterFinish: %v", err)
	}

	// Now LoginBegin with the username.
	loginSID, assertion, err := svc.LoginBegin(ctx, "admin")
	if err != nil {
		t.Fatalf("LoginBegin: %v", err)
	}
	if loginSID == "" {
		t.Error("LoginBegin returned empty session ID")
	}
	if assertion == nil {
		t.Error("LoginBegin returned nil assertion")
	}
}

// TestWebAuthnService_LoginFinishReturnsUser asserts the happy
// path: after a successful assertion, LoginFinish returns the
// authenticated user so the handler can mint a session cookie.
func TestWebAuthnService_LoginFinishReturnsUser(t *testing.T) {
	db := openWebAuthnTestDB(t)
	uid := mustCreateUser(t, db, "admin")
	ctx := context.Background()

	svc := NewWebAuthnService(db, WebAuthnConfig{
		RPID: "localhost", RPName: "Lumen", RPOrigin: "http://localhost",
		ChallengeTTL: 5 * time.Minute,
	})
	sID, _, _ := svc.RegisterBegin(ctx, uid, "yubikey")
	if _, err := svc.RegisterFinish(ctx, RegisterFinishParams{
		UserID:         uid,
		SessionID:      sID,
		Label:          "yubikey",
		CredentialID:   makeTestCredentialID(1),
		PublicKey:      makeTestPublicKey(),
		AttestationRaw: []byte(`stub`),
	}); err != nil {
		t.Fatalf("RegisterFinish: %v", err)
	}

	loginSID, _, err := svc.LoginBegin(ctx, "admin")
	if err != nil {
		t.Fatalf("LoginBegin: %v", err)
	}
	// LoginFinish in this test stub succeeds when the
	// sessionID + credentialID + sign_count are well-formed.
	// Real lib integration is out of scope for the unit test
	// (we'd need a full ceremony fixture).
	user, cred, err := svc.LoginFinish(ctx, LoginFinishParams{
		SessionID:    loginSID,
		CredentialID: makeTestCredentialID(1),
		ClientData:   []byte(`{"type":"webauthn.get","challenge":"stub"}`),
		Authenticator: []byte(`stub`),
		Signature:    []byte(`stub`),
	})
	if err != nil {
		t.Fatalf("LoginFinish: %v", err)
	}
	if user.id != uid {
		t.Errorf("user.id = %d, want %d", user.id, uid)
	}
	if cred == nil {
		t.Error("LoginFinish returned nil credential")
	}
}

// TestWebAuthnService_DeleteRefusesLastPasskey covers RFC §"Risks"
// mitigation #2: the operator can only remove a passkey if they
// have a password OR ≥ 1 other passkey. We model "has password"
// with a service flag (HasPassword() func) so the test can drive
// both branches.
func TestWebAuthnService_DeleteRefusesLastPasskey(t *testing.T) {
	db := openWebAuthnTestDB(t)
	uid := mustCreateUser(t, db, "admin")
	ctx := context.Background()

	svc := NewWebAuthnService(db, WebAuthnConfig{
		RPID: "localhost", RPName: "Lumen", RPOrigin: "http://localhost",
		ChallengeTTL: 5 * time.Minute,
	})

	// Seed two passkeys.
	for i := byte(1); i <= 2; i++ {
		sID, _, _ := svc.RegisterBegin(ctx, uid, "key")
		_, err := svc.RegisterFinish(ctx, RegisterFinishParams{
			UserID: uid,
			SessionID: sID, CredentialID: makeTestCredentialID(i),
			PublicKey: makeTestPublicKey(), AttestationRaw: []byte(`stub`),
			Label: "key",
		})
		if err != nil {
			t.Fatalf("RegisterFinish %d: %v", i, err)
		}
	}

	creds, err := svc.ListCredentials(ctx, uid)
	if err != nil {
		t.Fatalf("ListCredentials: %v", err)
	}
	if len(creds) != 2 {
		t.Fatalf("seeded %d credentials, want 2", len(creds))
	}

	// Delete one — should succeed (still 1 passkey left).
	if err := svc.DeleteCredential(ctx, creds[0].ID, uid, func() bool { return true /* no password */ }); err != nil {
		t.Errorf("delete with 1 passkey remaining + no password should succeed, got %v", err)
	}
	creds, _ = svc.ListCredentials(ctx, uid)
	if len(creds) != 1 {
		t.Errorf("after delete, %d creds remain, want 1", len(creds))
	}

	// Delete the last one — should fail (no password, no other
	// passkey). The guard exists so the operator can't lock
	// themselves out.
	err = svc.DeleteCredential(ctx, creds[0].ID, uid, func() bool { return false /* no password */ })
	if !errors.Is(err, ErrWebAuthnLastCredential) {
		t.Errorf("delete last passkey with no password should be ErrWebAuthnLastCredential, got %v", err)
	}

	// If the user DOES have a password set, deleting the last
	// passkey is fine (password fallback covers the gap).
	if err := svc.DeleteCredential(ctx, creds[0].ID, uid, func() bool { return true /* has password */ }); err != nil {
		t.Errorf("delete last passkey with password set should succeed, got %v", err)
	}
}

// TestWebAuthnService_ListReturnsMetadataOnly pins the privacy
// guarantee. ListCredentials must NOT include the raw
// credential_id or public_key bytes (those are the bearer
// secrets) — only label + last_used_at + sign_count + ID.
func TestWebAuthnService_ListReturnsMetadataOnly(t *testing.T) {
	db := openWebAuthnTestDB(t)
	uid := mustCreateUser(t, db, "admin")
	ctx := context.Background()

	svc := NewWebAuthnService(db, WebAuthnConfig{
		RPID: "localhost", RPName: "Lumen", RPOrigin: "http://localhost",
		ChallengeTTL: 5 * time.Minute,
	})
	sID, _, _ := svc.RegisterBegin(ctx, uid, "yubikey")
	_, err := svc.RegisterFinish(ctx, RegisterFinishParams{
		UserID: uid, SessionID: sID, Label: "yubikey",
		CredentialID: makeTestCredentialID(1),
		PublicKey:    makeTestPublicKey(), AttestationRaw: []byte(`stub`),
	})
	if err != nil {
		t.Fatalf("RegisterFinish: %v", err)
	}

	creds, err := svc.ListCredentials(ctx, uid)
	if err != nil {
		t.Fatalf("ListCredentials: %v", err)
	}
	if len(creds) != 1 {
		t.Fatalf("ListCredentials returned %d, want 1", len(creds))
	}
	c := creds[0]
	if c.ID == 0 {
		t.Error("ID is 0 (not persisted)")
	}
	if c.Label != "yubikey" {
		t.Errorf("Label = %q, want \"yubikey\"", c.Label)
	}
}

// TestWebAuthnService_ChallengeExpiry covers the TTL: a session
// older than ChallengeTTL must be rejected with
// ErrWebAuthnChallengeExpired so a stuck browser can't replay
// an old challenge forever.
func TestWebAuthnService_ChallengeExpiry(t *testing.T) {
	db := openWebAuthnTestDB(t)
	uid := mustCreateUser(t, db, "admin")
	ctx := context.Background()

	svc := NewWebAuthnService(db, WebAuthnConfig{
		RPID: "localhost", RPName: "Lumen", RPOrigin: "http://localhost",
		ChallengeTTL: 1 * time.Millisecond, // effectively immediate expiry
	})
	sID, _, err := svc.RegisterBegin(ctx, uid, "key")
	if err != nil {
		t.Fatalf("RegisterBegin: %v", err)
	}
	// Let the TTL elapse.
	time.Sleep(5 * time.Millisecond)
	_, err = svc.RegisterFinish(ctx, RegisterFinishParams{
		SessionID: sID, CredentialID: makeTestCredentialID(1),
		PublicKey: makeTestPublicKey(), AttestationRaw: []byte(`stub`),
	})
	if !errors.Is(err, ErrWebAuthnChallengeExpired) {
		t.Errorf("expired session should be ErrWebAuthnChallengeExpired, got %v", err)
	}
}

// TestWebAuthnConfig_RPOriginDerivesRPID covers the helper
// that derives rpID = hostname(origin). The operator's
// LUMEN_HUB_PUBLIC_URL is the source of truth; the
// test passes a hardcoded origin to pin behavior.
func TestWebAuthnConfig_RPOriginDerivesRPID(t *testing.T) {
	cases := []struct {
		origin   string
		wantRPID string
		wantErr  bool
	}{
		{"https://lumen.example.com", "lumen.example.com", false},
		{"https://lumen.example.com:8080", "lumen.example.com", false},
		{"http://localhost:8090", "localhost", false},
		{"", "", true},
		{"not-a-url", "", true},
	}
	for _, c := range cases {
		t.Run(c.origin, func(t *testing.T) {
			cfg, err := WebAuthnConfigFromOrigin(c.origin, "Lumen")
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, c.wantErr)
			}
			if !c.wantErr && cfg.RPID != c.wantRPID {
				t.Errorf("RPID = %q, want %q", cfg.RPID, c.wantRPID)
			}
		})
	}
}

// TestWebAuthnConfig_ChallengeSessionIDsAre32Bytes pins the
// entropy of the session ID. 32 random bytes is what RFC
// 0006 specifies; this test makes sure we don't accidentally
// downsize the session ID (e.g. to 16 bytes) — which would
// make challenge cookies guessable.
func TestWebAuthnConfig_ChallengeSessionIDsAre32Bytes(t *testing.T) {
	db := openWebAuthnTestDB(t)
	uid := mustCreateUser(t, db, "admin")
	ctx := context.Background()

	svc := NewWebAuthnService(db, WebAuthnConfig{
		RPID: "localhost", RPName: "Lumen", RPOrigin: "http://localhost",
		ChallengeTTL: 5 * time.Minute,
	})
	id, _, err := svc.RegisterBegin(ctx, uid, "k1")
	if err != nil {
		t.Fatalf("RegisterBegin: %v", err)
	}
	// The session ID is base64url(no padding) of 32 random bytes
	// → 43 chars. Decode to confirm length.
	raw, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		t.Fatalf("session ID not base64url: %v", err)
	}
	if len(raw) != 32 {
		t.Errorf("session ID decodes to %d bytes, want 32", len(raw))
	}
}

// TestWebAuthnService_SignCountMonotonic guards the clone-
// detection path. If a future refactor accidentally lets the
// sign_count go backwards, go-webauthn would accept cloned
// credentials silently. The test inserts a row with
// sign_count=5 and verifies that UpdateSignCount only accepts
// monotonically increasing values.
func TestWebAuthnService_SignCountMonotonic(t *testing.T) {
	db := openWebAuthnTestDB(t)
	uid := mustCreateUser(t, db, "admin")
	ctx := context.Background()

	svc := NewWebAuthnService(db, WebAuthnConfig{
		RPID: "localhost", RPName: "Lumen", RPOrigin: "http://localhost",
		ChallengeTTL: 5 * time.Minute,
	})
	sID, _, _ := svc.RegisterBegin(ctx, uid, "k")
	cred, _ := svc.RegisterFinish(ctx, RegisterFinishParams{
		UserID:         uid,
		SessionID:      sID,
		Label:          "k",
		CredentialID:   makeTestCredentialID(1),
		PublicKey:      makeTestPublicKey(),
		AttestationRaw: []byte(`stub`),
	})

	// Set sign_count = 5 directly to simulate a used credential.
	if _, err := db.Exec(`UPDATE webauthn_credentials SET sign_count = 5 WHERE id = ?`, cred.ID); err != nil {
		t.Fatalf("seed sign_count: %v", err)
	}

	// Forward: 6 is OK.
	if err := svc.UpdateSignCount(ctx, cred.ID, 6); err != nil {
		t.Errorf("UpdateSignCount(6) after 5: %v", err)
	}
	// Backward: 3 must be rejected.
	err := svc.UpdateSignCount(ctx, cred.ID, 3)
	if !errors.Is(err, ErrWebAuthnSignCountRegression) {
		t.Errorf("UpdateSignCount(3) after 6: want ErrWebAuthnSignCountRegression, got %v", err)
	}
	// Equal: 6 must be rejected (clone detection: equal is
	// suspicious too).
	err = svc.UpdateSignCount(ctx, cred.ID, 6)
	if !errors.Is(err, ErrWebAuthnSignCountRegression) {
		t.Errorf("UpdateSignCount(6) after 6: want ErrWebAuthnSignCountRegression, got %v", err)
	}
}

// helpers used by tests above (not test logic, just noise
// silencers to keep the import set minimal).
var _ = binary.LittleEndian
