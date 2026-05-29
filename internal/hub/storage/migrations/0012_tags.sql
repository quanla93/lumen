-- +goose Up
-- Tags become a first-class resource: operators manage an inventory of
-- (key, allowed_values) up front, hosts and alert rules pick from it.
--
-- Why two tables instead of stuffing values into a JSON column on `tags`:
--   * tag_values is the lookup index for the application-layer FK check
--     in hosts.SetTags. SELECT 1 FROM tag_values WHERE tag_key=? AND
--     value=? is the hot path on every host edit, and it stays an index
--     lookup instead of a JSON scan.
--   * Cascading a single value deletion (DELETE … WHERE tag_key=? AND
--     value=?) maps to a normal SQL operation. JSON-blob storage would
--     force a read-modify-write per row.
--
-- We don't add an FK from host_tags → tag_values. Reasons:
--   * Backfill below only seeds tag_values with whatever's already
--     referenced. An FK would still be satisfied right after migration,
--     but anything that lands in host_tags via a future code path before
--     the inventory check is the kind of bug worth surfacing in tests
--     rather than via FK rejection at insert time.
--   * Application-layer check in hosts.SetTags returns a clean
--     ErrTagNotInInventory; FK rejection is opaque ("constraint failed").
CREATE TABLE tags (
  key         TEXT PRIMARY KEY,
  description TEXT NOT NULL DEFAULT '',
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE tag_values (
  tag_key    TEXT NOT NULL REFERENCES tags(key) ON DELETE CASCADE ON UPDATE CASCADE,
  value      TEXT NOT NULL,
  created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tag_key, value)
);
CREATE INDEX idx_tag_values_value ON tag_values(value);

-- Backfill the inventory from whatever's already in use. Empty value is
-- a real, distinct entry — pre-existing "bare key" tags continue to work.
INSERT OR IGNORE INTO tags (key) SELECT DISTINCT key FROM host_tags;
INSERT OR IGNORE INTO tag_values (tag_key, value)
  SELECT DISTINCT key, value FROM host_tags;

-- +goose Down
DROP INDEX IF EXISTS idx_tag_values_value;
DROP TABLE IF EXISTS tag_values;
DROP TABLE IF EXISTS tags;
