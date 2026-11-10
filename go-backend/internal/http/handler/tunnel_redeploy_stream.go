package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"go-backend/internal/http/response"
)

type redeployProgress struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Success   int `json:"success"`
	Failed    int `json:"failed"`
}

type redeployItemResult struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Success bool   `json:"success"`
	Reason  string `json:"reason,omitempty"`
}

// tunnelBatchRedeployStream reports completed tunnels, not estimated runtime.
// A disconnected caller may have an in-flight mutation whose result it did not
// receive. Finish that mutation, but never start another or automatically retry.
func (h *Handler) tunnelBatchRedeployStream(w http.ResponseWriter, r *http.Request) {
	ids := idsFromBody(r, w)
	if ids == nil {
		return
	}
	// idsFromBody sorts IDs; ignore duplicate selections so each tunnel runs once.
	unique := ids[:0]
	for _, id := range ids {
		if len(unique) == 0 || unique[len(unique)-1] != id {
			unique = append(unique, id)
		}
	}
	ids = unique
	if len(ids) == 0 {
		response.WriteJSON(w, response.ErrDefault("ids不能为空"))
		return
	}
	_, err := prepareDiagnosisStreamResponse(w)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	encoder := json.NewEncoder(w)
	controller := http.NewResponseController(w)
	emit := func(kind string, data interface{}) bool {
		if r.Context().Err() != nil {
			return false
		}
		// The server's WriteTimeout is an absolute request deadline. Renew only
		// this stream's bounded write window as progress is emitted, so a long
		// batch is not cut off despite making steady progress. Recorders and
		// other non-network writers may not support deadlines.
		if err := controller.SetWriteDeadline(time.Now().Add(2 * time.Minute)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return false
		}
		event := diagnosisStreamEvent{Type: kind, Data: data, TS: time.Now().UnixMilli()}
		if err := encoder.Encode(event); err != nil {
			return false
		}
		// Buffered writes can succeed while the actual socket flush fails.
		// Do not start a mutation after its processing event failed to flush.
		return controller.Flush() == nil
	}
	progress := redeployProgress{Total: len(ids)}
	result := batchOperationResult{}
	if !emit("start", progress) {
		return
	}
	for _, id := range ids {
		if r.Context().Err() != nil {
			return
		}
		name, _ := h.repo.GetTunnelName(id)
		if !emit("processing", map[string]interface{}{"id": id, "name": name, "progress": progress}) {
			return
		}
		if r.Context().Err() != nil {
			return
		}
		item := redeployItemResult{ID: id, Name: name}
		if err := h.redeployTunnelConfiguration(id); err != nil {
			result.FailCount++
			result.Failures = appendBatchFailure(result.Failures, id, name, err)
			item.Reason = result.Failures[len(result.Failures)-1].Reason
			progress.Failed++
		} else {
			result.SuccessCount++
			item.Success = true
			progress.Success++
		}
		progress.Completed++
		if !emit("item", map[string]interface{}{"result": item, "progress": progress}) {
			return
		}
	}
	emit("done", map[string]interface{}{"progress": progress, "result": result})
}
