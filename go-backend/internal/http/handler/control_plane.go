package handler

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-backend/internal/http/client"
	backendruntime "go-backend/internal/runtime"
	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
	"go-backend/internal/ws"
)

var errForwardNotFound = errors.New("forward not found")

type forwardRecord = model.ForwardRecord
type tunnelRecord = model.TunnelRecord
type forwardPortRecord = model.ForwardPortRecord
type nodeRecord = model.NodeRecord

type chainNodeRecord = model.ChainNodeRecord

type diagnosisTarget struct {
	Address string
	IP      string
	Port    int
}

type diagnosisWorkItem struct {
	fromNodeID   int64
	targetIP     string
	targetPort   int
	description  string
	metadata     map[string]interface{}
	toNode       chainNodeRecord
	hasChainHop  bool
	ipPreference string
}

type diagnosisExecOptions struct {
	commandTimeout time.Duration
	pingTimeoutMS  int
	timeoutMessage string
}

type diagnosisProgress struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Success   int `json:"success"`
	Failed    int `json:"failed"`
}

type diagnosisItemEmitter func(index int, item map[string]interface{}, progress diagnosisProgress)

func (h *Handler) buildDiagnosisStreamStartItems(workItems []diagnosisWorkItem) []map[string]interface{} {
	if len(workItems) == 0 {
		return []map[string]interface{}{}
	}

	nodeCache := map[int64]*nodeRecord{}
	items := make([]map[string]interface{}, 0, len(workItems))
	for _, workItem := range workItems {
		targetIP := strings.TrimSpace(workItem.targetIP)
		targetPort := workItem.targetPort
		if workItem.hasChainHop {
			fromNode, _ := h.cachedNode(nodeCache, workItem.fromNodeID)
			targetNode, err := h.cachedNode(nodeCache, workItem.toNode.NodeID)
			if err == nil {
				resolvedIP, resolvedPort, resolveErr := resolveChainProbeTarget(fromNode, targetNode, workItem.toNode.Port, workItem.ipPreference, workItem.toNode.ConnectIP)
				if resolveErr == nil {
					targetIP = resolvedIP
					targetPort = resolvedPort
				}
			}
		}
		if targetPort <= 0 {
			targetPort = 443
		}

		nodeName := fmt.Sprintf("node_%d", workItem.fromNodeID)
		if node, err := h.cachedNode(nodeCache, workItem.fromNodeID); err == nil && strings.TrimSpace(node.Name) != "" {
			nodeName = node.Name
		}

		item := map[string]interface{}{
			"success":     false,
			"diagnosing":  true,
			"description": workItem.description,
			"nodeName":    nodeName,
			"nodeId":      strconv.FormatInt(workItem.fromNodeID, 10),
			"targetIp":    targetIP,
			"targetPort":  targetPort,
			"message":     "诊断中...",
		}
		for key, value := range workItem.metadata {
			item[key] = value
		}
		items = append(items, item)
	}

	return items
}

const diagnosisMaxConcurrency = 8

const (
	defaultNodeCommandTimeout  = 6 * time.Second
	diagnosisCommandTimeout    = 30 * time.Second
	diagnosisRequestTimeout    = 2 * time.Minute
	diagnosisCommandTimeoutMsg = "诊断超时（30秒）"
	diagnosisRequestTimeoutMsg = "诊断超时（2分钟）"
)

func (h *Handler) resolveForwardAccess(r *http.Request, forwardID int64) (*forwardRecord, int64, int, error) {
	userID, roleID, err := userRoleFromRequest(r)
	if err != nil {
		return nil, 0, 0, err
	}
	forward, err := h.ensureForwardAccessByActor(userID, roleID, forwardID)
	if err != nil {
		return nil, userID, roleID, err
	}
	return forward, userID, roleID, nil
}

func (h *Handler) ensureForwardAccessByActor(actorUserID int64, actorRole int, forwardID int64) (*forwardRecord, error) {
	forward, err := h.getForwardRecord(forwardID)
	if err != nil {
		return nil, err
	}
	if actorRole != 0 && forward.UserID != actorUserID {
		return nil, errForwardNotFound
	}
	return forward, nil
}

func (h *Handler) ensureTunnelPermission(userID int64, roleID int, tunnelID int64) error {
	if roleID == 0 {
		return nil
	}
	ok, err := h.repo.UserTunnelExistsByUserAndTunnel(userID, tunnelID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("你没有该隧道的权限")
	}
	return nil
}

func (h *Handler) getForwardRecord(forwardID int64) (*forwardRecord, error) {
	fr, err := h.repo.GetForwardRecord(forwardID)
	if err != nil {
		return nil, err
	}
	if fr == nil {
		return nil, errForwardNotFound
	}
	return fr, nil
}

func (h *Handler) getTunnelRecord(tunnelID int64) (*tunnelRecord, error) {
	tr, err := h.repo.GetTunnelRecord(tunnelID)
	if err != nil {
		return nil, err
	}
	if tr == nil {
		return nil, errors.New("隧道不存在")
	}
	return tr, nil
}

func (h *Handler) listForwardsByTunnel(tunnelID int64) ([]forwardRecord, error) {
	return h.repo.ListForwardsByTunnel(tunnelID)
}

func (h *Handler) listForwardPorts(forwardID int64) ([]forwardPortRecord, error) {
	return h.repo.ListForwardPorts(forwardID)
}

func (h *Handler) isTunnelSelectedTLSProtocol(tunnelID int64) (bool, error) {
	protocol, err := h.repo.GetTunnelOutProtocol(tunnelID)
	if err != nil {
		return false, err
	}
	return isTLSTunnelProtocol(protocol), nil
}

func (h *Handler) getNodeRecord(nodeID int64) (*nodeRecord, error) {
	n, err := h.repo.GetNodeRecord(nodeID)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, errors.New("节点不存在")
	}
	return n, nil
}

func (h *Handler) resolveUserTunnelAndLimiter(userID, tunnelID int64) (int64, *int64, *int, error) {
	info, err := h.repo.ResolveUserTunnelAndLimiter(userID, tunnelID)
	if err != nil {
		return 0, nil, nil, err
	}
	if info == nil {
		return 0, nil, nil, nil
	}
	return info.UserTunnelID, info.LimiterID, info.Speed, nil
}

func (h *Handler) listUserTunnelIDs(userID, tunnelID int64) ([]int64, error) {
	return h.repo.ListUserTunnelIDs(userID, tunnelID)
}

func (h *Handler) listUserTunnelIDsByUser(userID int64) ([]int64, error) {
	return h.repo.ListUserTunnelIDsByUser(userID)
}

func (h *Handler) syncForwardServices(forward *forwardRecord, method string, allowFallbackAdd bool) error {
	_, err := h.syncForwardServicesWithWarnings(forward, method, allowFallbackAdd)
	return err
}

func (h *Handler) syncForwardServicesWithWarnings(forward *forwardRecord, method string, allowFallbackAdd bool) ([]string, error) {
	fmt.Printf("DEBUG: syncForwardServicesWithWarnings start forwardID=%d\n", forward.ID)
	if h == nil || forward == nil {
		return nil, errors.New("invalid forward sync context")
	}

	tunnel, err := h.getTunnelRecord(forward.TunnelID)
	if err != nil {
		return nil, err
	}
	ports, err := h.listForwardPorts(forward.ID)
	if err != nil {
		return nil, err
	}
	if len(ports) == 0 {
		return nil, errors.New("转发入口端口不存在")
	}
	warnings := make([]string, 0)

	// Resolve user tunnel first so runtime service name can carry the real user_tunnel id.
	userTunnelID, utLimiterID, utSpeed, err := h.resolveUserTunnelAndLimiter(forward.UserID, forward.TunnelID)
	if err != nil {
		return nil, err
	}

	// Determine limiter from forward's SpeedID first, fallback to UserTunnel's limiter
	var limiterID *int64
	var speed *int

	if forward.SpeedID.Valid && forward.SpeedID.Int64 > 0 {
		// Forward has its own speed limit
		speedVal, err := h.repo.GetSpeedLimitSpeed(forward.SpeedID.Int64)
		if err == nil && speedVal > 0 {
			limiterID = &forward.SpeedID.Int64
			speed = &speedVal
		}
	}

	if limiterID == nil {
		// Fall back to UserTunnel speed limit
		limiterID = utLimiterID
		speed = utSpeed
	}

	serviceBase := buildForwardServiceBaseWithResolvedUserTunnel(forward.ID, forward.UserID, userTunnelID)
	tunnelTLSProtocol, err := h.isTunnelSelectedTLSProtocol(forward.TunnelID)
	if err != nil {
		return nil, err
	}

	for _, fp := range ports {
		fmt.Printf("DEBUG: syncForwardServices nodeID=%d port=%d\n", fp.NodeID, fp.Port)
		if limiterID != nil && speed != nil {
			if err := h.ensureLimiterOnNode(fp.NodeID, *limiterID, *speed); err != nil {
				// If the limiter push fails because the node is offline, skip it with a warning
				if isNodeOfflineOrTimeoutError(err) {
					node, _ := h.getNodeRecord(fp.NodeID)
					nodeName := fmt.Sprintf("%d", fp.NodeID)
					if node != nil && strings.TrimSpace(node.Name) != "" {
						nodeName = strings.TrimSpace(node.Name)
					}
					warnings = append(warnings, fmt.Sprintf("节点 %s 不在线，已跳过下发", nodeName))
					continue
				}
				return nil, err
			}
		}

		node, err := h.getNodeRecord(fp.NodeID)
		if err != nil {
			return nil, err
		}
		services := buildForwardServiceConfigs(serviceBase, forward, tunnel, node, fp.Port, strings.TrimSpace(fp.InIP), limiterID, tunnelTLSProtocol)
		_, err = h.sendNodeCommand(node.ID, method, services, true, false)
		if err != nil && allowFallbackAdd && method == "UpdateService" {
			if isNotFoundError(err) {
				if delErr := h.deleteForwardServicesOnNode(forward, node.ID); delErr != nil && !isNotFoundError(delErr) {
					return warnings, fmt.Errorf("节点 %s 清理旧服务失败: %w", node.Name, delErr)
				}
			}
			_, err = h.sendNodeCommand(node.ID, "AddService", services, true, false)
		}
		if err != nil && strings.EqualFold(strings.TrimSpace(method), "UpdateService") && isAddressAlreadyInUseError(err) {
			err = h.rebindForwardServiceOnSelfOccupiedPort(forward, node, fp.Port, services)
		}
		if err != nil && strings.EqualFold(strings.TrimSpace(method), "UpdateService") && isCannotAssignRequestedAddressError(err) {
			var warning string
			warning, err = h.fallbackForwardPortToDefaultBind(forward, tunnel, node, fp, serviceBase, limiterID, tunnelTLSProtocol)
			if err == nil && warning != "" {
				warnings = append(warnings, warning)
			}
		}
		// When a node is offline, skip it with a warning instead of failing.
		// This lets users modify forward rules even when some entry nodes are down.
		if err != nil && isNodeOfflineOrTimeoutError(err) {
			warnings = append(warnings, fmt.Sprintf("节点 %s 不在线，已跳过下发", node.Name))
			continue
		}
		if err != nil {
			return warnings, fmt.Errorf("节点 %s 下发失败: %w", node.Name, err)
		}
	}

	// Keep paused forwards paused after UpdateService/AddService, since agent-side UpdateService
	// always restarts services.
	if forward.Status != 1 {
		if err := h.controlForwardServices(forward, "PauseService", false); err != nil {
			return warnings, err
		}
	}
	return warnings, nil
}

func (h *Handler) fallbackForwardPortToDefaultBind(forward *forwardRecord, tunnel *tunnelRecord, node *nodeRecord, fp forwardPortRecord, serviceBase string, limiterID *int64, tunnelTLSProtocol bool) (string, error) {
	if h == nil || forward == nil || tunnel == nil || node == nil {
		return "", errors.New("invalid bind fallback context")
	}
	if fp.Port <= 0 {
		return "", errors.New("invalid forward port")
	}
	explicitBindIP := strings.TrimSpace(fp.InIP)
	if explicitBindIP == "" {
		return "", errors.New("default bind address cannot be assigned")
	}

	if err := h.deleteForwardServicesOnNode(forward, node.ID); err != nil {
		return "", err
	}

	time.Sleep(150 * time.Millisecond)
	defaultServices := buildForwardServiceConfigs(serviceBase, forward, tunnel, node, fp.Port, "", limiterID, tunnelTLSProtocol)
	if _, err := h.sendNodeCommand(node.ID, "AddService", defaultServices, true, false); err != nil {
		return "", err
	}
	if err := h.repo.UpdateForwardPortBindIP(forward.ID, node.ID, fp.Port, ""); err != nil {
		return "", err
	}

	warning := fmt.Sprintf("节点 %s 监听IP %s 不在主机网卡地址中，已自动回退为默认监听IP", strings.TrimSpace(node.Name), explicitBindIP)
	return warning, nil
}

func (h *Handler) rebindForwardServiceOnSelfOccupiedPort(forward *forwardRecord, node *nodeRecord, port int, services []map[string]interface{}) error {
	if h == nil || forward == nil || node == nil {
		return errors.New("invalid self-occupy rebind context")
	}
	if port <= 0 {
		return errors.New("invalid forward port")
	}

	hasOtherForward, err := h.repo.HasOtherForwardOnNodePort(node.ID, port, forward.ID)
	if err != nil {
		return err
	}
	if hasOtherForward {
		return fmt.Errorf("端口 %d 已被其他转发占用", port)
	}

	bases, err := h.forwardServiceBaseCandidates(forward)
	if err != nil {
		return err
	}

	if err := h.deleteForwardServiceBasesOnNode(node.ID, bases); err != nil {
		return err
	}

	time.Sleep(150 * time.Millisecond)

	_, err = h.sendNodeCommand(node.ID, "AddService", services, true, false)
	if err != nil {
		return err
	}

	return nil
}

func (h *Handler) deleteForwardServicesOnNode(forward *forwardRecord, nodeID int64) error {
	if h == nil || forward == nil {
		return errors.New("invalid forward delete context")
	}
	bases, err := h.forwardServiceBaseCandidates(forward)
	if err != nil {
		return err
	}
	return h.deleteForwardServiceBasesOnNode(nodeID, bases)

}

func (h *Handler) forwardServiceBaseCandidates(forward *forwardRecord) ([]string, error) {
	if h == nil || forward == nil {
		return nil, errors.New("invalid forward service base context")
	}
	userTunnelID, _, _, err := h.resolveUserTunnelAndLimiter(forward.UserID, forward.TunnelID)
	if err != nil {
		return nil, err
	}
	userTunnelIDs, err := h.listUserTunnelIDs(forward.UserID, forward.TunnelID)
	if err != nil {
		return nil, err
	}
	allUserTunnelIDs, err := h.listUserTunnelIDsByUser(forward.UserID)
	if err != nil {
		return nil, err
	}
	candidateTunnelIDs := make([]int64, 0, len(userTunnelIDs)+len(allUserTunnelIDs))
	candidateTunnelIDs = append(candidateTunnelIDs, userTunnelIDs...)
	candidateTunnelIDs = append(candidateTunnelIDs, allUserTunnelIDs...)
	return buildForwardServiceBaseCandidates(forward.ID, forward.UserID, userTunnelID, candidateTunnelIDs), nil

}

func (h *Handler) deleteForwardServiceBasesOnNode(nodeID int64, bases []string) error {
	return deleteForwardServiceCandidates(bases, func(name string) error {
		payload := map[string]interface{}{
			"services": []string{name},
		}
		_, err := h.sendNodeCommand(nodeID, "DeleteService", payload, false, false)
		return err
	})
}

func (h *Handler) controlForwardServices(forward *forwardRecord, commandType string, tolerateNotFound bool) error {
	if h == nil || forward == nil {
		return errors.New("invalid forward control context")
	}
	ports, err := h.listForwardPorts(forward.ID)
	if err != nil {
		return err
	}
	if len(ports) == 0 {
		return nil
	}
	userTunnelID, _, _, err := h.resolveUserTunnelAndLimiter(forward.UserID, forward.TunnelID)
	if err != nil {
		return err
	}
	userTunnelIDs, err := h.listUserTunnelIDs(forward.UserID, forward.TunnelID)
	if err != nil {
		return err
	}
	allUserTunnelIDs, err := h.listUserTunnelIDsByUser(forward.UserID)
	if err != nil {
		return err
	}
	candidateTunnelIDs := make([]int64, 0, len(userTunnelIDs)+len(allUserTunnelIDs))
	candidateTunnelIDs = append(candidateTunnelIDs, userTunnelIDs...)
	candidateTunnelIDs = append(candidateTunnelIDs, allUserTunnelIDs...)
	bases := buildForwardServiceBaseCandidates(forward.ID, forward.UserID, userTunnelID, candidateTunnelIDs)
	seen := map[int64]struct{}{}
	healed := false
	for _, fp := range ports {
		if _, ok := seen[fp.NodeID]; ok {
			continue
		}
		seen[fp.NodeID] = struct{}{}

		nodeHandled, lastNotFoundErr, err := h.controlForwardServicesOnNode(fp.NodeID, bases, commandType)
		if err != nil {
			return err
		}

		if !nodeHandled && lastNotFoundErr != nil && !healed && shouldSelfHealForwardServiceControl(commandType) {
			if healErr := h.syncForwardServices(forward, "UpdateService", true); healErr != nil {
				return healErr
			}
			healed = true
			nodeHandled, lastNotFoundErr, err = h.controlForwardServicesOnNode(fp.NodeID, bases, commandType)
			if err != nil {
				return err
			}
		}

		if nodeHandled {
			continue
		}
		if tolerateNotFound {
			continue
		}
		if lastNotFoundErr != nil {
			return lastNotFoundErr
		}
		return errors.New("service control failed")
	}
	return nil
}

func (h *Handler) controlForwardServicesOnNode(nodeID int64, bases []string, commandType string) (bool, error, error) {
	return controlForwardServiceCommand(bases, commandType, func(name string) error {
		switch strings.ToLower(strings.TrimSpace(commandType)) {
		case "pauseservice":
			return h.pauseForwardOnNode(context.Background(), nodeID, name)
		case "resumeservice":
			return h.resumeForwardOnNode(context.Background(), nodeID, name)
		default:
			payload := map[string]interface{}{
				"services": []string{name},
			}
			_, err := h.sendNodeCommand(nodeID, commandType, payload, false, false)
			return err
		}
	})
}

func (h *Handler) currentRuntimeClient() (backendruntime.RuntimeClient, error) {
	if h == nil || h.repo == nil {
		return nil, errors.New("repository not initialized")
	}
	engine, err := h.repo.GetRuntimeEngine()
	if err != nil {
		return nil, err
	}
	client := h.runtimeClients[backendruntime.Engine(engine)]
	if client == nil {
		return nil, fmt.Errorf("runtime client unavailable for engine %q", engine)
	}
	return client, nil
}

func (h *Handler) pauseForwardOnNode(ctx context.Context, nodeID int64, serviceName string) error {
	return h.controlForwardOnNode(ctx, nodeID, serviceName, func(client backendruntime.RuntimeClient, node repo.Node, services []string) error {
		return client.PauseServices(ctx, node, services)
	})
}

func (h *Handler) resumeForwardOnNode(ctx context.Context, nodeID int64, serviceName string) error {
	return h.controlForwardOnNode(ctx, nodeID, serviceName, func(client backendruntime.RuntimeClient, node repo.Node, services []string) error {
		return client.ResumeServices(ctx, node, services)
	})
}

func (h *Handler) controlForwardOnNode(ctx context.Context, nodeID int64, serviceName string, control func(backendruntime.RuntimeClient, repo.Node, []string) error) error {
	if h == nil {
		return errors.New("handler not initialized")
	}
	node, err := h.repo.GetNodeByID(nodeID)
	if err != nil {
		return err
	}
	if node == nil {
		return errors.New("节点不存在")
	}
	client, err := h.currentRuntimeClient()
	if err != nil {
		return err
	}
	return control(client, *node, []string{serviceName})
}

func controlForwardServiceCommand(bases []string, commandType string, send func(name string) error) (bool, error, error) {
	var lastNotFoundErr error
	for _, base := range bases {
		variants := []string{base + "_tcp", base + "_udp"}
		if shouldTryLegacySingleService(commandType) || strings.EqualFold(strings.TrimSpace(commandType), "DeleteService") {
			variants = append(variants, base)
		}

		candidateHandled := false
		for _, name := range variants {
			err := send(name)
			if err == nil {
				candidateHandled = true
				continue
			}
			if !isNotFoundError(err) {
				return false, lastNotFoundErr, err
			}
			lastNotFoundErr = err
		}

		if candidateHandled {
			return true, nil, nil
		}
	}
	return false, lastNotFoundErr, nil
}

func deleteForwardServiceCandidates(bases []string, send func(name string) error) error {
	for _, base := range bases {
		for _, name := range append([]string{base + "_tcp", base + "_udp", base}, []string{}...) {
			err := send(name)
			if err == nil {
				continue
			}
			if isNotFoundError(err) {
				continue
			}
			return err
		}
	}
	return nil
}

func shouldSelfHealForwardServiceControl(commandType string) bool {
	cmd := strings.ToLower(strings.TrimSpace(commandType))
	return cmd == "pauseservice" || cmd == "resumeservice"
}

func (h *Handler) applyNodeProtocolChange(nodeID int64, httpVal, tlsVal, socksVal int) error {
	_, err := h.sendNodeCommand(nodeID, "SetProtocol", map[string]interface{}{
		"http":  httpVal,
		"tls":   tlsVal,
		"socks": socksVal,
	}, false, false)
	return err
}

func (h *Handler) sendNodeCommand(nodeID int64, commandType string, data interface{}, tolerateExists bool, tolerateNotFound bool) (ws.CommandResult, error) {
	return h.sendNodeCommandWithTimeout(nodeID, commandType, data, defaultNodeCommandTimeout, tolerateExists, tolerateNotFound)
}

func (h *Handler) sendNodeCommandWithTimeout(nodeID int64, commandType string, data interface{}, timeout time.Duration, tolerateExists bool, tolerateNotFound bool) (ws.CommandResult, error) {
	var (
		result ws.CommandResult
		err    error
	)
	if timeout <= 0 {
		timeout = defaultNodeCommandTimeout
	}

	node, nodeErr := h.getNodeRecord(nodeID)
	if nodeErr == nil && node != nil && node.IsRemote == 1 {
		result, err = h.sendRemoteNodeCommandWithTimeout(node, commandType, data, timeout)
	} else {
		result, err = h.wsServer.SendCommand(nodeID, commandType, data, timeout)
	}
	if err == nil {
		return result, nil
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if tolerateExists {
		if isAlreadyExistsMessage(msg) {
			return result, nil
		}
	}
	if tolerateNotFound {
		if strings.Contains(msg, "not found") || strings.Contains(msg, "不存在") {
			return result, nil
		}
	}
	return result, err
}

func (h *Handler) sendRemoteNodeCommand(node *nodeRecord, commandType string, data interface{}) (ws.CommandResult, error) {
	return h.sendRemoteNodeCommandWithTimeout(node, commandType, data, 0)
}

func (h *Handler) sendRemoteNodeCommandWithTimeout(node *nodeRecord, commandType string, data interface{}, timeout time.Duration) (ws.CommandResult, error) {
	if node == nil {
		return ws.CommandResult{}, errors.New("节点不存在")
	}
	remoteURL := strings.TrimSpace(node.RemoteURL)
	remoteToken := strings.TrimSpace(node.RemoteToken)
	if remoteURL == "" || remoteToken == "" {
		return ws.CommandResult{}, errors.New("远程节点缺少共享配置")
	}

	fc := client.NewFederationClient()
	if timeout > 0 {
		fc = client.NewFederationClientWithTimeout(timeout)
	}
	res, err := fc.Command(remoteURL, remoteToken, h.federationLocalDomain(), client.RuntimeNodeCommandRequest{
		CommandType: commandType,
		Data:        data,
	})
	if err != nil {
		return ws.CommandResult{}, err
	}
	if res == nil {
		return ws.CommandResult{}, errors.New("远程节点未返回命令结果")
	}

	result := ws.CommandResult{
		Type:    res.Type,
		Success: res.Success,
		Message: res.Message,
		Data:    res.Data,
	}
	if !result.Success {
		msg := strings.TrimSpace(result.Message)
		if msg == "" {
			msg = "命令执行失败"
		}
		return result, errors.New(msg)
	}
	return result, nil
}

func (h *Handler) diagnoseForwardRuntime(ctx context.Context, forward *forwardRecord) (map[string]interface{}, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	forwardName, workItems, err := h.prepareForwardDiagnosis(forward)
	if err != nil {
		return nil, err
	}

	results := h.runDiagnosisWorkItems(ctx, workItems, nil)

	payload := map[string]interface{}{
		"forwardName": forwardName,
		"timestamp":   time.Now().UnixMilli(),
		"results":     results,
	}
	return payload, nil
}

func (h *Handler) prepareForwardDiagnosis(forward *forwardRecord) (string, []diagnosisWorkItem, error) {
	if forward == nil {
		return "", nil, errForwardNotFound
	}
	targets, err := resolveDiagnosisTargets(forward.RemoteAddr)
	if err != nil {
		return "", nil, err
	}

	tunnel, err := h.getTunnelRecord(forward.TunnelID)
	if err != nil {
		return "", nil, err
	}

	chainRows, err := h.listChainNodesForTunnel(forward.TunnelID)
	if err != nil {
		return "", nil, err
	}
	if len(chainRows) == 0 {
		return "", nil, errors.New("隧道配置不完整")
	}

	ipPreference := h.repo.GetTunnelIPPreference(forward.TunnelID)

	inNodes, chainHops, outNodes := splitChainNodeGroups(chainRows)
	workItems := make([]diagnosisWorkItem, 0, len(chainRows)*2+len(targets))

	switch tunnel.Type {
	case 1:
		for _, inNode := range inNodes {
			for _, target := range targets {
				description := fmt.Sprintf("入口(%s)->目标(%s)", inNode.NodeName, target.Address)
				workItems = append(workItems, diagnosisWorkItem{
					fromNodeID:  inNode.NodeID,
					targetIP:    target.IP,
					targetPort:  target.Port,
					description: description,
					metadata: map[string]interface{}{
						"fromChainType": 1,
					},
				})
			}
		}
	case 2:
		for _, inNode := range inNodes {
			if len(chainHops) > 0 {
				for _, firstNode := range chainHops[0] {
					description := fmt.Sprintf("入口(%s)->第1跳(%s)", inNode.NodeName, firstNode.NodeName)
					workItems = append(workItems, diagnosisWorkItem{
						fromNodeID:   inNode.NodeID,
						toNode:       firstNode,
						hasChainHop:  true,
						ipPreference: ipPreference,
						description:  description,
						metadata: map[string]interface{}{
							"fromChainType": 1,
							"toChainType":   2,
							"toInx":         firstNode.Inx,
						},
					})
				}
			} else {
				for _, outNode := range outNodes {
					description := fmt.Sprintf("入口(%s)->出口(%s)", inNode.NodeName, outNode.NodeName)
					workItems = append(workItems, diagnosisWorkItem{
						fromNodeID:   inNode.NodeID,
						toNode:       outNode,
						hasChainHop:  true,
						ipPreference: ipPreference,
						description:  description,
						metadata: map[string]interface{}{
							"fromChainType": 1,
							"toChainType":   3,
						},
					})
				}
			}
		}

		for i, hop := range chainHops {
			for _, currentNode := range hop {
				if i+1 < len(chainHops) {
					for _, nextNode := range chainHops[i+1] {
						description := fmt.Sprintf("第%d跳(%s)->第%d跳(%s)", i+1, currentNode.NodeName, i+2, nextNode.NodeName)
						workItems = append(workItems, diagnosisWorkItem{
							fromNodeID:   currentNode.NodeID,
							toNode:       nextNode,
							hasChainHop:  true,
							ipPreference: ipPreference,
							description:  description,
							metadata: map[string]interface{}{
								"fromChainType": 2,
								"fromInx":       currentNode.Inx,
								"toChainType":   2,
								"toInx":         nextNode.Inx,
							},
						})
					}
				} else {
					for _, outNode := range outNodes {
						description := fmt.Sprintf("第%d跳(%s)->出口(%s)", i+1, currentNode.NodeName, outNode.NodeName)
						workItems = append(workItems, diagnosisWorkItem{
							fromNodeID:   currentNode.NodeID,
							toNode:       outNode,
							hasChainHop:  true,
							ipPreference: ipPreference,
							description:  description,
							metadata: map[string]interface{}{
								"fromChainType": 2,
								"fromInx":       currentNode.Inx,
								"toChainType":   3,
							},
						})
					}
				}
			}
		}

		for _, outNode := range outNodes {
			for _, target := range targets {
				description := fmt.Sprintf("出口(%s)->目标(%s)", outNode.NodeName, target.Address)
				workItems = append(workItems, diagnosisWorkItem{
					fromNodeID:  outNode.NodeID,
					targetIP:    target.IP,
					targetPort:  target.Port,
					description: description,
					metadata: map[string]interface{}{
						"fromChainType": 3,
					},
				})
			}
		}
	default:
		for _, inNode := range inNodes {
			for _, target := range targets {
				description := fmt.Sprintf("入口(%s)->目标(%s)", inNode.NodeName, target.Address)
				workItems = append(workItems, diagnosisWorkItem{
					fromNodeID:  inNode.NodeID,
					targetIP:    target.IP,
					targetPort:  target.Port,
					description: description,
					metadata: map[string]interface{}{
						"fromChainType": 1,
					},
				})
			}
		}
	}

	return forward.Name, workItems, nil
}

func (h *Handler) diagnoseTunnelRuntime(ctx context.Context, tunnelID int64) (map[string]interface{}, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	tunnelName, tunnelType, workItems, err := h.prepareTunnelDiagnosis(tunnelID)
	if err != nil {
		return nil, err
	}

	results := h.runDiagnosisWorkItems(ctx, workItems, nil)

	payload := map[string]interface{}{
		"tunnelName": tunnelName,
		"tunnelType": tunnelType,
		"timestamp":  time.Now().UnixMilli(),
		"results":    results,
	}
	return payload, nil
}

func (h *Handler) prepareTunnelDiagnosis(tunnelID int64) (string, string, []diagnosisWorkItem, error) {
	tunnel, err := h.getTunnelRecord(tunnelID)
	if err != nil {
		return "", "", nil, err
	}

	tunnelName, err := h.repo.GetTunnelName(tunnelID)
	if err != nil {
		return "", "", nil, err
	}
	if tunnelName == "" {
		return "", "", nil, errors.New("隧道不存在")
	}

	chainRows, err := h.listChainNodesForTunnel(tunnelID)
	if err != nil {
		return "", "", nil, err
	}
	if len(chainRows) == 0 {
		return "", "", nil, errors.New("隧道配置不完整")
	}

	ipPreference := h.repo.GetTunnelIPPreference(tunnelID)
	inNodes, chainHops, outNodes := splitChainNodeGroups(chainRows)
	workItems := make([]diagnosisWorkItem, 0, len(chainRows)*2)

	switch tunnel.Type {
	case 1:
		for _, inNode := range inNodes {
			description := fmt.Sprintf("入口(%s)->外网", inNode.NodeName)
			workItems = append(workItems, diagnosisWorkItem{
				fromNodeID:  inNode.NodeID,
				targetIP:    "www.bing.com",
				targetPort:  443,
				description: description,
				metadata: map[string]interface{}{
					"fromChainType": 1,
				},
			})
		}
	case 2:
		for _, inNode := range inNodes {
			if len(chainHops) > 0 {
				for _, firstNode := range chainHops[0] {
					description := fmt.Sprintf("入口(%s)->第1跳(%s)", inNode.NodeName, firstNode.NodeName)
					workItems = append(workItems, diagnosisWorkItem{
						fromNodeID:   inNode.NodeID,
						toNode:       firstNode,
						hasChainHop:  true,
						ipPreference: ipPreference,
						description:  description,
						metadata: map[string]interface{}{
							"fromChainType": 1,
							"toChainType":   2,
							"toInx":         firstNode.Inx,
						},
					})
				}
			} else {
				for _, outNode := range outNodes {
					description := fmt.Sprintf("入口(%s)->出口(%s)", inNode.NodeName, outNode.NodeName)
					workItems = append(workItems, diagnosisWorkItem{
						fromNodeID:   inNode.NodeID,
						toNode:       outNode,
						hasChainHop:  true,
						ipPreference: ipPreference,
						description:  description,
						metadata: map[string]interface{}{
							"fromChainType": 1,
							"toChainType":   3,
						},
					})
				}
			}
		}

		for i, hop := range chainHops {
			for _, currentNode := range hop {
				if i+1 < len(chainHops) {
					for _, nextNode := range chainHops[i+1] {
						description := fmt.Sprintf("第%d跳(%s)->第%d跳(%s)", i+1, currentNode.NodeName, i+2, nextNode.NodeName)
						workItems = append(workItems, diagnosisWorkItem{
							fromNodeID:   currentNode.NodeID,
							toNode:       nextNode,
							hasChainHop:  true,
							ipPreference: ipPreference,
							description:  description,
							metadata: map[string]interface{}{
								"fromChainType": 2,
								"fromInx":       currentNode.Inx,
								"toChainType":   2,
								"toInx":         nextNode.Inx,
							},
						})
					}
				} else {
					for _, outNode := range outNodes {
						description := fmt.Sprintf("第%d跳(%s)->出口(%s)", i+1, currentNode.NodeName, outNode.NodeName)
						workItems = append(workItems, diagnosisWorkItem{
							fromNodeID:   currentNode.NodeID,
							toNode:       outNode,
							hasChainHop:  true,
							ipPreference: ipPreference,
							description:  description,
							metadata: map[string]interface{}{
								"fromChainType": 2,
								"fromInx":       currentNode.Inx,
								"toChainType":   3,
							},
						})
					}
				}
			}
		}

		for _, outNode := range outNodes {
			description := fmt.Sprintf("出口(%s)->外网", outNode.NodeName)
			workItems = append(workItems, diagnosisWorkItem{
				fromNodeID:  outNode.NodeID,
				targetIP:    "www.bing.com",
				targetPort:  443,
				description: description,
				metadata: map[string]interface{}{
					"fromChainType": 3,
				},
			})
		}
	default:
		for _, inNode := range inNodes {
			description := fmt.Sprintf("入口(%s)->外网", inNode.NodeName)
			workItems = append(workItems, diagnosisWorkItem{
				fromNodeID:  inNode.NodeID,
				targetIP:    "www.bing.com",
				targetPort:  443,
				description: description,
				metadata: map[string]interface{}{
					"fromChainType": 1,
				},
			})
		}
	}

	tunnelType := map[bool]string{true: "端口转发", false: "隧道转发"}[tunnel.Type == 1]
	return tunnelName, tunnelType, workItems, nil
}

func splitChainNodeGroups(rows []chainNodeRecord) ([]chainNodeRecord, [][]chainNodeRecord, []chainNodeRecord) {
	inNodes := make([]chainNodeRecord, 0)
	outNodes := make([]chainNodeRecord, 0)
	chainByInx := map[int64][]chainNodeRecord{}
	hopOrder := make([]int64, 0)

	for _, row := range rows {
		switch row.ChainType {
		case 1:
			inNodes = append(inNodes, row)
		case 2:
			if _, ok := chainByInx[row.Inx]; !ok {
				hopOrder = append(hopOrder, row.Inx)
			}
			chainByInx[row.Inx] = append(chainByInx[row.Inx], row)
		case 3:
			outNodes = append(outNodes, row)
		}
	}

	sort.Slice(hopOrder, func(i, j int) bool { return hopOrder[i] < hopOrder[j] })
	chainHops := make([][]chainNodeRecord, 0, len(hopOrder))
	for _, inx := range hopOrder {
		chainHops = append(chainHops, chainByInx[inx])
	}

	return inNodes, chainHops, outNodes
}

func resolveDiagnosisTargets(remoteAddr string) ([]diagnosisTarget, error) {
	rawTargets := splitRemoteTargets(remoteAddr)
	if len(rawTargets) == 0 {
		return nil, errors.New("目标地址不能为空")
	}

	targets := make([]diagnosisTarget, 0, len(rawTargets))
	for _, raw := range rawTargets {
		ip, port, err := parseTargetAddress(raw)
		if err != nil {
			continue
		}
		targets = append(targets, diagnosisTarget{Address: raw, IP: ip, Port: port})
	}
	if len(targets) == 0 {
		return nil, errors.New("目标地址格式错误")
	}
	return targets, nil
}

func diagnosisContextMessage(ctx context.Context) string {
	if ctx == nil {
		return diagnosisRequestTimeoutMsg
	}
	switch ctx.Err() {
	case context.DeadlineExceeded:
		return diagnosisRequestTimeoutMsg
	case context.Canceled:
		return "诊断已取消"
	default:
		return diagnosisRequestTimeoutMsg
	}
}

func diagnosisExecOptionsFromContext(ctx context.Context) diagnosisExecOptions {
	timeout := diagnosisCommandTimeout
	if ctx != nil {
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				remaining = 100 * time.Millisecond
			}
			if remaining < timeout {
				timeout = remaining
			}
		}
	}
	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}
	pingTimeoutMS := int(timeout / time.Millisecond)
	if pingTimeoutMS <= 0 {
		pingTimeoutMS = 100
	}
	return diagnosisExecOptions{
		commandTimeout: timeout,
		pingTimeoutMS:  pingTimeoutMS,
		timeoutMessage: diagnosisContextMessage(ctx),
	}
}

func newDiagnosisTimeoutItem(workItem diagnosisWorkItem, message string) map[string]interface{} {
	targetPort := workItem.targetPort
	if targetPort <= 0 {
		targetPort = workItem.toNode.Port
	}
	item := newDiagnosisResultItem(workItem.fromNodeID, workItem.targetIP, targetPort, workItem.description, workItem.metadata)
	item["success"] = false
	if strings.TrimSpace(message) == "" {
		message = diagnosisCommandTimeoutMsg
	}
	item["message"] = message
	return item
}

func (h *Handler) executeDiagnosisWorkItem(workItem diagnosisWorkItem, options diagnosisExecOptions) map[string]interface{} {
	single := make([]map[string]interface{}, 0, 1)
	nodeCache := map[int64]*nodeRecord{}
	if workItem.hasChainHop {
		h.appendChainHopDiagnosis(&single, nodeCache, workItem.fromNodeID, workItem.toNode, workItem.description, workItem.metadata, workItem.ipPreference, options)
	} else {
		h.appendPathDiagnosis(&single, nodeCache, workItem.fromNodeID, workItem.targetIP, workItem.targetPort, workItem.description, workItem.metadata, options)
	}

	if len(single) == 0 {
		return newDiagnosisTimeoutItem(workItem, "诊断任务未返回结果")
	}
	return single[0]
}

func (h *Handler) runDiagnosisWorkItems(ctx context.Context, workItems []diagnosisWorkItem, emitter diagnosisItemEmitter) []map[string]interface{} {
	if ctx == nil {
		ctx = context.Background()
	}
	results := make([]map[string]interface{}, len(workItems))
	if len(workItems) == 0 {
		return results
	}

	workerLimit := diagnosisMaxConcurrency
	if workerLimit < 1 {
		workerLimit = 1
	}
	if workerLimit > len(workItems) {
		workerLimit = len(workItems)
	}

	type diagnosisWorkResult struct {
		index int
		item  map[string]interface{}
	}

	jobs := make(chan int)
	resultCh := make(chan diagnosisWorkResult, len(workItems))

	var wg sync.WaitGroup
	for i := 0; i < workerLimit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				select {
				case <-ctx.Done():
					resultCh <- diagnosisWorkResult{index: index, item: newDiagnosisTimeoutItem(workItems[index], diagnosisContextMessage(ctx))}
					continue
				default:
				}
				options := diagnosisExecOptionsFromContext(ctx)
				resultCh <- diagnosisWorkResult{index: index, item: h.executeDiagnosisWorkItem(workItems[index], options)}
			}
		}()
	}

enqueueLoop:
	for i := 0; i < len(workItems); i++ {
		select {
		case <-ctx.Done():
			message := diagnosisContextMessage(ctx)
			for j := i; j < len(workItems); j++ {
				resultCh <- diagnosisWorkResult{index: j, item: newDiagnosisTimeoutItem(workItems[j], message)}
			}
			break enqueueLoop
		case jobs <- i:
		}
	}
	close(jobs)
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	progress := diagnosisProgress{Total: len(workItems)}
	for result := range resultCh {
		results[result.index] = result.item
		progress.Completed++
		if asBool(result.item["success"], false) {
			progress.Success++
		} else {
			progress.Failed++
		}
		if emitter != nil {
			emitter(result.index, result.item, progress)
		}
	}

	for i := range results {
		if results[i] == nil {
			results[i] = newDiagnosisTimeoutItem(workItems[i], diagnosisCommandTimeoutMsg)
		}
	}
	return results
}

func (h *Handler) cachedNode(nodeCache map[int64]*nodeRecord, nodeID int64) (*nodeRecord, error) {
	if node, ok := nodeCache[nodeID]; ok {
		return node, nil
	}
	node, err := h.getNodeRecord(nodeID)
	if err != nil {
		return nil, err
	}
	nodeCache[nodeID] = node
	return node, nil
}

func newDiagnosisResultItem(fromNodeID int64, targetIP string, targetPort int, description string, metadata map[string]interface{}) map[string]interface{} {
	item := map[string]interface{}{
		"nodeName":    fmt.Sprintf("node_%d", fromNodeID),
		"nodeId":      strconv.FormatInt(fromNodeID, 10),
		"targetIp":    targetIP,
		"targetPort":  targetPort,
		"description": description,
		"averageTime": 0,
		"packetLoss":  100,
	}
	for k, v := range metadata {
		item[k] = v
	}
	return item
}

func (h *Handler) appendFailedDiagnosis(results *[]map[string]interface{}, nodeCache map[int64]*nodeRecord, fromNodeID int64, targetIP string, targetPort int, description string, metadata map[string]interface{}, message string) {
	item := newDiagnosisResultItem(fromNodeID, targetIP, targetPort, description, metadata)
	if node, err := h.cachedNode(nodeCache, fromNodeID); err == nil {
		item["nodeName"] = node.Name
	}
	if strings.TrimSpace(message) == "" {
		message = "TCP连接失败"
	}
	item["success"] = false
	item["message"] = message
	*results = append(*results, item)
}

func (h *Handler) appendPathDiagnosis(results *[]map[string]interface{}, nodeCache map[int64]*nodeRecord, fromNodeID int64, targetIP string, targetPort int, description string, metadata map[string]interface{}, options diagnosisExecOptions) {
	item := newDiagnosisResultItem(fromNodeID, targetIP, targetPort, description, metadata)

	fromNode, err := h.cachedNode(nodeCache, fromNodeID)
	if err != nil {
		item["success"] = false
		item["message"] = err.Error()
		*results = append(*results, item)
		return
	}
	item["nodeName"] = fromNode.Name

	var (
		pingData map[string]interface{}
		pingErr  error
	)
	if fromNode.IsRemote == 1 {
		pingData, pingErr = h.tcpPingViaRemoteNode(fromNode, targetIP, targetPort, options)
	} else {
		pingData, pingErr = h.tcpPingViaNode(fromNodeID, targetIP, targetPort, options)
	}
	if pingErr != nil {
		item["success"] = false
		item["message"] = pingErr.Error()
		*results = append(*results, item)
		return
	}

	success := asBool(pingData["success"], false)
	item["success"] = success
	item["averageTime"] = asFloat(pingData["averageTime"], 0)
	item["packetLoss"] = asFloat(pingData["packetLoss"], 100)

	message := strings.TrimSpace(asString(pingData["message"]))
	if success {
		if message == "" {
			message = "TCP连接成功"
		}
	} else {
		if message == "" {
			message = strings.TrimSpace(asString(pingData["errorMessage"]))
		}
		if message == "" {
			message = "TCP连接失败"
		}
	}
	item["message"] = message
	*results = append(*results, item)
}

func (h *Handler) appendChainHopDiagnosis(results *[]map[string]interface{}, nodeCache map[int64]*nodeRecord, fromNodeID int64, toNode chainNodeRecord, description string, metadata map[string]interface{}, ipPreference string, options diagnosisExecOptions) {
	fromNode, _ := h.cachedNode(nodeCache, fromNodeID)
	targetNode, err := h.cachedNode(nodeCache, toNode.NodeID)
	if err != nil {
		h.appendFailedDiagnosis(results, nodeCache, fromNodeID, "", 0, description, metadata, err.Error())
		return
	}
	targetIP, targetPort, err := resolveChainProbeTarget(fromNode, targetNode, toNode.Port, ipPreference, toNode.ConnectIP)
	if err != nil {
		h.appendFailedDiagnosis(results, nodeCache, fromNodeID, strings.Trim(strings.TrimSpace(targetNode.ServerIP), "[]"), toNode.Port, description, metadata, err.Error())
		return
	}
	h.appendPathDiagnosis(results, nodeCache, fromNodeID, targetIP, targetPort, description, metadata, options)
}

func resolveChainProbeTarget(fromNode, targetNode *nodeRecord, preferredPort int, ipPreference string, connectIp string) (string, int, error) {
	if targetNode == nil {
		return "", 0, errors.New("目标节点不存在")
	}
	host, err := selectTunnelDialHost(fromNode, targetNode, ipPreference, connectIp)
	if err != nil {
		host = strings.Trim(strings.TrimSpace(targetNode.ServerIP), "[]")
	}
	if host == "" {
		return "", 0, errors.New("目标节点地址为空")
	}
	port := preferredPort
	if port <= 0 {
		port = firstPortFromRange(targetNode.PortRange)
	}
	if port <= 0 {
		port = 443
	}
	return host, port, nil
}

func firstPortFromRange(portRange string) int {
	portRange = strings.TrimSpace(portRange)
	if portRange == "" {
		return 0
	}
	first := strings.Split(portRange, ",")[0]
	first = strings.TrimSpace(first)
	if strings.Contains(first, "-") {
		parts := strings.SplitN(first, "-", 2)
		if len(parts) != 2 {
			return 0
		}
		p, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || p <= 0 {
			return 0
		}
		return p
	}
	p, err := strconv.Atoi(first)
	if err != nil || p <= 0 {
		return 0
	}
	return p
}

func (h *Handler) listChainNodesForTunnel(tunnelID int64) ([]chainNodeRecord, error) {
	return h.repo.ListChainNodesForTunnel(tunnelID)
}

func (h *Handler) tcpPingViaNode(nodeID int64, ip string, port int, options diagnosisExecOptions) (map[string]interface{}, error) {
	if options.commandTimeout <= 0 {
		options.commandTimeout = diagnosisCommandTimeout
	}
	if options.pingTimeoutMS <= 0 {
		options.pingTimeoutMS = int(diagnosisCommandTimeout / time.Millisecond)
	}
	res, err := h.sendNodeCommandWithTimeout(nodeID, "TcpPing", map[string]interface{}{
		"ip":      ip,
		"port":    port,
		"count":   4,
		"timeout": options.pingTimeoutMS,
	}, options.commandTimeout, false, false)
	if err != nil {
		return nil, err
	}
	if res.Data == nil {
		return nil, errors.New("节点未返回诊断数据")
	}
	return res.Data, nil
}

func (h *Handler) tcpPingViaRemoteNode(node *nodeRecord, ip string, port int, options diagnosisExecOptions) (map[string]interface{}, error) {
	if node == nil {
		return nil, errors.New("节点不存在")
	}
	remoteURL := strings.TrimSpace(node.RemoteURL)
	remoteToken := strings.TrimSpace(node.RemoteToken)
	if remoteURL == "" || remoteToken == "" {
		return nil, errors.New("远程节点缺少共享配置")
	}
	if options.commandTimeout <= 0 {
		options.commandTimeout = diagnosisCommandTimeout
	}
	if options.pingTimeoutMS <= 0 {
		options.pingTimeoutMS = int(diagnosisCommandTimeout / time.Millisecond)
	}

	fc := client.NewFederationClientWithTimeout(options.commandTimeout)
	return fc.Diagnose(remoteURL, remoteToken, h.federationLocalDomain(), client.RuntimeDiagnoseRequest{
		IP:      strings.TrimSpace(ip),
		Port:    port,
		Count:   4,
		Timeout: options.pingTimeoutMS,
	})
}

func splitRemoteTargets(remoteAddr string) []string {
	parts := strings.Split(remoteAddr, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, processServerAddress(part))
	}
	return out
}

func parseTargetAddress(addr string) (string, int, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", 0, errors.New("empty address")
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		idx := strings.LastIndex(addr, ":")
		if idx <= 0 || idx >= len(addr)-1 {
			return "", 0, err
		}
		host = strings.TrimSpace(addr[:idx])
		portStr = strings.TrimSpace(addr[idx+1:])
	}
	port, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, errors.New("invalid port")
	}
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" {
		return "", 0, errors.New("invalid host")
	}
	return host, port, nil
}

func buildForwardServiceBase(forwardID, userID, userTunnelID int64) string {
	return fmt.Sprintf("%d_%d_%d", forwardID, userID, userTunnelID)
}

func buildForwardServiceBaseWithResolvedUserTunnel(forwardID, userID, resolvedUserTunnelID int64) string {
	if resolvedUserTunnelID <= 0 {
		return buildForwardServiceBase(forwardID, userID, 0)
	}
	return buildForwardServiceBase(forwardID, userID, resolvedUserTunnelID)
}

func buildForwardServiceBaseCandidates(forwardID, userID, preferredUserTunnelID int64, userTunnelIDs []int64) []string {
	orderedIDs := make([]int64, 0, len(userTunnelIDs)+2)
	seen := make(map[int64]struct{}, len(userTunnelIDs)+2)

	appendID := func(id int64) {
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		orderedIDs = append(orderedIDs, id)
	}

	appendID(preferredUserTunnelID)
	for _, id := range userTunnelIDs {
		appendID(id)
	}
	appendID(0)

	bases := make([]string, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		bases = append(bases, buildForwardServiceBase(forwardID, userID, id))
	}
	return bases
}

func buildForwardControlServiceNames(base, commandType string) []string {
	names := []string{base + "_tcp", base + "_udp"}
	if strings.EqualFold(strings.TrimSpace(commandType), "DeleteService") {
		return append([]string{base}, names...)
	}
	return names
}

func shouldTryLegacySingleService(commandType string) bool {
	cmd := strings.ToLower(strings.TrimSpace(commandType))
	return cmd == "pauseservice" || cmd == "resumeservice"
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "not found") || strings.Contains(msg, "不存在")
}

func isAlreadyExistsMessage(message string) bool {
	msg := strings.ToLower(strings.TrimSpace(message))
	if msg == "" {
		return false
	}
	if isAddressAlreadyInUseMessage(msg) {
		return false
	}
	compact := compactErrorMessage(msg)
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "已存在") || strings.Contains(compact, "alreadyexists")
}

func isBindAddressInUseError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	return isAddressAlreadyInUseMessage(msg) || strings.Contains(msg, "cannot assign requested address")
}

func isAddressAlreadyInUseError(err error) bool {
	if err == nil {
		return false
	}
	return isAddressAlreadyInUseMessage(strings.ToLower(strings.TrimSpace(err.Error())))
}

func isAddressAlreadyInUseMessage(msg string) bool {
	if msg == "" {
		return false
	}
	if strings.Contains(msg, "address already in use") {
		return true
	}
	return strings.Contains(compactErrorMessage(msg), "addressalreadyinuse")
}

func isCannotAssignRequestedAddressError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	if strings.Contains(msg, "cannot assign requested address") {
		return true
	}
	return strings.Contains(compactErrorMessage(msg), "cannotassignrequestedaddress")
}

func compactErrorMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	return strings.Join(strings.Fields(strings.ToLower(msg)), "")
}

func buildForwardServiceConfigs(baseName string, forward *forwardRecord, tunnel *tunnelRecord, node *nodeRecord, port int, bindIP string, limiterID *int64, tunnelTLSProtocol bool) []map[string]interface{} {
	protocols := []string{"tcp", "udp"}
	services := make([]map[string]interface{}, 0, 2)
	targets := splitRemoteTargets(forward.RemoteAddr)
	strategy := strings.TrimSpace(forward.Strategy)
	if strategy == "" {
		strategy = "fifo"
	}

	for _, protocol := range protocols {
		listenerAddr := node.TCPListenAddr
		if protocol == "udp" {
			listenerAddr = node.UDPListenAddr
		}
		var serviceAddr string
		if bindIP != "" {
			trimmedBindIP := strings.TrimSpace(bindIP)
			if _, _, err := net.SplitHostPort(trimmedBindIP); err == nil {
				serviceAddr = processServerAddress(trimmedBindIP)
			} else {
				serviceAddr = processServerAddress(net.JoinHostPort(strings.Trim(trimmedBindIP, "[]"), strconv.Itoa(port)))
			}
		} else {
			serviceAddr = processServerAddress(fmt.Sprintf("%s:%d", listenerAddr, port))
		}
		service := map[string]interface{}{
			"name": fmt.Sprintf("%s_%s", baseName, protocol),
			"addr": serviceAddr,
			"handler": map[string]interface{}{
				"type": protocol,
			},
			"listener": map[string]interface{}{
				"type": protocol,
			},
			"forwarder": map[string]interface{}{
				"nodes": buildForwarderNodes(targets),
				"selector": map[string]interface{}{
					"strategy":    strategy,
					"maxFails":    1,
					"failTimeout": "600s",
				},
			},
		}
		if protocol == "udp" {
			listenerMetadata := map[string]interface{}{"keepAlive": true}
			if tunnelTLSProtocol {
				listenerMetadata["ttl"] = "10s"
			}
			service["listener"].(map[string]interface{})["metadata"] = listenerMetadata
		}
		if tunnel != nil && tunnel.Type == 2 {
			service["handler"].(map[string]interface{})["chain"] = fmt.Sprintf("chains_%d", forward.TunnelID)
		}
		if tunnel != nil && tunnel.Type == 1 && strings.TrimSpace(node.InterfaceName) != "" {
			service["metadata"] = map[string]interface{}{"interface": node.InterfaceName}
		}
		if limiterID != nil && *limiterID > 0 {
			service["limiter"] = strconv.FormatInt(*limiterID, 10)
		}
		services = append(services, service)
	}

	return services
}

func buildForwarderNodes(targets []string) []map[string]interface{} {
	nodes := make([]map[string]interface{}, 0, len(targets))
	for i, addr := range targets {
		nodes = append(nodes, map[string]interface{}{
			"name": fmt.Sprintf("node_%d", i+1),
			"addr": addr,
		})
	}
	return nodes
}

func processServerAddress(serverAddr string) string {
	serverAddr = normalizeServerAddressInput(serverAddr)
	if serverAddr == "" {
		return serverAddr
	}
	if strings.HasPrefix(serverAddr, "[") {
		return serverAddr
	}
	// If the input is a bare IPv6 host (no port), bracket it.
	// IPv6-with-port must be provided in bracket form: [::1]:443.
	if looksLikeIPv6(serverAddr) {
		if ip := net.ParseIP(serverAddr); ip != nil && ip.To4() == nil {
			return "[" + serverAddr + "]"
		}
	}

	idx := strings.LastIndex(serverAddr, ":")
	if idx < 0 {
		if looksLikeIPv6(serverAddr) {
			return "[" + serverAddr + "]"
		}
		return serverAddr
	}
	host := strings.TrimSpace(serverAddr[:idx])
	port := strings.TrimSpace(serverAddr[idx+1:])
	if host == "" || port == "" {
		return serverAddr
	}
	if looksLikeIPv6(host) {
		return "[" + host + "]:" + port
	}
	return serverAddr
}

func normalizeServerAddressInput(serverAddr string) string {
	serverAddr = strings.TrimSpace(serverAddr)
	if serverAddr == "" {
		return serverAddr
	}

	if idx := strings.Index(serverAddr, "://"); idx > 0 {
		if parsed, err := url.Parse(serverAddr); err == nil {
			if host := strings.TrimSpace(parsed.Host); host != "" {
				return host
			}
		}
		serverAddr = serverAddr[idx+3:]
	}

	if idx := strings.IndexAny(serverAddr, "/?#"); idx >= 0 {
		serverAddr = serverAddr[:idx]
	}
	return strings.TrimSpace(serverAddr)
}

func looksLikeIPv6(address string) bool {
	return strings.Count(address, ":") >= 2
}

func asBool(v interface{}, def bool) bool {
	s := strings.TrimSpace(strings.ToLower(asString(v)))
	if s == "" {
		return def
	}
	switch s {
	case "1", "t", "true", "yes", "y":
		return true
	case "0", "f", "false", "no", "n":
		return false
	default:
		return def
	}
}

func (h *Handler) ensureLimiterOnNode(nodeID int64, limiterID int64, speed int) error {
	if err := h.upsertLimiterOnNode(nodeID, limiterID, speed); err != nil {
		return fmt.Errorf("限速规则下发失败: %w", err)
	}

	return nil
}

func buildLimiterAddPayload(limiterID int64, speed int) (string, map[string]interface{}) {
	rate := float64(speed) / 8.0
	limitStr := fmt.Sprintf("$ %.1fMB %.1fMB", rate, rate)
	name := strconv.FormatInt(limiterID, 10)

	return name, map[string]interface{}{
		"name":   name,
		"limits": []string{limitStr},
	}
}

func buildLimiterUpdatePayload(name string, data map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"limiter": name,
		"data":    data,
	}
}

func (h *Handler) upsertLimiterOnNode(nodeID int64, limiterID int64, speed int) error {
	name, addPayload := buildLimiterAddPayload(limiterID, speed)
	if _, err := h.sendNodeCommand(nodeID, "AddLimiters", addPayload, false, false); err != nil {
		if !isAlreadyExistsMessage(err.Error()) {
			return err
		}
		payload := map[string]interface{}{
			"name":   name,
			"limits": addPayload["limits"],
		}
		if _, updateErr := h.sendNodeCommand(nodeID, "UpdateLimiters", buildLimiterUpdatePayload(name, payload), false, false); updateErr != nil {
			return updateErr
		}
	}

	return nil
}
