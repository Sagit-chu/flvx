package handler

import (
	"database/sql"
	"errors"
	"go-backend/internal/http/response"
	"gorm.io/gorm"
	"slices"
	"sort"
	"sync"
)

type tunnelLifecycleLock struct {
	mu    sync.Mutex
	users int
}

// lockTunnelLifecycle serializes snapshots, persistence and node acknowledgements.
// Multiple tunnels (forward migration) are locked in ID order to avoid deadlocks.
// Callers must not acquire these locks recursively or hold a database transaction.
func (h *Handler) lockTunnelLifecycle(ids ...int64) func() {
	unique := make(map[int64]bool, len(ids))
	ordered := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id > 0 && !unique[id] {
			unique[id] = true
			ordered = append(ordered, id)
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	locks := make([]*tunnelLifecycleLock, 0, len(ordered))
	for _, id := range ordered {
		h.tunnelLifecycleMu.Lock()
		if h.tunnelLifecycleLocks == nil {
			h.tunnelLifecycleLocks = make(map[int64]*tunnelLifecycleLock)
		}
		lock := h.tunnelLifecycleLocks[id]
		if lock == nil {
			lock = &tunnelLifecycleLock{}
			h.tunnelLifecycleLocks[id] = lock
		}
		lock.users++
		h.tunnelLifecycleMu.Unlock()
		lock.mu.Lock()
		locks = append(locks, lock)
	}
	return func() {
		for i := len(locks) - 1; i >= 0; i-- {
			h.tunnelLifecycleMu.Lock()
			locks[i].mu.Unlock()
			locks[i].users--
			if locks[i].users == 0 {
				delete(h.tunnelLifecycleLocks, ordered[i])
			}
			h.tunnelLifecycleMu.Unlock()
		}
	}
}

// Probes run without the lifecycle lock. Recheck their topology under the lock
// so a late result cannot restore a removed middle hop or deleted tunnel.
func (h *Handler) validateBestExitSnapshot(tunnelID, ownerNodeID int64, exits []chainNodeRecord, ipPreference string) error {
	tunnel, err := h.getTunnelRecord(tunnelID)
	if err != nil {
		return err
	}
	if tunnel.Type != 2 || h.repo.GetTunnelIPPreference(tunnelID) != ipPreference {
		return errors.New("best exit topology changed; retry next probe")
	}
	rows, err := h.listChainNodesForTunnel(tunnelID)
	if err != nil {
		return err
	}
	ins, hops, currentExits := splitChainNodeGroups(rows)
	if !slices.Equal(exits, currentExits) {
		return errors.New("best exit targets changed; retry next probe")
	}
	for _, owner := range bestExitChainOwners(ins, hops) {
		if owner.NodeID == ownerNodeID {
			return nil
		}
	}
	return errors.New("best exit owner changed; retry next probe")
}

// A newly allocated ID is not visible yet. Claim it without waiting while the
// insert transaction is open: an overlapping request for that ID must not make
// creation wait on a lifecycle lock whose owner is waiting for this transaction.
func (h *Handler) tryLockNewTunnelLifecycle(id int64) (func(), bool) {
	h.tunnelLifecycleMu.Lock()
	defer h.tunnelLifecycleMu.Unlock()
	if h.tunnelLifecycleLocks == nil {
		h.tunnelLifecycleLocks = make(map[int64]*tunnelLifecycleLock)
	}
	if h.tunnelLifecycleLocks[id] != nil {
		return nil, false
	}
	lock := &tunnelLifecycleLock{users: 1}
	lock.mu.Lock()
	h.tunnelLifecycleLocks[id] = lock
	return func() {
		h.tunnelLifecycleMu.Lock()
		defer h.tunnelLifecycleMu.Unlock()
		lock.mu.Unlock()
		lock.users--
		if lock.users == 0 {
			delete(h.tunnelLifecycleLocks, id)
		}
	}, true
}

// Pending is not successful application. HTTP create/update retain their
// accepted-config contract, while redeploy returns this error to its batch result.
type tunnelRuntimePendingError struct{ cause error }

func (e *tunnelRuntimePendingError) Error() string { return "运行时待同步: " + e.cause.Error() }
func isTunnelRuntimePending(err error) bool {
	var pending *tunnelRuntimePendingError
	return errors.As(err, &pending)
}
func tunnelRuntimePendingResponse(tunnelID int64, err error) response.R {
	message := "配置已保存，部分节点运行时待同步：" + err.Error()
	out := response.OK(map[string]interface{}{"runtimeStatus": "pending", "tunnelId": tunnelID, "message": message})
	out.Msg = message
	return out
}

// Reserve locally allocated ports with the initial desired state, before any
// remote acknowledgment. Remote ports remain unallocated until their provider
// returns; the second transaction replaces these rows with the final bindings.
func (h *Handler) persistPreparedTunnelChainsTx(tx *gorm.DB, state *tunnelCreateState) error {
	save := func(node tunnelRuntimeNode, kind string, inx int) error {
		return h.repo.CreateChainTunnelTx(tx, state.TunnelID, kind, node.NodeID, sql.NullInt64{Int64: int64(node.Port), Valid: node.Port > 0}, defaultString(node.Strategy, "round"), inx, defaultString(node.Protocol, "tls"), node.ConnectIP)
	}
	for i, node := range state.InNodes {
		if err := save(node, "1", i+1); err != nil {
			return err
		}
	}
	for i, node := range state.OutNodes {
		if err := save(node, "3", i+1); err != nil {
			return err
		}
	}
	for i, hop := range state.ChainHops {
		for _, node := range hop {
			if err := save(node, "2", i+1); err != nil {
				return err
			}
		}
	}
	return nil
}
