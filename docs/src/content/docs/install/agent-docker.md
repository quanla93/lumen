---
title: Agent — Docker Compose
description: Run and maintain each Lumen agent with its own Docker Compose file on the target host.
sidebar:
  order: 5
---

Docker Compose is the recommended long-running deployment path for Lumen agents. Each monitored VM/LXC/server owns its own `/opt/lumen-agent/docker-compose.yml`; the hub is not edited when you add or update agents.

The token is shown once by the hub, then persists in the target host's compose file. Future updates reuse that same token/config.

## Mental model

```
┌─ Hub UI ───────────────────────┐     ┌─ Target VM/LXC/server ──────────────┐
│ Settings → Hosts → Create       │     │ /opt/lumen-agent/docker-compose.yml │
│ copy/download docker-compose.yml│  →  │ docker compose up -d                │
│ token shown once                │     │ docker compose pull && up -d        │
└─────────────────────────────────┘     └────────────────────────────────────┘
```

Do **not** add agents to the hub's own compose file. The hub compose file runs the hub; each target host has a separate agent compose file.

## Requirements

- Docker 20.10+ and Docker Compose v2 on the target host.
- Network access from the target host to the hub URL.
- Root or a user allowed to run Docker commands.
- Optional: read-only Docker socket mount if you want container telemetry.

## 1. Create the host in the hub UI

1. Sign in to Lumen.
2. Go to **Settings → Hosts**.
3. Create a host with a stable name such as `pve-01`, `lxc-postgres`, or `nas`.
4. Copy or download the generated `docker-compose.yml`.

The token is displayed only once. If you close the panel before saving the compose file, rotate the token and save the new generated file.

## 2. Create `/opt/lumen-agent/docker-compose.yml`

SSH into the machine you want to monitor and create a dedicated directory:

```bash
sudo mkdir -p /opt/lumen-agent
cd /opt/lumen-agent
# Save the generated docker-compose.yml from the hub UI in this directory.
```

The generated file is ready to run. If you cannot use the generated file, use this manual fallback template and replace the three marked values:

```yaml
services:
  lumen-agent:
    image: ghcr.io/quanla93/lumen-agent:latest
    container_name: lumen-agent-my-server
    restart: unless-stopped
    user: "0:0"
    environment:
      # CHANGE: URL this target host uses to reach your hub.
      LUMEN_HUB_URL: "https://lumen.example.lan"

      # CHANGE: token shown once in Settings → Hosts.
      LUMEN_AGENT_TOKEN: "lum_REPLACE_WITH_UI_TOKEN"

      # CHANGE: display name for local logs; token host wins server-side.
      LUMEN_AGENT_HOST: "my-server"

      # Bootstrap interval. Hub Settings → Runtime can override this later.
      LUMEN_AGENT_INTERVAL: "5s"

      # Durable offline buffer stored in the named Docker volume below.
      LUMEN_AGENT_BUFFER_PATH: "/data/buffer.db"
      LUMEN_AGENT_BUFFER_MAX_AGE: "24h"
      LUMEN_AGENT_BUFFER_DRAIN: "10"
    volumes:
      # Keep buffered metrics across container recreate/update.
      - lumen-agent-data:/data

      # Optional: enables Docker container telemetry on this target host.
      - /var/run/docker.sock:/var/run/docker.sock:ro

volumes:
  lumen-agent-data:
```

Restrict access because the file contains the agent token:

```bash
sudo chmod 600 docker-compose.yml
```

Manual fallback values you must change:

| Placeholder | Example | Notes |
|---|---|---|
| `LUMEN_HUB_URL` | `http://10.0.0.10:8090` or `https://lumen.example.lan` | Use the URL reachable **from the target host**, not necessarily from your laptop. |
| `LUMEN_AGENT_TOKEN` | `lum_...` | Copy from Settings → Hosts. It is shown once. |
| `LUMEN_AGENT_HOST` | `pve-01` | Local label for logs/config. The hub still uses the token's host record as authoritative. |

If the target host does not run Docker containers, remove the Docker socket line and keep the `/data` volume.

## 3. Start the agent

```bash
cd /opt/lumen-agent
sudo docker compose up -d
sudo docker compose ps
sudo docker compose logs -f
```

The host card should appear in the dashboard within one collection interval. The default is `5s`, but the hub can later change it from **Settings → Runtime**.

## Compose file reference

| Field | Required | Purpose |
|---|---:|---|
| `image` | yes | Agent image to run. Use `latest` for fast updates or pin a version tag for controlled upgrades. |
| `container_name` | no | Stable name for `docker logs`, `docker compose ps`, and monitoring. |
| `restart: unless-stopped` | yes | Restarts the agent after host reboot or Docker daemon restart. |
| `user: "0:0"` | recommended | Gives access to host-level `/proc`, `/sys`, disk info, and Docker socket. |
| `LUMEN_HUB_URL` | yes | Hub URL reachable from this target host. |
| `LUMEN_AGENT_TOKEN` | yes | Per-host bearer token from the hub UI. |
| `LUMEN_AGENT_HOST` | recommended | Local display/log name; the token's host still wins server-side. |
| `LUMEN_AGENT_INTERVAL` | recommended | Startup collection interval before the hub policy is fetched. |
| `LUMEN_AGENT_BUFFER_PATH` | recommended | Path to the durable offline buffer inside the container. |
| `LUMEN_AGENT_BUFFER_MAX_AGE` | recommended | Maximum age of buffered frames during hub outages. |
| `LUMEN_AGENT_BUFFER_DRAIN` | recommended | Number of buffered frames replayed per successful tick. |
| `lumen-agent-data:/data` | recommended | Named volume that keeps the offline buffer across updates. |
| `/var/run/docker.sock` mount | optional | Enables Docker container telemetry from this target host. |

## Common compose variants

### Host metrics only

Use this when the target does not run Docker workloads or you do not want to expose the Docker socket:

```yaml
services:
  lumen-agent:
    image: ghcr.io/quanla93/lumen-agent:latest
    container_name: lumen-agent-my-server
    restart: unless-stopped
    user: "0:0"
    environment:
      LUMEN_HUB_URL: "https://lumen.example.lan"
      LUMEN_AGENT_TOKEN: "lum_REPLACE_WITH_UI_TOKEN"
      LUMEN_AGENT_HOST: "my-server"
      LUMEN_AGENT_INTERVAL: "5s"
      LUMEN_AGENT_BUFFER_PATH: "/data/buffer.db"
    volumes:
      - lumen-agent-data:/data

volumes:
  lumen-agent-data:
```

### Host metrics + Docker container telemetry

Use this when the target host runs Docker containers and you want the Lumen dashboard to show container CPU/RAM/state:

```yaml
services:
  lumen-agent:
    image: ghcr.io/quanla93/lumen-agent:latest
    container_name: lumen-agent-my-server
    restart: unless-stopped
    user: "0:0"
    environment:
      LUMEN_HUB_URL: "https://lumen.example.lan"
      LUMEN_AGENT_TOKEN: "lum_REPLACE_WITH_UI_TOKEN"
      LUMEN_AGENT_HOST: "my-server"
      LUMEN_AGENT_INTERVAL: "5s"
      LUMEN_AGENT_BUFFER_PATH: "/data/buffer.db"
    volumes:
      - lumen-agent-data:/data
      - /var/run/docker.sock:/var/run/docker.sock:ro

volumes:
  lumen-agent-data:
```

## Docker socket mount

Keep this mount when the target host runs Docker containers and you want the Lumen dashboard to show container CPU/RAM/state:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro
```

Remove it when:

- The host does not run Docker containers.
- You only want host-level CPU/RAM/disk metrics.
- Your security policy does not allow containers to read the Docker socket.

The agent keeps working without the socket; only container telemetry disappears.

## Update the agent

Run updates on the target host, in the directory that owns the agent compose file:

```bash
cd /opt/lumen-agent
sudo docker compose pull
sudo docker compose up -d
```

This recreates the container with the same token/config. Do not create a new host and do not rotate the token for a code update.

## Restart without updating

```bash
cd /opt/lumen-agent
sudo docker compose restart
```

## View logs

```bash
cd /opt/lumen-agent
sudo docker compose logs -f
```

Useful lines:

```text
time=... level=INFO msg="agent starting" hub=https://lumen.example.lan host=my-server interval=5s
time=... level=INFO msg=ingested cpu=2.4 ram=18.7 disk=42.1 load1=0.15
```

## Stop or uninstall

Stop but keep config and buffer:

```bash
cd /opt/lumen-agent
sudo docker compose down
```

Remove local agent files and buffer:

```bash
cd /
sudo rm -rf /opt/lumen-agent
sudo docker volume rm lumen-agent_lumen-agent-data
```

Only delete `/opt/lumen-agent` if you intentionally want to remove the local copy of the token/config. Delete the host in **Settings → Hosts** if you also want it gone from the hub.

## Rotate a token

Token rotation is a security action, not an update action. Rotate only if the token leaks or you intentionally want to invalidate the old credential.

After rotating in the hub UI:

1. Download or copy the newly generated per-agent compose file.
2. Replace `/opt/lumen-agent/docker-compose.yml` on the target host with that generated file.
3. Recreate the container:

```bash
cd /opt/lumen-agent
sudo docker compose up -d
```

If you are using the manual fallback template, replace only `LUMEN_AGENT_TOKEN` and run the same Compose command.

## Pin a version

For production-like installs, pin an image tag instead of `latest`:

```yaml
services:
  lumen-agent:
    image: ghcr.io/quanla93/lumen-agent:0.6.5
```

Image tags follow the GitHub release tags — see <https://github.com/quanla93/lumen/releases> for the latest stable version.

Then update with the same Compose commands:

```bash
cd /opt/lumen-agent
sudo docker compose pull
sudo docker compose up -d
```

## macOS Docker Desktop quirk

Docker Desktop 4.x ships with default Docker socket sharing disabled. Without it, the bind mount silently no-ops inside the agent container, and container telemetry is unavailable while host metrics keep working.

To enable container telemetry on macOS:

1. Docker Desktop → Settings → **Advanced**.
2. Enable **Allow the default Docker socket to be used**.
3. Apply & Restart.
4. Run `docker compose restart` from `/opt/lumen-agent`.

Linux + Docker on the host works out of the box.

## docker run fallback

`docker run` is only for quick tests. Compose is recommended for long-running agents because update/restart/log commands stay simple and the token/config remains in a file on the target host.

```bash
docker run -d \
  --name lumen-agent-my-server \
  --restart unless-stopped \
  --user 0:0 \
  -v lumen-agent-data:/data \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -e LUMEN_HUB_URL=https://lumen.example.lan \
  -e LUMEN_AGENT_TOKEN=lum_REPLACE_WITH_UI_TOKEN \
  -e LUMEN_AGENT_HOST=my-server \
  -e LUMEN_AGENT_BUFFER_PATH=/data/buffer.db \
  ghcr.io/quanla93/lumen-agent:latest
```

If the container was created with `docker run`, update means removing and recreating the container manually. Prefer Compose for anything you expect to keep.

## Troubleshooting

**`401 Authorization: Bearer <token> required`**
: `LUMEN_AGENT_TOKEN` is missing or empty in the compose file.

**`401 invalid token`**
: The token was rotated, copied incorrectly, or belongs to a different deleted host. Rotate the token in the UI and update the compose file on the target.

**Host never appears**
: Check `LUMEN_HUB_URL` from inside the target host. If the hub is behind a reverse proxy, use the URL the target can actually reach.

**Container table is empty**
: Keep the Docker socket mount and verify Docker is running on the target. Host metrics still work without container access.

**Data disappears during hub outage**
: Make sure the compose file includes the `lumen-agent-data:/data` volume and `LUMEN_AGENT_BUFFER_PATH=/data/buffer.db` so the offline buffer survives container recreation.
