---
title: Public status page
description: "Publish a read-only status page at /status that anyone can visit (no login). Per-host opt-in; default hidden."
sidebar:
  order: 7
---

The public status page is a single read-only route — `https://<your-hub-url>/status` — that anyone can visit without signing in. It shows the hosts you opt in plus their up/stale/down state and live CPU/RAM/disk. Use it for a small team's "is the lab still alive" link or a public homelab status board.

**Defaults are safe.** New deployments do not publish anything. You have to (a) flip the global toggle and (b) tick at least one host before any name leaves the dashboard.

## What the page shows

Per opted-in host, the page renders:

- A state dot — `up` (fresh snapshot), `stale` (last snapshot older than 30s), `down` (the host is known but has nothing in memory), or `unknown` (newly created host, never seen).
- The host's name (the same string you typed in **Settings → Hosts**).
- Last-seen timestamp in the visitor's local timezone.
- Three live numbers — CPU%, RAM%, Disk% — when state is `up` or `stale`.

Hosts in `down` or `unknown` state render with the dot + name + last seen only, so a visitor can tell "the host exists, it's just not reporting right now" from "no host at this URL".

The page polls `/api/public/status` every 15 seconds. There is no WebSocket — the page is meant to be left open for casual checking, not pushed to.

## Configure

Sign in as admin, open **Settings → Status page**:

1. Tick **Publish the status page**. You can save in either state; the rest of the form is required only when published.
2. Set a **Title** (default: `Status`) and an optional **Description** (one short sentence is plenty).
3. Tick **Show on /status** next to each host you want to expose. Hosts in this list aren't filtered any further — every opted-in host is visible regardless of tags, rules, or silence state.
4. Save. Visit `https://<your-hub-url>/status` to confirm.

The settings live in the same `settings` SQLite table as everything else; per-host opt-in is a `public_visible` column on `hosts` added in migration `0018`.

## What's NOT shown

To avoid surprising leaks, the public page deliberately omits:

- The host's IP / hostname / OS / kernel / agent version (all those still appear on the admin-only host detail page).
- Tags. The page never reveals a host's tag set, so naming hosts after their role + tagging them with PII is safe.
- Alert rules + recent alert events. The status page is "is the host alive right now", not a partial alerts dashboard.
- Container telemetry. Container names and resource usage are admin-only.
- Charts. Only current values, never time series.
- API keys, settings, the rest of the `/api/*` surface — those all require a session.

If you need any of the above on a public page, run a real status-page tool in front of Lumen and let it scrape `/api/v1/hosts` with a scoped API key. That keeps Lumen's public surface narrow.

## Hide / unpublish

Open **Settings → Status page**, untick **Publish the status page**, save. The page immediately returns `{enabled: false}` and the public URL renders a one-line "This status page isn't published" notice. Per-host visibility stays saved so you can republish without re-ticking everything.

To fully clear the rows:

```sql
DELETE FROM settings WHERE key LIKE 'public_status.%';
UPDATE hosts SET public_visible = 0;
```

## Caching, rate limit, abuse

The handler sets `Cache-Control: no-store` so a stale CDN edge can't pin a sensitive snapshot longer than you intended. There is no per-IP rate limit yet — the expected audience is small. If you front Lumen with a reverse proxy that supports a rate limit (nginx `limit_req`, Caddy `rate_limit`, Cloudflare rules), apply it to `/status` and `/api/public/status` first; the rest of `/api/*` is already session/API-key gated.

## Troubleshooting

**`/status` shows "Status page is unreachable"**
: The frontend couldn't reach `/api/public/status` at all. Check the hub is actually up and your reverse proxy isn't blocking the path. Try `curl https://<hub-url>/api/public/status` directly.

**`/status` shows "This status page isn't published"**
: The global toggle in **Settings → Status page** is off, or the row was deleted from the `settings` table.

**Host appears but state stays `unknown` even though it's running**
: The agent isn't actually reaching the hub — check `docker logs lumen-agent-*` for `ingest failed`, and verify the agent's `LUMEN_HUB_URL` from inside the agent's network. The status page reflects whatever the dashboard reflects.

**State flips between `up` and `stale`**
: The agent's collection interval is comparable to the 30s stale threshold. Tighten the interval (Settings → Runtime → Agent collection interval) or accept the flicker.
