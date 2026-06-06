package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-backend/internal/auth"
	"go-backend/internal/http/middleware"
	runtimenft "go-backend/internal/runtime/nftables"
	"go-backend/internal/store/repo"
)

type fakeNftablesManager struct {
	testErr        error
	reconcileErr   error
	reconcileHit   int
	clearErr       error
	clearHit       int
	collectErr     error
	collectHit     int
	counterSamples []runtimenft.CounterSample
	lastConfig     runtimenft.SSHConfig
	lastPlan       runtimenft.NodePlan
}

func (f *fakeNftablesManager) Test(_ context.Context, cfg runtimenft.SSHConfig) error {
	f.lastConfig = cfg
	return f.testErr
}

func (f *fakeNftablesManager) Reconcile(_ context.Context, cfg runtimenft.SSHConfig, plan runtimenft.NodePlan) (runtimenft.ApplyResult, error) {
	f.reconcileHit++
	f.lastConfig = cfg
	f.lastPlan = plan
	if f.reconcileErr != nil {
		return runtimenft.ApplyResult{}, f.reconcileErr
	}
	return runtimenft.ApplyResult{
		NodeID: plan.NodeID,
		Script: "table inet flvx {}",
		Hashes: map[int64]string{plan.NodeID: "hash"},
	}, nil
}

func (f *fakeNftablesManager) Clear(context.Context, runtimenft.SSHConfig) error {
	f.clearHit++
	return f.clearErr
}

func (f *fakeNftablesManager) CollectCounters(_ context.Context, cfg runtimenft.SSHConfig) ([]runtimenft.CounterSample, error) {
	f.collectHit++
	f.lastConfig = cfg
	if f.collectErr != nil {
		return nil, f.collectErr
	}
	return f.counterSamples, nil
}

type nftablesTestFixture struct {
	handler *Handler
	nodeID  int64
}

func TestTunnelCreateRejectsNftablesEntryNodeWithoutSSHConfig(t *testing.T) {
	fixture := setupNftablesHandler(t)
	err := fixture.handler.validateNftablesTunnelState([]int64{fixture.nodeID})
	if err == nil {
		t.Fatalf("expected validation failure")
	}
	if !strings.Contains(err.Error(), "SSH") {
		t.Fatalf("expected SSH config validation error, got %q", err)
	}
}

func TestTunnelUpdateRejectsNftablesEntryNodeWhenCapabilityTestFails(t *testing.T) {
	fixture := setupNftablesHandler(t)
	h := fixture.handler
	manager := &fakeNftablesManager{testErr: errors.New("ssh failed")}
	h.nftablesManager = manager
	seedNftablesSSHConfig(t, h, fixture.nodeID)
	err := h.validateNftablesTunnelState([]int64{fixture.nodeID})
	if err == nil {
		t.Fatalf("expected validation failure")
	}
	if !strings.Contains(err.Error(), "ssh failed") {
		t.Fatalf("expected capability error in response, got %q", err)
	}
}

func TestSyncForwardServicesWithWarningsUsesNftablesRuntime(t *testing.T) {
	fixture := setupNftablesHandler(t)
	h := fixture.handler
	manager := &fakeNftablesManager{}
	h.nftablesManager = manager
	seedNftablesSSHConfig(t, h, fixture.nodeID)
	tunnelID := seedTunnelForNftables(t, h, "nft-tunnel", fixture.nodeID)
	forward := seedForwardForNftables(t, h, tunnelID, fixture.nodeID, "203.0.113.9:8080")

	warnings, err := h.syncForwardServicesWithWarnings(forward, "UpdateService", true)
	if err != nil {
		t.Fatalf("sync forward services: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if manager.reconcileHit != 1 {
		t.Fatalf("expected nftables reconcile to run once, got %d", manager.reconcileHit)
	}
	if manager.lastPlan.NodeID != fixture.nodeID {
		t.Fatalf("expected plan for node %d, got %+v", fixture.nodeID, manager.lastPlan)
	}
	if len(manager.lastPlan.Rules) != 1 || manager.lastPlan.Rules[0].ForwardID != forward.ID {
		t.Fatalf("unexpected plan: %+v", manager.lastPlan)
	}
}

func TestNodeNftablesTestEndpointRunsCapabilityCheck(t *testing.T) {
	fixture := setupNftablesHandler(t)
	seedNftablesSSHConfig(t, fixture.handler, fixture.nodeID)
	manager := &fakeNftablesManager{}
	fixture.handler.nftablesManager = manager

	res := postJSONToHandler(t, fixture.handler.nodeNftablesTest, map[string]int64{"nodeId": fixture.nodeID})
	assertNftablesSuccess(t, res)
	if manager.lastConfig.Host != "203.0.113.10" {
		t.Fatalf("expected SSH config to be passed to manager, got %+v", manager.lastConfig)
	}
}

func TestNodeNftablesReconcileEndpointPersistsBindings(t *testing.T) {
	fixture := setupNftablesHandler(t)
	seedNftablesSSHConfig(t, fixture.handler, fixture.nodeID)
	tunnelID := seedTunnelForNftables(t, fixture.handler, "nft-tunnel", fixture.nodeID)
	forward := seedForwardForNftables(t, fixture.handler, tunnelID, fixture.nodeID, "203.0.113.9:8080")
	manager := &fakeNftablesManager{}
	fixture.handler.nftablesManager = manager

	res := postJSONToHandler(t, fixture.handler.nodeNftablesReconcile, map[string]int64{"nodeId": fixture.nodeID})
	assertNftablesSuccess(t, res)
	if manager.reconcileHit != 1 {
		t.Fatalf("expected reconcile once, got %d", manager.reconcileHit)
	}
	bindings, err := fixture.handler.repo.ListNftRuleBindingsByNode(fixture.nodeID)
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].ForwardID != forward.ID {
		t.Fatalf("unexpected bindings: %+v", bindings)
	}
}

func TestNodeNftablesClearEndpointClearsBindings(t *testing.T) {
	fixture := setupNftablesHandler(t)
	seedNftablesSSHConfig(t, fixture.handler, fixture.nodeID)
	now := time.Now().UnixMilli()
	if err := fixture.handler.repo.UpsertNftRuleBinding(repo.NftRuleBindingInput{
		ForwardID:  99,
		NodeID:     fixture.nodeID,
		InPort:     24000,
		Protocols:  "tcp",
		TargetAddr: "203.0.113.9:8080",
		Status:     runtimenft.StatusApplied,
	}, now); err != nil {
		t.Fatalf("seed binding: %v", err)
	}
	manager := &fakeNftablesManager{}
	fixture.handler.nftablesManager = manager

	res := postJSONToHandler(t, fixture.handler.nodeNftablesClear, map[string]int64{"nodeId": fixture.nodeID})
	assertNftablesSuccess(t, res)
	if manager.clearHit != 1 {
		t.Fatalf("expected clear once, got %d", manager.clearHit)
	}
	if bindings, err := fixture.handler.repo.ListNftRuleBindingsByNode(fixture.nodeID); err != nil {
		t.Fatalf("list bindings after clear: %v", err)
	} else if len(bindings) != 0 {
		t.Fatalf("expected bindings to be cleared, got %+v", bindings)
	}
}

func TestNodeCreatePersistsNftablesSSHConfig(t *testing.T) {
	fixture := setupNftablesHandler(t)
	req := newAuthenticatedJSONRequest(t, map[string]interface{}{
		"name":        "nft-node-created",
		"serverIp":    "203.0.113.20",
		"serverIpV4":  "203.0.113.20",
		"port":        "20000-20100",
		"forwardMode": "nftables",
		"sshConfig": map[string]interface{}{
			"host":       "203.0.113.21",
			"port":       2222,
			"username":   "root",
			"authType":   "private_key",
			"privateKey": "TEST-PRIVATE-KEY",
			"passphrase": "secret",
			"sudoMode":   "sudo",
		},
	})
	res := httptest.NewRecorder()
	fixture.handler.nodeCreate(res, req)
	assertNftablesSuccessWithBody(t, res)

	nodes, err := fixture.handler.repo.ListNodes()
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	var createdNodeID int64
	for _, item := range nodes {
		if item["name"] == "nft-node-created" {
			createdNodeID = item["id"].(int64)
			break
		}
	}
	if createdNodeID <= 0 {
		t.Fatalf("expected created node to exist")
	}
	createdNode, err := fixture.handler.repo.GetNodeRecord(createdNodeID)
	if err != nil {
		t.Fatalf("load created node: %v", err)
	}
	if createdNode == nil {
		t.Fatal("expected created node record, got nil")
	}
	if createdNode.Status != 1 {
		t.Fatalf("expected nftables node to be online, got status %d", createdNode.Status)
	}
	cfg, err := fixture.handler.repo.GetNodeSSHConfig(createdNodeID)
	if err != nil {
		t.Fatalf("load ssh config: %v", err)
	}
	if cfg.Host != "203.0.113.21" || cfg.Port != 2222 || cfg.Username != "root" || cfg.AuthType != "private_key" {
		t.Fatalf("unexpected ssh config: %+v", cfg)
	}
	if !cfg.PrivateKey.Valid || cfg.PrivateKey.String != "TEST-PRIVATE-KEY" {
		t.Fatalf("expected private key to persist, got %+v", cfg)
	}
}

func TestNodeUpdatePreservesExistingNftablesSecretsWhenFieldsOmitted(t *testing.T) {
	fixture := setupNftablesHandler(t)
	seedNftablesSSHConfig(t, fixture.handler, fixture.nodeID)

	req := newAuthenticatedJSONRequest(t, map[string]interface{}{
		"id":          fixture.nodeID,
		"name":        "nft-node-updated",
		"serverIp":    "198.51.100.10",
		"serverIpV4":  "198.51.100.10",
		"port":        "1000-65535",
		"forwardMode": "nftables",
		"sshConfig": map[string]interface{}{
			"host":     "203.0.113.30",
			"port":     22,
			"username": "admin",
			"authType": "password",
			"sudoMode": "none",
		},
	})
	res := httptest.NewRecorder()
	fixture.handler.nodeUpdate(res, req)
	assertNftablesSuccessWithBody(t, res)

	cfg, err := fixture.handler.repo.GetNodeSSHConfig(fixture.nodeID)
	if err != nil {
		t.Fatalf("load ssh config: %v", err)
	}
	if cfg.Host != "203.0.113.30" || cfg.Username != "admin" || cfg.AuthType != "password" {
		t.Fatalf("unexpected ssh config after update: %+v", cfg)
	}
	if !cfg.Password.Valid || cfg.Password.String != "secret" {
		t.Fatalf("expected password secret to be preserved, got %+v", cfg)
	}
}

func TestValidateNftablesForwardRequestRejectsHostnameTarget(t *testing.T) {
	fixture := setupNftablesHandler(t)
	h := fixture.handler
	tunnelID := seedTunnelForNftables(t, h, "nft-tunnel", fixture.nodeID)
	tunnel, err := h.getTunnelRecord(tunnelID)
	if err != nil {
		t.Fatalf("load tunnel: %v", err)
	}

	err = h.validateNftablesForwardRequest(tunnel, "example.com:443", []int64{fixture.nodeID})
	if err == nil {
		t.Fatalf("expected hostname target to be rejected")
	}
	if !strings.Contains(err.Error(), "IP") {
		t.Fatalf("expected IP literal validation error, got %q", err)
	}
}

func TestForwardDeleteReconcilesNftablesAfterDBDelete(t *testing.T) {
	fixture := setupNftablesHandler(t)
	h := fixture.handler
	seedNftablesSSHConfig(t, h, fixture.nodeID)
	tunnelID := seedTunnelForNftables(t, h, "nft-tunnel", fixture.nodeID)
	forward := seedForwardForNftables(t, h, tunnelID, fixture.nodeID, "203.0.113.9:8080")
	manager := &fakeNftablesManager{}
	h.nftablesManager = manager

	req := newAuthenticatedJSONRequest(t, map[string]int64{"id": forward.ID})
	res := httptest.NewRecorder()
	h.forwardDelete(res, req)
	assertNftablesSuccessWithBody(t, res)
	if manager.reconcileHit != 1 {
		t.Fatalf("expected reconcile once, got %d", manager.reconcileHit)
	}
	if len(manager.lastPlan.Rules) != 0 {
		t.Fatalf("expected reconcile after DB delete to render no rules, got %+v", manager.lastPlan.Rules)
	}
	if _, err := h.getForwardRecord(forward.ID); !errors.Is(err, errForwardNotFound) {
		t.Fatalf("expected forward to be deleted, got %v", err)
	}
}

func TestForwardForceDeleteRemovesNftablesBindingAndReconciles(t *testing.T) {
	fixture := setupNftablesHandler(t)
	h := fixture.handler
	seedNftablesSSHConfig(t, h, fixture.nodeID)
	tunnelID := seedTunnelForNftables(t, h, "nft-tunnel", fixture.nodeID)
	forward := seedForwardForNftables(t, h, tunnelID, fixture.nodeID, "203.0.113.9:8080")
	if err := h.repo.UpsertNftRuleBinding(repo.NftRuleBindingInput{
		ForwardID:  forward.ID,
		NodeID:     fixture.nodeID,
		InPort:     20000,
		Protocols:  "tcp,udp",
		TargetAddr: "203.0.113.9:8080",
		Status:     runtimenft.StatusApplied,
	}, time.Now().UnixMilli()); err != nil {
		t.Fatalf("seed binding: %v", err)
	}
	manager := &fakeNftablesManager{}
	h.nftablesManager = manager

	req := newAuthenticatedJSONRequest(t, map[string]int64{"id": forward.ID})
	req.URL.Path = "/api/v1/forward/force-delete"
	res := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.Register(mux)
	mux.ServeHTTP(res, req)
	assertNftablesSuccessWithBody(t, res)
	if manager.reconcileHit != 1 {
		t.Fatalf("expected reconcile once, got %d", manager.reconcileHit)
	}
	if _, err := h.getForwardRecord(forward.ID); !errors.Is(err, errForwardNotFound) {
		t.Fatalf("expected forward to be deleted, got %v", err)
	}
	if bindings, err := h.repo.ListNftRuleBindingsByNode(fixture.nodeID); err != nil {
		t.Fatalf("list bindings after delete: %v", err)
	} else if len(bindings) != 0 {
		t.Fatalf("expected no bindings after delete, got %+v", bindings)
	}
}

func TestForwardBatchDeleteReconcilesNftablesAfterDBDelete(t *testing.T) {
	fixture := setupNftablesHandler(t)
	h := fixture.handler
	seedNftablesSSHConfig(t, h, fixture.nodeID)
	tunnelID := seedTunnelForNftables(t, h, "nft-tunnel", fixture.nodeID)
	forward := seedForwardForNftables(t, h, tunnelID, fixture.nodeID, "203.0.113.9:8080")
	manager := &fakeNftablesManager{}
	h.nftablesManager = manager

	req := newAuthenticatedJSONRequest(t, map[string][]int64{"ids": {forward.ID}})
	res := httptest.NewRecorder()
	h.forwardBatchDelete(res, req)
	assertNftablesSuccessWithBody(t, res)
	if manager.reconcileHit != 1 {
		t.Fatalf("expected reconcile once, got %d", manager.reconcileHit)
	}
	if len(manager.lastPlan.Rules) != 0 {
		t.Fatalf("expected reconcile after DB delete to render no rules, got %+v", manager.lastPlan.Rules)
	}
	if _, err := h.getForwardRecord(forward.ID); !errors.Is(err, errForwardNotFound) {
		t.Fatalf("expected forward to be deleted, got %v", err)
	}
}

func TestForwardBatchRedeployUsesNftablesReconcile(t *testing.T) {
	fixture := setupNftablesHandler(t)
	h := fixture.handler
	seedNftablesSSHConfig(t, h, fixture.nodeID)
	tunnelID := seedTunnelForNftables(t, h, "nft-tunnel", fixture.nodeID)
	forward := seedForwardForNftables(t, h, tunnelID, fixture.nodeID, "203.0.113.9:8080")
	manager := &fakeNftablesManager{}
	h.nftablesManager = manager

	req := newAuthenticatedJSONRequest(t, map[string][]int64{"ids": {forward.ID}})
	res := httptest.NewRecorder()
	h.forwardBatchRedeploy(res, req)
	assertNftablesSuccessWithBody(t, res)
	if manager.reconcileHit != 1 {
		t.Fatalf("expected reconcile once, got %d", manager.reconcileHit)
	}
}

func TestTunnelBatchRedeployUsesNftablesReconcile(t *testing.T) {
	fixture := setupNftablesHandler(t)
	h := fixture.handler
	seedNftablesSSHConfig(t, h, fixture.nodeID)
	tunnelID := seedTunnelForNftables(t, h, "nft-tunnel", fixture.nodeID)
	seedForwardForNftables(t, h, tunnelID, fixture.nodeID, "203.0.113.9:8080")
	manager := &fakeNftablesManager{}
	h.nftablesManager = manager

	req := newAuthenticatedJSONRequest(t, map[string][]int64{"ids": {tunnelID}})
	res := httptest.NewRecorder()
	h.tunnelBatchRedeploy(res, req)
	assertNftablesSuccessWithBody(t, res)
	if manager.reconcileHit != 1 {
		t.Fatalf("expected reconcile once, got %d", manager.reconcileHit)
	}
}

func setupNftablesHandler(t *testing.T) nftablesTestFixture {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "handler-nftables.sqlite")
	r, err := repo.Open(dbPath)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}

	h := New(r, "test-secret")
	now := time.Now().UnixMilli()
	if _, err := r.CreateUser("admin", "hash", 0, now+86400000, 1, 1, 100, 1, 0, now); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := r.CreateNode("nft-node", "secret", "198.51.100.10", nil, nil, "1000-65535", nil, nil, nil, nil, nil, 0, 0, 0, now, 1, "", "", 1, 0, nil, nil, nil, nil, "nftables"); err != nil {
		t.Fatalf("create node: %v", err)
	}
	node, err := r.GetNodeRecord(1)
	if err != nil || node == nil {
		t.Fatalf("get node: %v", err)
	}
	return nftablesTestFixture{handler: h, nodeID: node.ID}
}

func seedNftablesSSHConfig(t *testing.T, h *Handler, nodeID int64) {
	t.Helper()
	if err := h.repo.UpsertNodeSSHConfig(nodeID, repo.NftSSHConfigInput{
		Host:     "203.0.113.10",
		Port:     22,
		Username: "root",
		AuthType: "password",
		Password: "secret",
		SudoMode: "none",
	}, time.Now().UnixMilli()); err != nil {
		t.Fatalf("upsert ssh config: %v", err)
	}
}

func seedTunnelForNftables(t *testing.T, h *Handler, name string, nodeID int64) int64 {
	t.Helper()
	now := time.Now().UnixMilli()
	tx := h.repo.BeginTx()
	if tx == nil {
		t.Fatal("begin tx: nil transaction")
	}
	if tx.Error != nil {
		t.Fatalf("begin tx: %v", tx.Error)
	}
	tunnelID, err := h.repo.CreateTunnelTx(tx, name, 1, 1, 1, now, 1, nil, 1, "", "", 0)
	if err != nil {
		_ = tx.Rollback().Error
		t.Fatalf("create tunnel: %v", err)
	}
	if err := h.repo.CreateChainTunnelTx(tx, tunnelID, "1", nodeID, sql.NullInt64{}, "", 1, "tls", ""); err != nil {
		_ = tx.Rollback().Error
		t.Fatalf("create chain tunnel: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		_ = tx.Rollback().Error
		t.Fatalf("commit tx: %v", err)
	}
	return tunnelID
}

func seedForwardForNftables(t *testing.T, h *Handler, tunnelID, nodeID int64, remoteAddr string) *forwardRecord {
	t.Helper()
	now := time.Now().UnixMilli()
	forwardID, err := h.repo.CreateForwardTx(
		1, "admin", "nft-forward", tunnelID, remoteAddr, "fifo", now, 1,
		[]int64{nodeID}, 20000, "", nil, 0, 0, nil, 0,
	)
	if err != nil {
		t.Fatalf("create forward: %v", err)
	}
	forward, err := h.getForwardRecord(forwardID)
	if err != nil {
		t.Fatalf("get forward: %v", err)
	}
	return forward
}

func postJSONToHandler(t *testing.T, fn func(http.ResponseWriter, *http.Request), payload any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	res := httptest.NewRecorder()
	fn(res, req)
	return res
}

func newAuthenticatedJSONRequest(t *testing.T, payload any) *http.Request {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	token, err := auth.GenerateToken(1, "admin", 0, "test-secret")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	req.Header.Set("Authorization", token)
	claims, ok := auth.ValidateToken(token, "test-secret")
	if !ok {
		t.Fatalf("validate token failed")
	}
	return req.WithContext(context.WithValue(req.Context(), middleware.ClaimsContextKey, claims))
}

func assertNftablesSuccess(t *testing.T, res *httptest.ResponseRecorder) {
	t.Helper()
	assertNftablesSuccessWithBody(t, res)
}

func assertNftablesSuccessWithBody(t *testing.T, res *httptest.ResponseRecorder) {
	t.Helper()
	var payload struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if res.Code != http.StatusOK {
		t.Fatalf("expected HTTP %d, got %d", http.StatusOK, res.Code)
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != 0 {
		t.Fatalf("expected success, got %+v", payload)
	}
}
