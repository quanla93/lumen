---
title: CI / CD
description: What the GitHub Actions workflows do, how to tag a release, and how to debug a failing job.
sidebar:
  order: 5
---

Lumen ships three workflows under `.github/workflows/`:

| Workflow | Trigger | What it does |
|---|---|---|
| **CI** | push to `main`, every PR | Lint + typecheck + cross-build + Docker image smoke |
| **Release** | annotated tag `v*` | Cross-build binaries, push multi-arch ghcr.io images, create GitHub Release |
| **CodeQL** | weekly + every PR | Security scan (Go + JS/TS) |

## CI (`.github/workflows/ci.yml`)

Six jobs run in parallel on every PR. None of them depend on each
other — a failure in `lint-web` won't block `test-go` from running.

| Job | What it runs | Failing because… |
|---|---|---|
| `lint-go` | `golangci-lint run ./...` | A `go vet` violation, unused imports, or a custom rule. Reproduce locally with `make lint-go`. |
| `lint-web` | `pnpm install && pnpm run lint && pnpm run typecheck` | `tsc --noEmit` rejected a web file, or Biome flagged a docs file. Reproduce with `pnpm run lint`. |
| `test-go` | `go test -race ./...` | A unit test failed (or a race was detected). Reproduce with `make test`. |
| `build` (matrix) | `make build-linux-{amd64,arm64,armv7}` + binary-size budget | Build error, web bundle compile error, or a binary exceeded 30 MB / 20 MB. The 7-day-retention artifact is what's stale. |
| `docker` (matrix) | `docker buildx build` for hub + agent | Dockerfile invalid, a `COPY` missed a file, distroless complained about a non-static binary. |
| `benchmark` | `go test -bench=.` on `./internal/hub/...` | Only runs if benchmarks exist (probed with grep). Skipped silently otherwise. |

The matrix targets (`linux-amd64`, `linux-arm64`, `linux-armv7`) are
the same set the install script + release tarballs target — so a
green CI means the artifacts ship on those archs.

## Release (`.github/workflows/release.yml`)

Triggered by pushing a tag that matches `v*`:

```bash
git tag -a v0.1.0 -m "v0.1.0: MVP release"
git push origin v0.1.0
```

Three jobs run in order:

1. **`binaries`** — cross-builds hub + agent for amd64 / arm64 / armv7,
   then runs `make release-hub-tarballs` to bundle each hub binary
   with `install-hub.sh`, the systemd unit, and `hub.env.example`.
   Uploads to a workflow artifact.

2. **`images`** — builds multi-arch Docker images via Buildx + QEMU
   and pushes to `ghcr.io/<owner>/lumen-{hub,agent}` with both
   `:VERSION` and `:latest` tags. Hub is amd64+arm64; agent is
   amd64+arm64+armv7 (matches the binary build set).

3. **`release`** — downloads the artifact from step 1, generates
   release notes from `git log` since the previous `v*` tag, and
   creates a GitHub Release with the tarballs + agent binaries
   attached. Pre-release flag toggles automatically on tag suffixes
   like `-rc1`, `-alpha`, `-beta`.

### Promote a pre-release

```bash
# RC first
git tag -a v0.1.0-rc1 -m "v0.1.0 RC1"
git push origin v0.1.0-rc1
# … fix issues, then …
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

The pre-release flag is just metadata on the GitHub Release — image
tags and tarballs are produced identically. `:latest` always points
at the most recent tag pushed, whether release or pre-release.

### Skipping a release

There's no built-in skip — if you push a tag, the workflow runs. If
you need to delete a botched release:

```bash
# delete the release page (via gh CLI)
gh release delete v0.1.0 --yes
# delete the tag locally + remotely
git tag -d v0.1.0
git push origin :refs/tags/v0.1.0
# delete the ghcr.io image (requires owner perms)
gh api -X DELETE /user/packages/container/lumen-hub/versions/<id>
```

## CodeQL

Standard CodeQL setup matrix on Go and JavaScript/TypeScript. Runs
weekly + on every PR. Findings show up under the repo's **Security
→ Code scanning** tab.

## Required secrets

None for CI. Release uses `secrets.GITHUB_TOKEN` (auto-provided by
Actions, no manual setup needed) to push to ghcr.io and create the
Release. The token has read+write on `contents` and `packages`
scoped to the repo — that's already what `release.yml` declares in
its `permissions:` block.

## Reproducing CI locally

```bash
# 1. Lint Go
make lint-go

# 2. Lint web + docs  (matches the lint-web CI job exactly)
pnpm install --frozen-lockfile
pnpm run lint
pnpm run typecheck

# 3. Tests
make test

# 4. Cross-build a single target
make build-linux-amd64
ls -la dist/

# 5. Docker images
docker build -f deploy/docker/Dockerfile.hub -t lumen-hub:ci .
docker build -f deploy/docker/Dockerfile.agent -t lumen-agent:ci .
```

If all five pass locally, your PR will pass CI. If CI fails and your
local doesn't, the cause is almost always one of:

- **Lockfile drift** — `pnpm install --frozen-lockfile` refuses to
  update `pnpm-lock.yaml`. Run `pnpm install` to refresh, commit
  the lockfile.
- **Go module drift** — `go mod tidy` produces a diff. Commit
  `go.mod` + `go.sum`.
- **Generated docs/dist** — Biome was scanning a build output. Add
  the dir to `docs/biome.json` `files.ignore`.
