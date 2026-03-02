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

	"go-backend/internal/auth"
	"go-backend/internal/http/response"
	"go-backend/internal/security"

	"github.com/gorilla/websocket"
)

func TestUDPTransparentModeE2E(t *testing.T) {
	secret := "udp-e2e-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(2, 'udp_user', '3c85cdebade1c51cf64ca9f3c09d182d', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES('udp-e2e-tunnel', 1.0, 1, 'tls', 99999, ?, ?, 1, NULL, 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, repo, "udp-e2e-tunnel")

	if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES('udp-e2e-node', 'udp-e2e-secret', '10.100.0.1', '10.100.0.1', '', '60000-60010', '', 'v1', 1, 1, 1, ?, ?, 1, '[::]', '[::]', 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	nodeID := mustLastInsertID(t, repo, "udp-e2e-node")

	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 60001, 'round', 1, 'tls')
	`, tunnelID, nodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	server := httptest.NewServer(router)
	defer server.Close()

	t.Run("create forward with transparent UDP mode", func(t *testing.T) {
		var (
			mu       sync.Mutex
			commands []map[string]interface{}
		)
		stopNode := startMockNodeSessionWithCommandCapture(t, server.URL, "udp-e2e-secret", func(cmd map[string]interface{}) {
			mu.Lock()
			commands = append(commands, cmd)
			mu.Unlock()
		})
		defer stopNode()

		createPayload := map[string]interface{}{
			"name":       "udp-transparent-forward",
			"tunnelId":   tunnelID,
			"remoteAddr": "8.8.8.8:53",
			"strategy":   "fifo",
			"udpMode":    "transparent",
		}
		createBody, _ := json.Marshal(createPayload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCode(t, res, 0)

		forwardID := mustQueryInt64(t, repo, `SELECT id FROM forward WHERE name = ? ORDER BY id DESC LIMIT 1`, "udp-transparent-forward")

		storedUDPMode := mustQueryString(t, repo, `SELECT udp_mode FROM forward WHERE id = ?`, forwardID)
		if storedUDPMode != "transparent" {
			t.Fatalf("expected udp_mode=transparent, got %s", storedUDPMode)
		}

		mu.Lock()
		captured := append([]map[string]interface{}{}, commands...)
		mu.Unlock()
		if !waitForEitherCapturedCommand(&mu, &commands, 2*time.Second, "UpdateService", "AddService") {
			t.Fatalf("expected UpdateService or AddService command")
		}

		mu.Lock()
		captured = append([]map[string]interface{}{}, commands...)
		mu.Unlock()

		findCommand := func(cmdType string) map[string]interface{} {
			for _, cmd := range captured {
				if strings.EqualFold(capturedCommandType(cmd), cmdType) {
					return cmd
				}
			}
			return nil
		}

		probeCmd := findCommand("ProbeTProxyCapability")
		if probeCmd == nil {
			t.Fatalf("expected ProbeTProxyCapability command, got commands: %v", captured)
		}

		ensureCmd := findCommand("EnsureTProxyPolicy")
		if ensureCmd == nil {
			t.Fatalf("expected EnsureTProxyPolicy command, got commands: %v", captured)
		}
		data, ok := ensureCmd["data"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected data in EnsureTProxyPolicy command")
		}
		port := valueAsInt(data["port"])
		if port <= 0 {
			t.Fatalf("expected valid port in EnsureTProxyPolicy, got %v", data["port"])
		}

		addServiceCmd := findCommand("UpdateService")
		if addServiceCmd == nil {
			addServiceCmd = findCommand("AddService")
		}
		services, ok := addServiceCmd["services"].([]interface{})
		if !ok {
			if dataMap, dataOK := addServiceCmd["data"].(map[string]interface{}); dataOK {
				services, ok = dataMap["services"].([]interface{})
			} else if dataArr, dataOK := addServiceCmd["data"].([]interface{}); dataOK {
				services = dataArr
				ok = true
			}
		}
		if !ok || len(services) < 2 {
			t.Fatalf("expected at least 2 services (tcp+udp), got %v", services)
		}

		var udpService map[string]interface{}
		for _, svc := range services {
			svcMap, ok := svc.(map[string]interface{})
			if !ok {
				continue
			}
			name := valueAsString(svcMap["name"])
			if strings.HasSuffix(name, "_udp") {
				udpService = svcMap
				break
			}
		}
		if udpService == nil {
			t.Fatalf("expected UDP service in AddService")
		}

		handler, ok := udpService["handler"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected handler in UDP service")
		}
		handlerType := valueAsString(handler["type"])
		if handlerType != "redu" {
			t.Fatalf("expected UDP handler type 'redu' for transparent mode, got %s", handlerType)
		}

		listener, ok := udpService["listener"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected listener in UDP service")
		}
		listenerType := valueAsString(listener["type"])
		if listenerType != "redu" {
			t.Fatalf("expected UDP listener type 'redu' for transparent mode, got %s", listenerType)
		}
	})

	t.Run("update forward from normal to transparent UDP mode", func(t *testing.T) {
		var (
			mu       sync.Mutex
			commands []map[string]interface{}
		)
		stopNode := startMockNodeSessionWithCommandCapture(t, server.URL, "udp-e2e-secret", func(cmd map[string]interface{}) {
			mu.Lock()
			commands = append(commands, cmd)
			mu.Unlock()
		})
		defer stopNode()

		createPayload := map[string]interface{}{
			"name":       "udp-normal-forward",
			"tunnelId":   tunnelID,
			"remoteAddr": "1.1.1.1:53",
			"strategy":   "fifo",
			"udpMode":    "normal",
		}
		createBody, _ := json.Marshal(createPayload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCode(t, res, 0)

		forwardID := mustQueryInt64(t, repo, `SELECT id FROM forward WHERE name = ? ORDER BY id DESC LIMIT 1`, "udp-normal-forward")

		storedUDPMode := mustQueryString(t, repo, `SELECT udp_mode FROM forward WHERE id = ?`, forwardID)
		if storedUDPMode != "normal" {
			t.Fatalf("expected udp_mode=normal, got %s", storedUDPMode)
		}

		commands = nil

		updatePayload := map[string]interface{}{
			"id":      forwardID,
			"udpMode": "transparent",
		}
		updateBody, _ := json.Marshal(updatePayload)
		req = httptest.NewRequest(http.MethodPost, "/api/v1/forward/update", bytes.NewReader(updateBody))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res = httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCode(t, res, 0)

		updatedUDPMode := mustQueryString(t, repo, `SELECT udp_mode FROM forward WHERE id = ?`, forwardID)
		if updatedUDPMode != "transparent" {
			t.Fatalf("expected udp_mode=transparent after update, got %s", updatedUDPMode)
		}

		mu.Lock()
		captured := append([]map[string]interface{}{}, commands...)
		mu.Unlock()

		findCommand := func(cmdType string) map[string]interface{} {
			for _, cmd := range captured {
				if strings.EqualFold(capturedCommandType(cmd), cmdType) {
					return cmd
				}
			}
			return nil
		}

		ensureCmd := findCommand("EnsureTProxyPolicy")
		if ensureCmd == nil {
			t.Fatalf("expected EnsureTProxyPolicy command on mode switch")
		}

		if findCommand("UpdateService") == nil && findCommand("AddService") == nil {
			t.Fatalf("expected UpdateService or AddService command on mode switch")
		}
	})

	t.Run("delete forward with transparent UDP mode cleans up TPROXY", func(t *testing.T) {
		var (
			mu       sync.Mutex
			commands []map[string]interface{}
		)
		stopNode := startMockNodeSessionWithCommandCapture(t, server.URL, "udp-e2e-secret", func(cmd map[string]interface{}) {
			mu.Lock()
			commands = append(commands, cmd)
			mu.Unlock()
		})
		defer stopNode()

		createPayload := map[string]interface{}{
			"name":       "udp-delete-forward",
			"tunnelId":   tunnelID,
			"remoteAddr": "9.9.9.9:53",
			"strategy":   "fifo",
			"udpMode":    "transparent",
		}
		createBody, _ := json.Marshal(createPayload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCode(t, res, 0)

		forwardID := mustQueryInt64(t, repo, `SELECT id FROM forward WHERE name = ? ORDER BY id DESC LIMIT 1`, "udp-delete-forward")

		commands = nil

		deletePayload := map[string]interface{}{"id": forwardID}
		deleteBody, _ := json.Marshal(deletePayload)
		req = httptest.NewRequest(http.MethodPost, "/api/v1/forward/delete", bytes.NewReader(deleteBody))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res = httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCode(t, res, 0)

		mu.Lock()
		captured := append([]map[string]interface{}{}, commands...)
		mu.Unlock()

		findCommand := func(cmdType string) map[string]interface{} {
			for _, cmd := range captured {
				if strings.EqualFold(capturedCommandType(cmd), cmdType) {
					return cmd
				}
			}
			return nil
		}

		deleteTProxyCmd := findCommand("DeleteTProxyPolicy")
		if deleteTProxyCmd == nil {
			t.Fatalf("expected DeleteTProxyPolicy command on forward delete")
		}

		deleteServiceCmd := findCommand("DeleteService")
		if deleteServiceCmd == nil {
			t.Fatalf("expected DeleteService command on forward delete")
		}
	})

	t.Run("pause and resume transparent UDP forward", func(t *testing.T) {
		var (
			mu       sync.Mutex
			commands []map[string]interface{}
		)
		stopNode := startMockNodeSessionWithCommandCapture(t, server.URL, "udp-e2e-secret", func(cmd map[string]interface{}) {
			mu.Lock()
			commands = append(commands, cmd)
			mu.Unlock()
		})
		defer stopNode()

		createPayload := map[string]interface{}{
			"name":       "udp-pause-forward",
			"tunnelId":   tunnelID,
			"remoteAddr": "208.67.222.222:53",
			"strategy":   "fifo",
			"udpMode":    "transparent",
		}
		createBody, _ := json.Marshal(createPayload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCode(t, res, 0)

		forwardID := mustQueryInt64(t, repo, `SELECT id FROM forward WHERE name = ? ORDER BY id DESC LIMIT 1`, "udp-pause-forward")

		commands = nil

		pausePayload := map[string]interface{}{"id": forwardID}
		pauseBody, _ := json.Marshal(pausePayload)
		req = httptest.NewRequest(http.MethodPost, "/api/v1/forward/pause", bytes.NewReader(pauseBody))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res = httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCode(t, res, 0)

		pausedStatus := mustQueryInt(t, repo, `SELECT status FROM forward WHERE id = ?`, forwardID)
		if pausedStatus != 0 {
			t.Fatalf("expected status=0 after pause, got %d", pausedStatus)
		}

		mu.Lock()
		pauseCommands := append([]map[string]interface{}{}, commands...)
		mu.Unlock()

		findCommandType := func(cmds []map[string]interface{}, cmdType string) bool {
			for _, cmd := range cmds {
				if strings.EqualFold(capturedCommandType(cmd), cmdType) {
					return true
				}
			}
			return false
		}

		if !findCommandType(pauseCommands, "PauseService") {
			t.Fatalf("expected PauseService command on pause")
		}

		commands = nil

		resumePayload := map[string]interface{}{"id": forwardID}
		resumeBody, _ := json.Marshal(resumePayload)
		req = httptest.NewRequest(http.MethodPost, "/api/v1/forward/resume", bytes.NewReader(resumeBody))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res = httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCode(t, res, 0)

		resumedStatus := mustQueryInt(t, repo, `SELECT status FROM forward WHERE id = ?`, forwardID)
		if resumedStatus != 1 {
			t.Fatalf("expected status=1 after resume, got %d", resumedStatus)
		}

		mu.Lock()
		resumeCommands := append([]map[string]interface{}{}, commands...)
		mu.Unlock()

		if !findCommandType(resumeCommands, "ResumeService") {
			t.Fatalf("expected ResumeService command on resume")
		}
	})

	t.Run("diagnose transparent UDP forward returns tproxy status", func(t *testing.T) {
		var (
			mu       sync.Mutex
			commands []map[string]interface{}
		)
		stopNode := startMockNodeSessionWithCommandCapture(t, server.URL, "udp-e2e-secret", func(cmd map[string]interface{}) {
			mu.Lock()
			commands = append(commands, cmd)
			mu.Unlock()
		})
		defer stopNode()

		createPayload := map[string]interface{}{
			"name":       "udp-diag-forward",
			"tunnelId":   tunnelID,
			"remoteAddr": "8.8.4.4:53",
			"strategy":   "fifo",
			"udpMode":    "transparent",
		}
		createBody, _ := json.Marshal(createPayload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCode(t, res, 0)

		forwardID := mustQueryInt64(t, repo, `SELECT id FROM forward WHERE name = ? ORDER BY id DESC LIMIT 1`, "udp-diag-forward")

		diagPayload := map[string]interface{}{"forwardId": forwardID}
		diagBody, _ := json.Marshal(diagPayload)
		req = httptest.NewRequest(http.MethodPost, "/api/v1/forward/diagnose", bytes.NewReader(diagBody))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res = httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode diagnose response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected diagnose code 0, got %d (%s)", out.Code, out.Msg)
		}

		payload, ok := out.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("expected object payload, got %T", out.Data)
		}

		tproxy, ok := payload["tproxy"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected tproxy payload, got %T", payload["tproxy"])
		}

		if !valueAsBool(tproxy["enabled"]) {
			t.Fatalf("expected tproxy.enabled=true for transparent UDP forward")
		}
	})

	t.Run("normal UDP forward does not trigger TPROXY commands", func(t *testing.T) {
		var (
			mu       sync.Mutex
			commands []map[string]interface{}
		)
		stopNode := startMockNodeSessionWithCommandCapture(t, server.URL, "udp-e2e-secret", func(cmd map[string]interface{}) {
			mu.Lock()
			commands = append(commands, cmd)
			mu.Unlock()
		})
		defer stopNode()

		createPayload := map[string]interface{}{
			"name":       "udp-normal-no-tproxy",
			"tunnelId":   tunnelID,
			"remoteAddr": "1.0.0.1:53",
			"strategy":   "fifo",
			"udpMode":    "normal",
		}
		createBody, _ := json.Marshal(createPayload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCode(t, res, 0)

		mu.Lock()
		captured := append([]map[string]interface{}{}, commands...)
		mu.Unlock()

		hasTProxyCommand := func() bool {
			for _, cmd := range captured {
				cmdType := strings.ToLower(capturedCommandType(cmd))
				if strings.Contains(cmdType, "tproxy") {
					return true
				}
			}
			return false
		}

		if hasTProxyCommand() {
			t.Fatalf("expected no TPROXY commands for normal UDP mode, got: %v", captured)
		}

		forwardID := mustQueryInt64(t, repo, `SELECT id FROM forward WHERE name = ? ORDER BY id DESC LIMIT 1`, "udp-normal-no-tproxy")

		diagPayload := map[string]interface{}{"forwardId": forwardID}
		diagBody, _ := json.Marshal(diagPayload)
		req = httptest.NewRequest(http.MethodPost, "/api/v1/forward/diagnose", bytes.NewReader(diagBody))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res = httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode diagnose response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected diagnose code 0, got %d (%s)", out.Code, out.Msg)
		}

		payload, ok := out.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("expected object payload, got %T", out.Data)
		}

		tproxy, exists := payload["tproxy"].(map[string]interface{})
		if exists && valueAsBool(tproxy["enabled"]) {
			t.Fatalf("expected tproxy.enabled=false for normal UDP forward")
		}
	})
}

func TestUDPTransparentExcludePortsE2E(t *testing.T) {
	secret := "udp-exclude-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES('udp-exclude-tunnel', 1.0, 1, 'tls', 99999, ?, ?, 1, NULL, 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, repo, "udp-exclude-tunnel")

	if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES('udp-exclude-node', 'udp-exclude-secret', '10.200.0.1', '10.200.0.1', '', '61000-61010', '', 'v1', 1, 1, 1, ?, ?, 1, '[::]', '[::]', 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	nodeID := mustLastInsertID(t, repo, "udp-exclude-node")

	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 61001, 'round', 1, 'tls')
	`, tunnelID, nodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	server := httptest.NewServer(router)
	defer server.Close()

	t.Run("multiple transparent forwards on same node use exclude ports", func(t *testing.T) {
		var (
			mu             sync.Mutex
			ensurePayloads []map[string]interface{}
		)
		stopNode := startMockNodeSessionWithCommandCapture(t, server.URL, "udp-exclude-secret", func(cmd map[string]interface{}) {
			if strings.EqualFold(capturedCommandType(cmd), "EnsureTProxyPolicy") {
				mu.Lock()
				if data, ok := cmd["data"].(map[string]interface{}); ok {
					ensurePayloads = append(ensurePayloads, data)
				}
				mu.Unlock()
			}
		})
		defer stopNode()

		createForward := func(name, remote string) int64 {
			createPayload := map[string]interface{}{
				"name":       name,
				"tunnelId":   tunnelID,
				"remoteAddr": remote,
				"strategy":   "fifo",
				"udpMode":    "transparent",
			}
			createBody, _ := json.Marshal(createPayload)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
			req.Header.Set("Authorization", adminToken)
			req.Header.Set("Content-Type", "application/json")
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			assertCode(t, res, 0)
			return mustLastInsertID(t, repo, name)
		}

		createForward("udp-exclude-fwd-1", "8.8.8.8:53")
		createForward("udp-exclude-fwd-2", "8.8.4.4:53")

		mu.Lock()
		payloads := append([]map[string]interface{}{}, ensurePayloads...)
		mu.Unlock()

		if len(payloads) < 2 {
			t.Fatalf("expected at least 2 EnsureTProxyPolicy commands, got %d", len(payloads))
		}

		var lastPayload map[string]interface{}
		for _, p := range payloads {
			port := valueAsInt(p["port"])
			if port > 0 {
				lastPayload = p
			}
		}

		if lastPayload == nil {
			t.Fatalf("no valid EnsureTProxyPolicy payload found")
		}

		excludePorts, ok := lastPayload["excludePorts"].([]interface{})
		if !ok {
			t.Fatalf("expected excludePorts array in EnsureTProxyPolicy")
		}

		if len(excludePorts) < 1 {
			t.Fatalf("expected at least 1 exclude port when multiple transparent forwards exist, got %v", excludePorts)
		}
	})
}

func TestUDPTransparentLoopPreventionRejectsConflictE2E(t *testing.T) {
	secret := "udp-loop-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES('udp-loop-tunnel', 1.0, 1, 'tls', 99999, ?, ?, 1, NULL, 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, repo, "udp-loop-tunnel")

	if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES('udp-loop-node', 'udp-loop-secret', '10.210.0.1', '10.210.0.1', '', '62000-62010', '', 'v1', 1, 1, 1, ?, ?, 1, '[::]', '[::]', 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	nodeID := mustLastInsertID(t, repo, "udp-loop-node")

	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 62001, 'round', 1, 'tls')
	`, tunnelID, nodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	createPayload := map[string]interface{}{
		"name":       "udp-loop-forward",
		"tunnelId":   tunnelID,
		"remoteAddr": "10.210.0.1:62001",
		"strategy":   "fifo",
		"udpMode":    "transparent",
		"inPort":     62001,
	}
	createBody, _ := json.Marshal(createPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code == 0 {
		t.Fatalf("expected transparent loop prevention failure")
	}
	if !strings.Contains(out.Msg, "透明UDP防环路") {
		t.Fatalf("expected loop prevention message, got %q", out.Msg)
	}

	count := mustQueryInt(t, repo, `SELECT COUNT(1) FROM forward WHERE name = ?`, "udp-loop-forward")
	if count != 0 {
		t.Fatalf("expected no forward created on loop prevention failure, got %d", count)
	}
}

func TestUDPTransparentLoopPreventionRejectsIPv6ConflictE2E(t *testing.T) {
	secret := "udp-loop-ipv6-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES('udp-loop-ipv6-tunnel', 1.0, 1, 'tls', 99999, ?, ?, 1, NULL, 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, repo, "udp-loop-ipv6-tunnel")

	if err := repo.DB().Exec(`
		INSERT INTO node(name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx)
		VALUES('udp-loop-ipv6-node', 'udp-loop-ipv6-secret', '2001:db8::10', '', '2001:db8::10', '63000-63010', '', 'v1', 1, 1, 1, ?, ?, 1, '[::]', '[::]', 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	nodeID := mustLastInsertID(t, repo, "udp-loop-ipv6-node")

	if err := repo.DB().Exec(`
		INSERT INTO chain_tunnel(tunnel_id, chain_type, node_id, port, strategy, inx, protocol)
		VALUES(?, 1, ?, 63001, 'round', 1, 'tls')
	`, tunnelID, nodeID).Error; err != nil {
		t.Fatalf("insert chain_tunnel: %v", err)
	}

	createPayload := map[string]interface{}{
		"name":       "udp-loop-ipv6-forward",
		"tunnelId":   tunnelID,
		"remoteAddr": "[::1]:63001",
		"strategy":   "fifo",
		"udpMode":    "transparent",
		"inPort":     63001,
	}
	createBody, _ := json.Marshal(createPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
	req.Header.Set("Authorization", adminToken)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	var out response.R
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code == 0 {
		t.Fatalf("expected transparent IPv6 loop prevention failure")
	}
	if !strings.Contains(out.Msg, "透明UDP防环路") {
		t.Fatalf("expected loop prevention message, got %q", out.Msg)
	}

	count := mustQueryInt(t, repo, `SELECT COUNT(1) FROM forward WHERE name = ?`, "udp-loop-ipv6-forward")
	if count != 0 {
		t.Fatalf("expected no forward created on IPv6 loop prevention failure, got %d", count)
	}
}

func capturedCommandType(cmd map[string]interface{}) string {
	typ := strings.TrimSpace(valueAsString(cmd["type"]))
	commandType := strings.TrimSpace(valueAsString(cmd["commandType"]))
	if strings.EqualFold(typ, "RuntimeCommand") && commandType != "" {
		return commandType
	}
	if typ != "" {
		return typ
	}
	return commandType
}

func waitForCapturedCommand(mu *sync.Mutex, commands *[]map[string]interface{}, wantType string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		mu.Lock()
		for _, cmd := range *commands {
			if strings.EqualFold(capturedCommandType(cmd), wantType) {
				mu.Unlock()
				return true
			}
		}
		mu.Unlock()
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForEitherCapturedCommand(mu *sync.Mutex, commands *[]map[string]interface{}, timeout time.Duration, wantTypes ...string) bool {
	deadline := time.Now().Add(timeout)
	for {
		mu.Lock()
		for _, cmd := range *commands {
			got := capturedCommandType(cmd)
			for _, want := range wantTypes {
				if strings.EqualFold(got, want) {
					mu.Unlock()
					return true
				}
			}
		}
		mu.Unlock()
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func startMockNodeSessionWithCommandCapture(t *testing.T, baseURL string, nodeSecret string, onCommand func(cmd map[string]interface{})) func() {
	t.Helper()

	u, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
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

			var cmd map[string]interface{}
			if err := json.Unmarshal(plain, &cmd); err != nil {
				continue
			}

			requestID, _ := cmd["requestId"].(string)
			if strings.TrimSpace(requestID) == "" {
				continue
			}

			if onCommand != nil {
				onCommand(cmd)
			}

			cmdType, _ := cmd["type"].(string)
			respType := fmt.Sprintf("%sResponse", cmdType)
			respPayload := map[string]interface{}{
				"type":      respType,
				"success":   true,
				"message":   "OK",
				"requestId": requestID,
			}
			if strings.EqualFold(cmdType, "TcpPing") {
				respPayload["data"] = map[string]interface{}{
					"success":     true,
					"averageTime": 8.5,
					"packetLoss":  0,
					"message":     "mock tcp ok",
				}
			}
			if strings.EqualFold(cmdType, "ProbeTProxyCapability") {
				respPayload["data"] = map[string]interface{}{
					"supported":    true,
					"platform":     "linux",
					"hasIP":        true,
					"hasIPTables":  true,
					"hasIP6Tables": true,
					"stateEntries": 0,
				}
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
