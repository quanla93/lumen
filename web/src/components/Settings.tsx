import { useCallback, useEffect, useState } from "react";
import { hostsApi, ApiError, type Host } from "@/lib/api";
import { relativeTime } from "@/lib/time";
import { ErrorText, Field, FieldInput, GhostButton, PrimaryButton } from "@/components/CenterCard";
import { TokenReveal } from "@/components/TokenReveal";

type RevealState = { hostName: string; token: string } | null;

export function Settings() {
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
