# ADR-0003: Language choice — Go for hub and agent

- **Status**: Accepted
- **Date**: 2026-05-27
- **Deciders**: Core team
- **Supersedes**: —
- **Superseded by**: —

## Context

Lumen ships two main binaries:

1. `lumen-hub`: a small self-hosted web/API server with embedded storage.
2. `lumen-agent`: a host metrics collector that should run on Linux servers, Proxmox nodes, LXC containers, and small homelab machines.

The runtime must support:

- Static or near-static Linux builds for amd64, arm64, and armv7.
- Low idle memory usage.
- Simple packaging into tarballs and containers.
- Direct OS metrics collection.
- A contributor-friendly codebase.
- Shared request/response types between hub and agent.

The web UI remains TypeScript/React because browser UI is the right place for that stack. This ADR covers the hub and agent runtime.

## Decision

Use Go for both hub and agent.

- Hub entrypoint: `cmd/lumen-hub`.
- Agent entrypoint: `cmd/lumen-agent`.
- Shared wire types live under `internal/shared`.
- Project code lives under `internal/hub` and `internal/agent`.
- Go's standard library is preferred; dependencies must be justified by size and operational value.

## Consequences

### Positive

- Single small binaries are straightforward to install with systemd or Docker.
- Cross-compilation is simple for Linux amd64, arm64, and armv7.
- Idle RAM stays low compared with JVM, Node server, or Python service stacks.
- Go has mature libraries for HTTP, WebSocket, SQLite, system metrics, and embedded KV stores.
- Hub and agent can share types without code generation.
- The language is widely understood by infrastructure contributors.

### Negative

- Go is less memory-safe than Rust for some classes of bugs, though still safer than C/C++.
- Some low-level system metrics require platform-specific handling or third-party packages.
- UI and backend use different languages, so contributors may specialize.
- Go dependency choices can raise the toolchain floor, as happened with gopsutil and goose.

### Neutral

- We accept a modern Go toolchain floor when dependencies require it, rather than pinning old dependency lines.
- Rust remains a possible future choice for isolated high-performance helpers, but not for core hub/agent code.

## Alternatives rejected

- **Rust for agent and/or hub**: excellent binaries and memory safety, but slower contributor velocity and more complex implementation for the current team. The performance upside is not needed for 1-200 hosts.
- **Java/Spring for hub**: productive for enterprise services, but too heavy for Lumen's low-RAM single-binary goal.
- **Node.js for hub**: aligns with the web stack, but requires a runtime, has higher baseline memory, and is less ideal for single-binary homelab installs.
- **Python for agent**: fast to prototype, but packaging, startup, and cross-platform service deployment are less clean than Go.
- **Split languages for hub and agent**: lets each component optimize locally, but creates duplicated models, more tooling, and slower contributor onboarding.

## References

- Decision log 2026-05-25: Hub language = Go.
- Decision log 2026-05-25: Agent language = Go.
- Decision log 2026-05-25/26: Go toolchain floor raised to follow maintained dependencies.

## Related ADRs

- [[ADR-0001: Storage architecture — SQLite (hot) + Parquet (cold)]]
- [[ADR-0002: Transport choice — HTTPS/WebSocket push over SSH]]
