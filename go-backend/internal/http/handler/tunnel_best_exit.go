package handler

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	tunnelStrategyBest              = "best"
	bestExitRuntimeStrategy         = "fifo"
	bestExitPublicTargetHost        = "www.bing.com"
	bestExitPublicTargetPort        = 443
	bestExitLossPenaltyMsPerPercent = 100.0
	bestExitConfirmationRounds      = 3
	bestExitSwitchCooldown          = 30 * time.Second
	bestExitApplyRetryCooldown      = bestExitSwitchCooldown
	bestExitMinLatencyAdvantageMs   = 20.0
	bestExitMinScoreAdvantageRatio  = 0.15
)

type bestExitOwnerKey struct {
	TunnelID    int64
	OwnerNodeID int64
}

type bestExitCandidateScore struct {
	OwnerNodeID int64
	ExitNodeID  int64
	ExitName    string

	OwnerToExitLatency float64
	ExitToBingLatency  float64
	OwnerToExitLoss    float64
	ExitToBingLoss     float64
	TotalLatency       float64
	TotalLoss          float64
	Score              float64
	Success            bool
	ErrorMessage       string
}

type bestExitSwitchDecision struct {
	Switch     bool
	ExitNodeID int64
	Reason     string
	Scores     []bestExitCandidateScore
}

type bestExitProbeFunc func(nodeID int64, ip string, port int, options diagnosisExecOptions) (latency float64, loss float64, err error)

type bestExitProbeResult struct {
	latency float64
	loss    float64
	err     error
}

type bestExitDecision struct {
	AppliedExitNodeID          int64
	PendingExitNodeID          int64
	PendingCount               int
	LastSwitchAt               time.Time
	LastApplyFailureAt         time.Time
	LastApplyFailureExitNodeID int64
	LastReason                 string
	Scores                     []bestExitCandidateScore
}

type bestExitManager struct {
	mu        sync.Mutex
	decisions map[bestExitOwnerKey]*bestExitDecision
}

func newBestExitManager() *bestExitManager {
	return &bestExitManager{decisions: make(map[bestExitOwnerKey]*bestExitDecision)}
}

func isBestTunnelStrategy(strategy string) bool {
	return strings.EqualFold(strings.TrimSpace(strategy), tunnelStrategyBest)
}

func runtimeTunnelStrategy(strategy string) string {
	if isBestTunnelStrategy(strategy) {
		return bestExitRuntimeStrategy
	}
	return strategy
}

func scoreBestExitCandidate(ownerNodeID int64, exit chainNodeRecord, ownerLatency, ownerLoss, publicLatency, publicLoss float64) bestExitCandidateScore {
	totalLatency := ownerLatency + publicLatency
	totalLoss := combineLossPercent(ownerLoss, publicLoss)
	return bestExitCandidateScore{
		OwnerNodeID:        ownerNodeID,
		ExitNodeID:         exit.NodeID,
		ExitName:           exit.NodeName,
		OwnerToExitLatency: ownerLatency,
		ExitToBingLatency:  publicLatency,
		OwnerToExitLoss:    ownerLoss,
		ExitToBingLoss:     publicLoss,
		TotalLatency:       totalLatency,
		TotalLoss:          totalLoss,
		Score:              totalLatency + totalLoss*bestExitLossPenaltyMsPerPercent,
		Success:            true,
	}
}

func failedBestExitCandidate(ownerNodeID int64, exit chainNodeRecord, message string) bestExitCandidateScore {
	return bestExitCandidateScore{
		OwnerNodeID:  ownerNodeID,
		ExitNodeID:   exit.NodeID,
		ExitName:     exit.NodeName,
		Success:      false,
		ErrorMessage: message,
	}
}

func combineLossPercent(a, b float64) float64 {
	a = clampPercent(a)
	b = clampPercent(b)
	return (1 - (1-a/100.0)*(1-b/100.0)) * 100.0
}

func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func sortBestExitScores(scores []bestExitCandidateScore) {
	sort.SliceStable(scores, func(i, j int) bool {
		return bestExitScoreLess(scores[i], scores[j])
	})
}

func evaluateBestExitOwner(owner chainNodeRecord, exits []chainNodeRecord, nodes map[int64]*nodeRecord, ipPreference string, options diagnosisExecOptions, ping bestExitProbeFunc) []bestExitCandidateScore {
	scores := make([]bestExitCandidateScore, 0, len(exits))
	if owner.NodeID <= 0 || len(exits) == 0 || ping == nil {
		return scores
	}
	ownerNode := nodes[owner.NodeID]
	for _, exit := range exits {
		exitNode := nodes[exit.NodeID]
		if exitNode == nil {
			scores = append(scores, failedBestExitCandidate(owner.NodeID, exit, "exit node unavailable"))
			continue
		}
		targetIP, targetPort, resolveErr := resolveBestExitProbeTarget(ownerNode, exitNode, exit.Port, ipPreference, exit.ConnectIP)
		if resolveErr != nil {
			scores = append(scores, failedBestExitCandidate(owner.NodeID, exit, resolveErr.Error()))
			continue
		}
		ownerLatency, ownerLoss, ownerErr := ping(owner.NodeID, targetIP, targetPort, options)
		if ownerErr != nil {
			scores = append(scores, failedBestExitCandidate(owner.NodeID, exit, ownerErr.Error()))
			continue
		}
		publicLatency, publicLoss, publicErr := ping(exit.NodeID, bestExitPublicTargetHost, bestExitPublicTargetPort, options)
		if publicErr != nil {
			scores = append(scores, failedBestExitCandidate(owner.NodeID, exit, publicErr.Error()))
			continue
		}
		scores = append(scores, scoreBestExitCandidate(owner.NodeID, exit, ownerLatency, ownerLoss, publicLatency, publicLoss))
	}
	sortBestExitScores(scores)
	return scores
}

func resolveBestExitProbeTarget(fromNode, targetNode *nodeRecord, preferredPort int, ipPreference string, connectIP string) (string, int, error) {
	if targetNode == nil {
		return "", 0, errors.New("目标节点不存在")
	}
	host, err := selectTunnelDialHost(fromNode, targetNode, ipPreference, connectIP)
	if err != nil {
		return "", 0, err
	}
	if strings.TrimSpace(host) == "" {
		return "", 0, errors.New("目标节点地址为空")
	}
	port := preferredPort
	if port <= 0 {
		port = firstPortFromRange(targetNode.PortRange)
	}
	if port <= 0 {
		port = 443
	}
	return host, port, nil
}

func newBestExitRoundPinger(base bestExitProbeFunc) bestExitProbeFunc {
	cache := make(map[int64]bestExitProbeResult)
	return func(nodeID int64, ip string, port int, options diagnosisExecOptions) (float64, float64, error) {
		if ip == bestExitPublicTargetHost && port == bestExitPublicTargetPort {
			if cached, ok := cache[nodeID]; ok {
				return cached.latency, cached.loss, cached.err
			}
			lat, loss, err := base(nodeID, ip, port, options)
			cache[nodeID] = bestExitProbeResult{latency: lat, loss: loss, err: err}
			return lat, loss, err
		}
		return base(nodeID, ip, port, options)
	}
}

func bestExitChainOwners(inNodes []chainNodeRecord, chainHops [][]chainNodeRecord) []chainNodeRecord {
	if len(chainHops) == 0 {
		return inNodes
	}
	return chainHops[len(chainHops)-1]
}

func chainRecordsToRuntimeTargets(rows []chainNodeRecord) []tunnelRuntimeNode {
	out := make([]tunnelRuntimeNode, 0, len(rows))
	for _, row := range rows {
		out = append(out, tunnelRuntimeNode{
			NodeID:    row.NodeID,
			Protocol:  row.Protocol,
			Strategy:  row.Strategy,
			Inx:       int(row.Inx),
			ChainType: row.ChainType,
			Port:      row.Port,
			ConnectIP: row.ConnectIP,
		})
	}
	return out
}

func orderRuntimeTargetsByNodeID(targets []tunnelRuntimeNode, orderedIDs []int64) []tunnelRuntimeNode {
	out := append([]tunnelRuntimeNode(nil), targets...)
	if len(out) <= 1 || len(orderedIDs) == 0 {
		return out
	}
	positions := make(map[int64]int, len(orderedIDs))
	for i, id := range orderedIDs {
		if _, ok := positions[id]; !ok {
			positions[id] = i
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi, iok := positions[out[i].NodeID]
		pj, jok := positions[out[j].NodeID]
		if iok != jok {
			return iok
		}
		if iok && jok && pi != pj {
			return pi < pj
		}
		return false
	})
	return out
}

func cloneBestExitScores(scores []bestExitCandidateScore) []bestExitCandidateScore {
	return append([]bestExitCandidateScore(nil), scores...)
}

func bestExitDecisionResult(switchNow bool, exitNodeID int64, reason string, scores []bestExitCandidateScore) bestExitSwitchDecision {
	return bestExitSwitchDecision{Switch: switchNow, ExitNodeID: exitNodeID, Reason: reason, Scores: cloneBestExitScores(scores)}
}

func bestExitScoreLess(a, b bestExitCandidateScore) bool {
	if a.Success != b.Success {
		return a.Success
	}
	if !a.Success && !b.Success {
		return a.ExitNodeID < b.ExitNodeID
	}
	if a.Score != b.Score {
		return a.Score < b.Score
	}
	return a.ExitNodeID < b.ExitNodeID
}

func bestExitHasMinimumAdvantage(candidate, current bestExitCandidateScore) bool {
	if !candidate.Success {
		return false
	}
	if !current.Success {
		return true
	}
	improvement := current.Score - candidate.Score
	threshold := current.Score * bestExitMinScoreAdvantageRatio
	if threshold < bestExitMinLatencyAdvantageMs {
		threshold = bestExitMinLatencyAdvantageMs
	}
	return improvement >= threshold
}

func (m *bestExitManager) setApplied(key bestExitOwnerKey, exitNodeID int64, at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.decisionLocked(key)
	d.AppliedExitNodeID = exitNodeID
	d.PendingExitNodeID = 0
	d.PendingCount = 0
	d.LastApplyFailureAt = time.Time{}
	d.LastApplyFailureExitNodeID = 0
	d.LastSwitchAt = at
}

func (m *bestExitManager) recordApplyFailure(key bestExitOwnerKey, exitNodeID int64, at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.decisionLocked(key)
	d.LastApplyFailureAt = at
	d.LastApplyFailureExitNodeID = exitNodeID
	d.LastReason = "apply retry cooldown"
}

func (m *bestExitManager) ensureApplied(key bestExitOwnerKey, exitNodeID int64, at time.Time) {
	if m == nil || exitNodeID <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	d := m.decisionLocked(key)
	if d.AppliedExitNodeID == 0 {
		d.AppliedExitNodeID = exitNodeID
		d.LastSwitchAt = at
	}
}

func (m *bestExitManager) observeScores(key bestExitOwnerKey, scores []bestExitCandidateScore, now time.Time) bestExitSwitchDecision {
	m.mu.Lock()
	defer m.mu.Unlock()

	ordered := append([]bestExitCandidateScore(nil), scores...)
	sortBestExitScores(ordered)
	d := m.decisionLocked(key)
	d.Scores = cloneBestExitScores(ordered)

	if len(ordered) == 0 || !ordered[0].Success {
		d.LastReason = "all exits failed"
		return bestExitDecisionResult(false, 0, d.LastReason, ordered)
	}

	candidate := ordered[0]
	if d.AppliedExitNodeID == 0 {
		d.AppliedExitNodeID = candidate.ExitNodeID
		d.LastSwitchAt = now
		d.LastReason = "initial best exit"
		return bestExitDecisionResult(false, 0, d.LastReason, ordered)
	}
	if candidate.ExitNodeID == d.AppliedExitNodeID {
		d.PendingExitNodeID = 0
		d.PendingCount = 0
		d.LastApplyFailureAt = time.Time{}
		d.LastApplyFailureExitNodeID = 0
		d.LastReason = "current exit remains best"
		return bestExitDecisionResult(false, 0, d.LastReason, ordered)
	}
	if candidate.ExitNodeID == d.LastApplyFailureExitNodeID && !d.LastApplyFailureAt.IsZero() && now.Sub(d.LastApplyFailureAt) < bestExitApplyRetryCooldown {
		d.LastReason = "apply retry cooldown"
		return bestExitDecisionResult(false, 0, d.LastReason, ordered)
	}
	if now.Sub(d.LastSwitchAt) < bestExitSwitchCooldown {
		d.LastReason = "cooldown"
		return bestExitDecisionResult(false, 0, d.LastReason, ordered)
	}

	current := findBestExitScore(ordered, d.AppliedExitNodeID)
	if !bestExitHasMinimumAdvantage(candidate, current) {
		d.PendingExitNodeID = 0
		d.PendingCount = 0
		d.LastReason = "insufficient advantage"
		return bestExitDecisionResult(false, 0, d.LastReason, ordered)
	}

	if d.PendingExitNodeID != candidate.ExitNodeID {
		d.PendingExitNodeID = candidate.ExitNodeID
		d.PendingCount = 1
		d.LastReason = "candidate pending confirmation"
		return bestExitDecisionResult(false, 0, d.LastReason, ordered)
	}
	d.PendingCount++
	if d.PendingCount < bestExitConfirmationRounds {
		d.LastReason = "candidate pending confirmation"
		return bestExitDecisionResult(false, 0, d.LastReason, ordered)
	}

	d.LastReason = "switch confirmed"
	return bestExitDecisionResult(true, candidate.ExitNodeID, d.LastReason, ordered)
}

func findBestExitScore(scores []bestExitCandidateScore, exitNodeID int64) bestExitCandidateScore {
	for _, score := range scores {
		if score.ExitNodeID == exitNodeID {
			return score
		}
	}
	return failedBestExitCandidate(0, chainNodeRecord{NodeID: exitNodeID}, "current exit has no successful score")
}

func (m *bestExitManager) decisionLocked(key bestExitOwnerKey) *bestExitDecision {
	if d := m.decisions[key]; d != nil {
		return d
	}
	d := &bestExitDecision{}
	m.decisions[key] = d
	return d
}

func (m *bestExitManager) orderTargets(key bestExitOwnerKey, targets []tunnelRuntimeNode) []tunnelRuntimeNode {
	out := append([]tunnelRuntimeNode(nil), targets...)
	if m == nil || len(out) <= 1 {
		return out
	}
	m.mu.Lock()
	applied := int64(0)
	if d := m.decisions[key]; d != nil {
		applied = d.AppliedExitNodeID
	}
	m.mu.Unlock()
	if applied <= 0 {
		return out
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].NodeID == applied {
			return true
		}
		if out[j].NodeID == applied {
			return false
		}
		return false
	})
	return out
}
