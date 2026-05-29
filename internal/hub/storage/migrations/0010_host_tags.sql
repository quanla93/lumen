-- +goose Up
-- Milestone C: host tags + label selectors on alert rules.
--
-- Tags are key=value labels in the Kubernetes/Prometheus tradition. Each
-- host may have many tags but at most one value per key (the (host_id,
-- key) PK enforces this — re-tagging a host with the same key replaces
-- the value, which is the expected operator mental model).
--
-- alert_rules gets a host_selector text column. When non-empty it wins
-- over the existing host field (name/glob). Format: comma-separated
-- key=value pairs, AND between pairs. Empty values are allowed
-- ("tier=" matches hosts where the 'tier' key exists with empty value;
-- bare keys are sugar for the same).
ALTER TABLE alert_rules ADD COLUMN host_selector TEXT NOT NULL DEFAULT '';

CREATE TABLE host_tags (
  host_id INTEGER NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (host_id, key)
);
CREATE INDEX idx_host_tags_kv ON host_tags(key, value);

-- +goose Down
DROP INDEX IF EXISTS idx_host_tags_kv;
DROP TABLE IF EXISTS host_tags;
ALTER TABLE alert_rules DROP COLUMN host_selector;
