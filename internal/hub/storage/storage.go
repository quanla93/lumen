// Package storage owns the hub's SQLite persistence: opens the database
// with HDD-friendly pragmas (WAL + synchronous=NORMAL), runs embedded
// goose migrations on startup, and exposes typed helpers for the rest of
// the hub to use.
//
// The in-memory store under internal/hub/store still serves the hot path
// (/api/stream reads from it). SQLite is the archive: every accepted
// ingest is written here too so a hub restart doesn't lose history.
package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"net/url"
	"time"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // registers "sqlite" driver (pure Go, no CGO)

	"github.com/lumenhq/lumen/internal/shared/api"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open returns a *sql.DB connected to the SQLite file at path. Migrations
// in migrations/*.sql run on every Open; the database is safe to use as
// soon as Open returns nil.
//
// Pragmas applied via the DSN:
//   - journal_mode=WAL          — concurrent reads + writes, batched flush
//   - synchronous=NORMAL        — don't fsync after every commit (HDD friendly)
//   - busy_timeout=5000         — retry up to 5s on writer contention
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=%s&_pragma=%s&_pragma=%s",
		url.QueryEscape(path),
		url.QueryEscape("journal_mode(WAL)"),
		url.QueryEscape("synchronous(NORMAL)"),
		url.QueryEscape("busy_timeout(5000)"),
	)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("db.Ping: %w", err)
	}

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("goose.SetDialect: %w", err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("goose.Up: %w", err)
	}
	return db, nil
}

// InsertSnapshot appends one row to the snapshots table. Returns the new
// row id so callers can log it for traceability.
//
// Timestamps are stored as RFC3339 with nanos (e.g. "2026-05-26T06:18:00.123Z")
// so SQLite's date functions (strftime, etc.) work against them — passing
// a raw time.Time leaves the modernc.org/sqlite driver to render it via
// time.Time.String() which uses a format SQLite can't parse.
func InsertSnapshot(ctx context.Context, db *sql.DB, snap api.HostSnapshot) (int64, error) {
	res, err := db.ExecContext(ctx, `
		INSERT INTO snapshots
			(host, ts, cpu_pct, ram_pct, swap_pct, disk_pct,
			 load1, load5, load15,
			 net_rx_bps, net_tx_bps, disk_r_bps, disk_w_bps, temp_c)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		snap.Host, formatTS(snap.Ts),
		snap.CpuPct, snap.RamPct, snap.SwapPct, snap.DiskPct,
		snap.Load1, snap.Load5, snap.Load15,
		snap.NetRxBps, snap.NetTxBps, snap.DiskRBps, snap.DiskWBps, snap.TempC,
	)
	if err != nil {
		return 0, fmt.Errorf("insert snapshot: %w", err)
	}
	return res.LastInsertId()
}

// formatTS renders a time.Time the same way every storage write/query does
// it, so string comparisons in WHERE clauses stay correct and SQLite's
// date functions can parse the value.
func formatTS(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// CountSnapshots is a small diagnostic for smoke tests / docs.
func CountSnapshots(ctx context.Context, db *sql.DB) (int64, error) {
	var n int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM snapshots`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
