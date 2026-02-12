# PostgreSQL 数据库指南

FLVX 默认使用 SQLite 作为数据库，同时也内置了对 PostgreSQL 的完整支持。本文档介绍如何使用 PostgreSQL 部署面板、从 SQLite 迁移以及日常维护。

## 一、SQLite 与 PostgreSQL 对比

| 特性 | SQLite | PostgreSQL |
|------|--------|------------|
| **部署复杂度** | 零配置，开箱即用 | 需要额外的数据库服务 |
| **并发性能** | 适合小规模单机使用 | 支持高并发读写 |
| **数据规模** | 适合中小规模数据 | 适合大规模数据 |
| **备份与恢复** | 直接复制文件 | 支持逻辑备份与物理备份 |
| **高可用** | 不支持 | 支持主从复制、流复制 |

**建议**：如果你只是个人使用或小团队使用，SQLite 完全够用。如果节点多，推荐使用 PostgreSQL。

---

## 二、环境变量说明

以下环境变量用于配置数据库连接，在 `.env` 文件或 Docker Compose `environment` 中设置。

### 后端服务 (backend) 使用

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `DB_TYPE` | 数据库类型，`sqlite` 或 `postgres` | `sqlite` | `postgres` |
| `DATABASE_URL` | PostgreSQL 连接字符串（仅 `DB_TYPE=postgres` 时必填） | 空 | `postgres://flux_panel:密码@postgres:5432/flux_panel?sslmode=disable` |
| `DB_PATH` | SQLite 数据库文件路径（仅 `DB_TYPE=sqlite` 时使用） | `/app/data/gost.db` | `/app/data/gost.db` |

### PostgreSQL 容器使用

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `POSTGRES_DB` | 数据库名称 | `flux_panel` |
| `POSTGRES_USER` | 数据库用户名 | `flux_panel` |
| `POSTGRES_PASSWORD` | 数据库密码 | `flux_panel_change_me` |

> ⚠️ **安全提示**：生产环境中请务必修改 `POSTGRES_PASSWORD` 为强密码，不要使用默认值！

---

## 三、全新部署（Docker Compose + PostgreSQL）

`docker-compose-v4.yml` / `docker-compose-v6.yml` 已包含 PostgreSQL 服务，默认使用 SQLite。只需配置环境变量即可切换到 PostgreSQL。

### 1. 创建 `.env` 文件

在 `docker-compose` 同目录创建 `.env` 文件：

```bash
# 基础配置
JWT_SECRET=替换为你的密钥
BACKEND_PORT=6365
FRONTEND_PORT=6366

# PostgreSQL 配置
DB_TYPE=postgres
DATABASE_URL=postgres://flux_panel:替换为强密码@postgres:5432/flux_panel?sslmode=disable

POSTGRES_DB=flux_panel
POSTGRES_USER=flux_panel
POSTGRES_PASSWORD=替换为强密码
```

> ⚠️ `DATABASE_URL` 中的密码必须与 `POSTGRES_PASSWORD` 保持一致。

### 2. 启动服务

IPv4 环境：
```bash
docker compose -f docker-compose-v4.yml up -d
```

IPv6 环境：
```bash
docker compose -f docker-compose-v6.yml up -d
```

### 3. 验证

```bash
# 检查所有容器是否正常运行
docker ps

# 查看后端日志，确认连接 PostgreSQL 成功
docker logs flux-panel-backend

# 查看 PostgreSQL 容器状态
docker logs flux-panel-postgres
```

---

## 四、从 SQLite 迁移到 PostgreSQL

如果你已经在使用 SQLite 并且希望迁移到 PostgreSQL，请按照以下步骤操作。

### 1. 备份 SQLite 数据

```bash
# 停止所有服务
docker compose -f docker-compose-v4.yml down

# 备份 SQLite 数据文件到当前目录
docker run --rm -v sqlite_data:/data -v "$(pwd)":/backup alpine sh -c "cp /data/gost.db /backup/gost.db.bak"
```

### 2. 配置 PostgreSQL 环境变量

在 `.env` 文件中添加 PostgreSQL 配置（参考上方"环境变量说明"）。

### 3. 仅启动 PostgreSQL

```bash
docker compose -f docker-compose-v4.yml up -d postgres
```

等待 PostgreSQL 完全就绪：

```bash
# 检查 PostgreSQL 健康状态
docker inspect --format='{{.State.Health.Status}}' flux-panel-postgres
# 输出 "healthy" 表示就绪
```

### 4. 使用 pgloader 迁移数据

```bash
docker run --rm \
  --network gost-network \
  -v sqlite_data:/sqlite \
  dimitri/pgloader:latest \
  pgloader /sqlite/gost.db postgresql://flux_panel:你的密码@postgres:5432/flux_panel
```

> 📌 将 `你的密码` 替换为实际的 `POSTGRES_PASSWORD`。

### 5. 启动全部服务

```bash
docker compose -f docker-compose-v4.yml up -d
```

### 6. 验证迁移

登录面板后，检查以下数据是否完整：
- 用户列表和权限
- 节点信息和状态
- 隧道配置
- 转发规则
- 流量统计数据

---

## 五、独立 PostgreSQL（非 Docker）

如果你不想使用 Docker Compose 中自带的 PostgreSQL 容器，也可以连接外部的 PostgreSQL 实例。

### 1. 准备 PostgreSQL

在目标 PostgreSQL 服务器上创建数据库和用户：

```sql
CREATE USER flux_panel WITH PASSWORD '你的强密码';
CREATE DATABASE flux_panel OWNER flux_panel;
```

### 2. 配置连接

修改 `.env` 文件，将 `DATABASE_URL` 指向外部 PostgreSQL：

```bash
DB_TYPE=postgres
DATABASE_URL=postgres://flux_panel:你的强密码@数据库地址:5432/flux_panel?sslmode=disable
```

> 📌 如果 PostgreSQL 在远程服务器且启用了 SSL，请将 `sslmode=disable` 改为 `sslmode=require` 或 `sslmode=verify-full`。

### 3. 停用内置 PostgreSQL 容器（可选）

如果使用外部 PostgreSQL，可以在启动时不启动内置的 postgres 服务：

```bash
docker compose -f docker-compose-v4.yml up -d backend frontend
```

---

## 六、数据备份与恢复

### 逻辑备份（pg_dump）

```bash
# 备份（在 Docker 环境下）
docker exec flux-panel-postgres pg_dump -U flux_panel flux_panel > backup_$(date +%Y%m%d_%H%M%S).sql

# 恢复
docker exec -i flux-panel-postgres psql -U flux_panel flux_panel < backup_20260101_120000.sql
```

### 定时备份（cron）

创建备份脚本 `/opt/flvx/backup.sh`：

```bash
#!/bin/bash
BACKUP_DIR="/opt/flvx/backups"
mkdir -p "$BACKUP_DIR"
docker exec flux-panel-postgres pg_dump -U flux_panel flux_panel | gzip > "$BACKUP_DIR/flvx_$(date +%Y%m%d_%H%M%S).sql.gz"
# 清理 30 天前的备份
find "$BACKUP_DIR" -name "flvx_*.sql.gz" -mtime +30 -delete
```

添加 cron 任务（每天凌晨 3 点执行）：

```bash
chmod +x /opt/flvx/backup.sh
echo "0 3 * * * /opt/flvx/backup.sh" | crontab -
```

---

## 七、常见问题

### Q: 切换到 PostgreSQL 后启动失败，提示连接被拒绝？

**A**: 
1. 确认 PostgreSQL 容器已启动并处于 `healthy` 状态：`docker ps`。
2. 确认 `DATABASE_URL` 中的主机名、端口、用户名、密码正确。
3. 在 Docker Compose 环境下，主机名应为 `postgres`（服务名），而非 `localhost`。

### Q: pgloader 迁移时报错？

**A**: 
1. 确认 PostgreSQL 容器已完全就绪（状态为 `healthy`）。
2. 确认 `--network gost-network` 参数正确，使 pgloader 容器与 PostgreSQL 在同一网络中。
3. 如果数据库已有表结构，pgloader 可能会报冲突。可以先清空目标数据库后重试。

### Q: 如何查看当前使用的数据库类型？

**A**: 查看后端容器的 `DB_TYPE` 环境变量：

```bash
docker exec flux-panel-backend printenv DB_TYPE
```

### Q: 可以同时使用 SQLite 和 PostgreSQL 吗？

**A**: 不可以。`DB_TYPE` 只能设置为 `sqlite` 或 `postgres` 之一。后端启动时根据此配置连接对应的数据库。

### Q: PostgreSQL 数据存储在哪里？

**A**: 在 Docker Compose 部署中，PostgreSQL 数据存储在名为 `postgres_data` 的 Docker Volume 中。可以通过以下命令查看：

```bash
docker volume inspect postgres_data
```
