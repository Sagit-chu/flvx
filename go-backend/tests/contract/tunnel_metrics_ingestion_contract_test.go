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

func TestFlowUploadInsertsTunnelMetrics(t *testing.T) {
	secret := "monitoring-jwt-secret"
	router, repo := setupContractRouter(t, secret)

	now := time.Now().UnixMilli()

	node := &model.Node{
		Name:          "node-1",
		Secret:        "node-secret",
		ServerIP:      "127.0.0.1",
		Port:          "10000-10010",
		TCPListenAddr: "[::]",
		UDPListenAddr: "[::]",
		CreatedTime:   now,
		Status:        1,
	}
	if err := repo.DB().Create(node).Error; err != nil {
		t.Fatalf("seed node: %v", err)
	}

	tunnel := &model.Tunnel{
		Name:         "tunnel-1",
		TrafficRatio: 1.0,
		Type:         1,
		Protocol:     "tls",
		Flow:         1,
		CreatedTime:  now,
		UpdatedTime:  now,
		Status:       1,
	}
	if err := repo.DB().Create(tunnel).Error; err != nil {
		t.Fatalf("seed tunnel: %v", err)
	}

	forward := &model.Forward{
		UserID:      123,
		UserName:    "user-123",
		Name:        "forward-1",
		TunnelID:    tunnel.ID,
		RemoteAddr:  "1.1.1.1:80",
		CreatedTime: now,
		UpdatedTime: now,
		Status:      1,
	}
	if err := repo.DB().Create(forward).Error; err != nil {
		t.Fatalf("seed forward: %v", err)
	}
	if err := repo.DB().Create(&model.ForwardPort{ForwardID: forward.ID, NodeID: node.ID, Port: 10000}).Error; err != nil {
		t.Fatalf("seed forward node ownership: %v", err)
	}

	serviceName := jsonNumber(forward.ID) + "_123_0"
	body, _ := json.Marshal([]map[string]interface{}{{
		"n": serviceName,
		"u": 200,
		"d": 100,
	}})

	req := httptest.NewRequest(http.MethodPost, "/flow/upload?secret="+node.Secret, bytes.NewReader(body))
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	metrics, err := repo.GetTunnelMetrics(tunnel.ID, 0, now+60_000)
	if err != nil {
		t.Fatalf("get tunnel metrics: %v", err)
	}
	if len(metrics) != 1 {
		t.Fatalf("expected 1 tunnel metric row, got %d", len(metrics))
	}

	if metrics[0].TunnelID != tunnel.ID {
		t.Fatalf("expected tunnelId %d, got %d", tunnel.ID, metrics[0].TunnelID)
	}
	if metrics[0].NodeID != node.ID {
		t.Fatalf("expected nodeId %d, got %d", node.ID, metrics[0].NodeID)
	}
	if metrics[0].BytesIn != 100 {
		t.Fatalf("expected bytesIn 100, got %d", metrics[0].BytesIn)
	}
	if metrics[0].BytesOut != 200 {
		t.Fatalf("expected bytesOut 200, got %d", metrics[0].BytesOut)
	}
}
