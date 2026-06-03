---
title: Agent — Linux (native)
description: Install the Lumen agent as a native systemd service on Linux hosts without Docker.
sidebar:
  order: 4
---

The agent is a small Go binary that POSTs metrics to a hub every
5 seconds. Install it on every machine you want to monitor: bare-metal
servers, VMs, Proxmox LXCs.

Each agent ships:

| Metric | Notes |
|---|---|
| CPU% (aggregate + per-core) | per-core live-only (not persisted) |
| RAM% + swap% | gopsutil virtual + swap memory |
| Disk% on `/` (or override) | `LUMEN_AGENT_DISK_PATH` |
| Load 1/5/15 | `/proc/loadavg`; 0 on Windows |
| Network rx/tx (B/s) | summed across all NICs, rate from cumulative |
| Disk I/O read/write (B/s) | summed across all block devices |
| Temperature (°C) | best CPU sensor from `coretemp` / `k10temp` / `package` / `tctl`; 0 if none readable |
| Container list | Docker socket (optional, see below) |

## Requirements

- Linux x86_64 / aarch64 / armv7 with systemd (Debian, Ubuntu, Alpine,
  RHEL/Rocky, Proxmox LXC).
- Outbound HTTPS / HTTP to the hub URL (NAT-friendly: agent pushes, hub
  doesn't dial agent).
- Root for the install step. The agent runs as root post-install to
  access `/proc`, `/sys/class/hwmon`, and the Docker socket.

## 1. Mint a host token in the hub

1. Sign in to the hub UI.
2. Settings → Hosts → **Create** → enter a friendly name (e.g.
   `pve-node-01`, `home-nas`, `mailserver`).
3. Copy the `lum_...` token shown **exactly once** — the hub stores
   only its SHA-256 hash. If you lose it, rotate to get a new one.

The host name you pick here is **authoritative**: it overrides whatever
`LUMEN_AGENT_HOST` the agent posts, so a leaked token can't be used to
spoof a different host.

## 2. Install the binary

Build or download the matching Linux agent binary, then copy it to the target host:

```bash
# Example from a build machine for linux/amd64.
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -o bin/lumen-agent-linux-amd64 ./cmd/lumen-agent

scp bin/lumen-agent-linux-amd64 root@10.0.0.50:/usr/local/bin/lumen-agent
ssh root@10.0.0.50 'chmod +x /usr/local/bin/lumen-agent'
```

For release tarballs, extract the archive and copy the included `lumen-agent` binary to `/usr/local/bin/lumen-agent`.

## 3. Create the systemd service

On the target host, write `/etc/systemd/system/lumen-agent.service`:

```ini
[Unit]
Description=Lumen agent
Documentation=https://github.com/quanla93/lumen
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=LUMEN_HUB_URL=https://lumen.example.lan
Environment=LUMEN_AGENT_HOST=pve-node-01
Environment=LUMEN_AGENT_TOKEN=lum_paste_your_token_here
Environment=LUMEN_AGENT_INTERVAL=5s
Environment=LUMEN_AGENT_BUFFER_PATH=/var/lib/lumen-agent/buffer.db
Environment=LUMEN_AGENT_BUFFER_MAX_AGE=24h
Environment=LUMEN_AGENT_BUFFER_DRAIN=10
ExecStart=/usr/local/bin/lumen-agent
Restart=always
RestartSec=5s

NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/lumen-agent

[Install]
WantedBy=multi-user.target
```

Then create the buffer directory and start the service:

```bash
sudo mkdir -p /var/lib/lumen-agent
sudo systemctl daemon-reload
sudo systemctl enable --now lumen-agent
```

Within one collection interval the host card appears on the hub dashboard. Click it to drill into per-core CPU, charts, and the container table.

## 4. Verify

```bash
systemctl status lumen-agent
journalctl -u lumen-agent -f
```

You should see `msg=ingested cpu=… ram=… …` every interval. In the hub
UI, the host card transitions from "no data" to live values.

## Fleet install across many LXCs

When you have a Proxmox cluster full of LXCs, the friction is "mint a token, copy a binary, write a service" times N. Two patterns help:

### Pattern A — script loop from the Proxmox host

Build the agent once, then copy it into each LXC and write a tiny env file plus a shared unit:

```bash
# On the Proxmox host. Mint one token per LXC in the hub UI first.
AGENT_BIN=./lumen-agent-linux-amd64
declare -A TOKENS=(
  [100]="lum_aaaa..."
  [101]="lum_bbbb..."
  [102]="lum_cccc..."
)
for vmid in "${!TOKENS[@]}"; do
  name=$(pct config $vmid | awk '/^hostname:/ {print $2}')
  pct push "$vmid" "$AGENT_BIN" /usr/local/bin/lumen-agent --perms 0755
  pct exec "$vmid" -- mkdir -p /etc/lumen /var/lib/lumen-agent
  pct exec "$vmid" -- tee /etc/lumen/agent.env >/dev/null <<EOF
LUMEN_HUB_URL=http://hub.lan:8090
LUMEN_AGENT_HOST=$name
LUMEN_AGENT_TOKEN=${TOKENS[$vmid]}
LUMEN_AGENT_INTERVAL=5s
LUMEN_AGENT_BUFFER_PATH=/var/lib/lumen-agent/buffer.db
EOF
  pct exec "$vmid" -- tee /etc/systemd/system/lumen-agent.service >/dev/null <<'EOF'
[Unit]
Description=Lumen agent
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=/etc/lumen/agent.env
ExecStart=/usr/local/bin/lumen-agent
Restart=always
RestartSec=5s
ReadWritePaths=/var/lib/lumen-agent

[Install]
WantedBy=multi-user.target
EOF
  pct exec "$vmid" -- systemctl daemon-reload
  pct exec "$vmid" -- systemctl enable --now lumen-agent
done
```

### Pattern B — Ansible

```yaml
- hosts: lumen_agents
  become: yes
  vars:
    lumen_hub_url: http://hub.lan:8090
  tasks:
    - name: Install Lumen agent binary
      copy:
        src: ./bin/lumen-agent-linux-amd64
        dest: /usr/local/bin/lumen-agent
        mode: "0755"

    - name: Create agent config directory
      file:
        path: /etc/lumen
        state: directory
        mode: "0700"

    - name: Write agent environment
      copy:
        dest: /etc/lumen/agent.env
        mode: "0600"
        content: |
          LUMEN_HUB_URL={{ lumen_hub_url }}
          LUMEN_AGENT_HOST={{ inventory_hostname }}
          LUMEN_AGENT_TOKEN={{ agent_token }}
          LUMEN_AGENT_INTERVAL=5s
          LUMEN_AGENT_BUFFER_PATH=/var/lib/lumen-agent/buffer.db

    - name: Write systemd unit
      copy:
        dest: /etc/systemd/system/lumen-agent.service
        mode: "0644"
        content: |
          [Unit]
          Description=Lumen agent
          After=network-online.target
          Wants=network-online.target

          [Service]
          EnvironmentFile=/etc/lumen/agent.env
          ExecStart=/usr/local/bin/lumen-agent
          Restart=always
          RestartSec=5s
          ReadWritePaths=/var/lib/lumen-agent

          [Install]
          WantedBy=multi-user.target

    - name: Start Lumen agent
      systemd:
        name: lumen-agent
        daemon_reload: true
        enabled: true
        state: restarted
```

`agent_token` is per-host (Ansible vault, group_vars, etc.).

## Docker container collection

If the host runs Docker, the agent will list and stat its containers
automatically — no extra config needed, the Docker socket at
`/var/run/docker.sock` is read directly. The shipped systemd unit
runs the agent as `root` which is sufficient.

To disable container collection, you don't need any agent change —
just stop Docker, or remove the socket from the agent's filesystem
namespace.

To use a non-standard socket path:

```bash
sudo systemctl edit lumen-agent
# Add:
[Service]
Environment=LUMEN_AGENT_DOCKER_SOCKET=/path/to/your/docker.sock
```

(`systemctl edit` creates a drop-in override at
`/etc/systemd/system/lumen-agent.service.d/override.conf` so the
main unit stays clean.)

## Upgrade

Replace `/usr/local/bin/lumen-agent` with the new binary and restart the service:

```bash
sudo systemctl stop lumen-agent
sudo install -m 0755 lumen-agent /usr/local/bin/lumen-agent
sudo systemctl start lumen-agent
```

Do not create a new host or rotate the token for a code update. The existing token stays in the systemd unit or YAML config.

## Uninstall

```bash
sudo systemctl disable --now lumen-agent
sudo rm /etc/systemd/system/lumen-agent.service /usr/local/bin/lumen-agent
sudo rm -rf /var/lib/lumen-agent
sudo systemctl daemon-reload
```

Don't forget to delete the host record in the hub UI (Settings →
Hosts → trash icon) — the row keeps its `last_seen_at` cosmetic stale
marker otherwise.

## Config file (YAML)

The installer wires the agent via `Environment=` lines in the systemd
unit — that's fine for one host but tedious to manage across a fleet
with Ansible/Salt. Drop a YAML config at `/etc/lumen/agent.yaml` and
fields fill any env var that isn't already set:

```yaml
# /etc/lumen/agent.yaml — mode 0600 root:root (token is sensitive)
hub_url: https://lumen.example.lan
token: lum_REPLACE_ME
host: ""              # empty → os.Hostname()
interval: 5s
disk_path: /
docker_socket: /var/run/docker.sock

# Offline buffer (see configure/reliability for details)
buffer_path: /var/lib/lumen-agent/buffer.db
buffer_max_age: 24h
buffer_drain: "10"
```

Field names map 1:1 to env vars (`hub_url` → `LUMEN_HUB_URL`, etc.).
Override `LUMEN_AGENT_CONFIG` to point at a different path; missing
file is a quiet no-op so env-only setups keep working.

**Precedence (highest first)**:
1. Process env (`Environment=` in the systemd unit)
2. This YAML file
3. `.env` in the agent's CWD (dev only)
4. Hardcoded defaults

That ordering lets you ship one fleet-wide YAML and override per-box
in the unit when needed. Boot logs name every key that came from
YAML vs. was already in the environment:

```
INFO config file loaded path=/etc/lumen/agent.yaml \
  applied_from_yaml=[LUMEN_HUB_URL LUMEN_AGENT_INTERVAL] \
  skipped_env_wins=[LUMEN_AGENT_TOKEN]
```

A malformed file is fatal at boot — half-loaded config is worse than
loud failure. Run `lumen-agent` once interactively to sanity-check
after edits.

## Limits and quirks

**Per-core CPU and containers are live-only**
: Both flow through the WebSocket stream but are NOT persisted to
  SQLite. After a hub restart the host detail page repopulates them
  on the agent's next tick (≤5s).

**Temperature unavailable in containers**
: `/sys/class/hwmon` is not exposed inside LXC by default. The
  temperature chart simply hides when no point > 0°C. To make it
  visible inside a privileged LXC, mount the directory in your
  Proxmox config:

  ```
  lxc.mount.entry: /sys/class/hwmon sys/class/hwmon none bind,optional,ro 0 0
  ```

**Multiple agents on the same host**
: Possible but unusual. Each `systemd` unit must have a different
  name (`lumen-agent@<id>.service` via templating). Mint one token
  per agent. Pattern only useful if you want to monitor *namespaces*
  separately on the same kernel; in 95% of cases one agent per host
  is right.

## Troubleshooting

**Agent logs `connect: connection refused` on every tick**
: Wrong `LUMEN_HUB_URL`. The installer bakes the URL the hub served
  itself at — if you reach the hub at a different URL (NAT, tunnel),
  pass `--hub`.

**Agent logs `hub returned 401: invalid token`**
: Token revoked / rotated in the UI after install. Mint a new one
  and re-run the installer with `--token`.

**Agent logs `hub returned 401: Authorization: Bearer <token> required`**
: No token passed to install. Re-run with `--token lum_...`.

**Host card never appears on dashboard**
: Check the hub's `journalctl -u lumen-hub -f` — if you see
  `ingest accepted`, the agent is talking but the dashboard WS may
  be disconnected. Hard-reload the browser.
