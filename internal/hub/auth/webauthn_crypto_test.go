// webauthn_crypto_test.go — regression tests for issue #44.
//
// Pre-fix, RegisterFinish and LoginFinish were stubs that skipped
// the cryptographic verification the go-webauthn library
// normally performs. An attacker who knew a valid SessionID +
// CredentialID could authenticate as any user without possessing
// the private key. The fix wires (*WebAuthn).FinishRegistration
// and (*WebAuthn).FinishLogin with a real *http.Request so the
// library does the full attestation/signature verification.
//
// These tests pin the new behavior at the boundary: they submit
// intentionally-malformed JSON to the production path and
// assert the call is rejected, which can only happen if the
// library's verifier ran (a stub would persist + return success).
// The unit-friendly "happy path" lives in the existing
// RegisterFinishRaw / LoginFinishRaw fixtures.

package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestRegisterFinish_RejectsInvalidJSON is the regression guard
// for issue #44 (WebAuthn passkey stubs skip crypto verify).
//
// Setup: real WebAuthnService backed by sqlite. The challenge
// session is registered via RegisterBegin so we have a valid
// sessionID + the library has a stored challenge to verify
// against. The HTTP body we hand to RegisterFinish is
// intentionally malformed JSON.
//
// Pre-fix: RegisterFinish persisted the credential row
// (p.CredentialID, "none" attestation) and returned success.
//
// Post-fix: RegisterFinish calls s.wa.FinishRegistration which
// fails JSON-parse and returns an error. We assert:
//   1. The error is non-nil (caller can map to 400).
//   2. No row landed in webauthn_credentials (defense in depth
//      against the stub ever re-appearing).
func TestRegisterFinish_RejectsInvalidJSON(t *testing.T) {
	db := openWebAuthnTestDB(t)
	uid := mustCreateUser(t, db, "admin")
	ctx := context.Background()

	svc, err := NewWebAuthnService(db, WebAuthnConfig{
		RPID: "localhost", RPName: "Lumen", RPOrigin: "http://localhost",
		ChallengeTTL: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewWebAuthnService: %v", err)
	}
	sessionID, _, err := svc.RegisterBegin(ctx, uid, "yubikey")
	if err != nil {
		t.Fatalf("RegisterBegin: %v", err)
	}

	// Submit malformed JSON — library will fail to parse the
	// CredentialCreationResponse.
	req := httptest.NewRequest(http.MethodPost, "/api/auth/webauthn/register/finish",
		// body is intentionally not a valid CredentialCreationResponse
		invalidJSONReader())
	_, err = svc.RegisterFinish(ctx, RegisterFinishParams{
		SessionID: sessionID,
		UserID:    uid,
		Label:     "yubikey",
	}, req)
	if err == nil {
		t.Fatal("RegisterFinish accepted invalid JSON — #44 regression: crypto verify is being skipped")
	}

	// Defense in depth: confirm no credential row landed in the
	// DB. If the stub ever creeps back in, this catches it.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM webauthn_credentials`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("RegisterFinish persisted a row despite invalid JSON (n=%d) — #44 regression", n)
	}
}

// TestLoginFinish_RejectsInvalidJSON is the symmetric regression
// guard for the login path. Same shape: real session + real
// request, malformed body, expect error + no sign_count bump.
func TestLoginFinish_RejectsInvalidJSON(t *testing.T) {
	db := openWebAuthnTestDB(t)
	uid := mustCreateUser(t, db, "admin")
	ctx := context.Background()

	svc, err := NewWebAuthnService(db, WebAuthnConfig{
		RPID: "localhost", RPName: "Lumen", RPOrigin: "http://localhost",
		ChallengeTTL: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewWebAuthnService: %v", err)
	}
	// Seed one credential so the user has something to assert
	// against.
	sessID, _, err := svc.RegisterBegin(ctx, uid, "yubikey")
	if err != nil {
		t.Fatalf("RegisterBegin: %v", err)
	}
	credID := makeTestCredentialID(1)
	pubKey := makeTestPublicKey()
	seeded, err := svc.RegisterFinishRaw(ctx, RegisterFinishParams{
		SessionID:    sessID,
		UserID:       uid,
		Label:        "yubikey",
		CredentialID: credID,
		PublicKey:    pubKey,
	})
	if err != nil {
		t.Fatalf("seed RegisterFinishRaw: %v", err)
	}

	// Start a real login ceremony so we have a valid
	// sessionID + challenge.
	loginSID, _, err := svc.LoginBegin(ctx, "admin")
	if err != nil {
		t.Fatalf("LoginBegin: %v", err)
	}

	// Look up the seeded credential's actual ID bytes so we
	// can submit them with the invalid login body. The library
	// would normally pull this out of the parsed assertion
	// response; our invalid body is malformed so we supply
	// the raw bytes explicitly.
	var storedID []byte
	if err := db.QueryRow(`SELECT credential_id FROM webauthn_credentials WHERE id = ?`, seeded.ID).Scan(&storedID); err != nil {
		t.Fatalf("read stored credential_id: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/webauthn/login/finish",
		invalidJSONReader())
	_, _, err = svc.LoginFinish(ctx, LoginFinishParams{
		SessionID:    loginSID,
		CredentialID: storedID,
	}, req)
	if err == nil {
		t.Fatal("LoginFinish accepted invalid JSON — #44 regression: signature verify is being skipped")
	}

	// sign_count must not have been bumped.
	var sc uint32
	if err := db.QueryRow(`SELECT sign_count FROM webauthn_credentials WHERE id = ?`, seeded.ID).Scan(&sc); err != nil {
		t.Fatalf("read sign_count: %v", err)
	}
	if sc != 0 {
		t.Errorf("sign_count = %d after rejected LoginFinish, want 0 (no-bump on auth failure)", sc)
	}
}

// invalidJSONReader returns a body that is intentionally not
// a valid CredentialCreationResponse / CredentialAssertionResponse
// JSON. Using a Reader (not a string) so the test exercises the
// library's actual JSON-decoder path, not a happy-path shortcut.
func invalidJSONReader() *malformedBody { return &malformedBody{} }

type malformedBody struct{ read bool }

func (m *malformedBody) Read(p []byte) (int, error) {
	if m.read {
		return 0, io.EOF
	}
	m.read = true
	const s = `{"this is not a valid CredentialCreationResponse"`
	return copy(p, s), nil
}
