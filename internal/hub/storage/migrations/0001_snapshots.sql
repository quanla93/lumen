-- +goose Up
CREATE TABLE snapshots (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  host      TEXT     NOT NULL,
  ts        DATETIME NOT NULL,
  cpu_pct   REAL     NOT NULL DEFAULT 0,
  ram_pct   REAL     NOT NULL DEFAULT 0,
  swap_pct  REAL     NOT NULL DEFAULT 0,
  disk_pct  REAL     NOT NULL DEFAULT 0,
  load1     REAL     NOT NULL DEFAULT 0,
  load5     REAL     NOT NULL DEFAULT 0,
  load15    REAL     NOT NULL DEFAULT 0
);

CREATE INDEX idx_snapshots_host_ts ON snapshots(host, ts);

-- +goose Down
DROP INDEX IF EXISTS idx_snapshots_host_ts;
DROP TABLE IF EXISTS snapshots;
