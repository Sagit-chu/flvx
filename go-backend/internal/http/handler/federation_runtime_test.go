package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-backend/internal/http/response"
	"go-backend/internal/store/repo"
)

func TestPickPeerSharePortUsesRuntimeReservations(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	h := &Handler{repo: r}
	now := time.Now().UnixMilli()

	if err := r.DB().Exec(`INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol) VALUES(?, ?, ?, ?, ?, ?, ?)`, 1, 2, 1, 3000, "round", 1, "tls").Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}
	if err := r.DB().Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(?, ?, ?)`, 1, 1, 3001).Error; err != nil {
		t.Fatalf("insert forward_port: %v", err)
	}
	if err := r.DB().Exec(`
		INSERT INTO peer_share_runtime(share_id, node_id, reservation_id, resource_key, binding_id, role, chain_name, service_name, protocol, strategy, port, target, applied, status, created_time, updated_time)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, 77, 1, "res-1", "rk-1", "b-1", "exit", "", "fed_svc_1", "tls", "round", 3002, "", 1, 1, now, now).Error; err != nil {
		t.Fatalf("insert peer_share_runtime: %v", err)
	}

	share := &repo.PeerShare{
		ID:             77,
		NodeID:         1,
		PortRangeStart: 3000,
		PortRangeEnd:   3004,
	}

	port, err := h.pickPeerSharePort(share, 0)
	if err != nil {
		t.Fatalf("pick auto port: %v", err)
	}
	if port != 3003 {
		t.Fatalf("expected port 3003, got %d", port)
	}

	if _, err := h.pickPeerSharePort(share, 3001); err == nil {
		t.Fatalf("expected requested busy port to fail")
	}
}

func TestApplyTunnelRuntimeSkipsRemoteChainAndOutNodes(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "rt-skip.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	h := &Handler{repo: r}
	now := time.Now().UnixMilli()
	for _, n := range []struct {
		id   int64
		name string
		ip   string
	}{
		{12, "remote-chain", "10.99.0.2"},
		{13, "remote-out", "10.99.0.3"},
	} {
		if err := r.DB().Exec(`
			INSERT INTO node(id, name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx, is_remote, remote_url, remote_token)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, n.id, n.name, n.name+"-secret", n.ip, n.ip, "", "40000-40010", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0, 1, "http://remote-peer", "remote-token").Error; err != nil {
			t.Fatalf("insert node %s: %v", n.name, err)
		}
	}

	state := &tunnelCreateState{
		TunnelID: 1,
		Type:     2,
		InNodes:  []tunnelRuntimeNode{},
		ChainHops: [][]tunnelRuntimeNode{
			{
				{NodeID: 12, ChainType: 2, Inx: 1, Port: 41000, Protocol: "tls", Strategy: "round"},
			},
		},
		OutNodes: []tunnelRuntimeNode{
			{NodeID: 13, ChainType: 3, Port: 42000, Protocol: "tls", Strategy: "round"},
		},
		Nodes: map[int64]*nodeRecord{
			12: {ID: 12, Name: "remote-chain", IsRemote: 1, ServerIPv4: "10.99.0.2"},
			13: {ID: 13, Name: "remote-out", IsRemote: 1, ServerIPv4: "10.99.0.3"},
		},
	}

	chains, services, err := h.applyTunnelRuntime(state)
	if err != nil {
		t.Fatalf("apply runtime: %v", err)
	}
	if len(chains) != 0 {
		t.Fatalf("expected no local chains for remote-only nodes, got %d", len(chains))
	}
	if len(services) != 0 {
		t.Fatalf("expected no local services for remote-only nodes, got %d", len(services))
	}
}

func TestBuildTunnelTunServiceConfigUsesTunHandlerAndListener(t *testing.T) {
	node := &nodeRecord{TCPListenAddr: "[::]", UDPListenAddr: "[::]"}
	inNode := tunnelRuntimeNode{NodeID: 1, ChainType: 1, Port: 30001}
	tunCfg := map[string]interface{}{"net": "10.0.0.1/24", "name": "demo0", "mtu": 1380}

	services := buildTunnelTunServiceConfig(99, inNode, node, tunCfg)
	if len(services) != 1 {
		t.Fatalf("expected one service, got %d", len(services))
	}
	svc := services[0]

	h, ok := svc["handler"].(map[string]interface{})
	if !ok || asString(h["type"]) != "tun" {
		t.Fatalf("expected handler.type=tun, got %#v", svc["handler"])
	}
	if asString(h["chain"]) != "chains_99" {
		t.Fatalf("expected handler.chain=chains_99, got %v", h["chain"])
	}

	l, ok := svc["listener"].(map[string]interface{})
	if !ok || asString(l["type"]) != "tun" {
		t.Fatalf("expected listener.type=tun, got %#v", svc["listener"])
	}

	lm, ok := l["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected listener metadata")
	}
	if _, ok := lm["tunConfig"]; !ok {
		t.Fatalf("expected listener metadata to include tunConfig")
	}
}

func TestPrepareTunnelCreateStateType3RequiresSingleEntryAndExit(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	h := &Handler{repo: r}
	now := time.Now().UnixMilli()

	insertNode := func(name, ip, portRange string) int64 {
		if execErr := r.DB().Exec(`
			INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx, is_remote, remote_url, remote_token, remote_config)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, name, name+"-secret", ip, ip, "", portRange, "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0, 0, "", "", "").Error; execErr != nil {
			t.Fatalf("insert node %s: %v", name, execErr)
		}
		return mustLastInsertID(t, r, name)
	}

	entry1 := insertNode("entry-1", "10.9.0.1", "30000-30010")
	entry2 := insertNode("entry-2", "10.9.0.2", "30100-30110")
	exit1 := insertNode("exit-1", "10.9.0.3", "30200-30210")

	tx := r.DB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx: %v", tx.Error)
	}
	defer tx.Rollback()

	req := map[string]interface{}{
		"name": "tun-topo-check",
		"inNodeId": []interface{}{
			map[string]interface{}{"nodeId": float64(entry1), "protocol": "tls", "strategy": "round"},
			map[string]interface{}{"nodeId": float64(entry2), "protocol": "tls", "strategy": "round"},
		},
		"chainNodes": []interface{}{},
		"outNodeId": []interface{}{
			map[string]interface{}{"nodeId": float64(exit1), "protocol": "tls", "strategy": "round"},
		},
	}

	if _, err := h.prepareTunnelCreateState(tx, req, 3, 0); err == nil || !strings.Contains(err.Error(), "仅允许单入口") {
		t.Fatalf("expected single-entry topology error, got %v", err)
	}
}

func TestPrepareTunnelCreateStateRemoteAutoPortDefersToFederation(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	h := &Handler{repo: r}
	now := time.Now().UnixMilli()

	insertNode := func(name string, status int, portRange string, isRemote int) int64 {
		if execErr := r.DB().Exec(`
			INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx, is_remote, remote_url, remote_token, remote_config)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, name, name+"-secret", "10.0.0.1", "10.0.0.1", "", portRange, "", "v1", 1, 1, 1, now, now, status, "[::]", "[::]", 0, isRemote, "http://peer", "peer-token", `{"shareId":1}`).Error; execErr != nil {
			t.Fatalf("insert node %s: %v", name, execErr)
		}
		return mustLastInsertID(t, r, name)
	}

	entryID := insertNode("entry", 1, "31000-31010", 0)
	remoteOutID := insertNode("remote-out", 1, "30000", 1)

	if err := r.DB().Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(?, ?, ?)`, 1, remoteOutID, 30000).Error; err != nil {
		t.Fatalf("insert forward_port: %v", err)
	}

	tx := r.DB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	req := map[string]interface{}{
		"name": "test-tunnel",
		"inNodeId": []interface{}{
			map[string]interface{}{"nodeId": float64(entryID), "protocol": "tls", "strategy": "round"},
		},
		"outNodeId": []interface{}{
			map[string]interface{}{"nodeId": float64(remoteOutID), "protocol": "tls", "strategy": "round", "port": float64(0)},
		},
		"chainNodes": []interface{}{},
	}

	state, err := h.prepareTunnelCreateState(tx, req, 2, 0)
	if err != nil {
		t.Fatalf("prepare state should not fail for remote auto-port: %v", err)
	}
	if len(state.OutNodes) != 1 {
		t.Fatalf("expected 1 out node, got %d", len(state.OutNodes))
	}
	if state.OutNodes[0].Port != 0 {
		t.Fatalf("expected remote out port to remain 0 before federation reserve, got %d", state.OutNodes[0].Port)
	}
}

func TestPrepareTunnelCreateStateAllowsOfflineRemoteMiddleNode(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	h := &Handler{repo: r}
	now := time.Now().UnixMilli()

	insertNode := func(name string, status int, portRange string, isRemote int) int64 {
		if execErr := r.DB().Exec(`
			INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx, is_remote, remote_url, remote_token, remote_config)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, name, name+"-secret", "10.0.0.1", "10.0.0.1", "", portRange, "", "v1", 1, 1, 1, now, now, status, "[::]", "[::]", 0, isRemote, "http://peer", "peer-token", `{"shareId":2}`).Error; execErr != nil {
			t.Fatalf("insert node %s: %v", name, execErr)
		}
		return mustLastInsertID(t, r, name)
	}

	entryID := insertNode("entry-local", 1, "32000-32010", 0)
	remoteMiddleID := insertNode("middle-remote", 0, "33000-33010", 1)
	outID := insertNode("out-local", 1, "34000-34010", 0)

	tx := r.DB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	req := map[string]interface{}{
		"name": "remote-middle-offline-status",
		"inNodeId": []interface{}{
			map[string]interface{}{"nodeId": float64(entryID), "protocol": "tls", "strategy": "round"},
		},
		"chainNodes": []interface{}{
			[]interface{}{
				map[string]interface{}{"nodeId": float64(remoteMiddleID), "protocol": "tls", "strategy": "round", "port": float64(0)},
			},
		},
		"outNodeId": []interface{}{
			map[string]interface{}{"nodeId": float64(outID), "protocol": "tls", "strategy": "round", "port": float64(0)},
		},
	}

	state, err := h.prepareTunnelCreateState(tx, req, 2, 0)
	if err != nil {
		t.Fatalf("prepare state should allow offline remote middle node: %v", err)
	}
	if len(state.ChainHops) != 1 || len(state.ChainHops[0]) != 1 {
		t.Fatalf("expected one middle hop node, got %+v", state.ChainHops)
	}
	if state.ChainHops[0][0].NodeID != remoteMiddleID {
		t.Fatalf("expected remote middle node id %d, got %d", remoteMiddleID, state.ChainHops[0][0].NodeID)
	}
	if state.Nodes[remoteMiddleID] == nil || state.Nodes[remoteMiddleID].IsRemote != 1 {
		t.Fatalf("expected remote middle node metadata in state")
	}
}

func TestFederationRuntimeReservePortRejectsWhenShareFlowExceeded(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	h := &Handler{repo: r}
	now := time.Now().UnixMilli()

	if err := r.CreatePeerShare(&repo.PeerShare{
		Name:           "limited-share",
		NodeID:         1,
		Token:          "limited-token",
		MaxBandwidth:   2048,
		CurrentFlow:    2048,
		PortRangeStart: 30000,
		PortRangeEnd:   30010,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	}); err != nil {
		t.Fatalf("create share: %v", err)
	}

	body, err := json.Marshal(map[string]interface{}{
		"resourceKey":   "tunnel:1:node:1:type:3:hop:0",
		"protocol":      "tls",
		"requestedPort": 0,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/runtime/reserve-port", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer limited-token")
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	h.federationRuntimeReservePort(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload response.R
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != 403 {
		t.Fatalf("expected response code 403, got %d (%s)", payload.Code, payload.Msg)
	}
	if payload.Msg != "Share traffic limit exceeded" {
		t.Fatalf("unexpected response message: %q", payload.Msg)
	}
}
