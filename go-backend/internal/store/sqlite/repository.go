package sqlite

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"go-backend/internal/store"
	pgstore "go-backend/internal/store/postgres"
	_ "modernc.org/sqlite"
)

//go:embed sql/schema.sql
var embeddedSchema string

//go:embed sql/data.sql
var embeddedSeedData string

// Execer is an interface that both *store.DB and *store.Tx satisfy.
// Used to allow import functions to work with both regular DB and transactions.
type Execer interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

type Repository struct {
	db *store.DB
}

func (r *Repository) DB() *store.DB {
	if r == nil {
		return nil
	}
	return r.db
}

type User struct {
	ID            int64
	User          string
	Pwd           string
	RoleID        int
	ExpTime       int64
	Flow          int64
	InFlow        int64
	OutFlow       int64
	FlowResetTime int64
	Num           int
	CreatedTime   int64
	UpdatedTime   sql.NullInt64
	Status        int
}

type ViteConfig struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Value string `json:"value"`
	Time  int64  `json:"time"`
}

type Announcement struct {
	ID          int64         `json:"id"`
	Content     string        `json:"content"`
	Enabled     int           `json:"enabled"`
	CreatedTime int64         `json:"created_time"`
	UpdatedTime sql.NullInt64 `json:"updated_time,omitempty"`
}

type UserTunnelDetail struct {
	ID            int64
	UserID        int64
	TunnelID      int64
	TunnelName    string
	TunnelFlow    int
	Flow          int64
	InFlow        int64
	OutFlow       int64
	Num           int
	FlowResetTime int64
	ExpTime       int64
	SpeedID       sql.NullInt64
	SpeedLimit    sql.NullString
	Speed         sql.NullInt64
}

type UserForwardDetail struct {
	ID         int64
	Name       string
	TunnelID   int64
	TunnelName string
	InIP       string
	InPort     sql.NullInt64
	RemoteAddr string
	InFlow     int64
	OutFlow    int64
	Status     int
	CreatedAt  int64
}

type StatisticsFlow struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"userId"`
	Flow      int64  `json:"flow"`
	TotalFlow int64  `json:"totalFlow"`
	Time      string `json:"time"`
}

type Node struct {
	ID           int64
	Secret       string
	Version      sql.NullString
	HTTP         int
	TLS          int
	Socks        int
	Status       int
	IsRemote     int
	RemoteURL    sql.NullString
	RemoteToken  sql.NullString
	RemoteConfig sql.NullString
}

type PeerShare struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	NodeID         int64  `json:"nodeId"`
	Token          string `json:"token"`
	MaxBandwidth   int64  `json:"maxBandwidth"`
	ExpiryTime     int64  `json:"expiryTime"`
	PortRangeStart int    `json:"portRangeStart"`
	PortRangeEnd   int    `json:"portRangeEnd"`
	CurrentFlow    int64  `json:"currentFlow"`
	IsActive       int    `json:"isActive"`
	CreatedTime    int64  `json:"createdTime"`
	UpdatedTime    int64  `json:"updatedTime"`
	AllowedDomains string `json:"allowedDomains"`
	AllowedIPs     string `json:"allowedIps"`
}

type PeerShareRuntime struct {
	ID            int64
	ShareID       int64
	NodeID        int64
	ReservationID string
	ResourceKey   string
	BindingID     string
	Role          string
	ChainName     string
	ServiceName   string
	Protocol      string
	Strategy      string
	Port          int
	Target        string
	Applied       int
	Status        int
	CreatedTime   int64
	UpdatedTime   int64
}

type FederationTunnelBinding struct {
	ID              int64
	TunnelID        int64
	NodeID          int64
	ChainType       int
	HopInx          int
	RemoteURL       string
	ResourceKey     string
	RemoteBindingID string
	AllocatedPort   int
	Status          int
	CreatedTime     int64
	UpdatedTime     int64
}

func Open(path string) (*Repository, error) {
	if err := ensureParentDir(path); err != nil {
		return nil, err
	}

	// Use _pragma DSN parameters so every connection from the pool gets
	// the same settings (busy_timeout and synchronous are per-connection).
	dsn := "file:" + path +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)"
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db := store.Wrap(raw, store.DialectSQLite)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := bootstrapSchema(db, embeddedSchema, embeddedSeedData); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := migrateSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Repository{db: db}, nil
}

func OpenPostgres(dsn string) (*Repository, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("empty postgres dsn")
	}

	raw, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db := store.Wrap(raw, store.DialectPostgres)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := bootstrapSchema(db, pgstore.EmbeddedSchema, pgstore.EmbeddedSeedData); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := migrateSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Repository{db: db}, nil
}

func (r *Repository) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

func (r *Repository) GetUserByUsername(username string) (*User, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	row := r.db.QueryRow(`
		SELECT id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status
		FROM user WHERE user = ? LIMIT 1
	`, username)
	user := &User{}
	if err := row.Scan(
		&user.ID, &user.User, &user.Pwd, &user.RoleID, &user.ExpTime,
		&user.Flow, &user.InFlow, &user.OutFlow, &user.FlowResetTime,
		&user.Num, &user.CreatedTime, &user.UpdatedTime, &user.Status,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func (r *Repository) GetConfigByName(name string) (*ViteConfig, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	row := r.db.QueryRow(`SELECT id, name, value, time FROM vite_config WHERE name = ? LIMIT 1`, name)
	cfg := &ViteConfig{}
	if err := row.Scan(&cfg.ID, &cfg.Name, &cfg.Value, &cfg.Time); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return cfg, nil
}

func (r *Repository) ListConfigs() (map[string]string, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	rows, err := r.db.Query(`SELECT name, value FROM vite_config`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return nil, err
		}
		result[name] = value
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Repository) UpsertConfig(name, value string, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}

	_, err := r.db.Exec(`
		INSERT INTO vite_config(name, value, time)
		VALUES(?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET value=excluded.value, time=excluded.time
	`, name, value, now)
	return err
}

func (r *Repository) GetAnnouncement() (*Announcement, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	row := r.db.QueryRow(`SELECT id, content, enabled, created_time, updated_time FROM announcement ORDER BY id DESC LIMIT 1`)
	ann := &Announcement{}
	if err := row.Scan(&ann.ID, &ann.Content, &ann.Enabled, &ann.CreatedTime, &ann.UpdatedTime); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return ann, nil
}

func (r *Repository) UpsertAnnouncement(content string, enabled int, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}

	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM announcement`).Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		_, err = r.db.Exec(`
			INSERT INTO announcement(content, enabled, created_time, updated_time)
			VALUES(?, ?, ?, ?)
		`, content, enabled, now, now)
	} else {
		_, err = r.db.Exec(`
			UPDATE announcement SET content = ?, enabled = ?, updated_time = ?
		`, content, enabled, now)
	}
	return err
}

func (r *Repository) GetUserByID(id int64) (*User, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	row := r.db.QueryRow(`
		SELECT id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status
		FROM user WHERE id = ? LIMIT 1
	`, id)
	user := &User{}
	if err := row.Scan(
		&user.ID, &user.User, &user.Pwd, &user.RoleID, &user.ExpTime,
		&user.Flow, &user.InFlow, &user.OutFlow, &user.FlowResetTime,
		&user.Num, &user.CreatedTime, &user.UpdatedTime, &user.Status,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func (r *Repository) UsernameExistsExceptID(username string, exceptID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("repository not initialized")
	}

	row := r.db.QueryRow(`SELECT COUNT(1) FROM user WHERE user = ? AND id != ?`, username, exceptID)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) UpdateUserNameAndPassword(userID int64, username, passwordMD5 string, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	_, err := r.db.Exec(`UPDATE user SET user = ?, pwd = ?, updated_time = ? WHERE id = ?`, username, passwordMD5, now, userID)
	return err
}

func (r *Repository) GetUserPackageTunnels(userID int64) ([]UserTunnelDetail, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	rows, err := r.db.Query(`
		SELECT ut.id, ut.user_id, ut.tunnel_id, t.name, t.flow, ut.flow, ut.in_flow, ut.out_flow,
		       ut.num, ut.flow_reset_time, ut.exp_time, ut.speed_id, sl.name, sl.speed
		FROM user_tunnel ut
		LEFT JOIN tunnel t ON t.id = ut.tunnel_id
		LEFT JOIN speed_limit sl ON sl.id = ut.speed_id
		WHERE ut.user_id = ?
		ORDER BY ut.id ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]UserTunnelDetail, 0)
	for rows.Next() {
		var item UserTunnelDetail
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.TunnelID, &item.TunnelName, &item.TunnelFlow,
			&item.Flow, &item.InFlow, &item.OutFlow, &item.Num, &item.FlowResetTime,
			&item.ExpTime, &item.SpeedID, &item.SpeedLimit, &item.Speed,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *Repository) GetUserPackageForwards(userID int64) ([]UserForwardDetail, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	rows, err := r.db.Query(`
		SELECT f.id, f.name, f.tunnel_id, COALESCE(t.name, ''), f.remote_addr, f.in_flow, f.out_flow, f.status, f.created_time
		FROM forward f
		LEFT JOIN tunnel t ON t.id = f.tunnel_id
		WHERE f.user_id = ?
		ORDER BY f.id ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]UserForwardDetail, 0)
	for rows.Next() {
		var item UserForwardDetail
		if err := rows.Scan(
			&item.ID, &item.Name, &item.TunnelID, &item.TunnelName, &item.RemoteAddr,
			&item.InFlow, &item.OutFlow, &item.Status, &item.CreatedAt,
		); err != nil {
			return nil, err
		}

		inIP, inPort, err := resolveForwardIngress(r.db, item.ID, item.TunnelID)
		if err != nil {
			return nil, err
		}
		item.InIP = inIP
		item.InPort = inPort

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *Repository) GetStatisticsFlows(userID int64, limit int) ([]StatisticsFlow, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	rows, err := r.db.Query(`
		SELECT id, user_id, flow, total_flow, time
		FROM statistics_flow
		WHERE user_id = ?
		ORDER BY id DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]StatisticsFlow, 0)
	for rows.Next() {
		var item StatisticsFlow
		if err := rows.Scan(&item.ID, &item.UserID, &item.Flow, &item.TotalFlow, &item.Time); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *Repository) NodeExistsBySecret(secret string) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("repository not initialized")
	}

	row := r.db.QueryRow(`SELECT COUNT(1) FROM node WHERE secret = ?`, secret)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) GetNodeBySecret(secret string) (*Node, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	row := r.db.QueryRow(`SELECT id, secret, version, http, tls, socks, status, is_remote, remote_url, remote_token, remote_config FROM node WHERE secret = ? LIMIT 1`, secret)
	var n Node
	if err := row.Scan(&n.ID, &n.Secret, &n.Version, &n.HTTP, &n.TLS, &n.Socks, &n.Status, &n.IsRemote, &n.RemoteURL, &n.RemoteToken, &n.RemoteConfig); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &n, nil
}

func (r *Repository) GetNodeByID(id int64) (*Node, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	row := r.db.QueryRow(`SELECT id, secret, version, http, tls, socks, status, is_remote, remote_url, remote_token, remote_config FROM node WHERE id = ? LIMIT 1`, id)
	var n Node
	if err := row.Scan(&n.ID, &n.Secret, &n.Version, &n.HTTP, &n.TLS, &n.Socks, &n.Status, &n.IsRemote, &n.RemoteURL, &n.RemoteToken, &n.RemoteConfig); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &n, nil
}

func (r *Repository) UpdateNodeOnline(nodeID int64, status int, version string, httpVal, tlsVal, socksVal int) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	_, err := r.db.Exec(`UPDATE node SET status = ?, version = ?, http = ?, tls = ?, socks = ?, updated_time = ? WHERE id = ?`,
		status, version, httpVal, tlsVal, socksVal, unixMilliNow(), nodeID)
	return err
}

func (r *Repository) UpdateNodeStatus(nodeID int64, status int) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	_, err := r.db.Exec(`UPDATE node SET status = ?, updated_time = ? WHERE id = ?`, status, unixMilliNow(), nodeID)
	return err
}

func (r *Repository) AddFlow(forwardID, userID int64, userTunnelID int64, inFlow, outFlow int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`UPDATE forward SET in_flow = in_flow + ?, out_flow = out_flow + ? WHERE id = ?`, inFlow, outFlow, forwardID); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE user SET in_flow = in_flow + ?, out_flow = out_flow + ? WHERE id = ?`, inFlow, outFlow, userID); err != nil {
		return err
	}
	if userTunnelID > 0 {
		if _, err = tx.Exec(`UPDATE user_tunnel SET in_flow = in_flow + ?, out_flow = out_flow + ? WHERE id = ?`, inFlow, outFlow, userTunnelID); err != nil {
			return err
		}
	}

	err = tx.Commit()
	return err
}

func (r *Repository) ListNodes() ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	rows, err := r.db.Query(`
		SELECT id, inx, name, server_ip, server_ip_v4, server_ip_v6, port, tcp_listen_addr, udp_listen_addr, version, http, tls, socks, status, is_remote, remote_url, remote_token, remote_config
		FROM node
		ORDER BY inx ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, inx int64
		var name, serverIP, port string
		var serverIPV4, serverIPV6, tcpListen, udpListen, version, remoteURL, remoteToken, remoteConfig sql.NullString
		var httpVal, tlsVal, socksVal, status, isRemote int

		if err := rows.Scan(&id, &inx, &name, &serverIP, &serverIPV4, &serverIPV6, &port, &tcpListen, &udpListen, &version, &httpVal, &tlsVal, &socksVal, &status, &isRemote, &remoteURL, &remoteToken, &remoteConfig); err != nil {
			return nil, err
		}

		items = append(items, map[string]interface{}{
			"id":            id,
			"inx":           inx,
			"name":          name,
			"ip":            serverIP,
			"serverIp":      serverIP,
			"serverIpV4":    nullableString(serverIPV4),
			"serverIpV6":    nullableString(serverIPV6),
			"port":          port,
			"tcpListenAddr": nullableString(tcpListen),
			"udpListenAddr": nullableString(udpListen),
			"version":       nullableString(version),
			"http":          httpVal,
			"tls":           tlsVal,
			"socks":         socksVal,
			"status":        status,
			"isRemote":      isRemote,
			"remoteUrl":     nullableString(remoteURL),
			"remoteToken":   nullableString(remoteToken),
			"remoteConfig":  nullableString(remoteConfig),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ListUsers() ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	rows, err := r.db.Query(`
		SELECT id, user, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status
		FROM user
		WHERE role_id != 0
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id int64
		var user string
		var roleID int
		var expTime, flow, inFlow, outFlow, flowResetTime, createdTime int64
		var num, status int
		var updatedTime sql.NullInt64

		if err := rows.Scan(&id, &user, &roleID, &expTime, &flow, &inFlow, &outFlow, &flowResetTime, &num, &createdTime, &updatedTime, &status); err != nil {
			return nil, err
		}

		items = append(items, map[string]interface{}{
			"id":            id,
			"user":          user,
			"name":          user,
			"roleId":        roleID,
			"status":        status,
			"flow":          flow,
			"num":           num,
			"expTime":       expTime,
			"flowResetTime": flowResetTime,
			"createdTime":   createdTime,
			"updatedTime":   nullableInt64(updatedTime),
			"inFlow":        inFlow,
			"outFlow":       outFlow,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ListSpeedLimits() ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	rows, err := r.db.Query(`
		SELECT id, name, speed, tunnel_id, tunnel_name, status, created_time, updated_time
		FROM speed_limit
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, tunnelID, createdTime int64
		var name, tunnelName string
		var speed, status int
		var updatedTime sql.NullInt64
		if err := rows.Scan(&id, &name, &speed, &tunnelID, &tunnelName, &status, &createdTime, &updatedTime); err != nil {
			return nil, err
		}
		items = append(items, map[string]interface{}{
			"id":          id,
			"name":        name,
			"speed":       speed,
			"tunnelId":    tunnelID,
			"tunnelName":  tunnelName,
			"status":      status,
			"createdTime": createdTime,
			"updatedTime": nullableInt64(updatedTime),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ListForwards() ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	rows, err := r.db.Query(`
		SELECT f.id, f.user_id, f.user_name, f.name, f.tunnel_id, COALESCE(t.name, ''), f.remote_addr, COALESCE(f.strategy, 'fifo'),
		       f.in_flow, f.out_flow, f.created_time, f.status, f.inx
		FROM forward f
		LEFT JOIN tunnel t ON t.id = f.tunnel_id
		ORDER BY f.inx ASC, f.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, userID, tunnelID, inFlow, outFlow, createdTime, inx int64
		var userName, name, tunnelName, remoteAddr, strategy string
		var status int

		if err := rows.Scan(&id, &userID, &userName, &name, &tunnelID, &tunnelName, &remoteAddr, &strategy, &inFlow, &outFlow, &createdTime, &status, &inx); err != nil {
			return nil, err
		}

		inIP, inPort, err := resolveForwardIngress(r.db, id, tunnelID)
		if err != nil {
			return nil, err
		}

		items = append(items, map[string]interface{}{
			"id":          id,
			"userId":      userID,
			"userName":    userName,
			"name":        name,
			"tunnelId":    tunnelID,
			"tunnelName":  tunnelName,
			"inIp":        nullableForwardIngress(inIP),
			"inPort":      nullableInt64(inPort),
			"remoteAddr":  remoteAddr,
			"strategy":    strategy,
			"inFlow":      inFlow,
			"outFlow":     outFlow,
			"createdTime": createdTime,
			"status":      status,
			"inx":         inx,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ListUserAccessibleTunnels(userID int64) ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	rows, err := r.db.Query(`
		SELECT t.id, t.name
		FROM user_tunnel ut
		JOIN tunnel t ON t.id = ut.tunnel_id
		WHERE ut.user_id = ? AND t.status = 1
		ORDER BY t.inx ASC, t.id ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		items = append(items, map[string]interface{}{"id": id, "name": name})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ListEnabledTunnelSummaries() ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	rows, err := r.db.Query(`
		SELECT id, name
		FROM tunnel
		WHERE status = 1
		ORDER BY inx ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		items = append(items, map[string]interface{}{"id": id, "name": name})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ListTunnels() ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	rows, err := r.db.Query(`
		SELECT id, inx, name, type, flow, traffic_ratio, status, created_time, in_ip, COALESCE(ip_preference, '')
		FROM tunnel
		ORDER BY inx ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tunnelMap := make(map[int64]map[string]interface{})
	orderedIDs := make([]int64, 0)

	for rows.Next() {
		var id, inx, flow, createdTime int64
		var name string
		var typ, status int
		var trafficRatio float64
		var inIP sql.NullString
		var ipPreference string
		if err := rows.Scan(&id, &inx, &name, &typ, &flow, &trafficRatio, &status, &createdTime, &inIP, &ipPreference); err != nil {
			return nil, err
		}

		tunnelMap[id] = map[string]interface{}{
			"id":           id,
			"inx":          inx,
			"name":         name,
			"type":         typ,
			"flow":         flow,
			"trafficRatio": trafficRatio,
			"status":       status,
			"createdTime":  createdTime,
			"inIp":         nullableString(inIP),
			"ipPreference": ipPreference,
			"inNodeId":     make([]map[string]interface{}, 0),
			"outNodeId":    make([]map[string]interface{}, 0),
			"chainNodes":   make([][]map[string]interface{}, 0),
		}
		orderedIDs = append(orderedIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	nodeIPMap := map[int64]string{}
	nRows, err := r.db.Query(`SELECT id, server_ip FROM node`)
	if err == nil {
		for nRows.Next() {
			var id int64
			var ip string
			if scanErr := nRows.Scan(&id, &ip); scanErr == nil {
				nodeIPMap[id] = ip
			}
		}
		_ = nRows.Close()
	}

	chainRows, err := r.db.Query(`
		SELECT tunnel_id, CAST(chain_type AS INTEGER), node_id, protocol, strategy, COALESCE(inx, 0)
		FROM chain_tunnel
		ORDER BY tunnel_id ASC, CAST(chain_type AS INTEGER) ASC, inx ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer chainRows.Close()

	chainBucket := map[int64]map[int][]map[string]interface{}{}
	inNodeIPs := map[int64][]string{}

	for chainRows.Next() {
		var tunnelID, nodeID, inx int64
		var chainType int
		var protocol, strategy sql.NullString
		if err := chainRows.Scan(&tunnelID, &chainType, &nodeID, &protocol, &strategy, &inx); err != nil {
			return nil, err
		}

		t, ok := tunnelMap[tunnelID]
		if !ok {
			continue
		}

		nodeObj := map[string]interface{}{
			"nodeId":    nodeID,
			"chainType": chainType,
			"inx":       inx,
		}
		if protocol.Valid {
			nodeObj["protocol"] = protocol.String
		}
		if strategy.Valid {
			nodeObj["strategy"] = strategy.String
		}

		switch chainType {
		case 1:
			t["inNodeId"] = append(t["inNodeId"].([]map[string]interface{}), nodeObj)
			if ip, ok := nodeIPMap[nodeID]; ok && ip != "" {
				inNodeIPs[tunnelID] = append(inNodeIPs[tunnelID], ip)
			}
		case 2:
			if _, ok := chainBucket[tunnelID]; !ok {
				chainBucket[tunnelID] = map[int][]map[string]interface{}{}
			}
			chainBucket[tunnelID][int(inx)] = append(chainBucket[tunnelID][int(inx)], nodeObj)
		case 3:
			t["outNodeId"] = append(t["outNodeId"].([]map[string]interface{}), nodeObj)
		}
	}
	if err := chainRows.Err(); err != nil {
		return nil, err
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

func (r *Repository) ListTunnelGroups() ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	rows, err := r.db.Query(`SELECT id, name, status, created_time FROM tunnel_group ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, createdTime int64
		var name string
		var status int
		if err := rows.Scan(&id, &name, &status, &createdTime); err != nil {
			return nil, err
		}

		ids, names, err := r.listTunnelGroupMembers(id)
		if err != nil {
			return nil, err
		}

		result = append(result, map[string]interface{}{
			"id":          id,
			"name":        name,
			"status":      status,
			"tunnelIds":   ids,
			"tunnelNames": names,
			"createdTime": createdTime,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Repository) ListUserGroups() ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	rows, err := r.db.Query(`SELECT id, name, status, created_time FROM user_group ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, createdTime int64
		var name string
		var status int
		if err := rows.Scan(&id, &name, &status, &createdTime); err != nil {
			return nil, err
		}

		ids, names, err := r.listUserGroupMembers(id)
		if err != nil {
			return nil, err
		}

		result = append(result, map[string]interface{}{
			"id":          id,
			"name":        name,
			"status":      status,
			"userIds":     ids,
			"userNames":   names,
			"createdTime": createdTime,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Repository) ListGroupPermissions() ([]map[string]interface{}, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	rows, err := r.db.Query(`
		SELECT gp.id, gp.user_group_id, ug.name, gp.tunnel_group_id, tg.name, gp.created_time
		FROM group_permission gp
		LEFT JOIN user_group ug ON ug.id = gp.user_group_id
		LEFT JOIN tunnel_group tg ON tg.id = gp.tunnel_group_id
		ORDER BY gp.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, userGroupID, tunnelGroupID, createdTime int64
		var userGroupName, tunnelGroupName sql.NullString
		if err := rows.Scan(&id, &userGroupID, &userGroupName, &tunnelGroupID, &tunnelGroupName, &createdTime); err != nil {
			return nil, err
		}

		result = append(result, map[string]interface{}{
			"id":              id,
			"userGroupId":     userGroupID,
			"userGroupName":   nullableString(userGroupName),
			"tunnelGroupId":   tunnelGroupID,
			"tunnelGroupName": nullableString(tunnelGroupName),
			"createdTime":     createdTime,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Repository) listTunnelGroupMembers(groupID int64) ([]int64, []string, error) {
	rows, err := r.db.Query(`
		SELECT t.id, t.name
		FROM tunnel_group_tunnel tgt
		JOIN tunnel t ON t.id = tgt.tunnel_id
		WHERE tgt.tunnel_group_id = ?
		ORDER BY t.id ASC
	`, groupID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	ids := make([]int64, 0)
	names := make([]string, 0)
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, nil, err
		}
		ids = append(ids, id)
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return ids, names, nil
}

func (r *Repository) listUserGroupMembers(groupID int64) ([]int64, []string, error) {
	rows, err := r.db.Query(`
		SELECT u.id, u.user
		FROM user_group_user ugu
		JOIN user u ON u.id = ugu.user_id
		WHERE ugu.user_group_id = ?
		ORDER BY u.id ASC
	`, groupID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	ids := make([]int64, 0)
	names := make([]string, 0)
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, nil, err
		}
		ids = append(ids, id)
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return ids, names, nil
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

func resolveForwardIngress(db *store.DB, forwardID int64, tunnelID int64) (string, sql.NullInt64, error) {
	var tunnelInIP sql.NullString
	if err := db.QueryRow(`SELECT in_ip FROM tunnel WHERE id = ? LIMIT 1`, tunnelID).Scan(&tunnelInIP); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return "", sql.NullInt64{}, err
		}
	}

	rows, err := db.Query(`
		SELECT fp.port, n.server_ip
		FROM forward_port fp
		LEFT JOIN node n ON n.id = fp.node_id
		WHERE fp.forward_id = ?
		ORDER BY fp.id ASC
	`, forwardID)
	if err != nil {
		return "", sql.NullInt64{}, err
	}
	defer rows.Close()

	ports := make([]int64, 0)
	nodePairs := make([]string, 0)
	seenPorts := make(map[int64]struct{})
	seenPairs := make(map[string]struct{})

	for rows.Next() {
		var port sql.NullInt64
		var nodeIP sql.NullString
		if err := rows.Scan(&port, &nodeIP); err != nil {
			return "", sql.NullInt64{}, err
		}
		if !port.Valid {
			continue
		}
		if _, ok := seenPorts[port.Int64]; !ok {
			seenPorts[port.Int64] = struct{}{}
			ports = append(ports, port.Int64)
		}
		if nodeIP.Valid && strings.TrimSpace(nodeIP.String) != "" {
			pair := fmt.Sprintf("%s:%d", strings.TrimSpace(nodeIP.String), port.Int64)
			if _, ok := seenPairs[pair]; !ok {
				seenPairs[pair] = struct{}{}
				nodePairs = append(nodePairs, pair)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return "", sql.NullInt64{}, err
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

func bootstrapSchema(db *store.DB, schemaSQL, seedSQL string) error {
	if db == nil {
		return errors.New("nil db")
	}

	if _, err := db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("apply schema.sql: %w", err)
	}

	if _, err := db.Exec(seedSQL); err != nil {
		return fmt.Errorf("apply data.sql: %w", err)
	}
	return nil
}

const currentSchemaVersion = 2

var ensurePostgresIDDefaultsFn = ensurePostgresIDDefaults

func getSchemaVersion(db *store.DB) int {
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL DEFAULT 0)`)
	var v int
	if err := db.QueryRow(`SELECT version FROM schema_version LIMIT 1`).Scan(&v); err != nil {
		_, _ = db.Exec(`INSERT INTO schema_version(version) VALUES(0)`)
		return 0
	}
	return v
}

func setSchemaVersion(db *store.DB, v int) {
	_, _ = db.Exec(`UPDATE schema_version SET version = ?`, v)
}

func migrateSchema(db *store.DB) error {
	if db == nil {
		return errors.New("nil db")
	}

	ver := getSchemaVersion(db)
	if db.Dialect() == store.DialectPostgres {
		if err := ensurePostgresIDDefaultsFn(db); err != nil {
			return err
		}
	}
	if ver >= currentSchemaVersion {
		return nil
	}

	ensureColumn := func(table, col, typ string) {
		var dummy interface{}
		err := db.QueryRow(fmt.Sprintf("SELECT %s FROM %s LIMIT 1", col, table)).Scan(&dummy)
		if err == nil || errors.Is(err, sql.ErrNoRows) {
			return
		}
		if isMissingColumnError(db.Dialect(), err) {
			if _, alterErr := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, col, typ)); alterErr != nil {
				log.Printf("failed to add column %s to %s: %v", col, table, alterErr)
			}
		}
	}

	columnsByTable := map[string]map[string]string{
		"peer_share": {
			"allowed_domains": "TEXT DEFAULT ''",
			"allowed_ips":     "TEXT DEFAULT ''",
		},
		"node": {
			"server_ip_v4":  "VARCHAR(100)",
			"server_ip_v6":  "VARCHAR(100)",
			"inx":           "INTEGER NOT NULL DEFAULT 0",
			"is_remote":     "INTEGER DEFAULT 0",
			"remote_url":    "TEXT",
			"remote_token":  "TEXT",
			"remote_config": "TEXT",
		},
		"tunnel": {
			"inx":           "INTEGER NOT NULL DEFAULT 0",
			"ip_preference": "VARCHAR(10) NOT NULL DEFAULT ''",
		},
		"forward": {
			"inx": "INTEGER NOT NULL DEFAULT 0",
		},
		"chain_tunnel": {
			"inx": "INTEGER",
		},
	}

	for table, columns := range columnsByTable {
		for col, typ := range columns {
			ensureColumn(table, col, typ)
		}
	}

	normalizeStrategy := func(table, defaultValue string) error {
		_, err := db.Exec(fmt.Sprintf("UPDATE %s SET strategy = ? WHERE strategy IS NULL", table), defaultValue)
		if err != nil {
			if isMissingTableError(db.Dialect(), err) {
				return nil
			}
			return fmt.Errorf("normalize %s.strategy: %w", table, err)
		}
		return nil
	}

	if err := normalizeStrategy("forward", "fifo"); err != nil {
		return err
	}
	if err := normalizeStrategy("chain_tunnel", "round"); err != nil {
		return err
	}
	if err := normalizeStrategy("peer_share_runtime", "round"); err != nil {
		return err
	}

	setSchemaVersion(db, currentSchemaVersion)
	return nil
}

func ensurePostgresIDDefaults(db *store.DB) error {
	rows, err := db.Query(`
		SELECT c.table_schema, c.table_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		 AND tc.table_schema = kcu.table_schema
		JOIN information_schema.columns c
		  ON c.table_schema = kcu.table_schema
		 AND c.table_name = kcu.table_name
		 AND c.column_name = kcu.column_name
		WHERE tc.constraint_type = 'PRIMARY KEY'
		  AND kcu.column_name = 'id'
		  AND c.data_type IN ('integer', 'bigint')
		  AND c.is_identity = 'NO'
		  AND c.table_schema = current_schema()
		ORDER BY c.table_name ASC
	`)
	if err != nil {
		return fmt.Errorf("discover postgres id columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var schemaName string
		var tableName string
		if err := rows.Scan(&schemaName, &tableName); err != nil {
			return fmt.Errorf("scan postgres id table row: %w", err)
		}
		if err := ensurePostgresTableIDDefault(db, schemaName, tableName); err != nil {
			return fmt.Errorf("repair %s.%s id default: %w", schemaName, tableName, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate postgres id tables: %w", err)
	}

	return nil
}

func ensurePostgresTableIDDefault(db *store.DB, schemaName, tableName string) error {
	var defaultExpr sql.NullString
	if err := db.QueryRow(`
		SELECT column_default
		FROM information_schema.columns
		WHERE table_schema = ?
		  AND table_name = ?
		  AND column_name = 'id'
		LIMIT 1
	`, schemaName, tableName).Scan(&defaultExpr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}

	hasNextvalDefault := defaultExpr.Valid && strings.Contains(strings.ToLower(defaultExpr.String), "nextval(")

	var serialSeq sql.NullString
	if err := db.QueryRow(`
		SELECT pg_get_serial_sequence(quote_ident(?) || '.' || quote_ident(?), 'id')
	`, schemaName, tableName).Scan(&serialSeq); err != nil {
		return err
	}

	seqRef := strings.TrimSpace(serialSeq.String)
	if seqRef == "" && hasNextvalDefault {
		seqRef = extractNextvalRegclass(defaultExpr.String)
	}

	if !hasNextvalDefault || seqRef == "" {
		seqName := tableName + "_id_seq"
		if _, err := db.Exec(fmt.Sprintf("CREATE SEQUENCE IF NOT EXISTS %s.%s", quoteSQLIdentifier(schemaName), quoteSQLIdentifier(seqName))); err != nil {
			return err
		}

		seqRef = schemaName + "." + seqName
		if _, err := db.Exec(fmt.Sprintf(
			"ALTER TABLE %s.%s ALTER COLUMN id SET DEFAULT nextval(%s::regclass)",
			quoteSQLIdentifier(schemaName),
			quoteSQLIdentifier(tableName),
			quoteSQLLiteral(seqRef),
		)); err != nil {
			return err
		}

		if _, err := db.Exec(fmt.Sprintf(
			"ALTER SEQUENCE %s.%s OWNED BY %s.%s.id",
			quoteSQLIdentifier(schemaName),
			quoteSQLIdentifier(seqName),
			quoteSQLIdentifier(schemaName),
			quoteSQLIdentifier(tableName),
		)); err != nil {
			return err
		}
	}

	return syncPostgresTableIDSequence(db, schemaName, tableName, seqRef)
}

func syncPostgresTableIDSequence(db *store.DB, schemaName, tableName, seqRef string) error {
	var maxID int64
	if err := db.QueryRow(fmt.Sprintf(
		"SELECT COALESCE(MAX(id), 0) FROM %s.%s",
		quoteSQLIdentifier(schemaName),
		quoteSQLIdentifier(tableName),
	)).Scan(&maxID); err != nil {
		return err
	}

	setVal := maxID
	isCalled := true
	if maxID <= 0 {
		setVal = 1
		isCalled = false
	}

	if _, err := db.Exec(`SELECT setval(?::regclass, ?, ?)`, seqRef, setVal, isCalled); err != nil {
		return err
	}

	return nil
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

func isMissingColumnError(dialect store.Dialect, err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if dialect == store.DialectPostgres {
		return strings.Contains(msg, "column") && strings.Contains(msg, "does not exist")
	}
	return strings.Contains(msg, "no such column")
}

func isMissingTableError(dialect store.Dialect, err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if dialect == store.DialectPostgres {
		return strings.Contains(msg, "relation") && strings.Contains(msg, "does not exist")
	}
	return strings.Contains(msg, "no such table")
}

func (r *Repository) CreatePeerShare(share *PeerShare) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	_, err := r.db.Exec(`
		INSERT INTO peer_share(name, node_id, token, max_bandwidth, expiry_time, port_range_start, port_range_end, current_flow, is_active, created_time, updated_time, allowed_domains, allowed_ips)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, share.Name, share.NodeID, share.Token, share.MaxBandwidth, share.ExpiryTime, share.PortRangeStart, share.PortRangeEnd, share.CurrentFlow, share.IsActive, share.CreatedTime, share.UpdatedTime, share.AllowedDomains, share.AllowedIPs)
	return err
}

func (r *Repository) UpdatePeerShare(share *PeerShare) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	_, err := r.db.Exec(`
		UPDATE peer_share SET name=?, max_bandwidth=?, expiry_time=?, port_range_start=?, port_range_end=?, is_active=?, updated_time=?, allowed_domains=?, allowed_ips=?
		WHERE id=?
	`, share.Name, share.MaxBandwidth, share.ExpiryTime, share.PortRangeStart, share.PortRangeEnd, share.IsActive, share.UpdatedTime, share.AllowedDomains, share.AllowedIPs, share.ID)
	return err
}

func (r *Repository) DeletePeerShare(id int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, _ = tx.Exec(`DELETE FROM peer_share_runtime WHERE share_id = ?`, id)
	if _, err := tx.Exec(`DELETE FROM peer_share WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) GetPeerShare(id int64) (*PeerShare, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	row := r.db.QueryRow(`SELECT id, name, node_id, token, max_bandwidth, expiry_time, port_range_start, port_range_end, current_flow, is_active, created_time, updated_time, allowed_domains, allowed_ips FROM peer_share WHERE id = ?`, id)
	var s PeerShare
	if err := row.Scan(&s.ID, &s.Name, &s.NodeID, &s.Token, &s.MaxBandwidth, &s.ExpiryTime, &s.PortRangeStart, &s.PortRangeEnd, &s.CurrentFlow, &s.IsActive, &s.CreatedTime, &s.UpdatedTime, &s.AllowedDomains, &s.AllowedIPs); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

func (r *Repository) GetPeerShareByToken(token string) (*PeerShare, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	row := r.db.QueryRow(`SELECT id, name, node_id, token, max_bandwidth, expiry_time, port_range_start, port_range_end, current_flow, is_active, created_time, updated_time, allowed_domains, allowed_ips FROM peer_share WHERE token = ?`, token)
	var s PeerShare
	if err := row.Scan(&s.ID, &s.Name, &s.NodeID, &s.Token, &s.MaxBandwidth, &s.ExpiryTime, &s.PortRangeStart, &s.PortRangeEnd, &s.CurrentFlow, &s.IsActive, &s.CreatedTime, &s.UpdatedTime, &s.AllowedDomains, &s.AllowedIPs); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

func (r *Repository) ListPeerShares() ([]PeerShare, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	rows, err := r.db.Query(`SELECT id, name, node_id, token, max_bandwidth, expiry_time, port_range_start, port_range_end, current_flow, is_active, created_time, updated_time, allowed_domains, allowed_ips FROM peer_share ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shares []PeerShare
	for rows.Next() {
		var s PeerShare
		if err := rows.Scan(&s.ID, &s.Name, &s.NodeID, &s.Token, &s.MaxBandwidth, &s.ExpiryTime, &s.PortRangeStart, &s.PortRangeEnd, &s.CurrentFlow, &s.IsActive, &s.CreatedTime, &s.UpdatedTime, &s.AllowedDomains, &s.AllowedIPs); err != nil {
			return nil, err
		}
		shares = append(shares, s)
	}
	return shares, nil
}

func (r *Repository) GetPeerShareRuntimeByResourceKey(shareID int64, resourceKey string) (*PeerShareRuntime, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	row := r.db.QueryRow(`
		SELECT id, share_id, node_id, reservation_id, resource_key, binding_id, role, chain_name, service_name, protocol, strategy, port, target, applied, status, created_time, updated_time
		FROM peer_share_runtime
		WHERE share_id = ? AND resource_key = ?
		LIMIT 1
	`, shareID, resourceKey)
	var item PeerShareRuntime
	if err := row.Scan(&item.ID, &item.ShareID, &item.NodeID, &item.ReservationID, &item.ResourceKey, &item.BindingID, &item.Role, &item.ChainName, &item.ServiceName, &item.Protocol, &item.Strategy, &item.Port, &item.Target, &item.Applied, &item.Status, &item.CreatedTime, &item.UpdatedTime); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) GetPeerShareRuntimeByReservationID(shareID int64, reservationID string) (*PeerShareRuntime, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	row := r.db.QueryRow(`
		SELECT id, share_id, node_id, reservation_id, resource_key, binding_id, role, chain_name, service_name, protocol, strategy, port, target, applied, status, created_time, updated_time
		FROM peer_share_runtime
		WHERE share_id = ? AND reservation_id = ?
		LIMIT 1
	`, shareID, reservationID)
	var item PeerShareRuntime
	if err := row.Scan(&item.ID, &item.ShareID, &item.NodeID, &item.ReservationID, &item.ResourceKey, &item.BindingID, &item.Role, &item.ChainName, &item.ServiceName, &item.Protocol, &item.Strategy, &item.Port, &item.Target, &item.Applied, &item.Status, &item.CreatedTime, &item.UpdatedTime); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) GetPeerShareRuntimeByBindingID(shareID int64, bindingID string) (*PeerShareRuntime, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	row := r.db.QueryRow(`
		SELECT id, share_id, node_id, reservation_id, resource_key, binding_id, role, chain_name, service_name, protocol, strategy, port, target, applied, status, created_time, updated_time
		FROM peer_share_runtime
		WHERE share_id = ? AND binding_id = ?
		LIMIT 1
	`, shareID, bindingID)
	var item PeerShareRuntime
	if err := row.Scan(&item.ID, &item.ShareID, &item.NodeID, &item.ReservationID, &item.ResourceKey, &item.BindingID, &item.Role, &item.ChainName, &item.ServiceName, &item.Protocol, &item.Strategy, &item.Port, &item.Target, &item.Applied, &item.Status, &item.CreatedTime, &item.UpdatedTime); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) GetPeerShareRuntimeByID(id int64) (*PeerShareRuntime, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	row := r.db.QueryRow(`
		SELECT id, share_id, node_id, reservation_id, resource_key, binding_id, role, chain_name, service_name, protocol, strategy, port, target, applied, status, created_time, updated_time
		FROM peer_share_runtime
		WHERE id = ?
		LIMIT 1
	`, id)
	var item PeerShareRuntime
	if err := row.Scan(&item.ID, &item.ShareID, &item.NodeID, &item.ReservationID, &item.ResourceKey, &item.BindingID, &item.Role, &item.ChainName, &item.ServiceName, &item.Protocol, &item.Strategy, &item.Port, &item.Target, &item.Applied, &item.Status, &item.CreatedTime, &item.UpdatedTime); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) ListActivePeerShareRuntimesByShareID(shareID int64) ([]PeerShareRuntime, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	rows, err := r.db.Query(`
		SELECT id, share_id, node_id, reservation_id, resource_key, binding_id, role, chain_name, service_name, protocol, strategy, port, target, applied, status, created_time, updated_time
		FROM peer_share_runtime
		WHERE share_id = ? AND status = 1
		ORDER BY port ASC, id ASC
	`, shareID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]PeerShareRuntime, 0)
	for rows.Next() {
		var item PeerShareRuntime
		if err := rows.Scan(&item.ID, &item.ShareID, &item.NodeID, &item.ReservationID, &item.ResourceKey, &item.BindingID, &item.Role, &item.ChainName, &item.ServiceName, &item.Protocol, &item.Strategy, &item.Port, &item.Target, &item.Applied, &item.Status, &item.CreatedTime, &item.UpdatedTime); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) AddPeerShareCurrentFlow(shareID int64, delta int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if shareID <= 0 || delta <= 0 {
		return nil
	}
	_, err := r.db.Exec(`UPDATE peer_share SET current_flow = current_flow + ?, updated_time = ? WHERE id = ?`, delta, unixMilliNow(), shareID)
	return err
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
	_, err := r.db.Exec(`UPDATE peer_share SET current_flow = 0, updated_time = ? WHERE id = ?`, updatedTime, shareID)
	return err
}

func (r *Repository) CreatePeerShareRuntime(item *PeerShareRuntime) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if item == nil {
		return errors.New("runtime item is nil")
	}
	_, err := r.db.Exec(`
		INSERT INTO peer_share_runtime(share_id, node_id, reservation_id, resource_key, binding_id, role, chain_name, service_name, protocol, strategy, port, target, applied, status, created_time, updated_time)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, item.ShareID, item.NodeID, item.ReservationID, item.ResourceKey, item.BindingID, item.Role, item.ChainName, item.ServiceName, item.Protocol, item.Strategy, item.Port, item.Target, item.Applied, item.Status, item.CreatedTime, item.UpdatedTime)
	return err
}

func (r *Repository) UpdatePeerShareRuntime(item *PeerShareRuntime) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if item == nil {
		return errors.New("runtime item is nil")
	}
	_, err := r.db.Exec(`
		UPDATE peer_share_runtime
		SET binding_id = ?, role = ?, chain_name = ?, service_name = ?, protocol = ?, strategy = ?, port = ?, target = ?, applied = ?, status = ?, updated_time = ?
		WHERE id = ?
	`, item.BindingID, item.Role, item.ChainName, item.ServiceName, item.Protocol, item.Strategy, item.Port, item.Target, item.Applied, item.Status, item.UpdatedTime, item.ID)
	return err
}

func (r *Repository) MarkPeerShareRuntimeReleased(id int64, updatedTime int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	_, err := r.db.Exec(`UPDATE peer_share_runtime SET status = 0, updated_time = ? WHERE id = ?`, updatedTime, id)
	return err
}

func (r *Repository) ListActivePeerShareRuntimePorts(shareID int64, nodeID int64) ([]int, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	rows, err := r.db.Query(`SELECT port FROM peer_share_runtime WHERE share_id = ? AND node_id = ? AND status = 1 AND port > 0`, shareID, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]int, 0)
	for rows.Next() {
		var port int
		if err := rows.Scan(&port); err != nil {
			return nil, err
		}
		if port > 0 {
			out = append(out, port)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) UpsertFederationTunnelBinding(item *FederationTunnelBinding) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if item == nil {
		return errors.New("binding item is nil")
	}
	_, err := r.db.Exec(`
		INSERT INTO federation_tunnel_binding(tunnel_id, node_id, chain_type, hop_inx, remote_url, resource_key, remote_binding_id, allocated_port, status, created_time, updated_time)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(tunnel_id, node_id, chain_type, hop_inx)
		DO UPDATE SET
			remote_url = excluded.remote_url,
			resource_key = excluded.resource_key,
			remote_binding_id = excluded.remote_binding_id,
			allocated_port = excluded.allocated_port,
			status = excluded.status,
			updated_time = excluded.updated_time
	`, item.TunnelID, item.NodeID, item.ChainType, item.HopInx, item.RemoteURL, item.ResourceKey, item.RemoteBindingID, item.AllocatedPort, item.Status, item.CreatedTime, item.UpdatedTime)
	return err
}

func (r *Repository) ListActiveFederationTunnelBindingsByTunnel(tunnelID int64) ([]FederationTunnelBinding, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	rows, err := r.db.Query(`
		SELECT id, tunnel_id, node_id, chain_type, hop_inx, remote_url, resource_key, remote_binding_id, allocated_port, status, created_time, updated_time
		FROM federation_tunnel_binding
		WHERE tunnel_id = ? AND status = 1
		ORDER BY chain_type ASC, hop_inx ASC, id ASC
	`, tunnelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]FederationTunnelBinding, 0)
	for rows.Next() {
		var item FederationTunnelBinding
		if err := rows.Scan(&item.ID, &item.TunnelID, &item.NodeID, &item.ChainType, &item.HopInx, &item.RemoteURL, &item.ResourceKey, &item.RemoteBindingID, &item.AllocatedPort, &item.Status, &item.CreatedTime, &item.UpdatedTime); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) DeleteFederationTunnelBindingsByTunnel(tunnelID int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	_, err := r.db.Exec(`DELETE FROM federation_tunnel_binding WHERE tunnel_id = ?`, tunnelID)
	return err
}

var osMkdirAll = func(path string) error {
	return os.MkdirAll(path, 0o755)
}

// ============ Backup/Export Data Structures ============

// BackupData represents the full backup structure
type BackupData struct {
	Version      string              `json:"version"`
	ExportedAt   int64               `json:"exportedAt"`
	Users        []UserBackup        `json:"users,omitempty"`
	Nodes        []NodeBackup        `json:"nodes,omitempty"`
	Tunnels      []TunnelBackup      `json:"tunnels,omitempty"`
	Forwards     []ForwardBackup     `json:"forwards,omitempty"`
	UserTunnels  []UserTunnelBackup  `json:"userTunnels,omitempty"`
	SpeedLimits  []SpeedLimitBackup  `json:"speedLimits,omitempty"`
	TunnelGroups []TunnelGroupBackup `json:"tunnelGroups,omitempty"`
	UserGroups   []UserGroupBackup   `json:"userGroups,omitempty"`
	Permissions  []PermissionBackup  `json:"permissions,omitempty"`
	Configs      map[string]string   `json:"configs,omitempty"`
}

type UserBackup struct {
	ID            int64  `json:"id"`
	User          string `json:"user"`
	Pwd           string `json:"pwd"`
	RoleID        int    `json:"roleId"`
	ExpTime       int64  `json:"expTime"`
	Flow          int64  `json:"flow"`
	InFlow        int64  `json:"inFlow"`
	OutFlow       int64  `json:"outFlow"`
	FlowResetTime int64  `json:"flowResetTime"`
	Num           int    `json:"num"`
	CreatedTime   int64  `json:"createdTime"`
	UpdatedTime   int64  `json:"updatedTime,omitempty"`
	Status        int    `json:"status"`
}

type NodeBackup struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Secret        string `json:"secret"`
	ServerIP      string `json:"serverIp"`
	ServerIPv4    string `json:"serverIpV4,omitempty"`
	ServerIPv6    string `json:"serverIpV6,omitempty"`
	Port          string `json:"port"`
	InterfaceName string `json:"interfaceName,omitempty"`
	Version       string `json:"version,omitempty"`
	HTTP          int    `json:"http"`
	TLS           int    `json:"tls"`
	Socks         int    `json:"socks"`
	CreatedTime   int64  `json:"createdTime"`
	UpdatedTime   int64  `json:"updatedTime,omitempty"`
	Status        int    `json:"status"`
	TCPListenAddr string `json:"tcpListenAddr"`
	UDPListenAddr string `json:"udpListenAddr"`
	Inx           int    `json:"inx"`
	IsRemote      int    `json:"isRemote"`
	RemoteURL     string `json:"remoteUrl,omitempty"`
	RemoteToken   string `json:"remoteToken,omitempty"`
	RemoteConfig  string `json:"remoteConfig,omitempty"`
}

type TunnelBackup struct {
	ID           int64               `json:"id"`
	Name         string              `json:"name"`
	TrafficRatio float64             `json:"trafficRatio"`
	Type         int                 `json:"type"`
	Protocol     string              `json:"protocol"`
	Flow         int64               `json:"flow"`
	CreatedTime  int64               `json:"createdTime"`
	UpdatedTime  int64               `json:"updatedTime"`
	Status       int                 `json:"status"`
	InIP         string              `json:"inIp,omitempty"`
	Inx          int                 `json:"inx"`
	IPPreference string              `json:"ipPreference,omitempty"`
	ChainTunnels []ChainTunnelBackup `json:"chainTunnels,omitempty"`
}

type ChainTunnelBackup struct {
	ID        int64  `json:"id"`
	TunnelID  int64  `json:"tunnelId"`
	ChainType string `json:"chainType"`
	NodeID    int64  `json:"nodeId"`
	Port      int    `json:"port,omitempty"`
	Strategy  string `json:"strategy,omitempty"`
	Inx       int    `json:"inx,omitempty"`
	Protocol  string `json:"protocol,omitempty"`
}

type ForwardBackup struct {
	ID           int64                `json:"id"`
	UserID       int64                `json:"userId"`
	UserName     string               `json:"userName"`
	Name         string               `json:"name"`
	TunnelID     int64                `json:"tunnelId"`
	RemoteAddr   string               `json:"remoteAddr"`
	Strategy     string               `json:"strategy"`
	InFlow       int64                `json:"inFlow"`
	OutFlow      int64                `json:"outFlow"`
	CreatedTime  int64                `json:"createdTime"`
	UpdatedTime  int64                `json:"updatedTime"`
	Status       int                  `json:"status"`
	Inx          int                  `json:"inx"`
	ForwardPorts *[]ForwardPortBackup `json:"forwardPorts,omitempty"`
}

type ForwardPortBackup struct {
	NodeID int64 `json:"nodeId"`
	Port   int   `json:"port"`
}

type UserTunnelBackup struct {
	ID            int64 `json:"id"`
	UserID        int64 `json:"userId"`
	TunnelID      int64 `json:"tunnelId"`
	SpeedID       int64 `json:"speedId,omitempty"`
	Num           int   `json:"num"`
	Flow          int64 `json:"flow"`
	InFlow        int64 `json:"inFlow"`
	OutFlow       int64 `json:"outFlow"`
	FlowResetTime int64 `json:"flowResetTime"`
	ExpTime       int64 `json:"expTime"`
	Status        int   `json:"status"`
}

type SpeedLimitBackup struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Speed       int64  `json:"speed"`
	TunnelID    int64  `json:"tunnelId"`
	TunnelName  string `json:"tunnelName"`
	CreatedTime int64  `json:"createdTime"`
	UpdatedTime int64  `json:"updatedTime,omitempty"`
	Status      int    `json:"status"`
}

type TunnelGroupBackup struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	CreatedTime int64   `json:"createdTime"`
	UpdatedTime int64   `json:"updatedTime"`
	Status      int     `json:"status"`
	Tunnels     []int64 `json:"tunnels,omitempty"`
}

type UserGroupBackup struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	CreatedTime int64   `json:"createdTime"`
	UpdatedTime int64   `json:"updatedTime"`
	Status      int     `json:"status"`
	Users       []int64 `json:"users,omitempty"`
}

type PermissionBackup struct {
	ID             int64                   `json:"id"`
	UserGroupID    int64                   `json:"userGroupId"`
	TunnelGroupID  int64                   `json:"tunnelGroupId"`
	CreatedTime    int64                   `json:"createdTime"`
	CreatedByGroup int                     `json:"createdByGroup"`
	Grants         []PermissionGrantBackup `json:"grants,omitempty"`
}

type PermissionGrantBackup struct {
	ID             int64 `json:"id"`
	UserGroupID    int64 `json:"userGroupId"`
	TunnelGroupID  int64 `json:"tunnelGroupId"`
	UserTunnelID   int64 `json:"userTunnelId"`
	CreatedTime    int64 `json:"createdTime"`
	CreatedByGroup int   `json:"createdByGroup"`
}

// ============ Export Methods ============

// ExportAll exports all data as BackupData
func (r *Repository) ExportAll() (*BackupData, error) {
	backup := &BackupData{
		Version:    "1.0",
		ExportedAt: unixMilliNow(),
	}

	// Export all data types
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

// ExportPartial exports selected data types
func (r *Repository) ExportPartial(types []string) (*BackupData, error) {
	backup := &BackupData{
		Version:    "1.0",
		ExportedAt: unixMilliNow(),
	}

	typeSet := make(map[string]bool)
	for _, t := range types {
		typeSet[t] = true
	}

	if typeSet["users"] {
		users, err := r.exportUsers()
		if err != nil {
			return nil, fmt.Errorf("export users failed: %w", err)
		}
		backup.Users = users
	}
	if typeSet["nodes"] {
		nodes, err := r.exportNodes()
		if err != nil {
			return nil, fmt.Errorf("export nodes failed: %w", err)
		}
		backup.Nodes = nodes
	}
	if typeSet["tunnels"] {
		tunnels, err := r.exportTunnels()
		if err != nil {
			return nil, fmt.Errorf("export tunnels failed: %w", err)
		}
		backup.Tunnels = tunnels
	}
	if typeSet["forwards"] {
		forwards, err := r.exportForwards()
		if err != nil {
			return nil, fmt.Errorf("export forwards failed: %w", err)
		}
		backup.Forwards = forwards
	}
	if typeSet["userTunnels"] {
		userTunnels, err := r.exportUserTunnels()
		if err != nil {
			return nil, fmt.Errorf("export user tunnels failed: %w", err)
		}
		backup.UserTunnels = userTunnels
	}
	if typeSet["speedLimits"] {
		speedLimits, err := r.exportSpeedLimits()
		if err != nil {
			return nil, fmt.Errorf("export speed limits failed: %w", err)
		}
		backup.SpeedLimits = speedLimits
	}
	if typeSet["tunnelGroups"] {
		tunnelGroups, err := r.exportTunnelGroups()
		if err != nil {
			return nil, fmt.Errorf("export tunnel groups failed: %w", err)
		}
		backup.TunnelGroups = tunnelGroups
	}
	if typeSet["userGroups"] {
		userGroups, err := r.exportUserGroups()
		if err != nil {
			return nil, fmt.Errorf("export user groups failed: %w", err)
		}
		backup.UserGroups = userGroups
	}
	if typeSet["permissions"] {
		permissions, err := r.exportPermissions()
		if err != nil {
			return nil, fmt.Errorf("export permissions failed: %w", err)
		}
		backup.Permissions = permissions
	}
	if typeSet["configs"] {
		configs, err := r.ListConfigs()
		if err != nil {
			return nil, fmt.Errorf("export configs failed: %w", err)
		}
		backup.Configs = configs
	}

	return backup, nil
}

func (r *Repository) exportUsers() ([]UserBackup, error) {
	rows, err := r.db.Query(`
		SELECT id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status
		FROM user ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []UserBackup
	for rows.Next() {
		var u UserBackup
		var updatedTime sql.NullInt64
		if err := rows.Scan(&u.ID, &u.User, &u.Pwd, &u.RoleID, &u.ExpTime, &u.Flow, &u.InFlow, &u.OutFlow, &u.FlowResetTime, &u.Num, &u.CreatedTime, &updatedTime, &u.Status); err != nil {
			return nil, err
		}
		if updatedTime.Valid {
			u.UpdatedTime = updatedTime.Int64
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *Repository) exportNodes() ([]NodeBackup, error) {
	rows, err := r.db.Query(`
		SELECT id, name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx, is_remote, remote_url, remote_token, remote_config
		FROM node ORDER BY inx ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []NodeBackup
	for rows.Next() {
		var n NodeBackup
		var updatedTime sql.NullInt64
		var serverIPv4, serverIPv6, interfaceName, version, remoteURL, remoteToken, remoteConfig sql.NullString
		if err := rows.Scan(&n.ID, &n.Name, &n.Secret, &n.ServerIP, &serverIPv4, &serverIPv6, &n.Port, &interfaceName, &version, &n.HTTP, &n.TLS, &n.Socks, &n.CreatedTime, &updatedTime, &n.Status, &n.TCPListenAddr, &n.UDPListenAddr, &n.Inx, &n.IsRemote, &remoteURL, &remoteToken, &remoteConfig); err != nil {
			return nil, err
		}
		if updatedTime.Valid {
			n.UpdatedTime = updatedTime.Int64
		}
		if serverIPv4.Valid {
			n.ServerIPv4 = serverIPv4.String
		}
		if serverIPv6.Valid {
			n.ServerIPv6 = serverIPv6.String
		}
		if interfaceName.Valid {
			n.InterfaceName = interfaceName.String
		}
		if version.Valid {
			n.Version = version.String
		}
		if remoteURL.Valid {
			n.RemoteURL = remoteURL.String
		}
		if remoteToken.Valid {
			n.RemoteToken = remoteToken.String
		}
		if remoteConfig.Valid {
			n.RemoteConfig = remoteConfig.String
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func (r *Repository) exportTunnels() ([]TunnelBackup, error) {
	rows, err := r.db.Query(`
		SELECT id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx, COALESCE(ip_preference, '')
		FROM tunnel ORDER BY inx ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tunnels []TunnelBackup
	for rows.Next() {
		var t TunnelBackup
		var protocol sql.NullString
		var updatedTime sql.NullInt64
		var inIP sql.NullString
		var inx sql.NullInt64
		if err := rows.Scan(&t.ID, &t.Name, &t.TrafficRatio, &t.Type, &protocol, &t.Flow, &t.CreatedTime, &updatedTime, &t.Status, &inIP, &inx, &t.IPPreference); err != nil {
			return nil, err
		}
		if protocol.Valid {
			t.Protocol = protocol.String
		}
		if updatedTime.Valid {
			t.UpdatedTime = updatedTime.Int64
		}
		if inIP.Valid {
			t.InIP = inIP.String
		}
		if inx.Valid {
			t.Inx = int(inx.Int64)
		}
		// Export chain tunnels
		chainTunnels, err := r.exportChainTunnels(t.ID)
		if err != nil {
			return nil, err
		}
		t.ChainTunnels = chainTunnels
		tunnels = append(tunnels, t)
	}
	return tunnels, rows.Err()
}

func (r *Repository) exportChainTunnels(tunnelID int64) ([]ChainTunnelBackup, error) {
	rows, err := r.db.Query(`
		SELECT id, tunnel_id, chain_type, node_id, port, strategy, inx, protocol
		FROM chain_tunnel WHERE tunnel_id = ? ORDER BY inx ASC, id ASC
	`, tunnelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chainTunnels []ChainTunnelBackup
	for rows.Next() {
		var ct ChainTunnelBackup
		var port sql.NullInt64
		var strategy, protocol sql.NullString
		var inx sql.NullInt64
		if err := rows.Scan(&ct.ID, &ct.TunnelID, &ct.ChainType, &ct.NodeID, &port, &strategy, &inx, &protocol); err != nil {
			return nil, err
		}
		if port.Valid {
			ct.Port = int(port.Int64)
		}
		if strategy.Valid {
			ct.Strategy = strategy.String
		}
		if inx.Valid {
			ct.Inx = int(inx.Int64)
		}
		if protocol.Valid {
			ct.Protocol = protocol.String
		}
		chainTunnels = append(chainTunnels, ct)
	}
	return chainTunnels, rows.Err()
}

func (r *Repository) exportForwards() ([]ForwardBackup, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx
		FROM forward ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var forwards []ForwardBackup
	for rows.Next() {
		var f ForwardBackup
		var strategy sql.NullString
		var updatedTime sql.NullInt64
		var inx sql.NullInt64
		if err := rows.Scan(&f.ID, &f.UserID, &f.UserName, &f.Name, &f.TunnelID, &f.RemoteAddr, &strategy, &f.InFlow, &f.OutFlow, &f.CreatedTime, &updatedTime, &f.Status, &inx); err != nil {
			return nil, err
		}
		if strategy.Valid {
			f.Strategy = strategy.String
		}
		if updatedTime.Valid {
			f.UpdatedTime = updatedTime.Int64
		}
		if inx.Valid {
			f.Inx = int(inx.Int64)
		}

		forwardPorts, err := r.exportForwardPorts(f.ID)
		if err != nil {
			return nil, err
		}
		portsCopy := append([]ForwardPortBackup(nil), forwardPorts...)
		f.ForwardPorts = &portsCopy

		forwards = append(forwards, f)
	}
	return forwards, rows.Err()
}

func (r *Repository) exportForwardPorts(forwardID int64) ([]ForwardPortBackup, error) {
	rows, err := r.db.Query(`
		SELECT node_id, port
		FROM forward_port
		WHERE forward_id = ?
		ORDER BY id ASC
	`, forwardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ports := make([]ForwardPortBackup, 0)
	for rows.Next() {
		var fp ForwardPortBackup
		if err := rows.Scan(&fp.NodeID, &fp.Port); err != nil {
			return nil, err
		}
		ports = append(ports, fp)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ports, nil
}

func (r *Repository) exportUserTunnels() ([]UserTunnelBackup, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status
		FROM user_tunnel ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userTunnels []UserTunnelBackup
	for rows.Next() {
		var ut UserTunnelBackup
		var speedID sql.NullInt64
		if err := rows.Scan(&ut.ID, &ut.UserID, &ut.TunnelID, &speedID, &ut.Num, &ut.Flow, &ut.InFlow, &ut.OutFlow, &ut.FlowResetTime, &ut.ExpTime, &ut.Status); err != nil {
			return nil, err
		}
		if speedID.Valid {
			ut.SpeedID = speedID.Int64
		}
		userTunnels = append(userTunnels, ut)
	}
	return userTunnels, rows.Err()
}

func (r *Repository) exportSpeedLimits() ([]SpeedLimitBackup, error) {
	rows, err := r.db.Query(`
		SELECT id, name, speed, tunnel_id, tunnel_name, created_time, updated_time, status
		FROM speed_limit ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var speedLimits []SpeedLimitBackup
	for rows.Next() {
		var sl SpeedLimitBackup
		var updatedTime sql.NullInt64
		if err := rows.Scan(&sl.ID, &sl.Name, &sl.Speed, &sl.TunnelID, &sl.TunnelName, &sl.CreatedTime, &updatedTime, &sl.Status); err != nil {
			return nil, err
		}
		if updatedTime.Valid {
			sl.UpdatedTime = updatedTime.Int64
		}
		speedLimits = append(speedLimits, sl)
	}
	return speedLimits, rows.Err()
}

func (r *Repository) exportTunnelGroups() ([]TunnelGroupBackup, error) {
	rows, err := r.db.Query(`
		SELECT id, name, created_time, updated_time, status
		FROM tunnel_group ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []TunnelGroupBackup
	for rows.Next() {
		var tg TunnelGroupBackup
		if err := rows.Scan(&tg.ID, &tg.Name, &tg.CreatedTime, &tg.UpdatedTime, &tg.Status); err != nil {
			return nil, err
		}
		// Get tunnel IDs for this group
		tunnelRows, err := r.db.Query(`SELECT tunnel_id FROM tunnel_group_tunnel WHERE tunnel_group_id = ?`, tg.ID)
		if err != nil {
			return nil, err
		}
		for tunnelRows.Next() {
			var tunnelID int64
			if err := tunnelRows.Scan(&tunnelID); err != nil {
				tunnelRows.Close()
				return nil, err
			}
			tg.Tunnels = append(tg.Tunnels, tunnelID)
		}
		tunnelRows.Close()
		groups = append(groups, tg)
	}
	return groups, rows.Err()
}

func (r *Repository) exportUserGroups() ([]UserGroupBackup, error) {
	rows, err := r.db.Query(`
		SELECT id, name, created_time, updated_time, status
		FROM user_group ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []UserGroupBackup
	for rows.Next() {
		var ug UserGroupBackup
		if err := rows.Scan(&ug.ID, &ug.Name, &ug.CreatedTime, &ug.UpdatedTime, &ug.Status); err != nil {
			return nil, err
		}
		// Get user IDs for this group
		userRows, err := r.db.Query(`SELECT user_id FROM user_group_user WHERE user_group_id = ?`, ug.ID)
		if err != nil {
			return nil, err
		}
		for userRows.Next() {
			var userID int64
			if err := userRows.Scan(&userID); err != nil {
				userRows.Close()
				return nil, err
			}
			ug.Users = append(ug.Users, userID)
		}
		userRows.Close()
		groups = append(groups, ug)
	}
	return groups, rows.Err()
}

func (r *Repository) exportPermissions() ([]PermissionBackup, error) {
	rows, err := r.db.Query(`
		SELECT id, user_group_id, tunnel_group_id, created_time
		FROM group_permission ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions []PermissionBackup
	for rows.Next() {
		var p PermissionBackup
		if err := rows.Scan(&p.ID, &p.UserGroupID, &p.TunnelGroupID, &p.CreatedTime); err != nil {
			return nil, err
		}
		p.CreatedByGroup = 0
		// Get grants for this permission
		grantRows, err := r.db.Query(`SELECT id, user_group_id, tunnel_group_id, user_tunnel_id, created_time, created_by_group FROM group_permission_grant WHERE user_group_id = ? AND tunnel_group_id = ?`, p.UserGroupID, p.TunnelGroupID)
		if err != nil {
			return nil, err
		}
		for grantRows.Next() {
			var g PermissionGrantBackup
			if err := grantRows.Scan(&g.ID, &g.UserGroupID, &g.TunnelGroupID, &g.UserTunnelID, &g.CreatedTime, &g.CreatedByGroup); err != nil {
				grantRows.Close()
				return nil, err
			}
			p.Grants = append(p.Grants, g)
		}
		grantRows.Close()
		permissions = append(permissions, p)
	}
	return permissions, rows.Err()
}

// ============ Import Methods ============

// ImportResult contains the result of an import operation
type ImportResult struct {
	UsersImported        int         `json:"usersImported"`
	NodesImported        int         `json:"nodesImported"`
	TunnelsImported      int         `json:"tunnelsImported"`
	ForwardsImported     int         `json:"forwardsImported"`
	UserTunnelsImported  int         `json:"userTunnelsImported"`
	SpeedLimitsImported  int         `json:"speedLimitsImported"`
	TunnelGroupsImported int         `json:"tunnelGroupsImported"`
	UserGroupsImported   int         `json:"userGroupsImported"`
	PermissionsImported  int         `json:"permissionsImported"`
	ConfigsImported      int         `json:"configsImported"`
	AutoBackup           *BackupData `json:"autoBackup,omitempty"`
}

// Import imports data from BackupData with transaction support
func (r *Repository) Import(backup *BackupData, types []string) (*ImportResult, error) {
	result := &ImportResult{}

	typeSet := make(map[string]bool)
	for _, t := range types {
		typeSet[t] = true
	}

	tx, err := r.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := unixMilliNow()

	if typeSet["users"] && len(backup.Users) > 0 {
		count, err := r.importUsers(tx, backup.Users, now)
		if err != nil {
			return nil, fmt.Errorf("import users failed: %w", err)
		}
		result.UsersImported = count
	}

	if typeSet["nodes"] && len(backup.Nodes) > 0 {
		count, err := r.importNodes(tx, backup.Nodes, now)
		if err != nil {
			return nil, fmt.Errorf("import nodes failed: %w", err)
		}
		result.NodesImported = count
	}

	if typeSet["tunnels"] && len(backup.Tunnels) > 0 {
		count, err := r.importTunnels(tx, backup.Tunnels, now)
		if err != nil {
			return nil, fmt.Errorf("import tunnels failed: %w", err)
		}
		result.TunnelsImported = count
	}

	if typeSet["forwards"] && len(backup.Forwards) > 0 {
		count, err := r.importForwards(tx, backup.Forwards, now)
		if err != nil {
			return nil, fmt.Errorf("import forwards failed: %w", err)
		}
		result.ForwardsImported = count
	}

	if typeSet["userTunnels"] && len(backup.UserTunnels) > 0 {
		count, err := r.importUserTunnels(tx, backup.UserTunnels, now)
		if err != nil {
			return nil, fmt.Errorf("import user tunnels failed: %w", err)
		}
		result.UserTunnelsImported = count
	}

	if typeSet["speedLimits"] && len(backup.SpeedLimits) > 0 {
		count, err := r.importSpeedLimits(tx, backup.SpeedLimits, now)
		if err != nil {
			return nil, fmt.Errorf("import speed limits failed: %w", err)
		}
		result.SpeedLimitsImported = count
	}

	if typeSet["tunnelGroups"] && len(backup.TunnelGroups) > 0 {
		count, err := r.importTunnelGroups(tx, backup.TunnelGroups, now)
		if err != nil {
			return nil, fmt.Errorf("import tunnel groups failed: %w", err)
		}
		result.TunnelGroupsImported = count
	}

	if typeSet["userGroups"] && len(backup.UserGroups) > 0 {
		count, err := r.importUserGroups(tx, backup.UserGroups, now)
		if err != nil {
			return nil, fmt.Errorf("import user groups failed: %w", err)
		}
		result.UserGroupsImported = count
	}

	if typeSet["permissions"] && len(backup.Permissions) > 0 {
		count, err := r.importPermissions(tx, backup.Permissions, now)
		if err != nil {
			return nil, fmt.Errorf("import permissions failed: %w", err)
		}
		result.PermissionsImported = count
	}

	if typeSet["configs"] && len(backup.Configs) > 0 {
		count, err := r.importConfigs(tx, backup.Configs, now)
		if err != nil {
			return nil, fmt.Errorf("import configs failed: %w", err)
		}
		result.ConfigsImported = count
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return result, nil
}

func (r *Repository) importUsers(db Execer, users []UserBackup, now int64) (int, error) {
	count := 0
	for _, u := range users {
		_, err := db.Exec(`
			INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				user = excluded.user,
				pwd = excluded.pwd,
				role_id = excluded.role_id,
				exp_time = excluded.exp_time,
				flow = excluded.flow,
				in_flow = excluded.in_flow,
				out_flow = excluded.out_flow,
				flow_reset_time = excluded.flow_reset_time,
				num = excluded.num,
				updated_time = excluded.updated_time,
				status = excluded.status
		`, u.ID, u.User, u.Pwd, u.RoleID, u.ExpTime, u.Flow, u.InFlow, u.OutFlow, u.FlowResetTime, u.Num, u.CreatedTime, now, u.Status)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (r *Repository) UsernameExists(username string) (bool, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(1) FROM user WHERE user = ?`, username).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) importNodes(db Execer, nodes []NodeBackup, now int64) (int, error) {
	count := 0
	for _, n := range nodes {
		_, err := db.Exec(`
			INSERT INTO node(id, name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx, is_remote, remote_url, remote_token, remote_config)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				secret = excluded.secret,
				server_ip = excluded.server_ip,
				server_ip_v4 = excluded.server_ip_v4,
				server_ip_v6 = excluded.server_ip_v6,
				port = excluded.port,
				interface_name = excluded.interface_name,
				version = excluded.version,
				http = excluded.http,
				tls = excluded.tls,
				socks = excluded.socks,
				updated_time = excluded.updated_time,
				status = excluded.status,
				tcp_listen_addr = excluded.tcp_listen_addr,
				udp_listen_addr = excluded.udp_listen_addr,
				inx = excluded.inx,
				is_remote = excluded.is_remote,
				remote_url = excluded.remote_url,
				remote_token = excluded.remote_token,
				remote_config = excluded.remote_config
		`, n.ID, n.Name, n.Secret, n.ServerIP, n.ServerIPv4, n.ServerIPv6, n.Port, n.InterfaceName, n.Version, n.HTTP, n.TLS, n.Socks, n.CreatedTime, now, n.Status, n.TCPListenAddr, n.UDPListenAddr, n.Inx, n.IsRemote, n.RemoteURL, n.RemoteToken, n.RemoteConfig)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (r *Repository) importTunnels(db Execer, tunnels []TunnelBackup, now int64) (int, error) {
	count := 0
	for _, t := range tunnels {
		_, err := db.Exec(`
			INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx, ip_preference)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				traffic_ratio = excluded.traffic_ratio,
				type = excluded.type,
				protocol = excluded.protocol,
				flow = excluded.flow,
				updated_time = excluded.updated_time,
				status = excluded.status,
				in_ip = excluded.in_ip,
				inx = excluded.inx,
				ip_preference = excluded.ip_preference
		`, t.ID, t.Name, t.TrafficRatio, t.Type, t.Protocol, t.Flow, t.CreatedTime, now, t.Status, t.InIP, t.Inx, t.IPPreference)
		if err != nil {
			return count, err
		}
		if len(t.ChainTunnels) > 0 {
			for _, ct := range t.ChainTunnels {
				_, err = db.Exec(`
					INSERT INTO chain_tunnel(id, tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
					VALUES(?, ?, ?, ?, ?, ?, ?, ?)
					ON CONFLICT(id) DO UPDATE SET
						chain_type = excluded.chain_type,
						node_id = excluded.node_id,
						port = excluded.port,
						strategy = excluded.strategy,
						inx = excluded.inx,
						protocol = excluded.protocol
				`, ct.ID, ct.TunnelID, ct.ChainType, ct.NodeID, ct.Port, ct.Strategy, ct.Inx, ct.Protocol)
				if err != nil {
					return count, err
				}
			}
		}
		count++
	}
	return count, nil
}

func (r *Repository) importForwards(db Execer, forwards []ForwardBackup, now int64) (int, error) {
	count := 0
	for _, f := range forwards {
		_, err := db.Exec(`
			INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				user_id = excluded.user_id,
				user_name = excluded.user_name,
				name = excluded.name,
				tunnel_id = excluded.tunnel_id,
				remote_addr = excluded.remote_addr,
				strategy = excluded.strategy,
				in_flow = excluded.in_flow,
				out_flow = excluded.out_flow,
				updated_time = excluded.updated_time,
				status = excluded.status,
				inx = excluded.inx
		`, f.ID, f.UserID, f.UserName, f.Name, f.TunnelID, f.RemoteAddr, f.Strategy, f.InFlow, f.OutFlow, f.CreatedTime, now, f.Status, f.Inx)
		if err != nil {
			return count, err
		}

		if f.ForwardPorts != nil {
			if _, err := db.Exec(`DELETE FROM forward_port WHERE forward_id = ?`, f.ID); err != nil {
				return count, err
			}
			for _, fp := range *f.ForwardPorts {
				if _, err := db.Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(?, ?, ?)`, f.ID, fp.NodeID, fp.Port); err != nil {
					return count, err
				}
			}
		}

		count++
	}
	return count, nil
}

func (r *Repository) importUserTunnels(db Execer, userTunnels []UserTunnelBackup, now int64) (int, error) {
	count := 0
	for _, ut := range userTunnels {
		var speedID interface{}
		if ut.SpeedID > 0 {
			speedID = ut.SpeedID
		}
		_, err := db.Exec(`
			INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				user_id = excluded.user_id,
				tunnel_id = excluded.tunnel_id,
				speed_id = excluded.speed_id,
				num = excluded.num,
				flow = excluded.flow,
				in_flow = excluded.in_flow,
				out_flow = excluded.out_flow,
				flow_reset_time = excluded.flow_reset_time,
				exp_time = excluded.exp_time,
				status = excluded.status
		`, ut.ID, ut.UserID, ut.TunnelID, speedID, ut.Num, ut.Flow, ut.InFlow, ut.OutFlow, ut.FlowResetTime, ut.ExpTime, ut.Status)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (r *Repository) importSpeedLimits(db Execer, speedLimits []SpeedLimitBackup, now int64) (int, error) {
	count := 0
	for _, sl := range speedLimits {
		_, err := db.Exec(`
			INSERT INTO speed_limit(id, name, speed, tunnel_id, tunnel_name, created_time, updated_time, status)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				speed = excluded.speed,
				tunnel_id = excluded.tunnel_id,
				tunnel_name = excluded.tunnel_name,
				updated_time = excluded.updated_time,
				status = excluded.status
		`, sl.ID, sl.Name, sl.Speed, sl.TunnelID, sl.TunnelName, sl.CreatedTime, now, sl.Status)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (r *Repository) importTunnelGroups(db Execer, tunnelGroups []TunnelGroupBackup, now int64) (int, error) {
	count := 0
	for _, tg := range tunnelGroups {
		_, err := db.Exec(`
			INSERT INTO tunnel_group(id, name, created_time, updated_time, status)
			VALUES(?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				updated_time = excluded.updated_time,
				status = excluded.status
		`, tg.ID, tg.Name, tg.CreatedTime, now, tg.Status)
		if err != nil {
			return count, err
		}
		_, err = db.Exec(`DELETE FROM tunnel_group_tunnel WHERE tunnel_group_id = ?`, tg.ID)
		if err != nil {
			return count, err
		}
		for _, tunnelID := range tg.Tunnels {
			_, err = db.Exec(`
				INSERT INTO tunnel_group_tunnel(tunnel_group_id, tunnel_id, created_time)
				VALUES(?, ?, ?)
			`, tg.ID, tunnelID, now)
			if err != nil {
				return count, err
			}
		}
		count++
	}
	return count, nil
}

func (r *Repository) importUserGroups(db Execer, userGroups []UserGroupBackup, now int64) (int, error) {
	count := 0
	for _, ug := range userGroups {
		_, err := db.Exec(`
			INSERT INTO user_group(id, name, created_time, updated_time, status)
			VALUES(?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				updated_time = excluded.updated_time,
				status = excluded.status
		`, ug.ID, ug.Name, ug.CreatedTime, now, ug.Status)
		if err != nil {
			return count, err
		}
		_, err = db.Exec(`DELETE FROM user_group_user WHERE user_group_id = ?`, ug.ID)
		if err != nil {
			return count, err
		}
		for _, userID := range ug.Users {
			_, err = db.Exec(`
				INSERT INTO user_group_user(user_group_id, user_id, created_time)
				VALUES(?, ?, ?)
			`, ug.ID, userID, now)
			if err != nil {
				return count, err
			}
		}
		count++
	}
	return count, nil
}

func (r *Repository) importPermissions(db Execer, permissions []PermissionBackup, now int64) (int, error) {
	count := 0
	for _, p := range permissions {
		_, err := db.Exec(`
			INSERT INTO group_permission(id, user_group_id, tunnel_group_id, created_time, created_by_group)
			VALUES(?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				user_group_id = excluded.user_group_id,
				tunnel_group_id = excluded.tunnel_group_id,
				created_by_group = excluded.created_by_group
		`, p.ID, p.UserGroupID, p.TunnelGroupID, p.CreatedTime, p.CreatedByGroup)
		if err != nil {
			return count, err
		}
		for _, g := range p.Grants {
			_, err = db.Exec(`
				INSERT INTO group_permission_grant(id, user_group_id, tunnel_group_id, user_tunnel_id, created_time, created_by_group)
				VALUES(?, ?, ?, ?, ?, ?)
				ON CONFLICT(id) DO UPDATE SET
					user_tunnel_id = excluded.user_tunnel_id,
					created_by_group = excluded.created_by_group
			`, g.ID, g.UserGroupID, g.TunnelGroupID, g.UserTunnelID, g.CreatedTime, g.CreatedByGroup)
			if err != nil {
				return count, err
			}
		}
		count++
	}
	return count, nil
}

func (r *Repository) importConfigs(db Execer, configs map[string]string, now int64) (int, error) {
	count := 0
	for name, value := range configs {
		err := r.UpsertConfig(name, value, now)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
