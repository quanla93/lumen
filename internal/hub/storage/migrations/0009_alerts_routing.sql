-- +goose Up
-- Phase 6 Milestone B: per-rule channel routing + per-channel severity filter.
--
-- Routing model: rules pick which channels they fan out to via a M:N
-- join. An empty link set for a rule keeps the Milestone-A behaviour
-- (broadcast to every enabled channel) so existing rules don't go silent
-- after upgrade.
--
-- min_severity caps the noise on a channel — useful when one channel is
-- a high-volume Slack room (info+) and another is a pager (critical
-- only). Default 'info' = no filter.
ALTER TABLE notification_channels
  ADD COLUMN min_severity TEXT NOT NULL DEFAULT 'info';

CREATE TABLE alert_rule_channels (
  rule_id    INTEGER NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
  channel_id INTEGER NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
  PRIMARY KEY (rule_id, channel_id)
);
CREATE INDEX idx_alert_rule_channels_channel ON alert_rule_channels(channel_id);

-- +goose Down
DROP INDEX IF EXISTS idx_alert_rule_channels_channel;
DROP TABLE IF EXISTS alert_rule_channels;
ALTER TABLE notification_channels DROP COLUMN min_severity;
