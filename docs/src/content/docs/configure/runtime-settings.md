---
title: Runtime settings
description: Configure agent collection interval and runtime policy from the Lumen web UI.
sidebar:
  order: 3
---

Runtime settings are stored in the hub database and can be changed from the web UI without rebuilding or redeploying agents.

## Agent collection interval

The main runtime knob today is **Settings → Runtime → Agent collection interval**.

| Setting | Default | Bounds | What it controls |
|---|---|---|---|
| `agent_interval` | `5s` | 2s – 1h | How often agents collect and POST a metrics snapshot. |

Shorter intervals make the dashboard feel more live but increase network traffic and SQLite rows. Longer intervals reduce writes and are friendlier to small HDD-backed hubs, but the dashboard can lag by up to one interval.

## How agents receive the policy

Agents still need a bootstrap interval from env/YAML so they can start before the hub is reachable. After successful ticks, the agent calls:

```http
GET /api/agent/policy
Authorization: Bearer lum_...
```

The hub responds with the current policy:

```json
{"collection_interval":"5s"}
```

If the interval changed in Settings, the agent applies it to future ticks without a redeploy. If the hub is unreachable, the agent keeps using its current local interval and continues buffering failed ingests.

## Precedence

At startup:

1. Process environment wins.
2. YAML config fills missing env keys.
3. `.env` fills missing keys in development.
4. Built-in defaults apply last.

After the agent has authenticated with the hub, hub policy can override the collection interval at runtime. The hub policy does not rewrite the target machine's env file, YAML file, or Docker Compose file.

## Practical values

| Interval | Good for | Tradeoff |
|---|---|---|
| `5s` | Default live dashboard feel | More rows and more frequent agent work. |
| `15s` | Small homelab with many hosts | Dashboard still feels live, writes drop 3×. |
| `30s` | HDD-first deployments | Lower write pressure, slower stale/online feedback. |
| `1m` | Low-priority hosts | Sparse charts and delayed incident visibility. |

For most installs, start with `5s` and increase only if disk writes or host count become a concern.

## Related settings

- [Retention](/configure/retention/) controls how long SQLite rows stay around.
- [Reliability](/configure/reliability/) explains buffering and hub batching.
- [API](/reference/api/#agent-policy) documents the policy endpoint.
