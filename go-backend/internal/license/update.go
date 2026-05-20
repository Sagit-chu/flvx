package license

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type UpdateCheckRequest struct {
	LicenseKey     string `json:"licenseKey"`
	CurrentVersion string `json:"currentVersion"`
	Channel        string `json:"channel"`
	Domain         string `json:"domain"`
	IPv4           string `json:"ipv4"`
	IPv6           string `json:"ipv6"`
	InstanceID     string `json:"instanceId"`
	DockerVersion  string `json:"dockerVersion"`
	ComposeType    string `json:"composeType"`
	DeployDir      string `json:"deployDir"`
	Arch           string `json:"arch"`
}

type UpdateCheckResponse struct {
	HasUpdate         bool           `json:"hasUpdate"`
	Version           string         `json:"version"`
	Channel           string         `json:"channel"`
	Force             bool           `json:"force"`
	AllowRollback     bool           `json:"allowRollback"`
	ReleaseNotes      string         `json:"releaseNotes"`
	ManifestURL       string         `json:"manifestUrl"`
	ManifestSignature string         `json:"manifestSignature"`
	PublicKey         string         `json:"publicKey"`
	Manifest          UpdateManifest `json:"manifest"`
	Reason            string         `json:"reason"`
}

type UpdateReportRequest struct {
	LicenseKey     string `json:"licenseKey"`
	CurrentVersion string `json:"currentVersion"`
	TargetVersion  string `json:"targetVersion"`
	Status         string `json:"status"`
	ErrorMessage   string `json:"errorMessage"`
}

type UpdateManifest struct {
	Version          string                 `json:"version"`
	Channel          string                 `json:"channel"`
	Force            bool                   `json:"force"`
	AllowRollback    bool                   `json:"allowRollback"`
	MinSupported     string                 `json:"minSupportedVersion"`
	ReleaseNotes     string                 `json:"releaseNotes"`
	Artifacts        []UpdateManifestAsset  `json:"artifacts"`
	GeneratedAt      int64                  `json:"generatedAt"`
	OfficialURL      string                 `json:"officialUrl"`
	ReleaseSignature string                 `json:"-"`
	Extra            map[string]interface{} `json:"extra,omitempty"`
}

type UpdateManifestAsset struct {
	ID     int64  `json:"id"`
	Type   string `json:"type"`
	File   string `json:"file"`
	Size   int64  `json:"sizeBytes"`
	SHA256 string `json:"sha256"`
	URL    string `json:"url"`
}

type SignedManifestResponse struct {
	Manifest  UpdateManifest `json:"manifest"`
	Signature string         `json:"signature"`
	PublicKey string         `json:"publicKey"`
}

func CheckUpdate(centerURL string, req UpdateCheckRequest) (UpdateCheckResponse, error) {
	var result UpdateCheckResponse
	if err := postJSON(centerURL, "/api/v1/update/check", req, &result); err != nil {
		return UpdateCheckResponse{}, err
	}
	return result, nil
}

func ReportUpdate(centerURL string, req UpdateReportRequest) error {
	return postJSON(centerURL, "/api/v1/update/report", req, nil)
}

func FetchSignedManifest(manifestURL string) (SignedManifestResponse, error) {
	endpoint, err := validateAbsoluteHTTPURL(manifestURL)
	if err != nil {
		return SignedManifestResponse{}, err
	}
	client := http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(endpoint)
	if err != nil {
		return SignedManifestResponse{}, fmt.Errorf("获取更新清单失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return SignedManifestResponse{}, fmt.Errorf("更新清单返回 HTTP %d", resp.StatusCode)
	}
	var wrapped apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&wrapped); err != nil {
		return SignedManifestResponse{}, fmt.Errorf("更新清单响应格式错误: %w", err)
	}
	if wrapped.Code != 0 {
		if strings.TrimSpace(wrapped.Msg) != "" {
			return SignedManifestResponse{}, errors.New(wrapped.Msg)
		}
		return SignedManifestResponse{}, fmt.Errorf("更新清单返回错误码 %d", wrapped.Code)
	}
	var signed SignedManifestResponse
	if err := json.Unmarshal(wrapped.Data, &signed); err != nil {
		return SignedManifestResponse{}, fmt.Errorf("更新清单数据格式错误: %w", err)
	}
	return signed, nil
}

func VerifySignedManifest(signed SignedManifestResponse, pinnedPublicKey string) error {
	publicKeyValue := strings.TrimSpace(signed.PublicKey)
	if publicKeyValue == "" {
		publicKeyValue = strings.TrimSpace(pinnedPublicKey)
	}
	if publicKeyValue == "" {
		return errors.New("更新清单缺少官方公钥")
	}
	if strings.TrimSpace(pinnedPublicKey) != "" && publicKeyValue != strings.TrimSpace(pinnedPublicKey) {
		return errors.New("更新清单公钥与本地授权公钥不一致")
	}
	publicKey, err := base64.StdEncoding.DecodeString(publicKeyValue)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return errors.New("更新清单公钥格式错误")
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signed.Signature))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("更新清单签名格式错误")
	}
	raw, err := json.Marshal(signed.Manifest)
	if err != nil {
		return fmt.Errorf("更新清单序列化失败: %w", err)
	}
	if !ed25519.Verify(ed25519.PublicKey(publicKey), raw, signature) {
		return errors.New("更新清单签名校验失败")
	}
	return nil
}

func DownloadAndVerifyAsset(asset UpdateManifestAsset, dest string, maxBytes int64) error {
	endpoint, err := validateAbsoluteHTTPURL(asset.URL)
	if err != nil {
		return err
	}
	if strings.TrimSpace(asset.SHA256) == "" {
		return fmt.Errorf("%s 缺少 SHA256", asset.File)
	}
	if maxBytes <= 0 {
		maxBytes = 256 << 20
	}
	client := http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(endpoint)
	if err != nil {
		return fmt.Errorf("下载%s失败: %w", asset.File, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("下载%s返回 HTTP %d: %s", asset.File, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	tmp := dest + ".download"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("创建下载文件失败: %w", err)
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(out, io.TeeReader(io.LimitReader(resp.Body, maxBytes+1), hasher))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("保存%s失败: %w", asset.File, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("保存%s失败: %w", asset.File, closeErr)
	}
	if written > maxBytes {
		_ = os.Remove(tmp)
		return fmt.Errorf("%s 超过允许大小", asset.File)
	}
	actual := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actual, strings.TrimSpace(asset.SHA256)) {
		_ = os.Remove(tmp)
		return fmt.Errorf("%s SHA256 校验失败", asset.File)
	}
	if asset.Size > 0 && written != asset.Size {
		_ = os.Remove(tmp)
		return fmt.Errorf("%s 文件大小不一致", asset.File)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("写入%s失败: %w", asset.File, err)
	}
	return nil
}

func validateAbsoluteHTTPURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("下载地址格式错误")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New("下载地址只支持 HTTP 或 HTTPS")
	}
	return parsed.String(), nil
}
