package repo

import (
	"database/sql"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"go-backend/internal/store/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Repository) UserExists(username string) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("repository not initialized")
	}
	var cnt int64
	err := r.db.Model(&model.User{}).Where(`"user" = ?`, username).Count(&cnt).Error
	return cnt > 0, err
}

func (r *Repository) UserExistsExcluding(username string, excludeID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("repository not initialized")
	}
	var cnt int64
	err := r.db.Model(&model.User{}).
		Where(`"user" = ? AND id != ?`, username, excludeID).
		Count(&cnt).Error
	return cnt > 0, err
}

func (r *Repository) CreateUser(username, pwdHash string, roleID int, expTime, flow, flowResetTime int64, num, status int, now int64) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("repository not initialized")
	}
	user := model.User{
		User:          username,
		Pwd:           pwdHash,
		RoleID:        roleID,
		ExpTime:       expTime,
		Flow:          flow,
		InFlow:        0,
		OutFlow:       0,
		FlowResetTime: flowResetTime,
		Num:           num,
		CreatedTime:   now,
		UpdatedTime:   sql.NullInt64{Int64: now, Valid: true},
		Status:        status,
	}
	if err := r.db.Create(&user).Error; err != nil {
		return 0, err
	}
	return user.ID, nil
}

func (r *Repository) GetUserRoleID(userID int64) (int, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("repository not initialized")
	}
	var user model.User
	err := r.db.Select("role_id").Where("id = ?", userID).First(&user).Error
	if err != nil {
		return 0, normalizeNotFoundErr(err)
	}
	return user.RoleID, nil
}

func (r *Repository) UpdateUserWithPassword(id int64, username, pwdHash string, flow int64, num int, expTime, flowResetTime int64, status int, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.User{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"user":            username,
			"pwd":             pwdHash,
			"flow":            flow,
			"num":             num,
			"exp_time":        expTime,
			"flow_reset_time": flowResetTime,
			"status":          status,
			"updated_time":    sql.NullInt64{Int64: now, Valid: true},
		}).Error
}

func (r *Repository) UpdateUserWithoutPassword(id int64, username string, flow int64, num int, expTime, flowResetTime int64, status int, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.User{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"user":            username,
			"flow":            flow,
			"num":             num,
			"exp_time":        expTime,
			"flow_reset_time": flowResetTime,
			"status":          status,
			"updated_time":    sql.NullInt64{Int64: now, Valid: true},
		}).Error
}

func (r *Repository) PropagateUserFlowToTunnels(userID int64, flow int64, num int, expTime, flowResetTime int64) {
	if r == nil || r.db == nil {
		return
	}
	_ = r.db.Model(&model.UserTunnel{}).
		Where("user_id = ?", userID).
		Updates(map[string]interface{}{
			"flow":            flow,
			"num":             num,
			"exp_time":        expTime,
			"flow_reset_time": flowResetTime,
		}).Error
}

func (r *Repository) DeleteUserCascade(userID int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		forwardIDs := tx.Model(&model.Forward{}).Select("id").Where("user_id = ?", userID)
		if err := tx.Where("forward_id IN (?)", forwardIDs).Delete(&model.ForwardPort{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&model.Forward{}).Error; err != nil {
			return err
		}
		userTunnelIDs := tx.Model(&model.UserTunnel{}).Select("id").Where("user_id = ?", userID)
		if err := tx.Where("user_tunnel_id IN (?)", userTunnelIDs).Delete(&model.GroupPermissionGrant{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&model.UserTunnel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&model.UserGroupUser{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&model.StatisticsFlow{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", userID).Delete(&model.User{}).Error
	})
}

func (r *Repository) ResetUserFlowByUser(userID int64, now int64) {
	if r == nil || r.db == nil {
		return
	}
	_ = r.db.Model(&model.User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"in_flow":      0,
			"out_flow":     0,
			"updated_time": sql.NullInt64{Int64: now, Valid: true},
		}).Error
	_ = r.db.Model(&model.UserTunnel{}).
		Where("user_id = ?", userID).
		Updates(map[string]interface{}{"in_flow": 0, "out_flow": 0}).Error
}

func (r *Repository) ResetUserFlowByUserTunnel(userTunnelID int64) {
	if r == nil || r.db == nil {
		return
	}
	_ = r.db.Model(&model.UserTunnel{}).
		Where("id = ?", userTunnelID).
		Updates(map[string]interface{}{"in_flow": 0, "out_flow": 0}).Error
}

func (r *Repository) GetUsernameByID(userID int64) string {
	if r == nil || r.db == nil {
		return ""
	}
	var user model.User
	if err := r.db.Select("user").Where("id = ?", userID).First(&user).Error; err != nil {
		return ""
	}
	return user.User
}

func (r *Repository) GetUserDefaultsForTunnel(userID int64) (flow int64, num int, expTime int64, flowReset int64, err error) {
	if r == nil || r.db == nil {
		return 0, 0, 0, 0, errors.New("repository not initialized")
	}
	var user model.User
	err = r.db.Select("flow", "num", "exp_time", "flow_reset_time").Where("id = ?", userID).First(&user).Error
	if err != nil {
		return 0, 0, 0, 0, normalizeNotFoundErr(err)
	}
	return user.Flow, user.Num, user.ExpTime, user.FlowResetTime, nil
}

func (r *Repository) CreateNode(name, secret, serverIP string, serverIPV4, serverIPV6, port, interfaceName, version interface{}, httpFlag, tlsFlag, socksFlag int, now int64, status int, tcpAddr, udpAddr string, inx, isRemote int, remoteURL, remoteToken, remoteConfig interface{}) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	node := model.Node{
		Name:          name,
		Secret:        secret,
		ServerIP:      serverIP,
		ServerIPV4:    nullStringFromInterface(serverIPV4),
		ServerIPV6:    nullStringFromInterface(serverIPV6),
		Port:          stringFromInterface(port),
		InterfaceName: nullStringFromInterface(interfaceName),
		Version:       nullStringFromInterface(version),
		HTTP:          httpFlag,
		TLS:           tlsFlag,
		Socks:         socksFlag,
		CreatedTime:   now,
		UpdatedTime:   sql.NullInt64{Int64: now, Valid: true},
		Status:        status,
		TCPListenAddr: tcpAddr,
		UDPListenAddr: udpAddr,
		Inx:           inx,
		IsRemote:      isRemote,
		RemoteURL:     nullStringFromInterface(remoteURL),
		RemoteToken:   nullStringFromInterface(remoteToken),
		RemoteConfig:  nullStringFromInterface(remoteConfig),
	}
	return r.db.Create(&node).Error
}

func (r *Repository) GetNodeStatusFields(nodeID int64) (status, httpFlag, tlsFlag, socksFlag int, err error) {
	if r == nil || r.db == nil {
		return 0, 0, 0, 0, errors.New("repository not initialized")
	}
	var node model.Node
	err = r.db.Select("status", "http", "tls", "socks").Where("id = ?", nodeID).First(&node).Error
	if err != nil {
		return 0, 0, 0, 0, normalizeNotFoundErr(err)
	}
	return node.Status, node.HTTP, node.TLS, node.Socks, nil
}

func (r *Repository) UpdateNode(id int64, name, serverIP string, serverIPV4, serverIPV6, port, interfaceName interface{}, httpFlag, tlsFlag, socksFlag int, tcpAddr, udpAddr string, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.Node{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"name":            name,
			"server_ip":       serverIP,
			"server_ip_v4":    nullStringFromInterface(serverIPV4),
			"server_ip_v6":    nullStringFromInterface(serverIPV6),
			"port":            stringFromInterface(port),
			"interface_name":  nullStringFromInterface(interfaceName),
			"http":            httpFlag,
			"tls":             tlsFlag,
			"socks":           socksFlag,
			"tcp_listen_addr": tcpAddr,
			"udp_listen_addr": udpAddr,
			"updated_time":    sql.NullInt64{Int64: now, Valid: true},
		}).Error
}

func (r *Repository) GetNodeSecret(nodeID int64) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("repository not initialized")
	}
	var node model.Node
	err := r.db.Select("secret").Where("id = ?", nodeID).First(&node).Error
	if err != nil {
		return "", normalizeNotFoundErr(err)
	}
	return node.Secret, nil
}

func (r *Repository) GetViteConfigValue(name string) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("repository not initialized")
	}
	var cfg model.ViteConfig
	err := r.db.Select("value").Where("name = ?", name).First(&cfg).Error
	if err != nil {
		return "", normalizeNotFoundErr(err)
	}
	return cfg.Value, nil
}

func (r *Repository) UpdateNodeOrder(nodeID int64, inx int, now int64) {
	if r == nil || r.db == nil {
		return
	}
	_ = r.db.Model(&model.Node{}).
		Where("id = ?", nodeID).
		Updates(map[string]interface{}{
			"inx":          inx,
			"updated_time": sql.NullInt64{Int64: now, Valid: true},
		}).Error
}

func (r *Repository) DeleteNodeCascade(nodeID int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("node_id = ?", nodeID).Delete(&model.ForwardPort{}).Error; err != nil {
			return err
		}
		if err := tx.Where("node_id = ?", nodeID).Delete(&model.ChainTunnel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("node_id = ?", nodeID).Delete(&model.FederationTunnelBinding{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", nodeID).Delete(&model.Node{}).Error
	})
}

func (r *Repository) GetNodeRemoteFields(nodeID int64) (isRemote int, remoteURL, remoteToken sql.NullString, err error) {
	if r == nil || r.db == nil {
		return 0, sql.NullString{}, sql.NullString{}, errors.New("repository not initialized")
	}
	return r.GetNodeRemoteFieldsTx(r.db, nodeID)
}

func (r *Repository) GetNodeRemoteFieldsTx(tx *gorm.DB, nodeID int64) (isRemote int, remoteURL, remoteToken sql.NullString, err error) {
	if tx == nil {
		return 0, sql.NullString{}, sql.NullString{}, errors.New("database unavailable")
	}
	var node model.Node
	err = tx.Select("is_remote", "remote_url", "remote_token").Where("id = ?", nodeID).First(&node).Error
	if err != nil {
		return 0, sql.NullString{}, sql.NullString{}, normalizeNotFoundErr(err)
	}
	return node.IsRemote, node.RemoteURL, node.RemoteToken, nil
}

func (r *Repository) GetNodePortRange(nodeID int64) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("repository not initialized")
	}
	var node model.Node
	err := r.db.Select("port").Where("id = ?", nodeID).First(&node).Error
	if err != nil {
		return "", normalizeNotFoundErr(err)
	}
	return node.Port, nil
}

func (r *Repository) TunnelNameExists(name string) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("repository not initialized")
	}
	var cnt int64
	err := r.db.Model(&model.Tunnel{}).Where("name = ?", name).Count(&cnt).Error
	return cnt > 0, err
}

func (r *Repository) BeginTx() *gorm.DB {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Begin()
}

func (r *Repository) UpdateTunnelOrder(tunnelID int64, inx int, now int64) {
	if r == nil || r.db == nil {
		return
	}
	_ = r.db.Model(&model.Tunnel{}).
		Where("id = ?", tunnelID).
		Updates(map[string]interface{}{"inx": inx, "updated_time": now}).Error
}

func (r *Repository) UpdateTunnelTx(tx *gorm.DB, tunnelID int64, name string, typeVal int, flow int64, trafficRatio float64, status int, inIP, ipPreference string, now int64) error {
	if tx == nil {
		return errors.New("database unavailable")
	}
	return tx.Model(&model.Tunnel{}).
		Where("id = ?", tunnelID).
		Updates(map[string]interface{}{
			"name":          name,
			"type":          typeVal,
			"flow":          flow,
			"traffic_ratio": trafficRatio,
			"status":        status,
			"in_ip":         nullStringFromInterface(inIP),
			"ip_preference": ipPreference,
			"updated_time":  now,
		}).Error
}

func (r *Repository) DeleteChainTunnelsByTunnelTx(tx *gorm.DB, tunnelID int64) error {
	if tx == nil {
		return errors.New("database unavailable")
	}
	return tx.Where("tunnel_id = ?", tunnelID).Delete(&model.ChainTunnel{}).Error
}

func (r *Repository) CreateChainTunnelTx(tx *gorm.DB, tunnelID int64, chainType string, nodeID int64, port sql.NullInt64, strategy string, inx int, protocol string) error {
	if tx == nil {
		return errors.New("database unavailable")
	}
	ct := model.ChainTunnel{
		TunnelID:  tunnelID,
		ChainType: chainType,
		NodeID:    nodeID,
		Port:      port,
		Strategy:  nullStringFromInterface(strategy),
		Inx:       nullInt64FromInterface(inx),
		Protocol:  nullStringFromInterface(protocol),
	}
	return tx.Create(&ct).Error
}

func (r *Repository) IsRemoteNodeTx(tx *gorm.DB, nodeID int64) (bool, error) {
	if tx == nil {
		return false, errors.New("database unavailable")
	}
	if nodeID <= 0 {
		return false, errors.New("节点不存在")
	}
	var node model.Node
	err := tx.Select("is_remote").Where("id = ?", nodeID).First(&node).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, errors.New("节点不存在")
	}
	if err != nil {
		return false, err
	}
	return node.IsRemote == 1, nil
}

func (r *Repository) PickNodePortTx(tx *gorm.DB, nodeID int64, allocated map[int64]int, excludeTunnelID int64) (int, error) {
	if tx == nil {
		return 0, errors.New("database unavailable")
	}
	if nodeID <= 0 {
		return 0, errors.New("节点不存在")
	}
	if port, ok := allocated[nodeID]; ok && port > 0 {
		return port, nil
	}

	var node model.Node
	err := tx.Select("port").Where("id = ?", nodeID).First(&node).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, errors.New("节点不存在")
	}
	if err != nil {
		return 0, err
	}

	candidates := parsePortRangeSpec(node.Port)
	if len(candidates) == 0 {
		return 0, errors.New("节点端口已满，无可用端口")
	}

	used := make(map[int]struct{})

	chainQuery := tx.Model(&model.ChainTunnel{}).Where("node_id = ? AND port > 0", nodeID)
	if excludeTunnelID > 0 {
		chainQuery = chainQuery.Where("tunnel_id != ?", excludeTunnelID)
	}
	var chainPorts []int
	if err := chainQuery.Pluck("port", &chainPorts).Error; err != nil {
		return 0, err
	}
	for _, p := range chainPorts {
		if p > 0 {
			used[p] = struct{}{}
		}
	}

	var forwardPorts []int
	if err := tx.Model(&model.ForwardPort{}).
		Where("node_id = ? AND port > 0", nodeID).
		Pluck("port", &forwardPorts).Error; err != nil {
		return 0, err
	}
	for _, p := range forwardPorts {
		if p > 0 {
			used[p] = struct{}{}
		}
	}

	for _, candidate := range candidates {
		if candidate <= 0 {
			continue
		}
		if _, ok := used[candidate]; ok {
			continue
		}
		allocated[nodeID] = candidate
		return candidate, nil
	}

	return 0, errors.New("节点端口已满，无可用端口")
}

func (r *Repository) GetTunnelIPPreference(tunnelID int64) string {
	if r == nil || r.db == nil {
		return ""
	}
	var tunnel model.Tunnel
	if err := r.db.Select("ip_preference").Where("id = ?", tunnelID).First(&tunnel).Error; err != nil {
		return ""
	}
	return tunnel.IPPreference
}

func (r *Repository) DeleteTunnelCascade(tunnelID int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		forwardIDs := tx.Model(&model.Forward{}).Select("id").Where("tunnel_id = ?", tunnelID)
		if err := tx.Where("forward_id IN (?)", forwardIDs).Delete(&model.ForwardPort{}).Error; err != nil {
			return err
		}
		if err := tx.Where("tunnel_id = ?", tunnelID).Delete(&model.Forward{}).Error; err != nil {
			return err
		}
		if err := tx.Where("tunnel_id = ?", tunnelID).Delete(&model.UserTunnel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("tunnel_id = ?", tunnelID).Delete(&model.SpeedLimit{}).Error; err != nil {
			return err
		}
		if err := tx.Where("tunnel_id = ?", tunnelID).Delete(&model.ChainTunnel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("tunnel_id = ?", tunnelID).Delete(&model.FederationTunnelBinding{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", tunnelID).Delete(&model.Tunnel{}).Error
	})
}

func (r *Repository) GetTunnelNameByID(tunnelID int64) string {
	if r == nil || r.db == nil {
		return ""
	}
	var tunnel model.Tunnel
	if err := r.db.Select("name").Where("id = ?", tunnelID).First(&tunnel).Error; err != nil {
		return ""
	}
	return tunnel.Name
}

func (r *Repository) TunnelEntryNodeIDs(tunnelID int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var ids []int64
	err := r.db.Model(&model.ChainTunnel{}).
		Where("tunnel_id = ? AND chain_type = ?", tunnelID, "1").
		Order("inx ASC, id ASC").
		Pluck("node_id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *Repository) DeleteUserTunnel(id int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Where("id = ?", id).Delete(&model.UserTunnel{}).Error
}

func (r *Repository) UpdateUserTunnel(id int64, flow int64, num int, expTime, flowResetTime int64, speedID interface{}, status int) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.UserTunnel{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"flow":            flow,
			"num":             num,
			"exp_time":        expTime,
			"flow_reset_time": flowResetTime,
			"speed_id":        nullInt64FromInterface(speedID),
			"status":          status,
		}).Error
}

func (r *Repository) GetUserTunnelUserAndTunnel(id int64) (userID, tunnelID int64, err error) {
	if r == nil || r.db == nil {
		return 0, 0, errors.New("repository not initialized")
	}
	var ut model.UserTunnel
	err = r.db.Select("user_id", "tunnel_id").Where("id = ?", id).First(&ut).Error
	if err != nil {
		return 0, 0, normalizeNotFoundErr(err)
	}
	return ut.UserID, ut.TunnelID, nil
}

func (r *Repository) GetExistingUserTunnel(userID, tunnelID int64) (id int64, flow, num, expTime, flowReset int64, speedID sql.NullInt64, status int, err error) {
	if r == nil || r.db == nil {
		return 0, 0, 0, 0, 0, sql.NullInt64{}, 0, errors.New("repository not initialized")
	}
	var ut model.UserTunnel
	err = r.db.Select("id", "flow", "num", "exp_time", "flow_reset_time", "speed_id", "status").
		Where("user_id = ? AND tunnel_id = ?", userID, tunnelID).
		First(&ut).Error
	if err != nil {
		return 0, 0, 0, 0, 0, sql.NullInt64{}, 0, normalizeNotFoundErr(err)
	}
	return ut.ID, ut.Flow, int64(ut.Num), ut.ExpTime, ut.FlowResetTime, ut.SpeedID, ut.Status, nil
}

func (r *Repository) InsertUserTunnel(userID, tunnelID int64, speedID interface{}, num int, flow, flowResetTime, expTime int64, status int) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	ut := model.UserTunnel{
		UserID:        userID,
		TunnelID:      tunnelID,
		SpeedID:       nullInt64FromInterface(speedID),
		Num:           num,
		Flow:          flow,
		InFlow:        0,
		OutFlow:       0,
		FlowResetTime: flowResetTime,
		ExpTime:       expTime,
		Status:        status,
	}
	return r.db.Create(&ut).Error
}

func (r *Repository) UpdateUserTunnelFields(id int64, speedID interface{}, flow int64, num int, expTime, flowResetTime int64, status int) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.UserTunnel{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"speed_id":        nullInt64FromInterface(speedID),
			"flow":            flow,
			"num":             num,
			"exp_time":        expTime,
			"flow_reset_time": flowResetTime,
			"status":          status,
		}).Error
}

func (r *Repository) GetMinForwardPort(forwardID int64) sql.NullInt64 {
	if r == nil || r.db == nil {
		return sql.NullInt64{}
	}
	var p sql.NullInt64
	_ = r.db.Model(&model.ForwardPort{}).
		Select("MIN(port)").
		Where("forward_id = ?", forwardID).
		Scan(&p).Error
	return p
}

func (r *Repository) UpdateForward(id int64, name string, tunnelID int64, remoteAddr, strategy string, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.Forward{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"name":         name,
			"tunnel_id":    tunnelID,
			"remote_addr":  remoteAddr,
			"strategy":     strategy,
			"updated_time": now,
		}).Error
}

func (r *Repository) UpdateForwardOrder(forwardID int64, inx int, now int64) {
	if r == nil || r.db == nil {
		return
	}
	_ = r.db.Model(&model.Forward{}).
		Where("id = ?", forwardID).
		Updates(map[string]interface{}{"inx": inx, "updated_time": now}).Error
}

func (r *Repository) UpdateForwardTunnel(forwardID, tunnelID int64, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.Forward{}).
		Where("id = ?", forwardID).
		Updates(map[string]interface{}{"tunnel_id": tunnelID, "updated_time": now}).Error
}

func (r *Repository) DeleteForwardCascade(forwardID int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("forward_id = ?", forwardID).Delete(&model.ForwardPort{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", forwardID).Delete(&model.Forward{}).Error
	})
}

func (r *Repository) ReplaceForwardPorts(forwardID int64, entries []struct {
	NodeID int64
	Port   int
}) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("forward_id = ?", forwardID).Delete(&model.ForwardPort{}).Error; err != nil {
			return err
		}
		if len(entries) == 0 {
			return nil
		}
		rows := make([]model.ForwardPort, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, model.ForwardPort{ForwardID: forwardID, NodeID: e.NodeID, Port: e.Port})
		}
		return tx.Create(&rows).Error
	})
}

func (r *Repository) RollbackForwardFields(id, userID int64, userName, name string, tunnelID int64, remoteAddr, strategy string, status int, now int64) {
	if r == nil || r.db == nil {
		return
	}
	_ = r.db.Model(&model.Forward{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"user_id":      userID,
			"user_name":    userName,
			"name":         name,
			"tunnel_id":    tunnelID,
			"remote_addr":  remoteAddr,
			"strategy":     strategy,
			"status":       status,
			"updated_time": now,
		}).Error
}

func (r *Repository) GetUsedPortsOnNodeAsMap(nodeID int64) (map[int]bool, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	used := make(map[int]bool)
	var forwardPorts []int
	if err := r.db.Model(&model.ForwardPort{}).Where("node_id = ?", nodeID).Pluck("port", &forwardPorts).Error; err != nil {
		return nil, err
	}
	for _, p := range forwardPorts {
		used[p] = true
	}
	var chainPorts []int
	if err := r.db.Model(&model.ChainTunnel{}).Where("node_id = ? AND port > 0", nodeID).Pluck("port", &chainPorts).Error; err != nil {
		return nil, err
	}
	for _, p := range chainPorts {
		used[p] = true
	}
	return used, nil
}

func (r *Repository) CreateSpeedLimit(name string, speed int, tunnelID int64, tunnelName string, now int64, status int) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("repository not initialized")
	}
	sl := model.SpeedLimit{
		Name:        name,
		Speed:       speed,
		TunnelID:    tunnelID,
		TunnelName:  tunnelName,
		CreatedTime: now,
		UpdatedTime: sql.NullInt64{Int64: now, Valid: true},
		Status:      status,
	}
	if err := r.db.Create(&sl).Error; err != nil {
		return 0, err
	}
	return sl.ID, nil
}

func (r *Repository) UpdateSpeedLimit(id int64, name string, speed int, tunnelID int64, tunnelName string, status int, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.SpeedLimit{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"name":        name,
			"speed":       speed,
			"tunnel_id":   tunnelID,
			"tunnel_name": tunnelName,
			"status":      status,
			"updated_time": sql.NullInt64{
				Int64: now,
				Valid: true,
			},
		}).Error
}

func (r *Repository) GetSpeedLimitTunnelID(speedLimitID int64) int64 {
	if r == nil || r.db == nil {
		return 0
	}
	var sl model.SpeedLimit
	if err := r.db.Select("tunnel_id").Where("id = ?", speedLimitID).First(&sl).Error; err != nil {
		return 0
	}
	return sl.TunnelID
}

func (r *Repository) DeleteSpeedLimit(id int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Where("id = ?", id).Delete(&model.SpeedLimit{}).Error
}

func (r *Repository) GroupCreate(table, name string, status int, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	switch table {
	case "tunnel_group":
		return r.db.Create(&model.TunnelGroup{
			Name:        name,
			CreatedTime: now,
			UpdatedTime: now,
			Status:      status,
		}).Error
	case "user_group":
		return r.db.Create(&model.UserGroup{
			Name:        name,
			CreatedTime: now,
			UpdatedTime: now,
			Status:      status,
		}).Error
	default:
		return errors.New("invalid group table")
	}
}

func (r *Repository) GroupUpdate(table string, id int64, name string, status int, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	updates := map[string]interface{}{"name": name, "status": status, "updated_time": now}
	switch table {
	case "tunnel_group":
		return r.db.Model(&model.TunnelGroup{}).Where("id = ?", id).Updates(updates).Error
	case "user_group":
		return r.db.Model(&model.UserGroup{}).Where("id = ?", id).Updates(updates).Error
	default:
		return errors.New("invalid group table")
	}
}

func (r *Repository) GroupDeleteCascade(table string, id int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		switch table {
		case "tunnel_group":
			if err := tx.Where("tunnel_group_id = ?", id).Delete(&model.TunnelGroupTunnel{}).Error; err != nil {
				return err
			}
			if err := tx.Where("tunnel_group_id = ?", id).Delete(&model.GroupPermission{}).Error; err != nil {
				return err
			}
			if err := tx.Where("tunnel_group_id = ?", id).Delete(&model.GroupPermissionGrant{}).Error; err != nil {
				return err
			}
			return tx.Where("id = ?", id).Delete(&model.TunnelGroup{}).Error
		case "user_group":
			if err := tx.Where("user_group_id = ?", id).Delete(&model.UserGroupUser{}).Error; err != nil {
				return err
			}
			if err := tx.Where("user_group_id = ?", id).Delete(&model.GroupPermission{}).Error; err != nil {
				return err
			}
			if err := tx.Where("user_group_id = ?", id).Delete(&model.GroupPermissionGrant{}).Error; err != nil {
				return err
			}
			return tx.Where("id = ?", id).Delete(&model.UserGroup{}).Error
		default:
			return errors.New("invalid group table")
		}
	})
}

func (r *Repository) ListUserIDsByUserGroupTx(tx *gorm.DB, userGroupID int64) ([]int64, error) {
	if tx == nil {
		return nil, errors.New("database unavailable")
	}
	var ids []int64
	err := tx.Model(&model.UserGroupUser{}).
		Where("user_group_id = ?", userGroupID).
		Pluck("user_id", &ids).Error
	if err != nil {
		return nil, err
	}
	if ids == nil {
		ids = make([]int64, 0)
	}
	return ids, nil
}

func (r *Repository) ReplaceTunnelGroupMembersTx(tx *gorm.DB, groupID int64, tunnelIDs []int64, now int64) error {
	if tx == nil {
		return errors.New("database unavailable")
	}
	if err := tx.Where("tunnel_group_id = ?", groupID).Delete(&model.TunnelGroupTunnel{}).Error; err != nil {
		return err
	}
	if len(tunnelIDs) == 0 {
		return nil
	}
	rows := make([]model.TunnelGroupTunnel, 0, len(tunnelIDs))
	for _, tunnelID := range tunnelIDs {
		if tunnelID <= 0 {
			continue
		}
		rows = append(rows, model.TunnelGroupTunnel{TunnelGroupID: groupID, TunnelID: tunnelID, CreatedTime: now})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func (r *Repository) ReplaceUserGroupMembersTx(tx *gorm.DB, groupID int64, userIDs []int64, now int64) error {
	if tx == nil {
		return errors.New("database unavailable")
	}
	if err := tx.Where("user_group_id = ?", groupID).Delete(&model.UserGroupUser{}).Error; err != nil {
		return err
	}
	if len(userIDs) == 0 {
		return nil
	}
	rows := make([]model.UserGroupUser, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		rows = append(rows, model.UserGroupUser{UserGroupID: groupID, UserID: userID, CreatedTime: now})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func (r *Repository) GetGroupPermissionPairByIDTx(tx *gorm.DB, id int64) (userGroupID int64, tunnelGroupID int64, exists bool, err error) {
	if tx == nil {
		return 0, 0, false, errors.New("database unavailable")
	}
	var gp model.GroupPermission
	err = tx.Select("user_group_id", "tunnel_group_id").Where("id = ?", id).First(&gp).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	return gp.UserGroupID, gp.TunnelGroupID, true, nil
}

func (r *Repository) DeleteGroupPermissionByIDTx(tx *gorm.DB, id int64) error {
	if tx == nil {
		return errors.New("database unavailable")
	}
	return tx.Where("id = ?", id).Delete(&model.GroupPermission{}).Error
}

func (r *Repository) RevokeGroupGrantsForRemovedUsersTx(tx *gorm.DB, userGroupID int64, previousUserIDs, currentUserIDs []int64) error {
	if tx == nil {
		return errors.New("database unavailable")
	}
	currentSet := make(map[int64]struct{}, len(currentUserIDs))
	for _, uid := range currentUserIDs {
		if uid > 0 {
			currentSet[uid] = struct{}{}
		}
	}

	removedUserIDs := make([]int64, 0)
	for _, uid := range previousUserIDs {
		if uid <= 0 {
			continue
		}
		if _, ok := currentSet[uid]; !ok {
			removedUserIDs = append(removedUserIDs, uid)
		}
	}
	if len(removedUserIDs) == 0 {
		return nil
	}

	type grantRow struct {
		UserTunnelID   int64
		CreatedByGroup int
	}

	for _, userID := range removedUserIDs {
		var rows []grantRow
		if err := tx.Model(&model.GroupPermissionGrant{}).
			Select("group_permission_grant.user_tunnel_id, group_permission_grant.created_by_group").
			Joins("JOIN user_tunnel ON user_tunnel.id = group_permission_grant.user_tunnel_id").
			Where("group_permission_grant.user_group_id = ? AND user_tunnel.user_id = ?", userGroupID, userID).
			Find(&rows).Error; err != nil {
			return err
		}

		groupCreatedTunnelIDs := make(map[int64]struct{})
		for _, row := range rows {
			if row.CreatedByGroup == 1 && row.UserTunnelID > 0 {
				groupCreatedTunnelIDs[row.UserTunnelID] = struct{}{}
			}
		}

		userTunnelIDs := tx.Model(&model.UserTunnel{}).Select("id").Where("user_id = ?", userID)
		if err := tx.Where("user_group_id = ? AND user_tunnel_id IN (?)", userGroupID, userTunnelIDs).
			Delete(&model.GroupPermissionGrant{}).Error; err != nil {
			return err
		}

		for userTunnelID := range groupCreatedTunnelIDs {
			var remaining int64
			if err := tx.Model(&model.GroupPermissionGrant{}).Where("user_tunnel_id = ?", userTunnelID).Count(&remaining).Error; err != nil {
				return err
			}
			if remaining == 0 {
				if err := tx.Where("id = ?", userTunnelID).Delete(&model.UserTunnel{}).Error; err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (r *Repository) RevokeGroupPermissionPairTx(tx *gorm.DB, userGroupID, tunnelGroupID int64) error {
	if tx == nil {
		return errors.New("database unavailable")
	}

	type grantRow struct {
		UserTunnelID   int64
		CreatedByGroup int
	}

	var rows []grantRow
	if err := tx.Model(&model.GroupPermissionGrant{}).
		Select("user_tunnel_id, created_by_group").
		Where("user_group_id = ? AND tunnel_group_id = ?", userGroupID, tunnelGroupID).
		Find(&rows).Error; err != nil {
		return err
	}

	groupCreatedTunnelIDs := make(map[int64]struct{})
	for _, row := range rows {
		if row.CreatedByGroup == 1 && row.UserTunnelID > 0 {
			groupCreatedTunnelIDs[row.UserTunnelID] = struct{}{}
		}
	}

	if err := tx.Where("user_group_id = ? AND tunnel_group_id = ?", userGroupID, tunnelGroupID).
		Delete(&model.GroupPermissionGrant{}).Error; err != nil {
		return err
	}

	for userTunnelID := range groupCreatedTunnelIDs {
		var remaining int64
		if err := tx.Model(&model.GroupPermissionGrant{}).Where("user_tunnel_id = ?", userTunnelID).Count(&remaining).Error; err != nil {
			return err
		}
		if remaining == 0 {
			if err := tx.Where("id = ?", userTunnelID).Delete(&model.UserTunnel{}).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *Repository) ReplaceFederationTunnelBindingsTx(tx *gorm.DB, tunnelID int64, bindings []FederationTunnelBinding) error {
	if tx == nil {
		return errors.New("database unavailable")
	}
	if err := tx.Where("tunnel_id = ?", tunnelID).Delete(&model.FederationTunnelBinding{}).Error; err != nil {
		return err
	}
	if len(bindings) == 0 {
		return nil
	}

	rows := make([]model.FederationTunnelBinding, 0, len(bindings))
	now := time.Now().UnixMilli()
	for _, b := range bindings {
		created := b.CreatedTime
		if created <= 0 {
			created = now
		}
		updated := b.UpdatedTime
		if updated <= 0 {
			updated = created
		}
		rows = append(rows, model.FederationTunnelBinding{
			TunnelID:        tunnelID,
			NodeID:          b.NodeID,
			ChainType:       b.ChainType,
			HopInx:          b.HopInx,
			RemoteURL:       b.RemoteURL,
			ResourceKey:     b.ResourceKey,
			RemoteBindingID: b.RemoteBindingID,
			AllocatedPort:   b.AllocatedPort,
			Status:          b.Status,
			CreatedTime:     created,
			UpdatedTime:     updated,
		})
	}

	return tx.Create(&rows).Error
}

func (r *Repository) InsertGroupPermission(userGroupID, tunnelGroupID int64, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	gp := model.GroupPermission{UserGroupID: userGroupID, TunnelGroupID: tunnelGroupID, CreatedTime: now}
	return r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&gp).Error
}

func (r *Repository) InsertGroupPermissionGrant(userGroupID, tunnelGroupID, userTunnelID int64, createdByGroup int, now int64) {
	if r == nil || r.db == nil {
		return
	}
	g := model.GroupPermissionGrant{
		UserGroupID:    userGroupID,
		TunnelGroupID:  tunnelGroupID,
		UserTunnelID:   userTunnelID,
		CreatedByGroup: createdByGroup,
		CreatedTime:    now,
	}
	_ = r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&g).Error
}

func (r *Repository) EnsureUserTunnelGrant(userID, tunnelID int64) (int64, bool, error) {
	if r == nil || r.db == nil {
		return 0, false, errors.New("repository not initialized")
	}
	var existing model.UserTunnel
	err := r.db.Select("id").Where("user_id = ? AND tunnel_id = ?", userID, tunnelID).First(&existing).Error
	if err == nil {
		return existing.ID, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, err
	}
	var user model.User
	if err := r.db.Select("flow, num, exp_time, flow_reset_time").Where("id = ?", userID).First(&user).Error; err != nil {
		return 0, false, err
	}
	flow := user.Flow
	num := user.Num
	expTime := user.ExpTime
	flowReset := user.FlowResetTime
	ut := model.UserTunnel{
		UserID:        userID,
		TunnelID:      tunnelID,
		Num:           num,
		Flow:          flow,
		InFlow:        0,
		OutFlow:       0,
		FlowResetTime: flowReset,
		ExpTime:       expTime,
		Status:        1,
	}
	if err := r.db.Create(&ut).Error; err != nil {
		return 0, false, err
	}
	return ut.ID, true, nil
}

func (r *Repository) CreateForwardTx(userID int64, userName, name string, tunnelID int64, remoteAddr, strategy string, now int64, inx int, entryNodeIDs []int64, port int) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("repository not initialized")
	}
	var forwardID int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
		fwd := model.Forward{
			UserID:      userID,
			UserName:    userName,
			Name:        name,
			TunnelID:    tunnelID,
			RemoteAddr:  remoteAddr,
			Strategy:    strategy,
			InFlow:      0,
			OutFlow:     0,
			CreatedTime: now,
			UpdatedTime: now,
			Status:      1,
			Inx:         inx,
		}
		if err := tx.Create(&fwd).Error; err != nil {
			return err
		}
		forwardID = fwd.ID
		for _, nodeID := range entryNodeIDs {
			fp := model.ForwardPort{
				ForwardID: forwardID,
				NodeID:    nodeID,
				Port:      port,
			}
			if err := tx.Create(&fp).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return forwardID, err
}

func (r *Repository) BatchUpdateForwardStatus(ids []int64, status int) (int, int) {
	if r == nil || r.db == nil {
		return 0, len(ids)
	}
	s := 0
	f := 0
	now := time.Now().UnixMilli()
	for _, id := range ids {
		if err := r.db.Model(&model.Forward{}).Where("id = ?", id).Updates(map[string]interface{}{"status": status, "updated_time": now}).Error; err != nil {
			f++
		} else {
			s++
		}
	}
	return s, f
}

func (r *Repository) CreateTunnelTx(tx *gorm.DB, name string, trafficRatio float64, typeVal int, flow int64, now int64, status int, inIP interface{}, inx int, ipPreference string) (int64, error) {
	inIPVal := nullStringFromInterface(inIP)
	tunnel := model.Tunnel{
		Name:         name,
		TrafficRatio: trafficRatio,
		Type:         typeVal,
		Protocol:     "tls",
		Flow:         flow,
		CreatedTime:  now,
		UpdatedTime:  now,
		Status:       status,
		InIP:         inIPVal,
		Inx:          inx,
		IPPreference: ipPreference,
	}
	if err := tx.Create(&tunnel).Error; err != nil {
		return 0, err
	}
	return tunnel.ID, nil
}

func normalizeNotFoundErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return sql.ErrNoRows
	}
	return err
}

func nullStringFromInterface(v interface{}) sql.NullString {
	switch t := v.(type) {
	case nil:
		return sql.NullString{}
	case sql.NullString:
		return t
	case *sql.NullString:
		if t == nil {
			return sql.NullString{}
		}
		return *t
	case string:
		if t == "" {
			return sql.NullString{}
		}
		return sql.NullString{String: t, Valid: true}
	case *string:
		if t == nil || *t == "" {
			return sql.NullString{}
		}
		return sql.NullString{String: *t, Valid: true}
	default:
		return sql.NullString{}
	}
}

func nullInt64FromInterface(v interface{}) sql.NullInt64 {
	switch t := v.(type) {
	case nil:
		return sql.NullInt64{}
	case sql.NullInt64:
		return t
	case *sql.NullInt64:
		if t == nil {
			return sql.NullInt64{}
		}
		return *t
	case int64:
		return sql.NullInt64{Int64: t, Valid: true}
	case *int64:
		if t == nil {
			return sql.NullInt64{}
		}
		return sql.NullInt64{Int64: *t, Valid: true}
	case int:
		return sql.NullInt64{Int64: int64(t), Valid: true}
	case int32:
		return sql.NullInt64{Int64: int64(t), Valid: true}
	case int16:
		return sql.NullInt64{Int64: int64(t), Valid: true}
	case int8:
		return sql.NullInt64{Int64: int64(t), Valid: true}
	case uint64:
		return sql.NullInt64{Int64: int64(t), Valid: true}
	case uint:
		return sql.NullInt64{Int64: int64(t), Valid: true}
	case uint32:
		return sql.NullInt64{Int64: int64(t), Valid: true}
	case uint16:
		return sql.NullInt64{Int64: int64(t), Valid: true}
	case uint8:
		return sql.NullInt64{Int64: int64(t), Valid: true}
	case float64:
		return sql.NullInt64{Int64: int64(t), Valid: true}
	default:
		return sql.NullInt64{}
	}
}

func stringFromInterface(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case sql.NullString:
		if t.Valid {
			return t.String
		}
		return ""
	case *sql.NullString:
		if t != nil && t.Valid {
			return t.String
		}
		return ""
	case []byte:
		return string(t)
	default:
		return ""
	}
}

func parsePortRangeSpec(input string) []int {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	set := make(map[int]struct{})
	parts := strings.Split(input, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			r := strings.SplitN(part, "-", 2)
			if len(r) != 2 {
				continue
			}
			start, err1 := strconv.Atoi(strings.TrimSpace(r[0]))
			end, err2 := strconv.Atoi(strings.TrimSpace(r[1]))
			if err1 != nil || err2 != nil || start <= 0 || end <= 0 {
				continue
			}
			if end < start {
				start, end = end, start
			}
			for p := start; p <= end; p++ {
				set[p] = struct{}{}
			}
			continue
		}
		p, err := strconv.Atoi(part)
		if err != nil || p <= 0 {
			continue
		}
		set[p] = struct{}{}
	}
	out := make([]int, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}

func (r *Repository) AddUserToGroups(userID int64, groupIDs []int64, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if len(groupIDs) == 0 {
		return nil
	}
	rows := make([]model.UserGroupUser, 0, len(groupIDs))
	for _, gid := range groupIDs {
		if gid <= 0 {
			continue
		}
		rows = append(rows, model.UserGroupUser{UserGroupID: gid, UserID: userID, CreatedTime: now})
	}
	if len(rows) == 0 {
		return nil
	}
	return r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

func (r *Repository) ReplaceUserGroupsByUserID(userID int64, newGroupIDs []int64, now int64) (affectedGroupIDs []int64, err error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	var oldGroupIDs []int64
	if err = r.db.Model(&model.UserGroupUser{}).
		Where("user_id = ?", userID).
		Pluck("user_group_id", &oldGroupIDs).Error; err != nil {
		return nil, err
	}

	seen := make(map[int64]struct{})
	for _, id := range oldGroupIDs {
		seen[id] = struct{}{}
	}
	for _, id := range newGroupIDs {
		if id > 0 {
			seen[id] = struct{}{}
		}
	}
	for id := range seen {
		affectedGroupIDs = append(affectedGroupIDs, id)
	}

	if err = r.db.Where("user_id = ?", userID).Delete(&model.UserGroupUser{}).Error; err != nil {
		return nil, err
	}

	if len(newGroupIDs) == 0 {
		return affectedGroupIDs, nil
	}
	rows := make([]model.UserGroupUser, 0, len(newGroupIDs))
	for _, gid := range newGroupIDs {
		if gid <= 0 {
			continue
		}
		rows = append(rows, model.UserGroupUser{UserGroupID: gid, UserID: userID, CreatedTime: now})
	}
	if len(rows) > 0 {
		if err = r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
			return nil, err
		}
	}
	return affectedGroupIDs, nil
}
