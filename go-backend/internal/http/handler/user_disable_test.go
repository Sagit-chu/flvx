package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go-backend/internal/http/response"
	"go-backend/internal/store/model"
	"go-backend/internal/ws"
)

func TestUserUpdateDisablesPermissionsAndRunningForwards(t *testing.T) {
	for _, password := range []string{"", "replacement-password"} {
		t.Run(fmt.Sprintf("password_changed=%t", password != ""), func(t *testing.T) {
			a := newCleanupAgent(t)
			now := time.Now().UnixMilli()
			for _, userID := range []int64{2, 3} {
				if err := a.h.repo.DB().Create(&model.User{ID: userID, User: fmt.Sprintf("user%d", userID), Pwd: "pwd", RoleID: 1, Status: 1, ExpTime: now + 86400000, Flow: 100, Num: 10}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := a.h.repo.DB().Create(&model.Tunnel{ID: 1, Name: "test", Type: 1, Protocol: "tls", Status: 1, Flow: 1}).Error; err != nil {
				t.Fatal(err)
			}
			for _, permission := range []model.UserTunnel{
				{ID: 10, UserID: 2, TunnelID: 1, Status: 1, Flow: 100, Num: 10},
				{ID: 11, UserID: 2, TunnelID: 2, Status: 0, Flow: 100, Num: 10},
				{ID: 12, UserID: 3, TunnelID: 1, Status: 1, Flow: 100, Num: 10},
			} {
				if err := a.h.repo.DB().Create(&permission).Error; err != nil {
					t.Fatal(err)
				}
			}
			for _, forward := range []model.Forward{
				{ID: 20, UserID: 2, UserName: "user2", Name: "running", TunnelID: 1, RemoteAddr: "127.0.0.1:80", Status: 1},
				{ID: 21, UserID: 2, UserName: "user2", Name: "paused", TunnelID: 1, RemoteAddr: "127.0.0.1:81", Status: 0},
				{ID: 22, UserID: 3, UserName: "user3", Name: "other-user", TunnelID: 1, RemoteAddr: "127.0.0.1:82", Status: 1},
			} {
				if err := a.h.repo.DB().Create(&forward).Error; err != nil {
					t.Fatal(err)
				}
				if err := a.h.repo.DB().Create(&model.ForwardPort{ForwardID: forward.ID, NodeID: 1, Port: 31000 + int(forward.ID)}).Error; err != nil {
					t.Fatal(err)
				}
			}

			update := func(status int) {
				t.Helper()
				payload, err := json.Marshal(map[string]interface{}{"id": 2, "user": "user2", "status": status, "pwd": password, "flow": 100, "num": 10, "expTime": now + 86400000})
				if err != nil {
					t.Fatal(err)
				}
				res := httptest.NewRecorder()
				a.h.userUpdate(res, httptest.NewRequest(http.MethodPost, "/api/v1/user/update", bytes.NewReader(payload)))
				var result response.R
				if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.Code != 0 {
					t.Fatalf("update user: %d %s", result.Code, result.Msg)
				}
			}

			update(1)
			if got := mustQueryInt(t, a.h.repo, "SELECT status FROM user_tunnel WHERE id = 10"); got != 1 {
				t.Errorf("ordinary account update disabled an enabled permission: %d", got)
			}
			if len(a.commandsOfType("PauseService")) != 0 {
				t.Error("ordinary account update paused services")
			}
			update(0)
			for _, check := range []struct {
				table string
				id    int64
				want  int
			}{{"user", 2, 0}, {"user_tunnel", 10, 0}, {"user_tunnel", 11, 0}, {"user_tunnel", 12, 1}, {"forward", 20, 0}, {"forward", 21, 0}, {"forward", 22, 1}} {
				if got := mustQueryInt(t, a.h.repo, "SELECT status FROM "+check.table+" WHERE id = ?", check.id); got != check.want {
					t.Errorf("%s %d status = %d, want %d", check.table, check.id, got, check.want)
				}
			}
			commands := a.commandsOfType("PauseService")
			if len(commands) == 0 {
				t.Error("running forward never received PauseService")
			}
			for _, command := range commands {
				var data struct {
					Services []string `json:"services"`
				}
				if err := json.Unmarshal(command.Data, &data); err != nil {
					t.Fatal(err)
				}
				for _, name := range data.Services {
					if !strings.HasPrefix(name, "20_2_") {
						t.Errorf("paused unrelated service %q", name)
					}
				}
			}
			update(0)
			if len(a.commandsOfType("PauseService")) != len(commands) {
				t.Error("repeated disable replayed pauses for already paused forwards")
			}

			// Account edits and re-enabling must not grant disabled permissions or
			// resume forwards that an administrator previously paused.
			update(1)
			if got := mustQueryInt(t, a.h.repo, "SELECT status FROM user_tunnel WHERE id = 10"); got != 0 {
				t.Errorf("account re-enable unexpectedly granted a disabled permission: %d", got)
			}
			if got := mustQueryInt(t, a.h.repo, "SELECT status FROM user_tunnel WHERE id = 11"); got != 0 {
				t.Errorf("re-enabled independently disabled permission: %d", got)
			}
			if got := mustQueryInt(t, a.h.repo, "SELECT status FROM forward WHERE id = 21"); got != 0 {
				t.Errorf("resumed previously paused forward: %d", got)
			}
			if len(a.commandsOfType("ResumeService")) != 0 {
				t.Error("account re-enable unexpectedly resumed services")
			}
		})
	}
}

func TestUserDisableMultiEntryNeverRecreatesMissingServices(t *testing.T) {
	for _, secondNodeState := range []string{"missing", "offline", "timeout"} {
		t.Run(secondNodeState, func(t *testing.T) {
			a := newCleanupAgent(t)
			var remoteMu sync.Mutex
			remoteServices := make(map[string]bool)
			var remoteCommands []string
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var command struct {
					Type string          `json:"commandType"`
					Data json.RawMessage `json:"data"`
				}
				if err := json.NewDecoder(r.Body).Decode(&command); err != nil {
					t.Error(err)
					return
				}
				remoteMu.Lock()
				defer remoteMu.Unlock()
				remoteCommands = append(remoteCommands, command.Type)
				switch command.Type {
				case "UpdateService", "AddService":
					var services []struct {
						Name string `json:"name"`
					}
					if err := json.Unmarshal(command.Data, &services); err != nil {
						t.Error(err)
						return
					}
					for _, service := range services {
						remoteServices[service.Name] = true
					}
				case "PauseService":
					if secondNodeState != "missing" {
						message := "节点不在线"
						if secondNodeState == "timeout" {
							message = "等待节点响应超时"
						}
						response.WriteJSON(w, response.ErrDefault(message))
						return
					}
					var data struct {
						Services []string `json:"services"`
					}
					if err := json.Unmarshal(command.Data, &data); err != nil {
						t.Error(err)
						return
					}
					for _, name := range data.Services {
						if _, exists := remoteServices[name]; !exists {
							response.WriteJSON(w, response.OK(map[string]interface{}{"success": false, "message": "service not found"}))
							return
						}
						remoteServices[name] = false
					}
				}
				response.WriteJSON(w, response.OK(map[string]interface{}{"success": true, "type": command.Type}))
			}))
			defer remote.Close()
			now := time.Now().UnixMilli()
			userID, err := a.h.repo.CreateUser("multi-entry-disable", "pwd", 1, now+86400000, 100, 1, 10, 1, 0, now)
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range []interface{}{
				&model.Node{ID: 2, Name: "second-entry", Secret: "test", ServerIP: "127.0.0.1", Status: 1, IsRemote: 1, RemoteURL: sql.NullString{String: remote.URL, Valid: true}, RemoteToken: sql.NullString{String: "test-token", Valid: true}},
				&model.Tunnel{ID: 1, Name: "multi-entry", Type: 1, Status: 1, Protocol: "tls", Flow: 1},
				&model.ChainTunnel{TunnelID: 1, NodeID: 1, ChainType: "1"},
				&model.ChainTunnel{TunnelID: 1, NodeID: 2, ChainType: "1"},
				&model.UserTunnel{ID: 10, UserID: userID, TunnelID: 1, Status: 1, Flow: 100, Num: 10},
				&model.Forward{ID: 20, UserID: userID, UserName: "multi-entry-disable", Name: "running", TunnelID: 1, RemoteAddr: "127.0.0.1:80", Status: 1},
				&model.ForwardPort{ForwardID: 20, NodeID: 1, Port: 31020},
				&model.ForwardPort{ForwardID: 20, NodeID: 2, Port: 31020},
			} {
				if err := a.h.repo.DB().Create(value).Error; err != nil {
					t.Fatal(err)
				}
			}
			res := postJSONToHandler(t, a.h.userUpdate, map[string]interface{}{"id": userID, "user": "multi-entry-disable", "status": 0})
			var out response.R
			if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			wantStatus := 1
			if secondNodeState == "missing" {
				wantStatus = 0
				if out.Code != 0 {
					t.Errorf("missing service should count as stopped: %s", out.Msg)
				}
			} else if out.Code == 0 {
				t.Error("unconfirmed second entry pause returned success")
			}
			if got := mustQueryInt(t, a.h.repo, "SELECT status FROM forward WHERE id = 20"); got != wantStatus {
				t.Errorf("forward status = %d, want %d", got, wantStatus)
			}
			a.mu.Lock()
			commands := append([]cleanupAgentCommand(nil), a.commands...)
			a.mu.Unlock()
			firstEntryRunning := true
			for _, command := range commands {
				switch command.Type {
				case "PauseService":
					firstEntryRunning = false
				case "UpdateService", "AddService", "ResumeService":
					firstEntryRunning = true
					t.Errorf("disable sent %s to already paused entry A", command.Type)
				}
			}
			if firstEntryRunning {
				t.Error("entry A is running after disable")
			}
			remoteMu.Lock()
			defer remoteMu.Unlock()
			if len(remoteCommands) == 0 {
				t.Error("entry B was never checked")
			}
			for _, command := range remoteCommands {
				if command != "PauseService" {
					t.Errorf("disable sent %s to entry B", command)
				}
			}
			for name, running := range remoteServices {
				if running {
					t.Errorf("entry B service %s is still running", name)
				}
			}
		})
	}
}

func TestUserDisableContinuesAcrossNodesAndForwards(t *testing.T) {
	for _, failure := range []string{"节点不在线", "等待节点响应超时", "pause rejected"} {
		for _, failedNodeFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/failed_node_first=%t", failure, failedNodeFirst), func(t *testing.T) {
				a := newCleanupAgent(t)
				var mu sync.Mutex
				recovered := false
				attempts := make(map[string]int)
				remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var command struct {
						Type string `json:"commandType"`
					}
					if err := json.NewDecoder(r.Body).Decode(&command); err != nil {
						t.Error(err)
						return
					}
					if command.Type != "PauseService" {
						t.Errorf("disable sent %s", command.Type)
					}
					mu.Lock()
					attempts[r.Header.Get("Authorization")]++
					isRecovered := recovered
					mu.Unlock()
					if !isRecovered {
						response.WriteJSON(w, response.ErrDefault(failure))
						return
					}
					response.WriteJSON(w, response.OK(map[string]interface{}{"success": true, "type": command.Type}))
				}))
				defer remote.Close()
				now := time.Now().UnixMilli()
				userID, err := a.h.repo.CreateUser("continue-disable", "pwd", 1, now+86400000, 100, 1, 10, 1, 0, now)
				if err != nil {
					t.Fatal(err)
				}
				for _, value := range []interface{}{
					&model.Tunnel{ID: 1, Name: "multi-entry", Type: 2, Status: 1, Flow: 1},
					&model.Node{ID: 4, Name: "offline-middle", Secret: "test", ServerIP: "127.0.0.1", Status: 0},
					&model.ChainTunnel{TunnelID: 1, NodeID: 4, ChainType: "2"},
					&model.UserTunnel{ID: 10, UserID: userID, TunnelID: 1, Status: 1, Flow: 100, Num: 10},
					&model.Forward{ID: 20, UserID: userID, UserName: "continue-disable", Name: "mixed-nodes", TunnelID: 1, RemoteAddr: "127.0.0.1:80", Status: 1},
					&model.Forward{ID: 21, UserID: userID, UserName: "continue-disable", Name: "healthy-forward", TunnelID: 1, RemoteAddr: "127.0.0.1:81", Status: 1},
				} {
					if err := a.h.repo.DB().Create(value).Error; err != nil {
						t.Fatal(err)
					}
				}
				for _, nodeID := range []int64{2, 3} {
					node := model.Node{ID: nodeID, Name: fmt.Sprintf("failed-node-%d", nodeID), Secret: "test", ServerIP: "127.0.0.1", Status: 1, IsRemote: 1, RemoteURL: sql.NullString{String: remote.URL, Valid: true}, RemoteToken: sql.NullString{String: fmt.Sprintf("test-node-%d", nodeID), Valid: true}}
					if err := a.h.repo.DB().Create(&node).Error; err != nil {
						t.Fatal(err)
					}
				}
				nodeOrder := []int64{1, 2, 3}
				if failedNodeFirst {
					nodeOrder = []int64{2, 1, 3}
				}
				for _, nodeID := range nodeOrder {
					if err := a.h.repo.DB().Create(&model.ForwardPort{ForwardID: 20, NodeID: nodeID, Port: 31020}).Error; err != nil {
						t.Fatal(err)
					}
					if err := a.h.repo.DB().Create(&model.ChainTunnel{TunnelID: 1, NodeID: nodeID, ChainType: "1"}).Error; err != nil {
						t.Fatal(err)
					}
				}
				if err := a.h.repo.DB().Create(&model.ForwardPort{ForwardID: 21, NodeID: 1, Port: 31021}).Error; err != nil {
					t.Fatal(err)
				}
				update := func() response.R {
					t.Helper()
					res := postJSONToHandler(t, a.h.userUpdate, map[string]interface{}{"id": userID, "user": "continue-disable", "status": 0})
					var out response.R
					if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
						t.Fatal(err)
					}
					return out
				}
				out := update()
				if out.Code == 0 {
					t.Error("partial node failure reported complete success")
				}
				state, ok := out.Data.(map[string]interface{})
				if !ok || state["userDisabled"] != true {
					t.Errorf("partial response did not tell the UI the account is disabled: %#v", out.Data)
				}
				for _, nodeID := range []int64{2, 3} {
					if !strings.Contains(out.Msg, fmt.Sprintf("failed-node-%d", nodeID)) || !strings.Contains(out.Msg, fmt.Sprintf("ID %d", nodeID)) {
						t.Errorf("failure response did not identify node %d: %s", nodeID, out.Msg)
					}
					mu.Lock()
					count := attempts[fmt.Sprintf("Bearer test-node-%d", nodeID)]
					mu.Unlock()
					if count == 0 {
						t.Errorf("failure on another node prevented node %d from being attempted", nodeID)
					}
				}
				paused := make(map[string]bool)
				for _, command := range a.commandsOfType("PauseService") {
					var data struct {
						Services []string `json:"services"`
					}
					if err := json.Unmarshal(command.Data, &data); err != nil {
						t.Fatal(err)
					}
					for _, name := range data.Services {
						paused[strings.Split(name, "_")[0]] = true
					}
				}
				for _, id := range []string{"20", "21"} {
					if !paused[id] {
						t.Errorf("healthy node never paused forward %s", id)
					}
				}
				if got := mustQueryInt(t, a.h.repo, "SELECT status FROM forward WHERE id = 20"); got != 1 {
					t.Errorf("unconfirmed forward status = %d", got)
				}
				if got := mustQueryInt(t, a.h.repo, "SELECT status FROM forward WHERE id = 21"); got != 0 {
					t.Errorf("healthy later forward was not paused: %d", got)
				}
				mu.Lock()
				recovered = true
				mu.Unlock()
				if retry := update(); retry.Code != 0 {
					t.Errorf("retry after recovery failed: %s", retry.Msg)
				}
				if got := mustQueryInt(t, a.h.repo, "SELECT status FROM forward WHERE id = 20"); got != 0 {
					t.Errorf("retry did not confirm all nodes: %d", got)
				}
				for _, kind := range []string{"UpdateService", "AddService", "ResumeService"} {
					if len(a.commandsOfType(kind)) != 0 {
						t.Errorf("disable sent %s", kind)
					}
				}
			})
		}
	}
}

func TestUserDisableRemovesNftablesRuleOnlyAfterConfirmedApply(t *testing.T) {
	fixture := setupNftablesHandler(t)
	h := fixture.handler
	t.Cleanup(func() { _ = h.repo.Close() })
	seedNftablesSSHConfig(t, h, fixture.nodeID)
	tunnelID := seedTunnelForNftables(t, h, "disable-nft", fixture.nodeID)
	forward := seedForwardForNftables(t, h, tunnelID, fixture.nodeID, "203.0.113.9:8080")
	now := time.Now().UnixMilli()
	userID, err := h.repo.CreateUser("disable-nft", "pwd", 1, now+86400000, 100, 1, 10, 1, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.repo.DB().Model(&model.Forward{}).Where("id = ?", forward.ID).Update("user_id", userID).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.repo.DB().Create(&model.UserTunnel{UserID: userID, TunnelID: tunnelID, Status: 1, Flow: 100, Num: 10}).Error; err != nil {
		t.Fatal(err)
	}
	otherForwardID := forward.ID + 100
	for _, value := range []interface{}{
		&model.Forward{ID: otherForwardID, UserID: 1, UserName: "admin", Name: "other-user", TunnelID: tunnelID, RemoteAddr: "203.0.113.9:8081", Status: 1},
		&model.ForwardPort{ForwardID: otherForwardID, NodeID: fixture.nodeID, Port: 20001},
	} {
		if err := h.repo.DB().Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	manager := &fakeNftablesManager{reconcileErr: errors.New("ssh unavailable")}
	h.nftablesManager = manager
	update := func() response.R {
		t.Helper()
		res := postJSONToHandler(t, h.userUpdate, map[string]interface{}{"id": userID, "user": "disable-nft", "status": 0})
		var out response.R
		if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if out := update(); out.Code == 0 {
		t.Error("failed SSH apply returned success")
	}
	if got := mustQueryInt(t, h.repo, "SELECT status FROM forward WHERE id = ?", forward.ID); got != 1 {
		t.Errorf("unconfirmed nftables pause marked completed: %d", got)
	}
	if manager.reconcileCount() != 1 || len(manager.lastPlan.Rules) != 1 || manager.lastPlan.Rules[0].ForwardID != otherForwardID {
		t.Fatalf("did not attempt to remove disabled user's nftables rule: %+v", manager.lastPlan)
	}
	manager.reconcileErr = nil
	if out := update(); out.Code != 0 {
		t.Fatalf("retry nftables disable: %s", out.Msg)
	}
	if got := mustQueryInt(t, h.repo, "SELECT status FROM forward WHERE id = ?", forward.ID); got != 0 {
		t.Errorf("confirmed nftables pause was not stored: %d", got)
	}
	if manager.reconcileCount() != 2 || len(manager.lastPlan.Rules) != 1 || manager.lastPlan.Rules[0].ForwardID != otherForwardID {
		t.Errorf("retry did not apply a plan without disabled forward: %+v", manager.lastPlan)
	}
	if got := mustQueryInt(t, h.repo, "SELECT status FROM forward WHERE id = ?", otherForwardID); got != 1 {
		t.Errorf("disabled another user's nftables forward: %d", got)
	}
}

func TestUserDisableReportsPauseFailureAndCanRetry(t *testing.T) {
	for _, offline := range []bool{false, true} {
		t.Run(fmt.Sprintf("offline=%t", offline), func(t *testing.T) {
			a := newCleanupAgent(t)
			now := time.Now().UnixMilli()
			userID, err := a.h.repo.CreateUser("disable-retry", "pwd", 1, now+86400000, 100, 1, 10, 1, 0, now)
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range []interface{}{
				&model.Tunnel{ID: 1, Name: "test", Type: 1, Status: 1, Flow: 1},
				&model.UserTunnel{ID: 10, UserID: userID, TunnelID: 1, Status: 1, Flow: 100, Num: 10},
				&model.Forward{ID: 20, UserID: userID, UserName: "disable-retry", Name: "running", TunnelID: 1, RemoteAddr: "127.0.0.1:80", Status: 1},
				&model.ForwardPort{ForwardID: 20, NodeID: 1, Port: 31020},
			} {
				if err := a.h.repo.DB().Create(value).Error; err != nil {
					t.Fatal(err)
				}
			}
			onlineServer := a.h.wsServer
			if offline {
				a.h.wsServer = ws.NewServer(a.h.repo, "test-jwt-secret")
			} else {
				ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					response.WriteJSON(w, response.ErrDefault("pause rejected"))
				}))
				defer ts.Close()
				if err := a.h.repo.DB().Model(&model.Node{}).Where("id = 1").Updates(map[string]interface{}{"is_remote": 1, "remote_url": ts.URL, "remote_token": "test-token"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			update := func() response.R {
				t.Helper()
				res := postJSONToHandler(t, a.h.userUpdate, map[string]interface{}{"id": userID, "user": "disable-retry", "status": 0})
				var out response.R
				if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
					t.Fatal(err)
				}
				return out
			}
			if out := update(); out.Code == 0 {
				t.Error("pause failure returned success")
			}
			if got := mustQueryInt(t, a.h.repo, "SELECT status FROM user WHERE id = ?", userID); got != 0 {
				t.Errorf("failed pause restored account access: %d", got)
			}
			if got := mustQueryInt(t, a.h.repo, "SELECT status FROM user_tunnel WHERE id = 10"); got != 0 {
				t.Errorf("failed pause restored tunnel access: %d", got)
			}
			if got := mustQueryInt(t, a.h.repo, "SELECT status FROM forward WHERE id = 20"); got != 1 {
				t.Errorf("unconfirmed pause marked completed, status = %d", got)
			}
			a.h.wsServer = onlineServer
			if err := a.h.repo.DB().Model(&model.Node{}).Where("id = 1").Update("is_remote", 0).Error; err != nil {
				t.Fatal(err)
			}
			if out := update(); out.Code != 0 {
				t.Fatalf("retry failed: %s", out.Msg)
			}
			if got := mustQueryInt(t, a.h.repo, "SELECT status FROM forward WHERE id = 20"); got != 0 {
				t.Errorf("retry did not pause forward: %d", got)
			}
			if len(a.commandsOfType("PauseService")) == 0 {
				t.Error("retry never sent PauseService")
			}
		})
	}
}
