package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go-backend/internal/http/middleware"
	runtimenft "go-backend/internal/runtime/nftables"
)

type blockingRedeployManager struct {
	fakeNftablesManager
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (m *blockingRedeployManager) Reconcile(ctx context.Context, cfg runtimenft.SSHConfig, plan runtimenft.NodePlan) (runtimenft.ApplyResult, error) {
	m.once.Do(func() { close(m.entered); <-m.release })
	return m.fakeNftablesManager.Reconcile(ctx, cfg, plan)
}

func TestTunnelRedeployStreamFlushesBeforeSlowMutationAndReportsMixedResults(t *testing.T) {
	fixture := setupNftablesHandler(t)
	h := fixture.handler
	seedNftablesSSHConfig(t, h, fixture.nodeID)
	id := seedTunnelForNftables(t, h, "stream-tunnel", fixture.nodeID)
	manager := &blockingRedeployManager{entered: make(chan struct{}), release: make(chan struct{})}
	h.nftablesManager = manager
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(manager.release) }) }
	defer release()
	server := httptest.NewServer(http.HandlerFunc(h.tunnelBatchRedeployStream))
	defer server.Close()
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Post(server.URL, "application/json", strings.NewReader(fmt.Sprintf(`{"ids":[%d,999999,%d]}`, id, id)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	decoder := json.NewDecoder(response.Body)
	read := func(want string) map[string]interface{} {
		t.Helper()
		var event struct {
			Type string
			Data map[string]interface{}
		}
		if err := decoder.Decode(&event); err != nil {
			t.Fatal(err)
		}
		if event.Type != want {
			t.Fatalf("expected %s, got %s", want, event.Type)
		}
		return event.Data
	}
	if start := read("start"); start["total"] != float64(2) {
		t.Fatalf("duplicate IDs not deduplicated: %+v", start)
	}
	processing := read("processing")
	if processing["name"] != "stream-tunnel" || processing["id"] != float64(id) {
		t.Fatalf("wrong current tunnel: %+v", processing)
	}
	select {
	case <-manager.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("did not start mutation")
	}
	// Both events arrived over HTTP while the actual mutation is still blocked.
	release()
	item := read("item")
	if item["result"].(map[string]interface{})["success"] != true || item["progress"].(map[string]interface{})["completed"] != float64(1) {
		t.Fatalf("wrong success result: %+v", item)
	}
	read("processing")
	item = read("item")
	if item["result"].(map[string]interface{})["success"] != false || item["result"].(map[string]interface{})["reason"] == "" {
		t.Fatalf("wrong failure result: %+v", item)
	}
	done := read("done")
	progress := done["progress"].(map[string]interface{})
	result := done["result"].(map[string]interface{})
	if progress["completed"] != float64(2) || progress["success"] != float64(1) || progress["failed"] != float64(1) || result["successCount"] != float64(1) || result["failCount"] != float64(1) {
		t.Fatalf("incorrect final result: %+v", done)
	}
	if manager.reconcileCount() != 1 {
		t.Fatalf("expected one execution, got %d", manager.reconcileCount())
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		t.Fatalf("expected EOF after done, got %v", err)
	}
}

func TestTunnelRedeployStreamStopsAfterDisconnect(t *testing.T) {
	fixture := setupNftablesHandler(t)
	h := fixture.handler
	seedNftablesSSHConfig(t, h, fixture.nodeID)
	first := seedTunnelForNftables(t, h, "first", fixture.nodeID)
	second := seedTunnelForNftables(t, h, "second", fixture.nodeID)
	manager := &blockingRedeployManager{entered: make(chan struct{}), release: make(chan struct{})}
	h.nftablesManager = manager
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/batch-redeploy/stream", strings.NewReader(fmt.Sprintf(`{"ids":[%d,%d]}`, first, second))).WithContext(ctx)
	res := httptest.NewRecorder()
	finished := make(chan struct{})
	go func() { defer close(finished); h.tunnelBatchRedeployStream(res, req) }()
	select {
	case <-manager.entered:
	case <-time.After(2 * time.Second):
		close(manager.release)
		t.Fatal("mutation did not start")
	}
	cancel()
	close(manager.release)
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not stop")
	}
	if manager.reconcileCount() != 1 {
		t.Fatalf("started another mutation after disconnect: %d", manager.reconcileCount())
	}
	if strings.Contains(res.Body.String(), `"type":"done"`) {
		t.Fatal("disconnected stream claimed completion")
	}
}

type failedRedeployWriter struct{ *httptest.ResponseRecorder }

func (w failedRedeployWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestTunnelRedeployStreamDoesNotMutateWhenInitialWriteFails(t *testing.T) {
	fixture := setupNftablesHandler(t)
	h := fixture.handler
	seedNftablesSSHConfig(t, h, fixture.nodeID)
	id := seedTunnelForNftables(t, h, "stream-tunnel", fixture.nodeID)
	manager := &fakeNftablesManager{}
	h.nftablesManager = manager
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(fmt.Sprintf(`{"ids":[%d]}`, id)))
	h.tunnelBatchRedeployStream(failedRedeployWriter{httptest.NewRecorder()}, req)
	if manager.reconcileCount() != 0 {
		t.Fatal("mutation ran despite failed progress stream")
	}
}

type delayedRedeployManager struct{ fakeNftablesManager }

func (m *delayedRedeployManager) Reconcile(ctx context.Context, cfg runtimenft.SSHConfig, plan runtimenft.NodePlan) (runtimenft.ApplyResult, error) {
	time.Sleep(100 * time.Millisecond)
	return m.fakeNftablesManager.Reconcile(ctx, cfg, plan)
}

type shortRedeployDeadlineWriter struct {
	http.ResponseWriter
	deadlineCalls *atomic.Int32
}

func (w shortRedeployDeadlineWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w shortRedeployDeadlineWriter) SetWriteDeadline(deadline time.Time) error {
	remaining := time.Until(deadline)
	if remaining < time.Minute || remaining > 2*time.Minute {
		return fmt.Errorf("expected bounded two-minute write window, got %v", remaining)
	}
	w.deadlineCalls.Add(1)
	// Shorten the production idle window for a real, fast timeout regression.
	return http.NewResponseController(w.ResponseWriter).SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
}

func TestTunnelRedeployStreamRenewsDeadlineThroughRequestLog(t *testing.T) {
	fixture := setupNftablesHandler(t)
	h := fixture.handler
	seedNftablesSSHConfig(t, h, fixture.nodeID)
	ids := make([]int64, 3)
	for i := range ids {
		ids[i] = seedTunnelForNftables(t, h, fmt.Sprintf("slow-%d", i), fixture.nodeID)
	}
	manager := &delayedRedeployManager{}
	h.nftablesManager = manager
	var deadlineCalls atomic.Int32
	logged := middleware.RequestLog(http.HandlerFunc(h.tunnelBatchRedeployStream))
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logged.ServeHTTP(shortRedeployDeadlineWriter{w, &deadlineCalls}, r)
	}))
	server.Config.WriteTimeout = 50 * time.Millisecond
	server.Start()
	defer server.Close()
	body, _ := json.Marshal(map[string]interface{}{"ids": ids})
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Post(server.URL, "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("stream timed out across batch: %v (body %s)", err, payload)
	}
	if !strings.Contains(string(payload), `"type":"done"`) {
		t.Fatalf("batch did not finish: %s", payload)
	}
	if manager.reconcileCount() != 3 {
		t.Fatalf("wrong execution count: %d", manager.reconcileCount())
	}
	if deadlineCalls.Load() != 8 {
		t.Fatalf("each of 8 events must renew its deadline, got %d", deadlineCalls.Load())
	}
}

type flushFailureRedeployWriter struct {
	*httptest.ResponseRecorder
	flushes int
}

func (w *flushFailureRedeployWriter) FlushError() error {
	w.flushes++
	if w.flushes == 2 {
		return io.ErrClosedPipe
	}
	w.ResponseRecorder.Flush()
	return nil
}

func TestTunnelRedeployStreamStopsBeforeMutationOnProcessingFlushError(t *testing.T) {
	fixture := setupNftablesHandler(t)
	h := fixture.handler
	seedNftablesSSHConfig(t, h, fixture.nodeID)
	id := seedTunnelForNftables(t, h, "flush-error", fixture.nodeID)
	manager := &fakeNftablesManager{}
	h.nftablesManager = manager
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(fmt.Sprintf(`{"ids":[%d]}`, id)))
	writer := &flushFailureRedeployWriter{ResponseRecorder: httptest.NewRecorder()}
	middleware.RequestLog(http.HandlerFunc(h.tunnelBatchRedeployStream)).ServeHTTP(writer, req)
	if writer.flushes != 2 {
		t.Fatalf("expected start and processing flush, got %d", writer.flushes)
	}
	if manager.reconcileCount() != 0 {
		t.Fatal("mutation ran after failed processing flush")
	}
}
