# FLVX

> **联系我们**: [Telegram群组](https://t.me/flvxpanel)

## Original Project
- **Name**: flux-panel
- **Source**: https://github.com/bqlpfy/flux-panel
- **License**: Apache License 2.0

## Modifications
The following major changes and additions have been made in this fork (FLVX):

### 1. Backend Architecture (Replaced)
- **Removed**: The original `springboot-backend/` (Java/Spring Boot) has been entirely removed.
- **Added**: A new `go-backend/` (Go/SQLite) implementation replaces the original backend.

### 2. Forwarding Agent (Modified)
- **Modified**: `go-gost/` - Modified forwarding agent wrapper.
- **Modified**: `go-gost/x/` - Modified local fork of the `gost` extensions library.

### 3. Frontend (Modified)
- **Modified**: `vite-frontend/` - Significant updates to the React/Vite dashboard to compatible with the new Go backend, including UI/UX improvements (HeroUI + Tailwind).

### 4. Mobile Applications (Removed)
- **Removed**: `android-app/` - Source code for the Android client.
- **Removed**: `ios-app/` - Source code for the iOS client.

### 5. Infrastructure & Scripts
- **Modified**: `docker-compose-v4.yml`, `docker-compose-v6.yml` (Updated for Go backend).
- **Modified**: `install.sh`, `panel_install.sh` (Updated installation logic).
- **Added**: `AGENTS.md` (Project documentation).

---
## 特性

- 支持按 **隧道账号级别** 管理流量转发数量，可用于用户/隧道配额控制
- 支持 **TCP** 和 **UDP** 协议的转发
- 支持两种转发模式：**端口转发** 与 **隧道转发**
- 可针对 **指定用户的指定隧道进行限速** 设置
- 支持配置 **单向或双向流量计费方式**，灵活适配不同计费模型
- 提供灵活的转发策略配置，适用于多种网络场景


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

#### 默认管理员账号

- **账号**: admin_user
- **密码**: admin_user

> ⚠️ 首次登录后请立即修改默认密码！


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
