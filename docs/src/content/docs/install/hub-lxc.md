---
title: Hub — Proxmox LXC
description: Install the Lumen hub inside a Proxmox LXC. Docker Compose with the official image, or the release tarball + systemd.
sidebar:
  order: 3
---

The recommended Proxmox deployment for the hub is a dedicated LXC. Both install shapes use only official artifacts — no `git clone`, no Go toolchain, no `make`.

| Shape | When to use | Disk | RAM |
|---|---|---|---|
| **A. Docker Compose inside LXC** | Recommended: fastest setup, easy update commands. | ~250 MiB image | 150 MiB |
| **B. Binary + systemd** | Smallest footprint, no Docker. | ~25 MiB binary + DB | 60–80 MiB |

Both shapes work on the same LXC; pick one to start.

## 1. Create the LXC (one time, on the Proxmox host)

Use the Proxmox UI **or** these `pct` commands. The LXC needs `nesting=1` for the Docker Compose shape. The binary shape does not.

```bash
# Pick a free VMID and download the Debian template once if needed.
pveam update
pveam available --section system | grep debian-12-standard
pveam download local debian-12-standard_12.7-1_amd64.tar.zst

# Create the container. Memory + disk are conservative for Docker Compose;
# bump to 1024 / 16 GB if you'll also keep long-term data here later.
pct create 200 local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst \
  --hostname lumen \
  --cores 1 --memory 512 \
  --rootfs local-lvm:8 \
  --net0 name=eth0,bridge=vmbr0,ip=dhcp,firewall=1 \
  --onboot 1 \
  --unprivileged 1 \
  --features nesting=1,keyctl=1   # only needed for Shape A (Docker)

pct start 200
pct enter 200
```

Inside the LXC, verify outbound network:

```bash
apt-get update
```

If `apt-get` resolves and downloads, you're good.

> Temperature readings are unavailable inside an LXC unless you bind-mount `/sys/class/hwmon` in the LXC config. The hub doesn't read temperatures itself — only agents do, and only if their host kernel exposes them.

## Shape A — Docker Compose inside LXC

This is the recommended fast path. Requires `features: nesting=1` on the LXC (set during `pct create` above, or add later via `pct set 200 --features nesting=1,keyctl=1` + reboot).

### A.1 Install Docker inside the LXC

```bash
apt-get install -y docker.io docker-compose-plugin curl
systemctl enable --now docker

# Smoke-test:
docker run --rm hello-world
```

If `hello-world` runs, Docker-in-LXC is wired correctly.

### A.2 Bring up the hub

Same compose template as on bare-metal Docker — see [Hub — Docker Compose](./hub-compose/) for the full file. The short version:

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

### A.3 Open the UI

Find the LXC IP:

```bash
ip -4 addr show eth0 | grep inet
# or from the Proxmox host: pct exec 200 ip -4 addr show eth0 | grep inet
```

Then `http://<lxc-ip>:8090`. Change the admin password from the UI before exposing the LXC to a network you don't trust.

## Shape B — Binary + systemd

Use this when you want the smallest footprint and no Docker. Skip the `--features nesting=1` flag in the `pct create` above; you don't need it.

### B.1 Download the release tarball

Inside the LXC:

```bash
TAG=v0.7.3
ARCH=amd64    # or arm64 if your Proxmox node is ARM (Ampere, etc.)

curl -fsSL \
  "https://github.com/quanla93/lumen/releases/download/${TAG}/lumen-hub-linux-${ARCH}.tar.gz" \
  -o /tmp/lumen-hub.tar.gz
```

If the LXC has no outbound HTTPS (air-gapped), download on the Proxmox host and push:

```bash
# On the Proxmox host:
curl -fsSL "https://github.com/quanla93/lumen/releases/download/${TAG}/lumen-hub-linux-amd64.tar.gz" \
  -o /tmp/lumen-hub.tar.gz
pct push 200 /tmp/lumen-hub.tar.gz /tmp/lumen-hub.tar.gz
```

`pct push` bypasses container networking entirely and works whether or not the LXC has SSH.

### B.2 Install

```bash
# Inside the LXC:
cd /tmp
tar xf lumen-hub.tar.gz
cd lumen-hub-linux-${ARCH}
./install-hub.sh
```

You should see the same installer output as in [Hub — binary § Install](./hub-binary#2-extract-and-install). The installer creates the `lumen` user, drops the systemd unit, generates a stable `LUMEN_HUB_SECRET`, and starts the service.

Open `http://<lxc-ip>:8090` and sign in with `admin / lumenadmin`. Change the password from the UI immediately.

## Note on agent scope

The agent shipped in either install shape, if you install one in this LXC, monitors **this LXC**, not the Proxmox host or other LXCs. It sees:

| Metric | Source |
|---|---|
| CPU / RAM / Disk | LXC cgroup view (LXCFS) |
| Containers | Docker daemon inside this LXC (its own siblings) |
| Network | LXC `eth0` |

For host-wide visibility, install the agent natively on the Proxmox host (see [Agent — Linux](./agent-linux/)) and point it at this hub.

For per-LXC visibility on every LXC in the cluster, install an agent inside each LXC. The fleet patterns in [Agent — Linux § Fleet install across many LXCs](./agent-linux/#fleet-install-across-many-lxcs) cover the install-script-per-LXC approach.

## Operations

| Task | Shape A — Docker | Shape B — systemd |
|---|---|---|
| Logs | `docker compose logs -f hub` | `journalctl -u lumen-hub -f` |
| Restart | `docker compose up -d --force-recreate hub` | `systemctl restart lumen-hub` |
| Edit config | `$EDITOR /opt/lumen-hub/docker-compose.yml` + restart | `$EDITOR /etc/lumen/hub.env` + restart |
| Backup DB | `docker run --rm -v lumen-hub_lumen-data:/data -v /backup:/out alpine cp /data/lumen.db /out/` | `cp /var/lib/lumen/lumen.db /backup/` (WAL — safe while hub runs) |
| Upgrade | `docker compose pull && up -d` | Download newer tarball, re-run `./install-hub.sh` |

## Backup the LXC

Use Proxmox's normal vzdump:

```bash
vzdump 200 --mode snapshot --storage local --compress zstd
```

For Shape B the DB is at `/var/lib/lumen/lumen.db` inside the LXC rootfs — captured. For Shape A the DB lives in the Docker volume `lumen-hub_lumen-data`, also inside the LXC rootfs — also captured.

## Snapshots before risky upgrades

```bash
pct snapshot 200 pre-lumen-upgrade --description "before v0.x.y bump"
# do upgrade
# if it goes wrong:
pct rollback 200 pre-lumen-upgrade
```

## Troubleshooting

**Shape A: `docker run hello-world` fails with `iptables: chain not found`**
: Nesting is off. Add it via the Proxmox host: `pct set 200 --features nesting=1,keyctl=1` then `pct reboot 200`.

**LXC has no `apt-get`**
: You picked a non-Debian template (Alpine, Ubuntu Core, …). The hub installer detects busybox `adduser` and works on Alpine; for other minimal images consult `install-hub.sh --help`.

**Wrong arch when downloading the tarball**
: `uname -m` tells you what to ask for. `x86_64` → `amd64`, `aarch64` → `arm64`. Mismatched binaries fail with `exec format error` on the first run.

**Pulled the latest image but it still runs an old version**
: After `docker compose pull`, you also need `docker compose up -d` so Docker recreates the container with the new image. `restart` keeps the old one.
