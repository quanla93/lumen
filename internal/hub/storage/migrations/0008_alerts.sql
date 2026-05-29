-- +goose Up
-- Phase 6 / RFC 0001: threshold alerting + notification dispatch.
--
-- alert_rules: operator-defined thresholds. NULL host = match all hosts.
-- The `offline` metric ignores comparator/threshold and fires when a host
-- stops appearing in the in-memory store (last-seen > for_seconds, min 60s).
--
-- notification_channels: pluggable HTTP destinations. owner_type is
-- forward-compatible with the Public API customer-webhook reuse (Decisions
-- log 2026-05-29) — today only 'admin' rows are created via the UI.
--
-- alert_events: persisted firing/resolved history. rule_name is denormalized
-- so deleting a rule leaves a usable audit trail.
CREATE TABLE alert_rules (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL,
  metric      TEXT NOT NULL,                       -- cpu_pct|ram_pct|swap_pct|disk_pct|load1|offline
  comparator  TEXT NOT NULL DEFAULT 'gt',          -- gt|lt (ignored for 'offline')
  threshold   REAL NOT NULL DEFAULT 0,
  for_seconds INTEGER NOT NULL DEFAULT 0,          -- breach must persist this long before firing
  host        TEXT,                                -- NULL = all hosts
  severity    TEXT NOT NULL DEFAULT 'warning',     -- info|warning|critical
  enabled     INTEGER NOT NULL DEFAULT 1,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE notification_channels (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL,
  type        TEXT NOT NULL,                        -- ntfy|discord|webhook
  config      TEXT NOT NULL,                        -- JSON: {"url":...,"topic":...,"priority":...}
  owner_type  TEXT NOT NULL DEFAULT 'admin',        -- fwd-compat: 'admin' | (future) 'api_key'
  enabled     INTEGER NOT NULL DEFAULT 1,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE alert_events (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  rule_id     INTEGER NOT NULL,
  rule_name   TEXT NOT NULL,
  host        TEXT NOT NULL,
  metric      TEXT NOT NULL,
  severity    TEXT NOT NULL,
  state       TEXT NOT NULL,                        -- firing|resolved
  value       REAL,
  message     TEXT NOT NULL,
  started_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  resolved_at DATETIME
);
CREATE INDEX idx_alert_events_state ON alert_events(state, started_at);

-- +goose Down
DROP INDEX IF EXISTS idx_alert_events_state;
DROP TABLE IF EXISTS alert_events;
DROP TABLE IF EXISTS notification_channels;
DROP TABLE IF EXISTS alert_rules;
