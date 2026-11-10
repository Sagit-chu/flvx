package handler

import (
	"errors"
	"go-backend/internal/http/response"
	"go-backend/internal/store/repo"
	"net/http"
)

func (h *Handler) userDisplayOrder(w http.ResponseWriter, r *http.Request) {
	updateDisplayOrder(w, r, h.repo.UpdateUserDisplayOrder)
}
func (h *Handler) speedLimitDisplayOrder(w http.ResponseWriter, r *http.Request) {
	updateDisplayOrder(w, r, h.repo.UpdateSpeedLimitDisplayOrder)
}
func updateDisplayOrder(w http.ResponseWriter, r *http.Request, save func([]int64, []int64) error) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.Err(400, "请求方法错误"))
		return
	}
	var req struct {
		IDs         []int64 `json:"ids"`
		ExpectedIDs []int64 `json:"expectedIds"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.Err(400, "请求参数错误"))
		return
	}
	if err := save(req.IDs, req.ExpectedIDs); err != nil {
		code := 500
		if errors.Is(err, repo.ErrInvalidDisplayOrder) {
			code = 400
		}
		if errors.Is(err, repo.ErrDisplayOrderConflict) {
			code = 409
		}
		msg := "排序保存失败，请刷新后重试"
		if code != 500 {
			msg = err.Error()
		}
		response.WriteJSON(w, response.Err(code, msg))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}
