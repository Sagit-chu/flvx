package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-backend/internal/auth"
	"go-backend/internal/http/middleware"
	"go-backend/internal/http/response"
	"go-backend/internal/security"
	"go-backend/internal/store/sqlite"
	"go-backend/internal/ws"
)

type Handler struct {
	repo      *sqlite.Repository
	jwtSecret string
	wsServer  *ws.Server

	captchaMu     sync.Mutex
	captchaTokens map[string]int64

	jobsMu      sync.Mutex
	jobsCancel  context.CancelFunc
	jobsStarted bool
	jobsWG      sync.WaitGroup
}

type loginRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	CaptchaID string `json:"captchaId"`
}

type captchaVerifyRequest struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

type nameRequest struct {
	Name string `json:"name"`
}

type configSingleRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type changePasswordRequest struct {
	NewUsername     string `json:"newUsername"`
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
	ConfirmPassword string `json:"confirmPassword"`
}

type flowItem struct {
	N string `json:"n"`
	U int64  `json:"u"`
	D int64  `json:"d"`
}

func New(repo *sqlite.Repository, jwtSecret string) *Handler {
	return &Handler{
		repo:          repo,
		jwtSecret:     jwtSecret,
		wsServer:      ws.NewServer(repo, jwtSecret),
		captchaTokens: make(map[string]int64),
	}
}

func (h *Handler) WebSocketHandler() http.Handler {
	return h.wsServer
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/user/login", h.login)
	mux.HandleFunc("/api/v1/user/list", h.userList)
	mux.HandleFunc("/api/v1/user/create", h.userCreate)
	mux.HandleFunc("/api/v1/user/update", h.userUpdate)
	mux.HandleFunc("/api/v1/user/delete", h.userDelete)
	mux.HandleFunc("/api/v1/user/reset", h.userResetFlow)
	mux.HandleFunc("/api/v1/config/get", h.getConfigByName)
	mux.HandleFunc("/api/v1/config/list", h.getConfigs)
	mux.HandleFunc("/api/v1/config/update", h.updateConfigs)
	mux.HandleFunc("/api/v1/config/update-single", h.updateSingleConfig)
	mux.HandleFunc("/api/v1/backup/export", h.backupExport)
	mux.HandleFunc("/api/v1/backup/import", h.backupImport)
	mux.HandleFunc("/api/v1/backup/restore", h.backupImport)
	mux.HandleFunc("/api/v1/api/v1/backup/export", h.backupExport)
	mux.HandleFunc("/api/v1/api/v1/backup/import", h.backupImport)
	mux.HandleFunc("/api/v1/api/v1/backup/restore", h.backupImport)
	mux.HandleFunc("/api/v1/captcha/check", h.checkCaptcha)
	mux.HandleFunc("/api/v1/captcha/verify", h.captchaVerify)
	mux.HandleFunc("/api/v1/user/package", h.userPackage)
	mux.HandleFunc("/api/v1/user/updatePassword", h.updatePassword)
	mux.HandleFunc("/api/v1/node/list", h.nodeList)
	mux.HandleFunc("/api/v1/node/create", h.nodeCreate)
	mux.HandleFunc("/api/v1/node/update", h.nodeUpdate)
	mux.HandleFunc("/api/v1/node/delete", h.nodeDelete)
	mux.HandleFunc("/api/v1/node/install", h.nodeInstall)
	mux.HandleFunc("/api/v1/node/update-order", h.nodeUpdateOrder)
	mux.HandleFunc("/api/v1/node/batch-delete", h.nodeBatchDelete)
	mux.HandleFunc("/api/v1/node/check-status", h.nodeCheckStatus)
	mux.HandleFunc("/api/v1/node/upgrade", h.nodeUpgrade)
	mux.HandleFunc("/api/v1/node/batch-upgrade", h.nodeBatchUpgrade)
	mux.HandleFunc("/api/v1/node/rollback", h.nodeRollback)
	mux.HandleFunc("/api/v1/node/releases", h.listReleases)
	mux.HandleFunc("/api/v1/tunnel/list", h.tunnelList)
	mux.HandleFunc("/api/v1/tunnel/create", h.tunnelCreate)
	mux.HandleFunc("/api/v1/tunnel/get", h.tunnelGet)
	mux.HandleFunc("/api/v1/tunnel/update", h.tunnelUpdate)
	mux.HandleFunc("/api/v1/tunnel/delete", h.tunnelDelete)
	mux.HandleFunc("/api/v1/tunnel/diagnose", h.tunnelDiagnose)
	mux.HandleFunc("/api/v1/tunnel/update-order", h.tunnelUpdateOrder)
	mux.HandleFunc("/api/v1/tunnel/batch-delete", h.tunnelBatchDelete)
	mux.HandleFunc("/api/v1/tunnel/batch-redeploy", h.tunnelBatchRedeploy)
	mux.HandleFunc("/api/v1/tunnel/user/assign", h.userTunnelAssign)
	mux.HandleFunc("/api/v1/tunnel/user/batch-assign", h.userTunnelBatchAssign)
	mux.HandleFunc("/api/v1/tunnel/user/remove", h.userTunnelRemove)
	mux.HandleFunc("/api/v1/tunnel/user/update", h.userTunnelUpdate)
	mux.HandleFunc("/api/v1/forward/list", h.forwardList)
	mux.HandleFunc("/api/v1/forward/create", h.forwardCreate)
	mux.HandleFunc("/api/v1/forward/update", h.forwardUpdate)
	mux.HandleFunc("/api/v1/forward/delete", h.forwardDelete)
	mux.HandleFunc("/api/v1/forward/force-delete", h.forwardForceDelete)
	mux.HandleFunc("/api/v1/forward/pause", h.forwardPause)
	mux.HandleFunc("/api/v1/forward/resume", h.forwardResume)
	mux.HandleFunc("/api/v1/forward/diagnose", h.forwardDiagnose)
	mux.HandleFunc("/api/v1/forward/update-order", h.forwardUpdateOrder)
	mux.HandleFunc("/api/v1/forward/batch-delete", h.forwardBatchDelete)
	mux.HandleFunc("/api/v1/forward/batch-pause", h.forwardBatchPause)
	mux.HandleFunc("/api/v1/forward/batch-resume", h.forwardBatchResume)
	mux.HandleFunc("/api/v1/forward/batch-redeploy", h.forwardBatchRedeploy)
	mux.HandleFunc("/api/v1/forward/batch-change-tunnel", h.forwardBatchChangeTunnel)
	mux.HandleFunc("/api/v1/speed-limit/list", h.speedLimitList)
	mux.HandleFunc("/api/v1/speed-limit/create", h.speedLimitCreate)
	mux.HandleFunc("/api/v1/speed-limit/update", h.speedLimitUpdate)
	mux.HandleFunc("/api/v1/speed-limit/delete", h.speedLimitDelete)
	mux.HandleFunc("/api/v1/speed-limit/tunnels", h.tunnelList)
	mux.HandleFunc("/api/v1/tunnel/user/tunnel", h.userTunnelVisibleList)
	mux.HandleFunc("/api/v1/tunnel/user/list", h.userTunnelList)
	mux.HandleFunc("/api/v1/group/tunnel/list", h.tunnelGroupList)
	mux.HandleFunc("/api/v1/group/tunnel/create", h.groupTunnelCreate)
	mux.HandleFunc("/api/v1/group/tunnel/update", h.groupTunnelUpdate)
	mux.HandleFunc("/api/v1/group/tunnel/delete", h.groupTunnelDelete)
	mux.HandleFunc("/api/v1/group/tunnel/assign", h.groupTunnelAssign)
	mux.HandleFunc("/api/v1/group/user/list", h.userGroupList)
	mux.HandleFunc("/api/v1/group/user/create", h.groupUserCreate)
	mux.HandleFunc("/api/v1/group/user/update", h.groupUserUpdate)
	mux.HandleFunc("/api/v1/group/user/delete", h.groupUserDelete)
	mux.HandleFunc("/api/v1/group/user/assign", h.groupUserAssign)
	mux.HandleFunc("/api/v1/group/permission/list", h.groupPermissionList)
	mux.HandleFunc("/api/v1/group/permission/assign", h.groupPermissionAssign)
	mux.HandleFunc("/api/v1/group/permission/remove", h.groupPermissionRemove)
	mux.HandleFunc("/api/v1/open_api/sub_store", h.openAPISubStore)
	mux.HandleFunc("/api/v1/federation/share/list", h.federationShareList)
	mux.HandleFunc("/api/v1/federation/share/create", h.federationShareCreate)
	mux.HandleFunc("/api/v1/federation/share/update", h.federationShareUpdate)
	mux.HandleFunc("/api/v1/federation/share/delete", h.federationShareDelete)
	mux.HandleFunc("/api/v1/federation/share/reset-flow", h.federationShareResetFlow)
	mux.HandleFunc("/api/v1/federation/share/remote-usage/list", h.federationRemoteUsageList)
	mux.HandleFunc("/api/v1/federation/connect", h.authPeer(h.federationConnect))
	mux.HandleFunc("/api/v1/federation/tunnel/create", h.authPeer(h.federationTunnelCreate))
	mux.HandleFunc("/api/v1/federation/runtime/reserve-port", h.authPeer(h.federationRuntimeReservePort))
	mux.HandleFunc("/api/v1/federation/runtime/apply-role", h.authPeer(h.federationRuntimeApplyRole))
	mux.HandleFunc("/api/v1/federation/runtime/release-role", h.authPeer(h.federationRuntimeReleaseRole))
	mux.HandleFunc("/api/v1/federation/runtime/diagnose", h.authPeer(h.federationRuntimeDiagnose))
	mux.HandleFunc("/api/v1/federation/runtime/command", h.authPeer(h.federationRuntimeCommand))
	mux.HandleFunc("/api/v1/federation/node/import", h.nodeImport)
	mux.HandleFunc("/api/v1/announcement/get", h.getAnnouncement)
	mux.HandleFunc("/api/v1/announcement/update", h.updateAnnouncement)

	mux.HandleFunc("/flow/test", h.flowTest)
	mux.HandleFunc("/flow/config", h.flowConfig)
	mux.HandleFunc("/flow/upload", h.flowUpload)
	mux.HandleFunc("/error", h.errorPage)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req loginRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.Err(500, "请求参数错误"))
		return
	}

	if strings.TrimSpace(req.Username) == "" {
		response.WriteJSON(w, response.Err(500, "用户名不能为空"))
		return
	}
	if strings.TrimSpace(req.Password) == "" {
		response.WriteJSON(w, response.Err(500, "密码不能为空"))
		return
	}

	captchaEnabled, err := h.captchaEnabled()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if captchaEnabled {
		captchaID := strings.TrimSpace(req.CaptchaID)
		if captchaID == "" {
			response.WriteJSON(w, response.ErrDefault("验证码校验失败"))
			return
		}

		if !h.consumeCaptchaToken(captchaID) {
			secretCfg, err := h.repo.GetConfigByName("cloudflare_secret_key")
			if err != nil || secretCfg == nil || strings.TrimSpace(secretCfg.Value) == "" {
				response.WriteJSON(w, response.ErrDefault("验证码校验失败"))
				return
			}

			if !h.verifyCloudflareTurnstile(captchaID, strings.TrimSpace(secretCfg.Value)) {
				response.WriteJSON(w, response.ErrDefault("验证码校验失败"))
				return
			}
		}
	}

	user, err := h.repo.GetUserByUsername(req.Username)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if user == nil {
		response.WriteJSON(w, response.ErrDefault("账号或密码错误"))
		return
	}
	if user.Pwd != security.MD5(req.Password) {
		response.WriteJSON(w, response.ErrDefault("账号或密码错误"))
		return
	}
	if user.Status == 0 {
		response.WriteJSON(w, response.ErrDefault("账号被停用"))
		return
	}

	token, err := auth.GenerateToken(user.ID, user.User, user.RoleID, h.jwtSecret)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	requirePasswordChange := req.Username == "admin_user" || req.Password == "admin_user"
	response.WriteJSON(w, response.OK(map[string]interface{}{
		"token":                 token,
		"name":                  user.User,
		"role_id":               user.RoleID,
		"requirePasswordChange": requirePasswordChange,
	}))
}

func (h *Handler) getConfigByName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req nameRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("配置名称不能为空"))
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		response.WriteJSON(w, response.ErrDefault("配置名称不能为空"))
		return
	}

	cfg, err := h.repo.GetConfigByName(req.Name)
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

func (h *Handler) getConfigs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	cfgMap, err := h.repo.ListConfigs()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(cfgMap))
}

func (h *Handler) userList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req struct {
		Current int    `json:"current"`
		Size    int    `json:"size"`
		Keyword string `json:"keyword"`
	}
	if err := decodeJSON(r.Body, &req); err != nil && err != io.EOF {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}

	users, err := h.repo.ListUsers()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	keyword := strings.ToLower(strings.TrimSpace(req.Keyword))
	if keyword != "" {
		filtered := make([]map[string]interface{}, 0, len(users))
		for _, item := range users {
			username := strings.ToLower(strings.TrimSpace(fmt.Sprint(item["user"])))
			displayName := strings.ToLower(strings.TrimSpace(fmt.Sprint(item["name"])))
			if strings.Contains(username, keyword) || strings.Contains(displayName, keyword) {
				filtered = append(filtered, item)
			}
		}
		users = filtered
	}

	response.WriteJSON(w, response.OK(users))
}

func (h *Handler) nodeList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	items, err := h.repo.ListNodes()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	h.syncRemoteNodeStatuses(items)

	response.WriteJSON(w, response.OK(items))
}

func (h *Handler) tunnelList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	items, err := h.repo.ListTunnels()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(items))
}

func (h *Handler) forwardList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	userID, roleID, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}

	items, err := h.repo.ListForwards()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if roleID != 0 {
		filtered := make([]map[string]interface{}, 0, len(items))
		for _, item := range items {
			if asInt64(item["userId"], 0) == userID {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	response.WriteJSON(w, response.OK(items))
}

func (h *Handler) speedLimitList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	items, err := h.repo.ListSpeedLimits()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(items))
}

func (h *Handler) openAPISubStore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if h == nil || h.repo == nil || h.repo.DB() == nil {
		response.WriteJSON(w, response.Err(-2, "database unavailable"))
		return
	}

	username := strings.TrimSpace(r.URL.Query().Get("user"))
	password := strings.TrimSpace(r.URL.Query().Get("pwd"))
	tunnel := strings.TrimSpace(r.URL.Query().Get("tunnel"))
	if tunnel == "" {
		tunnel = "-1"
	}

	if username == "" {
		response.WriteJSON(w, response.ErrDefault("用户不能为空"))
		return
	}
	if password == "" {
		response.WriteJSON(w, response.ErrDefault("密码不能为空"))
		return
	}

	user, err := h.repo.GetUserByUsername(username)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if user == nil || user.Pwd != security.MD5(password) {
		response.WriteJSON(w, response.ErrDefault("鉴权失败"))
		return
	}

	const giga = int64(1024 * 1024 * 1024)
	headerValue := ""

	if tunnel == "-1" {
		headerValue = buildSubscriptionHeader(user.OutFlow, user.InFlow, user.Flow*giga, user.ExpTime/1000)
	} else {
		tunnelID, parseErr := strconv.ParseInt(tunnel, 10, 64)
		if parseErr != nil || tunnelID <= 0 {
			response.WriteJSON(w, response.ErrDefault("隧道不存在"))
			return
		}

		var userID int64
		var inFlow int64
		var outFlow int64
		var flow int64
		var expTime int64
		err = h.repo.DB().QueryRow(`SELECT user_id, in_flow, out_flow, flow, exp_time FROM user_tunnel WHERE id = ? LIMIT 1`, tunnelID).
			Scan(&userID, &inFlow, &outFlow, &flow, &expTime)
		if err != nil {
			if err == sql.ErrNoRows {
				response.WriteJSON(w, response.ErrDefault("隧道不存在"))
				return
			}
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
		if userID != user.ID {
			response.WriteJSON(w, response.ErrDefault("隧道不存在"))
			return
		}

		headerValue = buildSubscriptionHeader(outFlow, inFlow, flow*giga, expTime/1000)
	}

	w.Header().Set("subscription-userinfo", headerValue)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(headerValue))
}

func (h *Handler) errorPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("<!DOCTYPE html><html lang='zh-CN'><head><meta charset='UTF-8'><meta name='viewport' content='width=device-width, initial-scale=1.0'><title>错误 404</title></head><body><div style='min-height:100vh;display:flex;align-items:center;justify-content:center;flex-direction:column;font-family:-apple-system,BlinkMacSystemFont,Segoe UI,Arial,sans-serif;'><div style='font-size:6rem;color:#333;font-weight:300;'>404</div><div style='font-size:1.2rem;color:#666;'>你推开了后端的大门，却发现里面只有寂寞。</div></div></body></html>"))
}

func buildSubscriptionHeader(upload, download, total, expire int64) string {
	return fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d", download, upload, total, expire)
}

func (h *Handler) userTunnelVisibleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	userID, roleID, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}

	items := make([]map[string]interface{}, 0)
	if roleID == 0 {
		items, err = h.repo.ListEnabledTunnelSummaries()
	} else {
		items, err = h.repo.ListUserAccessibleTunnels(userID)
	}
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(items))
}

func (h *Handler) userTunnelList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req struct {
		UserID int64 `json:"userId"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if req.UserID <= 0 {
		response.WriteJSON(w, response.OK([]interface{}{}))
		return
	}

	tunnels, err := h.repo.GetUserPackageTunnels(req.UserID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	out := make([]map[string]interface{}, 0, len(tunnels))
	for _, t := range tunnels {
		item := map[string]interface{}{
			"id":             t.ID,
			"userId":         t.UserID,
			"tunnelId":       t.TunnelID,
			"tunnelName":     t.TunnelName,
			"status":         1,
			"flow":           t.Flow,
			"num":            t.Num,
			"expTime":        t.ExpTime,
			"flowResetTime":  t.FlowResetTime,
			"inFlow":         t.InFlow,
			"outFlow":        t.OutFlow,
			"tunnelFlow":     t.TunnelFlow,
			"speedId":        nil,
			"speedLimitName": nil,
		}
		if t.SpeedID.Valid {
			item["speedId"] = t.SpeedID.Int64
		}
		if t.SpeedLimit.Valid {
			item["speedLimitName"] = t.SpeedLimit.String
		}
		out = append(out, item)
	}
	response.WriteJSON(w, response.OK(out))
}

func (h *Handler) tunnelGroupList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	items, err := h.repo.ListTunnelGroups()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(items))
}

func (h *Handler) userGroupList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	items, err := h.repo.ListUserGroups()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(items))
}

func (h *Handler) groupPermissionList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	items, err := h.repo.ListGroupPermissions()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(items))
}

func (h *Handler) checkCaptcha(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	enabled, err := h.captchaEnabled()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if enabled {
		response.WriteJSON(w, response.OK(1))
		return
	}
	response.WriteJSON(w, response.OK(0))
}

func (h *Handler) captchaVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req captchaVerifyRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		h.writeCaptchaVerifyResult(w, false, "")
		return
	}
	id := strings.TrimSpace(req.ID)
	data := strings.TrimSpace(req.Data)
	if id == "" || data == "" {
		h.writeCaptchaVerifyResult(w, false, "")
		return
	}

	verified := false
	secretCfg, err := h.repo.GetConfigByName("cloudflare_secret_key")
	if err == nil && secretCfg != nil && strings.TrimSpace(secretCfg.Value) != "" {
		verified = h.verifyCloudflareTurnstile(data, strings.TrimSpace(secretCfg.Value))
	} else {
		verified = data == "ok"
	}
	if !verified {
		h.writeCaptchaVerifyResult(w, false, "")
		return
	}

	h.markCaptchaToken(id)
	h.writeCaptchaVerifyResult(w, true, id)
}

func (h *Handler) flowTest(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("test"))
}

func (h *Handler) flowConfig(w http.ResponseWriter, r *http.Request) {
	secret := r.URL.Query().Get("secret")
	node, err := h.repo.GetNodeBySecret(secret)
	if err != nil || node == nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
		return
	}

	rawData, err := readAndDecryptFlowBody(r.Body, secret)
	if err == nil && strings.TrimSpace(rawData) != "" {
		h.cleanNodeConfigs(node.ID, rawData)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

func (h *Handler) flowUpload(w http.ResponseWriter, r *http.Request) {
	secret := r.URL.Query().Get("secret")
	if ok, _ := h.repo.NodeExistsBySecret(secret); !ok {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
		return
	}

	raw, err := readAndDecryptFlowBody(r.Body, secret)
	if err == nil && strings.TrimSpace(raw) != "" {
		var items []flowItem
		if json.Unmarshal([]byte(raw), &items) == nil {
			for _, item := range items {
				h.processFlowItem(item)
			}
		}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

func (h *Handler) updateConfigs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var payload map[string]string
	if err := decodeJSON(r.Body, &payload); err != nil {
		response.WriteJSON(w, response.ErrDefault("配置数据不能为空"))
		return
	}
	if len(payload) == 0 {
		response.WriteJSON(w, response.ErrDefault("配置数据不能为空"))
		return
	}

	now := time.Now().UnixMilli()
	for k, v := range payload {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		if err := h.repo.UpsertConfig(key, v, now); err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
	}

	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) updateSingleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req configSingleRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("配置名称不能为空"))
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		response.WriteJSON(w, response.ErrDefault("配置名称不能为空"))
		return
	}
	if strings.TrimSpace(req.Value) == "" {
		response.WriteJSON(w, response.ErrDefault("配置值不能为空"))
		return
	}

	if err := h.repo.UpsertConfig(strings.TrimSpace(req.Name), req.Value, time.Now().UnixMilli()); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) userPackage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	claims, ok := r.Context().Value(middleware.ClaimsContextKey).(auth.Claims)
	if !ok {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}

	userID, err := parseUserID(claims.Sub)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}

	user, err := h.repo.GetUserByID(userID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if user == nil {
		response.WriteJSON(w, response.ErrDefault("用户不存在"))
		return
	}

	tunnels, err := h.repo.GetUserPackageTunnels(userID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	forwards, err := h.repo.GetUserPackageForwards(userID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	stats, err := h.repo.GetStatisticsFlows(userID, 24)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	sort.Slice(stats, func(i, j int) bool { return stats[i].ID < stats[j].ID })

	tunnelOut := make([]map[string]interface{}, 0, len(tunnels))
	for _, t := range tunnels {
		item := map[string]interface{}{
			"id":             t.ID,
			"userId":         t.UserID,
			"tunnelId":       t.TunnelID,
			"tunnelName":     t.TunnelName,
			"tunnelFlow":     t.TunnelFlow,
			"flow":           t.Flow,
			"inFlow":         t.InFlow,
			"outFlow":        t.OutFlow,
			"num":            t.Num,
			"flowResetTime":  t.FlowResetTime,
			"expTime":        t.ExpTime,
			"speedId":        nil,
			"speedLimitName": nil,
			"speed":          nil,
		}
		if t.SpeedID.Valid {
			item["speedId"] = t.SpeedID.Int64
		}
		if t.SpeedLimit.Valid {
			item["speedLimitName"] = t.SpeedLimit.String
		}
		if t.Speed.Valid {
			item["speed"] = t.Speed.Int64
		}
		tunnelOut = append(tunnelOut, item)
	}

	forwardOut := make([]map[string]interface{}, 0, len(forwards))
	for _, f := range forwards {
		item := map[string]interface{}{
			"id":          f.ID,
			"name":        f.Name,
			"tunnelId":    f.TunnelID,
			"tunnelName":  f.TunnelName,
			"inIp":        f.InIP,
			"inPort":      nil,
			"remoteAddr":  f.RemoteAddr,
			"inFlow":      f.InFlow,
			"outFlow":     f.OutFlow,
			"status":      f.Status,
			"createdTime": f.CreatedAt,
		}
		if f.InPort.Valid {
			item["inPort"] = f.InPort.Int64
		}
		forwardOut = append(forwardOut, item)
	}

	payload := map[string]interface{}{
		"userInfo": map[string]interface{}{
			"id":            user.ID,
			"name":          user.User,
			"user":          user.User,
			"status":        user.Status,
			"flow":          user.Flow,
			"inFlow":        user.InFlow,
			"outFlow":       user.OutFlow,
			"num":           user.Num,
			"expTime":       user.ExpTime,
			"flowResetTime": user.FlowResetTime,
			"createdTime":   user.CreatedTime,
			"updatedTime":   nullableNullInt64(user.UpdatedTime),
		},
		"tunnelPermissions": tunnelOut,
		"forwards":          forwardOut,
		"statisticsFlows":   stats,
	}

	response.WriteJSON(w, response.OK(payload))
}

func (h *Handler) updatePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	claims, ok := r.Context().Value(middleware.ClaimsContextKey).(auth.Claims)
	if !ok {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}

	userID, err := parseUserID(claims.Sub)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}

	var req changePasswordRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("修改账号密码时发生错误"))
		return
	}

	if strings.TrimSpace(req.NewUsername) == "" {
		response.WriteJSON(w, response.ErrDefault("新用户名不能为空"))
		return
	}
	if strings.TrimSpace(req.CurrentPassword) == "" {
		response.WriteJSON(w, response.ErrDefault("当前密码不能为空"))
		return
	}
	if strings.TrimSpace(req.NewPassword) == "" {
		response.WriteJSON(w, response.ErrDefault("新密码不能为空"))
		return
	}
	if strings.TrimSpace(req.ConfirmPassword) == "" {
		response.WriteJSON(w, response.ErrDefault("确认密码不能为空"))
		return
	}
	if req.NewPassword != req.ConfirmPassword {
		response.WriteJSON(w, response.ErrDefault("新密码和确认密码不匹配"))
		return
	}

	user, err := h.repo.GetUserByID(userID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if user == nil {
		response.WriteJSON(w, response.ErrDefault("用户不存在"))
		return
	}

	if user.Pwd != security.MD5(req.CurrentPassword) {
		response.WriteJSON(w, response.ErrDefault("当前密码错误"))
		return
	}

	exists, err := h.repo.UsernameExistsExceptID(req.NewUsername, userID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if exists {
		response.WriteJSON(w, response.ErrDefault("用户名已存在"))
		return
	}

	if err := h.repo.UpdateUserNameAndPassword(userID, req.NewUsername, security.MD5(req.NewPassword), time.Now().UnixMilli()); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) captchaEnabled() (bool, error) {
	cfg, err := h.repo.GetConfigByName("captcha_enabled")
	if err != nil {
		return false, err
	}
	if cfg == nil {
		return false, nil
	}
	return strings.EqualFold(cfg.Value, "true"), nil
}

func (h *Handler) markCaptchaToken(token string) {
	if h == nil {
		return
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	now := time.Now().UnixMilli()
	exp := now + int64(5*time.Minute/time.Millisecond)

	h.captchaMu.Lock()
	defer h.captchaMu.Unlock()
	if h.captchaTokens == nil {
		h.captchaTokens = make(map[string]int64)
	}
	for k, v := range h.captchaTokens {
		if v <= now {
			delete(h.captchaTokens, k)
		}
	}
	h.captchaTokens[token] = exp
}

func (h *Handler) consumeCaptchaToken(token string) bool {
	if h == nil {
		return false
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	now := time.Now().UnixMilli()

	h.captchaMu.Lock()
	defer h.captchaMu.Unlock()
	if h.captchaTokens == nil {
		return false
	}
	for k, v := range h.captchaTokens {
		if v <= now {
			delete(h.captchaTokens, k)
		}
	}
	exp, ok := h.captchaTokens[token]
	if !ok {
		return false
	}
	delete(h.captchaTokens, token)
	return exp > now
}

func (h *Handler) writeCaptchaVerifyResult(w http.ResponseWriter, success bool, token string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	payload := map[string]interface{}{
		"success": success,
		"data": map[string]interface{}{
			"validToken": token,
		},
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func decodeJSON(body io.ReadCloser, out interface{}) error {
	defer body.Close()
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func parseUserID(sub string) (int64, error) {
	id, err := strconv.ParseInt(sub, 10, 64)
	if err != nil || id <= 0 {
		return 0, strconv.ErrSyntax
	}
	return id, nil
}

func userIDFromRequest(r *http.Request) (int64, error) {
	claims, ok := r.Context().Value(middleware.ClaimsContextKey).(auth.Claims)
	if !ok {
		return 0, strconv.ErrSyntax
	}
	return parseUserID(claims.Sub)
}

func userRoleFromRequest(r *http.Request) (int64, int, error) {
	claims, ok := r.Context().Value(middleware.ClaimsContextKey).(auth.Claims)
	if !ok {
		return 0, 0, strconv.ErrSyntax
	}
	userID, err := parseUserID(claims.Sub)
	if err != nil {
		return 0, 0, err
	}
	return userID, claims.RoleID, nil
}

func nullableNullInt64(v sql.NullInt64) interface{} {
	if v.Valid {
		return v.Int64
	}
	return nil
}

func readAndDecryptFlowBody(body io.ReadCloser, secret string) (string, error) {
	defer body.Close()
	raw, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", nil
	}

	var wrap struct {
		Encrypted bool   `json:"encrypted"`
		Data      string `json:"data"`
		Timestamp int64  `json:"timestamp"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil || !wrap.Encrypted || strings.TrimSpace(wrap.Data) == "" {
		return text, nil
	}

	crypto, err := security.NewAESCrypto(secret)
	if err != nil {
		return text, nil
	}
	plain, err := crypto.Decrypt(wrap.Data)
	if err != nil {
		return text, nil
	}
	return string(plain), nil
}

func (h *Handler) verifyCloudflareTurnstile(token, secretKey string) bool {
	if token == "" || secretKey == "" {
		return false
	}
	resp, err := http.PostForm("https://challenges.cloudflare.com/turnstile/v0/siteverify", url.Values{
		"secret":   {secretKey},
		"response": {token},
	})
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var body struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false
	}
	return body.Success
}

type backupExportRequest struct {
	Types []string `json:"types"`
}

func (h *Handler) backupExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req backupExportRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.Err(500, "请求参数错误"))
		return
	}

	var backup interface{}
	var err error

	if len(req.Types) == 0 {
		backup, err = h.repo.ExportAll()
	} else {
		backup, err = h.repo.ExportPartial(req.Types)
	}

	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename=backup.json")
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(backup); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
}

type backupImportRequest struct {
	Types []string `json:"types"`
	sqlite.BackupData
}

func (h *Handler) backupImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req backupImportRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.Err(500, "请求参数错误"))
		return
	}

	if len(req.Types) == 0 {
		response.WriteJSON(w, response.Err(500, "请选择要导入的数据类型"))
		return
	}

	autoBackup, err := h.repo.ExportAll()
	if err != nil {
		response.WriteJSON(w, response.Err(-2, fmt.Sprintf("导入前自动备份失败: %v", err)))
		return
	}

	if req.BackupData.Version == "" {
		response.WriteJSON(w, response.Err(500, "备份数据格式错误"))
		return
	}

	result, err := h.repo.Import(&req.BackupData, req.Types)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, fmt.Sprintf("导入失败: %v", err)))
		return
	}

	result.AutoBackup = autoBackup
	response.WriteJSON(w, response.OK(result))
}

func (h *Handler) getAnnouncement(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	ann, err := h.repo.GetAnnouncement()
	if err != nil {
		response.WriteJSON(w, response.Err(-1, fmt.Sprintf("获取公告失败: %v", err)))
		return
	}

	if ann == nil {
		response.WriteJSON(w, response.OK(map[string]interface{}{
			"content": "",
			"enabled": 0,
		}))
		return
	}

	response.WriteJSON(w, response.OK(map[string]interface{}{
		"content": ann.Content,
		"enabled": ann.Enabled,
	}))
}

func (h *Handler) updateAnnouncement(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req struct {
		Content string `json:"content"`
		Enabled int    `json:"enabled"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.Err(500, "请求参数错误"))
		return
	}

	now := time.Now().UnixMilli()
	if err := h.repo.UpsertAnnouncement(req.Content, req.Enabled, now); err != nil {
		response.WriteJSON(w, response.Err(-1, fmt.Sprintf("更新公告失败: %v", err)))
		return
	}

	response.WriteJSON(w, response.OKEmpty())
}
