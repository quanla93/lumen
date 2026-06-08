---
title: Maintenance windows
description: Schedule planned downtime so alerts matching a tag scope are suppressed while the window is active.
---

Lumen alerts can fire during planned downtime — firmware updates,
cable swaps, reboots after a kernel upgrade. The notification queue
ships them anyway because "the alert is real" is the engine's job.
**Maintenance windows** are the operator's tool for saying "this is
expected, please be quiet about it for a while."

A window has:

- A time range (start_at, end_at)
- A reason text (free-form; the operator's note to future-self)
- A tag-scope selector (e.g. `tier=db` matches every host tagged
  `tier=db`; empty scope matches all hosts)

While a window is active and a host's tag set matches the scope,
both **firing** transitions (breach detected) and **resolved**
transitions (breach cleared) are dropped for matching rules before
they reach `persistAndNotify`. The breach is still observed in the
in-memory snapshot — you just don't get a page about it.

> **Already-queued firings are not recalled.** A notification that
> the dispatcher worker has already pulled off the queue ships as
> normal. The window prevents *new* firings, not retroactive
> silence. (RFC 0003 Q1 decision.)

## How to use

**Alerts → Maintenance** tab. Pick a state filter (Active / Upcoming
/ Past / All) to navigate. The create form takes:

- **Start** / **End** — `datetime-local` inputs, browser-timezone.
  The server stores UTC; the form converts on the fly.
- **Reason** — free-form text. `"Firmware update"`, `"Cable swap"`,
  etc.
- **Tag scope** — `key=value` pairs, comma-separated. Empty = all
  hosts.

Active firings resume on the next alert tick after you cancel
(via the trash icon on the row) or after the window's end_at passes.

## Edit guards

Once a window has begun, `start_at` is locked — the operator can't
silently change which ticks the suppression applies to. The end_at
remains mutable: you can extend it (the cable swap ran long) or
shorten it (you finished early). A wrong start_at is the server's
response of `409 Conflict` so the UI shows it inline.

The Edit button on a row loads the form pre-filled; updating
without changing start_at is a no-op on the server side.

## Timezone behaviour

`datetime-local` inputs are naïve timestamps in the browser's
timezone. The JavaScript Date constructor on `new Date("2026-06-08T02:00")`
parses them as local time and `toISOString()` converts to UTC
for the server. The server stores UTC, displays UTC in the API
response, and the UI converts back to local for display via
`toLocaleString()`. A user in Tokyo who creates "2026-06-08 02:00"
sees the same wall-clock time the user in Berlin sees on their
own machine — they're both 02:00 local.

The underlying data (JSON, SQLite, alert engine comparisons) is
all UTC. The only timezone-conversion step is the UI's
`toLocaleString()` on display.

## Use cases

- **Firmware update window**: schedule the next 30 min, scope
  `tier=db`. All DB hosts get silent.
- **Cluster reboot**: scope `env=cluster-east`. East-coast hosts
  silent; west-coast hosts continue alerting normally.
- **Cable swap on a single host**: scope `host=homelab-nas`. Just
  the one host, the alerts on it don't fire.

## What the feature does NOT do

- **Recurring windows.** v1 is one-shot. Recurrence (weekly
  patching window, monthly maintenance) is a follow-up — operator
  patterns suggest v1 is enough for most homelab use.
- **Auto-acknowledge pre-window firings.** The dispatcher's queue
  ships whatever it has; the window only suppresses *new* events.
- **Per-rule windows.** v1 is per-tag-scope. Per-rule windows land
  with multi-user (Sprint 8) when rules themselves become a
  richer surface.
- **Calendar import (iCal).** Operators with established maintenance
  calendars in Google Calendar / Outlook would benefit; not in v1.
