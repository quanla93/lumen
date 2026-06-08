package collector

import "testing"

// TestParseNvidiaSmiCSV exercises the standard nvidia-smi
// --query-gpu=... output the agent shells out for. Two GPUs, one
// row each; the test confirms all six fields are mapped to the
// right struct member.
func TestParseNvidiaSmiCSV(t *testing.T) {
	const fixture = `0, NVIDIA GeForce RTX 4090, 35, 1024, 24576, 41
1, NVIDIA GeForce RTX 4090, 12, 256, 24576, 38`
	gpus, err := parseNvidiaSmiCSV(fixture)
	if err != nil {
		t.Fatalf("parseNvidiaSmiCSV: %v", err)
	}
	if len(gpus) != 2 {
		t.Fatalf("len(gpus) = %d, want 2", len(gpus))
	}
	if gpus[0].Index != 0 || gpus[0].Name != "NVIDIA GeForce RTX 4090" {
		t.Errorf("gpus[0] = %+v, want Index=0 Name=RTX 4090", gpus[0])
	}
	if gpus[0].UtilPct != 35 {
		t.Errorf("gpus[0].UtilPct = %v, want 35", gpus[0].UtilPct)
	}
	if gpus[0].MemUsedMB != 1024 || gpus[0].MemTotalMB != 24576 {
		t.Errorf("gpus[0] memory = used=%d total=%d, want 1024/24576", gpus[0].MemUsedMB, gpus[0].MemTotalMB)
	}
	if gpus[0].TempC != 41 {
		t.Errorf("gpus[0].TempC = %v, want 41", gpus[0].TempC)
	}
	if gpus[1].Index != 1 || gpus[1].UtilPct != 12 {
		t.Errorf("gpus[1] = %+v, want Index=1 UtilPct=12", gpus[1])
	}
}

// TestParseNvidiaSmiCSV_Empty confirms an empty result (no
// GPUs) parses to nil without error.
func TestParseNvidiaSmiCSV_Empty(t *testing.T) {
	gpus, err := parseNvidiaSmiCSV("")
	if err != nil {
		t.Fatalf("parseNvidiaSmiCSV: %v", err)
	}
	if gpus != nil {
		t.Errorf("gpus = %v, want nil", gpus)
	}
}

// TestParseNvidiaSmiCSV_ShortRow confirms a malformed row
// (fewer than 6 fields) is skipped without failing the whole
// batch.
func TestParseNvidiaSmiCSV_ShortRow(t *testing.T) {
	const fixture = `0, short, 1, 2
1, full-row, 10, 256, 1024, 30`
	gpus, err := parseNvidiaSmiCSV(fixture)
	if err != nil {
		t.Fatalf("parseNvidiaSmiCSV: %v", err)
	}
	if len(gpus) != 1 {
		t.Fatalf("len(gpus) = %d, want 1 (short row skipped)", len(gpus))
	}
	if gpus[0].Name != "full-row" {
		t.Errorf("gpus[0] = %+v, want Name=full-row", gpus[0])
	}
}

// TestParseRocmSmiJSON exercises the ROCm 5.0+ JSON shape — array
// of objects, dynamic keys, sometimes with spaces + units in the
// label.
func TestParseRocmSmiJSON(t *testing.T) {
	const fixture = `[
		{
			"ID": "0",
			"Name": "Radeon RX 7900 XT",
			"Temperature (Sensor edge) (C)": "45.0",
			"GPU use (%)": "12",
			"VRAM Total Memory (B)": "21474836480",
			"VRAM Total Used Memory (B)": "1073741824"
		}
	]`
	gpus, err := parseRocmSmiJSON(fixture)
	if err != nil {
		t.Fatalf("parseRocmSmiJSON: %v", err)
	}
	if len(gpus) != 1 {
		t.Fatalf("len(gpus) = %d, want 1", len(gpus))
	}
	if gpus[0].Name != "Radeon RX 7900 XT" {
		t.Errorf("Name = %q, want Radeon RX 7900 XT", gpus[0].Name)
	}
	if gpus[0].Index != 0 {
		t.Errorf("Index = %d, want 0", gpus[0].Index)
	}
	if gpus[0].TempC != 45.0 {
		t.Errorf("TempC = %v, want 45", gpus[0].TempC)
	}
	if gpus[0].UtilPct != 12 {
		t.Errorf("UtilPct = %v, want 12", gpus[0].UtilPct)
	}
	// 1073741824 bytes → 1024 MiB; 21474836480 → 20480 MiB.
	if gpus[0].MemUsedMB != 1024 || gpus[0].MemTotalMB != 20480 {
		t.Errorf("memory = used=%d total=%d, want 1024/20480", gpus[0].MemUsedMB, gpus[0].MemTotalMB)
	}
}

// TestParseRocmSmiJSON_AlternativeKeys covers the v5.5+ key
// spelling change ("Card series" instead of "Name").
func TestParseRocmSmiJSON_AlternativeKeys(t *testing.T) {
	const fixture = `[{"ID":"1","Card series":"Radeon Pro W7900","GPU use (%)":"0","Temperature (Sensor edge) (C)":"30"}]`
	gpus, err := parseRocmSmiJSON(fixture)
	if err != nil {
		t.Fatalf("parseRocmSmiJSON: %v", err)
	}
	if len(gpus) != 1 {
		t.Fatalf("len(gpus) = %d, want 1", len(gpus))
	}
	if gpus[0].Name != "Radeon Pro W7900" {
		t.Errorf("Name fallback didn't pick up Card series, got %q", gpus[0].Name)
	}
}

// TestParseRocmSmiJSON_Empty covers the missing-tooling case.
func TestParseRocmSmiJSON_Empty(t *testing.T) {
	gpus, err := parseRocmSmiJSON("")
	if err != nil {
		t.Fatalf("parseRocmSmiJSON: %v", err)
	}
	if gpus != nil {
		t.Errorf("gpus = %v, want nil", gpus)
	}
}
