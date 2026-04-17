package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	backendruntime "go-backend/internal/runtime"
	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
)

func TestGetRuntimeSettingsReturnsCurrentEngine(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	if err := r.DB().Create(&model.Node{
		Name:          "node-a",
		Secret:        "secret-a",
		ServerIP:      "10.0.0.1",
		Port:          "2024",
		CreatedTime:   1,
		Status:        1,
		TCPListenAddr: "[::]",
		UDPListenAddr: "[::]",
	}).Error; err != nil {
		t.Fatalf("insert node a: %v", err)
	}
	if err := r.DB().Create(&model.Node{
		Name:          "node-b",
		Secret:        "secret-b",
		ServerIP:      "10.0.0.2",
		Port:          "2025",
		CreatedTime:   1,
		Status:        1,
		TCPListenAddr: "[::]",
		UDPListenAddr: "[::]",
	}).Error; err != nil {
		t.Fatalf("insert node b: %v", err)
	}

	if err := r.SetRuntimeEngine(repo.RuntimeEngineDash, 10); err != nil {
		t.Fatalf("set runtime engine: %v", err)
	}
	if err := r.SetRuntimeSwitchState(repo.RuntimeSwitchState{
		Status:     repo.RuntimeSwitchStatusFailed,
		Generation: 7,
		Error:      "deploy failed",
	}, 10); err != nil {
		t.Fatalf("set runtime switch state: %v", err)
	}

	h := New(r, "secret")
	h.runtimeStatusProviders = map[backendruntime.Engine]runtimeStatusProvider{
		backendruntime.EngineDash: runtimeStatusProviderFunc(func(_ context.Context, node repo.Node) (backendruntime.NodeRuntimeStatus, error) {
			switch node.Name {
			case "node-a":
				return backendruntime.NodeRuntimeStatus{NodeID: node.ID, Engine: backendruntime.EngineDash, Ready: true, Progress: backendruntime.ProgressStateSucceeded}, nil
			default:
				return backendruntime.NodeRuntimeStatus{NodeID: node.ID, Engine: backendruntime.EngineDash, Ready: false, Progress: backendruntime.ProgressStatePending, Message: "waiting"}, nil
			}
		}),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/runtime", nil)
	res := httptest.NewRecorder()

	h.runtimeSettings(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	var payload runtimeSettingsEnvelope
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != 0 {
		t.Fatalf("expected success code, got %d with msg %q", payload.Code, payload.Msg)
	}
	if payload.Data.CurrentEngine != string(repo.RuntimeEngineDash) {
		t.Fatalf("expected current engine dash, got %q", payload.Data.CurrentEngine)
	}
	if payload.Data.SwitchStatus != string(repo.RuntimeSwitchStatusFailed) {
		t.Fatalf("expected failed switch status, got %q", payload.Data.SwitchStatus)
	}
	if payload.Data.Generation != 7 {
		t.Fatalf("expected generation 7, got %d", payload.Data.Generation)
	}
	if payload.Data.LastError != "deploy failed" {
		t.Fatalf("expected last error to be preserved, got %q", payload.Data.LastError)
	}
	if payload.Data.NodeSummary.Total != 2 || payload.Data.NodeSummary.Ready != 1 || payload.Data.NodeSummary.Pending != 1 {
		t.Fatalf("unexpected node summary: %+v", payload.Data.NodeSummary)
	}
	if payload.Data.RuntimeProgress.Engine != string(repo.RuntimeEngineDash) {
		t.Fatalf("expected runtime progress engine dash, got %q", payload.Data.RuntimeProgress.Engine)
	}
	if payload.Data.RuntimeProgress.State != string(backendruntime.ProgressStatePending) {
		t.Fatalf("expected runtime progress pending, got %q", payload.Data.RuntimeProgress.State)
	}
	if len(payload.Data.Nodes) != 2 {
		t.Fatalf("expected 2 node progress entries, got %d", len(payload.Data.Nodes))
	}
}

func TestPutRuntimeSettingsStartsSwitch(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	for _, node := range []model.Node{
		{
			Name:          "node-a",
			Secret:        "secret-a",
			ServerIP:      "10.0.0.1",
			Port:          "2024",
			CreatedTime:   1,
			Status:        1,
			TCPListenAddr: "[::]",
			UDPListenAddr: "[::]",
		},
		{
			Name:          "node-b",
			Secret:        "secret-b",
			ServerIP:      "10.0.0.2",
			Port:          "2025",
			CreatedTime:   1,
			Status:        1,
			TCPListenAddr: "[::]",
			UDPListenAddr: "[::]",
		},
	} {
		if err := r.DB().Create(&node).Error; err != nil {
			t.Fatalf("insert node %s: %v", node.Name, err)
		}
	}
	if err := r.SetRuntimeEngine(repo.RuntimeEngineGost, 10); err != nil {
		t.Fatalf("set runtime engine: %v", err)
	}

	switcher := &stubRuntimeSwitchStarter{switchFunc: func(_ context.Context, _ backendruntime.Engine, _ []repo.Node) error {
		return r.SetRuntimeSwitchState(repo.RuntimeSwitchState{
			Status:     repo.RuntimeSwitchStatusSwitching,
			Generation: 3,
			Error:      "",
		}, 11)
	}}
	h := New(r, "secret")
	h.runtimeSwitchStarter = switcher
	h.runtimeStatusProviders = map[backendruntime.Engine]runtimeStatusProvider{
		backendruntime.EngineGost: runtimeStatusProviderFunc(func(_ context.Context, node repo.Node) (backendruntime.NodeRuntimeStatus, error) {
			return backendruntime.NodeRuntimeStatus{NodeID: node.ID, Engine: backendruntime.EngineGost, Ready: true, Progress: backendruntime.ProgressStateSucceeded}, nil
		}),
	}

	body := bytes.NewBufferString(`{"engine":"dash"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/system/runtime", body)
	res := httptest.NewRecorder()

	h.runtimeSettings(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}
	if switcher.calls != 1 {
		t.Fatalf("expected switcher to be called once, got %d", switcher.calls)
	}
	if switcher.engine != backendruntime.EngineDash {
		t.Fatalf("expected dash switch, got %q", switcher.engine)
	}
	gotNodeIDs := make([]int64, 0, len(switcher.nodes))
	for _, node := range switcher.nodes {
		gotNodeIDs = append(gotNodeIDs, node.ID)
	}
	if !reflect.DeepEqual(gotNodeIDs, []int64{1, 2}) {
		t.Fatalf("expected switch nodes [1 2], got %v", gotNodeIDs)
	}

	var payload runtimeSettingsEnvelope
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != 0 {
		t.Fatalf("expected success code, got %d with msg %q", payload.Code, payload.Msg)
	}
	if payload.Data.CurrentEngine != string(repo.RuntimeEngineGost) {
		t.Fatalf("expected current engine gost during active switch, got %q", payload.Data.CurrentEngine)
	}
	if payload.Data.SwitchStatus != string(repo.RuntimeSwitchStatusSwitching) {
		t.Fatalf("expected switch status switching, got %q", payload.Data.SwitchStatus)
	}
	if payload.Data.RuntimeProgress.State != string(repo.RuntimeSwitchStatusSwitching) {
		t.Fatalf("expected runtime progress state switching, got %q", payload.Data.RuntimeProgress.State)
	}
}

func TestRuntimeProgressUsesOrchestratorProgressWhileSwitching(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	for _, node := range []model.Node{
		{Name: "node-a", Secret: "secret-a", ServerIP: "10.0.0.1", Port: "2024", CreatedTime: 1, Status: 1, TCPListenAddr: "[::]", UDPListenAddr: "[::]"},
		{Name: "node-b", Secret: "secret-b", ServerIP: "10.0.0.2", Port: "2025", CreatedTime: 1, Status: 1, TCPListenAddr: "[::]", UDPListenAddr: "[::]"},
	} {
		if err := r.DB().Create(&node).Error; err != nil {
			t.Fatalf("insert node %s: %v", node.Name, err)
		}
	}
	if err := r.SetRuntimeEngine(repo.RuntimeEngineDash, 10); err != nil {
		t.Fatalf("set runtime engine: %v", err)
	}
	if err := r.SetRuntimeSwitchState(repo.RuntimeSwitchState{Status: repo.RuntimeSwitchStatusSwitching, Generation: 4}, 10); err != nil {
		t.Fatalf("set runtime switch state: %v", err)
	}

	h := New(r, "secret")
	h.runtimeStatusProviders = map[backendruntime.Engine]runtimeStatusProvider{
		backendruntime.EngineDash: runtimeStatusProviderFunc(func(_ context.Context, node repo.Node) (backendruntime.NodeRuntimeStatus, error) {
			return backendruntime.NodeRuntimeStatus{NodeID: node.ID, Engine: backendruntime.EngineDash, Ready: true, Progress: backendruntime.ProgressStateSucceeded}, nil
		}),
	}
	h.runtimeSwitchStarter = &stubRuntimeSwitchStarter{
		progress: backendruntime.SwitchProgress{
			Engine:     backendruntime.EngineDash,
			Status:     backendruntime.RuntimeSwitchStatusSwitching,
			Generation: 4,
			Phase:      "rebuild_runtime",
			Message:    "rebuilding runtime objects",
			Nodes: []backendruntime.NodeRuntimeProgress{{
				NodeID:   1,
				Engine:   backendruntime.EngineDash,
				State:    backendruntime.ProgressStateRunning,
				Message:  "deploying runtime",
				Complete: false,
			}},
			Rebuild: backendruntime.RebuildRuntimeProgress{
				Engine:   backendruntime.EngineDash,
				State:    backendruntime.ProgressStateRunning,
				Message:  "rebuilding runtime objects",
				Warnings: []string{"skipped tunnel 7: missing chain node"},
				Complete: false,
			},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/runtime/progress", nil)
	res := httptest.NewRecorder()
	h.runtimeProgress(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}
	var payload runtimeSettingsEnvelope
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != 0 {
		t.Fatalf("expected success code, got %d with msg %q", payload.Code, payload.Msg)
	}
	if payload.Data.RuntimeProgress.Phase != "rebuild_runtime" {
		t.Fatalf("expected rebuild_runtime phase, got %q", payload.Data.RuntimeProgress.Phase)
	}
	if payload.Data.RebuildProgress.State != string(backendruntime.ProgressStateRunning) {
		t.Fatalf("expected rebuild progress running, got %+v", payload.Data.RebuildProgress)
	}
	if payload.Data.RebuildProgress.Message != "rebuilding runtime objects" {
		t.Fatalf("expected rebuild message, got %+v", payload.Data.RebuildProgress)
	}
	if payload.Data.NodeSummary.Completed != 0 {
		t.Fatalf("expected zero completed nodes while switching, got %+v", payload.Data.NodeSummary)
	}
	if len(payload.Data.Warnings) != 1 || payload.Data.Warnings[0] != "skipped tunnel 7: missing chain node" {
		t.Fatalf("expected warnings to propagate, got %+v", payload.Data.Warnings)
	}
	if len(payload.Data.Nodes) != 1 || payload.Data.Nodes[0].Progress != string(backendruntime.ProgressStateRunning) {
		t.Fatalf("expected orchestrator node progress, got %+v", payload.Data.Nodes)
	}
}

type runtimeSettingsEnvelope struct {
	Code int                           `json:"code"`
	Msg  string                        `json:"msg"`
	Data runtimeSettingsPayloadForTest `json:"data"`
}

type runtimeSettingsPayloadForTest struct {
	CurrentEngine   string                       `json:"currentEngine"`
	SwitchStatus    string                       `json:"switchStatus"`
	Generation      int64                        `json:"generation"`
	LastError       string                       `json:"lastError"`
	NodeSummary     runtimeNodeSummaryForTest    `json:"nodeSummary"`
	RuntimeProgress runtimeProgressStatusForTest `json:"runtimeProgress"`
	RebuildProgress runtimeProgressStatusForTest `json:"rebuildProgress"`
	Warnings        []string                     `json:"warnings"`
	Nodes           []runtimeNodeStatusForTest   `json:"nodes"`
}

type runtimeNodeSummaryForTest struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Ready     int `json:"ready"`
	Pending   int `json:"pending"`
	Failed    int `json:"failed"`
}

type runtimeProgressStatusForTest struct {
	Engine  string `json:"engine"`
	State   string `json:"state"`
	Phase   string `json:"phase"`
	Message string `json:"message"`
}

type runtimeNodeStatusForTest struct {
	NodeID   int64  `json:"nodeId"`
	NodeName string `json:"nodeName"`
	Engine   string `json:"engine"`
	Ready    bool   `json:"ready"`
	Progress string `json:"progress"`
	Message  string `json:"message"`
}

type nodeListEnvelope struct {
	Code int                      `json:"code"`
	Msg  string                   `json:"msg"`
	Data []map[string]interface{} `json:"data"`
}

type stubRuntimeSwitchStarter struct {
	calls      int
	engine     backendruntime.Engine
	nodes      []repo.Node
	err        error
	progress   backendruntime.SwitchProgress
	switchFunc func(context.Context, backendruntime.Engine, []repo.Node) error
}

func (s *stubRuntimeSwitchStarter) Switch(ctx context.Context, engine backendruntime.Engine, nodes []repo.Node) error {
	s.calls++
	s.engine = engine
	s.nodes = append([]repo.Node(nil), nodes...)
	if s.switchFunc != nil {
		if err := s.switchFunc(ctx, engine, nodes); err != nil {
			return err
		}
	}
	return s.err
}

func (s *stubRuntimeSwitchStarter) Start(ctx context.Context, engine backendruntime.Engine, nodes []repo.Node) error {
	return s.Switch(ctx, engine, nodes)
}

func (s *stubRuntimeSwitchStarter) Progress() backendruntime.SwitchProgress {
	return s.progress
}

func TestNodeCheckStatusUsesCurrentRuntimeProvider(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	for _, node := range []model.Node{
		{Name: "node-a", Secret: "secret-a", ServerIP: "10.0.0.1", Port: "2024", CreatedTime: 1, Status: 1, TCPListenAddr: "[::]", UDPListenAddr: "[::]"},
		{Name: "node-b", Secret: "secret-b", ServerIP: "10.0.0.2", Port: "2025", CreatedTime: 1, Status: 1, TCPListenAddr: "[::]", UDPListenAddr: "[::]"},
	} {
		if err := r.DB().Create(&node).Error; err != nil {
			t.Fatalf("insert node %s: %v", node.Name, err)
		}
	}
	if err := r.SetRuntimeEngine(repo.RuntimeEngineDash, 10); err != nil {
		t.Fatalf("set runtime engine: %v", err)
	}

	h := New(r, "secret")
	h.runtimeStatusProviders = map[backendruntime.Engine]runtimeStatusProvider{
		backendruntime.EngineDash: runtimeStatusProviderFunc(func(_ context.Context, node repo.Node) (backendruntime.NodeRuntimeStatus, error) {
			if node.Name == "node-a" {
				return backendruntime.NodeRuntimeStatus{NodeID: node.ID, Engine: backendruntime.EngineDash, Ready: true, Progress: backendruntime.ProgressStateSucceeded}, nil
			}
			return backendruntime.NodeRuntimeStatus{NodeID: node.ID, Engine: backendruntime.EngineDash, Ready: false, Progress: backendruntime.ProgressStatePending, Message: "waiting for dash runtime"}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/node/check-status", nil)
	res := httptest.NewRecorder()
	h.nodeCheckStatus(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}
	var payload nodeListEnvelope
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != 0 {
		t.Fatalf("expected success code, got %d with msg %q", payload.Code, payload.Msg)
	}
	if len(payload.Data) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(payload.Data))
	}
	if payload.Data[0]["runtimeEngine"] != "dash" || payload.Data[0]["runtimeReady"] != true || payload.Data[0]["status"] != float64(1) {
		t.Fatalf("unexpected first node payload: %+v", payload.Data[0])
	}
	if payload.Data[1]["runtimeEngine"] != "dash" || payload.Data[1]["runtimeReady"] != false || payload.Data[1]["status"] != float64(0) {
		t.Fatalf("unexpected second node payload: %+v", payload.Data[1])
	}
	if payload.Data[1]["syncError"] != "waiting for dash runtime" {
		t.Fatalf("expected runtime status message to surface as syncError, got %+v", payload.Data[1])
	}
}

func TestNodeCheckStatusPersistsReadyStatusForDashEngine(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	if err := r.DB().Create(&model.Node{
		Name:          "node-a",
		Secret:        "secret-a",
		ServerIP:      "10.0.0.1",
		Port:          "2024",
		CreatedTime:   1,
		Status:        0,
		TCPListenAddr: "[::]",
		UDPListenAddr: "[::]",
	}).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	if err := r.SetRuntimeEngine(repo.RuntimeEngineDash, 10); err != nil {
		t.Fatalf("set runtime engine: %v", err)
	}

	h := New(r, "secret")
	h.runtimeStatusProviders = map[backendruntime.Engine]runtimeStatusProvider{
		backendruntime.EngineDash: runtimeStatusProviderFunc(func(_ context.Context, node repo.Node) (backendruntime.NodeRuntimeStatus, error) {
			return backendruntime.NodeRuntimeStatus{NodeID: node.ID, Engine: backendruntime.EngineDash, Ready: true, Progress: backendruntime.ProgressStateSucceeded, Message: "dash runtime ready"}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/node/check-status", nil)
	res := httptest.NewRecorder()
	h.nodeCheckStatus(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}
	node, err := r.GetNodeByID(1)
	if err != nil {
		t.Fatalf("GetNodeByID: %v", err)
	}
	if node == nil || node.Status != 1 {
		t.Fatalf("expected persisted node status=1 after dash readiness check, got %+v", node)
	}
}

func TestNodeCheckStatusPreservesExistingStatusForGostEngine(t *testing.T) {
	r, err := repo.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	if err := r.DB().Create(&model.Node{
		Name:          "node-a",
		Secret:        "secret-a",
		ServerIP:      "10.0.0.1",
		Port:          "2024",
		CreatedTime:   1,
		Status:        0,
		TCPListenAddr: "[::]",
		UDPListenAddr: "[::]",
	}).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	if err := r.SetRuntimeEngine(repo.RuntimeEngineGost, 10); err != nil {
		t.Fatalf("set runtime engine: %v", err)
	}

	h := New(r, "secret")
	h.runtimeStatusProviders = map[backendruntime.Engine]runtimeStatusProvider{
		backendruntime.EngineGost: runtimeStatusProviderFunc(func(_ context.Context, node repo.Node) (backendruntime.NodeRuntimeStatus, error) {
			return backendruntime.NodeRuntimeStatus{NodeID: node.ID, Engine: backendruntime.EngineGost, Ready: true, Progress: backendruntime.ProgressStateSucceeded}, nil
		}),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/node/check-status", nil)
	res := httptest.NewRecorder()
	h.nodeCheckStatus(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}
	var payload nodeListEnvelope
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != 0 {
		t.Fatalf("expected success code, got %d with msg %q", payload.Code, payload.Msg)
	}
	if len(payload.Data) != 1 {
		t.Fatalf("expected 1 node, got %d", len(payload.Data))
	}
	if payload.Data[0]["status"] != float64(0) {
		t.Fatalf("expected gost mode to preserve original status, got %+v", payload.Data[0])
	}
	if _, ok := payload.Data[0]["runtimeEngine"]; ok {
		t.Fatalf("did not expect gost mode runtime overlay, got %+v", payload.Data[0])
	}
}

type runtimeStatusProviderFunc func(context.Context, repo.Node) (backendruntime.NodeRuntimeStatus, error)

func (f runtimeStatusProviderFunc) GetNodeRuntimeStatus(ctx context.Context, node repo.Node) (backendruntime.NodeRuntimeStatus, error) {
	return f(ctx, node)
}
