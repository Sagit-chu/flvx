package handler

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"go-backend/internal/auth"
	"go-backend/internal/http/response"
	"go-backend/internal/security"
	"go-backend/internal/store/model"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

const passkeyTTL = 2 * time.Minute

type passkeyCeremony struct {
	userID  int64
	kind    string
	origin  string
	session webauthn.SessionData
}

type passkeyUser struct {
	id          int64
	name        string
	credentials []webauthn.Credential
}

func (u passkeyUser) WebAuthnID() []byte {
	id := make([]byte, 8)
	binary.BigEndian.PutUint64(id, uint64(u.id))
	return id
}
func (u passkeyUser) WebAuthnName() string                       { return u.name }
func (u passkeyUser) WebAuthnDisplayName() string                { return u.name }
func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

func passkeyConfig() (*webauthn.WebAuthn, string) {
	raw := strings.TrimSpace(os.Getenv("FLVX_WEBAUTHN_ORIGIN"))
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, ""
	}
	host := u.Hostname()
	if host == "" || strings.ContainsAny(host, " /\\") || (u.Scheme != "https" && !(u.Scheme == "http" && (host == "localhost" || host == "127.0.0.1"))) {
		return nil, ""
	}
	origin := u.Scheme + "://" + u.Host
	wa, err := webauthn.New(&webauthn.Config{
		RPID:                   host,
		RPDisplayName:          "FLVX",
		RPOrigins:              []string{origin},
		AuthenticatorSelection: protocol.AuthenticatorSelection{UserVerification: protocol.VerificationRequired, ResidentKey: protocol.ResidentKeyRequirementPreferred},
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: passkeyTTL},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: passkeyTTL},
		},
	})
	if err != nil {
		return nil, ""
	}
	return wa, origin
}

func (h *Handler) passkeyStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	wa, _ := passkeyConfig()
	response.WriteJSON(w, response.OK(map[string]bool{"enabled": wa != nil}))
}

func (h *Handler) putPasskeyCeremony(c passkeyCeremony) (string, bool) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", false
	}
	id := base64.RawURLEncoding.EncodeToString(buf)
	h.passkeyMu.Lock()
	defer h.passkeyMu.Unlock()
	for k, v := range h.passkeyPending {
		if time.Now().After(v.session.Expires) {
			delete(h.passkeyPending, k)
		}
	}
	if len(h.passkeyPending) >= 1000 {
		return "", false
	}
	h.passkeyPending[id] = c
	return id, true
}

func (h *Handler) takePasskeyCeremony(id, kind, origin string, userID int64) (webauthn.SessionData, bool) {
	h.passkeyMu.Lock()
	c, ok := h.passkeyPending[id]
	delete(h.passkeyPending, id)
	h.passkeyMu.Unlock()
	if !ok || c.kind != kind || c.origin != origin || c.userID != userID || time.Now().After(c.session.Expires) {
		return webauthn.SessionData{}, false
	}
	return c.session, true
}

func (h *Handler) loadPasskeyUser(userID int64) (passkeyUser, error) {
	user, err := h.repo.GetUserByID(userID)
	if err != nil || user == nil || user.Status != 1 {
		return passkeyUser{}, errInvalidPasskey
	}
	rows, err := h.repo.ListPasskeys(userID)
	if err != nil {
		return passkeyUser{}, err
	}
	u := passkeyUser{id: user.ID, name: user.User}
	for _, row := range rows {
		var credential webauthn.Credential
		if err := json.Unmarshal([]byte(row.CredentialJSON), &credential); err != nil {
			return passkeyUser{}, err
		}
		u.credentials = append(u.credentials, credential)
	}
	return u, nil
}

var errInvalidPasskey = &passkeyError{}

type passkeyError struct{}

func (*passkeyError) Error() string { return "invalid passkey user" }

func passkeyBody(r *http.Request, out interface{}) bool {
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 64*1024)).Decode(out) == nil
}

func (h *Handler) passkeyRegisterBegin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	wa, origin := passkeyConfig()
	if wa == nil {
		response.WriteJSON(w, response.ErrDefault("通行证密钥未配置"))
		return
	}
	userID, err := userIDFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "未登录或token已过期"))
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if !passkeyBody(r, &req) {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	user, err := h.repo.GetUserByID(userID)
	if err != nil || user == nil || user.Status != 1 {
		response.WriteJSON(w, response.ErrDefault("账号不可用"))
		return
	}
	if ok, _ := security.VerifyPassword(user.Pwd, req.Password); !ok {
		response.WriteJSON(w, response.ErrDefault("当前密码错误"))
		return
	}
	u, err := h.loadPasskeyUser(userID)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("无法读取通行证密钥"))
		return
	}
	options, session, err := wa.BeginRegistration(u)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("无法创建通行证密钥挑战"))
		return
	}
	id, ok := h.putPasskeyCeremony(passkeyCeremony{userID: userID, kind: "register", origin: origin, session: *session})
	if !ok {
		response.WriteJSON(w, response.ErrDefault("无法创建通行证密钥挑战"))
		return
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{"sessionId": id, "options": options}))
}

type passkeyFinishRequest struct {
	SessionID  string          `json:"sessionId"`
	Credential json.RawMessage `json:"credential"`
	Name       string          `json:"name"`
}

func (h *Handler) passkeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	wa, origin := passkeyConfig()
	if wa == nil {
		response.WriteJSON(w, response.ErrDefault("通行证密钥未配置"))
		return
	}
	userID, err := userIDFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "未登录或token已过期"))
		return
	}
	var req passkeyFinishRequest
	if !passkeyBody(r, &req) {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	session, ok := h.takePasskeyCeremony(req.SessionID, "register", origin, userID)
	if !ok {
		response.WriteJSON(w, response.ErrDefault("通行证密钥挑战已过期"))
		return
	}
	u, err := h.loadPasskeyUser(userID)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("账号不可用"))
		return
	}
	credential, err := wa.FinishRegistration(u, session, credentialRequest(r, req.Credential))
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("通行证密钥验证失败"))
		return
	}
	credentialJSON, err := json.Marshal(credential)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("通行证密钥保存失败"))
		return
	}
	name := strings.TrimSpace(req.Name)
	if len([]rune(name)) > 100 {
		name = string([]rune(name)[:100])
	}
	if name == "" {
		name = "Passkey"
	}
	err = h.repo.CreatePasskey(&model.Passkey{ID: base64.RawURLEncoding.EncodeToString(credential.ID), UserID: userID, Name: name, CredentialJSON: string(credentialJSON), CreatedAt: time.Now().UnixMilli()})
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("通行证密钥保存失败"))
		return
	}
	response.WriteJSON(w, response.OK(nil))
}

func credentialRequest(original *http.Request, body []byte) *http.Request {
	r := original.Clone(original.Context())
	r.Body = http.NoBody
	if len(body) > 0 {
		r.Body = ioNopCloser{bytes.NewReader(body)}
	}
	return r
}

type ioNopCloser struct{ *bytes.Reader }

func (ioNopCloser) Close() error { return nil }

func (h *Handler) passkeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	wa, origin := passkeyConfig()
	if wa == nil {
		response.WriteJSON(w, response.ErrDefault("通行证密钥未配置"))
		return
	}
	var req struct {
		Username string `json:"username"`
	}
	if !passkeyBody(r, &req) {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	user, err := h.repo.GetUserByUsername(strings.TrimSpace(req.Username))
	if err != nil || user == nil || user.Status != 1 {
		response.WriteJSON(w, response.ErrDefault("无法使用通行证密钥登录"))
		return
	}
	u, err := h.loadPasskeyUser(user.ID)
	if err != nil || len(u.credentials) == 0 {
		response.WriteJSON(w, response.ErrDefault("无法使用通行证密钥登录"))
		return
	}
	options, session, err := wa.BeginLogin(u, webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("无法创建通行证密钥挑战"))
		return
	}
	id, ok := h.putPasskeyCeremony(passkeyCeremony{userID: user.ID, kind: "login", origin: origin, session: *session})
	if !ok {
		response.WriteJSON(w, response.ErrDefault("无法创建通行证密钥挑战"))
		return
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{"sessionId": id, "options": options}))
}

func (h *Handler) passkeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	wa, origin := passkeyConfig()
	if wa == nil {
		response.WriteJSON(w, response.ErrDefault("通行证密钥未配置"))
		return
	}
	var req passkeyFinishRequest
	if !passkeyBody(r, &req) {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	// Login sessions carry the user ID; the caller cannot choose a different account at finish.
	h.passkeyMu.Lock()
	pending := h.passkeyPending[req.SessionID]
	h.passkeyMu.Unlock()
	session, ok := h.takePasskeyCeremony(req.SessionID, "login", origin, pending.userID)
	if !ok {
		response.WriteJSON(w, response.ErrDefault("通行证密钥挑战已过期"))
		return
	}
	u, err := h.loadPasskeyUser(pending.userID)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("账号不可用"))
		return
	}
	credential, err := wa.FinishLogin(u, session, credentialRequest(r, req.Credential))
	if err != nil || credential.Authenticator.CloneWarning {
		response.WriteJSON(w, response.ErrDefault("通行证密钥验证失败"))
		return
	}
	credentialID := base64.RawURLEncoding.EncodeToString(credential.ID)
	stored, err := h.repo.GetPasskey(u.id, credentialID)
	if err != nil || stored == nil {
		response.WriteJSON(w, response.ErrDefault("通行证密钥验证失败"))
		return
	}
	updated, err := json.Marshal(credential)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("通行证密钥验证失败"))
		return
	}
	ok, err = h.repo.UpdatePasskeyCredential(u.id, credentialID, stored.CredentialJSON, string(updated), time.Now().UnixMilli())
	if err != nil || !ok {
		response.WriteJSON(w, response.ErrDefault("通行证密钥验证失败"))
		return
	}
	user, err := h.repo.GetUserByID(u.id)
	if err != nil || user == nil || user.Status != 1 {
		response.WriteJSON(w, response.ErrDefault("账号不可用"))
		return
	}
	token, err := auth.GenerateTokenAt(user.ID, user.User, user.RoleID, h.jwtSecret, time.Now())
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("登录失败"))
		return
	}
	response.WriteJSON(w, response.OK(map[string]interface{}{"token": token, "name": user.User, "role_id": user.RoleID, "requirePasswordChange": user.User == "admin_user"}))
}

func (h *Handler) passkeyList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	userID, err := userIDFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "未登录或token已过期"))
		return
	}
	rows, err := h.repo.ListPasskeys(userID)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("获取通行证密钥失败"))
		return
	}
	out := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]interface{}{"id": row.ID, "name": row.Name, "createdAt": row.CreatedAt, "lastUsedAt": row.LastUsedAt})
	}
	response.WriteJSON(w, response.OK(out))
}

func (h *Handler) passkeyDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	userID, err := userIDFromRequest(r)
	if err != nil {
		response.WriteJSON(w, response.Err(401, "未登录或token已过期"))
		return
	}
	var req struct {
		ID       string `json:"id"`
		Password string `json:"password"`
	}
	if !passkeyBody(r, &req) || req.ID == "" {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	user, err := h.repo.GetUserByID(userID)
	if err != nil || user == nil || user.Status != 1 {
		response.WriteJSON(w, response.ErrDefault("账号不可用"))
		return
	}
	if ok, _ := security.VerifyPassword(user.Pwd, req.Password); !ok {
		response.WriteJSON(w, response.ErrDefault("当前密码错误"))
		return
	}
	deleted, err := h.repo.DeletePasskey(userID, req.ID)
	if err != nil || !deleted {
		response.WriteJSON(w, response.ErrDefault("通行证密钥不存在"))
		return
	}
	response.WriteJSON(w, response.OK(nil))
}
