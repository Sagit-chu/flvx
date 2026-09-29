package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go-backend/internal/http/response"
	"go-backend/internal/store/repo"
	"gorm.io/gorm"
)

func resourceTestShare(t *testing.T, h *Handler, token string) *repo.PeerShare {
	t.Helper()
	now := time.Now().UnixMilli()
	share := &repo.PeerShare{Name: token, NodeID: 1, Token: token, IsActive: 1, PortRangeStart: 31000, PortRangeEnd: 32000, CreatedTime: now, UpdatedTime: now}
	if err := h.repo.CreatePeerShare(share); err != nil {
		t.Fatal(err)
	}
	return share
}
func resourceTestCommand(t *testing.T, h *Handler, share *repo.PeerShare, cmd string, data interface{}) response.R {
	t.Helper()
	body, err := json.Marshal(federationRuntimeCommandRequest{CommandType: cmd, Data: data})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/runtime/command", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+share.Token)
	rec := httptest.NewRecorder()
	h.federationRuntimeCommand(rec, req)
	var result response.R
	if err = json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func resourceTestService(name string, port int) []interface{} {
	return []interface{}{map[string]interface{}{"name": name, "addr": fmt.Sprintf(":%d", port), "handler": map[string]interface{}{"type": "tcp", "chain": "70"}, "listener": map[string]interface{}{"type": "tcp"}, "limiter": "10,20", "climiter": "30"}}
}

func TestPeerResourceCommandIsolationAndDurableRestore(t *testing.T) {
	a := newCleanupAgent(t)
	first := resourceTestShare(t, a.h, "resource-first")
	second := resourceTestShare(t, a.h, "resource-second")
	for i, share := range []*repo.PeerShare{first, second} {
		for _, entry := range []struct{ cmd, name string }{{"AddLimiters", "10"}, {"AddCLimiters", "30"}, {"AddChains", "70"}} {
			result := resourceTestCommand(t, a.h, share, entry.cmd, map[string]interface{}{"name": entry.name})
			if result.Code != 0 {
				t.Fatal(result.Msg)
			}
		}
		result := resourceTestCommand(t, a.h, share, "AddService", resourceTestService("70_1_0_tcp", 31001+i))
		if result.Code != 0 {
			t.Fatal(result.Msg)
		}
	}
	commands := a.commandsOfType("UpdateService")
	if len(commands) != 2 {
		t.Fatalf("commands: %+v", commands)
	}
	for i, share := range []*repo.PeerShare{first, second} {
		var configs []map[string]interface{}
		if err := json.Unmarshal(commands[i].Data, &configs); err != nil {
			t.Fatal(err)
		}
		c := configs[0]
		if c["name"] != peerShareResourceName(share.ID, "service", "70_1_0_tcp") {
			t.Fatalf("unscoped service: %v", c)
		}
		if c["handler"].(map[string]interface{})["chain"] != peerShareResourceName(share.ID, "chain", "70") {
			t.Fatalf("unscoped chain: %v", c)
		}
		want := peerShareResourceName(share.ID, "limiter", "10") + "," + peerShareResourceName(share.ID, "limiter", "20")
		if c["limiter"] != want || c["climiter"] != peerShareResourceName(share.ID, "climiter", "30") {
			t.Fatalf("unscoped limiter: %v", c)
		}
	}
	result := resourceTestCommand(t, a.h, first, "DeleteService", map[string]interface{}{"services": []string{"70_1_0_tcp"}})
	if result.Code != 0 {
		t.Fatal(result.Msg)
	}
	deleted := a.commandsOfType("DeleteService")
	if len(deleted) != 1 || !strings.Contains(string(deleted[0].Data), peerShareResourceName(first.ID, "service", "70_1_0_tcp")) {
		t.Fatalf("wrong deletion: %+v", deleted)
	}
	// A new Handler simulates restart with only durable desired state retained.
	restarted := &Handler{repo: a.h.repo, wsServer: a.h.wsServer}
	if err := restarted.reconcilePeerShareResourcesOnNode(1); err != nil {
		t.Fatal(err)
	}
	commands = a.commandsOfType("UpdateService")
	if len(commands) != 3 || !strings.Contains(string(commands[2].Data), peerShareResourceName(second.ID, "service", "70_1_0_tcp")) {
		t.Fatalf("wrong recovery: %+v", commands)
	}
	if got := resourceTestCommand(t, a.h, first, "DeleteService", map[string]interface{}{"services": []string{peerShareResourceName(second.ID, "service", "70_1_0_tcp")}}); got.Code == 0 {
		t.Fatal("accepted foreign scoped name")
	}
	if got := resourceTestCommand(t, a.h, first, "Reload", nil); got.Code == 0 {
		t.Fatal("accepted global reload")
	}
}

func TestPeerResourcePreRegistrationAndDatabaseFailure(t *testing.T) {
	a := newCleanupAgent(t)
	share := resourceTestShare(t, a.h, "resource-preregister")
	items, err := a.h.preparePeerResourceCommand(share, "AddService", resourceTestService("70_1_0_tcp", 31001))
	if err != nil {
		t.Fatal(err)
	}
	if len(a.commandsOfType("UpdateService")) != 0 {
		t.Fatal("prepare sent service before persistence")
	}
	stored, err := a.h.repo.GetPeerShareResource(share.ID, "service", "70_1_0_tcp")
	if err != nil || stored == nil || stored.Applied != 0 {
		t.Fatalf("missing pending ownership: %+v %v", stored, err)
	}
	runCleanupPath(a.h, "single", []string{items[0].RuntimeName})
	a.probe(t)
	if len(a.commandsOfType("DeleteService")) != 0 {
		t.Fatal("flow report removed pending service")
	}
	if err := a.h.repo.DB().Callback().Create().Before("gorm:create").Register("fail-resource-registration", func(tx *gorm.DB) {
		if tx.Statement.Table == "peer_share_resource" {
			tx.AddError(errors.New("simulated resource write failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer a.h.repo.DB().Callback().Create().Remove("fail-resource-registration")
	got := resourceTestCommand(t, a.h, share, "AddService", resourceTestService("71_1_0", 31002))
	if got.Code == 0 {
		t.Fatal("database failure returned success")
	}
	if len(a.commandsOfType("UpdateService")) != 0 {
		t.Fatal("node received command after persistence failure")
	}
}

func TestPeerResourceBindingDatabaseFailure(t *testing.T) {
	a := newCleanupAgent(t)
	share := resourceTestShare(t, a.h, "resource-bind-failure")
	if err := a.h.repo.DB().Callback().Create().Before("gorm:create").Register("fail-runtime-registration", func(tx *gorm.DB) {
		if tx.Statement.Table == "peer_share_runtime" {
			tx.AddError(errors.New("simulated binding failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer a.h.repo.DB().Callback().Create().Remove("fail-runtime-registration")
	got := resourceTestCommand(t, a.h, share, "AddService", resourceTestService("70_1_0", 31001))
	if got.Code == 0 {
		t.Fatal("binding failure returned success")
	}
	if len(a.commandsOfType("UpdateService")) != 0 {
		t.Fatal("service sent before successful binding")
	}
}

func TestPeerResourceScopedNameRoundTrip(t *testing.T) {
	for _, name := range []string{"70_1_0", "70_1_0_tcp", "70_1_0_udp", "a-b_c"} {
		scoped := peerShareResourceName(12, "service", name)
		id, got, ok := parsePeerShareServiceName(scoped)
		if !ok || id != 12 || got != name {
			t.Fatalf("round trip failed: %q", scoped)
		}
	}
}

func TestPeerResourceDeletesCandidateNamesAndPreservesFailedTombstone(t *testing.T) {
	a := newCleanupAgent(t)
	share := resourceTestShare(t, a.h, "resource-delete-candidates")
	got := resourceTestCommand(t, a.h, share, "AddService", resourceTestService("70_1_0_tcp", 31001))
	if got.Code != 0 {
		t.Fatal(got.Msg)
	}
	got = resourceTestCommand(t, a.h, share, "DeleteService", map[string]interface{}{"services": []string{"70_1_0_tcp", "70_1_0_udp", "70_1_0"}})
	if got.Code != 0 {
		t.Fatal(got.Msg)
	}
	if len(a.commandsOfType("DeleteService")) != 1 {
		t.Fatal("unregistered names were sent to the node")
	}
	// A disconnected node cannot acknowledge deletion: keep the pending row.
	if got = resourceTestCommand(t, a.h, share, "AddService", resourceTestService("71_1_0", 31002)); got.Code != 0 {
		t.Fatal(got.Msg)
	}
	offline := &Handler{repo: a.h.repo}
	got = resourceTestCommand(t, offline, share, "DeleteService", map[string]interface{}{"services": []string{"71_1_0"}})
	if got.Code == 0 {
		t.Fatal("offline deletion reported success")
	}
	pending, err := a.h.repo.GetPeerShareResource(share.ID, "service", "71_1_0")
	if err != nil || pending.DesiredState != "deleted" || pending.Applied != 0 {
		t.Fatalf("lost pending deletion: %+v %v", pending, err)
	}
	if err := a.h.reconcilePeerShareResourcesOnNode(1); err != nil {
		t.Fatal(err)
	}
	pending, err = a.h.repo.GetPeerShareResource(share.ID, "service", "71_1_0")
	if err != nil || pending.Applied != 1 {
		t.Fatalf("deletion not retried: %+v %v", pending, err)
	}
	if got = resourceTestCommand(t, a.h, share, "ResumeService", map[string]interface{}{"services": []string{"71_1_0"}}); got.Code == 0 {
		t.Fatal("resume resurrected a deleted resource")
	}
}

func TestPeerResourceLegacyMigrationRequiresUnambiguousOwner(t *testing.T) {
	a := newCleanupAgent(t)
	shareID := a.addRuntime(t, "70_1_0", 1, 1, 1, time.Now())
	share, err := a.h.repo.GetPeerShare(shareID)
	if err != nil {
		t.Fatal(err)
	}
	got := resourceTestCommand(t, a.h, share, "AddService", resourceTestService("70_1_0_tcp", 31001))
	if got.Code != 0 {
		t.Fatal(got.Msg)
	}
	deleted := a.commandsOfType("DeleteService")
	if len(deleted) != 1 || string(deleted[0].Data) != `{"services":["70_1_0_tcp"]}` {
		t.Fatalf("legacy deletion was not exact: %+v", deleted)
	}
	item, err := a.h.repo.GetPeerShareResource(share.ID, "service", "70_1_0_tcp")
	if err != nil || item.LegacyNames != "" {
		t.Fatalf("legacy migration acknowledgment not persisted: %+v %v", item, err)
	}
	// Two shares with the same legacy name are never resolved by guessing.
	other := resourceTestShare(t, a.h, "resource-ambiguous")
	now := time.Now().UnixMilli()
	for i, sid := range []int64{share.ID, other.ID} {
		if err := a.h.repo.CreatePeerShareRuntime(&repo.PeerShareRuntime{ShareID: sid, NodeID: 1, ReservationID: fmt.Sprintf("ambiguous-%d", i), ResourceKey: fmt.Sprintf("ambiguous-%d", i), Role: "forward", ServiceName: "71_1_0", Port: 31003 + i, Applied: 1, Status: 1, CreatedTime: now, UpdatedTime: now}); err != nil {
			t.Fatal(err)
		}
	}
	got = resourceTestCommand(t, a.h, share, "AddService", resourceTestService("71_1_0_tcp", 31003))
	if got.Code == 0 {
		t.Fatal("ambiguous legacy owner accepted")
	}
	if len(a.commandsOfType("DeleteService")) != 1 {
		t.Fatal("ambiguous legacy service deleted")
	}
}

func TestPeerResourceLegacyFamilyPersistsAcrossPartialMigration(t *testing.T) {
	a := newCleanupAgent(t)
	shareID := a.addRuntime(t, "70_1_0", 1, 1, 1, time.Now())
	share, err := a.h.repo.GetPeerShare(shareID)
	if err != nil {
		t.Fatal(err)
	}
	if got := resourceTestCommand(t, a.h, share, "AddService", resourceTestService("70_1_0_tcp", 31001)); got.Code != 0 {
		t.Fatal(got.Msg)
	}
	item, err := a.h.repo.GetPeerShareResource(share.ID, "service", "70_1_0_tcp")
	if err != nil || item.LegacyServiceBase != "70_1_0" {
		t.Fatalf("lost legacy family ownership: %+v %v", item, err)
	}
	// A later request can still prove ownership of the old UDP transport.
	if got := resourceTestCommand(t, a.h, share, "AddService", resourceTestService("70_1_0_udp", 31001)); got.Code != 0 {
		t.Fatal(got.Msg)
	}
	deletes := a.commandsOfType("DeleteService")
	if len(deletes) != 2 || string(deletes[1].Data) != `{"services":["70_1_0_udp"]}` {
		t.Fatalf("lost UDP migration ownership: %+v", deletes)
	}
	if got := resourceTestCommand(t, a.h, share, "DeleteService", map[string]interface{}{"services": []string{"70_1_0_tcp", "70_1_0_udp", "70_1_0"}}); got.Code != 0 {
		t.Fatal(got.Msg)
	}
	item, err = a.h.repo.GetPeerShareResource(share.ID, "service", "70_1_0_tcp")
	if err != nil || item.LegacyServiceBase != "" {
		t.Fatalf("legacy family not released: %+v %v", item, err)
	}
}

func TestPeerResourceFailedRegistrationDoesNotRenameLegacyRuntime(t *testing.T) {
	a := newCleanupAgent(t)
	shareID := a.addRuntime(t, "70_1_0", 1, 1, 1, time.Now())
	share, err := a.h.repo.GetPeerShare(shareID)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.h.repo.DB().Callback().Create().Before("gorm:create").Register("fail-atomic-resource", func(tx *gorm.DB) {
		if tx.Statement.Table == "peer_share_resource" {
			tx.AddError(errors.New("simulated desired-state failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer a.h.repo.DB().Callback().Create().Remove("fail-atomic-resource")
	if got := resourceTestCommand(t, a.h, share, "AddService", resourceTestService("70_1_0_tcp", 31001)); got.Code == 0 {
		t.Fatal("registration failure returned success")
	}
	runtimes, err := a.h.repo.ListActivePeerShareRuntimesByShareID(share.ID)
	if err != nil || len(runtimes) != 1 || runtimes[0].ServiceName != "70_1_0" {
		t.Fatalf("legacy binding changed after rollback: %+v %v", runtimes, err)
	}
	if len(a.commandsOfType("DeleteService"))+len(a.commandsOfType("UpdateService")) != 0 {
		t.Fatal("node mutated despite transaction rollback")
	}
}

func TestPeerResourcePausedReconcileNeverStartsListener(t *testing.T) {
	a := newCleanupAgent(t)
	share := resourceTestShare(t, a.h, "resource-pause-recovery")
	for _, cmd := range []string{"AddService", "PauseService"} {
		var data interface{} = resourceTestService("70_1_0_tcp", 31001)
		if cmd == "PauseService" {
			data = map[string]interface{}{"services": []string{"70_1_0_tcp"}}
		}
		if got := resourceTestCommand(t, a.h, share, cmd, data); got.Code != 0 {
			t.Fatal(got.Msg)
		}
	}
	if err := a.h.reconcilePeerShareResourcesOnNode(1); err != nil {
		t.Fatal(err)
	}
	if len(a.commandsOfType("UpdateService")) != 1 {
		t.Fatal("paused listener started during reconciliation")
	}
	if got := resourceTestCommand(t, a.h, share, "ResumeService", map[string]interface{}{"services": []string{"70_1_0_tcp"}}); got.Code != 0 {
		t.Fatal(got.Msg)
	}
	if len(a.commandsOfType("UpdateService")) != 2 {
		t.Fatal("resume did not restore saved service")
	}
}

func TestPeerResourceDeleteUnmigratedLegacyOwner(t *testing.T) {
	a := newCleanupAgent(t)
	shareID := a.addRuntime(t, "70_1_0", 1, 1, 1, time.Now())
	share, err := a.h.repo.GetPeerShare(shareID)
	if err != nil {
		t.Fatal(err)
	}
	got := resourceTestCommand(t, a.h, share, "DeleteService", map[string]interface{}{"services": []string{"70_1_0_tcp", "70_1_0_udp", "70_1_0"}})
	if got.Code != 0 {
		t.Fatal(got.Msg)
	}
	removed := false
	for _, cmd := range a.commandsOfType("DeleteService") {
		var body struct {
			Services []string `json:"services"`
		}
		if err = json.Unmarshal(cmd.Data, &body); err != nil {
			t.Fatal(err)
		}
		for _, name := range body.Services {
			if name == "70_1_0" {
				removed = true
			}
		}
	}
	if !removed {
		t.Fatal("legacy delete reported success without sending old service deletion")
	}
}

func TestPeerResourcePartialDeletePreservesLegacyFamily(t *testing.T) {
	a := newCleanupAgent(t)
	shareID := a.addRuntime(t, "70_1_0", 1, 1, 1, time.Now())
	share, err := a.h.repo.GetPeerShare(shareID)
	if err != nil {
		t.Fatal(err)
	}
	if got := resourceTestCommand(t, a.h, share, "AddService", resourceTestService("70_1_0_tcp", 31001)); got.Code != 0 {
		t.Fatal(got.Msg)
	}
	if got := resourceTestCommand(t, a.h, share, "DeleteService", map[string]interface{}{"services": []string{"70_1_0_tcp"}}); got.Code != 0 {
		t.Fatal(got.Msg)
	}
	item, err := a.h.repo.GetPeerShareResource(share.ID, "service", "70_1_0_tcp")
	if err != nil || item.LegacyServiceBase != "70_1_0" {
		t.Fatalf("partial delete lost remaining legacy family: %+v %v", item, err)
	}
	for _, cmd := range a.commandsOfType("DeleteService") {
		if strings.Contains(string(cmd.Data), `"70_1_0_udp"`) {
			t.Fatal("partial TCP delete removed legacy UDP")
		}
	}
	if err = a.h.releasePeerShareResources(share.ID); err != nil {
		t.Fatal(err)
	}
	item, err = a.h.repo.GetPeerShareResource(share.ID, "service", "70_1_0_tcp")
	if err != nil || item.LegacyServiceBase != "" {
		t.Fatalf("full release lost legacy cleanup: %+v %v", item, err)
	}
}

func TestPeerResourceChainGroupsAreScopedAndOtherRegistriesRejected(t *testing.T) {
	a := newCleanupAgent(t)
	share := resourceTestShare(t, a.h, "resource-chain-groups")
	data := resourceTestService("70_1_0", 31001)
	data[0].(map[string]interface{})["handler"].(map[string]interface{})["chainGroup"] = map[string]interface{}{"chains": []string{"one", "two"}}
	if got := resourceTestCommand(t, a.h, share, "AddService", data); got.Code != 0 {
		t.Fatal(got.Msg)
	}
	body := string(a.commandsOfType("UpdateService")[0].Data)
	for _, name := range []string{"one", "two"} {
		if !strings.Contains(body, peerShareResourceName(share.ID, "chain", name)) {
			t.Fatalf("chainGroup reference was not scoped: %s", body)
		}
	}
	for _, reference := range []string{"resolver", "auther", "observer", "hop"} {
		data := resourceTestService("71_1_0", 31002)
		data[0].(map[string]interface{})[reference] = "global-resource"
		if got := resourceTestCommand(t, a.h, share, "AddService", data); got.Code == 0 {
			t.Fatalf("accepted global %s reference", reference)
		}
	}
}

func TestPeerResourceReconcileContinuesAfterAnotherShareFails(t *testing.T) {
	a := newCleanupAgent(t)
	first := resourceTestShare(t, a.h, "resource-failed-share")
	second := resourceTestShare(t, a.h, "resource-good-share")
	for i, share := range []*repo.PeerShare{first, second} {
		if got := resourceTestCommand(t, a.h, share, "AddService", resourceTestService("70_1_0", 31001+i)); got.Code != 0 {
			t.Fatal(got.Msg)
		}
	}
	if err := a.h.repo.SavePeerShareResources([]repo.PeerShareResource{{ShareID: first.ID, NodeID: 1, Kind: "limiter", OriginalName: "broken", RuntimeName: peerShareResourceName(first.ID, "limiter", "broken"), Config: "{", DesiredState: "active", UpdatedTime: time.Now().UnixMilli()}}); err != nil {
		t.Fatal(err)
	}
	if err := a.h.reconcilePeerShareResourcesOnNode(1); err == nil {
		t.Fatal("invalid dependency was not reported")
	}
	commands := a.commandsOfType("UpdateService")
	if len(commands) != 3 || !strings.Contains(string(commands[2].Data), peerShareResourceName(second.ID, "service", "70_1_0")) {
		t.Fatalf("failed share blocked healthy share, or failed dependency service was started: %+v", commands)
	}
}

func TestPeerResourceRechecksShareBeforeRecreation(t *testing.T) {
	for _, state := range []string{"inactive", "expired", "exceeded"} {
		t.Run(state, func(t *testing.T) {
			a := newCleanupAgent(t)
			share := resourceTestShare(t, a.h, "resource-state-"+state)
			switch state {
			case "inactive":
				share.IsActive = 0
			case "expired":
				share.ExpiryTime = time.Now().Add(-time.Hour).UnixMilli()
			case "exceeded":
				share.MaxBandwidth = 1
				share.CurrentFlow = 1 << 40
			}
			if err := a.h.repo.UpdatePeerShare(share); err != nil {
				t.Fatal(err)
			}
			if state == "exceeded" {
				if err := a.h.repo.AddPeerShareCurrentFlow(share.ID, share.CurrentFlow); err != nil {
					t.Fatal(err)
				}
			}
			if got := resourceTestCommand(t, a.h, share, "AddService", resourceTestService("70_1_0", 31001)); got.Code == 0 {
				t.Fatal("invalid share recreated resources")
			}
			if len(a.commandsOfType("UpdateService")) != 0 {
				t.Fatal("invalid share reached the node")
			}
		})
	}
}

func TestPeerResourcePendingRetryLeavesAppliedSiblingAlone(t *testing.T) {
	a := newCleanupAgent(t)
	share := resourceTestShare(t, a.h, "resource-pending-only")
	offline := &Handler{repo: a.h.repo}
	if got := resourceTestCommand(t, offline, share, "AddService", resourceTestService("70_1_0", 31001)); got.Code == 0 {
		t.Fatal("offline apply reported success")
	}
	if got := resourceTestCommand(t, a.h, share, "AddService", resourceTestService("71_1_0", 31002)); got.Code != 0 {
		t.Fatal(got.Msg)
	}
	if err := a.h.retryPendingPeerShareResourcesOnNode(1); err != nil {
		t.Fatal(err)
	}
	commands := a.commandsOfType("UpdateService")
	if len(commands) != 2 || !strings.Contains(string(commands[1].Data), peerShareResourceName(share.ID, "service", "70_1_0")) {
		t.Fatalf("pending retry restarted applied sibling: %+v", commands)
	}
	if err := a.h.retryPendingPeerShareResourcesOnNode(1); err != nil {
		t.Fatal(err)
	}
	if len(a.commandsOfType("UpdateService")) != 2 {
		t.Fatal("no-op pending retry restarted applied services")
	}
}

func TestPeerResourceLegacyReleaseAcknowledgmentIsAtomic(t *testing.T) {
	a := newCleanupAgent(t)
	shareID := a.addRuntime(t, "70_1_0", 1, 1, 1, time.Now())
	share, err := a.h.repo.GetPeerShare(shareID)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.h.repo.DB().Callback().Update().Before("gorm:update").Register("fail-runtime-release", func(tx *gorm.DB) {
		if tx.Statement.Table == "peer_share_runtime" {
			tx.AddError(errors.New("simulated completion failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	got := resourceTestCommand(t, a.h, share, "DeleteService", map[string]interface{}{"services": []string{"70_1_0", "70_1_0_tcp", "70_1_0_udp"}})
	if got.Code == 0 {
		t.Fatal("completion database failure reported success")
	}
	items, err := a.h.repo.ListPeerShareResourcesByNode(1)
	if err != nil {
		t.Fatal(err)
	}
	proof := false
	pending := false
	for _, item := range items {
		proof = proof || item.LegacyServiceBase == "70_1_0"
		pending = pending || item.Applied == 0
	}
	if !proof || !pending {
		t.Fatalf("failure lost ownership or retry marker: %+v", items)
	}
	if err := a.h.repo.DB().Callback().Update().Remove("fail-runtime-release"); err != nil {
		t.Fatal(err)
	}
	if err := a.h.retryPendingPeerShareResourcesOnNode(1); err != nil {
		t.Fatal(err)
	}
	runtimes, err := a.h.repo.ListActivePeerShareRuntimesByShareID(share.ID)
	if err != nil || len(runtimes) != 0 {
		t.Fatalf("retry leaked legacy reservation: %+v %v", runtimes, err)
	}
}
