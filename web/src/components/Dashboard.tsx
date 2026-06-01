import { useEffect, useMemo, useState } from "react";
import { Server, AlertTriangle, Cpu, MemoryStick, HardDrive, Settings2, Search } from "lucide-react";
import { HostCard, type Snapshot } from "@/components/HostCard";
import { AppButton, EmptyState, Popover, StatusPill, TooltipProvider } from "@/components/ui";
import { hostsApi, settingsApi, versionApi } from "@/lib/api";
import { cpuTone, TONE_CLASS, type StatusTone } from "@/lib/status";
import { isStale, staleAfterForIntervalMs } from "@/lib/time";
import { useStreamConnection, type WsStatus } from "@/lib/useStreamConnection";
import { useI18n } from "@/i18n/useI18n";

const STATUS_META: Record<WsStatus, { tone: StatusTone; labelKey: "dashboard.wsConnected" | "dashboard.wsConnecting" | "dashboard.wsDisconnected" | "dashboard.wsError" }> = {
  connected:    { tone: "ok",     labelKey: "dashboard.wsConnected" },
  connecting:   { tone: "warn",   labelKey: "dashboard.wsConnecting" },
  disconnected: { tone: "muted",  labelKey: "dashboard.wsDisconnected" },
  error:        { tone: "danger", labelKey: "dashboard.wsError" },
};

export function Dashboard({
  onSelectHost,
}: {
  onSelectHost?: (hostName: string) => void;
}) {
  const { t } = useI18n();
  const [snapshots, setSnapshots] = useState<Snapshot[]>([]);
  const [agentInterval, setAgentInterval] = useState("5s");
  const [latestAgentVersion, setLatestAgentVersion] = useState<string | null>(null);
  const [silencedByHost, setSilencedByHost] = useState<Record<string, string | null>>({});
  const [query, setQuery] = useState("");
  // `now` ticks every second so relative timestamps refresh without a
  // server push.
  const [now, setNow] = useState(Date.now());

  useEffect(() => {
    let cancelled = false;
    settingsApi.get()
      .then((s) => {
        if (!cancelled) setAgentInterval(s.agent_interval);
      })
      .catch(() => {});

    versionApi.get()
      .then((v) => {
        if (!cancelled) setLatestAgentVersion(v.latest_agent_version);
      })
      .catch(() => {});

    return () => { cancelled = true; };
  }, []);

  useEffect(() => {
    const tick = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(tick);
  }, []);

  // Poll the host list so the dashboard can flag silenced hosts. The
  // snapshot stream carries live metrics but not silence state — that
  // only changes on operator action, so a 30s poll is plenty.
  useEffect(() => {
    let cancelled = false;
    const refresh = () => {
      hostsApi.list()
        .then((hs) => {
          if (cancelled) return;
          const map: Record<string, string | null> = {};
          for (const h of hs) map[h.name] = h.silenced_until ?? null;
          setSilencedByHost(map);
        })
        .catch(() => {});
    };
    refresh();
    const id = window.setInterval(refresh, 30_000);
    return () => { cancelled = true; window.clearInterval(id); };
  }, []);

  const wsUrl = useMemo(() => {
    const scheme = window.location.protocol === "https:" ? "wss" : "ws";
    return `${scheme}://${window.location.host}/api/stream`;
  }, []);

  const status = useStreamConnection<Snapshot[]>({
    url: wsUrl,
    onMessage: (parsed) => setSnapshots(parsed ?? []),
  });

  const meta = STATUS_META[status];
  const sorted = useMemo(
    () => [...snapshots].sort((a, b) => a.host.localeCompare(b.host)),
    [snapshots],
  );
  const hostLabel = sorted.length === 1 ? t("dashboard.hostSingular") : t("dashboard.hostPlural");
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return sorted;
    return sorted.filter((s) => s.host.toLowerCase().includes(q));
  }, [query, sorted]);
  const staleAfterMs = useMemo(() => staleAfterForIntervalMs(agentInterval), [agentInterval]);
  const summary = useMemo(() => summarizeSnapshots(snapshots, now, staleAfterMs), [snapshots, now, staleAfterMs]);

  return (
    <div className="space-y-8">
      <header className="flex flex-col gap-2">
        <div>
          <StatusPill tone={meta.tone}>{t("dashboard.webSocket", { status: t(meta.labelKey) })}</StatusPill>
        </div>
        <h2 className="text-3xl font-bold tracking-tight text-[color:var(--color-fg)] sm:text-[2rem]">
          {t("dashboard.title")}
        </h2>
        <p className="max-w-2xl text-sm text-[color:var(--color-muted)]">
          {t("dashboard.subtitle")}
        </p>
      </header>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
        <SummaryCard icon={Server}       label={t("dashboard.hosts")}      value={summary.total} detail={`${summary.online} ${t("dashboard.online")}`} tone="ok" />
        <SummaryCard icon={AlertTriangle} label={t("dashboard.stale")}     value={summary.stale} detail={t("dashboard.noRecentTick")} tone={summary.stale > 0 ? "warn" : "muted"} />
        <SummaryCard
          icon={Cpu}
          label={t("dashboard.hottestCpu")}
          value={summary.hottestCpu ? `${summary.hottestCpu.value.toFixed(0)}%` : "—"}
          detail={summary.hottestCpu?.host ?? t("dashboard.noLiveHost")}
          tone={cpuTone(summary.hottestCpu?.value ?? 0, !summary.hottestCpu)}
        />
        <SummaryCard
          icon={MemoryStick}
          label={t("dashboard.hottestRam")}
          value={summary.hottestRam ? `${summary.hottestRam.value.toFixed(0)}%` : "—"}
          detail={summary.hottestRam?.host ?? t("dashboard.noLiveHost")}
          tone={cpuTone(summary.hottestRam?.value ?? 0, !summary.hottestRam)}
        />
        <SummaryCard
          icon={HardDrive}
          label={t("dashboard.hottestDisk")}
          value={summary.hottestDisk ? `${summary.hottestDisk.value.toFixed(0)}%` : "—"}
          detail={summary.hottestDisk?.host ?? t("dashboard.noLiveHost")}
          tone={cpuTone(summary.hottestDisk?.value ?? 0, !summary.hottestDisk)}
        />
      </div>

      {sorted.length === 0 ? (
        <EmptyState
          title={t("dashboard.noHostDataTitle")}
          detail={t("dashboard.noHostDataDescription")}
        />
      ) : (
        <TooltipProvider>
          <section>
            <div className="mb-3 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <h3 className="text-sm font-medium text-[color:var(--color-muted)]">
                {t("dashboard.monitoredHosts", { filtered: filtered.length, total: sorted.length, hostLabel })}
              </h3>
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
                <label className="relative w-full sm:w-72">
                  <span className="sr-only">{t("dashboard.searchHostsLabel")}</span>
                  <Search
                    size={14}
                    strokeWidth={1.75}
                    className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[color:var(--color-muted)]"
                  />
                  <input
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    placeholder={t("dashboard.searchHostsPlaceholder")}
                    className="w-full rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-card)] pl-9 pr-3 py-2 text-sm outline-none transition-colors placeholder:text-[color:var(--color-muted)] focus:border-[color:var(--lumen-teal)] focus:ring-2 focus:ring-[color:var(--lumen-teal)]/30"
                  />
                </label>
                <CustomizeButton />
              </div>
            </div>
            {filtered.length === 0 ? (
              <EmptyState
                title={t("dashboard.noMatchingHostsTitle")}
                detail={t("dashboard.noMatchingHostsDescription", { query })}
                className="p-8"
              />
            ) : (
              <div className="grid gap-4 [grid-template-columns:repeat(auto-fill,minmax(320px,1fr))]">
                {filtered.map((s) => (
                  <HostCard
                    key={s.host}
                    snapshot={s}
                    now={now}
                    agentInterval={agentInterval}
                    latestAgentVersion={latestAgentVersion}
                    silencedUntil={silencedByHost[s.host] ?? null}
                    onSelect={onSelectHost}
                  />
                ))}
              </div>
            )}
          </section>
        </TooltipProvider>
      )}
    </div>
  );
}

// PR1: Customize button is a disabled stub — Popover opens with a
// "coming in next release" notice. PR2 wires sort/hide/views to
// the user_prefs backend.
function CustomizeButton() {
  const { t } = useI18n();
  return (
    <Popover
      trigger={
        <AppButton variant="secondary" className="gap-2 whitespace-nowrap">
          <Settings2 size={14} strokeWidth={1.75} />
          {t("dashboard.customize")}
        </AppButton>
      }
    >
      <div className="space-y-3">
        <div>
          <div className="text-sm font-semibold">{t("dashboard.customize")}</div>
          <p className="mt-1 text-xs text-[color:var(--color-muted)]">
            {t("dashboard.customizeStub")}
          </p>
        </div>
        <ul className="space-y-1.5 text-xs text-[color:var(--color-muted)]">
          <li>· {t("dashboard.customizeSortBy")}</li>
          <li>· {t("dashboard.customizeHide")}</li>
          <li>· {t("dashboard.customizeViews")}</li>
        </ul>
      </div>
    </Popover>
  );
}

function summarizeSnapshots(snapshots: Snapshot[], now: number, staleAfterMs: number) {
  const total = snapshots.length;
  const stale = snapshots.filter((s) => isStale(s.ts, staleAfterMs, now)).length;
  const online = total - stale;
  const live = snapshots.filter((s) => !isStale(s.ts, staleAfterMs, now));
  return {
    total,
    online,
    stale,
    hottestCpu: hottest(live, (s) => s.cpu_pct),
    hottestRam: hottest(live, (s) => s.ram_pct),
    hottestDisk: hottest(live, (s) => s.disk_pct),
  };
}

function hottest(snapshots: Snapshot[], pick: (s: Snapshot) => number): { host: string; value: number } | null {
  if (snapshots.length === 0) return null;
  return snapshots.reduce(
    (best, s) => (pick(s) > best.value ? { host: s.host, value: pick(s) } : best),
    { host: snapshots[0].host, value: pick(snapshots[0]) },
  );
}

function SummaryCard({
  icon: Icon,
  label,
  value,
  detail,
  tone,
}: {
  icon: typeof Server;
  label: string;
  value: string | number;
  detail: string;
  tone: StatusTone;
}) {
  return (
    <div className="rounded-xl border border-[color:var(--color-border)] bg-[color:var(--color-card)] p-4 transition-all hover:border-[color:var(--lumen-teal)]/40 hover:shadow-[var(--shadow-2)]">
      <div className="flex items-center justify-between gap-2">
        <span className="inline-flex items-center gap-1.5 text-[11px] font-medium uppercase tracking-wide text-[color:var(--color-muted)]">
          <Icon size={14} strokeWidth={1.75} />
          {label}
        </span>
        <span aria-hidden className={`h-2 w-2 rounded-full ${TONE_CLASS[tone]}`} />
      </div>
      <div className="mt-2 text-3xl font-bold lumen-num text-[color:var(--color-fg)]">{value}</div>
      <div className="mt-1 truncate text-xs text-[color:var(--color-muted)]">{detail}</div>
    </div>
  );
}
