package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"go-backend/internal/http/response"
)

type diagnosisStreamEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data,omitempty"`
	TS   int64       `json:"ts"`
}

func prepareDiagnosisStreamResponse(w http.ResponseWriter) (http.Flusher, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("当前服务不支持流式响应")
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	return flusher, nil
}

func writeDiagnosisStreamEvent(encoder *json.Encoder, flusher http.Flusher, eventType string, data interface{}) error {
	if encoder == nil || flusher == nil {
		return errors.New("流式响应写入器未初始化")
	}
	event := diagnosisStreamEvent{Type: eventType, Data: data, TS: time.Now().UnixMilli()}
	if err := encoder.Encode(event); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func summarizeDiagnosisProgress(results []map[string]interface{}) diagnosisProgress {
	progress := diagnosisProgress{Total: len(results)}
	for _, item := range results {
		progress.Completed++
		if asBool(item["success"], false) {
			progress.Success++
		} else {
			progress.Failed++
		}
	}
	return progress
}

func shouldIgnoreDiagnosisStreamError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(msg, "broken pipe") || strings.Contains(msg, "connection reset by peer") {
		return true
	}
	if strings.Contains(msg, "stream already closed") {
		return true
	}
	return false
}

func (h *Handler) streamDiagnosisRuntime(ctx context.Context, cancel context.CancelFunc, w http.ResponseWriter, startPayload map[string]interface{}, workItems []diagnosisWorkItem) error {
	flusher, err := prepareDiagnosisStreamResponse(w)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(w)

	payload := map[string]interface{}{
		"total":     len(workItems),
		"timestamp": time.Now().UnixMilli(),
		"items":     h.buildDiagnosisStreamStartItems(workItems),
	}
	for key, value := range startPayload {
		payload[key] = value
	}
	if err := writeDiagnosisStreamEvent(encoder, flusher, "start", payload); err != nil {
		return err
	}

	streamBroken := false
	emitter := func(index int, item map[string]interface{}, progress diagnosisProgress) {
		if streamBroken {
			return
		}
		itemPayload := map[string]interface{}{
			"index":    index,
			"result":   item,
			"progress": progress,
		}
		if err := writeDiagnosisStreamEvent(encoder, flusher, "item", itemPayload); err != nil {
			streamBroken = true
			if cancel != nil {
				cancel()
			}
		}
	}

	results := h.runDiagnosisWorkItems(ctx, workItems, emitter)
	if streamBroken {
		return context.Canceled
	}

	progress := summarizeDiagnosisProgress(results)
	donePayload := map[string]interface{}{
		"progress": progress,
		"timedOut": errors.Is(ctx.Err(), context.DeadlineExceeded),
	}
	return writeDiagnosisStreamEvent(encoder, flusher, "done", donePayload)
}

func (h *Handler) tunnelDiagnoseStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	id := asInt64FromBodyKey(r, w, "tunnelId")
	if id <= 0 {
		return
	}

	tunnelName, tunnelType, workItems, err := h.prepareTunnelDiagnosis(id)
	if err != nil {
		if strings.Contains(err.Error(), "不存在") || strings.Contains(err.Error(), "不完整") {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), diagnosisRequestTimeout)
	defer cancel()

	startPayload := map[string]interface{}{
		"tunnelName": tunnelName,
		"tunnelType": tunnelType,
	}
	if err := h.streamDiagnosisRuntime(ctx, cancel, w, startPayload, workItems); err != nil {
		if shouldIgnoreDiagnosisStreamError(err) {
			return
		}
		if strings.Contains(err.Error(), "不支持流式响应") {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
		return
	}
}

func (h *Handler) forwardDiagnoseStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	id := asInt64FromBodyKey(r, w, "forwardId")
	if id <= 0 {
		return
	}

	forward, _, _, err := h.resolveForwardAccess(r, id)
	if err != nil {
		if errors.Is(err, errForwardNotFound) {
			response.WriteJSON(w, response.ErrDefault("转发不存在"))
			return
		}
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	forwardName, workItems, err := h.prepareForwardDiagnosis(forward)
	if err != nil {
		if strings.Contains(err.Error(), "不存在") || strings.Contains(err.Error(), "不能为空") || strings.Contains(err.Error(), "错误") {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), diagnosisRequestTimeout)
	defer cancel()

	startPayload := map[string]interface{}{
		"forwardName": forwardName,
	}
	if err := h.streamDiagnosisRuntime(ctx, cancel, w, startPayload, workItems); err != nil {
		if shouldIgnoreDiagnosisStreamError(err) {
			return
		}
		if strings.Contains(err.Error(), "不支持流式响应") {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
		return
	}
}
