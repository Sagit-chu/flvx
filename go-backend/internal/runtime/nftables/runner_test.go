package nftables

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildApplyCommandStopsAfterValidationFailure(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	applyMarker := filepath.Join(dir, "applied")
	nftPath := filepath.Join(dir, "nft")
	fake := `#!/bin/sh
echo "$*" >> "` + logPath + `"
if [ "$1" = "list" ]; then
  exit 0
fi
if [ "$1" = "-c" ]; then
  exit 1
fi
touch "` + applyMarker + `"
`
	if err := os.WriteFile(nftPath, []byte(fake), 0o755); err != nil {
		t.Fatalf("write fake nft: %v", err)
	}

	command := buildApplyCommand(nftPath, "table inet flvx { }")
	result := exec.Command("sh", "-c", command)
	if err := result.Run(); err == nil {
		t.Fatal("expected validation failure")
	}
	if _, err := os.Stat(applyMarker); !os.IsNotExist(err) {
		t.Fatalf("apply ran after validation failure, stat err=%v", err)
	}
	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake nft calls: %v", err)
	}
	if strings.Count(string(calls), "-f ") != 1 {
		t.Fatalf("expected validation only, got calls:\n%s", calls)
	}
}

func TestBuildCapabilityCheckCommandValidatesRenderedRulesWithoutApplying(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	nftPath := filepath.Join(dir, "nft")
	fake := `#!/bin/sh
echo "$*" >> "` + logPath + `"
if [ "$1" = "--version" ] || [ "$1" = "-c" ]; then
  exit 0
fi
exit 1
`
	if err := os.WriteFile(nftPath, []byte(fake), 0o755); err != nil {
		t.Fatalf("write fake nft: %v", err)
	}

	command := strings.Replace(buildCapabilityCheckCommand(nftPath, "flvx_capability_test"), "command -v nft", "command -v "+nftPath, 1)
	result := exec.Command("sh", "-c", command)
	if output, err := result.CombinedOutput(); err != nil {
		t.Fatalf("capability command failed: %v: %s", err, output)
	}
	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake nft calls: %v", err)
	}
	if strings.Count(string(calls), "-c -f ") != 1 || strings.Contains(string(calls), "\n-f ") {
		t.Fatalf("expected one check-only invocation, got calls:\n%s", calls)
	}
	if !strings.Contains(command, "table inet flvx_capability_test") || !strings.Contains(command, "meta l4proto tcp ct original proto-dst") {
		t.Fatalf("capability check does not contain representative rendered rules:\n%s", command)
	}
}

func TestBuildApplyCommandUsesAtomicReplacementBatch(t *testing.T) {
	dir := t.TempDir()
	batchPath := filepath.Join(dir, "batch.nft")
	nftPath := filepath.Join(dir, "nft")
	fake := `#!/bin/sh
if [ "$1" = "list" ]; then
  exit 0
fi
if [ "$1" = "-f" ]; then
  cp "$2" "` + batchPath + `"
fi
exit 0
`
	if err := os.WriteFile(nftPath, []byte(fake), 0o755); err != nil {
		t.Fatalf("write fake nft: %v", err)
	}

	script := "table inet flvx {\n  chain forward { }\n}"
	result := exec.Command("sh", "-c", buildApplyCommand(nftPath, script))
	if output, err := result.CombinedOutput(); err != nil {
		t.Fatalf("apply command failed: %v: %s", err, output)
	}
	batch, err := os.ReadFile(batchPath)
	if err != nil {
		t.Fatalf("read applied batch: %v", err)
	}
	want := "delete table inet flvx\n" + script + "\n"
	if string(batch) != want {
		t.Fatalf("atomic batch = %q, want %q", batch, want)
	}
}

func TestAuthMethodsDefaultToPrivateKey(t *testing.T) {
	privateKey := mustGeneratePrivateKey(t)
	methods, err := authMethods(SSHConfig{PrivateKey: privateKey})
	if err != nil {
		t.Fatalf("authMethods: %v", err)
	}
	if len(methods) != 1 {
		t.Fatalf("expected 1 auth method, got %d", len(methods))
	}
}

func TestAuthMethodsDefaultPrivateKeyRequiresKey(t *testing.T) {
	_, err := authMethods(SSHConfig{})
	if err == nil || !strings.Contains(err.Error(), "SSH 私钥不能为空") {
		t.Fatalf("expected private key required error, got %v", err)
	}
}

func mustGeneratePrivateKey(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	return string(pem.EncodeToMemory(block))
}
