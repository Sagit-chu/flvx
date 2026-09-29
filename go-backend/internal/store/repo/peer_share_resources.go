package repo

import (
	"errors"
	"go-backend/internal/store/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Repository) SavePeerShareResources(items []PeerShareResource) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if len(items) == 0 {
		return nil
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		for i := range items {
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "share_id"}, {Name: "kind"}, {Name: "original_name"}}, DoUpdates: clause.AssignmentColumns([]string{"node_id", "runtime_name", "legacy_names", "legacy_service_base", "release_legacy_family", "config", "desired_state", "applied", "updated_time"})}).Create(&items[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) GetPeerShareResource(shareID int64, kind, originalName string) (*PeerShareResource, error) {
	var item model.PeerShareResource
	err := r.db.Where("share_id = ? AND kind = ? AND original_name = ?", shareID, kind, originalName).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &item, err
}

func (r *Repository) ListPeerShareResourcesByNode(nodeID int64) ([]PeerShareResource, error) {
	var items []PeerShareResource
	err := r.db.Where("node_id = ?", nodeID).Order("id").Find(&items).Error
	return items, err
}

func (r *Repository) MarkPeerShareResourceApplied(shareID int64, kind, name string) error {
	return r.db.Model(&model.PeerShareResource{}).Where("share_id = ? AND kind = ? AND original_name = ?", shareID, kind, name).Update("applied", 1).Error
}

func (r *Repository) ClearPeerShareResourceLegacyNames(shareID int64, kind, name string) error {
	return r.db.Model(&model.PeerShareResource{}).Where("share_id = ? AND kind = ? AND original_name = ?", shareID, kind, name).Update("legacy_names", "").Error
}

func (r *Repository) WithPeerShareResourceTransaction(fn func(*Repository) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error { return fn(&Repository{db: tx, dbPath: r.dbPath}) })
}
func (r *Repository) ClearPeerShareResourceLegacyFamily(shareID int64, base string) error {
	return r.db.Model(&model.PeerShareResource{}).Where("share_id = ? AND legacy_service_base = ?", shareID, base).Update("legacy_service_base", "").Error
}
