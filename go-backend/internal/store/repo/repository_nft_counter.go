package repo

import (
	"errors"
	"math"
	"strings"

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
	})
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
