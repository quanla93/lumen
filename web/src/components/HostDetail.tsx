import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { AlignedData, Options } from "uplot";
import {
  hostsApi,
  versionApi,
  agentUpdateAvailable,
  ApiError,
  type Host,
  type MetricsResponse,
  type MetricPoint,
} from "@/lib/api";
import { ArrowLeft, Clock, Cpu, MemoryStick, HardDrive, Activity, Network, Database, Thermometer, Boxes, Settings2 } from "lucide-react";
import { UPlotChart } from "@/components/UPlotChart";
import { AppButton, EmptyState, Popover, SegmentedControl, Surface } from "@/components/ui";
import type { Snapshot, ContainerInfo } from "@/components/HostCard";
import { cpuTone, TONE_CLASS, type StatusTone } from "@/lib/status";
import { copyToClipboard } from "@/lib/clipboard";
import { formatBytes, formatBps } from "@/lib/format";
import { isStale, relativeTime } from "@/lib/time";
import { useStreamConnection } from "@/lib/useStreamConnection";
import { useI18n } from "@/i18n/useI18n";
import type { Locale } from "@/i18n/types";

type Range = "1h" | "6h" | "24h";

const RANGE_SECONDS: Record<Range, number> = {
  "1h": 60 * 60,
  "6h": 6 * 60 * 60,
  "24h": 24 * 60 * 60,
};

const REFRESH_MS = 30_000;

// Canonical per-agent Compose update command. Run it in the folder that holds
// the agent's docker-compose.yml, on the target machine — never on the hub
// (the accompanying note spells out where). No fixed path is assumed.
const AGENT_UPDATE_CMD = "docker compose pull && docker compose up -d";

// Series strokes. uPlot draws to canvas, so colors are baked at construction
// time; on theme toggle the whole chart remounts via themeKey so they re-resolve.
const COLOR = {
  cpu:    "oklch(70% 0.16 145)",  // green
  ram:    "oklch(68% 0.13 240)",  // blue
  disk:   "oklch(75% 0.16 75)",   // amber
  load1:  "oklch(65% 0.22 30)",   // red
  load5:  "oklch(68% 0.14 200)",  // teal
  load15: "oklch(62% 0.12 290)",  // purple
  netRx:  "oklch(70% 0.16 145)",  // green
  netTx:  "oklch(65% 0.22 30)",   // red
  diskR:  "oklch(68% 0.13 240)",  // blue
  diskW:  "oklch(75% 0.16 75)",   // amber
  temp:   "oklch(65% 0.22 30)",   // red
};

// themeColors reads runtime CSS vars so uPlot axes adapt to dark/light.
// Called fresh inside each opts builder to avoid baking a stale palette.
function themeColors() {
  const s = getComputedStyle(document.documentElement);
  return {
    muted: s.getPropertyValue("--color-muted").trim() || "#888",
    border: s.getPropertyValue("--color-border").trim() || "#ddd",
  };
}

export function HostDetail({
  hostName,
  onBack,
}: {
  hostName: string;
  onBack: () => void;
}) {
  const { locale, t } = useI18n();
  const [range, setRange] = useState<Range>("1h");
  const [host, setHost] = useState<Host | null>(null);
  const [resp, setResp] = useState<MetricsResponse | null>(null);
  const [live, setLive] = useState<Snapshot | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [now, setNow] = useState(Date.now());
  const [latestAgentVersion, setLatestAgentVersion] = useState<string | null>(null);
  const reqIdRef = useRef(0);
  const hostId = host?.id ?? null;

  // themeKey changes when the user toggles dark/light. uPlot charts mount
  // their canvas paints with the colors that were active at construction;
  // forcing a key change remounts them with the new palette.
  const themeKey = useThemeKey();

  useEffect(() => {
    const t = window.setInterval(() => setNow(Date.now()), 1_000);
    return () => window.clearInterval(t);
  }, []);

  useEffect(() => {
    let cancelled = false;
    versionApi.get()
      .then((v) => { if (!cancelled) setLatestAgentVersion(v.latest_agent_version); })
      .catch(() => {});
    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    let cancelled = false;
    hostsApi.list().then((hosts) => {
      if (cancelled) return;
      const match = hosts.find((h) => h.name === hostName);
      if (!match) {
        setErr(t("host.removed", { host: hostName }));
        setLoading(false);
        return;
      }
      setHost(match);
    }).catch((e) => {
      if (!cancelled) {
        setErr(e instanceof ApiError ? e.message : String(e));
        setLoading(false);
      }
    });
    return () => { cancelled = true; };
  }, [hostName, t]);

  const wsUrl = useMemo(() => {
    const scheme = window.location.protocol === "https:" ? "wss" : "ws";
    return `${scheme}://${window.location.host}/api/stream`;
  }, []);

  // Re-sends the subscribe frame on every (re)connect so the server-side
  // filter survives an auto-reconnect. Without onOpen, a dropped socket
  // would come back as firehose and we'd churn through every host's
  // snapshot just to find this one.
  useStreamConnection<Snapshot[]>({
    url: wsUrl,
    onMessage: (arr) => {
      const match = arr.find((s) => s.host === hostName);
      if (match) setLive(match);
    },
    onOpen: (ws) => {
      try {
        ws.send(JSON.stringify({ type: "subscribe", hosts: [hostName] }));
      } catch { /* socket may have closed in the meantime */ }
    },
  });

  const fetchOnce = useCallback(async () => {
    if (hostId == null) return;
    const id = ++reqIdRef.current;
    const to = new Date();
    const from = new Date(to.getTime() - RANGE_SECONDS[range] * 1000);
    try {
      const r = await hostsApi.metrics(hostId, {
        from: from.toISOString(),
        to: to.toISOString(),
      });
      if (id !== reqIdRef.current) return;
      setResp(r);
      setErr(null);
    } catch (e) {
      if (id !== reqIdRef.current) return;
      setErr(e instanceof ApiError ? e.message : String(e));
    } finally {
      if (id === reqIdRef.current) setLoading(false);
    }
  }, [hostId, range]);

  useEffect(() => {
    if (hostId == null) return;
    setLoading(true);
    fetchOnce();
    const t = window.setInterval(fetchOnce, REFRESH_MS);
    return () => window.clearInterval(t);
  }, [hostId, fetchOnce]);

  const data = useMemo(() => buildSeries(resp), [resp]);
  const hasTemp = useMemo(
    () => !!resp?.points.some((p) => p.temp_c > 0),
    [resp],
  );
  // Prefer the live WS value for "current"; fall back to last historical point.
  const last = useMemo<Partial<MetricPoint & Snapshot> | null>(() => {
    if (live) return live;
    if (resp && resp.points.length > 0) return resp.points[resp.points.length - 1];
    return null;
  }, [live, resp]);

  return (
    <>
      <HostSummaryHeader
        host={host}
        live={live}
        now={now}
        range={range}
        onRangeChange={setRange}
        onBack={onBack}
        locale={locale}
        latestAgentVersion={latestAgentVersion}
        t={t}
      />

      {/* Hero KPI strip — live values at a glance, big mono numerics. */}
      <div className="mb-6 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
        <HeroStat icon={Cpu}         label={t("host.cpu")}    value={last?.cpu_pct}  unit="%" tone={cpuTone(last?.cpu_pct  ?? 0, !live)} />
        <HeroStat icon={MemoryStick} label={t("host.ram")}    value={last?.ram_pct}  unit="%" tone={cpuTone(last?.ram_pct  ?? 0, !live)} />
        <HeroStat icon={HardDrive}   label={t("host.disk")}   value={last?.disk_pct} unit="%" tone={cpuTone(last?.disk_pct ?? 0, !live)} />
        <HeroStat icon={Clock}       label={t("host.uptime")} valueText={formatUptime(live?.system?.uptime_seconds) ?? "—"} tone="muted" />
      </div>

      {live?.cpu_per_core && live.cpu_per_core.length > 0 && (
        <PerCoreStrip cores={live.cpu_per_core} t={t} />
      )}

      {err && (
        <div className="mb-4 rounded-md border border-[color:var(--color-danger)] bg-[color:var(--color-card)] px-3 py-2 text-sm text-[color:var(--color-danger)]">
          {err}
        </div>
      )}

      {loading && !resp ? (
        <p className="text-sm text-[color:var(--color-muted)]">{t("host.loadingHistory")}</p>
      ) : !resp || resp.points.length === 0 ? (
        <EmptyState
          title={t("host.noHistoryTitle")}
          detail={t("host.noHistoryDescription")}
        />
      ) : (
        <div className="space-y-4">
          {/* Featured row: CPU + RAM big */}
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            <ChartCard
              title={t("host.cpu")}
              icon={Cpu}
              badges={[swatch(COLOR.cpu, `${(last?.cpu_pct ?? 0).toFixed(1)}%`)]}
            >
              <UPlotChart
                key={`cpu-${themeKey}`}
                data={data.cpu}
                options={percentOpts(COLOR.cpu, t("host.seriesCpu"))}
                className="h-[240px] w-full"
              />
            </ChartCard>
            <ChartCard
              title={t("host.ram")}
              icon={MemoryStick}
              badges={[swatch(COLOR.ram, `${(last?.ram_pct ?? 0).toFixed(1)}%`)]}
            >
              <UPlotChart
                key={`ram-${themeKey}`}
                data={data.ram}
                options={percentOpts(COLOR.ram, t("host.ram"))}
                className="h-[240px] w-full"
              />
            </ChartCard>
          </div>

          {/* Secondary row: Disk + Load + Network + DiskIO + Temp — smaller, denser */}
          <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-4">
            <ChartCard
              title={t("host.disk")}
              icon={HardDrive}
              badges={[swatch(COLOR.disk, `${(last?.disk_pct ?? 0).toFixed(1)}%`)]}
            >
              <UPlotChart
                key={`disk-${themeKey}`}
                data={data.disk}
                options={percentOpts(COLOR.disk, t("host.disk"))}
                className="h-[170px] w-full"
              />
            </ChartCard>
            <ChartCard
              title={t("host.loadAverage")}
              icon={Activity}
              badges={[
                swatch(COLOR.load1, `1m ${(last?.load1 ?? 0).toFixed(2)}`),
                swatch(COLOR.load5, `5m ${(last?.load5 ?? 0).toFixed(2)}`),
                swatch(COLOR.load15, `15m ${(last?.load15 ?? 0).toFixed(2)}`),
              ]}
            >
              <UPlotChart
                key={`load-${themeKey}`}
                data={data.load}
                options={loadOpts([t("host.series1m"), t("host.series5m"), t("host.series15m")])}
                className="h-[170px] w-full"
              />
            </ChartCard>
            <ChartCard
              title={t("host.network")}
              icon={Network}
              badges={[
                swatch(COLOR.netRx, `↓ ${formatBps(last?.net_rx_bps ?? 0)}`),
                swatch(COLOR.netTx, `↑ ${formatBps(last?.net_tx_bps ?? 0)}`),
              ]}
            >
              <UPlotChart
                key={`net-${themeKey}`}
                data={data.net}
                options={bpsOpts([t("host.seriesDownload"), t("host.seriesUpload")], [COLOR.netRx, COLOR.netTx])}
                className="h-[170px] w-full"
              />
            </ChartCard>
            <ChartCard
              title={t("host.diskIo")}
              icon={Database}
              badges={[
                swatch(COLOR.diskR, `read ${formatBps(last?.disk_r_bps ?? 0)}`),
                swatch(COLOR.diskW, `write ${formatBps(last?.disk_w_bps ?? 0)}`),
              ]}
            >
              <UPlotChart
                key={`dio-${themeKey}`}
                data={data.diskIO}
                options={bpsOpts([t("host.seriesRead"), t("host.seriesWrite")], [COLOR.diskR, COLOR.diskW])}
                className="h-[170px] w-full"
              />
            </ChartCard>
            {hasTemp && (
              <ChartCard
                title={t("host.temperature")}
                icon={Thermometer}
                badges={[swatch(COLOR.temp, `${(last?.temp_c ?? 0).toFixed(1)}°C`)]}
              >
                <UPlotChart
                  key={`temp-${themeKey}`}
                  data={data.temp}
                  options={tempOpts(t("host.seriesTemp"))}
                  className="h-[170px] w-full"
                />
              </ChartCard>
            )}
          </div>
        </div>
      )}

      {live?.containers && live.containers.length > 0 && (
        <ContainersTable containers={live.containers} t={t} />
      )}

      {host && (
        <SilencePanel
          host={host}
          onChange={async () => {
            // refetch the host to refresh silenced_until display
            try {
              const list = await hostsApi.list();
              const match = list.find((x) => x.name === host.name);
              if (match) setHost(match);
            } catch { /* ignore — next page load will pick it up */ }
          }}
          t={t}
        />
      )}

      {(host || live) && (
        <UpdateAgentPanel
          agentVersion={live?.system?.agent_version ?? host?.system?.agent_version}
          latestAgentVersion={latestAgentVersion}
          t={t}
        />
      )}

      {resp && (
        <p className="mt-6 text-xs text-[color:var(--color-muted)]">
          {t("host.points", { points: resp.points.length, step: resp.step_seconds, refresh: Math.round(REFRESH_MS / 1000) })}
        </p>
      )}
    </>
  );
}

function HostSummaryHeader({
  host,
  live,
  now,
  range,
  onRangeChange,
  onBack,
  locale,
  latestAgentVersion,
  t,
}: {
  host: Host | null;
  live: Snapshot | null;
  now: number;
  range: Range;
  onRangeChange: (range: Range) => void;
  onBack: () => void;
  locale: Locale;
  latestAgentVersion: string | null;
  t: ReturnType<typeof useI18n>["t"];
}) {
  const lastSeen = live?.ts ?? host?.last_seen_at ?? null;
  const stale = live ? isStale(live.ts, undefined, now) : true;
  const status: { label: string; tone: StatusTone } = live && !stale
    ? { label: t("host.up"), tone: "ok" }
    : lastSeen
    ? { label: t("host.stale"), tone: "warn" }
    : { label: t("host.waiting"), tone: "muted" };
  const system = live?.system ?? host?.system;
  const rangeOptions = (Object.keys(RANGE_SECONDS) as Range[]).map((r) => ({
    value: r,
    label: rangeLabel(r, t),
  }));

  return (
    <Surface as="section" padded={false} className="mb-6 px-6 py-5">
      <button
        type="button"
        onClick={onBack}
        className="mb-3 inline-flex items-center gap-1.5 text-xs text-[color:var(--color-muted)] transition-colors hover:text-[color:var(--color-fg)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--lumen-teal)] rounded-sm"
      >
        <ArrowLeft size={13} strokeWidth={1.75} />
        {t("host.backToDashboard")}
      </button>
      <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div className="min-w-0">
          <h2 className="truncate text-3xl font-bold tracking-tight text-[color:var(--color-fg)]">
            {host?.name ?? live?.host ?? t("common.loading")}
          </h2>
          <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-[color:var(--color-muted)]">
            <MetaItem icon={<StatusIcon tone={status.tone} />} text={status.label} strong />
            <SystemMetaLine system={system} lastSeen={lastSeen} now={now} locale={locale} latestAgentVersion={latestAgentVersion} t={t} />
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <SegmentedControl
            value={range}
            onChange={onRangeChange}
            options={rangeOptions}
            ariaLabel={t("host.timeRange")}
          />
          <HostCustomizeButton t={t} />
        </div>
      </div>
    </Surface>
  );
}

// PR1.B: Customize button stub on host detail. Same pattern as
// Dashboard's CustomizeButton — opens a Popover explaining the
// upcoming personalisation features for *this* page (panel show/hide,
// default time range, density). Wired in PR2 with per-user prefs.
function HostCustomizeButton({ t }: { t: ReturnType<typeof useI18n>["t"] }) {
  return (
    <Popover
      trigger={
        <AppButton variant="secondary" className="gap-2 whitespace-nowrap">
          <Settings2 size={14} strokeWidth={1.75} />
          {t("host.customize")}
        </AppButton>
      }
    >
      <div className="space-y-3">
        <div>
          <div className="text-sm font-semibold">{t("host.customize")}</div>
          <p className="mt-1 text-xs text-[color:var(--color-muted)]">
            {t("host.customizeStub")}
          </p>
        </div>
        <ul className="space-y-1.5 text-xs text-[color:var(--color-muted)]">
          <li>· {t("host.customizeShowHide")}</li>
          <li>· {t("host.customizeDefaultRange")}</li>
          <li>· {t("host.customizeCompact")}</li>
        </ul>
      </div>
    </Popover>
  );
}

// HeroStat renders one big-number live KPI tile at the top of the
// host detail page. Pairs `value` (numeric) + `unit` for percentage
// metrics, or `valueText` (pre-formatted string) for things like
// uptime. Tone applies a colored status dot for at-a-glance read.
function HeroStat({
  icon: Icon,
  label,
  value,
  valueText,
  unit,
  tone,
}: {
  icon: typeof Cpu;
  label: string;
  value?: number;
  valueText?: string;
  unit?: string;
  tone: StatusTone;
}) {
  const display = valueText ?? (value != null ? `${value.toFixed(1)}${unit ?? ""}` : "—");
  return (
    <div className="rounded-xl border border-[color:var(--color-border)] bg-[color:var(--color-card)] p-4 transition-all hover:border-[color:var(--lumen-teal)]/40 hover:shadow-[var(--shadow-2)]">
      <div className="flex items-center justify-between gap-2">
        <span className="inline-flex items-center gap-1.5 text-[11px] font-medium uppercase tracking-wide text-[color:var(--color-muted)]">
          <Icon size={14} strokeWidth={1.75} />
          {label}
        </span>
        <span aria-hidden className={`h-2 w-2 rounded-full ${TONE_CLASS[tone]}`} />
      </div>
      <div className="mt-2 text-3xl font-bold lumen-num text-[color:var(--color-fg)]">{display}</div>
    </div>
  );
}

function SystemMetaLine({
  system,
  lastSeen,
  now,
  locale,
  latestAgentVersion,
  t,
}: {
  system?: Host["system"] | Snapshot["system"];
  lastSeen: string | null;
  now: number;
  locale: Locale;
  latestAgentVersion: string | null;
  t: ReturnType<typeof useI18n>["t"];
}) {
  const [copied, setCopied] = useState(false);
  const copyUpdateCmd = () => {
    void copyToClipboard(AGENT_UPDATE_CMD).then((ok) => {
      if (!ok) return;
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    });
  };
  const uptime = formatUptime(system?.uptime_seconds);
  const endpoint = system?.primary_ip ?? system?.hostname ?? null;
  const items: Array<{ icon: React.ReactNode; text: string }> = [];
  if (endpoint) items.push({ icon: <GlobeIcon />, text: endpoint });
  if (system?.os) items.push({ icon: <MonitorIcon />, text: system.os });
  if (uptime) items.push({ icon: <UptimeIcon />, text: uptime });
  if (system?.agent_version) {
    items.push({ icon: <TagIcon />, text: t("host.agentVersion", { version: system.agent_version }) });
  }
  if (!system && lastSeen) {
    items.push({ icon: <Clock size={14} strokeWidth={1.75} className="text-[color:var(--color-muted)]" />, text: t("host.lastSeen", { time: relativeTime(lastSeen, now, locale) }) });
  }

  const updateAvailable = agentUpdateAvailable(system?.agent_version, latestAgentVersion ?? undefined);

  if (items.length === 0 && !updateAvailable) return null;

  return (
    <>
      {items.map((item) => (
        <MetaItem key={item.text} icon={item.icon} text={item.text} />
      ))}
      {updateAvailable && (
        <button
          type="button"
          onClick={copyUpdateCmd}
          className="inline-flex items-center gap-1.5 rounded-full border border-[color:var(--color-warn)] px-2 py-0.5 text-xs font-medium text-[color:var(--color-warn)] transition-colors hover:bg-[color:var(--color-warn)] hover:text-[color:var(--color-bg)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--color-warn)]"
          title={t("host.updateCopyHint", { cmd: AGENT_UPDATE_CMD })}
        >
          {copied ? <CheckIcon /> : <UpdateIcon />}
          {copied ? t("common.copied") : t("host.updateAvailable", { version: latestAgentVersion ?? "" })}
        </button>
      )}
    </>
  );
}

function MetaItem({
  icon,
  text,
  strong,
}: {
  icon: React.ReactNode;
  text: string;
  strong?: boolean;
}) {
  return (
    <span className={`inline-flex min-w-0 items-center gap-1.5 ${strong ? "text-[color:var(--color-fg)]" : ""}`}>
      {icon}
      <span className="max-w-[320px] truncate" title={text}>{text}</span>
    </span>
  );
}

function StatusIcon({ tone }: { tone: StatusTone }) {
  const color = tone === "ok" ? "text-[color:var(--color-accent)]" : tone === "warn" ? "text-[color:var(--color-warn)]" : "text-[color:var(--color-muted)]";
  return (
    <svg aria-hidden className={`h-4 w-4 ${color}`} viewBox="0 0 16 16" fill="currentColor">
      <circle cx="8" cy="8" r="5" />
    </svg>
  );
}

function GlobeIcon() {
  return (
    <svg aria-hidden className="h-4 w-4 text-[color:var(--color-muted)]" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
      <circle cx="8" cy="8" r="5.5" />
      <path d="M2.5 8h11M8 2.5c1.4 1.5 2.1 3.3 2.1 5.5S9.4 12 8 13.5M8 2.5C6.6 4 5.9 5.8 5.9 8s.7 4 2.1 5.5" strokeLinecap="round" />
    </svg>
  );
}

function MonitorIcon() {
  return (
    <svg aria-hidden className="h-4 w-4 text-[color:var(--color-muted)]" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
      <rect x="2.5" y="3" width="11" height="7.5" rx="1.5" />
      <path d="M6.25 13h3.5M8 10.5V13" strokeLinecap="round" />
    </svg>
  );
}

function UptimeIcon() {
  return (
    <svg aria-hidden className="h-4 w-4 text-[color:var(--color-muted)]" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
      <path d="M8 2.5v3M5.5 3.25a5.5 5.5 0 1 0 5 0" strokeLinecap="round" />
    </svg>
  );
}

function TagIcon() {
  return (
    <svg aria-hidden className="h-4 w-4 text-[color:var(--color-muted)]" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
      <path d="M2.5 7.3V3a.5.5 0 0 1 .5-.5h4.3a1 1 0 0 1 .7.3l5.4 5.4a1 1 0 0 1 0 1.4l-4.3 4.3a1 1 0 0 1-1.4 0L2.8 8.7a1 1 0 0 1-.3-.7Z" strokeLinejoin="round" />
      <circle cx="5.5" cy="5.5" r="1" fill="currentColor" stroke="none" />
    </svg>
  );
}

function UpdateIcon() {
  return (
    <svg aria-hidden className="h-3.5 w-3.5" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
      <path d="M13 8a5 5 0 1 1-1.5-3.5M13 2.5v2.5h-2.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function CheckIcon() {
  return (
    <svg aria-hidden className="h-3.5 w-3.5" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2">
      <path d="m3.5 8.5 3 3 6-7" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function rangeLabel(range: Range, t: ReturnType<typeof useI18n>["t"]): string {
  if (range === "1h") return t("host.oneHour");
  if (range === "6h") return t("host.sixHours");
  return t("host.twentyFourHours");
}

function formatUptime(seconds?: number): string | null {
  if (!seconds || seconds < 0) return null;
  const days = Math.floor(seconds / 86_400);
  const hours = Math.floor((seconds % 86_400) / 3_600);
  const minutes = Math.floor((seconds % 3_600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

// ContainersTable lists every Docker container the agent reported in the
// live snapshot. Live-only (no historical query); sorted: running first,
// then alphabetical, so the top of the list is always the things actually
// burning CPU/RAM right now.
function ContainersTable({ containers, t }: { containers: ContainerInfo[]; t: ReturnType<typeof useI18n>["t"] }) {
  const sorted = useMemo(() => {
    return [...containers].sort((a, b) => {
      if (a.state === "running" && b.state !== "running") return -1;
      if (a.state !== "running" && b.state === "running") return 1;
      return a.name.localeCompare(b.name);
    });
  }, [containers]);
  const running = sorted.filter((c) => c.state === "running").length;
  return (
    <Surface as="section" padded={false} className="mt-6 rounded-lg overflow-hidden">
      <header className="flex items-center justify-between px-4 py-3 border-b border-[color:var(--color-border)]">
        <span className="inline-flex items-center gap-1.5 text-[11px] font-medium uppercase tracking-wide text-[color:var(--color-muted)]">
          <Boxes size={12} strokeWidth={1.75} />
          {t("host.containers")} · {containers.length} {t("common.total")}
        </span>
        <span className="lumen-num text-xs">
          <span className="text-[color:var(--color-muted)]">{t("common.running")}</span>{" "}
          {running}
        </span>
      </header>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-[10px] uppercase tracking-wide text-[color:var(--color-muted)]">
              <th className="px-4 py-2 font-normal">{t("common.name")}</th>
              <th className="px-2 py-2 font-normal">{t("common.state")}</th>
              <th className="px-2 py-2 font-normal">{t("common.image")}</th>
              <th className="px-2 py-2 font-normal text-right">{t("host.cpu")}</th>
              <th className="px-4 py-2 font-normal text-right">{t("common.memory")}</th>
            </tr>
          </thead>
          <tbody>
            {sorted.map((c) => (
              <ContainerRow key={c.id} c={c} />
            ))}
          </tbody>
        </table>
      </div>
    </Surface>
  );
}

// SilencePanel lets the operator suppress alerts for this host for a
// bounded window — covers planned maintenance like `docker compose pull
// && up -d` that briefly trips the offline rule. Backend stores
// silenced_until as a unix timestamp; FE renders the absolute time and
// offers a "Lift silence" button while it's active. Max window 7 days
// (server-enforced — past that, "forgot to unsilence" beats "wanted off
// for two weeks" as the failure mode).
function SilencePanel({
  host,
  onChange,
  t,
}: {
  host: Host;
  onChange: () => void | Promise<void>;
  t: ReturnType<typeof useI18n>["t"];
}) {
  const [seconds, setSeconds] = useState(3600);
  const [submitting, setSubmitting] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const silencedUntil = host.silenced_until ?? null;

  const apply = async (s: number) => {
    setSubmitting(true);
    setErr(null);
    try {
      await hostsApi.silence(host.id, s);
      await onChange();
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  };
  const lift = async () => {
    setSubmitting(true);
    setErr(null);
    try {
      await hostsApi.unsilence(host.id);
      await onChange();
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Surface as="section" padded={false} className="mt-6 rounded-lg px-4 py-4">
      <div className="mb-2 flex items-center justify-between gap-2">
        <span className="text-xs uppercase tracking-wide text-[color:var(--color-muted)]">
          {t("host.silenceTitle")}
        </span>
        {silencedUntil && (
          <span className="inline-flex items-center gap-1.5 rounded-full border border-[color:var(--color-muted)] px-2 py-0.5 text-xs text-[color:var(--color-muted)]">
            {t("host.silenceActive", { until: new Date(silencedUntil).toLocaleString() })}
          </span>
        )}
      </div>
      <p className="mb-3 text-xs text-[color:var(--color-muted)]">{t("host.silenceHint")}</p>
      <div className="flex flex-wrap items-center gap-2">
        {silencedUntil ? (
          <button
            type="button"
            onClick={lift}
            disabled={submitting}
            className="rounded-md border border-[color:var(--color-border)] px-3 py-1.5 text-sm hover:bg-[color:var(--color-bg)] disabled:opacity-50"
          >
            {submitting ? t("common.saving") : t("host.silenceLift")}
          </button>
        ) : (
          <>
            <select
              value={seconds}
              onChange={(e) => setSeconds(Number(e.target.value))}
              className="rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-2 py-1.5 text-sm"
            >
              <option value={15 * 60}>{t("host.silenceDur15m")}</option>
              <option value={60 * 60}>{t("host.silenceDur1h")}</option>
              <option value={4 * 60 * 60}>{t("host.silenceDur4h")}</option>
              <option value={24 * 60 * 60}>{t("host.silenceDur24h")}</option>
            </select>
            <button
              type="button"
              onClick={() => apply(seconds)}
              disabled={submitting}
              className="rounded-md border border-[color:var(--color-border)] px-3 py-1.5 text-sm hover:bg-[color:var(--color-bg)] disabled:opacity-50"
            >
              {submitting ? t("common.saving") : t("host.silenceApply")}
            </button>
          </>
        )}
        {err && <span className="text-xs text-[color:var(--color-danger)]">{err}</span>}
      </div>
    </Surface>
  );
}

// UpdateAgentPanel is the always-present "how to update this agent" card.
// It shows the canonical Compose update command + a copy button, plus the
// crucial note that it must run on the machine that owns the agent's
// docker-compose.yml — not on the hub. When the hub advertises a newer
// version than this host reports, it flags the update; otherwise it shows
// "up to date" (or just the reference command when versions are unknown/dev).
function UpdateAgentPanel({
  agentVersion,
  latestAgentVersion,
  t,
}: {
  agentVersion?: string;
  latestAgentVersion: string | null;
  t: ReturnType<typeof useI18n>["t"];
}) {
  const [copied, setCopied] = useState(false);
  const copy = () => {
    void copyToClipboard(AGENT_UPDATE_CMD).then((ok) => {
      if (!ok) return;
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    });
  };
  const updateAvailable = agentUpdateAvailable(agentVersion, latestAgentVersion ?? undefined);
  const upToDate = !!agentVersion && agentVersion !== "dev"
    && !!latestAgentVersion && latestAgentVersion !== "dev"
    && agentVersion === latestAgentVersion;

  return (
    <Surface as="section" padded={false} className="mt-6 rounded-lg px-4 py-4">
      <div className="mb-2 flex items-center justify-between gap-2">
        <span className="text-xs uppercase tracking-wide text-[color:var(--color-muted)]">
          {t("host.updatePanelTitle")}
        </span>
        {updateAvailable ? (
          <span className="inline-flex items-center gap-1.5 rounded-full border border-[color:var(--color-warn)] px-2 py-0.5 text-xs font-medium text-[color:var(--color-warn)]">
            <UpdateIcon />
            {t("host.updateAvailable", { version: latestAgentVersion ?? "" })}
          </span>
        ) : upToDate ? (
          <span className="inline-flex items-center gap-1.5 rounded-full border border-[color:var(--color-accent)] px-2 py-0.5 text-xs font-medium text-[color:var(--color-accent)]">
            <CheckIcon />
            {t("host.updatePanelUpToDate")}
          </span>
        ) : null}
      </div>
      <p className="mb-3 text-sm text-[color:var(--color-muted)]">
        {t("host.updatePanelDescription")}
      </p>
      <div className="flex items-center gap-2 rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-3 py-2">
        <code className="min-w-0 flex-1 overflow-x-auto whitespace-nowrap font-mono text-xs">
          {AGENT_UPDATE_CMD}
        </code>
        <button
          type="button"
          onClick={copy}
          className="inline-flex shrink-0 items-center gap-1.5 rounded-md border border-[color:var(--color-border)] px-2 py-1 text-xs transition-colors hover:bg-[color:var(--color-border)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--color-accent)]"
        >
          {copied ? <CheckIcon /> : null}
          {copied ? t("common.copied") : t("common.copy")}
        </button>
      </div>
      <p className="mt-2 text-xs text-[color:var(--color-muted)]">
        {t("host.updatePanelNote")}
      </p>
    </Surface>
  );
}

function ContainerRow({ c }: { c: ContainerInfo }) {
  const dim = c.state !== "running";
  return (
    <tr
      className={`border-t border-[color:var(--color-border)] ${dim ? "opacity-60" : ""}`}
    >
      <td className="px-4 py-2 font-mono text-xs">
        <div>{c.name}</div>
        <div className="text-[10px] text-[color:var(--color-muted)]">
          {c.id}
        </div>
      </td>
      <td className="px-2 py-2">
        <StateBadge state={c.state} />
      </td>
      <td className="px-2 py-2 font-mono text-xs text-[color:var(--color-muted)] truncate max-w-[260px]">
        {c.image}
      </td>
      <td className="px-2 py-2 text-right font-mono text-xs tabular-nums">
        {c.state === "running" ? `${c.cpu_pct.toFixed(1)}%` : "—"}
      </td>
      <td className="px-4 py-2 text-right font-mono text-xs tabular-nums">
        {c.state === "running" ? (
          <div className="flex items-center justify-end gap-2">
            <span>{formatBytes(c.mem_used_bytes)}</span>
            <span className="text-[color:var(--color-muted)]">
              / {formatBytes(c.mem_limit_bytes)}
            </span>
            <span
              className={
                c.mem_pct >= 90
                  ? "text-[color:var(--color-danger)]"
                  : c.mem_pct >= 70
                  ? "text-[color:var(--color-warn)]"
                  : ""
              }
            >
              ({c.mem_pct.toFixed(0)}%)
            </span>
          </div>
        ) : (
          "—"
        )}
      </td>
    </tr>
  );
}

function StateBadge({ state }: { state: string }) {
  const tone =
    state === "running" ? "lumen-status-ok"
    : state === "paused" ? "lumen-status-warn"
    : state === "restarting" ? "lumen-status-warn"
    : "lumen-status-muted";
  return (
    <span className="inline-flex items-center gap-1.5 font-mono text-xs">
      <span aria-hidden className={`inline-block h-2 w-2 rounded-full ${tone}`} />
      {state}
    </span>
  );
}

// PerCoreStrip renders one tile per logical core in an auto-wrapping grid.
// Tile width adapts to core count so 1-core VMs don't look empty and
// 64-core servers don't paginate forever:
//   ≤ 8 cores  → wide tiles with idx + percentage label below
//   ≤ 32 cores → medium tiles, percentage label only
//   > 32 cores → compact tiles (no labels, just colored fill)
// Empty tracks stay visible at 0% so the operator can always count cores.
function PerCoreStrip({ cores, t }: { cores: number[]; t: ReturnType<typeof useI18n>["t"] }) {
  const n = cores.length;
  const avg = cores.reduce((a, b) => a + b, 0) / n;
  const coreLabel = n === 1 ? t("host.coreSingular") : t("host.corePlural");

  const layout = n <= 8
    ? { tile: 64, height: 56, labels: "full" as const }
    : n <= 32
    ? { tile: 44, height: 48, labels: "pct" as const }
    : n <= 64
    ? { tile: 28, height: 40, labels: "none" as const }
    : { tile: 18, height: 32, labels: "none" as const };

  return (
    <Surface padded={false} className="mb-4 rounded-lg px-4 py-3">
      <div className="mb-3 flex items-center justify-between">
        <span className="text-xs uppercase tracking-wide text-[color:var(--color-muted)]">
          {t("host.perCoreCpu")} · {t("host.cores", { count: n, coreLabel })}
        </span>
        <span className="font-mono text-xs">
          <span className="text-[color:var(--color-muted)]">{t("common.avg")}</span>{" "}
          {avg.toFixed(1)}%
        </span>
      </div>
      <div
        className="grid gap-1.5"
        style={{ gridTemplateColumns: `repeat(auto-fit, ${layout.tile}px)` }}
      >
        {cores.map((pct, i) => (
          <CoreTile
            key={i}
            idx={i}
            pct={pct}
            height={layout.height}
            labels={layout.labels}
            t={t}
          />
        ))}
      </div>
    </Surface>
  );
}

function CoreTile({
  idx,
  pct,
  height,
  labels,
  t,
}: {
  idx: number;
  pct: number;
  height: number;
  labels: "full" | "pct" | "none";
  t: ReturnType<typeof useI18n>["t"];
}) {
  const tone = pct >= 90 ? "danger" : pct >= 60 ? "warn" : "ok";
  const toneClass =
    tone === "danger" ? "lumen-status-danger"
    : tone === "warn" ? "lumen-status-warn"
    : "lumen-status-ok";
  const fillPct = Math.min(100, pct);

  return (
    <div
      className="flex flex-col items-center gap-1"
      title={t("host.coreTooltip", { index: idx, pct: pct.toFixed(1) })} /* tooltip works even in compact mode */
    >
      <div
        className="relative w-full rounded-sm border border-[color:var(--color-border)] bg-[color:var(--color-bg)] overflow-hidden"
        style={{ height: `${height}px` }}
      >
        {pct > 0 && (
          <div
            className={`absolute bottom-0 left-0 right-0 ${toneClass} opacity-85 transition-[height] duration-300`}
            style={{ height: `${fillPct}%` }}
          />
        )}
      </div>
      {labels === "full" && (
        <div className="flex w-full items-center justify-between font-mono text-[10px] tabular-nums">
          <span className="text-[color:var(--color-muted)]">{idx}</span>
          <span>{pct.toFixed(0)}%</span>
        </div>
      )}
      {labels === "pct" && (
        <span className="font-mono text-[10px] tabular-nums">
          {pct.toFixed(0)}
        </span>
      )}
      {/* labels === "none" → no text; hover tooltip on the wrapper still surfaces idx + pct */}
    </div>
  );
}

function ChartCard({
  title,
  icon: Icon,
  badges,
  children,
}: {
  title: string;
  icon?: typeof Cpu;
  badges?: React.ReactNode[];
  children: React.ReactNode;
}) {
  return (
    <Surface padded={false} className="rounded-lg p-3 transition-colors hover:border-[color:var(--lumen-teal)]/30">
      <div className="mb-2 flex items-center justify-between gap-2 flex-wrap">
        <span className="inline-flex items-center gap-1.5 text-[11px] font-medium uppercase tracking-wide text-[color:var(--color-muted)]">
          {Icon && <Icon size={12} strokeWidth={1.75} />}
          {title}
        </span>
        {badges && badges.length > 0 && (
          <div className="lumen-num flex items-center gap-3 text-xs">
            {badges.map((b, i) => <span key={i}>{b}</span>)}
          </div>
        )}
      </div>
      {children}
    </Surface>
  );
}

function swatch(color: string, text: string) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span
        aria-hidden
        className="inline-block h-2 w-2 rounded-[2px]"
        style={{ backgroundColor: color }}
      />
      {text}
    </span>
  );
}

function buildSeries(r: MetricsResponse | null) {
  if (!r || r.points.length === 0) {
    const empty: AlignedData = [[]];
    return { cpu: empty, ram: empty, disk: empty, load: empty, net: empty, diskIO: empty, temp: empty };
  }
  const xs = r.points.map((p) => Math.floor(new Date(p.ts).getTime() / 1000));
  return {
    cpu:    [xs, r.points.map((p) => p.cpu_pct)] as AlignedData,
    ram:    [xs, r.points.map((p) => p.ram_pct)] as AlignedData,
    disk:   [xs, r.points.map((p) => p.disk_pct)] as AlignedData,
    load: [
      xs,
      r.points.map((p) => p.load1),
      r.points.map((p) => p.load5),
      r.points.map((p) => p.load15),
    ] as AlignedData,
    net: [
      xs,
      r.points.map((p) => p.net_rx_bps),
      r.points.map((p) => p.net_tx_bps),
    ] as AlignedData,
    diskIO: [
      xs,
      r.points.map((p) => p.disk_r_bps),
      r.points.map((p) => p.disk_w_bps),
    ] as AlignedData,
    temp: [xs, r.points.map((p) => p.temp_c)] as AlignedData,
  };
}

// Theme-aware base options. uPlot uses axis.stroke for tick labels and
// axis.grid.stroke for the faint guide lines; defaults are black/grey
// which look invisible in dark mode. We resolve the CSS vars at chart
// build time and remount on theme toggle (see themeKey).
function baseAxes(yValues?: (u: uPlot, vals: number[]) => string[], leftSize = 50) {
  const t = themeColors();
  return [
    { stroke: t.muted, grid: { stroke: t.border, width: 1 }, ticks: { stroke: t.border } },
    {
      stroke: t.muted,
      grid: { stroke: t.border, width: 1 },
      ticks: { stroke: t.border },
      size: leftSize,
      ...(yValues ? { values: yValues } : {}),
    },
  ];
}

// Disable uPlot's built-in legend overlay — it renders as a wide
// single-row <table> that overflows the chart card when nowrap is
// applied. Replaced by a custom near-cursor tooltip plugin below.
function legend() {
  return {
    show: false,
  };
}

// Custom hover tooltip plugin — replaces the default uPlot legend with
// a stacked label:value list anchored near the cursor. Constrained to
// the chart's bbox so it never overflows into adjacent panels.
function lumenTooltipPlugin(): uPlot.Plugin {
  let tooltip: HTMLDivElement | null = null;
  return {
    hooks: {
      init: (u) => {
        tooltip = document.createElement("div");
        tooltip.className = "lumen-chart-tooltip";
        tooltip.style.display = "none";
        u.root.appendChild(tooltip);
      },
      setCursor: (u) => {
        if (!tooltip) return;
        const { idx, left, top } = u.cursor;
        if (idx == null || left == null || top == null || left < 0 || top < 0) {
          tooltip.style.display = "none";
          return;
        }
        const xs = u.data[0];
        if (!xs || idx >= xs.length) {
          tooltip.style.display = "none";
          return;
        }
        const tsSeconds = xs[idx] as number;
        const time = new Date(tsSeconds * 1000).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
        let html = `<div class="t">${time}</div>`;
        for (let i = 1; i < u.series.length; i++) {
          const s = u.series[i];
          if (!s.show) continue;
          const raw = u.data[i]?.[idx];
          let valStr: string;
          if (raw == null) {
            valStr = "—";
          } else if (typeof s.value === "function") {
            valStr = String(s.value(u, raw, i, idx));
          } else {
            valStr = String(raw);
          }
          const color = typeof s.stroke === "function" ? s.stroke(u, i) : s.stroke ?? "currentColor";
          html += `<div class="r"><span class="dot" style="background:${color}"></span><span class="l">${s.label ?? ""}</span><span class="v">${valStr}</span></div>`;
        }
        tooltip.innerHTML = html;
        tooltip.style.display = "block";
        // Use the `.u-over` element rect for boundary math — its dimensions
        // are in CSS pixels, while u.bbox.* is in canvas/DPR units which
        // double-counts on HiDPI displays and breaks the overflow check.
        const overRect = u.over.getBoundingClientRect();
        const tw = tooltip.offsetWidth;
        const th = tooltip.offsetHeight;
        let x = left + 14;
        let y = top + 14;
        if (x + tw > overRect.width)  x = left - tw - 14;
        if (y + th > overRect.height) y = top  - th - 14;
        if (x < 4) x = 4;
        if (y < 4) y = 4;
        if (x + tw > overRect.width)  x = overRect.width - tw - 4;
        if (y + th > overRect.height) y = overRect.height - th - 4;
        // Tooltip is appended to u.root (chart container, parent of .u-over).
        // Add the over element's offset relative to that root.
        const rootRect = u.root.getBoundingClientRect();
        const offsetLeft = overRect.left - rootRect.left;
        const offsetTop  = overRect.top  - rootRect.top;
        tooltip.style.left = `${offsetLeft + x}px`;
        tooltip.style.top  = `${offsetTop  + y}px`;
      },
      destroy: () => {
        tooltip?.remove();
        tooltip = null;
      },
    },
  };
}

// gradientFill builds a Grafana-style vertical alpha gradient under the
// line. Anchors the strong end to the series' max value (where the line
// actually sits) so every chart shows a vivid fill near the line — not
// just auto-scaled ones. Without this, fixed-range scales (e.g. percent
// 0-100) drew the line in the low-alpha bottom of the bbox gradient and
// the fill looked invisible.
function gradientFill(color: string, topAlpha = 0.32) {
  const flatFallback = withAlpha(color, topAlpha * 0.5);
  return (u: uPlot, seriesIdx: number): CanvasGradient | string => {
    const ctx = u.ctx;
    const bbox = u.bbox;
    if (!ctx || !bbox) return flatFallback;

    const data = u.data?.[seriesIdx] as Array<number | null | undefined> | undefined;
    if (!data || data.length === 0) return flatFallback;

    let maxVal = -Infinity;
    for (let i = 0; i < data.length; i++) {
      const v = data[i];
      if (v != null && Number.isFinite(v) && v > maxVal) maxVal = v;
    }
    if (!Number.isFinite(maxVal)) return flatFallback;

    const scaleKey = u.series[seriesIdx]?.scale ?? "y";
    const topPx = u.valToPos(maxVal, scaleKey, true);
    const botPx = bbox.top + bbox.height;
    if (!Number.isFinite(topPx) || !Number.isFinite(botPx) || topPx >= botPx) {
      return flatFallback;
    }

    const grad = ctx.createLinearGradient(0, topPx, 0, botPx);
    grad.addColorStop(0, withAlpha(color, topAlpha));
    grad.addColorStop(1, withAlpha(color, 0));
    return grad;
  };
}

function withAlpha(oklch: string, alpha: number): string {
  // "oklch(70% 0.16 145)" → "oklch(70% 0.16 145 / 0.32)"
  return oklch.replace(/\)$/, ` / ${alpha})`);
}

function percentOpts(color: string, label: string): Omit<Options, "width" | "height"> {
  return {
    scales: { y: { range: [0, 100] } },
    axes: baseAxes((_u, vals) => vals.map((v) => `${v}%`), 44),
    legend: legend(),
    plugins: [lumenTooltipPlugin()],
    series: [
      {},
      {
        label,
        value: (_u, v) => v == null ? "—" : `${v.toFixed(1)}%`,
        stroke: color,
        width: 2,
        fill: gradientFill(color),
        points: { show: false },
      },
    ],
  };
}

function loadOpts(labels: [string, string, string]): Omit<Options, "width" | "height"> {
  return {
    axes: baseAxes(undefined, 44),
    legend: legend(),
    plugins: [lumenTooltipPlugin()],
    series: [
      {},
      { label: labels[0], value: (_u, v) => v == null ? "—" : v.toFixed(2), stroke: COLOR.load1,  width: 2,   fill: gradientFill(COLOR.load1, 0.18), points: { show: false } },
      { label: labels[1], value: (_u, v) => v == null ? "—" : v.toFixed(2), stroke: COLOR.load5,  width: 1.5, points: { show: false } },
      { label: labels[2], value: (_u, v) => v == null ? "—" : v.toFixed(2), stroke: COLOR.load15, width: 1.5, points: { show: false } },
    ],
  };
}

function bpsOpts(labels: [string, string], colors: [string, string]): Omit<Options, "width" | "height"> {
  return {
    axes: baseAxes((_u, vals) => vals.map((v) => formatBps(v)), 80),
    legend: legend(),
    plugins: [lumenTooltipPlugin()],
    series: [
      {},
      { label: labels[0], value: (_u, v) => v == null ? "—" : formatBps(v), stroke: colors[0], width: 2, fill: gradientFill(colors[0], 0.22), points: { show: false } },
      { label: labels[1], value: (_u, v) => v == null ? "—" : formatBps(v), stroke: colors[1], width: 2, fill: gradientFill(colors[1], 0.22), points: { show: false } },
    ],
  };
}

function tempOpts(label: string): Omit<Options, "width" | "height"> {
  return {
    axes: baseAxes((_u, vals) => vals.map((v) => `${v}°`), 44),
    legend: legend(),
    plugins: [lumenTooltipPlugin()],
    series: [
      {},
      { label, value: (_u, v) => v == null ? "—" : `${v.toFixed(1)}°C`, stroke: COLOR.temp, width: 2, fill: gradientFill(COLOR.temp), points: { show: false } },
    ],
  };
}

// useThemeKey returns a counter that bumps whenever the `dark` class is
// added/removed on <html>. Components key off this to force-remount
// canvas-backed children (uPlot) so they re-read CSS vars.
function useThemeKey(): number {
  const [k, setK] = useState(0);
  useEffect(() => {
    const obs = new MutationObserver(() => setK((x) => x + 1));
    obs.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["class"],
    });
    return () => obs.disconnect();
  }, []);
  return k;
}

// Pull in uPlot type only for the function signature above; not exported.
import type uPlot from "uplot";
