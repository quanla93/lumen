---
title: Retention
description: How Lumen prunes old snapshots, and how to tune it for HDD-friendly throughput.
sidebar:
  order: 2
---

Every accepted ingest writes a row to the `snapshots` table. Without
retention, the table grows forever — by default each agent produces
~17k rows/day (one per 5s tick). The retention loop keeps the table
bounded by deleting rows older than a configurable window.

## Tunable knobs

| Setting | Default | Bounds | What it does |
|---|---|---|---|
| `retention_window` | `24h` | 5m – 365d | Rows with `ts < now - WINDOW` are deleted. |
| `retention_interval` | `1h` | 1m – 24h | How often the sweep runs. |

Set both via the **Settings → Retention** UI tab. Changes apply on the
next sweep — **no hub restart needed**. The retention loop polls these
values per iteration and reset its ticker if `retention_interval`
changes.

## Env defaults

`LUMEN_HUB_RETENTION_WINDOW` and `LUMEN_HUB_RETENTION_INTERVAL`
(documented in [hub.env.example](https://github.com/quanla93/lumen/blob/main/deploy/systemd/hub.env.example))
seed the settings table on **first run only**. After the row exists,
the UI value wins — editing the env file and restarting has no effect
on retention (the seed is a no-op).

To force re-seed from env: stop the hub, `DELETE FROM settings WHERE
key LIKE 'retention.%'` in SQLite, restart. The next boot will copy the
env defaults back in.

## Disable retention

Set either knob to `0`:

| `window` | `interval` | Result |
|---|---|---|
| `0` | any | No deletion (sweep runs but is a no-op). |
| any | `0` | Sweep loop is paused; polls every 60s for re-enable. |
| `0` | `0` | Completely off. |

Disabling is useful while debugging or during a backfill from another
monitoring tool. **Not** what you want long-term — the SQLite file
grows unbounded.

## How big does the DB get?

Each `snapshots` row is ~80 bytes uncompressed (the 13 metric fields
+ host + ts + indexes). Approximate sizing:

| Hosts | Ingest rate | Per day | At 24h retention |
|---|---|---|---|
| 1 | 5s | ~1.4 MiB | ~1.4 MiB steady |
| 10 | 5s | ~14 MiB | ~14 MiB steady |
| 50 | 5s | ~70 MiB | ~70 MiB steady |
| 200 | 5s | ~280 MiB | ~280 MiB steady |

WAL means writes are appended to `lumen.db-wal` and folded into
`lumen.db` periodically. The retention sweep is a single `DELETE FROM
snapshots WHERE ts < ?` — index-backed, fast, doesn't hold a long lock.

After a sweep the freed pages stay in the file (SQLite doesn't shrink
automatically). To reclaim disk:

```bash
sudo systemctl stop lumen-hub
sudo -u lumen sqlite3 /var/lib/lumen/lumen.db 'VACUUM;'
sudo systemctl start lumen-hub
```

Run `VACUUM` infrequently (monthly) — it rewrites the whole file, which
is the opposite of HDD-friendly.

## Cold-tier policy

Today retention is a hard delete. The **Settings → Downsample** tab
already stores the policy the future Parquet cold tier will use:

| Setting | Default | Bounds | What it will do |
|---|---|---|---|
| `downsample_bucket_size` | `5m` | 1m – 24h | Time span represented by one archived point. `5m` means old samples are averaged into one point every 5 minutes. |
| `downsample_hot_window` | `24h` | 1h – 30d | How long full-detail raw samples stay in SQLite. `24h` means the last day keeps every agent sample. |
| `downsample_archive_window` | `8760h` | 1d – 365d | How long compressed history is kept before deletion. `8760h` means about one year. |

When cold-tier compaction ships, rows older than the **hot** window
will be aggregated to the configured bucket size and written to
Parquet files under `/var/lib/lumen/cold/`. The query API will
transparently span hot (SQLite) + cold (Parquet) so charts keep
working without a wire-format change.

## Tuning for HDD-friendliness

For a homelab on spinning disks the bottleneck isn't capacity, it's
IOPS. The hub already uses WAL + `synchronous=NORMAL` so commits don't
fsync per ingest. Two things you can do further:

**Increase the agent tick interval.**

Default agent interval `5s` → 12 writes/min/host. Change it in
Settings → Runtime; bumping to `30s` cuts writes 6×. Charts stay
readable but the "now" cell on the dashboard lags up to 30s.

**Tighten the retention window if you don't need a full day.**

A 6h window keeps the working set in memory more reliably and shrinks
the index. Useful on Raspberry Pi-class hardware.

## Inspecting retention activity

```bash
journalctl -u lumen-hub | grep retention
```

You should see lines like:

```
INFO retention loop starting default_window=24h0m0s default_interval=1h0m0s
INFO retention sweep done deleted=17280 cutoff=2026-05-25T08:30:00Z window=24h0m0s
DEBUG retention sweep clean cutoff=...  ← when nothing aged out yet
```

Change a setting in the UI and you should see:

```
INFO retention interval changed; resetting ticker old=1h0m0s new=30m0s
```

## Reset to defaults

Drop the rows:

```bash
sudo -u lumen sqlite3 /var/lib/lumen/lumen.db \
  "DELETE FROM settings WHERE key LIKE 'retention.%';"
sudo systemctl restart lumen-hub
```

The next boot re-seeds from `LUMEN_HUB_RETENTION_WINDOW` /
`LUMEN_HUB_RETENTION_INTERVAL`.
