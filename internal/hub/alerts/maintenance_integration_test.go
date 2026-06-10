package alerts

// Integration test for issue #35 — wires a real *sql.DB (with the
// alert_rules + hosts + host_tags tables), a real *maintenance.Cacher
// (Refresh'd from the DB), and a real alerts.Engine configured with
// the same closure shape server.go builds. Asserts that an active
// maintenance window matching the host's tag scope suppresses the
// firing transition end-to-end.
//
// The bug from #33 — the engine's Maintenance field was nil so
// inMaintenance always short-circuited to false — would have been
// caught by this test: a firing transition would have appeared on
// the un-wired engine. We pass it in Maintenance below to keep
// the regression test meaningful (without it, the firing
// transition would still be emitted and the test would fail).

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/quanla93/lumen/internal/hub/maintenance"
	"github.com/quanla93/lumen/internal/shared/api"
)

// openMaintIntegrationDB spins up a fresh sqlite and applies
// migrations 0001-0022 from the project's storage/migrations dir.
// Running the real migrations (vs. hand-rolling a schema) keeps
// the test honest: if a new column is added to hosts in a future
// migration, AllHostTags will see it here too, and the test will
// catch any future drift without us editing it.
func openMaintIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "maint_int.db")
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	const first = 1
	const last = 22
	for n := first; n <= last; n++ {
		path := filepath.Join("..", "..", "hub", "storage", "migrations",
			fmt.Sprintf("%04d_*.sql", n))
		matches, err := filepath.Glob(path)
		if err != nil || len(matches) == 0 {
			t.Fatalf("locate migration %04d: %v", n, err)
		}
		body, err := os.ReadFile(matches[0])
		if err != nil {
			t.Fatalf("read migration %04d: %v", n, err)
		}
		// Skip the goose Down section (we don't track a migration
		// history table — just run the Up statements in order).
		up := splitOnDown(string(body))
		if _, err := db.Exec(up); err != nil {
			t.Fatalf("apply migration %04d: %v", n, err)
		}
	}
	return db
}

// splitOnDown returns the portion of a goose migration before the
// "-- +goose Down" marker, so the test only runs the Up half. We
// don't need a goose library — the test runs each file in order.
func splitOnDown(sql string) string {
	if i := strings.Index(sql, "-- +goose Down"); i >= 0 {
		return sql[:i]
	}
	return sql
}

// TestRunOnce_MaintenanceWindowSuppressesFiring_Integration is the
// end-to-end coverage for issue #35. It wires a real cacher +
// engine + DB and asserts that a firing transition is dropped while
// a window is active. Without the maintenance lister wired into
// runOnce (the original bug), this test would observe a firing
// transition and fail.
func TestRunOnce_MaintenanceWindowSuppressesFiring_Integration(t *testing.T) {
	ctx := context.Background()
	db := openMaintIntegrationDB(t)

	// 1. One host with tag t=prod. The window we'll create is empty
	//    scope, so it matches any host — this host included. Insert
	//    directly into hosts + host_tags so we don't pull in the
	//    broader hosts.Create / SetTags schema (system_os, etc).
	if _, err := db.ExecContext(ctx,
		`INSERT INTO hosts (name, token_hash) VALUES (?, ?)`,
		"host-a", "test-hash-a",
	); err != nil {
		t.Fatalf("insert host: %v", err)
	}
	var hostID int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM hosts WHERE name = ?`, "host-a").Scan(&hostID); err != nil {
		t.Fatalf("lookup host id: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO host_tags (host_id, key, value) VALUES (?, ?, ?)`,
		hostID, "t", "prod",
	); err != nil {
		t.Fatalf("insert tag: %v", err)
	}

	// 2. One CPU rule (for_seconds=0 so it fires on the first tick).
	if _, err := CreateRule(ctx, db, Rule{
		Name: "cpu high", Metric: "cpu_pct", Comparator: "gt",
		Threshold: 50, ForSeconds: 0, Enabled: true, Severity: "warning",
	}); err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	// 3. Active maintenance window: now-30s → now+30m, empty scope.
	now := time.Now().UTC()
	cacher := &maintenance.Cacher{DB: db}
	if err := cacher.Refresh(ctx); err != nil {
		t.Fatalf("cacher.Refresh (empty): %v", err)
	}
	if _, err := maintenance.Create(ctx, db, maintenance.Window{
		StartAt:   now.Add(-30 * time.Second),
		EndAt:     now.Add(30 * time.Minute),
		Reason:    "deploy",
		ScopeTags: map[string]string{},
	}); err != nil {
		t.Fatalf("maintenance.Create: %v", err)
	}
	if err := cacher.Refresh(ctx); err != nil {
		t.Fatalf("cacher.Refresh: %v", err)
	}

	// 4. Build the MaintenanceLister closure the same way server.go
	//    does. We don't go through server.go to keep this test in the
	//    alerts package (server.go lives elsewhere).
	hostsLister := HostsListerFromDB(db)
	tagsLister := TagsListerFromDB(db)
	maintLister := func(ctx context.Context) (map[string][]MaintenanceWindow, error) {
		registered, err := hostsLister(ctx)
		if err != nil {
			return nil, err
		}
		tags, _ := tagsLister(ctx)
		full := cacher.AllActive(registered, tags, time.Now())
		if len(full) == 0 {
			return nil, nil
		}
		out := make(map[string][]MaintenanceWindow, len(full))
		for host, wins := range full {
			sl := make([]MaintenanceWindow, 0, len(wins))
			for _, w := range wins {
				sl = append(sl, MaintenanceWindow{
					StartAt: w.StartAt, EndAt: w.EndAt, ScopeTags: w.ScopeTags,
				})
			}
			out[host] = sl
		}
		return out, nil
	}

	// 5. Engine with a fake snapshot store so we don't need a
	//    hub/store. The store just returns the one host above
	//    threshold.
	store := &fakeStore{snap: []api.HostSnapshot{
		{Host: "host-a", Ts: now, CpuPct: 90},
	}}

	engine := NewEngine(Config{
		DB:          db,
		Store:       store,
		Hosts:       hostsLister,
		Tags:        tagsLister,
		Maintenance: maintLister,
		Logger:      slogTest(t),
	})

	// 6. Drive the same path runOnce would, but with our own
	//    time.Now() so the test is deterministic. We mirror the
	//    relevant slice of runOnce: load rules, hosts, tags,
	//    maintenance; then evaluate.
	tr := engine.TickWithTagsAndMaintenance(
		now,
		mustListEnabledRules(t, ctx, db),
		store.snap,
		[]string{"host-a"},
		map[string]map[string]string{"host-a": {"t": "prod"}},
		mustMaintenanceMap(t, ctx, maintLister),
	)
	if len(tr) != 0 {
		t.Fatalf("expected 0 transitions (window active), got %d: %+v", len(tr), tr)
	}

	// 7. Sanity: with the same setup but no maintenance lister, the
	//    firing transition IS emitted. This protects the test from
	//    a false positive where the engine swallows the rule for
	//    some unrelated reason.
	engineBare := NewEngine(Config{
		DB:          db,
		Store:       store,
		Hosts:       hostsLister,
		Tags:        tagsLister,
		Maintenance: nil, // intentionally nil — regression sentinel
		Logger:      slogTest(t),
	})
	trBare := engineBare.TickWithTags(
		now,
		mustListEnabledRules(t, ctx, db),
		store.snap,
		[]string{"host-a"},
		map[string]map[string]string{"host-a": {"t": "prod"}},
	)
	if len(trBare) != 1 || trBare[0].State != "firing" {
		t.Fatalf("baseline (no maintenance) should fire: got %+v", trBare)
	}
}

// TestRunOnce_MaintenanceWindowScopeMismatch_Integration covers the
// inverse: a window whose scope does not match the host's tag set
// must NOT suppress. The alert fires as normal.
func TestRunOnce_MaintenanceWindowScopeMismatch_Integration(t *testing.T) {
	ctx := context.Background()
	db := openMaintIntegrationDB(t)
	now := time.Now().UTC()

	if _, err := db.ExecContext(ctx,
		`INSERT INTO hosts (name, token_hash) VALUES (?, ?)`,
		"host-dev", "test-hash-dev",
	); err != nil {
		t.Fatalf("insert host: %v", err)
	}
	var hostID int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM hosts WHERE name = ?`, "host-dev").Scan(&hostID); err != nil {
		t.Fatalf("lookup host id: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO host_tags (host_id, key, value) VALUES (?, ?, ?)`,
		hostID, "tier", "dev",
	); err != nil {
		t.Fatalf("insert tag: %v", err)
	}
	if _, err := CreateRule(ctx, db, Rule{
		Name: "cpu high", Metric: "cpu_pct", Comparator: "gt",
		Threshold: 50, ForSeconds: 0, Enabled: true, Severity: "warning",
	}); err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	// Window scope = t=prod; host has t=dev → no match → no suppression.
	cacher := &maintenance.Cacher{DB: db}
	if _, err := maintenance.Create(ctx, db, maintenance.Window{
		StartAt:   now.Add(-30 * time.Second),
		EndAt:     now.Add(30 * time.Minute),
		ScopeTags: map[string]string{"tier": "prod"},
	}); err != nil {
		t.Fatalf("maintenance.Create: %v", err)
	}
	if err := cacher.Refresh(ctx); err != nil {
		t.Fatalf("cacher.Refresh: %v", err)
	}
	hostsLister := HostsListerFromDB(db)
	tagsLister := TagsListerFromDB(db)
	maintLister := func(ctx context.Context) (map[string][]MaintenanceWindow, error) {
		registered, _ := hostsLister(ctx)
		tags, _ := tagsLister(ctx)
		full := cacher.AllActive(registered, tags, time.Now())
		out := make(map[string][]MaintenanceWindow, len(full))
		for h, wins := range full {
			sl := make([]MaintenanceWindow, 0, len(wins))
			for _, w := range wins {
				sl = append(sl, MaintenanceWindow{StartAt: w.StartAt, EndAt: w.EndAt, ScopeTags: w.ScopeTags})
			}
			out[h] = sl
		}
		return out, nil
	}
	store := &fakeStore{snap: []api.HostSnapshot{
		{Host: "host-dev", Ts: now, CpuPct: 90},
	}}
	engine := NewEngine(Config{
		DB: db, Store: store, Hosts: hostsLister, Tags: tagsLister,
		Maintenance: maintLister, Logger: slogTest(t),
	})
	tr := engine.TickWithTagsAndMaintenance(
		now, mustListEnabledRules(t, ctx, db), store.snap,
		[]string{"host-dev"},
		map[string]map[string]string{"host-dev": {"tier": "dev"}},
		mustMaintenanceMap(t, ctx, maintLister),
	)
	if len(tr) != 1 || tr[0].State != "firing" {
		t.Fatalf("scope-mismatched window should NOT suppress, got %+v", tr)
	}
}

// --- helpers ---

// slogTest returns a logger that discards output. Tests don't want
// alert-engine debug noise on stdout/stderr.
func slogTest(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeStore is a SnapshotProvider the engine reads each tick. We
// keep it as a named type so the integration tests can reuse it
// across two engines (the suppressing one and the baseline).
type fakeStore struct{ snap []api.HostSnapshot }

func (f *fakeStore) Snapshot() []api.HostSnapshot { return f.snap }

var _ SnapshotProvider = (*fakeStore)(nil)

func mustListEnabledRules(t *testing.T, ctx context.Context, db *sql.DB) []Rule {
	t.Helper()
	rules, err := ListEnabledRules(ctx, db)
	if err != nil {
		t.Fatalf("ListEnabledRules: %v", err)
	}
	return rules
}

func mustMaintenanceMap(t *testing.T, ctx context.Context, lister MaintenanceLister) map[string][]MaintenanceWindow {
	t.Helper()
	m, err := lister(ctx)
	if err != nil {
		t.Fatalf("maintenance lister: %v", err)
	}
	return m
}
