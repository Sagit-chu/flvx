package handler

import (
	"errors"
	"math"
	"testing"
	"time"

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

func TestNftBatchCoversDeltasRequiresRawAndFlowEntries(t *testing.T) {
	deltas := []nftTrafficDelta{{ForwardID: 20, BytesIn: 1, BytesOut: 0}}
	batch := flowUploadBatch{
		forwardTraffic: map[int64]tunnelTrafficDelta{20: {bytesIn: 1}},
		flowDeltas:     []repo.FlowUploadCounterDelta{{ForwardID: 20, UserID: 2, UserTunnelID: 10, InFlow: 1}},
	}
	if missing, ok := firstNftBatchMissingDelta(deltas, batch); ok || missing != 0 {
		t.Fatalf("expected batch to cover delta, missing=%d ok=%v", missing, ok)
	}

	delete(batch.forwardTraffic, 20)
	if missing, ok := firstNftBatchMissingDelta(deltas, batch); !ok || missing != 20 {
		t.Fatalf("expected missing raw traffic for forward 20, got missing=%d ok=%v", missing, ok)
	}

	batch.forwardTraffic[20] = tunnelTrafficDelta{bytesIn: 1}
	batch.flowDeltas = nil
	if missing, ok := firstNftBatchMissingDelta(deltas, batch); !ok || missing != 20 {
		t.Fatalf("expected missing flow delta for forward 20, got missing=%d ok=%v", missing, ok)
	}
}

func TestNftBatchCoversDeltasRequiresAggregateRawTotals(t *testing.T) {
	deltas := []nftTrafficDelta{
		{ForwardID: 20, BytesIn: math.MaxInt64, BytesOut: 0},
		{ForwardID: 20, BytesIn: 1, BytesOut: 0},
	}
	batch := buildNftFlowUploadBatch(deltas, map[int64]repo.FlowUploadForwardMeta{
		20: {ForwardID: 20, UserID: 2, UserTunnelID: 10, TrafficRatio: 0.5, TunnelFlow: 1},
	})

	if missing, ok := firstNftBatchMissingDelta(deltas, batch); !ok || missing != 20 {
		t.Fatalf("expected aggregate raw overflow/mismatch for forward 20, got missing=%d ok=%v", missing, ok)
	}
}

func TestCollectNftablesNodeTrafficFirstBaselineSavesStateWithoutFlow(t *testing.T) {
	fixture := setupNftablesCollectionFixture(t)
	h := fixture.handler
	manager := &fakeNftablesManager{counterSamples: []runtimenft.CounterSample{
		{ForwardID: fixture.forwardID, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: 1000, Packets: 10},
		{ForwardID: fixture.forwardID, Protocol: "tcp", Direction: runtimenft.CounterDirectionFromTarget, Bytes: 2000, Packets: 20},
	}}
	h.nftablesManager = manager
	cfg := mustCollectionSSHConfig(t, h, fixture.nodeID)

	h.collectNftablesNodeTraffic(fixture.nodeID, cfg, time.Unix(1700000000, 0))

	if manager.collectHit != 1 {
		t.Fatalf("expected one collection, got %d", manager.collectHit)
	}
	states, err := h.repo.GetNftCounterStatesByNode(fixture.nodeID)
	if err != nil {
		t.Fatalf("load states: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("expected two baseline states, got %+v", states)
	}
	if got := mustHandlerCount(t, h, `SELECT in_flow FROM forward WHERE id = ?`, fixture.forwardID); got != 0 {
		t.Fatalf("expected no forward flow on baseline, got %d", got)
	}
	if got := mustHandlerCount(t, h, `SELECT out_flow FROM user WHERE id = 1`); got != 0 {
		t.Fatalf("expected no user flow on baseline, got %d", got)
	}
}

func TestCollectNftablesNodeTrafficGrowthAppliesFlowAndUpdatesState(t *testing.T) {
	fixture := setupNftablesCollectionFixture(t)
	h := fixture.handler
	manager := &fakeNftablesManager{}
	h.nftablesManager = manager
	cfg := mustCollectionSSHConfig(t, h, fixture.nodeID)

	manager.counterSamples = []runtimenft.CounterSample{
		{ForwardID: fixture.forwardID, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: 1000, Packets: 10},
		{ForwardID: fixture.forwardID, Protocol: "tcp", Direction: runtimenft.CounterDirectionFromTarget, Bytes: 2000, Packets: 20},
	}
	h.collectNftablesNodeTraffic(fixture.nodeID, cfg, time.Unix(1700000000, 0))

	manager.counterSamples = []runtimenft.CounterSample{
		{ForwardID: fixture.forwardID, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: 1400, Packets: 14},
		{ForwardID: fixture.forwardID, Protocol: "tcp", Direction: runtimenft.CounterDirectionFromTarget, Bytes: 2600, Packets: 26},
	}
	h.collectNftablesNodeTraffic(fixture.nodeID, cfg, time.Unix(1700000060, 0))

	if got := mustHandlerCount(t, h, `SELECT in_flow FROM forward WHERE id = ?`, fixture.forwardID); got != 400 {
		t.Fatalf("expected forward in_flow=400, got %d", got)
	}
	if got := mustHandlerCount(t, h, `SELECT out_flow FROM forward WHERE id = ?`, fixture.forwardID); got != 600 {
		t.Fatalf("expected forward out_flow=600, got %d", got)
	}
	if got := mustHandlerCount(t, h, `SELECT in_flow FROM user WHERE id = 1`); got != 400 {
		t.Fatalf("expected user in_flow=400, got %d", got)
	}
	if got := mustHandlerCount(t, h, `SELECT out_flow FROM user_tunnel WHERE id = ?`, fixture.userTunnelID); got != 600 {
		t.Fatalf("expected user_tunnel out_flow=600, got %d", got)
	}
	if got := mustHandlerCount(t, h, `SELECT COALESCE((SELECT daily_used_bytes FROM user_quota WHERE user_id = 1), 0)`); got != 1000 {
		t.Fatalf("expected daily quota usage=1000, got %d", got)
	}
	states, err := h.repo.GetNftCounterStatesByNode(fixture.nodeID)
	if err != nil {
		t.Fatalf("load states: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("expected two states after growth, got %+v", states)
	}
	for _, state := range states {
		if state.Direction == runtimenft.CounterDirectionToTarget && state.Bytes != 1400 {
			t.Fatalf("expected to-target state bytes 1400, got %+v", state)
		}
		if state.Direction == runtimenft.CounterDirectionFromTarget && state.Bytes != 2600 {
			t.Fatalf("expected from-target state bytes 2600, got %+v", state)
		}
	}
}

func TestCollectNftablesNodeTrafficSkippedBatchDeltaDoesNotAdvanceState(t *testing.T) {
	fixture := setupNftablesCollectionFixture(t)
	h := fixture.handler
	if err := h.repo.DB().Exec(`UPDATE tunnel SET traffic_ratio = 2 WHERE id = (SELECT tunnel_id FROM forward WHERE id = ?)`, fixture.forwardID).Error; err != nil {
		t.Fatalf("update tunnel ratio: %v", err)
	}
	manager := &fakeNftablesManager{}
	h.nftablesManager = manager
	cfg := mustCollectionSSHConfig(t, h, fixture.nodeID)

	manager.counterSamples = []runtimenft.CounterSample{
		{ForwardID: fixture.forwardID, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: 0, Packets: 0},
	}
	h.collectNftablesNodeTraffic(fixture.nodeID, cfg, time.Unix(1700000000, 0))

	manager.counterSamples = []runtimenft.CounterSample{
		{ForwardID: fixture.forwardID, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: uint64(math.MaxInt64), Packets: 1},
	}
	h.collectNftablesNodeTraffic(fixture.nodeID, cfg, time.Unix(1700000060, 0))

	states, err := h.repo.GetNftCounterStatesByNode(fixture.nodeID)
	if err != nil {
		t.Fatalf("load states: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("expected one state, got %+v", states)
	}
	if states[0].Bytes != 0 || states[0].Packets != 0 {
		t.Fatalf("expected state to remain at old baseline after skipped batch delta, got %+v", states[0])
	}
	if got := mustHandlerCount(t, h, `SELECT in_flow FROM forward WHERE id = ?`, fixture.forwardID); got != 0 {
		t.Fatalf("expected no forward flow for skipped batch delta, got %d", got)
	}
}

func TestCollectNftablesNodeTrafficMetadataErrorDoesNotAdvanceState(t *testing.T) {
	fixture := setupNftablesCollectionFixture(t)
	h := fixture.handler
	manager := &fakeNftablesManager{}
	h.nftablesManager = manager
	cfg := mustCollectionSSHConfig(t, h, fixture.nodeID)

	manager.counterSamples = []runtimenft.CounterSample{
		{ForwardID: fixture.forwardID, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: 1000, Packets: 10},
	}
	h.collectNftablesNodeTraffic(fixture.nodeID, cfg, time.Unix(1700000000, 0))

	if err := h.repo.DB().Exec(`DROP TABLE tunnel`).Error; err != nil {
		t.Fatalf("drop tunnel table: %v", err)
	}
	manager.counterSamples = []runtimenft.CounterSample{
		{ForwardID: fixture.forwardID, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: 1400, Packets: 14},
	}
	h.collectNftablesNodeTraffic(fixture.nodeID, cfg, time.Unix(1700000060, 0))

	states, err := h.repo.GetNftCounterStatesByNode(fixture.nodeID)
	if err != nil {
		t.Fatalf("load states: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("expected one baseline state, got %+v", states)
	}
	if states[0].Bytes != 1000 || states[0].Packets != 10 {
		t.Fatalf("expected state to remain at first baseline after metadata failure, got %+v", states[0])
	}
	if got := mustHandlerCount(t, h, `SELECT in_flow FROM forward WHERE id = ?`, fixture.forwardID); got != 0 {
		t.Fatalf("expected no flow after metadata failure, got %d", got)
	}
}

func TestCollectNftablesNodeTrafficMissingMetaDoesNotAdvanceState(t *testing.T) {
	fixture := setupNftablesHandler(t)
	h := fixture.handler
	seedNftablesSSHConfig(t, h, fixture.nodeID)
	forwardID := int64(4242)
	nowMs := time.Now().UnixMilli()
	if err := h.repo.UpsertNftRuleBinding(repo.NftRuleBindingInput{
		ForwardID:  forwardID,
		NodeID:     fixture.nodeID,
		InPort:     20000,
		Protocols:  "tcp",
		TargetAddr: "203.0.113.9:8080",
		RuleHash:   "hash-a",
		Status:     runtimenft.StatusApplied,
	}, nowMs); err != nil {
		t.Fatalf("seed stale applied binding: %v", err)
	}
	if err := h.repo.UpsertNftCounterStates([]repo.NftCounterStateInput{{
		NodeID:        fixture.nodeID,
		ForwardID:     forwardID,
		Protocol:      "tcp",
		Direction:     runtimenft.CounterDirectionToTarget,
		RuleHash:      "hash-a",
		Bytes:         1000,
		Packets:       10,
		CollectedTime: nowMs,
	}}, nowMs); err != nil {
		t.Fatalf("seed counter state: %v", err)
	}
	h.nftablesManager = &fakeNftablesManager{counterSamples: []runtimenft.CounterSample{
		{ForwardID: forwardID, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: 1400, Packets: 14},
	}}
	cfg := mustCollectionSSHConfig(t, h, fixture.nodeID)

	h.collectNftablesNodeTraffic(fixture.nodeID, cfg, time.Unix(1700000060, 0))

	states, err := h.repo.GetNftCounterStatesByNode(fixture.nodeID)
	if err != nil {
		t.Fatalf("load states: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("expected one state, got %+v", states)
	}
	if states[0].Bytes != 1000 || states[0].Packets != 10 {
		t.Fatalf("expected state to remain at old baseline when meta is missing, got %+v", states[0])
	}
}

func TestCollectNftablesNodeTrafficSkipsSamplesWithoutBinding(t *testing.T) {
	fixture := setupNftablesCollectionFixture(t)
	h := fixture.handler
	if err := h.repo.DeleteNftRuleBindingsByForward(fixture.forwardID); err != nil {
		t.Fatalf("delete nft binding: %v", err)
	}
	h.nftablesManager = &fakeNftablesManager{counterSamples: []runtimenft.CounterSample{
		{ForwardID: fixture.forwardID, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: 1000, Packets: 10},
	}}
	cfg := mustCollectionSSHConfig(t, h, fixture.nodeID)

	h.collectNftablesNodeTraffic(fixture.nodeID, cfg, time.Unix(1700000000, 0))

	states, err := h.repo.GetNftCounterStatesByNode(fixture.nodeID)
	if err != nil {
		t.Fatalf("load states: %v", err)
	}
	if len(states) != 0 {
		t.Fatalf("expected no state for unbound sample, got %+v", states)
	}
	if got := mustHandlerCount(t, h, `SELECT in_flow FROM forward WHERE id = ?`, fixture.forwardID); got != 0 {
		t.Fatalf("expected no flow for unbound sample, got %d", got)
	}
}

func TestCollectNftablesNodeTrafficSkipsNonAppliedBinding(t *testing.T) {
	fixture := setupNftablesCollectionFixture(t)
	h := fixture.handler
	if err := h.repo.MarkNftRuleBindingError(fixture.forwardID, fixture.nodeID, "apply failed", time.Now().UnixMilli()); err != nil {
		t.Fatalf("mark binding error: %v", err)
	}
	h.nftablesManager = &fakeNftablesManager{counterSamples: []runtimenft.CounterSample{
		{ForwardID: fixture.forwardID, Protocol: "tcp", Direction: runtimenft.CounterDirectionToTarget, Bytes: 1000, Packets: 10},
	}}
	cfg := mustCollectionSSHConfig(t, h, fixture.nodeID)

	h.collectNftablesNodeTraffic(fixture.nodeID, cfg, time.Unix(1700000000, 0))

	states, err := h.repo.GetNftCounterStatesByNode(fixture.nodeID)
	if err != nil {
		t.Fatalf("load states: %v", err)
	}
	if len(states) != 0 {
		t.Fatalf("expected no state for non-applied binding, got %+v", states)
	}
	if got := mustHandlerCount(t, h, `SELECT in_flow FROM forward WHERE id = ?`, fixture.forwardID); got != 0 {
		t.Fatalf("expected no flow for non-applied binding, got %d", got)
	}
}

func TestCollectNftablesNodeTrafficCollectionErrorDoesNotWriteState(t *testing.T) {
	fixture := setupNftablesCollectionFixture(t)
	h := fixture.handler
	h.nftablesManager = &fakeNftablesManager{collectErr: errors.New("ssh failed")}
	cfg := mustCollectionSSHConfig(t, h, fixture.nodeID)

	h.collectNftablesNodeTraffic(fixture.nodeID, cfg, time.Unix(1700000000, 0))

	states, err := h.repo.GetNftCounterStatesByNode(fixture.nodeID)
	if err != nil {
		t.Fatalf("load states: %v", err)
	}
	if len(states) != 0 {
		t.Fatalf("expected no state on collection error, got %+v", states)
	}
	if got := mustHandlerCount(t, h, `SELECT in_flow FROM forward WHERE id = ?`, fixture.forwardID); got != 0 {
		t.Fatalf("expected no flow on collection error, got %d", got)
	}
}

type nftablesCollectionFixture struct {
	handler      *Handler
	nodeID       int64
	forwardID    int64
	userTunnelID int64
}

func setupNftablesCollectionFixture(t *testing.T) nftablesCollectionFixture {
	t.Helper()
	fixture := setupNftablesHandler(t)
	h := fixture.handler
	seedNftablesSSHConfig(t, h, fixture.nodeID)
	tunnelID := seedTunnelForNftables(t, h, "nft-traffic-tunnel", fixture.nodeID)
	now := time.Now().UnixMilli()
	if err := h.repo.DB().Exec(`
		INSERT INTO user_tunnel(user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(1, ?, NULL, 99999, 99999, 0, 0, 1, 2727251700000, 1)
	`, tunnelID).Error; err != nil {
		t.Fatalf("seed user_tunnel: %v", err)
	}
	forward := seedForwardForNftables(t, h, tunnelID, fixture.nodeID, "203.0.113.9:8080")
	if err := h.repo.UpsertNftRuleBinding(repo.NftRuleBindingInput{
		ForwardID:  forward.ID,
		NodeID:     fixture.nodeID,
		InPort:     20000,
		Protocols:  "tcp,udp",
		TargetAddr: "203.0.113.9:8080",
		RuleHash:   "hash-a",
		Status:     runtimenft.StatusApplied,
	}, now); err != nil {
		t.Fatalf("seed nft binding: %v", err)
	}
	userTunnelID := mustHandlerCount(t, h, `SELECT id FROM user_tunnel WHERE user_id = 1 AND tunnel_id = ?`, tunnelID)
	return nftablesCollectionFixture{
		handler:      h,
		nodeID:       fixture.nodeID,
		forwardID:    forward.ID,
		userTunnelID: userTunnelID,
	}
}

func mustCollectionSSHConfig(t *testing.T, h *Handler, nodeID int64) *model.NodeSSHConfig {
	t.Helper()
	cfg, err := h.repo.GetNodeSSHConfig(nodeID)
	if err != nil {
		t.Fatalf("load ssh config: %v", err)
	}
	return cfg
}

func mustHandlerCount(t *testing.T, h *Handler, query string, args ...interface{}) int64 {
	t.Helper()
	var value int64
	if err := h.repo.DB().Raw(query, args...).Row().Scan(&value); err != nil {
		t.Fatalf("query %q failed: %v", query, err)
	}
	return value
}
