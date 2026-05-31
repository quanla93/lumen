-- +goose Up
-- v0.4.5: per-host alert silence (maintenance window).
--
-- silenced_until is a unix timestamp in seconds. When non-null and in
-- the future, the alert engine skips both the event row and the
-- delivery queue for any transition targeting this host — operator
-- can run `docker compose pull && up -d` without generating noisy
-- "offline → resolved" pairs.
--
-- NULL = not silenced. Past timestamps = silence already expired, also
-- treated as not silenced (lazy expiry; no sweep job needed since the
-- check is per-evaluation).
ALTER TABLE hosts ADD COLUMN silenced_until INTEGER;

-- +goose Down
ALTER TABLE hosts DROP COLUMN silenced_until;
