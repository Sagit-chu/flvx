package contract_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-backend/internal/auth"
	"go-backend/internal/http/response"
)

func TestUserCreateE2E(t *testing.T) {
	secret := "user-create-jwt-secret"
	router, repo := setupContractRouter(t, secret)

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	t.Run("create user with minimal fields", func(t *testing.T) {
		payload := map[string]interface{}{
			"user": "test_user_minimal",
			"pwd":  "password123",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/create", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		assertCode(t, res, 0)

		userID := mustQueryInt64(t, repo, `SELECT id FROM user WHERE user = ?`, "test_user_minimal")
		if userID <= 0 {
			t.Fatalf("expected user created")
		}

		roleID := mustQueryInt(t, repo, `SELECT role_id FROM user WHERE id = ?`, userID)
		if roleID != 1 {
			t.Fatalf("expected role_id=1 for normal user, got %d", roleID)
		}

		status := mustQueryInt(t, repo, `SELECT status FROM user WHERE id = ?`, userID)
		if status != 1 {
			t.Fatalf("expected status=1, got %d", status)
		}
	})

	t.Run("create user with all fields", func(t *testing.T) {
		payload := map[string]interface{}{
			"user":          "test_user_full",
			"pwd":           "password456",
			"flow":          500,
			"num":           5,
			"expTime":       time.Now().Add(30 * 24 * time.Hour).UnixMilli(),
			"flowResetTime": 15,
			"status":        1,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/create", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		assertCode(t, res, 0)

		userID := mustQueryInt64(t, repo, `SELECT id FROM user WHERE user = ?`, "test_user_full")
		flow := mustQueryInt64(t, repo, `SELECT flow FROM user WHERE id = ?`, userID)
		if flow != 500 {
			t.Fatalf("expected flow=500, got %d", flow)
		}

		num := mustQueryInt(t, repo, `SELECT num FROM user WHERE id = ?`, userID)
		if num != 5 {
			t.Fatalf("expected num=5, got %d", num)
		}
	})

	t.Run("create user with groups", func(t *testing.T) {
		now := time.Now().UnixMilli()
		if err := repo.DB().Exec(`INSERT INTO user_group(name, created_time, updated_time) VALUES(?, ?, ?)`, "test-group", now, now).Error; err != nil {
			t.Fatalf("insert user_group: %v", err)
		}
		groupID := mustLastInsertID(t, repo, "test-group")

		payload := map[string]interface{}{
			"user":     "test_user_with_group",
			"pwd":      "password789",
			"groupIds": []int64{groupID},
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/create", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		assertCode(t, res, 0)

		userID := mustQueryInt64(t, repo, `SELECT id FROM user WHERE user = ?`, "test_user_with_group")
		userGroupCount := mustQueryInt(t, repo, `SELECT COUNT(1) FROM user_group_member WHERE user_id = ?`, userID)
		if userGroupCount != 1 {
			t.Fatalf("expected 1 user_group_member, got %d", userGroupCount)
		}
	})

	t.Run("reject duplicate username", func(t *testing.T) {
		payload := map[string]interface{}{
			"user": "test_user_dup",
			"pwd":  "password1",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/create", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		assertCode(t, res, 0)

		payload2 := map[string]interface{}{
			"user": "test_user_dup",
			"pwd":  "password2",
		}
		body2, _ := json.Marshal(payload2)
		req2 := httptest.NewRequest(http.MethodPost, "/api/v1/user/create", bytes.NewReader(body2))
		req2.Header.Set("Authorization", adminToken)
		req2.Header.Set("Content-Type", "application/json")
		res2 := httptest.NewRecorder()
		router.ServeHTTP(res2, req2)

		assertCodeMsg(t, res2, -1, "用户名已存在")
	})

	t.Run("reject empty username", func(t *testing.T) {
		payload := map[string]interface{}{
			"user": "",
			"pwd":  "password",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/create", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		assertCodeMsg(t, res, -1, "用户名或密码不能为空")
	})

	t.Run("reject empty password", func(t *testing.T) {
		payload := map[string]interface{}{
			"user": "test_user_no_pwd",
			"pwd":  "",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/create", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		assertCodeMsg(t, res, -1, "用户名或密码不能为空")
	})
}

func TestUserUpdateE2E(t *testing.T) {
	secret := "user-update-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(100, 'update_user', '3c85cdebade1c51cf64ca9f3c09d182d', 1, 2727251700000, 100, 0, 0, 1, 10, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	t.Run("update user without password", func(t *testing.T) {
		payload := map[string]interface{}{
			"id":     100,
			"user":   "update_user_renamed",
			"flow":   200,
			"num":    20,
			"status": 1,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/update", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		assertCode(t, res, 0)

		username := mustQueryString(t, repo, `SELECT user FROM user WHERE id = ?`, 100)
		if username != "update_user_renamed" {
			t.Fatalf("expected username=update_user_renamed, got %s", username)
		}

		flow := mustQueryInt64(t, repo, `SELECT flow FROM user WHERE id = ?`, 100)
		if flow != 200 {
			t.Fatalf("expected flow=200, got %d", flow)
		}
	})

	t.Run("update user with password", func(t *testing.T) {
		payload := map[string]interface{}{
			"id":     100,
			"user":   "update_user_with_pwd",
			"pwd":    "newpassword123",
			"flow":   300,
			"num":    30,
			"status": 1,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/update", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		assertCode(t, res, 0)

		pwd := mustQueryString(t, repo, `SELECT pwd FROM user WHERE id = ?`, 100)
		expectedPwd := "e99a18c428cb38d5f260853678922e03"
		if pwd != expectedPwd {
			t.Fatalf("expected pwd=%s, got %s", expectedPwd, pwd)
		}
	})

	t.Run("reject update admin user", func(t *testing.T) {
		payload := map[string]interface{}{
			"id":     1,
			"user":   "hacked_admin",
			"flow":   999,
			"status": 1,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/update", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		assertCodeMsg(t, res, -1, "请不要作死")
	})

	t.Run("reject duplicate username on update", func(t *testing.T) {
		if err := repo.DB().Exec(`
			INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
			VALUES(101, 'existing_user', 'pwd', 1, 2727251700000, 100, 0, 0, 1, 10, ?, ?, 1)
		`, now, now).Error; err != nil {
			t.Fatalf("insert existing user: %v", err)
		}

		payload := map[string]interface{}{
			"id":     100,
			"user":   "existing_user",
			"flow":   100,
			"status": 1,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/update", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		assertCodeMsg(t, res, -1, "用户名已存在")
	})
}

func TestUserDeleteE2E(t *testing.T) {
	secret := "user-delete-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	t.Run("delete normal user cascade", func(t *testing.T) {
		if err := repo.DB().Exec(`
			INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
			VALUES(200, 'delete_user', 'pwd', 1, 2727251700000, 100, 0, 0, 1, 10, ?, ?, 1)
		`, now, now).Error; err != nil {
			t.Fatalf("insert user: %v", err)
		}

		if err := repo.DB().Exec(`
			INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
			VALUES('delete-user-tunnel', 1.0, 1, 'tls', 99999, ?, ?, 1, NULL, 0)
		`, now, now).Error; err != nil {
			t.Fatalf("insert tunnel: %v", err)
		}
		tunnelID := mustLastInsertID(t, repo, "delete-user-tunnel")

		if err := repo.DB().Exec(`
			INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
			VALUES(200, 200, ?, NULL, 10, 100, 0, 0, 1, 2727251700000, 1)
		`, tunnelID).Error; err != nil {
			t.Fatalf("insert user_tunnel: %v", err)
		}

		if err := repo.DB().Exec(`
			INSERT INTO user_group(name, created_time, updated_time) VALUES('delete-user-group', ?, ?)
		`, now, now).Error; err != nil {
			t.Fatalf("insert user_group: %v", err)
		}
		groupID := mustLastInsertID(t, repo, "delete-user-group")

		if err := repo.DB().Exec(`
			INSERT INTO user_group_member(user_group_id, user_id, created_time) VALUES(?, 200, ?)
		`, groupID, now).Error; err != nil {
			t.Fatalf("insert user_group_member: %v", err)
		}

		payload := map[string]interface{}{"id": 200}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/delete", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		assertCode(t, res, 0)

		userCount := mustQueryInt(t, repo, `SELECT COUNT(1) FROM user WHERE id = 200`)
		if userCount != 0 {
			t.Fatalf("expected user deleted, got count=%d", userCount)
		}

		utCount := mustQueryInt(t, repo, `SELECT COUNT(1) FROM user_tunnel WHERE user_id = 200`)
		if utCount != 0 {
			t.Fatalf("expected user_tunnel cascade deleted, got count=%d", utCount)
		}

		ugmCount := mustQueryInt(t, repo, `SELECT COUNT(1) FROM user_group_member WHERE user_id = 200`)
		if ugmCount != 0 {
			t.Fatalf("expected user_group_member cascade deleted, got count=%d", ugmCount)
		}
	})

	t.Run("reject delete admin user", func(t *testing.T) {
		payload := map[string]interface{}{"id": 1}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/delete", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		assertCodeMsg(t, res, -1, "请不要作死")
	})
}

func TestUserResetFlowE2E(t *testing.T) {
	secret := "user-reset-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(300, 'reset_user', 'pwd', 1, 2727251700000, 100, 5000, 3000, 1, 10, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES('reset-user-tunnel', 1.0, 1, 'tls', 99999, ?, ?, 1, NULL, 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, repo, "reset-user-tunnel")

	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
		VALUES(300, 300, ?, NULL, 10, 100, 2000, 1000, 1, 2727251700000, 1)
	`, tunnelID).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}

	t.Run("reset user flow by user level", func(t *testing.T) {
		payload := map[string]interface{}{
			"id":   300,
			"type": 1,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/reset", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		assertCode(t, res, 0)

		inFlow := mustQueryInt64(t, repo, `SELECT in_flow FROM user WHERE id = 300`)
		outFlow := mustQueryInt64(t, repo, `SELECT out_flow FROM user WHERE id = 300`)
		if inFlow != 0 || outFlow != 0 {
			t.Fatalf("expected user flow reset to 0, got in=%d out=%d", inFlow, outFlow)
		}
	})

	t.Run("reset user flow by tunnel level", func(t *testing.T) {
		if err := repo.DB().Exec(`UPDATE user SET in_flow = 5000, out_flow = 3000 WHERE id = 300`).Error; err != nil {
			t.Fatalf("reset user flow: %v", err)
		}

		payload := map[string]interface{}{
			"id":   300,
			"type": 2,
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/reset", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		assertCode(t, res, 0)

		utInFlow := mustQueryInt64(t, repo, `SELECT in_flow FROM user_tunnel WHERE user_id = 300`)
		utOutFlow := mustQueryInt64(t, repo, `SELECT out_flow FROM user_tunnel WHERE user_id = 300`)
		if utInFlow != 0 || utOutFlow != 0 {
			t.Fatalf("expected user_tunnel flow reset to 0, got in=%d out=%d", utInFlow, utOutFlow)
		}
	})
}

func TestUserGroupsE2E(t *testing.T) {
	secret := "user-groups-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(400, 'groups_user', 'pwd', 1, 2727251700000, 100, 0, 0, 1, 10, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user_group(id, name, created_time, updated_time) VALUES(400, 'test-group-a', ?, ?)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user_group a: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user_group(id, name, created_time, updated_time) VALUES(401, 'test-group-b', ?, ?)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user_group b: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user_group_member(user_group_id, user_id, created_time) VALUES(400, 400, ?)
	`, now).Error; err != nil {
		t.Fatalf("insert user_group_member a: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user_group_member(user_group_id, user_id, created_time) VALUES(401, 400, ?)
	`, now).Error; err != nil {
		t.Fatalf("insert user_group_member b: %v", err)
	}

	t.Run("get user groups", func(t *testing.T) {
		payload := map[string]interface{}{"id": 400}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/groups", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d (%s)", out.Code, out.Msg)
		}

		groupIDs, ok := out.Data.([]interface{})
		if !ok {
			t.Fatalf("expected array data, got %T", out.Data)
		}
		if len(groupIDs) != 2 {
			t.Fatalf("expected 2 groups, got %d", len(groupIDs))
		}
	})
}

func TestUserListE2E(t *testing.T) {
	secret := "user-list-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	adminToken, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(500, 'list_user_alpha', 'pwd', 1, 2727251700000, 100, 0, 0, 1, 10, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user alpha: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(501, 'list_user_beta', 'pwd', 1, 2727251700000, 100, 0, 0, 1, 10, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user beta: %v", err)
	}

	t.Run("list all users", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/list", bytes.NewBufferString(`{}`))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d (%s)", out.Code, out.Msg)
		}

		users, ok := out.Data.([]interface{})
		if !ok {
			t.Fatalf("expected array data, got %T", out.Data)
		}
		if len(users) < 3 {
			t.Fatalf("expected at least 3 users, got %d", len(users))
		}
	})

	t.Run("list users with keyword filter", func(t *testing.T) {
		payload := map[string]interface{}{"keyword": "alpha"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/list", bytes.NewReader(body))
		req.Header.Set("Authorization", adminToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d (%s)", out.Code, out.Msg)
		}

		users, ok := out.Data.([]interface{})
		if !ok {
			t.Fatalf("expected array data, got %T", out.Data)
		}
		if len(users) != 1 {
			t.Fatalf("expected 1 user with keyword 'alpha', got %d", len(users))
		}
	})
}

func TestUserPackageE2E(t *testing.T) {
	secret := "user-package-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(600, 'package_user', 'pwd', 1, 2727251700000, 100, 500, 300, 1, 10, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	userToken, err := auth.GenerateToken(600, "package_user", 1, secret)
	if err != nil {
		t.Fatalf("generate user token: %v", err)
	}

	if err := repo.DB().Exec(`
		INSERT INTO tunnel(name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
		VALUES('package-tunnel', 1.0, 1, 'tls', 99999, ?, ?, 1, NULL, 0)
	`, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	tunnelID := mustLastInsertID(t, repo, "package-tunnel")

	if err := repo.DB().Exec(`
		INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status, tunnel_name)
		VALUES(600, 600, ?, NULL, 10, 100, 200, 100, 1, 2727251700000, 1, 'package-tunnel')
	`, tunnelID).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}

	t.Run("get user package info", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/package", bytes.NewBufferString(`{}`))
		req.Header.Set("Authorization", userToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		var out response.R
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if out.Code != 0 {
			t.Fatalf("expected code 0, got %d (%s)", out.Code, out.Msg)
		}

		data, ok := out.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("expected object data, got %T", out.Data)
		}

		userInfo, ok := data["userInfo"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected userInfo object, got %T", data["userInfo"])
		}

		if userInfo["user"] != "package_user" {
			t.Fatalf("expected user=package_user, got %v", userInfo["user"])
		}

		tunnelPerms, ok := data["tunnelPermissions"].([]interface{})
		if !ok {
			t.Fatalf("expected tunnelPermissions array, got %T", data["tunnelPermissions"])
		}
		if len(tunnelPerms) != 1 {
			t.Fatalf("expected 1 tunnel permission, got %d", len(tunnelPerms))
		}
	})
}

func TestUpdatePasswordE2E(t *testing.T) {
	secret := "update-pwd-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now().UnixMilli()

	if err := repo.DB().Exec(`
		INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
		VALUES(700, 'pwd_user', '3c85cdebade1c51cf64ca9f3c09d182d', 1, 2727251700000, 100, 0, 0, 1, 10, ?, ?, 1)
	`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}

	userToken, err := auth.GenerateToken(700, "pwd_user", 1, secret)
	if err != nil {
		t.Fatalf("generate user token: %v", err)
	}

	t.Run("update password success", func(t *testing.T) {
		payload := map[string]interface{}{
			"newUsername":     "pwd_user_renamed",
			"currentPassword": "admin_user",
			"newPassword":     "newpassword123",
			"confirmPassword": "newpassword123",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/updatePassword", bytes.NewReader(body))
		req.Header.Set("Authorization", userToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		assertCode(t, res, 0)

		username := mustQueryString(t, repo, `SELECT user FROM user WHERE id = 700`)
		if username != "pwd_user_renamed" {
			t.Fatalf("expected username=pwd_user_renamed, got %s", username)
		}
	})

	t.Run("reject wrong current password", func(t *testing.T) {
		payload := map[string]interface{}{
			"newUsername":     "pwd_user",
			"currentPassword": "wrongpassword",
			"newPassword":     "newpassword456",
			"confirmPassword": "newpassword456",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/updatePassword", bytes.NewReader(body))
		req.Header.Set("Authorization", userToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		assertCodeMsg(t, res, -1, "当前密码错误")
	})

	t.Run("reject mismatched confirm password", func(t *testing.T) {
		payload := map[string]interface{}{
			"newUsername":     "pwd_user",
			"currentPassword": "newpassword123",
			"newPassword":     "newpassword789",
			"confirmPassword": "differentpassword",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/user/updatePassword", bytes.NewReader(body))
		req.Header.Set("Authorization", userToken)
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)

		assertCodeMsg(t, res, -1, "新密码和确认密码不匹配")
	})
}
