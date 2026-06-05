-- +goose Up
-- Backup feature (RFC 0001). No new table — the operator-facing config
-- lives in the existing key/value settings table. This migration seeds
-- the default row for each key so the Settings UI can render placeholder
-- fields and so callers can rely on every key being present after a
-- fresh install. Existing rows are left alone (INSERT OR IGNORE).
--
-- Sensitive values:
--   backup.s3_secret_key_enc — AES-GCM ciphertext keyed off LUMEN_HUB_SECRET
--                              (same pattern as auth.oidc_client_secret_enc).
--   backup.passphrase_hash   — Argon2id hash of the operator passphrase,
--                              stored so the CLI can surface "wrong
--                              passphrase" cleanly without keeping the
--                              passphrase itself.
INSERT OR IGNORE INTO settings (key, value) VALUES
    ('backup.enabled',             'false'),
    ('backup.target',              'local'),
    ('backup.local_path',          ''),
    ('backup.s3_endpoint',         ''),
    ('backup.s3_region',           'auto'),
    ('backup.s3_bucket',           ''),
    ('backup.s3_prefix',           'lumen/'),
    ('backup.s3_access_key',       ''),
    ('backup.s3_secret_key_enc',   ''),
    ('backup.s3_force_path_style', 'false'),
    ('backup.passphrase_hash',     ''),
    ('backup.cron',                '0 2 * * *'),
    ('backup.retain_last',         '14');

-- +goose Down
DELETE FROM settings WHERE key LIKE 'backup.%';
