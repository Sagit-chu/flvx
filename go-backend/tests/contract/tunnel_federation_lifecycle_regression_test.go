package contract_test

import (
	"bytes"
	"encoding/json"
	"go-backend/internal/auth"
	"go-backend/internal/store/repo"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestFederationRedeployPreservesExistingBindings(t *testing.T) {
	testFederationRedeploy(t, false, false)
}
func TestFederationFailedRedeployPreservesExistingBindings(t *testing.T) {
	testFederationRedeploy(t, true, false)
}
func TestFederationCreateDoesNotHoldDatabaseDuringAcknowledgment(t *testing.T) {
	testFederationRedeploy(t, false, true)
}

func testFederationRedeploy(t *testing.T, failRedeploy, gateCreate bool) {
	providerSecret := "provider-contract-jwt"
	providerRouter, providerRepo := setupContractRouter(t, providerSecret)
	entered, release := make(chan struct{}), make(chan struct{})
	var gateOnce, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gateCreate && r.URL.Path == "/api/v1/federation/runtime/apply-role" {
			gateOnce.Do(func() { close(entered); <-release })
		}
		providerRouter.ServeHTTP(w, r)
	}))
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
	var destructive []string
	var armed bool
	recorder := func(kind string, data json.RawMessage) (bool, string) {
		mu.Lock()
		defer mu.Unlock()
		if kind == "DeleteService" || kind == "DeleteChains" {
			destructive = append(destructive, kind)
		}
		if armed && failRedeploy && kind == "UpdateService" {
			return true, "simulated node update failure"
		}
		return false, ""
	}
	stopEntry := startMockNodeSessionWithCommandRecorder(t, providerServer.URL, "provider-entry-secret", recorder)
	defer stopEntry()
	stopMiddle := startMockNodeSessionWithCommandRecorder(t, providerServer.URL, "provider-middle-secret", recorder)
	defer stopMiddle()
	stopExit := startMockNodeSessionWithCommandRecorder(t, providerServer.URL, "provider-exit-secret", recorder)
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

	defer unblock()
	var firstTunnelID int64
	if gateCreate {
		created := make(chan int64, 1)
		go func() { created <- createTunnel("dual-panel-middle-exit-1") }()
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("create did not reach federation gate")
		}
		listed := make(chan struct{})
		go func() {
			defer close(listed)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/list", bytes.NewBufferString("{}"))
			req.Header.Set("Authorization", consumerAdminToken)
			consumerRouter.ServeHTTP(httptest.NewRecorder(), req)
		}()
		select {
		case <-listed:
		case <-time.After(300 * time.Millisecond):
			t.Error("federation create held database while waiting for acknowledgment")
		}
		unblock()
		select {
		case firstTunnelID = <-created:
		case <-time.After(5 * time.Second):
			t.Fatal("create stuck after release")
		}
		select {
		case <-listed:
		case <-time.After(5 * time.Second):
			t.Fatal("list stuck after release")
		}
		return
	}
	firstTunnelID = createTunnel("dual-panel-middle-exit-1")

	mu.Lock()
	destructive = nil
	armed = true
	mu.Unlock()
	body, _ := json.Marshal(map[string]interface{}{"ids": []int64{firstTunnelID}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/batch-redeploy", bytes.NewReader(body))
	req.Header.Set("Authorization", consumerAdminToken)
	res := httptest.NewRecorder()
	consumerRouter.ServeHTTP(res, req)
	var out struct {
		Code int `json:"code"`
		Data struct {
			SuccessCount int `json:"successCount"`
			FailCount    int `json:"failCount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	expectedSuccess, expectedFailure := 1, 0
	if failRedeploy {
		expectedSuccess, expectedFailure = 0, 1
	}
	if out.Code != 0 || out.Data.SuccessCount != expectedSuccess || out.Data.FailCount != expectedFailure {
		t.Fatalf("redeploy failed: %s", res.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(destructive) > 0 {
		t.Fatalf("redeploy destroyed existing federation runtime: %v", destructive)
	}
}
