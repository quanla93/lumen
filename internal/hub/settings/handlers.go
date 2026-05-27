package settings

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Bounds keep operator-changed settings sane. Generous on purpose — the
// homelab use case spans short test runs to year-scale retention, but we
// still want guardrails against obvious foot-guns like pegging the disk
// with sub-second policy or retention loops.
const (
	MinRetentionWindow         = 5 * time.Minute
	MaxRetentionWindow         = 365 * 24 * time.Hour
	MinRetentionInterval       = 1 * time.Minute
	MaxRetentionInterval       = 24 * time.Hour
	MinAgentInterval           = 2 * time.Second
	MaxAgentInterval           = 1 * time.Hour
	MinDownsampleBucketSize    = 1 * time.Minute
	MaxDownsampleBucketSize    = 24 * time.Hour
	MinDownsampleHotWindow     = 1 * time.Hour
	MaxDownsampleHotWindow     = 30 * 24 * time.Hour
	MinDownsampleArchiveWindow = 24 * time.Hour
	MaxDownsampleArchiveWindow = 365 * 24 * time.Hour
)

type Handlers struct {
	DB     *sql.DB
	Logger *slog.Logger
}

func NewHandlers(db *sql.DB, logger *slog.Logger) *Handlers {
	return &Handlers{DB: db, Logger: logger}
}

// settingsView is the single JSON shape used by GET and accepted by PUT.
// Durations go over the wire as Go strings ("24h", "1h", "30m") so they
// round-trip without ambiguity.
type settingsView struct {
	RetentionWindow         string `json:"retention_window"`
	RetentionInterval       string `json:"retention_interval"`
	AgentInterval           string `json:"agent_interval"`
	DownsampleBucketSize    string `json:"downsample_bucket_size"`
	DownsampleHotWindow     string `json:"downsample_hot_window"`
	DownsampleArchiveWindow string `json:"downsample_archive_window"`
}

// GET /api/settings
func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	win, err := Get(r.Context(), h.DB, KeyRetentionWindow)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusInternalServerError, "read failed")
		return
	}
	itv, err := Get(r.Context(), h.DB, KeyRetentionInterval)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusInternalServerError, "read failed")
		return
	}
	agentInterval, err := Get(r.Context(), h.DB, KeyAgentInterval)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusInternalServerError, "read failed")
		return
	}
	bucketSize, err := Get(r.Context(), h.DB, KeyDownsampleBucketSize)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusInternalServerError, "read failed")
		return
	}
	hotWindow, err := Get(r.Context(), h.DB, KeyDownsampleHotWindow)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusInternalServerError, "read failed")
		return
	}
	archiveWindow, err := Get(r.Context(), h.DB, KeyDownsampleArchiveWindow)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeJSONError(w, http.StatusInternalServerError, "read failed")
		return
	}
	writeJSON(w, http.StatusOK, settingsView{
		RetentionWindow:         win,
		RetentionInterval:       itv,
		AgentInterval:           agentInterval,
		DownsampleBucketSize:    bucketSize,
		DownsampleHotWindow:     hotWindow,
		DownsampleArchiveWindow: archiveWindow,
	})
}

// PUT /api/settings
//
// Body: settingsView. Empty/missing fields are left unchanged. Validation
// applies bounds + parseability — operators can't bypass them via the
// API.
func (h *Handlers) Put(w http.ResponseWriter, r *http.Request) {
	var req settingsView
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.RetentionWindow != "" {
		if err := validateDurationBounds(
			req.RetentionWindow, MinRetentionWindow, MaxRetentionWindow,
		); err != nil {
			writeJSONError(w, http.StatusBadRequest, "retention_window: "+err.Error())
			return
		}
	}
	if req.RetentionInterval != "" {
		if err := validateDurationBounds(
			req.RetentionInterval, MinRetentionInterval, MaxRetentionInterval,
		); err != nil {
			writeJSONError(w, http.StatusBadRequest, "retention_interval: "+err.Error())
			return
		}
	}
	if req.AgentInterval != "" {
		if err := validateDurationBounds(
			req.AgentInterval, MinAgentInterval, MaxAgentInterval,
		); err != nil {
			writeJSONError(w, http.StatusBadRequest, "agent_interval: "+err.Error())
			return
		}
	}
	if req.DownsampleBucketSize != "" {
		if err := validateDurationBounds(
			req.DownsampleBucketSize, MinDownsampleBucketSize, MaxDownsampleBucketSize,
		); err != nil {
			writeJSONError(w, http.StatusBadRequest, "downsample_bucket_size: "+err.Error())
			return
		}
	}
	if req.DownsampleHotWindow != "" {
		if err := validateDurationBounds(
			req.DownsampleHotWindow, MinDownsampleHotWindow, MaxDownsampleHotWindow,
		); err != nil {
			writeJSONError(w, http.StatusBadRequest, "downsample_hot_window: "+err.Error())
			return
		}
	}
	if req.DownsampleArchiveWindow != "" {
		if err := validateDurationBounds(
			req.DownsampleArchiveWindow, MinDownsampleArchiveWindow, MaxDownsampleArchiveWindow,
		); err != nil {
			writeJSONError(w, http.StatusBadRequest, "downsample_archive_window: "+err.Error())
			return
		}
	}

	if req.RetentionWindow != "" {
		if err := Set(r.Context(), h.DB, KeyRetentionWindow, req.RetentionWindow); err != nil {
			h.Logger.Error("settings set window failed", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "write failed")
			return
		}
	}
	if req.RetentionInterval != "" {
		if err := Set(r.Context(), h.DB, KeyRetentionInterval, req.RetentionInterval); err != nil {
			h.Logger.Error("settings set interval failed", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "write failed")
			return
		}
	}
	if req.AgentInterval != "" {
		if err := Set(r.Context(), h.DB, KeyAgentInterval, req.AgentInterval); err != nil {
			h.Logger.Error("settings set agent interval failed", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "write failed")
			return
		}
	}
	if req.DownsampleBucketSize != "" {
		if err := Set(r.Context(), h.DB, KeyDownsampleBucketSize, req.DownsampleBucketSize); err != nil {
			h.Logger.Error("settings set downsample bucket size failed", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "write failed")
			return
		}
	}
	if req.DownsampleHotWindow != "" {
		if err := Set(r.Context(), h.DB, KeyDownsampleHotWindow, req.DownsampleHotWindow); err != nil {
			h.Logger.Error("settings set downsample hot window failed", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "write failed")
			return
		}
	}
	if req.DownsampleArchiveWindow != "" {
		if err := Set(r.Context(), h.DB, KeyDownsampleArchiveWindow, req.DownsampleArchiveWindow); err != nil {
			h.Logger.Error("settings set downsample archive window failed", "err", err)
			writeJSONError(w, http.StatusInternalServerError, "write failed")
			return
		}
	}

	// Re-emit the now-current state so the UI can sync immediately
	// without a second fetch.
	h.Get(w, r)
}

func validateDurationBounds(s string, min, max time.Duration) error {
	d, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q (e.g. 24h, 1h, 30m)", s)
	}
	if d < min {
		return fmt.Errorf("must be >= %s", min)
	}
	if d > max {
		return fmt.Errorf("must be <= %s", max)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
