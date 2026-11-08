package contract_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go-backend/internal/auth"
)

func TestForwardEntryRequiresTunnelChainContract(t *testing.T) {
	for _, tunnelType := range []int{1, 2} {
		t.Run(fmt.Sprintf("type-%d", tunnelType), func(t *testing.T) {
			secret := "contract-jwt-secret"
			router, repo := setupContractRouter(t, secret)
			server := httptest.NewServer(router)
			defer server.Close()

			adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
			if err != nil {
				t.Fatalf("generate admin token: %v", err)
			}

			now := time.Now().UnixMilli()
			if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(202, 'forward_bind_retry_user', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now).Error; err != nil {
				t.Fatalf("insert user: %v", err)
			}

			if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "forward-bind-retry-tunnel", 1.0, tunnelType, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
				t.Fatalf("insert tunnel: %v", err)
			}
			tunnelID := mustLastInsertID(t, repo, "forward-bind-retry-tunnel")

			if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "forward-bind-retry-node", "forward-bind-retry-secret", "10.42.0.1", "10.42.0.1", "", "44000-44010", "", "v1", 1, 1, 1, now, now, 1, "10.42.0.9", "[::]", 0).Error; err != nil {
				t.Fatalf("insert node: %v", err)
			}
			nodeID := mustLastInsertID(t, repo, "forward-bind-retry-node")

			if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 44001, 'round', 1, 'tls')
	`, tunnelID, nodeID).Error; err != nil {
				t.Fatalf("insert chain_tunnel: %v", err)
			}

			if tunnelType == 2 {
				if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 3, ?, 44002, 'round', 1, 'tls')
	`, tunnelID, nodeID).Error; err != nil {
					t.Fatalf("insert exit chain: %v", err)
				}
			}

			if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(41, 202, ?, NULL, 999, 99999, 0, 0, 1, 2727251700000, 1)
	`, tunnelID).Error; err != nil {
				t.Fatalf("insert user_tunnel: %v", err)
			}

			createPayload := map[string]interface{}{
				"name":       "forward-bind-retry-target",
				"tunnelId":   tunnelID,
				"remoteAddr": "1.1.1.1:443",
				"strategy":   "fifo",
			}
			createBody, err := json.Marshal(createPayload)
			if err != nil {
				t.Fatalf("marshal create payload: %v", err)
			}

			var mu sync.Mutex
			var emitted []map[string]interface{}
			stop := startMockNodeSessionWithCommandRecorder(t, server.URL, "forward-bind-retry-secret", func(kind string, data json.RawMessage) (bool, string) {
				if kind == "AddService" || kind == "UpdateService" {
					var services []map[string]interface{}
					if json.Unmarshal(data, &services) == nil {
						mu.Lock()
						emitted = append(emitted, services...)
						mu.Unlock()
					}
				}
				return false, ""
			})
			defer stop()
			waitNodeStatus(t, repo, nodeID, 1)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
			req.Header.Set("Authorization", adminToken)
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			assertCode(t, res, 0)
			mu.Lock()
			defer mu.Unlock()
			found := 0
			for _, svc := range emitted {
				h, ok := svc["handler"].(map[string]interface{})
				if !ok || (h["type"] != "tcp" && h["type"] != "udp") {
					continue
				}
				found++
				md, _ := h["metadata"].(map[string]interface{})
				if tunnelType == 2 {
					if h["chain"] != fmt.Sprintf("chains_%d", tunnelID) || md["chain.strict"] != true {
						t.Fatalf("tunnel forward must require chain, got %v", h)
					}
				} else if md["chain.strict"] == true {
					t.Fatal("direct forward cannot require a tunnel chain")
				}
			}
			if found < 2 {
				t.Fatalf("expected TCP and UDP forwarding commands, got %d", found)
			}
		})
	}
}
