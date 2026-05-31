import { useCallback, useEffect, useState } from "react";
import { BellRing, History, Send, ShieldAlert, Cable, Tag as TagIcon } from "lucide-react";
import {
  alertsApi,
  ApiError,
  hostsApi,
  tagsApi,
  TELEGRAM_TOKEN_MASK,
  type AlertComparator,
  type AlertEvent,
  type AlertMetric,
  type AlertRule,
  type AlertRuleWrite,
  type AlertSeverity,
  type ChannelType,
  type DeliveryStatus,
  type DeliveryView,
  type Host,
  type NotificationChannel,
  type NotificationChannelWrite,
  type Tag,
} from "@/lib/api";
import { relativeTime } from "@/lib/time";
import {
  ErrorText,
  Field,
  FieldInput,
  GhostButton,
  PrimaryButton,
} from "@/components/CenterCard";
import { EmptyState, StatusPill, Surface } from "@/components/ui";
import { AlertTags } from "@/components/AlertTags";
import { useI18n } from "@/i18n/useI18n";
import type { TranslationKey } from "@/i18n/types";

type AlertsTab = "active" | "history" | "deliveries" | "rules" | "channels" | "tags";

const TABS: { id: AlertsTab; labelKey: TranslationKey; icon: typeof BellRing }[] = [
  { id: "active",     labelKey: "alerts.tabs.active",     icon: BellRing },
  { id: "history",    labelKey: "alerts.tabs.history",    icon: History },
  { id: "deliveries", labelKey: "alerts.tabs.deliveries", icon: Send },
  { id: "rules",      labelKey: "alerts.tabs.rules",      icon: ShieldAlert },
  { id: "channels",   labelKey: "alerts.tabs.channels",   icon: Cable },
  { id: "tags",       labelKey: "alerts.tabs.tags",       icon: TagIcon },
];

const METRICS: AlertMetric[] = ["cpu_pct", "ram_pct", "swap_pct", "disk_pct", "load1", "offline"];
const COMPARATORS: AlertComparator[] = ["gt", "lt"];
const SEVERITIES: AlertSeverity[] = ["info", "warning", "critical"];
const CHANNEL_TYPES: ChannelType[] = ["ntfy", "discord", "webhook", "telegram", "email"];

const EVENT_POLL_MS = 15_000;

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
  const [error, setError] = useState<string | null>(null);
  const [now, setNow] = useState(Date.now());
  const [limit, setLimit] = useState(EVENT_PAGE_STEP);
  const [loadingMore, setLoadingMore] = useState(false);

  const refresh = useCallback(async (nextLimit: number = limit) => {
    try {
      const evs = await alertsApi.events(state, nextLimit);
      setEvents(evs);
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
        {events.map((ev) => (
          <li key={ev.id} className="flex flex-col gap-1 px-5 py-4 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <StatusPill tone={toneForSeverity(ev.severity, ev.state)}>
                  {t(ev.state === "firing" ? "alerts.state.firing" : "alerts.state.resolved")}
                </StatusPill>
                <span className="text-sm font-medium">{ev.rule_name}</span>
                <span className="text-xs text-[color:var(--color-muted)]">{ev.host}</span>
              </div>
              <p className="mt-1 text-sm text-[color:var(--color-muted)]">{ev.message}</p>
            </div>
            <div className="text-right text-xs text-[color:var(--color-muted)]">
              <div>{t("alerts.sinceLabel", { time: relativeTime(ev.started_at, now, locale) })}</div>
              {ev.resolved_at && (
                <div>{t("alerts.resolvedAt", { time: relativeTime(ev.resolved_at, now, locale) })}</div>
              )}
            </div>
          </li>
        ))}
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
              return (
                <li key={d.id} className="flex flex-col gap-1 px-5 py-4 sm:flex-row sm:items-start sm:justify-between">
                  <div className="min-w-0 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <StatusPill tone={statusTone(d.status)}>
                        {t(`alerts.deliveryStatus.${d.status}` as TranslationKey)}
                      </StatusPill>
                      <StatusPill tone={toneForSeverity(d.severity, "firing")}>
                        {t(`alerts.severity.${d.severity}` as TranslationKey)}
                      </StatusPill>
                      <span className="text-sm font-medium">{d.channel_name}</span>
                      <span className="text-xs text-[color:var(--color-muted)]">
                        ({t(`alerts.channelType.${d.channel_type}` as TranslationKey)})
                      </span>
                    </div>
                    {typeof (d.payload as { message?: string })?.message === "string" && (
                      <p className="text-sm text-[color:var(--color-muted)]">
                        {(d.payload as { message: string }).message}
                      </p>
                    )}
                    <p className="text-xs text-[color:var(--color-muted)]">
                      {t("alerts.deliveryMeta", {
                        attempts: d.attempts,
                        httpStatus: d.http_status ?? "—",
                      })}
                    </p>
                    {d.error && (
                      <p className="text-xs text-[color:var(--color-danger)] break-words">
                        {d.error}
                      </p>
                    )}
                    {d.next_retry_at && d.status === "pending" && (
                      <p className="text-xs text-[color:var(--color-muted)]">
                        {t("alerts.deliveryNextRetry", { time: relativeTime(d.next_retry_at, now, locale) })}
                      </p>
                    )}
                  </div>
                  <div className="flex flex-col items-end gap-1 text-xs text-[color:var(--color-muted)]">
                    <span>{t("alerts.deliveryQueuedAt", { time: relativeTime(d.created_at, now, locale) })}</span>
                    {d.sent_at && (
                      <span>{t("alerts.deliverySentAt", { time: relativeTime(d.sent_at, now, locale) })}</span>
                    )}
                    {isRetryable && (
                      <GhostButton onClick={() => void retry(d.id)} disabled={retryBusy === d.id}>
                        {retryBusy === d.id ? t("common.saving") : t("alerts.deliveryRetry")}
                      </GhostButton>
                    )}
                  </div>
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

function RulesPanel() {
  const { t } = useI18n();
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
    if (!window.confirm(t("alerts.deleteRuleConfirm", { name: r.name }))) return;
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
        {error && <div className="mt-3"><ErrorText message={t("alerts.listError", { error })} /></div>}

        {editingId !== null && (
          <form onSubmit={save} className="mt-4 grid gap-3 sm:grid-cols-2">
            <Field label={t("alerts.fieldName")}>
              <FieldInput value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} required />
            </Field>
            <Field label={t("alerts.fieldMetric")}>
              <SelectInput
                value={draft.metric}
                onChange={(v) => setDraft({ ...draft, metric: v as AlertMetric })}
                options={METRICS.map((m) => ({ value: m, label: t(`alerts.metric.${m}` as TranslationKey) }))}
              />
            </Field>
            {draft.metric !== "offline" && (
              <>
                <Field label={t("alerts.fieldComparator")}>
                  <SelectInput
                    value={draft.comparator}
                    onChange={(v) => setDraft({ ...draft, comparator: v as AlertComparator })}
                    options={COMPARATORS.map((c) => ({ value: c, label: t(`alerts.comparator.${c}` as TranslationKey) }))}
                  />
                </Field>
                <Field label={t("alerts.fieldThreshold")}>
                  <FieldInput
                    type="number"
                    step="any"
                    value={draft.threshold}
                    onChange={(e) => setDraft({ ...draft, threshold: Number(e.target.value) })}
                  />
                </Field>
              </>
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
            <div className="sm:col-span-2">
              <HostTargetingFields draft={draft} setDraft={setDraft} hosts={hosts} inventory={tagInventory} />
            </div>
            <Field label={t("alerts.fieldSeverity")}>
              <SelectInput
                value={draft.severity}
                onChange={(v) => setDraft({ ...draft, severity: v as AlertSeverity })}
                options={SEVERITIES.map((s) => ({ value: s, label: t(`alerts.severity.${s}` as TranslationKey) }))}
              />
            </Field>
            <label className="flex items-center gap-2 self-end pb-1 text-sm">
              <input
                type="checkbox"
                checked={draft.enabled ?? true}
                onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })}
              />
              {t("alerts.fieldEnabled")}
            </label>
            <div className="sm:col-span-2">
              <span className="block text-xs uppercase tracking-wide text-[color:var(--color-muted)] mb-1">
                {t("alerts.fieldChannels")}
              </span>
              {channels.length === 0 ? (
                <p className="text-xs text-[color:var(--color-muted)]">
                  {t("alerts.fieldChannelsEmpty")}
                </p>
              ) : (
                <div className="space-y-1 rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] p-2">
                  {channels.map((ch) => {
                    const checked = (draft.channel_ids ?? []).includes(ch.id);
                    return (
                      <label key={ch.id} className="flex items-center gap-2 text-sm">
                        <input
                          type="checkbox"
                          checked={checked}
                          onChange={() => toggleDraftChannel(ch.id)}
                        />
                        <span>{ch.name}</span>
                        <span className="text-xs text-[color:var(--color-muted)]">
                          ({t(`alerts.channelType.${ch.type}` as TranslationKey)}
                          {ch.min_severity !== "info" ? ` · ≥ ${t(`alerts.severity.${ch.min_severity}` as TranslationKey)}` : ""}
                          {!ch.enabled ? ` · ${t("alerts.disabledLabel")}` : ""})
                        </span>
                      </label>
                    );
                  })}
                </div>
              )}
              <p className="mt-1 text-xs text-[color:var(--color-muted)]">
                {t("alerts.fieldChannelsHint")}
              </p>
            </div>
            <div className="sm:col-span-2 flex items-center gap-2">
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
        <EmptyState title={t("alerts.rulesEmpty")} />
      ) : (
        <Surface padded={false}>
          <ul className="divide-y divide-[color:var(--color-border)]">
            {rules.map((r) => {
              const isEditing = editingId === r.id;
              const otherEditing = editingId !== null && !isEditing;
              return (
                <li
                  key={r.id}
                  className={`flex flex-col gap-2 px-5 py-4 sm:flex-row sm:items-center sm:justify-between ${
                    isEditing ? "bg-[color:var(--color-bg)]" : otherEditing ? "opacity-50" : ""
                  }`}
                >
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-sm font-medium">{r.name}</span>
                      <StatusPill tone={r.enabled ? toneForSeverity(r.severity, "firing") : "muted"}>
                        {t(`alerts.severity.${r.severity}` as TranslationKey)}
                      </StatusPill>
                      {!r.enabled && <span className="text-xs text-[color:var(--color-muted)]">{t("alerts.disabledLabel")}</span>}
                      {isEditing && (
                        <span className="rounded-full bg-[color:var(--color-accent)]/15 px-2 py-0.5 text-xs text-[color:var(--color-accent)]">
                          {t("alerts.editingNow")}
                        </span>
                      )}
                    </div>
                    <p className="mt-1 text-xs text-[color:var(--color-muted)]">
                      {ruleDescription(r, t)}
                    </p>
                    <p className="mt-1 text-xs text-[color:var(--color-muted)]">
                      {ruleRoutingDescription(r, channels, t)}
                    </p>
                  </div>
                  <div className="flex items-center gap-2">
                    <GhostButton onClick={() => startEdit(r)} disabled={editingId !== null}>
                      {t("alerts.editRule")}
                    </GhostButton>
                    <GhostButton onClick={() => void remove(r)} disabled={editingId !== null}>
                      {t("alerts.delete")}
                    </GhostButton>
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
    if (!window.confirm(t("alerts.deleteChannelConfirm", { name: c.name }))) return;
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
          <form onSubmit={save} className="mt-4 grid gap-3 sm:grid-cols-2">
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
            {draft.type !== "telegram" && draft.type !== "email" && (
              <Field label={t("alerts.fieldUrl")}>
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
                <Field label={t("alerts.fieldBotToken")}>
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
            <Field label={t("alerts.fieldMinSeverity")}>
              <SelectInput
                value={draft.min_severity ?? "info"}
                onChange={(v) => setDraft({ ...draft, min_severity: v as AlertSeverity })}
                options={SEVERITIES.map((s) => ({ value: s, label: t(`alerts.severity.${s}` as TranslationKey) }))}
              />
              <p className="mt-1 text-xs text-[color:var(--color-muted)]">{t("alerts.minSeverityHint")}</p>
            </Field>
            <label className="flex items-center gap-2 self-end pb-1 text-sm">
              <input
                type="checkbox"
                checked={draft.enabled ?? true}
                onChange={(e) => setDraft({ ...draft, enabled: e.target.checked })}
              />
              {t("alerts.fieldEnabled")}
            </label>
            <div className="sm:col-span-2 flex items-center gap-2">
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
        <EmptyState title={t("alerts.channelsEmpty")} />
      ) : (
        <Surface padded={false}>
          <ul className="divide-y divide-[color:var(--color-border)]">
            {channels.map((c) => {
              const isEditing = editingId === c.id;
              const otherEditing = editingId !== null && !isEditing;
              return (
                <li
                  key={c.id}
                  className={`flex flex-col gap-2 px-5 py-4 sm:flex-row sm:items-center sm:justify-between ${
                    isEditing ? "bg-[color:var(--color-bg)]" : otherEditing ? "opacity-50" : ""
                  }`}
                >
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium">{c.name}</span>
                      <StatusPill tone={c.enabled ? "ok" : "muted"}>
                        {t(`alerts.channelType.${c.type}` as TranslationKey)}
                      </StatusPill>
                      {!c.enabled && <span className="text-xs text-[color:var(--color-muted)]">{t("alerts.disabledLabel")}</span>}
                      {isEditing && (
                        <span className="rounded-full bg-[color:var(--color-accent)]/15 px-2 py-0.5 text-xs text-[color:var(--color-accent)]">
                          {t("alerts.editingNow")}
                        </span>
                      )}
                    </div>
                    <p className="mt-1 break-all text-xs text-[color:var(--color-muted)]">
                      {c.type === "telegram"
                        ? t("alerts.telegramChannelSummary", { chat: c.config.chat_id ?? "" })
                        : c.type === "email"
                        ? t("alerts.emailChannelSummary", { to: c.config.to_addr ?? "", host: c.config.smtp_host ?? "" })
                        : c.config.url ?? ""}
                    </p>
                    {c.min_severity !== "info" && (
                      <p className="text-xs text-[color:var(--color-muted)]">
                        {t("alerts.minSeverityBadge", { severity: t(`alerts.severity.${c.min_severity}` as TranslationKey) })}
                      </p>
                    )}
                    {testResult?.id === c.id && (
                      <p className={`mt-1 text-xs ${testResult.ok ? "text-[color:var(--color-fg)]" : "text-[color:var(--color-danger)]"}`}>
                        {testResult.msg}
                      </p>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    <GhostButton onClick={() => void sendTest(c)} disabled={editingId !== null}>
                      {t("alerts.test")}
                    </GhostButton>
                    <GhostButton onClick={() => startEdit(c)} disabled={editingId !== null}>
                      {t("alerts.editChannel")}
                    </GhostButton>
                    <GhostButton onClick={() => void remove(c)} disabled={editingId !== null}>
                      {t("alerts.delete")}
                    </GhostButton>
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
