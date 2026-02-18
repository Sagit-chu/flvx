package repo

import (
	"errors"

	"go-backend/internal/store/model"
)

// ─── Semantic Group Queries (replacing QueryInt64List/QueryPairs passthrough) ─

// ListUserIDsByUserGroup returns all user IDs belonging to a user group.
func (r *Repository) ListUserIDsByUserGroup(userGroupID int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var ids []int64
	err := r.db.Model(&model.UserGroupUser{}).
		Where("user_group_id = ?", userGroupID).
		Pluck("user_id", &ids).Error
	return ids, err
}

// ListTunnelIDsByTunnelGroup returns all tunnel IDs belonging to a tunnel group.
func (r *Repository) ListTunnelIDsByTunnelGroup(tunnelGroupID int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var ids []int64
	err := r.db.Model(&model.TunnelGroupTunnel{}).
		Where("tunnel_group_id = ?", tunnelGroupID).
		Pluck("tunnel_id", &ids).Error
	return ids, err
}

// ListGroupPermissionPairsByUserGroup returns [userGroupID, tunnelGroupID] pairs
// for all group permissions associated with a user group.
func (r *Repository) ListGroupPermissionPairsByUserGroup(userGroupID int64) ([][2]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var perms []model.GroupPermission
	err := r.db.Where("user_group_id = ?", userGroupID).Find(&perms).Error
	if err != nil {
		return nil, err
	}
	result := make([][2]int64, len(perms))
	for i, p := range perms {
		result[i] = [2]int64{p.UserGroupID, p.TunnelGroupID}
	}
	return result, err
}

func (r *Repository) GetUserGroupIDsByUserID(userID int64) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var ids []int64
	err := r.db.Model(&model.UserGroupUser{}).
		Where("user_id = ?", userID).
		Pluck("user_group_id", &ids).Error
	return ids, err
}

func (r *Repository) ListGroupPermissionPairsByTunnelGroup(tunnelGroupID int64) ([][2]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var perms []model.GroupPermission
	err := r.db.Where("tunnel_group_id = ?", tunnelGroupID).Find(&perms).Error
	if err != nil {
		return nil, err
	}
	result := make([][2]int64, len(perms))
	for i, p := range perms {
		result[i] = [2]int64{p.UserGroupID, p.TunnelGroupID}
	}
	return result, err
}
