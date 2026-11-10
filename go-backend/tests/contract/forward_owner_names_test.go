package contract_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"go-backend/internal/auth"
	httpserver "go-backend/internal/http"
	"go-backend/internal/http/handler"
	"go-backend/internal/security"
	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
)

func TestForwardOwnerNamesFollowAccountRenames(t *testing.T) {
	for _, database := range []string{"sqlite", "postgres"} {
		t.Run(database, func(t *testing.T) {
			for _, mode := range []string{"admin-name-only", "admin-name-and-password", "self-name-and-password"} {
				t.Run(mode, func(t *testing.T) {
					const secret = "issue574-isolated-test"
					open := ownerNamesRepository(t, database)
					r := open()
					router := httpserver.NewRouter(handler.New(r, secret), secret)
					for _, u := range []model.User{
						{ID: 9103, User: "before-name", Pwd: security.MD5("old-password"), RoleID: 1, Status: 1, PasswordChangedAt: time.Now().Add(-10 * time.Second).UnixMilli()},
						{ID: 9104, User: "other-user", Pwd: security.MD5("other-password"), RoleID: 1, Status: 1, PasswordChangedAt: time.Now().Add(-10 * time.Second).UnixMilli()},
					} {
						if err := r.DB().Create(&u).Error; err != nil {
							t.Fatal(err)
						}
					}
					exp := time.Now().Add(time.Hour).UnixMilli()
					f := model.Forward{ID: 9301, UserID: 9103, UserName: "before-name", Name: "existing-rule", TunnelID: 9201, RemoteAddr: "127.0.0.1:8080", Status: 1, InFlow: 123, OutFlow: 456}
					for _, v := range []interface{}{&model.Tunnel{ID: 9201, Name: "isolated", Type: 1, Protocol: "tls", Status: 1, Flow: 1}, &model.UserTunnel{ID: 9401, UserID: 9103, TunnelID: 9201, Status: 1, Flow: 100, Num: 10, ExpTime: exp}, &f} {
						if err := r.DB().Create(v).Error; err != nil {
							t.Fatal(err)
						}
					}
					token := func(id int64, name string, role int) string {
						v, e := auth.GenerateTokenAt(id, name, role, secret, time.Now().Add(time.Second))
						if e != nil {
							t.Fatal(e)
						}
						return v
					}
					admin := token(1, "admin_user", 0)
					oldToken, e := auth.GenerateTokenAt(9103, "before-name", 1, secret, time.Now().Add(-time.Second))
					if e != nil {
						t.Fatal(e)
					}
					call := func(path, tok string, body interface{}) (int, json.RawMessage) {
						b, err := json.Marshal(body)
						if err != nil {
							t.Fatal(err)
						}
						q := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
						q.Header.Set("Authorization", tok)
						q.Header.Set("Content-Type", "application/json")
						w := httptest.NewRecorder()
						router.ServeHTTP(w, q)
						var out struct {
							Code int             `json:"code"`
							Data json.RawMessage `json:"data"`
							Msg  string          `json:"msg"`
						}
						if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
							t.Fatal(e)
						}
						return out.Code, out.Data
					}
					path, tok := "/api/v1/user/update", admin
					body := map[string]interface{}{"id": 9103, "user": "after-name", "flow": 100, "num": 10, "expTime": exp, "flowResetTime": 1, "status": 1, "maxConn": 0}
					if mode == "admin-name-and-password" {
						body["pwd"] = "new-password"
					}
					if mode == "self-name-and-password" {
						path = "/api/v1/user/updatePassword"
						tok = oldToken
						body = map[string]interface{}{"newUsername": "after-name", "currentPassword": "old-password", "newPassword": "new-password", "confirmPassword": "new-password"}
					}
					if code, _ := call(path, tok, body); code != 0 {
						t.Fatalf("rename code=%d", code)
					}

					fresh := token(9103, "after-name", 1)
					code, data := call("/api/v1/forward/list", admin, map[string]interface{}{})
					if code != 0 {
						t.Fatal(code)
					}
					var rows []map[string]interface{}
					if err := json.Unmarshal(data, &rows); err != nil {
						t.Fatal(err)
					}
					if len(rows) != 1 || rows[0]["userName"] != "after-name" || rows[0]["userId"] != float64(9103) {
						t.Fatalf("expected current owner name: %s", data)
					}
					if rows[0]["id"] != float64(9301) || rows[0]["inFlow"] != float64(123) || rows[0]["outFlow"] != float64(456) || rows[0]["status"] != float64(1) {
						t.Fatalf("rule identity/traffic/status changed: %s", data)
					}
					if code, data = call("/api/v1/forward/list", fresh, map[string]interface{}{}); code != 0 {
						t.Fatalf("owner denied: %d", code)
					}
					rows = nil
					if err := json.Unmarshal(data, &rows); err != nil {
						t.Fatal(err)
					}
					if len(rows) != 1 || rows[0]["userName"] != "after-name" {
						t.Fatalf("owner cannot see renamed rule: %s", data)
					}
					code, data = call("/api/v1/tunnel/delete-preview", admin, map[string]interface{}{"id": 9201})
					if code != 0 {
						t.Fatalf("preview code=%d", code)
					}
					var preview struct {
						SampleForwards []struct {
							ID       int64  `json:"id"`
							UserID   int64  `json:"userId"`
							UserName string `json:"userName"`
						} `json:"sampleForwards"`
					}
					if err := json.Unmarshal(data, &preview); err != nil {
						t.Fatal(err)
					}
					if len(preview.SampleForwards) != 1 || preview.SampleForwards[0].ID != 9301 || preview.SampleForwards[0].UserID != 9103 || preview.SampleForwards[0].UserName != "after-name" {
						t.Fatalf("preview must show current owner: %s", data)
					}
					code, data = call("/api/v1/tunnel/batch-delete-preview", admin, map[string]interface{}{"ids": []int64{9201}})
					if code != 0 {
						t.Fatalf("batch preview code=%d", code)
					}
					var batch struct {
						Items []struct {
							SampleForwards []struct {
								UserName string `json:"userName"`
							} `json:"sampleForwards"`
						} `json:"items"`
					}
					if err := json.Unmarshal(data, &batch); err != nil {
						t.Fatal(err)
					}
					if len(batch.Items) != 1 || len(batch.Items[0].SampleForwards) != 1 || batch.Items[0].SampleForwards[0].UserName != "after-name" {
						t.Fatalf("batch preview must show current owner: %s", data)
					}
					exported, err := r.ExportAll()
					if err != nil {
						t.Fatal(err)
					}
					if len(exported.UserTunnels) != 1 || exported.UserTunnels[0].UserID != 9103 || exported.UserTunnels[0].TunnelID != 9201 || exported.UserTunnels[0].Flow != 100 || exported.UserTunnels[0].Status != 1 {
						t.Fatalf("rename changed tunnel grant: %+v", exported.UserTunnels)
					}
					oldCode, _ := call("/api/v1/forward/list", oldToken, map[string]interface{}{})
					wantOld := 0
					if mode != "admin-name-only" {
						wantOld = 401
					}
					if oldCode != wantOld {
						t.Fatalf("old token=%d want %d", oldCode, wantOld)
					}
					// Re-use the abandoned username for another identity; it must not acquire rules.
					if code, _ := call("/api/v1/user/update", admin, map[string]interface{}{"id": 9104, "user": "before-name", "flow": 100, "num": 10, "expTime": exp, "flowResetTime": 1, "status": 1, "maxConn": 0}); code != 0 {
						t.Fatalf("reuse old name: %d", code)
					}
					other := token(9104, "before-name", 1)
					code, data = call("/api/v1/forward/list", other, map[string]interface{}{})
					if code != 0 {
						t.Fatal(code)
					}
					rows = nil
					if err := json.Unmarshal(data, &rows); err != nil {
						t.Fatal(err)
					}
					if len(rows) != 0 {
						t.Fatalf("old name gained rules: %s", data)
					}
					if code, _ := call("/api/v1/forward/reset-flow", other, map[string]interface{}{"id": 9301}); code == 0 {
						t.Fatal("other user reset original rule")
					}
					if code, _ := call("/api/v1/forward/reset-flow", fresh, map[string]interface{}{"id": 9301}); code != 0 {
						t.Fatalf("owner reset denied: %d", code)
					}
				})
			}
		})
	}
}

// Each PostgreSQL test owns only its unique schema, never the shared test database.
func ownerNamesRepository(t *testing.T, database string) func() *repo.Repository {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "owners.db")
	if database == "postgres" {
		baseDSN := strings.TrimSpace(os.Getenv("FLVX_POSTGRES_TEST_DSN"))
		if baseDSN == "" {
			t.Skip("set FLVX_POSTGRES_TEST_DSN to run postgres owner-name contracts")
		}
		schema := "contract_owner_names_" + strconv.FormatInt(time.Now().UnixNano(), 36)
		db, err := sql.Open("pgx", baseDSN)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`CREATE SCHEMA "` + schema + `"`); err != nil {
			db.Close()
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`); _ = db.Close() })
		dsn, err = withSearchPath(baseDSN, schema)
		if err != nil {
			t.Fatal(err)
		}
	}
	return func() *repo.Repository {
		t.Helper()
		var r *repo.Repository
		var err error
		if database == "postgres" {
			r, err = repo.OpenPostgres(dsn)
		} else {
			r, err = repo.Open(dsn)
		}
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = r.Close() })
		return r
	}
}

func TestForwardOwnerNamesSurviveLegacyBackupAndRestart(t *testing.T) {
	for _, database := range []string{"sqlite", "postgres"} {
		t.Run(database, func(t *testing.T) {
			open := ownerNamesRepository(t, database)
			r := open()
			// A legacy backup stores the rule's original name, not the user's current one.
			const legacy = `{"users":[{"id":9103,"user":"current-owner","pwd":"test-only-password","roleId":1,"status":1}],
        "forwards":[
        {"id":9301,"userId":9103,"userName":"former-owner","name":"historical-rule","tunnelId":9201,"inFlow":123,"outFlow":456,"status":1},
        {"id":9302,"userId":9199,"userName":"missing-owner","name":"orphan-rule","tunnelId":9201,"status":1},
        {"id":9303,"userId":9198,"userName":"","name":"unknown-owner-rule","tunnelId":9201,"status":1}]}`
			var backup model.BackupData
			if err := json.Unmarshal([]byte(legacy), &backup); err != nil {
				t.Fatal(err)
			}
			if _, err := r.Import(&backup, []string{"users", "forwards"}); err != nil {
				t.Fatal(err)
			}
			check := func(stage string) {
				t.Helper()
				rows, err := r.ListForwards()
				if err != nil {
					t.Fatal(err)
				}
				if len(rows) != 3 {
					t.Fatalf("%s: rows=%+v", stage, rows)
				}
				for i, want := range []string{"current-owner", "missing-owner", ""} {
					if rows[i]["userName"] != want {
						t.Fatalf("%s: row %d owner=%v want %q", stage, i, rows[i]["userName"], want)
					}
				}
				if rows[0]["id"] != int64(9301) || rows[0]["userId"] != int64(9103) || rows[0]["inFlow"] != int64(123) || rows[0]["outFlow"] != int64(456) {
					t.Fatalf("%s: rule changed: %+v", stage, rows[0])
				}
				previews, err := r.ListForwardsByTunnel(9201)
				if err != nil {
					t.Fatal(err)
				}
				if len(previews) != 3 || previews[0].UserName != "current-owner" || previews[1].UserName != "missing-owner" || previews[2].UserName != "" {
					t.Fatalf("%s: preview owners=%+v", stage, previews)
				}
				exported, err := r.ExportAll()
				if err != nil {
					t.Fatal(err)
				}
				if len(exported.Forwards) != 3 || exported.Forwards[0].UserName != "former-owner" {
					t.Fatalf("%s: stored legacy snapshot was rewritten: %+v", stage, exported.Forwards)
				}
			}
			check("import")
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
			r = open()
			check("restart")
			// A forwards-only restore must resolve against the existing user by ID.
			if _, err := r.Import(&backup, []string{"forwards"}); err != nil {
				t.Fatal(err)
			}
			check("partial restore")
		})
	}
}
