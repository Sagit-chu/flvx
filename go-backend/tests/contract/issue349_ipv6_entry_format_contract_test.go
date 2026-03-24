package contract_test

import (
	"testing"
	"time"
)

func TestIssue349_ForwardListFormatsIPv6EntryAddressesContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	adminToken := mustAdminToken(t, secret)
	now := time.Now().UnixMilli()

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "issue349-tunnel", 1.0, 1, "tcp", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, repo, "issue349-tunnel")

	if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "issue349-entry-node-a", "entry-secret-a", "2001:db8::10", "", "2001:db8::10", "32000-32010", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0).Error; err != nil {
		t.Fatalf("insert node a: %v", err)
	}
	nodeAID := mustLastInsertID(t, repo, "issue349-entry-node-a")

	if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "issue349-entry-node-b", "entry-secret-b", "2001:db8::30", "", "2001:db8::30", "32000-32010", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 1).Error; err != nil {
		t.Fatalf("insert node b: %v", err)
	}
	nodeBID := mustLastInsertID(t, repo, "issue349-entry-node-b")

	if err := repo.DB().Exec(`
		INSERT INTO forward(user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(?, ?, ?, ?, ?, ?, 0, 0, ?, ?, 1, ?)
	`, 1, "admin_user", "issue349-forward", tunnelID, "1.1.1.1:443", "fifo", now, now, 0).Error; err != nil {
		t.Fatalf("insert forward: %v", err)
	}
	forwardID := mustLastInsertID(t, repo, "issue349-forward")

	if err := repo.DB().Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(?, ?, ?)`, forwardID, nodeAID, 32001).Error; err != nil {
		t.Fatalf("insert forward_port a: %v", err)
	}
	if err := repo.DB().Exec(`INSERT INTO forward_port(forward_id, node_id, port, in_ip) VALUES(?, ?, ?, ?)`, forwardID, nodeBID, 32002, "2001:db8::20").Error; err != nil {
		t.Fatalf("insert forward_port b: %v", err)
	}

	out := requestContractEnvelope(t, router, adminToken, "/api/v1/forward/list", nil)
	if out.Code != 0 {
		t.Fatalf("forward list failed: code=%d msg=%q", out.Code, out.Msg)
	}

	rows := mustContractSlice(t, out.Data, "forward list data")
	var target map[string]interface{}
	for _, row := range rows {
		item, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		if contractValueAsInt64(item["id"]) == forwardID {
			target = item
			break
		}
	}
	if target == nil {
		t.Fatalf("target forward %d not found in /forward/list response", forwardID)
	}

	if got := contractValueAsString(target["inIp"]); got != "[2001:db8::10]:32001,[2001:db8::20]:32002" {
		t.Fatalf("expected bracketed IPv6 entry list, got %q", got)
	}
	if got := contractValueAsInt64(target["inPort"]); got != 32001 {
		t.Fatalf("expected first entry port 32001, got %d", got)
	}
}
