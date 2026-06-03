---
title: Quickstart
description: Run Lumen in under a minute with Docker Compose and the official image. No clone, no build.
sidebar:
  order: 2
---

The fastest path: pull the official multi-arch hub image, mint a host token from the UI, then install the agent on each machine you want to monitor with the one-line install script. No `git clone`, no Go toolchain, no `make`.

## Prerequisites

- Docker 20.10+ and Docker Compose v2 (`docker compose version`) on the host that will run the hub.
- A terminal — Linux, macOS, or Windows.
- Network reachability from each target machine to the hub URL.

## 1. Start the hub

```bash
sudo mkdir -p /opt/lumen-hub && cd /opt/lumen-hub

cat <<EOF | sudo tee docker-compose.yml >/dev/null
services:
  hub:
    image: ghcr.io/quanla93/lumen-hub:latest
    container_name: lumen-hub
    restart: unless-stopped
    ports:
      - "8090:8090"
    environment:
      LUMEN_HUB_SECRET: "$(openssl rand -hex 32)"
      LUMEN_HUB_DB_PATH: "/data/lumen.db"
      LUMEN_HUB_ADMIN_USERNAME: "admin"
      LUMEN_HUB_ADMIN_PASSWORD: "lumenadmin"
    volumes:
      - lumen-data:/data

volumes:
  lumen-data:
EOF

sudo docker compose up -d
```

The image is multi-arch (linux/amd64 + linux/arm64). The `LUMEN_HUB_SECRET` is generated once and inlined into the compose file so sessions survive restarts.

## 2. Sign in to the UI

Open `http://<your-host>:8090` and sign in with `admin` / `lumenadmin`. **Change the password from the UI** (Settings → Account) before exposing this hub on any network you don't fully control.

## 3. Add a host

In the UI: **Settings → Hosts → Create** → enter a friendly name (e.g. `pve-01`, `home-nas`). The hub shows the `lum_...` token **once** — copy it before closing the panel.

## 4. Install the agent on the target machine

Pick the path that fits the target host:

### Path A — install.sh (recommended)

The hub serves a templated install script. SSH into the target machine and run:

```bash
curl -fsSL https://YOUR-HUB/install.sh | sudo sh -s -- \
  --token lum_PASTE_FROM_HUB_UI \
  --host pve-01
```

The script auto-detects the arch, pulls the matching agent binary from the same hub, writes `/etc/systemd/system/lumen-agent.service`, and starts the service. The host card appears on the dashboard within one collection interval.

### Path B — Docker Compose on the target

If the target already runs Docker, **download the generated per-agent `docker-compose.yml`** from the hub UI (Settings → Hosts → host → ⋯ → Download compose) and put it on the target:

```bash
sudo mkdir -p /opt/lumen-agent
cd /opt/lumen-agent
# Save the generated docker-compose.yml from the hub UI here.
sudo docker compose up -d
sudo docker compose logs -f
```

The generated file already contains the token, hub URL, host name, Docker socket mount, and durable offline buffer volume.

### Path C — direct binary (advanced)

For air-gapped targets or custom packaging, see [Agent — Linux § Path B](../../install/agent-linux/#path-b--direct-binary-download--manual-unit) for the manual binary + systemd unit flow.

## 5. Update later

**Hub** (re-pull the image and recreate):

```bash
cd /opt/lumen-hub
sudo docker compose pull
sudo docker compose up -d
```

**Agent** — see the [update guide](../../how-to/update-agents/). The short version:

```bash
# Path A — re-run install.sh, it is idempotent:
curl -fsSL https://YOUR-HUB/install.sh | sudo sh

# Path B — Docker Compose agents:
cd /opt/lumen-agent
sudo docker compose pull
sudo docker compose up -d
```

Do not create a new host or rotate the token for a code update.

## Other install paths

- [Hub — Docker Compose](../../install/hub-compose/) — full hub install reference (operations, backups, reverse proxy).
- [Hub — binary + systemd](../../install/hub-binary/) — hub install from the release tarball, no Docker.
- [Hub — Proxmox LXC](../../install/hub-lxc/) — install inside a Proxmox LXC.
- [Agent — Docker Compose](../../install/agent-docker/) — full agent compose reference.
- [Agent — Linux (binary + systemd)](../../install/agent-linux/) — full native agent reference (install.sh + manual paths).

## Troubleshooting

**Compose port `:8090` already in use**
: Edit the host port mapping in `docker-compose.yml` (`"8091:8090"`), then `up -d --force-recreate hub`. Agents need to point at the new port.

**Agent logs `401 invalid token`**
: The token in the agent config is wrong or was rotated in the hub UI. Re-run the install script with `--token <new>` or edit the agent's systemd unit / compose file with the new value, then restart.

**Container table is empty in the host card**
: Host metrics still work. The container table only populates when the agent can read the Docker socket on the target — Path A install does this by default; Path B keeps the `/var/run/docker.sock` mount; Path C depends on the agent running as root.
