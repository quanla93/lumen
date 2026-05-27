package agent

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.AgentPolicyResponse{CollectionInterval: interval})
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
