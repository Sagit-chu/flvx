# GitHub 加速地址自定义配置实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 允许用户在面板设置中自定义 GitHub 加速地址，支持开启/关闭加速，配置影响全部下载场景。

**Architecture:** 使用现有 `vite_config` 表存储配置，后端 Handler 读取配置替换硬编码，前端复用现有配置项渲染逻辑，安装脚本支持环境变量和交互式询问。

**Tech Stack:** Go 1.24, React/TypeScript, Shell/Bash

---

## 文件结构

| 文件 | 操作 | 说明 |
|------|------|------|
| `go-backend/internal/http/handler/upgrade.go` | 修改 | 移除硬编码，添加配置读取函数 |
| `go-backend/internal/http/handler/mutations.go` | 修改 | `nodeInstall` 函数使用动态配置 |
| `vite-frontend/src/pages/config.tsx` | 修改 | 添加两个新配置项 |
| `install.sh` | 修改 | 支持交互式询问和环境变量 |
| `panel_install.sh` | 修改 | 支持交互式询问和环境变量 |
| `test-install-scripts-proxy.sh` | 新增 | 覆盖代理交互与下载 URL 回归 |

---

## Task 1: 后端 - upgrade.go 修改

**Files:**
- Modify: `go-backend/internal/http/handler/upgrade.go`

- [x] **Step 1: 移除硬编码常量，添加配置读取函数**

在 `upgrade.go` 中，移除 `githubProxy` 常量，添加 `getGithubProxyConfig` 函数：

找到第 16-26 行：
```go
const (
	githubRepo     = "Sagit-chu/flvx"
	githubProxy    = "https://gcode.hostcentral.cc"
	githubAPIBase  = "https://api.github.com"
	githubHTMLBase = "https://github.com"
	upgradeTimeout = 5 * time.Minute
	batchWorkers   = 5

	releaseChannelStable = "stable"
	releaseChannelDev    = "dev"
)
```

替换为：
```go
const (
	githubRepo     = "Sagit-chu/flvx"
	githubAPIBase  = "https://api.github.com"
	githubHTMLBase = "https://github.com"
	upgradeTimeout = 5 * time.Minute
	batchWorkers   = 5

	releaseChannelStable = "stable"
	releaseChannelDev    = "dev"

	defaultGithubProxyEnabled = true
	defaultGithubProxyURL     = "https://gcode.hostcentral.cc"
)
```

然后在 `releaseChannelLabel` 函数后（约第 71 行之后）添加新函数：
```go
// getGithubProxyConfig 获取 GitHub 加速配置
// 返回: (是否开启加速, 加速地址)
func (h *Handler) getGithubProxyConfig() (enabled bool, proxyURL string) {
	enabled = defaultGithubProxyEnabled
	proxyURL = defaultGithubProxyURL

	if h == nil || h.repo == nil {
		return
	}

	// 读取开启状态
	if enabledCfg, err := h.repo.GetConfigByName("github_proxy_enabled"); err == nil && enabledCfg != nil {
		enabled = enabledCfg.Value != "false"
	}

	// 读取加速地址
	if urlCfg, err := h.repo.GetConfigByName("github_proxy_url"); err == nil && urlCfg != nil && urlCfg.Value != "" {
		proxyURL = strings.TrimSpace(urlCfg.Value)
		// 确保 URL 格式正确
		if !strings.HasPrefix(proxyURL, "http://") && !strings.HasPrefix(proxyURL, "https://") {
			proxyURL = "https://" + proxyURL
		}
		proxyURL = strings.TrimSuffix(proxyURL, "/")
	}

	return
}

// buildGithubDownloadURL 构建 GitHub 下载地址
func (h *Handler) buildGithubDownloadURL(version, filename string) string {
	enabled, proxyURL := h.getGithubProxyConfig()
	base := fmt.Sprintf("%s/%s/releases/download/%s/%s", githubHTMLBase, githubRepo, version, filename)

	if enabled {
		return fmt.Sprintf("%s/%s", proxyURL, base)
	}
	return base
}
```

- [x] **Step 2: 修改 nodeUpgrade 函数使用动态配置**

找到第 152-159 行：
```go
	downloadURL := fmt.Sprintf(
		githubProxy+"/%s/%s/releases/download/%s/gost-{ARCH}",
		githubHTMLBase, githubRepo, version,
	)
	checksumURL := fmt.Sprintf(
		githubProxy+"/%s/%s/releases/download/%s/gost-{ARCH}.sha256",
		githubHTMLBase, githubRepo, version,
	)
```

替换为：
```go
	downloadURL := h.buildGithubDownloadURL(version, "gost-{ARCH}")
	checksumURL := h.buildGithubDownloadURL(version, "gost-{ARCH}.sha256")
```

- [x] **Step 3: 修改 nodeBatchUpgrade 函数使用动态配置**

找到第 216-223 行：
```go
	downloadURL := fmt.Sprintf(
		githubProxy+"/%s/%s/releases/download/%s/gost-{ARCH}",
		githubHTMLBase, githubRepo, version,
	)
	checksumURL := fmt.Sprintf(
		githubProxy+"/%s/%s/releases/download/%s/gost-{ARCH}.sha256",
		githubHTMLBase, githubRepo, version,
	)
```

替换为：
```go
	downloadURL := h.buildGithubDownloadURL(version, "gost-{ARCH}")
	checksumURL := h.buildGithubDownloadURL(version, "gost-{ARCH}.sha256")
```

- [x] **Step 4: 验证编译**

Run: `cd go-backend && go build ./...`
Expected: 编译成功，无错误

- [x] **Step 5: 提交**

```bash
git add go-backend/internal/http/handler/upgrade.go
git commit -m "feat(backend): use configurable github proxy for node upgrades"
```

---

## Task 2: 后端 - mutations.go 修改

**Files:**
- Modify: `go-backend/internal/http/handler/mutations.go`

- [x] **Step 1: 修改 nodeInstall 函数使用动态配置**

找到第 456 行：
```go
	cmd := fmt.Sprintf("curl -L https://gcode.hostcentral.cc/https://github.com/Sagit-chu/flvx/releases/download/%s/install.sh -o ./install.sh && chmod +x ./install.sh && VERSION=%s ./install.sh -a %s -s %s", version, version, processServerAddress(panelAddr), secret)
```

替换为：
```go
	enabled, proxyURL := h.getGithubProxyConfig()

	var cmd string
	if enabled {
		cmd = fmt.Sprintf("curl -L %s/https://github.com/%s/releases/download/%s/install.sh -o ./install.sh && chmod +x ./install.sh && PROXY_ENABLED=true PROXY_URL=%s VERSION=%s ./install.sh -a %s -s %s",
			proxyURL, githubRepo, version, proxyURL, version, processServerAddress(panelAddr), secret)
	} else {
		cmd = fmt.Sprintf("curl -L https://github.com/%s/releases/download/%s/install.sh -o ./install.sh && chmod +x ./install.sh && PROXY_ENABLED=false VERSION=%s ./install.sh -a %s -s %s",
			githubRepo, version, version, processServerAddress(panelAddr), secret)
	}
```

- [x] **Step 2: 验证编译**

Run: `cd go-backend && go build ./...`
Expected: 编译成功，无错误

- [x] **Step 3: 提交**

```bash
git add go-backend/internal/http/handler/mutations.go
git commit -m "feat(backend): use configurable github proxy for node install command"
```

---

## Task 3: 前端 - config.tsx 添加配置项

**Files:**
- Modify: `vite-frontend/src/pages/config.tsx`

- [x] **Step 1: 在 CONFIG_ITEMS 数组中添加配置项**

找到第 158 行（`CONFIG_ITEMS` 数组的结束位置）：
```go
  {
    key: "cloudflare_secret_key",
    label: "Cloudflare Secret Key",
    placeholder: "请输入 Cloudflare Secret Key",
    description: "Cloudflare Turnstile 密钥",
    type: "input",
    dependsOn: "captcha_enabled",
    dependsValue: "true",
  },
];
```

在 `];` 之前添加：
```typescript
  {
    key: "github_proxy_enabled",
    label: "开启 GitHub 加速",
    description: "用于节点更新和安装脚本下载，解决部分地区 GitHub 访问受限问题",
    type: "switch",
  },
  {
    key: "github_proxy_url",
    label: "加速地址",
    placeholder: "https://gcode.hostcentral.cc",
    description: "GitHub 下载加速代理地址，开启加速后生效",
    type: "input",
    dependsOn: "github_proxy_enabled",
    dependsValue: "true",
  },
```

- [x] **Step 2: 在缓存键列表中添加新键**

找到第 179-190 行：
```typescript
  const configKeys = [
    "app_name",
    "captcha_enabled",
    "cloudflare_site_key",
    "cloudflare_secret_key",
    "forward_compact_mode",
    "monitor_tunnel_quality_enabled",
    "ip",
    "panel_domain",
    "app_logo",
    "app_favicon",
  ];
```

在 `"app_favicon",` 之后添加：
```typescript
    "github_proxy_enabled",
    "github_proxy_url",
```

- [x] **Step 3: 验证前端编译**

Run: `cd vite-frontend && npm run build`
Expected: 编译成功，无错误

- [x] **Step 4: 提交**

```bash
git add vite-frontend/src/pages/config.tsx
git commit -m "feat(frontend): add github proxy config settings"
```

---

## Task 4: 安装脚本 - install.sh 修改

**Files:**
- Modify: `install.sh`

- [x] **Step 1: 添加环境变量声明和修改 maybe_proxy_url 函数**

找到第 28-32 行：
```bash
# 镜像加速（所有下载均经过镜像源，以支持 IPv6）
maybe_proxy_url() {
  local url="$1"
  echo "https://gcode.hostcentral.cc/${url}"
}
```

替换为：
```bash
# 镜像加速配置（可由面板传入或交互式询问）
PROXY_ENABLED="${PROXY_ENABLED:-}"
PROXY_URL="${PROXY_URL:-}"

# 镜像加速
maybe_proxy_url() {
  local url="$1"

  # 如果明确关闭加速
  if [[ "$PROXY_ENABLED" == "false" ]]; then
    echo "$url"
    return
  fi

  # 默认开启加速
  local proxy="${PROXY_URL:-gcode.hostcentral.cc}"

  # 处理 URL 格式
  if [[ "$proxy" == https://* || "$proxy" == http://* ]]; then
    proxy="${proxy%/}"
  else
    proxy="https://${proxy}"
  fi

  echo "${proxy}/${url}"
}

# 询问加速配置（如果未由面板传入）
ask_proxy_config() {
  if [[ -n "$PROXY_ENABLED" ]]; then
    return
  fi

  echo ""
  echo "==============================================="
  echo "           GitHub 加速配置"
  echo "==============================================="
  read -p "是否开启 GitHub 加速? (Y/n): " proxy_choice
  case "$proxy_choice" in
    n|N)
      PROXY_ENABLED="false"
      echo "已关闭加速，将直连 GitHub"
      ;;
    *)
      PROXY_ENABLED="true"
      read -p "加速地址 (默认 gcode.hostcentral.cc): " input_url
      PROXY_URL="${input_url:-gcode.hostcentral.cc}"
      echo "已开启加速: $PROXY_URL"
      ;;
  esac
  echo "==============================================="
}
```

- [x] **Step 2: 修改 install_flux_agent 函数添加询问**

找到第 211-214 行：
```bash
# 安装功能
install_flux_agent() {
  echo "🚀 开始安装 flux_agent..."
  get_config_params
```

替换为：
```bash
# 安装功能
install_flux_agent() {
  echo "🚀 开始安装 flux_agent..."

  # 询问加速配置（如果未由面板传入）
  ask_proxy_config

  get_config_params
```

- [ ] **Step 3: 提交**

```bash
git add install.sh
git commit -m "feat(script): add configurable github proxy for install.sh"
```

---

## Task 5: 安装脚本 - panel_install.sh 修改

**Files:**
- Modify: `panel_install.sh`

- [x] **Step 1: 添加环境变量声明和修改 maybe_proxy_url 函数**

找到第 16-20 行：
```bash
# 镜像加速（所有下载均经过镜像源，以支持 IPv6）
maybe_proxy_url() {
  local url="$1"
  echo "https://gcode.hostcentral.cc/${url}"
}
```

替换为：
```bash
# 镜像加速配置（可由面板传入或交互式询问）
PROXY_ENABLED="${PROXY_ENABLED:-}"
PROXY_URL="${PROXY_URL:-}"

# 镜像加速
maybe_proxy_url() {
  local url="$1"

  # 如果明确关闭加速
  if [[ "$PROXY_ENABLED" == "false" ]]; then
    echo "$url"
    return
  fi

  # 默认开启加速
  local proxy="${PROXY_URL:-gcode.hostcentral.cc}"

  # 处理 URL 格式
  if [[ "$proxy" == https://* || "$proxy" == http://* ]]; then
    proxy="${proxy%/}"
  else
    proxy="https://${proxy}"
  fi

  echo "${proxy}/${url}"
}

# 询问加速配置（如果未由面板传入）
ask_proxy_config() {
  if [[ -n "$PROXY_ENABLED" ]]; then
    return
  fi

  echo ""
  echo "==============================================="
  echo "           GitHub 加速配置"
  echo "==============================================="
  read -p "是否开启 GitHub 加速? (Y/n): " proxy_choice
  case "$proxy_choice" in
    n|N)
      PROXY_ENABLED="false"
      echo "已关闭加速，将直连 GitHub"
      ;;
    *)
      PROXY_ENABLED="true"
      read -p "加速地址 (默认 gcode.hostcentral.cc): " input_url
      PROXY_URL="${input_url:-gcode.hostcentral.cc}"
      echo "已开启加速: $PROXY_URL"
      ;;
  esac
  echo "==============================================="
}
```

- [x] **Step 2: 修改 install_panel 函数添加询问**

找到第 375-378 行：
```bash
# 安装功能
install_panel() {
  echo "🚀 开始安装面板..."
  check_docker
  get_config_params
```

替换为：
```bash
# 安装功能
install_panel() {
  echo "🚀 开始安装面板..."

  # 询问加速配置（如果未由面板传入）
  ask_proxy_config

  check_docker
  get_config_params
```

- [ ] **Step 3: 提交**

```bash
git add panel_install.sh
git commit -m "feat(script): add configurable github proxy for panel_install.sh"
```

---

## Task 6: 最终验证和提交

- [x] **Step 1: 验证后端编译**

Run: `cd go-backend && go build ./...`
Expected: 编译成功

- [x] **Step 2: 验证前端编译**

Run: `cd vite-frontend && npm run build`
Expected: 编译成功

- [x] **Step 3: 验证脚本语法**

Run: `bash -n install.sh && bash -n panel_install.sh && bash test-install-scripts-proxy.sh`
Expected: 无语法错误，且脚本代理回归测试通过

- [ ] **Step 4: 推送所有提交**

```bash
git push
```

---

## 验收标准

1. 面板设置页面显示 GitHub 加速配置项
2. 开关关闭后，下载地址直连 GitHub
3. 自定义加速地址后，节点更新和安装命令使用自定义地址
4. 安装脚本支持交互式询问加速配置
5. 面板生成的安装命令包含加速配置环境变量
