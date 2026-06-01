-- +goose Up
-- user_prefs backs RFC 0002 PR2 Level 3 personalization. Per-user
-- key/value blobs stored as JSON strings; no SQLite JSON1 dependency
-- since the reader writes the whole blob each call. Last-writer-wins
-- is fine in practice — one tab edits a key at a time.
--
-- Known keys (others tolerated by the reader, ignored on the wire):
--   'dashboard_prefs' — sort, default metric, hidden hosts, saved views
--   'display_prefs'   — theme, language, units, reduceMotion, density
--
-- Both blobs include schemaVersion=1; future shape changes bump that
-- and the reader migrates client-side. Server stays schema-agnostic
-- beyond the validation guards in internal/hub/userprefs.
CREATE TABLE user_prefs (
    user_id    INTEGER NOT NULL,
    key        TEXT    NOT NULL,
    json_value TEXT    NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, key),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE IF EXISTS user_prefs;
