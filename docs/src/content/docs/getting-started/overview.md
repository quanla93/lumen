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
- **SQLite** stores recent data; older data rolls into compressed **Parquet** files.

A full Architecture reference doc lands with v0.2. Until then, architecture
decisions live under [`docs/adr/`](https://github.com/quanla93/lumen/tree/main/docs/adr)
in the repo.

## What's next

- [Quickstart](./quickstart.md) — build from source, see live CPU in two terminals.
- [Concepts](./concepts.md) — Hub, Agent, Token, Tier.

The Docker Compose / Proxmox / install guides land in v0.1 → v0.3 (see the
[roadmap](https://github.com/quanla93/lumen/blob/main/ACTION_PLAN.md)).
