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
	"go-backend/internal/store/model"
)

func TestNodeMetricsEndpoints(t *testing.T) {
	secret := "monitoring-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	seedContractUser(t, repo, 2, "normal_user", 1, 1)

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()

	t.Run("insert and query node metrics", func(t *testing.T) {
		metric := &model.NodeMetric{
			NodeID:      1,
			Timestamp:   now - 1000,
			CPUUsage:    45.5,
			MemUsage:    60.2,
			DiskUsage:   30.1,
			NetInBytes:  1024000,
			NetOutBytes: 2048000,
			NetInSpeed:  51200,
			NetOutSpeed: 102400,
			Load1:       1.5,
			Load5:       1.2,
			Load15:      0.9,
			TCPConns:    100,
			UDPConns:    50,
			Uptime:      86400,
		}
		if err := repo.InsertNodeMetric(metric); err != nil {
			t.Fatalf("insert node metric: %v", err)
		}

		metric2 := &model.NodeMetric{
			NodeID:      1,
			Timestamp:   now,
			CPUUsage:    50.0,
			MemUsage:    65.0,
			DiskUsage:   32.0,
			NetInBytes:  2048000,
			NetOutBytes: 4096000,
			NetInSpeed:  102400,
			NetOutSpeed: 204800,
			Load1:       2.0,
			Load5:       1.5,
			Load15:      1.0,
			TCPConns:    150,
			UDPConns:    75,
			Uptime:      172800,
		}
		if err := repo.InsertNodeMetric(metric2); err != nil {
			t.Fatalf("insert node metric 2: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/nodes/1/metrics", nil)
		req.Header.Set("Authorization", adminToken)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
		}

		metrics, ok := out.Data.([]interface{})
		if !ok {
			t.Fatalf("expected array, got %T", out.Data)
		}
		if len(metrics) != 2 {
			t.Fatalf("expected 2 metrics, got %d", len(metrics))
		}
	})

	t.Run("get latest node metric", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/nodes/1/metrics/latest", nil)
		req.Header.Set("Authorization", adminToken)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
		}

		metric, ok := out.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("expected object, got %T", out.Data)
		}
		if cpu, _ := metric["cpuUsage"].(float64); cpu != 50.0 {
			t.Fatalf("expected cpuUsage 50.0, got %v", cpu)
		}
	})

	t.Run("get metrics for non-existent node returns empty array", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/nodes/999/metrics", nil)
		req.Header.Set("Authorization", adminToken)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
		}
		metrics, ok := out.Data.([]interface{})
		if !ok {
			t.Fatalf("expected array, got %T", out.Data)
		}
		if len(metrics) != 0 {
			t.Fatalf("expected 0 metrics for non-existent node, got %d", len(metrics))
		}
	})

	t.Run("get latest metric for non-existent node returns nil", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/nodes/999/metrics/latest", nil)
		req.Header.Set("Authorization", adminToken)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
		}
		if out.Data != nil {
			t.Fatalf("expected nil data for non-existent node, got %v", out.Data)
		}
	})

	t.Run("invalid node id returns error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/nodes/invalid/metrics", nil)
		req.Header.Set("Authorization", adminToken)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code == 0 {
			t.Fatalf("expected error for invalid node id")
		}
	})

	t.Run("query with time range", func(t *testing.T) {
		start := now - 2000
		end := now - 500
		path := "/api/v1/monitor/nodes/1/metrics?start=" + jsonNumber(start) + "&end=" + jsonNumber(end)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", adminToken)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
		}
		metrics, ok := out.Data.([]interface{})
		if !ok {
			t.Fatalf("expected array, got %T", out.Data)
		}
		if len(metrics) != 1 {
			t.Fatalf("expected 1 metric in time range, got %d", len(metrics))
		}
	})
}

func TestTunnelMetricsEndpoints(t *testing.T) {
	secret := "monitoring-jwt-secret"
	router, repo := setupContractRouter(t, secret)

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()

	t.Run("insert and query tunnel metrics", func(t *testing.T) {
		metric := &model.TunnelMetric{
			TunnelID:     1,
			NodeID:       1,
			Timestamp:    now - 1000,
			BytesIn:      1024000,
			BytesOut:     2048000,
			Connections:  10,
			Errors:       0,
			AvgLatencyMs: 15.5,
		}
		if err := repo.InsertTunnelMetric(metric); err != nil {
			t.Fatalf("insert tunnel metric: %v", err)
		}

		metric2 := &model.TunnelMetric{
			TunnelID:     1,
			NodeID:       1,
			Timestamp:    now,
			BytesIn:      2048000,
			BytesOut:     4096000,
			Connections:  20,
			Errors:       1,
			AvgLatencyMs: 20.0,
		}
		if err := repo.InsertTunnelMetric(metric2); err != nil {
			t.Fatalf("insert tunnel metric 2: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/tunnels/1/metrics", nil)
		req.Header.Set("Authorization", adminToken)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
		}

		metrics, ok := out.Data.([]interface{})
		if !ok {
			t.Fatalf("expected array, got %T", out.Data)
		}
		if len(metrics) != 2 {
			t.Fatalf("expected 2 metrics, got %d", len(metrics))
		}
	})

	t.Run("get metrics for non-existent tunnel returns empty array", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/tunnels/999/metrics", nil)
		req.Header.Set("Authorization", adminToken)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
		}
		metrics, ok := out.Data.([]interface{})
		if !ok {
			t.Fatalf("expected array, got %T", out.Data)
		}
		if len(metrics) != 0 {
			t.Fatalf("expected 0 metrics for non-existent tunnel, got %d", len(metrics))
		}
	})

	t.Run("invalid tunnel id returns error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/tunnels/invalid/metrics", nil)
		req.Header.Set("Authorization", adminToken)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code == 0 {
			t.Fatalf("expected error for invalid tunnel id")
		}
	})
}

func TestServiceMonitorCRUD(t *testing.T) {
	secret := "monitoring-jwt-secret"
	router, repo := setupContractRouter(t, secret)

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	// Seed a node for node-executed monitors.
	now := time.Now().UnixMilli()
	n := &model.Node{
		Name:          "node-1",
		Secret:        "node-secret",
		ServerIP:      "127.0.0.1",
		Port:          "10000-10010",
		TCPListenAddr: "[::]",
		UDPListenAddr: "[::]",
		CreatedTime:   now,
		Status:        0,
	}
	if err := repo.DB().Create(n).Error; err != nil {
		t.Fatalf("seed node: %v", err)
	}

	t.Run("create service monitor - TCP", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":        "DNS Monitor",
			"type":        "tcp",
			"target":      "8.8.8.8:53",
			"intervalSec": 60,
			"timeoutSec":  5,
			"nodeId":      0,
			"enabled":     1,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/monitor/services/create", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
		}

		monitor, ok := out.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("expected object, got %T", out.Data)
		}
		if name, _ := monitor["name"].(string); name != "DNS Monitor" {
			t.Fatalf("expected name 'DNS Monitor', got %v", name)
		}
		if monitorType, _ := monitor["type"].(string); monitorType != "tcp" {
			t.Fatalf("expected type 'tcp', got %v", monitorType)
		}
	})

	t.Run("create service monitor - ICMP", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":        "Ping Monitor",
			"type":        "icmp",
			"target":      "8.8.8.8",
			"intervalSec": 30,
			"timeoutSec":  10,
			"nodeId":      n.ID,
			"enabled":     1,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/monitor/services/create", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
		}
	})

	t.Run("list service monitors", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/services", nil)
		req.Header.Set("Authorization", adminToken)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
		}

		monitors, ok := out.Data.([]interface{})
		if !ok {
			t.Fatalf("expected array, got %T", out.Data)
		}
		if len(monitors) != 2 {
			t.Fatalf("expected 2 monitors, got %d", len(monitors))
		}
	})

	t.Run("create monitor with invalid type returns error", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":        "Invalid Monitor",
			"type":        "http",
			"target":      "https://example.com",
			"intervalSec": 60,
			"timeoutSec":  5,
			"enabled":     1,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/monitor/services/create", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code == 0 {
			t.Fatalf("expected error for invalid monitor type")
		}
	})

	t.Run("create monitor with empty name returns error", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":        "",
			"type":        "tcp",
			"target":      "8.8.8.8:53",
			"intervalSec": 60,
			"timeoutSec":  5,
			"enabled":     1,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/monitor/services/create", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code == 0 {
			t.Fatalf("expected error for empty name")
		}
	})

	t.Run("create monitor with empty target returns error", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":        "No Target",
			"type":        "tcp",
			"target":      "",
			"intervalSec": 60,
			"timeoutSec":  5,
			"enabled":     1,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/monitor/services/create", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code == 0 {
			t.Fatalf("expected error for empty target")
		}
	})

	var monitorID int64
	t.Run("update service monitor", func(t *testing.T) {
		monitors, _ := repo.ListServiceMonitors()
		if len(monitors) == 0 {
			t.Fatalf("no monitors to update")
		}
		monitorID = monitors[0].ID

		payload := map[string]interface{}{
			"id":          monitorID,
			"name":        "Updated DNS Monitor",
			"type":        "tcp",
			"target":      "1.1.1.1:53",
			"intervalSec": 120,
			"timeoutSec":  10,
			"nodeId":      0,
			"enabled":     1,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/monitor/services/update", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
		}

		updated, _ := repo.GetServiceMonitor(monitorID)
		if updated.Name != "Updated DNS Monitor" {
			t.Fatalf("expected name 'Updated DNS Monitor', got %s", updated.Name)
		}
		if updated.IntervalSec != 120 {
			t.Fatalf("expected interval 120, got %d", updated.IntervalSec)
		}
	})

	t.Run("update non-existent monitor returns error", func(t *testing.T) {
		payload := map[string]interface{}{
			"id":          99999,
			"name":        "Non-existent",
			"type":        "tcp",
			"target":      "1.1.1.1:53",
			"intervalSec": 60,
			"timeoutSec":  5,
			"enabled":     1,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/monitor/services/update", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code == 0 {
			t.Fatalf("expected error for non-existent monitor")
		}
	})

	t.Run("delete service monitor", func(t *testing.T) {
		monitors, _ := repo.ListServiceMonitors()
		if len(monitors) < 2 {
			t.Fatalf("need at least 2 monitors to test delete")
		}
		deleteID := monitors[1].ID

		// Seed history so we can verify deletion cleanup.
		if err := repo.InsertServiceMonitorResult(&model.ServiceMonitorResult{
			MonitorID:    deleteID,
			NodeID:       0,
			Timestamp:    now,
			Success:      1,
			LatencyMs:    1,
			StatusCode:   0,
			ErrorMessage: "",
		}); err != nil {
			t.Fatalf("seed monitor result: %v", err)
		}

		payload := map[string]interface{}{
			"id": deleteID,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/monitor/services/delete", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
		}

		remaining, _ := repo.ListServiceMonitors()
		if len(remaining) != 1 {
			t.Fatalf("expected 1 remaining monitor, got %d", len(remaining))
		}
		results, err := repo.GetServiceMonitorResults(deleteID, 10)
		if err != nil {
			t.Fatalf("get deleted monitor results: %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("expected 0 results for deleted monitor, got %d", len(results))
		}
	})

	t.Run("delete non-existent monitor returns success", func(t *testing.T) {
		payload := map[string]interface{}{
			"id": 99999,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/monitor/services/delete", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0 for delete non-existent, got %d: %s", out.Code, out.Msg)
		}
	})
}

func TestServiceMonitorResults(t *testing.T) {
	secret := "monitoring-jwt-secret"
	router, repo := setupContractRouter(t, secret)

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()

	monitor := &model.ServiceMonitor{
		Name:        "Test Monitor",
		Type:        "tcp",
		Target:      "8.8.8.8:53",
		IntervalSec: 60,
		TimeoutSec:  5,
		NodeID:      0,
		Enabled:     1,
		CreatedTime: now,
		UpdatedTime: now,
	}
	if err := repo.CreateServiceMonitor(monitor); err != nil {
		t.Fatalf("create monitor: %v", err)
	}

	t.Run("insert and query monitor results", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			result := &model.ServiceMonitorResult{
				MonitorID:    monitor.ID,
				NodeID:       0,
				Timestamp:    now - int64(i*60000),
				Success:      1,
				LatencyMs:    float64(10 + i),
				StatusCode:   0,
				ErrorMessage: "",
			}
			if err := repo.InsertServiceMonitorResult(result); err != nil {
				t.Fatalf("insert result %d: %v", i, err)
			}
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/services/"+jsonNumber(monitor.ID)+"/results", nil)
		req.Header.Set("Authorization", adminToken)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
		}

		results, ok := out.Data.([]interface{})
		if !ok {
			t.Fatalf("expected array, got %T", out.Data)
		}
		if len(results) != 5 {
			t.Fatalf("expected 5 results, got %d", len(results))
		}
	})

	t.Run("query results with limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/services/"+jsonNumber(monitor.ID)+"/results?limit=3", nil)
		req.Header.Set("Authorization", adminToken)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
		}

		results, ok := out.Data.([]interface{})
		if !ok {
			t.Fatalf("expected array, got %T", out.Data)
		}
		if len(results) != 3 {
			t.Fatalf("expected 3 results with limit, got %d", len(results))
		}
	})

	t.Run("query results for non-existent monitor returns empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/services/99999/results", nil)
		req.Header.Set("Authorization", adminToken)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
		}
		results, ok := out.Data.([]interface{})
		if !ok {
			t.Fatalf("expected array, got %T", out.Data)
		}
		if len(results) != 0 {
			t.Fatalf("expected 0 results for non-existent monitor, got %d", len(results))
		}
	})

	t.Run("result with error message", func(t *testing.T) {
		result := &model.ServiceMonitorResult{
			MonitorID:    monitor.ID,
			NodeID:       0,
			Timestamp:    now,
			Success:      0,
			LatencyMs:    0,
			StatusCode:   0,
			ErrorMessage: "connection refused",
		}
		if err := repo.InsertServiceMonitorResult(result); err != nil {
			t.Fatalf("insert failed result: %v", err)
		}

		results, _ := repo.GetServiceMonitorResults(monitor.ID, 100)
		found := false
		for _, r := range results {
			if r.ErrorMessage == "connection refused" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected to find result with error message")
		}
	})
}

func TestServiceMonitorLatestResultsEndpoint(t *testing.T) {
	secret := "monitoring-jwt-secret"
	router, repo := setupContractRouter(t, secret)

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()

	m1 := &model.ServiceMonitor{
		Name:        "m1",
		Type:        "tcp",
		Target:      "127.0.0.1:1",
		IntervalSec: 60,
		TimeoutSec:  1,
		NodeID:      0,
		Enabled:     1,
		CreatedTime: now,
		UpdatedTime: now,
	}
	if err := repo.CreateServiceMonitor(m1); err != nil {
		t.Fatalf("create monitor 1: %v", err)
	}
	m2 := &model.ServiceMonitor{
		Name:        "m2",
		Type:        "tcp",
		Target:      "127.0.0.1:2",
		IntervalSec: 60,
		TimeoutSec:  1,
		NodeID:      0,
		Enabled:     1,
		CreatedTime: now,
		UpdatedTime: now,
	}
	if err := repo.CreateServiceMonitor(m2); err != nil {
		t.Fatalf("create monitor 2: %v", err)
	}

	if err := repo.InsertServiceMonitorResult(&model.ServiceMonitorResult{
		MonitorID: m1.ID,
		NodeID:    0,
		Timestamp: now - 60_000,
		Success:   1,
		LatencyMs: 10,
	}); err != nil {
		t.Fatalf("insert m1 old result: %v", err)
	}
	if err := repo.InsertServiceMonitorResult(&model.ServiceMonitorResult{
		MonitorID: m1.ID,
		NodeID:    0,
		Timestamp: now,
		Success:   0,
		LatencyMs: 20,
	}); err != nil {
		t.Fatalf("insert m1 latest result: %v", err)
	}
	if err := repo.InsertServiceMonitorResult(&model.ServiceMonitorResult{
		MonitorID: m2.ID,
		NodeID:    0,
		Timestamp: now - 30_000,
		Success:   1,
		LatencyMs: 5,
	}); err != nil {
		t.Fatalf("insert m2 result: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/services/latest-results", nil)
	req.Header.Set("Authorization", adminToken)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
	}

	rows, ok := out.Data.([]interface{})
	if !ok {
		t.Fatalf("expected array, got %T", out.Data)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 latest-result rows, got %d", len(rows))
	}

	seen := make(map[int64]int64, len(rows))
	for _, raw := range rows {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		mid, _ := m["monitorId"].(float64)
		ts, _ := m["timestamp"].(float64)
		if mid <= 0 {
			continue
		}
		seen[int64(mid)] = int64(ts)
	}
	if len(seen) != 2 {
		t.Fatalf("expected 2 unique monitorIds, got %d", len(seen))
	}
	if seen[m1.ID] != now {
		t.Fatalf("expected m1 latest timestamp %d, got %d", now, seen[m1.ID])
	}
	if seen[m2.ID] != now-30_000 {
		t.Fatalf("expected m2 latest timestamp %d, got %d", now-30_000, seen[m2.ID])
	}
}

func TestServiceMonitorLimitsEndpoint(t *testing.T) {
	secret := "monitoring-jwt-secret"
	router, _ := setupContractRouter(t, secret)

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/services/limits", nil)
	req.Header.Set("Authorization", adminToken)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
	}

	limits, ok := out.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected object, got %T", out.Data)
	}
	for _, key := range []string{
		"checkerScanIntervalSec",
		"minIntervalSec",
		"defaultIntervalSec",
		"minTimeoutSec",
		"defaultTimeoutSec",
		"maxTimeoutSec",
	} {
		if _, ok := limits[key]; !ok {
			t.Fatalf("expected %q in limits", key)
		}
	}
}

func TestMonitorNodeAndTunnelListEndpoints(t *testing.T) {
	secret := "monitoring-jwt-secret"
	router, repo := setupContractRouter(t, secret)

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()

	n1 := &model.Node{
		Name:          "node-a",
		Secret:        "node-a-secret",
		ServerIP:      "127.0.0.1",
		Port:          "10000-10010",
		TCPListenAddr: "[::]",
		UDPListenAddr: "[::]",
		Inx:           2,
		CreatedTime:   now,
		Status:        1,
	}
	if err := repo.DB().Create(n1).Error; err != nil {
		t.Fatalf("seed node 1: %v", err)
	}
	n2 := &model.Node{
		Name:          "node-b",
		Secret:        "node-b-secret",
		ServerIP:      "127.0.0.1",
		Port:          "11000-11010",
		TCPListenAddr: "[::]",
		UDPListenAddr: "[::]",
		Inx:           1,
		CreatedTime:   now,
		Status:        0,
	}
	if err := repo.DB().Create(n2).Error; err != nil {
		t.Fatalf("seed node 2: %v", err)
	}

	t1 := &model.Tunnel{
		Name:         "tunnel-a",
		TrafficRatio: 1.0,
		Type:         1,
		Protocol:     "tls",
		Flow:         1,
		CreatedTime:  now,
		UpdatedTime:  now,
		Status:       1,
		Inx:          2,
	}
	if err := repo.DB().Create(t1).Error; err != nil {
		t.Fatalf("seed tunnel 1: %v", err)
	}
	t2 := &model.Tunnel{
		Name:         "tunnel-b",
		TrafficRatio: 1.0,
		Type:         1,
		Protocol:     "tls",
		Flow:         1,
		CreatedTime:  now,
		UpdatedTime:  now,
		Status:       0,
		Inx:          1,
	}
	if err := repo.DB().Create(t2).Error; err != nil {
		t.Fatalf("seed tunnel 2: %v", err)
	}

	// Nodes
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/nodes", nil)
		req.Header.Set("Authorization", adminToken)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
		}
		rows, ok := out.Data.([]interface{})
		if !ok {
			t.Fatalf("expected array, got %T", out.Data)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 nodes, got %d", len(rows))
		}
		first, _ := rows[0].(map[string]interface{})
		if name, _ := first["name"].(string); name != "node-b" {
			t.Fatalf("expected node order by inx (node-b first), got %q", name)
		}
	}

	// Tunnels
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/tunnels", nil)
		req.Header.Set("Authorization", adminToken)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
		}
		rows, ok := out.Data.([]interface{})
		if !ok {
			t.Fatalf("expected array, got %T", out.Data)
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 tunnels, got %d", len(rows))
		}
		first, _ := rows[0].(map[string]interface{})
		if name, _ := first["name"].(string); name != "tunnel-b" {
			t.Fatalf("expected tunnel order by inx (tunnel-b first), got %q", name)
		}
	}
}

func TestMonitorPermissionAdminEndpoints(t *testing.T) {
	secret := "monitoring-jwt-secret"
	router, repo := setupContractRouter(t, secret)

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()
	u := &model.User{
		User:          "test-user",
		Pwd:           "pwd",
		RoleID:        1,
		ExpTime:       now + 3600_000,
		Flow:          0,
		FlowResetTime: now,
		Num:           0,
		CreatedTime:   now,
		Status:        1,
	}
	if err := repo.DB().Create(u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	assignBody, _ := json.Marshal(map[string]interface{}{"userId": u.ID})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/monitor/permission/assign", bytes.NewReader(assignBody))
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode assign response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected code 0 on assign, got %d: %s", out.Code, out.Msg)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/permission/list", nil)
	listReq.Header.Set("Authorization", adminToken)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)

	if err := json.NewDecoder(listRes.Body).Decode(&out); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected code 0 on list, got %d: %s", out.Code, out.Msg)
	}
	rows, ok := out.Data.([]interface{})
	if !ok {
		t.Fatalf("expected array on list, got %T", out.Data)
	}
	found := false
	for _, raw := range rows {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		uid, _ := m["userId"].(float64)
		if int64(uid) == u.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected assigned permission to appear in list")
	}

	removeBody, _ := json.Marshal(map[string]interface{}{"userId": u.ID})
	remReq := httptest.NewRequest(http.MethodPost, "/api/v1/monitor/permission/remove", bytes.NewReader(removeBody))
	remReq.Header.Set("Authorization", adminToken)
	remReq.Header.Set("Content-Type", "application/json")
	remRes := httptest.NewRecorder()
	router.ServeHTTP(remRes, remReq)

	if err := json.NewDecoder(remRes.Body).Decode(&out); err != nil {
		t.Fatalf("decode remove response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected code 0 on remove, got %d: %s", out.Code, out.Msg)
	}

	listReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/permission/list", nil)
	listReq2.Header.Set("Authorization", adminToken)
	listRes2 := httptest.NewRecorder()
	router.ServeHTTP(listRes2, listReq2)

	if err := json.NewDecoder(listRes2.Body).Decode(&out); err != nil {
		t.Fatalf("decode list response after remove: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected code 0 on list after remove, got %d: %s", out.Code, out.Msg)
	}
	rows2, ok := out.Data.([]interface{})
	if !ok {
		t.Fatalf("expected array on list after remove, got %T", out.Data)
	}
	for _, raw := range rows2 {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		uid, _ := m["userId"].(float64)
		if int64(uid) == u.ID {
			t.Fatalf("expected removed permission to be absent")
		}
	}
}

func TestMetricBatchInsert(t *testing.T) {
	secret := "monitoring-jwt-secret"
	_, repo := setupContractRouter(t, secret)

	t.Run("batch insert node metrics", func(t *testing.T) {
		now := time.Now().UnixMilli()
		var metrics []*model.NodeMetric
		for i := 0; i < 10; i++ {
			metrics = append(metrics, &model.NodeMetric{
				NodeID:      1,
				Timestamp:   now - int64(i*1000),
				CPUUsage:    float64(40 + i),
				MemUsage:    float64(50 + i),
				DiskUsage:   30.0,
				NetInBytes:  int64(1000 * (i + 1)),
				NetOutBytes: int64(2000 * (i + 1)),
				NetInSpeed:  100,
				NetOutSpeed: 200,
				Load1:       1.0,
				Load5:       0.8,
				Load15:      0.6,
				TCPConns:    100,
				UDPConns:    50,
			})
		}

		if err := repo.InsertNodeMetricBatch(metrics); err != nil {
			t.Fatalf("batch insert: %v", err)
		}

		retrieved, err := repo.GetNodeMetrics(1, now-10000, now+1000)
		if err != nil {
			t.Fatalf("get metrics: %v", err)
		}
		if len(retrieved) != 10 {
			t.Fatalf("expected 10 metrics, got %d", len(retrieved))
		}
	})

	t.Run("batch insert tunnel metrics", func(t *testing.T) {
		now := time.Now().UnixMilli()
		var metrics []*model.TunnelMetric
		for i := 0; i < 5; i++ {
			metrics = append(metrics, &model.TunnelMetric{
				TunnelID:     1,
				NodeID:       1,
				Timestamp:    now - int64(i*1000),
				BytesIn:      int64(1000 * (i + 1)),
				BytesOut:     int64(2000 * (i + 1)),
				Connections:  int64(i + 1),
				Errors:       0,
				AvgLatencyMs: float64(10 + i),
			})
		}

		if err := repo.InsertTunnelMetricBatch(metrics); err != nil {
			t.Fatalf("batch insert: %v", err)
		}

		retrieved, err := repo.GetTunnelMetrics(1, 0, now+1000)
		if err != nil {
			t.Fatalf("get metrics: %v", err)
		}
		if len(retrieved) != 5 {
			t.Fatalf("expected 5 metrics, got %d", len(retrieved))
		}
	})
}

func TestMetricPruning(t *testing.T) {
	secret := "monitoring-jwt-secret"
	_, repo := setupContractRouter(t, secret)

	now := time.Now().UnixMilli()

	oldMetric := &model.NodeMetric{
		NodeID:    1,
		Timestamp: now - 8*24*60*60*1000,
		CPUUsage:  50,
		MemUsage:  60,
		DiskUsage: 30,
		Load1:     1.0,
		Load5:     0.8,
		Load15:    0.6,
		TCPConns:  100,
		UDPConns:  50,
	}
	if err := repo.InsertNodeMetric(oldMetric); err != nil {
		t.Fatalf("insert old metric: %v", err)
	}

	newMetric := &model.NodeMetric{
		NodeID:    1,
		Timestamp: now,
		CPUUsage:  55,
		MemUsage:  65,
		DiskUsage: 32,
		Load1:     1.2,
		Load5:     0.9,
		Load15:    0.7,
		TCPConns:  120,
		UDPConns:  60,
	}
	if err := repo.InsertNodeMetric(newMetric); err != nil {
		t.Fatalf("insert new metric: %v", err)
	}

	cutoff := now - 7*24*60*60*1000
	if err := repo.PruneNodeMetrics(cutoff); err != nil {
		t.Fatalf("prune metrics: %v", err)
	}

	retrieved, err := repo.GetNodeMetrics(1, 0, now+1000)
	if err != nil {
		t.Fatalf("get metrics: %v", err)
	}
	if len(retrieved) != 1 {
		t.Fatalf("expected 1 metric after pruning, got %d", len(retrieved))
	}
}

func TestMonitoringAuthRequired(t *testing.T) {
	secret := "monitoring-jwt-secret"
	router, _ := setupContractRouter(t, secret)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"node metrics", http.MethodGet, "/api/v1/monitor/nodes/1/metrics"},
		{"node metrics latest", http.MethodGet, "/api/v1/monitor/nodes/1/metrics/latest"},
		{"tunnel metrics", http.MethodGet, "/api/v1/monitor/tunnels/1/metrics"},
		{"service list", http.MethodGet, "/api/v1/monitor/services"},
		{"service results", http.MethodGet, "/api/v1/monitor/services/1/results"},
	}

	for _, tc := range tests {
		t.Run(tc.name+" requires auth", func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)

			var out response.R
			if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if out.Code != 401 {
				t.Fatalf("expected 401 for missing auth, got %d", out.Code)
			}
		})
	}
}

func TestMonitorAccessEndpoint(t *testing.T) {
	secret := "monitoring-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	seedContractUser(t, repo, 2, "normal_user", 1, 1)

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}
	userToken, err := auth.GenerateToken(2, "normal_user", 1, secret)
	if err != nil {
		t.Fatalf("generate user token: %v", err)
	}

	assertAllowed := func(t *testing.T, token string, want bool) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/monitor/access", nil)
		req.Header.Set("Authorization", token)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d: %s", out.Code, out.Msg)
		}
		data, ok := out.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("expected object, got %T", out.Data)
		}
		allowed, _ := data["allowed"].(bool)
		if allowed != want {
			t.Fatalf("expected allowed=%v, got %v", want, allowed)
		}
	}

	t.Run("admin is allowed", func(t *testing.T) {
		assertAllowed(t, adminToken, true)
	})

	t.Run("non-admin without grant can open empty monitor", func(t *testing.T) {
		assertAllowed(t, userToken, true)
	})

	now := time.Now().UnixMilli()
	if err := repo.InsertMonitorPermission(2, now); err != nil {
		t.Fatalf("insert monitor permission: %v", err)
	}

	t.Run("non-admin with grant is allowed", func(t *testing.T) {
		assertAllowed(t, userToken, true)
	})
}

func TestMonitoringPermissionRequired(t *testing.T) {
	secret := "monitoring-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	seedContractUser(t, repo, 2, "normal_user", 1, 1)

	userToken, err := auth.GenerateToken(2, "normal_user", 1, secret)
	if err != nil {
		t.Fatalf("generate user token: %v", err)
	}

	createBody, _ := json.Marshal(map[string]interface{}{
		"name":        "NonAdmin Monitor",
		"type":        "tcp",
		"target":      "127.0.0.1:1",
		"intervalSec": 60,
		"timeoutSec":  5,
		"nodeId":      0,
		"enabled":     1,
	})

	checks := []struct {
		name   string
		method string
		path   string
		body   []byte
		want   int
	}{
		{"service list", http.MethodGet, "/api/v1/monitor/services", nil, 0},
		{"service create", http.MethodPost, "/api/v1/monitor/services/create", createBody, 403},
		{"node metrics", http.MethodGet, "/api/v1/monitor/nodes/1/metrics", nil, 0},
	}

	for _, tc := range checks {
		t.Run(tc.name+" without grant", func(t *testing.T) {
			var req *http.Request
			if tc.body != nil {
				req = httptest.NewRequest(tc.method, tc.path, bytes.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
			req.Header.Set("Authorization", userToken)
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)

			var out response.R
			if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if out.Code != tc.want {
				t.Fatalf("expected code %d without grant, got %d msg=%q", tc.want, out.Code, out.Msg)
			}
		})
	}

	// Grant monitor permission and verify access.
	now := time.Now().UnixMilli()
	if err := repo.InsertMonitorPermission(2, now); err != nil {
		t.Fatalf("insert monitor permission: %v", err)
	}

	allowed := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{"service list", http.MethodGet, "/api/v1/monitor/services", nil},
	}

	for _, tc := range allowed {
		t.Run(tc.name+" allowed with grant", func(t *testing.T) {
			var req *http.Request
			if tc.body != nil {
				req = httptest.NewRequest(tc.method, tc.path, bytes.NewReader(tc.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
			req.Header.Set("Authorization", userToken)
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)

			var out response.R
			if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if out.Code != 0 {
				t.Fatalf("expected code 0 with grant, got %d msg=%q", out.Code, out.Msg)
			}
		})
	}

	t.Run("service create still admin only with grant", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/monitor/services/create", bytes.NewReader(createBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", userToken)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 403 {
			t.Fatalf("expected code 403 for service create with user grant, got %d msg=%q", out.Code, out.Msg)
		}
	})
}
