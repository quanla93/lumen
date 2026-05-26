# Lumen — Master Action Plan

> **Đây là single source of truth cho dự án Lumen.**
> Mọi session Claude/dev mới mở ra → đọc file này đầu tiên để biết:
> - Dự án là gì, làm gì, không làm gì
> - Đã quyết định gì (không bàn lại)
> - Đang làm đến bước nào
> - Bước tiếp theo cụ thể là gì
>
> **Quy tắc**:
> - ✅ = done · 🚧 = in progress · ⏳ = next · ⏸️ = blocked · ❌ = scrapped
> - Mỗi khi hoàn thành 1 step, tick checkbox và commit kèm.
> - Mọi quyết định lớn → thêm vào mục [Decisions log](#decisions-log) (append-only).
> - Đổi scope/hủy step → đánh dấu ❌ + ghi lý do, không xóa.

---

## 📌 Project identity (locked)

| | |
|---|---|
| **Name** | Lumen |
| **Tagline** | Proxmox-native monitoring for homelabs. HTTPS-only, HDD-friendly, mobile-ready. |
| **Repo (proposed)** | github.com/lumenhq/lumen (eventual); currently staged at github.com/quanla93/lumen (private) |
| **Domain (proposed)** | lumenhq.dev (alt: getlumen.io) |
| **License** | MIT |
| **Language** | Go (hub + agent), TypeScript (web + docs) |
| **Audience** | Homelab users, small infra teams, Proxmox/Docker/LXC operators |
| **Scale target** | 1-200 hosts per hub |
| **NOT a target** | Kubernetes, microservices, enterprise observability, 1k+ hosts |

---

## 🎯 The three wedges (north star)

Mọi feature phải support ít nhất 1 trong 3 wedge. Nếu không → reject/defer.

1. **Proxmox/LXC first-class** — đọc Proxmox API trực tiếp, hiển thị cluster/ZFS/PBS/LXC vs QEMU.
2. **HDD-friendly storage** — batched writes, WAL tuning, hot/cold tiering với Parquet.
3. **HTTPS-only push transport** — agent push outbound, qua được NAT/CF Tunnel/Tailscale.

Phụ trợ: mobile PWA + Web Push, public status page, bilingual docs.

---

## 🚫 Anti-features (locked — không bàn lại)

- ❌ Không build dashboard builder (Grafana đã có)
- ❌ Không log aggregation full-text (Loki đã có) — chỉ có tail viewer minimal
- ❌ Không AI anomaly detection
- ❌ Không SAML/SSO/RBAC phức tạp
- ❌ Không cluster mode cho hub
- ❌ Không retention >365 ngày
- ❌ Không telemetry phone-home

---

## 📋 Decisions log (append-only)

Mỗi quyết định ghi 1 dòng. Không xóa, không sửa — nếu đổi ý → append decision mới override.

| Date | Decision | Why |
|---|---|---|
| 2026-05-25 | Project name = Lumen | User chose |
| 2026-05-25 | License = MIT | Đơn giản, friendly cho OSS adoption |
| 2026-05-25 | Hub language = Go | Single binary, low RAM, vs Java/Spring quá nặng |
| 2026-05-25 | Agent language = Go (cùng repo, share types) | Cross-compile dễ, dev velocity cao hơn Rust |
| 2026-05-25 | Storage = SQLite (hot) + Parquet (cold) | HDD-friendly; MongoDB/TimescaleDB quá nặng |
| 2026-05-25 | Transport = HTTPS/WebSocket push | NAT-friendly, khác biệt với Beszel (SSH) |
| 2026-05-25 | Web stack = React 18 + Vite + TS + Tailwind + shadcn/ui + uPlot + zustand | Modern, nhẹ; uPlot là chart lib nhỏ nhất cho time-series |
| 2026-05-25 | Docs framework = Starlight (Astro) | Static, i18n built-in, deploy Cloudflare Pages free |
| 2026-05-25 | Commits = Conventional Commits | Auto changelog, semantic versioning |
| 2026-05-25 | Contributor model = gradual-trust tiers (Visitor → Triager → Contributor → Trusted → Maintainer → Core) | Bám model của Astro/Tauri/Hono |
| 2026-05-25 | Go toolchain floor = 1.24 (was 1.22) | gopsutil/v4 v4.26.4 requires Go 1.24 — accept newer floor instead of downgrading to gopsutil/v3 (deprecated +incompatible tag). Updated CI workflows + CONTRIBUTING. |
| 2026-05-25 | Go toolchain floor = 1.25 (was 1.24) | pressly/goose v3 (added in Phase 2 storage) requires Go 1.25. Smaller bump than downgrading goose to an older line. CI + Dockerfiles + CONTRIBUTING synced. |
| 2026-05-25 | HTTP router = github.com/go-chi/chi/v5 | Stdlib-friendly, middleware ecosystem, no codegen — matches "single binary, low RAM" decision. |
| 2026-05-25 | Metrics lib = github.com/shirou/gopsutil/v4 | De-facto cross-platform metrics for Go; covers Linux/Windows/macOS in one API. |
| 2026-05-25 | Repo staging = github.com/quanla93/lumen (PRIVATE) | Personal staging until `lumenhq` GitHub org registered; switch to public + transfer when v0.1.0 ready. Git author = quanla93 / quanla.work@gmail.com. |
| 2026-05-25 | Config = env-vars only (LUMEN_*) + .env loader via godotenv | 12-factor: hub/agent are Dockerized services; flags become per-bespoke-binary noise. .env.example documents knobs. CLI flags removed. |
| 2026-05-25 | WebSocket lib = github.com/gorilla/websocket | Most widely understood, large body of examples, stable v1.5+. coder/websocket is more modern but adds a learning surface for contributors with no spike-time benefit. Easy to swap behind the stream.Handler if needed. |
| 2026-05-26 | Chart lib = uPlot (vanilla, ~40KB gzipped) | Smallest fast time-series lib for the host detail page. Drives canvas directly; matches the "modern, nhẹ" decision and avoids React-charts pulling in d3. Wrapped in a thin `<UPlotChart/>` so swapping later (recharts, echarts) only touches one file. |
| 2026-05-26 | History downsampling = SQLite-side AVG by bucket | `strftime('%s', ts) / step * step AS bucket; AVG(...) GROUP BY bucket` runs on the existing `idx_snapshots_host_ts` index. Capped at 2000 points/response and 7-day window; auto-step picks ~120 buckets. Phase 4 swaps this for the Parquet cold tier without changing the wire contract. |
| 2026-05-26 | Retention = simple time-based DELETE | One goroutine `retention.Run` sweeps every `LUMEN_HUB_RETENTION_INTERVAL` (default 1h) and DELETEs rows older than `LUMEN_HUB_RETENTION_WINDOW` (default 24h). Both env vars accept `0` to disable. Phase 4 replaces this with downsample-and-archive to Parquet. |
| 2026-05-26 | Seed admin = env-bootstrap, idempotent | `LUMEN_HUB_ADMIN_{USERNAME,PASSWORD}` create the admin on first boot (Argon2id). Existing user left alone — UI password changes survive restart. Both empty disables the seed (register-via-UI still works). |
| 2026-05-26 | Strict bearer-token ingest | `/api/ingest` rejects 401 without `Authorization: Bearer <token>`. Closes the pre-v0.1 anonymous spike hole; token's host name (server-side lookup) overrides body.host so a leaked token can't spoof a different host. |
| 2026-05-26 | Per-core CPU = live only, not persisted | Variable-cardinality per host. Stored only in the in-memory snapshot; flows through WS to the host detail page's per-core strip. Aggregate `cpu_pct` is what historical buckets average. Avoids a JSON column or a join table for modest pre-v1 value. |
| 2026-05-26 | Docker collector = stdlib HTTP-over-unix-socket, no docker/docker SDK | The official Go SDK pulls ~200 transitive deps + adds 30+ MB to the agent binary. We only need `/containers/json` + `/containers/{id}/stats?stream=false`. A ~150-line HTTP client over `net.Dialer{Unix}` covers both. Trade-off: we have to track Engine API field shapes manually (`cpu_stats` / `precpu_stats` deltas, `memory_stats.stats.inactive_file` subtraction) instead of inheriting them. Acceptable — the wire format is stable since API v1.21 (2015). |
| 2026-05-26 | Containers = live only, not persisted | Same rationale as per-core CPU. Cardinality varies per host and over time; persisting requires a join table or JSON column. Live-only fits the homelab "what's running RIGHT NOW" question; historical container metrics land later if user demand surfaces. |
| 2026-05-26 | Hub install = self-contained tarball + idempotent install-hub.sh | Pre-v0.1 the repo is private staging, so no GitHub Releases. Operator runs `make release-hub-tarballs` on a build box → `lumen-hub-linux-<arch>.tar.gz` (5 MB) holds binary + install.sh + systemd unit + env template + README. install.sh creates `lumen` system user, /etc/lumen (640 root:lumen), /var/lib/lumen (lumen:lumen), generates LUMEN_HUB_SECRET, drops the systemd unit, enables it. Re-runnable for in-place upgrades. `--purge` for clean wipe. Verified end-to-end in fresh Debian:12 container. |
| 2026-05-26 | Settings = key/value table; env seeds defaults on first read | New `settings` table (migration 0005) holds runtime-mutable knobs. Env vars (LUMEN_HUB_RETENTION_*) seed rows on first boot; once a row exists the UI value wins (env becomes inert). Pattern extends to future user-prefs without schema churn. |
| 2026-05-26 | Retention loop = heartbeat-driven, not ticker-locked | Original design used `time.NewTicker(currentInterval)` — meant a UI change to interval only applied AFTER the old interval elapsed (up to 1h). Refactored to a 30s heartbeat that reads settings each tick + tracks `lastSweep`; sweep fires when `time.Since(lastSweep) >= interval`. UI changes apply within ≤30s instead of ≤1h. Cost: one SELECT per 30s — negligible. |
| 2026-05-26 | Agent offline buffer = bbolt (single-file embedded KV) | Need durable FIFO across agent restart + hub outage; SQLite would pull migrations + a goroutine for one bucket of bytes. bbolt: single ~150-line wrapper, no schema, mmap-backed, file lock prevents two agents stomping the same path. Key = `[ts-nano BE][seq BE]` so range scans drain in capture order; value = JSON envelope. Default cap 24h × 5s ticks ≈ 17k rows. Compose mounts `lumen-agent-data:/data`; systemd installer carves `ReadWritePaths=/var/lib/lumen-agent`. |
| 2026-05-26 | Hub ingest persistence = batch flush ring (60s) | Per-ingest INSERT = one fsync per ingest under WAL+synchronous=NORMAL. At 200 hosts × 5s = 40 fsyncs/s on the homelab HDD. Switched ingest to enqueue into an in-memory channel (queue 10k); a single goroutine flushes every 60s (or 5000 rows, whichever first) in one transaction with a prepared INSERT. Worst-case loss on hub crash = one flush interval — agent's bbolt buffer replays it on reconnect, so the failure mode is "delayed write" not "data lost." Hot path (in-memory store + WS broadcast) unchanged. |
| 2026-05-26 | WS stream = optional subscribe filter, default firehose | Phase 1 broadcasts every host to every WS client. HostDetail only cares about one host; on a 200-host fleet that's 99.5% wasted bandwidth. Added `{"type":"subscribe","hosts":["..."]}` control frame; the special host `*` reverts to firehose. Empty or no message = firehose (old web builds keep working). Mutex-protected `allowed` map filters in `writeSnapshot`. Per-conn state, no shared registry. |

---

## 📐 Conventions (locked)

### File/folder
- Mọi path dùng forward slash kể cả khi viết Windows.
- Go: `cmd/<binary>/main.go`, code chính trong `internal/`, shared trong `internal/shared/`.
- Web: `web/src/{pages,components,lib,stores}/`.
- Docs: `docs/src/content/docs/<section>/<page>.md`.

### Code
- Go: `gofmt`, `golangci-lint`, errors as values, no `init()` magic.
- TS: Biome (formatter + linter), React function components + hooks.
- No new dependency without justification in PR description.

### Commits
- Conventional Commits: `feat|fix|docs|chore|refactor|test|perf|build|ci(scope): subject`
- Squash merge mặc định.

### Branches
- `feat/...`, `fix/...`, `docs/...`, `chore/...`
- Base = `main`. Không có `develop` branch.

### Versioning
- SemVer. Pre-1.0 = breaking changes được phép trong minor.
- Release tag: `v0.1.0`, `v0.1.1`, etc.

### Testing
- Mỗi PR thay đổi logic phải có test.
- CI gates: lint + test + build all platforms + RAM/size benchmark.

---

## 🗺️ Phase plan

### Phase 0 — Bootstrap project (Week 0) ✅ (substantial part done)

**Goal**: Tất cả file governance, docs scaffold, CI skeleton, repo public-ready.

- [x] Decide name = Lumen
- [x] README.md (root)
- [x] LICENSE (MIT)
- [x] CONTRIBUTING.md
- [ ] ⏸️ CODE_OF_CONDUCT.md (deferred per user)
- [ ] ⏸️ GOVERNANCE.md (deferred per user)
- [ ] ⏸️ SECURITY.md (deferred per user)
- [ ] ⏸️ SUPPORT.md (deferred per user)
- [ ] CHANGELOG.md (Keep-a-Changelog format) — deferred until first `v0.0.x` tag
- [x] `.github/ISSUE_TEMPLATE/bug_report.yml`
- [x] `.github/ISSUE_TEMPLATE/feature_request.yml`
- [x] `.github/ISSUE_TEMPLATE/collector_proposal.yml`
- [x] `.github/ISSUE_TEMPLATE/config.yml`
- [x] `.github/PULL_REQUEST_TEMPLATE.md`
- [x] `.github/DISCUSSION_TEMPLATE/*.yml` (q-and-a, ideas, show-and-tell)
- [x] `.github/CODEOWNERS`
- [x] `.github/workflows/ci.yml` (lint + test + build)
- [x] `.github/workflows/release.yml` (goreleaser config)
- [x] `.github/workflows/codeql.yml`
- [x] `.gitignore` (Go + Node)
- [x] `.editorconfig`
- [x] `Makefile` (dev-hub, dev-agent, dev-docs, test, lint, build, build-all, benchmark)
- [x] `docs/` Starlight scaffold (package.json, astro.config.mjs, content config)
- [x] First doc: `getting-started/quickstart.md`
- [x] First doc: `getting-started/overview.md`
- [x] `getting-started/concepts.md`
- [x] `docs/src/content/docs/index.mdx` landing
- [x] ADR-0001: storage architecture (SQLite + Parquet)
- [ ] ADR-0002: transport choice (HTTPS/WS over SSH)
- [ ] ADR-0003: language choice (Go for both hub + agent)
- [x] Memory saved (`~/.claude/.../memory/`) for cross-session continuity
- [ ] Logo + brand assets (in progress)

**Definition of done**: Có thể `git init && git add . && git commit` ra 1 repo trông "professional OSS-ready" mà chưa cần dòng code thật.

---

### Phase 1 — MVP technical spike (Week 1) ✅

**Goal**: 1 end-to-end skinny slice. Validate stack hoạt động trước khi build wide.

- [x] `go.mod` init + dependency chọn (chi, gopsutil, gorilla/websocket, godotenv locked; modernc.org/sqlite deferred to Phase 2 storage)
- [x] Skeleton dirs: `cmd/{lumen-hub,lumen-agent}/`, `internal/{hub,agent,shared}/`, `web/`
- [x] `cmd/lumen-hub/main.go` — bind `:8090`, serve `/healthz`
- [x] `cmd/lumen-agent/main.go` — read CPU% via gopsutil, POST to hub mỗi 5s
- [x] Hub `internal/hub/ingest/` — accept POST, in-memory store last value per host
- [x] Hub `internal/hub/stream/` — WS `/api/stream` broadcast every `LUMEN_HUB_STREAM_INTERVAL`
- [x] `web/` Vite scaffold + 1 page hiển thị live CPU% qua WS (inline styles; Tailwind+shadcn deferred to Phase 2)
- [x] `embed.FS` web build vào hub binary (SPA fallback; `.gitkeep` placeholder so `go build` never fails)
- [x] Docker compose chạy được hub + 1 agent local (distroless, env-only)
- [x] Document: `how-to/run-from-source.md` (4 run modes + Docker + API testing + env reference)
- [x] Bonus: OpenAPI 3.1 + `.http` file under `api/` for Postman/REST Client import

**Definition of done**: 1 binary chạy, mở browser, thấy CPU% live update từ agent. ✅ Achieved (`go run ./cmd/lumen-hub` + `go run ./cmd/lumen-agent` + Vite dev → http://localhost:5173; or `docker compose up` → http://localhost:8090).

---

### Phase 2 — MVP feature breadth (Week 2-4) 🚧 (core loop shipping)

**Goal**: Đủ feature để 1 user homelab thật dùng được.

#### Hub
- [x] Auth: register first-admin flow, JWT (HS256, 30d), password Argon2id (RFC 9106 second-class)
- [x] Hosts CRUD + token generation (lum_… one-shot; SHA-256 hash stored; rotate; delete; ingest validates and overwrites body.host)
- [x] SQLite schema migration framework (pressly/goose v3, embedded migrations, WAL pragmas)
- [x] Per-host CPU ring buffer in-memory (120 samples, ships on WS)
- [x] Batch flush ring → SQLite mỗi 60s (coalesce N INSERTs into one tx; flush_size=5000; final flush on shutdown). HDD-friendly wedge — cuts fsync pressure ~100× at fleet scale.
- [x] Query API: `GET /api/hosts/:id/metrics?from&to&step` (SQLite AVG buckets, auto-step, capped at 2000 points / 7d window)
- [x] WS subscribe/unsubscribe protocol — client → server `{"type":"subscribe","hosts":["a","b"]}`; `["*"]` reverts to firehose; empty/no-message keeps Phase 1 behavior. HostDetail subscribes to its single host on open.
- [x] Retention task (default 1h sweep, delete snapshots older than 24h; `LUMEN_HUB_RETENTION_{WINDOW,INTERVAL}`)
- [x] Settings page: retention (window/interval — UI changes apply within 30s via heartbeat), password change (current+new+confirm, Argon2id rehash)

#### Agent
- [x] Host collector: CPU%, RAM%, Swap%, Disk%, load1/5/15 (gopsutil v4)
- [x] Per-core CPU + disk I/O + network throughput + temperature (rate-from-cumulative state in `collector.Rates`; temperature picker prefers coretemp/k10temp)
- [x] Docker collector (Engine API, minimal stdlib unix-socket client — no docker/docker SDK). Lists running + stopped containers, computes per-container CPU% (delta) + memory used/limit. Live-only, not persisted. Warns once on macOS Docker Desktop when socket sharing is disabled.
- [x] Local bbolt buffer cho offline — `internal/agent/buffer`; default cap 24h × 5s ticks (~17k rows); replays gradually (10 frames per successful tick) so a backlog drains without thundering herd. Corruption-tolerant: bad file renamed `.corrupt-<unix>` and a fresh DB is opened.
- [ ] Config file YAML + env override (currently env-only via godotenv)
- [x] Systemd service file (`deploy/systemd/lumen-agent.service` — hardened nonroot-ish, runs as root for /proc + /sys + docker.sock)
- [x] Install script `<hub>/install.sh` (already shipped earlier — hub serves it w/ baked-in URL + binaries)

#### Web
- [x] Overview page: host cards + CPU sparkline + 3 metric bars (CPU/RAM/Disk) + load avg footer
- [x] Dark/light mode toggle (class-based, persists in localStorage)
- [x] Auth UI: Register / Login / Logout + AppShell with tab nav
- [x] Settings UI: hosts table + create + rotate + delete + one-shot token reveal + .env snippet
- [x] Host detail page: 6 uPlot charts (CPU%, RAM%, Disk%, load avg, Network rx/tx, Disk I/O r/w) + conditional Temperature chart + per-core CPU live strip (subscribed via WS) + range picker (1h/6h/24h) + auto-refresh every 30s + Containers table (name + state badge + image + CPU + mem usage/limit, sorted running-first, danger highlight at mem ≥ 90%).
- [ ] PWA manifest + service worker

#### Docs (parallel)
- [x] `install/hub-compose.md`
- [x] `install/hub-binary.md`
- [x] `install/hub-lxc.md` (Proxmox LXC walkthrough — both native + Docker-in-LXC shapes)
- [x] `install/agent-linux.md`
- [ ] `install/agent-docker.md`
- [x] `configure/hosts-and-tokens.md`
- [ ] `configure/retention.md`
- [ ] `reference/architecture.md`
- [ ] `reference/api.md`
- [ ] `reference/metrics-catalog.md`
- [ ] `faq.md`

**Definition of done**: Có thể tag `v0.1.0`, public Show HN / r/selfhosted post.

---

### Phase 3 — v0.2: Proxmox wedge (Week 5-8)

**Goal**: Ship signature feature — Proxmox-native.

- [ ] LXC collector trong agent
- [ ] Proxmox API client (agentless mode cho host)
- [ ] Proxmox host config UI: enter URL + API token
- [ ] Cluster topology view (nodes, quorum)
- [ ] ZFS pool stats (`zpool list`, `arcstat`)
- [ ] PBS backup status (read PBS API hoặc parse `vzdump` logs)
- [ ] LXC vs QEMU distinction trong UI
- [ ] Migration history view
- [ ] Alert engine v1: rules (threshold-based), evaluation loop
- [ ] Notification channels: Discord, Telegram, ntfy, Webhook, Email (SMTP)
- [ ] Docs: `integrations/proxmox.md` + `integrations/lxc.md` + `integrations/zfs.md` + `integrations/pbs.md`
- [ ] LXC helper script: `bash -c "$(curl ...)"` style installer

**Definition of done**: Có thể add 1 Proxmox node, thấy LXC list + ZFS pools + cluster status + nhận alert khi 1 LXC crash.

---

### Phase 4 — v0.3: Cold tier + retention (Week 9-12)

- [ ] Parquet writer (parquet-go)
- [ ] Compaction job: SQLite >24h → Parquet 5-min downsample
- [ ] Query layer transparent over SQLite + Parquet
- [ ] DuckDB optional embed for cold queries
- [ ] Retention config UI: 24h hot / 30d cold / 365d archive
- [ ] Multi-user (admin + read-only viewer)
- [ ] TOTP 2FA
- [ ] Docs: `how-to/reduce-disk-writes-further.md`
- [ ] Benchmark: ghi IOPS/ngày so với Beszel + Prometheus, post lên docs

---

### Phase 5 — v0.4+: Polish & community features

- [ ] Public status page (read-only share)
- [ ] Log tail viewer (journald + Docker logs, last 500 lines, no search)
- [ ] Web Push notifications (VAPID)
- [ ] i18n UI: Vietnamese + English
- [ ] Translation docs

---

### Phase 6 — v1.0: Stable

- [ ] API freeze + version (`/api/v1/...`)
- [ ] Plugin SDK (Go plugin or external binary + JSON protocol)
- [ ] Migration tool from Beszel
- [ ] Performance regression test suite
- [ ] Security audit (community-driven)

---

## 🔭 How to resume work in a new session

Nếu bạn (hoặc Claude) mở session mới:

1. **Đọc file này từ đầu** đến hết — đặc biệt mục Decisions log và Phase plan.
2. **Tìm 🚧 (in-progress) hoặc ⏳ (next)** trong Phase plan → đó là điểm bắt đầu.
3. **Đọc `MEMORY.md`** trong memory dir của Claude (nếu là Claude session).
4. **Kiểm tra git log** để xem PR/commit gần nhất:
   ```bash
   git log --oneline -20
   ```
5. **Không bàn lại** những thứ trong Decisions log + Anti-features — đã chốt.
6. **Trước khi đổi phương án lớn** → ghi decision mới vào Decisions log (append-only).

### Mỗi khi kết thúc session
- Tick checkbox đã done.
- Update mục "Current focus" bên dưới.
- Commit `ACTION_PLAN.md` cùng các thay đổi khác.

---

## 📍 Current focus

> Cập nhật mục này mỗi session.

**Session**: 2026-05-26
**Đang làm**: Phase 2 — "History & detail view" slice ✅. Next: agent breadth (net/IO/temp + Docker) or WS subscribe protocol.
**Vừa hoàn thành**:
- README, LICENSE, CONTRIBUTING, ACTION_PLAN
- Toàn bộ `.github/` (issue/PR/discussion templates + CI/release/CodeQL workflows + CODEOWNERS)
- `.gitignore`, `.editorconfig`, `Makefile`
- Starlight docs scaffold + landing + getting-started (overview, quickstart, concepts)
- ADR-0001 storage architecture
- Memory files cho cross-session continuity
- Logo + brand assets (SVG)
- Toolchain dev unblocked: Go 1.26.3 + pnpm cài qua winget/npm
- `git init -b main` + initial commit Phase 0 baseline (`919e0a7 chore: initial Phase 0 baseline`)

**Phase 1 complete (10 micro-steps + 1 bonus, 8 commits on main):**
- ✅ 1.1–1.9 per Phase plan above + OpenAPI/`.http` spec for tooling.

**Phase 2 first slice (✅ all shipped):**
1. ✅ UI polish foundation — Tailwind v4 + shadcn-style cards, theme toggle.
2. ✅ Agent collector breadth — RAM%, swap, disk usage%, load avg (Net/IO/temp still pending; tracked under "Agent" above).
3. ✅ Web Overview page — host cards, status dots, CPU sparkline, RAM/Disk bars, load avg footer.
4. ✅ Hub auth + Hosts CRUD — register-first-admin, Argon2id, JWT cookie, rotatable bearer tokens, ingest validation.
5. ✅ Hub storage layer — modernc.org/sqlite + goose migrations, per-host CPU ring, sync INSERT per ingest.

**Phase 2 second slice — "History & detail view" (2026-05-26):**
6. ✅ Query API `GET /api/hosts/{id}/metrics?from&to&step` — server-side AVG bucketing, auto-step, 2000-point cap, 7-day window cap.
7. ✅ Retention loop — `LUMEN_HUB_RETENTION_WINDOW`/`_INTERVAL` (defaults 24h / 1h); `0` disables.
8. ✅ Host detail page — uPlot charts for CPU/RAM/Disk + load (1/5/15), 1h/6h/24h range picker, 30s auto-refresh, click-through from dashboard cards.

**Blockers / open questions**:
- Domain `lumenhq.dev` / GitHub org `lumenhq` chưa register — không block code nhưng nên xử lý trước public release.
- Discord/community URL chưa có — placeholder trong README.
- ADR-0002 (transport) + ADR-0003 (language) còn nợ — viết trước cuối Phase 1 để khớp với code thực tế.
- CHANGELOG.md (Keep-a-Changelog) chưa khởi tạo — thêm khi cắt tag `v0.0.1` đầu tiên.

**Đã verify trên máy dev**:
- ✅ Node v22.22.0
- ✅ Docker 28.0.4
- ✅ Git 2.48.1
- ✅ Go 1.26.3 (winget GoLang.Go) — installed at `C:\Program Files\Go\bin`
- ✅ pnpm (npm global)
- ⚠️ Shell hiện tại có thể cần re-open hoặc chạy `$env:Path += ";C:\Program Files\Go\bin"` để pick up Go

---

## 📚 Reference docs trong repo

- `docs/src/content/docs/reference/architecture.md` — kiến trúc chi tiết
- `docs/adr/` — Architecture Decision Records
- `docs/rfcs/` — RFCs cho feature lớn
- `CHANGELOG.md` — release history
- `GOVERNANCE.md` — cách project ra quyết định
