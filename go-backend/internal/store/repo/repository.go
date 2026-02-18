package repo

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gsqlite "github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	"go-backend/internal/store/model"
)

// ─── Type aliases for backward compatibility ─────────────────────────
// Handlers still reference repo.User, repo.BackupData, etc.

type User = model.User
type ViteConfig = model.ViteConfig
type Announcement = model.Announcement
type UserTunnelDetail = model.UserTunnelDetail
type UserForwardDetail = model.UserForwardDetail
type StatisticsFlow = model.StatisticsFlow
type Node = model.Node
type PeerShare = model.PeerShare
type PeerShareRuntime = model.PeerShareRuntime
type FederationTunnelBinding = model.FederationTunnelBinding
type BackupData = model.BackupData
type UserBackup = model.UserBackup
type NodeBackup = model.NodeBackup
type TunnelBackup = model.TunnelBackup
type ChainTunnelBackup = model.ChainTunnelBackup
type ForwardBackup = model.ForwardBackup
type ForwardPortBackup = model.ForwardPortBackup
type UserTunnelBackup = model.UserTunnelBackup
type SpeedLimitBackup = model.SpeedLimitBackup
type TunnelGroupBackup = model.TunnelGroupBackup
type UserGroupBackup = model.UserGroupBackup
type PermissionBackup = model.PermissionBackup
type PermissionGrantBackup = model.PermissionGrantBackup
type ImportResult = model.ImportResult

// ─── Repository ──────────────────────────────────────────────────────

type Repository struct {
	db *gorm.DB
}

func (r *Repository) DB() *gorm.DB {
	if r == nil {
		return nil
	}
	return r.db
}

// ─── Open / Close ────────────────────────────────────────────────────

func Open(path string) (*Repository, error) {
	if err := ensureParentDir(path); err != nil {
		return nil, err
	}

	dsn := "file:" + path +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)"

	db, err := gorm.Open(gsqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)

	if err := prepareSQLiteLegacyColumns(db); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("prepare sqlite legacy schema: %w", err)
	}

	if err := autoMigrateAll(db); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("auto migrate: %w", err)
	}

	seedData(db)

	if err := migrateSchema(db); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	return &Repository{db: db}, nil
}

func OpenPostgres(dsn string) (*Repository, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("empty postgres dsn")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	if err := preparePostgresLegacySchema(db); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("prepare postgres legacy schema: %w", err)
	}

	if err := autoMigrateAll(db); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("auto migrate: %w", err)
	}

	seedData(db)

	if err := migrateSchema(db); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	return &Repository{db: db}, nil
}

func (r *Repository) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func autoMigrateAll(db *gorm.DB) error {
	models := []interface{}{
		&model.User{},
		&model.Forward{},
		&model.ForwardPort{},
		&model.Node{},
		&model.SpeedLimit{},
		&model.StatisticsFlow{},
		&model.Tunnel{},
		&model.ChainTunnel{},
		&model.UserTunnel{},
		&model.TunnelGroup{},
		&model.UserGroup{},
		&model.TunnelGroupTunnel{},
		&model.UserGroupUser{},
		&model.GroupPermission{},
		&model.GroupPermissionGrant{},
		&model.ViteConfig{},
		&model.PeerShare{},
		&model.PeerShareRuntime{},
		&model.FederationTunnelBinding{},
		&model.Announcement{},
		&model.SchemaVersion{},
	}

	if db.Dialector.Name() != "sqlite" {
		return db.AutoMigrate(models...)
	}

	m := db.Migrator()
	hasNode := m.HasTable(&model.Node{})
	hasTunnel := m.HasTable(&model.Tunnel{})

	for _, item := range models {
		if hasNode {
			if _, ok := item.(*model.Node); ok {
				continue
			}
		}
		if hasTunnel {
			if _, ok := item.(*model.Tunnel); ok {
				continue
			}
		}
		if err := db.AutoMigrate(item); err != nil {
			return err
		}
	}

	return nil
}

// preparePostgresLegacySchema renames unique constraints that were created by
// the old schema.sql (which used inline UNIQUE column syntax) to the names
// expected by GORM's NamingStrategy ("uni_<table>_<column>"). Without this,
// GORM's AutoMigrate emits "DROP CONSTRAINT uni_..." against constraints that
// don't exist under that name, crashing startup on upgraded PostgreSQL installs.
func preparePostgresLegacySchema(db *gorm.DB) error {
	if db == nil || db.Dialector.Name() != "postgres" {
		return nil
	}

	type rename struct{ table, oldName, newName string }
	renames := []rename{
		{"vite_config", "vite_config_name_key", "uni_vite_config_name"},
		{"peer_share", "peer_share_token_key", "uni_peer_share_token"},
		{"peer_share_runtime", "peer_share_runtime_reservation_id_key", "uni_peer_share_runtime_reservation_id"},
		{"peer_share_runtime", "peer_share_runtime_resource_key_key", "uni_peer_share_runtime_resource_key"},
		{"federation_tunnel_binding", "federation_tunnel_binding_resource_key_key", "uni_federation_tunnel_binding_resource_key"},
	}

	for _, r := range renames {
		var count int64
		if err := db.Raw(
			`SELECT COUNT(*) FROM information_schema.table_constraints
			 WHERE constraint_schema = current_schema()
			   AND table_name = ?
			   AND constraint_name = ?
			   AND constraint_type = 'UNIQUE'`,
			r.table, r.oldName,
		).Scan(&count).Error; err != nil {
			return fmt.Errorf("check constraint %s.%s: %w", r.table, r.oldName, err)
		}
		if count == 0 {
			continue
		}
		if err := db.Exec(
			fmt.Sprintf(`ALTER TABLE %q RENAME CONSTRAINT %q TO %q`, r.table, r.oldName, r.newName),
		).Error; err != nil {
			return fmt.Errorf("rename constraint %s.%s→%s: %w", r.table, r.oldName, r.newName, err)
		}
	}
	return nil
}

func prepareSQLiteLegacyColumns(db *gorm.DB) error {
	if db == nil || db.Dialector.Name() != "sqlite" {
		return nil
	}
	m := db.Migrator()

	if m.HasTable(&model.Node{}) {
		for _, field := range []string{"ServerIPV4", "ServerIPV6", "Inx", "IsRemote", "RemoteURL", "RemoteToken", "RemoteConfig"} {
			if m.HasColumn(&model.Node{}, field) {
				continue
			}
			if err := m.AddColumn(&model.Node{}, field); err != nil {
				return fmt.Errorf("add node.%s: %w", field, err)
			}
		}
	}

	if m.HasTable(&model.Tunnel{}) {
		for _, field := range []string{"Inx", "IPPreference"} {
			if m.HasColumn(&model.Tunnel{}, field) {
				continue
			}
			if err := m.AddColumn(&model.Tunnel{}, field); err != nil {
				return fmt.Errorf("add tunnel.%s: %w", field, err)
			}
		}
	}

	return nil
}

func seedData(db *gorm.DB) {
	adminUser := model.User{
		ID: 1, User: "admin_user", Pwd: "3c85cdebade1c51cf64ca9f3c09d182d",
		RoleID: 0, ExpTime: 2727251700000, Flow: 99999, InFlow: 0, OutFlow: 0,
		FlowResetTime: 1, Num: 99999, CreatedTime: 1748914865000,
		UpdatedTime: sql.NullInt64{Int64: 1754011744252, Valid: true}, Status: 1,
	}
	db.Where("id = ?", 1).FirstOrCreate(&adminUser)

	appNameConfig := model.ViteConfig{ID: 1, Name: "app_name", Value: "flux", Time: 1755147963000}
	db.Where("id = ?", 1).FirstOrCreate(&appNameConfig)
}

// ─── User Queries ────────────────────────────────────────────────────

func (r *Repository) GetUserByUsername(username string) (*model.User, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var user model.User
	err := r.db.Where(`"user" = ?`, username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) GetUserByID(id int64) (*model.User, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var user model.User
	err := r.db.Where("id = ?", id).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *Repository) UsernameExists(username string) (bool, error) {
	var count int64
	err := r.db.Model(&model.User{}).Where(`"user" = ?`, username).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) UsernameExistsExceptID(username string, exceptID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("repository not initialized")
	}
	var count int64
	err := r.db.Model(&model.User{}).Where(`"user" = ? AND id != ?`, username, exceptID).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) UpdateUserNameAndPassword(userID int64, username, passwordMD5 string, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"user":         username,
		"pwd":          passwordMD5,
		"updated_time": now,
	}).Error
}

// ─── Config Queries ──────────────────────────────────────────────────

func (r *Repository) GetConfigByName(name string) (*model.ViteConfig, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var cfg model.ViteConfig
	err := r.db.Where("name = ?", name).First(&cfg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *Repository) ListConfigs() (map[string]string, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var configs []model.ViteConfig
	if err := r.db.Find(&configs).Error; err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, c := range configs {
		result[c.Name] = c.Value
	}
	return result, nil
}

func (r *Repository) UpsertConfig(name, value string, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "time"}),
	}).Create(&model.ViteConfig{Name: name, Value: value, Time: now}).Error
}

// ─── Announcement Queries ────────────────────────────────────────────

func (r *Repository) GetAnnouncement() (*model.Announcement, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var ann model.Announcement
	err := r.db.Order("id DESC").First(&ann).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ann, nil
}

func (r *Repository) UpsertAnnouncement(content string, enabled int, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	var count int64
	if err := r.db.Model(&model.Announcement{}).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return r.db.Create(&model.Announcement{
			Content: content, Enabled: enabled,
			CreatedTime: now, UpdatedTime: sql.NullInt64{Int64: now, Valid: true},
		}).Error
	}
	return r.db.Model(&model.Announcement{}).Where("1=1").Updates(map[string]interface{}{
		"content": content, "enabled": enabled, "updated_time": now,
	}).Error
}

// ─── User Package Queries ────────────────────────────────────────────

func (r *Repository) GetUserPackageTunnels(userID int64) ([]model.UserTunnelDetail, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var items []model.UserTunnelDetail
	err := r.db.Model(&model.UserTunnel{}).
		Select("user_tunnel.id, user_tunnel.user_id, user_tunnel.tunnel_id, tunnel.name AS tunnel_name, tunnel.flow AS tunnel_flow, user_tunnel.flow, user_tunnel.in_flow, user_tunnel.out_flow, user_tunnel.num, user_tunnel.flow_reset_time, user_tunnel.exp_time, user_tunnel.speed_id, speed_limit.name AS speed_limit, speed_limit.speed").
		Joins("LEFT JOIN tunnel ON tunnel.id = user_tunnel.tunnel_id").
		Joins("LEFT JOIN speed_limit ON speed_limit.id = user_tunnel.speed_id").
		Where("user_tunnel.user_id = ?", userID).
		Order("user_tunnel.id ASC").
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = make([]model.UserTunnelDetail, 0)
	}
	return items, nil
}

func (r *Repository) GetUserPackageForwards(userID int64) ([]model.UserForwardDetail, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	type fwdRow struct {
		ID         int64
		Name       string
		TunnelID   int64
		TunnelName string
		RemoteAddr string
		InFlow     int64
		OutFlow    int64
		Status     int
		CreatedAt  int64
	}

	var rows []fwdRow
	err := r.db.Model(&model.Forward{}).
		Select("forward.id, forward.name, forward.tunnel_id, COALESCE(tunnel.name, '') AS tunnel_name, forward.remote_addr, forward.in_flow, forward.out_flow, forward.status, forward.created_time AS created_at").
		Joins("LEFT JOIN tunnel ON tunnel.id = forward.tunnel_id").
		Where("forward.user_id = ?", userID).
		Order("forward.id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	items := make([]model.UserForwardDetail, 0, len(rows))
	for _, row := range rows {
		inIP, inPort, err := resolveForwardIngress(r.db, row.ID, row.TunnelID)
		if err != nil {
			return nil, err
		}
		items = append(items, model.UserForwardDetail{
			ID: row.ID, Name: row.Name, TunnelID: row.TunnelID,
			TunnelName: row.TunnelName, InIP: inIP, InPort: inPort,
			RemoteAddr: row.RemoteAddr, InFlow: row.InFlow, OutFlow: row.OutFlow,
			Status: row.Status, CreatedAt: row.CreatedAt,
		})
	}
	return items, nil
}

// ─── Statistics Queries ──────────────────────────────────────────────

func (r *Repository) GetStatisticsFlows(userID int64, limit int) ([]model.StatisticsFlow, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var items []model.StatisticsFlow
	err := r.db.Where("user_id = ?", userID).Order("id DESC").Limit(limit).Find(&items).Error
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = make([]model.StatisticsFlow, 0)
	}
	return items, nil
}

// ─── Node Queries ────────────────────────────────────────────────────

func (r *Repository) NodeExistsBySecret(secret string) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("repository not initialized")
	}
	var count int64
	err := r.db.Model(&model.Node{}).Where("secret = ?", secret).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) GetNodeBySecret(secret string) (*model.Node, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var n model.Node
	err := r.db.Where("secret = ?", secret).First(&n).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (r *Repository) GetNodeByID(id int64) (*model.Node, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var n model.Node
	err := r.db.Where("id = ?", id).First(&n).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (r *Repository) UpdateNodeOnline(nodeID int64, status int, version string, httpVal, tlsVal, socksVal int) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.Node{}).Where("id = ?", nodeID).Updates(map[string]interface{}{
		"status": status, "version": version, "http": httpVal, "tls": tlsVal,
		"socks": socksVal, "updated_time": unixMilliNow(),
	}).Error
}

func (r *Repository) UpdateNodeStatus(nodeID int64, status int) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.Node{}).Where("id = ?", nodeID).Updates(map[string]interface{}{
		"status": status, "updated_time": unixMilliNow(),
	}).Error
}

// ─── Flow ────────────────────────────────────────────────────────────

func (r *Repository) AddFlow(forwardID, userID int64, userTunnelID int64, inFlow, outFlow int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Forward{}).Where("id = ?", forwardID).
			UpdateColumns(map[string]interface{}{
				"in_flow":  gorm.Expr("in_flow + ?", inFlow),
				"out_flow": gorm.Expr("out_flow + ?", outFlow),
			}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.User{}).Where("id = ?", userID).
			UpdateColumns(map[string]interface{}{
				"in_flow":  gorm.Expr("in_flow + ?", inFlow),
				"out_flow": gorm.Expr("out_flow + ?", outFlow),
			}).Error; err != nil {
			return err
		}
		if userTunnelID > 0 {
			if err := tx.Model(&model.UserTunnel{}).Where("id = ?", userTunnelID).
				UpdateColumns(map[string]interface{}{
					"in_flow":  gorm.Expr("in_flow + ?", inFlow),
					"out_flow": gorm.Expr("out_flow + ?", outFlow),
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ─── List Methods (return map[string]interface{}) ────────────────────

func (r *Repository) ListNodes() ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var nodes []model.Node
	if err := r.db.Order("inx ASC, id ASC").Find(&nodes).Error; err != nil {
		return nil, err
	}
	items := make([]map[string]interface{}, 0, len(nodes))
	for _, n := range nodes {
		items = append(items, map[string]interface{}{
			"id": n.ID, "inx": n.Inx, "name": n.Name,
			"ip": n.ServerIP, "serverIp": n.ServerIP,
			"serverIpV4":    nullableString(n.ServerIPV4),
			"serverIpV6":    nullableString(n.ServerIPV6),
			"port":          n.Port,
			"tcpListenAddr": n.TCPListenAddr,
			"udpListenAddr": n.UDPListenAddr,
			"version":       nullableString(n.Version),
			"http":          n.HTTP, "tls": n.TLS, "socks": n.Socks,
			"status": n.Status, "isRemote": n.IsRemote,
			"remoteUrl":    nullableString(n.RemoteURL),
			"remoteToken":  nullableString(n.RemoteToken),
			"remoteConfig": nullableString(n.RemoteConfig),
		})
	}
	return items, nil
}

func (r *Repository) ListUsers() ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var users []model.User
	if err := r.db.Where("role_id != ?", 0).Order("id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	items := make([]map[string]interface{}, 0, len(users))
	for _, u := range users {
		items = append(items, map[string]interface{}{
			"id": u.ID, "user": u.User, "name": u.User,
			"roleId": u.RoleID, "status": u.Status,
			"flow": u.Flow, "num": u.Num, "expTime": u.ExpTime,
			"flowResetTime": u.FlowResetTime, "createdTime": u.CreatedTime,
			"updatedTime": nullableInt64(u.UpdatedTime),
			"inFlow":      u.InFlow, "outFlow": u.OutFlow,
		})
	}
	return items, nil
}

func (r *Repository) ListSpeedLimits() ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var limits []model.SpeedLimit
	if err := r.db.Order("id ASC").Find(&limits).Error; err != nil {
		return nil, err
	}
	items := make([]map[string]interface{}, 0, len(limits))
	for _, sl := range limits {
		items = append(items, map[string]interface{}{
			"id": sl.ID, "name": sl.Name, "speed": sl.Speed,
			"tunnelId": sl.TunnelID, "tunnelName": sl.TunnelName,
			"status": sl.Status, "createdTime": sl.CreatedTime,
			"updatedTime": nullableInt64(sl.UpdatedTime),
		})
	}
	return items, nil
}

func (r *Repository) ListForwards() ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	type fwdRow struct {
		ID          int64
		UserID      int64
		UserName    string
		Name        string
		TunnelID    int64
		TunnelName  string
		RemoteAddr  string
		Strategy    string
		InFlow      int64
		OutFlow     int64
		CreatedTime int64
		Status      int
		Inx         int
	}

	var rows []fwdRow
	err := r.db.Model(&model.Forward{}).
		Select("forward.id, forward.user_id, forward.user_name, forward.name, forward.tunnel_id, COALESCE(tunnel.name, '') AS tunnel_name, forward.remote_addr, COALESCE(forward.strategy, 'fifo') AS strategy, forward.in_flow, forward.out_flow, forward.created_time, forward.status, forward.inx").
		Joins("LEFT JOIN tunnel ON tunnel.id = forward.tunnel_id").
		Order("forward.inx ASC, forward.id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	items := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		inIP, inPort, err := resolveForwardIngress(r.db, row.ID, row.TunnelID)
		if err != nil {
			return nil, err
		}
		items = append(items, map[string]interface{}{
			"id": row.ID, "userId": row.UserID, "userName": row.UserName,
			"name": row.Name, "tunnelId": row.TunnelID, "tunnelName": row.TunnelName,
			"inIp": nullableForwardIngress(inIP), "inPort": nullableInt64(inPort),
			"remoteAddr": row.RemoteAddr, "strategy": row.Strategy,
			"inFlow": row.InFlow, "outFlow": row.OutFlow,
			"createdTime": row.CreatedTime, "status": row.Status, "inx": int64(row.Inx),
		})
	}
	return items, nil
}

func (r *Repository) ListUserAccessibleTunnels(userID int64) ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	type row struct {
		ID   int64
		Name string
	}
	var rows []row
	err := r.db.Model(&model.UserTunnel{}).
		Select("tunnel.id, tunnel.name").
		Joins("JOIN tunnel ON tunnel.id = user_tunnel.tunnel_id").
		Where("user_tunnel.user_id = ? AND tunnel.status = 1", userID).
		Order("tunnel.inx ASC, tunnel.id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	items := make([]map[string]interface{}, 0, len(rows))
	for _, r := range rows {
		items = append(items, map[string]interface{}{"id": r.ID, "name": r.Name})
	}
	return items, nil
}

func (r *Repository) ListEnabledTunnelSummaries() ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	type row struct {
		ID   int64
		Name string
	}
	var rows []row
	err := r.db.Model(&model.Tunnel{}).Select("id, name").Where("status = 1").Order("inx ASC, id ASC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	items := make([]map[string]interface{}, 0, len(rows))
	for _, r := range rows {
		items = append(items, map[string]interface{}{"id": r.ID, "name": r.Name})
	}
	return items, nil
}

func (r *Repository) ListTunnels() ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	var tunnels []model.Tunnel
	if err := r.db.Order("inx ASC, id ASC").Find(&tunnels).Error; err != nil {
		return nil, err
	}

	tunnelMap := make(map[int64]map[string]interface{})
	orderedIDs := make([]int64, 0, len(tunnels))

	for _, t := range tunnels {
		tunnelMap[t.ID] = map[string]interface{}{
			"id": t.ID, "inx": t.Inx, "name": t.Name,
			"type": t.Type, "flow": t.Flow, "trafficRatio": t.TrafficRatio,
			"status": t.Status, "createdTime": t.CreatedTime,
			"inIp":         nullableString(t.InIP),
			"ipPreference": t.IPPreference,
			"inNodeId":     make([]map[string]interface{}, 0),
			"outNodeId":    make([]map[string]interface{}, 0),
			"chainNodes":   make([][]map[string]interface{}, 0),
		}
		orderedIDs = append(orderedIDs, t.ID)
	}

	// Build node IP map
	nodeIPMap := map[int64]string{}
	var nodeList []model.Node
	if err := r.db.Select("id, server_ip").Find(&nodeList).Error; err == nil {
		for _, n := range nodeList {
			nodeIPMap[n.ID] = n.ServerIP
		}
	}

	// Load chain tunnels
	var chains []model.ChainTunnel
	if err := r.db.Order("tunnel_id ASC, chain_type ASC, inx ASC, id ASC").Find(&chains).Error; err != nil {
		return nil, err
	}

	chainBucket := map[int64]map[int][]map[string]interface{}{}
	inNodeIPs := map[int64][]string{}

	for _, c := range chains {
		t, ok := tunnelMap[c.TunnelID]
		if !ok {
			continue
		}

		chainTypeInt := 0
		fmt.Sscanf(c.ChainType, "%d", &chainTypeInt)

		inx := int64(0)
		if c.Inx.Valid {
			inx = c.Inx.Int64
		}

		nodeObj := map[string]interface{}{
			"nodeId":    c.NodeID,
			"chainType": chainTypeInt,
			"inx":       inx,
		}
		if c.Protocol.Valid {
			nodeObj["protocol"] = c.Protocol.String
		}
		if c.Strategy.Valid {
			nodeObj["strategy"] = c.Strategy.String
		}

		switch chainTypeInt {
		case 1:
			t["inNodeId"] = append(t["inNodeId"].([]map[string]interface{}), nodeObj)
			if ip, ok := nodeIPMap[c.NodeID]; ok && ip != "" {
				inNodeIPs[c.TunnelID] = append(inNodeIPs[c.TunnelID], ip)
			}
		case 2:
			if _, ok := chainBucket[c.TunnelID]; !ok {
				chainBucket[c.TunnelID] = map[int][]map[string]interface{}{}
			}
			chainBucket[c.TunnelID][int(inx)] = append(chainBucket[c.TunnelID][int(inx)], nodeObj)
		case 3:
			t["outNodeId"] = append(t["outNodeId"].([]map[string]interface{}), nodeObj)
		}
	}

	for tunnelID, groups := range chainBucket {
		t := tunnelMap[tunnelID]
		if t == nil {
			continue
		}
		keys := make([]int, 0, len(groups))
		for k := range groups {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		ordered := make([][]map[string]interface{}, 0, len(keys))
		for _, k := range keys {
			ordered = append(ordered, groups[k])
		}
		t["chainNodes"] = ordered

		if s, ok := t["inIp"].(string); !ok || strings.TrimSpace(s) == "" {
			if ips := inNodeIPs[tunnelID]; len(ips) > 0 {
				t["inIp"] = strings.Join(ips, ",")
			}
		}
	}

	result := make([]map[string]interface{}, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		if t, ok := tunnelMap[id]; ok {
			result = append(result, t)
		}
	}
	return result, nil
}

// ─── Group Queries ───────────────────────────────────────────────────

func (r *Repository) ListTunnelGroups() ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var groups []model.TunnelGroup
	if err := r.db.Order("id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	result := make([]map[string]interface{}, 0, len(groups))
	for _, g := range groups {
		ids, names, err := r.listTunnelGroupMembers(g.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, map[string]interface{}{
			"id": g.ID, "name": g.Name, "status": g.Status,
			"tunnelIds": ids, "tunnelNames": names,
			"createdTime": g.CreatedTime,
		})
	}
	return result, nil
}

func (r *Repository) ListUserGroups() ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var groups []model.UserGroup
	if err := r.db.Order("id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	result := make([]map[string]interface{}, 0, len(groups))
	for _, g := range groups {
		ids, names, err := r.listUserGroupMembers(g.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, map[string]interface{}{
			"id": g.ID, "name": g.Name, "status": g.Status,
			"userIds": ids, "userNames": names,
			"createdTime": g.CreatedTime,
		})
	}
	return result, nil
}

func (r *Repository) ListGroupPermissions() ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	type permRow struct {
		ID              int64
		UserGroupID     int64
		UserGroupName   sql.NullString
		TunnelGroupID   int64
		TunnelGroupName sql.NullString
		CreatedTime     int64
	}
	var rows []permRow
	err := r.db.Model(&model.GroupPermission{}).
		Select("group_permission.id, group_permission.user_group_id, user_group.name AS user_group_name, group_permission.tunnel_group_id, tunnel_group.name AS tunnel_group_name, group_permission.created_time").
		Joins("LEFT JOIN user_group ON user_group.id = group_permission.user_group_id").
		Joins("LEFT JOIN tunnel_group ON tunnel_group.id = group_permission.tunnel_group_id").
		Order("group_permission.id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]map[string]interface{}, 0, len(rows))
	for _, r := range rows {
		result = append(result, map[string]interface{}{
			"id": r.ID, "userGroupId": r.UserGroupID,
			"userGroupName":   nullableString(r.UserGroupName),
			"tunnelGroupId":   r.TunnelGroupID,
			"tunnelGroupName": nullableString(r.TunnelGroupName),
			"createdTime":     r.CreatedTime,
		})
	}
	return result, nil
}

func (r *Repository) listTunnelGroupMembers(groupID int64) ([]int64, []string, error) {
	type row struct {
		ID   int64
		Name string
	}
	var rows []row
	err := r.db.Model(&model.TunnelGroupTunnel{}).
		Select("tunnel.id, tunnel.name").
		Joins("JOIN tunnel ON tunnel.id = tunnel_group_tunnel.tunnel_id").
		Where("tunnel_group_tunnel.tunnel_group_id = ?", groupID).
		Order("tunnel.id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	ids := make([]int64, 0, len(rows))
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
		names = append(names, r.Name)
	}
	return ids, names, nil
}

func (r *Repository) listUserGroupMembers(groupID int64) ([]int64, []string, error) {
	type row struct {
		ID   int64
		Name string
	}
	var rows []row
	err := r.db.Model(&model.UserGroupUser{}).
		Select(`"user".id, "user"."user" AS name`).
		Joins(`JOIN "user" ON "user".id = user_group_user.user_id`).
		Where("user_group_user.user_group_id = ?", groupID).
		Order(`"user".id ASC`).
		Find(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	ids := make([]int64, 0, len(rows))
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
		names = append(names, r.Name)
	}
	return ids, names, nil
}

// ─── PeerShare CRUD ──────────────────────────────────────────────────

func (r *Repository) CreatePeerShare(share *model.PeerShare) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Create(share).Error
}

func (r *Repository) UpdatePeerShare(share *model.PeerShare) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.PeerShare{}).Where("id = ?", share.ID).Updates(map[string]interface{}{
		"name": share.Name, "max_bandwidth": share.MaxBandwidth,
		"expiry_time": share.ExpiryTime, "port_range_start": share.PortRangeStart,
		"port_range_end": share.PortRangeEnd, "is_active": share.IsActive,
		"updated_time": share.UpdatedTime, "allowed_domains": share.AllowedDomains,
		"allowed_ips": share.AllowedIPs,
	}).Error
}

func (r *Repository) DeletePeerShare(id int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		tx.Where("share_id = ?", id).Delete(&model.PeerShareRuntime{})
		return tx.Where("id = ?", id).Delete(&model.PeerShare{}).Error
	})
}

func (r *Repository) GetPeerShare(id int64) (*model.PeerShare, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var s model.PeerShare
	err := r.db.Where("id = ?", id).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) GetPeerShareByToken(token string) (*model.PeerShare, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var s model.PeerShare
	err := r.db.Where("token = ?", token).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) ListPeerShares() ([]model.PeerShare, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var shares []model.PeerShare
	err := r.db.Order("id DESC").Find(&shares).Error
	return shares, err
}

func (r *Repository) AddPeerShareCurrentFlow(shareID int64, delta int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if shareID <= 0 || delta <= 0 {
		return nil
	}
	return r.db.Model(&model.PeerShare{}).Where("id = ?", shareID).
		UpdateColumns(map[string]interface{}{
			"current_flow": gorm.Expr("current_flow + ?", delta),
			"updated_time": unixMilliNow(),
		}).Error
}

func (r *Repository) ResetPeerShareCurrentFlow(shareID int64, updatedTime int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if shareID <= 0 {
		return nil
	}
	if updatedTime <= 0 {
		updatedTime = unixMilliNow()
	}
	return r.db.Model(&model.PeerShare{}).Where("id = ?", shareID).Updates(map[string]interface{}{
		"current_flow": 0, "updated_time": updatedTime,
	}).Error
}

// ─── PeerShareRuntime CRUD ───────────────────────────────────────────

func (r *Repository) CreatePeerShareRuntime(item *model.PeerShareRuntime) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if item == nil {
		return errors.New("runtime item is nil")
	}
	return r.db.Create(item).Error
}

func (r *Repository) UpdatePeerShareRuntime(item *model.PeerShareRuntime) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if item == nil {
		return errors.New("runtime item is nil")
	}
	return r.db.Model(&model.PeerShareRuntime{}).Where("id = ?", item.ID).Updates(map[string]interface{}{
		"binding_id": item.BindingID, "role": item.Role,
		"chain_name": item.ChainName, "service_name": item.ServiceName,
		"protocol": item.Protocol, "strategy": item.Strategy,
		"port": item.Port, "target": item.Target,
		"applied": item.Applied, "status": item.Status,
		"updated_time": item.UpdatedTime,
	}).Error
}

func (r *Repository) MarkPeerShareRuntimeReleased(id int64, updatedTime int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.PeerShareRuntime{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status": 0, "updated_time": updatedTime,
	}).Error
}

func (r *Repository) GetPeerShareRuntimeByResourceKey(shareID int64, resourceKey string) (*model.PeerShareRuntime, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var item model.PeerShareRuntime
	err := r.db.Where("share_id = ? AND resource_key = ?", shareID, resourceKey).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) GetPeerShareRuntimeByReservationID(shareID int64, reservationID string) (*model.PeerShareRuntime, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var item model.PeerShareRuntime
	err := r.db.Where("share_id = ? AND reservation_id = ?", shareID, reservationID).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) GetPeerShareRuntimeByBindingID(shareID int64, bindingID string) (*model.PeerShareRuntime, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var item model.PeerShareRuntime
	err := r.db.Where("share_id = ? AND binding_id = ?", shareID, bindingID).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) GetPeerShareRuntimeByID(id int64) (*model.PeerShareRuntime, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var item model.PeerShareRuntime
	err := r.db.Where("id = ?", id).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) ListActivePeerShareRuntimesByShareID(shareID int64) ([]model.PeerShareRuntime, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var out []model.PeerShareRuntime
	err := r.db.Where("share_id = ? AND status = 1", shareID).Order("port ASC, id ASC").Find(&out).Error
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = make([]model.PeerShareRuntime, 0)
	}
	return out, nil
}

func (r *Repository) ListActivePeerShareRuntimePorts(shareID int64, nodeID int64) ([]int, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var ports []int
	err := r.db.Model(&model.PeerShareRuntime{}).
		Where("share_id = ? AND node_id = ? AND status = 1 AND port > 0", shareID, nodeID).
		Pluck("port", &ports).Error
	if err != nil {
		return nil, err
	}
	if ports == nil {
		ports = make([]int, 0)
	}
	return ports, nil
}

// ─── FederationTunnelBinding ─────────────────────────────────────────

func (r *Repository) UpsertFederationTunnelBinding(item *model.FederationTunnelBinding) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if item == nil {
		return errors.New("binding item is nil")
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "tunnel_id"}, {Name: "node_id"}, {Name: "chain_type"}, {Name: "hop_inx"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"remote_url", "resource_key", "remote_binding_id",
			"allocated_port", "status", "updated_time",
		}),
	}).Create(item).Error
}

func (r *Repository) ListActiveFederationTunnelBindingsByTunnel(tunnelID int64) ([]model.FederationTunnelBinding, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var out []model.FederationTunnelBinding
	err := r.db.Where("tunnel_id = ? AND status = 1", tunnelID).
		Order("chain_type ASC, hop_inx ASC, id ASC").Find(&out).Error
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = make([]model.FederationTunnelBinding, 0)
	}
	return out, nil
}

func (r *Repository) DeleteFederationTunnelBindingsByTunnel(tunnelID int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Where("tunnel_id = ?", tunnelID).Delete(&model.FederationTunnelBinding{}).Error
}

// ─── Export Methods ──────────────────────────────────────────────────

func (r *Repository) ExportAll() (*model.BackupData, error) {
	backup := &model.BackupData{Version: "1.0", ExportedAt: unixMilliNow()}

	users, err := r.exportUsers()
	if err != nil {
		return nil, fmt.Errorf("export users failed: %w", err)
	}
	backup.Users = users

	nodes, err := r.exportNodes()
	if err != nil {
		return nil, fmt.Errorf("export nodes failed: %w", err)
	}
	backup.Nodes = nodes

	tunnels, err := r.exportTunnels()
	if err != nil {
		return nil, fmt.Errorf("export tunnels failed: %w", err)
	}
	backup.Tunnels = tunnels

	forwards, err := r.exportForwards()
	if err != nil {
		return nil, fmt.Errorf("export forwards failed: %w", err)
	}
	backup.Forwards = forwards

	userTunnels, err := r.exportUserTunnels()
	if err != nil {
		return nil, fmt.Errorf("export user tunnels failed: %w", err)
	}
	backup.UserTunnels = userTunnels

	speedLimits, err := r.exportSpeedLimits()
	if err != nil {
		return nil, fmt.Errorf("export speed limits failed: %w", err)
	}
	backup.SpeedLimits = speedLimits

	tunnelGroups, err := r.exportTunnelGroups()
	if err != nil {
		return nil, fmt.Errorf("export tunnel groups failed: %w", err)
	}
	backup.TunnelGroups = tunnelGroups

	userGroups, err := r.exportUserGroups()
	if err != nil {
		return nil, fmt.Errorf("export user groups failed: %w", err)
	}
	backup.UserGroups = userGroups

	permissions, err := r.exportPermissions()
	if err != nil {
		return nil, fmt.Errorf("export permissions failed: %w", err)
	}
	backup.Permissions = permissions

	configs, err := r.ListConfigs()
	if err != nil {
		return nil, fmt.Errorf("export configs failed: %w", err)
	}
	backup.Configs = configs

	return backup, nil
}

func (r *Repository) ExportPartial(types []string) (*model.BackupData, error) {
	backup := &model.BackupData{Version: "1.0", ExportedAt: unixMilliNow()}
	typeSet := make(map[string]bool)
	for _, t := range types {
		typeSet[t] = true
	}

	if typeSet["users"] {
		v, err := r.exportUsers()
		if err != nil {
			return nil, fmt.Errorf("export users failed: %w", err)
		}
		backup.Users = v
	}
	if typeSet["nodes"] {
		v, err := r.exportNodes()
		if err != nil {
			return nil, fmt.Errorf("export nodes failed: %w", err)
		}
		backup.Nodes = v
	}
	if typeSet["tunnels"] {
		v, err := r.exportTunnels()
		if err != nil {
			return nil, fmt.Errorf("export tunnels failed: %w", err)
		}
		backup.Tunnels = v
	}
	if typeSet["forwards"] {
		v, err := r.exportForwards()
		if err != nil {
			return nil, fmt.Errorf("export forwards failed: %w", err)
		}
		backup.Forwards = v
	}
	if typeSet["userTunnels"] {
		v, err := r.exportUserTunnels()
		if err != nil {
			return nil, fmt.Errorf("export user tunnels failed: %w", err)
		}
		backup.UserTunnels = v
	}
	if typeSet["speedLimits"] {
		v, err := r.exportSpeedLimits()
		if err != nil {
			return nil, fmt.Errorf("export speed limits failed: %w", err)
		}
		backup.SpeedLimits = v
	}
	if typeSet["tunnelGroups"] {
		v, err := r.exportTunnelGroups()
		if err != nil {
			return nil, fmt.Errorf("export tunnel groups failed: %w", err)
		}
		backup.TunnelGroups = v
	}
	if typeSet["userGroups"] {
		v, err := r.exportUserGroups()
		if err != nil {
			return nil, fmt.Errorf("export user groups failed: %w", err)
		}
		backup.UserGroups = v
	}
	if typeSet["permissions"] {
		v, err := r.exportPermissions()
		if err != nil {
			return nil, fmt.Errorf("export permissions failed: %w", err)
		}
		backup.Permissions = v
	}
	if typeSet["configs"] {
		v, err := r.ListConfigs()
		if err != nil {
			return nil, fmt.Errorf("export configs failed: %w", err)
		}
		backup.Configs = v
	}
	return backup, nil
}

func (r *Repository) exportUsers() ([]model.UserBackup, error) {
	var users []model.User
	if err := r.db.Order("id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	out := make([]model.UserBackup, 0, len(users))
	for _, u := range users {
		b := model.UserBackup{
			ID: u.ID, User: u.User, Pwd: u.Pwd, RoleID: u.RoleID,
			ExpTime: u.ExpTime, Flow: u.Flow, InFlow: u.InFlow, OutFlow: u.OutFlow,
			FlowResetTime: u.FlowResetTime, Num: u.Num,
			CreatedTime: u.CreatedTime, Status: u.Status,
		}
		if u.UpdatedTime.Valid {
			b.UpdatedTime = u.UpdatedTime.Int64
		}
		out = append(out, b)
	}
	return out, nil
}

func (r *Repository) exportNodes() ([]model.NodeBackup, error) {
	var nodes []model.Node
	if err := r.db.Order("inx ASC, id ASC").Find(&nodes).Error; err != nil {
		return nil, err
	}
	out := make([]model.NodeBackup, 0, len(nodes))
	for _, n := range nodes {
		b := model.NodeBackup{
			ID: n.ID, Name: n.Name, Secret: n.Secret, ServerIP: n.ServerIP,
			Port: n.Port, HTTP: n.HTTP, TLS: n.TLS, Socks: n.Socks,
			CreatedTime: n.CreatedTime, Status: n.Status,
			TCPListenAddr: n.TCPListenAddr, UDPListenAddr: n.UDPListenAddr,
			Inx: n.Inx, IsRemote: n.IsRemote,
		}
		if n.UpdatedTime.Valid {
			b.UpdatedTime = n.UpdatedTime.Int64
		}
		if n.ServerIPV4.Valid {
			b.ServerIPv4 = n.ServerIPV4.String
		}
		if n.ServerIPV6.Valid {
			b.ServerIPv6 = n.ServerIPV6.String
		}
		if n.InterfaceName.Valid {
			b.InterfaceName = n.InterfaceName.String
		}
		if n.Version.Valid {
			b.Version = n.Version.String
		}
		if n.RemoteURL.Valid {
			b.RemoteURL = n.RemoteURL.String
		}
		if n.RemoteToken.Valid {
			b.RemoteToken = n.RemoteToken.String
		}
		if n.RemoteConfig.Valid {
			b.RemoteConfig = n.RemoteConfig.String
		}
		out = append(out, b)
	}
	return out, nil
}

func (r *Repository) exportTunnels() ([]model.TunnelBackup, error) {
	var tunnels []model.Tunnel
	if err := r.db.Order("inx ASC, id ASC").Find(&tunnels).Error; err != nil {
		return nil, err
	}
	out := make([]model.TunnelBackup, 0, len(tunnels))
	for _, t := range tunnels {
		b := model.TunnelBackup{
			ID: t.ID, Name: t.Name, TrafficRatio: t.TrafficRatio,
			Type: t.Type, Protocol: t.Protocol, Flow: t.Flow,
			CreatedTime: t.CreatedTime, UpdatedTime: t.UpdatedTime,
			Status: t.Status, Inx: t.Inx, IPPreference: t.IPPreference,
		}
		if t.InIP.Valid {
			b.InIP = t.InIP.String
		}
		chains, err := r.exportChainTunnels(t.ID)
		if err != nil {
			return nil, err
		}
		b.ChainTunnels = chains
		out = append(out, b)
	}
	return out, nil
}

func (r *Repository) exportChainTunnels(tunnelID int64) ([]model.ChainTunnelBackup, error) {
	var chains []model.ChainTunnel
	if err := r.db.Where("tunnel_id = ?", tunnelID).Order("inx ASC, id ASC").Find(&chains).Error; err != nil {
		return nil, err
	}
	out := make([]model.ChainTunnelBackup, 0, len(chains))
	for _, c := range chains {
		b := model.ChainTunnelBackup{
			ID: c.ID, TunnelID: c.TunnelID, ChainType: c.ChainType, NodeID: c.NodeID,
		}
		if c.Port.Valid {
			b.Port = int(c.Port.Int64)
		}
		if c.Strategy.Valid {
			b.Strategy = c.Strategy.String
		}
		if c.Inx.Valid {
			b.Inx = int(c.Inx.Int64)
		}
		if c.Protocol.Valid {
			b.Protocol = c.Protocol.String
		}
		out = append(out, b)
	}
	return out, nil
}

func (r *Repository) exportForwards() ([]model.ForwardBackup, error) {
	var forwards []model.Forward
	if err := r.db.Order("id ASC").Find(&forwards).Error; err != nil {
		return nil, err
	}
	out := make([]model.ForwardBackup, 0, len(forwards))
	for _, f := range forwards {
		b := model.ForwardBackup{
			ID: f.ID, UserID: f.UserID, UserName: f.UserName, Name: f.Name,
			TunnelID: f.TunnelID, RemoteAddr: f.RemoteAddr, Strategy: f.Strategy,
			InFlow: f.InFlow, OutFlow: f.OutFlow, CreatedTime: f.CreatedTime,
			UpdatedTime: f.UpdatedTime, Status: f.Status, Inx: f.Inx,
		}
		ports, err := r.exportForwardPorts(f.ID)
		if err != nil {
			return nil, err
		}
		portsCopy := append([]model.ForwardPortBackup(nil), ports...)
		b.ForwardPorts = &portsCopy
		out = append(out, b)
	}
	return out, nil
}

func (r *Repository) exportForwardPorts(forwardID int64) ([]model.ForwardPortBackup, error) {
	var fps []model.ForwardPort
	if err := r.db.Where("forward_id = ?", forwardID).Order("id ASC").Find(&fps).Error; err != nil {
		return nil, err
	}
	out := make([]model.ForwardPortBackup, 0, len(fps))
	for _, fp := range fps {
		out = append(out, model.ForwardPortBackup{NodeID: fp.NodeID, Port: fp.Port})
	}
	return out, nil
}

func (r *Repository) exportUserTunnels() ([]model.UserTunnelBackup, error) {
	var uts []model.UserTunnel
	if err := r.db.Order("id ASC").Find(&uts).Error; err != nil {
		return nil, err
	}
	out := make([]model.UserTunnelBackup, 0, len(uts))
	for _, ut := range uts {
		b := model.UserTunnelBackup{
			ID: ut.ID, UserID: ut.UserID, TunnelID: ut.TunnelID,
			Num: ut.Num, Flow: ut.Flow, InFlow: ut.InFlow, OutFlow: ut.OutFlow,
			FlowResetTime: ut.FlowResetTime, ExpTime: ut.ExpTime, Status: ut.Status,
		}
		if ut.SpeedID.Valid {
			b.SpeedID = ut.SpeedID.Int64
		}
		out = append(out, b)
	}
	return out, nil
}

func (r *Repository) exportSpeedLimits() ([]model.SpeedLimitBackup, error) {
	var sls []model.SpeedLimit
	if err := r.db.Order("id ASC").Find(&sls).Error; err != nil {
		return nil, err
	}
	out := make([]model.SpeedLimitBackup, 0, len(sls))
	for _, sl := range sls {
		b := model.SpeedLimitBackup{
			ID: sl.ID, Name: sl.Name, Speed: int64(sl.Speed),
			TunnelID: sl.TunnelID, TunnelName: sl.TunnelName,
			CreatedTime: sl.CreatedTime, Status: sl.Status,
		}
		if sl.UpdatedTime.Valid {
			b.UpdatedTime = sl.UpdatedTime.Int64
		}
		out = append(out, b)
	}
	return out, nil
}

func (r *Repository) exportTunnelGroups() ([]model.TunnelGroupBackup, error) {
	var groups []model.TunnelGroup
	if err := r.db.Order("id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	out := make([]model.TunnelGroupBackup, 0, len(groups))
	for _, tg := range groups {
		b := model.TunnelGroupBackup{
			ID: tg.ID, Name: tg.Name, CreatedTime: tg.CreatedTime,
			UpdatedTime: tg.UpdatedTime, Status: tg.Status,
		}
		var tunnelIDs []int64
		r.db.Model(&model.TunnelGroupTunnel{}).Where("tunnel_group_id = ?", tg.ID).Pluck("tunnel_id", &tunnelIDs)
		b.Tunnels = tunnelIDs
		out = append(out, b)
	}
	return out, nil
}

func (r *Repository) exportUserGroups() ([]model.UserGroupBackup, error) {
	var groups []model.UserGroup
	if err := r.db.Order("id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	out := make([]model.UserGroupBackup, 0, len(groups))
	for _, ug := range groups {
		b := model.UserGroupBackup{
			ID: ug.ID, Name: ug.Name, CreatedTime: ug.CreatedTime,
			UpdatedTime: ug.UpdatedTime, Status: ug.Status,
		}
		var userIDs []int64
		r.db.Model(&model.UserGroupUser{}).Where("user_group_id = ?", ug.ID).Pluck("user_id", &userIDs)
		b.Users = userIDs
		out = append(out, b)
	}
	return out, nil
}

func (r *Repository) exportPermissions() ([]model.PermissionBackup, error) {
	var perms []model.GroupPermission
	if err := r.db.Order("id ASC").Find(&perms).Error; err != nil {
		return nil, err
	}
	out := make([]model.PermissionBackup, 0, len(perms))
	for _, p := range perms {
		b := model.PermissionBackup{
			ID: p.ID, UserGroupID: p.UserGroupID, TunnelGroupID: p.TunnelGroupID,
			CreatedTime: p.CreatedTime,
		}
		var grants []model.GroupPermissionGrant
		r.db.Where("user_group_id = ? AND tunnel_group_id = ?", p.UserGroupID, p.TunnelGroupID).Find(&grants)
		for _, g := range grants {
			b.Grants = append(b.Grants, model.PermissionGrantBackup{
				ID: g.ID, UserGroupID: g.UserGroupID, TunnelGroupID: g.TunnelGroupID,
				UserTunnelID: g.UserTunnelID, CreatedTime: g.CreatedTime,
				CreatedByGroup: g.CreatedByGroup,
			})
		}
		out = append(out, b)
	}
	return out, nil
}

// ─── Import Methods ──────────────────────────────────────────────────

func (r *Repository) Import(backup *model.BackupData, types []string) (*model.ImportResult, error) {
	result := &model.ImportResult{}
	typeSet := make(map[string]bool)
	for _, t := range types {
		typeSet[t] = true
	}

	err := r.db.Transaction(func(tx *gorm.DB) error {
		now := unixMilliNow()

		if typeSet["users"] && len(backup.Users) > 0 {
			count, err := importUsers(tx, backup.Users, now)
			if err != nil {
				return fmt.Errorf("import users failed: %w", err)
			}
			result.UsersImported = count
		}
		if typeSet["nodes"] && len(backup.Nodes) > 0 {
			count, err := importNodes(tx, backup.Nodes, now)
			if err != nil {
				return fmt.Errorf("import nodes failed: %w", err)
			}
			result.NodesImported = count
		}
		if typeSet["tunnels"] && len(backup.Tunnels) > 0 {
			count, err := importTunnels(tx, backup.Tunnels, now)
			if err != nil {
				return fmt.Errorf("import tunnels failed: %w", err)
			}
			result.TunnelsImported = count
		}
		if typeSet["forwards"] && len(backup.Forwards) > 0 {
			count, err := importForwards(tx, backup.Forwards, now)
			if err != nil {
				return fmt.Errorf("import forwards failed: %w", err)
			}
			result.ForwardsImported = count
		}
		if typeSet["userTunnels"] && len(backup.UserTunnels) > 0 {
			count, err := importUserTunnels(tx, backup.UserTunnels, now)
			if err != nil {
				return fmt.Errorf("import user tunnels failed: %w", err)
			}
			result.UserTunnelsImported = count
		}
		if typeSet["speedLimits"] && len(backup.SpeedLimits) > 0 {
			count, err := importSpeedLimits(tx, backup.SpeedLimits, now)
			if err != nil {
				return fmt.Errorf("import speed limits failed: %w", err)
			}
			result.SpeedLimitsImported = count
		}
		if typeSet["tunnelGroups"] && len(backup.TunnelGroups) > 0 {
			count, err := importTunnelGroups(tx, backup.TunnelGroups, now)
			if err != nil {
				return fmt.Errorf("import tunnel groups failed: %w", err)
			}
			result.TunnelGroupsImported = count
		}
		if typeSet["userGroups"] && len(backup.UserGroups) > 0 {
			count, err := importUserGroups(tx, backup.UserGroups, now)
			if err != nil {
				return fmt.Errorf("import user groups failed: %w", err)
			}
			result.UserGroupsImported = count
		}
		if typeSet["permissions"] && len(backup.Permissions) > 0 {
			count, err := importPermissions(tx, backup.Permissions, now)
			if err != nil {
				return fmt.Errorf("import permissions failed: %w", err)
			}
			result.PermissionsImported = count
		}
		if typeSet["configs"] && len(backup.Configs) > 0 {
			count, err := importConfigs(tx, backup.Configs, now)
			if err != nil {
				return fmt.Errorf("import configs failed: %w", err)
			}
			result.ConfigsImported = count
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func importUsers(tx *gorm.DB, users []model.UserBackup, now int64) (int, error) {
	count := 0
	for _, u := range users {
		item := model.User{
			ID:            u.ID,
			User:          u.User,
			Pwd:           u.Pwd,
			RoleID:        u.RoleID,
			ExpTime:       u.ExpTime,
			Flow:          u.Flow,
			InFlow:        u.InFlow,
			OutFlow:       u.OutFlow,
			FlowResetTime: u.FlowResetTime,
			Num:           u.Num,
			CreatedTime:   u.CreatedTime,
			UpdatedTime:   sql.NullInt64{Int64: now, Valid: true},
			Status:        u.Status,
		}
		err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"user", "pwd", "role_id", "exp_time", "flow", "in_flow", "out_flow",
				"flow_reset_time", "num", "updated_time", "status",
			}),
		}).Create(&item).Error
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func importNodes(tx *gorm.DB, nodes []model.NodeBackup, now int64) (int, error) {
	count := 0
	for _, n := range nodes {
		item := model.Node{
			ID:            n.ID,
			Name:          n.Name,
			Secret:        n.Secret,
			ServerIP:      n.ServerIP,
			ServerIPV4:    sql.NullString{String: n.ServerIPv4, Valid: true},
			ServerIPV6:    sql.NullString{String: n.ServerIPv6, Valid: true},
			Port:          n.Port,
			InterfaceName: sql.NullString{String: n.InterfaceName, Valid: true},
			Version:       sql.NullString{String: n.Version, Valid: true},
			HTTP:          n.HTTP,
			TLS:           n.TLS,
			Socks:         n.Socks,
			CreatedTime:   n.CreatedTime,
			UpdatedTime:   sql.NullInt64{Int64: now, Valid: true},
			Status:        n.Status,
			TCPListenAddr: n.TCPListenAddr,
			UDPListenAddr: n.UDPListenAddr,
			Inx:           n.Inx,
			IsRemote:      n.IsRemote,
			RemoteURL:     sql.NullString{String: n.RemoteURL, Valid: true},
			RemoteToken:   sql.NullString{String: n.RemoteToken, Valid: true},
			RemoteConfig:  sql.NullString{String: n.RemoteConfig, Valid: true},
		}
		err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name", "secret", "server_ip", "server_ip_v4", "server_ip_v6", "port", "interface_name", "version",
				"http", "tls", "socks", "updated_time", "status", "tcp_listen_addr", "udp_listen_addr",
				"inx", "is_remote", "remote_url", "remote_token", "remote_config",
			}),
		}).Create(&item).Error
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func importTunnels(tx *gorm.DB, tunnels []model.TunnelBackup, now int64) (int, error) {
	count := 0
	for _, t := range tunnels {
		item := model.Tunnel{
			ID:           t.ID,
			Name:         t.Name,
			TrafficRatio: t.TrafficRatio,
			Type:         t.Type,
			Protocol:     t.Protocol,
			Flow:         t.Flow,
			CreatedTime:  t.CreatedTime,
			UpdatedTime:  now,
			Status:       t.Status,
			InIP:         sql.NullString{String: t.InIP, Valid: true},
			Inx:          t.Inx,
			IPPreference: t.IPPreference,
		}
		err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name", "traffic_ratio", "type", "protocol", "flow", "updated_time", "status", "in_ip", "inx", "ip_preference",
			}),
		}).Create(&item).Error
		if err != nil {
			return count, err
		}
		for _, ct := range t.ChainTunnels {
			chainItem := model.ChainTunnel{
				ID:        ct.ID,
				TunnelID:  ct.TunnelID,
				ChainType: ct.ChainType,
				NodeID:    ct.NodeID,
				Port:      sql.NullInt64{Int64: int64(ct.Port), Valid: true},
				Strategy:  sql.NullString{String: ct.Strategy, Valid: true},
				Inx:       sql.NullInt64{Int64: int64(ct.Inx), Valid: true},
				Protocol:  sql.NullString{String: ct.Protocol, Valid: true},
			}
			err = tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"chain_type", "node_id", "port", "strategy", "inx", "protocol",
				}),
			}).Create(&chainItem).Error
			if err != nil {
				return count, err
			}
		}
		count++
	}
	return count, nil
}

func importForwards(tx *gorm.DB, forwards []model.ForwardBackup, now int64) (int, error) {
	count := 0
	for _, f := range forwards {
		item := model.Forward{
			ID:          f.ID,
			UserID:      f.UserID,
			UserName:    f.UserName,
			Name:        f.Name,
			TunnelID:    f.TunnelID,
			RemoteAddr:  f.RemoteAddr,
			Strategy:    f.Strategy,
			InFlow:      f.InFlow,
			OutFlow:     f.OutFlow,
			CreatedTime: f.CreatedTime,
			UpdatedTime: now,
			Status:      f.Status,
			Inx:         f.Inx,
		}
		err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"user_id", "user_name", "name", "tunnel_id", "remote_addr", "strategy",
				"in_flow", "out_flow", "updated_time", "status", "inx",
			}),
		}).Create(&item).Error
		if err != nil {
			return count, err
		}
		if f.ForwardPorts != nil {
			if err := tx.Where("forward_id = ?", f.ID).Delete(&model.ForwardPort{}).Error; err != nil {
				return count, err
			}
			for _, fp := range *f.ForwardPorts {
				if err := tx.Create(&model.ForwardPort{ForwardID: f.ID, NodeID: fp.NodeID, Port: fp.Port}).Error; err != nil {
					return count, err
				}
			}
		}
		count++
	}
	return count, nil
}

func importUserTunnels(tx *gorm.DB, userTunnels []model.UserTunnelBackup, _ int64) (int, error) {
	count := 0
	for _, ut := range userTunnels {
		item := model.UserTunnel{
			ID:            ut.ID,
			UserID:        ut.UserID,
			TunnelID:      ut.TunnelID,
			SpeedID:       sql.NullInt64{Int64: ut.SpeedID, Valid: ut.SpeedID > 0},
			Num:           ut.Num,
			Flow:          ut.Flow,
			InFlow:        ut.InFlow,
			OutFlow:       ut.OutFlow,
			FlowResetTime: ut.FlowResetTime,
			ExpTime:       ut.ExpTime,
			Status:        ut.Status,
		}
		err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"user_id", "tunnel_id", "speed_id", "num", "flow", "in_flow", "out_flow",
				"flow_reset_time", "exp_time", "status",
			}),
		}).Create(&item).Error
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func importSpeedLimits(tx *gorm.DB, speedLimits []model.SpeedLimitBackup, now int64) (int, error) {
	count := 0
	for _, sl := range speedLimits {
		item := model.SpeedLimit{
			ID:          sl.ID,
			Name:        sl.Name,
			Speed:       int(sl.Speed),
			TunnelID:    sl.TunnelID,
			TunnelName:  sl.TunnelName,
			CreatedTime: sl.CreatedTime,
			UpdatedTime: sql.NullInt64{Int64: now, Valid: true},
			Status:      sl.Status,
		}
		err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name", "speed", "tunnel_id", "tunnel_name", "updated_time", "status",
			}),
		}).Create(&item).Error
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func importTunnelGroups(tx *gorm.DB, tunnelGroups []model.TunnelGroupBackup, now int64) (int, error) {
	count := 0
	for _, tg := range tunnelGroups {
		item := model.TunnelGroup{
			ID:          tg.ID,
			Name:        tg.Name,
			CreatedTime: tg.CreatedTime,
			UpdatedTime: now,
			Status:      tg.Status,
		}
		err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", "updated_time", "status"}),
		}).Create(&item).Error
		if err != nil {
			return count, err
		}
		if err := tx.Where("tunnel_group_id = ?", tg.ID).Delete(&model.TunnelGroupTunnel{}).Error; err != nil {
			return count, err
		}
		for _, tunnelID := range tg.Tunnels {
			if err := tx.Create(&model.TunnelGroupTunnel{TunnelGroupID: tg.ID, TunnelID: tunnelID, CreatedTime: now}).Error; err != nil {
				return count, err
			}
		}
		count++
	}
	return count, nil
}

func importUserGroups(tx *gorm.DB, userGroups []model.UserGroupBackup, now int64) (int, error) {
	count := 0
	for _, ug := range userGroups {
		item := model.UserGroup{
			ID:          ug.ID,
			Name:        ug.Name,
			CreatedTime: ug.CreatedTime,
			UpdatedTime: now,
			Status:      ug.Status,
		}
		err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", "updated_time", "status"}),
		}).Create(&item).Error
		if err != nil {
			return count, err
		}
		if err := tx.Where("user_group_id = ?", ug.ID).Delete(&model.UserGroupUser{}).Error; err != nil {
			return count, err
		}
		for _, userID := range ug.Users {
			if err := tx.Create(&model.UserGroupUser{UserGroupID: ug.ID, UserID: userID, CreatedTime: now}).Error; err != nil {
				return count, err
			}
		}
		count++
	}
	return count, nil
}

func importPermissions(tx *gorm.DB, permissions []model.PermissionBackup, _ int64) (int, error) {
	count := 0
	for _, p := range permissions {
		item := model.GroupPermission{
			ID:            p.ID,
			UserGroupID:   p.UserGroupID,
			TunnelGroupID: p.TunnelGroupID,
			CreatedTime:   p.CreatedTime,
		}
		err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"user_group_id", "tunnel_group_id"}),
		}).Create(&item).Error
		if err != nil {
			return count, err
		}
		for _, g := range p.Grants {
			grantItem := model.GroupPermissionGrant{
				ID:             g.ID,
				UserGroupID:    g.UserGroupID,
				TunnelGroupID:  g.TunnelGroupID,
				UserTunnelID:   g.UserTunnelID,
				CreatedTime:    g.CreatedTime,
				CreatedByGroup: g.CreatedByGroup,
			}
			err = tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "id"}},
				DoUpdates: clause.AssignmentColumns([]string{"user_tunnel_id", "created_by_group"}),
			}).Create(&grantItem).Error
			if err != nil {
				return count, err
			}
		}
		count++
	}
	return count, nil
}

func importConfigs(tx *gorm.DB, configs map[string]string, now int64) (int, error) {
	count := 0
	for name, value := range configs {
		err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{"value", "time"}),
		}).Create(&model.ViteConfig{Name: name, Value: value, Time: now}).Error
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// ─── Jobs Queries (background stats / expiry) ───────────────────────

func (r *Repository) PurgeOldStatisticsFlows(cutoffMs int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Where("created_time < ?", cutoffMs).Delete(&model.StatisticsFlow{}).Error
}

func (r *Repository) ListAllUserFlowSnapshots() ([]model.UserFlowSnapshot, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var users []model.User
	err := r.db.Order("id ASC").Find(&users).Error
	if err != nil {
		return nil, err
	}
	out := make([]model.UserFlowSnapshot, len(users))
	for i, u := range users {
		out[i] = model.UserFlowSnapshot{UserID: u.ID, InFlow: u.InFlow, OutFlow: u.OutFlow}
	}
	return out, nil
}

func (r *Repository) GetLastStatisticsFlowTotal(userID int64) (sql.NullInt64, error) {
	if r == nil || r.db == nil {
		return sql.NullInt64{}, errors.New("repository not initialized")
	}
	var sf model.StatisticsFlow
	err := r.db.Where("user_id = ?", userID).Order("id DESC").First(&sf).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return sql.NullInt64{}, nil
	}
	if err != nil {
		return sql.NullInt64{}, err
	}
	return sql.NullInt64{Int64: sf.TotalFlow, Valid: true}, nil
}

func (r *Repository) CreateStatisticsFlow(userID, flow, totalFlow int64, timeText string, createdTime int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Create(&model.StatisticsFlow{
		UserID: userID, Flow: flow, TotalFlow: totalFlow,
		Time: timeText, CreatedTime: createdTime,
	}).Error
}

func (r *Repository) ResetUserMonthlyFlow(day int, lastDay int) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	updates := map[string]interface{}{"in_flow": 0, "out_flow": 0}
	if day == lastDay {
		return r.db.Model(&model.User{}).
			Where("flow_reset_time != 0 AND (flow_reset_time = ? OR flow_reset_time > ?)", day, lastDay).
			Updates(updates).Error
	}
	return r.db.Model(&model.User{}).
		Where("flow_reset_time != 0 AND flow_reset_time = ?", day).
		Updates(updates).Error
}

func (r *Repository) ResetUserTunnelMonthlyFlow(day int, lastDay int) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	updates := map[string]interface{}{"in_flow": 0, "out_flow": 0}
	if day == lastDay {
		return r.db.Model(&model.UserTunnel{}).
			Where("flow_reset_time != 0 AND (flow_reset_time = ? OR flow_reset_time > ?)", day, lastDay).
			Updates(updates).Error
	}
	return r.db.Model(&model.UserTunnel{}).
		Where("flow_reset_time != 0 AND flow_reset_time = ?", day).
		Updates(updates).Error
}

func (r *Repository) ListExpiredActiveUserIDs(nowMs int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var ids []int64
	err := r.db.Model(&model.User{}).
		Where("role_id != 0 AND status = 1 AND exp_time IS NOT NULL AND exp_time < ?", nowMs).
		Pluck("id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *Repository) DisableUser(userID int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.User{}).Where("id = ?", userID).Update("status", 0).Error
}

func (r *Repository) ListExpiredActiveUserTunnels(nowMs int64) ([]model.ExpiredUserTunnel, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var uts []model.UserTunnel
	err := r.db.Where("status = 1 AND exp_time IS NOT NULL AND exp_time < ?", nowMs).Find(&uts).Error
	if err != nil {
		return nil, err
	}
	out := make([]model.ExpiredUserTunnel, len(uts))
	for i, ut := range uts {
		out[i] = model.ExpiredUserTunnel{ID: ut.ID, UserID: ut.UserID, TunnelID: ut.TunnelID}
	}
	return out, nil
}

func (r *Repository) DisableUserTunnel(id int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.UserTunnel{}).Where("id = ?", id).Update("status", 0).Error
}

func (r *Repository) GetUserTunnelByID(id int64) (*model.UserTunnel, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var ut model.UserTunnel
	err := r.db.Where("id = ?", id).First(&ut).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ut, nil
}

// ─── Migration ───────────────────────────────────────────────────────

const currentSchemaVersion = 2

var ensurePostgresIDDefaultsFn = ensurePostgresIDDefaults

func getSchemaVersion(db *gorm.DB) int {
	var v model.SchemaVersion
	if err := db.First(&v).Error; err != nil {
		db.Create(&model.SchemaVersion{Version: 0})
		return 0
	}
	return v.Version
}

func setSchemaVersion(db *gorm.DB, ver int) {
	db.Model(&model.SchemaVersion{}).Where("1=1").Update("version", ver)
}

func migrateSchema(db *gorm.DB) error {
	if db == nil {
		return errors.New("nil db")
	}

	if err := ensurePostgresIDDefaultsFn(db); err != nil {
		return err
	}

	ver := getSchemaVersion(db)
	if ver >= currentSchemaVersion {
		return nil
	}

	// Normalize strategy columns
	normalizeStrategy := func(modelRef interface{}, table, defaultValue string) error {
		result := db.Model(modelRef).Where("strategy IS NULL").Update("strategy", defaultValue)
		if result.Error != nil {
			msg := strings.ToLower(result.Error.Error())
			if strings.Contains(msg, "no such table") || (strings.Contains(msg, "relation") && strings.Contains(msg, "does not exist")) {
				return nil
			}
			return fmt.Errorf("normalize %s.strategy: %w", table, result.Error)
		}
		return nil
	}

	if err := normalizeStrategy(&model.Forward{}, "forward", "fifo"); err != nil {
		return err
	}
	if err := normalizeStrategy(&model.ChainTunnel{}, "chain_tunnel", "round"); err != nil {
		return err
	}
	if err := normalizeStrategy(&model.PeerShareRuntime{}, "peer_share_runtime", "round"); err != nil {
		return err
	}

	setSchemaVersion(db, currentSchemaVersion)
	return nil
}

func ensurePostgresIDDefaults(db *gorm.DB) error {
	if db.Dialector.Name() != "postgres" {
		return nil
	}
	type idRow struct {
		TableSchema string
		TableName   string
	}
	var rows []idRow
	err := db.Table("information_schema.table_constraints AS tc").
		Select("c.table_schema, c.table_name").
		Joins("JOIN information_schema.key_column_usage AS kcu ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema").
		Joins("JOIN information_schema.columns AS c ON c.table_schema = kcu.table_schema AND c.table_name = kcu.table_name AND c.column_name = kcu.column_name").
		Where("tc.constraint_type = ?", "PRIMARY KEY").
		Where("kcu.column_name = ?", "id").
		Where("c.data_type IN ?", []string{"integer", "bigint"}).
		Where("c.is_identity = ?", "NO").
		Where("c.table_schema = current_schema()").
		Order("c.table_name ASC").
		Scan(&rows).Error
	if err != nil {
		return fmt.Errorf("discover postgres id columns: %w", err)
	}

	for _, r := range rows {
		if err := ensurePostgresTableIDDefault(db, r.TableSchema, r.TableName); err != nil {
			return fmt.Errorf("repair %s.%s id default: %w", r.TableSchema, r.TableName, err)
		}
	}
	return nil
}

func ensurePostgresTableIDDefault(db *gorm.DB, schemaName, tableName string) error {
	type defaultRow struct {
		ColumnDefault sql.NullString `gorm:"column:column_default"`
	}
	var row defaultRow
	err := db.Table("information_schema.columns").
		Select("column_default").
		Where("table_schema = ? AND table_name = ? AND column_name = 'id'", schemaName, tableName).
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return err
	}
	defaultExpr := row.ColumnDefault

	hasNextvalDefault := defaultExpr.Valid && strings.Contains(strings.ToLower(defaultExpr.String), "nextval(")
	seqRef := ""
	if hasNextvalDefault {
		seqRef = extractNextvalRegclass(defaultExpr.String)
	}

	if !hasNextvalDefault || seqRef == "" {
		seqName := tableName + "_id_seq"
		if err := db.Exec(fmt.Sprintf("CREATE SEQUENCE IF NOT EXISTS %s.%s", quoteSQLIdentifier(schemaName), quoteSQLIdentifier(seqName))).Error; err != nil {
			return err
		}
		seqRef = schemaName + "." + seqName
		if err := db.Exec(fmt.Sprintf(
			"ALTER TABLE %s.%s ALTER COLUMN id SET DEFAULT nextval(%s::regclass)",
			quoteSQLIdentifier(schemaName), quoteSQLIdentifier(tableName), quoteSQLLiteral(seqRef),
		)).Error; err != nil {
			return err
		}
		if err := db.Exec(fmt.Sprintf(
			"ALTER SEQUENCE %s.%s OWNED BY %s.%s.id",
			quoteSQLIdentifier(schemaName), quoteSQLIdentifier(seqName),
			quoteSQLIdentifier(schemaName), quoteSQLIdentifier(tableName),
		)).Error; err != nil {
			return err
		}
	}

	return syncPostgresTableIDSequence(db, schemaName, tableName, seqRef)
}

func syncPostgresTableIDSequence(db *gorm.DB, schemaName, tableName, seqRef string) error {
	type maxRow struct {
		MaxID int64 `gorm:"column:max_id"`
	}
	var row maxRow
	qualifiedTable := fmt.Sprintf("%s.%s", quoteSQLIdentifier(schemaName), quoteSQLIdentifier(tableName))
	err := db.Table(qualifiedTable).
		Select("COALESCE(MAX(id), 0) AS max_id").
		Scan(&row).Error
	if err != nil {
		return err
	}
	maxID := row.MaxID

	setVal := maxID
	isCalled := true
	if maxID <= 0 {
		setVal = 1
		isCalled = false
	}

	return db.Exec(`SELECT setval(?::regclass, ?, ?)`, seqRef, setVal, isCalled).Error
}

func extractNextvalRegclass(defaultExpr string) string {
	nextvalIdx := strings.Index(strings.ToLower(defaultExpr), "nextval(")
	if nextvalIdx < 0 {
		return ""
	}
	expr := defaultExpr[nextvalIdx:]
	firstQuote := strings.Index(expr, "'")
	if firstQuote < 0 {
		return ""
	}
	expr = expr[firstQuote+1:]
	secondQuote := strings.Index(expr, "'")
	if secondQuote < 0 {
		return ""
	}
	return strings.TrimSpace(expr[:secondQuote])
}

func quoteSQLIdentifier(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

func quoteSQLLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// ─── Helper Functions ────────────────────────────────────────────────

func resolveForwardIngress(db *gorm.DB, forwardID int64, tunnelID int64) (string, sql.NullInt64, error) {
	var tunnelInIP sql.NullString
	db.Model(&model.Tunnel{}).Select("in_ip").Where("id = ?", tunnelID).Limit(1).Scan(&tunnelInIP)

	type fpRow struct {
		Port     sql.NullInt64
		ServerIP sql.NullString
	}
	var fpRows []fpRow
	err := db.Model(&model.ForwardPort{}).
		Select("forward_port.port, node.server_ip").
		Joins("LEFT JOIN node ON node.id = forward_port.node_id").
		Where("forward_port.forward_id = ?", forwardID).
		Order("forward_port.id ASC").
		Find(&fpRows).Error
	if err != nil {
		return "", sql.NullInt64{}, err
	}

	ports := make([]int64, 0)
	nodePairs := make([]string, 0)
	seenPorts := make(map[int64]struct{})
	seenPairs := make(map[string]struct{})

	for _, row := range fpRows {
		if !row.Port.Valid {
			continue
		}
		if _, ok := seenPorts[row.Port.Int64]; !ok {
			seenPorts[row.Port.Int64] = struct{}{}
			ports = append(ports, row.Port.Int64)
		}
		if row.ServerIP.Valid && strings.TrimSpace(row.ServerIP.String) != "" {
			pair := fmt.Sprintf("%s:%d", strings.TrimSpace(row.ServerIP.String), row.Port.Int64)
			if _, ok := seenPairs[pair]; !ok {
				seenPairs[pair] = struct{}{}
				nodePairs = append(nodePairs, pair)
			}
		}
	}

	if len(ports) == 0 {
		return "", sql.NullInt64{}, nil
	}

	inPort := sql.NullInt64{Int64: ports[0], Valid: true}

	entries := make([]string, 0)
	if tunnelInIP.Valid && strings.TrimSpace(tunnelInIP.String) != "" {
		tunnelIPs := strings.Split(tunnelInIP.String, ",")
		seen := make(map[string]struct{})
		for _, ip := range tunnelIPs {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			if _, ok := seen[ip]; ok {
				continue
			}
			seen[ip] = struct{}{}
			for _, port := range ports {
				entries = append(entries, fmt.Sprintf("%s:%d", ip, port))
			}
		}
	} else {
		entries = append(entries, nodePairs...)
	}

	return strings.Join(entries, ","), inPort, nil
}

func nullableString(v sql.NullString) interface{} {
	if v.Valid {
		return v.String
	}
	return nil
}

func nullableForwardIngress(v string) interface{} {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return v
}

func nullableInt64(v sql.NullInt64) interface{} {
	if v.Valid {
		return v.Int64
	}
	return nil
}

func unixMilliNow() int64 {
	return time.Now().UnixMilli()
}

func ensureParentDir(dbPath string) error {
	if dbPath == "" {
		return fmt.Errorf("empty db path")
	}
	dir := filepath.Dir(dbPath)
	if dir == "" || dir == "." {
		return nil
	}
	return osMkdirAll(dir)
}

var osMkdirAll = func(path string) error {
	return os.MkdirAll(path, 0o755)
}

// Suppress unused import warning for log
var _ = log.Printf
