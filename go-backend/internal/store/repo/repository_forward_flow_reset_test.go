package repo

import (
	"path/filepath"
	"testing"
)

func TestResetForwardFlowOnlyUpdatesSelectedForward(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "forward-flow-reset.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	const originalUpdated int64 = 1000
	if err := r.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(2, 'owner', 'pwd', 1, 0, 100, 700, 900, 0, 10, 1000, 1000, 1)
	`).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := r.DB().Exec(`
		INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(1, 'tunnel', 1, 1, 'tls', 1, 1000, 1000, 1, NULL, 0)
	`).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	if err := r.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(10, 2, 1, 10, 100, 500, 600, 0, 0, 1)
	`).Error; err != nil {
		t.Fatalf("insert user tunnel: %v", err)
	}
	if err := r.DB().Exec(`
		INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES
		  (20, 2, 'owner', 'target', 1, '127.0.0.1:80', 'fifo', 111, 222, 1000, ?, 1, 0),
		  (21, 2, 'owner', 'other', 1, '127.0.0.1:81', 'fifo', 333, 444, 1000, ?, 1, 1)
	`, originalUpdated, originalUpdated).Error; err != nil {
		t.Fatalf("insert forwards: %v", err)
	}

	const resetAt int64 = 2000
	if err := r.ResetForwardFlow(20, resetAt); err != nil {
		t.Fatalf("ResetForwardFlow: %v", err)
	}

	assertForwardFlowResetValue(t, r, "SELECT in_flow FROM forward WHERE id = 20", 0)
	assertForwardFlowResetValue(t, r, "SELECT out_flow FROM forward WHERE id = 20", 0)
	assertForwardFlowResetValue(t, r, "SELECT updated_time FROM forward WHERE id = 20", resetAt)
	assertForwardFlowResetValue(t, r, "SELECT in_flow FROM forward WHERE id = 21", 333)
	assertForwardFlowResetValue(t, r, "SELECT out_flow FROM forward WHERE id = 21", 444)
	assertForwardFlowResetValue(t, r, "SELECT in_flow FROM user WHERE id = 2", 700)
	assertForwardFlowResetValue(t, r, "SELECT out_flow FROM user WHERE id = 2", 900)
	assertForwardFlowResetValue(t, r, "SELECT in_flow FROM user_tunnel WHERE id = 10", 500)
	assertForwardFlowResetValue(t, r, "SELECT out_flow FROM user_tunnel WHERE id = 10", 600)
}

func TestResetForwardFlowRejectsUninitializedRepository(t *testing.T) {
	var r *Repository
	if err := r.ResetForwardFlow(20, 2000); err == nil {
		t.Fatal("expected uninitialized repository error")
	}
}

func assertForwardFlowResetValue(t *testing.T, r *Repository, query string, want int64) {
	t.Helper()
	var got int64
	if err := r.DB().Raw(query).Scan(&got).Error; err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	if got != want {
		t.Fatalf("query %q returned %d, want %d", query, got, want)
	}
}
