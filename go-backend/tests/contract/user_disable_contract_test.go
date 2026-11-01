package contract_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-backend/internal/auth"
	"go-backend/internal/http/response"
	"go-backend/internal/store/model"
)

func TestManualUserDisableRevokesLoginTokenAndForwardAccess(t *testing.T) {
	const secret = "contract-jwt-secret"
	router, r := setupContractRouter(t, secret)
	seedContractUser(t, r, 2, "disable-contract", 1, 1)
	for _, value := range []interface{}{
		&model.Tunnel{ID: 1, Name: "disable-contract", Type: 1, Status: 1, Flow: 1},
		&model.UserTunnel{ID: 10, UserID: 2, TunnelID: 1, Status: 1, Flow: 100, Num: 10},
		&model.Forward{ID: 20, UserID: 2, UserName: "disable-contract", Name: "paused", TunnelID: 1, RemoteAddr: "127.0.0.1:80", Status: 0},
	} {
		if err := r.DB().Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	userToken, err := auth.GenerateToken(2, "disable-contract", 1, secret)
	if err != nil {
		t.Fatal(err)
	}
	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatal(err)
	}
	request := func(path, token, body string) response.R {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		req.Header.Set("Authorization", token)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		var out response.R
		if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if out := request("/api/v1/forward/list", userToken, `{}`); out.Code != 0 {
		t.Fatalf("token invalid before disable: %s", out.Msg)
	}
	if out := request("/api/v1/user/update", adminToken, `{"id":2,"user":"disable-contract","status":0}`); out.Code != 0 {
		t.Fatalf("disable user: %s", out.Msg)
	}
	for _, check := range []struct{ path, body string }{
		{"/api/v1/forward/list", `{}`},
		{"/api/v1/forward/create", `{"tunnelId":1,"name":"bypass","remoteAddr":"127.0.0.1:80"}`},
		{"/api/v1/forward/resume", `{"id":20}`},
		{"/api/v1/forward/batch-resume", `{"ids":[20]}`},
	} {
		if out := request(check.path, userToken, check.body); out.Code != 401 {
			t.Errorf("disabled user's old token accepted at %s: %+v", check.path, out)
		}
	}
	if out := request("/api/v1/user/login", "", `{"username":"disable-contract","password":"contract-pass","captchaId":""}`); out.Code == 0 || !strings.Contains(out.Msg, "停用") {
		t.Errorf("disabled account login: %+v", out)
	}
	if out := request("/api/v1/forward/resume", adminToken, `{"id":20}`); out.Code == 0 || !strings.Contains(out.Msg, "禁用") {
		t.Errorf("admin bypassed disabled owner's policy: %+v", out)
	}
	out := request("/api/v1/tunnel/user/list", adminToken, `{"userId":2}`)
	if out.Code != 0 {
		t.Fatalf("list permissions: %s", out.Msg)
	}
	items, ok := out.Data.([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("permission list: %#v", out.Data)
	}
	permission, ok := items[0].(map[string]interface{})
	if !ok || permission["status"] != float64(0) {
		t.Errorf("permission still displayed as enabled: %#v", items[0])
	}
	if got := mustQueryInt(t, r, "SELECT status FROM forward WHERE id = 20"); got != 0 {
		t.Errorf("disabled forward resumed: %d", got)
	}
}
