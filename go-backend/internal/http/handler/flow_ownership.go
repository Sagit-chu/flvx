package handler

import (
	"log"
	"strings"

	"go-backend/internal/store/repo"
)

// Classify ownership before building local counters or enforcing local quotas.
// Numeric IDs from a consuming panel are not IDs in the provider's database.
func (h *Handler) buildNodeFlowUploadBatch(nodeID int64, items []flowItem, metas map[int64]repo.FlowUploadForwardMeta) flowUploadBatch {
	if h == nil || h.repo == nil || nodeID <= 0 {
		return flowUploadBatch{}
	}
	runtimes, err := h.repo.ListActiveForwardPeerShareRuntimesByNode(nodeID)
	if err != nil {
		log.Printf("flow ownership lookup failed node_id=%d err=%v", nodeID, err)
		return flowUploadBatch{}
	}
	resources, err := h.repo.ListPeerShareResourcesByNode(nodeID)
	if err != nil {
		log.Printf("flow resource lookup failed node_id=%d err=%v", nodeID, err)
		return flowUploadBatch{}
	}
	forwardIDs, err := h.repo.ListForwardIDsByNode(nodeID)
	if err != nil {
		log.Printf("flow node ownership lookup failed node_id=%d err=%v", nodeID, err)
		return flowUploadBatch{}
	}
	nodeForwards := make(map[int64]struct{}, len(forwardIDs))
	for _, forwardID := range forwardIDs {
		nodeForwards[forwardID] = struct{}{}
	}
	legacyOwners := make(map[string]map[int64]struct{})
	addLegacyOwner := func(name string, shareID int64) {
		name = normalizeForwardRuntimeServiceName(name)
		if name == "" {
			return
		}
		if legacyOwners[name] == nil {
			legacyOwners[name] = make(map[int64]struct{})
		}
		legacyOwners[name][shareID] = struct{}{}
	}
	for _, runtime := range runtimes {
		addLegacyOwner(runtime.ServiceName, runtime.ShareID)
	}
	resourceOwners := make(map[string]int64)
	for _, resource := range resources {
		// A migrated TCP service may still own a legacy UDP sibling. This alias
		// remains authoritative until the whole legacy family is acknowledged gone.
		addLegacyOwner(resource.LegacyServiceBase, resource.ShareID)
		// A deletion intent does not prove the listener is gone. Failed or
		// timed-out commands retain ownership until the node acknowledges it.
		if resource.Kind == "service" && (resource.DesiredState != "deleted" || resource.Applied == 0) {
			resourceOwners[resource.RuntimeName] = resource.ShareID
		}
	}
	localItems := make([]flowItem, 0, len(items))
	sharedUsage := make(map[int64]int64)
	for _, item := range items {
		name := strings.TrimSpace(item.N)
		if strings.HasPrefix(name, "peer-share-") {
			shareID, _, ok := parsePeerShareServiceName(name)
			if ok && resourceOwners[name] == shareID && item.U >= 0 && item.D >= 0 {
				sharedUsage[shareID] += item.U + item.D
			}
			// Unknown or confirmed-deleted scoped names must never become local IDs.
			continue
		}
		if owners := legacyOwners[normalizeForwardRuntimeServiceName(name)]; len(owners) > 0 {
			forwardID, userID, userTunnelID, parsed := parseFlowServiceIDs(name)
			meta, local := metas[forwardID]
			_, onNode := nodeForwards[forwardID]
			if parsed && local && onNode && meta.UserID == userID && meta.UserTunnelID == userTunnelID {
				// Legacy names can be genuinely ambiguous. Preserve the listener
				// but do not debit either owner based on an ID guess.
				log.Printf("ambiguous legacy flow ownership node_id=%d service=%s", nodeID, name)
				continue
			}
			if len(owners) == 1 && item.U >= 0 && item.D >= 0 {
				for shareID := range owners {
					sharedUsage[shareID] += item.U + item.D
				}
			}
			continue
		}
		if forwardID, userID, _, ok := parseFlowServiceIDs(name); ok {
			if meta, exists := metas[forwardID]; exists {
				if _, onNode := nodeForwards[forwardID]; !onNode || meta.UserID != userID {
					continue
				}
			}
		}
		localItems = append(localItems, item)
	}
	batch := h.buildFlowUploadBatch(localItems, metas)
	batch.peerShareUsage = sharedUsage
	return batch
}

func (h *Handler) addPeerShareFlow(nodeID, shareID, delta int64) {
	if nodeID <= 0 || shareID <= 0 || delta <= 0 {
		return
	}
	share, err := h.repo.GetPeerShare(shareID)
	if err != nil || share == nil || share.NodeID != nodeID {
		return
	}
	if err := h.repo.AddPeerShareCurrentFlow(shareID, delta); err != nil {
		return
	}
	share, err = h.repo.GetPeerShare(shareID)
	if err == nil && isPeerShareFlowExceeded(share) {
		h.enforcePeerShareFlowLimit(shareID)
	}
}
