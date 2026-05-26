-- +goose Up
-- Settings: simple key/value pairs the operator can change at runtime
-- via the Settings UI. Env vars (LUMEN_HUB_*) seed defaults on first
-- read; once a row exists here it wins over the env value.
CREATE TABLE settings (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +goose Down
DROP TABLE IF EXISTS settings;
