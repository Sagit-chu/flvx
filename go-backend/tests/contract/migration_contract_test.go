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
	"go-backend/internal/store"
	"go-backend/internal/store/sqlite"

	_ "modernc.org/sqlite"
)

func TestCaptchaVerifyLoginContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)

	_, err := repo.DB().Exec(`
		INSERT INTO vite_config(name, value, time)
		VALUES(?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET value = excluded.value, time = excluded.time
	`, "captcha_enabled", "true", time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("enable captcha: %v", err)
	}

	t.Run("login denied without verified captcha token", func(t *testing.T) {
		body := bytes.NewBufferString(`{"username":"admin_user","password":"admin_user","captchaId":""}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/login", body)
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assertCodeMsg(t, resp, -1, "验证码校验失败")
	})

	t.Run("captcha token is one-time and consumed by login", func(t *testing.T) {
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

		loginBody := bytes.NewBufferString(`{"username":"admin_user","password":"admin_user","captchaId":"captcha-token-1"}`)
		loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/user/login", loginBody)
		loginReq.Header.Set("Content-Type", "application/json")
		loginResp := httptest.NewRecorder()
		router.ServeHTTP(loginResp, loginReq)
		assertCode(t, loginResp, 0)

		replayBody := bytes.NewBufferString(`{"username":"admin_user","password":"admin_user","captchaId":"captcha-token-1"}`)
		replayReq := httptest.NewRequest(http.MethodPost, "/api/v1/user/login", replayBody)
		replayReq.Header.Set("Content-Type", "application/json")
		replayResp := httptest.NewRecorder()
		router.ServeHTTP(replayResp, replayReq)
		assertCodeMsg(t, replayResp, -1, "验证码校验失败")
	})
}

func TestOpenAPISubStoreContracts(t *testing.T) {
	router, repo := setupContractRouter(t, "contract-jwt-secret")

	const tunnelFlowGB = int64(500)
	const tunnelInFlow = int64(123)
	const tunnelOutFlow = int64(456)
	const tunnelExpTimeMs = int64(2727251700000)

	now := time.Now().UnixMilli()
	res, err := repo.DB().Exec(`INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"contract-tunnel", 1.0, 1, "tls", 1, now, now, 1, nil, 0)
	if err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	if _, err := repo.DB().Exec(`INSERT INTO user_tunnel(user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status) VALUES(?, ?, NULL, ?, ?, ?, ?, ?, ?, ?)`,
		1, tunnelID, 99999, tunnelFlowGB, tunnelInFlow, tunnelOutFlow, 1, tunnelExpTimeMs, 1); err != nil {
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

func TestSpeedLimitTunnelsRouteAlias(t *testing.T) {
	secret := "contract-jwt-secret"
	router, _ := setupContractRouter(t, secret)

	t.Run("missing token blocked", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/speed-limit/tunnels", nil)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		assertCodeMsg(t, resp, 401, "未登录或token已过期")
	})

	t.Run("admin token receives success envelope", func(t *testing.T) {
		token, err := auth.GenerateToken(1, "admin_user", 0, secret)
		if err != nil {
			t.Fatalf("generate token: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/v1/speed-limit/tunnels", nil)
		req.Header.Set("Authorization", token)
		resp := httptest.NewRecorder()

		router.ServeHTTP(resp, req)

		var out response.R
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d (%s)", out.Code, out.Msg)
		}
	})
}

func TestBackupExportImportRestoreContracts(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}
	userToken, err := auth.GenerateToken(2, "normal_user", 1, secret)
	if err != nil {
		t.Fatalf("generate user token: %v", err)
	}

	key := "backup_contract_key"
	if _, err := repo.DB().Exec(`
		INSERT INTO vite_config(name, value, time)
		VALUES(?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET value = excluded.value, time = excluded.time
	`, key, "v1", time.Now().UnixMilli()); err != nil {
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

		cfg, err := repo.GetConfigByName(key)
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

		cfg, err := repo.GetConfigByName(key)
		if err != nil {
			t.Fatalf("query restored config: %v", err)
		}
		if cfg == nil || cfg.Value != "v3" {
			t.Fatalf("expected restored config value v3, got %+v", cfg)
		}
	})

	t.Run("backup export tolerates nullable legacy tunnel chain fields", func(t *testing.T) {
		now := time.Now().UnixMilli()
		res, err := repo.DB().Exec(`
			INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, "legacy-null-chain", 1.0, 1, "tls", 1000, now, now, 1, nil, 1)
		if err != nil {
			t.Fatalf("seed tunnel for nullable chain export: %v", err)
		}
		tunnelID, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("read tunnel id for nullable chain export: %v", err)
		}

		if _, err := repo.DB().Exec(`
			INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
			VALUES(?, ?, ?, ?, ?, ?, ?)
		`, tunnelID, "1", 1, nil, nil, nil, nil); err != nil {
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

func setupContractRouter(t *testing.T, jwtSecret string) (http.Handler, *sqlite.Repository) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "contract.db")
	repo, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
	})

	h := handler.New(repo, jwtSecret)
	return httpserver.NewRouter(h, jwtSecret), repo
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

	repo, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("open migrated sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
	})

	nodes, err := repo.ListNodes()
	if err != nil {
		t.Fatalf("list nodes after migration: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node after migration, got %d", len(nodes))
	}

	columns := readTableColumns(t, repo.DB(), "node")

	for _, required := range []string{"server_ip_v4", "server_ip_v6", "inx"} {
		if !columns[required] {
			t.Fatalf("expected node column %q to exist after migration", required)
		}
	}

	tunnelColumns := readTableColumns(t, repo.DB(), "tunnel")
	if !tunnelColumns["inx"] {
		t.Fatalf("expected tunnel column %q to exist after migration", "inx")
	}
}

func readTableColumns(t *testing.T, db *store.DB, table string) map[string]bool {
	t.Helper()

	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("inspect %s columns: %v", table, err)
	}
	defer rows.Close()

	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan %s pragma row: %v", table, err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate %s pragma rows: %v", table, err)
	}

	return columns
}
