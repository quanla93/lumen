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
| **Repo** | github.com/quanla93/lumen (personal account, public OSS — no org transfer planned, decided 2026-06-02) |
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
| 2026-05-29 | "Custom UI kiểu Grafana" cho khách = mức 2 (expose data) + mức 3 (personalize nhẹ in-app), KHÔNG mở dashboard builder | Trả lời câu hỏi "cho khách custom UI như Grafana". **Mức 2** (khách dùng Grafana thật trên dữ liệu Lumen) đã có sẵn trong [Public API module](#-public-api--external-api--module-plan) §6: Prometheus exposition + Grafana Infinity datasource (P1) — *expose data, không build lại dashboard*. **Mức 3 (mới chốt là OK)**: personalize nhẹ *trong* fixed-views — sắp xếp/ẩn host card, chọn metric mặc định hiển thị, lưu 1-2 "view", theme/đơn vị; lưu per-user qua settings KV. **Ranh giới cứng**: KHÔNG panel tùy ý + query editor + grid kéo-thả — đó vẫn là anti-feature "dashboard builder", chỉ lật bằng decision mới. Mức 1 (builder đầy đủ trong Lumen) vẫn khóa. |
| 2026-05-29 | Phase 5 (Proxmox) redefined → then **deferred to ~v1**; do Alerting/other features first | Same-day arc: first redefined Phase 5 = **agentless** Proxmox-native (hub→API, not agent-on-node) + **split alerting out** into its own phase (Phase 6). Then, on objective user-value review, **deferred Proxmox** out of the immediate queue: a read-only mirror of nodes/cluster/ZFS/PBS **duplicates the Proxmox web UI** (same anti-pattern as rebuilding Grafana); the agent's push/stale model already gives faster failure detection than a 30s poll; and alerting + other features have higher daily value. Proxmox stays the marketing wedge but is not next. **When revived**: lean-core **"guests-as-hosts"** (agentless-pull each guest into the unified host model — shared dashboard/history/alerts, no per-guest agent), NOT a mirror tab; and it's the first of a general **integrations** pattern (ESXi/Docker/NAS/SNMP later), built concretely-first. Agentless poll cadence locked at 30s default + one bulk `cluster/resources` call + tiered slow calls + keep-alive (no per-guest fan-out). |
| 2026-05-29 | Tags become a first-class inventory (controlled vocabulary), not freeform `host_tags` rows | After shipping freeform host tags + label selectors (Milestone C), operators routinely typo'd values (`tier=critcal` vs `tier=critical`) or left stale references in rule selectors after deleting a tag. Promoted tags to a real resource: `tags` + `tag_values` tables (migration 0012, backfilled from any existing `host_tags`), a dedicated **Alerts → Tags** tab with CRUD on the inventory **plus** host assignment (consolidated — no more inline editor in Settings → Hosts), and per-key dropdowns in the rule selector picker instead of free-text chips. Deleting a tag or value cascades through `host_tags` and rewrites every affected rule's `host_selector` (`Selector.DropKey`/`DropPair`); the confirm dialog quotes host + rule counts via a dry-run `/api/tags/{key}/impact` so the operator sees blast radius before acting. `hosts.SetTags` rejects (key, value) pairs not in the inventory as a safety net. Tag rename is out of scope for v0.4.0 (each rename atomically touches three tables + rewrites selectors — its own ticket). |
| 2026-05-29 | Offline rules use a single 60s detection floor; no extra `for_seconds` clamp | The engine was applying `MinOfflineFor = 60s` twice on `offline` rules: once in `evaluateOne` (`age ≥ 60s` before reporting breach), once in the tick loop (clamping `for_seconds` up to 60s). Even `for_seconds=0` cost ~120s to fire — operators reasonably read "0" as "fire immediately on detect" and got a 2-minute wait. Dropped the `for_seconds` clamp; the 60s silence detection already absorbs ~12 missed heartbeats. `for_seconds=0` now fires on the first tick past 60s of silence; `for_seconds>0` still adds extra hold on top, as the i18n hint claims. |
| 2026-06-01 | **Mức 1 (full dashboard builder for Host detail) unlocked** — overrides the 2026-05-29 lock + RFC 0002 anti-feature list | Operator explicitly asked for "thêm, ẩn bớt các chart, di chuyển các chart, lên xuống theo vị trí, tự responsive" on the Host detail page. That is Mức 1 — full builder — which the 2026-05-29 decision parked behind a "must be reopened by a new decision". This is that decision. **What flips**: RFC 0002 anti-feature list (no add/remove panels, no drag-drop, no resize) is **superseded for the Host detail page only** — Dashboard host-grid stays fixed-views + Mức 3 (sort/hide/saved-views as already shipped in v0.6.0). The README "What Lumen is NOT" line *"Not a Grafana replacement — no dashboard builder, fixed views only"* is rewritten to *"Per-host Grafana-style layout via fixed chart catalog; Dashboard host grid stays fixed views"*. **Scope guardrail**: chart catalog stays operator-curated (Lumen-picked metric set, 10ish chart types — no query editor, no custom metrics). Operators can pick from the catalog, place, resize, hide; they cannot add arbitrary metrics or define new chart types. That keeps us out of "rebuild Grafana inside Lumen". **Implementation path**: RFC 0004 supersedes RFC 0002 §"Host detail customize" only. Shipping target: v0.6.0 (hold the personalization-only ship until builder lands). |
| 2026-06-02 | **Public-OSS flip without org transfer** — stay at `github.com/quanla93/lumen`, drop `lumenhq` org plan + `quanla.org` domain plan + Discord placeholder | Repo is shipping at v0.6.3; the original "stage as `quanla93` then transfer to `lumenhq` at v0.1" gate from 2026-05-25 has long since passed without the org actually getting created. Rather than block the OSS flip on org-bureaucracy + a batch rename of `go.mod` + ~30 import-path files + install-script URLs + ghcr tags, accept the personal-account namespace as permanent. **What flips:** SECURITY.md + CODE_OF_CONDUCT.md route private reports through GitHub Security Advisories (zero personal-email exposure); Discord placeholders dropped from CONTRIBUTING / SUPPORT / CoC scope (GitHub Discussions is enough until real demand surfaces); README "pre-1.0" disclaimer no longer mentions `quanla.org` / placeholder URLs; project identity table drops the Domain row. **Re-evaluate only if** a real maintainer org forms — the 2026-05-25 Decisions log "Repo staging = `quanla93/lumen` (PRIVATE)" entry is superseded by this one. |

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
> ⏸️ **Deferred post-v0.3 (2026-05-29).** v0.3.0 (Phase 4) shipped without log management. Only the product direction is locked; the RFC + Logs/Console surface move to a later release. Pick up before/with Phase 5 as capacity allows.
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
- [x] Agent includes build/version metadata in every ingest and host metadata update (fixed ldflags `main.Version` mismatch — agent `var Version`; `SystemMetadata.AgentVersion` already plumbed)
- [x] Hub exposes latest bundled agent image/version metadata from the running hub build (`GET /api/version` → `{hub_version, latest_agent_version}`; same release train so latest == hub build)
- [x] Host list/detail UI shows current agent version vs latest available version (host detail meta line shows `agent <ver>` + update-available pill; dashboard host card shows update badge; `"dev"` builds suppress it)
- [x] Docs explain that token rotation is unrelated to code updates; existing compose credentials stay valid during update

#### Compose-first onboarding
- [x] Token reveal shows a complete per-agent `docker-compose.yml`
- [x] Token reveal supports copying and downloading `docker-compose.yml`
- [x] Token reveal shows copy-ready commands: `mkdir -p /opt/lumen-agent && cd /opt/lumen-agent`, save the file, then `docker compose up -d`
- [x] Generated compose file uses stable container name, `restart: unless-stopped`, `user: "0:0"`, hub URL, host name, token, interval, and optional Docker socket mount
- [x] `docker run` remains available as a quick fallback, but docs and UI recommend Compose for long-running agents

#### Update path
- [x] Document update as: SSH into the VM/LXC that owns the agent compose file, then run `cd /opt/lumen-agent && docker compose pull && docker compose up -d`
- [x] Host detail `Update agent` panel shows the Compose update command and explains it must be run on the machine where that compose file exists (always-present card with copy button + up-to-date/update-available status; header pill also copies the command when an update is available)
- [x] No update flow creates or rotates host tokens unless the operator explicitly clicks rotate
- [x] No update flow requires editing the hub's `docker-compose.yml` or project `.env`

#### Product + safety
- [x] Docs cover initial install, update, restart, logs, and uninstall for the per-agent compose directory
- [x] Docs clarify that the token is shown once by the hub but then persists in the target host's `docker-compose.yml`
- [x] Keep any custom Docker updater prototype documented as experimental/advanced only, not the primary UX

**Definition of done**: A user can create a host token, download/copy a per-agent `docker-compose.yml`, start the agent with `docker compose up -d`, and later update it with `docker compose pull && docker compose up -d` without creating a new host/token or using Watchtower/custom updater tooling.

---

### Phase 5 — Proxmox-native monitoring (agentless) ⏸️ DEFERRED to ~v1

> ⏸️ **DEFERRED 2026-05-29** after an objective review (see Decisions log). Reasons: a read-only mirror of nodes/cluster/ZFS/PBS largely **duplicates the Proxmox web UI** (same anti-pattern as rebuilding Grafana); fast failure detection is already served better by the agent's push/stale model; and **Alerting + other features have higher day-to-day user value**. Proxmox remains the marketing wedge but is **not the immediate next phase** — revisit for the v1 push or when there is real user demand. The agentless definition below is kept as the spec for when we pick it up.
>
> **If/when revived**, the valuable shape is **lean-core "guests-as-hosts"** (agentless-pull each guest's CPU/RAM into the unified host model → shared dashboard + history + alerts, no per-guest agent install), NOT a separate Proxmox mirror tab. The agentless-pull mechanism is also the first of a general "integrations" pattern (ESXi / Docker / NAS / SNMP later) — build Proxmox concretely first, generalise only on real demand.

**Goal (when revived)**: Read the Proxmox VE REST API directly (agentless, API-token auth) so users see the virtualization layer (nodes, guests, cluster, ZFS, backups) inside Lumen without installing anything on Proxmox.

**Spec (when revived)**: heart = agentless hub→Proxmox API; agent-in-LXC collector + guest history charts deferred further. Auth = Proxmox **API token** (read-only `PVEAuditor`) via `Authorization: PVEAPIToken=<id>=<secret>`; verify-TLS toggle (default off for self-signed); Test-connection action. **Cadence (anti-overload)**: default 30s poll (env `LUMEN_HUB_PROXMOX_INTERVAL`), one bulk `cluster/resources` call (no per-guest fan-out on the hot path), tiered slow cadence for ZFS/backups (~5m), HTTP keep-alive. `cluster/resources` is the inventory workhorse.

**v0.4 scope (5A–5E):**
- [ ] **5A Connection** — `proxmox_sources` table (URL + token id/secret + verify_tls), CRUD API, Test-connection (`GET /version`), Settings → Proxmox section
- [ ] **5A** Background poller (`internal/hub/proxmox`, ctx goroutine + settings-driven interval), in-memory state store, `GET /api/proxmox/state`
- [ ] **5B Inventory** — `GET /cluster/resources`: nodes summary + guests list, **LXC vs QEMU** distinction, running/stopped, CPU/RAM/disk; new top-level **Proxmox** tab
- [ ] **5C Cluster** — `GET /cluster/status`: multi-node topology + quorum
- [ ] **5D Storage** — ZFS pools (`/nodes/{node}/disks/zfs`: health/size/alloc) + datastore usage from `cluster/resources`
- [ ] **5E Backups (PBS)** — recent backup task status (`/nodes/{node}/tasks?typefilter=vzdump`) + backup-type storage usage; full PBS-server API deferred
- [ ] Docs: `integrations/proxmox.md` (API token creation, add source, TLS/reachability)

**Deferred past v0.4:**
- [ ] 5F Guest metrics over time (per-VM/LXC history charts)
- [ ] LXC collector trong agent (agent-side deep OS metrics) + LXC helper script installer
- [ ] Migration history view
- [ ] Split docs: `integrations/lxc.md` / `integrations/zfs.md` / `integrations/pbs.md`

**Definition of done (v0.4)**: Add 1 Proxmox API token connection, Test-connection succeeds, and the Proxmox tab shows nodes + LXC/QEMU guests with state + cluster quorum + ZFS pools + recent backup status — reflecting changes (stop/start a guest) within one poll interval.

---

### Phase 6 — v0.4: Alerting & notifications ✅ CLOSED (released v0.4.0 — 2026-05-29; wrapped v0.4.5 — 2026-05-31)

**Goal**: Threshold-based alerting for **any** host/metric (incl. Proxmox guests once Phase 5 lands). Split out of the old Phase 5 (2026-05-29) — alerting is a general feature, not Proxmox-specific.

> 📋 **Implementation plan: [`docs/rfcs/0001-alerting.md`](docs/rfcs/0001-alerting.md)** — Milestones A–D shipped v0.4.0, follow-ups across v0.4.1 → v0.4.5.

- [x] Alert engine v1: rules (threshold-based on metric + comparator + duration), evaluation loop over the in-memory store + history (Milestone A — `internal/hub/alerts/engine.go`, eval every `alerts.eval_interval`, default 15s; offline metric uses a 60s silence-detection floor, then `for_seconds` adds extra hold on top — `for_seconds=0` fires at ~60s after silence, not 120s)
- [x] Alert state model: firing/resolved transitions, dedup, cooldown; persist alert events (per-(rule, host) in-memory state machine; persisted `alert_events` table; dedup via state — one firing row per breach until resolved)
- [x] Notification channels: ntfy + Discord + Webhook + **Telegram** + **Email (SMTP)** (HTTP POST shared dispatcher in `notify.go`; Test action). Webhook HMAC arrives with the Public API webhook unification.
- [x] Per-rule channel routing + per-channel `min_severity` floor (Milestone B).
- [x] Host tag inventory + label selectors (Milestone C): controlled `tags` + `tag_values` vocabulary (migration 0012), Alerts → Tags tab does inventory CRUD **and** host assignment, rule selector picker = per-key dropdowns, delete-tag cascades through `host_tags` + rewrites every rule selector via `Selector.DropKey`/`DropPair` after a confirm dialog with impact preview.
- [x] Persisted notification delivery queue (Milestone D): `notification_deliveries` table (migration 0011), background worker pool with per-channel serialisation, severity-aware retry (critical fails fast in ~5 min; warning/info back off to ~4 h), Deliveries sub-tab with filters + retry-now.
- [x] Alerts UI: rule CRUD + active/resolved list + deliveries + tag inventory (`web/src/components/Alerts.tsx`, `AlertTags.tsx`; Active / History / Rules / Channels / Deliveries / Tags sub-tabs).
- [x] **Noise-reduction levers (v0.4.5)**: per-rule `cooldown_seconds` (flap suppression) + per-host `silenced_until` (maintenance window). Operator picks per-rule cooldown for "this rule itself flaps" or per-host silence for "I'm about to restart the agent — please be quiet."
- [x] Docs: `configure/alerts.md` (`integrations/notifications.md` collapsed in — channel setup lives alongside the rule docs).

**Definition of done**: Create a CPU>90%-for-5m rule, get an Email/Discord/Telegram/ntfy notification when a host (or Proxmox guest) breaches it, and see it resolve. ✅

---

### Phase 6.x — v0.4.1+: Alerting follow-ups + server-test bugfixes

> Picked up *after* the user's server-test feedback on v0.4.0 lands. Order below is the current priority — retention is #1 because today neither `alert_events` nor `notification_deliveries` get pruned, only `snapshots` does, so the tables grow unbounded under real traffic and that will bite within days. Reassess if real usage surfaces a different #1 pain.

#### v0.4.1 — Retention sweep + scrollback + dashboard KPI rework + stale/offline unification  *(shipped 2026-05-31)*

Slot grew beyond the original "retention only" plan because server-test feedback surfaced three other pain points in the same window; bundled them rather than minting four micro-tags.

- [x] **Retention sweep** — new `retention.delete_alerts_after` setting (default `30d` / `720h`) reusing `internal/hub/retention/retention.go` (heartbeat + cutoff query). Env override `LUMEN_HUB_RETENTION_ALERTS_WINDOW`. Sweeps `alert_events` (`state='resolved'`, `COALESCE(resolved_at, started_at) < cutoff`) and `notification_deliveries` (`status IN ('sent','failed','dropped')`, `COALESCE(sent_at, created_at) < cutoff`); firing events + pending/inflight deliveries always survive. Settings UI + i18n via `RetentionSettings` DurationField. Tests: `storage/retention_alerts_test.go`, `retention/retention_test.go`, `settings/handlers_test.go`. Docs: `configure/alerts.md` Retention section.
- [x] **"Load more" pagination** for Alerts History + Deliveries — server limit cap raised 500→2000 on `/api/alerts/events` + `/api/alerts/deliveries`; UI footer with row count + 200-row stepping; filter/state change resets to 200; auto-refresh keeps current page size. New i18n keys `alerts.loadedCount` / `loadMore` / `loadMoreCeiling`.
- [x] **Dashboard KPI rework** — replaced "Avg CPU / Avg RAM" with "Hottest CPU / Hottest RAM / Hottest Disk" + host name, computed over live (non-stale) snapshots only. Fleet-average was a borrowed cluster KPI that diluted hot hosts in a discrete fleet (homelab + VPSes); hottest-host-per-metric matches the actual operator workflow ("which box is on fire?"). New i18n `dashboard.hottestCpu/Ram/Disk/noLiveHost`; removed `avgCpu/avgRam/fleetAverage`.
- [x] **Stale/offline threshold unified.** Pre-fix, `MinOfflineFor` was hardcoded 60s while the UI stale window scaled with `agent_interval`. For `agent_interval ≥ 60s` the alert fired BEFORE the dashboard marked the host yellow ("I got a push but the dashboard is still green"). New `OfflineAfter(interval) = max(2 × max(2*interval, 30s), 60s)` derived per-tick from the `agent.interval` setting; default 5s interval keeps the 60s threshold so existing rule timing is unchanged. UI stale yellow now strictly precedes alert red at every interval.

#### v0.4.2 / v0.4.3 / v0.4.4 — Stream reliability + clipboard fallback  *(shipped 2026-05-31, ate three patch slots)*

Numbered slots for Email/Flap/tag-rename got reassigned in-flight because dashboard-stale-while-idle and broken copy buttons turned up as bigger pain in real use. Original backlog moves to v0.4.5+ (below).

- [x] **v0.4.2 (git tag only, image cancelled)** — Dashboard / HostDetail WebSocket auto-reconnect (`useStreamConnection` hook with exponential backoff 1s→30s + `visibilitychange` force-reconnect; `onOpen` callback re-sends subscribe frame on every reconnect). Server-side `/api/stream` keepalive (ping every 30s, `SetReadDeadline 60s`, `PongHandler` + control-frame extends deadline). Image build was cancelled at ~25 min QEMU emulation when the operator confirmed the fleet is 100% x86 — the WS + keepalive code shipped under v0.4.3 instead.
- [x] **v0.4.3** — bundles v0.4.2's WS work + drops arm64 + armv7 from `release.yml` and `Dockerfile.hub` cross-build (operator fleet is 100% x86; QEMU emulation was costing ~40 min/tag for zero consumers — amd64-only lands in 2-3 min). Includes the `errcheck` fix for the two `SetReadDeadline` calls that the keepalive commit added without checking the return value.
- [x] **v0.4.4** — copy buttons (`TokenReveal` token reveal, `HostDetail` agent update commands) work on plain HTTP via `document.execCommand("copy")` fallback in `web/src/lib/clipboard.ts`. `navigator.clipboard.writeText` requires a secure context and silently no-op'd (or threw) when the dashboard was loaded over a LAN IP. Modern API takes over transparently when HTTPS lands.

#### v0.4.5 — Email (SMTP) channel + flap cooldown + per-host silence  *(shipped 2026-05-31, closes Phase 6.x)*

Wrap-up patch. Originally planned as Email-only (was v0.4.2 slot); absorbed Flap (was v0.4.6) and Maintenance window (surfaced as v0.4.7?) because all three are "alert noise reduction" and the engine evaluate() change touches them together — splitting would have left it half-instrumented.

- [x] **Email (SMTP) channel.** New `email` type alongside ntfy/Discord/webhook/Telegram. Config: `smtp_host`, `smtp_port`, `username`, `password` (masked on read), `from_addr`, `to_addr` (single recipient — multi recipient deferred). Dispatcher uses `net/smtp` over a context-aware `net.Dialer`, with PLAIN auth over STARTTLS (port 587) or implicit TLS (port 465). AUTH is gated on an encrypted connection — bare-plaintext relays like MailHog that advertise AUTH but don't require it now work transparently (stdlib's `PlainAuth` would otherwise refuse with "unencrypted connection"). Docs: `configure/alerts.md` Email section with Gmail/Outlook/SendGrid/SES setup recipes, troubleshooting table, swaks one-liner for credential sanity-check.
- [x] **Per-rule flap cooldown.** New `alert_rules.cooldown_seconds` column (migration 0013, default 0 = preserves pre-cooldown behavior). Engine tracks `ruleState.lastFiredAt`; firing transitions inside the cooldown window flip `firing=true` (so the next resolve still emits) but skip both `alert_events` insert and delivery queue insert — flap-prone rules stay out of both the channel AND the history table.
- [x] **Per-host maintenance silence.** New `hosts.silenced_until` column (migration 0014, nullable unix epoch). Engine refreshes silence map each `runOnce`; evaluate skips firing + resolved transitions for silenced hosts AND leaves `firing=false` so the rule re-evaluates from scratch after silence expires. New `POST /api/hosts/{id}/silence` (body `{seconds}`, max 7 days) + `DELETE /api/hosts/{id}/silence`; HostDetail page gets a SilencePanel with 15m / 1h / 4h / 24h presets and a "Lift silence" button while active.

**Phase 6 backlog (descoped from v0.4.x — pick up when user demand surfaces):**
- **Tag key rename** — atomic three-table swap (`tags` + `tag_values` + `host_tags`) + rewrite every affected `host_selector`. Workaround today is delete-and-recreate; rename only matters once a fleet rename touches dozens of rules.
- **Email subject template** — operator overrides default `[{{.Severity}}] {{.RuleName}} · {{.Host}}` with Go `text/template`. ~20 lines + 1 UI field. Most-likely first ask once the channel sees real production traffic.
- **Email body template** — full plain-text override; textarea + preview button. Useful for embedding runbook URL / dashboard deep-link.
- **HTML body option** — `Content-Type: text/html` with escape helpers, multipart/alternative fallback. Matches Alertmanager / Grafana / PagerDuty; higher edge-case surface — only pursue after operator asks for it.
- **Multi-recipient email** — split `to_addr` on comma, RCPT TO each. Trivial after single-recipient ships.
- **Maintenance auto-detect** — engine watches for "rule fires + resolves within N seconds" and auto-suppresses next K occurrences. Alternative to per-host silence for "I forgot to silence before restarting" scenarios.
- **Derived / rate metrics** — "RAM grew >10%/min" etc. Discrete fleets rarely need this; defer.
- **Webhook HMAC signing** — blocked by Public API webhook unification (Decisions 2026-05-29).
- **Pre-aggregated fleet summary on WS frame** — FE currently aggregates raw snapshots in `summarizeSnapshots`; cheap for <50 hosts. Backend `fleet_summary` field needed when fleet size + browser count make raw-firehose impractical.

---

### Phase 7 — v0.5/v0.6: External API + Cold tier 🚧 (Public API in flight; Cold tier deferred behind demand signal)

**Goal**: Open Lumen's data to external integrations (Grafana, automation, scripts) without forcing Cold tier first. Ship Cold tier only if SQLite + the v0.4.1 retention sweep proves insufficient for real homelab fleets.

**Reorder rationale (2026-06-01)**: original plan had Cold tier as the centerpiece with external API downstream (because cold tier serves long-range queries). Flipped because:
- Public API is a low-risk expose-layer over data we already have; ships in ~1 week.
- Cold tier is a multi-week storage rewrite with cgo / DuckDB unknowns.
- Demand-driven: ship Public API first, measure if anyone actually queries >7d ranges, then decide if Cold tier is justified.
- Fleet math: homelab (≤30 hosts, 30-90d retention) is already comfortably bounded by the v0.4.1 retention sweep — Cold tier only matters for power users above that.

See the **Public API / External API — Module Plan** section below for the detailed PAPI Phase 1..5 surface mapping; Phase 7's v0.5.0 ships PAPI Phase 1.

#### v0.5.0 — Public Read API 🚧 (in flight)

Four small patches under v0.4.x, then cut v0.5.0 as the sum.
- [x] **v0.4.11** — `Settings → API Keys` admin UI + `/api/apikeys` CRUD (mint / list / revoke, plaintext-shown-once flow, glob host_filter, scopes). `/api/v1/version` + `/api/v1/hosts` skeleton endpoints with Bearer auth middleware + per-key in-memory token bucket (100/min) + public envelope `{success, data, error, request_id}`. Migration 0016. Bilingual.
- [x] **v0.5.0** — completes the v1 read surface AND ships the docs: `/api/v1/hosts/{name}`, `/api/v1/hosts/{name}/metrics?from=&to=&bucket=` (≤7d, bucket required ≥30s, cap 1000 points), `/api/v1/alerts/events?state=&limit=`, `/api/v1/alerts/rules`. Host filter + scope checks honored on every endpoint. RFC 0003 + `reference/public-api.md` (endpoint catalog, error codes, rate-limit headers, Grafana JSON datasource recipe). Public Read API is now functional end-to-end. Docs folded in (was originally split to v0.5.1).

#### v0.6.0 — RFC 0002 PR2 Level 3 personalization ✅ (shipped 2026-06-01)

Pulled forward from Phase 8 because the operator asked for it next and Cold tier still has no concrete demand signal. v0.6.0 ships per-user prefs (theme/language/units/reduce-motion) + Dashboard customize popover (sort + hide-host); migrate `lumen.theme` / `lumen.locale` localStorage onto server-side `user_prefs(user_id, key, json_value)` (migration 0017). Saved views + density toggle are schema-reserved but UI lands in v0.6.x.

#### v0.7+ — Cold tier (still deferred behind demand signal)

Only ship if Public API metrics endpoint surfaces real demand for >7d queries, OR if SQLite size on a real deployment crosses ~10 GB. If neither happens, skip to v0.8 polish.

- [ ] **DuckDB feasibility spike + ADR** (do this first): packaging (cgo or native distro pkg), memory footprint on a Pi, query latency vs raw SQLite, whether DuckDB should be default-on / optional / off-by-default for homelab installs.
- [ ] Parquet writer (parquet-go).
- [ ] Compaction job: SQLite > configurable hot window → Parquet using configurable downsample bucket/policy.
- [ ] Query layer transparent over SQLite + Parquet (this is where DuckDB or hand-rolled merge logic lives, decided by the spike).
- [ ] Optional DuckDB cold-query layer only if spike confirms it is practical for homelab installs.
- [ ] Docs: `how-to/reduce-disk-writes-further.md`.
- [ ] Benchmark: write IOPS/day vs Beszel + Prometheus, post on docs.

#### v0.6.x — Multi-user + Grafana integration (independent of Cold tier)

These were originally lumped with Cold tier in the old Phase 7 because they were the same Phase 7 box. They're actually unblocked once the Public API surface exists, so they slot in alongside Cold tier (and ship even if Cold tier is skipped).

- [ ] Multi-user (admin + read-only viewer).
- [ ] TOTP 2FA.
- [ ] Grafana integration spike: prove a user can build Grafana dashboards from Lumen Public API data without using Lumen's web UI (start with the community JSON datasource plugin).
- [ ] Decide Prometheus-compatible endpoint vs Grafana datasource plugin vs plain REST/SQL-over-HTTP style API based on the spike output.

---

### Phase 8 — v0.7+: Polish & deferred product features

- [x] **Self-hosted SSO: custom OIDC provider config** (issuer / client ID / client secret / scopes / expected admin email), with local admin fallback preserved — *shipped v0.7.0 (2026-06-04)*. Single-admin scope. Settings → SSO tab + login button. Encrypted client_secret at rest via AES-GCM keyed off `LUMEN_HUB_SECRET`. Docs in `docs/configure/sso.md` with Authentik / Keycloak / Google recipes.
- [ ] SAML2 evaluation after OIDC; implement only if dependency and configuration complexity stay acceptable for homelab/self-hosted use
- [x] Backup RFC/UX: local/S3-compatible backup, restore flow, encryption, retention, and whether backup belongs in core or optional module — *shipped v0.7.1 (2026-06-08)*. Local + S3 targets, AES-256-GCM with Argon2id-derived key, cron scheduler with hot-reload + backoff, CLI restore + Web UI restore, 7 endpoints, Settings → Backup tab. Docs in `docs/configure/backup.md`.
- [ ] External data API/export RFC follow-up: Grafana first, auth model, query shape, rate limits, and Prometheus-compatible endpoint vs Grafana datasource plugin vs plain REST
- [ ] Grafana integration spike follow-up: prove a user can build Grafana dashboards from Lumen monitoring data without using Lumen's web UI
- [ ] First-run onboarding flow: create admin → add first host → use generated per-agent Docker Compose setup → wait for first metrics
- [x] **Public status page** (read-only share) — *shipped 2026-06-04*. `/status` route, per-host opt-in via `public_visible` column, Settings → Status page tab, polls every 15s. `Cache-Control: no-store`. Docs in `docs/configure/public-status.md`.
- [x] **Web Push notifications (VAPID)** — *shipped 2026-06-04*. New `web_push` channel type. VAPID key pair generated on first use, AES-GCM-encrypted at rest. Per-browser subscriptions in `web_push_subscriptions` table (migration 0019); 404/410 auto-prune. Service worker push + notificationclick handlers in `/sw.js`. Backend: `internal/hub/webpush` (SherClockHolmes/webpush-go dep) + `Dispatch` extended with `DispatchDeps{DB, HubSecret}`. Frontend: WebPushPanel inside the channel form. Docs in `docs/configure/web-push.md`.
- [ ] i18n polish follow-up: expand translations to new modules after the Phase 3 foundation lands
- [ ] Translation docs and contribution guide for adding/changing UI copy

#### Phase 8 — Sprint queue (2026-06-04 plan)

Locked execution order. SAML2 elevated above Beszel-parity per operator decision; Power monitoring explicitly dropped after the user evaluated the four mechanisms (smart plug / IPMI / RAPL / NUT) and declined to add it. Cold tier stays deferred behind demand signal.

| # | Sprint | Effort | RFC | Items |
|---|---|---|---|---|
| 1 | **Backup** | 5d | [RFC 0001](docs/rfc/0001-backup-restore.md) | RFC → engine → CLI restore → Web UI restore → frontend · shipped v0.7.1 |
| 2 | **SAML2** | 5d | [RFC 0002](docs/rfc/0002-saml-sso.md) | RFC → AuthnRequest+ACS → metadata + frontend → tests + docs · shipped v0.7.2 |
| 3 | **Beszel bundle 1** | 5d | [RFC 0003](docs/rfc/0003-beszel-bundle-1.md) | GPU monitoring (2d) + Process list top-N (1.5d) + Maintenance windows (1.5d) · shipped v0.7.3 |
| 4 | **Notification quality** | 3d | [RFC 0004](docs/rfc/0004-notification-quality.md) | Digest/grouping (1d) + per-host share link (1d) + Slack-native channel (0.5d) + multi-recipient email (0.5d) |
| 5 | **First-run onboarding** | 4d | [RFC 0005](docs/rfc/0005-onboarding.md) | 4-step guided wizard replacing ad-hoc bootstrap; Replay button in Settings |
| 6 | **WebAuthn/passkey** | 4d | [RFC 0006](docs/rfc/0006-webauthn.md) | `go-webauthn` register/login flows + Settings → Account credentials list |
| 7 | **i18n polish + translation docs** | 3d | [RFC 0007](docs/rfc/0007-i18n-polish.md) | Audit hardcoded EN strings; `docs/contributing/i18n.md`; lint CI for key parity |
| 8 | **Multi-user + TOTP 2FA** (v0.6.x backlog) | 6-7d | [RFC 0008](docs/rfc/0008-multi-user-totp.md) | `role` column on users; admin/viewer middleware; Settings → Users invite flow. TOTP via `pquerna/otp` with recovery codes |
| 9 | **External API + Grafana spike** | 8d | [RFC 0009](docs/rfc/0009-grafana-integration.md) | Wire Grafana JSON datasource against existing `/api/v1/hosts/:name/metrics`; decide Prometheus-compat or stay JSON; `docs/integrations/grafana.md` |
| 10 | **Cold tier** (conditional) | 14d+ | ADR 0004 (pending) | DuckDB feasibility → Parquet writer (`parquet-go`) → compaction → query layer → benchmark vs Beszel + Prometheus. Only if demand signal: operator queries >7d range OR SQLite >10GB on real fleet. |

**Windows agent dropped** (2026-06-04 operator decision) — homelab fleet stays Linux + macOS. Power monitoring also explicitly dropped same session after evaluating IPMI / RAPL / NUT / smart-plug paths.

**Lower-priority queue** (do when capacity / user requests):
- Hub auto-update (Watchtower-style) — 2d
- LDAP/AD bind — 3d
- Hibernation/sleep detection — 1d

**Sprint-level scope details** (preserved here so a future session has the recipe without re-deriving):

- **Backup (Sprint 1)**: `lumen.db` via SQLite `VACUUM INTO` → gzip → AES-GCM (passphrase derived via Argon2id) → local path OR S3-compatible (AWS / MinIO / R2 / B2 via `aws-sdk-go-v2` with `BaseEndpoint`). Cron scheduler (`robfig/cron/v3`). Settings → Backup tab with passphrase, target, S3 fields, cron, retention N, manual button + recent list. **Restore = CLI + Web UI both** (user choice 2026-06-04): CLI `lumen-hub --restore=<file>` is canonical/safe; Web UI is convenience (downloads + decrypts + SIGHUP self-restart, documented as "use CLI for production"). **S3 creds via Settings UI** with secret_key AES-GCM-encrypted at rest (same KEK pattern as OIDC client_secret). **Cron custom** (admin pastes expression).

- **SAML2 (Sprint 2)**: `crewjam/saml` SP role. Endpoints `/api/auth/saml/{login,acs,metadata}` (public) + `/api/settings/saml{,/test-metadata}` (session). Settings keys `saml.{enabled,idp_metadata_xml,sp_entity_id,expected_nameid,sp_private_key_enc,sp_cert}`. SP key+cert auto-generated on first save, encrypted at rest. Single-admin gate: only configured `expected_nameid` is allowed in. Phase 1 = signed-not-encrypted assertions only; encryption + SLO deferred. Test against `samltest.id` public IdP first; recipes in docs for Okta classic / Azure AD enterprise / ADFS / Shibboleth.

- **Beszel bundle 1 (Sprint 3)**:
  - **GPU monitoring**: agent reads `nvidia-smi --query-gpu=...` (NVIDIA) or `rocm-smi --json` (AMD). Schema extends `HostSnapshot` with `gpus: [{name, util_pct, mem_used_mb, mem_total_mb, temp_c}]`. Host detail charts + alert metric types `gpu_util/gpu_temp/gpu_mem_pct`. Docker containers need `/dev/nvidia*` mounted — documented gotcha.
  - **Process list top-N**: gopsutil `process.Processes()` sorted by CPU/RAM, top 10 (settings: enable/disable + N up to 50). Default OFF because cmdline can leak secrets — document the trade-off in `docs/configure/processes.md`.
  - **Maintenance windows**: new table `maintenance_windows(id, start_at, end_at, reason, scope_tags JSON, created_by, created_at)`. Alerts engine skips rules whose host matches `scope_tags` during the window. UI: `Alerts → Maintenance` tab — create, list active/upcoming/past. Timezone-aware in UI; storage stays UTC.

- **Notification quality (Sprint 4)**:
  - **Digest**: per-channel `digest_window` setting (0=off, 1m/5m/15m/1h). Dispatcher buffers events in the window, flushes one combined notification with body `"5 alerts: …"`. Default 0 (off); trade-off documented (latency vs spam).
  - **Per-host share link**: new table `host_share_tokens(token, host_id, expires_at, created_by, label)`. `GET /api/public/host/{token}` returns one host's read-only data plus charts. Host detail → "Share link" button mints token + copies URL. Expiry mandatory (default 7d).
  - **Slack-native**: new channel type `slack` reusing `url` field for the Incoming Webhook URL + optional `channel` override. `dispatchSlack` formats Block Kit (color by severity, fields for host/metric/value, action button).
  - **Multi-recipient email**: `to_addr` accepts comma-separated; SMTP `RCPT TO:` loop; per-address validation.

- **First-run onboarding (Sprint 5)**: 4-step wizard. (1) Welcome + create admin (reuses /register). (2) Add first host (form, mints token). (3) Generated install command (copy-paste docker-compose, prominent). (4) Wait for first metrics (live polling `/api/hosts`, success animation when one arrives). Shown when `setup-status` returns no admin OR no hosts. Settings → Onboarding has a "Replay" button so the admin can revisit. Overlay-style; doesn't refactor App routing.

- **WebAuthn/passkey (Sprint 6)**: `go-webauthn/webauthn`. Schema `webauthn_credentials(user_id, credential_id, public_key, counter, transports, attestation_type, created_at, last_used_at, label)`. Endpoints: `/api/auth/webauthn/register/{begin,finish}` + `/login/{begin,finish}`. Settings → Account "Add passkey" + list with revoke. Login form: "Sign in with passkey" alongside password. Document HTTPS-required gotcha (localhost works because of WebAuthn dev exception).

- **i18n polish (Sprint 7)**: grep hardcoded English in SSO/status page/web push panels/alerts channel hints; promote to `messages.ts`. Add namespace for dynamic pluralization ("1 host" / "5 hosts"). `docs/contributing/i18n.md`. CI script that fails when `en` and `vi` key sets diverge.

- **Multi-user + TOTP (Sprint 8)**:
  - **Multi-user**: `role` column on `users` (`admin` | `viewer`). `RequireRole` middleware. Settings → Users tab (list, invite via one-time signup link, set role, revoke). Existing single admin migrates as `admin`. SSO/SAML/WebAuthn bind via email. Multi-tenant scope stays in v1.
  - **TOTP**: `pquerna/otp`. `users.totp_secret_enc` (AES-GCM keyed off hub secret). Settings → Account → Enable 2FA: QR + verify-code save. Login form gains "code" field when enabled. 10 recovery codes generated at setup, hashed at rest.

- **External API + Grafana (Sprint 9)**: spike with Grafana JSON datasource against existing `/api/v1/hosts/:name/metrics` (built v0.5). Decide stick-with-JSON or add Prometheus-compatible subset (`/api/v1/prometheus/api/v1/{query,query_range}`). If Prometheus path picked: +3d. Docs: `docs/integrations/grafana.md` with datasource config + sample dashboard JSON.

- **Cold tier (Sprint 10, conditional)**: only if demand signal materialises. ADR 0004 evaluates DuckDB packaging (cgo footprint, Pi memory, embedded distribution). Parquet writer with `parquet-go`, one file per (host, downsample bucket). Compaction job sweeps SQLite rows past hot window → Parquet → SQLite delete. Query layer merges hot (SQLite) + cold (Parquet) transparently. Benchmark vs Beszel + Prometheus on standard fleet shapes. `docs/how-to/reduce-disk-writes-further.md`.

**Total**: ~3 months part-time for Sprints 1-9 (after dropping Windows agent). Sprints 1-4 (Backup + SAML + Beszel bundle 1 + Notification quality) = "core value" month; ship → re-evaluate based on user feedback.

**How to apply**: pick top of queue. When a sprint ships, tick the corresponding [ ] checkbox above (Self-hosted SSO, Public status page, Web Push already done — follow that pattern for Backup, SAML, etc.). Do not skip sprints unless a hard external blocker appears.

#### Public landing / marketing site
- [x] Keep `brand/index.html` as a standalone landing page, separate from the authenticated hub dashboard
- [x] Evolve landing page into Lumen's public home: product positioning, install CTA, docs link, GitHub link, roadmap highlights — bilingual EN/VI (2026-06-02)
- [x] Deployment target chosen: **Cloudflare Pages** (auto-deploy from `main` branch, build output `brand/`), production URL `https://lumen.quanla.org/`. Setup notes in `brand/DEPLOY.md`.
- [x] Add screenshots of Dashboard / Host detail / Alerts — *shipped 2026-06-03 (v0.6.5)*. Seven PNGs under `brand/screenshots/` (dashboard light + dark, host detail, settings, settings-display, alerts, mobile-dashboard) plus a screenshots section on the landing page and README.

---

### Phase 9 — v1.0: Stable

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

**Session**: 2026-06-08 (current)
**Đang làm**: **Sprint 3 — Beszel bundle 1 (GPU + processes + maintenance windows, RFC 0003).** Pulled after Sprints 1+2 shipped v0.7.1 (Backup) and v0.7.2 (SAML2) on 2026-06-08. Three Beszel-comparable features bundle'd into one release: GPU monitoring (NVIDIA nvidia-smi + AMD rocm-smi, live-only like per-core CPU + containers), top-N process list (gopsutil, default OFF for the cmdline-leaks-secrets trade-off), maintenance windows (alerts engine skips matching rules during a time window with tag-scope selector). Three doc pages so each topic is self-contained.

**Plan 4 ngày (locked, verify từng bước trước khi step tiếp)**:

| Day | Output | Verify |
|---|---|---|
| **D1** | `internal/shared/api` types: `GPUInfo`, `ProcessInfo`; extend `HostSnapshot` with `GPUs []GPUInfo` + `Processes []ProcessInfo`; `internal/agent/collector/gpu.go` (detect nvidia-smi / rocm-smi on $PATH, parse CSV / JSON, return empty if missing both, cache executable lookup at startup) + `internal/agent/collector/processes.go` (gopsutil `process.Processes()` → cmdline truncated to 200 chars + sort + truncate to N, off by default via env + server gate) + migration `0022_gpu_processes_maintenance.sql` (maintenance_windows table + 3 default `processes.*` settings rows) | `gpu_test.go` parses fixture nvidia-smi CSV + rocm-smi JSON; missing exec = empty slice; `processes_test.go` sorts by CPU vs RSS + top-N truncation |
| **D2** | `internal/hub/maintenance/` package: schema loader + cached `[]MaintenanceWindow` + matchScope (subset-of-tags semantics) + handlers (4 endpoints: `POST/GET/PUT/DELETE /api/maintenance`, GET supports `?state=active|upcoming|past` filter); alerts engine integration: skip notify + skip event insert when `now ∈ [start,end]` AND `matchScope(rule.Tags, win.ScopeTags)` — reuses the host-silence pattern | `maintenance_test.go`: scope match (subset of tags), active check at edge of start_at + end_at, edit guards (no start_at edit when active, end_at extend only) |
| **D3** | Host detail: `GPUSection` (per-GPU card with util / mem / temp progress bars + 1h time-series chart) + `ProcessesTable` (sortable); Rules form: 3 new metric types `gpu_util` / `gpu_temp` / `gpu_mem_pct` in the metric selector; Alerts → Maintenance tab (active/upcoming/past list with state badges + create form using datetime-local); Settings → Runtime: processes toggle + N + sort_by | tsc + pnpm web build clean; rules form accepts new metric types; alerts form lists maintenance windows |
| **D4** | `docs/configure/gpu.md` (NVIDIA driver + ROCm install pointers + Docker container caveat) + `docs/configure/processes.md` (what's collected + what's NOT + cmdline-leaks-secrets trade-off) + `docs/configure/maintenance-windows.md` (when to use + tag scope semantics + timezone behaviour); `CHANGELOG.md` v0.7.3 entry; flip Sprint 3 row → ✅; commit → push → tag v0.7.3 | docs render OK; CHANGELOG format; sprint queue updated |

**Decisions to log when shipping** (chốt giữa đường, không block start):
- RFC Q1 — suppress notification for events that ALREADY fired before window started: no. Window prevents *new* firings. The notification queue ships anything already queued (matches the existing host-silence pattern).
- RFC Q2 — redact `Cmd` if it matches `processes.redact_regex`: yes, implement as a default `(?i)(password|secret|token|api[_-]?key)\s*=` regex the operator can override via settings. Defensive default protects against accidental `password=...` exposure even when the operator opts in.

**Out of scope (đã chốt trong RFC, không vào sprint này)**: per-process GPU usage, Intel iGPU, Apple Silicon GPU, vendor-specific power/fan knobs, process tree, per-process net/disk I/O, send/kill from UI, recurring maintenance windows, auto-ack of pre-window events, calendar import, per-rule maintenance windows, per-GPU alert tag override.

**Trạng thái sprint queue sau D4**: Sprint 3 ✅ → next pull = Sprint 4 Notification quality (RFC 0004).

---

**Session**: 2026-06-08
**Đã làm**: **Sprint 2 — SAML2 SSO (RFC 0002).** Pulled after Sprint 1 (Backup) shipped v0.7.1 on 2026-06-08. SAML unlocks the older enterprise/EDU IdP estate where OIDC is absent or behind a higher tier. v1 ships the simplest interoperable subset: SP-initiated auth-code, signed assertions, no encryption, no SLO, single-admin gate via comma-separated `expected_nameid`. Reference: `internal/hub/auth/oidc.go` for the cookie/state pattern + `internal/hub/auth/crypto.go` for the AES-GCM KEK (distinct label `lumen/saml/v1`).

**Plan 4 ngày (locked, verify từng bước trước khi step tiếp)**:

| Day | Output | Verify |
|---|---|---|
| **D1** | `internal/hub/auth/saml.go` (SAMLConfig + LoadSAMLConfig + SaveSAMLConfig) + `saml_crypto.go` (AES-GCM with label `lumen/saml/v1` — distinct from OIDC's `lumen/oidc/v1`) + auto-gen 2048-bit RSA SP keypair on first save when `enabled=true` and no `sp_cert` row + self-sign cert (CN = SP entity ID, 10y validity) + migration `0021_saml_settings.sql` (seed 8 default rows, no new table) | `go test ./internal/hub/auth/...` still green; build all |
| **D2** | `saml_flow.go` (LoginRedirect, HandleACS, Metadata, TestMetadata) + `saml_handlers.go` (6 endpoints: `GET /api/auth/saml/{login,acs,metadata}` public, `GET/PUT /api/settings/saml` + `POST /api/settings/saml/test-metadata` session) + wire into `server.go` + extend `setup-status` to return `saml_enabled` alongside `oidc_enabled` + LoginForm renders 2 buttons when both enabled + `expected_nameid` accepts comma-separated list (intersect-any) per RFC Q4 + SP entity ID auto-default `https://<host>/api/auth/saml/metadata` per RFC Q2 + periodic IdP metadata refresh 1h when `saml.idp_metadata_url` set per RFC Q3 | `go build ./...` clean; setup-status endpoint smoke |
| **D3** | `saml_test.go`: parse Okta fixture, Azure AD fixture, ADFS fixture → extract SSO URL + cert + entity ID; validate known-good signed SAMLResponse (succeeds); altered byte (signature fails); NotOnOrAfter -1h (rejected); NotOnOrAfter -30s with 60s skew (accepted); NameID ≠ expected (rejected with clear error); ExpectedNameIDList with comma-separated values (intersect-any) | All tests pass; vet + lint clean |
| **D4** | `docs/configure/saml.md` (use-OIDC-if-you-can callout + compatibility matrix + recipes for Okta classic / Azure AD enterprise / ADFS / Shibboleth / samltest.id + troubleshooting clock-skew/signature/audience/NameID-format) + `CHANGELOG.md` v0.7.2 entry + flip Sprint 2 row → ✅ + commit → push → tag v0.7.2 | docs render OK; CHANGELOG format; sprint queue updated |

**Decisions to log when shipping** (chốt giữa đường, không block start):
- RFC Q1 — "test login" popup that round-trips against the configured IdP: defer to a follow-up; the metadata test endpoint already gives the operator a high-confidence check. Adding the full popup doubles the SAML test surface and we don't have UI hooks for a modal.
- RFC Q2 — SP entity ID auto-default vs require explicit: auto-default to `https://<host>/api/auth/saml/metadata` (the metadata URL itself) when empty. Documented in `saml.md`.
- RFC Q3 — periodic IdP metadata refresh: 1h default when `saml.idp_metadata_url` is set; off by default. Reuses the retention heartbeat's pattern.
- RFC Q4 — `expected_nameid` accepts multiple values: comma-separated, intersect-any (any one match accepts).

**Dependencies cần thêm vào go.mod**:
- `github.com/crewjam/saml` v0.4.x (pinned)
- (no other new dep — RSA via stdlib `crypto/rsa` + `crypto/x509`)

**Out of scope (đã chốt trong RFC, không vào sprint này)**: encrypted assertions, SLO, IdP role, attribute mapping → multi-user, IdP-initiated SSO, JIT provisioning, replay protection beyond crewjam defaults.

**Trạng thái sprint queue sau D4**: Sprint 2 ✅ → next pull = Sprint 3 Beszel bundle 1 (GPU + process top-N + maintenance windows, RFC 0003).

---

**Session**: 2026-06-04
**Đã làm**: **Sprint 1 — Backup + restore (RFC 0001).** Top of Phase 8 sprint queue. v0.7.0 đã ship 3 mục (OIDC SSO, public status page, web push); RFC 0001-0009 đã commit. Bắt đầu implement RFC 0001. Migration 0020 là số kế tiếp; package `internal/hub/backup/` chưa tồn tại nên tạo mới sạch; OIDC `internal/hub/auth/crypto.go` là reference pattern cho `s3_secret_key_enc`.

**Plan 5 ngày (locked, verify từng bước trước khi step tiếp)**:

| Day | Output | Verify |
|---|---|---|
| **D1** | `internal/hub/backup/` skeleton + `crypto.go` (Argon2id + AES-256-GCM seal/open + 39-byte header + magic) + migration `0020_backup_settings.sql` (seed defaults, no new table) | `crypto_test.go`: round-trip, wrong passphrase, tampered ciphertext, header magic mismatch |
| **D2** | `snapshot.go` (`VACUUM INTO` temp + gzip) + `target_local.go` (write/list/delete) + `target_s3.go` (aws-sdk-go-v2 BaseEndpoint + PathStyle) + `retention.go` | `snapshot_test.go` PRAGMA integrity; `retention_test.go` 20→5 deterministic mtimes |
| **D3** | `scheduler.go` (robfig/cron/v3, hot-reload on settings change, doubling backoff to 4h then alert per RFC Q4 proposed) + `handlers.go` (6 endpoints) + `restore.go` (download/decrypt/SIGHUP self-exec with `--restore=`) + `cmd/lumen-hub/main.go` parse `--restore` flag pre-startup | Integration test: hub + MinIO Docker containers; manual backup → object exists in bucket; pull + decrypt + integrity check passes; CLI restore on fresh hub |
| **D4** | `web/src/components/Settings.tsx` Backup tab: master switch, target select, conditional fields per target, passphrase + confirm, cron expression, retain N, Test target button, Backup now button, Recent backups list (size/age/target + Restore + Download per row). Inline EN strings (admin-only surface; same precedent as SSO tab) | tsc + web build clean; manual dev: enable → run-now → restore via UI |
| **D5** | `docs/configure/backup.md` (passphrase guidance + per-provider walkthroughs MinIO/R2/B2/AWS + cron cheatsheet + restore CLI canonical + UI caveat + file format spec per RFC Q5 proposed + `LUMEN_HUB_SECRET` rotation section); `CHANGELOG.md` v0.7.1 entry; flip Sprint 1 checkbox; commit → push → tag v0.7.1 | docs render OK; CHANGELOG follows Keep-a-Changelog; ACTION_PLAN.md table row Sprint 1 ☐ → ✅ |

**Decisions to log when shipping** (chốt giữa đường, không block start):
- RFC Q4 — backoff after consecutive failures: implement đề xuất RFC (doubling delay up to 4h, then surface as hub-level alert event).
- RFC Q5 — expose backup file format publicly: yes; format spec section in `docs/configure/backup.md` is authoritative.

**Dependencies cần thêm vào go.mod**:
- `github.com/aws/aws-sdk-go-v2` + `s3` + `credentials` (v1.x, pinned)
- `github.com/robfig/cron/v3`
- `golang.org/x/term` (CLI passphrase prompt; `golang.org/x/crypto/argon2` đã có sẵn từ auth.password)

**Out of scope (đã chốt trong RFC, không vào sprint này)**: WAL/PITR, format migration, web bundle/agent binaries backup, Docker volume backup ngoài `lumen.db`, hot-swap không restart.

**Trạng thái sprint queue sau D5**: Sprint 1 ✅ → next pull = Sprint 2 SAML2 (RFC 0002 đã có).

---

**Session**: 2026-06-02
**Đã làm**: **v0.6.3 — density toggle on Settings → Display.** Schema (`display.density: comfortable|compact`, server validator already accepting both) was reserved in v0.6.0; v0.6.3 ships the segmented control + the CSS hook that makes the page denser. Implementation: `html[data-density="compact"] { font-size: 15px }` in `index.css` base layer — Tailwind v4's rem-based spacing/padding/gap cascade through every utility class proportionally, no per-component overrides needed. `PrefsApply` already writes `data-density` onto `<html>` since v0.6.0; v0.6.3 just gives the attribute something to do. Auto-saves on change like other Display settings. Removed 4 orphan stubs in `host.customize*` namespace that pre-dated the builder. EN + VI i18n complete. tsc + web build clean.

**Session**: 2026-06-02 (earlier)
**Đã làm**: **v0.6.2 — saved views UI on the Dashboard customize popover.** Schema (`views[]`, `activeViewId`, server cap 5) was reserved in v0.6.0; v0.6.2 ships the UI. New section under sort + hide: list of saved views with bookmark + active badge + delete; "Save as new" form below. Apply view → writes view fields onto top-level dashboard prefs + sets activeViewId. Direct mutations to sort/hide clear activeViewId (auto-divergence). `defaultMetric` captured into views for forward compat. ID generated client-side via `crypto.randomUUID()` with fallback. Dropped three obsolete placeholder i18n keys (`customizeSavedViewsSoon`, `customizeStub`, `customizeViews`). EN + VI complete. Server validator unchanged (already validated this schema in v0.6.0).

**Session**: 2026-06-02 (earlier)
**Đã làm**: **v0.6.1 — per-core CPU + Containers join the builder grid.** v0.6.0 left both rendered outside the grid because their live-only lifecycles felt awkward; v0.6.1 promotes them to first-class catalog items with edit-mode × + drag/resize. `DEFAULT_LAYOUT_LG` now lists all 10 catalog IDs (per-core full-width below CPU/RAM; Containers full-width at the bottom). `availableIds` gates: per-core only when `live.cpu_per_core.length > 0` AND not virtualised; Containers only when `live.containers.length > 0`. Standalone "hidden on guest" notice retained above the grid so operators know why per-core isn't in the picker. ChartCard wraps both via new `editing`/`onRemove` props on `PerCoreChart` + `ContainersTable`; Containers body uses `overflow-auto` so a long list doesn't blow the card. Server validator already accepted both IDs since v0.6.0 (`internal/hub/userprefs/userprefs.go` `validChartIDs`). Saved layouts from v0.6.0 round-trip unchanged — no migration needed. tsc + lint + web build green.

**Session**: 2026-06-01
**Đã làm**: **Phase 7 v0.5.0 Public Read API — patch 1+2 shipped as v0.4.11.** Reordered Phase 7 to ship Public API before Cold tier (rationale logged above on Phase 7 section). Today's session output:
- v0.4.6–v0.4.10: Alerts UX redesign (inline Switch toggle, quick-templates, sectioned form); HostDetail per-core CPU auto-hide on virtualised guests (gopsutil `VirtualizationSystem`); SilencePanel "Until I lift" 1-year option + Hero pill; HostCard update badge + silence indicator; in-app ConfirmDialog replacing six `window.confirm()` callsites; TokenReveal Docker + Binary tabs (no-Docker install path) with hub-served + GitHub-raw fallback; ARMv7 dropped from CI/Dockerfile/Makefile for faster image builds; `/install.sh` 500 hotfix (literal `{{` in shell comment tripped Go template); Retention settings UX rewrite (self-explanatory labels, Interval dropdown restricted to s/m/h to enforce 24h cap); Hub Status panel (SQLite + WAL size, row counts on 6 tables, Go runtime counters, agent connected count, delivery queue depth — cached 15s, refreshed 30s).
- v0.4.11: `Settings → API Keys` admin UI + CRUD + `/api/v1/version` + `/api/v1/hosts` with Bearer auth + per-key in-memory token bucket (100/min) + public envelope. Migration 0016. Bilingual.
- v0.5.0 (this commit): full v1 read surface — `/api/v1/hosts/{name}`, `/api/v1/hosts/{name}/metrics?from=&to=&bucket=` (≤7d, bucket ≥30s, cap 1000 points), `/api/v1/alerts/events?state=&limit=`, `/api/v1/alerts/rules`. Host filter glob + scope checks honored on every endpoint. Skipped the 4-patch plan and tagged a real minor bump here — the v0.5.0 feature surface IS the Public Read API, and `/api/v1/*` is now functional end-to-end. v0.4.x stays reserved for polish only.

- v0.6.0 (this commit): **Personalization + Host detail builder.** RFC 0002 PR2 Level 3 personalization (`Settings → Display`, Dashboard customize popover, `GET/PUT /api/me/prefs`, migration 0017) **AND** RFC 0004 Mức 1 — Host detail dashboard builder unlocked: Edit-Layout mode with drag/resize, Add chart toggle-switch picker, Auto-arrange (greedy first-fit), Reset to defaults, × remove on each card, layout persisted to `dashboard_prefs.hostDetailLayouts[hostName]` (server caps: 50 hosts × 20 charts × valid catalog IDs only). Per-core CPU strip replaced by uPlot multi-line live ring buffer (10 min @ 5s, OKLCH golden-angle hues). Swap chart added next to Disk. Per-core CPU and Containers stay rendered outside the grid for v0.6.0 (different data lifecycles). Catalog locked at 10 IDs but only 8 historical charts participate in the builder for this release.

**Next**: v0.6.4+ candidates: host-grid virtualization (RFC 0002 N>50 cutover — defer until an operator complains). Saved-view polish backlog: rename a view, "update active view" button (capture current state into the active view), per-view tag filter once tag inventory hooks in. Density polish backlog: per-component compact overrides if any specific surface feels wrong after real use. Phase 8 items: Grafana spike, public status page, first-run onboarding. Cold tier (v0.7+) still deferred behind demand signal.

**Previous (2026-05-29)**: **Phase 6 SHIPPED end-to-end — release v0.4.0.** Milestones A–D delivered (rules/channels/engine + per-rule routing + Telegram + delivery queue with severity-aware retry + Deliveries tab). Tag inventory upgraded to first-class: migration 0012 + `tags`/`tag_values` tables, new **Alerts → Tags** tab (CRUD inventory on the left, host assignment on the right; Settings → Hosts is read-only for tags now), per-key dropdowns in the rule selector picker, cascade-delete with impact dry-run + confirm. Offline-rule double-clamp fixed (`for_seconds=0` now fires at ~60s, not 120s). DoD met: `cpu_pct>90% for 5m` → ntfy/Discord/Telegram → resolve, all visible in UI. Docs synced (CHANGELOG 0.4.0, ACTION_PLAN decisions log, RFC 0001 Milestone C, `configure/alerts.md`). Go test green, web `tsc --noEmit` clean.
**Phase 4 complete (2026-05-29):**
- Version awareness: fixed ldflags `main.Version` mismatch (agent `var Version` + hub `var Version`); new `GET /api/version` (`internal/hub/meta`, with test); host detail shows `agent <ver>` + update-available pill (click-to-copy update cmd); dashboard host card shows update badge; `"dev"` builds suppress badges.
- Update-agent panel on host detail: always-present card with the Compose update command, copy button, up-to-date/update-available status, and the "run on the agent's machine, not the hub" note.
- Verified end-to-end in Docker: hub `v0.2.0` + agent `v0.1.0` → badge shown → `docker compose up -d` (new image, same tag) → agent `v0.2.0` → badge clears, host+token preserved. Confirmed version is baked into the binary (ldflags), so `:latest` images still self-report the true release version (release.yml passes the git tag as VERSION build-arg).
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
- ~~Domain `quanla.org` / GitHub org `lumenhq` chưa register~~ — **Resolved 2026-06-02:** keeping `github.com/quanla93/lumen` (personal account, public OSS); no org transfer, no custom domain. Avoids batch-rename of go.mod + ~30 files. Re-evaluate only if a real org forms.
- ~~Discord/community URL chưa có~~ — **Resolved 2026-06-02:** dropped Discord placeholders from CONTRIBUTING / SUPPORT / CoC. Realtime chat is non-blocking; GitHub Discussions covers the immediate need. Add later if real demand surfaces.
- ~~Security / CoC contact emails~~ — **Resolved 2026-06-02:** SECURITY.md + CODE_OF_CONDUCT.md now route private reports through GitHub Security Advisories. No personal email exposed publicly.
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
