package contract_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
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

func TestPostgresBackupImportAllowsImmediateCreate(t *testing.T) {
	source := ownerNamesRepository(t, "postgres")()
	for _, name := range []string{"backup-one", "backup-two"} {
		if _, err := source.CreateUser(name, security.MD5("test-password"), 1, 0, 10, 1, 2, 1, 3, 0); err != nil {
			t.Fatal(err)
		}
		if _, err := source.CreateSpeedLimit(name, 100, 0, 1); err != nil {
			t.Fatal(err)
		}
	}
	backup, err := source.ExportAll()
	if err != nil {
		t.Fatal(err)
	}
	target := ownerNamesRepository(t, "postgres")()
	if _, err := target.Import(backup, []string{"users", "speedLimits"}); err != nil {
		t.Fatal(err)
	}
	if id, err := target.CreateUser("after-import", security.MD5("test-password"), 1, 0, 10, 1, 2, 1, 3, 0); err != nil || id <= 2 {
		t.Errorf("create user immediately after backup import: id=%d err=%v", id, err)
	}
	if id, err := target.CreateSpeedLimit("after-import", 100, 0, 1); err != nil || id <= 2 {
		t.Errorf("create speed limit immediately after backup import: id=%d err=%v", id, err)
	}
}

func TestPostgresBackupImportPreservesAllocatedIDs(t *testing.T) {
	for _, called := range []bool{false, true} {
		t.Run(fmt.Sprintf("called=%t", called), func(t *testing.T) {
			r := ownerNamesRepository(t, "postgres")()
			// A deleted or rolled-back allocation may be higher than every row.
			for _, table := range []string{"user", "speed_limit"} {
				if err := r.DB().Exec(`SELECT setval(pg_get_serial_sequence(?, 'id'), 500, ?)`, `"`+table+`"`, called).Error; err != nil {
					t.Fatal(err)
				}
			}
			b := &model.BackupData{Users: []model.UserBackup{{ID: 1, User: "restored", Pwd: "$2-test-fixture", RoleID: 1}}, SpeedLimits: []model.SpeedLimitBackup{{ID: 1, Name: "restored"}}}
			if _, err := r.Import(b, []string{"users", "speedLimits"}); err != nil {
				t.Fatal(err)
			}
			want := int64(500)
			if called {
				want++
			}
			if id, err := r.CreateUser("new", "hash", 1, 0, 0, 0, 0, 1, 0, 0); err != nil || id != want {
				t.Fatalf("user id=%d, want %d; err=%v", id, want, err)
			}
			if id, err := r.CreateSpeedLimit("new", 100, 0, 1); err != nil || id != want {
				t.Fatalf("speed limit id=%d, want %d; err=%v", id, want, err)
			}
		})
	}
}

func TestPostgresBackupImportInvalidatesCachedIDs(t *testing.T) {
	r := ownerNamesRepository(t, "postgres")()
	if err := r.DB().Exec(`ALTER SEQUENCE speed_limit_id_seq CACHE 20`).Error; err != nil {
		t.Fatal(err)
	}
	pool, err := r.DB().DB()
	if err != nil {
		t.Fatal(err)
	}
	cached, err := pool.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer cached.Close()
	// Hold this backend out of the pool with unused cached IDs 2..20.
	var allocated int64
	if err := cached.QueryRowContext(context.Background(), `SELECT nextval('speed_limit_id_seq')`).Scan(&allocated); err != nil {
		t.Fatal(err)
	}
	if allocated != 1 {
		t.Fatal(allocated)
	}
	b := &model.BackupData{SpeedLimits: []model.SpeedLimitBackup{{ID: 2, Name: "restored"}}}
	if _, err := r.Import(b, []string{"speedLimits"}); err != nil {
		t.Fatal(err)
	}
	// Return only the cached backend to the pool, so the public create must use it.
	pool.SetMaxIdleConns(0)
	pool.SetMaxIdleConns(1)
	if err := cached.Close(); err != nil {
		t.Fatal(err)
	}
	if id, err := r.CreateSpeedLimit("new", 100, 0, 1); err != nil || id <= 20 {
		t.Fatalf("cached ID reused: id=%d err=%v", id, err)
	}
}

func TestPostgresBackupImportRollsBackSequenceOnCommitFailure(t *testing.T) {
	r := ownerNamesRepository(t, "postgres")()
	// Deferred validation fails after import has already advanced the sequence.
	for _, query := range []string{
		`CREATE FUNCTION reject_import_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'reject imported row at commit'; END $$`,
		`CREATE CONSTRAINT TRIGGER reject_import_commit AFTER INSERT ON speed_limit DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (NEW.name = 'fail-at-commit') EXECUTE FUNCTION reject_import_commit()`,
	} {
		if err := r.DB().Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}
	b := &model.BackupData{SpeedLimits: []model.SpeedLimitBackup{{ID: 100, Name: "fail-at-commit"}}}
	if _, err := r.Import(b, []string{"speedLimits"}); err == nil || !strings.Contains(err.Error(), "reject imported row at commit") {
		t.Fatalf("expected commit failure, got %v", err)
	}
	exported, err := r.ExportAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.SpeedLimits) != 0 {
		t.Fatal("failed import persisted rows")
	}
	if id, err := r.CreateSpeedLimit("after-failure", 100, 0, 1); err != nil || id != 1 {
		t.Fatalf("failed import changed next allocation: id=%d err=%v", id, err)
	}
}

func TestPostgresBackupImportPreservesSequenceIncrement(t *testing.T) {
	r := ownerNamesRepository(t, "postgres")()
	if err := r.DB().Exec(`ALTER SEQUENCE speed_limit_id_seq INCREMENT BY 10`).Error; err != nil {
		t.Fatal(err)
	}
	b := &model.BackupData{SpeedLimits: []model.SpeedLimitBackup{{ID: 25, Name: "restored"}}}
	if _, err := r.Import(b, []string{"speedLimits"}); err != nil {
		t.Fatal(err)
	}
	if id, err := r.CreateSpeedLimit("new", 100, 0, 1); err != nil || id != 31 {
		t.Fatalf("changed ID progression: id=%d, want 31; err=%v", id, err)
	}
}

func TestPostgresBackupImportAdvancesEveryExplicitIDTable(t *testing.T) {
	source := ownerNamesRepository(t, "postgres")()
	types := []string{"users", "speedLimits", "nodes", "tunnels", "forwards", "userTunnels", "tunnelGroups", "userGroups", "permissions", "configs"}
	fixture := &model.BackupData{
		Users:        []model.UserBackup{{ID: 1, User: "old", Pwd: "$2-test-fixture", RoleID: 1}},
		SpeedLimits:  []model.SpeedLimitBackup{{ID: 1, Name: "old"}},
		Nodes:        []model.NodeBackup{{ID: 1, Name: "old", Secret: "old-node"}},
		Tunnels:      []model.TunnelBackup{{ID: 1, Name: "old", ChainTunnels: []model.ChainTunnelBackup{{ID: 1, TunnelID: 1, NodeID: 1, ChainType: "1"}}}},
		Forwards:     []model.ForwardBackup{{ID: 1, Name: "old", UserID: 1, TunnelID: 1}},
		UserTunnels:  []model.UserTunnelBackup{{ID: 1, UserID: 1, TunnelID: 1}},
		TunnelGroups: []model.TunnelGroupBackup{{ID: 1, Name: "old"}},
		UserGroups:   []model.UserGroupBackup{{ID: 1, Name: "old"}},
		Permissions:  []model.PermissionBackup{{ID: 1, UserGroupID: 1, TunnelGroupID: 1, Grants: []model.PermissionGrantBackup{{ID: 1, UserGroupID: 1, TunnelGroupID: 1, UserTunnelID: 1}}}},
	}
	if _, err := source.Import(fixture, types); err != nil {
		t.Fatal(err)
	}
	backup, err := source.ExportAll()
	if err != nil {
		t.Fatal(err)
	}
	r := ownerNamesRepository(t, "postgres")()
	if _, err := r.Import(backup, types); err != nil {
		t.Fatal(err)
	}
	// Use the normal public creation methods, including the nested child tables.
	for _, create := range []struct {
		name string
		run  func() error
	}{
		{"user", func() error { _, err := r.CreateUser("new", "hash", 1, 0, 0, 0, 0, 1, 0, 0); return err }},
		{"speed limit", func() error { _, err := r.CreateSpeedLimit("new", 100, 0, 1); return err }},
		{"node", func() error {
			return r.CreateRemoteNode("new", "new-node", "127.0.0.1", "10000-20000", 0, 1, 0, "", "", "")
		}},
		{"tunnel", func() error { _, err := r.CreateTunnelTx(r.DB(), "new", 1, 1, 0, 0, 1, nil, 0, "", "", 0); return err }},
		{"chain tunnel", func() error { return r.CreateChainTunnelTx(r.DB(), 1, "1", 2, sql.NullInt64{}, "", 1, "tls", "") }},
		{"forward", func() error {
			_, err := r.CreateForwardTx(1, "old", "new", 1, "127.0.0.1:80", "", 0, 0, nil, 0, "", nil, 0, 0, nil, 0, 0, 0)
			return err
		}},
		{"user tunnel", func() error { return r.InsertUserTunnel(2, 1, nil, 0, 0, 0, 0, 1) }},
		{"tunnel group", func() error { return r.GroupCreate("tunnel_group", "new", 1, 0) }},
		{"user group", func() error { return r.GroupCreate("user_group", "new", 1, 0) }},
		{"permission", func() error { return r.InsertGroupPermission(2, 2, 0) }},
		{"permission grant", func() error { r.InsertGroupPermissionGrant(1, 1, 2, 1, 0); return nil }},
	} {
		t.Run(create.name, func(t *testing.T) {
			if err := create.run(); err != nil {
				t.Fatal(err)
			}
		})
	}
	got, err := r.ExportAll()
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string][]int64{}
	for _, v := range got.Users {
		ids["user"] = append(ids["user"], v.ID)
	}
	for _, v := range got.SpeedLimits {
		ids["speed limit"] = append(ids["speed limit"], v.ID)
	}
	for _, v := range got.Nodes {
		ids["node"] = append(ids["node"], v.ID)
	}
	for _, v := range got.Tunnels {
		ids["tunnel"] = append(ids["tunnel"], v.ID)
		for _, child := range v.ChainTunnels {
			ids["chain tunnel"] = append(ids["chain tunnel"], child.ID)
		}
	}
	for _, v := range got.Forwards {
		ids["forward"] = append(ids["forward"], v.ID)
	}
	for _, v := range got.UserTunnels {
		ids["user tunnel"] = append(ids["user tunnel"], v.ID)
	}
	for _, v := range got.TunnelGroups {
		ids["tunnel group"] = append(ids["tunnel group"], v.ID)
	}
	for _, v := range got.UserGroups {
		ids["user group"] = append(ids["user group"], v.ID)
	}
	for _, v := range got.Permissions {
		ids["permission"] = append(ids["permission"], v.ID)
		for _, child := range v.Grants {
			ids["permission grant"] = append(ids["permission grant"], child.ID)
		}
	}
	if len(ids) != 11 {
		t.Fatalf("missing tables in export: %v", ids)
	}
	for name, values := range ids {
		slices.Sort(values)
		if !slices.Equal(values, []int64{1, 2}) {
			t.Errorf("%s: expected old and new IDs [1 2], got %v", name, values)
		}
	}
}

func TestPostgresBackupImportKeepsExistingRowsAndEmptySelections(t *testing.T) {
	r := ownerNamesRepository(t, "postgres")()
	// Fixture simulates an existing explicit-ID row with an unsynchronized sequence.
	if err := r.DB().Create(&model.SpeedLimit{ID: 500, Name: "existing"}).Error; err != nil {
		t.Fatal(err)
	}
	old := &model.BackupData{SpeedLimits: []model.SpeedLimitBackup{{ID: 1, Name: "legacy"}}}
	if _, err := r.Import(old, []string{"speedLimits"}); err != nil {
		t.Fatal(err)
	}
	if id, err := r.CreateSpeedLimit("new", 100, 0, 1); err != nil || id != 501 {
		t.Fatalf("existing maximum ignored: id=%d err=%v", id, err)
	}
	for _, b := range []*model.BackupData{{}, {Users: []model.UserBackup{{ID: 900, User: "not-selected", Pwd: "$2-test-fixture"}}}} {
		if _, err := r.Import(b, []string{"speedLimits"}); err != nil {
			t.Fatal(err)
		}
	}
	if id, err := r.CreateUser("first-user", "hash", 1, 0, 0, 0, 0, 1, 0, 0); err != nil || id != 2 {
		t.Fatalf("unselected sequence changed: id=%d err=%v", id, err)
	}
	if id, err := r.CreateSpeedLimit("after-empty", 100, 0, 1); err != nil || id != 502 {
		t.Fatalf("empty import changed allocation: id=%d err=%v", id, err)
	}
	got, err := r.ExportAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Users) != 2 || len(got.SpeedLimits) != 4 || got.SpeedLimits[1].Name != "existing" {
		t.Fatalf("existing or unselected data changed: %+v", got)
	}
}

func TestPostgresBackupImportResolvesLegacySequenceAndSearchPath(t *testing.T) {
	r := ownerNamesRepository(t, "postgres")()
	pool, err := r.DB().DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	var schema string
	if err := r.DB().Raw(`SELECT current_schema()`).Scan(&schema).Error; err != nil {
		t.Fatal(err)
	}
	prefix := "Empty-Prefix-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	if err := r.DB().Exec(`CREATE SCHEMA "` + prefix + `"`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.DB().Exec(`DROP SCHEMA "` + prefix + `" CASCADE`).Error })
	// The historical default uses a custom, unowned sequence; the original
	// owned serial sequence still exists but is no longer used by INSERT.
	for _, query := range []string{
		`CREATE SEQUENCE "Custom Limit Sequence"`,
		`ALTER TABLE speed_limit ALTER COLUMN id SET DEFAULT nextval('"Custom Limit Sequence"'::regclass)`,
		`SET search_path TO "` + prefix + `", "` + schema + `"`,
	} {
		if err := r.DB().Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}
	b := &model.BackupData{SpeedLimits: []model.SpeedLimitBackup{{ID: 1, Name: "legacy"}}}
	if _, err := r.Import(b, []string{"speedLimits"}); err != nil {
		t.Fatal(err)
	}
	if id, err := r.CreateSpeedLimit("new", 100, 0, 1); err != nil || id != 2 {
		t.Fatalf("wrong sequence or schema: id=%d err=%v", id, err)
	}
}

func waitForBackupDBCondition(t *testing.T, r *repo.Repository, query string, args ...interface{}) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var ready bool
		if err := r.DB().Raw(query, args...).Scan(&ready).Error; err != nil {
			t.Fatal(err)
		}
		if ready {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("database barrier was not reached")
}

func backupImportBarrier(t *testing.T, r *repo.Repository, table, timing, event, condition string) (*sql.Tx, int64) {
	t.Helper()
	pool, err := r.DB().DB()
	if err != nil {
		t.Fatal(err)
	}
	barrier, err := pool.Begin()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = barrier.Rollback() })
	key := time.Now().UnixNano() & 0x3fffffff
	if _, err := barrier.Exec(`SELECT pg_advisory_xact_lock($1)`, key); err != nil {
		t.Fatal(err)
	}
	guardEnd := ""
	if condition != "" {
		guardEnd = "END IF;"
	}
	function := fmt.Sprintf(`CREATE FUNCTION pause_backup_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN %s PERFORM pg_advisory_xact_lock(%d); %s RETURN NEW; END $$`, condition, key, guardEnd)
	if err := r.DB().Exec(function).Error; err != nil {
		t.Fatal(err)
	}
	trigger := fmt.Sprintf(`CREATE TRIGGER pause_backup_test %s %s ON %s FOR EACH ROW EXECUTE FUNCTION pause_backup_test()`, timing, event, table)
	if err := r.DB().Exec(trigger).Error; err != nil {
		t.Fatal(err)
	}
	return barrier, key
}

func TestPostgresBackupImportSerializesConcurrentCreatesAndImports(t *testing.T) {
	r := ownerNamesRepository(t, "postgres")()
	barrier, key := backupImportBarrier(t, r, "speed_limit", "BEFORE", "INSERT", "IF NEW.name = 'hold-import' THEN")
	first := make(chan error, 1)
	go func() {
		_, err := r.Import(&model.BackupData{SpeedLimits: []model.SpeedLimitBackup{{ID: 100, Name: "hold-import"}}}, []string{"speedLimits"})
		first <- err
	}()
	waitForBackupDBCondition(t, r, `SELECT EXISTS (SELECT 1 FROM pg_locks WHERE locktype='advisory' AND objid=? AND NOT granted)`, key)
	created := make(chan error, 1)
	go func() {
		id, err := r.CreateSpeedLimit("concurrent-create", 100, 0, 1)
		if err == nil && id <= 100 {
			err = fmt.Errorf("reused imported ID: %d", id)
		}
		created <- err
	}()
	waitForBackupDBCondition(t, r, `SELECT EXISTS (SELECT 1 FROM pg_locks WHERE relation='speed_limit'::regclass AND NOT granted)`)
	second := make(chan error, 1)
	go func() {
		result, err := r.Import(&model.BackupData{SpeedLimits: []model.SpeedLimitBackup{{ID: 200, Name: "second-import"}}}, []string{"speedLimits"})
		if err == nil && result.SpeedLimitsImported != 1 {
			err = fmt.Errorf("wrong retry count: %+v", result)
		}
		second <- err
	}()
	if err := barrier.Commit(); err != nil {
		t.Fatal(err)
	}
	for name, done := range map[string]<-chan error{"first import": first, "create": created, "second import": second} {
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("%s: %v", name, err)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("%s blocked", name)
		}
	}
	if id, err := r.CreateSpeedLimit("after-both", 100, 0, 1); err != nil || id <= 200 {
		t.Fatalf("second import sequence not advanced: id=%d err=%v", id, err)
	}
	got, err := r.ExportAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.SpeedLimits) != 4 {
		t.Fatalf("lost concurrent rows: %+v", got.SpeedLimits)
	}
}

func TestPostgresBackupImportAvoidsCascadeDeleteDeadlock(t *testing.T) {
	for _, mode := range []string{"retry-success", "busy-bounded"} {
		t.Run(mode, func(t *testing.T) {
			r := ownerNamesRepository(t, "postgres")()
			user, err := r.CreateUser("delete-me", "hash", 1, 0, 0, 0, 0, 1, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			forward, err := r.CreateForwardTx(user, "delete-me", "old", 1, "127.0.0.1:80", "", 0, 0, []int64{1}, 12345, "", nil, 0, 0, nil, 0, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			// Pause a real public cascade after deleting a child row, before it
			// reaches forward/user. Import must not retain partial parent locks.
			barrier, key := backupImportBarrier(t, r, "forward_port", "AFTER", "DELETE", "")
			deleted := make(chan error, 1)
			go func() { deleted <- r.DeleteUserCascade(user) }()
			waitForBackupDBCondition(t, r, `SELECT EXISTS (SELECT 1 FROM pg_locks WHERE locktype='advisory' AND objid=? AND NOT granted)`, key)
			emptyPorts := []model.ForwardPortBackup{}
			backup := &model.BackupData{
				Users:    []model.UserBackup{{ID: 100, User: "restored", Pwd: "$2-test-fixture", RoleID: 1}},
				Forwards: []model.ForwardBackup{{ID: forward, UserID: 100, Name: "restored", ForwardPorts: &emptyPorts}},
			}
			imported := make(chan error, 1)
			runImport := func() {
				result, err := r.Import(backup, []string{"users", "forwards"})
				if err == nil && (result.UsersImported != 1 || result.ForwardsImported != 1) {
					err = fmt.Errorf("wrong retry count: %+v", result)
				}
				imported <- err
			}
			go runImport()
			if mode == "busy-bounded" {
				select {
				case err := <-imported:
					if err == nil || !strings.Contains(err.Error(), "database is busy") {
						t.Fatalf("expected bounded busy error, got %v", err)
					}
				case <-time.After(3 * time.Second):
					_ = barrier.Rollback()
					t.Fatal("import waited while holding partial table locks")
				}
			} else {
				// The held child lock makes every attempt fail before writes.
				// Let several bounded attempts encounter it before release.
				time.Sleep(75 * time.Millisecond)
			}
			if err := barrier.Commit(); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-deleted:
				if err != nil {
					t.Fatalf("cascade: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("cascade deadlocked")
			}
			if mode == "busy-bounded" {
				go runImport()
			}
			select {
			case err := <-imported:
				if err != nil {
					t.Fatalf("import: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("import deadlocked")
			}
			if id, err := r.CreateUser("after", "hash", 1, 0, 0, 0, 0, 1, 0, 0); err != nil || id <= 100 {
				t.Fatalf("create after concurrent cascade: id=%d err=%v", id, err)
			}
		})
	}
}

func TestPostgresBackupImportRollsBackEarlierSequenceWhenLaterSequenceFails(t *testing.T) {
	r := ownerNamesRepository(t, "postgres")()
	if err := r.DB().Exec(`ALTER SEQUENCE speed_limit_id_seq MAXVALUE 10`).Error; err != nil {
		t.Fatal(err)
	}
	backup := &model.BackupData{
		Users:       []model.UserBackup{{ID: 100, User: "rollback-user", Pwd: "$2-test-fixture", RoleID: 1}},
		SpeedLimits: []model.SpeedLimitBackup{{ID: 10, Name: "exhausted"}},
	}
	// User sequence advances first. The exhausted limit sequence then fails.
	if _, err := r.Import(backup, []string{"users", "speedLimits"}); err == nil || !strings.Contains(err.Error(), "sequence exhausted") {
		t.Fatalf("expected sequence exhaustion: %v", err)
	}
	if id, err := r.CreateUser("after-failure", "hash", 1, 0, 0, 0, 0, 1, 0, 0); err != nil || id != 2 {
		t.Fatalf("earlier sequence failed to roll back: id=%d err=%v", id, err)
	}
	if id, err := r.CreateSpeedLimit("after-failure", 100, 0, 1); err != nil || id != 1 {
		t.Fatalf("later sequence changed: id=%d err=%v", id, err)
	}
	got, err := r.ExportAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Users) != 2 || len(got.SpeedLimits) != 1 {
		t.Fatalf("partial import committed: %+v", got)
	}
}

func TestPostgresBackupImportAPIAllowsImmediateCreate(t *testing.T) {
	r := ownerNamesRepository(t, "postgres")()
	const secret = "backup-sequence-contract"
	router := httpserver.NewRouter(handler.New(r, secret), secret)
	token, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatal(err)
	}
	post := func(path, body string) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Authorization", token)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		assertCode(t, response, 0)
	}
	post("/api/v1/backup/import", `{"version":"1.0","types":["users","speedLimits"],"users":[{"id":2,"user":"restored","pwd":"$2-test-fixture","roleId":1,"status":1}],"speedLimits":[{"id":1,"name":"restored","speed":100,"status":1}]}`)
	post("/api/v1/user/create", `{"user":"after-import","pwd":"test-only-password","status":1}`)
	post("/api/v1/speed-limit/create", `{"name":"after-import","speed":200}`)
	got, err := r.ExportAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Users) != 3 || got.Users[2].ID != 3 || len(got.SpeedLimits) != 2 || got.SpeedLimits[1].ID != 2 {
		t.Fatalf("API create did not retain restored records: %+v", got)
	}
}
