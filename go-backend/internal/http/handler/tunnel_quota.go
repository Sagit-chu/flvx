package handler

import (
	"errors"
	"net/http"
	"time"

	"go-backend/internal/http/response"
	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
)

func isTunnelQuotaExceeded(view *model.TunnelQuotaView) bool {
	if view == nil {
		return false
	}
	if view.DailyLimitGB > 0 && view.DailyUsedBytes >= view.DailyLimitGB*bytesPerGB {
		return true
	}
	if view.MonthlyLimitGB > 0 && view.MonthlyUsedBytes >= view.MonthlyLimitGB*bytesPerGB {
		return true
	}
	return false
}

func (h *Handler) tunnelQuotaBlockReason(tunnelID int64, now int64) (string, error) {
	if h == nil || h.repo == nil || tunnelID <= 0 {
		return "", nil
	}
	quota, err := h.repo.GetTunnelQuotaView(tunnelID, time.UnixMilli(now))
	if err != nil || quota == nil {
		return "", err
	}
	if quota.DisabledByQuota == 1 || isTunnelQuotaExceeded(quota) {
		return "该隧道流量配额已超额，禁止开启转发", nil
	}
	return "", nil
}

func (h *Handler) enforceTunnelQuotaIfNeeded(tunnelID int64, quota *model.TunnelQuotaView) {
	if h == nil || h.repo == nil || tunnelID <= 0 || quota == nil {
		return
	}
	if quota.DisabledByQuota == 1 || !isTunnelQuotaExceeded(quota) {
		return
	}

	forwards, err := h.listForwardsByTunnel(tunnelID)
	if err != nil {
		return
	}
	pausedIDs := make([]int64, 0, len(forwards))
	now := time.Now().UnixMilli()
	for i := range forwards {
		if forwards[i].Status != 1 {
			continue
		}
		if err := h.controlForwardServices(&forwards[i], "PauseService", false); err != nil {
			continue
		}
		if err := h.repo.UpdateForwardStatus(forwards[i].ID, 0, now); err != nil {
			continue
		}
		pausedIDs = append(pausedIDs, forwards[i].ID)
	}
	_ = h.repo.UpdateTunnelStatus(tunnelID, 0, now)
	_ = h.repo.MarkTunnelQuotaDisabled(tunnelID, pausedIDs, now)
}

func (h *Handler) applyTunnelQuotaRelease(release *repo.TunnelQuotaRelease, now int64) {
	if h == nil || h.repo == nil || release == nil || release.TunnelID <= 0 || !release.EnableTunnel {
		return
	}
	_ = h.repo.UpdateTunnelStatus(release.TunnelID, 1, now)
	for _, forwardID := range release.ForwardIDs {
		forward, err := h.getForwardRecord(forwardID)
		if err != nil || forward == nil {
			continue
		}
		if err := h.ensureUserTunnelForwardAllowed(forward.UserID, forward.TunnelID, now); err != nil {
			continue
		}
		if err := h.controlForwardServices(forward, "ResumeService", false); err != nil {
			continue
		}
		_ = h.repo.UpdateForwardStatus(forwardID, 1, now)
	}
}

func (h *Handler) resetTunnelQuotaWindows(now time.Time) {
	if h == nil || h.repo == nil {
		return
	}
	releases, err := h.repo.RollTunnelQuotaWindows(now)
	if err != nil {
		return
	}
	nowMs := now.UnixMilli()
	for i := range releases {
		h.applyTunnelQuotaRelease(&releases[i], nowMs)
	}
}

func (h *Handler) tunnelQuotaReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req struct {
		TunnelID int64  `json:"tunnelId"`
		Scope    string `json:"scope"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if req.TunnelID <= 0 {
		response.WriteJSON(w, response.ErrDefault("隧道ID不能为空"))
		return
	}
	release, err := h.repo.ResetTunnelQuotaUsage(req.TunnelID, req.Scope, time.Now())
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	nowMs := time.Now().UnixMilli()
	h.applyTunnelQuotaRelease(release, nowMs)
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) ensureTunnelForwardAllowedByQuota(tunnelID int64, now int64) error {
	reason, err := h.tunnelQuotaBlockReason(tunnelID, now)
	if err != nil {
		return err
	}
	if reason != "" {
		return errors.New(reason)
	}
	return nil
}
