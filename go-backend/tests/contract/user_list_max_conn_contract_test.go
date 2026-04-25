package contract_test

import (
	"testing"
	"time"

	"go-backend/internal/auth"
)

func TestUserListReturnsMaxConn(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, max_conn, created_time, updated_time, status)
		VALUES(2, 'max_conn_user', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 10, 37, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	out := requestContractEnvelope(t, router, adminToken, "/api/v1/user/list", map[string]interface{}{})
	if out.Code != 0 {
		t.Fatalf("expected /user/list success, got code=%d msg=%s", out.Code, out.Msg)
	}

	rows := mustContractSlice(t, out.Data, "user list")
	var target map[string]interface{}
	for _, row := range rows {
		item, ok := row.(map[string]interface{})
		if !ok {
			t.Fatalf("expected user item to be object, got %T", row)
		}
		idVal, ok := item["id"].(float64)
		if !ok {
			t.Fatalf("expected user id to be float64, got %T", item["id"])
		}
		if int64(idVal) == 2 {
			target = item
			break
		}
	}
	if target == nil {
		t.Fatalf("user 2 not found in /user/list response")
	}

	maxConnVal, ok := target["maxConn"].(float64)
	if !ok {
		t.Fatalf("expected maxConn to be float64, got %T (%v)", target["maxConn"], target["maxConn"])
	}
	if int(maxConnVal) != 37 {
		t.Fatalf("expected maxConn 37 in /user/list, got %v", maxConnVal)
	}
}
