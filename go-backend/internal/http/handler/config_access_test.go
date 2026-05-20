package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go-backend/internal/auth"
	"go-backend/internal/http/middleware"
	"go-backend/internal/http/response"
	"go-backend/internal/store/repo"
)

func TestPublicConfigGetAllowsBrandKeys(t *testing.T) {
	router, r := setupConfigAccessTestRouter(t)
	seedConfigValue(t, r, "app_name", "FLVX Brand")
	seedConfigValue(t, r, "app_logo", "logo-data")
	seedConfigValue(t, r, "app_favicon", "favicon-data")
	seedConfigValue(t, r, "app_bg_image", "bg-data")
	seedConfigValue(t, r, "cloudflare_site_key", "site-key")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/public/config/get", bytes.NewBufferString(`{"name":"app_name"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assertHandlerCode(t, resp, 0)
}

func TestPublicConfigGetRejectsSensitiveKeys(t *testing.T) {
	router, _ := setupConfigAccessTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/public/config/get", bytes.NewBufferString(`{"name":"jwt_secret"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assertHandlerCodeMsg(t, resp, 403, "禁止访问敏感配置")
}

func TestConfigGetAllowsPublicCloudflareSiteKeyWithoutAuthForCachedLoginPage(t *testing.T) {
	router, r := setupConfigAccessTestRouter(t)
	seedConfigValue(t, r, "cloudflare_site_key", "site-key")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/get", bytes.NewBufferString(`{"name":"cloudflare_site_key"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assertHandlerConfigValue(t, resp, "cloudflare_site_key", "site-key")
}

func TestConfigGetRejectsSensitiveKeysWithoutAuth(t *testing.T) {
	router, _ := setupConfigAccessTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/get", bytes.NewBufferString(`{"name":"jwt_secret"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assertHandlerCodeMsg(t, resp, 403, "禁止访问敏感配置")
}

func TestConfigGetAllowsSensitiveKeysForAdmin(t *testing.T) {
	router, r := setupConfigAccessTestRouter(t)
	adminToken := mustGenerateConfigAccessToken(t, 1, "admin_user", 0)
	seedConfigValue(t, r, "jwt_secret", "jwt-secret")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/get", bytes.NewBufferString(`{"name":"jwt_secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", adminToken)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assertHandlerConfigValue(t, resp, "jwt_secret", "jwt-secret")
}

func TestConfigListFiltersSensitiveKeysForNonAdmin(t *testing.T) {
	router, r := setupConfigAccessTestRouter(t)
	userToken := mustGenerateConfigAccessToken(t, 2, "user", 1)
	seedConfigAccessUser(t, r, 2, "user", 1)
	seedConfigValue(t, r, "app_name", "FLVX")
	seedConfigValue(t, r, "jwt_secret", "jwt-secret")
	seedConfigValue(t, r, "cloudflare_secret_key", "turnstile-secret")
	seedConfigValue(t, r, "license_key", "license-secret")
	seedConfigValue(t, r, "epay_key", "epay-secret")
	seedConfigValue(t, r, "usdt_secret_key", "usdt-secret")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/list", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", userToken)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	cfg := assertHandlerConfigMap(t, resp)
	if cfg["app_name"] != "FLVX" {
		t.Fatalf("expected public app_name to remain, got %+v", cfg)
	}
	for _, key := range []string{"jwt_secret", "cloudflare_secret_key", "license_key", "epay_key", "usdt_secret_key"} {
		if _, ok := cfg[key]; ok {
			t.Fatalf("expected %s to be filtered for non-admin, got %+v", key, cfg)
		}
	}
}

func TestConfigListAllowsSensitiveKeysForAdmin(t *testing.T) {
	router, r := setupConfigAccessTestRouter(t)
	adminToken := mustGenerateConfigAccessToken(t, 1, "admin_user", 0)
	seedConfigValue(t, r, "epay_key", "epay-secret")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/list", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", adminToken)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	cfg := assertHandlerConfigMap(t, resp)
	if cfg["epay_key"] != "epay-secret" {
		t.Fatalf("expected admin to see epay_key, got %+v", cfg)
	}
}

func TestConfigUpdateAllowsSensitiveKeysForAdmin(t *testing.T) {
	router, _ := setupConfigAccessTestRouter(t)
	adminToken := mustGenerateConfigAccessToken(t, 1, "admin_user", 0)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/update", bytes.NewBufferString(`{"jwt_secret":"rotated-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", adminToken)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assertHandlerCode(t, resp, 0)
}

func TestConfigUpdateSingleAllowsSensitiveKeysForAdmin(t *testing.T) {
	router, _ := setupConfigAccessTestRouter(t)
	adminToken := mustGenerateConfigAccessToken(t, 1, "admin_user", 0)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/update-single", bytes.NewBufferString(`{"name":"jwt_secret","value":"rotated-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", adminToken)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assertHandlerCode(t, resp, 0)
}

func TestConfigUpdateAllowsCloudflareSecretKeyWrite(t *testing.T) {
	router, r := setupConfigAccessTestRouter(t)
	adminToken := mustGenerateConfigAccessToken(t, 1, "admin_user", 0)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/update", bytes.NewBufferString(`{"cloudflare_secret_key":"turnstile-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", adminToken)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assertHandlerCode(t, resp, 0)

	cfg, err := r.GetConfigByName("cloudflare_secret_key")
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if cfg == nil || cfg.Value != "turnstile-secret" {
		t.Fatalf("expected cloudflare_secret_key to be updated, got %#v", cfg)
	}
}

func TestConfigUpdateSingleAllowsCloudflareSecretKeyWrite(t *testing.T) {
	router, r := setupConfigAccessTestRouter(t)
	adminToken := mustGenerateConfigAccessToken(t, 1, "admin_user", 0)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/update-single", bytes.NewBufferString(`{"name":"cloudflare_secret_key","value":"turnstile-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", adminToken)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assertHandlerCode(t, resp, 0)

	cfg, err := r.GetConfigByName("cloudflare_secret_key")
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if cfg == nil || cfg.Value != "turnstile-secret" {
		t.Fatalf("expected cloudflare_secret_key to be updated, got %#v", cfg)
	}
}

func TestConfigUpdateAllowsLicenseKeyWrite(t *testing.T) {
	router, r := setupConfigAccessTestRouter(t)
	adminToken := mustGenerateConfigAccessToken(t, 1, "admin_user", 0)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/update", bytes.NewBufferString(`{"license_key":"license-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", adminToken)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assertHandlerCode(t, resp, 0)

	cfg, err := r.GetConfigByName("license_key")
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if cfg == nil || cfg.Value != "license-secret" {
		t.Fatalf("expected license_key to be updated, got %#v", cfg)
	}
}

func TestConfigUpdateSingleAllowsLicenseKeyWrite(t *testing.T) {
	router, r := setupConfigAccessTestRouter(t)
	adminToken := mustGenerateConfigAccessToken(t, 1, "admin_user", 0)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/update-single", bytes.NewBufferString(`{"name":"license_key","value":"license-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", adminToken)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assertHandlerCode(t, resp, 0)

	cfg, err := r.GetConfigByName("license_key")
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if cfg == nil || cfg.Value != "license-secret" {
		t.Fatalf("expected license_key to be updated, got %#v", cfg)
	}
}

func setupConfigAccessTestRouter(t *testing.T) (http.Handler, *repo.Repository) {
	t.Helper()
	r, err := repo.Open(t.TempDir() + "/config-access.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
	})

	h := New(r, "unit-test-secret")
	mux := http.NewServeMux()
	h.Register(mux)
	wrapped := middleware.Recover(mux)
	wrapped = middleware.JWT(middleware.AuthOptions{JWTSecret: "unit-test-secret", GetUserAuthState: h.GetUserAuthState})(wrapped)
	wrapped = middleware.RequestLog(wrapped)
	wrapped = middleware.CORS(wrapped)
	return wrapped, r
}

func seedConfigValue(t *testing.T, r *repo.Repository, name, value string) {
	t.Helper()
	if err := r.DB().Exec(`INSERT INTO vite_config(name, value, time) VALUES(?, ?, 0) ON CONFLICT(name) DO UPDATE SET value = excluded.value, time = excluded.time`, name, value).Error; err != nil {
		t.Fatalf("seed config %s: %v", name, err)
	}
}

func seedConfigAccessUser(t *testing.T, r *repo.Repository, userID int64, username string, roleID int) {
	t.Helper()
	if err := r.DB().Exec(`INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status, password_changed_at) VALUES(?, ?, 'pwd', ?, 2727251700000, 0, 0, 0, 1, 0, 0, 0, 1, 0)`, userID, username, roleID).Error; err != nil {
		t.Fatalf("seed user %s: %v", username, err)
	}
}

func mustGenerateConfigAccessToken(t *testing.T, userID int64, username string, roleID int) string {
	t.Helper()
	token, err := auth.GenerateToken(userID, username, roleID, "unit-test-secret")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return token
}

func assertHandlerCode(t *testing.T, rec *httptest.ResponseRecorder, expected int) {
	t.Helper()
	var out response.R
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != expected {
		t.Fatalf("expected code %d, got %d", expected, out.Code)
	}
}

func assertHandlerCodeMsg(t *testing.T, rec *httptest.ResponseRecorder, expectedCode int, expectedMsg string) {
	t.Helper()
	var out response.R
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != expectedCode || out.Msg != expectedMsg {
		t.Fatalf("expected (%d,%q), got (%d,%q)", expectedCode, expectedMsg, out.Code, out.Msg)
	}
}

func assertHandlerConfigValue(t *testing.T, rec *httptest.ResponseRecorder, expectedName, expectedValue string) {
	t.Helper()
	var out struct {
		Code int `json:"code"`
		Data struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected code 0, got %d", out.Code)
	}
	if out.Data.Name != expectedName || out.Data.Value != expectedValue {
		t.Fatalf("expected config (%q,%q), got (%q,%q)", expectedName, expectedValue, out.Data.Name, out.Data.Value)
	}
}

func assertHandlerConfigMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var out struct {
		Code int               `json:"code"`
		Data map[string]string `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected code 0, got %d", out.Code)
	}
	return out.Data
}
