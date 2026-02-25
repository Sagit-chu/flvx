package handler

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"go-backend/internal/store/repo"
)

func TestProcessFlowItemTracksPeerShareFlowAndEnforcesLimit(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	if err := r.CreatePeerShare(&repo.PeerShare{
		Name:           "flow-share",
		NodeID:         1,
		Token:          "flow-share-token",
		MaxBandwidth:   3000,
		CurrentFlow:    1000,
		PortRangeStart: 32000,
		PortRangeEnd:   32010,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	}); err != nil {
		t.Fatalf("create peer share: %v", err)
	}
	share, err := r.GetPeerShareByToken("flow-share-token")
	if err != nil || share == nil {
		t.Fatalf("load peer share: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO peer_share_runtime(id, share_id, node_id, reservation_id, resource_key, binding_id, role, chain_name, service_name, protocol, strategy, port, target, applied, status, created_time, updated_time)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, 17, share.ID, share.NodeID, "res-17", "rk-17", "17", "exit", "", "fed_svc_17", "tls", "round", 32001, "", 1, 1, now, now).Error; err != nil {
		t.Fatalf("insert peer_share_runtime: %v", err)
	}

	h := &Handler{repo: r}
	h.processFlowItem(1, flowItem{N: "fed_svc_17", U: 1200, D: 900})

	updatedShare, err := r.GetPeerShare(share.ID)
	if err != nil || updatedShare == nil {
		t.Fatalf("reload share: %v", err)
	}
	if updatedShare.CurrentFlow != 3100 {
		t.Fatalf("expected current_flow=3100, got %d", updatedShare.CurrentFlow)
	}

	runtime, err := r.GetPeerShareRuntimeByID(17)
	if err != nil || runtime == nil {
		t.Fatalf("reload runtime: %v", err)
	}
	if runtime.Status != 0 {
		t.Fatalf("expected runtime status=0 after limit enforcement, got %d", runtime.Status)
	}
}

func TestProcessFlowItemTracksPeerShareFlowForFederationPortForward(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel-forward.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	if err := r.CreatePeerShare(&repo.PeerShare{
		Name:           "forward-share",
		NodeID:         1,
		Token:          "forward-share-token",
		MaxBandwidth:   0,
		CurrentFlow:    0,
		PortRangeStart: 30000,
		PortRangeEnd:   30010,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	}); err != nil {
		t.Fatalf("create peer share: %v", err)
	}
	share, err := r.GetPeerShareByToken("forward-share-token")
	if err != nil || share == nil {
		t.Fatalf("load peer share: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(2, 'u2', 'x', 1, ?, 99999, 0, 0, 1, 1, ?, ?, 1)
	`, now+24*60*60*1000, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	tunnelName := "Share-" + strconv.FormatInt(share.ID, 10) + "-Port-30001"
	if err := r.DB().Exec(`
		INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(1, ?, 1.0, 1, 'tls', 1, ?, ?, 1, NULL, 0)
	`, tunnelName, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(10, 2, 1, NULL, 1, 99999, 0, 0, 1, ?, 1)
	`, now+24*60*60*1000).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(20, 2, 'u2', 'f20', 1, '1.1.1.1:443', 'fifo', 0, 0, ?, ?, 1, 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert forward: %v", err)
	}

	h := &Handler{repo: r}
	h.processFlowItem(1, flowItem{N: "20_2_10", U: 120, D: 80})

	updatedShare, err := r.GetPeerShare(share.ID)
	if err != nil || updatedShare == nil {
		t.Fatalf("reload share: %v", err)
	}
	if updatedShare.CurrentFlow != 200 {
		t.Fatalf("expected current_flow=200, got %d", updatedShare.CurrentFlow)
	}
}

func TestProcessFlowItemTracksPeerShareFlowByForwardServiceName(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel-forward-service.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	if err := r.CreatePeerShare(&repo.PeerShare{
		Name:           "forward-service-share",
		NodeID:         1,
		Token:          "forward-service-token",
		MaxBandwidth:   0,
		CurrentFlow:    0,
		PortRangeStart: 31000,
		PortRangeEnd:   31010,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	}); err != nil {
		t.Fatalf("create peer share: %v", err)
	}
	share, err := r.GetPeerShareByToken("forward-service-token")
	if err != nil || share == nil {
		t.Fatalf("load peer share: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO peer_share_runtime(share_id, node_id, reservation_id, resource_key, binding_id, role, chain_name, service_name, protocol, strategy, port, target, applied, status, created_time, updated_time)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, share.ID, share.NodeID, "svc-r1", "svc-rk1", "", "forward", "", "20_2_10", "tcp", "fifo", 31001, "", 1, 1, now, now).Error; err != nil {
		t.Fatalf("insert peer_share_runtime: %v", err)
	}

	h := &Handler{repo: r}
	h.processFlowItem(1, flowItem{N: "20_2_10_tcp", U: 120, D: 80})

	updatedShare, err := r.GetPeerShare(share.ID)
	if err != nil || updatedShare == nil {
		t.Fatalf("reload share: %v", err)
	}
	if updatedShare.CurrentFlow != 200 {
		t.Fatalf("expected current_flow=200, got %d", updatedShare.CurrentFlow)
	}
}

func TestProcessFlowItemFallsBackToServiceNameWhenForwardIDCollidesAcrossPanels(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel-forward-collision.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	if err := r.CreatePeerShare(&repo.PeerShare{
		Name:           "collision-share",
		NodeID:         1,
		Token:          "collision-token",
		MaxBandwidth:   0,
		CurrentFlow:    0,
		PortRangeStart: 31400,
		PortRangeEnd:   31410,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	}); err != nil {
		t.Fatalf("create peer share: %v", err)
	}
	share, err := r.GetPeerShareByToken("collision-token")
	if err != nil || share == nil {
		t.Fatalf("load peer share: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO peer_share_runtime(share_id, node_id, reservation_id, resource_key, binding_id, role, chain_name, service_name, protocol, strategy, port, target, applied, status, created_time, updated_time)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, share.ID, share.NodeID, "collision-r1", "collision-rk1", "", "forward", "", "20_2_10", "tcp", "fifo", 31401, "", 1, 1, now, now).Error; err != nil {
		t.Fatalf("insert peer_share_runtime: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(2, 'local-tunnel-with-colliding-forward-id', 1.0, 1, 'tls', 1, ?, ?, 1, NULL, 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert local tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(20, 1, 'local-user', 'local-f20', 2, '8.8.8.8:53', 'fifo', 0, 0, ?, ?, 1, 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert local forward: %v", err)
	}

	h := &Handler{repo: r}
	h.processFlowItem(1, flowItem{N: "20_2_10_tcp", U: 120, D: 80})

	updatedShare, err := r.GetPeerShare(share.ID)
	if err != nil || updatedShare == nil {
		t.Fatalf("reload share: %v", err)
	}
	if updatedShare.CurrentFlow != 200 {
		t.Fatalf("expected current_flow=200, got %d", updatedShare.CurrentFlow)
	}
}

func TestProcessFlowItemSkipsPeerShareFlowWhenServiceNameIsAmbiguous(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel-forward-ambiguous.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	if err := r.CreatePeerShare(&repo.PeerShare{
		Name:           "ambiguous-share-a",
		NodeID:         1,
		Token:          "ambiguous-token-a",
		MaxBandwidth:   0,
		CurrentFlow:    0,
		PortRangeStart: 31100,
		PortRangeEnd:   31110,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	}); err != nil {
		t.Fatalf("create share A: %v", err)
	}
	if err := r.CreatePeerShare(&repo.PeerShare{
		Name:           "ambiguous-share-b",
		NodeID:         1,
		Token:          "ambiguous-token-b",
		MaxBandwidth:   0,
		CurrentFlow:    0,
		PortRangeStart: 31200,
		PortRangeEnd:   31210,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	}); err != nil {
		t.Fatalf("create share B: %v", err)
	}
	shareA, _ := r.GetPeerShareByToken("ambiguous-token-a")
	shareB, _ := r.GetPeerShareByToken("ambiguous-token-b")

	if err := r.DB().Exec(`
		INSERT INTO peer_share_runtime(share_id, node_id, reservation_id, resource_key, binding_id, role, chain_name, service_name, protocol, strategy, port, target, applied, status, created_time, updated_time)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?),
		      (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		shareA.ID, 1, "amb-r1", "amb-rk1", "", "forward", "", "99_2_10", "tcp", "fifo", 31101, "", 1, 1, now, now,
		shareB.ID, 1, "amb-r2", "amb-rk2", "", "forward", "", "99_2_10", "tcp", "fifo", 31201, "", 1, 1, now, now,
	).Error; err != nil {
		t.Fatalf("insert ambiguous runtimes: %v", err)
	}

	h := &Handler{repo: r}
	h.processFlowItem(1, flowItem{N: "99_2_10_tcp", U: 120, D: 80})

	updatedA, _ := r.GetPeerShare(shareA.ID)
	updatedB, _ := r.GetPeerShare(shareB.ID)
	if updatedA.CurrentFlow != 0 || updatedB.CurrentFlow != 0 {
		t.Fatalf("expected ambiguous service flow to be skipped, got shareA=%d shareB=%d", updatedA.CurrentFlow, updatedB.CurrentFlow)
	}
}

func TestCleanOrphanedServicesSkipsActiveSharedForwardRuntimeServices(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel-cleanup-runtime.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	if err := r.CreatePeerShare(&repo.PeerShare{
		Name:           "cleanup-runtime-share",
		NodeID:         1,
		Token:          "cleanup-runtime-token",
		MaxBandwidth:   0,
		CurrentFlow:    0,
		PortRangeStart: 31300,
		PortRangeEnd:   31310,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	}); err != nil {
		t.Fatalf("create peer share: %v", err)
	}
	share, err := r.GetPeerShareByToken("cleanup-runtime-token")
	if err != nil || share == nil {
		t.Fatalf("load peer share: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO peer_share_runtime(share_id, node_id, reservation_id, resource_key, binding_id, role, chain_name, service_name, protocol, strategy, port, target, applied, status, created_time, updated_time)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, share.ID, share.NodeID, "cleanup-r1", "cleanup-rk1", "", "forward", "", "20_2_10", "tcp", "fifo", 31301, "", 1, 1, now, now).Error; err != nil {
		t.Fatalf("insert peer_share_runtime: %v", err)
	}

	h := &Handler{repo: r}

	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("cleanOrphanedServices should skip active shared runtime service; got panic: %v", rec)
		}
	}()

	h.cleanOrphanedServices(share.NodeID, []namedConfigItem{{Name: "20_2_10_tcp"}})
}

func TestCleanOrphanedServicesSkipsFederationServicePrefix(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel-cleanup-fed-svc.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	h := &Handler{repo: r}

	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("cleanOrphanedServices should skip fed_svc_ service names; got panic: %v", rec)
		}
	}()

	h.cleanOrphanedServices(1, []namedConfigItem{{Name: "fed_svc_999_tcp"}})
}

func TestCleanOrphanedServicesSkipsForwardPatternWhenNodeHasActivePeerShareForwardRuntime(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel-cleanup-forward-runtime-empty-service.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	if err := r.CreatePeerShare(&repo.PeerShare{
		Name:           "cleanup-forward-runtime-empty-service",
		NodeID:         1,
		Token:          "cleanup-forward-runtime-empty-service-token",
		MaxBandwidth:   0,
		CurrentFlow:    0,
		PortRangeStart: 31420,
		PortRangeEnd:   31430,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	}); err != nil {
		t.Fatalf("create peer share: %v", err)
	}
	share, err := r.GetPeerShareByToken("cleanup-forward-runtime-empty-service-token")
	if err != nil || share == nil {
		t.Fatalf("load peer share: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO peer_share_runtime(share_id, node_id, reservation_id, resource_key, binding_id, role, chain_name, service_name, protocol, strategy, port, target, applied, status, created_time, updated_time)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, share.ID, share.NodeID, "cleanup-forward-empty-r1", "cleanup-forward-empty-rk1", "", "forward", "", "", "tcp", "fifo", 31421, "", 0, 1, now, now).Error; err != nil {
		t.Fatalf("insert peer_share_runtime with empty service name: %v", err)
	}

	h := &Handler{repo: r}

	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("cleanOrphanedServices should skip forward-pattern services when active peer-share forward runtime exists; got panic: %v", rec)
		}
	}()

	h.cleanOrphanedServices(share.NodeID, []namedConfigItem{{Name: "20_2_10_tcp"}})
}
