// Package stream exposes a WebSocket endpoint that pushes the current
// per-host snapshot to subscribers every Interval. Phase 1 spike: no
// filtering, no diffing — clients get the full snapshot each tick.
package stream

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/lumenhq/lumen/internal/hub/store"
)

// CheckOrigin is permissive in dev: any origin may connect. Phase 2 will
// tighten this based on the auth boundary.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

type Handler struct {
	Store    *store.Store
	Logger   *slog.Logger
	Interval time.Duration
}

func New(s *store.Store, logger *slog.Logger, interval time.Duration) *Handler {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Handler{Store: s, Logger: logger, Interval: interval}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.Logger.Warn("ws upgrade failed", "err", err, "remote", r.RemoteAddr)
		return
	}
	defer conn.Close()

	h.Logger.Debug("ws client connected", "remote", r.RemoteAddr)
	defer h.Logger.Debug("ws client disconnected", "remote", r.RemoteAddr)

	// Detect client-initiated close via a reader goroutine — gorilla's
	// ReadMessage is the canonical way to surface ping/pong/close frames.
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.NextReader(); err != nil {
				return
			}
		}
	}()

	// Push an initial snapshot immediately so the client doesn't wait a
	// full tick before seeing data.
	if err := writeSnapshot(conn, h.Store); err != nil {
		return
	}

	t := time.NewTicker(h.Interval)
	defer t.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-closed:
			return
		case <-t.C:
			if err := writeSnapshot(conn, h.Store); err != nil {
				return
			}
		}
	}
}

func writeSnapshot(conn *websocket.Conn, s *store.Store) error {
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	return conn.WriteJSON(s.Snapshot())
}
