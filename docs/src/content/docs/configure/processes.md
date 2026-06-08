---
title: Process list
description: Top-N processes by CPU% or RSS on each host. Default OFF — opt-in to avoid leaking cmdline secrets.
---

Lumen can ship a top-N process list (PID, name, user, CPU%, RSS,
cmdline) on every host tick. Useful for the "what's eating my RAM"
ad-hoc diagnostic — click the host detail page and see who's hot.

**Default OFF.** The cmdline field routinely contains secrets:
`redis-cli --password=hunter2`, `curl -H TOKEN=abc123 ...`,
`deploy.sh --git-token=ghp_...`. Reading the API gives an
attacker the secrets for free. RFC 0003 §"Process list" calls
this out and the Lumen gate is therefore *opt-in per deployment*.

## How to enable (per host)

Two flags, both must be true:

1. **Server-side**: Settings → Runtime (or directly via the
   settings table). Default is `false`.
2. **Agent-side**: the agent's environment variable
   `LUMEN_AGENT_PROCESSES=true`. The agent won't ship process
   data unless the operator explicitly opts the host in.

The defense-in-depth means a misconfigured server can't accidentally
ship a process list, and a misconfigured agent can't accidentally
read one. Both halves must be true.

## How to read the data

Host detail page → "Top processes" table. The default sort is by
CPU%; the operator can change it to RSS via the settings table
(`processes.sort_by` = `rss`). Top-N defaults to 10, max 50.

A row looks like:

```
PID    Name            User     CPU%   RSS      Cmd
1234   python3         alice    42.1   312MiB   python3 -m jupyter notebook --port=8888
5678   redis-server    redis    8.4    128MiB   /usr/bin/redis-server /etc/redis/redis.conf
9012   mydeploy.sh     deploy   0.3    24MiB    <redacted: matches processes.redact_regex>
```

The `<redacted: ...>` placeholder appears when the cmdline matches
the defensive regex. The default regex catches common env-style
leaks — `password=`, `secret=`, `token=`, `api_key=`, `api-key=`
(case-insensitive). Operators can override the pattern with the
`processes.redact_regex` setting.

## Settings reference

| Key | Default | Description |
|---|---|---|
| `processes.enabled` | `false` | Master switch. Server-side. |
| `processes.top_n` | `10` | Cap on rows shipped per tick. Max 50. |
| `processes.sort_by` | `cpu` | `cpu` or `rss`. |
| `processes.redact_regex` | built-in | Override the defensive regex. |

## What the feature does NOT do

- **Process tree.** gopsutil returns a flat list. The parent/child
  graph is in /proc/$pid/status but useful UI for it is its own
  sprint.
- **Per-process network / disk I/O.** Live-only just like the host's
  net_rx_bps / disk_r_bps; the per-process counterparts need
  /proc/$pid/io and a second sampling loop. Follow-up.
- **Container-aware grouping.** Containers already have their own
  table on the host detail page; grouping processes by container
  is its own UX sprint.
- **Send / kill from UI.** Anti-feature — the agent's mandate is
  observation, not control. v0.7.2 ships the same observation-only
  contract; future product decisions on management/control stay in
  the open question in `ACTION_PLAN.md`.

## Security trade-off in one paragraph

This feature exists because "what's eating my RAM" is the single
most common ad-hoc diagnostic a homelab operator runs. The trade-off
is "yes, an attacker with read access to the hub API can now see
host cmdlines." If your threat model excludes that, enable it. If
it doesn't, leave it off and use SSH + `top` / `ps` the way you
always have.
