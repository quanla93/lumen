---
title: Backup & restore
description: Encrypt and ship the hub's SQLite database to a local path or S3-compatible bucket. Restore via CLI or the Settings UI.
---

Lumen can encrypt a copy of the hub database on a schedule and ship it
to a local path or any S3-compatible bucket (AWS S3, MinIO, Cloudflare
R2, Backblaze B2, Wasabi). Restore runs either from a stopped hub
(canonical, safe path) or from the Web UI (convenience, with caveats
documented below).

> **Why bother?** The hub is the source of truth for hosts, alert
> rules, channels, API keys, OIDC config, web push subscriptions, and
> the most-recent 24 h of metrics. A single SSD failure wipes all of
> it. Operators with no backup story today are running on borrowed
> time; this feature gets you a first-class path in two minutes.

## Pick a target

Two options, selectable per deployment:

| Target | When to use | Trade-offs |
|---|---|---|
| **Local path** | The hub host has another disk / mount you trust (a separate SSD, a NAS mounted via NFS, a USB drive). | Simplest setup. Disaster resilience = whatever the host's own backup story is. |
| **S3-compatible** | You want off-machine resilience — bucket in a different room, account, or provider. | One more thing to misconfigure. Test target button exists exactly to surface 401/404/TLS errors *before* the cron tick. |

The settings row holds which one is active; only one runs at a time.

## Local target

Set **Target = Local path** and **Local path = `/var/lib/lumen-backups`**
(or wherever). The directory is created on save (mode `0755`); the
first backup creates files with mode `0600`. Retention sweep deletes
oldest files first; with `Retain last N = 14` you'll keep about 2
weeks of daily backups.

## S3-compatible target

The target needs:

| Field | AWS S3 | Cloudflare R2 | MinIO / Wasabi / B2 |
|---|---|---|---|
| Endpoint | `https://s3.amazonaws.com` | `https://&lt;acct&gt;.r2.cloudflarestorage.com` | `http://&lt;host&gt;:9000` |
| Region | `us-east-1` (your bucket's region) | `auto` | `us-east-1` (ignored) |
| Bucket | any bucket you own | any R2 bucket | any bucket |
| Prefix | `lumen/` | `lumen/` | `lumen/` |
| Access key + secret key | IAM user's keys | R2 API token | bucket user / access key |
| Path-style addressing | off | off | **on** (MinIO needs this) |

The hub calls `HeadBucket` on save and on the **Test target** button.
A 403 / 404 / TLS error surfaces immediately with a clear message
instead of "cron failed at 02:00 every day for 30 hours."

Server-side encryption is enabled by default (`AES256`) on every
PutObject — every supported S3-compatible provider accepts this. KMS
is a follow-up.

## Passphrase

The backup is encrypted with a passphrase you pick in the Web UI
**Settings → Backup → Passphrase**. The passphrase is **not stored
on the hub** — only its Argon2id hash. A future CLI restore that
can't verify the typed passphrase will surface "passphrase mismatch"
cleanly, not garbage data.

> **Save the passphrase in your password manager right after typing
> it.** Losing it means every backup is unrecoverable, even if the
> file lands safely in your bucket.

For automation (cron jobs that pre-stage a passphrase file, CI), set
`LUMEN_HUB_BACKUP_PASSPHRASE` in the hub's environment. The CLI
restore reads the env var first; falls back to a TTY prompt.

## Schedule

**Cron expression** is a standard 5-field cron. Defaults to `0 2 * * *`
(daily at 02:00 server-local). The scheduler hot-reloads the
expression on every 30 s heartbeat, so a UI change applies within
half a minute — no hub restart.

Useful starting points:

| Expression | Meaning |
|---|---|
| `0 2 * * *` | Daily at 02:00 |
| `0 */6 * * *` | Every 6 hours |
| `0 3 * * 0` | Weekly Sunday at 03:00 |
| `*/30 * * * *` | Every 30 minutes (for very small fleets / dev) |

Consecutive failures double the backoff delay up to 4 h, then
surface as a hub-level alert event — "cron silently stopped" is
explicitly the failure mode this guards against.

## Restore

Two paths, same code underneath:

### CLI (canonical, safe)

```bash
# stop the hub service first
sudo systemctl stop lumen-hub

# restore from a local file
LUMEN_HUB_DB_PATH=/var/lib/lumen/lumen.db \
LUMEN_HUB_BACKUP_PASSPHRASE='your-passphrase' \
  lumen-hub --restore=/path/to/lumen-2026-06-08T02-00-00Z.bak

# restore from a local file you downloaded from S3
LUMEN_HUB_DB_PATH=/var/lib/lumen/lumen.db \
LUMEN_HUB_BACKUP_PASSPHRASE='your-passphrase' \
  lumen-hub --restore=./downloaded-backup.bak
```

The hub does NOT start the server after restore. You see:

```
restored from lumen-2026-06-08T02-00-00Z.bak (size=184320 bytes, took=2.1s)
previous db preserved at /var/lib/lumen/lumen.db.before-restore-1717824000
hub is not restarted automatically; start the service manually to load the new db
```

The previous database is kept as `lumen.db.before-restore-<unix>` so
the operator can hand-roll back if the restore was a mistake. Start
the service manually (`systemctl start lumen-hub`) when ready.

Wrong passphrase:

```
restore failed: backup: restore: decrypt: backup: decryption failed (wrong passphrase or tampered file)
```

No data is touched. Pre-flight refuses if a `-wal` / `-shm` file is
fresher than 5 s (the hub was probably still running). Pass
`--force` to override the pre-flight.

### Web UI (convenience)

**Settings → Backup → Recent backups → Restore** next to any row.
Confirmation modal + passphrase prompt. The hub:

1. Downloads the encrypted blob from the target.
2. Decrypts + integrity-checks to a staging path.
3. Replaces the live database (preserving the previous one as
   `lumen.db.before-restore-<unix>`).
4. Sends `SIGHUP` to itself to relaunch with `--restore=<staging>`.

> **Use the CLI for production.** The Web UI races a live writer by
> design (the hub is still running when the user clicks Restore). The
> atomicity holds for "no in-flight writes" — which is most of the
> time, but not guaranteed. CLI from a stopped service is the
> canonical path.

## File format

Every backup is one self-contained file. The format is small enough
to inspect by hand:

```
LUMEN_BAK\x00        (10 bytes magic — "LUMEN_BAK" + null)
\x01                 (1 byte version: 1)
[16 bytes salt]      (Argon2id salt, random per backup)
[12 bytes nonce]     (AES-GCM nonce, random per backup)
[ciphertext]         (AES-256-GCM over gzipped SQLite snapshot)
```

Inspect with `dd if=backup.bin bs=1 count=39 | xxd` — you should see
the magic + version + first 28 bytes of header. Decrypting doesn't
require a Lumen binary running; the only secrets needed are the
passphrase + a Go-compatible Argon2id implementation. A reference
decryptor in any language is straightforward to write against this
spec.

## Rotating `LUMEN_HUB_SECRET`

The hub's session secret encrypts the **S3 secret_key** at rest (in
the `backup.s3_secret_key_enc` row). If you rotate the hub secret,
that value becomes un-decryptable and the S3 target will fail
authentication. Two ways to handle it:

1. **Before** rotating the hub secret: open **Settings → Backup**,
   re-type the S3 secret key, save. This re-encrypts the secret with
   the new key.
2. **After** rotating the hub secret (when nothing decrypts): type
   the S3 secret key again, save. The Settings page accepts the
   plaintext; encryption is automatic on save.

The **passphrase hash** (`backup.passphrase_hash`) is *not* affected
by the hub secret — Argon2id with its own per-backup salt. Rotating
the hub secret doesn't lock you out of restore.

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `Backup now` says "Target probe failed: 403" | Wrong access key, wrong secret key, or bucket policy denies the principal | Re-check the IAM / R2 token permissions, or run `aws s3 ls` from the hub host with the same credentials to isolate |
| Backup file size is ~6 KB on a hub with 50 hosts × 5 s × 24 h | Expected. SQLite VACUUM INTO produces a tight, defragmented copy; gzip makes it smaller. Numbers like 50–200 KB for a "real" hub are normal. | Nothing to do |
| Cron is on but the recent-backups list is empty 3 days later | `backup.enabled` is `false`, OR the cron expression doesn't parse (logged at INFO/ERROR), OR passphrase was cleared | Check `Settings → Backup` and the hub log for `"backup scheduler: failed to parse cron expression"` |
| Wrong-passphrase restore | The passphrase you typed doesn't match the one the hub used to encrypt | CLI doesn't have a "try again" — you need the original. This is the point of the hash. |
| `/api/backup/run` returns 401 | Session expired | Log in again |
| Web UI restore succeeds but the hub doesn't come back | The SIGHUP self-exec races the live writer | Stop the hub manually, run `lumen-hub --restore=<staging>` from the CLI |

## What the feature does NOT do

- **No WAL/PITR.** A backup is a snapshot at one moment in time.
  Operators wanting point-in-time recovery can run `sqlite3 .recover`
  themselves.
- **No format migration.** v1 format only. Future versions that
  change the layout get a new version byte in the header.
- **No backup of the embedded web bundle or installed agent
  binaries.** Those are reproducible from the release.
- **No hot-swap restore without a brief restart.** The Web UI races
  a live writer by design; use the CLI for production.
- **No multi-hub fan-out.** One hub, one target. Running multiple
  hubs means one backup per hub (each schedules its own).
