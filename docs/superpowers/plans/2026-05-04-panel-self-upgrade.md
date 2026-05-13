# Panel Self-Upgrade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an admin-only “panel self-upgrade” flow that checks FLVX GitHub releases and upgrades the backend/frontend Docker Compose deployment from the settings page.

**Architecture:** Keep the upgrade entrypoint inside `go-backend`, but do not let the backend container rebuild itself directly. The backend updates the mounted deployment files, discovers its current image ID through Docker, and launches a detached helper container from that same image. The helper container inherits the deployment bind mount and Docker socket, then runs the fixed `docker compose pull backend frontend` and `docker compose up -d backend frontend` sequence. The frontend adds a new card to `src/pages/config.tsx` that shows capability state, checks updates, and triggers the upgrade with a confirmation modal.

**Tech Stack:** Go 1.25 / net-http handlers / existing release helpers in `upgrade.go` / Docker CLI via multi-stage Dockerfile copy / React 18 / TypeScript / HeroUI bridge / existing `Network.post` API client.

**Execution Constraints:** Do not create a git commit during execution unless the user explicitly asks for one in that implementation session.

---

## File Map

- Create: `go-backend/internal/http/handler/system_upgrade.go`
  Responsibility: system upgrade data types, capability checks, `.env` updates, compose asset selection, helper-container launch, and HTTP handlers for `/api/v1/system/version`, `/api/v1/system/check-updates`, and `/api/v1/system/upgrade`.
- Create: `go-backend/internal/http/handler/system_upgrade_test.go`
  Responsibility: helper logic unit tests, method-guard tests, and upgrade lock tests.
- Modify: `go-backend/internal/http/handler/handler.go`
  Responsibility: add a dedicated mutex field and register new system upgrade routes.
- Modify: `go-backend/internal/http/handler/upgrade.go`
  Responsibility: reuse existing release-channel helpers and GitHub proxy URL builder from system upgrade code.
- Modify: `go-backend/Dockerfile`
  Responsibility: copy Docker CLI and compose plugin into the runtime image.
- Modify: `docker-compose-v4.yml`
  Responsibility: pass `FLUX_VERSION`, `PANEL_DEPLOY_DIR`, `PANEL_BACKEND_CONTAINER`, mount Docker socket, and mount the deployment directory.
- Modify: `docker-compose-v6.yml`
  Responsibility: same runtime wiring as v4 while preserving IPv6 network config.
- Modify: `vite-frontend/src/api/types.ts`
  Responsibility: add typed response contracts for system upgrade status, release list, and run result.
- Modify: `vite-frontend/src/api/index.ts`
  Responsibility: add `getSystemUpgradeVersion`, `checkSystemUpgrade`, and `runSystemUpgrade` wrappers.
- Modify: `vite-frontend/src/pages/config.tsx`
  Responsibility: add upgrade card, loading state, release list, capability reason, confirmation modal, and upgrade action.

### Task 1: Build the backend helper core with unit tests

**Files:**
- Create: `go-backend/internal/http/handler/system_upgrade.go`
- Create: `go-backend/internal/http/handler/system_upgrade_test.go`

- [ ] **Step 1: Write the failing helper tests**

```go
package handler

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSelectComposeAssetUsesIPv6Template(t *testing.T) {
	exec := &systemUpgradeExecutor{deployDir: "/opt/flvx-panel", backendContainer: "flux-panel-backend"}
	compose := []byte("networks:\n  gost-network:\n    enable_ipv6: true\n")

	if got := exec.selectComposeAsset(compose); got != "docker-compose-v6.yml" {
		t.Fatalf("selectComposeAsset() = %q, want %q", got, "docker-compose-v6.yml")
	}
}

func TestSelectComposeAssetFallsBackToIPv4Template(t *testing.T) {
	exec := &systemUpgradeExecutor{deployDir: "/opt/flvx-panel", backendContainer: "flux-panel-backend"}
	compose := []byte("services:\n  backend:\n    image: test\n")

	if got := exec.selectComposeAsset(compose); got != "docker-compose-v4.yml" {
		t.Fatalf("selectComposeAsset() = %q, want %q", got, "docker-compose-v4.yml")
	}
}

func TestUpdateEnvVersionReplacesExistingValue(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("FLUX_VERSION=2.1.8\nJWT_SECRET=test\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	exec := &systemUpgradeExecutor{deployDir: dir, backendContainer: "flux-panel-backend"}
	if err := exec.updateEnvVersion(envPath, "2.1.9"); err != nil {
		t.Fatalf("updateEnvVersion() error = %v", err)
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	want := "FLUX_VERSION=2.1.9\nJWT_SECRET=test\n"
	if string(data) != want {
		t.Fatalf("env content = %q, want %q", string(data), want)
	}
}

func TestUpdateEnvVersionAppendsMissingValue(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("JWT_SECRET=test\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	exec := &systemUpgradeExecutor{deployDir: dir, backendContainer: "flux-panel-backend"}
	if err := exec.updateEnvVersion(envPath, "2.1.9"); err != nil {
		t.Fatalf("updateEnvVersion() error = %v", err)
	}

	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	want := "JWT_SECRET=test\nFLUX_VERSION=2.1.9\n"
	if string(data) != want {
		t.Fatalf("env content = %q, want %q", string(data), want)
	}
}

func TestValidateBackendContainerNameRejectsUnsafeValue(t *testing.T) {
	if err := validateBackendContainerName("flux-panel-backend;rm -rf /"); err == nil {
		t.Fatal("expected unsafe container name to fail validation")
	}
}

func TestBuildHelperRunArgsUsesDetachedContainer(t *testing.T) {
	exec := &systemUpgradeExecutor{deployDir: "/opt/flvx-panel", backendContainer: "flux-panel-backend"}
	args := exec.buildHelperRunArgs("sha256:abc", "flvx-upgrade-helper")
	want := []string{
		"run", "-d", "--rm", "--name", "flvx-upgrade-helper",
		"--volumes-from", "flux-panel-backend",
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
		"-e", "PANEL_DEPLOY_DIR=/opt/flvx-panel",
		"--entrypoint", "/bin/sh", "sha256:abc",
		"-c", exec.helperScript(),
	}

	if !reflect.DeepEqual(args, want) {
		t.Fatalf("buildHelperRunArgs() = %#v, want %#v", args, want)
	}
}
```

- [ ] **Step 2: Run the focused backend tests to verify they fail**

Run:

```bash
go test ./internal/http/handler -run 'Test(SelectComposeAsset|UpdateEnvVersion|ValidateBackendContainerName|BuildHelperRunArgs)' -count=1
```

Expected: FAIL with errors such as `undefined: systemUpgradeExecutor`, `undefined: validateBackendContainerName`, and `undefined: (*systemUpgradeExecutor).updateEnvVersion`.

- [ ] **Step 3: Write the minimal helper implementation**

Create `go-backend/internal/http/handler/system_upgrade.go` with this initial helper core:

```go
package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	panelDeployDirEnv          = "PANEL_DEPLOY_DIR"
	panelBackendContainerEnv   = "PANEL_BACKEND_CONTAINER"
	defaultPanelDeployDir      = "/opt/flvx-panel"
	defaultPanelBackendName    = "flux-panel-backend"
	dockerSocketPath           = "/var/run/docker.sock"
	systemUpgradeMessage       = "升级 helper 已启动，面板服务将短暂重启"
	systemUpgradeConflictError = "已有面板升级任务执行中"
)

var safeBackendContainerPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

type systemUpgradeExecutor struct {
	deployDir        string
	backendContainer string
}

func newSystemUpgradeExecutor() *systemUpgradeExecutor {
	deployDir := strings.TrimSpace(os.Getenv(panelDeployDirEnv))
	if deployDir == "" {
		deployDir = defaultPanelDeployDir
	}

	backendContainer := strings.TrimSpace(os.Getenv(panelBackendContainerEnv))
	if backendContainer == "" {
		backendContainer = defaultPanelBackendName
	}

	return &systemUpgradeExecutor{
		deployDir:        deployDir,
		backendContainer: backendContainer,
	}
}

func validateBackendContainerName(value string) error {
	if value == "" {
		return fmt.Errorf("backend container name is empty")
	}
	if !safeBackendContainerPattern.MatchString(value) {
		return fmt.Errorf("unsafe backend container name: %s", value)
	}
	return nil
}

func (e *systemUpgradeExecutor) composePath() string {
	return filepath.Join(e.deployDir, "docker-compose.yml")
}

func (e *systemUpgradeExecutor) envPath() string {
	return filepath.Join(e.deployDir, ".env")
}

func (e *systemUpgradeExecutor) selectComposeAsset(current []byte) string {
	if strings.Contains(string(current), "enable_ipv6: true") {
		return "docker-compose-v6.yml"
	}
	return "docker-compose-v4.yml"
}

func (e *systemUpgradeExecutor) helperScript() string {
	return strings.Join([]string{
		"set -eu",
		`cd "$PANEL_DEPLOY_DIR"`,
		"docker compose pull backend frontend",
		"sleep 5",
		"docker compose up -d backend frontend",
	}, "\n")
}

func (e *systemUpgradeExecutor) buildHelperRunArgs(imageID, helperName string) []string {
	return []string{
		"run", "-d", "--rm", "--name", helperName,
		"--volumes-from", e.backendContainer,
		"-v", dockerSocketPath + ":" + dockerSocketPath,
		"-e", panelDeployDirEnv + "=" + e.deployDir,
		"--entrypoint", "/bin/sh",
		imageID,
		"-c", e.helperScript(),
	}
}

func (e *systemUpgradeExecutor) updateEnvVersion(envPath, version string) error {
	data, err := os.ReadFile(envPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	replaced := false
	for i, line := range lines {
		if strings.HasPrefix(line, "FLUX_VERSION=") {
			lines[i] = "FLUX_VERSION=" + version
			replaced = true
		}
	}
	if !replaced {
		trimmed := strings.TrimRight(strings.Join(lines, "\n"), "\n")
		if trimmed == "" {
			trimmed = "FLUX_VERSION=" + version
		} else {
			trimmed += "\nFLUX_VERSION=" + version
		}
		return os.WriteFile(envPath, []byte(trimmed+"\n"), 0o644)
	}

	content := strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
	return os.WriteFile(envPath, []byte(content), 0o644)
}
```

- [ ] **Step 4: Run the focused backend tests again**

Run:

```bash
go test ./internal/http/handler -run 'Test(SelectComposeAsset|UpdateEnvVersion|ValidateBackendContainerName|BuildHelperRunArgs)' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit only if the user explicitly requested a commit**

```bash
git add go-backend/internal/http/handler/system_upgrade.go go-backend/internal/http/handler/system_upgrade_test.go
git commit -m "feat: add panel upgrade helper core"
```

### Task 2: Wire backend handlers, capability checks, and upgrade orchestration

**Files:**
- Modify: `go-backend/internal/http/handler/handler.go`
- Modify: `go-backend/internal/http/handler/upgrade.go`
- Modify: `go-backend/internal/http/handler/system_upgrade.go`
- Modify: `go-backend/internal/http/handler/system_upgrade_test.go`

- [ ] **Step 1: Add failing handler tests for method guards and the upgrade lock**

Append these tests to `go-backend/internal/http/handler/system_upgrade_test.go`:

```go
func TestSystemVersionRejectsWrongMethod(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/version", nil)
	rr := httptest.NewRecorder()

	h.systemVersion(rr, req)

	if !strings.Contains(rr.Body.String(), "请求失败") {
		t.Fatalf("expected wrong-method response, got %s", rr.Body.String())
	}
}

func TestSystemUpgradeRejectsConcurrentRequests(t *testing.T) {
	h := &Handler{}
	h.systemUpgradeMu.Lock()
	defer h.systemUpgradeMu.Unlock()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/upgrade", strings.NewReader(`{"channel":"stable"}`))
	rr := httptest.NewRecorder()

	h.systemUpgrade(rr, req)

	if !strings.Contains(rr.Body.String(), systemUpgradeConflictError) {
		t.Fatalf("expected conflict message, got %s", rr.Body.String())
	}
}
```

- [ ] **Step 2: Run the new backend tests to verify they fail**

Run:

```bash
go test ./internal/http/handler -run 'Test(SystemVersionRejectsWrongMethod|SystemUpgradeRejectsConcurrentRequests)' -count=1
```

Expected: FAIL with errors such as `h.systemVersion undefined`, `h.systemUpgrade undefined`, and `Handler has no field or method systemUpgradeMu`.

- [ ] **Step 3: Implement the system upgrade handlers and orchestration**

Update `go-backend/internal/http/handler/handler.go` so `Handler` has a dedicated system-upgrade mutex and route registration:

```go
type Handler struct {
	repo        *repo.Repository
	jwtSecret   string
	wsServer    *ws.Server
	metrics     *metrics.IngestionService
	healthCheck *health.Checker

	captchaMu     sync.Mutex
	captchaTokens map[string]int64

	jobsMu      sync.Mutex
	jobsCancel  context.CancelFunc
	jobsStarted bool
	jobsWG      sync.WaitGroup

	systemUpgradeMu         sync.Mutex
	upgradeMu               sync.Mutex
	pendingUpgradeRedeploy  map[int64]struct{}
	nodeOnlineRedeployAt    map[int64]time.Time
	nodeOnlineRedeployQueued map[int64]struct{}
	nodeOnlineRedeploying   map[int64]struct{}

	qualityProber *tunnelQualityProber
}

func (h *Handler) Register(mux *http.ServeMux) {
	// existing handlers...
	mux.HandleFunc("/api/v1/system/storage", h.storageSummary)
	mux.HandleFunc("/api/v1/system/version", h.systemVersion)
	mux.HandleFunc("/api/v1/system/check-updates", h.systemCheckUpdates)
	mux.HandleFunc("/api/v1/system/upgrade", h.systemUpgrade)
	// existing handlers...
}
```

Extend `go-backend/internal/http/handler/system_upgrade.go` with the capability/result types and the minimal orchestration functions:

```go
type systemUpgradeCapability struct {
	Capable          bool   `json:"capable"`
	Reason           string `json:"reason,omitempty"`
	DeployDir        string `json:"deployDir,omitempty"`
	ComposeFile      string `json:"composeFile,omitempty"`
	BackendContainer string `json:"backendContainer,omitempty"`
}

type systemUpgradeVersionData struct {
	CurrentVersion   string `json:"currentVersion"`
	Channel          string `json:"channel"`
	LatestVersion    string `json:"latestVersion,omitempty"`
	HasUpdate        bool   `json:"hasUpdate"`
	Capable          bool   `json:"capable"`
	Reason           string `json:"reason,omitempty"`
	DeployDir        string `json:"deployDir,omitempty"`
	ComposeFile      string `json:"composeFile,omitempty"`
	BackendContainer string `json:"backendContainer,omitempty"`
}

type systemUpgradeCheckData struct {
	CurrentVersion   string                 `json:"currentVersion"`
	Channel          string                 `json:"channel"`
	LatestVersion    string                 `json:"latestVersion,omitempty"`
	HasUpdate        bool                   `json:"hasUpdate"`
	Capable          bool                   `json:"capable"`
	Reason           string                 `json:"reason,omitempty"`
	DeployDir        string                 `json:"deployDir,omitempty"`
	ComposeFile      string                 `json:"composeFile,omitempty"`
	BackendContainer string                 `json:"backendContainer,omitempty"`
	Releases         []systemUpgradeReleaseItem `json:"releases"`
}

type systemUpgradeReleaseItem struct {
	Version     string `json:"version"`
	Name        string `json:"name"`
	PublishedAt string `json:"publishedAt"`
	Prerelease  bool   `json:"prerelease"`
	Channel     string `json:"channel"`
}

type systemUpgradeRunData struct {
	Version           string   `json:"version"`
	Message           string   `json:"message"`
	Commands          []string `json:"commands"`
	HelperContainerID string   `json:"helperContainerId,omitempty"`
}

func currentPanelVersion() string {
	version := strings.TrimSpace(os.Getenv("FLUX_VERSION"))
	if version == "" {
		return "dev"
	}
	return version
}

func (e *systemUpgradeExecutor) checkCapability(ctx context.Context) systemUpgradeCapability {
	capability := systemUpgradeCapability{
		DeployDir:        e.deployDir,
		ComposeFile:      e.composePath(),
		BackendContainer: e.backendContainer,
	}

	if e.deployDir == "" || !filepath.IsAbs(e.deployDir) {
		capability.Reason = "PANEL_DEPLOY_DIR 必须是已挂载的绝对路径"
		return capability
	}
	if err := validateBackendContainerName(e.backendContainer); err != nil {
		capability.Reason = err.Error()
		return capability
	}
	if _, err := exec.LookPath("docker"); err != nil {
		capability.Reason = "docker CLI 不可用"
		return capability
	}
	if _, err := os.Stat(dockerSocketPath); err != nil {
		capability.Reason = "未挂载 /var/run/docker.sock"
		return capability
	}
	if _, err := os.Stat(capability.ComposeFile); err != nil {
		capability.Reason = "未找到 docker-compose.yml"
		return capability
	}
	if _, err := os.Stat(e.envPath()); err != nil {
		capability.Reason = "未找到 .env"
		return capability
	}

	cmd := exec.CommandContext(ctx, "docker", "compose", "version")
	if out, err := cmd.CombinedOutput(); err != nil {
		capability.Reason = fmt.Sprintf("docker compose 不可用: %s", strings.TrimSpace(string(out)))
		return capability
	}

	capability.Capable = true
	return capability
}

func (e *systemUpgradeExecutor) backupFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".upgrade.bak", data, 0o644)
}

func (e *systemUpgradeExecutor) replaceComposeFile(composeData []byte) error {
	tmpPath := e.composePath() + ".tmp"
	if err := os.WriteFile(tmpPath, composeData, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, e.composePath())
}

func (e *systemUpgradeExecutor) currentBackendImage(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.Image}}", e.backendContainer)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect backend image failed: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func (e *systemUpgradeExecutor) startHelper(ctx context.Context, imageID string) (string, error) {
	helperName := fmt.Sprintf("flvx-upgrade-helper-%d", time.Now().Unix())
	args := e.buildHelperRunArgs(imageID, helperName)
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("start helper failed: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func (h *Handler) downloadReleaseAsset(version, filename string) ([]byte, error) {
	url := h.buildGithubDownloadURL(version, filename)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download %s failed: %w", filename, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("download %s failed: %d %s", filename, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(resp.Body)
}

func (h *Handler) systemVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	executor := newSystemUpgradeExecutor()
	capability := executor.checkCapability(r.Context())
	latestVersion, _ := resolveLatestReleaseByChannel(releaseChannelStable)
	currentVersion := currentPanelVersion()

	response.WriteJSON(w, response.OK(systemUpgradeVersionData{
		CurrentVersion:   currentVersion,
		Channel:          releaseChannelStable,
		LatestVersion:    latestVersion,
		HasUpdate:        latestVersion != "" && latestVersion != currentVersion,
		Capable:          capability.Capable,
		Reason:           capability.Reason,
		DeployDir:        capability.DeployDir,
		ComposeFile:      capability.ComposeFile,
		BackendContainer: capability.BackendContainer,
	}))
}

func (h *Handler) systemCheckUpdates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req struct {
		Channel string `json:"channel"`
	}
	if err := decodeJSON(r.Body, &req); err != nil && err != io.EOF {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}

	channel := normalizeReleaseChannel(req.Channel)
	releases, err := fetchGitHubReleases(50)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, fmt.Sprintf("获取版本列表失败: %v", err)))
		return
	}

	items := make([]systemUpgradeReleaseItem, 0, len(releases))
	for _, release := range releases {
		if release.Draft {
			continue
		}
		tag := strings.TrimSpace(release.TagName)
		if tag == "" || releaseChannelFromTag(tag) != channel {
			continue
		}
		items = append(items, systemUpgradeReleaseItem{
			Version:     tag,
			Name:        release.Name,
			PublishedAt: release.PublishedAt,
			Prerelease:  channel == releaseChannelDev,
			Channel:     channel,
		})
	}

	executor := newSystemUpgradeExecutor()
	capability := executor.checkCapability(r.Context())
	latestVersion := ""
	if len(items) > 0 {
		latestVersion = items[0].Version
	}
	currentVersion := currentPanelVersion()

	response.WriteJSON(w, response.OK(systemUpgradeCheckData{
		CurrentVersion:   currentVersion,
		Channel:          channel,
		LatestVersion:    latestVersion,
		HasUpdate:        latestVersion != "" && latestVersion != currentVersion,
		Capable:          capability.Capable,
		Reason:           capability.Reason,
		DeployDir:        capability.DeployDir,
		ComposeFile:      capability.ComposeFile,
		BackendContainer: capability.BackendContainer,
		Releases:         items,
	}))
}

func (h *Handler) systemUpgrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}
	if !h.systemUpgradeMu.TryLock() {
		response.WriteJSON(w, response.Err(-2, systemUpgradeConflictError))
		return
	}
	defer h.systemUpgradeMu.Unlock()

	var req struct {
		Version string `json:"version"`
		Channel string `json:"channel"`
	}
	if err := decodeJSON(r.Body, &req); err != nil && err != io.EOF {
		response.WriteJSON(w, response.ErrDefault("请求参数错误"))
		return
	}

	channel := normalizeReleaseChannel(req.Channel)
	version := strings.TrimSpace(req.Version)
	if version == "" {
		resolved, err := resolveLatestReleaseByChannel(channel)
		if err != nil {
			response.WriteJSON(w, response.Err(-2, fmt.Sprintf("获取最新%s失败: %v", releaseChannelLabel(channel), err)))
			return
		}
		version = resolved
	}

	executor := newSystemUpgradeExecutor()
	capability := executor.checkCapability(r.Context())
	if !capability.Capable {
		response.WriteJSON(w, response.Err(-2, capability.Reason))
		return
	}

	currentCompose, err := os.ReadFile(executor.composePath())
	if err != nil {
		response.WriteJSON(w, response.Err(-2, fmt.Sprintf("读取 compose 文件失败: %v", err)))
		return
	}
	assetName := executor.selectComposeAsset(currentCompose)
	composeData, err := h.downloadReleaseAsset(version, assetName)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := executor.backupFile(executor.composePath()); err != nil {
		response.WriteJSON(w, response.Err(-2, fmt.Sprintf("备份 compose 文件失败: %v", err)))
		return
	}
	if err := executor.backupFile(executor.envPath()); err != nil {
		response.WriteJSON(w, response.Err(-2, fmt.Sprintf("备份 .env 失败: %v", err)))
		return
	}
	if err := executor.replaceComposeFile(composeData); err != nil {
		response.WriteJSON(w, response.Err(-2, fmt.Sprintf("更新 compose 文件失败: %v", err)))
		return
	}
	if err := executor.updateEnvVersion(executor.envPath(), version); err != nil {
		response.WriteJSON(w, response.Err(-2, fmt.Sprintf("更新 FLUX_VERSION 失败: %v", err)))
		return
	}

	helperCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	imageID, err := executor.currentBackendImage(helperCtx)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	helperContainerID, err := executor.startHelper(helperCtx, imageID)
	if err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OK(systemUpgradeRunData{
		Version:           version,
		Message:           systemUpgradeMessage,
		HelperContainerID: helperContainerID,
		Commands: []string{
			"docker run -d --rm --volumes-from flux-panel-backend ...",
			"docker compose pull backend frontend",
			"docker compose up -d backend frontend",
		},
	}))
}
```

Leave `go-backend/internal/http/handler/upgrade.go` using the existing release-channel helpers and `buildGithubDownloadURL` as-is; `system_upgrade.go` should call those helpers rather than duplicating them.

- [ ] **Step 4: Run the focused backend tests again**

Run:

```bash
go test ./internal/http/handler -run 'Test(SystemVersionRejectsWrongMethod|SystemUpgradeRejectsConcurrentRequests|SelectComposeAsset|UpdateEnvVersion|ValidateBackendContainerName|BuildHelperRunArgs)' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit only if the user explicitly requested a commit**

```bash
git add go-backend/internal/http/handler/handler.go go-backend/internal/http/handler/upgrade.go go-backend/internal/http/handler/system_upgrade.go go-backend/internal/http/handler/system_upgrade_test.go
git commit -m "feat: add panel self-upgrade backend"
```

### Task 3: Add Docker CLI support and deployment mounts

**Files:**
- Modify: `go-backend/Dockerfile`
- Modify: `docker-compose-v4.yml`
- Modify: `docker-compose-v6.yml`

- [ ] **Step 1: Update the Docker Compose templates with runtime env and mounts**

Change both `docker-compose-v4.yml` and `docker-compose-v6.yml` backend services to include these exact lines:

```yaml
  backend:
    image: ghcr.io/sagit-chu/flux-panel-backend:${FLUX_VERSION:-latest}
    container_name: flux-panel-backend
    restart: unless-stopped
    environment:
      DB_TYPE: ${DB_TYPE:-sqlite}
      DB_PATH: /app/data/gost.db
      DATABASE_URL: ${DATABASE_URL:-}
      JWT_SECRET: ${JWT_SECRET}
      SERVER_ADDR: :6365
      TZ: Asia/Shanghai
      FLUX_VERSION: ${FLUX_VERSION:-dev}
      PANEL_DEPLOY_DIR: /opt/flvx-panel
      PANEL_BACKEND_CONTAINER: flux-panel-backend
    volumes:
      - sqlite_data:/app/data
      - /var/run/docker.sock:/var/run/docker.sock
      - ./:/opt/flvx-panel
```

- [ ] **Step 2: Validate the two Compose templates render cleanly**

Run:

```bash
JWT_SECRET=test BACKEND_PORT=6365 FRONTEND_PORT=6366 FLUX_VERSION=dev docker compose -f docker-compose-v4.yml config >/tmp/flvx-v4.rendered.yml
JWT_SECRET=test BACKEND_PORT=6365 FRONTEND_PORT=6366 FLUX_VERSION=dev docker compose -f docker-compose-v6.yml config >/tmp/flvx-v6.rendered.yml
```

Expected: both commands exit `0`, and each rendered backend service includes `PANEL_DEPLOY_DIR`, `PANEL_BACKEND_CONTAINER`, and `/var/run/docker.sock:/var/run/docker.sock`.

- [ ] **Step 3: Update the backend runtime image to include Docker CLI + compose plugin**

Replace `go-backend/Dockerfile` with this structure:

```dockerfile
FROM golang:1.25-bookworm AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} env ${TARGETARCH:+GOARCH=${TARGETARCH}} go build -o /out/paneld ./cmd/paneld

FROM docker:27-cli AS dockercli

FROM debian:bookworm-slim
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates wget && rm -rf /var/lib/apt/lists/*
COPY --from=builder /out/paneld /app/paneld
COPY --from=dockercli /usr/local/bin/docker /usr/local/bin/docker
COPY --from=dockercli /usr/local/libexec/docker/cli-plugins/docker-compose /usr/local/libexec/docker/cli-plugins/docker-compose

ENV SERVER_ADDR=:6365
EXPOSE 6365
ENTRYPOINT ["/app/paneld"]
```

- [ ] **Step 4: Build and smoke-test the backend image**

Run:

```bash
docker build -t flvx-backend-system-upgrade-test ./go-backend
docker run --rm flvx-backend-system-upgrade-test docker compose version
```

Expected: the image build exits `0`, and the container prints a compose version string and exits `0`.

- [ ] **Step 5: Commit only if the user explicitly requested a commit**

```bash
git add go-backend/Dockerfile docker-compose-v4.yml docker-compose-v6.yml
git commit -m "feat: wire docker runtime for panel upgrades"
```

### Task 4: Add typed frontend API wrappers for system upgrade

**Files:**
- Modify: `vite-frontend/src/api/types.ts`
- Modify: `vite-frontend/src/api/index.ts`
- Modify: `vite-frontend/src/pages/config.tsx`

**Note:** This repo has no frontend unit test runner. Use `pnpm run build` as the failing/passing contract check for the TypeScript surface.

- [ ] **Step 1: Make the config page import the not-yet-existing API members**

Update the imports at the top of `vite-frontend/src/pages/config.tsx` to include the missing members before you implement them:

```tsx
import {
  updateConfigs,
  activateLicense,
  exportBackup,
  importBackup,
  getAnnouncement,
  updateAnnouncement,
  getStorageSummary,
  getSystemUpgradeVersion,
  checkSystemUpgrade,
  runSystemUpgrade,
  type AnnouncementData,
} from "@/api";
import type {
  NodeReleaseApiItem,
  SystemUpgradeCheckApiData,
  SystemUpgradeRunApiData,
  SystemUpgradeVersionApiData,
} from "@/api/types";
```

- [ ] **Step 2: Run the frontend build to verify it fails**

Run:

```bash
pnpm run build
```

Expected: FAIL with TypeScript errors such as `Module '"@/api"' has no exported member 'getSystemUpgradeVersion'` and `Module '"@/api/types"' has no exported member 'SystemUpgradeVersionApiData'`.

- [ ] **Step 3: Add the missing frontend types and API functions**

Append these interfaces near the existing `StorageSummaryApiData` and monitor types in `vite-frontend/src/api/types.ts`:

```ts
export interface SystemUpgradeVersionApiData {
  currentVersion: string;
  channel: "stable" | "dev";
  latestVersion?: string;
  hasUpdate: boolean;
  capable: boolean;
  reason?: string;
  deployDir?: string;
  composeFile?: string;
  backendContainer?: string;
}

export interface SystemUpgradeCheckApiData
  extends SystemUpgradeVersionApiData {
  releases: NodeReleaseApiItem[];
}

export interface SystemUpgradeRunApiData {
  version: string;
  message: string;
  commands: string[];
  helperContainerId?: string;
}
```

Then update the top import block and exports in `vite-frontend/src/api/index.ts`:

```ts
import type {
  BatchOperationResult,
  ForwardDiagnosisApiData,
  ForwardApiItem,
  GroupPermissionApiItem,
  NodeReleaseApiItem,
  NodeApiItem,
  SpeedLimitApiItem,
  StorageSummaryApiData,
  SystemUpgradeCheckApiData,
  SystemUpgradeRunApiData,
  SystemUpgradeVersionApiData,
  TunnelApiItem,
  TunnelBatchDeletePreviewApiData,
  TunnelBatchDeleteWithForwardsApiData,
  TunnelDeletePreviewApiData,
  TunnelDeleteWithForwardsApiData,
  TunnelDiagnosisApiData,
  TunnelGroupApiItem,
  TunnelMetricApiItem,
  TunnelQualityApiItem,
  UpdatePasswordPayload,
  UserApiItem,
  UserGroupApiItem,
  UserListQuery,
  UserMutationPayload,
  UserPackageInfoApiData,
  UserQuotaResetPayload,
  UserTunnelApiItem,
  UserTunnelAssignPayload,
  UserTunnelListQuery,
  UserTunnelPermissionApiItem,
  UserTunnelRemovePayload,
} from "./types";

export const getSystemUpgradeVersion = () =>
  Network.post<SystemUpgradeVersionApiData>("/system/version");

export const checkSystemUpgrade = (channel: ReleaseChannel = "stable") =>
  Network.post<SystemUpgradeCheckApiData>("/system/check-updates", {
    channel,
  });

export const runSystemUpgrade = (
  version?: string,
  channel: ReleaseChannel = "stable",
) =>
  Network.post<SystemUpgradeRunApiData>(
    "/system/upgrade",
    { version: version || "", channel },
    { timeout: 60 * 1000 },
  );
```

- [ ] **Step 4: Run the frontend build again**

Run:

```bash
pnpm run build
```

Expected: PASS, because the new imports now exist even though the config page does not render the upgrade UI yet.

- [ ] **Step 5: Commit only if the user explicitly requested a commit**

```bash
git add vite-frontend/src/api/index.ts vite-frontend/src/api/types.ts vite-frontend/src/pages/config.tsx
git commit -m "feat: add frontend api surface for panel upgrades"
```

### Task 5: Add the settings-page upgrade card and confirmation modal

**Files:**
- Modify: `vite-frontend/src/pages/config.tsx`

**Note:** This repo has no frontend unit test runner. Use `pnpm run build` as the failing/passing contract check for the UI state and JSX wiring.

- [ ] **Step 1: Add the upgrade card and modal markup before defining the state/handlers**

Insert this JSX block just after the database storage section in `vite-frontend/src/pages/config.tsx`, before the save button row:

```tsx
          <Divider className="my-2" />

          <div className="space-y-3">
            <div className="flex flex-col gap-1 md:flex-row md:items-start md:justify-between">
              <div>
                <p className="text-sm font-medium text-gray-700 dark:text-gray-300">
                  面板升级
                </p>
                <p className="text-xs text-gray-500 dark:text-gray-400">
                  升级 backend 与 frontend 容器。操作会短暂中断面板访问，仅管理员可用。
                </p>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button
                  isLoading={systemUpgradeChecking}
                  size="sm"
                  variant="flat"
                  onPress={handleCheckSystemUpgrade}
                >
                  检查更新
                </Button>
                <Button
                  color="primary"
                  isDisabled={!systemUpgradeInfo?.capable || systemUpgradeLoading}
                  size="sm"
                  onPress={handleOpenSystemUpgradeModal}
                >
                  立即升级
                </Button>
              </div>
            </div>

            <div className="rounded-lg border border-divider bg-default-50/60 px-4 py-3 dark:bg-default-100/10">
              {systemUpgradeLoading ? (
                <p className="text-sm text-default-500">加载升级状态中...</p>
              ) : (
                <div className="space-y-2 text-sm text-default-700 dark:text-default-300">
                  <p>当前版本：{systemUpgradeInfo?.currentVersion || "未知"}</p>
                  <p>最新版本：{systemUpgradeInfo?.latestVersion || "未检查"}</p>
                  <p>当前通道：{updateChannel === "stable" ? "稳定版" : "开发版"}</p>
                  <p>
                    升级能力：
                    {systemUpgradeInfo?.capable ? "可用" : systemUpgradeInfo?.reason || "不可用"}
                  </p>
                  {systemUpgradeInfo?.deployDir ? (
                    <p>部署目录：{systemUpgradeInfo.deployDir}</p>
                  ) : null}
                </div>
              )}
            </div>
          </div>

          <Modal
            isOpen={systemUpgradeModalOpen}
            onOpenChange={setSystemUpgradeModalOpen}
          >
            <ModalContent>
              <ModalHeader>确认升级面板</ModalHeader>
              <ModalBody className="space-y-4">
                <div className="rounded-lg border border-warning-300 bg-warning-50 px-4 py-3 text-sm text-warning-800 dark:border-warning-700 dark:bg-warning-900/20 dark:text-warning-200">
                  该操作会通过 Docker socket 控制宿主机 Docker，并短暂重启当前面板服务。
                </div>
                <Select
                  label="目标版本"
                  placeholder="留空时自动选择最新版本"
                  selectedKeys={systemUpgradeSelectedVersion ? [systemUpgradeSelectedVersion] : []}
                  variant="bordered"
                  onSelectionChange={(keys) => {
                    const selected = (Array.from(keys)[0] as string) || "";
                    setSystemUpgradeSelectedVersion(selected);
                  }}
                >
                  {systemUpgradeReleases.map((release) => (
                    <SelectItem
                      key={release.version}
                      description={release.publishedAt}
                    >
                      {release.version}
                    </SelectItem>
                  ))}
                </Select>
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={() => setSystemUpgradeModalOpen(false)}>
                  取消
                </Button>
                <Button
                  color="primary"
                  isLoading={systemUpgradeExecuting}
                  onPress={handleConfirmSystemUpgrade}
                >
                  确认升级
                </Button>
              </ModalFooter>
            </ModalContent>
          </Modal>
```

- [ ] **Step 2: Run the frontend build to verify it fails**

Run:

```bash
pnpm run build
```

Expected: FAIL with TypeScript errors like `Cannot find name 'systemUpgradeInfo'`, `Cannot find name 'handleCheckSystemUpgrade'`, and `Cannot find name 'systemUpgradeReleases'`.

- [ ] **Step 3: Add the missing state, loaders, handlers, and modal control**

Add these hooks near the existing config page state in `vite-frontend/src/pages/config.tsx`:

```tsx
  const [systemUpgradeInfo, setSystemUpgradeInfo] =
    useState<SystemUpgradeVersionApiData | null>(null);
  const [systemUpgradeChecking, setSystemUpgradeChecking] = useState(false);
  const [systemUpgradeExecuting, setSystemUpgradeExecuting] = useState(false);
  const [systemUpgradeLoading, setSystemUpgradeLoading] = useState(true);
  const [systemUpgradeModalOpen, setSystemUpgradeModalOpen] = useState(false);
  const [systemUpgradeReleases, setSystemUpgradeReleases] = useState<
    NodeReleaseApiItem[]
  >([]);
  const [systemUpgradeSelectedVersion, setSystemUpgradeSelectedVersion] =
    useState("");
```

Add these helpers near `loadStorageSummary` and `handleUpdateChannelChange`:

```tsx
  const loadSystemUpgradeInfo = async () => {
    setSystemUpgradeLoading(true);
    try {
      const response = await getSystemUpgradeVersion();

      if (response.code === 0 && response.data) {
        setSystemUpgradeInfo(response.data);
      } else {
        setSystemUpgradeInfo(null);
      }
    } catch {
      setSystemUpgradeInfo(null);
    } finally {
      setSystemUpgradeLoading(false);
    }
  };

  const handleCheckSystemUpgrade = async () => {
    setSystemUpgradeChecking(true);
    try {
      const response = await checkSystemUpgrade(updateChannel);

      if (response.code === 0 && response.data) {
        const data = response.data as SystemUpgradeCheckApiData;
        setSystemUpgradeInfo(data);
        setSystemUpgradeReleases(data.releases || []);
        toast.success(
          data.latestVersion
            ? `已检查到最新版本 ${data.latestVersion}`
            : "未获取到可用版本",
        );
      } else {
        toast.error(response.msg || "检查更新失败");
      }
    } catch {
      toast.error("检查更新失败，请重试");
    } finally {
      setSystemUpgradeChecking(false);
    }
  };

  const handleOpenSystemUpgradeModal = async () => {
    setSystemUpgradeModalOpen(true);
    if (systemUpgradeReleases.length === 0) {
      await handleCheckSystemUpgrade();
    }
  };

  const handleConfirmSystemUpgrade = async () => {
    setSystemUpgradeExecuting(true);
    try {
      const response = await runSystemUpgrade(
        systemUpgradeSelectedVersion || undefined,
        updateChannel,
      );

      if (response.code === 0 && response.data) {
        const data = response.data as SystemUpgradeRunApiData;
        toast.success(data.message || "升级已触发，请稍后刷新页面");
        setSystemUpgradeModalOpen(false);
      } else {
        toast.error(response.msg || "面板升级失败");
      }
    } catch {
      toast.error("面板升级失败，请重试");
    } finally {
      setSystemUpgradeExecuting(false);
    }
  };
```

Update the initial `useEffect` so it loads the new status alongside configs and storage:

```tsx
  useEffect(() => {
    const timer = setTimeout(() => {
      loadConfigs(initialConfigs);
      loadAnnouncement();
      loadStorageSummary();
      loadSystemUpgradeInfo();
    }, 100);

    return () => clearTimeout(timer);
  }, []);
```

Finally, extend `handleUpdateChannelChange` so changing the update channel clears stale release picks and refreshes the displayed status:

```tsx
  const handleUpdateChannelChange = (channel: UpdateReleaseChannel) => {
    setUpdateChannel(channel);
    setUpdateReleaseChannel(channel);
    setSystemUpgradeSelectedVersion("");
    setSystemUpgradeReleases([]);
    void loadSystemUpgradeInfo();
    toast.success(
      `更新通道已切换为${channel === "stable" ? "稳定版" : "开发版"}`,
    );
  };
```

- [ ] **Step 4: Run the frontend build again**

Run:

```bash
pnpm run build
```

Expected: PASS.

- [ ] **Step 5: Commit only if the user explicitly requested a commit**

```bash
git add vite-frontend/src/pages/config.tsx
git commit -m "feat: add panel self-upgrade settings ui"
```

### Task 6: Run the full verification suite and admin QA smoke checks

**Files:**
- No new files.

- [ ] **Step 1: Re-run the targeted backend unit tests for the new helper and handlers**

Run:

```bash
go test ./internal/http/handler -run 'Test(SelectComposeAsset|UpdateEnvVersion|ValidateBackendContainerName|BuildHelperRunArgs|SystemVersionRejectsWrongMethod|SystemUpgradeRejectsConcurrentRequests)' -count=1
```

Expected: PASS.

- [ ] **Step 2: Run the full backend test suite**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Run the frontend production build**

Run:

```bash
pnpm run build
```

Expected: PASS.

- [ ] **Step 4: Re-run the Compose template checks**

Run:

```bash
JWT_SECRET=test BACKEND_PORT=6365 FRONTEND_PORT=6366 FLUX_VERSION=dev docker compose -f docker-compose-v4.yml config >/tmp/flvx-v4.rendered.yml
JWT_SECRET=test BACKEND_PORT=6365 FRONTEND_PORT=6366 FLUX_VERSION=dev docker compose -f docker-compose-v6.yml config >/tmp/flvx-v6.rendered.yml
```

Expected: PASS for both templates.

- [ ] **Step 5: Re-run the backend image smoke test**

Run:

```bash
docker build -t flvx-backend-system-upgrade-test ./go-backend
docker run --rm flvx-backend-system-upgrade-test docker compose version
```

Expected: PASS, with the second command printing a compose version string.

- [ ] **Step 6: Manually smoke-test the admin flow in the browser**

Manual checklist:

```text
1. Log in as an admin user.
2. Open /config.
3. Confirm the “面板升级” card shows current version and capability state.
4. In an environment without /var/run/docker.sock, confirm the card shows the capability reason and disables the upgrade button.
5. In a Docker-enabled environment, click “检查更新” and confirm the latest version + release list populate.
6. Open the confirmation modal and confirm the warning about Docker socket / short restart is visible.
```

Expected: all six checks succeed.

- [ ] **Step 7: Commit only if the user explicitly requested a commit**

```bash
git add go-backend/internal/http/handler/handler.go go-backend/internal/http/handler/system_upgrade.go go-backend/internal/http/handler/system_upgrade_test.go go-backend/Dockerfile docker-compose-v4.yml docker-compose-v6.yml vite-frontend/src/api/index.ts vite-frontend/src/api/types.ts vite-frontend/src/pages/config.tsx
git commit -m "feat: add panel self-upgrade workflow"
```
