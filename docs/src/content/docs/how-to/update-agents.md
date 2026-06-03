---
title: Update agents
description: Update Lumen agents (Docker Compose or systemd binary) without creating new hosts or rotating tokens.
sidebar:
  order: 3
---

Agent updates are separate from host/token onboarding. A code update must reuse the existing host record and token. Two install paths, two update procedures — pick the one matching how the agent was set up on the target host.

## Rules (both paths)

- Do not create a new host for a code update.
- Do not create or rotate a token for a code update.
- Do not replay the one-shot token reveal from the UI.
- Do not edit the hub's own deploy files.
- Run update commands on the VM/LXC where the agent lives.

## Docker Compose agents

The recommended Lumen flow is a per-agent `docker-compose.yml` saved on the target VM/LXC. The token is shown once by the hub, then stored in that compose file on the target host.

### Recommended update

SSH into the target VM/LXC and run:

```bash
cd /opt/lumen-agent
sudo docker compose pull
sudo docker compose up -d
```

This pulls the latest agent image and recreates the container with the same token/config from `docker-compose.yml`.

## Check status

```bash
cd /opt/lumen-agent
sudo docker compose ps
sudo docker compose logs -f
```

The host should report again within one collection interval.

## Restart without updating

```bash
cd /opt/lumen-agent
sudo docker compose restart
```

## Roll back to a pinned image

If `latest` is not desired, edit the compose file and pin a known-good tag:

```yaml
services:
  lumen-agent:
    image: ghcr.io/quanla93/lumen-agent:0.1.3
```

Then apply it:

```bash
cd /opt/lumen-agent
sudo docker compose pull
sudo docker compose up -d
```

## If the token changes

Token rotation is a security action, not an update action. Rotate only if the token leaks or you intentionally want to invalidate the old credential.

After rotating in the hub UI, download or copy the newly generated per-agent compose file, replace the target host's `/opt/lumen-agent/docker-compose.yml`, then run:

```bash
cd /opt/lumen-agent
sudo docker compose up -d
```

## docker run fallback

`docker run` agents can still be used for quick tests, but they are harder to update because the token/config lives in the container configuration instead of a compose file.

For long-running agents, recreate them as Compose-managed agents with the generated `docker-compose.yml` from the hub UI.

## Binary + systemd agents (no Docker)

Agents installed through the one-liner install script (`curl -fsSL https://YOUR-HUB/install.sh | sudo sh`) run as a `lumen-agent.service` systemd unit with the binary at `/usr/local/bin/lumen-agent` and its config at `/etc/lumen-agent/lumen-agent.yaml`. The token sits in that yaml — restarting or updating the binary does **not** require a new token.

### Restart only (no code change)

```bash
sudo systemctl restart lumen-agent
sudo journalctl -u lumen-agent -f
```

The host should report again within one collection interval.

### Update the binary

The hub serves the matching agent binary at `/install/lumen-agent-linux-{amd64,arm64}`. Use the architecture of the LXC/VM the agent is running on:

```bash
# Stop the service so the binary isn't busy
sudo systemctl stop lumen-agent

# Pull the binary (pick the arch that matches `uname -m`)
sudo curl -fsSL https://YOUR-HUB/install/lumen-agent-linux-amd64 \
  -o /usr/local/bin/lumen-agent.new

# Atomic swap — install handles perms + replaces in one step
sudo install -m 0755 /usr/local/bin/lumen-agent.new /usr/local/bin/lumen-agent
sudo rm /usr/local/bin/lumen-agent.new

# Start + verify
sudo systemctl start lumen-agent
sudo journalctl -u lumen-agent -f
```

Or re-run the install script (idempotent — keeps the existing config + token, only replaces the binary and refreshes the unit file):

```bash
curl -fsSL https://YOUR-HUB/install.sh | sudo sh
```

### When the endpoint returns errors

- **404 Not Found** — the binary name in the URL is wrong. Use the *full* filename (`lumen-agent-linux-amd64`, not just `linux-amd64`).
- **503 Service Unavailable** — the hub is running but has no `LUMEN_HUB_INSTALL_DIR` set / no install assets on disk. The official `ghcr.io/quanla93/lumen-hub` images ship with `/install/` populated; a custom build needs `LUMEN_HUB_INSTALL_DIR=/path/to/install` pointing at a directory containing `install.sh` + the `lumen-agent-linux-{amd64,arm64}` binaries (use `make release-agents` plus `cp scripts/install-agent.sh dist/install.sh`, then mount that dir).
- **401 / 403** — the install endpoints are public on purpose (anyone who can reach your hub can already see the login page). If you've put the hub behind an auth proxy, allow the `/install*` paths through, or bypass the proxy for the update step.

### Rolling back the binary

The install script doesn't keep a backup. If a release breaks something:

```bash
sudo systemctl stop lumen-agent
# Pin a known-good tag from the GitHub Releases page
sudo curl -fsSL https://github.com/quanla93/lumen/releases/download/v0.6.4/lumen-agent-linux-amd64 \
  -o /usr/local/bin/lumen-agent
sudo chmod +x /usr/local/bin/lumen-agent
sudo systemctl start lumen-agent
```

This bypasses the hub entirely so it works even if the hub is currently shipping the bad version.
