import { useCallback, useEffect, useState } from "react";
import { Server, User as UserIcon, Gauge, Archive, BarChart3, ScrollText, Activity, KeyRound, Trash2, Copy, Check, Palette, Shield, Globe, Database, Play, Download, RotateCcw } from "lucide-react";
import {
  hostsApi,
  authApi,
  settingsApi,
  hubStatsApi,
  apiKeysApi,
  oidcApi,
  publicStatusApi,
  backupApi,
  ApiError,
  type Host,
  type User,
  type SettingsResponse,
  type HubStatsResponse,
  type ApiKey,
  type ApiKeyCreated,
  type ApiKeyScope,
  type Theme,
  type UnitsMode,
  type ReduceMotion,
  type Density,
  type OIDCSettings,
  type PublicStatusConfig,
  type BackupSettings,
  type BackupEntry,
} from "@/lib/api";
import { relativeTime } from "@/lib/time";
import { copyToClipboard } from "@/lib/clipboard";
import { ErrorText, Field, FieldInput, GhostButton, PrimaryButton } from "@/components/CenterCard";
import { IconButton, SegmentedControl, Surface } from "@/components/ui";
import { TokenReveal } from "@/components/TokenReveal";
import { useConfirm } from "@/components/ConfirmDialog";
import { usePrefs } from "@/lib/userPrefs";
import { useI18n } from "@/i18n/useI18n";

type SettingsTab = "hosts" | "account" | "display" | "runtime" | "retention" | "downsample" | "logs" | "hub-status" | "api-keys" | "sso" | "status-page" | "backup";

const TABS: {
  id: SettingsTab;
  labelKey:
    | "settings.tabs.hosts"
    | "settings.tabs.account"
    | "settings.tabs.display"
    | "settings.tabs.runtime"
    | "settings.tabs.retention"
    | "settings.tabs.downsample"
    | "settings.tabs.logs"
    | "settings.tabs.hubStatus"
    | "settings.tabs.apiKeys"
    | "settings.tabs.sso"
    | "settings.tabs.statusPage"
    | "settings.tabs.backup";
  icon: typeof Server;
}[] = [
  { id: "hosts",       labelKey: "settings.tabs.hosts",      icon: Server },
  { id: "account",     labelKey: "settings.tabs.account",    icon: UserIcon },
  { id: "display",     labelKey: "settings.tabs.display",    icon: Palette },
  { id: "runtime",     labelKey: "settings.tabs.runtime",    icon: Gauge },
  { id: "retention",   labelKey: "settings.tabs.retention",  icon: Archive },
  { id: "downsample",  labelKey: "settings.tabs.downsample", icon: BarChart3 },
  { id: "logs",        labelKey: "settings.tabs.logs",       icon: ScrollText },
  { id: "hub-status",  labelKey: "settings.tabs.hubStatus",  icon: Activity },
  { id: "api-keys",    labelKey: "settings.tabs.apiKeys",    icon: KeyRound },
  { id: "sso",         labelKey: "settings.tabs.sso",        icon: Shield },
  { id: "status-page", labelKey: "settings.tabs.statusPage", icon: Globe },
  { id: "backup",      labelKey: "settings.tabs.backup",     icon: Database },
];

export function Settings({ user }: { user: User }) {
  const { t } = useI18n();
  const [tab, setTab] = useState<SettingsTab>("hosts");
  return (
    <div className="space-y-6">
      <header>
        <h2 className="text-3xl font-bold tracking-tight text-[color:var(--color-fg)]">{t("shell.settings")}</h2>
      </header>
      <div className="grid grid-cols-1 gap-6 md:grid-cols-[200px_1fr]">
        <nav className="space-y-1" aria-label={t("shell.settings")}>
          {TABS.map((item) => {
            const Icon = item.icon;
            const active = item.id === tab;
            return (
              <button
                key={item.id}
                type="button"
                onClick={() => setTab(item.id)}
                aria-current={active ? "page" : undefined}
                className={`flex w-full items-center gap-2.5 rounded-md px-3 py-2 text-sm font-medium transition-colors ${
                  active
                    ? "bg-[color-mix(in_oklch,var(--lumen-teal)_15%,transparent)] text-[color:var(--color-fg)]"
                    : "text-[color:var(--color-muted)] hover:bg-[color:var(--color-border)]/40 hover:text-[color:var(--color-fg)]"
                }`}
              >
                <Icon size={16} strokeWidth={active ? 2.25 : 1.75} className={active ? "text-[color:var(--lumen-teal)]" : ""} />
                {t(item.labelKey)}
              </button>
            );
          })}
        </nav>
        <div>
          {tab === "hosts"      && <HostsSettings />}
          {tab === "account"    && <AccountSettings user={user} />}
          {tab === "display"    && <DisplaySettings />}
          {tab === "runtime"    && <RuntimeSettings />}
          {tab === "retention"  && <RetentionSettings />}
          {tab === "downsample" && <DownsampleSettings />}
          {tab === "logs"       && <LogManagementSettings />}
          {tab === "hub-status" && <HubStatusSettings />}
          {tab === "api-keys"   && <ApiKeysSettings />}
          {tab === "sso"        && <SSOSettings />}
          {tab === "status-page" && <StatusPageSettings />}
          {tab === "backup"      && <BackupSettings />}
        </div>
      </div>
    </div>
  );
}

// ─── Hosts sub-tab (unchanged content from old Settings) ─────────────────────

type RevealState = { hostName: string; token: string } | null;

function HostsSettings() {
  const { locale, t } = useI18n();
  const [hosts, setHosts] = useState<Host[]>([]);
  const [loading, setLoading] = useState(true);
  const [listError, setListError] = useState<string | null>(null);
  const [now, setNow] = useState(Date.now());
  const [reveal, setReveal] = useState<RevealState>(null);

  const refresh = useCallback(async () => {
    setListError(null);
    try {
      const list = await hostsApi.list();
      setHosts(list ?? []);
    } catch (err) {
      setListError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
    const t = window.setInterval(() => setNow(Date.now()), 5000);
    return () => window.clearInterval(t);
  }, [refresh]);

  return (
    <div className="space-y-6">
      <section>
        <h2 className="text-base font-semibold tracking-tight mb-3">{t("settings.hostsTitle")}</h2>
        <p className="text-sm text-[color:var(--color-muted)] mb-4">
          {t("settings.hostsDescription")}
        </p>

        <CreateHostForm
          onCreated={(host, token) => {
            setReveal({ hostName: host.name, token });
            refresh();
          }}
        />

        {reveal && (
          <div className="mt-4">
            <TokenReveal
              hostName={reveal.hostName}
              token={reveal.token}
              onDismiss={() => setReveal(null)}
            />
          </div>
        )}
      </section>

      <section>
        {listError && <ErrorText message={listError} />}
        {loading ? (
          <p className="text-sm text-[color:var(--color-muted)]">{t("settings.loadingHosts")}</p>
        ) : hosts.length === 0 ? (
          <p className="text-sm text-[color:var(--color-muted)]">
            {t("settings.noHosts")}
          </p>
        ) : (
          <HostsTable
            hosts={hosts}
            now={now}
            onChanged={refresh}
            onTokenRevealed={(hostName, token) => setReveal({ hostName, token })}
            onDeleted={(hostName) => {
              setReveal((current) => current?.hostName === hostName ? null : current);
            }}
            locale={locale}
            t={t}
          />
        )}
      </section>
    </div>
  );
}

function CreateHostForm({
  onCreated,
}: {
  onCreated: (host: Host, token: string) => void;
}) {
  const { t } = useI18n();
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      const res = await hostsApi.create(name);
      onCreated(res.host, res.token);
      setName("");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="flex items-end gap-2 max-w-md">
      <div className="flex-1">
        <Field label={t("settings.newHostName")}>
          <FieldInput
            type="text"
            placeholder="pve-01"
            value={name}
            onChange={(e) => setName(e.target.value)}
            pattern="[A-Za-z0-9_.\-]+"
            required
          />
        </Field>
      </div>
      <PrimaryButton disabled={busy || !name}>
        {busy ? t("common.creating") : t("common.create")}
      </PrimaryButton>
      {error && (
        <div className="ml-2">
          <ErrorText message={error} />
        </div>
      )}
    </form>
  );
}

function HostsTable({
  hosts,
  now,
  onChanged,
  onTokenRevealed,
  onDeleted,
  locale,
  t,
}: {
  hosts: Host[];
  now: number;
  onChanged: () => void;
  onTokenRevealed: (hostName: string, token: string) => void;
  onDeleted: (hostName: string) => void;
  locale: ReturnType<typeof useI18n>["locale"];
  t: ReturnType<typeof useI18n>["t"];
}) {
  return (
    <div className="overflow-x-auto rounded-lg border border-[color:var(--color-border)]">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-xs uppercase tracking-wide text-[color:var(--color-muted)] bg-[color:var(--color-card)]">
            <th className="px-3 py-2 font-medium">{t("settings.tableName")}</th>
            <th className="px-3 py-2 font-medium">{t("settings.tableTags")}</th>
            <th className="px-3 py-2 font-medium">{t("settings.tableLastSeen")}</th>
            <th className="px-3 py-2 font-medium">{t("settings.tableCreated")}</th>
            <th className="px-3 py-2 font-medium text-right">{t("common.actions")}</th>
          </tr>
        </thead>
        <tbody>
          {hosts.map((h) => (
            <HostRow
              key={h.id}
              host={h}
              now={now}
              onChanged={onChanged}
              onTokenRevealed={onTokenRevealed}
              onDeleted={onDeleted}
              locale={locale}
              t={t}
            />
          ))}
        </tbody>
      </table>
    </div>
  );
}

function HostRow({
  host,
  now,
  onChanged,
  onTokenRevealed,
  onDeleted,
  locale,
  t,
}: {
  host: Host;
  now: number;
  onChanged: () => void;
  onTokenRevealed: (hostName: string, token: string) => void;
  onDeleted: (hostName: string) => void;
  locale: ReturnType<typeof useI18n>["locale"];
  t: ReturnType<typeof useI18n>["t"];
}) {
  const [busy, setBusy] = useState(false);
  const confirm = useConfirm();

  async function rotate() {
    const ok = await confirm({
      title: t("settings.rotateTitle"),
      message: t("settings.rotateConfirm", { name: host.name }),
      confirmLabel: t("settings.rotateAction"),
      destructive: true,
    });
    if (!ok) return;
    setBusy(true);
    try {
      const res = await hostsApi.rotate(host.id);
      onTokenRevealed(host.name, res.token);
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : String(err);
      window.alert(`${t("settings.rotateFailed")}: ${msg}`);
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    const ok = await confirm({
      title: t("settings.deleteTitle"),
      message: t("settings.deleteConfirm", { name: host.name }),
      confirmLabel: t("common.delete"),
      destructive: true,
    });
    if (!ok) return;
    setBusy(true);
    try {
      await hostsApi.remove(host.id);
      onDeleted(host.name);
      onChanged();
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : String(err);
      window.alert(`${t("settings.deleteFailed")}: ${msg}`);
    } finally {
      setBusy(false);
    }
  }

  return (
    <tr className="border-t border-[color:var(--color-border)]">
      <td className="px-3 py-2 font-mono align-top">{host.name}</td>
      <td className="px-3 py-2 align-top">
        <HostTagsCell host={host} t={t} />
      </td>
      <td className="px-3 py-2 text-[color:var(--color-muted)] align-top">
        {host.last_seen_at ? relativeTime(host.last_seen_at, now, locale) : t("common.never")}
      </td>
      <td className="px-3 py-2 text-[color:var(--color-muted)] align-top">
        {new Date(host.created_at).toLocaleString()}
      </td>
      <td className="px-3 py-2 text-right space-x-2 whitespace-nowrap align-top">
        <GhostButton onClick={rotate} disabled={busy}>
          {t("settings.rotateToken")}
        </GhostButton>
        <GhostButton onClick={remove} disabled={busy}>
          {t("common.delete")}
        </GhostButton>
      </td>
    </tr>
  );
}

// HostTagsCell is read-only now. Tag assignment moved into Alerts → Tags
// so the same screen that defines the tag inventory also assigns it.
// Settings still surfaces the chips for at-a-glance "which tags does this
// host carry" while listing infrastructure.
function HostTagsCell({
  host,
  t,
}: {
  host: Host;
  t: ReturnType<typeof useI18n>["t"];
}) {
  const chips = Object.entries(host.tags ?? {});
  return (
    <div className="flex flex-col gap-1">
      <div className="flex flex-wrap items-center gap-1.5">
        {chips.length === 0 ? (
          <span className="text-xs text-[color:var(--color-muted)]">{t("settings.tagsEmpty")}</span>
        ) : (
          chips.map(([k, v]) => (
            <span key={k} className="rounded-full border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-2 py-0.5 text-xs">
              {v ? `${k}=${v}` : k}
            </span>
          ))
        )}
      </div>
      <span className="text-xs text-[color:var(--color-muted)]">
        {t("settings.tagsManageHint")}
      </span>
    </div>
  );
}

// ─── Account sub-tab ─────────────────────────────────────────────────────────

function AccountSettings({ user }: { user: User }) {
  const { t } = useI18n();
  return (
    <div className="space-y-6 max-w-md">
      <section>
        <h2 className="text-base font-semibold tracking-tight mb-3">{t("settings.accountTitle")}</h2>
        <dl className="text-sm grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5">
          <dt className="text-[color:var(--color-muted)]">{t("settings.accountUsername")}</dt>
          <dd className="font-mono">{user.username}</dd>
          <dt className="text-[color:var(--color-muted)]">{t("settings.accountCreated")}</dt>
          <dd>{new Date(user.created_at).toLocaleString()}</dd>
        </dl>
      </section>
      <section>
        <h3 className="text-sm font-semibold tracking-tight mb-2">{t("settings.changePasswordTitle")}</h3>
        <ChangePasswordForm />
      </section>
    </div>
  );
}

function ChangePasswordForm() {
  const { t } = useI18n();
  const [current, setCurrent] = useState("");
  const [next, setNext]       = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy]       = useState(false);
  const [error, setError]     = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSuccess(false);
    if (next.length < 8) {
      setError(t("settings.newPasswordMin"));
      return;
    }
    if (next !== confirm) {
      setError(t("settings.newPasswordsMismatch"));
      return;
    }
    setBusy(true);
    try {
      await authApi.changePassword(current, next);
      setCurrent(""); setNext(""); setConfirm("");
      setSuccess(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="space-y-3">
      <Field label={t("settings.currentPassword")}>
        <FieldInput
          type="password"
          autoComplete="current-password"
          value={current}
          onChange={(e) => setCurrent(e.target.value)}
          required
        />
      </Field>
      <Field label={t("settings.newPassword")}>
        <FieldInput
          type="password"
          autoComplete="new-password"
          value={next}
          onChange={(e) => setNext(e.target.value)}
          minLength={8}
          required
        />
      </Field>
      <Field label={t("settings.confirmNewPassword")}>
        <FieldInput
          type="password"
          autoComplete="new-password"
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          minLength={8}
          required
        />
      </Field>
      {error && <ErrorText message={error} />}
      {success && (
        <p role="status" className="text-sm text-[color:var(--color-accent)]">
          {t("settings.passwordUpdated")}
        </p>
      )}
      <PrimaryButton disabled={busy}>
        {busy ? t("settings.updating") : t("settings.changePassword")}
      </PrimaryButton>
    </form>
  );
}

// ─── Retention sub-tab ───────────────────────────────────────────────────────

type DurationUnit = "s" | "m" | "h" | "d";

type DurationInput = {
  value: string;
  unit: DurationUnit;
};

const DURATION_UNITS: { value: DurationUnit; labelKey: "common.seconds" | "common.minutes" | "common.hours" | "common.days"; seconds: number }[] = [
  { value: "s", labelKey: "common.seconds", seconds: 1 },
  { value: "m", labelKey: "common.minutes", seconds: 60 },
  { value: "h", labelKey: "common.hours", seconds: 60 * 60 },
  { value: "d", labelKey: "common.days", seconds: 24 * 60 * 60 },
];

function parseDurationInput(duration: string, allowed?: DurationUnit[]): DurationInput {
  const match = duration.match(/^(\d+)(s|m|h)$/);
  if (!match) {
    return { value: duration, unit: "h" };
  }
  const amount = Number(match[1]);
  const canUseDay = !allowed || allowed.includes("d");
  if (match[2] === "h" && amount % 24 === 0 && canUseDay) {
    return { value: String(amount / 24), unit: "d" };
  }
  return { value: match[1], unit: match[2] as DurationUnit };
}

function formatDurationInput(input: DurationInput): string | null {
  if (!/^\d+$/.test(input.value)) {
    return null;
  }
  const amount = Number(input.value);
  if (!Number.isSafeInteger(amount) || amount <= 0) {
    return null;
  }
  if (input.unit === "d") {
    return `${amount * 24}h`;
  }
  return `${amount}${input.unit}`;
}

function DurationField({
  label,
  value,
  onChange,
  units,
  help,
}: {
  label: string;
  value: DurationInput;
  onChange: (value: DurationInput) => void;
  units?: DurationUnit[];
  help?: string;
}) {
  const { t } = useI18n();
  const allowed = units
    ? DURATION_UNITS.filter((u) => units.includes(u.value))
    : DURATION_UNITS;
  return (
    <Field label={label}>
      <div className="flex gap-2">
        <FieldInput
          type="number"
          min={1}
          step={1}
          inputMode="numeric"
          value={value.value}
          onChange={(e) => onChange({ ...value, value: e.target.value })}
          required
        />
        <select
          aria-label={t("common.unitAria", { label })}
          className="rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-[color:var(--color-accent)]"
          value={value.unit}
          onChange={(e) => onChange({ ...value, unit: e.target.value as DurationUnit })}
        >
          {allowed.map((unit) => (
            <option key={unit.value} value={unit.value}>{t(unit.labelKey)}</option>
          ))}
        </select>
      </div>
      {help && (
        <p className="mt-1 text-xs text-[color:var(--color-muted)]">{help}</p>
      )}
    </Field>
  );
}

function RuntimeSettings() {
  const { t } = useI18n();
  const [settings, setSettings] = useState<SettingsResponse | null>(null);
  const [agentInterval, setAgentInterval] = useState<DurationInput>({ value: "", unit: "s" });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedAt, setSavedAt] = useState<number | null>(null);

  useEffect(() => {
    settingsApi.get().then((s) => {
      setSettings(s);
      setAgentInterval(parseDurationInput(s.agent_interval));
    }).catch((err) => {
      setError(err instanceof ApiError ? err.message : String(err));
    });
  }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    const nextInterval = formatDurationInput(agentInterval);
    if (!nextInterval) {
      setError(t("settings.agentIntervalInvalid"));
      return;
    }
    setBusy(true);
    try {
      const next = await settingsApi.put({ agent_interval: nextInterval });
      setSettings(next);
      setAgentInterval(parseDurationInput(next.agent_interval));
      setSavedAt(Date.now());
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  const agentDuration = formatDurationInput(agentInterval);
  const dirty = !!settings && agentDuration !== settings.agent_interval;

  return (
    <div className="max-w-md space-y-4">
      <section>
        <h2 className="text-base font-semibold tracking-tight mb-3">{t("settings.runtimeTitle")}</h2>
        <p className="text-sm text-[color:var(--color-muted)]">
          {t("settings.runtimeDescription")}
        </p>
      </section>

      {!settings ? (
        <p className="text-sm text-[color:var(--color-muted)]">{t("common.loading")}</p>
      ) : (
        <form onSubmit={submit} className="space-y-3">
          <DurationField
            label={t("settings.agentCollectionInterval")}
            value={agentInterval}
            onChange={setAgentInterval}
          />
          {error && <ErrorText message={error} />}
          {savedAt && !dirty && (
            <p role="status" className="text-sm text-[color:var(--color-accent)]">
              {t("common.savedAt", { time: new Date(savedAt).toLocaleTimeString() })}.
            </p>
          )}
          <PrimaryButton disabled={busy || !dirty}>
            {busy ? t("common.saving") : t("common.save")}
          </PrimaryButton>
        </form>
      )}
    </div>
  );
}

function DownsampleSettings() {
  const { t } = useI18n();
  const [settings, setSettings] = useState<SettingsResponse | null>(null);
  const [bucketSize, setBucketSize] = useState<DurationInput>({ value: "", unit: "m" });
  const [hotWindow, setHotWindow] = useState<DurationInput>({ value: "", unit: "h" });
  const [archiveWindow, setArchiveWindow] = useState<DurationInput>({ value: "", unit: "d" });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedAt, setSavedAt] = useState<number | null>(null);

  useEffect(() => {
    settingsApi.get().then((s) => {
      setSettings(s);
      setBucketSize(parseDurationInput(s.downsample_bucket_size));
      setHotWindow(parseDurationInput(s.downsample_hot_window));
      setArchiveWindow(parseDurationInput(s.downsample_archive_window));
    }).catch((err) => {
      setError(err instanceof ApiError ? err.message : String(err));
    });
  }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    const nextBucketSize = formatDurationInput(bucketSize);
    const nextHotWindow = formatDurationInput(hotWindow);
    const nextArchiveWindow = formatDurationInput(archiveWindow);
    if (!nextBucketSize || !nextHotWindow || !nextArchiveWindow) {
      setError(t("settings.downsampleInvalid"));
      return;
    }
    setBusy(true);
    try {
      const next = await settingsApi.put({
        downsample_bucket_size: nextBucketSize,
        downsample_hot_window: nextHotWindow,
        downsample_archive_window: nextArchiveWindow,
      });
      setSettings(next);
      setBucketSize(parseDurationInput(next.downsample_bucket_size));
      setHotWindow(parseDurationInput(next.downsample_hot_window));
      setArchiveWindow(parseDurationInput(next.downsample_archive_window));
      setSavedAt(Date.now());
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  const bucketDuration = formatDurationInput(bucketSize);
  const hotDuration = formatDurationInput(hotWindow);
  const archiveDuration = formatDurationInput(archiveWindow);
  const dirty =
    !!settings &&
    (bucketDuration !== settings.downsample_bucket_size ||
      hotDuration !== settings.downsample_hot_window ||
      archiveDuration !== settings.downsample_archive_window);

  return (
    <div className="max-w-md space-y-4">
      <section>
        <div className="mb-3 flex items-center gap-2 flex-wrap">
          <h2 className="text-base font-semibold tracking-tight">{t("settings.downsampleTitle")}</h2>
          <span className="inline-flex items-center rounded-full border border-[color:var(--lumen-teal)]/40 bg-[color:var(--lumen-teal)]/10 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-[color:var(--lumen-teal)]">
            {t("settings.statusBadgeRoadmap")}
          </span>
        </div>
        <p className="text-sm text-[color:var(--color-muted)]">
          {t("settings.downsampleDescription")}
        </p>
        <ul className="mt-3 space-y-1.5 text-sm text-[color:var(--color-muted)]">
          <li><strong className="text-[color:var(--color-fg)]">{t("settings.bucketSize")}</strong>: {t("settings.downsampleBucketHelp")}</li>
          <li><strong className="text-[color:var(--color-fg)]">{t("settings.hotWindow")}</strong>: {t("settings.downsampleHotHelp")}</li>
          <li><strong className="text-[color:var(--color-fg)]">{t("settings.archiveWindow")}</strong>: {t("settings.downsampleArchiveHelp")}</li>
        </ul>
      </section>

      {!settings ? (
        <p className="text-sm text-[color:var(--color-muted)]">{t("common.loading")}</p>
      ) : (
        <form onSubmit={submit} className="space-y-3">
          <DurationField
            label={t("settings.bucketSize")}
            value={bucketSize}
            onChange={setBucketSize}
          />
          <DurationField
            label={t("settings.hotWindow")}
            value={hotWindow}
            onChange={setHotWindow}
          />
          <DurationField
            label={t("settings.archiveWindow")}
            value={archiveWindow}
            onChange={setArchiveWindow}
          />
          {error && <ErrorText message={error} />}
          {savedAt && !dirty && (
            <p role="status" className="text-sm text-[color:var(--color-accent)]">
              {t("common.savedAt", { time: new Date(savedAt).toLocaleTimeString() })}.
            </p>
          )}
          <PrimaryButton disabled={busy || !dirty}>
            {busy ? t("common.saving") : t("common.save")}
          </PrimaryButton>
        </form>
      )}
    </div>
  );
}

function SettingsPanel({
  title,
  description,
  children,
  roadmap,
}: {
  title: string;
  description: string;
  children?: React.ReactNode;
  roadmap?: boolean;
}) {
  const { t } = useI18n();
  return (
    <Surface as="section">
      <div className="flex items-center gap-2 flex-wrap">
        <h2 className="text-base font-semibold tracking-tight">{title}</h2>
        {roadmap && (
          <span className="inline-flex items-center rounded-full border border-[color:var(--lumen-teal)]/40 bg-[color:var(--lumen-teal)]/10 px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-[color:var(--lumen-teal)]">
            {t("settings.statusBadgeRoadmap")}
          </span>
        )}
      </div>
      <p className="mt-2 text-sm text-[color:var(--color-muted)]">{description}</p>
      {children && <div className="mt-4">{children}</div>}
    </Surface>
  );
}

function LogManagementSettings() {
  const { t } = useI18n();
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <SettingsPanel
        title={t("settings.logsTitle")}
        description={t("settings.logsDescription")}
        roadmap
      >
        <ul className="space-y-2 text-sm text-[color:var(--color-muted)]">
          <li><strong className="text-[color:var(--color-fg)]">{t("common.sources")}:</strong> {t("settings.logsSources")}</li>
          <li><strong className="text-[color:var(--color-fg)]">{t("common.limits")}:</strong> {t("settings.logsLimits")}</li>
          <li><strong className="text-[color:var(--color-fg)]">{t("common.storage")}:</strong> {t("settings.logsStorage")}</li>
        </ul>
      </SettingsPanel>
      <SettingsPanel
        title={t("settings.notLoki")}
        description={t("settings.notLokiDescription")}
      >
        <div className="rounded-xl border border-[color:var(--color-border)] bg-[color:var(--color-bg)] p-4 font-mono text-xs text-[color:var(--color-muted)]">
          host=pve-01 source=docker target=nginx tail=500
        </div>
      </SettingsPanel>
    </div>
  );
}

function RetentionSettings() {
  const { t } = useI18n();
  const [settings, setSettings]       = useState<SettingsResponse | null>(null);
  const [window, setWindow]           = useState<DurationInput>({ value: "", unit: "h" });
  const [interval, setInterval]       = useState<DurationInput>({ value: "", unit: "h" });
  const [alertsWindow, setAlertsWindow] = useState<DurationInput>({ value: "", unit: "h" });
  const [busy, setBusy]               = useState(false);
  const [error, setError]             = useState<string | null>(null);
  const [savedAt, setSavedAt]         = useState<number | null>(null);

  useEffect(() => {
    settingsApi.get().then((s) => {
      setSettings(s);
      setWindow(parseDurationInput(s.retention_window));
      setInterval(parseDurationInput(s.retention_interval, ["m", "h"]));
      setAlertsWindow(parseDurationInput(s.retention_alerts_window));
    }).catch((err) => {
      setError(err instanceof ApiError ? err.message : String(err));
    });
  }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    const retentionWindow = formatDurationInput(window);
    const retentionInterval = formatDurationInput(interval);
    const retentionAlertsWindow = formatDurationInput(alertsWindow);
    if (!retentionWindow || !retentionInterval || !retentionAlertsWindow) {
      setError(t("settings.retentionInvalid"));
      return;
    }
    setBusy(true);
    try {
      const next = await settingsApi.put({
        retention_window:        retentionWindow,
        retention_interval:      retentionInterval,
        retention_alerts_window: retentionAlertsWindow,
      });
      setSettings(next);
      setWindow(parseDurationInput(next.retention_window));
      setInterval(parseDurationInput(next.retention_interval, ["m", "h"]));
      setAlertsWindow(parseDurationInput(next.retention_alerts_window));
      setSavedAt(Date.now());
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  const windowDuration = formatDurationInput(window);
  const intervalDuration = formatDurationInput(interval);
  const alertsWindowDuration = formatDurationInput(alertsWindow);
  const dirty =
    !!settings &&
    (windowDuration !== settings.retention_window
      || intervalDuration !== settings.retention_interval
      || alertsWindowDuration !== settings.retention_alerts_window);

  return (
    <div className="max-w-md space-y-4">
      <section>
        <h2 className="text-base font-semibold tracking-tight mb-3">{t("settings.retentionTitle")}</h2>
        <p className="text-sm text-[color:var(--color-muted)]">
          {t("settings.retentionDescription")}
        </p>
      </section>

      {!settings ? (
        <p className="text-sm text-[color:var(--color-muted)]">{t("common.loading")}</p>
      ) : (
        <form onSubmit={submit} className="space-y-4">
          <DurationField
            label={t("settings.retentionWindowLabel")}
            value={window}
            onChange={setWindow}
            help={t("settings.retentionWindowHelp")}
          />
          <DurationField
            label={t("settings.retentionIntervalLabel")}
            value={interval}
            onChange={setInterval}
            units={["m", "h"]}
            help={t("settings.retentionIntervalHelp")}
          />
          <DurationField
            label={t("settings.retentionAlertsWindowLabel")}
            value={alertsWindow}
            onChange={setAlertsWindow}
            help={t("settings.retentionAlertsWindowHelp")}
          />
          {error && <ErrorText message={error} />}
          {savedAt && !dirty && (
            <p role="status" className="text-sm text-[color:var(--color-accent)]">
              {t("common.savedAt", { time: new Date(savedAt).toLocaleTimeString() })}.
            </p>
          )}
          <PrimaryButton disabled={busy || !dirty}>
            {busy ? t("common.saving") : t("common.save")}
          </PrimaryButton>
        </form>
      )}
    </div>
  );
}

// ─── Hub status sub-tab ───────────────────────────────────────────────────

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return "—";
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let v = bytes / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 100 ? 0 : v >= 10 ? 1 : 2)} ${units[i]}`;
}

function formatUptime(seconds: number): string {
  if (seconds < 60) return `${seconds}s`;
  const m = Math.floor(seconds / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ${m % 60}m`;
  const d = Math.floor(h / 24);
  return `${d}d ${h % 24}h`;
}

function HubStatusSettings() {
  const { t } = useI18n();
  const [data, setData] = useState<HubStatsResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const fetchOnce = () => {
      hubStatsApi.get()
        .then((s) => { if (!cancelled) { setData(s); setError(null); } })
        .catch((err) => {
          if (cancelled) return;
          setError(err instanceof ApiError ? err.message : String(err));
        });
    };
    fetchOnce();
    const id = window.setInterval(fetchOnce, 30_000);
    return () => { cancelled = true; window.clearInterval(id); };
  }, []);

  if (error && !data) {
    return (
      <div className="max-w-md space-y-4">
        <h2 className="text-base font-semibold tracking-tight">{t("settings.hubStatusTitle")}</h2>
        <ErrorText message={error} />
      </div>
    );
  }
  if (!data) {
    return (
      <div className="max-w-md space-y-4">
        <h2 className="text-base font-semibold tracking-tight">{t("settings.hubStatusTitle")}</h2>
        <p className="text-sm text-[color:var(--color-muted)]">{t("common.loading")}</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <section>
        <h2 className="text-base font-semibold tracking-tight">{t("settings.hubStatusTitle")}</h2>
        <p className="mt-1 text-sm text-[color:var(--color-muted)]">{t("settings.hubStatusDescription")}</p>
      </section>

      <div className="grid gap-4 lg:grid-cols-2">
        <SettingsPanel
          title={t("settings.hubStatusHubTitle")}
          description={t("settings.hubStatusHubDescription")}
        >
          <StatRow label={t("settings.hubStatusVersion")} value={data.version || "—"} />
          <StatRow label={t("settings.hubStatusUptime")} value={formatUptime(data.uptime_seconds)} />
          <StatRow
            label={t("settings.hubStatusStartedAt")}
            value={new Date(data.started_at).toLocaleString()}
          />
          <StatRow label={t("settings.hubStatusGoVersion")} value={data.runtime.go_version} />
          <StatRow label={t("settings.hubStatusGoroutines")} value={data.runtime.goroutines.toLocaleString()} />
          <StatRow label={t("settings.hubStatusHeap")} value={formatBytes(data.runtime.heap_alloc_bytes)} />
          <StatRow label={t("settings.hubStatusNumGC")} value={data.runtime.num_gc.toLocaleString()} />
        </SettingsPanel>

        <SettingsPanel
          title={t("settings.hubStatusStorageTitle")}
          description={t("settings.hubStatusStorageDescription")}
        >
          <StatRow label={t("settings.hubStatusDBPath")} value={data.storage.db_path} mono />
          <StatRow label={t("settings.hubStatusDBSize")} value={formatBytes(data.storage.db_size_bytes)} />
          {data.storage.wal_size_bytes > 0 && (
            <StatRow label={t("settings.hubStatusWALSize")} value={formatBytes(data.storage.wal_size_bytes)} />
          )}
          <div className="mt-3 border-t border-[color:var(--color-border)] pt-3">
            <p className="mb-2 text-xs uppercase tracking-wide text-[color:var(--color-muted)]">
              {t("settings.hubStatusRowsLabel")}
            </p>
            {Object.entries(data.storage.rows).map(([table, count]) => (
              <StatRow key={table} label={table} value={count.toLocaleString()} mono />
            ))}
          </div>
        </SettingsPanel>

        <SettingsPanel
          title={t("settings.hubStatusAgentsTitle")}
          description={t("settings.hubStatusAgentsDescription")}
        >
          <StatRow
            label={t("settings.hubStatusAgentsConnected")}
            value={`${data.agents.connected.toLocaleString()} / ${data.agents.registered.toLocaleString()}`}
          />
        </SettingsPanel>

        <SettingsPanel
          title={t("settings.hubStatusDeliveriesTitle")}
          description={t("settings.hubStatusDeliveriesDescription")}
        >
          <StatRow label={t("settings.hubStatusDeliveriesPending")} value={data.deliveries.pending.toLocaleString()} />
          <StatRow label={t("settings.hubStatusDeliveriesInflight")} value={data.deliveries.inflight.toLocaleString()} />
        </SettingsPanel>
      </div>
    </div>
  );
}

function StatRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="flex items-baseline justify-between gap-3 py-1 text-sm">
      <span className="text-[color:var(--color-muted)]">{label}</span>
      <span className={mono ? "font-mono text-xs text-[color:var(--color-fg)] break-all text-right" : "text-[color:var(--color-fg)]"}>
        {value}
      </span>
    </div>
  );
}

// ─── API Keys sub-tab ─────────────────────────────────────────────────────

const ALL_SCOPES: ApiKeyScope[] = ["read:hosts", "read:metrics", "read:alerts"];
const DEFAULT_SCOPES: ApiKeyScope[] = ["read:hosts", "read:metrics"];

function ApiKeysSettings() {
  const { t, locale } = useI18n();
  const confirm = useConfirm();
  const [keys, setKeys] = useState<ApiKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [name, setName] = useState("");
  const [scopes, setScopes] = useState<ApiKeyScope[]>(DEFAULT_SCOPES);
  const [hostFilter, setHostFilter] = useState("");
  const [busy, setBusy] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [revealed, setRevealed] = useState<ApiKeyCreated | null>(null);
  const [copied, setCopied] = useState(false);
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    apiKeysApi.list()
      .then((rows) => setKeys(rows ?? []))
      .catch((err) => setLoadError(err instanceof ApiError ? err.message : String(err)))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 30_000);
    return () => window.clearInterval(id);
  }, []);

  function toggleScope(scope: ApiKeyScope) {
    setScopes((prev) => prev.includes(scope) ? prev.filter((s) => s !== scope) : [...prev, scope]);
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setFormError(null);
    if (!name.trim()) {
      setFormError(t("settings.apikeysNameRequired"));
      return;
    }
    if (scopes.length === 0) {
      setFormError(t("settings.apikeysScopesRequired"));
      return;
    }
    setBusy(true);
    try {
      const created = await apiKeysApi.create({
        name: name.trim(),
        scopes,
        host_filter: hostFilter.trim() ? hostFilter.trim() : null,
      });
      setKeys((prev) => [created, ...prev]);
      setRevealed(created);
      setCopied(false);
      setName("");
      setScopes(DEFAULT_SCOPES);
      setHostFilter("");
    } catch (err) {
      setFormError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function remove(key: ApiKey) {
    const ok = await confirm({
      title: t("settings.apikeysRevokeTitle"),
      message: t("settings.apikeysRevokeMessage", { name: key.name }),
      confirmLabel: t("settings.apikeysRevokeConfirm"),
      destructive: true,
    });
    if (!ok) return;
    try {
      await apiKeysApi.remove(key.id);
      setKeys((prev) => prev.filter((k) => k.id !== key.id));
      if (revealed?.id === key.id) setRevealed(null);
    } catch (err) {
      setLoadError(err instanceof ApiError ? err.message : String(err));
    }
  }

  async function copyPlaintext() {
    if (!revealed) return;
    const ok = await copyToClipboard(revealed.plaintext);
    if (ok) {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  }

  return (
    <div className="space-y-6">
      <header>
        <h2 className="text-base font-semibold tracking-tight">{t("settings.apikeysTitle")}</h2>
        <p className="mt-1 text-sm text-[color:var(--color-muted)]">{t("settings.apikeysDescription")}</p>
      </header>

      {revealed && (
        <Surface as="section" className="border-[color:var(--lumen-teal)]/40 bg-[color-mix(in_oklch,var(--lumen-teal)_8%,transparent)]">
          <div className="flex items-start justify-between gap-3">
            <div>
              <h3 className="text-sm font-semibold">{t("settings.apikeysRevealTitle")}</h3>
              <p className="mt-1 text-xs text-[color:var(--color-muted)]">{t("settings.apikeysRevealHelp")}</p>
            </div>
            <button
              type="button"
              onClick={() => setRevealed(null)}
              className="text-xs text-[color:var(--color-muted)] hover:text-[color:var(--color-fg)]"
            >
              {t("common.dismiss")}
            </button>
          </div>
          <div className="mt-3 flex items-center gap-2">
            <code className="flex-1 break-all rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-3 py-2 font-mono text-xs">
              {revealed.plaintext}
            </code>
            <GhostButton type="button" onClick={copyPlaintext}>
              {copied ? <><Check size={14} /> {t("common.copied")}</> : <><Copy size={14} /> {t("common.copy")}</>}
            </GhostButton>
          </div>
        </Surface>
      )}

      <Surface as="section">
        <h3 className="text-sm font-semibold tracking-tight">{t("settings.apikeysCreateTitle")}</h3>
        <form onSubmit={submit} className="mt-3 space-y-3">
          <Field label={t("settings.apikeysNameLabel")}>
            <FieldInput
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t("settings.apikeysNamePlaceholder")}
              maxLength={64}
              required
            />
          </Field>
          <Field label={t("settings.apikeysScopesLabel")}>
            <div className="flex flex-wrap gap-3">
              {ALL_SCOPES.map((scope) => (
                <label key={scope} className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={scopes.includes(scope)}
                    onChange={() => toggleScope(scope)}
                  />
                  <span className="font-mono text-xs">{scope}</span>
                </label>
              ))}
            </div>
          </Field>
          <Field label={t("settings.apikeysHostFilterLabel")}>
            <FieldInput
              value={hostFilter}
              onChange={(e) => setHostFilter(e.target.value)}
              placeholder={t("settings.apikeysHostFilterPlaceholder")}
              maxLength={256}
            />
            <p className="mt-1 text-xs text-[color:var(--color-muted)]">{t("settings.apikeysHostFilterHelp")}</p>
          </Field>
          {formError && <ErrorText message={formError} />}
          <PrimaryButton disabled={busy}>
            {busy ? t("common.saving") : t("settings.apikeysCreateSubmit")}
          </PrimaryButton>
        </form>
      </Surface>

      <Surface as="section">
        <h3 className="text-sm font-semibold tracking-tight">{t("settings.apikeysListTitle")}</h3>
        {loadError && <div className="mt-2"><ErrorText message={loadError} /></div>}
        {loading ? (
          <p className="mt-3 text-sm text-[color:var(--color-muted)]">{t("common.loading")}</p>
        ) : keys.length === 0 ? (
          <p className="mt-3 text-sm text-[color:var(--color-muted)]">{t("settings.apikeysEmpty")}</p>
        ) : (
          <ul className="mt-3 divide-y divide-[color:var(--color-border)]">
            {keys.map((k) => (
              <li key={k.id} className="flex items-start justify-between gap-3 py-3">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="font-medium text-[color:var(--color-fg)]">{k.name}</span>
                    <code className="font-mono text-xs text-[color:var(--color-muted)]">{k.preview}…</code>
                  </div>
                  <div className="mt-1 flex flex-wrap gap-1.5">
                    {k.scopes.map((s) => (
                      <span key={s} className="rounded-md border border-[color:var(--color-border)] px-1.5 py-0.5 font-mono text-[10px] text-[color:var(--color-muted)]">
                        {s}
                      </span>
                    ))}
                    {k.host_filter && (
                      <span className="rounded-md border border-[color:var(--color-border)] px-1.5 py-0.5 font-mono text-[10px] text-[color:var(--color-muted)]">
                        host:{k.host_filter}
                      </span>
                    )}
                  </div>
                  <p className="mt-1 text-xs text-[color:var(--color-muted)]">
                    {t("settings.apikeysLastUsed")}: {k.last_used_at ? relativeTime(k.last_used_at, now, locale) : t("common.never")} ·{" "}
                    {t("settings.apikeysCreatedAt")}: {relativeTime(k.created_at, now, locale)}
                  </p>
                </div>
                <IconButton
                  label={t("settings.apikeysRevokeAria", { name: k.name })}
                  onClick={() => remove(k)}
                >
                  <Trash2 size={14} />
                </IconButton>
              </li>
            ))}
          </ul>
        )}
      </Surface>
    </div>
  );
}

// ─── Display sub-tab (theme / language / units / reduce-motion) ──────────

function DisplaySettings() {
  const { t } = useI18n();
  const { display, ready, updateDisplay } = usePrefs();
  const [error, setError] = useState<string | null>(null);

  async function set<K extends keyof typeof display>(key: K, value: (typeof display)[K]) {
    setError(null);
    try {
      await updateDisplay({ ...display, [key]: value });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    }
  }

  return (
    <div className="space-y-6 max-w-xl">
      <header>
        <h2 className="text-base font-semibold tracking-tight">{t("settings.displayTitle")}</h2>
        <p className="mt-1 text-sm text-[color:var(--color-muted)]">{t("settings.displayDescription")}</p>
      </header>

      {!ready ? (
        <p className="text-sm text-[color:var(--color-muted)]">{t("common.loading")}</p>
      ) : (
        <div className="space-y-5">
          <div className="space-y-1.5">
            <label className="block text-sm font-medium">{t("settings.displayThemeLabel")}</label>
            <SegmentedControl<Theme>
              ariaLabel={t("settings.displayThemeLabel")}
              value={display.theme}
              onChange={(v) => set("theme", v)}
              options={[
                { value: "system", label: t("settings.displayThemeSystem") },
                { value: "light",  label: t("settings.displayThemeLight") },
                { value: "dark",   label: t("settings.displayThemeDark") },
              ]}
            />
          </div>

          <div className="space-y-1.5">
            <label className="block text-sm font-medium">{t("settings.displayLanguageLabel")}</label>
            <SegmentedControl<"en" | "vi">
              ariaLabel={t("settings.displayLanguageLabel")}
              value={display.language}
              onChange={(v) => set("language", v)}
              options={[
                { value: "en", label: "EN" },
                { value: "vi", label: "VI" },
              ]}
            />
          </div>

          <div className="space-y-1.5">
            <label className="block text-sm font-medium">{t("settings.displayUnitsLabel")}</label>
            <SegmentedControl<UnitsMode>
              ariaLabel={t("settings.displayUnitsLabel")}
              value={display.units}
              onChange={(v) => set("units", v)}
              options={[
                { value: "auto",    label: t("settings.displayUnitsAuto") },
                { value: "binary",  label: t("settings.displayUnitsBinary") },
                { value: "decimal", label: t("settings.displayUnitsDecimal") },
              ]}
            />
            <p className="text-xs text-[color:var(--color-muted)]">{t("settings.displayUnitsHelp")}</p>
          </div>

          <div className="space-y-1.5">
            <label className="block text-sm font-medium">{t("settings.displayReduceMotionLabel")}</label>
            <SegmentedControl<ReduceMotion>
              ariaLabel={t("settings.displayReduceMotionLabel")}
              value={display.reduceMotion}
              onChange={(v) => set("reduceMotion", v)}
              options={[
                { value: "system", label: t("settings.displayReduceMotionSystem") },
                { value: "on",     label: t("settings.displayReduceMotionOn") },
                { value: "off",    label: t("settings.displayReduceMotionOff") },
              ]}
            />
            <p className="text-xs text-[color:var(--color-muted)]">{t("settings.displayReduceMotionHelp")}</p>
          </div>

          <div className="space-y-1.5">
            <label className="block text-sm font-medium">{t("settings.displayDensityLabel")}</label>
            <SegmentedControl<Density>
              ariaLabel={t("settings.displayDensityLabel")}
              value={display.density}
              onChange={(v) => set("density", v)}
              options={[
                { value: "comfortable", label: t("settings.displayDensityComfortable") },
                { value: "compact",     label: t("settings.displayDensityCompact") },
              ]}
            />
            <p className="text-xs text-[color:var(--color-muted)]">{t("settings.displayDensityHelp")}</p>
          </div>

          {error && <ErrorText message={error} />}

          <p className="text-xs text-[color:var(--color-muted)]">{t("settings.displayDashboardHint")}</p>
        </div>
      )}
    </div>
  );
}

// SSOSettings — single-admin OIDC config. Labels are inline English
// because the surface is admin-only + rarely edited; a future PR can
// promote the strings to i18n.messages if a VI-speaking operator hits it.
function SSOSettings() {
  const [cfg, setCfg] = useState<OIDCSettings | null>(null);
  const [form, setForm] = useState<OIDCSettings>({
    enabled: false, issuer: "", client_id: "", client_secret: "",
    has_client_secret: false, scopes: "openid email profile", expected_email: "",
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedAt, setSavedAt] = useState<number | null>(null);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ ok: boolean; msg: string } | null>(null);

  useEffect(() => {
    oidcApi.get().then((c) => {
      setCfg(c);
      setForm({ ...c, client_secret: "" });
    }).catch((err) => setError(err instanceof ApiError ? err.message : String(err)));
  }, []);

  async function testDiscovery() {
    setTesting(true);
    setTestResult(null);
    try {
      const res = await oidcApi.testDiscovery(form.issuer);
      setTestResult({ ok: res.ok, msg: res.ok ? "Issuer reachable; .well-known/openid-configuration OK" : (res.error ?? "Discovery failed") });
    } catch (err) {
      setTestResult({ ok: false, msg: err instanceof ApiError ? err.message : String(err) });
    } finally {
      setTesting(false);
    }
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      const next = await oidcApi.put({
        enabled: form.enabled,
        issuer: form.issuer,
        client_id: form.client_id,
        client_secret: form.client_secret || undefined,
        scopes: form.scopes,
        expected_email: form.expected_email,
      });
      setCfg(next);
      setForm({ ...next, client_secret: "" });
      setSavedAt(Date.now());
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  if (!cfg) {
    return <p className="text-sm text-[color:var(--color-muted)]">Loading…</p>;
  }

  return (
    <div className="max-w-2xl space-y-4">
      <section>
        <h2 className="text-base font-semibold tracking-tight mb-3">Single sign-on (OIDC)</h2>
        <p className="text-sm text-[color:var(--color-muted)]">
          Bind a self-hosted IdP (Authentik, Keycloak, Google, etc.) so you sign in via your OIDC provider
          instead of the local password. Single-admin mode: only the email below can sign in via OIDC, and
          the existing local password keeps working as a fallback.
        </p>
        <p className="mt-2 text-xs text-[color:var(--color-muted)]">
          Callback URL to register with your IdP: <code className="text-[color:var(--color-fg)]">{new URL("/api/auth/oidc/callback", window.location.href).toString()}</code>
        </p>
      </section>

      <form onSubmit={submit} className="space-y-3">
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} />
          Enable OIDC login
        </label>

        <Field label="Issuer URL">
          <FieldInput
            value={form.issuer}
            placeholder="https://authentik.example.com/application/o/lumen/"
            onChange={(e) => setForm({ ...form, issuer: e.target.value })}
          />
        </Field>

        <Field label="Client ID">
          <FieldInput value={form.client_id} onChange={(e) => setForm({ ...form, client_id: e.target.value })} />
        </Field>

        <Field label={cfg.has_client_secret ? "Client secret (leave blank to keep saved)" : "Client secret"}>
          <FieldInput
            type="password"
            value={form.client_secret ?? ""}
            placeholder={cfg.has_client_secret ? "•••••• (saved)" : ""}
            onChange={(e) => setForm({ ...form, client_secret: e.target.value })}
          />
        </Field>

        <Field label="Scopes">
          <FieldInput value={form.scopes} onChange={(e) => setForm({ ...form, scopes: e.target.value })} />
        </Field>

        <Field label="Expected admin email">
          <FieldInput
            value={form.expected_email}
            placeholder="you@example.com"
            onChange={(e) => setForm({ ...form, expected_email: e.target.value })}
          />
          <p className="mt-1 text-xs text-[color:var(--color-muted)]">
            Only this email (from the ID token's <code>email</code> claim) can sign in via OIDC. Any other identity is rejected.
          </p>
        </Field>

        <div className="flex items-center gap-2">
          <GhostButton type="button" disabled={!form.issuer || testing} onClick={testDiscovery}>
            {testing ? "Testing…" : "Test discovery"}
          </GhostButton>
          {testResult && (
            <span className={`text-sm ${testResult.ok ? "text-[color:var(--color-accent)]" : "text-red-500"}`}>
              {testResult.msg}
            </span>
          )}
        </div>

        {error && <ErrorText message={error} />}
        {savedAt && (
          <p role="status" className="text-sm text-[color:var(--color-accent)]">
            Saved at {new Date(savedAt).toLocaleTimeString()}.
          </p>
        )}
        <PrimaryButton disabled={busy}>{busy ? "Saving…" : "Save"}</PrimaryButton>
      </form>
    </div>
  );
}

// StatusPageSettings — admin config for the unauthenticated /status page.
// Labels are inline English (same rationale as SSOSettings).
function StatusPageSettings() {
  const [cfg, setCfg] = useState<PublicStatusConfig | null>(null);
  const [hosts, setHosts] = useState<Host[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedAt, setSavedAt] = useState<number | null>(null);

  useEffect(() => {
    publicStatusApi.getConfig().then(setCfg).catch((e) => setError(String(e)));
    hostsApi.list().then(setHosts).catch((e) => setError(String(e)));
  }, []);

  async function submitConfig(e: React.FormEvent) {
    e.preventDefault();
    if (!cfg) return;
    setError(null);
    setBusy(true);
    try {
      const next = await publicStatusApi.putConfig(cfg);
      setCfg(next);
      setSavedAt(Date.now());
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function togglePublic(host: Host, next: boolean) {
    if (!hosts) return;
    setError(null);
    try {
      await hostsApi.setPublicVisible(host.id, next);
      setHosts(hosts.map((h) => (h.id === host.id ? { ...h, public_visible: next } : h)));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    }
  }

  if (!cfg) {
    return <p className="text-sm text-[color:var(--color-muted)]">Loading…</p>;
  }

  const visibleCount = hosts?.filter((h) => h.public_visible).length ?? 0;

  return (
    <div className="max-w-2xl space-y-6">
      <section>
        <h2 className="text-base font-semibold tracking-tight mb-3">Public status page</h2>
        <p className="text-sm text-[color:var(--color-muted)]">
          A read-only page at{" "}
          <a className="underline" href="/status" target="_blank" rel="noreferrer">
            {new URL("/status", window.location.href).toString()}
          </a>{" "}
          that anyone can visit (no login). Shows the hosts you opt in below, with up/stale/down state plus CPU/RAM/disk.
          Default: hidden until you flip the toggle and tick at least one host.
        </p>
      </section>

      <form onSubmit={submitConfig} className="space-y-3">
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={cfg.enabled}
            onChange={(e) => setCfg({ ...cfg, enabled: e.target.checked })}
          />
          Publish the status page
        </label>

        <Field label="Title">
          <FieldInput value={cfg.title} onChange={(e) => setCfg({ ...cfg, title: e.target.value })} placeholder="Status" />
        </Field>

        <Field label="Description">
          <FieldInput value={cfg.description} onChange={(e) => setCfg({ ...cfg, description: e.target.value })} placeholder="Optional — shown under the title." />
        </Field>

        {error && <ErrorText message={error} />}
        {savedAt && (
          <p role="status" className="text-sm text-[color:var(--color-accent)]">
            Saved at {new Date(savedAt).toLocaleTimeString()}.
          </p>
        )}
        <PrimaryButton disabled={busy}>{busy ? "Saving…" : "Save"}</PrimaryButton>
      </form>

      <section>
        <h3 className="text-sm font-semibold tracking-tight mb-2">Hosts on the public page ({visibleCount})</h3>
        {!hosts ? (
          <p className="text-sm text-[color:var(--color-muted)]">Loading hosts…</p>
        ) : hosts.length === 0 ? (
          <p className="text-sm text-[color:var(--color-muted)]">No hosts yet — create one in the Hosts tab.</p>
        ) : (
          <ul className="space-y-1.5">
            {hosts.map((h) => (
              <li key={h.id} className="flex items-center justify-between rounded-md border border-[color:var(--color-border)] px-3 py-2 text-sm">
                <span>{h.name}</span>
                <label className="flex items-center gap-2 text-xs text-[color:var(--color-muted)]">
                  <input
                    type="checkbox"
                    checked={h.public_visible}
                    onChange={(e) => togglePublic(h, e.target.checked)}
                  />
                  Show on /status
                </label>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

// ─── Backup sub-tab (RFC 0001) ──────────────────────────────────────────────

function BackupSettings() {
  const [cfg, setCfg] = useState<BackupSettings | null>(null);
  const [entries, setEntries] = useState<BackupEntry[] | null>(null);
  const [busy, setBusy] = useState<"test" | "run" | "save" | "pass" | "restore" | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [info, setInfo] = useState<string | null>(null);

  // Editable local copies
  const [enabled, setEnabled] = useState(false);
  const [target, setTarget] = useState<"local" | "s3">("local");
  const [localPath, setLocalPath] = useState("");
  const [s3Endpoint, setS3Endpoint] = useState("");
  const [s3Region, setS3Region] = useState("auto");
  const [s3Bucket, setS3Bucket] = useState("");
  const [s3Prefix, setS3Prefix] = useState("lumen/");
  const [s3AccessKey, setS3AccessKey] = useState("");
  const [s3SecretKey, setS3SecretKey] = useState("");
  const [s3ForcePathStyle, setS3ForcePathStyle] = useState(false);
  const [passphrase, setPassphrase] = useState("");
  const [cron, setCron] = useState("0 2 * * *");
  const [retainLast, setRetainLast] = useState(14);

  const refresh = useCallback(async () => {
    setError(null);
    try {
      const [c, l] = await Promise.all([backupApi.get(), backupApi.list()]);
      setCfg(c);
      setEnabled(c.enabled);
      setTarget(c.target);
      setLocalPath(c.local_path);
      setS3Endpoint(c.s3_endpoint);
      setS3Region(c.s3_region || "auto");
      setS3Bucket(c.s3_bucket);
      setS3Prefix(c.s3_prefix || "lumen/");
      setS3AccessKey(c.s3_access_key);
      setS3ForcePathStyle(c.s3_force_path_style);
      setCron(c.cron || "0 2 * * *");
      setRetainLast(c.retain_last || 14);
      setEntries(l.entries);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    }
  }, []);

  useEffect(() => { void refresh(); }, [refresh]);

  async function saveConfig() {
    setError(null); setInfo(null);
    setBusy("save");
    try {
      const body: Partial<BackupSettings> = {
        enabled, target,
        local_path: localPath,
        s3_endpoint: s3Endpoint, s3_region: s3Region, s3_bucket: s3Bucket,
        s3_prefix: s3Prefix, s3_access_key: s3AccessKey,
        s3_force_path_style: s3ForcePathStyle,
        cron, retain_last: retainLast,
      };
      if (s3SecretKey) body.s3_secret_key = s3SecretKey;
      await backupApi.put(body);
      setS3SecretKey("");
      setInfo("Saved.");
      await refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally { setBusy(null); }
  }

  async function savePassphrase() {
    if (!passphrase) {
      setError("Passphrase is required."); return;
    }
    setError(null); setInfo(null);
    setBusy("pass");
    try {
      await backupApi.setPassphrase(passphrase);
      setPassphrase("");
      setInfo("Passphrase saved. Remember: it is not stored — losing it means every backup is unrecoverable.");
      await refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally { setBusy(null); }
  }

  async function testTarget() {
    setError(null); setInfo(null);
    setBusy("test");
    try {
      const res = await backupApi.test();
      setInfo(res.ok ? "Target reachable." : (res.error ?? "Target probe failed."));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally { setBusy(null); }
  }

  async function runNow() {
    if (!cfg?.has_passphrase) {
      setError("Save a passphrase first."); return;
    }
    setError(null); setInfo(null);
    setBusy("run");
    try {
      const typed = window.prompt("Passphrase") ?? "";
      if (!typed) { setBusy(null); return; }
      const res = await backupApi.runNow(typed);
      setInfo(`Backup ${res.name} created (${(res.size_bytes/1024).toFixed(1)} KB).`);
      await refresh();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally { setBusy(null); }
  }

  async function doRestore(name: string) {
    if (!cfg?.has_passphrase) {
      setError("Save a passphrase first."); return;
    }
    const typed = window.prompt(`Passphrase to restore ${name}`) ?? "";
    if (!typed) return;
    if (!window.confirm(
      "Restore will replace the current database. The hub will restart. Continue?",
    )) return;
    setError(null); setInfo(null);
    setBusy("restore");
    try {
      const res = await backupApi.restore(name, typed, false);
      setInfo(`Restored from ${res.name}. Predecessor preserved at ${res.predecessor || "(none)"}.`);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally { setBusy(null); }
  }

  if (!cfg) {
    return (
      <SettingsPanel title="Backup" description="Loading…">
        <p className="text-sm text-[color:var(--color-muted)]">Loading backup configuration…</p>
        {error && <ErrorText message={error} />}
      </SettingsPanel>
    );
  }

  const dirty =
    enabled !== cfg.enabled ||
    target !== cfg.target ||
    localPath !== cfg.local_path ||
    s3Endpoint !== cfg.s3_endpoint ||
    s3Region !== (cfg.s3_region || "auto") ||
    s3Bucket !== cfg.s3_bucket ||
    s3Prefix !== (cfg.s3_prefix || "lumen/") ||
    s3AccessKey !== cfg.s3_access_key ||
    s3ForcePathStyle !== cfg.s3_force_path_style ||
    cron !== (cfg.cron || "0 2 * * *") ||
    retainLast !== (cfg.retain_last || 14) ||
    s3SecretKey.length > 0;

  return (
    <div className="space-y-4">
      <SettingsPanel
        title="Backup"
        description="Encrypt and ship the hub's SQLite database to a local path or S3-compatible bucket. Restore via CLI (lumen-hub --restore=&lt;file&gt;) or the list below."
      >
        <form
          onSubmit={(e) => { e.preventDefault(); void saveConfig(); }}
          className="space-y-4"
        >
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
            />
            <span>Enable scheduled backups</span>
          </label>

          <Field label="Target">
            <SegmentedControl
              value={target}
              onChange={(v) => setTarget(v as "local" | "s3")}
              ariaLabel="Backup target"
              options={[
                { value: "local", label: "Local path" },
                { value: "s3", label: "S3-compatible" },
              ]}
            />
          </Field>

          {target === "local" ? (
            <Field label="Local path">
              <FieldInput
                value={localPath}
                onChange={(e) => setLocalPath(e.target.value)}
                placeholder="/var/lib/lumen-backups"
              />
              <p className="mt-1 text-xs text-[color:var(--color-muted)]">Absolute path on the hub host. Created if missing.</p>
            </Field>
          ) : (
            <div className="space-y-3 rounded-md border border-[color:var(--color-border)] p-3">
              <Field label="Endpoint">
                <FieldInput value={s3Endpoint} onChange={(e) => setS3Endpoint(e.target.value)} placeholder="https://s3.amazonaws.com" />
                <p className="mt-1 text-xs text-[color:var(--color-muted)]">e.g. AWS, R2 (https://&lt;acct&gt;.r2.cloudflarestorage.com), MinIO, B2.</p>
              </Field>
              <div className="grid grid-cols-2 gap-3">
                <Field label="Region">
                  <FieldInput value={s3Region} onChange={(e) => setS3Region(e.target.value)} placeholder="auto" />
                </Field>
                <Field label="Bucket">
                  <FieldInput value={s3Bucket} onChange={(e) => setS3Bucket(e.target.value)} placeholder="lumen-backups" />
                </Field>
              </div>
              <Field label="Prefix">
                <FieldInput value={s3Prefix} onChange={(e) => setS3Prefix(e.target.value)} placeholder="lumen/" />
                <p className="mt-1 text-xs text-[color:var(--color-muted)]">Object-key prefix. Trailing slash recommended.</p>
              </Field>
              <div className="grid grid-cols-2 gap-3">
                <Field label="Access key">
                  <FieldInput value={s3AccessKey} onChange={(e) => setS3AccessKey(e.target.value)} />
                </Field>
                <Field label="Secret key">
                  <FieldInput type="password" value={s3SecretKey} onChange={(e) => setS3SecretKey(e.target.value)} placeholder="••••••" />
                  <p className="mt-1 text-xs text-[color:var(--color-muted)]">{cfg.has_secret_key ? "Already saved. Type a new value to replace." : "Not set."}</p>
                </Field>
              </div>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={s3ForcePathStyle}
                  onChange={(e) => setS3ForcePathStyle(e.target.checked)}
                />
                <span>Path-style addressing (MinIO, older endpoints)</span>
              </label>
            </div>
          )}

          <div className="grid grid-cols-2 gap-3">
            <Field label="Cron expression">
              <FieldInput value={cron} onChange={(e) => setCron(e.target.value)} />
              <p className="mt-1 text-xs text-[color:var(--color-muted)]">5-field. e.g. 0 2 * * * = daily 02:00, 0 */6 * * * = every 6 hours.</p>
            </Field>
            <Field label="Retain last N">
              <FieldInput
                type="number" min={1} max={365}
                value={retainLast}
                onChange={(e) => setRetainLast(parseInt(e.target.value || "0", 10) || 0)}
              />
              <p className="mt-1 text-xs text-[color:var(--color-muted)]">Older backups are swept after each successful run.</p>
            </Field>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <GhostButton type="button" onClick={() => void testTarget()} disabled={busy === "test"}>
              {busy === "test" ? "Testing…" : "Test target"}
            </GhostButton>
            <PrimaryButton type="submit" disabled={!dirty || busy === "save"}>
              {busy === "save" ? "Saving…" : "Save"}
            </PrimaryButton>
            {error && <ErrorText message={error} />}
            {info && <span className="text-xs text-[color:var(--color-muted)]">{info}</span>}
          </div>
        </form>
      </SettingsPanel>

      <SettingsPanel
        title="Passphrase"
        description="Encrypts the backup at rest. The hash is stored; the passphrase is not — losing it means every backup is unrecoverable. Save it in your password manager."
      >
        <form
          onSubmit={(e) => { e.preventDefault(); void savePassphrase(); }}
          className="flex flex-wrap items-end gap-2"
        >
          <Field label="New passphrase">
            <FieldInput
              type="password" value={passphrase}
              onChange={(e) => setPassphrase(e.target.value)}
              placeholder={cfg.has_passphrase ? "(already set — type to replace)" : ""}
            />
          </Field>
          <PrimaryButton type="submit" disabled={!passphrase || busy === "pass"}>
            {busy === "pass" ? "Saving…" : "Save passphrase"}
          </PrimaryButton>
        </form>
      </SettingsPanel>

      <SettingsPanel
        title="Recent backups"
        description="Newest first. Download saves the encrypted file. Restore replaces the live database and restarts the hub — production restore should use lumen-hub --restore=&lt;file&gt; from a stopped service."
      >
        <div className="mb-3">
          <GhostButton onClick={() => void runNow()} disabled={!cfg.has_passphrase || busy === "run"}>
            <Play size={14} className="mr-1.5" />
            {busy === "run" ? "Running…" : "Backup now"}
          </GhostButton>
        </div>
        {!entries || entries.length === 0 ? (
          <p className="text-sm text-[color:var(--color-muted)]">No backups yet.</p>
        ) : (
          <ul className="space-y-1.5">
            {entries.map((e) => (
              <li key={e.name} className="flex items-center justify-between rounded-md border border-[color:var(--color-border)] px-3 py-2 text-sm">
                <div>
                  <div className="font-medium">{e.name}</div>
                  <div className="text-xs text-[color:var(--color-muted)]">
                    {(e.size / 1024).toFixed(1)} KB · {new Date(e.created_at).toLocaleString()}
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <a
                    href={backupApi.downloadUrl(e.name)}
                    className="inline-flex items-center gap-1 rounded-md border border-[color:var(--color-border)] px-2 py-1 text-xs hover:bg-[color:var(--color-border)]/30"
                  >
                    <Download size={12} /> Download
                  </a>
                  <button
                    type="button"
                    disabled={busy === "restore"}
                    onClick={() => void doRestore(e.name)}
                    className="inline-flex items-center gap-1 rounded-md border border-[color:var(--color-border)] px-2 py-1 text-xs hover:bg-[color:var(--color-border)]/30 disabled:opacity-50"
                  >
                    <RotateCcw size={12} /> Restore
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </SettingsPanel>
    </div>
  );
}
