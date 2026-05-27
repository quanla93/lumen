---
title: API
description: Every REST endpoint and WebSocket frame the hub exposes. Stable wire format under /api/* (versioning lands at v0.1.0 as /api/v1/*).
sidebar:
  order: 2
---

The hub speaks two protocols:

- **HTTP/JSON** under `/api/*` for control + history + auth
- **WebSocket** at `/api/stream` for live updates

A canonical [OpenAPI 3.1 spec](https://github.com/quanla93/lumen/blob/main/api/openapi.yaml)
lives in the repo at `api/openapi.yaml` — import it into Postman /
Insomnia / Bruno / Hoppscotch for ready-to-use request collections.
There's also a `api/lumen.http` file for the VS Code REST Client.

## Authentication models

Two different schemes, picked per endpoint:

| Scheme | Used by | Sent how |
|---|---|---|
| **Session cookie** (`lumen_session`) | Browser + UI endpoints | HS256 JWT in HttpOnly cookie set by `/api/login` |
| **Bearer token** (`lum_…`) | Agent ingest only | `Authorization: Bearer lum_…` header |

The session cookie is set with `HttpOnly`, `SameSite=Lax`, and
`Secure` (under HTTPS). 30-day TTL. The bearer token is minted per
host in **Settings → Hosts → Create**; the plaintext is shown once
and never stored — only its SHA-256 hash lives in the DB.

## Endpoints

### Health

```http
GET /healthz
→ 200 {"status":"ok"}
```

No auth. Returns 200 as long as the hub process is up. Use this for
container healthchecks and uptime monitors.

### Ingest

```http
POST /api/ingest
Authorization: Bearer lum_REPLACE_ME
Content-Type: application/json

{
  "host": "ignored-server-overrides",
  "ts":   "2026-05-26T08:14:00Z",
  "cpu_pct":     12.5,
  "cpu_per_core": [10.1, 14.3, 12.7, 13.0],
  "ram_pct":     63.2,
  "swap_pct":     0.0,
  "disk_pct":    41.5,
  "load1":        0.42,
  "load5":        0.51,
  "load15":       0.49,
  "net_rx_bps":  10240,
  "net_tx_bps":   5120,
  "disk_r_bps":      0,
  "disk_w_bps":  20480,
  "temp_c":      48.3,
  "containers": [
    {
      "id": "abc123def456",
      "name": "nginx",
      "image": "nginx:1.27",
      "state": "running",
      "cpu_pct": 0.3,
      "mem_used_bytes":  78643200,
      "mem_limit_bytes": 536870912,
      "mem_pct": 14.6
    }
  ]
}
→ 204 No Content
```

The hub looks up the token, sets `req.host` from the host record
(any value the agent sent in `host` is overwritten), validates
ranges, then stores the snapshot. A 401 means the token is invalid
or absent; a 400 means a field is out of range (e.g. `cpu_pct > 100`).

`cpu_per_core` and `containers` are **live-only** — they flow
through the WebSocket but aren't persisted. The historical
`/api/hosts/{id}/metrics` endpoint returns only the aggregate
scalars.

### Auth — setup status

```http
GET /api/setup-status
→ 200 {"admin_exists": true}
```

The bootstrap flag the UI uses to decide between Register and Login.
Once an admin exists, register is closed.

### Auth — register first admin

```http
POST /api/register
Content-Type: application/json
{"username":"admin","password":"…"}
→ 201 {"user":{"id":1,"username":"admin","created_at":"…"}}
  + Set-Cookie: lumen_session=…
```

One-shot. Returns 409 if an admin already exists. Password is
Argon2id-hashed; minimum 8 chars.

### Auth — login

```http
POST /api/login
{"username":"admin","password":"…"}
→ 200 {"id":1,"username":"admin","created_at":"…"}
  + Set-Cookie: lumen_session=…
```

Returns 401 on wrong credentials. The body intentionally doesn't
distinguish "no such user" from "bad password" — same response
either way.

### Auth — logout / me

```http
POST /api/logout
→ 204  (idempotent)

GET /api/me
→ 200 {"id":1,"username":"admin","created_at":"…"}
→ 401 if no/expired session cookie
```

### Auth — change password

```http
POST /api/account/password
Cookie: lumen_session=…
{"current_password":"…","new_password":"…"}
→ 204
→ 401 if current_password wrong
→ 400 if new_password too short (<8)
```

Rehashes with Argon2id. The session cookie stays valid (we don't
force a re-login on password change).

### Hosts — list

```http
GET /api/hosts
Cookie: lumen_session=…
→ 200 [{"id":1,"name":"webA","created_at":"…","last_seen_at":"…"}, …]
```

`last_seen_at` updates on every successful ingest; null until the
host first checks in.

### Hosts — create

```http
POST /api/hosts
{"name":"webA"}
→ 201 {"host":{"id":1,"name":"webA",…},"token":"lum_…"}
```

The bearer token is returned **once** — copy it now or rotate. The
DB only stores its SHA-256 hash.

### Hosts — rotate token

```http
POST /api/hosts/{id}/rotate
→ 200 {"token":"lum_…"}
```

Invalidates the previous token; the existing agent will start
returning 401 until you re-deploy.

### Hosts — delete

```http
DELETE /api/hosts/{id}
→ 204
```

Also evicts that host's in-memory snapshot so the dashboard stops
showing a ghost card.

### Hosts — metric history

```http
GET /api/hosts/{id}/metrics?from=2026-05-25T08:00:00Z&to=2026-05-26T08:00:00Z&step=60
→ 200 {
    "host": "webA",
    "from": "2026-05-25T08:00:00Z",
    "to":   "2026-05-26T08:00:00Z",
    "step_seconds": 60,
    "points": [
      {"ts":"2026-05-25T08:00:00Z","cpu_pct":12.5,"ram_pct":63.2,…},
      …
    ]
  }
```

Server-side AVG bucketing on the
`idx_snapshots_host_ts` index. Limits:

- Window ≤ 7 days (rejected with 400 otherwise)
- `step` ≥ 5 s
- Maximum 2000 points per response (auto-step picks ~120 buckets if
  omitted)

Fields returned match the persisted scalars (no `cpu_per_core`,
`containers`, or `cpu_series`).

### Settings — get / put

```http
GET /api/settings
→ 200 {
  "retention_window":"24h",
  "retention_interval":"1h",
  "agent_interval":"5s",
  "downsample_bucket_size":"5m",
  "downsample_hot_window":"24h",
  "downsample_archive_window":"8760h"
}

PUT /api/settings
{"retention_window":"6h","downsample_bucket_size":"10m"}
→ 200 {
  "retention_window":"6h",
  "retention_interval":"1h",
  "agent_interval":"5s",
  "downsample_bucket_size":"10m",
  "downsample_hot_window":"24h",
  "downsample_archive_window":"8760h"
}
```

Bounds:

| Key | Range |
|---|---|
| `retention_window` | 5 m – 365 d (or `0` to disable) |
| `retention_interval` | 1 m – 24 h (or `0` to disable) |
| `agent_interval` | 2 s – 1 h |
| `downsample_bucket_size` | 1 m – 24 h |
| `downsample_hot_window` | 1 h – 30 d |
| `downsample_archive_window` | 1 d – 365 d |

The downsample values configure the future Parquet cold tier: bucket size is the time span represented by one archived point (`5m` averages old samples into one point every 5 minutes), hot window is how long full-detail raw SQLite rows are kept (`24h` keeps every sample for the last day), and archive window is how long compressed history is kept (`8760h` is about one year). Out-of-range or unparseable durations return 400. UI edits propagate to the retention loop within 30 s for retention fields; downsample fields are stored now and consumed once cold-tier compaction lands.

## WebSocket — `/api/stream`

Open with the session cookie attached:

```js
const ws = new WebSocket("wss://lumen.example.lan/api/stream");
ws.onmessage = (e) => {
  const snapshots = JSON.parse(e.data); // HostSnapshot[]
};
```

The hub pushes a snapshot every `LUMEN_HUB_STREAM_INTERVAL`
(default 5 s). Each frame is the entire array of currently-known
hosts (subject to the subscription filter — see below).

### HostSnapshot shape

```ts
type HostSnapshot = {
  host: string;
  ts: string;          // RFC3339
  cpu_pct: number;
  cpu_per_core?: number[];  // live-only
  ram_pct: number;
  swap_pct: number;
  disk_pct: number;
  load1: number; load5: number; load15: number;
  net_rx_bps: number; net_tx_bps: number;
  disk_r_bps: number; disk_w_bps: number;
  temp_c: number;
  containers?: ContainerInfo[];  // live-only
  cpu_series?: number[];         // last ~120 CPU% values, oldest first
};

type ContainerInfo = {
  id: string; name: string; image: string; state: string;
  cpu_pct: number;
  mem_used_bytes: number; mem_limit_bytes: number; mem_pct: number;
};
```

### Subscribe (control frame)

By default a connection receives every host snapshot. Send a control
frame to narrow:

```js
// Filter to one host (used by the detail view)
ws.send(JSON.stringify({type: "subscribe", hosts: ["webA"]}));

// Revert to firehose
ws.send(JSON.stringify({type: "subscribe", hosts: ["*"]}));
```

Empty list or no frame ever sent = firehose (Phase 1 dashboard
behavior). Unknown control types are ignored.

## Error format

Every 4xx / 5xx response from `/api/*` is JSON:

```json
{"error": "human-readable message"}
```

Validation errors include the offending field name when possible
(e.g. `"error":"cpu_pct out of [0,100]"`). 401 / 403 messages are
intentionally terse to avoid information leaks.

## Versioning

Pre-v0.1.0 the API is `/api/*` and can break between commits. At
v0.1.0 the stable surface moves to `/api/v1/*` and the unversioned
paths become aliases for one release. After v0.2.0, unversioned
paths are removed.
