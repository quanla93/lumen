---
title: Metrics catalog
description: Every metric the agent ships — source, unit, semantics, gotchas.
sidebar:
  order: 3
---

This page is the contract between the agent and everything that
consumes its data (hub, UI, future Prometheus exporter). When a
field's meaning is ambiguous, this page is the tie-breaker.

Two columns of "lives here":

| Column | Means |
|---|---|
| **Persisted** | Stored in the `snapshots` SQLite table, queryable via `GET /api/hosts/{id}/metrics`. |
| **Live-only** | Flows through `/api/stream` only. Lost on hub restart; the agent's next tick repopulates. |

## Core host metrics

All persisted. Reported every `LUMEN_AGENT_INTERVAL` (default 5 s).

### `cpu_pct` — aggregate CPU utilization

| | |
|---|---|
| **Source** | gopsutil `cpu.PercentWithContext(500ms, false)` |
| **Unit** | percent (0–100) |
| **Sample window** | 500 ms blocking sample at collection time |
| **Persisted** | yes |

Mirrors what `top` shows at the bottom — total CPU usage averaged
across all cores. A 4-core box pegged on one core reports `cpu_pct ≈
25`, not 100. Use `cpu_per_core` to see imbalance.

The 500 ms window is the smallest useful one — shorter and the
delta is too noisy; longer and the agent's own tick budget tightens.

### `cpu_per_core` — per-core CPU utilization

| | |
|---|---|
| **Source** | gopsutil `cpu.PercentWithContext(200ms, true)` |
| **Unit** | percent (0–100), one entry per logical CPU |
| **Cardinality** | matches `runtime.NumCPU()` on the agent host |
| **Persisted** | **no — live-only** |

The host detail page renders this as a per-core strip with 4 density
tiers (≤8 cores → individual bars, ≤16 → compressed, ≤32 → mini, >32
→ heatmap row). Cap is 256 cores per envelope at the hub validator.

Not stored because cardinality varies per host and over time
(hyperthread on/off, container CPU sets), and the analytical value
of per-core history pre-v1 is modest. Phase 5 may add a separate
table behind a feature flag.

### `ram_pct`, `swap_pct` — memory pressure

| | |
|---|---|
| **Source** | gopsutil `mem.VirtualMemoryWithContext` + `mem.SwapMemoryWithContext` |
| **Unit** | percent (0–100) |
| **Persisted** | yes |

`ram_pct` is `(total - available) / total * 100` — matches `free`'s
"used" excluding page cache. Means a Linux box with full page cache
reports a sane number, not 99%.

`swap_pct` is `used / total * 100`. A host with no swap configured
reports `0`. Sudden non-zero swap with steady ram_pct is the classic
"investigate, you might be near OOM" signal.

### `disk_pct` — filesystem fullness

| | |
|---|---|
| **Source** | gopsutil `disk.UsageWithContext(LUMEN_AGENT_DISK_PATH)` |
| **Unit** | percent (0–100) |
| **Path** | `LUMEN_AGENT_DISK_PATH`, defaults to `/` on Linux/macOS, `C:\` on Windows |
| **Persisted** | yes |

Only one filesystem per host. If you care about a non-root mount
(e.g. `/data`), set `LUMEN_AGENT_DISK_PATH=/data` in the agent env
or run a second agent with `LUMEN_AGENT_HOST=$host-data`. Phase 3
may add a per-mount-point list; the single-path field stays as the
"primary" disk forever.

### `load1`, `load5`, `load15` — load average

| | |
|---|---|
| **Source** | gopsutil `load.AvgWithContext` |
| **Unit** | runnable+uninterruptible processes (kernel-defined) |
| **Persisted** | yes |
| **Platform** | Linux + macOS only; Windows kernel doesn't expose this |

On Windows the agent logs Debug "load sample unavailable" and ships
zeros. The UI hides the load chart if all three are 0 across the
query window.

Interpretation: a 4-core machine with `load1 = 4.0` is at exactly
the saturation line. Above that, work is queuing.

## Rate metrics

All four are diffed from cumulative gopsutil counters across two
ticks. On the very first tick after agent start there's no prior
value — those fields are 0. On a counter wrap or reset (interface
flap, disk hot-swap) the next sample is also 0 rather than a
garbage huge value.

### `net_rx_bps`, `net_tx_bps` — network throughput

| | |
|---|---|
| **Source** | gopsutil `net.IOCountersWithContext(false)` summed across all interfaces |
| **Unit** | bytes per second |
| **Persisted** | yes |

Summed across every interface (loopback included — usually
negligible). The UI shows these as kB/s or MB/s depending on
magnitude.

### `disk_r_bps`, `disk_w_bps` — disk I/O throughput

| | |
|---|---|
| **Source** | gopsutil `disk.IOCountersWithContext()` summed across all devices |
| **Unit** | bytes per second |
| **Persisted** | yes |

Sum across all block devices reported by the kernel — physical
disks, partitions, dm devices, loop devices. May be higher than the
filesystem-visible write rate if dm/LVM is in the chain (double
counting).

## Optional metrics

### `temp_c` — hottest sensor temperature

| | |
|---|---|
| **Source** | gopsutil `host.SensorsTemperaturesWithContext`, prefers `coretemp` / `k10temp` |
| **Unit** | degrees Celsius |
| **Persisted** | yes |
| **Platform** | Linux only (reads `/sys/class/hwmon`); 0 elsewhere |

The agent picks the highest reading among `coretemp_*` and
`k10temp_*` sensors. Other sensor names are ignored to avoid e.g. a
disk SMART temperature dominating the chart.

In containers/LXC the `/sys/class/hwmon` path may not be exposed —
the value is 0 and the UI hides the temperature chart entirely.
Privileged LXC can bind-mount the path:

```
lxc.mount.entry: /sys/class/hwmon sys/class/hwmon none bind,optional,ro 0 0
```

### `containers` — Docker container snapshot

| | |
|---|---|
| **Source** | minimal HTTP client over `/var/run/docker.sock`, paths `/containers/json` + `/containers/{id}/stats?stream=false` |
| **Persisted** | **no — live-only** |
| **Platform** | Anywhere with a Docker socket reachable |
| **Cap** | 500 entries per envelope (hub-side validator) |

One row per running OR recently-exited container (Docker's "all"
filter, so a `state=exited` container shows once before the daemon
prunes it). Fields:

```ts
{
  id: string;            // short 12-char ID
  name: string;          // leading "/" stripped (Docker quirk)
  image: string;
  state: string;         // running | paused | exited | restarting | dead
  cpu_pct: number;       // delta math, matches `docker stats`
  mem_used_bytes: number;
  mem_limit_bytes: number;
  mem_pct: number;
}
```

The CPU% formula matches Docker stats exactly:

```
cpu_pct = (cpu_total_delta / system_total_delta) * online_cpus * 100
```

Memory is `usage - inactive_file` (subtracts page cache so a
container doing big sequential reads doesn't look full).

If the socket is missing: silent. If present but unreadable
(permissions): one Warn line at agent startup, then Debug — the
agent doesn't spam the log.

## Hub-side derived fields

The hub adds two fields to each `HostSnapshot` it broadcasts that
the agent doesn't send:

| Field | Purpose |
|---|---|
| `cpu_series` | Last ~120 CPU% samples (oldest first). Lets the dashboard render a sparkline on first paint without waiting for the next tick. |
| `last_seen_at` (in `/api/hosts`) | When the hub last accepted an ingest from this host. Drives the gray "stale" badge in the UI. |

Both are computed in-memory; neither survives a hub restart, but
both repopulate within one tick of agent activity.

## Wire envelope summary

What an agent POSTs to `/api/ingest`:

| Field | Persisted | Live-only | Notes |
|---|---|---|---|
| `host` | n/a — overwritten | n/a | Token's host name wins |
| `ts` | yes | — | RFC3339; UTC; nanoseconds OK |
| `cpu_pct` | yes | — | |
| `cpu_per_core` | — | yes | Variable cardinality |
| `ram_pct`, `swap_pct` | yes | — | |
| `disk_pct` | yes | — | One path |
| `load1`, `load5`, `load15` | yes | — | Windows = 0 |
| `net_rx_bps`, `net_tx_bps` | yes | — | All interfaces summed |
| `disk_r_bps`, `disk_w_bps` | yes | — | All block devices summed |
| `temp_c` | yes | — | 0 if no sensor |
| `containers[]` | — | yes | Cap 500 |

## Field stability

These names + units are the v0 contract. They won't change pre-1.0
without a deprecation note in the release announcement. New fields
are added as optional / zero-defaulted — older agents stay
compatible.
