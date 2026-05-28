---
title: Hosts and bearer tokens
description: How Lumen identifies hosts, mints + rotates per-host tokens, and survives leaks.
sidebar:
  order: 1
---

A "host" in Lumen is a row in the hub's `hosts` table. Each row has:

| Column | Notes |
|---|---|
| `id` | Auto-increment; used in URLs (`/api/hosts/{id}/metrics`) |
| `name` | Display name + SQLite key for snapshots (no spaces; letters, digits, `-_.`) |
| `token_hash` | SHA-256 of the plaintext `lum_...` token |
| `created_at` | When you minted it |
| `last_seen_at` | Updated when a valid token POSTs `/api/ingest` |

The plaintext token is **shown exactly once** at creation / rotation and
never persisted. If you close the dialog without copying it, rotate.

## Mint a token

UI: **Settings → Hosts → Create**.

The hub:

1. Generates 32 bytes of randomness (`crypto/rand`).
2. Prefixes `lum_` and base64-URL-encodes → that's the token you see
   (~46 chars total).
3. Stores `sha256(token)` hex in `hosts.token_hash`.
4. Returns the plaintext to the browser **one time**.

The token has full machine entropy — SHA-256 (not Argon2id) is the
right hash. Argon2 would just burn CPU on every ingest for no
brute-force gain.

## Use the token

Set in the agent's environment:

```ini
LUMEN_AGENT_TOKEN=lum_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

Agent sends it as `Authorization: Bearer <token>` on every
`POST /api/ingest`. Without the header (or with an unknown token) the
hub returns **401** — strict mode, no anonymous ingest.

## Host-name spoof protection

When the hub validates a token, it looks up the **host row** and
**overrides `body.host`** with the row's name before storing the
snapshot. So even if an agent posts:

```json
{ "host": "pretending-to-be-admin", "cpu_pct": 100, ... }
```

with the `pve-node-01` token, the row stored is `pve-node-01`. A leaked
token can only push noise into the host it was minted for — it can't
masquerade as another host.

## Rotate a token

UI: **Settings → Hosts → Rotate (↻)** next to the row.

Effects:

- New plaintext shown once.
- Old token immediately rejected (the stored hash is overwritten).
- Snapshots already accepted under the old token stay — `host` rows
  don't churn on rotation.
- The agent on the target host will log `hub returned 401: invalid
  token` until you update its `LUMEN_AGENT_TOKEN` and restart it.

For the recommended Docker Compose agent path, edit `/opt/lumen-agent/docker-compose.yml` on the target host and replace `LUMEN_AGENT_TOKEN`, then recreate the container:

```bash
cd /opt/lumen-agent
sudo docker compose up -d
```

For native/manual agents, update the systemd environment or YAML config and restart `lumen-agent`.

## When to rotate

- After exposing the token by accident (config in a public repo, screen
  share, shoulder surf).
- After an operator who knew the token leaves your team.
- On a regular schedule (every 90 / 180 days) — purely hygiene; tokens
  themselves don't expire.
- If `last_seen_at` is recent but the host was decommissioned
  (suspicious — the token may be in use somewhere you forgot).

## Delete a host

UI: **Settings → Hosts → Delete (🗑)**.

Effects:

- Row removed from `hosts` table.
- In-memory snapshot for that host name is evicted immediately (the
  WS stream stops broadcasting it; the dashboard card disappears).
- Historical `snapshots` rows for that host name are **preserved**
  (audit trail) — they age out via retention like any other.
- The agent on the deleted host will start logging 401 invalid-token.

If you intend to re-create with the same name later, the historical
rows automatically rejoin the new host on the next ingest (snapshots
table joins on host name, not id).

## Inspect from CLI

```bash
# List hosts (cookie auth — get one via /api/login first)
curl -sS -b cookies.txt http://your-hub:8090/api/hosts | jq

# Mint a token via API (same as the UI does)
curl -sS -b cookies.txt -X POST http://your-hub:8090/api/hosts \
  -H 'Content-Type: application/json' \
  -d '{"name":"new-host"}' | jq

# Rotate
curl -sS -b cookies.txt -X POST http://your-hub:8090/api/hosts/3/rotate | jq

# Delete
curl -sS -b cookies.txt -X DELETE http://your-hub:8090/api/hosts/3
```

The full schema is in [api/openapi.yaml](https://github.com/quanla93/lumen/blob/main/api/openapi.yaml).

## Common patterns

**One token per host, manually mint**: smallest setup; works fine
up to ~30 hosts.

**Token-per-environment + token-per-LXC**: separate tokens for
"production" vs "lab" so a lab leak doesn't reveal a prod token.

**Per-team scopes**: not yet supported. The token model is one-tier
(any token = full ingest for its host). Multi-tier access (read-only
viewer, per-team scopes) lands in Phase 4 (Multi-user).

## Limits

- Maximum host name length: **64 chars**.
- Allowed chars in host name: letters, digits, `-`, `_`, `.`.
- Tokens never expire on their own — only rotation / delete revokes
  them.
- No hard limit on host count, but the hub's in-memory store keeps
  120 CPU samples × N hosts (~1 KiB per host) so 1000 hosts ≈ 1 MiB
  resident. Disk grows linearly with ingest rate × retention window.
