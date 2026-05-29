package alerts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/quanla93/lumen/internal/hub/tagutil"
)

// Tag is one inventory entry — a key plus the closed list of values it
// accepts. HostCount / RuleCount are aggregates included in the list
// view so the operator can see usage at a glance.
type Tag struct {
	Key         string   `json:"key"`
	Description string   `json:"description"`
	Values      []string `json:"values"`
	HostCount   int      `json:"host_count"`
	RuleCount   int      `json:"rule_count"`
}

// TagImpact is the dry-run payload for "what happens if I delete this".
// Returned by GET /api/tags/{key}/impact and rendered in the confirm
// dialog so the operator can't be surprised by silent fan-out.
type TagImpact struct {
	HostCount  int     `json:"host_count"`
	RuleCount  int     `json:"rule_count"`
	RuleNames  []string `json:"rule_names"`
}

// ValueImpact is the dry-run payload for deleting a single value.
type ValueImpact struct {
	HostCount int      `json:"host_count"`
	RuleCount int      `json:"rule_count"`
	RuleNames []string `json:"rule_names"`
}

var (
	ErrTagNotFound      = errors.New("tag not found")
	ErrTagKeyExists     = errors.New("tag with that key already exists")
	ErrTagValueExists   = errors.New("value already in tag")
	ErrTagValueNotFound = errors.New("value not found on tag")
	ErrTagValueLastInUse = errors.New("cannot drop the last value while it's still assigned to hosts")
)

// ListTags returns every inventory tag, alphabetised, with usage counts.
// Two follow-up queries (host count, rule LIKE scan) are O(rows); fine
// at the scale tags exist (dozens, not thousands).
func ListTags(ctx context.Context, db *sql.DB) ([]Tag, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT key, description FROM tags ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type row struct {
		key, desc string
	}
	var base []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.key, &r.desc); err != nil {
			return nil, err
		}
		base = append(base, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]Tag, 0, len(base))
	for _, b := range base {
		t := Tag{Key: b.key, Description: b.desc, Values: []string{}}
		values, err := listTagValues(ctx, db, b.key)
		if err != nil {
			return nil, err
		}
		t.Values = values
		hostCount, ruleCount, _, err := tagImpact(ctx, db, b.key)
		if err != nil {
			return nil, err
		}
		t.HostCount = hostCount
		t.RuleCount = ruleCount
		out = append(out, t)
	}
	return out, nil
}

// GetTag reads a single inventory tag. Returns ErrTagNotFound if absent.
func GetTag(ctx context.Context, db *sql.DB, key string) (Tag, error) {
	var t Tag
	err := db.QueryRowContext(ctx,
		`SELECT key, description FROM tags WHERE key = ?`, key,
	).Scan(&t.Key, &t.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return Tag{}, ErrTagNotFound
	}
	if err != nil {
		return Tag{}, err
	}
	values, err := listTagValues(ctx, db, key)
	if err != nil {
		return Tag{}, err
	}
	t.Values = values
	hostCount, ruleCount, _, err := tagImpact(ctx, db, key)
	if err != nil {
		return Tag{}, err
	}
	t.HostCount = hostCount
	t.RuleCount = ruleCount
	return t, nil
}

// CreateTag inserts the tag and its initial values atomically. Values
// may be empty; duplicates within the input are silently deduped.
func CreateTag(ctx context.Context, db *sql.DB, key, desc string, values []string) error {
	key = tagutil.NormalizeKey(key)
	if err := tagutil.ValidateKey(key); err != nil {
		return err
	}
	cleaned, err := cleanValues(values)
	if err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO tags (key, description) VALUES (?, ?)`, key, desc)
	if err != nil {
		// SQLite returns a generic constraint error; normalise to the
		// sentinel so the handler can return 409.
		if isUniqueConstraintErr(err) {
			return ErrTagKeyExists
		}
		return err
	}
	for _, v := range cleaned {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tag_values (tag_key, value) VALUES (?, ?)`, key, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpdateTag changes the description only. Rename of key is intentionally
// out of scope for v1 — hosts/rules reference the key, and an atomic
// rename across all three tables plus selector rewrites is its own ticket.
func UpdateTag(ctx context.Context, db *sql.DB, key, desc string) error {
	res, err := db.ExecContext(ctx,
		`UPDATE tags SET description = ?, updated_at = CURRENT_TIMESTAMP WHERE key = ?`,
		desc, key)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrTagNotFound
	}
	return nil
}

// DeleteTag cascades: removes the tag from every host_tags row, rewrites
// every alert_rules.host_selector to drop the key, and deletes the tag
// (cascading tag_values). Single transaction.
func DeleteTag(ctx context.Context, db *sql.DB, key string) (TagImpact, error) {
	var impact TagImpact
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return impact, err
	}
	defer tx.Rollback()

	// Existence check first so DELETE on a missing tag is a 404, not a no-op 200.
	var exists int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM tags WHERE key = ?`, key).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return impact, ErrTagNotFound
	}
	if err != nil {
		return impact, err
	}

	hostCount, ruleCount, ruleNames, err := tagImpactTx(ctx, tx, key)
	if err != nil {
		return impact, err
	}
	impact = TagImpact{HostCount: hostCount, RuleCount: ruleCount, RuleNames: ruleNames}

	if err := rewriteRuleSelectors(ctx, tx, key, "", true); err != nil {
		return impact, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM host_tags WHERE key = ?`, key); err != nil {
		return impact, fmt.Errorf("delete host_tags: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tags WHERE key = ?`, key); err != nil {
		return impact, fmt.Errorf("delete tag: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return impact, err
	}
	return impact, nil
}

// AddValue appends a new value to the tag. Returns ErrTagValueExists on
// duplicate (handler maps to 409) and ErrTagNotFound if the key is gone.
func AddValue(ctx context.Context, db *sql.DB, key, value string) error {
	value = tagutil.NormalizeValue(value)
	if err := tagutil.ValidateValue(value); err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM tags WHERE key = ?`, key).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTagNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO tag_values (tag_key, value) VALUES (?, ?)`, key, value); err != nil {
		if isUniqueConstraintErr(err) {
			return ErrTagValueExists
		}
		return err
	}
	return tx.Commit()
}

// DeleteValue cascades: removes the (key, value) pair from host_tags and
// rewrites any rule selector that pins this exact value. Other values of
// the same key are untouched (this is value-level, not tag-level).
func DeleteValue(ctx context.Context, db *sql.DB, key, value string) (ValueImpact, error) {
	var impact ValueImpact
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return impact, err
	}
	defer tx.Rollback()

	var exists int
	err = tx.QueryRowContext(ctx,
		`SELECT 1 FROM tag_values WHERE tag_key = ? AND value = ?`, key, value,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return impact, ErrTagValueNotFound
	}
	if err != nil {
		return impact, err
	}

	hostCount, ruleCount, ruleNames, err := valueImpactTx(ctx, tx, key, value)
	if err != nil {
		return impact, err
	}
	impact = ValueImpact{HostCount: hostCount, RuleCount: ruleCount, RuleNames: ruleNames}

	if err := rewriteRuleSelectors(ctx, tx, key, value, false); err != nil {
		return impact, err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM host_tags WHERE key = ? AND value = ?`, key, value); err != nil {
		return impact, fmt.Errorf("delete host_tags: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM tag_values WHERE tag_key = ? AND value = ?`, key, value); err != nil {
		return impact, fmt.Errorf("delete tag_value: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return impact, err
	}
	return impact, nil
}

// TagImpactPreview is the read-only sibling of DeleteTag. The confirm
// dialog calls this so the operator sees blast radius before acting.
func TagImpactPreview(ctx context.Context, db *sql.DB, key string) (TagImpact, error) {
	hostCount, ruleCount, ruleNames, err := tagImpact(ctx, db, key)
	if err != nil {
		return TagImpact{}, err
	}
	return TagImpact{HostCount: hostCount, RuleCount: ruleCount, RuleNames: ruleNames}, nil
}

// ValueImpactPreview is the read-only sibling of DeleteValue.
func ValueImpactPreview(ctx context.Context, db *sql.DB, key, value string) (ValueImpact, error) {
	hostCount, ruleCount, ruleNames, err := valueImpact(ctx, db, key, value)
	if err != nil {
		return ValueImpact{}, err
	}
	return ValueImpact{HostCount: hostCount, RuleCount: ruleCount, RuleNames: ruleNames}, nil
}

// --- helpers -----------------------------------------------------------

func listTagValues(ctx context.Context, db *sql.DB, key string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT value FROM tag_values WHERE tag_key = ? ORDER BY value`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func cleanValues(in []string) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = tagutil.NormalizeValue(v)
		if err := tagutil.ValidateValue(v); err != nil {
			return nil, err
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out, nil
}

// tagImpact reads usage counts for the confirm dialog. Two queries: one
// for hosts, one for rules. Rule selector matching is done by parsing
// each candidate (LIKE on key narrows the scan first).
func tagImpact(ctx context.Context, db *sql.DB, key string) (int, int, []string, error) {
	return tagImpactQuery(ctx, db, key)
}
func tagImpactTx(ctx context.Context, tx *sql.Tx, key string) (int, int, []string, error) {
	return tagImpactQuery(ctx, tx, key)
}

// querier abstracts *sql.DB and *sql.Tx so the impact helpers work in
// both read-only and transactional contexts.
type querier interface {
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

func tagImpactQuery(ctx context.Context, q querier, key string) (int, int, []string, error) {
	var hostCount int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT host_id) FROM host_tags WHERE key = ?`, key,
	).Scan(&hostCount)
	if err != nil {
		return 0, 0, nil, err
	}

	rows, err := q.QueryContext(ctx,
		`SELECT id, name, host_selector FROM alert_rules
		 WHERE host_selector LIKE '%' || ? || '%'`, key)
	if err != nil {
		return 0, 0, nil, err
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var id int64
		var name, sel string
		if err := rows.Scan(&id, &name, &sel); err != nil {
			return 0, 0, nil, err
		}
		// LIKE is a coarse filter — "tier" matches "tiered=x". Confirm by parsing.
		parsed, perr := ParseSelector(sel)
		if perr != nil {
			continue
		}
		hit := false
		for _, req := range parsed.Reqs {
			if req.Key == key {
				hit = true
				break
			}
		}
		if hit {
			names = append(names, name)
		}
	}
	return hostCount, len(names), names, rows.Err()
}

func valueImpact(ctx context.Context, db *sql.DB, key, value string) (int, int, []string, error) {
	return valueImpactQuery(ctx, db, key, value)
}
func valueImpactTx(ctx context.Context, tx *sql.Tx, key, value string) (int, int, []string, error) {
	return valueImpactQuery(ctx, tx, key, value)
}
func valueImpactQuery(ctx context.Context, q querier, key, value string) (int, int, []string, error) {
	var hostCount int
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT host_id) FROM host_tags WHERE key = ? AND value = ?`,
		key, value,
	).Scan(&hostCount)
	if err != nil {
		return 0, 0, nil, err
	}

	rows, err := q.QueryContext(ctx,
		`SELECT id, name, host_selector FROM alert_rules
		 WHERE host_selector LIKE '%' || ? || '%'`, key)
	if err != nil {
		return 0, 0, nil, err
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var id int64
		var name, sel string
		if err := rows.Scan(&id, &name, &sel); err != nil {
			return 0, 0, nil, err
		}
		parsed, perr := ParseSelector(sel)
		if perr != nil {
			continue
		}
		for _, req := range parsed.Reqs {
			if req.Key == key && req.Value == value {
				names = append(names, name)
				break
			}
		}
	}
	return hostCount, len(names), names, rows.Err()
}

// rewriteRuleSelectors loads every rule whose host_selector references
// key, parses it, drops the matching requirement(s), and persists the
// canonical form. dropKey=true means drop any requirement on this key
// regardless of value (tag-level delete); dropKey=false means drop only
// the exact (key, value) pair (value-level delete).
func rewriteRuleSelectors(ctx context.Context, tx *sql.Tx, key, value string, dropKey bool) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, host_selector FROM alert_rules
		 WHERE host_selector LIKE '%' || ? || '%'`, key)
	if err != nil {
		return err
	}
	type pending struct {
		id  int64
		sel string
	}
	var updates []pending
	for rows.Next() {
		var id int64
		var sel string
		if err := rows.Scan(&id, &sel); err != nil {
			rows.Close()
			return err
		}
		parsed, perr := ParseSelector(sel)
		if perr != nil {
			// Skip selectors we can't parse — they're already broken; the
			// rule edit UI will flag them next time.
			continue
		}
		var changed bool
		if dropKey {
			changed = parsed.DropKey(key)
		} else {
			changed = parsed.DropPair(key, value)
		}
		if !changed {
			continue
		}
		updates = append(updates, pending{id: id, sel: parsed.String()})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	if len(updates) == 0 {
		return nil
	}
	stmt, err := tx.PrepareContext(ctx,
		`UPDATE alert_rules SET host_selector = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, u := range updates {
		if _, err := stmt.ExecContext(ctx, u.sel, u.id); err != nil {
			return fmt.Errorf("rewrite rule %d selector: %w", u.id, err)
		}
	}
	return nil
}

// isUniqueConstraintErr detects SQLite's UNIQUE constraint violation
// without coupling to the modernc driver's concrete error type — string
// match works across both modernc and mattn drivers.
func isUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "constraint failed: UNIQUE")
}
