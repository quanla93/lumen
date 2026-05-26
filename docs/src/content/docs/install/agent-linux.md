---
title: Agent — Linux (native)
description: Install the Lumen agent on any Linux host via the hub-served one-liner.
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

## 2. Run the one-liner on the target

```bash
curl -fsSL http://<your-hub-host>:8090/install.sh | sudo bash -s -- \
  --token lum_xxxxxxxxxxxxxxxxxxxxx \
  --host pve-node-01
```

For an HTTPS hub:

```bash
curl -fsSL https://lumen.example.com/install.sh | sudo bash -s -- \
  --token lum_xxxxxxxxxxxxxxxxxxxxx \
  --host pve-node-01
```

The script:

1. Detects OS + architecture (`amd64` / `arm64` / `armv7`).
2. Downloads the matching `lumen-agent-linux-<arch>` binary from the
   hub's `/install/` endpoint.
3. Installs it to `/usr/local/bin/lumen-agent`.
4. Writes `/etc/systemd/system/lumen-agent.service` with the token in
   `Environment=` (file mode 0600, root-only).
5. `systemctl daemon-reload && systemctl enable --now lumen-agent`.

Within ~5 seconds the host card appears on the hub dashboard. Click it
to drill into per-core CPU, charts, and the container table.

### Flags

| Flag | Default | Notes |
|---|---|---|
| `--token <lum_...>` | _required_ | One-shot token from the hub UI |
| `--host <name>` | `$(hostname)` | Overridden server-side by the token's host |
| `--hub <url>` | baked into the script by the hub | Set explicitly if the agent reaches the hub via a different URL (Cloudflare Tunnel, internal IP) |
| `--interval <duration>` | `5s` | Go duration; `1s` to `1m` is reasonable |
| `--uninstall` | — | Stop, disable, remove binary + unit |

## 3. Verify

```bash
systemctl status lumen-agent
journalctl -u lumen-agent -f
```

You should see `msg=ingested cpu=… ram=… …` every interval. In the hub
UI, the host card transitions from "no data" to live values.

## Fleet install across many LXCs

When you have a Proxmox cluster full of LXCs, the friction is
"mint a token, ssh in, paste it" times N. Two patterns help:

### Pattern A — script loop from the Proxmox host

```bash
# On the Proxmox host. Mint one token per LXC in the hub UI first.
declare -A TOKENS=(
  [100]="lum_aaaa..."
  [101]="lum_bbbb..."
  [102]="lum_cccc..."
)
for vmid in "${!TOKENS[@]}"; do
  name=$(pct config $vmid | awk '/^hostname:/ {print $2}')
  pct exec $vmid -- bash -c "
    curl -fsSL http://hub.lan:8090/install.sh | bash -s -- \
      --token ${TOKENS[$vmid]} --host $name"
done
```

### Pattern B — Ansible

```yaml
- hosts: lumen_agents
  become: yes
  vars:
    lumen_hub_url: http://hub.lan:8090
  tasks:
    - name: Install Lumen agent
      shell: |
        curl -fsSL {{ lumen_hub_url }}/install.sh | bash -s -- \
          --token {{ agent_token }} \
          --host {{ inventory_hostname }}
      args:
        creates: /usr/local/bin/lumen-agent
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

Re-run the one-liner — the script is idempotent and replaces the binary
in place + restarts the service.

```bash
curl -fsSL http://<hub>:8090/install.sh | sudo bash -s -- \
  --token lum_... --host my-host
```

The token in `/etc/systemd/system/lumen-agent.service` is preserved
because it's read from your `--token` argument again.

## Uninstall

```bash
curl -fsSL http://<hub>:8090/install.sh | sudo bash -s -- --uninstall
```

Or, if the hub is offline:

```bash
sudo systemctl disable --now lumen-agent
sudo rm /etc/systemd/system/lumen-agent.service /usr/local/bin/lumen-agent
sudo systemctl daemon-reload
```

Don't forget to delete the host record in the hub UI (Settings →
Hosts → trash icon) — the row keeps its `last_seen_at` cosmetic stale
marker otherwise.

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
