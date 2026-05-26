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

type SettingsTab = "hosts" | "account" | "retention";

const TABS: { id: SettingsTab; label: string }[] = [
  { id: "hosts",     label: "Hosts" },
  { id: "account",   label: "Account" },
  { id: "retention", label: "Retention" },
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
        {tab === "retention" && <RetentionSettings />}
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

function RetentionSettings() {
  const [settings, setSettings] = useState<SettingsResponse | null>(null);
  const [window, setWindow]     = useState("");
  const [interval, setInterval] = useState("");
  const [busy, setBusy]         = useState(false);
  const [error, setError]       = useState<string | null>(null);
  const [savedAt, setSavedAt]   = useState<number | null>(null);

  useEffect(() => {
    settingsApi.get().then((s) => {
      setSettings(s);
      setWindow(s.retention_window);
      setInterval(s.retention_interval);
    }).catch((err) => {
      setError(err instanceof ApiError ? err.message : String(err));
    });
  }, []);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      const next = await settingsApi.put({
        retention_window:   window,
        retention_interval: interval,
      });
      setSettings(next);
      setWindow(next.retention_window);
      setInterval(next.retention_interval);
      setSavedAt(Date.now());
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  const dirty =
    !!settings &&
    (window !== settings.retention_window || interval !== settings.retention_interval);

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
          Cold-tier (Parquet) archival lands in Phase 4 — for now this is
          a hard delete.
        </p>
      </section>

      {!settings ? (
        <p className="text-sm text-[color:var(--color-muted)]">Loading…</p>
      ) : (
        <form onSubmit={submit} className="space-y-3">
          <Field label="Window (Go duration: 24h, 7d-ish via 168h, …)">
            <FieldInput
              type="text"
              value={window}
              onChange={(e) => setWindow(e.target.value)}
              pattern="[0-9]+(ns|us|µs|ms|s|m|h)([0-9]+(ns|us|µs|ms|s|m|h))*"
              required
            />
          </Field>
          <Field label="Interval (e.g. 1h, 30m)">
            <FieldInput
              type="text"
              value={interval}
              onChange={(e) => setInterval(e.target.value)}
              pattern="[0-9]+(ns|us|µs|ms|s|m|h)([0-9]+(ns|us|µs|ms|s|m|h))*"
              required
            />
          </Field>
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
