package repo

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"go-backend/internal/store/model"
	"gorm.io/gorm"
	"math/big"
	"strings"
)

// lockPortAllocationTx serializes reservation mutations across processes sharing a database.
// PostgreSQL uses a transaction-scoped advisory lock; SQLite acquires its writer
// lock without changing any row, including when the node table is empty.
// All panel instances sharing this database must use this protocol; mixed old
// binaries and out-of-band SQL writes are not protected by advisory locks.
func lockPortAllocationTx(tx *gorm.DB) error {
	if tx.Dialector.Name() == "postgres" {
		return tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(0x464c5658504f5254)).Error
	}
	if tx.Dialector.Name() == "sqlite" {
		return tx.Model(&model.Node{}).Where("1 = 0").UpdateColumn("id", gorm.Expr("id")).Error
	}
	return errors.New("unsupported database for atomic port allocation")
}

// LockPortNodesTx must precede occupancy reads and reservation writes.
func (r *Repository) LockPortNodesTx(tx *gorm.DB, ids []int64) error {
	if err := lockPortAllocationTx(tx); err != nil {
		return err
	}
	for _, id := range ids {
		if id <= 0 {
			return errors.New("节点不存在")
		}
	}
	return nil
}

func (r *Repository) usedPortSetTx(tx *gorm.DB, nodeID, excludeForwardID, excludeTunnelID int64, excludeRuntimeIDs ...int64) (map[int]bool, error) {
	used := map[int]bool{}
	var ports []int
	if err := tx.Model(&model.ForwardPort{}).Where("node_id = ? AND forward_id <> ?", nodeID, excludeForwardID).Pluck("port", &ports).Error; err != nil {
		return nil, err
	}
	for _, p := range ports {
		used[p] = true
	}
	query := tx.Model(&model.ChainTunnel{}).Where("node_id = ? AND port > 0", nodeID)
	if excludeTunnelID > 0 {
		query = query.Where("tunnel_id <> ?", excludeTunnelID)
	}
	if err := query.Pluck("port", &ports).Error; err != nil {
		return nil, err
	}
	for _, p := range ports {
		used[p] = true
	}
	runtimeQuery := tx.Model(&model.PeerShareRuntime{}).Where("node_id = ? AND (status = 1 OR release_pending <> 0) AND port > 0", nodeID)
	if len(excludeRuntimeIDs) > 0 {
		runtimeQuery = runtimeQuery.Where("id <> ?", excludeRuntimeIDs[0])
	}
	if err := runtimeQuery.Pluck("port", &ports).Error; err != nil {
		return nil, err
	}
	for _, p := range ports {
		used[p] = true
	}
	pending, err := r.PendingTunnelCleanupPortsTx(tx, nodeID, excludeTunnelID)
	if err != nil {
		return nil, err
	}
	for _, p := range pending {
		used[p] = true
	}
	return used, nil
}

// Selection and insert must be in the same transaction after locking all nodes.
func (r *Repository) selectForwardPortTx(tx *gorm.DB, ids []int64, requested int, excludeForwardID int64, sequential bool) (int, error) {
	if len(ids) == 0 {
		return 0, errors.New("隧道没有可用入口节点")
	}
	var common []int
	for i, id := range ids {
		var node model.Node
		if err := tx.Where("id = ?", id).First(&node).Error; err != nil {
			return 0, err
		}
		spec := strings.TrimSpace(node.Port)
		if spec == "" {
			spec = "1000-65535"
			if requested > 0 {
				spec = "1-65535"
			}
		}
		candidates := parsePortRangeSpec(spec)
		used, err := r.usedPortSetTx(tx, id, excludeForwardID, 0)
		if err != nil {
			return 0, err
		}
		var remote struct {
			Start int `json:"portRangeStart"`
			End   int `json:"portRangeEnd"`
		}
		if node.IsRemote == 1 {
			_ = json.Unmarshal([]byte(node.RemoteConfig.String), &remote)
		}
		available := map[int]bool{}
		for _, p := range candidates {
			if p <= 65535 && !used[p] && (remote.Start <= 0 || remote.End <= 0 || (p >= remote.Start && p <= remote.End)) {
				available[p] = true
			}
		}
		if requested > 0 {
			if !available[requested] {
				return 0, errors.New("端口已占用或不在节点允许范围内")
			}
			continue
		}
		if i == 0 {
			for _, p := range candidates {
				if available[p] {
					common = append(common, p)
				}
			}
		} else {
			filtered := common[:0]
			for _, p := range common {
				if available[p] {
					filtered = append(filtered, p)
				}
			}
			common = filtered
		}
	}
	if requested > 0 {
		return requested, nil
	}
	if len(common) == 0 {
		return 0, errors.New("节点端口已满，无可用端口")
	}
	if sequential {
		return common[0], nil
	}
	idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(common))))
	if err != nil {
		return 0, err
	}
	return common[idx.Int64()], nil
}

// checkPeerRuntimePortTx permits only a verified federation-forward attachment
// to share its owning chain reservation. Other new reservations must be free.
func (r *Repository) checkPeerRuntimePortTx(tx *gorm.DB, item *model.PeerShareRuntime) error {
	if item.Port <= 0 || (item.Status != 1 && item.ReleasePending == 0) {
		return nil
	}
	var ownedTunnel int64
	if item.Role == "forward" {
		var shareID, tunnelID int64
		var port int
		_, err := fmt.Sscanf(item.ResourceKey, "federation-forward-%d-%d-%d", &shareID, &tunnelID, &port)
		if err == nil && shareID == item.ShareID && port == item.Port && item.ResourceKey == fmt.Sprintf("federation-forward-%d-%d-%d", shareID, tunnelID, port) {
			var count int64
			if err := tx.Model(&model.ChainTunnel{}).Where("tunnel_id = ? AND node_id = ? AND port = ?", tunnelID, item.NodeID, item.Port).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				ownedTunnel = tunnelID
			}
		}
	}
	used, err := r.usedPortSetTx(tx, item.NodeID, 0, ownedTunnel, item.ID)
	if err != nil {
		return err
	}
	if used[item.Port] {
		return errors.New("端口已占用")
	}
	return nil
}
