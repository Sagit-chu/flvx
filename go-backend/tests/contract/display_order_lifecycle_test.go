package contract_test

import (
	"database/sql"
	"errors"
	"fmt"
	"go-backend/internal/security"
	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func TestDisplayOrderBackupAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "order.db")
	r, err := repo.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := r.CreateSpeedLimit("a", 100, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.CreateSpeedLimit("b", 200, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.UpdateSpeedLimitDisplayOrder([]int64{a, b}, []int64{b, a}); err != nil {
		t.Fatal(err)
	}
	check := func(r *repo.Repository) {
		t.Helper()
		rows, err := r.ListSpeedLimits()
		if err != nil {
			t.Fatal(err)
		}
		ids := []int64{}
		for _, row := range rows {
			ids = append(ids, row["id"].(int64))
		}
		if !reflect.DeepEqual(ids, []int64{a, b}) {
			t.Fatalf("order: %v", ids)
		}
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	r, err = repo.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	check(r)
	backup, err := r.ExportPartial([]string{"speedLimits"})
	if err != nil {
		t.Fatal(err)
	}
	dest, err := repo.Open(filepath.Join(t.TempDir(), "restore.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer dest.Close()
	if _, err := dest.Import(backup, []string{"speedLimits"}); err != nil {
		t.Fatal(err)
	}
	check(dest)
}

// Lifecycle and migration fixtures use DDL only to reproduce a pre-upgrade
// database and inject a write failure. All observations use public interfaces.
func TestDisplayOrderLifecycleDialects(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			open := orderTestDatabase(t, dialect)
			r := open()
			for _, resource := range []string{"user", "speed_limit"} {
				t.Run(resource, func(t *testing.T) {
					create := func(name string) int64 {
						t.Helper()
						var id int64
						var err error
						if resource == "user" {
							id, err = r.CreateUser(name, security.MD5("test-password"), 1, 0, 123, 1, 4, 1, 5, 0)
						} else {
							id, err = r.CreateSpeedLimit(name, 123, 0, 1)
						}
						if err != nil {
							t.Fatal(err)
						}
						return id
					}
					list := func(db *repo.Repository) []int64 {
						t.Helper()
						var rows []map[string]interface{}
						var err error
						if resource == "user" {
							rows, err = db.ListUsers()
						} else {
							rows, err = db.ListSpeedLimits()
						}
						if err != nil {
							t.Fatal(err)
						}
						ids := []int64{}
						for _, row := range rows {
							ids = append(ids, row["id"].(int64))
						}
						return ids
					}
					save := func(db *repo.Repository, ids, expected []int64) error {
						if resource == "user" {
							return db.UpdateUserDisplayOrder(ids, expected)
						}
						return db.UpdateSpeedLimitDisplayOrder(ids, expected)
					}
					a, b, c := create("a"), create("b"), create("c")
					// Drop the new column to reproduce a database last opened by the old release.
					if err := r.DB().Exec("ALTER TABLE \"" + resource + "\" DROP COLUMN display_order").Error; err != nil {
						t.Fatal(err)
					}
					if err := r.Close(); err != nil {
						t.Fatal(err)
					}
					r = open()
					if got := list(r); !reflect.DeepEqual(got, []int64{c, b, a}) {
						t.Fatalf("legacy order %v", got)
					}
					if err := save(r, []int64{a, c, b}, []int64{c, b, a}); err != nil {
						t.Fatal(err)
					}
					backup, err := r.ExportPartial([]string{"users", "speedLimits"})
					if err != nil {
						t.Fatal(err)
					}
					restore := orderTestDatabase(t, dialect)()
					defer restore.Close()
					types := []string{"speedLimits"}
					if resource == "user" {
						types = []string{"users"}
					}
					if _, err := restore.Import(backup, types); err != nil {
						t.Fatal(err)
					}
					if got := list(restore); !reflect.DeepEqual(got, []int64{a, c, b}) {
						t.Fatalf("new backup order: %v", got)
					}
					before, err := r.ExportPartial([]string{"users", "speedLimits"})
					if err != nil {
						t.Fatal(err)
					}
					if err := r.Close(); err != nil {
						t.Fatal(err)
					}
					r = open()
					second := open()
					defer second.Close()
					if got := list(second); !reflect.DeepEqual(got, []int64{a, c, b}) {
						t.Fatalf("cross-client/restart order %v", got)
					}
					// The next save fails on its second row; the first row must roll back.
					if dialect == "sqlite" {
						if err := r.DB().Exec(fmt.Sprintf("CREATE TRIGGER reject_order BEFORE UPDATE OF display_order ON \"%s\" WHEN NEW.id = %d BEGIN SELECT RAISE(ABORT, 'injected failure'); END", resource, a)).Error; err != nil {
							t.Fatal(err)
						}
					} else {
						if err := r.DB().Exec(fmt.Sprintf("ALTER TABLE \"%s\" ADD CONSTRAINT reject_order CHECK (id != %d OR display_order = 0)", resource, a)).Error; err != nil {
							t.Fatal(err)
						}
					}
					if err := save(r, []int64{b, a, c}, []int64{a, c, b}); err == nil {
						t.Fatal("expected injected failure")
					}
					if got := list(second); !reflect.DeepEqual(got, []int64{a, c, b}) {
						t.Fatalf("partial write: %v", got)
					}
					if dialect == "sqlite" {
						if err := r.DB().Exec("DROP TRIGGER reject_order").Error; err != nil {
							t.Fatal(err)
						}
					} else {
						if err := r.DB().Exec("ALTER TABLE \"" + resource + "\" DROP CONSTRAINT reject_order").Error; err != nil {
							t.Fatal(err)
						}
					}
					// Exactly one cross-client save against the same observed order may win.
					start := make(chan struct{})
					results := make(chan error, 2)
					for _, db := range []*repo.Repository{r, second} {
						go func(db *repo.Repository) { <-start; results <- save(db, []int64{b, a, c}, []int64{a, c, b}) }(db)
					}
					close(start)
					e1, e2 := <-results, <-results
					if !((e1 == nil && errors.Is(e2, repo.ErrDisplayOrderConflict)) || (e2 == nil && errors.Is(e1, repo.ErrDisplayOrderConflict))) {
						t.Fatalf("concurrent saves: %v / %v", e1, e2)
					}
					// Reordering must not mutate any exported fields besides rank.
					after, err := r.ExportPartial([]string{"users", "speedLimits"})
					if err != nil {
						t.Fatal(err)
					}
					strip := func(b *model.BackupData) {
						b.ExportedAt = 0
						for i := range b.Users {
							b.Users[i].DisplayOrder = nil
						}
						for i := range b.SpeedLimits {
							b.SpeedLimits[i].DisplayOrder = nil
						}
					}
					strip(before)
					strip(after)
					if !reflect.DeepEqual(before, after) {
						t.Fatal("reorder modified non-order backup fields")
					}
					// Old backups omit ranks and must not overwrite an existing saved order.
					legacy := orderTestDatabase(t, dialect)()
					defer legacy.Close()
					if _, err := legacy.Import(after, types); err != nil {
						t.Fatal(err)
					}
					if got := list(legacy); !reflect.DeepEqual(got, []int64{c, b, a}) {
						t.Fatalf("new legacy records: %v", got)
					}
					if _, err := r.Import(after, types); err != nil {
						t.Fatal(err)
					}
					if got := list(r); !reflect.DeepEqual(got, []int64{b, a, c}) {
						t.Fatalf("old backup reset existing order: %v", got)
					}
					d := create("d")
					if got := list(r); !reflect.DeepEqual(got, []int64{d, b, a, c}) {
						t.Fatalf("create after reorder: %v", got)
					}
					if err := save(second, []int64{a, b, c}, []int64{b, a, c}); !errors.Is(err, repo.ErrDisplayOrderConflict) {
						t.Fatalf("stale membership: %v", err)
					}
				})
			}
			r.Close()
		})
	}
}

func orderTestDatabase(t *testing.T, dialect string) func() *repo.Repository {
	t.Helper()
	if dialect == "sqlite" {
		path := filepath.Join(t.TempDir(), "lifecycle.db")
		return func() *repo.Repository {
			r, err := repo.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { r.Close() })
			return r
		}
	}
	dsn := os.Getenv("FLVX_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("set FLVX_POSTGRES_TEST_DSN for isolated PostgreSQL contracts")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "contract_display_order_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	if _, err := admin.Exec("CREATE SCHEMA \"" + schema + "\""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec("DROP SCHEMA \"" + schema + "\" CASCADE"); admin.Close() })
	scoped, err := withSearchPath(dsn, schema)
	if err != nil {
		t.Fatal(err)
	}
	return func() *repo.Repository {
		r, err := repo.OpenPostgres(scoped)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { r.Close() })
		return r
	}
}

func TestDisplayOrderConcurrentCreate(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			for _, resource := range []string{"user", "speed-limit"} {
				t.Run(resource, func(t *testing.T) {
					open := orderTestDatabase(t, dialect)
					r, other := open(), open()
					create := func(db *repo.Repository, name string) (int64, error) {
						if resource == "user" {
							return db.CreateUser(name, "hash", 1, 0, 10, 1, 2, 1, 3, 0)
						}
						return db.CreateSpeedLimit(name, 100, 0, 1)
					}
					save := func(ids, expected []int64) error {
						if resource == "user" {
							return r.UpdateUserDisplayOrder(ids, expected)
						}
						return r.UpdateSpeedLimitDisplayOrder(ids, expected)
					}
					a, err := create(r, "a")
					if err != nil {
						t.Fatal(err)
					}
					b, err := create(r, "b")
					if err != nil {
						t.Fatal(err)
					}
					start := make(chan struct{})
					saved := make(chan error, 1)
					type creation struct {
						id  int64
						err error
					}
					created := make(chan creation, 1)
					go func() { <-start; saved <- save([]int64{a, b}, []int64{b, a}) }()
					go func() { <-start; id, err := create(other, "c"); created <- creation{id, err} }()
					close(start)
					saveErr, c := <-saved, <-created
					if c.err != nil {
						t.Fatal(c.err)
					}
					if saveErr != nil && !errors.Is(saveErr, repo.ErrDisplayOrderConflict) {
						t.Fatalf("concurrent save: %v", saveErr)
					}
					var rows []map[string]interface{}
					if resource == "user" {
						rows, err = r.ListUsers()
					} else {
						rows, err = r.ListSpeedLimits()
					}
					if err != nil {
						t.Fatal(err)
					}
					got := []int64{}
					for _, row := range rows {
						got = append(got, row["id"].(int64))
					}
					want := []int64{c.id, a, b}
					if saveErr != nil {
						want = []int64{c.id, b, a}
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("concurrent create order %v want %v", got, want)
					}
					if resource == "user" {
						err = r.DeleteUserCascade(c.id)
					} else {
						err = r.DeleteSpeedLimit(c.id)
					}
					if err != nil {
						t.Fatal(err)
					}
					if err := save(got, got); !errors.Is(err, repo.ErrDisplayOrderConflict) {
						t.Fatalf("deleted membership not rejected: %v", err)
					}
				})
			}
		})
	}
}
