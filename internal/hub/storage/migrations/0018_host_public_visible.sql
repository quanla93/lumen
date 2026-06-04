-- +goose Up
-- public_visible toggles per-host opt-in for the public status page
-- (/status). Default 0 (hidden) so existing fleets stay private; the
-- admin checks the box per-host in Settings → Status. The public
-- handler reads (public_status.enabled && public_visible) to decide
-- what to expose — no live filtering at query time beyond a single
-- WHERE clause keeps the page snappy even at fleet sizes.
ALTER TABLE hosts ADD COLUMN public_visible BOOLEAN NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite ALTER TABLE DROP COLUMN added in 3.35+ which goose uses fine.
ALTER TABLE hosts DROP COLUMN public_visible;
