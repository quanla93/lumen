---
title: Hub — binary + systemd
description: Run the Lumen hub as a native systemd service from the official release tarball. No Docker, no Go toolchain.
sidebar:
  order: 2
---

Run the hub as a regular Linux service from the official release tarball. Use this when you want the smallest footprint, no Docker, and standard systemd management. For the fastest default setup with Docker, use [Hub — Docker Compose](./hub-compose/).

You'll end up with:

| What | Where |
|---|---|
| Binary | `/usr/local/bin/lumen-hub` |
| Config | `/etc/lumen/hub.env` |
| Database | `/var/lib/lumen/lumen.db` (SQLite, WAL) |
| Service | `systemctl … lumen-hub` |
| Service user | `lumen` (system, no shell) |
| Logs | `journalctl -u lumen-hub -f` |

## Requirements

- Linux x86_64 / aarch64 with systemd (Debian 11+, Ubuntu 22.04+, RHEL/Rocky 9, Proxmox LXC running any of those).
- Outbound HTTPS to `github.com` for the download step. The hub itself doesn't need outbound internet at runtime.
- Root for the install step. After install the hub runs as the unprivileged `lumen` user.

## 1. Download the release tarball

Pick the tag you want from <https://github.com/quanla93/lumen/releases> and the arch that matches the host (`uname -m`: `x86_64` → `amd64`, `aarch64` → `arm64`):

```bash
TAG=v0.7.3
ARCH=amd64   # or arm64

curl -fsSL \
  "https://github.com/quanla93/lumen/releases/download/${TAG}/lumen-hub-linux-${ARCH}.tar.gz" \
  -o /tmp/lumen-hub.tar.gz
```

The tarball is ~6 MB. It contains the hub binary, the install script, and the systemd unit template — no Go toolchain or repo clone needed on this host.

## 2. Extract and install

```bash
cd /tmp
tar xf lumen-hub.tar.gz
cd lumen-hub-linux-${ARCH}
sudo ./install-hub.sh
```

The installer is idempotent and:

1. Creates a system user `lumen`.
2. Creates `/etc/lumen/` (mode 0750, root:lumen).
3. Creates `/var/lib/lumen/` (mode 0750, lumen:lumen).
4. Generates a random 32-byte hex `LUMEN_HUB_SECRET` and writes it into `/etc/lumen/hub.env`.
5. Installs the binary to `/usr/local/bin/lumen-hub`.
6. Drops `/etc/systemd/system/lumen-hub.service`.
7. Reloads systemd, enables, and starts the unit.
8. Prints the URL to open.

Sample output:

```
==> Creating system user lumen
==> Writing initial /etc/lumen/hub.env (random LUMEN_HUB_SECRET, default admin password)
==> Installing binary → /usr/local/bin/lumen-hub
==> Writing /etc/systemd/system/lumen-hub.service
==> Reloading systemd and starting lumen-hub
==> lumen-hub is running.
==> Open  http://my-host:8090
==> Logs  journalctl -u lumen-hub -f
==> Env   /etc/lumen/hub.env
```

## 3. First sign-in

The installer seeded an admin account from the env file's defaults:

| Field | Default |
|---|---|
| Username | `admin` |
| Password | `lumenadmin` |

**Change this password before exposing the hub on a network.** Sign in once via the UI, then change the password from Settings → Account.

If you need to re-seed (only on a brand-new install, before anyone logs in):

```bash
sudo systemctl stop lumen-hub
sudo $EDITOR /etc/lumen/hub.env       # change LUMEN_HUB_ADMIN_PASSWORD
sudo rm /var/lib/lumen/lumen.db       # destructive — wipes everything
sudo systemctl start lumen-hub
```

The env-seed only fires when the user record doesn't exist; if you've already logged in once the seed is a no-op even after editing `hub.env`.

## 4. Add an agent

A hub with no agents shows nothing. Pick an install path for each target machine:

- [Agent — install.sh one-liner](./agent-linux/#install-script) — fastest path for Linux LXCs / VMs / bare metal.
- [Agent — Docker Compose](./agent-docker/) — for hosts where Docker is already the deployment substrate.
- [Agent — binary + systemd](./agent-linux/) — for explicit control over the systemd unit.

## Operations

```bash
systemctl status lumen-hub                 # current state
systemctl restart lumen-hub                # after editing /etc/lumen/hub.env
systemctl stop lumen-hub                   # graceful shutdown
journalctl -u lumen-hub -f                 # live logs
journalctl -u lumen-hub --since '1h ago'   # recent
```

The systemd unit hardens the process via:

- `User=lumen` (unprivileged)
- `ProtectSystem=strict`, `ProtectHome=true`, `PrivateTmp=true`
- Only `/var/lib/lumen` is writable (`ReadWritePaths=`)
- Memory cap 512 MiB, task cap 128
- No network capabilities. To bind to a privileged port (`:443`), add `CAP_NET_BIND_SERVICE` — or, much better, put a reverse proxy in front and keep the hub on `:8090`.

## Upgrade

Download the newer tarball and re-run the installer:

```bash
TAG=v0.6.6
ARCH=amd64

curl -fsSL \
  "https://github.com/quanla93/lumen/releases/download/${TAG}/lumen-hub-linux-${ARCH}.tar.gz" \
  -o /tmp/lumen-hub.tar.gz
cd /tmp && tar xf lumen-hub.tar.gz && cd lumen-hub-linux-${ARCH}
sudo ./install-hub.sh   # replaces the binary, preserves /etc/lumen + /var/lib/lumen
```

`pressly/goose` migrations auto-apply on startup. WAL pragmas keep the DB online during the upgrade.

## Uninstall

```bash
# Keeps /etc/lumen + /var/lib/lumen:
sudo /tmp/lumen-hub-linux-${ARCH}/install-hub.sh --uninstall

# Also wipes config + DB + the lumen user:
sudo /tmp/lumen-hub-linux-${ARCH}/install-hub.sh --purge
```

`--uninstall` only needs the installer script itself — re-extract the tarball if you've cleaned up `/tmp`.

## Putting HTTPS in front

The hub speaks plain HTTP on `:8090`. For anything reachable from outside your trusted network, terminate TLS at a reverse proxy:

**Caddy** (simplest — auto-cert + WebSocket upgrade):

```text
lumen.example.com {
  reverse_proxy 127.0.0.1:8090
}
```

**nginx** (explicit WS upgrade):

```nginx
# /etc/nginx/sites-available/lumen
server {
  listen 443 ssl http2;
  server_name lumen.example.com;
  ssl_certificate     /etc/letsencrypt/live/lumen.example.com/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/lumen.example.com/privkey.pem;

  location / {
    proxy_pass http://127.0.0.1:8090;
    proxy_http_version 1.1;
    # WebSocket upgrade — /api/stream needs this:
    proxy_set_header Upgrade    $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host       $host;
    proxy_set_header X-Real-IP  $remote_addr;
    proxy_read_timeout 3600s;
  }
}
```

Same shape works with Traefik, HAProxy, Cloudflare Tunnel, Tailscale Funnel.

## Troubleshooting

**`lumen-hub failed to start`**
: `journalctl -u lumen-hub -n 50 --no-pager` — usually a port collision on `:8090` or a malformed `LUMEN_HUB_SECRET` (must be ≥64 hex chars).

**Sessions die every restart**
: `/etc/lumen/hub.env` has `LUMEN_HUB_SECRET=` empty or rotating between runs. The installer generates one on first run; if you wiped it every restart mints a fresh secret and invalidates every cookie.

**`open /var/lib/lumen/lumen.db: permission denied`**
: The data dir got the wrong owner. Fix:
`sudo chown -R lumen:lumen /var/lib/lumen && sudo chmod 0750 /var/lib/lumen`.

**Behind Cloudflare Tunnel (`cloudflared`)**
: That's why HTTPS-only push exists. Point `cloudflared` at `http://127.0.0.1:8090`; the agents reach the hub via the public tunnel URL set in their `LUMEN_HUB_URL`.
