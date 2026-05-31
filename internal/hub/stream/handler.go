// Package stream exposes a WebSocket endpoint that pushes per-host
// snapshots to subscribers every Interval. Clients can optionally send
// a control frame to narrow what they receive — see api.StreamControl.
//
// Wire format (server → client): an array of HostSnapshot, JSON-encoded.
// Wire format (client → server): api.StreamControl frames, JSON-encoded.
//
// Subscription rules:
//   - A connection starts with no filter — every host snapshot ships.
//     This keeps older web builds (Phase 1 dashboard) working with no
//     changes; they just ignore the bandwidth.
//   - A {"type":"subscribe","hosts":[...]} replaces the filter. The
//     special value "*" means "all hosts" (used to revert from a
//     specific subscription back to firehose mode when leaving a
//     detail view).
//   - The empty list is treated as the unsubscribed/firehose default
//     rather than "send nothing" — the latter is almost never what a
//     client wants, and a buggy client sending [] should still see
//     data so the operator notices.
package stream

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/quanla93/lumen/internal/hub/store"
	"github.com/quanla93/lumen/internal/shared/api"
)

// CheckOrigin is permissive in dev: any origin may connect. Phase 2 will
// tighten this based on the auth boundary.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

// Server-driven keepalive. Two purposes:
//   - Detect dead clients (browser killed, laptop slept, NAT mapping gone)
//     within ~pongTimeout so the goroutine pair doesn't leak waiting on
//     a socket that will never read again.
//   - Keep the TCP/NAT mapping warm — without periodic traffic in BOTH
//     directions, intermediate proxies (Cloudflare, corporate firewall,
//     CGNAT) silently drop the connection after ~60s of one-way silence.
//
// pingInterval is set to half of pongTimeout so a single dropped ping
// still leaves room for one more before the read deadline fires.
const (
	pongTimeout  = 60 * time.Second
	pingInterval = 30 * time.Second
	writeTimeout = 5 * time.Second
)

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

// subscription is the per-connection filter state. Guarded by mu so the
// reader goroutine (control frames) and the writer (ticker) coexist
// safely without a channel hop on every snapshot.
type subscription struct {
	mu      sync.RWMutex
	allowed map[string]bool // nil → firehose (all hosts)
}

func (s *subscription) set(hosts []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Empty list or {"*"} both mean firehose. The wildcard is the
	// explicit revert path the UI uses when leaving a detail view.
	if len(hosts) == 0 {
		s.allowed = nil
		return
	}
	for _, h := range hosts {
		if h == "*" {
			s.allowed = nil
			return
		}
	}
	m := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		m[h] = true
	}
	s.allowed = m
}

func (s *subscription) filter(in []api.HostSnapshot) []api.HostSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.allowed == nil {
		return in
	}
	out := make([]api.HostSnapshot, 0, len(in))
	for _, snap := range in {
		if s.allowed[snap.Host] {
			out = append(out, snap)
		}
	}
	return out
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

	sub := &subscription{}

	// Read deadline + pong handler form the dead-client detector. Browser
	// auto-replies pong to every server ping (built-in WS behavior, no FE
	// code needed); the handler extends the deadline so a healthy client
	// keeps the conn alive. No pong within pongTimeout → ReadMessage
	// errors → reader goroutine returns → main loop unblocks via `closed`.
	conn.SetReadDeadline(time.Now().Add(pongTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongTimeout))
	})

	// Reader goroutine surfaces close/ping AND parses control frames.
	// We accept text frames as JSON; binary frames are ignored.
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			// Any frame from the client counts as proof-of-life; extend
			// the deadline so an active subscribe-spamming client doesn't
			// get evicted just because pongs are slower than control
			// frames.
			conn.SetReadDeadline(time.Now().Add(pongTimeout))
			if msgType != websocket.TextMessage {
				continue
			}
			var ctrl api.StreamControl
			if err := json.Unmarshal(data, &ctrl); err != nil {
				h.Logger.Debug("ws control parse failed", "err", err, "remote", r.RemoteAddr)
				continue
			}
			switch ctrl.Type {
			case "subscribe":
				sub.set(ctrl.Hosts)
				h.Logger.Debug("ws subscription updated",
					"remote", r.RemoteAddr, "hosts", ctrl.Hosts)
			default:
				h.Logger.Debug("ws unknown control type", "type", ctrl.Type)
			}
		}
	}()

	// Push an initial snapshot immediately so the client doesn't wait a
	// full tick before seeing data.
	if err := writeSnapshot(conn, h.Store, sub); err != nil {
		return
	}

	t := time.NewTicker(h.Interval)
	defer t.Stop()
	pingT := time.NewTicker(pingInterval)
	defer pingT.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-closed:
			return
		case <-t.C:
			if err := writeSnapshot(conn, h.Store, sub); err != nil {
				return
			}
		case <-pingT.C:
			if err := conn.WriteControl(websocket.PingMessage, nil,
				time.Now().Add(writeTimeout)); err != nil {
				return
			}
		}
	}
}

func writeSnapshot(conn *websocket.Conn, s *store.Store, sub *subscription) error {
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	snaps := sub.filter(s.Snapshot())
	return conn.WriteJSON(snaps)
}
