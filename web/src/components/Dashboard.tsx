import { useEffect, useMemo, useRef, useState } from "react";
import { Server, AlertTriangle, Cpu, MemoryStick, HardDrive, Settings2, Search, EyeOff, Eye, Trash2, Bookmark, Sparkles, CheckCircle2, ArrowRight, Circle } from "lucide-react";
import { HostCard, type Snapshot } from "@/components/HostCard";
import { AppButton, EmptyState, IconButton, Popover, SegmentedControl, StatusPill, Surface, TooltipProvider } from "@/components/ui";
import { hostsApi, settingsApi, versionApi, type SavedView, type SortBy, type SortDir } from "@/lib/api";
import { cpuTone, TONE_CLASS, type StatusTone } from "@/lib/status";
import { isStale, staleAfterForIntervalMs } from "@/lib/time";
import { useStreamConnection, type WsStatus } from "@/lib/useStreamConnection";
import { usePrefs } from "@/lib/userPrefs";
import { useI18n } from "@/i18n/useI18n";

const STATUS_META: Record<WsStatus, { tone: StatusTone; labelKey: "dashboard.wsConnected" | "dashboard.wsConnecting" | "dashboard.wsDisconnected" | "dashboard.wsError" }> = {
  connected:    { tone: "ok",     labelKey: "dashboard.wsConnected" },
  connecting:   { tone: "warn",   labelKey: "dashboard.wsConnecting" },
  disconnected: { tone: "muted",  labelKey: "dashboard.wsDisconnected" },
  error:        { tone: "danger", labelKey: "dashboard.wsError" },
};

export function Dashboard({
  onSelectHost,
  onNavigateToSettings,
}: {
  onSelectHost?: (hostName: string) => void;
  onNavigateToSettings?: () => void;
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
  const { dashboard: dashPrefs } = usePrefs();

  // Sort + hide pipeline. Snapshot.host is the stable identifier (the
  // public API uses the same string), so hiddenHostIds stores host
  // names directly — no UUID indirection on the wire.
  const sorted = useMemo(() => {
    const hiddenSet = new Set(dashPrefs.hiddenHostIds);
    const visible = snapshots.filter((s) => !hiddenSet.has(s.host));
    return sortSnapshots(visible, dashPrefs.sortBy, dashPrefs.sortDir);
  }, [snapshots, dashPrefs.hiddenHostIds, dashPrefs.sortBy, dashPrefs.sortDir]);
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

      <TooltipProvider>
        <section>
          <div className="mb-3 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <h3 className="text-sm font-medium text-[color:var(--color-muted)]">
              {snapshots.length === 0
                ? t("dashboard.monitoredHostsEmpty")
                : t("dashboard.monitoredHosts", { filtered: filtered.length, total: sorted.length, hostLabel })}
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
              <CustomizeButton snapshots={snapshots} />
            </div>
          </div>
          {snapshots.length === 0 ? (
            <WelcomeCard onAddHost={onNavigateToSettings} t={t} />
          ) : sorted.length === 0 ? (
            <EmptyState
              title={t("dashboard.allHiddenTitle")}
              detail={t("dashboard.allHiddenDescription", { count: dashPrefs.hiddenHostIds.length })}
              className="p-8"
            />
          ) : filtered.length === 0 ? (
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
    </div>
  );
}

// MAX_SAVED_VIEWS mirrors the server cap (`maxSavedViews` in
// internal/hub/userprefs). Keep these in lockstep.
const MAX_SAVED_VIEWS = 5;

// CustomizeButton wires Dashboard prefs (sort + hidden hosts + saved
// views) through usePrefs. Saved views bundle a sort+hide combination
// the operator can re-apply later; max 5 per server validator.
function CustomizeButton({ snapshots }: { snapshots: Snapshot[] }) {
  const { t } = useI18n();
  const { dashboard, updateDashboard } = usePrefs();
  const [viewName, setViewName] = useState("");

  // Remember the view ID that was just orphaned by patch() so the
  // "Update active view" button can offer to write the new state
  // back into it (instead of forcing the operator to delete +
  // recreate, which loses the view ID and any external bookmark
  // references). Cleared when the remembered view is no longer in
  // dashboard.views (operator deleted it) or when its stored
  // state matches the current dashboard (no diff to save).
  const orphanedViewIdRef = useRef<string | null>(null);

  const hiddenSet = new Set(dashboard.hiddenHostIds);
  const visibleNames = snapshots.map((s) => s.host).filter((n) => !hiddenSet.has(n));
  // Hidden list may include hosts that aren't in the current snapshot
  // (deleted, offline beyond store TTL). Show all so the operator can
  // still un-hide them without re-adding the host first.
  const hiddenNames = [...dashboard.hiddenHostIds];

  // patch() handles direct mutations from the sort/hide controls. Any
  // such change diverges the dashboard from whatever view is "active"
  // — clear activeViewId so the highlight reflects reality. We also
  // remember the just-orphaned view ID so updateActiveView() can
  // offer to write the new state back into it.
  function patch(next: Partial<typeof dashboard>) {
    if (dashboard.activeViewId) {
      orphanedViewIdRef.current = dashboard.activeViewId;
    }
    updateDashboard({ ...dashboard, ...next, activeViewId: null }).catch(() => {});
  }

  function applyView(view: SavedView) {
    updateDashboard({
      ...dashboard,
      sortBy: view.sortBy,
      sortDir: view.sortDir,
      defaultMetric: view.defaultMetric,
      hiddenHostIds: [...view.hiddenHostIds],
      activeViewId: view.id,
    }).catch(() => {});
  }

  function saveAsNew() {
    const name = viewName.trim();
    if (!name || name.length > 32) return;
    if (dashboard.views.length >= MAX_SAVED_VIEWS) return;
    const id = (typeof crypto !== "undefined" && "randomUUID" in crypto)
      ? crypto.randomUUID()
      : `view-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
    const view: SavedView = {
      id,
      name,
      sortBy: dashboard.sortBy === "tag" ? "name" : dashboard.sortBy,
      sortDir: dashboard.sortDir,
      defaultMetric: dashboard.defaultMetric,
      hiddenHostIds: [...dashboard.hiddenHostIds],
    };
    updateDashboard({
      ...dashboard,
      views: [...dashboard.views, view],
      activeViewId: id,
    }).catch(() => {});
    setViewName("");
  }

  function deleteView(id: string) {
    updateDashboard({
      ...dashboard,
      views: dashboard.views.filter((v) => v.id !== id),
      activeViewId: dashboard.activeViewId === id ? null : dashboard.activeViewId,
    }).catch(() => {});
  }

  // updateActiveView writes the current dashboard state into the
  // view that was just orphaned by patch(). Restores
  // activeViewId so the bookmark highlight returns. After the
  // update, the remembered view ID is cleared because the
  // operator's next action should start from a clean slate —
  // they'll either keep the now-applied view active (no
  // orphaned view) or diverge again, which orphans a new ID.
  function updateActiveView() {
    const id = orphanedViewIdRef.current;
    if (!id) return;
    const view = dashboard.views.find((v) => v.id === id);
    if (!view) {
      // Remembered view was deleted; drop the orphan marker.
      orphanedViewIdRef.current = null;
      return;
    }
    const next: SavedView = {
      ...view,
      sortBy: dashboard.sortBy === "tag" ? "name" : dashboard.sortBy,
      sortDir: dashboard.sortDir,
      defaultMetric: dashboard.defaultMetric,
      hiddenHostIds: [...dashboard.hiddenHostIds],
    };
    orphanedViewIdRef.current = null;
    updateDashboard({
      ...dashboard,
      views: dashboard.views.map((v) => (v.id === id ? next : v)),
      activeViewId: id,
    }).catch(() => {});
  }

  // Compute the "Update active view" button visibility + label.
  // The button shows when:
  //   (a) activeViewId is null (the operator diverged)
  //   (b) the remembered view ID still exists in dashboard.views
  //   (c) current state differs from the view's stored state
  //       (no point updating if nothing changed)
  const orphanedId = orphanedViewIdRef.current;
  const orphanedView = orphanedId ? dashboard.views.find((v) => v.id === orphanedId) : null;
  const currentDivergesFromOrphaned =
    orphanedView !== undefined &&
    orphanedView !== null &&
    (orphanedView.sortBy !== (dashboard.sortBy === "tag" ? "name" : dashboard.sortBy) ||
      orphanedView.sortDir !== dashboard.sortDir ||
      orphanedView.defaultMetric !== dashboard.defaultMetric ||
      orphanedView.hiddenHostIds.length !== dashboard.hiddenHostIds.length ||
      orphanedView.hiddenHostIds.some((h, i) => h !== dashboard.hiddenHostIds[i]));
  const showUpdateActive = orphanedView !== null && currentDivergesFromOrphaned;
  // Reset the orphan marker if the view was deleted (computed
  // from the views array on every render, so a delete naturally
  // clears the marker).
  if (orphanedId && !orphanedView) {
    orphanedViewIdRef.current = null;
  }

  return (
    <Popover
      trigger={
        <AppButton variant="secondary" className="gap-2 whitespace-nowrap">
          <Settings2 size={14} strokeWidth={1.75} />
          {t("dashboard.customize")}
        </AppButton>
      }
    >
      <div className="w-72 space-y-4">
        <div>
          <div className="text-sm font-semibold">{t("dashboard.customize")}</div>
        </div>

        <div className="space-y-1.5">
          <label className="block text-xs font-medium text-[color:var(--color-muted)]">
            {t("dashboard.customizeSortBy")}
          </label>
          <SegmentedControl<SortBy>
            ariaLabel={t("dashboard.customizeSortBy")}
            value={dashboard.sortBy === "tag" ? "name" : dashboard.sortBy}
            size="sm"
            onChange={(v) => patch({ sortBy: v })}
            options={[
              { value: "name",      label: t("dashboard.customizeSortName") },
              { value: "hottest",   label: t("dashboard.customizeSortHottest") },
              { value: "last-seen", label: t("dashboard.customizeSortLastSeen") },
            ]}
          />
          <SegmentedControl<SortDir>
            ariaLabel={t("dashboard.customizeSortDir")}
            value={dashboard.sortDir}
            size="sm"
            onChange={(v) => patch({ sortDir: v })}
            options={[
              { value: "asc",  label: "↑" },
              { value: "desc", label: "↓" },
            ]}
          />
        </div>

        <div className="space-y-1.5">
          <label className="block text-xs font-medium text-[color:var(--color-muted)]">
            {t("dashboard.customizeHide")}
          </label>
          {visibleNames.length === 0 ? (
            <p className="text-xs text-[color:var(--color-muted)]">{t("dashboard.customizeHideNoVisible")}</p>
          ) : (
            <select
              className="w-full rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-2 py-1 text-xs"
              value=""
              onChange={(e) => {
                if (!e.target.value) return;
                patch({ hiddenHostIds: [...dashboard.hiddenHostIds, e.target.value] });
              }}
            >
              <option value="">{t("dashboard.customizeHidePlaceholder")}</option>
              {visibleNames.map((n) => <option key={n} value={n}>{n}</option>)}
            </select>
          )}
          {hiddenNames.length > 0 && (
            <ul className="mt-1 space-y-1">
              {hiddenNames.map((n) => (
                <li key={n} className="flex items-center justify-between gap-2 rounded-md border border-[color:var(--color-border)] px-2 py-1 text-xs">
                  <span className="flex items-center gap-1.5 truncate">
                    <EyeOff size={12} strokeWidth={1.75} className="text-[color:var(--color-muted)]" />
                    <span className="truncate">{n}</span>
                  </span>
                  <IconButton
                    label={t("dashboard.customizeRestoreAria", { name: n })}
                    onClick={() => patch({ hiddenHostIds: dashboard.hiddenHostIds.filter((x) => x !== n) })}
                  >
                    <Eye size={12} strokeWidth={1.75} />
                  </IconButton>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="space-y-1.5 border-t border-[color:var(--color-border)] pt-3">
          {showUpdateActive && orphanedView && (
            <AppButton
              type="button"
              variant="secondary"
              onClick={updateActiveView}
              aria-label={t("dashboard.savedViewUpdateAria", { name: orphanedView.name })}
              className="w-full gap-1.5 px-2 py-1 text-xs"
            >
              <Sparkles size={12} strokeWidth={1.75} />
              <span className="truncate">{t("dashboard.savedViewUpdate", { name: orphanedView.name })}</span>
            </AppButton>
          )}
          <div className="flex items-center justify-between gap-2">
            <label className="block text-xs font-medium text-[color:var(--color-muted)]">
              {t("dashboard.savedViews")}
            </label>
            <span className="text-[10px] text-[color:var(--color-muted)]">
              {t("dashboard.savedViewCapHint", { used: dashboard.views.length, max: MAX_SAVED_VIEWS })}
            </span>
          </div>
          {dashboard.views.length === 0 ? (
            <p className="text-xs text-[color:var(--color-muted)]">{t("dashboard.savedViewsEmpty")}</p>
          ) : (
            <ul className="space-y-1">
              {dashboard.views.map((v) => {
                const active = v.id === dashboard.activeViewId;
                return (
                  <li
                    key={v.id}
                    className={`flex items-center justify-between gap-2 rounded-md border px-2 py-1 text-xs ${
                      active
                        ? "border-[color:var(--lumen-teal)] bg-[color:var(--color-bg)]"
                        : "border-[color:var(--color-border)]"
                    }`}
                  >
                    <button
                      type="button"
                      onClick={() => applyView(v)}
                      aria-label={t("dashboard.savedViewApplyAria", { name: v.name })}
                      className="flex flex-1 items-center gap-1.5 truncate text-left hover:text-[color:var(--lumen-teal)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--lumen-teal)]"
                    >
                      <Bookmark size={12} strokeWidth={1.75} className={active ? "text-[color:var(--lumen-teal)]" : "text-[color:var(--color-muted)]"} />
                      <span className="truncate">{v.name}</span>
                      {active && (
                        <span className="text-[10px] uppercase tracking-wide text-[color:var(--lumen-teal)]">
                          {t("dashboard.savedViewActive")}
                        </span>
                      )}
                    </button>
                    <IconButton
                      label={t("dashboard.savedViewDeleteAria", { name: v.name })}
                      onClick={() => deleteView(v.id)}
                    >
                      <Trash2 size={12} strokeWidth={1.75} />
                    </IconButton>
                  </li>
                );
              })}
            </ul>
          )}
          {dashboard.views.length < MAX_SAVED_VIEWS && (
            <form
              onSubmit={(e) => { e.preventDefault(); saveAsNew(); }}
              className="flex gap-1.5 pt-0.5"
            >
              <input
                type="text"
                value={viewName}
                onChange={(e) => setViewName(e.target.value)}
                placeholder={t("dashboard.savedViewNamePlaceholder")}
                maxLength={32}
                className="flex-1 rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-2 py-1 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--lumen-teal)]"
              />
              <button
                type="submit"
                disabled={!viewName.trim()}
                aria-label={t("dashboard.savedViewSaveAria")}
                className="rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-2 py-1 text-xs disabled:opacity-50 hover:border-[color:var(--lumen-teal)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--lumen-teal)]"
              >
                {t("dashboard.savedViewSave")}
              </button>
            </form>
          )}
        </div>
      </div>
    </Popover>
  );
}

// sortSnapshots applies the operator's sortBy/sortDir from
// dashboard prefs. 'tag' (in the prefs schema) is not supported in
// PR2 and falls back to 'name'.
function sortSnapshots(snapshots: Snapshot[], sortBy: SortBy, dir: SortDir): Snapshot[] {
  const sign = dir === "desc" ? -1 : 1;
  const cmp = (a: Snapshot, b: Snapshot): number => {
    switch (sortBy) {
      case "hottest": {
        const ha = Math.max(a.cpu_pct, a.ram_pct, a.disk_pct);
        const hb = Math.max(b.cpu_pct, b.ram_pct, b.disk_pct);
        if (ha === hb) return a.host.localeCompare(b.host);
        return hb - ha; // hottest first regardless of direction; direction below flips
      }
      case "last-seen":
        return new Date(b.received_at).getTime() - new Date(a.received_at).getTime();
      case "name":
      case "tag":
      default:
        return a.host.localeCompare(b.host);
    }
  };
  return [...snapshots].sort((a, b) => sign * cmp(a, b));
}

function summarizeSnapshots(snapshots: Snapshot[], now: number, staleAfterMs: number) {
  const total = snapshots.length;
  const stale = snapshots.filter((s) => isStale(s.received_at, staleAfterMs, now)).length;
  const online = total - stale;
  const live = snapshots.filter((s) => !isStale(s.received_at, staleAfterMs, now));
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

// WelcomeCard renders the first-run onboarding wizard — replaces the
// bare "no host data" empty state with a numbered 4-step guide that
// tells a fresh operator exactly what to do next. Step 2 (Add host)
// is the only actionable step and routes to Settings → Hosts via the
// onAddHost callback. Auto-disappears the moment any host registers,
// no schema flag needed — `snapshots.length === 0` is the gate.
function WelcomeCard({
  onAddHost,
  t,
}: {
  onAddHost?: () => void;
  t: ReturnType<typeof useI18n>["t"];
}) {
  const steps = [
    {
      done: true,
      title: t("dashboard.welcomeStep1Title"),
      detail: t("dashboard.welcomeStep1Detail"),
    },
    {
      done: false,
      title: t("dashboard.welcomeStep2Title"),
      detail: t("dashboard.welcomeStep2Detail"),
      cta: onAddHost
        ? { label: t("dashboard.welcomeStep2Cta"), onClick: onAddHost }
        : undefined,
    },
    {
      done: false,
      title: t("dashboard.welcomeStep3Title"),
      detail: t("dashboard.welcomeStep3Detail"),
    },
    {
      done: false,
      title: t("dashboard.welcomeStep4Title"),
      detail: t("dashboard.welcomeStep4Detail"),
    },
  ];
  return (
    <Surface as="section" className="mx-auto max-w-2xl px-6 py-7">
      <div className="mb-4 flex items-center gap-2 text-[color:var(--lumen-teal)]">
        <Sparkles size={18} strokeWidth={1.75} />
        <h2 className="text-lg font-semibold tracking-tight text-[color:var(--color-fg)]">
          {t("dashboard.welcomeTitle")}
        </h2>
      </div>
      <p className="mb-6 text-sm text-[color:var(--color-muted)]">
        {t("dashboard.welcomeSubtitle")}
      </p>
      <ol className="space-y-4">
        {steps.map((step, i) => (
          <li key={i} className="flex gap-3">
            <span
              className={`mt-0.5 inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-full border ${
                step.done
                  ? "border-[color:var(--lumen-teal)] bg-[color:var(--lumen-teal)] text-[color:var(--color-bg)]"
                  : "border-[color:var(--color-border)] text-[color:var(--color-muted)]"
              }`}
              aria-hidden
            >
              {step.done ? (
                <CheckCircle2 size={14} strokeWidth={2} />
              ) : (
                <Circle size={14} strokeWidth={2} />
              )}
            </span>
            <div className="flex-1">
              <h3 className="text-sm font-medium text-[color:var(--color-fg)]">
                {step.title}
              </h3>
              <p className="mt-0.5 text-xs text-[color:var(--color-muted)]">
                {step.detail}
              </p>
              {step.cta && (
                <button
                  type="button"
                  onClick={step.cta.onClick}
                  className="mt-2 inline-flex items-center gap-1.5 rounded-md border border-[color:var(--lumen-teal)] bg-[color:var(--lumen-teal)] px-3 py-1.5 text-xs font-medium text-[color:var(--color-bg)] transition-colors hover:bg-[color:var(--lumen-teal)]/85 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--lumen-teal)]/40"
                >
                  {step.cta.label}
                  <ArrowRight size={13} strokeWidth={1.75} />
                </button>
              )}
            </div>
          </li>
        ))}
      </ol>
      <div className="mt-6 border-t border-[color:var(--color-border)] pt-4 text-xs">
        <a
          href="https://github.com/quanla93/lumen/blob/main/docs/src/content/docs/getting-started/quickstart.md"
          target="_blank"
          rel="noreferrer noopener"
          className="inline-flex items-center gap-1 text-[color:var(--color-muted)] hover:text-[color:var(--lumen-teal)]"
        >
          {t("dashboard.welcomeDocsLink")}
        </a>
      </div>
    </Surface>
  );
}
