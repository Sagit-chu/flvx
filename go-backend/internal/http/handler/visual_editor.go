package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"go-backend/internal/http/response"
)

const (
	visualGraphConfigKey    = "visual_editor_graph"
	visualLinkTestTimeout   = 10 * time.Second
	visualTraceMode         = "simulated_topology"
	visualProbeStatusUp     = "online"
	visualProbeStatusDown   = "offline"
	visualLinkModeNodeTCP   = "node_tcp_ping"
	visualLinkModeDirectTCP = "direct_tcp_ping"
)

// VisualGraphPayload stores the serialized React Flow layout.
type VisualGraphPayload struct {
	GraphJSON string `json:"graph_json"`
}

type VisualLinkTestRequest struct {
	SourceNodeID int64  `json:"sourceNodeId"`
	TargetNodeID int64  `json:"targetNodeId"`
	Target       string `json:"target"`
	Port         int    `json:"port"`
	IPPreference string `json:"ipPreference"`
}

type visualLinkTraceHop struct {
	Hop      int    `json:"hop"`
	Kind     string `json:"kind"`
	NodeID   int64  `json:"nodeId,omitempty"`
	NodeName string `json:"nodeName,omitempty"`
	Host     string `json:"host"`
	Port     int    `json:"port,omitempty"`
}

type visualLinkTestResult struct {
	Mode           string               `json:"mode"`
	SourceNodeID   int64                `json:"sourceNodeId"`
	SourceNodeName string               `json:"sourceNodeName"`
	TargetNodeID   int64                `json:"targetNodeId,omitempty"`
	TargetNodeName string               `json:"targetNodeName,omitempty"`
	TargetHost     string               `json:"targetHost"`
	TargetPort     int                  `json:"targetPort"`
	Success        bool                 `json:"success"`
	AverageTime    float64              `json:"averageTime"`
	PacketLoss     float64              `json:"packetLoss"`
	Message        string               `json:"message"`
	PingOutput     string               `json:"ping_output"`
	TraceOutput    string               `json:"trace_output"`
	TraceMode      string               `json:"trace_mode"`
	Simulated      bool                 `json:"simulated"`
	TraceHops      []visualLinkTraceHop `json:"trace_hops"`
}

func (h *Handler) visualGraphHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := h.repo.GetConfigByName(visualGraphConfigKey)
		if err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}

		graphJSON := "{}"
		if cfg != nil && strings.TrimSpace(cfg.Value) != "" {
			graphJSON = cfg.Value
		}

		response.WriteJSON(w, response.OK(map[string]interface{}{"graph_json": graphJSON}))
	case http.MethodPost:
		var req VisualGraphPayload
		if err := decodeJSON(r.Body, &req); err != nil {
			response.WriteJSON(w, response.ErrDefault("invalid JSON payload"))
			return
		}

		graphJSON := strings.TrimSpace(req.GraphJSON)
		if graphJSON == "" {
			graphJSON = "{}"
		}
		if !json.Valid([]byte(graphJSON)) {
			response.WriteJSON(w, response.ErrDefault("graph_json must be valid JSON"))
			return
		}

		if err := h.repo.UpsertConfig(visualGraphConfigKey, graphJSON, time.Now().UnixMilli()); err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}

		response.WriteJSON(w, response.OK(map[string]interface{}{"graph_json": graphJSON}))
	default:
		response.WriteJSON(w, response.ErrDefault("method not allowed"))
	}
}

func (h *Handler) visualProbeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("method not allowed"))
		return
	}

	nodeID, err := parseVisualNodeID(r.URL.Path)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("invalid node id"))
		return
	}

	node, err := h.repo.GetNodeByID(nodeID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if node == nil {
		response.WriteJSON(w, response.ErrDefault("node not found"))
		return
	}

	occupiedPorts, err := h.repo.ListUsedPortsOnNode(nodeID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	sort.Ints(occupiedPorts)

	metric, err := h.repo.GetLatestNodeMetric(nodeID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	payload := map[string]interface{}{
		"node_id":             nodeID,
		"node_name":           node.Name,
		"status":              visualProbeStatus(node.Status),
		"server_ip":           node.ServerIP,
		"allowed_port":        node.Port,
		"allowed_ports":       node.Port,
		"occupied_ports":      occupiedPorts,
		"occupied_port_count": len(occupiedPorts),
		"cpu_usage":           0.0,
		"mem_usage":           0.0,
		"disk_usage":          0.0,
		"connections":         int64(0),
		"tcp_connections":     int64(0),
		"udp_connections":     int64(0),
		"load1":               0.0,
		"load5":               0.0,
		"load15":              0.0,
		"net_in_speed":        int64(0),
		"net_out_speed":       int64(0),
		"uptime":              int64(0),
		"metric_timestamp":    int64(0),
	}

	if metric != nil {
		payload["cpu_usage"] = metric.CPUUsage
		payload["mem_usage"] = metric.MemUsage
		payload["disk_usage"] = metric.DiskUsage
		payload["tcp_connections"] = metric.TCPConns
		payload["udp_connections"] = metric.UDPConns
		payload["connections"] = metric.TCPConns + metric.UDPConns
		payload["load1"] = metric.Load1
		payload["load5"] = metric.Load5
		payload["load15"] = metric.Load15
		payload["net_in_speed"] = metric.NetInSpeed
		payload["net_out_speed"] = metric.NetOutSpeed
		payload["uptime"] = metric.Uptime
		payload["metric_timestamp"] = metric.Timestamp
	}

	response.WriteJSON(w, response.OK(payload))
}

func (h *Handler) visualLinkTestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("method not allowed"))
		return
	}

	var req VisualLinkTestRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("invalid request payload"))
		return
	}

	if req.SourceNodeID <= 0 {
		response.WriteJSON(w, response.ErrDefault("sourceNodeId is required"))
		return
	}

	sourceNode, err := h.getNodeRecord(req.SourceNodeID)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}

	targetNode, targetHost, targetPort, err := h.resolveVisualLinkTarget(sourceNode, req)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}

	pingData, pingErr := h.runVisualLinkPing(sourceNode, targetHost, targetPort)
	result := buildVisualLinkTestResult(sourceNode, targetNode, targetHost, targetPort, pingData, pingErr)
	response.WriteJSON(w, response.OK(result))
}

func parseVisualNodeID(path string) (int64, error) {
	path = strings.TrimSpace(path)
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return 0, fmt.Errorf("empty path")
	}

	parts := strings.Split(path, "/")
	idStr := strings.TrimSpace(parts[len(parts)-1])
	if idStr == "" {
		return 0, fmt.Errorf("missing id")
	}

	nodeID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || nodeID <= 0 {
		return 0, fmt.Errorf("invalid id")
	}
	return nodeID, nil
}

func visualProbeStatus(status int) string {
	if status == 1 {
		return visualProbeStatusUp
	}
	return visualProbeStatusDown
}

func (h *Handler) resolveVisualLinkTarget(sourceNode *nodeRecord, req VisualLinkTestRequest) (*nodeRecord, string, int, error) {
	if req.TargetNodeID > 0 {
		targetNode, err := h.getNodeRecord(req.TargetNodeID)
		if err != nil {
			return nil, "", 0, err
		}

		targetHost, targetPort, err := resolveChainProbeTarget(sourceNode, targetNode, req.Port, strings.TrimSpace(req.IPPreference), "")
		if err != nil {
			return nil, "", 0, err
		}
		return targetNode, targetHost, targetPort, nil
	}

	targetHost, targetPort, err := parseVisualLinkHostPort(req.Target, req.Port)
	if err != nil {
		return nil, "", 0, err
	}
	return nil, targetHost, targetPort, nil
}

func parseVisualLinkHostPort(target string, fallbackPort int) (string, int, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", 0, fmt.Errorf("target or targetNodeId is required")
	}

	host, port, err := parseTargetAddress(target)
	if err == nil {
		return host, port, nil
	}

	if fallbackPort <= 0 || fallbackPort > 65535 {
		return "", 0, fmt.Errorf("target must be in host:port form or include a valid port")
	}

	host = strings.Trim(strings.TrimSpace(target), "[]")
	if host == "" {
		return "", 0, fmt.Errorf("invalid target host")
	}
	return host, fallbackPort, nil
}

func (h *Handler) runVisualLinkPing(sourceNode *nodeRecord, targetHost string, targetPort int) (map[string]interface{}, error) {
	options := diagnosisExecOptions{
		commandTimeout: visualLinkTestTimeout,
		pingTimeoutMS:  int(visualLinkTestTimeout / time.Millisecond),
		timeoutMessage: "visual link test timed out",
	}

	if sourceNode != nil && sourceNode.IsRemote == 1 {
		return h.tcpPingViaRemoteNode(sourceNode, targetHost, targetPort, options)
	}
	return h.tcpPingViaNode(sourceNode.ID, targetHost, targetPort, options)
}

func buildVisualLinkTestResult(sourceNode, targetNode *nodeRecord, targetHost string, targetPort int, pingData map[string]interface{}, pingErr error) visualLinkTestResult {
	result := visualLinkTestResult{
		Mode:           visualLinkModeNodeTCP,
		SourceNodeID:   sourceNode.ID,
		SourceNodeName: visualNodeName(sourceNode),
		TargetHost:     targetHost,
		TargetPort:     targetPort,
		TraceMode:      visualTraceMode,
		Simulated:      true,
		Success:        false,
		PacketLoss:     100,
		Message:        "tcp probe failed",
	}

	if targetNode != nil {
		result.TargetNodeID = targetNode.ID
		result.TargetNodeName = visualNodeName(targetNode)
	} else {
		result.Mode = visualLinkModeDirectTCP
	}

	if pingErr != nil {
		result.Message = strings.TrimSpace(pingErr.Error())
	}

	if pingData != nil {
		result.Success = asBool(pingData["success"], false)
		result.AverageTime = asFloat(pingData["averageTime"], 0)
		result.PacketLoss = asFloat(pingData["packetLoss"], 100)

		message := strings.TrimSpace(asString(pingData["message"]))
		if message == "" && !result.Success {
			message = strings.TrimSpace(asString(pingData["errorMessage"]))
		}
		if message != "" {
			result.Message = message
		} else if result.Success {
			result.Message = "tcp probe completed"
		}
	}

	result.TraceHops = buildVisualLinkTraceHops(sourceNode, targetNode, targetHost, targetPort)
	result.PingOutput = buildVisualPingOutput(result)
	result.TraceOutput = buildVisualTraceOutput(result)
	return result
}

func buildVisualLinkTraceHops(sourceNode, targetNode *nodeRecord, targetHost string, targetPort int) []visualLinkTraceHop {
	hops := []visualLinkTraceHop{
		{
			Hop:      1,
			Kind:     "source",
			NodeID:   sourceNode.ID,
			NodeName: visualNodeName(sourceNode),
			Host:     strings.Trim(strings.TrimSpace(sourceNode.ServerIP), "[]"),
		},
	}

	targetHop := visualLinkTraceHop{
		Hop:  2,
		Kind: "target",
		Host: targetHost,
		Port: targetPort,
	}
	if targetNode != nil {
		targetHop.NodeID = targetNode.ID
		targetHop.NodeName = visualNodeName(targetNode)
	}
	hops = append(hops, targetHop)
	return hops
}

func buildVisualPingOutput(result visualLinkTestResult) string {
	lines := []string{
		fmt.Sprintf("mode: %s", result.Mode),
		fmt.Sprintf("source: %s (%d)", result.SourceNodeName, result.SourceNodeID),
		fmt.Sprintf("target: %s:%d", result.TargetHost, result.TargetPort),
		fmt.Sprintf("success: %t", result.Success),
		fmt.Sprintf("avg_ms: %.2f", result.AverageTime),
		fmt.Sprintf("packet_loss: %.2f", result.PacketLoss),
		fmt.Sprintf("message: %s", result.Message),
	}
	return strings.Join(lines, "\n")
}

func buildVisualTraceOutput(result visualLinkTestResult) string {
	lines := make([]string, 0, len(result.TraceHops)+2)
	for _, hop := range result.TraceHops {
		label := hop.Host
		if hop.Port > 0 {
			label = fmt.Sprintf("%s:%d", label, hop.Port)
		}
		if hop.NodeName != "" {
			label = fmt.Sprintf("%s [%s]", label, hop.NodeName)
		}
		lines = append(lines, fmt.Sprintf("%d  %s  (%s)", hop.Hop, label, hop.Kind))
	}
	lines = append(lines, fmt.Sprintf("mode: %s", result.TraceMode))
	lines = append(lines, "note: hop-by-hop traceroute is simulated from the selected visual graph path")
	return strings.Join(lines, "\n")
}

func visualNodeName(node *nodeRecord) string {
	if node == nil {
		return ""
	}
	name := strings.TrimSpace(node.Name)
	if name != "" {
		return name
	}
	return fmt.Sprintf("node-%d", node.ID)
}
