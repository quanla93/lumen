# Lumen Governance

Lumen uses a lightweight, gradual-trust governance model. The project should move quickly while keeping architectural decisions clear and reviewable.

## Roles

| Role | Permissions | How to join |
|---|---|---|
| Visitor | Open issues, join Discussions, submit PRs | Everyone starts here |
| Triager | Label issues, close duplicates, help with support | Invited after sustained helpful issue/Discussion work |
| Contributor | Submit PRs and participate in RFC discussion | Anyone with accepted contributions |
| Trusted Contributor | Advisory reviews, RFC votes, subsystem stewardship | Invited after repeated high-quality contributions |
| Maintainer | Merge PRs, cut releases, moderate project spaces | Nominated by Core and accepted by consensus |
| Core Team | Roadmap, architecture, governance changes | Promoted from Maintainers by consensus |

The current pre-public staging repo is owned by `quanla93`. Once the project moves to an organization, Core membership and maintainer permissions should be made explicit in this file.

## Decision making

Most decisions are made by lazy consensus:

1. Someone opens an issue, Discussion, PR, or RFC.
2. Maintainers and contributors discuss tradeoffs.
3. If no maintainer objects after a reasonable review window, the change can proceed.

Use these review windows by default:

- Small bug fix or docs change: no waiting period once CI passes.
- Medium feature or schema/API change: 3 days.
- Large architecture, storage, transport, security, or governance change: 7 days and an RFC or ADR.

Maintainers may fast-track urgent security fixes, CI fixes, or release blockers.

## Architecture decisions

Hard-to-reverse decisions require an ADR in [docs/adr/](docs/adr/). ADRs are append-only: if a decision changes, write a new ADR that supersedes the old one.

Examples that require an ADR:

- Storage engines or on-disk layout.
- Agent-to-hub transport.
- Authentication model.
- Language/runtime changes.
- Public API versioning.

## Pull requests

Lumen squashes PRs by default. A PR can merge when:

1. CI is green.
2. At least one Maintainer approves.
3. CODEOWNERS are satisfied for touched areas.
4. Architectural changes have two Maintainer approvals.
5. Docs and changelog are updated for user-facing changes.

Maintainers should avoid merging their own substantial PRs without another maintainer review once the project has more than one active maintainer.

## Releases

Maintainers cut releases from `main` using SemVer:

- Pre-1.0 breaking changes are allowed in minor releases.
- Patch releases are for bug fixes, docs fixes, and packaging fixes.
- Release notes are generated from [CHANGELOG.md](CHANGELOG.md).

Release blockers include known data-loss bugs, critical auth/security regressions, broken install paths, or missing license/security documentation.

## Conflict resolution

If contributors disagree, prefer written tradeoffs over authority. A Core Team member can make the final call when the project is blocked. The decision and rationale should be captured in the issue, PR, RFC, or ADR.

## Changes to governance

Governance changes require Core Team consensus and a PR updating this file.
