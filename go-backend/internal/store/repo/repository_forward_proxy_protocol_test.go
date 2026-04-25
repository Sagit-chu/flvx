package repo

import (
	"testing"
	"time"

	"go-backend/internal/store/model"
)

func TestGetForwardRecordIncludesProxyProtocol(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	if err := r.DB().Create(&model.Forward{
		UserID:        1,
		UserName:      "admin",
		Name:          "proxy-forward",
		TunnelID:      1,
		RemoteAddr:    "1.1.1.1:443",
		Strategy:      "fifo",
		CreatedTime:   now,
		UpdatedTime:   now,
		Status:        1,
		ProxyProtocol: 2,
	}).Error; err != nil {
		t.Fatalf("create forward: %v", err)
	}

	forwardID := mustRepoLastInsertID(t, r)
	record, err := r.GetForwardRecord(forwardID)
	if err != nil {
		t.Fatalf("GetForwardRecord: %v", err)
	}
	if record == nil {
		t.Fatalf("expected forward record")
	}
	if record.ProxyProtocol != 2 {
		t.Fatalf("expected proxyProtocol 2, got %d", record.ProxyProtocol)
	}
	if record.MaxConn != 0 {
		t.Fatalf("expected default maxConn 0, got %d", record.MaxConn)
	}
}

func mustRepoLastInsertID(t *testing.T, r *Repository) int64 {
	t.Helper()
	var id int64
	if err := r.DB().Raw("SELECT last_insert_rowid()").Row().Scan(&id); err != nil {
		t.Fatalf("last_insert_rowid: %v", err)
	}
	if id <= 0 {
		t.Fatalf("invalid last_insert_rowid %d", id)
	}
	return id
}
