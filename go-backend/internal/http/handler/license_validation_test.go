package handler

import (
	"bytes"
	"encoding/json"
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
			_, _ = fmt.Fprint(w, `{"data":{"type":"machines","id":"machine-id"}}`)
		case strings.Contains(req.URL.Path, "/machines/"):
			_, _ = fmt.Fprint(w, `{"data":{"type":"machines","id":"machine-id"}}`)
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

func TestValidateLicenseJobAcceptsExistingMachineActivation(t *testing.T) {
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
			_, _ = fmt.Fprint(w, `{"meta":{"valid":true,"code":"VALID"},"data":{"id":"license-id","attributes":{"expiry":"never"}}}`)
		case strings.HasSuffix(req.URL.Path, "/machines"):
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = fmt.Fprint(w, `{"errors":[{"code":"FINGERPRINT_TAKEN"},{"code":"MACHINE_LIMIT_EXCEEDED"}]}`)
		case strings.Contains(req.URL.Path, "/machines/"):
			_, _ = fmt.Fprint(w, `{"data":{"type":"machines","id":"machine-id"}}`)
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()
	restoreLicenseClientFactory(t, server.URL)

	h := &Handler{repo: r}
	h.validateLicenseJob()

	assertLicenseConfig(t, r, "is_commercial", "true")
	assertLicenseConfig(t, r, "license_expiry", "never")
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
			_, _ = fmt.Fprint(w, `{"data":{"type":"machines","id":"machine-id"}}`)
		case strings.Contains(req.URL.Path, "/machines/"):
			_, _ = fmt.Fprint(w, `{"data":{"type":"machines","id":"machine-id"}}`)
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
	assertLicenseConfig(t, r, "is_commercial", "false")
	for _, name := range []string{"license_key", "license_expiry"} {
		if value, err := r.GetViteConfigValue(name); err == nil || value != "" {
			t.Fatalf("%s should not be persisted, got value=%q err=%v", name, value, err)
		}
	}
}

func TestLicenseActivatePersistsValidatedState(t *testing.T) {
	r := openLicenseTestRepository(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/licenses/actions/validate-key"):
			_, _ = fmt.Fprint(w, `{"meta":{"valid":true,"code":"VALID"},"data":{"id":"license-id","attributes":{"expiry":"2030-01-02T00:00:00.000Z"}}}`)
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

	if !strings.Contains(res.Body.String(), `"code":0`) {
		t.Fatalf("expected activation success, got %s", res.Body.String())
	}
	assertLicenseConfig(t, r, "license_key", "license-secret")
	assertLicenseConfig(t, r, "is_commercial", "true")
	assertLicenseConfig(t, r, "license_expiry", "2030-01-02T00:00:00.000Z")
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

func TestValidateLicenseJobRestoresCommercialStateWhenLicenseRecovers(t *testing.T) {
	r := openLicenseTestRepository(t)
	now := time.Now().UnixMilli()
	seedLicenseConfig(t, r, "license_key", "license-secret", now)
	seedLicenseConfig(t, r, "is_commercial", "false", now)
	seedLicenseConfig(t, r, "machine_fingerprint", "fingerprint", now)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/licenses/actions/validate-key") {
			_, _ = fmt.Fprint(w, `{"meta":{"valid":true,"code":"VALID"},"data":{"id":"license-id","attributes":{"expiry":"never"}}}`)
			return
		}
		http.NotFound(w, req)
	}))
	defer server.Close()
	restoreLicenseClientFactory(t, server.URL)

	h := &Handler{repo: r}
	h.validateLicenseJob()

	assertLicenseConfig(t, r, "is_commercial", "true")
	assertLicenseConfig(t, r, "license_expiry", "never")
}

func TestValidateLicenseJobUsesStoredMachineScope(t *testing.T) {
	r := openLicenseTestRepository(t)
	now := time.Now().UnixMilli()
	seedLicenseConfig(t, r, "license_key", "license-secret", now)
	seedLicenseConfig(t, r, "is_commercial", "true", now)
	seedLicenseConfig(t, r, "machine_fingerprint", "fingerprint", now)
	seedLicenseConfig(t, r, "license_machine_id", "machine-id", now)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !strings.HasSuffix(req.URL.Path, "/licenses/actions/validate-key") {
			t.Fatalf("unexpected request %s %s", req.Method, req.URL.Path)
		}
		var body struct {
			Meta struct {
				Scope map[string]string `json:"scope"`
			} `json:"meta"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Meta.Scope["machine"] != "machine-id" || body.Meta.Scope["fingerprint"] != "fingerprint" {
			t.Fatalf("unexpected validation scope: %+v", body.Meta.Scope)
		}
		_, _ = fmt.Fprint(w, `{"meta":{"valid":true,"code":"VALID"},"data":{"id":"license-id","attributes":{"expiry":"never"}}}`)
	}))
	defer server.Close()
	restoreLicenseClientFactory(t, server.URL)

	h := &Handler{repo: r}
	h.validateLicenseJob()

	assertLicenseConfig(t, r, "is_commercial", "true")
	assertLicenseConfig(t, r, "license_machine_id", "machine-id")
}

func TestValidateLicenseJobKeepsStateOnMachineLookupServerFailure(t *testing.T) {
	r := openLicenseTestRepository(t)
	now := time.Now().UnixMilli()
	seedLicenseConfig(t, r, "license_key", "license-secret", now)
	seedLicenseConfig(t, r, "is_commercial", "true", now)
	seedLicenseConfig(t, r, "machine_fingerprint", "fingerprint", now)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/licenses/actions/validate-key"):
			_, _ = fmt.Fprint(w, `{"meta":{"valid":false,"code":"MACHINE_SCOPE_REQUIRED"},"data":{"id":"license-id","attributes":{}}}`)
		case strings.HasSuffix(req.URL.Path, "/machines"):
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = fmt.Fprint(w, `{"errors":[{"code":"FINGERPRINT_TAKEN"}]}`)
		case strings.Contains(req.URL.Path, "/machines/"):
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, `{"errors":[{"code":"SERVICE_UNAVAILABLE"}]}`)
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()
	restoreLicenseClientFactory(t, server.URL)

	h := &Handler{repo: r}
	h.validateLicenseJob()

	assertLicenseConfig(t, r, "is_commercial", "true")
}

func TestValidateLicenseJobKeepsStateOnMachineLookupNotFound(t *testing.T) {
	r := openLicenseTestRepository(t)
	now := time.Now().UnixMilli()
	seedLicenseConfig(t, r, "license_key", "license-secret", now)
	seedLicenseConfig(t, r, "is_commercial", "true", now)
	seedLicenseConfig(t, r, "machine_fingerprint", "fingerprint", now)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/licenses/actions/validate-key"):
			_, _ = fmt.Fprint(w, `{"meta":{"valid":false,"code":"MACHINE_SCOPE_REQUIRED"},"data":{"id":"license-id","attributes":{}}}`)
		case strings.HasSuffix(req.URL.Path, "/machines"):
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = fmt.Fprint(w, `{"errors":[{"code":"FINGERPRINT_TAKEN"}]}`)
		case strings.Contains(req.URL.Path, "/machines/"):
			http.NotFound(w, req)
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()
	restoreLicenseClientFactory(t, server.URL)

	h := &Handler{repo: r}
	h.validateLicenseJob()

	assertLicenseConfig(t, r, "is_commercial", "true")
}

func TestLicenseValidationErrorMessageDoesNotExposeKeygenResponse(t *testing.T) {
	err := &license.APIError{
		Operation:  "activate machine",
		StatusCode: http.StatusUnprocessableEntity,
		Body:       `{"errors":[{"code":"MACHINE_LIMIT_EXCEEDED","detail":"private detail"}]}`,
	}
	message := licenseValidationErrorMessage(err)
	if message != "授权设备数量已达上限" || strings.Contains(message, "private detail") {
		t.Fatalf("unexpected public error message %q", message)
	}
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
	t.Setenv("KEYGEN_ACCOUNT_ID", "account-id")
	previous := newLicenseClient
	newLicenseClient = func(accountID, token string) *license.KeygenClient {
		client := license.NewKeygenClient(accountID, token)
		client.BaseURL = baseURL
		return client
	}
	t.Cleanup(func() { newLicenseClient = previous })
}
