package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go-backend/internal/http/client"
	"go-backend/internal/http/response"
	"go-backend/internal/store/sqlite"
)

type federationTunnelRequest struct {
	Protocol   string `json:"protocol"`
	RemotePort int    `json:"remotePort"`
	Target     string `json:"target"`
}

type createPeerShareRequest struct {
	Name           string `json:"name"`
	NodeID         int64  `json:"nodeId"`
	MaxBandwidth   int64  `json:"maxBandwidth"`
	ExpiryTime     int64  `json:"expiryTime"`
	PortRangeStart int    `json:"portRangeStart"`
	PortRangeEnd   int    `json:"portRangeEnd"`
	AllowedDomains string `json:"allowedDomains"`
}

type deletePeerShareRequest struct {
	ID int64 `json:"id"`
}

type nodeImportRequest struct {
	RemoteURL string `json:"remoteUrl"`
	Token     string `json:"token"`
}

type federationRuntimeReservePortRequest struct {
	ResourceKey   string `json:"resourceKey"`
	Protocol      string `json:"protocol"`
	RequestedPort int    `json:"requestedPort"`
}

type federationRuntimeTarget struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

type federationRuntimeApplyRoleRequest struct {
	ReservationID string                    `json:"reservationId"`
	ResourceKey   string                    `json:"resourceKey"`
	Role          string                    `json:"role"`
	Protocol      string                    `json:"protocol"`
	Strategy      string                    `json:"strategy"`
	Targets       []federationRuntimeTarget `json:"targets"`
}

type federationRuntimeReleaseRoleRequest struct {
	BindingID     string `json:"bindingId"`
	ReservationID string `json:"reservationId"`
	ResourceKey   string `json:"resourceKey"`
}

type federationRuntimeDiagnoseRequest struct {
	IP      string `json:"ip"`
	Port    int    `json:"port"`
	Count   int    `json:"count"`
	Timeout int    `json:"timeout"`
}

func (h *Handler) federationShareList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("Invalid method"))
		return
	}

	shares, err := h.repo.ListPeerShares()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(shares))
}

func (h *Handler) federationShareCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("Invalid method"))
		return
	}

	var req createPeerShareRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("Invalid JSON"))
		return
	}

	if req.Name == "" || req.NodeID == 0 {
		response.WriteJSON(w, response.ErrDefault("Name and NodeID are required"))
		return
	}

	if req.MaxBandwidth < 0 {
		response.WriteJSON(w, response.ErrDefault("Max bandwidth cannot be negative"))
		return
	}

	if req.ExpiryTime < 0 {
		response.WriteJSON(w, response.ErrDefault("Expiry time cannot be negative"))
		return
	}

	if req.PortRangeStart < 0 || req.PortRangeStart > 65535 || req.PortRangeEnd < 0 || req.PortRangeEnd > 65535 {
		response.WriteJSON(w, response.ErrDefault("Invalid port range"))
		return
	}

	if req.PortRangeStart > req.PortRangeEnd {
		response.WriteJSON(w, response.ErrDefault("Port range start cannot be greater than end"))
		return
	}

	node, err := h.repo.GetNodeByID(req.NodeID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if node == nil {
		response.WriteJSON(w, response.ErrDefault("Node not found"))
		return
	}

	now := time.Now().UnixMilli()
	token := randomToken(32)

	share := &sqlite.PeerShare{
		Name:           req.Name,
		NodeID:         req.NodeID,
		Token:          token,
		MaxBandwidth:   req.MaxBandwidth,
		ExpiryTime:     req.ExpiryTime,
		PortRangeStart: req.PortRangeStart,
		PortRangeEnd:   req.PortRangeEnd,
		IsActive:       1,
		CreatedTime:    now,
		UpdatedTime:    now,
		AllowedDomains: req.AllowedDomains,
	}

	if err := h.repo.CreatePeerShare(share); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) federationShareDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("Invalid method"))
		return
	}

	var req deletePeerShareRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("Invalid JSON"))
		return
	}

	if err := h.repo.DeletePeerShare(req.ID); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) nodeImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("Invalid method"))
		return
	}

	var req nodeImportRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("Invalid JSON"))
		return
	}

	if req.RemoteURL == "" || req.Token == "" {
		response.WriteJSON(w, response.ErrDefault("Remote URL and Token are required"))
		return
	}

	domainCfg, _ := h.repo.GetConfigByName("panel_domain")
	localDomain := ""
	if domainCfg != nil {
		localDomain = domainCfg.Value
	}

	fc := client.NewFederationClient()
	info, err := fc.Connect(req.RemoteURL, req.Token, localDomain)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, "Failed to connect: "+err.Error()))
		return
	}

	// Prepare config json for local storage (metadata about limits)
	configData := map[string]interface{}{
		"shareId":        info.ShareID,
		"maxBandwidth":   info.MaxBandwidth,
		"expiryTime":     info.ExpiryTime,
		"portRangeStart": info.PortRangeStart,
		"portRangeEnd":   info.PortRangeEnd,
	}
	configBytes, _ := json.Marshal(configData)

	portRange := "0"
	if info.PortRangeStart > 0 && info.PortRangeEnd >= info.PortRangeStart {
		portRange = fmt.Sprintf("%d-%d", info.PortRangeStart, info.PortRangeEnd)
	}

	db := h.repo.DB()
	inx := nextIndex(db, "node")
	now := time.Now().UnixMilli()

	_, err = db.Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx, is_remote, remote_url, remote_token, remote_config)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)
	`,
		fmt.Sprintf("%s (Remote)", info.NodeName),
		randomToken(16), // Dummy secret
		info.ServerIP,
		"", "", // v4/v6 unknown, use server_ip
		portRange,
		"",
		"",
		now, now,
		info.Status,
		"[::]", "[::]",
		inx,
		req.RemoteURL,
		req.Token,
		string(configBytes),
	)

	if err != nil {
		response.WriteJSON(w, response.Err(-2, "Database error: "+err.Error()))
		return
	}

	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) authPeer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			response.WriteJSON(w, response.Err(401, "Missing Authorization header"))
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			response.WriteJSON(w, response.Err(401, "Invalid Authorization format"))
			return
		}

		token := parts[1]
		share, err := h.repo.GetPeerShareByToken(token)
		if err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
		if share == nil {
			response.WriteJSON(w, response.Err(401, "Invalid token"))
			return
		}

		if share.IsActive == 0 {
			response.WriteJSON(w, response.Err(403, "Share is disabled"))
			return
		}

		if share.ExpiryTime > 0 && share.ExpiryTime < time.Now().UnixMilli() {
			response.WriteJSON(w, response.Err(403, "Share expired"))
			return
		}

		if share.AllowedDomains != "" {
			clientDomain := r.Header.Get("X-Panel-Domain")
			if clientDomain == "" {
				response.WriteJSON(w, response.Err(403, "Domain verification required"))
				return
			}
			allowed := false
			domains := strings.Split(share.AllowedDomains, ",")
			for _, d := range domains {
				if strings.TrimSpace(d) == clientDomain {
					allowed = true
					break
				}
			}
			if !allowed {
				response.WriteJSON(w, response.Err(403, "Domain not allowed"))
				return
			}
		}

		next(w, r)
	}
}

func (h *Handler) federationConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("Invalid method"))
		return
	}

	token := extractBearerToken(r)
	share, err := h.repo.GetPeerShareByToken(token)
	if err != nil || share == nil {
		response.WriteJSON(w, response.Err(401, "Unauthorized"))
		return
	}

	var nodeName string
	var serverIP string
	var status int

	err = h.repo.DB().QueryRow("SELECT name, server_ip, status FROM node WHERE id = ?", share.NodeID).Scan(&nodeName, &serverIP, &status)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, "Node not found"))
		return
	}

	response.WriteJSON(w, response.OK(map[string]interface{}{
		"shareId":        share.ID,
		"shareName":      share.Name,
		"nodeId":         share.NodeID,
		"nodeName":       nodeName,
		"serverIp":       serverIP,
		"status":         status,
		"maxBandwidth":   share.MaxBandwidth,
		"expiryTime":     share.ExpiryTime,
		"portRangeStart": share.PortRangeStart,
		"portRangeEnd":   share.PortRangeEnd,
	}))
}

func (h *Handler) federationTunnelCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("Invalid method"))
		return
	}

	token := extractBearerToken(r)
	share, err := h.repo.GetPeerShareByToken(token)
	if err != nil || share == nil {
		response.WriteJSON(w, response.Err(401, "Unauthorized"))
		return
	}

	var req federationTunnelRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("Invalid JSON"))
		return
	}

	if req.RemotePort < share.PortRangeStart || req.RemotePort > share.PortRangeEnd {
		response.WriteJSON(w, response.Err(403, "Port out of range"))
		return
	}

	tunnelType := 1
	if strings.ToLower(req.Protocol) == "udp" {
		tunnelType = 2
	}

	tx, err := h.repo.DB().Begin()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	defer tx.Rollback()

	now := time.Now().UnixMilli()
	res, err := tx.Exec(`INSERT INTO tunnel (name, type, protocol, flow, created_time, updated_time, status, in_ip) VALUES (?, ?, ?, 0, ?, ?, 1, ?)`,
		fmt.Sprintf("Share-%d-Port-%d", share.ID, req.RemotePort),
		tunnelType,
		req.Protocol,
		now,
		now,
		"",
	)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	tunnelID, _ := res.LastInsertId()

	_, err = tx.Exec(`INSERT INTO chain_tunnel (tunnel_id, chain_type, node_id, port, strategy, inx, protocol) VALUES (?, 1, ?, ?, 'fifo', 0, ?)`,
		tunnelID,
		share.NodeID,
		req.RemotePort,
		req.Protocol,
	)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	if err := tx.Commit(); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	h.wsServer.SendCommand(share.NodeID, "reload", nil, time.Second*5)

	response.WriteJSON(w, response.OK(map[string]interface{}{
		"tunnelId": tunnelID,
	}))
}

func (h *Handler) federationRuntimeReservePort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("Invalid method"))
		return
	}

	token := extractBearerToken(r)
	share, err := h.repo.GetPeerShareByToken(token)
	if err != nil || share == nil {
		response.WriteJSON(w, response.Err(401, "Unauthorized"))
		return
	}

	var req federationRuntimeReservePortRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("Invalid JSON"))
		return
	}
	req.ResourceKey = strings.TrimSpace(req.ResourceKey)
	if req.ResourceKey == "" {
		response.WriteJSON(w, response.ErrDefault("resourceKey is required"))
		return
	}

	existing, err := h.repo.GetPeerShareRuntimeByResourceKey(share.ID, req.ResourceKey)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if existing != nil && existing.Status == 1 {
		response.WriteJSON(w, response.OK(map[string]interface{}{
			"reservationId": existing.ReservationID,
			"allocatedPort": existing.Port,
			"bindingId":     existing.BindingID,
		}))
		return
	}

	allocatedPort, err := h.pickPeerSharePort(share, req.RequestedPort)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}

	now := time.Now().UnixMilli()
	if existing != nil {
		existing.Protocol = defaultString(req.Protocol, "tls")
		existing.Port = allocatedPort
		existing.BindingID = ""
		existing.Role = ""
		existing.ChainName = ""
		existing.ServiceName = ""
		existing.Strategy = "round"
		existing.Target = ""
		existing.Applied = 0
		existing.Status = 1
		existing.UpdatedTime = now
		if err := h.repo.UpdatePeerShareRuntime(existing); err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
		response.WriteJSON(w, response.OK(map[string]interface{}{
			"reservationId": existing.ReservationID,
			"allocatedPort": existing.Port,
			"bindingId":     existing.BindingID,
		}))
		return
	}

	runtime := &sqlite.PeerShareRuntime{
		ShareID:       share.ID,
		NodeID:        share.NodeID,
		ReservationID: randomToken(24),
		ResourceKey:   req.ResourceKey,
		BindingID:     "",
		Role:          "",
		ChainName:     "",
		ServiceName:   "",
		Protocol:      defaultString(req.Protocol, "tls"),
		Strategy:      "round",
		Port:          allocatedPort,
		Target:        "",
		Applied:       0,
		Status:        1,
		CreatedTime:   now,
		UpdatedTime:   now,
	}
	if err := h.repo.CreatePeerShareRuntime(runtime); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OK(map[string]interface{}{
		"reservationId": runtime.ReservationID,
		"allocatedPort": runtime.Port,
		"bindingId":     runtime.BindingID,
	}))
}

func (h *Handler) federationRuntimeApplyRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("Invalid method"))
		return
	}

	token := extractBearerToken(r)
	share, err := h.repo.GetPeerShareByToken(token)
	if err != nil || share == nil {
		response.WriteJSON(w, response.Err(401, "Unauthorized"))
		return
	}

	var req federationRuntimeApplyRoleRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("Invalid JSON"))
		return
	}
	req.Role = strings.ToLower(strings.TrimSpace(req.Role))
	if req.Role != "middle" && req.Role != "exit" {
		response.WriteJSON(w, response.ErrDefault("Invalid role"))
		return
	}

	var runtime *sqlite.PeerShareRuntime
	if strings.TrimSpace(req.ReservationID) != "" {
		runtime, err = h.repo.GetPeerShareRuntimeByReservationID(share.ID, strings.TrimSpace(req.ReservationID))
	} else {
		runtime, err = h.repo.GetPeerShareRuntimeByResourceKey(share.ID, strings.TrimSpace(req.ResourceKey))
	}
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if runtime == nil || runtime.Status == 0 {
		response.WriteJSON(w, response.ErrDefault("Reservation not found"))
		return
	}

	if runtime.Applied == 1 && strings.TrimSpace(runtime.BindingID) != "" {
		response.WriteJSON(w, response.OK(map[string]interface{}{
			"bindingId":     runtime.BindingID,
			"allocatedPort": runtime.Port,
			"reservationId": runtime.ReservationID,
		}))
		return
	}

	node, err := h.getNodeRecord(share.NodeID)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}

	protocol := defaultString(req.Protocol, runtime.Protocol)
	strategy := defaultString(req.Strategy, "round")
	chainName := fmt.Sprintf("fed_chain_%d", runtime.ID)
	serviceName := fmt.Sprintf("fed_svc_%d", runtime.ID)

	if req.Role == "middle" {
		if len(req.Targets) == 0 {
			response.WriteJSON(w, response.ErrDefault("targets are required for middle role"))
			return
		}
		nodeItems := make([]map[string]interface{}, 0, len(req.Targets))
		for i, target := range req.Targets {
			host := strings.TrimSpace(target.Host)
			if host == "" || target.Port <= 0 {
				response.WriteJSON(w, response.ErrDefault("Invalid target"))
				return
			}
			nodeItems = append(nodeItems, map[string]interface{}{
				"name": fmt.Sprintf("node_%d", i+1),
				"addr": processServerAddress(fmt.Sprintf("%s:%d", host, target.Port)),
				"connector": map[string]interface{}{
					"type": "relay",
				},
				"dialer": map[string]interface{}{
					"type": defaultString(target.Protocol, protocol),
				},
			})
		}

		chainData := map[string]interface{}{
			"name": chainName,
			"hops": []map[string]interface{}{
				{
					"name": fmt.Sprintf("hop_%d", runtime.ID),
					"selector": map[string]interface{}{
						"strategy":    strategy,
						"maxFails":    1,
						"failTimeout": int64(600000000000),
					},
					"nodes": nodeItems,
				},
			},
		}
		if strings.TrimSpace(node.InterfaceName) != "" {
			hops := chainData["hops"].([]map[string]interface{})
			hops[0]["interface"] = node.InterfaceName
		}
		if _, err := h.sendNodeCommand(share.NodeID, "AddChains", chainData, true, false); err != nil {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
	}

	service := map[string]interface{}{
		"name": serviceName,
		"addr": fmt.Sprintf("%s:%d", node.TCPListenAddr, runtime.Port),
		"handler": map[string]interface{}{
			"type": "relay",
		},
		"listener": map[string]interface{}{
			"type": protocol,
		},
	}
	if req.Role == "middle" {
		service["handler"].(map[string]interface{})["chain"] = chainName
	}
	if req.Role == "exit" && strings.TrimSpace(node.InterfaceName) != "" {
		service["metadata"] = map[string]interface{}{"interface": node.InterfaceName}
	}
	if _, err := h.sendNodeCommand(share.NodeID, "AddService", []map[string]interface{}{service}, true, false); err != nil {
		if req.Role == "middle" {
			_, _ = h.sendNodeCommand(share.NodeID, "DeleteChains", map[string]interface{}{"chain": chainName}, false, true)
		}
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}

	targetBytes, _ := json.Marshal(req.Targets)
	runtime.BindingID = fmt.Sprintf("%d", runtime.ID)
	runtime.Role = req.Role
	runtime.ChainName = ""
	if req.Role == "middle" {
		runtime.ChainName = chainName
	}
	runtime.ServiceName = serviceName
	runtime.Protocol = protocol
	runtime.Strategy = strategy
	runtime.Target = string(targetBytes)
	runtime.Applied = 1
	runtime.Status = 1
	runtime.UpdatedTime = time.Now().UnixMilli()
	if err := h.repo.UpdatePeerShareRuntime(runtime); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OK(map[string]interface{}{
		"bindingId":     runtime.BindingID,
		"reservationId": runtime.ReservationID,
		"allocatedPort": runtime.Port,
	}))
}

func (h *Handler) federationRuntimeReleaseRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("Invalid method"))
		return
	}

	token := extractBearerToken(r)
	share, err := h.repo.GetPeerShareByToken(token)
	if err != nil || share == nil {
		response.WriteJSON(w, response.Err(401, "Unauthorized"))
		return
	}

	var req federationRuntimeReleaseRoleRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("Invalid JSON"))
		return
	}

	var runtime *sqlite.PeerShareRuntime
	if strings.TrimSpace(req.BindingID) != "" {
		runtime, err = h.repo.GetPeerShareRuntimeByBindingID(share.ID, strings.TrimSpace(req.BindingID))
	} else if strings.TrimSpace(req.ReservationID) != "" {
		runtime, err = h.repo.GetPeerShareRuntimeByReservationID(share.ID, strings.TrimSpace(req.ReservationID))
	} else if strings.TrimSpace(req.ResourceKey) != "" {
		runtime, err = h.repo.GetPeerShareRuntimeByResourceKey(share.ID, strings.TrimSpace(req.ResourceKey))
	} else {
		response.WriteJSON(w, response.ErrDefault("bindingId or reservationId or resourceKey is required"))
		return
	}
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if runtime == nil {
		response.WriteJSON(w, response.OKEmpty())
		return
	}

	if runtime.Applied == 1 {
		if strings.TrimSpace(runtime.ServiceName) != "" {
			_, _ = h.sendNodeCommand(share.NodeID, "DeleteService", map[string]interface{}{"services": []string{runtime.ServiceName}}, false, true)
		}
		if strings.TrimSpace(runtime.Role) == "middle" && strings.TrimSpace(runtime.ChainName) != "" {
			_, _ = h.sendNodeCommand(share.NodeID, "DeleteChains", map[string]interface{}{"chain": runtime.ChainName}, false, true)
		}
	}

	if err := h.repo.MarkPeerShareRuntimeReleased(runtime.ID, time.Now().UnixMilli()); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) federationRuntimeDiagnose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("Invalid method"))
		return
	}

	token := extractBearerToken(r)
	share, err := h.repo.GetPeerShareByToken(token)
	if err != nil || share == nil {
		response.WriteJSON(w, response.Err(401, "Unauthorized"))
		return
	}

	var req federationRuntimeDiagnoseRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("Invalid JSON"))
		return
	}

	req.IP = strings.TrimSpace(req.IP)
	if req.IP == "" || req.Port <= 0 || req.Port > 65535 {
		response.WriteJSON(w, response.ErrDefault("Invalid target"))
		return
	}
	if req.Count <= 0 {
		req.Count = 4
	}
	if req.Timeout <= 0 {
		req.Timeout = 5000
	}

	res, err := h.sendNodeCommand(share.NodeID, "TcpPing", map[string]interface{}{
		"ip":      req.IP,
		"port":    req.Port,
		"count":   req.Count,
		"timeout": req.Timeout,
	}, false, false)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	if res.Data == nil {
		response.WriteJSON(w, response.ErrDefault("Node did not return diagnosis data"))
		return
	}

	response.WriteJSON(w, response.OK(res.Data))
}

func (h *Handler) pickPeerSharePort(share *sqlite.PeerShare, requestedPort int) (int, error) {
	if share == nil {
		return 0, fmt.Errorf("share not found")
	}
	if share.PortRangeStart <= 0 || share.PortRangeEnd <= 0 || share.PortRangeEnd < share.PortRangeStart {
		return 0, fmt.Errorf("No available port")
	}

	used := make(map[int]struct{})

	rows, err := h.repo.DB().Query(`SELECT port FROM chain_tunnel WHERE node_id = ? AND port IS NOT NULL AND port > 0`, share.NodeID)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var p sql.NullInt64
		if scanErr := rows.Scan(&p); scanErr == nil && p.Valid && p.Int64 > 0 {
			used[int(p.Int64)] = struct{}{}
		}
	}
	_ = rows.Close()

	rows, err = h.repo.DB().Query(`SELECT port FROM forward_port WHERE node_id = ? AND port > 0`, share.NodeID)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var p sql.NullInt64
		if scanErr := rows.Scan(&p); scanErr == nil && p.Valid && p.Int64 > 0 {
			used[int(p.Int64)] = struct{}{}
		}
	}
	_ = rows.Close()

	ports, err := h.repo.ListActivePeerShareRuntimePorts(share.ID, share.NodeID)
	if err != nil {
		return 0, err
	}
	for _, p := range ports {
		if p > 0 {
			used[p] = struct{}{}
		}
	}

	if requestedPort > 0 {
		if requestedPort < share.PortRangeStart || requestedPort > share.PortRangeEnd {
			return 0, fmt.Errorf("Port out of range")
		}
		if _, ok := used[requestedPort]; ok {
			return 0, fmt.Errorf("No available port")
		}
		return requestedPort, nil
	}

	for p := share.PortRangeStart; p <= share.PortRangeEnd; p++ {
		if _, ok := used[p]; ok {
			continue
		}
		return p, nil
	}

	return 0, fmt.Errorf("No available port")
}

func extractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	parts := strings.Split(authHeader, " ")
	if len(parts) == 2 && parts[0] == "Bearer" {
		return parts[1]
	}
	return ""
}
