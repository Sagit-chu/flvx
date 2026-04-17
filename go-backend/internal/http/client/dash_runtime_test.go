package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-backend/internal/dashruntime"
)

func TestDashRuntimeClientAddsBearerAuth(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client := NewDashRuntimeClient(DashRuntimeClientConfig{
		BaseURLOverride: server.URL,
		Timeout:         time.Second,
	})

	_, err := client.ListRules(context.Background(), DashRuntimeNode{
		ServerIP: server.URL,
		Secret:   "node-secret",
	})
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if authHeader != "Bearer node-secret" {
		t.Fatalf("expected bearer header, got %q", authHeader)
	}
}

func TestDashRuntimeClientPauseServices(t *testing.T) {
	var (
		method string
		path   string
		body   dashRuntimeServiceControlRequest
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewDashRuntimeClient(DashRuntimeClientConfig{
		BaseURLOverride: server.URL,
		Timeout:         time.Second,
	})

	err := client.PauseServices(context.Background(), DashRuntimeNode{ServerIP: server.URL, Secret: "node-secret"}, []string{"svc-a_tcp"})
	if err != nil {
		t.Fatalf("PauseServices: %v", err)
	}
	if method != http.MethodPost {
		t.Fatalf("expected POST, got %s", method)
	}
	if path != "/api/relay/services/pause" {
		t.Fatalf("expected pause path, got %s", path)
	}
	if len(body.Services) != 1 || body.Services[0] != "svc-a_tcp" {
		t.Fatalf("unexpected pause body: %+v", body)
	}
}

func TestDashRuntimeClientCheckService(t *testing.T) {
	var reqBody DashServiceCheckRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/monitor/check" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"latencyMs":12,"statusCode":200}`))
	}))
	defer server.Close()

	client := NewDashRuntimeClient(DashRuntimeClientConfig{
		BaseURLOverride: server.URL,
		Timeout:         time.Second,
	})

	resp, err := client.CheckService(context.Background(), DashRuntimeNode{ServerIP: server.URL, Secret: "node-secret"}, DashServiceCheckRequest{
		Type:       "tcp",
		Target:     "example.com:443",
		TimeoutSec: 5,
	})
	if err != nil {
		t.Fatalf("CheckService: %v", err)
	}
	if reqBody.Type != "tcp" || reqBody.Target != "example.com:443" || reqBody.TimeoutSec != 5 {
		t.Fatalf("unexpected request body: %+v", reqBody)
	}
	if !resp.Success || resp.LatencyMs != 12 || resp.StatusCode != 200 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestDashRuntimeClientUpsertRule(t *testing.T) {
	var (
		method string
		path   string
		body   map[string]interface{}
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/relay/rules" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"id":"forward-11-node-3-port-8080"}]`))
			return
		}
		method = r.Method
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewDashRuntimeClient(DashRuntimeClientConfig{
		BaseURLOverride: server.URL,
		Timeout:         time.Second,
	})

	rule := dashruntime.RelayRulePayload{
		ID:       "forward-11-node-3-port-8080",
		Protocol: "tcp",
		Listen:   "0.0.0.0:8080",
		Enabled:  true,
		StagePools: []dashruntime.StagePoolPayload{{
			Policy: "round_robin",
			Backends: []dashruntime.StageBackendPayload{{
				ID:      "stage-a",
				Server:  "127.0.0.1:18080",
				Token:   "relay-secret",
				Enabled: true,
				Weight:  1,
			}},
		}},
	}
	if err := client.UpsertRule(context.Background(), DashRuntimeNode{ServerIP: server.URL, Secret: "node-secret"}, rule); err != nil {
		t.Fatalf("UpsertRule: %v", err)
	}
	if method != http.MethodPost {
		t.Fatalf("expected POST, got %s", method)
	}
	if path != "/api/relay/rules" {
		t.Fatalf("unexpected path: %s", path)
	}
	if body["id"] != rule.ID {
		t.Fatalf("create body should include id: %+v", body)
	}
	exitPool, ok := body["exit_pool"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected exit_pool compatibility field, got %+v", body)
	}
	if exitPool["policy"] != "round_robin" {
		t.Fatalf("unexpected exit_pool policy: %+v", exitPool)
	}
	if _, exists := body["stage_pools"]; exists {
		t.Fatalf("upsert body should not include stage_pools for current Dash API: %+v", body)
	}
	if body["listen"] != rule.Listen || body["protocol"] != rule.Protocol || body["enabled"] != rule.Enabled {
		t.Fatalf("unexpected rule payload: %+v", body)
	}
}

func TestDashRuntimeClientUpsertRuleCreatesWhenRuleMissing(t *testing.T) {
	var calls []struct {
		Method string
		Path   string
		Body   map[string]interface{}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/relay/rules" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
			return
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		calls = append(calls, struct {
			Method string
			Path   string
			Body   map[string]interface{}
		}{Method: r.Method, Path: r.URL.Path, Body: body})
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewDashRuntimeClient(DashRuntimeClientConfig{
		BaseURLOverride: server.URL,
		Timeout:         time.Second,
	})

	rule := dashruntime.RelayRulePayload{
		ID:       "forward-11-node-3-port-8080",
		Protocol: "tcp",
		Listen:   "0.0.0.0:8080",
		Enabled:  true,
		StagePools: []dashruntime.StagePoolPayload{{
			Policy: "round_robin",
			Backends: []dashruntime.StageBackendPayload{{
				ID:      "stage-a",
				Server:  "127.0.0.1:18080",
				Token:   "relay-secret",
				Enabled: true,
				Weight:  1,
			}},
		}},
	}
	if err := client.UpsertRule(context.Background(), DashRuntimeNode{ServerIP: server.URL, Secret: "node-secret"}, rule); err != nil {
		t.Fatalf("UpsertRule: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one mutating call, got %+v", calls)
	}
	if calls[0].Method != http.MethodPost || calls[0].Path != "/api/relay/rules" {
		t.Fatalf("expected create POST /api/relay/rules, got %+v", calls[0])
	}
	if calls[0].Body["id"] != rule.ID {
		t.Fatalf("expected create body to include id, got %+v", calls[0].Body)
	}
}

func TestDashRuntimeClientUpsertRuleFallsBackToUpdateOnCreateConflict(t *testing.T) {
	var calls []struct {
		Method string
		Path   string
		Body   map[string]interface{}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		calls = append(calls, struct {
			Method string
			Path   string
			Body   map[string]interface{}
		}{Method: r.Method, Path: r.URL.Path, Body: body})
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"validation error: rule 'forward-11-node-3-port-8080' already exists"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewDashRuntimeClient(DashRuntimeClientConfig{
		BaseURLOverride: server.URL,
		Timeout:         time.Second,
	})

	rule := dashruntime.RelayRulePayload{
		ID:       "forward-11-node-3-port-8080",
		Protocol: "tcp",
		Listen:   "0.0.0.0:8080",
		Enabled:  true,
		StagePools: []dashruntime.StagePoolPayload{{
			Policy: "round_robin",
			Backends: []dashruntime.StageBackendPayload{{
				ID:      "stage-a",
				Server:  "127.0.0.1:18080",
				Token:   "relay-secret",
				Enabled: true,
				Weight:  1,
			}},
		}},
	}
	if err := client.UpsertRule(context.Background(), DashRuntimeNode{ServerIP: server.URL, Secret: "node-secret"}, rule); err != nil {
		t.Fatalf("UpsertRule: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected create then update fallback, got %+v", calls)
	}
	if calls[0].Method != http.MethodPost || calls[0].Path != "/api/relay/rules" {
		t.Fatalf("unexpected first call: %+v", calls[0])
	}
	if calls[1].Method != http.MethodPut || calls[1].Path != "/api/relay/rules/forward-11-node-3-port-8080" {
		t.Fatalf("unexpected fallback call: %+v", calls[1])
	}
	if _, exists := calls[1].Body["id"]; exists {
		t.Fatalf("expected fallback update body to omit id, got %+v", calls[1].Body)
	}
}

func TestDashRuntimeClientGetStatus(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"exit_state":{"active":{"server":"127.0.0.1:18080"}},"rule_count":0,"rule_backends":[]}`))
	}))
	defer server.Close()

	client := NewDashRuntimeClient(DashRuntimeClientConfig{
		BaseURLOverride: server.URL,
		Timeout:         time.Second,
	})

	status, err := client.GetStatus(context.Background(), DashRuntimeNode{ServerIP: server.URL, Secret: "node-secret"})
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if method != http.MethodGet {
		t.Fatalf("expected GET, got %s", method)
	}
	if path != "/api/relay/status" {
		t.Fatalf("unexpected path: %s", path)
	}
	if status.RuleCount != 0 {
		t.Fatalf("unexpected status payload: %+v", status)
	}
}
