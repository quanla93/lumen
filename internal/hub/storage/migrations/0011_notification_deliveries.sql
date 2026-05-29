-- +goose Up
-- Milestone D: persisted notification delivery queue.
--
-- Why a queue instead of fanning out HTTP calls inline on every engine tick:
--   * 100 transitions × 3 channels = 300 sequential HTTP calls; at 1s/call
--     the engine ticker is blocked for 5 minutes. Tick cadence collapses.
--   * Discord webhooks have a per-URL rate limit; bursts get 429s with
--     no retry path → events lost.
--   * Hub crash mid-burst → in-flight notifications die. We want at-least-
--     once delivery for `failed` rows so operators can replay.
--
-- The dispatcher polls this table every ~1s, picks pending rows whose
-- next_retry_at has passed, and runs them through the HTTP dispatcher
-- with per-channel serialisation (so we don't 429 ourselves). Failed
-- rows back off exponentially up to a max attempt count.
--
-- channel_name + channel_type are denormalised so a delivery row stays
-- readable in the UI after the operator deletes / renames the channel.
-- payload is the JSON the engine generated for that event; surfacing it
-- in the UI makes debugging "did the right thing get sent" trivial.
CREATE TABLE notification_deliveries (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  event_id      INTEGER NOT NULL REFERENCES alert_events(id) ON DELETE CASCADE,
  channel_id    INTEGER NOT NULL,                              -- not FK: channel may be deleted
  channel_name  TEXT NOT NULL,                                 -- denormalised
  channel_type  TEXT NOT NULL,                                 -- denormalised
  severity      TEXT NOT NULL DEFAULT 'info',                  -- drives retry schedule (critical fails fast)
  status        TEXT NOT NULL DEFAULT 'pending',               -- pending|inflight|sent|failed|dropped
  attempts      INTEGER NOT NULL DEFAULT 0,
  http_status   INTEGER,
  error         TEXT,
  next_retry_at DATETIME,                                      -- NULL → eligible immediately
  payload       TEXT NOT NULL,                                 -- JSON of the Notification
  created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  sent_at       DATETIME
);
CREATE INDEX idx_deliveries_status_retry ON notification_deliveries(status, next_retry_at);
CREATE INDEX idx_deliveries_event ON notification_deliveries(event_id);
CREATE INDEX idx_deliveries_created ON notification_deliveries(created_at);
CREATE INDEX idx_deliveries_severity_status ON notification_deliveries(severity, status);

-- +goose Down
DROP INDEX IF EXISTS idx_deliveries_created;
DROP INDEX IF EXISTS idx_deliveries_event;
DROP INDEX IF EXISTS idx_deliveries_status_retry;
DROP TABLE IF EXISTS notification_deliveries;
