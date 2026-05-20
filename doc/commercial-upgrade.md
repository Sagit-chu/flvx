# 商业版客户升级手册

本文档用于后续给客户升级 FLVX 商业版。目标是做到：只升级程序，不覆盖客户数据；升级前可备份，升级后可验证，异常时可回滚。

## 一、升级原则

- 不直接替换客户数据库。
- 不删除客户 Docker volume。
- 不把内部测试数据、演示数据、授权中心管理端数据带到客户服务器。
- 每次升级必须生成唯一版本号，例如 `commercial-20260520010149`。
- 每次升级必须保留上一版镜像和 `docker-compose.yaml` 备份，方便回滚。
- 涉及支付、授权、订单、资源发放的版本，升级后必须做完整 smoke test。

## 二、客户侧需要准备的信息

升级前需要确认以下信息：

| 项目 | 示例 | 说明 |
| --- | --- | --- |
| 服务器 IP | `1.2.3.4` | 客户服务器公网 IP |
| SSH 端口 | `22` | 默认 22 |
| SSH 用户 | `root` | 推荐使用 root 或具备 Docker 权限的用户 |
| SSH 密码/密钥 | 不写入文档 | 只在执行升级时临时使用 |
| 面板域名 | `https://panel.example.com` | 用于升级后访问验证 |
| Compose 路径 | `/root/docker-compose.yaml` | 以实际服务器为准 |
| 数据库类型 | SQLite/PostgreSQL | 确认数据卷或数据库连接 |

如果客户使用 PostgreSQL，升级前还要确认 PostgreSQL 容器或外部数据库没有被一并重建。

## 三、升级前检查

登录客户服务器后先检查当前运行状态：

```bash
cd /root
docker compose ps
docker ps --format "{{.Names}} {{.Image}} {{.Status}}"
```

确认当前镜像版本：

```bash
grep -E "local/flvx-commercial-(backend|frontend):|FLUX_VERSION" /root/docker-compose.yaml
```

检查磁盘空间：

```bash
df -h
docker system df
```

查看最近错误日志：

```bash
docker logs --since=10m flux-panel-backend 2>&1 | tail -200
```

如果升级前已经存在大量支付回调失败、数据库错误、节点大量离线、端口占用等问题，应先记录下来，避免升级后误判为新版本问题。

## 四、备份

### 1. 备份 Compose

```bash
TAG=commercial-YYYYMMDDHHMMSS
cp /root/docker-compose.yaml /root/docker-compose.yaml.bak-$TAG
```

### 2. 备份 SQLite 数据

如果客户使用默认 SQLite，通常数据在 Docker volume 中。先确认 compose 里的挂载名称，再执行备份。

常见示例：

```bash
mkdir -p /root/flvx-backups
docker run --rm \
  -v sqlite_data:/data \
  -v /root/flvx-backups:/backup \
  alpine sh -c 'cp -a /data /backup/sqlite-data-$(date +%Y%m%d%H%M%S)'
```

如 compose 中 volume 名称不是 `sqlite_data`，必须按实际名称替换。

### 3. 备份 PostgreSQL

如果客户使用 PostgreSQL：

```bash
mkdir -p /root/flvx-backups
docker exec flux-panel-postgres pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB" \
  > /root/flvx-backups/postgres-$(date +%Y%m%d%H%M%S).sql
```

如果数据库在外部服务器，由客户在数据库侧执行备份。

## 五、准备升级包

推荐每次升级只上传商业版源码或已构建镜像，不上传本地 `.git`、`node_modules`、`dist`、临时文件。

本地打包上传示例：

```bash
TAG=commercial-YYYYMMDDHHMMSS
REMOTE_DIR=/tmp/flvx-commercial-build-$TAG

ssh root@客户IP "rm -rf $REMOTE_DIR && mkdir -p $REMOTE_DIR"

tar \
  --exclude='.git' \
  --exclude='node_modules' \
  --exclude='dist' \
  --exclude='.DS_Store' \
  -C /path/to/FLVXv1 \
  -czf - . | ssh root@客户IP "tar -xzf - -C $REMOTE_DIR"
```

如果只更新前端，可只上传 `vite-frontend`：

```bash
TAG=commercial-YYYYMMDDHHMMSS
REMOTE_DIR=/tmp/flvx-frontend-build-$TAG

ssh root@客户IP "rm -rf $REMOTE_DIR && mkdir -p $REMOTE_DIR"

tar \
  --exclude='node_modules' \
  --exclude='dist' \
  --exclude='.DS_Store' \
  -C /path/to/FLVXv1 \
  -czf - vite-frontend | ssh root@客户IP "tar -xzf - -C $REMOTE_DIR"
```

## 六、构建镜像

### 1. 前后端都升级

```bash
TAG=commercial-YYYYMMDDHHMMSS
REMOTE_DIR=/tmp/flvx-commercial-build-$TAG

cd $REMOTE_DIR
docker build -t local/flvx-commercial-backend:$TAG ./go-backend
docker build -t local/flvx-commercial-frontend:$TAG ./vite-frontend
```

### 2. 只升级前端

```bash
TAG=commercial-YYYYMMDDHHMMSS
REMOTE_DIR=/tmp/flvx-frontend-build-$TAG

cd $REMOTE_DIR
docker build -t local/flvx-commercial-frontend:$TAG ./vite-frontend
```

构建完成后确认镜像存在：

```bash
docker images --format "{{.Repository}}:{{.Tag}} {{.Size}}" | grep flvx-commercial
```

## 七、替换 Compose 镜像

### 1. 前后端都升级

```bash
TAG=commercial-YYYYMMDDHHMMSS
cp /root/docker-compose.yaml /root/docker-compose.yaml.bak-$TAG

python3 - <<PY
from pathlib import Path
import re

p = Path("/root/docker-compose.yaml")
s = p.read_text()
s = re.sub(r"local/flvx-commercial-backend:commercial-[0-9]+", f"local/flvx-commercial-backend:$TAG", s)
s = re.sub(r"local/flvx-commercial-frontend:commercial-[0-9]+", f"local/flvx-commercial-frontend:$TAG", s)
s = re.sub(r"FLUX_VERSION:\\s*commercial-[0-9]+", f"FLUX_VERSION: $TAG", s)
p.write_text(s)
PY

cd /root
docker compose up -d backend frontend
```

### 2. 只升级前端

```bash
TAG=commercial-YYYYMMDDHHMMSS
cp /root/docker-compose.yaml /root/docker-compose.yaml.bak-$TAG

python3 - <<PY
from pathlib import Path
import re

p = Path("/root/docker-compose.yaml")
s = p.read_text()
s = re.sub(r"local/flvx-commercial-frontend:commercial-[0-9]+", f"local/flvx-commercial-frontend:$TAG", s)
p.write_text(s)
PY

cd /root
docker compose up -d frontend
```

## 八、升级后验证

### 1. 容器状态

```bash
docker compose ps
docker ps --format "{{.Names}} {{.Image}} {{.Status}}" | grep -E "flux-panel-backend|vite-frontend|nginx"
```

后端应显示 `healthy`。

### 2. 路由检查

将 `https://panel.example.com` 替换为客户域名：

```bash
for url in \
  https://panel.example.com/ \
  https://panel.example.com/plans \
  https://panel.example.com/orders \
  https://panel.example.com/admin/payment-settings \
  https://panel.example.com/admin/license; do
  code=$(curl -k -sS -o /dev/null -w "%{http_code}" "$url")
  echo "$code $url"
done
```

正常情况下都应返回 `200`。如果是 SPA 路由，也必须由前端容器正确返回页面。

### 3. API 检查

```bash
curl -k -sS https://panel.example.com/api/v1/commerce/public/settings
curl -k -sS https://panel.example.com/api/v1/commerce/plans/public
```

### 4. 后端日志

```bash
docker logs --since=10m flux-panel-backend 2>&1 | tail -200
```

重点确认没有：

- 数据库迁移失败
- 支付回调验签失败持续刷屏
- 授权校验异常
- 订单开通 panic
- 资源任务大量异常

如果日志中出现旧隧道重下发失败、端口已占用、节点离线等信息，需要区分是客户原有数据/节点状态，还是升级导致的新问题。

## 九、业务测试清单

每次商业版升级后，至少用测试账号跑一遍以下链路：

1. 注册和登录
   - 普通用户可注册、登录、退出。
   - 邀请码开关行为正确。
   - 普通用户看不到后台入口。
   - 管理员能从个人资料进入 Admin。

2. 套餐商城
   - 套餐分类显示正确。
   - 套餐线路、倍率、流量、规则数量、最大连接数正确。
   - 不同线路套餐不能被错误判断成同一个套餐高低配。

3. 订单
   - 下单成功。
   - 未支付订单可继续支付。
   - 未支付订单可取消。
   - 已支付订单状态正确。

4. 支付
   - e支付可发起订单并回调开通。
   - U支付可发起订单并回调开通。
   - 金额显示为元，后台填 `1` 表示 `¥1.00`。
   - 重复回调不会重复开通。

5. 套餐资源
   - 支付成功后隧道权限、规则数量、连接数、流量额度生效。
   - 套餐到期策略符合预期。
   - 资源任务失败时后台可查看和重试。

6. 重置流量
   - 重置价格按当前套餐配置读取。
   - 支付成功后恢复当前套餐流量。

7. 监控权限
   - 普通用户只看到已购买套餐包含的节点/隧道监控。
   - 管理员不受限制。

8. 授权
   - 商业版能正常校验授权。
   - 授权过期/无效时提示正常。
   - 客户安装端不显示授权中心管理页面。

## 十、回滚方案

如果升级后出现阻断问题，先回滚 compose，再重启容器。

```bash
BACKUP=/root/docker-compose.yaml.bak-commercial-YYYYMMDDHHMMSS
cp "$BACKUP" /root/docker-compose.yaml
cd /root
docker compose up -d backend frontend
```

回滚后验证：

```bash
docker compose ps
docker logs --since=5m flux-panel-backend 2>&1 | tail -120
```

如果升级过程没有删除旧镜像，回滚通常只需要几十秒。

## 十一、升级完成后清理

确认客户已经验收后，清理临时源码目录：

```bash
rm -rf /tmp/flvx-commercial-build-commercial-YYYYMMDDHHMMSS
rm -rf /tmp/flvx-frontend-build-commercial-YYYYMMDDHHMMSS
```

不建议立刻删除上一版镜像。至少保留一个稳定旧版本用于回滚。

需要清理很老的镜像时，先确认当前运行镜像：

```bash
docker ps --format "{{.Names}} {{.Image}}"
docker images --format "{{.Repository}}:{{.Tag}} {{.Size}}" | grep flvx-commercial
```

再手动删除明确不用的旧镜像：

```bash
docker rmi local/flvx-commercial-frontend:commercial-旧版本
docker rmi local/flvx-commercial-backend:commercial-旧版本
```

## 十二、常见问题

### 1. 页面还是旧版本

可能是浏览器或 PWA 缓存。让客户强制刷新，或清理站点缓存后再打开。

### 2. 后端 healthy 失败

先看日志：

```bash
docker logs --since=10m flux-panel-backend 2>&1 | tail -200
```

常见原因：

- 数据库连接失败
- 配置环境变量缺失
- 数据迁移失败
- 端口被占用

### 3. 支付成功但没开通

检查：

- 支付平台异步通知地址是否是公网 HTTPS 地址。
- e支付/U支付密钥是否填写正确。
- 订单金额是否一致。
- 后端日志是否有验签失败。
- `provider_trade_no` 是否重复。

### 4. U支付无法跳转

检查：

- `U支付网关` 是否填写到站点根地址，例如 `https://pay.example.com`。
- `U支付商户PID` 是否正确。
- `U支付商户密钥` 是否已保存。
- 币种、网络是否符合收款平台要求，例如 `usdt`、`tron`。

### 5. 升级后节点或隧道异常

先区分是升级前已存在，还是升级后新增。检查：

```bash
docker logs --since=30m flux-panel-backend 2>&1 | grep -E "post-upgrade|redeploy|failed|端口|占用|node|tunnel" | tail -100
```

如果是端口已占用或转发链目标为空，通常需要修客户具体隧道配置或节点运行状态，不应直接判定为升级失败。

## 十三、交付记录模板

每次给客户升级后，建议按以下格式记录：

```text
客户：
域名：
服务器：
升级时间：
升级前后端版本：
升级前前端版本：
升级后后端版本：
升级后前端版本：
Compose 备份：
数据库备份：
验证结果：
发现的问题：
是否需要客户继续测试：
回滚版本：
```

