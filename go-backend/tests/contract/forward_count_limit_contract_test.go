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
	"go-backend/internal/http/response"
)

func TestForwardCreateBlockedWhenUserNumLimitExceeded(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	userID := int64(100)
	tunnelID := int64(1)

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(?, 'num_limit_user', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 2, ?, ?, 1)
	`, userID, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, 'num_limit_tunnel', 1.0, 1, 'tls', 99999, ?, ?, 1, NULL, 0)
	`, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(10, ?, ?, NULL, 99999, 99999, 0, 0, 1, 2727251700000, 1)
	`, userID, tunnelID).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(1, ?, 'num_limit_user', 'existing_forward_1', ?, '8.8.8.8:53', 'fifo', 0, 0, ?, ?, 1, 0)
	`, userID, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert existing forward 1: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(2, ?, 'num_limit_user', 'existing_forward_2', ?, '8.8.4.4:53', 'fifo', 0, 0, ?, ?, 1, 0)
	`, userID, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert existing forward 2: %v", err)
	}

	token, err := auth.GenerateToken(userID, "num_limit_user", 1, secret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	payload := `{"tunnelId":1,"name":"new_forward","remoteAddr":"1.1.1.1:53"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code == 0 {
		t.Fatalf("expected non-zero code when num limit exceeded, got code=%d msg=%q", out.Code, out.Msg)
	}
	if !strings.Contains(out.Msg, "转发数量已达上限") {
		t.Fatalf("expected forward count limit message, got %q", out.Msg)
	}
}

func TestForwardResumeBlockedWhenUserNumLimitExceeded(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	userID := int64(101)
	tunnelID := int64(1)
	pausedForwardID := int64(3)

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(?, 'num_resume_user', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 2, ?, ?, 1)
	`, userID, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, 'num_resume_tunnel', 1.0, 1, 'tls', 99999, ?, ?, 1, NULL, 0)
	`, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(10, ?, ?, NULL, 99999, 99999, 0, 0, 1, 2727251700000, 1)
	`, userID, tunnelID).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(1, ?, 'num_resume_user', 'existing_forward_1', ?, '8.8.8.8:53', 'fifo', 0, 0, ?, ?, 1, 0)
	`, userID, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert existing forward 1: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(2, ?, 'num_resume_user', 'existing_forward_2', ?, '8.8.4.4:53', 'fifo', 0, 0, ?, ?, 1, 0)
	`, userID, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert existing forward 2: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(?, ?, 'num_resume_user', 'paused_forward', ?, '1.1.1.1:53', 'fifo', 0, 0, ?, ?, 0, 0)
	`, pausedForwardID, userID, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert paused forward: %v", err)
	}

	token, err := auth.GenerateToken(userID, "num_resume_user", 1, secret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/resume", bytes.NewBufferString(`{"id":3}`))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code == 0 {
		t.Fatalf("expected non-zero code when num limit exceeded, got code=%d msg=%q", out.Code, out.Msg)
	}
	if !strings.Contains(out.Msg, "转发数量已达上限") {
		t.Fatalf("expected forward count limit message, got %q", out.Msg)
	}

	status := mustQueryInt(t, repo, `SELECT status FROM forward WHERE id = ?`, pausedForwardID)
	if status != 0 {
		t.Fatalf("expected forward status to remain 0, got %d", status)
	}
}

func TestForwardCreateBlockedWhenUserTunnelNumLimitExceeded(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	userID := int64(102)
	tunnelID := int64(1)

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(?, 'ut_num_limit_user', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, userID, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, 'ut_num_limit_tunnel', 1.0, 1, 'tls', 99999, ?, ?, 1, NULL, 0)
	`, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(10, ?, ?, NULL, 1, 99999, 0, 0, 1, 2727251700000, 1)
	`, userID, tunnelID).Error; err != nil {
		t.Fatalf("insert user_tunnel with num=1: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(1, ?, 'ut_num_limit_user', 'existing_tunnel_forward', ?, '8.8.8.8:53', 'fifo', 0, 0, ?, ?, 1, 0)
	`, userID, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert existing forward: %v", err)
	}

	token, err := auth.GenerateToken(userID, "ut_num_limit_user", 1, secret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	payload := `{"tunnelId":1,"name":"new_tunnel_forward","remoteAddr":"1.1.1.1:53"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code == 0 {
		t.Fatalf("expected non-zero code when user_tunnel num limit exceeded, got code=%d msg=%q", out.Code, out.Msg)
	}
	if !strings.Contains(out.Msg, "隧道转发数量已达上限") {
		t.Fatalf("expected tunnel forward count limit message, got %q", out.Msg)
	}
}

func TestForwardCreateAllowedWhenBelowUserNumLimit(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	userID := int64(103)
	tunnelID := int64(1)

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(?, 'num_ok_user', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 3, ?, ?, 1)
	`, userID, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, 'num_ok_tunnel', 1.0, 1, 'tls', 99999, ?, ?, 1, NULL, 0)
	`, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES('num-ok-entry', 'num-ok-secret', '10.50.0.1', '10.50.0.1', '', '10000-10010', '', 'v1', 1, 1, 1, ?, ?, 1, '[::]', '[::]', 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert entry node: %v", err)
	}
	entryNodeID := mustLastInsertID(t, repo, "num-ok-entry")

	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 10001, 'round', 1, 'tls')
	`, tunnelID, entryNodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(10, ?, ?, NULL, 99999, 99999, 0, 0, 1, 2727251700000, 1)
	`, userID, tunnelID).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(1, ?, 'num_ok_user', 'existing_forward_1', ?, '8.8.8.8:53', 'fifo', 0, 0, ?, ?, 1, 0)
	`, userID, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert existing forward 1: %v", err)
	}

	token, err := auth.GenerateToken(userID, "num_ok_user", 1, secret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	payload := `{"tunnelId":1,"name":"new_forward_ok","remoteAddr":"1.1.1.1:53"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected success (code=0) when below num limit, got code=%d msg=%q", out.Code, out.Msg)
	}
}

func TestForwardCreateAllowedWhenNumZero(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	userID := int64(104)
	tunnelID := int64(1)

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(?, 'num_zero_user', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 0, ?, ?, 1)
	`, userID, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, 'num_zero_tunnel', 1.0, 1, 'tls', 99999, ?, ?, 1, NULL, 0)
	`, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES('num-zero-entry', 'num-zero-secret', '10.60.0.1', '10.60.0.1', '', '11000-11010', '', 'v1', 1, 1, 1, ?, ?, 1, '[::]', '[::]', 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert entry node: %v", err)
	}
	entryNodeID := mustLastInsertID(t, repo, "num-zero-entry")

	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 11001, 'round', 1, 'tls')
	`, tunnelID, entryNodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(10, ?, ?, NULL, 0, 99999, 0, 0, 1, 2727251700000, 1)
	`, userID, tunnelID).Error; err != nil {
		t.Fatalf("insert user_tunnel with num=0: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(1, ?, 'num_zero_user', 'existing_forward_1', ?, '8.8.8.8:53', 'fifo', 0, 0, ?, ?, 1, 0)
	`, userID, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert existing forward 1: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(2, ?, 'num_zero_user', 'existing_forward_2', ?, '8.8.4.4:53', 'fifo', 0, 0, ?, ?, 1, 0)
	`, userID, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert existing forward 2: %v", err)
	}

	token, err := auth.GenerateToken(userID, "num_zero_user", 1, secret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	payload := `{"tunnelId":1,"name":"new_forward_zero","remoteAddr":"1.1.1.1:53"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected success (code=0) when num=0 (unlimited), got code=%d msg=%q", out.Code, out.Msg)
	}
}
