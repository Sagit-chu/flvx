package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"gorm.io/gorm"

	"go-backend/internal/security"
	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
	"go-backend/internal/ws"
)

type cleanupAgentCommand struct {
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	RequestID string          `json:"requestId"`
}

// cleanupAgent exercises the actual command transport. Each command is recorded
// before its ACK, so synchronous handler calls need no sleeps to inspect it.
type cleanupAgent struct {
	h        *Handler
	mu       sync.Mutex
	commands []cleanupAgentCommand
}

func newCleanupAgent(t *testing.T) *cleanupAgent {
	t.Helper()
	r, err := repo.Open(filepath.Join(t.TempDir(), "cleanup.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	const secret = "cleanup-regression-node"
	if err := r.DB().Create(&model.Node{ID: 1, Name: "relay", Secret: secret, ServerIP: "127.0.0.1", Port: "30000", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	server := ws.NewServer(r, "test-jwt-secret")
	online := make(chan struct{})
	server.SetNodeOnlineHook(func(int64) { close(online) })
	serverDone := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer close(serverDone)
		server.ServeHTTP(w, req)
	}))
	t.Cleanup(ts.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+"?type=1&secret="+secret, nil)
	if err != nil {
		t.Fatal(err)
	}
	crypto, err := security.NewAESCrypto(secret)
	if err != nil {
		t.Fatal(err)
	}
	a := &cleanupAgent{h: &Handler{repo: r, wsServer: server}}
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var envelope struct {
				Encrypted bool   `json:"encrypted"`
				Data      string `json:"data"`
			}
			if err := json.Unmarshal(payload, &envelope); err != nil {
				t.Errorf("decode command envelope: %v", err)
				return
			}
			if envelope.Encrypted {
				payload, err = crypto.Decrypt(envelope.Data)
				if err != nil {
					t.Errorf("decrypt command: %v", err)
					return
				}
			}
			var command cleanupAgentCommand
			if err := json.Unmarshal(payload, &command); err != nil {
				t.Errorf("decode command: %v", err)
				return
			}
			a.mu.Lock()
			a.commands = append(a.commands, command)
			a.mu.Unlock()
			if err := conn.WriteJSON(map[string]interface{}{"type": command.Type, "requestId": command.RequestID, "success": true}); err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		_ = conn.Close()
		<-readerDone
		<-serverDone
	})
	select {
	case <-online:
	case <-time.After(3 * time.Second):
		t.Fatal("mock node did not come online")
	}
	a.probe(t)
	return a
}

func (a *cleanupAgent) probe(t *testing.T) {
	t.Helper()
	if _, err := a.h.wsServer.SendCommand(1, "CleanupTestProbe", nil, time.Second); err != nil {
		t.Fatalf("mock agent transport unavailable: %v", err)
	}
}

func (a *cleanupAgent) commandsOfType(commandType string) []cleanupAgentCommand {
	a.mu.Lock()
	defer a.mu.Unlock()
	var commands []cleanupAgentCommand
	for _, command := range a.commands {
		if command.Type == commandType {
			commands = append(commands, command)
		}
	}
	return commands
}

func (a *cleanupAgent) addRuntime(t *testing.T, name string, nodeID int64, status, applied int, updated time.Time) int64 {
	t.Helper()
	now := time.Now().UnixMilli()
	share := &repo.PeerShare{Name: "shared", NodeID: nodeID, Token: "cleanup-share-token", PortRangeStart: 31000, PortRangeEnd: 31010, IsActive: 1, CreatedTime: now, UpdatedTime: now}
	if err := a.h.repo.CreatePeerShare(share); err != nil {
		t.Fatal(err)
	}
	stored, err := a.h.repo.GetPeerShareByToken(share.Token)
	if err != nil || stored == nil {
		t.Fatalf("load share: %v", err)
	}
	// SQL preserves status=0; GORM's default tag would replace that with 1.
	if err := a.h.repo.DB().Exec(`INSERT INTO peer_share_runtime
		(share_id, node_id, reservation_id, resource_key, role, service_name, applied, status, created_time, updated_time)
		VALUES (?, ?, 'cleanup-reservation', 'cleanup-resource', 'forward', ?, ?, ?, ?, ?)`,
		stored.ID, nodeID, name, applied, status, now, updated.UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	return stored.ID
}

func TestSharedForwardCleanupRegression(t *testing.T) {
	for _, mode := range []string{"single", "batch", "config"} {
		for _, runtimeName := range []string{"70_1_0", "70_1_0_tcp", "70_1_0_udp"} {
			t.Run(mode+"/active/"+runtimeName, func(t *testing.T) {
				a := newCleanupAgent(t)
				shareID := a.addRuntime(t, runtimeName, 1, 1, 1, time.Now())
				runCleanupPath(a.h, mode, []string{"70_1_0", "70_1_0_tcp", "70_1_0_udp"})
				a.probe(t)
				if commands := a.commandsOfType("DeleteService"); len(commands) != 0 {
					t.Fatalf("active shared service family was deleted: %+v", commands)
				}
				if mode != "config" && runtimeName == "70_1_0" {
					share, err := a.h.repo.GetPeerShare(shareID)
					if err != nil || share == nil || share.CurrentFlow != 600 {
						t.Fatalf("shared traffic should accumulate 600 bytes: share=%+v err=%v", share, err)
					}
				}
			})
		}
		for _, tc := range []struct {
			name        string
			serviceName string
			nodeID      int64
			status      int
			applied     int
			age         time.Duration
			wantDelete  bool
		}{
			{name: "recent-unbound", nodeID: 1, status: 1, age: time.Minute},
			{name: "stale-unbound", nodeID: 1, status: 1, age: 11 * time.Minute, wantDelete: true},
			{name: "released", serviceName: "70_1_0", nodeID: 1, applied: 1, wantDelete: true},
			{name: "other-node-bound", serviceName: "70_1_0", nodeID: 2, status: 1, applied: 1, wantDelete: true},
			{name: "other-node-unbound", nodeID: 2, status: 1, wantDelete: true},
			{name: "unrelated-active-family", serviceName: "71_1_0", nodeID: 1, status: 1, applied: 1, wantDelete: true},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				a := newCleanupAgent(t)
				a.addRuntime(t, tc.serviceName, tc.nodeID, tc.status, tc.applied, time.Now().Add(-tc.age))
				runCleanupPath(a.h, mode, []string{"70_1_0_tcp"})
				a.probe(t)
				commands := a.commandsOfType("DeleteService")
				if !tc.wantDelete {
					if len(commands) != 0 {
						t.Fatalf("recent unbound shared runtime was deleted: %+v", commands)
					}
					return
				}
				if len(commands) != 1 {
					t.Fatalf("expected one orphan cleanup command, got %+v", commands)
				}
				var data struct {
					Services []string `json:"services"`
				}
				if err := json.Unmarshal(commands[0].Data, &data); err != nil {
					t.Fatal(err)
				}
				sort.Strings(data.Services)
				if want := []string{"70_1_0", "70_1_0_tcp", "70_1_0_udp"}; !reflect.DeepEqual(data.Services, want) {
					t.Fatalf("orphan cleanup must delete complete family: got %v, want %v", data.Services, want)
				}
			})
		}
	}
}

func runCleanupPath(h *Handler, mode string, names []string) {
	items := make([]flowItem, 0, len(names))
	services := make([]namedConfigItem, 0, len(names))
	for _, name := range names {
		items = append(items, flowItem{N: name, U: 120, D: 80})
		services = append(services, namedConfigItem{Name: name})
	}
	switch mode {
	case "single":
		for _, item := range items {
			h.processFlowItem(1, item)
		}
	case "batch":
		h.applyFlowUploadBatch(1, h.buildNodeFlowUploadBatch(1, items, nil), time.Now())
	case "config":
		h.cleanOrphanedServices(1, services)
	}
}

func TestSharedForwardCleanupMissingMetadataPreservesLocalForward(t *testing.T) {
	a := newCleanupAgent(t)
	if err := a.h.repo.DB().Create(&model.Forward{ID: 70, UserID: 1, UserName: "local", Name: "local", TunnelID: 1, RemoteAddr: "127.0.0.1:80", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	runCleanupPath(a.h, "batch", []string{"70_1_0", "70_1_0_tcp", "70_1_0_udp"})
	a.probe(t)
	if commands := a.commandsOfType("DeleteService"); len(commands) != 0 {
		t.Fatalf("missing batch metadata must not delete a local forward: %+v", commands)
	}
}

func TestSharedForwardCleanupChecksActualDeleteFamily(t *testing.T) {
	for _, mode := range []string{"single", "batch", "config"} {
		t.Run(mode, func(t *testing.T) {
			a := newCleanupAgent(t)
			a.addRuntime(t, "70_1_0", 1, 1, 1, time.Now())
			// Legacy parsing accepts additional suffixes. Cleanup must check
			// the family it would delete, not just the reported service name.
			runCleanupPath(a.h, mode, []string{"70_1_0_old", "70_1_0_old_tcp"})
			a.probe(t)
			if commands := a.commandsOfType("DeleteService"); len(commands) != 0 {
				t.Fatalf("suffix variation must not delete a protected shared family: %+v", commands)
			}
		})
	}
}

func TestSharedForwardCleanupQueryFailurePreservesServices(t *testing.T) {
	for _, mode := range []string{"single", "batch", "config"} {
		for _, failure := range []string{"forward", "peer_share_runtime"} {
			t.Run(mode+"/"+failure, func(t *testing.T) {
				a := newCleanupAgent(t)
				// Fail only the ownership lookup; leave node lookup and WebSocket
				// delivery functional so an erroneous delete remains observable.
				callback := "test:cleanup-query-failure"
				var injected atomic.Int32
				if err := a.h.repo.DB().Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Table == failure {
						injected.Add(1)
						tx.AddError(errors.New("injected ownership query failure"))
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = a.h.repo.DB().Callback().Query().Remove(callback) })
				runCleanupPath(a.h, mode, []string{"70_1_0_tcp"})
				a.probe(t)
				if injected.Load() == 0 {
					t.Fatal("test did not inject the expected query failure")
				}
				if commands := a.commandsOfType("DeleteService"); len(commands) != 0 {
					t.Fatalf("ownership query failure must preserve services: %+v", commands)
				}
			})
		}
	}
}
