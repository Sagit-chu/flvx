package contract_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"go-backend/internal/auth"
	"go-backend/internal/http/response"
)

func TestTunnelCreateRuntimeRollbackContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	insertNode := func(name, ip, portRange string) int64 {
		if err := repo.DB().Exec(`
			INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, name, name+"-secret", ip, ip, "", portRange, "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0).Error; err != nil {
			t.Fatalf("insert node %s: %v", name, err)
		}
		return mustLastInsertID(t, repo, name)
	}

	entryID := insertNode("create-entry", "10.20.0.1", "30000-30010")
	chainID := insertNode("create-chain", "10.20.0.2", "31000-31010")
	exitID := insertNode("create-exit", "10.20.0.3", "32000-32010")

	payload := `{"name":"runtime-rollback-tunnel","type":2,"flow":99999,"status":1,"inNodeId":[{"nodeId":` + jsonInt(entryID) + `,"protocol":"tls"}],"chainNodes":[[{"nodeId":` + jsonInt(chainID) + `,"protocol":"tls","strategy":"round"}]],"outNodeId":[{"nodeId":` + jsonInt(exitID) + `,"protocol":"tls"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/create", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected create success (runtime deferred when nodes offline), got code=%d msg=%q", out.Code, out.Msg)
	}

	tunnelCount := mustQueryInt(t, repo, `SELECT COUNT(1) FROM tunnel WHERE name = ?`, "runtime-rollback-tunnel")
	if tunnelCount != 1 {
		t.Fatalf("expected tunnel record preserved (runtime deferred), found %d records", tunnelCount)
	}

	chainCount := mustQueryInt(t, repo, `SELECT COUNT(1) FROM chain_tunnel`)
	if chainCount != 3 {
		t.Fatalf("expected 3 chain_tunnel records preserved (in/chain/out), found %d records", chainCount)
	}
}

func TestTunnelUpdateAssignsChainPortsContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	insertNode := func(name, ip, portRange string) int64 {
		if err := repo.DB().Exec(`
			INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, name, name+"-secret", ip, ip, "", portRange, "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0).Error; err != nil {
			t.Fatalf("insert node %s: %v", name, err)
		}
		return mustLastInsertID(t, repo, name)
	}

	entryID := insertNode("update-entry", "10.30.0.1", "40000-40010")
	chainID := insertNode("update-chain", "10.30.0.2", "41000-41010")
	exitID := insertNode("update-exit", "10.30.0.3", "42000-42010")

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "update-port-tunnel", 1.0, 1, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, repo, "update-port-tunnel")

	payload := `{"id":` + jsonInt(tunnelID) + `,"name":"update-port-tunnel","type":2,"flow":99999,"trafficRatio":1.0,"status":1,"inNodeId":[{"nodeId":` + jsonInt(entryID) + `,"protocol":"tls"}],"chainNodes":[[{"nodeId":` + jsonInt(chainID) + `,"protocol":"tls","strategy":"round"}]],"outNodeId":[{"nodeId":` + jsonInt(exitID) + `,"protocol":"tls"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/update", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)
	assertCode(t, res, 0)

	chainPort := mustQueryInt(t, repo, `SELECT port FROM chain_tunnel WHERE tunnel_id = ? AND chain_type = 2 LIMIT 1`, tunnelID)
	if chainPort <= 0 {
		t.Fatalf("expected chain node port to be assigned, got %d", chainPort)
	}

	outPort := mustQueryInt(t, repo, `SELECT port FROM chain_tunnel WHERE tunnel_id = ? AND chain_type = 3 LIMIT 1`, tunnelID)
	if outPort <= 0 {
		t.Fatalf("expected out node port to be assigned, got %d", outPort)
	}

	entryStrategy := mustQueryNullString(t, repo, `SELECT strategy FROM chain_tunnel WHERE tunnel_id = ? AND chain_type = 1 LIMIT 1`, tunnelID)
	if !entryStrategy.Valid || strings.TrimSpace(entryStrategy.String) == "" {
		t.Fatalf("expected entry strategy to be non-null and non-empty")
	}
	if entryStrategy.String != "round" {
		t.Fatalf("expected entry strategy round, got %q", entryStrategy.String)
	}
}

func jsonInt(v int64) string {
	return strconv.FormatInt(v, 10)
}
