package contract_test

import (
	"bytes"
	"encoding/json"
	"go-backend/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTunnelDeleteWaitsForCreateRuntime(t *testing.T) {
	router, r := setupContractRouter(t, "create-lifecycle")
	server := httptest.NewServer(router)
	defer server.Close()
	token, err := auth.GenerateToken(1, "admin", 0, "create-lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	entry := insertContractNode(t, r, "create-entry", "192.0.2.10", "41000-41010", "create-entry-secret", 1)
	exit := insertContractNode(t, r, "create-exit", "192.0.2.11", "42000-42010", "create-exit-secret", 1)
	entered, release := make(chan struct{}), make(chan struct{})
	var once, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	deleted := make(chan struct{}, 1)
	stopEntry := startMockNodeSessionWithCommandRecorder(t, server.URL, "create-entry-secret", func(kind string, _ json.RawMessage) (bool, string) {
		if kind == "DeleteChains" {
			select {
			case deleted <- struct{}{}:
			default:
			}
		}
		return false, ""
	})
	defer stopEntry()
	stopExit := startMockNodeSessionWithCommandRecorder(t, server.URL, "create-exit-secret", func(kind string, _ json.RawMessage) (bool, string) {
		if kind == "AddService" {
			once.Do(func() { close(entered); <-release })
		}
		return false, ""
	})
	defer stopExit()
	defer unblock()
	post := func(path string, payload interface{}) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		req.Header.Set("Authorization", token)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	created := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		created <- post("/api/v1/tunnel/create", map[string]interface{}{"name": "create-race", "type": 2, "inNodeId": []map[string]interface{}{{"nodeId": entry, "protocol": "tls"}}, "outNodeId": []map[string]interface{}{{"nodeId": exit, "protocol": "tls"}}})
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("create did not reach runtime gate")
	}
	listed := post("/api/v1/tunnel/list", map[string]interface{}{})
	var list struct {
		Data []struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &list); err != nil || len(list.Data) != 1 {
		t.Fatal("committed create not listed")
	}
	deleteDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { deleteDone <- post("/api/v1/tunnel/delete", map[string]interface{}{"id": list.Data[0].ID}) }()
	select {
	case <-deleted:
		t.Error("delete removed runtime while create was still applying")
	case <-time.After(150 * time.Millisecond):
	}
	unblock()
	for _, done := range []chan *httptest.ResponseRecorder{created, deleteDone} {
		select {
		case res := <-done:
			assertCode(t, res, 0)
		case <-time.After(5 * time.Second):
			t.Fatal("lifecycle operation stuck")
		}
	}
	listed = post("/api/v1/tunnel/list", map[string]interface{}{})
	if err := json.Unmarshal(listed.Body.Bytes(), &list); err != nil || len(list.Data) != 0 {
		t.Fatal("deleted tunnel remains listed")
	}
}

func TestOfflineTunnelCreateReportsPendingRuntime(t *testing.T) {
	router, r := setupContractRouter(t, "pending-lifecycle")
	token, err := auth.GenerateToken(1, "admin", 0, "pending-lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	entry := insertContractNode(t, r, "pending-entry", "192.0.2.10", "41000-41010", "pending-entry-secret", 1)
	exit := insertContractNode(t, r, "pending-exit", "192.0.2.11", "42000-42010", "pending-exit-secret", 1)
	payload := map[string]interface{}{"name": "pending-tunnel", "type": 2, "inNodeId": []map[string]interface{}{{"nodeId": entry, "protocol": "tls"}}, "outNodeId": []map[string]interface{}{{"nodeId": exit, "protocol": "tls"}}}
	raw, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/create", bytes.NewReader(raw))
	req.Header.Set("Authorization", token)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			RuntimeStatus string `json:"runtimeStatus"`
			TunnelID      int64  `json:"tunnelId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Code != 0 || out.Data.RuntimeStatus != "pending" || out.Data.TunnelID <= 0 || out.Msg == "操作成功" {
		t.Fatalf("offline create must preserve config but explicitly report pending: %s", res.Body.String())
	}

	payload["id"] = out.Data.TunnelID
	raw, _ = json.Marshal(payload)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/update", bytes.NewReader(raw))
	req.Header.Set("Authorization", token)
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	out.Data.RuntimeStatus = ""
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Code != 0 || out.Data.RuntimeStatus != "pending" {
		t.Fatalf("offline update must report pending: %s", res.Body.String())
	}
	raw, _ = json.Marshal(map[string]interface{}{"ids": []int64{out.Data.TunnelID}})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/batch-redeploy", bytes.NewReader(raw))
	req.Header.Set("Authorization", token)
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	var batch struct {
		Data struct {
			SuccessCount int `json:"successCount"`
			FailCount    int `json:"failCount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if batch.Data.SuccessCount != 0 || batch.Data.FailCount != 1 {
		t.Fatalf("pending redeploy must not count as success: %s", res.Body.String())
	}
}

func TestPendingTunnelUpdateStillRemovesObsoleteRelay(t *testing.T) {
	testPendingTunnelRemoval(t, false, false)
}
func TestFailedObsoleteRelayRemovalPreservesItsChain(t *testing.T) {
	testPendingTunnelRemoval(t, true, false)
}
func TestRedesiredRelayKeepsOldPortReservedUntilReplacementAcknowledged(t *testing.T) {
	testPendingTunnelRemoval(t, true, true)
}

func testPendingTunnelRemoval(t *testing.T, failCleanup, readd bool) {
	router, r := setupContractRouter(t, "pending-removal")
	server := httptest.NewServer(router)
	defer server.Close()
	token, err := auth.GenerateToken(1, "admin", 0, "pending-removal")
	if err != nil {
		t.Fatal(err)
	}
	entry := insertContractNode(t, r, "removal-entry", "192.0.2.10", "41000-41010", "removal-entry-secret", 1)
	middle := insertContractNode(t, r, "removal-middle", "192.0.2.20", "42000-42010", "removal-middle-secret", 1)
	exit := insertContractNode(t, r, "removal-exit", "192.0.2.30", "43000-43010", "removal-exit-secret", 1)
	var entryTimeout atomic.Bool
	stopEntry := startMockNodeSessionWithCommandRecorder(t, server.URL, "removal-entry-secret", func(kind string, _ json.RawMessage) (bool, string) {
		if entryTimeout.Load() && (kind == "AddChains" || kind == "UpdateChains") {
			return true, "等待节点响应超时"
		}
		return false, ""
	})
	defer stopEntry()
	var mu sync.Mutex
	deleted := map[string]bool{}
	var timeoutReplacement bool
	stopMiddle := startMockNodeSessionWithCommandRecorder(t, server.URL, "removal-middle-secret", func(kind string, _ json.RawMessage) (bool, string) {
		mu.Lock()
		defer mu.Unlock()
		if kind == "DeleteService" || kind == "DeleteChains" {
			deleted[kind] = true
		}
		if timeoutReplacement && kind == "UpdateService" {
			return true, "等待节点响应超时"
		}
		if failCleanup && kind == "DeleteService" {
			return true, "cleanup rejected"
		}
		return false, ""
	})
	defer stopMiddle()
	stopExit := startMockNodeSession(t, server.URL, "removal-exit-secret")
	defer stopExit()
	post := func(path string, payload interface{}) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		req.Header.Set("Authorization", token)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	payload := map[string]interface{}{"name": "pending-removal", "type": 2, "inNodeId": []map[string]interface{}{{"nodeId": entry, "protocol": "tls"}}, "chainNodes": [][]map[string]interface{}{{{"nodeId": middle, "protocol": "tls", "port": 42000}}}, "outNodeId": []map[string]interface{}{{"nodeId": exit, "protocol": "tls"}}}
	assertCode(t, post("/api/v1/tunnel/create", payload), 0)
	listed := post("/api/v1/tunnel/list", map[string]interface{}{})
	var list struct {
		Data []struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &list); err != nil || len(list.Data) != 1 {
		t.Fatal("missing tunnel")
	}
	if readd {
		entryTimeout.Store(true)
	} else {
		stopEntry()
		waitNodeStatus(t, r, entry, 0)
	}
	payload["id"] = list.Data[0].ID
	payload["chainNodes"] = [][]map[string]interface{}{}
	res := post("/api/v1/tunnel/update", payload)
	var out struct {
		Msg  string `json:"msg"`
		Code int    `json:"code"`
		Data struct {
			RuntimeStatus string `json:"runtimeStatus"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Code != 0 || out.Data.RuntimeStatus != "pending" {
		t.Fatalf("expected pending update: %s", res.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if failCleanup {
		if !deleted["DeleteService"] || deleted["DeleteChains"] {
			t.Fatalf("failed listener deletion must preserve its required chain: %v", deleted)
		}
		if !strings.Contains(out.Msg, "cleanup rejected") {
			t.Fatalf("cleanup failure hidden: %s", out.Msg)
		}
		if readd {
			deleted = map[string]bool{}
			timeoutReplacement = true
			mu.Unlock()
			payload["chainNodes"] = [][]map[string]interface{}{{{"nodeId": middle, "protocol": "tls", "port": 42001}}}
			changed := post("/api/v1/tunnel/update", payload)
			if err := json.Unmarshal(changed.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if out.Code != 0 || out.Data.RuntimeStatus != "pending" {
				t.Fatalf("expected pending replacement: %s", changed.Body.String())
			}
			contender := map[string]interface{}{"name": "old-port-contender", "type": 2, "inNodeId": payload["inNodeId"], "outNodeId": payload["outNodeId"], "chainNodes": [][]map[string]interface{}{{{"nodeId": middle, "protocol": "tls", "port": 42000}}}}
			blocked := post("/api/v1/tunnel/create", contender)
			if err := json.Unmarshal(blocked.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if out.Code == 0 || !strings.Contains(out.Msg, "旧运行时清理") {
				t.Fatalf("unacknowledged old port was not reserved: %s", blocked.Body.String())
			}
			mu.Lock()
			timeoutReplacement = false
			mu.Unlock()
			post("/api/v1/tunnel/batch-redeploy", map[string]interface{}{"ids": []int64{list.Data[0].ID}})
			accepted := post("/api/v1/tunnel/create", contender)
			if err := json.Unmarshal(accepted.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if out.Code != 0 {
				t.Fatalf("acknowledged replacement did not release old port: %s", accepted.Body.String())
			}
			mu.Lock()
			if len(deleted) != 0 {
				t.Fatalf("delayed cleanup deleted re-desired relay: %v", deleted)
			}
			return
		}
		failCleanup = false
		deleted = map[string]bool{}
		mu.Unlock()
		post("/api/v1/tunnel/batch-redeploy", map[string]interface{}{"ids": []int64{list.Data[0].ID}})
		mu.Lock()
		if !deleted["DeleteService"] || !deleted["DeleteChains"] {
			t.Fatalf("redeploy lost obsolete cleanup ownership after failure: %v", deleted)
		}
		deleted = map[string]bool{}
		mu.Unlock()
		post("/api/v1/tunnel/batch-redeploy", map[string]interface{}{"ids": []int64{list.Data[0].ID}})
		mu.Lock()
		if len(deleted) != 0 {
			t.Fatalf("acknowledged cleanup repeated: %v", deleted)
		}
	} else if !deleted["DeleteService"] || !deleted["DeleteChains"] {
		t.Fatalf("pending update left removed relay live: %v", deleted)
	}
}
