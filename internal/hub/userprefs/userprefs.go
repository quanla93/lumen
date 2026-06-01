// Package userprefs backs RFC 0002 PR2 Level 3 personalization.
//
// Two well-known keys today — `dashboard_prefs` and `display_prefs` —
// each a JSON blob with `schemaVersion: 1`. The package validates the
// blob's shape on write so the dashboard / settings UI can trust what
// it reads. Unknown keys are tolerated (404 on GET, accepted on PUT
// only via the explicit handlers below).
package userprefs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	KeyDashboard = "dashboard_prefs"
	KeyDisplay   = "display_prefs"

	// maxJSONBytes caps how big a single pref blob can grow. Users
	// could in theory stuff thousands of hidden hosts into the
	// dashboard prefs blob; bound the row to keep table scans cheap.
	maxJSONBytes = 32 * 1024
)

var (
	ErrNotFound      = errors.New("prefs not found")
	ErrInvalidJSON   = errors.New("body is not valid JSON")
	ErrTooLarge      = errors.New("prefs blob exceeds 32 KiB")
	ErrSchemaVersion = errors.New("schemaVersion must be 1")
)

// Get returns the raw JSON for one (user, key). Returns ErrNotFound
// if the row doesn't exist so the caller can decide whether to fall
// back to defaults or 404.
func Get(ctx context.Context, db *sql.DB, userID int64, key string) (string, error) {
	var v string
	err := db.QueryRowContext(ctx,
		`SELECT json_value FROM user_prefs WHERE user_id = ? AND key = ?`,
		userID, key,
	).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

// Set upserts the (user, key) row. The body must be valid JSON of
// length ≤ maxJSONBytes; shape validation belongs to the caller
// (ValidateDashboard / ValidateDisplay below).
func Set(ctx context.Context, db *sql.DB, userID int64, key, jsonValue string) error {
	if len(jsonValue) > maxJSONBytes {
		return ErrTooLarge
	}
	var probe interface{}
	if err := json.Unmarshal([]byte(jsonValue), &probe); err != nil {
		return ErrInvalidJSON
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO user_prefs (user_id, key, json_value, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id, key) DO UPDATE
		SET json_value = excluded.json_value,
		    updated_at = excluded.updated_at`,
		userID, key, jsonValue, time.Now().Unix(),
	)
	return err
}

// ─── DashboardPrefs validation ─────────────────────────────────────────

type DashboardPrefs struct {
	SchemaVersion     int                            `json:"schemaVersion"`
	SortBy            string                         `json:"sortBy"`
	SortDir           string                         `json:"sortDir"`
	DefaultMetric     string                         `json:"defaultMetric"`
	HiddenHostIDs     []string                       `json:"hiddenHostIds"`
	ActiveViewID      *string                        `json:"activeViewId"`
	Views             []SavedView                    `json:"views"`
	HostDetailLayouts map[string][]ChartLayoutItem   `json:"hostDetailLayouts,omitempty"`
}

// ChartLayoutItem mirrors react-grid-layout's per-item position +
// size. `I` is the catalog chart ID (e.g. "cpu", "ram", "swap").
type ChartLayoutItem struct {
	I string `json:"i"`
	X int    `json:"x"`
	Y int    `json:"y"`
	W int    `json:"w"`
	H int    `json:"h"`
}

type SavedView struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	SortBy        string   `json:"sortBy"`
	SortDir       string   `json:"sortDir"`
	DefaultMetric string   `json:"defaultMetric"`
	HiddenHostIDs []string `json:"hiddenHostIds"`
	TagFilter     []string `json:"tagFilter,omitempty"`
}

var (
	validSortBy        = map[string]struct{}{"name": {}, "hottest": {}, "last-seen": {}, "tag": {}}
	validSortDir       = map[string]struct{}{"asc": {}, "desc": {}}
	validDefaultMetric = map[string]struct{}{"all": {}, "cpu": {}, "ram": {}, "disk": {}}
)

const (
	maxSavedViews        = 5
	maxHostLayouts       = 50  // cap on hosts in hostDetailLayouts; keeps blob bounded
	maxChartsPerHost     = 20  // cap on chart items per host
	maxLayoutCoord       = 100 // hard ceiling on x/y/w/h
)

// validChartIDs is the catalog of chart IDs the server will accept in
// a saved layout. Kept in sync with web/src/components/hostCharts/
// catalog.ts CATALOG_IDS. Adding a new chart requires updates on both
// sides — that lockstep is intentional so the client and server agree
// on the catalog surface.
var validChartIDs = map[string]struct{}{
	"cpu":          {},
	"cpu-per-core": {},
	"ram":          {},
	"swap":         {},
	"disk":         {},
	"disk-io":      {},
	"network":      {},
	"load":         {},
	"temperature":  {},
	"containers":   {},
}

// ValidateDashboard checks shape + enums + length caps. Returns the
// parsed struct so the handler can re-encode it (canonicalising
// whitespace + key order) before writing.
func ValidateDashboard(raw string) (DashboardPrefs, error) {
	var p DashboardPrefs
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return p, ErrInvalidJSON
	}
	if p.SchemaVersion != 1 {
		return p, ErrSchemaVersion
	}
	if _, ok := validSortBy[p.SortBy]; !ok {
		return p, fmt.Errorf("sortBy must be one of name|hottest|last-seen|tag, got %q", p.SortBy)
	}
	if p.SortDir == "" {
		p.SortDir = "asc"
	}
	if _, ok := validSortDir[p.SortDir]; !ok {
		return p, fmt.Errorf("sortDir must be asc|desc, got %q", p.SortDir)
	}
	if _, ok := validDefaultMetric[p.DefaultMetric]; !ok {
		return p, fmt.Errorf("defaultMetric must be all|cpu|ram|disk, got %q", p.DefaultMetric)
	}
	if p.HiddenHostIDs == nil {
		p.HiddenHostIDs = []string{}
	}
	if p.Views == nil {
		p.Views = []SavedView{}
	}
	if len(p.Views) > maxSavedViews {
		return p, fmt.Errorf("views cap is %d, got %d", maxSavedViews, len(p.Views))
	}
	seenIDs := map[string]struct{}{}
	for i := range p.Views {
		v := &p.Views[i]
		v.Name = strings.TrimSpace(v.Name)
		if v.Name == "" || len(v.Name) > 32 {
			return p, fmt.Errorf("view name length must be 1..32, got %d", len(v.Name))
		}
		if v.ID == "" {
			return p, errors.New("view id required")
		}
		if _, dup := seenIDs[v.ID]; dup {
			return p, fmt.Errorf("duplicate view id %q", v.ID)
		}
		seenIDs[v.ID] = struct{}{}
		if _, ok := validSortBy[v.SortBy]; !ok {
			return p, fmt.Errorf("view[%s] sortBy invalid", v.ID)
		}
		if v.SortDir == "" {
			v.SortDir = "asc"
		}
		if _, ok := validSortDir[v.SortDir]; !ok {
			return p, fmt.Errorf("view[%s] sortDir invalid", v.ID)
		}
		if _, ok := validDefaultMetric[v.DefaultMetric]; !ok {
			return p, fmt.Errorf("view[%s] defaultMetric invalid", v.ID)
		}
		if v.HiddenHostIDs == nil {
			v.HiddenHostIDs = []string{}
		}
	}
	if len(p.HostDetailLayouts) > maxHostLayouts {
		return p, fmt.Errorf("hostDetailLayouts cap is %d, got %d", maxHostLayouts, len(p.HostDetailLayouts))
	}
	for host, items := range p.HostDetailLayouts {
		if strings.TrimSpace(host) == "" {
			return p, errors.New("hostDetailLayouts has empty host name")
		}
		if len(items) > maxChartsPerHost {
			return p, fmt.Errorf("hostDetailLayouts[%s]: chart cap is %d, got %d", host, maxChartsPerHost, len(items))
		}
		seenItems := map[string]struct{}{}
		for _, item := range items {
			if _, ok := validChartIDs[item.I]; !ok {
				return p, fmt.Errorf("hostDetailLayouts[%s]: unknown chart id %q", host, item.I)
			}
			if _, dup := seenItems[item.I]; dup {
				return p, fmt.Errorf("hostDetailLayouts[%s]: duplicate chart id %q", host, item.I)
			}
			seenItems[item.I] = struct{}{}
			if item.X < 0 || item.Y < 0 || item.W < 1 || item.H < 1 {
				return p, fmt.Errorf("hostDetailLayouts[%s][%s]: x/y must be ≥0, w/h must be ≥1", host, item.I)
			}
			if item.X > maxLayoutCoord || item.Y > maxLayoutCoord || item.W > maxLayoutCoord || item.H > maxLayoutCoord {
				return p, fmt.Errorf("hostDetailLayouts[%s][%s]: coord exceeds %d cap", host, item.I, maxLayoutCoord)
			}
		}
	}
	return p, nil
}

// ─── DisplayPrefs validation ───────────────────────────────────────────

type DisplayPrefs struct {
	SchemaVersion int    `json:"schemaVersion"`
	Theme         string `json:"theme"`
	Language      string `json:"language"`
	Units         string `json:"units"`
	ReduceMotion  string `json:"reduceMotion"`
	Density       string `json:"density"`
}

var (
	validTheme        = map[string]struct{}{"system": {}, "light": {}, "dark": {}}
	validLanguage     = map[string]struct{}{"en": {}, "vi": {}}
	validUnits        = map[string]struct{}{"auto": {}, "binary": {}, "decimal": {}}
	validReduceMotion = map[string]struct{}{"system": {}, "on": {}, "off": {}}
	validDensity      = map[string]struct{}{"comfortable": {}, "compact": {}}
)

// ValidateDisplay checks the shape of the display blob. The `density`
// field is reserved (per RFC 0002 schema reservation) — only
// `comfortable` is honored in PR2 but `compact` is accepted so the
// schema doesn't break when PR3+ ships the density toggle.
func ValidateDisplay(raw string) (DisplayPrefs, error) {
	var p DisplayPrefs
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return p, ErrInvalidJSON
	}
	if p.SchemaVersion != 1 {
		return p, ErrSchemaVersion
	}
	if _, ok := validTheme[p.Theme]; !ok {
		return p, fmt.Errorf("theme must be system|light|dark, got %q", p.Theme)
	}
	if _, ok := validLanguage[p.Language]; !ok {
		return p, fmt.Errorf("language must be en|vi, got %q", p.Language)
	}
	if _, ok := validUnits[p.Units]; !ok {
		return p, fmt.Errorf("units must be auto|binary|decimal, got %q", p.Units)
	}
	if _, ok := validReduceMotion[p.ReduceMotion]; !ok {
		return p, fmt.Errorf("reduceMotion must be system|on|off, got %q", p.ReduceMotion)
	}
	if p.Density == "" {
		p.Density = "comfortable"
	}
	if _, ok := validDensity[p.Density]; !ok {
		return p, fmt.Errorf("density must be comfortable|compact, got %q", p.Density)
	}
	return p, nil
}
