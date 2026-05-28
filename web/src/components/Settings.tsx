import { useCallback, useEffect, useState } from "react";
import {
  hostsApi,
  authApi,
  settingsApi,
  ApiError,
  type Host,
  type User,
  type SettingsResponse,
} from "@/lib/api";
import { relativeTime } from "@/lib/time";
import { ErrorText, Field, FieldInput, GhostButton, PrimaryButton } from "@/components/CenterCard";
import { Surface } from "@/components/ui";
import { TokenReveal } from "@/components/TokenReveal";
import { useI18n } from "@/i18n/useI18n";

type SettingsTab = "hosts" | "account" | "runtime" | "retention" | "downsample" | "logs";

const TABS: { id: SettingsTab; labelKey: "settings.tabs.hosts" | "settings.tabs.account" | "settings.tabs.runtime" | "settings.tabs.retention" | "settings.tabs.downsample" | "settings.tabs.logs" }[] = [
  { id: "hosts",     labelKey: "settings.tabs.hosts" },
  { id: "account",   labelKey: "settings.tabs.account" },
  { id: "runtime",   labelKey: "settings.tabs.runtime" },
  { id: "retention", labelKey: "settings.tabs.retention" },
  { id: "downsample", labelKey: "settings.tabs.downsample" },
  { id: "logs",      labelKey: "settings.tabs.logs" },
];

export function Settings({ user }: { user: User }) {
  const { t } = useI18n();
  const [tab, setTab] = useState<SettingsTab>("hosts");
  return (
    <div className="space-y-4">
      <nav className="flex items-center gap-1 border-b border-[color:var(--color-border)] -mt-2 pb-0">
        {TABS.map((item) => (
          <SubTabButton
            key={item.id}
            active={item.id === tab}
            onClick={() => setTab(item.id)}
          >
            {t(item.labelKey)}
          </SubTabButton>
        ))}
      </nav>
      <div className="pt-2">
        {tab === "hosts"     && <HostsSettings />}
        {tab === "account"   && <AccountSettings user={user} />}
        {tab === "runtime"    && <RuntimeSettings />}
        {tab === "retention"  && <RetentionSettings />}
        {tab === "downsample" && <DownsampleSettings />}
        {tab === "logs"       && <LogManagementSettings />}
      </div>
    </div>
  );
}

function SubTabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  const base = "px-3 py-2 -mb-px text-sm border-b-2 transition-colors";
  return (
    <button
      type="button"
      onClick={onClick}
      className={
        active
          ? `${base} border-[color:var(--color-fg)] text-[color:var(--color-fg)] font-medium`
          : `${base} border-transparent text-[color:var(--color-muted)] hover:text-[color:var(--color-fg)]`
      }
    >
      {children}
    </button>
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
  locale,
  t,
}: {
  hosts: Host[];
  now: number;
  onChanged: () => void;
  onTokenRevealed: (hostName: string, token: string) => void;
  locale: ReturnType<typeof useI18n>["locale"];
  t: ReturnType<typeof useI18n>["t"];
}) {
  return (
    <div className="overflow-x-auto rounded-lg border border-[color:var(--color-border)]">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-xs uppercase tracking-wide text-[color:var(--color-muted)] bg-[color:var(--color-card)]">
            <th className="px-3 py-2 font-medium">{t("settings.tableName")}</th>
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
  locale,
  t,
}: {
  host: Host;
  now: number;
  onChanged: () => void;
  onTokenRevealed: (hostName: string, token: string) => void;
  locale: ReturnType<typeof useI18n>["locale"];
  t: ReturnType<typeof useI18n>["t"];
}) {
  const [busy, setBusy] = useState(false);

  async function rotate() {
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
    if (!window.confirm(t("settings.deleteConfirm", { name: host.name }))) {
      return;
    }
    setBusy(true);
    try {
      await hostsApi.remove(host.id);
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
      <td className="px-3 py-2 font-mono">{host.name}</td>
      <td className="px-3 py-2 text-[color:var(--color-muted)]">
        {host.last_seen_at ? relativeTime(host.last_seen_at, now, locale) : t("common.never")}
      </td>
      <td className="px-3 py-2 text-[color:var(--color-muted)]">
        {new Date(host.created_at).toLocaleString()}
      </td>
      <td className="px-3 py-2 text-right space-x-2 whitespace-nowrap">
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

function parseDurationInput(duration: string): DurationInput {
  const match = duration.match(/^(\d+)(s|m|h)$/);
  if (!match) {
    return { value: duration, unit: "h" };
  }
  const amount = Number(match[1]);
  if (match[2] === "h" && amount % 24 === 0) {
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
}: {
  label: string;
  value: DurationInput;
  onChange: (value: DurationInput) => void;
}) {
  const { t } = useI18n();
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
          {DURATION_UNITS.map((unit) => (
            <option key={unit.value} value={unit.value}>{t(unit.labelKey)}</option>
          ))}
        </select>
      </div>
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
        <h2 className="text-base font-semibold tracking-tight mb-3">{t("settings.downsampleTitle")}</h2>
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
}: {
  title: string;
  description: string;
  children?: React.ReactNode;
}) {
  return (
    <Surface as="section">
      <h2 className="text-base font-semibold tracking-tight">{title}</h2>
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
  const [busy, setBusy]               = useState(false);
  const [error, setError]             = useState<string | null>(null);
  const [savedAt, setSavedAt]         = useState<number | null>(null);

  useEffect(() => {
    settingsApi.get().then((s) => {
      setSettings(s);
      setWindow(parseDurationInput(s.retention_window));
      setInterval(parseDurationInput(s.retention_interval));
    }).catch((err) => {
      setError(err instanceof ApiError ? err.message : String(err));
    });
  }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    const retentionWindow = formatDurationInput(window);
    const retentionInterval = formatDurationInput(interval);
    if (!retentionWindow || !retentionInterval) {
      setError(t("settings.retentionInvalid"));
      return;
    }
    setBusy(true);
    try {
      const next = await settingsApi.put({
        retention_window:   retentionWindow,
        retention_interval: retentionInterval,
      });
      setSettings(next);
      setWindow(parseDurationInput(next.retention_window));
      setInterval(parseDurationInput(next.retention_interval));
      setSavedAt(Date.now());
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  const windowDuration = formatDurationInput(window);
  const intervalDuration = formatDurationInput(interval);
  const dirty =
    !!settings &&
    (windowDuration !== settings.retention_window || intervalDuration !== settings.retention_interval);

  return (
    <div className="max-w-md space-y-4">
      <section>
        <h2 className="text-base font-semibold tracking-tight mb-3">{t("settings.retentionTitle")}</h2>
        <p className="text-sm text-[color:var(--color-muted)]">
          {t("settings.retentionDescription")}
        </p>
        <p className="mt-2 text-sm text-[color:var(--color-muted)]">
          {t("settings.retentionWindowHelp")} {t("settings.retentionIntervalHelp")}
        </p>
      </section>

      {!settings ? (
        <p className="text-sm text-[color:var(--color-muted)]">{t("common.loading")}</p>
      ) : (
        <form onSubmit={submit} className="space-y-3">
          <DurationField
            label={t("settings.window")}
            value={window}
            onChange={setWindow}
          />
          <DurationField
            label={t("settings.interval")}
            value={interval}
            onChange={setInterval}
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
