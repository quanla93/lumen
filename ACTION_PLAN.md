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
> - Mỗi feature mới phải đi kèm docs/hướng dẫn sử dụng trong `docs/src/content/docs/` trước khi được xem là done.

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
| 2026-05-28 | Logs live viewer = separate surface, not Host Detail metrics ingest | Temporarily set log viewing aside from Host Detail. Future logs belong in a dedicated Logs/Console page using on-demand live streaming (WS/SSE style) only when opened/subscribed. Do not poll log-targets or ship Docker logs in periodic metrics ingest; keep it bounded, live-only by default, and globally/agent-disableable. |
| 2026-05-28 | Agent lifecycle = per-agent Docker Compose by default | For server management, the simplest supported lifecycle is a per-agent `docker-compose.yml` generated when the one-shot token is revealed. The file lives on the target VM/LXC, stores the existing token/config there, and future updates are `docker compose pull && docker compose up -d`. `docker run` remains a quick fallback, not the recommended long-running install path. |
| 2026-05-28 | Feature done means feature docs + usage guide done | Every new feature must include matching docs under `docs/src/content/docs/` before its checklist item can be considered complete. User-facing features need operator/user guidance, not just implementation notes. |
| 2026-05-28 | Install/onboarding docs are Docker-first | Keep the default operator path focused on Dockerfile/docker-compose.yml and Docker Compose for fast setup/update/debug. Native binary/systemd and `install.sh` can stay as optional/manual shortcuts, not the primary docs flow. |
| 2026-05-29 | Public API = consolidated module plan (see [Public API module](#-public-api--external-api--module-plan)) | External access was scattered across Phase 6/7/8 + the 2026-05-27 "external data access is official" decision. Pulled into one detailed spec so the team can build it coherently. Primary auth = **API keys with scopes** (not OAuth2/full RBAC — those stay optional/enterprise and remain flagged against the "no complex enterprise RBAC" anti-feature). "Tenant isolation" in a single-hub homelab context = scoping a key to a **host group** (subset of hosts), NOT true multi-tenancy / cluster. Rate limiting = in-memory token bucket (no Redis — respects single-binary/no-cluster). Public API gets a **richer response envelope** (`success`/`error.code`/`requestId`) while internal `/api/*` keeps the terse `{"error":"…"}` shape. |
| 2026-05-29 | Webhook = **một dispatch engine dùng chung** cho notification channels (Phase 5) + Public API customer webhooks | Đảo lại ý "tách riêng 2 hệ" cùng ngày: cả hai đều là "event → POST HTTP có HMAC/retry/log", khác nhau chỉ ở owner + host scope. Webhook là 1 channel type bên cạnh Discord/Telegram/ntfy/Email. `owner_type=admin` (UI, full scope) hoặc `owner_type=api_key` (Public API, host_scope ép = host group của key, enforce lúc match event). Dùng chung bảng `notification_channels` + `notification_deliveries`. Giảm trùng code, dễ audit, security boundary giữ nguyên ở bước match. |
| 2026-05-29 | Remote control (nếu làm sau) = **command-channel qua WebSocket sẵn có**, KHÔNG quay lại SSH | Transport push hiện tại (agent dial-out, HTTPS/WS) không khóa cửa control. Nếu tương lai cần điều khiển agent (restart service, update agent, chạy lệnh), đi qua chính WS mà agent đang giữ: hub đẩy lệnh **xuống** kênh đó. Giữ nguyên ưu thế zero-inbound + NAT/CGNAT-friendly (giống Tailscale / GitHub Actions runner / Cloudflare Tunnel). SSH cho control là "free + chín" nhưng đánh đổi cổng inbound + gãy NAT — đúng những thứ wedge HTTPS-only cố tránh. Chi phí WS-command: phải tự thiết kế tập lệnh + auth/scope từng lệnh + audit. Chỉ build khi scope "Management/control" được chốt là in-scope (xem open question). |

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
- [x] Settings API: agent refresh/collection interval as a runtime-configurable knob, surfaced to agents without rebuilding/redeploying
- [x] Settings API: Parquet downsample policy config (bucket size and hot/cold/archive windows) before Phase 5 cold-tier implementation locks format assumptions

#### Agent
- [x] Host collector: CPU%, RAM%, Swap%, Disk%, load1/5/15 (gopsutil v4)
- [x] Per-core CPU + disk I/O + network throughput + temperature (rate-from-cumulative state in `collector.Rates`; temperature picker prefers coretemp/k10temp)
- [x] Docker collector (Engine API, minimal stdlib unix-socket client — no docker/docker SDK). Lists running + stopped containers, computes per-container CPU% (delta) + memory used/limit. Live-only, not persisted. Warns once on macOS Docker Desktop when socket sharing is disabled.
- [x] Local bbolt buffer cho offline — `internal/agent/buffer`; default cap 24h × 5s ticks (~17k rows); replays gradually (10 frames per successful tick) so a backlog drains without thundering herd. Corruption-tolerant: bad file renamed `.corrupt-<unix>` and a fresh DB is opened.
- [x] Config file YAML + env override — `internal/agent/config`; YAML is a syntax convenience that folds into the env. Precedence: process env > YAML > .env > defaults. Default path `/etc/lumen/agent.yaml`; missing file is silent no-op, malformed is fatal at boot.
- [x] Agent refresh/collection interval must be configurable by operator policy from hub/settings, while preserving env/YAML as bootstrap defaults.
- [x] Systemd service file (`deploy/systemd/lumen-agent.service` — hardened nonroot-ish, runs as root for /proc + /sys + docker.sock)
- [x] Install script `<hub>/install.sh` (already shipped earlier — hub serves it w/ baked-in URL + binaries)

#### Web
- [x] Overview page: host cards + CPU sparkline + 3 metric bars (CPU/RAM/Disk) + load avg footer
- [x] Dark/light mode toggle (class-based, persists in localStorage)
- [x] Auth UI: Register / Login / Logout + AppShell with tab nav
- [x] Settings UI: hosts table + create + rotate + delete + one-shot token reveal + generated per-agent Docker Compose setup
- [x] Settings UI: configure retention, agent refresh interval, and Parquet downsample/cold-tier policy from the web app
- [x] Host detail page: 6 uPlot charts (CPU%, RAM%, Disk%, load avg, Network rx/tx, Disk I/O r/w) + conditional Temperature chart + per-core CPU live strip (subscribed via WS) + range picker (1h/6h/24h) + auto-refresh every 30s + Containers table (name + state badge + image + CPU + mem usage/limit, sorted running-first, danger highlight at mem ≥ 90%).
- [x] PWA manifest + service worker — installable to homescreen on mobile; SW caches the app shell (cache-first) but never `/api/*` (network-only). Falls back gracefully on browsers without SW support.

#### Docs (parallel)
- [x] `install/hub-compose.md`
- [x] `install/hub-binary.md`
- [x] `install/hub-lxc.md` (Proxmox LXC walkthrough — both native + Docker-in-LXC shapes)
- [x] `install/agent-linux.md`
- [x] `install/agent-docker.md` — per-agent Docker Compose as recommended path, update/restart/log commands, `docker run` fallback, macOS socket quirk.
- [x] `configure/hosts-and-tokens.md`
- [x] `configure/retention.md` — landed earlier with the retention heartbeat refactor.
- [x] `configure/reliability.md` (bonus) — agent buffer + hub batcher + WS subscribe protocol.
- [x] `contributing/ci-cd.md` (bonus) — reproduces CI locally + release flow + how to skip/promote.
- [x] `reference/architecture.md` — ASCII diagram + component breakdown + threat model.
- [x] `reference/api.md` — every REST endpoint + WS frame format + StreamControl + error shape.
- [x] `reference/metrics-catalog.md` — every metric, source, unit, persisted-vs-live, gotchas.
- [x] `how-to/use-the-web-ui.md` — guided tour for dashboard, host detail, settings, token management, and troubleshooting.
- [x] `faq.md`

**Definition of done**: Có thể tag `v0.1.0`, public Show HN / r/selfhosted post.

---

### Phase 3 — v0.2: Finish MVP priorities (Week 5-8)

**Goal**: Land the four current priorities before expanding the product surface: configurable agent refresh/collection interval, configurable Parquet downsample policy, product-grade UI polish, and lightweight log management scope.

#### Runtime + storage customization
- [x] Agent refresh/collection interval: hub setting, API contract, agent polling/apply path, env/YAML bootstrap default, docs
- [x] Parquet downsample policy: settings model for bucket size + hot/cold/archive windows, validation rules, UI controls, docs

#### Product-grade UI polish
- [x] Design system pass: shared surface, button, status pill, and empty-state primitives applied to dashboard/detail/settings hotspots
- [x] App shell redesign: logo is the explicit dashboard/home action; host detail title/meta no longer navigates accidentally
- [x] Dashboard redesign: summary strip, host cards, search/filter, and empty states use consistent product surfaces
- [x] Host detail redesign: header/back behavior, chart cards, loading/empty states, per-core strip, and container table use consistent surfaces
- [x] Settings redesign: structured sections for Hosts, Account, Runtime, Retention, Downsample policy, and future Log management

#### i18n foundation
- [x] Add lightweight UI i18n infrastructure before more copy lands: locale state, persistence, translation lookup, and language toggle
- [x] Ship English + Vietnamese strings for AppShell, Dashboard, Host detail, Settings, auth, empty/loading/error states
- [x] Keep docs i18n separate from app i18n; Starlight handles docs, the web app owns runtime UI translations

#### Lightweight log management
- [x] Log management product direction: separate Logs/Console surface, not Host Detail metrics ingest; on-demand live stream only when opened/subscribed
- [ ] Log management RFC: on-demand admin debugging only; no default persistence/indexing/full-text search
- [ ] Agent log source abstraction: hub logs, journald/systemd unit logs, Docker container logs, and Lumen agent self logs
- [ ] Hub API/WS/SSE for on-demand log retrieval by host + source + target + line limit/time range; no periodic log-target polling
- [ ] Dedicated Logs/Console page: source selector, target selector, limit presets, optional live tail, copy/download; bounded buffer/rate limits and auto-unsubscribe
- [ ] Docs: position Lumen log management as quick incident debugging, not a Loki replacement

**Definition of done**: Current four priorities are shipped or have accepted RFCs where implementation must follow a later phase.

---

### Phase 4 — v0.3: Docker Compose agent lifecycle UX (Week 9)

**Goal**: Make server-agent installs and upgrades simple by making per-agent Docker Compose the recommended path. The one-shot token reveal generates a complete `docker-compose.yml` that lives on the target VM/LXC. Future updates are standard Docker operations: `docker compose pull && docker compose up -d`.

#### Version awareness
- [ ] Agent includes build/version metadata in every ingest and host metadata update
- [ ] Hub exposes latest bundled agent image/version metadata from the running hub build
- [ ] Host list/detail UI shows current agent version vs latest available version
- [x] Docs explain that token rotation is unrelated to code updates; existing compose credentials stay valid during update

#### Compose-first onboarding
- [x] Token reveal shows a complete per-agent `docker-compose.yml`
- [x] Token reveal supports copying and downloading `docker-compose.yml`
- [x] Token reveal shows copy-ready commands: `mkdir -p /opt/lumen-agent && cd /opt/lumen-agent`, save the file, then `docker compose up -d`
- [x] Generated compose file uses stable container name, `restart: unless-stopped`, `user: "0:0"`, hub URL, host name, token, interval, and optional Docker socket mount
- [x] `docker run` remains available as a quick fallback, but docs and UI recommend Compose for long-running agents

#### Update path
- [x] Document update as: SSH into the VM/LXC that owns the agent compose file, then run `cd /opt/lumen-agent && docker compose pull && docker compose up -d`
- [ ] Host detail `Update agent` panel shows the Compose update command and explains it must be run on the machine where that compose file exists
- [x] No update flow creates or rotates host tokens unless the operator explicitly clicks rotate
- [x] No update flow requires editing the hub's `docker-compose.yml` or project `.env`

#### Product + safety
- [x] Docs cover initial install, update, restart, logs, and uninstall for the per-agent compose directory
- [x] Docs clarify that the token is shown once by the hub but then persists in the target host's `docker-compose.yml`
- [x] Keep any custom Docker updater prototype documented as experimental/advanced only, not the primary UX

**Definition of done**: A user can create a host token, download/copy a per-agent `docker-compose.yml`, start the agent with `docker compose up -d`, and later update it with `docker compose pull && docker compose up -d` without creating a new host/token or using Watchtower/custom updater tooling.

---

### Phase 5 — v0.4: Proxmox wedge (Week 10-13)

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

### Phase 6 — v0.5: Cold tier + retention

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

### Phase 7 — v0.6+: Polish & deferred product features

- [ ] Self-hosted SSO: custom OIDC provider config (issuer/client ID/client secret/scopes/redirect URL), with local admin fallback preserved
- [ ] SAML2 evaluation after OIDC; implement only if dependency and configuration complexity stay acceptable for homelab/self-hosted use
- [ ] Backup RFC/UX: local/S3-compatible backup, restore flow, encryption, retention, and whether backup belongs in core or optional module
- [ ] External data API/export RFC follow-up: Grafana first, auth model, query shape, rate limits, and Prometheus-compatible endpoint vs Grafana datasource plugin vs plain REST
- [ ] Grafana integration spike follow-up: prove a user can build Grafana dashboards from Lumen monitoring data without using Lumen's web UI
- [ ] First-run onboarding flow: create admin → add first host → use generated per-agent Docker Compose setup → wait for first metrics
- [ ] Public status page (read-only share)
- [ ] Web Push notifications (VAPID)
- [ ] i18n polish follow-up: expand translations to new modules after the Phase 3 foundation lands
- [ ] Translation docs and contribution guide for adding/changing UI copy

#### Public landing / marketing site
- [ ] Keep `brand/index.html` as a standalone coming-soon/landing page, separate from the authenticated hub dashboard
- [ ] Evolve landing page into Lumen's public home: product positioning, screenshots, install CTA, docs link, GitHub link, and roadmap highlights
- [ ] Decide deployment target for the static landing page (GitHub Pages / Cloudflare Pages / other) without coupling it to hub runtime

---

### Phase 8 — v1.0: Stable

- [ ] API freeze + version (`/api/v1/...`)
- [ ] Plugin SDK (Go plugin or external binary + JSON protocol)
- [ ] Migration tool from Beszel
- [ ] Performance regression test suite
- [ ] Security audit (community-driven)

---

## 🌐 Public API / External API — Module Plan

> **Mục tiêu module**: cho phép khách hàng / bên thứ ba lấy dữ liệu monitoring và nhận cảnh báo **không qua UI của Lumen** (tích hợp hệ thống riêng, Grafana, automation…).
> **Nguyên tắc nền (bám anti-features đã khóa)**:
> - Lumen là homelab tool (1–200 hosts, single-hub, no-cluster) → Public API phải **lean**, không biến thành observability platform enterprise.
> - **Không** build dashboard builder → Grafana = *expose data*, không *build dashboard*.
> - **Không** complex enterprise RBAC → primary auth là **API Key + scopes**; OAuth2/RBAC động là optional/enterprise, deferred + flagged.
> - **Không** log aggregation/full-text → Public API logs = on-demand bounded, KHÔNG bulk log export / KHÔNG là Loki.
> - "Tenant isolation" trong ngữ cảnh single-hub = **host group** (1 key chỉ thấy subset hosts), KHÔNG phải multi-tenancy thật.
> - Rate limit = **in-memory token bucket** (không Redis — giữ single-binary).

### Phụ thuộc (đọc trước khi estimate)

Public API chỉ là lớp **expose**, không tự sinh dữ liệu. Một số surface bị chặn bởi feature nguồn chưa build:

| Public surface | Nguồn dữ liệu | Trạng thái nguồn | Bị chặn bởi |
|---|---|---|---|
| hubs, agents, status, latest metrics | in-memory store + hosts table | ✅ có (Phase 2) | — |
| metrics query (≤7d, AVG bucket) | `/api/hosts/{id}/metrics` | ✅ có (Phase 2) | — |
| metrics range dài + p95/percentile | cold tier Parquet | ❌ chưa | Phase 6 (Parquet) |
| alerts / events read+write | alert engine | ❌ chưa | Phase 5 (alert engine v1) |
| logs query | on-demand log viewer | 🚧 dở (Phase 3) | Phase 3 log RFC; **bounded only** |
| webhooks (customer-managed) | dispatch engine notification channels (dùng chung) | ❌ chưa | Phase 5 (notification engine) + module này |
| Prometheus / Grafana | exporter trên dữ liệu sẵn có | ❌ chưa | module này |

→ Vì vậy roadmap module (PAPI Phase 1..5 bên dưới) **map lên** Phase 5/6/7/8 hiện có, không đánh số xung đột với Phase plan chính.

---

### 1. Public API Strategy

**Phân loại 4 nhóm API (chốt sớm — quyết định path & auth):**

| Nhóm | Base path | Auth | Envelope | Expose ra ngoài? |
|---|---|---|---|---|
| **Public API** (khách/bên thứ ba) | `/api/v1/*` | API Key (`lumk_…`) | giàu (`success`/`error.code`/`requestId`) | ✅ **Có** |
| **Internal/UI API** | `/api/*` (unversioned) | Session cookie (`lumen_session`) | terse (`{"error":"…"}`) | ❌ Không |
| **Agent API** | `/api/ingest`, `/api/agent/policy` | Bearer host token (`lum_…`) | terse | ❌ Không (chỉ agent) |
| **Admin API** (quản lý key/webhook/audit) | `/api/admin/*` (hoặc trong `/api/*`) | Session cookie (admin) | terse | ❌ Không (qua UI) |

**Quyết định namespace**: `/api/v1/*` được **dành riêng cho public contract** (versioned, stable, API-key). Internal UI endpoints giữ nguyên `/api/*` unversioned (đính chính lại note cũ trong `reference/api.md` từng nói "mọi endpoint move sang /api/v1"). Như vậy "breaking UI internal" không động vào contract công khai.

**Nguyên tắc public contract**: chỉ expose READ + customer-scoped WRITE (webhooks, alert ack). Không expose host CRUD, token mgmt, settings, user mgmt, hub config qua Public API — những thứ đó là Admin (UI/session) only.

---

### 2. API Versioning

- Path-based: `/api/v1/...`. Khi breaking → `/api/v2/...` song song.
- **Backward compatibility**: thêm field = non-breaking (clients phải tolerant unknown fields). Đổi/xóa field hoặc đổi semantics = breaking → version mới.
- **Deprecation policy**: khi `/api/v2` ra → `/api/v1` vào *deprecated* tối thiểu **6 tháng** (hoặc 2 minor releases), trả header `Deprecation: true` + `Sunset: <date>` + link changelog. Log cảnh báo cho key nào còn gọi v1.
- Version chỉ áp cho Public API; internal/agent có thể đổi tự do.

---

### 3. Authentication & Authorization

**Primary = API Key (OSS-friendly, dễ cho khách):**
- Format: `lumk_<base62-32-bytes>` (prefix `lumk_` phân biệt với agent `lum_`). Plaintext **hiện 1 lần**; DB chỉ lưu `key_prefix` (8 ký tự đầu để hiển thị/lookup) + `key_hash` (SHA-256, hoặc Argon2id nếu chấp nhận cost).
- Gửi qua header `Authorization: Bearer lumk_…`.
- Mỗi key gắn: **scopes**, **host group** (subset hosts được phép, null = tất cả), **IP allowlist** (optional), **expiry** (optional), **rate-limit tier**.

**Scope-based permission (enum khóa cứng, không phải RBAC động):**

| Scope | Cho phép |
|---|---|
| `hubs:read` | đọc thông tin hub |
| `agents:read` | đọc danh sách/chi tiết/status agent |
| `metrics:read` | đọc latest + query time-series |
| `logs:read` | đọc logs on-demand (bounded) |
| `events:read` | đọc events |
| `alerts:read` | đọc alerts |
| `alerts:write` | ack/resolve alert |
| `webhooks:read` / `webhooks:write` | quản lý webhook của chính khách |
| `export:read` | export dữ liệu (CSV/JSON/Parquet) |

**Authorization check order**: valid key → not revoked/expired → IP allowed → scope đủ → host nằm trong host group của key → rate limit còn quota.

**OAuth2 / RBAC động / SSO cho API**: ❌ **deferred + flagged** (đụng anti-feature "no complex enterprise RBAC"). Chỉ làm nếu có khách enterprise thật + ADR riêng. Khi làm thì là OAuth2 client-credentials grant phát hành short-lived JWT mang scopes — KHÔNG full authz server.

---

### 4. Endpoint List

Tất cả dưới `/api/v1/`, header `Authorization: Bearer lumk_…`, trả envelope giàu.

| Method | Endpoint | Scope | Mục đích | Rate limit (đề xuất) |
|---|---|---|---|---|
| GET | `/hubs` | `hubs:read` | thông tin hub (single-hub → trả 1 phần tử) | 60/min |
| GET | `/agents` | `agents:read` | list agents (paginated, filter, sort) | 120/min |
| GET | `/agents/{id}` | `agents:read` | chi tiết 1 agent | 120/min |
| GET | `/agents/{id}/status` | `agents:read` | online/offline + last_seen | 240/min |
| GET | `/metrics/latest` | `metrics:read` | snapshot mới nhất (1 hoặc nhiều agent) | 120/min |
| GET | `/metrics/query` | `metrics:read` | time-series (from/to/step/aggregation/groupBy) | 60/min |
| GET | `/logs/query` | `logs:read` | logs on-demand bounded (host+source+limit/time) | 30/min |
| GET | `/events` | `events:read` | list events (paginated, filter) | 120/min |
| GET | `/alerts` | `alerts:read` | list alerts (status/severity filter) | 120/min |
| POST | `/alerts/{id}/ack` | `alerts:write` | acknowledge alert | 60/min |
| POST | `/alerts/{id}/resolve` | `alerts:write` | resolve alert | 60/min |
| GET | `/webhooks` | `webhooks:read` | list webhook của khách | 60/min |
| POST | `/webhooks` | `webhooks:write` | tạo webhook | 20/min |
| PUT | `/webhooks/{id}` | `webhooks:write` | update webhook | 20/min |
| DELETE | `/webhooks/{id}` | `webhooks:write` | xóa webhook | 20/min |
| POST | `/webhooks/{id}/test` | `webhooks:write` | gửi test event | 10/min |
| GET | `/webhooks/{id}/deliveries` | `webhooks:read` | delivery log | 60/min |
| GET | `/export/metrics` | `export:read` | export bulk (CSV/JSON; async cho range lớn) | 6/min |
| GET | `/prometheus` | `metrics:read` | Prometheus text exposition (all agents) | 60/min |
| GET | `/prometheus/agents/{id}` | `metrics:read` | Prometheus text (1 agent) | 120/min |
| GET | `/openapi.json` | none | spec (public) | — |

**Lỗi chuẩn có thể xảy ra** mỗi endpoint: `401 UNAUTHENTICATED` (key thiếu/sai), `403 FORBIDDEN_SCOPE` / `403 HOST_OUT_OF_SCOPE` / `403 IP_NOT_ALLOWED`, `404 NOT_FOUND`, `422 VALIDATION_ERROR` (param sai), `429 RATE_LIMIT_EXCEEDED`, `500 INTERNAL`, `503 COLD_TIER_UNAVAILABLE` (khi query range cần Parquet chưa sẵn).

#### Chi tiết `GET /metrics/query` (mục 6 — time-series)

Params: `agentId` (bắt buộc, hoặc `agentIds` CSV), `metric` (cpu.usage, ram.usage, disk.usage, load1, net.rx, net.tx, diskio.read, diskio.write, temp), `from`/`to` (RFC3339 hoặc relative `now-1h`), `step` (vd `60s`), `aggregation` (`avg|min|max|sum|p95|last`), `groupBy` (optional: `agent`), `fill` (`null|previous|zero`).
Giới hạn (bám decision cũ): window ≤ 7d trên hot tier; > 7d cần cold tier (Phase 6) → nếu chưa có trả `503 COLD_TIER_UNAVAILABLE`. Max 2000 points/series. `p95` chỉ có sau khi cold tier hoặc on-the-fly compute landed.

```http
GET /api/v1/metrics/query?agentId=agent-01&metric=cpu.usage&from=now-1h&to=now&step=60s&aggregation=avg
```
```json
{
  "success": true,
  "data": {
    "metric": "cpu.usage",
    "unit": "percent",
    "step_seconds": 60,
    "aggregation": "avg",
    "series": [
      {
        "agentId": "agent-01",
        "labels": {"host": "webA"},
        "points": [
          {"ts": "2026-05-29T08:00:00Z", "value": 12.5},
          {"ts": "2026-05-29T08:01:00Z", "value": 13.1}
        ]
      }
    ]
  },
  "page": {"hasMore": false},
  "requestId": "req_01J...",
  "timestamp": "2026-05-29T08:30:00Z"
}
```

---

### 3.JSON Response Standard (mục 5)

**Success:**
```json
{
  "success": true,
  "data": { },
  "page": {"limit": 50, "cursor": "eyJ...", "hasMore": true},
  "requestId": "req_01J...",
  "timestamp": "2026-05-29T08:30:00Z"
}
```
**Error:**
```json
{
  "success": false,
  "error": {"code": "RATE_LIMIT_EXCEEDED", "message": "Too many requests", "details": {"retryAfter": 30}},
  "requestId": "req_01J...",
  "timestamp": "2026-05-29T08:30:00Z"
}
```
**Conventions:**
- **Pagination**: cursor-based (opaque `cursor` + `limit`, default 50/max 500). Offset chỉ cho dataset nhỏ.
- **Filtering**: query params whitelisted per endpoint (vd `?status=online&severity=critical`).
- **Sorting**: `?sort=field` / `?sort=-field` (dấu `-` = desc); chỉ field được whitelist.
- **Time range**: `from`/`to` RFC3339 **hoặc** relative (`now-1h`, `now-7d`). Server luôn lưu/trả **UTC**; client gửi `?tz=Asia/Ho_Chi_Minh` chỉ ảnh hưởng group-by-day/format hiển thị, không đổi giá trị.
- **traceId/requestId**: mọi response có `requestId`; cũng trả header `X-Request-Id`. Log nội bộ correlate theo requestId. Nếu client gửi `X-Request-Id` thì echo lại.
- **Rate-limit headers**: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, và `Retry-After` khi 429.
- **Error codes (enum khóa)**: `UNAUTHENTICATED`, `FORBIDDEN_SCOPE`, `HOST_OUT_OF_SCOPE`, `IP_NOT_ALLOWED`, `NOT_FOUND`, `VALIDATION_ERROR`, `RATE_LIMIT_EXCEEDED`, `COLD_TIER_UNAVAILABLE`, `INTERNAL`.

---

### 4. Security Model (mục 9)

- **API key hashing**: chỉ lưu hash (SHA-256; cân nhắc Argon2id), không bao giờ lưu plaintext. Hiển thị `key_prefix` + `••••`.
- **Key rotation**: tạo key mới → khách chuyển → revoke key cũ. Hỗ trợ nhiều key/khách để rotate không downtime. `expires_at` optional auto-expire.
- **IP allowlist**: optional per-key CIDR list; check ở middleware trước scope.
- **Rate limit**: in-memory token bucket per key + per IP fallback (anti-abuse với key chưa auth). Tier theo key. Trả 429 + `Retry-After`.
- **Audit log**: ghi key create/revoke/rotate, webhook CRUD, alert ack/resolve, auth_failed. Append-only table.
- **Request signing**: KHÔNG bắt buộc cho inbound (TLS + bearer là đủ cho homelab). **Bắt buộc cho outbound webhook** (HMAC-SHA256, xem mục Webhook).
- **Secret masking**: token/secret không bao giờ log; mask trong audit + error.
- **Tenant isolation**: enforce host-group filter ở **query layer** (mọi query Public API append `WHERE host IN (key's group)`), không chỉ ở handler.
- **Abuse prevention**: rate limit + cap response size + cap time-range + cap concurrent export jobs + auto-disable key sau N auth_failed liên tiếp (optional).
- **TLS-only**: Public API chỉ phục vụ qua HTTPS (bám wedge HTTPS-only). 

---

### 5. Webhook Model (mục 8) — *dùng chung delivery engine với notification channels (Phase 5)*

**Gộp 1 engine, không build 2 lần.** "Webhook" là **một channel type** bên cạnh Discord/Telegram/ntfy/Email của notification channels (Phase 5). Cùng dispatch core (event → match → format → deliver → retry → log), cùng bảng channels + delivery log, cùng HMAC signer. Phân biệt bằng:
- `owner_type=admin` → tạo qua UI, full host scope (operator routing alert của chính mình).
- `owner_type=api_key` → tạo qua Public API `/api/v1/webhooks`, `host_scope` **ép = host group của key** (khách chỉ nhận event của host trong nhóm mình; enforce ở bước match event, không chỉ ở handler).

Public API customer webhook là `type=webhook` + `owner_type=api_key`. CRUD + test + retry + signature + delivery log — tất cả qua engine chung.

**Outbound payload:**
```json
{
  "id": "evt_01J...",
  "eventType": "alert.triggered",
  "alertId": "alert_123",
  "severity": "critical",
  "agentId": "agent-01",
  "host": "webA",
  "message": "CPU usage is above 90%",
  "value": 93.4,
  "threshold": 90,
  "timestamp": "2026-05-29T08:30:00Z"
}
```
**Event types**: `alert.triggered`, `alert.resolved`, `agent.online`, `agent.offline`, `webhook.test`.
**Signature verification**: header `X-Lumen-Signature: t=<unix>,v1=<hex(HMAC-SHA256(secret, "<t>.<rawbody>"))>` + `X-Lumen-Event-Id`. Khách verify HMAC + reject nếu `|now-t| > 5m` (chống replay). Secret hiện 1 lần khi tạo webhook.
**Delivery + retry**: timeout 10s, coi 2xx là success. Retry exponential backoff (vd 0s,30s,2m,10m,1h) tối đa ~6 lần; sau đó mark `failed` + đếm vào metric. Persist delivery log (status, http_status, attempts, next_retry_at, error). Auto-disable webhook nếu fail liên tục quá ngưỡng.
**Test**: `POST /webhooks/{id}/test` gửi `webhook.test` ngay, trả kết quả delivery synchronous.

---

### 6. Grafana Integration Plan (mục 7 & 10) — *expose, không build dashboard*

- **P1 (ưu tiên)**: **Prometheus-compatible exposition** `GET /api/v1/prometheus` (và `/prometheus/agents/{id}`) trả text format → user cấu hình Prometheus scrape hoặc Grafana dùng Prometheus datasource. API key qua bearer trong scrape config.
- **P1 (song song)**: **Grafana JSON / Infinity datasource** — endpoint trả JSON đúng shape để Grafana Infinity plugin query trực tiếp `/metrics/query` (không cần viết plugin riêng). Doc kèm dashboard JSON mẫu.
- **Defer**: Grafana datasource plugin chuyên dụng (chỉ làm nếu nhu cầu thực).
- **Defer**: OpenTelemetry (OTLP) export — đánh giá sau, không phải P1.

**Prometheus text mẫu:**
```text
# HELP lumen_cpu_usage_percent CPU usage percent
# TYPE lumen_cpu_usage_percent gauge
lumen_cpu_usage_percent{agent="agent-01",host="webA"} 12.5
lumen_ram_usage_percent{agent="agent-01",host="webA"} 63.2
lumen_disk_usage_percent{agent="agent-01",host="webA"} 41.5
lumen_load1{agent="agent-01",host="webA"} 0.42
lumen_agent_up{agent="agent-01",host="webA"} 1
```

---

### 7. Database tables liên quan

Migrations mới (goose), pure-SQLite:

| Table | Cột chính |
|---|---|
| `api_keys` | id, name, key_prefix, key_hash, scopes (text/JSON CSV), host_group_id (nullable FK), ip_allowlist (text/CIDR CSV), rate_tier, created_by, created_at, last_used_at, expires_at, revoked_at |
| `host_groups` | id, name, created_at |
| `host_group_members` | host_group_id, host_id (PK kép) |
| `notification_channels` *(dùng chung với Phase 5)* | id, owner_type (admin/api_key), owner_id (api_key_id, null nếu admin), type (webhook/discord/telegram/ntfy/email), config (JSON: url/headers…), secret_hash (HMAC cho webhook), events (CSV), host_scope (null=all hoặc host_group_id; ép = group của key khi owner=api_key), active, description, created_at, disabled_reason, fail_count |
| `notification_deliveries` *(delivery log dùng chung)* | id, channel_id (FK), event_id, event_type, payload (JSON), status (pending/success/failed), http_status, attempts, next_retry_at, error, created_at, delivered_at |
| `api_audit_log` | id, actor_type (key/admin), actor_id, action, target, ip, user_agent, request_id, meta (JSON), ts |

Lưu ý dung lượng (HDD-friendly wedge): `public_webhook_deliveries` + `api_audit_log` cần **retention riêng** (vd 30d) để không phình DB; tái dùng pattern retention heartbeat sẵn có.

---

### 8. Roadmap theo phase (PAPI Phase 1..5 — map lên Phase plan chính)

#### PAPI Phase 1 — Basic Public Read API  *(map: sau Phase 5 alert engine, hoặc tách read-only sớm hơn)*
- **Chức năng**: API key infra + đọc agents/status/latest metrics/short-range metrics query.
- **Endpoint**: `/hubs`, `/agents`, `/agents/{id}`, `/agents/{id}/status`, `/metrics/latest`, `/metrics/query` (≤7d).
- **DB**: `api_keys`, `host_groups`, `host_group_members`, `api_audit_log`.
- **Security**: key hashing, scope check, host-group isolation, rate limit (in-memory), TLS-only, audit.
- **Testing**: unit (auth/scope/rate-limit middleware), integration (key→query→isolation), negative (401/403/429).
- **Rủi ro**: dữ liệu nhạy cảm leak nếu host-group enforce sai → enforce ở query layer + test isolation kỹ.
- **Effort**: ~M (2–3 tuần) — phần lớn là middleware + key mgmt UI.

#### PAPI Phase 2 — Time-series Query API hoàn chỉnh  *(map: Phase 6 cold tier)*
- **Chức năng**: aggregation đầy đủ (avg/min/max/sum/p95), groupBy, pagination, range dài qua cold tier.
- **Endpoint**: nâng cấp `/metrics/query`, thêm `/export/metrics` (async cho range lớn).
- **DB**: dựa Parquet cold tier (Phase 6); có thể thêm `export_jobs` nếu export async.
- **Security**: cap range/size/concurrent export.
- **Testing**: correctness của aggregation/p95 so hot vs cold; perf benchmark.
- **Rủi ro**: query Parquet tốn RAM (đụng DuckDB spike đang pending) → cap + có thể stream.
- **Effort**: ~L (3–4 tuần, phụ thuộc cold tier).

#### PAPI Phase 3 — Webhook & Export  *(map: Phase 5/6)*
- **Chức năng**: customer webhook CRUD/test/retry/signature/delivery log — **dùng chung dispatch engine với notification channels (Phase 5)**, chỉ thêm owner=api_key + scope; export CSV/JSON.
- **Endpoint**: `/webhooks*`, `/events`, `/alerts`, `/alerts/{id}/ack|resolve`, `/export/metrics`.
- **DB**: `notification_channels`, `notification_deliveries` (dùng chung Phase 5, + cột owner/scope + retention).
- **Phụ thuộc**: dispatch engine của notification channels (Phase 5) là nền — PAPI Phase 3 chỉ thêm surface api_key + scope lên engine đó.
- **Security**: HMAC signing, anti-replay, SSRF guard (block internal IP cho webhook URL), auto-disable.
- **Testing**: delivery retry/backoff, signature verify vector, SSRF block.
- **Rủi ro**: webhook → SSRF/abuse → validate URL + block private ranges + outbound rate cap.
- **Effort**: ~L (3–4 tuần) — phụ thuộc alert engine (Phase 5).

#### PAPI Phase 4 — Grafana / Prometheus Compatibility  *(map: Phase 6/7)*
- **Chức năng**: Prometheus exposition + JSON datasource doc + dashboard mẫu.
- **Endpoint**: `/prometheus`, `/prometheus/agents/{id}`.
- **DB**: không thêm (đọc từ store sẵn có).
- **Security**: scope `metrics:read`, host-group filter trong exposition.
- **Testing**: validate text format bằng promtool; e2e scrape thử.
- **Rủi ro**: cardinality (per-core/containers live-only) → chỉ expose scalar persisted.
- **Effort**: ~M (1–2 tuần).

#### PAPI Phase 5 — Enterprise (OPTIONAL, FLAGGED)  *(map: Phase 8 / chỉ khi có nhu cầu + ADR)*
- **Chức năng**: OAuth2 client-credentials, quota/SLA tiers, SDK, audit nâng cao.
- **⚠️ Cảnh báo**: RBAC động + OAuth2 đụng anti-feature "no complex enterprise RBAC" → chỉ làm sau ADR riêng. Không mặc định build.
- **Effort**: chưa estimate (deferred).

---

### 9. Backlog task chi tiết

**Backend (Go / chi):**
- [ ] Middleware `apikey.Authenticator`: parse bearer `lumk_`, lookup theo prefix, verify hash, load scopes/group/ip/expiry.
- [ ] Middleware `scope.Require("metrics:read")` + `hostgroup.Filter` (inject allowed host set vào ctx).
- [ ] Middleware `ratelimit.TokenBucket` (in-memory, per-key + per-IP), set rate headers.
- [ ] Envelope helpers `respondOK(data, page)` / `respondErr(code, msg, details)` + requestId middleware (`X-Request-Id`).
- [ ] Handlers `/api/v1/*` theo Endpoint List; bind vào router group riêng.
- [ ] Query layer: hàm metrics query nhận `allowedHosts` bắt buộc (enforce isolation tại DB).
- [ ] Notification dispatch engine (dùng chung Phase 5): event → match channels (type + host scope) → format → deliver (goroutine queue + backoff) → HMAC signer → delivery log + retention; SSRF guard cho webhook URL. Public API chỉ thêm CRUD owner=api_key + ép host scope.
- [ ] Prometheus exporter (text format) trên store sẵn có.
- [ ] Export: CSV/JSON streaming; (Phase 2) async job runner.
- [ ] Migrations goose cho 6 tables; retention sweep cho deliveries + audit.
- [ ] Audit logger (append-only).
- [ ] OpenAPI 3.1 spec cho `/api/v1/*` (mở rộng `api/openapi.yaml`).

**Frontend (React / web — Admin UI, KHÔNG phải Public API):**
- [ ] Settings → **API Keys**: list (prefix + last_used + scopes + group), create (chọn scopes/group/IP/expiry), reveal-once, revoke, rotate.
- [ ] Settings → **Host Groups**: tạo group, gán hosts.
- [ ] Settings → **Webhooks**: list/create/edit/delete/test + xem delivery log + secret reveal-once.
- [ ] **API Audit log** viewer (filter theo key/action/time).
- [ ] i18n EN + VI cho mọi copy mới (bám i18n foundation Phase 3).

**DevOps / Docs:**
- [ ] Doc `integrations/public-api.md`: quickstart, auth, scopes, curl + JS + Python examples.
- [ ] Doc `integrations/grafana.md`: Prometheus datasource + Infinity + dashboard JSON mẫu.
- [ ] Doc `integrations/webhooks.md`: payload, signature verify (code mẫu), retry.
- [ ] Publish OpenAPI + Postman collection + `.http` (mở rộng `api/lumen.http`).
- [ ] Observability cho chính API (mục 10): metrics `api_request_count`, `api_request_latency`, `api_error_rate`, `api_rate_limit_count`, `api_auth_failed_count`, `webhook_delivery_failed_count` → expose nội bộ (Prometheus `/metrics` của hub hoặc log).
- [ ] (Optional) Sandbox/demo hub read-only cho thử nghiệm.
- [ ] CHANGELOG + API changelog page.

---

### 10. Các quyết định kỹ thuật nên chốt TRƯỚC khi code

1. ✅ **Auth primary = API Key + scopes** (OAuth2/RBAC deferred+flagged). — *chốt 2026-05-29.*
2. ✅ **Webhook dùng chung 1 dispatch engine** với notification channels (Phase 5); webhook là 1 channel type, phân biệt bằng `owner_type` + `host_scope`. Public API customer webhook = `owner=api_key`, scope ép = host group. — *chốt 2026-05-29 (đảo lại quyết định "tách riêng" trước đó vì Public API cũng cần noti, gộp giảm trùng code + dễ audit).*
3. ✅ **Envelope giàu chỉ cho `/api/v1/*`**; internal `/api/*` giữ terse. — *chốt 2026-05-29.*
4. ✅ **`/api/v1/*` dành riêng cho public contract**; internal UI giữ `/api/*` unversioned (đính chính note cũ). — *chốt 2026-05-29.*
5. ✅ **Tenant isolation = host group**, enforce ở query layer. — *chốt 2026-05-29.*
6. ✅ **Rate limit in-memory token bucket** (no Redis). — *chốt 2026-05-29.*
7. ⏳ **Hashing**: SHA-256 (nhanh) vs Argon2id (chậm, an toàn hơn nếu DB leak) cho `key_hash` — cần chốt (đề xuất: SHA-256 vì key là high-entropy random, không phải password).
8. ⏳ **Pagination**: cursor opaque encode kiểu gì (base64 của `{ts,id}`?) — chốt format.
9. ⏳ **Export async**: có cần job runner + table `export_jobs` không, hay chỉ streaming sync với cap? — chốt khi vào PAPI Phase 2.
10. ⏳ **Prometheus auth**: scrape gửi API key qua bearer header có đủ không (Prometheus hỗ trợ `authorization` trong scrape_config) — confirm.
11. ⏳ **p95/percentile**: compute on-the-fly trên hot tier (tốn CPU) hay chỉ có sau cold tier — chốt khi vào PAPI Phase 2.
12. ⏳ **DuckDB dependency**: query cold tier cho range dài có chờ DuckDB spike (đang pending ADR) không — gắn với Phase 6.
13. ⏳ **Webhook ownership khi rotate/revoke key**: webhook `owner=api_key` nên giữ lại hay disable khi key bị rotate/revoke? (Đề xuất: tách owner thành "api consumer" ổn định thay vì gắn cứng key ephemeral, hoặc cho phép re-claim. Chốt khi vào PAPI Phase 3.)

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

**Session**: 2026-05-28
**Đang làm**: Phase 4 — Docker-first install/onboarding closeout. Tomorrow's system/UI update should align product flows with the docs: hub setup, host creation, token reveal, generated per-agent `docker-compose.yml`, and update panels should all lead with Docker/Compose. Native binary/systemd and `install.sh` remain optional/manual shortcuts.
**Vừa hoàn thành**:
- Docs website is now a branded Starlight site with a Lumen landing page and Web UI guide.
- Docs audit synced stale pages with implemented dashboard, Settings runtime policy, retention heartbeat, API shape, metrics metadata, and Compose-first agent lifecycle.
- Quickstart, hub install, Proxmox LXC, host token, and agent docs now use Docker Compose as the primary operator path.
- `configure/runtime-settings.md` added for hub-managed agent collection interval and `/api/agent/policy`.
- ACTION_PLAN now requires docs/hướng dẫn for every new feature before the feature is considered done.

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
- **⏳ Open question — scope "Management/control":** Tên sản phẩm mô tả ban đầu là "Monitoring & **Management** Platform" nhưng ACTION_PLAN đang khóa **monitoring-only** (anti-feature: không exec lệnh xuống agent). Cần chốt: remote control (restart service, update agent, run command) có nằm trong tầm nhìn Lumen không? Nếu **có** → ghi thành mục tiêu + thiết kế command-channel-over-WS sớm để transport hiện tại đỡ nó (xem Decisions log 2026-05-29). Nếu **chưa chắc** → giữ monitoring-only; transport push hiện tại **không cản** việc thêm control sau.
- **Public API / External API** giờ đã có spec chi tiết riêng — xem mục [Public API / External API — Module Plan](#-public-api--external-api--module-plan). 6 quyết định đã chốt (API-key primary, webhook **gộp chung 1 dispatch engine** với notification channels Phase 5, envelope giàu chỉ cho `/api/v1`, host-group isolation, in-memory rate limit, `/api/v1` = public contract); còn ~7 mục `⏳` cần chốt trước khi code (hashing, cursor format, export async, prometheus auth, p95, DuckDB, webhook ownership khi rotate key). Module map lên Phase 5/6/7/8, không tự build sớm vì phụ thuộc alert engine + notification engine + cold tier.
- Log management scope is intentionally lightweight: on-demand admin debugging via hub/agent/systemd/Docker logs, not Loki-style aggregation/search.
- UI polish is now tracked as a product requirement: benchmark Beszel for UX/completeness, redesign dashboard/host detail/settings without copying Beszel identity.
- Agent update UX is now a dedicated Phase 4: existing Docker agents update from their target VM/LXC via `docker compose pull && docker compose up -d`, preserving the existing token/config and avoiding host/token recreation or edits to the hub compose file.
- Tomorrow's implementation pass should update the actual hub/web onboarding surfaces to match the Docker-first docs: generated compose file first, install.sh/native path secondary.

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
