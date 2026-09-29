package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go-backend/internal/store/repo"
	"go-backend/internal/ws"
	"gorm.io/gorm"
)

func roleRuntimeFixture(t *testing.T, agent *cleanupAgent, role string) *repo.PeerShareRuntime {
	t.Helper()
	now := time.Now().UnixMilli()
	share := &repo.PeerShare{Name: "role-recovery", NodeID: 1, Token: "role-recovery-token", PortRangeStart: 31000, PortRangeEnd: 31010, IsActive: 1, CreatedTime: now, UpdatedTime: now}
	if err := agent.h.repo.CreatePeerShare(share); err != nil {
		t.Fatal(err)
	}
	runtime := &repo.PeerShareRuntime{ShareID: share.ID, NodeID: 1, ReservationID: "role-reservation", ResourceKey: "role-resource", BindingID: "333", Role: role, ServiceName: "fed_svc_333", Protocol: "tls", Strategy: "round", Port: 31000, Applied: 1, Status: 1, CreatedTime: now, UpdatedTime: now}
	if role == "middle" {
		runtime.ChainName = "fed_chain_333"
		runtime.Target = `[{"host":"127.0.0.1","port":32000,"protocol":"tls"}]`
	}
	if err := agent.h.repo.CreatePeerShareRuntime(runtime); err != nil {
		t.Fatal(err)
	}
	// Runtime-generated names use the provider's globally unique runtime ID.
	runtime.BindingID = fmt.Sprint(runtime.ID)
	runtime.ServiceName = fmt.Sprintf("fed_svc_%d", runtime.ID)
	if role == "middle" {
		runtime.ChainName = federationRuntimeChainName(runtime.BindingID)
	}
	if err := agent.h.repo.UpdatePeerShareRuntime(runtime); err != nil {
		t.Fatal(err)
	}
	return runtime
}

func roleRuntimeRequest(t *testing.T, h *Handler, body string, release bool) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/runtime", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer role-recovery-token")
	res := httptest.NewRecorder()
	if release {
		h.federationRuntimeReleaseRole(res, req)
	} else {
		h.federationRuntimeApplyRole(res, req)
	}
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	t.Logf("runtime response: %s", res.Body.String())
	return result.Code
}

func TestPeerShareAppliedRuntimeRepairsEmptyAgent(t *testing.T) {
	for _, role := range []string{"exit", "middle"} {
		t.Run(role, func(t *testing.T) {
			agent := newCleanupAgent(t)
			roleRuntimeFixture(t, agent, role)
			if !agent.h.redeployNodeRuntimeAfterUpgrade(1) {
				t.Fatal("reconnect reconciliation failed")
			}
			if got := len(agent.commandsOfType("UpdateService")); got != 1 {
				t.Fatalf("reconnect did not restore service: %d", got)
			}
			targets := ""
			if role == "middle" {
				targets = `,"targets":[{"host":"127.0.0.1","port":32000,"protocol":"tls"}]`
			}
			if code := roleRuntimeRequest(t, agent.h, `{"reservationId":"role-reservation","role":"`+role+`","protocol":"tls"`+targets+`}`, false); code != 0 {
				t.Fatalf("apply failed: %d", code)
			}
			if got := len(agent.commandsOfType("UpdateService")); got != 2 {
				t.Fatalf("Applied=1 skipped service repair: %d", got)
			}
			if role == "middle" {
				if got := len(agent.commandsOfType("UpdateChains")); got != 2 {
					t.Fatalf("missing middle chains: %d", got)
				}
				agent.mu.Lock()
				defer agent.mu.Unlock()
				chainSeen := false
				for _, cmd := range agent.commands {
					if cmd.Type == "UpdateChains" {
						chainSeen = true
					}
					if cmd.Type == "UpdateService" && !chainSeen {
						t.Fatal("service started before chain")
					}
				}
			}
		})
	}
}

func TestPeerShareReleaseOfflineKeepsPortAndReconnectDeletes(t *testing.T) {
	agent := newCleanupAgent(t)
	runtime := roleRuntimeFixture(t, agent, "middle")
	liveServer := agent.h.wsServer
	agent.h.wsServer = ws.NewServer(agent.h.repo, "offline-test")
	if code := roleRuntimeRequest(t, agent.h, `{"reservationId":"role-reservation"}`, true); code == 0 {
		t.Fatal("offline release reported success")
	}
	stored, err := agent.h.repo.GetPeerShareRuntimeByID(runtime.ID)
	if err != nil || stored.Status != 1 || stored.ReleasePending != 1 {
		t.Fatalf("pending release lost: %+v err=%v", stored, err)
	}
	occupied, err := agent.h.repo.ExistsActivePeerShareRuntimeOnNodePort(1, 31000)
	if err != nil || !occupied {
		t.Fatalf("pending release freed occupied port: %t %v", occupied, err)
	}
	if code := roleRuntimeRequest(t, agent.h, `{"reservationId":"role-reservation","role":"middle","targets":[{"host":"127.0.0.1","port":32000}]}`, false); code == 0 {
		t.Fatal("pending release was revived by apply")
	}
	agent.h.wsServer = liveServer
	if !agent.h.redeployNodeRuntimeAfterUpgrade(1) {
		t.Fatal("reconnect cleanup failed")
	}
	stored, err = agent.h.repo.GetPeerShareRuntimeByID(runtime.ID)
	if err != nil || stored.Status != 0 || stored.Applied != 0 || stored.ReleasePending != 0 {
		t.Fatalf("release not completed: %+v %v", stored, err)
	}
	if len(agent.commandsOfType("DeleteService")) != 1 || len(agent.commandsOfType("DeleteChains")) != 1 {
		t.Fatal("reconnect did not delete both service and chain")
	}
	if len(agent.commandsOfType("UpdateService")) != 0 || len(agent.commandsOfType("UpdateChains")) != 0 {
		t.Fatal("reconnect revived pending release")
	}
	occupied, err = agent.h.repo.ExistsActivePeerShareRuntimeOnNodePort(1, 31000)
	if err != nil || occupied {
		t.Fatalf("acknowledged release still occupies port: %t %v", occupied, err)
	}
}

func TestPeerShareApplyPersistsBeforeCommand(t *testing.T) {
	agent := newCleanupAgent(t)
	runtime := roleRuntimeFixture(t, agent, "exit")
	callback := "test:fail-role-desired-write"
	if err := agent.h.repo.DB().Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "peer_share_runtime" {
			tx.AddError(errors.New("desired-state write failed"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	// This database is test-local; keep the callback registered through the
	// websocket teardown so callback mutation cannot race node status writes.
	if code := roleRuntimeRequest(t, agent.h, `{"reservationId":"role-reservation","role":"exit"}`, false); code == 0 {
		t.Fatal("failed ownership persistence reported success")
	}
	if len(agent.commandsOfType("UpdateService")) != 0 {
		t.Fatal("service started before ownership persisted")
	}
	stored, err := agent.h.repo.GetPeerShareRuntimeByID(runtime.ID)
	if err != nil || stored.Applied != 1 {
		t.Fatalf("old runtime changed on failed persistence: %+v %v", stored, err)
	}
}

func TestPeerShareApplyInvalidMiddleKeepsDesiredConfig(t *testing.T) {
	agent := newCleanupAgent(t)
	runtime := roleRuntimeFixture(t, agent, "middle")
	if code := roleRuntimeRequest(t, agent.h, `{"reservationId":"role-reservation","role":"middle","targets":[]}`, false); code == 0 {
		t.Fatal("invalid middle targets accepted")
	}
	stored, err := agent.h.repo.GetPeerShareRuntimeByID(runtime.ID)
	if err != nil || stored.Target != runtime.Target || stored.Applied != 1 {
		t.Fatalf("invalid update changed desired runtime: %+v %v", stored, err)
	}
	if len(agent.commandsOfType("UpdateService"))+len(agent.commandsOfType("UpdateChains")) != 0 {
		t.Fatal("invalid config sent to agent")
	}
}

func TestPeerShareUnacknowledgedApplyRecoversOrReleases(t *testing.T) {
	for _, release := range []bool{false, true} {
		t.Run(fmt.Sprintf("release=%t", release), func(t *testing.T) {
			agent := newCleanupAgent(t)
			runtime := roleRuntimeFixture(t, agent, "middle")
			liveServer := agent.h.wsServer
			agent.h.wsServer = ws.NewServer(agent.h.repo, "offline-test")
			if code := roleRuntimeRequest(t, agent.h, `{"reservationId":"role-reservation","role":"middle","targets":[{"host":"127.0.0.1","port":32001,"protocol":"tls"}]}`, false); code == 0 {
				t.Fatal("offline apply reported success")
			}
			stored, err := agent.h.repo.GetPeerShareRuntimeByID(runtime.ID)
			if err != nil || stored.Applied != 0 || !strings.Contains(stored.Target, "32001") || stored.ServiceName == "" {
				t.Fatalf("unacknowledged desired state lost: %+v %v", stored, err)
			}
			if release {
				if code := roleRuntimeRequest(t, agent.h, `{"reservationId":"role-reservation"}`, true); code == 0 {
					t.Fatal("unacknowledged listener was freed offline")
				}
			}
			agent.h.wsServer = liveServer
			if !agent.h.redeployNodeRuntimeAfterUpgrade(1) {
				t.Fatal("reconnect failed")
			}
			stored, err = agent.h.repo.GetPeerShareRuntimeByID(runtime.ID)
			if err != nil {
				t.Fatal(err)
			}
			if release {
				if stored.Status != 0 || len(agent.commandsOfType("DeleteService")) != 1 || len(agent.commandsOfType("UpdateService")) != 0 {
					t.Fatalf("unacknowledged apply revived after release: %+v", stored)
				}
			} else {
				if stored.Status != 1 || stored.Applied != 1 || len(agent.commandsOfType("UpdateService")) != 1 {
					t.Fatalf("unacknowledged desired state not recovered: %+v", stored)
				}
			}
		})
	}
}

func TestPeerShareDeleteOfflineRetainsDisabledShare(t *testing.T) {
	agent := newCleanupAgent(t)
	runtime := roleRuntimeFixture(t, agent, "exit")
	liveServer := agent.h.wsServer
	agent.h.wsServer = ws.NewServer(agent.h.repo, "offline-test")
	req := httptest.NewRequest(http.MethodPost, "/share/delete", strings.NewReader(fmt.Sprintf(`{"id":%d}`, runtime.ShareID)))
	res := httptest.NewRecorder()
	agent.h.federationShareDelete(res, req)
	var result struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Code == 0 {
		t.Fatal("offline share delete reported success")
	}
	share, err := agent.h.repo.GetPeerShare(runtime.ShareID)
	if err != nil || share == nil || share.IsActive != 0 {
		t.Fatalf("cleanup retry record lost or still accepts allocations: %+v %v", share, err)
	}
	occupied, err := agent.h.repo.ExistsActivePeerShareRuntimeOnNodePort(1, 31000)
	if err != nil || !occupied {
		t.Fatal("offline share deletion released its port")
	}
	agent.h.wsServer = liveServer
	if !agent.h.redeployNodeRuntimeAfterUpgrade(1) {
		t.Fatal("disabled share cleanup did not retry")
	}
	if len(agent.commandsOfType("DeleteService")) != 1 || len(agent.commandsOfType("UpdateService")) != 0 {
		t.Fatal("disabled share was recreated")
	}
}

func TestPeerShareLegacyReleaseWithoutLocalCollisionDeletesFamily(t *testing.T) {
	agent := newCleanupAgent(t)
	runtime := roleRuntimeFixture(t, agent, "forward")
	runtime.ServiceName = "70_1_0"
	if err := agent.h.repo.UpdatePeerShareRuntime(runtime); err != nil {
		t.Fatal(err)
	}
	if err := agent.h.releasePeerShareRuntime(runtime); err != nil {
		t.Fatal(err)
	}
	commands := agent.commandsOfType("DeleteService")
	if len(commands) != 1 {
		t.Fatalf("expected one family cleanup, got %d", len(commands))
	}
	var payload struct {
		Services []string `json:"services"`
	}
	if err := json.Unmarshal(commands[0].Data, &payload); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(payload.Services, ",")
	if got != "70_1_0,70_1_0_tcp,70_1_0_udp" {
		t.Fatalf("incomplete legacy cleanup: %s", got)
	}
	stored, err := agent.h.repo.GetPeerShareRuntimeByID(runtime.ID)
	if err != nil || stored.Status != 0 || stored.ReleasePending != 0 {
		t.Fatalf("legacy release incomplete: %+v %v", stored, err)
	}
}

func TestPeerSharePendingRoleRetryDoesNotReplaySuccessfulServices(t *testing.T) {
	agent := newCleanupAgent(t)
	runtime := roleRuntimeFixture(t, agent, "exit")
	if err := agent.h.retryPendingPeerShareRoleRuntimesOnNode(1); err != nil {
		t.Fatal(err)
	}
	if len(agent.commandsOfType("UpdateService")) != 0 {
		t.Fatal("maintenance replayed an acknowledged service")
	}
	runtime.Applied = 0
	if err := agent.h.repo.UpdatePeerShareRuntime(runtime); err != nil {
		t.Fatal(err)
	}
	if err := agent.h.retryPendingPeerShareRoleRuntimesOnNode(1); err != nil {
		t.Fatal(err)
	}
	if len(agent.commandsOfType("UpdateService")) != 1 {
		t.Fatal("maintenance did not retry unfinished service")
	}
	if err := agent.h.retryPendingPeerShareRoleRuntimesOnNode(1); err != nil {
		t.Fatal(err)
	}
	if len(agent.commandsOfType("UpdateService")) != 1 {
		t.Fatal("maintenance repeated a successful retry")
	}
}

func TestPeerShareResourceFailureDoesNotBlockRoleOrLocalRecovery(t *testing.T) {
	agent := newCleanupAgent(t)
	runtime := roleRuntimeFixture(t, agent, "exit")
	if err := agent.h.repo.SavePeerShareResources([]repo.PeerShareResource{{ShareID: runtime.ShareID, NodeID: 1, Kind: "chain", OriginalName: "broken", RuntimeName: "broken", Config: "invalid-json", DesiredState: "active", Applied: 0, UpdatedTime: time.Now().UnixMilli()}}); err != nil {
		t.Fatal(err)
	}
	var localQueries atomic.Int32
	if err := agent.h.repo.DB().Callback().Query().Before("gorm:query").Register("test:observe-local-recovery", func(tx *gorm.DB) {
		if tx.Statement.Table == "chain_tunnel" || tx.Statement.Table == "forward_port" {
			localQueries.Add(1)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if agent.h.redeployNodeRuntimeAfterUpgrade(1) {
		t.Fatal("invalid shared resource was reported recovered")
	}
	if len(agent.commandsOfType("UpdateService")) != 1 {
		t.Fatal("resource failure blocked independent role service recovery")
	}
	if localQueries.Load() < 2 {
		t.Fatal("shared failure blocked independent local recovery")
	}
	// A shared failure is owned by pending maintenance, without full-redeploy
	// timers that would periodically restart healthy listeners.
	agent.h.onNodeOnline(1)
	agent.h.upgradeMu.Lock()
	_, fullQueued := agent.h.nodeOnlineRedeployQueued[1]
	_, localQueued := agent.h.nodeLocalRuntimeRetryQueued[1]
	agent.h.upgradeMu.Unlock()
	if fullQueued || localQueued {
		t.Fatal("shared-only failure queued a full or local redeploy")
	}
	before := len(agent.commandsOfType("UpdateService"))
	agent.h.retryNodeLocalRuntime(1)
	if len(agent.commandsOfType("UpdateService")) != before {
		t.Fatal("local retry replayed shared listeners")
	}
}
