package runtime

import (
	"context"
	"fmt"

	"go-backend/internal/store/repo"
)

type Engine = repo.RuntimeEngine

const (
	EngineGost = repo.RuntimeEngineGost
	EngineDash = repo.RuntimeEngineDash
)

type SwitchStatus = repo.RuntimeSwitchStatus

const (
	RuntimeSwitchStatusIdle      = repo.RuntimeSwitchStatusIdle
	RuntimeSwitchStatusSwitching = repo.RuntimeSwitchStatusSwitching
	RuntimeSwitchStatusFailed    = repo.RuntimeSwitchStatusFailed
)

type SwitchState = repo.RuntimeSwitchState

type ProgressState string

const (
	ProgressStatePending   ProgressState = "pending"
	ProgressStateRunning   ProgressState = "running"
	ProgressStateSucceeded ProgressState = "succeeded"
	ProgressStateFailed    ProgressState = "failed"
)

type NodeRuntimeProgress struct {
	NodeID   int64
	State    ProgressState
	Message  string
	Engine   Engine
	Complete bool
}

type RebuildRuntimeProgress struct {
	State    ProgressState
	Message  string
	Warnings []string
	Engine   Engine
	Complete bool
}

type SwitchProgress struct {
	Engine     Engine
	Status     SwitchStatus
	Generation int64
	Phase      string
	Message    string
	Nodes      []NodeRuntimeProgress
	Rebuild    RebuildRuntimeProgress
}

type NodeRuntimeStatus struct {
	NodeID     int64
	Engine     Engine
	Ready      bool
	Message    string
	Progress   ProgressState
	ActiveExit *ActiveExitStatus
}

type ActiveExitStatus struct {
	Server string
	Token  string
}

type ServiceCheckRequest struct {
	MonitorID  int64
	Type       string
	Target     string
	TimeoutSec int
}

type ServiceCheckResult struct {
	Success      bool
	LatencyMs    float64
	StatusCode   int
	ErrorMessage string
}

type ForwardApplyStatus string

const (
	ForwardApplyStatusSuccess        ForwardApplyStatus = "success"
	ForwardApplyStatusPartialSuccess ForwardApplyStatus = "partial_success"
	ForwardApplyStatusFailed         ForwardApplyStatus = "failed"
)

type ForwardProtocolApplyResult struct {
	Protocol string
	RuleID   string
	Status   ForwardApplyStatus
	Message  string
}

type ForwardApplyResult struct {
	ForwardID int64
	NodeID    int64
	Port      int
	Status    ForwardApplyStatus
	Protocols []ForwardProtocolApplyResult
	Warnings  []string
}

func (r ForwardApplyResult) Error() string {
	switch r.Status {
	case ForwardApplyStatusPartialSuccess:
		return fmt.Sprintf("forward %d node %d port %d applied partially", r.ForwardID, r.NodeID, r.Port)
	case ForwardApplyStatusFailed:
		return fmt.Sprintf("forward %d node %d port %d failed", r.ForwardID, r.NodeID, r.Port)
	default:
		return ""
	}
}

type RuntimeClient interface {
	EnsureNodeRuntime(ctx context.Context, node repo.Node) (NodeRuntimeProgress, error)
	RebuildAllRuntime(ctx context.Context) (RebuildRuntimeProgress, error)
	GetNodeRuntimeStatus(ctx context.Context, node repo.Node) (NodeRuntimeStatus, error)
	PauseServices(ctx context.Context, node repo.Node, services []string) error
	ResumeServices(ctx context.Context, node repo.Node, services []string) error
	CheckService(ctx context.Context, node repo.Node, req ServiceCheckRequest) (ServiceCheckResult, error)
}
