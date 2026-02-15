package handler

import (
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
	"time"

	"go-backend/internal/http/client"
	"go-backend/internal/http/response"
	"go-backend/internal/security"
	"go-backend/internal/store"
	"go-backend/internal/store/sqlite"
)

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

	db := h.repo.DB()
	if db == nil {
		response.WriteJSON(w, response.Err(-2, "database unavailable"))
		return
	}

	var cnt int
	if err := db.QueryRow(`SELECT COUNT(1) FROM user WHERE user = ?`, username).Scan(&cnt); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if cnt > 0 {
		response.WriteJSON(w, response.ErrDefault("用户名已存在"))
		return
	}

	status := asInt(req["status"], 1)
	flow := asInt64(req["flow"], 100)
	num := asInt(req["num"], 10)
	expTime := asInt64(req["expTime"], time.Now().Add(365*24*time.Hour).UnixMilli())
	flowResetTime := asInt64(req["flowResetTime"], 1)
	roleID := 1
	now := time.Now().UnixMilli()

	_, err := db.Exec(`
		INSERT INTO user(user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(?, ?, ?, ?, ?, 0, 0, ?, ?, ?, ?, ?)
	`, username, security.MD5(pwd), roleID, expTime, flow, flowResetTime, num, now, now, status)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
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

	db := h.repo.DB()
	if db == nil {
		response.WriteJSON(w, response.Err(-2, "database unavailable"))
		return
	}

	var roleID int
	if err := db.QueryRow(`SELECT role_id FROM user WHERE id = ?`, id).Scan(&roleID); err != nil {
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

	var cnt int
	if err := db.QueryRow(`SELECT COUNT(1) FROM user WHERE user = ? AND id != ?`, username, id).Scan(&cnt); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if cnt > 0 {
		response.WriteJSON(w, response.ErrDefault("用户名已存在"))
		return
	}

	flow := asInt64(req["flow"], 100)
	num := asInt(req["num"], 10)
	expTime := asInt64(req["expTime"], time.Now().Add(365*24*time.Hour).UnixMilli())
	flowResetTime := asInt64(req["flowResetTime"], 1)
	status := asInt(req["status"], 1)
	now := time.Now().UnixMilli()

	pwd := asString(req["pwd"])
	if strings.TrimSpace(pwd) == "" {
		_, err := db.Exec(`
			UPDATE user
			SET user = ?, flow = ?, num = ?, exp_time = ?, flow_reset_time = ?, status = ?, updated_time = ?
			WHERE id = ?
		`, username, flow, num, expTime, flowResetTime, status, now, id)
		if err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
	} else {
		_, err := db.Exec(`
			UPDATE user
			SET user = ?, pwd = ?, flow = ?, num = ?, exp_time = ?, flow_reset_time = ?, status = ?, updated_time = ?
			WHERE id = ?
		`, username, security.MD5(pwd), flow, num, expTime, flowResetTime, status, now, id)
		if err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
	}

	_, _ = db.Exec(`UPDATE user_tunnel SET flow = ?, num = ?, exp_time = ?, flow_reset_time = ? WHERE user_id = ?`, flow, num, expTime, flowResetTime, id)
	response.WriteJSON(w, response.OKEmpty())
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

	var roleID int
	if err := h.repo.DB().QueryRow(`SELECT role_id FROM user WHERE id = ?`, id).Scan(&roleID); err != nil {
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

	db := h.repo.DB()
	tx, err := db.Begin()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	defer func() { _ = tx.Rollback() }()

	if _, err = tx.Exec(`DELETE FROM forward_port WHERE forward_id IN (SELECT id FROM forward WHERE user_id = ?)`, id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if _, err = tx.Exec(`DELETE FROM forward WHERE user_id = ?`, id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if _, err = tx.Exec(`DELETE FROM group_permission_grant WHERE user_tunnel_id IN (SELECT id FROM user_tunnel WHERE user_id = ?)`, id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if _, err = tx.Exec(`DELETE FROM user_tunnel WHERE user_id = ?`, id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if _, err = tx.Exec(`DELETE FROM user_group_user WHERE user_id = ?`, id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if _, err = tx.Exec(`DELETE FROM statistics_flow WHERE user_id = ?`, id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if _, err = tx.Exec(`DELETE FROM user WHERE id = ?`, id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	if err = tx.Commit(); err != nil {
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

	db := h.repo.DB()
	if typeVal == 1 {
		_, _ = db.Exec(`UPDATE user SET in_flow = 0, out_flow = 0, updated_time = ? WHERE id = ?`, time.Now().UnixMilli(), id)
		_, _ = db.Exec(`UPDATE user_tunnel SET in_flow = 0, out_flow = 0 WHERE user_id = ?`, id)
	} else {
		_, _ = db.Exec(`UPDATE user_tunnel SET in_flow = 0, out_flow = 0 WHERE id = ?`, id)
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

	db := h.repo.DB()
	now := time.Now().UnixMilli()
	inx := nextIndex(db, "node")
	_, err := db.Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx, is_remote, remote_url, remote_token, remote_config)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		name,
		randomToken(16),
		serverIP,
		nullableText(asString(req["serverIpV4"])),
		nullableText(asString(req["serverIpV6"])),
		defaultString(asString(req["port"]), "1000-65535"),
		nullableText(asString(req["interfaceName"])),
		nullableText(""),
		asInt(req["http"], 0),
		asInt(req["tls"], 0),
		asInt(req["socks"], 0),
		now,
		now,
		0,
		defaultString(asString(req["tcpListenAddr"]), "[::]"),
		defaultString(asString(req["udpListenAddr"]), "[::]"),
		inx,
		asInt(req["isRemote"], 0),
		nullableText(asString(req["remoteUrl"])),
		nullableText(asString(req["remoteToken"])),
		nullableText(asString(req["remoteConfig"])),
	)
	if err != nil {
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

	var currentStatus int
	var currentHTTP int
	var currentTLS int
	var currentSocks int
	if err := h.repo.DB().QueryRow(`SELECT status, http, tls, socks FROM node WHERE id = ?`, id).Scan(&currentStatus, &currentHTTP, &currentTLS, &currentSocks); err != nil {
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
	if currentStatus == 1 && (newHTTP != currentHTTP || newTLS != currentTLS || newSocks != currentSocks) {
		if err := h.applyNodeProtocolChange(id, newHTTP, newTLS, newSocks); err != nil {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
	}

	now := time.Now().UnixMilli()
	_, err := h.repo.DB().Exec(`
		UPDATE node
		SET name = ?, server_ip = ?, server_ip_v4 = ?, server_ip_v6 = ?, port = ?, interface_name = ?, http = ?, tls = ?, socks = ?, tcp_listen_addr = ?, udp_listen_addr = ?, updated_time = ?
		WHERE id = ?
	`,
		asString(req["name"]),
		asString(req["serverIp"]),
		nullableText(asString(req["serverIpV4"])),
		nullableText(asString(req["serverIpV6"])),
		defaultString(asString(req["port"]), "1000-65535"),
		nullableText(asString(req["interfaceName"])),
		newHTTP,
		newTLS,
		newSocks,
		defaultString(asString(req["tcpListenAddr"]), "[::]"),
		defaultString(asString(req["udpListenAddr"]), "[::]"),
		now,
		id,
	)
	if err != nil {
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
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	db := h.repo.DB()
	var secret string
	if err := db.QueryRow(`SELECT secret FROM node WHERE id = ?`, id).Scan(&secret); err != nil {
		response.WriteJSON(w, response.ErrDefault("节点不存在"))
		return
	}
	var panelAddr string
	if err := db.QueryRow(`SELECT value FROM vite_config WHERE name = 'ip' LIMIT 1`).Scan(&panelAddr); err != nil {
		if err == sql.ErrNoRows {
			response.WriteJSON(w, response.ErrDefault("请先前往网站配置中设置ip"))
			return
		}
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	cmd := fmt.Sprintf("curl -L https://gcode.hostcentral.cc/https://github.com/Sagit-chu/flvx/releases/latest/download/install.sh -o ./install.sh && chmod +x ./install.sh && ./install.sh -a %s -s %s", processServerAddress(panelAddr), secret)
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
		_, _ = h.repo.DB().Exec(`UPDATE node SET inx = ?, updated_time = ? WHERE id = ?`, n.Inx, time.Now().UnixMilli(), n.ID)
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
	name := asString(req["name"])
	if name == "" {
		response.WriteJSON(w, response.ErrDefault("隧道名称不能为空"))
		return
	}
	var tunnelNameDup int
	if err := h.repo.DB().QueryRow(`SELECT COUNT(1) FROM tunnel WHERE name = ?`, name).Scan(&tunnelNameDup); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if tunnelNameDup > 0 {
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
	inx := nextIndex(h.repo.DB(), "tunnel")

	tx, err := h.repo.DB().Begin()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	defer func() { _ = tx.Rollback() }()

	runtimeState, err := h.prepareTunnelCreateState(tx, req, typeVal, 0)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	runtimeState.IPPreference = ipPreference
	if strings.TrimSpace(inIP) == "" {
		inIP = buildTunnelInIP(runtimeState.InNodes, runtimeState.Nodes)
	}

	if len(runtimeState.InNodes) > 0 {
		firstNodeID := runtimeState.InNodes[0].NodeID
		var isRemote int
		var rUrl, rToken sql.NullString
		if err := h.repo.DB().QueryRow("SELECT is_remote, remote_url, remote_token FROM node WHERE id = ?", firstNodeID).Scan(&isRemote, &rUrl, &rToken); err == nil && isRemote == 1 {
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

	tunnelID, err := tx.ExecReturningID(`INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx, ip_preference) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		name, trafficRatio, typeVal, "tls", flow, now, now, status, nullableText(inIP), inx, ipPreference)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	runtimeState.TunnelID = tunnelID
	var federationBindings []sqlite.FederationTunnelBinding
	var federationReleaseRefs []federationRuntimeReleaseRef
	federationBindings, federationReleaseRefs, err = h.applyFederationRuntime(runtimeState)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	applyTunnelPortsToRequest(req, runtimeState)
	if err := replaceTunnelChainsTx(tx, tunnelID, req); err != nil {
		h.releaseFederationRuntimeRefs(federationReleaseRefs)
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := replaceFederationTunnelBindingsTx(tx, tunnelID, federationBindings); err != nil {
		h.releaseFederationRuntimeRefs(federationReleaseRefs)
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := tx.Commit(); err != nil {
		h.releaseFederationRuntimeRefs(federationReleaseRefs)
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if typeVal == 2 {
		createdChains, createdServices, applyErr := h.applyTunnelRuntime(runtimeState)
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
	id := asInt64(req["id"], 0)
	if id <= 0 {
		response.WriteJSON(w, response.ErrDefault("隧道ID不能为空"))
		return
	}

	h.cleanupTunnelRuntime(id)
	h.cleanupFederationRuntime(id)

	now := time.Now().UnixMilli()
	typeVal := asInt(req["type"], 1)
	ipPreference := asString(req["ipPreference"])

	tx, err := h.repo.DB().Begin()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	defer func() { _ = tx.Rollback() }()

	runtimeState, err := h.prepareTunnelCreateState(tx, req, typeVal, id)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	runtimeState.TunnelID = id
	runtimeState.IPPreference = ipPreference

	inIp := buildTunnelInIP(runtimeState.InNodes, runtimeState.Nodes)

	var federationBindings []sqlite.FederationTunnelBinding
	var federationReleaseRefs []federationRuntimeReleaseRef
	federationBindings, federationReleaseRefs, err = h.applyFederationRuntime(runtimeState)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	applyTunnelPortsToRequest(req, runtimeState)

	_, err = tx.Exec(`UPDATE tunnel SET name=?, type=?, flow=?, traffic_ratio=?, status=?, in_ip=?, ip_preference=?, updated_time=? WHERE id=?`,
		asString(req["name"]), typeVal, asInt64(req["flow"], 1), asFloat(req["trafficRatio"], 1.0), asInt(req["status"], 1), nullableText(inIp), ipPreference, now, id)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	if _, err := tx.Exec(`DELETE FROM chain_tunnel WHERE tunnel_id = ?`, id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := replaceTunnelChainsTx(tx, id, req); err != nil {
		h.releaseFederationRuntimeRefs(federationReleaseRefs)
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := replaceFederationTunnelBindingsTx(tx, id, federationBindings); err != nil {
		h.releaseFederationRuntimeRefs(federationReleaseRefs)
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := tx.Commit(); err != nil {
		h.releaseFederationRuntimeRefs(federationReleaseRefs)
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	if typeVal == 2 {
		createdChains, createdServices, applyErr := h.applyTunnelRuntime(runtimeState)
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

	response.WriteJSON(w, response.OKEmpty())
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
	result, err := h.diagnoseTunnelRuntime(id)
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
		_, _ = h.repo.DB().Exec(`UPDATE tunnel SET inx = ?, updated_time = ? WHERE id = ?`, t.Inx, time.Now().UnixMilli(), t.ID)
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
	for _, id := range ids {
		h.cleanupTunnelRuntime(id)
		h.cleanupFederationRuntime(id)
		if err := h.deleteTunnelByID(id); err != nil {
			fail++
		} else {
			success++
		}
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{"successCount": success, "failCount": fail}))
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

	var ipPreference string
	_ = h.repo.DB().QueryRow(`SELECT COALESCE(ip_preference, '') FROM tunnel WHERE id = ?`, tunnelID).Scan(&ipPreference)

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

func (h *Handler) tunnelBatchRedeploy(w http.ResponseWriter, r *http.Request) {
	ids := idsFromBody(r, w)
	if ids == nil {
		return
	}
	success := 0
	fail := 0
	for _, tunnelID := range ids {
		tunnel, err := h.getTunnelRecord(tunnelID)
		if err != nil {
			fail++
			continue
		}

		if tunnel.Type == 2 {
			h.cleanupTunnelRuntime(tunnelID)
			h.cleanupFederationRuntime(tunnelID)
			state, err := h.reconstructTunnelState(tunnelID)
			if err != nil {
				fail++
				continue
			}
			federationBindings, federationReleaseRefs, fedErr := h.applyFederationRuntime(state)
			if fedErr != nil {
				fail++
				continue
			}
			tx, txErr := h.repo.DB().Begin()
			if txErr != nil {
				h.releaseFederationRuntimeRefs(federationReleaseRefs)
				fail++
				continue
			}
			if replaceErr := replaceFederationTunnelBindingsTx(tx, tunnelID, federationBindings); replaceErr != nil {
				_ = tx.Rollback()
				h.releaseFederationRuntimeRefs(federationReleaseRefs)
				fail++
				continue
			}
			if commitErr := tx.Commit(); commitErr != nil {
				h.releaseFederationRuntimeRefs(federationReleaseRefs)
				fail++
				continue
			}
			_, _, applyErr := h.applyTunnelRuntime(state)
			if applyErr != nil {
				h.releaseFederationRuntimeRefs(federationReleaseRefs)
				_ = h.repo.DeleteFederationTunnelBindingsByTunnel(tunnelID)
				fail++
				continue
			}
		}

		forwards, err := h.listForwardsByTunnel(tunnelID)
		if err != nil {
			fail++
			continue
		}
		if len(forwards) == 0 {
			success++
			continue
		}
		ok := true
		for i := range forwards {
			if err := h.syncForwardServices(&forwards[i], "UpdateService", true); err != nil {
				ok = false
				break
			}
		}
		if ok {
			success++
		} else {
			fail++
		}
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{"successCount": success, "failCount": fail}))
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
	_, err := h.repo.DB().Exec(`DELETE FROM user_tunnel WHERE id = ?`, id)
	if err != nil {
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
	_, err := h.repo.DB().Exec(`
		UPDATE user_tunnel SET flow = ?, num = ?, exp_time = ?, flow_reset_time = ?, speed_id = ?, status = ? WHERE id = ?
	`,
		asInt64(req["flow"], 0),
		asInt(req["num"], 0),
		asInt64(req["expTime"], time.Now().Add(365*24*time.Hour).UnixMilli()),
		asInt64(req["flowResetTime"], 1),
		nullableInt(asAnyToInt64Ptr(req["speedId"])),
		asInt(req["status"], 1),
		id,
	)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	// Fetch details to sync forwards
	var userID, tunnelID int64
	if err := h.repo.DB().QueryRow("SELECT user_id, tunnel_id FROM user_tunnel WHERE id = ?", id).Scan(&userID, &tunnelID); err == nil {
		h.syncUserTunnelForwards(userID, tunnelID)
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
	name := asString(req["name"])
	remoteAddr := asString(req["remoteAddr"])
	if name == "" || remoteAddr == "" {
		response.WriteJSON(w, response.ErrDefault("转发名称和目标地址不能为空"))
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
	for _, nodeID := range entryNodes {
		node, nodeErr := h.getNodeRecord(nodeID)
		if nodeErr != nil {
			continue
		}
		if err := validateRemoteNodePort(node, port); err != nil {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
	}
	now := time.Now().UnixMilli()
	inx := nextIndex(h.repo.DB(), "forward")
	var userName string
	_ = h.repo.DB().QueryRow(`SELECT user FROM user WHERE id = ?`, userID).Scan(&userName)
	if userName == "" {
		userName = "user"
	}
	tx, err := h.repo.DB().Begin()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	defer func() { _ = tx.Rollback() }()
	forwardID, err := tx.ExecReturningID(`
		INSERT INTO forward(user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
		VALUES(?, ?, ?, ?, ?, ?, 0, 0, ?, ?, 1, ?)
	`, userID, userName, name, tunnelID, remoteAddr, defaultString(asString(req["strategy"]), "fifo"), now, now, inx)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	for _, nodeID := range entryNodes {
		_, _ = tx.Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(?, ?, ?)`, forwardID, nodeID, port)
	}
	if err := tx.Commit(); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	createdForward, err := h.getForwardRecord(forwardID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := h.syncForwardServices(createdForward, "AddService", false); err != nil {
		_ = h.deleteForwardByID(forwardID)
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
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

	port := asInt(req["inPort"], 0)
	if port <= 0 {
		var minPort sql.NullInt64
		_ = h.repo.DB().QueryRow(`SELECT MIN(port) FROM forward_port WHERE forward_id = ?`, id).Scan(&minPort)
		if minPort.Valid {
			port = int(minPort.Int64)
		}
		if port <= 0 {
			port = h.pickTunnelPort(tunnelID)
		}
	}
	fwdEntryNodes, _ := h.tunnelEntryNodeIDs(tunnelID)
	for _, nodeID := range fwdEntryNodes {
		node, nodeErr := h.getNodeRecord(nodeID)
		if nodeErr != nil {
			continue
		}
		if err := validateRemoteNodePort(node, port); err != nil {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
	}
	now := time.Now().UnixMilli()
	_, err = h.repo.DB().Exec(`
		UPDATE forward SET name = ?, tunnel_id = ?, remote_addr = ?, strategy = ?, updated_time = ? WHERE id = ?
	`, name, tunnelID, remoteAddr, strategy, now, id)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := h.replaceForwardPorts(id, tunnelID, port); err != nil {
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
	if err := h.syncForwardServices(updatedForward, "UpdateService", true); err != nil {
		h.rollbackForwardMutation(forward, oldPorts)
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
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
	h.forwardDelete(w, r)
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
	_, _ = h.repo.DB().Exec(`UPDATE forward SET status = 0, updated_time = ? WHERE id = ?`, time.Now().UnixMilli(), id)
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
	if err := h.controlForwardServices(forward, "ResumeService", false); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	_, _ = h.repo.DB().Exec(`UPDATE forward SET status = 1, updated_time = ? WHERE id = ?`, time.Now().UnixMilli(), id)
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
	payload, err := h.diagnoseForwardRuntime(forward)
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
		_, _ = h.repo.DB().Exec(`UPDATE forward SET inx = ?, updated_time = ? WHERE id = ?`, f.Inx, time.Now().UnixMilli(), f.ID)
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
	for _, id := range ids {
		forward, accessErr := h.ensureForwardAccessByActor(actorUserID, actorRole, id)
		if accessErr != nil {
			f++
			continue
		}
		if err := h.controlForwardServices(forward, "DeleteService", true); err != nil {
			f++
			continue
		}
		if err := h.deleteForwardByID(id); err != nil {
			f++
		} else {
			s++
		}
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{"successCount": s, "failCount": f}))
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
	for _, id := range ids {
		forward, accessErr := h.ensureForwardAccessByActor(actorUserID, actorRole, id)
		if accessErr != nil {
			f++
			continue
		}
		if err := h.controlForwardServices(forward, "PauseService", false); err != nil {
			f++
			continue
		}
		if _, err := h.repo.DB().Exec(`UPDATE forward SET status = 0, updated_time = ? WHERE id = ?`, time.Now().UnixMilli(), id); err != nil {
			f++
		} else {
			s++
		}
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{"successCount": s, "failCount": f}))
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
	for _, id := range ids {
		forward, accessErr := h.ensureForwardAccessByActor(actorUserID, actorRole, id)
		if accessErr != nil {
			f++
			continue
		}
		if err := h.controlForwardServices(forward, "ResumeService", false); err != nil {
			f++
			continue
		}
		if _, err := h.repo.DB().Exec(`UPDATE forward SET status = 1, updated_time = ? WHERE id = ?`, time.Now().UnixMilli(), id); err != nil {
			f++
		} else {
			s++
		}
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{"successCount": s, "failCount": f}))
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
	for _, id := range ids {
		forward, accessErr := h.ensureForwardAccessByActor(actorUserID, actorRole, id)
		if accessErr != nil {
			f++
			continue
		}
		if err := h.syncForwardServices(forward, "UpdateService", true); err != nil {
			f++
		} else {
			s++
		}
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{"successCount": s, "failCount": f}))
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
	for _, id := range req.ForwardIDs {
		if id <= 0 {
			continue
		}
		forward, accessErr := h.ensureForwardAccessByActor(actorUserID, actorRole, id)
		if accessErr != nil {
			fail++
			continue
		}
		if forward.TunnelID == req.TargetTunnelID {
			fail++
			continue
		}
		oldPorts, listPortsErr := h.listForwardPorts(id)
		if listPortsErr != nil {
			fail++
			continue
		}
		var port sql.NullInt64
		_ = h.repo.DB().QueryRow(`SELECT MIN(port) FROM forward_port WHERE forward_id = ?`, id).Scan(&port)
		_, err := h.repo.DB().Exec(`UPDATE forward SET tunnel_id = ?, updated_time = ? WHERE id = ?`, req.TargetTunnelID, time.Now().UnixMilli(), id)
		if err != nil {
			fail++
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
		portRangeOk := true
		for _, nid := range bctEntryNodes {
			nd, ndErr := h.getNodeRecord(nid)
			if ndErr != nil {
				continue
			}
			if validateRemoteNodePort(nd, p) != nil {
				portRangeOk = false
				break
			}
		}
		if !portRangeOk {
			fail++
			continue
		}
		if err := h.replaceForwardPorts(id, req.TargetTunnelID, p); err != nil {
			h.rollbackForwardMutation(forward, oldPorts)
			fail++
			continue
		}
		updatedForward, fetchErr := h.getForwardRecord(id)
		if fetchErr != nil {
			h.rollbackForwardMutation(forward, oldPorts)
			fail++
			continue
		}
		if err := h.syncForwardServices(updatedForward, "UpdateService", true); err != nil {
			h.rollbackForwardMutation(forward, oldPorts)
			fail++
			continue
		}
		success++
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{"successCount": success, "failCount": fail}))
}

func (h *Handler) speedLimitCreate(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	tunnelID := asInt64(req["tunnelId"], 0)
	if tunnelID <= 0 {
		response.WriteJSON(w, response.ErrDefault("隧道ID不能为空"))
		return
	}
	name := asString(req["name"])
	if name == "" {
		response.WriteJSON(w, response.ErrDefault("名称不能为空"))
		return
	}
	var tunnelName string
	_ = h.repo.DB().QueryRow(`SELECT name FROM tunnel WHERE id = ?`, tunnelID).Scan(&tunnelName)
	if tunnelName == "" {
		response.WriteJSON(w, response.ErrDefault("隧道不存在"))
		return
	}
	now := time.Now().UnixMilli()
	speed := asInt(req["speed"], 100)
	id, err := h.repo.DB().ExecReturningID(`INSERT INTO speed_limit(name, speed, tunnel_id, tunnel_name, created_time, updated_time, status) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		name, speed, tunnelID, tunnelName, now, now, asInt(req["status"], 1))
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	_ = h.sendLimiterConfig(id, speed, tunnelID)
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) speedLimitUpdate(w http.ResponseWriter, r *http.Request) {
	var req map[string]interface{}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	id := asInt64(req["id"], 0)
	tunnelID := asInt64(req["tunnelId"], 0)
	if id <= 0 || tunnelID <= 0 {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	var tunnelName string
	_ = h.repo.DB().QueryRow(`SELECT name FROM tunnel WHERE id = ?`, tunnelID).Scan(&tunnelName)
	if tunnelName == "" {
		response.WriteJSON(w, response.ErrDefault("隧道不存在"))
		return
	}
	speed := asInt(req["speed"], 100)
	_, err := h.repo.DB().Exec(`UPDATE speed_limit SET name=?, speed=?, tunnel_id=?, tunnel_name=?, status=?, updated_time=? WHERE id=?`,
		asString(req["name"]), speed, tunnelID, tunnelName, asInt(req["status"], 1), time.Now().UnixMilli(), id)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	_ = h.sendLimiterConfig(id, speed, tunnelID)
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) speedLimitDelete(w http.ResponseWriter, r *http.Request) {
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	var tunnelID int64
	_ = h.repo.DB().QueryRow(`SELECT tunnel_id FROM speed_limit WHERE id = ?`, id).Scan(&tunnelID)

	_, err := h.repo.DB().Exec(`DELETE FROM speed_limit WHERE id = ?`, id)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if tunnelID > 0 {
		_ = h.sendDeleteLimiterConfig(id, tunnelID)
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
	tx, err := h.repo.DB().Begin()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	defer func() { _ = tx.Rollback() }()
	_, _ = tx.Exec(`DELETE FROM tunnel_group_tunnel WHERE tunnel_group_id = ?`, req.GroupID)
	for _, tid := range req.TunnelIDs {
		_, _ = tx.Exec(`INSERT INTO tunnel_group_tunnel(tunnel_group_id, tunnel_id, created_time) VALUES(?, ?, ?) ON CONFLICT DO NOTHING`, req.GroupID, tid, time.Now().UnixMilli())
	}
	if err := tx.Commit(); err != nil {
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
	tx, err := h.repo.DB().Begin()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	defer func() { _ = tx.Rollback() }()
	previousUserIDs, err := queryInt64ListTx(tx, `SELECT user_id FROM user_group_user WHERE user_group_id = ?`, req.GroupID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	_, _ = tx.Exec(`DELETE FROM user_group_user WHERE user_group_id = ?`, req.GroupID)
	for _, uid := range req.UserIDs {
		_, _ = tx.Exec(`INSERT INTO user_group_user(user_group_id, user_id, created_time) VALUES(?, ?, ?) ON CONFLICT DO NOTHING`, req.GroupID, uid, time.Now().UnixMilli())
	}
	if err := revokeGroupGrantsForRemovedUsersTx(tx, req.GroupID, previousUserIDs, req.UserIDs); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := tx.Commit(); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
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
	_, err := h.repo.DB().Exec(`INSERT INTO group_permission(user_group_id, tunnel_group_id, created_time) VALUES(?, ?, ?) ON CONFLICT DO NOTHING`, req.UserGroupID, req.TunnelGroupID, time.Now().UnixMilli())
	if err != nil {
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
	tx, err := h.repo.DB().Begin()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	defer func() { _ = tx.Rollback() }()

	var ug, tg int64
	err = tx.QueryRow(`SELECT user_group_id, tunnel_group_id FROM group_permission WHERE id = ?`, id).Scan(&ug, &tg)
	if err != nil && err != sql.ErrNoRows {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	if _, err := tx.Exec(`DELETE FROM group_permission WHERE id = ?`, id); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err == nil {
		if err := revokeGroupPermissionPairTx(tx, ug, tg); err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
	}

	if err := tx.Commit(); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
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
	_, err := h.repo.DB().Exec(`INSERT INTO `+table+`(name, created_time, updated_time, status) VALUES(?, ?, ?, ?)`, name, now, now, asInt(req["status"], 1))
	if err != nil {
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
	_, err := h.repo.DB().Exec(`UPDATE `+table+` SET name = ?, status = ?, updated_time = ? WHERE id = ?`, asString(req["name"]), asInt(req["status"], 1), time.Now().UnixMilli(), id)
	if err != nil {
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
	tx, err := h.repo.DB().Begin()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	defer func() { _ = tx.Rollback() }()
	if table == "tunnel_group" {
		_, _ = tx.Exec(`DELETE FROM tunnel_group_tunnel WHERE tunnel_group_id = ?`, id)
		_, _ = tx.Exec(`DELETE FROM group_permission WHERE tunnel_group_id = ?`, id)
		_, _ = tx.Exec(`DELETE FROM group_permission_grant WHERE tunnel_group_id = ?`, id)
	} else {
		_, _ = tx.Exec(`DELETE FROM user_group_user WHERE user_group_id = ?`, id)
		_, _ = tx.Exec(`DELETE FROM group_permission WHERE user_group_id = ?`, id)
		_, _ = tx.Exec(`DELETE FROM group_permission_grant WHERE user_group_id = ?`, id)
	}
	_, _ = tx.Exec(`DELETE FROM `+table+` WHERE id = ?`, id)
	if err := tx.Commit(); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) applyGroupPermission(userGroupID, tunnelGroupID int64) error {
	db := h.repo.DB()
	userIDs, _ := queryInt64List(db, `SELECT user_id FROM user_group_user WHERE user_group_id = ?`, userGroupID)
	tunnelIDs, _ := queryInt64List(db, `SELECT tunnel_id FROM tunnel_group_tunnel WHERE tunnel_group_id = ?`, tunnelGroupID)
	for _, uid := range userIDs {
		for _, tid := range tunnelIDs {
			utID, created, err := ensureUserTunnelGrant(db, uid, tid)
			if err != nil {
				continue
			}
			createdByGroup := 0
			if created {
				createdByGroup = 1
			}
			_, _ = db.Exec(`INSERT INTO group_permission_grant(user_group_id, tunnel_group_id, user_tunnel_id, created_by_group, created_time) VALUES(?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
				userGroupID, tunnelGroupID, utID, createdByGroup, time.Now().UnixMilli())
		}
	}
	return nil
}

func (h *Handler) syncPermissionsByUserGroup(userGroupID int64) error {
	db := h.repo.DB()
	pairs, err := queryPairs(db, `SELECT user_group_id, tunnel_group_id FROM group_permission WHERE user_group_id = ?`, userGroupID)
	if err != nil {
		return err
	}
	for _, p := range pairs {
		_ = h.applyGroupPermission(p[0], p[1])
	}
	return nil
}

func (h *Handler) syncPermissionsByTunnelGroup(tunnelGroupID int64) error {
	db := h.repo.DB()
	pairs, err := queryPairs(db, `SELECT user_group_id, tunnel_group_id FROM group_permission WHERE tunnel_group_id = ?`, tunnelGroupID)
	if err != nil {
		return err
	}
	for _, p := range pairs {
		_ = h.applyGroupPermission(p[0], p[1])
	}
	return nil
}

func ensureUserTunnelGrant(db *store.DB, userID, tunnelID int64) (int64, bool, error) {
	var id int64
	err := db.QueryRow(`SELECT id FROM user_tunnel WHERE user_id = ? AND tunnel_id = ? LIMIT 1`, userID, tunnelID).Scan(&id)
	if err == nil {
		return id, false, nil
	}
	if err != sql.ErrNoRows {
		return 0, false, err
	}
	var flow int64
	var num int
	var expTime int64
	var flowReset int64
	if err := db.QueryRow(`SELECT flow, num, exp_time, flow_reset_time FROM user WHERE id = ?`, userID).Scan(&flow, &num, &expTime, &flowReset); err != nil {
		return 0, false, err
	}
	id, err = db.ExecReturningID(`INSERT INTO user_tunnel(user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status) VALUES(?, ?, NULL, ?, ?, 0, 0, ?, ?, 1)`,
		userID, tunnelID, num, flow, flowReset, expTime)
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

func queryInt64List(db *store.DB, q string, args ...interface{}) ([]int64, error) {
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]int64, 0)
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func queryInt64ListTx(tx *store.Tx, q string, args ...interface{}) ([]int64, error) {
	rows, err := tx.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]int64, 0)
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func revokeGroupGrantsForRemovedUsersTx(tx *store.Tx, userGroupID int64, previousUserIDs, currentUserIDs []int64) error {
	currentSet := make(map[int64]struct{}, len(currentUserIDs))
	for _, uid := range currentUserIDs {
		if uid > 0 {
			currentSet[uid] = struct{}{}
		}
	}

	removedUserIDs := make([]int64, 0)
	for _, uid := range previousUserIDs {
		if uid <= 0 {
			continue
		}
		if _, ok := currentSet[uid]; !ok {
			removedUserIDs = append(removedUserIDs, uid)
		}
	}
	if len(removedUserIDs) == 0 {
		return nil
	}

	for _, userID := range removedUserIDs {
		rows, err := tx.Query(`
			SELECT g.user_tunnel_id, g.created_by_group
			FROM group_permission_grant g
			JOIN user_tunnel ut ON ut.id = g.user_tunnel_id
			WHERE g.user_group_id = ? AND ut.user_id = ?
		`, userGroupID, userID)
		if err != nil {
			return err
		}

		groupCreatedTunnelIDs := make(map[int64]struct{})
		for rows.Next() {
			var userTunnelID int64
			var createdByGroup int
			if err := rows.Scan(&userTunnelID, &createdByGroup); err != nil {
				rows.Close()
				return err
			}
			if createdByGroup == 1 && userTunnelID > 0 {
				groupCreatedTunnelIDs[userTunnelID] = struct{}{}
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()

		if _, err := tx.Exec(`
			DELETE FROM group_permission_grant
			WHERE user_group_id = ?
			  AND user_tunnel_id IN (SELECT id FROM user_tunnel WHERE user_id = ?)
		`, userGroupID, userID); err != nil {
			return err
		}

		for userTunnelID := range groupCreatedTunnelIDs {
			var remaining int
			if err := tx.QueryRow(`SELECT COUNT(1) FROM group_permission_grant WHERE user_tunnel_id = ?`, userTunnelID).Scan(&remaining); err != nil {
				return err
			}
			if remaining == 0 {
				if _, err := tx.Exec(`DELETE FROM user_tunnel WHERE id = ?`, userTunnelID); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func revokeGroupPermissionPairTx(tx *store.Tx, userGroupID, tunnelGroupID int64) error {
	rows, err := tx.Query(`
		SELECT user_tunnel_id, created_by_group
		FROM group_permission_grant
		WHERE user_group_id = ? AND tunnel_group_id = ?
	`, userGroupID, tunnelGroupID)
	if err != nil {
		return err
	}

	groupCreatedTunnelIDs := make(map[int64]struct{})
	for rows.Next() {
		var userTunnelID int64
		var createdByGroup int
		if err := rows.Scan(&userTunnelID, &createdByGroup); err != nil {
			rows.Close()
			return err
		}
		if createdByGroup == 1 && userTunnelID > 0 {
			groupCreatedTunnelIDs[userTunnelID] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	if _, err := tx.Exec(`DELETE FROM group_permission_grant WHERE user_group_id = ? AND tunnel_group_id = ?`, userGroupID, tunnelGroupID); err != nil {
		return err
	}

	for userTunnelID := range groupCreatedTunnelIDs {
		var remaining int
		if err := tx.QueryRow(`SELECT COUNT(1) FROM group_permission_grant WHERE user_tunnel_id = ?`, userTunnelID).Scan(&remaining); err != nil {
			return err
		}
		if remaining == 0 {
			if _, err := tx.Exec(`DELETE FROM user_tunnel WHERE id = ?`, userTunnelID); err != nil {
				return err
			}
		}
	}

	return nil
}

func queryPairs(db *store.DB, q string, args ...interface{}) ([][2]int64, error) {
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([][2]int64, 0)
	for rows.Next() {
		var a, b int64
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		out = append(out, [2]int64{a, b})
	}
	return out, rows.Err()
}

type tunnelRuntimeNode struct {
	NodeID    int64
	Protocol  string
	Strategy  string
	Inx       int
	ChainType int
	Port      int
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

func (h *Handler) prepareTunnelCreateState(tx *store.Tx, req map[string]interface{}, tunnelType int, excludeTunnelID int64) (*tunnelCreateState, error) {
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
				isRemote, remoteErr := isRemoteNodeTx(tx, nodeID)
				if remoteErr != nil {
					return nil, remoteErr
				}
				if !isRemote {
					var err error
					port, err = pickNodePortTx(tx, nodeID, allocated, excludeTunnelID)
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
					isRemote, remoteErr := isRemoteNodeTx(tx, nodeID)
					if remoteErr != nil {
						return nil, remoteErr
					}
					if !isRemote {
						var err error
						port, err = pickNodePortTx(tx, nodeID, allocated, excludeTunnelID)
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
				})
			}
			if len(hop) > 0 {
				state.ChainHops = append(state.ChainHops, hop)
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
		node, err := h.getNodeRecord(nodeID)
		if err != nil {
			if strings.Contains(err.Error(), "不存在") {
				return nil, errors.New("节点不存在")
			}
			return nil, err
		}
		if node.IsRemote != 1 && node.Status != 1 {
			return nil, errors.New("部分节点不在线")
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

func buildTunnelInIP(inNodes []tunnelRuntimeNode, nodes map[int64]*nodeRecord) string {
	set := make(map[string]struct{})
	ordered := make([]string, 0)
	for _, inNode := range inNodes {
		node := nodes[inNode.NodeID]
		if node == nil {
			continue
		}
		if v := strings.TrimSpace(node.ServerIPv4); v != "" {
			if _, ok := set[v]; !ok {
				set[v] = struct{}{}
				ordered = append(ordered, v)
			}
		}
		if v := strings.TrimSpace(node.ServerIPv6); v != "" {
			if _, ok := set[v]; !ok {
				set[v] = struct{}{}
				ordered = append(ordered, v)
			}
		}
		if strings.TrimSpace(node.ServerIPv4) == "" && strings.TrimSpace(node.ServerIPv6) == "" {
			if v := strings.TrimSpace(node.ServerIP); v != "" {
				if _, ok := set[v]; !ok {
					set[v] = struct{}{}
					ordered = append(ordered, v)
				}
			}
		}
	}
	return strings.Join(ordered, ",")
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

func (h *Handler) applyFederationRuntime(state *tunnelCreateState) ([]sqlite.FederationTunnelBinding, []federationRuntimeReleaseRef, error) {
	bindings := make([]sqlite.FederationTunnelBinding, 0)
	releaseRefs := make([]federationRuntimeReleaseRef, 0)
	if h == nil || state == nil {
		return bindings, releaseRefs, nil
	}
	fc := client.NewFederationClient()
	localDomain := h.federationLocalDomain()
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

		bindings = append(bindings, sqlite.FederationTunnelBinding{
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
				host, hostErr := selectTunnelDialHost(node, targetNode, state.IPPreference)
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

			bindings = append(bindings, sqlite.FederationTunnelBinding{
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

func replaceFederationTunnelBindingsTx(tx *store.Tx, tunnelID int64, bindings []sqlite.FederationTunnelBinding) error {
	if tx == nil {
		return errors.New("database unavailable")
	}
	if _, err := tx.Exec(`DELETE FROM federation_tunnel_binding WHERE tunnel_id = ?`, tunnelID); err != nil {
		return err
	}
	for _, b := range bindings {
		created := b.CreatedTime
		if created <= 0 {
			created = time.Now().UnixMilli()
		}
		updated := b.UpdatedTime
		if updated <= 0 {
			updated = created
		}
		_, err := tx.Exec(`
			INSERT INTO federation_tunnel_binding(tunnel_id, node_id, chain_type, hop_inx, remote_url, resource_key, remote_binding_id, allocated_port, status, created_time, updated_time)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, tunnelID, b.NodeID, b.ChainType, b.HopInx, b.RemoteURL, b.ResourceKey, b.RemoteBindingID, b.AllocatedPort, b.Status, created, updated)
		if err != nil {
			return err
		}
	}
	return nil
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
		node := state.Nodes[inNode.NodeID]
		targets := state.OutNodes
		if len(state.ChainHops) > 0 {
			targets = state.ChainHops[0]
		}
		chainData, err := buildTunnelChainConfig(state.TunnelID, inNode.NodeID, targets, state.Nodes, state.IPPreference)
		if err != nil {
			return createdChains, createdServices, err
		}
		if _, err := h.sendNodeCommand(inNode.NodeID, "AddChains", chainData, true, false); err != nil {
			if node != nil && node.IsRemote == 1 && shouldDeferTunnelRuntimeApplyError(err) {
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
			if node := state.Nodes[chainNode.NodeID]; node != nil && node.IsRemote == 1 {
				continue
			}
			chainData, err := buildTunnelChainConfig(state.TunnelID, chainNode.NodeID, nextTargets, state.Nodes, state.IPPreference)
			if err != nil {
				return createdChains, createdServices, err
			}
			if _, err := h.sendNodeCommand(chainNode.NodeID, "AddChains", chainData, true, false); err != nil {
				return createdChains, createdServices, fmt.Errorf("转发链节点 %s 下发转发链失败: %w", nodeDisplayName(state.Nodes[chainNode.NodeID]), err)
			}
			createdChains = append(createdChains, chainNode.NodeID)

			serviceData := buildTunnelChainServiceConfig(state.TunnelID, chainNode, state.Nodes[chainNode.NodeID])
			if _, err := h.sendNodeCommand(chainNode.NodeID, "AddService", serviceData, true, false); err != nil {
				return createdChains, createdServices, fmt.Errorf("转发链节点 %s 下发服务失败: %w", nodeDisplayName(state.Nodes[chainNode.NodeID]), err)
			}
			createdServices = append(createdServices, chainNode.NodeID)
		}
	}

	for _, outNode := range state.OutNodes {
		if node := state.Nodes[outNode.NodeID]; node != nil && node.IsRemote == 1 {
			continue
		}
		serviceData := buildTunnelChainServiceConfig(state.TunnelID, outNode, state.Nodes[outNode.NodeID])
		if _, err := h.sendNodeCommand(outNode.NodeID, "AddService", serviceData, true, false); err != nil {
			return createdChains, createdServices, fmt.Errorf("出口节点 %s 下发服务失败: %w", nodeDisplayName(state.Nodes[outNode.NodeID]), err)
		}
		createdServices = append(createdServices, outNode.NodeID)
	}

	return createdChains, createdServices, nil
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

func buildTunnelChainConfig(tunnelID int64, fromNodeID int64, targets []tunnelRuntimeNode, nodes map[int64]*nodeRecord, ipPreference string) (map[string]interface{}, error) {
	fromNode := nodes[fromNodeID]
	if fromNode == nil {
		return nil, errors.New("节点不存在")
	}
	if len(targets) == 0 {
		return nil, errors.New("转发链目标不能为空")
	}
	nodeItems := make([]map[string]interface{}, 0, len(targets))
	for idx, target := range targets {
		targetNode := nodes[target.NodeID]
		if targetNode == nil {
			return nil, errors.New("节点不存在")
		}
		host, err := selectTunnelDialHost(fromNode, targetNode, ipPreference)
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

	strategy := defaultString(strings.TrimSpace(targets[0].Strategy), "round")
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

func buildTunnelChainServiceConfig(tunnelID int64, chainNode tunnelRuntimeNode, node *nodeRecord) []map[string]interface{} {
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
	service := map[string]interface{}{
		"name":    fmt.Sprintf("%d_tls", tunnelID),
		"addr":    fmt.Sprintf("%s:%d", node.TCPListenAddr, chainNode.Port),
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

func selectTunnelDialHost(fromNode, toNode *nodeRecord, ipPreference string) (string, error) {
	if fromNode == nil || toNode == nil {
		return "", errors.New("节点不存在")
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

func isRemoteNodeTx(tx *store.Tx, nodeID int64) (bool, error) {
	if tx == nil {
		return false, errors.New("database unavailable")
	}
	if nodeID <= 0 {
		return false, errors.New("节点不存在")
	}
	var isRemote int
	if err := tx.QueryRow(`SELECT is_remote FROM node WHERE id = ? LIMIT 1`, nodeID).Scan(&isRemote); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, errors.New("节点不存在")
		}
		return false, err
	}
	return isRemote == 1, nil
}

func pickNodePortTx(tx *store.Tx, nodeID int64, allocated map[int64]int, excludeTunnelID int64) (int, error) {
	if tx == nil {
		return 0, errors.New("database unavailable")
	}
	if nodeID <= 0 {
		return 0, errors.New("节点不存在")
	}
	if port, ok := allocated[nodeID]; ok && port > 0 {
		return port, nil
	}

	var portRange string
	if err := tx.QueryRow(`SELECT port FROM node WHERE id = ? LIMIT 1`, nodeID).Scan(&portRange); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errors.New("节点不存在")
		}
		return 0, err
	}
	candidates := parsePortRangeSpec(portRange)
	if len(candidates) == 0 {
		return 0, errors.New("节点端口已满，无可用端口")
	}

	used := map[int]struct{}{}
	var chainRows *sql.Rows
	var err error
	if excludeTunnelID > 0 {
		chainRows, err = tx.Query(`SELECT port FROM chain_tunnel WHERE node_id = ? AND port IS NOT NULL AND tunnel_id != ?`, nodeID, excludeTunnelID)
	} else {
		chainRows, err = tx.Query(`SELECT port FROM chain_tunnel WHERE node_id = ? AND port IS NOT NULL`, nodeID)
	}
	if err != nil {
		return 0, err
	}
	for chainRows.Next() {
		var p sql.NullInt64
		if scanErr := chainRows.Scan(&p); scanErr == nil && p.Valid && p.Int64 > 0 {
			used[int(p.Int64)] = struct{}{}
		}
	}
	_ = chainRows.Close()

	forwardRows, err := tx.Query(`SELECT port FROM forward_port WHERE node_id = ?`, nodeID)
	if err != nil {
		return 0, err
	}
	for forwardRows.Next() {
		var p sql.NullInt64
		if scanErr := forwardRows.Scan(&p); scanErr == nil && p.Valid && p.Int64 > 0 {
			used[int(p.Int64)] = struct{}{}
		}
	}
	_ = forwardRows.Close()

	for _, candidate := range candidates {
		if candidate <= 0 {
			continue
		}
		if _, ok := used[candidate]; ok {
			continue
		}
		allocated[nodeID] = candidate
		return candidate, nil
	}
	return 0, errors.New("节点端口已满，无可用端口")
}

func parsePortRangeSpec(input string) []int {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	set := make(map[int]struct{})
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
			for p := start; p <= end; p++ {
				set[p] = struct{}{}
			}
			continue
		}
		p, err := strconv.Atoi(part)
		if err != nil || p <= 0 {
			continue
		}
		set[p] = struct{}{}
	}
	out := make([]int, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}

func replaceTunnelChainsTx(tx *store.Tx, tunnelID int64, req map[string]interface{}) error {
	allocated := map[int64]int{}
	inNodes := asMapSlice(req["inNodeId"])
	for _, n := range inNodes {
		nodeID := asInt64(n["nodeId"], 0)
		if nodeID <= 0 {
			continue
		}
		_, err := tx.Exec(`INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol) VALUES(?, '1', ?, NULL, ?, 0, ?)`,
			tunnelID, nodeID, defaultString(asString(n["strategy"]), "round"), defaultString(asString(n["protocol"]), "tls"))
		if err != nil {
			return err
		}
	}
	for _, n := range asMapSlice(req["outNodeId"]) {
		nodeID := asInt64(n["nodeId"], 0)
		if nodeID <= 0 {
			continue
		}
		port := asInt(n["port"], 0)
		if port <= 0 {
			var pickErr error
			port, pickErr = pickNodePortTx(tx, nodeID, allocated, 0)
			if pickErr != nil {
				return pickErr
			}
		}
		_, err := tx.Exec(`INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol) VALUES(?, '3', ?, ?, ?, 0, ?)`,
			tunnelID, nodeID, port, defaultString(asString(n["strategy"]), "round"), defaultString(asString(n["protocol"]), "tls"))
		if err != nil {
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
				port, pickErr = pickNodePortTx(tx, nodeID, allocated, 0)
				if pickErr != nil {
					return pickErr
				}
			}
			_, err := tx.Exec(`INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol) VALUES(?, '2', ?, ?, ?, ?, ?)`,
				tunnelID, nodeID, port, defaultString(asString(n["strategy"]), "round"), i+1, defaultString(asString(n["protocol"]), "tls"))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *Handler) deleteNodeByID(id int64) error {
	tx, err := h.repo.DB().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, _ = tx.Exec(`DELETE FROM forward_port WHERE node_id = ?`, id)
	_, _ = tx.Exec(`DELETE FROM chain_tunnel WHERE node_id = ?`, id)
	_, _ = tx.Exec(`DELETE FROM federation_tunnel_binding WHERE node_id = ?`, id)
	_, err = tx.Exec(`DELETE FROM node WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (h *Handler) deleteTunnelByID(id int64) error {
	tx, err := h.repo.DB().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, _ = tx.Exec(`DELETE FROM forward_port WHERE forward_id IN (SELECT id FROM forward WHERE tunnel_id = ?)`, id)
	_, _ = tx.Exec(`DELETE FROM forward WHERE tunnel_id = ?`, id)
	_, _ = tx.Exec(`DELETE FROM user_tunnel WHERE tunnel_id = ?`, id)
	_, _ = tx.Exec(`DELETE FROM speed_limit WHERE tunnel_id = ?`, id)
	_, _ = tx.Exec(`DELETE FROM chain_tunnel WHERE tunnel_id = ?`, id)
	_, _ = tx.Exec(`DELETE FROM federation_tunnel_binding WHERE tunnel_id = ?`, id)
	_, err = tx.Exec(`DELETE FROM tunnel WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (h *Handler) deleteForwardByID(id int64) error {
	tx, err := h.repo.DB().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, _ = tx.Exec(`DELETE FROM forward_port WHERE forward_id = ?`, id)
	_, err = tx.Exec(`DELETE FROM forward WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return tx.Commit()
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
	s := 0
	f := 0
	for _, id := range ids {
		if _, err := h.repo.DB().Exec(`UPDATE forward SET status = ?, updated_time = ? WHERE id = ?`, status, time.Now().UnixMilli(), id); err != nil {
			f++
		} else {
			s++
		}
	}
	return s, f
}

func (h *Handler) tunnelEntryNodeIDs(tunnelID int64) ([]int64, error) {
	rows, err := h.repo.DB().Query(`SELECT node_id FROM chain_tunnel WHERE tunnel_id = ? AND chain_type = '1' ORDER BY inx ASC, id ASC`, tunnelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}

func (h *Handler) pickTunnelPort(tunnelID int64) int {
	entryNodes, err := h.tunnelEntryNodeIDs(tunnelID)
	if err != nil || len(entryNodes) == 0 {
		return 10000
	}

	var commonAvailable []int
	firstNode := true

	for _, nodeID := range entryNodes {
		var portRange string
		if err := h.repo.DB().QueryRow("SELECT port FROM node WHERE id = ?", nodeID).Scan(&portRange); err != nil {
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
	used := make(map[int]bool)
	rows, err := h.repo.DB().Query("SELECT port FROM forward_port WHERE node_id = ?", nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err == nil {
			used[p] = true
		}
	}
	rows2, err := h.repo.DB().Query("SELECT port FROM chain_tunnel WHERE node_id = ? AND port > 0", nodeID)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var p int
		if err := rows2.Scan(&p); err == nil {
			used[p] = true
		}
	}
	return used, nil
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

func (h *Handler) replaceForwardPorts(forwardID, tunnelID int64, port int) error {
	tx, err := h.repo.DB().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM forward_port WHERE forward_id = ?`, forwardID); err != nil {
		return err
	}
	entryNodes, err := h.tunnelEntryNodeIDs(tunnelID)
	if err != nil {
		return err
	}
	for _, nodeID := range entryNodes {
		if _, err := tx.Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(?, ?, ?)`, forwardID, nodeID, port); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (h *Handler) replaceForwardPortsWithRecords(forwardID int64, ports []forwardPortRecord) error {
	tx, err := h.repo.DB().Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM forward_port WHERE forward_id = ?`, forwardID); err != nil {
		return err
	}
	for _, fp := range ports {
		if _, err := tx.Exec(`INSERT INTO forward_port(forward_id, node_id, port) VALUES(?, ?, ?)`, forwardID, fp.NodeID, fp.Port); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (h *Handler) rollbackForwardMutation(oldForward *forwardRecord, oldPorts []forwardPortRecord) {
	if h == nil || oldForward == nil || h.repo == nil || h.repo.DB() == nil {
		return
	}

	_, _ = h.repo.DB().Exec(`
		UPDATE forward
		SET user_id = ?, user_name = ?, name = ?, tunnel_id = ?, remote_addr = ?, strategy = ?, status = ?, updated_time = ?
		WHERE id = ?
	`, oldForward.UserID, oldForward.UserName, oldForward.Name, oldForward.TunnelID, oldForward.RemoteAddr, oldForward.Strategy, oldForward.Status, time.Now().UnixMilli(), oldForward.ID)

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
	db := h.repo.DB()
	var existingID int64
	var currentFlow, currentNum, currentExpTime, currentFlowReset int64
	var currentSpeedID sql.NullInt64
	var currentStatus int

	err := db.QueryRow(`
		SELECT id, flow, num, exp_time, flow_reset_time, speed_id, status 
		FROM user_tunnel 
		WHERE user_id = ? AND tunnel_id = ? 
		LIMIT 1
	`, userID, tunnelID).Scan(&existingID, &currentFlow, &currentNum, &currentExpTime, &currentFlowReset, &currentSpeedID, &currentStatus)

	speedID := asAnyToInt64Ptr(req["speedId"])
	reqFlow := asInt64(req["flow"], -1)
	reqNum := asInt(req["num"], -1)
	reqExpTime := asInt64(req["expTime"], -1)
	reqFlowReset := asInt64(req["flowResetTime"], -1)
	reqStatus := asInt(req["status"], -1)

	if err == sql.ErrNoRows {
		if reqFlow < 0 || reqNum < 0 || reqExpTime < 0 || reqFlowReset < 0 {
			var uFlow, uNum, uExp, uReset int64
			if uErr := db.QueryRow(`SELECT flow, num, exp_time, flow_reset_time FROM user WHERE id = ?`, userID).Scan(&uFlow, &uNum, &uExp, &uReset); uErr == nil {
				if reqFlow < 0 {
					reqFlow = uFlow
				}
				if reqNum < 0 {
					reqNum = int(uNum)
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

		_, err = db.Exec(`INSERT INTO user_tunnel(user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status) VALUES(?, ?, ?, ?, ?, 0, 0, ?, ?, ?)`,
			userID, tunnelID, nullableInt(speedID), reqNum, reqFlow, reqFlowReset, reqExpTime, reqStatus)
		return err
	}
	if err != nil {
		return err
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

	_, err = db.Exec(`UPDATE user_tunnel SET speed_id = ?, flow = ?, num = ?, exp_time = ?, flow_reset_time = ?, status = ? WHERE id = ?`,
		newSpeedID, newFlow, newNum, newExpTime, newFlowReset, newStatus, existingID)

	if err == nil {
		h.syncUserTunnelForwards(userID, tunnelID)
	}
	return err
}

func (h *Handler) syncUserTunnelForwards(userID, tunnelID int64) {
	forwards, err := h.listForwardsByTunnel(tunnelID)
	if err != nil {
		return
	}
	for i := range forwards {
		f := &forwards[i]
		if f.UserID == userID {
			_ = h.syncForwardServices(f, "UpdateService", true)
		}
	}
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

func nextIndex(db *store.DB, table string) int {
	if db == nil {
		return 0
	}
	row := db.QueryRow(`SELECT COALESCE(MAX(inx), -1) + 1 FROM ` + table)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0
	}
	if n < 0 {
		return 0
	}
	return n
}
