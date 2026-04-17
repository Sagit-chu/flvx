package handler

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go-backend/internal/http/response"
	"go-backend/internal/monitoring"
	"go-backend/internal/store/model"
)

const (
	defaultMetricsRangeMs = int64(60 * 60 * 1000)      // 1h
	maxMetricsRangeMs     = int64(24 * 60 * 60 * 1000) // 24h
)

func (h *Handler) resolveServiceMonitorLimits() monitoring.ServiceMonitorLimits {
	defaults := monitoring.DefaultServiceMonitorLimits()
	if h == nil || h.repo == nil {
		return defaults
	}
	cfg, err := h.repo.GetConfigsByNames([]string{
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

func (h *Handler) monitorNodeMetricsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.ensureMonitoringAccess(w, r) {
		return
	}

	path := r.URL.Path
	prefix := "/api/v1/monitor/nodes/"
	if !strings.HasPrefix(path, prefix) {
		response.WriteJSON(w, response.ErrDefault("无效的路径"))
		return
	}

	rest := strings.TrimPrefix(path, prefix)
	if strings.HasSuffix(rest, "/metrics/latest") {
		h.handleNodeMetricsLatest(w, r, strings.TrimSuffix(rest, "/metrics/latest"))
		return
	}
	if strings.HasSuffix(rest, "/metrics") {
		h.handleNodeMetrics(w, r, strings.TrimSuffix(rest, "/metrics"))
		return
	}

	response.WriteJSON(w, response.ErrDefault("无效的路径"))
}

type monitorNodeListItem struct {
	ID          int64  `json:"id"`
	Inx         int    `json:"inx"`
	Name        string `json:"name"`
	Status      int    `json:"status"`
	Version     string `json:"version"`
	UpdatedTime int64  `json:"updatedTime"`
}

func (h *Handler) monitorNodeListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.ensureMonitoringAccess(w, r) {
		return
	}

	nodes, err := h.repo.ListMonitorNodes()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	items := make([]monitorNodeListItem, 0, len(nodes))
	for _, n := range nodes {
		updated := int64(0)
		if n.UpdatedTime.Valid {
			updated = n.UpdatedTime.Int64
		}
		items = append(items, monitorNodeListItem{
			ID:          n.ID,
			Inx:         n.Inx,
			Name:        n.Name,
			Status:      n.Status,
			Version:     n.Version.String,
			UpdatedTime: updated,
		})
	}

	response.WriteJSON(w, response.OK(items))
}

type monitorTunnelListItem struct {
	ID          int64  `json:"id"`
	Inx         int    `json:"inx"`
	Name        string `json:"name"`
	Status      int    `json:"status"`
	UpdatedTime int64  `json:"updatedTime"`
}

func (h *Handler) monitorTunnelListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.ensureMonitoringAccess(w, r) {
		return
	}

	tunnels, err := h.repo.ListMonitorTunnels()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	items := make([]monitorTunnelListItem, 0, len(tunnels))
	for _, t := range tunnels {
		items = append(items, monitorTunnelListItem{
			ID:          t.ID,
			Inx:         t.Inx,
			Name:        t.Name,
			Status:      t.Status,
			UpdatedTime: t.UpdatedTime,
		})
	}

	response.WriteJSON(w, response.OK(items))
}

func (h *Handler) handleNodeMetrics(w http.ResponseWriter, r *http.Request, nodeIDStr string) {
	nodeID, err := strconv.ParseInt(nodeIDStr, 10, 64)
	if err != nil || nodeID <= 0 {
		response.WriteJSON(w, response.ErrDefault("无效的节点ID"))
		return
	}

	now := time.Now().UnixMilli()
	startMs := now - defaultMetricsRangeMs
	endMs := now

	if s := r.URL.Query().Get("start"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			startMs = v
		}
	}
	if e := r.URL.Query().Get("end"); e != "" {
		if v, err := strconv.ParseInt(e, 10, 64); err == nil {
			endMs = v
		}
	}
	if startMs <= 0 || endMs <= 0 {
		response.WriteJSON(w, response.ErrDefault("无效的时间范围"))
		return
	}
	if endMs < startMs {
		response.WriteJSON(w, response.ErrDefault("无效的时间范围"))
		return
	}
	if endMs-startMs > maxMetricsRangeMs {
		response.WriteJSON(w, response.ErrDefault("时间范围过大"))
		return
	}

	metrics, err := h.repo.GetNodeMetrics(nodeID, startMs, endMs)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OK(metrics))
}

func (h *Handler) handleNodeMetricsLatest(w http.ResponseWriter, _ *http.Request, nodeIDStr string) {
	nodeID, err := strconv.ParseInt(nodeIDStr, 10, 64)
	if err != nil || nodeID <= 0 {
		response.WriteJSON(w, response.ErrDefault("无效的节点ID"))
		return
	}

	metric, err := h.repo.GetLatestNodeMetric(nodeID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if metric == nil {
		response.WriteJSON(w, response.OK(nil))
		return
	}

	response.WriteJSON(w, response.OK(metric))
}

func (h *Handler) monitorTunnelQualityHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.ensureMonitoringAccess(w, r) {
		return
	}

	// Try in-memory cache first
	if h.qualityProber != nil {
		items := h.qualityProber.GetAll()
		if len(items) > 0 {
			response.WriteJSON(w, response.OK(items))
			return
		}
	}

	// Fallback to database (latest per tunnel)
	qualities, err := h.repo.GetLatestTunnelQualities()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	snapshots := make([]tunnelQualitySnapshot, 0, len(qualities))
	for _, q := range qualities {
		snapshots = append(snapshots, tunnelQualitySnapshot{
			TunnelID:           q.TunnelID,
			EntryToExitLatency: q.EntryToExitLatency,
			ExitToBingLatency:  q.ExitToBingLatency,
			EntryToExitLoss:    q.EntryToExitLoss,
			ExitToBingLoss:     q.ExitToBingLoss,
			Success:            q.Success == 1,
			ErrorMessage:       q.ErrorMessage,
			Timestamp:          q.Timestamp,
			ChainDetails:       q.ChainDetails,
		})
	}
	response.WriteJSON(w, response.OK(snapshots))
}

// monitorTunnelQualityHistory returns quality probe history for charting.
// GET /api/v1/monitor/tunnels/{id}/quality?start=...&end=...
// Mirrors monitorTunnelMetrics / monitorServiceResultsHandler pattern.
func (h *Handler) monitorTunnelQualityHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.ensureMonitoringAccess(w, r) {
		return
	}

	tunnelIDStr := extractPathParam(r.URL.Path, "/api/v1/monitor/tunnels/", "/quality")
	tunnelID, err := strconv.ParseInt(tunnelIDStr, 10, 64)
	if err != nil || tunnelID <= 0 {
		response.WriteJSON(w, response.ErrDefault("无效的隧道ID"))
		return
	}

	now := time.Now().UnixMilli()
	startMs := now - defaultMetricsRangeMs
	endMs := now

	if s := r.URL.Query().Get("start"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			startMs = v
		}
	}
	if e := r.URL.Query().Get("end"); e != "" {
		if v, err := strconv.ParseInt(e, 10, 64); err == nil {
			endMs = v
		}
	}
	if startMs <= 0 || endMs <= 0 || endMs < startMs {
		response.WriteJSON(w, response.ErrDefault("无效的时间范围"))
		return
	}
	if endMs-startMs > maxMetricsRangeMs {
		response.WriteJSON(w, response.ErrDefault("时间范围过大"))
		return
	}

	results, err := h.repo.GetTunnelQualityHistory(tunnelID, startMs, endMs)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OK(results))
}

func (h *Handler) monitorTunnelMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.ensureMonitoringAccess(w, r) {
		return
	}

	path := r.URL.Path
	prefix := "/api/v1/monitor/tunnels/"
	if !strings.HasPrefix(path, prefix) {
		response.WriteJSON(w, response.ErrDefault("无效的路径"))
		return
	}

	rest := strings.TrimPrefix(path, prefix)

	// Route: /api/v1/monitor/tunnels/{id}/quality
	if strings.HasSuffix(rest, "/quality") {
		h.monitorTunnelQualityHistory(w, r)
		return
	}

	// Route: /api/v1/monitor/tunnels/{id}/metrics (original)
	tunnelIDStr := extractPathParam(path, prefix, "/metrics")
	tunnelID, err := strconv.ParseInt(tunnelIDStr, 10, 64)
	if err != nil || tunnelID <= 0 {
		response.WriteJSON(w, response.ErrDefault("无效的隧道ID"))
		return
	}

	now := time.Now().UnixMilli()
	startMs := now - defaultMetricsRangeMs
	endMs := now

	if s := r.URL.Query().Get("start"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			startMs = v
		}
	}
	if e := r.URL.Query().Get("end"); e != "" {
		if v, err := strconv.ParseInt(e, 10, 64); err == nil {
			endMs = v
		}
	}
	if startMs <= 0 || endMs <= 0 {
		response.WriteJSON(w, response.ErrDefault("无效的时间范围"))
		return
	}
	if endMs < startMs {
		response.WriteJSON(w, response.ErrDefault("无效的时间范围"))
		return
	}
	if endMs-startMs > maxMetricsRangeMs {
		response.WriteJSON(w, response.ErrDefault("时间范围过大"))
		return
	}

	metrics, err := h.repo.GetTunnelMetricsAggregated(tunnelID, startMs, endMs)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OK(metrics))
}

func (h *Handler) monitorServiceListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.ensureMonitoringAccess(w, r) {
		return
	}

	monitors, err := h.repo.ListServiceMonitors()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OK(monitors))
}

type createServiceMonitorRequest struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Target      string `json:"target"`
	IntervalSec int    `json:"intervalSec"`
	TimeoutSec  int    `json:"timeoutSec"`
	NodeID      int64  `json:"nodeId"`
	Enabled     *int   `json:"enabled"`
}

func (h *Handler) monitorServiceCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.ensureMonitoringAccess(w, r) {
		return
	}

	var req createServiceMonitorRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		response.WriteJSON(w, response.ErrDefault("名称不能为空"))
		return
	}

	monitorType := strings.ToLower(strings.TrimSpace(req.Type))
	if monitorType != "tcp" && monitorType != "icmp" {
		response.WriteJSON(w, response.ErrDefault("类型必须是 tcp 或 icmp"))
		return
	}

	target := strings.TrimSpace(req.Target)
	if target == "" {
		response.WriteJSON(w, response.ErrDefault("目标地址不能为空"))
		return
	}

	limits := h.resolveServiceMonitorLimits()

	intervalSec := req.IntervalSec
	if intervalSec <= 0 {
		intervalSec = limits.DefaultIntervalSec
	}
	if intervalSec < limits.MinIntervalSec {
		intervalSec = limits.MinIntervalSec
	}

	timeoutSec := req.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = limits.DefaultTimeoutSec
	}
	if timeoutSec < limits.MinTimeoutSec {
		timeoutSec = limits.MinTimeoutSec
	}
	if timeoutSec > limits.MaxTimeoutSec {
		timeoutSec = limits.MaxTimeoutSec
	}

	enabled := 1
	if req.Enabled != nil {
		if *req.Enabled == 0 || *req.Enabled == 1 {
			enabled = *req.Enabled
		}
	}

	now := time.Now().UnixMilli()
	if req.NodeID > 0 {
		n, err := h.repo.GetNodeByID(req.NodeID)
		if err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
		if n == nil {
			response.WriteJSON(w, response.ErrDefault("节点不存在"))
			return
		}
	}
	m := &model.ServiceMonitor{
		Name:        name,
		Type:        monitorType,
		Target:      target,
		IntervalSec: intervalSec,
		TimeoutSec:  timeoutSec,
		NodeID:      req.NodeID,
		Enabled:     enabled,
		CreatedTime: now,
		UpdatedTime: now,
	}
	if m.Type == "icmp" && m.NodeID <= 0 {
		response.WriteJSON(w, response.ErrDefault("ICMP 监控必须选择执行节点"))
		return
	}
	// enabled is already normalized above.

	if err := h.repo.CreateServiceMonitor(m); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OK(m))
}

type updateServiceMonitorRequest struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Target      string `json:"target"`
	IntervalSec int    `json:"intervalSec"`
	TimeoutSec  int    `json:"timeoutSec"`
	NodeID      *int64 `json:"nodeId"`
	Enabled     *int   `json:"enabled"`
}

func (h *Handler) monitorServiceUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.ensureMonitoringAccess(w, r) {
		return
	}

	var req updateServiceMonitorRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}

	if req.ID <= 0 {
		response.WriteJSON(w, response.ErrDefault("无效的监控ID"))
		return
	}

	existing, err := h.repo.GetServiceMonitor(req.ID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if existing == nil {
		response.WriteJSON(w, response.ErrDefault("监控不存在"))
		return
	}

	name := strings.TrimSpace(req.Name)
	if name != "" {
		existing.Name = name
	}

	monitorType := strings.ToLower(strings.TrimSpace(req.Type))
	if monitorType == "tcp" || monitorType == "icmp" {
		existing.Type = monitorType
	}

	target := strings.TrimSpace(req.Target)
	if target != "" {
		existing.Target = target
	}

	limits := h.resolveServiceMonitorLimits()

	if req.IntervalSec > 0 {
		intervalSec := req.IntervalSec
		if intervalSec < limits.MinIntervalSec {
			intervalSec = limits.MinIntervalSec
		}
		existing.IntervalSec = intervalSec
	}
	if req.TimeoutSec > 0 {
		timeoutSec := req.TimeoutSec
		if timeoutSec < limits.MinTimeoutSec {
			timeoutSec = limits.MinTimeoutSec
		}
		if timeoutSec > limits.MaxTimeoutSec {
			timeoutSec = limits.MaxTimeoutSec
		}
		existing.TimeoutSec = timeoutSec
	}

	if req.NodeID != nil {
		existing.NodeID = *req.NodeID
	}
	if req.Enabled != nil {
		if *req.Enabled == 0 || *req.Enabled == 1 {
			existing.Enabled = *req.Enabled
		}
	}

	existing.UpdatedTime = time.Now().UnixMilli()
	if existing.Type == "icmp" && existing.NodeID <= 0 {
		response.WriteJSON(w, response.ErrDefault("ICMP 监控必须选择执行节点"))
		return
	}
	if existing.NodeID > 0 {
		n, err := h.repo.GetNodeByID(existing.NodeID)
		if err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
		if n == nil {
			response.WriteJSON(w, response.ErrDefault("节点不存在"))
			return
		}
	}

	if err := h.repo.UpdateServiceMonitor(existing); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OK(existing))
}

type deleteServiceMonitorRequest struct {
	ID int64 `json:"id"`
}

func (h *Handler) monitorServiceDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.ensureMonitoringAccess(w, r) {
		return
	}

	var req deleteServiceMonitorRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}

	if req.ID <= 0 {
		response.WriteJSON(w, response.ErrDefault("无效的监控ID"))
		return
	}

	if err := h.repo.DeleteServiceMonitor(req.ID); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) monitorServiceRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.ensureMonitoringAccess(w, r) {
		return
	}
	if h.healthCheck == nil {
		response.WriteJSON(w, response.ErrDefault("监控服务不可用"))
		return
	}

	var req deleteServiceMonitorRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if req.ID <= 0 {
		response.WriteJSON(w, response.ErrDefault("无效的监控ID"))
		return
	}

	m, err := h.repo.GetServiceMonitor(req.ID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if m == nil {
		response.WriteJSON(w, response.ErrDefault("监控不存在"))
		return
	}

	res, err := h.healthCheck.RunOnce(m)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := h.repo.InsertServiceMonitorResult(res); err != nil {
		log.Printf("monitoring write failed op=service_monitor_result.manual_insert monitor_id=%d err=%v", res.MonitorID, err)
	}
	response.WriteJSON(w, response.OK(res))
}

func (h *Handler) monitorServiceResultsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.ensureMonitoringAccess(w, r) {
		return
	}

	monitorIDStr := extractPathParam(r.URL.Path, "/api/v1/monitor/services/", "/results")
	monitorID, err := strconv.ParseInt(monitorIDStr, 10, 64)
	if err != nil || monitorID <= 0 {
		response.WriteJSON(w, response.ErrDefault("无效的监控ID"))
		return
	}

	// If start/end time range is provided, use time-based query (mirrors node metrics / tunnel quality pattern).
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	if startStr != "" && endStr != "" {
		startMs, err1 := strconv.ParseInt(startStr, 10, 64)
		endMs, err2 := strconv.ParseInt(endStr, 10, 64)
		if err1 != nil || err2 != nil || startMs <= 0 || endMs <= 0 || endMs < startMs {
			response.WriteJSON(w, response.ErrDefault("无效的时间范围"))
			return
		}
		if endMs-startMs > maxMetricsRangeMs {
			response.WriteJSON(w, response.ErrDefault("时间范围过大"))
			return
		}
		results, err := h.repo.GetServiceMonitorResultsByTimeRange(monitorID, startMs, endMs)
		if err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
		response.WriteJSON(w, response.OK(results))
		return
	}

	// Fallback: count-based limit query (backward compat).
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 1000 {
			limit = v
		}
	}

	results, err := h.repo.GetServiceMonitorResults(monitorID, limit)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OK(results))
}

func (h *Handler) monitorServiceLatestResultsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.ensureMonitoringAccess(w, r) {
		return
	}

	// Try in-memory cache first (updated every 1s)
	if h.healthCheck != nil {
		cached := h.healthCheck.GetLatestCached()
		if len(cached) > 0 {
			response.WriteJSON(w, response.OK(cached))
			return
		}
	}

	// Fallback to database
	results, err := h.repo.GetLatestServiceMonitorResults()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(results))
}

func (h *Handler) monitorServiceLimitsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.ensureMonitoringAccess(w, r) {
		return
	}
	response.WriteJSON(w, response.OK(h.resolveServiceMonitorLimits()))
}

func extractPathParam(path, prefix, suffix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	if suffix != "" {
		rest = strings.TrimSuffix(rest, suffix)
	}
	return rest
}

type monitorAccessData struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// monitorAccessHandler is a lightweight capability check for frontend navigation.
// It does NOT replace authorization on the actual monitoring endpoints.
func (h *Handler) monitorAccessHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	userID, roleID, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "未登录或token已过期"))
		return
	}
	if roleID == 0 {
		response.WriteJSON(w, response.OK(monitorAccessData{Allowed: true}))
		return
	}

	allowed, err := h.repo.HasMonitorPermission(userID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	data := monitorAccessData{Allowed: allowed}
	if !allowed {
		data.Reason = "need_admin_grant"
	}
	response.WriteJSON(w, response.OK(data))
}

func (h *Handler) ensureAdminAccess(w http.ResponseWriter, r *http.Request) bool {
	_, roleID, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "未登录或token已过期"))
		return false
	}
	if roleID != 0 {
		response.WriteJSON(w, response.Err(403, "权限不足，仅管理员可操作"))
		return false
	}
	return true
}

func (h *Handler) ensureMonitoringAccess(w http.ResponseWriter, r *http.Request) bool {
	userID, roleID, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "未登录或token已过期"))
		return false
	}
	if roleID == 0 {
		return true
	}
	allowed, err := h.repo.HasMonitorPermission(userID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return false
	}
	if !allowed {
		response.WriteJSON(w, response.Err(403, "权限不足：当前账户非管理员，且未被授予监控权限。请联系管理员在用户管理中授权监控权限。"))
		return false
	}
	return true
}

func (h *Handler) monitorPermissionList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.ensureAdminAccess(w, r) {
		return
	}

	items, err := h.repo.ListMonitorPermissions()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(items))
}

type monitorPermissionMutationRequest struct {
	UserID int64 `json:"userId"`
}

func (h *Handler) monitorPermissionAssign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.ensureAdminAccess(w, r) {
		return
	}

	var req monitorPermissionMutationRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if req.UserID <= 0 {
		response.WriteJSON(w, response.ErrDefault("无效的用户ID"))
		return
	}

	u, err := h.repo.GetUserByID(req.UserID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if u == nil {
		response.WriteJSON(w, response.ErrDefault("用户不存在"))
		return
	}

	if err := h.repo.InsertMonitorPermission(req.UserID, time.Now().UnixMilli()); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) monitorPermissionRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.ensureAdminAccess(w, r) {
		return
	}

	var req monitorPermissionMutationRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if req.UserID <= 0 {
		response.WriteJSON(w, response.ErrDefault("无效的用户ID"))
		return
	}

	if err := h.repo.DeleteMonitorPermission(req.UserID); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}
