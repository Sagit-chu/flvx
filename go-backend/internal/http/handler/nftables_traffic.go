package handler

import (
	"context"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	runtimenft "go-backend/internal/runtime/nftables"
	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
)

type nftTrafficDelta struct {
	ForwardID int64
	BytesIn   int64
	BytesOut  int64
}

type nftCounterStateKey struct {
	forwardID int64
	protocol  string
	direction string
}

func (h *Handler) runNftablesTrafficCollectJob(now time.Time) {
	if h == nil || h.repo == nil {
		return
	}
	nodes, err := h.repo.ListNftablesNodesForCollection()
	if err != nil {
		log.Printf("nftables traffic collection failed op=list_nodes err=%v", err)
		return
	}
	for i := range nodes {
		node := &nodes[i]
		h.collectNftablesNodeTraffic(node.NodeID, &node.Config, now)
	}
}

func (h *Handler) collectNftablesNodeTraffic(nodeID int64, cfgModel *model.NodeSSHConfig, now time.Time) {
	if h == nil || h.repo == nil {
		return
	}
	if h.nftablesManager == nil {
		log.Printf("nftables traffic collection failed op=collect node_id=%d err=%v", nodeID, "nftables manager not initialized")
		return
	}
	sshCfg, err := sshConfigFromModel(cfgModel)
	if err != nil {
		log.Printf("nftables traffic collection failed op=ssh_config node_id=%d err=%v", nodeID, err)
		return
	}
	samples, err := h.nftablesManager.CollectCounters(context.Background(), sshCfg)
	if err != nil {
		log.Printf("nftables traffic collection failed op=collect node_id=%d err=%v", nodeID, err)
		return
	}
	oldStates, err := h.repo.GetNftCounterStatesByNode(nodeID)
	if err != nil {
		log.Printf("nftables traffic collection failed op=list_states node_id=%d err=%v", nodeID, err)
		return
	}
	bindings, err := h.repo.ListNftRuleBindingsByNode(nodeID)
	if err != nil {
		log.Printf("nftables traffic collection failed op=list_bindings node_id=%d err=%v", nodeID, err)
		return
	}
	hashes := make(map[int64]string, len(bindings))
	for _, binding := range bindings {
		if strings.ToLower(strings.TrimSpace(binding.Status)) != runtimenft.StatusApplied {
			continue
		}
		ruleHash := strings.TrimSpace(binding.RuleHash)
		if ruleHash == "" {
			continue
		}
		hashes[binding.ForwardID] = ruleHash
	}

	nowMs := now.UnixMilli()
	boundSamples := filterNftCounterSamplesWithBinding(samples, hashes)
	deltas, newStates := buildNftCounterDeltas(nodeID, boundSamples, oldStates, hashes, nowMs)
	if len(newStates) == 0 {
		if len(deltas) != 0 {
			log.Printf("nftables traffic collection skipped suspicious deltas without states node_id=%d deltas=%d", nodeID, len(deltas))
		}
		return
	}

	var metas map[int64]repo.FlowUploadForwardMeta
	forwardIDs := make([]int64, 0, len(deltas))
	for _, delta := range deltas {
		if delta.ForwardID > 0 {
			forwardIDs = append(forwardIDs, delta.ForwardID)
		}
	}
	if len(deltas) != 0 {
		metas, err = h.repo.GetFlowUploadForwardMetas(forwardIDs)
		if err != nil {
			log.Printf("nftables traffic collection failed op=load_flow_metas node_id=%d err=%v", nodeID, err)
			return
		}
		if missingForwardID, ok := firstNftDeltaMissingMeta(deltas, metas); ok {
			log.Printf("nftables traffic collection skipped state advance op=missing_flow_meta node_id=%d forward_id=%d", nodeID, missingForwardID)
			return
		}
	}
	if len(deltas) == 0 {
		if err := h.repo.UpsertNftCounterStates(newStates, nowMs); err != nil {
			log.Printf("nftables traffic collection failed op=upsert_states node_id=%d err=%v", nodeID, err)
			return
		}
		return
	}

	batch := buildNftFlowUploadBatch(deltas, metas)
	if missingForwardID, ok := firstNftBatchMissingDelta(deltas, batch); ok {
		log.Printf("nftables traffic collection skipped state advance op=unaccounted_delta node_id=%d forward_id=%d", nodeID, missingForwardID)
		return
	}
	quotaViews, err := h.repo.ApplyNftTrafficAccounting(batch.flowDeltas, batch.quotaUsage, newStates, now)
	if err != nil {
		log.Printf("nftables traffic collection failed op=accounting node_id=%d err=%v", nodeID, err)
		return
	}
	h.recordTunnelMetricsFromForwardBatch(nodeID, batch.forwardTraffic, metas, nowMs)
	for userID, quota := range quotaViews {
		h.enforceUserQuotaIfNeeded(userID, quota)
	}
	for _, target := range batch.policyTargets {
		if target.UserID <= 0 || target.UserTunnelID <= 0 {
			continue
		}
		h.enforceFlowPolicies(target.UserID, target.UserTunnelID)
	}
}

func firstNftBatchMissingDelta(deltas []nftTrafficDelta, batch flowUploadBatch) (int64, bool) {
	flowSeen := make(map[int64]struct{}, len(batch.flowDeltas))
	for _, delta := range batch.flowDeltas {
		flowSeen[delta.ForwardID] = struct{}{}
	}

	expectedRaw := make(map[int64]tunnelTrafficDelta, len(batch.forwardTraffic))
	for _, delta := range deltas {
		if delta.ForwardID <= 0 || (delta.BytesIn == 0 && delta.BytesOut == 0) {
			continue
		}
		if delta.BytesIn < 0 || delta.BytesOut < 0 {
			return delta.ForwardID, true
		}
		raw := expectedRaw[delta.ForwardID]
		if raw.bytesIn > math.MaxInt64-delta.BytesIn || raw.bytesOut > math.MaxInt64-delta.BytesOut {
			return delta.ForwardID, true
		}
		raw.bytesIn += delta.BytesIn
		raw.bytesOut += delta.BytesOut
		expectedRaw[delta.ForwardID] = raw
	}

	for forwardID, expected := range expectedRaw {
		actual, ok := batch.forwardTraffic[forwardID]
		if !ok || actual.bytesIn != expected.bytesIn || actual.bytesOut != expected.bytesOut {
			return forwardID, true
		}
		if expected.bytesIn != 0 || expected.bytesOut != 0 {
			if _, ok := flowSeen[forwardID]; !ok {
				return forwardID, true
			}
		}
	}
	return 0, false
}

func firstNftDeltaMissingMeta(deltas []nftTrafficDelta, metas map[int64]repo.FlowUploadForwardMeta) (int64, bool) {
	for _, delta := range deltas {
		if delta.ForwardID <= 0 {
			continue
		}
		if _, ok := metas[delta.ForwardID]; !ok {
			return delta.ForwardID, true
		}
	}
	return 0, false
}

func filterNftCounterSamplesWithBinding(samples []runtimenft.CounterSample, hashes map[int64]string) []runtimenft.CounterSample {
	if len(samples) == 0 || len(hashes) == 0 {
		return nil
	}
	filtered := make([]runtimenft.CounterSample, 0, len(samples))
	for _, sample := range samples {
		if _, ok := hashes[sample.ForwardID]; !ok {
			continue
		}
		filtered = append(filtered, sample)
	}
	return filtered
}

func nftCounterKey(forwardID int64, protocol, direction string) nftCounterStateKey {
	return nftCounterStateKey{
		forwardID: forwardID,
		protocol:  strings.ToLower(strings.TrimSpace(protocol)),
		direction: strings.ToLower(strings.TrimSpace(direction)),
	}
}

func buildNftCounterDeltas(nodeID int64, samples []runtimenft.CounterSample, oldStates []model.NftCounterState, hashes map[int64]string, nowMs int64) ([]nftTrafficDelta, []repo.NftCounterStateInput) {
	oldByKey := make(map[nftCounterStateKey]model.NftCounterState, len(oldStates))
	for _, old := range oldStates {
		if old.NodeID != nodeID {
			continue
		}
		oldByKey[nftCounterKey(old.ForwardID, old.Protocol, old.Direction)] = old
	}

	stateInputs := make([]repo.NftCounterStateInput, 0, len(samples))
	deltaByForward := make(map[int64]nftTrafficDelta)
	for _, sample := range samples {
		direction := strings.ToLower(strings.TrimSpace(sample.Direction))
		if direction != runtimenft.CounterDirectionToTarget && direction != runtimenft.CounterDirectionFromTarget {
			continue
		}

		protocol := strings.ToLower(strings.TrimSpace(sample.Protocol))
		if protocol != "tcp" && protocol != "udp" {
			continue
		}
		if sample.Bytes > uint64(math.MaxInt64) || sample.Packets > uint64(math.MaxInt64) {
			continue
		}
		ruleHash := strings.TrimSpace(hashes[sample.ForwardID])
		stateInput := repo.NftCounterStateInput{
			NodeID:        nodeID,
			ForwardID:     sample.ForwardID,
			Protocol:      protocol,
			Direction:     direction,
			RuleHash:      ruleHash,
			Bytes:         sample.Bytes,
			Packets:       sample.Packets,
			CollectedTime: nowMs,
		}

		old, exists := oldByKey[nftCounterKey(sample.ForwardID, protocol, direction)]
		if !exists || old.RuleHash != ruleHash {
			stateInputs = append(stateInputs, stateInput)
			continue
		}
		if old.Bytes < 0 {
			stateInputs = append(stateInputs, stateInput)
			continue
		}
		oldBytes := uint64(old.Bytes)
		if sample.Bytes < oldBytes {
			stateInputs = append(stateInputs, stateInput)
			continue
		}
		rawDelta := sample.Bytes - oldBytes
		if rawDelta == 0 {
			stateInputs = append(stateInputs, stateInput)
			continue
		}

		delta := deltaByForward[sample.ForwardID]
		delta.ForwardID = sample.ForwardID
		rawDeltaInt := int64(rawDelta)
		if direction == runtimenft.CounterDirectionToTarget {
			if delta.BytesIn > math.MaxInt64-rawDeltaInt {
				continue
			}
			delta.BytesIn += rawDeltaInt
		} else {
			if delta.BytesOut > math.MaxInt64-rawDeltaInt {
				continue
			}
			delta.BytesOut += rawDeltaInt
		}
		stateInputs = append(stateInputs, stateInput)
		deltaByForward[sample.ForwardID] = delta
	}

	forwardIDs := make([]int64, 0, len(deltaByForward))
	for forwardID := range deltaByForward {
		forwardIDs = append(forwardIDs, forwardID)
	}
	sort.Slice(forwardIDs, func(i, j int) bool { return forwardIDs[i] < forwardIDs[j] })

	deltas := make([]nftTrafficDelta, 0, len(forwardIDs))
	for _, forwardID := range forwardIDs {
		delta := deltaByForward[forwardID]
		if delta.BytesIn == 0 && delta.BytesOut == 0 {
			continue
		}
		deltas = append(deltas, delta)
	}
	return deltas, stateInputs
}

func buildNftFlowUploadBatch(deltas []nftTrafficDelta, metas map[int64]repo.FlowUploadForwardMeta) flowUploadBatch {
	batch := flowUploadBatch{
		quotaUsage:            make(map[int64]int64),
		forwardTraffic:        make(map[int64]tunnelTrafficDelta),
		orphanServices:        make(map[string]struct{}),
		peerShareForwardItems: make(map[string]flowItem),
		peerShareRuntimeItems: make(map[int64]flowItem),
	}
	policySeen := map[flowPolicyTarget]struct{}{}
	flowSeen := map[int64]int{}

	for _, delta := range deltas {
		meta, exists := metas[delta.ForwardID]
		if !exists {
			continue
		}

		raw := batch.forwardTraffic[delta.ForwardID]
		if delta.BytesIn < 0 || delta.BytesOut < 0 || raw.bytesIn > math.MaxInt64-delta.BytesIn || raw.bytesOut > math.MaxInt64-delta.BytesOut {
			continue
		}

		scaledIn, ok := scaleNftTrafficBytes(delta.BytesIn, meta.TrafficRatio, meta.TunnelFlow)
		if !ok {
			continue
		}
		scaledOut, ok := scaleNftTrafficBytes(delta.BytesOut, meta.TrafficRatio, meta.TunnelFlow)
		if !ok {
			continue
		}
		if scaledIn > math.MaxInt64-scaledOut {
			continue
		}
		quotaDelta := scaledIn + scaledOut
		if batch.quotaUsage[meta.UserID] > math.MaxInt64-quotaDelta {
			continue
		}

		flowIdx, flowExists := flowSeen[delta.ForwardID]
		if flowExists && (batch.flowDeltas[flowIdx].InFlow > math.MaxInt64-scaledIn || batch.flowDeltas[flowIdx].OutFlow > math.MaxInt64-scaledOut) {
			continue
		}

		raw.bytesIn += delta.BytesIn
		raw.bytesOut += delta.BytesOut
		batch.forwardTraffic[delta.ForwardID] = raw

		if flowExists {
			batch.flowDeltas[flowIdx].InFlow += scaledIn
			batch.flowDeltas[flowIdx].OutFlow += scaledOut
		} else {
			flowSeen[delta.ForwardID] = len(batch.flowDeltas)
			batch.flowDeltas = append(batch.flowDeltas, repo.FlowUploadCounterDelta{
				ForwardID:    delta.ForwardID,
				UserID:       meta.UserID,
				UserTunnelID: meta.UserTunnelID,
				InFlow:       scaledIn,
				OutFlow:      scaledOut,
			})
		}
		batch.quotaUsage[meta.UserID] += quotaDelta

		target := flowPolicyTarget{UserID: meta.UserID, UserTunnelID: meta.UserTunnelID}
		if _, seen := policySeen[target]; !seen {
			policySeen[target] = struct{}{}
			batch.policyTargets = append(batch.policyTargets, target)
		}
	}

	sort.Slice(batch.policyTargets, func(i, j int) bool {
		if batch.policyTargets[i].UserID == batch.policyTargets[j].UserID {
			return batch.policyTargets[i].UserTunnelID < batch.policyTargets[j].UserTunnelID
		}
		return batch.policyTargets[i].UserID < batch.policyTargets[j].UserID
	})

	return batch
}

func scaleNftTrafficBytes(bytes int64, ratio float64, tunnelFlow int64) (int64, bool) {
	if bytes < 0 || ratio < 0 || tunnelFlow < 0 {
		return 0, false
	}
	var scaled int64
	if ratio == 1 {
		scaled = bytes
	} else {
		scaledFloat := float64(bytes) * ratio
		if math.IsNaN(scaledFloat) || math.IsInf(scaledFloat, 0) || scaledFloat < 0 || scaledFloat >= math.Pow(2, 63) {
			return 0, false
		}
		scaled = int64(scaledFloat)
	}
	if tunnelFlow != 0 && scaled > math.MaxInt64/tunnelFlow {
		return 0, false
	}
	return scaled * tunnelFlow, true
}
