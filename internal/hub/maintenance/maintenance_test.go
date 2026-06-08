package maintenance

import (
	"context"
	"database/sql"
	"os"
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

	mig, err := os.ReadFile(filepath.Join("..", "..", "storage", "migrations", "0022_gpu_processes_maintenance.sql"))
	if err != nil {
		// local-dev path
		mig, err = os.ReadFile("../../../storage/migrations/0022_gpu_processes_maintenance.sql")
		if err != nil {
			t.Skipf("migration file not found: %v", err)
		}
	}
	// Run the maintenance_windows portion (skip the processes.*
	// INSERTs — they live in the same migration file but aren't
	// what we're testing here).
	if _, err := db.Exec(string(mig)); err != nil {
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
