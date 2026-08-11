package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go-backend/internal/license"
	"go-backend/internal/store/repo"
)

func TestValidateLicenseJobRepairsMissingMachineBinding(t *testing.T) {
	r := openLicenseTestRepository(t)
	now := time.Now().UnixMilli()
	seedLicenseConfig(t, r, "license_key", "license-secret", now)
	seedLicenseConfig(t, r, "is_commercial", "true", now)

	var validations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/licenses/actions/validate-key"):
			if validations.Add(1) == 1 {
				_, _ = fmt.Fprint(w, `{"meta":{"valid":false,"code":"NO_MACHINE"},"data":{"id":"license-id","attributes":{}}}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"meta":{"valid":true,"code":"VALID"},"data":{"id":"license-id","attributes":{"expiry":"2030-01-02T00:00:00.000Z"}}}`)
		case strings.HasSuffix(req.URL.Path, "/machines"):
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprint(w, `{}`)
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()
	restoreLicenseClientFactory(t, server.URL)

	h := &Handler{repo: r}
	h.validateLicenseJob()

	assertLicenseConfig(t, r, "is_commercial", "true")
	assertLicenseConfig(t, r, "license_expiry", "2030-01-02T00:00:00.000Z")
	fingerprint, err := r.GetViteConfigValue("machine_fingerprint")
	if err != nil || strings.TrimSpace(fingerprint) == "" {
		t.Fatalf("expected persisted machine fingerprint, got value=%q err=%v", fingerprint, err)
	}
	if got := validations.Load(); got != 2 {
		t.Fatalf("validation calls = %d, want 2", got)
	}
}

func TestLicenseActivateRequiresSuccessfulPostActivationValidation(t *testing.T) {
	r := openLicenseTestRepository(t)

	var validations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/licenses/actions/validate-key"):
			code := "NO_MACHINE"
			if validations.Add(1) > 1 {
				code = "FINGERPRINT_SCOPE_MISMATCH"
			}
			_, _ = fmt.Fprintf(w, `{"meta":{"valid":false,"code":%q},"data":{"id":"license-id","attributes":{}}}`, code)
		case strings.HasSuffix(req.URL.Path, "/machines"):
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprint(w, `{}`)
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()
	restoreLicenseClientFactory(t, server.URL)

	h := &Handler{repo: r}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/license/activate", bytes.NewBufferString(`{"license_key":"license-secret"}`))
	res := httptest.NewRecorder()
	h.licenseActivate(res, req)

	if !strings.Contains(res.Body.String(), "FINGERPRINT_SCOPE_MISMATCH") {
		t.Fatalf("expected post-activation validation failure, got %s", res.Body.String())
	}
	if value, err := r.GetViteConfigValue("is_commercial"); err == nil || value != "" {
		t.Fatalf("commercial status should not be persisted, got value=%q err=%v", value, err)
	}
}

func TestValidateLicenseJobDowngradesWhenMachineBindingIsRejected(t *testing.T) {
	r := openLicenseTestRepository(t)
	now := time.Now().UnixMilli()
	seedLicenseConfig(t, r, "license_key", "license-secret", now)
	seedLicenseConfig(t, r, "is_commercial", "true", now)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/licenses/actions/validate-key"):
			_, _ = fmt.Fprint(w, `{"meta":{"valid":false,"code":"NO_MACHINE"},"data":{"id":"license-id","attributes":{}}}`)
		case strings.HasSuffix(req.URL.Path, "/machines"):
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = fmt.Fprint(w, `{"errors":[{"code":"MACHINE_LIMIT_EXCEEDED"}]}`)
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()
	restoreLicenseClientFactory(t, server.URL)

	h := &Handler{repo: r}
	h.validateLicenseJob()

	assertLicenseConfig(t, r, "is_commercial", "false")
}

func openLicenseTestRepository(t *testing.T) *repo.Repository {
	t.Helper()
	r, err := repo.Open(filepath.Join(t.TempDir(), "license.db"))
	if err != nil {
		t.Fatalf("repo.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func seedLicenseConfig(t *testing.T, r *repo.Repository, name, value string, now int64) {
	t.Helper()
	if err := r.UpsertConfig(name, value, now); err != nil {
		t.Fatalf("UpsertConfig(%q) error = %v", name, err)
	}
}

func assertLicenseConfig(t *testing.T, r *repo.Repository, name, want string) {
	t.Helper()
	got, err := r.GetViteConfigValue(name)
	if err != nil {
		t.Fatalf("GetViteConfigValue(%q) error = %v", name, err)
	}
	if got != want {
		t.Fatalf("config %q = %q, want %q", name, got, want)
	}
}

func restoreLicenseClientFactory(t *testing.T, baseURL string) {
	t.Helper()
	previous := newLicenseClient
	newLicenseClient = func(accountID, token string) *license.KeygenClient {
		client := license.NewKeygenClient(accountID, token)
		client.BaseURL = baseURL
		return client
	}
	t.Cleanup(func() { newLicenseClient = previous })
}
