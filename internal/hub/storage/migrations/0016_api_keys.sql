-- +goose Up
-- api_keys backs the Public Read API. Bearer keys are minted by an
-- admin via /api/apikeys, stored as a SHA-256 hex hash (same scheme as
-- host tokens — server-generated 32 random bytes, so Argon2id would
-- only burn CPU on every authenticated request without adding entropy
-- a brute-force search can't already do).
--
-- preview is the first ~12 chars of the plaintext key, kept so the UI
-- can show "lumk_AbCdEf…" in the list view without revealing the secret
-- a second time (plaintext is shown exactly once at creation).
--
-- scopes is a JSON array of strings ('read:hosts', 'read:metrics',
-- 'read:alerts'). host_filter is an optional glob ('*pve*') that
-- restricts which hosts the key can see; NULL = all hosts (subject to
-- scope checks). Tag-pair selectors are deferred to v0.5.x.
CREATE TABLE api_keys (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    hash         TEXT NOT NULL UNIQUE,
    preview      TEXT NOT NULL,
    scopes       TEXT NOT NULL,
    host_filter  TEXT,
    last_used_at INTEGER,
    created_at   INTEGER NOT NULL DEFAULT (strftime('%s','now'))
);

-- Lookup hot path: incoming Bearer token gets SHA-256'd, then we
-- SELECT by hash. UNIQUE already creates an index but spelling it
-- out makes intent explicit.
CREATE INDEX idx_api_keys_hash ON api_keys(hash);

-- +goose Down
DROP INDEX IF EXISTS idx_api_keys_hash;
DROP TABLE IF EXISTS api_keys;
