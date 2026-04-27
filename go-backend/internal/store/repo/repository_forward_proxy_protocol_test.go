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

func TestListForwardsByTunnelIncludesProxyProtocol(t *testing.T) {
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
		TunnelID:      7,
		RemoteAddr:    "1.1.1.1:443",
		Strategy:      "fifo",
		CreatedTime:   now,
		UpdatedTime:   now,
		Status:        1,
		ProxyProtocol: 2,
	}).Error; err != nil {
		t.Fatalf("create forward: %v", err)
	}

	records, err := r.ListForwardsByTunnel(7)
	if err != nil {
		t.Fatalf("ListForwardsByTunnel: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 forward record, got %d", len(records))
	}
	if records[0].ProxyProtocol != 2 {
		t.Fatalf("expected proxyProtocol 2, got %d", records[0].ProxyProtocol)
	}
}

func TestListForwardsByTunnelIncludesMaxConn(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	if err := r.DB().Create(&model.Forward{
		UserID:      1,
		UserName:    "admin",
		Name:        "max-conn-forward",
		TunnelID:    8,
		RemoteAddr:  "1.1.1.1:443",
		Strategy:    "fifo",
		CreatedTime: now,
		UpdatedTime: now,
		Status:      1,
		MaxConn:     42,
	}).Error; err != nil {
		t.Fatalf("create forward: %v", err)
	}

	records, err := r.ListForwardsByTunnel(8)
	if err != nil {
		t.Fatalf("ListForwardsByTunnel: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 forward record, got %d", len(records))
	}
	if records[0].MaxConn != 42 {
		t.Fatalf("expected maxConn 42, got %d", records[0].MaxConn)
	}
}

func TestListActiveForwardsByUserTunnelIncludesMaxConn(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	if err := r.DB().Create(&model.Forward{
		UserID:      2,
		UserName:    "user",
		Name:        "active-max-conn-forward",
		TunnelID:    9,
		RemoteAddr:  "1.1.1.1:443",
		Strategy:    "fifo",
		CreatedTime: now,
		UpdatedTime: now,
		Status:      1,
		MaxConn:     55,
	}).Error; err != nil {
		t.Fatalf("create forward: %v", err)
	}

	records, err := r.ListActiveForwardsByUserTunnel(2, 9)
	if err != nil {
		t.Fatalf("ListActiveForwardsByUserTunnel: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 forward record, got %d", len(records))
	}
	if records[0].MaxConn != 55 {
		t.Fatalf("expected maxConn 55, got %d", records[0].MaxConn)
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
