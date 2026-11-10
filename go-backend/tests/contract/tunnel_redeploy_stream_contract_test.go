package contract_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-backend/internal/auth"
)

func TestTunnelRedeployStreamContract(t *testing.T) {
	const secret = "contract-jwt-secret"
	router, _ := setupContractRouter(t, secret)
	token, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/batch-redeploy/stream", strings.NewReader(`{"ids":[999998,999999]}`))
	req.Header.Set("Authorization", token)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if !strings.HasPrefix(res.Header().Get("Content-Type"), "application/x-ndjson") {
		t.Fatalf("expected streamed progress, got %s: %s", res.Header().Get("Content-Type"), res.Body.String())
	}
	decoder := json.NewDecoder(res.Body)
	for _, want := range []string{"start", "processing", "item", "processing", "item", "done"} {
		var event struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := decoder.Decode(&event); err != nil {
			t.Fatal(err)
		}
		if event.Type != want {
			t.Fatalf("got %s, want %s", event.Type, want)
		}
		if want == "item" {
			var data struct{ Result map[string]interface{} }
			if err := json.Unmarshal(event.Data, &data); err != nil {
				t.Fatal(err)
			}
			if _, ok := data.Result["name"].(string); !ok {
				t.Fatalf("item name must be a string, including missing tunnels: %s", event.Data)
			}
		}
		if want == "done" {
			var data struct {
				Progress struct{ Total, Completed, Success, Failed int }
				Result   struct {
					SuccessCount, FailCount int
					Failures                []struct {
						ID     int64
						Reason string
					}
				}
			}
			if err := json.Unmarshal(event.Data, &data); err != nil {
				t.Fatal(err)
			}
			if data.Progress.Total != 2 || data.Progress.Completed != 2 || data.Progress.Failed != 2 || data.Progress.Success != 0 || data.Result.FailCount != 2 || len(data.Result.Failures) != 2 {
				t.Fatalf("incorrect completion: %+v", data)
			}
			if data.Result.Failures[0].Reason == "" {
				t.Fatal("missing failure detail")
			}
		}
	}
	if !res.Flushed {
		t.Fatal("stream did not flush")
	}
}

func TestTunnelRedeployStreamAuthorizationContract(t *testing.T) {
	const secret = "contract-jwt-secret"
	router, repository := setupContractRouter(t, secret)
	seedLegacyUser(t, repository, 9111, "stream-normal-user", "password")
	token, err := auth.GenerateToken(9111, "stream-normal-user", 1, secret)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, token string
		code        int
	}{{"anonymous", "", 401}, {"non-admin", token, 403}} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/batch-redeploy/stream", strings.NewReader(`{"ids":[999999]}`))
			req.Header.Set("Authorization", tc.token)
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			assertCode(t, res, tc.code)
			if strings.Contains(res.Body.String(), `"type":"start"`) {
				t.Fatal("unauthorized request started stream")
			}
		})
	}
}
