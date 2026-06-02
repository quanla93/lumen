---
title: FAQ
description: Common questions about Lumen — what it monitors, who it's for, and what it deliberately doesn't do.
sidebar:
  order: 7
---

## What is Lumen, in one line?

A lightweight self-hosted monitoring tool for homelabs: HTTPS push,
HDD-friendly writes, a mobile-ready web UI, and a roadmap toward
Proxmox-native monitoring.

## Who is this for?

People running 1–50 hosts at home. Bare metal, VMs, Proxmox LXCs,
Docker. If your fleet fits on a shelf, Lumen is the right shape.

If you operate hundreds of hosts in a datacenter and already run
Prometheus + Grafana + Loki, Lumen isn't a replacement. It's
specifically tuned to be small enough to install in 30 seconds and
boring enough to forget about.

## Why another monitoring tool?

The three needles Lumen threads:

1. **Truly self-hosted, HTTPS-only**. Beszel uses SSH, Netdata uses
   a cloud parent-child, Glances has no auth at all. Lumen wants an
   agent that POSTs over HTTPS with a per-host bearer token — same
   shape as cAdvisor's metrics endpoint but with a hub on the
   receiving end.
2. **HDD-friendly storage**. SQLite WAL + 60s batched flushes,
   bounded retention. A future Parquet cold tier will let a
   Raspberry-Pi-class hub serve longer history from a USB HDD.
3. **Mobile-first dashboard**. Most homelab monitoring UIs are
   built desktop-first and look terrible on a phone. Lumen is the
   inverse — phone-readable first, desktop refinements second.

## What does Lumen monitor today?

Per host: CPU% (aggregate + per-core), RAM, swap, disk %, load
averages, network throughput, disk I/O throughput, temperature,
and per-container CPU + memory if Docker is reachable.

See [Metrics catalog](/reference/metrics-catalog/) for the full
list with units and gotchas.

## What's NOT in scope?

Locked anti-features, won't be added even if asked:

- **Log aggregation / full-text search.** Loki / Grafana / journalctl already do this well. Lumen's planned log surface is lightweight, on-demand operator debugging only.
- **Distributed tracing.** Tempo / Jaeger / OpenTelemetry land. Not
  a homelab problem.
- **Multi-tenant.** One hub, one operator. No org/team/role
  hierarchy.
- **Enterprise RBAC.** Username + password + bearer tokens today. Custom OIDC is deferred; SAML2 is only considered later if complexity stays acceptable.
- **Synthetic checks / blackbox monitoring.** Run `uptime-kuma`
  alongside; that's its scope.

## Why Go for the hub and agent?

- Single static binary, no runtime, no shared libraries.
- Cross-compiles to amd64 / arm64 / armv7 in one command — covers
  every homelab CPU.
- Low RAM (~30 MB hub, ~10 MB agent at idle) without much effort.
- gopsutil exists and works everywhere.

## Why SQLite, not Postgres / Influx / Timescale?

For homelab cardinality (1–50 hosts × 1 metric snapshot per 5s),
SQLite is fast enough and operationally invisible. No separate
database process to crash-recover, no port to fail2ban, no version
upgrade to plan. WAL mode + a 60s batched flush makes it
HDD-friendly even on a Raspberry Pi 4.

The planned Parquet cold tier will handle queries past the hot window;
SQLite stays as the hot layer. Today, old SQLite rows are deleted by
retention, while downsample settings are stored for the future cold-tier
compactor.

## Will Lumen work on Proxmox?

Yes. Today you can run the hub in a Proxmox LXC and install agents
inside Linux VMs/LXCs like any other host. The Proxmox API collector is
the planned Phase 5 / v0.4 wedge: ZFS pools, LXC vs QEMU distinction,
cluster quorum, and PBS backup status. See the
[ACTION_PLAN](https://github.com/quanla93/lumen/blob/main/ACTION_PLAN.md)
for the breakdown.

## Can I run the hub inside a Docker container? An LXC? Native?

All three. Walkthroughs:

- [Hub — Compose](/install/hub-compose/) — fastest, one command.
- [Hub — LXC](/install/hub-lxc/) — Proxmox-native, two shapes
  (binary or Docker-in-LXC).
- [Hub — Binary](/install/hub-binary/) — `install-hub.sh` on any
  Linux + systemd box.

For the agent: [native](/install/agent-linux/) (recommended) or
[Docker](/install/agent-docker/).

## How big does the SQLite file get?

Roughly 80 bytes per snapshot row. At default 5s ticks + 24h
retention:

| Hosts | Steady-state size |
|---|---|
| 1 | ~1.4 MiB |
| 10 | ~14 MiB |
| 50 | ~70 MiB |
| 200 | ~280 MiB |

WAL adds a few MB on top; reclaim with `VACUUM` if it bothers you
(see [Retention](/configure/retention/)).

## What happens if the hub goes down?

Agents keep collecting and buffer to a local bbolt file (default
24h cap, ~17 000 frames). When the hub comes back, agents drain the
backlog gradually — 10 frames per successful tick by default. No
operator action needed; data isn't lost unless the outage exceeds
the buffer's age limit.

See [Reliability](/configure/reliability/) for the full picture.

## What happens if an agent goes down?

The host disappears from the dashboard's "live" view and the
`last_seen_at` cosmetic stale marker turns gray. Historical data is
preserved as long as the retention window holds.

## Is the wire format stable?

Pre-v0.1.0 the API can break between commits — see the
[CHANGELOG](https://github.com/quanla93/lumen/blob/main/CHANGELOG.md).
At v0.1.0 the stable surface moves to `/api/v1/*` with backward
compatibility for one minor release.

## Can I send metrics from a non-Go agent?

Yes. Anything that can POST JSON to `/api/ingest` with the right
bearer token works. The [API reference](/reference/api/) documents
the envelope. We don't bundle a Python / Rust / Node client today
because the wire format is small enough that re-implementing
takes ~50 lines.

## Is there a Prometheus exporter?

Not as a native `/metrics` endpoint today. Lumen's **[Public Read API](https://github.com/quanla93/lumen/blob/main/docs/src/content/docs/reference/public-api.md)** (`/api/v1/*`, shipped in v0.5.0) is the integration surface — it works directly with the **Grafana Infinity datasource** (recipe in the Public API reference) so Grafana / n8n / scripts can pull host metrics with a Bearer key.

A native `/metrics` endpoint that exposes the hub's *own* counters in Prometheus exposition format is on the backlog — open an issue if you'd find it valuable.

## Why is X measured this way?

See the [Metrics catalog](/reference/metrics-catalog/) — every
field has a "source" + "semantics" line. If something still seems
off, open a [discussion](https://github.com/quanla93/lumen/discussions).

## How do I contribute?

Read [CONTRIBUTING.md](https://github.com/quanla93/lumen/blob/main/CONTRIBUTING.md).
TL;DR: open an issue first for anything non-trivial, follow
[Conventional Commits](https://www.conventionalcommits.org/),
the [CI](/contributing/ci-cd/) tells you what to fix.

## How do I get help?

- Bug or unexpected behavior → [GitHub Issues](https://github.com/quanla93/lumen/issues/new/choose)
- Question or how-do-I → [Discussions](https://github.com/quanla93/lumen/discussions)
- Security report → see [SECURITY.md](https://github.com/quanla93/lumen/blob/main/SECURITY.md);
  please don't open a public issue for vulnerabilities.

## What's the name about?

*Lumen* = a unit of luminous flux + the central cavity of a tube
(blood vessel, neuron, intestine). Both fit: you point a light at
your homelab and you look inside the tubes. Pronunciation: like
"loomin", not "lou-men".

The wordmark + lightbulb logo in `web/src/components/Logo.tsx` was
sketched while the architecture was being decided; if you find a
better one in a future redesign, fine, but the name is locked.
