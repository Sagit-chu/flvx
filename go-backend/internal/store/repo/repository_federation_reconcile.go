package repo

import (
	"sort"

	"go-backend/internal/store/model"
)

// Only retry unfinished operations. Successful desired state must be replayed
// on a real reconnect, not every maintenance tick while the agent stays online.
func (r *Repository) ListPendingPeerShareNodeIDs() ([]int64, error) {
	var resourceNodes, runtimeNodes []int64
	if err := r.db.Model(&model.PeerShareResource{}).Where("applied = 0").Distinct("node_id").Pluck("node_id", &resourceNodes).Error; err != nil {
		return nil, err
	}
	if err := r.db.Model(&model.PeerShareRuntime{}).
		Where("status = 1 AND (release_pending <> 0 OR (applied = 0 AND service_name <> '' AND role IN ?))", []string{"middle", "exit"}).
		Distinct("node_id").Pluck("node_id", &runtimeNodes).Error; err != nil {
		return nil, err
	}
	seen := make(map[int64]struct{})
	for _, ids := range [][]int64{resourceNodes, runtimeNodes} {
		for _, id := range ids {
			if id > 0 {
				seen[id] = struct{}{}
			}
		}
	}
	out := make([]int64, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}
