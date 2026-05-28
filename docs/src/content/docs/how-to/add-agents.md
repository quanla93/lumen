---
title: Add agents
description: Three ways to ship a Lumen agent to a new machine — native binary (recommended), Docker container, or compose service.
sidebar:
  order: 2
---

Every machine you want to monitor runs **one Lumen agent**. The agent is a
single Go binary (~15 MB, no runtime deps) that POSTs a metrics snapshot
to the hub every `LUMEN_AGENT_INTERVAL`. Docker is **never required** on
the target — it's just one of three packaging options.

## The flow at a glance

```
┌─ Hub UI ──────────┐    ┌─ Target machine ────────────┐
│ Settings → Hosts  │    │ LUMEN_HUB_URL=...           │
│ "Add host"        │ →  │ LUMEN_AGENT_HOST=...        │
│ → token (1-shot)  │    │ LUMEN_AGENT_TOKEN=lum_...   │
└───────────────────┘    │ ./lumen-agent  (or systemd) │
                         └─────────────────────────────┘
```

The token shown in Settings is displayed **exactly once**. The hub stores
only its SHA-256 hash. If you lose it, click **Rotate** to mint a fresh
one — the old token immediately stops working.

## Which mode should I pick?

| Mode | When to use | Footprint on target |
|---|---|---|
| **[One-liner install](#the-one-liner-fastest-path)** | Linux + systemd target (covers 95 % of homelab) | ~15 MB binary, ~10 MB RAM |
| **[Native binary + manual systemd](#a-native-binary-manual-install)** | You want to inspect/customize the unit before installing | Same as above |
| **[Docker container](#b-docker-container)** | Target already runs Docker and you want one orchestration model | ~30 MB image, ~25 MB RAM |

**For Proxmox LXC specifically: pick the one-liner or native.** LXC is
already a container — nesting Docker inside it adds ~100 MB RAM overhead
and a privileged-flag requirement for no benefit.

---

## The one-liner (fastest path)

After clicking **Create** on a host in Settings → Hosts, the token panel
shows a ready-to-paste install command. On the target machine (as root):

```bash
curl -fsSL http://<hub-host>:8090/install.sh | sudo bash -s -- \
  --token lum_xxxxxxxxxxxxxxxxxxxxx \
  --host pve-node-1
```

What it does:

1. Detects OS + arch (`linux/amd64`, `linux/arm64`, `linux/armv7`).
2. Downloads the matching agent binary from **your hub** at `/install/lumen-agent-linux-<arch>`.
3. Installs to `/usr/local/bin/lumen-agent`.
4. Writes `/etc/systemd/system/lumen-agent.service` with the token + hub URL baked in (mode `0600` — token isn't world-readable).
5. `systemctl enable --now lumen-agent`.

Re-running upgrades the binary in place and restarts the service.
Idempotent.

**Uninstall:**

```bash
curl -fsSL http://<hub-host>:8090/install.sh | sudo bash -s -- --uninstall
```

**No public domain needed.** The script downloads from whatever URL you
ran `curl` against — LAN IP, mDNS name, Tailscale name, anything
reachable from the target. The hub bakes its own URL into `install.sh`
at render time (from the `Host` header you used).

**When the install endpoint is disabled.** If `/install.sh` returns
`503 install endpoint disabled`, the hub was built without staging the
binaries (e.g. you're running it natively without
`LUMEN_HUB_INSTALL_DIR`). Either use the [manual install below](#a-native-binary-manual-install)
or rebuild the hub Docker image (it stages binaries automatically).

---

## A. Native binary (manual install)

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

> 🚧 Pre-built release binaries (`lumen-agent_<version>_linux_amd64.tar.gz`)
> and an install one-liner ship with v0.1.0 (see [UI & deploy roadmap](#whats-coming-next)).

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

## B. Docker container

Useful when the target host already runs Docker. Create the host in
Settings first, then copy the generated Docker command and run it on
the target machine. The customer flow is token-first; don't edit the
hub compose file or add one `.env` variable per agent.

```bash
docker run -d --name lumen-agent \
  --restart unless-stopped \
  -e LUMEN_HUB_URL=http://10.0.0.10:8090 \
  -e LUMEN_AGENT_HOST=docker-node-2 \
  -e LUMEN_AGENT_TOKEN=lum_... \
  -e LUMEN_AGENT_INTERVAL=5s \
  lumen-agent:dev
```

**Caveat — host vs container metrics:** by default the agent reports
the *container's* cgroup-restricted view (CPU/RAM look tiny). To monitor
the **host** from a containerized agent, mount `/proc` and `/sys`:

```bash
docker run -d --name lumen-agent \
  --restart unless-stopped \
  --pid host \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys:ro \
  -e HOST_PROC=/host/proc \
  -e HOST_SYS=/host/sys \
  -e LUMEN_HUB_URL=http://10.0.0.10:8090 \
  -e LUMEN_AGENT_HOST=docker-node-2 \
  -e LUMEN_AGENT_TOKEN=lum_... \
  lumen-agent:dev
```

The `HOST_PROC` / `HOST_SYS` env vars are read by `gopsutil` directly.

---

## Rotating a token

If a token leaks (e.g. committed to git) or an agent moves to a new
machine and you want a fresh credential:

1. Hub UI → **Settings → Hosts** → click **Rotate** next to the host.
2. Copy the new `lum_…` shown once.
3. Update `LUMEN_AGENT_TOKEN` on the target and restart the agent
   (`systemctl restart lumen-agent` / `docker restart lumen-agent` /
   `docker compose up -d --force-recreate agent2`).

The old token starts returning `401` immediately. The host record
(metrics history, name, position on dashboard) is preserved.

---

## What's coming next

The fast paths are tracked in [ACTION_PLAN.md](https://github.com/quanla93/lumen/blob/main/ACTION_PLAN.md):

- **GitHub Release tarballs** — pre-built `linux/{amd64,arm64,armv7}`, `darwin/{amd64,arm64}`, `windows/amd64` so install doesn't depend on a reachable hub. Lands with **v0.1.0**.
- **LXC helper script** (Proxmox-flavor) — `pct exec <id> -- bash -c "$(curl ...)"` style, mirrors the [tteck pattern](https://github.com/community-scripts/ProxmoxVE). Lands with **v0.2.0** (Proxmox wedge).
- **Bulk-add** — paste a list of hosts in Settings, get one token per host. Phase 2 stretch.

---

## Troubleshooting

**Agent logs `ingest send failed: hub returned 401: invalid token`**
: The token doesn't match anything in the `hosts` table. Re-check
`LUMEN_AGENT_TOKEN` (no leading/trailing whitespace), or rotate.

**Agent logs `ingest send failed: connection refused`**
: `LUMEN_HUB_URL` is wrong or the hub isn't reachable from the target.
Test with `curl <LUMEN_HUB_URL>/healthz`.

**Host appears on the dashboard but CPU is always 0**
: Containerized agent without `--pid host` + `/proc` mount. See [mode B](#b-docker-container).

**Two agents share the same token — what happens?**
: Both succeed at ingest, but every snapshot overwrites the previous
one because the hub keys metrics by **host name from the token**, not
the agent's self-reported name. You'll see CPU jumping erratically.
Mint a separate token per host.
