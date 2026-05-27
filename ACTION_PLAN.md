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
| **Repo** | github.com/quanla93/lumen |
| **Domain** | quanla.org (temporary) |
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
- ❌ Không enterprise RBAC phức tạp; SSO self-hosted được phép trong scope giới hạn nhưng đang deferred: custom OIDC trước, SAML2 cân nhắc sau OIDC nếu complexity chấp nhận được
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
| 2026-05-26 | History downsampling = SQLite-side AVG by bucket | `strftime('%s', ts) / step * step AS bucket; AVG(...) GROUP BY bucket` runs on the existing `idx_snapshots_host_ts` index. Capped at 2000 points/response and 7-day window; auto-step picks ~120 buckets. Phase 5 swaps this for the Parquet cold tier without changing the wire contract. |
| 2026-05-26 | Retention = simple time-based DELETE | One goroutine `retention.Run` sweeps every `LUMEN_HUB_RETENTION_INTERVAL` (default 1h) and DELETEs rows older than `LUMEN_HUB_RETENTION_WINDOW` (default 24h). Both env vars accept `0` to disable. Phase 5 replaces this with downsample-and-archive to Parquet. |
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
| 2026-05-26 | Agent YAML config = sugar over env, not parallel system | YAML file (`/etc/lumen/agent.yaml`) is parsed once at boot and applied to the environment, only setting keys that aren't already in `os.LookupEnv`. The rest of the agent reads its config the same way it always has (envcfg). Reasoning: shipping one /etc/lumen/agent.yaml across a fleet via Ansible/Salt is operator-friendly, but maintaining two parallel config paths (env vs YAML structs) doubles the surface area for nil/zero-value bugs. Process env always wins; missing file is silent; malformed is fatal at boot. |
| 2026-05-26 | CI/CD = GitHub Actions, no GoReleaser | The repo already produces hub tarballs via `make release-hub-tarballs` (binary + install.sh + unit + env example bundled). GoReleaser would duplicate that logic; replaced its workflow with three jobs that call the existing Make targets, push multi-arch images to ghcr.io via Buildx+QEMU, and use `softprops/action-gh-release` to upload binaries. CI got a Docker-build smoke job so every PR exercises both Dockerfiles; lint-web pinned to actual scripts (`tsc --noEmit` for web, biome for docs with `dist/` ignored). |
| 2026-05-26 | PWA = minimal vanilla SW, no plugin | Goal is "installable to phone homescreen + paint instantly on cold start," not "full offline app" — live metrics fundamentally need network reachability to the hub. Skipped vite-plugin-pwa to avoid pulling Workbox + its build-time config surface; a 50-line `sw.js` covers cache-first for the shell + network-only for `/api/*` (caching metric snapshots would be misleading). Manifest + 192/512 SVG icons in `web/public/`, registered from `main.tsx` with `if ("serviceWorker" in navigator)` so non-supporting browsers no-op gracefully. |
| 2026-05-27 | MVP scope += operator customization; SSO deferred from immediate queue | User wants Lumen to prioritize configurable agent refresh/collection interval, configurable Parquet downsample policy, product-grade UI polish, and lightweight log management first. Self-hosted SSO remains important (custom OIDC first; SAML2 later if feasible) but moves to a later phase after the current four priorities land. Docker container monitoring remains roadmap/future unless pulled forward explicitly. |
| 2026-05-27 | DuckDB cold-query layer requires research before commitment | Querying old Parquet data through DuckDB sounds useful, but must be validated for practicality, operational footprint, packaging, memory usage, and whether it should be optional instead of default. Do not treat DuckDB as locked until a feasibility spike/ADR lands. |
| 2026-05-27 | External data access/export is an official expansion path | Lumen should not assume users only consume data through the built-in web UI. Future API/export surfaces should allow external dashboards such as Grafana to query or ingest monitoring data while Lumen can remain the preferred dashboard for users who want it. |
| 2026-05-27 | Log management = lightweight on-demand debugging, not Loki | Lumen should support quick incident debugging via hub/agent/systemd/journald/Docker log viewing, but must not become centralized log aggregation or full-text log analytics. Default behavior: admin-only, on-demand last-N/live-tail, no persistence/indexing unless a later RFC explicitly expands scope. |
| 2026-05-27 | UI polish is a product requirement, not cosmetic cleanup | Lumen must feel like a polished self-hosted monitoring product, not just a technical MVP. Use Beszel as a benchmark for completeness and UX quality without copying its visual identity. Prioritize dashboard, host detail, settings, onboarding, and reusable design components. |

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
- [x] CODE_OF_CONDUCT.md
- [x] GOVERNANCE.md
- [x] SECURITY.md
- [x] SUPPORT.md
- [x] CHANGELOG.md (Keep-a-Changelog format)
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
- [x] ADR-0002: transport choice (HTTPS/WS over SSH)
- [x] ADR-0003: language choice (Go for both hub + agent)
- [x] Memory saved (`~/.claude/.../memory/`) for cross-session continuity
- [x] Logo + brand assets

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

#### MVP scope guardrails
- [ ] Current MVP priority: configurable agent refresh/collection interval, configurable Parquet downsample policy, product-grade UI polish, and lightweight log management scope.
- [ ] Operator customization is first-class: log retention time, agent refresh/collection interval, and Parquet downsample policy must be configurable instead of hard-coded.
- [ ] Self-hosted SSO (custom OIDC first, SAML2 later if feasible) is important but moved out of the immediate MVP priority queue until the current four items land.
- [ ] Docker container monitoring is roadmap/future unless explicitly pulled into MVP; current live-only container visibility can remain as-is.

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
- [ ] Settings API: agent refresh/collection interval as a runtime-configurable knob, surfaced to agents without rebuilding/redeploying
- [ ] Settings API: Parquet downsample policy config (bucket size and hot/cold/archive windows) before Phase 5 cold-tier implementation locks format assumptions

#### Agent
- [x] Host collector: CPU%, RAM%, Swap%, Disk%, load1/5/15 (gopsutil v4)
- [x] Per-core CPU + disk I/O + network throughput + temperature (rate-from-cumulative state in `collector.Rates`; temperature picker prefers coretemp/k10temp)
- [x] Docker collector (Engine API, minimal stdlib unix-socket client — no docker/docker SDK). Lists running + stopped containers, computes per-container CPU% (delta) + memory used/limit. Live-only, not persisted. Warns once on macOS Docker Desktop when socket sharing is disabled.
- [x] Local bbolt buffer cho offline — `internal/agent/buffer`; default cap 24h × 5s ticks (~17k rows); replays gradually (10 frames per successful tick) so a backlog drains without thundering herd. Corruption-tolerant: bad file renamed `.corrupt-<unix>` and a fresh DB is opened.
- [x] Config file YAML + env override — `internal/agent/config`; YAML is a syntax convenience that folds into the env. Precedence: process env > YAML > .env > defaults. Default path `/etc/lumen/agent.yaml`; missing file is silent no-op, malformed is fatal at boot.
- [ ] Agent refresh/collection interval must be configurable by operator policy from hub/settings, while preserving env/YAML as bootstrap defaults.
- [x] Systemd service file (`deploy/systemd/lumen-agent.service` — hardened nonroot-ish, runs as root for /proc + /sys + docker.sock)
- [x] Install script `<hub>/install.sh` (already shipped earlier — hub serves it w/ baked-in URL + binaries)

#### Web
- [x] Overview page: host cards + CPU sparkline + 3 metric bars (CPU/RAM/Disk) + load avg footer
- [x] Dark/light mode toggle (class-based, persists in localStorage)
- [x] Auth UI: Register / Login / Logout + AppShell with tab nav
- [x] Settings UI: hosts table + create + rotate + delete + one-shot token reveal + .env snippet
- [ ] Settings UI: configure retention, agent refresh interval, and Parquet downsample/cold-tier policy from the web app
- [x] Host detail page: 6 uPlot charts (CPU%, RAM%, Disk%, load avg, Network rx/tx, Disk I/O r/w) + conditional Temperature chart + per-core CPU live strip (subscribed via WS) + range picker (1h/6h/24h) + auto-refresh every 30s + Containers table (name + state badge + image + CPU + mem usage/limit, sorted running-first, danger highlight at mem ≥ 90%).
- [x] PWA manifest + service worker — installable to homescreen on mobile; SW caches the app shell (cache-first) but never `/api/*` (network-only). Falls back gracefully on browsers without SW support.

#### Docs (parallel)
- [x] `install/hub-compose.md`
- [x] `install/hub-binary.md`
- [x] `install/hub-lxc.md` (Proxmox LXC walkthrough — both native + Docker-in-LXC shapes)
- [x] `install/agent-linux.md`
- [x] `install/agent-docker.md` — compose snippet + standalone `docker run` + macOS socket quirk + YAML config in container + fleet pattern.
- [x] `configure/hosts-and-tokens.md`
- [x] `configure/retention.md` — landed earlier with the retention heartbeat refactor.
- [x] `configure/reliability.md` (bonus) — agent buffer + hub batcher + WS subscribe protocol.
- [x] `contributing/ci-cd.md` (bonus) — reproduces CI locally + release flow + how to skip/promote.
- [x] `reference/architecture.md` — ASCII diagram + component breakdown + threat model.
- [x] `reference/api.md` — every REST endpoint + WS frame format + StreamControl + error shape.
- [x] `reference/metrics-catalog.md` — every metric, source, unit, persisted-vs-live, gotchas.
- [x] `faq.md`

**Definition of done**: Có thể tag `v0.1.0`, public Show HN / r/selfhosted post.

---

### Phase 3 — v0.2: Finish MVP priorities (Week 5-8)

**Goal**: Land the four current priorities before expanding the product surface: configurable agent refresh/collection interval, configurable Parquet downsample policy, product-grade UI polish, and lightweight log management scope.

#### Runtime + storage customization
- [ ] Agent refresh/collection interval: hub setting, API contract, agent polling/apply path, env/YAML bootstrap default, docs
- [ ] Parquet downsample policy: settings model for bucket size + hot/cold/archive windows, validation rules, UI controls, docs

#### Product-grade UI polish
- [ ] Design system pass: tokens, typography, spacing, cards, buttons, forms, tables, tabs, badges, empty/loading/error states
- [ ] App shell redesign: stronger navigation, account menu, responsive/mobile layout, clearer active states
- [ ] Dashboard redesign: summary strip, better host cards, status grouping, search/filter/sort, improved empty state with add-host CTA
- [ ] Host detail redesign: header/status block, metric overview cards, chart polish, range selector, loading states, container UX
- [ ] Settings redesign: structured sections for Hosts, Account, Runtime, Retention, Downsample policy, and future Log management

#### Lightweight log management
- [ ] Log management RFC: on-demand admin debugging only; no default persistence/indexing/full-text search
- [ ] Agent log source abstraction: journald/systemd unit logs, Docker container logs, and Lumen agent self logs
- [ ] Hub API/WS for on-demand log retrieval by host + source + target + line limit/time range
- [ ] Host detail Logs tab: source selector, target selector, limit presets, refresh, optional live tail, copy/download
- [ ] Docs: position Lumen log management as quick incident debugging, not a Loki replacement

**Definition of done**: Current four priorities are shipped or have accepted RFCs where implementation must follow a later phase.

---

### Phase 4 — v0.3: Proxmox wedge (Week 9-12)

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

### Phase 5 — v0.4: Cold tier + retention

- [ ] Parquet writer (parquet-go)
- [ ] Compaction job: SQLite > configurable hot window → Parquet using configurable downsample bucket/policy
- [ ] Query layer transparent over SQLite + Parquet
- [ ] DuckDB feasibility spike + ADR before implementation: validate practicality, packaging, memory footprint, query latency, and whether DuckDB should be optional/default/off by default
- [ ] Optional DuckDB cold-query layer only if spike confirms it is practical for homelab installs
- [ ] Multi-user (admin + read-only viewer)
- [ ] TOTP 2FA
- [ ] Docs: `how-to/reduce-disk-writes-further.md`
- [ ] Benchmark: ghi IOPS/ngày so với Beszel + Prometheus, post lên docs
- [ ] External data API/export RFC: define supported consumers (Grafana first), auth model, query shape, rate limits, and whether to expose Prometheus-compatible endpoints, Grafana datasource plugin, or plain REST/SQL-over-HTTP style API
- [ ] Grafana integration spike: prove a user can build Grafana dashboards from Lumen monitoring data without using Lumen's web UI

---

### Phase 6 — v0.5+: Polish & deferred product features

- [ ] Self-hosted SSO: custom OIDC provider config (issuer/client ID/client secret/scopes/redirect URL), with local admin fallback preserved
- [ ] SAML2 evaluation after OIDC; implement only if dependency and configuration complexity stay acceptable for homelab/self-hosted use
- [ ] Backup RFC/UX: local/S3-compatible backup, restore flow, encryption, retention, and whether backup belongs in core or optional module
- [ ] External data API/export RFC follow-up: Grafana first, auth model, query shape, rate limits, and Prometheus-compatible endpoint vs Grafana datasource plugin vs plain REST
- [ ] Grafana integration spike follow-up: prove a user can build Grafana dashboards from Lumen monitoring data without using Lumen's web UI
- [ ] First-run onboarding flow: create admin → add first host → copy agent command → wait for first metrics
- [ ] Public status page (read-only share)
- [ ] Web Push notifications (VAPID)
- [ ] i18n UI: Vietnamese + English
- [ ] Translation docs

---

### Phase 7 — v1.0: Stable

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

**Session**: 2026-05-27
**Đang làm**: Phase 2 closeout ✅. Next: Phase 3 — finish current MVP priorities (agent refresh interval, Parquet downsample policy, UI polish, lightweight log management).
**Vừa hoàn thành**:
- OSS readiness docs: CODE_OF_CONDUCT.md, GOVERNANCE.md, SECURITY.md, SUPPORT.md.
- CHANGELOG.md initialized with Keep-a-Changelog structure and Unreleased Phase 0-2 summary.
- ADR-0002 transport choice and ADR-0003 language choice accepted and linked from ADR index.
- ACTION_PLAN Phase 0 checklist synced with shipped brand assets and docs.
- Phase 2 checklist is fully green: hub, agent, web, docs, reliability, PWA, CI/CD.

**Phase 1 complete:**
- ✅ End-to-end skinny slice: hub, agent, ingest, live WS stream, embedded web, Docker Compose, source-run docs, OpenAPI/`.http` tooling.

**Phase 2 complete:**
1. ✅ Hub core: auth, hosts CRUD, strict bearer ingest, SQLite/goose, batched persistence, history query API, retention/settings, WS subscribe filters.
2. ✅ Agent core: host metrics, per-core CPU, net/disk rates, temperature, Docker collector, bbolt offline buffer, YAML config, systemd/install path.
3. ✅ Web core: overview, auth, settings, host detail charts, live per-core strip, containers table, dark/light mode, PWA shell.
4. ✅ Docs breadth: install, configure, reliability, CI/CD, architecture, API, metrics catalog, FAQ.

**Blockers / open questions before public release**:
- Domain `quanla.org` / GitHub org `lumenhq` chưa register — không block Phase 3 code nhưng nên xử lý trước public release.
- Discord/community URL chưa có — placeholder trong README.
- Current MVP priority queue is now limited to four items: configurable agent refresh/collection interval, configurable Parquet downsample policy, product-grade UI polish, and lightweight log management scope.
- Self-hosted SSO, DuckDB, external Grafana/API export, and backup are deferred to later phases/RFCs until the current four priorities land.
- Log management scope is intentionally lightweight: on-demand admin debugging via hub/agent/systemd/Docker logs, not Loki-style aggregation/search.
- UI polish is now tracked as a product requirement: benchmark Beszel for UX/completeness, redesign dashboard/host detail/settings without copying Beszel identity.

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
