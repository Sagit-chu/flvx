package contract_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"go-backend/internal/auth"
	"go-backend/internal/http/response"
	"go-backend/internal/security"
)

func TestForwardCreateRollbackWhenLimiterDispatchFailsContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, r := setupContractRouter(t, secret)
	server := httptest.NewServer(router)
	defer server.Close()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()
	if err := r.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "limiter-fail-tunnel", 1.0, 1, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, r, "limiter-fail-tunnel")

	if err := r.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "limiter-fail-node", "limiter-fail-secret", "10.20.0.1", "10.20.0.1", "", "32000-32010", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	nodeID := mustLastInsertID(t, r, "limiter-fail-node")

	if err := r.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 32001, 'round', 1, 'tls')
	`, tunnelID, nodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO speed_limit(name, speed, tunnel_id, tunnel_name, created_time, updated_time, status)
		VALUES(?, ?, NULL, NULL, ?, NULL, ?)
	`, "limiter-fail-rule", 1024, now, 1).Error; err != nil {
		t.Fatalf("insert speed limit: %v", err)
	}
	speedID := mustLastInsertID(t, r, "limiter-fail-rule")

	stopNode := startMockNodeSessionWithCommandFailures(t, server.URL, "limiter-fail-secret", map[string]string{
		"addlimiters": "mock add limiters failed",
	})
	defer stopNode()

	payload := map[string]interface{}{
		"name":       "limiter-fail-forward",
		"tunnelId":   tunnelID,
		"remoteAddr": "1.1.1.1:443",
		"strategy":   "fifo",
		"speedId":    speedID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(body))
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code == 0 {
		t.Fatalf("expected create failure on limiter dispatch, got code=0")
	}

	forwardCount := mustQueryInt(t, r, `SELECT COUNT(1) FROM forward WHERE name = ?`, "limiter-fail-forward")
	if forwardCount != 0 {
		t.Fatalf("expected forward rollback delete on limiter failure, got count=%d", forwardCount)
	}
}

func TestForwardCreateSucceedsWhenLimiterAlreadyExistsAndUpdateSucceedsContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, r := setupContractRouter(t, secret)
	server := httptest.NewServer(router)
	defer server.Close()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()
	if err := r.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "limiter-exists-update-ok-tunnel", 1.0, 1, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, r, "limiter-exists-update-ok-tunnel")

	if err := r.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "limiter-exists-update-ok-node", "limiter-exists-update-ok-secret", "10.20.1.1", "10.20.1.1", "", "32200-32210", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	nodeID := mustLastInsertID(t, r, "limiter-exists-update-ok-node")

	if err := r.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 32201, 'round', 1, 'tls')
	`, tunnelID, nodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO speed_limit(name, speed, tunnel_id, tunnel_name, created_time, updated_time, status)
		VALUES(?, ?, NULL, NULL, ?, NULL, ?)
	`, "limiter-exists-update-ok-rule", 1024, now, 1).Error; err != nil {
		t.Fatalf("insert speed limit: %v", err)
	}
	speedID := mustLastInsertID(t, r, "limiter-exists-update-ok-rule")

	stopNode := startMockNodeSessionWithCommandFailures(t, server.URL, "limiter-exists-update-ok-secret", map[string]string{
		"addlimiters": "limiter 8 already exists",
	})
	defer stopNode()

	payload := map[string]interface{}{
		"name":       "limiter-exists-update-ok-forward",
		"tunnelId":   tunnelID,
		"remoteAddr": "1.1.1.1:443",
		"strategy":   "fifo",
		"speedId":    speedID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(body))
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected create success when updater succeeds, got code=%d msg=%s", out.Code, out.Msg)
	}

	forwardCount := mustQueryInt(t, r, `SELECT COUNT(1) FROM forward WHERE name = ?`, "limiter-exists-update-ok-forward")
	if forwardCount != 1 {
		t.Fatalf("expected forward kept when update limiter succeeds, got count=%d", forwardCount)
	}
}

func TestForwardCreateRollbackWhenLimiterAlreadyExistsAndUpdateFailsContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, r := setupContractRouter(t, secret)
	server := httptest.NewServer(router)
	defer server.Close()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()
	if err := r.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "limiter-exists-update-fail-tunnel", 1.0, 1, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, r, "limiter-exists-update-fail-tunnel")

	if err := r.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "limiter-exists-update-fail-node", "limiter-exists-update-fail-secret", "10.20.2.1", "10.20.2.1", "", "32300-32310", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	nodeID := mustLastInsertID(t, r, "limiter-exists-update-fail-node")

	if err := r.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 32301, 'round', 1, 'tls')
	`, tunnelID, nodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO speed_limit(name, speed, tunnel_id, tunnel_name, created_time, updated_time, status)
		VALUES(?, ?, NULL, NULL, ?, NULL, ?)
	`, "limiter-exists-update-fail-rule", 1024, now, 1).Error; err != nil {
		t.Fatalf("insert speed limit: %v", err)
	}
	speedID := mustLastInsertID(t, r, "limiter-exists-update-fail-rule")

	stopNode := startMockNodeSessionWithCommandFailures(t, server.URL, "limiter-exists-update-fail-secret", map[string]string{
		"addlimiters":    "limiter 9 already exists",
		"updatelimiters": "mock update limiters failed",
	})
	defer stopNode()

	payload := map[string]interface{}{
		"name":       "limiter-exists-update-fail-forward",
		"tunnelId":   tunnelID,
		"remoteAddr": "1.1.1.1:443",
		"strategy":   "fifo",
		"speedId":    speedID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(body))
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code == 0 {
		t.Fatalf("expected create failure when update limiter fails, got code=0")
	}
	if !strings.Contains(out.Msg, "mock update limiters failed") {
		t.Fatalf("expected update failure message, got %q", out.Msg)
	}

	forwardCount := mustQueryInt(t, r, `SELECT COUNT(1) FROM forward WHERE name = ?`, "limiter-exists-update-fail-forward")
	if forwardCount != 0 {
		t.Fatalf("expected forward rollback delete when update limiter fails, got count=%d", forwardCount)
	}
}

func TestForwardCreateRollbackWhenServiceDispatchReturnsAddressInUseContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, r := setupContractRouter(t, secret)
	server := httptest.NewServer(router)
	defer server.Close()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()
	if err := r.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "addr-in-use-tunnel", 1.0, 1, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, r, "addr-in-use-tunnel")

	if err := r.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "addr-in-use-node", "addr-in-use-secret", "10.20.0.11", "10.20.0.11", "", "32100-32110", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	nodeID := mustLastInsertID(t, r, "addr-in-use-node")

	if err := r.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 32101, 'round', 1, 'tls')
	`, tunnelID, nodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	stopNode := startMockNodeSessionWithCommandFailures(t, server.URL, "addr-in-use-secret", map[string]string{
		"updateservice": "listen tcp [::]:32101: bind: address already in use",
		"addservice":    "listen tcp [::]:32101: bind: address already in use",
	})
	defer stopNode()

	payload := map[string]interface{}{
		"name":       "addr-in-use-forward",
		"tunnelId":   tunnelID,
		"remoteAddr": "1.1.1.1:443",
		"strategy":   "fifo",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(body))
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code == 0 {
		t.Fatalf("expected create failure on address-in-use service dispatch, got code=0")
	}

	forwardCount := mustQueryInt(t, r, `SELECT COUNT(1) FROM forward WHERE name = ?`, "addr-in-use-forward")
	if forwardCount != 0 {
		t.Fatalf("expected forward rollback delete on address-in-use failure, got count=%d", forwardCount)
	}
}

func TestBatchAssignRollbackWhenLimiterDispatchFailsContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, r := setupContractRouter(t, secret)
	server := httptest.NewServer(router)
	defer server.Close()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()
	if err := r.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(2, 'assign_user', '3c85cdebade1c51cf64ca9f3c09d182d', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "assign-limiter-fail-tunnel", 1.0, 1, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, r, "assign-limiter-fail-tunnel")

	if err := r.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "assign-limiter-fail-node", "assign-limiter-fail-secret", "10.21.0.1", "10.21.0.1", "", "33000-33010", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	nodeID := mustLastInsertID(t, r, "assign-limiter-fail-node")

	if err := r.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 33001, 'round', 1, 'tls')
	`, tunnelID, nodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO speed_limit(name, speed, tunnel_id, tunnel_name, created_time, updated_time, status)
		VALUES(?, ?, NULL, NULL, ?, NULL, ?)
	`, "assign-limiter-fail-rule", 2048, now, 1).Error; err != nil {
		t.Fatalf("insert speed limit: %v", err)
	}
	speedID := mustLastInsertID(t, r, "assign-limiter-fail-rule")

	if err := r.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(21, 2, ?, NULL, 999, 99999, 0, 0, 1, 2727251700000, 1)
	`, tunnelID).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO forward(user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(2, 'assign_user', 'assign-limiter-fail-forward', ?, '9.9.9.9:53', 'fifo', 0, 0, ?, ?, 1, 0)
	`, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert forward: %v", err)
	}
	forwardID := mustLastInsertID(t, r, "assign-limiter-fail-forward")

	if err := r.DB().Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(?, ?, ?)`, forwardID, nodeID, 33001).Error; err != nil {
		t.Fatalf("insert forward_port: %v", err)
	}

	stopNode := startMockNodeSessionWithCommandFailures(t, server.URL, "assign-limiter-fail-secret", map[string]string{
		"addlimiters": "mock add limiters failed",
	})
	defer stopNode()

	assignPayload := map[string]interface{}{
		"userId": 2,
		"tunnels": []map[string]interface{}{{
			"tunnelId": tunnelID,
			"speedId":  speedID,
		}},
	}
	body, err := json.Marshal(assignPayload)
	if err != nil {
		t.Fatalf("marshal assign payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/user/batch-assign", bytes.NewReader(body))
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code == 0 {
		t.Fatalf("expected assign failure on limiter dispatch, got code=0")
	}

	var persistedSpeedID sql.NullInt64
	if err := r.DB().Raw(`SELECT speed_id FROM user_tunnel WHERE user_id = 2 AND tunnel_id = ?`, tunnelID).Row().Scan(&persistedSpeedID); err != nil {
		t.Fatalf("query user_tunnel speed_id: %v", err)
	}
	if persistedSpeedID.Valid {
		t.Fatalf("expected speed_id rollback to NULL, got %d", persistedSpeedID.Int64)
	}
}

func TestBatchAssignInsertRollbackWhenLimiterDispatchFailsContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, r := setupContractRouter(t, secret)
	server := httptest.NewServer(router)
	defer server.Close()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()
	if err := r.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(3, 'assign_insert_user', '3c85cdebade1c51cf64ca9f3c09d182d', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "assign-insert-limiter-fail-tunnel", 1.0, 1, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, r, "assign-insert-limiter-fail-tunnel")

	if err := r.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "assign-insert-limiter-fail-node", "assign-insert-limiter-fail-secret", "10.22.0.1", "10.22.0.1", "", "34000-34010", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	nodeID := mustLastInsertID(t, r, "assign-insert-limiter-fail-node")

	if err := r.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 34001, 'round', 1, 'tls')
	`, tunnelID, nodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO speed_limit(name, speed, tunnel_id, tunnel_name, created_time, updated_time, status)
		VALUES(?, ?, NULL, NULL, ?, NULL, ?)
	`, "assign-insert-limiter-fail-rule", 3072, now, 1).Error; err != nil {
		t.Fatalf("insert speed limit: %v", err)
	}
	speedID := mustLastInsertID(t, r, "assign-insert-limiter-fail-rule")

	if err := r.DB().Exec(`
		INSERT INTO forward(user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(3, 'assign_insert_user', 'assign-insert-limiter-fail-forward', ?, '8.8.4.4:53', 'fifo', 0, 0, ?, ?, 1, 0)
	`, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert forward: %v", err)
	}
	forwardID := mustLastInsertID(t, r, "assign-insert-limiter-fail-forward")

	if err := r.DB().Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(?, ?, ?)`, forwardID, nodeID, 34001).Error; err != nil {
		t.Fatalf("insert forward_port: %v", err)
	}

	stopNode := startMockNodeSessionWithCommandFailures(t, server.URL, "assign-insert-limiter-fail-secret", map[string]string{
		"addlimiters": "mock add limiters failed",
	})
	defer stopNode()

	assignPayload := map[string]interface{}{
		"userId": 3,
		"tunnels": []map[string]interface{}{{
			"tunnelId": tunnelID,
			"speedId":  speedID,
		}},
	}
	body, err := json.Marshal(assignPayload)
	if err != nil {
		t.Fatalf("marshal assign payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/user/batch-assign", bytes.NewReader(body))
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code == 0 {
		t.Fatalf("expected assign(insert) failure on limiter dispatch, got code=0")
	}

	insertedCount := mustQueryInt(t, r, `SELECT COUNT(1) FROM user_tunnel WHERE user_id = 3 AND tunnel_id = ?`, tunnelID)
	if insertedCount != 0 {
		t.Fatalf("expected inserted user_tunnel rollback delete, got count=%d", insertedCount)
	}
}

func TestTunnelUpdateRecoversFromAddressInUseContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, r := setupContractRouter(t, secret)
	server := httptest.NewServer(router)
	defer server.Close()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()
	if err := r.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "tunnel-bind-retry", 1.0, 1, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, r, "tunnel-bind-retry")

	if err := r.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "tunnel-bind-entry", "tunnel-bind-entry-secret", "10.41.0.1", "10.41.0.1", "", "43000-43010", "", "v1", 1, 1, 1, now, now, 1, "10.41.0.1", "[::]", 0).Error; err != nil {
		t.Fatalf("insert entry node: %v", err)
	}
	entryNodeID := mustLastInsertID(t, r, "tunnel-bind-entry")

	if err := r.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "tunnel-bind-exit", "tunnel-bind-exit-secret", "10.41.0.2", "10.41.0.2", "", "43100-43110", "eth0", "v1", 1, 1, 1, now, now, 1, "10.41.0.9", "[::]", 0).Error; err != nil {
		t.Fatalf("insert exit node: %v", err)
	}
	exitNodeID := mustLastInsertID(t, r, "tunnel-bind-exit")

	if err := r.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 43001, 'round', 1, 'tls')
	`, tunnelID, entryNodeID).Error; err != nil {
		t.Fatalf("insert entry chain_tunnel: %v", err)
	}
	if err := r.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol, connect_ip)
		VALUES(?, 3, ?, 43101, 'round', 1, 'tls', ?)
	`, tunnelID, exitNodeID, "10.41.0.99").Error; err != nil {
		t.Fatalf("insert exit chain_tunnel: %v", err)
	}

	var commandMu sync.Mutex
	commandCounts := map[string]int{}
	var addServiceAddrs []string
	stopEntry := startMockNodeSessionWithCommandRecorder(t, server.URL, "tunnel-bind-entry-secret", func(cmdType string, data json.RawMessage) (bool, string) {
		commandMu.Lock()
		defer commandMu.Unlock()
		commandCounts["entry:"+strings.ToLower(strings.TrimSpace(cmdType))]++
		return false, ""
	})
	defer stopEntry()
	stopExit := startMockNodeSessionWithCommandRecorder(t, server.URL, "tunnel-bind-exit-secret", func(cmdType string, data json.RawMessage) (bool, string) {
		key := "exit:" + strings.ToLower(strings.TrimSpace(cmdType))
		commandMu.Lock()
		commandCounts[key]++
		attempt := commandCounts[key]
		if strings.EqualFold(strings.TrimSpace(cmdType), "AddService") {
			var services []map[string]interface{}
			if err := json.Unmarshal(data, &services); err == nil {
				for _, svc := range services {
					if addr, _ := svc["addr"].(string); strings.TrimSpace(addr) != "" {
						addServiceAddrs = append(addServiceAddrs, addr)
					}
				}
			}
		}
		commandMu.Unlock()
		if strings.EqualFold(strings.TrimSpace(cmdType), "AddService") && attempt == 1 {
			return true, "listen tcp 10.41.0.99:43101: bind: address already in use"
		}
		return false, ""
	})
	defer stopExit()
	waitNodeStatus(t, r, entryNodeID, 1)
	waitNodeStatus(t, r, exitNodeID, 1)

	payload := map[string]interface{}{
		"id":           tunnelID,
		"name":         "tunnel-bind-retry",
		"type":         2,
		"flow":         99999,
		"trafficRatio": 1.0,
		"status":       1,
		"inNodeId": []map[string]interface{}{
			{"nodeId": entryNodeID, "protocol": "tls", "strategy": "round"},
		},
		"chainNodes": []interface{}{},
		"outNodeId": []map[string]interface{}{
			{"nodeId": exitNodeID, "protocol": "tls", "strategy": "round", "port": 43101, "connectIp": "10.41.0.99"},
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
	assertCode(t, res, 0)

	commandMu.Lock()
	defer commandMu.Unlock()
	if commandCounts["exit:addservice"] != 2 {
		t.Fatalf("expected exit AddService twice, got %d (%v)", commandCounts["exit:addservice"], sortedCommandCounts(commandCounts))
	}
	if commandCounts["exit:deleteservice"] == 0 {
		t.Fatalf("expected exit DeleteService retry cleanup to run (%v)", sortedCommandCounts(commandCounts))
	}
	if len(addServiceAddrs) < 2 {
		t.Fatalf("expected recorded AddService addresses, got %v", addServiceAddrs)
	}
	for _, addr := range addServiceAddrs {
		if addr != "10.41.0.99:43101" {
			t.Fatalf("expected connectIp to stay preferred in AddService addr, got %q", addr)
		}
	}
}

func TestTunnelUpdateChangesEntryNodeButLeavesOldForwardRuntimeContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, r := setupContractRouter(t, secret)
	server := httptest.NewServer(router)
	defer server.Close()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()
	if err := r.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(2, 'issue281_user', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "issue281-tunnel", 1.0, 2, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, r, "issue281-tunnel")

	insertNode := func(name, secretValue, ip, portRange string, inx int) int64 {
		if err := r.DB().Exec(`
			INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, name, secretValue, ip, ip, "", portRange, "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", inx).Error; err != nil {
			t.Fatalf("insert node %s: %v", name, err)
		}
		return mustLastInsertID(t, r, name)
	}

	oldEntryNodeID := insertNode("issue281-old-entry", "issue281-old-entry-secret", "10.51.0.1", "51000-51010", 0)
	newEntryNodeID := insertNode("issue281-new-entry", "issue281-new-entry-secret", "10.51.0.2", "52000-52010", 1)
	exitNodeID := insertNode("issue281-exit", "issue281-exit-secret", "10.51.0.3", "53000-53010", 2)

	if err := r.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 51001, 'round', 1, 'tls')
	`, tunnelID, oldEntryNodeID).Error; err != nil {
		t.Fatalf("insert old entry chain_tunnel: %v", err)
	}
	if err := r.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 3, ?, 53001, 'round', 1, 'tls')
	`, tunnelID, exitNodeID).Error; err != nil {
		t.Fatalf("insert exit chain_tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(281, 2, ?, NULL, 999, 99999, 0, 0, 1, 2727251700000, 1)
	`, tunnelID).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO forward(user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(2, 'issue281_user', 'issue281-forward', ?, '8.8.8.8:53', 'fifo', 0, 0, ?, ?, 1, 0)
	`, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert forward: %v", err)
	}
	forwardID := mustLastInsertID(t, r, "issue281-forward")

	if err := r.DB().Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(?, ?, ?)`, forwardID, oldEntryNodeID, 51001).Error; err != nil {
		t.Fatalf("insert forward_port: %v", err)
	}

	forwardBase := fmt.Sprintf("%d_%d_%d", forwardID, 2, 281)

	var commandMu sync.Mutex
	oldEntryDeleteNames := make([]string, 0)
	newEntryUpdateNames := make([]string, 0)

	recordForwardServiceNames := func(data json.RawMessage, list *[]string) {
		var serviceList []map[string]interface{}
		if err := json.Unmarshal(data, &serviceList); err == nil {
			for _, service := range serviceList {
				name, _ := service["name"].(string)
				if strings.HasPrefix(strings.TrimSpace(name), forwardBase) {
					*list = append(*list, name)
				}
			}
			return
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(data, &payload); err != nil {
			return
		}
		if rawServices, ok := payload["services"].([]interface{}); ok {
			for _, raw := range rawServices {
				name, _ := raw.(string)
				if strings.HasPrefix(strings.TrimSpace(name), forwardBase) {
					*list = append(*list, name)
				}
			}
			return
		}
	}

	stopOldEntry := startMockNodeSessionWithCommandRecorder(t, server.URL, "issue281-old-entry-secret", func(cmdType string, data json.RawMessage) (bool, string) {
		commandMu.Lock()
		defer commandMu.Unlock()
		if strings.EqualFold(strings.TrimSpace(cmdType), "DeleteService") {
			recordForwardServiceNames(data, &oldEntryDeleteNames)
		}
		return false, ""
	})
	defer stopOldEntry()

	stopNewEntry := startMockNodeSessionWithCommandRecorder(t, server.URL, "issue281-new-entry-secret", func(cmdType string, data json.RawMessage) (bool, string) {
		commandMu.Lock()
		defer commandMu.Unlock()
		if strings.EqualFold(strings.TrimSpace(cmdType), "UpdateService") || strings.EqualFold(strings.TrimSpace(cmdType), "AddService") {
			recordForwardServiceNames(data, &newEntryUpdateNames)
		}
		return false, ""
	})
	defer stopNewEntry()

	stopExit := startMockNodeSessionWithCommandRecorder(t, server.URL, "issue281-exit-secret", func(cmdType string, data json.RawMessage) (bool, string) {
		return false, ""
	})
	defer stopExit()

	waitNodeStatus(t, r, oldEntryNodeID, 1)
	waitNodeStatus(t, r, newEntryNodeID, 1)
	waitNodeStatus(t, r, exitNodeID, 1)

	payload := map[string]interface{}{
		"id":           tunnelID,
		"name":         "issue281-tunnel",
		"type":         2,
		"flow":         99999,
		"trafficRatio": 1.0,
		"status":       1,
		"inNodeId": []map[string]interface{}{
			{"nodeId": newEntryNodeID, "protocol": "tls", "strategy": "round"},
		},
		"chainNodes": []interface{}{},
		"outNodeId": []map[string]interface{}{
			{"nodeId": exitNodeID, "protocol": "tls", "strategy": "round", "port": 53001},
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
	assertCode(t, res, 0)

	nodeAfter, portAfter := mustQueryInt64Int(t, r, `SELECT node_id, port FROM forward_port WHERE forward_id = ? LIMIT 1`, forwardID)
	if nodeAfter != newEntryNodeID || portAfter != 51001 {
		t.Fatalf("expected forward_port rebound to node=%d port=51001, got node=%d port=%d", newEntryNodeID, nodeAfter, portAfter)
	}

	commandMu.Lock()
	defer commandMu.Unlock()
	if len(newEntryUpdateNames) == 0 {
		t.Fatalf("expected new entry node to receive forward runtime sync for %s", forwardBase)
	}
	if len(oldEntryDeleteNames) == 0 {
		t.Fatalf("expected old entry node to receive forward DeleteService cleanup for %s, got none", forwardBase)
	}
}

func TestTunnelUpdateEntryTransitionsCleanupForwardRuntimeContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, r := setupContractRouter(t, secret)
	server := httptest.NewServer(router)
	defer server.Close()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()
	if err := r.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(2, 'issue281_transition_user', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "issue281-transition-tunnel", 1.0, 2, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, r, "issue281-transition-tunnel")

	insertNode := func(name, secretValue, ip, portRange string, inx int) int64 {
		if err := r.DB().Exec(`
			INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, name, secretValue, ip, ip, "", portRange, "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", inx).Error; err != nil {
			t.Fatalf("insert node %s: %v", name, err)
		}
		return mustLastInsertID(t, r, name)
	}

	entryA := insertNode("issue281-transition-entry-a", "issue281-transition-entry-a-secret", "10.52.0.1", "54000-54010", 0)
	entryB := insertNode("issue281-transition-entry-b", "issue281-transition-entry-b-secret", "10.52.0.2", "55000-55010", 1)
	entryC := insertNode("issue281-transition-entry-c", "issue281-transition-entry-c-secret", "10.52.0.3", "56000-56010", 2)
	exitNodeID := insertNode("issue281-transition-exit", "issue281-transition-exit-secret", "10.52.0.4", "57000-57010", 3)

	if err := r.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 54001, 'round', 1, 'tls')
	`, tunnelID, entryA).Error; err != nil {
		t.Fatalf("insert initial entry chain_tunnel: %v", err)
	}
	if err := r.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 3, ?, 57001, 'round', 1, 'tls')
	`, tunnelID, exitNodeID).Error; err != nil {
		t.Fatalf("insert exit chain_tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(282, 2, ?, NULL, 999, 99999, 0, 0, 1, 2727251700000, 1)
	`, tunnelID).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO forward(user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(2, 'issue281_transition_user', 'issue281-transition-forward', ?, '1.1.1.1:443', 'fifo', 0, 0, ?, ?, 1, 0)
	`, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert forward: %v", err)
	}
	forwardID := mustLastInsertID(t, r, "issue281-transition-forward")

	if err := r.DB().Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(?, ?, ?)`, forwardID, entryA, 54001).Error; err != nil {
		t.Fatalf("insert forward_port: %v", err)
	}

	forwardBase := fmt.Sprintf("%d_%d_%d", forwardID, 2, 282)
	recorder := newForwardRuntimeCommandRecorder(forwardBase)

	stopEntryA := startMockNodeSessionWithCommandRecorder(t, server.URL, "issue281-transition-entry-a-secret", recorder.handler("entry-a"))
	defer stopEntryA()
	stopEntryB := startMockNodeSessionWithCommandRecorder(t, server.URL, "issue281-transition-entry-b-secret", recorder.handler("entry-b"))
	defer stopEntryB()
	stopEntryC := startMockNodeSessionWithCommandRecorder(t, server.URL, "issue281-transition-entry-c-secret", recorder.handler("entry-c"))
	defer stopEntryC()
	stopExit := startMockNodeSessionWithCommandRecorder(t, server.URL, "issue281-transition-exit-secret", recorder.handler("exit"))
	defer stopExit()

	waitNodeStatus(t, r, entryA, 1)
	waitNodeStatus(t, r, entryB, 1)
	waitNodeStatus(t, r, entryC, 1)
	waitNodeStatus(t, r, exitNodeID, 1)

	updateTunnelEntries := func(entries []map[string]interface{}) {
		payload := map[string]interface{}{
			"id":           tunnelID,
			"name":         "issue281-transition-tunnel",
			"type":         2,
			"flow":         99999,
			"trafficRatio": 1.0,
			"status":       1,
			"inNodeId":     entries,
			"chainNodes":   []interface{}{},
			"outNodeId": []map[string]interface{}{
				{"nodeId": exitNodeID, "protocol": "tls", "strategy": "round", "port": 57001},
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
		assertCode(t, res, 0)
	}

	updateTunnelEntries([]map[string]interface{}{
		{"nodeId": entryA, "protocol": "tls", "strategy": "round"},
		{"nodeId": entryB, "protocol": "tls", "strategy": "round"},
	})

	afterMulti := mustQueryNodePorts(t, r, `SELECT node_id, port FROM forward_port WHERE forward_id = ? ORDER BY id ASC`, forwardID)
	if len(afterMulti) != 2 || afterMulti[entryA] != 54001 || afterMulti[entryB] != 54001 {
		t.Fatalf("expected forward_port on entryA+entryB with port 54001, got %v", afterMulti)
	}
	if recorder.syncCount("entry-b") == 0 {
		t.Fatalf("expected entry-b to receive forward runtime sync for %s", forwardBase)
	}
	if recorder.deleteCount("entry-a") != 0 {
		t.Fatalf("expected no cleanup on retained entry-a during single->multi transition, got %v", recorder.deleteNames("entry-a"))
	}

	updateTunnelEntries([]map[string]interface{}{
		{"nodeId": entryC, "protocol": "tls", "strategy": "round"},
	})

	afterSingle := mustQueryNodePorts(t, r, `SELECT node_id, port FROM forward_port WHERE forward_id = ? ORDER BY id ASC`, forwardID)
	if len(afterSingle) != 1 || afterSingle[entryC] != 54001 {
		t.Fatalf("expected forward_port on entryC with port 54001, got %v", afterSingle)
	}
	if recorder.deleteCount("entry-a") == 0 {
		t.Fatalf("expected cleanup on removed entry-a during multi->single transition, got %v", recorder.deleteNames("entry-a"))
	}
	if recorder.deleteCount("entry-b") == 0 {
		t.Fatalf("expected cleanup on removed entry-b during multi->single transition, got %v", recorder.deleteNames("entry-b"))
	}
	if recorder.syncCount("entry-c") == 0 {
		t.Fatalf("expected entry-c to receive forward runtime sync for %s", forwardBase)
	}
}

type forwardRuntimeCommandRecorder struct {
	prefix string

	mu        sync.Mutex
	deletes   map[string][]string
	syncNames map[string][]string
}

func newForwardRuntimeCommandRecorder(prefix string) *forwardRuntimeCommandRecorder {
	return &forwardRuntimeCommandRecorder{
		prefix:    strings.TrimSpace(prefix),
		deletes:   make(map[string][]string),
		syncNames: make(map[string][]string),
	}
}

func (r *forwardRuntimeCommandRecorder) handler(node string) func(string, json.RawMessage) (bool, string) {
	return func(cmdType string, data json.RawMessage) (bool, string) {
		names := collectForwardServiceNames(data, r.prefix)
		if len(names) == 0 {
			return false, ""
		}

		r.mu.Lock()
		defer r.mu.Unlock()

		if strings.EqualFold(strings.TrimSpace(cmdType), "DeleteService") {
			r.deletes[node] = append(r.deletes[node], names...)
		}
		if strings.EqualFold(strings.TrimSpace(cmdType), "UpdateService") || strings.EqualFold(strings.TrimSpace(cmdType), "AddService") {
			r.syncNames[node] = append(r.syncNames[node], names...)
		}
		return false, ""
	}
}

func (r *forwardRuntimeCommandRecorder) deleteCount(node string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.deletes[node])
}

func (r *forwardRuntimeCommandRecorder) syncCount(node string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.syncNames[node])
}

func (r *forwardRuntimeCommandRecorder) deleteNames(node string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.deletes[node]...)
}

func collectForwardServiceNames(data json.RawMessage, prefix string) []string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil
	}

	names := make([]string, 0)
	var serviceList []map[string]interface{}
	if err := json.Unmarshal(data, &serviceList); err == nil {
		for _, service := range serviceList {
			name, _ := service["name"].(string)
			if strings.HasPrefix(strings.TrimSpace(name), prefix) {
				names = append(names, name)
			}
		}
		return names
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	if rawServices, ok := payload["services"].([]interface{}); ok {
		for _, raw := range rawServices {
			name, _ := raw.(string)
			if strings.HasPrefix(strings.TrimSpace(name), prefix) {
				names = append(names, name)
			}
		}
	}
	return names
}

func startMockNodeSessionWithCommandFailures(t *testing.T, baseURL string, nodeSecret string, failCommands map[string]string) func() {
	t.Helper()

	u, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse provider url: %v", err)
	}
	if strings.EqualFold(u.Scheme, "https") {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/system-info"
	q := u.Query()
	q.Set("type", "1")
	q.Set("secret", nodeSecret)
	q.Set("version", "v1")
	q.Set("http", "1")
	q.Set("tls", "1")
	q.Set("socks", "1")
	u.RawQuery = q.Encode()

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial mock node websocket: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			_, raw, readErr := conn.ReadMessage()
			if readErr != nil {
				return
			}

			plain := raw
			var wrap struct {
				Encrypted bool   `json:"encrypted"`
				Data      string `json:"data"`
			}
			if err := json.Unmarshal(raw, &wrap); err == nil && wrap.Encrypted && strings.TrimSpace(wrap.Data) != "" {
				crypto, cryptoErr := security.NewAESCrypto(nodeSecret)
				if cryptoErr == nil {
					if dec, decErr := crypto.Decrypt(wrap.Data); decErr == nil {
						plain = []byte(dec)
					}
				}
			}

			var cmd struct {
				Type      string `json:"type"`
				RequestID string `json:"requestId"`
			}
			if err := json.Unmarshal(plain, &cmd); err != nil {
				continue
			}
			if strings.TrimSpace(cmd.RequestID) == "" {
				continue
			}

			cmdType := strings.TrimSpace(cmd.Type)
			failMsg, shouldFail := failCommands[strings.ToLower(cmdType)]

			respType := fmt.Sprintf("%sResponse", cmdType)
			respPayload := map[string]interface{}{
				"type":      respType,
				"success":   !shouldFail,
				"message":   "OK",
				"requestId": cmd.RequestID,
			}
			if shouldFail {
				if strings.TrimSpace(failMsg) == "" {
					failMsg = "mock command failed"
				}
				respPayload["message"] = failMsg
			}

			respBytes, err := json.Marshal(respPayload)
			if err != nil {
				continue
			}
			_ = conn.WriteMessage(websocket.TextMessage, respBytes)
		}
	}()

	var stopOnce sync.Once
	return func() {
		stopOnce.Do(func() {
			_ = conn.Close()
			wg.Wait()
		})
	}
}

func startMockNodeSessionWithCommandRecorder(t *testing.T, baseURL string, nodeSecret string, onCommand func(cmdType string, data json.RawMessage) (bool, string)) func() {
	t.Helper()

	u, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse provider url: %v", err)
	}
	if strings.EqualFold(u.Scheme, "https") {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/system-info"
	q := u.Query()
	q.Set("type", "1")
	q.Set("secret", nodeSecret)
	q.Set("version", "v1")
	q.Set("http", "1")
	q.Set("tls", "1")
	q.Set("socks", "1")
	u.RawQuery = q.Encode()

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial mock node websocket: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			_, raw, readErr := conn.ReadMessage()
			if readErr != nil {
				return
			}

			plain := raw
			var wrap struct {
				Encrypted bool   `json:"encrypted"`
				Data      string `json:"data"`
			}
			if err := json.Unmarshal(raw, &wrap); err == nil && wrap.Encrypted && strings.TrimSpace(wrap.Data) != "" {
				crypto, cryptoErr := security.NewAESCrypto(nodeSecret)
				if cryptoErr == nil {
					if dec, decErr := crypto.Decrypt(wrap.Data); decErr == nil {
						plain = []byte(dec)
					}
				}
			}

			var cmd struct {
				Type      string          `json:"type"`
				RequestID string          `json:"requestId"`
				Data      json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(plain, &cmd); err != nil {
				continue
			}
			if strings.TrimSpace(cmd.RequestID) == "" {
				continue
			}

			shouldFail := false
			failMsg := ""
			if onCommand != nil {
				shouldFail, failMsg = onCommand(strings.TrimSpace(cmd.Type), cmd.Data)
			}

			respType := fmt.Sprintf("%sResponse", cmd.Type)
			respPayload := map[string]interface{}{
				"type":      respType,
				"success":   !shouldFail,
				"message":   "OK",
				"requestId": cmd.RequestID,
			}
			if shouldFail {
				if strings.TrimSpace(failMsg) == "" {
					failMsg = "mock command failed"
				}
				respPayload["message"] = failMsg
			}

			respBytes, err := json.Marshal(respPayload)
			if err != nil {
				continue
			}
			_ = conn.WriteMessage(websocket.TextMessage, respBytes)
		}
	}()

	var stopOnce sync.Once
	return func() {
		stopOnce.Do(func() {
			_ = conn.Close()
			wg.Wait()
		})
	}
}

func sortedCommandCounts(counts map[string]int) []string {
	items := make([]string, 0, len(counts))
	for key, value := range counts {
		items = append(items, fmt.Sprintf("%s=%d", key, value))
	}
	sort.Strings(items)
	return items
}
