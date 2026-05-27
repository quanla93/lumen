-- +goose Up
ALTER TABLE hosts ADD COLUMN system_os TEXT;
ALTER TABLE hosts ADD COLUMN system_hostname TEXT;
ALTER TABLE hosts ADD COLUMN system_primary_ip TEXT;
ALTER TABLE hosts ADD COLUMN system_kernel TEXT;
ALTER TABLE hosts ADD COLUMN system_arch TEXT;
ALTER TABLE hosts ADD COLUMN system_cpu_model TEXT;
ALTER TABLE hosts ADD COLUMN system_uptime_seconds INTEGER;
ALTER TABLE hosts ADD COLUMN agent_version TEXT;
ALTER TABLE hosts ADD COLUMN metadata_updated_at DATETIME;

-- +goose Down
ALTER TABLE hosts DROP COLUMN metadata_updated_at;
ALTER TABLE hosts DROP COLUMN agent_version;
ALTER TABLE hosts DROP COLUMN system_uptime_seconds;
ALTER TABLE hosts DROP COLUMN system_cpu_model;
ALTER TABLE hosts DROP COLUMN system_arch;
ALTER TABLE hosts DROP COLUMN system_kernel;
ALTER TABLE hosts DROP COLUMN system_primary_ip;
ALTER TABLE hosts DROP COLUMN system_hostname;
ALTER TABLE hosts DROP COLUMN system_os;
