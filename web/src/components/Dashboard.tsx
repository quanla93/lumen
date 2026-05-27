import { useEffect, useMemo, useState } from "react";
import { HostCard, type Snapshot } from "@/components/HostCard";
import { EmptyState, StatusPill, Surface } from "@/components/ui";
import { settingsApi } from "@/lib/api";
import { cpuTone, TONE_CLASS, type StatusTone } from "@/lib/status";
import { isStale, staleAfterForIntervalMs } from "@/lib/time";

type WsStatus = "connecting" | "connected" | "disconnected" | "error";

const STATUS_META: Record<WsStatus, { tone: StatusTone; label: string }> = {
  connected:    { tone: "ok",     label: "connected" },
  connecting:   { tone: "warn",   label: "connecting…" },
  disconnected: { tone: "muted",  label: "disconnected" },
  error:        { tone: "danger", label: "error" },
};

export function Dashboard({
  onSelectHost,
}: {
  onSelectHost?: (hostName: string) => void;
}) {
  const [snapshots, setSnapshots] = useState<Snapshot[]>([]);
  const [status, setStatus] = useState<WsStatus>("connecting");
  const [agentInterval, setAgentInterval] = useState("5s");
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

    const scheme = window.location.protocol === "https:" ? "wss" : "ws";
    const url = `${scheme}://${window.location.host}/api/stream`;
    const ws = new WebSocket(url);

    ws.addEventListener("open", () => setStatus("connected"));
    ws.addEventListener("message", (e) => {
      try {
        const parsed = JSON.parse(e.data as string) as Snapshot[];
        setSnapshots(parsed ?? []);
      } catch {
        // ignore malformed frames
      }
    });
    ws.addEventListener("close", () => setStatus("disconnected"));
    ws.addEventListener("error", () => setStatus("error"));

    const tick = window.setInterval(() => setNow(Date.now()), 1000);
    return () => {
      cancelled = true;
      window.clearInterval(tick);
      ws.close();
    };
  }, []);

  const meta = STATUS_META[status];
  const sorted = useMemo(
    () => [...snapshots].sort((a, b) => a.host.localeCompare(b.host)),
    [snapshots],
  );
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return sorted;
    return sorted.filter((s) => s.host.toLowerCase().includes(q));
  }, [query, sorted]);
  const staleAfterMs = useMemo(() => staleAfterForIntervalMs(agentInterval), [agentInterval]);
  const summary = useMemo(() => summarizeSnapshots(snapshots, now, staleAfterMs), [snapshots, now, staleAfterMs]);

  return (
    <div className="space-y-6">
      <Surface as="section">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <div className="mb-2">
              <StatusPill tone={meta.tone}>WebSocket {meta.label}</StatusPill>
            </div>
            <h2 className="text-2xl font-semibold tracking-tight">Homelab overview</h2>
            <p className="mt-1 text-sm text-[color:var(--color-muted)]">
              Live host health, resource pressure, and stale agent detection.
            </p>
          </div>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:min-w-[32rem]">
            <SummaryCard label="Hosts" value={summary.total} detail={`${summary.online} online`} tone="ok" />
            <SummaryCard label="Stale" value={summary.stale} detail="no recent tick" tone={summary.stale > 0 ? "warn" : "muted"} />
            <SummaryCard label="Avg CPU" value={`${summary.avgCpu.toFixed(0)}%`} detail="fleet average" tone={cpuTone(summary.avgCpu, summary.total === 0)} />
            <SummaryCard label="Avg RAM" value={`${summary.avgRam.toFixed(0)}%`} detail="fleet average" tone={cpuTone(summary.avgRam, summary.total === 0)} />
          </div>
        </div>
      </Surface>

      {sorted.length === 0 ? (
        <EmptyState
          title="No host data yet"
          detail={<>Add a host in <strong>Settings</strong>, copy its token, then start the agent to stream the first metrics.</>}
        />
      ) : (
        <section>
          <div className="mb-3 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <h3 className="text-sm font-medium text-[color:var(--color-muted)]">
              {filtered.length} of {sorted.length} monitored host{sorted.length === 1 ? "" : "s"}
            </h3>
            <label className="relative w-full sm:w-72">
              <span className="sr-only">Search hosts</span>
              <input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search hosts…"
                className="w-full rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-3 py-2 text-sm outline-none transition-colors placeholder:text-[color:var(--color-muted)] focus:border-[color:var(--color-accent)]"
              />
            </label>
          </div>
          {filtered.length === 0 ? (
            <EmptyState
              title="No matching hosts"
              detail={<>No hosts match “{query}”.</>}
              className="p-8"
            />
          ) : (
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
              {filtered.map((s) => (
                <HostCard key={s.host} snapshot={s} now={now} agentInterval={agentInterval} onSelect={onSelectHost} />
              ))}
            </div>
          )}
        </section>
      )}
    </div>
  );
}

function summarizeSnapshots(snapshots: Snapshot[], now: number, staleAfterMs: number) {
  const total = snapshots.length;
  const stale = snapshots.filter((s) => isStale(s.ts, staleAfterMs, now)).length;
  const online = total - stale;
  const avgCpu = average(snapshots.map((s) => s.cpu_pct));
  const avgRam = average(snapshots.map((s) => s.ram_pct));
  return { total, online, stale, avgCpu, avgRam };
}

function average(values: number[]) {
  if (values.length === 0) return 0;
  return values.reduce((sum, value) => sum + value, 0) / values.length;
}

function SummaryCard({
  label,
  value,
  detail,
  tone,
}: {
  label: string;
  value: string | number;
  detail: string;
  tone: StatusTone;
}) {
  return (
    <div className="rounded-xl border border-[color:var(--color-border)] bg-[color:var(--color-bg)] p-3">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs uppercase tracking-wide text-[color:var(--color-muted)]">{label}</span>
        <span aria-hidden className={`h-2 w-2 rounded-full ${TONE_CLASS[tone]}`} />
      </div>
      <div className="mt-2 text-2xl font-semibold tabular-nums">{value}</div>
      <div className="mt-1 text-xs text-[color:var(--color-muted)]">{detail}</div>
    </div>
  );
}
