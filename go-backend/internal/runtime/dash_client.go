package runtime

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	"go-backend/internal/dashruntime"
	httpclient "go-backend/internal/http/client"
	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
)

type dashRuntimeAPI interface {
	PauseServices(ctx context.Context, node httpclient.DashRuntimeNode, services []string) error
	ResumeServices(ctx context.Context, node httpclient.DashRuntimeNode, services []string) error
	CheckService(ctx context.Context, node httpclient.DashRuntimeNode, req httpclient.DashServiceCheckRequest) (httpclient.DashServiceCheckResponse, error)
	GetStatus(ctx context.Context, node httpclient.DashRuntimeNode) (httpclient.DashStatusResponse, error)
	UpsertRule(ctx context.Context, node httpclient.DashRuntimeNode, rule dashruntime.RelayRulePayload) error
	DeleteRule(ctx context.Context, node httpclient.DashRuntimeNode, ruleID string) error
}

type DashRuntimeClient struct {
	repo   *repo.Repository
	client dashRuntimeAPI
}

func NewDashRuntimeClient(store *repo.Repository, client dashRuntimeAPI) *DashRuntimeClient {
	return &DashRuntimeClient{repo: store, client: client}
}

func (c *DashRuntimeClient) EnsureNodeRuntime(context.Context, repo.Node) (NodeRuntimeProgress, error) {
	return NodeRuntimeProgress{Engine: EngineDash, State: ProgressStateSucceeded, Complete: true}, nil
}

func (c *DashRuntimeClient) RebuildAllRuntime(ctx context.Context) (RebuildRuntimeProgress, error) {
	if c.client == nil {
		return RebuildRuntimeProgress{Engine: EngineDash, State: ProgressStateFailed, Message: "dash runtime client not configured", Complete: true}, fmt.Errorf("dash runtime client not configured")
	}
	if c.repo == nil {
		return RebuildRuntimeProgress{Engine: EngineDash, State: ProgressStateFailed, Message: "repository not configured", Complete: true}, fmt.Errorf("repository not configured")
	}

	tunnelIDs, err := c.repo.ListEnabledTunnelIDs()
	if err != nil {
		return RebuildRuntimeProgress{Engine: EngineDash, State: ProgressStateFailed, Message: err.Error(), Complete: true}, err
	}
	skipped := make([]string, 0)
	for _, tunnelID := range tunnelIDs {
		tunnelName := fmt.Sprintf("tunnel-%d", tunnelID)
		if tunnel, lookupErr := c.lookupTunnelName(tunnelID); lookupErr == nil && strings.TrimSpace(tunnel) != "" {
			tunnelName = tunnel
		}
		if err := c.rebuildTunnelRules(ctx, tunnelID); err != nil {
			skipped = append(skipped, fmt.Sprintf("tunnel %d (%s): %v", tunnelID, tunnelName, err))
			continue
		}
		if err := c.rebuildForwardRules(ctx, tunnelID); err != nil {
			skipped = append(skipped, fmt.Sprintf("tunnel %d (%s): %v", tunnelID, tunnelName, err))
			continue
		}
	}
	progress := RebuildRuntimeProgress{Engine: EngineDash, State: ProgressStateSucceeded, Complete: true}
	if len(skipped) > 0 {
		progress.Message = "skipped tunnels: " + strings.Join(skipped, "; ")
		progress.Warnings = append([]string(nil), skipped...)
	}
	return progress, nil
}

func (c *DashRuntimeClient) lookupTunnelName(tunnelID int64) (string, error) {
	if c == nil || c.repo == nil {
		return "", fmt.Errorf("repository not configured")
	}
	var tunnel model.Tunnel
	if err := c.repo.DB().Select("name").First(&tunnel, tunnelID).Error; err != nil {
		return "", err
	}
	return tunnel.Name, nil
}

func (c *DashRuntimeClient) GetNodeRuntimeStatus(ctx context.Context, node repo.Node) (NodeRuntimeStatus, error) {
	status := NodeRuntimeStatus{
		NodeID:   node.ID,
		Engine:   EngineDash,
		Ready:    false,
		Progress: ProgressStatePending,
		Message:  "dash runtime readiness verification not implemented",
	}
	if c.client == nil {
		status.Message = "dash runtime client not configured"
	}
	if c.client != nil {
		statusResponse, err := c.client.GetStatus(ctx, dashRuntimeNode(node))
		if err != nil {
			status.Message = err.Error()
			return status, nil
		}
		if statusResponse.ExitState.Active != nil {
			status.ActiveExit = &ActiveExitStatus{
				Server: strings.TrimSpace(statusResponse.ExitState.Active.Server),
				Token:  strings.TrimSpace(statusResponse.ExitState.Active.Token),
			}
		}
		if status.ActiveExit == nil || status.ActiveExit.Server == "" || status.ActiveExit.Token == "" {
			status.Message = "dash active exit not ready"
			return status, nil
		}
		status.Ready = true
		status.Progress = ProgressStateSucceeded
		status.Message = "dash runtime ready"
	}
	return status, nil
}

func (c *DashRuntimeClient) PauseServices(ctx context.Context, node repo.Node, services []string) error {
	if c.client == nil {
		return fmt.Errorf("dash runtime client not configured")
	}
	return c.client.PauseServices(ctx, dashRuntimeNode(node), services)
}

func (c *DashRuntimeClient) ResumeServices(ctx context.Context, node repo.Node, services []string) error {
	if c.client == nil {
		return fmt.Errorf("dash runtime client not configured")
	}
	return c.client.ResumeServices(ctx, dashRuntimeNode(node), services)
}

func (c *DashRuntimeClient) CheckService(ctx context.Context, node repo.Node, req ServiceCheckRequest) (ServiceCheckResult, error) {
	if c.client == nil {
		return ServiceCheckResult{}, fmt.Errorf("dash runtime client not configured")
	}
	res, err := c.client.CheckService(ctx, dashRuntimeNode(node), httpclient.DashServiceCheckRequest{
		Type:       req.Type,
		Target:     req.Target,
		TimeoutSec: req.TimeoutSec,
	})
	if err != nil {
		return ServiceCheckResult{}, err
	}
	return ServiceCheckResult{
		Success:      res.Success,
		LatencyMs:    res.LatencyMs,
		StatusCode:   res.StatusCode,
		ErrorMessage: res.ErrorMessage,
	}, nil
}

func (c *DashRuntimeClient) ApplyTunnel(ctx context.Context, tunnelID int64) error {
	if c.client == nil {
		return fmt.Errorf("dash runtime client not configured")
	}
	if c.repo == nil {
		return fmt.Errorf("repository not configured")
	}
	return c.rebuildTunnelRules(ctx, tunnelID)
}

func (c *DashRuntimeClient) ApplyForwards(ctx context.Context, tunnelID int64) error {
	results, err := c.ApplyForwardsDetailed(ctx, tunnelID)
	if err != nil {
		return err
	}
	for _, result := range results {
		if result.Status == ForwardApplyStatusPartialSuccess {
			return result
		}
	}
	return nil
}

func (c *DashRuntimeClient) ApplyForwardsDetailed(ctx context.Context, tunnelID int64) ([]ForwardApplyResult, error) {
	if c.client == nil {
		return nil, fmt.Errorf("dash runtime client not configured")
	}
	if c.repo == nil {
		return nil, fmt.Errorf("repository not configured")
	}
	return c.rebuildForwardRulesDetailed(ctx, tunnelID)
}

func (c *DashRuntimeClient) DeleteRule(ctx context.Context, nodeID int64, ruleID string) error {
	if c.client == nil {
		return fmt.Errorf("dash runtime client not configured")
	}
	if c.repo == nil {
		return fmt.Errorf("repository not configured")
	}
	_, targetNode, err := c.loadRuntimeNode(nodeID)
	if err != nil {
		return err
	}
	return c.client.DeleteRule(ctx, targetNode, ruleID)
}

func dashRuntimeNode(node repo.Node) httpclient.DashRuntimeNode {
	return httpclient.DashRuntimeNode{
		ServerIP: node.ServerIP,
		Secret:   node.Secret,
	}
}

func (c *DashRuntimeClient) rebuildForwardRules(ctx context.Context, tunnelID int64) error {
	_, err := c.rebuildForwardRulesDetailed(ctx, tunnelID)
	return err
}

func (c *DashRuntimeClient) rebuildForwardRulesDetailed(ctx context.Context, tunnelID int64) ([]ForwardApplyResult, error) {
	tunnel, err := c.repo.GetTunnelRecord(tunnelID)
	if err != nil {
		return nil, err
	}
	if tunnel == nil {
		return nil, fmt.Errorf("tunnel %d not found", tunnelID)
	}

	chainRows, err := c.repo.ListChainNodesForTunnel(tunnelID)
	if err != nil {
		return nil, err
	}
	forwardChain := filterChainRows(chainRows, func(row model.ChainNodeRecord) bool {
		return row.ChainType != 1
	})
	chainNodes, chainSecrets, err := c.loadChainResources(forwardChain)
	if err != nil {
		return nil, err
	}
	stagePools, err := dashruntime.BuildStagePools(forwardChain, chainNodes, chainSecrets)
	if err != nil {
		return nil, err
	}

	forwards, err := c.repo.ListForwardsByTunnel(tunnelID)
	if err != nil {
		return nil, err
	}
	results := make([]ForwardApplyResult, 0)
	for _, forward := range forwards {
		if forward.Status != 1 {
			continue
		}
		ports, err := c.repo.ListForwardPorts(forward.ID)
		if err != nil {
			return results, err
		}
		targetNodes := make(map[int64]httpclient.DashRuntimeNode, len(ports))
		entryNodes := make(map[int64]model.NodeRecord, len(ports))
		for _, port := range ports {
			nodeRecord, targetNode, err := c.loadRuntimeNode(port.NodeID)
			if err != nil {
				return results, err
			}
			entryNodes[port.NodeID] = nodeRecord
			targetNodes[port.NodeID] = targetNode
		}

		for _, port := range ports {
			targetNode, ok := targetNodes[port.NodeID]
			if !ok {
				return results, fmt.Errorf("dash runtime target node %d not found", port.NodeID)
			}

			portStagePools := stagePools
			if tunnel.Type == 1 {
				activeExitStagePool, err := c.loadActiveExitStagePool(ctx, port.NodeID, targetNode, forward.Strategy)
				if err != nil {
					return results, err
				}
				portStagePools = []dashruntime.StagePoolPayload{activeExitStagePool}
			}

			rules, err := dashruntime.BuildForwardRulesForPort(forward, port, entryNodes[port.NodeID], portStagePools, nil)
			if err != nil {
				return results, err
			}

			result := c.applyForwardRuleSet(ctx, targetNode, forward.ID, port, rules)
			results = append(results, result)
			if result.Status == ForwardApplyStatusFailed {
				return results, fmt.Errorf("forward %d node %d port %d failed to apply tcp rule", forward.ID, port.NodeID, port.Port)
			}
		}
	}
	return results, nil
}

func (c *DashRuntimeClient) applyForwardRuleSet(ctx context.Context, targetNode httpclient.DashRuntimeNode, forwardID int64, port model.ForwardPortRecord, rules []dashruntime.RelayRulePayload) ForwardApplyResult {
	result := ForwardApplyResult{
		ForwardID: forwardID,
		NodeID:    port.NodeID,
		Port:      port.Port,
		Status:    ForwardApplyStatusSuccess,
		Protocols: make([]ForwardProtocolApplyResult, 0, len(rules)),
	}
	for _, rule := range rules {
		protocolResult := ForwardProtocolApplyResult{
			Protocol: rule.Protocol,
			RuleID:   rule.ID,
			Status:   ForwardApplyStatusSuccess,
		}
		if err := c.client.UpsertRule(ctx, targetNode, rule); err != nil {
			protocolResult.Status = ForwardApplyStatusFailed
			protocolResult.Message = err.Error()
			if rule.Protocol == "udp" {
				result.Warnings = append(result.Warnings, fmt.Sprintf("UDP 子规则创建失败: %v", err))
			}
		}
		result.Protocols = append(result.Protocols, protocolResult)
	}
	result.Status = overallForwardApplyStatus(result.Protocols)
	return result
}

func overallForwardApplyStatus(protocols []ForwardProtocolApplyResult) ForwardApplyStatus {
	if len(protocols) == 0 {
		return ForwardApplyStatusFailed
	}
	for _, protocol := range protocols {
		if protocol.Protocol == "tcp" && protocol.Status != ForwardApplyStatusSuccess {
			return ForwardApplyStatusFailed
		}
	}
	for _, protocol := range protocols {
		if protocol.Status != ForwardApplyStatusSuccess {
			return ForwardApplyStatusPartialSuccess
		}
	}
	return ForwardApplyStatusSuccess
}

func (c *DashRuntimeClient) loadActiveExitStagePool(ctx context.Context, nodeID int64, targetNode httpclient.DashRuntimeNode, strategy string) (dashruntime.StagePoolPayload, error) {
	status, err := c.client.GetStatus(ctx, targetNode)
	if err != nil {
		return dashruntime.StagePoolPayload{}, err
	}
	if status.ExitState.Active == nil {
		return dashruntime.StagePoolPayload{}, fmt.Errorf("entry node %d missing active exit for dash forward: exit_state=%+v", nodeID, status.ExitState)
	}
	stagePool, err := dashruntime.BuildActiveExitStagePool(strategy, nodeID, status.ExitState.Active.Server, status.ExitState.Active.Token)
	if err != nil {
		return dashruntime.StagePoolPayload{}, fmt.Errorf("entry node %d missing active exit for dash forward: exit_state=%+v", nodeID, status.ExitState)
	}
	return stagePool, nil
}

func (c *DashRuntimeClient) rebuildTunnelRules(ctx context.Context, tunnelID int64) error {
	tunnel, err := c.repo.GetTunnelRecord(tunnelID)
	if err != nil || tunnel == nil || tunnel.Type != 2 {
		return err
	}

	chainRows, err := c.repo.ListChainNodesForTunnel(tunnelID)
	if err != nil {
		return err
	}
	inNodes, chainHops, outNodes := splitChainNodeGroups(chainRows)
	if len(inNodes) == 0 || len(outNodes) == 0 || inNodes[0].Port <= 0 || outNodes[0].Port <= 0 {
		return nil
	}

	_, targetNode, err := c.loadRuntimeNode(inNodes[0].NodeID)
	if err != nil {
		return err
	}
	chain := flattenChainHops(chainHops)
	chainNodes, chainSecrets, err := c.loadChainResources(chain)
	if err != nil {
		return err
	}
	outNode, err := c.repo.GetNodeRecord(outNodes[0].NodeID)
	if err != nil {
		return err
	}
	if outNode == nil {
		return fmt.Errorf("node %d not found", outNodes[0].NodeID)
	}
	host := strings.TrimSpace(outNodes[0].ConnectIP)
	if host == "" {
		host = strings.TrimSpace(outNode.ServerIP)
	}
	if host == "" {
		return nil
	}
	rule, err := dashruntime.BuildTunnelRule(*tunnel, inNodes[0].Port, net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(outNodes[0].Port)), chain, chainNodes, chainSecrets, nil)
	if err != nil {
		return err
	}
	return c.client.UpsertRule(ctx, targetNode, rule)
}

func (c *DashRuntimeClient) loadRuntimeNode(nodeID int64) (model.NodeRecord, httpclient.DashRuntimeNode, error) {
	node, err := c.repo.GetNodeRecord(nodeID)
	if err != nil {
		return model.NodeRecord{}, httpclient.DashRuntimeNode{}, err
	}
	if node == nil {
		return model.NodeRecord{}, httpclient.DashRuntimeNode{}, fmt.Errorf("node %d not found", nodeID)
	}
	secret, err := c.repo.GetNodeSecret(nodeID)
	if err != nil {
		return model.NodeRecord{}, httpclient.DashRuntimeNode{}, err
	}
	return *node, httpclient.DashRuntimeNode{ServerIP: node.ServerIP, Secret: secret}, nil
}

func (c *DashRuntimeClient) loadChainResources(rows []model.ChainNodeRecord) (map[int64]model.NodeRecord, map[int64]string, error) {
	nodes := make(map[int64]model.NodeRecord, len(rows))
	secrets := make(map[int64]string, len(rows))
	for _, row := range rows {
		if _, ok := nodes[row.NodeID]; ok {
			continue
		}
		node, err := c.repo.GetNodeRecord(row.NodeID)
		if err != nil {
			return nil, nil, err
		}
		if node == nil {
			return nil, nil, fmt.Errorf("node %d not found", row.NodeID)
		}
		secret, err := c.repo.GetNodeSecret(row.NodeID)
		if err != nil {
			return nil, nil, err
		}
		nodes[row.NodeID] = *node
		secrets[row.NodeID] = secret
	}
	return nodes, secrets, nil
}

func filterChainRows(rows []model.ChainNodeRecord, keep func(model.ChainNodeRecord) bool) []model.ChainNodeRecord {
	filtered := make([]model.ChainNodeRecord, 0, len(rows))
	for _, row := range rows {
		if keep(row) {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func splitChainNodeGroups(rows []model.ChainNodeRecord) ([]model.ChainNodeRecord, [][]model.ChainNodeRecord, []model.ChainNodeRecord) {
	inNodes := make([]model.ChainNodeRecord, 0)
	outNodes := make([]model.ChainNodeRecord, 0)
	chainByInx := map[int64][]model.ChainNodeRecord{}
	hopOrder := make([]int64, 0)

	for _, row := range rows {
		switch row.ChainType {
		case 1:
			inNodes = append(inNodes, row)
		case 2:
			if _, ok := chainByInx[row.Inx]; !ok {
				hopOrder = append(hopOrder, row.Inx)
			}
			chainByInx[row.Inx] = append(chainByInx[row.Inx], row)
		case 3:
			outNodes = append(outNodes, row)
		}
	}

	chainHops := make([][]model.ChainNodeRecord, 0, len(hopOrder))
	for _, inx := range hopOrder {
		chainHops = append(chainHops, chainByInx[inx])
	}
	return inNodes, chainHops, outNodes
}

func flattenChainHops(hops [][]model.ChainNodeRecord) []model.ChainNodeRecord {
	flattened := make([]model.ChainNodeRecord, 0)
	for _, hop := range hops {
		flattened = append(flattened, hop...)
	}
	return flattened
}
