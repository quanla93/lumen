import { useEffect, useState } from "react";
import { publicStatusApi, type PublicStatus } from "@/lib/api";

// StatusPage — unauthenticated /status route. Polls /api/public/status
// every 15s so visitors see roughly-live state without flooding the hub.
// Renders three terminal states deterministically (loading / not-published
// / published) so an operator can link to /status from a public page
// even before they've enabled the feature.
export function StatusPage() {
  const [data, setData] = useState<PublicStatus | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    async function tick() {
      try {
        const next = await publicStatusApi.getPublic();
        if (!cancelled) {
          setData(next);
          setErr(null);
        }
      } catch (e) {
        if (!cancelled) setErr(String(e));
      }
    }
    tick();
    const id = window.setInterval(tick, 15_000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, []);

  if (err && !data) {
    return (
      <Shell>
        <p className="text-sm text-red-500">Status page is unreachable: {err}</p>
      </Shell>
    );
  }
  if (!data) {
    return (
      <Shell>
        <p className="text-sm text-[color:var(--color-muted)]">Loading…</p>
      </Shell>
    );
  }
  if (!data.enabled) {
    return (
      <Shell title="Status">
        <p className="text-sm text-[color:var(--color-muted)]">
          This status page isn't published. The Lumen admin hasn't enabled it yet.
        </p>
      </Shell>
    );
  }
  return (
    <Shell title={data.title}>
      {data.description && (
        <p className="mb-6 text-sm text-[color:var(--color-muted)]">{data.description}</p>
      )}
      {data.hosts.length === 0 ? (
        <p className="text-sm text-[color:var(--color-muted)]">No hosts are public yet.</p>
      ) : (
        <ul className="space-y-3">
          {data.hosts.map((h) => (
            <li
              key={h.name}
              className="rounded-md border border-[color:var(--color-border)] p-4"
            >
              <div className="flex items-center justify-between gap-4">
                <div className="flex items-center gap-2">
                  <StateDot state={h.state} />
                  <span className="font-medium">{h.name}</span>
                  <span className="text-xs uppercase tracking-wide text-[color:var(--color-muted)]">
                    {h.state}
                  </span>
                </div>
                {h.last_seen_at && (
                  <span className="text-xs text-[color:var(--color-muted)]">
                    last seen {new Date(h.last_seen_at).toLocaleString()}
                  </span>
                )}
              </div>
              {h.state === "up" || h.state === "stale" ? (
                <div className="mt-3 grid grid-cols-3 gap-3 text-xs">
                  <Metric label="CPU" value={h.cpu_pct} />
                  <Metric label="RAM" value={h.ram_pct} />
                  <Metric label="Disk" value={h.disk_pct} />
                </div>
              ) : null}
            </li>
          ))}
        </ul>
      )}
      <p className="mt-8 text-xs text-[color:var(--color-muted)]">
        Updated {new Date(data.generated_at).toLocaleTimeString()} ·{" "}
        <a href="/" className="underline">Lumen</a>
      </p>
    </Shell>
  );
}

function Shell({ children, title }: { children: React.ReactNode; title?: string }) {
  return (
    <main className="mx-auto max-w-2xl px-4 py-12">
      {title && <h1 className="mb-6 text-2xl font-bold">{title}</h1>}
      {children}
    </main>
  );
}

function StateDot({ state }: { state: string }) {
  const color =
    state === "up"
      ? "bg-emerald-500"
      : state === "stale"
        ? "bg-amber-500"
        : state === "down"
          ? "bg-red-500"
          : "bg-zinc-500";
  return <span aria-hidden className={`inline-block h-2.5 w-2.5 rounded-full ${color}`} />;
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div>
      <div className="text-[color:var(--color-muted)]">{label}</div>
      <div className="font-medium text-[color:var(--color-fg)]">{value.toFixed(1)}%</div>
    </div>
  );
}
