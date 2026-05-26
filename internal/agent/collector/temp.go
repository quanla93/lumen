package collector

import (
	"context"
	"fmt"
	"strings"

	"github.com/shirou/gopsutil/v4/sensors"
)

// Temperature returns the hottest CPU-package temperature in °C the agent
// can read. Returns 0 with nil error if no sensor is available (common on
// macOS without root, on Windows without WMI access, and in Docker
// containers that can't read /sys/class/hwmon).
//
// Selection rule: prefer sensors whose key looks CPU-ish (coretemp,
// k10temp, cpu, package, tctl). If none match, fall back to the
// hottest of all non-zero readings — better than nothing for non-Intel/AMD
// systems where the convention differs.
func Temperature(_ context.Context) (float64, error) {
	temps, err := sensors.SensorsTemperatures()
	if err != nil {
		// Many platforms surface a "warnings" error that still includes
		// usable readings — only fail hard if we got nothing at all.
		if len(temps) == 0 {
			// On macOS/Windows/containers this is just "unsupported".
			// Don't propagate — the caller will report 0 temp.
			return 0, nil
		}
	}
	var cpuMax, anyMax float64
	for _, t := range temps {
		if t.Temperature == 0 {
			continue
		}
		key := strings.ToLower(t.SensorKey)
		if t.Temperature > anyMax {
			anyMax = t.Temperature
		}
		if strings.Contains(key, "coretemp") ||
			strings.Contains(key, "k10temp") ||
			strings.Contains(key, "package") ||
			strings.Contains(key, "tctl") ||
			strings.HasPrefix(key, "cpu") {
			if t.Temperature > cpuMax {
				cpuMax = t.Temperature
			}
		}
	}
	if cpuMax > 0 {
		return cpuMax, nil
	}
	if anyMax > 0 {
		return anyMax, nil
	}
	if err != nil {
		return 0, fmt.Errorf("sensors: %w", err)
	}
	return 0, nil
}
