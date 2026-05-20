package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestVerifyCertificateAcceptsSignedActivePayload(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	now := time.Now().UnixMilli()
	cert := Certificate{
		Algorithm: "Ed25519",
		PublicKey: base64.StdEncoding.EncodeToString(publicKey),
		Payload: CertificatePayload{
			LicenseID: 1, KeyPrefix: "FLVX-ABCDE", Product: "FLVX",
			Edition: "commercial-v1", VersionChannel: "stable", Status: "active",
			Features: map[string]interface{}{"commercial": true}, InstanceID: "instance-1",
			IssuedAt: now, NextHeartbeatAt: now + HeartbeatIntervalMs,
			GraceUntil: now + GracePeriodMs, ActivationStatus: "active",
		},
	}
	payload, err := json.Marshal(cert.Payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	cert.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	raw, err := MarshalCertificate(cert)
	if err != nil {
		t.Fatalf("marshal cert: %v", err)
	}
	result, _ := VerifyCertificate(raw, cert.PublicKey, now)
	if !result.Valid || result.State != "active" {
		t.Fatalf("expected active certificate, got %+v", result)
	}
}

func TestVerifyCertificateRejectsExpiredGrace(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	now := time.Now().UnixMilli()
	cert := Certificate{
		Algorithm: "Ed25519",
		PublicKey: base64.StdEncoding.EncodeToString(publicKey),
		Payload: CertificatePayload{
			LicenseID: 1, KeyPrefix: "FLVX-ABCDE", Product: "FLVX",
			Edition: "commercial-v1", VersionChannel: "stable", Status: "active",
			Features: map[string]interface{}{"commercial": true}, InstanceID: "instance-1",
			IssuedAt: now - GracePeriodMs, NextHeartbeatAt: now - GracePeriodMs/2,
			GraceUntil: now - 1, ActivationStatus: "active",
		},
	}
	payload, err := json.Marshal(cert.Payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	cert.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	raw, err := MarshalCertificate(cert)
	if err != nil {
		t.Fatalf("marshal cert: %v", err)
	}
	result, _ := VerifyCertificate(raw, cert.PublicKey, now)
	if result.Valid || result.State != "expired_grace" {
		t.Fatalf("expected expired grace certificate, got %+v", result)
	}
}
