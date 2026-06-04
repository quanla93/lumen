# Changelog

All notable changes to Lumen will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- **Public status page** at `/status` (unauthenticated). Per-host opt-in via a new `public_visible` column on `hosts` (migration `0018`) + global enable/title/description in **Settings → Status page**. Shows up/stale/down state plus live CPU/RAM/disk for opted-in hosts, polled every 15s. Defaults are safe — nothing is published until the operator both flips the global toggle and ticks at least one host. Tags, host metadata, container telemetry, alerts, and charts are deliberately omitted to keep the public surface narrow. See [`docs/configure/public-status.md`](./docs/src/content/docs/configure/public-status.md).
  - `GET /api/public/status` always returns 200 with `{enabled: false, hosts: []}` when disabled so the frontend renders a deterministic "not published" notice instead of branching on HTTP status.
  - Handler sets `Cache-Control: no-store` to keep CDN edges from pinning stale snapshots.
  - Three new endpoints under the session-protected group: `GET/PUT /api/settings/public-status` for the global config, `PUT /api/hosts/{id}/public-visible` for per-host opt-in.
  - Frontend `/status` route short-circuits the auth bootstrap entirely — visitors never hit `/api/setup-status`.

## [0.7.0] - 2026-06-04

**Single sign-on via OIDC.** First v0.7 feature: bind a self-hosted IdP (Authentik, Keycloak, Google, Okta, Entra) so the admin signs in via OIDC instead of (or in addition to) the local password. Single-admin scope — exactly one email from the ID token's `email` claim is allowed, matched against `Settings → SSO → Expected admin email`; everyone else is rejected at the callback regardless of IdP authentication outcome. The local password keeps working as a fallback so OIDC misconfiguration can't lock the admin out.

### Added

- **`/api/auth/oidc/{login,callback}` endpoints** in `internal/hub/auth/handlers.go` driving a standard OAuth 2.0 authorization-code flow against the configured IdP. JWKS-based ID token verification via [`github.com/coreos/go-oidc/v3`](https://github.com/coreos/go-oidc) (new dep), with state + nonce carried across the redirect in a 5-minute AES-GCM-encrypted `lumen_oidc_state` cookie.
- **`/api/settings/oidc` GET/PUT + `/api/settings/oidc/test`** session-protected handlers in the auth package. UI never reads the plaintext client secret back over the wire — `has_client_secret: true` is the only signal that one exists. Empty `client_secret` on PUT means "leave saved value untouched".
- **AES-256-GCM at-rest encryption for the OIDC client secret** (`internal/hub/auth/crypto.go`). Key derived via `SHA-256("lumen/oidc/v1" || hub_secret)` so the same `LUMEN_HUB_SECRET` reused as a JWT signing key produces a different AEAD key. **Requires `LUMEN_HUB_SECRET` to be set to a stable hex value** — without it the saved client secret becomes unreadable after restart.
- **Settings → SSO tab** with enable toggle, issuer URL, client ID, client secret (write-only), scopes (defaults to `openid email profile`), expected admin email, and a "Test discovery" button that hits the issuer's `/.well-known/openid-configuration` before saving.
- **Login page** renders **Sign in with SSO** next to the password form when `setup-status` reports OIDC enabled. `?sso_error=` query params from the callback surface as inline error text on the login form (then cleaned from the URL so a refresh doesn't repeat).
- **`docs/configure/sso.md`** — single-admin scope notes, the canonical callback URL, end-to-end flow diagram, and per-provider recipes for Authentik / Keycloak / Google plus a troubleshooting section.

### Changed

- **`GET /api/setup-status`** now also returns `oidc_enabled`. The frontend uses it to conditionally render the SSO button without an extra round-trip.
- **JWT signing key reuse**: the same `LUMEN_HUB_SECRET` byte sequence now keys three things — session JWT HMAC, OIDC state cookie AEAD, OIDC client-secret-at-rest AEAD. Each derives a distinct key via a labelled hash, so cross-protocol replays are impossible, but operators with auto-rotated secrets need to be aware that rotation invalidates saved OIDC configs.

### Notes

- **Multi-user / read-only SSO** stays on the v1.0+ roadmap. Today's single-admin model is enough for homelab + small-team deployments; multi-user implies role columns + admin promote UI + role-aware OIDC group mapping, all of which is out of v0.7.0 scope.
- **SAML 2.0** is also deferred. The vast majority of self-hosted IdPs speak OIDC natively; SAML support would land as a separate connector if a user actually asks.

## [0.6.5.9] - 2026-06-04

**Use lxcfs MemTotal as the divisor when `/host-cgroup/memory.max` reads "max".** v0.6.5.8's startup diagnostic on the user's Docker-in-LXC agent showed `host_cgroup_mounted=true` and `hcv2_cur=688181248` (the LXC's real accumulated usage, 688 MB), but `hcv2_max=0`. Inside the LXC's own cgroup namespace, the namespace's root cgroup reports `memory.max=max` because the actual 4 GiB limit lives on the parent (Proxmox's `lxc.scope/<ctid>`) which the namespace hides. v0.6.5.8's `limit > 0 && limit < hostTotal` check then rejected the unlimited number and `Memory()` returned 0 — strictly worse than v0.6.5.7. lxcfs's `MemTotal` (which gopsutil reads as `v.Total`) IS that hidden limit, so it's the right denominator.

### Fixed

- **`computeCgroupRAMPct` in `internal/agent/collector/mem.go`** — pure formula `(current - file_cache) / limit`, with `limit` falling back to `hostTotal` when `memory.max` parses to 0 ("max") or exceeds `hostTotal` (cgroup v1 sentinel like `2^63-1`). New `TestComputeCgroupRAMPct` seeds the LXC reproducer (current=688 MB, limit=0, cache=500 MB, hostTotal=4 GiB) and asserts ≈4.59% — the number Proxmox shows.

### Notes

- **Existing v0.6.5.8 deployments**: `docker compose pull && docker compose up -d --force-recreate lumen-agent`. No compose changes — the `/host-cgroup` bind-mount from v0.6.5.8 is still correct.
- **Local smoke test** on Docker Desktop revealed an unrelated edge case: the macOS Docker VM's root cgroup has the v2 controllers listed but no `memory.current` at the namespace root (memory accounting only kicks in for child cgroups). That's a non-LXC shape, so `hostCgroupAvailable()` returns false and the code correctly falls through. Documented mentally only — not a code change.

## [0.6.5.8] - 2026-06-04

**Docker-in-LXC RAM% actually matches Proxmox now.** v0.6.5.7 trusted the bind-mounted `/proc/meminfo` but field verification showed lxcfs is cgroup-aware on the read path: when the agent reads `/proc/meminfo` from inside Docker, lxcfs serves numbers computed against the Docker container's *own* cgroup (a few MB used out of 4 GiB ≈ 0.18%) — not the LXC's view. The fix needs a different signal: read the LXC's cgroup files directly. New `/sys/fs/cgroup:/host-cgroup:ro` bind-mount in the agent compose exposes the LXC's own `memory.current` and `memory.max` to the agent regardless of which nested cgroup it's running in.

### Added

- **`hostCgroupRAMPct` + `hostCgroupAvailable` in `internal/agent/collector/mem.go`** — when `/host-cgroup/memory.{max,current}` (v2) or `/host-cgroup/memory/memory.{limit,usage}_in_bytes` (v1) exist, compute RAM% from those (cache-subtracted, matches Proxmox accounting). This is now the first branch in `Memory()`'s priority chain.
- **`/sys/fs/cgroup:/host-cgroup:ro` in `web/src/components/TokenReveal.tsx`** generated agent compose template, with explanatory comment on why each of the three mounts (`cpuinfo`, `meminfo`, `cgroup`) is there.
- **Startup `memory diagnostics` log** now also reports `host_cgroup_mounted` + the four cgroup numbers (v1+v2 max+current) seen at `/host-cgroup`, so an operator can confirm the bind-mount in one log line.

### Changed

- **`MemoryLimitStatus` warning** updated to name all three remediation paths (Docker-in-LXC → host-cgroup; native LXC → /proc/meminfo; bare-host Docker → mem_limit) instead of only the first.
- **`docs/install/agent-docker.md`** all four compose snippets, the `docker run` example, and the reference table updated to include the host-cgroup mount; troubleshooting section rewritten with the actual cause.

### Notes

- **Existing v0.6.5.x deployments**: edit `/opt/lumen-agent/docker-compose.yml` to add `- /sys/fs/cgroup:/host-cgroup:ro` under `volumes:`, then `docker compose pull && docker compose up -d --force-recreate lumen-agent`. The two existing `/proc/*` mounts can stay (no-op on Docker-in-LXC, still useful on native LXC and bare-host).

## [0.6.5.7] - 2026-06-03

**RAM% on Docker-in-LXC actually works now.** v0.6.5.4–v0.6.5.6 all assumed gopsutil's `mem.VirtualMemory()` honoured the bind-mounted `/proc/meminfo` (lxcfs view). The startup diagnostic added in v0.6.5.6 proved it does not: gopsutil v4 mixes `/sys/fs/cgroup/memory.current` into its Available calculation, which on Docker-in-LXC is the Docker container's own RSS (2.1 MB out of 4 GiB ≈ 0.05%) — not the LXC's view. The bind-mount file itself reads correctly: `(MemTotal - MemAvailable) / MemTotal = (4194304 - 3956832) / 4194304 ≈ 5.66%`, matching Proxmox. Skip gopsutil for the RAM path when the bind-mount is present and parse `/proc/meminfo` directly.

### Fixed

- **`internal/agent/collector/mem.go`** — when `procMeminfoIsBindMounted()` returns true, `Memory()` now reads `/proc/meminfo` directly via the new `procMeminfoRamPct` and computes `(MemTotal - MemAvailable) / MemTotal`. gopsutil's `VirtualMemory` is only consulted on bare-host setups (no bind-mount), where its cgroup blending is harmless. Test seeded with the actual lxcfs view captured from the reproducer asserts the formula returns ~5.66%, not ~0.05%.

### Notes

- **Existing v0.6.5.x deployments**: pull `ghcr.io/quanla93/lumen-{hub,agent}:0.6.5.7` (or `:latest`), `docker compose up -d --force-recreate lumen-agent`. No compose changes needed — the bind-mount from v0.6.5.2 is still the right shape.
- **Startup diagnostic stays** because Docker-in-LXC is the worst-served deployment shape we have and one more reproducer would otherwise cost another debugging round-trip. Will revisit at v0.7 once the path has stabilised.

## [0.6.5.6] - 2026-06-03

**Diagnostic: agent logs `meminfo_bindmount`, mountinfo length, first `/proc/meminfo` line, and both cgroup v1/v2 limit+current at startup.** Live verification on a Docker-in-LXC agent running v0.6.5.5 still showed RAM% ≈ 0.06%, even with the `/proc/meminfo` bind-mount confirmed present in both compose YAML and the kernel's `/proc/<pid>/mountinfo` (lxcfs FUSE entry, `f[4] == "/proc/meminfo"`). A unit test (`TestParseMountinfoForMeminfo`) confirms the parser handles the exact line the kernel produced — so the bug, if any, is not in parsing. This release adds a one-shot startup log line so the next reproducer surfaces which side is actually failing (`os.ReadFile` returning empty under nonroot? Cgroup numbers swapping out a correct bind-mount number? gopsutil reading the wrong file?). No behaviour change to RAM%; once the log identifies the path, the actual fix can be targeted.

### Added

- **`MemoryDiagnostics()` in `internal/agent/collector/mem.go`** — pure function that returns a one-line string with the bind-mount detection result, mountinfo byte count, first line of `/proc/meminfo`, and cgroup v1/v2 max+current numbers. Called once at agent startup from `cmd/lumen-agent/main.go`. Quiet on bare-host setups (cgroup numbers will be zero).
- **`parseMountinfoForMeminfo([]byte) bool`** — pure parser split out of `procMeminfoIsBindMounted()` for testability. Backed by `TestParseMountinfoForMeminfo` in `mem_test.go` that feeds the actual kernel line captured on the Docker-in-LXC reproducer.

## [0.6.5.5] - 2026-06-03

**Re-ships v0.6.5.4 after dropping arm64 from the container image build.** The v0.6.5.4 tag was pushed but the release workflow's `Push images to ghcr.io` job stalled for >1h on QEMU-emulated `linux/arm64` image build (twice — first run hit a ghcr.io 502 mid-push and the rerun never finished the hub image at all). Same v0.6.5.3 job had completed in 15 minutes the day before, so this is a runner/QEMU regression, not a code problem. No GitHub Release or ghcr image was ever published for v0.6.5.4; pull v0.6.5.5 instead.

### Changed

- **`.github/workflows/release.yml`** drops `linux/arm64` from the Docker image build matrix. amd64 images now build natively (no QEMU). arm64 operators continue to install via the binary tarball from the GitHub Release — the cross-compiled `lumen-agent-linux-arm64` / `lumen-hub-linux-arm64.tar.gz` artifacts ship unchanged, because the Go cross-compile path on the amd64 runner was never the bottleneck.

### Notes

- **No agent or hub behaviour change vs v0.6.5.4.** The mountinfo bind-mount detection fix described under v0.6.5.4 below is the actual fix shipping in this release.
- **Existing v0.6.5.x deployments**: pull `ghcr.io/quanla93/lumen-{hub,agent}:0.6.5.5` (or `:latest`), `docker compose up -d`. No compose changes needed.

## [0.6.5.4] - 2026-06-03

**Bind-mount detection now reads `mountinfo`, not `mounts`.** v0.6.5.3 added the bind-mount-wins-over-cgroup rule but the detection itself (`procMeminfoIsBindMounted`) checked `/proc/mounts` — which silently omits Docker's file-level bind mounts. Verified live on a Docker-in-LXC agent: `cat /proc/mounts | grep meminfo` returned empty, while `/proc/self/mountinfo` clearly showed the lxcfs FUSE entry. v0.6.5.3 thus changed nothing in practice for Docker-in-LXC; this release makes the detection actually fire.

### Fixed

- **`procMeminfoIsBindMounted` now scans `/proc/self/mountinfo`** with the mount point at field index 4. `mountinfo` is the only one of the two `/proc` files that lists per-file bind mounts; `/proc/mounts` only carries filesystem-level mounts. Behaviour for LXC-native (lxcfs FUSE mount on `/proc/meminfo`), Docker bind-mount, and bare-host (no bind) all distinguished correctly.

## [0.6.5.3] - 2026-06-03

**Bind-mount now wins over cgroup in nested setups.** Caught immediately after v0.6.5.2 shipped: with the `/proc/meminfo` bind-mount in place on a Docker-in-LXC agent, RAM% still read ~0.1% instead of the expected ~5%. Root cause: Docker inside LXC exposes a cgroup v1 compat view at `/sys/fs/cgroup/memory/memory.limit_in_bytes` that inherits the LXC's 4 GiB effective limit, which v0.6.5.1's cgroup-aware path picked up — and then read the Docker container's *own* `memory.usage_in_bytes` (~5 MB, just the agent process) as numerator, drowning the MemAvailable formula's correct number.

### Fixed

- **`internal/agent/collector/mem.go`** now skips the cgroup override entirely when `procMeminfoIsBindMounted()` returns true. Reasoning: when the operator bind-mounted `/proc/meminfo` from the host, gopsutil's view is already container-scoped (lxcfs); reading the Docker container's *own* cgroup would silently swap it for the agent process's footprint. The cgroup path stays in place for users who chose `mem_limit`-only (no bind-mount) — they're intentionally monitoring the container itself.

### Notes

- **Existing v0.6.5.2 deployments**: pull `:0.6.5.3` (or `:latest`), `docker compose up -d`, no compose changes. The compose template generated by v0.6.5.2 already has the right bind mounts.

## [0.6.5.2] - 2026-06-03

**Auto-correct RAM% for Docker-in-LXC.** Follow-up to v0.6.5.1. The cgroup path that release shipped still required operators to set `mem_limit:` on the Docker container — and to remember to bump it whenever they bumped the LXC's RAM. The cleaner answer is to bind-mount the host's `/proc/meminfo` (and `/proc/cpuinfo`) into the agent container so the agent sees the LXC's lxcfs-overlaid view automatically. No `mem_limit` to maintain, tracks LXC RAM changes for free, no-op on bare-host Docker.

### Changed

- **Generated `docker-compose.yml`** (`web/src/components/TokenReveal.tsx`) now bind-mounts `/proc/meminfo:/proc/meminfo:ro` and `/proc/cpuinfo:/proc/cpuinfo:ro` by default. The earlier commented-out `mem_limit:` hint from v0.6.5.1 is removed — the bind-mount is strictly better and needs no per-host tuning.
- **Local-test compose** (`deploy/docker/docker-compose.agent.example.yml`) gets the same two bind mounts.
- **`install/agent-docker.md`** updates all 3 compose variants (manual fallback, host-metrics-only, host+Docker), the `docker run` fallback, the compose-field reference table, and adds a Troubleshooting entry "RAM% shows the wrong number on Docker-in-LXC / Docker-in-VM".
- **`how-to/add-agents.md`** compose snippet adds the two bind mounts.

### Fixed

- **Startup warning no longer false-fires on a correct bind-mount setup.** `MemoryLimitStatus()` (`internal/agent/collector/mem.go`) now checks `/proc/mounts` for a `/proc/meminfo` bind-mount entry first; if present, the agent has a trustworthy view and the warning stays silent. Without the bind-mount AND without a real cgroup limit, the warning fires with both fix options listed (bind-mount preferred, `mem_limit` as alternative).
- **RAM% now matches Proxmox / `free -m` accounting.** Caught while validating the bind-mount fix above: gopsutil's `UsedPercent` counts `SReclaimable` as cache, which lxcfs reports large enough to drive the number to "near zero used" even while the LXC is actively serving traffic. `Memory()` now overrides the percent with `(Total - MemAvailable) / Total` when the kernel exposes `MemAvailable` (Linux 3.14+), matching what operators read off their hypervisor UI.

### Notes

- **Existing v0.6.5.1 deployments**: pull the new image (`ghcr.io/quanla93/lumen-agent:0.6.5.2` or `:latest`), edit `/opt/lumen-agent/docker-compose.yml` to add the two `/proc` bind mounts under `volumes:`, then `docker compose up -d`. Drop the `mem_limit:` line if you added one for v0.6.5.1 — it's redundant once the bind-mount is in place.
- **Bare-host Docker users**: the bind-mount is a no-op for you (your container's `/proc/meminfo` already matches the host's `/proc/meminfo`). Adding the lines is harmless.

## [0.6.5.1] - 2026-06-03

**Container-aware RAM accounting.** Reported by an operator whose Lumen UI showed 9router (Docker-in-LXC) stuck at RAM 60% while Proxmox said 7.5%. Root cause: gopsutil reads `/proc/meminfo`, and a Docker container does not inherit the LXC's lxcfs `/proc/meminfo` bind mount — so the agent saw the PVE host's 11.6 GiB total instead of the LXC's 4 GiB. The same shape bites any nested setup (Docker-in-VM where the agent should report VM RAM, k8s pods with limits, …).

### Fixed

- **`internal/agent/collector/mem.go` reads cgroup memory directly** when a real limit is configured. cgroup v2: `memory.max`, `memory.current`, `memory.stat:file`. cgroup v1: `memory.limit_in_bytes`, `memory.usage_in_bytes`, `memory.stat:cache`. "Used" subtracts page cache to match Proxmox-style accounting. When no real limit applies (limit == "max" or ≥ host total), falls back to gopsutil — bare-host behaviour unchanged.
- **Swap follows the same path** via `memory.swap.{max,current}` (v2) or `memsw - memory` (v1).

### Added

- **Startup warning** `MemoryLimitStatus()` in `mem.go`, logged once at `Warn` level from `cmd/lumen-agent/main.go` when the agent is inside a cgroup but no memory limit is set. Tells the operator their RAM% will reflect kernel host total, with the exact config keys to fix it (`mem_limit` for Docker, `lxc.cgroup2.memory.max` for LXC).
- **Compose template hint** in `web/src/components/TokenReveal.tsx`: the generated per-agent `docker-compose.yml` now includes a commented-out `mem_limit:` line with a one-line explanation, so operators see the knob exists when they paste the file.

### Notes

- **Pre-existing LXC users on Proxmox**: if your container's `/proc/meminfo` already shows the right `MemTotal` (lxcfs healthy), nothing changes — gopsutil keeps reading it. The cgroup path only kicks in when a real cgroup limit is present.
- **lxcfs FUSE death** (the `Transport endpoint is not connected` symptom that triggered this report) is a host-side issue fixed by `systemctl restart lxcfs && pct stop/start <vmid>`. The agent change does not paper over that.

## [0.6.5] - 2026-06-03

**Backlog-drain hosts now read online.** Reported by an operator running a hub redeploy with the old data dir preserved: after the hub came back up, agents reconnected and `/api/ingest` returned 200 for every push, but the Dashboard kept the host cards in the yellow "stale" state for hours. Data IS flowing — the UI just couldn't tell. Root cause: the FE keyed its online/stale indicator off `snapshot.ts` (agent collection time), and a reconnected agent shipping a backlog from its bbolt buffer sends frames whose `ts` is the original (sometimes 24h-old) collection moment, even though the hub received them seconds ago.

### Fixed

- **`HostSnapshot` carries a new server-stamped `received_at` field** (`internal/shared/api/types.go`). The ingest handler (`internal/hub/ingest/handler.go`) stamps `time.Now().UTC()` once per request, before the snapshot reaches the in-memory store, the batcher, or the WS fanout. This separates the two semantics that were tangled in one field: `ts` (collection time, drives chart x-axes and "last sample N ago" labels) vs `received_at` (hub-side liveness, drives online / stale).
- **FE liveness now reads `received_at`** in `HostCard`, `Dashboard` (summary card counts + sort-by-last-seen), and `HostDetail` (header status + "last seen" label). `ts` keeps its real job: data-point timeline.

### Notes

- **No DB migration.** `received_at` is an in-memory / wire-only liveness signal. Hub binary embeds the web bundle, so old-hub-new-web (or vice versa) is not a deploy state to plan for.
- **What operators see after upgrading**: an agent draining a 24h backlog shows ONLINE immediately on reconnect, with the "last sample 23h ago" label staying accurate while the backlog catches up. Sort-by-last-seen on the Dashboard now orders by hub-reachability, not by stale collection time.
- Buffer cap behaviour unchanged (`defaultMaxAge = 24h` + `defaultMaxRows = 17,280` in `internal/agent/buffer/buffer.go`) — agents still won't grow the local buffer file unboundedly during a long hub outage.

## [0.6.4] - 2026-06-02

**arm64 back in the release pipeline.** The 2026-06-02 OSS-flip smoke test caught a real adoption blocker — `ghcr.io/quanla93/lumen-hub:latest` and `lumen-agent:latest` only carried amd64 manifests, even though the README + CI matrix + `internal/hub/install/install.go` allowedBinaries all assumed arm64 worked. The original removal (release.yml comment block, pre-OSS-flip) made sense when the repo was private and the operator fleet was 100% x86; once the repo is public the dropped arch hits anyone on Apple Silicon dev, Raspberry Pi 4/5, Ampere, Graviton, RK3566 NAS — exactly the homelab population the README claims as audience.

### Fixed

- `release.yml` now builds **both linux/amd64 and linux/arm64** for the hub image, agent image, hub tarball, and standalone agent binary. The multi-arch image is produced via `docker/setup-qemu-action` + a single `docker/build-push-action` call with `platforms: linux/amd64,linux/arm64`. amd64 builds native on `ubuntu-latest`; arm64 builds under QEMU emulation. Total release time goes from ~3–5 min back to ~10–15 min — acceptable for OSS adoption gain.
- Hub-side `allowedBinaries` (`internal/hub/install/install.go`) already accepted `lumen-agent-linux-arm64`, so the `/install/<arch>` endpoint will start serving arm64 binaries the moment the next release ships.
- Release notes generation now labels the container images as **multi-arch**.

### Notes

- **No code change.** Hub binary, agent binary, and Docker images are bit-for-bit identical to v0.6.3 for amd64 — only the release pipeline changes.
- **armv7 stays dropped** (CI matrix only ships amd64 + arm64 per CHANGELOG v0.4.7). RPi 2/3 32-bit users can still build from source; bring armv7 back if anyone files an issue.
- Existing v0.6.3 images / tarballs are unchanged. Pull `lumen-hub:0.6.4` (or `:latest` once tagged) for the first multi-arch artifacts.

## [0.6.3] - 2026-06-02

**Density toggle on `Settings → Display`.** RFC 0002 reserved this in v0.6.0 (`display.density: comfortable | compact`, server validator already accepting both) — v0.6.3 ships the toggle plus the global CSS hook that actually makes the page denser.

### Added

- **`Settings → Display → Density` segmented control.** Two values: Comfortable (default) and Compact. Auto-saves on change like the other Display settings.
- **`html[data-density="compact"]` CSS rule** in `web/src/index.css` (base layer): drops the root font-size from 16 px to 15 px. Tailwind v4's spacing/padding/gap utilities are rem-based, so a single root-size flip cascades through every padding-, gap-, and text-* class proportionally. Explicit `text-xs` / `text-sm` typography also shrinks by the same ratio. No component-specific overrides needed.
- **i18n keys (EN + VI):** `displayDensityLabel`, `displayDensityComfortable`, `displayDensityCompact`, `displayDensityHelp`. Removed four orphan stub strings on the `host.customize*` namespace (`customizeStub`, `customizeShowHide`, `customizeDefaultRange`, `customizeCompact`) — the Host detail "customize coming soon" hint they backed has been live as the builder for two releases now.

### Notes

- 16 → 15 px is a ~6.25% shrink — noticeable but readable. Picked deliberately over 14 px (12.5%) which felt cramped during a quick eyeball test of the host card hero stat and the metric tables.
- `PrefsApply` already writes `data-density` onto `<html>` from display prefs (since v0.6.0); v0.6.3 just gives the attribute something to do.
- Per-component compact rules (e.g. tighter host card hero, smaller per-core legend) are deliberately not in this release — the root-font flip already covers the high-leverage spacing. Add targeted rules in a follow-up only if specific components feel wrong after real use.

## [0.6.2] - 2026-06-02

**Saved views UI for the Dashboard customize popover.** The schema was reserved in v0.6.0 (`dashboard_prefs.views[]` + `activeViewId`, max 5 per server validator) — v0.6.2 adds the UI to actually use them. Operators can now bundle their current sort + hidden host list as a named view, switch between views with one click, and delete views they no longer want.

### Added

- **`Dashboard → Customize → Saved views` section.** Below the existing sort + hide controls. Lists every saved view with a bookmark icon, name, and an active badge on the one most recently applied. Click the view body to apply (loads sort + sortDir + defaultMetric + hidden hosts in one write). × to delete.
- **`Save as new` form.** Name input (1–32 chars, server-validated) + button. Saving captures the current dashboard state into a new view, sets it active, then clears the input. Disabled when at 5/5 view cap (server hard limit).
- **Active-view auto-divergence.** Any direct mutation to sort/sortDir/hide while a view is active clears `activeViewId` — the bookmark highlight reflects whether the dashboard still matches its saved state. Re-clicking the view re-applies.
- **i18n keys (EN + VI):** `savedViews`, `savedViewsEmpty`, `savedViewNamePlaceholder`, `savedViewSave`, `savedViewSaveAria`, `savedViewApplyAria`, `savedViewDeleteAria`, `savedViewActive`, `savedViewCapHint`. Removed unused placeholder strings (`customizeSavedViewsSoon`, `customizeStub`, `customizeViews`) — the section it gated is now real.

### Notes

- ID generation uses `crypto.randomUUID()` with a `Date.now()/Math.random()` fallback for any browser that lacks the API. Server doesn't care about the ID format — only uniqueness within the user's `views[]` (validator already rejects duplicates).
- `defaultMetric` is captured into saved views even though the customize popover doesn't expose it yet — the schema-validated field rides along so a future "default metric on host cards" toggle can use it without breaking saved views from this release.

## [0.6.1] - 2026-06-02

**Per-core CPU + Containers join the Host detail builder grid.** v0.6.0 shipped the dashboard builder but kept per-core CPU and the Containers table rendered outside the grid (above and below) because their live-only data lifecycles were felt to be a poor fit. With the catalog and persistence layer settled, both now slot into the grid like the historical charts — operators can hide, place, and resize them per host.

### Changed

- **`cpu-per-core` and `containers` are now first-class grid items.** Both appear in the Edit-Layout Add-chart picker, support the × remove button, drag-by-header, and resize-by-handle. Layouts persist into `dashboard_prefs.hostDetailLayouts[hostName]` like any other chart. Default layout now places per-core full-width below CPU/RAM and Containers full-width at the bottom; the catalog availability gate hides per-core on virtualised guests and hides Containers on hosts with no Docker workload.
- **Containers card now scrolls within the grid cell.** Header is the standard chart-card strip (icon, count, running badge); the table body uses `overflow-auto` so a long container list doesn't blow the card height. Column padding tightened from `px-4` to `px-2` to match the standard chart card body inset.
- **Per-core CPU card fills its grid cell.** Previous fixed `h-[200px]` swapped for `h-full` so resizing it taller via the grid handle actually shows more of the lines.

### Notes

- The standalone "Per-core CPU hidden on guest" notice still surfaces above the grid when the agent reports a `virt_type`, so operators understand why the chart is missing from the picker rather than seeing a silent gap.
- Saved layouts from v0.6.0 round-trip unchanged — neither chart was reserved a position before, so existing layouts simply gain the option without losing anything. Users who want the new defaults can hit Reset.

## [0.6.0] - 2026-06-01

**Level 3 personalization + Host detail dashboard builder.** RFC 0002 PR2 ([`docs/rfcs/0002-ui-polish-and-personalization.md`](docs/rfcs/0002-ui-polish-and-personalization.md)) moves per-user prefs off `localStorage` onto the hub. RFC 0004 ([`docs/rfcs/0004-host-detail-builder.md`](docs/rfcs/0004-host-detail-builder.md)) turns the per-host detail page into a drag/resize/add/remove grid — operators can shape every host's view to the metrics they actually care about, persistent per-user.

### Added — personalization

- **`Settings → Display` tab**: theme (System / Light / Dark), language (EN / VI), units (Auto / Binary / Decimal), reduce-motion (System / On / Off). Auto-saved on each change. Per-user, server-stored — synced across browsers.
- **Dashboard customize popover**: sort by Name / Hottest / Last seen with asc/desc direction, hide hosts from a dropdown + restore from the hidden list. Saved views are schema-reserved but UI lands in a follow-up patch.
- **`GET/PUT /api/me/prefs`** + **`/api/me/prefs/{dashboard,display}`**: session-protected per-user key/value JSON blobs. Server validates shape (enum guards, schemaVersion=1, view count cap = 5, name length 1..32). Migration 0017 with composite PK `(user_id, key)`.

### Added — Host detail dashboard builder (RFC 0004)

- **Edit Layout mode** on Host detail: click toolbar button to enter, drag any chart by its header, resize from the bottom-right corner, remove with the × on each card. Click Done to exit.
- **Add chart picker**: toggle-switch list of all available charts (cpu, ram, swap, disk, disk-io, network, load, temperature). Click any switch to show/hide that chart.
- **Auto-arrange**: one-click compaction that re-flows the current arrangement leftward + upward (greedy first-fit) so gaps from drag-around vanish.
- **Reset to defaults**: drops the saved layout for this host and falls back to the catalog defaults.
- **Smart placement**: removing a chart auto-heals the gap; adding a chart lands it in the first empty slot.
- **Per-user, per-host persistence**: layouts saved into `dashboard_prefs.hostDetailLayouts[hostName]`. Server caps the blob at 50 hosts × 20 charts/host and validates every chart ID against the catalog whitelist.
- **Per-core CPU as live chart**: replaced the per-core tile strip with a uPlot multi-line live ring buffer (last 10 min, sampled at 5s). OKLCH golden-angle hue rotation so cores stay visually distinct without one looking "hotter" by colour. Legend hides above 8 cores; hover tooltip lists each.
- **Swap chart**: dedicated time-series for `swap_pct` next to Disk on the secondary row. Already in the historical schema, just wasn't surfaced.

### Changed

- Top-bar `ThemeToggle` / `LanguageToggle` now write through `usePrefs` instead of `localStorage`. First load after upgrade auto-seeds display prefs from any pre-existing `lumen.theme` / `lumen.locale` so users don't lose their pick.
- `PrefsApply` bridges `display.theme` to the `<html>` `.dark` class and live-tracks the OS color-scheme media query when `theme: 'system'`. `display.reduceMotion` sets `data-reduce-motion` on `<html>` so future stylesheet rules can opt in independently of the OS media query.

### Notes

- The "NO drag-drop grid" stance from earlier ranges was lifted only for **Host detail**. Dashboard (host grid) stays a fixed view — personalization there is sort + hide, not free placement.
- Per-core CPU and Containers panels render outside the builder grid for v0.6.0 (live ring buffer + live table — different data lifecycles). Catalog already lists them so a future patch can pull them in.
- Saved views deferred to v0.6.x (schema reserved, UI is the only missing piece).
- Density toggle (`comfortable` / `compact`) reserved in the schema but UI is post-v0.6.
- Host grid virtualization (RFC 0002 N>50 cutover) deferred — homelab fleets are small enough that `@tanstack/react-virtual` is wasted complexity for now. Ship when an operator complains.

## [0.5.0] - 2026-06-01

**Phase 7 ships its first slice — the Public Read API is live.** Mint a bearer key in Settings → API Keys, point Grafana / n8n / scripts at `/api/v1/*`, integrate without touching the admin session. v0.4.11 introduced the API Keys + first two endpoints; this release completes the read surface.

### Added (v0.5.0)

- **`GET /api/v1/hosts/{name}`** — host detail (name, last_seen_at, created_at). Requires `read:hosts`. Host filter glob from the key is enforced; 404 if the host is unknown or excluded (same response either way, so a key can't probe for hosts outside its filter).
- **`GET /api/v1/hosts/{name}/metrics?from=&to=&bucket=`** — downsampled time-series. `from` / `to` are RFC3339 timestamps; `bucket` is a Go duration (`30s`, `1m`, `5m`, …). Caps: range ≤ 7 days, bucket ≥ 30s, (to-from)/bucket ≤ 1000 points. Requires `read:metrics`. Bucket is mandatory — there's no "raw 5s" path on the public API; that's reserved for the UI's WebSocket stream.
- **`GET /api/v1/alerts/events?state=&limit=`** — alert event history. `state` = `firing` / `resolved` / `all` (default `all`); `limit` 1–500 (default 100). Host filter glob enforced post-query (over-fetch + filter; fine at homelab scale). Requires `read:alerts`.
- **`GET /api/v1/alerts/rules`** — read-only rule inventory (id, name, metric, comparator, threshold, severity, host_selector, enabled). Channel routing is NOT exposed — that stays operator-internal. Requires `read:alerts`.

### Carried in from v0.4.11

- `Settings → API Keys` admin UI + `/api/apikeys` CRUD: mint / list / revoke, plaintext-shown-once flow, glob host_filter, scopes (`read:hosts`, `read:metrics`, `read:alerts`).
- `/api/v1/version` and `/api/v1/hosts` with Bearer auth + per-key in-memory token bucket (100/min) + public envelope `{success, data, error, request_id}` + `X-RateLimit-*` headers.
- Migration 0016: `api_keys` table, SHA-256 hex hash, unique index on hash for the verify hot path.

### Documentation

- **[RFC 0003 — Public Read API](docs/rfcs/0003-public-api.md)**: design record covering the auth model, scope choice, envelope shape, rate-limit decisions, host filter probing protection, and the deferred-feature pickup order.
- **[Public Read API reference](docs/src/content/docs/reference/public-api.md)**: endpoint catalog with curl examples, error code table, rate-limit headers, Grafana JSON datasource recipe, shell-script + n8n integration patterns, stability promise.

### Notes

- Phase 7 reorder logged in ACTION_PLAN: Public API ships ahead of Cold tier because (a) it's a lower-risk expose layer over data we already have, (b) homelab fleets bounded by the v0.4.1 retention sweep don't need Cold tier yet. Cold tier becomes v0.6.0 if real demand for >7d queries surfaces via the new metrics endpoint.
- Smoke test once shipped:
  ```bash
  curl -H "Authorization: Bearer lumk_..." \
    "http://hub:8090/api/v1/hosts/lumen-hub/metrics?from=2026-06-01T00:00:00Z&to=2026-06-01T06:00:00Z&bucket=5m"
  ```

## [0.4.11] - 2026-06-01

Public Read API foundation (Phase 7 / v0.5.0, patch 1+2 of 4). API keys mint, list, revoke + the first two `/api/v1/*` endpoints with Bearer auth + rate limit.

### Added

- **`Settings → API Keys` tab + `/api/apikeys` admin endpoints.** Operator mints bearer keys (`lumk_<32 bytes base64url>`), picks scopes (`read:hosts`, `read:metrics`, `read:alerts`), and optionally restricts a key to a glob host filter (e.g. `*pve*`). Keys are stored as SHA-256 hex hash — plaintext is shown exactly once on create with a copy button, then never persisted. List view shows preview (`lumk_AbCdEfGh…`), scopes, last-used, created-at. Revoke uses the in-app confirm dialog. Migration 0016 with unique index on `hash` for the verify hot path.
- **`GET /api/v1/version`** — public ping. Auth required (any valid key) but no scope check.
- **`GET /api/v1/hosts`** — host list filtered by the key's host_filter glob. Requires `read:hosts` scope.
- **Public API envelope.** Every `/api/v1/*` response wraps data in `{success, data, error: {code, message}, request_id}`. Internal `/api/*` keeps its terse shape; the two surfaces are now cleanly separated.
- **Per-key rate limit.** In-memory token bucket — 100 burst / 100 per minute refill. `X-RateLimit-Limit` + `X-RateLimit-Remaining` headers on every response; 429 + `Retry-After` when exhausted. No Redis — single-binary discipline.

### Notes

- Metrics (`/api/v1/hosts/{name}/metrics`) and alerts (`/api/v1/alerts/*`) endpoints land in v0.4.12. RFC + reference docs in v0.4.13. v0.5.0 cuts the sum.
- Quick smoke test once shipped:
  ```bash
  # Mint a key in Settings → API Keys, then:
  curl -H "Authorization: Bearer lumk_..." http://hub:8090/api/v1/version
  curl -H "Authorization: Bearer lumk_..." http://hub:8090/api/v1/hosts
  ```

## [0.4.10] - 2026-06-01

Hub self-visibility — a new **Settings → Hub status** tab.

### Added

- **`GET /api/admin/hub-stats` + Settings → Hub status panel.** Operator-only health snapshot that shows what only the hub can see: SQLite file + WAL size, per-table row counts (`snapshots`, `alert_events`, `notification_deliveries`, `hosts`, `alert_rules`, `notification_channels`), Go runtime counters (goroutines, heap, GC cycles), connected/registered agent count, and the notification queue depth (pending / inflight). Response cached 15s server-side; UI auto-refreshes every 30s. To monitor the hub host's CPU/RAM/disk, install the agent on it like any other host — this panel covers the gap an agent can't see. Bilingual (EN + VI).

## [0.4.9] - 2026-06-01

Retention settings UX polish.

### Fixed

- **Settings → Retention no longer rejects valid-looking input.** The "Interval" dropdown offered "days" even though the backend caps the sweep heartbeat at 24h, so picking "10 days" failed validation with a cryptic `retention_interval: must be <= 24h0m0s`. The unit dropdown for Interval is now restricted to minutes/hours only — invalid values can't be picked in the first place.

### Changed

- **Retention field labels rewritten to self-explain.** "Window" / "Interval" / "Alert history window" became "Keep raw snapshots for" / "Cleanup runs every" / "Keep alert history for". Each field carries its own help text directly underneath, instead of a mixed paragraph at the top — you don't have to map field-to-sentence anymore. Bilingual (EN + VI).

## [0.4.8] - 2026-06-01

Hotfix for v0.4.7 — `/install.sh` returned 500 on the hub.

### Fixed

- **`/install.sh` endpoint serves the script again.** A code comment in `scripts/install-agent.sh` contained a stray literal `{{` (talking *about* the Go template delimiter); `text/template.ParseFiles` doesn't care that it's inside a `#`-prefixed shell comment and tripped with `unterminated raw quoted string`. Every `curl http://<hub>/install.sh` came back 500. Comment rewritten without the literal delimiter; renders fine now.

### Added (also confirms in-app dialog from this round)

- **In-app confirm dialog replaces `window.confirm()` across six callsites.** Rotate token, delete host (Settings); delete rule, delete channel (Alerts); delete tag, delete value (Tags) — all now show a Radix `AlertDialog` styled to match the rest of the UI, with per-flow Title + body + destructive-red confirm button. New `useConfirm()` hook in `components/ConfirmDialog.tsx` so future destructive flows can swap in with one line.

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
