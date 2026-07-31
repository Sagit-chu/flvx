package monitoring

import "testing"

func TestTunnelQualityProbeIntervalSecondsFromConfigMap(t *testing.T) {
	tests := []struct {
		name string
		cfg  map[string]string
		want int
	}{
		{name: "missing config", cfg: nil, want: DefaultTunnelQualityProbeIntervalSec},
		{name: "configured", cfg: map[string]string{ConfigTunnelQualityProbeIntervalSec: "15"}, want: 15},
		{name: "invalid", cfg: map[string]string{ConfigTunnelQualityProbeIntervalSec: "0"}, want: DefaultTunnelQualityProbeIntervalSec},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TunnelQualityProbeIntervalSecondsFromConfigMap(tt.cfg); got != tt.want {
				t.Fatalf("interval = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNormalizeTunnelQualityProbeIntervalSeconds(t *testing.T) {
	for _, value := range []string{"1", "15", "3600"} {
		if got, err := NormalizeTunnelQualityProbeIntervalSeconds(value); err != nil || got != value {
			t.Fatalf("normalize %q = %q, %v", value, got, err)
		}
	}

	for _, value := range []string{"", "0", "3601", "1.5", "abc"} {
		if got, err := NormalizeTunnelQualityProbeIntervalSeconds(value); err == nil {
			t.Fatalf("normalize %q unexpectedly succeeded with %q", value, got)
		}
	}
}
