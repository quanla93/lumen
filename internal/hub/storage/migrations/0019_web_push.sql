-- +goose Up
-- Web Push delivery — a notification_channels row with type='web_push'
-- has no URL of its own; instead it points at zero or more browser
-- subscriptions that signed up via /api/alerts/web-push/subscribe. One
-- channel can fan out to multiple browsers (admin's laptop + phone).
--
-- Subscription fields follow the W3C Push API spec:
--   endpoint  — the push service URL the browser provided
--   p256dh    — Base64-URL ECDH public key for payload encryption
--   auth      — Base64-URL 16-byte client auth secret
--   label     — short human-readable name (typically User-Agent excerpt)
--               so the Settings UI can show "Chrome on MacBook" rather
--               than the opaque endpoint URL.
CREATE TABLE web_push_subscriptions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id INTEGER NOT NULL,
    endpoint   TEXT    NOT NULL,
    p256dh     TEXT    NOT NULL,
    auth       TEXT    NOT NULL,
    label      TEXT    NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (channel_id) REFERENCES notification_channels(id) ON DELETE CASCADE,
    UNIQUE (channel_id, endpoint)
);

CREATE INDEX idx_web_push_subscriptions_channel ON web_push_subscriptions(channel_id);

-- +goose Down
DROP INDEX IF EXISTS idx_web_push_subscriptions_channel;
DROP TABLE IF EXISTS web_push_subscriptions;
