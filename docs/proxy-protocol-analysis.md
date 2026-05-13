# Proxy Protocol 传输分析报告

**日期**: 2026-05-07  
**测试环境**: 20.118.172.127 (Server 1) ↔ 108.181.90.137 (Server 2)

---

## 1. 代码流程分析

### 完整数据链路

```
前端 (proxyProtocol: 0|1|2)
  → 后端 handler mutations.go:1936
  → 数据库存储 forward.proxy_protocol (model.go:50)
  → 控制面 buildForwardServiceConfigs (control_plane.go:1791-1796)
  → handler metadata: {"proxyProtocol": 2}
  → Agent metadata 解析 (metadata.go:42)
  → handler.go:256 WrapClientConn()
  → conn.go:14 HeaderProxyFromAddrs(byte(ppv), src, dst)
  → conn.go:15 header.WriteTo(c)
  → 目标服务器收到 PROXY protocol header
```

### 关键代码

**写入 PROXY header** (`go-gost/x/internal/net/proxyproto/conn.go`):
```go
func WrapClientConn(ppv int, src, dst net.Addr, c net.Conn) net.Conn {
    if ppv <= 0 {
        return c
    }
    header := proxyproto.HeaderProxyFromrs(byte(ppv), src, dst)
    header.WriteTo(c)
    return c
}
```

**Handler 调用** (`go-gost/x/handler/forward/local/handler.go:256`):
```go
cc = proxyproto.WrapClientConn(h.md.proxyProtocol, conn.RemoteAddr(), conn.LocalAddr(), cc)
```

- `src` = `conn.RemoteAddr()` → 客户端真实 IP ✅
- `dst` = `conn.LocalAddr()` → agent 监听地址 ✅
- `ppv` = 1 或 2 → 版本号正确 ✅

---

## 2. 实际传输测试结果

### 测试方法

1. 在 Server 2 启动 Python TCP 监听器，解析 PROXY protocol header
2. 在 Server 1 用当前代码编译 gost，配置 `proxyProtocol: 2` 转发到 Server 2
3. 通过 `nc` 发送测试数据，验证 Server 2 是否收到正确的 PROXY header

### 测试结果

| 版本 | 状态 | 接收到的 Header |
|------|------|----------------|
| **PPv2** | ✅ 成功 | `PP2 family=1 alen=12 SRC=127.0.0.1:45410 DST=127.0.0.1:20001` |
| **PPv1** | ✅ 成功 | `PROXY TCP4 127.0.0.1 127.0.0.1 43816 20001` |

### 测试详情

**PPv2 原始数据**:
```
Got 28 bytes
PP2 family=1 alen=12
SRC=127.0.0.1:45410 DST=127.0.0.1:20001
```

**PPv1 原始数据** (hex):
```
50524f58592054435034203132372e302e302e31203132372e302e302e312034333831362032303030310d0a
```
解码: `PROXY TCP4 127.0.0.1 127.0.0.1 43816 20001`

---

## 3. 单元测试结果

```
go-gost/x/handler/forward/local/  → TestLocalForwardHandlerSendsProxyProtocolToTarget ✅
go-backend/internal/http/handler/ → TestBuildForwardServiceConfigsSendsProxyProtocolToForwardHandler ✅
go-backend/internal/store/repo/   → TestGetForwardRecordIncludesProxyProtocol ✅
```

全部通过 (3/3)。

---

## 4. 发现的问题

### 问题 1: `WriteTo` 错误未检查

**位置**: `go-gost/x/internal/net/proxyproto/conn.go:15`

```go
header.WriteTo(c)  // 返回 (int64, error) 被忽略
```

**影响**: 如果写入失败（连接已断开、网络错误等），后续数据传输会在没有 PROXY header 的情况下继续，目标服务器可能解析出错。

**建议**:
```go
if _, err := header.WriteTo(c); err != nil {
    return c  // 或包装一个带错误的 conn
}
```

**严重程度**: 低（实际场景中，写入失败后 `Transport` 也会很快失败）

---

### 问题 2: 部署版本过旧

**服务器状态**:

| 服务器 | 组件 | 版本 | 状态 |
|--------|------|------|------|
| 20.118.172.127 | flux_agent | UPX 压缩，无法读取版本 | ✅ 运行中 |
| 20.118.172.127 | paneld | `/app/paneld` | ✅ 运行中 |
| 20.118.172.127 | /usr/local/bin/gost | v3.0.0 (go1.23.4) | 旧版，不支持 handler metadata 中的 proxyProtocol |
| 108.181.90.137 | flux_agent | 8.8MB | ✅ 运行中 |

**影响**: 旧版 gost 二进制不识别 handler metadata 中的 `proxyProtocol` 字段，PROXY protocol 功能在生产环境不可用。

**验证**: 用旧版 gost 测试时，目标服务器收到的原始数据为空，无 PROXY header。

---

### 问题 3: 数据库 Schema 缺失

**位置**: 20.118.172.127 的 `/app/data/gost.db`

**当前 forward 表 schema**:
```sql
CREATE TABLE `forward` (
    `id` integer PRIMARY KEY AUTOINCREMENT,
    `user_id` integer NOT NULL,
    `user_name` varchar(100) NOT NULL,
    `name` varchar(100) NOT NULL,
    `tunnel_id` integer NOT NULL,
    `remote_addr` text NOT NULL,
    `strategy` varchar(100) NOT NULL DEFAULT "fifo",
    `in_flow` integer NOT NULL DEFAULT 0,
    `out_flow` integer NOT NULL DEFAULT 0,
    `created_time` integer NOT NULL,
    `updated_time` integer NOT NULL,
    `status` integer NOT NULL,
    `inx` integer NOT NULL DEFAULT 0,
    `speed_id` integer
);
```

**缺失字段**:
- `proxy_protocol` — PROXY protocol 版本
- `max_conn` — 最大连接数
- `ip_max_conn` — 每 IP 最大连接数
- `ip_speed_id` — 每 IP 限速 ID

**影响**: 后端无法存储和读取 proxy_protocol 配置，前端设置不会生效。

---

## 5. 结论

| 维度 | 状态 | 说明 |
|------|------|------|
| **代码实现** | ✅ 正确 | 完整的写入链路，版本/地址正确 |
| **单元测试** | ✅ 通过 | 3/3 测试覆盖 handler、repo、控制面 |
| **实际传输 (新编译版)** | ✅ 成功 | PPv1 和 PPv2 均正确传输 |
| **实际传输 (部署版)** | ❌ 不工作 | 旧版不支持 handler metadata 中的 proxyProtocol |
| **数据库 Schema** | ❌ 缺字段 | 需要迁移添加 proxy_protocol 等列 |

**总结**: 代码实现正确，PROXY protocol 传输逻辑无误。但生产服务器运行的是旧版本，需要升级 backend 和 agent 才能启用此功能。
