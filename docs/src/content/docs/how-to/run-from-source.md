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

- **Go 1.25+** (`go version`) — floored by pressly/goose v3 (added in Phase 2 storage).
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

## First-time setup (auth flow)

The hub validates each agent's Bearer token against the `hosts` table.
For a brand-new hub:

1. Start the hub (any mode below).
2. Open **http://localhost:8090** (or `:5173` for Vite dev).
3. **Sign in.** Two options:
   - **Seeded admin** — if `LUMEN_HUB_ADMIN_USERNAME` + `_PASSWORD` are
     set in `.env`, the hub created that user on first boot (look for
     `seed admin created` in the log). Just log in.
   - **Register first admin via UI** — if the seed vars are empty, the
     UI shows a register form while the users table is empty.
4. Go to **Settings → Hosts**, type a host name, click **Create**.
5. Copy the `lum_…` token shown **once** (paired with a ready-to-paste
   `.env` snippet). The hub stores only its SHA-256 hash; if you lose it,
   rotate.
6. Paste `LUMEN_AGENT_TOKEN=lum_…` (and `LUMEN_HUB_URL`, `LUMEN_AGENT_HOST`)
   into the agent's `.env`, then start the agent.

The dashboard shows the host live within one tick. The agent's
`LUMEN_AGENT_HOST` is overridden by the token's host name server-side,
so a leaked token can't be used to spoof a different host.

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
| `LUMEN_HUB_DB_PATH` | `./lumen.db` | SQLite file (auto-created; WAL pragmas applied). |
| `LUMEN_HUB_SECRET` | _random per startup_ | Hex-encoded HMAC secret for session JWTs (>=64 hex chars). If unset, hub generates one at boot and **all sessions die on restart** — set this in prod. Generate with `openssl rand -hex 32`. |
| `LUMEN_HUB_RETENTION_WINDOW` | `24h` | Snapshots older than `now − WINDOW` are pruned on every sweep. Set to `0` to disable. |
| `LUMEN_HUB_RETENTION_INTERVAL` | `1h` | Retention sweep cadence. Set to `0` to disable. |
| `LUMEN_HUB_AGENT_INTERVAL` | `5s` | Runtime policy for agent collection cadence. Seeds the DB on first hub startup; later edits happen in Settings → Runtime. |
| `LUMEN_HUB_ADMIN_USERNAME` | _empty_ | Seed admin username. On every boot, if this user doesn't exist yet, the hub creates it with the password below. Existing users are left alone — passwords changed via UI survive restart. |
| `LUMEN_HUB_ADMIN_PASSWORD` | _empty_ | Plaintext password for the seed admin (Argon2id-hashed at write time). Both vars must be set together; both empty disables the seed and you have to register the first admin via the UI. |

### Agent

| Var | Default | Meaning |
|---|---|---|
| `LUMEN_HUB_URL` | `http://localhost:8090` | Hub base URL (no trailing slash). |
| `LUMEN_AGENT_TOKEN` | _empty_ | Per-host bearer token minted in Settings → Hosts. Required by strict ingest. |
| `LUMEN_AGENT_INTERVAL` | `5s` | Bootstrap sample interval. After connect, the hub Runtime setting can override it without redeploying the agent. |
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
