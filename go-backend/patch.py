import re

with open('internal/http/handler/control_plane.go', 'r') as f:
    content = f.read()

# 1. Update ensureLimiterOnNode and add ensureConnLimiterOnNode
ensure_conn_limiter = """
func (h *Handler) ensureConnLimiterOnNode(nodeID int64, limiterName string, maxConn int) error {
\tlimitStr := fmt.Sprintf("$ %d", maxConn)
\t
\tpayload := map[string]interface{}{
\t\t"name":   limiterName,
\t\t"limits": []string{limitStr},
\t}
\t
\tif _, err := h.sendNodeCommand(nodeID, "AddCLimiters", payload, false, false); err != nil {
\t\tif !isAlreadyExistsMessage(err.Error()) {
\t\t\treturn fmt.Errorf("连接限制器下发失败: %w", err)
\t\t}
\t\tupdatePayload := map[string]interface{}{
\t\t\t"limiter": limiterName,
\t\t\t"data":    payload,
\t\t}
\t\tif _, updateErr := h.sendNodeCommand(nodeID, "UpdateCLimiters", updatePayload, false, false); updateErr != nil {
\t\t\treturn fmt.Errorf("连接限制器更新失败: %w", updateErr)
\t\t}
\t}
\treturn nil
}
"""

content = content.replace('func (h *Handler) ensureLimiterOnNode(nodeID int64, limiterID int64, speed int) error {\n\tif err := h.upsertLimiterOnNode(nodeID, limiterID, speed); err != nil {\n\t\treturn fmt.Errorf("限速规则下发失败: %w", err)\n\t}\n\n\treturn nil\n}',
'func (h *Handler) ensureLimiterOnNode(nodeID int64, limiterID int64, speed int) error {\n\tif err := h.upsertLimiterOnNode(nodeID, limiterID, speed); err != nil {\n\t\treturn fmt.Errorf("限速规则下发失败: %w", err)\n\t}\n\n\treturn nil\n}\n' + ensure_conn_limiter)


# 2. Update buildForwardServiceConfigs declaration
content = content.replace(
    'func buildForwardServiceConfigs(baseName string, forward *forwardRecord, tunnel *tunnelRecord, node *nodeRecord, port int, bindIP string, limiterID *int64) []map[string]interface{} {',
    'func buildForwardServiceConfigs(baseName string, forward *forwardRecord, tunnel *tunnelRecord, node *nodeRecord, port int, bindIP string, limiterID *int64, cLimiterName string) []map[string]interface{} {'
)


# 3. Inject climiter into generated service
service_map_end = """		}
		if protocol == "udp" {"""
service_map_end_new = """		}
		if cLimiterName != "" {
			service["climiter"] = cLimiterName
		}
		if protocol == "udp" {"""
content = content.replace(service_map_end, service_map_end_new)


# 4. Update syncForwardServicesWithWarnings
# Find user tunnel resolution
resolution = """	serviceBase := buildForwardServiceBaseWithResolvedUserTunnel(forward.ID, forward.UserID, userTunnelID)

	for _, fp := range ports {"""

resolution_new = """	serviceBase := buildForwardServiceBaseWithResolvedUserTunnel(forward.ID, forward.UserID, userTunnelID)

	user, err := h.repo.GetUserByID(forward.UserID)
	if err != nil {
		return nil, err
	}

	var cLimiterName string
	var maxConnToSet int

	if forward.MaxConn > 0 {
		maxConnToSet = forward.MaxConn
		cLimiterName = fmt.Sprintf("rule_conn_limit_%d", forward.ID)
	} else if user != nil && user.MaxConn > 0 {
		maxConnToSet = user.MaxConn
		cLimiterName = fmt.Sprintf("user_conn_limit_%d", user.ID)
	}

	for _, fp := range ports {"""
content = content.replace(resolution, resolution_new)

# Inject ensureConnLimiterOnNode inside loop
loop_inner = """		if limiterID != nil && speed != nil {
			if err := h.ensureLimiterOnNode(fp.NodeID, *limiterID, *speed); err != nil {
				// If the limiter push fails because the node is offline, skip it with a warning
				if isNodeOfflineOrTimeoutError(err) {
					node, _ := h.getNodeRecord(fp.NodeID)
					nodeName := fmt.Sprintf("%d", fp.NodeID)
					if node != nil && strings.TrimSpace(node.Name) != "" {
						nodeName = strings.TrimSpace(node.Name)
					}
					warnings = append(warnings, fmt.Sprintf("节点 %s 不在线，已跳过下发", nodeName))
					continue
				}
				return nil, err
			}
		}

		node, err := h.getNodeRecord(fp.NodeID)"""

loop_inner_new = """		if limiterID != nil && speed != nil {
			if err := h.ensureLimiterOnNode(fp.NodeID, *limiterID, *speed); err != nil {
				// If the limiter push fails because the node is offline, skip it with a warning
				if isNodeOfflineOrTimeoutError(err) {
					node, _ := h.getNodeRecord(fp.NodeID)
					nodeName := fmt.Sprintf("%d", fp.NodeID)
					if node != nil && strings.TrimSpace(node.Name) != "" {
						nodeName = strings.TrimSpace(node.Name)
					}
					warnings = append(warnings, fmt.Sprintf("节点 %s 不在线，已跳过下发", nodeName))
					continue
				}
				return nil, err
			}
		}

		if cLimiterName != "" {
			if err := h.ensureConnLimiterOnNode(fp.NodeID, cLimiterName, maxConnToSet); err != nil {
				warnings = append(warnings, fmt.Sprintf("节点 %d 连接限制器下发失败: %v", fp.NodeID, err))
			}
		}

		node, err := h.getNodeRecord(fp.NodeID)"""
content = content.replace(loop_inner, loop_inner_new)

# Update buildForwardServiceConfigs call in syncForwardServicesWithWarnings
content = content.replace(
    'services := buildForwardServiceConfigs(serviceBase, forward, tunnel, node, fp.Port, strings.TrimSpace(fp.InIP), limiterID)',
    'services := buildForwardServiceConfigs(serviceBase, forward, tunnel, node, fp.Port, strings.TrimSpace(fp.InIP), limiterID, cLimiterName)'
)

# Update fallbackForwardPortToDefaultBind call
content = content.replace(
    'warning, err = h.fallbackForwardPortToDefaultBind(forward, tunnel, node, fp, serviceBase, limiterID)',
    'warning, err = h.fallbackForwardPortToDefaultBind(forward, tunnel, node, fp, serviceBase, limiterID, cLimiterName)'
)

# 5. Update fallbackForwardPortToDefaultBind declaration and logic
content = content.replace(
    'func (h *Handler) fallbackForwardPortToDefaultBind(forward *forwardRecord, tunnel *tunnelRecord, node *nodeRecord, fp forwardPortRecord, serviceBase string, limiterID *int64) (string, error) {',
    'func (h *Handler) fallbackForwardPortToDefaultBind(forward *forwardRecord, tunnel *tunnelRecord, node *nodeRecord, fp forwardPortRecord, serviceBase string, limiterID *int64, cLimiterName string) (string, error) {'
)

content = content.replace(
    'defaultServices := buildForwardServiceConfigs(serviceBase, forward, tunnel, node, fp.Port, "", limiterID)',
    'defaultServices := buildForwardServiceConfigs(serviceBase, forward, tunnel, node, fp.Port, "", limiterID, cLimiterName)'
)

with open('internal/http/handler/control_plane.go', 'w') as f:
    f.write(content)


# Update control_plane_test.go
with open('internal/http/handler/control_plane_test.go', 'r') as f:
    test_content = f.read()

test_content = re.sub(
    r'buildForwardServiceConfigs\((.*?),(.*?),(.*?),(.*?),(.*?),(.*?),(.*?)\)',
    r'buildForwardServiceConfigs(\1,\2,\3,\4,\5,\6,\7, "")',
    test_content
)

with open('internal/http/handler/control_plane_test.go', 'w') as f:
    f.write(test_content)
