package runtime

import (
	"context"
	"fmt"
	"time"

	"go-backend/internal/store/repo"
	"go-backend/internal/ws"
)

type gostCommandSender interface {
	SendCommand(nodeID int64, cmdType string, data interface{}, timeout time.Duration) (ws.CommandResult, error)
}

type GostRuntimeClient struct {
	engine    Engine
	commander gostCommandSender
}

func NewGostRuntimeClient(engine Engine, commander ...gostCommandSender) *GostRuntimeClient {
	client := &GostRuntimeClient{engine: engine}
	if len(commander) > 0 {
		client.commander = commander[0]
	}
	return client
}

func (c *GostRuntimeClient) EnsureNodeRuntime(ctx context.Context, node repo.Node) (NodeRuntimeProgress, error) {
	if c.commander != nil {
		_, err := c.commander.SendCommand(node.ID, "SetEngine", map[string]interface{}{"engine": string(c.engine)}, 15*time.Second)
		if err != nil {
			return NodeRuntimeProgress{Engine: c.engine, State: ProgressStateFailed, Message: err.Error()}, err
		}
	}
	return NodeRuntimeProgress{Engine: c.engine, State: ProgressStateSucceeded, Complete: true}, nil
}

func (c *GostRuntimeClient) RebuildAllRuntime(context.Context) (RebuildRuntimeProgress, error) {
	return RebuildRuntimeProgress{Engine: c.engine, State: ProgressStateSucceeded, Complete: true}, nil
}

func (c *GostRuntimeClient) GetNodeRuntimeStatus(context.Context, repo.Node) (NodeRuntimeStatus, error) {
	return NodeRuntimeStatus{Engine: c.engine, Ready: true, Progress: ProgressStateSucceeded}, nil
}

func (c *GostRuntimeClient) PauseServices(ctx context.Context, node repo.Node, services []string) error {
	return c.controlServices(ctx, node, "PauseService", services)
}

func (c *GostRuntimeClient) ResumeServices(ctx context.Context, node repo.Node, services []string) error {
	return c.controlServices(ctx, node, "ResumeService", services)
}

func (c *GostRuntimeClient) CheckService(ctx context.Context, node repo.Node, req ServiceCheckRequest) (ServiceCheckResult, error) {
	if c.commander == nil {
		return ServiceCheckResult{}, fmt.Errorf("gost runtime commander unavailable")
	}
	cmdRes, err := c.commander.SendCommand(node.ID, "ServiceMonitorCheck", map[string]interface{}{
		"monitorId":  req.MonitorID,
		"type":       req.Type,
		"target":     req.Target,
		"timeoutSec": req.TimeoutSec,
	}, serviceMonitorTimeout(ctx, req.TimeoutSec))
	if err != nil {
		return ServiceCheckResult{}, err
	}
	return serviceCheckResultFromCommand(cmdRes), nil
}

func (c *GostRuntimeClient) controlServices(ctx context.Context, node repo.Node, commandType string, services []string) error {
	if c.commander == nil {
		return fmt.Errorf("gost runtime commander unavailable")
	}
	_, err := c.commander.SendCommand(node.ID, commandType, map[string]interface{}{"services": services}, serviceControlTimeout(ctx))
	return err
}

func serviceControlTimeout(ctx context.Context) time.Duration {
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 {
			return remaining
		}
	}
	return 6 * time.Second
}

func serviceMonitorTimeout(ctx context.Context, timeoutSec int) time.Duration {
	timeout := time.Duration(timeoutSec) * time.Second
	if timeout < 2*time.Second {
		timeout = 2 * time.Second
	}
	timeout += 2 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			return remaining
		}
	}
	return timeout
}

func serviceCheckResultFromCommand(cmdRes ws.CommandResult) ServiceCheckResult {
	result := ServiceCheckResult{}
	if cmdRes.Data == nil {
		result.ErrorMessage = "node returned empty response"
		return result
	}
	if v, ok := cmdRes.Data["success"].(bool); ok {
		result.Success = v
	}
	if v, ok := cmdRes.Data["latencyMs"].(float64); ok {
		result.LatencyMs = v
	}
	if v, ok := cmdRes.Data["statusCode"].(float64); ok {
		result.StatusCode = int(v)
	}
	if v, ok := cmdRes.Data["errorMessage"].(string); ok {
		result.ErrorMessage = v
	}
	return result
}
