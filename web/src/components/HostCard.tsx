import { cpuTone, TONE_CLASS, widthClass } from "@/lib/status";
import { isStale, relativeTime } from "@/lib/time";

export type Snapshot = {
  host: string;
  ts: string;
  cpu_pct: number;
};

export function HostCard({ snapshot, now }: { snapshot: Snapshot; now: number }) {
  const stale = isStale(snapshot.ts, 15_000, now);
  const tone = cpuTone(snapshot.cpu_pct, stale);
  const toneClass = TONE_CLASS[tone];
  const barWidth = widthClass(snapshot.cpu_pct);

  return (
    <div className="rounded-lg border border-[color:var(--color-border)] bg-[color:var(--color-card)] p-4 shadow-sm">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <span
            aria-hidden
            className={`inline-block h-2.5 w-2.5 rounded-full ${toneClass}`}
          />
          <span className="font-mono text-sm font-medium tracking-tight">
            {snapshot.host}
          </span>
        </div>
        <span className="text-xs text-[color:var(--color-muted)]">
          {stale ? "stale · " : ""}{relativeTime(snapshot.ts, now)}
        </span>
      </div>

      <div className="flex items-baseline gap-2 mb-2">
        <span className="font-mono text-3xl font-semibold tabular-nums">
          {snapshot.cpu_pct.toFixed(1)}
        </span>
        <span className="text-sm text-[color:var(--color-muted)]">% CPU</span>
      </div>

      <div className="h-1.5 w-full rounded-full bg-[color:var(--color-border)] overflow-hidden">
        <div
          className={`h-full transition-[width] duration-300 ease-out ${toneClass} ${barWidth}`}
        />
      </div>
    </div>
  );
}
