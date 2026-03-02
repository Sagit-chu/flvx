package repo

import (
	"database/sql"
	"testing"

	gsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"go-backend/internal/store/model"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(gsqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestListTunnelsReturnsTunConfig(t *testing.T) {
	db := openTestDB(t)
	if err := db.AutoMigrate(&model.Node{}, &model.Tunnel{}, &model.ChainTunnel{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	tunnel := model.Tunnel{
		Name:         "tun-1",
		TrafficRatio: 1,
		Type:         3,
		TunConfig:    sql.NullString{String: `{"mode":"auto"}`, Valid: true},
		Protocol:     "tls",
		Flow:         1,
		CreatedTime:  1,
		UpdatedTime:  1,
		Status:       1,
		Inx:          1,
	}
	if err := db.Create(&tunnel).Error; err != nil {
		t.Fatalf("create tunnel: %v", err)
	}

	r := &Repository{db: db}
	items, err := r.ListTunnels()
	if err != nil {
		t.Fatalf("list tunnels: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(items))
	}
	if got := items[0]["tunConfig"]; got != `{"mode":"auto"}` {
		t.Fatalf("expected tunConfig to be preserved, got %#v", got)
	}
}

func TestExportAndImportTunnelsWithTunConfig(t *testing.T) {
	db := openTestDB(t)
	if err := db.AutoMigrate(&model.Tunnel{}, &model.ChainTunnel{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	r := &Repository{db: db}
	if err := db.Create(&model.Tunnel{
		ID:           10,
		Name:         "tun-export",
		TrafficRatio: 1,
		Type:         3,
		TunConfig:    sql.NullString{String: `{"dev":"tun0"}`, Valid: true},
		Protocol:     "tls",
		Flow:         1,
		CreatedTime:  1,
		UpdatedTime:  1,
		Status:       1,
		Inx:          1,
	}).Error; err != nil {
		t.Fatalf("seed tunnel: %v", err)
	}

	exported, err := r.exportTunnels()
	if err != nil {
		t.Fatalf("export tunnels: %v", err)
	}
	if len(exported) != 1 {
		t.Fatalf("expected 1 exported tunnel, got %d", len(exported))
	}
	if exported[0].TunConfig != `{"dev":"tun0"}` {
		t.Fatalf("expected exported tunConfig, got %q", exported[0].TunConfig)
	}

	if err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.Tunnel{}).Error; err != nil {
		t.Fatalf("clear tunnels: %v", err)
	}

	importRows := []model.TunnelBackup{
		{
			ID:           21,
			Name:         "normal",
			TrafficRatio: 1,
			Type:         1,
			TunConfig:    `{"should":"clear"}`,
			Protocol:     "tls",
			Flow:         1,
			CreatedTime:  2,
			UpdatedTime:  2,
			Status:       1,
			Inx:          1,
		},
		{
			ID:           22,
			Name:         "tun",
			TrafficRatio: 1,
			Type:         3,
			TunConfig:    `{"dev":"tun1"}`,
			Protocol:     "tls",
			Flow:         1,
			CreatedTime:  2,
			UpdatedTime:  2,
			Status:       1,
			Inx:          2,
		},
	}

	count, err := importTunnels(db, importRows, 3)
	if err != nil {
		t.Fatalf("import tunnels: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 imported tunnels, got %d", count)
	}

	var normal model.Tunnel
	if err := db.Where("id = ?", 21).First(&normal).Error; err != nil {
		t.Fatalf("query normal tunnel: %v", err)
	}
	if normal.TunConfig.Valid {
		t.Fatalf("expected type=1 tunnel tun_config to be NULL, got %q", normal.TunConfig.String)
	}

	var tun model.Tunnel
	if err := db.Where("id = ?", 22).First(&tun).Error; err != nil {
		t.Fatalf("query type=3 tunnel: %v", err)
	}
	if !tun.TunConfig.Valid || tun.TunConfig.String != `{"dev":"tun1"}` {
		t.Fatalf("expected type=3 tun_config persisted, got %#v", tun.TunConfig)
	}
}
