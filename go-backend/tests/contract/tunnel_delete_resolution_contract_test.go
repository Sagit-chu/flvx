package contract_test

import (
	"testing"
	"time"

	"go-backend/internal/http/response"
	storeRepo "go-backend/internal/store/repo"
)

func TestTunnelDeletePreviewIncludesDependentRulesContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	adminToken := mustAdminToken(t, secret)
	now := time.Now().UnixMilli()

	sourceTunnelID, sourceNodeID := seedTunnelDeleteTunnelWithNode(t, repo, now, "preview-source-tunnel", "preview-source-node", "21000-21010")
	seedTunnelDeleteForward(t, repo, now, sourceTunnelID, sourceNodeID, "preview-forward", 21001)

	out := requestContractEnvelope(t, router, adminToken, "/api/v1/tunnel/delete-preview", map[string]interface{}{"id": sourceTunnelID})
	if out.Code != 0 {
		t.Fatalf("expected success, got code=%d msg=%q", out.Code, out.Msg)
	}

	data, ok := out.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected preview data object, got %T", out.Data)
	}
	if contractValueAsInt64(data["tunnelId"]) != sourceTunnelID {
		t.Fatalf("unexpected tunnelId: %#v", data["tunnelId"])
	}
	if contractValueAsInt64(data["forwardCount"]) != 1 {
		t.Fatalf("expected forwardCount=1, got %#v", data["forwardCount"])
	}

	samples, ok := data["sampleForwards"].([]interface{})
	if !ok || len(samples) != 1 {
		t.Fatalf("expected one sample forward, got %#v", data["sampleForwards"])
	}
	first, ok := samples[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected sample object, got %T", samples[0])
	}
	if first["name"] != "preview-forward" {
		t.Fatalf("unexpected sample name: %#v", first["name"])
	}
	if contractValueAsInt64(first["inPort"]) != 21001 {
		t.Fatalf("unexpected sample inPort: %#v", first["inPort"])
	}
}

func TestTunnelDeleteWithForwardsDeleteActionRemovesTunnelAndRulesContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	adminToken := mustAdminToken(t, secret)
	now := time.Now().UnixMilli()

	sourceTunnelID, sourceNodeID := seedTunnelDeleteTunnelWithNode(t, repo, now, "delete-source-tunnel", "delete-source-node", "22000-22010")
	forwardID := seedTunnelDeleteForward(t, repo, now, sourceTunnelID, sourceNodeID, "delete-forward", 22001)

	out := requestContractEnvelope(t, router, adminToken, "/api/v1/tunnel/delete-with-forwards", map[string]interface{}{
		"id":     sourceTunnelID,
		"action": "delete_forwards",
	})
	if out.Code != 0 {
		t.Fatalf("expected success, got code=%d msg=%q", out.Code, out.Msg)
	}

	if count := mustQueryInt(t, repo, `SELECT COUNT(1) FROM tunnel WHERE id = ?`, sourceTunnelID); count != 0 {
		t.Fatalf("expected tunnel deleted, got count=%d", count)
	}
	if count := mustQueryInt(t, repo, `SELECT COUNT(1) FROM forward WHERE id = ?`, forwardID); count != 0 {
		t.Fatalf("expected forward deleted, got count=%d", count)
	}
	if count := mustQueryInt(t, repo, `SELECT COUNT(1) FROM forward_port WHERE forward_id = ?`, forwardID); count != 0 {
		t.Fatalf("expected forward ports deleted, got count=%d", count)
	}
}

func TestTunnelDeleteWithForwardsReplaceReturnsFailureDetailsContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	adminToken := mustAdminToken(t, secret)
	now := time.Now().UnixMilli()

	sourceTunnelID, sourceNodeID := seedTunnelDeleteTunnelWithNode(t, repo, now, "replace-source-tunnel", "replace-source-node", "23000-23010")
	forwardID := seedTunnelDeleteForward(t, repo, now, sourceTunnelID, sourceNodeID, "replace-forward", 23001)
	targetTunnelID, targetNodeID := seedTunnelDeleteTunnelWithNode(t, repo, now, "replace-target-tunnel", "replace-target-node", "23000-23010")
	seedTunnelDeleteForward(t, repo, now, targetTunnelID, targetNodeID, "occupied-forward", 23001)

	out := requestContractEnvelope(t, router, adminToken, "/api/v1/tunnel/delete-with-forwards", map[string]interface{}{
		"id":             sourceTunnelID,
		"action":         "replace",
		"targetTunnelId": targetTunnelID,
	})
	if out.Code != -2 {
		t.Fatalf("expected failure code -2, got code=%d msg=%q", out.Code, out.Msg)
	}

	result := mustTunnelDeleteFailureResult(t, out)
	if contractValueAsInt64(result["failCount"]) != 1 {
		t.Fatalf("expected failCount=1, got %#v", result["failCount"])
	}
	assertBatchFailureNameAndReason(t, result, "replace-forward", "节点 replace-target-node 端口 23001 已被其他转发占用")

	if count := mustQueryInt(t, repo, `SELECT COUNT(1) FROM tunnel WHERE id = ?`, sourceTunnelID); count != 1 {
		t.Fatalf("expected source tunnel kept, got count=%d", count)
	}
	if tunnelAfter := mustQueryInt64(t, repo, `SELECT tunnel_id FROM forward WHERE id = ?`, forwardID); tunnelAfter != sourceTunnelID {
		t.Fatalf("expected forward tunnel unchanged, got %d", tunnelAfter)
	}
}

func TestTunnelBatchDeletePreviewIncludesTotalsContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	adminToken := mustAdminToken(t, secret)
	now := time.Now().UnixMilli()

	tunnelA, nodeA := seedTunnelDeleteTunnelWithNode(t, repo, now, "batch-preview-a", "batch-preview-node-a", "24000-24010")
	tunnelB, _ := seedTunnelDeleteTunnelWithNode(t, repo, now, "batch-preview-b", "batch-preview-node-b", "24100-24110")
	seedTunnelDeleteForward(t, repo, now, tunnelA, nodeA, "batch-preview-forward", 24001)

	out := requestContractEnvelope(t, router, adminToken, "/api/v1/tunnel/batch-delete-preview", map[string]interface{}{
		"ids": []int64{tunnelA, tunnelB},
	})
	if out.Code != 0 {
		t.Fatalf("expected success, got code=%d msg=%q", out.Code, out.Msg)
	}
	data, ok := out.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected preview object, got %T", out.Data)
	}
	if contractValueAsInt64(data["tunnelCount"]) != 2 {
		t.Fatalf("expected tunnelCount=2, got %#v", data["tunnelCount"])
	}
	if contractValueAsInt64(data["totalForwardCount"]) != 1 {
		t.Fatalf("expected totalForwardCount=1, got %#v", data["totalForwardCount"])
	}
}

func TestTunnelBatchDeleteWithForwardsReturnsTunnelLevelFailuresContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	adminToken := mustAdminToken(t, secret)
	now := time.Now().UnixMilli()

	sourceTunnelA, _ := seedTunnelDeleteTunnelWithNode(t, repo, now, "batch-replace-source-a", "batch-replace-source-node-a", "25000-25010")
	sourceTunnelB, sourceNodeB := seedTunnelDeleteTunnelWithNode(t, repo, now, "batch-replace-source-b", "batch-replace-source-node-b", "25100-25110")
	targetTunnelID, targetNodeID := seedTunnelDeleteTunnelWithNode(t, repo, now, "batch-replace-target", "batch-replace-target-node", "25000-25010")

	seedTunnelDeleteForward(t, repo, now, sourceTunnelB, sourceNodeB, "batch-replace-forward-b", 25002)
	seedTunnelDeleteForward(t, repo, now, targetTunnelID, targetNodeID, "batch-replace-occupied", 25002)

	out := requestContractEnvelope(t, router, adminToken, "/api/v1/tunnel/batch-delete-with-forwards", map[string]interface{}{
		"ids":            []int64{sourceTunnelA, sourceTunnelB},
		"action":         "replace",
		"targetTunnelId": targetTunnelID,
	})
	if out.Code != 0 {
		t.Fatalf("expected success envelope, got code=%d msg=%q", out.Code, out.Msg)
	}

	result := mustTunnelDeleteFailureResult(t, out)
	if contractValueAsInt64(result["successCount"]) != 1 {
		t.Fatalf("expected successCount=1, got %#v", result["successCount"])
	}
	if contractValueAsInt64(result["failCount"]) != 1 {
		t.Fatalf("expected failCount=1, got %#v", result["failCount"])
	}
	assertBatchFailureNameAndReason(t, result, "batch-replace-source-b", "batch-replace-forward-b: 节点 batch-replace-target-node 端口 25002 已被其他转发占用")

	if count := mustQueryInt(t, repo, `SELECT COUNT(1) FROM tunnel WHERE id = ?`, sourceTunnelA); count != 0 {
		t.Fatalf("expected source tunnel A deleted, got count=%d", count)
	}
	if count := mustQueryInt(t, repo, `SELECT COUNT(1) FROM tunnel WHERE id = ?`, sourceTunnelB); count != 1 {
		t.Fatalf("expected source tunnel B kept, got count=%d", count)
	}
}

func seedTunnelDeleteTunnelWithNode(t *testing.T, repo *storeRepo.Repository, now int64, tunnelName, nodeName, portRange string) (int64, int64) {
	t.Helper()

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, status, created_time, updated_time, in_ip, inx, ip_preference)
		VALUES(?, 1.0, 1, 'tls', 1, 1, ?, ?, NULL, 0, '')
	`, tunnelName, now, now).Error; err != nil {
		t.Fatalf("insert tunnel %s: %v", tunnelName, err)
	}
	tunnelID := mustLastInsertID(t, repo, tunnelName)

	if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, '10.0.0.1', '10.0.0.1', '', ?, '', 'v1', 1, 1, 1, ?, ?, 1, '[::]', '[::]', 0)
	`, nodeName, nodeName+"-secret", portRange, now, now).Error; err != nil {
		t.Fatalf("insert node %s: %v", nodeName, err)
	}
	nodeID := mustLastInsertID(t, repo, nodeName)

	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 0, 'round', 1, 'tls')
	`, tunnelID, nodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel for %s: %v", tunnelName, err)
	}

	return tunnelID, nodeID
}

func seedTunnelDeleteForward(t *testing.T, repo *storeRepo.Repository, now int64, tunnelID, nodeID int64, forwardName string, port int) int64 {
	t.Helper()

	if err := repo.DB().Exec(`
		INSERT INTO forward(user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(2, 'contract-user', ?, ?, '1.1.1.1:443', 'fifo', 0, 0, ?, ?, 1, 0)
	`, forwardName, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert forward %s: %v", forwardName, err)
	}
	forwardID := mustLastInsertID(t, repo, forwardName)

	if err := repo.DB().Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(?, ?, ?)`, forwardID, nodeID, port).Error; err != nil {
		t.Fatalf("insert forward_port for %s: %v", forwardName, err)
	}

	return forwardID
}

func mustTunnelDeleteFailureResult(t *testing.T, out response.R) map[string]interface{} {
	t.Helper()
	result, ok := out.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected result object, got %T", out.Data)
	}
	return result
}
