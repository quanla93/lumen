package retention

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/quanla93/lumen/internal/hub/settings"
	"github.com/quanla93/lumen/internal/hub/storage"
)

// TestSweep_PrunesAlertsAndDeliveries exercises the v0.4.1 path: a single
// sweep call should reap resolved alerts + terminal deliveries older than
// the configured window. Snapshots are intentionally left alone here so a
// regression in the alerts path doesn't get masked by a healthy snapshot
// sweep.
func TestSweep_PrunesAlertsAndDeliveries(t *testing.T) {
	db, cfg := setupRetentionTest(t)
	defer db.Close()
	ctx := context.Background()

	now := time.Now().UTC()
	old := formatTS(now.Add(-60 * 24 * time.Hour))

	// resolved 60d ago → should be deleted (alerts window is 30d).
	if _, err := db.ExecContext(ctx, `
		INSERT INTO alert_events
			(rule_id, rule_name, host, metric, severity, state, value,
			 message, started_at, resolved_at)
		VALUES (1, 'r', 'h', 'cpu_pct', 'warning', 'resolved', 0, 'm', ?, ?)`,
		old, old); err != nil {
		t.Fatalf("seed alert: %v", err)
	}
	// firing 60d ago → must NOT be deleted.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO alert_events
			(rule_id, rule_name, host, metric, severity, state, value,
			 message, started_at)
		VALUES (2, 'r2', 'h', 'cpu_pct', 'critical', 'firing', 0, 'm', ?)`,
		old); err != nil {
		t.Fatalf("seed firing alert: %v", err)
	}
	// terminal delivery 60d ago → should be deleted.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO notification_deliveries
			(event_id, channel_id, channel_name, channel_type, severity,
			 status, payload, created_at, sent_at)
		VALUES (2, 1, 'c', 'ntfy', 'warning', 'sent', '{}', ?, ?)`,
		old, old); err != nil {
		t.Fatalf("seed delivery: %v", err)
	}
	// pending delivery 60d ago → must NOT be deleted (dispatcher still owns it).
	if _, err := db.ExecContext(ctx, `
		INSERT INTO notification_deliveries
			(event_id, channel_id, channel_name, channel_type, severity,
			 status, payload, created_at)
		VALUES (2, 1, 'c', 'ntfy', 'warning', 'pending', '{}', ?)`,
		old); err != nil {
		t.Fatalf("seed pending delivery: %v", err)
	}

	sweep(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if got := countRows(t, db, "alert_events"); got != 1 {
		t.Fatalf("alert_events = %d, want 1 (firing kept)", got)
	}
	if got := countRows(t, db, "notification_deliveries"); got != 1 {
		t.Fatalf("notification_deliveries = %d, want 1 (pending kept)", got)
	}
}

// TestSweep_AlertsWindowDisabled verifies a zero window short-circuits the
// alerts sweep without touching the snapshot sweep.
func TestSweep_AlertsWindowDisabled(t *testing.T) {
	db, cfg := setupRetentionTest(t)
	defer db.Close()
	cfg.DefaultAlertsWindow = 0
	// Wipe the seeded setting so the zero default takes effect.
	if err := settings.Set(context.Background(), db, settings.KeyRetentionAlertsWindow, "0s"); err != nil {
		t.Fatalf("set alerts window: %v", err)
	}

	old := formatTS(time.Now().UTC().Add(-90 * 24 * time.Hour))
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO alert_events
			(rule_id, rule_name, host, metric, severity, state, value,
			 message, started_at, resolved_at)
		VALUES (1, 'r', 'h', 'cpu_pct', 'warning', 'resolved', 0, 'm', ?, ?)`,
		old, old); err != nil {
		t.Fatalf("seed alert: %v", err)
	}

	sweep(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if got := countRows(t, db, "alert_events"); got != 1 {
		t.Fatalf("alert_events = %d, want 1 (sweep disabled)", got)
	}
}

func setupRetentionTest(t *testing.T) (*sql.DB, Config) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "lumen.db"))
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	if err := settings.EnsureDefaults(context.Background(), db, map[string]string{
		settings.KeyRetentionWindow:       "24h",
		settings.KeyRetentionInterval:     "1h",
		settings.KeyRetentionAlertsWindow: "720h",
	}); err != nil {
		t.Fatalf("ensure defaults: %v", err)
	}
	return db, Config{
		DB:                  db,
		DefaultWindow:       24 * time.Hour,
		DefaultInterval:     time.Hour,
		DefaultAlertsWindow: 30 * 24 * time.Hour,
		Logger:              slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	// table name is hard-coded in callers — safe to interpolate.
	if err := db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// formatTS mirrors storage.formatTS (which is unexported) so tests can
// stamp deterministic timestamps without going through InsertSnapshot.
func formatTS(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
