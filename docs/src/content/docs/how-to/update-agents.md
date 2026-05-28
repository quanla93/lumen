---
title: Update Docker agents
description: Update Compose-managed Lumen Docker agents without creating new hosts or rotating tokens.
sidebar:
  order: 3
---

Agent updates are separate from host/token onboarding. A code update must reuse the existing host record and token.

For Docker agents, the recommended Lumen flow is a per-agent `docker-compose.yml` saved on the target VM/LXC. The token is shown once by the hub, then stored in that compose file on the target host.

## Rules

- Do not create a new host for a code update.
- Do not create or rotate a token for a code update.
- Do not replay the one-shot token reveal from the UI.
- Do not edit the hub's own `docker-compose.yml`.
- Run update commands on the VM/LXC where the agent's compose file lives.

## Recommended update

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

After rotating in the hub UI, edit the target host's `/opt/lumen-agent/docker-compose.yml` and replace `LUMEN_AGENT_TOKEN`, then run:

```bash
cd /opt/lumen-agent
sudo docker compose up -d
```

## docker run fallback

`docker run` agents can still be used for quick tests, but they are harder to update because the token/config lives in the container configuration instead of a compose file.

For long-running agents, recreate them as Compose-managed agents with the generated `docker-compose.yml` from the hub UI.
