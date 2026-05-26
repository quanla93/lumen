# Lumen hub — release tarball

You've extracted `lumen-hub-linux-<arch>.tar.gz`. Here's what's inside:

| File | Purpose |
|---|---|
| `lumen-hub` | The hub binary (web UI is embedded — no separate frontend bundle to ship). |
| `install-hub.sh` | One-shot installer: creates `lumen` user, installs to `/usr/local/bin`, writes systemd unit, starts service. Idempotent + supports `--uninstall` / `--purge`. |
| `lumen-hub.service` | systemd unit installed at `/etc/systemd/system/lumen-hub.service`. |
| `hub.env.example` | Environment template copied to `/etc/lumen/hub.env` on first install (preserved on upgrade). |

## Quick start

```bash
# As root on the target Linux machine:
sudo ./install-hub.sh
```

That's it. The installer:

1. Creates a system user `lumen` (no shell).
2. Creates `/etc/lumen/hub.env` (random `LUMEN_HUB_SECRET`, default admin `admin / lumenadmin`).
3. Creates `/var/lib/lumen/` (where SQLite lives, owned by the `lumen` user).
4. Installs the binary at `/usr/local/bin/lumen-hub`.
5. Writes the systemd unit, reloads, enables, and starts it.
6. Prints `http://<your-host>:8090` — open that, sign in, you're done.

## Change the bootstrap admin password before exposing

The `lumen / lumenadmin` default is **for local testing**. Before opening the
hub on a network:

```bash
sudo systemctl stop lumen-hub
sudo $EDITOR /etc/lumen/hub.env       # change LUMEN_HUB_ADMIN_PASSWORD
sudo rm /var/lib/lumen/lumen.db       # only on a brand-new install, to re-seed
sudo systemctl start lumen-hub
```

(If you've already created accounts via UI, those survive — `LUMEN_HUB_ADMIN_*`
only seeds when the user doesn't exist.)

## Upgrade in place

```bash
tar xf lumen-hub-linux-amd64.tar.gz  # newer release
cd lumen-hub-linux-amd64
sudo ./install-hub.sh                # re-running is an in-place upgrade
```

`/etc/lumen/hub.env` and the SQLite DB are preserved.

## Uninstall

```bash
sudo ./install-hub.sh --uninstall   # keep DB + config
sudo ./install-hub.sh --purge       # also wipe /etc/lumen + /var/lib/lumen
```

## Logs and ops

```bash
journalctl -u lumen-hub -f          # live logs
systemctl status lumen-hub          # service state
systemctl restart lumen-hub         # apply env changes
```

## Next: add agents

Hub by itself shows nothing — agents push the data. From any other Linux box:

```bash
# 1. On the hub UI (the one you just opened):
#    Settings → Hosts → Create "my-server" → copy the lum_... token

# 2. On the target machine (any Linux), as root:
curl -fsSL http://<your-hub-host>:8090/install.sh | sudo bash -s -- \
  --token lum_xxxxxxxxxxxxxxxxxxxxx \
  --host my-server
```

That's installable in any Proxmox LXC, bare-metal host, or VM.

See `docs/install/agent-linux.md` and `docs/install/hub-lxc.md` for the full
walk-through.
