package repo

import (
	"errors"
	"math"
	"strings"
	"time"

	"go-backend/internal/store/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	nftCounterProtocolTCP = "tcp"
	nftCounterProtocolUDP = "udp"

	nftCounterDirectionToTarget   = "to-target"
	nftCounterDirectionFromTarget = "from-target"
)

type NftCounterStateInput struct {
	NodeID        int64
	ForwardID     int64
	Protocol      string
	Direction     string
	RuleHash      string
	Bytes         uint64
	Packets       uint64
	CollectedTime int64
}

type NftablesCollectionNode struct {
	NodeID int64
	Config model.NodeSSHConfig
}

func (r *Repository) ListNftablesNodesForCollection() ([]NftablesCollectionNode, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	type collectionRow struct {
		NodeID      int64  `gorm:"column:node_id"`
		ConfigID    int64  `gorm:"column:config_id"`
		Host        string `gorm:"column:host"`
		Port        int    `gorm:"column:port"`
		Username    string `gorm:"column:username"`
		AuthType    string `gorm:"column:auth_type"`
		Password    string `gorm:"column:password"`
		PrivateKey  string `gorm:"column:private_key"`
		Passphrase  string `gorm:"column:passphrase"`
		SudoMode    string `gorm:"column:sudo_mode"`
		CreatedTime int64  `gorm:"column:created_time"`
		UpdatedTime int64  `gorm:"column:updated_time"`
	}

	var rows []collectionRow
	if err := r.db.Table("node").
		Select("node.id AS node_id, node_ssh_config.id AS config_id, node_ssh_config.host, node_ssh_config.port, node_ssh_config.username, node_ssh_config.auth_type, node_ssh_config.password, node_ssh_config.private_key, node_ssh_config.passphrase, node_ssh_config.sudo_mode, node_ssh_config.created_time, node_ssh_config.updated_time").
		Joins("JOIN node_ssh_config ON node_ssh_config.node_id = node.id").
		Where("node.status = ? AND LOWER(TRIM(node.forward_mode)) = ?", 1, "nftables").
		Order("node.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	nodes := make([]NftablesCollectionNode, 0, len(rows))
	for _, row := range rows {
		nodes = append(nodes, NftablesCollectionNode{
			NodeID: row.NodeID,
			Config: model.NodeSSHConfig{
				ID:          row.ConfigID,
				NodeID:      row.NodeID,
				Host:        row.Host,
				Port:        row.Port,
				Username:    row.Username,
				AuthType:    row.AuthType,
				Password:    nullStringFromInterface(row.Password),
				PrivateKey:  nullStringFromInterface(row.PrivateKey),
				Passphrase:  nullStringFromInterface(row.Passphrase),
				SudoMode:    row.SudoMode,
				CreatedTime: row.CreatedTime,
				UpdatedTime: row.UpdatedTime,
			},
		})
	}
	return nodes, nil
}

func (r *Repository) GetNftCounterStatesByNode(nodeID int64) ([]model.NftCounterState, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var rows []model.NftCounterState
	err := r.db.Where("node_id = ?", nodeID).
		Order("forward_id ASC, protocol ASC, direction ASC").
		Find(&rows).Error
	return rows, err
}

func (r *Repository) UpsertNftCounterStates(inputs []NftCounterStateInput, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if len(inputs) == 0 {
		return nil
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		return upsertNftCounterStatesTx(tx, inputs, now)
	})
}

func (r *Repository) ApplyNftTrafficAccounting(deltas []FlowUploadCounterDelta, quotaUsage map[int64]int64, states []NftCounterStateInput, now time.Time) (map[int64]*model.UserQuotaView, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}

	quotaViews := map[int64]*model.UserQuotaView{}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := applyFlowUploadDeltasTx(tx, deltas); err != nil {
			return err
		}
		var err error
		quotaViews, err = r.addUserQuotaUsageBatchTx(tx, quotaUsage, now)
		if err != nil {
			return err
		}
		return upsertNftCounterStatesTx(tx, states, now.UnixMilli())
	})
	if err != nil {
		return nil, err
	}
	return quotaViews, nil
}

func upsertNftCounterStatesTx(tx *gorm.DB, inputs []NftCounterStateInput, now int64) error {
	if tx == nil {
		return errors.New("database unavailable")
	}
	for _, input := range inputs {
		row, ok := nftCounterStateFromInput(input, now)
		if !ok {
			continue
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "node_id"},
				{Name: "forward_id"},
				{Name: "protocol"},
				{Name: "direction"},
			},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"rule_hash":      row.RuleHash,
				"bytes":          row.Bytes,
				"packets":        row.Packets,
				"collected_time": row.CollectedTime,
				"updated_time":   row.UpdatedTime,
			}),
		}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) DeleteNftCounterStatesByForward(forwardID int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Where("forward_id = ?", forwardID).Delete(&model.NftCounterState{}).Error
}

func nftCounterStateFromInput(input NftCounterStateInput, now int64) (model.NftCounterState, bool) {
	protocol := strings.ToLower(strings.TrimSpace(input.Protocol))
	direction := strings.ToLower(strings.TrimSpace(input.Direction))
	if input.NodeID <= 0 || input.ForwardID <= 0 || !isValidNftCounterProtocol(protocol) || !isValidNftCounterDirection(direction) {
		return model.NftCounterState{}, false
	}
	if input.Bytes > uint64(math.MaxInt64) || input.Packets > uint64(math.MaxInt64) {
		return model.NftCounterState{}, false
	}
	return model.NftCounterState{
		NodeID:        input.NodeID,
		ForwardID:     input.ForwardID,
		Protocol:      protocol,
		Direction:     direction,
		RuleHash:      strings.TrimSpace(input.RuleHash),
		Bytes:         int64(input.Bytes),
		Packets:       int64(input.Packets),
		CollectedTime: input.CollectedTime,
		CreatedTime:   now,
		UpdatedTime:   now,
	}, true
}

func isValidNftCounterProtocol(protocol string) bool {
	return protocol == nftCounterProtocolTCP || protocol == nftCounterProtocolUDP
}

func isValidNftCounterDirection(direction string) bool {
	return direction == nftCounterDirectionToTarget || direction == nftCounterDirectionFromTarget
}
