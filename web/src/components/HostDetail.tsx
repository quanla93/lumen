import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { AlignedData, Options } from "uplot";
import {
  hostsApi,
  ApiError,
  type MetricsResponse,
  type MetricPoint,
} from "@/lib/api";
import { UPlotChart } from "@/components/UPlotChart";
import type { Snapshot, ContainerInfo } from "@/components/HostCard";

type Range = "1h" | "6h" | "24h";

const RANGE_SECONDS: Record<Range, number> = {
  "1h": 60 * 60,
  "6h": 6 * 60 * 60,
  "24h": 24 * 60 * 60,
};

const REFRESH_MS = 30_000;

// Series strokes. uPlot draws to canvas, so colors are baked at construction
// time; on theme toggle the whole chart remounts via themeKey so they re-resolve.
const COLOR = {
  cpu:    "oklch(70% 0.16 145)",  // green
  ram:    "oklch(68% 0.13 240)",  // blue
  disk:   "oklch(75% 0.16 75)",   // amber
  load1:  "oklch(65% 0.22 30)",   // red
  load5:  "oklch(68% 0.14 200)",  // teal
  load15: "oklch(62% 0.12 290)",  // purple
  netRx:  "oklch(70% 0.16 145)",  // green
  netTx:  "oklch(65% 0.22 30)",   // red
  diskR:  "oklch(68% 0.13 240)",  // blue
  diskW:  "oklch(75% 0.16 75)",   // amber
  temp:   "oklch(65% 0.22 30)",   // red
};

// themeColors reads runtime CSS vars so uPlot axes adapt to dark/light.
// Called fresh inside each opts builder to avoid baking a stale palette.
function themeColors() {
  const s = getComputedStyle(document.documentElement);
  return {
    muted: s.getPropertyValue("--color-muted").trim() || "#888",
    border: s.getPropertyValue("--color-border").trim() || "#ddd",
  };
}

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

  // themeKey changes when the user toggles dark/light. uPlot charts mount
  // their canvas paints with the colors that were active at construction;
  // forcing a key change remounts them with the new palette.
  const themeKey = useThemeKey();

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

  useEffect(() => {
    const scheme = window.location.protocol === "https:" ? "wss" : "ws";
    const ws = new WebSocket(`${scheme}://${window.location.host}/api/stream`);
    // Narrow the firehose to just this host once the socket opens.
    // The hub falls back to broadcasting everything if we never send
    // a subscribe frame, so the dashboard view (no subscribe) keeps
    // working unchanged. See internal/hub/stream/handler.go.
    ws.addEventListener("open", () => {
      try {
        ws.send(JSON.stringify({ type: "subscribe", hosts: [hostName] }));
      } catch { /* socket may have closed in the meantime */ }
    });
    ws.addEventListener("message", (e) => {
      try {
        const arr = JSON.parse(e.data as string) as Snapshot[];
        const match = arr.find((s) => s.host === hostName);
        if (match) setLive(match);
      } catch { /* ignore malformed */ }
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
  // Prefer the live WS value for "current"; fall back to last historical point.
  const last = useMemo<Partial<MetricPoint & Snapshot> | null>(() => {
    if (live) return live;
    if (resp && resp.points.length > 0) return resp.points[resp.points.length - 1];
    return null;
  }, [live, resp]);

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
            <RangeButton key={r} active={r === range} onClick={() => setRange(r)}>
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
          <ChartCard
            title="CPU"
            badges={[swatch(COLOR.cpu, `${(last?.cpu_pct ?? 0).toFixed(1)}%`)]}
          >
            <UPlotChart
              key={`cpu-${themeKey}`}
              data={data.cpu}
              options={percentOpts(COLOR.cpu)}
              className="h-[220px] w-full"
            />
          </ChartCard>
          <ChartCard
            title="RAM"
            badges={[swatch(COLOR.ram, `${(last?.ram_pct ?? 0).toFixed(1)}%`)]}
          >
            <UPlotChart
              key={`ram-${themeKey}`}
              data={data.ram}
              options={percentOpts(COLOR.ram)}
              className="h-[220px] w-full"
            />
          </ChartCard>
          <ChartCard
            title="Disk"
            badges={[swatch(COLOR.disk, `${(last?.disk_pct ?? 0).toFixed(1)}%`)]}
          >
            <UPlotChart
              key={`disk-${themeKey}`}
              data={data.disk}
              options={percentOpts(COLOR.disk)}
              className="h-[220px] w-full"
            />
          </ChartCard>
          <ChartCard
            title="Load avg"
            badges={[
              swatch(COLOR.load1, `1m ${(last?.load1 ?? 0).toFixed(2)}`),
              swatch(COLOR.load5, `5m ${(last?.load5 ?? 0).toFixed(2)}`),
              swatch(COLOR.load15, `15m ${(last?.load15 ?? 0).toFixed(2)}`),
            ]}
          >
            <UPlotChart
              key={`load-${themeKey}`}
              data={data.load}
              options={loadOpts()}
              className="h-[220px] w-full"
            />
          </ChartCard>
          <ChartCard
            title="Network"
            badges={[
              swatch(COLOR.netRx, `↓ ${formatBps(last?.net_rx_bps ?? 0)}`),
              swatch(COLOR.netTx, `↑ ${formatBps(last?.net_tx_bps ?? 0)}`),
            ]}
          >
            <UPlotChart
              key={`net-${themeKey}`}
              data={data.net}
              options={bpsOpts([COLOR.netRx, COLOR.netTx])}
              className="h-[220px] w-full"
            />
          </ChartCard>
          <ChartCard
            title="Disk I/O"
            badges={[
              swatch(COLOR.diskR, `read ${formatBps(last?.disk_r_bps ?? 0)}`),
              swatch(COLOR.diskW, `write ${formatBps(last?.disk_w_bps ?? 0)}`),
            ]}
          >
            <UPlotChart
              key={`dio-${themeKey}`}
              data={data.diskIO}
              options={bpsOpts([COLOR.diskR, COLOR.diskW])}
              className="h-[220px] w-full"
            />
          </ChartCard>
          {hasTemp && (
            <ChartCard
              title="Temperature"
              badges={[swatch(COLOR.temp, `${(last?.temp_c ?? 0).toFixed(1)}°C`)]}
            >
              <UPlotChart
                key={`temp-${themeKey}`}
                data={data.temp}
                options={tempOpts()}
                className="h-[220px] w-full"
              />
            </ChartCard>
          )}
        </div>
      )}

      {live?.containers && live.containers.length > 0 && (
        <ContainersTable containers={live.containers} />
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

// ContainersTable lists every Docker container the agent reported in the
// live snapshot. Live-only (no historical query); sorted: running first,
// then alphabetical, so the top of the list is always the things actually
// burning CPU/RAM right now.
function ContainersTable({ containers }: { containers: ContainerInfo[] }) {
  const sorted = useMemo(() => {
    return [...containers].sort((a, b) => {
      if (a.state === "running" && b.state !== "running") return -1;
      if (a.state !== "running" && b.state === "running") return 1;
      return a.name.localeCompare(b.name);
    });
  }, [containers]);
  const running = sorted.filter((c) => c.state === "running").length;
  return (
    <section className="mt-6 rounded-lg border border-[color:var(--color-border)] bg-[color:var(--color-card)] shadow-sm">
      <header className="flex items-center justify-between px-4 py-3 border-b border-[color:var(--color-border)]">
        <span className="text-xs uppercase tracking-wide text-[color:var(--color-muted)]">
          Containers · {containers.length} total
        </span>
        <span className="font-mono text-xs">
          <span className="text-[color:var(--color-muted)]">running</span>{" "}
          {running}
        </span>
      </header>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-[10px] uppercase tracking-wide text-[color:var(--color-muted)]">
              <th className="px-4 py-2 font-normal">Name</th>
              <th className="px-2 py-2 font-normal">State</th>
              <th className="px-2 py-2 font-normal">Image</th>
              <th className="px-2 py-2 font-normal text-right">CPU</th>
              <th className="px-4 py-2 font-normal text-right">Memory</th>
            </tr>
          </thead>
          <tbody>
            {sorted.map((c) => (
              <ContainerRow key={c.id} c={c} />
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function ContainerRow({ c }: { c: ContainerInfo }) {
  const dim = c.state !== "running";
  return (
    <tr
      className={`border-t border-[color:var(--color-border)] ${dim ? "opacity-60" : ""}`}
    >
      <td className="px-4 py-2 font-mono text-xs">
        <div>{c.name}</div>
        <div className="text-[10px] text-[color:var(--color-muted)]">
          {c.id}
        </div>
      </td>
      <td className="px-2 py-2">
        <StateBadge state={c.state} />
      </td>
      <td className="px-2 py-2 font-mono text-xs text-[color:var(--color-muted)] truncate max-w-[260px]">
        {c.image}
      </td>
      <td className="px-2 py-2 text-right font-mono text-xs tabular-nums">
        {c.state === "running" ? `${c.cpu_pct.toFixed(1)}%` : "—"}
      </td>
      <td className="px-4 py-2 text-right font-mono text-xs tabular-nums">
        {c.state === "running" ? (
          <div className="flex items-center justify-end gap-2">
            <span>{formatBytes(c.mem_used_bytes)}</span>
            <span className="text-[color:var(--color-muted)]">
              / {formatBytes(c.mem_limit_bytes)}
            </span>
            <span
              className={
                c.mem_pct >= 90
                  ? "text-[color:var(--color-danger)]"
                  : c.mem_pct >= 70
                  ? "text-[color:var(--color-warn)]"
                  : ""
              }
            >
              ({c.mem_pct.toFixed(0)}%)
            </span>
          </div>
        ) : (
          "—"
        )}
      </td>
    </tr>
  );
}

function StateBadge({ state }: { state: string }) {
  const tone =
    state === "running" ? "lumen-status-ok"
    : state === "paused" ? "lumen-status-warn"
    : state === "restarting" ? "lumen-status-warn"
    : "lumen-status-muted";
  return (
    <span className="inline-flex items-center gap-1.5 font-mono text-xs">
      <span aria-hidden className={`inline-block h-2 w-2 rounded-full ${tone}`} />
      {state}
    </span>
  );
}

function formatBytes(b: number): string {
  if (!b) return "0 B";
  const abs = Math.abs(b);
  if (abs >= 1024 * 1024 * 1024) return `${(b / (1024 * 1024 * 1024)).toFixed(2)} GiB`;
  if (abs >= 1024 * 1024)        return `${(b / (1024 * 1024)).toFixed(1)} MiB`;
  if (abs >= 1024)               return `${(b / 1024).toFixed(0)} KiB`;
  return `${b.toFixed(0)} B`;
}

// PerCoreStrip renders one tile per logical core in an auto-wrapping grid.
// Tile width adapts to core count so 1-core VMs don't look empty and
// 64-core servers don't paginate forever:
//   ≤ 8 cores  → wide tiles with idx + percentage label below
//   ≤ 32 cores → medium tiles, percentage label only
//   > 32 cores → compact tiles (no labels, just colored fill)
// Empty tracks stay visible at 0% so the operator can always count cores.
function PerCoreStrip({ cores }: { cores: number[] }) {
  const n = cores.length;
  const avg = cores.reduce((a, b) => a + b, 0) / n;

  const layout = n <= 8
    ? { tile: 64, height: 56, labels: "full" as const }
    : n <= 32
    ? { tile: 44, height: 48, labels: "pct" as const }
    : n <= 64
    ? { tile: 28, height: 40, labels: "none" as const }
    : { tile: 18, height: 32, labels: "none" as const };

  return (
    <div className="mb-4 rounded-lg border border-[color:var(--color-border)] bg-[color:var(--color-card)] px-4 py-3 shadow-sm">
      <div className="mb-3 flex items-center justify-between">
        <span className="text-xs uppercase tracking-wide text-[color:var(--color-muted)]">
          per-core CPU · {n} core{n === 1 ? "" : "s"}
        </span>
        <span className="font-mono text-xs">
          <span className="text-[color:var(--color-muted)]">avg</span>{" "}
          {avg.toFixed(1)}%
        </span>
      </div>
      <div
        className="grid gap-1.5"
        style={{ gridTemplateColumns: `repeat(auto-fit, ${layout.tile}px)` }}
      >
        {cores.map((pct, i) => (
          <CoreTile
            key={i}
            idx={i}
            pct={pct}
            height={layout.height}
            labels={layout.labels}
          />
        ))}
      </div>
    </div>
  );
}

function CoreTile({
  idx,
  pct,
  height,
  labels,
}: {
  idx: number;
  pct: number;
  height: number;
  labels: "full" | "pct" | "none";
}) {
  const tone = pct >= 90 ? "danger" : pct >= 60 ? "warn" : "ok";
  const toneClass =
    tone === "danger" ? "lumen-status-danger"
    : tone === "warn" ? "lumen-status-warn"
    : "lumen-status-ok";
  const fillPct = Math.min(100, pct);

  return (
    <div
      className="flex flex-col items-center gap-1"
      title={`core ${idx} · ${pct.toFixed(1)}%`} /* tooltip works even in compact mode */
    >
      <div
        className="relative w-full rounded-sm border border-[color:var(--color-border)] bg-[color:var(--color-bg)] overflow-hidden"
        style={{ height: `${height}px` }}
      >
        {pct > 0 && (
          <div
            className={`absolute bottom-0 left-0 right-0 ${toneClass} opacity-85 transition-[height] duration-300`}
            style={{ height: `${fillPct}%` }}
          />
        )}
      </div>
      {labels === "full" && (
        <div className="flex w-full items-center justify-between font-mono text-[10px] tabular-nums">
          <span className="text-[color:var(--color-muted)]">{idx}</span>
          <span>{pct.toFixed(0)}%</span>
        </div>
      )}
      {labels === "pct" && (
        <span className="font-mono text-[10px] tabular-nums">
          {pct.toFixed(0)}
        </span>
      )}
      {/* labels === "none" → no text; hover tooltip on the wrapper still surfaces idx + pct */}
    </div>
  );
}

function ChartCard({
  title,
  badges,
  children,
}: {
  title: string;
  badges?: React.ReactNode[];
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-lg border border-[color:var(--color-border)] bg-[color:var(--color-card)] p-3 shadow-sm">
      <div className="mb-2 flex items-center justify-between gap-2 flex-wrap">
        <span className="text-xs uppercase tracking-wide text-[color:var(--color-muted)]">
          {title}
        </span>
        {badges && badges.length > 0 && (
          <div className="flex items-center gap-3 font-mono text-xs tabular-nums">
            {badges.map((b, i) => <span key={i}>{b}</span>)}
          </div>
        )}
      </div>
      {children}
    </div>
  );
}

function swatch(color: string, text: string) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span
        aria-hidden
        className="inline-block h-2 w-2 rounded-[2px]"
        style={{ backgroundColor: color }}
      />
      {text}
    </span>
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

// Theme-aware base options. uPlot uses axis.stroke for tick labels and
// axis.grid.stroke for the faint guide lines; defaults are black/grey
// which look invisible in dark mode. We resolve the CSS vars at chart
// build time and remount on theme toggle (see themeKey).
function baseAxes(yValues?: (u: uPlot, vals: number[]) => string[], leftSize = 50) {
  const t = themeColors();
  return [
    { stroke: t.muted, grid: { stroke: t.border, width: 1 }, ticks: { stroke: t.border } },
    {
      stroke: t.muted,
      grid: { stroke: t.border, width: 1 },
      ticks: { stroke: t.border },
      size: leftSize,
      ...(yValues ? { values: yValues } : {}),
    },
  ];
}

function percentOpts(color: string): Omit<Options, "width" | "height"> {
  return {
    scales: { y: { range: [0, 100] } },
    axes: baseAxes((_u, vals) => vals.map((v) => `${v}%`), 44),
    legend: { show: false },
    series: [
      {},
      {
        stroke: color,
        width: 1.75,
        fill: color.replace(/^oklch\((\d+)%/, "oklch($1% / 0.14)"),
        points: { show: false },
      },
    ],
  };
}

function loadOpts(): Omit<Options, "width" | "height"> {
  return {
    axes: baseAxes(undefined, 44),
    legend: { show: false },
    series: [
      {},
      { stroke: COLOR.load1,  width: 1.75, points: { show: false } },
      { stroke: COLOR.load5,  width: 1.5,  points: { show: false } },
      { stroke: COLOR.load15, width: 1.5,  points: { show: false } },
    ],
  };
}

function bpsOpts(colors: [string, string]): Omit<Options, "width" | "height"> {
  return {
    axes: baseAxes((_u, vals) => vals.map((v) => formatBps(v)), 80),
    legend: { show: false },
    series: [
      {},
      { stroke: colors[0], width: 1.75, points: { show: false } },
      { stroke: colors[1], width: 1.75, points: { show: false } },
    ],
  };
}

function tempOpts(): Omit<Options, "width" | "height"> {
  return {
    axes: baseAxes((_u, vals) => vals.map((v) => `${v}°`), 44),
    legend: { show: false },
    series: [
      {},
      { stroke: COLOR.temp, width: 1.75, points: { show: false } },
    ],
  };
}

function formatBps(bps: number): string {
  const abs = Math.abs(bps);
  if (abs >= 1_000_000_000) return `${(bps / 1_000_000_000).toFixed(2)} GB/s`;
  if (abs >= 1_000_000)     return `${(bps / 1_000_000).toFixed(2)} MB/s`;
  if (abs >= 1_000)         return `${(bps / 1_000).toFixed(1)} kB/s`;
  return `${bps.toFixed(0)} B/s`;
}

// useThemeKey returns a counter that bumps whenever the `dark` class is
// added/removed on <html>. Components key off this to force-remount
// canvas-backed children (uPlot) so they re-read CSS vars.
function useThemeKey(): number {
  const [k, setK] = useState(0);
  useEffect(() => {
    const obs = new MutationObserver(() => setK((x) => x + 1));
    obs.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["class"],
    });
    return () => obs.disconnect();
  }, []);
  return k;
}

// Pull in uPlot type only for the function signature above; not exported.
import type uPlot from "uplot";
