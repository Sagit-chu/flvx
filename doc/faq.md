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

### Q6: 如何切换到 PostgreSQL？
**A**: 在 `.env` 文件中设置 `DB_TYPE=postgres`，并让 `DATABASE_URL` 与 `POSTGRES_*` 保持一致，然后执行 `docker compose up -d` 重启服务即可。使用安装脚本部署时，`POSTGRES_PASSWORD` 会自动随机生成并写入 `.env`。详见 [PostgreSQL 数据库指南](./postgresql.md)。

### Q7: 从 SQLite 迁移到 PostgreSQL 后数据丢失？
**A**:
1. 确认迁移前已备份 SQLite 文件（`gost.db.bak`）。
2. 确认 `pgloader` 命令执行成功，检查其输出是否有报错。
3. 确认 `.env` 中 `DATABASE_URL` 的密码与 `POSTGRES_PASSWORD` 一致。
4. 详细迁移步骤参考 [PostgreSQL 数据库指南 - 从 SQLite 迁移](./postgresql.md)。

### Q8: PostgreSQL 容器启动失败？
**A**:
1. 检查 `POSTGRES_PASSWORD` 是否已设置（不能为空）。
2. 查看容器日志：`docker logs flux-panel-postgres`。
3. 如果是首次启动后修改了密码，需要删除旧的数据卷重新初始化：`docker volume rm postgres_data`。
