package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go-backend/internal/http/response"
	backendruntime "go-backend/internal/runtime"
	"go-backend/internal/store/repo"
)

type runtimeSwitchStarter interface {
	Start(ctx context.Context, engine backendruntime.Engine, nodes []repo.Node) error
	Progress() backendruntime.SwitchProgress
}

type runtimeStatusProvider interface {
	GetNodeRuntimeStatus(ctx context.Context, node repo.Node) (backendruntime.NodeRuntimeStatus, error)
}

type runtimeSettingsUpdateRequest struct {
	Engine string `json:"engine"`
}

type runtimeSettingsResponse struct {
	CurrentEngine   string                 `json:"currentEngine"`
	SwitchStatus    string                 `json:"switchStatus"`
	Generation      int64                  `json:"generation"`
	LastError       string                 `json:"lastError"`
	NodeSummary     runtimeNodeSummary     `json:"nodeSummary"`
	RuntimeProgress runtimeProgressSummary `json:"runtimeProgress"`
	RebuildProgress runtimeProgressSummary `json:"rebuildProgress"`
	Warnings        []string               `json:"warnings"`
	Nodes           []runtimeNodeProgress  `json:"nodes"`
}

type runtimeNodeSummary struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Ready     int `json:"ready"`
	Pending   int `json:"pending"`
	Failed    int `json:"failed"`
}

type runtimeProgressSummary struct {
	Engine  string `json:"engine"`
	State   string `json:"state"`
	Phase   string `json:"phase,omitempty"`
	Message string `json:"message,omitempty"`
}

type runtimeNodeProgress struct {
	NodeID   int64  `json:"nodeId"`
	NodeName string `json:"nodeName"`
	Engine   string `json:"engine"`
	Ready    bool   `json:"ready"`
	Progress string `json:"progress"`
	Message  string `json:"message,omitempty"`
}

func newRuntimeStatusProviders() map[backendruntime.Engine]runtimeStatusProvider {
	return map[backendruntime.Engine]runtimeStatusProvider{
		backendruntime.EngineGost: backendruntime.NewGostRuntimeClient(backendruntime.EngineGost),
		backendruntime.EngineDash: backendruntime.NewGostRuntimeClient(backendruntime.EngineDash),
	}
}

func newRuntimeSwitchStarter(r *repo.Repository, clients map[backendruntime.Engine]backendruntime.RuntimeClient) runtimeSwitchStarter {
	if r == nil {
		return nil
	}

	return backendruntime.NewSwitchOrchestrator(r, func() int64 { return time.Now().UnixMilli() }, clients)
}

func (h *Handler) runtimeSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.writeRuntimeSettings(w, r)
	case http.MethodPut:
		h.updateRuntimeSettings(w, r)
	default:
		response.WriteJSON(w, response.ErrDefault("请求失败"))
	}
}

func (h *Handler) runtimeProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	h.writeRuntimeSettings(w, r)
}

func (h *Handler) updateRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	var req runtimeSettingsUpdateRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}

	engine, ok := parseRuntimeEngine(req.Engine)
	if !ok {
		response.WriteJSON(w, response.ErrDefault("运行时引擎无效"))
		return
	}
	if h.runtimeSwitchStarter == nil {
		response.WriteJSON(w, response.Err(-2, "runtime switch orchestrator unavailable"))
		return
	}

	nodes, err := h.loadRuntimeNodes()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := h.runtimeSwitchStarter.Start(r.Context(), engine, nodes); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	h.writeRuntimeSettings(w, r)
}

func (h *Handler) writeRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	payload, err := h.buildRuntimeSettingsResponse(r.Context())
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(payload))
}

func (h *Handler) buildRuntimeSettingsResponse(ctx context.Context) (runtimeSettingsResponse, error) {
	if h == nil || h.repo == nil {
		return runtimeSettingsResponse{}, fmt.Errorf("repository not initialized")
	}

	engine, err := h.repo.GetRuntimeEngine()
	if err != nil {
		return runtimeSettingsResponse{}, err
	}
	state, err := h.repo.GetRuntimeSwitchState()
	if err != nil {
		return runtimeSettingsResponse{}, err
	}
	nodes, err := h.loadRuntimeNodes()
	if err != nil {
		return runtimeSettingsResponse{}, err
	}

	provider := h.runtimeStatusProviders[backendruntime.Engine(engine)]
	items := make([]runtimeNodeProgress, 0, len(nodes))
	summary := runtimeNodeSummary{Total: len(nodes)}
	runtimeProgress := runtimeProgressSummary{Engine: string(engine), State: string(backendruntime.ProgressStateSucceeded)}
	rebuildProgress := runtimeProgressSummary{Engine: string(engine), State: string(backendruntime.ProgressStatePending)}
	warnings := make([]string, 0)
	if provider == nil {
		runtimeProgress.State = string(backendruntime.ProgressStatePending)
		runtimeProgress.Message = fmt.Sprintf("runtime status unavailable for engine %q", engine)
	}

	for _, node := range nodes {
		status := runtimeNodeProgress{
			NodeID:   node.ID,
			NodeName: node.Name,
			Engine:   string(engine),
			Ready:    false,
			Progress: string(backendruntime.ProgressStatePending),
		}

		if provider != nil {
			nodeStatus, err := provider.GetNodeRuntimeStatus(ctx, node)
			if err != nil {
				status.Progress = string(backendruntime.ProgressStateFailed)
				status.Message = err.Error()
			} else {
				status.Engine = string(nodeStatus.Engine)
				status.Ready = nodeStatus.Ready
				status.Progress = string(nodeStatus.Progress)
				status.Message = nodeStatus.Message
			}
		}

		switch status.Progress {
		case string(backendruntime.ProgressStateFailed):
			summary.Failed++
			if runtimeProgress.State != string(backendruntime.ProgressStateFailed) {
				runtimeProgress.State = string(backendruntime.ProgressStateFailed)
				runtimeProgress.Message = status.Message
			}
		case string(backendruntime.ProgressStatePending), string(backendruntime.ProgressStateRunning):
			summary.Pending++
			if runtimeProgress.State == string(backendruntime.ProgressStateSucceeded) {
				runtimeProgress.State = string(backendruntime.ProgressStatePending)
				runtimeProgress.Message = status.Message
			}
		}
		if status.Progress == string(backendruntime.ProgressStateSucceeded) {
			summary.Completed++
		}
		if status.Ready {
			summary.Ready++
		}

		items = append(items, status)
	}
	if state.Status == repo.RuntimeSwitchStatusSwitching {
		runtimeProgress.State = string(state.Status)
		if h.runtimeSwitchStarter != nil {
			switchProgress := h.runtimeSwitchStarter.Progress()
			if switchProgress.Engine != "" {
				runtimeProgress.Engine = string(switchProgress.Engine)
				rebuildProgress.Engine = string(switchProgress.Engine)
			}
			if switchProgress.Status != "" {
				runtimeProgress.State = string(switchProgress.Status)
			}
			runtimeProgress.Phase = switchProgress.Phase
			if switchProgress.Message != "" {
				runtimeProgress.Message = switchProgress.Message
			}
			if switchProgress.Rebuild.State != "" {
				rebuildProgress.State = string(switchProgress.Rebuild.State)
			}
			if switchProgress.Rebuild.Message != "" {
				rebuildProgress.Message = switchProgress.Rebuild.Message
			}
			if len(switchProgress.Rebuild.Warnings) > 0 {
				warnings = append(warnings, switchProgress.Rebuild.Warnings...)
			}
			if len(switchProgress.Nodes) > 0 {
				items = make([]runtimeNodeProgress, 0, len(switchProgress.Nodes))
				summary = runtimeNodeSummary{Total: len(switchProgress.Nodes)}
				for _, nodeProgress := range switchProgress.Nodes {
					item := runtimeNodeProgress{
						NodeID:   nodeProgress.NodeID,
						NodeName: findRuntimeNodeName(nodes, nodeProgress.NodeID),
						Engine:   string(nodeProgress.Engine),
						Ready:    nodeProgress.Complete,
						Progress: string(nodeProgress.State),
						Message:  nodeProgress.Message,
					}
					switch item.Progress {
					case string(backendruntime.ProgressStateFailed):
						summary.Failed++
					case string(backendruntime.ProgressStatePending), string(backendruntime.ProgressStateRunning):
						summary.Pending++
					}
					if item.Progress == string(backendruntime.ProgressStateSucceeded) {
						summary.Completed++
					}
					if item.Ready {
						summary.Ready++
					}
					items = append(items, item)
				}
			}
		}
		if runtimeProgress.Message == "" {
			runtimeProgress.Message = "runtime switch in progress"
		}
		if rebuildProgress.Message == "" {
			rebuildProgress.Message = "waiting to rebuild runtime"
		}
	}

	if len(items) == 0 && runtimeProgress.Message == "" {
		runtimeProgress.Message = "no nodes registered"
	}

	return runtimeSettingsResponse{
		CurrentEngine:   string(engine),
		SwitchStatus:    string(state.Status),
		Generation:      state.Generation,
		LastError:       state.Error,
		NodeSummary:     summary,
		RuntimeProgress: runtimeProgress,
		RebuildProgress: rebuildProgress,
		Warnings:        warnings,
		Nodes:           items,
	}, nil
}

func findRuntimeNodeName(nodes []repo.Node, nodeID int64) string {
	for _, node := range nodes {
		if node.ID == nodeID {
			return node.Name
		}
	}
	return ""
}

func (h *Handler) loadRuntimeNodes() ([]repo.Node, error) {
	if h == nil || h.repo == nil {
		return nil, fmt.Errorf("repository not initialized")
	}

	var nodes []repo.Node
	if err := h.repo.DB().Order("inx ASC, id ASC").Find(&nodes).Error; err != nil {
		return nil, err
	}
	return nodes, nil
}

func (h *Handler) overlayCurrentRuntimeNodeStatuses(ctx context.Context, items []map[string]interface{}) {
	if h == nil || h.repo == nil || len(items) == 0 {
		return
	}
	engine, err := h.repo.GetRuntimeEngine()
	if err != nil {
		return
	}
	if engine == repo.RuntimeEngineGost {
		return
	}
	provider := h.runtimeStatusProviders[backendruntime.Engine(engine)]
	if provider == nil {
		return
	}
	nodes, err := h.loadRuntimeNodes()
	if err != nil {
		return
	}
	nodeByID := make(map[int64]repo.Node, len(nodes))
	for _, node := range nodes {
		nodeByID[node.ID] = node
	}
	for _, item := range items {
		nodeID, _ := item["id"].(int64)
		if nodeID <= 0 {
			continue
		}
		isRemote, _ := item["isRemote"].(int)
		if isRemote == 1 {
			continue
		}
		node, ok := nodeByID[nodeID]
		if !ok {
			continue
		}
		status, err := provider.GetNodeRuntimeStatus(ctx, node)
		if err != nil {
			_ = h.repo.UpdateNodeStatus(nodeID, 0)
			item["status"] = 0
			item["syncError"] = err.Error()
			item["runtimeEngine"] = string(engine)
			item["runtimeProgress"] = string(backendruntime.ProgressStateFailed)
			item["runtimeReady"] = false
			continue
		}
		item["runtimeEngine"] = string(status.Engine)
		item["runtimeProgress"] = string(status.Progress)
		item["runtimeReady"] = status.Ready
		if status.Ready {
			_ = h.repo.UpdateNodeStatus(nodeID, 1)
			item["status"] = 1
			if _, ok := item["syncError"]; ok {
				delete(item, "syncError")
			}
		} else {
			_ = h.repo.UpdateNodeStatus(nodeID, 0)
			item["status"] = 0
			if strings.TrimSpace(status.Message) != "" {
				item["syncError"] = status.Message
			}
		}
	}
}

func parseRuntimeEngine(value string) (backendruntime.Engine, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(backendruntime.EngineGost):
		return backendruntime.EngineGost, true
	case string(backendruntime.EngineDash):
		return backendruntime.EngineDash, true
	default:
		return "", false
	}
}
