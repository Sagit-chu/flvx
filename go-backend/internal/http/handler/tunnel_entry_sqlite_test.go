package handler

import (
	"path/filepath"
	"testing"
	"time"

	"go-backend/internal/store/repo"
)

func TestValidateTunnelEntryPortConflictsForNewEntriesDoesNotBlockOnSQLiteTx(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
	})
	h := &Handler{repo: r}
	now := time.Now().UnixMilli()

	if err := r.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, port, created_time, status, tcp_listen_addr, udp_listen_addr, is_remote)
		VALUES
			('entry-old', 'secret-old', '10.0.0.1', '12000-12010', ?, 1, '[::]', '[::]', 0),
			('entry-new', 'secret-new', '10.0.0.2', '12000-12010', ?, 1, '[::]', '[::]', 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert nodes: %v", err)
	}
	var oldEntryID, newEntryID int64
	if err := r.DB().Raw(`SELECT id FROM node WHERE name = 'entry-old'`).Scan(&oldEntryID).Error; err != nil {
		t.Fatalf("load old entry id: %v", err)
	}
	if err := r.DB().Raw(`SELECT id FROM node WHERE name = 'entry-new'`).Scan(&newEntryID).Error; err != nil {
		t.Fatalf("load new entry id: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, inx, ip_preference)
		VALUES('sqlite-tunnel', 1, 1, 'tls', 1, ?, ?, 1, 1, '')
	`, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	var tunnelID int64
	if err := r.DB().Raw(`SELECT id FROM tunnel WHERE name = 'sqlite-tunnel'`).Scan(&tunnelID).Error; err != nil {
		t.Fatalf("load tunnel id: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, inx, protocol)
		VALUES(?, '1', ?, 1, 'tls')
	`, tunnelID, oldEntryID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO forward(user_id, user_name, name, tunnel_id, remote_addr, strategy, created_time, updated_time, status, inx)
		VALUES(1, 'tester', 'forward-a', ?, '127.0.0.1:8080', 'fifo', ?, ?, 1, 1)
	`, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert forward: %v", err)
	}
	var forwardID int64
	if err := r.DB().Raw(`SELECT id FROM forward WHERE name = 'forward-a'`).Scan(&forwardID).Error; err != nil {
		t.Fatalf("load forward id: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO forward_port(forward_id, node_id, port)
		VALUES(?, ?, 12001)
	`, forwardID, oldEntryID).Error; err != nil {
		t.Fatalf("insert forward_port: %v", err)
	}

	tx := r.BeginTx()
	if tx == nil {
		t.Fatal("begin tx: nil transaction")
	}
	if tx.Error != nil {
		t.Fatalf("begin tx: %v", tx.Error)
	}

	errCh := make(chan error, 1)
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		errCh <- h.validateTunnelEntryPortConflictsForNewEntries(tx, tunnelID, []int64{oldEntryID}, []int64{oldEntryID, newEntryID})
	}()

	select {
	case err := <-errCh:
		if err != nil {
			_ = tx.Rollback().Error
			t.Fatalf("unexpected validation error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		_ = tx.Rollback().Error
		<-doneCh
		t.Fatal("validation blocked while transaction was open on sqlite")
	}

	if err := tx.Rollback().Error; err != nil {
		t.Fatalf("rollback tx: %v", err)
	}
}
