package hosts

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/lumenhq/lumen/internal/hub/storage"
)

const (
	metricsMaxWindow  = 7 * 24 * time.Hour
	metricsDefaultWin = 1 * time.Hour
	metricsMinStep    = 5 * time.Second
	metricsMaxPoints  = 2000
)

// Metrics handles GET /api/hosts/{id}/metrics?from=...&to=...&step=...
//
// Query string:
//   - from, to — RFC3339 timestamps. Defaults: to=now, from=to-1h.
//   - step    — Go duration (e.g. "30s", "5m"). Default: auto from window.
//
// Response:
//
//	{ "host": "...", "from": "...", "to": "...",
//	  "step_seconds": 30, "points": [ {ts, cpu_pct, ...}, ... ] }
//
// Empty buckets are omitted — clients fill gaps if a continuous line is
// needed. Max 2000 points per response; widen `step` or shrink the window
// to fit.
func (h *Handlers) Metrics(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}
	host, err := getByID(r.Context(), h.DB, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "host not found")
			return
		}
		h.Logger.Error("lookup host failed", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	from, to, step, err := parseMetricsRange(r.URL.Query())
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	points, err := storage.QueryMetrics(
		r.Context(), h.DB, host.Name, from, to, int64(step.Seconds()),
	)
	if err != nil {
		h.Logger.Error("query metrics failed", "err", err, "host", host.Name)
		writeJSONError(w, http.StatusInternalServerError, "query failed")
		return
	}
	// Empty result set serializes as `null` by default; flip to [] so the
	// client doesn't have to special-case the empty case.
	if points == nil {
		points = []storage.MetricPoint{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"host":         host.Name,
		"from":         from.UTC().Format(time.RFC3339),
		"to":           to.UTC().Format(time.RFC3339),
		"step_seconds": int64(step.Seconds()),
		"points":       points,
	})
}

func parseMetricsRange(q url.Values) (from, to time.Time, step time.Duration, err error) {
	to = time.Now().UTC()
	if s := q.Get("to"); s != "" {
		to, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return time.Time{}, time.Time{}, 0, errors.New("invalid to (need RFC3339)")
		}
	}
	from = to.Add(-metricsDefaultWin)
	if s := q.Get("from"); s != "" {
		from, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return time.Time{}, time.Time{}, 0, errors.New("invalid from (need RFC3339)")
		}
	}
	if !from.Before(to) {
		return time.Time{}, time.Time{}, 0, errors.New("from must be < to")
	}
	window := to.Sub(from)
	if window > metricsMaxWindow {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("window > %s (max)", metricsMaxWindow)
	}

	if s := q.Get("step"); s != "" {
		step, err = time.ParseDuration(s)
		if err != nil {
			return time.Time{}, time.Time{}, 0, errors.New("invalid step (e.g. 30s, 5m)")
		}
	} else {
		step = autoStep(window)
	}
	if step < metricsMinStep {
		step = metricsMinStep
	}
	if n := window / step; n > metricsMaxPoints {
		return time.Time{}, time.Time{}, 0, fmt.Errorf(
			"too many points: %d > %d (widen step or shrink window)",
			n, metricsMaxPoints,
		)
	}
	return from.UTC(), to.UTC(), step, nil
}

// autoStep picks a friendly bucket size for a window so a chart renders
// ~120 points by default. The ticks are the same ones a human would
// reach for (5s, 10s, 30s, 1m, 5m, ...).
func autoStep(window time.Duration) time.Duration {
	target := window / 120
	ticks := []time.Duration{
		5 * time.Second, 10 * time.Second, 30 * time.Second,
		1 * time.Minute, 5 * time.Minute, 15 * time.Minute,
		1 * time.Hour, 6 * time.Hour, 24 * time.Hour,
	}
	for _, t := range ticks {
		if target <= t {
			return t
		}
	}
	return 24 * time.Hour
}
