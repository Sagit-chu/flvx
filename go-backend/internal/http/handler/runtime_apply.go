package handler

import (
	"context"
	"errors"
	"fmt"
	"log"

	backendruntime "go-backend/internal/runtime"
)

type dashForwardRuntimeApplier interface {
	ApplyForwards(ctx context.Context, tunnelID int64) error
}

type dashTunnelRuntimeApplier interface {
	ApplyTunnel(ctx context.Context, tunnelID int64) error
}

type dashForwardRuntimeDetailApplier interface {
	ApplyForwardsDetailed(ctx context.Context, tunnelID int64) ([]backendruntime.ForwardApplyResult, error)
}

type dashRuleDeleter interface {
	DeleteRule(ctx context.Context, nodeID int64, ruleID string) error
}

func (h *Handler) applyForwardRuntimeForCurrentEngine(ctx context.Context, tunnelID int64, fallback func() error) error {
	_, err := h.applyForwardRuntimeForCurrentEngineWithMetadata(ctx, 0, tunnelID, fallback)
	return err
}

func (h *Handler) applyForwardRuntimeForCurrentEngineWithMetadata(ctx context.Context, forwardID, tunnelID int64, fallback func() error) (*forwardRuntimeMetadata, error) {
	if h == nil {
		return nil, errors.New("handler not initialized")
	}
	client, err := h.currentRuntimeClient()
	if err != nil {
		return nil, err
	}
	if applier, ok := client.(dashForwardRuntimeDetailApplier); ok {
		results, err := applier.ApplyForwardsDetailed(ctx, tunnelID)
		return buildForwardRuntimeMetadata(backendruntime.EngineDash, filterForwardApplyResults(results, forwardID), nil), err
	}
	if applier, ok := client.(dashForwardRuntimeApplier); ok {
		return nil, applier.ApplyForwards(ctx, tunnelID)
	}
	if fallback == nil {
		return nil, nil
	}
	return nil, fallback()
}

func (h *Handler) applyTunnelRuntimeForCurrentEngine(ctx context.Context, state *tunnelCreateState, fallback func() ([]int64, []int64, error)) ([]int64, []int64, error) {
	if h == nil {
		return nil, nil, errors.New("handler not initialized")
	}
	if state == nil {
		return nil, nil, errors.New("invalid tunnel runtime state")
	}
	client, err := h.currentRuntimeClient()
	if err != nil {
		return nil, nil, err
	}
	if applier, ok := client.(dashTunnelRuntimeApplier); ok {
		if err := applier.ApplyTunnel(ctx, state.TunnelID); err != nil {
			return nil, nil, err
		}
		return nil, nil, nil
	}
	if fallback == nil {
		return nil, nil, nil
	}
	return fallback()
}

func (h *Handler) reconcileForwardRuntimeForCurrentEngine(ctx context.Context, forwardID, tunnelID int64, oldPorts, newPorts []forwardPortRecord, fallback func() error) error {
	_, err := h.reconcileForwardRuntimeForCurrentEngineWithMetadata(ctx, forwardID, tunnelID, oldPorts, newPorts, fallback)
	return err
}

func (h *Handler) reconcileForwardRuntimeForCurrentEngineWithMetadata(ctx context.Context, forwardID, tunnelID int64, oldPorts, newPorts []forwardPortRecord, fallback func() error) (*forwardRuntimeMetadata, error) {
	if h == nil {
		return nil, errors.New("handler not initialized")
	}
	client, err := h.currentRuntimeClient()
	if err != nil {
		return nil, err
	}
	if applier, ok := client.(dashForwardRuntimeDetailApplier); ok {
		results, err := applier.ApplyForwardsDetailed(ctx, tunnelID)
		if err == nil {
			if deleter, ok := client.(dashRuleDeleter); ok {
				for _, stale := range staleForwardDashRules(forwardID, oldPorts, newPorts) {
					if delErr := deleter.DeleteRule(ctx, stale.nodeID, stale.ruleID); delErr != nil {
						log.Printf("runtime_apply warning: failed to delete stale dash forward rule node=%d rule=%s: %v", stale.nodeID, stale.ruleID, delErr)
					}
				}
			}
		}
		return buildForwardRuntimeMetadata(backendruntime.EngineDash, filterForwardApplyResults(results, forwardID), nil), err
	}
	applier, ok := client.(dashForwardRuntimeApplier)
	if !ok {
		if fallback == nil {
			return nil, nil
		}
		return nil, fallback()
	}
	if err := applier.ApplyForwards(ctx, tunnelID); err != nil {
		return nil, err
	}
	if deleter, ok := client.(dashRuleDeleter); ok {
		for _, stale := range staleForwardDashRules(forwardID, oldPorts, newPorts) {
			if err := deleter.DeleteRule(ctx, stale.nodeID, stale.ruleID); err != nil {
				log.Printf("runtime_apply warning: failed to delete stale dash forward rule node=%d rule=%s: %v", stale.nodeID, stale.ruleID, err)
			}
		}
	}
	return nil, nil
}

type forwardRuntimeMetadata struct {
	Engine   string                `json:"engine"`
	Overall  string                `json:"overall"`
	Children []forwardRuntimeChild `json:"children"`
	Warnings []string              `json:"warnings"`
}

type forwardRuntimeChild struct {
	NodeID   int64  `json:"nodeId"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	RuleID   string `json:"ruleId"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
}

func buildForwardRuntimeMetadata(engine backendruntime.Engine, results []backendruntime.ForwardApplyResult, extraWarnings []string) *forwardRuntimeMetadata {
	if len(results) == 0 && len(extraWarnings) == 0 {
		return nil
	}
	metadata := &forwardRuntimeMetadata{
		Engine:   string(engine),
		Overall:  string(backendruntime.ForwardApplyStatusSuccess),
		Children: make([]forwardRuntimeChild, 0),
		Warnings: append([]string{}, extraWarnings...),
	}
	for _, result := range results {
		metadata.Overall = combineForwardRuntimeOverall(metadata.Overall, string(result.Status))
		metadata.Warnings = append(metadata.Warnings, result.Warnings...)
		for _, protocol := range result.Protocols {
			metadata.Children = append(metadata.Children, forwardRuntimeChild{
				NodeID:   result.NodeID,
				Port:     result.Port,
				Protocol: protocol.Protocol,
				RuleID:   protocol.RuleID,
				Status:   string(protocol.Status),
				Message:  protocol.Message,
			})
		}
	}
	return metadata
}

func filterForwardApplyResults(results []backendruntime.ForwardApplyResult, forwardID int64) []backendruntime.ForwardApplyResult {
	if forwardID <= 0 {
		return results
	}
	filtered := make([]backendruntime.ForwardApplyResult, 0, len(results))
	for _, result := range results {
		if result.ForwardID == forwardID {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

func combineForwardRuntimeOverall(current, next string) string {
	if current == string(backendruntime.ForwardApplyStatusFailed) || next == string(backendruntime.ForwardApplyStatusFailed) {
		return string(backendruntime.ForwardApplyStatusFailed)
	}
	if current == string(backendruntime.ForwardApplyStatusPartialSuccess) || next == string(backendruntime.ForwardApplyStatusPartialSuccess) {
		return string(backendruntime.ForwardApplyStatusPartialSuccess)
	}
	return string(backendruntime.ForwardApplyStatusSuccess)
}

func (h *Handler) reconcileTunnelRuntimeForCurrentEngine(ctx context.Context, oldChainRows []chainNodeRecord, state *tunnelCreateState, fallback func() ([]int64, []int64, error)) ([]int64, []int64, error) {
	if h == nil {
		return nil, nil, errors.New("handler not initialized")
	}
	if state == nil {
		return nil, nil, errors.New("invalid tunnel runtime state")
	}
	client, err := h.currentRuntimeClient()
	if err != nil {
		return nil, nil, err
	}
	applier, ok := client.(dashTunnelRuntimeApplier)
	if !ok {
		if fallback == nil {
			return nil, nil, nil
		}
		return fallback()
	}
	if err := applier.ApplyTunnel(ctx, state.TunnelID); err != nil {
		return nil, nil, err
	}
	if deleter, ok := client.(dashRuleDeleter); ok {
		for _, stale := range staleTunnelDashRules(state.TunnelID, oldChainRows, state) {
			if err := deleter.DeleteRule(ctx, stale.nodeID, stale.ruleID); err != nil {
				log.Printf("runtime_apply warning: failed to delete stale dash tunnel rule node=%d rule=%s: %v", stale.nodeID, stale.ruleID, err)
			}
		}
	}
	return nil, nil, nil
}

type dashRuleTarget struct {
	nodeID int64
	ruleID string
}

func staleForwardDashRules(forwardID int64, oldPorts, newPorts []forwardPortRecord) []dashRuleTarget {
	if forwardID <= 0 || len(oldPorts) == 0 {
		return nil
	}
	protocols := []string{"tcp", "udp"}
	active := make(map[string]struct{}, len(newPorts))
	for _, port := range newPorts {
		if port.NodeID <= 0 || port.Port <= 0 {
			continue
		}
		active[forwardRuleKey(port.NodeID, port.Port)] = struct{}{}
	}
	stale := make([]dashRuleTarget, 0)
	seen := make(map[string]struct{}, len(oldPorts))
	for _, port := range oldPorts {
		if port.NodeID <= 0 || port.Port <= 0 {
			continue
		}
		key := forwardRuleKey(port.NodeID, port.Port)
		if _, ok := active[key]; ok {
			continue
		}
		for _, protocol := range protocols {
			ruleID := fmt.Sprintf("forward-%d-node-%d-port-%d-%s", forwardID, port.NodeID, port.Port, protocol)
			if _, ok := seen[ruleID]; ok {
				continue
			}
			seen[ruleID] = struct{}{}
			stale = append(stale, dashRuleTarget{nodeID: port.NodeID, ruleID: ruleID})
		}
	}
	return stale
}

func staleTunnelDashRules(tunnelID int64, oldChainRows []chainNodeRecord, state *tunnelCreateState) []dashRuleTarget {
	if tunnelID <= 0 || state == nil || len(oldChainRows) == 0 {
		return nil
	}
	oldEntry, oldOK := firstTunnelEntry(oldChainRows)
	newEntry, newOK := firstRuntimeEntry(state)
	if !oldOK || !newOK {
		return nil
	}
	if oldEntry.NodeID == newEntry.NodeID {
		if oldEntry.Port <= 0 || newEntry.Port <= 0 || oldEntry.Port == newEntry.Port {
			return nil
		}
	}
	if oldEntry.NodeID == newEntry.NodeID && oldEntry.Port == newEntry.Port {
		return nil
	}
	return []dashRuleTarget{{nodeID: oldEntry.NodeID, ruleID: fmt.Sprintf("tunnel-%d", tunnelID)}}
}

func forwardRuleKey(nodeID int64, port int) string {
	return fmt.Sprintf("%d:%d", nodeID, port)
}

func firstTunnelEntry(rows []chainNodeRecord) (chainNodeRecord, bool) {
	for _, row := range rows {
		if row.ChainType == 1 && row.NodeID > 0 && row.Port > 0 {
			return row, true
		}
	}
	return chainNodeRecord{}, false
}

func firstRuntimeEntry(state *tunnelCreateState) (tunnelRuntimeNode, bool) {
	if state == nil {
		return tunnelRuntimeNode{}, false
	}
	for _, node := range state.InNodes {
		if node.NodeID > 0 {
			return node, true
		}
	}
	return tunnelRuntimeNode{}, false
}
