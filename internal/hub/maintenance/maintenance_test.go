package maintenance

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// openTestDB spins up a fresh sqlite file in t.TempDir() and runs
// just the 0022 migration (maintenance_windows table + processes
// settings). We isolate this to one test DB per test for
// independent results.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Migration 0022 contains INSERT OR IGNORE INTO settings (...) for
	// the processes.* defaults. Create the table so the INSERT
	// doesn't fail with "no such table: settings". The contents
	// don't matter to the maintenance tests — the cacher reads from
	// maintenance_windows only.
	if _, err := db.Exec(`CREATE TABLE settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create settings: %v", err)
	}

	// Run only the maintenance_windows DDL (skip the processes.*
	// INSERTs in migration 0022 — they require a `users` table from
	// the FKEY on maintenance_windows.created_by too).
	if _, err := db.Exec(
		`CREATE TABLE maintenance_windows (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			start_at   DATETIME NOT NULL,
			end_at     DATETIME NOT NULL,
			reason     TEXT     NOT NULL DEFAULT '',
			scope_tags TEXT     NOT NULL DEFAULT '{}',
			created_by INTEGER,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX idx_mw_active ON maintenance_windows(start_at, end_at);
		CREATE INDEX idx_mw_created_at ON maintenance_windows(created_at);`,
	); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
	return db
}

// TestCreateAndList exercises the create / list flow. Past
// windows are pulled from the DB; active/upcoming come from the
// cacher. We round-trip both.
func TestCreateAndList(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	now := time.Now().UTC()
	past := Window{StartAt: now.Add(-2 * time.Hour), EndAt: now.Add(-1 * time.Hour), Reason: "past"}
	active := Window{StartAt: now.Add(-30 * time.Minute), EndAt: now.Add(30 * time.Minute), Reason: "active"}
	upcoming := Window{StartAt: now.Add(1 * time.Hour), EndAt: now.Add(2 * time.Hour), Reason: "upcoming"}

	for _, w := range []Window{past, active, upcoming} {
		if _, err := Create(ctx, db, w); err != nil {
			t.Fatalf("create %q: %v", w.Reason, err)
		}
	}

	// Past list comes from the DB directly.
	rows, err := db.QueryContext(ctx,
		`SELECT id, start_at, end_at, reason FROM maintenance_windows WHERE end_at <= ?`,
		now,
	)
	if err != nil {
		t.Fatalf("query past: %v", err)
	}
	defer rows.Close()
	pastCount := 0
	for rows.Next() {
		pastCount++
	}
	if pastCount != 1 {
		t.Errorf("past count = %d, want 1", pastCount)
	}

	// Cacher picks up the active+upcoming on Refresh.
	c := &Cacher{DB: db}
	if err := c.Refresh(ctx); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	activeList := c.List("active", now)
	if len(activeList) != 1 || activeList[0].Reason != "active" {
		t.Errorf("active list = %+v, want 1 entry with reason=active", activeList)
	}
	upcomingList := c.List("upcoming", now)
	if len(upcomingList) != 1 || upcomingList[0].Reason != "upcoming" {
		t.Errorf("upcoming list = %+v, want 1 entry with reason=upcoming", upcomingList)
	}
}

// TestScopeMatch covers matchScope's subset-of-tags semantics. The
// window is "matches all hosts with tag t=prod"; the host is
// "tag=t=prod, env=db" — match. A host with only env=db doesn't
// match.
func TestScopeMatch(t *testing.T) {
	host := map[string]string{"t": "prod", "env": "db"}
	cases := []struct {
		name  string
		scope map[string]string
		want  bool
	}{
		{"empty scope matches all", map[string]string{}, true},
		{"subset exact", map[string]string{"t": "prod"}, true},
		{"subset case-insensitive value", map[string]string{"t": "PROD"}, true},
		{"subset extra tag in host", map[string]string{"t": "prod", "env": "db"}, true},
		{"missing required key", map[string]string{"region": "us"}, false},
		{"wrong value", map[string]string{"t": "dev"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := matchScope(host, c.scope); got != c.want {
				t.Errorf("matchScope(%v) = %v, want %v", c.scope, got, c.want)
			}
		})
	}
}

// TestActiveAt covers the alerts engine's view: ActiveAt should
// return only windows whose [start_at, end_at] contains now AND
// whose scope matches the host.
func TestActiveAt(t *testing.T) {
	now := time.Now().UTC()
	wins := []Window{
		{ID: 1, StartAt: now.Add(-1 * time.Hour), EndAt: now.Add(1 * time.Hour), ScopeTags: map[string]string{"t": "prod"}},
		{ID: 2, StartAt: now.Add(-30 * time.Minute), EndAt: now.Add(30 * time.Minute), ScopeTags: map[string]string{}},
		{ID: 3, StartAt: now.Add(2 * time.Hour), EndAt: now.Add(3 * time.Hour), ScopeTags: map[string]string{}}, // future
		{ID: 4, StartAt: now.Add(-2 * time.Hour), EndAt: now.Add(-1 * time.Hour), ScopeTags: map[string]string{}}, // past
	}
	c := &Cacher{cache: wins}

	hostProd := map[string]string{"t": "prod"}
	hostOther := map[string]string{"t": "dev"}

	active := c.ActiveAt(hostProd, now)
	// Window 1 (t=prod scope, time-matched) AND window 2 (empty
	// scope = matches all) both match hostProd. Window 3 is in the
	// future, window 4 is in the past.
	if len(active) != 2 {
		t.Errorf("hostProd active len = %d, want 2 (scope-matched + empty-scope), got %+v", len(active), active)
	}

	activeOther := c.ActiveAt(hostOther, now)
	// hostOther has t=dev; window 1's scope requires t=prod, so
	// only window 2 (empty scope) matches.
	if len(activeOther) != 1 || activeOther[0].ID != 2 {
		t.Errorf("hostOther active = %+v, want only window 2 (empty scope matches all)", activeOther)
	}
}

// TestUpdateStartAtLocked confirms the RFC 0003 edit guard:
// start_at is immutable once a window has begun. The operator
// can only extend or shorten end_at.
func TestUpdateStartAtLocked(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	id, err := Create(ctx, db, Window{
		StartAt: now.Add(-1 * time.Hour),
		EndAt:   now.Add(1 * time.Hour),
		Reason:  "active",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Try to move start_at into the future — should be rejected.
	err = Update(ctx, db, Window{
		ID:      id,
		StartAt: now.Add(-5 * time.Minute), // changed
		EndAt:   now.Add(1 * time.Hour),
		Reason:  "tampered",
	})
	if err != ErrStartAtLocked {
		t.Errorf("update start_at on active window returned %v, want ErrStartAtLocked", err)
	}

	// Extending end_at is fine.
	if err := Update(ctx, db, Window{
		ID:      id,
		StartAt: now.Add(-1 * time.Hour), // unchanged
		EndAt:   now.Add(2 * time.Hour),  // extended
		Reason:  "extended",
	}); err != nil {
		t.Errorf("update end_at on active window returned %v, want nil", err)
	}
}

// TestUpdateNotFound confirms an unknown id returns the sentinel
// so the handler can map to 404.
func TestUpdateNotFound(t *testing.T) {
	db := openTestDB(t)
	err := Update(context.Background(), db, Window{ID: 9999, StartAt: time.Now(), EndAt: time.Now().Add(time.Hour)})
	if err != ErrNotFound {
		t.Errorf("update non-existent id returned %v, want ErrNotFound", err)
	}
}

// TestCreateEndBeforeStart rejects inverted windows at insert
// time so we don't end up with a stored window that has end_at <
// start_at.
func TestCreateEndBeforeStart(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()
	if _, err := Create(context.Background(), db, Window{
		StartAt: now.Add(time.Hour),
		EndAt:   now,
	}); err == nil {
		t.Error("Create with end_at < start_at returned nil error, want non-nil")
	}
}

// TestAllActive covers the engine-facing adapter: AllActive takes the
// host list + host tag map + a wall-clock instant, and returns the
// host→windows map shape the engine consumes on every tick. The
// alerts engine's runOnce() passes this map straight into evaluate().
func TestAllActive(t *testing.T) {
	now := time.Now().UTC()
	c := &Cacher{cache: []Window{
		// Active, empty scope → matches every host.
		{ID: 1, StartAt: now.Add(-30 * time.Minute), EndAt: now.Add(30 * time.Minute), ScopeTags: map[string]string{}},
		// Active, scope t=prod → matches only the prod host.
		{ID: 2, StartAt: now.Add(-10 * time.Minute), EndAt: now.Add(10 * time.Minute), ScopeTags: map[string]string{"t": "prod"}},
		// Future window → matches nothing.
		{ID: 3, StartAt: now.Add(1 * time.Hour), EndAt: now.Add(2 * time.Hour)},
		// Past window → matches nothing.
		{ID: 4, StartAt: now.Add(-2 * time.Hour), EndAt: now.Add(-1 * time.Hour)},
	}}
	hostTags := map[string]map[string]string{
		"host-prod": {"t": "prod"},
		"host-dev":  {"t": "dev"},
		"host-none": nil, // no tags → only empty-scope windows apply
	}

	got := c.AllActive([]string{"host-prod", "host-dev", "host-none"}, hostTags, now)

	// host-prod: window 1 (empty) + window 2 (scope t=prod matches).
	if wins := got["host-prod"]; len(wins) != 2 {
		t.Errorf("host-prod wins = %d, want 2 (empty + prod-scope), got %+v", len(wins), wins)
	}
	// host-dev: window 1 only (scope t=prod does NOT match t=dev).
	if wins := got["host-dev"]; len(wins) != 1 {
		t.Errorf("host-dev wins = %d, want 1 (empty scope only), got %+v", len(wins), wins)
	}
	// host-none: window 1 only (empty scope matches, scoped window doesn't).
	if wins := got["host-none"]; len(wins) != 1 {
		t.Errorf("host-none wins = %d, want 1 (empty scope only), got %+v", len(wins), wins)
	}
}

// TestAllActive_NoMatchesReturnsNil covers the case where no host has
// any active window: AllActive should return nil (not an empty map) so
// the engine's inMaintenance short-circuit (len(map) == 0) still works.
func TestAllActive_NoMatchesReturnsNil(t *testing.T) {
	now := time.Now().UTC()
	// All windows are in the past.
	c := &Cacher{cache: []Window{
		{StartAt: now.Add(-3 * time.Hour), EndAt: now.Add(-2 * time.Hour)},
	}}
	got := c.AllActive([]string{"h1", "h2"}, nil, now)
	if got != nil {
		t.Errorf("AllActive with no matches = %+v, want nil", got)
	}
}

// TestAllActive_NilHostsReturnsNil covers the empty-host-list path
// (caller passed no hosts; engine may not have called the hosts
// lister yet). AllActive must not allocate an empty map.
func TestAllActive_NilHostsReturnsNil(t *testing.T) {
	now := time.Now().UTC()
	c := &Cacher{cache: []Window{
		{StartAt: now.Add(-time.Hour), EndAt: now.Add(time.Hour)},
	}}
	if got := c.AllActive(nil, nil, now); got != nil {
		t.Errorf("AllActive(nil, ...) = %+v, want nil", got)
	}
}

// TestAllActive_NilTagsStillMatchesEmptyScope covers the case where
// tagsListerFromDB failed (e.g. transient DB blip) and the closure in
// server.go passed nil for the hostTags map. Hosts with nil tags
// should still match windows with an empty scope (the common
// "applies-to-all" window shape).
func TestAllActive_NilTagsStillMatchesEmptyScope(t *testing.T) {
	now := time.Now().UTC()
	c := &Cacher{cache: []Window{
		{StartAt: now.Add(-time.Hour), EndAt: now.Add(time.Hour), ScopeTags: map[string]string{}},
	}}
	got := c.AllActive([]string{"h1"}, nil, now)
	if wins := got["h1"]; len(wins) != 1 {
		t.Errorf("nil tags should still match empty-scope window, got %d wins", len(wins))
	}
}
