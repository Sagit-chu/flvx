# Agent-Panel 通信优化：提升稳定性与效率

## 背景

Agent（`go-gost/x/socket/websocket_reporter.go`）与 Panel（`go-backend/internal/ws/server.go`）之间通过 WebSocket 进行实时通信，包括指标上报（每 5s）、命令下发/响应、和流量上报（HTTP）。经过代码审查，以下是发现的问题和优化建议。

---

## 发现的问题

### 1. Keepalive 时序不匹配 —— 导致误断连

| 参数 | Agent 侧 | Panel 侧 |
|------|----------|----------|
| Read deadline | `reporterReadWait` = 60s | `wsPongWait` = 45s |
| Ping 发送间隔 | 无主动 ping（靠指标数据 5s 续命） | `wsPingPeriod` = 15s |
| Write timeout | `reporterWriteWait` = 5s | `wsWriteWait` = 5s |

**问题**：Panel 每 15s 发 ping，Agent read deadline 60s，但 Panel pong deadline 只有 45s。如果 Agent 的指标消息被延迟（网络抖动），Panel 可能因 pong 超时而关闭连接。两侧的超时参数缺乏协调设计。

### 2. 固定重连间隔 —— 无退避策略

Agent 断线后以固定 5s 间隔重试（`reconnectTime = 5 * time.Second`），在 Panel 长时间不可用（升级、网络故障）的情况下，会产生大量无用连接尝试。

### 3. Panel 侧每次解密都重建 AES 加密器

`ws/server.go` 的 `decryptIfNeeded()` 和 `SendCommand()` 每次调用都 `security.NewAESCrypto(secret)` 重新创建 cipher（SHA256 + AES-GCM 初始化），对于高频指标消息（5s/次 × N 节点），有不必要的 CPU 开销。

### 4. 指标消息使用 JSON Text 格式传输

每 5s 发送一次包含 13 个字段的 SystemInfo JSON，加密后还需 base64 编码，一条消息约 300-500 bytes（加密后约 700 bytes）。对于大量节点场景，存在优化空间。

### 5. `receiveMessages` 紧循环中有频繁锁竞争

`receiveMessages()` 在每次 `ReadMessage()` 前都要 `Lock/Unlock connMutex` 检查连接状态，但 `ReadMessage` 本身是阻塞的，实际不需要在循环外检查。

### 6. 状态变更命令阻塞读消息循环

`routeCommand` 中的 Service/Chain/Limiter CRUD 命令是同步执行的，包括 `saveConfig()` 文件写入。执行期间会阻塞 `receiveMessages` 的读取循环。

---

## 推荐的优化方案（按优先级排列）

### P0 — 高收益、低风险

#### 优化 1：协调 Keepalive 参数

**文件**：`websocket_reporter.go`

- Agent 增加独立的 WebSocket ping 发送（每 20s），不依赖指标数据来维持连接
- 统一 read deadline 设置，确保两侧 read timeout > 2×ping interval

#### 优化 2：指数退避重连

**文件**：`websocket_reporter.go`

- 初始间隔 2s，按指数退避增长至最大 2 分钟
- 连接成功后立即重置退避
- 增加随机抖动（jitter）避免大量 Agent 同时重连

#### 优化 3：Panel 侧缓存 AES 加密器

**文件**：`ws/server.go`

- 将 `AESCrypto` 实例缓存在 `nodeSession` 中，避免每条消息重建
- `SendCommand` 复用缓存实例

### P1 — 中等收益

#### 优化 4：减少 `receiveMessages` 锁竞争

**文件**：`websocket_reporter.go`

- 将连接状态检查移到循环外，只在出错/关闭时通过 channel 通知退出
- 用 `context.WithCancel` 代替锁检查 `connected` flag 来控制生命周期

#### 优化 5：异步化状态变更命令处理

**文件**：`websocket_reporter.go`

- 所有命令统一异步执行（通过 goroutine + response channel），避免阻塞 readLoop
- 当前只有 TcpPing/ServiceMonitorCheck/UpgradeAgent/RollbackAgent 是异步的

---

## 具体代码变更

### Agent 侧 (`go-gost/x/socket`)

---

#### [MODIFY] [websocket_reporter.go](file:///Users/sagit/Documents/github/flvx/go-gost/x/socket/websocket_reporter.go)

1. **指数退避重连**：将 `reconnectTime` 从固定 `5s` 改为动态退避字段，增加 `curBackoff/maxBackoff` 字段
2. **独立 Ping 发送**：在 `handleConnection()` 中增加 WebSocket ping ticker（20s），独立于指标上报
3. **减少锁竞争**：`receiveMessages` 中只在循环入口检查一次连接，此后靠 `ReadMessage` 的 error 退出
4. **统一命令异步化**：所有 `routeCommand` 调用统一使用 goroutine

---

### Panel 侧 (`go-backend/internal/ws`)

---

#### [MODIFY] [server.go](file:///Users/sagit/Documents/github/flvx/go-backend/internal/ws/server.go)

1. **缓存 AES 加密器**：在 `nodeSession` 中增加 `crypto *security.AESCrypto` 字段，节点连接时初始化
2. **`decryptIfNeeded` 接收 crypto 参数**而非 secret 字符串
3. **`SendCommand` 使用缓存 crypto** 实例

---

## Verification Plan

### Automated Tests

```bash
# 运行现有 agent 侧单元测试（验证不回归）
(cd go-gost/x && go test ./socket/... -v -count=1)

# 运行现有流量上报测试
(cd go-gost/x && go test ./service/... -v -count=1)

# 运行 panel 侧全部测试
(cd go-backend && go test ./... -count=1)
```

### Manual Verification

> [!IMPORTANT]
> 本次改动涉及实时通信核心路径，建议在 staging 环境部署后观察至少 30 分钟：
> 1. 检查节点在面板中状态是否正常显示为在线
> 2. 手动停止面板后观察 Agent 日志，确认重连间隔呈指数增长
> 3. 恢复面板后确认 Agent 能自动恢复连接并恢复指标上报
> 4. 通过面板下发命令（如添加/删除 service），确认命令执行成功

---

## 任务清单

- [x] 优化 1：Agent 增加独立 WebSocket ping 发送
- [x] 优化 2：Agent 指数退避重连
- [x] 优化 3：Panel 缓存 AES 加密器
- [x] 优化 4：Agent 减少 receiveMessages 锁竞争
- [x] 优化 5：Agent 命令处理统一异步化
- [x] 运行现有测试验证不回归
