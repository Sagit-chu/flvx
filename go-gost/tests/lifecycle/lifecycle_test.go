//go:build linux || darwin

package lifecycle_test

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Exercise the real binary: initial parsing is held at a FIFO while the panel
// tries to send a rule as soon as the WebSocket connects. This reproduced the
// old startup overwrite reliably without timing a large config load.
func TestAgentStartupAndFailedReloadPreservePanelRules(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the real agent")
	}
	binary := filepath.Join(t.TempDir(), "gost")
	build := exec.Command("go", "build", "-o", binary, "../..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	dir := t.TempDir()
	early, baseline := address(t), address(t)
	apiAddr := address(t)
	panelConnections := make(chan *websocket.Conn, 2)
	wsReady := make(chan struct{})
	responses := make(chan map[string]any, 8)
	var ready sync.Once
	upgrader := websocket.Upgrader{}
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/system-info" {
			w.Write([]byte("ok"))
			return
		}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		ready.Do(func() { close(wsReady) })
		panelConnections <- c
		if err := c.WriteJSON(map[string]any{"type": "AddService", "requestId": "early-rule", "data": []any{service("70_1_0", early)}}); err != nil {
			return
		}
		for {
			_, payload, err := c.ReadMessage()
			if err != nil {
				return
			}
			env := decodeResponse(t, payload)
			if env["requestId"] == "early-rule" {
				responses <- env
			}
		}
	}))
	defer panel.Close()
	writeJSON(t, filepath.Join(dir, "config.json"), map[string]any{"addr": panel.URL, "secret": "audit-secret", "http": 1, "tls": 1, "socks": 1})
	fifo := filepath.Join(dir, "delayed.json")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "agent.log")
	logfile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer logfile.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	agent := exec.CommandContext(ctx, binary, "-C", fifo)
	agent.Dir, agent.Stdout, agent.Stderr = dir, logfile, logfile
	if err := agent.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- agent.Wait() }()
	defer func() {
		agent.Process.Kill()
		if t.Failed() {
			b, _ := os.ReadFile(logPath)
			t.Logf("agent log:\n%s", b)
		}
	}()
	select {
	case <-wsReady:
		t.Fatal("panel connected before initial config loaded")
	case err := <-exited:
		t.Fatalf("agent exited during startup: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	boot := map[string]any{"services": []any{service("71_1_0", baseline)}, "api": map[string]any{"addr": apiAddr}}
	writeJSON(t, filepath.Join(dir, "gost.json"), boot)
	writeFIFO(t, fifo, boot)
	select {
	case <-wsReady:
	case <-time.After(10 * time.Second):
		t.Fatal("panel did not connect after startup")
	}
	select {
	case response := <-responses:
		if response["success"] != true {
			t.Fatalf("AddService failed: %v", response)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("missing AddService response")
	}
	await(t, "both startup and panel listeners", func() bool { return listening(early) && listening(baseline) })
	saved, err := os.ReadFile(fifo)
	if err != nil || !strings.Contains(string(saved), "70_1_0") {
		t.Fatalf("acknowledged rule not persisted: %v", err)
	}

	// A valid listener plus an invalid handler also exercises rollback of a
	// partially initialized service which already bound the original port.
	invalid := service("71_1_0", baseline)
	invalid["handler"] = map[string]any{"type": "handler-does-not-exist"}
	// The first persisted mutation atomically replaced the FIFO with a regular
	// config file, so subsequent reloads use the same real persistence path.
	writeJSON(t, fifo, map[string]any{"services": []any{invalid}})
	if err := agent.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	await(t, "failed reload rollback", func() bool {
		b, _ := os.ReadFile(logPath)
		return strings.Contains(string(b), "previous config restored")
	})
	if !listening(early) || !listening(baseline) {
		t.Fatal("failed reload lost a previously acknowledged listener")
	}

	// Starting the candidate API on the old service port must not prevent
	// rollback when a later auxiliary listener fails to initialize.
	logBefore, _ := os.ReadFile(logPath)
	writeJSON(t, fifo, map[string]any{
		"services": []any{service("candidate", address(t))},
		"api":      map[string]any{"addr": baseline},
		"metrics":  map[string]any{"addr": "127.0.0.1:not-a-port"},
	})
	if err := agent.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	await(t, "auxiliary listener rollback", func() bool {
		b, _ := os.ReadFile(logPath)
		return strings.Count(string(b), "previous config restored") > strings.Count(string(logBefore), "previous config restored")
	})
	if !listening(early) || !listening(baseline) || !listening(apiAddr) {
		t.Fatal("candidate auxiliary listener prevented rollback")
	}

	// An authenticated API request that never finishes its body must not hold
	// the runtime transaction lock against WS mutations or process shutdown.
	slow, err := net.Dial("tcp", apiAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer slow.Close()
	if _, err = fmt.Fprintf(slow, "POST /config/services HTTP/1.1\r\nHost: localhost\r\nAuthorization: Basic dGVzdDp0ZXN0\r\nContent-Type: application/json\r\nContent-Length: 100000\r\n\r\n{"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	panelConn := <-panelConnections
	if err := panelConn.WriteJSON(map[string]any{"type": "DeleteService", "requestId": "slow-body-check", "data": map[string]any{"services": []string{"70_1_0"}}}); err != nil {
		t.Fatal(err)
	}
	await(t, "WS mutation despite slow API upload", func() bool { return !listening(early) })
	if err := agent.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-exited:
		if err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("agent did not stop")
	}
	if listening(early) || listening(baseline) {
		t.Fatal("shutdown left listeners open")
	}

	// Startup errors must exit without advertising a node ready to accept rules.
	bad := filepath.Join(dir, "invalid.json")
	writeJSON(t, bad, map[string]any{"services": []any{invalid}})
	failed := exec.CommandContext(ctx, binary, "-C", bad)
	failed.Dir = dir
	if output, err := failed.CombinedOutput(); err == nil {
		t.Fatalf("invalid startup succeeded: %s", output)
	}
	select {
	case response := <-responses:
		t.Fatalf("failed startup accepted panel command: %v", response)
	default:
	}
}

func service(name, addr string) map[string]any {
	return map[string]any{"name": name, "addr": addr, "listener": map[string]any{"type": "tcp"}, "handler": map[string]any{"type": "auto"}}
}
func address(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}
func listening(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}
func await(t *testing.T, label string, pred func() bool) {
	t.Helper()
	for until := time.Now().Add(5 * time.Second); time.Now().Before(until); time.Sleep(10 * time.Millisecond) {
		if pred() {
			return
		}
	}
	t.Fatal("timeout: " + label)
}
func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
}
func writeFIFO(t *testing.T, path string, value any) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			done <- err
			return
		}
		_, err = f.Write(b)
		f.Close()
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("agent did not read config FIFO")
	}
}
func decodeResponse(t *testing.T, payload []byte) map[string]any {
	t.Helper()
	env := map[string]any{}
	if err := json.Unmarshal(payload, &env); err != nil {
		t.Error(err)
		return nil
	}
	if encrypted, _ := env["encrypted"].(bool); !encrypted {
		return env
	}
	encoded, _ := env["data"].(string)
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Error(err)
		return nil
	}
	hash := sha256.Sum256([]byte("audit-secret"))
	block, _ := aes.NewCipher(hash[:])
	gcm, _ := cipher.NewGCM(block)
	if len(raw) < gcm.NonceSize() {
		t.Error("short encrypted response")
		return nil
	}
	payload, err = gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		t.Error(err)
		return nil
	}
	env = map[string]any{}
	if err := json.Unmarshal(payload, &env); err != nil {
		t.Error(err)
		return nil
	}
	return env
}
