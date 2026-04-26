package handler

import (
	"log"
	"sort"
	"strings"
	"time"

	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
)

type flowPolicyTarget struct {
	UserID       int64
	UserTunnelID int64
}

type flowUploadBatch struct {
	flowDeltas            []repo.FlowUploadCounterDelta
	quotaUsage            map[int64]int64
	policyTargets         []flowPolicyTarget
	forwardTraffic        map[int64]tunnelTrafficDelta
	orphanServices        map[string]struct{}
	peerShareForwardItems map[string]flowItem
	peerShareRuntimeItems map[int64]flowItem
}

func (h *Handler) buildFlowUploadBatch(items []flowItem, metas map[int64]repo.FlowUploadForwardMeta) flowUploadBatch {
	batch := flowUploadBatch{
		quotaUsage:            make(map[int64]int64),
		forwardTraffic:        make(map[int64]tunnelTrafficDelta),
		orphanServices:        make(map[string]struct{}),
		peerShareForwardItems: make(map[string]flowItem),
		peerShareRuntimeItems: make(map[int64]flowItem),
	}
	policySeen := map[flowPolicyTarget]struct{}{}
	flowSeen := map[int64]int{}

	for _, item := range items {
		serviceName := strings.TrimSpace(item.N)
		if serviceName == "" || serviceName == "web_api" {
			continue
		}
		if runtimeID, ok := parsePeerShareRuntimeServiceID(serviceName); ok {
			merged := batch.peerShareRuntimeItems[runtimeID]
			merged.N = serviceName
			merged.U += item.U
			merged.D += item.D
			batch.peerShareRuntimeItems[runtimeID] = merged
			continue
		}
		forwardID, userID, userTunnelID, ok := parseFlowServiceIDs(serviceName)
		if !ok {
			continue
		}
		normalized := normalizeForwardRuntimeServiceName(serviceName)
		merged := batch.peerShareForwardItems[normalized]
		merged.N = normalized
		merged.U += item.U
		merged.D += item.D
		batch.peerShareForwardItems[normalized] = merged

		meta, exists := metas[forwardID]
		if !exists {
			batch.orphanServices[serviceName] = struct{}{}
			continue
		}

		raw := batch.forwardTraffic[forwardID]
		raw.bytesIn += item.D
		raw.bytesOut += item.U
		batch.forwardTraffic[forwardID] = raw

		scaledIn := int64(float64(item.D)*meta.TrafficRatio) * meta.TunnelFlow
		scaledOut := int64(float64(item.U)*meta.TrafficRatio) * meta.TunnelFlow
		if idx, ok := flowSeen[forwardID]; ok {
			batch.flowDeltas[idx].InFlow += scaledIn
			batch.flowDeltas[idx].OutFlow += scaledOut
		} else {
			flowSeen[forwardID] = len(batch.flowDeltas)
			batch.flowDeltas = append(batch.flowDeltas, repo.FlowUploadCounterDelta{
				ForwardID:    forwardID,
				UserID:       userID,
				UserTunnelID: userTunnelID,
				InFlow:       scaledIn,
				OutFlow:      scaledOut,
			})
		}
		batch.quotaUsage[userID] += scaledIn + scaledOut

		target := flowPolicyTarget{UserID: userID, UserTunnelID: userTunnelID}
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

func (h *Handler) applyFlowUploadBatch(nodeID int64, batch flowUploadBatch, now time.Time) {
	if h == nil || h.repo == nil {
		return
	}
	h.applyFlowDeltasWithFallback(nodeID, batch.flowDeltas)
	for userID, quota := range h.applyQuotaUsageWithFallback(nodeID, batch.quotaUsage, now) {
		h.enforceUserQuotaIfNeeded(userID, quota)
	}
	for _, target := range batch.policyTargets {
		if target.UserID <= 0 || target.UserTunnelID <= 0 {
			continue
		}
		h.enforceFlowPolicies(target.UserID, target.UserTunnelID)
	}
	for serviceName := range batch.orphanServices {
		h.sendDeleteOrphanedForwardService(nodeID, serviceName)
	}
	for serviceName, item := range batch.peerShareForwardItems {
		forwardID, _, _, ok := parseFlowServiceIDs(serviceName)
		if ok {
			h.processPeerShareFlowFromForward(forwardID, nodeID, serviceName, item)
		}
	}
	for runtimeID, item := range batch.peerShareRuntimeItems {
		h.processPeerShareFlow(runtimeID, item)
	}
}

func (h *Handler) applyFlowDeltasWithFallback(nodeID int64, deltas []repo.FlowUploadCounterDelta) {
	if h == nil || h.repo == nil || len(deltas) == 0 {
		return
	}
	if err := h.repo.ApplyFlowUploadDeltasBatch(deltas); err == nil {
		return
	} else {
		log.Printf("flow upload write failed op=flow.batch_apply node_id=%d err=%v", nodeID, err)
	}
	for _, delta := range deltas {
		if err := h.repo.AddFlow(delta.ForwardID, delta.UserID, delta.UserTunnelID, delta.InFlow, delta.OutFlow); err != nil {
			log.Printf("flow upload write failed op=flow.single_apply node_id=%d forward_id=%d user_id=%d user_tunnel_id=%d err=%v", nodeID, delta.ForwardID, delta.UserID, delta.UserTunnelID, err)
		}
	}
}

func (h *Handler) applyQuotaUsageWithFallback(nodeID int64, usages map[int64]int64, now time.Time) map[int64]*model.UserQuotaView {
	if h == nil || h.repo == nil || len(usages) == 0 {
		return map[int64]*model.UserQuotaView{}
	}
	quotaViews, err := h.repo.AddUserQuotaUsageBatch(usages, now)
	if err == nil {
		return quotaViews
	}
	log.Printf("flow upload write failed op=quota.batch_apply node_id=%d err=%v", nodeID, err)

	userIDs := make([]int64, 0, len(usages))
	for userID := range usages {
		if userID > 0 {
			userIDs = append(userIDs, userID)
		}
	}
	sort.Slice(userIDs, func(i, j int) bool { return userIDs[i] < userIDs[j] })

	quotaViews = make(map[int64]*model.UserQuotaView, len(userIDs))
	for _, userID := range userIDs {
		quota, singleErr := h.repo.AddUserQuotaUsage(userID, usages[userID], now)
		if singleErr != nil {
			log.Printf("flow upload write failed op=quota.single_apply node_id=%d user_id=%d err=%v", nodeID, userID, singleErr)
			continue
		}
		if quota != nil {
			quotaViews[userID] = quota
		}
	}
	return quotaViews
}
