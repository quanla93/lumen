import { useEffect, useState } from "react";
import { HostCard, type Snapshot } from "@/components/HostCard";
import { ThemeToggle } from "@/components/ThemeToggle";
import { TONE_CLASS, type StatusTone } from "@/lib/status";

type Status = "connecting" | "connected" | "disconnected" | "error";

const STATUS_META: Record<Status, { tone: StatusTone; label: string }> = {
  connected:    { tone: "ok",     label: "connected" },
  connecting:   { tone: "warn",   label: "connecting…" },
  disconnected: { tone: "muted",  label: "disconnected" },
  error:        { tone: "danger", label: "error" },
};

export default function App() {
  const [snapshots, setSnapshots] = useState<Snapshot[]>([]);
  const [status, setStatus] = useState<Status>("connecting");
  // `now` ticks every second so the relative timestamps refresh without a
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
    <main className="mx-auto max-w-5xl px-4 py-8 sm:py-12">
      <header className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Lumen</h1>
          <p className="text-sm text-[color:var(--color-muted)]">
            <span
              aria-hidden
              className={`inline-block h-2 w-2 rounded-full align-middle mr-1.5 ${TONE_CLASS[meta.tone]}`}
            />
            <span>WebSocket {meta.label} · {snapshots.length} host{snapshots.length === 1 ? "" : "s"}</span>
          </p>
        </div>
        <ThemeToggle />
      </header>

      {sorted.length === 0 ? (
        <div className="rounded-lg border border-dashed border-[color:var(--color-border)] p-10 text-center">
          <p className="text-[color:var(--color-muted)]">
            Waiting for agent data…
          </p>
          <p className="mt-2 text-sm text-[color:var(--color-muted)]">
            Start{" "}
            <code className="font-mono px-1.5 py-0.5 rounded bg-[color:var(--color-border)] text-[color:var(--color-fg)]">
              make dev-agent
            </code>{" "}
            in another terminal.
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {sorted.map((s) => (
            <HostCard key={s.host} snapshot={s} now={now} />
          ))}
        </div>
      )}

      <footer className="mt-12 text-xs text-[color:var(--color-muted)] text-center">
        Lumen pre-v0.1 spike · CPU only (Phase 2 adds RAM, disk, network, charts)
      </footer>
    </main>
  );
}
