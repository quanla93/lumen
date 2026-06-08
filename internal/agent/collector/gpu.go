// gpu.go — Per-GPU utilisation, memory, and temperature.
//
// Supports NVIDIA via `nvidia-smi` and AMD via `rocm-smi`. Both
// binaries are looked up once at startup and cached for the rest of
// the agent's lifetime — exec.LookPath is cheap but not free, and
// the per-tick cost is dominated by the child's process startup
// anyway. Missing both binaries → empty slice + nil error (the
// common case for homelab hosts without a discrete GPU).
//
// nvidia-smi output we parse (CSV, no header):
//
//	0, NVIDIA GeForce RTX 4090, 35, 1024, 24576, 41
//
// Fields: index, name, util.gpu %, memory.used MiB, memory.total
// MiB, temperature.gpu °C. Filtered through --query-gpu= so the
// field order is fixed.
//
// rocm-smi output we parse (JSON, ROCm ≥ 5.0):
//
//	[{"ID":"0","Name":"Radeon RX 7900 XT","Temperature (Sensor edge) (C)":"45.0",
//	  "GPU use (%)":"12","VRAM Total Memory (B)":"...","VRAM Total Used Memory (B)":"..."}]
//
// Field names vary across driver versions; the parsing picks the
// first field matching the suffix (case-sensitive after the spaces
// in the labels). Older ROCm (<5.0) returns different keys and
// gets a warn log + empty slice.

package collector

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/quanla93/lumen/internal/shared/api"
)

var (
	gpuOnce      sync.Once
	gpuNVPath    string
	gpuAMDPath   string
	gpuHasNV     bool
	gpuHasAMD    bool
)

// GPU runs nvidia-smi / rocm-smi (whichever is on $PATH) and returns
// one api.GPUInfo per physical GPU. Returns an empty slice (no
// error) when neither tool is installed — a homelab without a
// discrete GPU is the common case, and we don't want to spam logs
// every 5 s with "no GPU found".
func GPU(ctx context.Context) ([]api.GPUInfo, error) {
	detectGPUExecutables()

	// NVIDIA path: CSV, one row per GPU.
	if gpuHasNV {
		out, err := runGPUCommand(ctx, gpuNVPath,
			"--query-gpu=index,name,utilization.gpu,memory.used,memory.total,temperature.gpu",
			"--format=csv,noheader,nounits",
		)
		if err == nil {
			return parseNvidiaSmiCSV(out)
		}
		// exec failed but we still found the binary at startup.
		// Empty slice is fine — the operator's driver may be
		// misbehaving; we don't want to block ingest.
		return nil, nil
	}

	// AMD path: JSON. We use --json (the ROCm 5.0+ default) and
	// pick the first matching key for each field.
	if gpuHasAMD {
		out, err := runGPUCommand(ctx, gpuAMDPath, "--json")
		if err == nil {
			return parseRocmSmiJSON(out)
		}
		return nil, nil
	}

	// No GPU tooling installed — silent no-op.
	return nil, nil
}

// detectGPUExecutables caches nvidia-smi / rocm-smi lookups. Called
// once via sync.Once; safe for concurrent invocation.
func detectGPUExecutables() {
	gpuOnce.Do(func() {
		if p, err := exec.LookPath("nvidia-smi"); err == nil {
			gpuNVPath = p
			gpuHasNV = true
		}
		if p, err := exec.LookPath("rocm-smi"); err == nil {
			gpuAMDPath = p
			gpuHasAMD = true
		}
	})
}

// runGPUCommand shells out to the named binary with args. 5 s
// timeout protects the agent from a stuck driver (the nvidia-smi
// tool can block for tens of seconds when the kernel module is
// wedged). Stdout is returned; stderr is folded into the error so
// it's visible in the agent log.
func runGPUCommand(ctx context.Context, path string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, path, args...)
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr = strings.TrimSpace(string(exitErr.Stderr))
		}
		return "", fmt.Errorf("gpu: %s %v: %w (stderr=%q)", filepath.Base(path), args, err, stderr)
	}
	return string(out), nil
}

// parseNvidiaSmiCSV turns the --format=csv,noheader output into a
// slice of GPUInfo. The field order is fixed by the query string
// in runGPUCommand. Empty rows or rows with fewer than 6 fields are
// skipped — older driver versions sometimes emit a header line
// anyway.
func parseNvidiaSmiCSV(s string) ([]api.GPUInfo, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	r := csv.NewReader(strings.NewReader(s))
	r.FieldsPerRecord = -1 // accept any number of fields
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("gpu: parse nvidia-smi csv: %w", err)
	}
	out := make([]api.GPUInfo, 0, len(records))
	for _, rec := range records {
		if len(rec) < 6 {
			continue
		}
		idx, _ := strconv.Atoi(strings.TrimSpace(rec[0]))
		util, _ := strconv.ParseFloat(strings.TrimSpace(rec[2]), 64)
		memUsed, _ := strconv.ParseUint(strings.TrimSpace(rec[3]), 10, 64)
		memTotal, _ := strconv.ParseUint(strings.TrimSpace(rec[4]), 10, 64)
		temp, _ := strconv.ParseFloat(strings.TrimSpace(rec[5]), 64)
		out = append(out, api.GPUInfo{
			Index:      idx,
			Name:       strings.TrimSpace(rec[1]),
			UtilPct:    util,
			MemUsedMB:  memUsed,
			MemTotalMB: memTotal,
			TempC:      temp,
		})
	}
	return out, nil
}

// parseRocmSmiJSON handles the JSON variant. The output is an array
// of objects whose keys are dynamic (e.g. "Temperature (Sensor edge)
// (C)"); we pick the first key matching the suffix. Missing fields
// default to 0 — the alert metric just sees "no reading" rather than
// dropping the GPU entirely.
func parseRocmSmiJSON(s string) ([]api.GPUInfo, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var rows []map[string]string
	if err := json.Unmarshal([]byte(s), &rows); err != nil {
		// Older ROCm returns a single object keyed by "card0" /
		// "card1" — try the wrapped shape too.
		var wrapped map[string]map[string]string
		if err2 := json.Unmarshal([]byte(s), &wrapped); err2 == nil {
			for _, card := range wrapped {
				row := map[string]string{}
				for k, v := range card {
					row[k] = v
				}
				rows = append(rows, row)
			}
		} else {
			return nil, fmt.Errorf("gpu: parse rocm-smi json: %w", err)
		}
	}
	out := make([]api.GPUInfo, 0, len(rows))
	for _, row := range rows {
		gpu := api.GPUInfo{
			Name: rocmPickFirst(row, "Name", "Card series", "Card Model"),
		}
		if idx, ok := rocmPickFirstNumeric(row, "ID", "Index", "card0"); ok {
			gpu.Index = int(idx)
		}
		if util, ok := rocmPickFirstNumeric(row, "GPU use (%)", "GPU Use (%)", "GPU utilization"); ok {
			gpu.UtilPct = util
		}
		if used, ok := rocmPickFirstNumeric(row, "VRAM Total Used Memory (B)", "VRAM Used Memory (B)"); ok {
			gpu.MemUsedMB = uint64(used / (1024 * 1024))
		}
		if total, ok := rocmPickFirstNumeric(row, "VRAM Total Memory (B)", "VRAM Total Memory"); ok {
			gpu.MemTotalMB = uint64(total / (1024 * 1024))
		}
		if temp, ok := rocmPickFirstNumeric(row, "Temperature (Sensor edge) (C)", "Temperature (C)"); ok {
			gpu.TempC = temp
		}
		out = append(out, gpu)
	}
	return out, nil
}

// rocmPickFirst returns the first row[k] that exists for any of the
// candidate keys (in order). ROCm's key naming is unstable across
// driver versions, so we accept a few.
func rocmPickFirst(row map[string]string, keys ...string) string {
	for _, k := range keys {
		if v, ok := row[k]; ok && v != "" {
			return v
		}
	}
	return ""
}

// rocmPickFirstNumeric is rocmPickFirst that parses to float64. The
// boolean ok is true only when a value was found AND parsed.
func rocmPickFirstNumeric(row map[string]string, keys ...string) (float64, bool) {
	s := rocmPickFirst(row, keys...)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// _ = errors.Is keeps the import list honest when go vet runs on
// platforms where the executable path check returns early.
var _ = errors.Is
