# FLVX

> **联系我们**: [Telegram群组](https://t.me/flvxpanel)


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

面板端（以 2.1.0 为例）：
```bash
curl -L https://github.com/Sagit-chu/flux-panel/releases/download/2.1.0/panel_install.sh -o panel_install.sh && chmod +x panel_install.sh && ./panel_install.sh
```
节点端（以 2.1.0 为例）：
```bash
curl -L https://github.com/Sagit-chu/flux-panel/releases/download/2.1.0/install.sh -o install.sh && chmod +x install.sh && ./install.sh
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
| BNB(BEP20) | `0xa608708fdc6279a2433fd4b82f0b72b8cbe97ed5`                          |
| TRC20      | `TM8VYdU3s3gSX5PC8swjAJrAzZFCHKqG2k`                                  |
| Aptos      | `0x49427bfcba1006a346447430689b2307ac156316bb34850d1d3029ff9d118da5`  |
| polygon    |  `0xa608708fdc6279a2433fd4b82f0b72b8cbe97ed5`    |
