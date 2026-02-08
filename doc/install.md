# 安装部署指南

本文档介绍如何部署 FLVX 面板端及节点端。

## 一、面板端部署

面板端负责管理用户、节点和转发规则。

### 1. 环境要求
- 操作系统：Linux (推荐 Debian 10+ / Ubuntu 20.04+)
- 必须安装 Docker 和 Docker Compose

### 2. 一键安装脚本

使用以下命令即可快速安装面板：

```bash
curl -L https://raw.githubusercontent.com/Sagit-chu/flux-panel/main/panel_install.sh -o panel_install.sh && chmod +x panel_install.sh && ./panel_install.sh
```

**安装过程中会提示输入以下信息：**
- **前端端口**: 默认为 `6366`
- **后端端口**: 默认为 `6365`

脚本会自动检测系统是否支持 IPv6，并自动配置 Docker 的 IPv6 支持。

### 3. 访问面板

安装完成后，访问：
`http://<服务器IP>:<前端端口>` (默认: `http://<服务器IP>:6366`)

**默认管理员账号：**
- 用户名: `admin_user`
- 密码: `admin_user`

> ⚠️ **注意**: 首次登录后，请务必在“个人中心”或“设置”中修改默认密码！

### 4. 维护命令

再次运行 `./panel_install.sh` 脚本可以看到管理菜单：
1. 安装面板
2. 更新面板
3. 卸载面板

---

## 二、节点端部署

节点端运行在实际进行流量转发的服务器上，需要连接到面板端进行管理。

### 1. 获取接入密钥
1. 登录面板端。
2. 进入 **节点管理 (Node)** 页面。
3. 点击 **添加节点**。
4. 获取该节点的 **接入密钥 (Secret)**。

### 2. 一键安装脚本

在节点服务器上运行：

```bash
curl -L https://raw.githubusercontent.com/Sagit-chu/flux-panel/main/install.sh -o install.sh && chmod +x install.sh && ./install.sh
```

**安装过程中会提示输入：**
- **服务器地址**: 面板端的通信地址（通常是 `http://<面板IP>:<后端端口>`，例如 `http://1.2.3.4:6365`）。
- **密钥**: 刚才在面板中获取的节点密钥。

或者直接使用带参数的命令（适用于自动化部署）：

```bash
# 替换 <面板地址> 和 <密钥>
./install.sh -a "http://1.2.3.4:6365" -s "your_node_secret"
```

### 3. 验证安装
安装完成后，服务会自动启动。
- 查看状态: `systemctl status flux_agent`
- 回到面板 **节点管理** 页面，该节点状态应显示为 **在线**。
