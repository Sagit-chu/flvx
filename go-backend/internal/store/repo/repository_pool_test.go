package repo

import (
	"testing"

	gsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestConfigurePostgresPoolSetsMaxOpenConnections(t *testing.T) {
	db, err := gorm.Open(gsqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	configurePostgresPool(sqlDB)

	if got := sqlDB.Stats().MaxOpenConnections; got != defaultPostgresMaxOpenConns {
		t.Fatalf("expected max open conns %d, got %d", defaultPostgresMaxOpenConns, got)
	}
}
