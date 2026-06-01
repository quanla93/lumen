# RFC 0003 — Public Read API (Phase 7, v0.5.0)

Status: **Shipped** · Created 2026-06-01 · Released as v0.5.0 on 2026-06-01

> Design record (not an implementation plan). The code shipped in v0.4.11 + v0.5.0; this RFC captures the decisions for future reference and to anchor the v0.5.x follow-ups.

## Context

Lumen had no way for external tools (Grafana, n8n, custom scripts, mobile apps) to read its data — every query went through the admin web UI's session cookie. Phase 7's original framing put Cold tier (Parquet) first with the external API as a downstream consumer of cold data; that order was flipped because:

- Public API is a low-risk expose layer over data that already exists in SQLite. Ships in ~1 week.
- Cold tier is a multi-week storage rewrite with cgo / DuckDB unknowns. Worth doing only if SQLite + v0.4.1 retention sweep proves insufficient.
- **Demand-driven**: ship the API, measure if anyone queries >7d ranges, then decide if Cold tier is justified.
- Homelab fleet math (≤30 hosts × 30–90d retention) is comfortably bounded by retention sweep already. Cold tier matters only for power users above that.

See ACTION_PLAN.md Phase 7 section for the reorder rationale.

## Scope

**v0.5.0 (this RFC — shipped):**
- Bearer-key authentication on a versioned `/api/v1/*` surface, separate from the internal session-cookie `/api/*` API.
- Admin UI to mint / list / revoke keys with scopes and an optional glob host filter.
- Read-only endpoints: hosts (list + detail), metrics (downsampled time-series, ≤7d), alerts (events + rules).
- In-memory per-key token-bucket rate limit (100/min default).
- Rich envelope `{success, data, error: {code, message}, request_id}` on every public response.

**v0.5.1 (next):** RFC + reference docs in `docs/`, Grafana JSON datasource recipe, curl example index. (This RFC is the RFC half; reference doc ships alongside.)

**Deferred (post-v0.5.x):**
- Write endpoints (POST/PUT/DELETE) — out of scope for "Read API"; require a separate scope model and audit log.
- Webhook outbound (customer-managed receivers + HMAC) — blocks on unifying with the alert notification dispatcher (Phase 6 channel).
- Prometheus-compatible endpoint — wait for Grafana spike to confirm which compatibility shape is actually demanded.
- OAuth2 / SAML / SSO — homelab single-admin doesn't need these.
- Per-key rate-limit override — fixed default for now; add per-key column when an operator asks.
- Tag-pair host filter — glob is enough for v0.5.0; tag selectors are heavier UI work, defer until demand surfaces.
- CORS — caller must be behind a reverse proxy / on the same LAN. Open up when a browser-side integration appears.

## Decisions

### Auth model: per-key Bearer token, SHA-256 hash, plaintext-shown-once

Keys are `lumk_` + 32 random bytes, base64url-encoded (43 chars total). Storage holds only the SHA-256 hex hash; the plaintext is returned to the operator exactly once at create time and never persisted (same scheme as host tokens — server-generated entropy, Argon2id would be wasted work on every authenticated request).

A 12-char `preview` ("lumk_AbCdEfGh") is persisted for the admin list view so an operator can recognise which key is which without seeing the secret a second time.

**Why per-key, not per-IP or global**:
| Approach | Why not |
|---|---|
| Per-IP | NAT / Grafana cluster / shared egress collapse multiple consumers onto one IP |
| Per-endpoint | More granular but unnecessary for v1; same key wanting both hosts + metrics is the common case |
| Global | One noisy key throttles every integration; can't pinpoint who's misbehaving |
| **Per-key** | Operator-controlled, revocable per consumer, matches GitHub/Stripe/OpenAI |

### Versioning: path-based, `/api/v1/*`

Path prefix lets operators see the version in any URL / log line / Grafana datasource config. v2 would land at `/api/v2/*` in parallel; the two surfaces could coexist for a deprecation window. Header-based versioning (`Accept-Version: 1`) was rejected — too easy to forget, harder to debug, and the rest of the industry has settled on path prefix.

### Envelope: rich on public, terse on internal

Public `/api/v1/*` responses wrap every result:

```json
{
  "success": true,
  "data": { ... },
  "error": null,
  "request_id": "abc123"
}
```

Error shape:

```json
{
  "success": false,
  "data": null,
  "error": { "code": "INVALID_AUTH", "message": "..." },
  "request_id": "abc123"
}
```

Internal `/api/*` (used by the web UI) keeps its terse shape `{"error": "..."}`. Mixing the two would force every internal handler to either ignore the new envelope or grow a schema-detect branch — cleaner to keep them separate and document the boundary.

`request_id` is the chi-middleware request ID, surfaced so an operator can grep server logs after a failed call.

Error codes are uppercase snake (`MISSING_AUTH`, `INVALID_AUTH`, `INSUFFICIENT_SCOPE`, `RATE_LIMITED`, `NOT_FOUND`, `BAD_REQUEST`, `INTERNAL_ERROR`). Adding a code requires a new constant in `internal/hub/publicapi/envelope.go` so we don't end up with three spellings of the same failure.

### Scopes: minimal read-only, no wildcards

v0.5.0 ships exactly three:
- `read:hosts`
- `read:metrics`
- `read:alerts`

No wildcard `read:*`. No write scopes. Operator selects ≥1 scope at create; the verify middleware rejects with `INSUFFICIENT_SCOPE` (403) if the endpoint's required scope is missing.

### Host filter: glob (`path.Match`), not regex, not tag selector

Optional per-key glob (`*pve*`, `web-01`, `prod-*`). Empty/null = all hosts (subject to scopes). Filter is enforced:
- On `/api/v1/hosts` — filtered server-side, key never sees excluded hosts.
- On `/api/v1/hosts/{name}` and `/metrics` — 404 returned (same response as "host doesn't exist") so a key can't probe for hosts outside its filter.
- On `/api/v1/alerts/events` — over-fetch then drop in Go (alert_events table indexes don't fit a glob WHERE clause; fine at homelab scale).
- On `/api/v1/alerts/rules` — **not** enforced; rules describe rule-level state (selector, threshold), not per-host facts. Exposing a rule that fires for hosts the key can't see is fine because the rule definition itself isn't sensitive.

Regex was rejected (operator footgun, ReDoS risk). Tag-pair selectors (like alert rule selectors) deferred — heavier UI work, defer until an operator asks.

### Rate limit: per-key in-memory token bucket

`Limiter` in `internal/hub/publicapi/middleware.go`. Default 100-token burst capacity, 100/min refill (≈ 1.67/s). Each authenticated request consumes one token; an exhausted bucket returns `429 RATE_LIMITED` with `Retry-After: <seconds>` and `X-RateLimit-Limit` / `X-RateLimit-Remaining` headers (set on every response, not just on throttle, so a polite client can self-pace).

**Why bucket (not counter)**: a bucket allows a brief burst (Grafana loading 10 panels at once consumes 10 tokens in 1s, but doesn't throttle), while a counter would reject the burst even if sustained rate is low.

**Why in-memory, not Redis**: single-binary discipline. Trade-offs:
- Hub restart resets buckets → every key gets full quota immediately. Acceptable — restarts are rare and a brief over-allowance is harmless.
- Memory growth bounded by key count (operator-created, realistically 5–20 keys).
- No cluster scaling. Fine — Lumen is anti-feature single-hub by design.

Per-key override (e.g. a Grafana key with 500/min) is deferred to v0.5.x — add a `rate_limit_per_min INTEGER NULL` column to `api_keys` and read it in the limiter.

### Probing protection: same 404 for "absent" and "filtered out"

If a key has filter `web-*` and queries `/api/v1/hosts/db-1`, the response is `404 NOT_FOUND` — not `403 FORBIDDEN`. Same response if `db-1` genuinely doesn't exist. This prevents key holders from enumerating hostnames outside their filter by binary-searching with 403 vs 404.

### Storage: SHA-256 hex hash, unique-indexed

Migration 0016 created `api_keys(id, name, hash, preview, scopes, host_filter, last_used_at, created_at)` with a unique index on `hash` for the verify hot path. `scopes` is stored as a JSON array string (vs a join table) because the scope set is small and finite — no need for a normalised table.

`last_used_at` is updated fire-and-forget after a successful verify; failing the touch doesn't fail the request (auth itself already succeeded). It's the only writeable field on the public API path.

## Implementation map

- `internal/hub/storage/migrations/0016_api_keys.sql` — schema.
- `internal/hub/apikey/apikey.go` — `Create`, `List`, `Delete`, `VerifyAndTouch`, `HasScope`, token mint + hash helpers.
- `internal/hub/apikey/handlers.go` — admin CRUD endpoints, session-protected at the router.
- `internal/hub/publicapi/envelope.go` — `Envelope`, `WriteSuccess`, `WriteError`, error code constants.
- `internal/hub/publicapi/middleware.go` — `Authn`, `RequireScope`, `Limiter` (token bucket).
- `internal/hub/publicapi/handlers.go` — `Version`, `Hosts`, `HostDetail`, `HostMetrics`, `AlertEvents`, `AlertRules`.
- `internal/hub/server/server.go` — route wiring: admin CRUD inside the existing `requireSession` group, public `/api/v1/*` in its own group with `Authn` + `Limiter` middleware.
- `web/src/components/Settings.tsx` — `ApiKeysSettings` tab (mint / list / revoke with plaintext-shown-once reveal).
- `web/src/lib/api.ts` — `apiKeysApi` client.
- `web/src/i18n/messages.ts` — EN + VI strings.

## What's deferred (v0.5.x and v0.6+ pickup order)

| Item | When to pick up | Why deferred |
|---|---|---|
| Reference docs + Grafana recipe | v0.5.1 | Ships alongside this RFC as the doc half of v0.5.0 |
| `/api/v1/alerts/channels` read | v0.5.x if asked | Operator-internal config; expose only if integrations need it |
| Per-key rate-limit override | v0.5.x if asked | Default 100/min is fine for most integrations |
| Tag-pair host filter | v0.5.x if asked | Glob covers the common case; heavier UI work |
| Prometheus-compatible exporter | v0.6+ after Grafana spike | Decide based on what Grafana spike learns |
| Write endpoints | v0.6+ behind a new scope set | New scope model (`write:rules` etc.) + audit log |
| Webhook outbound | v0.6+ | Blocked on unifying with Phase 6 alert notification engine |
| OAuth2 / SSO | v0.7+ (Phase 8) | Homelab anti-feature; expand only if multi-user lands |
| CORS | When a browser integration shows up | Currently caller must be on LAN / behind reverse proxy |
