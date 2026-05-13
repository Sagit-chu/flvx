# 面板本体一键升级设计

**日期**: 2026-05-04
**状态**: 待审核
**作者**: AI Assistant

## 概述

在 FLVX 管理面板中增加“面板升级”能力，使管理员可以在网页上检查 GitHub Release 并触发面板本体升级。目标是升级整套面板，而不是只升级转发节点或只替换后端二进制。

本设计采用 Docker Compose 整套升级方案：后端容器通过受限的 Docker socket 能力更新宿主机部署目录中的 `docker-compose.yml` 和 `.env`，然后启动独立的升级 helper 容器，由 helper 拉取新版 backend/frontend 镜像并重新启动 `backend` 与 `frontend` 服务。

## sub2api 参考结论

sub2api 的一键升级不是在宿主机执行 `docker compose pull/up`。它的运行形态是单体 Go 服务：前端构建产物 embed 到后端二进制，容器内只运行 `/app/sub2api`。升级接口下载 GitHub Release 中匹配当前系统和架构的 `sub2api_<version>_<os>_<arch>.tar.gz` 以及 `checksums.txt`，校验后把当前 `os.Executable()` 指向的 `/app/sub2api` 改名为 `/app/sub2api.backup`，再把新二进制原子替换到原路径。重启接口延迟调用 `os.Exit(0)`，依赖 Docker Compose 的 `restart: unless-stopped` 拉起同一个容器。

这种方式在 sub2api 的 Docker 部署中可行，是因为它的前后端在同一个二进制里。FLVX 当前是 `flux-panel-backend` 与 `vite-frontend` 两个容器，替换 `/app/paneld` 只能升级后端，不能升级前端页面。因此 FLVX 的“面板本体升级”需要更新 Compose 版本和两个镜像，而不是照搬二进制替换。

## 目标

1. 管理员可在面板上查看当前版本、最新版本、升级通道和升级能力状态。
2. 管理员可一键升级整套面板 backend/frontend。
3. 升级复用现有 GitHub Release 和 `FLUX_VERSION` 版本机制。
4. 升级复用现有 GitHub 加速配置 `github_proxy_enabled` / `github_proxy_url`。
5. 升级过程不接受任意命令、任意 URL 或任意 compose 路径。
6. 环境不满足时清晰提示不可用原因，不静默失败。

## 非目标

1. 不实现 sub2api 式后端二进制替换作为本次主路径。
2. 不支持从非本仓库 Release 下载升级资产。
3. 不支持普通用户触发升级。
4. 不支持在前端执行 shell 命令。
5. 不修改 `install.sh` 或 `panel_install.sh` 的本地安装菜单逻辑；发布流程仍可能覆盖这些脚本。
6. 不引入前端测试框架。

## 影响范围

### 后端

- `go-backend/internal/http/handler/handler.go`
- `go-backend/internal/http/handler/upgrade.go`
- 新增 `go-backend/internal/http/handler/system_upgrade.go`
- 新增 `go-backend/internal/http/handler/system_upgrade_test.go`
- `go-backend/Dockerfile`

### 部署模板

- `docker-compose-v4.yml`
- `docker-compose-v6.yml`

### 前端

- `vite-frontend/src/api/index.ts`
- `vite-frontend/src/api/types.ts`
- `vite-frontend/src/pages/config.tsx`

## 运行前提

升级能力仅在 Docker Compose 部署中可用，并要求后端容器具备以下条件：

1. 容器内存在 Docker CLI，且支持 `docker compose version`。
2. `/var/run/docker.sock` 挂载到后端容器。
3. 宿主部署目录挂载到容器内固定路径，例如 `/opt/flvx-panel`。
4. 环境变量 `PANEL_DEPLOY_DIR=/opt/flvx-panel`。
5. 环境变量 `PANEL_BACKEND_CONTAINER=flux-panel-backend`，为空时默认使用 `flux-panel-backend`；值必须匹配容器名安全字符集 `[A-Za-z0-9_.-]+`。
6. 部署目录内存在 `.env` 和 `docker-compose.yml`。

如果任一条件不满足，检查接口返回 `capable=false` 和明确的 `reason`，升级按钮禁用。

## 后端设计

### API

新增系统升级接口，路径使用 `/api/v1/system/*`，继续受现有 middleware 管控，仅管理员可访问。

| 方法 | 路径 | 用途 |
|------|------|------|
| `POST` | `/api/v1/system/version` | 返回当前版本、升级通道、能力状态和可选最新版本 |
| `POST` | `/api/v1/system/check-updates` | 强制查询 GitHub Release，返回最新版本和候选列表 |
| `POST` | `/api/v1/system/upgrade` | 执行升级 |

请求体：

```json
{
  "channel": "stable",
  "version": ""
}
```

`channel` 使用现有节点升级的通道语义：`stable` 匹配纯数字版本，`dev` 匹配 `alpha` / `beta` / `rc`。`version` 为空时自动选择该通道最新 Release。

`/api/v1/system/version` 返回：

```json
{
  "currentVersion": "2.1.9-beta14",
  "channel": "stable",
  "latestVersion": "2.1.9",
  "hasUpdate": true,
  "capable": true,
  "reason": "",
  "deployDir": "/opt/flvx-panel",
  "composeFile": "/opt/flvx-panel/docker-compose.yml",
  "backendContainer": "flux-panel-backend"
}
```

`/api/v1/system/upgrade` 成功返回：

```json
{
  "version": "2.1.9",
  "message": "升级 helper 已启动，面板服务将短暂重启",
  "commands": [
    "docker run -d --rm --volumes-from flux-panel-backend ...",
    "docker compose pull backend frontend",
    "docker compose up -d backend frontend"
  ]
}
```

返回的 `commands` 只用于 UI 展示固定步骤，不包含用户输入或 shell 拼接结果。

### 版本来源

当前版本优先从容器环境变量读取：

1. `FLUX_VERSION`
2. `VITE_APP_VERSION` 不在后端容器中可靠存在，不作为后端版本来源。
3. 为空时返回 `dev`。

发布流程已经在 `panel_install.sh` 写入 `.env` 的 `FLUX_VERSION`，Compose 模板需要把该变量传给 backend 容器，保证后端可感知当前版本。

### Release 查询

复用现有 `fetchGitHubReleases`、`resolveLatestReleaseByChannel`、`normalizeReleaseChannel`、`releaseChannelFromTag`、`releaseChannelLabel` 和 GitHub 加速配置能力。新增函数只负责筛选系统升级所需资产：

- `docker-compose-v4.yml`
- `docker-compose-v6.yml`

是否下载 v4/v6 compose 文件通过当前部署目录中的 `docker-compose.yml` 判断：如果网络定义包含 `enable_ipv6: true`，选择 `docker-compose-v6.yml`；否则选择 `docker-compose-v4.yml`。

### 升级执行器

新增 `systemUpgradeExecutor`，职责明确分为可测试的小函数：

1. `checkSystemUpgradeCapability()` 检查 Docker CLI、Docker socket、部署目录、`.env`、`docker-compose.yml`。
2. `selectComposeAsset(currentCompose []byte) string` 选择 v4/v6 compose 资产。
3. `updateEnvVersion(path, version string) error` 原子更新 `.env` 中的 `FLUX_VERSION`。
4. `downloadCompose(version, assetName, dest string) error` 下载新版 compose 模板到临时文件。
5. `currentBackendImage(containerName string) (string, error)` 获取当前 backend 容器镜像 ID。
6. `startSystemUpgradeHelper(version string) error` 启动独立 helper 容器执行固定升级流程。

升级流程：

1. 获取全局升级锁，拒绝并发升级。
2. 校验目标版本存在且不是 draft。
3. 检查升级能力。
4. 备份 `.env` 为 `.env.upgrade.bak`，备份 `docker-compose.yml` 为 `docker-compose.yml.upgrade.bak`。
5. 下载目标版本的 compose 文件到部署目录临时文件。
6. 原子替换 `docker-compose.yml`。
7. 原子更新 `.env` 的 `FLUX_VERSION`。
8. 通过 Docker socket 查询当前 backend 容器的镜像 ID。
9. 使用当前 backend 镜像启动一个不属于 Compose 项目的临时 helper 容器。
10. helper 通过 `--volumes-from flux-panel-backend` 继承部署目录挂载，并显式挂载 `/var/run/docker.sock`。
11. helper 在 `PANEL_DEPLOY_DIR` 下执行 `docker compose pull backend frontend`。
12. helper 等待 5 秒，让 SQLite WAL 等文件刷盘。
13. helper 执行 `docker compose up -d backend frontend`，由 Compose 重建前端和后端。
14. 后端接口在 helper 成功启动后立即返回；浏览器随后会经历短暂断线。

PostgreSQL 模式不主动 pull 或重建 `postgres` 服务，避免无关数据库变动。新版 compose 文件仍保留 postgres 配置供后续手动迁移或重建使用。

### 命令安全

后端不暴露通用命令执行能力。后端只直接执行 Docker CLI 的固定参数，用于获取当前镜像和启动 helper：

```go
exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.Image}}", backendContainer)
exec.CommandContext(ctx, "docker", "run", "-d", "--rm", "--name", helperName,
  "--volumes-from", backendContainer,
  "-v", "/var/run/docker.sock:/var/run/docker.sock",
  "-e", "PANEL_DEPLOY_DIR=/opt/flvx-panel",
  "--entrypoint", "/bin/sh", imageID,
  "-c", helperScript)
```

`helperScript` 由后端固定生成，不拼接用户输入：

```sh
cd "$PANEL_DEPLOY_DIR" && docker compose pull backend frontend && sleep 5 && docker compose up -d backend frontend
```

工作目录固定为 `PANEL_DEPLOY_DIR`。`PANEL_DEPLOY_DIR` 必须是绝对路径，且必须包含 `.env` 和 `docker-compose.yml`。接口输入只允许影响 `channel` 和已验证的 Release `version`。

### 超时和错误处理

1. Release 查询超时沿用现有 GitHub API 客户端超时。
2. 下载 compose 文件使用 60 秒超时。
3. 启动 helper 使用 30 秒超时，helper 内部命令不受原 HTTP 请求生命周期影响。
4. 任一步失败时返回错误信息，并尽量保留 `.upgrade.bak` 供人工恢复。
5. 如果 `.env` 更新后后续步骤失败，不自动回滚镜像或容器，避免误判导致更大破坏；错误信息提示备份文件位置。

## 部署模板设计

`docker-compose-v4.yml` 和 `docker-compose-v6.yml` 的 backend 服务增加：

```yaml
environment:
  FLUX_VERSION: ${FLUX_VERSION:-dev}
  PANEL_DEPLOY_DIR: /opt/flvx-panel
  PANEL_BACKEND_CONTAINER: flux-panel-backend
volumes:
  - sqlite_data:/app/data
  - /var/run/docker.sock:/var/run/docker.sock
  - ./:/opt/flvx-panel
```

`go-backend/Dockerfile` 的 runtime 镜像通过多阶段构建从官方 `docker:27-cli` 镜像复制 Docker CLI 和 compose 插件到 Debian runtime 镜像，避免依赖 Debian apt 源中的 Docker 包可用性：

```dockerfile
FROM docker:27-cli AS dockercli
FROM debian:bookworm-slim
COPY --from=dockercli /usr/local/bin/docker /usr/local/bin/docker
COPY --from=dockercli /usr/local/libexec/docker/cli-plugins/docker-compose /usr/local/libexec/docker/cli-plugins/docker-compose
```

实现时保留现有 Go builder 和 `/app/paneld` 入口，仅增加 Docker CLI stage 和复制步骤。

helper 容器使用当前 backend 容器的镜像 ID 启动，而不是额外依赖 `docker:cli` 镜像。这样不引入新的镜像仓库依赖，并保证 helper 内可用的 Docker CLI 与当前后端一致。

## 前端设计

在 `vite-frontend/src/pages/config.tsx` 的基本设置或数据库占用附近增加“面板升级”卡片，避免隐藏在节点页导致误解为“节点升级”。

展示内容：

1. 当前版本。
2. 最新版本。
3. 更新通道选择，复用现有 `stable` / `dev` 语义和 `UpdateReleaseChannel` 本地存储。
4. 升级能力状态：可用、不可用原因、Docker socket 高权限提示。
5. 操作按钮：检查更新、立即升级。

交互：

1. 页面加载时调用 `/system/version`。
2. 点击“检查更新”调用 `/system/check-updates`。
3. 点击“立即升级”前弹出确认框，明确提示服务会短暂中断，并提示 Docker socket 具备宿主高权限。
4. 升级请求只等待 helper 启动，超时设置为 60 秒。
5. 成功后 toast 显示“升级已触发，面板将在数十秒内重启”，并可提示用户稍后刷新。

## 安全边界

Docker socket 挂载等同于给后端容器宿主机级别控制能力。这是本设计的主要风险。缓解措施：

1. 仅 `/api/v1/system/*` 管理员接口可触发。
2. 不提供任意命令执行接口。
3. 不允许用户传入下载 URL。
4. 不允许用户传入 compose 路径。
5. 只升级本仓库 GitHub Release，且跳过 draft。
6. 前端明确展示 Docker socket 权限提示。

## 测试策略

### Go 单测

新增 `system_upgrade_test.go` 覆盖：

1. `selectComposeAsset` 对 v4/v6 compose 内容的判断。
2. `.env` 中已有 `FLUX_VERSION` 时更新值。
3. `.env` 中缺少 `FLUX_VERSION` 时追加值。
4. 缺少部署目录、`.env`、`docker-compose.yml`、Docker socket 时返回不可用原因。
5. helper 命令构造固定命令序列，不拼接用户输入。
6. 并发升级锁会拒绝第二个升级请求。

### 手动/集成验证

1. `go-backend`: `go test ./...`
2. `vite-frontend`: `pnpm run build`
3. 本地容器验证：启动 Compose 后检查设置页升级卡片可显示能力状态。
4. 在无 Docker socket 的开发环境验证按钮禁用并显示原因。

## 回滚与恢复

自动升级失败时不做自动容器回滚。后端会保留：

1. `.env.upgrade.bak`
2. `docker-compose.yml.upgrade.bak`

人工恢复步骤由错误信息提示：进入部署目录，按需恢复备份文件，再执行 `docker compose up -d backend frontend`。

## 决策记录

本设计已确定采用 Docker socket 整套升级方案，不再保留二进制替换作为本次实现路径。Docker socket 的权限风险通过管理员限制、命令白名单和前端提示控制。
