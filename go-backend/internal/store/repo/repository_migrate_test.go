package repo

import (
	"database/sql"
	"errors"
	"testing"

	gsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"go-backend/internal/store/model"
)

func TestMigrateSchemaRunsPostgresIDRepairEvenAtCurrentVersion(t *testing.T) {
	db, err := gorm.Open(gsqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	if err := db.Exec(`CREATE TABLE schema_version (version INTEGER NOT NULL DEFAULT 0)`).Error; err != nil {
		t.Fatalf("create schema_version: %v", err)
	}
	if err := db.Exec(`INSERT INTO schema_version(version) VALUES(?)`, currentSchemaVersion).Error; err != nil {
		t.Fatalf("seed schema_version: %v", err)
	}

	called := 0
	original := ensurePostgresIDDefaultsFn
	ensurePostgresIDDefaultsFn = func(db *gorm.DB) error {
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
	db, err := gorm.Open(gsqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	if err := db.Exec(`CREATE TABLE schema_version (version INTEGER NOT NULL DEFAULT 0)`).Error; err != nil {
		t.Fatalf("create schema_version: %v", err)
	}
	if err := db.Exec(`INSERT INTO schema_version(version) VALUES(?)`, currentSchemaVersion).Error; err != nil {
		t.Fatalf("seed schema_version: %v", err)
	}

	wantErr := errors.New("repair failed")
	original := ensurePostgresIDDefaultsFn
	ensurePostgresIDDefaultsFn = func(db *gorm.DB) error {
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

func TestMigrateSchemaRunsViteConfigValueMigrationForLegacySchema(t *testing.T) {
	db, err := gorm.Open(gsqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	if err := db.Exec(`CREATE TABLE schema_version (version INTEGER NOT NULL DEFAULT 0)`).Error; err != nil {
		t.Fatalf("create schema_version: %v", err)
	}
	if err := db.Exec(`INSERT INTO schema_version(version) VALUES(?)`, 2).Error; err != nil {
		t.Fatalf("seed schema_version: %v", err)
	}

	originalIDRepair := ensurePostgresIDDefaultsFn
	ensurePostgresIDDefaultsFn = func(db *gorm.DB) error {
		return nil
	}
	t.Cleanup(func() {
		ensurePostgresIDDefaultsFn = originalIDRepair
	})

	called := 0
	originalMigrate := migrateViteConfigValueColumnTypeFn
	migrateViteConfigValueColumnTypeFn = func(db *gorm.DB) error {
		called++
		return nil
	}
	t.Cleanup(func() {
		migrateViteConfigValueColumnTypeFn = originalMigrate
	})

	if err := migrateSchema(db); err != nil {
		t.Fatalf("migrateSchema: %v", err)
	}

	if called != 1 {
		t.Fatalf("expected vite_config migration to run once, got %d", called)
	}
}

func TestMigrateSchemaReturnsViteConfigMigrationError(t *testing.T) {
	db, err := gorm.Open(gsqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	if err := db.Exec(`CREATE TABLE schema_version (version INTEGER NOT NULL DEFAULT 0)`).Error; err != nil {
		t.Fatalf("create schema_version: %v", err)
	}
	if err := db.Exec(`INSERT INTO schema_version(version) VALUES(?)`, 2).Error; err != nil {
		t.Fatalf("seed schema_version: %v", err)
	}

	originalIDRepair := ensurePostgresIDDefaultsFn
	ensurePostgresIDDefaultsFn = func(db *gorm.DB) error {
		return nil
	}
	t.Cleanup(func() {
		ensurePostgresIDDefaultsFn = originalIDRepair
	})

	wantErr := errors.New("vite config migration failed")
	originalMigrate := migrateViteConfigValueColumnTypeFn
	migrateViteConfigValueColumnTypeFn = func(db *gorm.DB) error {
		return wantErr
	}
	t.Cleanup(func() {
		migrateViteConfigValueColumnTypeFn = originalMigrate
	})

	err = migrateSchema(db)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
}

func TestMigrateSchemaClearsSpeedLimitTunnelBinding(t *testing.T) {
	db, err := gorm.Open(gsqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	if err := db.Exec(`CREATE TABLE schema_version (version INTEGER NOT NULL DEFAULT 0)`).Error; err != nil {
		t.Fatalf("create schema_version: %v", err)
	}
	if err := db.Exec(`INSERT INTO schema_version(version) VALUES(?)`, 3).Error; err != nil {
		t.Fatalf("seed schema_version: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE speed_limit (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(100) NOT NULL,
			speed INTEGER NOT NULL,
			tunnel_id INTEGER,
			tunnel_name VARCHAR(100),
			created_time INTEGER NOT NULL,
			updated_time INTEGER,
			status INTEGER NOT NULL
		)
	`).Error; err != nil {
		t.Fatalf("create speed_limit: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO speed_limit(name, speed, tunnel_id, tunnel_name, created_time, updated_time, status)
		VALUES(?, ?, ?, ?, ?, ?, ?)
	`, "legacy-speed-limit", 100, 101, "legacy-tunnel", 1, 1, 1).Error; err != nil {
		t.Fatalf("seed speed_limit: %v", err)
	}

	originalIDRepair := ensurePostgresIDDefaultsFn
	ensurePostgresIDDefaultsFn = func(db *gorm.DB) error {
		return nil
	}
	t.Cleanup(func() {
		ensurePostgresIDDefaultsFn = originalIDRepair
	})

	if err := migrateSchema(db); err != nil {
		t.Fatalf("migrateSchema: %v", err)
	}

	var tunnelID sql.NullInt64
	var tunnelName sql.NullString
	if err := db.Raw(`SELECT tunnel_id, tunnel_name FROM speed_limit WHERE name = ?`, "legacy-speed-limit").Row().Scan(&tunnelID, &tunnelName); err != nil {
		t.Fatalf("query speed_limit: %v", err)
	}
	if tunnelID.Valid {
		t.Fatalf("expected tunnel_id cleared to NULL, got %d", tunnelID.Int64)
	}
	if tunnelName.Valid {
		t.Fatalf("expected tunnel_name cleared to NULL, got %q", tunnelName.String)
	}

	var schemaVersion int
	if err := db.Raw(`SELECT version FROM schema_version LIMIT 1`).Row().Scan(&schemaVersion); err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if schemaVersion != currentSchemaVersion {
		t.Fatalf("expected schema version %d, got %d", currentSchemaVersion, schemaVersion)
	}
}

func TestMigrateSchemaAddsTunnelTunConfigColumn(t *testing.T) {
	db, err := gorm.Open(gsqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	if err := db.Exec(`CREATE TABLE schema_version (version INTEGER NOT NULL DEFAULT 0)`).Error; err != nil {
		t.Fatalf("create schema_version: %v", err)
	}
	if err := db.Exec(`INSERT INTO schema_version(version) VALUES(?)`, 4).Error; err != nil {
		t.Fatalf("seed schema_version: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE tunnel (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(100) NOT NULL,
			traffic_ratio REAL NOT NULL DEFAULT 1.0,
			type INTEGER NOT NULL,
			protocol VARCHAR(10) NOT NULL DEFAULT 'tls',
			flow INTEGER NOT NULL,
			created_time INTEGER NOT NULL,
			updated_time INTEGER NOT NULL,
			status INTEGER NOT NULL,
			in_ip TEXT,
			inx INTEGER NOT NULL DEFAULT 0,
			ip_preference VARCHAR(10) NOT NULL DEFAULT ''
		)
	`).Error; err != nil {
		t.Fatalf("create tunnel: %v", err)
	}

	originalIDRepair := ensurePostgresIDDefaultsFn
	ensurePostgresIDDefaultsFn = func(db *gorm.DB) error { return nil }
	t.Cleanup(func() {
		ensurePostgresIDDefaultsFn = originalIDRepair
	})

	if err := migrateSchema(db); err != nil {
		t.Fatalf("migrateSchema: %v", err)
	}

	if !db.Migrator().HasColumn(&model.Tunnel{}, "TunConfig") {
		t.Fatalf("expected tunnel.tun_config column to be added")
	}

	var schemaVersion int
	if err := db.Raw(`SELECT version FROM schema_version LIMIT 1`).Row().Scan(&schemaVersion); err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if schemaVersion != currentSchemaVersion {
		t.Fatalf("expected schema version %d, got %d", currentSchemaVersion, schemaVersion)
	}
}
