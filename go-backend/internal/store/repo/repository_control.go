package repo

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"go-backend/internal/store/model"
)

func (r *Repository) UserTunnelExistsByUserAndTunnel(userID, tunnelID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("repository not initialized")
	}
	var count int64
	err := r.db.Model(&model.UserTunnel{}).
		Where("user_id = ? AND tunnel_id = ? AND status = 1", userID, tunnelID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repository) ListForwardsByTunnel(tunnelID int64) ([]model.ForwardRecord, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var forwards []model.Forward
	err := r.db.Where("tunnel_id = ?", tunnelID).Order("id ASC").Find(&forwards).Error
	if err != nil {
		return nil, err
	}
	rows := make([]model.ForwardRecord, 0, len(forwards))
	for _, f := range forwards {
		rows = append(rows, model.ForwardRecord{
			ID:         f.ID,
			UserID:     f.UserID,
			UserName:   f.UserName,
			Name:       f.Name,
			TunnelID:   f.TunnelID,
			RemoteAddr: f.RemoteAddr,
			Strategy:   f.Strategy,
			Status:     f.Status,
		})
	}
	for i := range rows {
		if strings.TrimSpace(rows[i].Strategy) == "" {
			rows[i].Strategy = "fifo"
		}
	}
	return rows, nil
}

func (r *Repository) ListActiveTunnelIDsByNode(nodeID int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var ids []int64
	err := r.db.Model(&model.ChainTunnel{}).
		Joins("JOIN tunnel ON tunnel.id = chain_tunnel.tunnel_id").
		Where("chain_tunnel.node_id = ? AND tunnel.status = 1", nodeID).
		Select("DISTINCT chain_tunnel.tunnel_id").
		Order("chain_tunnel.tunnel_id ASC").
		Pluck("chain_tunnel.tunnel_id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *Repository) ListActiveForwardIDsByNode(nodeID int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var ids []int64
	err := r.db.Model(&model.ForwardPort{}).
		Joins("JOIN forward ON forward.id = forward_port.forward_id").
		Where("forward_port.node_id = ? AND forward.status = 1", nodeID).
		Select("DISTINCT forward_port.forward_id").
		Order("forward_port.forward_id ASC").
		Pluck("forward_port.forward_id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *Repository) ListForwardPorts(forwardID int64) ([]model.ForwardPortRecord, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var ports []model.ForwardPort
	err := r.db.Where("forward_id = ?", forwardID).Order("id ASC").Find(&ports).Error
	if err != nil {
		return nil, err
	}
	rows := make([]model.ForwardPortRecord, 0, len(ports))
	for _, p := range ports {
		rows = append(rows, model.ForwardPortRecord{NodeID: p.NodeID, Port: p.Port})
	}
	return rows, nil
}

func (r *Repository) GetTunnelOutProtocol(tunnelID int64) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("repository not initialized")
	}
	var ct model.ChainTunnel
	err := r.db.Select("protocol").
		Where("tunnel_id = ? AND chain_type = ?", tunnelID, "3").
		Order("id ASC").
		Take(&ct).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	if ct.Protocol.Valid {
		return ct.Protocol.String, nil
	}
	return "", nil
}

func (r *Repository) GetNodeRecord(nodeID int64) (*model.NodeRecord, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var n model.Node
	err := r.db.Where("id = ?", nodeID).First(&n).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return nodeRecordFromModel(&n), nil
}

func (r *Repository) GetNodeRecordTx(tx *gorm.DB, nodeID int64) (*model.NodeRecord, error) {
	if tx == nil {
		return nil, errors.New("database unavailable")
	}
	var n model.Node
	err := tx.Where("id = ?", nodeID).First(&n).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return nodeRecordFromModel(&n), nil
}

func nodeRecordFromModel(n *model.Node) *model.NodeRecord {
	if n == nil {
		return nil
	}
	rec := &model.NodeRecord{
		ID:            n.ID,
		Name:          n.Name,
		ServerIP:      n.ServerIP,
		Status:        n.Status,
		PortRange:     n.Port,
		TCPListenAddr: n.TCPListenAddr, UDPListenAddr: n.UDPListenAddr,
		IsRemote: n.IsRemote,
	}
	if n.ServerIPV4.Valid {
		rec.ServerIPv4 = strings.TrimSpace(n.ServerIPV4.String)
	}
	if n.ServerIPV6.Valid {
		rec.ServerIPv6 = strings.TrimSpace(n.ServerIPV6.String)
	}
	if n.InterfaceName.Valid {
		rec.InterfaceName = strings.TrimSpace(n.InterfaceName.String)
	}
	if n.RemoteURL.Valid {
		rec.RemoteURL = strings.TrimSpace(n.RemoteURL.String)
	}
	if n.RemoteToken.Valid {
		rec.RemoteToken = strings.TrimSpace(n.RemoteToken.String)
	}
	if n.RemoteConfig.Valid {
		rec.RemoteConfig = strings.TrimSpace(n.RemoteConfig.String)
	}
	if rec.TCPListenAddr == "" {
		rec.TCPListenAddr = "[::]"
	}
	if rec.UDPListenAddr == "" {
		rec.UDPListenAddr = "[::]"
	}
	if strings.TrimSpace(rec.Name) == "" {
		rec.Name = fmt.Sprintf("node_%d", rec.ID)
	}
	return rec
}

func (r *Repository) ResolveUserTunnelAndLimiter(userID, tunnelID int64) (*model.UserTunnelLimiterInfo, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	type row struct {
		UserTunnelID int64         `gorm:"column:user_tunnel_id"`
		LimiterID    sql.NullInt64 `gorm:"column:limiter_id"`
		Speed        sql.NullInt64 `gorm:"column:speed"`
	}
	var rec row
	err := r.db.Model(&model.UserTunnel{}).
		Select("user_tunnel.id AS user_tunnel_id, speed_limit.id AS limiter_id, speed_limit.speed AS speed").
		Joins("LEFT JOIN speed_limit ON speed_limit.id = user_tunnel.speed_id").
		Where("user_tunnel.user_id = ? AND user_tunnel.tunnel_id = ?", userID, tunnelID).
		Order("user_tunnel.id ASC").
		Limit(1).
		Take(&rec).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &model.UserTunnelLimiterInfo{}, nil
		}
		return nil, err
	}
	info := &model.UserTunnelLimiterInfo{UserTunnelID: rec.UserTunnelID}
	if rec.LimiterID.Valid && rec.LimiterID.Int64 > 0 {
		v := rec.LimiterID.Int64
		info.LimiterID = &v
		s := int(rec.Speed.Int64)
		info.Speed = &s
	}
	return info, nil
}

func (r *Repository) ListUserTunnelIDs(userID, tunnelID int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var ids []int64
	err := r.db.Model(&model.UserTunnel{}).
		Where("user_id = ? AND tunnel_id = ?", userID, tunnelID).
		Order("id ASC").Pluck("id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *Repository) ListUserTunnelIDsByUser(userID int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var ids []int64
	err := r.db.Model(&model.UserTunnel{}).
		Where("user_id = ?", userID).
		Order("id ASC").Pluck("id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *Repository) GetTunnelName(tunnelID int64) (string, error) {
	if r == nil || r.db == nil {
		return "", errors.New("repository not initialized")
	}
	var name string
	err := r.db.Model(&model.Tunnel{}).Where("id = ?", tunnelID).Pluck("name", &name).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return name, nil
}

func (r *Repository) ListChainNodesForTunnel(tunnelID int64) ([]model.ChainNodeRecord, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	type row struct {
		ChainType string
		Inx       sql.NullInt64
		NodeID    int64
		Port      sql.NullInt64
		Name      sql.NullString
		Protocol  sql.NullString
		Strategy  sql.NullString
	}
	var rows []row
	err := r.db.Model(&model.ChainTunnel{}).
		Select("chain_tunnel.chain_type, chain_tunnel.inx, chain_tunnel.node_id, chain_tunnel.port, node.name, chain_tunnel.protocol, chain_tunnel.strategy").
		Joins("LEFT JOIN node ON node.id = chain_tunnel.node_id").
		Where("chain_tunnel.tunnel_id = ?", tunnelID).
		Order("chain_tunnel.chain_type ASC, chain_tunnel.inx ASC, chain_tunnel.id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]model.ChainNodeRecord, 0, len(rows))
	for _, row := range rows {
		chainType := 0
		if v := strings.TrimSpace(row.ChainType); v != "" {
			if parsed, parseErr := strconv.Atoi(v); parseErr == nil {
				chainType = parsed
			}
		}
		inx := int64(0)
		if row.Inx.Valid {
			inx = row.Inx.Int64
		}
		port := 0
		if row.Port.Valid {
			port = int(row.Port.Int64)
		}
		item := model.ChainNodeRecord{
			ChainType: chainType,
			Inx:       inx,
			NodeID:    row.NodeID,
			Port:      port,
		}
		if strings.TrimSpace(row.Name.String) == "" {
			item.NodeName = fmt.Sprintf("node_%d", row.NodeID)
		} else {
			item.NodeName = row.Name.String
		}
		if strings.TrimSpace(row.Protocol.String) == "" {
			item.Protocol = "tls"
		} else {
			item.Protocol = row.Protocol.String
		}
		if strings.TrimSpace(row.Strategy.String) == "" {
			item.Strategy = "round"
		} else {
			item.Strategy = row.Strategy.String
		}
		result = append(result, item)
	}
	return result, nil
}
