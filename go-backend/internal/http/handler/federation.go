package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
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
	AllowedIPs     string `json:"allowedIps"`
}

type deletePeerShareRequest struct {
	ID int64 `json:"id"`
}

type resetPeerShareFlowRequest struct {
	ID int64 `json:"id"`
}

type updatePeerShareRequest struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	MaxBandwidth   int64  `json:"maxBandwidth"`
	ExpiryTime     int64  `json:"expiryTime"`
	PortRangeStart int    `json:"portRangeStart"`
	PortRangeEnd   int    `json:"portRangeEnd"`
	AllowedDomains string `json:"allowedDomains"`
	AllowedIPs     string `json:"allowedIps"`
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

type peerShareUsedPort struct {
	RuntimeID   int64  `json:"runtimeId"`
	Port        int    `json:"port"`
	Role        string `json:"role"`
	Protocol    string `json:"protocol"`
	ResourceKey string `json:"resourceKey"`
	Applied     int    `json:"applied"`
	UpdatedTime int64  `json:"updatedTime"`
}

type peerShareListItem struct {
	sqlite.PeerShare
	UsedPorts        []int               `json:"usedPorts"`
	UsedPortDetails  []peerShareUsedPort `json:"usedPortDetails"`
	ActiveRuntimeNum int                 `json:"activeRuntimeNum"`
}

type remoteUsageBindingItem struct {
	BindingID       int64  `json:"bindingId"`
	TunnelID        int64  `json:"tunnelId"`
	TunnelName      string `json:"tunnelName"`
	ChainType       int    `json:"chainType"`
	HopInx          int    `json:"hopInx"`
	AllocatedPort   int    `json:"allocatedPort"`
	ResourceKey     string `json:"resourceKey"`
	RemoteBindingID string `json:"remoteBindingId"`
	UpdatedTime     int64  `json:"updatedTime"`
}

type remoteUsageNodeItem struct {
	NodeID           int64                    `json:"nodeId"`
	NodeName         string                   `json:"nodeName"`
	RemoteURL        string                   `json:"remoteUrl"`
	ShareID          int64                    `json:"shareId"`
	PortRangeStart   int                      `json:"portRangeStart"`
	PortRangeEnd     int                      `json:"portRangeEnd"`
	MaxBandwidth     int64                    `json:"maxBandwidth"`
	CurrentFlow      int64                    `json:"currentFlow"`
	ExpiryTime       int64                    `json:"expiryTime"`
	UsedPorts        []int                    `json:"usedPorts"`
	Bindings         []remoteUsageBindingItem `json:"bindings"`
	ActiveBindingNum int                      `json:"activeBindingNum"`
	SyncError        string                   `json:"syncError,omitempty"`
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

	items := make([]peerShareListItem, 0, len(shares))
	for i := range shares {
		share := shares[i]
		runtimes, err := h.repo.ListActivePeerShareRuntimesByShareID(share.ID)
		if err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}

		usedSet := make(map[int]struct{}, len(runtimes))
		details := make([]peerShareUsedPort, 0, len(runtimes))
		for _, runtime := range runtimes {
			if runtime.Port > 0 {
				usedSet[runtime.Port] = struct{}{}
			}
			details = append(details, peerShareUsedPort{
				RuntimeID:   runtime.ID,
				Port:        runtime.Port,
				Role:        runtime.Role,
				Protocol:    runtime.Protocol,
				ResourceKey: runtime.ResourceKey,
				Applied:     runtime.Applied,
				UpdatedTime: runtime.UpdatedTime,
			})
		}

		usedPorts := make([]int, 0, len(usedSet))
		for port := range usedSet {
			usedPorts = append(usedPorts, port)
		}
		sort.Ints(usedPorts)

		sort.Slice(details, func(i, j int) bool {
			if details[i].Port == details[j].Port {
				return details[i].RuntimeID < details[j].RuntimeID
			}
			return details[i].Port < details[j].Port
		})

		items = append(items, peerShareListItem{
			PeerShare:        share,
			UsedPorts:        usedPorts,
			UsedPortDetails:  details,
			ActiveRuntimeNum: len(details),
		})
	}

	response.WriteJSON(w, response.OK(items))
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

	allowedIPs, err := normalizePeerShareAllowedIPs(req.AllowedIPs)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
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
	if node.IsRemote == 1 {
		response.WriteJSON(w, response.ErrDefault("Only local nodes can be shared"))
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
		AllowedIPs:     allowedIPs,
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

	h.cleanupPeerShareRuntimes(req.ID)

	if err := h.repo.DeletePeerShare(req.ID); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) federationShareResetFlow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("Invalid method"))
		return
	}

	var req resetPeerShareFlowRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("Invalid JSON"))
		return
	}
	if req.ID <= 0 {
		response.WriteJSON(w, response.ErrDefault("Share ID is required"))
		return
	}

	share, err := h.repo.GetPeerShare(req.ID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if share == nil {
		response.WriteJSON(w, response.ErrDefault("Share not found"))
		return
	}

	if err := h.repo.ResetPeerShareCurrentFlow(req.ID, time.Now().UnixMilli()); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) federationShareUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("Invalid method"))
		return
	}

	var req updatePeerShareRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("Invalid JSON"))
		return
	}
	if req.ID <= 0 {
		response.WriteJSON(w, response.ErrDefault("Share ID is required"))
		return
	}

	share, err := h.repo.GetPeerShare(req.ID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if share == nil {
		response.WriteJSON(w, response.ErrDefault("Share not found"))
		return
	}

	if req.Name == "" {
		response.WriteJSON(w, response.ErrDefault("Name is required"))
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

	allowedIPs, err := normalizePeerShareAllowedIPs(req.AllowedIPs)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}

	share.Name = req.Name
	share.MaxBandwidth = req.MaxBandwidth
	share.ExpiryTime = req.ExpiryTime
	share.PortRangeStart = req.PortRangeStart
	share.PortRangeEnd = req.PortRangeEnd
	share.AllowedDomains = req.AllowedDomains
	share.AllowedIPs = allowedIPs
	share.UpdatedTime = time.Now().UnixMilli()

	if err := h.repo.UpdatePeerShare(share); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) federationRemoteUsageList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("Invalid method"))
		return
	}

	rows, err := h.repo.DB().Query(`
		SELECT id, name, remote_url, remote_token, remote_config
		FROM node
		WHERE is_remote = 1
		ORDER BY id DESC
	`)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	defer rows.Close()

	fc := client.NewFederationClient()
	localDomain := h.federationLocalDomain()

	items := make([]remoteUsageNodeItem, 0)
	for rows.Next() {
		var (
			nodeID       int64
			nodeName     string
			remoteURL    sql.NullString
			remoteToken  sql.NullString
			remoteConfig sql.NullString
		)
		if err := rows.Scan(&nodeID, &nodeName, &remoteURL, &remoteToken, &remoteConfig); err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}

		shareID, maxBandwidth, currentFlow, expiryTime, portRangeStart, portRangeEnd := parseRemoteShareUsageConfig(remoteConfig.String)

		var syncError string
		url := strings.TrimSpace(remoteURL.String)
		token := strings.TrimSpace(remoteToken.String)
		if url != "" && token != "" {
			info, connectErr := fc.Connect(url, token, localDomain)
			if connectErr != nil {
				syncError = connectErr.Error()
			} else if info != nil {
				shareID = info.ShareID
				maxBandwidth = info.MaxBandwidth
				currentFlow = info.CurrentFlow
				expiryTime = info.ExpiryTime
				portRangeStart = info.PortRangeStart
				portRangeEnd = info.PortRangeEnd

				configData, _ := json.Marshal(map[string]interface{}{
					"shareId":        info.ShareID,
					"maxBandwidth":   info.MaxBandwidth,
					"currentFlow":    info.CurrentFlow,
					"expiryTime":     info.ExpiryTime,
					"portRangeStart": info.PortRangeStart,
					"portRangeEnd":   info.PortRangeEnd,
				})
				_, _ = h.repo.DB().Exec(`UPDATE node SET remote_config = ? WHERE id = ?`, string(configData), nodeID)
			}
		}

		bindingRows, err := h.repo.DB().Query(`
			SELECT fb.id, fb.tunnel_id, COALESCE(t.name, ''), fb.chain_type, fb.hop_inx, fb.allocated_port, fb.resource_key, fb.remote_binding_id, fb.updated_time
			FROM federation_tunnel_binding fb
			LEFT JOIN tunnel t ON t.id = fb.tunnel_id
			WHERE fb.node_id = ? AND fb.status = 1
			ORDER BY fb.allocated_port ASC, fb.id ASC
		`, nodeID)
		if err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}

		usedSet := make(map[int]struct{})
		bindings := make([]remoteUsageBindingItem, 0)
		for bindingRows.Next() {
			var item remoteUsageBindingItem
			if err := bindingRows.Scan(&item.BindingID, &item.TunnelID, &item.TunnelName, &item.ChainType, &item.HopInx, &item.AllocatedPort, &item.ResourceKey, &item.RemoteBindingID, &item.UpdatedTime); err != nil {
				_ = bindingRows.Close()
				response.WriteJSON(w, response.Err(-2, err.Error()))
				return
			}
			bindings = append(bindings, item)
			if item.AllocatedPort > 0 {
				usedSet[item.AllocatedPort] = struct{}{}
			}
		}
		if err := bindingRows.Err(); err != nil {
			_ = bindingRows.Close()
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
		_ = bindingRows.Close()

		usedPorts := make([]int, 0, len(usedSet))
		for port := range usedSet {
			usedPorts = append(usedPorts, port)
		}
		sort.Ints(usedPorts)

		items = append(items, remoteUsageNodeItem{
			NodeID:           nodeID,
			NodeName:         nodeName,
			RemoteURL:        url,
			ShareID:          shareID,
			PortRangeStart:   portRangeStart,
			PortRangeEnd:     portRangeEnd,
			MaxBandwidth:     maxBandwidth,
			CurrentFlow:      currentFlow,
			ExpiryTime:       expiryTime,
			UsedPorts:        usedPorts,
			Bindings:         bindings,
			ActiveBindingNum: len(bindings),
			SyncError:        syncError,
		})
	}
	if err := rows.Err(); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OK(items))
}

func parseRemoteShareUsageConfig(raw string) (int64, int64, int64, int64, int, int) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0, 0, 0, 0, 0
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return 0, 0, 0, 0, 0, 0
	}

	shareID := asInt64(cfg["shareId"], 0)
	maxBandwidth := asInt64(cfg["maxBandwidth"], 0)
	currentFlow := asInt64(cfg["currentFlow"], 0)
	expiryTime := asInt64(cfg["expiryTime"], 0)
	portRangeStart := int(asInt64(cfg["portRangeStart"], 0))
	portRangeEnd := int(asInt64(cfg["portRangeEnd"], 0))
	return shareID, maxBandwidth, currentFlow, expiryTime, portRangeStart, portRangeEnd
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
		"currentFlow":    info.CurrentFlow,
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

		if strings.TrimSpace(share.AllowedIPs) != "" {
			clientIP := resolvePeerClientIP(r)
			if clientIP == nil {
				response.WriteJSON(w, response.Err(403, "Unable to determine client IP"))
				return
			}
			if !isPeerIPAllowed(clientIP, share.AllowedIPs) {
				response.WriteJSON(w, response.Err(403, "IP not allowed"))
				return
			}
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
		"currentFlow":    share.CurrentFlow,
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
	if isPeerShareFlowExceeded(share) {
		response.WriteJSON(w, response.Err(403, "Share traffic limit exceeded"))
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
	tunnelID, err := tx.ExecReturningID(`INSERT INTO tunnel (name, type, protocol, flow, created_time, updated_time, status, in_ip) VALUES (?, ?, ?, 0, ?, ?, 1, ?)`,
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

	_, err = tx.Exec(`INSERT INTO chain_tunnel (tunnel_id, chain_type, node_id, port, strategy, inx, protocol) VALUES (?, '1', ?, ?, 'fifo', 0, ?)`,
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
	if isPeerShareFlowExceeded(share) {
		response.WriteJSON(w, response.Err(403, "Share traffic limit exceeded"))
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
	if isPeerShareFlowExceeded(share) {
		response.WriteJSON(w, response.Err(403, "Share traffic limit exceeded"))
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

func isPeerShareFlowExceeded(share *sqlite.PeerShare) bool {
	if share == nil {
		return false
	}
	if share.MaxBandwidth <= 0 {
		return false
	}
	return share.CurrentFlow >= share.MaxBandwidth
}

func normalizePeerShareAllowedIPs(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}

	parts := strings.Split(raw, ",")
	normalized := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))

	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}

		if strings.Contains(item, "/") {
			_, network, err := net.ParseCIDR(item)
			if err != nil {
				return "", fmt.Errorf("Invalid allowed IP or CIDR: %s", item)
			}
			item = network.String()
		} else {
			ip := parseIPLiteral(item)
			if ip == nil {
				return "", fmt.Errorf("Invalid allowed IP or CIDR: %s", item)
			}
			item = ip.String()
		}

		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		normalized = append(normalized, item)
	}

	return strings.Join(normalized, ","), nil
}

func resolvePeerClientIP(r *http.Request) net.IP {
	if r == nil {
		return nil
	}

	remoteIP := parseIPLiteral(r.RemoteAddr)
	if isTrustedProxyIP(remoteIP) {
		if ip := parseForwardedFor(r.Header.Get("X-Forwarded-For")); ip != nil {
			return ip
		}
		if ip := parseIPLiteral(r.Header.Get("X-Real-IP")); ip != nil {
			return ip
		}
	}

	return remoteIP
}

func parseForwardedFor(raw string) net.IP {
	for _, part := range strings.Split(raw, ",") {
		if ip := parseIPLiteral(part); ip != nil {
			return ip
		}
	}
	return nil
}

func parseIPLiteral(raw string) net.IP {
	value := strings.Trim(strings.TrimSpace(raw), "\"")
	if value == "" {
		return nil
	}

	if ip := net.ParseIP(value); ip != nil {
		return ip
	}

	host, _, err := net.SplitHostPort(value)
	if err != nil {
		return nil
	}

	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" {
		return nil
	}
	return net.ParseIP(host)
}

func isTrustedProxyIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func isPeerIPAllowed(clientIP net.IP, whitelist string) bool {
	if clientIP == nil {
		return false
	}

	for _, part := range strings.Split(whitelist, ",") {
		entry := strings.TrimSpace(part)
		if entry == "" {
			continue
		}

		if strings.Contains(entry, "/") {
			_, network, err := net.ParseCIDR(entry)
			if err != nil {
				continue
			}
			if network.Contains(clientIP) {
				return true
			}
			continue
		}

		allowedIP := parseIPLiteral(entry)
		if allowedIP != nil && allowedIP.Equal(clientIP) {
			return true
		}
	}

	return false
}

func (h *Handler) syncRemoteNodeStatuses(items []map[string]interface{}) {
	type remoteEntry struct {
		index       int
		remoteURL   string
		remoteToken string
	}

	var remotes []remoteEntry
	for i, item := range items {
		isRemote, _ := item["isRemote"].(int)
		if isRemote != 1 {
			continue
		}
		url, _ := item["remoteUrl"].(string)
		token, _ := item["remoteToken"].(string)
		url = strings.TrimSpace(url)
		token = strings.TrimSpace(token)
		if url == "" || token == "" {
			continue
		}
		remotes = append(remotes, remoteEntry{index: i, remoteURL: url, remoteToken: token})
	}
	if len(remotes) == 0 {
		return
	}

	localDomain := h.federationLocalDomain()
	fc := client.NewFederationClientWithTimeout(5 * time.Second)

	type syncResult struct {
		index     int
		status    int
		syncError string
	}

	results := make([]syncResult, len(remotes))
	var wg sync.WaitGroup
	for i, entry := range remotes {
		wg.Add(1)
		go func(idx int, e remoteEntry) {
			defer wg.Done()
			info, err := fc.Connect(e.remoteURL, e.remoteToken, localDomain)
			if err != nil {
				errMsg := err.Error()
				if strings.Contains(errMsg, "401") || strings.Contains(errMsg, "Invalid token") || strings.Contains(errMsg, "Unauthorized") {
					results[idx] = syncResult{index: e.index, status: 0, syncError: "provider_share_deleted"}
				} else if strings.Contains(errMsg, "403") || strings.Contains(errMsg, "Share is disabled") {
					results[idx] = syncResult{index: e.index, status: 0, syncError: "provider_share_disabled"}
				} else if strings.Contains(errMsg, "Share expired") {
					results[idx] = syncResult{index: e.index, status: 0, syncError: "provider_share_expired"}
				} else {
					results[idx] = syncResult{index: e.index, status: 0, syncError: errMsg}
				}
			} else {
				results[idx] = syncResult{index: e.index, status: info.Status, syncError: ""}
			}
		}(i, entry)
	}
	wg.Wait()

	for _, r := range results {
		items[r.index]["status"] = r.status
		if r.syncError != "" {
			items[r.index]["syncError"] = r.syncError
		}
	}
}

func (h *Handler) cleanupPeerShareRuntimes(shareID int64) {
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
