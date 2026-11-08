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

func TestConcurrentCreatesReserveLocalPortsBeforeFederationAcknowledgment(t *testing.T) {
	provider, pr := setupContractRouter(t, "port-provider")
	entered, release := make(chan struct{}), make(chan struct{})
	var gateOnce, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	ps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/federation/runtime/apply-role" {
			gateOnce.Do(func() { close(entered); <-release })
		}
		provider.ServeHTTP(w, r)
	}))
	defer ps.Close()
	exit := insertContractNode(t, pr, "provider-port-exit", "192.0.2.30", "43000-43010", "provider-port-exit-secret", 1)
	now := time.Now().UnixMilli()
	insertPeerShare(t, pr, &repo.PeerShare{Name: "port-share", NodeID: exit, Token: "port-share-token", PortRangeStart: 43000, PortRangeEnd: 43010, IsActive: 1, CreatedTime: now, UpdatedTime: now})
	stop := startMockNodeSession(t, ps.URL, "provider-port-exit-secret")
	defer stop()
	defer unblock()
	consumer, cr := setupContractRouter(t, "port-consumer")
	token, err := auth.GenerateToken(1, "admin", 0, "port-consumer")
	if err != nil {
		t.Fatal(err)
	}
	entry := insertContractNode(t, cr, "local-port-entry", "192.0.2.10", "41000-41010", "local-port-entry-secret", 1)
	middle := insertContractNode(t, cr, "local-port-middle", "192.0.2.20", "42000", "local-port-middle-secret", 1)
	importRemoteNodeForContract(t, consumer, token, ps.URL, "port-share-token")
	remoteExit := queryRemoteNodeIDByToken(t, cr, "port-share-token")
	create := func(name string) *httptest.ResponseRecorder {
		payload := map[string]interface{}{"name": name, "type": 2, "inNodeId": []map[string]interface{}{{"nodeId": entry, "protocol": "tls"}}, "chainNodes": [][]map[string]interface{}{{{"nodeId": middle, "protocol": "tls"}}}, "outNodeId": []map[string]interface{}{{"nodeId": remoteExit, "protocol": "tls"}}}
		raw, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/create", bytes.NewReader(raw))
		req.Header.Set("Authorization", token)
		res := httptest.NewRecorder()
		consumer.ServeHTTP(res, req)
		return res
	}
	first := make(chan *httptest.ResponseRecorder, 1)
	go func() { first <- create("port-owner") }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first create did not reach provider gate")
	}
	second := make(chan *httptest.ResponseRecorder, 1)
	go func() { second <- create("port-contender") }()
	secondCompleted := false
	select {
	case res := <-second:
		secondCompleted = true
		var out struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.Code == 0 {
			t.Errorf("second create reused the only reserved local port: %s", res.Body.String())
		}
	case <-time.After(300 * time.Millisecond):
		t.Error("second create did not reject the reserved local port before provider acknowledgment")
	}
	unblock()
	select {
	case res := <-first:
		assertCode(t, res, 0)
	case <-time.After(5 * time.Second):
		t.Fatal("first create stuck")
	}
	// If the regression let the second request reach the gated provider, drain it
	// before closing test servers; its success still cannot erase the failure above.
	if !secondCompleted {
		select {
		case <-second:
		case <-time.After(5 * time.Second):
			t.Fatal("second create stuck after gate release")
		}
	}
}
