---
title: Quickstart
description: Build Lumen from source and see live CPU metrics in two terminals.
sidebar:
  order: 2
---

> ℹ️ This is the **pre-v0.1 dev quickstart** — you build the binaries from
> source. The polished `curl quanla.org/lumen/install | bash` flow lands with v0.1.0
> (see the [roadmap](https://github.com/quanla93/lumen/blob/main/ACTION_PLAN.md)).

## Prerequisites

- Go **1.25+** (`go version` to check)
- Git
- A terminal — Linux, macOS, or Windows (PowerShell or bash)

## 1. Clone and build

```bash
git clone https://github.com/quanla93/lumen
cd lumen
cp .env.example .env
```

`.env` is gitignored and holds all config (12-factor — there are no CLI
flags). Edit it if you want different ports or interval; the defaults are
fine for local testing.

## 2. Run the hub

In terminal **1**:

```bash
make dev-hub
# or:  go run ./cmd/lumen-hub
```

Expected output:

```
time=... level=INFO msg="hub listening" addr=:8090 dev=true
```

Sanity-check the liveness probe:

```bash
curl -i http://localhost:8090/healthz
# HTTP/1.1 200 OK
# Content-Type: application/json
#
# {"status":"ok"}
```

## 3. Run the agent

In terminal **2**:

```bash
make dev-agent
# or:  go run ./cmd/lumen-agent
```

You should see one log line per tick (default 5s, tunable in `.env` via
`LUMEN_AGENT_INTERVAL`):

```
time=... level=INFO msg="agent starting" hub=http://localhost:8090 host=<hostname> interval=5s
time=... level=INFO msg=ingested cpu_pct=12.34
time=... level=INFO msg=ingested cpu_pct=8.76
```

And the hub log (terminal 1) shows each ingest accepted:

```
time=... level=DEBUG msg="ingest accepted" host=<hostname> cpu_pct=12.34
```

## 4. (coming soon) Live dashboard

The web dashboard lands in Phase 1.6 of the [roadmap](https://github.com/quanla93/lumen/blob/main/ACTION_PLAN.md).
Until then, the hub's `POST /api/ingest` is the only ingest path, and
`GET /api/stream` (WebSocket) is the only realtime output. A minimal
React+Vite UI consuming `/api/stream` is the next milestone.

## Troubleshooting

**`go: command not found`**
: Install Go 1.24+ from [go.dev/dl](https://go.dev/dl/). On Windows, after
the MSI installer, open a fresh terminal so the new PATH is picked up.

**`bind: address already in use` on port 8090**
: Another process holds the port. Either stop it or change `LUMEN_HUB_ADDR`
in `.env` (e.g. `:9090`) and update `LUMEN_HUB_URL` to match.

**Agent logs `ingest send failed`**
: The hub isn't reachable. Verify `LUMEN_HUB_URL` matches `LUMEN_HUB_ADDR`
and that the hub terminal is still running.
