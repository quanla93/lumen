---
title: GPU monitoring
description: Per-GPU utilization, memory, and temperature for NVIDIA (nvidia-smi) and AMD (rocm-smi) hosts. Alerts: gpu_util, gpu_temp, gpu_mem_pct.
---

Lumen can read per-GPU utilization, memory, and temperature for
hosts with a discrete GPU. Useful for homelabs running Plex/Jellyfin
transcoding, local LLM inference, or anything else GPU-bound. Multi-GPU
per host is supported — the alerts engine fires on the **worst-of**
value across the host's GPUs (documented below).

Two backends are supported:

- **NVIDIA** via `nvidia-smi` (any driver version that supports the
  `--query-gpu=...` flags — CUDA ≥ 8.0 / driver ≥ 390).
- **AMD** via `rocm-smi --json` (ROCm ≥ 5.0; older ROCm emits different
  field names and gets a debug log + empty slice).

Hosts without a discrete GPU (typical Intel/AMD CPU servers, macOS,
Raspberry Pi) simply don't have a GPU section in the host detail page
— silent no-op, no error log spam.

## Install the GPU tooling

| Platform | Install |
|---|---|
| Debian / Ubuntu (NVIDIA) | `apt install nvidia-driver-XXX` (where XXX matches the GPU) + reboot. `nvidia-smi` ships with the driver. |
| Debian / Ubuntu (AMD) | `apt install rocm-smi` (ROCm ≥ 5.0). |
| Fedora / RHEL (NVIDIA) | `dnf install kmod-nvidia` + reboot. |
| Fedora / RHEL (AMD) | ROCm repo + `dnf install rocm-smi`. |
| Proxmox LXC (NVIDIA) | The container needs `/dev/nvidia*` + `/dev/nvidiactl` + `/dev/nvidia-uvm` mounted (auto-detected if you pass through a GPU). |
| Docker (NVIDIA) | Use the `nvidia/cuda` base image or the `--gpus all` flag; mount `/dev/nvidia*` into the agent container. |
| Docker (AMD) | Mount `/dev/dri` + install `rocm-smi` in the agent image. |

After install, verify from the agent host:

```bash
nvidia-smi --query-gpu=index,name,utilization.gpu,memory.used,memory.total,temperature.gpu --format=csv,noheader
# or
rocm-smi --json | head -50
```

If the binary is on `$PATH` and the user the agent runs as can read
the GPU device files, the host detail page picks up the data on
the next agent tick (default 5 s).

## What the agent sends

One `api.GPUInfo` per physical GPU:

```json
{
  "index": 0,
  "name": "NVIDIA GeForce RTX 4090",
  "util_pct": 35,
  "mem_used_mb": 1024,
  "mem_total_mb": 24576,
  "temp_c": 41
}
```

GPU data is **live-only** — same lifecycle as `CpuPerCore` and
`Containers`. No SQLite rows; the host detail page reads it from
the in-memory store + WS broadcast.

## Alerts

Three new metric types (RFC 0003):

| Metric | Comparison semantics | Example rule |
|---|---|---|
| `gpu_util` | Worst-of util% across host's GPUs | `gpu_util > 90 for 5m` |
| `gpu_temp` | Worst-of temp °C | `gpu_temp > 80 for 1m` |
| `gpu_mem_pct` | Worst-of mem_used / mem_total % | `gpu_mem_pct > 90 for 5m` |

The "worst-of" choice is intentional — a homelab with two GPUs
doing parallel inference wants a single alarm when either is
overloaded, not two parallel alerts to the same channel. Per-GPU
alert overrides land in a follow-up if anyone asks.

## Docker container caveats

NVIDIA: the agent container must see the device files. The
generated per-agent Docker Compose (Settings → Hosts → Token
reveal) does NOT auto-mount `/dev/nvidia*` — operators running
GPU passthrough need to add a `devices:` block to the compose
file before `docker compose up -d`. If the agent container can't
see the GPU, the host detail page just shows no GPU section —
silent, no error.

AMD: same, except `/dev/dri/renderD128` + `/dev/kfd` instead of
`/dev/nvidia*`.

## Troubleshooting

### Host detail page shows "no GPU" on a host with an NVIDIA card

The agent's user can't read `/dev/nvidia*` (Linux), or the
container doesn't have them mounted. Check:

```bash
sudo -u lumen-agent nvidia-smi
# or from inside the container
docker exec lumen-agent nvidia-smi
```

Permission denied → fix device permissions or run the agent as a
user in the `video` group (most distros).

### nvidia-smi not found in $PATH

The agent's `PATH` doesn't include the binary's directory. The
most common case is the agent running inside a Docker container
without the NVIDIA driver baked in. Fix: use the `nvidia/cuda`
base image (which ships `nvidia-smi` + the driver userspace) or
mount the host's `/usr/bin/nvidia-smi` into the container.

### rocm-smi JSON parse error

Driver version is older than ROCm 5.0 and emits different field
names. The agent logs a `gpu sample failed` debug line on the
first failure. Upgrade to ROCm ≥ 5.0 (or downgrade your version
expectation — Lumen doesn't backport to older keys).

### Multi-GPU alert fires when only one GPU is hot

By design. "Worst-of" is the v1 semantics. Per-GPU rules are a
follow-up.
