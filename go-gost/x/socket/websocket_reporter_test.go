package socket

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = orig
		_ = w.Close()
		_ = r.Close()
	}()

	fn()

	_ = w.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return buf.String()
}

func TestBuildWebSocketCandidatesSecureFirst(t *testing.T) {
	candidates := buildWebSocketCandidates("panel.example.com:443", "abc", "2.0.2", 1, 0, 1, "")

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if !strings.HasPrefix(candidates[0], "wss://") {
		t.Fatalf("expected first candidate to start with wss://, got %s", candidates[0])
	}
	if !strings.HasPrefix(candidates[1], "ws://") {
		t.Fatalf("expected second candidate to start with ws://, got %s", candidates[1])
	}
}

func TestBuildWebSocketCandidatesUsesPreferredScheme(t *testing.T) {
	candidates := buildWebSocketCandidates("panel.example.com:443", "abc", "2.0.2", 1, 0, 1, "ws")

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if !strings.HasPrefix(candidates[0], "ws://") {
		t.Fatalf("expected preferred ws:// candidate first, got %s", candidates[0])
	}
	if !strings.HasPrefix(candidates[1], "wss://") {
		t.Fatalf("expected fallback wss:// candidate second, got %s", candidates[1])
	}
}

func TestBuildWebSocketCandidatesNormalizesSchemePrefixedAddr(t *testing.T) {
	candidates := buildWebSocketCandidates("https://panel.example.com:443/path?q=1", "abc", "2.0.2", 0, 0, 0, "")

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if !strings.HasPrefix(candidates[0], "wss://panel.example.com:443/") {
		t.Fatalf("expected normalized wss candidate, got %s", candidates[0])
	}
	if !strings.HasPrefix(candidates[1], "ws://panel.example.com:443/") {
		t.Fatalf("expected normalized ws fallback candidate, got %s", candidates[1])
	}
}

func TestDialWebSocketWithFallbackTriesWSAfterWSSFailure(t *testing.T) {
	orig := wsDial
	defer func() { wsDial = orig }()

	var attempts []string
	wsDial = func(_ *websocket.Dialer, rawURL string) (*websocket.Conn, *http.Response, error) {
		attempts = append(attempts, rawURL)
		if strings.HasPrefix(rawURL, "wss://") {
			return nil, nil, errors.New("tls failed")
		}
		return &websocket.Conn{}, nil, nil
	}

	_, usedURL, err := dialWebSocketWithFallback(
		&websocket.Dialer{},
		[]string{
			"wss://panel.example.com/system-info?type=1&secret=abc",
			"ws://panel.example.com/system-info?type=1&secret=abc",
		},
	)
	if err != nil {
		t.Fatalf("expected fallback success, got err=%v", err)
	}
	if !strings.HasPrefix(usedURL, "ws://") {
		t.Fatalf("expected fallback ws:// url, got %s", usedURL)
	}
	if len(attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(attempts))
	}
	if !strings.HasPrefix(attempts[0], "wss://") || !strings.HasPrefix(attempts[1], "ws://") {
		t.Fatalf("unexpected attempt order: %#v", attempts)
	}
}

func TestDetectWebSocketScheme(t *testing.T) {
	if detectWebSocketScheme("wss://panel.example.com/system-info") != "wss" {
		t.Fatalf("expected wss detection")
	}
	if detectWebSocketScheme("ws://panel.example.com/system-info") != "ws" {
		t.Fatalf("expected ws detection")
	}
	if detectWebSocketScheme("http://panel.example.com/system-info") != "" {
		t.Fatalf("expected empty detection for non-websocket scheme")
	}
}

func TestSanitizeWebSocketURL(t *testing.T) {
	raw := "wss://panel.example.com/system-info?type=1&secret=abc&version=2.0.2"
	sanitized := sanitizeWebSocketURL(raw)

	if strings.Contains(sanitized, "secret=abc") {
		t.Fatalf("expected secret to be masked, got %s", sanitized)
	}
	if !strings.Contains(sanitized, "secret=%2A%2A%2A") {
		t.Fatalf("expected masked secret in url, got %s", sanitized)
	}
}

func TestNewWebSocketReporterUsesReducedMetricInterval(t *testing.T) {
	reporter := NewWebSocketReporter("panel.example.com:443", "abc")

	if reporter.pingInterval != defaultMetricReportInterval {
		t.Fatalf("expected metric interval %s, got %s", defaultMetricReportInterval, reporter.pingInterval)
	}
}

func TestFormatWebSocketDialErrorIncludesHTTPStatus(t *testing.T) {
	err := errors.New("websocket: bad handshake")
	resp := &http.Response{
		Status: "403 Forbidden",
		Body:   io.NopCloser(strings.NewReader("forbidden")),
	}

	msg := formatWebSocketDialError(err, resp)
	if !strings.Contains(msg, "HTTP 403 Forbidden") {
		t.Fatalf("expected status in message, got %s", msg)
	}
	if !strings.Contains(msg, "forbidden") {
		t.Fatalf("expected response body in message, got %s", msg)
	}
}

func TestAgentUpgradeRestartScriptStopsLegacyGostService(t *testing.T) {
	script := buildAgentRestartScript("/tmp/flux_agent.new", "/etc/flux_agent/flux_agent")

	if !strings.Contains(script, "systemctl stop flux_agent") {
		t.Fatalf("expected script to stop flux_agent, got %s", script)
	}
	if !strings.Contains(script, "mv /tmp/flux_agent.new /etc/flux_agent/flux_agent") {
		t.Fatalf("expected script to replace the flux_agent binary, got %s", script)
	}
	if !strings.Contains(script, "systemctl stop gost") {
		t.Fatalf("expected script to stop the legacy gost service, got %s", script)
	}
	if !strings.Contains(script, "systemctl disable gost") {
		t.Fatalf("expected script to disable the legacy gost service, got %s", script)
	}
	if !strings.Contains(script, "rm -f /usr/local/bin/gost") {
		t.Fatalf("expected script to remove the legacy gost binary, got %s", script)
	}
	if !strings.Contains(script, "WorkingDirectory=/etc/gost") {
		t.Fatalf("expected script to scope cleanup to the legacy FLVX gost service definition, got %s", script)
	}
	if !strings.Contains(script, "systemctl start flux_agent") {
		t.Fatalf("expected script to restart flux_agent, got %s", script)
	}
	if strings.Contains(script, "systemctl stop flux_agent && systemctl stop gost 2>/dev/null || true") {
		t.Fatalf("expected legacy gost cleanup fallback to be scoped, got %s", script)
	}
	if runtime.GOARCH == "" {
		t.Fatalf("unexpected empty runtime arch")
	}
}

func TestStartWebSocketReporterWithConfigPreservesProtocolDefaultsWithoutConfigFile(t *testing.T) {
	origDial := wsDial
	defer func() { wsDial = origDial }()

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWD)
	})
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("change working directory: %v", err)
	}

	urls := make(chan string, 1)
	wsDial = func(_ *websocket.Dialer, rawURL string) (*websocket.Conn, *http.Response, error) {
		select {
		case urls <- rawURL:
		default:
		}
		return nil, nil, errors.New("dial failed")
	}

	reporter := StartWebSocketReporterWithConfig("panel.example.com:443", "abc", 1, 0, 1, "2.0.2")
	defer reporter.Stop()

	select {
	case rawURL := <-urls:
		if !strings.Contains(rawURL, "http=1&tls=0&socks=1") {
			t.Fatalf("expected reconnect URL to preserve startup protocol values, got %s", rawURL)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for websocket dial")
	}
}

func TestStartWebSocketReporterWithConfigLogsSanitizedURL(t *testing.T) {
	origDial := wsDial
	defer func() { wsDial = origDial }()

	ready := make(chan struct{}, 1)
	wsDial = func(_ *websocket.Dialer, rawURL string) (*websocket.Conn, *http.Response, error) {
		select {
		case ready <- struct{}{}:
		default:
		}
		return nil, nil, errors.New("dial failed")
	}

	output := captureStdout(t, func() {
		reporter := StartWebSocketReporterWithConfig("panel.example.com:443", "abc123", 1, 0, 1, "2.0.2")
		select {
		case <-ready:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for websocket dial")
		}
		reporter.Stop()
	})

	if strings.Contains(output, "secret=abc123") {
		t.Fatalf("expected logged websocket URL to mask the node secret, got %s", output)
	}
	if !strings.Contains(output, "secret=%2A%2A%2A") {
		t.Fatalf("expected logged websocket URL to include masked secret, got %s", output)
	}
}
