package dashruntime

import (
	"fmt"
	"strings"

	"go-backend/internal/store/model"
)

func BuildForwardRules(
	forward model.ForwardRecord,
	ports []model.ForwardPortRecord,
	entryNodes map[int64]model.NodeRecord,
	chain []model.ChainNodeRecord,
	nodes map[int64]model.NodeRecord,
	nodeSecrets map[int64]string,
	traffic *TrafficPayload,
) ([]RelayRulePayload, error) {
	stagePools, err := BuildStagePools(chain, nodes, nodeSecrets)
	if err != nil {
		return nil, err
	}
	rules := make([]RelayRulePayload, 0, len(ports)*2)
	for _, port := range ports {
		entryNode, ok := entryNodes[port.NodeID]
		if !ok {
			return nil, fmt.Errorf("entry node %d not found", port.NodeID)
		}
		expandedRules, err := BuildForwardRulesForPort(forward, port, entryNode, stagePools, traffic)
		if err != nil {
			return nil, err
		}
		rules = append(rules, expandedRules...)
	}
	return rules, nil
}

func BuildForwardRulesForPort(
	forward model.ForwardRecord,
	port model.ForwardPortRecord,
	entryNode model.NodeRecord,
	stagePools []StagePoolPayload,
	traffic *TrafficPayload,
) ([]RelayRulePayload, error) {
	protocols := []string{"tcp", "udp"}
	rules := make([]RelayRulePayload, 0, len(protocols))
	for _, protocol := range protocols {
		rule, err := buildForwardRule(forward, port, entryNode, protocol, stagePools, traffic)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func BuildForwardTcpRule(
	forward model.ForwardRecord,
	port model.ForwardPortRecord,
	entryNode model.NodeRecord,
	stagePools []StagePoolPayload,
	traffic *TrafficPayload,
) (RelayRulePayload, error) {
	return buildForwardRulePayload(forward, port, entryNode, fmt.Sprintf("forward-%d-node-%d-port-%d-tcp", forward.ID, port.NodeID, port.Port), "tcp", stagePools, traffic)
}

func buildForwardRule(
	forward model.ForwardRecord,
	port model.ForwardPortRecord,
	entryNode model.NodeRecord,
	protocol string,
	stagePools []StagePoolPayload,
	traffic *TrafficPayload,
) (RelayRulePayload, error) {
	return buildForwardRulePayload(forward, port, entryNode, fmt.Sprintf("forward-%d-node-%d-port-%d-%s", forward.ID, port.NodeID, port.Port, protocol), protocol, stagePools, traffic)
}

func buildForwardRulePayload(
	forward model.ForwardRecord,
	port model.ForwardPortRecord,
	entryNode model.NodeRecord,
	ruleID string,
	protocol string,
	stagePools []StagePoolPayload,
	traffic *TrafficPayload,
) (RelayRulePayload, error) {
	listenAddr := entryNode.TCPListenAddr
	if protocol == "udp" {
		listenAddr = entryNode.UDPListenAddr
	}
	listenHost := strings.Trim(listenAddr, "[]")
	if listenHost == "" {
		listenHost = "0.0.0.0"
	}
	description := forward.Name
	return RelayRulePayload{
		ID:          ruleID,
		Protocol:    protocol,
		Listen:      fmt.Sprintf("%s:%d", listenHost, port.Port),
		Enabled:     true,
		Description: &description,
		StagePools:  stagePools,
		TargetPool: TargetPoolPayload{
			Policy:   normalizeStrategy(forward.Strategy),
			Backends: buildTargetBackends(splitTargets(forward.RemoteAddr)),
		},
		Traffic: traffic,
	}, nil
}

func BuildTunnelRule(
	tunnel model.TunnelRecord,
	listenPort int,
	target string,
	chain []model.ChainNodeRecord,
	nodes map[int64]model.NodeRecord,
	nodeSecrets map[int64]string,
	traffic *TrafficPayload,
) (RelayRulePayload, error) {
	stagePools, err := BuildStagePools(chain, nodes, nodeSecrets)
	if err != nil {
		return RelayRulePayload{}, err
	}
	description := fmt.Sprintf("tunnel-%d", tunnel.ID)
	return RelayRulePayload{
		ID:          fmt.Sprintf("tunnel-%d", tunnel.ID),
		Protocol:    "tcp",
		Listen:      fmt.Sprintf("0.0.0.0:%d", listenPort),
		Enabled:     true,
		Description: &description,
		StagePools:  stagePools,
		TargetPool: TargetPoolPayload{
			Policy:   "round_robin",
			Backends: buildTargetBackends([]string{target}),
		},
		Traffic: traffic,
	}, nil
}

func BuildStagePools(chain []model.ChainNodeRecord, nodes map[int64]model.NodeRecord, nodeSecrets map[int64]string) ([]StagePoolPayload, error) {
	stagePools := make([]StagePoolPayload, 0, len(chain))
	for _, hop := range chain {
		node, ok := nodes[hop.NodeID]
		if !ok {
			return nil, fmt.Errorf("chain node %d not found", hop.NodeID)
		}
		secret, ok := nodeSecrets[hop.NodeID]
		if !ok || strings.TrimSpace(secret) == "" {
			return nil, fmt.Errorf("secret for node %d not found", hop.NodeID)
		}
		var bindInterface *string
		if strings.TrimSpace(node.InterfaceName) != "" {
			value := node.InterfaceName
			bindInterface = &value
		}
		stagePools = append(stagePools, StagePoolPayload{
			Policy: normalizeStrategy(hop.Strategy),
			Backends: []StageBackendPayload{{
				ID:            fmt.Sprintf("node-%d", hop.NodeID),
				Server:        fmt.Sprintf("%s:8080", node.ServerIP),
				Token:         secret,
				Enabled:       true,
				Weight:        1,
				BindInterface: bindInterface,
			}},
		})
	}
	return stagePools, nil
}

func buildTargetBackends(targets []string) []TargetBackendPayload {
	backends := make([]TargetBackendPayload, 0, len(targets))
	for index, target := range targets {
		backends = append(backends, TargetBackendPayload{
			ID:      fmt.Sprintf("target-%d", index+1),
			Address: strings.TrimSpace(target),
			Enabled: true,
			Weight:  1,
		})
	}
	return backends
}

func splitTargets(remoteAddr string) []string {
	parts := strings.Split(remoteAddr, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return []string{strings.TrimSpace(remoteAddr)}
	}
	return result
}

func normalizeStrategy(strategy string) string {
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "primary", "primary_backup":
		return "primary_backup"
	case "round", "round_robin":
		return "round_robin"
	case "weighted_round_robin", "wrr":
		return "weighted_round_robin"
	default:
		return "round_robin"
	}
}

func buildActiveExitStagePool(strategy string, nodeID int64, server, token string) (StagePoolPayload, error) {
	server = strings.TrimSpace(server)
	token = strings.TrimSpace(token)
	if server == "" || token == "" {
		return StagePoolPayload{}, fmt.Errorf("active exit not resolved for node %d", nodeID)
	}
	return StagePoolPayload{
		Policy: normalizeStrategy(strategy),
		Backends: []StageBackendPayload{{
			ID:      fmt.Sprintf("active-exit-node-%d", nodeID),
			Server:  server,
			Token:   token,
			Enabled: true,
			Weight:  1,
		}},
	}, nil
}

func BuildActiveExitStagePool(strategy string, nodeID int64, server, token string) (StagePoolPayload, error) {
	return buildActiveExitStagePool(strategy, nodeID, server, token)
}
