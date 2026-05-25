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
| **Repo (proposed)** | github.com/lumenhq/lumen |
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
| 2026-05-25 | HTTP router = github.com/go-chi/chi/v5 | Stdlib-friendly, middleware ecosystem, no codegen — matches "single binary, low RAM" decision. |
| 2026-05-25 | Metrics lib = github.com/shirou/gopsutil/v4 | De-facto cross-platform metrics for Go; covers Linux/Windows/macOS in one API. |

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

### Phase 1 — MVP technical spike (Week 1) ⏳

**Goal**: 1 end-to-end skinny slice. Validate stack hoạt động trước khi build wide.

- [x] `go.mod` init + dependency chọn (chi, gopsutil locked; gorilla/websocket + modernc.org/sqlite deferred to phase-of-use)
- [x] Skeleton dirs: `cmd/{lumen-hub,lumen-agent}/`, `internal/{hub,agent,shared}/`, `web/`
- [ ] `cmd/lumen-hub/main.go` — bind 8090, serve 1 endpoint `/healthz` *(stub exists, not yet listening)*
- [ ] `cmd/lumen-agent/main.go` — read 1 metric (CPU%), POST to hub mỗi 5s *(stub reads CPU once, no loop/POST yet)*
- [ ] Hub `internal/hub/ingest/` — accept POST, in-memory store last value per host
- [ ] Hub `internal/hub/stream/` — WS `/api/stream` echo current value mỗi 5s
- [ ] `web/` Vite scaffold + 1 page hiển thị live CPU% qua WS
- [ ] `embed.FS` web build vào hub binary
- [ ] Docker compose chạy được hub + 1 agent local
- [ ] Document: `how-to/run-from-source.md`

**Definition of done**: 1 binary chạy, mở browser, thấy CPU% live update từ agent.

---

### Phase 2 — MVP feature breadth (Week 2-4)

**Goal**: Đủ feature để 1 user homelab thật dùng được.

#### Hub
- [ ] Auth: register first-admin flow, JWT, password Argon2id
- [ ] Hosts CRUD + token generation (display once)
- [ ] SQLite schema migration framework
- [ ] Ring buffer in-memory per host
- [ ] Batch flush ring → SQLite mỗi 60s
- [ ] Query API: `GET /api/hosts/:id/metrics?from&to&step`
- [ ] WS subscribe/unsubscribe protocol
- [ ] Retention task (1h cron, delete >24h SQLite rows)
- [ ] Settings page: retention, password change

#### Agent
- [ ] Full host collector: CPU per-core, RAM, swap, disk usage + I/O, network, load, temperature
- [ ] Docker collector (Engine API)
- [ ] Local BoltDB buffer cho offline
- [ ] Config file YAML + env override
- [ ] Systemd service file
- [ ] Install script `get.lumenhq.dev/agent`

#### Web
- [ ] Overview page: host cards + sparklines
- [ ] Host detail: 4 charts (CPU/RAM/Disk/Net) với uPlot, container list
- [ ] Dark/light mode toggle
- [ ] Settings UI
- [ ] PWA manifest + service worker

#### Docs (parallel)
- [ ] `install/hub-compose.md`
- [ ] `install/hub-binary.md`
- [ ] `install/agent-linux.md`
- [ ] `install/agent-docker.md`
- [ ] `configure/hosts-and-tokens.md`
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

**Session**: 2026-05-25 (kickoff, day 1)
**Đang làm**: Phase 0 closed → vào Phase 1 (MVP spike)
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

**Bước tiếp theo cụ thể**:
1. ✅ Phase 1.1 — `go mod init github.com/lumenhq/lumen` + chi/gopsutil pinned (go.sum committed)
2. ✅ Phase 1.2 — Skeleton dirs created; both stub mains compile + run
3. ⏳ Phase 1.3 — Hub: bind `:8090`, serve `/healthz`
4. ⏳ Phase 1.4 — Agent: read CPU% mỗi 5s, POST tới hub
5. ⏳ Phase 1.5 — Hub WS `/api/stream` echo current value (lock websocket lib: gorilla vs coder)
6. ⏳ Phase 1.6 — Web: Vite scaffold + 1 page hiển thị live CPU qua WS
7. ⏳ Phase 1.7 — `embed.FS` web build vào hub binary
8. ⏳ Phase 1.8 — `docker-compose` dev (hub + 1 agent local)
9. ⏳ Phase 1 docs — `how-to/run-from-source.md`

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
