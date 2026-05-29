package hosts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Tag is one host_tags row. Keys are short labels (no embedded `=` or
// `,` — those are reserved by the selector grammar). Empty value is
// allowed and represents "bare tag" semantics ("prod" with no value).
type Tag struct {
	Key   string
	Value string
}

var (
	ErrTagKeyRequired   = errors.New("tag key required")
	ErrTagKeyTooLong    = errors.New("tag key too long (max 64 chars)")
	ErrTagValueTooLong  = errors.New("tag value too long (max 128 chars)")
	ErrTagKeyInvalid    = errors.New("tag key may only contain letters, digits, '-', '_', '.'")
	ErrTagValueInvalid  = errors.New("tag value contains reserved chars (',' '=')")
	ErrTooManyTags      = errors.New("too many tags on a single host (max 32)")
)

const maxTagsPerHost = 32

func validateTag(t Tag) (Tag, error) {
	t.Key = strings.TrimSpace(t.Key)
	t.Value = strings.TrimSpace(t.Value)
	if t.Key == "" {
		return t, ErrTagKeyRequired
	}
	if len(t.Key) > 64 {
		return t, ErrTagKeyTooLong
	}
	if len(t.Value) > 128 {
		return t, ErrTagValueTooLong
	}
	for _, r := range t.Key {
		if !(r == '-' || r == '_' || r == '.' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9')) {
			return t, ErrTagKeyInvalid
		}
	}
	if strings.ContainsAny(t.Value, "=,") {
		return t, ErrTagValueInvalid
	}
	return t, nil
}

// ListTags returns every tag for a host, sorted by key for stable UI rendering.
func ListTags(ctx context.Context, db *sql.DB, hostID int64) ([]Tag, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT key, value FROM host_tags WHERE host_id = ? ORDER BY key`,
		hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Tag, 0)
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.Key, &t.Value); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListTagsByName resolves a host name to its id, then returns its tags.
// Engine-facing convenience — saves a join with the hosts table.
func ListTagsByName(ctx context.Context, db *sql.DB, name string) ([]Tag, error) {
	var id int64
	err := db.QueryRowContext(ctx, `SELECT id FROM hosts WHERE name = ?`, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ListTags(ctx, db, id)
}

// TagFacet is one distinct (key, value) pair in use across the fleet
// plus how many hosts carry it. Powers the rule-form tag picker so an
// operator can click an existing tag instead of typing it.
type TagFacet struct {
	Key       string
	Value     string
	HostCount int
}

// ListTagFacets returns every distinct (key, value) currently assigned
// to at least one host, with the host count. Sorted by key then value
// for stable rendering. Used by the alerts rule form's tag picker.
func ListTagFacets(ctx context.Context, db *sql.DB) ([]TagFacet, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT key, value, COUNT(DISTINCT host_id) AS host_count
		FROM host_tags
		GROUP BY key, value
		ORDER BY key, value`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TagFacet, 0)
	for rows.Next() {
		var f TagFacet
		if err := rows.Scan(&f.Key, &f.Value, &f.HostCount); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// AllHostTags returns every (host_name → tags map) pair. The engine
// calls this once per tick when at least one rule has a non-empty
// host_selector; the cost is dominated by the snapshot iteration, not
// this query.
func AllHostTags(ctx context.Context, db *sql.DB) (map[string]map[string]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT h.name, t.key, t.value
		FROM host_tags t JOIN hosts h ON h.id = t.host_id
		ORDER BY h.name, t.key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]string{}
	for rows.Next() {
		var name, k, v string
		if err := rows.Scan(&name, &k, &v); err != nil {
			return nil, err
		}
		m, ok := out[name]
		if !ok {
			m = map[string]string{}
			out[name] = m
		}
		m[k] = v
	}
	return out, rows.Err()
}

// SetTags replaces a host's tag set atomically. Empty list clears all
// tags. Capped at maxTagsPerHost to prevent runaway editor bugs.
func SetTags(ctx context.Context, db *sql.DB, hostID int64, tags []Tag) ([]Tag, error) {
	if len(tags) > maxTagsPerHost {
		return nil, ErrTooManyTags
	}
	cleaned := make([]Tag, 0, len(tags))
	seen := map[string]struct{}{}
	for _, t := range tags {
		v, err := validateTag(t)
		if err != nil {
			return nil, fmt.Errorf("%w (key=%q)", err, t.Key)
		}
		if _, dup := seen[v.Key]; dup {
			continue
		}
		seen[v.Key] = struct{}{}
		cleaned = append(cleaned, v)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// Confirm the host exists — FK alone would reject on insert but the
	// resulting error is opaque. Explicit check gives a clean 404.
	var exists int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM hosts WHERE id = ?`, hostID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM host_tags WHERE host_id = ?`, hostID); err != nil {
		return nil, fmt.Errorf("clear tags: %w", err)
	}
	if len(cleaned) > 0 {
		stmt, err := tx.PrepareContext(ctx,
			`INSERT INTO host_tags (host_id, key, value) VALUES (?, ?, ?)`)
		if err != nil {
			return nil, fmt.Errorf("prepare tag insert: %w", err)
		}
		defer stmt.Close()
		for _, t := range cleaned {
			if _, err := stmt.ExecContext(ctx, hostID, t.Key, t.Value); err != nil {
				return nil, fmt.Errorf("insert tag %s: %w", t.Key, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	// Re-read so the response reflects sort order + storage normalisation.
	sort.Slice(cleaned, func(i, j int) bool { return cleaned[i].Key < cleaned[j].Key })
	return cleaned, nil
}
