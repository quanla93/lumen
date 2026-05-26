-- +goose Up
-- Agent breadth: 5 new scalar metrics. Per-core CPU is NOT stored here —
-- it only flows live through the WS broadcast (variable cardinality
-- per host; storing it would force JSON columns or a join table for
-- modest analytical value pre-v1).
ALTER TABLE snapshots ADD COLUMN net_rx_bps REAL NOT NULL DEFAULT 0;
ALTER TABLE snapshots ADD COLUMN net_tx_bps REAL NOT NULL DEFAULT 0;
ALTER TABLE snapshots ADD COLUMN disk_r_bps REAL NOT NULL DEFAULT 0;
ALTER TABLE snapshots ADD COLUMN disk_w_bps REAL NOT NULL DEFAULT 0;
ALTER TABLE snapshots ADD COLUMN temp_c     REAL NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite ≤ 3.34 can't drop columns; ≥ 3.35 can. The bundled
-- modernc.org/sqlite ships a recent build, so DROP COLUMN works.
ALTER TABLE snapshots DROP COLUMN temp_c;
ALTER TABLE snapshots DROP COLUMN disk_w_bps;
ALTER TABLE snapshots DROP COLUMN disk_r_bps;
ALTER TABLE snapshots DROP COLUMN net_tx_bps;
ALTER TABLE snapshots DROP COLUMN net_rx_bps;
