// digest_test.go — RFC 0004 §"Digest" failing tests.
//
// These tests assert the behaviour Sprint 4 has to implement: per-channel
// `digest_window` setting, dispatcher buffering, flush-on-window-expiry,
// early-flush at N=10, and firing+resolved collapse for the same rule.
//
// All tests are expected to FAIL on the current v0.7.3 dispatcher (no
// digest_window concept exists; the field is absent from ChannelConfig;
// the dispatcher enqueues-and-flushes synchronously per event). The
// implementation will land in Sprint 4 D1 — these tests are the spec.
package alerts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// openAlertsTestDB spins up a fresh sqlite file with the minimum schema
// the alert subsystem needs (notification_channels +
// notification_deliveries + alert_events) so we can drive the
// dispatcher + claimNext() path. Mirrors the openTestDB helper in
// maintenance_test.go / backup/snapshot_test.go.
func openAlertsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "alerts.db")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Minimal subset of the real schema. Only the columns our tests
	// touch are populated; the dispatcher's claimNext + process path
	// reads the rest.
	_, _ = db.Exec(`CREATE TABLE notification_channels (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		config TEXT NOT NULL DEFAULT '{}',
		owner_type TEXT NOT NULL DEFAULT 'admin',
		enabled INTEGER NOT NULL DEFAULT 1,
		min_severity TEXT NOT NULL DEFAULT 'info',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	_, _ = db.Exec(`CREATE TABLE alert_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		rule_id INTEGER NOT NULL,
		host TEXT NOT NULL,
		started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		resolved_at DATETIME
	)`)
	_, _ = db.Exec(`CREATE TABLE notification_deliveries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		event_id INTEGER NOT NULL,
		channel_id INTEGER NOT NULL,
		channel_name TEXT NOT NULL,
		channel_type TEXT NOT NULL,
		severity TEXT NOT NULL DEFAULT 'info',
		status TEXT NOT NULL DEFAULT 'pending',
		attempts INTEGER NOT NULL DEFAULT 0,
		http_status INTEGER,
		error TEXT,
		next_retry_at DATETIME,
		payload TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		sent_at DATETIME,
		-- RFC 0004 digest: when non-empty + non-zero, the dispatcher
		-- holds the row until next_flush_at passes. rows_count tracks
		-- how many rows are buffered in the same window for the early-
		-- flush rule (N≥10 → flush immediately).
		digest_window TEXT NOT NULL DEFAULT '',
		next_flush_at DATETIME,
		rows_count INTEGER NOT NULL DEFAULT 0
	)`)
	_, _ = db.Exec(`CREATE INDEX idx_deliveries_digest ON notification_deliveries(digest_window, next_flush_at)`)
	return db
}

// TestParseDigestWindow covers the digest_window field validation.
// The RFC restricts the set to {"", "0", "1m", "5m", "15m", "1h"}.
// Anything else must be rejected with a sentinel error so the
// validateChannel path can return 400.
func TestParseDigestWindow(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"empty → no buffering", "", 0, false},
		{"0 → no buffering", "0", 0, false},
		{"1m", "1m", 1 * time.Minute, false},
		{"5m", "5m", 5 * time.Minute, false},
		{"15m", "15m", 15 * time.Minute, false},
		{"1h", "1h", 1 * time.Hour, false},
		{"2m → unsupported", "2m", 0, true},
		{"30s → unsupported (granularity)", "30s", 0, true},
		{"garbage", "not-a-duration", 0, true},
		{"whitespace", "  ", 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, err := ParseDigestWindow(c.in)
			if (err != nil) != c.wantErr {
				t.Fatalf("ParseDigestWindow(%q) err = %v, wantErr = %v", c.in, err, c.wantErr)
			}
			if !c.wantErr && d != c.want {
				t.Errorf("ParseDigestWindow(%q) = %v, want %v", c.in, d, c.want)
			}
		})
	}
}

// TestChannelConfig_HasDigestWindow asserts the wire shape gains the
// new field. Operators paste the digest window alongside the existing
// channel config JSON; without the field, the JSON unmarshal would
// silently drop the key and the feature would be no-op for everyone.
func TestChannelConfig_HasDigestWindow(t *testing.T) {
	var cc ChannelConfig
	if err := json.Unmarshal([]byte(`{"digest_window":"5m","url":"https://x"}`), &cc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cc.DigestWindow != "5m" {
		t.Errorf("ChannelConfig.DigestWindow = %q, want \"5m\"", cc.DigestWindow)
	}
}

// TestChannelConfig_MaskedDigestWindowIsInert covers the secret-mask
// pattern: an operator who edits other channel fields shouldn't lose
// their digest_window. The preserveSecrets() function already handles
// telegram.bot_token + email.password; the digest field is not a
// secret but the test is here to guarantee the mask-style UPDATE path
// doesn't accidentally clobber it.
func TestChannelConfig_MaskedDigestWindowIsInert(t *testing.T) {
	// This is a structural test — the exact preservation semantics
	// will be defined alongside the mask logic. For now we assert
	// that the JSON round-trip preserves the field even after a
	// second edit.
	original := `{"digest_window":"5m","url":"https://x"}`
	var cc ChannelConfig
	if err := json.Unmarshal([]byte(original), &cc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	b, err := json.Marshal(cc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var cc2 ChannelConfig
	if err := json.Unmarshal(b, &cc2); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if cc2.DigestWindow != cc.DigestWindow {
		t.Errorf("round-trip changed digest_window: %q → %q", cc.DigestWindow, cc2.DigestWindow)
	}
}

// TestValidateChannel_DigestWindow_Invalid covers the channel-level
// validation: a digest_window that doesn't parse must reject the
// create / update. We re-use the openAlertsTestDB pattern so we
// exercise the same code path the HTTP handler does.
func TestValidateChannel_DigestWindow_Invalid(t *testing.T) {
	_ = openAlertsTestDB(t) // confirm the test setup itself works
	c := &Channel{
		Name:   "ops",
		Type:   "webhook",
		Config: `{"url":"https://example.com/hook","digest_window":"2m"}`,
	}
	err := validateChannel(c)
	if err == nil {
		t.Fatal("validateChannel with digest_window=2m should have rejected, got nil")
	}
	if !errors.Is(err, ErrInvalidDigestWindow) {
		t.Errorf("expected ErrInvalidDigestWindow, got %v", err)
	}
}

// TestDispatcher_BuffersAndFlushesOnWindow covers the dispatcher's
// new buffering path: with digest_window=5m, N=3 events enqueued
// within the window must NOT dispatch (status stays 'pending' or a
// new 'buffered' state) until the window expires. We don't drive the
// real time clock — we exercise the "should this row dispatch now?"
// predicate against an injected clock.
func TestDispatcher_BuffersAndFlushesOnWindow(t *testing.T) {
	db := openAlertsTestDB(t)
	// Insert a channel + 3 event rows + 3 delivery rows.
	ch := mustInsertChannel(t, db, "ops", "webhook", `{"url":"https://x","digest_window":"5m"}`)
	for i := 0; i < 3; i++ {
		mustInsertEvent(t, db)
	}
	mustInsertPendingDeliveries(t, db, ch.ID, ch.Name, ch.Type, 3)

	// At t=now, the dispatcher must NOT claim any of the 3 rows
	// (still within the 5m window). After advancing the clock by
	// 5m+1s, the dispatcher must claim all 3 in one drain cycle.
	now := time.Now().UTC()
	d := NewDispatcher(DispatcherConfig{
		DB:           db,
		PollInterval: 1 * time.Second,
		Workers:      1,
	})

	clock := func() time.Time { return now } // freeze
	_ = clock
	// claimNext is the seam — the buffered-row predicate will
	// short-circuit when next_flush_at > clock. We don't have an
	// explicit clock hook yet, so this test currently fails because
	// claimNext treats the rows as immediately eligible.
	row, err := d.claimNext(context.Background())
	if err != nil {
		t.Fatalf("claimNext: %v", err)
	}
	if row != nil {
		t.Fatalf("expected nil row (still within 5m window), got %+v", row)
	}
}

// TestDispatcher_EarlyFlushAtTen covers RFC 0004: "flush early once
// N≥10 rows accumulate to avoid silent drops". A storm of 10 events
// within a 5m window must flush immediately, even though the window
// hasn't expired.
func TestDispatcher_EarlyFlushAtTen(t *testing.T) {
	db := openAlertsTestDB(t)
	ch := mustInsertChannel(t, db, "ops", "webhook", `{"url":"https://x","digest_window":"5m"}`)
	for i := 0; i < 10; i++ {
		mustInsertEvent(t, db)
	}
	mustInsertPendingDeliveries(t, db, ch.ID, ch.Name, ch.Type, 10)

	d := NewDispatcher(DispatcherConfig{DB: db, PollInterval: 1 * time.Second, Workers: 1})
	// claimNext should return a row even though the window is fresh
	// — the "N≥10 → flush now" rule overrides the time gate.
	row, err := d.claimNext(context.Background())
	if err != nil {
		t.Fatalf("claimNext: %v", err)
	}
	if row == nil {
		t.Fatal("expected early-flush row at N=10, got nil")
	}
}

// TestDispatcher_FiringPlusResolvedCollapse covers the merge rule:
// when a single rule has both a firing transition AND a resolved
// transition inside the same digest window, the rendered Message
// collapses them into a single notification. Without this, the
// operator gets an alert storm followed immediately by an
// all-clear — confusing on a digest channel.
func TestDispatcher_FiringPlusResolvedCollapse(t *testing.T) {
	// Two notifications for the same (rule, host) — one firing, one
	// resolved — should render to ONE Message body in the digest.
	firing := Notification{RuleID: 1, RuleName: "cpu high", Host: "h1", Metric: "cpu_pct", Severity: "warning", State: "firing", Value: 92, Threshold: 80}
	resolved := Notification{RuleID: 1, RuleName: "cpu high", Host: "h1", Metric: "cpu_pct", Severity: "warning", State: "resolved", Value: 75, Threshold: 80}

	got := FormatDigestBody([]Notification{firing, resolved})
	// FormatMessage renders firing as "is <value> (threshold ...)" and
	// resolved as "back below threshold (...)" — check both shapes
	// are present, and that the host is named.
	if !strings.Contains(got, "h1") {
		t.Errorf("digest body should mention the host, got %q", got)
	}
	if !strings.Contains(got, "92.00") {
		t.Errorf("digest body should show firing value, got %q", got)
	}
	if !strings.Contains(got, "back below") {
		t.Errorf("digest body should preserve resolved phrasing, got %q", got)
	}
	// Header counts the events so the operator sees "2 alert(s)"
	// at the top of the digest — not "1 alert" which would imply
	// collapse instead of the per-event bullet we render.
	if !strings.Contains(got, "2 alert") {
		t.Errorf("digest header should count 2 events, got %q", got)
	}
}

// --- helpers ---

func mustInsertChannel(t *testing.T, db *sql.DB, name, typ, cfg string) Channel {
	t.Helper()
	res, err := db.Exec(`INSERT INTO notification_channels (name, type, config) VALUES (?, ?, ?)`,
		name, typ, cfg)
	if err != nil {
		t.Fatalf("insert channel: %v", err)
	}
	id, _ := res.LastInsertId()
	return Channel{ID: id, Name: name, Type: typ, Config: cfg, Enabled: true, MinSeverity: "info"}
}

func mustInsertEvent(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO alert_events (rule_id, host) VALUES (1, 'h1')`)
	if err != nil {
		t.Fatalf("insert event: %v", err)
	}
}

func mustInsertPendingDeliveries(t *testing.T, db *sql.DB, chID int64, chName, chType string, n int) {
	t.Helper()
	now := time.Now().UTC()
	for i := 0; i < n; i++ {
		_, err := db.Exec(`INSERT INTO notification_deliveries
			(event_id, channel_id, channel_name, channel_type, payload, digest_window, next_flush_at, rows_count)
			VALUES (1, ?, ?, ?, '{}', '5m', ?, ?)`,
			chID, chName, chType,
			now.Add(5*time.Minute), // future: still within window
			n,
		)
		if err != nil {
			t.Fatalf("insert delivery %d: %v", i, err)
		}
	}
}

// os + filepath imports kept for parity with the maintenance openTestDB
// pattern (t.TempDir is used through openAlertsTestDB). These aliases
// silence the "imported and not used" linter when this file is built
// in isolation before helpers are wired in.
var (
	_ = os.Getenv
	_ = filepath.Join
)
