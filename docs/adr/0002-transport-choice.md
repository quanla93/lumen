# ADR-0002: Transport choice — HTTPS/WebSocket push over SSH

- **Status**: Accepted
- **Date**: 2026-05-27
- **Deciders**: Core team
- **Supersedes**: —
- **Superseded by**: —

## Context

Lumen agents run on homelab hosts, Proxmox nodes, LXC containers, and small servers that often sit behind NAT, Cloudflare Tunnel, Tailscale, or residential ISP routers. The transport must:

1. Work outbound-only from the agent to the hub.
2. Be easy to proxy through standard HTTPS infrastructure.
3. Avoid requiring inbound SSH access to every monitored host.
4. Support live updates without polling every browser view.
5. Stay understandable for contributors and operators.
6. Keep the hub deployable as a single binary.

Lumen also needs a clear differentiator from Beszel's SSH-shaped model. We want the operator experience to be: create a host token in the hub, run the agent with the hub URL and token, and receive metrics.

## Decision

Use HTTPS as the only agent-to-hub transport.

- Agents push snapshots to the hub with `POST /api/ingest` and a bearer token.
- The hub validates the token server-side and binds the ingest to the configured host identity.
- Web clients receive live updates over WebSocket at `/api/stream`.
- WebSocket clients can subscribe to specific hosts to avoid fleet-wide firehose traffic.
- The hub serves the SPA, API, and WebSocket endpoints from the same origin.

The hub does not SSH into agents or require inbound ports on monitored hosts.

## Consequences

### Positive

- Works behind NAT, CGNAT, reverse proxies, Cloudflare Tunnel, and Tailscale.
- Uses infrastructure operators already understand: HTTP, TLS, reverse proxies, bearer tokens.
- One origin simplifies cookies, CORS, deployment, and docs.
- Agent install is copy-paste friendly: hub URL plus one host token.
- No hub-side credential vault for SSH private keys.
- Clear product positioning: HTTPS-only push transport.

### Negative

- The hub cannot pull metrics on demand from an offline agent.
- Agent tokens must be protected, rotated, and scoped correctly.
- Operators need HTTPS termination for internet-exposed deployments.
- WebSocket proxying must be documented for reverse proxies.
- Push ingest can burst after outages; the agent buffer and hub batcher must drain gradually.

### Neutral

- Polling remains available for REST history queries; WebSocket is only the live path.
- Future Proxmox API integration may be agentless, but that is a separate integration path and does not change the base agent transport.

## Alternatives rejected

- **SSH pull from hub to agents**: familiar for Unix operators, but requires inbound reachability, SSH key management, and host-level credentials on the hub. It is also less friendly to Cloudflare Tunnel and residential NAT setups.
- **Prometheus scrape model**: proven, but requires target discovery and inbound agent endpoints. It pushes Lumen toward Prometheus UX instead of a simple homelab appliance.
- **MQTT broker**: NAT-friendly, but adds another service or embedded broker semantics. It complicates install and auth for limited benefit.
- **gRPC streaming**: strong protocol ergonomics, but harder to debug with common tools and more awkward through some homelab reverse proxy setups than plain HTTP/WebSocket.
- **Peer-to-peer overlay**: powerful, but outside Lumen's scope and too operationally complex for v0.x.

## References

- Lumen anti-feature list: no enterprise observability stack, no cluster hub.
- Decision log 2026-05-25: Transport = HTTPS/WebSocket push.
- Decision log 2026-05-26: WS stream optional subscribe filter.

## Related ADRs

- [[ADR-0001: Storage architecture — SQLite (hot) + Parquet (cold)]]
- [[ADR-0003: Language choice (Go for hub + agent)]]
