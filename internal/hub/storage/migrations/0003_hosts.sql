-- +goose Up
CREATE TABLE hosts (
  id            INTEGER  PRIMARY KEY AUTOINCREMENT,
  name          TEXT     NOT NULL UNIQUE COLLATE NOCASE,
  token_hash    TEXT     NOT NULL UNIQUE,
  created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_seen_at  DATETIME
);

CREATE INDEX idx_hosts_token_hash ON hosts(token_hash);

-- +goose Down
DROP INDEX IF EXISTS idx_hosts_token_hash;
DROP TABLE IF EXISTS hosts;
