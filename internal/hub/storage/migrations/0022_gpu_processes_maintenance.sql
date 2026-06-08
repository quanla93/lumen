-- +goose Up
-- Beszel bundle 1 (RFC 0003) — maintenance windows table + process
-- list settings seeds. GPU + per-process data are wire-only (no DB
-- rows): the host detail page reads them from the in-memory store
-- and the alerts engine extracts worst-of values per tick.
--
-- Maintenance windows: a time range with a tag-scope selector. The
-- alerts engine reads cached rows and skips notify+event-insert for
-- any rule whose host matches the scope while now ∈ [start_at, end_at].
-- One-shot only in v1 (no recurrence); edit guards disallow start_at
-- changes after the window has begun.

CREATE TABLE maintenance_windows (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    start_at   DATETIME NOT NULL,
    end_at     DATETIME NOT NULL,
    reason     TEXT     NOT NULL DEFAULT '',
    -- scope_tags is a JSON object {tag_key: tag_value}. Stored as TEXT
    -- to avoid a join table — the alerts engine already loads + parses
    -- a JSON tag set per rule. Empty object = "matches all hosts".
    scope_tags TEXT     NOT NULL DEFAULT '{}',
    created_by INTEGER,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX idx_mw_active ON maintenance_windows(start_at, end_at);
CREATE INDEX idx_mw_created_at ON maintenance_windows(created_at);

-- Process list defaults. processes.enabled is OFF by default — the
-- opt-in is per-deployment and the cmdline field can leak secrets
-- (RFC 0003 §"Process list").
INSERT OR IGNORE INTO settings (key, value) VALUES
    ('processes.enabled',         'false'),
    ('processes.top_n',           '10'),
    ('processes.sort_by',         'cpu'),
    ('processes.redact_regex',    '');

-- +goose Down
DROP TABLE IF EXISTS maintenance_windows;
DELETE FROM settings WHERE key LIKE 'processes.%';
