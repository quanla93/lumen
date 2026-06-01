-- +goose Up
-- system_virt_type holds gopsutil's host.InfoStat.VirtualizationSystem
-- ("kvm", "lxc", "docker", "vmware", "wsl", …) when the agent runs in
-- a guest, or empty/NULL on bare metal. The frontend uses it to hide
-- the per-core CPU chart on guest hosts, where the data either
-- reflects the hypervisor's shared cores (LXC) or vCPUs that don't
-- map 1:1 to physical cores (VM) — both misleading as a "per-core
-- breakdown for this agent".
ALTER TABLE hosts ADD COLUMN system_virt_type TEXT;

-- +goose Down
ALTER TABLE hosts DROP COLUMN system_virt_type;
