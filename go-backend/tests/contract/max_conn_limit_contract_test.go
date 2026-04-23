package contract_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"go-backend/internal/auth"
	"go-backend/internal/http/response"
	"go-backend/internal/security"
)

func TestMaxConnLimit(t *testing.T) {
	secret := "contract-jwt-secret"
	router, r := setupContractRouter(t, secret)
	server := httptest.NewServer(router)
	defer server.Close()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	now := time.Now().UnixMilli()
	if err := r.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "max-conn-tunnel", 1.0, 1, "tls", 99999, now, now, 1, nil, 0).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	var tunnelID int64
	if err := r.DB().Raw("SELECT id FROM tunnel WHERE name = ?", "max-conn-tunnel").Scan(&tunnelID).Error; err != nil {
		t.Fatalf("get tunnel ID: %v", err)
	}

	if err := r.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "max-conn-node", "max-conn-secret", "10.20.0.1", "10.20.0.1", "", "32000-32010", "", "v1", 1, 1, 1, now, now, 1, "[::]", "[::]", 0).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	var nodeID int64
	if err := r.DB().Raw("SELECT id FROM node WHERE name = ?", "max-conn-node").Scan(&nodeID).Error; err != nil {
		t.Fatalf("get node ID: %v", err)
	}

	if err := r.DB().Exec(`
	        INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
	        VALUES(?, 1, ?, 32001, 'round', 1, 'tls')
	`, tunnelID, nodeID).Error; err != nil {
	        t.Fatalf("insert chain_tunnel: %v", err)
	}

	if err := r.DB().Exec(`
	        INSERT INTO user_tunnel(user_id, tunnel_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
	        VALUES(1, ?, 10, 99999, 0, 0, 1, ?, 1)
	`, tunnelID, now + 365*24*3600*1000).Error; err != nil {	        t.Fatalf("insert user_tunnel: %v", err)
	}

	var commandMu sync.Mutex
	receivedCommands := make([]string, 0)
	var addCLimitersData json.RawMessage
	var updateCLimitersData json.RawMessage

	stopNode := startMockSessionForMaxConn(t, server.URL, "max-conn-secret", func(cmdType string, data json.RawMessage) (bool, string) {
		commandMu.Lock()
		defer commandMu.Unlock()
		receivedCommands = append(receivedCommands, cmdType)

		if cmdType == "AddCLimiters" {
			addCLimitersData = append([]byte(nil), data...)
			return true, "already exists"
		}
		if cmdType == "UpdateCLimiters" {
			updateCLimitersData = append([]byte(nil), data...)
		}

		return false, ""
	})
	defer stopNode()

	waitNodeStatus(t, r, nodeID, 1)

	payload := map[string]interface{}{
		"name":       "max-conn-forward",
		"tunnelId":   tunnelID,
		"remoteAddr": "1.1.1.1:443",
		"strategy":   "fifo",
		"maxConn":    42,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(body))
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected create success, got code=%d msg=%s", out.Code, out.Msg)
	}

	var forwardID int64
	if err := r.DB().Raw("SELECT id FROM forward WHERE name = ?", "max-conn-forward").Scan(&forwardID).Error; err != nil {
		t.Fatalf("get forward ID: %v", err)
	}

	commandMu.Lock()
	defer commandMu.Unlock()

	hasAdd := false
	hasUpdate := false
	for _, cmd := range receivedCommands {
		if cmd == "AddCLimiters" {
			hasAdd = true
		}
		if cmd == "UpdateCLimiters" {
			hasUpdate = true
		}
	}

	if !hasAdd {
		t.Fatalf("expected AddCLimiters to be sent, but it was not. Received: %v", receivedCommands)
	}
	if !hasUpdate {
		t.Fatalf("expected UpdateCLimiters to be sent after AddCLimiters failed with already exists. Received: %v", receivedCommands)
	}

	expectedName := fmt.Sprintf("rule_conn_limit_%d", forwardID)

	// verify payload for AddCLimiters
	var addData map[string]interface{}
	if err := json.Unmarshal(addCLimitersData, &addData); err != nil {
		t.Fatalf("unmarshal AddCLimiters data: %v", err)
	}
	if addData["name"] != expectedName {
		t.Fatalf("expected limiter name %s, got %v", expectedName, addData["name"])
	}
	if limits, ok := addData["limits"].([]interface{}); ok {
		if len(limits) != 1 || limits[0] != "$ 42" {
			t.Fatalf("expected limits to contain '$ 42', got %v", limits)
		}
	} else {
		t.Fatalf("invalid limits type in AddCLimiters data: %v", addData)
	}

	// verify payload for UpdateCLimiters
	var updateData map[string]interface{}
	if err := json.Unmarshal(updateCLimitersData, &updateData); err != nil {
		t.Fatalf("unmarshal UpdateCLimiters data: %v", err)
	}
	if updateData["limiter"] != expectedName {
		t.Fatalf("expected update limiter name %s, got %v", expectedName, updateData["limiter"])
	}
	nestedData, ok := updateData["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested 'data' in UpdateCLimiters, got %v", updateData)
	}
	if nestedData["name"] != expectedName {
		t.Fatalf("expected nested name %s, got %v", expectedName, nestedData["name"])
	}
	if nestedLimits, ok := nestedData["limits"].([]interface{}); ok {
		if len(nestedLimits) != 1 || nestedLimits[0] != "$ 42" {
			t.Fatalf("expected nested limits to contain '$ 42', got %v", nestedLimits)
		}
	} else {
		t.Fatalf("invalid limits type in UpdateCLimiters nested data: %v", nestedData)
	}
}

func startMockSessionForMaxConn(t *testing.T, baseURL string, nodeSecret string, onCommand func(cmdType string, data json.RawMessage) (bool, string)) func() {
	t.Helper()

	u, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse provider url: %v", err)
	}
	if strings.EqualFold(u.Scheme, "https") {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/system-info"
	q := u.Query()
	q.Set("type", "1")
	q.Set("secret", nodeSecret)
	q.Set("version", "v1")
	q.Set("http", "1")
	q.Set("tls", "1")
	q.Set("socks", "1")
	u.RawQuery = q.Encode()

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial mock node websocket: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			_, raw, readErr := conn.ReadMessage()
			if readErr != nil {
				return
			}

			plain := raw
			var wrap struct {
				Encrypted bool   `json:"encrypted"`
				Data      string `json:"data"`
			}
			if err := json.Unmarshal(raw, &wrap); err == nil && wrap.Encrypted && strings.TrimSpace(wrap.Data) != "" {
				crypto, cryptoErr := security.NewAESCrypto(nodeSecret)
				if cryptoErr == nil {
					if dec, decErr := crypto.Decrypt(wrap.Data); decErr == nil {
						plain = []byte(dec)
					}
				}
			}

			var cmd struct {
				Type      string          `json:"type"`
				RequestID string          `json:"requestId"`
				Data      json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(plain, &cmd); err != nil {
				continue
			}
			if strings.TrimSpace(cmd.RequestID) == "" {
				continue
			}

			shouldFail := false
			failMsg := ""
			if onCommand != nil {
				shouldFail, failMsg = onCommand(strings.TrimSpace(cmd.Type), cmd.Data)
			}

			respType := fmt.Sprintf("%sResponse", cmd.Type)
			respPayload := map[string]interface{}{
				"type":      respType,
				"success":   !shouldFail,
				"message":   "OK",
				"requestId": cmd.RequestID,
			}
			if shouldFail {
				if strings.TrimSpace(failMsg) == "" {
					failMsg = "mock command failed"
				}
				respPayload["message"] = failMsg
			}

			respBytes, err := json.Marshal(respPayload)
			if err != nil {
				continue
			}
			_ = conn.WriteMessage(websocket.TextMessage, respBytes)
		}
	}()

	var stopOnce sync.Once
	return func() {
		stopOnce.Do(func() {
			_ = conn.Close()
			wg.Wait()
		})
	}
}
