---
title: Reliability
description: How Lumen survives hub restarts and network blips — agent-side offline buffer and hub-side batched persistence.
sidebar:
  order: 3
---

Lumen is designed for a homelab: one hub, a handful of agents on
LXC / VMs / bare metal, traffic over your home network. Two failure
modes are routine:

- **Hub restarts** — kernel patches, config edits, accidental reboot.
- **Network blips** — Wi-Fi roam, VLAN reload, switch reboot.

Out of the box the agent + hub absorb both without operator
intervention. This page documents *how*, and the two knobs that
control it.

## Agent offline buffer

Every agent has a small bbolt file (single-file embedded key/value
store, like SQLite-but-simpler) that holds ingest frames the hub
hasn't acknowledged yet. The flow:

```
tick → collect envelope
  │
  ├─ POST /api/ingest → 204 No Content
  │     └─ on success: drain up to 10 buffered frames
  │
  └─ POST /api/ingest → connection refused / 5xx
        └─ on failure: enqueue this frame; next tick tries again
```

The buffer is bounded by **age** (default 24h) and **count** (default
~17 280 = 24h × 12/min). Hit either ceiling and the oldest frames are
dropped on the next prune. Long outages → some data loss; short ones
→ zero loss.

### Configuration

| Env var | Default | What it does |
|---|---|---|
| `LUMEN_AGENT_BUFFER_PATH` | `./lumen-agent-buffer.db` | Where the bbolt file lives. Docker compose overrides to `/data/buffer.db` (persistent volume); the systemd installer points it at `/var/lib/lumen-agent/buffer.db`. |
| `LUMEN_AGENT_BUFFER_MAX_AGE` | `24h` | Frames older than this are pruned even if unsent. Avoids replaying stale data onto a chart when the hub comes back after a multi-day outage. |
| `LUMEN_AGENT_BUFFER_DRAIN` | `10` | Max frames to ship per successful tick. Higher = faster catch-up, lower = gentler on a recovering hub. |

### Operational signals

In agent logs you'll see:

```
INFO  agent starting  buffer_path=/data/buffer.db buffer_max_age=24h0m0s
INFO  buffer carries forward frames from previous run  queued=42
WARN  ingest failed — frame buffered  err="dial tcp: ... refused"  buffer_size=43
INFO  buffer drained  shipped=10 still_queued=33 drain_err=<nil>
```

The `WARN` lines are loud because they mean an outage is in progress
— but they're harmless: the agent will catch up automatically. If
they persist for >24h the buffer starts shedding old frames.

### Corruption recovery

If the bbolt file is mangled by a power-loss or disk fault, the agent
detects it at open time, renames the file aside with a
`.corrupt-<unix-timestamp>` suffix, and starts a fresh one. You lose
the buffered backlog but the agent keeps shipping live frames — far
better than crash-looping on a stuck file.

## Hub batch flush

The hub receives every ingest into an in-memory store (hot path —
WebSocket subscribers see the new value within milliseconds) and
*also* queues it for SQLite persistence. The persistence path is
batched, not synchronous:

```
ingest → store.Put (live)        ← hot path, sub-ms
       → batcher.Add (queue)     ← non-blocking, returns immediately
                                ↓
                  every 60s OR every 5000 rows
                                ↓
                BEGIN; INSERT * N; COMMIT  (one fsync)
```

### Why batch

SQLite in WAL mode with `synchronous=NORMAL` still fsyncs the WAL on
every `COMMIT`. On a 200-host fleet at 5s tick that's 40 fsyncs per
second, plus seek penalty on a spinning disk. Coalescing into one
transaction every 60s drops that to one fsync per minute regardless
of fleet size — the entire batch fits in a single sequential write to
the WAL.

The trade-off is **up to one flush interval of data lost on a hub
crash**. The agent-side buffer (above) replays it on reconnect, so
the practical failure mode is "delayed write, not lost write."

### Configuration

| Env var | Default | What it does |
|---|---|---|
| `LUMEN_HUB_BATCH_FLUSH_EVERY` | `60s` | How often the in-memory queue is flushed to SQLite. Drop to `5s` for development (immediate visibility in `lumen.db`); raise to `5m` for very large fleets on slow disks. |
| `LUMEN_HUB_BATCH_FLUSH_SIZE` | `5000` | Flush early once pending rows hit this, regardless of the interval. Caps memory at fleet scale; flush size × `~120 bytes` is the rough RAM ceiling. |

### Operational signals

In hub logs (`LUMEN_HUB_DEV=true` for Debug visibility):

```
INFO  batcher starting  flush_interval=1m0s flush_size=5000 queue_size=10000
DEBUG batch flushed  rows=240 reason=interval took=12ms
DEBUG batch flushed  rows=5000 reason=size took=68ms
WARN  batcher dropped snapshots due to full queue  dropped_since_last_flush=17
INFO  batcher stopped
```

The `WARN` line only fires if ingest pressure exceeds the queue size
between flushes. At default settings (10k queue, 60s flush) that's
~167 hosts/s sustained — well beyond what a homelab will produce. If
you see it, either raise `LUMEN_HUB_BATCH_FLUSH_SIZE` (smaller
flushes, faster turnover) or shorten `LUMEN_HUB_BATCH_FLUSH_EVERY`.

On graceful shutdown the batcher performs a final flush, bounded by
the same 10s deadline as the HTTP server. A SIGKILL skips this.

## WS subscribe protocol

The hub broadcasts host snapshots over `/api/stream` every
`LUMEN_HUB_STREAM_INTERVAL` (default 5s). Clients may optionally send
a control frame to narrow what they receive:

```json
// firehose (default if no message ever sent)
{}

// filter to a specific host
{"type":"subscribe","hosts":["webA"]}

// revert to firehose
{"type":"subscribe","hosts":["*"]}
```

The host detail page uses this to ignore the 99% of WS traffic that
isn't about the host it's showing. The dashboard view continues to
use the firehose default; nothing breaks for older web builds that
never send a subscribe frame.

This is per-connection state — no shared registry, no resubscribe on
reconnect needed (the next browser navigation re-opens the WS and
sends a fresh subscribe).
