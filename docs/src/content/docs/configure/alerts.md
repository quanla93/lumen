---
title: Alerts & notifications
description: Threshold-based alerts on hosts and metrics, delivered to ntfy, Discord, or any HTTPS webhook.
sidebar:
  order: 5
---

Lumen alerts you when a host goes off-spec. Rules evaluate every ~15s
against the in-memory snapshot store; transitions persist to SQLite and
fan out to every enabled notification channel.

This page covers all of v0.4.0 / [RFC 0001](https://github.com/quanla93/lumen/blob/main/docs/rfcs/0001-alerting.md) Milestones A–D:
**ntfy + Discord + Telegram + generic webhook**, **threshold rules + offline rule**, **host name/glob patterns**, **host tag inventory + label selectors** (Alerts → Tags tab), **per-rule channel routing**, **per-channel severity floor**, and a **persisted delivery queue** with severity-aware retry (Active / History / Rules / Channels / Deliveries / Tags sub-tabs). v0.4.1 added the **alert history retention sweep** (see [Retention](#retention) below). Email channel, HMAC on webhooks, and cooldown/flap suppression are deferred (see "What's not in v0.4.x" below).

## What you can alert on

| Metric | Use it for | Comparator | Threshold |
|---|---|---|---|
| `cpu_pct` | host CPU pressure | `gt` / `lt` | 0–100 |
| `ram_pct` | memory pressure | `gt` / `lt` | 0–100 |
| `swap_pct` | swap usage | `gt` / `lt` | 0–100 |
| `disk_pct` | root filesystem fill | `gt` / `lt` | 0–100 |
| `load1` | 1-minute load average | `gt` / `lt` | float |
| `offline` | agent stopped reporting | — | — |

The `offline` metric ignores comparator/threshold and fires when no
ingest has landed in at least 60 seconds. The 60-second floor is the
only "ignore blips" guard (≈12 missed 5-second heartbeats) — `for_seconds`
adds extra hold **on top** of that floor, not under it. So `for_seconds=0`
fires the moment a host has been silent for 60s; `for_seconds=120` waits
60s (detect) + 120s (hold) = 3 minutes total.

## Rule fields

- **Name** – shown in notifications and the Alerts tab.
- **Metric** – one of the values above.
- **Comparator** – `gt` (greater than) or `lt` (less than).
- **Threshold** – numeric value the metric is compared against.
- **For (seconds)** – the breach must persist this long before the rule
  fires. `0` means "fire immediately when the breach is observed".
- **Host** – three modes:
  - Empty → matches every host (registered + ever-seen).
  - Exact name (e.g. `webA`) → that one host only.
  - Glob pattern with `*`, `?`, `[…]` (e.g. `web-*`, `*-prod`,
    `db-[0-9]*`) → every matching host. Glob matching uses Go's stdlib
    `path.Match` semantics — see [glob reference](https://pkg.go.dev/path#Match).
- **Severity** – `info`, `warning`, or `critical`. Drives the
  notification title, the priority headers, and the colour pill.
- **Enabled** – uncheck to silence a rule without deleting it.
- **Send to channels** – pick which notification channels this rule
  fans out to. **Leave all unchecked to broadcast to every enabled
  channel** (default, preserves the Milestone-A behaviour for rules
  created before routing existed).

## Per-agent thresholds

A common need: different hosts deserve different thresholds. A noisy
batch worker shouldn't page on `cpu_pct > 80%`, but a customer-facing
API should. Lumen has three idioms for this; pick whichever scales to
your fleet shape.

### Idiom 1 — one rule per host

If you have a handful of hosts that each need a bespoke threshold,
create one rule per host:

| Rule name | Host | Metric | Comparator | Threshold | Severity |
|---|---|---|---|---|---|
| api-1 cpu hot | `api-1` | `cpu_pct` | `gt` | 60 | warning |
| worker-1 dead | `worker-1` | `offline` | — | — | critical |
| db-1 cpu spike | `db-1` | `cpu_pct` | `gt` | 80 | warning |

Each rule can also route to different channels (e.g. `worker-1 dead` →
pager, `api-1 cpu hot` → ntfy). Linear scaling: 100 hosts × 4 metrics =
400 rules. Fine for ≤ ~50 hosts; tedious past that.

### Idiom 2 — host glob, one rule per *tier*

If you name hosts with a structured prefix/suffix, encode the tier in
the name and use a glob rule per tier. New hosts inherit the right
threshold the moment they're created:

| Rule name | Host pattern | Threshold | Severity |
|---|---|---|---|
| prod cpu critical | `*-prod` | 60% | critical |
| staging cpu warm | `*-staging` | 80% | warning |
| any host offline | (empty = all hosts) | offline 60 s | critical |

Glob matching uses Go's [`path.Match`](https://pkg.go.dev/path#Match)
— `*`, `?`, and `[…]` character classes work. Pattern is the rule's
`host` field. Empty `host` still means "every host" — that's how the
"any host offline" line above covers the whole fleet with one rule.

Best when host naming is disciplined (`<role>-<env>` or similar).

### Idiom 3 — host tags + label selectors (Milestone C)

When the fleet is large *and* naming is heterogeneous, neither idiom
above scales: every host needs a per-rule line, and globs can't express
"hosts the ops team owns regardless of name." Lumen supports
Kubernetes-style **host tags** and **label selectors** on rules, backed
by a controlled tag inventory so operators can't typo themselves into a
broken selector.

#### Step 1 — define the tag inventory

Go to **Alerts → Tags**. Each tag is a `key` plus a fixed list of
allowed `value`s, e.g. `tier` → `{critical, important, normal}`.

- *New tag*: click **New tag**, type the key (letters/digits/`-`/`_`/`.`,
  ≤ 64 chars) and one value per line in the initial values box. Description
  is optional.
- *Add / remove values later*: click **Edit tag** in the table row. Adding
  a value is instant; removing one shows a confirm dialog with the impact
  ("removes from N hosts, drops from M rule selectors") before it cascades.
- *Delete the whole tag*: same flow, same impact preview. Every reference
  in `host_tags` is removed and every affected `host_selector` is rewritten
  to drop that key.

The inventory is the source of truth — hosts and rules can only pick
from it.

#### Step 2 — assign tags to hosts (in the same tab)

The right pane of **Alerts → Tags** lists every host. Click **Edit** on
a row, then for each tag key in the inventory pick a value from the
dropdown (or `— none —` to leave that key unassigned for this host).
Settings → Hosts still shows each host's tags as chips read-only — the
"Manage in Alerts → Tags" link points back here.

#### Step 3 — write a rule selector

In the rule form, **Host selector (wins over name)** renders one
dropdown per inventory key. Pick a value per key you want to constrain
on — the selector is the AND of every picked pair, e.g. `tier=critical,env=prod`.

| Rule | Selector | Target |
|---|---|---|
| prod 60% | `tier=prod` | every host tagged `tier=prod` |
| ops critical | `tier=prod,team=ops` | prod hosts owned by ops |
| dead db | `role=db` (with metric=`offline`) | any DB host that stops reporting |

When the selector field is non-empty it **wins** over the name field
(the UI greys out names so you can see which is active). Empty selector
falls back to the name field (which itself accepts exact, comma list,
or glob — see idioms 1 + 2 above). A free-text textarea is available
under the dropdowns for power users / scripted edits.

If you previously typed a tag pair that no longer exists in the
inventory (e.g. a value got deleted), the rule editor shows it as a
**red strike-through pill** so you can spot and remove it. New `host_tags`
writes via `PUT /api/hosts/{id}/tags` are validated against the
inventory and rejected with `ErrTagNotInInventory` if the pair isn't
registered — same guarantee for scripted callers.

**Multi-select shortcut**: even without tags, the rule form's name
input lets you tick agents from a checklist. Lumen stores the result as
a comma-separated list (e.g. `web-1,db-2,api-3`), so the same value can
also be hand-typed or generated by an external script.

Tags + selectors are also the planned grounding for Public API
`host_scope` (a key can only see hosts matching a selector) — building
both on the same primitive keeps alerting and RBAC consistent.

> **Rename / migrate a tag key**: not supported in v0.4.0. To rename
> `tier` → `level`, create `level` with the same values, re-assign hosts
> to the new key in the right pane, then delete the old `tier` (cascade
> rewrites the rule selectors automatically).

## Notification channels

Each channel has a **Minimum severity** dropdown (`info` / `warning` /
`critical`). The channel only receives events whose severity is at least
that rank — useful when one channel is a high-volume Slack room (`info`)
and another is a pager (`critical`). Combined with per-rule routing, you
can set up:

- A loud channel that receives everything (`info`).
- A quiet pager channel scoped to `critical` only, linked from your
  most-important rules.

All four channels are POST destinations:

### ntfy

Create an ntfy topic at [ntfy.sh](https://ntfy.sh/) (or self-hosted) and
point the channel URL at the topic, e.g.
`https://ntfy.sh/lumen-alerts-7c8b`.

Lumen sets:

- `Title` – `[SEVERITY] <rule> · <host>`, or `RESOLVED · <rule> · <host>`.
- `Priority` – critical → `urgent`, warning → `high`, info → `default`;
  resolved → `default`. Override per-channel via the **Priority** field.
- `Tags` – severity-mapped emoji (`rotating_light`, `warning`, etc.).

### Discord

Create a channel webhook in **Server Settings → Integrations → Webhooks**
and paste the full URL. Lumen sends a plain-text message with an emoji
prefix (`:rotating_light:`, `:warning:`, `:white_check_mark:`). Embeds
land in Milestone B+.

### Telegram

Telegram channels send via the [Bot API](https://core.telegram.org/bots/api),
not a webhook URL.

1. Talk to [@BotFather](https://t.me/BotFather) → `/newbot` → copy the
   **bot token** (`123456789:ABC...`). This is the channel's secret —
   anyone with it can post as your bot.
2. Add the bot to the target group/channel (or DM it once from your
   account). For a group: open the group → Settings → Add member → search
   the bot. For a channel: Channel info → Administrators → Add → search
   the bot → enable "Post messages".
3. Get the **chat id**. Easiest path: send any message to the chat, then
   open `https://api.telegram.org/bot<TOKEN>/getUpdates` in a browser
   and read `result[].message.chat.id`. Group ids start with `-100…`;
   a personal chat is a positive integer. You can also paste
   `@channelusername` for a public channel.
4. In Lumen, **Channels → New channel → Telegram**, paste the bot token
   and chat id, set **Minimum severity**, click **Send test**.

Lumen sends with `parse_mode=HTML` and prefixes the message with an
emoji + severity tag (`🚨 <b>CRITICAL</b> …`). The bot token is **shown
masked after save**; when you edit the channel later, leave the masked
field untouched to keep the stored token, or paste a new value to rotate.

**Troubleshooting** — common Telegram errors surfaced by the **Send test** button:

| Error | Meaning | Fix |
|---|---|---|
| `403 Forbidden: the bot can't send messages to the bot` | The `chat_id` points to the bot's own id. | Use a real destination: your user id (after `/start`), a group id (`-100…`), or `@channelusername`. |
| `403 Forbidden: bot was kicked from the group chat` | Bot was removed from the group. | Re-add the bot to the group. |
| `403 Forbidden: bot is not a member of the channel chat` | Bot isn't an admin of the channel yet. | Channel info → Administrators → Add → search bot username → enable "Post messages". |
| `400 Bad Request: chat not found` | `chat_id` is wrong / typo. | Re-fetch via `https://api.telegram.org/bot<TOKEN>/getUpdates` after sending a message in the target chat. |
| `401 Unauthorized` | Bot token is wrong or revoked. | Regenerate via @BotFather → `/token`. |

Quick sanity check without Lumen: paste this in a browser —

```
https://api.telegram.org/bot<TOKEN>/sendMessage?chat_id=<CHAT_ID>&text=hello
```

If it returns `"ok":true`, your bot/chat combo is healthy and the same values will work in Lumen.

### Generic webhook

Any HTTPS endpoint that accepts a `POST` with a JSON body:

```json
{
  "rule_id": 42,
  "rule_name": "CPU hot",
  "host": "webA",
  "metric": "cpu_pct",
  "severity": "warning",
  "state": "firing",
  "value": 93.4,
  "threshold": 80,
  "message": "CPU hot · cpu_pct on webA is 93.40 (threshold 80.00)",
  "time": "2026-05-29T08:30:00Z"
}
```

HMAC signing is **not** added in Milestone A — it ships with the Public
API webhook unification. Until then, treat the URL itself as the secret
and restrict the endpoint with a network ACL where possible.

### Test button

Each channel has a **Send test** action that dispatches a synthetic
`firing` event (`rule_name: "Lumen test"`, severity `info`). Useful when
you change the channel URL or want to verify routing without waiting for
a real breach.

## How the engine runs

- Evaluation cadence is read from the `alerts.eval_interval` settings
  row every tick (heartbeat pattern). The default is `15s`; change it
  with `LUMEN_HUB_ALERT_INTERVAL` at boot or by writing the settings row
  at runtime. Lower values use more CPU; higher values delay firing.
- Engine state (the per-(rule, host) pending/firing/resolved map) lives
  in memory and is **rebuilt on hub restart**. The breach condition
  re-detects within one tick, so a true firing event stays firing across
  a restart — only the original `pendingSince` is lost.
- The persisted `alert_events` table is the source of truth for the
  **Active** and **History** tabs and survives restarts.

## Verifying end-to-end

1. **Channels → New channel → ntfy**, URL `https://ntfy.sh/<your-topic>`,
   then subscribe to that topic in the [ntfy app or web](https://ntfy.sh/app).
2. Click **Send test** — the push arrives within a few seconds.
3. **Rules → New rule**: `cpu_pct gt 50 for 0s`, host blank.
4. On any host, load CPU: `yes > /dev/null` for a few seconds.
5. Within ~15 seconds the Active tab shows a `firing` row and the same
   notification arrives on the channel.
6. Stop the CPU load (`Ctrl+C`). The next tick records a `resolved` row
   and a follow-up push.

## Delivery queue, retry, and burst handling

Lumen does not call the channel webhook inline while evaluating a rule.
Every (alert × channel) gets persisted as a `pending` row in the
`notification_deliveries` table; a background worker pool then dispatches
them in parallel with **per-channel serialisation** — one in-flight HTTP
call per channel at a time, so a Discord webhook can't 429 itself and
the engine ticker is never blocked.

**The Deliveries tab** (Alerts → Đã gửi) shows every attempt:

- Counters at the top: how many pending / in-flight / sent / failed /
  dropped right now. Useful for a quick "is my pager being delivered?"
  glance.
- Filters: status, severity.
- Each row: channel, severity badge, attempt count, HTTP status, last
  error if any, queued/sent timestamps, and a **Retry now** button for
  failed/dropped rows after you fix the underlying channel.

### Severity-aware retry

A 6-hour retry on a `critical` alert is worse than no retry — by the
time it lands, the incident is over and the operator either fixed it
through another channel or has gone home. So Lumen retries by tier:

| Severity | Max attempts | Backoff schedule |
|---|---|---|
| `critical` | 4 | 5 s · 15 s · 1 min · 5 min |
| `warning`  | 6 | 30 s · 2 min · 10 min · 1 h · 2 h · 4 h |
| `info`     | 6 | 30 s · 2 min · 10 min · 1 h · 2 h · 4 h |

Critical fails fast; warning/info hang on for hours. Workers also drain
critical rows ahead of lower-severity ones during a burst — so even if
the queue is hundreds deep, a paging alert jumps the line.

### Status meaning

| Status | What it means | What to do |
|---|---|---|
| `pending` | Queued, waiting for a worker (next_retry_at past). | Nothing — the worker will pick it up. |
| `inflight` | A worker is currently sending it. | Nothing. If stuck > 30 s, check hub logs. |
| `sent` | 2xx from the channel. | Done. |
| `failed` | Exhausted retries (severity envelope). | Fix the channel URL/token, click **Retry now**. |
| `dropped` | Channel was deleted or disabled between enqueue and dispatch. | Re-route the rule. |

### Bursts

100 firing rules × 3 channels = 300 deliveries. Lumen inserts those 300
rows in well under a second (SQLite WAL); workers drain at roughly
4 deliveries/sec sustained (default `Workers=4` × `PollInterval=1s`),
so a 300-row burst clears in ~75 s. The engine itself is unaffected and
keeps evaluating at its 15 s tick.

If the queue grows past 10 000 pending rows the hub logs a warning so
the operator knows throughput is the bottleneck. Tunable knobs (worker
count, poll interval, backoff schedule) live in the dispatcher config
today; they'll be exposed in Settings → Runtime in a follow-up.

## Retention

`alert_events` and `notification_deliveries` would otherwise grow
unbounded — a chronic flap can generate hundreds of rows per day. The
retention loop (same heartbeat that prunes snapshots) sweeps both
tables on the cadence configured in **Settings → Retention**:

- **Resolved alerts** older than `retention.delete_alerts_after`
  (default **30 days**) are deleted. **Still-firing events are kept
  regardless of age** — operators always see active breaches in the
  History tab.
- **Terminal deliveries** (`sent`, `failed`, `dropped`) older than the
  same window are deleted. `pending` and `inflight` rows are never
  touched — the dispatcher is still working on them.

Set the window to `0s` (or the env override `LUMEN_HUB_RETENTION_ALERTS_WINDOW=0`)
to disable the sweep. Bounds: 1h ≤ window ≤ 365d. Sweep cadence is the
same `retention.interval` knob that drives the snapshot prune.

## What's not in v0.4.x

- **Email (SMTP)** channel.
- **HMAC signing** on the webhook channel — lands with the
  [Public API module](../../reference/api/) webhook unification.
- **Cool-down / flap suppression** beyond the per-rule `for_seconds`.
- **Derived / rate metrics** (e.g. "RAM grew >10%/min").
- **Tag key rename** — recreate with the new name + re-assign + delete
  the old one (cascade rewrites rule selectors automatically).

Watch [RFC 0001](https://github.com/quanla93/lumen/blob/main/docs/rfcs/0001-alerting.md)
for the next milestone.
