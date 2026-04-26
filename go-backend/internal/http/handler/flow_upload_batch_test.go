package handler

import (
	"testing"

	"go-backend/internal/store/repo"
)

func TestBuildFlowUploadBatchAggregatesForwardQuotaPeerShareAndCleanupTargets(t *testing.T) {
	h := &Handler{}
	metas := map[int64]repo.FlowUploadForwardMeta{
		20: {
			ForwardID:    20,
			TunnelID:     1,
			TrafficRatio: 2,
			TunnelFlow:   3,
		},
	}

	batch := h.buildFlowUploadBatch([]flowItem{
		{N: "20_2_10", U: 70, D: 50},
		{N: "20_2_10_tcp", U: 40, D: 30},
		{N: "99_2_10", U: 12, D: 8},
		{N: "fed_svc_17", U: 9, D: 1},
	}, metas)

	if len(batch.flowDeltas) != 1 {
		t.Fatalf("expected 1 flow delta, got %d", len(batch.flowDeltas))
	}
	delta := batch.flowDeltas[0]
	if delta.ForwardID != 20 || delta.UserID != 2 || delta.UserTunnelID != 10 {
		t.Fatalf("unexpected flow delta identity: %#v", delta)
	}
	if delta.InFlow != 480 || delta.OutFlow != 660 {
		t.Fatalf("expected scaled flow in=480 out=660, got in=%d out=%d", delta.InFlow, delta.OutFlow)
	}
	if batch.quotaUsage[2] != 1140 {
		t.Fatalf("expected quota usage 1140, got %d", batch.quotaUsage[2])
	}
	if len(batch.policyTargets) != 1 {
		t.Fatalf("expected 1 policy target, got %d", len(batch.policyTargets))
	}
	if batch.policyTargets[0].UserID != 2 || batch.policyTargets[0].UserTunnelID != 10 {
		t.Fatalf("unexpected policy target: %#v", batch.policyTargets[0])
	}
	traffic := batch.forwardTraffic[20]
	if traffic.bytesIn != 80 || traffic.bytesOut != 110 {
		t.Fatalf("expected raw traffic in=80 out=110, got in=%d out=%d", traffic.bytesIn, traffic.bytesOut)
	}
	if _, ok := batch.orphanServices["99_2_10"]; !ok {
		t.Fatalf("expected orphan service cleanup target for 99_2_10")
	}
	if item, ok := batch.peerShareForwardItems["20_2_10"]; !ok || item.U != 110 || item.D != 80 {
		t.Fatalf("expected merged peer-share forward item, got %#v ok=%v", item, ok)
	}
	if item, ok := batch.peerShareRuntimeItems[17]; !ok || item.U != 9 || item.D != 1 {
		t.Fatalf("expected merged peer-share runtime item, got %#v ok=%v", item, ok)
	}
}
