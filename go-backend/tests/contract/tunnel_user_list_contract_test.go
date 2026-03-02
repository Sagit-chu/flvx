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

func TestUserTunnelListIncludesTunnelTypeContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(2, 'contract_user', '3c85cdebade1c51cf64ca9f3c09d182d', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "tun-contract-tunnel", 1.0, 3, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, repo, "tun-contract-tunnel")

	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(?, ?, NULL, ?, ?, 0, 0, ?, ?, ?)
	`, 2, tunnelID, 100, 1000, 1, 2727251700000, 1).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	body, _ := json.Marshal(map[string]interface{}{"userId": 2})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/user/list", bytes.NewReader(body))
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
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

	found := false
	for _, item := range arr {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		id, ok := obj["tunnelId"].(float64)
		if !ok || int64(id) != tunnelID {
			continue
		}
		tunnelType, ok := obj["tunnelType"].(float64)
		if !ok {
			t.Fatalf("expected tunnelType field, got %T", obj["tunnelType"])
		}
		if int(tunnelType) != 3 {
			t.Fatalf("expected tunnelType=3, got %v", tunnelType)
		}
		found = true
		break
	}

	if !found {
		t.Fatalf("expected tunnelId=%d in /api/v1/tunnel/user/list response", tunnelID)
	}
}
