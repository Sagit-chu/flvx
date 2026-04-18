package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"go-backend/internal/store/repo"
)

func TestSwitchOrchestratorRejectsConcurrentSwitches(t *testing.T) {
	r, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	client := &stubRuntimeClient{
		ensureStarted: make(chan struct{}, 1),
		ensureBlock:   make(chan struct{}),
		done:          make(chan struct{}),
	}
	orchestrator := NewSwitchOrchestrator(r, func() int64 { return 100 }, map[Engine]RuntimeClient{
		EngineDash: client,
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- orchestrator.Switch(context.Background(), EngineDash, []repo.Node{{ID: 1}})
	}()

	<-client.ensureStarted

	err = orchestrator.Switch(context.Background(), EngineDash, []repo.Node{{ID: 2}})
	if !errors.Is(err, ErrSwitchInProgress) {
		t.Fatalf("expected ErrSwitchInProgress, got %v", err)
	}

	close(client.ensureBlock)

	if err := <-errCh; err != nil {
		t.Fatalf("first switch: %v", err)
	}
}

func TestSwitchOrchestratorStartRunsAsynchronouslyAndPublishesProgress(t *testing.T) {
	r, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	client := &stubRuntimeClient{
		ensureStarted: make(chan struct{}, 1),
		ensureBlock:   make(chan struct{}),
	}
	orchestrator := NewSwitchOrchestrator(r, func() int64 { return 500 }, map[Engine]RuntimeClient{
		EngineDash: client,
	})

	if err := orchestrator.Start(context.Background(), EngineDash, []repo.Node{{ID: 1, Name: "node-a"}}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	<-client.ensureStarted

	progress := orchestrator.Progress()
	if progress.Engine != EngineDash {
		t.Fatalf("expected dash progress engine, got %q", progress.Engine)
	}
	if progress.Status != RuntimeSwitchStatusSwitching {
		t.Fatalf("expected switching status, got %q", progress.Status)
	}
	if progress.Phase != "deploy_nodes" {
		t.Fatalf("expected deploy_nodes phase, got %q", progress.Phase)
	}
	if len(progress.Nodes) != 1 || progress.Nodes[0].State != ProgressStateRunning {
		t.Fatalf("unexpected node progress: %+v", progress.Nodes)
	}

	close(client.ensureBlock)
	progress = waitForSwitchProgressState(t, orchestrator, RuntimeSwitchStatusIdle)
	if progress.Status != RuntimeSwitchStatusIdle {
		t.Fatalf("expected idle status after completion, got %q", progress.Status)
	}
	if progress.Phase != "completed" {
		t.Fatalf("expected completed phase, got %q", progress.Phase)
	}
	if len(progress.Nodes) != 1 || progress.Nodes[0].State != ProgressStateSucceeded {
		t.Fatalf("unexpected completed node progress: %+v", progress.Nodes)
	}
}

func TestSwitchOrchestratorMarksFailedOnNodeDeploymentError(t *testing.T) {
	r, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	client := &stubRuntimeClient{
		ensureErr: errors.New("deploy failed"),
	}
	orchestrator := NewSwitchOrchestrator(r, func() int64 { return 200 }, map[Engine]RuntimeClient{
		EngineDash: client,
	})

	err = orchestrator.Switch(context.Background(), EngineDash, []repo.Node{{ID: 1}})
	if err == nil {
		t.Fatal("expected switch error")
	}

	state, err := r.GetRuntimeSwitchState()
	if err != nil {
		t.Fatalf("get runtime switch state: %v", err)
	}
	if state.Status != RuntimeSwitchStatusFailed {
		t.Fatalf("expected failed status, got %q", state.Status)
	}
	if state.Generation != 1 {
		t.Fatalf("expected generation 1, got %d", state.Generation)
	}
	if state.Error != "deploy failed" {
		t.Fatalf("expected deploy error to be persisted, got %q", state.Error)
	}
}

func TestSwitchOrchestratorSucceedsAndPersistsSelectedEngine(t *testing.T) {
	r, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	if err := r.DB().Create(&repo.Node{Name: "node-a", Secret: "secret-a", ServerIP: "10.0.0.1", Port: "2024", CreatedTime: 1, Status: 1}).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}
	client := &stubRuntimeClient{}
	orchestrator := NewSwitchOrchestrator(r, func() int64 { return 400 }, map[Engine]RuntimeClient{
		EngineDash: client,
	})

	err = orchestrator.Switch(context.Background(), EngineDash, []repo.Node{{ID: 1, Name: "node-a", Secret: "secret-a", ServerIP: "10.0.0.1", Port: "2024", Status: 1}})
	if err != nil {
		t.Fatalf("Switch: %v", err)
	}

	engine, err := r.GetRuntimeEngine()
	if err != nil {
		t.Fatalf("GetRuntimeEngine: %v", err)
	}
	if engine != repo.RuntimeEngineDash {
		t.Fatalf("expected persisted engine dash, got %q", engine)
	}
	state, err := r.GetRuntimeSwitchState()
	if err != nil {
		t.Fatalf("GetRuntimeSwitchState: %v", err)
	}
	if state.Status != RuntimeSwitchStatusIdle {
		t.Fatalf("expected idle status, got %q", state.Status)
	}
	if state.Generation != 1 {
		t.Fatalf("expected generation 1, got %d", state.Generation)
	}
	if client.ensureErr != nil {
		t.Fatalf("expected successful node ensure, got %v", client.ensureErr)
	}
}

func TestSwitchOrchestratorMarksFailedOnFinalPersistenceError(t *testing.T) {
	store := &failingSwitchStore{
		state:         repo.RuntimeSwitchState{Status: repo.RuntimeSwitchStatusIdle},
		failSetEngine: true,
	}
	orchestrator := NewSwitchOrchestrator(store, func() int64 { return 300 }, map[Engine]RuntimeClient{
		EngineDash: &stubRuntimeClient{},
	})

	err := orchestrator.Switch(context.Background(), EngineDash, []repo.Node{{ID: 1}})
	if err == nil {
		t.Fatal("expected switch error")
	}
	if !errors.Is(err, errSetEngineFailed) {
		t.Fatalf("expected engine persistence error, got %v", err)
	}
	if store.state.Status != RuntimeSwitchStatusFailed {
		t.Fatalf("expected failed status, got %q", store.state.Status)
	}
	if store.state.Generation != 1 {
		t.Fatalf("expected generation 1, got %d", store.state.Generation)
	}
	if store.state.Error != errSetEngineFailed.Error() {
		t.Fatalf("expected persisted error %q, got %q", errSetEngineFailed.Error(), store.state.Error)
	}
	if len(store.stateWrites) < 2 {
		t.Fatalf("expected switching and failed writes, got %d", len(store.stateWrites))
		t.Fatal("expected explanatory status message")
	}
}

type stubRuntimeClient struct {
	ensureStarted chan struct{}
	ensureBlock   chan struct{}
	ensureErr     error
	rebuildBlock  chan struct{}
	done          chan struct{}
}

func (c *stubRuntimeClient) EnsureNodeRuntime(ctx context.Context, node repo.Node) (NodeRuntimeProgress, error) {
	if c.ensureStarted != nil {
		select {
		case c.ensureStarted <- struct{}{}:
		default:
		}
	}
	if c.ensureBlock != nil {
		select {
		case <-ctx.Done():
			return NodeRuntimeProgress{}, ctx.Err()
		case <-c.ensureBlock:
		}
	}
	return NodeRuntimeProgress{NodeID: node.ID}, c.ensureErr
}

func (c *stubRuntimeClient) RebuildAllRuntime(context.Context) (RebuildRuntimeProgress, error) {
	if c.rebuildBlock != nil {
		<-c.rebuildBlock
	}
	if c.done != nil {
		close(c.done)
	}
	return RebuildRuntimeProgress{}, nil
}

func (c *stubRuntimeClient) GetNodeRuntimeStatus(context.Context, repo.Node) (NodeRuntimeStatus, error) {
	return NodeRuntimeStatus{}, nil
}

func (c *stubRuntimeClient) PauseServices(context.Context, repo.Node, []string) error {
	return nil
}

func (c *stubRuntimeClient) ResumeServices(context.Context, repo.Node, []string) error {
	return nil
}

func (c *stubRuntimeClient) CheckService(context.Context, repo.Node, ServiceCheckRequest) (ServiceCheckResult, error) {
	return ServiceCheckResult{}, nil
}

func (c *stubRuntimeClient) waitForDone(t *testing.T) {
	t.Helper()
	if c.done == nil {
		return
	}
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async switch completion")
	}
}

func waitForSwitchProgressState(t *testing.T, orchestrator *SwitchOrchestrator, want SwitchStatus) SwitchProgress {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		progress := orchestrator.Progress()
		if progress.Status == want {
			return progress
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for switch status %q", want)
	return SwitchProgress{}
}

var errSetEngineFailed = errors.New("persist engine failed")

type failingSwitchStore struct {
	state         repo.RuntimeSwitchState
	stateWrites   []repo.RuntimeSwitchState
	engine        repo.RuntimeEngine
	failSetEngine bool
}

func (s *failingSwitchStore) GetRuntimeSwitchState() (repo.RuntimeSwitchState, error) {
	return s.state, nil
}

func (s *failingSwitchStore) SetRuntimeSwitchState(state repo.RuntimeSwitchState, _ int64) error {
	s.state = state
	s.stateWrites = append(s.stateWrites, state)
	return nil
}

func (s *failingSwitchStore) SetRuntimeEngine(engine repo.RuntimeEngine, _ int64) error {
	if s.failSetEngine {
		return errSetEngineFailed
	}
	s.engine = engine
	return nil
}
