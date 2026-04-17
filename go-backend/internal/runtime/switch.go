package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go-backend/internal/store/repo"
)

var ErrSwitchInProgress = errors.New("runtime switch already in progress")

type switchStateStore interface {
	GetRuntimeSwitchState() (repo.RuntimeSwitchState, error)
	SetRuntimeSwitchState(state repo.RuntimeSwitchState, now int64) error
	SetRuntimeEngine(engine repo.RuntimeEngine, now int64) error
}

type SwitchOrchestrator struct {
	mu       sync.Mutex
	active   bool
	store    switchStateStore
	now      func() int64
	clients  map[Engine]RuntimeClient
	progress SwitchProgress
}

func NewSwitchOrchestrator(store switchStateStore, now func() int64, clients map[Engine]RuntimeClient) *SwitchOrchestrator {
	if now == nil {
		now = func() int64 { return 0 }
	}
	return &SwitchOrchestrator{
		store:   store,
		now:     now,
		clients: clients,
	}
}

func (o *SwitchOrchestrator) Switch(ctx context.Context, engine Engine, nodes []repo.Node) error {
	if err := o.beginSwitch(engine, nodes); err != nil {
		return err
	}
	defer o.finishSwitch()
	return o.runSwitch(ctx, engine, nodes)
}

func (o *SwitchOrchestrator) Start(ctx context.Context, engine Engine, nodes []repo.Node) error {
	if err := o.beginSwitch(engine, nodes); err != nil {
		return err
	}
	go func() {
		defer o.finishSwitch()
		_ = o.runSwitch(ctx, engine, nodes)
	}()
	return nil
}

func (o *SwitchOrchestrator) Progress() SwitchProgress {
	o.mu.Lock()
	defer o.mu.Unlock()
	progress := o.progress
	progress.Nodes = append([]NodeRuntimeProgress(nil), progress.Nodes...)
	return progress
}

func (o *SwitchOrchestrator) beginSwitch(engine Engine, nodes []repo.Node) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.active {
		return ErrSwitchInProgress
	}
	o.active = true
	o.progress = SwitchProgress{
		Engine:  engine,
		Status:  RuntimeSwitchStatusSwitching,
		Phase:   "deploy_nodes",
		Message: "starting runtime switch",
		Nodes:   make([]NodeRuntimeProgress, 0, len(nodes)),
		Rebuild: RebuildRuntimeProgress{Engine: engine, State: ProgressStatePending, Message: "waiting to rebuild runtime", Complete: false},
	}
	for _, node := range nodes {
		o.progress.Nodes = append(o.progress.Nodes, NodeRuntimeProgress{
			NodeID:   node.ID,
			Engine:   engine,
			State:    ProgressStatePending,
			Message:  "pending",
			Complete: false,
		})
	}
	return nil
}

func (o *SwitchOrchestrator) finishSwitch() {
	o.mu.Lock()
	o.active = false
	o.mu.Unlock()
}

func (o *SwitchOrchestrator) runSwitch(ctx context.Context, engine Engine, nodes []repo.Node) error {
	client, ok := o.clients[engine]
	if !ok {
		o.setFailureState(0, fmt.Sprintf("runtime client unavailable for engine %q", engine))
		return fmt.Errorf("runtime client unavailable for engine %q", engine)
	}

	state, err := o.store.GetRuntimeSwitchState()
	if err != nil {
		o.setFailureState(0, err.Error())
		return err
	}
	state.Generation++
	state.Status = RuntimeSwitchStatusSwitching
	state.Error = ""
	o.setGeneration(state.Generation)

	if err := o.store.SetRuntimeSwitchState(state, o.now()); err != nil {
		o.setFailureState(state.Generation, err.Error())
		return err
	}

	for index, node := range nodes {
		o.setNodeProgress(index, NodeRuntimeProgress{
			NodeID:   node.ID,
			Engine:   engine,
			State:    ProgressStateRunning,
			Message:  "deploying runtime",
			Complete: false,
		})
		progress, err := client.EnsureNodeRuntime(ctx, node)
		if err != nil {
			o.setNodeProgress(index, NodeRuntimeProgress{
				NodeID:   node.ID,
				Engine:   engine,
				State:    ProgressStateFailed,
				Message:  err.Error(),
				Complete: false,
			})
			return o.fail(state.Generation, err)
		}
		if progress.NodeID == 0 {
			progress.NodeID = node.ID
		}
		if progress.Engine == "" {
			progress.Engine = engine
		}
		if progress.State == "" {
			progress.State = ProgressStateSucceeded
		}
		if progress.Message == "" && progress.State == ProgressStateSucceeded {
			progress.Message = "runtime ready"
		}
		progress.Complete = progress.State == ProgressStateSucceeded
		o.setNodeProgress(index, progress)
	}

	o.setPhase("rebuild_runtime", "rebuilding runtime objects")
	rebuildProgress, err := client.RebuildAllRuntime(ctx)
	if err != nil {
		o.setRebuildProgress(RebuildRuntimeProgress{Engine: engine, State: ProgressStateFailed, Message: err.Error(), Complete: false})
		return o.fail(state.Generation, err)
	}
	if rebuildProgress.Engine == "" {
		rebuildProgress.Engine = engine
	}
	if rebuildProgress.State == "" {
		rebuildProgress.State = ProgressStateSucceeded
	}
	rebuildProgress.Complete = rebuildProgress.State == ProgressStateSucceeded
	o.setRebuildProgress(rebuildProgress)

	o.setPhase("persist_runtime_engine", "persisting selected engine")
	if err := o.store.SetRuntimeEngine(repo.RuntimeEngine(engine), o.now()); err != nil {
		return o.fail(state.Generation, err)
	}

	if err := o.store.SetRuntimeSwitchState(repo.RuntimeSwitchState{
		Status:     RuntimeSwitchStatusIdle,
		Generation: state.Generation,
		Error:      "",
	}, o.now()); err != nil {
		return o.fail(state.Generation, err)
	}
	o.setCompleted(state.Generation)
	return nil
}

func (o *SwitchOrchestrator) fail(generation int64, cause error) error {
	o.setFailureState(generation, cause.Error())
	stateErr := o.store.SetRuntimeSwitchState(repo.RuntimeSwitchState{
		Status:     RuntimeSwitchStatusFailed,
		Generation: generation,
		Error:      cause.Error(),
	}, o.now())
	if stateErr != nil {
		return errors.Join(cause, stateErr)
	}
	return cause
}

func (o *SwitchOrchestrator) setGeneration(generation int64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.progress.Generation = generation
}

func (o *SwitchOrchestrator) setPhase(phase, message string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.progress.Phase = phase
	o.progress.Message = message
	o.progress.Status = RuntimeSwitchStatusSwitching
	if phase == "rebuild_runtime" {
		o.progress.Rebuild = RebuildRuntimeProgress{
			Engine:   o.progress.Engine,
			State:    ProgressStateRunning,
			Message:  message,
			Complete: false,
		}
	}
}

func (o *SwitchOrchestrator) setNodeProgress(index int, progress NodeRuntimeProgress) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if index >= 0 && index < len(o.progress.Nodes) {
		o.progress.Nodes[index] = progress
	}
}

func (o *SwitchOrchestrator) setRebuildProgress(progress RebuildRuntimeProgress) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.progress.Rebuild = progress
}

func (o *SwitchOrchestrator) setCompleted(generation int64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.progress.Generation = generation
	o.progress.Status = RuntimeSwitchStatusIdle
	o.progress.Phase = "completed"
	o.progress.Message = "runtime switch completed"
	for i := range o.progress.Nodes {
		if o.progress.Nodes[i].State == ProgressStateRunning || o.progress.Nodes[i].State == ProgressStatePending {
			o.progress.Nodes[i].State = ProgressStateSucceeded
			o.progress.Nodes[i].Complete = true
			if o.progress.Nodes[i].Message == "" || o.progress.Nodes[i].Message == "pending" || o.progress.Nodes[i].Message == "deploying runtime" {
				o.progress.Nodes[i].Message = "runtime ready"
			}
		}
	}
	if o.progress.Rebuild.State == ProgressStatePending || o.progress.Rebuild.State == ProgressStateRunning {
		o.progress.Rebuild.State = ProgressStateSucceeded
		o.progress.Rebuild.Complete = true
		if o.progress.Rebuild.Message == "" || o.progress.Rebuild.Message == "rebuilding runtime objects" || o.progress.Rebuild.Message == "waiting to rebuild runtime" {
			o.progress.Rebuild.Message = "runtime objects rebuilt"
		}
	}
}

func (o *SwitchOrchestrator) setFailureState(generation int64, message string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.progress.Generation = generation
	o.progress.Status = RuntimeSwitchStatusFailed
	o.progress.Phase = "failed"
	o.progress.Message = message
	if o.progress.Rebuild.State == ProgressStateRunning || o.progress.Rebuild.State == ProgressStatePending {
		o.progress.Rebuild.State = ProgressStateFailed
		o.progress.Rebuild.Message = message
		if o.progress.Rebuild.Engine == "" {
			o.progress.Rebuild.Engine = o.progress.Engine
		}
	}
}
