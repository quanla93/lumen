---
title: Run from source
description: All four ways to run a pre-v0.1 Lumen hub and agent locally — Go-only, Vite dev, embedded build, Docker Compose.
sidebar:
  order: 1
---

This page lists every supported way to run Lumen today (pre-v0.1). Pick
the one that matches what you're working on.

| Mode | Use when | Pros | Cons |
|---|---|---|---|
| [Go-only (no web)](#1-go-only) | You're hacking on hub or agent and don't care about the UI | Fastest loop (no node), `go run` only | No web at `/`; you'll see the fallback page |
| [Go + Vite dev](#2-go--vite-dev) | You're working on the React UI | HMR, source maps, fast iteration | Three terminals |
| [Embedded build](#3-embedded-build-1-binary) | You want the "one binary" experience | Single artifact, prod-like | Slowest loop — rebuild on every web change |
| [Docker Compose](#4-docker-compose) | You want to demo the full stack or build for CI | One command, isolated, distroless | Slower start (first build ~3 min) |

## Prerequisites

- **Go 1.24+** (`go version`) — floored by gopsutil/v4.
- **Node 20+** and **pnpm 9+** (`pnpm --version`) — only for modes that touch the web bundle.
- **Docker** with Compose v2 (`docker compose version`) — only for the Compose mode.
- **Git** to clone.

## Clone and configure

```bash
git clone https://github.com/quanla93/lumen
cd lumen
cp .env.example .env
```

`.env` is gitignored. Edit it freely; both binaries pick env vars up from
the file at startup. See [the env reference](#env-vars) at the bottom.

---

## 1. Go-only

Fastest loop for backend work. The hub will serve a "web not embedded"
fallback page at `/` — that's expected.

```bash
# Terminal 1
go run ./cmd/lumen-hub

# Terminal 2
go run ./cmd/lumen-agent
```

Verify:

```bash
curl http://localhost:8090/healthz
# {"status":"ok"}
```

You should see the agent log `ingested cpu_pct=...` lines every
`LUMEN_AGENT_INTERVAL` (default 5s) and the hub log a `POST /api/ingest`
204 for each.

## 2. Go + Vite dev

The right mode for UI iteration. Vite dev server runs on `:5173` and
proxies `/api/*` and `/healthz` to the Go hub on `:8090`.

```bash
pnpm install   # once

# Terminal 1
go run ./cmd/lumen-hub

# Terminal 2
go run ./cmd/lumen-agent

# Terminal 3
pnpm --filter web dev
```

Open **http://localhost:5173** — you'll see the live CPU table, updated
on every WebSocket tick (`LUMEN_HUB_STREAM_INTERVAL`, default 5s). React
HMR works; edit `web/src/App.tsx` and the browser updates without reload.

## 3. Embedded build (1 binary)

Closest to what the v0.1 release will look like. Builds the web bundle,
stages it into the hub's embed directory, then compiles a self-contained
hub binary.

```bash
pnpm install                              # once
pnpm --filter web build                   # builds web/dist/
rm -rf internal/hub/web/dist
mkdir -p internal/hub/web/dist
cp -r web/dist/. internal/hub/web/dist/   # stage for go:embed
touch internal/hub/web/dist/.gitkeep      # restore the placeholder

go build -o bin/lumen-hub ./cmd/lumen-hub
go build -o bin/lumen-agent ./cmd/lumen-agent

./bin/lumen-hub &
./bin/lumen-agent
```

On a Unix-y shell, `make build` does the whole pipeline in one command.
On Windows, run the lines above directly (the Makefile expects bash).

Open **http://localhost:8090** — same UI, but served from inside the
single binary. No Vite, no node at runtime.

## 4. Docker Compose

One command spins up hub + one agent in distroless containers.

```bash
docker compose -f deploy/docker/docker-compose.yml up --build
```

First run takes ~3 minutes (pulls golang, node, distroless images and
runs both stages). Subsequent runs are seconds thanks to layer caching.

Open **http://localhost:8090**. The hub container exposes :8090; the
agent container talks to it via the compose DNS name `hub:8090`.

Stop and clean up:

```bash
docker compose -f deploy/docker/docker-compose.yml down --rmi local
```

The agent inside the container reports cgroup-restricted CPU (so values
look low). To monitor the *host* CPU from a containerized agent you'd
need `--pid=host` + bind-mount `/proc:/host/proc:ro` and set
`HOST_PROC=/host/proc`. That's a Phase-2 concern.

---

## Testing the API

Two import-ready spec files live under [`api/`](https://github.com/quanla93/lumen/tree/main/api):

- **`api/openapi.yaml`** — OpenAPI 3.1. Import into Postman, Insomnia,
  Bruno, Hoppscotch, Stoplight, etc.
- **`api/lumen.http`** — REST Client format for VS Code (humao.rest-client),
  JetBrains HTTP Client, and Visual Studio.

Both cover `GET /healthz`, `POST /api/ingest`, and document `GET /api/stream`
(WebSocket — drive with the snippets at the bottom of `lumen.http`).

---

## Env vars

All knobs live in `.env`. Defaults shown.

### Hub

| Var | Default | Meaning |
|---|---|---|
| `LUMEN_HUB_ADDR` | `:8090` | Bind address. |
| `LUMEN_HUB_DEV` | `false` | Verbose request logs + slog DEBUG level. |
| `LUMEN_HUB_STREAM_INTERVAL` | `5s` | Cadence at which `/api/stream` re-sends the snapshot. |

### Agent

| Var | Default | Meaning |
|---|---|---|
| `LUMEN_HUB_URL` | `http://localhost:8090` | Hub base URL (no trailing slash). |
| `LUMEN_AGENT_TOKEN` | _empty_ | Bearer token (accepted but not validated until Phase 2). |
| `LUMEN_AGENT_INTERVAL` | `5s` | How often the agent samples + POSTs. |
| `LUMEN_AGENT_HOST` | `os.Hostname()` | Override the host identifier. |

---

## Troubleshooting

**`bind: address already in use`**
: Something else is on `:8090`. Either stop it or change `LUMEN_HUB_ADDR`
to `:9090` (and `LUMEN_HUB_URL` to match).

**Hub at `/` shows "web bundle not embedded"**
: You built the hub without staging the web bundle into
`internal/hub/web/dist/`. Either use mode 2 (Vite dev) or follow mode 3
to stage + rebuild.

**Agent logs `ingest send failed: connection refused`**
: Hub isn't listening. Check `LUMEN_HUB_URL` matches the hub's
`LUMEN_HUB_ADDR`, and that the hub terminal is still running.

**`go: command not found` (after winget install on Windows)**
: The MSI installer set the system PATH, but your current shell
inherited the old one. Open a fresh terminal.

**Docker Compose: `bind: address already in use`**
: Same as above but for the compose port mapping. Edit the `ports:`
entry in `deploy/docker/docker-compose.yml`.
