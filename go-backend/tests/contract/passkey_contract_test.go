package contract_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"go-backend/internal/auth"
	"go-backend/internal/http/response"
)

const passkeyTestOrigin = "https://panel.example.test"

func passkeyPost(t *testing.T, router http.Handler, path, token string, body interface{}) response.R {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var out response.R
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return out
}

func passkeyData(t *testing.T, out response.R) map[string]interface{} {
	t.Helper()
	if out.Code != 0 {
		t.Fatalf("unexpected response: code=%d msg=%s", out.Code, out.Msg)
	}
	return out.Data.(map[string]interface{})
}

func passkeyChallenge(t *testing.T, data map[string]interface{}) string {
	t.Helper()
	options := data["options"].(map[string]interface{})
	return options["publicKey"].(map[string]interface{})["challenge"].(string)
}

func TestPasskeyConfigurationFailsClosedAndPasswordLoginStillWorks(t *testing.T) {
	t.Setenv("FLVX_WEBAUTHN_ORIGIN", "https://panel.example.test/path")
	router, r := setupContractRouter(t, "passkey-test-secret")
	seedLegacyUser(t, r, 9301, "passkey-disabled", "test-password")
	status := passkeyData(t, passkeyPost(t, router, "/api/v1/user/passkey/status", "", map[string]string{}))
	if status["enabled"] != false {
		t.Fatalf("invalid origin enabled passkeys: %v", status)
	}
	if result := passkeyPost(t, router, "/api/v1/user/passkey/login/begin", "", map[string]string{}); result.Code == 0 {
		t.Fatal("login begin succeeded with invalid origin")
	}
	if result := passkeyPost(t, router, "/api/v1/user/login", "", map[string]string{"username": "passkey-disabled", "password": "test-password"}); result.Code != 0 {
		t.Fatalf("password login regressed: %s", result.Msg)
	}
}

func TestPasskeyRegistrationAndOwnership(t *testing.T) {
	t.Setenv("FLVX_WEBAUTHN_ORIGIN", passkeyTestOrigin)
	router, r := setupContractRouter(t, "passkey-test-secret")
	seedLegacyUser(t, r, 9302, "passkey-alice", "alice-password")
	seedLegacyUser(t, r, 9303, "passkey-bob", "bob-password")
	aliceToken, _ := auth.GenerateToken(9302, "passkey-alice", 1, "passkey-test-secret")
	bobToken, _ := auth.GenerateToken(9303, "passkey-bob", 1, "passkey-test-secret")

	if result := passkeyPost(t, router, "/api/v1/user/passkey/register/begin", "", map[string]string{"password": "alice-password"}); result.Code == 0 {
		t.Fatal("unauthenticated registration was allowed")
	}
	if result := passkeyPost(t, router, "/api/v1/user/passkey/register/begin", aliceToken, map[string]string{"password": "wrong"}); result.Code == 0 {
		t.Fatal("registration did not require password re-verification")
	}
	begin := passkeyData(t, passkeyPost(t, router, "/api/v1/user/passkey/register/begin", aliceToken, map[string]string{"password": "alice-password"}))
	if result := passkeyPost(t, router, "/api/v1/user/passkey/register/finish", bobToken, map[string]interface{}{"sessionId": begin["sessionId"], "credential": map[string]string{}}); result.Code == 0 {
		t.Fatal("another user completed Alice's registration")
	}
	// The failed cross-user attempt consumes the challenge.
	begin = passkeyData(t, passkeyPost(t, router, "/api/v1/user/passkey/register/begin", aliceToken, map[string]string{"password": "alice-password"}))
	selection := begin["options"].(map[string]interface{})["publicKey"].(map[string]interface{})["authenticatorSelection"].(map[string]interface{})
	if selection["residentKey"] != "required" || selection["requireResidentKey"] != true {
		t.Fatalf("registration did not require a discoverable credential: %v", selection)
	}
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	credentialID := make([]byte, 32)
	if _, err := rand.Read(credentialID); err != nil {
		t.Fatal(err)
	}
	registerResponse := makeRegistrationResponse(t, privateKey, credentialID, passkeyChallenge(t, begin), passkeyTestOrigin)
	finishBody := map[string]interface{}{"sessionId": begin["sessionId"], "credential": registerResponse, "name": "Laptop"}
	if result := passkeyPost(t, router, "/api/v1/user/passkey/register/finish", aliceToken, finishBody); result.Code != 0 {
		t.Fatalf("register: %s", result.Msg)
	}
	if result := passkeyPost(t, router, "/api/v1/user/passkey/register/finish", aliceToken, finishBody); result.Code == 0 {
		t.Fatal("registration challenge was reusable")
	}
	keyID := base64.RawURLEncoding.EncodeToString(credentialID)
	if items := passkeyPost(t, router, "/api/v1/user/passkey/list", bobToken, map[string]string{}).Data.([]interface{}); len(items) != 0 {
		t.Fatal("Alice's credential appeared in Bob's list")
	}
	if result := passkeyPost(t, router, "/api/v1/user/passkey/delete", bobToken, map[string]string{"id": keyID, "password": "bob-password"}); result.Code == 0 {
		t.Fatal("Bob deleted Alice's credential")
	}
	if result := passkeyPost(t, router, "/api/v1/user/passkey/delete", aliceToken, map[string]string{"id": keyID, "password": "wrong"}); result.Code == 0 {
		t.Fatal("delete did not require password re-verification")
	}
	if result := passkeyPost(t, router, "/api/v1/user/passkey/delete", aliceToken, map[string]string{"id": keyID, "password": "alice-password"}); result.Code != 0 {
		t.Fatalf("delete: %s", result.Msg)
	}
	loginBegin := passkeyData(t, passkeyPost(t, router, "/api/v1/user/passkey/login/begin", "", map[string]string{}))
	aliceHandle := make([]byte, 8)
	binary.BigEndian.PutUint64(aliceHandle, 9302)
	assertion := makeAssertionResponseWithHandle(t, privateKey, credentialID, passkeyChallenge(t, loginBegin), passkeyTestOrigin, "panel.example.test", true, 1, aliceHandle)
	if result := passkeyPost(t, router, "/api/v1/user/passkey/login/finish", "", map[string]interface{}{"sessionId": loginBegin["sessionId"], "credential": assertion}); result.Code == 0 {
		t.Fatal("deleted credential could still complete login")
	}
	// Deletion must not prevent the same account from binding a new key.
	rebind := passkeyData(t, passkeyPost(t, router, "/api/v1/user/passkey/register/begin", aliceToken, map[string]string{"password": "alice-password"}))
	newID := make([]byte, 32)
	if _, err := rand.Read(newID); err != nil {
		t.Fatal(err)
	}
	newResponse := makeRegistrationResponse(t, privateKey, newID, passkeyChallenge(t, rebind), passkeyTestOrigin)
	if result := passkeyPost(t, router, "/api/v1/user/passkey/register/finish", aliceToken, map[string]interface{}{"sessionId": rebind["sessionId"], "credential": newResponse}); result.Code != 0 {
		t.Fatalf("rebind after delete: %s", result.Msg)
	}
	if result := passkeyPost(t, router, "/api/v1/user/login", "", map[string]string{"username": "passkey-alice", "password": "alice-password"}); result.Code != 0 {
		t.Fatalf("password login regressed after enabling passkeys: %s", result.Msg)
	}
}

func TestPasskeyDiscoverableLoginAndOwnership(t *testing.T) {
	t.Setenv("FLVX_WEBAUTHN_ORIGIN", passkeyTestOrigin)
	router, r := setupContractRouter(t, "passkey-test-secret")
	seedLegacyUser(t, r, 9311, "discover-alice", "alice-password")
	seedLegacyUser(t, r, 9312, "discover-bob", "bob-password")
	aliceToken, _ := auth.GenerateToken(9311, "discover-alice", 1, "passkey-test-secret")
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	credentialID := make([]byte, 32)
	if _, err := rand.Read(credentialID); err != nil {
		t.Fatal(err)
	}
	register := passkeyData(t, passkeyPost(t, router, "/api/v1/user/passkey/register/begin", aliceToken, map[string]string{"password": "alice-password"}))
	registration := makeRegistrationResponse(t, privateKey, credentialID, passkeyChallenge(t, register), passkeyTestOrigin)
	if result := passkeyPost(t, router, "/api/v1/user/passkey/register/finish", aliceToken, map[string]interface{}{"sessionId": register["sessionId"], "credential": registration}); result.Code != 0 {
		t.Fatalf("register: %s", result.Msg)
	}
	aliceHandle := make([]byte, 8)
	binary.BigEndian.PutUint64(aliceHandle, 9311)
	bobHandle := make([]byte, 8)
	binary.BigEndian.PutUint64(bobHandle, 9312)
	begin := func(body interface{}) map[string]interface{} {
		t.Helper()
		return passkeyData(t, passkeyPost(t, router, "/api/v1/user/passkey/login/begin", "", body))
	}
	if result := passkeyPost(t, router, "/api/v1/user/passkey/login/begin", "", map[string]string{"username": "discover-alice"}); result.Code == 0 {
		t.Fatal("username-based passkey fallback was accepted")
	}
	finish := func(session map[string]interface{}, assertion map[string]interface{}) response.R {
		t.Helper()
		return passkeyPost(t, router, "/api/v1/user/passkey/login/finish", "", map[string]interface{}{"sessionId": session["sessionId"], "credential": assertion})
	}
	assertionFor := func(session map[string]interface{}, handle []byte, origin, rpID string, uv bool, count uint32) map[string]interface{} {
		t.Helper()
		return makeAssertionResponseWithHandle(t, privateKey, credentialID, passkeyChallenge(t, session), origin, rpID, uv, count, handle)
	}

	// No username and no allowCredentials let the authenticator offer an account.
	session := begin(map[string]string{})
	options := session["options"].(map[string]interface{})["publicKey"].(map[string]interface{})
	if list, exists := options["allowCredentials"]; exists && list != nil {
		t.Fatalf("discoverable login unexpectedly restricted credentials: %v", list)
	}
	if options["userVerification"] != "required" {
		t.Fatalf("discoverable login did not require UV: %v", options)
	}
	wrongHandle := assertionFor(session, bobHandle, passkeyTestOrigin, "panel.example.test", true, 1)
	if result := finish(session, wrongHandle); result.Code == 0 {
		t.Fatal("Bob's user handle selected Alice's credential")
	}
	if result := finish(session, wrongHandle); result.Code == 0 {
		t.Fatal("failed discoverable challenge was reusable")
	}

	session = begin(map[string]string{})
	// Older preferred resident-key registrations may have no discoverable userHandle.
	missingHandle := assertionFor(session, nil, passkeyTestOrigin, "panel.example.test", true, 1)
	if result := finish(session, missingHandle); result.Code == 0 {
		t.Fatal("discoverable login accepted a missing user handle")
	}
	session = begin(map[string]string{})
	unknownID := assertionFor(session, aliceHandle, passkeyTestOrigin, "panel.example.test", true, 1)
	unknownID["id"] = base64.RawURLEncoding.EncodeToString([]byte("unknown-credential"))
	unknownID["rawId"] = unknownID["id"]
	if result := finish(session, unknownID); result.Code == 0 {
		t.Fatal("unknown credential ID selected Alice's account")
	}

	for _, test := range []struct {
		name   string
		origin string
		rpID   string
		uv     bool
	}{
		{"origin", "https://wrong.example.test", "panel.example.test", true},
		{"RP ID", passkeyTestOrigin, "wrong.example.test", true},
		{"user verification", passkeyTestOrigin, "panel.example.test", false},
	} {
		session := begin(map[string]string{})
		assertion := assertionFor(session, aliceHandle, test.origin, test.rpID, test.uv, 1)
		if result := finish(session, assertion); result.Code == 0 {
			t.Fatalf("invalid discoverable assertion was accepted: %s", test.name)
		}
	}
	session = begin(map[string]string{})
	badSignature := assertionFor(session, aliceHandle, passkeyTestOrigin, "panel.example.test", true, 1)
	badSignature["response"].(map[string]interface{})["signature"] = base64.RawURLEncoding.EncodeToString([]byte("invalid-signature"))
	if result := finish(session, badSignature); result.Code == 0 {
		t.Fatal("invalid signature was accepted")
	}

	session = begin(map[string]string{})
	valid := assertionFor(session, aliceHandle, passkeyTestOrigin, "panel.example.test", true, 1)
	if got := passkeyData(t, finish(session, valid))["name"]; got != "discover-alice" {
		t.Fatalf("discoverable login selected the wrong account: %v", got)
	}
	if result := finish(session, valid); result.Code == 0 {
		t.Fatal("discoverable assertion was replayed")
	}
	session = begin(map[string]string{})
	reusedCounter := assertionFor(session, aliceHandle, passkeyTestOrigin, "panel.example.test", true, 1)
	if result := finish(session, reusedCounter); result.Code == 0 {
		t.Fatal("discoverable login accepted a reused nonzero signature counter")
	}

	if err := r.DB().Exec("UPDATE user SET status = 0 WHERE id = ?", 9311).Error; err != nil {
		t.Fatal(err)
	}
	session = begin(map[string]string{})
	disabled := assertionFor(session, aliceHandle, passkeyTestOrigin, "panel.example.test", true, 2)
	if result := finish(session, disabled); result.Code == 0 {
		t.Fatal("disabled account logged in with a discoverable credential")
	}
}

func makeRegistrationResponse(t *testing.T, privateKey *ecdsa.PrivateKey, id []byte, challenge, origin string) map[string]interface{} {
	t.Helper()
	pub := privateKey.PublicKey
	cose, err := cbor.Marshal(map[int]interface{}{1: 2, 3: -7, -1: 1, -2: pub.X.FillBytes(make([]byte, 32)), -3: pub.Y.FillBytes(make([]byte, 32))})
	if err != nil {
		t.Fatal(err)
	}
	rpHash := sha256.Sum256([]byte("panel.example.test"))
	authData := append([]byte{}, rpHash[:]...)
	authData = append(authData, 0x45, 0, 0, 0, 0)
	authData = append(authData, make([]byte, 16)...)
	length := make([]byte, 2)
	binary.BigEndian.PutUint16(length, uint16(len(id)))
	authData = append(authData, length...)
	authData = append(authData, id...)
	authData = append(authData, cose...)
	attestation, err := cbor.Marshal(map[string]interface{}{"fmt": "none", "authData": authData, "attStmt": map[string]interface{}{}})
	if err != nil {
		t.Fatal(err)
	}
	client, _ := json.Marshal(map[string]interface{}{"type": "webauthn.create", "challenge": challenge, "origin": origin})
	return map[string]interface{}{"id": base64.RawURLEncoding.EncodeToString(id), "rawId": base64.RawURLEncoding.EncodeToString(id), "type": "public-key", "response": map[string]interface{}{"attestationObject": base64.RawURLEncoding.EncodeToString(attestation), "clientDataJSON": base64.RawURLEncoding.EncodeToString(client)}}
}

func makeAssertionResponse(t *testing.T, privateKey *ecdsa.PrivateKey, id []byte, challenge, origin, rpID string, verified bool, count uint32) map[string]interface{} {
	return makeAssertionResponseWithHandle(t, privateKey, id, challenge, origin, rpID, verified, count, nil)
}

func makeAssertionResponseWithHandle(t *testing.T, privateKey *ecdsa.PrivateKey, id []byte, challenge, origin, rpID string, verified bool, count uint32, handle []byte) map[string]interface{} {
	t.Helper()
	rpHash := sha256.Sum256([]byte(rpID))
	authData := append([]byte{}, rpHash[:]...)
	flags := byte(0x01)
	if verified {
		flags |= 0x04
	}
	authData = append(authData, flags, 0, 0, 0, 0)
	binary.BigEndian.PutUint32(authData[33:37], count)
	client, _ := json.Marshal(map[string]interface{}{"type": "webauthn.get", "challenge": challenge, "origin": origin})
	clientHash := sha256.Sum256(client)
	signed := append(append([]byte{}, authData...), clientHash[:]...)
	hash := sha256.Sum256(signed)
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	var userHandle interface{}
	if handle != nil {
		userHandle = base64.RawURLEncoding.EncodeToString(handle)
	}
	return map[string]interface{}{"id": base64.RawURLEncoding.EncodeToString(id), "rawId": base64.RawURLEncoding.EncodeToString(id), "type": "public-key", "response": map[string]interface{}{"authenticatorData": base64.RawURLEncoding.EncodeToString(authData), "clientDataJSON": base64.RawURLEncoding.EncodeToString(client), "signature": base64.RawURLEncoding.EncodeToString(signature), "userHandle": userHandle}}
}
