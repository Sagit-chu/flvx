package contract_test

import (
	"bytes"
	"encoding/json"
	"go-backend/internal/auth"
	httpserver "go-backend/internal/http"
	"go-backend/internal/http/handler"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDeletedTunnelCleanupSurvivesHandlerReplacement(t *testing.T) {
	const secret = "deleted-cleanup"
	router, r := setupContractRouter(t, secret)
	server := httptest.NewServer(router)
	defer server.Close()
	token, err := auth.GenerateToken(1, "admin", 0, secret)
	if err != nil {
		t.Fatal(err)
	}
	entry := insertContractNode(t, r, "deleted-entry", "192.0.2.10", "41000-41010", "deleted-entry-secret", 1)
	exit := insertContractNode(t, r, "deleted-exit", "192.0.2.20", "42000", "deleted-exit-secret", 1)
	stopEntry := startMockNodeSession(t, server.URL, "deleted-entry-secret")
	defer stopEntry()
	var rejectDelete atomic.Bool
	rejectDelete.Store(true)
	stopExit := startMockNodeSessionWithCommandRecorder(t, server.URL, "deleted-exit-secret", func(kind string, _ json.RawMessage) (bool, string) {
		if kind == "DeleteService" && rejectDelete.Load() {
			return true, "cleanup rejected"
		}
		return false, ""
	})
	defer stopExit()
	post := func(path string, payload interface{}) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		req.Header.Set("Authorization", token)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	payload := map[string]interface{}{"name": "deleted-owner", "type": 2, "inNodeId": []map[string]interface{}{{"nodeId": entry, "protocol": "tls"}}, "outNodeId": []map[string]interface{}{{"nodeId": exit, "protocol": "tls", "port": 42000}}}
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
	id := list.Data[0].ID
	res := post("/api/v1/tunnel/delete", map[string]interface{}{"id": id})
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Code == 0 || !strings.Contains(out.Msg, "待重试") {
		t.Fatalf("failed post-delete cleanup not reported: %s", res.Body.String())
	}
	listed = post("/api/v1/tunnel/list", map[string]interface{}{})
	if err := json.Unmarshal(listed.Body.Bytes(), &list); err != nil || len(list.Data) != 0 {
		t.Fatal("desired tunnel not deleted")
	}
	payload["name"] = "deleted-port-contender"
	res = post("/api/v1/tunnel/create", payload)
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Code == 0 || !strings.Contains(out.Msg, "旧运行时清理") {
		t.Fatalf("deleted tunnel lost port ownership: %s", res.Body.String())
	}
	stopEntry()
	stopExit()
	waitNodeStatus(t, r, entry, 0)
	waitNodeStatus(t, r, exit, 0)
	// A fresh handler has no old in-memory topology, locks, or cleanup state.
	router = httpserver.NewRouter(handler.New(r, secret), secret)
	recoveredServer := httptest.NewServer(router)
	defer recoveredServer.Close()
	newEntry := startMockNodeSession(t, recoveredServer.URL, "deleted-entry-secret")
	defer newEntry()
	var deleted atomic.Int32
	newExit := startMockNodeSessionWithCommandRecorder(t, recoveredServer.URL, "deleted-exit-secret", func(kind string, _ json.RawMessage) (bool, string) {
		if kind == "DeleteService" {
			deleted.Add(1)
		}
		return false, ""
	})
	defer newExit()
	waitNodeStatus(t, r, entry, 1)
	waitNodeStatus(t, r, exit, 1)
	assertCode(t, post("/api/v1/tunnel/delete", map[string]interface{}{"id": id}), 0)
	if deleted.Load() != 1 {
		t.Fatalf("fresh handler did not recover obsolete listener ownership: %d", deleted.Load())
	}
	assertCode(t, post("/api/v1/tunnel/create", payload), 0)
	post("/api/v1/tunnel/batch-redeploy", map[string]interface{}{"ids": []int64{id}})
	if deleted.Load() != 1 {
		t.Fatal("acknowledged cleanup replayed against replacement tunnel")
	}
}
