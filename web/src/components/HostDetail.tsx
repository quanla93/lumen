import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { AlignedData, Options } from "uplot";
import {
  hostsApi,
  ApiError,
  type MetricsResponse,
} from "@/lib/api";
import { UPlotChart } from "@/components/UPlotChart";

type Range = "1h" | "6h" | "24h";

const RANGE_SECONDS: Record<Range, number> = {
  "1h": 60 * 60,
  "6h": 6 * 60 * 60,
  "24h": 24 * 60 * 60,
};

const REFRESH_MS = 30_000;

// uPlot draws to <canvas>, so we have to bake colors at construction time.
// These oklch tokens come from index.css; they're tuned to look fine in
// both light and dark themes. Charts are recreated when the host or range
// changes, but NOT when the user toggles theme — re-toggling forces a
// refetch via range click if a refresh is needed.
const COLOR = {
  cpu: "oklch(70% 0.16 145)",   // accent green
  ram: "oklch(68% 0.13 240)",   // blue
  disk: "oklch(75% 0.16 75)",   // amber
  load1: "oklch(65% 0.22 30)",  // red
  load5: "oklch(68% 0.14 200)", // teal
  load15: "oklch(62% 0.12 290)", // purple
};

export function HostDetail({
  hostName,
  onBack,
}: {
  hostName: string;
  onBack: () => void;
}) {
  const [range, setRange] = useState<Range>("1h");
  const [hostId, setHostId] = useState<number | null>(null);
  const [resp, setResp] = useState<MetricsResponse | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const reqIdRef = useRef(0);

  // Resolve hostName → id once on mount. The hosts list is small and the
  // dashboard already pays the WS cost, so a one-shot fetch here is fine.
  useEffect(() => {
    let cancelled = false;
    hostsApi.list().then((hosts) => {
      if (cancelled) return;
      const match = hosts.find((h) => h.name === hostName);
      if (!match) {
        setErr(`Host "${hostName}" is no longer registered.`);
        setLoading(false);
        return;
      }
      setHostId(match.id);
    }).catch((e) => {
      if (!cancelled) {
        setErr(e instanceof ApiError ? e.message : String(e));
        setLoading(false);
      }
    });
    return () => { cancelled = true; };
  }, [hostName]);

  const fetchOnce = useCallback(async () => {
    if (hostId == null) return;
    const id = ++reqIdRef.current;
    const to = new Date();
    const from = new Date(to.getTime() - RANGE_SECONDS[range] * 1000);
    try {
      const r = await hostsApi.metrics(hostId, {
        from: from.toISOString(),
        to: to.toISOString(),
      });
      if (id !== reqIdRef.current) return;
      setResp(r);
      setErr(null);
    } catch (e) {
      if (id !== reqIdRef.current) return;
      setErr(e instanceof ApiError ? e.message : String(e));
    } finally {
      if (id === reqIdRef.current) setLoading(false);
    }
  }, [hostId, range]);

  useEffect(() => {
    if (hostId == null) return;
    setLoading(true);
    fetchOnce();
    const t = window.setInterval(fetchOnce, REFRESH_MS);
    return () => window.clearInterval(t);
  }, [hostId, fetchOnce]);

  const data = useMemo(() => buildSeries(resp), [resp]);

  return (
    <>
      <div className="mb-6 flex items-center justify-between gap-3 flex-wrap">
        <div className="flex items-center gap-3">
          <button
            type="button"
            onClick={onBack}
            className="text-sm rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-card)] px-2.5 py-1.5 hover:bg-[color:var(--color-border)] transition-colors"
          >
            ← Dashboard
          </button>
          <h2 className="font-mono text-base font-medium tracking-tight">
            {hostName}
          </h2>
        </div>
        <div className="flex items-center gap-1">
          {(Object.keys(RANGE_SECONDS) as Range[]).map((r) => (
            <RangeButton
              key={r}
              active={r === range}
              onClick={() => setRange(r)}
            >
              {r}
            </RangeButton>
          ))}
        </div>
      </div>

      {err && (
        <div className="mb-4 rounded-md border border-[color:var(--color-danger)] bg-[color:var(--color-card)] px-3 py-2 text-sm text-[color:var(--color-danger)]">
          {err}
        </div>
      )}

      {loading && !resp ? (
        <p className="text-sm text-[color:var(--color-muted)]">Loading…</p>
      ) : !resp || resp.points.length === 0 ? (
        <div className="rounded-lg border border-dashed border-[color:var(--color-border)] p-10 text-center">
          <p className="text-[color:var(--color-muted)]">
            No history yet for this host.
          </p>
          <p className="mt-2 text-sm text-[color:var(--color-muted)]">
            Once the agent sends a few samples, charts will fill in.
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          <ChartCard title="CPU %">
            <UPlotChart
              data={data.cpu}
              options={percentOpts("CPU", COLOR.cpu)}
              className="h-[220px] w-full"
            />
          </ChartCard>
          <ChartCard title="RAM %">
            <UPlotChart
              data={data.ram}
              options={percentOpts("RAM", COLOR.ram)}
              className="h-[220px] w-full"
            />
          </ChartCard>
          <ChartCard title="Disk %">
            <UPlotChart
              data={data.disk}
              options={percentOpts("Disk", COLOR.disk)}
              className="h-[220px] w-full"
            />
          </ChartCard>
          <ChartCard title="Load avg (1 / 5 / 15)">
            <UPlotChart
              data={data.load}
              options={loadOpts()}
              className="h-[220px] w-full"
            />
          </ChartCard>
        </div>
      )}

      {resp && (
        <p className="mt-6 text-xs text-[color:var(--color-muted)]">
          {resp.points.length} points · step {resp.step_seconds}s · refreshing
          every {Math.round(REFRESH_MS / 1000)}s
        </p>
      )}
    </>
  );
}

function ChartCard({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-lg border border-[color:var(--color-border)] bg-[color:var(--color-card)] p-3 shadow-sm">
      <div className="mb-2 text-xs uppercase tracking-wide text-[color:var(--color-muted)]">
        {title}
      </div>
      {children}
    </div>
  );
}

function RangeButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  const base = "px-2.5 py-1 text-xs rounded-md transition-colors";
  return (
    <button
      type="button"
      onClick={onClick}
      className={
        active
          ? `${base} bg-[color:var(--color-border)] text-[color:var(--color-fg)]`
          : `${base} text-[color:var(--color-muted)] hover:text-[color:var(--color-fg)] hover:bg-[color:var(--color-border)]`
      }
    >
      {children}
    </button>
  );
}

function buildSeries(r: MetricsResponse | null) {
  if (!r || r.points.length === 0) {
    const empty: AlignedData = [[]];
    return { cpu: empty, ram: empty, disk: empty, load: empty };
  }
  const xs = r.points.map((p) => Math.floor(new Date(p.ts).getTime() / 1000));
  return {
    cpu:  [xs, r.points.map((p) => p.cpu_pct)] as AlignedData,
    ram:  [xs, r.points.map((p) => p.ram_pct)] as AlignedData,
    disk: [xs, r.points.map((p) => p.disk_pct)] as AlignedData,
    load: [
      xs,
      r.points.map((p) => p.load1),
      r.points.map((p) => p.load5),
      r.points.map((p) => p.load15),
    ] as AlignedData,
  };
}

function percentOpts(label: string, color: string): Omit<Options, "width" | "height"> {
  return {
    scales: { y: { range: [0, 100] } },
    axes: [{}, { values: (_u, vals) => vals.map((v) => `${v}%`) }],
    series: [
      {},
      {
        label,
        stroke: color,
        width: 1.5,
        fill: color.replace(/^oklch\((\d+)%/, "oklch($1% / 0.12)"),
        points: { show: false },
      },
    ],
    legend: { show: true },
  };
}

function loadOpts(): Omit<Options, "width" | "height"> {
  return {
    series: [
      {},
      { label: "1m",  stroke: COLOR.load1,  width: 1.5, points: { show: false } },
      { label: "5m",  stroke: COLOR.load5,  width: 1.25, points: { show: false } },
      { label: "15m", stroke: COLOR.load15, width: 1.25, points: { show: false } },
    ],
    legend: { show: true },
  };
}
