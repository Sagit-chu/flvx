package repo

import "errors"

func (r *Repository) ListVisibleMonitorTunnelIDs(userID int64, nowMs int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	if userID <= 0 {
		return []int64{}, nil
	}

	var ids []int64
	err := r.db.Table("user_tunnel").
		Distinct("user_tunnel.tunnel_id").
		Joins("JOIN tunnel ON tunnel.id = user_tunnel.tunnel_id").
		Where("user_tunnel.user_id = ? AND user_tunnel.status = ? AND user_tunnel.exp_time > ? AND tunnel.status = ?", userID, 1, nowMs, 1).
		Order("user_tunnel.tunnel_id ASC").
		Pluck("user_tunnel.tunnel_id", &ids).Error
	return ids, err
}

func (r *Repository) ListVisibleMonitorNodeIDs(userID int64, nowMs int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	if userID <= 0 {
		return []int64{}, nil
	}

	var ids []int64
	err := r.db.Table("chain_tunnel").
		Distinct("chain_tunnel.node_id").
		Joins("JOIN user_tunnel ON user_tunnel.tunnel_id = chain_tunnel.tunnel_id").
		Joins("JOIN tunnel ON tunnel.id = user_tunnel.tunnel_id").
		Joins("JOIN node ON node.id = chain_tunnel.node_id").
		Where("user_tunnel.user_id = ? AND user_tunnel.status = ? AND user_tunnel.exp_time > ? AND tunnel.status = ? AND node.is_remote = ?", userID, 1, nowMs, 1, 0).
		Order("chain_tunnel.node_id ASC").
		Pluck("chain_tunnel.node_id", &ids).Error
	return ids, err
}
