package collector

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"

	"github.com/quanla93/lumen/internal/shared/api"
)

func SystemMetadata(ctx context.Context, agentVersion string) (api.SystemMetadata, error) {
	meta := api.SystemMetadata{
		Arch:         runtime.GOARCH,
		AgentVersion: agentVersion,
		PrimaryIP:    primaryIP(),
	}
	if hostname, err := os.Hostname(); err == nil {
		meta.Hostname = strings.TrimSpace(hostname)
	}
	var errs []error

	info, err := host.InfoWithContext(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("host.Info: %w", err))
	} else {
		meta.OS = systemOS(info)
		meta.Kernel = info.KernelVersion
		meta.UptimeSeconds = info.Uptime
		// VirtualizationSystem is "" on bare metal and "kvm"/"lxc"/
		// "docker"/"wsl"/etc inside a guest. Hub uses this to decide
		// whether per-core CPU is meaningful for this agent.
		meta.VirtType = strings.TrimSpace(info.VirtualizationSystem)
	}

	infos, err := cpu.InfoWithContext(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("cpu.Info: %w", err))
	} else {
		for _, c := range infos {
			if strings.TrimSpace(c.ModelName) != "" {
				meta.CPUModel = strings.TrimSpace(c.ModelName)
				break
			}
		}
	}

	return meta, errors.Join(errs...)
}

func systemOS(info *host.InfoStat) string {
	parts := make([]string, 0, 3)
	for _, p := range []string{info.Platform, info.PlatformVersion, info.OS} {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " ")
}

func primaryIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			return ip.String()
		}
	}
	return ""
}
