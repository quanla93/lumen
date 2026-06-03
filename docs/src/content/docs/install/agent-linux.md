---
title: Agent — Linux (binary + systemd)
description: Install the Lumen agent on Linux from the hub one-liner or the official release binary. No Docker, no source clone.
sidebar:
  order: 5
---

The agent is a single static Go binary (~7 MB) that POSTs metrics to a hub every 5 seconds. Two install paths cover everything:

| Path | When |
|---|---|
| **A. `install.sh` one-liner from your hub** (recommended) | Linux LXCs / VMs / bare metal where the target can reach the hub URL. |
| **B. Direct binary download + manual systemd unit** | Air-gapped targets, custom packaging, or when you want the unit file fully under your control. |

For containerised hosts, use [Agent — Docker Compose](./agent-docker/) instead.

Each agent ships:

| Metric | Notes |
|---|---|
| CPU% (aggregate + per-core) | per-core is live-only (not persisted) |
| RAM% + swap% | gopsutil virtual + swap memory |
| Disk% on `/` (or override) | `LUMEN_AGENT_DISK_PATH` |
| Load 1/5/15 | `/proc/loadavg` |
| Network rx/tx (B/s) | summed across all NICs, rate from cumulative counters |
| Disk I/O read/write (B/s) | summed across all block devices |
| Temperature (°C) | best CPU sensor from `coretemp` / `k10temp` / `package` / `tctl`; hidden if none readable |
| Container list | Docker socket (optional) |

## Requirements

- Linux x86_64 / aarch64 with systemd (Debian, Ubuntu, Alpine, RHEL/Rocky, Proxmox LXC).
- Outbound HTTPS / HTTP to the hub URL. The agent pushes — the hub doesn't dial the agent — so this is NAT-friendly.
- Root for the install step. The agent runs as root post-install to access `/proc`, `/sys/class/hwmon`, and the Docker socket.

## 1. Mint a host token in the hub

1. Sign in to the hub UI.
2. **Settings → Hosts → Create** → enter a friendly name (e.g. `pve-node-01`, `home-nas`, `mailserver`).
3. Copy the `lum_...` token shown **once** — the hub stores only its SHA-256 hash. If you lose it, rotate to get a new one.

The host name you pick is **authoritative**: it overrides whatever `LUMEN_AGENT_HOST` the agent posts, so a leaked token cannot be used to spoof a different host.

## Path A — Install script (recommended)

The install script lives at [`scripts/install-agent.sh`](https://github.com/quanla93/lumen/blob/main/scripts/install-agent.sh) in the repo. Two ways to run it — both produce the same systemd-managed agent:

### A.1 — From your hub (URL baked in)

```bash
curl -fsSL https://YOUR-HUB/install.sh | sudo sh -s -- \
  --token lum_PASTE_FROM_HUB_UI \
  --host pve-node-01
```

The hub templates its own URL into the script before serving it, so you don't need `--hub`. The script then pulls the matching agent binary from `/install/lumen-agent-linux-<arch>` on the same hub.

### A.2 — From GitHub raw (no hub round-trip)

When the target host can reach `github.com` more reliably than the hub URL (air-gapped LANs that allow GitHub but not the hub yet, or fleet provisioning that hasn't pointed at the hub yet):

```bash
curl -fsSL https://raw.githubusercontent.com/quanla93/lumen/main/scripts/install-agent.sh \
  | sudo sh -s -- \
    --hub https://YOUR-HUB \
    --token lum_PASTE_FROM_HUB_UI \
    --host pve-node-01
```

The script detects the un-templated hub URL placeholder and falls back to whatever you pass via `--hub`. Pin a specific release tag instead of `main` for reproducibility:

```bash
curl -fsSL https://raw.githubusercontent.com/quanla93/lumen/v0.6.5/scripts/install-agent.sh | sudo sh -s -- ...
```

### What the script does (both paths)

1. Detects `uname -m` to pick `linux-amd64` or `linux-arm64`.
2. Pulls the matching agent binary — from the hub's `/install/lumen-agent-linux-<arch>` (A.1) or from the GitHub release of the script version (A.2).
3. Installs the binary to `/usr/local/bin/lumen-agent`.
4. Writes `/etc/lumen-agent/lumen-agent.yaml` with hub URL + token.
5. Drops `/etc/systemd/system/lumen-agent.service`.
6. Reloads systemd, enables, and starts the unit.

You should see the host card appear on the dashboard within one collection interval.

> If the hub URL is HTTPS and you haven't put a trusted cert in front yet, add `--insecure` to the `curl` command. Don't ship that in fleet automation.

### Updating with the same script

The install script is idempotent — re-running replaces the binary and refreshes the unit while preserving the existing token + config:

```bash
# A.1 — hub-served:
curl -fsSL https://YOUR-HUB/install.sh | sudo sh

# A.2 — GitHub raw, pinned to a tag:
curl -fsSL https://raw.githubusercontent.com/quanla93/lumen/v0.6.5/scripts/install-agent.sh | sudo sh -s -- --hub https://YOUR-HUB
```

See [How-to — Update agents](../../how-to/update-agents/#binary--systemd-agents-no-docker) for the manual binary-swap flow if you'd rather not pipe `curl` to `sh`.

## Path B — Direct binary download + manual unit

Use this when the install script isn't appropriate (air-gapped, custom config layout, fleet config-management tools).

### B.1 Download the agent binary

Pick the tag and arch (`uname -m`: `x86_64` → `amd64`, `aarch64` → `arm64`):

```bash
TAG=v0.6.5
ARCH=amd64

sudo curl -fsSL \
  "https://github.com/quanla93/lumen/releases/download/${TAG}/lumen-agent-linux-${ARCH}" \
  -o /usr/local/bin/lumen-agent
sudo chmod +x /usr/local/bin/lumen-agent
```

If the target host can't reach `github.com`, pull from your own hub instead — it serves the same binary:

```bash
sudo curl -fsSL https://YOUR-HUB/install/lumen-agent-linux-amd64 \
  -o /usr/local/bin/lumen-agent
sudo chmod +x /usr/local/bin/lumen-agent
```

### B.2 Write the systemd unit

```bash
sudo mkdir -p /var/lib/lumen-agent

sudo tee /etc/systemd/system/lumen-agent.service >/dev/null <<'EOF'
[Unit]
Description=Lumen agent
Documentation=https://lumen.quanla.org/docs/
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=LUMEN_HUB_URL=https://lumen.example.lan
Environment=LUMEN_AGENT_HOST=pve-node-01
Environment=LUMEN_AGENT_TOKEN=lum_PASTE_FROM_HUB_UI
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
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now lumen-agent
```

Edit the three `Environment=` lines that say `PASTE_FROM_HUB_UI` / `pve-node-01` / `lumen.example.lan` to match your setup before starting the service.

## Verify

```bash
systemctl status lumen-agent
journalctl -u lumen-agent -f
```

You should see `msg=ingested cpu=… ram=… …` every interval. In the hub UI, the host card transitions from "no data" to live values.

## Fleet install across many LXCs

When you have a Proxmox cluster full of LXCs, the friction is "mint a token + run install.sh" × N. Two patterns help:

### Pattern A — install script loop from the Proxmox host

Mint one token per LXC in the hub UI first, then pipe `install.sh` from the hub into each container:

```bash
HUB=https://lumen.lan
declare -A TOKENS=(
  [100]="lum_aaaa..."
  [101]="lum_bbbb..."
  [102]="lum_cccc..."
)
for vmid in "${!TOKENS[@]}"; do
  name=$(pct config $vmid | awk '/^hostname:/ {print $2}')
  pct exec "$vmid" -- sh -c "curl -fsSL ${HUB}/install.sh | sh -s -- --token ${TOKENS[$vmid]} --host ${name}"
done
```

### Pattern B — Ansible (direct binary download)

```yaml
- hosts: lumen_agents
  become: yes
  vars:
    lumen_tag: v0.6.5
    lumen_arch: amd64
    lumen_hub_url: https://lumen.lan
  tasks:
    - name: Install Lumen agent binary
      get_url:
        url: "https://github.com/quanla93/lumen/releases/download/{{ lumen_tag }}/lumen-agent-linux-{{ lumen_arch }}"
        dest: /usr/local/bin/lumen-agent
        mode: "0755"
        force: yes

    - name: Create agent config directory
      file: { path: /etc/lumen-agent, state: directory, mode: "0700" }

    - name: Write agent environment
      copy:
        dest: /etc/lumen-agent/agent.env
        mode: "0600"
        content: |
          LUMEN_HUB_URL={{ lumen_hub_url }}
          LUMEN_AGENT_HOST={{ inventory_hostname }}
          LUMEN_AGENT_TOKEN={{ agent_token }}
          LUMEN_AGENT_INTERVAL=5s
          LUMEN_AGENT_BUFFER_PATH=/var/lib/lumen-agent/buffer.db

    - name: Ensure buffer directory exists
      file: { path: /var/lib/lumen-agent, state: directory, mode: "0700" }

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
          EnvironmentFile=/etc/lumen-agent/agent.env
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

`agent_token` is per-host (Ansible vault, `group_vars/host_vars`).

## Docker container collection

If the host runs Docker, the agent lists and stats its containers automatically — no extra config needed; the Docker socket at `/var/run/docker.sock` is read directly. The shipped unit runs the agent as `root`, which is sufficient.

To disable container collection: stop Docker, or override the socket env var to a non-existent path:

```bash
sudo systemctl edit lumen-agent
# Add:
[Service]
Environment=LUMEN_AGENT_DOCKER_SOCKET=/path/to/your/docker.sock
```

(`systemctl edit` creates a drop-in override at `/etc/systemd/system/lumen-agent.service.d/override.conf` so the main unit stays clean.)

## Upgrade

Either re-run the install script (Path A) or swap the binary manually (Path B):

```bash
# Path A — re-run, idempotent:
curl -fsSL https://YOUR-HUB/install.sh | sudo sh

# Path B — manual swap:
sudo systemctl stop lumen-agent
sudo curl -fsSL "https://github.com/quanla93/lumen/releases/download/v0.6.6/lumen-agent-linux-amd64" \
  -o /usr/local/bin/lumen-agent
sudo chmod +x /usr/local/bin/lumen-agent
sudo systemctl start lumen-agent
```

Do not create a new host or rotate the token for a code update — the existing token in the systemd unit / YAML config keeps working.

See [How-to — Update agents](../../how-to/update-agents/#binary--systemd-agents-no-docker) for the full reference including failure modes and tagged-release rollback.

## Uninstall

```bash
sudo systemctl disable --now lumen-agent
sudo rm /etc/systemd/system/lumen-agent.service /usr/local/bin/lumen-agent
sudo rm -rf /var/lib/lumen-agent /etc/lumen-agent
sudo systemctl daemon-reload
```

Don't forget to delete the host record in the hub UI (**Settings → Hosts → trash icon**) — the row keeps its `last_seen_at` stale marker otherwise.

## Config file (YAML)

The install script writes a YAML config at `/etc/lumen-agent/lumen-agent.yaml` and points the unit at it via `EnvironmentFile`. You can use the same pattern with Path B for fleet config-management:

```yaml
# /etc/lumen-agent/lumen-agent.yaml — mode 0600 root:root (token is sensitive)
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

Field names map 1:1 to env vars (`hub_url` → `LUMEN_HUB_URL`, etc.). Override `LUMEN_AGENT_CONFIG` to point at a different path; a missing file is a quiet no-op so env-only setups keep working.

**Precedence (highest first)**:

1. Process env (`Environment=` in the systemd unit)
2. This YAML file
3. `.env` in the agent's CWD (dev only)
4. Hardcoded defaults

A malformed file is fatal at boot — half-loaded config is worse than loud failure. Run `lumen-agent` once interactively to sanity-check after edits.

## Limits and quirks

**Per-core CPU and containers are live-only**
: Both flow through the WebSocket stream but are NOT persisted to SQLite. After a hub restart the host detail page repopulates them on the agent's next tick (≤5s).

**Temperature unavailable in containers**
: `/sys/class/hwmon` is not exposed inside LXC by default. The temperature chart hides itself when no point > 0°C. To make it visible inside a privileged LXC, mount the directory in your Proxmox config:

```
lxc.mount.entry: /sys/class/hwmon sys/class/hwmon none bind,optional,ro 0 0
```

**Multiple agents on the same host**
: Possible but unusual. Each `systemd` unit must have a different name (`lumen-agent@<id>.service` via templating). Mint one token per agent. Useful only if you want to monitor namespaces separately on the same kernel — in 95% of cases one agent per host is right.

## Troubleshooting

**Agent logs `connect: connection refused` on every tick**
: Wrong `LUMEN_HUB_URL`. Confirm the URL is reachable *from the target host*, not just from your laptop. Pass `--hub` to `install.sh` to override.

**Agent logs `hub returned 401: invalid token`**
: Token revoked / rotated in the UI after install. Mint a new one and re-run install with `--token`.

**Agent logs `hub returned 401: Authorization: Bearer <token> required`**
: No token passed to install. Re-run with `--token lum_...`.

**Host card never appears on dashboard**
: Check the hub's `journalctl -u lumen-hub -f` — if you see `ingest accepted`, the agent is talking but the dashboard WS may be disconnected. Hard-reload the browser.
