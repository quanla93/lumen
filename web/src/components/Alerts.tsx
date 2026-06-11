import { useCallback, useEffect, useState } from "react";
import {
  BellRing, History, Send, ShieldAlert, Cable, Tag as TagIcon,
  Cpu, MemoryStick, HardDrive, Activity, ServerOff, Wrench,
  Pencil, Trash2, Megaphone, MessagesSquare, Webhook, Mail,
  CheckCircle2, XCircle, Clock, Loader2, MinusCircle, Zap,
  VolumeX,
} from "lucide-react";
import {
  alertsApi,
  ApiError,
  hostsApi,
  tagsApi,
  webPushApi,
  maintenanceApi,
  TELEGRAM_TOKEN_MASK,
  type AlertComparator,
  type AlertEvent,
  type AlertMetric,
  type AlertRule,
  type AlertRuleWrite,
  type AlertSeverity,
  type ChannelType,
  type DeliveryStatus,
  type MaintenanceWindow,
  type DeliveryView,
  type Host,
  type NotificationChannel,
  type NotificationChannelWrite,
  type Tag,
  type WebPushSubscription,
} from "@/lib/api";
import { relativeTime } from "@/lib/time";
import {
  ErrorText,
  Field,
  FieldInput,
  GhostButton,
  PrimaryButton,
} from "@/components/CenterCard";
import { EmptyState, IconButton, Popover, SegmentedControl, StatusPill, Surface, Switch } from "@/components/ui";
import { useConfirm } from "@/components/ConfirmDialog";
import { AlertTags } from "@/components/AlertTags";
import { SettingsPanel } from "@/components/Settings";
import { useI18n } from "@/i18n/useI18n";
import type { TranslationKey } from "@/i18n/types";

type AlertsTab = "active" | "history" | "deliveries" | "rules" | "channels" | "tags" | "maintenance";

const TABS: { id: AlertsTab; labelKey: TranslationKey; icon: typeof BellRing }[] = [
  { id: "active",      labelKey: "alerts.tabs.active",      icon: BellRing },
  { id: "history",     labelKey: "alerts.tabs.history",     icon: History },
  { id: "deliveries",  labelKey: "alerts.tabs.deliveries",  icon: Send },
  { id: "rules",       labelKey: "alerts.tabs.rules",       icon: ShieldAlert },
  { id: "channels",    labelKey: "alerts.tabs.channels",    icon: Cable },
  { id: "tags",        labelKey: "alerts.tabs.tags",        icon: TagIcon },
  { id: "maintenance", labelKey: "alerts.tabs.maintenance", icon: Wrench },
];

const METRICS: AlertMetric[] = ["cpu_pct", "ram_pct", "swap_pct", "disk_pct", "load1", "offline", "gpu_util", "gpu_temp", "gpu_mem_pct"];
const COMPARATORS: AlertComparator[] = ["gt", "lt"];
const SEVERITIES: AlertSeverity[] = ["info", "warning", "critical"];
const CHANNEL_TYPES: ChannelType[] = ["ntfy", "discord", "webhook", "telegram", "email", "web_push"];

const EVENT_POLL_MS = 15_000;

// Per-row "silence this host" Popover presets. Seconds map to backend
// silence durations; clicking auto-resolves the firing event on the next
// engine tick (silence suppresses future fires).
const SILENCE_PRESETS: { seconds: number; labelKey: TranslationKey }[] = [
  { seconds: 15 * 60,           labelKey: "alerts.silenceFor15m"     },
  { seconds: 60 * 60,           labelKey: "alerts.silenceFor1h"      },
  { seconds: 4 * 60 * 60,       labelKey: "alerts.silenceFor4h"      },
  // "Until I lift" uses the backend's 1-year cap as a practical
  // sentinel — long enough to feel indefinite for homelab maintenance,
  // short enough that an abandoned silence eventually self-clears.
  { seconds: 365 * 24 * 60 * 60, labelKey: "alerts.silenceUntilLift" },
];

export function Alerts() {
  const { t, locale } = useI18n();
  const [tab, setTab] = useState<AlertsTab>("active");
  const [activeCount, setActiveCount] = useState<number | null>(null);

  // Active count flows into the header summary chip; polled at the same
  // cadence the engine evaluates so the badge feels live.
  const refreshActive = useCallback(async () => {
    try {
      const evs = await alertsApi.events("firing", 200);
      setActiveCount(evs.length);
    } catch {
      // Surface nothing — the active tab below shows its own error if any.
    }
  }, []);

  useEffect(() => {
    refreshActive();
    const id = window.setInterval(refreshActive, EVENT_POLL_MS);
    return () => window.clearInterval(id);
  }, [refreshActive]);

  return (
    <div className="space-y-6">
      <header className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight text-[color:var(--color-fg)]">{t("alerts.title")}</h1>
          <p className="mt-1 text-sm text-[color:var(--color-muted)]">{t("alerts.subtitle")}</p>
        </div>
        <StatusPill tone={activeCount && activeCount > 0 ? "danger" : "ok"}>
          {activeCount && activeCount > 0
            ? t("alerts.summaryFiring", { count: activeCount })
            : t("alerts.summaryClean")}
        </StatusPill>
      </header>

      <nav className="flex flex-wrap items-center gap-1 rounded-lg border border-[color:var(--color-border)] bg-[color:var(--color-card)] p-1">
        {TABS.map((item) => {
          const Icon = item.icon;
          const active = item.id === tab;
          return (
            <button
              key={item.id}
              type="button"
              onClick={() => setTab(item.id)}
              aria-current={active ? "page" : undefined}
              className={`inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
                active
                  ? "bg-[color-mix(in_oklch,var(--lumen-teal)_15%,transparent)] text-[color:var(--color-fg)]"
                  : "text-[color:var(--color-muted)] hover:bg-[color:var(--color-border)]/40 hover:text-[color:var(--color-fg)]"
              }`}
            >
              <Icon size={14} strokeWidth={active ? 2.25 : 1.75} className={active ? "text-[color:var(--lumen-teal)]" : ""} />
              {t(item.labelKey)}
            </button>
          );
        })}
      </nav>

      <div>
        {tab === "active"     && <EventsList state="firing" />}
        {tab === "history"    && <EventsList state="resolved" />}
        {tab === "deliveries" && <DeliveriesPanel />}
        {tab === "rules"      && <RulesPanel />}
        {tab === "channels"   && <ChannelsPanel />}
        {tab === "tags"       && <AlertTags />}
        {tab === "maintenance" && <MaintenancePanel />}
      </div>
      {/* locale is read so the relativeTime in nested rows refreshes on language change */}
      <span className="sr-only" aria-hidden>{locale}</span>
    </div>
  );
}

// ---------------- events ----------------

// Page size matches the server cap step: every "Load more" click bumps
// the LIMIT by this many rows, up to PAGE_LIMIT_MAX. Auto-refresh keeps
// using the current page size so the newest rows stay live without
// resetting the user's scrollback.
const EVENT_PAGE_STEP = 200;
const EVENT_PAGE_MAX = 2000;

function EventsList({ state }: { state: "firing" | "resolved" | "all" }) {
  const { t, locale } = useI18n();
  const [events, setEvents] = useState<AlertEvent[] | null>(null);
  const [hostIndex, setHostIndex] = useState<Map<string, Host>>(new Map());
  const [error, setError] = useState<string | null>(null);
  const [now, setNow] = useState(Date.now());
  const [limit, setLimit] = useState(EVENT_PAGE_STEP);
  const [loadingMore, setLoadingMore] = useState(false);
  const [silenceBusy, setSilenceBusy] = useState<number | null>(null);

  const refresh = useCallback(async (nextLimit: number = limit) => {
    try {
      // Pull events + hosts together so we always have a fresh name→id map
      // for the per-row silence action (event payload only carries host name).
      const [evs, hs] = await Promise.all([
        alertsApi.events(state, nextLimit),
        hostsApi.list(),
      ]);
      setEvents(evs);
      setHostIndex(new Map(hs.map((h) => [h.name, h])));
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    }
  }, [state, limit]);

  useEffect(() => {
    refresh(limit);
    const evId = window.setInterval(() => refresh(limit), EVENT_POLL_MS);
    const nowId = window.setInterval(() => setNow(Date.now()), 5_000);
    return () => {
      window.clearInterval(evId);
      window.clearInterval(nowId);
    };
  }, [refresh, limit]);

  async function silenceHost(host: Host, seconds: number) {
    setSilenceBusy(host.id);
    try {
      await hostsApi.silence(host.id, seconds);
      await refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setSilenceBusy(null);
    }
  }

  // Reset the page when the user switches state (Active ↔ History).
  useEffect(() => {
    setLimit(EVENT_PAGE_STEP);
  }, [state]);

  async function loadMore() {
    const next = Math.min(limit + EVENT_PAGE_STEP, EVENT_PAGE_MAX);
    if (next === limit) return;
    setLoadingMore(true);
    setLimit(next);
    // The useEffect on `limit` re-fires refresh; release the busy state
    // on the next tick so the button feedback feels instant.
    setTimeout(() => setLoadingMore(false), 0);
  }

  if (error) return <ErrorText message={t("alerts.listError", { error: error })} />;
  if (events === null) return <p className="text-sm text-[color:var(--color-muted)]">{t("alerts.listLoading")}</p>;

  if (events.length === 0) {
    return (
      <EmptyState
        title={state === "firing" ? t("alerts.activeEmpty") : t("alerts.historyEmpty")}
        detail={state === "firing" ? t("alerts.activeEmptyHint") : t("alerts.historyEmptyHint")}
      />
    );
  }

  // "Maybe more" = the server returned a full page. Hide the button once
  // we know the tail is in view or we already hit the ceiling.
  const maybeMore = events.length >= limit && limit < EVENT_PAGE_MAX;
  const atCeiling = limit >= EVENT_PAGE_MAX;

  return (
    <Surface padded={false}>
      <ul className="divide-y divide-[color:var(--color-border)]">
        {events.map((ev) => {
          const tone = toneForSeverity(ev.severity, ev.state);
          const StateIcon = ev.state === "firing" ? BellRing : CheckCircle2;
          const host = hostIndex.get(ev.host);
          const isFiring = ev.state === "firing";
          const isSilenced = !!host?.silenced_until;
          return (
            <li key={ev.id} className="group relative flex items-start gap-3 pl-4 pr-3 py-3.5 sm:items-center">
              <span aria-hidden className={`absolute left-0 top-2 bottom-2 w-1 rounded-r ${severityStripeClass(tone)}`} />
              <div
                className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-md ${
                  tone === "danger" ? "bg-[color:var(--color-danger)]/12 text-[color:var(--color-danger)]"
                  : tone === "warn" ? "bg-[color:var(--color-warn)]/12 text-[color:var(--color-warn)]"
                  : tone === "ok"   ? "bg-[color:var(--color-accent)]/12 text-[color:var(--color-accent)]"
                                    : "bg-[color:var(--color-border)]/40 text-[color:var(--color-muted)]"
                }`}
              >
                <StateIcon size={16} strokeWidth={1.75} />
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-medium">{ev.rule_name}</span>
                  <StatusPill tone={tone}>
                    {t(`alerts.severity.${ev.severity}` as TranslationKey)}
                  </StatusPill>
                  <span className="text-xs text-[color:var(--color-muted)]">·</span>
                  <span className="lumen-num text-xs text-[color:var(--color-muted)]">{ev.host}</span>
                  {isSilenced && (
                    <StatusPill tone="muted">
                      <VolumeX size={10} strokeWidth={2} className="inline -mt-0.5 mr-0.5" />
                      {t("alerts.silencedNow")}
                    </StatusPill>
                  )}
                </div>
                <p className="mt-0.5 text-sm text-[color:var(--color-muted)]">{ev.message}</p>
              </div>
              <div className="flex shrink-0 items-start gap-2 sm:items-center">
                <div className="text-right text-xs text-[color:var(--color-muted)]">
                  <div>{t("alerts.sinceLabel", { time: relativeTime(ev.started_at, now, locale) })}</div>
                  {ev.resolved_at && (
                    <div>{t("alerts.resolvedAt", { time: relativeTime(ev.resolved_at, now, locale) })}</div>
                  )}
                </div>
                {isFiring && host && (
                  <Popover
                    trigger={
                      <IconButton
                        label={t("alerts.silenceHost")}
                        disabled={silenceBusy === host.id}
                        className="h-8 w-8 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100"
                      >
                        <VolumeX size={14} strokeWidth={1.75} />
                      </IconButton>
                    }
                  >
                    <div className="space-y-2">
                      <div className="text-sm font-semibold text-[color:var(--color-fg)]">
                        {t("alerts.silenceForLabel", { host: host.name })}
                      </div>
                      <p className="text-xs text-[color:var(--color-muted)]">
                        {t("alerts.silenceHostHint")}
                      </p>
                      <div className="flex flex-wrap gap-1.5 pt-1">
                        {SILENCE_PRESETS.map((p) => (
                          <button
                            key={p.seconds}
                            type="button"
                            onClick={() => void silenceHost(host, p.seconds)}
                            disabled={silenceBusy === host.id}
                            className="rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-2.5 py-1 text-xs font-medium hover:border-[color:var(--lumen-teal)]/50 hover:bg-[color-mix(in_oklch,var(--lumen-teal)_8%,var(--color-card))] disabled:opacity-50"
                          >
                            {t(p.labelKey)}
                          </button>
                        ))}
                      </div>
                    </div>
                  </Popover>
                )}
              </div>
            </li>
          );
        })}
      </ul>
      <div className="flex items-center justify-between gap-3 border-t border-[color:var(--color-border)] px-5 py-3 text-xs text-[color:var(--color-muted)]">
        <span>{t("alerts.loadedCount", { count: events.length })}</span>
        {maybeMore ? (
          <button
            type="button"
            onClick={loadMore}
            disabled={loadingMore}
            className="rounded-md border border-[color:var(--color-border)] px-3 py-1 text-xs hover:bg-[color:var(--color-bg-muted)] disabled:opacity-60"
          >
            {loadingMore ? t("common.loading") : t("alerts.loadMore", { step: EVENT_PAGE_STEP })}
          </button>
        ) : atCeiling ? (
          <span>{t("alerts.loadMoreCeiling", { max: EVENT_PAGE_MAX })}</span>
        ) : null}
      </div>
    </Surface>
  );
}

function toneForSeverity(sev: AlertSeverity, state: "firing" | "resolved"): "ok" | "warn" | "danger" | "muted" {
  if (state === "resolved") return "ok";
  if (sev === "critical") return "danger";
  if (sev === "warning") return "warn";
  return "muted";
}

// ---------------- deliveries ----------------

const DELIVERY_STATUSES: DeliveryStatus[] = ["pending", "inflight", "sent", "failed", "dropped"];
const DELIVERY_POLL_MS = 5_000;

function statusTone(s: DeliveryStatus): "ok" | "warn" | "danger" | "muted" {
  switch (s) {
    case "sent":     return "ok";
    case "pending":  return "warn";
    case "inflight": return "warn";
    case "failed":   return "danger";
    case "dropped":  return "muted";
  }
}

const DELIVERY_PAGE_STEP = 200;
const DELIVERY_PAGE_MAX = 2000;

function DeliveriesPanel() {
  const { t, locale } = useI18n();
  const [rows, setRows] = useState<DeliveryView[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<DeliveryStatus | "">("");
  const [severityFilter, setSeverityFilter] = useState<AlertSeverity | "">("");
  const [now, setNow] = useState(Date.now());
  const [retryBusy, setRetryBusy] = useState<number | null>(null);
  const [limit, setLimit] = useState(DELIVERY_PAGE_STEP);
  const [loadingMore, setLoadingMore] = useState(false);

  const refresh = useCallback(async (nextLimit: number = limit) => {
    try {
      const filter: { status?: DeliveryStatus; severity?: AlertSeverity; limit: number } = { limit: nextLimit };
      if (statusFilter) filter.status = statusFilter;
      if (severityFilter) filter.severity = severityFilter;
      const data = await alertsApi.deliveries(filter);
      setRows(data);
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    }
  }, [statusFilter, severityFilter, limit]);

  useEffect(() => {
    void refresh(limit);
    const id = window.setInterval(() => refresh(limit), DELIVERY_POLL_MS);
    const nowId = window.setInterval(() => setNow(Date.now()), 5_000);
    return () => {
      window.clearInterval(id);
      window.clearInterval(nowId);
    };
  }, [refresh, limit]);

  // Filter change collapses the page back to the first step — keeps the
  // scrollback intuitive (otherwise switching from "any" to "failed" with
  // limit=1000 would show 1000 failed rows, far more than the user asked).
  useEffect(() => {
    setLimit(DELIVERY_PAGE_STEP);
  }, [statusFilter, severityFilter]);

  async function loadMore() {
    const next = Math.min(limit + DELIVERY_PAGE_STEP, DELIVERY_PAGE_MAX);
    if (next === limit) return;
    setLoadingMore(true);
    setLimit(next);
    setTimeout(() => setLoadingMore(false), 0);
  }

  async function retry(id: number) {
    setRetryBusy(id);
    try {
      await alertsApi.retryDelivery(id);
      await refresh();
    } catch (err) {
      window.alert(err instanceof ApiError ? err.message : String(err));
    } finally {
      setRetryBusy(null);
    }
  }

  // Counts by status for the summary chip strip — gives the operator a
  // quick "how is the queue?" read without reading each row.
  const counts = (rows ?? []).reduce(
    (acc, r) => {
      acc[r.status] = (acc[r.status] ?? 0) + 1;
      return acc;
    },
    {} as Record<DeliveryStatus, number>,
  );

  return (
    <div className="space-y-4">
      <Surface>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold">{t("alerts.deliveriesTitle")}</h2>
            <p className="mt-1 text-sm text-[color:var(--color-muted)]">{t("alerts.deliveriesDescription")}</p>
          </div>
          <div className="flex flex-wrap gap-2 text-xs">
            {DELIVERY_STATUSES.map((s) => (
              <StatusPill key={s} tone={statusTone(s)}>
                {t(`alerts.deliveryStatus.${s}` as TranslationKey)}: {counts[s] ?? 0}
              </StatusPill>
            ))}
          </div>
        </div>
        <div className="mt-4 flex flex-wrap items-end gap-3">
          <div>
            <label className="block text-xs uppercase tracking-wide text-[color:var(--color-muted)] mb-1">
              {t("alerts.deliveryFilterStatus")}
            </label>
            <SelectInput
              value={statusFilter}
              onChange={(v) => setStatusFilter(v as DeliveryStatus | "")}
              options={[
                { value: "", label: t("alerts.deliveryFilterAny") },
                ...DELIVERY_STATUSES.map((s) => ({
                  value: s,
                  label: t(`alerts.deliveryStatus.${s}` as TranslationKey),
                })),
              ]}
            />
          </div>
          <div>
            <label className="block text-xs uppercase tracking-wide text-[color:var(--color-muted)] mb-1">
              {t("alerts.deliveryFilterSeverity")}
            </label>
            <SelectInput
              value={severityFilter}
              onChange={(v) => setSeverityFilter(v as AlertSeverity | "")}
              options={[
                { value: "", label: t("alerts.deliveryFilterAny") },
                ...SEVERITIES.map((s) => ({
                  value: s,
                  label: t(`alerts.severity.${s}` as TranslationKey),
                })),
              ]}
            />
          </div>
        </div>
        {error && <div className="mt-3"><ErrorText message={t("alerts.listError", { error })} /></div>}
      </Surface>

      {rows === null ? (
        <p className="text-sm text-[color:var(--color-muted)]">{t("alerts.listLoading")}</p>
      ) : rows.length === 0 ? (
        <EmptyState title={t("alerts.deliveriesEmpty")} detail={t("alerts.deliveriesEmptyHint")} />
      ) : (
        <Surface padded={false}>
          <ul className="divide-y divide-[color:var(--color-border)]">
            {rows.map((d) => {
              const isRetryable = d.status === "failed" || d.status === "dropped";
              const StatusIcon = deliveryStatusIcon(d.status);
              const ChIcon = channelIcon(d.channel_type as ChannelType);
              const sTone = statusTone(d.status);
              const isInflight = d.status === "inflight";
              const message = typeof (d.payload as { message?: string })?.message === "string"
                ? (d.payload as { message: string }).message
                : null;
              // Single timestamp line — "sent X ago" wins over "queued" if
              // both exist; pending/inflight show queued + next retry.
              const timeAgo = d.sent_at
                ? t("alerts.deliverySentAt", { time: relativeTime(d.sent_at, now, locale) })
                : t("alerts.deliveryQueuedAt", { time: relativeTime(d.created_at, now, locale) });
              return (
                <li key={d.id} className="group flex items-center gap-3 px-4 py-2.5">
                  <div
                    className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-md ${
                      sTone === "danger" ? "bg-[color:var(--color-danger)]/12 text-[color:var(--color-danger)]"
                      : sTone === "warn" ? "bg-[color:var(--color-warn)]/12 text-[color:var(--color-warn)]"
                      : sTone === "ok"   ? "bg-[color:var(--color-accent)]/12 text-[color:var(--color-accent)]"
                                         : "bg-[color:var(--color-border)]/40 text-[color:var(--color-muted)]"
                    }`}
                  >
                    <StatusIcon size={14} strokeWidth={1.75} className={isInflight ? "animate-spin" : ""} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <ChIcon size={13} strokeWidth={1.75} className="shrink-0 text-[color:var(--color-muted)]" />
                      <span className="truncate text-sm font-medium">{d.channel_name}</span>
                      <StatusPill tone={toneForSeverity(d.severity, "firing")}>
                        {t(`alerts.severity.${d.severity}` as TranslationKey)}
                      </StatusPill>
                      {message && (
                        <span className="truncate text-xs text-[color:var(--color-muted)]">
                          · {message}
                        </span>
                      )}
                    </div>
                    <p className="lumen-num text-xs text-[color:var(--color-muted)] truncate">
                      <span className="uppercase tracking-wide">{t(`alerts.deliveryStatus.${d.status}` as TranslationKey)}</span>
                      {" · "}{t("alerts.deliveryMeta", { attempts: d.attempts, httpStatus: d.http_status ?? "—" })}
                      {" · "}{timeAgo}
                      {d.next_retry_at && d.status === "pending" && (
                        <> {" · "}{t("alerts.deliveryNextRetry", { time: relativeTime(d.next_retry_at, now, locale) })}</>
                      )}
                    </p>
                    {d.error && (
                      <p className="text-xs text-[color:var(--color-danger)] break-words">
                        {d.error}
                      </p>
                    )}
                  </div>
                  {isRetryable && (
                    <GhostButton onClick={() => void retry(d.id)} disabled={retryBusy === d.id} className="shrink-0">
                      {retryBusy === d.id ? t("common.saving") : t("alerts.deliveryRetry")}
                    </GhostButton>
                  )}
                </li>
              );
            })}
          </ul>
          <div className="flex items-center justify-between gap-3 border-t border-[color:var(--color-border)] px-5 py-3 text-xs text-[color:var(--color-muted)]">
            <span>{t("alerts.loadedCount", { count: rows.length })}</span>
            {rows.length >= limit && limit < DELIVERY_PAGE_MAX ? (
              <button
                type="button"
                onClick={loadMore}
                disabled={loadingMore}
                className="rounded-md border border-[color:var(--color-border)] px-3 py-1 text-xs hover:bg-[color:var(--color-bg-muted)] disabled:opacity-60"
              >
                {loadingMore ? t("common.loading") : t("alerts.loadMore", { step: DELIVERY_PAGE_STEP })}
              </button>
            ) : limit >= DELIVERY_PAGE_MAX ? (
              <span>{t("alerts.loadMoreCeiling", { max: DELIVERY_PAGE_MAX })}</span>
            ) : null}
          </div>
        </Surface>
      )}
    </div>
  );
}

// ---------------- rules ----------------

function blankRule(): AlertRuleWrite {
  return {
    name: "",
    metric: "cpu_pct",
    comparator: "gt",
    threshold: 80,
    for_seconds: 60,
    cooldown_seconds: 0,
    host: "",
    host_selector: "",
    severity: "warning",
    enabled: true,
    channel_ids: [],
  };
}

function ruleToWrite(r: AlertRule): AlertRuleWrite {
  return {
    name: r.name,
    metric: r.metric,
    comparator: r.comparator,
    threshold: r.threshold,
    for_seconds: r.for_seconds,
    cooldown_seconds: r.cooldown_seconds ?? 0,
    host: r.host,
    host_selector: r.host_selector ?? "",
    severity: r.severity,
    enabled: r.enabled,
    channel_ids: r.channel_ids ?? [],
  };
}

// Preset rules covering the 80% case — CPU/RAM/Disk thresholds, host
// offline, load. Clicking one opens the form prefilled so the operator
// only has to confirm targeting and channels, instead of typing 11 fields.
type RuleTemplate = {
  key: string;
  labelKey: TranslationKey;
  icon: typeof Cpu;
  preset: Partial<AlertRuleWrite>;
};

const RULE_TEMPLATES: RuleTemplate[] = [
  { key: "cpu_high",  labelKey: "alerts.tplCpuHigh",  icon: Cpu,
    preset: { name: "CPU high",     metric: "cpu_pct",  comparator: "gt", threshold: 80, for_seconds: 120, severity: "warning" } },
  { key: "ram_high",  labelKey: "alerts.tplRamHigh",  icon: MemoryStick,
    preset: { name: "RAM high",     metric: "ram_pct",  comparator: "gt", threshold: 90, for_seconds: 120, severity: "warning" } },
  { key: "disk_full", labelKey: "alerts.tplDiskFull", icon: HardDrive,
    preset: { name: "Disk full",    metric: "disk_pct", comparator: "gt", threshold: 85, for_seconds: 300, severity: "critical" } },
  { key: "offline",   labelKey: "alerts.tplOffline",  icon: ServerOff,
    preset: { name: "Host offline", metric: "offline",  comparator: "gt", threshold: 0,  for_seconds: 60,  severity: "critical" } },
  { key: "load_high", labelKey: "alerts.tplLoadHigh", icon: Activity,
    preset: { name: "Load high",    metric: "load1",    comparator: "gt", threshold: 4,  for_seconds: 120, severity: "warning" } },
];

function metricIcon(metric: AlertMetric): typeof Cpu {
  switch (metric) {
    case "cpu_pct":    return Cpu;
    case "ram_pct":    return MemoryStick;
    case "swap_pct":   return MemoryStick;
    case "disk_pct":   return HardDrive;
    case "load1":      return Activity;
    case "offline":    return ServerOff;
    case "gpu_util":   return Cpu;
    case "gpu_temp":   return Activity;
    case "gpu_mem_pct": return MemoryStick;
  }
}

function channelIcon(type: ChannelType): typeof Cpu {
  switch (type) {
    case "ntfy":     return Megaphone;
    case "discord":  return MessagesSquare;
    case "webhook":  return Webhook;
    case "telegram": return Send;
    case "email":    return Mail;
    case "web_push": return BellRing;
  }
}

function deliveryStatusIcon(s: DeliveryStatus): typeof Cpu {
  switch (s) {
    case "sent":     return CheckCircle2;
    case "pending":  return Clock;
    case "inflight": return Loader2;
    case "failed":   return XCircle;
    case "dropped":  return MinusCircle;
  }
}

function severityStripeClass(tone: "ok" | "warn" | "danger" | "muted"): string {
  switch (tone) {
    case "ok":     return "bg-[color:var(--color-accent)]";
    case "warn":   return "bg-[color:var(--color-warn)]";
    case "danger": return "bg-[color:var(--color-danger)]";
    case "muted":  return "bg-[color:var(--color-muted)]";
  }
}

function RulesPanel() {
  const { t } = useI18n();
  const confirm = useConfirm();
  const [rules, setRules] = useState<AlertRule[] | null>(null);
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [hosts, setHosts] = useState<Host[]>([]);
  const [tagInventory, setTagInventory] = useState<Tag[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [editingId, setEditingId] = useState<number | "new" | null>(null);
  const [draft, setDraft] = useState<AlertRuleWrite>(blankRule());
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const [rs, chs, hs, inv] = await Promise.all([
        alertsApi.rules.list(),
        alertsApi.channels.list(),
        hostsApi.list(),
        tagsApi.list(),
      ]);
      setRules(rs);
      setChannels(chs);
      setHosts(hs);
      setTagInventory(inv);
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    }
  }, []);

  useEffect(() => { void refresh(); }, [refresh]);

  // refreshLatest pulls hosts + tag inventory again so the picker
  // reflects edits the operator made in Alerts → Tags between rule
  // form sessions. Called when entering New / Edit so options never lag
  // behind a tag rename or addition.
  const refreshLatest = useCallback(async () => {
    try {
      const [hs, inv] = await Promise.all([hostsApi.list(), tagsApi.list()]);
      setHosts(hs);
      setTagInventory(inv);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    }
  }, []);

  function toggleDraftChannel(id: number) {
    const cur = draft.channel_ids ?? [];
    if (cur.includes(id)) {
      setDraft({ ...draft, channel_ids: cur.filter((x) => x !== id) });
    } else {
      setDraft({ ...draft, channel_ids: [...cur, id] });
    }
  }

  function startNew() {
    setEditingId("new");
    setDraft(blankRule());
    setFormError(null);
    void refreshLatest();
  }

  function startFromTemplate(tmpl: RuleTemplate) {
    setEditingId("new");
    setDraft({ ...blankRule(), ...tmpl.preset });
    setFormError(null);
    void refreshLatest();
  }

  function startEdit(r: AlertRule) {
    setEditingId(r.id);
    setDraft(ruleToWrite(r));
    setFormError(null);
    void refreshLatest();
  }

  function cancel() {
    setEditingId(null);
    setFormError(null);
  }

  // Optimistic toggle — flip in UI immediately, PUT in background, revert
  // if the server rejects. Operator sees instant feedback; no form roundtrip.
  async function toggleEnabled(r: AlertRule) {
    if (editingId !== null) return; // don't race a form save
    const prev = rules;
    setRules((cur) => (cur ?? []).map((x) => x.id === r.id ? { ...x, enabled: !r.enabled } : x));
    try {
      await alertsApi.rules.update(r.id, { ...ruleToWrite(r), enabled: !r.enabled });
    } catch (err) {
      setRules(prev);
      setError(err instanceof ApiError ? err.message : String(err));
    }
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setFormError(null);
    try {
      if (editingId === "new") {
        await alertsApi.rules.create(draft);
      } else if (typeof editingId === "number") {
        await alertsApi.rules.update(editingId, draft);
      }
      setEditingId(null);
      await refresh();
    } catch (err) {
      setFormError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  }

  async function remove(r: AlertRule) {
    const ok = await confirm({
      title: t("alerts.deleteRuleTitle"),
      message: t("alerts.deleteRuleConfirm", { name: r.name }),
      confirmLabel: t("common.delete"),
      destructive: true,
    });
    if (!ok) return;
    try {
      await alertsApi.rules.remove(r.id);
      await refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    }
  }

  return (
    <div className="space-y-4">
      <Surface>
        <div className="flex items-start justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold">{t("alerts.rulesTitle")}</h2>
            <p className="mt-1 text-sm text-[color:var(--color-muted)]">{t("alerts.rulesDescription")}</p>
          </div>
          <GhostButton onClick={startNew} disabled={editingId !== null}>
            {t("alerts.newRule")}
          </GhostButton>
        </div>

        {editingId === null && (
          <div className="mt-4 rounded-lg border border-[color:var(--color-border)] bg-[color:var(--color-bg)] p-3">
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <span className="text-xs font-semibold uppercase tracking-wide text-[color:var(--color-muted)]">
                {t("alerts.quickTemplates")}
              </span>
              <span className="text-xs text-[color:var(--color-muted)]">
                {t("alerts.quickTemplatesHint")}
              </span>
            </div>
            <div className="mt-2 flex flex-wrap gap-1.5">
              {RULE_TEMPLATES.map((tmpl) => {
                const Icon = tmpl.icon;
                return (
                  <button
                    key={tmpl.key}
                    type="button"
                    onClick={() => startFromTemplate(tmpl)}
                    className="group inline-flex items-center gap-1.5 rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-card)] px-2.5 py-1.5 text-xs font-medium text-[color:var(--color-fg)] transition-colors hover:border-[color:var(--lumen-teal)]/50 hover:bg-[color-mix(in_oklch,var(--lumen-teal)_8%,var(--color-card))]"
                  >
                    <Icon size={14} strokeWidth={1.75} className="text-[color:var(--color-muted)] group-hover:text-[color:var(--lumen-teal)]" />
                    {t(tmpl.labelKey)}
                  </button>
                );
              })}
            </div>
          </div>
        )}

        {error && <div className="mt-3"><ErrorText message={t("alerts.listError", { error })} /></div>}

        {editingId !== null && (
          <form onSubmit={save} className="mt-4 space-y-5">
            {/* Header: name spans full width so the rule's identity reads first */}
            <Field label={t("alerts.fieldName")}>
              <FieldInput value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} required />
            </Field>

            {/* Section 1: Condition — what to alert on */}
            <FormSection title={t("alerts.sectionCondition")} icon={ShieldAlert}>
              <div className="grid gap-3 sm:grid-cols-2">
                <Field label={t("alerts.fieldMetric")}>
                  <SelectInput
                    value={draft.metric}
                    onChange={(v) => setDraft({ ...draft, metric: v as AlertMetric })}
                    options={METRICS.map((m) => ({ value: m, label: t(`alerts.metric.${m}` as TranslationKey) }))}
                  />
                </Field>
                {draft.metric !== "offline" && (
                  <Field label={t("alerts.fieldComparator")}>
                    <SegmentedControl
                      value={draft.comparator}
                      onChange={(v) => setDraft({ ...draft, comparator: v as AlertComparator })}
                      options={COMPARATORS.map((c) => ({ value: c, label: t(`alerts.comparator.${c}` as TranslationKey) }))}
                      ariaLabel={t("alerts.fieldComparator")}
                    />
                  </Field>
                )}
                {draft.metric !== "offline" && (
                  <Field label={t("alerts.fieldThreshold")}>
                    <FieldInput
                      type="number"
                      step="any"
                      value={draft.threshold}
                      onChange={(e) => setDraft({ ...draft, threshold: Number(e.target.value) })}
                    />
                  </Field>
                )}
                <Field label={t("alerts.fieldForSeconds")}>
                  <FieldInput
                    type="number"
                    min={0}
                    value={draft.for_seconds}
                    onChange={(e) => setDraft({ ...draft, for_seconds: Math.max(0, Number(e.target.value)) })}
                  />
                  <p className="mt-1 text-xs text-[color:var(--color-muted)]">{t("alerts.forSecondsHint")}</p>
                </Field>
                <Field label={t("alerts.fieldCooldownSeconds")}>
                  <FieldInput
                    type="number"
                    min={0}
                    value={draft.cooldown_seconds}
                    onChange={(e) => setDraft({ ...draft, cooldown_seconds: Math.max(0, Number(e.target.value)) })}
                  />
                  <p className="mt-1 text-xs text-[color:var(--color-muted)]">{t("alerts.cooldownSecondsHint")}</p>
                </Field>
              </div>
            </FormSection>

            {/* Section 2: Targeting — which hosts */}
            <FormSection title={t("alerts.sectionTargeting")} icon={TagIcon}>
              <HostTargetingFields draft={draft} setDraft={setDraft} hosts={hosts} inventory={tagInventory} />
            </FormSection>

            {/* Section 3: Notification — severity + routing + enabled */}
            <FormSection title={t("alerts.sectionNotification")} icon={Send}>
              <div className="space-y-3">
                <Field label={t("alerts.fieldSeverity")}>
                  <SegmentedControl
                    value={draft.severity}
                    onChange={(v) => setDraft({ ...draft, severity: v as AlertSeverity })}
                    options={SEVERITIES.map((s) => ({ value: s, label: t(`alerts.severity.${s}` as TranslationKey) }))}
                    ariaLabel={t("alerts.fieldSeverity")}
                  />
                </Field>
                <div>
                  <span className="block text-xs uppercase tracking-wide text-[color:var(--color-muted)] mb-1.5">
                    {t("alerts.fieldChannels")}
                  </span>
                  {channels.length === 0 ? (
                    <p className="text-xs text-[color:var(--color-muted)]">
                      {t("alerts.fieldChannelsEmpty")}
                    </p>
                  ) : (
                    <div className="grid gap-1.5 sm:grid-cols-2">
                      {channels.map((ch) => {
                        const checked = (draft.channel_ids ?? []).includes(ch.id);
                        return (
                          <label
                            key={ch.id}
                            className={`flex cursor-pointer items-center gap-2 rounded-md border px-2.5 py-1.5 text-sm transition-colors ${
                              checked
                                ? "border-[color:var(--lumen-teal)]/60 bg-[color-mix(in_oklch,var(--lumen-teal)_8%,var(--color-card))]"
                                : "border-[color:var(--color-border)] hover:bg-[color:var(--color-bg)]/40"
                            }`}
                          >
                            <input
                              type="checkbox"
                              checked={checked}
                              onChange={() => toggleDraftChannel(ch.id)}
                            />
                            <span className="font-medium">{ch.name}</span>
                            <span className="text-xs text-[color:var(--color-muted)]">
                              {t(`alerts.channelType.${ch.type}` as TranslationKey)}
                              {ch.min_severity !== "info" ? ` · ≥ ${t(`alerts.severity.${ch.min_severity}` as TranslationKey)}` : ""}
                              {!ch.enabled ? ` · ${t("alerts.disabledLabel")}` : ""}
                            </span>
                          </label>
                        );
                      })}
                    </div>
                  )}
                  <p className="mt-1.5 text-xs text-[color:var(--color-muted)]">
                    {t("alerts.fieldChannelsHint")}
                  </p>
                </div>
                <label className="flex items-center gap-2.5 rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-3 py-2 text-sm">
                  <Switch
                    checked={draft.enabled ?? true}
                    onCheckedChange={(v) => setDraft({ ...draft, enabled: v })}
                    ariaLabel={t("alerts.fieldEnabled")}
                  />
                  <span className="font-medium">{t("alerts.fieldEnabled")}</span>
                </label>
              </div>
            </FormSection>

            {/* Footer */}
            <div className="flex items-center gap-2 border-t border-[color:var(--color-border)] pt-4">
              <PrimaryButton disabled={submitting}>{submitting ? t("common.saving") : t("alerts.save")}</PrimaryButton>
              <GhostButton type="button" onClick={cancel}>{t("alerts.cancel")}</GhostButton>
              {formError && <ErrorText message={t("alerts.formError", { error: formError })} />}
            </div>
          </form>
        )}
      </Surface>

      {rules === null ? (
        <p className="text-sm text-[color:var(--color-muted)]">{t("alerts.listLoading")}</p>
      ) : rules.length === 0 ? (
        <EmptyState title={t("alerts.rulesEmpty")} detail={t("alerts.rulesEmptyHint")} />
      ) : (
        <Surface padded={false}>
          <ul className="divide-y divide-[color:var(--color-border)]">
            {rules.map((r) => {
              const isEditing = editingId === r.id;
              const otherEditing = editingId !== null && !isEditing;
              const Icon = metricIcon(r.metric);
              return (
                <li
                  key={r.id}
                  className={`group flex items-center gap-3 px-4 py-3 transition-colors ${
                    isEditing ? "bg-[color:var(--color-bg)]" : otherEditing ? "opacity-50" : "hover:bg-[color:var(--color-bg)]/60"
                  } ${!r.enabled ? "opacity-70" : ""}`}
                >
                  <Switch
                    checked={r.enabled}
                    onCheckedChange={() => void toggleEnabled(r)}
                    ariaLabel={r.enabled ? t("alerts.disableRule") : t("alerts.enableRule")}
                    disabled={editingId !== null}
                  />
                  <div
                    className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-md ${
                      r.enabled
                        ? "bg-[color-mix(in_oklch,var(--lumen-teal)_12%,var(--color-card))] text-[color:var(--lumen-teal)]"
                        : "bg-[color:var(--color-border)]/40 text-[color:var(--color-muted)]"
                    }`}
                  >
                    <Icon size={16} strokeWidth={1.75} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className={`text-sm font-medium ${r.enabled ? "text-[color:var(--color-fg)]" : "text-[color:var(--color-muted)]"}`}>
                        {r.name}
                      </span>
                      <StatusPill tone={r.enabled ? toneForSeverity(r.severity, "firing") : "muted"}>
                        {t(`alerts.severity.${r.severity}` as TranslationKey)}
                      </StatusPill>
                      {isEditing && (
                        <span className="rounded-full bg-[color:var(--color-accent)]/15 px-2 py-0.5 text-xs text-[color:var(--color-accent)]">
                          {t("alerts.editingNow")}
                        </span>
                      )}
                    </div>
                    <p className="mt-0.5 truncate text-xs text-[color:var(--color-muted)]">
                      {ruleDescription(r, t)}
                    </p>
                    <p className="truncate text-xs text-[color:var(--color-muted)]">
                      {ruleRoutingDescription(r, channels, t)}
                    </p>
                  </div>
                  <div className="flex items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
                    <IconButton
                      onClick={() => startEdit(r)}
                      disabled={editingId !== null}
                      label={t("alerts.editRule")}
                      className="h-8 w-8"
                    >
                      <Pencil size={14} strokeWidth={1.75} />
                    </IconButton>
                    <IconButton
                      onClick={() => void remove(r)}
                      disabled={editingId !== null}
                      label={t("alerts.delete")}
                      className="h-8 w-8 hover:text-[color:var(--color-danger)]"
                    >
                      <Trash2 size={14} strokeWidth={1.75} />
                    </IconButton>
                  </div>
                </li>
              );
            })}
          </ul>
        </Surface>
      )}
    </div>
  );
}

function ruleRoutingDescription(
  r: AlertRule,
  channels: NotificationChannel[],
  t: (k: TranslationKey, p?: Record<string, string | number>) => string,
): string {
  if (!r.channel_ids || r.channel_ids.length === 0) {
    return t("alerts.routingAll");
  }
  const names = r.channel_ids
    .map((id) => channels.find((c) => c.id === id)?.name)
    .filter((n): n is string => Boolean(n));
  if (names.length === 0) return t("alerts.routingNone");
  return t("alerts.routingTo", { channels: names.join(", ") });
}

function ruleDescription(r: AlertRule, t: (k: TranslationKey, p?: Record<string, string | number>) => string): string {
  const metric = t(`alerts.metric.${r.metric}` as TranslationKey);
  const target = r.host_selector ? `[${r.host_selector}]` : (r.host || "*");
  if (r.metric === "offline") {
    return `${metric} · ${target} · for ≥ ${r.for_seconds}s`;
  }
  const cmp = t(`alerts.comparator.${r.comparator}` as TranslationKey);
  return `${metric} ${cmp} ${r.threshold} · ${target} · for ${r.for_seconds}s`;
}

// ---------------- channels ----------------

function blankChannel(): NotificationChannelWrite {
  return {
    name: "",
    type: "ntfy",
    config: { url: "" },
    enabled: true,
    min_severity: "info",
  };
}

function channelToWrite(c: NotificationChannel): NotificationChannelWrite {
  return {
    name: c.name,
    type: c.type,
    config: {
      url: c.config.url ?? "",
      topic: c.config.topic,
      priority: c.config.priority,
      bot_token: c.config.bot_token,
      chat_id: c.config.chat_id,
      parse_mode: c.config.parse_mode,
      smtp_host: c.config.smtp_host,
      smtp_port: c.config.smtp_port,
      username: c.config.username,
      password: c.config.password,
      from_addr: c.config.from_addr,
      to_addr: c.config.to_addr,
    },
    enabled: c.enabled,
    min_severity: c.min_severity,
  };
}

function ChannelsPanel() {
  const { t } = useI18n();
  const confirm = useConfirm();
  const [channels, setChannels] = useState<NotificationChannel[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editingId, setEditingId] = useState<number | "new" | null>(null);
  const [draft, setDraft] = useState<NotificationChannelWrite>(blankChannel());
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<{ id: number; ok: boolean; msg: string } | null>(null);

  const refresh = useCallback(async () => {
    try {
      setChannels(await alertsApi.channels.list());
      setError(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    }
  }, []);

  useEffect(() => { void refresh(); }, [refresh]);

  function startNew() {
    setEditingId("new");
    setDraft(blankChannel());
    setFormError(null);
  }
  function startEdit(c: NotificationChannel) {
    setEditingId(c.id);
    setDraft(channelToWrite(c));
    setFormError(null);
  }
  function cancel() {
    setEditingId(null);
    setFormError(null);
  }

  // Optimistic enable/disable — flip in UI, PUT in background, revert on error.
  // Mirrors RulesPanel.toggleEnabled.
  async function toggleEnabled(c: NotificationChannel) {
    if (editingId !== null) return;
    const prev = channels;
    setChannels((cur) => (cur ?? []).map((x) => x.id === c.id ? { ...x, enabled: !c.enabled } : x));
    try {
      await alertsApi.channels.update(c.id, { ...channelToWrite(c), enabled: !c.enabled });
    } catch (err) {
      setChannels(prev);
      setError(err instanceof ApiError ? err.message : String(err));
    }
  }

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setFormError(null);
    try {
      if (editingId === "new") {
        await alertsApi.channels.create(draft);
      } else if (typeof editingId === "number") {
        await alertsApi.channels.update(editingId, draft);
      }
      setEditingId(null);
      await refresh();
    } catch (err) {
      setFormError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  }

  async function remove(c: NotificationChannel) {
    const ok = await confirm({
      title: t("alerts.deleteChannelTitle"),
      message: t("alerts.deleteChannelConfirm", { name: c.name }),
      confirmLabel: t("common.delete"),
      destructive: true,
    });
    if (!ok) return;
    try {
      await alertsApi.channels.remove(c.id);
      await refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    }
  }

  async function sendTest(c: NotificationChannel) {
    setTestResult(null);
    try {
      await alertsApi.channels.test(c.id);
      setTestResult({ id: c.id, ok: true, msg: t("alerts.testOk") });
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : String(err);
      setTestResult({ id: c.id, ok: false, msg: t("alerts.testFailed", { error: msg }) });
    }
  }

  return (
    <div className="space-y-4">
      <Surface>
        <div className="flex items-start justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold">{t("alerts.channelsTitle")}</h2>
            <p className="mt-1 text-sm text-[color:var(--color-muted)]">{t("alerts.channelsDescription")}</p>
          </div>
          <GhostButton onClick={startNew} disabled={editingId !== null}>
            {t("alerts.newChannel")}
          </GhostButton>
        </div>
        {error && <div className="mt-3"><ErrorText message={t("alerts.listError", { error })} /></div>}

        {editingId !== null && (
          <form onSubmit={save} className="mt-4 space-y-5">
            {/* Section 1: Identity — name + type. Type drives which config
                fields render below, so it lives at the top with the name. */}
            <FormSection title={t("alerts.sectionIdentity")} icon={TagIcon}>
              <div className="grid gap-3 sm:grid-cols-2">
                <Field label={t("alerts.fieldName")}>
                  <FieldInput value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} required />
                </Field>
                <Field label={t("alerts.fieldChannelType")}>
                  <SelectInput
                    value={draft.type}
                    onChange={(v) => setDraft({ ...draft, type: v as ChannelType })}
                    options={CHANNEL_TYPES.map((c) => ({ value: c, label: t(`alerts.channelType.${c}` as TranslationKey) }))}
                  />
                </Field>
              </div>
            </FormSection>

            {/* Section 2: Config — type-specific fields. Hidden URL/SMTP/Telegram
                blocks below all collapse into this section so the form length
                stays predictable regardless of channel type. */}
            <FormSection title={t("alerts.sectionConfig")} icon={channelIcon(draft.type)}>
              <div className="grid gap-3 sm:grid-cols-2">
                {draft.type !== "telegram" && draft.type !== "email" && draft.type !== "web_push" && (
                  <Field label={t("alerts.fieldUrl")} className="sm:col-span-2">
                    <FieldInput
                      type="url"
                      value={draft.config.url ?? ""}
                      onChange={(e) => setDraft({ ...draft, config: { ...draft.config, url: e.target.value } })}
                      required
                      placeholder={
                        draft.type === "ntfy" ? "https://ntfy.sh/lumen-alerts"
                        : draft.type === "discord" ? "https://discord.com/api/webhooks/…"
                        : "https://example.com/hooks/lumen"
                      }
                    />
                    <p className="mt-1 text-xs text-[color:var(--color-muted)]">
                      {draft.type === "ntfy"    && t("alerts.ntfyUrlHint")}
                      {draft.type === "discord" && t("alerts.discordUrlHint")}
                      {draft.type === "webhook" && t("alerts.webhookUrlHint")}
                    </p>
                  </Field>
                )}
                {draft.type === "web_push" && (
                  <div className="sm:col-span-2">
                    <WebPushPanel channelID={typeof editingId === "number" ? editingId : null} />
                  </div>
                )}
                {draft.type === "email" && (
                  <>
                    <Field label={t("alerts.fieldSmtpHost")}>
                      <FieldInput
                        type="text"
                        value={draft.config.smtp_host ?? ""}
                        onChange={(e) => setDraft({ ...draft, config: { ...draft.config, smtp_host: e.target.value } })}
                        required
                        placeholder="smtp.gmail.com"
                        autoComplete="off"
                      />
                      <p className="mt-1 text-xs text-[color:var(--color-muted)]">{t("alerts.smtpHostHint")}</p>
                    </Field>
                    <Field label={t("alerts.fieldSmtpPort")}>
                      <FieldInput
                        type="number"
                        value={draft.config.smtp_port ?? ""}
                        onChange={(e) => setDraft({ ...draft, config: { ...draft.config, smtp_port: e.target.value ? Number(e.target.value) : undefined } })}
                        required
                        placeholder="587"
                        min={1}
                        max={65535}
                      />
                      <p className="mt-1 text-xs text-[color:var(--color-muted)]">{t("alerts.smtpPortHint")}</p>
                    </Field>
                    <Field label={t("alerts.fieldFromAddr")}>
                      <FieldInput
                        type="email"
                        value={draft.config.from_addr ?? ""}
                        onChange={(e) => setDraft({ ...draft, config: { ...draft.config, from_addr: e.target.value } })}
                        required
                        placeholder="alerts@example.com"
                        autoComplete="off"
                      />
                    </Field>
                    <Field label={t("alerts.fieldToAddr")}>
                      <FieldInput
                        type="email"
                        value={draft.config.to_addr ?? ""}
                        onChange={(e) => setDraft({ ...draft, config: { ...draft.config, to_addr: e.target.value } })}
                        required
                        placeholder="oncall@example.com"
                        autoComplete="off"
                      />
                      <p className="mt-1 text-xs text-[color:var(--color-muted)]">{t("alerts.toAddrHint")}</p>
                    </Field>
                    <Field label={t("alerts.fieldSmtpUsername")}>
                      <FieldInput
                        type="text"
                        value={draft.config.username ?? ""}
                        onChange={(e) => setDraft({ ...draft, config: { ...draft.config, username: e.target.value } })}
                        required
                        placeholder="alerts@example.com"
                        autoComplete="off"
                      />
                      <p className="mt-1 text-xs text-[color:var(--color-muted)]">{t("alerts.smtpUsernameHint")}</p>
                    </Field>
                    <Field label={t("alerts.fieldSmtpPassword")}>
                      <FieldInput
                        type="password"
                        value={draft.config.password ?? ""}
                        onChange={(e) => setDraft({ ...draft, config: { ...draft.config, password: e.target.value } })}
                        required
                        autoComplete="new-password"
                      />
                      <p className="mt-1 text-xs text-[color:var(--color-muted)]">
                        {t("alerts.smtpPasswordHint")}
                        {draft.config.password === TELEGRAM_TOKEN_MASK && (
                          <> {" · "}{t("alerts.smtpPasswordKept")}</>
                        )}
                      </p>
                    </Field>
                  </>
                )}
                {draft.type === "telegram" && (
                  <>
                    <Field label={t("alerts.fieldBotToken")} className="sm:col-span-2">
                      <FieldInput
                        type="text"
                        value={draft.config.bot_token ?? ""}
                        onChange={(e) => setDraft({ ...draft, config: { ...draft.config, bot_token: e.target.value } })}
                        required
                        placeholder="123456:ABC-DEF…"
                        autoComplete="off"
                      />
                      <p className="mt-1 text-xs text-[color:var(--color-muted)]">
                        {t("alerts.telegramTokenHint")}
                        {draft.config.bot_token === TELEGRAM_TOKEN_MASK && (
                          <> {" · "}{t("alerts.telegramTokenKept")}</>
                        )}
                      </p>
                    </Field>
                    <Field label={t("alerts.fieldChatId")}>
                      <FieldInput
                        type="text"
                        value={draft.config.chat_id ?? ""}
                        onChange={(e) => setDraft({ ...draft, config: { ...draft.config, chat_id: e.target.value } })}
                        required
                        placeholder="-1001234567890"
                      />
                      <p className="mt-1 text-xs text-[color:var(--color-muted)]">{t("alerts.telegramChatHint")}</p>
                    </Field>
                  </>
                )}
                {draft.type === "ntfy" && (
                  <Field label={t("alerts.fieldPriority")}>
                    <SelectInput
                      value={draft.config.priority ?? ""}
                      onChange={(v) => setDraft({ ...draft, config: { ...draft.config, priority: v || undefined } })}
                      options={[
                        { value: "", label: "auto" },
                        { value: "min", label: "min" },
                        { value: "low", label: "low" },
                        { value: "default", label: "default" },
                        { value: "high", label: "high" },
                        { value: "urgent", label: "urgent" },
                      ]}
                    />
                  </Field>
                )}
              </div>
            </FormSection>

            {/* Section 3: Routing — min severity gate + enabled state */}
            <FormSection title={t("alerts.sectionRouting")} icon={Send}>
              <div className="space-y-3">
                <Field label={t("alerts.fieldMinSeverity")}>
                  <SegmentedControl
                    value={draft.min_severity ?? "info"}
                    onChange={(v) => setDraft({ ...draft, min_severity: v as AlertSeverity })}
                    options={SEVERITIES.map((s) => ({ value: s, label: t(`alerts.severity.${s}` as TranslationKey) }))}
                    ariaLabel={t("alerts.fieldMinSeverity")}
                  />
                  <p className="mt-1 text-xs text-[color:var(--color-muted)]">{t("alerts.minSeverityHint")}</p>
                </Field>
                <label className="flex items-center gap-2.5 rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-3 py-2 text-sm">
                  <Switch
                    checked={draft.enabled ?? true}
                    onCheckedChange={(v) => setDraft({ ...draft, enabled: v })}
                    ariaLabel={t("alerts.fieldEnabled")}
                  />
                  <span className="font-medium">{t("alerts.fieldEnabled")}</span>
                </label>
              </div>
            </FormSection>

            <div className="flex items-center gap-2 border-t border-[color:var(--color-border)] pt-4">
              <PrimaryButton disabled={submitting}>{submitting ? t("common.saving") : t("alerts.save")}</PrimaryButton>
              <GhostButton type="button" onClick={cancel}>{t("alerts.cancel")}</GhostButton>
              {formError && <ErrorText message={t("alerts.formError", { error: formError })} />}
            </div>
          </form>
        )}
      </Surface>

      {channels === null ? (
        <p className="text-sm text-[color:var(--color-muted)]">{t("alerts.listLoading")}</p>
      ) : channels.length === 0 ? (
        <EmptyState title={t("alerts.channelsEmpty")} detail={t("alerts.channelsEmptyHint")} />
      ) : (
        <Surface padded={false}>
          <ul className="divide-y divide-[color:var(--color-border)]">
            {channels.map((c) => {
              const isEditing = editingId === c.id;
              const otherEditing = editingId !== null && !isEditing;
              const Icon = channelIcon(c.type);
              const summary =
                c.type === "telegram"
                  ? t("alerts.telegramChannelSummary", { chat: c.config.chat_id ?? "" })
                  : c.type === "email"
                  ? t("alerts.emailChannelSummary", { to: c.config.to_addr ?? "", host: c.config.smtp_host ?? "" })
                  : c.config.url ?? "";
              return (
                <li
                  key={c.id}
                  className={`group flex items-center gap-3 px-4 py-3 transition-colors ${
                    isEditing ? "bg-[color:var(--color-bg)]" : otherEditing ? "opacity-50" : "hover:bg-[color:var(--color-bg)]/60"
                  } ${!c.enabled ? "opacity-70" : ""}`}
                >
                  <Switch
                    checked={c.enabled}
                    onCheckedChange={() => void toggleEnabled(c)}
                    ariaLabel={c.enabled ? t("alerts.disableChannel") : t("alerts.enableChannel")}
                    disabled={editingId !== null}
                  />
                  <div
                    className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-md ${
                      c.enabled
                        ? "bg-[color-mix(in_oklch,var(--lumen-teal)_12%,var(--color-card))] text-[color:var(--lumen-teal)]"
                        : "bg-[color:var(--color-border)]/40 text-[color:var(--color-muted)]"
                    }`}
                  >
                    <Icon size={16} strokeWidth={1.75} />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className={`text-sm font-medium ${c.enabled ? "text-[color:var(--color-fg)]" : "text-[color:var(--color-muted)]"}`}>
                        {c.name}
                      </span>
                      <span className="rounded-full bg-[color:var(--color-border)]/60 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-[color:var(--color-muted)]">
                        {t(`alerts.channelType.${c.type}` as TranslationKey)}
                      </span>
                      {c.min_severity !== "info" && (
                        <StatusPill tone={c.min_severity === "critical" ? "danger" : "warn"}>
                          ≥ {t(`alerts.severity.${c.min_severity}` as TranslationKey)}
                        </StatusPill>
                      )}
                      {isEditing && (
                        <span className="rounded-full bg-[color:var(--color-accent)]/15 px-2 py-0.5 text-xs text-[color:var(--color-accent)]">
                          {t("alerts.editingNow")}
                        </span>
                      )}
                    </div>
                    <p className="mt-0.5 truncate break-all text-xs text-[color:var(--color-muted)]">
                      {summary}
                    </p>
                    {testResult?.id === c.id && (
                      <p className={`mt-0.5 text-xs ${testResult.ok ? "text-[color:var(--color-accent)]" : "text-[color:var(--color-danger)]"}`}>
                        {testResult.msg}
                      </p>
                    )}
                  </div>
                  <div className="flex items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
                    <IconButton
                      onClick={() => void sendTest(c)}
                      disabled={editingId !== null}
                      label={t("alerts.test")}
                      className="h-8 w-8"
                    >
                      <Zap size={14} strokeWidth={1.75} />
                    </IconButton>
                    <IconButton
                      onClick={() => startEdit(c)}
                      disabled={editingId !== null}
                      label={t("alerts.editChannel")}
                      className="h-8 w-8"
                    >
                      <Pencil size={14} strokeWidth={1.75} />
                    </IconButton>
                    <IconButton
                      onClick={() => void remove(c)}
                      disabled={editingId !== null}
                      label={t("alerts.delete")}
                      className="h-8 w-8 hover:text-[color:var(--color-danger)]"
                    >
                      <Trash2 size={14} strokeWidth={1.75} />
                    </IconButton>
                  </div>
                </li>
              );
            })}
          </ul>
        </Surface>
      )}
    </div>
  );
}

// ---------------- host targeting ----------------

// HostTargetingFields encapsulates the three host-targeting inputs on
// the rule form:
//   1. Host names — multi-select checklist + free-text pattern (comma list,
//      single name, glob; whatever the operator types is stored as-is).
//   2. Host selector — tag-based label selector (`tier=critical,env=prod`).
//   3. A summary line that names which idiom is active and which hosts
//      currently match.
//
// Precedence on the backend: selector > host name(s) > empty (all).
// The UI mirrors that and disables the names block while a selector
// is set so the operator can't mistakenly think both are AND-ed.
function HostTargetingFields({
  draft,
  setDraft,
  hosts,
  inventory,
}: {
  draft: AlertRuleWrite;
  setDraft: (next: AlertRuleWrite) => void;
  hosts: Host[];
  inventory: Tag[];
}) {
  const { t } = useI18n();
  const selectorActive = (draft.host_selector ?? "").trim() !== "";

  // Parse the current draft.host comma list into a set for the checkbox UI.
  const segments = (draft.host || "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  const checkedSet = new Set(segments);

  function toggleName(name: string) {
    const next = new Set(checkedSet);
    if (next.has(name)) next.delete(name);
    else next.add(name);
    setDraft({ ...draft, host: Array.from(next).join(",") });
  }

  // Parse selector string into a Set of "key=value" entries; clicking a
  // chip toggles membership and we serialise back to the comma list.
  const selectorEntries = (draft.host_selector ?? "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  const selectorSet = new Set(selectorEntries);

  function toggleTag(key: string, value: string) {
    const entry = value === "" ? key : `${key}=${value}`;
    const next = new Set(selectorSet);
    // Per-key uniqueness — clicking another value for the same key
    // replaces the previous one rather than ANDing two impossible
    // values together.
    for (const existing of next) {
      const eqIdx = existing.indexOf("=");
      const existingKey = eqIdx < 0 ? existing : existing.slice(0, eqIdx);
      if (existingKey === key) next.delete(existing);
    }
    if (!selectorSet.has(entry)) next.add(entry);
    setDraft({ ...draft, host_selector: Array.from(next).join(",") });
  }

  // Quick preview: how many hosts the current rule would target.
  const preview = previewHostMatch(draft, hosts);

  return (
    <div className="space-y-3 rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] p-3">
      <div className="text-xs uppercase tracking-wide text-[color:var(--color-muted)]">
        {t("alerts.targetingTitle")}
      </div>

      <div className={selectorActive ? "opacity-50 pointer-events-none" : ""}>
        <label className="block text-xs font-medium mb-1">
          {t("alerts.fieldHostNames")}
        </label>
        {hosts.length === 0 ? (
          <p className="text-xs text-[color:var(--color-muted)]">
            {t("alerts.fieldHostNamesEmpty")}
          </p>
        ) : (
          <div className="mb-2 flex flex-wrap gap-2 rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-card)] p-2">
            {hosts.map((h) => (
              <label key={h.id} className="flex items-center gap-1 text-sm">
                <input
                  type="checkbox"
                  checked={checkedSet.has(h.name)}
                  onChange={() => toggleName(h.name)}
                  disabled={selectorActive}
                />
                <span className="font-mono">{h.name}</span>
              </label>
            ))}
          </div>
        )}
        <FieldInput
          value={draft.host}
          onChange={(e) => setDraft({ ...draft, host: e.target.value })}
          placeholder="web-1, web-*, db-1"
          disabled={selectorActive}
        />
        <p className="mt-1 text-xs text-[color:var(--color-muted)]">
          {t("alerts.fieldHostNamesHint")}
        </p>
      </div>

      <div>
        <label className="block text-xs font-medium mb-1">
          {t("alerts.fieldHostSelector")}
        </label>
        {(() => {
          // Each inventory tag becomes one dropdown. "— none —" means
          // "don't constrain by this tag". Picking a value sets/replaces
          // the requirement for that key.
          // Orphan entries — current selector references a (key, value)
          // not in the inventory — get rendered as deletable red pills
          // so the operator can spot/repair stale references after the
          // inventory shrinks.
          const inventoryPairs = new Set<string>();
          for (const tag of inventory) {
            for (const v of tag.values) {
              inventoryPairs.add(v === "" ? tag.key : `${tag.key}=${v}`);
            }
          }
          const orphans = selectorEntries.filter((e) => !inventoryPairs.has(e));

          // Current selected value per key in the draft selector.
          const selectedByKey: Record<string, string> = {};
          for (const entry of selectorEntries) {
            const eq = entry.indexOf("=");
            const k = eq < 0 ? entry : entry.slice(0, eq);
            const v = eq < 0 ? "" : entry.slice(eq + 1);
            selectedByKey[k] = v;
          }

          function setKeyValue(key: string, value: string | null) {
            const next = new Set(selectorSet);
            for (const existing of next) {
              const eqIdx = existing.indexOf("=");
              const existingKey = eqIdx < 0 ? existing : existing.slice(0, eqIdx);
              if (existingKey === key) next.delete(existing);
            }
            if (value !== null) {
              next.add(value === "" ? key : `${key}=${value}`);
            }
            setDraft({ ...draft, host_selector: Array.from(next).join(",") });
          }

          if (inventory.length === 0 && orphans.length === 0) {
            return (
              <p className="mb-2 text-xs text-[color:var(--color-muted)]">
                {t("alerts.fieldHostSelectorNoTags")}
              </p>
            );
          }
          return (
            <div className="mb-2 space-y-2 rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-card)] p-2">
              <div className="grid gap-2 sm:grid-cols-2">
                {inventory.map((tag) => {
                  const current = tag.key in selectedByKey ? selectedByKey[tag.key] : "__NONE__";
                  return (
                    <label key={tag.key} className="text-xs">
                      <span className="block text-[color:var(--color-muted)] mb-0.5 font-mono">
                        {tag.key}
                      </span>
                      <select
                        value={current === "__NONE__" ? "__NONE__" : current}
                        onChange={(e) => {
                          const v = e.target.value;
                          setKeyValue(tag.key, v === "__NONE__" ? null : v);
                        }}
                        className="w-full rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-2 py-1 text-sm focus:outline-none focus:ring-2 focus:ring-[color:var(--color-accent)]"
                      >
                        <option value="__NONE__">—</option>
                        {tag.values.map((v) => (
                          <option key={v} value={v}>
                            {v === "" ? "(empty)" : v}
                          </option>
                        ))}
                      </select>
                    </label>
                  );
                })}
              </div>
              {orphans.length > 0 && (
                <div className="flex flex-wrap gap-1.5 border-t border-[color:var(--color-border)] pt-2">
                  {orphans.map((entry) => {
                    const eqIdx = entry.indexOf("=");
                    const key = eqIdx < 0 ? entry : entry.slice(0, eqIdx);
                    const value = eqIdx < 0 ? "" : entry.slice(eqIdx + 1);
                    return (
                      <button
                        key={`orphan-${entry}`}
                        type="button"
                        onClick={() => toggleTag(key, value)}
                        title={t("alerts.orphanChipTitle")}
                        className="rounded-full border border-[color:var(--color-danger)] bg-[color:var(--color-danger)]/10 px-2 py-0.5 text-xs text-[color:var(--color-danger)] line-through hover:bg-[color:var(--color-danger)]/20"
                      >
                        {entry} <span className="not-italic no-underline">×</span>
                      </button>
                    );
                  })}
                </div>
              )}
            </div>
          );
        })()}
        <FieldInput
          value={draft.host_selector ?? ""}
          onChange={(e) => setDraft({ ...draft, host_selector: e.target.value })}
          placeholder="tier=critical,env=prod"
        />
        <p className="mt-1 text-xs text-[color:var(--color-muted)]">
          {t("alerts.fieldHostSelectorHint")}
        </p>
      </div>

      <p className={`text-xs ${preview.count === 0 && preview.idiom !== "all" ? "text-[color:var(--color-danger)]" : "text-[color:var(--color-fg)]"}`}>
        {preview.idiom === "selector" && preview.count === 0 && t("alerts.previewSelectorNoMatch")}
        {preview.idiom === "selector" && preview.count > 0  && t("alerts.previewSelector",   { count: preview.count })}
        {preview.idiom === "names"    && preview.count === 0 && t("alerts.previewNamesNoMatch")}
        {preview.idiom === "names"    && preview.count > 0  && t("alerts.previewNames",      { count: preview.count })}
        {preview.idiom === "all"      && t("alerts.previewAll", { count: preview.count })}
      </p>
    </div>
  );
}

// previewHostMatch is a quick UI-side estimate of how many registered
// hosts the rule targets. It's not authoritative — the engine also
// considers ever-seen hosts that aren't currently registered — but it's
// good enough to catch obvious "0 hosts match" mistakes before save.
function previewHostMatch(draft: AlertRuleWrite, hosts: Host[]): { idiom: "selector" | "names" | "all"; count: number } {
  const sel = (draft.host_selector ?? "").trim();
  if (sel !== "") {
    const reqs = parseSelectorClient(sel);
    let count = 0;
    for (const h of hosts) {
      let ok = true;
      for (const [k, v] of reqs) {
        if ((h.tags ?? {})[k] !== v) {
          ok = false;
          break;
        }
      }
      if (ok) count++;
    }
    return { idiom: "selector", count };
  }
  const raw = (draft.host || "").trim();
  if (raw === "") return { idiom: "all", count: hosts.length };
  const segs = raw.split(",").map((s) => s.trim()).filter(Boolean);
  const set = new Set<string>();
  for (const seg of segs) {
    if (/[*?[]/.test(seg)) {
      for (const h of hosts) if (globMatch(seg, h.name)) set.add(h.name);
    } else {
      set.add(seg);
    }
  }
  return { idiom: "names", count: set.size };
}

function parseSelectorClient(raw: string): [string, string][] {
  return raw
    .split(",")
    .map((p) => p.trim())
    .filter(Boolean)
    .map((p) => {
      const eq = p.indexOf("=");
      return eq < 0 ? [p, ""] : [p.slice(0, eq).trim(), p.slice(eq + 1).trim()];
    });
}

// Tiny glob: supports * and ?, no character classes (rare in host names).
function globMatch(pattern: string, name: string): boolean {
  // Escape regex specials except * and ?, then translate.
  const re = "^" + pattern.replace(/[.+^${}()|\\]/g, "\\$&").replace(/\*/g, ".*").replace(/\?/g, ".") + "$";
  return new RegExp(re).test(name);
}

// ---------------- shared bits ----------------

// FormSection wraps a labeled group inside the rule form. Just a small
// header (icon + uppercase title) over its children — no inner border,
// since the parent Surface already provides the card chrome.
function FormSection({
  title,
  icon: Icon,
  children,
}: {
  title: string;
  icon: typeof Cpu;
  children: React.ReactNode;
}) {
  return (
    <div>
      <div className="mb-2.5 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-[color:var(--color-muted)]">
        <Icon size={12} strokeWidth={2.25} />
        {title}
      </div>
      {children}
    </div>
  );
}

function SelectInput({
  value,
  onChange,
  options,
}: {
  value: string;
  onChange: (v: string) => void;
  options: { value: string; label: string }[];
}) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="w-full rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-[color:var(--color-accent)]"
    >
      {options.map((o) => (
        <option key={o.value} value={o.value}>{o.label}</option>
      ))}
    </select>
  );
}

// WebPushPanel — admin's per-channel browser subscription manager.
// Inline English (admin-only surface; rare edit). Subscribe flow runs
// entirely in the browser before POSTing the resulting PushSubscription
// to /api/alerts/web-push/subscribe.
function WebPushPanel({ channelID }: { channelID: number | null }) {
  const { t } = useI18n();
  const [subs, setSubs] = useState<WebPushSubscription[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [okMsg, setOkMsg] = useState<string | null>(null);

  useEffect(() => {
    if (channelID == null) {
      setSubs(null);
      return;
    }
    webPushApi.listSubscriptions(channelID).then(setSubs).catch((e) =>
      setErr(e instanceof ApiError ? e.message : String(e)),
    );
  }, [channelID]);

  async function subscribeThisBrowser() {
    if (channelID == null) return;
    setErr(null);
    setOkMsg(null);
    setBusy(true);
    try {
      if (!("serviceWorker" in navigator) || !("PushManager" in window)) {
        throw new Error(t("alerts.webPush.unsupported"));
      }
      const reg = await navigator.serviceWorker.ready;
      const perm = await Notification.requestPermission();
      if (perm !== "granted") {
        throw new Error(`Notification permission ${perm}.`);
      }
      const vapid = await webPushApi.getVAPIDPublicKey();
      // PushManager wants applicationServerKey as a BufferSource backed
      // by ArrayBuffer (not SharedArrayBuffer). Going via .buffer makes
      // TypeScript pick the right overload across browser/DOM types.
      const keyArr = urlBase64ToUint8Array(vapid.public_key);
      const sub = await reg.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: keyArr.buffer as ArrayBuffer,
      });
      const json = sub.toJSON();
      const p256dh = json.keys?.p256dh ?? "";
      const auth = json.keys?.auth ?? "";
      if (!p256dh || !auth) {
        throw new Error("Push subscription missing keys.");
      }
      const saved = await webPushApi.subscribe({
        channel_id: channelID,
        endpoint: sub.endpoint,
        p256dh,
        auth,
        label: navigator.userAgent.slice(0, 200),
      });
      setSubs((cur) => {
        const next = (cur ?? []).filter((s) => s.id !== saved.id);
        return [...next, saved];
      });
      setOkMsg(t("alerts.webPush.subscribed"));
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  async function removeSub(id: number) {
    try {
      await webPushApi.deleteSubscription(id);
      setSubs((cur) => (cur ?? []).filter((s) => s.id !== id));
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : String(e));
    }
  }

  if (channelID == null) {
    return (
      <div className="rounded-md border border-dashed border-[color:var(--color-border)] p-3 text-sm text-[color:var(--color-muted)]">
        {t("alerts.webPush.saveFirst")}
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <p className="text-xs text-[color:var(--color-muted)]">
        {t("alerts.webPush.intro")}
      </p>
      <div className="flex items-center gap-2">
        <GhostButton type="button" onClick={subscribeThisBrowser} disabled={busy}>
          {busy ? t("alerts.webPush.subscribing") : t("alerts.webPush.subscribe")}
        </GhostButton>
        {okMsg && <span className="text-xs text-[color:var(--color-accent)]">{okMsg}</span>}
        {err && <span className="text-xs text-red-500">{err}</span>}
      </div>
      <div className="rounded-md border border-[color:var(--color-border)]">
        <div className="border-b border-[color:var(--color-border)] px-3 py-2 text-xs font-medium text-[color:var(--color-muted)]">
          {t("alerts.webPush.listHeading", { count: subs?.length ?? 0 })}
        </div>
        {!subs ? (
          <div className="px-3 py-2 text-sm text-[color:var(--color-muted)]">{t("common.loading")}</div>
        ) : subs.length === 0 ? (
          <div className="px-3 py-2 text-sm text-[color:var(--color-muted)]">{t("alerts.webPush.noSubs")}</div>
        ) : (
          <ul>
            {subs.map((s) => (
              <li key={s.id} className="flex items-center justify-between gap-3 border-b border-[color:var(--color-border)] px-3 py-2 text-sm last:border-b-0">
                <div className="min-w-0">
                  <div className="truncate font-medium">{s.label || s.endpoint}</div>
                  <div className="truncate text-xs text-[color:var(--color-muted)]">
                    {new URL(s.endpoint).host} · {t("alerts.webPush.added", { ts: new Date(s.created_at).toLocaleString() })}
                  </div>
                </div>
                <IconButton label={t("alerts.webPush.remove")} onClick={() => removeSub(s.id)}>
                  <Trash2 size={14} />
                </IconButton>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

// urlBase64ToUint8Array converts a VAPID public key from base64url
// (the format /api/alerts/web-push/vapid-public-key returns it in)
// into the Uint8Array PushManager.subscribe expects as
// applicationServerKey. Standard recipe from the Push API spec.
function urlBase64ToUint8Array(base64String: string): Uint8Array {
  const padding = "=".repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/-/g, "+").replace(/_/g, "/");
  const raw = atob(base64);
  const out = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}

// MaintenancePanel — list + create / edit / delete maintenance windows
// (RFC 0003). Mirrors the ChannelsPanel shape (single list with
// state badges + an inline create form). Edit form is opened via
// a row's edit button; deleting is via trash icon with a confirm
// dialog.
function MaintenancePanel() {
  const [state, setState] = useState<"active" | "upcoming" | "past" | "all">("active");
  const [windows, setWindows] = useState<MaintenanceWindow[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<MaintenanceWindow | null>(null);
  const [draft, setDraft] = useState({ start_at: "", end_at: "", reason: "", scope: "" });
  const confirm = useConfirm();

  const refresh = useCallback(async () => {
    setError(null);
    try {
      const r = await maintenanceApi.list(state);
      setWindows(r.windows);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    }
  }, [state]);
  useEffect(() => { void refresh(); }, [refresh]);

  async function submit() {
    setBusy(true); setError(null);
    try {
      const body = {
        start_at: new Date(draft.start_at).toISOString(),
        end_at: new Date(draft.end_at).toISOString(),
        reason: draft.reason,
        scope_tags: draft.scope ? Object.fromEntries(draft.scope.split(",").map((kv) => {
          const [k, ...rest] = kv.split("=");
          return [k.trim(), rest.join("=").trim()];
        })) : {},
      };
      if (editing) {
        await maintenanceApi.update(editing.id, body);
      } else {
        await maintenanceApi.create(body);
      }
      setDraft({ start_at: "", end_at: "", reason: "", scope: "" });
      setEditing(null);
      await refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally { setBusy(false); }
  }

  async function del(w: MaintenanceWindow) {
    if (!await confirm({
      title: "Cancel maintenance window",
      message: `Cancel "${w.reason || w.id}"? Active firings resume on the next alert tick.`,
      confirmLabel: "Cancel window",
      destructive: true,
    })) return;
    try { await maintenanceApi.delete(w.id); await refresh(); }
    catch (err) { setError(err instanceof ApiError ? err.message : String(err)); }
  }

  function beginEdit(w: MaintenanceWindow) {
    setEditing(w);
    setDraft({
      start_at: w.start_at.slice(0, 16),
      end_at: w.end_at.slice(0, 16),
      reason: w.reason,
      scope: Object.entries(w.scope_tags).map(([k, v]) => `${k}=${v}`).join(","),
    });
  }

  const stateBadge = (w: MaintenanceWindow): { text: string; tone: "ok" | "warn" | "muted" } => {
    const now = Date.now();
    const start = Date.parse(w.start_at);
    const end = Date.parse(w.end_at);
    if (now >= start && now < end) return { text: "active", tone: "ok" };
    if (now < start) return { text: "upcoming", tone: "warn" };
    return { text: "past", tone: "muted" };
  };

  return (
    <div className="space-y-4">
      <SettingsPanel
        title="Maintenance windows"
        description="Schedule planned downtime. Alerts matching the scope are suppressed while a window is active."
      >
        <SegmentedControl
          value={state}
          onChange={(v) => setState(v as typeof state)}
          ariaLabel="Window state filter"
          options={[
            { value: "active", label: "Active" },
            { value: "upcoming", label: "Upcoming" },
            { value: "past", label: "Past" },
            { value: "all", label: "All" },
          ]}
        />
        <form
          onSubmit={(e) => { e.preventDefault(); void submit(); }}
          className="grid grid-cols-2 gap-3 mt-3"
        >
          <Field label="Start (browser time)">
            <FieldInput type="datetime-local" value={draft.start_at} onChange={(e) => setDraft({ ...draft, start_at: e.target.value })} />
          </Field>
          <Field label="End (browser time)">
            <FieldInput type="datetime-local" value={draft.end_at} onChange={(e) => setDraft({ ...draft, end_at: e.target.value })} />
          </Field>
          <Field label="Reason">
            <FieldInput value={draft.reason} onChange={(e) => setDraft({ ...draft, reason: e.target.value })} placeholder="Firmware update" />
          </Field>
          <Field label="Tag scope">
            <FieldInput value={draft.scope} onChange={(e) => setDraft({ ...draft, scope: e.target.value })} placeholder="env=prod, tier=db" />
          </Field>
          <div className="col-span-2 flex items-center gap-2">
            <PrimaryButton type="submit" disabled={busy || !draft.start_at || !draft.end_at}>
              {busy ? "Saving…" : editing ? "Update" : "Create"}
            </PrimaryButton>
            {editing && (
              <GhostButton type="button" onClick={() => { setEditing(null); setDraft({ start_at: "", end_at: "", reason: "", scope: "" }); }}>
                Cancel edit
              </GhostButton>
            )}
            {error && <ErrorText message={error} />}
          </div>
        </form>
      </SettingsPanel>

      <SettingsPanel title={`${state.charAt(0).toUpperCase() + state.slice(1)} windows (${windows.length})`} description="List of windows in this state. Edit or cancel via the row actions.">
        {windows.length === 0 ? (
          <p className="text-sm text-[color:var(--color-muted)]">No {state} windows.</p>
        ) : (
          <ul className="space-y-1.5">
            {windows.map((w) => {
              const b = stateBadge(w);
              return (
                <li key={w.id} className="flex items-center justify-between rounded-md border border-[color:var(--color-border)] px-3 py-2 text-sm">
                  <div>
                    <div className="font-medium">
                      {w.reason || <em className="text-[color:var(--color-muted)]">(no reason)</em>}
                    </div>
                    <div className="text-xs text-[color:var(--color-muted)]">
                      {new Date(w.start_at).toLocaleString()} → {new Date(w.end_at).toLocaleString()}
                      {Object.keys(w.scope_tags).length > 0 && (
                        <span className="ml-2">
                          scope: {Object.entries(w.scope_tags).map(([k, v]) => `${k}=${v}`).join(", ")}
                        </span>
                      )}
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <StatusPill tone={b.tone}>{b.text}</StatusPill>
                    <IconButton label="Edit window" onClick={() => beginEdit(w)}><Pencil size={14} /></IconButton>
                    <IconButton label="Cancel window" onClick={() => void del(w)}><Trash2 size={14} /></IconButton>
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </SettingsPanel>
    </div>
  );
}
