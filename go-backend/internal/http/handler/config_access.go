package handler

import (
	"net/http"
	"strings"

	"go-backend/internal/http/response"
	"go-backend/internal/store/repo"
)

func (h *Handler) getPublicConfigByName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req nameRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("配置名称不能为空"))
		return
	}

	configName := strings.ToLower(strings.TrimSpace(req.Name))
	if configName == "" {
		response.WriteJSON(w, response.ErrDefault("配置名称不能为空"))
		return
	}
	if !repo.IsPublicConfigKey(configName) {
		response.WriteJSON(w, response.Err(403, "禁止访问敏感配置"))
		return
	}
	if value, gated := h.unlicensedPublicBrandValue(configName); gated {
		response.WriteJSON(w, response.OK(map[string]string{"name": configName, "value": value}))
		return
	}

	cfg, err := h.repo.GetConfigByName(configName)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if cfg == nil {
		response.WriteJSON(w, response.ErrDefault("配置不存在"))
		return
	}

	response.WriteJSON(w, response.OK(cfg))
}

func (h *Handler) unlicensedPublicBrandValue(configName string) (string, bool) {
	switch configName {
	case "app_name", "app_logo", "app_favicon", "hide_footer_brand":
	default:
		return "", false
	}
	isCommercial, _ := h.repo.GetViteConfigValue("is_commercial")
	if isCommercial == "true" {
		return "", false
	}
	if configName == "app_name" {
		return "FLVX", true
	}
	if configName == "hide_footer_brand" {
		return "false", true
	}
	return "", true
}
