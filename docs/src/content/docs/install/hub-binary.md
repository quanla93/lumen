---
title: Hub — binary install
description: Install Lumen hub as a native Linux service (no Docker required).
sidebar:
  order: 1
---

This is the path when you want the hub running as a regular Linux service —
on bare metal, a VM, or a Proxmox LXC — without Docker.

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

- Linux x86_64 / aarch64 / armv7 with systemd (Debian 11+, Ubuntu 22.04+,
  Alpine, RHEL/Rocky 9, Proxmox LXC running any of those).
- Outbound HTTPS (only for `apt`/`curl` during install — the hub itself
  doesn't need internet).
- Root for the install step. After install the hub runs as the unprivileged
  `lumen` user.

## 1. Get the tarball

Pre-built tarballs are produced by `make release-hub-tarballs` on any
machine with Go + pnpm. While the project is in pre-v0.1 and the
[github.com/quanla93/lumen](https://github.com/quanla93/lumen) repo is
private, you build the tarball yourself once and ship it to every target.

```bash
# On a build machine (Mac or Linux) with Go 1.25+, pnpm, node 20+:
git clone https://github.com/quanla93/lumen.git
cd lumen
make build-web                          # builds + stages the embedded UI
make release-hub-tarball-amd64          # or -arm64 / -armv7
# → dist/lumen-hub-linux-amd64.tar.gz (~5 MB, contains binary + installer + unit)
```

## 2. Copy to the target

```bash
scp dist/lumen-hub-linux-amd64.tar.gz root@my-host:/tmp/
```

For a Proxmox LXC see also [Proxmox LXC walkthrough](./hub-lxc).

## 3. Install

```bash
ssh root@my-host
cd /tmp
tar xf lumen-hub-linux-amd64.tar.gz
cd lumen-hub-linux-amd64
sudo ./install-hub.sh
```

The installer is idempotent and:

1. Creates a system user `lumen`.
2. Creates `/etc/lumen/` (mode 0750, root:lumen).
3. Creates `/var/lib/lumen/` (mode 0750, lumen:lumen).
4. Generates a random 32-byte hex `LUMEN_HUB_SECRET` and writes it
   into `/etc/lumen/hub.env`.
5. Installs the binary to `/usr/local/bin/lumen-hub`.
6. Drops `/etc/systemd/system/lumen-hub.service`.
7. Reloads systemd, enables, and starts the unit.
8. Prints `http://<your-host>:8090` — open it.

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

## 4. First sign-in

The installer seeded an admin account from the env file's defaults:

| Field | Default |
|---|---|
| Username | `admin` |
| Password | `lumenadmin` |

**Change this password before exposing the hub on a network**. Two paths:

**Through the UI (recommended)** — sign in once, then add a proper account
via Settings → Hosts (full account management UI ships in slice B).

**By rotating the seed and re-running** — only on a brand-new install:

```bash
sudo systemctl stop lumen-hub
sudo $EDITOR /etc/lumen/hub.env       # change LUMEN_HUB_ADMIN_PASSWORD
sudo rm /var/lib/lumen/lumen.db       # ONLY on a fresh install — wipes everything
sudo systemctl start lumen-hub
```

(The env-seed only fires when the user doesn't exist; if you've already
logged in once the seed is a no-op even after `hub.env` changes.)

## 5. Next: install an agent

A hub with no agents shows nothing. From any other Linux box:

```bash
# In the hub UI: Settings → Hosts → Create "my-server" → copy the lum_... token.
# Then on the target:
curl -fsSL http://<hub-host>:8090/install.sh | sudo bash -s -- \
  --token lum_xxxxxxxxxxxxxxxxxxxxx \
  --host my-server
```

See [Agent — Linux](./agent-linux) for details, fleet patterns, and
uninstall.

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
- No network capabilities; if you change `LUMEN_HUB_ADDR` to a port
  below 1024 add `CAP_NET_BIND_SERVICE` or — much better — put a
  reverse proxy in front.

## Upgrade

Build a new tarball, scp it, re-run the installer:

```bash
tar xf lumen-hub-linux-amd64.tar.gz
cd lumen-hub-linux-amd64
sudo ./install-hub.sh   # in-place: replaces binary, preserves /etc/lumen + /var/lib/lumen
```

The migration framework (`pressly/goose`) auto-applies any new schema
changes on startup. WAL pragmas keep the DB online during the upgrade.

## Uninstall

```bash
sudo /tmp/lumen-hub-linux-*/install-hub.sh --uninstall   # keep config + DB
sudo /tmp/lumen-hub-linux-*/install-hub.sh --purge       # also wipe /etc/lumen + /var/lib/lumen + lumen user
```

If you ran the script from a different directory, re-extract the tarball
or invoke the now-uninstalled installer — `--uninstall` only needs the
script itself, not the binary.

## Putting HTTPS in front

The hub speaks plain HTTP on `:8090`. For anything reachable from outside
your trusted network, terminate TLS at a reverse proxy:

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

Same shape works with Caddy (it does WS upgrade by default), Traefik,
HAProxy, and Cloudflare Tunnel.

## Troubleshooting

**`lumen-hub failed to start`**
: `journalctl -u lumen-hub -n 50 --no-pager` — usually a port collision
on `:8090` or a malformed `LUMEN_HUB_SECRET` (must be ≥64 hex chars).

**Sessions die every restart**
: `/etc/lumen/hub.env` has `LUMEN_HUB_SECRET=` empty. The installer
generates one on first run; if you wiped it (or set it empty for
testing) every restart mints a fresh secret and invalidates every
cookie. Set it once, stable.

**`open /var/lib/lumen/lumen.db: permission denied`**
: The data dir got mangled (wrong owner). Fix:
  `sudo chown -R lumen:lumen /var/lib/lumen && sudo chmod 0750 /var/lib/lumen`.

**Want to bind to a privileged port (`:443`)**
: Either edit the unit to add `AmbientCapabilities=CAP_NET_BIND_SERVICE`
and the matching `CapabilityBoundingSet`, or — recommended — keep the
hub on `:8090` behind nginx/Caddy.

**Behind Cloudflare Tunnel (`cloudflared`)**
: That's why HTTPS-only push exists. Just point `cloudflared` at
`http://127.0.0.1:8090`; the agents reach the hub via the public tunnel
URL set in their `LUMEN_HUB_URL`.
