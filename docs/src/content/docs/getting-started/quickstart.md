---
title: Quickstart
description: Get a Lumen hub and one agent running in 5 minutes.
sidebar:
  order: 2
---

This guide gets you from zero to live metrics in about five minutes. We'll run the **hub** on one machine and connect a single **agent** to it.

## Prerequisites

- A Linux server (or VM) with **Docker** and **Docker Compose** for the hub.
- One more Linux machine to monitor (can be the same host for testing).
- Network: the agent must be able to reach the hub over HTTP/HTTPS (port 8090 by default).

## 1. Start the hub

On your hub machine:

```bash
mkdir -p ~/lumen && cd ~/lumen
curl -fsSL https://get.lumenhq.dev/compose -o docker-compose.yml
docker compose up -d
```

Check it's running:

```bash
docker compose ps
# lumen-hub   Up   0.0.0.0:8090->8090/tcp
```

Open <a href="http://localhost:8090">http://localhost:8090</a> (or `http://<server-ip>:8090`).

## 2. Create the admin account

The first time you open the UI, you'll see a setup screen. Choose a username and a strong password. This account has full admin rights.

:::caution
There is no "forgot password" recovery flow yet — write it down.
:::

## 3. Add your first host

In the UI:

1. Go to **Hosts** → **Add host**.
2. Give it a name (e.g. `pve-01`, `vps-tokyo`, `nas`).
3. Click **Create**. A token is shown **once** — copy the install command displayed.

Example install command:

```bash
curl -fsSL https://get.lumenhq.dev/agent | sudo bash -s -- \
  --hub https://lumen.example.com \
  --token lum_AbCdEf123456789
```

## 4. Install the agent

Run the command from step 3 on the machine you want to monitor. The installer:

- Downloads the agent binary for your architecture.
- Creates a `lumen` user.
- Writes config to `/etc/lumen-agent/config.yml`.
- Installs and starts a systemd service `lumen-agent`.

Verify:

```bash
sudo systemctl status lumen-agent
# Active: active (running)
```

## 5. See your metrics

Back in the UI, your host's status dot should turn **green** within 10 seconds. Click it to see live CPU, RAM, disk, and network charts.

You're done. 🎉

## Troubleshooting

**Status dot stays red**
: Check the agent log: `sudo journalctl -u lumen-agent -f`. Most common cause is a wrong `--hub` URL (typo, missing scheme) or a firewall blocking outbound.

**`connection refused`**
: The hub is unreachable. Check `curl -v https://lumen.example.com` from the agent host.

**`401 unauthorized`**
: The token is wrong. Re-create the host in the UI (tokens are not recoverable; you must rotate).

**`permission denied /var/run/docker.sock`**
: If you want Docker monitoring, add the agent's user to the `docker` group: `sudo usermod -aG docker lumen && sudo systemctl restart lumen-agent`.

More fixes: [How-to: Debug agent not connecting](/how-to/debug-agent-not-connecting/).

## What's next

- [Add a Proxmox host](/integrations/proxmox/) — Lumen's signature integration.
- [Configure alerts](/configure/alerts/) — Discord, Telegram, ntfy.
- [Set retention](/configure/retention/) — adjust how long metrics are kept.
- [Run behind a reverse proxy](/configure/reverse-proxy/) — Caddy / Nginx / Traefik examples.
