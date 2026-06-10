package alerts

import (
	"log/slog"
	"testing"
	"time"

	"github.com/quanla93/lumen/internal/shared/api"
)

func newTestEngine() *Engine {
	return NewEngine(Config{Logger: slog.Default()})
}

func snap(host string, cpu float64, age time.Duration, now time.Time) api.HostSnapshot {
	return api.HostSnapshot{
		Host:   host,
		Ts:     now.Add(-age),
		CpuPct: cpu,
	}
}

// CPU threshold breach must persist past for_seconds before firing once,
// then resolve once the value drops. Clear→breach→breach→clear should
// produce exactly two transitions (fire, resolve).
func TestEvaluate_CpuThresholdFireAndResolve(t *testing.T) {
	e := newTestEngine()
	rule := Rule{
		ID: 1, Name: "cpu hot", Metric: "cpu_pct", Comparator: "gt",
		Threshold: 80, ForSeconds: 30, Severity: "warning", Enabled: true,
	}
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)

	// t=0: breach starts, but for=30s so no fire yet.
	tr := e.Tick(now, []Rule{rule}, []api.HostSnapshot{snap("h1", 90, 0, now)}, nil)
	if len(tr) != 0 {
		t.Fatalf("expected no transitions on initial breach, got %d", len(tr))
	}

	// t=20: still breaching, still under for.
	tr = e.Tick(now.Add(20*time.Second), []Rule{rule},
		[]api.HostSnapshot{snap("h1", 92, 0, now.Add(20*time.Second))}, nil)
	if len(tr) != 0 {
		t.Fatalf("expected no transition at 20s, got %d", len(tr))
	}

	// t=30: now we've crossed for_seconds, must fire.
	tr = e.Tick(now.Add(30*time.Second), []Rule{rule},
		[]api.HostSnapshot{snap("h1", 91, 0, now.Add(30*time.Second))}, nil)
	if len(tr) != 1 || tr[0].State != "firing" {
		t.Fatalf("expected firing transition at 30s, got %#v", tr)
	}

	// t=40: still breaching, but already firing → no duplicate.
	tr = e.Tick(now.Add(40*time.Second), []Rule{rule},
		[]api.HostSnapshot{snap("h1", 91, 0, now.Add(40*time.Second))}, nil)
	if len(tr) != 0 {
		t.Fatalf("expected no duplicate firing, got %#v", tr)
	}

	// t=50: clears → resolve once.
	tr = e.Tick(now.Add(50*time.Second), []Rule{rule},
		[]api.HostSnapshot{snap("h1", 10, 0, now.Add(50*time.Second))}, nil)
	if len(tr) != 1 || tr[0].State != "resolved" {
		t.Fatalf("expected resolved at 50s, got %#v", tr)
	}

	// t=60: still clear → no transition.
	tr = e.Tick(now.Add(60*time.Second), []Rule{rule},
		[]api.HostSnapshot{snap("h1", 10, 0, now.Add(60*time.Second))}, nil)
	if len(tr) != 0 {
		t.Fatalf("expected no transition after resolve, got %#v", tr)
	}
}

// for_seconds=0 should fire on the first breach tick.
func TestEvaluate_CpuImmediateFire(t *testing.T) {
	e := newTestEngine()
	rule := Rule{
		ID: 2, Name: "cpu now", Metric: "cpu_pct", Comparator: "gt",
		Threshold: 50, ForSeconds: 0, Severity: "warning", Enabled: true,
	}
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	tr := e.Tick(now, []Rule{rule},
		[]api.HostSnapshot{snap("h1", 75, 0, now)}, nil)
	if len(tr) != 1 || tr[0].State != "firing" {
		t.Fatalf("expected immediate fire, got %#v", tr)
	}
}

// "all hosts" rule expands per host; each host's breach is independent.
func TestEvaluate_AllHostsRule_PerHostState(t *testing.T) {
	e := newTestEngine()
	rule := Rule{
		ID: 3, Name: "cpu all", Metric: "cpu_pct", Comparator: "gt",
		Threshold: 50, ForSeconds: 0, Host: "", Enabled: true,
		Severity: "warning",
	}
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	tr := e.Tick(now, []Rule{rule},
		[]api.HostSnapshot{
			snap("h1", 90, 0, now), // breach
			snap("h2", 10, 0, now), // not breach
		}, nil)
	if len(tr) != 1 || tr[0].Host != "h1" {
		t.Fatalf("expected one firing for h1, got %#v", tr)
	}
}

// 'offline' rule fires once last-seen age crosses MinOfflineFor (60s).
// With for_seconds=0 the alert fires on the first tick that detects the
// breach — the 60s detection window is the only "ignore blips" floor.
func TestEvaluate_OfflineRule_MinFloor(t *testing.T) {
	e := newTestEngine()
	rule := Rule{
		ID: 4, Name: "offline", Metric: "offline", Comparator: "gt",
		Threshold: 0, ForSeconds: 0, Host: "h1", Severity: "critical",
		Enabled: true,
	}
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)

	// Fresh tick: not offline.
	tr := e.Tick(now, []Rule{rule},
		[]api.HostSnapshot{snap("h1", 0, 5*time.Second, now)}, nil)
	if len(tr) != 0 {
		t.Fatalf("expected not offline at 5s age, got %#v", tr)
	}

	// 90s age > MinOfflineFor (60s): breach + immediate fire (no extra
	// persistence window when for_seconds=0).
	t1 := now.Add(90 * time.Second)
	tr = e.Tick(t1, []Rule{rule},
		[]api.HostSnapshot{snap("h1", 0, 90*time.Second, t1)}, nil)
	if len(tr) != 1 || tr[0].State != "firing" {
		t.Fatalf("expected firing on first tick past 60s silence, got %#v", tr)
	}

	// Host reports a fresh sample → resolve.
	t3 := t1.Add(5 * time.Second)
	tr = e.Tick(t3, []Rule{rule},
		[]api.HostSnapshot{snap("h1", 0, 0, t3)}, nil)
	if len(tr) != 1 || tr[0].State != "resolved" {
		t.Fatalf("expected resolved when sample fresh, got %#v", tr)
	}
}

// for_seconds > 0 on offline still adds extra hold on top of the 60s
// detection window. Confirms we didn't accidentally turn it into a no-op.
func TestEvaluate_OfflineRule_ForSecondsAddsHold(t *testing.T) {
	e := newTestEngine()
	rule := Rule{
		ID: 4, Name: "offline-hold", Metric: "offline", Comparator: "gt",
		Threshold: 0, ForSeconds: 30, Host: "h1", Severity: "critical",
		Enabled: true,
	}
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)

	// First breach tick: only sets pendingSince, doesn't fire yet.
	t1 := now.Add(90 * time.Second)
	tr := e.Tick(t1, []Rule{rule},
		[]api.HostSnapshot{snap("h1", 0, 90*time.Second, t1)}, nil)
	if len(tr) != 0 {
		t.Fatalf("expected pending (not yet firing) with for_seconds=30, got %#v", tr)
	}

	// 30s later: hold satisfied, fires.
	t2 := t1.Add(31 * time.Second)
	tr = e.Tick(t2, []Rule{rule},
		[]api.HostSnapshot{snap("h1", 0, t2.Sub(now), t2)}, nil)
	if len(tr) != 1 || tr[0].State != "firing" {
		t.Fatalf("expected firing after for_seconds hold, got %#v", tr)
	}
}

// Offline against a registered host that has NEVER reported fires on
// the first tick — evaluateOne returns breach=true unconditionally.
func TestEvaluate_OfflineRule_NeverReported(t *testing.T) {
	e := newTestEngine()
	rule := Rule{
		ID: 5, Name: "offline-all", Metric: "offline", Comparator: "gt",
		Threshold: 0, ForSeconds: 0, Host: "", Severity: "critical",
		Enabled: true,
	}
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	tr := e.Tick(now, []Rule{rule}, nil, []string{"ghost"})
	if len(tr) != 1 || tr[0].State != "firing" || tr[0].Host != "ghost" {
		t.Fatalf("expected firing for never-reported host on first tick, got %#v", tr)
	}
}

// Host glob pattern matches by prefix; non-matching hosts don't fire.
func TestEvaluate_HostGlob(t *testing.T) {
	e := newTestEngine()
	rule := Rule{
		ID: 10, Name: "web cpu", Metric: "cpu_pct", Comparator: "gt",
		Threshold: 50, ForSeconds: 0, Host: "web-*", Severity: "warning",
		Enabled: true,
	}
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	tr := e.Tick(now, []Rule{rule}, []api.HostSnapshot{
		snap("web-1", 90, 0, now),
		snap("web-2", 91, 0, now),
		snap("db-1", 95, 0, now),
		snap("api", 10, 0, now),
	}, []string{"web-1", "web-2", "db-1", "api"})
	gotHosts := map[string]bool{}
	for _, x := range tr {
		gotHosts[x.Host] = true
	}
	if !gotHosts["web-1"] || !gotHosts["web-2"] {
		t.Fatalf("expected web-1 and web-2 firing, got %#v", tr)
	}
	if gotHosts["db-1"] || gotHosts["api"] {
		t.Fatalf("unexpected non-glob host firing, got %#v", tr)
	}
}

// Host comma-list fires on every listed host (OR), de-duped, glob ok.
func TestEvaluate_HostList(t *testing.T) {
	e := newTestEngine()
	rule := Rule{
		ID: 20, Name: "list pick", Metric: "cpu_pct", Comparator: "gt",
		Threshold: 50, ForSeconds: 0, Host: "agent-1, web-*, db-1",
		Severity: "warning", Enabled: true,
	}
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	tr := e.Tick(now, []Rule{rule}, []api.HostSnapshot{
		snap("agent-1", 90, 0, now),
		snap("web-A", 95, 0, now),
		snap("web-B", 96, 0, now),
		snap("db-1", 99, 0, now),
		snap("other", 10, 0, now),
	}, []string{"agent-1", "web-A", "web-B", "db-1", "other"})
	hits := map[string]bool{}
	for _, x := range tr {
		hits[x.Host] = true
	}
	for _, want := range []string{"agent-1", "web-A", "web-B", "db-1"} {
		if !hits[want] {
			t.Errorf("expected %s firing, got %#v", want, tr)
		}
	}
	if hits["other"] {
		t.Errorf("unexpected 'other' firing, got %#v", tr)
	}
}

// host_selector with tag map filters to matching tagged hosts only.
func TestEvaluate_HostSelector(t *testing.T) {
	e := newTestEngine()
	rule := Rule{
		ID: 21, Name: "tier critical", Metric: "cpu_pct", Comparator: "gt",
		Threshold: 50, ForSeconds: 0, HostSelector: "tier=critical,env=prod",
		Severity: "critical", Enabled: true,
	}
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	tags := map[string]map[string]string{
		"db-prod":  {"tier": "critical", "env": "prod"},
		"api-prod": {"tier": "critical", "env": "prod"},
		"web-prod": {"tier": "warning", "env": "prod"},  // wrong tier
		"db-stg":   {"tier": "critical", "env": "stg"},  // wrong env
		"loose":    {},                                    // no tags
	}
	tr := e.TickWithTags(now, []Rule{rule}, []api.HostSnapshot{
		snap("db-prod", 90, 0, now),
		snap("api-prod", 90, 0, now),
		snap("web-prod", 90, 0, now),
		snap("db-stg", 90, 0, now),
		snap("loose", 90, 0, now),
	}, []string{"db-prod", "api-prod", "web-prod", "db-stg", "loose"}, tags)
	hits := map[string]bool{}
	for _, x := range tr {
		hits[x.Host] = true
	}
	if !hits["db-prod"] || !hits["api-prod"] {
		t.Fatalf("expected db-prod + api-prod firing, got %#v", tr)
	}
	if hits["web-prod"] || hits["db-stg"] || hits["loose"] {
		t.Fatalf("unexpected non-matching host firing, got %#v", tr)
	}
}

// Selector parse: trims whitespace, supports bare keys, AND between pairs.
func TestParseSelector(t *testing.T) {
	cases := []struct {
		raw    string
		expect map[string]string
	}{
		{"", nil},
		{"tier=critical", map[string]string{"tier": "critical"}},
		{"  tier = critical , env = prod  ", map[string]string{"tier": "critical", "env": "prod"}},
		{"bare", map[string]string{"bare": ""}},
		{"a=1,a=2", map[string]string{"a": "2"}}, // later wins
	}
	for _, c := range cases {
		sel, err := ParseSelector(c.raw)
		if err != nil {
			t.Errorf("ParseSelector(%q) failed: %v", c.raw, err)
			continue
		}
		got := map[string]string{}
		for _, r := range sel.Reqs {
			got[r.Key] = r.Value
		}
		if c.expect == nil && len(got) != 0 {
			t.Errorf("expected empty selector for %q, got %v", c.raw, got)
		}
		for k, v := range c.expect {
			if got[k] != v {
				t.Errorf("for %q: expected %s=%s, got %s=%s", c.raw, k, v, k, got[k])
			}
		}
	}
}

// Severity rank is monotonic: critical > warning > info > unknown.
func TestSeverityRank(t *testing.T) {
	if SeverityRank("critical") <= SeverityRank("warning") {
		t.Fatal("critical must outrank warning")
	}
	if SeverityRank("warning") <= SeverityRank("info") {
		t.Fatal("warning must outrank info")
	}
	if SeverityRank("garbage") != SeverityRank("info") {
		t.Fatal("unknown severity should default to info rank")
	}
}

// Comparator 'lt' fires when value drops below threshold.
func TestEvaluate_LtComparator(t *testing.T) {
	e := newTestEngine()
	rule := Rule{
		ID: 6, Name: "disk low", Metric: "disk_pct", Comparator: "lt",
		Threshold: 20, ForSeconds: 0, Enabled: true, Severity: "info",
	}
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	tr := e.Tick(now, []Rule{rule},
		[]api.HostSnapshot{{Host: "h1", Ts: now, DiskPct: 10}}, nil)
	if len(tr) != 1 || tr[0].State != "firing" {
		t.Fatalf("expected firing on lt 20%% with 10%%, got %#v", tr)
	}
}

// TestEvaluate_MaintenanceWindowSuppressesFiring covers issue #33 /
// #35 — the regression where the alerts engine's MaintenanceLister
// wasn't wired into runOnce, so a window in the maintenance cacher
// didn't suppress a firing transition. A host with an active window
// matching its tag scope must see ZERO transitions during the window.
func TestEvaluate_MaintenanceWindowSuppressesFiring(t *testing.T) {
	e := newTestEngine()
	rule := Rule{
		ID: 7, Name: "cpu high", Metric: "cpu_pct", Comparator: "gt",
		Threshold: 50, ForSeconds: 0, Enabled: true, Severity: "warning",
	}
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	wins := map[string][]MaintenanceWindow{
		"host-a": {{
			StartAt:   now.Add(-1 * time.Hour),
			EndAt:     now.Add(1 * time.Hour),
			ScopeTags: map[string]string{}, // empty = matches all
		}},
	}
	tr := e.TickWithTagsAndMaintenance(now, []Rule{rule},
		[]api.HostSnapshot{snap("host-a", 90, 0, now)},
		[]string{"host-a"}, nil, wins)
	if len(tr) != 0 {
		t.Fatalf("expected 0 transitions (suppressed by window), got %d: %+v", len(tr), tr)
	}
}

// TestEvaluate_MaintenanceWindowScopeMismatchKeepsFiring covers the
// inverse: a window whose tag scope doesn't match the host's tags
// must NOT suppress. The alert should still fire.
func TestEvaluate_MaintenanceWindowScopeMismatchKeepsFiring(t *testing.T) {
	e := newTestEngine()
	rule := Rule{
		ID: 8, Name: "cpu high", Metric: "cpu_pct", Comparator: "gt",
		Threshold: 50, ForSeconds: 0, Enabled: true, Severity: "warning",
	}
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	wins := map[string][]MaintenanceWindow{
		"host-a": {{
			StartAt:   now.Add(-1 * time.Hour),
			EndAt:     now.Add(1 * time.Hour),
			ScopeTags: map[string]string{"tier": "prod"}, // doesn't match dev
		}},
	}
	tags := map[string]map[string]string{"host-a": {"tier": "dev"}}
	tr := e.TickWithTagsAndMaintenance(now, []Rule{rule},
		[]api.HostSnapshot{snap("host-a", 90, 0, now)},
		[]string{"host-a"}, tags, wins)
	if len(tr) != 1 || tr[0].State != "firing" {
		t.Fatalf("expected 1 firing (scope mismatch should NOT suppress), got %+v", tr)
	}
}

// TestEvaluate_MaintenanceWindowExpiredAllowsFiring covers the time
// bound: a window that has already ended must NOT suppress — the
// pre-window firing was already dispatched (per RFC 0003 Q1) and any
// new transition should land normally.
func TestEvaluate_MaintenanceWindowExpiredAllowsFiring(t *testing.T) {
	e := newTestEngine()
	rule := Rule{
		ID: 9, Name: "cpu high", Metric: "cpu_pct", Comparator: "gt",
		Threshold: 50, ForSeconds: 0, Enabled: true, Severity: "warning",
	}
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	wins := map[string][]MaintenanceWindow{
		"host-a": {{
			StartAt:   now.Add(-3 * time.Hour),
			EndAt:     now.Add(-1 * time.Hour), // ended an hour ago
			ScopeTags: map[string]string{},
		}},
	}
	tr := e.TickWithTagsAndMaintenance(now, []Rule{rule},
		[]api.HostSnapshot{snap("host-a", 90, 0, now)},
		[]string{"host-a"}, nil, wins)
	if len(tr) != 1 || tr[0].State != "firing" {
		t.Fatalf("expected 1 firing (window expired), got %+v", tr)
	}
}

// TestEvaluate_CooldownExtendsOnFlap covers audit finding C4 —
// the cooldown branch must update lastFiredAt so a sustained flap
// during cooldown doesn't pin the window to the original fire
// instant. Scenario: rule with cooldown=60s, breach-resolve-breach
// pattern where each new breach happens just BEFORE the cooldown
// would elapse.
//
//   t=0  breach → fire (notif, lastFiredAt=t0)
//   t=10 resolve → state.firing=false (but lastFiredAt stays t0)
//   t=20 breach → in-cooldown (20<60), NOT notified, lastFiredAt MUST update to t=20
//   t=30 resolve
//   t=40 breach → t-lastFiredAt=20, 20<60, in-cooldown, lastFiredAt=t=40
//   ...pattern continues. With C4 fix, every in-cooldown breach
//   bumps lastFiredAt, so a flap that NEVER resolves the cooldown
//   can still re-fire after 60s of silence.
//
// Without the fix, lastFiredAt stays at t=0; the cooldown would
// elapse at t=60 and the next breach fires immediately — which
// seems fine, but the silent window has been "carried" past t=60
// by in-cooldown breaches that should have extended it.
func TestEvaluate_CooldownExtendsOnFlap(t *testing.T) {
	e := newTestEngine()
	rule := Rule{
		ID: 10, Name: "cpu hot", Metric: "cpu_pct", Comparator: "gt",
		Threshold: 50, ForSeconds: 0, CooldownSeconds: 60, Enabled: true,
		Severity: "warning",
	}
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)

	// Cycle: breach→in-cooldown→resolve→re-breach→in-cooldown→…
	// Each tick: pick CPU above/below threshold to drive a state
	// transition. The breach (high) ticks are what count for
	// lastFiredAt updates.
	ticks := []struct {
		offsetSec int
		cpu       float64
		wantState string // firing|resolved|""
	}{
		{0, 90, "firing"},   // initial fire, lastFiredAt=0
		{5, 90, ""},         // already firing, no transition
		{10, 5, "resolved"}, // resolve
		{20, 90, ""},       // re-breach but in-cooldown (20s<60s), no notify. WITH fix: lastFiredAt=20.
		{30, 5, "resolved"}, // resolve
		{40, 90, ""},       // re-breach in-cooldown (40-20=20<60s). lastFiredAt=40.
		{50, 5, "resolved"},
		{60, 90, ""},       // 60-40=20s<60s, in-cooldown. lastFiredAt=60.
		{70, 5, "resolved"},
		{80, 90, ""},       // 80-60=20s<60s, in-cooldown. lastFiredAt=80.
		{100, 5, "resolved"},
		{140, 90, "firing"}, // 140-80=60s ≥ 60s, cooldown done — must fire.
	}
	for i, tick := range ticks {
		tAt := now.Add(time.Duration(tick.offsetSec) * time.Second)
		tr := e.Tick(tAt, []Rule{rule},
			[]api.HostSnapshot{snap("h1", tick.cpu, 0, tAt)}, nil)
		switch tick.wantState {
		case "firing":
			if len(tr) != 1 || tr[0].State != "firing" {
				t.Errorf("tick #%d (t=%d, cpu=%.0f): expected firing, got %+v", i, tick.offsetSec, tick.cpu, tr)
			}
		case "resolved":
			if len(tr) != 1 || tr[0].State != "resolved" {
				t.Errorf("tick #%d (t=%d, cpu=%.0f): expected resolved, got %+v", i, tick.offsetSec, tick.cpu, tr)
			}
		case "":
			if len(tr) != 0 {
				t.Errorf("tick #%d (t=%d, cpu=%.0f): expected no transition, got %+v", i, tick.offsetSec, tick.cpu, tr)
			}
		}
	}
	// The final tick (t=140) MUST fire because lastFiredAt was
	// bumped at t=80 (in-cooldown re-breach), and 140-80=60s
	// exactly meets the cooldown threshold. The engine uses
	// <, so 60s elapsed means the condition is false → falls
	// through to the notification branch. If the C4 fix is
	// missing, lastFiredAt stayed at t=0 and the tick at t=60
	// would have fired earlier — but more importantly, the
	// t=140 outcome depends on cumulative in-cooldown bumps
	// having kept the window active through t=80.
}
