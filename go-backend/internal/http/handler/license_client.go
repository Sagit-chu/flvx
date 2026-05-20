package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"go-backend/internal/http/response"
	"go-backend/internal/license"
)

type localLicenseActivateRequest struct {
	CenterURL  string `json:"centerUrl"`
	PublicKey  string `json:"publicKey"`
	LicenseKey string `json:"licenseKey"`
}

type localLicenseBootstrapRequest struct {
	CenterURL  string `json:"centerUrl"`
	PublicKey  string `json:"publicKey"`
	LicenseKey string `json:"licenseKey"`
	Token      string `json:"token"`
}

type localLicenseStatus struct {
	license.VerifyResult
	Configured   bool   `json:"configured"`
	CenterURL    string `json:"centerUrl"`
	InstanceID   string `json:"instanceId"`
	LastError    string `json:"lastError"`
	UpdatedAt    int64  `json:"updatedAt"`
	PublicKeySet bool   `json:"publicKeySet"`
}

func (h *Handler) localLicenseStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	status, err := h.getLocalLicenseStatus(time.Now().UnixMilli())
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(status))
}

func (h *Handler) localLicenseActivate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	var req localLicenseActivateRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("授权参数格式错误"))
		return
	}
	centerURL := strings.TrimSpace(req.CenterURL)
	publicKey := strings.TrimSpace(req.PublicKey)
	licenseKey := strings.TrimSpace(req.LicenseKey)
	if centerURL == "" || publicKey == "" || licenseKey == "" {
		response.WriteJSON(w, response.ErrDefault("授权中心地址、公钥和授权码不能为空"))
		return
	}
	status, err := h.activateLocalLicense(centerURL, publicKey, licenseKey)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(status))
}

func (h *Handler) localLicenseBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	expectedToken := strings.TrimSpace(os.Getenv("FLVX_INSTALL_TOKEN"))
	if expectedToken == "" {
		response.WriteJSON(w, response.Err(403, "安装激活通道未启用"))
		return
	}
	var req localLicenseBootstrapRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("授权参数格式错误"))
		return
	}
	if subtle.ConstantTimeCompare([]byte(expectedToken), []byte(strings.TrimSpace(req.Token))) != 1 {
		response.WriteJSON(w, response.Err(403, "安装激活凭证无效"))
		return
	}
	current, err := h.getLocalLicenseStatus(time.Now().UnixMilli())
	if err == nil && current.Valid {
		response.WriteJSON(w, response.OK(current))
		return
	}
	centerURL := strings.TrimSpace(req.CenterURL)
	publicKey := strings.TrimSpace(req.PublicKey)
	licenseKey := strings.TrimSpace(req.LicenseKey)
	if centerURL == "" || publicKey == "" || licenseKey == "" {
		response.WriteJSON(w, response.ErrDefault("授权中心地址、公钥和授权码不能为空"))
		return
	}
	status, err := h.activateLocalLicense(centerURL, publicKey, licenseKey)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(status))
}

func (h *Handler) activateLocalLicense(centerURL, publicKey, licenseKey string) (localLicenseStatus, error) {
	if err := validateLicenseCenterURL(centerURL); err != nil {
		return localLicenseStatus{}, err
	}

	instanceID, err := h.getOrCreateLicenseInstanceID()
	if err != nil {
		return localLicenseStatus{}, errors.New("生成实例 ID 失败")
	}
	fingerprint, label := h.localLicenseFingerprint()
	cert, err := license.Activate(centerURL, license.ActivateRequest{
		LicenseKey: licenseKey, InstanceID: instanceID, Fingerprint: fingerprint,
		FingerprintLabel: label, Hostname: localHostname(), Version: "commercial-v1",
	})
	if err != nil {
		_ = h.repo.SaveLicenseClientError(err.Error(), time.Now().UnixMilli())
		return localLicenseStatus{}, err
	}
	rawCert, err := license.MarshalCertificate(cert)
	if err != nil {
		return localLicenseStatus{}, errors.New("授权凭证保存失败")
	}
	verify, _ := license.VerifyCertificate(rawCert, publicKey, time.Now().UnixMilli())
	if !verify.Valid {
		return localLicenseStatus{}, errors.New(verify.Reason)
	}
	now := time.Now().UnixMilli()
	if err := h.repo.SaveLicenseClientActivation(centerURL, publicKey, licenseKey, rawCert, instanceID, now); err != nil {
		return localLicenseStatus{}, err
	}
	status, _ := h.getLocalLicenseStatus(now)
	return status, nil
}

func (h *Handler) localLicenseHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	status, err := h.syncLocalLicenseHeartbeat(time.Now())
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(status))
}

func (h *Handler) getLocalLicenseStatus(now int64) (localLicenseStatus, error) {
	cfg, err := h.repo.GetLicenseClientConfig()
	if err != nil {
		return localLicenseStatus{}, err
	}
	verify, _ := license.VerifyCertificate(cfg.Certificate, cfg.PublicKey, now)
	return localLicenseStatus{
		VerifyResult: verify, Configured: cfg.LicenseKey != "" && cfg.PublicKey != "",
		CenterURL: cfg.CenterURL, InstanceID: cfg.InstanceID, LastError: cfg.LastError,
		UpdatedAt: cfg.UpdatedAt, PublicKeySet: cfg.PublicKey != "",
	}, nil
}

func (h *Handler) CommercialLicenseAllowed() (bool, string) {
	status, err := h.getLocalLicenseStatus(time.Now().UnixMilli())
	if err != nil {
		return false, "授权状态读取失败"
	}
	if status.Valid {
		return true, ""
	}
	if strings.TrimSpace(status.Reason) != "" {
		return false, status.Reason
	}
	return false, "商业授权未激活"
}

func (h *Handler) syncLocalLicenseHeartbeat(now time.Time) (localLicenseStatus, error) {
	cfg, err := h.repo.GetLicenseClientConfig()
	if err != nil {
		return localLicenseStatus{}, err
	}
	if cfg.CenterURL == "" || cfg.PublicKey == "" || cfg.LicenseKey == "" || cfg.InstanceID == "" {
		return localLicenseStatus{}, errors.New("授权尚未激活")
	}
	if err := validateLicenseCenterURL(cfg.CenterURL); err != nil {
		return localLicenseStatus{}, err
	}
	fingerprint, _ := h.localLicenseFingerprint()
	cert, err := license.Heartbeat(cfg.CenterURL, license.HeartbeatRequest{
		LicenseKey: cfg.LicenseKey, InstanceID: cfg.InstanceID,
		Fingerprint: fingerprint, Version: "commercial-v1",
	})
	if err != nil {
		_ = h.repo.SaveLicenseClientError(err.Error(), now.UnixMilli())
		return h.getLocalLicenseStatus(now.UnixMilli())
	}
	rawCert, err := license.MarshalCertificate(cert)
	if err != nil {
		return localLicenseStatus{}, err
	}
	verify, _ := license.VerifyCertificate(rawCert, cfg.PublicKey, now.UnixMilli())
	if !verify.Valid {
		_ = h.repo.SaveLicenseClientError(verify.Reason, now.UnixMilli())
		return h.getLocalLicenseStatus(now.UnixMilli())
	}
	if err := h.repo.SaveLicenseClientCertificate(rawCert, now.UnixMilli()); err != nil {
		return localLicenseStatus{}, err
	}
	return h.getLocalLicenseStatus(now.UnixMilli())
}

func (h *Handler) getOrCreateLicenseInstanceID() (string, error) {
	cfg, err := h.repo.GetLicenseClientConfig()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(cfg.InstanceID) != "" {
		return strings.TrimSpace(cfg.InstanceID), nil
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	instanceID := "flvx-" + strings.TrimRight(base32.StdEncoding.EncodeToString(raw), "=")
	if err := h.repo.EnsureLicenseInstanceID(instanceID, time.Now().UnixMilli()); err != nil {
		return "", err
	}
	return instanceID, nil
}

func (h *Handler) localLicenseFingerprint() (string, string) {
	parts := []string{
		"hostname=" + localHostname(),
		"os=" + runtime.GOOS,
		"arch=" + runtime.GOARCH,
		"machine=" + readFirstExistingFile("/etc/machine-id", "/var/lib/dbus/machine-id"),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:]), fmt.Sprintf("%s/%s/%s", localHostname(), runtime.GOOS, runtime.GOARCH)
}

func validateLicenseCenterURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("授权中心地址格式错误")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return errors.New("授权中心地址只支持 HTTP 或 HTTPS")
	}
	host := parsed.Hostname()
	if host == "" {
		return errors.New("授权中心地址缺少主机名")
	}
	if os.Getenv("FLVX_LICENSE_ALLOW_PRIVATE_CENTER") == "1" {
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return errors.New("授权中心域名无法解析")
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return errors.New("授权中心地址不能指向内网或本机地址")
		}
	}
	return nil
}

func localHostname() string {
	name, err := os.Hostname()
	if err != nil || strings.TrimSpace(name) == "" {
		return "unknown"
	}
	return strings.TrimSpace(name)
}

func readFirstExistingFile(paths ...string) string {
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err == nil && strings.TrimSpace(string(raw)) != "" {
			return strings.TrimSpace(string(raw))
		}
	}
	return ""
}

func (s localLicenseStatus) MarshalJSON() ([]byte, error) {
	type alias localLicenseStatus
	return json.Marshal(alias(s))
}
