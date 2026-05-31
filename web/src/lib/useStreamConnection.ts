import { useEffect, useRef, useState } from "react";

export type WsStatus = "connecting" | "connected" | "disconnected" | "error";

export type StreamConnectionOptions<T> = {
  url: string;
  onMessage: (data: T) => void;
  // Called once on every successful open — used to re-send subscribe
  // frames so the server-side filter survives a reconnect. Must not
  // assume previous server state; it gets re-applied each time.
  onOpen?: (ws: WebSocket) => void;
};

// Backoff schedule for reconnect attempts (ms). Caps at 30s to avoid
// silent multi-minute gaps after long outages. Reset to attempt=0 on
// successful open AND on visibility change so user attention is fast.
const BACKOFF_MS = [1_000, 2_000, 4_000, 8_000, 16_000, 30_000];

// useStreamConnection wraps a WebSocket with auto-reconnect + visibility-
// aware fast-reconnect. Returns the current status for callers that want
// to render a connection pill; the data itself arrives via onMessage.
//
// Why this exists: a bare `new WebSocket(...)` in a useEffect has no
// reconnect path, so any transient close (browser throttle on background
// tab, NAT timeout, laptop sleep, server restart) leaves the snapshots
// state frozen while the `now` ticker keeps advancing — every host
// drifts into "stale" within ~30s even though the hub is still healthy.
export function useStreamConnection<T>({ url, onMessage, onOpen }: StreamConnectionOptions<T>): WsStatus {
  const [status, setStatus] = useState<WsStatus>("connecting");
  // Refs so the effect doesn't tear down + reconnect every render just
  // because the caller passed a fresh closure.
  const onMessageRef = useRef(onMessage);
  const onOpenRef = useRef(onOpen);
  onMessageRef.current = onMessage;
  onOpenRef.current = onOpen;

  useEffect(() => {
    let cancelled = false;
    let ws: WebSocket | null = null;
    let reconnectTimer: number | null = null;
    let attempt = 0;

    const clearTimer = () => {
      if (reconnectTimer !== null) {
        window.clearTimeout(reconnectTimer);
        reconnectTimer = null;
      }
    };

    const scheduleReconnect = () => {
      clearTimer();
      const delay = BACKOFF_MS[Math.min(attempt, BACKOFF_MS.length - 1)];
      attempt++;
      reconnectTimer = window.setTimeout(connect, delay);
    };

    const connect = () => {
      if (cancelled) return;
      clearTimer();
      setStatus("connecting");
      const socket = new WebSocket(url);
      ws = socket;
      socket.addEventListener("open", () => {
        if (cancelled) return;
        attempt = 0;
        setStatus("connected");
        onOpenRef.current?.(socket);
      });
      socket.addEventListener("message", (e) => {
        try {
          onMessageRef.current(JSON.parse(e.data as string) as T);
        } catch {
          // ignore malformed frames
        }
      });
      socket.addEventListener("close", () => {
        if (cancelled) return;
        setStatus("disconnected");
        scheduleReconnect();
      });
      socket.addEventListener("error", () => {
        if (cancelled) return;
        setStatus("error");
        // 'error' is always followed by 'close' on the same socket per
        // the WS spec, so leave reconnect scheduling to the close handler
        // — chaining both would double-schedule.
      });
    };

    // Force-reconnect immediately when the tab regains focus. Browser
    // throttling can stretch setTimeout in background tabs from 1s to
    // 60s+, so the scheduled reconnect may not have fired yet; this
    // bypasses the backoff and gets the dashboard fresh without the
    // user clicking around to remount a component.
    const onVisible = () => {
      if (document.visibilityState !== "visible") return;
      if (ws && ws.readyState === WebSocket.OPEN) return;
      attempt = 0;
      connect();
    };
    document.addEventListener("visibilitychange", onVisible);

    connect();

    return () => {
      cancelled = true;
      document.removeEventListener("visibilitychange", onVisible);
      clearTimer();
      ws?.close();
    };
  }, [url]);

  return status;
}
