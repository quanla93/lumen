package agent

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/quanla93/lumen/internal/hub/hosts"
	"github.com/quanla93/lumen/internal/hub/settings"
	"github.com/quanla93/lumen/internal/shared/api"
)

type PolicyHandler struct {
	DB     *sql.DB
	Logger *slog.Logger
}

func NewPolicyHandler(db *sql.DB, logger *slog.Logger) *PolicyHandler {
	return &PolicyHandler{DB: db, Logger: logger}
}

func (h *PolicyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hdr := r.Header.Get("Authorization")
	if !strings.HasPrefix(hdr, "Bearer ") {
		writeErr(w, http.StatusUnauthorized, "missing or malformed Authorization: Bearer <token> header")
		return
	}
	if _, err := hosts.VerifyToken(r.Context(), h.DB, strings.TrimPrefix(hdr, "Bearer ")); err != nil {
		if errors.Is(err, hosts.ErrInvalidToken) {
			writeErr(w, http.StatusUnauthorized, "invalid token")
			return
		}
		h.Logger.Error("token verify failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}

	interval, err := settings.Get(r.Context(), h.DB, settings.KeyAgentInterval)
	if errors.Is(err, sql.ErrNoRows) {
		interval = "5s"
	} else if err != nil {
		h.Logger.Error("agent policy read failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "read failed")
		return
	}

	// Process list policy (RFC 0003). Server-side gate is read here;
	// the agent's own env var (LUMEN_AGENT_PROCESSES) is the other
	// half — both must be true for the collector to ship rows.
	procsEnabled := false
	if v, _ := settings.Get(r.Context(), h.DB, "processes.enabled"); v == "true" {
		procsEnabled = true
	}
	procsTopN := 10
	if v, _ := settings.Get(r.Context(), h.DB, "processes.top_n"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
			procsTopN = n
		}
	}
	procsSortBy := "cpu"
	if v, _ := settings.Get(r.Context(), h.DB, "processes.sort_by"); v == "rss" {
		procsSortBy = "rss"
	}
	procsRedact, _ := settings.Get(r.Context(), h.DB, "processes.redact_regex")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.AgentPolicyResponse{
		CollectionInterval:    interval,
		ProcessesEnabled:      procsEnabled,
		ProcessesTopN:         procsTopN,
		ProcessesSortBy:       procsSortBy,
		ProcessesRedactRegex:  procsRedact,
	})
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
