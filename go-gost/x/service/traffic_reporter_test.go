package service

import (
	"bytes"
	"context"
	"errors"
	"github.com/go-gost/x/config"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildReportURLCandidatesSecureFirst(t *testing.T) {
	upload, config := buildReportURLCandidates("panel.example.com:443", "abc")

	if len(upload) != 2 {
		t.Fatalf("expected 2 upload candidates, got %d", len(upload))
	}
	if len(config) != 2 {
		t.Fatalf("expected 2 config candidates, got %d", len(config))
	}

	if upload[0] != "https://panel.example.com:443/flow/upload?secret=abc" {
		t.Fatalf("unexpected upload[0]: %s", upload[0])
	}
	if upload[1] != "http://panel.example.com:443/flow/upload?secret=abc" {
		t.Fatalf("unexpected upload[1]: %s", upload[1])
	}
	if config[0] != "https://panel.example.com:443/flow/config?secret=abc" {
		t.Fatalf("unexpected config[0]: %s", config[0])
	}
	if config[1] != "http://panel.example.com:443/flow/config?secret=abc" {
		t.Fatalf("unexpected config[1]: %s", config[1])
	}
}

func TestBuildReportURLCandidatesNormalizeSchemeAddr(t *testing.T) {
	upload, config := buildReportURLCandidates("https://panel.example.com:8443/path", "abc")

	if upload[0] != "https://panel.example.com:8443/flow/upload?secret=abc" {
		t.Fatalf("unexpected upload[0]: %s", upload[0])
	}
	if upload[1] != "http://panel.example.com:8443/flow/upload?secret=abc" {
		t.Fatalf("unexpected upload[1]: %s", upload[1])
	}
	if config[0] != "https://panel.example.com:8443/flow/config?secret=abc" {
		t.Fatalf("unexpected config[0]: %s", config[0])
	}
	if config[1] != "http://panel.example.com:8443/flow/config?secret=abc" {
		t.Fatalf("unexpected config[1]: %s", config[1])
	}
}

func TestPostJSONWithFallbackUsesHTTPAfterHTTPSFailure(t *testing.T) {
	orig := reportDo
	defer func() { reportDo = orig }()

	var calls []string
	reportDo = func(_ context.Context, req *http.Request, _ time.Duration) (*http.Response, error) {
		calls = append(calls, req.URL.String())
		if strings.HasPrefix(req.URL.String(), "https://") {
			return nil, errors.New("tls handshake failed")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	}

	ok, err := postJSONWithFallback(
		context.Background(),
		[]string{
			"https://panel.example.com:443/flow/upload?secret=abc",
			"http://panel.example.com:443/flow/upload?secret=abc",
		},
		[]byte(`[]`),
		"GOST-Traffic-Reporter/1.0",
		5*time.Second,
		nil,
	)
	if !ok || err != nil {
		t.Fatalf("expected fallback success, ok=%v err=%v", ok, err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if !strings.HasPrefix(calls[0], "https://") || !strings.HasPrefix(calls[1], "http://") {
		t.Fatalf("unexpected call order: %#v", calls)
	}
}

func TestPostJSONWithFallbackRemembersDetectedURL(t *testing.T) {
	orig := reportDo
	defer func() { reportDo = orig }()

	targets := []string{
		"https://panel.example.com:443/flow/upload?secret=abc",
		"http://panel.example.com:443/flow/upload?secret=abc",
	}

	var preferred string
	var calls []string
	reportDo = func(_ context.Context, req *http.Request, _ time.Duration) (*http.Response, error) {
		calls = append(calls, req.URL.String())
		if strings.HasPrefix(req.URL.String(), "https://") {
			return nil, errors.New("tls handshake failed")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	}

	ok, err := postJSONWithFallback(
		context.Background(),
		targets,
		[]byte(`[]`),
		"GOST-Traffic-Reporter/1.0",
		5*time.Second,
		&preferred,
	)
	if !ok || err != nil {
		t.Fatalf("expected first call success, ok=%v err=%v", ok, err)
	}
	if preferred != targets[1] {
		t.Fatalf("expected preferred url to be remembered as %s, got %s", targets[1], preferred)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls on first attempt, got %d", len(calls))
	}

	calls = nil
	ok, err = postJSONWithFallback(
		context.Background(),
		targets,
		[]byte(`[]`),
		"GOST-Traffic-Reporter/1.0",
		5*time.Second,
		&preferred,
	)
	if !ok || err != nil {
		t.Fatalf("expected second call success, ok=%v err=%v", ok, err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected second call to use remembered url once, got %d calls", len(calls))
	}
	if !strings.HasPrefix(calls[0], "http://") {
		t.Fatalf("expected remembered http url first, got %s", calls[0])
	}
}

func TestConfigReportDoesNotPersistStaleRuntime(t *testing.T) {
	previous, path := config.Global(), config.PersistPath()
	defer config.Set(previous)
	defer config.SetPersistPath(path)
	config.Set(&config.Config{Services: []*config.ServiceConfig{{Name: "old-runtime"}}})
	filename := filepath.Join(t.TempDir(), "gost.json")
	candidate := []byte(`{"services":[{"name":"new-on-disk"}]}`)
	if err := os.WriteFile(filename, candidate, 0600); err != nil {
		t.Fatal(err)
	}
	config.SetPersistPath(filename)
	config.EnablePersist()
	report, err := getConfigData()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(report, []byte("old-runtime")) {
		t.Fatalf("unexpected report: %s", report)
	}
	saved, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, candidate) {
		t.Fatalf("report overwrote candidate config: %s", saved)
	}
}
