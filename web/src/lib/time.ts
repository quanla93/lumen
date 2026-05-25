/** Format an ISO timestamp as a short relative string ("3s ago", "12m ago"). */
export function relativeTime(iso: string, now = Date.now()): string {
  const ts = new Date(iso).getTime();
  const deltaMs = now - ts;
  if (deltaMs < 0) return "in the future";
  const s = Math.floor(deltaMs / 1000);
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const d = Math.floor(h / 24);
  return `${d}d ago`;
}

/** True when the snapshot timestamp is more than `staleAfterMs` old. */
export function isStale(iso: string, staleAfterMs = 15_000, now = Date.now()): boolean {
  return now - new Date(iso).getTime() > staleAfterMs;
}
