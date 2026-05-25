package collector

import (
	"context"
	"fmt"
	"runtime"

	"github.com/shirou/gopsutil/v4/disk"
)

// Disk returns the used-percentage of the filesystem mounted at path.
func Disk(_ context.Context, path string) (float64, error) {
	u, err := disk.Usage(path)
	if err != nil {
		return 0, fmt.Errorf("disk.Usage(%q): %w", path, err)
	}
	return u.UsedPercent, nil
}

// DefaultDiskPath returns the conventional root filesystem path for the
// current OS — `C:\` on Windows, `/` everywhere else.
func DefaultDiskPath() string {
	if runtime.GOOS == "windows" {
		return `C:\`
	}
	return "/"
}
