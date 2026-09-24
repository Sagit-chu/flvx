package handler

import (
	"testing"
	"time"
)

func TestTrafficLimitMiBOverridesLegacyGB(t *testing.T) {
	flowGB, flowMiB, err := parseTrafficLimit(map[string]interface{}{
		"flow": float64(1), "flowMiB": float64(500),
	}, 100)
	if err != nil || flowGB != 1 || flowMiB != 500 {
		t.Fatalf("parseTrafficLimit = (%d, %d, %v), want (1, 500, nil)", flowGB, flowMiB, err)
	}
	limit := flowLimitBytes(flowGB, flowMiB)
	if limit != 500*bytesPerMiB {
		t.Fatalf("limit = %d, want %d", limit, 500*bytesPerMiB)
	}
	policy := &userTunnelPolicy{Flow: flowGB, FlowMiB: flowMiB, InFlow: limit - 1, Status: 1}
	if shouldPauseUserTunnel(policy, time.Now().UnixMilli()) {
		t.Fatal("policy paused before reaching 500 MiB")
	}
	policy.InFlow = limit
	if !shouldPauseUserTunnel(policy, time.Now().UnixMilli()) {
		t.Fatal("policy did not pause at 500 MiB")
	}
}

func TestTrafficLimitLegacyAndInvalidValues(t *testing.T) {
	flowGB, flowMiB, err := parseTrafficLimit(map[string]interface{}{"flow": float64(2)}, 100)
	if err != nil || flowGB != 2 || flowMiB != 0 || flowLimitBytes(flowGB, flowMiB) != 2*bytesPerGB {
		t.Fatalf("legacy GB limit changed: (%d, %d, %v)", flowGB, flowMiB, err)
	}
	for _, value := range []interface{}{"1.5", -1, "999999999999999999999"} {
		if _, _, err := parseTrafficLimit(map[string]interface{}{"flowMiB": value}, 100); err == nil {
			t.Fatalf("accepted invalid flowMiB %v", value)
		}
	}
}
