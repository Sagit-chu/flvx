package contract_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-backend/internal/auth"
	"go-backend/internal/http/response"
)

func TestIssue313_EntryPortCrossTunnelConflictContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	insertNode := func(name, ip, portRange string) int64 {
		if err := repo.DB().Exec(`
			INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, name, name+"-secret", ip, ip, "", portRange, "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0).Error; err != nil {
			t.Fatalf("insert node %s: %v", name, err)
		}
		return mustLastInsertID(t, repo, name)
	}

	entryA := insertNode("issue313-entry-a", "10.100.0.1", "2000-2010")
	entryB1 := insertNode("issue313-entry-b1", "10.100.0.2", "2000-2010")
	entryB2 := insertNode("issue313-entry-b2", "10.100.0.3", "2000-2010")
	chainA := insertNode("issue313-chain-a", "10.100.0.4", "3000-3010")
	chainB := insertNode("issue313-chain-b", "10.100.0.5", "3000-3010")
	exitA := insertNode("issue313-exit-a", "10.100.0.6", "4000-4010")
	exitB := insertNode("issue313-exit-b", "10.100.0.7", "4000-4010")

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "issue313-tunnel-a", 1.0, 2, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel a: %v", err)
	}
	tunnelAID := mustLastInsertID(t, repo, "issue313-tunnel-a")

	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 2000, 'round', 1, 'tls')
	`, tunnelAID, entryA).Error; err != nil {
		t.Fatalf("insert chain_tunnel entry a: %v", err)
	}
	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 2, ?, 3000, 'round', 1, 'tls')
	`, tunnelAID, chainA).Error; err != nil {
		t.Fatalf("insert chain_tunnel chain a: %v", err)
	}
	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 3, ?, 4000, 'round', 1, 'tls')
	`, tunnelAID, exitA).Error; err != nil {
		t.Fatalf("insert chain_tunnel exit a: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "issue313-tunnel-b", 1.0, 2, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel b: %v", err)
	}
	tunnelBID := mustLastInsertID(t, repo, "issue313-tunnel-b")

	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 2000, 'round', 1, 'tls')
	`, tunnelBID, entryB1).Error; err != nil {
		t.Fatalf("insert chain_tunnel entry b1: %v", err)
	}
	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 2, ?, 3000, 'round', 1, 'tls')
	`, tunnelBID, chainB).Error; err != nil {
		t.Fatalf("insert chain_tunnel chain b: %v", err)
	}
	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 3, ?, 4000, 'round', 1, 'tls')
	`, tunnelBID, exitB).Error; err != nil {
		t.Fatalf("insert chain_tunnel exit b: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(3131, 1, ?, NULL, 999, 99999, 0, 0, 1, 2727251700000, 1)
	`, tunnelAID).Error; err != nil {
		t.Fatalf("insert user_tunnel for tunnel a: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO forward(user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(1, 'admin_user', 'issue313-forward-a', ?, '1.1.1.1:443', 'fifo', 0, 0, ?, ?, 1, 0)
	`, tunnelAID, now, now).Error; err != nil {
		t.Fatalf("insert forward a: %v", err)
	}
	forwardAID := mustLastInsertID(t, repo, "issue313-forward-a")

	if err := repo.DB().Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(?, ?, ?)`, forwardAID, entryA, 2000).Error; err != nil {
		t.Fatalf("insert forward_port a: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(3132, 1, ?, NULL, 999, 99999, 0, 0, 1, 2727251700000, 1)
	`, tunnelBID).Error; err != nil {
		t.Fatalf("insert user_tunnel for tunnel b: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO forward(user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(1, 'admin_user', 'issue313-forward-b', ?, '2.2.2.2:443', 'fifo', 0, 0, ?, ?, 1, 0)
	`, tunnelBID, now, now).Error; err != nil {
		t.Fatalf("insert forward b: %v", err)
	}
	forwardBID := mustLastInsertID(t, repo, "issue313-forward-b")

	if err := repo.DB().Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(?, ?, ?)`, forwardBID, entryB1, 2000).Error; err != nil {
		t.Fatalf("insert forward_port b: %v", err)
	}

	payload := map[string]interface{}{
		"id":           tunnelBID,
		"name":         "issue313-tunnel-b",
		"type":         2,
		"flow":         99999,
		"trafficRatio": 1.0,
		"status":       1,
		"inNodeId": []map[string]interface{}{
			{"nodeId": entryB1, "protocol": "tls", "strategy": "round"},
			{"nodeId": entryB2, "protocol": "tls", "strategy": "round"},
		},
		"chainNodes": []interface{}{
			[]map[string]interface{}{{"nodeId": chainB, "protocol": "tls", "strategy": "round"}},
		},
		"outNodeId": []map[string]interface{}{
			{"nodeId": exitB, "protocol": "tls", "strategy": "round"},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/update", bytes.NewReader(body))
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if out.Code == 0 {
		t.Fatalf("expected update failure due to cross-tunnel port conflict, got success with code 0")
	}

	if !bytes.Contains(res.Body.Bytes(), []byte("端口")) && !bytes.Contains(res.Body.Bytes(), []byte("占用")) {
		t.Fatalf("expected port conflict error message, got %q", out.Msg)
	}

	countB2 := mustQueryInt(t, repo, `SELECT COUNT(1) FROM forward_port WHERE forward_id = ? AND node_id = ?`, forwardBID, entryB2)
	if countB2 > 0 {
		t.Fatalf("expected no forward_port record for entryB2, but found %d", countB2)
	}

	chainCountB2 := mustQueryInt(t, repo, `SELECT COUNT(1) FROM chain_tunnel WHERE tunnel_id = ? AND node_id = ?`, tunnelBID, entryB2)
	if chainCountB2 > 0 {
		t.Fatalf("expected no chain_tunnel record for entryB2, but found %d", chainCountB2)
	}
}
