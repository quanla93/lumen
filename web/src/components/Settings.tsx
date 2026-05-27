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
import { TokenReveal } from "@/components/TokenReveal";

type SettingsTab = "hosts" | "account" | "runtime" | "retention" | "downsample" | "logs";

const TABS: { id: SettingsTab; label: string }[] = [
  { id: "hosts",     label: "Hosts" },
  { id: "account",   label: "Account" },
  { id: "runtime",    label: "Runtime" },
  { id: "retention",  label: "Retention" },
  { id: "downsample", label: "Downsample" },
  { id: "logs",       label: "Logs" },
];

export function Settings({ user }: { user: User }) {
  const [tab, setTab] = useState<SettingsTab>("hosts");
  return (
    <div className="space-y-4">
      <nav className="flex items-center gap-1 border-b border-[color:var(--color-border)] -mt-2 pb-0">
        {TABS.map((t) => (
          <SubTabButton
            key={t.id}
            active={t.id === tab}
            onClick={() => setTab(t.id)}
          >
            {t.label}
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
        <h2 className="text-base font-semibold tracking-tight mb-3">Hosts</h2>
        <p className="text-sm text-[color:var(--color-muted)] mb-4">
          Create a host to mint a bearer token. The agent uses that token
          to push metrics; the hub stores only its SHA-256 hash.
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
          <p className="text-sm text-[color:var(--color-muted)]">Loading hosts…</p>
        ) : hosts.length === 0 ? (
          <p className="text-sm text-[color:var(--color-muted)]">
            No hosts yet. Create one above.
          </p>
        ) : (
          <HostsTable
            hosts={hosts}
            now={now}
            onChanged={refresh}
            onTokenRevealed={(hostName, token) => setReveal({ hostName, token })}
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
        <Field label="New host name">
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
        {busy ? "Creating…" : "Create"}
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
}: {
  hosts: Host[];
  now: number;
  onChanged: () => void;
  onTokenRevealed: (hostName: string, token: string) => void;
}) {
  return (
    <div className="overflow-x-auto rounded-lg border border-[color:var(--color-border)]">
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-xs uppercase tracking-wide text-[color:var(--color-muted)] bg-[color:var(--color-card)]">
            <th className="px-3 py-2 font-medium">Name</th>
            <th className="px-3 py-2 font-medium">Last seen</th>
            <th className="px-3 py-2 font-medium">Created</th>
            <th className="px-3 py-2 font-medium text-right">Actions</th>
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
}: {
  host: Host;
  now: number;
  onChanged: () => void;
  onTokenRevealed: (hostName: string, token: string) => void;
}) {
  const [busy, setBusy] = useState(false);

  async function rotate() {
    setBusy(true);
    try {
      const res = await hostsApi.rotate(host.id);
      onTokenRevealed(host.name, res.token);
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : String(err);
      window.alert(`Rotate failed: ${msg}`);
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!window.confirm(`Delete host "${host.name}"? Past snapshots stay; agent will start failing on next tick.`)) {
      return;
    }
    setBusy(true);
    try {
      await hostsApi.remove(host.id);
      onChanged();
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : String(err);
      window.alert(`Delete failed: ${msg}`);
    } finally {
      setBusy(false);
    }
  }

  return (
    <tr className="border-t border-[color:var(--color-border)]">
      <td className="px-3 py-2 font-mono">{host.name}</td>
      <td className="px-3 py-2 text-[color:var(--color-muted)]">
        {host.last_seen_at ? relativeTime(host.last_seen_at, now) : "never"}
      </td>
      <td className="px-3 py-2 text-[color:var(--color-muted)]">
        {new Date(host.created_at).toLocaleString()}
      </td>
      <td className="px-3 py-2 text-right space-x-2 whitespace-nowrap">
        <GhostButton onClick={rotate} disabled={busy}>
          Rotate token
        </GhostButton>
        <GhostButton onClick={remove} disabled={busy}>
          Delete
        </GhostButton>
      </td>
    </tr>
  );
}

// ─── Account sub-tab ─────────────────────────────────────────────────────────

function AccountSettings({ user }: { user: User }) {
  return (
    <div className="space-y-6 max-w-md">
      <section>
        <h2 className="text-base font-semibold tracking-tight mb-3">Account</h2>
        <dl className="text-sm grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5">
          <dt className="text-[color:var(--color-muted)]">Username</dt>
          <dd className="font-mono">{user.username}</dd>
          <dt className="text-[color:var(--color-muted)]">Created</dt>
          <dd>{new Date(user.created_at).toLocaleString()}</dd>
        </dl>
      </section>
      <section>
        <h3 className="text-sm font-semibold tracking-tight mb-2">Change password</h3>
        <ChangePasswordForm />
      </section>
    </div>
  );
}

function ChangePasswordForm() {
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
      setError("New password must be at least 8 characters");
      return;
    }
    if (next !== confirm) {
      setError("New password and confirmation don't match");
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
      <Field label="Current password">
        <FieldInput
          type="password"
          autoComplete="current-password"
          value={current}
          onChange={(e) => setCurrent(e.target.value)}
          required
        />
      </Field>
      <Field label="New password (min 8 chars)">
        <FieldInput
          type="password"
          autoComplete="new-password"
          value={next}
          onChange={(e) => setNext(e.target.value)}
          minLength={8}
          required
        />
      </Field>
      <Field label="Confirm new password">
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
          Password updated. Your existing session stays valid.
        </p>
      )}
      <PrimaryButton disabled={busy}>
        {busy ? "Updating…" : "Change password"}
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

const DURATION_UNITS: { value: DurationUnit; label: string; seconds: number }[] = [
  { value: "s", label: "seconds", seconds: 1 },
  { value: "m", label: "minutes", seconds: 60 },
  { value: "h", label: "hours",   seconds: 60 * 60 },
  { value: "d", label: "days",    seconds: 24 * 60 * 60 },
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
          aria-label={`${label} unit`}
          className="rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-[color:var(--color-accent)]"
          value={value.unit}
          onChange={(e) => onChange({ ...value, unit: e.target.value as DurationUnit })}
        >
          {DURATION_UNITS.map((unit) => (
            <option key={unit.value} value={unit.value}>{unit.label}</option>
          ))}
        </select>
      </div>
    </Field>
  );
}

function RuntimeSettings() {
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
      setError("Agent interval must be a positive whole number.");
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
        <h2 className="text-base font-semibold tracking-tight mb-3">Runtime</h2>
        <p className="text-sm text-[color:var(--color-muted)]">
          Agent collection interval controls how often agents sample host metrics.
          Running agents apply changes after their next policy refresh.
        </p>
      </section>

      {!settings ? (
        <p className="text-sm text-[color:var(--color-muted)]">Loading…</p>
      ) : (
        <form onSubmit={submit} className="space-y-3">
          <DurationField
            label="Agent collection interval"
            value={agentInterval}
            onChange={setAgentInterval}
          />
          {error && <ErrorText message={error} />}
          {savedAt && !dirty && (
            <p role="status" className="text-sm text-[color:var(--color-accent)]">
              Saved {new Date(savedAt).toLocaleTimeString()}.
            </p>
          )}
          <PrimaryButton disabled={busy || !dirty}>
            {busy ? "Saving…" : "Save"}
          </PrimaryButton>
        </form>
      )}
    </div>
  );
}

function DownsampleSettings() {
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
      setError("Downsample policy values must be positive whole numbers.");
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
        <h2 className="text-base font-semibold tracking-tight mb-3">Downsample policy</h2>
        <p className="text-sm text-[color:var(--color-muted)]">
          These settings control how long Lumen keeps detailed raw metrics, and
          how older metrics will be compressed into long-term history.
        </p>
        <ul className="mt-3 space-y-1.5 text-sm text-[color:var(--color-muted)]">
          <li><strong className="text-[color:var(--color-fg)]">Bucket size</strong> is the time span for one archived point. Example: 5m means old data is averaged into one point every 5 minutes.</li>
          <li><strong className="text-[color:var(--color-fg)]">Hot window</strong> is how long full-detail raw data stays in SQLite. Example: 24h means the last day keeps every agent sample.</li>
          <li><strong className="text-[color:var(--color-fg)]">Archive window</strong> is how long compressed history is kept. Example: 365d means archived data is kept for one year.</li>
        </ul>
      </section>

      {!settings ? (
        <p className="text-sm text-[color:var(--color-muted)]">Loading…</p>
      ) : (
        <form onSubmit={submit} className="space-y-3">
          <DurationField
            label="Bucket size"
            value={bucketSize}
            onChange={setBucketSize}
          />
          <DurationField
            label="Hot window"
            value={hotWindow}
            onChange={setHotWindow}
          />
          <DurationField
            label="Archive window"
            value={archiveWindow}
            onChange={setArchiveWindow}
          />
          {error && <ErrorText message={error} />}
          {savedAt && !dirty && (
            <p role="status" className="text-sm text-[color:var(--color-accent)]">
              Saved {new Date(savedAt).toLocaleTimeString()}.
            </p>
          )}
          <PrimaryButton disabled={busy || !dirty}>
            {busy ? "Saving…" : "Save"}
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
    <section className="rounded-2xl border border-[color:var(--color-border)] bg-[color:var(--color-card)] p-5 shadow-sm">
      <h2 className="text-base font-semibold tracking-tight">{title}</h2>
      <p className="mt-2 text-sm text-[color:var(--color-muted)]">{description}</p>
      {children && <div className="mt-4">{children}</div>}
    </section>
  );
}

function LogManagementSettings() {
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <SettingsPanel
        title="On-demand log viewer"
        description="Phase 3 will add bounded, admin-only log retrieval for incident debugging. Lumen will show recent lines on request instead of indexing every log line."
      >
        <ul className="space-y-2 text-sm text-[color:var(--color-muted)]">
          <li><strong className="text-[color:var(--color-fg)]">Sources:</strong> Lumen agent, systemd/journald units, and Docker containers.</li>
          <li><strong className="text-[color:var(--color-fg)]">Limits:</strong> last N lines, short time ranges, optional live tail.</li>
          <li><strong className="text-[color:var(--color-fg)]">Storage:</strong> no default persistence, indexing, or global search.</li>
        </ul>
      </SettingsPanel>
      <SettingsPanel
        title="Not a Loki replacement"
        description="Log management stays lightweight so the hub remains HDD-friendly and predictable for homelab installs. Export/integration can be researched later."
      >
        <div className="rounded-xl border border-[color:var(--color-border)] bg-[color:var(--color-bg)] p-4 font-mono text-xs text-[color:var(--color-muted)]">
          host=pve-01 source=docker target=nginx tail=500
        </div>
      </SettingsPanel>
    </div>
  );
}

function RetentionSettings() {
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
      setError("Window and interval must be positive whole numbers.");
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
        <h2 className="text-base font-semibold tracking-tight mb-3">Retention</h2>
        <p className="text-sm text-[color:var(--color-muted)]">
          Snapshots older than <strong>Window</strong> are pruned every{" "}
          <strong>Interval</strong>. Changes apply on the next sweep —
          no hub restart required.
        </p>
        <p className="mt-2 text-sm text-[color:var(--color-muted)]">
          Cold-tier (Parquet) archival lands in Phase 5 — for now this is
          a hard delete.
        </p>
      </section>

      {!settings ? (
        <p className="text-sm text-[color:var(--color-muted)]">Loading…</p>
      ) : (
        <form onSubmit={submit} className="space-y-3">
          <DurationField
            label="Window"
            value={window}
            onChange={setWindow}
          />
          <DurationField
            label="Interval"
            value={interval}
            onChange={setInterval}
          />
          {error && <ErrorText message={error} />}
          {savedAt && !dirty && (
            <p role="status" className="text-sm text-[color:var(--color-accent)]">
              Saved {new Date(savedAt).toLocaleTimeString()}.
            </p>
          )}
          <PrimaryButton disabled={busy || !dirty}>
            {busy ? "Saving…" : "Save"}
          </PrimaryButton>
        </form>
      )}
    </div>
  );
}
