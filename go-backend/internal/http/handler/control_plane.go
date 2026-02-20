package handler

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"go-backend/internal/http/client"
	"go-backend/internal/store/model"
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
	if h == nil || forward == nil {
		return errors.New("invalid forward sync context")
	}

	tunnel, err := h.getTunnelRecord(forward.TunnelID)
	if err != nil {
		return err
	}
	ports, err := h.listForwardPorts(forward.ID)
	if err != nil {
		return err
	}
	if len(ports) == 0 {
		return errors.New("转发入口端口不存在")
	}

	userTunnelID, limiterID, speed, err := h.resolveUserTunnelAndLimiter(forward.UserID, forward.TunnelID)
	if err != nil {
		return err
	}
	serviceBase := buildForwardServiceBase(forward.ID, forward.UserID, userTunnelID)
	tunnelTLSProtocol, err := h.isTunnelSelectedTLSProtocol(forward.TunnelID)
	if err != nil {
		return err
	}

	for _, fp := range ports {
		if limiterID != nil && speed != nil {
			h.ensureLimiterOnNode(fp.NodeID, *limiterID, *speed)
		}

		node, err := h.getNodeRecord(fp.NodeID)
		if err != nil {
			return err
		}
		services := buildForwardServiceConfigs(serviceBase, forward, tunnel, node, fp.Port, limiterID, tunnelTLSProtocol)
		_, err = h.sendNodeCommand(node.ID, method, services, true, false)
		if err != nil && allowFallbackAdd && method == "UpdateService" {
			_, err = h.sendNodeCommand(node.ID, "AddService", services, true, false)
		}
		if err != nil {
			return fmt.Errorf("节点 %s 下发失败: %w", node.Name, err)
		}
	}
	return nil
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
	for _, fp := range ports {
		if _, ok := seen[fp.NodeID]; ok {
			continue
		}
		seen[fp.NodeID] = struct{}{}

		var lastNotFoundErr error
		nodeHandled := false

		for _, base := range bases {
			variants := []string{base + "_tcp", base + "_udp"}
			if shouldTryLegacySingleService(commandType) || strings.EqualFold(strings.TrimSpace(commandType), "DeleteService") {
				variants = append(variants, base)
			}

			candidateHandled := false
			for _, name := range variants {
				payload := map[string]interface{}{
					"services": []string{name},
				}
				_, err := h.sendNodeCommand(fp.NodeID, commandType, payload, false, false)
				if err == nil {
					candidateHandled = true
					continue
				}
				if !isNotFoundError(err) {
					return err
				}
				lastNotFoundErr = err
			}

			if candidateHandled {
				nodeHandled = true
				break
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

func (h *Handler) applyNodeProtocolChange(nodeID int64, httpVal, tlsVal, socksVal int) error {
	_, err := h.sendNodeCommand(nodeID, "SetProtocol", map[string]interface{}{
		"http":  httpVal,
		"tls":   tlsVal,
		"socks": socksVal,
	}, false, false)
	return err
}

func (h *Handler) sendNodeCommand(nodeID int64, commandType string, data interface{}, tolerateExists bool, tolerateNotFound bool) (ws.CommandResult, error) {
	var (
		result ws.CommandResult
		err    error
	)

	node, nodeErr := h.getNodeRecord(nodeID)
	if nodeErr == nil && node != nil && node.IsRemote == 1 {
		result, err = h.sendRemoteNodeCommand(node, commandType, data)
	} else {
		result, err = h.wsServer.SendCommand(nodeID, commandType, data, 12*time.Second)
	}
	if err == nil {
		return result, nil
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if tolerateExists {
		if strings.Contains(msg, "exists") || strings.Contains(msg, "already") || strings.Contains(msg, "已存在") {
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
	if node == nil {
		return ws.CommandResult{}, errors.New("节点不存在")
	}
	remoteURL := strings.TrimSpace(node.RemoteURL)
	remoteToken := strings.TrimSpace(node.RemoteToken)
	if remoteURL == "" || remoteToken == "" {
		return ws.CommandResult{}, errors.New("远程节点缺少共享配置")
	}

	fc := client.NewFederationClient()
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

func (h *Handler) diagnoseForwardRuntime(forward *forwardRecord) (map[string]interface{}, error) {
	if forward == nil {
		return nil, errForwardNotFound
	}
	targets, err := resolveDiagnosisTargets(forward.RemoteAddr)
	if err != nil {
		return nil, err
	}

	tunnel, err := h.getTunnelRecord(forward.TunnelID)
	if err != nil {
		return nil, err
	}

	chainRows, err := h.listChainNodesForTunnel(forward.TunnelID)
	if err != nil {
		return nil, err
	}
	if len(chainRows) == 0 {
		return nil, errors.New("隧道配置不完整")
	}

	inNodes, chainHops, outNodes := splitChainNodeGroups(chainRows)
	results := make([]map[string]interface{}, 0, len(chainRows)*2+len(targets))
	nodeCache := map[int64]*nodeRecord{}

	switch tunnel.Type {
	case 1:
		for _, inNode := range inNodes {
			for _, target := range targets {
				description := fmt.Sprintf("入口(%s)->目标(%s)", inNode.NodeName, target.Address)
				h.appendPathDiagnosis(&results, nodeCache, inNode.NodeID, target.IP, target.Port, description, map[string]interface{}{
					"fromChainType": 1,
				})
			}
		}
	case 2:
		for _, inNode := range inNodes {
			if len(chainHops) > 0 {
				for _, firstNode := range chainHops[0] {
					description := fmt.Sprintf("入口(%s)->第1跳(%s)", inNode.NodeName, firstNode.NodeName)
					h.appendChainHopDiagnosis(&results, nodeCache, inNode.NodeID, firstNode, description, map[string]interface{}{
						"fromChainType": 1,
						"toChainType":   2,
						"toInx":         firstNode.Inx,
					}, "")
				}
			} else {
				for _, outNode := range outNodes {
					description := fmt.Sprintf("入口(%s)->出口(%s)", inNode.NodeName, outNode.NodeName)
					h.appendChainHopDiagnosis(&results, nodeCache, inNode.NodeID, outNode, description, map[string]interface{}{
						"fromChainType": 1,
						"toChainType":   3,
					}, "")
				}
			}
		}

		for i, hop := range chainHops {
			for _, currentNode := range hop {
				if i+1 < len(chainHops) {
					for _, nextNode := range chainHops[i+1] {
						description := fmt.Sprintf("第%d跳(%s)->第%d跳(%s)", i+1, currentNode.NodeName, i+2, nextNode.NodeName)
						h.appendChainHopDiagnosis(&results, nodeCache, currentNode.NodeID, nextNode, description, map[string]interface{}{
							"fromChainType": 2,
							"fromInx":       currentNode.Inx,
							"toChainType":   2,
							"toInx":         nextNode.Inx,
						}, "")
					}
				} else {
					for _, outNode := range outNodes {
						description := fmt.Sprintf("第%d跳(%s)->出口(%s)", i+1, currentNode.NodeName, outNode.NodeName)
						h.appendChainHopDiagnosis(&results, nodeCache, currentNode.NodeID, outNode, description, map[string]interface{}{
							"fromChainType": 2,
							"fromInx":       currentNode.Inx,
							"toChainType":   3,
						}, "")
					}
				}
			}
		}

		for _, outNode := range outNodes {
			for _, target := range targets {
				description := fmt.Sprintf("出口(%s)->目标(%s)", outNode.NodeName, target.Address)
				h.appendPathDiagnosis(&results, nodeCache, outNode.NodeID, target.IP, target.Port, description, map[string]interface{}{
					"fromChainType": 3,
				})
			}
		}
	default:
		for _, inNode := range inNodes {
			for _, target := range targets {
				description := fmt.Sprintf("入口(%s)->目标(%s)", inNode.NodeName, target.Address)
				h.appendPathDiagnosis(&results, nodeCache, inNode.NodeID, target.IP, target.Port, description, map[string]interface{}{
					"fromChainType": 1,
				})
			}
		}
	}

	payload := map[string]interface{}{
		"forwardName": forward.Name,
		"timestamp":   time.Now().UnixMilli(),
		"results":     results,
	}
	return payload, nil
}

func (h *Handler) diagnoseTunnelRuntime(tunnelID int64) (map[string]interface{}, error) {
	tunnel, err := h.getTunnelRecord(tunnelID)
	if err != nil {
		return nil, err
	}

	tunnelName, err := h.repo.GetTunnelName(tunnelID)
	if err != nil {
		return nil, err
	}
	if tunnelName == "" {
		return nil, errors.New("隧道不存在")
	}

	chainRows, err := h.listChainNodesForTunnel(tunnelID)
	if err != nil {
		return nil, err
	}
	if len(chainRows) == 0 {
		return nil, errors.New("隧道配置不完整")
	}

	ipPreference := h.repo.GetTunnelIPPreference(tunnelID)
	inNodes, chainHops, outNodes := splitChainNodeGroups(chainRows)
	results := make([]map[string]interface{}, 0, len(chainRows)*2)
	nodeCache := map[int64]*nodeRecord{}

	switch tunnel.Type {
	case 1:
		for _, inNode := range inNodes {
			description := fmt.Sprintf("入口(%s)->外网", inNode.NodeName)
			h.appendPathDiagnosis(&results, nodeCache, inNode.NodeID, "www.bing.com", 443, description, map[string]interface{}{
				"fromChainType": 1,
			})
		}
	case 2:
		for _, inNode := range inNodes {
			if len(chainHops) > 0 {
				for _, firstNode := range chainHops[0] {
					description := fmt.Sprintf("入口(%s)->第1跳(%s)", inNode.NodeName, firstNode.NodeName)
					h.appendChainHopDiagnosis(&results, nodeCache, inNode.NodeID, firstNode, description, map[string]interface{}{
						"fromChainType": 1,
						"toChainType":   2,
						"toInx":         firstNode.Inx,
					}, ipPreference)
				}
			} else {
				for _, outNode := range outNodes {
					description := fmt.Sprintf("入口(%s)->出口(%s)", inNode.NodeName, outNode.NodeName)
					h.appendChainHopDiagnosis(&results, nodeCache, inNode.NodeID, outNode, description, map[string]interface{}{
						"fromChainType": 1,
						"toChainType":   3,
					}, ipPreference)
				}
			}
		}

		for i, hop := range chainHops {
			for _, currentNode := range hop {
				if i+1 < len(chainHops) {
					for _, nextNode := range chainHops[i+1] {
						description := fmt.Sprintf("第%d跳(%s)->第%d跳(%s)", i+1, currentNode.NodeName, i+2, nextNode.NodeName)
						h.appendChainHopDiagnosis(&results, nodeCache, currentNode.NodeID, nextNode, description, map[string]interface{}{
							"fromChainType": 2,
							"fromInx":       currentNode.Inx,
							"toChainType":   2,
							"toInx":         nextNode.Inx,
						}, ipPreference)
					}
				} else {
					for _, outNode := range outNodes {
						description := fmt.Sprintf("第%d跳(%s)->出口(%s)", i+1, currentNode.NodeName, outNode.NodeName)
						h.appendChainHopDiagnosis(&results, nodeCache, currentNode.NodeID, outNode, description, map[string]interface{}{
							"fromChainType": 2,
							"fromInx":       currentNode.Inx,
							"toChainType":   3,
						}, ipPreference)
					}
				}
			}
		}

		for _, outNode := range outNodes {
			description := fmt.Sprintf("出口(%s)->外网", outNode.NodeName)
			h.appendPathDiagnosis(&results, nodeCache, outNode.NodeID, "www.bing.com", 443, description, map[string]interface{}{
				"fromChainType": 3,
			})
		}
	default:
		for _, inNode := range inNodes {
			description := fmt.Sprintf("入口(%s)->外网", inNode.NodeName)
			h.appendPathDiagnosis(&results, nodeCache, inNode.NodeID, "www.bing.com", 443, description, map[string]interface{}{
				"fromChainType": 1,
			})
		}
	}

	payload := map[string]interface{}{
		"tunnelName": tunnelName,
		"tunnelType": map[bool]string{true: "端口转发", false: "隧道转发"}[tunnel.Type == 1],
		"timestamp":  time.Now().UnixMilli(),
		"results":    results,
	}
	return payload, nil
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

func (h *Handler) appendPathDiagnosis(results *[]map[string]interface{}, nodeCache map[int64]*nodeRecord, fromNodeID int64, targetIP string, targetPort int, description string, metadata map[string]interface{}) {
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
		pingData, pingErr = h.tcpPingViaRemoteNode(fromNode, targetIP, targetPort)
	} else {
		pingData, pingErr = h.tcpPingViaNode(fromNodeID, targetIP, targetPort)
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

func (h *Handler) appendChainHopDiagnosis(results *[]map[string]interface{}, nodeCache map[int64]*nodeRecord, fromNodeID int64, toNode chainNodeRecord, description string, metadata map[string]interface{}, ipPreference string) {
	fromNode, _ := h.cachedNode(nodeCache, fromNodeID)
	targetNode, err := h.cachedNode(nodeCache, toNode.NodeID)
	if err != nil {
		h.appendFailedDiagnosis(results, nodeCache, fromNodeID, "", 0, description, metadata, err.Error())
		return
	}
	targetIP, targetPort, err := resolveChainProbeTarget(fromNode, targetNode, toNode.Port, ipPreference)
	if err != nil {
		h.appendFailedDiagnosis(results, nodeCache, fromNodeID, strings.Trim(strings.TrimSpace(targetNode.ServerIP), "[]"), toNode.Port, description, metadata, err.Error())
		return
	}
	h.appendPathDiagnosis(results, nodeCache, fromNodeID, targetIP, targetPort, description, metadata)
}

func resolveChainProbeTarget(fromNode, targetNode *nodeRecord, preferredPort int, ipPreference string) (string, int, error) {
	if targetNode == nil {
		return "", 0, errors.New("目标节点不存在")
	}
	host, err := selectTunnelDialHost(fromNode, targetNode, ipPreference)
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

func (h *Handler) tcpPingViaNode(nodeID int64, ip string, port int) (map[string]interface{}, error) {
	res, err := h.sendNodeCommand(nodeID, "TcpPing", map[string]interface{}{
		"ip":      ip,
		"port":    port,
		"count":   4,
		"timeout": 5000,
	}, false, false)
	if err != nil {
		return nil, err
	}
	if res.Data == nil {
		return nil, errors.New("节点未返回诊断数据")
	}
	return res.Data, nil
}

func (h *Handler) tcpPingViaRemoteNode(node *nodeRecord, ip string, port int) (map[string]interface{}, error) {
	if node == nil {
		return nil, errors.New("节点不存在")
	}
	remoteURL := strings.TrimSpace(node.RemoteURL)
	remoteToken := strings.TrimSpace(node.RemoteToken)
	if remoteURL == "" || remoteToken == "" {
		return nil, errors.New("远程节点缺少共享配置")
	}

	fc := client.NewFederationClient()
	return fc.Diagnose(remoteURL, remoteToken, h.federationLocalDomain(), client.RuntimeDiagnoseRequest{
		IP:      strings.TrimSpace(ip),
		Port:    port,
		Count:   4,
		Timeout: 5000,
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

func buildForwardServiceConfigs(baseName string, forward *forwardRecord, tunnel *tunnelRecord, node *nodeRecord, port int, limiterID *int64, tunnelTLSProtocol bool) []map[string]interface{} {
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
		service := map[string]interface{}{
			"name": fmt.Sprintf("%s_%s", baseName, protocol),
			"addr": fmt.Sprintf("%s:%d", listenerAddr, port),
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
	serverAddr = strings.TrimSpace(serverAddr)
	if serverAddr == "" {
		return serverAddr
	}
	if strings.HasPrefix(serverAddr, "[") {
		return serverAddr
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

func (h *Handler) sendLimiterConfig(limiterID int64, speedMbps int, tunnelID int64) error {
	rate := float64(speedMbps) / 8.0
	limitStr := fmt.Sprintf("$ %.1fMB %.1fMB", rate, rate)

	payload := map[string]interface{}{
		"name":   strconv.FormatInt(limiterID, 10),
		"limits": []string{limitStr},
	}

	nodes, err := h.tunnelEntryNodeIDs(tunnelID)
	if err != nil {
		return err
	}

	for _, nodeID := range nodes {
		_, _ = h.sendNodeCommand(nodeID, "AddLimiters", payload, false, false)
	}
	return nil
}

func (h *Handler) sendDeleteLimiterConfig(limiterID int64, tunnelID int64) error {
	payload := map[string]interface{}{
		"limiter": strconv.FormatInt(limiterID, 10),
	}

	nodes, err := h.tunnelEntryNodeIDs(tunnelID)
	if err != nil {
		return err
	}

	for _, nodeID := range nodes {
		_, _ = h.sendNodeCommand(nodeID, "DeleteLimiters", payload, false, true)
	}
	return nil
}

func (h *Handler) ensureLimiterOnNode(nodeID int64, limiterID int64, speed int) {
	rate := float64(speed) / 8.0
	limitStr := fmt.Sprintf("$ %.1fMB %.1fMB", rate, rate)
	payload := map[string]interface{}{
		"name":   strconv.FormatInt(limiterID, 10),
		"limits": []string{limitStr},
	}
	_, _ = h.sendNodeCommand(nodeID, "AddLimiters", payload, false, false)
}
