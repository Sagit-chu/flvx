package handler

import (
	"encoding/json"
	"errors"
	"log"
	"math"
	"strconv"
	"strings"
	"time"
)

const bytesPerGB int64 = 1024 * 1024 * 1024
const bytesPerMiB int64 = 1024 * 1024

func flowLimitBytes(flowGB, flowMiB int64) int64 {
	if flowMiB > 0 {
		if flowMiB > math.MaxInt64/bytesPerMiB {
			return math.MaxInt64
		}
		return flowMiB * bytesPerMiB
	}
	if flowGB > math.MaxInt64/bytesPerGB {
		return math.MaxInt64
	}
	return flowGB * bytesPerGB
}

type userTunnelPolicy struct {
	ID       int64
	UserID   int64
	TunnelID int64
	Flow     int64
	FlowMiB  int64
	InFlow   int64
	OutFlow  int64
	ExpTime  int64
	Status   int
	Num      int
}

type gostConfigSnapshot struct {
	Services []namedConfigItem `json:"services"`
	Chains   []namedConfigItem `json:"chains"`
	Limiters []namedConfigItem `json:"limiters"`
}

type namedConfigItem struct {
	Name    string `json:"name"`
	Limiter string `json:"limiter,omitempty"`
	Handler *struct {
		Chain string `json:"chain"`
	} `json:"handler,omitempty"`
}

func (h *Handler) processFlowItem(nodeID int64, item flowItem) {
	if h == nil || h.repo == nil || nodeID <= 0 {
		return
	}
	metas, err := h.repo.GetFlowUploadForwardMetas(collectFlowUploadForwardIDs([]flowItem{item}))
	if err != nil {
		metas = nil
	}
	h.applyFlowUploadBatch(nodeID, h.buildNodeFlowUploadBatch(nodeID, []flowItem{item}, metas), time.Now())
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

func (h *Handler) processPeerShareFlow(nodeID, runtimeID int64, item flowItem) {
	if h == nil || h.repo == nil || nodeID <= 0 || runtimeID <= 0 {
		return
	}
	runtime, err := h.repo.GetPeerShareRuntimeByID(runtimeID)
	if err != nil || runtime == nil || runtime.NodeID != nodeID || runtime.Status != 1 {
		return
	}
	h.addPeerShareFlow(nodeID, runtime.ShareID, item.D+item.U)
}

func (h *Handler) processPeerShareFlowFromForward(forwardID int64, nodeID int64, serviceName string, item flowItem) {
	if h == nil || h.repo == nil || forwardID <= 0 || nodeID <= 0 {
		return
	}
	// Prefer the reporting node's explicit shared ownership over a coincidentally
	// equal local forward ID. Never fall back to a service on another node.
	runtimes, err := h.repo.ListActiveForwardPeerShareRuntimesByNode(nodeID)
	if err != nil {
		return
	}
	for _, runtime := range runtimes {
		if normalizeForwardRuntimeServiceName(runtime.ServiceName) == normalizeForwardRuntimeServiceName(serviceName) {
			h.processPeerShareFlowByServiceName(nodeID, serviceName, item)
			return
		}
	}
	forward, err := h.getForwardRecord(forwardID)
	if err != nil || forward == nil {
		return
	}
	_, userID, _, ok := parseFlowServiceIDs(serviceName)
	if !ok || userID != forward.UserID {
		return
	}
	tunnelName, err := h.repo.GetTunnelName(forward.TunnelID)
	if err != nil {
		return
	}
	shareID, ok := parsePeerShareIDFromFederationTunnelName(tunnelName)
	if ok {
		h.addPeerShareFlow(nodeID, shareID, item.D+item.U)
	}
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
	if h == nil || h.repo == nil || nodeID <= 0 || strings.TrimSpace(serviceName) == "" {
		return
	}
	runtimes, err := h.repo.ListActiveForwardPeerShareRuntimesByNode(nodeID)
	if err != nil {
		return
	}
	var shareID int64
	for _, runtime := range runtimes {
		if normalizeForwardRuntimeServiceName(runtime.ServiceName) != normalizeForwardRuntimeServiceName(serviceName) {
			continue
		}
		if shareID != 0 {
			log.Printf("ambiguous peer share runtime service=%s node_id=%d", serviceName, nodeID)
			return
		}
		shareID = runtime.ShareID
	}
	if shareID > 0 {
		h.addPeerShareFlow(nodeID, shareID, item.D+item.U)
	}
}

func (h *Handler) enforcePeerShareFlowLimit(shareID int64) {
	if h == nil || h.repo == nil || shareID <= 0 {
		return
	}
	if err := h.cleanupPeerShareRuntimes(shareID); err != nil {
		log.Printf("peer share quota cleanup pending share_id=%d err=%v", shareID, err)
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

	flowLimit := flowLimitBytes(user.Flow, user.FlowMiB)
	current := user.InFlow + user.OutFlow
	if flowLimit < current {
		return errors.New("流量已超额，禁止开启转发")
	}
	if err := h.ensureUserForwardAllowedByQuota(userID, now); err != nil {
		return err
	}

	if user.Num > 0 {
		currentForwardCount, err := h.repo.CountActiveForwardsByUser(userID)
		if err != nil {
			return err
		}
		if currentForwardCount >= int64(user.Num) {
			return errors.New("转发数量已达上限")
		}
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

	utFlowLimit := flowLimitBytes(policy.Flow, policy.FlowMiB)
	utCurrent := policy.InFlow + policy.OutFlow
	if utCurrent >= utFlowLimit {
		return errors.New("该隧道流量已超额，禁止开启转发")
	}

	if policy.Num > 0 {
		currentTunnelForwardCount, err := h.repo.CountActiveForwardsByUserTunnel(userID, tunnelID)
		if err != nil {
			return err
		}
		if currentTunnelForwardCount >= int64(policy.Num) {
			return errors.New("该隧道转发数量已达上限")
		}
	}

	return nil
}

func (h *Handler) shouldPauseUser(userID int64, now int64) bool {
	user, err := h.repo.GetUserByID(userID)
	if err != nil || user == nil {
		return false
	}

	flowLimit := flowLimitBytes(user.Flow, user.FlowMiB)
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

	flowLimit := flowLimitBytes(policy.Flow, policy.FlowMiB)
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
		Flow: ut.Flow, FlowMiB: ut.FlowMiB, InFlow: ut.InFlow, OutFlow: ut.OutFlow,
		ExpTime: ut.ExpTime, Status: ut.Status, Num: ut.Num,
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

	protection, err := h.loadForwardServiceProtection(nodeID)
	if err != nil {
		return
	}
	h.cleanOrphanedServicesWithProtection(nodeID, snapshot.Services, protection)
	// Dependencies are sent before services. A pending shared reservation may
	// therefore have chains/limiters that are not referenced in this snapshot yet.
	if protection.unbound {
		return
	}
	chainsInUse := make(map[string]struct{})
	limitersInUse := make(map[string]struct{})
	for _, service := range snapshot.Services {
		if service.Handler != nil {
			if chain := strings.TrimSpace(service.Handler.Chain); chain != "" {
				chainsInUse[chain] = struct{}{}
			}
		}
		for _, limiter := range strings.Split(service.Limiter, ",") {
			if limiter = strings.TrimSpace(limiter); limiter != "" {
				limitersInUse[limiter] = struct{}{}
			}
		}
	}
	// Keep dependencies referenced by the reported services, even when their
	// IDs belong to a different panel. Orphan dependencies can be collected on
	// the next report after their services have actually disappeared.
	h.cleanOrphanedChains(nodeID, snapshot.Chains, chainsInUse)
	h.cleanOrphanedLimiters(nodeID, snapshot.Limiters, limitersInUse)
}

type forwardServiceProtection struct {
	sharedNames map[string]struct{}
	unbound     bool
}

// Shared forward IDs belong to another panel and need not exist in our forward
// table. Use the same node-scoped ownership check for config and flow reports.
func (h *Handler) loadForwardServiceProtection(nodeID int64) (forwardServiceProtection, error) {
	protection := forwardServiceProtection{sharedNames: make(map[string]struct{})}
	// Read names and pending bindings in one snapshot, so a concurrent bind
	// cannot fall between two queries and disappear from both protections.
	runtimes, err := h.repo.ListActiveForwardPeerShareRuntimesByNode(nodeID)
	if err != nil {
		return protection, err
	}
	minUpdatedTime := time.Now().Add(-10 * time.Minute).UnixMilli()
	for _, runtime := range runtimes {
		serviceName := normalizeForwardRuntimeServiceName(runtime.ServiceName)
		if serviceName == "" {
			if runtime.Applied == 0 && runtime.UpdatedTime >= minUpdatedTime {
				protection.unbound = true
			}
			continue
		}
		protection.sharedNames[serviceName] = struct{}{}
	}
	resources, err := h.repo.ListPeerShareResourcesByNode(nodeID)
	if err != nil {
		return protection, err
	}
	for _, resource := range resources {
		if base := normalizeForwardRuntimeServiceName(resource.LegacyServiceBase); base != "" {
			protection.sharedNames[base] = struct{}{}
		}
	}
	return protection, nil
}

func (p forwardServiceProtection) preserves(serviceName string) bool {
	_, shared := p.sharedNames[normalizeForwardRuntimeServiceName(serviceName)]
	return shared || p.unbound
}

func (h *Handler) cleanOrphanedServices(nodeID int64, services []namedConfigItem) {
	if h == nil || h.repo == nil || nodeID <= 0 {
		return
	}
	protection, err := h.loadForwardServiceProtection(nodeID)
	if err != nil {
		// A failed ownership lookup must never authorize deletion.
		return
	}
	h.cleanOrphanedServicesWithProtection(nodeID, services, protection)
}

func (h *Handler) cleanOrphanedServicesWithProtection(nodeID int64, services []namedConfigItem, protection forwardServiceProtection) {
	for _, item := range services {
		name := strings.TrimSpace(item.Name)
		if name == "" || name == "web_api" {
			continue
		}
		if strings.HasPrefix(name, "fed_svc_") || strings.HasPrefix(name, "peer-share-") {
			continue
		}
		if _, ok := protection.sharedNames[normalizeForwardRuntimeServiceName(name)]; ok {
			continue
		}

		parts := strings.Split(name, "_")
		if len(parts) == 2 && parts[0] == "tunnel" {
			tunnelID, err := strconv.ParseInt(parts[1], 10, 64)
			if err == nil && tunnelID > 0 && !h.tunnelExists(tunnelID) {
				_, _ = h.sendNodeCommand(nodeID, "DeleteService", map[string]interface{}{"services": []string{name}}, false, true)
			}
			continue
		}

		if _, _, _, ok := parseFlowServiceIDs(name); ok {
			h.deleteOrphanedForwardService(nodeID, name, protection)
			continue
		}
		suffix := parts[len(parts)-1]

		switch suffix {
		case "tls", "kcp", "wss", "mtls", "mwss", "mtcp":
			tunnelID, err := strconv.ParseInt(parts[0], 10, 64)
			if err != nil || tunnelID <= 0 || h.tunnelExists(tunnelID) {
				continue
			}
			_, _ = h.sendNodeCommand(nodeID, "DeleteService", map[string]interface{}{"services": []string{name}}, false, true)
		case "tcp":
			if len(parts) < 4 {
				tunnelID, err := strconv.ParseInt(parts[0], 10, 64)
				if err == nil && tunnelID > 0 && !h.tunnelExists(tunnelID) {
					_, _ = h.sendNodeCommand(nodeID, "DeleteService", map[string]interface{}{"services": []string{name}}, false, true)
				}
				continue
			}
		}
	}
}

func (h *Handler) cleanOrphanedChains(nodeID int64, chains []namedConfigItem, inUse map[string]struct{}) {
	for _, item := range chains {
		name := strings.TrimSpace(item.Name)
		if name == "" || strings.HasPrefix(name, "fed_chain_") || strings.HasPrefix(name, "peer-share-") {
			continue
		}
		if _, ok := inUse[name]; ok {
			continue
		}

		idx := strings.LastIndex(name, "_")
		if idx <= 0 || idx >= len(name)-1 {
			continue
		}
		tunnelID, err := strconv.ParseInt(name[idx+1:], 10, 64)
		if err != nil || tunnelID <= 0 {
			continue
		}
		exists, err := h.repo.TunnelExists(tunnelID)
		if err != nil || exists {
			continue
		}
		_, _ = h.sendNodeCommand(nodeID, "DeleteChains", map[string]interface{}{"chain": name}, false, true)
	}
}

func (h *Handler) cleanOrphanedLimiters(nodeID int64, limiters []namedConfigItem, inUse map[string]struct{}) {
	for _, item := range limiters {
		name := strings.TrimSpace(item.Name)
		if name == "" || strings.HasPrefix(name, "peer-share-") {
			continue
		}
		if _, ok := inUse[name]; ok {
			continue
		}
		exists, err := h.lookupSpeedLimiter(name)
		if err != nil || exists {
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

func (h *Handler) sendDeleteOrphanedForwardService(nodeID int64, serviceName string) {
	h.sendDeleteOrphanedForwardServices(nodeID, []string{serviceName})
}

func (h *Handler) sendDeleteOrphanedForwardServices(nodeID int64, serviceNames []string) {
	if h == nil || h.repo == nil || nodeID <= 0 || len(serviceNames) == 0 {
		return
	}
	protection, err := h.loadForwardServiceProtection(nodeID)
	if err != nil {
		return
	}
	seen := make(map[string]struct{}, len(serviceNames))
	for _, serviceName := range serviceNames {
		serviceName = normalizeForwardRuntimeServiceName(serviceName)
		if _, ok := seen[serviceName]; ok {
			continue
		}
		seen[serviceName] = struct{}{}
		h.deleteOrphanedForwardService(nodeID, serviceName, protection)
	}
}

func (h *Handler) deleteOrphanedForwardService(nodeID int64, serviceName string, protection forwardServiceProtection) {
	forwardID, _, _, ok := parseFlowServiceIDs(serviceName)
	if !ok || protection.preserves(serviceName) {
		return
	}
	parts := strings.Split(serviceName, "_")
	base := parts[0] + "_" + parts[1] + "_" + parts[2]
	// Parsing accepts legacy suffixes, while deletion targets the entire base
	// family. Verify the actual targets cannot include a protected share.
	if protection.preserves(base) {
		return
	}
	// Batch metadata can be missing after a read failure, or stale by the time
	// cleanup runs. Confirm absence before issuing a destructive command.
	exists, err := h.repo.ForwardExists(forwardID)
	if err != nil || exists {
		return
	}
	_, _ = h.sendNodeCommand(nodeID, "DeleteService", map[string]interface{}{
		"services": buildForwardServiceDeleteNames([]string{base}),
	}, false, true)
}

func (h *Handler) speedLimiterExists(name string) bool {
	exists, _ := h.lookupSpeedLimiter(name)
	return exists
}

func (h *Handler) lookupSpeedLimiter(name string) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, nil
	}

	const forwardRulePrefix = "rule_traffic_limit_"
	if strings.HasPrefix(name, forwardRulePrefix) {
		forwardID, err := strconv.ParseInt(strings.TrimPrefix(name, forwardRulePrefix), 10, 64)
		if err != nil || forwardID <= 0 {
			return false, nil
		}
		forward, err := h.getForwardRecord(forwardID)
		if errors.Is(err, errForwardNotFound) {
			return false, nil
		}
		return forward != nil && forward.IPSpeedID.Valid && forward.IPSpeedID.Int64 > 0, err
	}

	id, err := strconv.ParseInt(name, 10, 64)
	if err != nil || id <= 0 {
		return false, nil
	}
	return h.repo.SpeedLimitExists(id)
}
