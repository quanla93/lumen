---
title: Quickstart
description: Run Lumen quickly with Docker Compose, then add agents from the web UI.
sidebar:
  order: 2
---

The fastest path is Docker Compose: one compose file starts the hub, stores the SQLite data in a Docker volume, and lets you add agents from the web UI.

## Prerequisites

- Docker 20.10+ and Docker Compose v2 (`docker compose version`).
- Git.
- A terminal — Linux, macOS, or Windows.

## 1. Clone and configure

```bash
git clone https://github.com/quanla93/lumen
cd lumen
cp .env.example .env
```

Edit `.env` and set a stable hub secret plus a real admin password before exposing the hub:

```ini
LUMEN_HUB_SECRET=<generate with: openssl rand -hex 32>
LUMEN_HUB_ADMIN_USERNAME=admin
LUMEN_HUB_ADMIN_PASSWORD=<your-choice>
```

## 2. Start the hub with Docker Compose

```bash
docker compose -f deploy/docker/docker-compose.yml up --build -d
```

Open the hub UI:

```text
http://localhost:8090
```

On a fresh database, create the first admin account or sign in with the seeded admin from `.env`.

## 3. Add an agent with its own compose file

In the hub UI, go to **Settings → Hosts**, create a host, and copy or download the generated `docker-compose.yml`. Save it on the target machine:

```bash
sudo mkdir -p /opt/lumen-agent
cd /opt/lumen-agent
sudo nano docker-compose.yml
sudo chmod 600 docker-compose.yml
sudo docker compose up -d
sudo docker compose logs -f
```

The agent card appears on the dashboard within one collection interval. Click it to open historical charts, per-core CPU, network/disk throughput, temperature when available, and live Docker container data when the agent can read the Docker socket.

## 4. Update later

Hub updates:

```bash
git pull
docker compose -f deploy/docker/docker-compose.yml up -d --build
```

Agent updates run on each target machine that owns an agent compose file:

```bash
cd /opt/lumen-agent
sudo docker compose pull
sudo docker compose up -d
```

Do not create a new host or rotate the token for a code update.

## Other install paths

- [Hub — Docker Compose](/install/hub-compose/) is the main install guide.
- [Agent — Docker Compose](/install/agent-docker/) has the full agent compose reference.
- [Run from source](/how-to/run-from-source/) is for development.
- [Native hub](/install/hub-binary/) and [native agent](/install/agent-linux/) are advanced/manual paths.

## Troubleshooting

**Compose port 8090 already in use**
: Edit the host port in `deploy/docker/docker-compose.yml`, for example `8091:8090`, then start the stack again. Agents must use the URL reachable from their target machine.

**Agent logs `401 invalid token`**
: The token in the target machine's `/opt/lumen-agent/docker-compose.yml` is wrong or was rotated. Rotate only when you intentionally want to invalidate the old credential.

**Container table is empty**
: Host metrics still work. Keep the read-only Docker socket mount in the agent compose file only when you want container telemetry.
