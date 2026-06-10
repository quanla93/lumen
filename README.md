<div align="center">

# Lumen

**Proxmox-native monitoring for homelabs. HTTPS-only, HDD-friendly, mobile-ready.**

Lightweight self-hosted server monitoring with realtime dashboards and historical
metrics — designed to run comfortably on a Raspberry Pi.

[![CI](https://github.com/quanla93/lumen/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/quanla93/lumen/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/quanla93/lumen?sort=semver)](https://github.com/quanla93/lumen/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-14b8a6.svg)](LICENSE)
[![Discussions](https://img.shields.io/github/discussions/quanla93/lumen)](https://github.com/quanla93/lumen/discussions)
[![Good first issues](https://img.shields.io/github/issues/quanla93/lumen/good%20first%20issue?label=good%20first%20issues)](https://github.com/quanla93/lumen/labels/good%20first%20issue)

[Quickstart](docs/src/content/docs/getting-started/quickstart.md) ·
[Documentation](docs/src/content/docs/getting-started/overview.md) ·
[Public API](docs/src/content/docs/reference/public-api.md) ·
[Roadmap](ACTION_PLAN.md)

> ⚠️ **Pre-1.0.** Latest tag is **v0.7.3**. Breaking changes are allowed in
> minor releases until v1.0.

</div>

![Lumen dashboard — 8 homelab hosts with CPU / RAM / disk sparklines, hottest-first summary](brand/screenshots/01-dashboard.png)

<details>
<summary><b>More screenshots</b> — host detail · alerts · settings · light theme · mobile</summary>

| | |
|---|---|
| **Host detail** — drag/resize charts, 10-entry curated catalog | **Alerts** — threshold rules, 5 channel types, per-rule routing |
| ![host detail](brand/screenshots/02-host-detail.png) | ![alerts](brand/screenshots/05-alerts.png) |
| **Settings** — host tokens, runtime, retention, API keys | **Light theme** |
| ![settings](brand/screenshots/03-settings.png) | ![light theme](brand/screenshots/07-dashboard-light.png) |

**Mobile (installable PWA — no native app):**

<img src="brand/screenshots/06-mobile-dashboard.png" alt="Lumen mobile dashboard" width="320" />

</details>

---

## Why Lumen?

Most monitoring tools are built for enterprise clusters, not the homelab in your closet.
They eat RAM, hammer your HDD, and need three services just to start.
**Lumen does one thing: monitor your servers without getting in the way.**

### What makes Lumen different

The three wedges below are Lumen's north star — every feature serves at least one.
Each line is tagged **✅ shipped** or **🛣️ roadmap** so you know what works today
(see the [roadmap](ACTION_PLAN.md) for phase-by-phase detail).

- **HTTPS-only push transport** ✅ — Agents push out via HTTPS/WebSocket.
  Works behind any NAT, Cloudflare Tunnel, or Tailscale Funnel — no SSH key infrastructure required.
- **HDD-friendly storage** ✅ — Batched writes (60s flush ring) and WAL-tuned SQLite cut fsync
  pressure ~100× at fleet scale. Parquet hot/cold tiering is on the roadmap (Sprint 10, conditional).
- **Mobile-ready PWA** ✅ — Installable to your phone's homescreen; app shell paints instantly
  on cold start. **Web Push notifications** ✅ (VAPID) — alerts land on desktop Chrome/Edge/Firefox
  + Android + iOS Safari (PWA install) via a one-click subscribe inside Alerts → Channels.
- **Bilingual UI + docs** ✅ — Both the web app and the docs ship in English and Vietnamese.
- **Self-hosted SSO** ✅ — **OIDC** (Authentik, Keycloak, Google, Okta, Entra…) shipped in v0.7.0;
  **SAML2** (Okta classic, Azure AD enterprise, ADFS, Shibboleth) shipped in v0.7.2. Local password
  remains as a fallback so misconfiguration can't lock the admin out.
- **Per-host dashboard builder** ✅ (v0.6.0) — drag, resize, add, and remove charts on the
  Host detail page over a curated 10-entry catalog. The Dashboard host grid stays fixed-views.
- **First-run onboarding wizard** ✅ (v0.7.0) — 4-step guided flow: create admin → add first host →
  run the generated per-agent Compose → wait for first metrics. Replayable from Settings → Account.
- **Hardware-backed passkeys (WebAuthn)** ✅ (v0.7.0) — phishing-resistant, hardware-backed
  credential for the admin account. Local password + SSO + passkey all coexist.
- **Built-in public status page** ✅ (v0.7.0) — share a read-only `/status` URL of your homelab
  health. Per-host opt-in; safe defaults (nothing public until both flips are on).
- **Encrypted backups** ✅ (v0.7.1) — Local path or S3-compatible bucket (AWS S3, MinIO,
  Cloudflare R2, Backblaze B2, Wasabi). AES-256-GCM with Argon2id-derived key. Public spec so
  third-party tools can decrypt without Lumen running. CLI + Web UI restore.
- **Proxmox / LXC first-class** 🛣️ — Direct Proxmox API reads (agentless cluster/ZFS/PBS view)
  are deferred to ~v1. Proxmox guests (LXC, QEMU, Docker) work today via the per-host agent.

### What Lumen is NOT

So you can decide quickly if this is for you:

- ❌ Not a Grafana replacement — no query editor, no user-defined metrics, no arbitrary panels.
  The **Host detail** page has a drag/resize/add/remove builder over a curated 10-entry chart
  catalog (CPU, per-core, RAM, swap, disk, disk I/O, network, load, temperature, containers);
  the **Dashboard host grid** stays fixed views (sort + hide + saved views).
- ❌ Not for Kubernetes / microservices observability — use Prometheus + Grafana.
- ❌ Not multi-tenant or enterprise — single admin, optional read-only users (Sprint 8).
- ❌ Not a log aggregator — minimal log tail viewer only (no Loki/ELK).
- ❌ Not 1-year+ data retention — homelab focused (30-90 day default, 365d hard cap).
- ❌ No Windows agent — homelab fleet stays Linux + macOS (Windows dropped by design).

If those are dealbreakers, look at [Grafana + Prometheus](https://grafana.com) or [Netdata](https://www.netdata.cloud).

---

## Feature highlights

| | Lumen | Status |
|---|---|---|
| Hub footprint | Single Go binary, embedded web UI | ✅ |
| Agent footprint | Single Go binary | ✅ |
| Metrics | CPU (+ per-core), RAM, swap, disk, disk I/O, network, load, temperature, **GPU** (NVIDIA/AMD), **top-N processes** | ✅ |
| Storage | SQLite (hot), WAL-tuned, batched writes; Parquet cold tier planned | ✅ / 🛣️ |
| Transport | HTTPS / WebSocket (agent pushes outbound) | ✅ |
| Realtime | WebSocket fan-out with per-host subscribe filtering + auto-reconnect + server keepalive | ✅ |
| Containers | Docker container CPU / memory / state (live) | ✅ |
| Auth | First-admin register, JWT (HS256), Argon2id, per-host bearer tokens, **OIDC SSO**, **SAML2 SSO**, **WebAuthn/passkeys** | ✅ |
| Settings | Runtime agent interval, retention window/interval, downsample policy, **backup target + cron**, **alert maintenance windows** | ✅ |
| UI | Dashboard, host detail charts (uPlot), dark/light, EN + VI | ✅ |
| Personalization | Theme / language / units / reduce-motion / density saved per-user on the hub; Dashboard saved views (up to 5); per-host dashboard builder over a 10-entry chart catalog | ✅ |
| Public Read API | `/api/v1/*` Bearer-key authenticated endpoints (version / hosts / metrics / alerts) with scopes + host-glob filter + per-key rate limit; Grafana JSON datasource recipe in docs | ✅ |
| Deploy | Docker Compose (primary), single binary + systemd, install script | ✅ |
| Agent lifecycle | Per-agent Docker Compose, version awareness, in-UI "Update agent" guidance | ✅ |
| Public status page | Read-only `/status` URL with per-host opt-in (`/api/public/status`) | ✅ |
| Web Push | VAPID, AES-GCM-encrypted private key, per-browser subscriptions, 404/410 auto-prune | ✅ |
| Encrypted backups | Local + S3-compatible (S3/MinIO/R2/B2/Wasabi), AES-256-GCM, cron scheduler with hot-reload, CLI + Web UI restore | ✅ |
| Maintenance windows | Time-bounded alert suppression scoped by tag; alerts engine gate | ✅ |
| Auto-discovery | LXC, Proxmox VMs (agentless) | 🛣️ |
| Alerts | Threshold rules + offline detection; ntfy / Discord / Slack / webhook / Telegram / Email (SMTP) / **Web Push** delivery; per-rule routing, per-channel severity floor, host glob + tag selectors, persisted delivery queue with severity-aware retry, history + delivery scrollback, retention sweep, per-rule flap cooldown, per-host maintenance silence, **per-channel digest window**, **per-host share link** | ✅ |
| Multi-user + TOTP 2FA | Roles (admin / viewer), read-only users, TOTP 2FA + recovery codes | 🛣️ |
| Cold tier (Parquet) | >7d queries, multi-week retention, DuckDB query layer | 🛣️ (conditional on demand) |

---

## Quickstart

Docker Compose is the primary install path:

```bash
git clone https://github.com/quanla93/lumen
cd lumen
docker compose -f deploy/docker/docker-compose.yml up --build -d
```

Then open `http://localhost:8090`, create/sign in to the admin account, and go to
**Settings → Hosts**. Create a host, download/copy the generated per-agent
`docker-compose.yml`, place it on the target machine at `/opt/lumen-agent`, and
run `docker compose up -d` there.

For development from source, see [Run from source](docs/src/content/docs/contributing/run-from-source.md).
Full setup: see [Quickstart](docs/src/content/docs/getting-started/quickstart.md).

---

## Tech stack

- **Hub & agent** — Go 1.25+ (single binary each). [chi](https://github.com/go-chi/chi) router,
  [gopsutil/v4](https://github.com/shirou/gopsutil) metrics, [gorilla/websocket](https://github.com/gorilla/websocket),
  [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) (pure-Go, no cgo) with [goose](https://github.com/pressly/goose)
  migrations, Argon2id + JWT auth, [bbolt](https://github.com/etcd-io/bbolt) agent offline buffer.
- **Web** — React 18 + Vite + TypeScript + Tailwind, [uPlot](https://github.com/leeoniya/uPlot) charts,
  runtime i18n (EN/VI), PWA shell. Built and embedded into the hub binary via `embed.FS`.
- **Docs** — [Starlight](https://starlight.astro.build) (Astro), bilingual.

## Project layout

```
cmd/lumen-hub      Hub server entrypoint
cmd/lumen-agent    Agent entrypoint
internal/hub       Auth, hosts, ingest, storage (SQLite/goose), stream (WS), settings, retention
internal/agent     Collectors, sender, bbolt buffer, YAML/env config
web/               React + Vite SPA (embedded into the hub binary)
docs/              Starlight documentation site (EN/VI)
deploy/            Docker (Dockerfiles + compose) and systemd units
api/               OpenAPI 3.1 spec + .http REST Client examples
```

---

## Status

Lumen is **pre-1.0**. Expect breaking changes until v1.0. We aim for stable APIs after that.

| Version | State | Notes |
|---|---|---|
| v0.1 | ✅ Released | MVP: hub + agent, auth, Docker collector, realtime dashboard, history API, PWA |
| v0.2 | ✅ Released | Runtime settings, downsample policy, UI polish, bilingual (EN/VI) UI |
| v0.3 | ✅ Released | Docker Compose agent lifecycle: generated per-agent compose, agent version awareness, in-UI "Update agent" guidance |
| v0.4 | ✅ Released | Phase 6 alert engine end-to-end (rules, five channel types — ntfy/Discord/webhook/Telegram/Email — per-rule routing, host tag inventory, persisted delivery queue, retention sweep, flap cooldown, per-host maintenance silence). v0.4.7+ added no-Docker agent install + virt-aware UI + retention settings polish + Hub Status panel. |
| v0.5 | ✅ Released | Public Read API (`/api/v1/*` with Bearer keys, scopes, host-glob filter, in-memory rate limit, public envelope). Settings → API Keys mint/list/revoke. Grafana JSON datasource recipe in docs. |
| v0.6 | ✅ Released | Personalization (theme / language / units / reduce-motion / **density** on the hub, replacing localStorage), Dashboard **saved views** (up to 5 per user), per-host **dashboard builder** — drag/resize/add/remove charts over a 10-entry curated catalog (CPU, per-core, RAM, swap, disk, disk I/O, network, load, temperature, containers). |
| v0.7.0 | ✅ Released | **OIDC SSO** (Authentik, Keycloak, Google, Okta, Entra), **public status page** at `/status`, **Web Push** notifications (VAPID), **first-run onboarding wizard** (4-step), **WebAuthn/passkeys**, **Hub status** panel, **Per-host dashboard builder** stabilisation, Screenshots on the landing page. |
| v0.7.1 | ✅ Released | **Encrypted backups** — local + S3-compatible, AES-256-GCM, cron scheduler, CLI + Web UI restore. |
| v0.7.2 | ✅ Released | **SAML2 SSO** (Okta classic, Azure AD enterprise, ADFS, Shibboleth). |
| v0.7.3 | ✅ Released | **Beszel bundle 1** — GPU monitoring (NVIDIA + AMD), top-N process list, maintenance windows (alerts gate). |
| v0.7.4+ | Unreleased | **Notification quality** (Sprint 4: digest window, per-host share link, Slack-native channel, multi-recipient email) + **first-run onboarding polish** (Sprint 5) + **WebAuthn crypto-verify wiring** (Sprint 6 wrap-up). All code merged to Unreleased; tag pending. |
| v0.8+ | Planned | i18n polish (RFC 0007), multi-user + TOTP 2FA (RFC 0008), Grafana spike (RFC 0009), Cold tier conditional (Sprint 10). |
| v1.0 | Planned | API freeze (`/api/v1`), plugin SDK, Beszel migration tool, Proxmox agentless integration (deferred from earlier phase). |

See the full [roadmap](ACTION_PLAN.md) (phase-by-phase plan, decisions log, anti-features).

---

## Documentation

Docs are a Starlight site under [docs/](docs/) (English + Vietnamese).

**Getting started**
- **[Overview](docs/src/content/docs/getting-started/overview.md)** — What Lumen is and isn't
- **[Quickstart](docs/src/content/docs/getting-started/quickstart.md)** — Up and running locally
- **[Concepts](docs/src/content/docs/getting-started/concepts.md)** — Hub, agent, host, metric

**Install & operate**
- **[Hub (Docker Compose)](docs/src/content/docs/install/hub-compose.md)** · **[Hub (binary)](docs/src/content/docs/install/hub-binary.md)** · **[Hub on Proxmox LXC](docs/src/content/docs/install/hub-lxc.md)**
- **[Agent (Docker)](docs/src/content/docs/install/agent-docker.md)** · **[Agent (Linux)](docs/src/content/docs/install/agent-linux.md)**
- **[Add agents (Docker)](docs/src/content/docs/install/agent-docker.md)** · **[Add agents (Linux)](docs/src/content/docs/install/agent-linux.md)** · **[Update agents](docs/src/content/docs/how-to/update-agents.md)** · **[Use the web UI](docs/src/content/docs/how-to/use-the-web-ui.md)**

**Configure**
- **[Hosts & tokens](docs/src/content/docs/configure/hosts-and-tokens.md)** · **[Runtime settings](docs/src/content/docs/configure/runtime-settings.md)** · **[Retention](docs/src/content/docs/configure/retention.md)** · **[Reliability](docs/src/content/docs/configure/reliability.md)**

**Reference**
- **[Architecture](docs/src/content/docs/reference/architecture.md)** · **[API](docs/src/content/docs/reference/api.md)** · **[Public Read API](docs/src/content/docs/reference/public-api.md)** · **[Metrics catalog](docs/src/content/docs/reference/metrics-catalog.md)** · **[FAQ](docs/src/content/docs/faq.md)**
- **[ADR-0001: Storage](docs/adr/0001-storage-architecture.md)** · **[ADR-0002: Transport](docs/adr/0002-transport-choice.md)** · **[ADR-0003: Language](docs/adr/0003-language-choice.md)**

**Develop**
- **[Run from source](docs/src/content/docs/contributing/run-from-source.md)** · **[CI/CD](docs/src/content/docs/contributing/ci-cd.md)** · **[Contributing](CONTRIBUTING.md)**

Proxmox/LXC/ZFS/PBS integration guides land when the Proxmox wedge ships (deferred from v0.4 to a later release) — see the [roadmap](ACTION_PLAN.md).

---

## Contributing

We welcome contributions of every kind: code, docs, bug reports, ideas, and helping others.

- 🐛 **Bug?** [Open an issue](https://github.com/quanla93/lumen/issues/new/choose)
- 💡 **Idea?** [Start a discussion](https://github.com/quanla93/lumen/discussions)
- 🛠️ **Code?** Read [CONTRIBUTING.md](CONTRIBUTING.md) first

By participating you agree to our [Code of Conduct](CODE_OF_CONDUCT.md). See also
[GOVERNANCE.md](GOVERNANCE.md), [SECURITY.md](SECURITY.md), and [SUPPORT.md](SUPPORT.md).
Translating the UI to a new locale? Walk through the [translation guide](docs/src/content/docs/contributing/translating.md).

---

## License

[MIT](LICENSE) © Lumen contributors

Lumen is, and will remain, free and open source software.
