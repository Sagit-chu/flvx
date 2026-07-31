package monitoring

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	ConfigTunnelQualityProbeIntervalSec  = "monitor_tunnel_quality_interval_sec"
	DefaultTunnelQualityProbeIntervalSec = 1
	MinTunnelQualityProbeIntervalSec     = 1
	MaxTunnelQualityProbeIntervalSec     = 3600
)

func TunnelQualityProbeIntervalSecondsFromConfigMap(cfg map[string]string) int {
	if cfg == nil {
		return DefaultTunnelQualityProbeIntervalSec
	}
	seconds, err := parseTunnelQualityProbeIntervalSeconds(cfg[ConfigTunnelQualityProbeIntervalSec])
	if err != nil {
		return DefaultTunnelQualityProbeIntervalSec
	}
	return seconds
}

func NormalizeTunnelQualityProbeIntervalSeconds(value string) (string, error) {
	seconds, err := parseTunnelQualityProbeIntervalSeconds(value)
	if err != nil {
		return "", err
	}
	return strconv.Itoa(seconds), nil
}

func parseTunnelQualityProbeIntervalSeconds(value string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, fmt.Errorf("隧道质量探测间隔不能为空")
	}
	seconds, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("隧道质量探测间隔必须是整数")
	}
	if seconds < MinTunnelQualityProbeIntervalSec || seconds > MaxTunnelQualityProbeIntervalSec {
		return 0, fmt.Errorf(
			"隧道质量探测间隔必须在 %d 到 %d 秒之间",
			MinTunnelQualityProbeIntervalSec,
			MaxTunnelQualityProbeIntervalSec,
		)
	}
	return seconds, nil
}
