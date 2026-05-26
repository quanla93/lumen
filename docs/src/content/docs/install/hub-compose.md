---
title: Hub — Docker compose
description: Run the Lumen hub via Docker compose on any host with Docker.
sidebar:
  order: 3
---

Run hub + a same-host agent in two containers with one command. Use this
if Docker is already your deployment substrate or you want to demo the
full stack without touching systemd.

## Requirements

- Docker 20.10+ and Docker Compose v2 (`docker compose version`).
- 200 MiB free RAM, ~250 MiB disk.
- Outbound HTTPS (for the first build only).

## 1. Clone and configure

```bash
git clone https://github.com/quanla93/lumen
cd lumen
cp .env.example .env
```

Edit `.env` and set at minimum:

```ini
# Stable secret so sessions survive restart.
LUMEN_HUB_SECRET=<generate with: openssl rand -hex 32>

# Bootstrap admin. CHANGE THE PASSWORD before exposing the hub.
LUMEN_HUB_ADMIN_USERNAME=admin
LUMEN_HUB_ADMIN_PASSWORD=<your-choice>
```

The other variables are documented inline — defaults are sensible.

## 2. Bring up the stack

```bash
docker compose -f deploy/docker/docker-compose.yml up --build -d
```

First build takes ~3 min (pulls Node, Go, distroless; cross-compiles the
agent for three Linux arches so the hub can serve `/install.sh`). Later
runs use the layer cache and finish in seconds.

When `docker compose ps` shows both `lumen-hub` and `lumen-agent` as
`Up`, open `http://<this-host>:8090`.

## 3. Connect the agent (one-shot)

The agent in the compose stack starts without a token, so on first
boot it logs `401 Authorization: Bearer <token> required` every 5s.
That's expected — it's waiting for you to mint a token:

1. Sign in to the hub UI with `admin` / your password from step 1.
2. Settings → Hosts → Create `compose-agent` → copy the `lum_...` token.
3. Put the token in `.env`:

   ```bash
   sed -i "s|^LUMEN_AGENT_TOKEN=.*|LUMEN_AGENT_TOKEN=lum_xxxxxxxxxxxxxxxxxxxxx|" .env
   ```

4. Restart **only** the agent so it picks up the new env var:

   ```bash
   docker compose -f deploy/docker/docker-compose.yml up -d --force-recreate agent
   ```

The `compose-agent` card should appear on the dashboard within ~5s and
the 401s in the agent log stop.

## Stack details

| Service | Image | Memory | Ports | Mounts |
|---|---|---|---|---|
| `hub` | `lumen-hub:dev` (built locally) | <100 MiB | `8090:8090` | named volume `lumen-data` → `/data` |
| `agent` | `lumen-agent:dev` | <50 MiB | none | `/var/run/docker.sock:ro` (for container collector) |

The agent runs as `user: "0:0"` (root) so it can read the docker socket
and `/proc`. Same posture as cAdvisor, Beszel, node-exporter.

### On macOS Docker Desktop

Container collection silently no-ops unless you enable
**Settings → Advanced → "Allow the default Docker socket to be used
(requires password)"**. Once enabled, the agent's first attempt
surfaces a one-shot Warn, then quietly works.

### On Linux (including inside an LXC)

Just works. See [Hub — Proxmox LXC](./hub-lxc) for the LXC walkthrough,
which also covers the `nesting=1` requirement for Docker-in-LXC.

## Operations

```bash
# Live logs (both services)
docker compose -f deploy/docker/docker-compose.yml logs -f

# Restart hub only after editing .env
docker compose -f deploy/docker/docker-compose.yml up -d --force-recreate hub

# Stop everything but keep data
docker compose -f deploy/docker/docker-compose.yml down

# Stop and wipe SQLite
docker compose -f deploy/docker/docker-compose.yml down -v
```

> `docker compose restart` does **not** reload `env_file`. Use
> `up -d --force-recreate <service>` after editing `.env`.

## Upgrade

```bash
git pull
docker compose -f deploy/docker/docker-compose.yml up -d --build
```

The hub image rebuilds; the SQLite DB in the `lumen-data` volume
survives. Migrations apply on the next hub start.

## Backup the DB

Hot-copy via a sidecar container — WAL means `cp` is safe while the
hub is running:

```bash
docker run --rm \
  -v lumen_lumen-data:/data:ro \
  -v "$PWD/backup":/out \
  alpine cp /data/lumen.db /out/lumen-$(date +%F).db
```

For point-in-time snapshots use `sqlite3 .backup` in the running hub
container (the bundled distroless image has no shell — exec
`sqlite3` via a temporary alpine sidecar bound to the same volume).

## Exposing externally

Put a reverse proxy in front (Caddy / nginx / Traefik / Cloudflare
Tunnel) — same shape as in [hub-binary § Putting HTTPS in front](./hub-binary#putting-https-in-front).

For Caddy:

```caddy
lumen.example.com {
  reverse_proxy 127.0.0.1:8090
}
```

(Caddy auto-upgrades WebSocket; nothing else to configure.)

## Adding more agents

The compose stack only includes one same-host agent. To add agents on
**other** machines (any Linux box, LXC, or VM):

```bash
# In the hub UI: Settings → Hosts → Create "<host>" → copy token
# Then on the target machine:
curl -fsSL http://<your-hub-host>:8090/install.sh | sudo bash -s -- \
  --token lum_xxxxxxxxxxxxxxxxxxxxx \
  --host <host-name>
```

See [Agent — Linux](./agent-linux) for fleet patterns.

## Troubleshooting

**`pull access denied for lumen-hub:dev`**
: The compose file references locally-built images (`:dev` tag).
  Always include `--build` on first `up` so they get built.

**Compose port 8090 already in use**
: Edit `ports: ["8091:8090"]` in `deploy/docker/docker-compose.yml`
  and re-bring up. Agents then point at `:8091`.

**Browser can't reach the hub**
: Docker's default bridge binds to all interfaces; check your firewall
  (`ufw allow 8090/tcp`, Proxmox node firewall, etc.).

**Need to debug inside a distroless image**
: The runtime image has no shell. Use a sidecar:
  `docker run --rm -it --network container:lumen-hub alpine sh`.
