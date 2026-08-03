package repo

import (
	"errors"

	"go-backend/internal/store/model"
)

// InsertTunnelQuality appends a tunnel quality probe result.
// (Follows the same pattern as InsertServiceMonitorResult.)
func (r *Repository) InsertTunnelQuality(q *model.TunnelQuality) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if q == nil || q.TunnelID <= 0 {
		return nil
	}
	return r.db.Create(q).Error
}

// GetTunnelQualityHistory returns quality probe results for a tunnel
// within a time range, ordered by timestamp ascending.
// (Mirrors GetServiceMonitorResults pattern.)
func (r *Repository) GetTunnelQualityHistory(tunnelID int64, startMs, endMs int64) ([]model.TunnelQuality, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var results []model.TunnelQuality
	err := r.db.Where("tunnel_id = ? AND timestamp >= ? AND timestamp <= ?", tunnelID, startMs, endMs).
		Order("timestamp ASC").
		Find(&results).Error
	return results, err
}

// GetLatestTunnelQualities returns the newest quality result per tunnel_id.
// (Mirrors GetLatestServiceMonitorResults pattern.)
func (r *Repository) GetLatestTunnelQualities() ([]model.TunnelQuality, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}

	var results []model.TunnelQuality

	// Use window function (works on modern SQLite 3.25+ and PostgreSQL).
	q := `
		SELECT id, tunnel_id, entry_to_exit_latency, exit_to_bing_latency,
		       entry_to_exit_loss, exit_to_bing_loss, success, error_message, timestamp,
		       chain_details
		FROM (
			SELECT *, ROW_NUMBER() OVER (PARTITION BY tunnel_id ORDER BY timestamp DESC, id DESC) AS rn
			FROM tunnel_quality
		) t
		WHERE rn = 1
		ORDER BY tunnel_id ASC
	`
	if err := r.db.Raw(q).Scan(&results).Error; err == nil {
		return results, nil
	}

	// Fallback for older SQLite
	results = nil
	err := r.db.Order("timestamp DESC, id DESC").Limit(5000).Find(&results).Error
	if err != nil {
		return nil, err
	}

	seen := make(map[int64]struct{}, len(results))
	out := make([]model.TunnelQuality, 0, len(results))
	for _, row := range results {
		if row.TunnelID <= 0 {
			continue
		}
		if _, ok := seen[row.TunnelID]; ok {
			continue
		}
		seen[row.TunnelID] = struct{}{}
		out = append(out, row)
	}
	return out, nil
}

// PruneTunnelQualityResults deletes quality results older than the given timestamp.
// (Mirrors PruneServiceMonitorResults pattern.)
func (r *Repository) PruneTunnelQualityResults(olderThanMs int64) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Where("timestamp < ?", olderThanMs).Delete(&model.TunnelQuality{}).Error
}

// ListEnabledTunnelIDs returns IDs of all tunnels with status=1.
func (r *Repository) ListEnabledTunnelIDs() ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var ids []int64
	err := r.db.Model(&model.Tunnel{}).Where("status = ?", 1).Pluck("id", &ids).Error
	return ids, err
}
