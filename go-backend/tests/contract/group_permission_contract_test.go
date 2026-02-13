package contract_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-backend/internal/auth"
)

func TestGroupUserUnbindRevokesInheritedTunnelPermission(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	if _, err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(200, 'group_user_contract', '3c85cdebade1c51cf64ca9f3c09d182d', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	tunnelRes, err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES('group-contract-tunnel', 1.0, 1, 'tls', 99999, ?, ?, 1, NULL, 0)
	`, now, now)
	if err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID, err := tunnelRes.LastInsertId()
	if err != nil {
		t.Fatalf("read tunnel id: %v", err)
	}

	ugRes, err := repo.DB().Exec(`INSERT INTO user_group(name, created_time, updated_time, status) VALUES('ug-contract', ?, ?, 1)`, now, now)
	if err != nil {
		t.Fatalf("insert user_group: %v", err)
	}
	userGroupID, err := ugRes.LastInsertId()
	if err != nil {
		t.Fatalf("read user_group id: %v", err)
	}

	tgRes, err := repo.DB().Exec(`INSERT INTO tunnel_group(name, created_time, updated_time, status) VALUES('tg-contract', ?, ?, 1)`, now, now)
	if err != nil {
		t.Fatalf("insert tunnel_group: %v", err)
	}
	tunnelGroupID, err := tgRes.LastInsertId()
	if err != nil {
		t.Fatalf("read tunnel_group id: %v", err)
	}

	if _, err := repo.DB().Exec(`INSERT INTO tunnel_group_tunnel(tunnel_group_id, tunnel_id, created_time) VALUES(?, ?, ?)`, tunnelGroupID, tunnelID, now); err != nil {
		t.Fatalf("insert tunnel_group_tunnel: %v", err)
	}
	if _, err := repo.DB().Exec(`INSERT INTO group_permission(user_group_id, tunnel_group_id, created_time) VALUES(?, ?, ?)`, userGroupID, tunnelGroupID, now); err != nil {
		t.Fatalf("insert group_permission: %v", err)
	}

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	bindReq := httptest.NewRequest(http.MethodPost, "/api/v1/group/user/assign", bytes.NewBufferString(`{"groupId":`+jsonNumber(userGroupID)+`,"userIds":[200]}`))
	bindReq.Header.Set("Authorization", adminToken)
	bindRes := httptest.NewRecorder()
	router.ServeHTTP(bindRes, bindReq)
	assertCode(t, bindRes, 0)

	var userTunnelID int64
	if err := repo.DB().QueryRow(`SELECT id FROM user_tunnel WHERE user_id = 200 AND tunnel_id = ?`, tunnelID).Scan(&userTunnelID); err != nil {
		t.Fatalf("query user_tunnel after bind: %v", err)
	}

	var grantCount int
	if err := repo.DB().QueryRow(`SELECT COUNT(1) FROM group_permission_grant WHERE user_tunnel_id = ?`, userTunnelID).Scan(&grantCount); err != nil {
		t.Fatalf("query group_permission_grant after bind: %v", err)
	}
	if grantCount == 0 {
		t.Fatalf("expected non-zero grants after bind")
	}

	unbindReq := httptest.NewRequest(http.MethodPost, "/api/v1/group/user/assign", bytes.NewBufferString(`{"groupId":`+jsonNumber(userGroupID)+`,"userIds":[]}`))
	unbindReq.Header.Set("Authorization", adminToken)
	unbindRes := httptest.NewRecorder()
	router.ServeHTTP(unbindRes, unbindReq)
	assertCode(t, unbindRes, 0)

	if err := repo.DB().QueryRow(`SELECT COUNT(1) FROM group_permission_grant WHERE user_tunnel_id = ?`, userTunnelID).Scan(&grantCount); err != nil {
		t.Fatalf("query group_permission_grant after unbind: %v", err)
	}
	if grantCount != 0 {
		t.Fatalf("expected grants revoked after unbind, got %d", grantCount)
	}

	var userTunnelCount int
	if err := repo.DB().QueryRow(`SELECT COUNT(1) FROM user_tunnel WHERE id = ?`, userTunnelID).Scan(&userTunnelCount); err != nil {
		t.Fatalf("query user_tunnel after unbind: %v", err)
	}
	if userTunnelCount != 0 {
		t.Fatalf("expected user_tunnel revoked after unbind, got %d", userTunnelCount)
	}
}

func TestGroupPermissionRemoveRevokesInheritedTunnelPermission(t *testing.T) {
	secret := "contract-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	if _, err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(201, 'group_user_permission_remove', '3c85cdebade1c51cf64ca9f3c09d182d', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)
	`, now, now); err != nil {
		t.Fatalf("insert test user: %v", err)
	}

	tunnelRes, err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES('group-remove-tunnel', 1.0, 1, 'tls', 99999, ?, ?, 1, NULL, 0)
	`, now, now)
	if err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID, err := tunnelRes.LastInsertId()
	if err != nil {
		t.Fatalf("read tunnel id: %v", err)
	}

	ugRes, err := repo.DB().Exec(`INSERT INTO user_group(name, created_time, updated_time, status) VALUES('ug-remove-contract', ?, ?, 1)`, now, now)
	if err != nil {
		t.Fatalf("insert user_group: %v", err)
	}
	userGroupID, err := ugRes.LastInsertId()
	if err != nil {
		t.Fatalf("read user_group id: %v", err)
	}

	tgRes, err := repo.DB().Exec(`INSERT INTO tunnel_group(name, created_time, updated_time, status) VALUES('tg-remove-contract', ?, ?, 1)`, now, now)
	if err != nil {
		t.Fatalf("insert tunnel_group: %v", err)
	}
	tunnelGroupID, err := tgRes.LastInsertId()
	if err != nil {
		t.Fatalf("read tunnel_group id: %v", err)
	}

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	assignTunnelReq := httptest.NewRequest(http.MethodPost, "/api/v1/group/tunnel/assign", bytes.NewBufferString(`{"groupId":`+jsonNumber(tunnelGroupID)+`,"tunnelIds":[`+jsonNumber(tunnelID)+`]}`))
	assignTunnelReq.Header.Set("Authorization", adminToken)
	assignTunnelRes := httptest.NewRecorder()
	router.ServeHTTP(assignTunnelRes, assignTunnelReq)
	assertCode(t, assignTunnelRes, 0)

	assignUserReq := httptest.NewRequest(http.MethodPost, "/api/v1/group/user/assign", bytes.NewBufferString(`{"groupId":`+jsonNumber(userGroupID)+`,"userIds":[201]}`))
	assignUserReq.Header.Set("Authorization", adminToken)
	assignUserRes := httptest.NewRecorder()
	router.ServeHTTP(assignUserRes, assignUserReq)
	assertCode(t, assignUserRes, 0)

	assignPermissionReq := httptest.NewRequest(http.MethodPost, "/api/v1/group/permission/assign", bytes.NewBufferString(`{"userGroupId":`+jsonNumber(userGroupID)+`,"tunnelGroupId":`+jsonNumber(tunnelGroupID)+`}`))
	assignPermissionReq.Header.Set("Authorization", adminToken)
	assignPermissionRes := httptest.NewRecorder()
	router.ServeHTTP(assignPermissionRes, assignPermissionReq)
	assertCode(t, assignPermissionRes, 0)

	var permissionID int64
	if err := repo.DB().QueryRow(`SELECT id FROM group_permission WHERE user_group_id = ? AND tunnel_group_id = ?`, userGroupID, tunnelGroupID).Scan(&permissionID); err != nil {
		t.Fatalf("query group_permission id: %v", err)
	}

	var userTunnelID int64
	if err := repo.DB().QueryRow(`SELECT id FROM user_tunnel WHERE user_id = 201 AND tunnel_id = ?`, tunnelID).Scan(&userTunnelID); err != nil {
		t.Fatalf("query user_tunnel after assign: %v", err)
	}

	var grantCount int
	if err := repo.DB().QueryRow(`SELECT COUNT(1) FROM group_permission_grant WHERE user_tunnel_id = ?`, userTunnelID).Scan(&grantCount); err != nil {
		t.Fatalf("query group_permission_grant after assign: %v", err)
	}
	if grantCount == 0 {
		t.Fatalf("expected non-zero grants after permission assign")
	}

	removeReq := httptest.NewRequest(http.MethodPost, "/api/v1/group/permission/remove", bytes.NewBufferString(`{"id":`+jsonNumber(permissionID)+`}`))
	removeReq.Header.Set("Authorization", adminToken)
	removeRes := httptest.NewRecorder()
	router.ServeHTTP(removeRes, removeReq)
	assertCode(t, removeRes, 0)

	var permissionCount int
	if err := repo.DB().QueryRow(`SELECT COUNT(1) FROM group_permission WHERE id = ?`, permissionID).Scan(&permissionCount); err != nil {
		t.Fatalf("query group_permission after remove: %v", err)
	}
	if permissionCount != 0 {
		t.Fatalf("expected group_permission removed, got %d", permissionCount)
	}

	if err := repo.DB().QueryRow(`SELECT COUNT(1) FROM group_permission_grant WHERE user_tunnel_id = ?`, userTunnelID).Scan(&grantCount); err != nil {
		t.Fatalf("query group_permission_grant after remove: %v", err)
	}
	if grantCount != 0 {
		t.Fatalf("expected grants removed after permission remove, got %d", grantCount)
	}

	var userTunnelCount int
	if err := repo.DB().QueryRow(`SELECT COUNT(1) FROM user_tunnel WHERE id = ?`, userTunnelID).Scan(&userTunnelCount); err != nil {
		t.Fatalf("query user_tunnel after permission remove: %v", err)
	}
	if userTunnelCount != 0 {
		t.Fatalf("expected user_tunnel revoked after permission remove, got %d", userTunnelCount)
	}
}
