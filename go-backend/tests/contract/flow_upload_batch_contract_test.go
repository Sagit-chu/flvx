package contract_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-backend/internal/store/model"
)

func TestFlowUploadAggregatesRepeatedItemsAndDisablesQuotaImmediately(t *testing.T) {
	secret := "monitoring-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now()
	nowMs := now.UnixMilli()
	dayKey := int64(now.Year()*10000 + int(now.Month())*100 + now.Day())
	monthKey := int64(now.Year()*100 + int(now.Month()))
	const bytesPerGB = int64(1024 * 1024 * 1024)

	node := &model.Node{Name: "node-1", Secret: "node-secret", ServerIP: "127.0.0.1", Port: "10000-10010", TCPListenAddr: "[::]", UDPListenAddr: "[::]", CreatedTime: nowMs, Status: 1}
	if err := repo.DB().Create(node).Error; err != nil {
		t.Fatalf("seed node: %v", err)
	}
	if err := repo.DB().Exec(`INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status) VALUES(2, 'flow_user', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)`, nowMs, nowMs).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	tunnel := &model.Tunnel{Name: "tunnel-1", TrafficRatio: 1.0, Type: 1, Protocol: "tls", Flow: 1, CreatedTime: nowMs, UpdatedTime: nowMs, Status: 1}
	if err := repo.DB().Create(tunnel).Error; err != nil {
		t.Fatalf("seed tunnel: %v", err)
	}
	if err := repo.DB().Exec(`INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status) VALUES(10, 2, ?, NULL, 99999, 99999, 0, 0, 1, 2727251700000, 1)`, tunnel.ID).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}
	forward := &model.Forward{ID: 20, UserID: 2, UserName: "flow_user", Name: "forward-20", TunnelID: tunnel.ID, RemoteAddr: "1.1.1.1:80", Strategy: "fifo", CreatedTime: nowMs, UpdatedTime: nowMs, Status: 1}
	if err := repo.DB().Create(forward).Error; err != nil {
		t.Fatalf("seed forward: %v", err)
	}
	if err := repo.DB().Exec(`INSERT INTO user_quota(user_id, daily_limit_gb, monthly_limit_gb, daily_used_bytes, monthly_used_bytes, day_key, month_key, disabled_by_quota, disabled_at, paused_forward_ids, created_time, updated_time) VALUES(2, 1, 0, ?, ?, ?, ?, 0, 0, '', ?, ?)`, bytesPerGB-100, bytesPerGB-100, dayKey, monthKey, nowMs, nowMs).Error; err != nil {
		t.Fatalf("insert user_quota: %v", err)
	}

	body, err := json.Marshal([]map[string]interface{}{
		{"n": "20_2_10", "u": 70, "d": 50},
		{"n": "20_2_10_tcp", "u": 40, "d": 30},
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/flow/upload?secret="+node.Secret, bytes.NewReader(body))
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}
	if got := mustQueryInt(t, repo, `SELECT status FROM forward WHERE id = 20`); got != 0 {
		t.Fatalf("expected forward paused immediately, got status=%d", got)
	}
	if got := mustQueryInt(t, repo, `SELECT disabled_by_quota FROM user_quota WHERE user_id = 2`); got != 1 {
		t.Fatalf("expected quota disabled flag=1, got %d", got)
	}
	if got := mustQueryInt(t, repo, `SELECT in_flow FROM forward WHERE id = 20`); got != 80 {
		t.Fatalf("expected forward in_flow=80, got %d", got)
	}
	if got := mustQueryInt(t, repo, `SELECT out_flow FROM forward WHERE id = 20`); got != 110 {
		t.Fatalf("expected forward out_flow=110, got %d", got)
	}
	metrics, err := repo.GetTunnelMetrics(tunnel.ID, 0, nowMs+60_000)
	if err != nil {
		t.Fatalf("get tunnel metrics: %v", err)
	}
	if len(metrics) != 1 || metrics[0].BytesIn != 80 || metrics[0].BytesOut != 110 {
		t.Fatalf("expected one aggregated metric row, got %#v", metrics)
	}

	body, err = json.Marshal([]map[string]interface{}{
		{"n": "20_2_10", "u": 10, "d": 20},
		{"n": "20_2_10", "u": 10, "d": 20},
		{"n": "20_2_10_tcp", "u": 10, "d": 20},
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req = httptest.NewRequest(http.MethodPost, "/flow/upload?secret="+node.Secret, bytes.NewReader(body))
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected second request status 200, got %d", res.Code)
	}
	if got := mustQueryInt(t, repo, `SELECT in_flow FROM forward WHERE id = 20`); got != 140 {
		t.Fatalf("expected forward in_flow=140 after second request, got %d", got)
	}
	if got := mustQueryInt(t, repo, `SELECT out_flow FROM forward WHERE id = 20`); got != 140 {
		t.Fatalf("expected forward out_flow=140 after second request, got %d", got)
	}
	metrics, err = repo.GetTunnelMetrics(tunnel.ID, 0, nowMs+60_000)
	if err != nil {
		t.Fatalf("get tunnel metrics after second request: %v", err)
	}
	if len(metrics) != 1 || metrics[0].BytesIn != 140 || metrics[0].BytesOut != 140 {
		t.Fatalf("expected one aggregated metric row after second request, got %#v", metrics)
	}
}
