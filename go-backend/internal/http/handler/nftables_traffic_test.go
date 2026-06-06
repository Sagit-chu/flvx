package handler

import (
	"math"
	"testing"

	runtimenft "go-backend/internal/runtime/nftables"
	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
)

func TestBuildNftCounterDeltasSavesFirstBaselineWithoutDelta(t *testing.T) {
	nowMs := int64(1700000000123)
	deltas, states := buildNftCounterDeltas(11, []runtimenft.CounterSample{
		{ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: 1000, Packets: 10},
	}, nil, map[int64]string{42: "hash-a"}, nowMs)

	if len(deltas) != 0 {
		t.Fatalf("expected no deltas for first baseline, got %#v", deltas)
	}
	if len(states) != 1 {
		t.Fatalf("expected one state input, got %d", len(states))
	}
	state := states[0]
	if state.NodeID != 11 || state.ForwardID != 42 || state.Protocol != "tcp" || state.Direction != runtimenft.CounterDirectionToTarget {
		t.Fatalf("unexpected state identity: %#v", state)
	}
	if state.RuleHash != "hash-a" || state.Bytes != 1000 || state.Packets != 10 || state.CollectedTime != nowMs {
		t.Fatalf("unexpected state values: %#v", state)
	}
}

func TestBuildNftCounterDeltasNormalGrowthProducesDirectionalBytes(t *testing.T) {
	deltas, states := buildNftCounterDeltas(11, []runtimenft.CounterSample{
		{ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: 1500, Packets: 15},
		{ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionFromTarget, Bytes: 2600, Packets: 26},
	}, []model.NftCounterState{
		{NodeID: 11, ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, RuleHash: "hash-a", Bytes: 1000, Packets: 10},
		{NodeID: 11, ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionFromTarget, RuleHash: "hash-a", Bytes: 2000, Packets: 20},
	}, map[int64]string{42: "hash-a"}, 2000)

	if len(deltas) != 1 {
		t.Fatalf("expected one aggregated delta, got %#v", deltas)
	}
	if deltas[0].ForwardID != 42 || deltas[0].BytesIn != 500 || deltas[0].BytesOut != 600 {
		t.Fatalf("unexpected delta: %#v", deltas[0])
	}
	if len(states) != 2 {
		t.Fatalf("expected two state inputs, got %d", len(states))
	}
}

func TestBuildNftCounterDeltasResetRefreshesBaselineWithoutDelta(t *testing.T) {
	deltas, states := buildNftCounterDeltas(11, []runtimenft.CounterSample{
		{ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: 25, Packets: 2},
	}, []model.NftCounterState{
		{NodeID: 11, ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, RuleHash: "hash-a", Bytes: 1000, Packets: 10},
	}, map[int64]string{42: "hash-a"}, 2000)

	if len(deltas) != 0 {
		t.Fatalf("expected reset to produce no deltas, got %#v", deltas)
	}
	if len(states) != 1 || states[0].Bytes != 25 || states[0].RuleHash != "hash-a" {
		t.Fatalf("expected refreshed baseline state, got %#v", states)
	}
}

func TestBuildNftCounterDeltasRuleHashChangeRefreshesBaselineWithoutDelta(t *testing.T) {
	deltas, states := buildNftCounterDeltas(11, []runtimenft.CounterSample{
		{ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: 1500, Packets: 15},
	}, []model.NftCounterState{
		{NodeID: 11, ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, RuleHash: "hash-a", Bytes: 1000, Packets: 10},
	}, map[int64]string{42: "hash-b"}, 2000)

	if len(deltas) != 0 {
		t.Fatalf("expected rule hash change to produce no deltas, got %#v", deltas)
	}
	if len(states) != 1 || states[0].Bytes != 1500 || states[0].RuleHash != "hash-b" {
		t.Fatalf("expected refreshed hash baseline state, got %#v", states)
	}
}

func TestBuildNftCounterDeltasEqualBytesRefreshesBaselineWithoutDelta(t *testing.T) {
	deltas, states := buildNftCounterDeltas(11, []runtimenft.CounterSample{
		{ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: 1000, Packets: 11},
	}, []model.NftCounterState{
		{NodeID: 11, ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, RuleHash: "hash-a", Bytes: 1000, Packets: 10},
	}, map[int64]string{42: "hash-a"}, 2000)

	if len(deltas) != 0 {
		t.Fatalf("expected equal bytes to produce no deltas, got %#v", deltas)
	}
	if len(states) != 1 || states[0].Bytes != 1000 || states[0].Packets != 11 || states[0].RuleHash != "hash-a" {
		t.Fatalf("expected refreshed baseline state, got %#v", states)
	}
}

func TestBuildNftCounterDeltasAggregatesProtocolsAndDirections(t *testing.T) {
	deltas, _ := buildNftCounterDeltas(11, []runtimenft.CounterSample{
		{ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: 1100, Packets: 11},
		{ForwardID: 42, Protocol: "udp", Direction: runtimenft.CounterDirectionToTarget, Bytes: 2200, Packets: 22},
		{ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionFromTarget, Bytes: 3300, Packets: 33},
		{ForwardID: 42, Protocol: "udp", Direction: runtimenft.CounterDirectionFromTarget, Bytes: 4400, Packets: 44},
	}, []model.NftCounterState{
		{NodeID: 11, ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, RuleHash: "hash-a", Bytes: 1000},
		{NodeID: 11, ForwardID: 42, Protocol: "udp", Direction: runtimenft.CounterDirectionToTarget, RuleHash: "hash-a", Bytes: 2000},
		{NodeID: 11, ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionFromTarget, RuleHash: "hash-a", Bytes: 3000},
		{NodeID: 11, ForwardID: 42, Protocol: "udp", Direction: runtimenft.CounterDirectionFromTarget, RuleHash: "hash-a", Bytes: 4000},
	}, map[int64]string{42: "hash-a"}, 2000)

	if len(deltas) != 1 {
		t.Fatalf("expected one aggregated delta, got %#v", deltas)
	}
	if deltas[0].ForwardID != 42 || deltas[0].BytesIn != 300 || deltas[0].BytesOut != 700 {
		t.Fatalf("unexpected aggregated delta: %#v", deltas[0])
	}
}

func TestBuildNftCounterDeltasSkipsInvalidProtocolBeforeStateAndDelta(t *testing.T) {
	deltas, states := buildNftCounterDeltas(11, []runtimenft.CounterSample{
		{ForwardID: 42, Protocol: "icmp", Direction: runtimenft.CounterDirectionToTarget, Bytes: 1500, Packets: 15},
	}, []model.NftCounterState{
		{NodeID: 11, ForwardID: 42, Protocol: "icmp", Direction: runtimenft.CounterDirectionToTarget, RuleHash: "hash-a", Bytes: 1000, Packets: 10},
	}, map[int64]string{42: "hash-a"}, 2000)

	if len(deltas) != 0 {
		t.Fatalf("expected invalid protocol to produce no deltas, got %#v", deltas)
	}
	if len(states) != 0 {
		t.Fatalf("expected invalid protocol to produce no state inputs, got %#v", states)
	}
}

func TestBuildNftCounterDeltasSkipsOversizedPacketsBeforeStateAndDelta(t *testing.T) {
	deltas, states := buildNftCounterDeltas(11, []runtimenft.CounterSample{
		{ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: 1500, Packets: uint64(math.MaxInt64) + 1},
	}, []model.NftCounterState{
		{NodeID: 11, ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, RuleHash: "hash-a", Bytes: 1000, Packets: 10},
	}, map[int64]string{42: "hash-a"}, 2000)

	if len(deltas) != 0 {
		t.Fatalf("expected oversized packets to produce no deltas, got %#v", deltas)
	}
	if len(states) != 0 {
		t.Fatalf("expected oversized packets to produce no state inputs, got %#v", states)
	}
}

func TestBuildNftCounterDeltasSkipsOversizedBytesBeforeStateAndDelta(t *testing.T) {
	deltas, states := buildNftCounterDeltas(11, []runtimenft.CounterSample{
		{ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: uint64(math.MaxInt64) + 1, Packets: 10},
	}, []model.NftCounterState{
		{NodeID: 11, ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, RuleHash: "hash-a", Bytes: 1000, Packets: 10},
	}, map[int64]string{42: "hash-a"}, 2000)

	if len(deltas) != 0 {
		t.Fatalf("expected oversized bytes to produce no deltas, got %#v", deltas)
	}
	if len(states) != 0 {
		t.Fatalf("expected oversized bytes to produce no state inputs, got %#v", states)
	}
}

func TestBuildNftCounterDeltasSkipsOverflowingAggregateSampleWithoutState(t *testing.T) {
	deltas, states := buildNftCounterDeltas(11, []runtimenft.CounterSample{
		{ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: uint64(math.MaxInt64), Packets: 10},
		{ForwardID: 42, Protocol: "udp", Direction: runtimenft.CounterDirectionToTarget, Bytes: 10, Packets: 1},
	}, []model.NftCounterState{
		{NodeID: 11, ForwardID: 42, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, RuleHash: "hash-a", Bytes: 1, Packets: 1},
		{NodeID: 11, ForwardID: 42, Protocol: "udp", Direction: runtimenft.CounterDirectionToTarget, RuleHash: "hash-a", Bytes: 1, Packets: 1},
	}, map[int64]string{42: "hash-a"}, 2000)

	if len(deltas) != 1 {
		t.Fatalf("expected only non-overflowing aggregate delta, got %#v", deltas)
	}
	if deltas[0].ForwardID != 42 || deltas[0].BytesIn != math.MaxInt64-1 || deltas[0].BytesOut != 0 {
		t.Fatalf("unexpected aggregate delta: %#v", deltas[0])
	}
	if len(states) != 1 {
		t.Fatalf("expected only the accounted safe sample to advance baseline, got %#v", states)
	}
	if states[0].ForwardID != 42 || states[0].Protocol != "tcp" || states[0].Bytes != uint64(math.MaxInt64) {
		t.Fatalf("expected safe sample state input to be preserved, got %#v", states)
	}
}

func TestBuildNftCounterDeltasSkipsUnknownDirectionAndOversizedDelta(t *testing.T) {
	deltas, states := buildNftCounterDeltas(11, []runtimenft.CounterSample{
		{ForwardID: 42, Protocol: "tcp", Direction: "sideways", Bytes: 1500, Packets: 15},
		{ForwardID: 43, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: uint64(math.MaxInt64) + 1, Packets: 1},
	}, []model.NftCounterState{
		{NodeID: 11, ForwardID: 43, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, RuleHash: "hash-b", Bytes: 100},
	}, map[int64]string{42: "hash-a", 43: "hash-b"}, 2000)

	if len(deltas) != 0 {
		t.Fatalf("expected no delta for skipped/oversized samples, got %#v", deltas)
	}
	if len(states) != 0 {
		t.Fatalf("expected no state inputs for skipped/oversized samples, got %#v", states)
	}
}

func TestBuildNftFlowUploadBatchScalesFlowAndPreservesRawTunnelTraffic(t *testing.T) {
	batch := buildNftFlowUploadBatch([]nftTrafficDelta{
		{ForwardID: 20, BytesIn: 80, BytesOut: 110},
		{ForwardID: 21, BytesIn: 7, BytesOut: 11},
		{ForwardID: 20, BytesIn: 20, BytesOut: 10},
	}, map[int64]repo.FlowUploadForwardMeta{
		20: {ForwardID: 20, UserID: 2, UserTunnelID: 10, TunnelID: 1, TrafficRatio: 2, TunnelFlow: 3},
		21: {ForwardID: 21, UserID: 2, UserTunnelID: 10, TunnelID: 1, TrafficRatio: 1.5, TunnelFlow: 2},
	})

	if len(batch.flowDeltas) != 2 {
		t.Fatalf("expected two flow deltas, got %#v", batch.flowDeltas)
	}
	if batch.flowDeltas[0].ForwardID != 20 || batch.flowDeltas[0].InFlow != 600 || batch.flowDeltas[0].OutFlow != 720 {
		t.Fatalf("unexpected first flow delta: %#v", batch.flowDeltas[0])
	}
	if batch.flowDeltas[1].ForwardID != 21 || batch.flowDeltas[1].InFlow != 20 || batch.flowDeltas[1].OutFlow != 32 {
		t.Fatalf("unexpected second flow delta: %#v", batch.flowDeltas[1])
	}
	if batch.quotaUsage[2] != 1372 {
		t.Fatalf("expected quota usage 1372, got %d", batch.quotaUsage[2])
	}
	if len(batch.policyTargets) != 1 || batch.policyTargets[0].UserID != 2 || batch.policyTargets[0].UserTunnelID != 10 {
		t.Fatalf("expected deduped policy target, got %#v", batch.policyTargets)
	}
	if traffic := batch.forwardTraffic[20]; traffic.bytesIn != 100 || traffic.bytesOut != 120 {
		t.Fatalf("expected raw traffic for forward 20, got %#v", traffic)
	}
	if traffic := batch.forwardTraffic[21]; traffic.bytesIn != 7 || traffic.bytesOut != 11 {
		t.Fatalf("expected raw traffic for forward 21, got %#v", traffic)
	}
}

func TestBuildNftFlowUploadBatchSkipsOverflowingScaledFlow(t *testing.T) {
	batch := buildNftFlowUploadBatch([]nftTrafficDelta{
		{ForwardID: 20, BytesIn: math.MaxInt64, BytesOut: 0},
	}, map[int64]repo.FlowUploadForwardMeta{
		20: {ForwardID: 20, UserID: 2, UserTunnelID: 10, TrafficRatio: 2, TunnelFlow: 2},
	})

	if len(batch.flowDeltas) != 0 {
		t.Fatalf("expected overflowing scaled flow to be skipped, got %#v", batch.flowDeltas)
	}
	if len(batch.quotaUsage) != 0 {
		t.Fatalf("expected no quota usage for overflowing scaled flow, got %#v", batch.quotaUsage)
	}
	if len(batch.policyTargets) != 0 {
		t.Fatalf("expected no policy targets for overflowing scaled flow, got %#v", batch.policyTargets)
	}
	if len(batch.forwardTraffic) != 0 {
		t.Fatalf("expected no raw traffic for overflowing scaled flow, got %#v", batch.forwardTraffic)
	}
}

func TestBuildNftFlowUploadBatchSkipsRawForwardTrafficOverflow(t *testing.T) {
	batch := buildNftFlowUploadBatch([]nftTrafficDelta{
		{ForwardID: 20, BytesIn: math.MaxInt64, BytesOut: 0},
		{ForwardID: 20, BytesIn: 1, BytesOut: 0},
	}, map[int64]repo.FlowUploadForwardMeta{
		20: {ForwardID: 20, UserID: 2, UserTunnelID: 10, TrafficRatio: 0.5, TunnelFlow: 1},
	})

	traffic := batch.forwardTraffic[20]
	if traffic.bytesIn != math.MaxInt64 || traffic.bytesOut != 0 {
		t.Fatalf("expected overflowing raw delta to be skipped without negative traffic, got %#v", traffic)
	}
	if len(batch.flowDeltas) != 1 || batch.flowDeltas[0].ForwardID != 20 {
		t.Fatalf("expected only the safe flow delta, got %#v", batch.flowDeltas)
	}
	if len(batch.policyTargets) != 1 || batch.policyTargets[0].UserID != 2 || batch.policyTargets[0].UserTunnelID != 10 {
		t.Fatalf("expected policy target only from safe delta, got %#v", batch.policyTargets)
	}
}

func TestBuildNftFlowUploadBatchSkipsQuotaOverflow(t *testing.T) {
	batch := buildNftFlowUploadBatch([]nftTrafficDelta{
		{ForwardID: 20, BytesIn: math.MaxInt64, BytesOut: 0},
		{ForwardID: 21, BytesIn: 1, BytesOut: 0},
	}, map[int64]repo.FlowUploadForwardMeta{
		20: {ForwardID: 20, UserID: 2, UserTunnelID: 10, TrafficRatio: 1, TunnelFlow: 1},
		21: {ForwardID: 21, UserID: 2, UserTunnelID: 10, TrafficRatio: 1, TunnelFlow: 1},
	})

	if len(batch.flowDeltas) != 1 || batch.flowDeltas[0].ForwardID != 20 || batch.flowDeltas[0].InFlow != math.MaxInt64 {
		t.Fatalf("expected only non-overflowing quota delta, got %#v", batch.flowDeltas)
	}
	if batch.quotaUsage[2] != math.MaxInt64 {
		t.Fatalf("expected quota usage to remain at max int64, got %#v", batch.quotaUsage)
	}
	if len(batch.policyTargets) != 1 || batch.policyTargets[0].UserID != 2 || batch.policyTargets[0].UserTunnelID != 10 {
		t.Fatalf("expected one policy target from non-overflowing delta, got %#v", batch.policyTargets)
	}
	if _, ok := batch.forwardTraffic[21]; ok {
		t.Fatalf("expected quota-overflowing delta to be skipped from raw traffic")
	}
}

func TestBuildNftFlowUploadBatchSkipsMissingMeta(t *testing.T) {
	batch := buildNftFlowUploadBatch([]nftTrafficDelta{
		{ForwardID: 20, BytesIn: 80, BytesOut: 110},
		{ForwardID: 99, BytesIn: 1, BytesOut: 2},
	}, map[int64]repo.FlowUploadForwardMeta{
		20: {ForwardID: 20, UserID: 2, UserTunnelID: 10, TrafficRatio: 1, TunnelFlow: 1},
	})

	if len(batch.flowDeltas) != 1 || batch.flowDeltas[0].ForwardID != 20 {
		t.Fatalf("expected only forward 20 delta, got %#v", batch.flowDeltas)
	}
	if _, ok := batch.forwardTraffic[99]; ok {
		t.Fatalf("expected missing meta forward to be skipped from raw traffic")
	}
}
