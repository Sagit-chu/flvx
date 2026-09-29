package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"go-backend/internal/http/response"
	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
	"go-backend/internal/ws"
)

func consumerCleanupFixture(t *testing.T, remoteURL string) (*repo.Repository, *Handler) {
	t.Helper()
	r, err := repo.Open(filepath.Join(t.TempDir(), "consumer-cleanup.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	if err := r.DB().Create(&model.Node{ID: 1, Name: "remote", Secret: "remote", ServerIP: "127.0.0.1", Port: "31000-31010", IsRemote: 1, Status: 1, RemoteURL: sql.NullString{String: remoteURL, Valid: true}, RemoteToken: sql.NullString{String: "token", Valid: true}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := r.UpsertFederationTunnelBinding(&repo.FederationTunnelBinding{TunnelID: 42, NodeID: 1, ChainType: 3, RemoteURL: remoteURL, ResourceKey: "tunnel:42:node:1:type:3:hop:0", RemoteBindingID: "binding-old", AllocatedPort: 31001, Status: 1}); err != nil {
		t.Fatal(err)
	}
	if err := r.DB().Create(&model.Tunnel{ID: 42, Name: "live tunnel", Type: 2, Protocol: "tls", Flow: 1, TrafficRatio: 1, Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	return r, &Handler{repo: r, wsServer: ws.NewServer(r, "consumer-cleanup")}
}

func TestTunnelUpdateValidationPreservesSharedRuntime(t *testing.T) {
	for _, body := range []string{
		`{"id":42,"type":2,"name":"invalid update","inNodeId":[]}`,
		`{"id":42,"type":2,"name":"invalid update","inNodeId":[{"nodeId":999}],"outNodeId":[{"nodeId":1,"port":31001}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			var releases atomic.Int64
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				releases.Add(1)
				response.WriteJSON(w, response.OKEmpty())
			}))
			defer remote.Close()
			r, h := consumerCleanupFixture(t, remote.URL)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/update", bytes.NewBufferString(body))
			rec := httptest.NewRecorder()
			h.tunnelUpdate(rec, req)
			var payload response.R
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			bindings, err := r.ListActiveFederationTunnelBindingsByTunnel(42)
			if err != nil {
				t.Fatal(err)
			}
			name, err := r.GetTunnelName(42)
			if err != nil {
				t.Fatal(err)
			}
			if payload.Code == 0 || releases.Load() != 0 || len(bindings) != 1 || name != "live tunnel" {
				t.Fatalf("invalid update changed runtime: code=%d releases=%d bindings=%v name=%s", payload.Code, releases.Load(), bindings, name)
			}
		})
	}
}

func TestFederationCleanupRetainsFailedBindingAndRetriesOnlyPending(t *testing.T) {
	var unavailable atomic.Bool
	unavailable.Store(true)
	var releases atomic.Int64
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		releases.Add(1)
		if unavailable.Load() {
			http.Error(w, "peer unavailable", http.StatusServiceUnavailable)
			return
		}
		response.WriteJSON(w, response.OKEmpty())
	}))
	defer remote.Close()
	r, h := consumerCleanupFixture(t, remote.URL)
	if err := r.UpsertFederationTunnelBinding(&repo.FederationTunnelBinding{TunnelID: 43, NodeID: 1, ChainType: 3, RemoteURL: remote.URL, RemoteBindingID: "unrelated", ResourceKey: "unrelated", Status: 1}); err != nil {
		t.Fatal(err)
	}
	if err := h.cleanupFederationRuntime(42); err == nil {
		t.Fatal("expected release failure")
	}
	pending, err := r.ListPendingFederationTunnelBindings()
	if err != nil || len(pending) != 1 {
		t.Fatalf("lost pending cleanup: %v %v", pending, err)
	}
	if err := h.cleanupFederationRuntime(42); err == nil {
		t.Fatal("expected second release failure")
	}
	if releases.Load() != 2 {
		t.Fatalf("cleanup was not retried: %d", releases.Load())
	}
	unavailable.Store(false)
	if err := h.retryPendingFederationRuntimeCleanup(); err != nil {
		t.Fatal(err)
	}
	pending, err = r.ListPendingFederationTunnelBindings()
	if err != nil || len(pending) != 0 {
		t.Fatalf("completed cleanup remains: %v %v", pending, err)
	}
	active, err := r.ListActiveFederationTunnelBindingsByTunnel(43)
	if err != nil || len(active) != 1 || releases.Load() != 3 {
		t.Fatalf("retry touched active binding: %v %v calls=%d", active, err, releases.Load())
	}
}

func TestTunnelDeleteReportsRemoteFailureAndKeepsTunnel(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, "peer unavailable", http.StatusServiceUnavailable)
	}))
	defer remote.Close()
	r, h := consumerCleanupFixture(t, remote.URL)
	rec := httptest.NewRecorder()
	h.tunnelDelete(rec, httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/delete", strings.NewReader(`{"id":42}`)))
	var payload response.R
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code == 0 {
		t.Fatal("delete reported success before peer cleanup")
	}
	if name, err := r.GetTunnelName(42); err != nil || name != "live tunnel" {
		t.Fatalf("lost tunnel: %s %v", name, err)
	}
	pending, err := r.ListPendingFederationTunnelBindings()
	if err != nil || len(pending) != 1 {
		t.Fatalf("lost binding: %v %v", pending, err)
	}
}

func TestFederationRollbackReleaseIsDurableAndRetryable(t *testing.T) {
	var unavailable atomic.Bool
	unavailable.Store(true)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if unavailable.Load() {
			http.Error(w, "peer unavailable", http.StatusServiceUnavailable)
			return
		}
		response.WriteJSON(w, response.OKEmpty())
	}))
	defer remote.Close()
	r, h := consumerCleanupFixture(t, remote.URL)
	refs := []federationRuntimeReleaseRef{{RemoteURL: remote.URL, RemoteToken: "token", BindingID: "new-binding", ReservationID: "new-reservation", ResourceKey: "new-key"}}
	if err := h.releaseFederationRuntimeRefs(refs); err == nil {
		t.Fatal("expected release error")
	}
	if err := h.releaseFederationRuntimeRefs(refs); err == nil {
		t.Fatal("expected repeat error")
	}
	pending, err := r.ListPendingFederationReleases()
	if err != nil || len(pending) != 1 {
		t.Fatalf("rollback queue lost or duplicated release: %v %v", pending, err)
	}
	unavailable.Store(false)
	// A new Handler has no in-memory state from the failed rollback.
	restarted := &Handler{repo: r}
	if err := restarted.retryPendingFederationRuntimeCleanup(); err != nil {
		t.Fatal(err)
	}
	pending, err = r.ListPendingFederationReleases()
	if err != nil || len(pending) != 0 {
		t.Fatalf("completed rollback remains queued: %v %v", pending, err)
	}
	active, err := r.ListActiveFederationTunnelBindingsByTunnel(42)
	if err != nil || len(active) != 1 {
		t.Fatalf("rollback retry removed active binding: %v %v", active, err)
	}
}

func TestFederationApplyFailureReturnsReservationForRollback(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "reserve-port") {
			response.WriteJSON(w, response.OK(map[string]interface{}{"reservationId": "reserved", "bindingId": "binding", "allocatedPort": 31001}))
			return
		}
		http.Error(w, "apply failed", http.StatusServiceUnavailable)
	}))
	defer remote.Close()
	_, h := consumerCleanupFixture(t, remote.URL)
	state := &tunnelCreateState{TunnelID: 43, Type: 2, OutNodes: []tunnelRuntimeNode{{NodeID: 1, Port: 31001}}, Nodes: map[int64]*nodeRecord{1: {ID: 1, Name: "remote", IsRemote: 1, RemoteURL: remote.URL, RemoteToken: "token"}}}
	_, refs, err := h.applyFederationRuntime(state, "")
	if err == nil || len(refs) != 1 || refs[0].ReservationID != "reserved" {
		t.Fatalf("failed apply lost reservation: refs=%+v err=%v", refs, err)
	}
}

func TestTunnelUpdateDatabaseValidationPreservesSharedRuntime(t *testing.T) {
	var releases atomic.Int64
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		releases.Add(1)
		response.WriteJSON(w, response.OKEmpty())
	}))
	defer remote.Close()
	r, h := consumerCleanupFixture(t, remote.URL)
	if err := r.DB().Create(&model.Node{ID: 2, Name: "entry", Secret: "entry", ServerIP: "127.0.0.2", Port: "31000-31010", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := r.DB().Exec(`CREATE TRIGGER reject_tunnel_update BEFORE UPDATE ON tunnel BEGIN SELECT RAISE(ABORT, 'test constraint rejected'); END`).Error; err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.tunnelUpdate(rec, httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/update", strings.NewReader(`{"id":42,"type":2,"name":"new name","inNodeId":[{"nodeId":2}],"outNodeId":[{"nodeId":1,"port":31001}]}`)))
	var payload response.R
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	active, err := r.ListActiveFederationTunnelBindingsByTunnel(42)
	if err != nil || payload.Code == 0 || !strings.Contains(payload.Msg, "test constraint rejected") || releases.Load() != 0 || len(active) != 1 {
		t.Fatalf("DB validation touched runtime: payload=%+v bindings=%v calls=%d err=%v", payload, active, releases.Load(), err)
	}
}

func TestTunnelUpdateRemoteReleaseFailurePreservesMetadata(t *testing.T) {
	var requests atomic.Int64
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests.Add(1)
		http.Error(w, "peer unavailable", http.StatusServiceUnavailable)
	}))
	defer remote.Close()
	r, h := consumerCleanupFixture(t, remote.URL)
	if err := r.DB().Create(&model.Node{ID: 2, Name: "entry", Secret: "entry", ServerIP: "127.0.0.2", Port: "31000-31010", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.tunnelUpdate(rec, httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/update", strings.NewReader(`{"id":42,"type":2,"name":"new name","inNodeId":[{"nodeId":2}],"outNodeId":[{"nodeId":1,"port":31001}]}`)))
	var payload response.R
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	name, err := r.GetTunnelName(42)
	pending, pendingErr := r.ListPendingFederationTunnelBindings()
	if err != nil || pendingErr != nil || payload.Code == 0 || name != "live tunnel" || requests.Load() != 1 || len(pending) != 1 {
		t.Fatalf("failed cleanup continued update: payload=%+v name=%s calls=%d pending=%v err=%v/%v", payload, name, requests.Load(), pending, err, pendingErr)
	}
}

func TestTunnelUpdateChainWriteValidationPreservesSharedRuntime(t *testing.T) {
	var calls atomic.Int64
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		response.WriteJSON(w, response.OKEmpty())
	}))
	defer remote.Close()
	r, h := consumerCleanupFixture(t, remote.URL)
	if err := r.DB().Create(&model.Node{ID: 2, Name: "entry", Secret: "entry", ServerIP: "127.0.0.2", Port: "31000-31010", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := r.DB().Exec(`CREATE TRIGGER reject_chain_insert BEFORE INSERT ON chain_tunnel BEGIN SELECT RAISE(ABORT, 'chain write rejected'); END`).Error; err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.tunnelUpdate(rec, httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/update", strings.NewReader(`{"id":42,"type":2,"name":"new name","inNodeId":[{"nodeId":2}],"outNodeId":[{"nodeId":1,"port":31001}]}`)))
	var payload response.R
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	active, err := r.ListActiveFederationTunnelBindingsByTunnel(42)
	if err != nil || payload.Code == 0 || !strings.Contains(payload.Msg, "chain write rejected") || calls.Load() != 0 || len(active) != 1 {
		t.Fatalf("chain validation touched runtime: payload=%+v active=%v calls=%d err=%v", payload, active, calls.Load(), err)
	}
	if name, err := r.GetTunnelName(42); err != nil || name != "live tunnel" {
		t.Fatalf("preflight metadata update was committed: %s %v", name, err)
	}
}
