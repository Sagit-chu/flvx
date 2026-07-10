package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"go-backend/internal/auth"
	"go-backend/internal/http/middleware"
	"go-backend/internal/store/repo"
)

func TestForwardResetFlowPermissionsAndIsolation(t *testing.T) {
	tests := []struct {
		name        string
		actorID     int64
		actorRole   int
		forwardID   int64
		wantCode    int
		wantInFlow  int64
		wantOutFlow int64
	}{
		{name: "admin resets another user's rule", actorID: 1, actorRole: 0, forwardID: 20, wantCode: 0, wantInFlow: 0, wantOutFlow: 0},
		{name: "owner resets own rule", actorID: 2, actorRole: 1, forwardID: 20, wantCode: 0, wantInFlow: 0, wantOutFlow: 0},
		{name: "user cannot reset another user's rule", actorID: 3, actorRole: 1, forwardID: 20, wantCode: -1, wantInFlow: 111, wantOutFlow: 222},
		{name: "missing rule is rejected", actorID: 1, actorRole: 0, forwardID: 999, wantCode: -1, wantInFlow: 111, wantOutFlow: 222},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, r := setupForwardResetFlowHandler(t)
			req := newForwardResetFlowRequest(t, http.MethodPost, tt.forwardID, tt.actorID, tt.actorRole)
			res := httptest.NewRecorder()

			h.forwardResetFlow(res, req)

			if got := decodeForwardResetFlowCode(t, res); got != tt.wantCode {
				t.Fatalf("code = %d, want %d; body=%s", got, tt.wantCode, res.Body.String())
			}
			assertForwardResetFlowDBValue(t, r, "SELECT in_flow FROM forward WHERE id = 20", tt.wantInFlow)
			assertForwardResetFlowDBValue(t, r, "SELECT out_flow FROM forward WHERE id = 20", tt.wantOutFlow)
			assertForwardResetFlowDBValue(t, r, "SELECT in_flow FROM user WHERE id = 2", 700)
			assertForwardResetFlowDBValue(t, r, "SELECT out_flow FROM user_tunnel WHERE id = 10", 600)
		})
	}
}

func TestForwardResetFlowRejectsInvalidRequests(t *testing.T) {
	h, _ := setupForwardResetFlowHandler(t)

	t.Run("non post", func(t *testing.T) {
		req := newForwardResetFlowRequest(t, http.MethodGet, 20, 1, 0)
		res := httptest.NewRecorder()
		h.forwardResetFlow(res, req)
		if code := decodeForwardResetFlowCode(t, res); code != -1 {
			t.Fatalf("code = %d, want -1", code)
		}
	})

	t.Run("invalid id", func(t *testing.T) {
		req := newForwardResetFlowRequest(t, http.MethodPost, 0, 1, 0)
		res := httptest.NewRecorder()
		h.forwardResetFlow(res, req)
		if code := decodeForwardResetFlowCode(t, res); code != -1 {
			t.Fatalf("code = %d, want -1", code)
		}
	})
}

func setupForwardResetFlowHandler(t *testing.T) (*Handler, *repo.Repository) {
	t.Helper()
	r, err := repo.Open(filepath.Join(t.TempDir(), "forward-reset-handler.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	statements := []string{
		`INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status) VALUES(2, 'owner', 'pwd', 1, 0, 100, 700, 900, 0, 10, 1000, 1000, 1)`,
		`INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status) VALUES(3, 'other', 'pwd', 1, 0, 100, 0, 0, 0, 10, 1000, 1000, 1)`,
		`INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx) VALUES(1, 'tunnel', 1, 1, 'tls', 1, 1000, 1000, 1, NULL, 0)`,
		`INSERT INTO user_tunnel(id, user_id, tunnel_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status) VALUES(10, 2, 1, 10, 100, 500, 600, 0, 0, 1)`,
		`INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx) VALUES(20, 2, 'owner', 'target', 1, '127.0.0.1:80', 'fifo', 111, 222, 1000, 1000, 1, 0)`,
	}
	for _, statement := range statements {
		if err := r.DB().Exec(statement).Error; err != nil {
			t.Fatalf("seed database: %v", err)
		}
	}
	return New(r, "test-secret"), r
}

func newForwardResetFlowRequest(t *testing.T, method string, forwardID, actorID int64, roleID int) *http.Request {
	t.Helper()
	body, err := json.Marshal(map[string]int64{"id": forwardID})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(method, "/api/v1/forward/reset-flow", bytes.NewReader(body))
	claims := auth.Claims{Sub: strconv.FormatInt(actorID, 10), RoleID: roleID}
	return req.WithContext(context.WithValue(req.Context(), middleware.ClaimsContextKey, claims))
}

func decodeForwardResetFlowCode(t *testing.T, res *httptest.ResponseRecorder) int {
	t.Helper()
	var payload struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, res.Body.String())
	}
	return payload.Code
}

func assertForwardResetFlowDBValue(t *testing.T, r *repo.Repository, query string, want int64) {
	t.Helper()
	var got int64
	if err := r.DB().Raw(query).Scan(&got).Error; err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	if got != want {
		t.Fatalf("query %q returned %d, want %d", query, got, want)
	}
}
