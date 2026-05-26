import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { AlignedData, Options } from "uplot";
import {
  hostsApi,
  ApiError,
  type MetricsResponse,
} from "@/lib/api";
import { UPlotChart } from "@/components/UPlotChart";
import type { Snapshot } from "@/components/HostCard";

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
  cpu:    "oklch(70% 0.16 145)", // accent green
  ram:    "oklch(68% 0.13 240)", // blue
  disk:   "oklch(75% 0.16 75)",  // amber
  load1:  "oklch(65% 0.22 30)",  // red
  load5:  "oklch(68% 0.14 200)", // teal
  load15: "oklch(62% 0.12 290)", // purple
  netRx:  "oklch(70% 0.16 145)", // green for "in"
  netTx:  "oklch(65% 0.22 30)",  // red for "out"
  diskR:  "oklch(68% 0.13 240)", // blue for "read"
  diskW:  "oklch(75% 0.16 75)",  // amber for "write"
  temp:   "oklch(65% 0.22 30)",  // red
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
  const [live, setLive] = useState<Snapshot | null>(null);
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

  // WS subscription: picks out the live snapshot for THIS host so we can
  // show fields we don't persist historically (per-core CPU strip) and a
  // current-rate footer that updates faster than the 30s metrics refresh.
  useEffect(() => {
    const scheme = window.location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${scheme}://${window.location.host}/api/stream`);
    ws.addEventListener("message", (e) => {
      try {
        const arr = JSON.parse(e.data as string) as Snapshot[];
        const match = arr.find((s) => s.host === hostName);
        if (match) setLive(match);
      } catch {
        // ignore malformed
      }
    });
    return () => ws.close();
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
  const hasTemp = useMemo(
    () => !!resp?.points.some((p) => p.temp_c > 0),
    [resp],
  );

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

      {live?.cpu_per_core && live.cpu_per_core.length > 0 && (
        <PerCoreStrip cores={live.cpu_per_core} />
      )}

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
          <ChartCard title="Network (rx / tx)">
            <UPlotChart
              data={data.net}
              options={bpsOpts({ rx: "rx", tx: "tx" }, [COLOR.netRx, COLOR.netTx])}
              className="h-[220px] w-full"
            />
          </ChartCard>
          <ChartCard title="Disk I/O (read / write)">
            <UPlotChart
              data={data.diskIO}
              options={bpsOpts({ rx: "read", tx: "write" }, [COLOR.diskR, COLOR.diskW])}
              className="h-[220px] w-full"
            />
          </ChartCard>
          {hasTemp && (
            <ChartCard title="Temperature (°C)">
              <UPlotChart
                data={data.temp}
                options={tempOpts()}
                className="h-[220px] w-full"
              />
            </ChartCard>
          )}
        </div>
      )}

      {resp && (
        <p className="mt-6 text-xs text-[color:var(--color-muted)]">
          {resp.points.length} points · step {resp.step_seconds}s · refreshing
          every {Math.round(REFRESH_MS / 1000)}s
          {live && (
            <>
              {" · "}
              <span className="font-mono">
                now: ↓ {formatBps(live.net_rx_bps)} · ↑ {formatBps(live.net_tx_bps)}
                {" · "}
                read {formatBps(live.disk_r_bps)} · write {formatBps(live.disk_w_bps)}
                {live.temp_c > 0 && <> · {live.temp_c.toFixed(1)}°C</>}
              </span>
            </>
          )}
        </p>
      )}
    </>
  );
}

function PerCoreStrip({ cores }: { cores: number[] }) {
  return (
    <div className="mb-4 rounded-lg border border-[color:var(--color-border)] bg-[color:var(--color-card)] p-3 shadow-sm">
      <div className="mb-2 flex items-center justify-between">
        <span className="text-xs uppercase tracking-wide text-[color:var(--color-muted)]">
          per-core CPU · {cores.length} core{cores.length === 1 ? "" : "s"}
        </span>
        <span className="font-mono text-xs text-[color:var(--color-muted)]">
          avg {(cores.reduce((a, b) => a + b, 0) / cores.length).toFixed(1)}%
        </span>
      </div>
      <div
        className="grid gap-1"
        style={{
          gridTemplateColumns: `repeat(${Math.min(cores.length, 16)}, minmax(0, 1fr))`,
        }}
      >
        {cores.map((pct, i) => (
          <div key={i} className="flex flex-col items-center gap-0.5">
            <div className="h-10 w-full rounded-sm bg-[color:var(--color-border)] overflow-hidden flex flex-col-reverse">
              <div
                className={`w-full ${coreToneClass(pct)} transition-[height] duration-300`}
                style={{ height: `${Math.max(2, Math.min(100, pct))}%` }}
              />
            </div>
            <span className="font-mono text-[10px] text-[color:var(--color-muted)] tabular-nums">
              {pct.toFixed(0)}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

function coreToneClass(pct: number): string {
  if (pct >= 90) return "lumen-status-danger";
  if (pct >= 60) return "lumen-status-warn";
  return "lumen-status-ok";
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
    return { cpu: empty, ram: empty, disk: empty, load: empty, net: empty, diskIO: empty, temp: empty };
  }
  const xs = r.points.map((p) => Math.floor(new Date(p.ts).getTime() / 1000));
  return {
    cpu:    [xs, r.points.map((p) => p.cpu_pct)] as AlignedData,
    ram:    [xs, r.points.map((p) => p.ram_pct)] as AlignedData,
    disk:   [xs, r.points.map((p) => p.disk_pct)] as AlignedData,
    load: [
      xs,
      r.points.map((p) => p.load1),
      r.points.map((p) => p.load5),
      r.points.map((p) => p.load15),
    ] as AlignedData,
    net: [
      xs,
      r.points.map((p) => p.net_rx_bps),
      r.points.map((p) => p.net_tx_bps),
    ] as AlignedData,
    diskIO: [
      xs,
      r.points.map((p) => p.disk_r_bps),
      r.points.map((p) => p.disk_w_bps),
    ] as AlignedData,
    temp: [xs, r.points.map((p) => p.temp_c)] as AlignedData,
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

function bpsOpts(
  labels: { rx: string; tx: string },
  colors: [string, string],
): Omit<Options, "width" | "height"> {
  return {
    // Y-axis size 80 (default ~50) gives MB/s labels enough room not to
    // truncate to ".00 MB/s" when the chart is narrow.
    axes: [{}, { size: 80, values: (_u, vals) => vals.map((v) => formatBps(v)) }],
    series: [
      {},
      { label: labels.rx, stroke: colors[0], width: 1.5, points: { show: false }, value: (_u, v) => formatBps(v ?? 0) },
      { label: labels.tx, stroke: colors[1], width: 1.5, points: { show: false }, value: (_u, v) => formatBps(v ?? 0) },
    ],
    legend: { show: true },
  };
}

function tempOpts(): Omit<Options, "width" | "height"> {
  return {
    axes: [{}, { values: (_u, vals) => vals.map((v) => `${v}°`) }],
    series: [
      {},
      { label: "°C", stroke: COLOR.temp, width: 1.5, points: { show: false } },
    ],
    legend: { show: true },
  };
}

function formatBps(bps: number): string {
  const abs = Math.abs(bps);
  if (abs >= 1_000_000_000) return `${(bps / 1_000_000_000).toFixed(2)} GB/s`;
  if (abs >= 1_000_000)     return `${(bps / 1_000_000).toFixed(2)} MB/s`;
  if (abs >= 1_000)         return `${(bps / 1_000).toFixed(1)} kB/s`;
  return `${bps.toFixed(0)} B/s`;
}
