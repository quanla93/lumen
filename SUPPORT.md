# Support

This document explains where to ask for help and what information to include.

## Where to ask

| Need | Best place |
|---|---|
| Bug report | GitHub Issues → Bug report template |
| Feature idea | GitHub Discussions → Ideas |
| Usage question | GitHub Discussions → Q&A |
| Install problem | GitHub Discussions or Issue if reproducible |
| Security issue | See [SECURITY.md](SECURITY.md); do not open a public issue |
| Code contribution question | GitHub Discussions or the relevant PR |

Realtime chat (Discord, Matrix, or similar) is not active yet. Open a Discussion or Issue for now — a community-chat space may land later, but it is not a blocker.

## Before asking

Please check:

- [README.md](README.md)
- [docs/src/content/docs/getting-started/quickstart.md](docs/src/content/docs/getting-started/quickstart.md)
- [docs/src/content/docs/install/](docs/src/content/docs/install/)
- Existing Issues and Discussions

## What to include

For install or runtime problems, include:

- Lumen version or commit.
- Install method: Docker Compose, native binary, Proxmox LXC, or source.
- Host OS and architecture.
- Relevant `LUMEN_*` settings with secrets redacted.
- Hub and agent logs around the failure.
- Browser console output for web UI issues.
- Steps to reproduce.

For performance issues, include:

- Number of hosts and ingest interval.
- Storage device type: HDD, SSD, SD card, or network disk.
- Approximate database size.
- CPU and RAM of the hub machine.

## Maintainer response expectations

Lumen is community-maintained. We aim for:

- Security reports: acknowledgement within 7 days.
- Bug reports: first triage within 7 days.
- Pull requests: first review within 7 days.
- General usage questions: best effort.

Polite pings are welcome after a week.

## Community standards

All support spaces follow the [Code of Conduct](CODE_OF_CONDUCT.md). Be specific, patient, and respectful of volunteer time.
