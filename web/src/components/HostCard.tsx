import { cpuTone, TONE_CLASS, widthClass, type StatusTone } from "@/lib/status";
import { isStale, relativeTime } from "@/lib/time";
import { Sparkline } from "@/components/Sparkline";

export type Snapshot = {
  host: string;
  ts: string;
  cpu_pct: number;
  cpu_per_core?: number[];
  ram_pct: number;
  swap_pct: number;
  disk_pct: number;
  load1: number;
  load5: number;
  load15: number;
  net_rx_bps: number;
  net_tx_bps: number;
  disk_r_bps: number;
  disk_w_bps: number;
  temp_c: number;
  cpu_series?: number[];
};

const TONE_TEXT: Record<StatusTone, string> = {
  ok: "text-[color:var(--color-accent)]",
  warn: "text-[color:var(--color-warn)]",
  danger: "text-[color:var(--color-danger)]",
  muted: "text-[color:var(--color-muted)]",
};

type MetricRow = {
  label: string;
  value: number;
  tone: StatusTone;
};

function metricRow(label: string, value: number, stale: boolean): MetricRow {
  return { label, value, tone: cpuTone(value, stale) };
}

export function HostCard({
  snapshot,
  now,
  onSelect,
}: {
  snapshot: Snapshot;
  now: number;
  onSelect?: (hostName: string) => void;
}) {
  const stale = isStale(snapshot.ts, 15_000, now);
  const headerTone = cpuTone(snapshot.cpu_pct, stale);

  const rows: MetricRow[] = [
    metricRow("CPU", snapshot.cpu_pct, stale),
    metricRow("RAM", snapshot.ram_pct, stale),
    metricRow("Disk", snapshot.disk_pct, stale),
  ];

  const hasLoad = snapshot.load1 + snapshot.load5 + snapshot.load15 > 0;
  const series = snapshot.cpu_series ?? [];

  const interactive = !!onSelect;
  const handleClick = () => onSelect?.(snapshot.host);
  const handleKey = (e: React.KeyboardEvent) => {
    if (!onSelect) return;
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      onSelect(snapshot.host);
    }
  };

  return (
    <div
      role={interactive ? "button" : undefined}
      tabIndex={interactive ? 0 : undefined}
      onClick={interactive ? handleClick : undefined}
      onKeyDown={interactive ? handleKey : undefined}
      className={
        "rounded-lg border border-[color:var(--color-border)] bg-[color:var(--color-card)] p-4 shadow-sm " +
        (interactive
          ? "cursor-pointer transition-shadow hover:shadow-md focus:outline-none focus:ring-2 focus:ring-[color:var(--color-accent)]"
          : "")
      }
    >
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <span
            aria-hidden
            className={`inline-block h-2.5 w-2.5 rounded-full ${TONE_CLASS[headerTone]}`}
          />
          <span className="font-mono text-sm font-medium tracking-tight">
            {snapshot.host}
          </span>
        </div>
        <span className="text-xs text-[color:var(--color-muted)]">
          {stale ? "stale · " : ""}
          {relativeTime(snapshot.ts, now)}
        </span>
      </div>

      {series.length >= 2 && (
        <div className={`mb-3 h-[18px] ${TONE_TEXT[headerTone]}`}>
          <Sparkline values={series} width={100} height={18} className="w-full h-full" />
        </div>
      )}

      <div className="space-y-2.5">
        {rows.map((r) => (
          <MetricBar key={r.label} {...r} />
        ))}
      </div>

      {hasLoad && (
        <div className="mt-4 pt-3 border-t border-[color:var(--color-border)] flex items-center justify-between text-xs text-[color:var(--color-muted)]">
          <span>load avg</span>
          <span className="font-mono tabular-nums">
            {snapshot.load1.toFixed(2)} · {snapshot.load5.toFixed(2)} · {snapshot.load15.toFixed(2)}
          </span>
        </div>
      )}
    </div>
  );
}

function MetricBar({ label, value, tone }: MetricRow) {
  return (
    <div>
      <div className="flex items-baseline justify-between mb-1">
        <span className="text-xs uppercase tracking-wide text-[color:var(--color-muted)]">
          {label}
        </span>
        <span className="font-mono text-sm tabular-nums">{value.toFixed(1)}%</span>
      </div>
      <div className="h-1.5 w-full rounded-full bg-[color:var(--color-border)] overflow-hidden">
        <div
          className={`h-full transition-[width] duration-300 ease-out ${TONE_CLASS[tone]} ${widthClass(value)}`}
        />
      </div>
    </div>
  );
}
