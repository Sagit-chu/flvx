package sqlite

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed sql/schema.sql
var embeddedSchema string

//go:embed sql/data.sql
var embeddedSeedData string

type Repository struct {
	db *sql.DB
}

func (r *Repository) DB() *sql.DB {
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
}

func Open(path string) (*Repository, error) {
	if err := ensureParentDir(path); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := bootstrapSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	if err := ensurePeerSchema(db); err != nil {
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
		SELECT f.id, f.name, f.tunnel_id, t.name, f.remote_addr, f.in_flow, f.out_flow, f.status, f.created_time
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
		SELECT f.id, f.user_id, f.user_name, f.name, f.tunnel_id, t.name, f.remote_addr, f.strategy,
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
		SELECT DISTINCT t.id, t.name
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
		SELECT id, inx, name, type, flow, traffic_ratio, status, created_time, in_ip
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
		if err := rows.Scan(&id, &inx, &name, &typ, &flow, &trafficRatio, &status, &createdTime, &inIP); err != nil {
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
		SELECT tunnel_id, chain_type, node_id, protocol, strategy, COALESCE(inx, 0)
		FROM chain_tunnel
		ORDER BY tunnel_id ASC, chain_type ASC, inx ASC, id ASC
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

func resolveForwardIngress(db *sql.DB, forwardID int64, tunnelID int64) (string, sql.NullInt64, error) {
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

func bootstrapSchema(db *sql.DB) error {
	if db == nil {
		return errors.New("nil db")
	}

	if _, err := db.Exec(embeddedSchema); err != nil {
		return fmt.Errorf("apply schema.sql: %w", err)
	}

	if _, err := db.Exec(embeddedSeedData); err != nil {
		return fmt.Errorf("apply data.sql: %w", err)
	}
	return nil
}

func ensurePeerSchema(db *sql.DB) error {
	if db == nil {
		return errors.New("nil db")
	}

	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS peer_share (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		node_id INTEGER NOT NULL,
		token TEXT NOT NULL UNIQUE,
		max_bandwidth INTEGER DEFAULT 0,
		expiry_time INTEGER DEFAULT 0,
		port_range_start INTEGER DEFAULT 0,
		port_range_end INTEGER DEFAULT 0,
		current_flow INTEGER DEFAULT 0,
		is_active INTEGER DEFAULT 1,
		created_time INTEGER NOT NULL,
		updated_time INTEGER NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create peer_share: %w", err)
	}

	columns := map[string]string{
		"is_remote":     "INTEGER DEFAULT 0",
		"remote_url":    "TEXT",
		"remote_token":  "TEXT",
		"remote_config": "TEXT",
	}

	for col, typ := range columns {
		var dummy interface{}
		err := db.QueryRow(fmt.Sprintf("SELECT %s FROM node LIMIT 1", col)).Scan(&dummy)
		if err != nil {
			if strings.Contains(err.Error(), "no such column") {
				_, err = db.Exec(fmt.Sprintf("ALTER TABLE node ADD COLUMN %s %s", col, typ))
				if err != nil {
					log.Printf("failed to add column %s: %v", col, err)
				}
			}
		}
	}
	return nil
}

func (r *Repository) CreatePeerShare(share *PeerShare) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	_, err := r.db.Exec(`
		INSERT INTO peer_share(name, node_id, token, max_bandwidth, expiry_time, port_range_start, port_range_end, current_flow, is_active, created_time, updated_time)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, share.Name, share.NodeID, share.Token, share.MaxBandwidth, share.ExpiryTime, share.PortRangeStart, share.PortRangeEnd, share.CurrentFlow, share.IsActive, share.CreatedTime, share.UpdatedTime)
	return err
}

func (r *Repository) UpdatePeerShare(share *PeerShare) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	_, err := r.db.Exec(`
		UPDATE peer_share SET name=?, max_bandwidth=?, expiry_time=?, port_range_start=?, port_range_end=?, is_active=?, updated_time=?
		WHERE id=?
	`, share.Name, share.MaxBandwidth, share.ExpiryTime, share.PortRangeStart, share.PortRangeEnd, share.IsActive, share.UpdatedTime, share.ID)
	return err
}

func (r *Repository) DeletePeerShare(id int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	_, err := r.db.Exec(`DELETE FROM peer_share WHERE id=?`, id)
	return err
}

func (r *Repository) GetPeerShare(id int64) (*PeerShare, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	row := r.db.QueryRow(`SELECT id, name, node_id, token, max_bandwidth, expiry_time, port_range_start, port_range_end, current_flow, is_active, created_time, updated_time FROM peer_share WHERE id = ?`, id)
	var s PeerShare
	if err := row.Scan(&s.ID, &s.Name, &s.NodeID, &s.Token, &s.MaxBandwidth, &s.ExpiryTime, &s.PortRangeStart, &s.PortRangeEnd, &s.CurrentFlow, &s.IsActive, &s.CreatedTime, &s.UpdatedTime); err != nil {
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
	row := r.db.QueryRow(`SELECT id, name, node_id, token, max_bandwidth, expiry_time, port_range_start, port_range_end, current_flow, is_active, created_time, updated_time FROM peer_share WHERE token = ?`, token)
	var s PeerShare
	if err := row.Scan(&s.ID, &s.Name, &s.NodeID, &s.Token, &s.MaxBandwidth, &s.ExpiryTime, &s.PortRangeStart, &s.PortRangeEnd, &s.CurrentFlow, &s.IsActive, &s.CreatedTime, &s.UpdatedTime); err != nil {
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
	rows, err := r.db.Query(`SELECT id, name, node_id, token, max_bandwidth, expiry_time, port_range_start, port_range_end, current_flow, is_active, created_time, updated_time FROM peer_share ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shares []PeerShare
	for rows.Next() {
		var s PeerShare
		if err := rows.Scan(&s.ID, &s.Name, &s.NodeID, &s.Token, &s.MaxBandwidth, &s.ExpiryTime, &s.PortRangeStart, &s.PortRangeEnd, &s.CurrentFlow, &s.IsActive, &s.CreatedTime, &s.UpdatedTime); err != nil {
			return nil, err
		}
		shares = append(shares, s)
	}
	return shares, nil
}

var osMkdirAll = func(path string) error {
	return os.MkdirAll(path, 0o755)
}
