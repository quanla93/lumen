-- +goose Up
ALTER TABLE hosts ADD COLUMN system_hostname TEXT;
ALTER TABLE hosts ADD COLUMN system_primary_ip TEXT;

-- +goose Down
ALTER TABLE hosts DROP COLUMN system_primary_ip;
ALTER TABLE hosts DROP COLUMN system_hostname;
