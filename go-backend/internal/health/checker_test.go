package health

import (
	"context"
	"net"
	"testing"
	"time"

	"go-backend/internal/monitoring"
	backendruntime "go-backend/internal/runtime"
	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
	"go-backend/internal/ws"
)

type fakeCommander struct {
	lastNodeID int64
	lastType   string
	lastData   interface{}
	res        ws.CommandResult
	err        error
	calls      int
}

type fakeRuntimeClient struct {
	lastNode repo.Node
	lastReq  backendruntime.ServiceCheckRequest
	res      backendruntime.ServiceCheckResult
	err      error
	calls    int
}

type delayedCommander struct {
	delayByMonitorID map[int64]time.Duration
}

func (d *delayedCommander) SendCommand(nodeID int64, cmdType string, data interface{}, _ time.Duration) (ws.CommandResult, error) {
	_ = nodeID
	_ = cmdType
	if req, ok := data.(serviceMonitorCheckRequest); ok {
		if delay := d.delayByMonitorID[req.MonitorID]; delay > 0 {
			time.Sleep(delay)
		}
	}
	return ws.CommandResult{
		Success: true,
		Data: map[string]interface{}{
			"success":   true,
			"latencyMs": float64(1),
		},
	}, nil
}

func (f *fakeCommander) SendCommand(nodeID int64, cmdType string, data interface{}, _ time.Duration) (ws.CommandResult, error) {
	f.calls++
	f.lastNodeID = nodeID
	f.lastType = cmdType
	f.lastData = data
	return f.res, f.err
}

func (f *fakeRuntimeClient) EnsureNodeRuntime(context.Context, repo.Node) (backendruntime.NodeRuntimeProgress, error) {
	return backendruntime.NodeRuntimeProgress{}, nil
}

func (f *fakeRuntimeClient) RebuildAllRuntime(context.Context) (backendruntime.RebuildRuntimeProgress, error) {
	return backendruntime.RebuildRuntimeProgress{}, nil
}

func (f *fakeRuntimeClient) GetNodeRuntimeStatus(context.Context, repo.Node) (backendruntime.NodeRuntimeStatus, error) {
	return backendruntime.NodeRuntimeStatus{}, nil
}

func (f *fakeRuntimeClient) PauseServices(context.Context, repo.Node, []string) error {
	return nil
}

func (f *fakeRuntimeClient) ResumeServices(context.Context, repo.Node, []string) error {
	return nil
}

func (f *fakeRuntimeClient) CheckService(_ context.Context, node repo.Node, req backendruntime.ServiceCheckRequest) (backendruntime.ServiceCheckResult, error) {
	f.calls++
	f.lastNode = node
	f.lastReq = req
	return f.res, f.err
}

func TestCheckerUsesSelectedRuntimeEngineForServiceCheck(t *testing.T) {
	r, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	if err := r.DB().Exec(`
		INSERT INTO node(id, name, secret, server_ip, server_ip_v4, server_ip_v6, port, interface_name, version, http, tls, socks, created_time, updated_time, status, tcp_listen_addr, udp_listen_addr, inx, is_remote)
		VALUES(7, 'node-a', 'secret-a', '10.0.0.7', '10.0.0.7', '', '2000-2010', '', 'v1', 1, 1, 1, 1, 1, 1, '[::]', '[::]', 0, 0)
	`).Error; err != nil {
		t.Fatalf("insert node: %v", err)
	}

	commander := &fakeCommander{}
	runtimeClient := &fakeRuntimeClient{res: backendruntime.ServiceCheckResult{Success: true, LatencyMs: 42, StatusCode: 204}}
	checker := NewChecker(r, commander, func() backendruntime.RuntimeClient { return runtimeClient })
	limits := checker.loadServiceMonitorLimits()
	now := time.Now().UnixMilli()
	monitor := &model.ServiceMonitor{
		ID:         11,
		Type:       "tcp",
		Target:     "example.com:443",
		TimeoutSec: 5,
		NodeID:     7,
	}

	result := checker.executeCheck(monitor, now, limits)

	if runtimeClient.calls != 1 {
		t.Fatalf("expected runtime client to be called once, got %d", runtimeClient.calls)
	}
	if commander.calls != 0 {
		t.Fatalf("expected websocket commander to be bypassed, got %d calls", commander.calls)
	}
	if runtimeClient.lastNode.ID != 7 {
		t.Fatalf("expected node id 7, got %d", runtimeClient.lastNode.ID)
	}
	if runtimeClient.lastReq.MonitorID != 11 || runtimeClient.lastReq.Type != "tcp" || runtimeClient.lastReq.Target != "example.com:443" || runtimeClient.lastReq.TimeoutSec != 5 {
		t.Fatalf("unexpected runtime check request: %+v", runtimeClient.lastReq)
	}
	if result.Success != 1 || result.LatencyMs != 42 || result.StatusCode != 204 {
		t.Fatalf("unexpected runtime check result: %+v", result)
	}
}

func TestTCPHealthCheckViaMonitor(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	addr := listener.Addr().String()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	t.Run("successful tcp check", func(t *testing.T) {
		checker := NewChecker(nil, nil)
		limits := checker.loadServiceMonitorLimits()
		now := time.Now().UnixMilli()
		monitor := &model.ServiceMonitor{
			Type:       "tcp",
			Target:     addr,
			TimeoutSec: 5,
		}
		result := checker.executeCheck(monitor, now, limits)
		if result.Success != 1 {
			t.Fatalf("expected success, got error: %s", result.ErrorMessage)
		}
		if result.LatencyMs < 0 {
			t.Fatalf("expected non-negative latency, got %f", result.LatencyMs)
		}
	})

	t.Run("failed tcp check - connection refused", func(t *testing.T) {
		checker := NewChecker(nil, nil)
		limits := checker.loadServiceMonitorLimits()
		now := time.Now().UnixMilli()
		monitor := &model.ServiceMonitor{
			Type:       "tcp",
			Target:     "127.0.0.1:1",
			TimeoutSec: 1,
		}
		result := checker.executeCheck(monitor, now, limits)
		if result.Success == 1 {
			t.Fatalf("expected failure for connection refused")
		}
		if result.ErrorMessage == "" {
			t.Fatalf("expected error message")
		}
	})
}

func TestCheckerRunChecks(t *testing.T) {
	r, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	tcpAddr := listener.Addr().String()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	now := time.Now().UnixMilli()

	monitors := []*model.ServiceMonitor{
		{
			Name:        "TCP Monitor",
			Type:        "tcp",
			Target:      tcpAddr,
			IntervalSec: 60,
			TimeoutSec:  5,
			NodeID:      0,
			Enabled:     1,
			CreatedTime: now,
			UpdatedTime: now,
		},
		{
			Name:        "TCP Monitor 2",
			Type:        "tcp",
			Target:      tcpAddr,
			IntervalSec: 60,
			TimeoutSec:  5,
			NodeID:      0,
			Enabled:     1,
			CreatedTime: now,
			UpdatedTime: now,
		},
		{
			Name:        "Disabled Monitor",
			Type:        "tcp",
			Target:      "127.0.0.1:1",
			IntervalSec: 60,
			TimeoutSec:  5,
			NodeID:      0,
			Enabled:     0,
			CreatedTime: now,
			UpdatedTime: now,
		},
	}

	for _, m := range monitors {
		if err := r.CreateServiceMonitor(m); err != nil {
			t.Fatalf("create monitor: %v", err)
		}
	}

	monitors[2].Enabled = 0
	if err := r.UpdateServiceMonitor(monitors[2]); err != nil {
		t.Fatalf("update disabled monitor: %v", err)
	}

	enabledMonitors, err := r.ListEnabledServiceMonitors()
	if err != nil {
		t.Fatalf("list enabled monitors: %v", err)
	}
	if len(enabledMonitors) != 2 {
		t.Fatalf("expected 2 enabled monitors, got %d", len(enabledMonitors))
	}

	checker := NewChecker(r, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go checker.Start(ctx)
	time.Sleep(500 * time.Millisecond)

	results, err := r.GetServiceMonitorResults(monitors[0].ID, 10)
	if err != nil {
		t.Fatalf("get tcp results: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected at least one result for tcp monitor")
	}
	for _, res := range results {
		if res.Success != 1 {
			t.Fatalf("expected success for tcp monitor, got failure: %s", res.ErrorMessage)
		}
	}

	results2, err := r.GetServiceMonitorResults(monitors[1].ID, 10)
	if err != nil {
		t.Fatalf("get tcp results 2: %v", err)
	}
	if len(results2) == 0 {
		t.Fatalf("expected at least one result for tcp monitor 2")
	}
	for _, res := range results2 {
		if res.Success != 1 {
			t.Fatalf("expected success for tcp monitor 2, got failure: %s", res.ErrorMessage)
		}
	}

	disabledResults, err := r.GetServiceMonitorResults(monitors[2].ID, 10)
	if err != nil {
		t.Fatalf("get disabled results: %v", err)
	}
	if len(disabledResults) != 0 {
		t.Fatalf("expected no results for disabled monitor, got %d", len(disabledResults))
	}
}

func TestCheckerUnsupportedType(t *testing.T) {
	checker := NewChecker(nil, nil)
	limits := checker.loadServiceMonitorLimits()
	now := time.Now().UnixMilli()
	monitor := &model.ServiceMonitor{
		Type:       "http",
		Target:     "https://example.com",
		TimeoutSec: 5,
	}
	result := checker.executeCheck(monitor, now, limits)
	if result.Success == 1 {
		t.Fatalf("expected failure for unsupported type")
	}
	if result.ErrorMessage == "" {
		t.Fatalf("expected error message for unsupported type")
	}
}

func TestCheckerDefaultTimeout(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	addr := listener.Addr().String()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	checker := NewChecker(nil, nil)
	limits := checker.loadServiceMonitorLimits()
	now := time.Now().UnixMilli()
	monitor := &model.ServiceMonitor{
		Type:       "tcp",
		Target:     addr,
		TimeoutSec: 0,
	}
	result := checker.executeCheck(monitor, now, limits)
	if result.Success != 1 {
		t.Fatalf("expected success with default timeout, got error: %s", result.ErrorMessage)
	}
}

func TestCheckerStop(t *testing.T) {
	r, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	now := time.Now().UnixMilli()
	monitor := &model.ServiceMonitor{
		Name:        "Test Monitor",
		Type:        "tcp",
		Target:      listener.Addr().String(),
		IntervalSec: 60,
		TimeoutSec:  5,
		NodeID:      0,
		Enabled:     1,
		CreatedTime: now,
		UpdatedTime: now,
	}
	if err := r.CreateServiceMonitor(monitor); err != nil {
		t.Fatalf("create monitor: %v", err)
	}

	checker := NewChecker(r, nil)

	ctx := context.Background()
	go checker.Start(ctx)

	time.Sleep(100 * time.Millisecond)
	checker.Stop()

	results, err := r.GetServiceMonitorResults(monitor.ID, 10)
	if err != nil {
		t.Fatalf("get results: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected at least one result before stop")
	}
}

func TestCheckerRunsOnNodeWhenNodeIDSet(t *testing.T) {
	fake := &fakeCommander{
		res: ws.CommandResult{
			Success: true,
			Data: map[string]interface{}{
				"success":      false,
				"latencyMs":    float64(12),
				"errorMessage": "unreachable",
			},
		},
	}
	checker := NewChecker(nil, fake)
	limits := checker.loadServiceMonitorLimits()
	now := time.Now().UnixMilli()
	monitor := &model.ServiceMonitor{
		ID:         99,
		Type:       "icmp",
		Target:     "8.8.8.8",
		TimeoutSec: 2,
		NodeID:     123,
	}
	res := checker.executeCheck(monitor, now, limits)
	if fake.lastNodeID != 123 {
		t.Fatalf("expected command to be sent to node 123, got %d", fake.lastNodeID)
	}
	if fake.lastType != "ServiceMonitorCheck" {
		t.Fatalf("expected ServiceMonitorCheck command, got %s", fake.lastType)
	}
	if res.Success != 0 {
		t.Fatalf("expected failed result from node check")
	}
	if res.ErrorMessage != "unreachable" {
		t.Fatalf("expected errorMessage unreachable, got %q", res.ErrorMessage)
	}
}

func TestCheckerDoesNotBurstOnRestartWhenRecentResultsExist(t *testing.T) {
	r, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	monitor := &model.ServiceMonitor{
		Name:        "recent-monitor",
		Type:        "tcp",
		Target:      "127.0.0.1:1",
		IntervalSec: 60,
		TimeoutSec:  1,
		NodeID:      0,
		Enabled:     1,
		CreatedTime: now,
		UpdatedTime: now,
	}
	if err := r.CreateServiceMonitor(monitor); err != nil {
		t.Fatalf("create monitor: %v", err)
	}
	if err := r.InsertServiceMonitorResult(&model.ServiceMonitorResult{
		MonitorID: monitor.ID,
		NodeID:    0,
		Timestamp: now - 10_000,
		Success:   1,
	}); err != nil {
		t.Fatalf("seed recent result: %v", err)
	}

	checker := NewChecker(r, nil)
	ctx, cancel := context.WithCancel(context.Background())
	go checker.Start(ctx)
	// Give the initial scan a chance to run.
	time.Sleep(200 * time.Millisecond)
	cancel()
	checker.Stop()

	results, err := r.GetServiceMonitorResults(monitor.ID, 10)
	if err != nil {
		t.Fatalf("get results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected no immediate rerun (1 result), got %d", len(results))
	}
}

func TestCheckerConcurrencyPreventsSlowMonitorBlockingOthers(t *testing.T) {
	r, err := repo.Open(":memory:")
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	// Force worker limit to at least 2 for this test.
	_ = r.UpsertConfig(monitoring.ConfigServiceMonitorWorkerLimit, "2", now)

	slow := &model.ServiceMonitor{
		Name:        "slow",
		Type:        "icmp",
		Target:      "8.8.8.8",
		IntervalSec: 60,
		TimeoutSec:  1,
		NodeID:      123,
		Enabled:     1,
		CreatedTime: now,
		UpdatedTime: now,
	}
	if err := r.CreateServiceMonitor(slow); err != nil {
		t.Fatalf("create slow monitor: %v", err)
	}
	fast := &model.ServiceMonitor{
		Name:        "fast",
		Type:        "icmp",
		Target:      "1.1.1.1",
		IntervalSec: 60,
		TimeoutSec:  1,
		NodeID:      123,
		Enabled:     1,
		CreatedTime: now,
		UpdatedTime: now,
	}
	if err := r.CreateServiceMonitor(fast); err != nil {
		t.Fatalf("create fast monitor: %v", err)
	}

	cmd := &delayedCommander{delayByMonitorID: map[int64]time.Duration{slow.ID: 800 * time.Millisecond}}
	checker := NewChecker(r, cmd)
	ctx, cancel := context.WithCancel(context.Background())
	go checker.Start(ctx)

	// Fast monitor should complete even while slow one is still running.
	time.Sleep(250 * time.Millisecond)
	results, err := r.GetServiceMonitorResults(fast.ID, 10)
	if err != nil {
		t.Fatalf("get fast results: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected fast monitor to have results without waiting for slow")
	}

	cancel()
	checker.Stop()
}
