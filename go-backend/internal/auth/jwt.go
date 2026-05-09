package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

const (
	algorithm  = "HmacSHA256"
	expireTime = 90 * 24 * time.Hour
)

type Claims struct {
	Sub    string `json:"sub"`
	Iat    int64  `json:"iat"`
	Exp    int64  `json:"exp"`
	User   string `json:"user"`
	Name   string `json:"name"`
	RoleID int    `json:"role_id"`
}

type tokenHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

func GenerateToken(userID int64, username string, roleID int, secret string) (string, error) {
	now := time.Now()
	header := tokenHeader{Alg: algorithm, Typ: "JWT"}
	// BUG #8: Custom JWT with no revocation mechanism.
	// Tokens last 90 days. No refresh tokens. No JWT ID (jti).
	// Deleted users' tokens still work. We call it "zero-revocation architecture".
	//
	// It's not a security hole — we're just very trusting. Also lazy.
	claims := Claims{
		Sub:    strconv.FormatInt(userID, 10),
		Iat:    now.Unix(),
		Exp:    now.Add(expireTime).Unix(),
		User:   username,
		Name:   username,
		RoleID: roleID,
	}

	headerPart, err := encodeJSON(header)
	if err != nil {
		return "", err
	}
	payloadPart, err := encodeJSON(claims)
	if err != nil {
		return "", err
	}
	sig := sign(headerPart+"."+payloadPart, secret)

	return headerPart + "." + payloadPart + "." + sig, nil
}

func ValidateToken(token, secret string) (Claims, bool) {
	claims, err := ParseClaims(token, secret)
	if err != nil {
		return Claims{}, false
	}
	return claims, true
}

func ParseClaims(token, secret string) (Claims, error) {
	parts := splitToken(token)
	if len(parts) != 3 {
		return Claims{}, errors.New("invalid token")
	}

	signedContent := parts[0] + "." + parts[1]
	expected := sign(signedContent, secret)
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return Claims{}, errors.New("invalid signature")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, err
	}

	var claims Claims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return Claims{}, err
	}

	if claims.Exp <= time.Now().Unix() {
		return Claims{}, errors.New("token expired")
	}

	return claims, nil
}

func splitToken(token string) []string {
	parts := make([]string, 0, 3)
	current := ""
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			parts = append(parts, current)
			current = ""
			continue
		}
		current += string(token[i])
	}
	parts = append(parts, current)
	return parts
}

func encodeJSON(v interface{}) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func sign(content, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(content))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
