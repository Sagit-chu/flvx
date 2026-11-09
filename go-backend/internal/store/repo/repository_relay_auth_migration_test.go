package repo

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	gsqlite "github.com/glebarez/sqlite"
	"go-backend/internal/relaysecurity"
	"go-backend/internal/store/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestOpenBackfillsSQLiteTunnelRelayAuthMode(t *testing.T) {
	for _, version := range []int{0, currentSchemaVersion} {
		t.Run(fmt.Sprintf("schema_version_%d", version), func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "legacy-relay.db")
			db, err := gorm.Open(gsqlite.Open(dbPath), &gorm.Config{
				Logger: logger.Default.LogMode(logger.Silent),
			})
			if err != nil {
				t.Fatalf("open legacy sqlite: %v", err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatalf("get legacy sqlite connection: %v", err)
			}
			t.Cleanup(func() { _ = sqlDB.Close() })

			// This is the tunnel schema immediately before relay authentication.
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
					ip_preference VARCHAR(10) NOT NULL DEFAULT '',
					probe_target_host TEXT NOT NULL DEFAULT '',
					probe_target_port INTEGER NOT NULL DEFAULT 0
				)
			`).Error; err != nil {
				t.Fatalf("create legacy tunnel table: %v", err)
			}
			legacy := model.Tunnel{
				ID: 41, Name: "existing-relay", TrafficRatio: 1.5,
				Type: 2, Protocol: "tcp", Flow: 123,
				CreatedTime: 111, UpdatedTime: 222, Status: 1,
				InIP: sql.NullString{String: "192.0.2.1", Valid: true},
				Inx:  3, IPPreference: "ipv4",
				ProbeTargetHost: "example.com", ProbeTargetPort: 8443,
			}
			if err := db.Omit("RelayAuthMode").Create(&legacy).Error; err != nil {
				t.Fatalf("seed legacy tunnel: %v", err)
			}
			if err := db.AutoMigrate(&model.SchemaVersion{}); err != nil {
				t.Fatalf("create schema version table: %v", err)
			}
			if err := db.Create(&model.SchemaVersion{Version: version}).Error; err != nil {
				t.Fatalf("seed schema version: %v", err)
			}
			if err := sqlDB.Close(); err != nil {
				t.Fatalf("close legacy sqlite: %v", err)
			}

			r, err := Open(dbPath)
			if err != nil {
				t.Fatalf("open migrated sqlite: %v", err)
			}
			t.Cleanup(func() { _ = r.Close() })
			if !r.DB().Migrator().HasColumn(&model.Tunnel{}, "RelayAuthMode") {
				t.Fatal("expected tunnel.relay_auth_mode after upgrading an existing database")
			}
			legacy.RelayAuthMode = relaysecurity.Legacy
			assertSQLiteRelayTunnel(t, r, legacy)

			var createdID int64
			if err := r.DB().Transaction(func(tx *gorm.DB) error {
				var err error
				createdID, err = r.CreateTunnelTx(tx, "after-upgrade", 1, 1, 1, 333, 1, nil, 0, "", "", 0)
				return err
			}); err != nil {
				t.Fatalf("create tunnel after upgrade: %v", err)
			}
			if createdID <= legacy.ID {
				t.Fatalf("expected new tunnel ID above %d, got %d", legacy.ID, createdID)
			}
			var created model.Tunnel
			if err := r.DB().First(&created, createdID).Error; err != nil {
				t.Fatalf("read newly created tunnel: %v", err)
			}
			if created.RelayAuthMode != relaysecurity.Legacy {
				t.Fatalf("expected legacy default for created tunnel, got %q", created.RelayAuthMode)
			}
			if err := r.DB().Transaction(func(tx *gorm.DB) error {
				return r.SetTunnelRelayAuthModeTx(tx, createdID, relaysecurity.Required)
			}); err != nil {
				t.Fatalf("enable relay authentication: %v", err)
			}
			created.RelayAuthMode = relaysecurity.Required

			// Startup repair must be idempotent and never downgrade required mode.
			for attempt := 0; attempt < 2; attempt++ {
				if err := r.Close(); err != nil {
					t.Fatalf("close migrated sqlite: %v", err)
				}
				r, err = Open(dbPath)
				if err != nil {
					t.Fatalf("reopen migrated sqlite: %v", err)
				}
				assertSQLiteRelayTunnel(t, r, legacy)
				assertSQLiteRelayTunnel(t, r, created)
			}
			var count int64
			if err := r.DB().Model(&model.Tunnel{}).Count(&count).Error; err != nil {
				t.Fatalf("count migrated tunnels: %v", err)
			}
			if count != 2 {
				t.Fatalf("expected two tunnels after repeated startup, got %d", count)
			}
			var integrity string
			if err := r.DB().Raw("PRAGMA integrity_check").Scan(&integrity).Error; err != nil {
				t.Fatalf("check migrated database integrity: %v", err)
			}
			if integrity != "ok" {
				t.Fatalf("unexpected database integrity result: %s", integrity)
			}
		})
	}
}

func TestOpenNewSQLiteTunnelRelayAuthMode(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "new-relay.db")
	r, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open new sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	if !r.DB().Migrator().HasColumn(&model.Tunnel{}, "RelayAuthMode") {
		t.Fatal("expected tunnel.relay_auth_mode on a fresh database")
	}
	// Omit the field explicitly to test SQLite's default, not GORM's default.
	tunnel := model.Tunnel{
		Name: "new-tunnel", TrafficRatio: 1, Type: 1, Protocol: "tls",
		Flow: 1, CreatedTime: 111, UpdatedTime: 111, Status: 1,
	}
	if err := r.DB().Omit("RelayAuthMode").Create(&tunnel).Error; err != nil {
		t.Fatalf("create tunnel without explicit relay mode: %v", err)
	}
	tunnel.RelayAuthMode = relaysecurity.Legacy
	assertSQLiteRelayTunnel(t, r, tunnel)
	if err := r.Close(); err != nil {
		t.Fatalf("close new sqlite: %v", err)
	}
	r, err = Open(dbPath)
	if err != nil {
		t.Fatalf("reopen new sqlite: %v", err)
	}
	assertSQLiteRelayTunnel(t, r, tunnel)
}

func assertSQLiteRelayTunnel(t *testing.T, r *Repository, want model.Tunnel) {
	t.Helper()
	var got model.Tunnel
	if err := r.DB().First(&got, want.ID).Error; err != nil {
		t.Fatalf("read tunnel %d: %v", want.ID, err)
	}
	if got != want {
		t.Fatalf("tunnel data changed: got %+v, want %+v", got, want)
	}
}
