package health

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go-backend/internal/monitoring"
	backendruntime "go-backend/internal/runtime"
	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
	"go-backend/internal/ws"
)

type nodeCommander interface {
	SendCommand(nodeID int64, cmdType string, data interface{}, timeout time.Duration) (ws.CommandResult, error)
}

const serviceMonitorReportInterval = 30 * time.Second // DB write interval per monitor

type Checker struct {
	repo       *repo.Repository
	commander  nodeCommander
	runtimeCtx context.Context
	lastRun    map[int64]int64
	inFlight   map[int64]struct{}

	// In-memory latest result per monitor (for real-time API reads)
	latestResults map[int64]*model.ServiceMonitorResult
	lastDBWrite   map[int64]int64 // last DB write timestamp per monitorID
	runtimeClient func() backendruntime.RuntimeClient

	mu       sync.RWMutex
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	checking int32 // atomic flag: 1 = runChecks running, 0 = idle
}

func NewChecker(repo *repo.Repository, commander nodeCommander, runtimeClient ...func() backendruntime.RuntimeClient) *Checker {
	var runtimeClientProvider func() backendruntime.RuntimeClient
	if len(runtimeClient) > 0 {
		runtimeClientProvider = runtimeClient[0]
	}
	return &Checker{
		repo:          repo,
		commander:     commander,
		runtimeCtx:    context.Background(),
		lastRun:       make(map[int64]int64),
		inFlight:      make(map[int64]struct{}),
		latestResults: make(map[int64]*model.ServiceMonitorResult),
		lastDBWrite:   make(map[int64]int64),
		runtimeClient: runtimeClientProvider,
	}
}

// GetLatestCached returns the in-memory latest results (updated every 1s).
// Returns nil if no results are cached.
func (c *Checker) GetLatestCached() []*model.ServiceMonitorResult {
	c.mu.RLock()
	defer c.mu.RUnlock()
	results := make([]*model.ServiceMonitorResult, 0, len(c.latestResults))
	for _, r := range c.latestResults {
		results = append(results, r)
	}
	return results
}

func (c *Checker) Start(ctx context.Context) {
	c.mu.Lock()
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.runtimeCtx = ctx
	c.mu.Unlock()

	c.runChecks(ctx)

	for {
		limits := c.loadServiceMonitorLimits()
		scanInterval := time.Duration(limits.CheckerScanIntervalSec) * time.Second
		if scanInterval <= 0 {
			scanInterval = 1 * time.Second
		}

		timer := time.NewTimer(scanInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			c.runChecks(ctx)
		}
	}
}

func (c *Checker) Stop() {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Unlock()
	c.wg.Wait()
}

func (c *Checker) RunOnce(m *model.ServiceMonitor) (*model.ServiceMonitorResult, error) {
	if c == nil {
		return nil, errors.New("checker not initialized")
	}
	if m == nil {
		return nil, errors.New("monitor is nil")
	}
	limits := c.loadServiceMonitorLimits()
	return c.executeCheck(m, time.Now().UnixMilli(), limits), nil
}

func (c *Checker) runChecks(ctx context.Context) {
	// Skip if previous round is still running (interval < timeout guard)
	if !atomic.CompareAndSwapInt32(&c.checking, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&c.checking, 0)

	if c == nil || c.repo == nil {
		return
	}

	limits := c.loadServiceMonitorLimits()
	monitors, err := c.repo.ListEnabledServiceMonitors()
	if err != nil {
		log.Printf("service monitor scheduler failed op=list_enabled err=%v", err)
		return
	}
	if len(monitors) == 0 {
		return
	}

	// Use persisted result timestamps to avoid restart bursts.
	latest, err := c.repo.GetLatestServiceMonitorResults()
	if err != nil {
		log.Printf("service monitor scheduler failed op=get_latest_results err=%v", err)
		latest = nil
	}
	persistedLast := make(map[int64]int64, len(latest))
	for _, r := range latest {
		if r.MonitorID <= 0 || r.Timestamp <= 0 {
			continue
		}
		persistedLast[r.MonitorID] = r.Timestamp
	}

	now := time.Now().UnixMilli()
	due := make([]model.ServiceMonitor, 0, len(monitors))
	for _, m := range monitors {
		select {
		case <-ctx.Done():
			return
		default:
		}

		intervalSec := m.IntervalSec
		if intervalSec <= 0 {
			intervalSec = limits.DefaultIntervalSec
		}
		if intervalSec < limits.MinIntervalSec {
			intervalSec = limits.MinIntervalSec
		}
		intervalMs := int64(intervalSec) * 1000

		c.mu.Lock()
		if _, ok := c.inFlight[m.ID]; ok {
			c.mu.Unlock()
			continue
		}

		lastSeen := persistedLast[m.ID]
		if v := c.lastRun[m.ID]; v > lastSeen {
			lastSeen = v
		}
		if lastSeen > 0 && intervalMs > 0 && now-lastSeen < intervalMs {
			c.mu.Unlock()
			continue
		}

		c.inFlight[m.ID] = struct{}{}
		// Use now as a best-effort guard against overlapping scans; the final
		// timestamp is updated again when the result is persisted.
		c.lastRun[m.ID] = now
		c.mu.Unlock()

		due = append(due, m)
	}
	if len(due) == 0 {
		return
	}

	workerLimit := limits.WorkerLimit
	if workerLimit <= 0 {
		workerLimit = 1
	}
	if workerLimit > len(due) {
		workerLimit = len(due)
	}

	jobs := make(chan model.ServiceMonitor, len(due))
	for _, m := range due {
		jobs <- m
	}
	close(jobs)

	reportIntervalMs := int64(serviceMonitorReportInterval / time.Millisecond)

	for i := 0; i < workerLimit; i++ {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case m, ok := <-jobs:
					if !ok {
						return
					}
					ts := time.Now().UnixMilli()
					result := c.executeCheck(&m, ts, limits)

					// Always update in-memory cache for real-time reads
					c.mu.Lock()
					c.latestResults[m.ID] = result
					c.lastRun[m.ID] = result.Timestamp
					delete(c.inFlight, m.ID)

					// Only write to DB every 30s per monitor
					lastWrite := c.lastDBWrite[m.ID]
					writeToDB := ts-lastWrite >= reportIntervalMs
					if writeToDB {
						c.lastDBWrite[m.ID] = ts
					}
					c.mu.Unlock()

					if writeToDB {
						if err := c.repo.InsertServiceMonitorResult(result); err != nil {
							log.Printf("monitoring write failed op=service_monitor_result.insert monitor_id=%d err=%v", result.MonitorID, err)
						}
					}
				}
			}
		}()
	}
}

func (c *Checker) executeCheck(m *model.ServiceMonitor, timestamp int64, limits monitoring.ServiceMonitorLimits) *model.ServiceMonitorResult {
	result := &model.ServiceMonitorResult{
		MonitorID: m.ID,
		NodeID:    m.NodeID,
		Timestamp: timestamp,
	}

	timeoutSec := m.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = limits.DefaultTimeoutSec
	}
	if timeoutSec < limits.MinTimeoutSec {
		timeoutSec = limits.MinTimeoutSec
	}
	if timeoutSec > limits.MaxTimeoutSec {
		timeoutSec = limits.MaxTimeoutSec
	}

	timeout := time.Duration(timeoutSec) * time.Second

	// When nodeId is set, run checks on the specified node.
	if m.NodeID > 0 {
		c.checkOnNode(m, timeoutSec, timeout, result)
		return result
	}

	switch strings.ToLower(strings.TrimSpace(m.Type)) {
	case "tcp":
		c.checkTCP(m.Target, timeout, result)
	case "icmp":
		result.Success = 0
		result.ErrorMessage = "ICMP 监控必须指定执行节点"
	default:
		result.Success = 0
		result.ErrorMessage = fmt.Sprintf("不支持的检查类型: %s", m.Type)
	}

	return result
}

func (c *Checker) loadServiceMonitorLimits() monitoring.ServiceMonitorLimits {
	defaults := monitoring.DefaultServiceMonitorLimits()
	if c == nil || c.repo == nil {
		return defaults
	}
	cfg, err := c.repo.GetConfigsByNames([]string{
		monitoring.ConfigServiceMonitorCheckerScanIntervalSec,
		monitoring.ConfigServiceMonitorWorkerLimit,
		monitoring.ConfigServiceMonitorMinIntervalSec,
		monitoring.ConfigServiceMonitorDefaultIntervalSec,
		monitoring.ConfigServiceMonitorMinTimeoutSec,
		monitoring.ConfigServiceMonitorDefaultTimeoutSec,
		monitoring.ConfigServiceMonitorMaxTimeoutSec,
	})
	if err != nil {
		return defaults
	}
	return monitoring.ServiceMonitorLimitsFromConfigMap(cfg)
}

type serviceMonitorCheckRequest struct {
	MonitorID  int64  `json:"monitorId"`
	Type       string `json:"type"`
	Target     string `json:"target"`
	TimeoutSec int    `json:"timeoutSec"`
}

func (c *Checker) checkOnNode(m *model.ServiceMonitor, timeoutSec int, timeout time.Duration, result *model.ServiceMonitorResult) {
	if c == nil || m == nil || result == nil {
		return
	}
	if c.commander == nil {
		if c.selectedRuntimeClient() == nil {
			result.Success = 0
			result.ErrorMessage = "节点检查不可用"
			return
		}
	}

	checkType := strings.ToLower(strings.TrimSpace(m.Type))
	if checkType != "tcp" && checkType != "icmp" {
		result.Success = 0
		result.ErrorMessage = fmt.Sprintf("不支持的检查类型: %s", m.Type)
		return
	}
	if strings.TrimSpace(m.Target) == "" {
		result.Success = 0
		result.ErrorMessage = "检查目标为空"
		return
	}
	if runtimeClient := c.selectedRuntimeClient(); runtimeClient != nil {
		c.checkOnRuntimeClient(runtimeClient, m, timeoutSec, result)
		return
	}

	req := serviceMonitorCheckRequest{
		MonitorID:  m.ID,
		Type:       checkType,
		Target:     m.Target,
		TimeoutSec: timeoutSec,
	}

	cmdTimeout := timeout
	if cmdTimeout < 2*time.Second {
		cmdTimeout = 2 * time.Second
	}
	cmdTimeout = cmdTimeout + 2*time.Second

	cmdRes, err := c.commander.SendCommand(m.NodeID, "ServiceMonitorCheck", req, cmdTimeout)
	if err != nil {
		result.Success = 0
		result.ErrorMessage = err.Error()
		return
	}
	if cmdRes.Data == nil {
		result.Success = 0
		result.ErrorMessage = "节点返回为空"
		return
	}

	if v, ok := cmdRes.Data["success"]; ok {
		if b, ok := v.(bool); ok {
			if b {
				result.Success = 1
			} else {
				result.Success = 0
			}
		}
	}
	if v, ok := cmdRes.Data["latencyMs"]; ok {
		if f, ok := v.(float64); ok {
			result.LatencyMs = f
		}
	}
	if v, ok := cmdRes.Data["statusCode"]; ok {
		if f, ok := v.(float64); ok {
			result.StatusCode = int(f)
		}
	}
	if v, ok := cmdRes.Data["errorMessage"]; ok {
		if s, ok := v.(string); ok {
			result.ErrorMessage = s
		}
	}
}

func (c *Checker) selectedRuntimeClient() backendruntime.RuntimeClient {
	if c == nil || c.runtimeClient == nil {
		return nil
	}
	return c.runtimeClient()
}

func (c *Checker) runtimeContext() context.Context {
	if c == nil {
		return context.Background()
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.runtimeCtx != nil {
		return c.runtimeCtx
	}
	return context.Background()
}

func (c *Checker) checkOnRuntimeClient(runtimeClient backendruntime.RuntimeClient, m *model.ServiceMonitor, timeoutSec int, result *model.ServiceMonitorResult) {
	if c == nil || c.repo == nil {
		result.Success = 0
		result.ErrorMessage = "节点检查不可用"
		return
	}
	var node repo.Node
	if err := c.repo.DB().First(&node, m.NodeID).Error; err != nil {
		result.Success = 0
		result.ErrorMessage = err.Error()
		return
	}
	timeout := time.Duration(timeoutSec) * time.Second
	if timeout <= 0 {
		timeout = time.Second
	}
	ctx, cancel := context.WithTimeout(c.runtimeContext(), timeout)
	defer cancel()
	res, err := runtimeClient.CheckService(ctx, node, backendruntime.ServiceCheckRequest{
		MonitorID:  m.ID,
		Type:       strings.ToLower(strings.TrimSpace(m.Type)),
		Target:     m.Target,
		TimeoutSec: timeoutSec,
	})
	if err != nil {
		result.Success = 0
		result.ErrorMessage = err.Error()
		return
	}
	if res.Success {
		result.Success = 1
	}
	result.LatencyMs = res.LatencyMs
	result.StatusCode = res.StatusCode
	result.ErrorMessage = res.ErrorMessage
}

func (c *Checker) checkTCP(target string, timeout time.Duration, result *model.ServiceMonitorResult) {
	start := time.Now()

	conn, err := net.DialTimeout("tcp", target, timeout)
	latency := time.Since(start)

	result.LatencyMs = float64(latency.Milliseconds())

	if err != nil {
		result.Success = 0
		result.ErrorMessage = err.Error()
		return
	}
	_ = conn.Close()
	result.Success = 1
}
