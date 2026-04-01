# GitHub 加速地址自定义配置设计

**日期**: 2026-04-01
**状态**: 待审核
**作者**: AI Assistant

## 概述

允许用户在面板设置中自定义 GitHub 加速地址，支持开启/关闭加速功能。配置后，面板更新节点、生成安装命令以及安装脚本都使用配置的加速地址。

## 背景

当前 `gcode.hostcentral.cc` 硬编码在多个位置：
- `go-backend/internal/http/handler/upgrade.go` - 节点升级下载 URL
- `go-backend/internal/http/handler/mutations.go` - 节点安装命令生成
- `install.sh` - 节点安装脚本
- `panel_install.sh` - 面板安装脚本

用户无法自定义加速地址或关闭加速功能。

## 目标

1. 面板设置中支持配置加速开关和加速地址
2. 配置影响全部下载场景（面板端 + 安装脚本）
3. 安装脚本支持交互式询问加速配置
4. 面板生成的安装命令自动嵌入加速配置

## 影响范围

### 后端
- `go-backend/internal/http/handler/upgrade.go`
- `go-backend/internal/http/handler/mutations.go`

### 前端
- `vite-frontend/src/pages/config.tsx`
- `vite-frontend/src/config/site.ts`（缓存配置键）

### 安装脚本
- `install.sh`
- `panel_install.sh`

## 详细设计

### 1. 数据存储

使用现有 `vite_config` 表存储两个配置项：

| name | value | 说明 |
|------|-------|------|
| `github_proxy_enabled` | `"true"` / `"false"` | 是否开启加速，默认 `"true"` |
| `github_proxy_url` | URL 字符串 | 加速地址，默认 `"https://gcode.hostcentral.cc"` |

### 2. 后端 Handler 修改

#### upgrade.go

移除硬编码常量，新增辅助函数：

```go
// getGithubProxyConfig 获取 GitHub 加速配置
// 返回: (是否开启, 加速地址)
func (h *Handler) getGithubProxyConfig() (enabled bool, proxyURL string) {
    enabled = true  // 默认开启
    proxyURL = "https://gcode.hostcentral.cc"  // 默认地址
    
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

// buildDownloadURL 构建下载地址
func (h *Handler) buildDownloadURL(version, arch string) string {
    enabled, proxyURL := h.getGithubProxyConfig()
    base := fmt.Sprintf("https://github.com/%s/releases/download/%s/gost-%s", githubRepo, version, arch)
    
    if enabled {
        return fmt.Sprintf("%s/%s", proxyURL, base)
    }
    return base
}
```

修改 `nodeUpgrade` 和 `nodeBatchUpgrade` 使用动态配置。

#### mutations.go

修改 `getNodeInstallCmd` 函数（约第 440-456 行）：

```go
func (h *Handler) getNodeInstallCmd(w http.ResponseWriter, r *http.Request) {
    // ... 现有逻辑 ...
    
    enabled, proxyURL := h.getGithubProxyConfig()
    
    var cmd string
    if enabled {
        cmd = fmt.Sprintf(
            "curl -L %s/https://github.com/%s/releases/download/%s/install.sh -o ./install.sh && chmod +x ./install.sh && PROXY_ENABLED=true PROXY_URL=%s VERSION=%s ./install.sh -a %s -s %s",
            proxyURL, githubRepo, version, proxyURL, version, processServerAddress(panelAddr), secret,
        )
    } else {
        cmd = fmt.Sprintf(
            "curl -L https://github.com/%s/releases/download/%s/install.sh -o ./install.sh && chmod +x ./install.sh && PROXY_ENABLED=false VERSION=%s ./install.sh -a %s -s %s",
            githubRepo, version, version, processServerAddress(panelAddr), secret,
        )
    }
    
    response.WriteJSON(w, response.OK(cmd))
}
```

### 3. 前端修改

#### config.tsx

在 `CONFIG_ITEMS` 数组中添加配置项（约第 87-158 行之后）：

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

在 `getInitialConfigs` 函数的 `configKeys` 数组中添加缓存键：

```typescript
"github_proxy_enabled",
"github_proxy_url",
```

### 4. 安装脚本修改

#### install.sh

在脚本开头添加配置变量和环境变量读取：

```bash
# 镜像加速配置（可由面板传入）
PROXY_ENABLED="${PROXY_ENABLED:-}"
PROXY_URL="${PROXY_URL:-}"
```

修改 `maybe_proxy_url` 函数：

```bash
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
    proxy="${proxy%/}"  # 移除末尾斜杠
  else
    proxy="https://${proxy}"
  fi
  
  echo "${proxy}/${url}"
}
```

在 `install_flux_agent` 函数开头添加交互式询问：

```bash
install_flux_agent() {
  echo "🚀 开始安装 flux_agent..."
  
  # 询问加速配置（如果未由面板传入）
  if [[ -z "$PROXY_ENABLED" ]]; then
    echo ""
    read -p "是否开启 GitHub 加速? (Y/n): " proxy_choice
    case "$proxy_choice" in
      n|N) PROXY_ENABLED="false" ;;
      *)   
        PROXY_ENABLED="true"
        read -p "加速地址 (默认 gcode.hostcentral.cc): " input_url
        PROXY_URL="${input_url:-gcode.hostcentral.cc}"
        ;;
    esac
  fi
  
  # ... 现有安装逻辑 ...
}
```

#### panel_install.sh

类似修改，在 `install_panel` 函数开头添加询问逻辑。

### 5. 配置缓存

#### site.ts

在配置缓存键列表中添加新键（如果需要前端缓存加速配置）。

## 默认行为

- `github_proxy_enabled`: 默认 `"true"`（开启加速）
- `github_proxy_url`: 默认 `"https://gcode.hostcentral.cc"`

## 测试要点

1. **后端 API 测试**：
   - 未配置时使用默认值
   - 配置后正确读取并应用
   - 关闭加速后直连 GitHub

2. **前端 UI 测试**：
   - Switch 开关正确切换
   - 关闭加速时隐藏地址输入框
   - 保存配置后正确持久化

3. **安装脚本测试**：
   - 交互式询问正常工作
   - 环境变量传入时跳过询问
   - 加速关闭时直连 GitHub

4. **集成测试**：
   - 面板生成安装命令正确包含加速配置
   - 节点升级下载使用配置的加速地址

## 风险与缓解

| 风险 | 缓解措施 |
|------|----------|
| 用户输入无效加速地址 | 后端验证 URL 格式，前端添加格式提示 |
| 旧版本安装脚本不兼容 | 保持 `maybe_proxy_url` 函数签名不变，仅修改内部逻辑 |
| 配置缺失时行为不一致 | 在 `getGithubProxyConfig` 中提供合理的默认值 |

## 任务清单

- [ ] 后端：upgrade.go 修改
- [ ] 后端：mutations.go 修改
- [ ] 前端：config.tsx 添加配置项
- [ ] 脚本：install.sh 修改
- [ ] 脚本：panel_install.sh 修改
- [ ] 测试：验证功能正常