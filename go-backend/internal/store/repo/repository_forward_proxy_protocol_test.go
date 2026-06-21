package repo

import (
	"database/sql"
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

func TestForwardRepositoryPersistsPerIPLimits(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	forwardID, err := r.CreateForwardTx(1, "admin", "per-ip-forward", 2, "1.1.1.1:443", "fifo", now, 1, []int64{3}, 24000, "", nil, 0, 5, int64(21), 0, 0, 0)
	if err != nil {
		t.Fatalf("CreateForwardTx: %v", err)
	}
	record, err := r.GetForwardRecord(forwardID)
	if err != nil {
		t.Fatalf("GetForwardRecord after create: %v", err)
	}
	if record.IPMaxConn != 5 {
		t.Fatalf("expected created ipMaxConn 5, got %d", record.IPMaxConn)
	}
	if !record.IPSpeedID.Valid || record.IPSpeedID.Int64 != 21 {
		t.Fatalf("expected created ipSpeedId 21, got %+v", record.IPSpeedID)
	}

	if err := r.UpdateForward(forwardID, "per-ip-forward", 2, "2.2.2.2:443", "fifo", now+1, nil, 0, 9, int64(22), 0, 0, 0); err != nil {
		t.Fatalf("UpdateForward: %v", err)
	}
	record, err = r.GetForwardRecord(forwardID)
	if err != nil {
		t.Fatalf("GetForwardRecord after update: %v", err)
	}
	if record.IPMaxConn != 9 {
		t.Fatalf("expected updated ipMaxConn 9, got %d", record.IPMaxConn)
	}
	if !record.IPSpeedID.Valid || record.IPSpeedID.Int64 != 22 {
		t.Fatalf("expected updated ipSpeedId 22, got %+v", record.IPSpeedID)
	}

	if err := r.DB().Create(&model.Forward{
		UserID:      4,
		UserName:    "user",
		Name:        "listed-per-ip-forward",
		TunnelID:    8,
		RemoteAddr:  "3.3.3.3:443",
		Strategy:    "fifo",
		CreatedTime: now,
		UpdatedTime: now,
		Status:      1,
		IPMaxConn:   11,
		IPSpeedID:   sql.NullInt64{Int64: 33, Valid: true},
	}).Error; err != nil {
		t.Fatalf("create listed forward: %v", err)
	}
	records, err := r.ListForwardsByTunnel(8)
	if err != nil {
		t.Fatalf("ListForwardsByTunnel: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 listed record, got %d", len(records))
	}
	if records[0].IPMaxConn != 11 || !records[0].IPSpeedID.Valid || records[0].IPSpeedID.Int64 != 33 {
		t.Fatalf("expected listed per-IP limits 11/33, got ipMaxConn=%d ipSpeedId=%+v", records[0].IPMaxConn, records[0].IPSpeedID)
	}
}

func TestForwardRepositoryPersistsProxyProtocolReceiveAndSend(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	forwardID, err := r.CreateForwardTx(1, "admin", "proxy-protocol-forward", 2, "1.1.1.1:443", "fifo", now, 1, []int64{3}, 24000, "", nil, 0, 0, nil, 0, 1, 2)
	if err != nil {
		t.Fatalf("CreateForwardTx: %v", err)
	}

	record, err := r.GetForwardRecord(forwardID)
	if err != nil {
		t.Fatalf("GetForwardRecord after create: %v", err)
	}
	if record.ProxyProtocolReceive != 1 || record.ProxyProtocolSend != 2 {
		t.Fatalf("expected proxyProtocol receive/send 1/2 after create, got %d/%d", record.ProxyProtocolReceive, record.ProxyProtocolSend)
	}

	if err := r.UpdateForward(forwardID, "proxy-protocol-forward", 2, "2.2.2.2:443", "fifo", now+1, nil, 0, 0, nil, 0, 2, 1); err != nil {
		t.Fatalf("UpdateForward: %v", err)
	}
	record, err = r.GetForwardRecord(forwardID)
	if err != nil {
		t.Fatalf("GetForwardRecord after update: %v", err)
	}
	if record.ProxyProtocolReceive != 2 || record.ProxyProtocolSend != 1 {
		t.Fatalf("expected proxyProtocol receive/send 2/1 after update, got %d/%d", record.ProxyProtocolReceive, record.ProxyProtocolSend)
	}

	records, err := r.ListForwardsByTunnel(2)
	if err != nil {
		t.Fatalf("ListForwardsByTunnel: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 listed record, got %d", len(records))
	}
	if records[0].ProxyProtocolReceive != 2 || records[0].ProxyProtocolSend != 1 {
		t.Fatalf("expected listed proxyProtocol receive/send 2/1, got %d/%d", records[0].ProxyProtocolReceive, records[0].ProxyProtocolSend)
	}
}

func TestForwardRepositoryMapsLegacyProxyProtocolToSend(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	if err := r.DB().Create(&model.Forward{
		UserID:        2,
		UserName:      "user",
		Name:          "legacy-proxy-protocol-forward",
		TunnelID:      9,
		RemoteAddr:    "1.1.1.1:443",
		Strategy:      "fifo",
		CreatedTime:   now,
		UpdatedTime:   now,
		Status:        1,
		ProxyProtocol: 2,
	}).Error; err != nil {
		t.Fatalf("create legacy forward: %v", err)
	}
	forwardID := mustRepoLastInsertID(t, r)

	record, err := r.GetForwardRecord(forwardID)
	if err != nil {
		t.Fatalf("GetForwardRecord: %v", err)
	}
	if record.ProxyProtocolReceive != 0 || record.ProxyProtocolSend != 2 {
		t.Fatalf("expected legacy proxyProtocol to map to receive/send 0/2, got %d/%d", record.ProxyProtocolReceive, record.ProxyProtocolSend)
	}
}

func TestRollbackForwardFieldsRestoresPerIPLimits(t *testing.T) {
	r, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	forwardID, err := r.CreateForwardTx(1, "admin", "rollback-per-ip-forward", 2, "1.1.1.1:443", "fifo", now, 1, nil, 0, "", nil, 7, 5, int64(21), 2, 0, 2)
	if err != nil {
		t.Fatalf("CreateForwardTx: %v", err)
	}
	if err := r.UpdateForward(forwardID, "rollback-per-ip-forward", 2, "2.2.2.2:443", "fifo", now+1, nil, 0, 0, nil, 0, 0, 0); err != nil {
		t.Fatalf("UpdateForward: %v", err)
	}

	r.RollbackForwardFields(forwardID, 1, "admin", "rollback-per-ip-forward", 2, "1.1.1.1:443", "fifo", 1, nil, 7, 5, int64(21), 2, 0, 2, now+2)

	record, err := r.GetForwardRecord(forwardID)
	if err != nil {
		t.Fatalf("GetForwardRecord: %v", err)
	}
	if record.IPMaxConn != 5 {
		t.Fatalf("expected rollback ipMaxConn 5, got %d", record.IPMaxConn)
	}
	if !record.IPSpeedID.Valid || record.IPSpeedID.Int64 != 21 {
		t.Fatalf("expected rollback ipSpeedId 21, got %+v", record.IPSpeedID)
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
