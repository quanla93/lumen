# Changelog

All notable changes to Lumen will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.4.7] - 2026-06-01

No-Docker install path, virt-aware per-core CPU, silence UX bigger.

### Added

- **Binary + systemd install one-liner.** New `Binary + systemd` tab next to Docker in the token reveal panel — `curl http://<hub>/install.sh | sudo bash -s -- --token X --host Y` (HUB_URL auto-baked from request Host header). Hub cross-builds `lumen-agent-linux-{amd64,arm64}` and serves them at `/install/{binary}`. GitHub raw fallback included for hub-firewalled targets.
- **`system.virt_type` reported by the agent.** New field from gopsutil's `host.Info().VirtualizationSystem` — `"kvm"`, `"lxc"`, `"docker"`, `"wsl"`, … or empty on bare metal. Migration 0015 adds the column.
- **"Until I lift it" silence preset (1 year).** 5th option in both the HostDetail SilencePanel select and the per-alert-row popover. Server silence cap bumped from 7 days → 1 year.
- **Silence visibility on Dashboard + HostDetail.** Dashboard HostCard shows a `VolumeX` icon next to silenced host names; HostDetail hero gets a warn-tinted "Alerts silenced" pill. No more opening SilencePanel just to find out why an agent went quiet.

### Changed

- **Per-core CPU hidden on guest hosts.** Strip collapses to a one-line note when `virt_type` is non-empty. LXC shares kernel with Proxmox host (per-core reflects sibling LXC load, not this agent); KVM vCPUs don't isolate on oversubscribed nodes. Bare-metal hosts (empty virt_type, including older agents that don't report it) keep the grid.
- **Cross-build matrix trimmed to amd64 + arm64.** armv7 dropped from Dockerfile, CI, and Makefile aggregates — zero real users. ~30% faster hub image build, ~15 MB smaller. Per-arch Makefile targets stay for one-off armv7 builds.

## [0.4.6] - 2026-06-01

Stuck-alerts fix + Alerts UI full pass. A real operator-pain bug (firing events that never auto-resolved after the underlying rule was disabled or the hub restarted mid-firing) is closed at the source, and every tab of the Alerts section gets the visual + interaction treatment Rules already got in v0.4.5 — inline Switch toggles, quick-create templates, sectioned forms, and per-row quick actions (silence host from Active, retry from Deliveries).

### Fixed

- **Firing alerts now auto-resolve when their rule is disabled.** `UpdateRule` was a plain UPDATE — flipping `enabled` from true→false stopped the engine from ticking the rule, but any live firing rows in `alert_events` had nothing left to drive their resolved transition. They sat in Active forever until either re-enabling the rule or hand-editing the DB. `UpdateRule` now runs inside a tx that detects the true→false transition and marks firing events resolved + drops their pending deliveries — same pattern `DeleteRule` already uses. Closing the gap in three places at once: the live transition above; a one-shot boot sweep that resolves any pre-existing firing events whose rule is currently disabled (covers state from before this fix landed); plus engine boot now hydrates `ruleState.eventID` from existing firing rows so a restart mid-firing doesn't lose the row reference and silently skip the eventual resolve transition.

### Changed

- **Alerts UI redesign across all six tabs.**
  - **Rules:** inline Switch on each row with optimistic update (no more "open form → tick Enabled → Save" 3-click pause). Quick-template chip strip above the list (CPU > 80, RAM > 90, Disk > 85, Host offline, Load > 4) prefills the new-rule form so the 80% case starts with one click instead of 11 blank fields. Form regrouped into Condition / Targeting / Notification sections; comparator and severity use SegmentedControl; enabled is a Switch in a labeled card. Row layout: metric icon tinted teal/muted by enabled state, hover-revealed Edit/Delete IconButtons.
  - **Channels:** same Switch + IconButton + sectioned form treatment (Identity / Configuration / Routing & state), with channel-type icons (Megaphone, MessagesSquare, Webhook, Send, Mail) on rows and inside the Config section header.
  - **Active / History:** severity stripe on the left edge, severity-tinted state icon (BellRing for firing, CheckCircle2 for resolved). Each firing row has a hover-reveal `VolumeX` IconButton that pops a quick-silence panel (15m / 1h / 4h) wired to the existing `hostsApi.silence` endpoint. Rows whose host has an active silence get a "silenced" pill.
  - **Deliveries:** rows are roughly half their previous height. Single mono meta line — `STATUS · attempts · http · queued/sent · next retry` — replaces the prior three-line stack. Channel-type icon next to channel name; inflight status spins.
  - **Tags:** pane headers gain teal Tag/Server icons; "New tag" becomes a Ghost + Plus button to match Rules/Channels; row actions become hover IconButtons.
- **Sidebar footer cleanup.** Three stacked rows (username label, lang/theme/logout, collapse toggle) collapsed into a user pill (avatar + name + logout) over a single utility row (lang, theme, collapse on the right). Collapsed state mirrors with a vertical stack of avatar / theme / logout / expand.
- **Chart fill gradient anchors to series max, not chart bbox.** `gradientFill` in HostDetail used the chart's full bbox as the gradient stop range, so fixed-scale charts (CPU/RAM/Disk on 0–100, Disk I/O on its auto-scale) drew the line near the low-alpha end and the Grafana-style fill was invisible. Now the strong-alpha stop sits at the series' actual max value pixel — every chart shows a visible fill below the line regardless of scale.

## [0.4.5] - 2026-05-31

Phase 6 wrap-up. Email (SMTP) joins the channel lineup and two cooperating alert-noise levers land together: per-rule flap cooldown (rule-level, "this rule itself flaps") and per-host maintenance silence (operator-level, "I'm about to restart the agent — please be quiet"). With these, Phase 6.x is closed; remaining items (template polish, tag rename, derived metrics, webhook HMAC, fleet-summary pre-agg) move to a "post-Phase-6 backlog" pending real user demand.

### Added

- **Email (SMTP) notification channel.** Fifth channel type alongside ntfy/Discord/webhook/Telegram. Config: `smtp_host`, `smtp_port`, `username`, `password` (masked on read like the Telegram bot token), `from_addr`, `to_addr` (single recipient; multi-recipient deferred). Dispatcher uses `net/smtp` over a context-aware `net.Dialer` so the engine's dispatch timeout / cancellation propagates; PLAIN auth runs over STARTTLS (port 587) or implicit TLS (port 465). No new dependency — `net/smtp` + `crypto/tls` are stdlib. Docs: `configure/alerts.md` gets a full Email section with Gmail / Outlook / SendGrid / SES setup recipes, troubleshooting table for SMTP errors the Send-test button surfaces, and a swaks one-liner for credential sanity-check outside Lumen.
- **Per-rule flap cooldown.** New `alert_rules.cooldown_seconds` column (migration 0013, default 0 = preserves pre-cooldown behaviour). Engine tracks `ruleState.lastFiredAt`; firing transitions inside the cooldown window flip `firing=true` (so the next resolve still emits a notification) but skip both the `alert_events` insert and the delivery queue insert — flap-prone rules stay out of both the channel AND the history table. Rule form gains a "Cooldown (seconds)" field next to "For (seconds)".
- **Per-host maintenance silence.** New `hosts.silenced_until` column (migration 0014, nullable unix epoch). Engine refreshes silence map each `runOnce` (SQL pre-filters past values); evaluate skips firing + resolved transitions for silenced hosts AND leaves `firing=false` so the rule re-evaluates from scratch after silence expires. New `POST /api/hosts/{id}/silence` (body `{seconds}`, max 7 days) + `DELETE /api/hosts/{id}/silence`; HostDetail page gets a SilencePanel with 15m / 1h / 4h / 24h presets and a "Lift silence" button while a silence is active. Covers planned-maintenance workflows like `docker compose pull && docker compose up -d` that briefly trip the offline rule.

### Fixed

- **Email dispatcher: only AUTH on encrypted connection.** Initial dispatcher blindly called `c.Auth(...)` whenever the server advertised the AUTH extension. MailHog (and similar dev relays) advertise AUTH PLAIN but don't actually require it — and Go's `net/smtp.PlainAuth` refuses to send credentials over plaintext (`unencrypted connection` error). Now AUTH only runs after a confirmed encrypted connection (implicit TLS on 465 OR a successful STARTTLS upgrade). MailHog / unencrypted dev relays work transparently; real production relays (Gmail, SES, SendGrid) keep authenticating exactly as before because they all have TLS. The narrow loss is "internal relay that requires AUTH but doesn't offer TLS" — that misconfiguration now surfaces as the relay's own `530 5.7.0` at `MAIL FROM`, which is a clearer signal than swallowing the operator's creds.

## [0.4.4] - 2026-05-31

### Fixed

- **Copy buttons now work over plain HTTP.** The dashboard's "copy compose / copy token / copy update command" buttons silently no-op'd (or threw, in TokenReveal's case) when the operator loaded the UI at a LAN IP like `http://192.168.x.y:8090` — `navigator.clipboard.writeText()` requires a secure context (HTTPS or `localhost`) and is undefined elsewhere. The biggest hit was TokenReveal: the one-shot agent token was effectively unrecoverable from the UI on plain HTTP without manual text selection. New `copyToClipboard` helper tries the modern API first then falls back to the off-screen-textarea + `document.execCommand("copy")` legacy path that still ships in every browser as of 2026 (Grafana / Vault / Gitea use the same fallback for the same reason). When HTTPS is eventually put in front of the hub, the modern path transparently takes over.

## [0.4.3] - 2026-05-31

Release-pipeline cleanup + lint follow-up. v0.4.2 was tagged but its
multi-arch image build was cancelled mid-flight (~25 min into QEMU
emulation) when the operator confirmed the fleet is 100% x86; the
shipped images for the v0.4.2 stream-reliability work therefore land
under v0.4.3 instead, on top of the simplified amd64-only pipeline.

### Changed

- **Release builds are now amd64-only.** arm64 + armv7 platforms removed from both image (`docker buildx`) and binary (`make`) targets. Operator fleet is 100% x86 (Proxmox + VPS); QEMU emulation was costing ~40 min per tag (arm64 ~15 min, armv7 ~25 min) for zero consumers. amd64-only release should land in 3-5 min. `Dockerfile.hub` follows: the agent cross-build inside the hub image (which feeds the `/install` one-liner) drops arm64 + armv7 too, shaving ~30 MB from the hub image. Re-adding ARM later is two-file change documented inline; for arm64, switch to `ubuntu-24.04-arm` native runner to skip QEMU.

### Fixed

- **`SetReadDeadline` return value now checked.** golangci-lint `errcheck` flagged the two `conn.SetReadDeadline(...)` calls added in the v0.4.2 keepalive commit. Both now return on error (the only realistic cause is a conn that's already dead, in which case bailing is correct). Runtime behaviour is identical — only the CI lint status changes.

## [0.4.2] - 2026-05-31

Stream reliability patch: dashboards no longer drift into a false "stale" state after the browser tab idles for a while, and dead WebSocket clients on the hub no longer pin goroutines indefinitely. **Image build cancelled and re-shipped under v0.4.3** — git tag exists but no container image was pushed for v0.4.2; pull `v0.4.3` to get both the stream-reliability fixes and the new amd64-only pipeline.

### Fixed

- **Dashboard / HostDetail WebSocket now auto-reconnects.** Before, a bare `new WebSocket(...)` had no reconnect path; any transient close (browser throttle on background tab, NAT timeout, laptop sleep, server restart) froze the snapshots state while the `now` ticker kept advancing — every host drifted into "stale" within ~30s even though the hub was still healthy. Clicking into a host card "fixed" it only by remounting the component and creating a fresh WS, not by fixing the agent. New `useStreamConnection` hook centralises the WS lifecycle with exponential backoff (1s→2s→4s→8s→16s→30s) on close, plus a `visibilitychange` listener that force-reconnects the moment the tab regains focus (browser `setTimeout` throttling in background tabs can otherwise stretch reconnect attempts to 60s+). HostDetail re-sends its `subscribe` frame on each (re)connect via the hook's `onOpen` callback so the per-host filter survives the round-trip.

### Added

- **Server-side WebSocket keepalive on `/api/stream`.** Hub now pings clients every 30s and enforces a 60s read deadline (extended by every pong or control frame). Without keepalive, a vanished client (browser killed, laptop slept, NAT mapping reaped by CGNAT/proxy) left the goroutine pair pinned waiting on `ReadMessage`; one-way silence from the client direction also tripped NAT idle reapers at ~60s and silently killed otherwise-healthy connections. Browser auto-replies pong with zero FE code change.

## [0.4.1] - 2026-05-31

Phase 6 follow-up patch: alert history bounded by a real retention sweep, paginated scrollback in the Alerts UI, a discrete-fleet KPI rework on the dashboard, and a unified stale/offline threshold so notifications no longer fire before the UI marks the host yellow.

### Added

- **Retention sweep for alert history.** `alert_events` (`state='resolved'`) and `notification_deliveries` (`status IN ('sent','failed','dropped')`) older than the new `retention.delete_alerts_after` window (default 30 days; env override `LUMEN_HUB_RETENTION_ALERTS_WINDOW`; bounds 1h–365d) are pruned on the same heartbeat as the snapshot sweep. Firing events and pending/inflight deliveries always survive regardless of age. The window is exposed in **Settings → Retention** as "Alert history window" so it can be tuned without a hub restart.
- **"Load more" pagination for History + Deliveries.** Both tabs previously hardcoded a single 200-row page with no way to scroll back. Server limit cap raised from 500 → 2000 on `/api/alerts/events` and `/api/alerts/deliveries` (default still 100). The UI footer shows the row count and a "Load more" button that steps in 200-row pages up to the 2000 ceiling. Filter/state changes reset the page back to 200 so a "failed-only" switch doesn't suddenly show 1000 failed rows; auto-refresh keeps the current page size so the newest rows stay live without losing the scrollback. New i18n: `alerts.loadedCount` / `loadMore` / `loadMoreCeiling` (en + vi).

### Changed

- **Dashboard KPI bar: fleet averages replaced with hottest host per metric.** "Avg CPU" / "Avg RAM" were a borrowed cluster KPI that's misleading for a discrete fleet (homelab + VPSes) — an 85% hot host gets diluted by idle peers and the green card hides the only signal that matters. The bar now shows **Hottest CPU / Hottest RAM / Hottest Disk** with the worst host's name underneath each value, computed only over live (non-stale) snapshots so a dead agent's last reading doesn't leak into the headline number. Disk also gets a slot now, matching the per-host card. New i18n: `dashboard.hottestCpu` / `hottestRam` / `hottestDisk` / `noLiveHost`; removed `dashboard.avgCpu` / `avgRam` / `fleetAverage`.

### Fixed

- **Offline alert threshold now derives from `agent_interval` instead of hardcoded 60s.** Pre-fix, with `agent_interval ≥ 60s` the alert fired BEFORE the dashboard marked the host stale (the UI scaled to `max(2*interval, 30s)`, alerts didn't) — operators got a push and then loaded a still-green dashboard. The engine now refreshes `offlineAfter = max(2 * max(2*interval, 30s), 60s)` each `runOnce` from the `agent.interval` setting; UI yellow always precedes alert red regardless of how the interval is tuned. Default `agent_interval=5s` keeps the same 60s offline threshold so existing rule timing is unchanged.

## [0.4.0] - 2026-05-29

Phase 6 release: threshold-based alerting end-to-end. Operator-defined rules over any host metric, with state-machine evaluation, persisted history, a delivery queue with severity-aware retry, four notification channel types, per-channel severity floors, per-rule routing, host name/glob/tag selectors, and a first-class tag inventory shared by hosts and rules.

### Added

- Phase 6 / RFC 0001 Milestone A — threshold alerting and notifications. Operator-defined rules (CPU/RAM/swap/disk/load1 thresholds + `offline` rule); per-(rule, host) state machine evaluated every ~15s (`LUMEN_HUB_ALERT_INTERVAL`, runtime-tunable via `alerts.eval_interval`); persisted `alert_events` history; new top-level **Alerts** tab with Active/History/Rules/Channels sub-tabs; **ntfy / Discord / webhook** channel dispatch with a synchronous **Send test** action.
- Phase 6 / RFC 0001 Milestone B — finer-grained routing and a new channel type. **Telegram channel** (Bot API, `bot_token` + `chat_id`, HTML message body, masked re-edit) is now a first-class option alongside ntfy/Discord/webhook. **Per-rule channel routing**: each rule picks the subset of channels it fans out to; leaving the picker empty preserves the Milestone-A broadcast-to-all behaviour. **Per-channel severity floor** (`min_severity` info/warning/critical) so a pager can ignore low-severity noise. **Host glob patterns** in rule `host` (`web-*`, `*-prod`, `db-[0-9]*`) via stdlib `path.Match`. New tables `alert_rule_channels` + `notification_channels.min_severity` (migration 0009).
- Phase 6 / RFC 0001 Milestone C — **host tags and label selectors**, then promoted to a first-class **tag inventory**. Alert rules gained a `host_selector` field (`tier=critical,env=prod`, AND semantics) that wins over the `host` name field when set, plus the rule `host` field now accepts a **comma list** (`web-1,db-2,api-3`) so the UI can offer a multi-select agent picker. Tags then graduated from freeform `host_tags(host_id, key, value)` rows to a controlled inventory: a new **Alerts → Tags** tab where each tag is defined once (key + allowed values), hosts and rule selectors pick values from per-key dropdowns instead of free text, and deleting a tag/value cascades through `host_tags` and rewrites every affected rule selector (`Selector.DropKey`/`DropPair`) after a confirm dialog that shows the impact. Migration 0010 adds `host_tags` + `alert_rules.host_selector`; migration 0012 adds `tags` + `tag_values` and backfills the inventory from any tags already in use. `hosts.SetTags` enforces the inventory at the storage layer (`ErrTagNotInInventory`).
- Phase 6 / RFC 0001 Milestone D — **persisted notification delivery queue with severity-aware retry**. Engine is now non-blocking: each (alert × channel) lands as a `pending` row in `notification_deliveries`; a background worker pool (default 4 goroutines, 1 s poll) drains them with per-channel serialisation so a single Discord webhook can't back-pressure the others. Bursts of 100+ alerts no longer block the engine ticker. **Severity-aware retry policy**: critical alerts retry fast and give up in ~5 minutes (5 s, 15 s, 1 min, 5 min — a 6-hour retry on paging-grade alerts is useless); warning/info back off longer (30 s → 4 h, 6 attempts). The Alerts tab gains a **Deliveries** sub-tab with per-status filter, severity filter, queued/sent timestamps, retry-now button for failed/dropped rows, and a summary chip strip showing queue depth at a glance. Migration 0011 adds the `notification_deliveries` table.

### Fixed

- Offline rules no longer double-clamp `for_seconds`. The engine previously required `age ≥ 60s` to report breach **and** then forced `for_seconds` up to 60s, so even `for_seconds=0` took ~120s to fire. The clamp on `for_seconds` is gone; the 60s silence detection in `evaluateOne` is the only "ignore blips" floor. `for_seconds=0` now fires on the first tick past the 60s silence window; `for_seconds>0` still adds extra hold on top.

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
