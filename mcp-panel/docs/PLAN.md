# FLVX 面板全量控制 MCP 服务方案（第一版）

## 1. 目标

构建一个独立的 `mcp-panel/` 服务层，不改造现有 `go-backend` API 语义，在 MCP 协议下提供“面板可做的全部能力”，并在默认策略上做到可审计、可回滚、可分权。

## 2. 现状约束（来自代码现状）

- 后端主路由在 `go-backend/internal/http/handler/handler.go`，API 前缀以 `/api/v1/*` 为主。
- 认证：面板端 JWT 走 `Authorization` 头，且为原始 token（非 `Bearer` 前缀）。
- 返回包络：`{code, msg, data, ts}`，`code=0` 表示成功。
- 大部分接口是 `POST`（包括 list 类接口），少数 `GET`（如公告、订阅）。
- 权限：中间件按路径做管理员/普通用户分流（group/node/tunnel/speed-limit/backup 等默认管理员）。
- 特殊通道：`/system-info` WebSocket、`/flow/*` 节点流量上报接口、federation 对接接口。

## 3. 全量能力覆盖范围（MCP Tool Domain）

按“面板功能域”进行工具命名空间拆分，确保覆盖当前可控能力：

1. `auth.*`
   - 登录、token 校验、用户上下文解析
2. `user.*`
   - 用户 CRUD、重置流量、改密、用户可见资源
3. `node.*`
   - 节点 CRUD、安装命令、检查状态、升级/批量升级/回滚、版本列表
4. `tunnel.*`
   - 隧道 CRUD、诊断、批量删除、批量重部署、顺序调整
5. `forward.*`
   - 转发 CRUD、强删、启停、诊断、批量删/启停/重部署/切换隧道
6. `speed_limit.*`
   - 限速策略 CRUD 与关联查询
7. `group.*`
   - 用户组/隧道组/权限组 CRUD、分配、撤销
8. `config.*`
   - 系统配置读取、批量更新、单项更新
9. `backup.*`
   - 导出、导入、恢复（含导入前自动备份信息）
10. `announcement.*`
   - 公告读取与更新
11. `federation.*`
   - share 管理、remote usage、connect/tunnel/runtime 指令、远端节点导入
12. `openapi.*`
   - 订阅信息导出能力
13. `ops.*`
   - 诊断聚合、健康检查、能力发现、路由映射输出（便于 agent 自省）

## 4. MCP 服务架构

```text
LLM/Client
   -> MCP Transport (stdio / streamable-http)
   -> mcp-panel (tool router + policy engine + audit)
   -> go-backend HTTP API
   -> go-gost/ws side effects (由后端现有逻辑触发)
```

### 4.1 目录规划（已创建）

```text
mcp-panel/
  README.md
  docs/
    PLAN.md
  cmd/
    flvx-mcp/
  internal/
  pkg/
```

### 4.2 关键模块设计

- `internal/transport/`
  - stdio 与 streamable-http 启动器
- `internal/auth/`
  - MCP 会话鉴权（可选 OAuth/JWT）
  - 上游面板 token 透传/托管
- `internal/panelclient/`
  - 对 `go-backend` 的 typed client
  - 统一处理 `{code,msg,data,ts}` 解包与错误映射
- `internal/tools/`
  - 按 domain 拆分工具注册：`tools/node.go`、`tools/tunnel.go`...
- `internal/policy/`
  - 危险操作二次确认、幂等键、并发限制、速率限制
- `internal/audit/`
  - 结构化审计日志（tool、参数摘要、操作者、结果、耗时）
- `internal/tasks/`
  - 长任务状态（批量操作、导入导出、批量升级）与进度查询

## 5. 安全与控制策略（默认强约束）

1. 最小权限
   - 只暴露显式注册的工具；按角色过滤工具列表。
2. 危险操作双阶段
   - `preview_*`（预检） + `apply_*`（执行）
   - 对 `delete/force-delete/import/rollback/batch-*` 强制 `confirm_token`。
3. 幂等与重放保护
   - 所有 mutation 工具接受 `idempotency_key`，服务端做时间窗去重。
4. 审计
   - 每次工具调用写审计日志，支持按用户/时间/工具检索。
5. 限流与熔断
   - 针对批量接口和 federation 接口设置更严流控，避免级联故障。

## 6. 工具入参/出参统一约定

- 入参统一结构：
  - `auth_context`（可选）：token 引用或会话凭据
  - `request`: 业务参数
  - `idempotency_key`: mutation 必填
- 出参统一结构：
  - `ok: boolean`
  - `panel_code / panel_msg`
  - `data`
  - `trace_id`

> 说明：内部仍保留对面板原始 `{code,msg,data,ts}` 的完整映射，避免语义丢失。

## 7. 分阶段落地计划

### Phase 0（本轮）
- 建立独立目录与方案文档
- 梳理路由、认证、权限、响应约定

### Phase 1（骨架）
- 初始化 Go module、配置加载、健康检查
- 接入 transport（先 stdio，后 streamable-http）
- 实现 `panelclient` 基础能力

### Phase 2（核心域）
- `auth/user/node/tunnel/forward` 工具集
- 危险操作二次确认
- 审计日志

### Phase 3（全量域）
- `speed_limit/group/config/backup/announcement/federation/openapi/ops`
- 长任务执行器与进度工具

### Phase 4（硬化）
- 权限矩阵、限流、熔断、压测、回归验证
- 发布与运维文档

## 8. 验收标准

- 功能覆盖：`/api/v1` 可控能力在 MCP 层 1:1 或等价覆盖。
- 安全覆盖：所有高风险工具需二次确认+审计。
- 可观测性：每个工具调用具备 trace 与审计记录。
- 一致性：错误语义与原面板 API 保持可追溯映射。

## 9. 任务追踪（每完成一项就打标）

- [x] 任务1：梳理现有面板能力边界（路由/鉴权/响应/模块）
- [x] 任务2：并行外部调研（MCP 规范 + Go 实现生态）
- [x] 任务3：创建独立 `mcp-panel/` 目录与基础结构
- [x] 任务4：输出完整 Markdown 方案（本文件，后续可继续迭代）
- [x] 任务5：进入实现阶段（Phase 1 骨架已启动并可构建）
- [x] 任务6：推进 Phase 2 第一批核心工具（auth/tunnel/forward）
- [x] 任务7：推进 Phase 2 第二批危险操作与幂等保护（delete/pause/resume）

## 11. Phase 1 执行记录（本轮）

- [x] 初始化 `mcp-panel/go.mod`，引入官方 `github.com/modelcontextprotocol/go-sdk`
- [x] 新增配置模块：`internal/config/config.go`
- [x] 新增面板客户端：`internal/panelclient/client.go`
  - 已实现统一包络解析：`{code,msg,data,ts}`
  - 已实现：`ListNodes`、`ListUsers`
- [x] 新增工具注册：`internal/tools/tools.go`
  - 已实现只读样板工具：`node.list`、`user.list`
- [x] 新增入口：`cmd/flvx-mcp/main.go`
  - 已支持 `stdio` 与 `http`（streamable-http）两种模式
  - 已暴露健康检查端点（默认 `/healthz`）
- [x] 已完成构建验证：`go build ./...`

### 11.1 当前可用环境变量

- `MCP_SERVER_NAME`（默认：`flvx-panel-mcp`）
- `MCP_SERVER_VERSION`（默认：`0.1.0`）
- `PANEL_BASE_URL`（默认：`http://127.0.0.1:6365`）
- `MCP_TRANSPORT`（可选：`stdio`/`http`，默认：`stdio`）
- `MCP_HTTP_ADDR`（默认：`:8088`）
- `MCP_HEALTH_PATH`（默认：`/healthz`）
- `MCP_CONFIRM_TOKEN`（危险操作确认令牌，未配置时危险操作默认禁用）
- `MCP_AUDIT_ENABLED`（是否开启审计日志，默认：`true`）
- `MCP_IDEMPOTENCY_TTL_SECONDS`（幂等结果缓存秒数，默认：`3600`）

## 12. Phase 2 执行记录

### 12.1 第一批

- [x] 扩展 `panelclient`：
  - `Login` -> `/api/v1/user/login`
  - `UserPackage` -> `/api/v1/user/package`
  - `ListTunnels` -> `/api/v1/tunnel/list`
  - `ListForwards` -> `/api/v1/forward/list`
  - `DeleteForward` -> `/api/v1/forward/delete`
- [x] 新增 `internal/policy/confirm.go`
  - 危险操作 `confirm_token` 校验（fail-closed）
- [x] 新增 `internal/audit/logger.go`
  - 工具调用结构化日志（tool、outcome、duration）
- [x] 扩展 MCP tools：
  - `auth.login`
  - `auth.user_package`
  - `tunnel.list`
  - `forward.list`
  - `forward.delete`（危险操作）
- [x] 主程序装配 `policy + audit`，完成工具注册

### 12.2 第二批（危险操作 + 幂等）

- [x] 扩展 `panelclient`：
  - `DeleteUser` -> `/api/v1/user/delete`
  - `DeleteNode` -> `/api/v1/node/delete`
  - `DeleteTunnel` -> `/api/v1/tunnel/delete`
  - `PauseForward` -> `/api/v1/forward/pause`
  - `ResumeForward` -> `/api/v1/forward/resume`
- [x] 新增 `internal/policy/idempotency.go`
  - `IdempotencyStore`（TTL、冲突检测、重放返回）
- [x] 扩展危险操作工具并强制：
  - `confirm_token`
  - `idempotency_key`
- [x] 新增危险操作 MCP tools：
  - `user.delete`
  - `node.delete`
  - `tunnel.delete`
  - `forward.pause`
  - `forward.resume`
  - `forward.delete`（升级为强制幂等）
- [x] 主程序装配 `IdempotencyStore`，并接入配置 `MCP_IDEMPOTENCY_TTL_SECONDS`

## 13. 下一步建议

优先推进 Phase 2 第三批：补齐 `user.create/update`、`node.create/update`、`tunnel.create/update`，再补 `forward.create/update/batch-*`，并把 `idempotency_key` 扩展到所有 mutation 工具。
