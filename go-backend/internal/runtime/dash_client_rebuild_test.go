package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"go-backend/internal/dashruntime"
	httpclient "go-backend/internal/http/client"
	"go-backend/internal/store/repo"
)

func TestDashRuntimeClientRebuildAllRuntimeUpsertsForwardRules(t *testing.T) {
	r, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	const now int64 = 123
	if err := r.CreateNode("entry", "entry-secret", "198.51.100.10", nil, nil, "8080-8085", nil, nil, nil, nil, nil, 1, 1, 1, now, 1, "0.0.0.0", "0.0.0.0", 1, 0, nil, nil, nil, nil); err != nil {
		t.Fatalf("create entry node: %v", err)
	}

	var entryNodeID int64
	if err := r.DB().Raw("SELECT id FROM node WHERE name = ?", "entry").Scan(&entryNodeID).Error; err != nil {
		t.Fatalf("query entry node id: %v", err)
	}

	var tunnelID int64
	tx := r.DB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx: %v", tx.Error)
	}
	tunnelID, err = r.CreateTunnelTx(tx, "ssh", 1, 1, 0, now, 1, nil, 1, "")
	if err != nil {
		tx.Rollback()
		t.Fatalf("create tunnel: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit tunnel: %v", err)
	}

	forwardID, err := r.CreateForwardTx(7, "alice", "web", tunnelID, "10.0.0.10:80", "round", now, 1, []int64{entryNodeID}, 8080, "", nil)
	if err != nil {
		t.Fatalf("create forward: %v", err)
	}

	fake := &fakeDashRuntimeAPI{status: httpclient.DashStatusResponse{RuleCount: 0, ExitState: httpclient.DashExitState{Active: &httpclient.DashActiveBackend{Server: "198.51.100.50:18080", Token: "active-exit-token"}}}}
	client := NewDashRuntimeClient(r, fake)

	progress, err := client.RebuildAllRuntime(context.Background())
	if err != nil {
		t.Fatalf("RebuildAllRuntime: %v", err)
	}
	if progress.Engine != EngineDash || progress.State != ProgressStateSucceeded || !progress.Complete {
		t.Fatalf("unexpected progress: %+v", progress)
	}
	if len(fake.upserts) != 2 {
		t.Fatalf("expected 2 upserts, got %d", len(fake.upserts))
	}
	call := fake.upserts[0]
	if call.node.ServerIP != "198.51.100.10" || call.node.Secret != "entry-secret" {
		t.Fatalf("unexpected target node: %+v", call.node)
	}
	if call.rule.ID != "forward-"+int64String(forwardID)+"-node-"+int64String(entryNodeID)+"-port-8080-tcp" {
		t.Fatalf("unexpected rule id: %s", call.rule.ID)
	}
	if call.rule.Listen != "0.0.0.0:8080" {
		t.Fatalf("unexpected listen address: %s", call.rule.Listen)
	}
	if len(call.rule.TargetPool.Backends) != 1 || call.rule.TargetPool.Backends[0].Address != "10.0.0.10:80" {
		t.Fatalf("unexpected target backends: %+v", call.rule.TargetPool.Backends)
	}
	if len(call.rule.StagePools) != 1 || len(call.rule.StagePools[0].Backends) != 1 {
		t.Fatalf("expected active-exit stage pool, got %+v", call.rule.StagePools)
	}
	if call.rule.StagePools[0].Backends[0].Server != "198.51.100.50:18080" || call.rule.StagePools[0].Backends[0].Token != "active-exit-token" {
		t.Fatalf("expected active exit backend, got %+v", call.rule.StagePools[0].Backends[0])
	}
	if call.rule.StagePools[0].Backends[0].Token == "runtime-token-required" {
		t.Fatalf("expected real active exit token, got synthetic fallback: %+v", call.rule.StagePools[0].Backends[0])
	}
}

func TestDashRuntimeClientRebuildAllRuntimeUsesTunnelEntryPortForListen(t *testing.T) {
	r, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	entryNodeID := createNodeForDashRebuildTest(t, r, "entry-tunnel", "entry-secret", "198.51.100.20")
	chainNodeID := createNodeForDashRebuildTest(t, r, "chain-tunnel", "chain-secret", "198.51.100.21")
	outNodeID := createNodeForDashRebuildTest(t, r, "out-tunnel", "out-secret", "198.51.100.22")

	tunnelID := createTunnelForDashRebuildTest(t, r, "tcp-tunnel", 2)
	addChainHopForDashRebuildTest(t, r, tunnelID, "1", entryNodeID, 2201, 0)
	addChainHopForDashRebuildTest(t, r, tunnelID, "2", chainNodeID, 3301, 1)
	addChainHopForDashRebuildTest(t, r, tunnelID, "3", outNodeID, 4401, 0)

	fake := &fakeDashRuntimeAPI{}
	client := NewDashRuntimeClient(r, fake)

	progress, err := client.RebuildAllRuntime(context.Background())
	if err != nil {
		t.Fatalf("RebuildAllRuntime: %v", err)
	}
	if progress.State != ProgressStateSucceeded {
		t.Fatalf("unexpected progress: %+v", progress)
	}
	if len(fake.upserts) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(fake.upserts))
	}
	call := fake.upserts[0]
	if call.rule.ID != "tunnel-"+int64String(tunnelID) {
		t.Fatalf("unexpected rule id: %s", call.rule.ID)
	}
	if call.rule.Listen != "0.0.0.0:2201" {
		t.Fatalf("expected tunnel listen to use entry port, got %s", call.rule.Listen)
	}
	if len(call.rule.TargetPool.Backends) != 1 || call.rule.TargetPool.Backends[0].Address != "198.51.100.22:4401" {
		t.Fatalf("unexpected tunnel target backends: %+v", call.rule.TargetPool.Backends)
	}
}

func TestDashRuntimeClientRebuildAllRuntimeSkipsUnresolvableTunnelAndContinues(t *testing.T) {
	r, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	validEntryNodeID := createNodeForDashRebuildTest(t, r, "valid-entry", "valid-entry-secret", "198.51.100.30")
	validOutNodeID := createNodeForDashRebuildTest(t, r, "valid-out", "valid-out-secret", "198.51.100.31")

	staleTunnelID := createTunnelForDashRebuildTest(t, r, "stale-tunnel", 2)
	addChainHopForDashRebuildTest(t, r, staleTunnelID, "1", 999001, 5501, 0)
	addChainHopForDashRebuildTest(t, r, staleTunnelID, "3", validOutNodeID, 6601, 0)

	validTunnelID := createTunnelForDashRebuildTest(t, r, "valid-tunnel", 2)
	addChainHopForDashRebuildTest(t, r, validTunnelID, "1", validEntryNodeID, 2202, 0)
	addChainHopForDashRebuildTest(t, r, validTunnelID, "3", validOutNodeID, 4402, 0)

	fake := &fakeDashRuntimeAPI{}
	client := NewDashRuntimeClient(r, fake)

	progress, err := client.RebuildAllRuntime(context.Background())
	if err != nil {
		t.Fatalf("RebuildAllRuntime: %v", err)
	}
	if progress.State != ProgressStateSucceeded || !progress.Complete {
		t.Fatalf("unexpected progress: %+v", progress)
	}
	if len(fake.upserts) != 1 {
		t.Fatalf("expected valid tunnel to still upsert, got %d upserts", len(fake.upserts))
	}
	if fake.upserts[0].rule.ID != "tunnel-"+int64String(validTunnelID) {
		t.Fatalf("unexpected rebuilt rule: %+v", fake.upserts[0].rule)
	}
	if progress.Message == "" {
		t.Fatal("expected skip details in rebuild progress message")
	}
	if len(progress.Warnings) == 0 {
		t.Fatal("expected warnings for skipped tunnels")
	}
	if !containsAll(progress.Message, []string{"skipped", int64String(staleTunnelID)}) {
		t.Fatalf("expected skipped tunnel details in message, got %q", progress.Message)
	}
	if !containsAll(strings.Join(progress.Warnings, "; "), []string{int64String(staleTunnelID), "stale-tunnel"}) {
		t.Fatalf("expected stale tunnel details in warnings, got %#v", progress.Warnings)
	}
}

type fakeDashRuntimeAPI struct {
	upserts        []fakeDashUpsert
	upsertAttempts []fakeDashUpsert
	upsertErrs     map[string]error
	status         httpclient.DashStatusResponse
}

type fakeDashUpsert struct {
	node httpclient.DashRuntimeNode
	rule dashruntime.RelayRulePayload
}

func (f *fakeDashRuntimeAPI) UpsertRule(_ context.Context, node httpclient.DashRuntimeNode, rule dashruntime.RelayRulePayload) error {
	f.upsertAttempts = append(f.upsertAttempts, fakeDashUpsert{node: node, rule: rule})
	if err, ok := f.upsertErrs[rule.ID]; ok {
		return err
	}
	f.upserts = append(f.upserts, fakeDashUpsert{node: node, rule: rule})
	return nil
}

func (f *fakeDashRuntimeAPI) DeleteRule(context.Context, httpclient.DashRuntimeNode, string) error {
	return nil
}

func (f *fakeDashRuntimeAPI) PauseServices(context.Context, httpclient.DashRuntimeNode, []string) error {
	return nil
}

func (f *fakeDashRuntimeAPI) ResumeServices(context.Context, httpclient.DashRuntimeNode, []string) error {
	return nil
}

func (f *fakeDashRuntimeAPI) CheckService(context.Context, httpclient.DashRuntimeNode, httpclient.DashServiceCheckRequest) (httpclient.DashServiceCheckResponse, error) {
	return httpclient.DashServiceCheckResponse{}, nil
}

func (f *fakeDashRuntimeAPI) GetStatus(context.Context, httpclient.DashRuntimeNode) (httpclient.DashStatusResponse, error) {
	return f.status, nil
}

func TestDashRuntimeClientGetNodeRuntimeStatusUsesStatusApi(t *testing.T) {
	client := NewDashRuntimeClient(nil, &fakeDashRuntimeAPI{
		status: httpclient.DashStatusResponse{RuleCount: 0, ExitState: httpclient.DashExitState{Active: &httpclient.DashActiveBackend{Server: "127.0.0.1:18080", Token: "relay-secret"}}},
	})

	status, err := client.GetNodeRuntimeStatus(context.Background(), repo.Node{ID: 7, ServerIP: "127.0.0.1", Secret: "node-secret"})
	if err != nil {
		t.Fatalf("GetNodeRuntimeStatus: %v", err)
	}
	if !status.Ready {
		t.Fatalf("expected dash node to be ready when status api is reachable: %+v", status)
	}
	if status.Progress != ProgressStateSucceeded {
		t.Fatalf("expected succeeded progress, got %+v", status)
	}
}

func TestDashRuntimeClientGetNodeRuntimeStatusRequiresActiveExitToken(t *testing.T) {
	client := NewDashRuntimeClient(nil, &fakeDashRuntimeAPI{
		status: httpclient.DashStatusResponse{RuleCount: 0, ExitState: httpclient.DashExitState{Active: &httpclient.DashActiveBackend{Server: "127.0.0.1:18080", Token: ""}}},
	})

	status, err := client.GetNodeRuntimeStatus(context.Background(), repo.Node{ID: 7, ServerIP: "127.0.0.1", Secret: "node-secret"})
	if err != nil {
		t.Fatalf("GetNodeRuntimeStatus: %v", err)
	}
	if status.Ready {
		t.Fatalf("expected dash node without active-exit token to stay not ready: %+v", status)
	}
	if status.Progress != ProgressStatePending {
		t.Fatalf("expected pending progress, got %+v", status)
	}
}

func TestDashRuntimeClientApplyForwardsUsesEntryNodeActiveExitForType1Tunnel(t *testing.T) {
	r, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	const now int64 = 123
	if err := r.CreateNode("entry", "entry-secret", "198.51.100.10", nil, nil, "8080-8085", nil, nil, nil, nil, nil, 1, 1, 1, now, 1, "0.0.0.0", "0.0.0.0", 1, 0, nil, nil, nil, nil); err != nil {
		t.Fatalf("create entry node: %v", err)
	}

	var entryNodeID int64
	if err := r.DB().Raw("SELECT id FROM node WHERE name = ?", "entry").Scan(&entryNodeID).Error; err != nil {
		t.Fatalf("query entry node id: %v", err)
	}

	tx := r.DB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx: %v", tx.Error)
	}
	tunnelID, err := r.CreateTunnelTx(tx, "ssh", 1, 1, 0, now, 1, nil, 1, "")
	if err != nil {
		tx.Rollback()
		t.Fatalf("create tunnel: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit tunnel: %v", err)
	}

	if _, err := r.CreateForwardTx(7, "alice", "web", tunnelID, "10.0.0.10:80", "round", now, 1, []int64{entryNodeID}, 8080, "", nil); err != nil {
		t.Fatalf("create forward: %v", err)
	}

	fake := &fakeDashRuntimeAPI{status: httpclient.DashStatusResponse{RuleCount: 0, ExitState: httpclient.DashExitState{Active: &httpclient.DashActiveBackend{Server: "198.51.100.50:18080", Token: "active-exit-token"}}}}
	client := NewDashRuntimeClient(r, fake)

	if err := client.ApplyForwards(context.Background(), tunnelID); err != nil {
		t.Fatalf("ApplyForwards: %v", err)
	}
	if len(fake.upserts) != 2 {
		t.Fatalf("expected 2 upserts, got %d", len(fake.upserts))
	}
	backend := fake.upserts[0].rule.StagePools[0].Backends[0]
	if backend.Server != "198.51.100.50:18080" || backend.Token != "active-exit-token" {
		t.Fatalf("expected active exit backend, got %+v", backend)
	}
}

func TestDashRuntimeClientApplyForwardsFailsWhenType1TunnelActiveExitMissing(t *testing.T) {
	r, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	const now int64 = 123
	if err := r.CreateNode("entry", "entry-secret", "198.51.100.10", nil, nil, "8080-8085", nil, nil, nil, nil, nil, 1, 1, 1, now, 1, "0.0.0.0", "0.0.0.0", 1, 0, nil, nil, nil, nil); err != nil {
		t.Fatalf("create entry node: %v", err)
	}

	var entryNodeID int64
	if err := r.DB().Raw("SELECT id FROM node WHERE name = ?", "entry").Scan(&entryNodeID).Error; err != nil {
		t.Fatalf("query entry node id: %v", err)
	}

	tx := r.DB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx: %v", tx.Error)
	}
	tunnelID, err := r.CreateTunnelTx(tx, "ssh", 1, 1, 0, now, 1, nil, 1, "")
	if err != nil {
		tx.Rollback()
		t.Fatalf("create tunnel: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit tunnel: %v", err)
	}

	if _, err := r.CreateForwardTx(7, "alice", "web", tunnelID, "10.0.0.10:80", "round", now, 1, []int64{entryNodeID}, 8080, "", nil); err != nil {
		t.Fatalf("create forward: %v", err)
	}

	client := NewDashRuntimeClient(r, &fakeDashRuntimeAPI{})

	err = client.ApplyForwards(context.Background(), tunnelID)
	if err == nil {
		t.Fatal("expected missing active exit to fail")
	}
	if !strings.Contains(err.Error(), "entry node 1 missing active exit for dash forward") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDashRuntimeClientApplyForwardsCreatesTcpAndUdpRules(t *testing.T) {
	r, tunnelID, forwardID, entryNodeID := setupType1DashForwardTest(t)
	defer r.Close()

	fake := &fakeDashRuntimeAPI{status: httpclient.DashStatusResponse{RuleCount: 0, ExitState: httpclient.DashExitState{Active: &httpclient.DashActiveBackend{Server: "198.51.100.50:18080", Token: "active-exit-token"}}}}
	client := NewDashRuntimeClient(r, fake)

	results, err := client.ApplyForwardsDetailed(context.Background(), tunnelID)
	if err != nil {
		t.Fatalf("ApplyForwardsDetailed: %v", err)
	}
	if len(fake.upsertAttempts) != 2 {
		t.Fatalf("expected 2 upsert attempts, got %d", len(fake.upsertAttempts))
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 logical forward result, got %d", len(results))
	}
	result := results[0]
	if result.Status != ForwardApplyStatusSuccess {
		t.Fatalf("expected success result, got %+v", result)
	}
	if len(result.Protocols) != 2 {
		t.Fatalf("expected 2 protocol results, got %+v", result.Protocols)
	}
	assertForwardProtocolResult(t, result.Protocols[0], "tcp", "forward-"+int64String(forwardID)+"-node-"+int64String(entryNodeID)+"-port-8080-tcp", ForwardApplyStatusSuccess)
	assertForwardProtocolResult(t, result.Protocols[1], "udp", "forward-"+int64String(forwardID)+"-node-"+int64String(entryNodeID)+"-port-8080-udp", ForwardApplyStatusSuccess)
}

func TestDashRuntimeClientApplyForwardsReturnsPartialSuccessWhenUdpFails(t *testing.T) {
	r, tunnelID, forwardID, entryNodeID := setupType1DashForwardTest(t)
	defer r.Close()

	udpRuleID := "forward-" + int64String(forwardID) + "-node-" + int64String(entryNodeID) + "-port-8080-udp"
	fake := &fakeDashRuntimeAPI{
		status:     httpclient.DashStatusResponse{RuleCount: 0, ExitState: httpclient.DashExitState{Active: &httpclient.DashActiveBackend{Server: "198.51.100.50:18080", Token: "active-exit-token"}}},
		upsertErrs: map[string]error{udpRuleID: fmt.Errorf("udp apply failed")},
	}
	client := NewDashRuntimeClient(r, fake)

	results, err := client.ApplyForwardsDetailed(context.Background(), tunnelID)
	if err != nil {
		t.Fatalf("ApplyForwardsDetailed: %v", err)
	}
	if len(fake.upsertAttempts) != 2 {
		t.Fatalf("expected 2 upsert attempts, got %d", len(fake.upsertAttempts))
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 logical forward result, got %d", len(results))
	}
	result := results[0]
	if result.Status != ForwardApplyStatusPartialSuccess {
		t.Fatalf("expected partial success result, got %+v", result)
	}
	assertForwardProtocolResult(t, result.Protocols[0], "tcp", "forward-"+int64String(forwardID)+"-node-"+int64String(entryNodeID)+"-port-8080-tcp", ForwardApplyStatusSuccess)
	assertForwardProtocolResult(t, result.Protocols[1], "udp", udpRuleID, ForwardApplyStatusFailed)
	if result.Protocols[1].Message == "" {
		t.Fatalf("expected udp failure message, got %+v", result.Protocols[1])
	}
}

func TestDashRuntimeClientApplyForwardsReturnsErrorWhenUdpFails(t *testing.T) {
	r, tunnelID, forwardID, entryNodeID := setupType1DashForwardTest(t)
	defer r.Close()

	udpRuleID := "forward-" + int64String(forwardID) + "-node-" + int64String(entryNodeID) + "-port-8080-udp"
	fake := &fakeDashRuntimeAPI{
		status:     httpclient.DashStatusResponse{RuleCount: 0, ExitState: httpclient.DashExitState{Active: &httpclient.DashActiveBackend{Server: "198.51.100.50:18080", Token: "active-exit-token"}}},
		upsertErrs: map[string]error{udpRuleID: fmt.Errorf("udp apply failed")},
	}
	client := NewDashRuntimeClient(r, fake)

	err := client.ApplyForwards(context.Background(), tunnelID)
	if err == nil {
		t.Fatal("expected partial success to return an error")
	}
	result, ok := err.(ForwardApplyResult)
	if !ok {
		t.Fatalf("expected ForwardApplyResult error, got %T", err)
	}
	if result.Status != ForwardApplyStatusPartialSuccess {
		t.Fatalf("expected partial success error, got %+v", result)
	}
	if len(result.Protocols) != 2 {
		t.Fatalf("expected protocol details in partial success error, got %+v", result)
	}
	assertForwardProtocolResult(t, result.Protocols[0], "tcp", "forward-"+int64String(forwardID)+"-node-"+int64String(entryNodeID)+"-port-8080-tcp", ForwardApplyStatusSuccess)
	assertForwardProtocolResult(t, result.Protocols[1], "udp", udpRuleID, ForwardApplyStatusFailed)
	if len(result.Warnings) == 0 {
		t.Fatalf("expected warning details in partial success error, got %+v", result)
	}
}

func TestDashRuntimeClientApplyForwardsFailsWhenTCPFails(t *testing.T) {
	r, tunnelID, forwardID, entryNodeID := setupType1DashForwardTest(t)
	defer r.Close()

	tcpRuleID := "forward-" + int64String(forwardID) + "-node-" + int64String(entryNodeID) + "-port-8080-tcp"
	fake := &fakeDashRuntimeAPI{
		status:     httpclient.DashStatusResponse{RuleCount: 0, ExitState: httpclient.DashExitState{Active: &httpclient.DashActiveBackend{Server: "198.51.100.50:18080", Token: "active-exit-token"}}},
		upsertErrs: map[string]error{tcpRuleID: fmt.Errorf("tcp apply failed")},
	}
	client := NewDashRuntimeClient(r, fake)

	results, err := client.ApplyForwardsDetailed(context.Background(), tunnelID)
	if err == nil {
		t.Fatal("expected tcp failure to return error")
	}
	if len(fake.upsertAttempts) != 2 {
		t.Fatalf("expected 2 upsert attempts, got %d", len(fake.upsertAttempts))
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 logical forward result, got %d", len(results))
	}
	result := results[0]
	if result.Status != ForwardApplyStatusFailed {
		t.Fatalf("expected failed result, got %+v", result)
	}
	assertForwardProtocolResult(t, result.Protocols[0], "tcp", tcpRuleID, ForwardApplyStatusFailed)
	assertForwardProtocolResult(t, result.Protocols[1], "udp", "forward-"+int64String(forwardID)+"-node-"+int64String(entryNodeID)+"-port-8080-udp", ForwardApplyStatusSuccess)
}

func int64String(v int64) string {
	return fmt.Sprintf("%d", v)
}

func createNodeForDashRebuildTest(t *testing.T, r *repo.Repository, name, secret, serverIP string) int64 {
	t.Helper()
	const now int64 = 123
	if err := r.CreateNode(name, secret, serverIP, nil, nil, "2000-9000", nil, nil, nil, nil, nil, 1, 1, 1, now, 1, "0.0.0.0", "0.0.0.0", 1, 0, nil, nil, nil, nil); err != nil {
		t.Fatalf("create node %s: %v", name, err)
	}
	var nodeID int64
	if err := r.DB().Raw("SELECT id FROM node WHERE name = ?", name).Scan(&nodeID).Error; err != nil {
		t.Fatalf("query node id %s: %v", name, err)
	}
	return nodeID
}

func setupType1DashForwardTest(t *testing.T) (*repo.Repository, int64, int64, int64) {
	t.Helper()

	r, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}

	const now int64 = 123
	if err := r.CreateNode("entry", "entry-secret", "198.51.100.10", nil, nil, "8080-8085", nil, nil, nil, nil, nil, 1, 1, 1, now, 1, "0.0.0.0", "0.0.0.0", 1, 0, nil, nil, nil, nil); err != nil {
		r.Close()
		t.Fatalf("create entry node: %v", err)
	}

	var entryNodeID int64
	if err := r.DB().Raw("SELECT id FROM node WHERE name = ?", "entry").Scan(&entryNodeID).Error; err != nil {
		r.Close()
		t.Fatalf("query entry node id: %v", err)
	}

	tx := r.DB().Begin()
	if tx.Error != nil {
		r.Close()
		t.Fatalf("begin tx: %v", tx.Error)
	}
	tunnelID, err := r.CreateTunnelTx(tx, "ssh", 1, 1, 0, now, 1, nil, 1, "")
	if err != nil {
		tx.Rollback()
		r.Close()
		t.Fatalf("create tunnel: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		r.Close()
		t.Fatalf("commit tunnel: %v", err)
	}

	forwardID, err := r.CreateForwardTx(7, "alice", "web", tunnelID, "10.0.0.10:80", "round", now, 1, []int64{entryNodeID}, 8080, "", nil)
	if err != nil {
		r.Close()
		t.Fatalf("create forward: %v", err)
	}

	return r, tunnelID, forwardID, entryNodeID
}

func createTunnelForDashRebuildTest(t *testing.T, r *repo.Repository, name string, tunnelType int) int64 {
	t.Helper()
	const now int64 = 123
	tx := r.DB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx: %v", tx.Error)
	}
	tunnelID, err := r.CreateTunnelTx(tx, name, 1, tunnelType, 0, now, 1, nil, 1, "")
	if err != nil {
		tx.Rollback()
		t.Fatalf("create tunnel %s: %v", name, err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit tunnel %s: %v", name, err)
	}
	return tunnelID
}

func addChainHopForDashRebuildTest(t *testing.T, r *repo.Repository, tunnelID int64, chainType string, nodeID int64, port int, inx int) {
	t.Helper()
	tx := r.DB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin chain tx: %v", tx.Error)
	}
	if err := r.CreateChainTunnelTx(tx, tunnelID, chainType, nodeID, sql.NullInt64{Int64: int64(port), Valid: port > 0}, "round", inx, "tls", ""); err != nil {
		tx.Rollback()
		t.Fatalf("create chain hop: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit chain hop: %v", err)
	}
}

func containsAll(s string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}

func assertForwardProtocolResult(t *testing.T, got ForwardProtocolApplyResult, wantProtocol, wantRuleID string, wantStatus ForwardApplyStatus) {
	t.Helper()
	if got.Protocol != wantProtocol || got.RuleID != wantRuleID || got.Status != wantStatus {
		t.Fatalf("unexpected protocol result: %+v", got)
	}
}
