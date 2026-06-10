-- +goose Up
-- Sprint 4 / RFC 0004 §"Digest": per-channel digest_window buffering.
--
-- The dispatcher (internal/hub/alerts/dispatcher.go) reads three new
-- columns:
--
--   * digest_window — the channel's digest_window string at enqueue
--     time. Empty / "0" = single-shot (today's behaviour, no
--     buffering). The dispatcher copies it here at INSERT so a later
--     operator edit doesn't change the gate on already-buffered rows.
--
--   * next_flush_at — when the buffer expires and the dispatcher may
--     claim the row. NULL when digest_window is empty.
--
--   * rows_count — how many rows are currently buffered in the same
--     window for the same channel. The dispatcher checks
--     rows_count >= 10 → early-flush so a burst doesn't sit silent
--     for the full window. The Enqueue path updates this column for
--     every buffered row when a new row arrives, so a SELECT in
--     claimNext() can use it as a filter without a per-row join.

ALTER TABLE notification_deliveries ADD COLUMN digest_window TEXT NOT NULL DEFAULT '';
ALTER TABLE notification_deliveries ADD COLUMN next_flush_at DATETIME;
ALTER TABLE notification_deliveries ADD COLUMN rows_count INTEGER NOT NULL DEFAULT 0;

-- Index makes the claimNext() filter scan small. Per-channel rows
-- tend to cluster in time so a window-keyed composite index is
-- enough — no need to add severity / retry to the index.
CREATE INDEX idx_deliveries_digest ON notification_deliveries(digest_window, next_flush_at);

-- +goose Down
DROP INDEX IF EXISTS idx_deliveries_digest;
ALTER TABLE notification_deliveries DROP COLUMN rows_count;
ALTER TABLE notification_deliveries DROP COLUMN next_flush_at;
ALTER TABLE notification_deliveries DROP COLUMN digest_window;
