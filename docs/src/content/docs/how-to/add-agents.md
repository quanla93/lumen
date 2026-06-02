---
title: Add agents
description: Add a Lumen agent to a new machine with the generated Docker Compose file, or use native/manual fallback paths.
sidebar:
  order: 2
---

Every machine you want to monitor runs **one Lumen agent**. The agent POSTs a metrics snapshot to the hub every `LUMEN_AGENT_INTERVAL`. Docker Compose is the recommended long-running path when Docker is available; native/manual install remains available for hosts where Docker does not belong.

## The flow at a glance

```
┌─ Hub UI ───────────────────────┐    ┌─ Target machine ────────────────┐
│ Settings → Hosts → Create      │    │ /opt/lumen-agent/               │
│ → generated docker-compose.yml │ →  │ docker-compose.yml              │
│ → token shown once             │    │ docker compose up -d            │
└────────────────────────────────┘    └─────────────────────────────────┘
```

The token shown in Settings is displayed **exactly once**. The hub stores
only its SHA-256 hash. If you lose it, click **Rotate** to mint a fresh
one — the old token immediately stops working.

Updating an existing agent is a different flow from onboarding. Do not
create a new host or token for code updates; see [Update agents](/how-to/update-agents/).

## Which mode should I pick?

| Mode | When to use | Footprint on target |
|---|---|---|
| **[Docker Compose agent](#a-docker-compose-agent-recommended)** | Target already runs Docker, or you want simple update/restart/log commands | ~30 MB image, ~25 MB RAM |
| **[Native binary + manual systemd](#b-native-binary-manual-install)** | Minimal Linux/systemd install, no Docker on the target | ~15 MB binary, ~10 MB RAM |

**Recommended default: Docker Compose.** It keeps the agent token/config in one file on the target host and future updates are just `docker compose pull && docker compose up -d`.

For Proxmox LXC, choose based on how that LXC is managed: use Compose if Docker already belongs there; use native systemd if you want the smallest footprint.

---

## A. Docker Compose agent (recommended)

Create the host in Settings first, then download or copy the generated per-agent `docker-compose.yml`. Save that exact file on the target machine:

```bash
sudo mkdir -p /opt/lumen-agent
cd /opt/lumen-agent
# Save the generated docker-compose.yml from the hub UI in this directory.
```

The generated file is ready to run. If you cannot use the generated file, use this manual fallback template and replace the three values marked `CHANGE`:

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

      # CHANGE: local display/log name for this target.
      LUMEN_AGENT_HOST: "my-server"

      LUMEN_AGENT_INTERVAL: "5s"
      LUMEN_AGENT_BUFFER_PATH: "/data/buffer.db"
      LUMEN_AGENT_BUFFER_MAX_AGE: "24h"
      LUMEN_AGENT_BUFFER_DRAIN: "10"
    volumes:
      - lumen-agent-data:/data
      - /var/run/docker.sock:/var/run/docker.sock:ro

volumes:
  lumen-agent-data:
```

Then secure the file and start the agent:

```bash
sudo chmod 600 docker-compose.yml
sudo docker compose up -d
sudo docker compose logs -f
```

Future updates are simple and do not need a new token:

```bash
cd /opt/lumen-agent
sudo docker compose pull
sudo docker compose up -d
```

The flow is UI-generated Compose-first: don't edit the hub's `docker-compose.yml` or add one `.env` variable per agent. Each target host owns its own per-agent compose file.

See [Agent — Docker Compose](/install/agent-docker/) for the generated file shape, update path, logs, uninstall, Docker socket options, and troubleshooting.

---

## B. Native binary (manual install)

### Build the binary

On any machine with Go 1.25+:

```bash
# linux/amd64 — Proxmox host, most x86 VMs/LXC
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -o bin/lumen-agent-linux-amd64 ./cmd/lumen-agent

# linux/arm64 — Raspberry Pi 4/5, Apple Silicon Linux VMs
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
  go build -o bin/lumen-agent-linux-arm64 ./cmd/lumen-agent
```

`CGO_ENABLED=0` makes the binary fully static — no glibc/musl mismatch,
runs on Alpine/Ubuntu/Debian/Arch identically.

> Pre-built release binaries (`lumen-agent_<version>_linux_{amd64,arm64}.tar.gz`)
> are attached to every [GitHub Release](https://github.com/quanla93/lumen/releases) —
> grab them directly instead of cross-compiling, unless you want a specific
> commit. For long-running Docker-based agents, prefer the generated Compose
> file above.

### Mint a token

In the hub UI → **Settings → Hosts → Add host** → name it after the
target (e.g. `pve-node-1`, `lxc-postgres`, `pi-bedroom`) → copy the
`lum_…` token immediately.

### Install on the target

```bash
# 1. Copy the binary
scp bin/lumen-agent-linux-amd64 root@10.0.0.50:/usr/local/bin/lumen-agent
ssh root@10.0.0.50 'chmod +x /usr/local/bin/lumen-agent'

# 2. Install the systemd unit (on the target)
ssh root@10.0.0.50 'cat >/etc/systemd/system/lumen-agent.service' <<'EOF'
[Unit]
Description=Lumen agent
Documentation=https://github.com/quanla93/lumen
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=LUMEN_HUB_URL=http://10.0.0.10:8090
Environment=LUMEN_AGENT_HOST=pve-node-1
Environment=LUMEN_AGENT_TOKEN=lum_paste_your_token_here
Environment=LUMEN_AGENT_INTERVAL=5s
ExecStart=/usr/local/bin/lumen-agent
Restart=always
RestartSec=5s

# Hardening — agent only reads /proc, /sys, disk usage. No writes needed.
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

# 3. Start it
ssh root@10.0.0.50 'systemctl daemon-reload && systemctl enable --now lumen-agent'
```

Verify:

```bash
ssh root@10.0.0.50 'journalctl -u lumen-agent -n 20 --no-pager'
# time=... level=INFO msg="agent starting" hub=http://10.0.0.10:8090 host=pve-node-1
# time=... level=INFO msg=ingested cpu=2.4 ram=18.7 disk=42.1 load1=0.15
```

The host appears on the dashboard within one tick (5 s default).

### LXC quirk: no privileged container needed

Unprivileged LXC works fine for the default collectors (CPU/RAM/disk/load).
Network throughput and per-process metrics may be restricted on
unprivileged containers — that's a Phase 2 collector concern, not a
deployment blocker today.

---

## Rotating a token

If a token leaks (e.g. committed to git) or an agent moves to a new
machine and you want a fresh credential:

1. Hub UI → **Settings → Hosts** → click **Rotate** next to the host.
2. Copy the new `lum_…` shown once.
3. For Docker Compose agents, replace the target host's `/opt/lumen-agent/docker-compose.yml` with the newly generated file and run `docker compose up -d`. For native agents, update `LUMEN_AGENT_TOKEN` in the systemd environment or config and restart `lumen-agent`.

The old token starts returning `401` immediately. The host record
(metrics history, name, position on dashboard) is preserved.

---

## What's coming next

The fast paths are tracked in [ACTION_PLAN.md](https://github.com/quanla93/lumen/blob/main/ACTION_PLAN.md):

- **Version awareness** — show current agent version vs the latest bundled/released agent version.
- **Host Detail update panel** — show the Compose update command in the UI for the selected host.
- **GitHub Release tarballs** — pre-built native binaries for non-Docker installs.
- **Bulk-add** — paste a list of hosts in Settings, get one token/compose file per host.

---

## Troubleshooting

**Agent logs `ingest send failed: hub returned 401: invalid token`**
: The token doesn't match anything in the `hosts` table. Re-check
`LUMEN_AGENT_TOKEN` (no leading/trailing whitespace), or rotate.

**Agent logs `ingest send failed: connection refused`**
: `LUMEN_HUB_URL` is wrong or the hub isn't reachable from the target.
Test with `curl <LUMEN_HUB_URL>/healthz`.

**Container table is empty**
: Host metrics work without Docker access. If you want container telemetry, keep the read-only Docker socket mount documented in [Agent — Docker Compose](/install/agent-docker/#docker-socket-mount).

**Two agents share the same token — what happens?**
: Both succeed at ingest, but every snapshot overwrites the previous
one because the hub keys metrics by **host name from the token**, not
the agent's self-reported name. You'll see CPU jumping erratically.
Mint a separate token per host.
