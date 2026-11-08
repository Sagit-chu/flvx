package repo

import (
	"go-backend/internal/store/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Repository) QueueTunnelRuntimeCleanupTx(tx *gorm.DB, items []model.TunnelRuntimeCleanup) error {
	for _, item := range items {
		var node model.Node
		if err := tx.Select("id", "is_remote").Where("id = ?", item.NodeID).First(&node).Error; err != nil {
			return err
		}
		if node.IsRemote == 1 {
			continue
		}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tunnel_id"}, {Name: "node_id"}, {Name: "kind"}, {Name: "port"}}, DoNothing: true}).Create(&item).Error; err != nil {
			return err
		}
	}
	return nil
}
func (r *Repository) ListTunnelRuntimeCleanup(tunnelID int64) ([]model.TunnelRuntimeCleanup, error) {
	var items []model.TunnelRuntimeCleanup
	q := r.db.Model(&model.TunnelRuntimeCleanup{})
	if tunnelID > 0 {
		q = q.Where("tunnel_id = ?", tunnelID)
	}
	err := q.Order("tunnel_id, node_id, kind DESC").Find(&items).Error
	return items, err
}
func (r *Repository) DeleteTunnelRuntimeCleanup(id int64) error {
	return r.db.Where("id = ?", id).Delete(&model.TunnelRuntimeCleanup{}).Error
}
func (r *Repository) PendingTunnelCleanupPortsTx(tx *gorm.DB, nodeID, excludeTunnelID int64) ([]int, error) {
	if tx == nil {
		tx = r.db
	}
	q := tx.Model(&model.TunnelRuntimeCleanup{}).Where("node_id = ? AND kind = ? AND port > 0", nodeID, "service")
	if excludeTunnelID > 0 {
		q = q.Where("tunnel_id != ?", excludeTunnelID)
	}
	var ports []int
	err := q.Pluck("port", &ports).Error
	return ports, err
}

func (r *Repository) queueDeletedTunnelRuntimeTx(tx *gorm.DB, tunnelID int64) error {
	var tunnels []model.Tunnel
	if err := tx.Where("id = ? AND type = ?", tunnelID, 2).Find(&tunnels).Error; err != nil {
		return err
	}
	if len(tunnels) == 0 {
		return nil
	}
	var rows []model.ChainTunnel
	if err := tx.Where("tunnel_id = ?", tunnelID).Find(&rows).Error; err != nil {
		return err
	}
	var items []model.TunnelRuntimeCleanup
	for _, row := range rows {
		if row.ChainType == "2" || row.ChainType == "3" {
			items = append(items, model.TunnelRuntimeCleanup{TunnelID: tunnelID, NodeID: row.NodeID, Kind: "service", Port: int(row.Port.Int64)})
		}
		if row.ChainType == "1" || row.ChainType == "2" {
			items = append(items, model.TunnelRuntimeCleanup{TunnelID: tunnelID, NodeID: row.NodeID, Kind: "chain"})
		}
	}
	return r.QueueTunnelRuntimeCleanupTx(tx, items)
}
