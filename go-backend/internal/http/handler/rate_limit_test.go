package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-backend/internal/http/response"
	"go-backend/internal/store/repo"
)

func TestLoginRateLimitBlocksRepeatedAttempts(t *testing.T) {
	r, err := repo.Open(t.TempDir() + "/login-rate-limit.db")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	h := New(r, "unit-test-secret")

	var out response.R
	for i := 0; i < 21; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/login", strings.NewReader(`{"username":"target","password":"badpass"}`))
		req.RemoteAddr = "203.0.113.10:54321"
		rec := httptest.NewRecorder()
		h.login(rec, req)
		out = response.R{}
		if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	if out.Msg != "请求过于频繁，请稍后再试" {
		t.Fatalf("expected rate limit message, got code=%d msg=%s", out.Code, out.Msg)
	}
}

func TestRegisterRateLimitBlocksRepeatedAttempts(t *testing.T) {
	r, h := setupCommerceResetFlowTestHandler(t)
	seedEpayConfig(t, r, map[string]string{
		"registration_enabled":         "true",
		"invite_registration_required": "false",
		"captcha_enabled":              "false",
	})

	var out response.R
	for i := 0; i < 9; i++ {
		body := strings.NewReader(`{"username":"same_user","password":"secret123"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/register", body)
		req.RemoteAddr = "203.0.113.11:54321"
		rec := httptest.NewRecorder()
		h.userRegister(rec, req)
		out = response.R{}
		if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	if out.Msg != "请求过于频繁，请稍后再试" {
		t.Fatalf("expected rate limit message, got code=%d msg=%s", out.Code, out.Msg)
	}
}
