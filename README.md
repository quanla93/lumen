<div align="center">

# Lumen

**Proxmox-native monitoring for homelabs. HTTPS-only, HDD-friendly, mobile-ready.**

Lightweight self-hosted server monitoring with realtime dashboards, historical metrics,
and alerts — designed to run comfortably on a Raspberry Pi.

[Quickstart](docs/src/content/docs/getting-started/quickstart.md) ·
[Documentation](https://lumenhq.dev) ·
[Discord](#) ·
[Roadmap](docs/src/content/docs/reference/roadmap.md)

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

```bash
# Run the hub (Docker Compose)
curl -fsSL https://get.lumenhq.dev/compose > docker-compose.yml
docker compose up -d

# Open http://localhost:8090 and create the admin account.
# Then add a host in the UI to get an install command for the agent:

curl -fsSL https://get.lumenhq.dev/agent | sudo bash -s -- \
  --hub https://your-lumen.example.com \
  --token lum_xxxxxxxxxxxxx
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

See the full [roadmap](docs/src/content/docs/reference/roadmap.md).

---

## Documentation

- **[Getting started](docs/src/content/docs/getting-started/quickstart.md)** — Up and running in 5 minutes
- **[Install guide](docs/src/content/docs/install/)** — Hub, agent, Proxmox host
- **[Configuration](docs/src/content/docs/configure/)** — Hosts, alerts, retention, auth
- **[Integrations](docs/src/content/docs/integrations/)** — Proxmox, Docker, LXC, ZFS, PBS
- **[Reference](docs/src/content/docs/reference/)** — API, architecture, metrics catalog
- **[Contributing](CONTRIBUTING.md)** — How to help

---

## Contributing

We welcome contributions of every kind: code, docs, translations, bug reports, ideas, and helping others.

- 🐛 **Bug?** [Open an issue](https://github.com/lumenhq/lumen/issues/new/choose)
- 💡 **Idea?** [Start a discussion](https://github.com/lumenhq/lumen/discussions)
- 🛠️ **Code?** Read [CONTRIBUTING.md](CONTRIBUTING.md) first
- 🌍 **Translate?** See [translating.md](docs/src/content/docs/contributing/translating.md)

By participating, you agree to our [Code of Conduct](CODE_OF_CONDUCT.md).

---

## License

[MIT](LICENSE) © Lumen contributors

Lumen is, and will remain, free and open source software.
