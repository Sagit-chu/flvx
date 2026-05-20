package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go-backend/internal/auth"
	"go-backend/internal/http/middleware"
	"go-backend/internal/http/response"
	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
)

func TestResetFlowOrderUsesCurrentPlanPrice(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	now := time.Now().UnixMilli()
	seedEpayConfig(t, r, map[string]string{
		"epay_enabled":       "true",
		"epay_gateway":       "https://max.xinyuqicheng.cn/plugin/EpayApi/GatewayV1",
		"epay_submit_url":    "https://max.xinyuqicheng.cn/plugin/EpayApi/GatewayV1/submit.php",
		"epay_pid":           "1007",
		"epay_key":           "secret",
		"epay_notify_url":    "https://vluftest.vipmax.shop/api/v1/payment/epay/notify",
		"epay_return_url":    "https://vluftest.vipmax.shop/plans",
		"reset_flow_enabled": "true",
		"reset_flow_name":    "重置套餐流量",
	})
	if err := r.DB().Create(&model.Plan{
		ID: 101, Name: "港日套餐", Category: "默认", PriceCents: 300, ResetFlowPriceCents: 88,
		Currency: "CNY", DurationDays: 30, Flow: 10, Num: 5, CreatedTime: now, UpdatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	if err := r.DB().Create(&model.UserSubscription{
		UserID: 9, PlanID: 101, OrderID: 1, Status: "active",
		StartsAt: now, ExpiresAt: now + int64(24*time.Hour/time.Millisecond), CreatedTime: now, UpdatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	body := strings.NewReader(`{"type":"alipay"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/commerce/subscription/reset-flow", body)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsContextKey, auth.Claims{
		Sub: "9", RoleID: 1, User: "u9", Name: "u9",
	}))
	rec := httptest.NewRecorder()

	h.resetMySubscriptionFlow(rec, req)

	var out response.R
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected code 0, got %d msg=%s", out.Code, out.Msg)
	}
	var order model.CommerceOrder
	if err := r.DB().Where("user_id = ? AND order_type = ?", 9, orderTypeResetFlow).First(&order).Error; err != nil {
		t.Fatalf("find reset order: %v", err)
	}
	if order.AmountCents != 88 {
		t.Fatalf("expected plan reset price 88, got %d", order.AmountCents)
	}
	if !strings.Contains(order.Snapshot, "港日套餐") {
		t.Fatalf("expected snapshot to include current plan name, got %s", order.Snapshot)
	}
}

func TestResetFlowReplacesStalePendingOrderWithCancelledFields(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	now := time.Now().UnixMilli()
	seedEpayConfig(t, r, map[string]string{
		"epay_enabled":       "true",
		"epay_pid":           "1007",
		"epay_key":           "secret",
		"epay_notify_url":    "https://vluftest.vipmax.shop/api/v1/payment/epay/notify",
		"epay_return_url":    "https://vluftest.vipmax.shop/plans",
		"reset_flow_enabled": "true",
		"reset_flow_name":    "重置套餐流量",
	})
	if err := r.DB().Create(&model.Plan{
		ID: 101, Name: "港日套餐", Category: "默认", PriceCents: 300, ResetFlowPriceCents: 88,
		Currency: "CNY", DurationDays: 30, Flow: 10, Num: 5, CreatedTime: now, UpdatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	if err := r.DB().Create(&model.UserSubscription{
		ID: 70, UserID: 9, PlanID: 101, OrderID: 1, Status: "active",
		StartsAt: now, ExpiresAt: now + int64(24*time.Hour/time.Millisecond), CreatedTime: now, UpdatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	stale := model.CommerceOrder{
		ID: 71, OrderNo: "FLVX-STALE-RESET", UserID: 9, PlanID: 101,
		AmountCents: 1, Currency: "CNY", Status: orderStatusPending,
		PaymentStatus: paymentStatusUnpaid, FulfillmentStatus: fulfillmentStatusPending,
		RefundStatus: refundStatusNone, OrderType: orderTypeResetFlow, PaymentProvider: "epay",
		Snapshot:    `{"subscriptionId":70}`,
		CreatedTime: now - 1000, UpdatedTime: now - 1000,
	}
	if err := r.DB().Create(&stale).Error; err != nil {
		t.Fatalf("seed stale order: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/commerce/subscription/reset-flow", strings.NewReader(`{"type":"alipay","subscriptionId":70}`))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsContextKey, auth.Claims{
		Sub: "9", RoleID: 1, User: "u9", Name: "u9",
	}))
	rec := httptest.NewRecorder()

	h.resetMySubscriptionFlow(rec, req)

	var out response.R
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected reset order success, got code=%d msg=%s", out.Code, out.Msg)
	}
	var cancelled model.CommerceOrder
	if err := r.DB().Where("id = ?", stale.ID).First(&cancelled).Error; err != nil {
		t.Fatalf("find stale order: %v", err)
	}
	if cancelled.Status != orderStatusCancelled || cancelled.FulfillmentStatus != fulfillmentStatusCancelled || cancelled.CancelledTime == 0 {
		t.Fatalf("expected stale order fully cancelled, got %#v", cancelled)
	}
	var count int64
	if err := r.DB().Model(&model.CommerceOrder{}).Where("user_id = ? AND plan_id = ? AND order_type = ?", 9, 101, orderTypeResetFlow).Count(&count).Error; err != nil {
		t.Fatalf("count reset orders: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected stale plus replacement reset order, got %d", count)
	}
}

func TestDifferentPlanScopeCreatesAdditionalSubscription(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	now := time.Now().UnixMilli()
	seedEpayConfig(t, r, map[string]string{
		"epay_enabled":    "true",
		"epay_pid":        "1007",
		"epay_key":        "secret",
		"epay_notify_url": "https://vluftest.vipmax.shop/api/v1/payment/epay/notify",
		"epay_return_url": "https://vluftest.vipmax.shop/plans",
	})
	if err := r.DB().Create(&model.User{
		ID: 9, User: "u9", Pwd: "x", RoleID: 1, ExpTime: now + int64(30*24*time.Hour/time.Millisecond),
		Flow: 0, Num: 0, CreatedTime: now, Status: 1,
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	for _, tunnel := range []model.Tunnel{
		{ID: 1, Name: "广港", Type: 1, Protocol: "tls", Flow: 100, CreatedTime: now, UpdatedTime: now, Status: 1, TrafficRatio: 10},
		{ID: 2, Name: "港日", Type: 1, Protocol: "tls", Flow: 100, CreatedTime: now, UpdatedTime: now, Status: 1, TrafficRatio: 10},
	} {
		if err := r.DB().Create(&tunnel).Error; err != nil {
			t.Fatalf("seed tunnel: %v", err)
		}
	}
	for _, plan := range []model.Plan{
		{ID: 101, Name: "广港套餐", Category: "精品线路", PriceCents: 300, Currency: "CNY", DurationDays: 30, Flow: 5, Num: 5, CreatedTime: now, UpdatedTime: now, Status: 1},
		{ID: 102, Name: "港日套餐", Category: "精品线路", PriceCents: 300, Currency: "CNY", DurationDays: 30, Flow: 5, Num: 5, CreatedTime: now, UpdatedTime: now, Status: 1},
	} {
		if err := r.DB().Create(&plan).Error; err != nil {
			t.Fatalf("seed plan: %v", err)
		}
	}
	for _, entitlement := range []model.PlanEntitlement{
		{PlanID: 101, ScopeType: "tunnel", ScopeID: 1, CreatedTime: now},
		{PlanID: 102, ScopeType: "tunnel", ScopeID: 2, CreatedTime: now},
	} {
		if err := r.DB().Create(&entitlement).Error; err != nil {
			t.Fatalf("seed entitlement: %v", err)
		}
	}
	if err := r.DB().Create(&model.UserSubscription{
		UserID: 9, PlanID: 101, OrderID: 1, Status: "active",
		StartsAt: now, ExpiresAt: now + int64(30*24*time.Hour/time.Millisecond), CreatedTime: now, UpdatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/commerce/order/create", strings.NewReader(`{"planId":102,"action":"upgrade"}`))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsContextKey, auth.Claims{
		Sub: "9", RoleID: 1, User: "u9", Name: "u9",
	}))
	rec := httptest.NewRecorder()

	h.createCommerceOrder(rec, req)

	var out response.R
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected code 0, got %d msg=%s", out.Code, out.Msg)
	}
	var order model.CommerceOrder
	if err := r.DB().Where("user_id = ? AND plan_id = ?", 9, 102).First(&order).Error; err != nil {
		t.Fatalf("find order: %v", err)
	}
	if order.OrderType != orderTypeNew {
		t.Fatalf("expected different scope to create new order, got %s", order.OrderType)
	}
	var activeCount int64
	if err := r.DB().Model(&model.UserSubscription{}).Where("user_id = ? AND status = ?", 9, "active").Count(&activeCount).Error; err != nil {
		t.Fatalf("count subscriptions: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("unpaid order should not change active subscriptions, got %d", activeCount)
	}
}

func TestOverlappingPlanScopeRejectsNewOrder(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	now := time.Now().UnixMilli()
	seedEpayConfig(t, r, map[string]string{
		"epay_enabled":    "true",
		"epay_pid":        "1007",
		"epay_key":        "secret",
		"epay_notify_url": "https://vluftest.vipmax.shop/api/v1/payment/epay/notify",
		"epay_return_url": "https://vluftest.vipmax.shop/plans",
	})
	if err := r.DB().Create(&model.User{
		ID: 9, User: "u9", Pwd: "x", RoleID: 1, ExpTime: now + int64(30*24*time.Hour/time.Millisecond),
		Flow: 0, Num: 0, CreatedTime: now, Status: 1,
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	for _, tunnel := range []model.Tunnel{
		{ID: 1, Name: "广港", Type: 1, Protocol: "tls", Flow: 100, CreatedTime: now, UpdatedTime: now, Status: 1, TrafficRatio: 10},
		{ID: 2, Name: "港日", Type: 1, Protocol: "tls", Flow: 100, CreatedTime: now, UpdatedTime: now, Status: 1, TrafficRatio: 10},
	} {
		if err := r.DB().Create(&tunnel).Error; err != nil {
			t.Fatalf("seed tunnel: %v", err)
		}
	}
	for _, plan := range []model.Plan{
		{ID: 101, Name: "广港套餐", Category: "精品线路", PriceCents: 300, Currency: "CNY", DurationDays: 30, Flow: 5, Num: 5, CreatedTime: now, UpdatedTime: now, Status: 1},
		{ID: 102, Name: "混合套餐", Category: "精品线路", PriceCents: 500, Currency: "CNY", DurationDays: 30, Flow: 5, Num: 5, CreatedTime: now, UpdatedTime: now, Status: 1},
	} {
		if err := r.DB().Create(&plan).Error; err != nil {
			t.Fatalf("seed plan: %v", err)
		}
	}
	for _, entitlement := range []model.PlanEntitlement{
		{PlanID: 101, ScopeType: "tunnel", ScopeID: 1, CreatedTime: now},
		{PlanID: 102, ScopeType: "tunnel", ScopeID: 1, CreatedTime: now},
		{PlanID: 102, ScopeType: "tunnel", ScopeID: 2, CreatedTime: now},
	} {
		if err := r.DB().Create(&entitlement).Error; err != nil {
			t.Fatalf("seed entitlement: %v", err)
		}
	}
	if err := r.DB().Create(&model.UserSubscription{
		UserID: 9, PlanID: 101, OrderID: 1, Status: "active",
		StartsAt: now, ExpiresAt: now + int64(30*24*time.Hour/time.Millisecond), CreatedTime: now, UpdatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/commerce/order/create", strings.NewReader(`{"planId":102}`))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsContextKey, auth.Claims{
		Sub: "9", RoleID: 1, User: "u9", Name: "u9",
	}))
	rec := httptest.NewRecorder()

	h.createCommerceOrder(rec, req)

	var out response.R
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code == 0 || !strings.Contains(out.Msg, "相同隧道") {
		t.Fatalf("expected overlap rejection, got code=%d msg=%s", out.Code, out.Msg)
	}
	var count int64
	if err := r.DB().Model(&model.CommerceOrder{}).Where("user_id = ? AND plan_id = ?", 9, 102).Count(&count).Error; err != nil {
		t.Fatalf("count order: %v", err)
	}
	if count != 0 {
		t.Fatalf("overlap order should not be created, got %d", count)
	}
}

func TestCreateCommerceOrderPaysExistingPendingOrderWithBalance(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	now := time.Now().UnixMilli()
	if err := r.DB().Create(&model.User{
		ID: 9, User: "u9", Pwd: "x", RoleID: 1, ExpTime: now + int64(30*24*time.Hour/time.Millisecond),
		Flow: 0, Num: 0, CreatedTime: now, Status: 1,
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := r.DB().Create(&model.UserWallet{
		UserID: 9, BalanceCents: 500, CreatedTime: now, UpdatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed wallet: %v", err)
	}
	if err := r.DB().Create(&model.Tunnel{
		ID: 1, Name: "广港", Type: 1, Protocol: "tls", Flow: 100, CreatedTime: now, UpdatedTime: now, Status: 1, TrafficRatio: 10,
	}).Error; err != nil {
		t.Fatalf("seed tunnel: %v", err)
	}
	if err := r.DB().Create(&model.Plan{
		ID: 101, Name: "广港套餐", Category: "精品线路", PriceCents: 300, Currency: "CNY",
		DurationDays: 30, Flow: 5, Num: 5, CreatedTime: now, UpdatedTime: now, Status: 1,
	}).Error; err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	if err := r.DB().Create(&model.PlanEntitlement{
		PlanID: 101, ScopeType: "tunnel", ScopeID: 1, CreatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed entitlement: %v", err)
	}
	order := model.CommerceOrder{
		ID: 91, OrderNo: "FLVX-BAL-OLD", UserID: 9, PlanID: 101,
		AmountCents: 300, Currency: "CNY", Status: orderStatusPending,
		PaymentStatus: paymentStatusUnpaid, FulfillmentStatus: fulfillmentStatusPending,
		RefundStatus: refundStatusNone, OrderType: orderTypeNew, PaymentProvider: "epay",
		CreatedTime: now, UpdatedTime: now,
	}
	if err := r.DB().Create(&order).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/commerce/order/create", strings.NewReader(`{"planId":101,"type":"balance"}`))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsContextKey, auth.Claims{
		Sub: "9", RoleID: 1, User: "u9", Name: "u9",
	}))
	rec := httptest.NewRecorder()

	h.createCommerceOrder(rec, req)

	var out response.R
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected balance pay success, got code=%d msg=%s", out.Code, out.Msg)
	}
	var got model.CommerceOrder
	if err := r.DB().Where("id = ?", order.ID).First(&got).Error; err != nil {
		t.Fatalf("find order: %v", err)
	}
	if got.PaymentStatus != paymentStatusPaid || got.FulfillmentStatus != fulfillmentStatusDone || got.Status != orderStatusActive {
		t.Fatalf("expected existing order paid and provisioned, got %#v", got)
	}
	if got.PaymentProvider != "balance" || !strings.HasPrefix(got.ProviderTradeNo, "wallet-FLVX-BAL-OLD-") {
		t.Fatalf("expected wallet trade no, got provider=%s trade=%s", got.PaymentProvider, got.ProviderTradeNo)
	}
	var wallet model.UserWallet
	if err := r.DB().Where("user_id = ?", 9).First(&wallet).Error; err != nil {
		t.Fatalf("find wallet: %v", err)
	}
	if wallet.BalanceCents != 200 {
		t.Fatalf("expected wallet balance 200, got %d", wallet.BalanceCents)
	}
	var records int64
	if err := r.DB().Model(&model.PaymentRecord{}).Where("order_id = ? AND provider = ?", order.ID, "balance").Count(&records).Error; err != nil {
		t.Fatalf("count payment records: %v", err)
	}
	if records != 1 {
		t.Fatalf("expected one balance payment record, got %d", records)
	}
}

func TestPayOrderWithBalanceRejectsWalletRechargeOrder(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	now := time.Now().UnixMilli()
	if err := r.DB().Create(&model.User{
		ID: 9, User: "u9", Pwd: "x", RoleID: 1, ExpTime: now + int64(30*24*time.Hour/time.Millisecond),
		Flow: 0, Num: 0, CreatedTime: now, Status: 1,
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := r.DB().Create(&model.UserWallet{
		UserID: 9, BalanceCents: 500, CreatedTime: now, UpdatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed wallet: %v", err)
	}
	order := model.CommerceOrder{
		ID: 92, OrderNo: "FLVX-WALLET-RECHARGE", UserID: 9, PlanID: 0,
		AmountCents: 300, Currency: "CNY", Status: orderStatusPending,
		PaymentStatus: paymentStatusUnpaid, FulfillmentStatus: fulfillmentStatusPending,
		RefundStatus: refundStatusNone, OrderType: orderTypeWalletRecharge, PaymentProvider: "epay",
		CreatedTime: now, UpdatedTime: now,
	}
	if err := r.DB().Create(&order).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/commerce/order/pay-balance", strings.NewReader(`{"id":92}`))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsContextKey, auth.Claims{
		Sub: "9", RoleID: 1, User: "u9", Name: "u9",
	}))
	rec := httptest.NewRecorder()

	h.payCommerceOrderWithBalance(rec, req)

	var out response.R
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code == 0 || !strings.Contains(out.Msg, "余额充值订单不能使用余额支付") {
		t.Fatalf("expected wallet recharge rejection, got code=%d msg=%s", out.Code, out.Msg)
	}
	var got model.CommerceOrder
	if err := r.DB().Where("id = ?", order.ID).First(&got).Error; err != nil {
		t.Fatalf("find order: %v", err)
	}
	if got.PaymentStatus != paymentStatusUnpaid || got.Status != orderStatusPending || got.ProviderTradeNo != "" {
		t.Fatalf("wallet recharge order should stay unpaid, got %#v", got)
	}
	var wallet model.UserWallet
	if err := r.DB().Where("user_id = ?", 9).First(&wallet).Error; err != nil {
		t.Fatalf("find wallet: %v", err)
	}
	if wallet.BalanceCents != 500 {
		t.Fatalf("wallet balance should not change, got %d", wallet.BalanceCents)
	}
	var ledgers int64
	if err := r.DB().Model(&model.WalletLedger{}).Where("ref_type = ? AND ref_id = ?", "commerce_order", order.ID).Count(&ledgers).Error; err != nil {
		t.Fatalf("count wallet ledger: %v", err)
	}
	if ledgers != 0 {
		t.Fatalf("wallet recharge rejection should not write ledger, got %d", ledgers)
	}
}

func TestCancelExpiredPendingCommerceOrdersReleasesCoupon(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	now := time.Now()
	nowMs := now.UnixMilli()
	seedEpayConfig(t, r, map[string]string{
		"pending_order_timeout_minutes": "15",
	})
	if err := r.DB().Create(&model.Coupon{
		ID: 51, Code: "OLDPAY", Name: "超时优惠码", DiscountType: "fixed", DiscountValue: 100,
		MaxUses: 1, UsedCount: 1, Status: 1, CreatedTime: nowMs, UpdatedTime: nowMs,
	}).Error; err != nil {
		t.Fatalf("seed coupon: %v", err)
	}
	order := model.CommerceOrder{
		ID: 61, OrderNo: "FLVX-OLDPAY", UserID: 9, PlanID: 101,
		AmountCents: 200, Currency: "CNY", Status: orderStatusPending,
		PaymentStatus: paymentStatusUnpaid, FulfillmentStatus: fulfillmentStatusPending,
		RefundStatus: refundStatusNone, OrderType: orderTypeNew, PaymentProvider: "epay",
		Snapshot:    `{"coupon":{"couponId":51,"couponCode":"OLDPAY","discountCents":100}}`,
		CreatedTime: now.Add(-20 * time.Minute).UnixMilli(), UpdatedTime: now.Add(-20 * time.Minute).UnixMilli(),
	}
	if err := r.DB().Create(&order).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}
	if err := r.DB().Create(&model.CouponUsage{
		CouponID: 51, OrderID: 61, OrderNo: "FLVX-OLDPAY", UserID: 9, DiscountCents: 100, CreatedTime: order.CreatedTime,
	}).Error; err != nil {
		t.Fatalf("seed coupon usage: %v", err)
	}

	h.cancelExpiredPendingCommerceOrders(now)

	var got model.CommerceOrder
	if err := r.DB().Where("id = ?", order.ID).First(&got).Error; err != nil {
		t.Fatalf("find order: %v", err)
	}
	if got.Status != orderStatusCancelled || got.FulfillmentStatus != fulfillmentStatusCancelled || got.CancelledTime == 0 {
		t.Fatalf("expected cancelled order, got %#v", got)
	}
	var coupon model.Coupon
	if err := r.DB().Where("id = ?", 51).First(&coupon).Error; err != nil {
		t.Fatalf("find coupon: %v", err)
	}
	if coupon.UsedCount != 0 {
		t.Fatalf("expected coupon usage released, got used_count=%d", coupon.UsedCount)
	}
	var usages int64
	if err := r.DB().Model(&model.CouponUsage{}).Where("coupon_id = ?", 51).Count(&usages).Error; err != nil {
		t.Fatalf("count coupon usage: %v", err)
	}
	if usages != 0 {
		t.Fatalf("expected coupon usage row removed, got %d", usages)
	}
}

func TestPaymentURLFailureMarksNewOrderFailedAndReleasesCoupon(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	now := time.Now().UnixMilli()
	seedEpayConfig(t, r, map[string]string{
		"epay_enabled": "true",
		"epay_pid":     "1007",
		"epay_key":     "secret",
	})
	if err := r.DB().Create(&model.User{
		ID: 9, User: "u9", Pwd: "x", RoleID: 1, ExpTime: now + int64(30*24*time.Hour/time.Millisecond),
		Flow: 0, Num: 0, CreatedTime: now, Status: 1,
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := r.DB().Create(&model.Tunnel{
		ID: 1, Name: "广港", Type: 1, Protocol: "tls", Flow: 100, CreatedTime: now, UpdatedTime: now, Status: 1, TrafficRatio: 10,
	}).Error; err != nil {
		t.Fatalf("seed tunnel: %v", err)
	}
	if err := r.DB().Create(&model.Plan{
		ID: 101, Name: "广港套餐", Category: "精品线路", PriceCents: 300, Currency: "CNY",
		DurationDays: 30, Flow: 5, Num: 5, CreatedTime: now, UpdatedTime: now, Status: 1,
	}).Error; err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	if err := r.DB().Create(&model.PlanEntitlement{
		PlanID: 101, ScopeType: "tunnel", ScopeID: 1, CreatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed entitlement: %v", err)
	}
	if err := r.DB().Create(&model.Coupon{
		ID: 51, Code: "PAYFAIL", Name: "失败释放", DiscountType: "fixed", DiscountValue: 100,
		MaxUses: 1, Status: 1, CreatedTime: now, UpdatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed coupon: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/commerce/order/create", strings.NewReader(`{"planId":101,"type":"alipay","couponCode":"PAYFAIL"}`))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsContextKey, auth.Claims{
		Sub: "9", RoleID: 1, User: "u9", Name: "u9",
	}))
	rec := httptest.NewRecorder()

	h.createCommerceOrder(rec, req)

	var out response.R
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code == 0 {
		t.Fatalf("expected payment url build failure")
	}
	var order model.CommerceOrder
	if err := r.DB().Where("user_id = ? AND plan_id = ?", 9, 101).First(&order).Error; err != nil {
		t.Fatalf("find order: %v", err)
	}
	if order.Status != orderStatusFailed || order.FulfillmentStatus != fulfillmentStatusFailed || order.FailureReason == "" {
		t.Fatalf("expected failed order with reason, got %#v", order)
	}
	var coupon model.Coupon
	if err := r.DB().Where("id = ?", 51).First(&coupon).Error; err != nil {
		t.Fatalf("find coupon: %v", err)
	}
	if coupon.UsedCount != 0 {
		t.Fatalf("expected coupon usage released, got used_count=%d", coupon.UsedCount)
	}
	var usages int64
	if err := r.DB().Model(&model.CouponUsage{}).Where("coupon_id = ?", 51).Count(&usages).Error; err != nil {
		t.Fatalf("count coupon usage: %v", err)
	}
	if usages != 0 {
		t.Fatalf("expected no coupon usage rows, got %d", usages)
	}
}

func TestUserRegisterCreatesActiveAccountWithoutExpiry(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	seedEpayConfig(t, r, map[string]string{
		"registration_enabled":         "true",
		"invite_registration_required": "false",
		"captcha_enabled":              "false",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/public/register", strings.NewReader(`{"username":"new_user","password":"secret123"}`))
	rec := httptest.NewRecorder()

	h.userRegister(rec, req)

	var out response.R
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected registration success, got code=%d msg=%s", out.Code, out.Msg)
	}
	var user model.User
	if err := r.DB().Where(`"user" = ?`, "new_user").First(&user).Error; err != nil {
		t.Fatalf("find registered user: %v", err)
	}
	if user.Status != 1 || user.ExpTime != 0 {
		t.Fatalf("expected active registered account without expiry, got status=%d exp=%d", user.Status, user.ExpTime)
	}
	expiredIDs, err := r.ListExpiredActiveUserIDs(time.Now().Add(24 * time.Hour).UnixMilli())
	if err != nil {
		t.Fatalf("list expired active users: %v", err)
	}
	for _, id := range expiredIDs {
		if id == user.ID {
			t.Fatalf("registered account without package should not be expired")
		}
	}
}

func TestUserRegisterRollsBackWhenInviteUsageFails(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	now := time.Now().UnixMilli()
	seedEpayConfig(t, r, map[string]string{
		"registration_enabled":            "true",
		"invite_registration_required":    "true",
		"captcha_enabled":                 "false",
		"public_registration_placeholder": "unused",
	})
	if err := r.DB().Create(&model.InviteCode{
		ID: 77, Code: "INVITE77", MaxUses: 1, UsedCount: 0, Status: 1, CreatedTime: now, UpdatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed invite: %v", err)
	}
	if err := r.DB().Exec(`
		CREATE TRIGGER fail_invite_usage_insert
		BEFORE INSERT ON invite_code_usage
		BEGIN
			SELECT RAISE(FAIL, 'invite usage blocked');
		END;
	`).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/public/register", strings.NewReader(`{"username":"new_user","password":"secret123","inviteCode":"INVITE77"}`))
	rec := httptest.NewRecorder()

	h.userRegister(rec, req)

	var out response.R
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code == 0 {
		t.Fatalf("expected registration failure")
	}
	var users int64
	if err := r.DB().Model(&model.User{}).Where(`"user" = ?`, "new_user").Count(&users).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if users != 0 {
		t.Fatalf("user should be rolled back, got %d", users)
	}
	var invite model.InviteCode
	if err := r.DB().Where("id = ?", 77).First(&invite).Error; err != nil {
		t.Fatalf("find invite: %v", err)
	}
	if invite.UsedCount != 0 {
		t.Fatalf("invite used_count should be rolled back, got %d", invite.UsedCount)
	}
}

func TestAdminSavePlanCouponAndInvitePreserveDisabledStatus(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	now := time.Now().UnixMilli()
	if err := r.DB().Create(&model.Tunnel{
		ID: 1, Name: "广港", Type: 1, Protocol: "tls", Flow: 100, CreatedTime: now, UpdatedTime: now, Status: 1, TrafficRatio: 10,
	}).Error; err != nil {
		t.Fatalf("seed tunnel: %v", err)
	}
	if err := r.DB().Create(&model.Plan{
		ID: 83, Name: "旧套餐", Category: "默认", PriceCents: 300, Currency: "CNY",
		DurationDays: 30, Flow: 100, Num: 5, Status: 1, CreatedTime: now, UpdatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	if err := r.DB().Create(&model.PlanEntitlement{
		PlanID: 83, ScopeType: "tunnel", ScopeID: 1, CreatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed plan entitlement: %v", err)
	}
	if err := r.DB().Create(&model.Coupon{
		ID: 81, Code: "OLDOFF", Name: "旧优惠", DiscountType: "fixed", DiscountValue: 100,
		Status: 1, CreatedTime: now, UpdatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed coupon: %v", err)
	}
	if err := r.DB().Create(&model.InviteCode{
		ID: 82, Code: "OLDINV", MaxUses: 3, UsedCount: 1, Status: 1, CreatedTime: now, UpdatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed invite: %v", err)
	}

	planReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/commerce/plan/save", strings.NewReader(`{
		"id":83,"name":"旧套餐","description":"","category":"默认","priceCents":300,"resetFlowPriceCents":0,
		"currency":"CNY","durationDays":30,"flow":100,"dailyQuotaGB":0,"monthlyQuotaGB":0,
		"num":5,"maxConn":0,"sort":0,"status":0,"tunnelIds":[1],"tunnelGroupIds":[]
	}`))
	planRec := httptest.NewRecorder()
	h.adminSavePlan(planRec, planReq)
	var planResp response.R
	if err := json.NewDecoder(planRec.Body).Decode(&planResp); err != nil {
		t.Fatalf("decode plan response: %v", err)
	}
	if planResp.Code != 0 {
		t.Fatalf("expected plan save success, got code=%d msg=%s", planResp.Code, planResp.Msg)
	}
	var plan model.Plan
	if err := r.DB().Where("id = ?", 83).First(&plan).Error; err != nil {
		t.Fatalf("find plan: %v", err)
	}
	if plan.Status != 0 {
		t.Fatalf("disabled plan should stay disabled, got %d", plan.Status)
	}

	couponReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/commerce/coupon/save", strings.NewReader(`{
		"id":81,"code":"oldoff","name":"旧优惠","discountType":"fixed","discountValue":100,"status":0
	}`))
	couponRec := httptest.NewRecorder()
	h.adminSaveCoupon(couponRec, couponReq)
	var couponResp response.R
	if err := json.NewDecoder(couponRec.Body).Decode(&couponResp); err != nil {
		t.Fatalf("decode coupon response: %v", err)
	}
	if couponResp.Code != 0 {
		t.Fatalf("expected coupon save success, got code=%d msg=%s", couponResp.Code, couponResp.Msg)
	}
	var coupon model.Coupon
	if err := r.DB().Where("id = ?", 81).First(&coupon).Error; err != nil {
		t.Fatalf("find coupon: %v", err)
	}
	if coupon.Status != 0 {
		t.Fatalf("disabled coupon should stay disabled, got %d", coupon.Status)
	}

	inviteReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/commerce/invite/save", strings.NewReader(`{
		"id":82,"code":"OLDINV","maxUses":3,"status":0
	}`))
	inviteRec := httptest.NewRecorder()
	h.adminSaveInvite(inviteRec, inviteReq)
	var inviteResp response.R
	if err := json.NewDecoder(inviteRec.Body).Decode(&inviteResp); err != nil {
		t.Fatalf("decode invite response: %v", err)
	}
	if inviteResp.Code != 0 {
		t.Fatalf("expected invite save success, got code=%d msg=%s", inviteResp.Code, inviteResp.Msg)
	}
	var invite model.InviteCode
	if err := r.DB().Where("id = ?", 82).First(&invite).Error; err != nil {
		t.Fatalf("find invite: %v", err)
	}
	if invite.Status != 0 {
		t.Fatalf("disabled invite should stay disabled, got %d", invite.Status)
	}
	if invite.UsedCount != 1 {
		t.Fatalf("invite used_count should not be reset, got %d", invite.UsedCount)
	}
}

func TestAdminCommerceOrderListIsPaginated(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	now := time.Now().UnixMilli()
	for i := 1; i <= 65; i++ {
		order := model.CommerceOrder{
			OrderNo:           fmt.Sprintf("FLVX-PAGE-%03d", i),
			UserID:            9,
			PlanID:            101,
			AmountCents:       300,
			Currency:          "CNY",
			Status:            orderStatusPending,
			PaymentStatus:     paymentStatusUnpaid,
			FulfillmentStatus: fulfillmentStatusPending,
			RefundStatus:      refundStatusNone,
			OrderType:         orderTypeNew,
			PaymentProvider:   "epay",
			CreatedTime:       now + int64(i),
			UpdatedTime:       now + int64(i),
		}
		if err := r.DB().Create(&order).Error; err != nil {
			t.Fatalf("seed order %d: %v", i, err)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/commerce/order/list", strings.NewReader(`{"page":2,"pageSize":50}`))
	rec := httptest.NewRecorder()
	h.adminListOrders(rec, req)

	var out response.R
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected success, got code=%d msg=%s", out.Code, out.Msg)
	}
	data, ok := out.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected paginated payload, got %#v", out.Data)
	}
	items, ok := data["items"].([]interface{})
	if !ok {
		t.Fatalf("expected items array, got %#v", data["items"])
	}
	if len(items) != 15 {
		t.Fatalf("expected 15 rows on page 2, got %d", len(items))
	}
	if total := int64(data["total"].(float64)); total != 65 {
		t.Fatalf("expected total 65, got %d", total)
	}
}

func TestUserCommerceOrderListIsPaginated(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	now := time.Now().UnixMilli()
	for i := 1; i <= 12; i++ {
		userID := int64(9)
		if i > 7 {
			userID = 10
		}
		order := model.CommerceOrder{
			OrderNo:           fmt.Sprintf("FLVX-MY-%03d", i),
			UserID:            userID,
			PlanID:            101,
			AmountCents:       300,
			Currency:          "CNY",
			Status:            orderStatusPending,
			PaymentStatus:     paymentStatusUnpaid,
			FulfillmentStatus: fulfillmentStatusPending,
			RefundStatus:      refundStatusNone,
			OrderType:         orderTypeNew,
			PaymentProvider:   "epay",
			CreatedTime:       now + int64(i),
			UpdatedTime:       now + int64(i),
		}
		if err := r.DB().Create(&order).Error; err != nil {
			t.Fatalf("seed order %d: %v", i, err)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/commerce/order/list", strings.NewReader(`{"page":2,"pageSize":5}`))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsContextKey, auth.Claims{
		Sub: "9", RoleID: 1, User: "u9", Name: "u9",
	}))
	rec := httptest.NewRecorder()
	h.listMyOrders(rec, req)

	var out response.R
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected success, got code=%d msg=%s", out.Code, out.Msg)
	}
	data, ok := out.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected paginated payload, got %#v", out.Data)
	}
	items, ok := data["items"].([]interface{})
	if !ok {
		t.Fatalf("expected items array, got %#v", data["items"])
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 rows on page 2, got %d", len(items))
	}
	if total := int64(data["total"].(float64)); total != 7 {
		t.Fatalf("expected total 7, got %d", total)
	}
}

func TestAdminSyncUserResourcesClearsInactiveEntitlementsAndWritesAudit(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	now := time.Now().UnixMilli()
	if err := r.DB().Create(&model.User{
		ID: 55, User: "u55", Pwd: "x", RoleID: 1, ExpTime: now + 1000,
		Flow: 100, Num: 3, MaxConn: 2, FlowResetTime: 1,
		CreatedTime: now, Status: 1, PasswordChangedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/commerce/user/resources/sync", strings.NewReader(`{"userId":55}`))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsContextKey, auth.Claims{
		Sub: "1", RoleID: 2, User: "admin", Name: "admin",
	}))
	rec := httptest.NewRecorder()
	h.adminSyncUserResources(rec, req)

	var out response.R
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected success, got code=%d msg=%s", out.Code, out.Msg)
	}
	var user model.User
	if err := r.DB().Where("id = ?", 55).First(&user).Error; err != nil {
		t.Fatalf("find user: %v", err)
	}
	if user.Flow != 0 || user.Num != 0 || user.ExpTime != 0 || user.MaxConn != 0 {
		t.Fatalf("expected resources cleared, got flow=%d num=%d exp=%d maxConn=%d", user.Flow, user.Num, user.ExpTime, user.MaxConn)
	}
	var count int64
	if err := r.DB().Model(&model.AuditLog{}).Where("action = ? AND target_type = ? AND target_id = ?", "resource.sync", "user", int64(55)).Count(&count).Error; err != nil {
		t.Fatalf("count audit logs: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one resource sync audit log, got %d", count)
	}
}

func TestAdminCouponAndInviteListsArePaginated(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	now := time.Now().UnixMilli()
	for i := 1; i <= 12; i++ {
		if err := r.DB().Create(&model.Coupon{
			Code: fmt.Sprintf("PAGE%02d", i), Name: "分页优惠", DiscountType: "fixed", DiscountValue: 100,
			Status: 1, CreatedTime: now + int64(i), UpdatedTime: now + int64(i),
		}).Error; err != nil {
			t.Fatalf("seed coupon %d: %v", i, err)
		}
		if err := r.DB().Create(&model.InviteCode{
			Code: fmt.Sprintf("INV%02d", i), MaxUses: 1, Status: 1, CreatedTime: now + int64(i), UpdatedTime: now + int64(i),
		}).Error; err != nil {
			t.Fatalf("seed invite %d: %v", i, err)
		}
	}

	couponReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/commerce/coupon/list", strings.NewReader(`{"page":2,"pageSize":5}`))
	couponRec := httptest.NewRecorder()
	h.adminListCoupons(couponRec, couponReq)
	var couponOut response.R
	if err := json.NewDecoder(couponRec.Body).Decode(&couponOut); err != nil {
		t.Fatalf("decode coupon response: %v", err)
	}
	if couponOut.Code != 0 {
		t.Fatalf("expected coupon list success, got code=%d msg=%s", couponOut.Code, couponOut.Msg)
	}
	couponData := couponOut.Data.(map[string]interface{})
	if got := len(couponData["items"].([]interface{})); got != 5 {
		t.Fatalf("expected 5 coupons on page 2, got %d", got)
	}
	if total := int64(couponData["total"].(float64)); total != 12 {
		t.Fatalf("expected coupon total 12, got %d", total)
	}

	inviteReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/commerce/invite/list", strings.NewReader(`{"page":3,"pageSize":5}`))
	inviteRec := httptest.NewRecorder()
	h.adminListInvites(inviteRec, inviteReq)
	var inviteOut response.R
	if err := json.NewDecoder(inviteRec.Body).Decode(&inviteOut); err != nil {
		t.Fatalf("decode invite response: %v", err)
	}
	if inviteOut.Code != 0 {
		t.Fatalf("expected invite list success, got code=%d msg=%s", inviteOut.Code, inviteOut.Msg)
	}
	inviteData := inviteOut.Data.(map[string]interface{})
	if got := len(inviteData["items"].([]interface{})); got != 2 {
		t.Fatalf("expected 2 invites on page 3, got %d", got)
	}
	if total := int64(inviteData["total"].(float64)); total != 12 {
		t.Fatalf("expected invite total 12, got %d", total)
	}
}

func TestRunCommerceResourceJobsCompletesSyncJob(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	now := time.Now().UnixMilli()
	if err := r.DB().Create(&model.User{
		ID: 66, User: "u66", Pwd: "x", RoleID: 1, ExpTime: now + 1000,
		Flow: 100, Num: 3, MaxConn: 2, FlowResetTime: 1,
		CreatedTime: now, Status: 1, PasswordChangedAt: now,
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := r.DB().Create(&model.CommerceResourceJob{
		JobType: "sync_user_resources", UserID: 66, Status: "pending",
		MaxAttempts: 5, NextRunAt: now - 1, CreatedTime: now, UpdatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed job: %v", err)
	}

	h.runCommerceResourceJobs(time.UnixMilli(now))

	var job model.CommerceResourceJob
	if err := r.DB().Where("user_id = ?", 66).First(&job).Error; err != nil {
		t.Fatalf("find job: %v", err)
	}
	if job.Status != "done" || job.Attempts != 1 {
		t.Fatalf("expected done job with one attempt, got status=%s attempts=%d", job.Status, job.Attempts)
	}
	var user model.User
	if err := r.DB().Where("id = ?", 66).First(&user).Error; err != nil {
		t.Fatalf("find user: %v", err)
	}
	if user.Flow != 0 || user.Num != 0 || user.ExpTime != 0 || user.MaxConn != 0 {
		t.Fatalf("expected resources cleared, got flow=%d num=%d exp=%d maxConn=%d", user.Flow, user.Num, user.ExpTime, user.MaxConn)
	}
}

func TestAdminDeletePlanDeletesUnusedAndArchivesReferencedPlan(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	now := time.Now().UnixMilli()
	for _, plan := range []model.Plan{
		{ID: 301, Name: "未使用套餐", Category: "默认", PriceCents: 100, Currency: "CNY", DurationDays: 30, Status: 1, CreatedTime: now, UpdatedTime: now},
		{ID: 302, Name: "有订单套餐", Category: "默认", PriceCents: 200, Currency: "CNY", DurationDays: 30, Status: 1, CreatedTime: now, UpdatedTime: now},
	} {
		if err := r.DB().Create(&plan).Error; err != nil {
			t.Fatalf("seed plan %d: %v", plan.ID, err)
		}
		if err := r.DB().Create(&model.PlanEntitlement{PlanID: plan.ID, ScopeType: "tunnel", ScopeID: 1, CreatedTime: now}).Error; err != nil {
			t.Fatalf("seed entitlement %d: %v", plan.ID, err)
		}
	}
	if err := r.DB().Create(&model.CommerceOrder{
		OrderNo: "FLVX-PLAN-REF", UserID: 9, PlanID: 302, AmountCents: 200, Currency: "CNY",
		Status: orderStatusPending, PaymentStatus: paymentStatusUnpaid, FulfillmentStatus: fulfillmentStatusPending,
		RefundStatus: refundStatusNone, OrderType: orderTypeNew, CreatedTime: now, UpdatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}

	for _, id := range []int64{301, 302} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/commerce/plan/delete", strings.NewReader(fmt.Sprintf(`{"id":%d}`, id)))
		rec := httptest.NewRecorder()
		h.adminDeletePlan(rec, req)
		var out response.R
		if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
			t.Fatalf("decode delete response for %d: %v", id, err)
		}
		if out.Code != 0 {
			t.Fatalf("expected delete success for %d, got code=%d msg=%s", id, out.Code, out.Msg)
		}
	}

	var unusedCount int64
	if err := r.DB().Model(&model.Plan{}).Where("id = ?", 301).Count(&unusedCount).Error; err != nil {
		t.Fatalf("count unused plan: %v", err)
	}
	if unusedCount != 0 {
		t.Fatalf("unused plan should be deleted, got count %d", unusedCount)
	}
	var archived model.Plan
	if err := r.DB().Where("id = ?", 302).First(&archived).Error; err != nil {
		t.Fatalf("find archived plan: %v", err)
	}
	if archived.Status != -1 {
		t.Fatalf("referenced plan should be archived with status -1, got %d", archived.Status)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/commerce/plan/list", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.adminListPlans(rec, req)
	var out response.R
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode plan list: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected list success, got code=%d msg=%s", out.Code, out.Msg)
	}
	for _, item := range out.Data.([]interface{}) {
		plan := item.(map[string]interface{})
		if int64(plan["id"].(float64)) == 302 {
			t.Fatalf("archived plan should be hidden from admin list")
		}
	}
}

func TestRequestRefundRequiresReason(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	now := time.Now().UnixMilli()
	if err := r.DB().Create(&model.CommerceOrder{
		ID: 401, OrderNo: "FLVX-REFUND-EMPTY", UserID: 9, PlanID: 101, AmountCents: 300, Currency: "CNY",
		Status: orderStatusActive, PaymentStatus: paymentStatusPaid, FulfillmentStatus: fulfillmentStatusDone,
		RefundStatus: refundStatusNone, OrderType: orderTypeNew, CreatedTime: now, UpdatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/commerce/order/refund", strings.NewReader(`{"id":401,"reason":"   "}`))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsContextKey, auth.Claims{
		Sub: "9", RoleID: 1, User: "u9", Name: "u9",
	}))
	rec := httptest.NewRecorder()
	h.requestOrderRefund(rec, req)

	var out response.R
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode refund response: %v", err)
	}
	if out.Code == 0 {
		t.Fatalf("empty refund reason should be rejected")
	}
	var refunds int64
	if err := r.DB().Model(&model.RefundRequest{}).Where("order_id = ?", 401).Count(&refunds).Error; err != nil {
		t.Fatalf("count refunds: %v", err)
	}
	if refunds != 0 {
		t.Fatalf("expected no refund rows, got %d", refunds)
	}
}

func TestUserCanCloseOwnTicket(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	now := time.Now().UnixMilli()
	if err := r.DB().Create(&model.SupportTicket{
		ID: 501, UserID: 9, Title: "已解决问题", Category: "general", Status: ticketStatusOpen,
		Priority: "normal", CreatedTime: now, UpdatedTime: now,
	}).Error; err != nil {
		t.Fatalf("seed ticket: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/commerce/ticket/close", strings.NewReader(`{"id":501}`))
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsContextKey, auth.Claims{
		Sub: "9", RoleID: 1, User: "u9", Name: "u9",
	}))
	rec := httptest.NewRecorder()
	h.closeMyTicket(rec, req)

	var out response.R
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode close response: %v", err)
	}
	if out.Code != 0 {
		t.Fatalf("expected close success, got code=%d msg=%s", out.Code, out.Msg)
	}
	var ticket model.SupportTicket
	if err := r.DB().Where("id = ?", 501).First(&ticket).Error; err != nil {
		t.Fatalf("find ticket: %v", err)
	}
	if ticket.Status != ticketStatusClosed || ticket.ClosedTime == 0 {
		t.Fatalf("expected closed ticket with closed time, got status=%s closed=%d", ticket.Status, ticket.ClosedTime)
	}
}

func setupCommerceResetFlowTestHandler(t *testing.T) (*repo.Repository, *Handler) {
	t.Helper()
	r, err := repo.Open(t.TempDir() + "/commerce-reset-flow.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
	})
	return r, New(r, "unit-test-secret")
}
