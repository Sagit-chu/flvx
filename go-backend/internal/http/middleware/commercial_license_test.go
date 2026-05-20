package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCommercialLicenseBlocksProtectedBusinessAPI(t *testing.T) {
	handler := CommercialLicense(func() (bool, string) {
		return false, "授权未激活"
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/node/list", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("response status = %d", rec.Code)
	}
	if body := rec.Body.String(); body == "" || !strings.Contains(body, `"code":451`) {
		t.Fatalf("expected license error body, got %q", body)
	}
}

func TestCommercialLicenseAllowsLicenseRepairAPI(t *testing.T) {
	handler := CommercialLicense(func() (bool, string) {
		return false, "授权未激活"
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/license/local/status", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected pass-through, got %d body=%s", rec.Code, rec.Body.String())
	}
}
