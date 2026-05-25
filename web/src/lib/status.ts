/** Discrete status colors used across the dashboard. Each maps to a CSS
 * class declared in `index.css` so components can avoid inline `style`
 * attributes for color (linter rule). */
export type StatusTone = "ok" | "warn" | "danger" | "muted";

export const TONE_CLASS: Record<StatusTone, string> = {
  ok: "lumen-status-ok",
  warn: "lumen-status-warn",
  danger: "lumen-status-danger",
  muted: "lumen-status-muted",
};

/** Map a CPU% reading + staleness to a tone. */
export function cpuTone(cpuPct: number, stale: boolean): StatusTone {
  if (stale) return "muted";
  if (cpuPct >= 85) return "danger";
  if (cpuPct >= 60) return "warn";
  return "ok";
}

/** Round a 0–100 percentage to the nearest 5% bucket and return the matching
 * `lumen-w-XX` class. Keeps progress-bar widths declarative (no inline style).
 * 5% precision is visually indistinguishable on a 1.5px-tall bar. */
export function widthClass(pct: number): string {
  const clamped = Math.max(0, Math.min(100, pct));
  const bucket = Math.round(clamped / 5) * 5;
  return `lumen-w-${bucket}`;
}
