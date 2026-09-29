package handler

import (
	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
	"testing"
	"time"
)

func TestFlowOwnershipSharedFlowDoesNotChargeLocalCollision(t *testing.T) {
	a := newCleanupAgent(t)
	shareID := a.addRuntime(t, "70_1_0", 1, 1, 1, time.Now())
	if err := a.h.repo.DB().Create(&model.Tunnel{ID: 9, Name: "local", TrafficRatio: 1, Flow: 1, Type: 1, Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.h.repo.DB().Create(&model.Forward{ID: 70, UserID: 2, UserName: "local", Name: "local", TunnelID: 9, RemoteAddr: "127.0.0.1:80", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	metas, err := a.h.repo.GetFlowUploadForwardMetas([]int64{70})
	if err != nil {
		t.Fatal(err)
	}
	a.h.applyFlowUploadBatch(1, a.h.buildNodeFlowUploadBatch(1, []flowItem{{N: "70_1_0_tcp", U: 120, D: 80}}, metas), time.Now())
	var local model.Forward
	if err := a.h.repo.DB().First(&local, 70).Error; err != nil {
		t.Fatal(err)
	}
	shared, err := a.h.repo.GetPeerShare(shareID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("local user=%d local bytes=%d shared bytes=%d", local.UserID, local.InFlow+local.OutFlow, shared.CurrentFlow)
	if local.InFlow+local.OutFlow != 0 {
		t.Errorf("shared flow charged colliding local forward: %d", local.InFlow+local.OutFlow)
	}
	if shared.CurrentFlow != 200 {
		t.Errorf("shared flow missing: %d", shared.CurrentFlow)
	}
}

func TestFlowOwnershipScopedServiceRequiresStoredNodeOwnership(t *testing.T) {
	a := newCleanupAgent(t)
	shareID := a.addRuntime(t, "70_1_0", 1, 1, 1, time.Now())
	name := peerShareResourceName(shareID, "service", "70_1_0_tcp")
	if err := a.h.repo.SavePeerShareResources([]repo.PeerShareResource{{
		ShareID: shareID, NodeID: 1, Kind: "service", OriginalName: "70_1_0_tcp",
		RuntimeName: name, DesiredState: "active", Applied: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	for _, nodeID := range []int64{2, 1} {
		batch := a.h.buildNodeFlowUploadBatch(nodeID, []flowItem{{N: name, U: 120, D: 80}}, nil)
		a.h.applyFlowUploadBatch(nodeID, batch, time.Now())
		share, err := a.h.repo.GetPeerShare(shareID)
		if err != nil {
			t.Fatal(err)
		}
		want := int64(0)
		if nodeID == 1 {
			want = 200
		}
		if share.CurrentFlow != want {
			t.Fatalf("node=%d: got flow=%d want=%d", nodeID, share.CurrentFlow, want)
		}
		if len(batch.flowDeltas) != 0 || len(batch.quotaUsage) != 0 || len(batch.orphanServices) != 0 {
			t.Fatalf("scoped traffic entered local accounting: %+v", batch)
		}
	}
	a.probe(t)
	if cmds := a.commandsOfType("DeleteService"); len(cmds) != 0 {
		t.Fatalf("scoped service deleted: %+v", cmds)
	}
}

func TestFlowOwnershipRoleRuntimeRequiresReportingNode(t *testing.T) {
	a := newCleanupAgent(t)
	shareID := a.addRuntime(t, "fed_svc_17", 1, 1, 1, time.Now())
	if err := a.h.repo.DB().Exec("UPDATE peer_share_runtime SET id = 17, role = 'exit'").Error; err != nil {
		t.Fatal(err)
	}
	a.h.processFlowItem(2, flowItem{N: "fed_svc_17", U: 120, D: 80})
	share, err := a.h.repo.GetPeerShare(shareID)
	if err != nil {
		t.Fatal(err)
	}
	if share.CurrentFlow != 0 {
		t.Fatalf("wrong node charged role runtime: %d", share.CurrentFlow)
	}
	a.h.processFlowItem(1, flowItem{N: "fed_svc_17", U: 120, D: 80})
	share, err = a.h.repo.GetPeerShare(shareID)
	if err != nil {
		t.Fatal(err)
	}
	if share.CurrentFlow != 200 {
		t.Fatalf("owner node flow missing: %d", share.CurrentFlow)
	}
}

func TestFlowOwnershipAmbiguousLegacyNameDoesNotDebitEitherOwner(t *testing.T) {
	a := newCleanupAgent(t)
	shareID := a.addRuntime(t, "70_1_0", 1, 1, 1, time.Now())
	if err := a.h.repo.DB().Create(&model.Forward{ID: 70, UserID: 1, UserName: "local", Name: "local", TunnelID: 9, RemoteAddr: "127.0.0.1:80", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.h.repo.DB().Create(&model.ForwardPort{ForwardID: 70, NodeID: 1, Port: 32000}).Error; err != nil {
		t.Fatal(err)
	}
	metas, err := a.h.repo.GetFlowUploadForwardMetas([]int64{70})
	if err != nil {
		t.Fatal(err)
	}
	batch := a.h.buildNodeFlowUploadBatch(1, []flowItem{{N: "70_1_0_tcp", U: 120, D: 80}}, metas)
	a.h.applyFlowUploadBatch(1, batch, time.Now())
	share, err := a.h.repo.GetPeerShare(shareID)
	if err != nil {
		t.Fatal(err)
	}
	if share.CurrentFlow != 0 || len(batch.flowDeltas) != 0 {
		t.Fatalf("ambiguous flow charged an owner: share=%d local=%+v", share.CurrentFlow, batch.flowDeltas)
	}
	a.probe(t)
	if commands := a.commandsOfType("DeleteService"); len(commands) != 0 {
		t.Fatalf("ambiguous legacy service deleted: %+v", commands)
	}
}

func TestFlowOwnershipPreservesUnmigratedLegacyTransport(t *testing.T) {
	a := newCleanupAgent(t)
	shareID := a.addRuntime(t, peerShareResourceName(1, "service", "70_1_0"), 1, 1, 1, time.Now())
	if err := a.h.repo.SavePeerShareResources([]repo.PeerShareResource{{
		ShareID: shareID, NodeID: 1, Kind: "service", OriginalName: "70_1_0_tcp",
		RuntimeName:       peerShareResourceName(shareID, "service", "70_1_0_tcp"),
		LegacyServiceBase: "70_1_0", DesiredState: "deleted", Applied: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	a.h.processFlowItem(1, flowItem{N: "70_1_0_udp", U: 120, D: 80})
	a.h.cleanNodeConfigs(1, `{"services":[{"name":"70_1_0_udp"}]}`)
	a.probe(t)
	if commands := a.commandsOfType("DeleteService"); len(commands) != 0 {
		t.Fatalf("unmigrated transport deleted: %+v", commands)
	}
	share, err := a.h.repo.GetPeerShare(shareID)
	if err != nil {
		t.Fatal(err)
	}
	if share.CurrentFlow != 200 {
		t.Fatalf("legacy transport accounting lost: %d", share.CurrentFlow)
	}
}

func TestFlowOwnershipLocalForwardRequiresReportingNode(t *testing.T) {
	a := newCleanupAgent(t)
	if err := a.h.repo.DB().Create(&model.Forward{ID: 70, UserID: 1, UserName: "local", Name: "local", TunnelID: 9, RemoteAddr: "127.0.0.1:80", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.h.repo.DB().Create(&model.ForwardPort{ForwardID: 70, NodeID: 2, Port: 32000}).Error; err != nil {
		t.Fatal(err)
	}
	metas, err := a.h.repo.GetFlowUploadForwardMetas([]int64{70})
	if err != nil {
		t.Fatal(err)
	}
	for _, nodeID := range []int64{1, 2} {
		batch := a.h.buildNodeFlowUploadBatch(nodeID, []flowItem{{N: "70_1_0_tcp", U: 120, D: 80}}, metas)
		a.h.applyFlowUploadBatch(nodeID, batch, time.Now())
		var forward model.Forward
		if err := a.h.repo.DB().First(&forward, 70).Error; err != nil {
			t.Fatal(err)
		}
		want := int64(0)
		if nodeID == 2 {
			want = 200
		}
		if forward.InFlow+forward.OutFlow != want {
			t.Fatalf("node %d local traffic=%d want=%d", nodeID, forward.InFlow+forward.OutFlow, want)
		}
	}
}

func TestPeerShareMaintenanceRetriesOnlyUnfinishedOperations(t *testing.T) {
	a := newCleanupAgent(t)
	shareID := a.addRuntime(t, "70_1_0", 1, 1, 1, time.Now())
	name := peerShareResourceName(shareID, "service", "80_1_0_tcp")
	if err := a.h.repo.SavePeerShareResources([]repo.PeerShareResource{{ShareID: shareID, NodeID: 1,
		Kind: "service", OriginalName: "80_1_0_tcp", RuntimeName: name, DesiredState: "deleted", Applied: 0,
	}}); err != nil {
		t.Fatal(err)
	}
	a.h.retryPendingPeerShareOperations()
	a.probe(t)
	if commands := a.commandsOfType("DeleteService"); len(commands) != 1 {
		t.Fatalf("pending delete was not retried: %+v", commands)
	}
	a.h.retryPendingPeerShareOperations()
	a.probe(t)
	if commands := a.commandsOfType("DeleteService"); len(commands) != 1 {
		t.Fatalf("completed delete was replayed: %+v", commands)
	}
	ids, err := a.h.repo.ListPendingPeerShareNodeIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("completed operation still pending: %v", ids)
	}
}

func TestFlowOwnershipDoesNotCrossNodes(t *testing.T) {
	a := newCleanupAgent(t)
	shareID := a.addRuntime(t, "70_1_0", 1, 1, 1, time.Now())
	share, err := a.h.repo.GetPeerShare(shareID)
	if err != nil {
		t.Fatal(err)
	}
	share.MaxBandwidth = 100
	if err := a.h.repo.UpdatePeerShare(share); err != nil {
		t.Fatal(err)
	}
	// Node 2 has no shared runtime. It reports the same local numeric service name.
	a.h.applyFlowUploadBatch(2, a.h.buildNodeFlowUploadBatch(2, []flowItem{{N: "70_1_0_tcp", U: 120, D: 80}}, nil), time.Now())
	a.probe(t)
	share, err = a.h.repo.GetPeerShare(shareID)
	if err != nil {
		t.Fatal(err)
	}
	commands := a.commandsOfType("DeleteService")
	t.Logf("node1 share charged from node2: %d; node1 delete commands: %+v", share.CurrentFlow, commands)
	if share.CurrentFlow != 0 {
		t.Errorf("flow crossed node boundary: %d", share.CurrentFlow)
	}
	if len(commands) != 0 {
		t.Errorf("node2 flow deleted node1 shared service: %+v", commands)
	}
}
