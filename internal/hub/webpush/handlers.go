package webpush

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Handlers exposes the web push HTTP surface — VAPID key bootstrap,
// browser subscribe/unsubscribe, and admin listing per channel. The
// alerts channel CRUD (create/update/delete of the channel row itself)
// lives in alerts/handlers.go; web push only owns the *subscription*
// side because that's the bit that needs DB-level encryption + a stable
// VAPID key the rest of alerts doesn't care about.
type Handlers struct {
	DB        *sql.DB
	HubSecret []byte
	Logger    *slog.Logger
}

// GET /api/alerts/web-push/vapid-public-key — session-protected.
// Generates the VAPID pair on first call so the operator never has to
// "run an init script" before subscribing a browser.
func (h *Handlers) GetVAPIDPublicKey(w http.ResponseWriter, r *http.Request) {
	keys, err := EnsureKeys(r.Context(), h.DB, h.HubSecret)
	if err != nil {
		h.Logger.Error("ensure VAPID keys", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "VAPID key bootstrap failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"public_key": keys.PublicKey,
		"subject":    keys.Subject,
	})
}

// PUT /api/alerts/web-push/subject — session-protected. Updates the
// `sub` claim the VAPID JWT carries. Must be `mailto:` or `https://`.
func (h *Handlers) PutSubject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Subject string `json:"subject"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := SetSubject(r.Context(), h.DB, req.Subject); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/alerts/web-push/subscribe — session-protected.
// Body: { channel_id, endpoint, p256dh, auth, label? }.
// Idempotent — re-subscribing the same browser overwrites the row.
func (h *Handlers) Subscribe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ChannelID int64  `json:"channel_id"`
		Endpoint  string `json:"endpoint"`
		P256dh    string `json:"p256dh"`
		Auth      string `json:"auth"`
		Label     string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Endpoint = strings.TrimSpace(req.Endpoint)
	req.P256dh = strings.TrimSpace(req.P256dh)
	req.Auth = strings.TrimSpace(req.Auth)
	req.Label = strings.TrimSpace(req.Label)
	if req.ChannelID == 0 || req.Endpoint == "" || req.P256dh == "" || req.Auth == "" {
		writeJSONError(w, http.StatusBadRequest, "channel_id, endpoint, p256dh, auth required")
		return
	}
	if !strings.HasPrefix(req.Endpoint, "https://") {
		writeJSONError(w, http.StatusBadRequest, "endpoint must be https://")
		return
	}
	saved, err := AddSubscription(r.Context(), h.DB, Subscription{
		ChannelID: req.ChannelID,
		Endpoint:  req.Endpoint,
		P256dh:    req.P256dh,
		Auth:      req.Auth,
		Label:     req.Label,
	})
	if err != nil {
		h.Logger.Error("add subscription", "err", err, "channel_id", req.ChannelID)
		writeJSONError(w, http.StatusInternalServerError, "save failed")
		return
	}
	writeJSON(w, http.StatusOK, subscriptionView(saved))
}

// GET /api/alerts/channels/{id}/web-push/subscriptions — session-protected.
// Returns the current subscription list for the admin to see + manage.
// The endpoint URL is included; it leaks the push service host (Google /
// Mozilla / Apple), which is admin-only and not sensitive.
func (h *Handlers) ListSubscriptions(w http.ResponseWriter, r *http.Request) {
	chID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid channel id")
		return
	}
	subs, err := ListSubscriptions(r.Context(), h.DB, chID)
	if err != nil {
		h.Logger.Error("list subscriptions", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	out := make([]map[string]any, 0, len(subs))
	for _, s := range subs {
		out = append(out, subscriptionView(s))
	}
	writeJSON(w, http.StatusOK, out)
}

// DELETE /api/alerts/web-push/subscriptions/{id} — session-protected.
// Admin pruning of a specific browser registration.
func (h *Handlers) DeleteSubscription(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid subscription id")
		return
	}
	if err := DeleteSubscription(r.Context(), h.DB, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "subscription not found")
			return
		}
		h.Logger.Error("delete subscription", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func subscriptionView(s Subscription) map[string]any {
	return map[string]any{
		"id":         s.ID,
		"channel_id": s.ChannelID,
		"endpoint":   s.Endpoint,
		"label":      s.Label,
		"created_at": s.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
