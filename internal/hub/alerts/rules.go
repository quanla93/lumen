// Package alerts implements Phase 6 (RFC 0001) threshold alerting:
// operator-defined rules over per-host metrics, a background evaluation
// engine that holds firing/resolved state in memory and persists events,
// and pluggable HTTP notification channels (ntfy/discord/webhook).
//
// Engine state is rebuilt on hub restart from the in-memory snapshot store;
// the alert_events table is the source of truth for the UI/history.
package alerts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Allowed enum values. Kept as map+slice so handlers can both validate
// and list options for the UI without drifting.
var (
	AllowedMetrics = []string{
		"cpu_pct", "ram_pct", "swap_pct", "disk_pct", "load1", "offline",
	}
	AllowedComparators = []string{"gt", "lt"}
	AllowedSeverities  = []string{"info", "warning", "critical"}
)

var (
	ErrRuleNotFound        = errors.New("alert rule not found")
	ErrRuleNameRequired    = errors.New("name required")
	ErrInvalidMetric       = errors.New("invalid metric")
	ErrInvalidComparator   = errors.New("invalid comparator")
	ErrInvalidSeverity     = errors.New("invalid severity")
	ErrNegativeForSeconds  = errors.New("for_seconds must be >= 0")
	ErrNegativeCooldown    = errors.New("cooldown_seconds must be >= 0")
)

// Rule mirrors one row of alert_rules.
//
// Host targeting rules (checked in order; first match wins):
//   1. HostSelector non-empty  → match hosts whose tags satisfy the selector.
//   2. Host non-empty          → comma-separated list of names or globs.
//                                  Each segment may be exact ("web-1") or
//                                  glob ("web-*"); host matches if ANY
//                                  segment matches.
//   3. both empty              → every host (union of registered + ever-seen).
type Rule struct {
	ID           int64
	Name         string
	Metric       string
	Comparator   string
	Threshold    float64
	ForSeconds   int
	// CooldownSeconds is the flap-suppression window: after a firing
	// transition emits, further firing transitions for the same
	// (rule, host) within this many seconds are silently suppressed
	// (no event row, no delivery). 0 = no cooldown (default).
	// Resolved transitions are always emitted regardless of cooldown.
	CooldownSeconds int
	Host         string
	HostSelector string
	Severity     string
	Enabled      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

const ruleColumns = `id, name, metric, comparator, threshold, for_seconds, cooldown_seconds,
	host, host_selector, severity, enabled, created_at, updated_at`

func scanRule(scanner interface{ Scan(dest ...any) error }) (Rule, error) {
	var r Rule
	var host sql.NullString
	var enabled int
	err := scanner.Scan(
		&r.ID, &r.Name, &r.Metric, &r.Comparator, &r.Threshold, &r.ForSeconds, &r.CooldownSeconds,
		&host, &r.HostSelector, &r.Severity, &enabled, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return Rule{}, err
	}
	if host.Valid {
		r.Host = host.String
	}
	r.Enabled = enabled != 0
	return r, nil
}

func validateRule(r *Rule) error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return ErrRuleNameRequired
	}
	if len(r.Name) > 128 {
		return errors.New("name too long (max 128 chars)")
	}
	if !contains(AllowedMetrics, r.Metric) {
		return fmt.Errorf("%w: %q (allowed: %s)", ErrInvalidMetric, r.Metric, strings.Join(AllowedMetrics, ","))
	}
	// 'offline' is a presence check — comparator/threshold are ignored on
	// the engine side, but we still normalize to a known value so the row
	// passes validation if a UI accidentally posts something stale.
	if r.Metric == "offline" {
		r.Comparator = "gt"
		r.Threshold = 0
	} else if !contains(AllowedComparators, r.Comparator) {
		return fmt.Errorf("%w: %q", ErrInvalidComparator, r.Comparator)
	}
	if !contains(AllowedSeverities, r.Severity) {
		return fmt.Errorf("%w: %q", ErrInvalidSeverity, r.Severity)
	}
	if r.ForSeconds < 0 {
		return ErrNegativeForSeconds
	}
	if r.CooldownSeconds < 0 {
		return ErrNegativeCooldown
	}
	r.Host = strings.TrimSpace(r.Host)
	r.HostSelector = strings.TrimSpace(r.HostSelector)
	if r.HostSelector != "" {
		if _, err := ParseSelector(r.HostSelector); err != nil {
			return fmt.Errorf("host_selector: %w", err)
		}
	}
	return nil
}

// ListRules returns every rule, newest-first. Enabled+disabled both come back
// so the UI can flip the toggle without a separate fetch.
func ListRules(ctx context.Context, db *sql.DB) ([]Rule, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT `+ruleColumns+` FROM alert_rules ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Rule, 0)
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListEnabledRules is used by the engine on every tick. Filtering at the
// SQL layer keeps the engine simple and lets disabled rules sit in the
// table without paying for them every cycle.
func ListEnabledRules(ctx context.Context, db *sql.DB) ([]Rule, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT `+ruleColumns+` FROM alert_rules WHERE enabled = 1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Rule, 0)
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func GetRule(ctx context.Context, db *sql.DB, id int64) (Rule, error) {
	r, err := scanRule(db.QueryRowContext(ctx,
		`SELECT `+ruleColumns+` FROM alert_rules WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Rule{}, ErrRuleNotFound
	}
	return r, err
}

func CreateRule(ctx context.Context, db *sql.DB, r Rule) (Rule, error) {
	if err := validateRule(&r); err != nil {
		return Rule{}, err
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO alert_rules
			(name, metric, comparator, threshold, for_seconds, cooldown_seconds, host, host_selector, severity, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Name, r.Metric, r.Comparator, r.Threshold, r.ForSeconds, r.CooldownSeconds,
		nullHost(r.Host), r.HostSelector, r.Severity, boolToInt(r.Enabled),
	)
	if err != nil {
		return Rule{}, fmt.Errorf("insert rule: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Rule{}, err
	}
	return GetRule(ctx, db, id)
}

func UpdateRule(ctx context.Context, db *sql.DB, r Rule) (Rule, error) {
	if r.ID <= 0 {
		return Rule{}, ErrRuleNotFound
	}
	if err := validateRule(&r); err != nil {
		return Rule{}, err
	}
	res, err := db.ExecContext(ctx, `
		UPDATE alert_rules SET
			name = ?, metric = ?, comparator = ?, threshold = ?,
			for_seconds = ?, cooldown_seconds = ?, host = ?, host_selector = ?, severity = ?, enabled = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		r.Name, r.Metric, r.Comparator, r.Threshold, r.ForSeconds, r.CooldownSeconds,
		nullHost(r.Host), r.HostSelector, r.Severity, boolToInt(r.Enabled),
		r.ID,
	)
	if err != nil {
		return Rule{}, fmt.Errorf("update rule: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return Rule{}, err
	}
	if n == 0 {
		return Rule{}, ErrRuleNotFound
	}
	return GetRule(ctx, db, r.ID)
}

// DeleteRule removes a rule and auto-resolves any of its firing events.
// Without the auto-resolve step a deleted rule leaves "ghost" firing
// rows in alert_events that nothing ever closes — the engine no longer
// evaluates the rule, so no resolved transition is ever generated. The
// UI then shows a perpetual incident that can't be cleared without
// touching the DB directly. We close them with a synthetic resolved_at
// = now and let the operator see "Resolved" in the History tab.
func DeleteRule(ctx context.Context, db *sql.DB, id int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Snapshot the firing rows so we can also delete their pending
	// deliveries — there's no point retrying notifications for a rule
	// the operator just deleted.
	if _, err := tx.ExecContext(ctx, `
		UPDATE alert_events
		SET state = 'resolved', resolved_at = CURRENT_TIMESTAMP
		WHERE rule_id = ? AND state = 'firing'`, id); err != nil {
		return fmt.Errorf("auto-resolve events: %w", err)
	}
	// Drop pending/in-flight deliveries from rows that match this rule's
	// events. They'd otherwise keep retrying against channels the
	// operator clearly stopped caring about.
	if _, err := tx.ExecContext(ctx, `
		UPDATE notification_deliveries
		SET status = 'dropped', error = 'rule deleted', next_retry_at = NULL
		WHERE status IN ('pending', 'inflight')
		  AND event_id IN (SELECT id FROM alert_events WHERE rule_id = ?)`, id); err != nil {
		return fmt.Errorf("drop deliveries: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM alert_rules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrRuleNotFound
	}
	return tx.Commit()
}

func nullHost(s string) sql.NullString {
	s = strings.TrimSpace(s)
	return sql.NullString{String: s, Valid: s != ""}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
