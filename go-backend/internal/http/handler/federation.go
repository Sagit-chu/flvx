package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go-backend/internal/http/client"
	"go-backend/internal/http/response"
)

type federationTunnelRequest struct {
	Protocol   string `json:"protocol"`
	RemotePort int    `json:"remotePort"`
	Target     string `json:"target"`
}

type nodeImportRequest struct {
	RemoteURL string `json:"remoteUrl"`
	Token     string `json:"token"`
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

	fc := client.NewFederationClient()
	info, err := fc.Connect(req.RemoteURL, req.Token)
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
		"0", // port range not applicable for remote
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

func extractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	parts := strings.Split(authHeader, " ")
	if len(parts) == 2 && parts[0] == "Bearer" {
		return parts[1]
	}
	return ""
}
