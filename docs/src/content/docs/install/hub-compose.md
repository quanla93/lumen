---
title: Hub — Docker Compose
description: Run the Lumen hub from the official multi-arch image. No clone, no build — just pull and run.
sidebar:
  order: 1
---

Run the Lumen hub from the official multi-arch image (`ghcr.io/quanla93/lumen-hub`). This is the fastest path: no source clone, no Go toolchain, no build step. Works on any host with Docker.

## Requirements

- Docker 20.10+ and Docker Compose v2 (`docker compose version`).
- 200 MiB free RAM, ~250 MiB disk.
- Internet access to pull the image once (`ghcr.io`).
- A free port for the UI (default `8090`).

## 1. Create a working directory

```bash
sudo mkdir -p /opt/lumen-hub
cd /opt/lumen-hub
```

## 2. Write `docker-compose.yml`

```yaml
# /opt/lumen-hub/docker-compose.yml
services:
  hub:
    image: ghcr.io/quanla93/lumen-hub:latest
    container_name: lumen-hub
    restart: unless-stopped
    ports:
      - "8090:8090"
    environment:
      # Stable HMAC secret — keeps sessions valid across restarts.
      # Generate with: openssl rand -hex 32
      LUMEN_HUB_SECRET: "REPLACE_WITH_32_BYTE_HEX"

      # Where SQLite lives inside the container (volume mounted below).
      LUMEN_HUB_DB_PATH: "/data/lumen.db"

      # Bootstrap admin — change the password before exposing the hub.
      LUMEN_HUB_ADMIN_USERNAME: "admin"
      LUMEN_HUB_ADMIN_PASSWORD: "lumenadmin"
    volumes:
      - lumen-data:/data

volumes:
  lumen-data:
```

Generate the secret and write it in place:

```bash
sudo sed -i "s|REPLACE_WITH_32_BYTE_HEX|$(openssl rand -hex 32)|" docker-compose.yml
```

Pinning a version is recommended for production. Replace `:latest` with the tag you want, e.g. `ghcr.io/quanla93/lumen-hub:0.6.5`. The image is multi-arch (linux/amd64 + linux/arm64) so the same tag works on x86 servers, Apple Silicon, Raspberry Pi 4/5, Ampere, Graviton, etc.

## 3. Start the hub

```bash
sudo docker compose up -d
sudo docker compose ps
sudo docker compose logs -f
```

You should see:

```
time=... level=INFO msg="storage ready" path=/data/lumen.db
time=... level=INFO msg="hub listening" addr=:8090 dev=false
```

Open `http://<your-host>:8090` and sign in with the admin credentials from the compose file.

> Change the admin password from the UI before exposing the hub on a network you don't fully trust.

## 4. Add an agent

The hub on its own shows nothing. Add agents per target machine — each one gets its own token. Pick the agent install path that matches the target host:

- [Agent — install.sh one-liner](./agent-linux/#install-script) — recommended for Linux LXCs / VMs / bare metal.
- [Agent — Docker Compose](./agent-docker/) — for hosts where Docker is already the deployment substrate.
- [Agent — binary + systemd](./agent-linux/) — for explicit control over the systemd unit.

## Operations

```bash
# Live logs
sudo docker compose logs -f

# Restart after editing docker-compose.yml
sudo docker compose up -d --force-recreate hub

# Stop everything but keep data
sudo docker compose down

# Stop and wipe SQLite (destructive)
sudo docker compose down -v
```

> `docker compose restart` does **not** reload environment variables. Use `up -d --force-recreate hub` after editing the compose file.

## Upgrade

```bash
cd /opt/lumen-hub
sudo docker compose pull
sudo docker compose up -d
```

This pulls the newer image and recreates the container with the same config. The SQLite DB in the `lumen-data` volume survives — migrations apply on the next start.

If you pinned a version tag, edit `image:` to the new tag first, then `pull` + `up -d`.

## Backup the database

WAL means `cp` is safe while the hub is running:

```bash
docker run --rm \
  -v lumen-hub_lumen-data:/data:ro \
  -v "$PWD/backup":/out \
  alpine cp /data/lumen.db /out/lumen-$(date +%F).db
```

The volume name is `<compose-project>_<volume-name>` — if your working directory is `/opt/lumen-hub` the project is `lumen-hub` and the volume is `lumen-hub_lumen-data`.

## Exposing externally

The hub speaks plain HTTP on `:8090`. Put a reverse proxy in front for anything reachable from outside your trusted network — Caddy is the smallest config:

```text
lumen.example.com {
  reverse_proxy 127.0.0.1:8090
}
```

(Caddy upgrades WebSocket automatically.) Same shape works with nginx, Traefik, Cloudflare Tunnel, Tailscale Funnel.

## Troubleshooting

**`port is already allocated`**
: Another service is on `:8090`. Change the port mapping (`"8091:8090"`) and re-up. Agents need to know about the new port.

**`Sessions die every restart`**
: `LUMEN_HUB_SECRET` is empty or changes between runs. Generate it once and write it into the compose file — the hub re-uses the same secret across restarts to keep cookies valid.

**Container restarts in a loop**
: `docker compose logs hub` — usually a malformed `LUMEN_HUB_SECRET` (must be ≥64 hex chars) or wrong volume permissions.

**Browser cannot reach the hub**
: Check the host firewall (`ufw allow 8090/tcp`, Proxmox node firewall, security group) — Docker's default bridge binds to all interfaces, so usually the firewall is the culprit.

**Need a shell inside the distroless image**
: The runtime image has no shell. Use a sidecar that shares the same network namespace:
`docker run --rm -it --network container:lumen-hub alpine sh`.
