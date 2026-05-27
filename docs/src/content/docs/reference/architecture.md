---
title: Architecture
description: How the hub, agents, storage, and UI fit together — and where the seams are deliberate.
sidebar:
  order: 1
---

Lumen is two Go binaries and a static web bundle:

```
┌────────────────────────────────────────────────────────────────────┐
│                                                                    │
│                          host being monitored                      │
│   ┌──────────────────┐                                             │
│   │  lumen-agent     │ ─────── reads ──────── /proc, /sys, gopsutil│
│   │                  │ ─────── reads ──────── /var/run/docker.sock │
│   │                  │                                             │
│   │  buffer.db ──┐   │                                             │
│   └─────┬────────┴───┘                                             │
└─────────┼──────────────────────────────────────────────────────────┘
          │ HTTPS POST /api/ingest    (or buffer + retry)
          │ Authorization: Bearer lum_…
          ▼
┌────────────────────────────────────────────────────────────────────┐
│                          lumen-hub (one box)                       │
│                                                                    │
│   ┌────────────┐   put    ┌──────────────────┐                     │
│   │ ingest     │ ───────► │ in-memory store  │ ◄──── /api/stream   │
│   │ handler    │          │ (per-host ring)  │       WebSocket fan │
│   └─────┬──────┘          └────────┬─────────┘        out to UI    │
│         │ Add                       │                              │
│         ▼                           ▼                              │
│   ┌────────────┐   60s flush  ┌──────────────────┐                 │
│   │ batcher    │ ───────────► │  SQLite (WAL)    │                 │
│   │ (channel)  │              │  snapshots tbl   │                 │
│   └────────────┘              └────────┬─────────┘                 │
│                                        │                           │
│                                        ▼                           │
│                              ┌──────────────────┐                  │
│                              │ retention loop   │                  │
│                              │ (heartbeat 30s)  │                  │
│                              └──────────────────┘                  │
│                                                                    │
│   ┌──────────────────────┐                                         │
│   │ embedded web bundle  │ ←──────── browser ── /  (SPA)           │
│   │ //go:embed dist/     │                                         │
│   └──────────────────────┘                                         │
└────────────────────────────────────────────────────────────────────┘
```

## Components

### Agent (`cmd/lumen-agent`)

A small Go binary that runs as `root` on every monitored host and
collects metrics every 5 seconds (configurable). Reads:

- `/proc/*` — CPU, memory, load average, swap (via gopsutil v4)
- `/sys/class/hwmon/*` — temperatures
- Network + disk counters (cumulative; the agent diffs across ticks
  to produce bytes-per-second rates)
- `/var/run/docker.sock` — running containers + per-container CPU/RAM
  (via a minimal HTTP-over-unix-socket client, no docker/docker SDK)

POSTs `application/json` envelopes to the hub's `/api/ingest`
endpoint with a per-host bearer token. On failure, frames go into a
local bbolt file (`buffer.db`) and replay gradually after the hub
comes back.

### Hub (`cmd/lumen-hub`)

A single Go binary with the web UI embedded via `//go:embed`. Owns:

- **HTTP server** — chi router, REST endpoints under `/api/*`,
  WebSocket fan-out at `/api/stream`, static SPA at `/`.
- **In-memory store** — the hot path. Every accepted ingest replaces
  that host's latest snapshot and appends to a 120-sample CPU ring
  (10 min of history per host @ 5s ticks, ~1 KB RAM per host).
- **Batcher** — coalesces snapshot INSERTs into one transaction every
  60 s (or 5000 rows). HDD-friendly.
- **SQLite (WAL)** — the persistent layer. WAL + `synchronous=NORMAL`
  + 5 s busy timeout. One file at `/var/lib/lumen/lumen.db`.
- **Retention loop** — 30 s heartbeat re-reads window + interval from
  the `settings` table; sweeps when `time.Since(lastSweep) >=
  interval`. UI edits take effect within 30 s, no restart.
- **Auth** — Argon2id passwords + HS256 JWT in an HttpOnly cookie
  (30 d TTL). Strict bearer-token gate on `/api/ingest`.

### Web (`web/`)

React 18 + Vite + Tailwind v4 + uPlot. Built into a `dist/` bundle
that gets embedded into the hub binary. The browser connects to:

- `/api/*` for REST (login, host list, metric history, settings)
- `/api/stream` for WebSocket live updates (every 5 s, filterable by
  `subscribe` control frame)

### Docs (`docs/`)

Starlight (Astro) static site. Independent from the hub binary —
deployed separately to Cloudflare Pages or GitHub Pages. Not embedded.

## Data flow per ingest

```
agent tick (every 5s)
   ↓
collect() → IngestRequest{ host, ts, cpu_pct, ram_pct, ... }
   ↓
POST /api/ingest  (Authorization: Bearer lum_...)
   ↓ (if hub down) → buffer.Enqueue → retry next tick
   ↓
hub.ingest.Handler
   ↓
hosts.VerifyToken    →  reject 401 if token invalid
   ↓
validate ranges     →  reject 400 if cpu_pct > 100, etc.
   ↓                       ↓
store.Put (in-mem)         batcher.Add (queue → 60s flush → SQLite)
   ↓
WebSocket subscribers receive the update on the next 5s tick
   ↓
host detail page redraws its uPlot charts; per-core strip animates
```

The split between in-memory store and batched SQLite is deliberate:
the live view never blocks on disk, and a slow disk never blocks
ingest. The trade-off is up to one flush interval of data loss on a
hub crash, which the agent buffer replays on reconnect.

## Why this shape

**One binary per role.** No sidecar, no separate metrics service, no
external time-series DB. The hub is the time-series DB; SQLite + a
60 s flush ring is enough for homelab cardinality. Phase 4 adds a
cold-tier Parquet writer for queries beyond the configured hot window,
but the hot path stays the same.

**Push, not pull.** Agents POST to the hub. The hub never connects
out. Reasons:

- NAT-friendly — agents behind a router don't need port-forwarding.
- One credential per host (bearer token) instead of bidirectional
  trust setup.
- Differentiates Lumen from Beszel (SSH-based pull) and Netdata
  (cloud parent-child) — both excellent but assume infrastructure
  Lumen-class homelabs often don't have.

**Web bundle embedded, docs not.** The hub is meant to be a single
file you `scp` and run. Docs are meant to be public + searchable +
i18n; that's a different deploy target.

**Per-core CPU and containers are live-only.** They flow through the
WebSocket but aren't persisted to SQLite. Cardinality varies per
host and over time (containers come and go); storing them would
force a JSON column or a join table for modest analytical value.
Phase 5 may reverse this if user demand materializes.

## Threat model (high-level)

- **Untrusted network between agent and hub** → TLS in front of the
  hub (reverse proxy), bearer-token auth, strict ingest validation.
- **Untrusted browser** → session cookie is HttpOnly + Secure (under
  TLS) + SameSite=Lax. CSRF via SameSite + state-changing routes
  requiring a session.
- **Compromised agent host** → can spoof its own metrics; can't
  spoof another host (the bearer token's host name is the
  authoritative one, server-side lookup; body.host is overridden).
- **Compromised hub host** → game over. Operate the hub on a host
  you trust as much as the data you collect.

What Lumen does NOT protect against (today):

- Brute-force on the login endpoint (no rate limit yet — Phase 3).
- Replay of a captured ingest envelope (no nonce — accepted because
  the agent's local clock would have to be off by hours for this to
  matter on a homelab).
- Sniffing of `/install.sh` traffic if served over plain HTTP (the
  token is in the URL fragment + curl `-d` body) — always serve
  the hub over HTTPS.

## See also

- [API reference](/reference/api/) — every endpoint + WS frame
  format.
- [Metrics catalog](/reference/metrics-catalog/) — every field, its
  source, units, and semantics.
- [Reliability](/configure/reliability/) — how the agent buffer +
  hub batcher absorb outages.
- [Retention](/configure/retention/) — how old snapshots get pruned.
