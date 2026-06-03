import { ArrowDown, ArrowUp, Cpu, MemoryStick, HardDrive, ArrowUpCircle, VolumeX } from "lucide-react";
import { cpuTone, TONE_CLASS, widthClass, type StatusTone } from "@/lib/status";
import { isStale, relativeTime, staleAfterForIntervalMs } from "@/lib/time";
import { Sparkline } from "@/components/Sparkline";
import { Surface } from "@/components/ui";
import { agentUpdateAvailable } from "@/lib/api";
import { formatBps } from "@/lib/format";
import { useI18n } from "@/i18n/useI18n";

export type ContainerInfo = {
  id: string;
  name: string;
  image: string;
  state: string;
  cpu_pct: number;
  mem_used_bytes: number;
  mem_limit_bytes: number;
  mem_pct: number;
};

export type SystemMetadata = {
  os?: string;
  hostname?: string;
  primary_ip?: string;
  kernel?: string;
  arch?: string;
  cpu_model?: string;
  uptime_seconds?: number;
  agent_version?: string;
  virt_type?: string;
};

export type Snapshot = {
  host: string;
  // ts: agent's collection time. Use for "how old is this datapoint".
  ts: string;
  // received_at: hub's ingest time. Use for "is the agent reaching us".
  received_at: string;
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
  containers?: ContainerInfo[];
  system?: SystemMetadata;
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
  icon: typeof Cpu;
};

function metricRow(label: string, value: number, stale: boolean, icon: typeof Cpu): MetricRow {
  return { label, value, tone: cpuTone(value, stale), icon };
}

export function HostCard({
  snapshot,
  now,
  agentInterval,
  latestAgentVersion,
  silencedUntil,
  onSelect,
}: {
  snapshot: Snapshot;
  now: number;
  agentInterval?: string;
  latestAgentVersion?: string | null;
  silencedUntil?: string | null;
  onSelect?: (hostName: string) => void;
}) {
  const { locale, t } = useI18n();
  const stale = isStale(snapshot.received_at, staleAfterForIntervalMs(agentInterval), now);
  const headerTone = cpuTone(snapshot.cpu_pct, stale);
  const updateAvailable = agentUpdateAvailable(
    snapshot.system?.agent_version,
    latestAgentVersion ?? undefined,
  );
  // Silence is operator state — the backend only emits silenced_until
  // when it's in the future, but guard anyway in case of clock drift.
  const isSilenced = !!silencedUntil && new Date(silencedUntil).getTime() > now;

  const rows: MetricRow[] = [
    metricRow(t("host.cpu"),  snapshot.cpu_pct,  stale, Cpu),
    metricRow(t("host.ram"),  snapshot.ram_pct,  stale, MemoryStick),
    metricRow(t("host.disk"), snapshot.disk_pct, stale, HardDrive),
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

  const meta = [
    snapshot.system?.os,
    snapshot.system?.primary_ip,
    snapshot.system?.agent_version ? `agent ${snapshot.system.agent_version}` : null,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <Surface
      role={interactive ? "button" : undefined}
      tabIndex={interactive ? 0 : undefined}
      onClick={interactive ? handleClick : undefined}
      onKeyDown={interactive ? handleKey : undefined}
      className={
        "p-5 " +
        (interactive
          ? "cursor-pointer transition-all duration-[var(--dur-150)] ease-[var(--ease-out)] hover:-translate-y-0.5 hover:border-[color:var(--lumen-teal)]/40 hover:shadow-[var(--shadow-2)] focus:outline-none focus:ring-2 focus:ring-[color:var(--lumen-teal)]"
          : "")
      }
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2.5">
            <span
              aria-hidden
              className={`inline-block h-2.5 w-2.5 rounded-full ${TONE_CLASS[headerTone]}`}
            />
            <span className="truncate text-lg font-semibold tracking-tight">
              {snapshot.host}
            </span>
            {isSilenced && (
              <VolumeX
                size={14}
                strokeWidth={1.75}
                className="shrink-0 text-[color:var(--color-muted)]"
                aria-label={t("host.silencedTitle", { time: silencedUntil ?? "" })}
              >
                <title>{t("host.silencedTitle", { time: silencedUntil ?? "" })}</title>
              </VolumeX>
            )}
          </div>
          {meta && (
            <div className="mt-1 truncate lumen-num text-[11px] text-[color:var(--color-muted)]">
              {meta}
            </div>
          )}
        </div>
        {updateAvailable && (
          <span
            className="inline-flex shrink-0 items-center gap-1 whitespace-nowrap rounded-full bg-[color-mix(in_oklch,var(--color-warn)_14%,var(--color-card))] px-2 py-0.5 text-[10px] font-medium text-[color:var(--color-warn)] ring-1 ring-[color:var(--color-warn)]/35"
            title={t("host.updateAvailableTitle", { version: latestAgentVersion ?? "" })}
          >
            <ArrowUpCircle size={11} strokeWidth={2.25} />
            {t("host.updateBadge")}
          </span>
        )}
      </div>

      {series.length >= 2 && (
        <div className={`mt-3 h-[24px] rounded-md bg-[color:var(--color-bg)] p-1 ${TONE_TEXT[headerTone]}`}>
          <Sparkline values={series} width={100} height={20} className="w-full h-full" />
        </div>
      )}

      <div className="mt-3 space-y-2.5">
        {rows.map((r) => (
          <MetricBar key={r.label} {...r} />
        ))}
      </div>

      <div className="mt-4 pt-3 border-t border-[color:var(--color-border)] flex items-center justify-between gap-3 text-[11px] text-[color:var(--color-muted)]">
        <span className="lumen-num inline-flex items-center gap-2.5">
          <span className="inline-flex items-center gap-1">
            <ArrowDown size={11} strokeWidth={2} />
            {formatBps(snapshot.net_rx_bps)}
          </span>
          <span className="inline-flex items-center gap-1">
            <ArrowUp size={11} strokeWidth={2} />
            {formatBps(snapshot.net_tx_bps)}
          </span>
        </span>
        <span>
          {stale
            ? t("host.staleLastSeen", { time: relativeTime(snapshot.ts, now, locale) })
            : t("host.lastSeen", { time: relativeTime(snapshot.ts, now, locale) })}
        </span>
      </div>

      {hasLoad && (
        <div className="mt-2 flex items-center justify-between text-[11px] text-[color:var(--color-muted)]">
          <span>{t("host.loadAvg")}</span>
          <span className="lumen-num">
            {snapshot.load1.toFixed(2)} · {snapshot.load5.toFixed(2)} · {snapshot.load15.toFixed(2)}
          </span>
        </div>
      )}
    </Surface>
  );
}

function MetricBar({ label, value, tone, icon: Icon }: MetricRow) {
  return (
    <div>
      <div className="flex items-baseline justify-between mb-1.5">
        <span className="inline-flex items-center gap-1.5 text-[11px] font-medium uppercase tracking-wide text-[color:var(--color-muted)]">
          <Icon size={12} strokeWidth={1.75} />
          {label}
        </span>
        <span className="lumen-num text-sm font-semibold text-[color:var(--color-fg)]">{value.toFixed(1)}%</span>
      </div>
      <div className="h-1.5 w-full rounded-full bg-[color:var(--color-border)]/60 overflow-hidden">
        <div
          className={`h-full transition-[width] duration-300 ease-out ${TONE_CLASS[tone]} ${widthClass(value)}`}
        />
      </div>
    </div>
  );
}
