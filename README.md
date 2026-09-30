# FLVX

> **联系我们**: [Telegram群组](https://t.me/flvxchannel)


## 特性

- 支持按 **隧道账号级别** 管理流量转发数量，可用于用户/隧道配额控制
- 支持 **TCP** 和 **UDP** 协议的转发
- 支持两种转发模式：**端口转发** 与 **隧道转发**
- 可针对 **指定用户的指定隧道进行限速** 设置
- 支持配置 **单向或双向流量计费方式**，灵活适配不同计费模型
- 提供灵活的转发策略配置，适用于多种网络场景
- 面板分享，支持将节点分享给其他人，面板对接面板
- 支持分组权限管理，隧道分组、用户分组
- 支持批量功能，可以批量下发配置，启停等
- 支持隧道修改配置、转发修改隧道


## 部署流程
---
### Docker Compose部署
#### 快速部署（安装最新版）
面板端：
```bash
curl -L https://raw.githubusercontent.com/Sagit-chu/flux-panel/main/panel_install.sh -o panel_install.sh && chmod +x panel_install.sh && ./panel_install.sh
```
节点端：
```bash
curl -L https://raw.githubusercontent.com/Sagit-chu/flux-panel/main/install.sh -o install.sh && chmod +x install.sh && ./install.sh
```

#### 安装特定版本
从 [Releases](https://github.com/Sagit-chu/flux-panel/releases) 页面复制对应版本的安装命令，脚本会自动安装该版本而非最新版。

面板端（以 2.1.9-beta6 为例）：
```bash
curl -L https://github.com/Sagit-chu/flux-panel/releases/download/2.1.9-beta6/panel_install.sh -o panel_install.sh && chmod +x panel_install.sh && ./panel_install.sh
```
节点端（以 2.1.9-beta6 为例）：
```bash
curl -L https://github.com/Sagit-chu/flux-panel/releases/download/2.1.9-beta6/install.sh -o install.sh && chmod +x install.sh && ./install.sh
```

#### PostgreSQL 部署（Docker Compose）

安装脚本会根据环境自动下载对应的 Compose 配置并保存为 `docker-compose.yml`。默认仍使用 SQLite，切换到 PostgreSQL 只需要配置环境变量。

1) 在 `docker-compose` 同目录创建或修改 `.env`：

```bash
JWT_SECRET=replace_with_your_secret
BACKEND_PORT=6365
FRONTEND_PORT=6366

DB_TYPE=postgres
DATABASE_URL=postgres://flux_panel:replace_with_strong_password@postgres:5432/flux_panel?sslmode=disable

POSTGRES_DB=flux_panel
POSTGRES_USER=flux_panel
POSTGRES_PASSWORD=replace_with_strong_password
```

> 📌 使用安装脚本部署时，`POSTGRES_PASSWORD` 会自动随机生成并写入 `.env`。

2) 启动服务：

```bash
docker compose up -d
```

3) 如果你想继续使用 SQLite，保留 `DB_TYPE=sqlite`（或不设置 `DB_TYPE`）即可。

#### 通行证密钥登录（可选）

通行证密钥登录默认关闭，需要管理员在部署环境中主动启用。它不依赖 Cloudflare Turnstile，但用户必须在验证码可用时先用密码登录并绑定密钥；未绑定的账号不能用此功能绕过验证码。原密码登录始终可用。

**新部署与旧版本升级：**新安装生成的 `.env` 默认没有 `FLVX_WEBAUTHN_ORIGIN`。升级到支持通行证密钥的版本也**不会自动启用**：安装脚本保留已有 `.env`，不会向其中添加该变量。如果手动升级时保留了旧版 `docker-compose.yml`，还需检查 `backend.environment` 是否包含下文的变量传递项。未设置该变量时，仅更新镜像或版本号后看不到入口是预期行为。

1. 在实际部署目录的 `.env` 中添加浏览器访问面板时的公开前端 origin，将示例域名换成自己的域名：

   ```dotenv
   FLVX_WEBAUTHN_ORIGIN=https://panel.example.com
   ```

   必须使用完整 HTTPS origin（协议、域名、必要时的端口），不能带路径、查询参数或尾部 `/`。仅本机开发可用 `http://localhost:3000` 或 `http://127.0.0.1:3000`。经反向代理访问时仍填浏览器地址栏中的**前端** origin，不填容器地址、后端监听地址或代理转发头；API 即使在另一个域名，也不改变此值。自行部署时将该变量传给 `paneld` 进程。

2. 确认实际使用的 `docker-compose.yml` 在 `backend` 服务的 `environment` 下有以下传递项；新版 Compose 模板已包含，旧模板若缺失需补上：

   ```yaml
   FLVX_WEBAUTHN_ORIGIN: ${FLVX_WEBAUTHN_ORIGIN:-}
   ```

   在该部署目录运行 `docker compose up -d --force-recreate backend`，使新环境变量进入后端容器。后端重建期间，面板 API 会短暂中断。容器恢复后重新加载前端页面；如果 PWA 提示“发现新版本”，选择“刷新”，也可以用新的无痕窗口核对是否仍加载旧页面。

3. 用户照常用密码登录，桌面端点右上角“个人资料”，手机端点底部“我的”（均进入 `/profile`）；在页面顶部的“通行证密钥”卡片输入**当前密码**、可选名称，点击“绑定通行证密钥”并完成设备解锁。新绑定要求设备支持可发现凭据（resident key），不支持的设备会在绑定时失败。绑定或删除均需当前密码，且用户只能管理自己账号的密钥。

4. 以后在登录页直接点击“选择通行证密钥登录”，由设备选择账号并解锁，**无需先填写用户名**。账号被停用时密钥登录也会被拒绝。撤销密钥仍到个人资料页输入当前密码并删除。

   旧版本绑定时只“优先”请求可发现凭据，现有密钥是否能直接选账号取决于当时的设备，不能按绑定版本统一判定或批量删除。若设备不显示某把旧密钥，请使用密码登录，在个人资料页删除该密钥并重新绑定；确认新密钥与密码恢复途径可用后再清理其他旧密钥。登录页不提供用户名密钥回退。

**入口未显示时，按顺序检查：**

1. 在部署目录运行 `docker compose exec backend printenv FLVX_WEBAUTHN_ORIGIN`，仅检查这一项是否为预期的公开前端 origin；不要贴出完整 `docker compose config` 或其他环境变量，以免泄露密码和密钥。若为空，检查 `.env` 和实际 Compose 模板的 `backend.environment`，然后按步骤 2 重建后端。
2. 向浏览器实际调用的后端地址发送公开状态请求，例如将以下示例地址替换为自己的 API 地址后运行 `curl -sS -X POST 'https://api.example.com/api/v1/user/passkey/status' -H 'Content-Type: application/json' -d '{}'`。应返回 `code: 0` 且 `data.enabled: true`；若为 `false`，后端尚未接受有效配置。此接口无需登录或提供密码。
3. 在访问面板的浏览器控制台检查 `window.isSecureContext` 为 `true`，且 `typeof window.PublicKeyCredential` 不是 `"undefined"`。普通公网 HTTP 页面或不支持 WebAuthn 的浏览器不会显示入口。确认后刷新页面，并查看 `/api/v1/user/passkey/status` 的实际网络响应。
4. 若状态已启用且浏览器满足条件，检查 `docker compose images frontend` 中的前端镜像版本，并按步骤 2 的 PWA 提示刷新或使用无痕窗口。登录页按钮与个人资料页卡片均不存在时，通常是旧前端页面或状态请求未成功；只有其中一个缺失时，记录页面地址和该请求的响应再排查。

**安全配置与恢复：**未设置或设置错误时，通行证密钥接口拒绝新的绑定和登录，登录页与个人资料页不显示对应入口；密码登录不受影响。不要把不可信的请求 `Host` / `X-Forwarded-Host` 当作可信 origin。域名变化会改变 RP ID，旧域名下绑定的密钥不能用于新域名；迁移时需保留原域名或可用的密码登录途径，再在新域名重新绑定。仅更改同一域名的端口也需同步更新此 origin。正在进行的密钥挑战只保存在后端内存中，重启后需重新开始；多副本部署需要共享会话方案，未实现前请保持单个后端实例。

建议保留至少一种可用的密码恢复途径。数据库级备份会保留密钥记录；当前面板的 JSON 导出不包含通行证密钥，使用 JSON 导入迁移后，用户需要通过密码登录并重新绑定。

#### 从 SQLite 迁移到 PostgreSQL

如果你是通过 `panel_install.sh` 安装面板，推荐直接使用脚本菜单一键迁移：

```bash
./panel_install.sh
# 选择 4. 迁移到 PostgreSQL
```

脚本会自动完成 SQLite 备份、PostgreSQL 启动、`pgloader` 导入、`.env` 中 `DB_TYPE`/`DATABASE_URL` 更新，并重启服务。

如果你希望手动迁移，以下示例基于 Docker Volume `sqlite_data`（项目默认配置）与 `pgloader`：

1) 停止服务并备份 SQLite 数据：

```bash
docker compose down
docker run --rm -v sqlite_data:/data -v "$(pwd)":/backup alpine sh -c "cp /data/gost.db /backup/gost.db.bak"
```

2) 仅启动 PostgreSQL：

```bash
docker compose up -d postgres
```

3) 使用 `pgloader` 迁移：

```bash
source .env
docker run --rm --network gost-network -v sqlite_data:/sqlite dimitri/pgloader:latest pgloader /sqlite/gost.db "postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}"
```

4) 切换后端到 PostgreSQL 并启动：

```bash
source .env
export DB_TYPE=postgres
export DATABASE_URL="postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}?sslmode=disable"
docker compose up -d
```

5) 迁移完成后，登录面板检查用户、隧道、转发、节点数据是否正确。

#### 默认管理员账号

- **账号**: admin_user
- **密码**: admin_user

> ⚠️ 首次登录后请立即修改默认密码！

---
## Original Project
- **Name**: flux-panel
- **Source**: https://github.com/bqlpfy/flux-panel
- **License**: Apache License 2.0

## Modifications
This fork (FLVX) is no longer a light patch on top of the upstream project. It has been deeply reworked, with both backend and frontend rebuilt around a Go-based architecture.

### 1. Backend (Rewritten)
- **Removed**: The original `springboot-backend/` (Java/Spring Boot) implementation.
- **Added**: A fully rewritten `go-backend/` service (Go), including updated data and API handling for panel management.

### 2. Frontend (Reworked)
- **Reworked**: `vite-frontend/` has been substantially rebuilt to match the new backend contract and current UI layer architecture.
- **Updated**: Dashboard pages/components and interaction flows for the current React/Vite stack.

### 3. Forwarding Stack (Modified)
- **Modified**: `go-gost/` forwarding agent wrapper.
- **Modified**: `go-gost/x/` local fork of `github.com/go-gost/x`.

### 4. Mobile Clients (Removed)
- **Removed**: `android-app/` source code.
- **Removed**: `ios-app/` source code.

### 5. Deployment & Project Infrastructure
- **Updated**: Docker deployment templates and installer output flow (IPv4/IPv6 compose variants).
- **Updated**: Release installation scripts (`install.sh`, `panel_install.sh`) and supporting automation.
- **Added/Updated**: Project-level engineering documentation (for example `AGENTS.md`).

---


## 免责声明

本项目仅供个人学习与研究使用，基于开源项目进行二次开发。  

使用本项目所带来的任何风险均由使用者自行承担，包括但不限于：  

- 配置不当或使用错误导致的服务异常或不可用；  
- 使用本项目引发的网络攻击、封禁、滥用等行为；  
- 服务器因使用本项目被入侵、渗透、滥用导致的数据泄露、资源消耗或损失；  
- 因违反当地法律法规所产生的任何法律责任。  

本项目为开源的流量转发工具，仅限合法、合规用途。  
使用者必须确保其使用行为符合所在国家或地区的法律法规。  

**作者不对因使用本项目导致的任何法律责任、经济损失或其他后果承担责任。**  
**禁止将本项目用于任何违法或未经授权的行为，包括但不限于网络攻击、数据窃取、非法访问等。**  

如不同意上述条款，请立即停止使用本项目。  

作者对因使用本项目所造成的任何直接或间接损失概不负责，亦不提供任何形式的担保、承诺或技术支持。  


请务必在合法、合规、安全的前提下使用本项目。

---
## ⭐ 喝杯咖啡！（USDT）

| 网络       | 地址                                                                 |
|------------|----------------------------------------------------------------------|
| BNB(BEP20) | `0x271327ce49140e670eA0F772d9886BF90E9022Ee`                          |
| TRC20      | `TARxZWggaxFqYgxGVBxPkyykgYKNmGndmE`                                  |
| polygon    |  `0x271327ce49140e670eA0F772d9886BF90E9022Ee`    |
