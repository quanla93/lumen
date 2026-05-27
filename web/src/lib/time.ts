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
export function isStale(iso: string, staleAfterMs = staleAfterForIntervalMs(), now = Date.now()): boolean {
  return now - new Date(iso).getTime() > staleAfterMs;
}

export function parseDurationMs(duration: string): number | null {
  const match = duration.match(/^(\d+)(s|m|h)$/);
  if (!match) return null;

  const amount = Number(match[1]);
  if (!Number.isSafeInteger(amount) || amount <= 0) return null;

  const unitMs: Record<string, number> = {
    s: 1_000,
    m: 60_000,
    h: 60 * 60_000,
  };
  return amount * unitMs[match[2]];
}

export function staleAfterForIntervalMs(agentInterval = "5s"): number {
  const intervalMs = parseDurationMs(agentInterval) ?? 5_000;
  return Math.max(intervalMs * 2, 30_000);
}
