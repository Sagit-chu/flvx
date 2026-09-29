package repo

import (
	"crypto/sha256"
	"fmt"

	"go-backend/internal/store/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const FederationBindingPendingRelease = 2

func (r *Repository) ListTunnelChainNodeIDsTx(tx *gorm.DB, tunnelID int64) ([]int64, error) {
	var ids []int64
	err := tx.Model(&model.ChainTunnel{}).Where("tunnel_id = ?", tunnelID).Pluck("node_id", &ids).Error
	return ids, err
}

func (r *Repository) ListFederationTunnelBindingsForCleanup(tunnelID int64) ([]model.FederationTunnelBinding, error) {
	var rows []model.FederationTunnelBinding
	err := r.db.Where("tunnel_id = ? AND status IN ?", tunnelID, []int{1, FederationBindingPendingRelease}).Order("id").Find(&rows).Error
	return rows, err
}

func (r *Repository) ListPendingFederationTunnelBindings() ([]model.FederationTunnelBinding, error) {
	var rows []model.FederationTunnelBinding
	err := r.db.Where("status = ?", FederationBindingPendingRelease).Order("id").Find(&rows).Error
	return rows, err
}

func (r *Repository) MarkFederationTunnelBindingPendingRelease(id int64) error {
	return r.db.Model(&model.FederationTunnelBinding{}).Where("id = ?", id).
		Updates(map[string]interface{}{"status": FederationBindingPendingRelease, "updated_time": unixMilliNow()}).Error
}

func (r *Repository) DeleteFederationTunnelBinding(id int64) error {
	return r.db.Where("id = ?", id).Delete(&model.FederationTunnelBinding{}).Error
}

func (r *Repository) SavePendingFederationRelease(item *model.FederationPendingRelease) error {
	item.ID = fmt.Sprintf("%x", sha256.Sum256([]byte(item.RemoteURL+"\n"+item.BindingID+"\n"+item.ReservationID+"\n"+item.ResourceKey)))
	item.CreatedTime = unixMilliNow()
	return r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(item).Error
}

func (r *Repository) ListPendingFederationReleases() ([]model.FederationPendingRelease, error) {
	var rows []model.FederationPendingRelease
	err := r.db.Order("created_time, id").Find(&rows).Error
	return rows, err
}

func (r *Repository) DeletePendingFederationRelease(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.FederationPendingRelease{}).Error
}
