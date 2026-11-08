package contract_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go-backend/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTunnelRedeployPreservesLiveRoutes(t *testing.T) {
	testTunnelLifecycle(t, false, false, false)
}

func TestTunnelEditWaitsForRedeploy(t *testing.T) {
	testTunnelLifecycle(t, true, false, false)
}

func TestTunnelFailedRedeployPreservesLiveRoutes(t *testing.T) {
	testTunnelLifecycle(t, false, true, false)
}

func TestTunnelDeleteWaitsForRedeploy(t *testing.T) { testTunnelLifecycle(t, true, false, true) }

func testTunnelLifecycle(t *testing.T, concurrentEdit, failRedeploy, concurrentDelete bool) {
	secret := "contract-jwt-secret"
	router, r := setupContractRouter(t, secret)
	server := httptest.NewServer(router)
	defer server.Close()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()
	if err := r.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(2, 'issue281_transition_user', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "issue281-transition-tunnel", 1.0, 2, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, r, "issue281-transition-tunnel")

	insertNode := func(name, secretValue, ip, portRange string, inx int) int64 {
		if err := r.DB().Exec(`
			INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, name, secretValue, ip, ip, "", portRange, "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", inx).Error; err != nil {
			t.Fatalf("insert node %s: %v", name, err)
		}
		return mustLastInsertID(t, r, name)
	}

	entryA := insertNode("issue281-transition-entry-a", "issue281-transition-entry-a-secret", "10.52.0.1", "54000-54010", 0)
	entryB := insertNode("issue281-transition-entry-b", "issue281-transition-entry-b-secret", "10.52.0.2", "54000-54010", 1)
	entryC := insertNode("issue281-transition-entry-c", "issue281-transition-entry-c-secret", "10.52.0.3", "54000-54010", 2)
	exitNodeID := insertNode("issue281-transition-exit", "issue281-transition-exit-secret", "10.52.0.4", "57000-57010", 3)

	if err := r.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 54001, 'round', 1, 'tls')
	`, tunnelID, entryA).Error; err != nil {
		t.Fatalf("insert initial entry chain_tunnel: %v", err)
	}
	if err := r.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 3, ?, 57001, 'round', 1, 'tls')
	`, tunnelID, exitNodeID).Error; err != nil {
		t.Fatalf("insert exit chain_tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(282, 2, ?, NULL, 999, 99999, 0, 0, 1, 2727251700000, 1)
	`, tunnelID).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO forward(user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(2, 'issue281_transition_user', 'issue281-transition-forward', ?, '1.1.1.1:443', 'fifo', 0, 0, ?, ?, 1, 0)
	`, tunnelID, now, now).Error; err != nil {
		t.Fatalf("insert forward: %v", err)
	}
	forwardID := mustLastInsertID(t, r, "issue281-transition-forward")

	if err := r.DB().Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(?, ?, ?)`, forwardID, entryA, 54001).Error; err != nil {
		t.Fatalf("insert forward_port: %v", err)
	}

	// This mock records control-plane commands only; it does not forward traffic.
	type command struct {
		node, kind string
		data       json.RawMessage
	}
	var mu sync.Mutex
	var commands []command
	var armed, gateClaimed bool
	entered := make(chan struct{})
	release := make(chan struct{})
	var gateOnce, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	editApplied := make(chan struct{}, 1)

	for _, n := range []struct {
		name, secret string
		id           int64
	}{
		{"A", "issue281-transition-entry-a-secret", entryA},
		{"B", "issue281-transition-entry-b-secret", entryB},
		{"C", "issue281-transition-entry-c-secret", entryC},
		{"D", "issue281-transition-exit-secret", exitNodeID},
	} {
		name := n.name
		stop := startMockNodeSessionWithCommandRecorder(t, server.URL, n.secret, func(kind string, data json.RawMessage) (bool, string) {
			mu.Lock()
			gate := armed && !gateClaimed && name == "D" && kind == "UpdateService"
			if gate {
				gateClaimed = true
			}
			if armed && ((name == "C" && (kind == "UpdateChains" || kind == "AddChains")) || (concurrentDelete && kind == "DeleteChains")) {
				select {
				case editApplied <- struct{}{}:
				default:
				}
			}
			mu.Unlock()
			if gate {
				gateOnce.Do(func() { close(entered); <-release })
			}
			mu.Lock()
			defer mu.Unlock()
			commands = append(commands, command{name, kind, append(json.RawMessage(nil), data...)})
			if armed && failRedeploy && name == "B" && kind == "UpdateService" {
				return true, "bind: address already in use"
			}
			return false, ""
		})
		defer stop()
		waitNodeStatus(t, r, n.id, 1)
	}
	// Let initial mock-node reconnect reconciliation finish before the topology edit.
	time.Sleep(time.Second)
	node := func(id int64) map[string]interface{} {
		return map[string]interface{}{"nodeId": id, "protocol": "tls", "strategy": "round", "port": 54002}
	}
	update := func(hops [][]map[string]interface{}) {
		mu.Lock()
		commands = nil
		mu.Unlock()
		payload := map[string]interface{}{"id": tunnelID, "name": "issue281-transition-tunnel", "type": 2, "flow": 99999, "trafficRatio": 1.0, "status": 1,
			"inNodeId": []map[string]interface{}{node(entryA)}, "chainNodes": hops,
			"outNodeId": []map[string]interface{}{{"nodeId": exitNodeID, "protocol": "tls", "strategy": "round", "port": 57001}}}
		raw, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/update", bytes.NewReader(raw))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCode(t, res, 0)
	}
	check := func(edges map[string]int64) {
		mu.Lock()
		defer mu.Unlock()
		expectedChain := fmt.Sprintf("chains_%d", tunnelID)
		got := map[string]bool{}
		services := map[string]bool{}
		lastChains := map[string]int{}
		lastServices := map[string]int{}
		for i, c := range commands {
			if c.kind == "UpdateChains" || c.kind == "AddChains" {
				lastChains[c.node] = i
			}
			if c.kind == "UpdateService" || c.kind == "AddService" {
				lastServices[c.node] = i
			}
		}
		for i, c := range commands {
			if c.kind == "DeleteChains" {
				t.Fatalf("unexpected chain deletion on %s: %s", c.node, c.data)
			}
			if c.kind == "UpdateChains" || c.kind == "AddChains" {
				if lastChains[c.node] != i {
					continue
				}
				var raw map[string]interface{}
				if err := json.Unmarshal(c.data, &raw); err != nil {
					t.Fatal(err)
				}
				if nested, ok := raw["data"].(map[string]interface{}); ok {
					raw = nested
				}
				target, ok := edges[c.node]
				if !ok {
					t.Fatalf("unexpected chain owner %s", c.node)
				}
				port := 54002
				if target == exitNodeID {
					port = 57001
				}
				ip := map[int64]string{entryB: "10.52.0.2", entryC: "10.52.0.3", exitNodeID: "10.52.0.4"}[target]
				hops := raw["hops"].([]interface{})
				nodes := hops[0].(map[string]interface{})["nodes"].([]interface{})
				addr := nodes[0].(map[string]interface{})["addr"]
				if len(hops) != 1 || len(nodes) != 1 || addr != fmt.Sprintf("%s:%d", ip, port) {
					t.Fatalf("bad edge %s: %s", c.node, c.data)
				}
				got[c.node] = true
				t.Logf("%s -> %s via %s", c.node, ip, addr)
			}
			if strings.EqualFold(c.kind, "UpdateService") || strings.EqualFold(c.kind, "AddService") {
				if lastServices[c.node] != i {
					continue
				}
				var ss []map[string]interface{}
				if err := json.Unmarshal(c.data, &ss); err != nil {
					t.Fatal(err)
				}
				for _, s := range ss {
					if s["name"] != fmt.Sprintf("tunnel_%d", tunnelID) {
						continue
					}
					h := s["handler"].(map[string]interface{})
					if c.node == "B" || c.node == "C" {
						if h["chain"] != expectedChain {
							t.Fatalf("missing chain on %s: %s", c.node, c.data)
						}
					}
					if c.node == "D" && h["chain"] != nil {
						t.Fatalf("exit has unexpected chain")
					}
					services[c.node] = true
				}
			}
		}
		for owner := range edges {
			if !got[owner] {
				t.Errorf("no chain update for %s", owner)
			}
			if owner != "A" && !services[owner] {
				t.Errorf("no relay service for %s", owner)
			}
		}
		if !services["D"] {
			t.Error("no exit relay service")
		}
	}
	update([][]map[string]interface{}{{node(entryB)}})
	check(map[string]int64{"A": entryB, "B": exitNodeID})
	mu.Lock()
	commands = nil
	mu.Unlock()
	raw, _ := json.Marshal(map[string]interface{}{"ids": []int64{tunnelID}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/batch-redeploy", bytes.NewReader(raw))
	req.Header.Set("Authorization", adminToken)
	res := httptest.NewRecorder()
	if !concurrentEdit {
		mu.Lock()
		armed = failRedeploy
		mu.Unlock()
		router.ServeHTTP(res, req)
		if failRedeploy {
			var out struct {
				Data struct {
					FailCount    int `json:"failCount"`
					SuccessCount int `json:"successCount"`
				} `json:"data"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if out.Data.FailCount != 1 || out.Data.SuccessCount != 0 {
				t.Fatalf("expected redeploy failure: %s", res.Body.String())
			}
			mu.Lock()
			defer mu.Unlock()
			for _, c := range commands {
				if c.kind == "DeleteService" || c.kind == "DeleteChains" {
					t.Errorf("failed redeploy removed existing runtime: %s on %s", c.kind, c.node)
				}
			}
		} else {
			assertCode(t, res, 0)
			check(map[string]int64{"A": entryB, "B": exitNodeID})
		}
		return
	}
	mu.Lock()
	armed = true
	mu.Unlock()
	redeployDone := make(chan struct{})
	go func() { defer close(redeployDone); router.ServeHTTP(res, req) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("redeploy did not reach command gate")
	}
	editDone := make(chan struct{})
	go func() {
		defer close(editDone)
		if concurrentDelete {
			body, _ := json.Marshal(map[string]interface{}{"id": tunnelID})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/delete", bytes.NewReader(body))
			req.Header.Set("Authorization", adminToken)
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			assertCode(t, res, 0)
		} else {
			update([][]map[string]interface{}{{node(entryB)}, {node(entryC)}})
		}
	}()
	select {
	case <-editApplied:
		t.Error("concurrent mutation changed runtime before redeploy finished")
	case <-time.After(150 * time.Millisecond):
	}
	unblock()
	for _, done := range []chan struct{}{redeployDone, editDone} {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("lifecycle operation did not complete")
		}
	}
	assertCode(t, res, 0)
	if !concurrentDelete {
		mu.Lock()
		defer mu.Unlock()
		latest := map[string]string{}
		for _, c := range commands {
			if c.kind != "UpdateChains" && c.kind != "AddChains" {
				continue
			}
			var raw map[string]interface{}
			if err := json.Unmarshal(c.data, &raw); err != nil {
				t.Fatal(err)
			}
			if nested, ok := raw["data"].(map[string]interface{}); ok {
				raw = nested
			}
			hops := raw["hops"].([]interface{})
			nodes := hops[0].(map[string]interface{})["nodes"].([]interface{})
			latest[c.node] = nodes[0].(map[string]interface{})["addr"].(string)
		}
		if latest["B"] != "10.52.0.3:54002" {
			t.Fatalf("stale redeploy overwrote edit: B targets %s", latest["B"])
		}
	}
}
