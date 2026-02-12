package ws

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"go-backend/internal/auth"
	"go-backend/internal/security"
	"go-backend/internal/store/sqlite"
)

type encryptedMessage struct {
	Encrypted bool   `json:"encrypted"`
	Data      string `json:"data"`
	Timestamp int64  `json:"timestamp"`
}

type broadcastMessage struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
	Data string `json:"data"`
}

type connWrap struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

type nodeSession struct {
	nodeID int64
	secret string
	conn   *connWrap
}

type commandResponse struct {
	Type      string          `json:"type"`
	Success   bool            `json:"success"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data,omitempty"`
	RequestID string          `json:"requestId,omitempty"`
}

type pendingRequest struct {
	nodeID int64
	ch     chan CommandResult
}

const (
	wsPingPeriod = 15 * time.Second
	wsPongWait   = 45 * time.Second
	wsWriteWait  = 5 * time.Second
)

type CommandResult struct {
	Type    string                 `json:"type"`
	Success bool                   `json:"success"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

type Server struct {
	repo      *sqlite.Repository
	jwtSecret string
	upgrader  websocket.Upgrader

	mu      sync.RWMutex
	admins  map[*connWrap]struct{}
	nodes   map[int64]*nodeSession
	byConn  map[*websocket.Conn]*nodeSession
	pending map[string]pendingRequest
}

func NewServer(repo *sqlite.Repository, jwtSecret string) *Server {
	return &Server{
		repo:      repo,
		jwtSecret: jwtSecret,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		admins:  make(map[*connWrap]struct{}),
		nodes:   make(map[int64]*nodeSession),
		byConn:  make(map[*websocket.Conn]*nodeSession),
		pending: make(map[string]pendingRequest),
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	typeVal := query.Get("type")
	secret := query.Get("secret")

	if typeVal == "1" {
		node, err := s.repo.GetNodeBySecret(secret)
		if err != nil || node == nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		s.handleNode(w, r, node.ID, secret)
		return
	}

	if typeVal == "0" {
		if _, ok := auth.ValidateToken(secret, s.jwtSecret); !ok {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		s.handleAdmin(w, r)
		return
	}

	http.Error(w, "bad request", http.StatusBadRequest)
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	cw := &connWrap{conn: conn}
	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})
	done := make(chan struct{})
	go startKeepalive(cw, done)

	s.mu.Lock()
	s.admins[cw] = struct{}{}
	s.mu.Unlock()

	defer func() {
		close(done)
		s.mu.Lock()
		delete(s.admins, cw)
		s.mu.Unlock()
		_ = conn.Close()
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (s *Server) handleNode(w http.ResponseWriter, r *http.Request, nodeID int64, secret string) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	cw := &connWrap{conn: conn}
	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})
	done := make(chan struct{})
	go startKeepalive(cw, done)

	version := r.URL.Query().Get("version")
	httpVal := parseIntDefault(r.URL.Query().Get("http"), 0)
	tlsVal := parseIntDefault(r.URL.Query().Get("tls"), 0)
	socksVal := parseIntDefault(r.URL.Query().Get("socks"), 0)

	s.mu.Lock()
	if old, ok := s.nodes[nodeID]; ok {
		_ = old.conn.conn.Close()
		delete(s.byConn, old.conn.conn)
	}
	ns := &nodeSession{nodeID: nodeID, secret: secret, conn: cw}
	s.nodes[nodeID] = ns
	s.byConn[conn] = ns
	s.mu.Unlock()

	_ = s.repo.UpdateNodeOnline(nodeID, 1, version, httpVal, tlsVal, socksVal)
	s.broadcastStatus(nodeID, 1)

	defer func() {
		close(done)
		needOfflineBroadcast := false
		s.mu.Lock()
		current, ok := s.nodes[nodeID]
		if ok && current.conn.conn == conn {
			delete(s.nodes, nodeID)
			needOfflineBroadcast = true
		}
		delete(s.byConn, conn)
		s.mu.Unlock()
		if needOfflineBroadcast {
			s.failPendingForNode(nodeID, "节点连接已断开")
			_ = s.repo.UpdateNodeStatus(nodeID, 0)
			s.broadcastStatus(nodeID, 0)
		}
		_ = conn.Close()
	}()

	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}

		msg := decryptIfNeeded(payload, secret)
		s.tryResolvePending(nodeID, msg)

		var parsed struct {
			Type string `json:"type"`
		}
		if json.Unmarshal([]byte(msg), &parsed) == nil && parsed.Type == "UpgradeProgress" {
			s.broadcastTyped(nodeID, "upgrade_progress", msg)
		} else {
			s.broadcastInfo(nodeID, msg)
		}
	}
}

func (s *Server) SendCommand(nodeID int64, cmdType string, data interface{}, timeout time.Duration) (CommandResult, error) {
	if s == nil {
		return CommandResult{}, errors.New("server not initialized")
	}
	if strings.TrimSpace(cmdType) == "" {
		return CommandResult{}, errors.New("command type is empty")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	s.mu.RLock()
	ns, ok := s.nodes[nodeID]
	s.mu.RUnlock()
	if !ok || ns == nil || ns.conn == nil || ns.conn.conn == nil {
		return CommandResult{}, errors.New("节点不在线")
	}

	requestID := fmt.Sprintf("%d_%d", nodeID, time.Now().UnixNano())
	ch := make(chan CommandResult, 1)

	s.mu.Lock()
	s.pending[requestID] = pendingRequest{nodeID: nodeID, ch: ch}
	s.mu.Unlock()

	cleanup := func() {
		s.mu.Lock()
		if p, exists := s.pending[requestID]; exists {
			delete(s.pending, requestID)
			close(p.ch)
		}
		s.mu.Unlock()
	}

	cmdPayload := map[string]interface{}{
		"type":      cmdType,
		"data":      data,
		"requestId": requestID,
	}
	rawCmd, err := json.Marshal(cmdPayload)
	if err != nil {
		cleanup()
		return CommandResult{}, err
	}

	messageData := rawCmd
	if strings.TrimSpace(ns.secret) != "" {
		crypto, err := security.NewAESCrypto(ns.secret)
		if err != nil {
			cleanup()
			return CommandResult{}, err
		}
		encrypted, err := crypto.Encrypt(rawCmd)
		if err != nil {
			cleanup()
			return CommandResult{}, err
		}
		wrapper := map[string]interface{}{
			"encrypted": true,
			"data":      encrypted,
			"timestamp": time.Now().UnixMilli(),
		}
		messageData, err = json.Marshal(wrapper)
		if err != nil {
			cleanup()
			return CommandResult{}, err
		}
	}

	ns.conn.mu.Lock()
	_ = ns.conn.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
	err = ns.conn.conn.WriteMessage(websocket.TextMessage, messageData)
	_ = ns.conn.conn.SetWriteDeadline(time.Time{})
	ns.conn.mu.Unlock()
	if err != nil {
		cleanup()
		return CommandResult{}, err
	}

	select {
	case result, ok := <-ch:
		if !ok {
			return CommandResult{}, errors.New("命令通道已关闭")
		}
		if !result.Success {
			if strings.TrimSpace(result.Message) == "" {
				result.Message = "命令执行失败"
			}
			return result, errors.New(result.Message)
		}
		return result, nil
	case <-time.After(timeout):
		cleanup()
		return CommandResult{}, errors.New("等待节点响应超时")
	}
}

func (s *Server) tryResolvePending(nodeID int64, message string) {
	if s == nil || strings.TrimSpace(message) == "" {
		return
	}

	var resp commandResponse
	if err := json.Unmarshal([]byte(message), &resp); err != nil {
		return
	}
	if strings.TrimSpace(resp.RequestID) == "" {
		return
	}

	s.mu.Lock()
	p, ok := s.pending[resp.RequestID]
	if ok {
		delete(s.pending, resp.RequestID)
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	if p.nodeID != nodeID {
		select {
		case p.ch <- CommandResult{Type: resp.Type, Success: false, Message: "节点响应与请求不匹配"}:
		default:
		}
		close(p.ch)
		return
	}

	result := CommandResult{
		Type:    resp.Type,
		Success: resp.Success,
		Message: resp.Message,
	}
	if len(resp.Data) > 0 {
		var data map[string]interface{}
		if err := json.Unmarshal(resp.Data, &data); err == nil {
			result.Data = data
		}
	}

	select {
	case p.ch <- result:
	default:
	}
	close(p.ch)
}

func (s *Server) failPendingForNode(nodeID int64, message string) {
	if s == nil {
		return
	}

	type pair struct {
		id string
		pr pendingRequest
	}
	items := make([]pair, 0)

	s.mu.Lock()
	for id, pr := range s.pending {
		if pr.nodeID != nodeID {
			continue
		}
		items = append(items, pair{id: id, pr: pr})
		delete(s.pending, id)
	}
	s.mu.Unlock()

	for _, item := range items {
		select {
		case item.pr.ch <- CommandResult{Success: false, Message: message}:
		default:
		}
		close(item.pr.ch)
	}
}

func (s *Server) broadcastStatus(nodeID int64, status int) {
	payload := map[string]interface{}{
		"id":   strconv.FormatInt(nodeID, 10),
		"type": "status",
		"data": status,
	}
	raw, _ := json.Marshal(payload)
	s.broadcastToAdmins(string(raw))
}

func (s *Server) broadcastInfo(nodeID int64, data string) {
	payload := broadcastMessage{ID: nodeID, Type: "info", Data: data}
	raw, _ := json.Marshal(payload)
	s.broadcastToAdmins(string(raw))
}

func (s *Server) broadcastTyped(nodeID int64, msgType string, data string) {
	payload := broadcastMessage{ID: nodeID, Type: msgType, Data: data}
	raw, _ := json.Marshal(payload)
	s.broadcastToAdmins(string(raw))
}

func (s *Server) broadcastToAdmins(message string) {
	s.mu.RLock()
	admins := make([]*connWrap, 0, len(s.admins))
	for c := range s.admins {
		admins = append(admins, c)
	}
	s.mu.RUnlock()

	for _, c := range admins {
		c.mu.Lock()
		_ = c.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
		err := c.conn.WriteMessage(websocket.TextMessage, []byte(message))
		_ = c.conn.SetWriteDeadline(time.Time{})
		c.mu.Unlock()
		if err != nil {
			log.Printf("websocket broadcast failed: %v", err)
		}
	}
}

func decryptIfNeeded(payload []byte, secret string) string {
	text := string(payload)
	var wrap encryptedMessage
	if err := json.Unmarshal(payload, &wrap); err != nil || !wrap.Encrypted || strings.TrimSpace(wrap.Data) == "" {
		return text
	}

	crypto, err := security.NewAESCrypto(secret)
	if err != nil {
		return text
	}
	plain, err := crypto.Decrypt(wrap.Data)
	if err != nil {
		return text
	}
	return string(plain)
}

func parseIntDefault(v string, fallback int) int {
	x, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return x
}

func startKeepalive(cw *connWrap, done <-chan struct{}) {
	if cw == nil || cw.conn == nil {
		return
	}
	ticker := time.NewTicker(wsPingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			cw.mu.Lock()
			_ = cw.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			err := cw.conn.WriteMessage(websocket.PingMessage, nil)
			_ = cw.conn.SetWriteDeadline(time.Time{})
			cw.mu.Unlock()
			if err != nil {
				_ = cw.conn.Close()
				return
			}
		}
	}
}
