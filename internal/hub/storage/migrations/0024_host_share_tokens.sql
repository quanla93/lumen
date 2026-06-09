-- +goose Up
-- Sprint 4 / RFC 0004 §"Per-host share link".
--
-- A share link mints a 32-byte random token (base64url, no padding)
-- bound to one host with a TTL. The plaintext is the bearer — we
-- don't hash because the table is already time-bounded (1h..720h)
-- + revocable, and the public endpoint takes the URL as-is.
--
-- expires_at is the gating column: a row past its expires_at must
-- not resolve. The fetch path (internal/hub/hosts/share.go) does
-- the time check; a background sweeper (SweepExpiredShares) deletes
-- rows once per hour to keep the table small.

CREATE TABLE host_share_tokens (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    token      TEXT NOT NULL UNIQUE,
    host_id    INTEGER NOT NULL,
    expires_at DATETIME NOT NULL,
    label      TEXT NOT NULL DEFAULT '',
    created_by INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (host_id) REFERENCES hosts(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

-- Per-row index on expires_at so the hourly sweep can prune in one
-- pass without scanning the whole table. The lookup by token is
-- PK-driven so doesn't need an index.
CREATE INDEX idx_share_token_expires ON host_share_tokens(expires_at);
CREATE INDEX idx_share_token_host ON host_share_tokens(host_id);

-- +goose Down
DROP INDEX IF EXISTS idx_share_token_host;
DROP INDEX IF EXISTS idx_share_token_expires;
DROP TABLE IF EXISTS host_share_tokens;
