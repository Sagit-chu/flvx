package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-backend/internal/http/response"
	"go-backend/internal/store/repo"
)

func TestFederationShareCreateRejectsRemoteNode(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	h := New(r, "test-jwt-secret")
	now := time.Now().UnixMilli()

	if err := r.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx, is_remote, remote_url, remote_token, remote_config)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "remote-share-node", "remote-share-secret", "10.10.10.1", "10.10.10.1", "", "20000-20010", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0, 1, "http://peer.example", "peer-token", `{"shareId":1}`).Error; err != nil {
		t.Fatalf("insert remote node: %v", err)
	}
	remoteNodeID := mustLastInsertID(t, r, "remote-share-node")

	body, err := json.Marshal(createPeerShareRequest{
		Name:           "remote-node-share",
		NodeID:         remoteNodeID,
		MaxBandwidth:   0,
		ExpiryTime:     0,
		PortRangeStart: 20000,
		PortRangeEnd:   20010,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/share/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	h.federationShareCreate(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload response.R
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != -1 {
		t.Fatalf("expected response code -1, got %d", payload.Code)
	}
	if payload.Msg != "Only local nodes can be shared" {
		t.Fatalf("expected rejection message %q, got %q", "Only local nodes can be shared", payload.Msg)
	}

	shareCount := mustQueryInt(t, r, `SELECT COUNT(1) FROM peer_share WHERE node_id = ?`, remoteNodeID)
	if shareCount != 0 {
		t.Fatalf("expected no share rows for remote node, got %d", shareCount)
	}
}

func TestFederationShareCreateRejectsInvalidAllowedIPs(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	h := New(r, "test-jwt-secret")
	now := time.Now().UnixMilli()

	if err := r.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx, is_remote, remote_url, remote_token, remote_config)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "local-share-node", "local-share-secret", "10.20.30.40", "10.20.30.40", "", "21000-21010", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0, 0, "", "", "").Error; err != nil {
		t.Fatalf("insert local node: %v", err)
	}
	localNodeID := mustLastInsertID(t, r, "local-share-node")

	body, err := json.Marshal(createPeerShareRequest{
		Name:           "local-node-share",
		NodeID:         localNodeID,
		MaxBandwidth:   0,
		ExpiryTime:     0,
		PortRangeStart: 21000,
		PortRangeEnd:   21010,
		AllowedIPs:     "bad-ip-entry",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/share/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	h.federationShareCreate(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload response.R
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != -1 {
		t.Fatalf("expected response code -1, got %d", payload.Code)
	}
	if !strings.Contains(payload.Msg, "Invalid allowed IP or CIDR") {
		t.Fatalf("expected invalid IP message, got %q", payload.Msg)
	}

	shareCount := mustQueryInt(t, r, `SELECT COUNT(1) FROM peer_share WHERE node_id = ?`, localNodeID)
	if shareCount != 0 {
		t.Fatalf("expected no share rows for node, got %d", shareCount)
	}
}

func TestFederationShareListIncludesRemoteUsedPorts(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	h := New(r, "test-jwt-secret")
	now := time.Now().UnixMilli()

	if err := r.CreatePeerShare(&repo.PeerShare{
		Name:           "provider-share",
		NodeID:         9,
		Token:          "share-list-token",
		MaxBandwidth:   1024,
		CurrentFlow:    512,
		PortRangeStart: 22000,
		PortRangeEnd:   22010,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	}); err != nil {
		t.Fatalf("create peer share: %v", err)
	}

	share, err := r.GetPeerShareByToken("share-list-token")
	if err != nil || share == nil {
		t.Fatalf("load peer share: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO peer_share_runtime(share_id, node_id, reservation_id, resource_key, binding_id, role, chain_name, service_name, protocol, strategy, port, target, applied, status, created_time, updated_time)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?),
		      (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?),
		      (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		share.ID, share.NodeID, "r-1", "rk-1", "b-1", "middle", "fed_chain_1", "fed_svc_1", "tls", "round", 22001, "", 1, 1, now, now,
		share.ID, share.NodeID, "r-2", "rk-2", "b-2", "exit", "", "fed_svc_2", "tls", "round", 22002, "", 1, 1, now, now,
		share.ID, share.NodeID, "r-3", "rk-3", "", "", "", "", "tls", "round", 22003, "", 0, 0, now, now,
	).Error; err != nil {
		t.Fatalf("insert peer_share_runtime rows: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/share/list", nil)
	res := httptest.NewRecorder()
	h.federationShareList(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload response.R
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != 0 {
		t.Fatalf("expected response code 0, got %d (%s)", payload.Code, payload.Msg)
	}

	rows, ok := payload.Data.([]interface{})
	if !ok || len(rows) == 0 {
		t.Fatalf("expected non-empty share list, got %T", payload.Data)
	}

	first, ok := rows[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected share row object, got %T", rows[0])
	}

	if int(first["activeRuntimeNum"].(float64)) != 2 {
		t.Fatalf("expected activeRuntimeNum=2, got %v", first["activeRuntimeNum"])
	}

	usedPortsRaw, ok := first["usedPorts"].([]interface{})
	if !ok {
		t.Fatalf("expected usedPorts array, got %T", first["usedPorts"])
	}
	if len(usedPortsRaw) != 2 {
		t.Fatalf("expected 2 used ports, got %d", len(usedPortsRaw))
	}
	if int(usedPortsRaw[0].(float64)) != 22001 || int(usedPortsRaw[1].(float64)) != 22002 {
		t.Fatalf("unexpected used ports payload: %v", usedPortsRaw)
	}

	detailsRaw, ok := first["usedPortDetails"].([]interface{})
	if !ok {
		t.Fatalf("expected usedPortDetails array, got %T", first["usedPortDetails"])
	}
	if len(detailsRaw) != 2 {
		t.Fatalf("expected 2 usedPortDetails rows, got %d", len(detailsRaw))
	}
}

func TestFederationShareDeleteCleansUpRuntimes(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	h := New(r, "test-jwt-secret")
	now := time.Now().UnixMilli()

	if err := r.CreatePeerShare(&repo.PeerShare{
		Name:           "delete-cleanup-share",
		NodeID:         99,
		Token:          "delete-cleanup-token",
		MaxBandwidth:   4096,
		PortRangeStart: 40000,
		PortRangeEnd:   40010,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	}); err != nil {
		t.Fatalf("create peer share: %v", err)
	}

	share, err := r.GetPeerShareByToken("delete-cleanup-token")
	if err != nil || share == nil {
		t.Fatalf("load peer share: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO peer_share_runtime(share_id, node_id, reservation_id, resource_key, binding_id, role, chain_name, service_name, protocol, strategy, port, target, applied, status, created_time, updated_time)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?),
		      (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		share.ID, 99, "dc-r1", "dc-rk1", "dc-b1", "exit", "", "fed_svc_dc1", "tls", "round", 40001, "", 1, 1, now, now,
		share.ID, 99, "dc-r2", "dc-rk2", "dc-b2", "middle", "fed_chain_dc2", "fed_svc_dc2", "tls", "round", 40002, "", 1, 1, now, now,
	).Error; err != nil {
		t.Fatalf("insert peer_share_runtime rows: %v", err)
	}

	runtimeCount := mustQueryInt(t, r, `SELECT COUNT(1) FROM peer_share_runtime WHERE share_id = ? AND status = 1`, share.ID)
	if runtimeCount != 2 {
		t.Fatalf("expected 2 active runtimes before delete, got %d", runtimeCount)
	}

	body, err := json.Marshal(deletePeerShareRequest{ID: share.ID})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/share/delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	h.federationShareDelete(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
	var payload response.R
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != 0 {
		t.Fatalf("expected response code 0, got %d (%s)", payload.Code, payload.Msg)
	}

	shareCount := mustQueryInt(t, r, `SELECT COUNT(1) FROM peer_share WHERE id = ?`, share.ID)
	if shareCount != 0 {
		t.Fatalf("expected peer_share deleted, got %d rows", shareCount)
	}

	runtimeCountAfter := mustQueryInt(t, r, `SELECT COUNT(1) FROM peer_share_runtime WHERE share_id = ?`, share.ID)
	if runtimeCountAfter != 0 {
		t.Fatalf("expected all peer_share_runtime rows deleted, got %d", runtimeCountAfter)
	}
}

func TestFederationRemoteUsageListSyncErrorFallback(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	h := New(r, "test-jwt-secret")
	now := time.Now().UnixMilli()

	if err := r.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx, is_remote, remote_url, remote_token, remote_config)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "sync-error-node", "sync-error-secret", "10.50.60.70", "10.50.60.70", "", "32000-32010", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0, 1, "http://unreachable.invalid:9999", "bad-token", `{"shareId":42,"maxBandwidth":5368709120,"currentFlow":999999,"portRangeStart":32000,"portRangeEnd":32010}`).Error; err != nil {
		t.Fatalf("insert remote node: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/share/remote-usage/list", nil)
	res := httptest.NewRecorder()
	h.federationRemoteUsageList(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload response.R
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != 0 {
		t.Fatalf("expected response code 0, got %d (%s)", payload.Code, payload.Msg)
	}

	rows, ok := payload.Data.([]interface{})
	if !ok || len(rows) == 0 {
		t.Fatalf("expected non-empty usage list, got %T", payload.Data)
	}

	first, ok := rows[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected row map, got %T", rows[0])
	}

	if int64(first["shareId"].(float64)) != 42 {
		t.Fatalf("expected stale shareId=42 on sync failure, got %v", first["shareId"])
	}
	if int64(first["currentFlow"].(float64)) != 999999 {
		t.Fatalf("expected stale currentFlow=999999 on sync failure, got %v", first["currentFlow"])
	}

	syncErr, _ := first["syncError"].(string)
	if syncErr == "" {
		t.Fatalf("expected non-empty syncError field on unreachable provider")
	}
}

func TestFederationShareResetFlow(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	h := New(r, "test-jwt-secret")
	now := time.Now().UnixMilli()
	if err := r.CreatePeerShare(&repo.PeerShare{
		Name:           "reset-flow-share",
		NodeID:         11,
		Token:          "reset-flow-token",
		MaxBandwidth:   4096,
		CurrentFlow:    2048,
		PortRangeStart: 23000,
		PortRangeEnd:   23010,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	}); err != nil {
		t.Fatalf("create peer share: %v", err)
	}
	share, err := r.GetPeerShareByToken("reset-flow-token")
	if err != nil || share == nil {
		t.Fatalf("load peer share: %v", err)
	}

	body, err := json.Marshal(resetPeerShareFlowRequest{ID: share.ID})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/share/reset-flow", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	h.federationShareResetFlow(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
	var payload response.R
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != 0 {
		t.Fatalf("expected response code 0, got %d (%s)", payload.Code, payload.Msg)
	}

	updated, err := r.GetPeerShare(share.ID)
	if err != nil || updated == nil {
		t.Fatalf("reload peer share: %v", err)
	}
	if updated.CurrentFlow != 0 {
		t.Fatalf("expected current flow reset to 0, got %d", updated.CurrentFlow)
	}
}

func TestFederationTunnelCreateCreatesPeerShareRuntime(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	h := New(r, "test-jwt-secret")
	now := time.Now().UnixMilli()

	if err := r.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx, is_remote, remote_url, remote_token, remote_config)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "federation-forward-node", "federation-forward-secret", "10.90.80.70", "10.90.80.70", "", "24000-24020", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0, 0, "", "", "").Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	nodeID := mustLastInsertID(t, r, "federation-forward-node")

	if err := r.CreatePeerShare(&repo.PeerShare{
		Name:           "federation-forward-share",
		NodeID:         nodeID,
		Token:          "federation-forward-token",
		MaxBandwidth:   0,
		CurrentFlow:    0,
		PortRangeStart: 24000,
		PortRangeEnd:   24020,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	}); err != nil {
		t.Fatalf("create share: %v", err)
	}
	share, err := r.GetPeerShareByToken("federation-forward-token")
	if err != nil || share == nil {
		t.Fatalf("load share: %v", err)
	}

	body, err := json.Marshal(federationTunnelRequest{
		Protocol:   "tcp",
		RemotePort: 24001,
		Target:     "1.1.1.1:443",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/tunnel/create", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+share.Token)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	h.federationTunnelCreate(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload response.R
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != 0 {
		t.Fatalf("expected response code 0, got %d (%s)", payload.Code, payload.Msg)
	}

	runtimeCount := mustQueryInt(t, r, `SELECT COUNT(1) FROM peer_share_runtime WHERE share_id = ? AND port = ? AND status = 1`, share.ID, 24001)
	if runtimeCount != 1 {
		t.Fatalf("expected 1 runtime row for new federation forward tunnel, got %d", runtimeCount)
	}
}

func TestFederationTunnelCreateRejectsOccupiedPort(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	h := New(r, "test-jwt-secret")
	now := time.Now().UnixMilli()

	if err := r.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx, is_remote, remote_url, remote_token, remote_config)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "federation-port-check-node", "federation-port-check-secret", "10.91.80.70", "10.91.80.70", "", "24100-24120", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0, 0, "", "", "").Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	nodeID := mustLastInsertID(t, r, "federation-port-check-node")

	if err := r.CreatePeerShare(&repo.PeerShare{
		Name:           "federation-port-check-share",
		NodeID:         nodeID,
		Token:          "federation-port-check-token",
		MaxBandwidth:   0,
		CurrentFlow:    0,
		PortRangeStart: 24100,
		PortRangeEnd:   24120,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	}); err != nil {
		t.Fatalf("create share: %v", err)
	}

	create := func() response.R {
		body, err := json.Marshal(federationTunnelRequest{Protocol: "tcp", RemotePort: 24101, Target: "1.1.1.1:443"})
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/tunnel/create", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer federation-port-check-token")
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		h.federationTunnelCreate(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
		}
		var payload response.R
		if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return payload
	}

	first := create()
	if first.Code != 0 {
		t.Fatalf("expected first create success, got %d (%s)", first.Code, first.Msg)
	}

	second := create()
	if second.Code != 403 {
		t.Fatalf("expected second create to be rejected with 403, got %d (%s)", second.Code, second.Msg)
	}
	if second.Msg != "Port already in use" {
		t.Fatalf("expected occupied port message, got %q", second.Msg)
	}
}

func TestDeleteTunnelReleasesFederationForwardRuntimeByPort(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	h := New(r, "test-jwt-secret")
	now := time.Now().UnixMilli()

	if err := r.CreatePeerShare(&repo.PeerShare{
		Name:           "delete-forward-share",
		NodeID:         1,
		Token:          "delete-forward-token",
		MaxBandwidth:   0,
		CurrentFlow:    0,
		PortRangeStart: 25000,
		PortRangeEnd:   25020,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	}); err != nil {
		t.Fatalf("create share: %v", err)
	}
	share, err := r.GetPeerShareByToken("delete-forward-token")
	if err != nil || share == nil {
		t.Fatalf("load share: %v", err)
	}

	tunnelName := fmt.Sprintf("Share-%d-Port-%d", share.ID, 25001)
	if err := r.DB().Exec(`
		INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, 1, tunnelName, 1.0, 1, "tcp", 1, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO peer_share_runtime(share_id, node_id, reservation_id, resource_key, binding_id, role, chain_name, service_name, protocol, strategy, port, target, applied, status, created_time, updated_time)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, share.ID, share.NodeID, "del-r1", "del-rk1", "", "forward", "", "20_2_10", "tcp", "fifo", 25001, "", 1, 1, now, now).Error; err != nil {
		t.Fatalf("insert runtime: %v", err)
	}

	if err := h.deleteTunnelByID(1); err != nil {
		t.Fatalf("delete tunnel: %v", err)
	}

	activeCount := mustQueryInt(t, r, `SELECT COUNT(1) FROM peer_share_runtime WHERE share_id = ? AND port = ? AND status = 1`, share.ID, 25001)
	if activeCount != 0 {
		t.Fatalf("expected runtime released after tunnel delete, active rows=%d", activeCount)
	}
}

func TestBindPeerShareForwardRuntimeServicesOnlyBindsForwardRole(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	h := New(r, "test-jwt-secret")
	now := time.Now().UnixMilli()

	if err := r.CreatePeerShare(&repo.PeerShare{
		Name:           "bind-forward-role-share",
		NodeID:         1,
		Token:          "bind-forward-role-token",
		MaxBandwidth:   0,
		CurrentFlow:    0,
		PortRangeStart: 26000,
		PortRangeEnd:   26020,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	}); err != nil {
		t.Fatalf("create share: %v", err)
	}
	share, err := r.GetPeerShareByToken("bind-forward-role-token")
	if err != nil || share == nil {
		t.Fatalf("load share: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO peer_share_runtime(id, share_id, node_id, reservation_id, resource_key, binding_id, role, chain_name, service_name, protocol, strategy, port, target, applied, status, created_time, updated_time)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?),
		      (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		1, share.ID, share.NodeID, "bind-r1", "bind-rk1", "", "forward", "", "", "tcp", "fifo", 26001, "", 0, 1, now, now,
		2, share.ID, share.NodeID, "bind-r2", "bind-rk2", "", "middle", "", "", "tcp", "round", 26002, "", 0, 1, now, now,
	).Error; err != nil {
		t.Fatalf("insert runtimes: %v", err)
	}

	h.bindPeerShareForwardRuntimeServices(share, map[string]interface{}{
		"services": []interface{}{
			map[string]interface{}{"name": "77_2_10_tcp", "addr": "[::]:26001"},
			map[string]interface{}{"name": "88_2_10_tcp", "addr": "[::]:26002"},
		},
	})

	forwardServiceName := ""
	middleServiceName := ""
	if err := r.DB().Raw(`SELECT service_name FROM peer_share_runtime WHERE id = 1`).Scan(&forwardServiceName).Error; err != nil {
		t.Fatalf("load forward runtime service name: %v", err)
	}
	if err := r.DB().Raw(`SELECT service_name FROM peer_share_runtime WHERE id = 2`).Scan(&middleServiceName).Error; err != nil {
		t.Fatalf("load middle runtime service name: %v", err)
	}

	if forwardServiceName != "77_2_10" {
		t.Fatalf("expected forward runtime service name bound, got %q", forwardServiceName)
	}
	if middleServiceName != "" {
		t.Fatalf("expected non-forward runtime unchanged, got %q", middleServiceName)
	}
}

func TestBindPeerShareForwardRuntimeServicesAcceptsTopLevelServiceArray(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	h := New(r, "test-jwt-secret")
	now := time.Now().UnixMilli()

	if err := r.CreatePeerShare(&repo.PeerShare{
		Name:           "bind-array-share",
		NodeID:         1,
		Token:          "bind-array-token",
		MaxBandwidth:   0,
		CurrentFlow:    0,
		PortRangeStart: 26100,
		PortRangeEnd:   26120,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	}); err != nil {
		t.Fatalf("create share: %v", err)
	}
	share, err := r.GetPeerShareByToken("bind-array-token")
	if err != nil || share == nil {
		t.Fatalf("load share: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO peer_share_runtime(id, share_id, node_id, reservation_id, resource_key, binding_id, role, chain_name, service_name, protocol, strategy, port, target, applied, status, created_time, updated_time)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		1, share.ID, share.NodeID, "bind-array-r1", "bind-array-rk1", "", "forward", "", "", "tcp", "fifo", 26101, "", 0, 1, now, now,
	).Error; err != nil {
		t.Fatalf("insert runtime: %v", err)
	}

	h.bindPeerShareForwardRuntimeServices(share, []interface{}{
		map[string]interface{}{"name": "99_2_10_tcp", "addr": "[::]:26101"},
		map[string]interface{}{"name": "99_2_10_udp", "addr": "[::]:26101"},
	})

	forwardServiceName := ""
	if err := r.DB().Raw(`SELECT service_name FROM peer_share_runtime WHERE id = 1`).Scan(&forwardServiceName).Error; err != nil {
		t.Fatalf("load forward runtime service name: %v", err)
	}
	if forwardServiceName != "99_2_10" {
		t.Fatalf("expected forward runtime service name bound from top-level array, got %q", forwardServiceName)
	}
}

func TestBindPeerShareForwardRuntimeServicesCreatesRuntimeWhenMissing(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel-bind-create-runtime.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	h := New(r, "test-jwt-secret")
	now := time.Now().UnixMilli()

	if err := r.CreatePeerShare(&repo.PeerShare{
		Name:           "bind-create-runtime-share",
		NodeID:         1,
		Token:          "bind-create-runtime-token",
		MaxBandwidth:   0,
		CurrentFlow:    0,
		PortRangeStart: 26300,
		PortRangeEnd:   26320,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	}); err != nil {
		t.Fatalf("create share: %v", err)
	}
	share, err := r.GetPeerShareByToken("bind-create-runtime-token")
	if err != nil || share == nil {
		t.Fatalf("load share: %v", err)
	}

	h.bindPeerShareForwardRuntimeServices(share, map[string]interface{}{
		"services": []interface{}{
			map[string]interface{}{"name": "55_2_10_tcp", "addr": "[::]:26301"},
		},
	})

	var count int64
	if err := r.DB().Raw(`SELECT COUNT(1) FROM peer_share_runtime WHERE share_id = ? AND role = ? AND status = 1`, share.ID, "forward").Scan(&count).Error; err != nil {
		t.Fatalf("query runtime count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 active forward runtime row, got %d", count)
	}

	var serviceName string
	var port int
	var applied int
	if err := r.DB().Raw(`SELECT service_name, port, applied FROM peer_share_runtime WHERE share_id = ? AND role = ? ORDER BY id DESC LIMIT 1`, share.ID, "forward").Row().Scan(&serviceName, &port, &applied); err != nil {
		t.Fatalf("query created runtime: %v", err)
	}
	if serviceName != "55_2_10" {
		t.Fatalf("expected service_name=55_2_10, got %q", serviceName)
	}
	if port != 26301 {
		t.Fatalf("expected port=26301, got %d", port)
	}
	if applied != 1 {
		t.Fatalf("expected applied=1, got %d", applied)
	}
}

func TestReleasePeerShareForwardRuntimeServicesMarksRuntimeReleased(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel-release-runtime.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	h := New(r, "test-jwt-secret")
	now := time.Now().UnixMilli()

	if err := r.CreatePeerShare(&repo.PeerShare{
		Name:           "release-runtime-share",
		NodeID:         1,
		Token:          "release-runtime-token",
		MaxBandwidth:   0,
		CurrentFlow:    0,
		PortRangeStart: 26400,
		PortRangeEnd:   26420,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	}); err != nil {
		t.Fatalf("create share: %v", err)
	}
	share, err := r.GetPeerShareByToken("release-runtime-token")
	if err != nil || share == nil {
		t.Fatalf("load share: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO peer_share_runtime(share_id, node_id, reservation_id, resource_key, binding_id, role, chain_name, service_name, protocol, strategy, port, target, applied, status, created_time, updated_time)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, share.ID, share.NodeID, "release-r1", "release-rk1", "", "forward", "", "77_2_10", "tcp", "fifo", 26401, "", 1, 1, now, now).Error; err != nil {
		t.Fatalf("insert runtime: %v", err)
	}

	h.releasePeerShareForwardRuntimeServices(share, map[string]interface{}{
		"services": []interface{}{"77_2_10_tcp"},
	})

	var status int
	var applied int
	var serviceName string
	if err := r.DB().Raw(`SELECT status, applied, service_name FROM peer_share_runtime WHERE share_id = ? AND role = ? ORDER BY id DESC LIMIT 1`, share.ID, "forward").Row().Scan(&status, &applied, &serviceName); err != nil {
		t.Fatalf("query released runtime: %v", err)
	}
	if status != 0 {
		t.Fatalf("expected status=0 after release, got %d", status)
	}
	if applied != 0 {
		t.Fatalf("expected applied=0 after release, got %d", applied)
	}
	if serviceName != "" {
		t.Fatalf("expected service_name cleared after release, got %q", serviceName)
	}
}

func TestValidateFederationCommandPortsAcceptsTopLevelServiceArray(t *testing.T) {
	share := &repo.PeerShare{
		PortRangeStart: 26200,
		PortRangeEnd:   26210,
	}
	err := validateFederationCommandPorts(share, []interface{}{
		map[string]interface{}{"name": "11_2_10_tcp", "addr": "[::]:26201"},
		map[string]interface{}{"name": "11_2_10_udp", "addr": "[::]:26201"},
	})
	if err != nil {
		t.Fatalf("expected top-level service array to pass port validation, got: %v", err)
	}
}

func TestFederationRemoteUsageList(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	h := New(r, "test-jwt-secret")
	now := time.Now().UnixMilli()

	if err := r.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx, is_remote, remote_url, remote_token, remote_config)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "remote-consumer-node", "remote-consumer-secret", "10.30.40.50", "10.30.40.50", "", "31000-31010", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0, 1, "http://peer.example", "peer-token", `{"shareId":88,"maxBandwidth":2147483648,"currentFlow":1073741824,"portRangeStart":31000,"portRangeEnd":31010}`).Error; err != nil {
		t.Fatalf("insert remote node: %v", err)
	}
	nodeID := mustLastInsertID(t, r, "remote-consumer-node")

	if err := r.DB().Exec(`INSERT INTO tunnel(name, type, protocol, flow, created_time, updated_time, status, in_ip, inx) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, "consumer-tunnel-a", 2, "tls", 1, now, now, 1, "", 0).Error; err != nil {
		t.Fatalf("insert tunnel a: %v", err)
	}
	tunnelAID := mustLastInsertID(t, r, "consumer-tunnel-a")

	if err := r.DB().Exec(`INSERT INTO tunnel(name, type, protocol, flow, created_time, updated_time, status, in_ip, inx) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, "consumer-tunnel-b", 2, "tls", 1, now, now, 1, "", 0).Error; err != nil {
		t.Fatalf("insert tunnel b: %v", err)
	}
	tunnelBID := mustLastInsertID(t, r, "consumer-tunnel-b")

	if err := r.DB().Exec(`
		INSERT INTO federation_tunnel_binding(tunnel_id, node_id, chain_type, hop_inx, remote_url, resource_key, remote_binding_id, allocated_port, status, created_time, updated_time)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?),
		      (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		tunnelAID, nodeID, 2, 1, "http://peer.example", "rk-a", "rb-a", 31001, 1, now, now,
		tunnelBID, nodeID, 3, 0, "http://peer.example", "rk-b", "rb-b", 31002, 1, now, now,
	).Error; err != nil {
		t.Fatalf("insert federation bindings: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/share/remote-usage/list", nil)
	res := httptest.NewRecorder()
	h.federationRemoteUsageList(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload response.R
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != 0 {
		t.Fatalf("expected response code 0, got %d (%s)", payload.Code, payload.Msg)
	}

	rows, ok := payload.Data.([]interface{})
	if !ok || len(rows) == 0 {
		t.Fatalf("expected non-empty usage list, got %T", payload.Data)
	}

	first, ok := rows[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected first usage row map, got %T", rows[0])
	}
	if int64(first["shareId"].(float64)) != 88 {
		t.Fatalf("expected shareId=88, got %v", first["shareId"])
	}

	usedPortsRaw, ok := first["usedPorts"].([]interface{})
	if !ok {
		t.Fatalf("expected usedPorts array, got %T", first["usedPorts"])
	}
	if len(usedPortsRaw) != 2 {
		t.Fatalf("expected 2 used ports, got %d", len(usedPortsRaw))
	}
	if int(usedPortsRaw[0].(float64)) != 31001 || int(usedPortsRaw[1].(float64)) != 31002 {
		t.Fatalf("unexpected used ports payload: %v", usedPortsRaw)
	}

	bindingsRaw, ok := first["bindings"].([]interface{})
	if !ok {
		t.Fatalf("expected bindings array, got %T", first["bindings"])
	}
	if len(bindingsRaw) != 2 {
		t.Fatalf("expected 2 binding rows, got %d", len(bindingsRaw))
	}
}

func TestAuthPeerAllowedIPs(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	h := New(r, "test-jwt-secret")
	now := time.Now().UnixMilli()

	tests := []struct {
		name        string
		allowedIPs  string
		remoteAddr  string
		xff         string
		wantAllowed bool
	}{
		{
			name:        "exact ip allowed",
			allowedIPs:  "203.0.113.10",
			remoteAddr:  "203.0.113.10:23456",
			wantAllowed: true,
		},
		{
			name:        "cidr allowed",
			allowedIPs:  "203.0.113.0/24",
			remoteAddr:  "203.0.113.11:23456",
			wantAllowed: true,
		},
		{
			name:        "trusted proxy xff allowed",
			allowedIPs:  "198.51.100.20",
			remoteAddr:  "172.20.0.3:34567",
			xff:         "198.51.100.20, 172.20.0.3",
			wantAllowed: true,
		},
		{
			name:        "ipv4-mapped proxy xff allowed",
			allowedIPs:  "198.51.100.20",
			remoteAddr:  "[::ffff:172.20.0.3]:34567",
			xff:         "198.51.100.20, 172.20.0.3",
			wantAllowed: true,
		},
		{
			name:        "non whitelisted ip denied",
			allowedIPs:  "203.0.113.10",
			remoteAddr:  "203.0.113.99:23456",
			wantAllowed: false,
		},
	}

	for idx, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := fmt.Sprintf("share-token-%d", idx)
			if err := r.CreatePeerShare(&repo.PeerShare{
				Name:           "share-" + tt.name,
				NodeID:         1,
				Token:          token,
				PortRangeStart: 10000,
				PortRangeEnd:   10010,
				IsActive:       1,
				CreatedTime:    now,
				UpdatedTime:    now,
				AllowedIPs:     tt.allowedIPs,
			}); err != nil {
				t.Fatalf("create peer share: %v", err)
			}

			nextCalled := false
			wrapped := h.authPeer(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				response.WriteJSON(w, response.OKEmpty())
			})

			req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/connect", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			req.RemoteAddr = tt.remoteAddr

			res := httptest.NewRecorder()
			wrapped(res, req)

			var payload response.R
			if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
				t.Fatalf("decode response: %v", err)
			}

			if tt.wantAllowed {
				if !nextCalled {
					t.Fatalf("expected next handler to be called")
				}
				if payload.Code != 0 {
					t.Fatalf("expected code 0, got %d (%s)", payload.Code, payload.Msg)
				}
				return
			}

			if nextCalled {
				t.Fatalf("expected next handler not to be called")
			}
			if payload.Code != 403 {
				t.Fatalf("expected code 403, got %d (%s)", payload.Code, payload.Msg)
			}
			if payload.Msg != "IP not allowed" {
				t.Fatalf("expected IP rejection message, got %q", payload.Msg)
			}
		})
	}
}
