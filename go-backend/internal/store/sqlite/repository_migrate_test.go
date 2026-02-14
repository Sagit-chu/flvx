package sqlite

import (
	"database/sql"
	"errors"
	"testing"

	"go-backend/internal/store"

	_ "modernc.org/sqlite"
)

func TestMigrateSchemaRunsPostgresIDRepairEvenAtCurrentVersion(t *testing.T) {
	raw, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = raw.Close()
	})

	db := store.Wrap(raw, store.DialectPostgres)
	if _, err := db.Exec(`CREATE TABLE schema_version (version INTEGER NOT NULL DEFAULT 0)`); err != nil {
		t.Fatalf("create schema_version: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_version(version) VALUES(?)`, currentSchemaVersion); err != nil {
		t.Fatalf("seed schema_version: %v", err)
	}

	called := 0
	original := ensurePostgresIDDefaultsFn
	ensurePostgresIDDefaultsFn = func(db *store.DB) error {
		called++
		return nil
	}
	t.Cleanup(func() {
		ensurePostgresIDDefaultsFn = original
	})

	if err := migrateSchema(db); err != nil {
		t.Fatalf("migrateSchema: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected postgres id repair to run once, got %d", called)
	}
}

func TestMigrateSchemaReturnsPostgresIDRepairError(t *testing.T) {
	raw, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = raw.Close()
	})

	db := store.Wrap(raw, store.DialectPostgres)
	if _, err := db.Exec(`CREATE TABLE schema_version (version INTEGER NOT NULL DEFAULT 0)`); err != nil {
		t.Fatalf("create schema_version: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_version(version) VALUES(?)`, currentSchemaVersion); err != nil {
		t.Fatalf("seed schema_version: %v", err)
	}

	wantErr := errors.New("repair failed")
	original := ensurePostgresIDDefaultsFn
	ensurePostgresIDDefaultsFn = func(db *store.DB) error {
		return wantErr
	}
	t.Cleanup(func() {
		ensurePostgresIDDefaultsFn = original
	})

	err = migrateSchema(db)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
}
