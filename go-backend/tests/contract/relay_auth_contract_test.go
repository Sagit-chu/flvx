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
	"go-backend/internal/store/repo"
)

func TestNewRelayTunnelEmitsMatchingPrivateEndpointAuthentication(t *testing.T) {
	router, r := setupContractRouter(t, "relay-auth-contract")
	server := httptest.NewServer(router)
	defer server.Close()
	token, err := auth.GenerateToken(1, "admin", 0, "relay-auth-contract")
	if err != nil {
		t.Fatal(err)
	}
	entry := insertContractNode(t, r, "auth-entry", "192.0.2.10", "41000-41010", "entry-management-secret", 1)
	exit := insertContractNode(t, r, "auth-exit", "192.0.2.11", "42000-42010", "exit-management-secret", 1)
	var mu sync.Mutex
	var connector, receiver map[string]interface{}
	reconnectEvents := make(chan string, 8)
	watchReconnect := false
	record := func(kind string, data json.RawMessage) (bool, string) {
		mu.Lock()
		defer mu.Unlock()
		if kind == "AddChains" || kind == "UpdateChains" {
			var c map[string]interface{}
			_ = json.Unmarshal(data, &c)
			if nested, ok := c["data"].(map[string]interface{}); ok {
				c = nested
			}
			hops, _ := c["hops"].([]interface{})
			if len(hops) > 0 {
				nodes := hops[0].(map[string]interface{})["nodes"].([]interface{})
				connector, _ = nodes[0].(map[string]interface{})["connector"].(map[string]interface{})["auth"].(map[string]interface{})
			}
		}
		if kind == "AddService" || kind == "UpdateService" {
			var services []map[string]interface{}
			_ = json.Unmarshal(data, &services)
			if len(services) > 0 {
				receiver, _ = services[0]["handler"].(map[string]interface{})["auth"].(map[string]interface{})
				if watchReconnect {
					p, _ := receiver["password"].(string)
					select {
					case reconnectEvents <- p:
					default:
					}
				}
			}
		}
		return false, ""
	}
	stopEntry := startMockNodeSessionWithCommandRecorder(t, server.URL, "entry-management-secret", record)
	defer stopEntry()
	stopExit := startMockNodeSessionWithCommandRecorder(t, server.URL, "exit-management-secret", record)
	defer stopExit()
	payload := map[string]interface{}{"name": "authenticated-new-tunnel", "type": 2, "inNodeId": []map[string]interface{}{{"nodeId": entry, "protocol": "tls"}}, "outNodeId": []map[string]interface{}{{"nodeId": exit, "protocol": "tls"}}}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/create", bytes.NewReader(body))
	req.Header.Set("Authorization", token)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	assertCode(t, res, 0)
	mu.Lock()
	if connector == nil || receiver == nil {
		t.Fatal("new relay tunnel emitted no endpoint authentication")
	}
	if connector["username"] == "" || connector["password"] == "" || connector["username"] != receiver["username"] || connector["password"] != receiver["password"] {
		t.Fatal("relay endpoint credentials missing or mismatched")
	}
	if receiver["password"] == "entry-management-secret" || receiver["password"] == "exit-management-secret" {
		t.Fatal("management secret reused for relay")
	}
	listReq := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/list", bytes.NewBufferString(`{}`))
	listReq.Header.Set("Authorization", token)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)
	if !bytes.Contains(listRes.Body.Bytes(), []byte(`"relayAuthMode":"required"`)) {
		t.Fatalf("tunnel list does not expose required authentication state: %s", listRes.Body.String())
	}
	password, _ := receiver["password"].(string)
	if bytes.Contains(listRes.Body.Bytes(), []byte(password)) {
		t.Fatal("tunnel list disclosed relay credential")
	}
	exportReq := httptest.NewRequest(http.MethodPost, "/api/v1/backup/export", bytes.NewBufferString(`{"types":["tunnels"]}`))
	exportReq.Header.Set("Authorization", token)
	exportRes := httptest.NewRecorder()
	router.ServeHTTP(exportRes, exportReq)
	if !bytes.Contains(exportRes.Body.Bytes(), []byte(`"relayAuthMode":"required"`)) {
		t.Fatal("configuration export lost required authentication policy")
	}
	if bytes.Contains(exportRes.Body.Bytes(), []byte(password)) {
		t.Fatal("configuration export disclosed private relay password")
	}
	originalPassword := password
	mu.Unlock()
	var listed struct {
		Data []struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listed); err != nil || len(listed.Data) != 1 {
		t.Fatal("missing created tunnel in list")
	}
	payload["id"] = listed.Data[0].ID
	body, _ = json.Marshal(payload)
	updateReq := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/update", bytes.NewReader(body))
	updateReq.Header.Set("Authorization", token)
	updateRes := httptest.NewRecorder()
	router.ServeHTTP(updateRes, updateReq)

	assertCode(t, updateRes, 0)
	mu.Lock()
	if connector == nil || receiver == nil || connector["password"] != originalPassword || receiver["password"] != originalPassword {
		t.Fatal("topology reapply lost or changed persisted endpoint authentication")
	}

	watchReconnect = true
	mu.Unlock()
	stopExit()
	stopReconnected := startMockNodeSessionWithCommandRecorder(t, server.URL, "exit-management-secret", record)
	defer stopReconnected()
	// Request reconciliation explicitly rather than waiting for the production
	// reconnect debounce (30 seconds). This crosses the same HTTP/command seam.
	redeployBody, _ := json.Marshal(map[string]interface{}{"ids": []int64{listed.Data[0].ID}})
	redeployReq := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/batch-redeploy", bytes.NewReader(redeployBody))
	redeployReq.Header.Set("Authorization", token)
	redeployRes := httptest.NewRecorder()
	router.ServeHTTP(redeployRes, redeployReq)
	assertCode(t, redeployRes, 0)

	select {
	case password := <-reconnectEvents:
		if password != originalPassword {
			t.Fatal("node reconnect lost persisted authentication")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("node reconnect did not reconcile relay service")
	}
	// Corrupt private desired state as a fault fixture. Reapply must stop rather
	// than regenerating a password or emitting an anonymous listener.
	if err := r.DB().Exec("DELETE FROM relay_endpoint_auth WHERE tunnel_id = ?", listed.Data[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	missingReq := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/update", bytes.NewReader(body))
	missingReq.Header.Set("Authorization", token)
	missingRes := httptest.NewRecorder()
	router.ServeHTTP(missingRes, missingReq)
	var missing struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(missingRes.Body.Bytes(), &missing)
	if missing.Code == 0 {
		t.Fatal("missing required credentials silently regenerated or downgraded")
	}
}

func TestFederatedRelayRequiresVersionedAuthAndRejectsDowngrade(t *testing.T) {
	router, r := setupContractRouter(t, "provider-auth-contract")
	server := httptest.NewServer(router)
	defer server.Close()
	node := insertContractNode(t, r, "provider-auth", "192.0.2.30", "45000-45010", "provider-management", 1)
	insertPeerShare(t, r, &repo.PeerShare{Name: "auth-share", NodeID: node, Token: "auth-share-token", PortRangeStart: 45000, PortRangeEnd: 45010, IsActive: 1})
	var mu sync.Mutex
	var received map[string]interface{}
	stop := startMockNodeSessionWithCommandRecorder(t, server.URL, "provider-management", func(kind string, data json.RawMessage) (bool, string) {
		if kind == "UpdateService" {
			var services []map[string]interface{}
			_ = json.Unmarshal(data, &services)
			if len(services) > 0 {
				mu.Lock()
				received, _ = services[0]["handler"].(map[string]interface{})["auth"].(map[string]interface{})
				mu.Unlock()
			}
		}
		return false, ""
	})
	defer stop()
	post := func(path string, payload interface{}) *httptest.ResponseRecorder {
		b, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer auth-share-token")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	reserve := post("/api/v1/federation/runtime/reserve-port", map[string]interface{}{"resourceKey": "auth-contract-endpoint", "protocol": "tls"})
	var reservation struct {
		Code int `json:"code"`
		Data struct {
			ReservationID    string `json:"reservationId"`
			RelayAuthVersion int    `json:"relayAuthVersion"`
		} `json:"data"`
	}
	if err := json.Unmarshal(reserve.Body.Bytes(), &reservation); err != nil || reservation.Code != 0 {
		t.Fatal("reserve failed")
	}
	if reservation.Data.RelayAuthVersion != 1 {
		t.Fatal("provider did not advertise relay authentication capability")
	}
	creds := map[string]interface{}{"version": 1, "username": "test-relay-user", "password": "test-only-relay-password-32-bytes-long"}
	request := map[string]interface{}{"reservationId": reservation.Data.ReservationID, "resourceKey": "auth-contract-endpoint", "role": "exit", "protocol": "tls", "relayAuth": creds}
	applied := post("/api/v1/federation/runtime/apply-role", request)
	if !bytes.Contains(applied.Body.Bytes(), []byte(`"relayAuthVersion":1`)) {
		t.Fatal("provider did not acknowledge authenticated runtime")
	}
	assertCode(t, applied, 0)
	mu.Lock()
	actual := received
	mu.Unlock()
	if actual == nil || actual["username"] != creds["username"] || actual["password"] != creds["password"] {
		t.Fatal("provider omitted requested relay authentication")
	}
	delete(request, "relayAuth")
	rejected := post("/api/v1/federation/runtime/apply-role", request)
	var result struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(rejected.Body.Bytes(), &result)
	if result.Code == 0 {
		t.Fatal("provider silently downgraded authenticated runtime")
	}

	// Loss of private persisted state must not be mistaken for legacy mode.
	if err := r.DB().Exec("UPDATE peer_share_runtime SET relay_auth = '' WHERE resource_key = ?", "auth-contract-endpoint").Error; err != nil {
		t.Fatal(err)
	}
	request["relayAuth"] = creds
	missing := post("/api/v1/federation/runtime/apply-role", request)
	_ = json.Unmarshal(missing.Body.Bytes(), &result)
	if result.Code == 0 {
		t.Fatal("provider recreated missing protected state instead of failing closed")
	}
}

func TestNewFederatedTunnelAuthenticatesEveryRelayHop(t *testing.T) {
	providerSecret := "provider-contract-jwt"
	providerRouter, providerRepo := setupContractRouter(t, providerSecret)
	providerServer := httptest.NewServer(providerRouter)
	defer providerServer.Close()

	consumerSecret := "consumer-contract-jwt"
	consumerRouter, consumerRepo := setupContractRouter(t, consumerSecret)

	consumerAdminToken, err := auth.GenerateToken(1, "consumer-admin", 0, consumerSecret)
	if err != nil {
		t.Fatalf("generate consumer admin token: %v", err)
	}

	now := time.Now().UnixMilli()
	providerEntryNodeID := insertContractNode(t, providerRepo, "provider-entry", "198.51.100.11", "43000-43010", "provider-entry-secret", 1)
	providerMiddleNodeID := insertContractNode(t, providerRepo, "provider-middle", "198.51.100.12", "44000-44010", "provider-middle-secret", 1)
	providerExitNodeID := insertContractNode(t, providerRepo, "provider-exit", "198.51.100.13", "45000-45010", "provider-exit-secret", 1)

	_ = insertPeerShare(t, providerRepo, &repo.PeerShare{
		Name:           "entry-share",
		NodeID:         providerEntryNodeID,
		Token:          "share-entry-token",
		PortRangeStart: 43000,
		PortRangeEnd:   43010,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	})
	_ = insertPeerShare(t, providerRepo, &repo.PeerShare{
		Name:           "middle-share",
		NodeID:         providerMiddleNodeID,
		Token:          "share-middle-token",
		PortRangeStart: 44000,
		PortRangeEnd:   44010,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	})
	_ = insertPeerShare(t, providerRepo, &repo.PeerShare{
		Name:           "exit-share",
		NodeID:         providerExitNodeID,
		Token:          "share-exit-token",
		PortRangeStart: 45000,
		PortRangeEnd:   45010,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
	})

	importRemoteNodeForContract(t, consumerRouter, consumerAdminToken, providerServer.URL, "share-entry-token")
	importRemoteNodeForContract(t, consumerRouter, consumerAdminToken, providerServer.URL, "share-middle-token")
	importRemoteNodeForContract(t, consumerRouter, consumerAdminToken, providerServer.URL, "share-exit-token")

	entryRemoteNodeID := queryRemoteNodeIDByToken(t, consumerRepo, "share-entry-token")
	middleRemoteNodeID := queryRemoteNodeIDByToken(t, consumerRepo, "share-middle-token")
	exitRemoteNodeID := queryRemoteNodeIDByToken(t, consumerRepo, "share-exit-token")

	var mu sync.Mutex
	services := map[string]string{}
	strictMiddle, strictExit := false, false
	connectors := map[string]string{}
	record := func(kind string, data json.RawMessage) (bool, string) {
		mu.Lock()
		defer mu.Unlock()
		if kind == "UpdateService" || kind == "AddService" {
			var items []map[string]interface{}
			_ = json.Unmarshal(data, &items)
			for _, item := range items {
				handler, _ := item["handler"].(map[string]interface{})
				if handler["type"] == "relay" {
					md, _ := handler["metadata"].(map[string]interface{})
					if handler["chain"] != nil {
						strictMiddle, _ = md["chain.strict"].(bool)
					} else {
						strictExit, _ = md["chain.strict"].(bool)
					}
					a, _ := handler["auth"].(map[string]interface{})
					if a != nil {
						user, _ := a["username"].(string)
						pass, _ := a["password"].(string)
						services[user] = pass
					}
				}
			}
		}
		if kind == "UpdateChains" || kind == "AddChains" {
			var c map[string]interface{}
			_ = json.Unmarshal(data, &c)
			if d, ok := c["data"].(map[string]interface{}); ok {
				c = d
			}
			hops, _ := c["hops"].([]interface{})
			for _, hop := range hops {
				nodes, _ := hop.(map[string]interface{})["nodes"].([]interface{})
				for _, node := range nodes {
					a, _ := node.(map[string]interface{})["connector"].(map[string]interface{})["auth"].(map[string]interface{})
					if a != nil {
						user, _ := a["username"].(string)
						pass, _ := a["password"].(string)
						connectors[user] = pass
					}
				}
			}
		}
		return false, ""
	}
	stopEntry := startMockNodeSessionWithCommandRecorder(t, providerServer.URL, "provider-entry-secret", record)
	defer stopEntry()
	stopMiddle := startMockNodeSessionWithCommandRecorder(t, providerServer.URL, "provider-middle-secret", record)
	defer stopMiddle()
	stopExit := startMockNodeSessionWithCommandRecorder(t, providerServer.URL, "provider-exit-secret", record)
	defer stopExit()

	createTunnel := func(name string) int64 {
		payload := map[string]interface{}{
			"name":   name,
			"type":   2,
			"flow":   99999,
			"status": 1,
			"inNodeId": []map[string]interface{}{
				{"nodeId": entryRemoteNodeID, "protocol": "tls", "strategy": "round"},
			},
			"chainNodes": [][]map[string]interface{}{
				{{"nodeId": middleRemoteNodeID, "protocol": "tls", "strategy": "round"}},
			},
			"outNodeId": []map[string]interface{}{
				{"nodeId": exitRemoteNodeID, "protocol": "tls", "strategy": "round"},
			},
		}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal create payload: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/create", bytes.NewReader(body))
		req.Header.Set("Authorization", consumerAdminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		consumerRouter.ServeHTTP(res, req)
		assertCode(t, res, 0)

		tunnelID := mustQueryInt64(t, consumerRepo, `SELECT id FROM tunnel WHERE name = ? ORDER BY id DESC LIMIT 1`, name)
		if tunnelID <= 0 {
			t.Fatalf("invalid tunnel id for %s", name)
		}
		return tunnelID
	}

	_ = createTunnel("dual-panel-middle-exit-1")

	mu.Lock()
	defer mu.Unlock()
	if !strictMiddle || strictExit {
		t.Fatal("middle must require its chain and exit must permit direct dialing")
	}
	if len(services) != 2 || len(connectors) != 2 {
		t.Fatalf("expected two authenticated receivers and matching connectors, got %d and %d", len(services), len(connectors))
	}
	for user, pass := range services {
		if pass == "" || connectors[user] != pass {
			t.Fatal("federation relay credentials do not match")
		}
	}
}

func TestRelayAuthenticationSurvivesInsertingBeforeExistingMiddle(t *testing.T) {
	router, r := setupContractRouter(t, "insert-auth-contract")
	server := httptest.NewServer(router)
	defer server.Close()
	token, _ := auth.GenerateToken(1, "admin", 0, "insert-auth-contract")
	ids := []int64{}
	var mu sync.Mutex
	received := map[string]string{}
	for i, name := range []string{"entry-a", "middle-b", "middle-c", "exit-d"} {
		id := insertContractNode(t, r, name, fmt.Sprintf("192.0.2.%d", 40+i), fmt.Sprintf("%d-%d", 46000+i*100, 46010+i*100), name+"-secret", 1)
		ids = append(ids, id)
		stop := startMockNodeSessionWithCommandRecorder(t, server.URL, name+"-secret", func(kind string, data json.RawMessage) (bool, string) {
			if kind == "AddService" || kind == "UpdateService" {
				var items []map[string]interface{}
				_ = json.Unmarshal(data, &items)
				for _, item := range items {
					handler, _ := item["handler"].(map[string]interface{})
					if a, ok := handler["auth"].(map[string]interface{}); ok {
						mu.Lock()
						received[name], _ = a["password"].(string)
						mu.Unlock()
					}
				}
			}
			return false, ""
		})
		defer stop()
	}
	node := func(id int64) map[string]interface{} { return map[string]interface{}{"nodeId": id, "protocol": "tls"} }
	payload := map[string]interface{}{"name": "insert-auth", "type": 2, "inNodeId": []map[string]interface{}{node(ids[0])}, "chainNodes": [][]map[string]interface{}{{node(ids[1])}}, "outNodeId": []map[string]interface{}{node(ids[3])}}
	post := func(path string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
		req.Header.Set("Authorization", token)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	assertCode(t, post("/api/v1/tunnel/create"), 0)
	mu.Lock()
	original := received["middle-b"]
	mu.Unlock()
	if original == "" {
		t.Fatal("original middle missing auth")
	}
	payload["id"] = mustQueryInt64(t, r, "SELECT id FROM tunnel WHERE name = ?", "insert-auth")
	payload["chainNodes"] = [][]map[string]interface{}{{node(ids[2])}, {node(ids[1])}}
	assertCode(t, post("/api/v1/tunnel/update"), 0)
	mu.Lock()
	defer mu.Unlock()
	if received["middle-b"] != original || received["middle-c"] == "" || received["middle-c"] == original {
		t.Fatal("hop insertion changed existing receiver identity or reused another credential")
	}
}

func TestConvertingDirectTunnelRequiresRelayAuthentication(t *testing.T) {
	router, r := setupContractRouter(t, "relay-auth-contract")
	server := httptest.NewServer(router)
	defer server.Close()
	token, err := auth.GenerateToken(1, "admin", 0, "relay-auth-contract")
	if err != nil {
		t.Fatal(err)
	}
	entry := insertContractNode(t, r, "auth-entry", "192.0.2.10", "41000-41010", "entry-management-secret", 1)
	exit := insertContractNode(t, r, "auth-exit", "192.0.2.11", "42000-42010", "exit-management-secret", 1)
	var mu sync.Mutex
	var connector, receiver map[string]interface{}
	record := func(kind string, data json.RawMessage) (bool, string) {
		mu.Lock()
		defer mu.Unlock()
		if kind == "AddChains" || kind == "UpdateChains" {
			var c map[string]interface{}
			_ = json.Unmarshal(data, &c)
			if nested, ok := c["data"].(map[string]interface{}); ok {
				c = nested
			}
			hops, _ := c["hops"].([]interface{})
			if len(hops) > 0 {
				nodes := hops[0].(map[string]interface{})["nodes"].([]interface{})
				connector, _ = nodes[0].(map[string]interface{})["connector"].(map[string]interface{})["auth"].(map[string]interface{})
			}
		}
		if kind == "AddService" || kind == "UpdateService" {
			var services []map[string]interface{}
			_ = json.Unmarshal(data, &services)
			if len(services) > 0 {
				receiver, _ = services[0]["handler"].(map[string]interface{})["auth"].(map[string]interface{})
			}
		}
		return false, ""
	}
	stopEntry := startMockNodeSessionWithCommandRecorder(t, server.URL, "entry-management-secret", record)
	defer stopEntry()
	stopExit := startMockNodeSessionWithCommandRecorder(t, server.URL, "exit-management-secret", record)
	defer stopExit()
	payload := map[string]interface{}{"name": "authenticated-new-tunnel", "type": 1, "inNodeId": []map[string]interface{}{{"nodeId": entry, "protocol": "tls"}}, "outNodeId": []map[string]interface{}{{"nodeId": exit, "protocol": "tls"}}}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/create", bytes.NewReader(body))
	req.Header.Set("Authorization", token)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	assertCode(t, res, 0)
	payload["type"] = 2
	payload["id"] = mustQueryInt64(t, r, "SELECT id FROM tunnel WHERE name = ?", "authenticated-new-tunnel")
	conversionBody, _ := json.Marshal(payload)
	conversionReq := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/update", bytes.NewReader(conversionBody))
	conversionReq.Header.Set("Authorization", token)
	conversionRes := httptest.NewRecorder()
	router.ServeHTTP(conversionRes, conversionReq)
	assertCode(t, conversionRes, 0)

	mu.Lock()
	if connector == nil || receiver == nil {
		t.Fatal("new relay tunnel emitted no endpoint authentication")
	}
	if connector["username"] == "" || connector["password"] == "" || connector["username"] != receiver["username"] || connector["password"] != receiver["password"] {
		t.Fatal("relay endpoint credentials missing or mismatched")
	}
	if receiver["password"] == "entry-management-secret" || receiver["password"] == "exit-management-secret" {
		t.Fatal("management secret reused for relay")
	}
	listReq := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/list", bytes.NewBufferString(`{}`))
	listReq.Header.Set("Authorization", token)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)
	if !bytes.Contains(listRes.Body.Bytes(), []byte(`"relayAuthMode":"required"`)) {
		t.Fatalf("tunnel list does not expose required authentication state: %s", listRes.Body.String())
	}
	password, _ := receiver["password"].(string)
	if bytes.Contains(listRes.Body.Bytes(), []byte(password)) {
		t.Fatal("tunnel list disclosed relay credential")
	}
	originalPassword := password
	mu.Unlock()
	var listed struct {
		Data []struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listed); err != nil || len(listed.Data) != 1 {
		t.Fatal("missing created tunnel in list")
	}
	payload["id"] = listed.Data[0].ID
	body, _ = json.Marshal(payload)
	updateReq := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/update", bytes.NewReader(body))
	updateReq.Header.Set("Authorization", token)
	updateRes := httptest.NewRecorder()
	router.ServeHTTP(updateRes, updateReq)

	assertCode(t, updateRes, 0)
	mu.Lock()
	defer mu.Unlock()
	if connector == nil || receiver == nil || connector["password"] != originalPassword || receiver["password"] != originalPassword {
		t.Fatal("topology reapply lost or changed persisted endpoint authentication")
	}
}

func TestBackupImportCannotSilentlyDowngradeRequiredRelay(t *testing.T) {
	router, _ := setupContractRouter(t, "backup-relay-auth")
	token, _ := auth.GenerateToken(1, "admin", 0, "backup-relay-auth")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/backup/import", bytes.NewBufferString(`{"version":"1","types":["tunnels"],"tunnels":[{"id":92,"name":"protected-backup","type":2,"protocol":"tls","flow":1,"trafficRatio":1,"createdTime":1000,"status":1,"relayAuthMode":"required"}]}`))
	request.Header.Set("Authorization", token)
	result := httptest.NewRecorder()
	router.ServeHTTP(result, request)
	var out struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}

	if out.Code == 0 {
		t.Fatal("configuration backup restored required relay without its private credentials")
	}
}

func TestNewRelayTunnelRefusesLegacyFederationBeforeReservation(t *testing.T) {
	var mu sync.Mutex
	mutations := 0
	oldPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/api/v1/federation/connect" {
			mu.Lock()
			mutations++
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"shareId":1,"shareName":"legacy","nodeId":1,"nodeName":"legacy-exit","serverIp":"192.0.2.80","status":1,"portRangeStart":49000,"portRangeEnd":49010}}`))
	}))
	defer oldPeer.Close()
	router, r := setupContractRouter(t, "legacy-peer-contract")
	token, _ := auth.GenerateToken(1, "admin", 0, "legacy-peer-contract")
	entry := insertContractNode(t, r, "legacy-test-entry", "192.0.2.79", "48000-48010", "legacy-test-entry-secret", 1)
	importRemoteNodeForContract(t, router, token, oldPeer.URL, "legacy-share-token")
	exit := queryRemoteNodeIDByToken(t, r, "legacy-share-token")
	payload := map[string]interface{}{"name": "must-not-downgrade", "type": 2, "inNodeId": []map[string]interface{}{{"nodeId": entry, "protocol": "tls"}}, "outNodeId": []map[string]interface{}{{"nodeId": exit, "protocol": "tls"}}}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/create", bytes.NewReader(b))
	req.Header.Set("Authorization", token)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	var out struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(res.Body.Bytes(), &out)
	if out.Code == 0 {
		t.Fatal("new tunnel silently accepted legacy federation")
	}
	mu.Lock()
	defer mu.Unlock()
	if mutations != 0 {
		t.Fatal("legacy peer was mutated before capability refusal")
	}
}

func TestLegacyRelayStateRemainsExplicitInList(t *testing.T) {
	router, r := setupContractRouter(t, "legacy-mode-contract")
	token, _ := auth.GenerateToken(1, "admin", 0, "legacy-mode-contract")
	if err := r.DB().Exec(`INSERT INTO tunnel(name,traffic_ratio,type,protocol,flow,created_time,updated_time,status,inx) VALUES('legacy-existing',1,2,'tls',1,1000,1000,1,0)`).Error; err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/list", nil)
	req.Header.Set("Authorization", token)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if !bytes.Contains(res.Body.Bytes(), []byte(`"relayAuthMode":"legacy"`)) {
		t.Fatal("legacy relay exposure is hidden from API clients")
	}
}
