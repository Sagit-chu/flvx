package handler

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"go-backend/internal/http/response"
	"go-backend/internal/license"
)

const (
	maxOfficialComposeBytes = 2 << 20
)

type localLicenseUpdateRunRequest struct {
	Version string `json:"version"`
	Channel string `json:"channel"`
}

type localLicenseUpdateData struct {
	CurrentVersion string                        `json:"currentVersion"`
	LatestVersion  string                        `json:"latestVersion"`
	HasUpdate      bool                          `json:"hasUpdate"`
	Channel        string                        `json:"channel"`
	Force          bool                          `json:"force"`
	AllowRollback  bool                          `json:"allowRollback"`
	ReleaseNotes   string                        `json:"releaseNotes"`
	ManifestURL    string                        `json:"manifestUrl"`
	Capability     systemUpgradeCapabilityData   `json:"capability"`
	Manifest       license.UpdateManifest        `json:"manifest,omitempty"`
	Artifacts      []license.UpdateManifestAsset `json:"artifacts,omitempty"`
	Reason         string                        `json:"reason,omitempty"`
}

type localLicenseUpdateRunData struct {
	Version         string `json:"version"`
	Channel         string `json:"channel"`
	ComposeAsset    string `json:"composeAsset"`
	HelperContainer string `json:"helperContainer"`
	BackendImageID  string `json:"backendImageId"`
	Message         string `json:"message"`
}

type localLicenseUpdateLogData struct {
	Log       string `json:"log"`
	DeployDir string `json:"deployDir"`
	LogPath   string `json:"logPath"`
}

func (h *Handler) localLicenseUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	data, err := h.checkOfficialUpdate(r.Context(), "")
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(data))
}

func (h *Handler) localLicenseUpdateRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.systemUpgradeMu.TryLock() {
		response.WriteJSON(w, response.ErrDefault(systemUpgradeConflictError))
		return
	}
	defer h.systemUpgradeMu.Unlock()

	var req localLicenseUpdateRunRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}
	cfg, status, err := h.requireLocalUpdateLicense()
	if err != nil {
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	channel := strings.TrimSpace(req.Channel)
	if channel == "" {
		channel = status.VersionChannel
	}
	check, err := h.checkOfficialUpdate(r.Context(), channel)
	if err != nil {
		_ = h.reportOfficialUpdate(cfg.CenterURL, cfg.LicenseKey, currentPanelVersion(), req.Version, "failed", err.Error())
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	if !check.HasUpdate && strings.TrimSpace(req.Version) == "" {
		response.WriteJSON(w, response.ErrDefault("当前已是最新版本"))
		return
	}
	manifest, err := h.resolveOfficialUpdateManifest(check, cfg.PublicKey)
	if err != nil {
		_ = h.reportOfficialUpdate(cfg.CenterURL, cfg.LicenseKey, currentPanelVersion(), check.LatestVersion, "failed", err.Error())
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	if strings.TrimSpace(req.Version) != "" && manifest.Version != strings.TrimSpace(req.Version) {
		response.WriteJSON(w, response.ErrDefault("指定版本与官方更新清单不一致"))
		return
	}
	result, err := h.executeOfficialUpdate(r.Context(), cfg.CenterURL, cfg.LicenseKey, manifest)
	if err != nil {
		_ = h.reportOfficialUpdate(cfg.CenterURL, cfg.LicenseKey, currentPanelVersion(), manifest.Version, "failed", err.Error())
		response.WriteJSON(w, response.ErrDefault(err.Error()))
		return
	}
	response.WriteJSON(w, response.OK(result))
}

func (h *Handler) localLicenseUpdateLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	exec := newSystemUpgradeExecutor()
	logPath := filepath.Join(exec.deployDir, "upgrade.log")
	raw, err := os.ReadFile(logPath)
	if err != nil && !os.IsNotExist(err) {
		response.WriteJSON(w, response.ErrDefault("读取升级日志失败: "+err.Error()))
		return
	}
	if len(raw) > 200*1024 {
		raw = raw[len(raw)-200*1024:]
	}
	response.WriteJSON(w, response.OK(localLicenseUpdateLogData{
		Log: string(raw), DeployDir: exec.deployDir, LogPath: logPath,
	}))
}

func (h *Handler) checkOfficialUpdate(ctx context.Context, channel string) (localLicenseUpdateData, error) {
	cfg, status, err := h.requireLocalUpdateLicense()
	if err != nil {
		return localLicenseUpdateData{}, err
	}
	exec := newSystemUpgradeExecutor()
	capability := exec.capability(ctx)
	currentVersion := currentPanelVersion()
	if channel == "" {
		channel = status.VersionChannel
	}
	if channel == "" {
		channel = "stable"
	}
	check, err := license.CheckUpdate(cfg.CenterURL, license.UpdateCheckRequest{
		LicenseKey:     cfg.LicenseKey,
		CurrentVersion: currentVersion,
		Channel:        channel,
		Domain:         localHostname(),
		IPv4:           firstOutboundIP(false),
		IPv6:           firstOutboundIP(true),
		InstanceID:     cfg.InstanceID,
		DeployDir:      exec.deployDir,
		Arch:           runtime.GOARCH,
		ComposeType:    detectLocalComposeType(exec.composePath()),
	})
	if err != nil {
		return localLicenseUpdateData{}, err
	}
	if check.Manifest.Version != "" && check.ManifestSignature != "" {
		if err := license.VerifySignedManifest(license.SignedManifestResponse{
			Manifest: check.Manifest, Signature: check.ManifestSignature, PublicKey: check.PublicKey,
		}, cfg.PublicKey); err != nil {
			return localLicenseUpdateData{}, err
		}
	}
	latest := strings.TrimSpace(check.Version)
	if latest == "" {
		latest = check.Manifest.Version
	}
	return localLicenseUpdateData{
		CurrentVersion: currentVersion, LatestVersion: latest, HasUpdate: check.HasUpdate,
		Channel: defaultString(check.Channel, channel), Force: check.Force, AllowRollback: check.AllowRollback,
		ReleaseNotes: check.ReleaseNotes, ManifestURL: check.ManifestURL, Capability: capability,
		Manifest: check.Manifest, Artifacts: check.Manifest.Artifacts, Reason: check.Reason,
	}, nil
}

func (h *Handler) requireLocalUpdateLicense() (cfg licenseClientConfigView, status localLicenseStatus, err error) {
	repoCfg, err := h.repo.GetLicenseClientConfig()
	if err != nil {
		return licenseClientConfigView{}, localLicenseStatus{}, err
	}
	status, err = h.getLocalLicenseStatus(time.Now().UnixMilli())
	if err != nil {
		return licenseClientConfigView{}, localLicenseStatus{}, err
	}
	if !status.Valid {
		reason := strings.TrimSpace(status.Reason)
		if reason == "" {
			reason = "商业授权未激活"
		}
		return licenseClientConfigView{}, localLicenseStatus{}, fmt.Errorf("%s", reason)
	}
	if repoCfg.CenterURL == "" || repoCfg.PublicKey == "" || repoCfg.LicenseKey == "" || repoCfg.InstanceID == "" {
		return licenseClientConfigView{}, localLicenseStatus{}, fmt.Errorf("授权配置不完整")
	}
	return licenseClientConfigView{
		CenterURL: repoCfg.CenterURL, PublicKey: repoCfg.PublicKey, LicenseKey: repoCfg.LicenseKey, InstanceID: repoCfg.InstanceID,
	}, status, nil
}

type licenseClientConfigView struct {
	CenterURL  string
	PublicKey  string
	LicenseKey string
	InstanceID string
}

func (h *Handler) resolveOfficialUpdateManifest(check localLicenseUpdateData, publicKey string) (license.UpdateManifest, error) {
	if strings.TrimSpace(check.ManifestURL) != "" {
		signed, err := license.FetchSignedManifest(check.ManifestURL)
		if err != nil {
			return license.UpdateManifest{}, err
		}
		if err := license.VerifySignedManifest(signed, publicKey); err != nil {
			return license.UpdateManifest{}, err
		}
		return signed.Manifest, nil
	}
	if strings.TrimSpace(check.Manifest.Version) == "" {
		return license.UpdateManifest{}, fmt.Errorf("官方更新清单为空")
	}
	return check.Manifest, nil
}

func (h *Handler) executeOfficialUpdate(ctx context.Context, centerURL, licenseKey string, manifest license.UpdateManifest) (localLicenseUpdateRunData, error) {
	if err := validateUpgradeVersion(manifest.Version); err != nil {
		return localLicenseUpdateRunData{}, err
	}
	exec := newSystemUpgradeExecutor()
	capability := exec.capability(ctx)
	if !capability.Capable {
		return localLicenseUpdateRunData{}, fmt.Errorf("当前环境不支持在线更新: %s", strings.Join(capability.Reasons, "; "))
	}
	composePath := exec.composePath()
	envPath := exec.envPath()
	composeData, err := os.ReadFile(composePath)
	if err != nil {
		return localLicenseUpdateRunData{}, fmt.Errorf("读取 compose 失败: %w", err)
	}
	composeAsset, ok := selectOfficialComposeAsset(manifest.Artifacts, exec.selectComposeAsset(composeData))
	if !ok {
		return localLicenseUpdateRunData{}, fmt.Errorf("当前版本缺少匹配的 compose 文件")
	}
	packageAsset, hasPackage := selectOfficialPackageAsset(manifest.Artifacts)
	if err := h.reportOfficialUpdate(centerURL, licenseKey, currentPanelVersion(), manifest.Version, "downloading", ""); err != nil {
		return localLicenseUpdateRunData{}, err
	}
	tempDir, err := os.MkdirTemp("", "flvx-official-update-*")
	if err != nil {
		return localLicenseUpdateRunData{}, fmt.Errorf("创建升级临时目录失败: %w", err)
	}
	defer os.RemoveAll(tempDir)
	composeDownloadPath := filepath.Join(tempDir, composeAsset.File)
	if err := license.DownloadAndVerifyAsset(composeAsset, composeDownloadPath, maxOfficialComposeBytes); err != nil {
		return localLicenseUpdateRunData{}, err
	}
	if hasPackage {
		packagePath := filepath.Join(tempDir, packageAsset.File)
		if err := license.DownloadAndVerifyAsset(packageAsset, packagePath, 1024<<20); err != nil {
			return localLicenseUpdateRunData{}, err
		}
		if err := buildOfficialPackageImages(ctx, packagePath, tempDir, manifest.Version); err != nil {
			return localLicenseUpdateRunData{}, err
		}
	}
	newCompose, err := os.ReadFile(composeDownloadPath)
	if err != nil {
		return localLicenseUpdateRunData{}, fmt.Errorf("读取升级 compose 失败: %w", err)
	}
	imageID, err := exec.currentBackendImage(ctx)
	if err != nil {
		return localLicenseUpdateRunData{}, err
	}
	if _, err := exec.backupFile(composePath); err != nil {
		return localLicenseUpdateRunData{}, fmt.Errorf("备份 compose 失败: %w", err)
	}
	if _, err := exec.backupFile(envPath); err != nil {
		return localLicenseUpdateRunData{}, fmt.Errorf("备份 .env 失败: %w", err)
	}
	if err := exec.replaceCompose(composePath, newCompose); err != nil {
		if restoreErr := exec.restoreUpgradeBackups(composePath, envPath); restoreErr != nil {
			err = fmt.Errorf("%v; 回滚失败: %v", err, restoreErr)
		}
		return localLicenseUpdateRunData{}, fmt.Errorf("替换 compose 失败: %w", err)
	}
	if err := exec.updateEnvVersion(envPath, manifest.Version); err != nil {
		if restoreErr := exec.restoreUpgradeBackups(composePath, envPath); restoreErr != nil {
			err = fmt.Errorf("%v; 回滚失败: %v", err, restoreErr)
		}
		return localLicenseUpdateRunData{}, fmt.Errorf("更新版本配置失败: %w", err)
	}
	if hasPackage {
		if err := upsertEnvValues(envPath, map[string]string{
			"FLVX_BACKEND_IMAGE":  "local/flvx-panel-backend:" + manifest.Version,
			"FLVX_FRONTEND_IMAGE": "local/flvx-panel-frontend:" + manifest.Version,
		}); err != nil {
			return localLicenseUpdateRunData{}, fmt.Errorf("更新本地镜像配置失败: %w", err)
		}
	}
	if err := h.reportOfficialUpdate(centerURL, licenseKey, currentPanelVersion(), manifest.Version, "upgrading", ""); err != nil {
		return localLicenseUpdateRunData{}, err
	}
	helperName := fmt.Sprintf("flvx-official-update-%d", time.Now().Unix())
	reportURL := buildCenterEndpoint(centerURL, "/api/v1/update/report")
	helperContainer, err := exec.startOfficialUpdateHelper(ctx, imageID, helperName, officialUpdateHelperEnv{
		ReportURL: reportURL, LicenseKey: licenseKey, CurrentVersion: currentPanelVersion(), TargetVersion: manifest.Version,
	})
	if err != nil {
		if restoreErr := exec.restoreUpgradeBackups(composePath, envPath); restoreErr != nil {
			err = fmt.Errorf("%v; 回滚失败: %v", err, restoreErr)
		}
		return localLicenseUpdateRunData{}, err
	}
	return localLicenseUpdateRunData{
		Version: manifest.Version, Channel: manifest.Channel, ComposeAsset: composeAsset.File,
		HelperContainer: helperContainer, BackendImageID: imageID, Message: systemUpgradeMessage,
	}, nil
}

func (h *Handler) reportOfficialUpdate(centerURL, licenseKey, currentVersion, targetVersion, status, message string) error {
	return license.ReportUpdate(centerURL, license.UpdateReportRequest{
		LicenseKey: licenseKey, CurrentVersion: currentVersion, TargetVersion: targetVersion,
		Status: status, ErrorMessage: message,
	})
}

func selectOfficialComposeAsset(assets []license.UpdateManifestAsset, preferred string) (license.UpdateManifestAsset, bool) {
	preferred = strings.ToLower(strings.TrimSpace(preferred))
	for _, asset := range assets {
		if strings.EqualFold(asset.File, preferred) {
			return asset, true
		}
	}
	wantV6 := strings.Contains(preferred, "v6")
	for _, asset := range assets {
		file := strings.ToLower(asset.File)
		typ := strings.ToLower(asset.Type)
		if wantV6 && (strings.Contains(file, "compose-v6") || strings.Contains(file, "docker-compose-v6") || typ == "compose-v6") {
			return asset, true
		}
		if !wantV6 && (strings.Contains(file, "compose-v4") || strings.Contains(file, "docker-compose-v4") || typ == "compose-v4") {
			return asset, true
		}
	}
	for _, asset := range assets {
		if strings.HasSuffix(strings.ToLower(asset.File), ".yml") || strings.Contains(strings.ToLower(asset.Type), "compose") {
			return asset, true
		}
	}
	return license.UpdateManifestAsset{}, false
}

func selectOfficialPackageAsset(assets []license.UpdateManifestAsset) (license.UpdateManifestAsset, bool) {
	for _, asset := range assets {
		file := strings.ToLower(asset.File)
		typ := strings.ToLower(asset.Type)
		if typ == "package" || strings.HasSuffix(file, ".tar.gz") || strings.HasSuffix(file, ".tgz") {
			return asset, true
		}
	}
	return license.UpdateManifestAsset{}, false
}

func buildOfficialPackageImages(ctx context.Context, packagePath, workDir, version string) error {
	sourceDir := filepath.Join(workDir, "source")
	if err := os.MkdirAll(sourceDir, 0o700); err != nil {
		return fmt.Errorf("创建商业包解压目录失败: %w", err)
	}
	if out, err := exec.CommandContext(ctx, "tar", "-xzf", packagePath, "-C", sourceDir).CombinedOutput(); err != nil {
		return fmt.Errorf("解压商业包失败: %v: %s", err, strings.TrimSpace(string(out)))
	}
	backendDir := filepath.Join(sourceDir, "go-backend")
	frontendDir := filepath.Join(sourceDir, "vite-frontend")
	if out, err := exec.CommandContext(ctx, "docker", "build", "-t", "local/flvx-panel-backend:"+version, backendDir).CombinedOutput(); err != nil {
		return fmt.Errorf("构建后端镜像失败: %v: %s", err, strings.TrimSpace(string(out)))
	}
	if out, err := exec.CommandContext(ctx, "docker", "build", "-t", "local/flvx-panel-frontend:"+version, "--build-arg", "VITE_BASE_PATH=/", frontendDir).CombinedOutput(); err != nil {
		return fmt.Errorf("构建前端镜像失败: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func upsertEnvValues(envPath string, values map[string]string) error {
	mode, err := fileModeOrDefault(envPath, 0o600)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(envPath)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	seen := map[string]bool{}
	for i, line := range lines {
		for key, value := range values {
			if strings.HasPrefix(line, key+"=") {
				lines[i] = key + "=" + value
				seen[key] = true
			}
		}
	}
	for key, value := range values {
		if !seen[key] {
			lines = append(lines, key+"="+value)
		}
	}
	return writeFileWithMode(envPath, []byte(strings.Join(lines, "\n")+"\n"), mode)
}

func detectLocalComposeType(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if enableIPv6ComposePattern.Match(raw) {
		return "v6"
	}
	return "v4"
}

func firstOutboundIP(ipv6 bool) string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			isV4 := ip.To4() != nil
			if ipv6 && !isV4 {
				return ip.String()
			}
			if !ipv6 && isV4 {
				return ip.String()
			}
		}
	}
	return ""
}

func buildCenterEndpoint(centerURL, path string) string {
	parsed, err := url.Parse(strings.TrimSpace(centerURL))
	if err != nil {
		return ""
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

type officialUpdateHelperEnv struct {
	ReportURL      string
	LicenseKey     string
	CurrentVersion string
	TargetVersion  string
}

func (e *systemUpgradeExecutor) startOfficialUpdateHelper(ctx context.Context, imageID, helperName string, env officialUpdateHelperEnv) (string, error) {
	if err := validateBackendContainerName(e.backendContainer); err != nil {
		return "", err
	}
	args := []string{
		"run", "-d", "--rm", "--name", helperName,
		"--volumes-from", e.backendContainer,
		"-v", dockerSocketPath + ":" + dockerSocketPath,
		"-e", panelDeployDirEnv + "=" + e.deployDir,
		"-e", "FLVX_UPDATE_REPORT_URL=" + env.ReportURL,
		"-e", "FLVX_UPDATE_LICENSE_KEY=" + env.LicenseKey,
		"-e", "FLVX_UPDATE_CURRENT_VERSION=" + env.CurrentVersion,
		"-e", "FLVX_UPDATE_TARGET_VERSION=" + env.TargetVersion,
		"--entrypoint", "/bin/sh", imageID,
		"-c", e.officialUpdateHelperScript(),
	}
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("启动升级 helper 失败: %v: %s", err, strings.TrimSpace(string(out)))
	}
	containerID := strings.TrimSpace(string(out))
	if containerID == "" {
		containerID = helperName
	}
	return containerID, nil
}

func (e *systemUpgradeExecutor) officialUpdateHelperScript() string {
	return `set -eu
LOGFILE="$PANEL_DEPLOY_DIR/upgrade.log"
log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOGFILE"; }
report() {
  status="$1"
  message="${2:-}"
  [ -z "${FLVX_UPDATE_REPORT_URL:-}" ] && return 0
  payload="{\"licenseKey\":\"${FLVX_UPDATE_LICENSE_KEY:-}\",\"currentVersion\":\"${FLVX_UPDATE_CURRENT_VERSION:-}\",\"targetVersion\":\"${FLVX_UPDATE_TARGET_VERSION:-}\",\"status\":\"$status\",\"errorMessage\":\"$message\"}"
  if command -v curl >/dev/null 2>&1; then
    curl -fsS -X POST -H "Content-Type: application/json" --data "$payload" "$FLVX_UPDATE_REPORT_URL" >/dev/null 2>&1 || true
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- --header="Content-Type: application/json" --post-data="$payload" "$FLVX_UPDATE_REPORT_URL" >/dev/null 2>&1 || true
  fi
}
fail() {
  code="$?"
  log "错误: 升级 helper 执行失败，退出码 $code"
  if [ -f docker-compose.yml.upgrade.bak ]; then cp docker-compose.yml.upgrade.bak docker-compose.yml || true; fi
  if [ -f .env.upgrade.bak ]; then cp .env.upgrade.bak .env || true; fi
  report failed "升级 helper 执行失败"
  exit "$code"
}
trap fail EXIT

cd "$PANEL_DEPLOY_DIR"
echo "" > "$LOGFILE"
log "开始商业包在线更新"
log "工作目录: $(pwd)"
report upgrading ""

if [ ! -f docker-compose.yml ]; then log "错误: docker-compose.yml 不存在"; exit 1; fi
if [ ! -f .env ]; then log "错误: .env 不存在"; exit 1; fi

log "拉取新镜像（本地镜像会跳过拉取）..."
docker compose pull backend frontend >> "$LOGFILE" 2>&1 || log "镜像拉取跳过或失败，将继续使用本地已构建镜像"

log "等待旧容器释放资源..."
sleep 3

log "重启服务..."
docker compose up -d --force-recreate --remove-orphans backend frontend >> "$LOGFILE" 2>&1

trap - EXIT
log "商业包在线更新完成"
report success ""
`
}
