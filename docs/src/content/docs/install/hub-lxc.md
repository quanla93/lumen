---
title: Hub — Proxmox LXC
description: Install Lumen hub inside a Proxmox LXC container, preferably with Docker Compose for the fastest path.
sidebar:
  order: 2
---

The recommended Proxmox deployment for the hub is a dedicated LXC. For now, the fastest default is Docker Compose inside that LXC; native systemd remains available as an advanced/manual path.

| Shape | When to use | Disk | RAM |
|---|---|---|---|
| **A. Docker Compose inside LXC** | Recommended first path: quick setup, easy update/restart/log commands, same Docker flow as agents. | ~250 MiB image | 150 MiB |
| **B. Native binary + systemd** | Advanced/manual path when you want the smallest footprint and no Docker. | ~25 MiB image + DB | 60–80 MiB |

Both shapes work on the same LXC; pick one to start.

## 1. Create the LXC (one time, on the Proxmox host)

Use the Proxmox UI **or** these `pct` commands. The LXC needs `nesting=1` for the recommended Docker Compose shape. Native systemd does not need it.

```bash
# Pick a free VMID and download the Debian template once if needed.
pveam update
pveam available --section system | grep debian-12-standard
pveam download local debian-12-standard_12.7-1_amd64.tar.zst

# Create the container. Memory + disk are conservative for Docker Compose;
# bump to 1024 / 16 GB if you'll also store cold tier here later.
pct create 200 local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst \
  --hostname lumen \
  --cores 1 --memory 512 \
  --rootfs local-lvm:8 \
  --net0 name=eth0,bridge=vmbr0,ip=dhcp,firewall=1 \
  --onboot 1 \
  --unprivileged 1 \
  --features nesting=1,keyctl=1   # needed for the recommended Docker Compose shape

pct start 200
pct enter 200
```

Inside the LXC you should land at a root shell. Verify outbound network:

```bash
apt-get update
```

If `apt-get` resolves and downloads, you're good.

## Shape B — Native binary + systemd

Use this advanced/manual path when you want the smallest footprint and no Docker. This is identical to a bare-metal Debian install. The only LXC-specific
note: temperature readings are unavailable inside an LXC because
`/sys/class/hwmon` is not mounted by default. The agent reports
`temp_c=0` — the temperature chart simply hides itself.

### A.1 Get the tarball

On your **build machine** (Mac/Linux with Go + pnpm — not inside the LXC):

```bash
git clone https://github.com/quanla93/lumen
cd lumen
make build-web
make release-hub-tarball-amd64   # or -arm64 if your Proxmox node is ARM
```

You now have `dist/lumen-hub-linux-amd64.tar.gz` (~5 MiB).

### A.2 Push into the LXC

The easiest path uses `pct push` from the Proxmox host:

```bash
# On the Proxmox host:
pct push 200 dist/lumen-hub-linux-amd64.tar.gz /tmp/lumen-hub.tar.gz
```

(`pct push` works whether or not the LXC has SSH, and bypasses
container networking entirely.)

Alternative: `scp` if SSH is enabled in the LXC.

### A.3 Install

```bash
# Back inside the LXC:
cd /tmp
tar xf lumen-hub.tar.gz
cd lumen-hub-linux-amd64
./install-hub.sh
```

You should see the same output as in [hub-binary](./hub-binary#3-install).
Open `http://<lxc-ip>:8090` from a browser on the same network.

### A.4 Find the LXC IP

```bash
# Inside the LXC:
ip -4 addr show eth0 | grep inet

# Or from the Proxmox host:
pct exec 200 ip -4 addr show eth0 | grep inet
```

### A.5 Done — log in

`admin / lumenadmin` is the seeded admin. Change it via the UI before
opening this LXC to a network you don't control.

## Shape A — Docker Compose inside LXC

This is the recommended fast path. It requires `features: nesting=1` on the LXC (set during `pct create` above,
or add later via `pct set 200 --features nesting=1,keyctl=1` + reboot).

### B.1 Install Docker inside the LXC

```bash
apt-get install -y docker.io docker-compose-plugin git curl
systemctl enable --now docker

# Smoke-test:
docker run --rm hello-world
```

If `hello-world` runs, Docker-in-LXC is wired correctly.

### B.2 Clone Lumen and bring up the stack

```bash
cd /opt
git clone https://github.com/quanla93/lumen
cd lumen
cp .env.example .env

# Set a stable secret + a real admin password before starting:
sed -i "s|^LUMEN_HUB_SECRET=.*|LUMEN_HUB_SECRET=$(openssl rand -hex 32)|" .env
$EDITOR .env    # change LUMEN_HUB_ADMIN_PASSWORD

docker compose -f deploy/docker/docker-compose.yml up -d
```

### B.3 Open the UI

`http://<lxc-ip>:8090` — same URL shape as the native path.

### B.4 Note on agent scope

The agent shipped in the compose stack monitors **this LXC**, not the
Proxmox host or other LXCs. It sees:

| Metric | Source |
|---|---|
| CPU / RAM / Disk | LXC cgroup view (LXCFS) |
| Containers | Docker daemon inside this LXC (i.e. its own siblings) |
| Network | LXC `eth0` |

For host-wide visibility, install the agent natively on the Proxmox host
(see [Agent — Linux](./agent-linux)) and point it at this hub — or wait
for Phase 3 (Proxmox API client, agentless multi-LXC enumeration).

## Operations

| Task | Docker Compose | Native systemd |
|---|---|---|
| Logs | `docker compose logs -f hub` | `journalctl -u lumen-hub -f` |
| Restart | `docker compose restart hub` | `systemctl restart lumen-hub` |
| Edit config | `$EDITOR .env` + `up -d --force-recreate hub` | `$EDITOR /etc/lumen/hub.env` + restart |
| Backup DB | `docker run --rm -v lumen_lumen-data:/data -v /backup:/out alpine cp /data/lumen.db /out/` | `cp /var/lib/lumen/lumen.db /backup/` (hub-running OK — WAL) |
| Upgrade | `git pull && docker compose up -d --build hub` | `tar xf newer.tar.gz && ./install-hub.sh` |

## Backup the LXC

Use Proxmox's normal vzdump:

```bash
vzdump 200 --mode snapshot --storage local --compress zstd
```

For native systemd the DB is at `/var/lib/lumen/lumen.db`; vzdump captures it along with everything else. For Docker Compose the DB lives in the Docker volume `lumen_lumen-data`, also inside the LXC rootfs → also captured.

## Snapshots before risky upgrades

```bash
pct snapshot 200 pre-lumen-upgrade --description "before v0.x.y bump"
# do upgrade
# if it goes wrong:
pct rollback 200 pre-lumen-upgrade
```

## Troubleshooting

**Docker Compose: `docker run hello-world` fails with `iptables: chain not found`**
: Nesting is off. Add via Proxmox host:
  `pct set 200 --features nesting=1,keyctl=1` then `pct reboot 200`.

**Docker Compose: agent inside the LXC reports zero containers**
: Same docker.sock permission story as on the Mac. The agent must run
  as root to read the LXC's docker socket. The shipped compose already
  sets `user: "0:0"` on the agent service.

**LXC has no `apt-get`**
: You picked a non-Debian template (Alpine, Ubuntu Core, …). The
  installer detects busybox `adduser` and works on Alpine; for other
  minimal images consult `install-hub.sh --help`.

**Hub binary built on Mac doesn't run inside the LXC**
: Wrong arch. Build with the right target: `make
  release-hub-tarball-amd64` for Intel Proxmox nodes, `-arm64` for
  Ampere/Pi-style nodes.
