-- +goose Up
-- No-op: 0006_host_system_metadata.sql already added system_hostname and system_primary_ip.
SELECT 1;

-- +goose Down
SELECT 1;
