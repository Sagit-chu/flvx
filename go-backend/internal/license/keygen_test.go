package license

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestActivateMachineTreatsFingerprintTakenAsIdempotent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = fmt.Fprint(w, `{"errors":[{"code":"FINGERPRINT_TAKEN"},{"code":"MACHINE_LIMIT_EXCEEDED"}]}`)
	}))
	defer server.Close()

	client := NewKeygenClient("account-id", "license-key")
	client.BaseURL = server.URL

	if err := client.ActivateMachine("license-id", "fingerprint"); err != nil {
		t.Fatalf("ActivateMachine() error = %v, want idempotent success", err)
	}
}

func TestActivateMachineRejectsMachineLimitWithoutFingerprintTaken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = fmt.Fprint(w, `{"errors":[{"code":"MACHINE_LIMIT_EXCEEDED"}]}`)
	}))
	defer server.Close()

	client := NewKeygenClient("account-id", "license-key")
	client.BaseURL = server.URL

	err := client.ActivateMachine("license-id", "fingerprint")
	if err == nil || !strings.Contains(err.Error(), "MACHINE_LIMIT_EXCEEDED") {
		t.Fatalf("ActivateMachine() error = %v, want machine limit failure", err)
	}
}
