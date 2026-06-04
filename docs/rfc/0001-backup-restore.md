# RFC 0001 — Backup + restore (local / S3)

- **Status**: Draft
- **Sprint**: Phase 8 Sprint 1
- **Effort**: 5 days
- **Author**: 2026-06-04 planning session

## Motivation

A self-hosted hub holding 30-90 days of metrics + alert rules + user accounts + API keys + OIDC config is exactly the thing operators forget to back up until the SSD fails. Lumen has no built-in path today; admins fall back to "cron + sqlite3 .backup + rclone". That's brittle (no encryption, no retention, no documented restore) and several user-facing features (API keys, OIDC client secret, VAPID private key) are already encrypted at rest with `LUMEN_HUB_SECRET` — meaning a naive file copy is useless if the operator loses the secret separately.

A first-class backup feature lets us:
- Encrypt the snapshot with an operator-chosen passphrase (separate from `LUMEN_HUB_SECRET`), so backup is intelligible even if the hub host is destroyed.
- Push to S3-compatible storage (AWS / MinIO / Cloudflare R2 / Backblaze B2 / Wasabi) without an external script.
- Run on a schedule with retention.
- Offer a restore flow that doesn't require the operator to remember internal file layouts.

## Scope

**In scope**
- One snapshot file per backup, containing `lumen.db` only (the SQLite database — all settings, hosts, rules, users, OIDC config, web push subscriptions, etc. live here).
- Two targets selectable per deployment: local filesystem path **or** S3-compatible bucket.
- Cron-style schedule (custom expression) + manual "Backup now" button.
- Per-deployment passphrase + AES-256-GCM encryption with key derived via Argon2id (parameters tuned to ≥1 s on a Raspberry Pi 4).
- Retention: keep last N (default 14, configurable).
- Settings → Backup UI for configuration + recent backups list.
- Restore: CLI flag `lumen-hub --restore=<path>` (canonical, safe) **and** Web UI "Restore from S3" (convenience, documented caveats).
- Docs: `docs/configure/backup.md` with MinIO + R2 + B2 + restore runbooks.

**Out of scope**
- Multi-version backup format / format migration. v1 only.
- WAL/checkpoint archiving for point-in-time recovery. (Operators wanting PITR can run `sqlite3 .recover` themselves.)
- Backup of the embedded web bundle / installed agent binaries (`/opt/lumen-hub/install/`) — those are reproducible from the release.
- Backup of operator-mounted Docker volumes outside `lumen.db` (logs, caches).
- Hot-swap restore without restart. Documented as "stop the hub first; the Web UI button is convenience and may race a live writer".

## Design

### Migration

`internal/hub/storage/migrations/0020_backup_settings.sql` — no new tables; just seed default settings keys (the generic `settings` k/v table already exists).

Keys:

| Key | Default | Description |
|---|---|---|
| `backup.enabled` | `false` | Master switch — disables scheduler + UI button. |
| `backup.target` | `local` | `local` \| `s3` |
| `backup.local_path` | `""` | Absolute filesystem path; created if missing. |
| `backup.s3_endpoint` | `""` | e.g. `https://s3.amazonaws.com`, `https://<acct>.r2.cloudflarestorage.com`, `http://minio.local:9000`. |
| `backup.s3_region` | `auto` | Mostly ignored by S3-compatible providers; AWS requires real region. |
| `backup.s3_bucket` | `""` | |
| `backup.s3_prefix` | `lumen/` | Object-key prefix. Trailing slash recommended. |
| `backup.s3_access_key` | `""` | |
| `backup.s3_secret_key_enc` | `""` | AES-GCM encrypted with hub secret (same KEK pattern as OIDC client_secret). |
| `backup.s3_force_path_style` | `false` | Set true for MinIO / older endpoints. |
| `backup.passphrase_hash` | `""` | Argon2id hash of the passphrase, stored to detect "operator typed wrong passphrase on restore" without storing the passphrase itself. |
| `backup.cron` | `0 2 * * *` | Standard 5-field cron. Default: every day at 02:00 server-local. |
| `backup.retain_last` | `14` | Keep last N backups; older swept after each successful new backup. |

### Package layout

`internal/hub/backup/`

- `backup.go` — `Plan` struct, `RunNow(ctx)`, `Run(ctx)` (scheduler entry).
- `crypto.go` — passphrase → key derivation (Argon2id, fixed salt prefix + per-backup random salt embedded in header), AES-256-GCM seal/open, header struct.
- `target_local.go` — write to filesystem path.
- `target_s3.go` — `aws-sdk-go-v2` with `BaseEndpoint`, `PathStyle` toggle, multi-part upload off (snapshots stay <100 MB for any reasonable fleet; single PUT is fine).
- `snapshot.go` — `VACUUM INTO` against the live DB into a temp file.
- `scheduler.go` — `robfig/cron/v3` wrapper, hot-reload on settings change.
- `retention.go` — list, sort, delete excess.
- `restore.go` — open file → header parse → key derive → decrypt → integrity-check SQLite → rename.
- `handlers.go` — `GET/PUT /api/settings/backup`, `POST /api/backup/run`, `GET /api/backup/list`, `POST /api/backup/restore/{name}` (Web UI restore).

### Backup file format

```
LUMEN_BAK\x00       (10 bytes magic — "LUMEN_BAK" + null)
\x01                (version: 1)
[16 bytes salt]     (Argon2id salt, random per backup)
[12 bytes nonce]    (AES-GCM nonce, random per backup)
[ciphertext]        (AES-256-GCM over gzipped SQLite snapshot)
```

No per-row metadata. The header is small enough to inspect with `dd if=backup.bin bs=1 count=39 | xxd`.

### Argon2id parameters

Tuned for ~1 s wall time on a Raspberry Pi 4 (the slowest realistic operator hardware):

- `time = 3`
- `memory = 64 * 1024` (64 MiB)
- `threads = 4`
- `keyLen = 32`

Operators on bigger hardware get faster key derivation; nobody benefits from making the floor lower.

### Endpoints

| Verb + path | Auth | Body / response |
|---|---|---|
| `GET /api/settings/backup` | session | Returns config (secret_key field replaced by `has_secret_key: bool`). |
| `PUT /api/settings/backup` | session | Saves config; empty `s3_secret_key` keeps the existing one (same UX as OIDC). |
| `POST /api/settings/backup/test-target` | session | Lists 1 object / dials the target — surfaces connectivity errors before saving. |
| `POST /api/backup/run` | session | Synchronous trigger. Returns the created backup's filename + size + duration. |
| `GET /api/backup/list` | session | Returns array of `{name, size_bytes, created_at, target}` sorted newest first. |
| `POST /api/backup/restore/{name}` | session | Web UI restore — downloads + decrypts to staging path, queues self-restart via SIGHUP. Documented "use CLI for production". |

### CLI flag

`lumen-hub --restore=<file>` (mutually exclusive with normal run):

1. Open the file, read the header (magic + version + salt + nonce).
2. Prompt for passphrase on stdin (read with `golang.org/x/term`, no echo) if not provided via `LUMEN_HUB_BACKUP_PASSPHRASE` env var.
3. Argon2id with the embedded salt.
4. AES-GCM decrypt.
5. gunzip into a temp file.
6. SQLite integrity check (`PRAGMA integrity_check`).
7. If the live `lumen.db` exists, rename it to `lumen.db.before-restore-<timestamp>` (don't delete — give the operator an undo).
8. Rename the restored temp file to `lumen.db`.
9. Exit 0 with a one-line "restored from <name> (created <ts>)".

The CLI does NOT start the server after restore — operator restarts the service manually. That avoids "did the restore actually succeed?" ambiguity.

### Web UI restore

- "Restore" button next to each entry in the recent backups list.
- Confirmation modal: "This will replace the current database. The hub will restart. Are you sure?"
- On confirm: hub downloads + decrypts + integrity-checks to staging path, then sends `SIGHUP` to itself (handler does `os.Exec` to relaunch with `--restore=<staging>`).
- Caveat documented: race window if a write is in-flight at restart. Production restore should use CLI with the service stopped.

### Frontend

`web/src/components/Settings.tsx` adds a new tab "Backup" with:
- Master switch (enable).
- Target select (local / s3).
- Conditional fields per target.
- Passphrase + confirm-passphrase inputs (write-only; `has_passphrase: bool` for "already set" UX).
- Cron expression input with a small example legend (`0 2 * * *` = daily 02:00; `0 */6 * * *` = every 6 hours; `0 3 * * 0` = weekly Sunday 03:00).
- Retention N.
- "Test target" button → POST `/api/settings/backup/test-target`.
- "Backup now" button → POST `/api/backup/run` (disabled while running).
- Recent backups list with size, age, target, and a "Restore" + "Download" button per row.

i18n: inline English (admin-only surface, same precedent as SSO settings tab); future PR can promote to `messages.ts`.

### Dependencies

- `aws-sdk-go-v2` (already on supply-chain radar; pin to v1.x).
- `robfig/cron/v3` (small, well-maintained).
- `golang.org/x/crypto/argon2` (already in the module — used by `auth.password.go`).
- `golang.org/x/term` (CLI passphrase prompt).

### Wiring

`cmd/lumen-hub/main.go` adds:
- Parse `--restore` flag before any other startup work; if set, run restore mode and exit.
- After normal startup, start `backup.Scheduler` if `backup.enabled` is true.

`internal/hub/server/server.go` wires `backupHandlers` into the session-protected route group.

## Risks

| Risk | Mitigation |
|---|---|
| Operator loses passphrase → backup is unrecoverable | UI displays a one-time warning on first save: "Passphrase is non-recoverable. Save it in your password manager now." `passphrase_hash` lets the CLI return "wrong passphrase" cleanly instead of garbage data. |
| `LUMEN_HUB_SECRET` rotation invalidates stored `s3_secret_key_enc` | Same constraint as OIDC client_secret; documented in `docs/configure/backup.md` § "Rotating LUMEN_HUB_SECRET". |
| S3 endpoint typo silently fails for hours until cron fires | "Test target" button. Scheduler also surfaces consecutive failures as a warn-level log + a hub-level alert event. |
| Hot-swap restore races a live writer → DB corruption | UI confirmation modal makes the risk explicit. Documentation pushes operators to CLI for production. Test target lockfile semantics in integration test. |
| Argon2id 64 MiB OOMs on a really small VPS | Document minimum hub host = 256 MiB RAM. Same floor we already imply elsewhere. |
| `VACUUM INTO` on a hub under heavy ingest takes minutes | Snapshot happens off the request path; scheduler waits for the previous run to finish before starting the next. Document expected snapshot time per fleet size. |
| CLI restore overwrites a live `lumen.db` if operator forgot to stop the service | Pre-flight check: refuse to restore if a `lumen.db-wal` file is newer than 5 s (recent write). Operator can `--force` past this. |

## Testing

### Unit
- `crypto_test.go` — passphrase → key → encrypt → decrypt round-trip; wrong passphrase fails; tampered ciphertext fails authentication.
- `header_test.go` — magic byte mismatch rejected; version > 1 rejected with "future format".
- `retention_test.go` — given 20 backups with deterministic mtimes, retain_last=5 leaves 5 newest.
- `snapshot_test.go` — `VACUUM INTO` produces a file with PRAGMA-clean integrity.

### Integration (Docker test)
- Run hub + MinIO containers. Configure backup to MinIO. Trigger manual backup. Assert object exists in bucket. Pull object, decrypt with passphrase, verify integrity check passes.
- Trigger restore via CLI flag against a fresh hub container. Verify post-restore DB has expected rows.

### Manual smoke
- AWS S3 real account (one-off pre-merge).
- Cloudflare R2 + Backblaze B2 (one-off pre-merge).

## Docs deliverables

- `docs/configure/backup.md`:
  - Why back up (data loss scenarios).
  - Choosing a passphrase (length + storage).
  - Target setup: local path, AWS S3, MinIO, R2, B2 (full per-provider walkthrough).
  - Cron expression cheatsheet.
  - Retention semantics.
  - Restore via CLI (the canonical path).
  - Restore via Web UI (with the caveats).
  - Backup file format (operators can pry it open).
  - Troubleshooting (passphrase wrong, S3 401/403, integrity check failed).
  - Rotating `LUMEN_HUB_SECRET` and how that interacts.
- `CHANGELOG.md` entry on ship.
- `ACTION_PLAN.md` checkbox flip.

## Open questions

1. ~~Restore in CLI only vs Web UI vs both?~~ — Both (operator decision 2026-06-04).
2. ~~Credentials via env vs Settings UI?~~ — Settings UI with encrypted secret_key (operator decision 2026-06-04).
3. ~~Schedule manual + daily auto vs custom cron?~~ — Custom cron (operator decision 2026-06-04).
4. Should the scheduler back off after consecutive failures? Proposed: yes, doubling delay up to 4 h, then surface as a hub-level alert.
5. Should we expose the backup file format publicly so a third-party tool can decrypt without Lumen running? Proposed: yes — format spec section in `docs/configure/backup.md` is authoritative.

## Related

- `ACTION_PLAN.md` § "Phase 8 — Sprint queue".
- ADR 0001 (storage architecture) — defines the SQLite database file we're snapshotting.
- The OIDC client_secret encryption (`internal/hub/auth/crypto.go`) is the reference pattern for `s3_secret_key_enc`.
