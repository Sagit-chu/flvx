package contract_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go-backend/internal/auth"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestMiddleToExitKeepsChainUntilReceiverReplacementAcknowledged(t *testing.T) {
	for _, port := range []int{42000, 42001} {
		for _, failure := range []string{"等待节点响应超时", "replacement rejected"} {
			t.Run(fmt.Sprintf("port=%d/failure=%s", port, failure), func(t *testing.T) {
				router, r := setupContractRouter(t, "role-transition")
				server := httptest.NewServer(router)
				defer server.Close()
				token, err := auth.GenerateToken(1, "admin", 0, "role-transition")
				if err != nil {
					t.Fatal(err)
				}
				entry := insertContractNode(t, r, "role-entry", "192.0.2.10", "41000-41010", "role-entry-secret", 1)
				middle := insertContractNode(t, r, "role-middle", "192.0.2.20", "42000-42010", "role-middle-secret", 1)
				exit := insertContractNode(t, r, "role-exit", "192.0.2.30", "43000-43010", "role-exit-secret", 1)
				stopEntry := startMockNodeSession(t, server.URL, "role-entry-secret")
				defer stopEntry()
				var mu sync.Mutex
				failReplacement := false
				replacementAccepted := false
				unsafeDeletion := false
				chainDeletes := 0
				stopMiddle := startMockNodeSessionWithCommandRecorder(t, server.URL, "role-middle-secret", func(kind string, data json.RawMessage) (bool, string) {
					mu.Lock()
					defer mu.Unlock()
					if kind == "UpdateService" {
						if failReplacement {
							return true, failure
						}
						var services []struct {
							Handler struct {
								Chain string `json:"chain"`
							} `json:"handler"`
						}
						if err := json.Unmarshal(data, &services); err == nil && len(services) > 0 && services[0].Handler.Chain == "" {
							replacementAccepted = true
						}
					}
					if kind == "DeleteChains" {
						chainDeletes++
						if !replacementAccepted {
							unsafeDeletion = true
						}
					}
					return false, ""
				})
				defer stopMiddle()
				stopExit := startMockNodeSession(t, server.URL, "role-exit-secret")
				defer stopExit()
				post := func(path string, payload interface{}) *httptest.ResponseRecorder {
					raw, _ := json.Marshal(payload)
					req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
					req.Header.Set("Authorization", token)
					res := httptest.NewRecorder()
					router.ServeHTTP(res, req)
					return res
				}
				payload := map[string]interface{}{"name": "role-transition", "type": 2, "inNodeId": []map[string]interface{}{{"nodeId": entry, "protocol": "tls"}}, "chainNodes": [][]map[string]interface{}{{{"nodeId": middle, "protocol": "tls", "port": 42000}}}, "outNodeId": []map[string]interface{}{{"nodeId": exit, "protocol": "tls", "port": 43000}}}
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
				mu.Lock()
				failReplacement = true
				mu.Unlock()
				payload["id"] = id
				payload["chainNodes"] = [][]map[string]interface{}{}
				payload["outNodeId"] = []map[string]interface{}{{"nodeId": middle, "protocol": "tls", "port": port}}
				post("/api/v1/tunnel/update", payload)
				post("/api/v1/tunnel/batch-redeploy", map[string]interface{}{"ids": []int64{id}})
				mu.Lock()
				if chainDeletes != 0 || unsafeDeletion {
					t.Errorf("old middle chain removed before receiver replacement ACK: deletes=%d unsafe=%v", chainDeletes, unsafeDeletion)
				}
				failReplacement = false
				mu.Unlock()
				recovered := post("/api/v1/tunnel/batch-redeploy", map[string]interface{}{"ids": []int64{id}})
				var result struct {
					Data struct {
						SuccessCount int `json:"successCount"`
						FailCount    int `json:"failCount"`
					} `json:"data"`
				}
				if err := json.Unmarshal(recovered.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.Data.SuccessCount != 1 || result.Data.FailCount != 0 {
					t.Fatalf("replacement retry failed: %s", recovered.Body.String())
				}
				mu.Lock()
				defer mu.Unlock()
				if !replacementAccepted || chainDeletes != 1 || unsafeDeletion {
					t.Fatalf("obsolete chain must be removed exactly after successful exit replacement: accepted=%v deletes=%d unsafe=%v", replacementAccepted, chainDeletes, unsafeDeletion)
				}
			})
		}
	}
}
