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

const contractBytesPerGB int64 = 1024 * 1024 * 1024

func TestForwardResumeBlockedWhenUserFlowExceeded(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	userID := int64(2)
	tunnelID := int64(1)
	forwardID := int64(1)

	flowGB := int64(120)
	used := flowGB*contractBytesPerGB + 1

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(?, 'flow_user', 'pwd', 1, 2727251700000, ?, ?, 0, 1, 99999, ?, ?, 1)
	`, userID, flowGB, used, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, 'flow_tunnel', 1.0, 2, 'tls', 99999, ?, ?, 1, NULL, 0)
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
		VALUES(?, ?, 'flow_user', 'flow_forward', ?, '8.8.8.8:53', 'fifo', 0, 0, ?, ?, 0, 0)
	`, forwardID, userID, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert forward: %v", err)
	}

	token, err := auth.GenerateToken(userID, "flow_user", 1, secret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/resume", bytes.NewBufferString(`{"id":1}`))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code == 0 {
		t.Fatalf("expected non-zero code when flow exceeded")
	}
	if !strings.Contains(out.Msg, "流量") {
		t.Fatalf("expected flow exceeded message, got %q", out.Msg)
	}

	status := mustQueryInt(t, repo, `SELECT status FROM forward WHERE id = ?`, forwardID)
	if status != 0 {
		t.Fatalf("expected forward status to remain 0, got %d", status)
	}
}

func TestForwardResumeBlockedWhenUserTunnelFlowExceeded(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	userID := int64(2)
	tunnelID := int64(1)
	forwardID := int64(1)

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(?, 'ut_flow_user', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, userID, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, 'ut_flow_tunnel', 1.0, 2, 'tls', 99999, ?, ?, 1, NULL, 0)
	`, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}

	utFlowGB := int64(120)
	utUsed := utFlowGB * contractBytesPerGB
	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(10, ?, ?, NULL, 99999, ?, ?, 0, 1, 2727251700000, 1)
	`, userID, tunnelID, utFlowGB, utUsed).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}
	if err := repo.DB().Exec(`
		INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(?, ?, 'ut_flow_user', 'ut_flow_forward', ?, '8.8.8.8:53', 'fifo', 0, 0, ?, ?, 0, 0)
	`, forwardID, userID, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert forward: %v", err)
	}

	token, err := auth.GenerateToken(userID, "ut_flow_user", 1, secret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/resume", bytes.NewBufferString(`{"id":1}`))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code == 0 {
		t.Fatalf("expected non-zero code when tunnel flow exceeded")
	}
	if !strings.Contains(out.Msg, "隧道") || !strings.Contains(out.Msg, "流量") {
		t.Fatalf("expected tunnel flow exceeded message, got %q", out.Msg)
	}
}

func TestForwardCreateBlockedWhenFlowExceeded(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	userID := int64(2)
	tunnelID := int64(1)

	flowGB := int64(120)
	used := flowGB*contractBytesPerGB + 1

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(?, 'create_flow_user', 'pwd', 1, 2727251700000, ?, ?, 0, 1, 99999, ?, ?, 1)
	`, userID, flowGB, used, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, 'create_flow_tunnel', 1.0, 2, 'tls', 99999, ?, ?, 1, NULL, 0)
	`, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(10, ?, ?, NULL, 99999, 99999, 0, 0, 1, 2727251700000, 1)
	`, userID, tunnelID).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}

	token, err := auth.GenerateToken(userID, "create_flow_user", 1, secret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	payload := `{"tunnelId":1,"name":"n","remoteAddr":"1.1.1.1:53"}`
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
		t.Fatalf("expected non-zero code when flow exceeded")
	}
	if !strings.Contains(out.Msg, "流量") {
		t.Fatalf("expected flow exceeded message, got %q", out.Msg)
	}
}
