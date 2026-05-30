package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// These tests cover the v0.4.1 alert + delivery retention helpers. They
// use the real migration set via storage.Open so the CASCADE behaviour
// between alert_events and notification_deliveries gets exercised end-
// to-end.

func TestDeleteResolvedAlertsBefore_KeepsFiring(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	old := time.Now().Add(-90 * 24 * time.Hour).UTC()
	insertAlertEvent(t, db, "firing", old, time.Time{})
	insertAlertEvent(t, db, "resolved", old, old.Add(1*time.Hour))

	n, err := DeleteResolvedAlertsBefore(ctx, db, time.Now().Add(-30*24*time.Hour).UTC())
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n != 1 {
		t.Fatalf("deleted = %d, want 1", n)
	}

	got := countAlertEvents(t, db)
	if got != 1 {
		t.Fatalf("remaining alert_events = %d, want 1 (firing kept)", got)
	}
}

func TestDeleteResolvedAlertsBefore_RespectsCutoff(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	now := time.Now().UTC()
	// Resolved 10 days ago — inside the 30d window, keep.
	insertAlertEvent(t, db, "resolved", now.Add(-40*24*time.Hour), now.Add(-10*24*time.Hour))
	// Resolved 60 days ago — outside the 30d window, drop.
	insertAlertEvent(t, db, "resolved", now.Add(-90*24*time.Hour), now.Add(-60*24*time.Hour))

	cutoff := now.Add(-30 * 24 * time.Hour)
	n, err := DeleteResolvedAlertsBefore(ctx, db, cutoff)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n != 1 {
		t.Fatalf("deleted = %d, want 1", n)
	}
	if got := countAlertEvents(t, db); got != 1 {
		t.Fatalf("remaining = %d, want 1", got)
	}
}

func TestDeleteResolvedAlertsBefore_FallsBackToStartedAtForGhostRow(t *testing.T) {
	// A pre-fix ghost: state='resolved' but resolved_at NULL. Should age
	// out on started_at so it doesn't get stuck forever.
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	now := time.Now().UTC()
	insertAlertEvent(t, db, "resolved", now.Add(-90*24*time.Hour), time.Time{})

	n, err := DeleteResolvedAlertsBefore(ctx, db, now.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n != 1 {
		t.Fatalf("deleted = %d, want 1", n)
	}
}

func TestDeleteResolvedAlertsBefore_PairsWithDeliveriesSweep(t *testing.T) {
	// Migration 0011 declares ON DELETE CASCADE on event_id, but Open
	// doesn't enable PRAGMA foreign_keys (other latent CASCADEs across
	// the schema would need an audit first). So in practice deleting an
	// alert_event leaves its deliveries as orphans. The retention loop
	// pairs DeleteResolvedAlertsBefore with DeleteTerminalDeliveriesBefore
	// to clean both — verify together they leave the tables empty.
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	now := time.Now().UTC()
	eventID := insertAlertEvent(t, db, "resolved",
		now.Add(-90*24*time.Hour), now.Add(-60*24*time.Hour))
	insertDelivery(t, db, eventID, "sent", now.Add(-60*24*time.Hour))
	insertDelivery(t, db, eventID, "failed", now.Add(-60*24*time.Hour))

	cutoff := now.Add(-30 * 24 * time.Hour)
	if _, err := DeleteResolvedAlertsBefore(ctx, db, cutoff); err != nil {
		t.Fatalf("delete alerts: %v", err)
	}
	if _, err := DeleteTerminalDeliveriesBefore(ctx, db, cutoff); err != nil {
		t.Fatalf("delete deliveries: %v", err)
	}
	if got := countAlertEvents(t, db); got != 0 {
		t.Fatalf("alerts remaining = %d, want 0", got)
	}
	if got := countDeliveries(t, db); got != 0 {
		t.Fatalf("deliveries remaining = %d, want 0", got)
	}
}

func TestDeleteTerminalDeliveriesBefore_KeepsPendingAndInflight(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	now := time.Now().UTC()
	eventID := insertAlertEvent(t, db, "firing", now.Add(-90*24*time.Hour), time.Time{})

	insertDelivery(t, db, eventID, "sent", now.Add(-90*24*time.Hour))
	insertDelivery(t, db, eventID, "failed", now.Add(-90*24*time.Hour))
	insertDelivery(t, db, eventID, "dropped", now.Add(-90*24*time.Hour))
	insertDelivery(t, db, eventID, "pending", now.Add(-90*24*time.Hour))
	insertDelivery(t, db, eventID, "inflight", now.Add(-90*24*time.Hour))

	n, err := DeleteTerminalDeliveriesBefore(ctx, db, now.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n != 3 {
		t.Fatalf("deleted = %d, want 3 (sent/failed/dropped)", n)
	}
	if got := countDeliveries(t, db); got != 2 {
		t.Fatalf("remaining = %d, want 2 (pending+inflight)", got)
	}
}

func TestDeleteTerminalDeliveriesBefore_RespectsCutoff(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()

	now := time.Now().UTC()
	eventID := insertAlertEvent(t, db, "firing", now.Add(-90*24*time.Hour), time.Time{})
	insertDelivery(t, db, eventID, "sent", now.Add(-10*24*time.Hour)) // inside window — keep
	insertDelivery(t, db, eventID, "sent", now.Add(-60*24*time.Hour)) // outside — drop

	n, err := DeleteTerminalDeliveriesBefore(ctx, db, now.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n != 1 {
		t.Fatalf("deleted = %d, want 1", n)
	}
}

// openTestDB applies the full migration set to a fresh sqlite file.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "lumen.db"))
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	return db
}

// insertAlertEvent mimics the engine.go INSERT, with explicit started_at
// + resolved_at so tests can place rows at known ages. resolved.IsZero()
// means leave the column NULL (firing rows, or the ghost-row case).
func insertAlertEvent(t *testing.T, db *sql.DB, state string, started, resolved time.Time) int64 {
	t.Helper()
	var resolvedArg any
	if resolved.IsZero() {
		resolvedArg = nil
	} else {
		resolvedArg = formatTS(resolved)
	}
	res, err := db.ExecContext(context.Background(), `
		INSERT INTO alert_events
			(rule_id, rule_name, host, metric, severity, state, value, message,
			 started_at, resolved_at)
		VALUES (1, 'test', 'h1', 'cpu_pct', 'warning', ?, 0, 'msg', ?, ?)`,
		state, formatTS(started), resolvedArg,
	)
	if err != nil {
		t.Fatalf("insert alert: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last id: %v", err)
	}
	return id
}

// insertDelivery mimics dispatcher.Enqueue but with an explicit created_at
// + sent_at so tests can age rows deterministically. For sent rows we set
// sent_at to the same instant — the helper compares COALESCE(sent_at,
// created_at) so either works.
func insertDelivery(t *testing.T, db *sql.DB, eventID int64, status string, created time.Time) {
	t.Helper()
	var sentArg any
	if status == "sent" {
		sentArg = formatTS(created)
	}
	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO notification_deliveries
			(event_id, channel_id, channel_name, channel_type, severity,
			 status, payload, created_at, sent_at)
		VALUES (?, 1, 'ch', 'ntfy', 'warning', ?, '{}', ?, ?)`,
		eventID, status, formatTS(created), sentArg,
	); err != nil {
		t.Fatalf("insert delivery: %v", err)
	}
}

func countAlertEvents(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM alert_events`).Scan(&n); err != nil {
		t.Fatalf("count alerts: %v", err)
	}
	return n
}

func countDeliveries(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM notification_deliveries`).Scan(&n); err != nil {
		t.Fatalf("count deliveries: %v", err)
	}
	return n
}
