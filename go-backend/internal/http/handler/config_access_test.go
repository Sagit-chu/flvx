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

func TestPublicBrandConfigFallsBackWithoutCommercialLicense(t *testing.T) {
	router, r := setupConfigAccessTestRouter(t)
	seedConfigValue(t, r, "app_name", "Paid Brand")
	seedConfigValue(t, r, "app_logo", "logo-data")
	seedConfigValue(t, r, "app_favicon", "favicon-data")
	seedConfigValue(t, r, "hide_footer_brand", "true")
	seedConfigValue(t, r, "is_commercial", "false")

	for name, want := range map[string]string{
		"app_name":          "FLVX",
		"app_logo":          "",
		"app_favicon":       "",
		"hide_footer_brand": "false",
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/public/config/get", bytes.NewBufferString(`{"name":"`+name+`"}`))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assertHandlerConfigValue(t, resp, name, want)
	}
}

func TestPublicBrandConfigUsesSavedValuesWithCommercialLicense(t *testing.T) {
	router, r := setupConfigAccessTestRouter(t)
	seedConfigValue(t, r, "app_name", "Paid Brand")
	seedConfigValue(t, r, "is_commercial", "true")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/public/config/get", bytes.NewBufferString(`{"name":"app_name"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	assertHandlerConfigValue(t, resp, "app_name", "Paid Brand")
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

func TestConfigGetNeverReturnsLicenseCredentials(t *testing.T) {
	router, r := setupConfigAccessTestRouter(t)
	adminToken := mustGenerateConfigAccessToken(t, 1, "admin_user", 0)
	seedConfigValue(t, r, "license_key", "license-secret")
	seedConfigValue(t, r, "machine_fingerprint", "fingerprint-secret")

	for _, name := range []string{"license_key", "license_machine_id", "machine_fingerprint"} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/config/get", bytes.NewBufferString(`{"name":"`+name+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", adminToken)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		assertHandlerCodeMsg(t, resp, 403, "禁止访问系统授权凭据")
	}
}

func TestConfigListNeverReturnsLicenseCredentials(t *testing.T) {
	router, r := setupConfigAccessTestRouter(t)
	adminToken := mustGenerateConfigAccessToken(t, 1, "admin_user", 0)
	seedConfigValue(t, r, "license_key", "license-secret")
	seedConfigValue(t, r, "license_machine_id", "machine-id")
	seedConfigValue(t, r, "machine_fingerprint", "fingerprint-secret")
	seedConfigValue(t, r, "is_commercial", "true")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/list", nil)
	req.Header.Set("Authorization", adminToken)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	var out struct {
		Code int               `json:"code"`
		Data map[string]string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 || out.Data["is_commercial"] != "true" {
		t.Fatalf("unexpected config response: %+v", out)
	}
	if _, ok := out.Data["license_key"]; ok {
		t.Fatal("license_key must not be returned")
	}
	if _, ok := out.Data["license_machine_id"]; ok {
		t.Fatal("license_machine_id must not be returned")
	}
	if _, ok := out.Data["machine_fingerprint"]; ok {
		t.Fatal("machine_fingerprint must not be returned")
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

func TestConfigUpdateRejectsLicenseKeyWrite(t *testing.T) {
	router, r := setupConfigAccessTestRouter(t)
	adminToken := mustGenerateConfigAccessToken(t, 1, "admin_user", 0)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/update", bytes.NewBufferString(`{"license_key":"license-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", adminToken)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assertHandlerCodeMsg(t, resp, -1, "该配置由系统管理")
	if cfg, err := r.GetConfigByName("license_key"); err != nil || cfg != nil {
		t.Fatalf("license_key should not be written, got %#v err=%v", cfg, err)
	}
}

func TestConfigUpdateSingleRejectsLicenseKeyWrite(t *testing.T) {
	router, r := setupConfigAccessTestRouter(t)
	adminToken := mustGenerateConfigAccessToken(t, 1, "admin_user", 0)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/config/update-single", bytes.NewBufferString(`{"name":"license_key","value":"license-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", adminToken)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	assertHandlerCodeMsg(t, resp, -1, "该配置由系统管理")
	if cfg, err := r.GetConfigByName("license_key"); err != nil || cfg != nil {
		t.Fatalf("license_key should not be written, got %#v err=%v", cfg, err)
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
