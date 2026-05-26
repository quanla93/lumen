import { useEffect, useState } from "react";
import { HostCard, type Snapshot } from "@/components/HostCard";
import { TONE_CLASS, type StatusTone } from "@/lib/status";

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
  // `now` ticks every second so relative timestamps refresh without a
  // server push.
  const [now, setNow] = useState(Date.now());

  useEffect(() => {
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
      window.clearInterval(tick);
      ws.close();
    };
  }, []);

  const meta = STATUS_META[status];
  const sorted = [...snapshots].sort((a, b) => a.host.localeCompare(b.host));

  return (
    <>
      <p className="text-sm text-[color:var(--color-muted)] mb-6">
        <span
          aria-hidden
          className={`inline-block h-2 w-2 rounded-full align-middle mr-1.5 ${TONE_CLASS[meta.tone]}`}
        />
        WebSocket {meta.label} · {snapshots.length} host{snapshots.length === 1 ? "" : "s"}
      </p>

      {sorted.length === 0 ? (
        <div className="rounded-lg border border-dashed border-[color:var(--color-border)] p-10 text-center">
          <p className="text-[color:var(--color-muted)]">
            No host data yet.
          </p>
          <p className="mt-2 text-sm text-[color:var(--color-muted)]">
            Add a host in <strong>Settings</strong>, then run the agent with
            its token.
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {sorted.map((s) => (
            <HostCard key={s.host} snapshot={s} now={now} onSelect={onSelectHost} />
          ))}
        </div>
      )}
    </>
  );
}
