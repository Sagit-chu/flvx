package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go-backend/internal/store/repo"
)

// Serialize desired-state changes with reconnect reconciliation. In particular,
// a stale reconnect snapshot must never revive an acknowledged release.
var peerRoleRuntimeMu sync.Mutex

// applyPeerShareRoleRuntime requires peerRoleRuntimeMu. Persist the validated
// desired configuration before creating resources, so failed or interrupted
// commands remain recoverable rather than producing untracked listeners.
func (h *Handler) applyPeerShareRoleRuntime(runtime *repo.PeerShareRuntime) error {
	if runtime.ReleasePending != 0 || runtime.Status != 1 {
		return errors.New("runtime is being released")
	}
	if runtime.Role != "middle" && runtime.Role != "exit" {
		return errors.New("invalid runtime role")
	}
	node, err := h.getNodeRecord(runtime.NodeID)
	if err != nil {
		return err
	}
	var targets []federationRuntimeTarget
	if strings.TrimSpace(runtime.Target) != "" {
		if err := json.Unmarshal([]byte(runtime.Target), &targets); err != nil {
			return err
		}
	}
	var chain map[string]interface{}
	if runtime.Role == "middle" {
		chain, err = buildFederationMiddleChainConfig(runtime.ChainName, runtime.ID, runtime.Protocol, runtime.Strategy, targets, node.InterfaceName)
		if err != nil {
			return err
		}
	}
	service := buildFederationServiceConfig(runtime.ServiceName, fmt.Sprintf("%s:%d", node.TCPListenAddr, runtime.Port), runtime.Protocol, runtime.Role, runtime.ChainName, len(targets), node.InterfaceName)
	runtime.Applied = 0
	runtime.UpdatedTime = time.Now().UnixMilli()
	if err := h.repo.UpdatePeerShareRuntime(runtime); err != nil {
		return err
	}
	if h.wsServer == nil {
		return errors.New("node command transport unavailable")
	}
	if chain != nil {
		if _, err := h.sendNodeCommand(runtime.NodeID, "UpdateChains", updateChainPayload(runtime.ChainName, chain), false, false); err != nil {
			return err
		}
	}
	// UpdateService and UpdateChains are upserts, including on an empty agent.
	if _, err := h.sendNodeCommand(runtime.NodeID, "UpdateService", []map[string]interface{}{service}, false, false); err != nil {
		return err
	}
	runtime.Applied = 1
	runtime.UpdatedTime = time.Now().UnixMilli()
	return h.repo.UpdatePeerShareRuntime(runtime)
}

func (h *Handler) releasePeerShareRuntime(runtime *repo.PeerShareRuntime) error {
	peerRoleRuntimeMu.Lock()
	defer peerRoleRuntimeMu.Unlock()
	if runtime == nil {
		return nil
	}
	current, err := h.repo.GetPeerShareRuntimeByID(runtime.ID)
	if err != nil {
		return err
	}
	if current != nil && (current.ReservationID != runtime.ReservationID || current.BindingID != runtime.BindingID) {
		// A completed reservation may have been reused while this release
		// waited for the mutation lock. Never release the new generation.
		return nil
	}
	return h.releasePeerShareRuntimeLocked(current)
}

func (h *Handler) releasePeerShareRuntimeLocked(runtime *repo.PeerShareRuntime) error {
	if runtime == nil || runtime.Status == 0 {
		return nil
	}
	if err := h.repo.SetPeerShareRuntimeReleasePending(runtime.ID); err != nil {
		return err
	}
	runtime.ReleasePending = 1
	if runtime.Role == "forward" {
		if _, _, scoped := parsePeerShareServiceName(runtime.ServiceName); scoped {
			if err := h.releasePeerShareForwardRuntimeResources(runtime); err != nil {
				return err
			}
			return h.repo.CompletePeerShareRuntimeRelease(runtime.ID)
		}
		// Older agents used unscoped names. Do not delete a name shared by
		// another reservation or by a local forward during migration.
		owners, err := h.repo.ListActiveForwardPeerShareRuntimesByNodeAndServiceName(runtime.NodeID, runtime.ServiceName)
		if err != nil {
			return err
		}
		for _, owner := range owners {
			if owner.ShareID != runtime.ShareID {
				return fmt.Errorf("legacy runtime %q has ambiguous ownership", runtime.ServiceName)
			}
		}
		if forwardID, _, _, ok := parseFlowServiceIDs(normalizeForwardRuntimeServiceName(runtime.ServiceName)); ok {
			local, err := h.repo.GetForwardRecord(forwardID)
			if err != nil {
				return err
			}
			if local != nil {
				return fmt.Errorf("legacy runtime %q collides with a local forward", runtime.ServiceName)
			}
		}
	}
	// ServiceName is saved before apply; Applied=0 can therefore mean an
	// unacknowledged command, and must not bypass deletion.
	if strings.TrimSpace(runtime.ServiceName) != "" || strings.TrimSpace(runtime.ChainName) != "" {
		if h.wsServer == nil {
			return errors.New("node command transport unavailable")
		}
		if strings.TrimSpace(runtime.ServiceName) != "" {
			names := []string{runtime.ServiceName}
			if runtime.Role == "forward" {
				names = append(names, runtime.ServiceName+"_tcp", runtime.ServiceName+"_udp")
			}
			if _, err := h.sendNodeCommand(runtime.NodeID, "DeleteService", map[string]interface{}{"services": names}, false, true); err != nil {
				return err
			}
		}
		if strings.TrimSpace(runtime.ChainName) != "" {
			if _, err := h.sendNodeCommand(runtime.NodeID, "DeleteChains", map[string]interface{}{"chain": runtime.ChainName}, false, true); err != nil {
				return err
			}
		}
	}
	return h.repo.CompletePeerShareRuntimeRelease(runtime.ID)
}

func (h *Handler) reconcilePeerShareRoleRuntimesOnNode(nodeID int64) error {
	return h.reconcilePeerShareRoleRuntimes(nodeID, false)
}

// Maintenance retries only unfinished desired state. Replaying acknowledged
// services while an unrelated operation is failing can interrupt live traffic.
func (h *Handler) retryPendingPeerShareRoleRuntimesOnNode(nodeID int64) error {
	return h.reconcilePeerShareRoleRuntimes(nodeID, true)
}

func (h *Handler) reconcilePeerShareRoleRuntimes(nodeID int64, pendingOnly bool) error {
	peerRoleRuntimeMu.Lock()
	defer peerRoleRuntimeMu.Unlock()
	runtimes, err := h.repo.ListActivePeerShareRuntimesByNode(nodeID)
	if err != nil {
		return err
	}
	var reconcileErr error
	for i := range runtimes {
		runtime := &runtimes[i]
		if pendingOnly && runtime.Applied == 1 && runtime.ReleasePending == 0 {
			continue
		}
		if runtime.ReleasePending != 0 {
			reconcileErr = errors.Join(reconcileErr, h.releasePeerShareRuntimeLocked(runtime))
			continue
		}
		if runtime.Role != "middle" && runtime.Role != "exit" {
			continue
		}
		share, err := h.repo.GetPeerShare(runtime.ShareID)
		if err != nil {
			reconcileErr = errors.Join(reconcileErr, err)
			continue
		}
		if share == nil || share.IsActive != 1 || (share.ExpiryTime > 0 && share.ExpiryTime <= time.Now().UnixMilli()) || isPeerShareFlowExceeded(share) {
			reconcileErr = errors.Join(reconcileErr, h.releasePeerShareRuntimeLocked(runtime))
			continue
		}
		if err := h.applyPeerShareRoleRuntime(runtime); err != nil {
			reconcileErr = errors.Join(reconcileErr, fmt.Errorf("shared runtime %d: %w", runtime.ID, err))
		}
	}
	return reconcileErr
}
