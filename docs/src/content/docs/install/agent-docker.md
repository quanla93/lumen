---
title: Agent — Docker
description: Run the Lumen agent as a sidecar container (compose or standalone) — minimal config and the macOS Docker Desktop socket quirk.
sidebar:
  order: 5
---

The agent runs equally well as a Docker container. This page covers
the two common shapes:

1. **Compose-managed alongside the hub** — single `docker compose up`,
   both services share a network. Already wired in the repo's
   `deploy/docker/docker-compose.yml`; described below for fleets that
   want to copy the snippet.
2. **Standalone container on a remote host** — one `docker run`
   command per box, pushing to a hub somewhere else.

If you don't already run Docker on the host you want to monitor, use
the [native install](/install/agent-linux/) instead — it's a 5 MB
binary, no container runtime overhead.

## Standalone container

```bash
docker run -d \
  --name lumen-agent \
  --restart unless-stopped \
  --user 0:0 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -e LUMEN_HUB_URL=https://lumen.example.lan \
  -e LUMEN_AGENT_TOKEN=lum_REPLACE_ME \
  -e LUMEN_AGENT_HOST=my-server \
  ghcr.io/lumenhq/lumen-agent:latest
```

In the product, this command is generated after you create a host token in Settings. Copy it and run it on the target machine; customers should not edit the hub compose file or add per-agent values to `.env` for the normal setup.

That's the whole setup. Two things to know:

**`--user 0:0`** is intentional. The Docker socket on Linux is
`root:root` mode 660; the distroless image's default `nonroot` user
(uid 65532) can't read it. Running as root inside the container
matches what cAdvisor / Beszel / node-exporter all do — the agent
only READS host resources, never writes, so the elevated UID is for
socket + `/proc` + `/sys` access, not privilege escalation.

**The Docker socket mount is optional**. Drop the `-v
/var/run/docker.sock` line if you don't want container telemetry —
host metrics (CPU, RAM, disk, network, temperature) still ship.

## Compose snippet

If your fleet already runs Docker Compose, drop this service into an
existing stack. The hub doesn't need to be in the same compose file —
just point `LUMEN_HUB_URL` at wherever it lives.

```yaml
services:
  lumen-agent:
    image: ghcr.io/lumenhq/lumen-agent:latest
    container_name: lumen-agent
    restart: unless-stopped
    user: "0:0"
    environment:
      LUMEN_HUB_URL: "https://lumen.example.lan"
      LUMEN_AGENT_TOKEN: "lum_REPLACE_WITH_UI_TOKEN"
      LUMEN_AGENT_HOST: "my-server"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
```

Prefer the generated `docker run` command from Settings for normal onboarding. Use compose only when you already template host-specific stacks in your own deployment tooling.

If the hub and agent live in the same compose project, replace the
URL with the compose service name (`http://hub:8090`) and drop the
public DNS dependency.

## macOS Docker Desktop quirk

Docker Desktop 4.x ships with the default Docker socket sharing
**disabled** for security. Without it, the bind mount silently no-ops
inside the agent container, and you'll see one warn line at startup:

```
WARN  docker containers unavailable — on macOS Docker Desktop, enable
      Settings → Advanced → "Allow the default Docker socket to be used
      (requires password)"
```

Host metrics keep working. To enable container telemetry on macOS:

1. Docker Desktop → ⚙️ Settings → **Advanced**
2. Toggle on **"Allow the default Docker socket to be used (requires
   password)"**
3. Click Apply & Restart; you'll be prompted for the macOS password
4. `docker compose restart lumen-agent` (or `docker restart
   lumen-agent` for standalone)

Linux + Docker on the host: works out of the box, no toggle needed.

## YAML config in a container

If you'd rather ship config as a file than env vars, mount a YAML
file at `/etc/lumen/agent.yaml`. The container looks there by
default; override with `LUMEN_AGENT_CONFIG=...`.

```bash
docker run -d \
  --name lumen-agent \
  --restart unless-stopped \
  --user 0:0 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v $PWD/agent.yaml:/etc/lumen/agent.yaml:ro \
  ghcr.io/lumenhq/lumen-agent:latest
```

See [Agent — Linux → Config file (YAML)](/install/agent-linux/#config-file-yaml)
for the field reference. Env vars still win when both are present, so
you can ship a fleet-wide YAML and override per-box with `-e`.

## Inspecting the container

The distroless image has no shell — `docker exec lumen-agent /bin/sh`
fails. Use the logs and the host's `docker stats` instead:

```bash
docker logs -f lumen-agent
```

If you genuinely need a shell to debug, build with `--target=debug`
locally or run the agent binary natively for the duration of the
investigation.

## Fleet pattern

A 20-host fleet: mint one token per host in the hub UI, then run each host's generated Docker command on that machine. If you automate fleet rollout, template those generated token values into your own deployment system; don't require operators to keep editing the hub's compose file.

If you'd rather mint tokens centrally and template them out, the hub
API supports `POST /api/hosts` and returns the bearer token in one
shot — see [API → Hosts](/reference/api/#hosts).
