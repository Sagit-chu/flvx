package contract_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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
	`, "contract-tunnel", 2.5, 1, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
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
		ratioFloat, ok := item["tunnelTrafficRatio"].(float64)
		if !ok {
			t.Fatalf("expected tunnelTrafficRatio to be float64, got %T", item["tunnelTrafficRatio"])
		}
		if ratioFloat != 2.5 {
			t.Fatalf("expected tunnelTrafficRatio 2.5, got %v", ratioFloat)
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

func TestUserTunnelSaveIgnoresDeletedSpeedLimitContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(101, 'user_tunnel_speed_user_a', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user a: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "user-tunnel-missing-speed-tunnel", 1.0, 1, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, repo, "user-tunnel-missing-speed-tunnel")

	if err := repo.DB().Exec(`
		INSERT INTO speed_limit(name, speed, tunnel_id, tunnel_name, created_time, updated_time, status)
		VALUES(?, ?, NULL, NULL, ?, NULL, ?)
	`, "user-tunnel-missing-speed-limit", 2048, now, 1).Error; err != nil {
		t.Fatalf("insert speed limit: %v", err)
	}
	speedID := mustLastInsertID(t, repo, "user-tunnel-missing-speed-limit")

	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(31, 101, ?, ?, 999, 99999, 0, 0, 1, 2727251700000, 1)
	`, tunnelID, speedID).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}

	if err := repo.DB().Exec(`DELETE FROM speed_limit WHERE id = ?`, speedID).Error; err != nil {
		t.Fatalf("delete speed limit: %v", err)
	}

	t.Run("user tunnel update auto clears missing speed", func(t *testing.T) {
		updatePayload := map[string]interface{}{
			"id":            31,
			"flow":          99999,
			"num":           999,
			"expTime":       int64(2727251700000),
			"flowResetTime": 1,
			"status":        1,
			"speedId":       speedID,
		}
		updateBody, err := json.Marshal(updatePayload)
		if err != nil {
			t.Fatalf("marshal update payload: %v", err)
		}
		updateReq := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/user/update", bytes.NewReader(updateBody))
		updateReq.Header.Set("Authorization", adminToken)
		updateReq.Header.Set("Content-Type", "application/json")
		updateRes := httptest.NewRecorder()
		router.ServeHTTP(updateRes, updateReq)
		assertCode(t, updateRes, 0)

		var updatedSpeed sql.NullInt64
		if err := repo.DB().Raw(`SELECT speed_id FROM user_tunnel WHERE id = 31`).Row().Scan(&updatedSpeed); err != nil {
			t.Fatalf("query updated user_tunnel speed_id: %v", err)
		}
		if updatedSpeed.Valid {
			t.Fatalf("expected updated user_tunnel speed_id to be NULL, got %d", updatedSpeed.Int64)
		}
	})

	t.Run("user tunnel batch assign auto clears missing speed", func(t *testing.T) {
		if err := repo.DB().Exec(`UPDATE user_tunnel SET speed_id = ? WHERE id = 31`, speedID).Error; err != nil {
			t.Fatalf("prepare user_tunnel speed_id for batch assign: %v", err)
		}

		assignPayload := map[string]interface{}{
			"userId": 101,
			"tunnels": []map[string]interface{}{{
				"tunnelId": tunnelID,
				"speedId":  speedID,
			}},
		}
		assignBody, err := json.Marshal(assignPayload)
		if err != nil {
			t.Fatalf("marshal assign payload: %v", err)
		}
		assignReq := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/user/batch-assign", bytes.NewReader(assignBody))
		assignReq.Header.Set("Authorization", adminToken)
		assignReq.Header.Set("Content-Type", "application/json")
		assignRes := httptest.NewRecorder()
		router.ServeHTTP(assignRes, assignReq)
		assertCode(t, assignRes, 0)

		var assignedSpeed sql.NullInt64
		if err := repo.DB().Raw(`SELECT speed_id FROM user_tunnel WHERE id = 31`).Row().Scan(&assignedSpeed); err != nil {
			t.Fatalf("query assigned user_tunnel speed_id: %v", err)
		}
		if assignedSpeed.Valid {
			t.Fatalf("expected assigned user_tunnel speed_id to be NULL, got %d", assignedSpeed.Int64)
		}
	})
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

	updateToBPayload := map[string]interface{}{
		"id":      forwardID,
		"speedId": speedIDB,
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
}

func TestForwardUpdateIgnoresDeletedSpeedLimitContract(t *testing.T) {
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
	`, "forward-update-missing-speed-tunnel", 1.0, 1, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, repo, "forward-update-missing-speed-tunnel")

	if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "forward-update-missing-speed-node", "forward-update-missing-speed-secret", "10.32.0.1", "10.32.0.1", "", "42000-42010", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	nodeID := mustLastInsertID(t, repo, "forward-update-missing-speed-node")

	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 42001, 'round', 1, 'tls')
	`, tunnelID, nodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO speed_limit(name, speed, tunnel_id, tunnel_name, created_time, updated_time, status)
		VALUES(?, ?, NULL, NULL, ?, NULL, ?)
	`, "forward-update-missing-speed-limit", 2048, now, 1).Error; err != nil {
		t.Fatalf("insert speed limit: %v", err)
	}
	speedID := mustLastInsertID(t, repo, "forward-update-missing-speed-limit")

	server := httptest.NewServer(router)
	defer server.Close()
	stopNode := startMockNodeSession(t, server.URL, "forward-update-missing-speed-secret")
	defer stopNode()

	createPayload := map[string]interface{}{
		"name":       "forward-update-missing-speed-target",
		"tunnelId":   tunnelID,
		"remoteAddr": "1.1.1.1:443",
		"strategy":   "fifo",
		"speedId":    speedID,
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

	forwardID := mustLastInsertID(t, repo, "forward-update-missing-speed-target")

	if err := repo.DB().Exec(`DELETE FROM speed_limit WHERE id = ?`, speedID).Error; err != nil {
		t.Fatalf("delete speed limit: %v", err)
	}

	updatePayload := map[string]interface{}{
		"id":         forwardID,
		"name":       "forward-update-missing-speed-target-updated",
		"tunnelId":   tunnelID,
		"remoteAddr": "1.1.1.1:443",
		"strategy":   "fifo",
		"speedId":    speedID,
	}
	updateBody, err := json.Marshal(updatePayload)
	if err != nil {
		t.Fatalf("marshal update payload: %v", err)
	}
	updateReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/update", bytes.NewReader(updateBody))
	updateReq.Header.Set("Authorization", adminToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRes := httptest.NewRecorder()
	router.ServeHTTP(updateRes, updateReq)
	assertCode(t, updateRes, 0)

	storedSpeed := repo.DB().Raw(`SELECT speed_id FROM forward WHERE id = ?`, forwardID).Row()
	var updatedSpeed sql.NullInt64
	if err := storedSpeed.Scan(&updatedSpeed); err != nil {
		t.Fatalf("query updated forward speed_id: %v", err)
	}
	if updatedSpeed.Valid {
		t.Fatalf("expected updated speed_id to be NULL after missing speed limit, got %d", updatedSpeed.Int64)
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

func TestForwardUpdateRecoversFromAddressInUseContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	server := httptest.NewServer(router)
	defer server.Close()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()
	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(202, 'forward_bind_retry_user', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "forward-bind-retry-tunnel", 1.0, 1, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, repo, "forward-bind-retry-tunnel")

	if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "forward-bind-retry-node", "forward-bind-retry-secret", "10.42.0.1", "10.42.0.1", "", "44000-44010", "", "v1", 1, 1, 1, now, now, 1, "10.42.0.9", "[::]", 0).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	nodeID := mustLastInsertID(t, repo, "forward-bind-retry-node")

	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 44001, 'round', 1, 'tls')
	`, tunnelID, nodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(41, 202, ?, NULL, 999, 99999, 0, 0, 1, 2727251700000, 1)
	`, tunnelID).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}

	createPayload := map[string]interface{}{
		"name":       "forward-bind-retry-target",
		"tunnelId":   tunnelID,
		"remoteAddr": "1.1.1.1:443",
		"strategy":   "fifo",
	}
	createBody, err := json.Marshal(createPayload)
	if err != nil {
		t.Fatalf("marshal create payload: %v", err)
	}

	var mu sync.Mutex
	counts := map[string]int{}
	var addServiceAddrs []string
	triggerConflict := false
	stopNode := startMockNodeSessionWithCommandRecorder(t, server.URL, "forward-bind-retry-secret", func(cmdType string, data json.RawMessage) (bool, string) {
		key := strings.ToLower(strings.TrimSpace(cmdType))
		mu.Lock()
		counts[key]++
		attempt := counts[key]
		if strings.EqualFold(strings.TrimSpace(cmdType), "AddService") || strings.EqualFold(strings.TrimSpace(cmdType), "UpdateService") {
			var services []map[string]interface{}
			if err := json.Unmarshal(data, &services); err == nil {
				for _, svc := range services {
					if addr, _ := svc["addr"].(string); strings.TrimSpace(addr) != "" {
						addServiceAddrs = append(addServiceAddrs, addr)
					}
				}
			}
		}
		shouldFail := false
		if triggerConflict {
			if strings.EqualFold(strings.TrimSpace(cmdType), "UpdateService") && attempt == 1 {
				shouldFail = true
			}
			if strings.EqualFold(strings.TrimSpace(cmdType), "AddService") && attempt == 1 {
				shouldFail = true
			}
		}
		mu.Unlock()
		if shouldFail {
			return true, "create service 57_7_7_tcp failed: listen tcp4 0.0.0.0:46222: bind: address alreadyin use"
		}
		return false, ""
	})
	defer stopNode()
	waitNodeStatus(t, repo, nodeID, 1)

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	router.ServeHTTP(createRes, createReq)
	assertCode(t, createRes, 0)
	mu.Lock()
	counts = map[string]int{}
	addServiceAddrs = nil
	triggerConflict = true
	mu.Unlock()

	forwardID := mustLastInsertID(t, repo, "forward-bind-retry-target")

	updatePayload := map[string]interface{}{
		"id":         forwardID,
		"name":       "forward-bind-retry-target-updated",
		"tunnelId":   tunnelID,
		"remoteAddr": "9.9.9.9:8443",
		"strategy":   "fifo",
	}
	updateBody, err := json.Marshal(updatePayload)
	if err != nil {
		t.Fatalf("marshal update payload: %v", err)
	}
	updateReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/update", bytes.NewReader(updateBody))
	updateReq.Header.Set("Authorization", adminToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRes := httptest.NewRecorder()
	router.ServeHTTP(updateRes, updateReq)
	assertCode(t, updateRes, 0)

	mu.Lock()
	defer mu.Unlock()
	boundPort := mustQueryInt(t, repo, `SELECT port FROM forward_port WHERE forward_id = ? LIMIT 1`, forwardID)
	if counts["updateservice"] != 1 {
		t.Fatalf("expected one UpdateService attempt, got %d (%v)", counts["updateservice"], counts)
	}
	if counts["deleteservice"] == 0 {
		t.Fatalf("expected DeleteService cleanup after address-in-use (%v)", counts)
	}
	if counts["addservice"] < 2 {
		t.Fatalf("expected AddService retry path to run at least twice total, got %d (%v)", counts["addservice"], counts)
	}
	foundBindAddr := false
	for _, addr := range addServiceAddrs {
		if addr == "10.42.0.9:"+strconv.Itoa(boundPort) {
			foundBindAddr = true
			break
		}
	}
	if !foundBindAddr {
		t.Fatalf("expected forward runtime to keep node listen addr 10.42.0.9:%d, got %v", boundPort, addServiceAddrs)
	}

	storedRemoteAddr := mustQueryString(t, repo, `SELECT remote_addr FROM forward WHERE id = ?`, forwardID)
	if storedRemoteAddr != "9.9.9.9:8443" {
		t.Fatalf("expected remote_addr update to persist, got %q", storedRemoteAddr)
	}
}

func jsonNumber(v int64) string {
	return strconv.FormatInt(v, 10)
}

func TestNonAdminCannotSetSpeedIdOrPort(t *testing.T) {
	secret := "contract-jwt-secret-perm"
	router, repo := setupContractRouter(t, secret)
	server := httptest.NewServer(router)
	defer server.Close()
	now := time.Now().UnixMilli()

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(2, 'normal_user_perm', '3c85cdebade1c51cf64ca9f3c09d182d', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "perm-tunnel", 1.0, 2, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, repo, "perm-tunnel")

	if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "perm-node", "perm-secret", "10.0.0.20", "10.0.0.20", "", "30000-30010", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	entryNodeID := mustLastInsertID(t, repo, "perm-node")

	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 30001, 'round', 1, 'tls')
	`, tunnelID, entryNodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(?, ?, NULL, 10, 99999, 0, 0, 1, 2727251700000, 1)
	`, 2, tunnelID).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO speed_limit(name, speed, tunnel_id, tunnel_name, created_time, updated_time, status)
		VALUES(?, ?, ?, ?, ?, ?, 1)
	`, "perm-speed-limit", 2048, tunnelID, "perm-tunnel", now, now).Error; err != nil {
		t.Fatalf("insert speed limit: %v", err)
	}
	speedID := mustLastInsertID(t, repo, "perm-speed-limit")

	userToken, err := auth.GenerateToken(2, "normal_user_perm", 1, secret)
	if err != nil {
		t.Fatalf("generate user token: %v", err)
	}

	stopNode := startMockNodeSession(t, server.URL, "perm-secret")
	defer stopNode()

	t.Run("non-admin cannot set speedId on create", func(t *testing.T) {
		createPayload := map[string]interface{}{
			"name":       "perm-forward-speed",
			"tunnelId":   tunnelID,
			"remoteAddr": "1.2.3.4:443",
			"strategy":   "fifo",
			"speedId":    speedID,
		}
		createBody, err := json.Marshal(createPayload)
		if err != nil {
			t.Fatalf("marshal create payload: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
		req.Header.Set("Authorization", userToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCodeMsg(t, res, -1, "普通用户无法设置限速规则")
	})

	t.Run("non-admin cannot set inPort out of range on create", func(t *testing.T) {
		createPayload := map[string]interface{}{
			"name":       "perm-forward-port-out",
			"tunnelId":   tunnelID,
			"remoteAddr": "1.2.3.4:443",
			"strategy":   "fifo",
			"inPort":     12345,
		}
		createBody, err := json.Marshal(createPayload)
		if err != nil {
			t.Fatalf("marshal create payload: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
		req.Header.Set("Authorization", userToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code >= 0 {
			t.Errorf("expected port out of range error, got code=%d msg=%s", out.Code, out.Msg)
		}
	})

	t.Run("non-admin can set inPort within range on create", func(t *testing.T) {
		createPayload := map[string]interface{}{
			"name":       "perm-forward-port-in",
			"tunnelId":   tunnelID,
			"remoteAddr": "1.2.3.4:443",
			"strategy":   "fifo",
			"inPort":     30005,
		}
		createBody, err := json.Marshal(createPayload)
		if err != nil {
			t.Fatalf("marshal create payload: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
		req.Header.Set("Authorization", userToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCode(t, res, 0)
	})

	t.Run("non-admin can create without speedId and inPort", func(t *testing.T) {
		createPayload := map[string]interface{}{
			"name":       "perm-forward-ok",
			"tunnelId":   tunnelID,
			"remoteAddr": "1.2.3.4:443",
			"strategy":   "fifo",
		}
		createBody, err := json.Marshal(createPayload)
		if err != nil {
			t.Fatalf("marshal create payload: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
		req.Header.Set("Authorization", userToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCode(t, res, 0)
	})

	forwardID := mustLastInsertID(t, repo, "perm-forward-ok")

	t.Run("non-admin cannot update speedId", func(t *testing.T) {
		updatePayload := map[string]interface{}{
			"id":         forwardID,
			"name":       "perm-forward-updated",
			"tunnelId":   tunnelID,
			"remoteAddr": "5.6.7.8:443",
			"speedId":    speedID,
		}
		updateBody, err := json.Marshal(updatePayload)
		if err != nil {
			t.Fatalf("marshal update payload: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/update", bytes.NewReader(updateBody))
		req.Header.Set("Authorization", userToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCodeMsg(t, res, -1, "普通用户无法修改限速规则")
	})

	t.Run("non-admin cannot update inPort out of range", func(t *testing.T) {
		updatePayload := map[string]interface{}{
			"id":         forwardID,
			"name":       "perm-forward-updated2",
			"tunnelId":   tunnelID,
			"remoteAddr": "5.6.7.8:443",
			"inPort":     54321,
		}
		updateBody, err := json.Marshal(updatePayload)
		if err != nil {
			t.Fatalf("marshal update payload: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/update", bytes.NewReader(updateBody))
		req.Header.Set("Authorization", userToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code >= 0 {
			t.Errorf("expected port out of range error, got code=%d msg=%s", out.Code, out.Msg)
		}
	})

	t.Run("non-admin can update inPort within range", func(t *testing.T) {
		updatePayload := map[string]interface{}{
			"id":         forwardID,
			"name":       "perm-forward-updated3",
			"tunnelId":   tunnelID,
			"remoteAddr": "5.6.7.8:443",
			"inPort":     30006,
		}
		updateBody, err := json.Marshal(updatePayload)
		if err != nil {
			t.Fatalf("marshal update payload: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/update", bytes.NewReader(updateBody))
		req.Header.Set("Authorization", userToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCode(t, res, 0)
	})

	t.Run("non-admin can update without speedId and inPort", func(t *testing.T) {
		updatePayload := map[string]interface{}{
			"id":         forwardID,
			"name":       "perm-forward-updated-ok",
			"tunnelId":   tunnelID,
			"remoteAddr": "9.10.11.12:443",
		}
		updateBody, err := json.Marshal(updatePayload)
		if err != nil {
			t.Fatalf("marshal update payload: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/update", bytes.NewReader(updateBody))
		req.Header.Set("Authorization", userToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCode(t, res, 0)
	})

	t.Run("non-admin can update when request keeps existing speedId", func(t *testing.T) {
		if err := repo.DB().Exec(`UPDATE forward SET speed_id = ? WHERE id = ?`, speedID, forwardID).Error; err != nil {
			t.Fatalf("assign forward speed limit: %v", err)
		}

		updatePayload := map[string]interface{}{
			"id":         forwardID,
			"name":       "perm-forward-keep-speed",
			"tunnelId":   tunnelID,
			"remoteAddr": "9.10.11.12:443",
			"speedId":    speedID,
		}
		updateBody, err := json.Marshal(updatePayload)
		if err != nil {
			t.Fatalf("marshal update payload: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/update", bytes.NewReader(updateBody))
		req.Header.Set("Authorization", userToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCode(t, res, 0)
	})

	t.Run("non-admin can create with speedId null and inPort 0", func(t *testing.T) {
		createPayload := map[string]interface{}{
			"name":       "perm-forward-null-values",
			"tunnelId":   tunnelID,
			"remoteAddr": "1.2.3.4:443",
			"strategy":   "fifo",
			"speedId":    nil,
			"inPort":     0,
		}
		createBody, err := json.Marshal(createPayload)
		if err != nil {
			t.Fatalf("marshal create payload: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
		req.Header.Set("Authorization", userToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCode(t, res, 0)
	})

	t.Run("non-admin can update with speedId null", func(t *testing.T) {
		updatePayload := map[string]interface{}{
			"id":         forwardID,
			"name":       "perm-forward-null-speed",
			"tunnelId":   tunnelID,
			"remoteAddr": "9.10.11.12:443",
			"speedId":    nil,
		}
		updateBody, err := json.Marshal(updatePayload)
		if err != nil {
			t.Fatalf("marshal update payload: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/update", bytes.NewReader(updateBody))
		req.Header.Set("Authorization", userToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCode(t, res, 0)
	})
}
