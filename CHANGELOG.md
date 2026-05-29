# Changelog

All notable changes to Lumen will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Phase 6 / RFC 0001 Milestone A — threshold alerting and notifications. Operator-defined rules (CPU/RAM/swap/disk/load1 thresholds + `offline` rule with a 60s floor); per-(rule, host) state machine evaluated every ~15s (`LUMEN_HUB_ALERT_INTERVAL`, runtime-tunable via `alerts.eval_interval`); persisted `alert_events` history; new top-level **Alerts** tab with Active/History/Rules/Channels sub-tabs; **ntfy / Discord / webhook** channel dispatch with a synchronous **Send test** action.
- Phase 6 / RFC 0001 Milestone B — finer-grained routing and a new channel type. **Telegram channel** (Bot API, `bot_token` + `chat_id`, HTML message body, masked re-edit) is now a first-class option alongside ntfy/Discord/webhook. **Per-rule channel routing**: each rule picks the subset of channels it fans out to; leaving the picker empty preserves the Milestone-A broadcast-to-all behaviour. **Per-channel severity floor** (`min_severity` info/warning/critical) so a pager can ignore low-severity noise. **Host glob patterns** in rule `host` (`web-*`, `*-prod`, `db-[0-9]*`) via stdlib `path.Match`. New tables `alert_rule_channels` + `notification_channels.min_severity` (migration 0009).
- Phase 6 / RFC 0001 Milestone D — **persisted notification delivery queue with severity-aware retry**. Engine is now non-blocking: each (alert × channel) lands as a `pending` row in `notification_deliveries`; a background worker pool (default 4 goroutines, 1 s poll) drains them with per-channel serialisation so a single Discord webhook can't back-pressure the others. Bursts of 100+ alerts no longer block the engine ticker. **Severity-aware retry policy**: critical alerts retry fast and give up in ~5 minutes (5 s, 15 s, 1 min, 5 min — a 6-hour retry on paging-grade alerts is useless); warning/info back off longer (30 s → 4 h, 6 attempts). The Alerts tab gains a **Deliveries** sub-tab with per-status filter, severity filter, queued/sent timestamps, retry-now button for failed/dropped rows, and a summary chip strip showing queue depth at a glance. Migration 0011 adds the `notification_deliveries` table.
- Phase 6 / RFC 0001 Milestone C — **host tags and label selectors**. Each host carries a Kubernetes-style label set (up to 32 `key=value` tags, edited inline in Settings → Hosts via a chip editor). Alert rules gain a `host_selector` field (`tier=critical,env=prod`, AND semantics) that wins over the `host` name field when set. The rule `host` field also now accepts a **comma list** (`web-1,db-2,api-3`) so the UI can offer a multi-select agent picker, with the same value also being hand-typeable. New `host_tags` table + `alert_rules.host_selector` column (migration 0010); engine reads tags only when at least one enabled rule has a selector. `PUT /api/hosts/{id}/tags` exposes the same surface to scripts and the (future) Public API.

## [0.3.0] - 2026-05-29

Phase 4 release: Docker Compose agent lifecycle UX — compose-first onboarding, agent version awareness, and in-UI update guidance. Lightweight log management (dedicated Logs/Console surface) is deferred to a later release.

### Added

- Agent version awareness: agents report their build version in every ingest; new `GET /api/version` exposes the hub build, which equals the latest agent version since the hub and agent ship from the same release train. Host detail and the dashboard surface each host's running agent version and flag out-of-date agents.
- "Update agent" panel on host detail: the Compose update command (`docker compose pull && docker compose up -d`) with a copy button, an up-to-date/update-available status, and a note that the command must run on the agent's machine — not on the hub.
- Compose-first agent onboarding: the one-shot token reveal generates a complete per-agent `docker-compose.yml` (copy/download) plus the run and update commands; `docker run` remains a quick fallback.

### Fixed

- Build-version injection (`-ldflags -X main.Version`) now works for both the hub and agent binaries; it was silently stuck at `"dev"` because the injected symbol did not match the variable name. Published images (including `:latest`) now self-report the real release version.

## [0.2.0] - 2026-05-28

Phase 3 release: operator customization, UI polish, i18n foundation, and clarified lightweight log direction.

### Added

- Runtime agent collection interval policy from hub settings, with agent polling/apply path and env/YAML bootstrap defaults.
- Parquet downsample policy controls in settings for bucket size and hot/cold/archive windows ahead of the cold-tier implementation.
- Product-grade UI polish across app shell, dashboard, host detail, settings, reusable surfaces, empty states, and onboarding-oriented host actions.
- Bilingual web UI foundation with English and Vietnamese runtime strings plus persisted language toggle.
- System metadata in host detail headers for hostname/IP, OS, uptime, kernel/arch, CPU model, and agent version context.

### Changed

- Docker agent onboarding is hub-first: create a host in the UI, then use the generated per-agent Docker Compose file instead of editing hub compose or per-agent config manually.
- Lightweight logs are explicitly deferred to a future dedicated Logs/Console surface with on-demand live streaming; logs must not be shipped through periodic metrics ingest or Host Detail polling.

## [0.1.0] - 2026-05-27

Initial public MVP release.

### Added

- Phase 0 project bootstrap: README, MIT license, contribution guide, GitHub templates, CI, release workflow, CodeQL workflow, Makefile, docs scaffold, and ADR-0001.
- Phase 1 technical spike: Go hub and agent, ingest endpoint, WebSocket live stream, embedded web build, Docker Compose path, source-run docs, OpenAPI spec, and REST Client examples.
- Phase 2 MVP breadth: authentication, host/token management, SQLite migrations, HDD-friendly batched persistence, metrics history API, retention settings, offline agent buffer, Docker collector, YAML agent config, host detail charts, PWA shell, install docs, reference docs, and FAQ.
- OSS readiness docs: Code of Conduct, Governance, Security Policy, Support guide, ADR-0002, and ADR-0003.

### Changed

- CodeQL workflow is gated behind manual dispatch while the staging repository remains private.

### Fixed

- golangci-lint CI configuration updated for golangci-lint v2 and the current GitHub Action version.
