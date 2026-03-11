package handler

import (
	"path/filepath"
	"testing"
	"time"

	"go-backend/internal/store/repo"
)

func TestRunStatisticsFlowJobTracksIncrementAndPrunes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "jobs-stats.db")
	r, err := repo.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	h := New(r, "secret")
	now := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	nowMs := now.UnixMilli()

	if err := r.DB().Exec(`UPDATE user SET in_flow = 100, out_flow = 200 WHERE id = 1`).Error; err != nil {
		t.Fatalf("seed user flow: %v", err)
	}

	if err := r.DB().Exec(`INSERT INTO statistics_flow(user_id, flow, total_flow, time, created_time) VALUES(1, 250, 250, '11:00', ?)`, now.Add(-time.Hour).UnixMilli()).Error; err != nil {
		t.Fatalf("seed recent statistics row: %v", err)
	}
	if err := r.DB().Exec(`INSERT INTO statistics_flow(user_id, flow, total_flow, time, created_time) VALUES(1, 10, 10, '00:00', ?)`, now.Add(-49*time.Hour).UnixMilli()).Error; err != nil {
		t.Fatalf("seed stale statistics row: %v", err)
	}

	h.runStatisticsFlowJob(now)

	staleCount := mustQueryInt(t, r, `SELECT COUNT(1) FROM statistics_flow WHERE created_time < ?`, nowMs-int64((48*time.Hour)/time.Millisecond))
	if staleCount != 0 {
		t.Fatalf("expected stale statistics rows to be pruned, got %d", staleCount)
	}

	flow, total, hour := mustQueryInt64Int64String(t, r, `SELECT flow, total_flow, time FROM statistics_flow WHERE user_id = 1 ORDER BY id DESC LIMIT 1`)
	if flow != 50 {
		t.Fatalf("expected increment flow 50, got %d", flow)
	}
	if total != 300 {
		t.Fatalf("expected total flow 300, got %d", total)
	}
	if hour != "12:00" {
		t.Fatalf("expected hour mark 12:00, got %s", hour)
	}
}

func TestRunResetAndExpiryJobResetsFlowAndDisablesExpiredRecords(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "jobs-reset.db")
	r, err := repo.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	h := New(r, "secret")
	now := time.Date(2026, 3, 15, 0, 0, 5, 0, time.UTC)
	nowMs := now.UnixMilli()

	if err := r.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(2, 'expired_user', 'x', 1, ?, 100, 1000, 2000, 15, 1, ?, ?, 1)
	`, nowMs-1000, nowMs, nowMs).Error; err != nil {
		t.Fatalf("insert expired user: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(3, 'non_expiring_user', 'x', 1, 0, 100, 1000, 2000, 15, 1, ?, ?, 1)
	`, nowMs, nowMs).Error; err != nil {
		t.Fatalf("insert non-expiring user: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(1, 't1', 1.0, 1, 'tls', 1, ?, ?, 1, NULL, 0)
	`, nowMs, nowMs).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(10, 2, 1, NULL, 1, 1, 300, 400, 15, ?, 1)
	`, nowMs-1000).Error; err != nil {
		t.Fatalf("insert expired user_tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(11, 3, 1, NULL, 1, 1, 300, 400, 15, 0, 1)
	`).Error; err != nil {
		t.Fatalf("insert non-expiring user_tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(20, 2, 'expired_user', 'f1', 1, '1.1.1.1:443', 'fifo', 0, 0, ?, ?, 1, 0)
	`, nowMs, nowMs).Error; err != nil {
		t.Fatalf("insert forward: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(21, 3, 'non_expiring_user', 'f2', 1, '1.1.1.1:443', 'fifo', 0, 0, ?, ?, 1, 1)
	`, nowMs, nowMs).Error; err != nil {
		t.Fatalf("insert non-expiring forward: %v", err)
	}

	h.runResetAndExpiryJob(now)

	userIn, userOut, userStatus := mustQueryInt64Int64Int(t, r, `SELECT in_flow, out_flow, status FROM user WHERE id = 2`)
	if userIn != 0 || userOut != 0 || userStatus != 0 {
		t.Fatalf("expected user reset+disabled, got in=%d out=%d status=%d", userIn, userOut, userStatus)
	}

	utIn, utOut, utStatus := mustQueryInt64Int64Int(t, r, `SELECT in_flow, out_flow, status FROM user_tunnel WHERE id = 10`)
	if utIn != 0 || utOut != 0 || utStatus != 0 {
		t.Fatalf("expected user_tunnel reset+disabled, got in=%d out=%d status=%d", utIn, utOut, utStatus)
	}

	forwardStatus := mustQueryInt(t, r, `SELECT status FROM forward WHERE id = 20`)
	if forwardStatus != 0 {
		t.Fatalf("expected forward status=0 after expiry handling, got %d", forwardStatus)
	}

	nonExpUserStatus := mustQueryInt(t, r, `SELECT status FROM user WHERE id = 3`)
	if nonExpUserStatus != 1 {
		t.Fatalf("expected non-expiring user to remain enabled, got status=%d", nonExpUserStatus)
	}

	nonExpTunnelStatus := mustQueryInt(t, r, `SELECT status FROM user_tunnel WHERE id = 11`)
	if nonExpTunnelStatus != 1 {
		t.Fatalf("expected non-expiring user_tunnel to remain enabled, got status=%d", nonExpTunnelStatus)
	}

	nonExpForwardStatus := mustQueryInt(t, r, `SELECT status FROM forward WHERE id = 21`)
	if nonExpForwardStatus != 1 {
		t.Fatalf("expected non-expiring forward to remain enabled, got status=%d", nonExpForwardStatus)
	}
}

func TestRunResetAndExpiryJobResetsTunnelQuotaAndReEnablesTunnel(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "jobs-quota-reset.db")
	r, err := repo.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	h := New(r, "secret")
	now := time.Date(2026, 3, 12, 0, 0, 5, 0, time.UTC)
	nowMs := now.UnixMilli()

	if err := r.DB().Exec(`
		INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(1, 'quota-reset-tunnel', 1.0, 1, 'tls', 1, ?, ?, 0, NULL, 0)
	`, nowMs, nowMs).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	if err := r.DB().Exec(`
		INSERT INTO tunnel_quota(tunnel_id, daily_limit_gb, monthly_limit_gb, daily_used_bytes, monthly_used_bytes, day_key, month_key, disabled_by_quota, disabled_at, paused_forward_ids, created_time, updated_time)
		VALUES(1, 10, 0, ?, ?, 20260311, 202603, 1, ?, '', ?, ?)
	`, 11*int64(1024*1024*1024), 11*int64(1024*1024*1024), nowMs, nowMs, nowMs).Error; err != nil {
		t.Fatalf("insert tunnel quota: %v", err)
	}

	h.runResetAndExpiryJob(now)

	tunnelStatus := mustQueryInt(t, r, `SELECT status FROM tunnel WHERE id = 1`)
	if tunnelStatus != 1 {
		t.Fatalf("expected tunnel re-enabled after quota reset, got %d", tunnelStatus)
	}
	dailyUsed := mustQueryInt(t, r, `SELECT daily_used_bytes FROM tunnel_quota WHERE tunnel_id = 1`)
	if dailyUsed != 0 {
		t.Fatalf("expected daily quota usage reset, got %d", dailyUsed)
	}
	quotaDisabled := mustQueryInt(t, r, `SELECT disabled_by_quota FROM tunnel_quota WHERE tunnel_id = 1`)
	if quotaDisabled != 0 {
		t.Fatalf("expected quota disabled flag cleared, got %d", quotaDisabled)
	}
}
