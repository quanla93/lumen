# Contributing to Lumen

Thanks for your interest! Lumen is community-maintained open source.
This document tells you the fastest way to be useful.

> 🇻🇳 Tiếng Việt: xem [docs/src/content/docs/vi/contributing/](docs/src/content/docs/vi/contributing/)

## Table of contents

- [Ways to contribute](#ways-to-contribute)
- [Before you start](#before-you-start)
- [Code contribution workflow](#code-contribution-workflow)
- [Merge criteria](#merge-criteria)
- [Development setup](#development-setup)
- [Style guides](#style-guides)
- [Contributor tiers](#contributor-tiers)
- [Where to ask questions](#where-to-ask-questions)

---

## Ways to contribute

You don't need to write code to help:

| Type | How |
|---|---|
| 🐛 Bug reports | Open an [issue](https://github.com/lumenhq/lumen/issues/new/choose) — use the template |
| 💡 Feature ideas | Start a [Discussion](https://github.com/lumenhq/lumen/discussions/categories/ideas) |
| 📖 Docs | PR directly into `docs/` — small fixes don't need an issue first |
| 🌍 Translations | See [translating.md](docs/src/content/docs/contributing/translating.md) |
| 💬 Help others | Answer Discussions / Discord questions |
| 🧪 Test pre-releases | Try `next` tag, report regressions |
| 🛠️ Code | Read the rest of this document |
| ⭐ Star + share | Genuinely helps |

---

## Before you start

**Discuss large changes first.** Anything beyond a small fix should have an issue or RFC accepted before you write code. This protects your time — we'd rather say "let's adjust the approach" before the PR than reject finished work.

- **Small (< 100 lines, no new dep, no behavior change)** — PR straight away, no issue needed.
- **Medium (new feature, new endpoint, schema change)** — open an issue, get a maintainer to add label `accepted`.
- **Large (architecture change, new module, new transport)** — write a short RFC in [`docs/rfcs/`](docs/rfcs/) and let it bake in Discussions for at least a week.

**Things we will not accept** (please save your time):

- New dependencies that significantly increase binary size or RAM. Lightweight is core value.
- Enterprise complexity: multi-tenancy, RBAC roles, SAML/SSO, federation.
- General-purpose dashboard builder (Grafana already exists).
- Full-text log search engines (Loki exists).
- AI/ML anomaly detection.
- Cloud-only features that require external services to function.

If unsure, **ask first** in [Discussions](https://github.com/lumenhq/lumen/discussions).

---

## Code contribution workflow

1. **Find or open an issue** describing what you want to do.
2. **Comment "I'd like to work on this"** so we can assign you and avoid duplicate work.
3. **Fork** the repo, create a branch:
   ```
   feat/proxmox-collector
   fix/agent-reconnect-loop
   docs/quickstart-typo
   ```
4. **Develop locally** — see [Development setup](#development-setup).
5. **Commit** using [Conventional Commits](https://www.conventionalcommits.org/):
   ```
   feat(agent): add UPS collector
   fix(hub): correct retention boundary off-by-one
   docs: clarify Proxmox token permissions
   ```
6. **Test** — `make test`, `make lint`, `make benchmark` (catches RAM regressions).
7. **Open PR** against `main` — fill out the template completely.
8. **Respond to review** — we aim for first review within **7 days**. Polite ping welcome after that.
9. **Merge** — maintainer squashes when green.

### Pull request checklist

Before requesting review, make sure:

- [ ] Linked to an issue (`Closes #123`) — unless trivial
- [ ] `make test` passes locally
- [ ] `make lint` clean
- [ ] No new dependency unless absolutely necessary (justified in PR description)
- [ ] Binary size delta < 5% (CI will warn)
- [ ] Idle RAM delta < 5% (CI benchmark)
- [ ] Docs updated for any user-facing change
- [ ] Changelog entry under `## Unreleased` in `CHANGELOG.md`

---

## Merge criteria

A PR is merged when:

1. CI is green (lint, tests, build all platforms, security scan).
2. At least one maintainer approves.
3. No unresolved review comments.
4. For changes touching a module with a [CODEOWNER](.github/CODEOWNERS), the owner approves.
5. Architectural changes require **two** maintainer approvals.

We **squash and merge** by default. PR title becomes the commit message — write it carefully.

---

## Development setup

### Prerequisites

- Go 1.22+
- Node.js 20+ and pnpm 9+ (for the web UI and docs)
- Docker (for integration tests)
- Make

### Clone and run

```bash
git clone https://github.com/lumenhq/lumen
cd lumen

# Install web/docs deps
pnpm install

# Run hub in dev mode (auto-reload, embedded dev frontend)
make dev-hub

# In another terminal, run a local agent pointed at the dev hub
make dev-agent

# Run docs site (Starlight)
make dev-docs
```

Hub: http://localhost:8090 · Docs: http://localhost:4321

### Useful commands

```bash
make test           # unit tests
make test-e2e       # end-to-end (slow)
make lint           # golangci-lint + biome
make benchmark      # RAM + ingest throughput benchmark
make build          # build hub + agent binaries to ./dist/
make build-all      # cross-compile for linux/amd64, arm64, armv7
```

More detail: [docs/src/content/docs/contributing/development-setup.md](docs/src/content/docs/contributing/development-setup.md).

---

## Style guides

### Go
- `gofmt`/`goimports` enforced.
- `golangci-lint` config in repo — must pass.
- Effective Go conventions: short names in short scopes, errors as values, no `panic` in libraries.
- No `init()` for anything beyond registering with a registry.
- Avoid `interface{}` / `any` unless truly generic.

### TypeScript (web + docs)
- Biome enforced (formatter + linter).
- React function components only, hooks-based.
- State: `zustand` for app state, `react-query` for server state. No Redux.
- Styling: Tailwind + shadcn/ui only — no new CSS-in-JS libraries.

### Commits
[Conventional Commits](https://www.conventionalcommits.org/):
```
<type>(<scope>): <subject>

<body — optional>

<footer — optional, e.g. "Closes #123">
```
Types: `feat`, `fix`, `docs`, `chore`, `refactor`, `test`, `perf`, `build`, `ci`.

### Docs
- Write to the user, not for the author. Use "you", active voice.
- Diátaxis: tutorial / how-to / reference / explanation — keep them separate.
- Code blocks must be copy-paste runnable.
- Add to the relevant `sidebar` config when creating new pages.

---

## Contributor tiers

We use a gradual-trust model. Nobody is gatekept by a test or interview — trust is earned through quality contributions over time.

| Tier | Permissions | How to reach |
|---|---|---|
| **Visitor** | Open issues, post in Discussions | — |
| **Triager** | Label & close duplicate issues, support newcomers | 5+ helpful comments, invited by a maintainer |
| **Contributor** | Open PRs (everyone can), get the `contributor` Discord role after first merge | 1 merged PR |
| **Trusted Contributor** | Advisory PR reviews, vote on RFCs, contributors-only Discord channel | 5+ merged PRs across code + docs/tests, sustained over 1+ month |
| **Maintainer** | Merge PRs, cut releases, set roadmap | Nominated by Core after ~6 months of steady, high-judgment contribution; consensus of Core team |
| **Core Team** | Architecture decisions, project direction | Promoted from Maintainer by consensus |

**First-time contributor?** Look for issues labeled [`good first issue`](https://github.com/lumenhq/lumen/labels/good%20first%20issue). A maintainer will mentor your first PR.

**What gets you noticed for higher tiers** (not a checklist — guidance):
- Consistently high-quality PRs (not just quantity)
- Good judgment in reviews — what to push back on, what to let go
- Helping others in Discussions/Discord
- Owning a subsystem (becoming the de-facto expert on, e.g., the Proxmox collector)
- Writing & shipping an RFC

See [GOVERNANCE.md](GOVERNANCE.md) for how decisions get made.

---

## Where to ask questions

- **Bug / feature** → GitHub Issues / Discussions
- **Usage help** → Discussions → Q&A category
- **Realtime chat** → [Discord](#) (link TBD)
- **Security issue** → see [SECURITY.md](SECURITY.md) — do NOT open a public issue

---

By contributing, you agree to license your contribution under the [MIT License](LICENSE)
and abide by our [Code of Conduct](CODE_OF_CONDUCT.md).

Thanks for making Lumen better. 💚
