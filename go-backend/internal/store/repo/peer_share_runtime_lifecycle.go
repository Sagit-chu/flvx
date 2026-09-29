package repo

import (
	"errors"
	"go-backend/internal/store/model"
	"time"
)

// A pending release remains active until the agent acknowledges deletion. This
// keeps its port reserved even if the control connection is unavailable.
func (r *Repository) SetPeerShareRuntimeReleasePending(id int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.PeerShareRuntime{}).Where("id = ? AND status = 1", id).Updates(map[string]interface{}{"release_pending": 1, "updated_time": time.Now().UnixMilli()}).Error
}

func (r *Repository) CompletePeerShareRuntimeRelease(id int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Model(&model.PeerShareRuntime{}).Where("id = ?", id).Updates(map[string]interface{}{"status": 0, "applied": 0, "release_pending": 0, "updated_time": time.Now().UnixMilli()}).Error
}

func (r *Repository) ListActivePeerShareRuntimesByNode(nodeID int64) ([]model.PeerShareRuntime, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var items []model.PeerShareRuntime
	err := r.db.Where("node_id = ? AND status = 1", nodeID).Order("release_pending DESC, id ASC").Find(&items).Error
	return items, err
}
