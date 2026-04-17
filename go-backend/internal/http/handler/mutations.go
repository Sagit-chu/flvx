package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-backend/internal/http/client"
	"go-backend/internal/http/response"
	"go-backend/internal/security"
	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"

	"gorm.io/gorm"
)

const tunnelServiceBindRetryDelay = 150 * time.Millisecond

func (h *Handler) userCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req map[string]interface{}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}

	username := asString(req["user"])
	pwd := asString(req["pwd"])
	if username == "" || pwd == "" {
		response.WriteJSON(w, response.ErrDefault("用户名或密码不能为空"))
		return
	}

	exists, err := h.repo.UserExists(username)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if exists {
		response.WriteJSON(w, response.ErrDefault("用户名已存在"))
		return
	}

	status := asInt(req["status"], 1)
	flow := asInt64(req["flow"], 100)
	num := asInt(req["num"], 10)
	expTime := asInt64(req["expTime"], time.Now().Add(365*24*time.Hour).UnixMilli())
	flowResetTime := asInt64(req["flowResetTime"], 1)
	dailyQuotaGB := asInt64(req["dailyQuotaGB"], 0)
	monthlyQuotaGB := asInt64(req["monthlyQuotaGB"], 0)
	if dailyQuotaGB < 0 || monthlyQuotaGB < 0 {
		response.WriteJSON(w, response.ErrDefault("配额不能小于0"))
		return
	}
	roleID := 1
	now := time.Now().UnixMilli()

	userID, err := h.repo.CreateUser(username, security.MD5(pwd), roleID, expTime, flow, flowResetTime, num, status, now)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if dailyQuotaGB > 0 || monthlyQuotaGB > 0 {
		tx := h.repo.BeginTx()
		if tx == nil || tx.Error != nil {
			response.WriteJSON(w, response.Err(-2, "database unavailable"))
			return
		}
		defer func() { tx.Rollback() }()
		if err := h.repo.SaveUserQuotaConfigTx(tx, userID, dailyQuotaGB, monthlyQuotaGB, now); err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
		if err := tx.Commit().Error; err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
	}

	groupIDs := asInt64Slice(req["groupIds"])
	if len(groupIDs) > 0 {
		if addErr := h.repo.AddUserToGroups(userID, groupIDs, now); addErr == nil {
			for _, gid := range groupIDs {
				_ = h.syncPermissionsByUserGroup(gid)
			}
		}
	}

	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) userUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req map[string]interface{}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	id := asInt64(req["id"], 0)
	if id <= 0 {
		response.WriteJSON(w, response.ErrDefault("用户ID不能为空"))
		return
	}
	username := asString(req["user"])
	if username == "" {
		response.WriteJSON(w, response.ErrDefault("用户名不能为空"))
		return
	}

	roleID, err := h.repo.GetUserRoleID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			response.WriteJSON(w, response.ErrDefault("用户不存在"))
			return
		}
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if roleID == 0 {
		response.WriteJSON(w, response.ErrDefault("请不要作死"))
		return
	}

	dup, err := h.repo.UserExistsExcluding(username, id)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if dup {
		response.WriteJSON(w, response.ErrDefault("用户名已存在"))
		return
	}

	flow := asInt64(req["flow"], 100)
	num := asInt(req["num"], 10)
	expTime := asInt64(req["expTime"], time.Now().Add(365*24*time.Hour).UnixMilli())
	flowResetTime := asInt64(req["flowResetTime"], 1)
	status := asInt(req["status"], 1)
	_, hasDailyQuota := req["dailyQuotaGB"]
	_, hasMonthlyQuota := req["monthlyQuotaGB"]
	now := time.Now().UnixMilli()

	pwd := asString(req["pwd"])
	if strings.TrimSpace(pwd) == "" {
		if err := h.repo.UpdateUserWithoutPassword(id, username, flow, num, expTime, flowResetTime, status, now); err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
	} else {
		if err := h.repo.UpdateUserWithPassword(id, username, security.MD5(pwd), flow, num, expTime, flowResetTime, status, now); err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
	}

	h.repo.PropagateUserFlowToTunnels(id, flow, num, expTime, flowResetTime)
	if hasDailyQuota || hasMonthlyQuota {
		dailyQuotaGB := asInt64(req["dailyQuotaGB"], 0)
		monthlyQuotaGB := asInt64(req["monthlyQuotaGB"], 0)
		if !(hasDailyQuota && hasMonthlyQuota) {
			if currentQuota, err := h.repo.GetUserQuotaView(id, time.Now()); err == nil && currentQuota != nil {
				if !hasDailyQuota {
					dailyQuotaGB = currentQuota.DailyLimitGB
				}
				if !hasMonthlyQuota {
					monthlyQuotaGB = currentQuota.MonthlyLimitGB
				}
			}
		}
		tx := h.repo.BeginTx()
		if tx == nil || tx.Error != nil {
			response.WriteJSON(w, response.Err(-2, "database unavailable"))
			return
		}
		defer func() { tx.Rollback() }()
		if err := h.repo.SaveUserQuotaConfigTx(tx, id, dailyQuotaGB, monthlyQuotaGB, now); err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
		if err := tx.Commit().Error; err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
	}

	if groupIDsRaw, ok := req["groupIds"]; ok {
		newGroupIDs := asInt64Slice(groupIDsRaw)
		if affected, replaceErr := h.repo.ReplaceUserGroupsByUserID(id, newGroupIDs, now); replaceErr == nil {
			for _, gid := range affected {
				_ = h.syncPermissionsByUserGroup(gid)
			}
		}
	}

	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) userGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	ids, err := h.repo.GetUserGroupIDsByUserID(id)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(ids))
}

func (h *Handler) userDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}

	roleID, err := h.repo.GetUserRoleID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			response.WriteJSON(w, response.ErrDefault("用户不存在"))
			return
		}
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if roleID == 0 {
		response.WriteJSON(w, response.ErrDefault("请不要作死"))
		return
	}

	if err := h.repo.DeleteUserCascade(id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) userResetFlow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req map[string]interface{}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	id := asInt64(req["id"], 0)
	typeVal := asInt(req["type"], 0)
	if id <= 0 || (typeVal != 1 && typeVal != 2) {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}

	if typeVal == 1 {
		h.repo.ResetUserFlowByUser(id, time.Now().UnixMilli())
	} else {
		h.repo.ResetUserFlowByUserTunnel(id)
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) nodeCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req map[string]interface{}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	name := asString(req["name"])
	serverIP := asString(req["serverIp"])
	if name == "" || serverIP == "" {
		response.WriteJSON(w, response.ErrDefault("节点名称和地址不能为空"))
		return
	}
	if err := IsValidNodeAddress(serverIP); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}

	now := time.Now().UnixMilli()
	inx := h.repo.NextIndex("node")
	if err := h.repo.CreateNode(
		name,
		randomToken(16),
		serverIP,
		nullableText(asString(req["serverIpV4"])),
		nullableText(asString(req["serverIpV6"])),
		defaultString(asString(req["port"]), "1000-65535"),
		nullableText(asString(req["interfaceName"])),
		nullableText(""),
		nullableText(strings.TrimSpace(asString(req["remark"]))),
		nullableUnixMilli(asInt64(req["expiryTime"], 0)),
		nullableText(normalizeNodeRenewalCycle(asString(req["renewalCycle"]))),
		asInt(req["http"], 0),
		asInt(req["tls"], 0),
		asInt(req["socks"], 0),
		now,
		0,
		defaultString(asString(req["tcpListenAddr"]), "[::]"),
		defaultString(asString(req["udpListenAddr"]), "[::]"),
		inx,
		asInt(req["isRemote"], 0),
		nullableText(asString(req["remoteUrl"])),
		nullableText(asString(req["remoteToken"])),
		nullableText(asString(req["remoteConfig"])),
		nullableText(asString(req["extraIPs"])),
	); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) nodeUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req map[string]interface{}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	id := asInt64(req["id"], 0)
	if id <= 0 {
		response.WriteJSON(w, response.ErrDefault("节点ID不能为空"))
		return
	}

	currentStatus, currentHTTP, currentTLS, currentSocks, err := h.repo.GetNodeStatusFields(id)
	if err != nil {
		if err == sql.ErrNoRows {
			response.WriteJSON(w, response.ErrDefault("节点不存在"))
			return
		}
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	newHTTP := asInt(req["http"], currentHTTP)
	newTLS := asInt(req["tls"], currentTLS)
	newSocks := asInt(req["socks"], currentSocks)
	
	serverIP := asString(req["serverIp"])
	if serverIP != "" {
		if err := IsValidNodeAddress(serverIP); err != nil {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
	}

	if currentStatus == 1 && (newHTTP != currentHTTP || newTLS != currentTLS || newSocks != currentSocks) {
		if err := h.applyNodeProtocolChange(id, newHTTP, newTLS, newSocks); err != nil {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
	}

	now := time.Now().UnixMilli()
	if err := h.repo.UpdateNode(id,
		asString(req["name"]),
		serverIP,
		nullableText(asString(req["serverIpV4"])),
		nullableText(asString(req["serverIpV6"])),
		defaultString(asString(req["port"]), "1000-65535"),
		nullableText(asString(req["interfaceName"])),
		nullableText(asString(req["extraIPs"])),
		nullableText(strings.TrimSpace(asString(req["remark"]))),
		nullableUnixMilli(asInt64(req["expiryTime"], 0)),
		nullableText(normalizeNodeRenewalCycle(asString(req["renewalCycle"]))),
		newHTTP,
		newTLS,
		newSocks,
		defaultString(asString(req["tcpListenAddr"]), "[::]"),
		defaultString(asString(req["udpListenAddr"]), "[::]"),
		now,
	); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) nodeDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	if err := h.deleteNodeByID(id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) nodeInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req struct {
		ID      int64  `json:"id"`
		Channel string `json:"channel"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if req.ID <= 0 {
		response.WriteJSON(w, response.ErrDefault("参数错误"))
		return
	}

	channel := normalizeReleaseChannel(req.Channel)
	version, err := resolveLatestReleaseByChannel(channel)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, fmt.Sprintf("获取最新%s失败: %v", releaseChannelLabel(channel), err)))
		return
	}

	secret, err := h.repo.GetNodeSecret(req.ID)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("节点不存在"))
		return
	}
	panelAddr, err := h.repo.GetViteConfigValue("ip")
	if err != nil {
		if err == sql.ErrNoRows {
			response.WriteJSON(w, response.ErrDefault("请先前往网站配置中设置ip"))
			return
		}
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	enabled, proxyURL := h.getGithubProxyConfig()

	var cmd string
	if enabled {
		cmd = fmt.Sprintf("curl -L %s/https://github.com/%s/releases/download/%s/install.sh -o ./install.sh && chmod +x ./install.sh && PROXY_ENABLED=true PROXY_URL=%s VERSION=%s ./install.sh -a %s -s %s",
			proxyURL, githubRepo, version, proxyURL, version, processServerAddress(panelAddr), secret)
	} else {
		cmd = fmt.Sprintf("curl -L https://github.com/%s/releases/download/%s/install.sh -o ./install.sh && chmod +x ./install.sh && PROXY_ENABLED=false VERSION=%s ./install.sh -a %s -s %s",
			githubRepo, version, version, processServerAddress(panelAddr), secret)
	}
	response.WriteJSON(w, response.OK(cmd))
}

func (h *Handler) nodeUpdateOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req struct {
		Nodes []struct {
			ID  int64 `json:"id"`
			Inx int   `json:"inx"`
		} `json:"nodes"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	for _, n := range req.Nodes {
		h.repo.UpdateNodeOrder(n.ID, n.Inx, time.Now().UnixMilli())
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) nodeDismissExpiryReminder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req struct {
		ID int64 `json:"id"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if req.ID <= 0 {
		response.WriteJSON(w, response.ErrDefault("节点ID不能为空"))
		return
	}
	if err := h.repo.UpdateNodeExpiryReminderDismissed(req.ID, 1); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) nodeBatchDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	ids := idsFromBody(r, w)
	if ids == nil {
		return
	}
	for _, id := range ids {
		_ = h.deleteNodeByID(id)
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) nodeCheckStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	items, err := h.repo.ListNodes()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	h.syncRemoteNodeStatuses(items)
	h.overlayCurrentRuntimeNodeStatuses(r.Context(), items)
	response.WriteJSON(w, response.OK(items))
}

func (h *Handler) tunnelCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req map[string]interface{}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if err := validateTunnelConnectIPConstraints(req); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	name := asString(req["name"])
	if name == "" {
		response.WriteJSON(w, response.ErrDefault("隧道名称不能为空"))
		return
	}
	nameDup, err := h.repo.TunnelNameExists(name)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if nameDup {
		response.WriteJSON(w, response.ErrDefault("隧道名称重复"))
		return
	}

	typeVal := asInt(req["type"], 1)
	flow := asInt64(req["flow"], 1)
	status := asInt(req["status"], 1)
	trafficRatio := asFloat(req["trafficRatio"], 1.0)
	inIP := asString(req["inIp"])
	ipPreference := asString(req["ipPreference"])
	now := time.Now().UnixMilli()
	inx := h.repo.NextIndex("tunnel")
	localDomain := h.federationLocalDomain()

	tx := h.repo.BeginTx()
	if tx.Error != nil {
		response.WriteJSON(w, response.Err(-2, tx.Error.Error()))
		return
	}
	defer func() { tx.Rollback() }()

	runtimeState, err := h.prepareTunnelCreateState(tx, req, typeVal, 0)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	runtimeState.IPPreference = ipPreference
	if strings.TrimSpace(inIP) == "" {
		inIP = buildTunnelInIP(runtimeState.InNodes, runtimeState.Nodes, ipPreference)
	}

	if len(runtimeState.InNodes) > 0 {
		firstNodeID := runtimeState.InNodes[0].NodeID
		isRemote, rUrl, rToken, _ := h.repo.GetNodeRemoteFieldsTx(tx, firstNodeID)
		if isRemote == 1 {
			fc := client.NewFederationClient()

			targetProto := "tcp"
			targetPort := 0
			targetAddr := ""

			if typeVal == 1 {
				if len(runtimeState.OutNodes) > 0 {
					outNode := runtimeState.OutNodes[0]
					outNodeRec := runtimeState.Nodes[outNode.NodeID]
					targetAddr = processServerAddress(outNodeRec.ServerIP)
					if outNode.Port > 0 {
						targetAddr = fmt.Sprintf("%s:%d", targetAddr, outNode.Port)
					}
				}
				if len(runtimeState.InNodes) > 0 {
					inNodesRaw := asMapSlice(req["inNodeId"])
					if len(inNodesRaw) > 0 {
						targetPort = asInt(inNodesRaw[0]["port"], 0)
						targetProto = defaultString(asString(inNodesRaw[0]["protocol"]), "tcp")
					}
				}

				if targetPort > 0 && targetAddr != "" {
					inNodeRec := runtimeState.Nodes[firstNodeID]
					if err := validateRemoteNodePort(inNodeRec, targetPort); err != nil {
						response.WriteJSON(w, response.ErrDefault(err.Error()))
						return
					}
					domainCfg, _ := h.repo.GetConfigByName("panel_domain")
					localDomain := ""
					if domainCfg != nil {
						localDomain = domainCfg.Value
					}
					_, err := fc.CreateTunnel(rUrl.String, rToken.String, localDomain, targetProto, targetPort, targetAddr)
					if err != nil {
						response.WriteJSON(w, response.ErrDefault("Remote tunnel creation failed: "+err.Error()))
						return
					}
				}
			}
		}
	}

	var tunnelInIP sql.NullString
	if trimmed := strings.TrimSpace(inIP); trimmed != "" {
		tunnelInIP = sql.NullString{String: trimmed, Valid: true}
	}
	tunnel := model.Tunnel{
		Name:         name,
		TrafficRatio: trafficRatio,
		Type:         typeVal,
		Protocol:     "tls",
		Flow:         flow,
		CreatedTime:  now,
		UpdatedTime:  now,
		Status:       status,
		InIP:         tunnelInIP,
		Inx:          inx,
		IPPreference: ipPreference,
	}
	if err := tx.Create(&tunnel).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	tunnelID := tunnel.ID
	runtimeState.TunnelID = tunnelID
	var federationBindings []repo.FederationTunnelBinding
	var federationReleaseRefs []federationRuntimeReleaseRef
	federationBindings, federationReleaseRefs, err = h.applyFederationRuntime(runtimeState, localDomain)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	applyTunnelPortsToRequest(req, runtimeState)
	if err := h.replaceTunnelChainsTx(tx, tunnelID, req); err != nil {
		h.releaseFederationRuntimeRefs(federationReleaseRefs)
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := h.repo.ReplaceFederationTunnelBindingsTx(tx, tunnelID, federationBindings); err != nil {
		h.releaseFederationRuntimeRefs(federationReleaseRefs)
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := tx.Commit().Error; err != nil {
		h.releaseFederationRuntimeRefs(federationReleaseRefs)
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if typeVal == 2 {
		createdChains, createdServices, applyErr := h.applyTunnelRuntimeForCurrentEngine(r.Context(), runtimeState, func() ([]int64, []int64, error) {
			return h.applyTunnelRuntime(runtimeState)
		})
		if applyErr != nil {
			h.rollbackTunnelRuntime(createdChains, createdServices, tunnelID)
			h.releaseFederationRuntimeRefs(federationReleaseRefs)
			_ = h.deleteTunnelByID(tunnelID)
			response.WriteJSON(w, response.ErrDefault(applyErr.Error()))
			return
		}
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) cleanupTunnelRuntime(tunnelID int64) {
	tunnel, err := h.getTunnelRecord(tunnelID)
	if err != nil || tunnel.Type != 2 {
		return
	}
	chainRows, err := h.listChainNodesForTunnel(tunnelID)
	if err != nil {
		return
	}

	serviceName := fmt.Sprintf("%d_tls", tunnelID)
	chainName := fmt.Sprintf("chains_%d", tunnelID)

	for _, row := range chainRows {
		if row.ChainType == 1 {
			_, _ = h.sendNodeCommand(row.NodeID, "DeleteChains", map[string]interface{}{"chain": chainName}, false, true)
		} else if row.ChainType == 2 {
			_, _ = h.sendNodeCommand(row.NodeID, "DeleteChains", map[string]interface{}{"chain": chainName}, false, true)
			_, _ = h.sendNodeCommand(row.NodeID, "DeleteService", map[string]interface{}{"services": []string{serviceName}}, false, true)
		} else if row.ChainType == 3 {
			_, _ = h.sendNodeCommand(row.NodeID, "DeleteService", map[string]interface{}{"services": []string{serviceName}}, false, true)
		}
	}
}

func (h *Handler) tunnelGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	items, err := h.repo.ListTunnels()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	for _, it := range items {
		if asInt64(it["id"], 0) == id {
			response.WriteJSON(w, response.OK(it))
			return
		}
	}
	response.WriteJSON(w, response.ErrDefault("隧道不存在"))
}

func (h *Handler) tunnelUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req map[string]interface{}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if err := validateTunnelConnectIPConstraints(req); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	id := asInt64(req["id"], 0)
	if id <= 0 {
		response.WriteJSON(w, response.ErrDefault("隧道ID不能为空"))
		return
	}
	oldChainRows, _ := h.listChainNodesForTunnel(id)
	oldEntryNodeIDs, _ := h.tunnelEntryNodeIDs(id)

	h.cleanupTunnelRuntime(id)
	h.cleanupFederationRuntime(id)

	now := time.Now().UnixMilli()
	typeVal := asInt(req["type"], 1)
	ipPreference := asString(req["ipPreference"])
	localDomain := h.federationLocalDomain()

	runtimeState, err := h.prepareTunnelCreateState(h.repo.DB(), req, typeVal, id)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	runtimeState.TunnelID = id
	runtimeState.IPPreference = ipPreference

	inIp := buildTunnelInIP(runtimeState.InNodes, runtimeState.Nodes, ipPreference)

	var federationBindings []repo.FederationTunnelBinding
	var federationReleaseRefs []federationRuntimeReleaseRef
	federationBindings, federationReleaseRefs, err = h.applyFederationRuntime(runtimeState, localDomain)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	applyTunnelPortsToRequest(req, runtimeState)

	tx := h.repo.BeginTx()
	if tx.Error != nil {
		h.releaseFederationRuntimeRefs(federationReleaseRefs)
		response.WriteJSON(w, response.Err(-2, tx.Error.Error()))
		return
	}
	defer func() { tx.Rollback() }()

	if err := h.repo.UpdateTunnelTx(
		tx,
		id,
		asString(req["name"]),
		typeVal,
		asInt64(req["flow"], 1),
		asFloat(req["trafficRatio"], 1.0),
		asInt(req["status"], 1),
		inIp,
		ipPreference,
		now,
	); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	if err := h.repo.DeleteChainTunnelsByTunnelTx(tx, id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := h.replaceTunnelChainsTx(tx, id, req); err != nil {
		h.releaseFederationRuntimeRefs(federationReleaseRefs)
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := h.repo.ReplaceFederationTunnelBindingsTx(tx, id, federationBindings); err != nil {
		h.releaseFederationRuntimeRefs(federationReleaseRefs)
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	newEntryNodeIDs := make([]int64, 0, len(runtimeState.InNodes))
	for _, in := range runtimeState.InNodes {
		if in.NodeID > 0 {
			newEntryNodeIDs = append(newEntryNodeIDs, in.NodeID)
		}
	}
	if err := h.validateTunnelEntryPortConflictsForNewEntriesTx(tx, id, oldEntryNodeIDs, newEntryNodeIDs); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}

	if err := tx.Commit().Error; err != nil {
		h.releaseFederationRuntimeRefs(federationReleaseRefs)
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	newEntryNodeIDs, _ = h.tunnelEntryNodeIDs(id)
	if !sameInt64Set(oldEntryNodeIDs, newEntryNodeIDs) {
		h.cleanupTunnelForwardRuntimesOnRemovedEntryNodes(id, oldEntryNodeIDs, newEntryNodeIDs)
		h.syncTunnelForwardsEntryPorts(id, newEntryNodeIDs)
	}

	if typeVal == 2 {
		createdChains, createdServices, applyErr := h.reconcileTunnelRuntimeForCurrentEngine(r.Context(), oldChainRows, runtimeState, func() ([]int64, []int64, error) {
			return h.applyTunnelRuntime(runtimeState)
		})
		if applyErr != nil {
			h.rollbackTunnelRuntime(createdChains, createdServices, id)
			h.releaseFederationRuntimeRefs(federationReleaseRefs)
			_ = h.repo.DeleteFederationTunnelBindingsByTunnel(id)
			if len(federationReleaseRefs) == 0 && shouldDeferTunnelRuntimeApplyError(applyErr) {
				response.WriteJSON(w, response.OKEmpty())
				return
			}
			response.WriteJSON(w, response.ErrDefault(applyErr.Error()))
			return
		}
	}

	if forwards, fwdErr := h.listForwardsByTunnel(id); fwdErr == nil {
		_ = h.applyForwardRuntimeForCurrentEngine(r.Context(), id, func() error {
			for i := range forwards {
				if err := h.syncForwardServices(&forwards[i], "UpdateService", true); err != nil {
					return err
				}
			}
			return nil
		})
	}

	response.WriteJSON(w, response.OKEmpty())
}

func sameInt64Set(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	m := make(map[int64]int, len(a))
	for _, v := range a {
		m[v]++
	}
	for _, v := range b {
		c, ok := m[v]
		if !ok || c <= 0 {
			return false
		}
		if c == 1 {
			delete(m, v)
			continue
		}
		m[v] = c - 1
	}
	return len(m) == 0
}

func pickForwardPortFromRecords(ports []forwardPortRecord) int {
	min := 0
	for _, fp := range ports {
		if fp.Port <= 0 {
			continue
		}
		if min == 0 || fp.Port < min {
			min = fp.Port
		}
	}
	return min
}

func forwardPortNodeIDs(ports []forwardPortRecord) []int64 {
	ids := make([]int64, 0, len(ports))
	for _, fp := range ports {
		if fp.NodeID <= 0 {
			continue
		}
		ids = append(ids, fp.NodeID)
	}
	return uniqueInt64s(ids)
}

func (h *Handler) deleteForwardServicesOnNodeBatch(forward *forwardRecord, nodeID int64) error {
	if h == nil || forward == nil || nodeID <= 0 {
		return errors.New("invalid forward service cleanup context")
	}
	bases, err := h.forwardServiceBaseCandidates(forward)
	if err != nil {
		return err
	}
	if len(bases) == 0 {
		return nil
	}

	names := make([]string, 0, len(bases)*3)
	seen := make(map[string]struct{}, len(bases)*3)
	appendName := func(name string) {
		if strings.TrimSpace(name) == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	for _, base := range bases {
		appendName(base + "_tcp")
		appendName(base + "_udp")
		appendName(base)
	}
	if len(names) == 0 {
		return nil
	}

	payload := map[string]interface{}{"services": names}
	_, err = h.sendNodeCommand(nodeID, "DeleteService", payload, false, true)
	return err
}

func uniqueInt64s(input []int64) []int64 {
	if len(input) <= 1 {
		return input
	}
	seen := make(map[int64]struct{}, len(input))
	out := make([]int64, 0, len(input))
	for _, v := range input {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func diffInt64s(base, subtract []int64) []int64 {
	if len(base) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(subtract))
	for _, v := range subtract {
		seen[v] = struct{}{}
	}
	out := make([]int64, 0, len(base))
	for _, v := range base {
		if _, ok := seen[v]; ok {
			continue
		}
		out = append(out, v)
	}
	return uniqueInt64s(out)
}

func (h *Handler) cleanupTunnelForwardRuntimesOnRemovedEntryNodes(tunnelID int64, oldEntryNodeIDs, newEntryNodeIDs []int64) {
	if h == nil || h.repo == nil || tunnelID <= 0 {
		return
	}

	removedNodeIDs := diffInt64s(oldEntryNodeIDs, newEntryNodeIDs)
	if len(removedNodeIDs) == 0 {
		return
	}

	forwards, err := h.listForwardsByTunnel(tunnelID)
	if err != nil || len(forwards) == 0 {
		return
	}

	for i := range forwards {
		f := &forwards[i]
		if f == nil {
			continue
		}
		for _, nodeID := range removedNodeIDs {
			_ = h.deleteForwardServicesOnNode(f, nodeID)
		}
	}
}

func (h *Handler) validateForwardPortAvailabilityTx(tx *gorm.DB, node *nodeRecord, port int, currentForwardID int64) error {
	if h == nil || h.repo == nil || tx == nil || node == nil || port <= 0 {
		return nil
	}
	occupied, err := h.repo.HasOtherForwardOnNodePortTx(tx, node.ID, port, currentForwardID)
	if err != nil {
		return err
	}
	if occupied {
		return fmt.Errorf("节点 %s 端口 %d 已被其他转发占用", node.Name, port)
	}
	return nil
}

func (h *Handler) validateTunnelEntryPortConflictsForNewEntriesTx(tx *gorm.DB, tunnelID int64, oldEntryNodeIDs, newEntryNodeIDs []int64) error {
	if h == nil || h.repo == nil || tx == nil || tunnelID <= 0 {
		return nil
	}

	addedNodeIDs := diffInt64s(newEntryNodeIDs, oldEntryNodeIDs)
	if len(addedNodeIDs) == 0 {
		return nil
	}

	forwards, err := h.repo.ListForwardsByTunnelTx(tx, tunnelID)
	if err != nil || len(forwards) == 0 {
		return nil
	}

	for i := range forwards {
		f := &forwards[i]
		if f == nil {
			continue
		}
		oldPorts, portsErr := h.repo.ListForwardPortsTx(tx, f.ID)
		if portsErr != nil {
			continue
		}
		port := pickForwardPortFromRecords(oldPorts)
		if port <= 0 {
			continue
		}

		for _, nodeID := range addedNodeIDs {
			node, nodeErr := h.repo.GetNodeRecordTx(tx, nodeID)
			if nodeErr != nil {
				continue
			}

			if err := h.validateForwardPortAvailabilityTx(tx, node, port, f.ID); err != nil {
				return fmt.Errorf("转发 %s 入口端口冲突: %w", f.Name, err)
			}
		}
	}

	return nil
}

func (h *Handler) syncTunnelForwardsEntryPorts(tunnelID int64, entryNodeIDs []int64) {
	if h == nil || h.repo == nil || tunnelID <= 0 {
		return
	}
	entryNodeIDs = uniqueInt64s(entryNodeIDs)
	if len(entryNodeIDs) == 0 {
		return
	}

	forwards, err := h.listForwardsByTunnel(tunnelID)
	if err != nil || len(forwards) == 0 {
		return
	}

	allowInIP := len(entryNodeIDs) == 1
	for i := range forwards {
		f := &forwards[i]
		if f == nil {
			continue
		}
		oldPorts, err := h.listForwardPorts(f.ID)
		if err != nil {
			continue
		}
		referencePort := pickForwardPortFromRecords(oldPorts)
		if referencePort <= 0 {
			continue
		}

		// Build a map of existing node → port/inIP from old records.
		oldPortByNode := make(map[int64]forwardPortRecord)
		for _, fp := range oldPorts {
			if fp.NodeID > 0 {
				oldPortByNode[fp.NodeID] = fp
			}
		}

		entries := make([]forwardPortReplaceEntry, 0, len(entryNodeIDs))
		for _, nid := range entryNodeIDs {
			if existing, ok := oldPortByNode[nid]; ok && existing.Port > 0 {
				// Existing entry node: keep its current port.
				inIP := existing.InIP
				if !allowInIP {
					inIP = ""
				}
				entries = append(entries, forwardPortReplaceEntry{NodeID: nid, Port: existing.Port, InIP: inIP})
				continue
			}

			// New entry node: try to follow the reference port.
			port := h.resolvePortForNewEntryNode(nid, referencePort, f.ID)
			inIP := ""
			if allowInIP {
				// For single-entry tunnels, try to preserve inIP from old records.
				for _, fp := range oldPorts {
					if strings.TrimSpace(fp.InIP) != "" {
						inIP = fp.InIP
						break
					}
				}
			}
			entries = append(entries, forwardPortReplaceEntry{NodeID: nid, Port: port, InIP: inIP})
		}
		_ = h.repo.ReplaceForwardPorts(f.ID, entries)
	}
}

// resolvePortForNewEntryNode determines the port for a forward on a newly added
// entry node. It tries to reuse referencePort (from existing entries); if that
// port is out of range or already occupied, it picks a random available port
// for this specific node.
func (h *Handler) resolvePortForNewEntryNode(nodeID int64, referencePort int, forwardID int64) int {
	node, err := h.getNodeRecord(nodeID)
	if err != nil {
		return referencePort
	}

	// Check if referencePort is within the node's allowed range.
	if validateLocalNodePort(node, referencePort) == nil &&
		validateRemoteNodePort(node, referencePort) == nil {
		// In range — check availability.
		occupied, occErr := h.repo.HasOtherForwardOnNodePort(nodeID, referencePort, forwardID)
		if occErr == nil && !occupied {
			return referencePort
		}
	}

	// referencePort doesn't work for this node; pick a random one.
	newPort := h.pickRandomPortForNode(nodeID)
	if newPort > 0 {
		return newPort
	}
	return referencePort // last resort fallback
}

// pickRandomPortForNode picks a random available port from a single node's
// port range, excluding ports already occupied by other forwards or chains.
func (h *Handler) pickRandomPortForNode(nodeID int64) int {
	portRange, err := h.repo.GetNodePortRange(nodeID)
	if err != nil {
		return 0
	}
	if portRange == "" {
		portRange = "1000-65535"
	}

	nodePorts, err := parsePorts(portRange)
	if err != nil || len(nodePorts) == 0 {
		return 0
	}

	used, err := h.getUsedPorts(nodeID)
	if err != nil {
		return 0
	}

	var available []int
	for _, p := range nodePorts {
		if !used[p] {
			available = append(available, p)
		}
	}

	if len(available) == 0 {
		return 0
	}

	idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(available))))
	return available[idx.Int64()]
}

func (h *Handler) tunnelDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	h.cleanupTunnelRuntime(id)
	h.cleanupFederationRuntime(id)
	if err := h.deleteTunnelByID(id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) tunnelDiagnose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	id := asInt64FromBodyKey(r, w, "tunnelId")
	if id <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), diagnosisRequestTimeout)
	defer cancel()
	result, err := h.diagnoseTunnelRuntime(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "不存在") || strings.Contains(err.Error(), "不完整") {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(result))
}

func (h *Handler) tunnelUpdateOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req struct {
		Tunnels []struct {
			ID  int64 `json:"id"`
			Inx int   `json:"inx"`
		} `json:"tunnels"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	for _, t := range req.Tunnels {
		h.repo.UpdateTunnelOrder(t.ID, t.Inx, time.Now().UnixMilli())
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) tunnelBatchDelete(w http.ResponseWriter, r *http.Request) {
	ids := idsFromBody(r, w)
	if ids == nil {
		return
	}
	success := 0
	fail := 0
	failures := make([]batchFailureDetail, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			tunnelName, _ := h.repo.GetTunnelName(id)
			if _, err := h.getTunnelRecord(id); err != nil {
				mu.Lock()
				fail++
				failures = appendBatchFailure(failures, id, tunnelName, err)
				mu.Unlock()
				return
			}
			h.cleanupTunnelRuntime(id)
			h.cleanupFederationRuntime(id)
			if err := h.deleteTunnelByID(id); err != nil {
				mu.Lock()
				fail++
				failures = appendBatchFailure(failures, id, tunnelName, err)
				mu.Unlock()
			} else {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}(id)
	}
	wg.Wait()
	response.WriteJSON(w, response.OK(batchOperationResult{SuccessCount: success, FailCount: fail, Failures: failures}))
}

func (h *Handler) reconstructTunnelState(tunnelID int64) (*tunnelCreateState, error) {
	tunnel, err := h.getTunnelRecord(tunnelID)
	if err != nil {
		return nil, err
	}

	chainRows, err := h.listChainNodesForTunnel(tunnelID)
	if err != nil {
		return nil, err
	}

	ipPreference := h.repo.GetTunnelIPPreference(tunnelID)

	state := &tunnelCreateState{
		TunnelID:     tunnelID,
		Type:         tunnel.Type,
		IPPreference: ipPreference,
		InNodes:      make([]tunnelRuntimeNode, 0),
		ChainHops:    make([][]tunnelRuntimeNode, 0),
		OutNodes:     make([]tunnelRuntimeNode, 0),
		Nodes:        make(map[int64]*nodeRecord),
		NodeIDList:   make([]int64, 0),
	}

	inNodes, chainHops, outNodes := splitChainNodeGroups(chainRows)

	for _, r := range inNodes {
		state.InNodes = append(state.InNodes, tunnelRuntimeNode{
			NodeID:    r.NodeID,
			Protocol:  r.Protocol,
			Strategy:  r.Strategy,
			ChainType: 1,
		})
		state.NodeIDList = append(state.NodeIDList, r.NodeID)
	}

	for _, r := range outNodes {
		state.OutNodes = append(state.OutNodes, tunnelRuntimeNode{
			NodeID:    r.NodeID,
			Protocol:  r.Protocol,
			Strategy:  r.Strategy,
			ChainType: 3,
			Port:      r.Port,
			ConnectIP: r.ConnectIP,
		})
		state.NodeIDList = append(state.NodeIDList, r.NodeID)
	}

	for _, hop := range chainHops {
		stateHop := make([]tunnelRuntimeNode, 0)
		for _, r := range hop {
			stateHop = append(stateHop, tunnelRuntimeNode{
				NodeID:    r.NodeID,
				Protocol:  r.Protocol,
				Strategy:  r.Strategy,
				ChainType: 2,
				Inx:       int(r.Inx),
				Port:      r.Port,
				ConnectIP: r.ConnectIP,
			})
			state.NodeIDList = append(state.NodeIDList, r.NodeID)
		}
		state.ChainHops = append(state.ChainHops, stateHop)
	}

	seen := make(map[int64]struct{})
	for _, id := range state.NodeIDList {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		node, err := h.getNodeRecord(id)
		if err != nil {
			return nil, err
		}
		state.Nodes[id] = node
	}

	return state, nil
}

func (h *Handler) redeployTunnelAndForwards(tunnelID int64) error {
	tunnel, err := h.getTunnelRecord(tunnelID)
	if err != nil {
		return err
	}

	if tunnel.Type == 2 {
		h.cleanupTunnelRuntime(tunnelID)
		h.cleanupFederationRuntime(tunnelID)
		state, err := h.reconstructTunnelState(tunnelID)
		if err != nil {
			return err
		}
		federationBindings, federationReleaseRefs, fedErr := h.applyFederationRuntime(state, h.federationLocalDomain())
		if fedErr != nil {
			return fedErr
		}
		tx := h.repo.BeginTx()
		if tx.Error != nil {
			h.releaseFederationRuntimeRefs(federationReleaseRefs)
			return tx.Error
		}
		if replaceErr := h.repo.ReplaceFederationTunnelBindingsTx(tx, tunnelID, federationBindings); replaceErr != nil {
			tx.Rollback()
			h.releaseFederationRuntimeRefs(federationReleaseRefs)
			return replaceErr
		}
		if commitErr := tx.Commit().Error; commitErr != nil {
			h.releaseFederationRuntimeRefs(federationReleaseRefs)
			return commitErr
		}
		_, _, applyErr := h.applyTunnelRuntime(state)
		if applyErr != nil {
			h.releaseFederationRuntimeRefs(federationReleaseRefs)
			_ = h.repo.DeleteFederationTunnelBindingsByTunnel(tunnelID)
			return applyErr
		}
	}

	forwards, err := h.listForwardsByTunnel(tunnelID)
	if err != nil {
		return err
	}
	for i := range forwards {
		if err := h.syncForwardServices(&forwards[i], "UpdateService", true); err != nil {
			return err
		}
	}

	return nil
}

type batchFailureDetail struct {
	ID     int64  `json:"id"`
	Name   string `json:"name,omitempty"`
	Reason string `json:"reason"`
}

type batchOperationResult struct {
	SuccessCount int                  `json:"successCount"`
	FailCount    int                  `json:"failCount"`
	Failures     []batchFailureDetail `json:"failures,omitempty"`
}

func appendBatchFailure(failures []batchFailureDetail, id int64, name string, err error) []batchFailureDetail {
	reason := normalizeBatchFailureReason(errString(err))
	if reason == "" {
		reason = "未知错误"
	}
	return append(failures, batchFailureDetail{
		ID:     id,
		Name:   strings.TrimSpace(name),
		Reason: reason,
	})
}

func appendBatchFailureReason(failures []batchFailureDetail, id int64, name, reason string) []batchFailureDetail {
	normalized := normalizeBatchFailureReason(reason)
	if normalized == "" {
		normalized = "未知错误"
	}
	return append(failures, batchFailureDetail{
		ID:     id,
		Name:   strings.TrimSpace(name),
		Reason: normalized,
	})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func normalizeBatchFailureReason(reason string) string {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return ""
	}
	if strings.EqualFold(trimmed, errForwardNotFound.Error()) {
		return "转发不存在"
	}
	return trimmed
}

func (h *Handler) tunnelBatchRedeploy(w http.ResponseWriter, r *http.Request) {
	ids := idsFromBody(r, w)
	if ids == nil {
		return
	}
	success := 0
	fail := 0
	failures := make([]batchFailureDetail, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, tunnelID := range ids {
		wg.Add(1)
		go func(tunnelID int64) {
			defer wg.Done()
			tunnel, tunnelErr := h.getTunnelRecord(tunnelID)
			tunnelName := ""
			if tunnelErr == nil && tunnel != nil {
				tunnelName, _ = h.repo.GetTunnelName(tunnelID)
			}
			if tunnelErr != nil {
				mu.Lock()
				fail++
				failures = appendBatchFailure(failures, tunnelID, tunnelName, tunnelErr)
				mu.Unlock()
				return
			}
			if err := h.redeployTunnelAndForwards(tunnelID); err != nil {
				mu.Lock()
				fail++
				failures = appendBatchFailure(failures, tunnelID, tunnelName, err)
				mu.Unlock()
				return
			}
			mu.Lock()
			success++
			mu.Unlock()
		}(tunnelID)
	}
	wg.Wait()
	response.WriteJSON(w, response.OK(batchOperationResult{SuccessCount: success, FailCount: fail, Failures: failures}))
}

func (h *Handler) userTunnelAssign(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if err := h.upsertUserTunnel(req); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) userTunnelBatchAssign(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID  int64 `json:"userId"`
		Tunnels []struct {
			TunnelID int64  `json:"tunnelId"`
			SpeedID  *int64 `json:"speedId"`
		} `json:"tunnels"`
	}
	if err := decodeJSON(r.Body, &req); err != nil || req.UserID <= 0 {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	for _, t := range req.Tunnels {
		m := map[string]interface{}{"userId": req.UserID, "tunnelId": t.TunnelID}
		if t.SpeedID != nil {
			m["speedId"] = *t.SpeedID
		}
		if err := h.upsertUserTunnel(m); err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) userTunnelRemove(w http.ResponseWriter, r *http.Request) {
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	userID, tunnelID, lookupErr := h.repo.GetUserTunnelUserAndTunnel(id)
	if lookupErr != nil {
		response.WriteJSON(w, response.Err(-2, lookupErr.Error()))
		return
	}
	h.cleanupForwardsForUserTunnel(userID, tunnelID)
	if err := h.repo.DeleteUserTunnel(id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) userTunnelUpdate(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	id := asInt64(req["id"], 0)
	if id <= 0 {
		response.WriteJSON(w, response.ErrDefault("权限ID不能为空"))
		return
	}

	speedID := asAnyToInt64Ptr(req["speedId"])
	speedID, err := h.normalizeSpeedLimitReference(speedID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	userID, tunnelID, utErr := h.repo.GetUserTunnelUserAndTunnel(id)
	if utErr != nil {
		response.WriteJSON(w, response.Err(-2, utErr.Error()))
		return
	}

	_, oldFlow, oldNum, oldExpTime, oldFlowReset, oldSpeedID, oldStatus, oldErr :=
		h.repo.GetExistingUserTunnel(userID, tunnelID)
	if oldErr != nil {
		response.WriteJSON(w, response.Err(-2, oldErr.Error()))
		return
	}

	if err := h.repo.UpdateUserTunnel(id,
		asInt64(req["flow"], 0),
		asInt(req["num"], 0),
		asInt64(req["expTime"], time.Now().Add(365*24*time.Hour).UnixMilli()),
		asInt64(req["flowResetTime"], 1),
		nullableInt(speedID),
		asInt(req["status"], 1),
	); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	if syncErr := h.syncUserTunnelForwards(userID, tunnelID); syncErr != nil {
		rollbackErr := h.repo.UpdateUserTunnel(
			id,
			oldFlow,
			int(oldNum),
			oldExpTime,
			oldFlowReset,
			oldSpeedID,
			oldStatus,
		)
		if rollbackErr != nil {
			response.WriteJSON(w, response.Err(-2, fmt.Sprintf("下发失败且回滚失败: %v; 回滚错误: %v", syncErr, rollbackErr)))
			return
		}

		response.WriteJSON(w, response.Err(-2, fmt.Sprintf("下发失败，已回滚: %v", syncErr)))
		return
	}

	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) forwardCreate(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	userID, roleID, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	tunnelID := asInt64(req["tunnelId"], 0)
	if tunnelID <= 0 {
		response.WriteJSON(w, response.ErrDefault("隧道ID不能为空"))
		return
	}
	if err := h.ensureTunnelPermission(userID, roleID, tunnelID); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	tunnel, err := h.getTunnelRecord(tunnelID)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("隧道不存在"))
		return
	}
	if tunnel.Status != 1 {
		response.WriteJSON(w, response.ErrDefault("隧道已禁用，无法创建转发"))
		return
	}
	if roleID != 0 && tunnel.Type == 1 {
		response.WriteJSON(w, response.Err(403, "普通用户禁止在端口转发隧道上创建规则，以防滥用服务器网络"))
		return
	}
	if err := h.ensureUserTunnelForwardAllowed(userID, tunnelID, time.Now().UnixMilli()); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	name := asString(req["name"])
	remoteAddr := asString(req["remoteAddr"])
	if name == "" || remoteAddr == "" {
		response.WriteJSON(w, response.ErrDefault("转发名称和目标地址不能为空"))
		return
	}
	if roleID != 0 {
		if speedIDVal, ok := req["speedId"]; ok && speedIDVal != nil {
			response.WriteJSON(w, response.Err(-1, "普通用户无法设置限速规则"))
			return
		}
	}
	speedID := asAnyToInt64Ptr(req["speedId"])
	speedID, err = h.normalizeSpeedLimitReference(speedID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	port := asInt(req["inPort"], 0)
	if port <= 0 {
		port = h.pickTunnelPort(tunnelID)
	}
	if port <= 0 {
		port = 10000
	}
	entryNodes, _ := h.tunnelEntryNodeIDs(tunnelID)
	inIp := strings.TrimSpace(asString(req["inIp"]))
	if inIp != "" && len(entryNodes) > 1 {
		response.WriteJSON(w, response.ErrDefault("多入口隧道的转发不支持自定义监听IP"))
		return
	}
	for _, nodeID := range entryNodes {
		node, nodeErr := h.getNodeRecord(nodeID)
		if nodeErr != nil {
			continue
		}
		if err := validateRemoteNodePort(node, port); err != nil {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
		if err := validateLocalNodePort(node, port); err != nil {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
		if err := h.validateForwardPortAvailability(node, port, 0); err != nil {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
	}
	now := time.Now().UnixMilli()
	inx := h.repo.NextIndex("forward")
	userName := h.repo.GetUsernameByID(userID)
	if userName == "" {
		userName = "user"
	}
	forwardID, err := h.repo.CreateForwardTx(userID, userName, name, tunnelID, remoteAddr, defaultString(asString(req["strategy"]), "fifo"), now, inx, entryNodes, port, inIp, nullableInt(speedID))
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	createdForward, err := h.getForwardRecord(forwardID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	runtimeMeta, err := h.applyForwardRuntimeForCurrentEngineWithMetadata(r.Context(), forwardID, tunnelID, func() error {
		return h.syncForwardServices(createdForward, "UpdateService", true)
	})
	if err != nil {
		_ = h.deleteForwardByID(forwardID)
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	writeForwardMutationSuccess(w, nil, runtimeMeta)
}

func (h *Handler) forwardUpdate(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	id := asInt64(req["id"], 0)
	if id <= 0 {
		response.WriteJSON(w, response.ErrDefault("转发ID不能为空"))
		return
	}
	forward, actorUserID, actorRole, err := h.resolveForwardAccess(r, id)
	if err != nil {
		if errors.Is(err, errForwardNotFound) {
			response.WriteJSON(w, response.ErrDefault("转发不存在"))
			return
		}
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	oldPorts, err := h.listForwardPorts(id)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	tunnelID := asInt64(req["tunnelId"], forward.TunnelID)
	if tunnelID <= 0 {
		response.WriteJSON(w, response.ErrDefault("隧道ID不能为空"))
		return
	}
	if err := h.ensureTunnelPermission(actorUserID, actorRole, tunnelID); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	tunnel, err := h.getTunnelRecord(tunnelID)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("隧道不存在"))
		return
	}
	if tunnel.Status != 1 {
		response.WriteJSON(w, response.ErrDefault("隧道已禁用，无法更新转发"))
		return
	}
	if actorRole != 0 && tunnel.Type == 1 {
		response.WriteJSON(w, response.Err(403, "普通用户禁止在端口转发隧道上更新规则，以防滥用服务器网络"))
		return
	}

	name := strings.TrimSpace(asString(req["name"]))
	if name == "" {
		name = forward.Name
	}
	remoteAddr := strings.TrimSpace(asString(req["remoteAddr"]))
	if remoteAddr == "" {
		remoteAddr = forward.RemoteAddr
	}
	strategy := strings.TrimSpace(asString(req["strategy"]))
	if strategy == "" {
		strategy = forward.Strategy
	}
	rawSpeedID, hasSpeedID := req["speedId"]
	requestedSpeedID := asAnyToInt64Ptr(rawSpeedID)
	if actorRole != 0 && hasSpeedID && requestedSpeedID != nil && !sameSpeedLimitSelection(forward.SpeedID, requestedSpeedID) {
		response.WriteJSON(w, response.Err(-1, "普通用户无法修改限速规则"))
		return
	}
	speedID := requestedSpeedID
	speedID, err = h.normalizeSpeedLimitReference(speedID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	newSpeedID := forward.SpeedID
	if speedID != nil {
		newSpeedID = sql.NullInt64{Int64: *speedID, Valid: true}
	} else if _, ok := req["speedId"]; ok {
		newSpeedID = sql.NullInt64{Valid: false}
	}

	port := asInt(req["inPort"], 0)
	if port <= 0 {
		minPort := h.repo.GetMinForwardPort(id)
		if minPort.Valid {
			port = int(minPort.Int64)
		}
		if port <= 0 {
			port = h.pickTunnelPort(tunnelID)
		}
	}
	hasInIP := false
	inIp := ""
	if rawInIP, ok := req["inIp"]; ok {
		hasInIP = true
		inIp = asString(rawInIP)
	}
	fwdEntryNodes, _ := h.tunnelEntryNodeIDs(tunnelID)
	if hasInIP && strings.TrimSpace(inIp) != "" && len(fwdEntryNodes) > 1 {
		response.WriteJSON(w, response.ErrDefault("多入口隧道的转发不支持自定义监听IP"))
		return
	}
	// When switching tunnels, entry nodes / service base may change. We must clean up old
	// listeners on nodes that will be removed, otherwise the old ports keep listening.
	tunnelChanged := tunnelID != forward.TunnelID
	oldNodeIDs := forwardPortNodeIDs(oldPorts)
	newNodeIDs := uniqueInt64s(fwdEntryNodes)
	var removedNodeIDs []int64
	var keptNodeIDs []int64
	if tunnelChanged {
		removedNodeIDs = diffInt64s(oldNodeIDs, newNodeIDs)
		keptNodeIDs = diffInt64s(oldNodeIDs, removedNodeIDs)
	}
	for _, nodeID := range fwdEntryNodes {
		node, nodeErr := h.getNodeRecord(nodeID)
		if nodeErr != nil {
			continue
		}
		if err := validateRemoteNodePort(node, port); err != nil {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
		if err := validateLocalNodePort(node, port); err != nil {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
		if err := h.validateForwardPortAvailability(node, port, id); err != nil {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
	}
	now := time.Now().UnixMilli()
	if err := h.repo.UpdateForward(id, name, tunnelID, remoteAddr, strategy, now, newSpeedID); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if hasInIP {
		err = h.replaceForwardPorts(id, tunnelID, port, inIp)
	} else if tunnelID != forward.TunnelID {
		err = h.replaceForwardPorts(id, tunnelID, port, "")
	} else {
		err = h.replaceForwardPortsPreservingInIP(id, tunnelID, port, oldPorts)
	}
	if err != nil {
		h.rollbackForwardMutation(forward, oldPorts)
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	newPorts, err := h.listForwardPorts(id)
	if err != nil {
		h.rollbackForwardMutation(forward, oldPorts)
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	updatedForward, err := h.getForwardRecord(id)
	if err != nil {
		h.rollbackForwardMutation(forward, oldPorts)
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	warnings := make([]string, 0)
	if tunnelChanged && len(keptNodeIDs) > 0 {
		for _, nodeID := range keptNodeIDs {
			if delErr := h.deleteForwardServicesOnNodeBatch(forward, nodeID); delErr != nil {
				nodeLabel := fmt.Sprintf("%d", nodeID)
				if n, nErr := h.getNodeRecord(nodeID); nErr == nil && n != nil && strings.TrimSpace(n.Name) != "" {
					nodeLabel = strings.TrimSpace(n.Name)
				}
				warnings = append(warnings, fmt.Sprintf("节点 %s 清理旧转发监听失败: %v", nodeLabel, delErr))
			}
		}
		time.Sleep(tunnelServiceBindRetryDelay)
	}

	runtimeMeta, err := h.reconcileForwardRuntimeForCurrentEngineWithMetadata(r.Context(), id, tunnelID, oldPorts, newPorts, func() error {
		syncWarnings, syncErr := h.syncForwardServicesWithWarnings(updatedForward, "UpdateService", true)
		warnings = append(warnings, syncWarnings...)
		return syncErr
	})
	if err != nil {
		h.rollbackForwardMutation(forward, oldPorts)
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}

	// Best-effort cleanup for old entry nodes after a successful tunnel switch.
	// Avoid cleaning nodes that are still used by the updated forward.
	if tunnelChanged && len(removedNodeIDs) > 0 {
		for _, nodeID := range removedNodeIDs {
			if delErr := h.deleteForwardServicesOnNodeBatch(forward, nodeID); delErr != nil {
				nodeLabel := fmt.Sprintf("%d", nodeID)
				if n, nErr := h.getNodeRecord(nodeID); nErr == nil && n != nil && strings.TrimSpace(n.Name) != "" {
					nodeLabel = strings.TrimSpace(n.Name)
				}
				warnings = append(warnings, fmt.Sprintf("节点 %s 清理旧隧道残留服务失败: %v", nodeLabel, delErr))
			}
		}
	}
	writeForwardMutationSuccess(w, warnings, runtimeMeta)
}

func writeForwardMutationSuccess(w http.ResponseWriter, warnings []string, runtimeMeta *forwardRuntimeMetadata) {
	if len(warnings) == 0 && runtimeMeta == nil {
		response.WriteJSON(w, response.OKEmpty())
		return
	}
	payload := make(map[string]interface{})
	if len(warnings) > 0 {
		payload["warnings"] = warnings
	}
	if runtimeMeta != nil {
		payload["runtime"] = *runtimeMeta
	}
	response.WriteJSON(w, response.OK(payload))
}

func (h *Handler) forwardDelete(w http.ResponseWriter, r *http.Request) {
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	forward, _, _, err := h.resolveForwardAccess(r, id)
	if err != nil {
		if errors.Is(err, errForwardNotFound) {
			response.WriteJSON(w, response.ErrDefault("转发不存在"))
			return
		}
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := h.controlForwardServices(forward, "DeleteService", true); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	if err := h.deleteForwardByID(id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) forwardForceDelete(w http.ResponseWriter, r *http.Request) {
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	_, _, _, err := h.resolveForwardAccess(r, id)
	if err != nil {
		if errors.Is(err, errForwardNotFound) {
			response.WriteJSON(w, response.ErrDefault("转发不存在"))
			return
		}
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	// Force delete: remove DB record without touching node services.
	// This is used when nodes are offline or service deletion fails.
	if err := h.deleteForwardByID(id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) forwardPause(w http.ResponseWriter, r *http.Request) {
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	forward, _, _, err := h.resolveForwardAccess(r, id)
	if err != nil {
		if errors.Is(err, errForwardNotFound) {
			response.WriteJSON(w, response.ErrDefault("转发不存在"))
			return
		}
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := h.controlForwardServices(forward, "PauseService", false); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	_ = h.repo.UpdateForwardStatus(id, 0, time.Now().UnixMilli())
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) forwardResume(w http.ResponseWriter, r *http.Request) {
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	forward, _, _, err := h.resolveForwardAccess(r, id)
	if err != nil {
		if errors.Is(err, errForwardNotFound) {
			response.WriteJSON(w, response.ErrDefault("转发不存在"))
			return
		}
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	now := time.Now().UnixMilli()
	if err := h.ensureUserTunnelForwardAllowed(forward.UserID, forward.TunnelID, now); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	if err := h.controlForwardServices(forward, "ResumeService", false); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	_ = h.repo.UpdateForwardStatus(id, 1, now)
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) forwardDiagnose(w http.ResponseWriter, r *http.Request) {
	id := asInt64FromBodyKey(r, w, "forwardId")
	if id <= 0 {
		return
	}
	forward, _, _, err := h.resolveForwardAccess(r, id)
	if err != nil {
		if errors.Is(err, errForwardNotFound) {
			response.WriteJSON(w, response.ErrDefault("转发不存在"))
			return
		}
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), diagnosisRequestTimeout)
	defer cancel()
	payload, err := h.diagnoseForwardRuntime(ctx, forward)
	if err != nil {
		if strings.Contains(err.Error(), "不存在") || strings.Contains(err.Error(), "不能为空") || strings.Contains(err.Error(), "错误") {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(payload))
}

func (h *Handler) forwardUpdateOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Forwards []struct {
			ID  int64 `json:"id"`
			Inx int   `json:"inx"`
		} `json:"forwards"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	for _, f := range req.Forwards {
		h.repo.UpdateForwardOrder(f.ID, f.Inx, time.Now().UnixMilli())
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) forwardBatchDelete(w http.ResponseWriter, r *http.Request) {
	ids := idsFromBody(r, w)
	if ids == nil {
		return
	}
	actorUserID, actorRole, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	s := 0
	f := 0
	failures := make([]batchFailureDetail, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			forward, accessErr := h.ensureForwardAccessByActor(actorUserID, actorRole, id)
			if accessErr != nil {
				mu.Lock()
				f++
				failures = appendBatchFailure(failures, id, "", accessErr)
				mu.Unlock()
				return
			}
			if err := h.controlForwardServices(forward, "DeleteService", true); err != nil {
				mu.Lock()
				f++
				failures = appendBatchFailure(failures, id, forward.Name, err)
				mu.Unlock()
				return
			}
			if err := h.deleteForwardByID(id); err != nil {
				mu.Lock()
				f++
				failures = appendBatchFailure(failures, id, forward.Name, err)
				mu.Unlock()
			} else {
				mu.Lock()
				s++
				mu.Unlock()
			}
		}(id)
	}
	wg.Wait()
	response.WriteJSON(w, response.OK(batchOperationResult{SuccessCount: s, FailCount: f, Failures: failures}))
}

func (h *Handler) forwardBatchPause(w http.ResponseWriter, r *http.Request) {
	ids := idsFromBody(r, w)
	if ids == nil {
		return
	}
	actorUserID, actorRole, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	s := 0
	f := 0
	failures := make([]batchFailureDetail, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			forward, accessErr := h.ensureForwardAccessByActor(actorUserID, actorRole, id)
			if accessErr != nil {
				mu.Lock()
				f++
				failures = appendBatchFailure(failures, id, "", accessErr)
				mu.Unlock()
				return
			}
			if err := h.controlForwardServices(forward, "PauseService", false); err != nil {
				mu.Lock()
				f++
				failures = appendBatchFailure(failures, id, forward.Name, err)
				mu.Unlock()
				return
			}
			if err := h.repo.UpdateForwardStatus(id, 0, time.Now().UnixMilli()); err != nil {
				mu.Lock()
				f++
				failures = appendBatchFailure(failures, id, forward.Name, err)
				mu.Unlock()
			} else {
				mu.Lock()
				s++
				mu.Unlock()
			}
		}(id)
	}
	wg.Wait()
	response.WriteJSON(w, response.OK(batchOperationResult{SuccessCount: s, FailCount: f, Failures: failures}))
}

func (h *Handler) forwardBatchResume(w http.ResponseWriter, r *http.Request) {
	ids := idsFromBody(r, w)
	if ids == nil {
		return
	}
	actorUserID, actorRole, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	s := 0
	f := 0
	now := time.Now().UnixMilli()
	failures := make([]batchFailureDetail, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			forward, accessErr := h.ensureForwardAccessByActor(actorUserID, actorRole, id)
			if accessErr != nil {
				mu.Lock()
				f++
				failures = appendBatchFailure(failures, id, "", accessErr)
				mu.Unlock()
				return
			}
			if err := h.ensureUserTunnelForwardAllowed(forward.UserID, forward.TunnelID, now); err != nil {
				mu.Lock()
				f++
				failures = appendBatchFailure(failures, id, forward.Name, err)
				mu.Unlock()
				return
			}
			if err := h.controlForwardServices(forward, "ResumeService", false); err != nil {
				mu.Lock()
				f++
				failures = appendBatchFailure(failures, id, forward.Name, err)
				mu.Unlock()
				return
			}
			if err := h.repo.UpdateForwardStatus(id, 1, now); err != nil {
				mu.Lock()
				f++
				failures = appendBatchFailure(failures, id, forward.Name, err)
				mu.Unlock()
			} else {
				mu.Lock()
				s++
				mu.Unlock()
			}
		}(id)
	}
	wg.Wait()
	response.WriteJSON(w, response.OK(batchOperationResult{SuccessCount: s, FailCount: f, Failures: failures}))
}

func (h *Handler) forwardBatchRedeploy(w http.ResponseWriter, r *http.Request) {
	ids := idsFromBody(r, w)
	if ids == nil {
		return
	}
	actorUserID, actorRole, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	s := 0
	f := 0
	failures := make([]batchFailureDetail, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			forward, accessErr := h.ensureForwardAccessByActor(actorUserID, actorRole, id)
			if accessErr != nil {
				mu.Lock()
				f++
				failures = appendBatchFailure(failures, id, "", accessErr)
				mu.Unlock()
				return
			}
			if err := h.syncForwardServices(forward, "UpdateService", true); err != nil {
				mu.Lock()
				f++
				failures = appendBatchFailure(failures, id, forward.Name, err)
				mu.Unlock()
			} else {
				mu.Lock()
				s++
				mu.Unlock()
			}
		}(id)
	}
	wg.Wait()
	response.WriteJSON(w, response.OK(batchOperationResult{SuccessCount: s, FailCount: f, Failures: failures}))
}

func (h *Handler) forwardBatchChangeTunnel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ForwardIDs     []int64 `json:"forwardIds"`
		TargetTunnelID int64   `json:"targetTunnelId"`
	}
	if err := decodeJSON(r.Body, &req); err != nil || req.TargetTunnelID <= 0 {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	actorUserID, actorRole, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	if err := h.ensureTunnelPermission(actorUserID, actorRole, req.TargetTunnelID); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	targetTunnel, err := h.getTunnelRecord(req.TargetTunnelID)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("目标隧道不存在"))
		return
	}
	if targetTunnel.Status != 1 {
		response.WriteJSON(w, response.ErrDefault("目标隧道已禁用"))
		return
	}
	success := 0
	fail := 0
	failures := make([]batchFailureDetail, 0)
	for _, id := range req.ForwardIDs {
		if id <= 0 {
			continue
		}
		forward, accessErr := h.ensureForwardAccessByActor(actorUserID, actorRole, id)
		if accessErr != nil {
			fail++
			failures = appendBatchFailure(failures, id, "", accessErr)
			continue
		}
		if forward.TunnelID == req.TargetTunnelID {
			fail++
			failures = appendBatchFailureReason(failures, id, forward.Name, "规则已在目标隧道中")
			continue
		}
		oldPorts, listPortsErr := h.listForwardPorts(id)
		if listPortsErr != nil {
			fail++
			failures = appendBatchFailure(failures, id, forward.Name, listPortsErr)
			continue
		}
		if len(oldPorts) == 0 {
			fail++
			failures = appendBatchFailureReason(failures, id, forward.Name, "转发入口端口不存在")
			continue
		}
		oldNodeIDs := forwardPortNodeIDs(oldPorts)
		port := h.repo.GetMinForwardPort(id)
		if err := h.repo.UpdateForwardTunnel(id, req.TargetTunnelID, time.Now().UnixMilli()); err != nil {
			fail++
			failures = appendBatchFailure(failures, id, forward.Name, err)
			continue
		}
		p := 0
		if port.Valid {
			p = int(port.Int64)
		}
		if p <= 0 {
			p = h.pickTunnelPort(req.TargetTunnelID)
		}
		bctEntryNodes, _ := h.tunnelEntryNodeIDs(req.TargetTunnelID)
		newNodeIDs := uniqueInt64s(bctEntryNodes)
		removedNodeIDs := diffInt64s(oldNodeIDs, newNodeIDs)
		keptNodeIDs := diffInt64s(oldNodeIDs, removedNodeIDs)
		portRangeOk := true
		var portRangeErr error
		for _, nid := range bctEntryNodes {
			nd, ndErr := h.getNodeRecord(nid)
			if ndErr != nil {
				portRangeErr = ndErr
				portRangeOk = false
				break
			}
			if validateErr := validateRemoteNodePort(nd, p); validateErr != nil {
				portRangeOk = false
				portRangeErr = validateErr
				break
			}
			if validateErr := validateLocalNodePort(nd, p); validateErr != nil {
				portRangeOk = false
				portRangeErr = validateErr
				break
			}
			if validateErr := h.validateForwardPortAvailability(nd, p, id); validateErr != nil {
				portRangeOk = false
				portRangeErr = validateErr
				break
			}
		}
		if !portRangeOk {
			fail++
			failures = appendBatchFailure(failures, id, forward.Name, portRangeErr)
			h.rollbackForwardMutation(forward, oldPorts)
			continue
		}
		if err := h.replaceForwardPorts(id, req.TargetTunnelID, p, ""); err != nil {
			h.rollbackForwardMutation(forward, oldPorts)
			fail++
			failures = appendBatchFailure(failures, id, forward.Name, err)
			continue
		}
		newPorts, portsErr := h.listForwardPorts(id)
		if portsErr != nil {
			h.rollbackForwardMutation(forward, oldPorts)
			fail++
			failures = appendBatchFailure(failures, id, forward.Name, portsErr)
			continue
		}
		updatedForward, fetchErr := h.getForwardRecord(id)
		if fetchErr != nil {
			h.rollbackForwardMutation(forward, oldPorts)
			fail++
			failures = appendBatchFailure(failures, id, forward.Name, fetchErr)
			continue
		}
		if len(keptNodeIDs) > 0 {
			for _, nodeID := range keptNodeIDs {
				_ = h.deleteForwardServicesOnNodeBatch(forward, nodeID)
			}
			time.Sleep(tunnelServiceBindRetryDelay)
		}
		if err := h.reconcileForwardRuntimeForCurrentEngine(r.Context(), id, req.TargetTunnelID, oldPorts, newPorts, func() error {
			return h.syncForwardServices(updatedForward, "UpdateService", true)
		}); err != nil {
			h.rollbackForwardMutation(forward, oldPorts)
			fail++
			failures = appendBatchFailure(failures, id, forward.Name, err)
			continue
		}
		if len(removedNodeIDs) > 0 {
			for _, nodeID := range removedNodeIDs {
				_ = h.deleteForwardServicesOnNodeBatch(forward, nodeID)
			}
		}
		success++
	}
	response.WriteJSON(w, response.OK(batchOperationResult{SuccessCount: success, FailCount: fail, Failures: failures}))
}

func (h *Handler) speedLimitCreate(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}

	name := asString(req["name"])
	if name == "" {
		response.WriteJSON(w, response.ErrDefault("名称不能为空"))
		return
	}

	speed := asInt(req["speed"], 100)

	now := time.Now().UnixMilli()
	_, err := h.repo.CreateSpeedLimit(name, speed, now, asInt(req["status"], 1))
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) speedLimitUpdate(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}

	id := asInt64(req["id"], 0)
	if id <= 0 {
		response.WriteJSON(w, response.ErrDefault("限速规则ID不能为空"))
		return
	}

	name := asString(req["name"])
	if name == "" {
		response.WriteJSON(w, response.ErrDefault("名称不能为空"))
		return
	}

	speed := asInt(req["speed"], 100)

	if err := h.repo.UpdateSpeedLimit(id, name, speed, asInt(req["status"], 1), time.Now().UnixMilli()); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) speedLimitDelete(w http.ResponseWriter, r *http.Request) {
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}

	if err := h.repo.DeleteSpeedLimit(id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) groupTunnelCreate(w http.ResponseWriter, r *http.Request) {
	h.groupCreate(w, r, "tunnel_group")
}

func (h *Handler) groupTunnelUpdate(w http.ResponseWriter, r *http.Request) {
	h.groupUpdate(w, r, "tunnel_group")
}

func (h *Handler) groupTunnelDelete(w http.ResponseWriter, r *http.Request) {
	h.groupDelete(w, r, "tunnel_group")
}

func (h *Handler) groupUserCreate(w http.ResponseWriter, r *http.Request) {
	h.groupCreate(w, r, "user_group")
}

func (h *Handler) groupUserUpdate(w http.ResponseWriter, r *http.Request) {
	h.groupUpdate(w, r, "user_group")
}

func (h *Handler) groupUserDelete(w http.ResponseWriter, r *http.Request) {
	h.groupDelete(w, r, "user_group")
}

func (h *Handler) groupTunnelAssign(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GroupID   int64   `json:"groupId"`
		TunnelIDs []int64 `json:"tunnelIds"`
	}
	if err := decodeJSON(r.Body, &req); err != nil || req.GroupID <= 0 {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	tx := h.repo.BeginTx()
	if tx.Error != nil {
		response.WriteJSON(w, response.Err(-2, tx.Error.Error()))
		return
	}
	defer func() { tx.Rollback() }()
	if err := h.repo.ReplaceTunnelGroupMembersTx(tx, req.GroupID, req.TunnelIDs, time.Now().UnixMilli()); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := tx.Commit().Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	_ = h.syncPermissionsByTunnelGroup(req.GroupID)
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) groupUserAssign(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GroupID int64   `json:"groupId"`
		UserIDs []int64 `json:"userIds"`
	}
	if err := decodeJSON(r.Body, &req); err != nil || req.GroupID <= 0 {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	tx := h.repo.BeginTx()
	if tx.Error != nil {
		response.WriteJSON(w, response.Err(-2, tx.Error.Error()))
		return
	}
	defer func() { tx.Rollback() }()
	previousUserIDs, err := h.repo.ListUserIDsByUserGroupTx(tx, req.GroupID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := h.repo.ReplaceUserGroupMembersTx(tx, req.GroupID, req.UserIDs, time.Now().UnixMilli()); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	revokedPairs, revokeErr := h.repo.RevokeGroupGrantsForRemovedUsersTx(tx, req.GroupID, previousUserIDs, req.UserIDs)
	if revokeErr != nil {
		response.WriteJSON(w, response.Err(-2, revokeErr.Error()))
		return
	}
	if err := tx.Commit().Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	for _, pair := range revokedPairs {
		h.cleanupForwardsForUserTunnel(pair.UserID, pair.TunnelID)
	}
	_ = h.syncPermissionsByUserGroup(req.GroupID)
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) groupPermissionAssign(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserGroupID   int64 `json:"userGroupId"`
		TunnelGroupID int64 `json:"tunnelGroupId"`
	}
	if err := decodeJSON(r.Body, &req); err != nil || req.UserGroupID <= 0 || req.TunnelGroupID <= 0 {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if err := h.repo.InsertGroupPermission(req.UserGroupID, req.TunnelGroupID, time.Now().UnixMilli()); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	_ = h.applyGroupPermission(req.UserGroupID, req.TunnelGroupID)
	response.WriteJSON(w, response.OK("权限分配成功"))
}

func (h *Handler) groupPermissionRemove(w http.ResponseWriter, r *http.Request) {
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	tx := h.repo.BeginTx()
	if tx.Error != nil {
		response.WriteJSON(w, response.Err(-2, tx.Error.Error()))
		return
	}
	defer func() { tx.Rollback() }()

	ug, tg, exists, err := h.repo.GetGroupPermissionPairByIDTx(tx, id)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	if err := h.repo.DeleteGroupPermissionByIDTx(tx, id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	var revokedPairs []repo.RevokedUserTunnelPair
	if exists {
		var revokeErr error
		revokedPairs, revokeErr = h.repo.RevokeGroupPermissionPairTx(tx, ug, tg)
		if revokeErr != nil {
			response.WriteJSON(w, response.Err(-2, revokeErr.Error()))
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	for _, pair := range revokedPairs {
		h.cleanupForwardsForUserTunnel(pair.UserID, pair.TunnelID)
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) groupCreate(w http.ResponseWriter, r *http.Request, table string) {
	var req map[string]interface{}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	name := asString(req["name"])
	if name == "" {
		response.WriteJSON(w, response.ErrDefault("分组名称不能为空"))
		return
	}
	now := time.Now().UnixMilli()
	if err := h.repo.GroupCreate(table, name, asInt(req["status"], 1), now); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) groupUpdate(w http.ResponseWriter, r *http.Request, table string) {
	var req map[string]interface{}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	id := asInt64(req["id"], 0)
	if id <= 0 {
		response.WriteJSON(w, response.ErrDefault("分组ID不能为空"))
		return
	}
	if err := h.repo.GroupUpdate(table, id, asString(req["name"]), asInt(req["status"], 1), time.Now().UnixMilli()); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) groupDelete(w http.ResponseWriter, r *http.Request, table string) {
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	if err := h.repo.GroupDeleteCascade(table, id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) applyGroupPermission(userGroupID, tunnelGroupID int64) error {
	userIDs, _ := h.repo.ListUserIDsByUserGroup(userGroupID)
	tunnelIDs, _ := h.repo.ListTunnelIDsByTunnelGroup(tunnelGroupID)
	for _, uid := range userIDs {
		for _, tid := range tunnelIDs {
			utID, created, err := h.repo.EnsureUserTunnelGrant(uid, tid)
			if err != nil {
				continue
			}
			createdByGroup := 0
			if created {
				createdByGroup = 1
			}
			h.repo.InsertGroupPermissionGrant(userGroupID, tunnelGroupID, utID, createdByGroup, time.Now().UnixMilli())
		}
	}
	return nil
}

func (h *Handler) syncPermissionsByUserGroup(userGroupID int64) error {
	pairs, err := h.repo.ListGroupPermissionPairsByUserGroup(userGroupID)
	if err != nil {
		return err
	}
	for _, p := range pairs {
		_ = h.applyGroupPermission(p[0], p[1])
	}
	return nil
}

func (h *Handler) syncPermissionsByTunnelGroup(tunnelGroupID int64) error {
	pairs, err := h.repo.ListGroupPermissionPairsByTunnelGroup(tunnelGroupID)
	if err != nil {
		return err
	}
	for _, p := range pairs {
		_ = h.applyGroupPermission(p[0], p[1])
	}
	return nil
}

type tunnelRuntimeNode struct {
	NodeID    int64
	Protocol  string
	Strategy  string
	Inx       int
	ChainType int
	Port      int
	ConnectIP string
}

type tunnelCreateState struct {
	TunnelID     int64
	Type         int
	IPPreference string // "" = auto, "v4" = prefer IPv4, "v6" = prefer IPv6
	InNodes      []tunnelRuntimeNode
	ChainHops    [][]tunnelRuntimeNode
	OutNodes     []tunnelRuntimeNode
	Nodes        map[int64]*nodeRecord
	NodeIDList   []int64
}

func (h *Handler) prepareTunnelCreateState(tx *gorm.DB, req map[string]interface{}, tunnelType int, excludeTunnelID int64) (*tunnelCreateState, error) {
	state := &tunnelCreateState{
		Type:      tunnelType,
		InNodes:   make([]tunnelRuntimeNode, 0),
		ChainHops: make([][]tunnelRuntimeNode, 0),
		OutNodes:  make([]tunnelRuntimeNode, 0),
		Nodes:     make(map[int64]*nodeRecord),
	}
	nodeIDs := make([]int64, 0)

	for _, item := range asMapSlice(req["inNodeId"]) {
		nodeID := asInt64(item["nodeId"], 0)
		if nodeID <= 0 {
			continue
		}
		nodeIDs = append(nodeIDs, nodeID)
		state.InNodes = append(state.InNodes, tunnelRuntimeNode{
			NodeID:    nodeID,
			Protocol:  defaultString(asString(item["protocol"]), "tls"),
			Strategy:  defaultString(asString(item["strategy"]), "round"),
			ChainType: 1,
		})
	}
	if len(state.InNodes) == 0 {
		return nil, errors.New("入口不能为空")
	}

	if tunnelType == 2 {
		outNodesRaw := asMapSlice(req["outNodeId"])
		if len(outNodesRaw) == 0 {
			return nil, errors.New("出口不能为空")
		}

		allocated := map[int64]int{}
		for _, item := range outNodesRaw {
			nodeID := asInt64(item["nodeId"], 0)
			if nodeID <= 0 {
				continue
			}
			nodeIDs = append(nodeIDs, nodeID)
			port := asInt(item["port"], 0)
			if port <= 0 {
				isRemote, remoteErr := h.repo.IsRemoteNodeTx(tx, nodeID)
				if remoteErr != nil {
					return nil, remoteErr
				}
				if !isRemote {
					var err error
					if excludeTunnelID > 0 {
						port, err = h.repo.PickNodePortTx(tx, nodeID, allocated, excludeTunnelID)
					} else {
						port, err = h.repo.PickRandomNodePortTx(tx, nodeID, allocated, excludeTunnelID)
					}
					if err != nil {
						return nil, err
					}
				}
			}
			state.OutNodes = append(state.OutNodes, tunnelRuntimeNode{
				NodeID:    nodeID,
				Protocol:  defaultString(asString(item["protocol"]), "tls"),
				Strategy:  defaultString(asString(item["strategy"]), "round"),
				ChainType: 3,
				Port:      port,
				ConnectIP: asString(item["connectIp"]),
			})
		}
		if len(state.OutNodes) == 0 {
			return nil, errors.New("出口不能为空")
		}

		for hopIdx, hopRaw := range asAnySlice(req["chainNodes"]) {
			hop := make([]tunnelRuntimeNode, 0)
			for _, item := range asMapSlice(hopRaw) {
				nodeID := asInt64(item["nodeId"], 0)
				if nodeID <= 0 {
					continue
				}
				nodeIDs = append(nodeIDs, nodeID)
				port := asInt(item["port"], 0)
				if port <= 0 {
					isRemote, remoteErr := h.repo.IsRemoteNodeTx(tx, nodeID)
					if remoteErr != nil {
						return nil, remoteErr
					}
					if !isRemote {
						var err error
						if excludeTunnelID > 0 {
							port, err = h.repo.PickNodePortTx(tx, nodeID, allocated, excludeTunnelID)
						} else {
							port, err = h.repo.PickRandomNodePortTx(tx, nodeID, allocated, excludeTunnelID)
						}
						if err != nil {
							return nil, err
						}
					}
				}
				hop = append(hop, tunnelRuntimeNode{
					NodeID:    nodeID,
					Protocol:  defaultString(asString(item["protocol"]), "tls"),
					Strategy:  defaultString(asString(item["strategy"]), "round"),
					Inx:       hopIdx + 1,
					ChainType: 2,
					Port:      port,
					ConnectIP: asString(item["connectIp"]),
				})
			}
			if len(hop) > 0 {
				state.ChainHops = append(state.ChainHops, hop)
			}
		}
	}

	// When updating an existing tunnel (excludeTunnelID > 0), build a set of
	// node IDs that already belong to the tunnel so we can tolerate offline
	// nodes that the user is keeping or removing, while still rejecting newly
	// added offline nodes.
	existingNodeIDs := make(map[int64]struct{})
	if excludeTunnelID > 0 {
		var existIDs []int64
		if err := tx.Model(&model.ChainTunnel{}).
			Where("tunnel_id = ?", excludeTunnelID).
			Pluck("node_id", &existIDs).Error; err == nil {
			for _, eid := range existIDs {
				existingNodeIDs[eid] = struct{}{}
			}
		}
	}

	seen := make(map[int64]struct{}, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		if _, ok := seen[nodeID]; ok {
			return nil, errors.New("节点重复")
		}
		seen[nodeID] = struct{}{}
		state.NodeIDList = append(state.NodeIDList, nodeID)
		node, err := h.repo.GetNodeRecordTx(tx, nodeID)
		if err != nil {
			if strings.Contains(err.Error(), "不存在") {
				return nil, errors.New("节点不存在")
			}
			return nil, err
		}
		if node == nil {
			return nil, errors.New("节点不存在")
		}
		if node.IsRemote != 1 && node.Status != 1 {
			// For tunnel updates, allow offline nodes that already belong to the
			// tunnel (user may be removing them). Only reject genuinely new offline nodes.
			_, isExisting := existingNodeIDs[nodeID]
			if excludeTunnelID <= 0 || !isExisting {
				return nil, errors.New("部分节点不在线")
			}
		}
		state.Nodes[nodeID] = node
	}

	for _, outNode := range state.OutNodes {
		if err := validateRemoteNodePort(state.Nodes[outNode.NodeID], outNode.Port); err != nil {
			return nil, err
		}
	}
	for _, hop := range state.ChainHops {
		for _, chainNode := range hop {
			if err := validateRemoteNodePort(state.Nodes[chainNode.NodeID], chainNode.Port); err != nil {
				return nil, err
			}
		}
	}

	return state, nil
}

func buildTunnelInIP(inNodes []tunnelRuntimeNode, nodes map[int64]*nodeRecord, ipPreference string) string {
	set := make(map[string]struct{})
	ordered := make([]string, 0)
	preferV6 := strings.TrimSpace(ipPreference) == "v6"
	for _, inNode := range inNodes {
		node := nodes[inNode.NodeID]
		if node == nil {
			continue
		}
		v4 := strings.TrimSpace(node.ServerIPv4)
		v6 := strings.TrimSpace(node.ServerIPv6)
		var addrs []string
		if preferV6 {
			if v6 != "" {
				addrs = append(addrs, v6)
			}
			if v4 != "" {
				addrs = append(addrs, v4)
			}
		} else {
			if v4 != "" {
				addrs = append(addrs, v4)
			}
			if v6 != "" {
				addrs = append(addrs, v6)
			}
		}
		if len(addrs) == 0 {
			if v := strings.TrimSpace(node.ServerIP); v != "" {
				addrs = append(addrs, v)
			}
		}
		for _, a := range addrs {
			if _, ok := set[a]; !ok {
				set[a] = struct{}{}
				ordered = append(ordered, a)
			}
		}
	}
	return strings.Join(ordered, ",")
}

func validateTunnelConnectIPConstraints(req map[string]interface{}) error {
	outNodes := asMapSlice(req["outNodeId"])
	if len(outNodes) > 1 {
		for _, item := range outNodes {
			if strings.TrimSpace(asString(item["connectIp"])) != "" {
				return fmt.Errorf("多出口隧道不支持设置自定义连接IP")
			}
		}
	}

	for hopIdx, hopRaw := range asAnySlice(req["chainNodes"]) {
		hopNodes := asMapSlice(hopRaw)
		if len(hopNodes) <= 1 {
			continue
		}

		for _, item := range hopNodes {
			if strings.TrimSpace(asString(item["connectIp"])) != "" {
				return fmt.Errorf("转发链第%d跳有多个节点时不支持设置自定义连接IP", hopIdx+1)
			}
		}
	}

	return nil
}

func applyTunnelPortsToRequest(req map[string]interface{}, state *tunnelCreateState) {
	if req == nil || state == nil {
		return
	}
	outPorts := make(map[int64]int)
	for _, n := range state.OutNodes {
		outPorts[n.NodeID] = n.Port
	}
	for _, item := range asMapSlice(req["outNodeId"]) {
		nodeID := asInt64(item["nodeId"], 0)
		if port, ok := outPorts[nodeID]; ok && port > 0 {
			item["port"] = port
		}
	}

	chainPorts := make(map[int64]int)
	for _, hop := range state.ChainHops {
		for _, n := range hop {
			chainPorts[n.NodeID] = n.Port
		}
	}
	for _, hopRaw := range asAnySlice(req["chainNodes"]) {
		for _, item := range asMapSlice(hopRaw) {
			nodeID := asInt64(item["nodeId"], 0)
			if port, ok := chainPorts[nodeID]; ok && port > 0 {
				item["port"] = port
			}
		}
	}
}

type federationRuntimeReleaseRef struct {
	RemoteURL     string
	RemoteToken   string
	BindingID     string
	ReservationID string
	ResourceKey   string
}

func federationRuntimeResourceKey(tunnelID int64, nodeID int64, chainType int, hopInx int) string {
	return fmt.Sprintf("tunnel:%d:node:%d:type:%d:hop:%d", tunnelID, nodeID, chainType, hopInx)
}

func remoteShareIDFromConfig(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return 0
	}
	return asInt64(cfg["shareId"], 0)
}

func (h *Handler) federationLocalDomain() string {
	cfg, _ := h.repo.GetConfigByName("panel_domain")
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Value)
}

func (h *Handler) applyFederationRuntime(state *tunnelCreateState, localDomain string) ([]repo.FederationTunnelBinding, []federationRuntimeReleaseRef, error) {
	bindings := make([]repo.FederationTunnelBinding, 0)
	releaseRefs := make([]federationRuntimeReleaseRef, 0)
	if h == nil || state == nil {
		return bindings, releaseRefs, nil
	}
	fc := client.NewFederationClient()
	now := time.Now().UnixMilli()

	for outIdx := range state.OutNodes {
		outNode := state.OutNodes[outIdx]
		node := state.Nodes[outNode.NodeID]
		if node == nil || node.IsRemote != 1 {
			continue
		}
		remoteURL := strings.TrimSpace(node.RemoteURL)
		remoteToken := strings.TrimSpace(node.RemoteToken)
		if remoteURL == "" || remoteToken == "" {
			h.releaseFederationRuntimeRefs(releaseRefs)
			return nil, nil, fmt.Errorf("远程节点 %s 缺少共享配置", nodeDisplayName(node))
		}

		resourceKey := federationRuntimeResourceKey(state.TunnelID, outNode.NodeID, 3, 0)
		reserveReq := client.RuntimeReservePortRequest{
			ResourceKey:   resourceKey,
			Protocol:      defaultString(outNode.Protocol, "tls"),
			RequestedPort: outNode.Port,
		}
		reserveRes, err := fc.ReservePort(remoteURL, remoteToken, localDomain, reserveReq)
		if err != nil && reserveReq.RequestedPort > 0 {
			reserveReq.RequestedPort = 0
			reserveRes, err = fc.ReservePort(remoteURL, remoteToken, localDomain, reserveReq)
		}
		if err != nil {
			h.releaseFederationRuntimeRefs(releaseRefs)
			return nil, nil, fmt.Errorf("远程节点 %s 端口分配失败: %w", nodeDisplayName(node), err)
		}

		state.OutNodes[outIdx].Port = reserveRes.AllocatedPort
		outNode = state.OutNodes[outIdx]

		applyReq := client.RuntimeApplyRoleRequest{
			ReservationID: reserveRes.ReservationID,
			ResourceKey:   resourceKey,
			Role:          "exit",
			Protocol:      defaultString(outNode.Protocol, "tls"),
			Strategy:      defaultString(outNode.Strategy, "round"),
		}
		applyRes, err := fc.ApplyRole(remoteURL, remoteToken, localDomain, applyReq)
		if err != nil {
			h.releaseFederationRuntimeRefs(releaseRefs)
			return nil, nil, fmt.Errorf("远程节点 %s 运行时下发失败: %w", nodeDisplayName(node), err)
		}
		if applyRes.AllocatedPort > 0 {
			state.OutNodes[outIdx].Port = applyRes.AllocatedPort
			outNode = state.OutNodes[outIdx]
		}

		bindings = append(bindings, repo.FederationTunnelBinding{
			TunnelID:        state.TunnelID,
			NodeID:          outNode.NodeID,
			ChainType:       3,
			HopInx:          0,
			RemoteURL:       remoteURL,
			ResourceKey:     resourceKey,
			RemoteBindingID: defaultString(applyRes.BindingID, reserveRes.BindingID),
			AllocatedPort:   outNode.Port,
			Status:          1,
			CreatedTime:     now,
			UpdatedTime:     now,
		})
		releaseRefs = append(releaseRefs, federationRuntimeReleaseRef{
			RemoteURL:     remoteURL,
			RemoteToken:   remoteToken,
			BindingID:     applyRes.BindingID,
			ReservationID: reserveRes.ReservationID,
			ResourceKey:   resourceKey,
		})
	}

	for hopIdx := len(state.ChainHops) - 1; hopIdx >= 0; hopIdx-- {
		for nodeIdx := range state.ChainHops[hopIdx] {
			chainNode := state.ChainHops[hopIdx][nodeIdx]
			node := state.Nodes[chainNode.NodeID]
			if node == nil || node.IsRemote != 1 {
				continue
			}
			remoteURL := strings.TrimSpace(node.RemoteURL)
			remoteToken := strings.TrimSpace(node.RemoteToken)
			if remoteURL == "" || remoteToken == "" {
				h.releaseFederationRuntimeRefs(releaseRefs)
				return nil, nil, fmt.Errorf("远程节点 %s 缺少共享配置", nodeDisplayName(node))
			}

			resourceKey := federationRuntimeResourceKey(state.TunnelID, chainNode.NodeID, 2, hopIdx+1)
			reserveReq := client.RuntimeReservePortRequest{
				ResourceKey:   resourceKey,
				Protocol:      defaultString(chainNode.Protocol, "tls"),
				RequestedPort: chainNode.Port,
			}
			reserveRes, err := fc.ReservePort(remoteURL, remoteToken, localDomain, reserveReq)
			if err != nil && reserveReq.RequestedPort > 0 {
				reserveReq.RequestedPort = 0
				reserveRes, err = fc.ReservePort(remoteURL, remoteToken, localDomain, reserveReq)
			}
			if err != nil {
				h.releaseFederationRuntimeRefs(releaseRefs)
				return nil, nil, fmt.Errorf("远程节点 %s 端口分配失败: %w", nodeDisplayName(node), err)
			}

			state.ChainHops[hopIdx][nodeIdx].Port = reserveRes.AllocatedPort
			chainNode = state.ChainHops[hopIdx][nodeIdx]

			nextTargets := state.OutNodes
			if hopIdx+1 < len(state.ChainHops) {
				nextTargets = state.ChainHops[hopIdx+1]
			}
			applyTargets := make([]client.RuntimeTarget, 0, len(nextTargets))
			for _, target := range nextTargets {
				targetNode := state.Nodes[target.NodeID]
				if targetNode == nil {
					h.releaseFederationRuntimeRefs(releaseRefs)
					return nil, nil, errors.New("节点不存在")
				}
				host, hostErr := selectTunnelDialHost(node, targetNode, state.IPPreference, target.ConnectIP)
				if hostErr != nil {
					h.releaseFederationRuntimeRefs(releaseRefs)
					return nil, nil, hostErr
				}
				if target.Port <= 0 {
					h.releaseFederationRuntimeRefs(releaseRefs)
					return nil, nil, errors.New("节点端口不能为空")
				}
				applyTargets = append(applyTargets, client.RuntimeTarget{
					Host:     host,
					Port:     target.Port,
					Protocol: defaultString(target.Protocol, "tls"),
				})
			}

			applyReq := client.RuntimeApplyRoleRequest{
				ReservationID: reserveRes.ReservationID,
				ResourceKey:   resourceKey,
				Role:          "middle",
				Protocol:      defaultString(chainNode.Protocol, "tls"),
				Strategy:      defaultString(chainNode.Strategy, "round"),
				Targets:       applyTargets,
			}
			applyRes, err := fc.ApplyRole(remoteURL, remoteToken, localDomain, applyReq)
			if err != nil {
				h.releaseFederationRuntimeRefs(releaseRefs)
				return nil, nil, fmt.Errorf("远程节点 %s 运行时下发失败: %w", nodeDisplayName(node), err)
			}
			if applyRes.AllocatedPort > 0 {
				state.ChainHops[hopIdx][nodeIdx].Port = applyRes.AllocatedPort
				chainNode = state.ChainHops[hopIdx][nodeIdx]
			}

			bindings = append(bindings, repo.FederationTunnelBinding{
				TunnelID:        state.TunnelID,
				NodeID:          chainNode.NodeID,
				ChainType:       2,
				HopInx:          hopIdx + 1,
				RemoteURL:       remoteURL,
				ResourceKey:     resourceKey,
				RemoteBindingID: defaultString(applyRes.BindingID, reserveRes.BindingID),
				AllocatedPort:   chainNode.Port,
				Status:          1,
				CreatedTime:     now,
				UpdatedTime:     now,
			})
			releaseRefs = append(releaseRefs, federationRuntimeReleaseRef{
				RemoteURL:     remoteURL,
				RemoteToken:   remoteToken,
				BindingID:     applyRes.BindingID,
				ReservationID: reserveRes.ReservationID,
				ResourceKey:   resourceKey,
			})
		}
	}

	return bindings, releaseRefs, nil
}

func (h *Handler) releaseFederationRuntimeRefs(refs []federationRuntimeReleaseRef) {
	if h == nil || len(refs) == 0 {
		return
	}
	fc := client.NewFederationClient()
	localDomain := h.federationLocalDomain()
	for i := len(refs) - 1; i >= 0; i-- {
		ref := refs[i]
		if strings.TrimSpace(ref.RemoteURL) == "" || strings.TrimSpace(ref.RemoteToken) == "" {
			continue
		}
		req := client.RuntimeReleaseRoleRequest{
			BindingID:     ref.BindingID,
			ReservationID: ref.ReservationID,
			ResourceKey:   ref.ResourceKey,
		}
		_ = fc.ReleaseRole(ref.RemoteURL, ref.RemoteToken, localDomain, req)
	}
}

func (h *Handler) cleanupFederationRuntime(tunnelID int64) {
	if h == nil || tunnelID <= 0 {
		return
	}
	bindings, err := h.repo.ListActiveFederationTunnelBindingsByTunnel(tunnelID)
	if err != nil || len(bindings) == 0 {
		return
	}

	fc := client.NewFederationClient()
	localDomain := h.federationLocalDomain()
	for _, b := range bindings {
		node, nodeErr := h.repo.GetNodeByID(b.NodeID)
		if nodeErr != nil || node == nil {
			continue
		}
		remoteURL := strings.TrimSpace(node.RemoteURL.String)
		if remoteURL == "" {
			remoteURL = strings.TrimSpace(b.RemoteURL)
		}
		remoteToken := strings.TrimSpace(node.RemoteToken.String)
		if remoteURL == "" || remoteToken == "" {
			continue
		}
		req := client.RuntimeReleaseRoleRequest{
			BindingID:   strings.TrimSpace(b.RemoteBindingID),
			ResourceKey: strings.TrimSpace(b.ResourceKey),
		}
		_ = fc.ReleaseRole(remoteURL, remoteToken, localDomain, req)
	}
	_ = h.repo.DeleteFederationTunnelBindingsByTunnel(tunnelID)
}

func (h *Handler) applyTunnelRuntime(state *tunnelCreateState) ([]int64, []int64, error) {
	if h == nil || state == nil {
		return nil, nil, errors.New("invalid tunnel runtime state")
	}
	createdChains := make([]int64, 0)
	createdServices := make([]int64, 0)
	if state.Type != 2 {
		return createdChains, createdServices, nil
	}

	for _, inNode := range state.InNodes {
		targets := state.OutNodes
		if len(state.ChainHops) > 0 {
			targets = state.ChainHops[0]
		}
		chainData, err := buildTunnelChainConfig(state.TunnelID, inNode.NodeID, targets, state.Nodes, state.IPPreference)
		if err != nil {
			return createdChains, createdServices, err
		}
		if _, err := h.sendNodeCommand(inNode.NodeID, "AddChains", chainData, true, false); err != nil {
			if shouldDeferTunnelRuntimeApplyError(err) {
				continue
			}
			return createdChains, createdServices, fmt.Errorf("入口节点 %s 下发转发链失败: %w", nodeDisplayName(state.Nodes[inNode.NodeID]), err)
		}
		createdChains = append(createdChains, inNode.NodeID)
	}

	for i, hop := range state.ChainHops {
		nextTargets := state.OutNodes
		if i+1 < len(state.ChainHops) {
			nextTargets = state.ChainHops[i+1]
		}
		for _, chainNode := range hop {
			node := state.Nodes[chainNode.NodeID]
			if node != nil && (node.IsRemote == 1 || node.Status != 1) {
				continue
			}
			chainData, err := buildTunnelChainConfig(state.TunnelID, chainNode.NodeID, nextTargets, state.Nodes, state.IPPreference)
			if err != nil {
				return createdChains, createdServices, err
			}
			if _, err := h.sendNodeCommand(chainNode.NodeID, "AddChains", chainData, true, false); err != nil {
				if shouldDeferTunnelRuntimeApplyError(err) {
					continue
				}
				return createdChains, createdServices, fmt.Errorf("转发链节点 %s 下发转发链失败: %w", nodeDisplayName(state.Nodes[chainNode.NodeID]), err)
			}
			createdChains = append(createdChains, chainNode.NodeID)

			serviceData := buildTunnelChainServiceConfig(state.TunnelID, chainNode, state.Nodes[chainNode.NodeID], len(nextTargets))
			if err := h.addTunnelServiceOnNode(chainNode.NodeID, state.TunnelID, serviceData); err != nil {
				if shouldDeferTunnelRuntimeApplyError(err) {
					continue
				}
				return createdChains, createdServices, fmt.Errorf("转发链节点 %s 下发服务失败: %w", nodeDisplayName(state.Nodes[chainNode.NodeID]), err)
			}
			createdServices = append(createdServices, chainNode.NodeID)
		}
	}

	for _, outNode := range state.OutNodes {
		node := state.Nodes[outNode.NodeID]
		if node != nil && (node.IsRemote == 1 || node.Status != 1) {
			continue
		}
		serviceData := buildTunnelChainServiceConfig(state.TunnelID, outNode, state.Nodes[outNode.NodeID], 1)
		if err := h.addTunnelServiceOnNode(outNode.NodeID, state.TunnelID, serviceData); err != nil {
			if shouldDeferTunnelRuntimeApplyError(err) {
				continue
			}
			return createdChains, createdServices, fmt.Errorf("出口节点 %s 下发服务失败: %w", nodeDisplayName(state.Nodes[outNode.NodeID]), err)
		}
		createdServices = append(createdServices, outNode.NodeID)
	}

	return createdChains, createdServices, nil
}

func retryTunnelServiceAddWithCleanup(add func() error, cleanup func() error, wait time.Duration) error {
	if add == nil {
		return errors.New("invalid tunnel service add callback")
	}
	err := add()
	if err == nil || !isAddressAlreadyInUseError(err) {
		return err
	}
	if cleanup == nil {
		return err
	}
	if cleanupErr := cleanup(); cleanupErr != nil {
		return cleanupErr
	}
	if wait > 0 {
		time.Sleep(wait)
	}
	return add()
}

func (h *Handler) addTunnelServiceOnNode(nodeID, tunnelID int64, serviceData []map[string]interface{}) error {
	if h == nil {
		return errors.New("invalid tunnel service context")
	}
	serviceName := fmt.Sprintf("%d_tls", tunnelID)
	return retryTunnelServiceAddWithCleanup(
		func() error {
			_, err := h.sendNodeCommand(nodeID, "AddService", serviceData, true, false)
			return err
		},
		func() error {
			_, err := h.sendNodeCommand(nodeID, "DeleteService", map[string]interface{}{"services": []string{serviceName}}, false, true)
			return err
		},
		tunnelServiceBindRetryDelay,
	)
}

func (h *Handler) rollbackTunnelRuntime(chainNodeIDs, serviceNodeIDs []int64, tunnelID int64) {
	if h == nil || tunnelID <= 0 {
		return
	}
	seenServices := make(map[int64]struct{})
	serviceName := fmt.Sprintf("%d_tls", tunnelID)
	for i := len(serviceNodeIDs) - 1; i >= 0; i-- {
		nodeID := serviceNodeIDs[i]
		if _, ok := seenServices[nodeID]; ok {
			continue
		}
		seenServices[nodeID] = struct{}{}
		_, _ = h.sendNodeCommand(nodeID, "DeleteService", map[string]interface{}{"services": []string{serviceName}}, false, true)
	}

	seenChains := make(map[int64]struct{})
	chainName := fmt.Sprintf("chains_%d", tunnelID)
	for i := len(chainNodeIDs) - 1; i >= 0; i-- {
		nodeID := chainNodeIDs[i]
		if _, ok := seenChains[nodeID]; ok {
			continue
		}
		seenChains[nodeID] = struct{}{}
		_, _ = h.sendNodeCommand(nodeID, "DeleteChains", map[string]interface{}{"chain": chainName}, false, true)
	}
}

func shouldDeferTunnelRuntimeApplyError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	if strings.Contains(msg, "节点不在线") {
		return true
	}
	if strings.Contains(msg, "等待节点响应超时") || strings.Contains(msg, "timeout") || strings.Contains(msg, "超时") {
		return true
	}
	return false
}

// isNodeOfflineOrTimeoutError returns true when the error indicates a node
// is unreachable (offline or timed out), matching the same patterns used by
// shouldDeferTunnelRuntimeApplyError.
func isNodeOfflineOrTimeoutError(err error) bool {
	return shouldDeferTunnelRuntimeApplyError(err)
}

func buildTunnelChainConfig(tunnelID int64, fromNodeID int64, targets []tunnelRuntimeNode, nodes map[int64]*nodeRecord, ipPreference string) (map[string]interface{}, error) {
	fromNode := nodes[fromNodeID]
	if fromNode == nil {
		return nil, errors.New("节点不存在")
	}
	if len(targets) == 0 {
		return nil, errors.New("转发链目标不能为空")
	}

	var onlineTargets []tunnelRuntimeNode
	for _, target := range targets {
		if node := nodes[target.NodeID]; node != nil && (node.IsRemote == 1 || node.Status == 1) {
			onlineTargets = append(onlineTargets, target)
		}
	}
	if len(onlineTargets) == 0 {
		return nil, errors.New("所有目标节点均已失效")
	}

	nodeItems := make([]map[string]interface{}, 0, len(onlineTargets))
	for idx, target := range onlineTargets {
		targetNode := nodes[target.NodeID]
		if targetNode == nil {
			return nil, errors.New("节点不存在")
		}
		host, err := selectTunnelDialHost(fromNode, targetNode, ipPreference, target.ConnectIP)
		if err != nil {
			return nil, err
		}
		port := target.Port
		if port <= 0 {
			return nil, errors.New("节点端口不能为空")
		}
		protocol := defaultString(target.Protocol, "tls")
		connector := map[string]interface{}{
			"type": "relay",
		}
		if isTLSTunnelProtocol(protocol) {
			connector["metadata"] = map[string]interface{}{"nodelay": true}
		}
		nodeItems = append(nodeItems, map[string]interface{}{
			"name":      fmt.Sprintf("node_%d", idx+1),
			"addr":      processServerAddress(fmt.Sprintf("%s:%d", host, port)),
			"connector": connector,
			"dialer": map[string]interface{}{
				"type": protocol,
			},
		})
	}

	strategy := defaultString(strings.TrimSpace(onlineTargets[0].Strategy), "round")
	hop := map[string]interface{}{
		"name": fmt.Sprintf("hop_%d", tunnelID),
		"selector": map[string]interface{}{
			"strategy":    strategy,
			"maxFails":    1,
			"failTimeout": int64(600000000000),
		},
		"nodes": nodeItems,
	}
	if strings.TrimSpace(fromNode.InterfaceName) != "" {
		hop["interface"] = fromNode.InterfaceName
	}

	return map[string]interface{}{
		"name": fmt.Sprintf("chains_%d", tunnelID),
		"hops": []map[string]interface{}{hop},
	}, nil
}

func buildTunnelChainServiceConfig(tunnelID int64, chainNode tunnelRuntimeNode, node *nodeRecord, nextHopCandidateCount int) []map[string]interface{} {
	if node == nil {
		return nil
	}
	protocol := defaultString(chainNode.Protocol, "tls")
	handlerCfg := map[string]interface{}{
		"type": "relay",
	}
	if isTLSTunnelProtocol(protocol) {
		handlerCfg["metadata"] = map[string]interface{}{"nodelay": true}
	}
	if nextHopCandidateCount > 1 {
		handlerCfg["retries"] = nextHopCandidateCount - 1
	}
	service := map[string]interface{}{
		"name":    fmt.Sprintf("%d_tls", tunnelID),
		"addr":    processServerAddress(fmt.Sprintf("%s:%d", defaultString(strings.TrimSpace(chainNode.ConnectIP), node.TCPListenAddr), chainNode.Port)),
		"handler": handlerCfg,
		"listener": map[string]interface{}{
			"type": protocol,
		},
	}
	if chainNode.ChainType == 2 {
		service["handler"].(map[string]interface{})["chain"] = fmt.Sprintf("chains_%d", tunnelID)
	}
	if chainNode.ChainType == 3 && strings.TrimSpace(node.InterfaceName) != "" {
		service["metadata"] = map[string]interface{}{"interface": node.InterfaceName}
	}
	return []map[string]interface{}{service}
}

func selectTunnelDialHost(fromNode, toNode *nodeRecord, ipPreference string, connectIp string) (string, error) {
	if fromNode == nil || toNode == nil {
		return "", errors.New("节点不存在")
	}
	if strings.TrimSpace(connectIp) != "" {
		return strings.TrimSpace(connectIp), nil
	}
	fromV4 := nodeSupportsV4(fromNode)
	fromV6 := nodeSupportsV6(fromNode)
	toV4 := nodeSupportsV4(toNode)
	toV6 := nodeSupportsV6(toNode)

	switch strings.TrimSpace(ipPreference) {
	case "v6":
		if fromV6 && toV6 {
			if host := pickNodeAddressV6(toNode); host != "" {
				return host, nil
			}
		}
		if fromV4 && toV4 {
			if host := pickNodeAddressV4(toNode); host != "" {
				return host, nil
			}
		}
	case "v4":
		if fromV4 && toV4 {
			if host := pickNodeAddressV4(toNode); host != "" {
				return host, nil
			}
		}
		if fromV6 && toV6 {
			if host := pickNodeAddressV6(toNode); host != "" {
				return host, nil
			}
		}
	default:
		// 同版本优先
		if fromV4 && toV4 {
			if host := pickNodeAddressV4(toNode); host != "" {
				return host, nil
			}
		}
		if fromV6 && toV6 {
			if host := pickNodeAddressV6(toNode); host != "" {
				return host, nil
			}
		}
		// 跨版本支持：v6入v4出 / v4入v6出
		if fromV6 && toV4 {
			if host := pickNodeAddressV4(toNode); host != "" {
				return host, nil
			}
		}
		if fromV4 && toV6 {
			if host := pickNodeAddressV6(toNode); host != "" {
				return host, nil
			}
		}
	}
	return "", fmt.Errorf("节点链路不兼容：%s(v4=%t,v6=%t) -> %s(v4=%t,v6=%t)", nodeDisplayName(fromNode), fromV4, fromV6, nodeDisplayName(toNode), toV4, toV6)
}

func nodeDisplayName(node *nodeRecord) string {
	if node == nil {
		return "node"
	}
	if strings.TrimSpace(node.Name) != "" {
		return strings.TrimSpace(node.Name)
	}
	return fmt.Sprintf("node_%d", node.ID)
}

func isTLSTunnelProtocol(protocol string) bool {
	return strings.EqualFold(strings.TrimSpace(defaultString(protocol, "tls")), "tls")
}

func nodeSupportsV4(node *nodeRecord) bool {
	if node == nil {
		return false
	}
	if strings.TrimSpace(node.ServerIPv4) != "" {
		return true
	}
	if strings.TrimSpace(node.ServerIPv6) != "" {
		return false
	}
	legacy := strings.Trim(strings.TrimSpace(node.ServerIP), "[]")
	if legacy == "" {
		return false
	}
	if ip := net.ParseIP(legacy); ip != nil {
		return ip.To4() != nil
	}
	return true
}

func nodeSupportsV6(node *nodeRecord) bool {
	if node == nil {
		return false
	}
	if strings.TrimSpace(node.ServerIPv6) != "" {
		return true
	}
	if strings.TrimSpace(node.ServerIPv4) != "" {
		return false
	}
	legacy := strings.Trim(strings.TrimSpace(node.ServerIP), "[]")
	if legacy == "" {
		return false
	}
	if ip := net.ParseIP(legacy); ip != nil {
		return ip.To4() == nil
	}
	return true
}

func pickNodeAddressV4(node *nodeRecord) string {
	if node == nil {
		return ""
	}
	if v := strings.TrimSpace(node.ServerIPv4); v != "" {
		return v
	}
	return strings.TrimSpace(node.ServerIP)
}

func pickNodeAddressV6(node *nodeRecord) string {
	if node == nil {
		return ""
	}
	if v := strings.TrimSpace(node.ServerIPv6); v != "" {
		return v
	}
	return strings.TrimSpace(node.ServerIP)
}

func (h *Handler) replaceTunnelChainsTx(tx *gorm.DB, tunnelID int64, req map[string]interface{}) error {
	allocated := map[int64]int{}
	inNodes := asMapSlice(req["inNodeId"])
	for i, n := range inNodes {
		nodeID := asInt64(n["nodeId"], 0)
		if nodeID <= 0 {
			continue
		}
		if err := h.repo.CreateChainTunnelTx(
			tx,
			tunnelID,
			"1",
			nodeID,
			sql.NullInt64{},
			defaultString(asString(n["strategy"]), "round"),
			i+1,
			defaultString(asString(n["protocol"]), "tls"),
			"",
		); err != nil {
			return err
		}
	}
	for i, n := range asMapSlice(req["outNodeId"]) {
		nodeID := asInt64(n["nodeId"], 0)
		if nodeID <= 0 {
			continue
		}
		port := asInt(n["port"], 0)
		if port <= 0 {
			var pickErr error
			port, pickErr = h.repo.PickRandomNodePortTx(tx, nodeID, allocated, 0)
			if pickErr != nil {
				return pickErr
			}
		}
		connectIp := asString(n["connectIp"])
		if err := h.repo.CreateChainTunnelTx(
			tx,
			tunnelID,
			"3",
			nodeID,
			sql.NullInt64{Int64: int64(port), Valid: true},
			defaultString(asString(n["strategy"]), "round"),
			i+1,
			defaultString(asString(n["protocol"]), "tls"),
			connectIp,
		); err != nil {
			return err
		}
	}
	chainNodes := asAnySlice(req["chainNodes"])
	for i, grp := range chainNodes {
		for _, n := range asMapSlice(grp) {
			nodeID := asInt64(n["nodeId"], 0)
			if nodeID <= 0 {
				continue
			}
			port := asInt(n["port"], 0)
			if port <= 0 {
				var pickErr error
				port, pickErr = h.repo.PickRandomNodePortTx(tx, nodeID, allocated, 0)
				if pickErr != nil {
					return pickErr
				}
			}
			connectIp := asString(n["connectIp"])
			if err := h.repo.CreateChainTunnelTx(
				tx,
				tunnelID,
				"2",
				nodeID,
				sql.NullInt64{Int64: int64(port), Valid: true},
				defaultString(asString(n["strategy"]), "round"),
				i+1,
				defaultString(asString(n["protocol"]), "tls"),
				connectIp,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *Handler) deleteNodeByID(id int64) error {
	return h.repo.DeleteNodeCascade(id)
}

func (h *Handler) deleteTunnelByID(id int64) error {
	if h == nil || h.repo == nil {
		return errors.New("repository not initialized")
	}

	tunnelName, _ := h.repo.GetTunnelName(id)
	if err := h.repo.DeleteTunnelCascade(id); err != nil {
		return err
	}

	shareID, port, ok := parsePeerShareInfoFromFederationTunnelName(tunnelName)
	if !ok {
		return nil
	}

	return h.repo.MarkPeerShareRuntimeReleasedByPort(shareID, port, time.Now().UnixMilli())
}

func (h *Handler) deleteForwardByID(id int64) error {
	return h.repo.DeleteForwardCascade(id)
}

func (h *Handler) batchForwardDelete(ids []int64) (int, int) {
	s := 0
	f := 0
	for _, id := range ids {
		if err := h.deleteForwardByID(id); err != nil {
			f++
		} else {
			s++
		}
	}
	return s, f
}

func (h *Handler) batchForwardStatus(ids []int64, status int) (int, int) {
	return h.repo.BatchUpdateForwardStatus(ids, status)
}

func (h *Handler) tunnelEntryNodeIDs(tunnelID int64) ([]int64, error) {
	return h.repo.TunnelEntryNodeIDs(tunnelID)
}

func (h *Handler) pickTunnelPort(tunnelID int64) int {
	entryNodes, err := h.tunnelEntryNodeIDs(tunnelID)
	if err != nil || len(entryNodes) == 0 {
		return 10000
	}

	var commonAvailable []int
	firstNode := true

	for _, nodeID := range entryNodes {
		portRange, err := h.repo.GetNodePortRange(nodeID)
		if err != nil {
			continue
		}
		if portRange == "" {
			portRange = "1000-65535"
		}

		nodePorts, err := parsePorts(portRange)
		if err != nil {
			continue
		}

		used, err := h.getUsedPorts(nodeID)
		if err != nil {
			continue
		}

		var available []int
		for _, p := range nodePorts {
			if !used[p] {
				available = append(available, p)
			}
		}

		if firstNode {
			commonAvailable = available
			firstNode = false
		} else {
			set := make(map[int]bool)
			for _, p := range available {
				set[p] = true
			}
			var newCommon []int
			for _, p := range commonAvailable {
				if set[p] {
					newCommon = append(newCommon, p)
				}
			}
			commonAvailable = newCommon
		}

		if len(commonAvailable) == 0 {
			break
		}
	}

	if len(commonAvailable) > 0 {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(commonAvailable))))
		return commonAvailable[idx.Int64()]
	}

	return 10000
}

func (h *Handler) getUsedPorts(nodeID int64) (map[int]bool, error) {
	return h.repo.GetUsedPortsOnNodeAsMap(nodeID)
}

func parsePorts(portRange string) ([]int, error) {
	var ports []int
	parts := strings.Split(portRange, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, errors.New("invalid port range format")
			}
			start, err1 := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			end, err2 := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err1 != nil || err2 != nil || start > end {
				return nil, errors.New("invalid port range values")
			}
			for i := start; i <= end; i++ {
				ports = append(ports, i)
			}
		} else {
			p, err := strconv.Atoi(part)
			if err != nil {
				return nil, errors.New("invalid port value")
			}
			ports = append(ports, p)
		}
	}
	return ports, nil
}

type forwardPortReplaceEntry = struct {
	NodeID int64
	Port   int
	InIP   string
}

func buildForwardPortEntriesWithPreservedInIP(entryNodeIDs []int64, oldPorts []forwardPortRecord, port int) []forwardPortReplaceEntry {
	preservedByNode := make(map[int64]string)
	for _, fp := range oldPorts {
		current, exists := preservedByNode[fp.NodeID]
		if !exists {
			preservedByNode[fp.NodeID] = fp.InIP
			continue
		}
		if strings.TrimSpace(current) == "" && strings.TrimSpace(fp.InIP) != "" {
			preservedByNode[fp.NodeID] = fp.InIP
		}
	}

	entries := make([]forwardPortReplaceEntry, 0, len(entryNodeIDs))
	for _, nid := range entryNodeIDs {
		entries = append(entries, forwardPortReplaceEntry{
			NodeID: nid,
			Port:   port,
			InIP:   preservedByNode[nid],
		})
	}

	return entries
}

func (h *Handler) replaceForwardPorts(forwardID, tunnelID int64, port int, inIp string) error {
	entryNodes, err := h.tunnelEntryNodeIDs(tunnelID)
	if err != nil {
		return err
	}
	entries := make([]forwardPortReplaceEntry, len(entryNodes))
	for i, nid := range entryNodes {
		entries[i] = forwardPortReplaceEntry{NodeID: nid, Port: port, InIP: inIp}
	}
	return h.repo.ReplaceForwardPorts(forwardID, entries)
}

func (h *Handler) replaceForwardPortsPreservingInIP(forwardID, tunnelID int64, port int, oldPorts []forwardPortRecord) error {
	entryNodes, err := h.tunnelEntryNodeIDs(tunnelID)
	if err != nil {
		return err
	}
	entries := buildForwardPortEntriesWithPreservedInIP(entryNodes, oldPorts, port)
	return h.repo.ReplaceForwardPorts(forwardID, entries)
}

func (h *Handler) replaceForwardPortsWithRecords(forwardID int64, ports []forwardPortRecord) error {
	entries := make([]forwardPortReplaceEntry, len(ports))
	for i, fp := range ports {
		entries[i] = forwardPortReplaceEntry{NodeID: fp.NodeID, Port: fp.Port, InIP: fp.InIP}
	}
	return h.repo.ReplaceForwardPorts(forwardID, entries)
}

func (h *Handler) rollbackForwardMutation(oldForward *forwardRecord, oldPorts []forwardPortRecord) {
	if h == nil || oldForward == nil || h.repo == nil {
		return
	}

	h.repo.RollbackForwardFields(
		oldForward.ID, oldForward.UserID, oldForward.UserName, oldForward.Name,
		oldForward.TunnelID, oldForward.RemoteAddr, oldForward.Strategy, oldForward.Status,
		oldForward.SpeedID,
		time.Now().UnixMilli(),
	)

	if err := h.replaceForwardPortsWithRecords(oldForward.ID, oldPorts); err != nil {
		return
	}

	_ = h.syncForwardServices(oldForward, "UpdateService", true)
}

func (h *Handler) upsertUserTunnel(req map[string]interface{}) error {
	userID := asInt64(req["userId"], 0)
	tunnelID := asInt64(req["tunnelId"], 0)
	if userID <= 0 || tunnelID <= 0 {
		return fmt.Errorf("userId or tunnelId missing")
	}

	existingID, currentFlow, currentNum, currentExpTime, currentFlowReset, currentSpeedID, currentStatus, lookupErr :=
		h.repo.GetExistingUserTunnel(userID, tunnelID)

	speedID := asAnyToInt64Ptr(req["speedId"])
	var err error
	speedID, err = h.normalizeSpeedLimitReference(speedID)
	if err != nil {
		return err
	}

	reqFlow := asInt64(req["flow"], -1)
	reqNum := asInt(req["num"], -1)
	reqExpTime := asInt64(req["expTime"], -1)
	reqFlowReset := asInt64(req["flowResetTime"], -1)
	reqStatus := asInt(req["status"], -1)

	if lookupErr == sql.ErrNoRows {
		if reqFlow < 0 || reqNum < 0 || reqExpTime < 0 || reqFlowReset < 0 {
			uFlow, uNum, uExp, uReset, uErr := h.repo.GetUserDefaultsForTunnel(userID)
			if uErr == nil {
				if reqFlow < 0 {
					reqFlow = uFlow
				}
				if reqNum < 0 {
					reqNum = uNum
				}
				if reqExpTime < 0 {
					reqExpTime = uExp
				}
				if reqFlowReset < 0 {
					reqFlowReset = uReset
				}
			}
		}
		if reqFlow < 0 {
			reqFlow = 0
		}
		if reqNum < 0 {
			reqNum = 0
		}
		if reqExpTime < 0 {
			reqExpTime = time.Now().Add(365 * 24 * time.Hour).UnixMilli()
		}
		if reqFlowReset < 0 {
			reqFlowReset = 1
		}
		if reqStatus < 0 {
			reqStatus = 1
		}

		if err := h.repo.InsertUserTunnel(userID, tunnelID, nullableInt(speedID), reqNum, reqFlow, reqFlowReset, reqExpTime, reqStatus); err != nil {
			return err
		}

		if syncErr := h.syncUserTunnelForwards(userID, tunnelID); syncErr != nil {
			insertedID, _, _, _, _, _, _, lookupErr := h.repo.GetExistingUserTunnel(userID, tunnelID)
			if lookupErr != nil {
				return fmt.Errorf("下发失败且回滚失败: %v; 回滚查询错误: %w", syncErr, lookupErr)
			}

			if rollbackErr := h.repo.DeleteUserTunnel(insertedID); rollbackErr != nil {
				return fmt.Errorf("下发失败且回滚失败: %v; 回滚删除错误: %w", syncErr, rollbackErr)
			}

			return fmt.Errorf("下发失败，已回滚: %w", syncErr)
		}

		return nil
	}
	if lookupErr != nil {
		return lookupErr
	}

	newFlow := currentFlow
	if reqFlow >= 0 {
		newFlow = reqFlow
	}

	newNum := int(currentNum)
	if reqNum >= 0 {
		newNum = reqNum
	}

	newExpTime := currentExpTime
	if reqExpTime >= 0 {
		newExpTime = reqExpTime
	}

	newFlowReset := currentFlowReset
	if reqFlowReset >= 0 {
		newFlowReset = reqFlowReset
	}

	newStatus := currentStatus
	if reqStatus >= 0 {
		newStatus = reqStatus
	}

	newSpeedID := currentSpeedID
	if speedID != nil {
		newSpeedID = sql.NullInt64{Int64: *speedID, Valid: true}
	} else if _, ok := req["speedId"]; ok {
		newSpeedID = sql.NullInt64{Valid: false}
	}

	if err := h.repo.UpdateUserTunnelFields(existingID, newSpeedID, newFlow, newNum, newExpTime, newFlowReset, newStatus); err != nil {
		return err
	}

	if syncErr := h.syncUserTunnelForwards(userID, tunnelID); syncErr != nil {
		rollbackErr := h.repo.UpdateUserTunnelFields(
			existingID,
			currentSpeedID,
			currentFlow,
			int(currentNum),
			currentExpTime,
			currentFlowReset,
			currentStatus,
		)
		if rollbackErr != nil {
			return fmt.Errorf("下发失败且回滚失败: %v; 回滚错误: %w", syncErr, rollbackErr)
		}

		return fmt.Errorf("下发失败，已回滚: %w", syncErr)
	}

	return nil
}

func (h *Handler) syncUserTunnelForwards(userID, tunnelID int64) error {
	forwards, err := h.listForwardsByTunnel(tunnelID)
	if err != nil {
		return err
	}
	for i := range forwards {
		f := &forwards[i]
		if f.UserID == userID {
			if err := h.syncForwardServices(f, "UpdateService", true); err != nil {
				return err
			}
		}
	}

	return nil
}

// cleanupForwardsForUserTunnel deletes all forwarding rules belonging to a
// specific user+tunnel pair. It notifies nodes to remove the runtime services
// first, then deletes the DB records. This is best-effort: individual failures
// do not abort the overall cleanup so that remaining forwards are still cleaned.
func (h *Handler) cleanupForwardsForUserTunnel(userID, tunnelID int64) {
	if userID <= 0 || tunnelID <= 0 {
		return
	}
	forwards, err := h.repo.ListForwardsByUserAndTunnel(userID, tunnelID)
	if err != nil || len(forwards) == 0 {
		return
	}
	for i := range forwards {
		f := &forwards[i]
		if f.Status == 1 {
			_ = h.controlForwardServices(f, "DeleteService", true)
		}
		_ = h.deleteForwardByID(f.ID)
	}
}

func (h *Handler) normalizeSpeedLimitReference(speedID *int64) (*int64, error) {
	if speedID == nil {
		return nil, nil
	}

	exists, err := h.repo.SpeedLimitExists(*speedID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	return speedID, nil
}

func sameSpeedLimitSelection(current sql.NullInt64, requested *int64) bool {
	if requested == nil {
		return !current.Valid
	}

	return current.Valid && current.Int64 == *requested
}

func asAnySlice(v interface{}) []interface{} {
	if v == nil {
		return nil
	}
	if arr, ok := v.([]interface{}); ok {
		return arr
	}
	return nil
}

func asMapSlice(v interface{}) []map[string]interface{} {
	arr := asAnySlice(v)
	if arr == nil {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

func asString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int, int32, int64:
		return fmt.Sprintf("%v", t)
	default:
		b, _ := json.Marshal(t)
		return strings.Trim(string(b), "\"")
	}
}

func asInt(v interface{}, def int) int {
	s := asString(v)
	if s == "" {
		return def
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return i
}

func asInt64(v interface{}, def int64) int64 {
	s := asString(v)
	if s == "" {
		return def
	}
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return i
}

func asFloat(v interface{}, def float64) float64 {
	s := asString(v)
	if s == "" {
		return def
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return f
}

func asAnyToInt64Ptr(v interface{}) *int64 {
	s := asString(v)
	if s == "" || strings.EqualFold(s, "null") {
		return nil
	}
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return &i
}

func idFromBody(r *http.Request, w http.ResponseWriter) int64 {
	return asInt64FromBodyKey(r, w, "id")
}

func asInt64FromBodyKey(r *http.Request, w http.ResponseWriter, key string) int64 {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return 0
	}
	var req map[string]interface{}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return 0
	}
	id := asInt64(req[key], 0)
	if id <= 0 {
		response.WriteJSON(w, response.ErrDefault("参数错误"))
		return 0
	}
	return id
}

func idsFromBody(r *http.Request, w http.ResponseWriter) []int64 {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return nil
	}
	var req map[string]interface{}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return nil
	}
	arr := asAnySlice(req["ids"])
	if len(arr) == 0 {
		response.WriteJSON(w, response.ErrDefault("ids不能为空"))
		return nil
	}
	ids := make([]int64, 0, len(arr))
	for _, x := range arr {
		id := asInt64(x, 0)
		if id > 0 {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func nullableText(s string) interface{} {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func nullableUnixMilli(v int64) interface{} {
	if v <= 0 {
		return nil
	}
	return v
}

func normalizeNodeRenewalCycle(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "month", "quarter", "year":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return ""
	}
}

func nullableInt(v *int64) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

func defaultString(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func randomToken(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}

func asInt64Slice(v interface{}) []int64 {
	arr := asAnySlice(v)
	if len(arr) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(arr))
	for _, x := range arr {
		if id := asInt64(x, 0); id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func validateLocalNodePort(node *nodeRecord, port int) error {
	if node == nil || node.IsRemote == 1 || port <= 0 {
		return nil
	}
	portRange := strings.TrimSpace(node.PortRange)
	if portRange == "" {
		return nil
	}
	minPort, maxPort := parsePortRangeMinMax(portRange)
	if minPort <= 0 || maxPort <= 0 {
		return nil
	}
	if port < minPort || port > maxPort {
		return fmt.Errorf("端口 %d 超出节点 %s 允许范围 %d-%d", port, node.Name, minPort, maxPort)
	}
	return nil
}

func (h *Handler) validateForwardPortAvailability(node *nodeRecord, port int, currentForwardID int64) error {
	if h == nil || h.repo == nil || node == nil || port <= 0 {
		return nil
	}
	occupied, err := h.repo.HasOtherForwardOnNodePort(node.ID, port, currentForwardID)
	if err != nil {
		return err
	}
	if occupied {
		return fmt.Errorf("节点 %s 端口 %d 已被其他转发占用", node.Name, port)
	}
	return nil
}

func parsePortRangeMinMax(input string) (int, int) {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0, 0
	}
	minPort, maxPort := 0, 0
	parts := strings.Split(input, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			r := strings.SplitN(part, "-", 2)
			if len(r) != 2 {
				continue
			}
			start, err1 := strconv.Atoi(strings.TrimSpace(r[0]))
			end, err2 := strconv.Atoi(strings.TrimSpace(r[1]))
			if err1 != nil || err2 != nil || start <= 0 || end <= 0 {
				continue
			}
			if end < start {
				start, end = end, start
			}
			if minPort == 0 || start < minPort {
				minPort = start
			}
			if maxPort == 0 || end > maxPort {
				maxPort = end
			}
			continue
		}
		p, err := strconv.Atoi(part)
		if err != nil || p <= 0 {
			continue
		}
		if minPort == 0 || p < minPort {
			minPort = p
		}
		if maxPort == 0 || p > maxPort {
			maxPort = p
		}
	}
	return minPort, maxPort
}
