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

func TestForwardCreateBlockedWhenTunnelQuotaExceeded(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(2, 'quota_user', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := repo.DB().Exec(`
		INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(1, 'quota_tunnel', 1.0, 1, 'tls', 1, ?, ?, 0, NULL, 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(10, 2, 1, NULL, 99999, 99999, 0, 0, 1, 2727251700000, 1)
	`).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}
	if err := repo.DB().Exec(`
		INSERT INTO tunnel_quota(tunnel_id, daily_limit_gb, monthly_limit_gb, daily_used_bytes, monthly_used_bytes, day_key, month_key, disabled_by_quota, disabled_at, paused_forward_ids, created_time, updated_time)
		VALUES(1, 10, 0, ?, ?, 20260311, 202603, 1, ?, '', ?, ?)
	`, 11*contractBytesPerGB, 11*contractBytesPerGB, now, now, now).Error; err != nil {
		t.Fatalf("insert tunnel_quota: %v", err)
	}

	token, err := auth.GenerateToken(2, "quota_user", 1, secret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewBufferString(`{"tunnelId":1,"name":"quota-forward","remoteAddr":"1.1.1.1:53"}`))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code == 0 {
		t.Fatalf("expected non-zero code when tunnel quota exceeded")
	}
	if !strings.Contains(out.Msg, "配额") {
		t.Fatalf("expected quota error, got %q", out.Msg)
	}
}

func TestForwardResumeBlockedWhenTunnelQuotaExceeded(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(2, 'quota_resume_user', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := repo.DB().Exec(`
		INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(1, 'quota_resume_tunnel', 1.0, 1, 'tls', 1, ?, ?, 0, NULL, 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(10, 2, 1, NULL, 99999, 99999, 0, 0, 1, 2727251700000, 1)
	`).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}
	if err := repo.DB().Exec(`
		INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(1, 2, 'quota_resume_user', 'quota_resume_forward', 1, '1.1.1.1:53', 'fifo', 0, 0, ?, ?, 0, 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert forward: %v", err)
	}
	if err := repo.DB().Exec(`
		INSERT INTO tunnel_quota(tunnel_id, daily_limit_gb, monthly_limit_gb, daily_used_bytes, monthly_used_bytes, day_key, month_key, disabled_by_quota, disabled_at, paused_forward_ids, created_time, updated_time)
		VALUES(1, 10, 0, ?, ?, 20260311, 202603, 1, ?, '1', ?, ?)
	`, 11*contractBytesPerGB, 11*contractBytesPerGB, now, now, now).Error; err != nil {
		t.Fatalf("insert tunnel_quota: %v", err)
	}

	token, err := auth.GenerateToken(2, "quota_resume_user", 1, secret)
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
		t.Fatalf("expected non-zero code when tunnel quota exceeded")
	}
	if !strings.Contains(out.Msg, "配额") {
		t.Fatalf("expected quota error, got %q", out.Msg)
	}
	status := mustQueryInt(t, repo, `SELECT status FROM forward WHERE id = 1`)
	if status != 0 {
		t.Fatalf("expected forward to remain paused, got %d", status)
	}
}

func TestTunnelQuotaResetReEnablesTunnel(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(1, 'quota_reset_tunnel', 1.0, 1, 'tls', 1, ?, ?, 0, NULL, 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	if err := repo.DB().Exec(`
		INSERT INTO tunnel_quota(tunnel_id, daily_limit_gb, monthly_limit_gb, daily_used_bytes, monthly_used_bytes, day_key, month_key, disabled_by_quota, disabled_at, paused_forward_ids, created_time, updated_time)
		VALUES(1, 10, 0, ?, ?, 20260311, 202603, 1, ?, '', ?, ?)
	`, 11*contractBytesPerGB, 11*contractBytesPerGB, now, now, now).Error; err != nil {
		t.Fatalf("insert tunnel_quota: %v", err)
	}

	token, err := auth.GenerateToken(1, "admin", 0, secret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/quota/reset", bytes.NewBufferString(`{"tunnelId":1,"scope":"all"}`))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected reset success, got code=%d msg=%q", out.Code, out.Msg)
	}
	tunnelStatus := mustQueryInt(t, repo, `SELECT status FROM tunnel WHERE id = 1`)
	if tunnelStatus != 1 {
		t.Fatalf("expected tunnel to be re-enabled, got %d", tunnelStatus)
	}
	quotaDisabled := mustQueryInt(t, repo, `SELECT disabled_by_quota FROM tunnel_quota WHERE tunnel_id = 1`)
	if quotaDisabled != 0 {
		t.Fatalf("expected quota disable flag cleared, got %d", quotaDisabled)
	}
}
