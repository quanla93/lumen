-- +goose Up
-- Sprint 6 / RFC 0006 §"Per-host share link" — actually WebAuthn
-- (was mislabeled in the RFC draft; corrected here).
--
-- The webauthn_credentials table holds the per-credential
-- attestation blob + public key the agent uses to verify
-- assertions. Multiple credentials per user (yubikey + phone,
-- etc.); the UNIQUE(credential_id) constraint dedupes any
-- re-registration races.
--
-- credential_id is the raw WebAuthn credential ID (opaque bytes
-- from the authenticator). We store it verbatim — the client
-- echoes it back during login so we can look up the row.
--
-- public_key is the COSE-encoded public key bytes per
-- RFC 8152 §7. Stored as BLOB so it round-trips through
-- WebAuthn library's (*Credential).PublicKey without encoding
-- shenanigans.
--
-- transports is a JSON array (e.g. ["usb","nfc","ble"]) per
-- WebAuthn §5.1.4. Empty = unknown. Used for the registration
-- options' hints but never as an authz decision.
--
-- sign_count tracks the authenticator's monotonic counter.
-- go-webauthn rejects assertions whose counter is <= the
-- stored value (a counter regression means a clone). We start
-- at 0 and trust whatever the library writes.
--
-- last_used_at is NULL on register, set on each successful
-- FinishLogin. Surfaced in Settings → Account → Passkeys so
-- the operator can prune stale credentials.

CREATE TABLE webauthn_credentials (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id          INTEGER NOT NULL,
    credential_id    BLOB    NOT NULL UNIQUE,
    public_key       BLOB    NOT NULL,
    attestation_type TEXT     NOT NULL DEFAULT 'none',
    transports       TEXT     NOT NULL DEFAULT '[]',
    sign_count       INTEGER  NOT NULL DEFAULT 0,
    aaguid           BLOB,
    label            TEXT     NOT NULL DEFAULT '',
    backup_eligible  INTEGER  NOT NULL DEFAULT 0,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at     DATETIME,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Per-user lookup is the hot path during FinishLogin (one row
-- per credential, in priority order). user_id-only index is
-- enough — the library filters by credential_id client-side.
CREATE INDEX idx_webauthn_user ON webauthn_credentials(user_id);

-- +goose Down
DROP INDEX IF EXISTS idx_webauthn_user;
DROP TABLE IF EXISTS webauthn_credentials;
