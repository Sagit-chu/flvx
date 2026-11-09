package contract_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"go-backend/internal/auth"
	"go-backend/internal/http/response"
)

func TestSystemUpgradeStatusRequiresAdminAndReturnsLocalState(t *testing.T) {
	secret := "upgrade-status-contract-secret"
	dir := t.TempDir()
	t.Setenv("PANEL_DEPLOY_DIR", dir)
	t.Setenv("FLUX_VERSION", "3.0.1-rc10")
	// A status read with no job must not require Docker or contact GitHub.
	t.Setenv("PATH", t.TempDir())
	router, r := setupContractRouter(t, secret)
	seedContractUser(t, r, 2, "normal_user", 1, 1)
	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatal(err)
	}
	userToken, err := auth.GenerateToken(2, "normal_user", 1, secret)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, token string
		want        int
	}{{"anonymous", "", 401}, {"normal user", userToken, 403}, {"admin", adminToken, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/system/upgrade/status", strings.NewReader(`{}`))
			req.Header.Set("Authorization", tc.token)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			var out response.R
			if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
				t.Fatal(err)
			}
			if out.Code != tc.want {
				t.Fatalf("expected code %d, got %d: %s", tc.want, out.Code, out.Msg)
			}
			if tc.want == 0 {
				data, ok := out.Data.(map[string]interface{})
				if !ok || data["currentVersion"] != "3.0.1-rc10" || data["job"] != nil {
					t.Fatalf("unexpected idle status: %#v", out.Data)
				}
			}
		})
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("idle status should not mutate deployment: entries=%v err=%v", entries, err)
	}
	for _, body := range []string{`{"jobId":"../../.env"}`, `{"jobId":`, `{"jobId":123}`} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/system/upgrade/status", strings.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		var out response.R
		if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if out.Code == 0 || out.Data != nil || strings.Contains(out.Msg, dir) || strings.Contains(out.Msg, ".env") {
			t.Fatalf("invalid request must fail without leaking filesystem detail: %#v", out)
		}
	}
}
