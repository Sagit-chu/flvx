package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
)

func TestBuildEpayURLUsesDocumentSubmitEndpoint(t *testing.T) {
	r, h := setupCommerceEpayTestHandler(t)
	seedEpayConfig(t, r, map[string]string{
		"epay_enabled":    "true",
		"epay_pid":        "1007",
		"epay_key":        "secret",
		"epay_notify_url": "https://vluftest.vipmax.shop/api/v1/payment/epay/notify",
		"epay_return_url": "https://vluftest.vipmax.shop/plans",
	})

	payURL, err := h.buildEpayURL(model.CommerceOrder{
		OrderNo: "FLVX1001", AmountCents: 300,
	}, model.Plan{Name: "不爽", Currency: "CNY"}, "alipay")
	if err != nil {
		t.Fatalf("build epay url: %v", err)
	}

	parsed, err := url.Parse(payURL)
	if err != nil {
		t.Fatalf("parse pay url: %v", err)
	}
	if parsed.String() == "" ||
		parsed.Scheme != "https" ||
		parsed.Host != "max.xinyuqicheng.cn" ||
		parsed.Path != "/plugin/EpayApi/GatewayV1/submit.php" {
		t.Fatalf("unexpected submit endpoint: %s", payURL)
	}
	values := parsed.Query()
	if values.Get("pid") != "1007" ||
		values.Get("type") != "alipay" ||
		values.Get("out_trade_no") != "FLVX1001" ||
		values.Get("name") != "不爽" ||
		values.Get("money") != "3.00" ||
		values.Get("sign_type") != "MD5" {
		t.Fatalf("unexpected payment params: %#v", values)
	}
	if values.Get("sitename") != "" {
		t.Fatalf("sitename is not part of the documented V1 submit API")
	}
	if !verifyEpaySign(values, "secret") {
		t.Fatalf("generated sign should verify")
	}
}

func TestBuildEpayURLRejectsMissingCallbackURLs(t *testing.T) {
	r, h := setupCommerceEpayTestHandler(t)
	seedEpayConfig(t, r, map[string]string{
		"epay_enabled": "true",
		"epay_pid":     "1007",
		"epay_key":     "secret",
	})

	_, err := h.buildEpayURL(model.CommerceOrder{
		OrderNo: "FLVX1001", AmountCents: 300,
	}, model.Plan{Name: "不爽", Currency: "CNY"}, "alipay")
	if err == nil || err.Error() != "e支付通知地址未配置" {
		t.Fatalf("expected missing callback URL error, got %v", err)
	}
}

func TestEpaySignSkipsEmptySignAndSignType(t *testing.T) {
	values := url.Values{}
	values.Set("pid", "1007")
	values.Set("money", "3.00")
	values.Set("out_trade_no", "FLVX1001")
	sign := epaySign(values, "secret")

	values.Set("sign", "ignored")
	values.Set("sign_type", "MD5")
	values.Set("param", "")
	if got := epaySign(values, "secret"); got != sign {
		t.Fatalf("expected sign to skip sign/sign_type/empty values, got %s want %s", got, sign)
	}
}

func TestBuildEpusdtURLCreatesSignedGMPayOrder(t *testing.T) {
	r, h := setupCommerceEpayTestHandler(t)
	var received map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/payments/gmpay/v1/order/create-transaction" {
			t.Fatalf("unexpected epusdt path: %s", req.URL.Path)
		}
		if err := json.NewDecoder(req.Body).Decode(&received); err != nil {
			t.Fatalf("decode epusdt request: %v", err)
		}
		if !verifyEpusdtSign(received, "usdt-secret") {
			t.Fatalf("epusdt request signature mismatch: %#v", received)
		}
		_, _ = w.Write([]byte(`{"status_code":200,"message":"success","data":{"trade_id":"T1001","payment_url":"https://pay.example.com/pay/checkout-counter/T1001"}}`))
	}))
	defer server.Close()
	seedEpayConfig(t, r, map[string]string{
		"usdt_enabled":    "true",
		"usdt_api_base":   server.URL,
		"usdt_pid":        "1007",
		"usdt_secret_key": "usdt-secret",
		"usdt_notify_url": "https://vluftest.vipmax.shop/api/v1/payment/usdt/notify",
		"usdt_return_url": "https://vluftest.vipmax.shop/plans",
		"usdt_currency":   "cny",
		"usdt_token":      "usdt",
		"usdt_network":    "tron",
	})
	order := model.CommerceOrder{
		ID: 77, OrderNo: "FLVX-USDT", AmountCents: 300,
		Status: orderStatusPending, PaymentStatus: paymentStatusUnpaid, FulfillmentStatus: fulfillmentStatusPending,
		RefundStatus: refundStatusNone, PaymentProvider: paymentProviderEpusdt, CreatedTime: time.Now().UnixMilli(), UpdatedTime: time.Now().UnixMilli(),
	}
	if err := r.DB().Create(&order).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}

	payURL, err := h.buildEpusdtURL(order, model.Plan{Name: "USDT套餐", Currency: "CNY"})
	if err != nil {
		t.Fatalf("build epusdt url: %v", err)
	}
	if payURL != "https://pay.example.com/pay/checkout-counter/T1001" {
		t.Fatalf("unexpected pay url: %s", payURL)
	}
	if received["pid"] != "1007" || received["order_id"] != "FLVX-USDT" || received["currency"] != "cny" || received["token"] != "usdt" || received["network"] != "tron" {
		t.Fatalf("unexpected epusdt request: %#v", received)
	}
	if received["payment_type"] != "GMPay" {
		t.Fatalf("expected GMPay payment_type, got %#v", received["payment_type"])
	}
	var latest model.CommerceOrder
	if err := r.DB().Where("id = ?", order.ID).First(&latest).Error; err != nil {
		t.Fatalf("load order: %v", err)
	}
	if latest.PaymentProvider != paymentProviderEpusdt || latest.ProviderTradeNo != "T1001" {
		t.Fatalf("expected epusdt trade id persisted, got provider=%s trade=%s", latest.PaymentProvider, latest.ProviderTradeNo)
	}
}

func TestEpusdtNotifyVerifiesSignatureAmountAndMarksPaid(t *testing.T) {
	r, h := setupCommerceEpayTestHandler(t)
	seedEpayConfig(t, r, map[string]string{
		"usdt_pid":        "1007",
		"usdt_secret_key": "usdt-secret",
	})
	now := time.Now().UnixMilli()
	order := model.CommerceOrder{
		ID: 82, OrderNo: "FLVX-USDT-NOTIFY", UserID: 9, PlanID: 404,
		AmountCents: 300, Currency: "CNY", Status: orderStatusPending,
		PaymentStatus: paymentStatusUnpaid, FulfillmentStatus: fulfillmentStatusPending,
		RefundStatus: refundStatusNone, OrderType: orderTypeNew, PaymentProvider: paymentProviderEpusdt,
		ProviderTradeNo: "T1002", CreatedTime: now, UpdatedTime: now,
	}
	if err := r.DB().Create(&order).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}
	payload := map[string]interface{}{
		"pid":                  "1007",
		"trade_id":             "T1002",
		"order_id":             order.OrderNo,
		"amount":               3.0,
		"actual_amount":        0.42,
		"receive_address":      "TTest",
		"token":                "USDT",
		"block_transaction_id": "block-1",
		"status":               2,
	}
	signature, err := epusdtSign(payload, "usdt-secret")
	if err != nil {
		t.Fatalf("sign notify: %v", err)
	}
	payload["signature"] = signature
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payment/usdt/notify", strings.NewReader(string(body)))
	rr := httptest.NewRecorder()

	h.epusdtNotify(rr, req)

	if rr.Body.String() != "ok" {
		t.Fatalf("expected ok, got %q", rr.Body.String())
	}
	var latest model.CommerceOrder
	if err := r.DB().Where("id = ?", order.ID).First(&latest).Error; err != nil {
		t.Fatalf("load order: %v", err)
	}
	if latest.PaymentStatus != paymentStatusPaid || latest.ProviderTradeNo != "T1002" || latest.FulfillmentStatus != fulfillmentStatusFailed {
		t.Fatalf("unexpected order state after epusdt notify: %#v", latest)
	}
}

func TestNormalizeSupportAttachmentURLRejectsUnsafeSchemesAndPrivateHosts(t *testing.T) {
	valid, err := normalizeSupportAttachmentURL("https://example.com/file.png")
	if err != nil || valid != "https://example.com/file.png" {
		t.Fatalf("expected public https URL to pass, got url=%q err=%v", valid, err)
	}

	for _, raw := range []string{
		"javascript:alert(1)",
		"data:text/html,hello",
		"http://localhost/file.txt",
		"http://127.0.0.1/file.txt",
		"http://192.168.1.10/file.txt",
	} {
		if _, err := normalizeSupportAttachmentURL(raw); err == nil {
			t.Fatalf("expected unsafe attachment URL %q to be rejected", raw)
		}
	}
}

func TestEpayNotifyReturnsSuccessWhenPaymentAcceptedButProvisionQueued(t *testing.T) {
	r, h := setupCommerceEpayTestHandler(t)
	seedEpayConfig(t, r, map[string]string{
		"epay_pid": "1007",
		"epay_key": "secret",
	})
	now := time.Now().UnixMilli()
	order := model.CommerceOrder{
		ID:                81,
		OrderNo:           "FLVX-QUEUE",
		UserID:            9,
		PlanID:            404,
		AmountCents:       300,
		Currency:          "CNY",
		Status:            orderStatusPending,
		PaymentStatus:     paymentStatusUnpaid,
		FulfillmentStatus: fulfillmentStatusPending,
		RefundStatus:      refundStatusNone,
		OrderType:         orderTypeNew,
		CreatedTime:       now,
		UpdatedTime:       now,
	}
	if err := r.DB().Create(&order).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}

	values := url.Values{}
	values.Set("pid", "1007")
	values.Set("trade_status", "TRADE_SUCCESS")
	values.Set("out_trade_no", order.OrderNo)
	values.Set("trade_no", "trade-queued")
	values.Set("money", "3.00")
	values.Set("sign_type", "MD5")
	values.Set("sign", epaySign(values, "secret"))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payment/epay/notify", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	h.epayNotify(rr, req)

	if rr.Body.String() != "success" {
		t.Fatalf("expected provider success after payment accepted, got %q", rr.Body.String())
	}
	var latest model.CommerceOrder
	if err := r.DB().Where("id = ?", order.ID).First(&latest).Error; err != nil {
		t.Fatalf("load order: %v", err)
	}
	if latest.PaymentStatus != paymentStatusPaid || latest.ProviderTradeNo != "trade-queued" || latest.FulfillmentStatus != fulfillmentStatusFailed {
		t.Fatalf("unexpected order state after queued provision failure: %#v", latest)
	}
	var jobs int64
	if err := r.DB().Model(&model.CommerceResourceJob{}).Where("order_id = ? AND job_type = ?", order.ID, "provision_order").Count(&jobs).Error; err != nil {
		t.Fatalf("count resource jobs: %v", err)
	}
	if jobs != 1 {
		t.Fatalf("expected one queued resource job, got %d", jobs)
	}
}

func TestMarkOrderPaidRejectsDifferentTradeNoAfterActive(t *testing.T) {
	r, h := setupCommerceEpayTestHandler(t)
	now := time.Now().UnixMilli()
	order := model.CommerceOrder{
		ID: 91, OrderNo: "FLVX-ACTIVE", UserID: 9, PlanID: 101,
		AmountCents: 300, Currency: "CNY", Status: orderStatusActive,
		PaymentStatus: paymentStatusPaid, FulfillmentStatus: fulfillmentStatusDone,
		RefundStatus: refundStatusNone, OrderType: orderTypeNew, PaymentProvider: "epay",
		ProviderTradeNo: "trade-a", PaidTime: now, ProvisionedTime: now, CreatedTime: now, UpdatedTime: now,
	}
	if err := r.DB().Create(&order).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}

	err := h.markOrderPaidAndProvision(&order, "epay", "trade-b")

	if err == nil {
		t.Fatalf("expected different trade no to be rejected")
	}
	var records int64
	if countErr := r.DB().Model(&model.PaymentRecord{}).Where("provider_trade_no = ?", "trade-b").Count(&records).Error; countErr != nil {
		t.Fatalf("count payment records: %v", countErr)
	}
	if records != 0 {
		t.Fatalf("unexpected payment record for rejected trade no")
	}
}

func TestMarkOrderPaidReconcilesLegacyActiveOrder(t *testing.T) {
	r, h := setupCommerceEpayTestHandler(t)
	now := time.Now().UnixMilli()
	order := model.CommerceOrder{
		ID: 92, OrderNo: "FLVX-LEGACY-ACTIVE", UserID: 9, PlanID: 101,
		AmountCents: 300, Currency: "CNY", Status: orderStatusActive,
		PaymentStatus: paymentStatusUnpaid, FulfillmentStatus: fulfillmentStatusPending,
		RefundStatus: refundStatusNone, OrderType: orderTypeNew, PaymentProvider: "admin-manual",
		PaidTime: now - 1000, ProvisionedTime: now - 1000, CreatedTime: now - 2000, UpdatedTime: now - 1000,
	}
	if err := r.DB().Create(&order).Error; err != nil {
		t.Fatalf("seed order: %v", err)
	}

	err := h.markOrderPaidAndProvision(&order, "admin-manual", "manual-FLVX-LEGACY-ACTIVE")

	if err != nil {
		t.Fatalf("reconcile legacy active order: %v", err)
	}
	var latest model.CommerceOrder
	if err := r.DB().Where("id = ?", order.ID).First(&latest).Error; err != nil {
		t.Fatalf("load order: %v", err)
	}
	if latest.PaymentStatus != paymentStatusPaid || latest.FulfillmentStatus != fulfillmentStatusDone || latest.Status != orderStatusActive {
		t.Fatalf("expected canonical paid/done active order, got %#v", latest)
	}
	if latest.ProviderTradeNo != "manual-FLVX-LEGACY-ACTIVE" {
		t.Fatalf("expected manual trade no backfilled, got %s", latest.ProviderTradeNo)
	}
	var records int64
	if err := r.DB().Model(&model.PaymentRecord{}).Where("provider = ? AND provider_trade_no = ?", "admin-manual", "manual-FLVX-LEGACY-ACTIVE").Count(&records).Error; err != nil {
		t.Fatalf("count payment records: %v", err)
	}
	if records != 1 {
		t.Fatalf("expected one reconciled payment record, got %d", records)
	}
}

func setupCommerceEpayTestHandler(t *testing.T) (*repo.Repository, *Handler) {
	t.Helper()
	r, err := repo.Open(t.TempDir() + "/commerce-epay.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
	})
	return r, New(r, "unit-test-secret")
}

func seedEpayConfig(t *testing.T, r *repo.Repository, values map[string]string) {
	t.Helper()
	for name, value := range values {
		seedConfigValue(t, r, name, value)
	}
}
