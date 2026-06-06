package nftables

import (
	"context"
	"errors"
)

type Manager struct {
	runner Runner
}

func NewManager(runner Runner) *Manager {
	if runner == nil {
		runner = NewSSHRunner()
	}
	return &Manager{runner: runner}
}

func (m *Manager) Test(ctx context.Context, cfg SSHConfig) error {
	if err := m.ensureInitialized(); err != nil {
		return err
	}
	return m.runner.Test(ctx, cfg)
}

func (m *Manager) Reconcile(ctx context.Context, cfg SSHConfig, plan NodePlan) (ApplyResult, error) {
	if err := m.ensureInitialized(); err != nil {
		return ApplyResult{}, err
	}
	result := ApplyResult{
		NodeID: plan.NodeID,
		Script: RenderTable(plan),
		Hashes: PlanHashes(plan),
	}
	if err := m.runner.ApplyScript(ctx, cfg, result.Script); err != nil {
		return ApplyResult{}, err
	}
	return result, nil
}

func (m *Manager) Clear(ctx context.Context, cfg SSHConfig) error {
	if err := m.ensureInitialized(); err != nil {
		return err
	}
	script := RenderTable(NodePlan{})
	return m.runner.ApplyScript(ctx, cfg, script)
}

func (m *Manager) CollectCounters(ctx context.Context, cfg SSHConfig) ([]CounterSample, error) {
	if err := m.ensureInitialized(); err != nil {
		return nil, err
	}
	raw, err := m.runner.ListTableJSON(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return ParseCounterSamples(raw)
}

func (m *Manager) ensureInitialized() error {
	if m == nil || m.runner == nil {
		return errors.New("nftables manager not initialized")
	}
	return nil
}
