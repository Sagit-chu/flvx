package repo

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestChunkFlowUploadForwardIDs(t *testing.T) {
	ids := make([]int64, 0, 1001)
	for i := int64(1); i <= 1001; i++ {
		ids = append(ids, i)
	}

	chunks := chunkFlowUploadForwardIDs(ids)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if len(chunks[0]) != 500 || len(chunks[1]) != 500 || len(chunks[2]) != 1 {
		t.Fatalf("unexpected chunk sizes: %d, %d, %d", len(chunks[0]), len(chunks[1]), len(chunks[2]))
	}
	if chunks[0][0] != 1 || chunks[1][0] != 501 || chunks[2][0] != 1001 {
		t.Fatalf("unexpected chunk boundaries: %#v %#v %#v", chunks[0][:1], chunks[1][:1], chunks[2][:1])
	}
}

func TestSortedFlowUploadTargetIDs(t *testing.T) {
	totals := map[int64][2]int64{
		9: {1, 1},
		2: {1, 1},
		7: {1, 1},
	}

	got := sortedFlowUploadTargetIDs(totals)
	want := []int64{2, 7, 9}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected sorted ids %v, got %v", want, got)
	}
}

func TestGetFlowUploadForwardMetasAndApplyFlowUploadDeltasBatch(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "flow-batch.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	if err := r.DB().Exec(`INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status) VALUES(2, 'u2', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := r.DB().Exec(`INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx) VALUES(1, 't1', 2.0, 1, 'tls', 3, ?, ?, 1, NULL, 0)`, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	if err := r.DB().Exec(`INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status) VALUES(10, 2, 1, NULL, 99999, 99999, 0, 0, 1, 2727251700000, 1)`).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}
	if err := r.DB().Exec(`INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx) VALUES(20, 2, 'u2', 'f20', 1, '1.1.1.1:80', 'fifo', 0, 0, ?, ?, 1, 0)`, now, now).Error; err != nil {
		t.Fatalf("insert forward: %v", err)
	}

	metas, err := r.GetFlowUploadForwardMetas([]int64{20, 99})
	if err != nil {
		t.Fatalf("get metas: %v", err)
	}
	if metas[20].TunnelID != 1 || metas[20].TrafficRatio != 2 || metas[20].TunnelFlow != 3 {
		t.Fatalf("unexpected meta for forward 20: %#v", metas[20])
	}
	if _, ok := metas[99]; ok {
		t.Fatalf("did not expect meta for missing forward 99")
	}

	err = r.ApplyFlowUploadDeltasBatch([]FlowUploadCounterDelta{{ForwardID: 20, UserID: 2, UserTunnelID: 10, InFlow: 480, OutFlow: 660}})
	if err != nil {
		t.Fatalf("apply flow batch: %v", err)
	}
	if got := mustFlowBatchCount(t, r, `SELECT in_flow FROM forward WHERE id = 20`); got != 480 {
		t.Fatalf("expected forward in_flow=480, got %d", got)
	}
	if got := mustFlowBatchCount(t, r, `SELECT out_flow FROM user WHERE id = 2`); got != 660 {
		t.Fatalf("expected user out_flow=660, got %d", got)
	}
	if got := mustFlowBatchCount(t, r, `SELECT in_flow FROM user_tunnel WHERE id = 10`); got != 480 {
		t.Fatalf("expected user_tunnel in_flow=480, got %d", got)
	}
}

func TestGetFlowUploadForwardMetasKeepsForwardsWhenTunnelRowMissing(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "flow-batch-missing-tunnel.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	if err := r.DB().Exec(`INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx) VALUES(25, 2, 'u2', 'f25', 99, '1.1.1.1:80', 'fifo', 0, 0, ?, ?, 1, 0)`, now, now).Error; err != nil {
		t.Fatalf("insert forward: %v", err)
	}

	metas, err := r.GetFlowUploadForwardMetas([]int64{25})
	if err != nil {
		t.Fatalf("get metas: %v", err)
	}
	meta, ok := metas[25]
	if !ok {
		t.Fatalf("expected metadata for forward with missing tunnel row")
	}
	if meta.ForwardID != 25 || meta.TunnelID != 99 || meta.TrafficRatio != 1 || meta.TunnelFlow != 1 {
		t.Fatalf("unexpected fallback meta: %#v", meta)
	}
}

func TestAddUserQuotaUsageBatchReturnsNormalizedViews(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "quota-batch.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now()
	nowMs := now.UnixMilli()
	if err := r.DB().Exec(`INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status) VALUES(2, 'u2', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)`, nowMs, nowMs).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	views, err := r.AddUserQuotaUsageBatch(map[int64]int64{2: 1140}, now)
	if err != nil {
		t.Fatalf("batch quota update: %v", err)
	}
	if views[2] == nil || views[2].DailyUsedBytes != 1140 || views[2].MonthlyUsedBytes != 1140 {
		t.Fatalf("unexpected quota view: %#v", views[2])
	}
}

func mustFlowBatchCount(t *testing.T, r *Repository, query string, args ...interface{}) int64 {
	t.Helper()
	var value int64
	if err := r.DB().Raw(query, args...).Row().Scan(&value); err != nil {
		t.Fatalf("query %q failed: %v", query, err)
	}
	return value
}
