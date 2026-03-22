package socket

import (
	"strings"

	"github.com/shirou/gopsutil/v3/host"
)

// DetectDistro returns the Linux distribution name (e.g. "ubuntu", "centos",
// "debian"). Falls back to "linux" when detection fails.
func DetectDistro() string {
	info, err := host.Info()
	if err != nil || info == nil {
		return "linux"
	}
	platform := strings.ToLower(strings.TrimSpace(info.Platform))
	if platform == "" {
		return "linux"
	}
	return platform
}
