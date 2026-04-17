package dashruntime

import (
	"reflect"
	"testing"

	"go-backend/internal/store/model"
)

func TestBuildForwardRulesMapsPortForwardToDashRelayRule(t *testing.T) {
	forward := model.ForwardRecord{ID: 11, Name: "web", RemoteAddr: "10.0.0.10:80,10.0.0.11:80", Strategy: "round"}
	ports := []model.ForwardPortRecord{{NodeID: 3, Port: 8080}}
	entryNodes := map[int64]model.NodeRecord{
		3: {ID: 3, TCPListenAddr: "0.0.0.0", ServerIP: "203.0.113.10"},
	}
	chain := []model.ChainNodeRecord{{NodeID: 9, Strategy: "primary"}}
	nodes := map[int64]model.NodeRecord{
		9: {ID: 9, ServerIP: "198.51.100.20", InterfaceName: "eth0"},
	}
	secrets := map[int64]string{9: "edge-secret"}

	rules, err := BuildForwardRules(forward, ports, entryNodes, chain, nodes, secrets, nil)
	if err != nil {
		t.Fatalf("BuildForwardRules: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].StagePools[0].Backends[0].Server != "198.51.100.20:8080" {
		t.Fatalf("unexpected stage backend: %+v", rules[0].StagePools[0].Backends[0])
	}
	if len(rules[0].TargetPool.Backends) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(rules[0].TargetPool.Backends))
	}
}

func TestBuildForwardRulesExpandsIntoTcpAndUdpRules(t *testing.T) {
	forward := model.ForwardRecord{ID: 11, Name: "web", RemoteAddr: "10.0.0.10:80", Strategy: "round"}
	ports := []model.ForwardPortRecord{{NodeID: 3, Port: 8080}}
	entryNodes := map[int64]model.NodeRecord{
		3: {ID: 3, TCPListenAddr: "0.0.0.0"},
	}
	chain := []model.ChainNodeRecord{{NodeID: 9, Strategy: "primary"}}
	nodes := map[int64]model.NodeRecord{
		9: {ID: 9, ServerIP: "198.51.100.20"},
	}
	secrets := map[int64]string{9: "edge-secret"}

	rules, err := BuildForwardRules(forward, ports, entryNodes, chain, nodes, secrets, nil)
	if err != nil {
		t.Fatalf("BuildForwardRules: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].ID != "forward-11-node-3-port-8080-tcp" {
		t.Fatalf("unexpected tcp rule id: %q", rules[0].ID)
	}
	if rules[0].Protocol != "tcp" {
		t.Fatalf("unexpected tcp protocol: %q", rules[0].Protocol)
	}
	if rules[1].ID != "forward-11-node-3-port-8080-udp" {
		t.Fatalf("unexpected udp rule id: %q", rules[1].ID)
	}
	if rules[1].Protocol != "udp" {
		t.Fatalf("unexpected udp protocol: %q", rules[1].Protocol)
	}
	if rules[0].Listen != "0.0.0.0:8080" || rules[1].Listen != "0.0.0.0:8080" {
		t.Fatalf("expected both rules to listen on 0.0.0.0:8080, got %q and %q", rules[0].Listen, rules[1].Listen)
	}
}

func TestBuildForwardRulesForPortUsesProtocolSpecificListenAddresses(t *testing.T) {
	forward := model.ForwardRecord{ID: 31, Name: "api", RemoteAddr: "10.0.0.10:443", Strategy: "round"}
	port := model.ForwardPortRecord{NodeID: 7, Port: 9443}
	entryNode := model.NodeRecord{ID: 7, TCPListenAddr: "127.0.0.2", UDPListenAddr: "127.0.0.3"}
	stagePools := []StagePoolPayload{{
		Policy:   "round_robin",
		Backends: []StageBackendPayload{{ID: "exit-a", Server: "127.0.0.1:18080", Token: "relay-secret", Enabled: true, Weight: 1}},
	}}

	rules, err := BuildForwardRulesForPort(forward, port, entryNode, stagePools, nil)
	if err != nil {
		t.Fatalf("BuildForwardRulesForPort: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].Protocol != "tcp" || rules[0].Listen != "127.0.0.2:9443" {
		t.Fatalf("unexpected tcp listen mapping: %+v", rules[0])
	}
	if rules[1].Protocol != "udp" || rules[1].Listen != "127.0.0.3:9443" {
		t.Fatalf("unexpected udp listen mapping: %+v", rules[1])
	}
}

func TestBuildForwardRulesReuseStagePoolsAndTargetPoolValuesAcrossProtocols(t *testing.T) {
	forward := model.ForwardRecord{ID: 21, Name: "api", RemoteAddr: "10.0.0.10:443,10.0.0.11:443", Strategy: "primary"}
	ports := []model.ForwardPortRecord{{NodeID: 7, Port: 9443}}
	entryNodes := map[int64]model.NodeRecord{
		7: {ID: 7, TCPListenAddr: "[::]"},
	}
	chain := []model.ChainNodeRecord{{NodeID: 8, Strategy: "round"}}
	nodes := map[int64]model.NodeRecord{
		8: {ID: 8, ServerIP: "198.51.100.30", InterfaceName: "eth9"},
	}
	secrets := map[int64]string{8: "stage-secret"}

	rules, err := BuildForwardRules(forward, ports, entryNodes, chain, nodes, secrets, nil)
	if err != nil {
		t.Fatalf("BuildForwardRules: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if !reflect.DeepEqual(rules[0].StagePools, rules[1].StagePools) {
		t.Fatalf("expected stage pools to match across protocols: %+v vs %+v", rules[0].StagePools, rules[1].StagePools)
	}
	if !reflect.DeepEqual(rules[0].TargetPool, rules[1].TargetPool) {
		t.Fatalf("expected target pools to match across protocols: %+v vs %+v", rules[0].TargetPool, rules[1].TargetPool)
	}
}

func TestBuildTunnelRuleMapsChainNodesToStagePools(t *testing.T) {
	tunnel := model.TunnelRecord{ID: 7}
	chain := []model.ChainNodeRecord{{NodeID: 11, Strategy: "round"}, {NodeID: 12, Strategy: "primary"}}
	nodes := map[int64]model.NodeRecord{
		11: {ID: 11, ServerIP: "203.0.113.10"},
		12: {ID: 12, ServerIP: "198.51.100.20", InterfaceName: "eth1"},
	}
	secrets := map[int64]string{11: "edge-secret", 12: "core-secret"}

	rule, err := BuildTunnelRule(tunnel, 2201, "10.0.0.10:22", chain, nodes, secrets, nil)
	if err != nil {
		t.Fatalf("BuildTunnelRule: %v", err)
	}
	if len(rule.StagePools) != 2 {
		t.Fatalf("expected 2 stage pools, got %d", len(rule.StagePools))
	}
	if rule.StagePools[1].Backends[0].BindInterface == nil || *rule.StagePools[1].Backends[0].BindInterface != "eth1" {
		t.Fatalf("expected bind interface on second stage: %+v", rule.StagePools[1].Backends[0])
	}
}

func TestBuildActiveExitStagePoolUsesCurrentActiveExit(t *testing.T) {
	pool, err := buildActiveExitStagePool("primary", 3, "198.51.100.33:18080", "exit-token")
	if err != nil {
		t.Fatalf("buildActiveExitStagePool: %v", err)
	}
	if pool.Policy != "primary_backup" {
		t.Fatalf("unexpected policy: %+v", pool)
	}
	if len(pool.Backends) != 1 {
		t.Fatalf("expected single active-exit backend, got %+v", pool.Backends)
	}
	if pool.Backends[0].Server != "198.51.100.33:18080" || pool.Backends[0].Token != "exit-token" {
		t.Fatalf("unexpected active-exit backend: %+v", pool.Backends[0])
	}
	if pool.Backends[0].ID != "active-exit-node-3" {
		t.Fatalf("unexpected backend id: %+v", pool.Backends[0])
	}
}
