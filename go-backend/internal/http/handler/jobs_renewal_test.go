package handler

import (
	"database/sql"
	"testing"
	"time"

	"go-backend/internal/store/repo"
)

func TestRunNodeRenewalCycleJob_AdvancesOverdueAnchorTimes(t *testing.T) {
	dbPath := t.TempDir() + "/renewal-test.db"
	r, err := repo.Open(dbPath)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
	})

	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	nowMs := now.UnixMilli()

	nodeID := int64(101)
	err = r.DB().Exec(`
		INSERT INTO node (id, name, secret, server_ip, port, http, tls, socks, created_time, status, renewal_cycle, expiry_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, nodeID, "no-cycle-node", "test-secret", "192.168.1.1", "1000-65535", 1, 1, 1, nowMs, 1, "", nil).Error
	if err != nil {
		t.Fatalf("insert test node: %v", err)
	}

	quarterNodeID := int64(102)
	err = r.DB().Exec(`
		INSERT INTO node (id, name, secret, server_ip, port, http, tls, socks, created_time, status, renewal_cycle, expiry_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, quarterNodeID, "quarter-node", "test-secret", "192.168.1.1", "1000-65535", 1, 1, 1, nowMs, 1, "quarter", now.AddDate(0, -4, 0).UnixMilli()).Error
	if err != nil {
		t.Fatalf("insert test node: %v", err)
	}

	h := &Handler{repo: r}
	h.runNodeRenewalCycleJob(now)

	var anchor sql.NullInt64
	err = r.DB().Raw(`SELECT expiry_time FROM node WHERE id = ?`, quarterNodeID).Row().Scan(&anchor)
	if err != nil {
		t.Fatalf("query expiry_time: %v", err)
	}

	expectedAnchor := now.AddDate(0, 2, 0).UnixMilli()
	if !anchor.Valid || anchor.Int64 != expectedAnchor {
		t.Fatalf("expected anchor %d (2026-05-08), got %d", expectedAnchor, anchor.Int64)
	}
}
