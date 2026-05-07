package repo

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	gsqlite "github.com/glebarez/sqlite"
	"go-backend/internal/store/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPrepareSQLiteLegacyColumnsAddsNodeMetadataColumns(t *testing.T) {
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

	if err := db.Exec(`
		CREATE TABLE node (
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
	`).Error; err != nil {
		t.Fatalf("create legacy node table: %v", err)
	}

	if err := prepareSQLiteLegacyColumns(db); err != nil {
		t.Fatalf("prepareSQLiteLegacyColumns: %v", err)
	}

	m := db.Migrator()
	for _, field := range []string{"Remark", "ExpiryTime", "RenewalCycle"} {
		if !m.HasColumn(&model.Node{}, field) {
			t.Fatalf("expected node.%s column to exist", field)
		}
	}
}

func TestOpenBackfillsSQLiteLegacyTunnelProbeTargetColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	db, err := gorm.Open(gsqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open legacy sqlite: %v", err)
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
			in_ip TEXT
		)
	`).Error; err != nil {
		t.Fatalf("create legacy tunnel table: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip)
		VALUES(1, 'legacy-tunnel', 1, 1, 'tls', 1, 1, 1, 1, '')
	`).Error; err != nil {
		t.Fatalf("insert legacy tunnel: %v", err)
	}
	if sqlDB, _ := db.DB(); sqlDB != nil {
		_ = sqlDB.Close()
	}

	r, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open migrated sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	m := r.DB().Migrator()
	for _, field := range []string{"ProbeTargetHost", "ProbeTargetPort"} {
		if !m.HasColumn(&model.Tunnel{}, field) {
			t.Fatalf("expected tunnel.%s column to exist", field)
		}
	}

	var host string
	var port int
	if err := r.DB().Raw(`SELECT probe_target_host, probe_target_port FROM tunnel WHERE id = 1`).Row().Scan(&host, &port); err != nil {
		t.Fatalf("query probe target defaults: %v", err)
	}
	if host != "" || port != 0 {
		t.Fatalf("expected default probe target empty/0, got %q/%d", host, port)
	}
}

func TestOpenBackfillsSQLiteLegacyForwardColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-forward.db")
	db, err := gorm.Open(gsqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open legacy sqlite: %v", err)
	}

	if err := db.Exec(`
		CREATE TABLE forward (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			user_name VARCHAR(100) NOT NULL,
			name VARCHAR(100) NOT NULL,
			tunnel_id INTEGER NOT NULL,
			remote_addr TEXT NOT NULL,
			strategy VARCHAR(100) NOT NULL DEFAULT 'fifo',
			in_flow INTEGER NOT NULL DEFAULT 0,
			out_flow INTEGER NOT NULL DEFAULT 0,
			created_time INTEGER NOT NULL,
			updated_time INTEGER NOT NULL,
			status INTEGER NOT NULL,
			inx INTEGER NOT NULL DEFAULT 0,
			speed_id INTEGER
		)
	`).Error; err != nil {
		t.Fatalf("create legacy forward table: %v", err)
	}
	if err := db.Exec(`
		INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx, speed_id)
		VALUES(1, 2, 'legacy-user', 'legacy-forward', 3, '127.0.0.1:9000', 'fifo', 0, 0, 1, 1, 1, 0, NULL)
	`).Error; err != nil {
		t.Fatalf("insert legacy forward: %v", err)
	}
	if sqlDB, _ := db.DB(); sqlDB != nil {
		_ = sqlDB.Close()
	}

	r, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open migrated sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	m := r.DB().Migrator()
	for _, field := range []string{"MaxConn", "IPMaxConn", "IPSpeedID", "ProxyProtocol"} {
		if !m.HasColumn(&model.Forward{}, field) {
			t.Fatalf("expected forward.%s column to exist", field)
		}
	}

	var maxConn, ipMaxConn, proxyProtocol int
	var ipSpeedID sql.NullInt64
	if err := r.DB().Raw(`SELECT max_conn, ip_max_conn, ip_speed_id, proxy_protocol FROM forward WHERE id = 1`).Row().Scan(&maxConn, &ipMaxConn, &ipSpeedID, &proxyProtocol); err != nil {
		t.Fatalf("query forward defaults: %v", err)
	}
	if maxConn != 0 || ipMaxConn != 0 || ipSpeedID.Valid || proxyProtocol != 0 {
		t.Fatalf("expected default forward columns 0/0/NULL/0, got max_conn=%d ip_max_conn=%d ip_speed_id=%+v proxy_protocol=%d", maxConn, ipMaxConn, ipSpeedID, proxyProtocol)
	}
}

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

func TestMigrateSchemaRunsTrafficInt64MigrationForLegacySchema(t *testing.T) {
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

	originalIDRepair := ensurePostgresIDDefaultsFn
	ensurePostgresIDDefaultsFn = func(db *gorm.DB) error {
		return nil
	}
	t.Cleanup(func() {
		ensurePostgresIDDefaultsFn = originalIDRepair
	})

	called := 0
	originalMigrate := migratePostgresTrafficInt64ColumnsFn
	migratePostgresTrafficInt64ColumnsFn = func(db *gorm.DB) error {
		called++
		return nil
	}
	t.Cleanup(func() {
		migratePostgresTrafficInt64ColumnsFn = originalMigrate
	})

	if err := migrateSchema(db); err != nil {
		t.Fatalf("migrateSchema: %v", err)
	}

	if called != 1 {
		t.Fatalf("expected traffic bigint migration to run once, got %d", called)
	}

	var schemaVersion int
	if err := db.Raw(`SELECT version FROM schema_version LIMIT 1`).Row().Scan(&schemaVersion); err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if schemaVersion != currentSchemaVersion {
		t.Fatalf("expected schema version %d, got %d", currentSchemaVersion, schemaVersion)
	}
}

func TestMigrateSchemaReturnsTrafficInt64MigrationError(t *testing.T) {
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

	originalIDRepair := ensurePostgresIDDefaultsFn
	ensurePostgresIDDefaultsFn = func(db *gorm.DB) error {
		return nil
	}
	t.Cleanup(func() {
		ensurePostgresIDDefaultsFn = originalIDRepair
	})

	wantErr := errors.New("traffic bigint migration failed")
	originalMigrate := migratePostgresTrafficInt64ColumnsFn
	migratePostgresTrafficInt64ColumnsFn = func(db *gorm.DB) error {
		return wantErr
	}
	t.Cleanup(func() {
		migratePostgresTrafficInt64ColumnsFn = originalMigrate
	})

	err = migrateSchema(db)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
}

func TestAlterPostgresColumnToBigIntIfNeededValidatesNames(t *testing.T) {
	if err := alterPostgresColumnToBigIntIfNeeded(nil, "peer_share", "max_bandwidth"); err == nil || !strings.Contains(err.Error(), "nil db") {
		t.Fatalf("expected nil db error, got %v", err)
	}
	if err := alterPostgresColumnToBigIntIfNeeded(&gorm.DB{}, "", "max_bandwidth"); err == nil || !strings.Contains(err.Error(), "empty table or column name") {
		t.Fatalf("expected empty name error, got %v", err)
	}
	if err := alterPostgresColumnToBigIntIfNeeded(&gorm.DB{}, "peer_share", ""); err == nil || !strings.Contains(err.Error(), "empty table or column name") {
		t.Fatalf("expected empty name error, got %v", err)
	}
}
