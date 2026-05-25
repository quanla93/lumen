<div align="center">

# Lumen

**Proxmox-native monitoring for homelabs. HTTPS-only, HDD-friendly, mobile-ready.**

Lightweight self-hosted server monitoring with realtime dashboards, historical metrics,
and alerts — designed to run comfortably on a Raspberry Pi.

[Quickstart](docs/src/content/docs/getting-started/quickstart.md) ·
[Roadmap](ACTION_PLAN.md)

> ⚠️ **Pre-1.0 / pre-launch.** The project is being staged at
> [`quanla93/lumen`](https://github.com/quanla93/lumen) (private) until
> v0.1.0; the `lumenhq.dev` site, Discord, and installer URLs below are
> placeholders and don't exist yet.

</div>

---

## Why Lumen?

Most monitoring tools are built for enterprise clusters, not the homelab in your closet.
They eat RAM, hammer your HDD, and need three services just to start.
**Lumen does one thing: monitor your servers without getting in the way.**

### What makes Lumen different

- **Proxmox / LXC first-class** — Read Proxmox API directly (no agent needed on the host).
  See cluster topology, ZFS pools, PBS backups, LXC vs QEMU at a glance.
- **HDD-friendly storage** — Batched writes, WAL-tuned SQLite, hot/cold tiering with Parquet.
  Designed to keep your spinning rust quiet.
- **HTTPS-only push transport** — Agents push out via HTTPS/WebSocket.
  Works behind any NAT, Cloudflare Tunnel, Tailscale Funnel — no SSH key infrastructure required.
- **Built-in public status page** — Share a read-only URL of your homelab health, no extra tool needed.
- **Mobile-first PWA** — Install on your phone, get Web Push notifications natively.
- **Bilingual docs** — English and Vietnamese, first-class.

### What Lumen is NOT

So you can decide quickly if this is for you:

- ❌ Not a Grafana replacement — no dashboard builder, fixed views only.
- ❌ Not for Kubernetes / microservices observability — use Prometheus + Grafana.
- ❌ Not multi-tenant or enterprise — single admin, optional read-only users.
- ❌ Not a log aggregator — minimal log tail viewer only (no Loki/ELK).
- ❌ Not 1-year+ data retention — homelab focused (30-90 day default).

If those are dealbreakers, look at [Grafana + Prometheus](https://grafana.com) or [Netdata](https://www.netdata.cloud).

---

## Feature highlights

| | Lumen |
|---|---|
| Hub footprint | ~60 MB RAM, single binary |
| Agent footprint | ~10 MB RAM, single binary |
| Storage | SQLite (hot) + Parquet (cold), ZSTD compressed |
| Transport | HTTPS / WebSocket (push from agent) |
| Realtime | WebSocket fan-out, sub-second update |
| Auto-discovery | Docker, LXC, Proxmox VMs |
| Alerts | Discord, Telegram, ntfy, Email, Webhook |
| Deploy | Docker, Compose, single binary, LXC helper script |
| Auth | Username/password, JWT, optional TOTP, per-host tokens |

---

## Quickstart

The Docker Compose and `get.lumenhq.dev` installer flow shown below is the
target UX for v0.1.0. Today (pre-v0.1) you build from source:

```bash
git clone https://github.com/quanla93/lumen
cd lumen
cp .env.example .env
make dev-hub      # terminal 1
make dev-agent    # terminal 2
```

Future v0.1+ flow (not live yet):

```bash
curl -fsSL https://get.lumenhq.dev/compose > docker-compose.yml
docker compose up -d
# then add a host in the UI to copy the agent install command.
```

Full setup: see [Quickstart](docs/src/content/docs/getting-started/quickstart.md).

---

## Status

Lumen is **pre-1.0**. Expect breaking changes until v1.0. We aim for stable APIs after that.

| Version | State | Notes |
|---|---|---|
| v0.1 | 🚧 In development | MVP: hub + agent, Docker, realtime dashboard |
| v0.2 | Planned | LXC + Proxmox integration, alerts |
| v0.3 | Planned | Cold tier Parquet, multi-user, retention config |
| v1.0 | Planned | Stable API, plugin SDK |

See the full [roadmap](ACTION_PLAN.md) (phase-by-phase plan, decisions log, anti-features).

---

## Documentation

Currently available:

- **[Overview](docs/src/content/docs/getting-started/overview.md)** — What Lumen is and isn't
- **[Quickstart](docs/src/content/docs/getting-started/quickstart.md)** — Up and running locally
- **[Concepts](docs/src/content/docs/getting-started/concepts.md)** — Hub, agent, host, metric
- **[ADR-0001: Storage architecture](docs/adr/0001-storage-architecture.md)** — SQLite hot + Parquet cold
- **[Contributing](CONTRIBUTING.md)** — Dev setup, commit style, PR workflow

Install guide, configuration reference, integrations (Proxmox/LXC/ZFS/PBS),
and the API reference land in phases v0.1 → v0.3 — see the [roadmap](ACTION_PLAN.md).

---

## Contributing

We welcome contributions of every kind: code, docs, bug reports, ideas, and helping others.

- 🐛 **Bug?** [Open an issue](https://github.com/quanla93/lumen/issues/new/choose)
- 💡 **Idea?** [Start a discussion](https://github.com/quanla93/lumen/discussions)
- 🛠️ **Code?** Read [CONTRIBUTING.md](CONTRIBUTING.md) first

A translation guide and a formal Code of Conduct land before v0.1.0 (see
the [roadmap](ACTION_PLAN.md) Phase 0 deferred items).

---

## License

[MIT](LICENSE) © Lumen contributors

Lumen is, and will remain, free and open source software.
