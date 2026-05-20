package contract_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"go-backend/internal/auth"
	httpserver "go-backend/internal/http"
	"go-backend/internal/http/handler"
	"go-backend/internal/http/response"
	"go-backend/internal/security"
	"go-backend/internal/store/repo"

	"gorm.io/gorm"
)

func TestCaptchaVerifyLoginContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, r := setupContractRouter(t, secret)
	verifiedToken := ""

	if err := r.DB().Exec(`
		INSERT INTO vite_config(name, value, time)
		VALUES(?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET value = excluded.value, time = excluded.time
	`, "captcha_enabled", "true", time.Now().UnixMilli()).Error; err != nil {
		t.Fatalf("enable captcha: %v", err)
	}

	t.Run("login allowed when cloudflare keys are missing", func(t *testing.T) {
		body := bytes.NewBufferString(`{"username":"admin_user","password":"admin_user","captchaId":""}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/login", body)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assertCode(t, resp, 0)
	})

	t.Run("captcha verify remains compatible without cloudflare secret", func(t *testing.T) {
		verifyReq := httptest.NewRequest(http.MethodPost, "/api/v1/captcha/verify", bytes.NewBufferString(`{"id":"captcha-token-1","data":"ok"}`))
		verifyReq.Header.Set("Content-Type", "application/json")
		verifyResp := httptest.NewRecorder()

		router.ServeHTTP(verifyResp, verifyReq)

		var verifyOut struct {
			Success bool `json:"success"`
			Data    struct {
				ValidToken string `json:"validToken"`
			} `json:"data"`
		}
		if err := json.NewDecoder(verifyResp.Body).Decode(&verifyOut); err != nil {
			t.Fatalf("decode captcha verify response: %v", err)
		}
		if !verifyOut.Success || verifyOut.Data.ValidToken != "captcha-token-1" {
			t.Fatalf("unexpected captcha verify payload: success=%v token=%q", verifyOut.Success, verifyOut.Data.ValidToken)
		}

		verifiedToken = verifyOut.Data.ValidToken
	})

	if err := r.DB().Exec(`
		INSERT INTO vite_config(name, value, time)
		VALUES(?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET value = excluded.value, time = excluded.time
	`, "cloudflare_site_key", "test-site-key", time.Now().UnixMilli()).Error; err != nil {
		t.Fatalf("set cloudflare site key: %v", err)
	}
	if err := r.DB().Exec(`
		INSERT INTO vite_config(name, value, time)
		VALUES(?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET value = excluded.value, time = excluded.time
	`, "cloudflare_secret_key", "test-secret-key", time.Now().UnixMilli()).Error; err != nil {
		t.Fatalf("set cloudflare secret key: %v", err)
	}

	t.Run("login denied without verified captcha token", func(t *testing.T) {
		body := bytes.NewBufferString(`{"username":"admin_user","password":"admin_user","captchaId":""}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/login", body)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assertCodeMsg(t, resp, -1, "验证码校验失败")
	})

	t.Run("whmcs api client bypasses captcha", func(t *testing.T) {
		body := bytes.NewBufferString(`{"username":"admin_user","password":"admin_user","captchaId":""}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/login", body)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-FLVX-API-Client", "whmcs")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assertCode(t, resp, 0)
	})

	t.Run("captcha token is one-time and consumed by login", func(t *testing.T) {
		if strings.TrimSpace(verifiedToken) == "" {
			t.Fatalf("expected verified token from compatibility captcha verify")
		}

		loginBody := bytes.NewBufferString(`{"username":"admin_user","password":"admin_user","captchaId":"` + verifiedToken + `"}`)
		loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/user/login", loginBody)
		loginReq.Header.Set("Content-Type", "application/json")
		loginResp := httptest.NewRecorder()
		router.ServeHTTP(loginResp, loginReq)
		assertCode(t, loginResp, 0)

		replayBody := bytes.NewBufferString(`{"username":"admin_user","password":"admin_user","captchaId":"` + verifiedToken + `"}`)
		replayReq := httptest.NewRequest(http.MethodPost, "/api/v1/user/login", replayBody)
		replayReq.Header.Set("Content-Type", "application/json")
		replayResp := httptest.NewRecorder()
		router.ServeHTTP(replayResp, replayReq)
		assertCodeMsg(t, replayResp, -1, "验证码校验失败")
	})
}

func TestPublicConfigGetAndAuthConfigContract(t *testing.T) {
	router, r := setupContractRouter(t, "contract-jwt-secret")

	for name, value := range map[string]string{
		"app_name":              "FLVX Public",
		"app_logo":              "logo",
		"app_favicon":           "favicon",
		"app_bg_image":          "bg",
		"cloudflare_site_key":   "site-key",
		"cloudflare_secret_key": "secret-key",
		"jwt_secret":            "jwt-secret",
	} {
		if err := r.DB().Exec(`
			INSERT INTO vite_config(name, value, time)
			VALUES(?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET value = excluded.value, time = excluded.time
		`, name, value, time.Now().UnixMilli()).Error; err != nil {
			t.Fatalf("seed config %s: %v", name, err)
		}
	}

	publicReq := httptest.NewRequest(http.MethodPost, "/api/v1/public/config/get", bytes.NewBufferString(`{"name":"app_name"}`))
	publicReq.Header.Set("Content-Type", "application/json")
	publicResp := httptest.NewRecorder()
	router.ServeHTTP(publicResp, publicReq)
	assertCode(t, publicResp, 0)

	secretReq := httptest.NewRequest(http.MethodPost, "/api/v1/public/config/get", bytes.NewBufferString(`{"name":"jwt_secret"}`))
	secretReq.Header.Set("Content-Type", "application/json")
	secretResp := httptest.NewRecorder()
	router.ServeHTTP(secretResp, secretReq)
	assertCodeMsg(t, secretResp, 403, "禁止访问敏感配置")

	configReq := httptest.NewRequest(http.MethodPost, "/api/v1/config/get", bytes.NewBufferString(`{"name":"cloudflare_site_key"}`))
	configReq.Header.Set("Content-Type", "application/json")
	configResp := httptest.NewRecorder()
	router.ServeHTTP(configResp, configReq)
	assertCode(t, configResp, 0)

	configSecretReq := httptest.NewRequest(http.MethodPost, "/api/v1/config/get", bytes.NewBufferString(`{"name":"jwt_secret"}`))
	configSecretReq.Header.Set("Content-Type", "application/json")
	configSecretResp := httptest.NewRecorder()
	router.ServeHTTP(configSecretResp, configSecretReq)
	assertCodeMsg(t, configSecretResp, 403, "禁止访问敏感配置")
}

func TestOpenAPISubStoreContracts(t *testing.T) {
	router, r := setupContractRouter(t, "contract-jwt-secret")

	const tunnelFlowGB = int64(500)
	const tunnelInFlow = int64(123)
	const tunnelOutFlow = int64(456)
	const tunnelExpTimeMs = int64(2727251700000)

	now := time.Now().UnixMilli()
	if err := r.DB().Exec(`INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"contract-tunnel", 1.0, 1, "tls", 1, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, r, "contract-tunnel")
	if err := r.DB().Exec(`INSERT INTO user_tunnel(user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status) VALUES(?, ?, NULL, ?, ?, ?, ?, ?, ?, ?)`,
		1, tunnelID, 99999, tunnelFlowGB, tunnelInFlow, tunnelOutFlow, 1, tunnelExpTimeMs, 1).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}

	t.Run("default user subscription payload", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/open_api/sub_store?user=admin_user&pwd=admin_user", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}

		expected := "upload=0; download=0; total=107373108658176; expire=2727251700"
		if string(body) != expected {
			t.Fatalf("expected body %q, got %q", expected, string(body))
		}
		if got := resp.Header().Get("subscription-userinfo"); got != expected {
			t.Fatalf("expected subscription-userinfo %q, got %q", expected, got)
		}
		if !strings.Contains(resp.Header().Get("Content-Type"), "text/plain") {
			t.Fatalf("expected text/plain content type, got %q", resp.Header().Get("Content-Type"))
		}
	})

	t.Run("tunnel scoped subscription payload", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/open_api/sub_store?user=admin_user&pwd=admin_user&tunnel="+strconv.FormatInt(tunnelID, 10), nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}

		expected := "upload=123; download=456; total=536870912000; expire=2727251700"
		if string(body) != expected {
			t.Fatalf("expected body %q, got %q", expected, string(body))
		}
		if got := resp.Header().Get("subscription-userinfo"); got != expected {
			t.Fatalf("expected subscription-userinfo %q, got %q", expected, got)
		}
	})

	t.Run("invalid credentials returns contract error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/open_api/sub_store?user=admin_user&pwd=wrong", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assertCodeMsg(t, resp, -1, "鉴权失败")
	})

	t.Run("missing tunnel returns contract error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/open_api/sub_store?user=admin_user&pwd=admin_user&tunnel=999999", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assertCodeMsg(t, resp, -1, "隧道不存在")
	})
}

func TestLegacyPasswordMigratesOnSubStore(t *testing.T) {
	router, r := setupContractRouter(t, "contract-jwt-secret")
	legacyChangedAt := seedLegacyUser(t, r, 9102, "legacy-substore-user", "legacy-substore-pass")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/open_api/sub_store?user=legacy-substore-user&pwd=legacy-substore-pass", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	expected := "upload=0; download=0; total=107373108658176; expire=2727251700"
	if string(body) != expected {
		t.Fatalf("expected body %q, got %q", expected, string(body))
	}
	assertUserPasswordIsBcrypt(t, r, "legacy-substore-user", "legacy-substore-pass")
	if changedAt := mustQueryPasswordChangedAtByUsername(t, r, "legacy-substore-user"); changedAt <= legacyChangedAt {
		t.Fatalf("expected password_changed_at to advance on sub-store migration, got %d <= %d", changedAt, legacyChangedAt)
	}
}

func TestDisabledLegacyPasswordIsRejectedOnSubStoreWithoutMigration(t *testing.T) {
	router, r := setupContractRouter(t, "contract-jwt-secret")
	legacyChangedAt := seedLegacyUserWithStatus(t, r, 9106, "disabled-substore-user", "disabled-substore-pass", 0)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/open_api/sub_store?user=disabled-substore-user&pwd=disabled-substore-pass", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)
	assertCodeMsg(t, resp, -1, "账号被停用")

	user, err := r.GetUserByUsername("disabled-substore-user")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if user == nil {
		t.Fatal("expected disabled user to exist")
	}
	if ok, migrated := security.VerifyPassword(user.Pwd, "disabled-substore-pass"); !ok || !migrated {
		t.Fatalf("expected disabled user to remain legacy MD5, got (%v,%v) with hash %q", ok, migrated, user.Pwd)
	}
	if changedAt := mustQueryPasswordChangedAtByUsername(t, r, "disabled-substore-user"); changedAt != legacyChangedAt {
		t.Fatalf("expected password_changed_at to remain unchanged, got %d want %d", changedAt, legacyChangedAt)
	}
}

func TestUserCreateStoresStrongHash(t *testing.T) {
	router, r := setupContractRouter(t, "contract-jwt-secret")
	startedAt := time.Now().UnixMilli()
	adminToken, err := auth.GenerateToken(1, "admin_user", 0, "contract-jwt-secret")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	body := bytes.NewBufferString(`{"user":"created-user","pwd":"created-pass"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/create", body)
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)
	assertCode(t, resp, 0)
	assertUserPasswordIsBcrypt(t, r, "created-user", "created-pass")
	if changedAt := mustQueryPasswordChangedAtByUsername(t, r, "created-user"); changedAt < startedAt {
		t.Fatalf("expected password_changed_at to be set on create, got %d < %d", changedAt, startedAt)
	}
}

func TestUserUpdateStoresStrongHash(t *testing.T) {
	router, r := setupContractRouter(t, "contract-jwt-secret")
	seedLegacyUser(t, r, 9103, "legacy-update-user", "legacy-update-pass")
	startedAt := time.Now().UnixMilli()
	adminToken, err := auth.GenerateToken(1, "admin_user", 0, "contract-jwt-secret")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	body := bytes.NewBufferString(`{"id":9103,"user":"updated-user","pwd":"updated-pass","flow":99999,"num":99999,"expTime":2727251700000,"flowResetTime":1,"status":1,"maxConn":0}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/update", body)
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)
	assertCode(t, resp, 0)
	assertUserPasswordIsBcrypt(t, r, "updated-user", "updated-pass")
	if changedAt := mustQueryPasswordChangedAtByUsername(t, r, "updated-user"); changedAt < startedAt {
		t.Fatalf("expected password_changed_at to be updated on user update, got %d < %d", changedAt, startedAt)
	}
}

func TestUpdatePasswordStoresStrongHash(t *testing.T) {
	router, r := setupContractRouter(t, "contract-jwt-secret")
	seedLegacyUser(t, r, 9104, "legacy-self-user", "legacy-self-pass")
	startedAt := time.Now().UnixMilli()
	token, err := auth.GenerateToken(9104, "legacy-self-user", 1, "contract-jwt-secret")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	body := bytes.NewBufferString(`{"newUsername":"self-updated-user","currentPassword":"legacy-self-pass","newPassword":"self-updated-pass","confirmPassword":"self-updated-pass"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/updatePassword", body)
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)
	assertCode(t, resp, 0)
	assertUserPasswordIsBcrypt(t, r, "self-updated-user", "self-updated-pass")
	if changedAt := mustQueryPasswordChangedAtByUsername(t, r, "self-updated-user"); changedAt < startedAt {
		t.Fatalf("expected password_changed_at to be updated on password change, got %d < %d", changedAt, startedAt)
	}
}

func TestSpeedLimitTunnelsRouteRemoved(t *testing.T) {
	secret := "contract-jwt-secret"
	router, _ := setupContractRouter(t, secret)

	token, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/speed-limit/tunnels", nil)
	req.Header.Set("Authorization", token)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 after route removal, got %d", resp.Code)
	}
}

func TestBackupExportImportRestoreContracts(t *testing.T) {
	secret := "contract-jwt-secret"
	router, r := setupContractRouter(t, secret)
	seedContractUser(t, r, 2, "normal_user", 1, 1)

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}
	userToken, err := auth.GenerateToken(2, "normal_user", 1, secret)
	if err != nil {
		t.Fatalf("generate user token: %v", err)
	}

	key := "backup_contract_key"
	if err := r.DB().Exec(`
		INSERT INTO vite_config(name, value, time)
		VALUES(?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET value = excluded.value, time = excluded.time
	`, key, "v1", time.Now().UnixMilli()).Error; err != nil {
		t.Fatalf("seed config for backup contract: %v", err)
	}

	t.Run("non-admin is blocked on backup export", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/export", nil)
		req.Header.Set("Authorization", userToken)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)
		assertCodeMsg(t, resp, 403, "权限不足，仅管理员可操作")
	})

	t.Run("standard and duplicate export routes both work", func(t *testing.T) {
		payloadA := exportBackupPayload(t, router, "/api/v1/backup/export", adminToken)
		if len(payloadA.Configs) == 0 {
			t.Fatalf("expected exported configs, got none")
		}
		if _, ok := payloadA.Configs[key]; !ok {
			t.Fatalf("expected %q in exported configs", key)
		}

		payloadB := exportBackupPayload(t, router, "/api/v1/api/v1/backup/export", adminToken)
		if len(payloadB.Configs) == 0 {
			t.Fatalf("expected exported configs from duplicate-prefix route, got none")
		}
	})

	t.Run("backup import applies exported data", func(t *testing.T) {
		payload := exportBackupPayload(t, router, "/api/v1/backup/export", adminToken)
		payload.Configs[key] = "v2"
		raw, err := json.Marshal(backupImportPayload{Types: []string{"configs"}, backupExportPayload: payload})
		if err != nil {
			t.Fatalf("marshal import payload: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/import", bytes.NewReader(raw))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)
		var out response.R
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode import response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected import code 0, got %d (%s)", out.Code, out.Msg)
		}

		cfg, err := r.GetConfigByName(key)
		if err != nil {
			t.Fatalf("query imported config: %v", err)
		}
		if cfg == nil || cfg.Value != "v2" {
			t.Fatalf("expected imported config value v2, got %+v", cfg)
		}
	})

	t.Run("backup restore alias applies exported data", func(t *testing.T) {
		payload := exportBackupPayload(t, router, "/api/v1/backup/export", adminToken)
		payload.Configs[key] = "v3"
		raw, err := json.Marshal(backupImportPayload{Types: []string{"configs"}, backupExportPayload: payload})
		if err != nil {
			t.Fatalf("marshal restore payload: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/restore", bytes.NewReader(raw))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)
		var out response.R
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode restore response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected restore code 0, got %d (%s)", out.Code, out.Msg)
		}

		cfg, err := r.GetConfigByName(key)
		if err != nil {
			t.Fatalf("query restored config: %v", err)
		}
		if cfg == nil || cfg.Value != "v3" {
			t.Fatalf("expected restored config value v3, got %+v", cfg)
		}
	})

	t.Run("backup export and import preserve forward ports", func(t *testing.T) {
		now := time.Now().UnixMilli()

		if err := r.DB().Exec(`
			INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, "backup-forward-tunnel", 1.0, 1, "tls", 0, now, now, 1, "", 88).Error; err != nil {
			t.Fatalf("seed tunnel for forward backup: %v", err)
		}
		tunnelID := mustLastInsertID(t, r, "backup-forward-tunnel")

		if err := r.DB().Exec(`
			INSERT INTO forward(user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx, proxy_protocol)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, 1, "admin_user", "backup-forward", tunnelID, "127.0.0.1:9000", "fifo", 0, 0, now, now, 1, 88, 2).Error; err != nil {
			t.Fatalf("seed forward for backup: %v", err)
		}
		forwardID := mustLastInsertID(t, r, "backup-forward")

		expected := map[int64]int{
			2001: 21001,
			2002: 21002,
		}
		for nodeID, port := range expected {
			if err := r.DB().Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(?, ?, ?)`, forwardID, nodeID, port).Error; err != nil {
				t.Fatalf("seed forward_port %d:%d: %v", nodeID, port, err)
			}
		}

		exportReq := httptest.NewRequest(http.MethodPost, "/api/v1/backup/export", bytes.NewBufferString(`{"types":["forwards"]}`))
		exportReq.Header.Set("Authorization", adminToken)
		exportReq.Header.Set("Content-Type", "application/json")
		exportResp := httptest.NewRecorder()
		router.ServeHTTP(exportResp, exportReq)

		if exportResp.Code != http.StatusOK {
			t.Fatalf("expected export status 200, got %d", exportResp.Code)
		}

		exportBody, err := io.ReadAll(exportResp.Body)
		if err != nil {
			t.Fatalf("read forwards backup body: %v", err)
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(exportBody, &payload); err != nil {
			t.Fatalf("decode forwards backup payload: %v", err)
		}
		version, _ := payload["version"].(string)
		if strings.TrimSpace(version) == "" {
			t.Fatalf("expected backup payload version, body=%s", string(exportBody))
		}

		forwardsRaw, ok := payload["forwards"].([]interface{})
		if !ok {
			t.Fatalf("expected forwards array in payload, body=%s", string(exportBody))
		}

		foundForward := false
		foundPorts := map[int64]int{}
		for _, item := range forwardsRaw {
			forwardMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			idValue, ok := forwardMap["id"].(float64)
			if !ok || int64(idValue) != forwardID {
				continue
			}
			foundForward = true

			portsRaw, ok := forwardMap["forwardPorts"].([]interface{})
			if !ok {
				t.Fatalf("expected forwardPorts for forward %d in payload", forwardID)
			}
			if proxyProtocol, ok := forwardMap["proxyProtocol"].(float64); !ok || int(proxyProtocol) != 2 {
				t.Fatalf("expected exported proxyProtocol 2 for forward %d, got %v", forwardID, forwardMap["proxyProtocol"])
			}
			for _, p := range portsRaw {
				portMap, ok := p.(map[string]interface{})
				if !ok {
					continue
				}
				nodeID, nodeOK := portMap["nodeId"].(float64)
				port, portOK := portMap["port"].(float64)
				if nodeOK && portOK {
					foundPorts[int64(nodeID)] = int(port)
				}
			}
			break
		}

		if !foundForward {
			t.Fatalf("expected forward %d in exported forwards payload", forwardID)
		}
		if len(foundPorts) != len(expected) {
			t.Fatalf("expected %d exported forward ports, got %d", len(expected), len(foundPorts))
		}
		for nodeID, port := range expected {
			if got, ok := foundPorts[nodeID]; !ok || got != port {
				t.Fatalf("expected exported forward port node=%d port=%d, got %v", nodeID, port, foundPorts)
			}
		}

		if err := r.DB().Exec(`DELETE FROM forward_port WHERE forward_id = ?`, forwardID).Error; err != nil {
			t.Fatalf("clear forward_port before import: %v", err)
		}
		if err := r.DB().Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(?, ?, ?)`, forwardID, 9999, 39999).Error; err != nil {
			t.Fatalf("seed wrong forward_port before import: %v", err)
		}

		payload["types"] = []string{"forwards"}
		importBody, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal forwards import payload: %v", err)
		}

		importReq := httptest.NewRequest(http.MethodPost, "/api/v1/backup/import", bytes.NewReader(importBody))
		importReq.Header.Set("Authorization", adminToken)
		importReq.Header.Set("Content-Type", "application/json")
		importResp := httptest.NewRecorder()
		router.ServeHTTP(importResp, importReq)

		var out response.R
		if err := json.NewDecoder(importResp.Body).Decode(&out); err != nil {
			t.Fatalf("decode forwards import response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected forwards import code 0, got %d (%s)", out.Code, out.Msg)
		}

		after := mustQueryNodePorts(t, r, `SELECT node_id, port FROM forward_port WHERE forward_id = ? ORDER BY id ASC`, forwardID)

		if len(after) != len(expected) {
			t.Fatalf("expected %d forward ports after import, got %d (%v)", len(expected), len(after), after)
		}
		for nodeID, port := range expected {
			if got, ok := after[nodeID]; !ok || got != port {
				t.Fatalf("expected forward_port node=%d port=%d after import, got %v", nodeID, port, after)
			}
		}

		var proxyProtocol int
		if err := r.DB().Raw(`SELECT proxy_protocol FROM forward WHERE id = ?`, forwardID).Row().Scan(&proxyProtocol); err != nil {
			t.Fatalf("query proxy_protocol after import: %v", err)
		}
		if proxyProtocol != 2 {
			t.Fatalf("expected proxy_protocol 2 after import, got %d", proxyProtocol)
		}
	})

	t.Run("backup export tolerates nullable legacy tunnel chain fields", func(t *testing.T) {
		now := time.Now().UnixMilli()
		if err := r.DB().Exec(`
			INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, "legacy-null-chain", 1.0, 1, "tls", 1000, now, now, 1, nil, 1).Error; err != nil {
			t.Fatalf("seed tunnel for nullable chain export: %v", err)
		}
		tunnelID := mustLastInsertID(t, r, "legacy-null-chain")

		if err := r.DB().Exec(`
			INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
			VALUES(?, ?, ?, ?, ?, ?, ?)
		`, tunnelID, "1", 1, nil, nil, nil, nil).Error; err != nil {
			t.Fatalf("seed nullable chain_tunnel row: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/export", bytes.NewBufferString(`{"types":["tunnels"]}`))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", resp.Code)
		}

		var payload struct {
			Version string `json:"version"`
			Tunnels []struct {
				ID           int64 `json:"id"`
				ChainTunnels []struct {
					Inx      int    `json:"inx"`
					Strategy string `json:"strategy"`
					Protocol string `json:"protocol"`
				} `json:"chainTunnels"`
			} `json:"tunnels"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode tunnels backup payload: %v", err)
		}
		if strings.TrimSpace(payload.Version) == "" {
			t.Fatalf("expected backup payload version, got empty")
		}

		found := false
		for _, tunnel := range payload.Tunnels {
			if tunnel.ID != tunnelID {
				continue
			}
			if len(tunnel.ChainTunnels) != 1 {
				t.Fatalf("expected one chain tunnel for seeded tunnel %d, got %d", tunnelID, len(tunnel.ChainTunnels))
			}
			if tunnel.ChainTunnels[0].Inx != 0 {
				t.Fatalf("expected nullable chain inx to export as 0, got %d", tunnel.ChainTunnels[0].Inx)
			}
			if tunnel.ChainTunnels[0].Strategy != "" {
				t.Fatalf("expected nullable chain strategy to export as empty string, got %q", tunnel.ChainTunnels[0].Strategy)
			}
			if tunnel.ChainTunnels[0].Protocol != "" {
				t.Fatalf("expected nullable chain protocol to export as empty string, got %q", tunnel.ChainTunnels[0].Protocol)
			}
			found = true
			break
		}
		if !found {
			t.Fatalf("expected seeded tunnel %d in backup export", tunnelID)
		}
	})
}

func TestBackupConfigFilteringContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, r := setupContractRouter(t, secret)

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	configs := map[string]string{
		"app_name":              "contract-before",
		"cloudflare_site_key":   "site-key-before",
		"jwt_secret":            "jwt-before",
		"license_key":           "license-before",
		"cloudflare_secret_key": "cloudflare-before",
	}
	for name, value := range configs {
		if err := r.DB().Exec(`
			INSERT INTO vite_config(name, value, time)
			VALUES(?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET value = excluded.value, time = excluded.time
		`, name, value, time.Now().UnixMilli()).Error; err != nil {
			t.Fatalf("seed config %s: %v", name, err)
		}
	}

	payload := exportBackupPayload(t, router, "/api/v1/backup/export", adminToken)
	for _, key := range []string{"jwt_secret", "license_key", "cloudflare_secret_key"} {
		if _, ok := payload.Configs[key]; ok {
			t.Fatalf("expected %s to be omitted from exported configs: %+v", key, payload.Configs)
		}
	}
	if payload.Configs["app_name"] != "contract-before" {
		t.Fatalf("expected public config to be exported, got %+v", payload.Configs)
	}

	payload.Configs["app_name"] = "contract-after"
	payload.Configs["jwt_secret"] = "jwt-after"
	payload.Configs["license_key"] = "license-after"
	payload.Configs["cloudflare_secret_key"] = "cloudflare-after"
	raw, err := json.Marshal(backupImportPayload{Types: []string{"configs"}, backupExportPayload: payload})
	if err != nil {
		t.Fatalf("marshal import payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/import", bytes.NewReader(raw))
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)
	var out response.R
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode import response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected import code 0, got %d (%s)", out.Code, out.Msg)
	}

	assertConfigValue(t, r, "app_name", "contract-after")
	assertConfigValue(t, r, "jwt_secret", "jwt-before")
	assertConfigValue(t, r, "license_key", "license-before")
	assertConfigValue(t, r, "cloudflare_secret_key", "cloudflare-before")
}

type backupExportPayload struct {
	Version    string            `json:"version"`
	ExportedAt int64             `json:"exportedAt"`
	Configs    map[string]string `json:"configs"`
}

type backupImportPayload struct {
	Types []string `json:"types"`
	backupExportPayload
}

func exportBackupPayload(t *testing.T, router http.Handler, path, token string) backupExportPayload {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{"types":["configs"]}`))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200 on %s, got %d", path, resp.Code)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read backup payload from %s: %v", path, err)
	}

	var payload backupExportPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode backup payload from %s: %v", path, err)
	}
	if strings.TrimSpace(payload.Version) == "" {
		var out response.R
		if err := json.Unmarshal(body, &out); err == nil {
			t.Fatalf("expected backup payload on %s, got envelope code=%d msg=%q", path, out.Code, out.Msg)
		}
		t.Fatalf("expected non-empty backup payload version on %s, body=%s", path, string(body))
	}
	if payload.Configs == nil {
		t.Fatalf("expected configs map in backup payload on %s", path)
	}
	return payload
}

func setupContractRouter(t *testing.T, jwtSecret string) (http.Handler, *repo.Repository) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "contract.db")
	r, err := repo.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
	})

	h := handler.New(r, jwtSecret)
	return httpserver.NewRouter(h, jwtSecret, ""), r
}

func assertConfigValue(t *testing.T, r *repo.Repository, name, want string) {
	t.Helper()
	cfg, err := r.GetConfigByName(name)
	if err != nil {
		t.Fatalf("get config %s: %v", name, err)
	}
	if cfg == nil || cfg.Value != want {
		t.Fatalf("expected config %s=%q, got %+v", name, want, cfg)
	}
}

func seedLegacyUser(t *testing.T, r *repo.Repository, id int64, username, password string) int64 {
	return seedLegacyUserWithStatus(t, r, id, username, password, 1)
}

func seedContractUser(t *testing.T, r *repo.Repository, id int64, username string, roleID, status int) int64 {
	t.Helper()
	now := time.Now().UnixMilli()
	passwordChangedAt := now - 10_000
	if err := r.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, max_conn, created_time, updated_time, status, password_changed_at)
		VALUES(?, ?, ?, ?, 2727251700000, 99999, 0, 0, 1, 99999, 0, ?, ?, ?, ?)
	`, id, username, security.MD5("contract-pass"), roleID, now, now, status, passwordChangedAt).Error; err != nil {
		t.Fatalf("seed contract user %s: %v", username, err)
	}
	return passwordChangedAt
}

func seedLegacyUserWithStatus(t *testing.T, r *repo.Repository, id int64, username, password string, status int) int64 {
	t.Helper()
	now := time.Now().UnixMilli()
	legacyChangedAt := now - 10_000
	if err := r.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, max_conn, created_time, updated_time, status, password_changed_at)
		VALUES(?, ?, ?, 1, 2727251700000, 99999, 0, 0, 1, 99999, 0, ?, ?, ?, ?)
	`, id, username, security.MD5(password), now, now, status, legacyChangedAt).Error; err != nil {
		t.Fatalf("seed legacy user %s: %v", username, err)
	}
	return legacyChangedAt
}

func mustQueryPasswordChangedAtByUsername(t *testing.T, r *repo.Repository, username string) int64 {
	t.Helper()
	return mustQueryInt64(t, r, `SELECT password_changed_at FROM user WHERE user = ?`, username)
}

func assertUserPasswordIsBcrypt(t *testing.T, r *repo.Repository, username, password string) {
	t.Helper()
	user, err := r.GetUserByUsername(username)
	if err != nil {
		t.Fatalf("get user %s: %v", username, err)
	}
	if user == nil {
		t.Fatalf("expected user %s to exist", username)
	}
	if ok, migrated := security.VerifyPassword(user.Pwd, password); !ok || migrated {
		t.Fatalf("expected bcrypt password for %s, got %q", username, user.Pwd)
	}
}

func TestOpenMigratesLegacyNodeDualStackColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-2.0.7-beta.db")
	legacyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy sqlite: %v", err)
	}

	t.Cleanup(func() {
		_ = legacyDB.Close()
	})

	if _, err := legacyDB.Exec(`
		CREATE TABLE IF NOT EXISTS node (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(100) NOT NULL,
			secret VARCHAR(100) NOT NULL,
			server_ip VARCHAR(100) NOT NULL,
			port TEXT NOT NULL,
			interface_name VARCHAR(200),
			version VARCHAR(100),
			http INTEGER NOT NULL DEFAULT 0,
			tls INTEGER NOT NULL DEFAULT 0,
			socks INTEGER NOT NULL DEFAULT 0,
			created_time INTEGER NOT NULL,
			updated_time INTEGER,
			status INTEGER NOT NULL,
			tcp_listen_addr VARCHAR(100) NOT NULL DEFAULT '[::]',
			udp_listen_addr VARCHAR(100) NOT NULL DEFAULT '[::]'
		)
	`); err != nil {
		t.Fatalf("create legacy node table: %v", err)
	}

	if _, err := legacyDB.Exec(`
		CREATE TABLE IF NOT EXISTS tunnel (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(100) NOT NULL,
			traffic_ratio REAL NOT NULL DEFAULT 1.0,
			type INTEGER NOT NULL,
			protocol VARCHAR(10) NOT NULL DEFAULT 'tls',
			flow INTEGER NOT NULL,
			created_time INTEGER NOT NULL,
			updated_time INTEGER NOT NULL,
			status INTEGER NOT NULL,
			in_ip TEXT
		)
	`); err != nil {
		t.Fatalf("create legacy tunnel table: %v", err)
	}

	now := time.Now().UnixMilli()
	if _, err := legacyDB.Exec(`
		INSERT INTO node(name, secret, server_ip, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "legacy-node", "legacy-secret", "10.10.0.1", "10000-10010", "eth0", "v-old", 1, 1, 1, now, now, 1, "[::]", "[::]"); err != nil {
		t.Fatalf("seed legacy node row: %v", err)
	}

	r, err := repo.Open(dbPath)
	if err != nil {
		t.Fatalf("open migrated sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
	})

	nodes, err := r.ListNodes()
	if err != nil {
		t.Fatalf("list nodes after migration: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node after migration, got %d", len(nodes))
	}

	columns := readTableColumns(t, r.DB(), "node")

	for _, required := range []string{"server_ip_v4", "server_ip_v6", "inx", "extra_ips"} {
		if !columns[required] {
			t.Fatalf("expected node column %q to exist after migration", required)
		}
	}

	tunnelColumns := readTableColumns(t, r.DB(), "tunnel")
	for _, required := range []string{"inx", "ip_preference"} {
		if !tunnelColumns[required] {
			t.Fatalf("expected tunnel column %q to exist after migration", required)
		}
	}
}

func TestOpenMigratesVeryLegacyNodeAndTunnelColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-1.x.db")
	legacyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy sqlite: %v", err)
	}

	t.Cleanup(func() {
		_ = legacyDB.Close()
	})

	if _, err := legacyDB.Exec(`
		CREATE TABLE IF NOT EXISTS node (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(100) NOT NULL,
			secret VARCHAR(100) NOT NULL,
			server_ip VARCHAR(100) NOT NULL,
			port TEXT NOT NULL,
			interface_name VARCHAR(200),
			version VARCHAR(100),
			http INTEGER NOT NULL DEFAULT 0,
			tls INTEGER NOT NULL DEFAULT 0,
			socks INTEGER NOT NULL DEFAULT 0,
			created_time INTEGER NOT NULL,
			updated_time INTEGER,
			status INTEGER NOT NULL
		)
	`); err != nil {
		t.Fatalf("create very legacy node table: %v", err)
	}

	if _, err := legacyDB.Exec(`
		CREATE TABLE IF NOT EXISTS tunnel (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(100) NOT NULL,
			traffic_ratio REAL NOT NULL DEFAULT 1.0,
			type INTEGER NOT NULL,
			protocol VARCHAR(10) NOT NULL DEFAULT 'tls',
			flow INTEGER NOT NULL,
			created_time INTEGER NOT NULL,
			updated_time INTEGER NOT NULL,
			status INTEGER NOT NULL,
			in_ip TEXT
		)
	`); err != nil {
		t.Fatalf("create very legacy tunnel table: %v", err)
	}

	now := time.Now().UnixMilli()
	if _, err := legacyDB.Exec(`
		INSERT INTO node(name, secret, server_ip, port, interface_name, version, http, tls, socks, created_time, updated_time, status)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "legacy-node", "legacy-secret", "10.10.0.1", "10000-10010", "eth0", "v-old", 1, 1, 1, now, now, 1); err != nil {
		t.Fatalf("seed legacy node row: %v", err)
	}

	r, err := repo.Open(dbPath)
	if err != nil {
		t.Fatalf("open migrated sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
	})

	columns := readTableColumns(t, r.DB(), "node")
	for _, required := range []string{
		"server_ip_v4",
		"server_ip_v6",
		"extra_ips",
		"tcp_listen_addr",
		"udp_listen_addr",
		"inx",
		"is_remote",
		"remote_url",
		"remote_token",
		"remote_config",
	} {
		if !columns[required] {
			t.Fatalf("expected node column %q to exist after migration", required)
		}
	}

	tunnelColumns := readTableColumns(t, r.DB(), "tunnel")
	for _, required := range []string{"inx", "ip_preference"} {
		if !tunnelColumns[required] {
			t.Fatalf("expected tunnel column %q to exist after migration", required)
		}
	}
}

func readTableColumns(t *testing.T, db *gorm.DB, table string) map[string]bool {
	t.Helper()

	columnTypes, err := db.Migrator().ColumnTypes(table)
	if err != nil {
		t.Fatalf("inspect %s columns: %v", table, err)
	}

	columns := map[string]bool{}
	for _, col := range columnTypes {
		name := strings.TrimSpace(col.Name())
		if name == "" {
			continue
		}
		columns[name] = true
	}

	return columns
}
