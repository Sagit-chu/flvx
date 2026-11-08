package handler

import (
	"errors"
	"fmt"
	"go-backend/internal/store/model"
	"gorm.io/gorm"
)

func tunnelStateRuntimeRows(state *tunnelCreateState) []chainNodeRecord {
	if state.Type != 2 {
		return nil
	}
	var rows []chainNodeRecord
	for _, n := range state.InNodes {
		rows = append(rows, chainNodeRecord{NodeID: n.NodeID, ChainType: 1})
	}
	for _, hop := range state.ChainHops {
		for _, n := range hop {
			rows = append(rows, chainNodeRecord{NodeID: n.NodeID, ChainType: 2, Port: n.Port})
		}
	}
	for _, n := range state.OutNodes {
		rows = append(rows, chainNodeRecord{NodeID: n.NodeID, ChainType: 3, Port: n.Port})
	}
	return rows
}
func (h *Handler) queueObsoleteTunnelRuntimeTx(tx *gorm.DB, tunnelID int64, oldRows, newRows []chainNodeRecord) error {
	var items []model.TunnelRuntimeCleanup
	for _, nodeID := range removedTunnelRuntimeNodeIDs(oldRows, newRows, tunnelRuntimeNeedsService) {
		port := 0
		for _, row := range oldRows {
			if row.NodeID == nodeID {
				port = row.Port
				break
			}
		}
		items = append(items, model.TunnelRuntimeCleanup{TunnelID: tunnelID, NodeID: nodeID, Kind: "service", Port: port})
	}
	for _, old := range oldRows {
		if !tunnelRuntimeNeedsService(old) {
			continue
		}
		for _, current := range newRows {
			if current.NodeID == old.NodeID && tunnelRuntimeNeedsService(current) && (current.Port != old.Port || current.ChainType != old.ChainType) {
				items = append(items, model.TunnelRuntimeCleanup{TunnelID: tunnelID, NodeID: old.NodeID, Kind: "service", Port: old.Port})
			}
		}
	}
	for _, nodeID := range removedTunnelRuntimeNodeIDs(oldRows, newRows, tunnelRuntimeNeedsChain) {
		items = append(items, model.TunnelRuntimeCleanup{TunnelID: tunnelID, NodeID: nodeID, Kind: "chain"})
	}
	return h.repo.QueueTunnelRuntimeCleanupTx(tx, items)
}

// Caller holds the tunnel lifecycle lock. Recheck current desired roles before
// every retry so a delayed deletion cannot remove a re-added runtime.
func (h *Handler) retryTunnelRuntimeCleanupLocked(tunnelID int64) error {
	items, err := h.repo.ListTunnelRuntimeCleanup(tunnelID)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	tunnel, err := h.repo.GetTunnelRecord(tunnelID)
	if err != nil {
		return err
	}
	var rows []chainNodeRecord
	if tunnel != nil && tunnel.Type == 2 {
		rows, err = h.listChainNodesForTunnel(tunnelID)
		if err != nil {
			return err
		}
	}
	var failures []error
	liveServices := map[int64]bool{}
	for _, item := range items {
		desired := false
		for _, row := range rows {
			if row.NodeID == item.NodeID && ((item.Kind == "service" && tunnelRuntimeNeedsService(row)) || (item.Kind == "chain" && tunnelRuntimeNeedsChain(row))) {
				desired = true
				break
			}
		}
		if desired && item.Kind == "service" {
			// Desired state does not prove that the old receiver was replaced.
			// Keep this barrier (even at the same port) and protect its old
			// chain until acknowledgeTunnelServiceReplacements observes ACK.
			liveServices[item.NodeID] = true
			continue
		}
		if !desired {
			if item.Kind == "chain" && liveServices[item.NodeID] {
				continue
			}
			var commandErr error
			if item.Kind == "service" {
				_, commandErr = h.sendNodeCommand(item.NodeID, "DeleteService", map[string]interface{}{"services": tunnelRuntimeServiceNames(tunnelID)}, false, true)
			} else {
				_, commandErr = h.sendNodeCommand(item.NodeID, "DeleteChains", map[string]interface{}{"chain": fmt.Sprintf("chains_%d", tunnelID)}, false, true)
			}
			if commandErr != nil {
				if item.Kind == "service" {
					liveServices[item.NodeID] = true
				}
				failures = append(failures, fmt.Errorf("节点 %d 旧运行时清理失败: %w", item.NodeID, commandErr))
				continue
			}
		}
		if err := h.repo.DeleteTunnelRuntimeCleanup(item.ID); err != nil {
			if item.Kind == "service" {
				liveServices[item.NodeID] = true
			}
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (h *Handler) acknowledgeTunnelServiceReplacements(tunnelID int64, nodeIDs []int64) error {
	if len(nodeIDs) == 0 {
		return nil
	}
	items, err := h.repo.ListTunnelRuntimeCleanup(tunnelID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Kind == "service" {
			for _, nodeID := range nodeIDs {
				if item.NodeID == nodeID {
					if err := h.repo.DeleteTunnelRuntimeCleanup(item.ID); err != nil {
						return err
					}
					break
				}
			}
		}
	}
	return nil
}
func (h *Handler) retryPendingTunnelRuntimeCleanup() error {
	return h.retryPendingTunnelRuntimeCleanupOnNode(0)
}

func (h *Handler) retryPendingTunnelRuntimeCleanupOnNode(nodeID int64) error {
	items, err := h.repo.ListTunnelRuntimeCleanup(0)
	if err != nil {
		return err
	}
	seen := map[int64]bool{}
	var failures []error
	for _, item := range items {
		if nodeID > 0 && item.NodeID != nodeID {
			continue
		}
		if seen[item.TunnelID] {
			continue
		}
		seen[item.TunnelID] = true
		unlock := h.lockTunnelLifecycle(item.TunnelID)
		err := h.retryTunnelRuntimeCleanupLocked(item.TunnelID)
		unlock()
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// The desired tunnel has already been deleted; callers must not roll back a
// completed forward migration merely because asynchronous cleanup is pending.
type tunnelDeletionCleanupPendingError struct{ cause error }

func (e *tunnelDeletionCleanupPendingError) Error() string {
	return "隧道配置已删除，运行时清理待重试: " + e.cause.Error()
}
func (e *tunnelDeletionCleanupPendingError) Unwrap() error { return e.cause }
