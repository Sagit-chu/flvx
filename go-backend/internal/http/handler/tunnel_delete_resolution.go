package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go-backend/internal/http/response"
)

const tunnelDeletePreviewSampleLimit = 5

const (
	tunnelDeleteActionReplace        = "replace"
	tunnelDeleteActionDeleteForwards = "delete_forwards"
)

var (
	errInvalidTunnelDeleteTarget = errors.New("invalid tunnel delete target")
)

type tunnelDeleteForwardPreviewItem struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	UserID   int64  `json:"userId"`
	UserName string `json:"userName"`
	InPort   int    `json:"inPort"`
}

type tunnelDeletePreviewData struct {
	TunnelID       int64                            `json:"tunnelId"`
	TunnelName     string                           `json:"tunnelName"`
	ForwardCount   int                              `json:"forwardCount"`
	SampleForwards []tunnelDeleteForwardPreviewItem `json:"sampleForwards"`
}

type tunnelBatchDeletePreviewData struct {
	TunnelCount       int                       `json:"tunnelCount"`
	TotalForwardCount int                       `json:"totalForwardCount"`
	Items             []tunnelDeletePreviewData `json:"items"`
}

type tunnelDeleteWithForwardsRequest struct {
	ID             int64  `json:"id"`
	Action         string `json:"action"`
	TargetTunnelID int64  `json:"targetTunnelId"`
}

type tunnelBatchDeleteWithForwardsRequest struct {
	IDs            []int64 `json:"ids"`
	Action         string  `json:"action"`
	TargetTunnelID int64   `json:"targetTunnelId"`
}

type tunnelDeleteWithForwardsResult struct {
	ForwardCount        int      `json:"forwardCount"`
	MigratedCount       int      `json:"migratedCount"`
	DeletedForwardCount int      `json:"deletedForwardCount"`
	PortAdjustedCount   int      `json:"portAdjustedCount"`
	Warnings            []string `json:"warnings,omitempty"`
}

type tunnelBatchDeleteWithForwardsResult struct {
	SuccessCount        int                  `json:"successCount"`
	FailCount           int                  `json:"failCount"`
	Failures            []batchFailureDetail `json:"failures,omitempty"`
	DeletedForwardCount int                  `json:"deletedForwardCount"`
	MigratedCount       int                  `json:"migratedCount"`
	PortAdjustedCount   int                  `json:"portAdjustedCount"`
	Warnings            []string             `json:"warnings,omitempty"`
}

type tunnelForwardMigrationPlan struct {
	forward        *forwardRecord
	oldPorts       []forwardPortRecord
	targetTunnelID int64
	targetPort     int
	keptNodeIDs    []int64
	removedNodeIDs []int64
	portAdjusted   bool
}

func (h *Handler) tunnelDeletePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	id := idFromBody(r, w)
	if id <= 0 {
		return
	}

	preview, err := h.buildTunnelDeletePreview(id)
	if err != nil {
		if strings.Contains(err.Error(), "不存在") {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OK(preview))
}

func (h *Handler) tunnelBatchDeletePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := decodeJSON(r.Body, &req); err != nil || len(req.IDs) == 0 {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}

	preview, err := h.buildTunnelBatchDeletePreview(req.IDs)
	if err != nil {
		if strings.Contains(err.Error(), "不存在") {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OK(preview))
}

func (h *Handler) tunnelDeleteWithForwards(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req tunnelDeleteWithForwardsRequest
	if err := decodeJSON(r.Body, &req); err != nil || req.ID <= 0 {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}

	action, err := normalizeTunnelDeleteAction(req.Action)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if action == tunnelDeleteActionReplace {
		if _, _, authErr := userRoleFromRequest(r); authErr != nil {
			response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
			return
		}
	}

	result, failures, err := h.processTunnelDeleteWithForwards(req.ID, action, req.TargetTunnelID)
	if err != nil {
		if err == errInvalidTunnelDeleteTarget {
			response.WriteJSON(w, response.ErrDefault("目标隧道不能为空"))
			return
		}
		if strings.Contains(err.Error(), "目标隧道不能与当前隧道相同") || strings.Contains(err.Error(), "目标隧道不存在") || strings.Contains(err.Error(), "目标隧道已禁用") || strings.Contains(err.Error(), "隧道不存在") {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if len(failures) > 0 {
		response.WriteJSON(w, response.R{
			Code: -2,
			Msg:  "部分规则迁移失败",
			TS:   time.Now().UnixMilli(),
			Data: batchOperationResult{SuccessCount: 0, FailCount: len(failures), Failures: failures},
		})
		return
	}

	response.WriteJSON(w, response.OK(result))
}

func (h *Handler) tunnelBatchDeleteWithForwards(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req tunnelBatchDeleteWithForwardsRequest
	if err := decodeJSON(r.Body, &req); err != nil || len(req.IDs) == 0 {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}

	action, err := normalizeTunnelDeleteAction(req.Action)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if action == tunnelDeleteActionReplace {
		if _, _, authErr := userRoleFromRequest(r); authErr != nil {
			response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
			return
		}
	}

	normalizedIDs := normalizeTunnelIDs(req.IDs)
	if len(normalizedIDs) == 0 {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}

	if action == tunnelDeleteActionReplace {
		if req.TargetTunnelID <= 0 {
			response.WriteJSON(w, response.ErrDefault("目标隧道不能为空"))
			return
		}
		for _, id := range normalizedIDs {
			if id == req.TargetTunnelID {
				response.WriteJSON(w, response.ErrDefault("目标隧道不能包含在删除列表中"))
				return
			}
		}
	}

	result := tunnelBatchDeleteWithForwardsResult{}
	for _, tunnelID := range normalizedIDs {
		tunnelName, _ := h.repo.GetTunnelName(tunnelID)
		singleResult, failures, processErr := h.processTunnelDeleteWithForwards(tunnelID, action, req.TargetTunnelID)
		if processErr != nil {
			result.FailCount++
			result.Failures = appendBatchFailure(result.Failures, tunnelID, tunnelName, processErr)
			continue
		}
		if len(failures) > 0 {
			result.FailCount++
			result.Failures = appendBatchFailureReason(
				result.Failures,
				tunnelID,
				tunnelName,
				summarizeTunnelDeleteRuleFailures(failures),
			)
			continue
		}

		result.SuccessCount++
		result.DeletedForwardCount += singleResult.DeletedForwardCount
		result.MigratedCount += singleResult.MigratedCount
		result.PortAdjustedCount += singleResult.PortAdjustedCount
		if len(singleResult.Warnings) > 0 {
			result.Warnings = append(result.Warnings, singleResult.Warnings...)
		}
	}

	response.WriteJSON(w, response.OK(result))
}

func (h *Handler) buildTunnelDeletePreview(tunnelID int64) (*tunnelDeletePreviewData, error) {
	if _, err := h.getTunnelRecord(tunnelID); err != nil {
		return nil, err
	}

	tunnelName, err := h.repo.GetTunnelName(tunnelID)
	if err != nil {
		return nil, err
	}

	forwards, err := h.listForwardsByTunnel(tunnelID)
	if err != nil {
		return nil, err
	}

	samples := make([]tunnelDeleteForwardPreviewItem, 0, minInt(len(forwards), tunnelDeletePreviewSampleLimit))
	for i, forward := range forwards {
		if i >= tunnelDeletePreviewSampleLimit {
			break
		}
		ports, portsErr := h.listForwardPorts(forward.ID)
		if portsErr != nil {
			return nil, portsErr
		}
		inPort := 0
		if len(ports) > 0 {
			inPort = ports[0].Port
		}
		samples = append(samples, tunnelDeleteForwardPreviewItem{
			ID:       forward.ID,
			Name:     forward.Name,
			UserID:   forward.UserID,
			UserName: forward.UserName,
			InPort:   inPort,
		})
	}

	return &tunnelDeletePreviewData{
		TunnelID:       tunnelID,
		TunnelName:     tunnelName,
		ForwardCount:   len(forwards),
		SampleForwards: samples,
	}, nil
}

func (h *Handler) buildTunnelBatchDeletePreview(ids []int64) (*tunnelBatchDeletePreviewData, error) {
	normalizedIDs := normalizeTunnelIDs(ids)
	items := make([]tunnelDeletePreviewData, 0, len(normalizedIDs))
	totalForwardCount := 0
	for _, id := range normalizedIDs {
		preview, err := h.buildTunnelDeletePreview(id)
		if err != nil {
			return nil, err
		}
		items = append(items, *preview)
		totalForwardCount += preview.ForwardCount
	}
	return &tunnelBatchDeletePreviewData{
		TunnelCount:       len(items),
		TotalForwardCount: totalForwardCount,
		Items:             items,
	}, nil
}

func normalizeTunnelDeleteAction(action string) (string, error) {
	normalized := strings.TrimSpace(action)
	if normalized == "" {
		return tunnelDeleteActionDeleteForwards, nil
	}
	if normalized != tunnelDeleteActionReplace && normalized != tunnelDeleteActionDeleteForwards {
		return "", errors.New("invalid tunnel delete action")
	}
	return normalized, nil
}

func normalizeTunnelIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func summarizeTunnelDeleteRuleFailures(failures []batchFailureDetail) string {
	if len(failures) == 0 {
		return "未知错误"
	}
	parts := make([]string, 0, minInt(len(failures), 3))
	for i, failure := range failures {
		if i >= 3 {
			break
		}
		name := strings.TrimSpace(failure.Name)
		if name == "" {
			name = fmt.Sprintf("规则 #%d", failure.ID)
		}
		parts = append(parts, fmt.Sprintf("%s: %s", name, strings.TrimSpace(failure.Reason)))
	}
	if len(failures) > 3 {
		parts = append(parts, fmt.Sprintf("另有 %d 条规则失败", len(failures)-3))
	}
	return strings.Join(parts, "；")
}

func (h *Handler) processTunnelDeleteWithForwards(tunnelID int64, action string, targetTunnelID int64) (tunnelDeleteWithForwardsResult, []batchFailureDetail, error) {
	preview, err := h.buildTunnelDeletePreview(tunnelID)
	if err != nil {
		return tunnelDeleteWithForwardsResult{}, nil, err
	}

	result := tunnelDeleteWithForwardsResult{ForwardCount: preview.ForwardCount}
	if preview.ForwardCount == 0 {
		if err := h.deleteTunnelAndCleanup(tunnelID); err != nil {
			return tunnelDeleteWithForwardsResult{}, nil, err
		}
		return result, nil, nil
	}

	if action == tunnelDeleteActionDeleteForwards {
		result.DeletedForwardCount = preview.ForwardCount
		if err := h.deleteTunnelAndCleanup(tunnelID); err != nil {
			return tunnelDeleteWithForwardsResult{}, nil, err
		}
		return result, nil, nil
	}

	if targetTunnelID <= 0 {
		return tunnelDeleteWithForwardsResult{}, nil, errInvalidTunnelDeleteTarget
	}
	if targetTunnelID == tunnelID {
		return tunnelDeleteWithForwardsResult{}, nil, errors.New("目标隧道不能与当前隧道相同")
	}
	return h.processTunnelDeleteReplaceAction(tunnelID, targetTunnelID, result)
}

func (h *Handler) processTunnelDeleteReplaceAction(tunnelID, targetTunnelID int64, result tunnelDeleteWithForwardsResult) (tunnelDeleteWithForwardsResult, []batchFailureDetail, error) {
	targetTunnel, err := h.getTunnelRecord(targetTunnelID)
	if err != nil {
		return tunnelDeleteWithForwardsResult{}, nil, errors.New("目标隧道不存在")
	}
	if targetTunnel.Status != 1 {
		return tunnelDeleteWithForwardsResult{}, nil, errors.New("目标隧道已禁用")
	}

	plans, failures, err := h.planTunnelDeleteForwardMigrations(tunnelID, targetTunnelID)
	if err != nil {
		return tunnelDeleteWithForwardsResult{}, nil, err
	}
	if len(failures) > 0 {
		return tunnelDeleteWithForwardsResult{}, failures, nil
	}

	portAdjustedCount := 0
	warnings, execErr, execFailure := h.executeTunnelDeleteForwardMigrations(plans)
	for _, plan := range plans {
		if plan.portAdjusted {
			portAdjustedCount++
		}
	}
	if execErr != nil {
		failures = append(failures, execFailure)
		return tunnelDeleteWithForwardsResult{}, failures, nil
	}

	if err := h.deleteTunnelAndCleanup(tunnelID); err != nil {
		h.rollbackTunnelForwardMigrationPlans(plans)
		_ = h.redeployTunnelAndForwards(tunnelID)
		return tunnelDeleteWithForwardsResult{}, nil, err
	}

	result.MigratedCount = len(plans)
	result.PortAdjustedCount = portAdjustedCount
	if len(warnings) > 0 {
		result.Warnings = warnings
	}
	return result, nil, nil
}

func (h *Handler) planTunnelDeleteForwardMigrations(sourceTunnelID, targetTunnelID int64) ([]tunnelForwardMigrationPlan, []batchFailureDetail, error) {
	forwards, err := h.listForwardsByTunnel(sourceTunnelID)
	if err != nil {
		return nil, nil, err
	}

	entryNodes, err := h.tunnelEntryNodeIDs(targetTunnelID)
	if err != nil {
		return nil, nil, err
	}
	if len(entryNodes) == 0 {
		return nil, nil, errors.New("目标隧道缺少入口节点")
	}

	plans := make([]tunnelForwardMigrationPlan, 0, len(forwards))
	failures := make([]batchFailureDetail, 0)
	reservedPorts := make(map[int64]map[int]bool)

	for _, forward := range forwards {
		plan, planErr := h.planSingleTunnelDeleteForwardMigration(&forward, targetTunnelID, entryNodes, reservedPorts)
		if planErr != nil {
			failures = appendBatchFailure(failures, forward.ID, forward.Name, planErr)
			continue
		}
		plans = append(plans, plan)
	}

	return plans, failures, nil
}

func (h *Handler) planSingleTunnelDeleteForwardMigration(forward *forwardRecord, targetTunnelID int64, targetEntryNodes []int64, reservedPorts map[int64]map[int]bool) (tunnelForwardMigrationPlan, error) {
	if forward == nil {
		return tunnelForwardMigrationPlan{}, errors.New("转发不存在")
	}

	oldPorts, err := h.listForwardPorts(forward.ID)
	if err != nil {
		return tunnelForwardMigrationPlan{}, err
	}
	if len(oldPorts) == 0 {
		return tunnelForwardMigrationPlan{}, errors.New("转发入口端口不存在")
	}

	minPort := h.repo.GetMinForwardPort(forward.ID)
	targetPort := 0
	if minPort.Valid {
		targetPort = int(minPort.Int64)
	}
	if targetPort <= 0 {
		targetPort = h.pickTunnelPort(targetTunnelID)
	}
	if targetPort <= 0 {
		targetPort = 10000
	}

	hasCustomInIP := false
	for _, oldPort := range oldPorts {
		if strings.TrimSpace(oldPort.InIP) != "" {
			hasCustomInIP = true
			break
		}
	}
	if hasCustomInIP && len(targetEntryNodes) > 1 {
		return tunnelForwardMigrationPlan{}, errors.New("多入口隧道的转发不支持保留自定义监听IP，请先手动调整该规则")
	}

	for _, nodeID := range targetEntryNodes {
		node, nodeErr := h.getNodeRecord(nodeID)
		if nodeErr != nil {
			return tunnelForwardMigrationPlan{}, nodeErr
		}
		if err := validateRemoteNodePort(node, targetPort); err != nil {
			return tunnelForwardMigrationPlan{}, err
		}
		if err := validateLocalNodePort(node, targetPort); err != nil {
			return tunnelForwardMigrationPlan{}, err
		}
		if err := h.validateForwardPortAvailability(node, targetPort, forward.ID); err != nil {
			return tunnelForwardMigrationPlan{}, err
		}
		if reservedOnNode, ok := reservedPorts[nodeID]; ok && reservedOnNode[targetPort] {
			return tunnelForwardMigrationPlan{}, fmt.Errorf("目标隧道入口节点端口 %d 已被本次迁移中的其他规则占用", targetPort)
		}
	}

	for _, nodeID := range targetEntryNodes {
		reservedOnNode := reservedPorts[nodeID]
		if reservedOnNode == nil {
			reservedOnNode = make(map[int]bool)
			reservedPorts[nodeID] = reservedOnNode
		}
		reservedOnNode[targetPort] = true
	}

	oldNodeIDs := forwardPortNodeIDs(oldPorts)
	newNodeIDs := uniqueInt64s(targetEntryNodes)
	removedNodeIDs := diffInt64s(oldNodeIDs, newNodeIDs)
	keptNodeIDs := diffInt64s(oldNodeIDs, removedNodeIDs)

	previousPort := 0
	if len(oldPorts) > 0 {
		previousPort = oldPorts[0].Port
	}

	return tunnelForwardMigrationPlan{
		forward:        forward,
		oldPorts:       oldPorts,
		targetTunnelID: targetTunnelID,
		targetPort:     targetPort,
		keptNodeIDs:    keptNodeIDs,
		removedNodeIDs: removedNodeIDs,
		portAdjusted:   previousPort > 0 && previousPort != targetPort,
	}, nil
}

func (h *Handler) executeTunnelDeleteForwardMigrations(plans []tunnelForwardMigrationPlan) ([]string, error, batchFailureDetail) {
	warnings := make([]string, 0)
	completed := make([]tunnelForwardMigrationPlan, 0, len(plans))

	for _, plan := range plans {
		migrationWarnings, err := h.applyTunnelDeleteForwardMigration(plan)
		if err != nil {
			h.rollbackTunnelForwardMigrationPlans(completed)
			return warnings, err, batchFailureDetail{ID: plan.forward.ID, Name: plan.forward.Name, Reason: normalizeBatchFailureReason(errString(err))}
		}
		warnings = append(warnings, migrationWarnings...)
		completed = append(completed, plan)
	}

	return warnings, nil, batchFailureDetail{}
}

func (h *Handler) applyTunnelDeleteForwardMigration(plan tunnelForwardMigrationPlan) ([]string, error) {
	if plan.forward == nil {
		return nil, errors.New("转发不存在")
	}

	if err := h.repo.UpdateForwardTunnel(plan.forward.ID, plan.targetTunnelID, time.Now().UnixMilli()); err != nil {
		return nil, err
	}
	if err := h.replaceForwardPorts(plan.forward.ID, plan.targetTunnelID, plan.targetPort, ""); err != nil {
		h.rollbackForwardMutation(plan.forward, plan.oldPorts)
		return nil, err
	}

	updatedForward, err := h.getForwardRecord(plan.forward.ID)
	if err != nil {
		h.rollbackForwardMutation(plan.forward, plan.oldPorts)
		return nil, err
	}

	warnings := make([]string, 0)
	if len(plan.keptNodeIDs) > 0 {
		for _, nodeID := range plan.keptNodeIDs {
			if delErr := h.deleteForwardServicesOnNodeBatch(plan.forward, nodeID); delErr != nil {
				nodeLabel := fmt.Sprintf("%d", nodeID)
				if n, nErr := h.getNodeRecord(nodeID); nErr == nil && n != nil && strings.TrimSpace(n.Name) != "" {
					nodeLabel = strings.TrimSpace(n.Name)
				}
				warnings = append(warnings, fmt.Sprintf("节点 %s 清理旧转发监听失败: %v", nodeLabel, delErr))
			}
		}
		time.Sleep(tunnelServiceBindRetryDelay)
	}

	syncWarnings, err := h.syncForwardServicesWithWarnings(updatedForward, "UpdateService", true)
	if err != nil {
		h.rollbackForwardMutation(plan.forward, plan.oldPorts)
		return nil, err
	}
	warnings = append(warnings, syncWarnings...)

	if len(plan.removedNodeIDs) > 0 {
		for _, nodeID := range plan.removedNodeIDs {
			if delErr := h.deleteForwardServicesOnNodeBatch(plan.forward, nodeID); delErr != nil {
				nodeLabel := fmt.Sprintf("%d", nodeID)
				if n, nErr := h.getNodeRecord(nodeID); nErr == nil && n != nil && strings.TrimSpace(n.Name) != "" {
					nodeLabel = strings.TrimSpace(n.Name)
				}
				warnings = append(warnings, fmt.Sprintf("节点 %s 清理旧隧道残留服务失败: %v", nodeLabel, delErr))
			}
		}
	}

	return warnings, nil
}

func (h *Handler) rollbackTunnelForwardMigrationPlans(plans []tunnelForwardMigrationPlan) {
	for i := len(plans) - 1; i >= 0; i-- {
		plan := plans[i]
		h.rollbackForwardMutation(plan.forward, plan.oldPorts)
	}
}

func (h *Handler) deleteTunnelAndCleanup(tunnelID int64) error {
	h.cleanupTunnelRuntime(tunnelID)
	h.cleanupFederationRuntime(tunnelID)
	if err := h.deleteTunnelByID(tunnelID); err != nil {
		return err
	}
	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
