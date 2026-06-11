---
title: Public Read API
description: Bearer-key authenticated /api/v1/* surface for Grafana, automation scripts, and external integrations that read Lumen data without an admin session.
sidebar:
  order: 3
---

The Public Read API lets Grafana, n8n, custom scripts, mobile apps, and any other external tool read Lumen data via stable versioned endpoints — no admin session, no scraping the dashboard. Available since **v0.5.0** (Phase 7 first slice).

This page is the operational reference. For the design decisions behind it (auth model, scope choice, rate-limit shape), see [RFC 0003 — Public Read API](https://github.com/quanla93/lumen/blob/main/docs/rfcs/0003-public-api.md).

## How it differs from `/api/*`

| | Internal `/api/*` | Public `/api/v1/*` |
|---|---|---|
| Auth | Session cookie (`lumen_session`) | Bearer key (`Authorization: Bearer lumk_…`) |
| Audience | Browser UI | External integrations |
| Versioned | No (unversioned) | Yes (`/v1/`) |
| Envelope | Terse `{"error": "..."}` | Rich `{success, data, error, request_id}` |
| Rate limit | None | Per-key token bucket (100/min default) |
| Stable? | Free to change between releases | Stable within a major version |

The two surfaces share storage but are wired through different middleware. Internal endpoints will keep their terse shape; this page only documents `/api/v1/*`.

## Minting a key

1. Sign in to Lumen as admin.
2. **Settings → API Keys**.
3. Fill the form:
   - **Name** — operator-facing label (e.g. `grafana-prod`, `n8n-incident-bot`).
   - **Scopes** — tick at least one of `read:hosts`, `read:metrics`, `read:alerts`.
   - **Host filter** — optional glob pattern (`*pve*`, `web-01`, `prod-*`). Empty = all hosts (subject to scopes).
4. Click **Generate key**.
5. Copy the plaintext that appears in the green banner — it's shown **exactly once**. The hub stores only a SHA-256 hash; if you lose it, revoke and create a new one.

Keys are listed below with their prefix (`lumk_AbCdEfGh…`), scopes, host filter, last-used time, and a revoke button.

## Envelope

Every `/api/v1/*` response wraps its result:

```json
{
  "success": true,
  "data":    { ... },
  "error":   null,
  "request_id": "abc123"
}
```

On error:

```json
{
  "success": false,
  "data":    null,
  "error":   { "code": "INVALID_AUTH", "message": "unknown or revoked key" },
  "request_id": "abc123"
}
```

The `request_id` is the same as the `X-Request-Id` header. Quote it when reporting bugs — it's how server logs are correlated to your call.

### Error codes

| Code | HTTP | Meaning |
|---|---|---|
| `MISSING_AUTH` | 401 | No `Authorization: Bearer` header |
| `INVALID_AUTH` | 401 | Key doesn't look like a Lumen key, or doesn't match a row |
| `INSUFFICIENT_SCOPE` | 403 | Key authenticated but lacks the scope the endpoint requires |
| `RATE_LIMITED` | 429 | Token bucket exhausted; see `Retry-After` |
| `BAD_REQUEST` | 400 | Required query param missing or out of range |
| `NOT_FOUND` | 404 | Host doesn't exist OR your key's host filter excludes it (same response either way — so a key can't enumerate hosts outside its filter) |
| `INTERNAL_ERROR` | 500 | Hub-side bug; check `request_id` against server logs |

## Rate limit

Per-key in-memory token bucket. Default **100 token burst capacity, 100 tokens-per-minute refill**.

Every response (success OR throttle) sets:

| Header | Meaning |
|---|---|
| `X-RateLimit-Limit` | Burst capacity (always `100` in v0.5.0) |
| `X-RateLimit-Remaining` | Tokens left in your bucket right now |
| `Retry-After` | Seconds to wait — set only on 429 responses |

Bucket state is in-memory: a hub restart resets every key's tokens to full. There's no per-IP or global limit — only per-key.

## Endpoint catalog

All endpoints require `Authorization: Bearer lumk_…`. The scope column shows what the key must hold; calls missing a scope return 403 `INSUFFICIENT_SCOPE`.

| Method | Path | Scope | Description |
|---|---|---|---|
| GET | `/api/v1/version` | (any valid key) | Hub build version + ping |
| GET | `/api/v1/hosts` | `read:hosts` | List hosts (filtered by key's host filter) |
| GET | `/api/v1/hosts/{name}` | `read:hosts` | One host detail |
| GET | `/api/v1/hosts/{name}/metrics` | `read:metrics` | Downsampled time-series |
| GET | `/api/v1/alerts/events` | `read:alerts` | Alert event history |
| GET | `/api/v1/alerts/rules` | `read:alerts` | Read-only rule inventory |

### Version

```bash
curl -H "Authorization: Bearer lumk_..." http://hub:8090/api/v1/version
```

```json
{
  "success": true,
  "data": { "version": "0.5.0" },
  "error": null,
  "request_id": "..."
}
```

Use this as a cheap ping to confirm the key works.

### List hosts

```bash
curl -H "Authorization: Bearer lumk_..." http://hub:8090/api/v1/hosts
```

```json
{
  "success": true,
  "data": {
    "hosts": [
      { "name": "lumen-hub",  "created_at": "2026-05-29T08:00:00Z", "last_seen_at": "2026-06-01T13:25:00Z" },
      { "name": "adguard",    "created_at": "2026-05-30T14:00:00Z", "last_seen_at": "2026-06-01T13:25:05Z" }
    ]
  },
  "error": null,
  "request_id": "..."
}
```

Filtered server-side by the key's host filter. Hosts not matching the glob are not returned (and not counted toward 404 — they're just absent).

### Host detail

```bash
curl -H "Authorization: Bearer lumk_..." http://hub:8090/api/v1/hosts/adguard
```

Returns the same shape as one entry in `/api/v1/hosts` but as a single object under `data`. Returns 404 `NOT_FOUND` if the host doesn't exist OR the key's filter excludes it.

### Host metrics

Downsampled time-series for one host. Mandatory query params:

| Param | Format | Bounds |
|---|---|---|
| `from` | RFC3339 UTC timestamp | — |
| `to` | RFC3339 UTC timestamp | Must be > `from` |
| `bucket` | Go duration (`30s`, `1m`, `5m`, `1h`) | ≥ 30s |

Caps enforced:
- `to - from ≤ 7 days`
- `(to - from) / bucket ≤ 1000 points` — increase `bucket` if you hit this

```bash
curl -G -H "Authorization: Bearer lumk_..." \
  --data-urlencode 'from=2026-06-01T00:00:00Z' \
  --data-urlencode 'to=2026-06-01T06:00:00Z' \
  --data-urlencode 'bucket=5m' \
  http://hub:8090/api/v1/hosts/adguard/metrics
```

```json
{
  "success": true,
  "data": {
    "host": "adguard",
    "from": "2026-06-01T00:00:00Z",
    "to":   "2026-06-01T06:00:00Z",
    "bucket_seconds": 300,
    "points": [
      {
        "ts": "2026-06-01T00:00:00Z",
        "cpu_pct": 3.1, "ram_pct": 41.0, "swap_pct": 0.0, "disk_pct": 24.1,
        "load1": 0.05, "load5": 0.08, "load15": 0.07,
        "net_rx_bps": 1024, "net_tx_bps": 2048,
        "disk_r_bps": 0, "disk_w_bps": 4096,
        "temp_c": 41.0
      },
      ...
    ]
  },
  "error": null,
  "request_id": "..."
}
```

Bucket is mandatory — there's **no "raw 5s" path on the public API**. Raw points are reserved for the UI's WebSocket stream (which is not part of the public API).

Empty buckets are dropped, not padded. If a host had no points in a bucket window, that timestamp simply isn't in the response — fill client-side if you need a continuous series.

### Alert events

```bash
# All events, last 100
curl -H "Authorization: Bearer lumk_..." http://hub:8090/api/v1/alerts/events

# Only currently firing, up to 50
curl -G -H "Authorization: Bearer lumk_..." \
  --data-urlencode 'state=firing' \
  --data-urlencode 'limit=50' \
  http://hub:8090/api/v1/alerts/events
```

Query params:

| Param | Values | Default |
|---|---|---|
| `state` | `firing` / `resolved` / `all` | `all` |
| `limit` | 1–500 | `100` |

```json
{
  "success": true,
  "data": {
    "events": [
      {
        "id": 421,
        "rule_id": 3,
        "rule_name": "cpu over 90% for 5m",
        "host": "adguard",
        "metric": "cpu_pct",
        "severity": "warning",
        "state": "firing",
        "value": 92.4,
        "message": "cpu_pct = 92.4 (threshold 90.0)",
        "started_at": "2026-06-01T12:00:00Z",
        "resolved_at": null
      }
    ]
  },
  "error": null,
  "request_id": "..."
}
```

Events for hosts outside the key's filter are dropped post-query. `resolved_at` is non-null only on resolved events.

### Alert rules

```bash
curl -H "Authorization: Bearer lumk_..." http://hub:8090/api/v1/alerts/rules
```

```json
{
  "success": true,
  "data": {
    "rules": [
      {
        "id": 3,
        "name": "cpu over 90% for 5m",
        "enabled": true,
        "metric": "cpu_pct",
        "comparator": "gt",
        "threshold": 90.0,
        "for_seconds": 300,
        "cooldown_seconds": 0,
        "severity": "warning",
        "host_selector": "",
        "host": ""
      }
    ]
  },
  "error": null,
  "request_id": "..."
}
```

Channel routing (which channels the rule notifies) is **not** exposed — that's operator-internal config. If you need to see notification deliveries, that endpoint will arrive in a later release.

## Integrations

### Grafana — JSON API datasource plugin

Install the [JSON API community datasource](https://grafana.com/grafana/plugins/marcusolsson-json-datasource/) and:

1. **Add data source → JSON API**.
2. URL: `http://hub:8090`
3. Custom HTTP Headers → add `Authorization` = `Bearer lumk_...`
4. Save & test.
5. In a panel, choose your datasource and set the query:
   - Path: `/api/v1/hosts/adguard/metrics?from=${__from:date:iso}&to=${__to:date:iso}&bucket=1m`
   - Fields tab:
     - JSONPath `$.data.points[*].ts` → time
     - JSONPath `$.data.points[*].cpu_pct` → cpu
6. Plot.

Grafana's `$__from` / `$__to` macros emit RFC3339, which the API accepts directly. Set the dashboard auto-refresh ≥ 30s so you don't burn the rate-limit budget.

### Shell scripts

A simple "is anything firing right now?" probe:

```bash
#!/usr/bin/env bash
set -euo pipefail
HUB=${HUB:-http://hub:8090}
TOKEN=${TOKEN:?set TOKEN=lumk_...}

firing=$(curl -fsS -H "Authorization: Bearer ${TOKEN}" \
  "${HUB}/api/v1/alerts/events?state=firing&limit=1" \
  | jq '.data.events | length')

if [ "${firing}" -gt 0 ]; then
  echo "ALERT: ${firing} event(s) firing"
  exit 1
fi
echo "OK"
```

### n8n / Zapier / similar

Use the HTTP Request node:
- Method: GET
- URL: `http://hub:8090/api/v1/alerts/events?state=firing`
- Authentication: Generic credential → Header Auth with `Authorization` = `Bearer lumk_...`

Map `data.events[*].host` and `data.events[*].rule_name` into the next step (Discord post, ticket creation, etc.).

## Stability promise

Within a major version (`/api/v1/*`):
- No endpoint removals.
- No breaking shape changes to existing fields.
- New fields may be added — clients should ignore unknown fields.
- New endpoints may be added without notice.

A `/api/v2/*` would be introduced for breaking changes, in parallel with `/api/v1/*` for a deprecation window. None planned right now.

Pre-1.0 caveat: the Lumen project itself is pre-1.0. The Public Read API is the most stable surface we have, but if a fundamental issue surfaces before v1.0 we reserve the right to ship a v2 sooner than we'd like.

## What's deferred

Items deliberately out of scope for v0.5.0; track [RFC 0003](https://github.com/quanla93/lumen/blob/main/docs/rfcs/0003-public-api.md) for the pickup order.

- Write endpoints (POST/PUT/DELETE) — need a new scope set + audit log
- Webhook outbound (customer-managed receivers, HMAC-signed)
- Prometheus exporter format
- Per-key rate-limit override
- Tag-pair host filter (glob is enough for v1)
- CORS (caller must be behind a reverse proxy / on LAN)
- `>7d` metrics queries (waits on Cold tier; Sprint 10 in the [roadmap](https://github.com/quanla93/lumen/blob/main/ACTION_PLAN.md) — conditional on real demand, defer otherwise)
