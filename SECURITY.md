# Security Policy

## Supported versions

Lumen is pre-1.0. Security fixes target the latest commit on `main` until the first public release.

After `v0.1.0`, we support:

| Version | Supported |
|---|---|
| Latest minor | Yes |
| Older pre-1.0 minors | Best effort |

## Reporting a vulnerability

Please do not open a public issue for suspected vulnerabilities.

Report security issues privately to `security@quanla.org`.

If that address is not active yet, contact the current repository owner directly through GitHub and include `SECURITY` in the subject or first line.

Include as much detail as you can:

- Affected version or commit.
- Impact and attacker capabilities required.
- Reproduction steps or proof of concept.
- Logs, screenshots, or packet captures if relevant.
- Whether the issue is already public.

We aim to acknowledge reports within 7 days.

## Disclosure process

1. Maintainers confirm receipt and reproduce the issue.
2. Maintainers assess severity and scope.
3. A fix is prepared privately when needed.
4. A release is published with a security note.
5. Credit is given unless the reporter requests otherwise.

We do not currently run a paid bug bounty program.

## Security boundaries

Lumen's main security boundaries are:

- Hub authentication and session handling.
- Agent bearer tokens used for ingest.
- Host and container metadata visible in the web UI.
- Local files under `/etc/lumen` and `/var/lib/lumen`.
- Docker socket access when the agent Docker collector is enabled.

The agent may need privileged host access to read system metrics and Docker state. Treat agent hosts and hub tokens as sensitive.

## Hardening expectations

Production-like installs should:

- Run the hub behind HTTPS.
- Use a strong `LUMEN_HUB_SECRET`.
- Rotate host ingest tokens when a host is decommissioned.
- Restrict access to `/etc/lumen` and `/var/lib/lumen`.
- Avoid exposing the hub directly to the public internet without a reverse proxy or tunnel with authentication.

## Out of scope

The following are usually out of scope unless they expose a real Lumen vulnerability:

- Social engineering.
- Denial-of-service attacks against public infrastructure.
- Reports requiring physical access to the host.
- Vulnerabilities in unsupported versions or modified builds.
- Scanner-only reports without impact or reproduction.
