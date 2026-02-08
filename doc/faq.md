# 常见问题 (FAQ)

### Q1: 安装脚本提示 "Docker command not found"？
**A**: 请确保您的系统已安装 Docker 和 Docker Compose。
- Ubuntu/Debian 安装 Docker: `curl -fsSL https://get.docker.com | bash`

### Q2: 面板无法访问 (Connection Refused)？
**A**:
1. 检查防火墙是否放行了前端端口（默认 `6366`）。
2. 检查容器是否正常运行: `docker ps`。
3. 查看容器日志: `docker logs flux-panel-backend` 或 `docker logs vite-frontend`。

### Q3: 节点显示离线？
**A**:
1. 检查节点服务器与面板服务器之间的网络连通性。
2. 确认在节点端安装时输入的 **面板地址** 和 **密钥** 是否正确。
3. 检查节点端服务状态: `systemctl status flux_agent`。
4. 查看节点端日志: `journalctl -u flux_agent -f`。

### Q4: 只有 TCP 能通，UDP 不通？
**A**: 请检查服务器防火墙和安全组（AWS/阿里云/腾讯云等）是否同时放行了对应端口的 **TCP 和 UDP** 协议。

### Q5: IPv6 无法使用？
**A**: 面板安装脚本会自动尝试配置 Docker 的 IPv6。如果失败，请手动检查 `/etc/docker/daemon.json` 配置，确保 `ipv6: true` 且分配了正确的 `fixed-cidr-v6` 子网。
