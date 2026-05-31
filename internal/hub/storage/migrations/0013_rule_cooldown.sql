-- +goose Up
-- v0.4.5: per-rule flap suppression cooldown.
--
-- cooldown_seconds is the minimum gap between two firing notifications
-- for the same (rule, host) pair. Default 0 preserves pre-cooldown
-- behavior (every firing transition emits an event + notification).
--
-- Suppressed firings still flip the engine's in-memory state to firing
-- (so the next resolve transition fires normally) but skip both the
-- alert_events insert and the delivery queue insert — they're invisible
-- to history by design, otherwise a flapping rule would still pollute
-- the table the way it pollutes the channel.
ALTER TABLE alert_rules ADD COLUMN cooldown_seconds INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE alert_rules DROP COLUMN cooldown_seconds;
