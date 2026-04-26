package contract_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go-backend/internal/auth"
	"go-backend/internal/http/handler"
	"go-backend/internal/http/response"
)

func TestForwardLocalRemoteAddrToggleContracts(t *testing.T) {
	handler.DisableSafeRemoteAddrCheckForTesting = false
	t.Cleanup(func() {
		handler.DisableSafeRemoteAddrCheckForTesting = true
	})

	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	server := httptest.NewServer(router)
	defer server.Close()

	now := time.Now().UnixMilli()
	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(2, 'local_remote_user', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "local-remote-tunnel", 1.0, 1, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, repo, "local-remote-tunnel")

	if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "local-remote-entry", "local-remote-secret", "10.60.0.1", "10.60.0.1", "", "31000-31010", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	entryNodeID := mustLastInsertID(t, repo, "local-remote-entry")

	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 31001, 'round', 1, 'tls')
	`, tunnelID, entryNodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(601, 2, ?, NULL, 999, 99999, 0, 0, 1, 2727251700000, 1)
	`, tunnelID).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}

	userToken, err := auth.GenerateToken(2, "local_remote_user", 1, secret)
	if err != nil {
		t.Fatalf("generate user token: %v", err)
	}

	stopNode := startMockNodeSession(t, server.URL, "local-remote-secret")
	defer stopNode()
	waitNodeStatus(t, repo, entryNodeID, 1)

	t.Run("local remote address is rejected on create when toggle is off", func(t *testing.T) {
		createPayload := map[string]interface{}{
			"name":       "deny-local-create",
			"tunnelId":   tunnelID,
			"remoteAddr": "127.0.0.1:8080",
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

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code == 0 {
			t.Fatalf("expected local remote address to be rejected when toggle is off")
		}
		if !strings.Contains(out.Msg, "internal IP") && !strings.Contains(out.Msg, "内部") {
			t.Fatalf("expected internal IP error, got code=%d msg=%q", out.Code, out.Msg)
		}
	})

	t.Run("local remote address is allowed on create when toggle is on", func(t *testing.T) {
		if err := repo.DB().Exec(`
			INSERT INTO vite_config(name, value, time)
			VALUES(?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET value = excluded.value, time = excluded.time
		`, "allow_local_remote_addr", "true", time.Now().UnixMilli()).Error; err != nil {
			t.Fatalf("enable allow_local_remote_addr: %v", err)
		}

		createPayload := map[string]interface{}{
			"name":       "allow-local-create",
			"tunnelId":   tunnelID,
			"remoteAddr": "127.0.0.1:8080",
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

	t.Run("local remote address is rejected on update when toggle is off", func(t *testing.T) {
		if err := repo.DB().Exec(`
			INSERT INTO vite_config(name, value, time)
			VALUES(?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET value = excluded.value, time = excluded.time
		`, "allow_local_remote_addr", "false", time.Now().UnixMilli()).Error; err != nil {
			t.Fatalf("disable allow_local_remote_addr: %v", err)
		}

		createPayload := map[string]interface{}{
			"name":       "safe-remote-before-update",
			"tunnelId":   tunnelID,
			"remoteAddr": "8.8.8.8:53",
			"strategy":   "fifo",
		}
		createBody, err := json.Marshal(createPayload)
		if err != nil {
			t.Fatalf("marshal safe create payload: %v", err)
		}
		createReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
		createReq.Header.Set("Authorization", userToken)
		createReq.Header.Set("Content-Type", "application/json")
		createRes := httptest.NewRecorder()
		router.ServeHTTP(createRes, createReq)
		assertCode(t, createRes, 0)

		forwardID := mustLastInsertID(t, repo, "safe-remote-before-update")
		updatePayload := map[string]interface{}{
			"id":         forwardID,
			"name":       "safe-remote-before-update",
			"tunnelId":   tunnelID,
			"remoteAddr": "127.0.0.1:8081",
			"strategy":   "fifo",
		}
		updateBody, err := json.Marshal(updatePayload)
		if err != nil {
			t.Fatalf("marshal update payload: %v", err)
		}
		updateReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/update", bytes.NewReader(updateBody))
		updateReq.Header.Set("Authorization", userToken)
		updateReq.Header.Set("Content-Type", "application/json")
		updateRes := httptest.NewRecorder()
		router.ServeHTTP(updateRes, updateReq)

		var out response.R
		if err := json.NewDecoder(updateRes.Body).Decode(&out); err != nil {
			t.Fatalf("decode update response: %v", err)
		}
		if out.Code == 0 {
			t.Fatalf("expected local remote address to be rejected on update when toggle is off")
		}
		if !strings.Contains(out.Msg, "internal IP") && !strings.Contains(out.Msg, "内部") {
			t.Fatalf("expected internal IP error on update, got code=%d msg=%q", out.Code, out.Msg)
		}
	})

	t.Run("local remote address is allowed on update when toggle is on", func(t *testing.T) {
		if err := repo.DB().Exec(`
			INSERT INTO vite_config(name, value, time)
			VALUES(?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET value = excluded.value, time = excluded.time
		`, "allow_local_remote_addr", "true", time.Now().UnixMilli()).Error; err != nil {
			t.Fatalf("enable allow_local_remote_addr: %v", err)
		}

		var forwardID int64
		if err := repo.DB().Raw(`SELECT id FROM forward WHERE name = ? ORDER BY id DESC LIMIT 1`, "safe-remote-before-update").Row().Scan(&forwardID); err != nil {
			t.Fatalf("query forward id: %v", err)
		}

		updatePayload := map[string]interface{}{
			"id":         forwardID,
			"name":       "safe-remote-before-update",
			"tunnelId":   tunnelID,
			"remoteAddr": "127.0.0.1:8081",
			"strategy":   "fifo",
		}
		updateBody, err := json.Marshal(updatePayload)
		if err != nil {
			t.Fatalf("marshal update payload: %v", err)
		}
		updateReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/update", bytes.NewReader(updateBody))
		updateReq.Header.Set("Authorization", userToken)
		updateReq.Header.Set("Content-Type", "application/json")
		updateRes := httptest.NewRecorder()
		router.ServeHTTP(updateRes, updateReq)
		assertCode(t, updateRes, 0)
	})
}
