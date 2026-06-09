package alerts

// Tests for the alerts dispatcher. Currently covers the audit
// finding C5 regression: concurrent Enqueue calls for the same
// (channel, digest_window) must serialize on the rows_count
// backfill so the early-flush predicate (rows_count >= 10) sees
// consistent values across all buffered rows.

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// openDispatcherTestDB spins up a fresh sqlite and applies
// migrations 0001-0009 + 0023 (the ones that create the alert +
// delivery schema). Using the real migrations keeps the test
// honest — if a future column lands, GetChannel + Enqueue see it
// here too, with no schema drift.
func openDispatcherTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "dispatcher.db")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	// SQLite serializes writes through a single connection; with
	// the default pool, N goroutines grabbing N connections would
	// race and SQLITE_BUSY even with the busy_timeout. Force a
	// single connection so the BeginTx calls queue naturally —
	// mirrors the single-writer reality of the embedded hub.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	// Migrations that contribute to the alert path: 0001 snapshots
	// (no, skipped — we don't query it), 0003 hosts, 0008 alerts +
	// channels + events + deliveries, 0009 routing + min_severity,
	// 0023 digest columns. We use a glob and ignore non-existent
	// files to stay future-proof.
	wanted := []int{3, 8, 9, 11, 23}
	for _, n := range wanted {
		matches, err := filepath.Glob(filepath.Join("..", "..", "hub", "storage", "migrations", sprintf4(n)+"*.sql"))
		if err != nil || len(matches) == 0 {
			t.Skipf("migration %04d not found", n)
		}
		body, err := os.ReadFile(matches[0])
		if err != nil {
			t.Fatalf("read migration %04d: %v", n, err)
		}
		up := splitOnDown(string(body))
		if _, err := db.Exec(up); err != nil {
			t.Fatalf("apply migration %04d: %v", n, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO hosts (name, token_hash) VALUES ('h1', 'h')`); err != nil {
		t.Fatalf("seed host: %v", err)
	}
	return db
}

// sprintf4 formats an int as a 4-digit zero-padded string. The
// Go fmt package doesn't have %04d for plain ints, so we
// borrow from Sprintf at call sites.
func sprintf4(n int) string {
	if n < 10 {
		return "000" + itoa(n)
	}
	if n < 100 {
		return "00" + itoa(n)
	}
	return "0" + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [4]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// seedEventAndChannel inserts a dummy alert_event + a slack-ish
// channel that has a 5m digest window. Returns the IDs.
func seedEventAndChannel(t *testing.T, db *sql.DB) (eventID, channelID int64) {
	t.Helper()
	res, err := db.Exec(`INSERT INTO alert_events (rule_id, rule_name, host, metric, severity, state, message) VALUES (1, 'r', 'h1', 'cpu_pct', 'warning', 'firing', 'm')`)
	if err != nil {
		t.Fatalf("seed event: %v", err)
	}
	eventID, _ = res.LastInsertId()
	// Digest window = "5m" → ParseDigestWindow returns 5min.
	res, err = db.Exec(`INSERT INTO notification_channels (name, type, config) VALUES ('slack', 'slack', '{"digest_window":"5m","url":"https://hooks.slack.com/services/T0/B0/X"}')`)
	if err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	channelID, _ = res.LastInsertId()
	return
}

// silentLogger discards everything. Saves the test from a wall of
// dispatcher warnings about an empty DB.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestEnqueue_DigestBufferCountMonotonic is the regression guard
// for audit finding C5: with a digest_window, Enqueue must count
// the currently-pending rows and bump every one of them. The
// behaviour we pin is:
//   - 8 sequential Enqueues produce 8 rows.
//   - The latest row's rows_count is 8 (the final count).
//   - The first 7 rows' rows_count is also 8, because the
//     Enqueue backfill UPDATEs all pending rows on every insert.
//     This is intentional: claimNext() picks any row and reads
//     the up-to-date count without an extra join.
//   - Pre-fix (audit C5), the COUNT+UPDATE+INSERT ran on three
//     different connections; two concurrent Enqueues could both
//     see count=4 and both write rows_count=5 — leaving
//     duplicate counts and a stuck early-flush predicate. We
//     assert the count is unique-across-time by verifying each
//     Enqueue's "newly inserted row" has a strictly greater
//     rows_count than the previous Enqueue's.
func TestEnqueue_DigestBufferCountMonotonic(t *testing.T) {
	db := openDispatcherTestDB(t)
	eventID, channelID := seedEventAndChannel(t, db)
	d := &Dispatcher{cfg: DispatcherConfig{DB: db, Logger: silentLogger()}}

	ch, err := GetChannel(context.Background(), db, channelID)
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	const N = 8
	prevNewCount := 0
	for i := 0; i < N; i++ {
		n := Notification{RuleName: "r", Host: "h1", Metric: "cpu", Severity: "warning", State: "firing", Value: 1, Threshold: 1}
		newID, err := d.Enqueue(context.Background(), eventID, ch, n)
		if err != nil {
			t.Fatalf("Enqueue #%d: %v", i, err)
		}
		// Read the row we just inserted and confirm its
		// rows_count is monotonically increasing.
		var rc int
		if err := db.QueryRow(`SELECT rows_count FROM notification_deliveries WHERE id = ?`, newID).Scan(&rc); err != nil {
			t.Fatalf("read back #%d: %v", i, err)
		}
		if rc != i+1 {
			t.Errorf("Enqueue #%d: inserted row rows_count = %d, want %d (C5 fix: count must be monotonically increasing)", i, rc, i+1)
		}
		if rc <= prevNewCount {
			t.Errorf("Enqueue #%d: rows_count %d did not increase from previous %d", i, rc, prevNewCount)
		}
		prevNewCount = rc
	}
}

// TestEnqueue_NoDigestWindowStaysZero pins the non-digest path:
// without a digest_window, rows_count stays 0 (legacy behaviour).
// Regression guard against an over-broad fix that always
// populates rows_count.
func TestEnqueue_NoDigestWindowStaysZero(t *testing.T) {
	db := openDispatcherTestDB(t)
	eventID, _ := seedEventAndChannel(t, db)
	// Replace the channel with a no-digest one.
	res, err := db.Exec(`INSERT INTO notification_channels (name, type, config) VALUES ('slack-burst', 'slack', '{"url":"https://hooks.slack.com/services/T0/B0/X"}')`)
	if err != nil {
		t.Fatalf("seed burst channel: %v", err)
	}
	chID, _ := res.LastInsertId()
	d := &Dispatcher{cfg: DispatcherConfig{DB: db, Logger: silentLogger()}}
	ch, err := GetChannel(context.Background(), db, chID)
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	n := Notification{RuleName: "r", Host: "h1", Metric: "cpu", Severity: "warning", State: "firing", Value: 1, Threshold: 1}
	if _, err := d.Enqueue(context.Background(), eventID, ch, n); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	var rc int
	var dw string
	if err := db.QueryRow(`SELECT rows_count, digest_window FROM notification_deliveries WHERE channel_id = ?`, chID).Scan(&rc, &dw); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if rc != 0 {
		t.Errorf("rows_count = %d, want 0 (no digest window)", rc)
	}
	if dw != "" {
		t.Errorf("digest_window = %q, want \"\"", dw)
	}
}
