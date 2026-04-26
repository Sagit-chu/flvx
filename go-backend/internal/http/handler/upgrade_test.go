package handler

import (
	"testing"
	"time"
)

func TestStartNodeOnlineRedeploySkipsRecentReconnects(t *testing.T) {
	h := &Handler{
		pendingUpgradeRedeploy:   map[int64]struct{}{},
		nodeOnlineRedeployAt:     map[int64]time.Time{},
		nodeOnlineRedeployQueued: map[int64]struct{}{},
		nodeOnlineRedeploying:    map[int64]struct{}{},
	}
	now := time.Unix(1_777_176_720, 0)

	if !h.startNodeOnlineRedeploy(54, now) {
		t.Fatalf("expected first reconnect to redeploy")
	}
	h.finishNodeOnlineRedeploy(54)

	if h.startNodeOnlineRedeploy(54, now.Add(5*time.Second)) {
		t.Fatalf("expected recent reconnect to skip redeploy")
	}
	if h.consumeNodePendingUpgradeRedeploy(54) {
		t.Fatalf("did not expect pending upgrade marker to be consumed")
	}
}

func TestStartNodeOnlineRedeployAllowsPendingUpgradeDuringCooldown(t *testing.T) {
	h := &Handler{
		pendingUpgradeRedeploy:   map[int64]struct{}{},
		nodeOnlineRedeployAt:     map[int64]time.Time{},
		nodeOnlineRedeployQueued: map[int64]struct{}{},
		nodeOnlineRedeploying:    map[int64]struct{}{},
	}
	now := time.Unix(1_777_176_720, 0)

	if !h.startNodeOnlineRedeploy(54, now) {
		t.Fatalf("expected first reconnect to redeploy")
	}
	h.finishNodeOnlineRedeploy(54)
	h.markNodePendingUpgradeRedeploy(54)

	if !h.startNodeOnlineRedeploy(54, now.Add(5*time.Second)) {
		t.Fatalf("expected pending upgrade reconnect to bypass cooldown")
	}
	if h.consumeNodePendingUpgradeRedeploy(54) {
		t.Fatalf("expected pending upgrade marker to be consumed during redeploy")
	}
}

func TestStartNodeOnlineRedeployQueuesCooldownReconnect(t *testing.T) {
	h := &Handler{
		pendingUpgradeRedeploy:   map[int64]struct{}{},
		nodeOnlineRedeployAt:     map[int64]time.Time{},
		nodeOnlineRedeployQueued: map[int64]struct{}{},
		nodeOnlineRedeploying:    map[int64]struct{}{},
	}
	now := time.Unix(1_777_176_720, 0)

	if !h.startNodeOnlineRedeploy(54, now) {
		t.Fatalf("expected first reconnect to redeploy")
	}
	h.finishNodeOnlineRedeploy(54)

	if h.startNodeOnlineRedeploy(54, now.Add(5*time.Second)) {
		t.Fatalf("expected cooldown reconnect to skip immediate redeploy")
	}
	if _, queued := h.nodeOnlineRedeployQueued[54]; !queued {
		t.Fatalf("expected cooldown reconnect to queue a follow-up redeploy")
	}
}

func TestStartNodeOnlineRedeployKeepsPendingUpgradeWhileInFlight(t *testing.T) {
	h := &Handler{
		pendingUpgradeRedeploy:   map[int64]struct{}{},
		nodeOnlineRedeployAt:     map[int64]time.Time{},
		nodeOnlineRedeployQueued: map[int64]struct{}{},
		nodeOnlineRedeploying:    map[int64]struct{}{},
	}
	now := time.Unix(1_777_176_720, 0)

	if !h.startNodeOnlineRedeploy(54, now) {
		t.Fatalf("expected first reconnect to redeploy")
	}
	h.markNodePendingUpgradeRedeploy(54)

	if h.startNodeOnlineRedeploy(54, now.Add(time.Second)) {
		t.Fatalf("expected in-flight redeploy to suppress parallel restart")
	}
	if !h.consumeNodePendingUpgradeRedeploy(54) {
		t.Fatalf("expected pending upgrade marker to remain for the next retry")
	}
	h.finishNodeOnlineRedeploy(54)
}

func TestNextNodeOnlineRedeployFireAtDefersExpiredInFlightReconnect(t *testing.T) {
	now := time.Unix(1_777_176_720, 0)
	last := now.Add(-nodeOnlineRedeployCooldown - 5*time.Second)

	fireAt, start := nextNodeOnlineRedeployFireAt(last, now, false, true)
	if start {
		t.Fatalf("expected in-flight reconnect to queue instead of starting immediately")
	}

	want := now.Add(nodeOnlineRedeployCooldown)
	if !fireAt.Equal(want) {
		t.Fatalf("expected queued reconnect at %s, got %s", want, fireAt)
	}
}
