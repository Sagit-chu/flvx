package handler

import (
	"testing"
	"time"
)

func TestFlowOwnershipPendingDeletionRemainsBillableUntilAcknowledged(t *testing.T) {
	for _, mode := range []string{"single", "batch"} {
		t.Run(mode, func(t *testing.T) {
			a := newCleanupAgent(t)
			share := resourceTestShare(t, a.h, "pending-deletion-"+mode)
			if got := resourceTestCommand(t, a.h, share, "AddService", resourceTestService("70_1_0_tcp", 31001)); got.Code != 0 {
				t.Fatal(got.Msg)
			}
			name := peerShareResourceName(share.ID, "service", "70_1_0_tcp")
			// Simulate a failed delivery after the deletion intent is committed. The
			// mock node's previously installed listener has received no delete command.
			disconnected := &Handler{repo: a.h.repo}
			if got := resourceTestCommand(t, disconnected, share, "DeleteService", map[string]interface{}{"services": []string{"70_1_0_tcp"}}); got.Code == 0 {
				t.Fatal("failed deletion reported success")
			}
			resource, err := a.h.repo.GetPeerShareResource(share.ID, "service", "70_1_0_tcp")
			if err != nil || resource == nil || resource.DesiredState != "deleted" || resource.Applied != 0 {
				t.Fatalf("missing pending tombstone: %+v %v", resource, err)
			}
			report := func() {
				item := flowItem{N: name, U: 120, D: 80}
				if mode == "single" {
					a.h.processFlowItem(1, item)
				} else {
					a.h.applyFlowUploadBatch(1, a.h.buildNodeFlowUploadBatch(1, []flowItem{item}, nil), time.Now())
				}
			}
			report()
			stored, err := a.h.repo.GetPeerShare(share.ID)
			if err != nil || stored.CurrentFlow != 200 {
				t.Fatalf("unconfirmed deletion stopped billing: share=%+v err=%v", stored, err)
			}
			if len(a.commandsOfType("DeleteService")) != 0 {
				t.Fatal("flow handling deleted a pending shared listener")
			}
			if err := a.h.retryPendingPeerShareResourcesOnNode(1); err != nil {
				t.Fatal(err)
			}
			resource, err = a.h.repo.GetPeerShareResource(share.ID, "service", "70_1_0_tcp")
			if err != nil || resource.Applied != 1 {
				t.Fatalf("deletion acknowledgment not recorded: %+v %v", resource, err)
			}
			report()
			stored, err = a.h.repo.GetPeerShare(share.ID)
			if err != nil || stored.CurrentFlow != 200 {
				t.Fatalf("confirmed-deleted listener was billed again: share=%+v err=%v", stored, err)
			}
		})
	}
}
