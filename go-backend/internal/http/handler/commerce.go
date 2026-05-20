package handler

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"go-backend/internal/auth"
	"go-backend/internal/http/response"
	"go-backend/internal/security"
	"go-backend/internal/store/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	orderStatusPending      = "pending"
	orderStatusPaid         = "paid"
	orderStatusProvisioning = "provisioning"
	orderStatusActive       = "active"
	orderStatusFailed       = "failed"
	orderStatusCancelled    = "cancelled"
	orderStatusRefunded     = "refunded"

	orderTypeNew            = "new"
	orderTypeUpgrade        = "upgrade"
	orderTypeRenew          = "renew"
	orderTypeResetFlow      = "reset_flow"
	orderTypeWalletRecharge = "wallet_recharge"

	paymentStatusUnpaid   = "unpaid"
	paymentStatusPaid     = "paid"
	paymentStatusRefunded = "refunded"

	fulfillmentStatusPending      = "pending"
	fulfillmentStatusProvisioning = "provisioning"
	fulfillmentStatusDone         = "done"
	fulfillmentStatusFailed       = "failed"
	fulfillmentStatusCancelled    = "cancelled"

	refundStatusNone     = "none"
	refundStatusPending  = "pending"
	refundStatusApproved = "approved"
	refundStatusRejected = "rejected"

	ticketStatusOpen   = "open"
	ticketStatusClosed = "closed"

	epayDefaultGateway   = "https://max.xinyuqicheng.cn/plugin/EpayApi/GatewayV1"
	epayDefaultSubmitURL = epayDefaultGateway + "/submit.php"

	paymentProviderEpay   = "epay"
	paymentProviderEpusdt = "epusdt"
)

type planPayload struct {
	ID                  int64               `json:"id"`
	Name                string              `json:"name"`
	Description         string              `json:"description"`
	Category            string              `json:"category"`
	PriceCents          int64               `json:"priceCents"`
	ResetFlowPriceCents int64               `json:"resetFlowPriceCents"`
	Currency            string              `json:"currency"`
	DurationDays        int                 `json:"durationDays"`
	Flow                int64               `json:"flow"`
	DailyQuotaGB        int64               `json:"dailyQuotaGB"`
	MonthlyQuotaGB      int64               `json:"monthlyQuotaGB"`
	Num                 int                 `json:"num"`
	MaxConn             int                 `json:"maxConn"`
	SpeedID             *int64              `json:"speedId"`
	Sort                int                 `json:"sort"`
	Status              int                 `json:"status"`
	TunnelIDs           []int64             `json:"tunnelIds"`
	TunnelGroupIDs      []int64             `json:"tunnelGroupIds"`
	TunnelNames         []string            `json:"tunnelNames"`
	TunnelGroupNames    []string            `json:"tunnelGroupNames"`
	Tunnels             []planTunnelPayload `json:"tunnels"`
}

type planTunnelPayload struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	TrafficRatio float64 `json:"trafficRatio"`
}

type userSubscriptionPayload struct {
	ID                  int64               `json:"id"`
	UserID              int64               `json:"userId"`
	PlanID              int64               `json:"planId"`
	OrderID             int64               `json:"orderId"`
	Status              string              `json:"status"`
	StartsAt            int64               `json:"startsAt"`
	ExpiresAt           int64               `json:"expiresAt"`
	Snapshot            string              `json:"snapshot"`
	CreatedTime         int64               `json:"createdTime"`
	UpdatedTime         int64               `json:"updatedTime"`
	PlanName            string              `json:"planName"`
	PlanCategory        string              `json:"planCategory"`
	PlanScopeKey        string              `json:"planScopeKey"`
	PlanPriceCents      int64               `json:"planPriceCents"`
	PlanFlow            int64               `json:"planFlow"`
	PlanNum             int                 `json:"planNum"`
	PlanMaxConn         int                 `json:"planMaxConn"`
	PlanTunnels         []planTunnelPayload `json:"planTunnels"`
	ResetFlowPriceCents int64               `json:"resetFlowPriceCents"`
	ResetFlowName       string              `json:"resetFlowName"`
}

type activeSubscriptionMatch struct {
	Sub      model.UserSubscription
	Plan     model.Plan
	SamePlan bool
}

type invitePayload struct {
	ID      int64  `json:"id"`
	Code    string `json:"code"`
	MaxUses int    `json:"maxUses"`
	ExpTime int64  `json:"expTime"`
	Status  int    `json:"status"`
}

func normalizeSupportAttachmentURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	if len(trimmed) > 2048 {
		return "", fmt.Errorf("附件链接过长")
	}
	parsed, err := url.ParseRequestURI(trimmed)
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("附件链接格式无效")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("附件链接仅支持 http 或 https")
	}
	host := parsed.Hostname()
	if host == "" {
		return "", fmt.Errorf("附件链接格式无效")
	}
	normalizedHost := strings.ToLower(strings.TrimSuffix(host, "."))
	if normalizedHost == "localhost" || normalizedHost == "localhost.localdomain" {
		return "", fmt.Errorf("附件链接不能指向本机地址")
	}
	if ip := net.ParseIP(normalizedHost); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return "", fmt.Errorf("附件链接不能指向内网地址")
		}
	}
	return trimmed, nil
}

type commerceListFilter struct {
	Keyword   string `json:"keyword"`
	OrderNo   string `json:"orderNo"`
	Status    string `json:"status"`
	OrderType string `json:"orderType"`
	Provider  string `json:"provider"`
	Action    string `json:"action"`
	UserID    int64  `json:"userId"`
	DateFrom  int64  `json:"dateFrom"`
	DateTo    int64  `json:"dateTo"`
	Page      int    `json:"page"`
	PageSize  int    `json:"pageSize"`
}

func commercePagination(req commerceListFilter) (int, int, int) {
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize, (page - 1) * pageSize
}

func commercePaginatedPayload(items interface{}, total int64, page, pageSize int) map[string]interface{} {
	return map[string]interface{}{
		"items":    items,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	}
}

func (h *Handler) publicCommerceSettings(w http.ResponseWriter, r *http.Request) {
	response.WriteJSON(w, response.OK(map[string]interface{}{
		"registrationEnabled": h.boolConfig("registration_enabled", false),
		"inviteRequired":      h.boolConfig("invite_registration_required", false),
		"captchaEnabled":      h.boolConfig("captcha_enabled", false),
		"epayEnabled":         h.boolConfig("epay_enabled", false),
		"usdtEnabled":         h.boolConfig("usdt_enabled", false),
		"resetFlowEnabled":    h.boolConfig("reset_flow_enabled", false),
		"resetFlowName":       h.configValue("reset_flow_name", "重置套餐流量"),
	}))
}

func (h *Handler) userRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.boolConfig("registration_enabled", false) {
		response.WriteJSON(w, response.ErrDefault("当前未开放用户注册"))
		return
	}

	var req struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		InviteCode string `json:"inviteCode"`
		CaptchaID  string `json:"captchaId"`
	}
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" || strings.TrimSpace(req.Password) == "" {
		response.WriteJSON(w, response.ErrDefault("用户名或密码不能为空"))
		return
	}
	if len(req.Password) < 6 {
		response.WriteJSON(w, response.ErrDefault("密码长度至少6位"))
		return
	}
	if !h.allowRateLimitedRequest("register:"+clientIPFromRequest(r)+":"+strings.ToLower(username), 8, 15*time.Minute) {
		response.WriteJSON(w, response.ErrDefault("请求过于频繁，请稍后再试"))
		return
	}
	if h.boolConfig("captcha_enabled", false) && !h.consumeCaptchaToken(strings.TrimSpace(req.CaptchaID)) {
		secretCfg, err := h.repo.GetConfigByName("cloudflare_secret_key")
		if err != nil || secretCfg == nil || strings.TrimSpace(secretCfg.Value) == "" ||
			!h.verifyCloudflareTurnstile(strings.TrimSpace(req.CaptchaID), strings.TrimSpace(secretCfg.Value)) {
			response.WriteJSON(w, response.ErrDefault("验证码校验失败"))
			return
		}
	}

	now := time.Now().UnixMilli()
	var invite *model.InviteCode
	if h.boolConfig("invite_registration_required", false) {
		code := strings.TrimSpace(req.InviteCode)
		if code == "" {
			response.WriteJSON(w, response.ErrDefault("请输入邀请码"))
			return
		}
		found, err := h.findUsableInvite(code, now)
		if err != nil {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
		invite = found
	}

	hashedPassword, err := security.HashPassword(req.Password)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	var userID int64
	err = h.repo.DB().Transaction(func(tx *gorm.DB) error {
		var existing int64
		if err := tx.Model(&model.User{}).Where(`"user" = ?`, username).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return fmt.Errorf("用户名已存在")
		}
		if invite != nil {
			var lockedInvite model.InviteCode
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", invite.ID).First(&lockedInvite).Error; err != nil {
				return err
			}
			if lockedInvite.Status != 1 {
				return fmt.Errorf("邀请码已禁用")
			}
			if lockedInvite.ExpTime > 0 && lockedInvite.ExpTime < now {
				return fmt.Errorf("邀请码已过期")
			}
			if lockedInvite.MaxUses > 0 && lockedInvite.UsedCount >= lockedInvite.MaxUses {
				return fmt.Errorf("邀请码已达到使用上限")
			}
		}
		user := model.User{
			User:              username,
			Pwd:               hashedPassword,
			RoleID:            1,
			ExpTime:           now,
			Flow:              0,
			InFlow:            0,
			OutFlow:           0,
			FlowResetTime:     1,
			Num:               0,
			MaxConn:           0,
			CreatedTime:       now,
			UpdatedTime:       sql.NullInt64{Int64: now, Valid: true},
			Status:            1,
			PasswordChangedAt: now,
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		userID = user.ID
		if invite != nil {
			result := tx.Model(&model.InviteCode{}).
				Where("id = ? AND (max_uses = 0 OR used_count < max_uses)", invite.ID).
				UpdateColumn("used_count", gorm.Expr("used_count + 1"))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return fmt.Errorf("邀请码已达到使用上限")
			}
			return tx.Create(&model.InviteCodeUsage{
				CodeID: invite.ID, UserID: userID, Username: username, CreatedTime: now,
			}).Error
		}
		return nil
	})
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}

	token, err := auth.GenerateTokenAt(userID, username, 1, h.jwtSecret, time.Now())
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{
		"token":                 token,
		"name":                  username,
		"role_id":               1,
		"requirePasswordChange": false,
	}))
}

func (h *Handler) listPublicPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.listPlans(true)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(plans))
}

func (h *Handler) adminListPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.listPlans(false)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(plans))
}

func (h *Handler) adminSavePlan(w http.ResponseWriter, r *http.Request) {
	actorID, _, _ := userRoleFromRequest(r)
	var req planPayload
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		response.WriteJSON(w, response.ErrDefault("套餐名称不能为空"))
		return
	}
	if req.DurationDays <= 0 {
		response.WriteJSON(w, response.ErrDefault("套餐周期必须大于0天"))
		return
	}
	if req.PriceCents < 0 {
		response.WriteJSON(w, response.ErrDefault("套餐售价不能小于0"))
		return
	}
	if req.ResetFlowPriceCents < 0 {
		response.WriteJSON(w, response.ErrDefault("重置流量价格不能小于0"))
		return
	}
	if req.Flow < 0 || req.DailyQuotaGB < 0 || req.MonthlyQuotaGB < 0 {
		response.WriteJSON(w, response.ErrDefault("套餐流量和配额不能小于0"))
		return
	}
	if req.Num < 0 || req.MaxConn < 0 {
		response.WriteJSON(w, response.ErrDefault("规则数量和最大连接数不能小于0"))
		return
	}
	resolvedTunnelIDs, err := h.resolveTunnelIDsFromScopes(req.TunnelIDs, req.TunnelGroupIDs)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if len(resolvedTunnelIDs) == 0 {
		response.WriteJSON(w, response.ErrDefault("套餐至少需要选择一个可开通隧道或隧道分组"))
		return
	}
	now := time.Now().UnixMilli()
	if req.Currency == "" {
		req.Currency = "CNY"
	}
	req.Category = strings.TrimSpace(req.Category)
	if req.Category == "" {
		req.Category = "默认"
	}
	if req.Status == 0 {
		req.Status = 1
	}
	creatingPlan := req.ID <= 0

	err = h.repo.DB().Transaction(func(tx *gorm.DB) error {
		plan := model.Plan{
			ID: req.ID, Name: strings.TrimSpace(req.Name), Description: strings.TrimSpace(req.Description), Category: req.Category,
			PriceCents: req.PriceCents, ResetFlowPriceCents: req.ResetFlowPriceCents, Currency: strings.ToUpper(req.Currency), DurationDays: req.DurationDays,
			Flow: req.Flow, DailyQuotaGB: req.DailyQuotaGB, MonthlyQuotaGB: req.MonthlyQuotaGB,
			Num: req.Num, MaxConn: req.MaxConn, Sort: req.Sort, Status: req.Status, UpdatedTime: now,
		}
		if req.SpeedID != nil && *req.SpeedID > 0 {
			plan.SpeedID = sql.NullInt64{Int64: *req.SpeedID, Valid: true}
		}
		if plan.ID > 0 {
			if err := tx.Model(&model.Plan{}).Where("id = ?", plan.ID).Updates(plan).Error; err != nil {
				return err
			}
		} else {
			plan.CreatedTime = now
			if err := tx.Create(&plan).Error; err != nil {
				return err
			}
			req.ID = plan.ID
		}
		if err := tx.Where("plan_id = ?", req.ID).Delete(&model.PlanEntitlement{}).Error; err != nil {
			return err
		}
		for _, id := range uniqueInt64(req.TunnelIDs) {
			if id > 0 {
				if err := tx.Create(&model.PlanEntitlement{PlanID: req.ID, ScopeType: "tunnel", ScopeID: id, CreatedTime: now}).Error; err != nil {
					return err
				}
			}
		}
		for _, id := range uniqueInt64(req.TunnelGroupIDs) {
			if id > 0 {
				if err := tx.Create(&model.PlanEntitlement{PlanID: req.ID, ScopeType: "tunnel_group", ScopeID: id, CreatedTime: now}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	action := "plan.create"
	if !creatingPlan {
		action = "plan.update"
	}
	h.writeAuditLog(actorID, "", action, "plan", req.ID, "保存套餐 "+strings.TrimSpace(req.Name), map[string]interface{}{
		"name": req.Name, "category": req.Category, "priceCents": req.PriceCents, "status": req.Status,
		"tunnelIds": uniqueInt64(req.TunnelIDs), "tunnelGroupIds": uniqueInt64(req.TunnelGroupIDs),
	})
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) adminDeletePlan(w http.ResponseWriter, r *http.Request) {
	actorID, _, _ := userRoleFromRequest(r)
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	now := time.Now().UnixMilli()
	if err := h.repo.DB().Model(&model.Plan{}).Where("id = ?", id).Updates(map[string]interface{}{"status": 0, "updated_time": now}).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	h.writeAuditLog(actorID, "", "plan.disable", "plan", id, fmt.Sprintf("下架套餐 #%d", id), nil)
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) createCommerceOrder(w http.ResponseWriter, r *http.Request) {
	userID, _, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	var req struct {
		PlanID     int64  `json:"planId"`
		Type       string `json:"type"`
		Action     string `json:"action"`
		CouponCode string `json:"couponCode"`
	}
	if err := decodeJSON(r.Body, &req); err != nil || req.PlanID <= 0 {
		response.WriteJSON(w, response.ErrDefault("请选择套餐"))
		return
	}
	var plan model.Plan
	if err := h.repo.DB().Where("id = ? AND status = 1", req.PlanID).First(&plan).Error; err != nil {
		response.WriteJSON(w, response.ErrDefault("套餐不存在或已下架"))
		return
	}
	var activeSub *model.UserSubscription
	var activePlan model.Plan
	orderType := orderTypeNew
	match, err := h.activeSubscriptionForPlanScope(userID, plan)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if match != nil {
		activeSub = &match.Sub
		activePlan = match.Plan
		if match.SamePlan {
			orderType = orderTypeRenew
		} else {
			if plan.PriceCents <= activePlan.PriceCents {
				response.WriteJSON(w, response.ErrDefault("当前线路已有同价或更高套餐，不能降级购买"))
				return
			}
			orderType = orderTypeUpgrade
		}
	}
	now := time.Now().UnixMilli()
	skipSubscriptionID := int64(0)
	if activeSub != nil {
		skipSubscriptionID = activeSub.ID
	}
	overlapNames, err := h.activeSubscriptionTunnelOverlap(userID, plan, skipSubscriptionID, now)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if len(overlapNames) > 0 {
		response.WriteJSON(w, response.ErrDefault("目标套餐与已开通套餐包含相同隧道："+strings.Join(overlapNames, "、")+"，请先处理原套餐或联系管理员调整套餐"))
		return
	}
	requestedAction := strings.ToLower(strings.TrimSpace(req.Action))
	if requestedAction != "" && requestedAction != orderTypeNew && requestedAction != orderTypeUpgrade && requestedAction != orderTypeRenew {
		response.WriteJSON(w, response.ErrDefault("订单类型无效"))
		return
	}
	amountCents := plan.PriceCents
	upgradeSnapshot := map[string]interface{}{}
	if orderType == orderTypeUpgrade && activeSub != nil {
		originalAmount := amountCents
		amountCents = h.proratedUpgradeAmount(plan, activePlan, *activeSub, now)
		upgradeSnapshot = map[string]interface{}{
			"fromPlanId": activePlan.ID, "fromPlanName": activePlan.Name,
			"fromPlanPriceCents": activePlan.PriceCents, "targetPlanPriceCents": plan.PriceCents,
			"originalAmountCents": originalAmount, "proratedAmountCents": amountCents,
			"activeSubscriptionId": activeSub.ID, "activeExpiresAt": activeSub.ExpiresAt,
		}
	}
	couponSnapshot := map[string]interface{}{}
	if strings.TrimSpace(req.CouponCode) != "" {
		discount, coupon, err := h.applyCoupon(strings.TrimSpace(req.CouponCode), plan, amountCents, userID, now)
		if err != nil {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
		amountCents -= discount
		if amountCents < 0 {
			amountCents = 0
		}
		couponSnapshot = map[string]interface{}{"couponId": coupon.ID, "couponCode": coupon.Code, "discountCents": discount}
	}
	planScopeKey, _ := h.planScopeKey(plan)
	snapshotBytes, _ := json.Marshal(map[string]interface{}{
		"plan": plan, "coupon": couponSnapshot, "upgrade": upgradeSnapshot,
		"planScopeKey": planScopeKey, "resolvedAction": orderType, "requestedAction": requestedAction,
	})
	if strings.TrimSpace(req.CouponCode) == "" {
		var existing model.CommerceOrder
		err := h.repo.DB().Where("user_id = ? AND plan_id = ? AND order_type = ? AND payment_status = ? AND status IN ?", userID, plan.ID, orderType, paymentStatusUnpaid, []string{orderStatusPending, orderStatusFailed}).
			Order("id DESC").First(&existing).Error
		if err == nil {
			if existing.AmountCents == 0 {
				response.WriteJSON(w, response.OK(map[string]interface{}{"order": h.commerceOrderPayload(existing), "payUrl": ""}))
				return
			}
			if strings.EqualFold(strings.TrimSpace(req.Type), "balance") {
				if err := h.payOrderWithBalance(&existing); err != nil {
					response.WriteJSON(w, response.ErrDefault(err.Error()))
					return
				}
				response.WriteJSON(w, response.OK(map[string]interface{}{"order": h.commerceOrderPayload(existing), "payUrl": ""}))
				return
			}
			provider := paymentProviderFromType(req.Type, existing.PaymentProvider)
			if err := h.prepareOrderPaymentProvider(&existing, provider); err != nil {
				response.WriteJSON(w, response.ErrDefault(err.Error()))
				return
			}
			payURL, payErr := h.buildPaymentURL(existing, plan, provider, req.Type)
			if payErr != nil {
				response.WriteJSON(w, response.Err(-2, payErr.Error()))
				return
			}
			response.WriteJSON(w, response.OK(map[string]interface{}{"order": h.commerceOrderPayload(existing), "payUrl": payURL}))
			return
		}
		if err != nil && err != gorm.ErrRecordNotFound {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
	}
	order := model.CommerceOrder{
		OrderNo: fmt.Sprintf("FLVX%d%d%s", time.Now().Unix(), userID, strings.ToUpper(randomHex(3))),
		UserID:  userID, PlanID: plan.ID, AmountCents: amountCents, Currency: plan.Currency,
		Status: orderStatusPending, PaymentStatus: paymentStatusUnpaid, FulfillmentStatus: fulfillmentStatusPending,
		RefundStatus: refundStatusNone, OrderType: orderType, PaymentProvider: paymentProviderFromType(req.Type, ""), Snapshot: string(snapshotBytes),
		CreatedTime: now, UpdatedTime: now,
	}
	if err := h.repo.DB().Create(&order).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := h.reserveCouponForOrder(order); err != nil {
		_ = h.repo.DB().Delete(&model.CommerceOrder{}, order.ID).Error
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	h.writeAuditLog(userID, "", "order.create", "commerce_order", order.ID, "创建订单 "+order.OrderNo, order)
	if order.AmountCents == 0 {
		if err := h.markOrderPaidAndProvision(&order, "coupon", "coupon-"+order.OrderNo); err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
		response.WriteJSON(w, response.OK(map[string]interface{}{"order": h.commerceOrderPayload(order), "payUrl": ""}))
		return
	}
	if strings.EqualFold(strings.TrimSpace(req.Type), "balance") {
		if err := h.payOrderWithBalance(&order); err != nil {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
		response.WriteJSON(w, response.OK(map[string]interface{}{"order": h.commerceOrderPayload(order), "payUrl": ""}))
		return
	}
	payURL, err := h.buildPaymentURL(order, plan, order.PaymentProvider, req.Type)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{"order": order, "payUrl": payURL}))
}

func (h *Handler) listMyOrders(w http.ResponseWriter, r *http.Request) {
	userID, _, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	var req commerceListFilter
	_ = decodeJSON(r.Body, &req)
	var orders []model.CommerceOrder
	query := h.repo.DB().Model(&model.CommerceOrder{}).Where("user_id = ?", userID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	page, pageSize, offset := commercePagination(req)
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&orders).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(commercePaginatedPayload(h.commerceOrderPayloads(orders), total, page, pageSize)))
}

func (h *Handler) payCommerceOrder(w http.ResponseWriter, r *http.Request) {
	userID, _, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	var req struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	}
	if err := decodeJSON(r.Body, &req); err != nil || req.ID <= 0 {
		response.WriteJSON(w, response.ErrDefault("参数错误"))
		return
	}
	var order model.CommerceOrder
	if err := h.repo.DB().Where("id = ? AND user_id = ?", req.ID, userID).First(&order).Error; err != nil {
		response.WriteJSON(w, response.ErrDefault("订单不存在"))
		return
	}
	if normalizedPaymentStatus(order) == paymentStatusPaid || (order.Status != orderStatusPending && order.Status != orderStatusFailed) {
		response.WriteJSON(w, response.ErrDefault("当前订单状态不能继续付款"))
		return
	}
	plan, err := h.planForOrder(order)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	provider := paymentProviderFromType(req.Type, order.PaymentProvider)
	if err := h.prepareOrderPaymentProvider(&order, provider); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	payURL, err := h.buildPaymentURL(order, plan, provider, req.Type)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{"payUrl": payURL, "order": h.commerceOrderPayload(order)}))
}

func (h *Handler) payCommerceOrderWithBalance(w http.ResponseWriter, r *http.Request) {
	userID, _, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	var order model.CommerceOrder
	if err := h.repo.DB().Where("id = ? AND user_id = ?", id, userID).First(&order).Error; err != nil {
		response.WriteJSON(w, response.ErrDefault("订单不存在"))
		return
	}
	if err := h.payOrderWithBalance(&order); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{"order": h.commerceOrderPayload(order), "payUrl": ""}))
}

func (h *Handler) cancelCommerceOrder(w http.ResponseWriter, r *http.Request) {
	userID, _, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	var order model.CommerceOrder
	if err := h.repo.DB().Where("id = ? AND user_id = ?", id, userID).First(&order).Error; err != nil {
		response.WriteJSON(w, response.ErrDefault("订单不存在"))
		return
	}
	if normalizedPaymentStatus(order) == paymentStatusPaid || (order.Status != orderStatusPending && order.Status != orderStatusFailed) {
		response.WriteJSON(w, response.ErrDefault("当前订单状态不能取消"))
		return
	}
	now := time.Now().UnixMilli()
	if err := h.repo.DB().Model(&model.CommerceOrder{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status": orderStatusCancelled, "fulfillment_status": fulfillmentStatusCancelled, "cancelled_time": now, "updated_time": now,
	}).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	h.releaseCouponForOrder(order)
	h.writeAuditLog(userID, "", "order.cancel", "commerce_order", id, "取消订单 "+order.OrderNo, nil)
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) mySubscription(w http.ResponseWriter, r *http.Request) {
	userID, _, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	var subs []model.UserSubscription
	err = h.repo.DB().Where("user_id = ? AND status = ? AND expires_at > ?", userID, "active", time.Now().UnixMilli()).Order("expires_at DESC, id DESC").Find(&subs).Error
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if len(subs) == 0 {
		response.WriteJSON(w, response.OK(nil))
		return
	}
	items := make([]userSubscriptionPayload, 0, len(subs))
	for _, sub := range subs {
		items = append(items, h.subscriptionPayload(sub))
	}
	primary := subscriptionPayloadMap(items[0], h.boolConfig("reset_flow_enabled", false))
	primary["subscriptions"] = items
	response.WriteJSON(w, response.OK(primary))
}

func (h *Handler) resetMySubscriptionFlow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	userID, _, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	if !h.boolConfig("reset_flow_enabled", false) {
		response.WriteJSON(w, response.ErrDefault("流量重置暂未开放"))
		return
	}

	var req struct {
		Type           string `json:"type"`
		SubscriptionID int64  `json:"subscriptionId"`
	}
	_ = decodeJSON(r.Body, &req)

	sub, plan, err := h.activeSubscriptionByIDOrLatest(userID, req.SubscriptionID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if sub == nil {
		response.WriteJSON(w, response.ErrDefault("当前没有可重置流量的有效套餐"))
		return
	}
	priceCents := plan.ResetFlowPriceCents
	if priceCents <= 0 {
		response.WriteJSON(w, response.ErrDefault("当前套餐未配置流量重置价格"))
		return
	}

	now := time.Now().UnixMilli()
	productName := fmt.Sprintf("%s - %s", h.configValue("reset_flow_name", "重置套餐流量"), plan.Name)
	snapshotBytes, _ := json.Marshal(map[string]interface{}{
		"type":                orderTypeResetFlow,
		"name":                productName,
		"planName":            plan.Name,
		"resetFlowPriceCents": priceCents,
		"subscriptionId":      sub.ID,
		"planId":              sub.PlanID,
		"expiresAt":           sub.ExpiresAt,
	})

	var order model.CommerceOrder
	err = h.repo.DB().
		Where("user_id = ? AND plan_id = ? AND order_type = ? AND status IN ?", userID, sub.PlanID, orderTypeResetFlow, []string{orderStatusPending, orderStatusFailed}).
		Order("id DESC").
		First(&order).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err == nil && (order.AmountCents != priceCents || subscriptionIDFromSnapshot(order.Snapshot) != sub.ID) {
		_ = h.repo.DB().Model(&model.CommerceOrder{}).Where("id = ?", order.ID).Updates(map[string]interface{}{
			"status": orderStatusCancelled, "fulfillment_status": fulfillmentStatusCancelled,
			"cancelled_time": now, "updated_time": now,
		}).Error
		order = model.CommerceOrder{}
	}
	if order.ID == 0 {
		order = model.CommerceOrder{
			OrderNo: fmt.Sprintf("FLVX%d%d%s", time.Now().Unix(), userID, strings.ToUpper(randomHex(3))),
			UserID:  userID, PlanID: sub.PlanID, AmountCents: priceCents, Currency: "CNY",
			Status: orderStatusPending, PaymentStatus: paymentStatusUnpaid, FulfillmentStatus: fulfillmentStatusPending,
			RefundStatus: refundStatusNone, OrderType: orderTypeResetFlow, PaymentProvider: paymentProviderFromType(req.Type, ""), Snapshot: string(snapshotBytes),
			CreatedTime: now, UpdatedTime: now,
		}
		if err := h.repo.DB().Create(&order).Error; err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
		h.writeAuditLog(userID, "", "order.create_reset_flow", "commerce_order", order.ID, "创建重置流量订单 "+order.OrderNo, order)
	}

	product := model.Plan{Name: productName, Currency: order.Currency}
	if strings.EqualFold(strings.TrimSpace(req.Type), "balance") {
		if err := h.payOrderWithBalance(&order); err != nil {
			response.WriteJSON(w, response.ErrDefault(err.Error()))
			return
		}
		response.WriteJSON(w, response.OK(map[string]interface{}{"order": h.commerceOrderPayload(order), "payUrl": ""}))
		return
	}
	provider := paymentProviderFromType(req.Type, order.PaymentProvider)
	if err := h.prepareOrderPaymentProvider(&order, provider); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	payURL, err := h.buildPaymentURL(order, product, provider, req.Type)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{"order": h.commerceOrderPayload(order), "payUrl": payURL}))
}

func (h *Handler) adminListOrders(w http.ResponseWriter, r *http.Request) {
	var req commerceListFilter
	_ = decodeJSON(r.Body, &req)
	var orders []model.CommerceOrder
	query := h.applyCommerceOrderFilter(h.repo.DB().Model(&model.CommerceOrder{}), req)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	page, pageSize, offset := commercePagination(req)
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&orders).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(commercePaginatedPayload(h.commerceOrderPayloads(orders), total, page, pageSize)))
}

func (h *Handler) adminConfirmOrder(w http.ResponseWriter, r *http.Request) {
	actorID, _, _ := userRoleFromRequest(r)
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	var order model.CommerceOrder
	if err := h.repo.DB().Where("id = ?", id).First(&order).Error; err != nil {
		response.WriteJSON(w, response.ErrDefault("订单不存在"))
		return
	}
	if err := h.markOrderPaidAndProvision(&order, "admin-manual", fmt.Sprintf("manual-%s-%d", order.OrderNo, time.Now().UnixMilli())); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	h.writeAuditLog(actorID, "", "order.manual_confirm", "commerce_order", order.ID, "管理员手动确认订单 "+order.OrderNo, nil)
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) adminListPayments(w http.ResponseWriter, r *http.Request) {
	var req commerceListFilter
	_ = decodeJSON(r.Body, &req)
	var rows []model.PaymentRecord
	query := h.repo.DB().Model(&model.PaymentRecord{})
	if strings.TrimSpace(req.OrderNo) != "" {
		query = query.Where("order_no LIKE ?", "%"+strings.TrimSpace(req.OrderNo)+"%")
	}
	if strings.TrimSpace(req.Provider) != "" {
		query = query.Where("provider = ?", strings.TrimSpace(req.Provider))
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	page, pageSize, offset := commercePagination(req)
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(commercePaginatedPayload(rows, total, page, pageSize)))
}

func (h *Handler) adminListAuditLogs(w http.ResponseWriter, r *http.Request) {
	var req commerceListFilter
	_ = decodeJSON(r.Body, &req)
	var rows []model.AuditLog
	query := h.repo.DB().Model(&model.AuditLog{})
	if strings.TrimSpace(req.Action) != "" {
		query = query.Where("action LIKE ?", "%"+strings.TrimSpace(req.Action)+"%")
	}
	if strings.TrimSpace(req.Keyword) != "" {
		kw := "%" + strings.TrimSpace(req.Keyword) + "%"
		query = query.Where("summary LIKE ? OR target_type LIKE ?", kw, kw)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	page, pageSize, offset := commercePagination(req)
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(commercePaginatedPayload(rows, total, page, pageSize)))
}

func (h *Handler) requestOrderRefund(w http.ResponseWriter, r *http.Request) {
	userID, _, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	var req struct {
		ID     int64  `json:"id"`
		Reason string `json:"reason"`
	}
	if err := decodeJSON(r.Body, &req); err != nil || req.ID <= 0 {
		response.WriteJSON(w, response.ErrDefault("请选择订单"))
		return
	}
	var order model.CommerceOrder
	if err := h.repo.DB().Where("id = ? AND user_id = ?", req.ID, userID).First(&order).Error; err != nil {
		response.WriteJSON(w, response.ErrDefault("订单不存在"))
		return
	}
	if normalizedPaymentStatus(order) != paymentStatusPaid || normalizedFulfillmentStatus(order) != fulfillmentStatusDone || normalizedRefundStatus(order) != refundStatusNone {
		response.WriteJSON(w, response.ErrDefault("当前订单不能申请退款"))
		return
	}
	now := time.Now().UnixMilli()
	refund := model.RefundRequest{
		OrderID: order.ID, OrderNo: order.OrderNo, UserID: userID, AmountCents: order.AmountCents,
		Reason: strings.TrimSpace(req.Reason), Status: refundStatusPending, CreatedTime: now, UpdatedTime: now,
	}
	if err := h.repo.DB().Create(&refund).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	_ = h.repo.DB().Model(&model.CommerceOrder{}).Where("id = ?", order.ID).Updates(map[string]interface{}{
		"refund_status": refundStatusPending, "refund_reason": refund.Reason, "updated_time": now,
	}).Error
	h.writeAuditLog(userID, "", "refund.request", "commerce_order", order.ID, "用户申请退款 "+order.OrderNo, refund)
	response.WriteJSON(w, response.OK(refund))
}

func (h *Handler) adminListRefunds(w http.ResponseWriter, r *http.Request) {
	var req commerceListFilter
	_ = decodeJSON(r.Body, &req)
	var rows []model.RefundRequest
	query := h.repo.DB().Model(&model.RefundRequest{})
	if strings.TrimSpace(req.Status) != "" {
		query = query.Where("status = ?", strings.TrimSpace(req.Status))
	}
	if strings.TrimSpace(req.OrderNo) != "" {
		query = query.Where("order_no LIKE ?", "%"+strings.TrimSpace(req.OrderNo)+"%")
	}
	if req.UserID > 0 {
		query = query.Where("user_id = ?", req.UserID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	page, pageSize, offset := commercePagination(req)
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(commercePaginatedPayload(rows, total, page, pageSize)))
}

func (h *Handler) adminHandleRefund(w http.ResponseWriter, r *http.Request) {
	actorID, _, _ := userRoleFromRequest(r)
	var req struct {
		ID        int64  `json:"id"`
		Decision  string `json:"decision"`
		AdminNote string `json:"adminNote"`
	}
	if err := decodeJSON(r.Body, &req); err != nil || req.ID <= 0 {
		response.WriteJSON(w, response.ErrDefault("请选择退款申请"))
		return
	}
	decision := strings.ToLower(strings.TrimSpace(req.Decision))
	if decision != refundStatusApproved && decision != refundStatusRejected {
		response.WriteJSON(w, response.ErrDefault("处理结果无效"))
		return
	}
	now := time.Now().UnixMilli()
	var refund model.RefundRequest
	err := h.repo.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", req.ID).First(&refund).Error; err != nil {
			return fmt.Errorf("退款申请不存在")
		}
		if refund.Status != refundStatusPending {
			return fmt.Errorf("当前退款申请已处理")
		}
		var order model.CommerceOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", refund.OrderID).First(&order).Error; err != nil {
			return fmt.Errorf("退款订单不存在")
		}
		if decision == refundStatusApproved {
			if err := h.revokeOrderResourcesTx(tx, order, now); err != nil {
				return err
			}
			if err := h.refundBalancePaymentTx(tx, refund, order, now); err != nil {
				return err
			}
		}
		if err := tx.Model(&model.RefundRequest{}).Where("id = ?", refund.ID).Updates(map[string]interface{}{
			"status": decision, "admin_note": strings.TrimSpace(req.AdminNote), "handled_time": now, "updated_time": now,
		}).Error; err != nil {
			return err
		}
		orderUpdates := map[string]interface{}{"refund_status": decision, "updated_time": now}
		if decision == refundStatusApproved {
			orderUpdates["status"] = orderStatusRefunded
			orderUpdates["payment_status"] = paymentStatusRefunded
			orderUpdates["refund_amount_cents"] = refund.AmountCents
			orderUpdates["refunded_time"] = now
		}
		return tx.Model(&model.CommerceOrder{}).Where("id = ?", refund.OrderID).Updates(orderUpdates).Error
	})
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	if decision == refundStatusApproved {
		if err := h.syncUserPackageResources(refund.UserID, now); err != nil {
			h.enqueueCommerceResourceJob("sync_user_resources", refund.UserID, refund.OrderID, "退款后资源同步失败："+err.Error(), map[string]interface{}{"refundId": refund.ID}, now)
			h.writeAuditLog(actorID, "", "resource.sync_queued", "user", refund.UserID, "退款后资源同步失败，已加入重试队列", map[string]interface{}{"error": err.Error(), "refundId": refund.ID})
		}
		h.createNotification(refund.UserID, "退款已通过", fmt.Sprintf("订单 %s 已标记退款，相关套餐资源已暂停。", refund.OrderNo), "warning")
	} else {
		h.createNotification(refund.UserID, "退款未通过", fmt.Sprintf("订单 %s 的退款申请未通过。", refund.OrderNo), "info")
	}
	h.writeAuditLog(actorID, "", "refund."+decision, "refund_request", refund.ID, "管理员处理退款 "+refund.OrderNo, map[string]interface{}{"decision": decision, "adminNote": req.AdminNote})
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) refundBalancePaymentTx(tx *gorm.DB, refund model.RefundRequest, order model.CommerceOrder, now int64) error {
	if order.PaymentProvider != "balance" {
		return nil
	}
	_, err := h.addWalletLedgerTx(tx, refund.UserID, refund.AmountCents, "refund_credit", "refund_request", refund.ID, "余额支付退款 "+refund.OrderNo, now, false)
	return err
}

func (h *Handler) revokeOrderResourcesTx(tx *gorm.DB, order model.CommerceOrder, now int64) error {
	if err := tx.Model(&model.UserSubscription{}).Where("order_id = ? AND status = ?", order.ID, "active").Updates(map[string]interface{}{"status": "refunded", "updated_time": now}).Error; err != nil {
		return err
	}
	if order.OrderType == orderTypeUpgrade {
		if oldSubID, ok := upgradeSourceSubscriptionID(order); ok {
			var oldSub model.UserSubscription
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", oldSubID, "replaced").First(&oldSub).Error
			if err == nil && oldSub.ExpiresAt > now {
				return tx.Model(&model.UserSubscription{}).Where("id = ?", oldSub.ID).Updates(map[string]interface{}{"status": "active", "updated_time": now}).Error
			}
			if err != nil && err != gorm.ErrRecordNotFound {
				return err
			}
		}
	}
	return nil
}

func upgradeSourceSubscriptionID(order model.CommerceOrder) (int64, bool) {
	if strings.TrimSpace(order.Snapshot) == "" {
		return 0, false
	}
	var snap struct {
		Upgrade struct {
			ActiveSubscriptionID int64 `json:"activeSubscriptionId"`
		} `json:"upgrade"`
	}
	if err := json.Unmarshal([]byte(order.Snapshot), &snap); err != nil || snap.Upgrade.ActiveSubscriptionID <= 0 {
		return 0, false
	}
	return snap.Upgrade.ActiveSubscriptionID, true
}

func (h *Handler) expireCommerceSubscriptions(now int64) {
	var subs []model.UserSubscription
	if err := h.repo.DB().Where("status = ? AND expires_at > 0 AND expires_at < ?", "active", now).Find(&subs).Error; err != nil {
		return
	}
	affectedUsers := map[int64]struct{}{}
	for _, sub := range subs {
		_ = h.repo.DB().Model(&model.UserSubscription{}).Where("id = ?", sub.ID).Updates(map[string]interface{}{"status": "expired", "updated_time": now}).Error
		affectedUsers[sub.UserID] = struct{}{}
		h.createNotification(sub.UserID, "套餐已到期", "您的套餐已到期，相关隧道资源已暂停。", "warning")
		h.writeAuditLog(sub.UserID, "", "subscription.expired", "user_subscription", sub.ID, "套餐到期并暂停资源", nil)
	}
	for userID := range affectedUsers {
		_ = h.syncUserPackageResources(userID, now)
	}
}

func (h *Handler) cancelExpiredPendingCommerceOrders(now time.Time) {
	if h == nil || h.repo == nil {
		return
	}
	timeoutMinutes := h.int64Config("pending_order_timeout_minutes", 30)
	if timeoutMinutes <= 0 {
		timeoutMinutes = 30
	}
	cutoff := now.Add(-time.Duration(timeoutMinutes) * time.Minute).UnixMilli()
	nowMs := now.UnixMilli()
	var orders []model.CommerceOrder
	if err := h.repo.DB().
		Where("payment_status = ? AND status IN ? AND created_time < ?", paymentStatusUnpaid, []string{orderStatusPending, orderStatusFailed}, cutoff).
		Order("id ASC").
		Limit(200).
		Find(&orders).Error; err != nil {
		return
	}
	for _, order := range orders {
		result := h.repo.DB().Model(&model.CommerceOrder{}).
			Where("id = ? AND payment_status = ? AND status IN ?", order.ID, paymentStatusUnpaid, []string{orderStatusPending, orderStatusFailed}).
			Updates(map[string]interface{}{
				"status": orderStatusCancelled, "fulfillment_status": fulfillmentStatusCancelled,
				"cancelled_time": nowMs, "updated_time": nowMs,
			})
		if result.Error == nil && result.RowsAffected > 0 {
			h.releaseCouponForOrder(order)
			h.writeAuditLog(0, "", "order.auto_cancel", "commerce_order", order.ID, "超时未支付自动取消 "+order.OrderNo, nil)
		}
	}
}

func (h *Handler) listMyNotifications(w http.ResponseWriter, r *http.Request) {
	userID, _, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	var req commerceListFilter
	_ = decodeJSON(r.Body, &req)
	var rows []model.Notification
	query := h.repo.DB().Model(&model.Notification{}).Where("user_id = ?", userID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	page, pageSize, offset := commercePagination(req)
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(commercePaginatedPayload(rows, total, page, pageSize)))
}

func (h *Handler) readMyNotification(w http.ResponseWriter, r *http.Request) {
	userID, _, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	now := time.Now().UnixMilli()
	if err := h.repo.DB().Model(&model.Notification{}).Where("id = ? AND user_id = ?", id, userID).Update("read_time", now).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) readAllMyNotifications(w http.ResponseWriter, r *http.Request) {
	userID, _, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	now := time.Now().UnixMilli()
	if err := h.repo.DB().Model(&model.Notification{}).
		Where("user_id = ? AND read_time = 0", userID).
		Update("read_time", now).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) walletBalance(userID int64) int64 {
	var wallet model.UserWallet
	if err := h.repo.DB().Where("user_id = ?", userID).First(&wallet).Error; err == nil {
		return wallet.BalanceCents
	}
	var balance int64
	_ = h.repo.DB().Model(&model.WalletLedger{}).Where("user_id = ?", userID).Select("COALESCE(SUM(amount_cents),0)").Scan(&balance).Error
	return balance
}

func (h *Handler) ensureUserWalletTx(tx *gorm.DB, userID int64, now int64) (model.UserWallet, error) {
	var wallet model.UserWallet
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", userID).First(&wallet).Error
	if err == nil {
		return wallet, nil
	}
	if err != gorm.ErrRecordNotFound {
		return wallet, err
	}
	var balance int64
	if err := tx.Model(&model.WalletLedger{}).Where("user_id = ?", userID).Select("COALESCE(SUM(amount_cents),0)").Scan(&balance).Error; err != nil {
		return wallet, err
	}
	wallet = model.UserWallet{UserID: userID, BalanceCents: balance, CreatedTime: now, UpdatedTime: now}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&wallet).Error; err != nil {
		return wallet, err
	}
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", userID).First(&wallet).Error
	return wallet, err
}

func (h *Handler) addWalletLedgerTx(tx *gorm.DB, userID, amountCents int64, typ, refType string, refID int64, note string, now int64, rejectNegative bool) (model.WalletLedger, error) {
	wallet, err := h.ensureUserWalletTx(tx, userID, now)
	if err != nil {
		return model.WalletLedger{}, err
	}
	nextBalance := wallet.BalanceCents + amountCents
	if rejectNegative && nextBalance < 0 {
		return model.WalletLedger{}, fmt.Errorf("余额不足")
	}
	if err := tx.Model(&model.UserWallet{}).Where("user_id = ?", userID).Updates(map[string]interface{}{
		"balance_cents": nextBalance, "updated_time": now,
	}).Error; err != nil {
		return model.WalletLedger{}, err
	}
	item := model.WalletLedger{
		UserID: userID, AmountCents: amountCents, BalanceAfterCents: nextBalance,
		Type: typ, RefType: refType, RefID: refID, Note: note, CreatedTime: now,
	}
	if err := tx.Create(&item).Error; err != nil {
		return model.WalletLedger{}, err
	}
	return item, nil
}

func (h *Handler) myWallet(w http.ResponseWriter, r *http.Request) {
	userID, _, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	var req commerceListFilter
	_ = decodeJSON(r.Body, &req)
	var rows []model.WalletLedger
	query := h.repo.DB().Model(&model.WalletLedger{}).Where("user_id = ?", userID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	page, pageSize, offset := commercePagination(req)
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	balance := h.walletBalance(userID)
	payload := commercePaginatedPayload(rows, total, page, pageSize)
	payload["balanceCents"] = balance
	response.WriteJSON(w, response.OK(payload))
}

func (h *Handler) rechargeMyWallet(w http.ResponseWriter, r *http.Request) {
	userID, _, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	var req struct {
		AmountCents int64  `json:"amountCents"`
		Type        string `json:"type"`
	}
	if err := decodeJSON(r.Body, &req); err != nil || req.AmountCents <= 0 {
		response.WriteJSON(w, response.ErrDefault("请输入充值金额"))
		return
	}
	if req.AmountCents < 100 {
		response.WriteJSON(w, response.ErrDefault("最低充值金额为1元"))
		return
	}
	now := time.Now().UnixMilli()
	snapshotBytes, _ := json.Marshal(map[string]interface{}{"name": "账户余额充值", "amountCents": req.AmountCents})
	order := model.CommerceOrder{
		OrderNo: fmt.Sprintf("FLVX%d%d%s", time.Now().Unix(), userID, strings.ToUpper(randomHex(3))),
		UserID:  userID, PlanID: 0, AmountCents: req.AmountCents, Currency: "CNY",
		Status: orderStatusPending, PaymentStatus: paymentStatusUnpaid, FulfillmentStatus: fulfillmentStatusPending,
		RefundStatus: refundStatusNone, OrderType: orderTypeWalletRecharge, PaymentProvider: paymentProviderFromType(req.Type, ""), Snapshot: string(snapshotBytes),
		CreatedTime: now, UpdatedTime: now,
	}
	if err := h.repo.DB().Create(&order).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	h.writeAuditLog(userID, "", "wallet.recharge_create", "commerce_order", order.ID, "创建余额充值订单 "+order.OrderNo, order)
	product := model.Plan{Name: "账户余额充值", Currency: "CNY"}
	payURL, err := h.buildPaymentURL(order, product, order.PaymentProvider, req.Type)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{"order": h.commerceOrderPayload(order), "payUrl": payURL}))
}

func (h *Handler) payOrderWithBalance(order *model.CommerceOrder) error {
	if order == nil || order.ID <= 0 {
		return fmt.Errorf("订单不存在")
	}
	if order.OrderType == orderTypeWalletRecharge {
		return fmt.Errorf("余额充值订单不能使用余额支付")
	}
	if order.AmountCents <= 0 {
		return h.markOrderPaidAndProvision(order, "balance", "balance-"+order.OrderNo)
	}
	now := time.Now().UnixMilli()
	var ledgerID int64
	err := h.repo.DB().Transaction(func(tx *gorm.DB) error {
		var current model.CommerceOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", order.ID).First(&current).Error; err != nil {
			return err
		}
		if current.UserID != order.UserID {
			return fmt.Errorf("订单用户不匹配")
		}
		if normalizedPaymentStatus(current) == paymentStatusPaid || (current.Status != orderStatusPending && current.Status != orderStatusFailed) {
			return fmt.Errorf("当前订单状态不能余额支付")
		}
		item, err := h.addWalletLedgerTx(tx, current.UserID, -current.AmountCents, "order_pay", "commerce_order", current.ID, "余额支付订单 "+current.OrderNo, now, true)
		if err != nil {
			return err
		}
		ledgerID = item.ID
		*order = current
		return nil
	})
	if err != nil {
		return err
	}
	tradeNo := fmt.Sprintf("wallet-%s-%d", order.OrderNo, ledgerID)
	if err := h.markOrderPaidAndProvision(order, "balance", tradeNo); err != nil {
		h.refundWalletDeduction(order, ledgerID, now, err.Error())
		return err
	}
	h.writeAuditLog(order.UserID, "", "wallet.order_pay", "commerce_order", order.ID, "余额支付订单 "+order.OrderNo, map[string]interface{}{"ledgerId": ledgerID})
	return nil
}

func (h *Handler) refundWalletDeduction(order *model.CommerceOrder, ledgerID int64, now int64, reason string) {
	if order == nil || order.ID <= 0 || ledgerID <= 0 {
		return
	}
	var item model.WalletLedger
	_ = h.repo.DB().Transaction(func(tx *gorm.DB) error {
		created, err := h.addWalletLedgerTx(tx, order.UserID, order.AmountCents, "order_pay_reversal", "wallet_ledger", ledgerID, "余额支付失败退回 "+order.OrderNo, now, false)
		item = created
		return err
	})
	h.writeAuditLog(order.UserID, "", "wallet.order_pay_reversal", "commerce_order", order.ID, reason, item)
	h.createNotification(order.UserID, "余额支付已退回", fmt.Sprintf("订单 %s 发放失败，已将余额支付金额退回。", order.OrderNo), "warning")
}

func (h *Handler) listMyTickets(w http.ResponseWriter, r *http.Request) {
	userID, _, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	var req commerceListFilter
	_ = decodeJSON(r.Body, &req)
	var rows []model.SupportTicket
	query := h.repo.DB().Model(&model.SupportTicket{}).Where("user_id = ?", userID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	page, pageSize, offset := commercePagination(req)
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(commercePaginatedPayload(rows, total, page, pageSize)))
}

func (h *Handler) myTicketMessages(w http.ResponseWriter, r *http.Request) {
	userID, _, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	var ticket model.SupportTicket
	if err := h.repo.DB().Where("id = ? AND user_id = ?", id, userID).First(&ticket).Error; err != nil {
		response.WriteJSON(w, response.ErrDefault("工单不存在"))
		return
	}
	var rows []model.SupportTicketMessage
	if err := h.repo.DB().Where("ticket_id = ?", id).Order("id ASC").Find(&rows).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{"ticket": ticket, "messages": rows}))
}

func (h *Handler) createMyTicket(w http.ResponseWriter, r *http.Request) {
	userID, _, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	var req struct {
		Title         string `json:"title"`
		Category      string `json:"category"`
		Content       string `json:"content"`
		AttachmentURL string `json:"attachmentUrl"`
	}
	if err := decodeJSON(r.Body, &req); err != nil || strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Content) == "" {
		response.WriteJSON(w, response.ErrDefault("请填写工单标题和内容"))
		return
	}
	attachmentURL, err := normalizeSupportAttachmentURL(req.AttachmentURL)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	now := time.Now().UnixMilli()
	var ticket model.SupportTicket
	err = h.repo.DB().Transaction(func(tx *gorm.DB) error {
		category := strings.TrimSpace(req.Category)
		if category == "" {
			category = "general"
		}
		ticket = model.SupportTicket{UserID: userID, Title: strings.TrimSpace(req.Title), Category: category, Status: ticketStatusOpen, Priority: "normal", CreatedTime: now, UpdatedTime: now}
		if err := tx.Create(&ticket).Error; err != nil {
			return err
		}
		return tx.Create(&model.SupportTicketMessage{TicketID: ticket.ID, UserID: userID, IsAdmin: 0, Content: strings.TrimSpace(req.Content), AttachmentURL: attachmentURL, CreatedTime: now}).Error
	})
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	h.writeAuditLog(userID, "", "ticket.create", "support_ticket", ticket.ID, "用户创建工单 "+ticket.Title, nil)
	response.WriteJSON(w, response.OK(ticket))
}

func (h *Handler) replyMyTicket(w http.ResponseWriter, r *http.Request) {
	userID, _, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	var req struct {
		ID            int64  `json:"id"`
		Content       string `json:"content"`
		AttachmentURL string `json:"attachmentUrl"`
	}
	if err := decodeJSON(r.Body, &req); err != nil || req.ID <= 0 || strings.TrimSpace(req.Content) == "" {
		response.WriteJSON(w, response.ErrDefault("请填写回复内容"))
		return
	}
	var ticket model.SupportTicket
	if err := h.repo.DB().Where("id = ? AND user_id = ?", req.ID, userID).First(&ticket).Error; err != nil {
		response.WriteJSON(w, response.ErrDefault("工单不存在"))
		return
	}
	if ticket.Status == ticketStatusClosed {
		response.WriteJSON(w, response.ErrDefault("工单已关闭"))
		return
	}
	attachmentURL, err := normalizeSupportAttachmentURL(req.AttachmentURL)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	now := time.Now().UnixMilli()
	if err := h.repo.DB().Create(&model.SupportTicketMessage{TicketID: ticket.ID, UserID: userID, IsAdmin: 0, Content: strings.TrimSpace(req.Content), AttachmentURL: attachmentURL, CreatedTime: now}).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	_ = h.repo.DB().Model(&model.SupportTicket{}).Where("id = ?", ticket.ID).Update("updated_time", now).Error
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) adminListTickets(w http.ResponseWriter, r *http.Request) {
	var req commerceListFilter
	_ = decodeJSON(r.Body, &req)
	var rows []model.SupportTicket
	query := h.repo.DB().Model(&model.SupportTicket{})
	if req.UserID > 0 {
		query = query.Where("user_id = ?", req.UserID)
	}
	if strings.TrimSpace(req.Status) != "" {
		query = query.Where("status = ?", strings.TrimSpace(req.Status))
	}
	if strings.TrimSpace(req.Keyword) != "" {
		kw := "%" + strings.TrimSpace(req.Keyword) + "%"
		query = query.Where("title LIKE ? OR category LIKE ? OR internal_note LIKE ?", kw, kw, kw)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	page, pageSize, offset := commercePagination(req)
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(commercePaginatedPayload(rows, total, page, pageSize)))
}

func (h *Handler) adminTicketMessages(w http.ResponseWriter, r *http.Request) {
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	var ticket model.SupportTicket
	if err := h.repo.DB().Where("id = ?", id).First(&ticket).Error; err != nil {
		response.WriteJSON(w, response.ErrDefault("工单不存在"))
		return
	}
	var rows []model.SupportTicketMessage
	if err := h.repo.DB().Where("ticket_id = ?", id).Order("id ASC").Find(&rows).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{"ticket": ticket, "messages": rows}))
}

func (h *Handler) adminUpdateTicket(w http.ResponseWriter, r *http.Request) {
	actorID, _, _ := userRoleFromRequest(r)
	var req struct {
		ID           int64  `json:"id"`
		Category     string `json:"category"`
		Priority     string `json:"priority"`
		InternalNote string `json:"internalNote"`
	}
	if err := decodeJSON(r.Body, &req); err != nil || req.ID <= 0 {
		response.WriteJSON(w, response.ErrDefault("请选择工单"))
		return
	}
	updates := map[string]interface{}{"updated_time": time.Now().UnixMilli()}
	if strings.TrimSpace(req.Category) != "" {
		updates["category"] = strings.TrimSpace(req.Category)
	}
	if strings.TrimSpace(req.Priority) != "" {
		updates["priority"] = strings.TrimSpace(req.Priority)
	}
	updates["internal_note"] = strings.TrimSpace(req.InternalNote)
	if err := h.repo.DB().Model(&model.SupportTicket{}).Where("id = ?", req.ID).Updates(updates).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	h.writeAuditLog(actorID, "", "ticket.update", "support_ticket", req.ID, fmt.Sprintf("更新工单 #%d", req.ID), map[string]interface{}{
		"category": strings.TrimSpace(req.Category), "priority": strings.TrimSpace(req.Priority),
		"hasInternalNote": strings.TrimSpace(req.InternalNote) != "",
	})
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) adminReplyTicket(w http.ResponseWriter, r *http.Request) {
	actorID, _, _ := userRoleFromRequest(r)
	var req struct {
		ID            int64  `json:"id"`
		Content       string `json:"content"`
		AttachmentURL string `json:"attachmentUrl"`
	}
	if err := decodeJSON(r.Body, &req); err != nil || req.ID <= 0 || strings.TrimSpace(req.Content) == "" {
		response.WriteJSON(w, response.ErrDefault("请填写回复内容"))
		return
	}
	var ticket model.SupportTicket
	if err := h.repo.DB().Where("id = ?", req.ID).First(&ticket).Error; err != nil {
		response.WriteJSON(w, response.ErrDefault("工单不存在"))
		return
	}
	attachmentURL, err := normalizeSupportAttachmentURL(req.AttachmentURL)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	now := time.Now().UnixMilli()
	if err := h.repo.DB().Create(&model.SupportTicketMessage{TicketID: ticket.ID, UserID: actorID, IsAdmin: 1, Content: strings.TrimSpace(req.Content), AttachmentURL: attachmentURL, CreatedTime: now}).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	_ = h.repo.DB().Model(&model.SupportTicket{}).Where("id = ?", ticket.ID).Update("updated_time", now).Error
	h.createNotification(ticket.UserID, "工单有新回复", fmt.Sprintf("工单「%s」有新的管理员回复。", ticket.Title), "info")
	h.writeAuditLog(actorID, "", "ticket.reply", "support_ticket", ticket.ID, "管理员回复工单 "+ticket.Title, map[string]interface{}{"hasAttachment": attachmentURL != ""})
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) adminCloseTicket(w http.ResponseWriter, r *http.Request) {
	actorID, _, _ := userRoleFromRequest(r)
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	now := time.Now().UnixMilli()
	if err := h.repo.DB().Model(&model.SupportTicket{}).Where("id = ?", id).Updates(map[string]interface{}{"status": ticketStatusClosed, "closed_time": now, "updated_time": now}).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	h.writeAuditLog(actorID, "", "ticket.close", "support_ticket", id, fmt.Sprintf("关闭工单 #%d", id), nil)
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) adminListCoupons(w http.ResponseWriter, r *http.Request) {
	var req commerceListFilter
	_ = decodeJSON(r.Body, &req)
	var rows []model.Coupon
	query := h.repo.DB().Model(&model.Coupon{})
	if strings.TrimSpace(req.Keyword) != "" {
		kw := "%" + strings.ToUpper(strings.TrimSpace(req.Keyword)) + "%"
		query = query.Where("UPPER(code) LIKE ? OR name LIKE ? OR category LIKE ?", kw, "%"+strings.TrimSpace(req.Keyword)+"%", "%"+strings.TrimSpace(req.Keyword)+"%")
	}
	if strings.TrimSpace(req.Status) != "" {
		switch strings.TrimSpace(req.Status) {
		case "enabled", "active", "1":
			query = query.Where("status = ?", 1)
		case "disabled", "inactive", "0":
			query = query.Where("status = ?", 0)
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	page, pageSize, offset := commercePagination(req)
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(commercePaginatedPayload(rows, total, page, pageSize)))
}

func (h *Handler) adminSaveCoupon(w http.ResponseWriter, r *http.Request) {
	actorID, _, _ := userRoleFromRequest(r)
	var req model.Coupon
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	if req.Code == "" {
		response.WriteJSON(w, response.ErrDefault("优惠码不能为空"))
		return
	}
	if req.DiscountType != "percent" {
		req.DiscountType = "fixed"
	}
	if req.DiscountValue <= 0 {
		response.WriteJSON(w, response.ErrDefault("优惠金额或折扣必须大于0"))
		return
	}
	if req.DiscountType == "percent" && req.DiscountValue > 100 {
		response.WriteJSON(w, response.ErrDefault("百分比优惠不能超过100%"))
		return
	}
	if req.DiscountType == "fixed" && req.DiscountValue < 0 {
		response.WriteJSON(w, response.ErrDefault("固定减免金额不能小于0"))
		return
	}
	if req.PlanID < 0 || req.MinAmountCents < 0 || req.PerUserLimit < 0 || req.MaxUses < 0 || req.ExpTime < 0 {
		response.WriteJSON(w, response.ErrDefault("优惠码限制参数不能小于0"))
		return
	}
	if req.Status != 1 {
		req.Status = 0
	}
	creatingCoupon := req.ID <= 0
	now := time.Now().UnixMilli()
	req.UpdatedTime = now
	if req.ID > 0 {
		if err := h.repo.DB().Model(&model.Coupon{}).Where("id = ?", req.ID).Updates(map[string]interface{}{
			"code": req.Code, "name": strings.TrimSpace(req.Name), "discount_type": req.DiscountType, "discount_value": req.DiscountValue,
			"plan_id": req.PlanID, "category": strings.TrimSpace(req.Category), "min_amount_cents": req.MinAmountCents,
			"per_user_limit": req.PerUserLimit, "max_uses": req.MaxUses, "exp_time": req.ExpTime, "status": req.Status,
			"updated_time": now,
		}).Error; err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
	} else {
		req.CreatedTime = now
		if err := h.repo.DB().Create(&req).Error; err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
	}
	action := "coupon.create"
	if !creatingCoupon {
		action = "coupon.update"
	}
	h.writeAuditLog(actorID, "", action, "coupon", req.ID, "保存优惠码 "+req.Code, map[string]interface{}{
		"code": req.Code, "name": strings.TrimSpace(req.Name), "discountType": req.DiscountType,
		"planId": req.PlanID, "category": strings.TrimSpace(req.Category), "status": req.Status,
	})
	response.WriteJSON(w, response.OK(req))
}

func (h *Handler) adminDeleteCoupon(w http.ResponseWriter, r *http.Request) {
	actorID, _, _ := userRoleFromRequest(r)
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	if err := h.repo.DB().Model(&model.Coupon{}).Where("id = ?", id).Update("status", 0).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	h.writeAuditLog(actorID, "", "coupon.disable", "coupon", id, fmt.Sprintf("停用优惠码 #%d", id), nil)
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) adminListWalletLedger(w http.ResponseWriter, r *http.Request) {
	var req commerceListFilter
	_ = decodeJSON(r.Body, &req)
	var rows []model.WalletLedger
	query := h.repo.DB().Model(&model.WalletLedger{})
	if req.UserID > 0 {
		query = query.Where("user_id = ?", req.UserID)
	}
	if strings.TrimSpace(req.Keyword) != "" {
		query = query.Where("note LIKE ?", "%"+strings.TrimSpace(req.Keyword)+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	page, pageSize, offset := commercePagination(req)
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(commercePaginatedPayload(rows, total, page, pageSize)))
}

func (h *Handler) adminAdjustWallet(w http.ResponseWriter, r *http.Request) {
	actorID, _, _ := userRoleFromRequest(r)
	var req struct {
		UserID      int64  `json:"userId"`
		AmountCents int64  `json:"amountCents"`
		Note        string `json:"note"`
	}
	if err := decodeJSON(r.Body, &req); err != nil || req.UserID <= 0 || req.AmountCents == 0 {
		response.WriteJSON(w, response.ErrDefault("请填写用户和调账金额"))
		return
	}
	now := time.Now().UnixMilli()
	var item model.WalletLedger
	err := h.repo.DB().Transaction(func(tx *gorm.DB) error {
		created, err := h.addWalletLedgerTx(tx, req.UserID, req.AmountCents, "manual_adjust", "admin", actorID, strings.TrimSpace(req.Note), now, false)
		item = created
		return err
	})
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	h.writeAuditLog(actorID, "", "wallet.adjust", "wallet_ledger", item.ID, "管理员余额调账", item)
	h.createNotification(req.UserID, "余额变动", fmt.Sprintf("账户余额变动 %s，当前余额 %s。", formatCentsForNotice(req.AmountCents), formatCentsForNotice(item.BalanceAfterCents)), "info")
	response.WriteJSON(w, response.OK(item))
}

func (h *Handler) adminCommerceReportSummary(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UnixMilli()
	dayAgo := now - int64((24*time.Hour)/time.Millisecond)
	monthAgo := now - int64((30*24*time.Hour)/time.Millisecond)
	var paidTotal, paidMonth, refundTotal, walletBalance int64
	_ = h.repo.DB().Model(&model.CommerceOrder{}).
		Where("payment_status = ? AND status != ?", paymentStatusPaid, orderStatusRefunded).
		Select("COALESCE(SUM(amount_cents),0)").Scan(&paidTotal).Error
	_ = h.repo.DB().Model(&model.CommerceOrder{}).
		Where("payment_status = ? AND status != ? AND paid_time >= ?", paymentStatusPaid, orderStatusRefunded, monthAgo).
		Select("COALESCE(SUM(amount_cents),0)").Scan(&paidMonth).Error
	_ = h.repo.DB().Model(&model.CommerceOrder{}).
		Where("refund_status = ?", refundStatusApproved).
		Select("COALESCE(SUM(refund_amount_cents),0)").Scan(&refundTotal).Error
	_ = h.repo.DB().Model(&model.WalletLedger{}).Select("COALESCE(SUM(amount_cents),0)").Scan(&walletBalance).Error
	countBy := func(modelRef interface{}, where string, args ...interface{}) int64 {
		var count int64
		h.repo.DB().Model(modelRef).Where(where, args...).Count(&count)
		return count
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{
		"paidTotalCents": paidTotal, "paidMonthCents": paidMonth, "refundTotalCents": refundTotal,
		"walletBalanceCents":  walletBalance,
		"ordersToday":         countBy(&model.CommerceOrder{}, "created_time >= ?", dayAgo),
		"pendingOrders":       countBy(&model.CommerceOrder{}, "status = ?", orderStatusPending),
		"activeSubscriptions": countBy(&model.UserSubscription{}, "status = ? AND expires_at > ?", "active", now),
		"openTickets":         countBy(&model.SupportTicket{}, "status = ?", ticketStatusOpen),
		"pendingRefunds":      countBy(&model.RefundRequest{}, "status = ?", refundStatusPending),
	}))
}

func (h *Handler) adminCommerceRiskList(w http.ResponseWriter, r *http.Request) {
	var req commerceListFilter
	_ = decodeJSON(r.Body, &req)
	now := time.Now().UnixMilli()
	dayAgo := now - int64((24*time.Hour)/time.Millisecond)
	type riskRow struct {
		UserID  int64  `json:"userId"`
		Type    string `json:"type"`
		Level   string `json:"level"`
		Summary string `json:"summary"`
		Count   int64  `json:"count"`
	}
	rows := []riskRow{}
	var failed []struct {
		UserID int64
		Count  int64
	}
	_ = h.repo.DB().Model(&model.CommerceOrder{}).
		Select("user_id, COUNT(1) as count").
		Where("created_time >= ? AND status IN ?", dayAgo, []string{orderStatusFailed, orderStatusCancelled}).
		Group("user_id").Having("COUNT(1) >= ?", 3).Scan(&failed).Error
	for _, item := range failed {
		rows = append(rows, riskRow{UserID: item.UserID, Type: "order_abnormal", Level: "medium", Summary: "24小时内失败或取消订单过多", Count: item.Count})
	}
	var refunds []struct {
		UserID int64
		Count  int64
	}
	_ = h.repo.DB().Model(&model.RefundRequest{}).
		Select("user_id, COUNT(1) as count").
		Where("created_time >= ?", now-int64((30*24*time.Hour)/time.Millisecond)).
		Group("user_id").Having("COUNT(1) >= ?", 3).Scan(&refunds).Error
	for _, item := range refunds {
		rows = append(rows, riskRow{UserID: item.UserID, Type: "refund_abnormal", Level: "high", Summary: "30天内退款申请过多", Count: item.Count})
	}
	var negativeWallet []struct {
		UserID  int64
		Balance int64
	}
	_ = h.repo.DB().Model(&model.WalletLedger{}).
		Select("user_id, COALESCE(SUM(amount_cents),0) as balance").
		Group("user_id").Having("COALESCE(SUM(amount_cents),0) < 0").Scan(&negativeWallet).Error
	for _, item := range negativeWallet {
		rows = append(rows, riskRow{UserID: item.UserID, Type: "wallet_negative", Level: "high", Summary: "账户余额为负", Count: item.Balance})
	}
	if req.UserID > 0 || strings.TrimSpace(req.Keyword) != "" || strings.TrimSpace(req.Status) != "" {
		filtered := make([]riskRow, 0, len(rows))
		keyword := strings.TrimSpace(req.Keyword)
		level := strings.TrimSpace(req.Status)
		for _, row := range rows {
			if req.UserID > 0 && row.UserID != req.UserID {
				continue
			}
			if level != "" && !strings.EqualFold(row.Level, level) {
				continue
			}
			if keyword != "" && !strings.Contains(row.Type, keyword) && !strings.Contains(row.Summary, keyword) {
				continue
			}
			filtered = append(filtered, row)
		}
		rows = filtered
	}
	total := int64(len(rows))
	page, pageSize, offset := commercePagination(req)
	if offset >= len(rows) {
		rows = []riskRow{}
	} else {
		end := offset + pageSize
		if end > len(rows) {
			end = len(rows)
		}
		rows = rows[offset:end]
	}
	response.WriteJSON(w, response.OK(commercePaginatedPayload(rows, total, page, pageSize)))
}

func formatCentsForNotice(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s¥%.2f", sign, float64(cents)/100)
}

func (h *Handler) publicLegalPages(w http.ResponseWriter, r *http.Request) {
	response.WriteJSON(w, response.OK(map[string]string{
		"terms": h.configValue("legal_terms", strings.Join([]string{
			"使用本平台服务前，请确认你已了解并同意本服务条款。",
			"平台提供网络转发与相关资源管理服务，用户应自行保证业务内容、访问目标和使用方式符合当地法律法规及平台规则。",
			"平台有权对异常流量、攻击行为、滥用行为、影响服务稳定的连接或违反规则的账号采取限速、暂停、终止服务等处理。",
			"因用户自身配置错误、违规使用、第三方网络波动或不可抗力造成的损失，平台将在合理范围内协助排查，但不承担超出已支付服务费用范围的责任。",
		}, "\n\n")),
		"privacy": h.configValue("legal_privacy", strings.Join([]string{
			"平台仅收集账号、订单、支付状态、资源开通、流量统计、登录安全和审计排障所需的数据。",
			"这些数据用于完成服务交付、账务核对、风险控制、客服支持和系统安全维护，不会用于与服务无关的用途。",
			"除法律法规要求、支付核验、基础设施运维或获得用户授权外，平台不会向无关第三方出售或共享用户数据。",
			"用户应妥善保管账号密码和访问凭据，如发现异常登录、资源异常或疑似泄露，应及时修改密码并联系平台处理。",
		}, "\n\n")),
		"refundPolicy": h.configValue("legal_refund_policy", strings.Join([]string{
			"退款需通过订单退款入口或售后工单提交，平台会结合套餐有效期、资源消耗、实际使用情况、优惠抵扣和风控记录进行审核。",
			"已产生明显资源消耗、存在违规使用、攻击滥用、恶意退款、重复购买后长期占用资源等情况的订单，平台可拒绝退款或按实际情况扣减费用。",
			"余额支付订单通过审核后优先退回账户余额；第三方支付订单需要人工核对支付流水，具体到账时间以支付渠道处理为准。",
			"平台暂不承诺自动退款，所有退款处理结果以后台审核记录和用户通知为准。",
		}, "\n\n")),
		"acceptableUse": h.configValue("legal_acceptable_use", strings.Join([]string{
			"禁止使用本平台服务进行垃圾邮件、端口扫描、暴力破解、DDoS、恶意代理、钓鱼欺诈、侵权分发、恶意程序传播等行为。",
			"禁止将服务用于违反当地法律法规、侵犯第三方权益、干扰网络稳定或绕过平台风控策略的用途。",
			"平台会根据流量特征、投诉记录、节点告警和安全审计结果处理异常账号，必要时可暂停相关隧道、规则、套餐或账号。",
			"用户如需开展高并发、压测、采集或其他可能影响平台稳定的业务，应提前与平台确认可用范围和资源限制。",
		}, "\n\n")),
	}))
}

func (h *Handler) epayNotify(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		_, _ = w.Write([]byte("fail"))
		return
	}
	values := r.Form
	key := h.configValue("epay_key", "")
	if key == "" || !verifyEpaySign(values, key) {
		_, _ = w.Write([]byte("fail"))
		return
	}
	if values.Get("pid") != h.configValue("epay_pid", "") {
		_, _ = w.Write([]byte("fail"))
		return
	}
	if values.Get("trade_status") != "TRADE_SUCCESS" {
		_, _ = w.Write([]byte("success"))
		return
	}
	orderNo := values.Get("out_trade_no")
	tradeNo := values.Get("trade_no")
	if strings.TrimSpace(orderNo) == "" || strings.TrimSpace(tradeNo) == "" {
		_, _ = w.Write([]byte("fail"))
		return
	}
	var order model.CommerceOrder
	if err := h.repo.DB().Where("order_no = ?", orderNo).First(&order).Error; err != nil {
		_, _ = w.Write([]byte("fail"))
		return
	}
	paidCents, err := parseMoneyCents(values.Get("money"))
	if err != nil || paidCents != order.AmountCents {
		_, _ = w.Write([]byte("fail"))
		return
	}
	if err := h.markOrderPaidAndProvision(&order, "epay", tradeNo, values.Encode()); err != nil {
		var latest model.CommerceOrder
		if readErr := h.repo.DB().Where("id = ?", order.ID).First(&latest).Error; readErr == nil &&
			normalizedPaymentStatus(latest) == paymentStatusPaid &&
			strings.TrimSpace(latest.ProviderTradeNo) == tradeNo {
			_, _ = w.Write([]byte("success"))
			return
		}
		_, _ = w.Write([]byte("fail"))
		return
	}
	_, _ = w.Write([]byte("success"))
}

func (h *Handler) epusdtNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		_, _ = w.Write([]byte("fail"))
		return
	}
	key := h.configValue("usdt_secret_key", "")
	if strings.TrimSpace(key) == "" {
		_, _ = w.Write([]byte("fail"))
		return
	}
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	var payload map[string]interface{}
	if err := decoder.Decode(&payload); err != nil {
		_, _ = w.Write([]byte("fail"))
		return
	}
	if !verifyEpusdtSign(payload, key) {
		_, _ = w.Write([]byte("fail"))
		return
	}
	if gotPID := epusdtStringValue(payload["pid"]); gotPID != "" && gotPID != h.configValue("usdt_pid", "") {
		_, _ = w.Write([]byte("fail"))
		return
	}
	if !epusdtStatusPaid(payload["status"]) {
		_, _ = w.Write([]byte("ok"))
		return
	}
	orderNo := epusdtStringValue(payload["order_id"])
	tradeNo := epusdtStringValue(payload["trade_id"])
	if strings.TrimSpace(orderNo) == "" || strings.TrimSpace(tradeNo) == "" {
		_, _ = w.Write([]byte("fail"))
		return
	}
	var order model.CommerceOrder
	if err := h.repo.DB().Where("order_no = ?", orderNo).First(&order).Error; err != nil {
		_, _ = w.Write([]byte("fail"))
		return
	}
	paidCents, err := epusdtAmountCents(payload["amount"])
	if err != nil || paidCents != order.AmountCents {
		_, _ = w.Write([]byte("fail"))
		return
	}
	rawBytes, _ := json.Marshal(payload)
	if err := h.markOrderPaidAndProvision(&order, paymentProviderEpusdt, tradeNo, string(rawBytes)); err != nil {
		var latest model.CommerceOrder
		if readErr := h.repo.DB().Where("id = ?", order.ID).First(&latest).Error; readErr == nil &&
			normalizedPaymentStatus(latest) == paymentStatusPaid &&
			strings.TrimSpace(latest.ProviderTradeNo) == tradeNo {
			_, _ = w.Write([]byte("ok"))
			return
		}
		_, _ = w.Write([]byte("fail"))
		return
	}
	_, _ = w.Write([]byte("ok"))
}

func (h *Handler) adminListInvites(w http.ResponseWriter, r *http.Request) {
	var req commerceListFilter
	_ = decodeJSON(r.Body, &req)
	var invites []model.InviteCode
	query := h.repo.DB().Model(&model.InviteCode{})
	if strings.TrimSpace(req.Keyword) != "" {
		query = query.Where("code LIKE ?", "%"+strings.TrimSpace(req.Keyword)+"%")
	}
	if strings.TrimSpace(req.Status) != "" {
		switch strings.TrimSpace(req.Status) {
		case "enabled", "active", "1":
			query = query.Where("status = ?", 1)
		case "disabled", "inactive", "0":
			query = query.Where("status = ?", 0)
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	page, pageSize, offset := commercePagination(req)
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&invites).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(commercePaginatedPayload(invites, total, page, pageSize)))
}

func (h *Handler) adminSaveInvite(w http.ResponseWriter, r *http.Request) {
	actorID, _, _ := userRoleFromRequest(r)
	var req invitePayload
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	now := time.Now().UnixMilli()
	code := strings.TrimSpace(req.Code)
	if code == "" {
		code = strings.ToUpper(randomHex(4))
	}
	if req.MaxUses <= 0 {
		req.MaxUses = 1
	}
	if req.Status != 1 {
		req.Status = 0
	}
	invite := model.InviteCode{ID: req.ID, Code: code, MaxUses: req.MaxUses, ExpTime: req.ExpTime, Status: req.Status, UpdatedTime: now}
	creatingInvite := invite.ID <= 0
	if invite.ID > 0 {
		if err := h.repo.DB().Model(&model.InviteCode{}).Where("id = ?", invite.ID).Updates(map[string]interface{}{
			"code": invite.Code, "max_uses": invite.MaxUses, "exp_time": invite.ExpTime, "status": invite.Status, "updated_time": invite.UpdatedTime,
		}).Error; err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
	} else {
		invite.CreatedTime = now
		if err := h.repo.DB().Create(&invite).Error; err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
	}
	action := "invite.create"
	if !creatingInvite {
		action = "invite.update"
	}
	h.writeAuditLog(actorID, "", action, "invite_code", invite.ID, "保存邀请码 "+invite.Code, map[string]interface{}{
		"code": invite.Code, "maxUses": invite.MaxUses, "expTime": invite.ExpTime, "status": invite.Status,
	})
	response.WriteJSON(w, response.OK(invite))
}

func (h *Handler) adminDeleteInvite(w http.ResponseWriter, r *http.Request) {
	actorID, _, _ := userRoleFromRequest(r)
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	if err := h.repo.DB().Model(&model.InviteCode{}).Where("id = ?", id).Update("status", 0).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	h.writeAuditLog(actorID, "", "invite.disable", "invite_code", id, fmt.Sprintf("停用邀请码 #%d", id), nil)
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) adminCommerceSettings(w http.ResponseWriter, r *http.Request) {
	keys := []string{
		"registration_enabled", "invite_registration_required",
		"epay_enabled", "epay_gateway", "epay_submit_url", "epay_pid", "epay_sitename", "epay_return_url", "epay_notify_url",
		"usdt_enabled", "usdt_api_base", "usdt_pid", "usdt_currency", "usdt_token", "usdt_network", "usdt_notify_url", "usdt_return_url",
		"reset_flow_enabled", "reset_flow_name",
		"legal_terms", "legal_privacy", "legal_refund_policy", "legal_acceptable_use",
	}
	cfg, _ := h.repo.GetConfigsByNames(keys)
	if strings.TrimSpace(h.configValue("epay_key", "")) != "" {
		cfg["epay_key_configured"] = "true"
	} else {
		cfg["epay_key_configured"] = "false"
	}
	if strings.TrimSpace(h.configValue("usdt_secret_key", "")) != "" {
		cfg["usdt_secret_key_configured"] = "true"
	} else {
		cfg["usdt_secret_key_configured"] = "false"
	}
	response.WriteJSON(w, response.OK(cfg))
}

func (h *Handler) adminUpdateCommerceSettings(w http.ResponseWriter, r *http.Request) {
	actorID, _, _ := userRoleFromRequest(r)
	var req map[string]string
	if err := decodeJSON(r.Body, &req); err != nil && err != io.EOF {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	allowed := map[string]struct{}{
		"registration_enabled": {}, "invite_registration_required": {}, "epay_enabled": {},
		"epay_gateway": {}, "epay_submit_url": {}, "epay_pid": {}, "epay_key": {}, "epay_sitename": {},
		"epay_return_url": {}, "epay_notify_url": {},
		"usdt_enabled": {}, "usdt_api_base": {}, "usdt_pid": {}, "usdt_secret_key": {}, "usdt_currency": {},
		"usdt_token": {}, "usdt_network": {}, "usdt_notify_url": {}, "usdt_return_url": {},
		"reset_flow_enabled": {}, "reset_flow_name": {},
		"legal_terms": {}, "legal_privacy": {}, "legal_refund_policy": {}, "legal_acceptable_use": {},
	}
	now := time.Now().UnixMilli()
	for key, value := range req {
		if _, ok := allowed[key]; !ok {
			continue
		}
		if err := h.repo.UpsertConfig(key, strings.TrimSpace(value), now); err != nil {
			response.WriteJSON(w, response.Err(-2, err.Error()))
			return
		}
	}
	updatedKeys := make([]string, 0, len(req))
	for key := range req {
		if _, ok := allowed[key]; ok {
			updatedKeys = append(updatedKeys, key)
		}
	}
	h.writeAuditLog(actorID, "", "commerce.settings.update", "commerce_settings", 0, "更新商业化配置", map[string]interface{}{"keys": updatedKeys})
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) adminSyncUserResources(w http.ResponseWriter, r *http.Request) {
	actorID, _, err := userRoleFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "无效的token或token已过期"))
		return
	}
	var req struct {
		UserID int64 `json:"userId"`
	}
	if err := decodeJSON(r.Body, &req); err != nil || req.UserID <= 0 {
		response.WriteJSON(w, response.ErrDefault("请选择需要同步的用户"))
		return
	}
	now := time.Now().UnixMilli()
	if err := h.syncUserPackageResources(req.UserID, now); err != nil {
		h.writeAuditLog(actorID, "", "resource.sync_failed", "user", req.UserID, fmt.Sprintf("同步用户 #%d 套餐资源失败", req.UserID), map[string]interface{}{"error": err.Error()})
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	h.writeAuditLog(actorID, "", "resource.sync", "user", req.UserID, fmt.Sprintf("同步用户 #%d 套餐资源", req.UserID), nil)
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) adminListResourceJobs(w http.ResponseWriter, r *http.Request) {
	var req commerceListFilter
	_ = decodeJSON(r.Body, &req)
	var rows []model.CommerceResourceJob
	query := h.repo.DB().Model(&model.CommerceResourceJob{})
	if req.UserID > 0 {
		query = query.Where("user_id = ?", req.UserID)
	}
	if strings.TrimSpace(req.Status) != "" {
		query = query.Where("status = ?", strings.TrimSpace(req.Status))
	}
	if strings.TrimSpace(req.Keyword) != "" {
		kw := "%" + strings.TrimSpace(req.Keyword) + "%"
		query = query.Where("job_type LIKE ? OR last_error LIKE ?", kw, kw)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	page, pageSize, offset := commercePagination(req)
	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(commercePaginatedPayload(rows, total, page, pageSize)))
}

func (h *Handler) adminRetryResourceJob(w http.ResponseWriter, r *http.Request) {
	actorID, _, _ := userRoleFromRequest(r)
	id := idFromBody(r, w)
	if id <= 0 {
		return
	}
	var job model.CommerceResourceJob
	if err := h.repo.DB().Where("id = ?", id).First(&job).Error; err != nil {
		response.WriteJSON(w, response.ErrDefault("资源任务不存在"))
		return
	}
	if job.Status == "done" {
		response.WriteJSON(w, response.ErrDefault("资源任务已完成"))
		return
	}
	if err := h.processCommerceResourceJob(job, time.Now().UnixMilli()); err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	h.writeAuditLog(actorID, "", "resource_job.retry", "commerce_resource_job", id, fmt.Sprintf("人工重试资源任务 #%d", id), nil)
	response.WriteJSON(w, response.OKEmpty())
}

func (h *Handler) listPlans(publicOnly bool) ([]map[string]interface{}, error) {
	var plans []model.Plan
	query := h.repo.DB().Order("sort ASC, id DESC")
	if publicOnly {
		query = query.Where("status = 1")
	}
	if err := query.Find(&plans).Error; err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(plans))
	for _, plan := range plans {
		entitlements, err := h.planEntitlementIDs(plan.ID)
		if err != nil {
			return nil, err
		}
		speedID := interface{}(nil)
		if plan.SpeedID.Valid {
			speedID = plan.SpeedID.Int64
		}
		scopeKey, err := h.planScopeKey(plan)
		if err != nil {
			return nil, err
		}
		out = append(out, map[string]interface{}{
			"id": plan.ID, "name": plan.Name, "description": plan.Description, "category": plan.Category,
			"scopeKey":   scopeKey,
			"priceCents": plan.PriceCents, "resetFlowPriceCents": plan.ResetFlowPriceCents, "currency": plan.Currency, "durationDays": plan.DurationDays,
			"flow": plan.Flow, "dailyQuotaGB": plan.DailyQuotaGB, "monthlyQuotaGB": plan.MonthlyQuotaGB,
			"num": plan.Num, "maxConn": plan.MaxConn, "speedId": speedID, "sort": plan.Sort, "status": plan.Status,
			"tunnelIds": entitlements["tunnel"], "tunnelGroupIds": entitlements["tunnel_group"],
			"tunnelNames":      h.tunnelNames(entitlements["tunnel"]),
			"tunnelGroupNames": h.tunnelGroupNames(entitlements["tunnel_group"]),
			"tunnels":          h.planTunnelPayloads(plan.ID),
		})
	}
	return out, nil
}

func (h *Handler) subscriptionPayload(sub model.UserSubscription) userSubscriptionPayload {
	var plan model.Plan
	_ = h.repo.DB().Where("id = ?", sub.PlanID).First(&plan).Error
	scopeKey, _ := h.planScopeKey(plan)
	return userSubscriptionPayload{
		ID: sub.ID, UserID: sub.UserID, PlanID: sub.PlanID, OrderID: sub.OrderID,
		Status: sub.Status, StartsAt: sub.StartsAt, ExpiresAt: sub.ExpiresAt,
		Snapshot: sub.Snapshot, CreatedTime: sub.CreatedTime, UpdatedTime: sub.UpdatedTime,
		PlanName: plan.Name, PlanCategory: plan.Category, PlanScopeKey: scopeKey, PlanPriceCents: plan.PriceCents,
		PlanFlow: plan.Flow, PlanNum: plan.Num, PlanMaxConn: plan.MaxConn,
		PlanTunnels:         h.planTunnelPayloads(sub.PlanID),
		ResetFlowPriceCents: plan.ResetFlowPriceCents,
		ResetFlowName:       fmt.Sprintf("%s - %s", h.configValue("reset_flow_name", "重置套餐流量"), plan.Name),
	}
}

func subscriptionPayloadMap(item userSubscriptionPayload, resetFlowEnabled bool) map[string]interface{} {
	return map[string]interface{}{
		"id": item.ID, "userId": item.UserID, "planId": item.PlanID, "orderId": item.OrderID,
		"status": item.Status, "startsAt": item.StartsAt, "expiresAt": item.ExpiresAt,
		"snapshot": item.Snapshot, "createdTime": item.CreatedTime, "updatedTime": item.UpdatedTime,
		"planName": item.PlanName, "planCategory": item.PlanCategory, "planScopeKey": item.PlanScopeKey,
		"planPriceCents": item.PlanPriceCents, "planFlow": item.PlanFlow, "planNum": item.PlanNum,
		"planMaxConn": item.PlanMaxConn, "planTunnels": item.PlanTunnels,
		"resetFlowEnabled": resetFlowEnabled, "resetFlowPriceCents": item.ResetFlowPriceCents, "resetFlowName": item.ResetFlowName,
	}
}

func (h *Handler) commerceOrderPayloads(orders []model.CommerceOrder) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(orders))
	for _, order := range orders {
		out = append(out, h.commerceOrderPayload(order))
	}
	return out
}

func (h *Handler) commerceOrderPayload(order model.CommerceOrder) map[string]interface{} {
	plan, _ := h.planForOrder(order)
	paymentStatus := normalizedPaymentStatus(order)
	fulfillmentStatus := normalizedFulfillmentStatus(order)
	refundStatus := normalizedRefundStatus(order)
	canPay := paymentStatus == paymentStatusUnpaid && (order.Status == orderStatusPending || order.Status == orderStatusFailed)
	canCancel := paymentStatus == paymentStatusUnpaid && (order.Status == orderStatusPending || order.Status == orderStatusFailed)
	canRefund := paymentStatus == paymentStatusPaid && fulfillmentStatus == fulfillmentStatusDone && refundStatus == refundStatusNone && order.AmountCents > 0
	return map[string]interface{}{
		"id": order.ID, "orderNo": order.OrderNo, "userId": order.UserID, "planId": order.PlanID,
		"planName": plan.Name, "amountCents": order.AmountCents, "currency": order.Currency,
		"status": order.Status, "paymentStatus": paymentStatus, "fulfillmentStatus": fulfillmentStatus,
		"refundStatus": refundStatus, "refundAmountCents": order.RefundAmountCents, "refundReason": order.RefundReason,
		"orderType": order.OrderType, "paymentProvider": order.PaymentProvider, "providerTradeNo": order.ProviderTradeNo,
		"paidTime": order.PaidTime, "provisionedTime": order.ProvisionedTime, "createdTime": order.CreatedTime,
		"updatedTime": order.UpdatedTime, "cancelledTime": order.CancelledTime, "refundedTime": order.RefundedTime,
		"failureReason": order.FailureReason, "canPay": canPay, "canCancel": canCancel, "canRefund": canRefund,
	}
}

func (h *Handler) applyCommerceOrderFilter(query *gorm.DB, req commerceListFilter) *gorm.DB {
	if query == nil {
		return query
	}
	if strings.TrimSpace(req.Keyword) != "" {
		kw := "%" + strings.TrimSpace(req.Keyword) + "%"
		query = query.Where("order_no LIKE ? OR provider_trade_no LIKE ?", kw, kw)
	}
	if strings.TrimSpace(req.OrderNo) != "" {
		query = query.Where("order_no LIKE ?", "%"+strings.TrimSpace(req.OrderNo)+"%")
	}
	if strings.TrimSpace(req.Status) != "" {
		query = query.Where("status = ? OR payment_status = ? OR fulfillment_status = ? OR refund_status = ?", req.Status, req.Status, req.Status, req.Status)
	}
	if strings.TrimSpace(req.OrderType) != "" {
		query = query.Where("order_type = ?", strings.TrimSpace(req.OrderType))
	}
	if strings.TrimSpace(req.Provider) != "" {
		query = query.Where("payment_provider = ?", strings.TrimSpace(req.Provider))
	}
	if req.UserID > 0 {
		query = query.Where("user_id = ?", req.UserID)
	}
	if req.DateFrom > 0 {
		query = query.Where("created_time >= ?", req.DateFrom)
	}
	if req.DateTo > 0 {
		query = query.Where("created_time <= ?", req.DateTo)
	}
	return query
}

func (h *Handler) proratedUpgradeAmount(targetPlan, activePlan model.Plan, sub model.UserSubscription, now int64) int64 {
	if targetPlan.PriceCents <= 0 {
		return 0
	}
	if activePlan.PriceCents <= 0 || activePlan.DurationDays <= 0 || sub.ExpiresAt <= now {
		return targetPlan.PriceCents
	}
	totalMs := int64(activePlan.DurationDays) * int64((24*time.Hour)/time.Millisecond)
	if totalMs <= 0 {
		return targetPlan.PriceCents
	}
	remainingMs := sub.ExpiresAt - now
	if remainingMs < 0 {
		remainingMs = 0
	}
	if remainingMs > totalMs {
		remainingMs = totalMs
	}
	credit := activePlan.PriceCents * remainingMs / totalMs
	amount := targetPlan.PriceCents - credit
	if amount < 1 {
		amount = 1
	}
	if amount > targetPlan.PriceCents {
		amount = targetPlan.PriceCents
	}
	return amount
}

func (h *Handler) activeSubscriptionPlan(userID int64) (*model.UserSubscription, model.Plan, error) {
	var sub model.UserSubscription
	err := h.repo.DB().Where("user_id = ? AND status = ? AND expires_at > ?", userID, "active", time.Now().UnixMilli()).
		Order("expires_at DESC, id DESC").First(&sub).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, model.Plan{}, nil
		}
		return nil, model.Plan{}, err
	}
	var plan model.Plan
	if err := h.repo.DB().Where("id = ?", sub.PlanID).First(&plan).Error; err != nil {
		return &sub, model.Plan{}, err
	}
	return &sub, plan, nil
}

func (h *Handler) activeSubscriptionByIDOrLatest(userID, subscriptionID int64) (*model.UserSubscription, model.Plan, error) {
	query := h.repo.DB().Where("user_id = ? AND status = ? AND expires_at > ?", userID, "active", time.Now().UnixMilli())
	if subscriptionID > 0 {
		query = query.Where("id = ?", subscriptionID)
	}
	var sub model.UserSubscription
	err := query.Order("expires_at DESC, id DESC").First(&sub).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, model.Plan{}, nil
		}
		return nil, model.Plan{}, err
	}
	var plan model.Plan
	if err := h.repo.DB().Where("id = ?", sub.PlanID).First(&plan).Error; err != nil {
		return &sub, model.Plan{}, err
	}
	return &sub, plan, nil
}

func (h *Handler) activeSubscriptionForPlanScope(userID int64, target model.Plan) (*activeSubscriptionMatch, error) {
	targetScope, err := h.planScopeKey(target)
	if err != nil {
		return nil, err
	}
	var subs []model.UserSubscription
	if err := h.repo.DB().Where("user_id = ? AND status = ? AND expires_at > ?", userID, "active", time.Now().UnixMilli()).
		Order("expires_at DESC, id DESC").Find(&subs).Error; err != nil {
		return nil, err
	}
	for _, sub := range subs {
		var plan model.Plan
		if err := h.repo.DB().Where("id = ?", sub.PlanID).First(&plan).Error; err != nil {
			return nil, err
		}
		scope, err := h.planScopeKey(plan)
		if err != nil {
			return nil, err
		}
		if scope == targetScope {
			return &activeSubscriptionMatch{Sub: sub, Plan: plan, SamePlan: plan.ID == target.ID}, nil
		}
	}
	return nil, nil
}

func (h *Handler) activeSubscriptionTunnelOverlap(userID int64, target model.Plan, skipSubscriptionID int64, now int64) ([]string, error) {
	targetIDs, err := h.resolvePlanTunnelIDs(target.ID)
	if err != nil {
		return nil, err
	}
	if len(targetIDs) == 0 {
		return nil, nil
	}
	targetSet := make(map[int64]struct{}, len(targetIDs))
	for _, id := range targetIDs {
		targetSet[id] = struct{}{}
	}

	var subs []model.UserSubscription
	if err := h.repo.DB().Where("user_id = ? AND status = ? AND expires_at > ?", userID, "active", now).
		Order("expires_at DESC, id DESC").Find(&subs).Error; err != nil {
		return nil, err
	}
	overlap := map[int64]struct{}{}
	for _, sub := range subs {
		if sub.ID == skipSubscriptionID {
			continue
		}
		ids, err := h.resolvePlanTunnelIDs(sub.PlanID)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if _, ok := targetSet[id]; ok {
				overlap[id] = struct{}{}
			}
		}
	}
	if len(overlap) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(overlap))
	for id := range overlap {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return h.tunnelNames(ids), nil
}

func (h *Handler) planScopeKey(plan model.Plan) (string, error) {
	ids, err := h.resolvePlanTunnelIDs(plan.ID)
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return fmt.Sprintf("plan:%d", plan.ID), nil
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return "tunnels:" + strings.Join(parts, ","), nil
}

func (h *Handler) planForOrder(order model.CommerceOrder) (model.Plan, error) {
	if order.OrderType == orderTypeResetFlow {
		name := h.configValue("reset_flow_name", "重置套餐流量")
		if strings.TrimSpace(order.Snapshot) != "" {
			var snap map[string]interface{}
			if err := json.Unmarshal([]byte(order.Snapshot), &snap); err == nil {
				if v, ok := snap["name"].(string); ok && strings.TrimSpace(v) != "" {
					name = strings.TrimSpace(v)
				}
			}
		}
		return model.Plan{Name: name, Currency: order.Currency}, nil
	}
	if order.OrderType == orderTypeWalletRecharge {
		return model.Plan{Name: "账户余额充值", Currency: order.Currency}, nil
	}

	var plan model.Plan
	if strings.TrimSpace(order.Snapshot) != "" {
		if err := json.Unmarshal([]byte(order.Snapshot), &plan); err != nil || plan.Name == "" {
			var snap struct {
				Plan model.Plan `json:"plan"`
			}
			_ = json.Unmarshal([]byte(order.Snapshot), &snap)
			if snap.Plan.Name != "" {
				plan = snap.Plan
			}
		}
	}
	if plan.Name != "" {
		return plan, nil
	}
	err := h.repo.DB().Where("id = ?", order.PlanID).First(&plan).Error
	return plan, err
}

func (h *Handler) tunnelNames(ids []int64) []string {
	if len(ids) == 0 {
		return []string{}
	}
	var rows []model.Tunnel
	if err := h.repo.DB().Select("id", "name").Where("id IN ?", ids).Order("id ASC").Find(&rows).Error; err != nil {
		return []string{}
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	return names
}

func (h *Handler) planTunnelPayloads(planID int64) []planTunnelPayload {
	ids, err := h.resolvePlanTunnelIDs(planID)
	if err != nil || len(ids) == 0 {
		return []planTunnelPayload{}
	}
	var rows []model.Tunnel
	if err := h.repo.DB().Select("id", "name", "traffic_ratio").Where("id IN ?", ids).Order("id ASC").Find(&rows).Error; err != nil {
		return []planTunnelPayload{}
	}
	out := make([]planTunnelPayload, 0, len(rows))
	for _, row := range rows {
		ratio := row.TrafficRatio
		if ratio <= 0 {
			ratio = 1
		}
		out = append(out, planTunnelPayload{ID: row.ID, Name: row.Name, TrafficRatio: ratio})
	}
	return out
}

func (h *Handler) tunnelGroupNames(ids []int64) []string {
	if len(ids) == 0 {
		return []string{}
	}
	var rows []model.TunnelGroup
	if err := h.repo.DB().Select("id", "name").Where("id IN ?", ids).Order("id ASC").Find(&rows).Error; err != nil {
		return []string{}
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	return names
}

func (h *Handler) planEntitlementIDs(planID int64) (map[string][]int64, error) {
	var items []model.PlanEntitlement
	if err := h.repo.DB().Where("plan_id = ?", planID).Order("id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	out := map[string][]int64{"tunnel": {}, "tunnel_group": {}}
	for _, item := range items {
		out[item.ScopeType] = append(out[item.ScopeType], item.ScopeID)
	}
	return out, nil
}

func (h *Handler) markOrderPaidAndProvision(order *model.CommerceOrder, provider, tradeNo string, rawPayload ...string) error {
	now := time.Now().UnixMilli()
	raw := ""
	if len(rawPayload) > 0 {
		raw = rawPayload[0]
	}
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		tradeNo = fmt.Sprintf("%s-%s-%d", provider, order.OrderNo, now)
	}

	var shouldProvision bool
	if err := h.repo.DB().Transaction(func(tx *gorm.DB) error {
		var current model.CommerceOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", order.ID).First(&current).Error; err != nil {
			return err
		}
		if normalizedFulfillmentStatus(current) == fulfillmentStatusDone || current.Status == orderStatusActive {
			if current.ProviderTradeNo != "" && current.ProviderTradeNo != tradeNo {
				return fmt.Errorf("订单已绑定其他支付流水")
			}
			if current.PaymentProvider != "" && current.PaymentProvider != provider && current.ProviderTradeNo != "" {
				return fmt.Errorf("支付渠道不一致")
			}
			updates := map[string]interface{}{
				"payment_status":     paymentStatusPaid,
				"fulfillment_status": fulfillmentStatusDone,
				"updated_time":       now,
			}
			if current.PaidTime == 0 {
				updates["paid_time"] = now
			}
			if current.ProvisionedTime == 0 {
				updates["provisioned_time"] = now
			}
			if current.PaymentProvider == "" || current.ProviderTradeNo == "" {
				updates["payment_provider"] = provider
				updates["provider_trade_no"] = tradeNo
				current.PaymentProvider = provider
				current.ProviderTradeNo = tradeNo
			}
			if err := h.createPaymentRecordTx(tx, current, provider, tradeNo, raw, now); err != nil {
				return err
			}
			if err := tx.Model(&model.CommerceOrder{}).Where("id = ?", current.ID).Updates(updates).Error; err != nil {
				return err
			}
			*order = current
			return nil
		}
		if current.Status == orderStatusCancelled || current.Status == orderStatusRefunded {
			return fmt.Errorf("订单已取消或已退款，不能发放")
		}
		if current.ProviderTradeNo != "" && current.ProviderTradeNo != tradeNo {
			return fmt.Errorf("订单已绑定其他支付流水")
		}
		if current.AmountCents < 0 {
			return fmt.Errorf("支付金额异常")
		}
		if tradeNo == "" {
			return fmt.Errorf("支付流水号为空")
		}
		if current.PaymentProvider != "" && current.PaymentProvider != provider && current.ProviderTradeNo != "" {
			return fmt.Errorf("支付渠道不一致")
		}
		if normalizedPaymentStatus(current) == paymentStatusPaid {
			if current.ProviderTradeNo == tradeNo {
				shouldProvision = normalizedFulfillmentStatus(current) != fulfillmentStatusDone
				return nil
			}
			return fmt.Errorf("订单已支付")
		}
		if current.Status != orderStatusPending && current.Status != orderStatusFailed && current.Status != orderStatusProvisioning && current.Status != orderStatusPaid {
			return fmt.Errorf("当前订单状态不能支付")
		}
		if normalizedRefundStatus(current) != refundStatusNone {
			return fmt.Errorf("订单存在退款状态，不能支付")
		}
		if err := h.createPaymentRecordTx(tx, current, provider, tradeNo, raw, now); err != nil {
			return err
		}
		updates := map[string]interface{}{
			"status": orderStatusProvisioning, "payment_status": paymentStatusPaid, "fulfillment_status": fulfillmentStatusProvisioning,
			"payment_provider": provider, "provider_trade_no": tradeNo, "updated_time": now,
		}
		if current.PaidTime == 0 {
			updates["paid_time"] = now
		}
		if err := tx.Model(&model.CommerceOrder{}).Where("id = ?", current.ID).Updates(updates).Error; err != nil {
			return err
		}
		shouldProvision = true
		return nil
	}); err != nil {
		h.writeAuditLog(0, "", "payment.failed", "commerce_order", order.ID, err.Error(), map[string]interface{}{"provider": provider, "tradeNo": tradeNo})
		return err
	}
	if !shouldProvision {
		return nil
	}
	h.writeAuditLog(order.UserID, "", "payment.success", "commerce_order", order.ID, "订单支付成功 "+order.OrderNo, map[string]interface{}{"provider": provider, "tradeNo": tradeNo})
	h.createNotification(order.UserID, "订单支付成功", fmt.Sprintf("订单 %s 已完成支付，正在发放资源。", order.OrderNo), "success")
	if err := h.provisionOrder(order.ID); err != nil {
		_ = h.repo.DB().Model(&model.CommerceOrder{}).Where("id = ?", order.ID).Updates(map[string]interface{}{
			"status": orderStatusFailed, "fulfillment_status": fulfillmentStatusFailed, "failure_reason": err.Error(), "updated_time": now,
		}).Error
		h.enqueueCommerceResourceJob("provision_order", order.UserID, order.ID, err.Error(), nil, now)
		h.writeAuditLog(order.UserID, "", "order.provision_failed", "commerce_order", order.ID, err.Error(), nil)
		h.createNotification(order.UserID, "资源发放失败", fmt.Sprintf("订单 %s 支付成功但资源发放失败，请联系管理员。", order.OrderNo), "error")
		return err
	}
	return nil
}

func (h *Handler) createPaymentRecordTx(tx *gorm.DB, order model.CommerceOrder, provider, tradeNo, raw string, now int64) error {
	record := model.PaymentRecord{
		OrderID: order.ID, OrderNo: order.OrderNo, Provider: provider, ProviderTradeNo: tradeNo,
		AmountCents: order.AmountCents, Status: "success", RawPayload: raw, CreatedTime: now, UpdatedTime: now,
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	var existing model.PaymentRecord
	if err := tx.Where("provider = ? AND provider_trade_no = ?", provider, tradeNo).First(&existing).Error; err != nil {
		return err
	}
	if existing.OrderID != order.ID || existing.OrderNo != order.OrderNo || existing.AmountCents != order.AmountCents {
		return fmt.Errorf("支付流水号已被其他订单使用")
	}
	return nil
}

func (h *Handler) provisionOrder(orderID int64) error {
	var order model.CommerceOrder
	if err := h.repo.DB().Where("id = ?", orderID).First(&order).Error; err != nil {
		return err
	}
	if order.OrderType == orderTypeResetFlow {
		return h.provisionResetFlowOrder(order)
	}
	if order.OrderType == orderTypeWalletRecharge {
		return h.provisionWalletRechargeOrder(order)
	}
	var plan model.Plan
	if err := h.repo.DB().Where("id = ?", order.PlanID).First(&plan).Error; err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	expiresBase := now
	activeMatch, err := h.activeSubscriptionForPlanScope(order.UserID, plan)
	if err != nil {
		return err
	}
	if order.OrderType == orderTypeRenew && activeMatch != nil && activeMatch.SamePlan && activeMatch.Sub.ExpiresAt > expiresBase {
		expiresBase = activeMatch.Sub.ExpiresAt
	}
	expiresAt := time.UnixMilli(expiresBase).Add(time.Duration(plan.DurationDays) * 24 * time.Hour).UnixMilli()
	txErr := h.repo.DB().Transaction(func(tx *gorm.DB) error {
		if order.OrderType == orderTypeRenew && activeMatch != nil && activeMatch.SamePlan {
			if err := tx.Model(&model.UserSubscription{}).Where("id = ?", activeMatch.Sub.ID).Updates(map[string]interface{}{
				"order_id": order.ID, "expires_at": expiresAt, "snapshot": order.Snapshot, "updated_time": now,
			}).Error; err != nil {
				return err
			}
		} else {
			if activeMatch != nil {
				if err := tx.Model(&model.UserSubscription{}).
					Where("id = ? AND status = ?", activeMatch.Sub.ID, "active").
					Updates(map[string]interface{}{"status": "replaced", "updated_time": now}).Error; err != nil {
					return err
				}
			}
			startsAt := now
			if activeMatch != nil && activeMatch.Sub.StartsAt > 0 {
				startsAt = activeMatch.Sub.StartsAt
			}
			sub := model.UserSubscription{
				UserID: order.UserID, PlanID: plan.ID, OrderID: order.ID, Status: "active",
				StartsAt: startsAt, ExpiresAt: expiresAt, Snapshot: order.Snapshot, CreatedTime: now, UpdatedTime: now,
			}
			if err := tx.Create(&sub).Error; err != nil {
				return err
			}
		}
		return tx.Model(&model.CommerceOrder{}).Where("id = ?", order.ID).Updates(map[string]interface{}{
			"status": orderStatusActive, "payment_status": paymentStatusPaid, "fulfillment_status": fulfillmentStatusDone,
			"provisioned_time": now, "updated_time": now,
		}).Error
	})
	if txErr == nil {
		if err := h.syncUserPackageResources(order.UserID, now); err != nil {
			h.rollbackProvisionedOrder(order, activeMatch, now)
			return err
		}
		h.writeAuditLog(order.UserID, "", "order.provisioned", "commerce_order", order.ID, "订单资源已发放 "+order.OrderNo, nil)
		h.createNotification(order.UserID, "套餐已开通", fmt.Sprintf("套餐 %s 已开通，到期时间 %s。", plan.Name, time.UnixMilli(expiresAt).Format("2006-01-02 15:04:05")), "success")
		h.consumeCouponForOrder(order)
	}
	return txErr
}

func (h *Handler) rollbackProvisionedOrder(order model.CommerceOrder, activeMatch *activeSubscriptionMatch, now int64) {
	_ = h.repo.DB().Transaction(func(tx *gorm.DB) error {
		if activeMatch != nil && order.OrderType == orderTypeRenew && activeMatch.SamePlan {
			if err := tx.Model(&model.UserSubscription{}).Where("id = ?", activeMatch.Sub.ID).Updates(map[string]interface{}{
				"order_id": activeMatch.Sub.OrderID, "starts_at": activeMatch.Sub.StartsAt, "expires_at": activeMatch.Sub.ExpiresAt,
				"snapshot": activeMatch.Sub.Snapshot, "status": activeMatch.Sub.Status, "updated_time": now,
			}).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&model.UserSubscription{}).Where("order_id = ? AND status = ?", order.ID, "active").Updates(map[string]interface{}{
				"status": "failed", "updated_time": now,
			}).Error; err != nil {
				return err
			}
			if activeMatch != nil {
				if err := tx.Model(&model.UserSubscription{}).Where("id = ?", activeMatch.Sub.ID).Updates(map[string]interface{}{
					"status": activeMatch.Sub.Status, "updated_time": now,
				}).Error; err != nil {
					return err
				}
			}
		}
		return tx.Model(&model.CommerceOrder{}).Where("id = ?", order.ID).Updates(map[string]interface{}{
			"status": orderStatusFailed, "payment_status": paymentStatusPaid, "fulfillment_status": fulfillmentStatusFailed,
			"failure_reason": "资源发放失败，已回滚套餐状态", "updated_time": now,
		}).Error
	})
	_ = h.syncUserPackageResources(order.UserID, now)
}

func (h *Handler) provisionResetFlowOrder(order model.CommerceOrder) error {
	now := time.Now()
	nowMs := now.UnixMilli()
	var sub model.UserSubscription
	query := h.repo.DB().Where("user_id = ? AND plan_id = ? AND status = ? AND expires_at > ?", order.UserID, order.PlanID, "active", nowMs)
	if subscriptionID := subscriptionIDFromSnapshot(order.Snapshot); subscriptionID > 0 {
		query = query.Where("id = ?", subscriptionID)
	}
	if err := query.Order("expires_at DESC, id DESC").First(&sub).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("当前套餐已失效，无法重置流量")
		}
		return err
	}
	tunnelIDs, err := h.resolvePlanTunnelIDs(sub.PlanID)
	if err != nil {
		return err
	}
	if len(tunnelIDs) == 0 {
		return fmt.Errorf("当前没有有效套餐，无法重置流量")
	}
	var userTunnels []model.UserTunnel
	if err := h.repo.DB().Where("user_id = ? AND tunnel_id IN ?", order.UserID, tunnelIDs).Find(&userTunnels).Error; err != nil {
		return err
	}
	for _, userTunnel := range userTunnels {
		h.repo.ResetUserFlowByUserTunnel(userTunnel.ID)
	}
	if err := h.repo.ResetSubscriptionQuotaUsage(sub.ID, now); err != nil {
		return err
	}
	release, err := h.repo.RebuildUserQuotaUsageFromActiveSubscriptions(order.UserID, now)
	if err != nil {
		return err
	}
	h.applyUserQuotaRelease(release, nowMs)

	return h.repo.DB().Model(&model.CommerceOrder{}).Where("id = ?", order.ID).Updates(map[string]interface{}{
		"status": orderStatusActive, "payment_status": paymentStatusPaid, "fulfillment_status": fulfillmentStatusDone,
		"provisioned_time": nowMs, "updated_time": nowMs,
	}).Error
}

func (h *Handler) provisionWalletRechargeOrder(order model.CommerceOrder) error {
	now := time.Now().UnixMilli()
	return h.repo.DB().Transaction(func(tx *gorm.DB) error {
		item, err := h.addWalletLedgerTx(tx, order.UserID, order.AmountCents, "recharge", "commerce_order", order.ID, "余额充值订单 "+order.OrderNo, now, false)
		if err != nil {
			return err
		}
		if err := tx.Model(&model.CommerceOrder{}).Where("id = ?", order.ID).Updates(map[string]interface{}{
			"status": orderStatusActive, "payment_status": paymentStatusPaid, "fulfillment_status": fulfillmentStatusDone,
			"provisioned_time": now, "updated_time": now,
		}).Error; err != nil {
			return err
		}
		h.createNotification(order.UserID, "余额充值成功", fmt.Sprintf("已充值 %s，当前余额 %s。", formatCentsForNotice(order.AmountCents), formatCentsForNotice(item.BalanceAfterCents)), "success")
		h.writeAuditLog(order.UserID, "", "wallet.recharged", "commerce_order", order.ID, "余额充值成功 "+order.OrderNo, item)
		return nil
	})
}

type tunnelEntitlementAggregate struct {
	TunnelID int64
	Flow     int64
	Num      int
	ExpTime  int64
	SpeedID  sql.NullInt64
}

func (h *Handler) syncUserPackageResources(userID int64, now int64) error {
	var subs []model.UserSubscription
	if err := h.repo.DB().Where("user_id = ? AND status = ? AND expires_at > ?", userID, "active", now).Find(&subs).Error; err != nil {
		return err
	}
	tunnelEntitlements := map[int64]tunnelEntitlementAggregate{}
	var totalFlow int64
	var totalNum int
	var maxConn int
	var dailyQuota int64
	var monthlyQuota int64
	var maxExp int64
	for _, sub := range subs {
		var plan model.Plan
		if err := h.repo.DB().Where("id = ?", sub.PlanID).First(&plan).Error; err != nil {
			return err
		}
		totalFlow += plan.Flow
		totalNum += plan.Num
		dailyQuota += plan.DailyQuotaGB
		monthlyQuota += plan.MonthlyQuotaGB
		if plan.MaxConn > maxConn {
			maxConn = plan.MaxConn
		}
		if sub.ExpiresAt > maxExp {
			maxExp = sub.ExpiresAt
		}
		tunnelIDs, err := h.resolvePlanTunnelIDs(plan.ID)
		if err != nil {
			return err
		}
		for _, tunnelID := range tunnelIDs {
			item := tunnelEntitlements[tunnelID]
			item.TunnelID = tunnelID
			item.Flow += plan.Flow
			item.Num += plan.Num
			if sub.ExpiresAt > item.ExpTime {
				item.ExpTime = sub.ExpiresAt
			}
			if plan.SpeedID.Valid {
				item.SpeedID = plan.SpeedID
			}
			tunnelEntitlements[tunnelID] = item
		}
	}
	if maxExp == 0 {
		h.pauseUserTunnelsOutsidePlan(userID, []int64{}, now)
		return h.repo.DB().Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
				"flow": int64(0), "num": 0, "exp_time": int64(0), "max_conn": 0,
				"updated_time": sql.NullInt64{Int64: now, Valid: true},
			}).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.UserQuota{}).Where("user_id = ?", userID).Updates(map[string]interface{}{
				"daily_limit_gb": int64(0), "monthly_limit_gb": int64(0), "updated_time": now,
			}).Error; err != nil {
				return err
			}
			return tx.Model(&model.UserSubscriptionQuota{}).Where("user_id = ?", userID).Updates(map[string]interface{}{
				"daily_limit_gb": int64(0), "monthly_limit_gb": int64(0), "updated_time": now,
			}).Error
		})
	}
	for _, item := range tunnelEntitlements {
		req := map[string]interface{}{
			"userId": userID, "tunnelId": item.TunnelID, "flow": item.Flow, "num": item.Num,
			"expTime": item.ExpTime, "flowResetTime": int64(1), "status": 1,
		}
		if item.SpeedID.Valid {
			req["speedId"] = item.SpeedID.Int64
		}
		if err := h.upsertUserTunnel(req); err != nil {
			return err
		}
	}
	keepTunnelIDs := make([]int64, 0, len(tunnelEntitlements))
	for tunnelID := range tunnelEntitlements {
		keepTunnelIDs = append(keepTunnelIDs, tunnelID)
	}
	h.pauseUserTunnelsOutsidePlan(userID, keepTunnelIDs, now)
	return h.repo.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
			"flow": totalFlow, "num": totalNum, "exp_time": maxExp, "flow_reset_time": int64(1),
			"max_conn": maxConn, "status": 1, "updated_time": sql.NullInt64{Int64: now, Valid: true},
		}).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"daily_limit_gb", "monthly_limit_gb", "updated_time"}),
		}).Create(&model.UserQuota{
			UserID: userID, DailyLimitGB: dailyQuota, MonthlyLimitGB: monthlyQuota,
			CreatedTime: now, UpdatedTime: now,
		}).Error; err != nil {
			return err
		}
		for _, sub := range subs {
			var plan model.Plan
			if err := tx.Where("id = ?", sub.PlanID).First(&plan).Error; err != nil {
				return err
			}
			if err := h.repo.UpsertSubscriptionQuotaConfigTx(tx, sub, plan, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func (h *Handler) resolvePlanTunnelIDs(planID int64) ([]int64, error) {
	var entitlements []model.PlanEntitlement
	if err := h.repo.DB().Where("plan_id = ?", planID).Find(&entitlements).Error; err != nil {
		return nil, err
	}
	tunnelIDs := make([]int64, 0)
	groupIDs := make([]int64, 0)
	for _, item := range entitlements {
		switch item.ScopeType {
		case "tunnel":
			tunnelIDs = append(tunnelIDs, item.ScopeID)
		case "tunnel_group":
			groupIDs = append(groupIDs, item.ScopeID)
		}
	}
	return h.resolveTunnelIDsFromScopes(tunnelIDs, groupIDs)
}

func (h *Handler) resolveTunnelIDsFromScopes(tunnelIDs []int64, groupIDs []int64) ([]int64, error) {
	seen := map[int64]struct{}{}
	for _, id := range uniqueInt64(tunnelIDs) {
		if id > 0 {
			seen[id] = struct{}{}
		}
	}
	groups := uniqueInt64(groupIDs)
	if len(groups) > 0 {
		var rows []model.TunnelGroupTunnel
		if err := h.repo.DB().Where("tunnel_group_id IN ?", groups).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.TunnelID > 0 {
				seen[row.TunnelID] = struct{}{}
			}
		}
	}
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func normalizedPaymentStatus(order model.CommerceOrder) string {
	if strings.TrimSpace(order.PaymentStatus) != "" {
		return order.PaymentStatus
	}
	switch order.Status {
	case orderStatusActive, orderStatusPaid, orderStatusProvisioning:
		return paymentStatusPaid
	case orderStatusRefunded:
		return paymentStatusRefunded
	default:
		return paymentStatusUnpaid
	}
}

func normalizedFulfillmentStatus(order model.CommerceOrder) string {
	if strings.TrimSpace(order.FulfillmentStatus) != "" {
		return order.FulfillmentStatus
	}
	switch order.Status {
	case orderStatusActive:
		return fulfillmentStatusDone
	case orderStatusProvisioning:
		return fulfillmentStatusProvisioning
	case orderStatusFailed:
		return fulfillmentStatusFailed
	case orderStatusCancelled:
		return fulfillmentStatusCancelled
	default:
		return fulfillmentStatusPending
	}
}

func normalizedRefundStatus(order model.CommerceOrder) string {
	if strings.TrimSpace(order.RefundStatus) != "" {
		return order.RefundStatus
	}
	if order.Status == orderStatusRefunded {
		return refundStatusApproved
	}
	return refundStatusNone
}

func subscriptionIDFromSnapshot(snapshot string) int64 {
	if strings.TrimSpace(snapshot) == "" {
		return 0
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(snapshot), &data); err != nil {
		return 0
	}
	switch v := data["subscriptionId"].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case json.Number:
		id, _ := v.Int64()
		return id
	}
	return 0
}

func (h *Handler) pauseUserTunnelsOutsidePlan(userID int64, keepTunnelIDs []int64, now int64) {
	if userID <= 0 {
		return
	}
	query := h.repo.DB().Where("user_id = ? AND status = 1", userID)
	if len(keepTunnelIDs) > 0 {
		query = query.Where("tunnel_id NOT IN ?", keepTunnelIDs)
	}
	var rows []model.UserTunnel
	if err := query.Find(&rows).Error; err != nil {
		return
	}
	for _, row := range rows {
		forwards, err := h.listActiveForwardsByUserTunnel(row.UserID, row.TunnelID)
		if err == nil {
			h.pauseForwardRecords(forwards, now)
		}
		_ = h.repo.DisableUserTunnel(row.ID)
	}
}

func (h *Handler) writeAuditLog(actorID int64, actorName, action, targetType string, targetID int64, summary string, payload interface{}) {
	if h == nil || h.repo == nil {
		return
	}
	raw := ""
	if payload != nil {
		if b, err := json.Marshal(payload); err == nil {
			raw = string(b)
		}
	}
	_ = h.repo.DB().Create(&model.AuditLog{
		ActorID: actorID, ActorName: strings.TrimSpace(actorName), Action: action, TargetType: targetType,
		TargetID: targetID, Summary: summary, Payload: raw, CreatedTime: time.Now().UnixMilli(),
	}).Error
}

func (h *Handler) createNotification(userID int64, title, content, level string) {
	if h == nil || h.repo == nil || userID <= 0 {
		return
	}
	if strings.TrimSpace(level) == "" {
		level = "info"
	}
	_ = h.repo.DB().Create(&model.Notification{
		UserID: userID, Title: strings.TrimSpace(title), Content: strings.TrimSpace(content), Level: level,
		CreatedTime: time.Now().UnixMilli(),
	}).Error
}

func (h *Handler) enqueueCommerceResourceJob(jobType string, userID, orderID int64, reason string, payload interface{}, now int64) {
	if h == nil || h.repo == nil || strings.TrimSpace(jobType) == "" {
		return
	}
	raw := ""
	if payload != nil {
		if b, err := json.Marshal(payload); err == nil {
			raw = string(b)
		}
	}
	var existing model.CommerceResourceJob
	err := h.repo.DB().Where("job_type = ? AND user_id = ? AND order_id = ? AND status IN ?", jobType, userID, orderID, []string{"pending", "running", "failed"}).
		Order("id DESC").First(&existing).Error
	if err == nil && existing.Status != "done" && existing.Attempts < existing.MaxAttempts {
		_ = h.repo.DB().Model(&model.CommerceResourceJob{}).Where("id = ?", existing.ID).Updates(map[string]interface{}{
			"status": "pending", "next_run_at": now, "last_error": strings.TrimSpace(reason), "payload": raw, "updated_time": now,
		}).Error
		return
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return
	}
	_ = h.repo.DB().Create(&model.CommerceResourceJob{
		JobType: strings.TrimSpace(jobType), UserID: userID, OrderID: orderID, Status: "pending",
		Attempts: 0, MaxAttempts: 5, NextRunAt: now, LastError: strings.TrimSpace(reason),
		Payload: raw, CreatedTime: now, UpdatedTime: now,
	}).Error
}

func (h *Handler) runCommerceResourceJobs(now time.Time) {
	if h == nil || h.repo == nil {
		return
	}
	nowMs := now.UnixMilli()
	var rows []model.CommerceResourceJob
	err := h.repo.DB().
		Where("status IN ? AND attempts < max_attempts AND next_run_at <= ?", []string{"pending", "failed"}, nowMs).
		Order("next_run_at ASC, id ASC").
		Limit(20).
		Find(&rows).Error
	if err != nil {
		return
	}
	for _, job := range rows {
		_ = h.processCommerceResourceJob(job, nowMs)
	}
}

func (h *Handler) processCommerceResourceJob(job model.CommerceResourceJob, now int64) error {
	if job.ID <= 0 {
		return fmt.Errorf("资源任务不存在")
	}
	if err := h.repo.DB().Model(&model.CommerceResourceJob{}).
		Where("id = ? AND status IN ?", job.ID, []string{"pending", "failed", "running"}).
		Updates(map[string]interface{}{"status": "running", "attempts": job.Attempts + 1, "updated_time": now}).Error; err != nil {
		return err
	}
	err := h.executeCommerceResourceJob(job)
	if err == nil {
		if updateErr := h.repo.DB().Model(&model.CommerceResourceJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
			"status": "done", "last_error": "", "updated_time": now, "finished_time": now,
		}).Error; updateErr != nil {
			return updateErr
		}
		h.writeAuditLog(0, "", "resource_job.done", "commerce_resource_job", job.ID, fmt.Sprintf("资源任务 #%d 执行成功", job.ID), map[string]interface{}{"jobType": job.JobType, "userId": job.UserID, "orderId": job.OrderID})
		return nil
	}
	attempts := job.Attempts + 1
	status := "failed"
	nextRunAt := now + int64((time.Duration(attempts)*time.Duration(attempts)*time.Minute)/time.Millisecond)
	if attempts >= job.MaxAttempts {
		nextRunAt = 0
	}
	_ = h.repo.DB().Model(&model.CommerceResourceJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
		"status": status, "attempts": attempts, "next_run_at": nextRunAt, "last_error": err.Error(), "updated_time": now,
	}).Error
	h.writeAuditLog(0, "", "resource_job.failed", "commerce_resource_job", job.ID, err.Error(), map[string]interface{}{"jobType": job.JobType, "userId": job.UserID, "orderId": job.OrderID, "attempts": attempts})
	return err
}

func (h *Handler) executeCommerceResourceJob(job model.CommerceResourceJob) error {
	switch job.JobType {
	case "provision_order":
		if job.OrderID <= 0 {
			return fmt.Errorf("资源发放任务缺少订单ID")
		}
		var order model.CommerceOrder
		if err := h.repo.DB().Where("id = ?", job.OrderID).First(&order).Error; err != nil {
			return err
		}
		if normalizedPaymentStatus(order) != paymentStatusPaid {
			return fmt.Errorf("订单未支付，不能执行资源发放")
		}
		if normalizedFulfillmentStatus(order) == fulfillmentStatusDone && order.Status == orderStatusActive {
			return nil
		}
		return h.provisionOrder(job.OrderID)
	case "sync_user_resources":
		if job.UserID <= 0 {
			return fmt.Errorf("资源同步任务缺少用户ID")
		}
		return h.syncUserPackageResources(job.UserID, time.Now().UnixMilli())
	default:
		return fmt.Errorf("未知资源任务类型：%s", job.JobType)
	}
}

func (h *Handler) applyCoupon(code string, plan model.Plan, amountCents int64, userID int64, now int64) (int64, model.Coupon, error) {
	var coupon model.Coupon
	if err := h.repo.DB().Where("code = ?", strings.ToUpper(strings.TrimSpace(code))).First(&coupon).Error; err != nil {
		return 0, coupon, fmt.Errorf("优惠码无效")
	}
	if coupon.Status != 1 {
		return 0, coupon, fmt.Errorf("优惠码已停用")
	}
	if coupon.ExpTime > 0 && coupon.ExpTime < now {
		return 0, coupon, fmt.Errorf("优惠码已过期")
	}
	if coupon.PlanID > 0 && coupon.PlanID != plan.ID {
		return 0, coupon, fmt.Errorf("优惠码不适用于当前套餐")
	}
	if strings.TrimSpace(coupon.Category) != "" && strings.TrimSpace(coupon.Category) != strings.TrimSpace(plan.Category) {
		return 0, coupon, fmt.Errorf("优惠码不适用于当前套餐分类")
	}
	if coupon.MinAmountCents > 0 && amountCents < coupon.MinAmountCents {
		return 0, coupon, fmt.Errorf("订单金额未达到优惠码最低消费")
	}
	if coupon.MaxUses > 0 && coupon.UsedCount >= coupon.MaxUses {
		return 0, coupon, fmt.Errorf("优惠码已用完")
	}
	if coupon.PerUserLimit > 0 && userID > 0 {
		var used int64
		if err := h.repo.DB().Model(&model.CouponUsage{}).Where("coupon_id = ? AND user_id = ?", coupon.ID, userID).Count(&used).Error; err != nil {
			return 0, coupon, err
		}
		if used >= int64(coupon.PerUserLimit) {
			return 0, coupon, fmt.Errorf("该优惠码已达到当前用户使用上限")
		}
	}
	discount := int64(0)
	switch coupon.DiscountType {
	case "percent":
		discount = amountCents * coupon.DiscountValue / 100
	default:
		discount = coupon.DiscountValue
	}
	if discount < 0 {
		discount = 0
	}
	if discount > amountCents {
		discount = amountCents
	}
	return discount, coupon, nil
}

type couponSnapshotData struct {
	CouponID      int64  `json:"couponId"`
	CouponCode    string `json:"couponCode"`
	DiscountCents int64  `json:"discountCents"`
}

func couponSnapshotFromOrder(order model.CommerceOrder) (couponSnapshotData, bool) {
	if strings.TrimSpace(order.Snapshot) == "" {
		return couponSnapshotData{}, false
	}
	var snap struct {
		Coupon couponSnapshotData `json:"coupon"`
	}
	if err := json.Unmarshal([]byte(order.Snapshot), &snap); err != nil {
		return couponSnapshotData{}, false
	}
	if snap.Coupon.CouponID <= 0 || snap.Coupon.DiscountCents <= 0 {
		return couponSnapshotData{}, false
	}
	return snap.Coupon, true
}

func (h *Handler) reserveCouponForOrder(order model.CommerceOrder) error {
	coupon, ok := couponSnapshotFromOrder(order)
	if !ok {
		return nil
	}
	now := time.Now().UnixMilli()
	return h.repo.DB().Transaction(func(tx *gorm.DB) error {
		var row model.Coupon
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", coupon.CouponID).First(&row).Error; err != nil {
			return err
		}
		if row.Status != 1 {
			return fmt.Errorf("优惠码已停用")
		}
		if row.ExpTime > 0 && row.ExpTime < now {
			return fmt.Errorf("优惠码已过期")
		}
		if row.MaxUses > 0 && row.UsedCount >= row.MaxUses {
			return fmt.Errorf("优惠码已用完")
		}
		if row.PerUserLimit > 0 {
			var used int64
			if err := tx.Model(&model.CouponUsage{}).Where("coupon_id = ? AND user_id = ?", row.ID, order.UserID).Count(&used).Error; err != nil {
				return err
			}
			if used >= int64(row.PerUserLimit) {
				return fmt.Errorf("该优惠码已达到当前用户使用上限")
			}
		}
		usage := model.CouponUsage{
			CouponID: coupon.CouponID, OrderID: order.ID, OrderNo: order.OrderNo,
			UserID: order.UserID, DiscountCents: coupon.DiscountCents, CreatedTime: now,
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&usage)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		update := tx.Model(&model.Coupon{}).Where("id = ? AND (max_uses = 0 OR used_count < max_uses)", coupon.CouponID).UpdateColumn("used_count", gorm.Expr("used_count + 1"))
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			return fmt.Errorf("优惠码已用完")
		}
		return nil
	})
}

func (h *Handler) releaseCouponForOrder(order model.CommerceOrder) {
	coupon, ok := couponSnapshotFromOrder(order)
	if !ok {
		return
	}
	_ = h.repo.DB().Transaction(func(tx *gorm.DB) error {
		result := tx.Where("coupon_id = ? AND order_id = ?", coupon.CouponID, order.ID).Delete(&model.CouponUsage{})
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		return tx.Model(&model.Coupon{}).
			Where("id = ? AND used_count > 0", coupon.CouponID).
			UpdateColumn("used_count", gorm.Expr("used_count - 1")).Error
	})
}

func (h *Handler) consumeCouponForOrder(order model.CommerceOrder) {
	coupon, ok := couponSnapshotFromOrder(order)
	if !ok {
		return
	}
	var existing int64
	if err := h.repo.DB().Model(&model.CouponUsage{}).Where("coupon_id = ? AND order_id = ?", coupon.CouponID, order.ID).Count(&existing).Error; err != nil || existing > 0 {
		return
	}
	_ = h.reserveCouponForOrder(order)
}

func (h *Handler) findUsableInvite(code string, now int64) (*model.InviteCode, error) {
	var invite model.InviteCode
	if err := h.repo.DB().Where("code = ?", code).First(&invite).Error; err != nil {
		return nil, fmt.Errorf("邀请码无效")
	}
	if invite.Status != 1 {
		return nil, fmt.Errorf("邀请码已停用")
	}
	if invite.ExpTime > 0 && invite.ExpTime < now {
		return nil, fmt.Errorf("邀请码已过期")
	}
	if invite.MaxUses > 0 && invite.UsedCount >= invite.MaxUses {
		return nil, fmt.Errorf("邀请码已用完")
	}
	return &invite, nil
}

func (h *Handler) buildEpayURL(order model.CommerceOrder, plan model.Plan, payType string) (string, error) {
	if !h.boolConfig("epay_enabled", false) {
		return "", fmt.Errorf("e支付未启用")
	}
	gateway := strings.TrimRight(h.configValue("epay_gateway", epayDefaultGateway), "/")
	submitURL := strings.TrimSpace(h.configValue("epay_submit_url", ""))
	pid := h.configValue("epay_pid", "")
	key := h.configValue("epay_key", "")
	if gateway == "" || pid == "" || key == "" {
		return "", fmt.Errorf("e支付配置不完整")
	}
	submitURL = epaySubmitURL(gateway, submitURL)
	if submitURL == "" {
		return "", fmt.Errorf("e支付提交地址未配置")
	}
	notifyURL := h.configValue("epay_notify_url", "")
	returnURL := h.configValue("epay_return_url", "")
	if strings.TrimSpace(notifyURL) == "" || strings.TrimSpace(returnURL) == "" {
		return "", fmt.Errorf("e支付通知地址未配置")
	}
	if payType == "" {
		payType = "alipay"
	}
	payType = strings.ToLower(strings.TrimSpace(payType))
	if payType != "alipay" && payType != "wxpay" && payType != "qqpay" {
		payType = "alipay"
	}
	v := url.Values{}
	v.Set("pid", pid)
	v.Set("type", payType)
	v.Set("out_trade_no", order.OrderNo)
	v.Set("notify_url", notifyURL)
	v.Set("return_url", returnURL)
	v.Set("name", plan.Name)
	v.Set("money", fmt.Sprintf("%.2f", float64(order.AmountCents)/100))
	v.Set("sign", epaySign(v, key))
	v.Set("sign_type", "MD5")
	sep := "?"
	if strings.Contains(submitURL, "?") {
		sep = "&"
	}
	return submitURL + sep + v.Encode(), nil
}

func (h *Handler) buildPaymentURL(order model.CommerceOrder, plan model.Plan, provider, payType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case paymentProviderEpusdt:
		return h.buildEpusdtURL(order, plan)
	case "", paymentProviderEpay:
		return h.buildEpayURL(order, plan, payType)
	default:
		return "", fmt.Errorf("不支持的支付方式")
	}
}

func paymentProviderFromType(payType, currentProvider string) string {
	normalizedType := strings.ToLower(strings.TrimSpace(payType))
	if normalizedType == "usdt" || normalizedType == "epusdt" || normalizedType == "u" {
		return paymentProviderEpusdt
	}
	normalizedProvider := strings.ToLower(strings.TrimSpace(currentProvider))
	if normalizedProvider == paymentProviderEpusdt {
		return paymentProviderEpusdt
	}
	return paymentProviderEpay
}

func (h *Handler) prepareOrderPaymentProvider(order *model.CommerceOrder, provider string) error {
	if order == nil || order.ID <= 0 {
		return fmt.Errorf("订单不存在")
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		provider = paymentProviderEpay
	}
	currentProvider := strings.TrimSpace(order.PaymentProvider)
	currentTradeNo := strings.TrimSpace(order.ProviderTradeNo)
	if currentTradeNo != "" && currentProvider != "" && currentProvider != provider {
		return fmt.Errorf("订单已绑定其他支付方式")
	}
	if currentTradeNo == "" && currentProvider != provider {
		if err := h.repo.DB().Model(&model.CommerceOrder{}).Where("id = ?", order.ID).Update("payment_provider", provider).Error; err != nil {
			return err
		}
		order.PaymentProvider = provider
	}
	return nil
}

func (h *Handler) buildEpusdtURL(order model.CommerceOrder, plan model.Plan) (string, error) {
	if !h.boolConfig("usdt_enabled", false) {
		return "", fmt.Errorf("U支付未启用")
	}
	apiBase := strings.TrimRight(strings.TrimSpace(h.configValue("usdt_api_base", "")), "/")
	pid := strings.TrimSpace(h.configValue("usdt_pid", ""))
	key := strings.TrimSpace(h.configValue("usdt_secret_key", ""))
	if apiBase == "" || pid == "" || key == "" {
		return "", fmt.Errorf("U支付配置不完整")
	}
	if tradeNo := strings.TrimSpace(order.ProviderTradeNo); tradeNo != "" {
		return apiBase + "/pay/checkout-counter/" + url.PathEscape(tradeNo), nil
	}
	notifyURL := strings.TrimSpace(h.configValue("usdt_notify_url", ""))
	returnURL := strings.TrimSpace(h.configValue("usdt_return_url", ""))
	if notifyURL == "" || returnURL == "" {
		return "", fmt.Errorf("U支付通知地址未配置")
	}
	currency := strings.ToLower(strings.TrimSpace(h.configValue("usdt_currency", order.Currency)))
	if currency == "" {
		currency = "cny"
	}
	token := strings.ToLower(strings.TrimSpace(h.configValue("usdt_token", "usdt")))
	if token == "" {
		token = "usdt"
	}
	network := strings.ToLower(strings.TrimSpace(h.configValue("usdt_network", "tron")))
	if network == "" {
		network = "tron"
	}
	amount := float64(order.AmountCents) / 100
	params := map[string]interface{}{
		"pid":          pid,
		"order_id":     order.OrderNo,
		"currency":     currency,
		"token":        token,
		"network":      network,
		"amount":       amount,
		"notify_url":   notifyURL,
		"redirect_url": returnURL,
		"name":         plan.Name,
		"payment_type": "GMPay",
	}
	signature, err := epusdtSign(params, key)
	if err != nil {
		return "", err
	}
	params["signature"] = signature

	body, _ := json.Marshal(params)
	req, err := http.NewRequest(http.MethodPost, apiBase+"/payments/gmpay/v1/order/create-transaction", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("U支付请求失败：%w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("读取U支付响应失败：%w", err)
	}
	var parsed epusdtCreateResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("U支付响应格式错误")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !parsed.Success() {
		msg := parsed.MessageText()
		if msg == "" {
			msg = strings.TrimSpace(string(respBody))
		}
		if msg == "" {
			msg = resp.Status
		}
		return "", fmt.Errorf("U支付建单失败：%s", msg)
	}
	payURL := strings.TrimSpace(parsed.PaymentURLValue())
	tradeID := strings.TrimSpace(parsed.TradeIDValue())
	if payURL == "" {
		return "", fmt.Errorf("U支付未返回支付链接")
	}
	updates := map[string]interface{}{"payment_provider": paymentProviderEpusdt, "updated_time": time.Now().UnixMilli()}
	if tradeID != "" && strings.TrimSpace(order.ProviderTradeNo) == "" {
		updates["provider_trade_no"] = tradeID
	}
	_ = h.repo.DB().Model(&model.CommerceOrder{}).Where("id = ? AND provider_trade_no = ''", order.ID).Updates(updates).Error
	return payURL, nil
}

func epaySubmitURL(gateway, configuredSubmitURL string) string {
	if strings.TrimSpace(configuredSubmitURL) != "" {
		return strings.TrimSpace(configuredSubmitURL)
	}
	base := strings.TrimRight(strings.TrimSpace(gateway), "/")
	if base == "" {
		return epayDefaultSubmitURL
	}
	if strings.HasSuffix(strings.ToLower(base), "/submit.php") {
		return base
	}
	return base + "/submit.php"
}

func (h *Handler) boolConfig(name string, def bool) bool {
	v := strings.ToLower(h.configValue(name, ""))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func (h *Handler) int64Config(name string, def int64) int64 {
	v := strings.TrimSpace(h.configValue(name, ""))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func (h *Handler) configValue(name, def string) string {
	cfg, err := h.repo.GetConfigByName(name)
	if err != nil || cfg == nil {
		return def
	}
	return cfg.Value
}

func verifyEpaySign(values url.Values, key string) bool {
	return strings.EqualFold(values.Get("sign"), epaySign(values, key))
}

func verifyEpusdtSign(values map[string]interface{}, key string) bool {
	got := epusdtStringValue(values["signature"])
	if got == "" {
		return false
	}
	expected, err := epusdtSign(values, key)
	return err == nil && strings.EqualFold(got, expected)
}

func epusdtSign(values map[string]interface{}, key string) (string, error) {
	keys := make([]string, 0, len(values))
	stringValues := make(map[string]string, len(values))
	for k, v := range values {
		if k == "signature" {
			continue
		}
		s, err := epusdtSignValue(v)
		if err != nil {
			return "", err
		}
		if s == "" {
			continue
		}
		keys = append(keys, k)
		stringValues[k] = s
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+stringValues[k])
	}
	sum := md5.Sum([]byte(strings.Join(parts, "&") + key))
	return hex.EncodeToString(sum[:]), nil
}

func epusdtSignValue(v interface{}) (string, error) {
	switch value := v.(type) {
	case nil:
		return "", nil
	case string:
		return value, nil
	case json.Number:
		return value.String(), nil
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64), nil
	case float32:
		return strconv.FormatFloat(float64(value), 'f', -1, 64), nil
	case int:
		return strconv.Itoa(value), nil
	case int64:
		return strconv.FormatInt(value, 10), nil
	case int32:
		return strconv.FormatInt(int64(value), 10), nil
	case uint:
		return strconv.FormatUint(uint64(value), 10), nil
	case uint64:
		return strconv.FormatUint(value, 10), nil
	case uint32:
		return strconv.FormatUint(uint64(value), 10), nil
	case []byte:
		return string(value), nil
	default:
		return "", fmt.Errorf("U支付签名字段类型不支持")
	}
}

func epusdtStringValue(v interface{}) string {
	s, err := epusdtSignValue(v)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

func epusdtStatusPaid(v interface{}) bool {
	raw := strings.ToLower(epusdtStringValue(v))
	return raw == "2" || raw == "paid" || raw == "success" || raw == "trade_success"
}

func epusdtAmountCents(v interface{}) (int64, error) {
	raw := epusdtStringValue(v)
	if raw == "" {
		return 0, fmt.Errorf("invalid amount")
	}
	return parseMoneyCents(raw)
}

func epaySign(values url.Values, key string) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		if k == "sign" || k == "sign_type" {
			continue
		}
		if values.Get(k) == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+values.Get(k))
	}
	sum := md5.Sum([]byte(strings.Join(parts, "&") + key))
	return hex.EncodeToString(sum[:])
}

func parseMoneyCents(raw string) (int64, error) {
	parts := strings.SplitN(strings.TrimSpace(raw), ".", 2)
	if len(parts) == 0 || parts[0] == "" {
		return 0, fmt.Errorf("invalid money")
	}
	yuan, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	cents := int64(0)
	if len(parts) > 1 {
		frac := parts[1]
		if len(frac) > 2 {
			frac = frac[:2]
		}
		for len(frac) < 2 {
			frac += "0"
		}
		cents, err = strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, err
		}
	}
	return yuan*100 + cents, nil
}

type epusdtCreateResponse struct {
	StatusCode int    `json:"status_code"`
	Code       int    `json:"code"`
	Message    string `json:"message"`
	Msg        string `json:"msg"`
	Data       struct {
		TradeID    string `json:"trade_id"`
		PaymentURL string `json:"payment_url"`
	} `json:"data"`
	TradeID    string `json:"trade_id"`
	PaymentURL string `json:"payment_url"`
}

func (r epusdtCreateResponse) Success() bool {
	if r.StatusCode != 0 {
		return r.StatusCode == 200
	}
	if r.Code != 0 {
		return r.Code == 200
	}
	return r.PaymentURLValue() != ""
}

func (r epusdtCreateResponse) MessageText() string {
	if strings.TrimSpace(r.Message) != "" {
		return strings.TrimSpace(r.Message)
	}
	return strings.TrimSpace(r.Msg)
}

func (r epusdtCreateResponse) PaymentURLValue() string {
	if strings.TrimSpace(r.Data.PaymentURL) != "" {
		return strings.TrimSpace(r.Data.PaymentURL)
	}
	return strings.TrimSpace(r.PaymentURL)
}

func (r epusdtCreateResponse) TradeIDValue() string {
	if strings.TrimSpace(r.Data.TradeID) != "" {
		return strings.TrimSpace(r.Data.TradeID)
	}
	return strings.TrimSpace(r.TradeID)
}

func uniqueInt64(input []int64) []int64 {
	seen := map[int64]struct{}{}
	out := make([]int64, 0, len(input))
	for _, v := range input {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
