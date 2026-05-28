---
title: Overview
description: What Lumen is, who it's for, and what it isn't.
sidebar:
  order: 1
---

Lumen is a **lightweight self-hosted monitoring platform** for homelabs and small infrastructure. It collects CPU, RAM, disk, network, and container metrics from your servers, stores them efficiently, and shows them in a realtime dashboard.

## Who Lumen is for

- You run a few servers — homelab, VPS, small office.
- You use **Proxmox**, Docker, or LXC.
- You want monitoring that sets up in 5 minutes, not 5 hours.
- You'd rather run a 60 MB process than a Prometheus + Grafana stack.
- You care about disk write amplification because you use HDDs.

## Who Lumen is NOT for

Honest is faster than oversell:

- ❌ **Kubernetes clusters / microservices** — use Prometheus + Grafana.
- ❌ **General-purpose observability** — Lumen doesn't do traces or full log search.
- ❌ **Custom dashboard building** — Lumen has fixed dashboards. If you want a dashboard builder, use Grafana.
- ❌ **Long-term metric archives (years)** — Lumen retains 30-90 days by default, capped at 365.
- ❌ **Enterprise multi-tenant** — single admin, optional read-only users.

## How Lumen is built

- **One hub** binary serves the API, the WebSocket realtime stream, and the React dashboard. ~60 MB RAM.
- **One agent** binary runs on each server you monitor. Pushes metrics to the hub over HTTPS. ~10 MB RAM.
- **SQLite** stores current hot data with WAL and batched writes. The **Parquet** cold tier is planned; downsample settings already exist so the compaction policy can land without changing the UI contract.

For the full system shape, see [Architecture](/reference/architecture/). Architecture decisions also live under [`docs/adr/`](https://github.com/quanla93/lumen/tree/main/docs/adr) in the repo.

## What's next

- [Quickstart](./quickstart.md) — run the hub with Docker Compose, then add an agent from the web UI.
- [Hub — Docker Compose](/install/hub-compose/) — the main install path for now.
- [Add agents](/how-to/add-agents/) — create a host and run the generated per-agent `docker-compose.yml`.
- [Use the web UI](/how-to/use-the-web-ui/) — navigate the dashboard, host detail, and settings.
- [Concepts](./concepts.md) — Hub, Agent, Token, Tier.

Advanced/manual guides are still available for [native hub install](/install/hub-binary/), [Proxmox LXC](/install/hub-lxc/), and [native Linux agents](/install/agent-linux/). Proxmox API integration is a planned product wedge tracked in the [roadmap](https://github.com/quanla93/lumen/blob/main/ACTION_PLAN.md).
