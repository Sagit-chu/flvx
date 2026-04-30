package handler

import (
	"errors"
	"testing"
	"time"
)

var errBestExitProbeForTest = errors.New("probe failed")

func TestBestExitScoreCombinesLatencyAndLoss(t *testing.T) {
	exit := chainNodeRecord{NodeID: 30, NodeName: "exit-a"}
	score := scoreBestExitCandidate(10, exit, 25, 2, 80, 3)

	if !score.Success {
		t.Fatalf("expected successful score")
	}
	if score.OwnerNodeID != 10 || score.ExitNodeID != 30 {
		t.Fatalf("unexpected owner/exit ids: %+v", score)
	}
	if score.TotalLatency != 105 {
		t.Fatalf("expected total latency 105, got %v", score.TotalLatency)
	}
	if score.TotalLoss < 4.9 || score.TotalLoss > 5.0 {
		t.Fatalf("expected combined loss about 4.94, got %v", score.TotalLoss)
	}
	if score.Score < 599 || score.Score > 600 {
		t.Fatalf("expected score about 599, got %v", score.Score)
	}
}

func TestBestExitScorePenalizesLoss(t *testing.T) {
	stable := scoreBestExitCandidate(10, chainNodeRecord{NodeID: 30}, 80, 0, 80, 0)
	lowLatencyLossy := scoreBestExitCandidate(10, chainNodeRecord{NodeID: 31}, 10, 5, 10, 5)

	if !bestExitScoreLess(stable, lowLatencyLossy) {
		t.Fatalf("expected stable exit to beat low-latency lossy exit: stable=%+v lossy=%+v", stable, lowLatencyLossy)
	}
}

func TestBestExitFailedCandidateSortsLast(t *testing.T) {
	failed := failedBestExitCandidate(10, chainNodeRecord{NodeID: 30}, "dial timeout")
	good := scoreBestExitCandidate(10, chainNodeRecord{NodeID: 31}, 100, 0, 100, 0)

	scores := []bestExitCandidateScore{failed, good}
	sortBestExitScores(scores)

	if scores[0].ExitNodeID != 31 || scores[1].ExitNodeID != 30 {
		t.Fatalf("expected good score first and failed score last, got %+v", scores)
	}
}

func TestBestExitInitialObservationAppliesWithoutSwitch(t *testing.T) {
	m := newBestExitManager()
	key := bestExitOwnerKey{TunnelID: 7, OwnerNodeID: 10}
	now := time.Unix(100, 0)
	candidate := scoreBestExitCandidate(10, chainNodeRecord{NodeID: 31}, 40, 0, 60, 0)

	decision := m.observeScores(key, []bestExitCandidateScore{candidate}, now)
	if decision.Switch {
		t.Fatalf("initial observation should not return switch: %+v", decision)
	}
	if m.decisions[key].AppliedExitNodeID != 31 {
		t.Fatalf("expected applied exit 31, got %+v", m.decisions[key])
	}
}

func TestBestExitDecisionRequiresMinimumAdvantage(t *testing.T) {
	m := newBestExitManager()
	key := bestExitOwnerKey{TunnelID: 7, OwnerNodeID: 10}
	now := time.Unix(100, 0)
	current := scoreBestExitCandidate(10, chainNodeRecord{NodeID: 30}, 100, 0, 100, 0)
	candidate := scoreBestExitCandidate(10, chainNodeRecord{NodeID: 31}, 90, 0, 90, 0)

	m.setApplied(key, 30, now.Add(-time.Minute))
	for i := 0; i < bestExitConfirmationRounds+1; i++ {
		decision := m.observeScores(key, []bestExitCandidateScore{candidate, current}, now.Add(time.Duration(i)*time.Second))
		if decision.Switch {
			t.Fatalf("candidate below minimum advantage should not switch after repeated observations: %+v", decision)
		}
	}
	if m.decisions[key].AppliedExitNodeID != 30 {
		t.Fatalf("expected applied exit to remain 30, got %+v", m.decisions[key])
	}
}

func TestBestExitDecisionSwitchesWithMinimumAdvantage(t *testing.T) {
	m := newBestExitManager()
	key := bestExitOwnerKey{TunnelID: 7, OwnerNodeID: 10}
	now := time.Unix(100, 0)
	current := scoreBestExitCandidate(10, chainNodeRecord{NodeID: 30}, 100, 0, 100, 0)
	candidate := scoreBestExitCandidate(10, chainNodeRecord{NodeID: 31}, 40, 0, 60, 0)

	m.setApplied(key, 30, now.Add(-time.Minute))
	for i := 0; i < bestExitConfirmationRounds-1; i++ {
		decision := m.observeScores(key, []bestExitCandidateScore{candidate, current}, now.Add(time.Duration(i)*time.Second))
		if decision.Switch {
			t.Fatalf("candidate should wait for confirmations before switching: %+v", decision)
		}
	}
	decision := m.observeScores(key, []bestExitCandidateScore{candidate, current}, now.Add((bestExitConfirmationRounds-1)*time.Second))
	if !decision.Switch || decision.ExitNodeID != 31 {
		t.Fatalf("candidate with enough advantage should switch after confirmations: %+v", decision)
	}
}

func TestBestExitConfirmedSwitchDoesNotMarkAppliedUntilSetApplied(t *testing.T) {
	m := newBestExitManager()
	key := bestExitOwnerKey{TunnelID: 7, OwnerNodeID: 10}
	now := time.Unix(100, 0)
	current := scoreBestExitCandidate(10, chainNodeRecord{NodeID: 30}, 100, 0, 100, 0)
	candidate := scoreBestExitCandidate(10, chainNodeRecord{NodeID: 31}, 40, 0, 60, 0)

	m.setApplied(key, 30, now.Add(-time.Minute))
	for i := 0; i < bestExitConfirmationRounds-1; i++ {
		decision := m.observeScores(key, []bestExitCandidateScore{candidate, current}, now.Add(time.Duration(i)*time.Second))
		if decision.Switch {
			t.Fatalf("candidate should wait for confirmations before switching: %+v", decision)
		}
	}
	decision := m.observeScores(key, []bestExitCandidateScore{candidate, current}, now.Add((bestExitConfirmationRounds-1)*time.Second))
	if !decision.Switch || decision.ExitNodeID != 31 {
		t.Fatalf("candidate with enough advantage should switch after confirmations: %+v", decision)
	}
	if m.decisions[key].AppliedExitNodeID != 30 {
		t.Fatalf("confirmed switch should not mark applied before runtime update: %+v", m.decisions[key])
	}

	m.setApplied(key, decision.ExitNodeID, now.Add(time.Second))
	if m.decisions[key].AppliedExitNodeID != 31 {
		t.Fatalf("setApplied should commit confirmed switch: %+v", m.decisions[key])
	}
}

func TestBestExitApplyFailureStartsRetryCooldownWithoutChangingAppliedExit(t *testing.T) {
	m := newBestExitManager()
	key := bestExitOwnerKey{TunnelID: 7, OwnerNodeID: 10}
	now := time.Unix(100, 0)
	current := scoreBestExitCandidate(10, chainNodeRecord{NodeID: 30}, 100, 0, 100, 0)
	candidate := scoreBestExitCandidate(10, chainNodeRecord{NodeID: 31}, 40, 0, 60, 0)

	m.setApplied(key, 30, now.Add(-time.Minute))
	for i := 0; i < bestExitConfirmationRounds-1; i++ {
		decision := m.observeScores(key, []bestExitCandidateScore{candidate, current}, now.Add(time.Duration(i)*time.Second))
		if decision.Switch {
			t.Fatalf("candidate should wait for confirmations before switching: %+v", decision)
		}
	}
	confirmed := m.observeScores(key, []bestExitCandidateScore{candidate, current}, now.Add((bestExitConfirmationRounds-1)*time.Second))
	if !confirmed.Switch || confirmed.ExitNodeID != 31 {
		t.Fatalf("expected confirmed switch before apply failure: %+v", confirmed)
	}

	m.recordApplyFailure(key, confirmed.ExitNodeID, now.Add(bestExitConfirmationRounds*time.Second))
	if m.decisions[key].AppliedExitNodeID != 30 {
		t.Fatalf("apply failure should leave applied exit unchanged: %+v", m.decisions[key])
	}

	decision := m.observeScores(key, []bestExitCandidateScore{candidate, current}, now.Add((bestExitConfirmationRounds+1)*time.Second))
	if decision.Switch {
		t.Fatalf("apply retry cooldown should suppress immediate retry: %+v", decision)
	}
	if decision.Reason != "apply retry cooldown" {
		t.Fatalf("expected apply retry cooldown reason, got %q", decision.Reason)
	}

	retry := m.observeScores(key, []bestExitCandidateScore{candidate, current}, now.Add(bestExitConfirmationRounds*time.Second+bestExitApplyRetryCooldown))
	if !retry.Switch || retry.ExitNodeID != 31 {
		t.Fatalf("expected retry after apply cooldown: %+v", retry)
	}
}

func TestBestExitEnsureAppliedDoesNotOverrideExistingAppliedExit(t *testing.T) {
	m := newBestExitManager()
	key := bestExitOwnerKey{TunnelID: 7, OwnerNodeID: 10}
	now := time.Unix(100, 0)

	m.ensureApplied(key, 30, now)
	if m.decisions[key].AppliedExitNodeID != 30 {
		t.Fatalf("expected initial applied exit 30, got %+v", m.decisions[key])
	}
	if !m.decisions[key].LastSwitchAt.Equal(now) {
		t.Fatalf("expected initial applied timestamp, got %+v", m.decisions[key])
	}

	m.ensureApplied(key, 31, now.Add(time.Minute))
	if m.decisions[key].AppliedExitNodeID != 30 {
		t.Fatalf("ensureApplied should not override existing applied exit: %+v", m.decisions[key])
	}
}

func TestBestExitRoundPingerCachesPublicProbeOnly(t *testing.T) {
	publicCalls := 0
	ownerCalls := 0
	pinger := newBestExitRoundPinger(func(nodeID int64, ip string, port int, _ diagnosisExecOptions) (float64, float64, error) {
		if ip == bestExitPublicTargetHost && port == bestExitPublicTargetPort {
			publicCalls++
			return float64(nodeID), 0, nil
		}
		ownerCalls++
		return float64(ownerCalls), 0, nil
	})

	if lat, _, err := pinger(30, bestExitPublicTargetHost, bestExitPublicTargetPort, diagnosisExecOptions{}); err != nil || lat != 30 {
		t.Fatalf("unexpected first public ping result lat=%v err=%v", lat, err)
	}
	if lat, _, err := pinger(30, bestExitPublicTargetHost, bestExitPublicTargetPort, diagnosisExecOptions{}); err != nil || lat != 30 {
		t.Fatalf("unexpected cached public ping result lat=%v err=%v", lat, err)
	}
	if _, _, err := pinger(31, bestExitPublicTargetHost, bestExitPublicTargetPort, diagnosisExecOptions{}); err != nil {
		t.Fatalf("unexpected second exit public ping err=%v", err)
	}
	if publicCalls != 2 {
		t.Fatalf("expected public probes cached per exit node, got %d calls", publicCalls)
	}

	if _, _, err := pinger(10, "10.0.0.30", 30030, diagnosisExecOptions{}); err != nil {
		t.Fatalf("unexpected owner ping err=%v", err)
	}
	if _, _, err := pinger(10, "10.0.0.30", 30030, diagnosisExecOptions{}); err != nil {
		t.Fatalf("unexpected repeated owner ping err=%v", err)
	}
	if ownerCalls != 2 {
		t.Fatalf("expected owner-to-exit probes not cached, got %d calls", ownerCalls)
	}
}

func TestBestExitDecisionScoresAreDefensiveCopies(t *testing.T) {
	m := newBestExitManager()
	key := bestExitOwnerKey{TunnelID: 7, OwnerNodeID: 10}
	now := time.Unix(100, 0)
	candidate := scoreBestExitCandidate(10, chainNodeRecord{NodeID: 31}, 40, 0, 60, 0)
	current := scoreBestExitCandidate(10, chainNodeRecord{NodeID: 30}, 100, 0, 100, 0)

	decision := m.observeScores(key, []bestExitCandidateScore{candidate, current}, now)
	decision.Scores[0].ExitNodeID = 99

	if m.decisions[key].Scores[0].ExitNodeID != 31 {
		t.Fatalf("decision scores mutation leaked into manager state: %+v", m.decisions[key].Scores)
	}
}

func TestBestExitDecisionRequiresConfirmationsAndCooldown(t *testing.T) {
	m := newBestExitManager()
	key := bestExitOwnerKey{TunnelID: 7, OwnerNodeID: 10}
	now := time.Unix(100, 0)
	current := scoreBestExitCandidate(10, chainNodeRecord{NodeID: 30}, 100, 0, 100, 0)
	candidate := scoreBestExitCandidate(10, chainNodeRecord{NodeID: 31}, 40, 0, 60, 0)

	m.setApplied(key, 30, now.Add(-time.Minute))

	if decision := m.observeScores(key, []bestExitCandidateScore{candidate, current}, now); decision.Switch {
		t.Fatalf("first observation should not switch: %+v", decision)
	}
	if decision := m.observeScores(key, []bestExitCandidateScore{candidate, current}, now.Add(time.Second)); decision.Switch {
		t.Fatalf("second observation should not switch: %+v", decision)
	}
	decision := m.observeScores(key, []bestExitCandidateScore{candidate, current}, now.Add(2*time.Second))
	if !decision.Switch || decision.ExitNodeID != 31 {
		t.Fatalf("third confirmed observation should switch to 31: %+v", decision)
	}

	betterAgain := scoreBestExitCandidate(10, chainNodeRecord{NodeID: 30}, 20, 0, 20, 0)
	if decision := m.observeScores(key, []bestExitCandidateScore{betterAgain, candidate}, now.Add(3*time.Second)); decision.Switch {
		t.Fatalf("cooldown should block immediate switch back: %+v", decision)
	}
}

func TestBestExitOrderingUsesAppliedDecision(t *testing.T) {
	m := newBestExitManager()
	key := bestExitOwnerKey{TunnelID: 7, OwnerNodeID: 10}
	m.setApplied(key, 31, time.Unix(100, 0))
	targets := []tunnelRuntimeNode{
		{NodeID: 30, Strategy: tunnelStrategyBest},
		{NodeID: 31, Strategy: tunnelStrategyBest},
		{NodeID: 32, Strategy: tunnelStrategyBest},
	}

	ordered := m.orderTargets(key, targets)
	if ordered[0].NodeID != 31 || ordered[1].NodeID != 30 || ordered[2].NodeID != 32 {
		t.Fatalf("unexpected order: %+v", ordered)
	}
	if targets[0].NodeID != 30 {
		t.Fatalf("orderTargets mutated input: %+v", targets)
	}
}

func TestBuildTunnelChainConfigMapsBestStrategyToFIFO(t *testing.T) {
	nodes := map[int64]*nodeRecord{
		10: {ID: 10, ServerIP: "10.0.0.10", ServerIPv4: "10.0.0.10", TCPListenAddr: "[::]"},
		30: {ID: 30, ServerIP: "10.0.0.30", ServerIPv4: "10.0.0.30", TCPListenAddr: "[::]"},
		31: {ID: 31, ServerIP: "10.0.0.31", ServerIPv4: "10.0.0.31", TCPListenAddr: "[::]"},
	}
	targets := []tunnelRuntimeNode{
		{NodeID: 30, Port: 30030, Protocol: "tls", Strategy: tunnelStrategyBest, ChainType: 3},
		{NodeID: 31, Port: 30031, Protocol: "tls", Strategy: tunnelStrategyBest, ChainType: 3},
	}

	chainData, err := buildTunnelChainConfig(77, 10, targets, nodes, "")
	if err != nil {
		t.Fatalf("build chain: %v", err)
	}
	hops := chainData["hops"].([]map[string]interface{})
	selector := hops[0]["selector"].(map[string]interface{})
	if selector["strategy"] != bestExitRuntimeStrategy {
		t.Fatalf("expected best to render as fifo, got %v", selector["strategy"])
	}
}

func TestHandlerOrdersBestExitTargetsForOwner(t *testing.T) {
	h := &Handler{bestExit: newBestExitManager()}
	key := bestExitOwnerKey{TunnelID: 77, OwnerNodeID: 10}
	h.bestExit.setApplied(key, 31, time.Unix(100, 0))
	targets := []tunnelRuntimeNode{
		{NodeID: 30, Port: 30030, Strategy: tunnelStrategyBest},
		{NodeID: 31, Port: 30031, Strategy: tunnelStrategyBest},
	}

	ordered := h.orderBestExitTargets(77, 10, targets)
	if ordered[0].NodeID != 31 || ordered[1].NodeID != 30 {
		t.Fatalf("unexpected ordered targets: %+v", ordered)
	}
}

func TestRuntimeStrategyForTargetsMapsBestTargetStrategyToFIFO(t *testing.T) {
	owner := tunnelRuntimeNode{Strategy: "round"}
	targets := []tunnelRuntimeNode{{Strategy: tunnelStrategyBest}}

	if got := runtimeStrategyForTargets(owner, targets); got != bestExitRuntimeStrategy {
		t.Fatalf("expected best target strategy to map to fifo, got %q", got)
	}
}

func TestRuntimeStrategyForTargetsPreservesNonBestTargetStrategy(t *testing.T) {
	owner := tunnelRuntimeNode{Strategy: tunnelStrategyBest}
	targets := []tunnelRuntimeNode{{Strategy: "round"}}

	if got := runtimeStrategyForTargets(owner, targets); got != "round" {
		t.Fatalf("expected target strategy round to remain unchanged, got %q", got)
	}
}

func TestRuntimeStrategyForTargetsMapsBestOwnerStrategyWhenTargetsEmpty(t *testing.T) {
	owner := tunnelRuntimeNode{Strategy: tunnelStrategyBest}

	if got := runtimeStrategyForTargets(owner, nil); got != bestExitRuntimeStrategy {
		t.Fatalf("expected best owner fallback strategy to map to fifo, got %q", got)
	}
}

func TestEvaluateBestExitOwnerScoresAllCandidates(t *testing.T) {
	owner := chainNodeRecord{NodeID: 10, NodeName: "entry"}
	exits := []chainNodeRecord{
		{NodeID: 30, NodeName: "exit-a", Port: 30030},
		{NodeID: 31, NodeName: "exit-b", Port: 30031},
	}
	nodes := map[int64]*nodeRecord{
		10: {ID: 10, ServerIP: "10.0.0.10", ServerIPv4: "10.0.0.10", TCPListenAddr: "[::]"},
		30: {ID: 30, ServerIP: "10.0.0.30", ServerIPv4: "10.0.0.30", TCPListenAddr: "[::]"},
		31: {ID: 31, ServerIP: "10.0.0.31", ServerIPv4: "10.0.0.31", TCPListenAddr: "[::]"},
	}
	pinger := func(nodeID int64, ip string, port int, _ diagnosisExecOptions) (float64, float64, error) {
		switch {
		case nodeID == 10 && port == 30030:
			return 60, 0, nil
		case nodeID == 10 && port == 30031:
			return 20, 0, nil
		case nodeID == 30 && ip == bestExitPublicTargetHost:
			return 60, 0, nil
		case nodeID == 31 && ip == bestExitPublicTargetHost:
			return 20, 0, nil
		default:
			t.Fatalf("unexpected ping node=%d ip=%s port=%d", nodeID, ip, port)
			return 0, 100, nil
		}
	}

	scores := evaluateBestExitOwner(owner, exits, nodes, "", diagnosisExecOptions{}, pinger)
	if len(scores) != 2 {
		t.Fatalf("expected two scores, got %+v", scores)
	}
	if scores[0].ExitNodeID != 31 {
		t.Fatalf("expected exit-b first, got %+v", scores)
	}
}

func TestEvaluateBestExitOwnerMarksCandidateFailedWhenOwnerToExitFails(t *testing.T) {
	owner := chainNodeRecord{NodeID: 10, NodeName: "entry"}
	exits := []chainNodeRecord{{NodeID: 30, NodeName: "exit-a", Port: 30030}}
	nodes := map[int64]*nodeRecord{
		10: {ID: 10, ServerIP: "10.0.0.10", ServerIPv4: "10.0.0.10", TCPListenAddr: "[::]"},
		30: {ID: 30, ServerIP: "10.0.0.30", ServerIPv4: "10.0.0.30", TCPListenAddr: "[::]"},
	}
	pinger := func(nodeID int64, ip string, port int, _ diagnosisExecOptions) (float64, float64, error) {
		return 0, 100, errBestExitProbeForTest
	}

	scores := evaluateBestExitOwner(owner, exits, nodes, "", diagnosisExecOptions{}, pinger)
	if len(scores) != 1 || scores[0].Success {
		t.Fatalf("expected failed candidate, got %+v", scores)
	}
}

func TestEvaluateBestExitOwnerMarksCandidateFailedWhenTargetResolutionFails(t *testing.T) {
	owner := chainNodeRecord{NodeID: 10, NodeName: "entry"}
	exits := []chainNodeRecord{{NodeID: 30, NodeName: "exit-v6", Port: 30030}}
	nodes := map[int64]*nodeRecord{
		10: {ID: 10, Name: "entry", ServerIP: "10.0.0.10", ServerIPv4: "10.0.0.10", TCPListenAddr: "[::]"},
		30: {ID: 30, Name: "exit-v6", ServerIP: "2001:db8::30", ServerIPv6: "2001:db8::30", TCPListenAddr: "[::]"},
	}
	pinger := func(nodeID int64, ip string, port int, _ diagnosisExecOptions) (float64, float64, error) {
		t.Fatalf("ping should not be called when target resolution fails: node=%d ip=%s port=%d", nodeID, ip, port)
		return 0, 100, nil
	}

	scores := evaluateBestExitOwner(owner, exits, nodes, "v4", diagnosisExecOptions{}, pinger)
	if len(scores) != 1 || scores[0].Success {
		t.Fatalf("expected failed candidate, got %+v", scores)
	}
}
