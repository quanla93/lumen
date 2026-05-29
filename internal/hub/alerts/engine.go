package alerts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/quanla93/lumen/internal/hub/hosts"
	"github.com/quanla93/lumen/internal/hub/settings"
	"github.com/quanla93/lumen/internal/shared/api"
)

// MinOfflineFor is the silence window before a host is considered
// offline. The agent ships every ~5s by default; 60s is roughly 12
// missed ticks — comfortably past transient network blips. This is the
// only "ignore blips" floor; we do NOT also clamp the rule's
// for_seconds on top, so for_seconds=0 means "fire as soon as breach
// is detected" (i.e. at T=60s after silence).
const MinOfflineFor = 60 * time.Second

// SnapshotProvider is what the engine reads each tick. Implemented by
// *hub/store.Store; the test substitutes a fake.
type SnapshotProvider interface {
	Snapshot() []api.HostSnapshot
}

// HostsLister returns the names of every registered host. Used only by
// the 'offline' rule so we can fire on hosts that have NEVER reported
// (and aren't in the in-memory snapshot at all).
type HostsLister func(ctx context.Context) ([]string, error)

// TagsLister returns the full map of host_name → tag set. Engine calls
// it once per tick when at least one enabled rule has a non-empty
// host_selector; an implementation that doesn't have tags can return nil.
type TagsLister func(ctx context.Context) (map[string]map[string]string, error)

// Config wires the engine into the rest of the hub. DefaultInterval is the
// fallback eval cadence when the settings row is missing/unparseable.
type Config struct {
	DB              *sql.DB
	Store           SnapshotProvider
	Hosts           HostsLister
	Tags            TagsLister
	DefaultInterval time.Duration
	// Dispatcher persists outbound notifications and ships them on its
	// own schedule. nil → fall back to inline Dispatch (legacy path,
	// kept for the in-process tests that don't want a real DB).
	Dispatcher *Dispatcher
	Logger     *slog.Logger
}

// stateKey is per-(rule, host). For "all hosts" rules each host gets its
// own state entry (a CPU breach on hostA doesn't fire hostB).
type stateKey struct {
	RuleID int64
	Host   string
}

type ruleState struct {
	pendingSince time.Time
	firing       bool
	eventID      int64
	// rule snapshot at last fire — used so the resolve message can
	// reference the threshold even if the operator changed the rule.
	lastSeverity  string
	lastMetric    string
	lastRuleName  string
	lastThreshold float64
}

// Engine holds in-memory state that is rebuilt across hub restarts.
// Firing alerts re-detect within a tick after restart (the breach
// condition is still true), so the only thing lost is the original
// `pendingSince` — acceptable for a homelab tool.
type Engine struct {
	cfg   Config
	mu    sync.Mutex
	state map[stateKey]*ruleState
}

func NewEngine(cfg Config) *Engine {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Engine{cfg: cfg, state: make(map[stateKey]*ruleState)}
}

// Run is the long-lived ticker. Reads eval interval from settings every
// loop (heartbeat pattern, like retention) so a UI change applies fast.
func (e *Engine) Run(ctx context.Context) {
	logger := e.cfg.Logger
	logger.Info("alerts engine starting", "default_interval", e.cfg.DefaultInterval)

	// One-shot cleanup at boot: resolve any 'firing' events whose rule
	// no longer exists. These are ghosts from rules deleted before the
	// auto-resolve path in DeleteRule (Milestone D) landed, or from a
	// hub crash mid-delete. Without this sweep they stay firing forever
	// because nothing evaluates them.
	if e.cfg.DB != nil {
		res, err := e.cfg.DB.ExecContext(ctx, `
			UPDATE alert_events
			SET state = 'resolved', resolved_at = CURRENT_TIMESTAMP
			WHERE state = 'firing'
			  AND rule_id NOT IN (SELECT id FROM alert_rules)`)
		if err != nil {
			logger.Warn("alerts: ghost sweep failed", "err", err)
		} else if n, _ := res.RowsAffected(); n > 0 {
			logger.Info("alerts: resolved orphan firing events at boot", "count", n)
		}
	}

	// First evaluation eagerly — avoids waiting one interval after boot.
	e.runOnce(ctx)

	t := time.NewTicker(e.readInterval(ctx))
	defer t.Stop()
	current := e.readInterval(ctx)

	for {
		select {
		case <-ctx.Done():
			logger.Info("alerts engine stopped")
			return
		case <-t.C:
		}
		next := e.readInterval(ctx)
		if next != current {
			logger.Info("alerts eval interval changed", "old", current, "new", next)
			t.Reset(next)
			current = next
		}
		e.runOnce(ctx)
	}
}

func (e *Engine) readInterval(ctx context.Context) time.Duration {
	d, err := settings.GetDuration(ctx, e.cfg.DB,
		settings.KeyAlertEvalInterval, e.cfg.DefaultInterval)
	if err != nil {
		e.cfg.Logger.Warn("alerts: read eval interval failed", "err", err,
			"fallback", e.cfg.DefaultInterval)
		return e.cfg.DefaultInterval
	}
	if d <= 0 {
		return e.cfg.DefaultInterval
	}
	return d
}

// runOnce performs one evaluation cycle. Exposed via Tick for tests.
func (e *Engine) runOnce(ctx context.Context) {
	rules, err := ListEnabledRules(ctx, e.cfg.DB)
	if err != nil {
		e.cfg.Logger.Error("alerts: list rules failed", "err", err)
		return
	}
	registered, err := e.cfg.Hosts(ctx)
	if err != nil {
		e.cfg.Logger.Error("alerts: list hosts failed", "err", err)
		return
	}
	snap := e.cfg.Store.Snapshot()
	// Only load tags when at least one enabled rule uses a selector —
	// keeps the SELECT off the hot path for fleets that don't tag.
	var tagSet map[string]map[string]string
	if e.cfg.Tags != nil && anySelectorUsed(rules) {
		tagSet, err = e.cfg.Tags(ctx)
		if err != nil {
			e.cfg.Logger.Error("alerts: list host tags failed", "err", err)
			// continue with nil — selector-using rules will match nothing
		}
	}

	transitions := e.evaluate(time.Now(), rules, snap, registered, tagSet)
	if len(transitions) == 0 {
		return
	}
	for _, tr := range transitions {
		// Per-rule routing: fall back to "all enabled channels" if no
		// explicit links exist (Milestone-A compat). Looked up per
		// transition so a routing edit applies on the next event without
		// a restart.
		channels, err := ChannelsForRule(ctx, e.cfg.DB, tr.Rule.ID)
		if err != nil {
			e.cfg.Logger.Error("alerts: list rule channels failed",
				"rule_id", tr.Rule.ID, "err", err)
		}
		e.persistAndNotify(ctx, tr, channels)
	}
}

// Tick is the seam tests drive: pure-ish step that returns the
// transitions a real cycle would persist/notify on, without touching
// the DB or network. Engine state is still mutated.
func (e *Engine) Tick(now time.Time, rules []Rule, snap []api.HostSnapshot, registered []string) []Transition {
	return e.evaluate(now, rules, snap, registered, nil)
}

// TickWithTags is like Tick but also passes a host→tags map for rules
// with non-empty host_selector. Tests that exercise tag selection use
// this; the legacy Tick keeps behaviour identical for older tests.
func (e *Engine) TickWithTags(now time.Time, rules []Rule, snap []api.HostSnapshot, registered []string, tags map[string]map[string]string) []Transition {
	return e.evaluate(now, rules, snap, registered, tags)
}

func anySelectorUsed(rules []Rule) bool {
	for _, r := range rules {
		if r.HostSelector != "" {
			return true
		}
	}
	return false
}

// Transition is one (rule, host) firing or resolving on this tick.
type Transition struct {
	Rule      Rule
	Host      string
	State     string // firing|resolved
	Value     float64
	Threshold float64
	Now       time.Time
}

// evaluate is the state machine — pure given inputs + the existing engine
// map. Mutates e.state but does no IO. Test friendly.
func (e *Engine) evaluate(now time.Time, rules []Rule, snap []api.HostSnapshot, registered []string, tagSet map[string]map[string]string) []Transition {
	e.mu.Lock()
	defer e.mu.Unlock()

	byHost := map[string]api.HostSnapshot{}
	for _, s := range snap {
		byHost[s.Host] = s
	}

	out := make([]Transition, 0)
	for _, r := range rules {
		targets := targetHosts(r, byHost, registered, tagSet)
		for _, host := range targets {
			key := stateKey{RuleID: r.ID, Host: host}
			st, ok := e.state[key]
			if !ok {
				st = &ruleState{}
				e.state[key] = st
			}

			breach, value := evaluateOne(r, host, byHost, now)
			forDur := time.Duration(r.ForSeconds) * time.Second

			switch {
			case breach && !st.firing:
				if st.pendingSince.IsZero() {
					st.pendingSince = now
				}
				if now.Sub(st.pendingSince) >= forDur {
					st.firing = true
					st.lastSeverity = r.Severity
					st.lastMetric = r.Metric
					st.lastRuleName = r.Name
					st.lastThreshold = r.Threshold
					out = append(out, Transition{
						Rule: r, Host: host, State: "firing",
						Value: value, Threshold: r.Threshold, Now: now,
					})
				}
			case !breach && st.firing:
				st.firing = false
				st.pendingSince = time.Time{}
				out = append(out, Transition{
					Rule: r, Host: host, State: "resolved",
					Value: value, Threshold: r.Threshold, Now: now,
				})
			case !breach && !st.firing:
				st.pendingSince = time.Time{}
			}
		}
	}
	return out
}

func targetHosts(r Rule, byHost map[string]api.HostSnapshot, registered []string, tagSet map[string]map[string]string) []string {
	// 1. Selector wins when set. Filter the union by tag set; an empty
	//    tagSet (caller passed nil or load failed) → selector matches
	//    nothing, which is intentional ("don't fire for tier=critical
	//    on hosts you haven't tagged yet").
	if r.HostSelector != "" {
		sel, err := ParseSelector(r.HostSelector)
		if err != nil || sel.IsEmpty() {
			return nil
		}
		out := make([]string, 0)
		for _, h := range unionHosts(byHost, registered) {
			if sel.Matches(tagSet[h]) {
				out = append(out, h)
			}
		}
		return out
	}
	// 2. Empty host → all hosts (union of registered + ever-seen so
	//    offline rules can fire on hosts that never connected).
	if r.Host == "" {
		return unionHosts(byHost, registered)
	}
	// 3. Comma list → OR over each segment. Each segment may itself be
	//    exact or glob. Whitespace around commas tolerated.
	if strings.Contains(r.Host, ",") {
		seenOut := map[string]struct{}{}
		out := make([]string, 0)
		for _, seg := range strings.Split(r.Host, ",") {
			seg = strings.TrimSpace(seg)
			if seg == "" {
				continue
			}
			for _, h := range matchSegment(seg, byHost, registered) {
				if _, dup := seenOut[h]; dup {
					continue
				}
				seenOut[h] = struct{}{}
				out = append(out, h)
			}
		}
		return out
	}
	// 4. Single segment — exact or glob.
	return matchSegment(r.Host, byHost, registered)
}

// matchSegment resolves one host-list segment (exact name or glob) to
// the set of hosts it covers.
func matchSegment(seg string, byHost map[string]api.HostSnapshot, registered []string) []string {
	if !strings.ContainsAny(seg, "*?[") {
		// Exact name. Pass through even if absent from snapshot so the
		// 'offline' rule fires on hosts that never reported.
		return []string{seg}
	}
	out := make([]string, 0)
	for _, h := range unionHosts(byHost, registered) {
		if ok, _ := path.Match(seg, h); ok {
			out = append(out, h)
		}
	}
	return out
}

func unionHosts(byHost map[string]api.HostSnapshot, registered []string) []string {
	seen := map[string]struct{}{}
	for _, h := range registered {
		seen[h] = struct{}{}
	}
	for h := range byHost {
		seen[h] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for h := range seen {
		out = append(out, h)
	}
	return out
}

// evaluateOne returns (breach, value). For 'offline' the value is the
// "seconds since last seen" so it's still meaningful in the message.
func evaluateOne(r Rule, host string, byHost map[string]api.HostSnapshot, now time.Time) (bool, float64) {
	snap, present := byHost[host]
	if r.Metric == "offline" {
		if !present {
			return true, -1
		}
		age := now.Sub(snap.Ts).Seconds()
		return age >= MinOfflineFor.Seconds(), age
	}
	if !present {
		// Can't evaluate a metric rule for a host we have no snapshot of —
		// the 'offline' rule is the right tool for that.
		return false, 0
	}
	v := metricValue(r.Metric, snap)
	return compare(v, r.Comparator, r.Threshold), v
}

func metricValue(metric string, s api.HostSnapshot) float64 {
	switch metric {
	case "cpu_pct":
		return s.CpuPct
	case "ram_pct":
		return s.RamPct
	case "swap_pct":
		return s.SwapPct
	case "disk_pct":
		return s.DiskPct
	case "load1":
		return s.Load1
	}
	return 0
}

func compare(value float64, comparator string, threshold float64) bool {
	switch comparator {
	case "gt":
		return value > threshold
	case "lt":
		return value < threshold
	}
	return false
}

// persistAndNotify writes the event row and dispatches to every enabled
// channel. Best-effort: a single channel failure logs at Warn and does
// not abort the others.
func (e *Engine) persistAndNotify(ctx context.Context, tr Transition, channels []Channel) {
	notif := Notification{
		RuleID: tr.Rule.ID, RuleName: tr.Rule.Name, Host: tr.Host,
		Metric: tr.Rule.Metric, Severity: tr.Rule.Severity, State: tr.State,
		Value: tr.Value, Threshold: tr.Threshold, Time: tr.Now,
	}
	notif.Message = FormatMessage(notif)

	if tr.State == "firing" {
		id, err := insertFiringEvent(ctx, e.cfg.DB, notif)
		if err != nil {
			e.cfg.Logger.Error("alerts: persist firing event failed",
				"rule", tr.Rule.Name, "host", tr.Host, "err", err)
		} else {
			e.mu.Lock()
			if st, ok := e.state[stateKey{RuleID: tr.Rule.ID, Host: tr.Host}]; ok {
				st.eventID = id
			}
			e.mu.Unlock()
		}
	} else {
		e.mu.Lock()
		eventID := int64(0)
		if st, ok := e.state[stateKey{RuleID: tr.Rule.ID, Host: tr.Host}]; ok {
			eventID = st.eventID
			st.eventID = 0
		}
		e.mu.Unlock()
		if eventID > 0 {
			if err := markResolved(ctx, e.cfg.DB, eventID, tr.Now); err != nil {
				e.cfg.Logger.Error("alerts: mark resolved failed",
					"event_id", eventID, "err", err)
			}
		}
	}

	eventID := int64(0)
	e.mu.Lock()
	if st, ok := e.state[stateKey{RuleID: tr.Rule.ID, Host: tr.Host}]; ok {
		eventID = st.eventID
	}
	e.mu.Unlock()

	eventRank := SeverityRank(notif.Severity)
	for _, ch := range channels {
		// Channel-level severity floor. A pager set to `critical` should
		// not get woken for `warning` events.
		if eventRank < SeverityRank(ch.MinSeverity) {
			e.cfg.Logger.Debug("alerts: skip channel below min_severity",
				"channel", ch.Name, "event_severity", notif.Severity,
				"min_severity", ch.MinSeverity)
			continue
		}
		if e.cfg.Dispatcher != nil && eventID > 0 {
			// Async path (production): enqueue + return immediately.
			// The dispatcher's workers carry it out, retry, and persist
			// every attempt in notification_deliveries.
			if _, err := e.cfg.Dispatcher.Enqueue(ctx, eventID, ch, notif); err != nil {
				e.cfg.Logger.Error("alerts: enqueue delivery failed",
					"channel", ch.Name, "err", err)
				continue
			}
			e.cfg.Logger.Debug("alerts: enqueued",
				"channel", ch.Name, "type", ch.Type,
				"rule", tr.Rule.Name, "host", tr.Host, "state", tr.State)
			continue
		}
		// Legacy synchronous fallback — only when no dispatcher wired
		// (engine_test, or pre-Milestone-D unit harnesses).
		if err := Dispatch(ctx, ch, notif, e.cfg.Logger); err != nil {
			e.cfg.Logger.Warn("alerts: dispatch failed",
				"channel", ch.Name, "type", ch.Type, "err", err)
			continue
		}
		e.cfg.Logger.Info("alerts: notification sent",
			"channel", ch.Name, "type", ch.Type,
			"rule", tr.Rule.Name, "host", tr.Host, "state", tr.State)
	}
}

func insertFiringEvent(ctx context.Context, db *sql.DB, n Notification) (int64, error) {
	res, err := db.ExecContext(ctx, `
		INSERT INTO alert_events
			(rule_id, rule_name, host, metric, severity, state, value, message)
		VALUES (?, ?, ?, ?, ?, 'firing', ?, ?)`,
		n.RuleID, n.RuleName, n.Host, n.Metric, n.Severity, n.Value, n.Message,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func markResolved(ctx context.Context, db *sql.DB, eventID int64, now time.Time) error {
	res, err := db.ExecContext(ctx, `
		UPDATE alert_events
		SET state = 'resolved', resolved_at = ?
		WHERE id = ? AND state = 'firing'`,
		now.UTC(), eventID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("event %d not in firing state", eventID)
	}
	return nil
}

// HostsListerFromDB wraps the hosts package so the engine can use it
// without a circular import in tests.
func HostsListerFromDB(db *sql.DB) HostsLister {
	return func(ctx context.Context) ([]string, error) {
		all, err := hosts.List(ctx, db)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(all))
		for _, h := range all {
			out = append(out, h.Name)
		}
		return out, nil
	}
}

// TagsListerFromDB wraps hosts.AllHostTags so the engine can read the
// host_tags table without circular imports in tests.
func TagsListerFromDB(db *sql.DB) TagsLister {
	return func(ctx context.Context) (map[string]map[string]string, error) {
		return hosts.AllHostTags(ctx, db)
	}
}

// ErrNoEventToResolve is exported so callers can ignore it cleanly.
var ErrNoEventToResolve = errors.New("no firing event to resolve")
