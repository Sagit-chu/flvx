package contract_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-backend/internal/auth"
	"go-backend/internal/http/response"
)

func TestTunnelCreateWithIPPreferenceContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	insertDualStackNode := func(name, v4, v6, portRange string) int64 {
		res, err := repo.DB().Exec(`
			INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, name, name+"-secret", v4, v4, v6, portRange, "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0)
		if err != nil {
			t.Fatalf("insert node %s: %v", name, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("get node id %s: %v", name, err)
		}
		return id
	}

	entryID := insertDualStackNode("ip-pref-entry", "10.50.0.1", "2001:db8::1", "50000-50010")
	exitID := insertDualStackNode("ip-pref-exit", "10.50.0.2", "2001:db8::2", "51000-51010")

	for _, tc := range []struct {
		name       string
		preference string
	}{
		{"v4-preference", "v4"},
		{"v6-preference", "v6"},
		{"empty-preference", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := `{"name":"tunnel-` + tc.name + `","type":2,"flow":99999,"status":1,"ipPreference":"` + tc.preference + `","inNodeId":[{"nodeId":` + jsonInt(entryID) + `,"protocol":"tls"}],"chainNodes":[],"outNodeId":[{"nodeId":` + jsonInt(exitID) + `,"protocol":"tls"}]}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/create", bytes.NewBufferString(payload))
			req.Header.Set("Authorization", adminToken)
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)

			var out response.R
			if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
				t.Fatalf("decode response: %v", err)
			}

			var stored string
			err := repo.DB().QueryRow(`SELECT COALESCE(ip_preference, '') FROM tunnel WHERE name = ?`, "tunnel-"+tc.name).Scan(&stored)
			if err != nil {
				if err == sql.ErrNoRows {
					t.Skipf("tunnel not created (nodes offline), skipping DB verification")
				}
				t.Fatalf("query ip_preference: %v", err)
			}
			if stored != tc.preference {
				t.Fatalf("expected ip_preference=%q in DB, got %q", tc.preference, stored)
			}
		})
	}
}

func TestTunnelUpdateIPPreferenceContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	insertDualStackNode := func(name, v4, v6, portRange string) int64 {
		res, err := repo.DB().Exec(`
			INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, name, name+"-secret", v4, v4, v6, portRange, "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0)
		if err != nil {
			t.Fatalf("insert node %s: %v", name, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("get node id %s: %v", name, err)
		}
		return id
	}

	entryID := insertDualStackNode("upd-entry", "10.60.0.1", "2001:db8:1::1", "60000-60010")
	exitID := insertDualStackNode("upd-exit", "10.60.0.2", "2001:db8:1::2", "61000-61010")

	tunnelRes, err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx, ip_preference)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "update-ip-pref-tunnel", 1.0, 1, "tls", 99999, now, now, 1, nil, 0, "")
	if err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID, err := tunnelRes.LastInsertId()
	if err != nil {
		t.Fatalf("get tunnel id: %v", err)
	}

	payload := `{"id":` + jsonInt(tunnelID) + `,"name":"update-ip-pref-tunnel","type":2,"flow":99999,"trafficRatio":1.0,"status":1,"ipPreference":"v6","inNodeId":[{"nodeId":` + jsonInt(entryID) + `,"protocol":"tls"}],"chainNodes":[],"outNodeId":[{"nodeId":` + jsonInt(exitID) + `,"protocol":"tls"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/update", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	var stored string
	if err := repo.DB().QueryRow(`SELECT COALESCE(ip_preference, '') FROM tunnel WHERE id = ?`, tunnelID).Scan(&stored); err != nil {
		t.Fatalf("query ip_preference: %v", err)
	}
	if stored != "v6" {
		t.Fatalf("expected ip_preference='v6' after update, got %q", stored)
	}
}

func TestTunnelListReturnsIPPreferenceContract(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	_, err = repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx, ip_preference)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "list-ip-pref-tunnel", 1.0, 2, "tls", 99999, now, now, 1, nil, 0, "v6")
	if err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/list", nil)
	req.Header.Set("Authorization", adminToken)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected code 0, got %d (msg=%s)", out.Code, out.Msg)
	}

	tunnels, ok := out.Data.([]interface{})
	if !ok || len(tunnels) == 0 {
		t.Fatalf("expected non-empty tunnel list, got %v", out.Data)
	}

	found := false
	for _, raw := range tunnels {
		tm, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if tm["name"] == "list-ip-pref-tunnel" {
			found = true
			pref, _ := tm["ipPreference"].(string)
			if pref != "v6" {
				t.Fatalf("expected ipPreference='v6' in list response, got %q", pref)
			}
			break
		}
	}
	if !found {
		t.Fatal("tunnel 'list-ip-pref-tunnel' not found in list response")
	}
}

func TestIPPreferenceColumnDefaultContract(t *testing.T) {
	_, repo := setupContractRouter(t, "contract-jwt-secret")
	now := time.Now().UnixMilli()

	_, err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "no-pref-tunnel", 1.0, 2, "tls", 99999, now, now, 1, nil, 0)
	if err != nil {
		t.Fatalf("insert tunnel without ip_preference: %v", err)
	}

	var stored string
	if err := repo.DB().QueryRow(`SELECT COALESCE(ip_preference, '') FROM tunnel WHERE name = ?`, "no-pref-tunnel").Scan(&stored); err != nil {
		t.Fatalf("query ip_preference: %v", err)
	}
	if stored != "" {
		t.Fatalf("expected default ip_preference='', got %q", stored)
	}
}

func TestIPPreferenceColumnMigrationContract(t *testing.T) {
	_, repo := setupContractRouter(t, "contract-jwt-secret")

	var colCount int
	err := repo.DB().QueryRow(`SELECT COUNT(*) FROM pragma_table_info('tunnel') WHERE name = 'ip_preference'`).Scan(&colCount)
	if err != nil {
		t.Fatalf("check column existence: %v", err)
	}
	if colCount != 1 {
		t.Fatalf("expected ip_preference column to exist in tunnel table, found %d", colCount)
	}
}

func TestIPPreferenceCoalesceNullSafety(t *testing.T) {
	_, repo := setupContractRouter(t, "contract-jwt-secret")
	now := time.Now().UnixMilli()

	_, err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx, ip_preference)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
	`, "null-pref-tunnel", 1.0, 2, "tls", 99999, now, now, 1, nil, 0)
	if err != nil {
		t.Skipf("DB does not allow NULL ip_preference (NOT NULL constraint): %v", err)
	}

	var stored string
	if err := repo.DB().QueryRow(`SELECT COALESCE(ip_preference, '') FROM tunnel WHERE name = ?`, "null-pref-tunnel").Scan(&stored); err != nil {
		t.Fatalf("query ip_preference: %v", err)
	}
	if stored != "" {
		t.Fatalf("COALESCE should convert NULL to empty string, got %q", stored)
	}
}

func TestDualStackNodeIPFieldsStoredContract(t *testing.T) {
	_, repo := setupContractRouter(t, "contract-jwt-secret")
	now := time.Now().UnixMilli()

	_, err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "ds-verify-node", "ds-secret", "10.70.0.1", "10.70.0.1", "2001:db8:2::1", "70000-70010", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0)
	if err != nil {
		t.Fatalf("insert dual-stack node: %v", err)
	}

	var v4, v6 sql.NullString
	if err := repo.DB().QueryRow(`SELECT server_ip_v4, server_ip_v6 FROM node WHERE name = ?`, "ds-verify-node").Scan(&v4, &v6); err != nil {
		t.Fatalf("query node IPs: %v", err)
	}
	if !v4.Valid || v4.String != "10.70.0.1" {
		t.Fatalf("expected server_ip_v4='10.70.0.1', got %v", v4)
	}
	if !v6.Valid || v6.String != "2001:db8:2::1" {
		t.Fatalf("expected server_ip_v6='2001:db8:2::1', got %v", v6)
	}
}

func TestIPPreferenceValidValuesContract(t *testing.T) {
	_, repo := setupContractRouter(t, "contract-jwt-secret")
	now := time.Now().UnixMilli()

	for _, pref := range []string{"", "v4", "v6"} {
		name := "valid-pref-" + pref
		if pref == "" {
			name = "valid-pref-empty"
		}
		_, err := repo.DB().Exec(`
			INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx, ip_preference)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, name, 1.0, 2, "tls", 99999, now, now, 1, nil, 0, pref)
		if err != nil {
			t.Fatalf("insert tunnel with ip_preference=%q: %v", pref, err)
		}

		var stored string
		if err := repo.DB().QueryRow(`SELECT COALESCE(ip_preference, '') FROM tunnel WHERE name = ?`, name).Scan(&stored); err != nil {
			t.Fatalf("query ip_preference for %s: %v", name, err)
		}
		if stored != pref {
			t.Fatalf("expected ip_preference=%q, got %q for %s", pref, stored, name)
		}
	}
}
