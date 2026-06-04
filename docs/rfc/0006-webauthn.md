# RFC 0006 — WebAuthn / passkey login

- **Status**: Draft
- **Sprint**: Phase 8 Sprint 6
- **Effort**: 4 days

## Motivation

Passkeys (synced via iCloud Keychain, Google Password Manager, Bitwarden, 1Password) are the modern replacement for password-only login. Phishing-resistant, hardware-backed on most devices, no shared secret to leak. Lumen's single-admin account is the ideal place to require one.

OIDC SSO already covers the "delegate auth" case. WebAuthn covers operators who don't want to run an IdP but still want hardware-backed login.

## Scope

**In**: Register a passkey from Settings → Account. Login with passkey from the login page. Multiple passkeys per user (yubikey + phone, etc.). Per-credential label + last-used timestamp. Revoke.

**Out**: WebAuthn-only mode (password fallback always available; otherwise a single broken passkey locks the operator out). Cross-device passkey sync attestation. Discoverable credentials (`residentKey` always `preferred` — works without requiring it).

## Design

### Library

`github.com/go-webauthn/webauthn` (~500 KB, MIT, maintained).

### Migration

`migrations/0023_webauthn.sql`:

```sql
CREATE TABLE webauthn_credentials (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id             INTEGER NOT NULL,
    credential_id       BLOB    NOT NULL UNIQUE,
    public_key          BLOB    NOT NULL,
    attestation_type    TEXT    NOT NULL DEFAULT '',
    transports          TEXT    NOT NULL DEFAULT '[]',  -- JSON
    sign_count          INTEGER NOT NULL DEFAULT 0,
    label               TEXT    NOT NULL DEFAULT '',
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at        DATETIME,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_webauthn_user ON webauthn_credentials(user_id);
```

### Endpoints

| Verb + path | Auth | Purpose |
|---|---|---|
| `POST /api/auth/webauthn/register/begin` | session | Returns the WebAuthn `PublicKeyCredentialCreationOptions`. |
| `POST /api/auth/webauthn/register/finish` | session | Receives the attestation, validates, persists. Body includes the operator's `label`. |
| `POST /api/auth/webauthn/login/begin` | none | Returns `PublicKeyCredentialRequestOptions`. Username optional — if provided, narrows the allow-list. |
| `POST /api/auth/webauthn/login/finish` | none | Receives the assertion, validates, mints `lumen_session`. |
| `GET /api/me/webauthn` | session | Lists current credentials (no public-key bytes, just metadata). |
| `DELETE /api/me/webauthn/{id}` | session | Revokes a credential. Refuses if it would leave the user with zero passkeys AND password-less. |

### Challenge storage

Server-side ephemeral state per registration / login attempt: stored in an in-memory map keyed by a random 32-byte session ID, set as a short-lived HttpOnly cookie (`lumen_webauthn_chal`). 5-min TTL. Cleared on success / fail.

### Frontend

`web/src/components/Settings.tsx` → Account section gains a "Passkeys" subsection: list current + "Add passkey" button.

`web/src/components/LoginForm.tsx`: "Sign in with passkey" button alongside password + SSO buttons. Calls login/begin, runs `navigator.credentials.get()`, posts the result to login/finish.

WebAuthn JSON-encoded payloads need byte-array conversions (base64url ↔ Uint8Array); add helpers in `web/src/lib/webauthn.ts`.

### Origin / RP ID

`rpID = hostname(window.location.href)` (no port). `rpName = "Lumen"`. Document the gotcha: WebAuthn requires HTTPS in production. localhost is exempt.

## Risks

| Risk | Mitigation |
|---|---|
| Misconfigured reverse proxy strips the `Origin` header | Document the requirement; surface a clear error from `webauthn.Verify`. |
| Operator registers a passkey, deletes it, can't log in | Allow delete only if password is set OR ≥ 1 other passkey remains. Server enforces. |
| Sync attestation = privacy concern | Default `Attestation = none`; document. |
| Counter regression on cloned credentials | go-webauthn checks `sign_count > stored`; failure surfaces as "credential clone detected". Document. |
| Browser without WebAuthn (very old) | Detect on the client; hide the button with a tooltip. |

## Testing

- Fixture-based: known credential JSON + assertion JSON → verify path.
- Sign-count regression: persist sign_count=5; submit assertion with sign_count=3 → reject.
- Multi-credential allow-list: present 2 credentials, browser-equivalent test picks the right one.
- Delete-last-passkey guard.

## Docs deliverables

- `docs/configure/passkeys.md` — what works on what (Touch ID, Windows Hello, security keys, Android, iOS). HTTPS gotcha.
- CHANGELOG + ACTION_PLAN tick.

## Open questions

1. Should we offer a "passkeys only" mode where the password is disabled after the first passkey is registered? Proposed: as a per-user toggle in Settings → Account → "Disable password login". Requires confirming with a second passkey before flipping.
2. Should login/begin return the username allow-list publicly? Proposed: no — accept any credential, look it up server-side; avoids username enumeration.
3. Audit trail for passkey events (register / revoke / login)? Proposed: yes, append to a hub-events ring buffer surfaced under Settings → Account → Activity in a follow-up.
