package handler

import (
	"bytes"
	"context"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"go-backend/internal/http/response"
	"go-backend/internal/store/model"
)

// VisualGraphPayload represents the React Flow generic storage
type VisualGraphPayload struct {
	GraphJSON string `json:"graph_json"`
}

func (h *Handler) visualGraphHandler(w http.ResponseWriter, r *http.Request) {
	configKey := "visual_editor_graph"

	if r.Method == http.MethodGet {
		cfg, err := h.repo.GetConfigByName(configKey)
		if err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
		val := "{}"
		if cfg != nil {
			val = cfg.Value
		}
		response.WriteJSON(w, response.OK(map[string]interface{}{"graph_json": val}))
		return
	}

	if r.Method == http.MethodPost {
		var req VisualGraphPayload
		if err := decodeJSON(r.Body, &req); err != nil {
			response.WriteJSON(w, response.ErrDefault("无效的 JSON payload"))
			return
		}

		if req.GraphJSON == "" {
			req.GraphJSON = "{}"
		}

		err := h.repo.UpsertConfig(configKey, req.GraphJSON, time.Now().UnixMilli())
		if err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}

		response.WriteJSON(w, response.OK("success"))
		return
	}

	response.WriteJSON(w, response.ErrDefault("不支持的方法"))
}

func (h *Handler) visualProbeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("仅支持 POST 方法"))
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) == 0 {
		response.WriteJSON(w, response.ErrDefault("无效 Node ID"))
		return
	}
	idStr := parts[len(parts)-1]
	if idStr == "" && len(parts) > 1 {
		idStr = parts[len(parts)-2]
	}

	nodeID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || nodeID <= 0 {
		response.WriteJSON(w, response.ErrDefault("无效 Node ID"))
		return
	}

	node, err := h.repo.GetNodeByID(nodeID)
	if err != nil || node == nil {
		response.WriteJSON(w, response.ErrDefault("Node 未找到"))
		return
	}

	// Fetch occupied ports
	// We can query forward_port for this node
	db := h.repo.DB()
	var forwardPorts []model.ForwardPort
	db.Where("node_id = ?", nodeID).Find(&forwardPorts)

	var occupiedPorts []int
	for _, fp := range forwardPorts {
		occupiedPorts = append(occupiedPorts, fp.Port)
	}

	// Realtime metrics (Latest memory / CPU / Status)
	isOnline := node.Status == 1
	var lastMetric model.NodeMetric
	h.repo.DB().Where("node_id = ?", nodeID).Order("timestamp DESC").First(&lastMetric)
	
	cpu := lastMetric.CPUUsage
	mem := lastMetric.MemUsage
	conns := lastMetric.TCPConns + lastMetric.UDPConns

	response.WriteJSON(w, response.OK(map[string]interface{}{
		"node_id":         nodeID,
		"status":          func() string { if isOnline { return "online" }; return "offline" }(),
		"cpu_usage":       cpu,
		"mem_usage":       mem,
		"connections":     conns,
		"occupied_ports":  occupiedPorts,
		"allowed_port":    node.Port,
	}))
}

type LinkTestRequest struct {
	Target string `json:"target"` // IP or hostname
}

func (h *Handler) visualLinkTestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求方法不允许"))
		return
	}

	var req LinkTestRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("参数错误"))
		return
	}

	target := strings.TrimSpace(req.Target)
	if target == "" {
		response.WriteJSON(w, response.ErrDefault("测试目标不能为空"))
		return
	}

	// Timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var pingCmd *exec.Cmd
	var traceCmd *exec.Cmd

	if runtime.GOOS == "windows" {
		pingCmd = exec.CommandContext(ctx, "ping", "-n", "3", "-w", "1000", target)
		traceCmd = exec.CommandContext(ctx, "tracert", "-d", "-h", "15", "-w", "500", target)
	} else {
		pingCmd = exec.CommandContext(ctx, "ping", "-c", "3", "-W", "1", target)
		traceCmd = exec.CommandContext(ctx, "traceroute", "-m", "15", "-w", "1", "-n", target)
	}

	// Capture output
	var pingOut bytes.Buffer
	pingCmd.Stdout = &pingOut
	pingCmd.Run() // Error doesn't matter, we parse output

	var traceOut bytes.Buffer
	traceCmd.Stdout = &traceOut
	traceCmd.Run()

	response.WriteJSON(w, response.OK(map[string]interface{}{
		"target": target,
		"ping_output": pingOut.String(),
		"trace_output": traceOut.String(),
	}))
}
