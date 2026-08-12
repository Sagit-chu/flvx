package handler

import (
	"path/filepath"
	"testing"

	"go-backend/internal/store/repo"
)

func TestSpeedLimiterExistsPreservesForwardRuleLimiter(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	if err := r.DB().Exec(`
		INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx, ip_speed_id)
		VALUES(8, 1, 'user', 'forward', 1, '127.0.0.1:80', 'fifo', 0, 0, 1, 1, 1, 0, 3),
		       (9, 1, 'user', 'forward-without-ip-limit', 1, '127.0.0.1:81', 'fifo', 0, 0, 1, 1, 1, 0, NULL)
	`).Error; err != nil {
		t.Fatalf("insert forward: %v", err)
	}

	h := &Handler{repo: r}
	if !h.speedLimiterExists("rule_traffic_limit_8") {
		t.Fatal("expected runtime limiter for existing forward to be preserved")
	}
	if h.speedLimiterExists("rule_traffic_limit_9") {
		t.Fatal("expected runtime limiter for forward without per-IP speed limit to be treated as orphaned")
	}
	if h.speedLimiterExists("rule_traffic_limit_10") {
		t.Fatal("expected runtime limiter for missing forward to be treated as orphaned")
	}
	if h.speedLimiterExists("rule_traffic_limit_invalid") {
		t.Fatal("expected malformed runtime limiter name to be treated as orphaned")
	}
}
