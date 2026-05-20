package license

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	HeartbeatIntervalMs = int64(6 * time.Hour / time.Millisecond)
	GracePeriodMs       = int64(72 * time.Hour / time.Millisecond)
)

type CertificatePayload struct {
	LicenseID        int64                  `json:"licenseId"`
	KeyPrefix        string                 `json:"keyPrefix"`
	Product          string                 `json:"product"`
	Edition          string                 `json:"edition"`
	VersionChannel   string                 `json:"versionChannel"`
	Status           string                 `json:"status"`
	Features         map[string]interface{} `json:"features"`
	ExpiresAt        int64                  `json:"expiresAt"`
	InstanceID       string                 `json:"instanceId"`
	FingerprintHash  string                 `json:"fingerprintHash"`
	IssuedAt         int64                  `json:"issuedAt"`
	NextHeartbeatAt  int64                  `json:"nextHeartbeatAt"`
	GraceUntil       int64                  `json:"graceUntil"`
	MaxActivations   int                    `json:"maxActivations"`
	ActivationID     int64                  `json:"activationId"`
	ActivationStatus string                 `json:"activationStatus"`
}

type Certificate struct {
	Algorithm string             `json:"algorithm"`
	Payload   CertificatePayload `json:"payload"`
	Signature string             `json:"signature"`
	PublicKey string             `json:"publicKey"`
}

type VerifyResult struct {
	Valid           bool                   `json:"valid"`
	State           string                 `json:"state"`
	Reason          string                 `json:"reason"`
	KeyPrefix       string                 `json:"keyPrefix"`
	Product         string                 `json:"product"`
	Edition         string                 `json:"edition"`
	VersionChannel  string                 `json:"versionChannel"`
	Features        map[string]interface{} `json:"features"`
	ExpiresAt       int64                  `json:"expiresAt"`
	IssuedAt        int64                  `json:"issuedAt"`
	NextHeartbeatAt int64                  `json:"nextHeartbeatAt"`
	GraceUntil      int64                  `json:"graceUntil"`
}

type apiResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type ActivateRequest struct {
	LicenseKey       string `json:"licenseKey"`
	InstanceID       string `json:"instanceId"`
	Fingerprint      string `json:"fingerprint"`
	FingerprintLabel string `json:"fingerprintLabel"`
	Hostname         string `json:"hostname"`
	Version          string `json:"version"`
}

type HeartbeatRequest struct {
	LicenseKey  string `json:"licenseKey"`
	InstanceID  string `json:"instanceId"`
	Fingerprint string `json:"fingerprint"`
	Version     string `json:"version"`
}

func Activate(centerURL string, req ActivateRequest) (Certificate, error) {
	var cert Certificate
	if err := postJSON(centerURL, "/api/v1/license/activate", req, &cert); err != nil {
		return Certificate{}, err
	}
	return cert, nil
}

func Heartbeat(centerURL string, req HeartbeatRequest) (Certificate, error) {
	var cert Certificate
	if err := postJSON(centerURL, "/api/v1/license/heartbeat", req, &cert); err != nil {
		return Certificate{}, err
	}
	return cert, nil
}

func VerifyCertificate(rawCert, pinnedPublicKey string, now int64) (VerifyResult, Certificate) {
	result := VerifyResult{Valid: false, State: "inactive", Reason: "未激活"}
	rawCert = strings.TrimSpace(rawCert)
	if rawCert == "" {
		return result, Certificate{}
	}
	pinnedPublicKey = strings.TrimSpace(pinnedPublicKey)
	if pinnedPublicKey == "" {
		result.State = "invalid"
		result.Reason = "未配置官方公钥"
		return result, Certificate{}
	}

	var cert Certificate
	if err := json.Unmarshal([]byte(rawCert), &cert); err != nil {
		result.State = "invalid"
		result.Reason = "授权凭证格式错误"
		return result, Certificate{}
	}
	if cert.Algorithm != "Ed25519" {
		result.State = "invalid"
		result.Reason = "授权签名算法不支持"
		return result, cert
	}
	if cert.PublicKey != "" && strings.TrimSpace(cert.PublicKey) != pinnedPublicKey {
		result.State = "invalid"
		result.Reason = "授权公钥不匹配"
		return result, cert
	}
	publicKey, err := base64.StdEncoding.DecodeString(pinnedPublicKey)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		result.State = "invalid"
		result.Reason = "官方公钥格式错误"
		return result, cert
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cert.Signature))
	if err != nil || len(signature) != ed25519.SignatureSize {
		result.State = "invalid"
		result.Reason = "授权签名格式错误"
		return result, cert
	}
	payloadBytes, err := json.Marshal(cert.Payload)
	if err != nil {
		result.State = "invalid"
		result.Reason = "授权载荷格式错误"
		return result, cert
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), payloadBytes, signature) {
		result.State = "invalid"
		result.Reason = "授权签名校验失败"
		return result, cert
	}

	result = VerifyResult{
		Valid: true, State: "active", Reason: "", KeyPrefix: cert.Payload.KeyPrefix,
		Product: cert.Payload.Product, Edition: cert.Payload.Edition,
		VersionChannel: cert.Payload.VersionChannel, Features: cert.Payload.Features,
		ExpiresAt: cert.Payload.ExpiresAt, IssuedAt: cert.Payload.IssuedAt,
		NextHeartbeatAt: cert.Payload.NextHeartbeatAt, GraceUntil: cert.Payload.GraceUntil,
	}
	if cert.Payload.Status != "active" || cert.Payload.ActivationStatus != "active" {
		result.Valid = false
		result.State = "disabled"
		result.Reason = "授权已暂停或实例已解绑"
		return result, cert
	}
	if cert.Payload.ExpiresAt > 0 && cert.Payload.ExpiresAt <= now {
		result.Valid = false
		result.State = "expired"
		result.Reason = "授权已到期"
		return result, cert
	}
	if cert.Payload.GraceUntil > 0 && now > cert.Payload.GraceUntil {
		result.Valid = false
		result.State = "expired_grace"
		result.Reason = "授权心跳宽限期已过"
		return result, cert
	}
	if cert.Payload.NextHeartbeatAt > 0 && now > cert.Payload.NextHeartbeatAt {
		result.State = "grace"
		result.Reason = "授权心跳等待同步"
	}
	return result, cert
}

func MarshalCertificate(cert Certificate) (string, error) {
	raw, err := json.Marshal(cert)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func postJSON(centerURL, path string, payload interface{}, out interface{}) error {
	endpoint, err := buildEndpoint(centerURL, path)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	client := http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(endpoint, "application/json", bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("连接授权中心失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("授权中心返回 HTTP %d", resp.StatusCode)
	}
	var wrapped apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&wrapped); err != nil {
		return fmt.Errorf("授权中心响应格式错误: %w", err)
	}
	if wrapped.Code != 0 {
		if strings.TrimSpace(wrapped.Msg) != "" {
			return errors.New(wrapped.Msg)
		}
		return fmt.Errorf("授权中心返回错误码 %d", wrapped.Code)
	}
	if out != nil {
		if err := json.Unmarshal(wrapped.Data, out); err != nil {
			return fmt.Errorf("授权中心数据格式错误: %w", err)
		}
	}
	return nil
}

func buildEndpoint(centerURL, path string) (string, error) {
	centerURL = strings.TrimSpace(centerURL)
	if centerURL == "" {
		return "", errors.New("授权中心地址不能为空")
	}
	parsed, err := url.Parse(centerURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("授权中心地址格式错误")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New("授权中心地址只支持 HTTP 或 HTTPS")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
