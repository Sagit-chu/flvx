package license

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateKeyWithMachineSendsFingerprintAndMachineScopes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Meta struct {
				Scope map[string]string `json:"scope"`
			} `json:"meta"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Meta.Scope["fingerprint"] != "fingerprint" {
			t.Fatalf("fingerprint scope = %q", body.Meta.Scope["fingerprint"])
		}
		if body.Meta.Scope["machine"] != "machine-id" {
			t.Fatalf("machine scope = %q", body.Meta.Scope["machine"])
		}
		_, _ = fmt.Fprint(w, `{"meta":{"valid":true,"code":"VALID"},"data":{"id":"license-id","attributes":{}}}`)
	}))
	defer server.Close()

	client := NewKeygenClient("account-id", "")
	client.BaseURL = server.URL
	validation, err := client.ValidateKeyWithMachine("license-key", "fingerprint", "machine-id")
	if err != nil || !validation.Meta.Valid {
		t.Fatalf("ValidateKeyWithMachine() validation=%+v err=%v", validation, err)
	}
}

func TestGetMachineIDRetrievesMachineByFingerprint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/machines/fingerprint") {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "License license-key" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = fmt.Fprint(w, `{"data":{"type":"machines","id":"machine-id"}}`)
	}))
	defer server.Close()

	client := NewKeygenClient("account-id", "license-key")
	client.BaseURL = server.URL
	machineID, err := client.GetMachineID("fingerprint")
	if err != nil || machineID != "machine-id" {
		t.Fatalf("GetMachineID() = %q, %v", machineID, err)
	}
}

func TestActivateMachineTreatsFingerprintTakenAsIdempotent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = fmt.Fprint(w, `{"errors":[{"code":"FINGERPRINT_TAKEN"},{"code":"MACHINE_LIMIT_EXCEEDED"}]}`)
	}))
	defer server.Close()

	client := NewKeygenClient("account-id", "license-key")
	client.BaseURL = server.URL

	if machineID, err := client.ActivateMachine("license-id", "fingerprint"); err != nil || machineID != "" {
		t.Fatalf("ActivateMachine() error = %v, want idempotent success", err)
	}
}

func TestActivateMachineReturnsCreatedMachineID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"data":{"type":"machines","id":"machine-id"}}`)
	}))
	defer server.Close()

	client := NewKeygenClient("account-id", "license-key")
	client.BaseURL = server.URL
	machineID, err := client.ActivateMachine("license-id", "fingerprint")
	if err != nil || machineID != "machine-id" {
		t.Fatalf("ActivateMachine() = %q, %v", machineID, err)
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

	_, err := client.ActivateMachine("license-id", "fingerprint")
	if err == nil || !strings.Contains(err.Error(), "MACHINE_LIMIT_EXCEEDED") {
		t.Fatalf("ActivateMachine() error = %v, want machine limit failure", err)
	}
}
