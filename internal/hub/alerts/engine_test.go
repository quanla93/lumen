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

// 'offline' rule fires when last-seen exceeds the MinOfflineFor floor.
// for_seconds smaller than 60s is clamped up to 60s.
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

	// 90s age > MinOfflineFor (60s): breach + immediate fire (for clamps
	// to 60s but pendingSince also got bumped, so we need to advance time).
	// First call: pending set.
	t1 := now.Add(90 * time.Second)
	tr = e.Tick(t1, []Rule{rule},
		[]api.HostSnapshot{snap("h1", 0, 90*time.Second, t1)}, nil)
	if len(tr) != 0 {
		t.Fatalf("expected pending (not yet firing) at first breach, got %#v", tr)
	}

	// Advance past clamp window.
	t2 := t1.Add(MinOfflineFor + time.Second)
	tr = e.Tick(t2, []Rule{rule},
		[]api.HostSnapshot{snap("h1", 0, t2.Sub(now), t2)}, nil)
	if len(tr) != 1 || tr[0].State != "firing" {
		t.Fatalf("expected firing after clamp window, got %#v", tr)
	}

	// Host reports a fresh sample → resolve.
	t3 := t2.Add(5 * time.Second)
	tr = e.Tick(t3, []Rule{rule},
		[]api.HostSnapshot{snap("h1", 0, 0, t3)}, nil)
	if len(tr) != 1 || tr[0].State != "resolved" {
		t.Fatalf("expected resolved when sample fresh, got %#v", tr)
	}
}

// Offline against a registered host that has NEVER reported still fires.
func TestEvaluate_OfflineRule_NeverReported(t *testing.T) {
	e := newTestEngine()
	rule := Rule{
		ID: 5, Name: "offline-all", Metric: "offline", Comparator: "gt",
		Threshold: 0, ForSeconds: 0, Host: "", Severity: "critical",
		Enabled: true,
	}
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	// First tick: pending.
	tr := e.Tick(now, []Rule{rule}, nil, []string{"ghost"})
	if len(tr) != 0 {
		t.Fatalf("expected pending first tick, got %#v", tr)
	}
	// After clamp window: fire.
	tr = e.Tick(now.Add(MinOfflineFor+time.Second), []Rule{rule},
		nil, []string{"ghost"})
	if len(tr) != 1 || tr[0].State != "firing" || tr[0].Host != "ghost" {
		t.Fatalf("expected firing for never-reported host, got %#v", tr)
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
