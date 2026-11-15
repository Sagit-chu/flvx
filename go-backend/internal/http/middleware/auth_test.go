package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-backend/internal/auth"
	"go-backend/internal/http/response"
)

func TestJWTRejectsPasswordChangedToken(t *testing.T) {
	secret := "unit-test-secret"
	token, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	claims, err := auth.ParseClaims(token, secret)
	if err != nil {
		t.Fatalf("parse claims: %v", err)
	}

	wrapped := JWT(AuthOptions{
		JWTSecret: secret,
		GetUserAuthState: func(userID int64) (*auth.UserAuthState, error) {
			return &auth.UserAuthState{ID: userID, RoleID: 0, Status: 1, PasswordChangedAt: claims.IatMs + 1}, nil
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response.WriteJSON(w, response.OK("pass"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/list", nil)
	req.Header.Set("Authorization", token)
	res := httptest.NewRecorder()
	wrapped.ServeHTTP(res, req)
	assertAuthDenied(t, res)
}

func TestJWTAcceptsCurrentUserState(t *testing.T) {
	secret := "unit-test-secret"
	token, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	claims, err := auth.ParseClaims(token, secret)
	if err != nil {
		t.Fatalf("parse claims: %v", err)
	}

	wrapped := JWT(AuthOptions{
		JWTSecret: secret,
		GetUserAuthState: func(userID int64) (*auth.UserAuthState, error) {
			return &auth.UserAuthState{ID: userID, RoleID: 0, Status: 1, PasswordChangedAt: claims.IatMs - 1}, nil
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response.WriteJSON(w, response.OK("pass"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/list", nil)
	req.Header.Set("Authorization", token)
	res := httptest.NewRecorder()
	wrapped.ServeHTTP(res, req)
	assertCode(t, res, 0)
}

func TestJWTRejectsDisabledUserToken(t *testing.T) {
	secret := "unit-test-secret"
	token, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	wrapped := JWT(AuthOptions{
		JWTSecret: secret,
		GetUserAuthState: func(userID int64) (*auth.UserAuthState, error) {
			return &auth.UserAuthState{ID: userID, RoleID: 0, Status: 0, PasswordChangedAt: time.Now().Unix()}, nil
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response.WriteJSON(w, response.OK("pass"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/list", nil)
	req.Header.Set("Authorization", token)
	res := httptest.NewRecorder()
	wrapped.ServeHTTP(res, req)
	assertAuthDenied(t, res)
}

func TestJWTRejectsPasswordChangedAtSameMillisecond(t *testing.T) {
	secret := "unit-test-secret"
	token, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	claims, err := auth.ParseClaims(token, secret)
	if err != nil {
		t.Fatalf("parse claims: %v", err)
	}

	wrapped := JWT(AuthOptions{
		JWTSecret: secret,
		GetUserAuthState: func(userID int64) (*auth.UserAuthState, error) {
			return &auth.UserAuthState{ID: userID, RoleID: 0, Status: 1, PasswordChangedAt: claims.IatMs}, nil
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response.WriteJSON(w, response.OK("pass"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/list", nil)
	req.Header.Set("Authorization", token)
	res := httptest.NewRecorder()
	wrapped.ServeHTTP(res, req)
	assertAuthDenied(t, res)
}

func TestJWTRejectsRoleMismatch(t *testing.T) {
	secret := "unit-test-secret"
	token, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	wrapped := JWT(AuthOptions{
		JWTSecret: secret,
		GetUserAuthState: func(userID int64) (*auth.UserAuthState, error) {
			return &auth.UserAuthState{ID: userID, RoleID: 1, Status: 1, PasswordChangedAt: 0}, nil
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response.WriteJSON(w, response.OK("pass"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/list", nil)
	req.Header.Set("Authorization", token)
	res := httptest.NewRecorder()
	wrapped.ServeHTTP(res, req)
	assertAuthDenied(t, res)
}

func TestJWTRejectsMissingUserState(t *testing.T) {
	secret := "unit-test-secret"
	token, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	wrapped := JWT(AuthOptions{
		JWTSecret: secret,
		GetUserAuthState: func(userID int64) (*auth.UserAuthState, error) {
			return nil, nil
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response.WriteJSON(w, response.OK("pass"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/list", nil)
	req.Header.Set("Authorization", token)
	res := httptest.NewRecorder()
	wrapped.ServeHTTP(res, req)
	assertAuthDenied(t, res)
}

func TestJWTRejectsAuthStateLookupError(t *testing.T) {
	secret := "unit-test-secret"
	token, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	wrapped := JWT(AuthOptions{
		JWTSecret: secret,
		GetUserAuthState: func(userID int64) (*auth.UserAuthState, error) {
			return nil, errors.New("boom")
		},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response.WriteJSON(w, response.OK("pass"))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tunnel/list", nil)
	req.Header.Set("Authorization", token)
	res := httptest.NewRecorder()
	wrapped.ServeHTTP(res, req)
	assertAuthDenied(t, res)
}

func TestShouldSkipDoesNotBypassConfigGet(t *testing.T) {
	if shouldSkip("/api/v1/config/get") {
		t.Fatal("expected /api/v1/config/get to require auth")
	}
}

func TestShouldSkipBypassesPublicConfigGet(t *testing.T) {
	if !shouldSkip("/api/v1/public/config/get") {
		t.Fatal("expected /api/v1/public/config/get to remain public")
	}
}

func TestJWTExpiresAfterSevenDays(t *testing.T) {
	secret := "unit-test-secret"
	token, err := auth.GenerateToken(1, "admin_user", 0, secret)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	claims, err := auth.ParseClaims(token, secret)
	if err != nil {
		t.Fatalf("parse claims: %v", err)
	}
	if got := claims.Exp - claims.Iat; got != int64(7*24*time.Hour/time.Second) {
		t.Fatalf("expected 7 day token lifetime, got %d seconds", got)
	}
	if claims.IatMs <= 0 {
		t.Fatalf("expected millisecond issuance time to be populated, got %d", claims.IatMs)
	}
}

func assertCode(t *testing.T, rec *httptest.ResponseRecorder, expected int) {
	t.Helper()
	var out response.R
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != expected {
		t.Fatalf("expected code %d, got %d", expected, out.Code)
	}
}

func assertAuthDenied(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	assertCodeMsg(t, rec, 401, "无效的token或token已过期")
}

func assertCodeMsg(t *testing.T, rec *httptest.ResponseRecorder, expectedCode int, expectedMsg string) {
	t.Helper()
	var out response.R
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.Code != expectedCode || out.Msg != expectedMsg {
		t.Fatalf("expected (%d,%q), got (%d,%q)", expectedCode, expectedMsg, out.Code, out.Msg)
	}
}
