package handler

import (
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"go-backend/internal/store/model"
)

const bytesPerGB int64 = 1024 * 1024 * 1024

type userTunnelPolicy struct {
	ID       int64
	UserID   int64
	TunnelID int64
	Flow     int64
	InFlow   int64
	OutFlow  int64
	ExpTime  int64
	Status   int
}

type gostConfigSnapshot struct {
	Services []namedConfigItem `json:"services"`
	Chains   []namedConfigItem `json:"chains"`
	Limiters []namedConfigItem `json:"limiters"`
}

type namedConfigItem struct {
	Name string `json:"name"`
}

func (h *Handler) processFlowItem(nodeID int64, item flowItem) {
	serviceName := strings.TrimSpace(item.N)
	if serviceName == "" || serviceName == "web_api" {
		return
	}

	forwardID, userID, userTunnelID, ok := parseFlowServiceIDs(serviceName)
	if ok {
		inFlow, outFlow := h.scaleFlowByTunnel(forwardID, item.D, item.U)
		_ = h.repo.AddFlow(forwardID, userID, userTunnelID, inFlow, outFlow)
		if forward, err := h.getForwardRecord(forwardID); err == nil && forward != nil {
			if quota, quotaErr := h.repo.AddTunnelQuotaUsage(forward.TunnelID, inFlow+outFlow, time.Now()); quotaErr == nil {
				h.enforceTunnelQuotaIfNeeded(forward.TunnelID, quota)
			}
		}
		h.processPeerShareFlowFromForward(forwardID, nodeID, serviceName, item)

		if userTunnelID > 0 {
			h.enforceFlowPolicies(userID, userTunnelID)
		}
		return
	}

	runtimeID, ok := parsePeerShareRuntimeServiceID(serviceName)
	if !ok {
		return
	}
	h.processPeerShareFlow(runtimeID, item)
}

func parseFlowServiceIDs(serviceName string) (int64, int64, int64, bool) {
	parts := strings.Split(serviceName, "_")
	if len(parts) < 3 {
		return 0, 0, 0, false
	}

	forwardID, err1 := strconv.ParseInt(parts[0], 10, 64)
	userID, err2 := strconv.ParseInt(parts[1], 10, 64)
	userTunnelID, err3 := strconv.ParseInt(parts[2], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil || forwardID <= 0 || userID <= 0 {
		return 0, 0, 0, false
	}

	return forwardID, userID, userTunnelID, true
}

func parsePeerShareRuntimeServiceID(serviceName string) (int64, bool) {
	const prefix = "fed_svc_"
	if !strings.HasPrefix(serviceName, prefix) {
		return 0, false
	}
	raw := strings.TrimPrefix(serviceName, prefix)
	if raw == "" {
		return 0, false
	}
	parts := strings.SplitN(raw, "_", 2)
	runtimeID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || runtimeID <= 0 {
		return 0, false
	}
	return runtimeID, true
}

func parsePeerShareInfoFromFederationTunnelName(tunnelName string) (int64, int, bool) {
	tunnelName = strings.TrimSpace(tunnelName)
	if !strings.HasPrefix(tunnelName, "Share-") {
		return 0, 0, false
	}
	raw := strings.TrimPrefix(tunnelName, "Share-")
	idx := strings.Index(raw, "-Port-")
	if idx <= 0 {
		return 0, 0, false
	}
	shareID, err := strconv.ParseInt(raw[:idx], 10, 64)
	if err != nil || shareID <= 0 {
		return 0, 0, false
	}
	portValue := strings.TrimSpace(raw[idx+len("-Port-"):])
	port, err := strconv.Atoi(portValue)
	if err != nil || port <= 0 {
		return 0, 0, false
	}
	return shareID, port, true
}

func parsePeerShareIDFromFederationTunnelName(tunnelName string) (int64, bool) {
	tunnelName = strings.TrimSpace(tunnelName)
	if !strings.HasPrefix(tunnelName, "Share-") {
		return 0, false
	}
	raw := strings.TrimPrefix(tunnelName, "Share-")
	idx := strings.Index(raw, "-Port-")
	if idx <= 0 {
		return 0, false
	}
	shareID, err := strconv.ParseInt(raw[:idx], 10, 64)
	if err != nil || shareID <= 0 {
		return 0, false
	}
	return shareID, true
}

func (h *Handler) processPeerShareFlow(runtimeID int64, item flowItem) {
	if h == nil || h.repo == nil || runtimeID <= 0 {
		return
	}
	runtime, err := h.repo.GetPeerShareRuntimeByID(runtimeID)
	if err != nil || runtime == nil || runtime.ShareID <= 0 || runtime.Status != 1 {
		return
	}

	delta := item.D + item.U
	if delta <= 0 {
		return
	}

	_ = h.repo.AddPeerShareCurrentFlow(runtime.ShareID, delta)

	share, err := h.repo.GetPeerShare(runtime.ShareID)
	if err != nil || share == nil {
		return
	}
	if !isPeerShareFlowExceeded(share) {
		return
	}
	h.enforcePeerShareFlowLimit(share.ID)
}

func (h *Handler) processPeerShareFlowFromForward(forwardID int64, nodeID int64, serviceName string, item flowItem) {
	if h == nil || h.repo == nil || forwardID <= 0 {
		return
	}

	delta := item.D + item.U
	if delta <= 0 {
		return
	}

	forward, err := h.getForwardRecord(forwardID)
	if err != nil || forward == nil {
		// Forward not found in local database - might be a federation port-forward
		// Try to find by service name in peer_share_runtime
		h.processPeerShareFlowByServiceName(nodeID, serviceName, item)
		return
	}
	tunnelName, err := h.repo.GetTunnelName(forward.TunnelID)
	if err != nil {
		h.processPeerShareFlowByServiceName(nodeID, serviceName, item)
		return
	}
	shareID, ok := parsePeerShareIDFromFederationTunnelName(tunnelName)
	if !ok {
		h.processPeerShareFlowByServiceName(nodeID, serviceName, item)
		return
	}

	if err := h.repo.AddPeerShareCurrentFlow(shareID, delta); err != nil {
		h.processPeerShareFlowByServiceName(nodeID, serviceName, item)
		return
	}

	share, err := h.repo.GetPeerShare(shareID)
	if err != nil || share == nil {
		return
	}
	if !isPeerShareFlowExceeded(share) {
		return
	}
	h.enforcePeerShareFlowLimit(share.ID)
}

func normalizeForwardRuntimeServiceName(serviceName string) string {
	name := strings.TrimSpace(serviceName)
	if strings.HasSuffix(name, "_tcp") {
		return strings.TrimSuffix(name, "_tcp")
	}
	if strings.HasSuffix(name, "_udp") {
		return strings.TrimSuffix(name, "_udp")
	}
	return name
}

func (h *Handler) processPeerShareFlowByServiceName(nodeID int64, serviceName string, item flowItem) {
	if h == nil || h.repo == nil || strings.TrimSpace(serviceName) == "" {
		return
	}

	delta := item.D + item.U
	if delta <= 0 {
		return
	}

	normalized := normalizeForwardRuntimeServiceName(serviceName)
	var runtimes []model.PeerShareRuntime
	var err error

	// Try node-scoped query first if nodeID is valid
	if nodeID > 0 {
		runtimes, err = h.repo.ListActiveForwardPeerShareRuntimesByNodeAndServiceName(nodeID, normalized)
		if err != nil {
			return
		}
		if len(runtimes) == 0 && normalized != serviceName {
			runtimes, err = h.repo.ListActiveForwardPeerShareRuntimesByNodeAndServiceName(nodeID, serviceName)
			if err != nil {
				return
			}
		}
	}

	// Fallback to global query if node-scoped query returned nothing or nodeID is invalid
	if len(runtimes) == 0 {
		runtimes, err = h.repo.ListActiveForwardPeerShareRuntimesByServiceName(normalized)
		if err != nil {
			return
		}
		if len(runtimes) == 0 && normalized != serviceName {
			runtimes, err = h.repo.ListActiveForwardPeerShareRuntimesByServiceName(serviceName)
			if err != nil {
				return
			}
		}
	}

	if len(runtimes) != 1 {
		if len(runtimes) > 1 {
			log.Printf("WARN: ambiguous peer share runtime match for service=%s nodeID=%d count=%d", serviceName, nodeID, len(runtimes))
		}
		return
	}
	runtime := runtimes[0]

	_ = h.repo.AddPeerShareCurrentFlow(runtime.ShareID, delta)

	matchedShare, err := h.repo.GetPeerShare(runtime.ShareID)
	if err != nil || matchedShare == nil {
		return
	}
	if isPeerShareFlowExceeded(matchedShare) {
		h.enforcePeerShareFlowLimit(matchedShare.ID)
	}
}

func (h *Handler) enforcePeerShareFlowLimit(shareID int64) {
	if h == nil || h.repo == nil || shareID <= 0 {
		return
	}
	runtimes, err := h.repo.ListActivePeerShareRuntimesByShareID(shareID)
	if err != nil || len(runtimes) == 0 {
		return
	}

	now := time.Now().UnixMilli()
	for _, runtime := range runtimes {
		if h.wsServer != nil && runtime.Applied == 1 {
			if strings.TrimSpace(runtime.ServiceName) != "" {
				_, _ = h.sendNodeCommand(runtime.NodeID, "DeleteService", map[string]interface{}{"services": []string{runtime.ServiceName}}, false, true)
			}
			if strings.TrimSpace(runtime.Role) == "middle" && strings.TrimSpace(runtime.ChainName) != "" {
				_, _ = h.sendNodeCommand(runtime.NodeID, "DeleteChains", map[string]interface{}{"chain": runtime.ChainName}, false, true)
			}
		}
		_ = h.repo.MarkPeerShareRuntimeReleased(runtime.ID, now)
	}
}

func (h *Handler) scaleFlowByTunnel(forwardID int64, inFlow int64, outFlow int64) (int64, int64) {
	forward, err := h.getForwardRecord(forwardID)
	if err != nil || forward == nil {
		return inFlow, outFlow
	}

	tunnel, err := h.getTunnelRecord(forward.TunnelID)
	if err != nil || tunnel == nil {
		return inFlow, outFlow
	}

	scaledIn := int64(float64(inFlow)*tunnel.TrafficRatio) * tunnel.Flow
	scaledOut := int64(float64(outFlow)*tunnel.TrafficRatio) * tunnel.Flow
	return scaledIn, scaledOut
}

func (h *Handler) enforceFlowPolicies(userID int64, userTunnelID int64) {
	now := time.Now().UnixMilli()

	if h.shouldPauseUser(userID, now) {
		h.pauseUserForwards(userID, now)
	}

	policy, err := h.getUserTunnelPolicy(userTunnelID)
	if err != nil || policy == nil {
		return
	}

	if shouldPauseUserTunnel(policy, now) {
		h.pauseUserTunnelForwards(policy.UserID, policy.TunnelID, now)
	}
}

func (h *Handler) ensureUserTunnelForwardAllowed(userID int64, tunnelID int64, now int64) error {
	if h == nil || h.repo == nil {
		return errors.New("invalid flow policy context")
	}
	if userID <= 0 || tunnelID <= 0 {
		return nil
	}

	user, err := h.repo.GetUserByID(userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New("用户不存在")
	}

	if user.Status != 1 {
		return errors.New("账号已禁用")
	}
	if user.ExpTime > 0 && user.ExpTime <= now {
		return errors.New("账号已过期")
	}

	flowLimit := user.Flow * bytesPerGB
	current := user.InFlow + user.OutFlow
	if flowLimit < current {
		return errors.New("流量已超额，禁止开启转发")
	}
	if err := h.ensureTunnelForwardAllowedByQuota(tunnelID, now); err != nil {
		return err
	}

	userTunnelID, _, _, err := h.resolveUserTunnelAndLimiter(userID, tunnelID)
	if err != nil {
		return err
	}
	if userTunnelID <= 0 {
		return nil
	}

	policy, err := h.getUserTunnelPolicy(userTunnelID)
	if err != nil {
		return err
	}
	if policy == nil {
		return nil
	}

	if policy.Status != 1 {
		return errors.New("该隧道已禁用")
	}
	if policy.ExpTime > 0 && policy.ExpTime <= now {
		return errors.New("该隧道已过期")
	}

	utFlowLimit := policy.Flow * bytesPerGB
	utCurrent := policy.InFlow + policy.OutFlow
	if utCurrent >= utFlowLimit {
		return errors.New("该隧道流量已超额，禁止开启转发")
	}

	return nil
}

func (h *Handler) shouldPauseUser(userID int64, now int64) bool {
	user, err := h.repo.GetUserByID(userID)
	if err != nil || user == nil {
		return false
	}

	flowLimit := user.Flow * bytesPerGB
	current := user.InFlow + user.OutFlow
	if flowLimit < current {
		return true
	}
	if user.ExpTime > 0 && user.ExpTime <= now {
		return true
	}
	return user.Status != 1
}

func shouldPauseUserTunnel(policy *userTunnelPolicy, now int64) bool {
	if policy == nil {
		return false
	}

	flowLimit := policy.Flow * bytesPerGB
	current := policy.InFlow + policy.OutFlow
	if current >= flowLimit {
		return true
	}
	if policy.ExpTime > 0 && policy.ExpTime <= now {
		return true
	}
	return policy.Status != 1
}

func (h *Handler) getUserTunnelPolicy(userTunnelID int64) (*userTunnelPolicy, error) {
	if userTunnelID <= 0 {
		return nil, nil
	}
	ut, err := h.repo.GetUserTunnelByID(userTunnelID)
	if err != nil {
		return nil, err
	}
	if ut == nil {
		return nil, nil
	}
	return &userTunnelPolicy{
		ID: ut.ID, UserID: ut.UserID, TunnelID: ut.TunnelID,
		Flow: ut.Flow, InFlow: ut.InFlow, OutFlow: ut.OutFlow,
		ExpTime: ut.ExpTime, Status: ut.Status,
	}, nil
}

func (h *Handler) pauseUserForwards(userID int64, now int64) {
	forwards, err := h.listActiveForwardsByUser(userID)
	if err != nil {
		return
	}
	h.pauseForwardRecords(forwards, now)
}

func (h *Handler) pauseUserTunnelForwards(userID int64, tunnelID int64, now int64) {
	forwards, err := h.listActiveForwardsByUserTunnel(userID, tunnelID)
	if err != nil {
		return
	}
	h.pauseForwardRecords(forwards, now)
}

func (h *Handler) pauseForwardRecords(forwards []forwardRecord, now int64) {
	for i := range forwards {
		forward := forwards[i]
		_ = h.controlForwardServices(&forward, "PauseService", false)
		_ = h.repo.UpdateForwardStatus(forward.ID, 0, now)
	}
}

func (h *Handler) listActiveForwardsByUser(userID int64) ([]forwardRecord, error) {
	return h.repo.ListActiveForwardsByUser(userID)
}

func (h *Handler) listActiveForwardsByUserTunnel(userID int64, tunnelID int64) ([]forwardRecord, error) {
	return h.repo.ListActiveForwardsByUserTunnel(userID, tunnelID)
}

func (h *Handler) cleanNodeConfigs(nodeID int64, rawConfig string) {
	if h == nil || h.repo == nil || nodeID <= 0 {
		return
	}
	if strings.TrimSpace(rawConfig) == "" {
		return
	}

	var snapshot gostConfigSnapshot
	if err := json.Unmarshal([]byte(rawConfig), &snapshot); err != nil {
		return
	}

	h.cleanOrphanedServices(nodeID, snapshot.Services)
	h.cleanOrphanedChains(nodeID, snapshot.Chains)
	h.cleanOrphanedLimiters(nodeID, snapshot.Limiters)
}

func (h *Handler) cleanOrphanedServices(nodeID int64, services []namedConfigItem) {
	runtimeServiceNames, err := h.repo.ListActiveForwardPeerShareRuntimeServiceNamesByNode(nodeID)
	if err != nil {
		return
	}
	minUpdatedTime := time.Now().Add(-10 * time.Minute).UnixMilli()
	hasUnboundForwardPeerRuntime, err := h.repo.HasRecentUnboundForwardPeerShareRuntimeOnNode(nodeID, minUpdatedTime)
	if err != nil {
		hasUnboundForwardPeerRuntime = false
	}
	runtimeServiceSet := make(map[string]struct{}, len(runtimeServiceNames))
	for _, serviceName := range runtimeServiceNames {
		serviceName = strings.TrimSpace(serviceName)
		if serviceName == "" {
			continue
		}
		runtimeServiceSet[serviceName] = struct{}{}
	}

	for _, item := range services {
		name := strings.TrimSpace(item.Name)
		if name == "" || name == "web_api" {
			continue
		}
		if strings.HasPrefix(name, "fed_svc_") {
			continue
		}
		normalizedName := normalizeForwardRuntimeServiceName(name)
		if _, ok := runtimeServiceSet[normalizedName]; ok {
			continue
		}
		if _, ok := runtimeServiceSet[name]; ok {
			continue
		}

		parts := strings.Split(name, "_")
		if len(parts) >= 3 {
			forwardID, err := strconv.ParseInt(parts[0], 10, 64)
			if err == nil && forwardID > 0 && hasUnboundForwardPeerRuntime {
				continue
			}
			if err == nil && forwardID > 0 && !h.forwardExists(forwardID) {
				_, _ = h.sendNodeCommand(nodeID, "DeleteService", map[string]interface{}{"services": []string{name, parts[0] + "_" + parts[1] + "_" + parts[2], parts[0] + "_" + parts[1] + "_" + parts[2] + "_tcp", parts[0] + "_" + parts[1] + "_" + parts[2] + "_udp"}}, false, true)
				continue
			}
		}
		suffix := parts[len(parts)-1]

		switch suffix {
		case "tls":
			tunnelID, err := strconv.ParseInt(parts[0], 10, 64)
			if err != nil || tunnelID <= 0 || h.tunnelExists(tunnelID) {
				continue
			}
			_, _ = h.sendNodeCommand(nodeID, "DeleteService", map[string]interface{}{"services": []string{name}}, false, true)
		case "tcp":
			if len(parts) < 4 {
				continue
			}
			forwardID, err := strconv.ParseInt(parts[0], 10, 64)
			if err == nil && forwardID > 0 && hasUnboundForwardPeerRuntime {
				continue
			}
			if err != nil || forwardID <= 0 || h.forwardExists(forwardID) {
				continue
			}
			base := strings.TrimSuffix(name, "_tcp")
			_, _ = h.sendNodeCommand(nodeID, "DeleteService", map[string]interface{}{"services": []string{base + "_tcp", base + "_udp"}}, false, true)
		}
	}
}

func (h *Handler) cleanOrphanedChains(nodeID int64, chains []namedConfigItem) {
	for _, item := range chains {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}

		idx := strings.LastIndex(name, "_")
		if idx <= 0 || idx >= len(name)-1 {
			continue
		}
		tunnelID, err := strconv.ParseInt(name[idx+1:], 10, 64)
		if err != nil || tunnelID <= 0 || h.tunnelExists(tunnelID) {
			continue
		}
		_, _ = h.sendNodeCommand(nodeID, "DeleteChains", map[string]interface{}{"chain": name}, false, true)
	}
}

func (h *Handler) cleanOrphanedLimiters(nodeID int64, limiters []namedConfigItem) {
	for _, item := range limiters {
		name := strings.TrimSpace(item.Name)
		if name == "" || h.speedLimiterExists(name) {
			continue
		}
		_, _ = h.sendNodeCommand(nodeID, "DeleteLimiters", map[string]interface{}{"limiter": name}, false, true)
	}
}

func (h *Handler) tunnelExists(tunnelID int64) bool {
	ok, _ := h.repo.TunnelExists(tunnelID)
	return ok
}

func (h *Handler) forwardExists(forwardID int64) bool {
	ok, _ := h.repo.ForwardExists(forwardID)
	return ok
}

func (h *Handler) speedLimiterExists(name string) bool {
	if name == "" {
		return false
	}
	id, err := strconv.ParseInt(name, 10, 64)
	if err != nil || id <= 0 {
		return false
	}
	ok, _ := h.repo.SpeedLimitExists(id)
	return ok
}
