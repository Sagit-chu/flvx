package contract_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"go-backend/internal/auth"
	"go-backend/internal/http/response"
)

func TestForwardOwnershipAndScopeContracts(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(2, 'normal_user', '3c85cdebade1c51cf64ca9f3c09d182d', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "contract-tunnel", 1.0, 1, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, repo, "contract-tunnel")

	if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "entry-node", "entry-secret", "10.0.0.10", "10.0.0.10", "", "20000-20010", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	entryNodeID := mustLastInsertID(t, repo, "entry-node")

	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 20001, 'round', 1, 'tls')
	`, tunnelID, entryNodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO forward(user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(?, ?, ?, ?, ?, ?, 0, 0, ?, ?, 1, ?)
	`, 1, "admin_user", "admin-forward", tunnelID, "1.1.1.1:443", "fifo", now, now, 0).Error; err != nil {
		t.Fatalf("insert admin forward: %v", err)
	}
	adminForwardID := mustLastInsertID(t, repo, "admin-forward")

	if err := repo.DB().Exec(`
		INSERT INTO forward(user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(?, ?, ?, ?, ?, ?, 0, 0, ?, ?, 1, ?)
	`, 2, "normal_user", "user-forward", tunnelID, "8.8.8.8:53", "fifo", now, now, 1).Error; err != nil {
		t.Fatalf("insert user forward: %v", err)
	}
	userForwardID := mustLastInsertID(t, repo, "user-forward")

	userToken, err := auth.GenerateToken(2, "normal_user", 1, secret)
	if err != nil {
		t.Fatalf("generate user token: %v", err)
	}
	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	t.Run("non-owner cannot delete another user's forward", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/delete", bytes.NewBufferString(`{"id":`+jsonNumber(adminForwardID)+`}`))
		req.Header.Set("Authorization", userToken)
		res := httptest.NewRecorder()

		router.ServeHTTP(res, req)

		assertCodeMsg(t, res, -1, "转发不存在")
	})

	t.Run("non-admin forward list is scoped to owner", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/list", bytes.NewBufferString(`{}`))
		req.Header.Set("Authorization", userToken)
		res := httptest.NewRecorder()

		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d (%s)", out.Code, out.Msg)
		}
		arr, ok := out.Data.([]interface{})
		if !ok {
			t.Fatalf("expected array data, got %T", out.Data)
		}
		if len(arr) != 1 {
			t.Fatalf("expected 1 forward, got %d", len(arr))
		}
		item, ok := arr[0].(map[string]interface{})
		if !ok {
			t.Fatalf("expected object item, got %T", arr[0])
		}
		idFloat, ok := item["id"].(float64)
		if !ok {
			t.Fatalf("expected id to be float64, got %T", item["id"])
		}
		if got := int64(idFloat); got != userForwardID {
			t.Fatalf("expected forward id %d, got %d", userForwardID, got)
		}
	})

	t.Run("forward diagnose returns structured payload", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/diagnose", bytes.NewBufferString(`{"forwardId":`+jsonNumber(userForwardID)+`}`))
		req.Header.Set("Authorization", userToken)
		res := httptest.NewRecorder()

		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d (%s)", out.Code, out.Msg)
		}

		payload, ok := out.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("expected object payload, got %T", out.Data)
		}
		results, ok := payload["results"].([]interface{})
		if !ok || len(results) == 0 {
			t.Fatalf("expected non-empty results, got %v", payload["results"])
		}
		first, ok := results[0].(map[string]interface{})
		if !ok {
			t.Fatalf("expected result object, got %T", results[0])
		}
		if _, ok := first["message"]; !ok {
			t.Fatalf("expected message field in diagnosis result")
		}
		fromChainTypeFloat, ok := first["fromChainType"].(float64)
		if !ok {
			t.Fatalf("expected fromChainType to be float64, got %T", first["fromChainType"])
		}
		if got := int(fromChainTypeFloat); got != 1 {
			t.Fatalf("expected fromChainType=1, got %d", got)
		}
	})

	t.Run("tunnel diagnose returns structured payload", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/diagnose", bytes.NewBufferString(`{"tunnelId":`+jsonNumber(tunnelID)+`}`))
		req.Header.Set("Authorization", adminToken)
		res := httptest.NewRecorder()

		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d (%s)", out.Code, out.Msg)
		}

		payload, ok := out.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("expected object payload, got %T", out.Data)
		}
		results, ok := payload["results"].([]interface{})
		if !ok || len(results) == 0 {
			t.Fatalf("expected non-empty results, got %v", payload["results"])
		}
		first, ok := results[0].(map[string]interface{})
		if !ok {
			t.Fatalf("expected result object, got %T", results[0])
		}
		if _, ok := first["message"]; !ok {
			t.Fatalf("expected message field in tunnel diagnosis result")
		}
	})
}

func TestForwardSwitchTunnelRollbackOnSyncFailure(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(2, 'switch_user', '3c85cdebade1c51cf64ca9f3c09d182d', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	insertTunnel := func(name string, inx int) int64 {
		if err := repo.DB().Exec(`
			INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, name, 1.0, 1, "tls", 99999, now, now, 1, nil, inx).Error; err != nil {
			t.Fatalf("insert tunnel %s: %v", name, err)
		}
		return mustLastInsertID(t, repo, name)
	}

	insertNode := func(name, ip, portRange string, inx int) int64 {
		if err := repo.DB().Exec(`
			INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, name, name+"-secret", ip, ip, "", portRange, "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", inx).Error; err != nil {
			t.Fatalf("insert node %s: %v", name, err)
		}
		return mustLastInsertID(t, repo, name)
	}

	tunnelA := insertTunnel("switch-tunnel-a", 0)
	tunnelB := insertTunnel("switch-tunnel-b", 1)
	nodeA := insertNode("switch-node-a", "10.10.0.1", "21000-21010", 0)
	nodeB := insertNode("switch-node-b", "10.10.0.2", "22000-22010", 1)

	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 21001, 'round', 1, 'tls')
	`, tunnelA, nodeA).Error; err != nil {
		t.Fatalf("insert chain_tunnel tunnelA: %v", err)
	}
	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 22001, 'round', 1, 'tls')
	`, tunnelB, nodeB).Error; err != nil {
		t.Fatalf("insert chain_tunnel tunnelB: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(10, 2, ?, NULL, 999, 99999, 0, 0, 1, 2727251700000, 1)
	`, tunnelA).Error; err != nil {
		t.Fatalf("insert user_tunnel A: %v", err)
	}
	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(11, 2, ?, NULL, 999, 99999, 0, 0, 1, 2727251700000, 1)
	`, tunnelB).Error; err != nil {
		t.Fatalf("insert user_tunnel B: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO forward(user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(2, 'switch_user', 'switch-forward', ?, '8.8.8.8:53', 'fifo', 0, 0, ?, ?, 1, 0)
	`, tunnelA, now, now).Error; err != nil {
		t.Fatalf("insert forward: %v", err)
	}
	forwardID := mustLastInsertID(t, repo, "switch-forward")

	if err := repo.DB().Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(?, ?, ?)`, forwardID, nodeA, 21001).Error; err != nil {
		t.Fatalf("insert forward_port: %v", err)
	}

	payload := `{"id":` + jsonNumber(forwardID) + `,"tunnelId":` + jsonNumber(tunnelB) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/update", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code == 0 {
		t.Fatalf("expected update failure when node is offline")
	}

	tunnelAfter := mustQueryInt64(t, repo, `SELECT tunnel_id FROM forward WHERE id = ?`, forwardID)
	if tunnelAfter != tunnelA {
		t.Fatalf("expected tunnel rollback to %d, got %d", tunnelA, tunnelAfter)
	}

	nodeAfter, portAfter := mustQueryInt64Int(t, repo, `SELECT node_id, port FROM forward_port WHERE forward_id = ? LIMIT 1`, forwardID)
	if nodeAfter != nodeA || portAfter != 21001 {
		t.Fatalf("expected forward_port rollback to node=%d port=21001, got node=%d port=%d", nodeA, nodeAfter, portAfter)
	}
}

func TestForwardBatchChangeTunnelRollbackOnSyncFailure(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(2, 'batch_switch_user', '3c85cdebade1c51cf64ca9f3c09d182d', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES('batch-switch-tunnel-a', 1.0, 1, 'tls', 99999, ?, ?, 1, NULL, 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert tunnel A: %v", err)
	}
	tunnelA := mustLastInsertID(t, repo, "batch-switch-tunnel-a")

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES('batch-switch-tunnel-b', 1.0, 1, 'tls', 99999, ?, ?, 1, NULL, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert tunnel B: %v", err)
	}
	tunnelB := mustLastInsertID(t, repo, "batch-switch-tunnel-b")

	if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES('batch-switch-node-a', 'batch-switch-node-a-secret', '10.11.0.1', '10.11.0.1', '', '23000-23010', '', 'v1', 1, 1, 1, ?, ?, 1, '[::]', '[::]', 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert node A: %v", err)
	}
	nodeA := mustLastInsertID(t, repo, "batch-switch-node-a")

	if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES('batch-switch-node-b', 'batch-switch-node-b-secret', '10.11.0.2', '10.11.0.2', '', '24000-24010', '', 'v1', 1, 1, 1, ?, ?, 1, '[::]', '[::]', 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert node B: %v", err)
	}
	nodeB := mustLastInsertID(t, repo, "batch-switch-node-b")

	if err := repo.DB().Exec(`INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol) VALUES(?, 1, ?, 23001, 'round', 1, 'tls')`, tunnelA, nodeA).Error; err != nil {
		t.Fatalf("insert chain_tunnel A: %v", err)
	}
	if err := repo.DB().Exec(`INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol) VALUES(?, 1, ?, 24001, 'round', 1, 'tls')`, tunnelB, nodeB).Error; err != nil {
		t.Fatalf("insert chain_tunnel B: %v", err)
	}

	if err := repo.DB().Exec(`INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status) VALUES(20, 2, ?, NULL, 999, 99999, 0, 0, 1, 2727251700000, 1)`, tunnelA).Error; err != nil {
		t.Fatalf("insert user_tunnel A: %v", err)
	}
	if err := repo.DB().Exec(`INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status) VALUES(21, 2, ?, NULL, 999, 99999, 0, 0, 1, 2727251700000, 1)`, tunnelB).Error; err != nil {
		t.Fatalf("insert user_tunnel B: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO forward(user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(2, 'batch_switch_user', 'batch-switch-forward', ?, '1.1.1.1:443', 'fifo', 0, 0, ?, ?, 1, 0)
	`, tunnelA, now, now).Error; err != nil {
		t.Fatalf("insert forward: %v", err)
	}
	forwardID := mustLastInsertID(t, repo, "batch-switch-forward")

	if err := repo.DB().Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(?, ?, ?)`, forwardID, nodeA, 23001).Error; err != nil {
		t.Fatalf("insert forward_port: %v", err)
	}

	payload := `{"forwardIds":[` + jsonNumber(forwardID) + `],"targetTunnelId":` + jsonNumber(tunnelB) + `}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/batch-change-tunnel", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected API success envelope, got code=%d msg=%q", out.Code, out.Msg)
	}

	result, ok := out.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", out.Data)
	}
	if int(result["failCount"].(float64)) != 1 {
		t.Fatalf("expected failCount=1, got %v", result["failCount"])
	}

	tunnelAfter := mustQueryInt64(t, repo, `SELECT tunnel_id FROM forward WHERE id = ?`, forwardID)
	if tunnelAfter != tunnelA {
		t.Fatalf("expected tunnel rollback to %d, got %d", tunnelA, tunnelAfter)
	}

	nodeAfter, portAfter := mustQueryInt64Int(t, repo, `SELECT node_id, port FROM forward_port WHERE forward_id = ? LIMIT 1`, forwardID)
	if nodeAfter != nodeA || portAfter != 23001 {
		t.Fatalf("expected forward_port rollback to node=%d port=23001, got node=%d port=%d", nodeA, nodeAfter, portAfter)
	}
}

func TestUserTunnelReassignmentKeepsStableID(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(100, 'stable_user', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES('stable-tunnel', 1.0, 1, 'tls', 99999, ?, ?, 1, NULL, 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, repo, "stable-tunnel")

	// 1. Assign permission (creates new user_tunnel)
	// userTunnelBatchAssign expects structure: {userId: 123, tunnels: [{tunnelId: 456, ...}]}
	assignPayload := `{"userId":100,"tunnels":[{"tunnelId":` + jsonNumber(tunnelID) + `}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/user/batch-assign", bytes.NewBufferString(assignPayload))
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected code 0, got %d msg=%q", out.Code, out.Msg)
	}

	initialID := mustQueryInt64(t, repo, `SELECT id FROM user_tunnel WHERE user_id = 100 AND tunnel_id = ?`, tunnelID)

	// 2. Re-assign permission (should UPDATE, not INSERT)
	reassignPayload := `{"userId":100,"tunnels":[{"tunnelId":` + jsonNumber(tunnelID) + `}]}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/user/batch-assign", bytes.NewBufferString(reassignPayload))
	req2.Header.Set("Authorization", adminToken)
	req2.Header.Set("Content-Type", "application/json")
	res2 := httptest.NewRecorder()
	router.ServeHTTP(res2, req2)

	var out2 response.R
	if err := json.NewDecoder(res2.Body).Decode(&out2); err != nil {
		t.Fatalf("decode response 2: %v", err)
	}
	if out2.Code != 0 {
		t.Fatalf("expected code 0, got %d msg=%q", out2.Code, out2.Msg)
	}

	// 3. Verify stable ID and no duplicates
	count := mustQueryInt(t, repo, `SELECT COUNT(1) FROM user_tunnel WHERE user_id = 100 AND tunnel_id = ?`, tunnelID)
	if count != 1 {
		t.Fatalf("expected exactly 1 user_tunnel record, got %d", count)
	}

	currentID := mustQueryInt64(t, repo, `SELECT id FROM user_tunnel WHERE user_id = 100 AND tunnel_id = ?`, tunnelID)

	if currentID != initialID {
		t.Fatalf("user_tunnel ID changed from %d to %d (unstable ID!)", initialID, currentID)
	}
}

func TestForwardSpeedIDWriteAndClearContracts(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(2, 'speed_user', '3c85cdebade1c51cf64ca9f3c09d182d', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "forward-speed-tunnel", 1.0, 1, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, repo, "forward-speed-tunnel")

	if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "forward-speed-node", "forward-speed-secret", "10.30.0.1", "10.30.0.1", "", "31000-31010", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	nodeID := mustLastInsertID(t, repo, "forward-speed-node")

	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 31001, 'round', 1, 'tls')
	`, tunnelID, nodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO speed_limit(name, speed, tunnel_id, tunnel_name, created_time, updated_time, status)
		VALUES(?, ?, NULL, NULL, ?, NULL, ?)
	`, "forward-speed-limit-a", 2048, now, 1).Error; err != nil {
		t.Fatalf("insert speed limit a: %v", err)
	}
	speedIDA := mustLastInsertID(t, repo, "forward-speed-limit-a")

	if err := repo.DB().Exec(`
		INSERT INTO speed_limit(name, speed, tunnel_id, tunnel_name, created_time, updated_time, status)
		VALUES(?, ?, NULL, NULL, ?, NULL, ?)
	`, "forward-speed-limit-b", 4096, now, 1).Error; err != nil {
		t.Fatalf("insert speed limit b: %v", err)
	}
	speedIDB := mustLastInsertID(t, repo, "forward-speed-limit-b")

	server := httptest.NewServer(router)
	defer server.Close()
	stopNode := startMockNodeSession(t, server.URL, "forward-speed-secret")
	defer stopNode()

	createPayload := map[string]interface{}{
		"name":       "forward-speed-target",
		"tunnelId":   tunnelID,
		"remoteAddr": "1.1.1.1:443",
		"strategy":   "fifo",
		"speedId":    speedIDA,
		"udpMode":    "normal",
	}
	createBody, err := json.Marshal(createPayload)
	if err != nil {
		t.Fatalf("marshal create payload: %v", err)
	}
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	router.ServeHTTP(createRes, createReq)
	assertCode(t, createRes, 0)

	forwardID := mustLastInsertID(t, repo, "forward-speed-target")
	storedSpeed := repo.DB().Raw(`SELECT speed_id FROM forward WHERE id = ?`, forwardID).Row()
	var createdSpeed sql.NullInt64
	if err := storedSpeed.Scan(&createdSpeed); err != nil {
		t.Fatalf("query created forward speed_id: %v", err)
	}
	if !createdSpeed.Valid || createdSpeed.Int64 != speedIDA {
		t.Fatalf("expected created speed_id=%d, got valid=%v value=%d", speedIDA, createdSpeed.Valid, createdSpeed.Int64)
	}
	createdUDPMode := mustQueryString(t, repo, `SELECT udp_mode FROM forward WHERE id = ?`, forwardID)
	if createdUDPMode != "normal" {
		t.Fatalf("expected created udp_mode=normal, got %s", createdUDPMode)
	}

	updateToBPayload := map[string]interface{}{
		"id":      forwardID,
		"speedId": speedIDB,
		"udpMode": "transparent",
	}
	updateToBBody, err := json.Marshal(updateToBPayload)
	if err != nil {
		t.Fatalf("marshal update-to-b payload: %v", err)
	}
	updateToBReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/update", bytes.NewReader(updateToBBody))
	updateToBReq.Header.Set("Authorization", adminToken)
	updateToBReq.Header.Set("Content-Type", "application/json")
	updateToBRes := httptest.NewRecorder()
	router.ServeHTTP(updateToBRes, updateToBReq)
	assertCode(t, updateToBRes, 0)

	storedSpeed = repo.DB().Raw(`SELECT speed_id FROM forward WHERE id = ?`, forwardID).Row()
	var updatedSpeed sql.NullInt64
	if err := storedSpeed.Scan(&updatedSpeed); err != nil {
		t.Fatalf("query updated forward speed_id: %v", err)
	}
	if !updatedSpeed.Valid || updatedSpeed.Int64 != speedIDB {
		t.Fatalf("expected updated speed_id=%d, got valid=%v value=%d", speedIDB, updatedSpeed.Valid, updatedSpeed.Int64)
	}
	updatedUDPMode := mustQueryString(t, repo, `SELECT udp_mode FROM forward WHERE id = ?`, forwardID)
	if updatedUDPMode != "transparent" {
		t.Fatalf("expected updated udp_mode=transparent, got %s", updatedUDPMode)
	}

	clearPayload := map[string]interface{}{
		"id":      forwardID,
		"speedId": nil,
	}
	clearBody, err := json.Marshal(clearPayload)
	if err != nil {
		t.Fatalf("marshal clear payload: %v", err)
	}
	clearReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/update", bytes.NewReader(clearBody))
	clearReq.Header.Set("Authorization", adminToken)
	clearReq.Header.Set("Content-Type", "application/json")
	clearRes := httptest.NewRecorder()
	router.ServeHTTP(clearRes, clearReq)
	assertCode(t, clearRes, 0)

	storedSpeed = repo.DB().Raw(`SELECT speed_id FROM forward WHERE id = ?`, forwardID).Row()
	var clearedSpeed sql.NullInt64
	if err := storedSpeed.Scan(&clearedSpeed); err != nil {
		t.Fatalf("query cleared forward speed_id: %v", err)
	}
	if clearedSpeed.Valid {
		t.Fatalf("expected cleared speed_id to be NULL, got %d", clearedSpeed.Int64)
	}

	listReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/list", bytes.NewBufferString(`{}`))
	listReq.Header.Set("Authorization", adminToken)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)

	var out response.R
	if err := json.NewDecoder(listRes.Body).Decode(&out); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected list code 0, got %d (%s)", out.Code, out.Msg)
	}
	arr, ok := out.Data.([]interface{})
	if !ok {
		t.Fatalf("expected list data array, got %T", out.Data)
	}
	var matched map[string]interface{}
	for _, item := range arr {
		row, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		idFloat, ok := row["id"].(float64)
		if ok && int64(idFloat) == forwardID {
			matched = row
			break
		}
	}
	if matched == nil {
		t.Fatalf("expected forward %d in list", forwardID)
	}
	udpMode, ok := matched["udpMode"].(string)
	if !ok {
		t.Fatalf("expected udpMode in list item, got %T", matched["udpMode"])
	}
	if udpMode != "transparent" {
		t.Fatalf("expected list udpMode=transparent, got %s", udpMode)
	}
}

func TestForwardCreateThenPauseResumeContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()
	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "forward-toggle-tunnel", 1.0, 1, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, repo, "forward-toggle-tunnel")

	if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "forward-toggle-node", "forward-toggle-secret", "10.31.0.1", "10.31.0.1", "", "41000-41010", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	nodeID := mustLastInsertID(t, repo, "forward-toggle-node")

	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 41001, 'round', 1, 'tls')
	`, tunnelID, nodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	server := httptest.NewServer(router)
	defer server.Close()
	stopNode := startMockNodeSession(t, server.URL, "forward-toggle-secret")
	defer stopNode()

	createPayload := map[string]interface{}{
		"name":       "forward-toggle-target",
		"tunnelId":   tunnelID,
		"remoteAddr": "1.1.1.1:443",
		"strategy":   "fifo",
	}
	createBody, err := json.Marshal(createPayload)
	if err != nil {
		t.Fatalf("marshal create payload: %v", err)
	}
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	router.ServeHTTP(createRes, createReq)
	assertCode(t, createRes, 0)

	forwardID := mustLastInsertID(t, repo, "forward-toggle-target")

	pauseBody, err := json.Marshal(map[string]interface{}{"id": forwardID})
	if err != nil {
		t.Fatalf("marshal pause payload: %v", err)
	}
	pauseReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/pause", bytes.NewReader(pauseBody))
	pauseReq.Header.Set("Authorization", adminToken)
	pauseReq.Header.Set("Content-Type", "application/json")
	pauseRes := httptest.NewRecorder()
	router.ServeHTTP(pauseRes, pauseReq)
	assertCode(t, pauseRes, 0)

	pausedStatus := mustQueryInt(t, repo, `SELECT status FROM forward WHERE id = ?`, forwardID)
	if pausedStatus != 0 {
		t.Fatalf("expected status=0 after pause, got %d", pausedStatus)
	}

	resumeBody, err := json.Marshal(map[string]interface{}{"id": forwardID})
	if err != nil {
		t.Fatalf("marshal resume payload: %v", err)
	}
	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/resume", bytes.NewReader(resumeBody))
	resumeReq.Header.Set("Authorization", adminToken)
	resumeReq.Header.Set("Content-Type", "application/json")
	resumeRes := httptest.NewRecorder()
	router.ServeHTTP(resumeRes, resumeReq)
	assertCode(t, resumeRes, 0)

	resumedStatus := mustQueryInt(t, repo, `SELECT status FROM forward WHERE id = ?`, forwardID)
	if resumedStatus != 1 {
		t.Fatalf("expected status=1 after resume, got %d", resumedStatus)
	}
}

func TestForwardTransparentUDPControlFlowContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()
	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "forward-transparent-control-tunnel", 1.0, 1, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, repo, "forward-transparent-control-tunnel")

	if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "forward-transparent-control-node", "forward-transparent-control-secret", "10.41.0.1", "10.41.0.1", "", "51000-51010", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	nodeID := mustLastInsertID(t, repo, "forward-transparent-control-node")

	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 51001, 'round', 1, 'tls')
	`, tunnelID, nodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	server := httptest.NewServer(router)
	defer server.Close()

	var (
		mu       sync.Mutex
		commands []string
	)
	stopNode := startMockNodeSessionWithHook(t, server.URL, "forward-transparent-control-secret", func(cmdType string) {
		mu.Lock()
		commands = append(commands, cmdType)
		mu.Unlock()
	})
	defer stopNode()

	createPayload := map[string]interface{}{
		"name":       "forward-transparent-control-target",
		"tunnelId":   tunnelID,
		"remoteAddr": "1.1.1.1:443",
		"strategy":   "fifo",
		"udpMode":    "transparent",
	}
	createBody, err := json.Marshal(createPayload)
	if err != nil {
		t.Fatalf("marshal create payload: %v", err)
	}
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	router.ServeHTTP(createRes, createReq)
	assertCode(t, createRes, 0)

	forwardID := mustLastInsertID(t, repo, "forward-transparent-control-target")

	diagBody, _ := json.Marshal(map[string]interface{}{"forwardId": forwardID})
	diagReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/diagnose", bytes.NewReader(diagBody))
	diagReq.Header.Set("Authorization", adminToken)
	diagReq.Header.Set("Content-Type", "application/json")
	diagRes := httptest.NewRecorder()
	router.ServeHTTP(diagRes, diagReq)

	var diagOut response.R
	if err := json.NewDecoder(diagRes.Body).Decode(&diagOut); err != nil {
		t.Fatalf("decode diagnose response: %v", err)
	}
	if diagOut.Code != 0 {
		t.Fatalf("expected diagnose code 0, got %d (%s)", diagOut.Code, diagOut.Msg)
	}
	diagPayload, ok := diagOut.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected diagnose payload object, got %T", diagOut.Data)
	}
	tproxy, ok := diagPayload["tproxy"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tproxy payload, got %T", diagPayload["tproxy"])
	}
	if !valueAsBool(tproxy["enabled"]) {
		t.Fatalf("expected tproxy.enabled=true, got %v", tproxy["enabled"])
	}

	pauseBody, _ := json.Marshal(map[string]interface{}{"id": forwardID})
	pauseReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/pause", bytes.NewReader(pauseBody))
	pauseReq.Header.Set("Authorization", adminToken)
	pauseReq.Header.Set("Content-Type", "application/json")
	pauseRes := httptest.NewRecorder()
	router.ServeHTTP(pauseRes, pauseReq)
	assertCode(t, pauseRes, 0)

	resumeBody, _ := json.Marshal(map[string]interface{}{"id": forwardID})
	resumeReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/resume", bytes.NewReader(resumeBody))
	resumeReq.Header.Set("Authorization", adminToken)
	resumeReq.Header.Set("Content-Type", "application/json")
	resumeRes := httptest.NewRecorder()
	router.ServeHTTP(resumeRes, resumeReq)
	assertCode(t, resumeRes, 0)

	redeployBody, _ := json.Marshal(map[string]interface{}{"ids": []int64{forwardID}})
	redeployReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/batch-redeploy", bytes.NewReader(redeployBody))
	redeployReq.Header.Set("Authorization", adminToken)
	redeployReq.Header.Set("Content-Type", "application/json")
	redeployRes := httptest.NewRecorder()
	router.ServeHTTP(redeployRes, redeployReq)
	assertCode(t, redeployRes, 0)

	deleteBody, _ := json.Marshal(map[string]interface{}{"id": forwardID})
	deleteReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/delete", bytes.NewReader(deleteBody))
	deleteReq.Header.Set("Authorization", adminToken)
	deleteReq.Header.Set("Content-Type", "application/json")
	deleteRes := httptest.NewRecorder()
	router.ServeHTTP(deleteRes, deleteReq)
	assertCode(t, deleteRes, 0)

	remaining := mustQueryInt(t, repo, `SELECT COUNT(1) FROM forward WHERE id = ?`, forwardID)
	if remaining != 0 {
		t.Fatalf("expected forward deleted, got count=%d", remaining)
	}

	mu.Lock()
	seen := append([]string(nil), commands...)
	mu.Unlock()

	hasCommand := func(want string) bool {
		for _, got := range seen {
			if got == want {
				return true
			}
		}
		return false
	}

	for _, cmd := range []string{"ProbeTProxyCapability", "EnsureTProxyPolicy", "UpdateService", "PauseService", "ResumeService", "DeleteService", "DeleteTProxyPolicy"} {
		if !hasCommand(cmd) {
			t.Fatalf("expected command %s in node command stream, got %v", cmd, seen)
		}
	}
}

func jsonNumber(v int64) string {
	return strconv.FormatInt(v, 10)
}
