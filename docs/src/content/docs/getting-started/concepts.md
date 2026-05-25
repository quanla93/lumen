---
title: Concepts
description: The four ideas you need to understand Lumen — Hub, Agent, Token, Tier.
sidebar:
  order: 3
---

Lumen is small on purpose. There are only four concepts to learn.

## Hub

The central server. It:

- Accepts metric pushes from agents.
- Stores them (in memory → SQLite → Parquet).
- Serves the web dashboard.
- Streams live updates to connected browsers over WebSocket.
- Evaluates alert rules.

You run **one** hub per fleet you want a unified view of. Typical sizing: 1-200 hosts per hub.

## Agent

A small binary that runs on each machine you want to monitor. It:

- Collects host metrics (CPU, RAM, disk, network, load, temperature).
- Optionally collects Docker container metrics.
- Optionally collects LXC / QEMU metrics (Proxmox).
- Pushes batches to the hub every 5 seconds (configurable).
- Buffers locally if the hub is unreachable.

The agent only sends data **out**. It never accepts inbound connections. This is what makes Lumen NAT-friendly.

### Special case: Proxmox host

For monitoring the Proxmox node itself (not the LXCs/VMs on it), Lumen can read the **Proxmox API** directly without installing an agent on the host. See [Proxmox integration](/integrations/proxmox/).

## Token

Each host has a **per-host bearer token** issued by the hub. The agent uses this token to authenticate when pushing metrics.

- Tokens look like `lum_AbCdEf123456...` (64 bytes).
- Shown **once** when you create the host — copy it immediately.
- Can be rotated from the UI if compromised.
- Scoped to one host — leaking one token doesn't leak others.

There is a separate user/password login for the UI itself.

## Tier

Lumen uses **three storage tiers** to balance speed, retention, and disk wear:

| Tier | Where | Resolution | Default retention | Use |
|---|---|---|---|---|
| **RAM** | In-memory ring buffer | 5-second raw | 15 minutes | Live dashboard, charts |
| **Hot** | SQLite (WAL) | 1-minute | 24 hours | Recent history queries |
| **Cold** | Parquet (ZSTD) | 5-minute | 30 days (config: up to 365) | Long-range queries |

Data moves through tiers automatically — you don't manage this directly. You can adjust retention per tier in [Settings → Retention](/configure/retention/).

## Next

- [Quickstart](/getting-started/quickstart/) — try it.
- [Architecture deep dive](/reference/architecture/) — how the pieces fit together.
