package handler

import (
	"context"
)

func (h *Handler) applyForwardRuntimeForCurrentEngine(ctx context.Context, tunnelID int64, fallback func() error) error {
	if fallback == nil {
		return nil
	}
	return fallback()
}

func (h *Handler) applyForwardRuntimeForCurrentEngineWithMetadata(ctx context.Context, forwardID, tunnelID int64, fallback func() error) (*forwardRuntimeMetadata, error) {
	if fallback == nil {
		return nil, nil
	}
	return nil, fallback()
}

func (h *Handler) applyTunnelRuntimeForCurrentEngine(ctx context.Context, state *tunnelCreateState, fallback func() ([]int64, []int64, error)) ([]int64, []int64, error) {
	if fallback == nil {
		return nil, nil, nil
	}
	return fallback()
}

func (h *Handler) reconcileForwardRuntimeForCurrentEngine(ctx context.Context, forwardID, tunnelID int64, oldPorts, newPorts []forwardPortRecord, fallback func() error) error {
	if fallback == nil {
		return nil
	}
	return fallback()
}

func (h *Handler) reconcileForwardRuntimeForCurrentEngineWithMetadata(ctx context.Context, forwardID, tunnelID int64, oldPorts, newPorts []forwardPortRecord, fallback func() error) (*forwardRuntimeMetadata, error) {
	if fallback == nil {
		return nil, nil
	}
	return nil, fallback()
}

func (h *Handler) reconcileTunnelRuntimeForCurrentEngine(ctx context.Context, oldChainRows []chainNodeRecord, state *tunnelCreateState, fallback func() ([]int64, []int64, error)) ([]int64, []int64, error) {
	if fallback == nil {
		return nil, nil, nil
	}
	return fallback()
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
