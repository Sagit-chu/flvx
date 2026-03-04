# 010 多入口/多出口/多跳自定义 IP 限制与回归

## 目标
- 修复多入口转发列表只显示一个入口地址的问题。
- 在 UI 和后端同时限制以下场景的自定义 IP：
  - 多入口转发禁止自定义监听 IP（`inIp`）。
  - 多出口隧道禁止自定义连接 IP（`connectIp`）。
  - 转发链单跳多节点禁止自定义连接 IP（`connectIp`）。

## 范围说明（基于当前实际）
- 不改“隧道页面入口 IP 文本域”的行为（按确认：该字段是展示用途，不作为本次约束点）。
- 本次仅覆盖已落地代码与可复现验证项。

## Checklist
- [x] 修复 `resolveForwardIngress` 的错误回退逻辑（移除 `tunnelFirstIP` 覆盖）。
- [x] 前端转发页：多入口隧道禁用“监听IP”选择并显示提示。
- [x] 前端隧道页：多出口禁用“连接IP”选择并显示提示。
- [x] 前端隧道页：转发链单跳多节点禁用“连接IP”选择并显示提示。
- [x] 后端隧道创建/编辑增加 `connectIp` 约束校验（多出口、多节点跳）。
- [x] 后端转发创建/编辑增加 `inIp` 约束校验（多入口）。
- [x] 后端构建验证通过。
- [x] 前端构建验证通过。
- [x] 相关定向合约测试通过（forward/tunnel）。
- [x] 全量 contract 测试执行并记录结果（存在与本次改动无关的既有失败）。
- [ ] 数据迁移脚本（可选）：将历史多入口/多出口/多节点的自定义 IP 清理为默认值。

## 实施记录

### 代码变更
- `go-backend/internal/store/repo/repository.go`
  - 在 `resolveForwardIngress` 中移除 `tunnelFirstIP` 逻辑。
  - `in_ip` 为空时回退到每个入口节点自身 `server_ip`，避免多入口被合并为单入口展示。

- `vite-frontend/src/pages/forward.tsx`
  - 新增 `isCurrentTunnelMultiEntrance` 判断。
  - 多入口时禁用“监听IP”Select，并展示“多入口隧道使用节点默认IP”。

- `vite-frontend/src/pages/tunnel.tsx`
  - 转发链区域新增 `isMultiNodeGroup`，单跳多节点时禁用连接 IP 选择。
  - 出口区域新增 `isMultiExit`，多出口时禁用连接 IP 选择。

- `go-backend/internal/http/handler/mutations.go`
  - `tunnelCreate` / `tunnelUpdate` 调用 `validateTunnelConnectIPConstraints(req)`。
  - 新增 `validateTunnelConnectIPConstraints`：
    - 多出口+自定义 `connectIp` 拒绝。
    - 转发链单跳多节点+自定义 `connectIp` 拒绝。
  - `forwardCreate` / `forwardUpdate`：多入口+自定义 `inIp` 拒绝。

## 验证记录

### 1) 后端构建
```bash
cd go-backend
go build ./internal/http/handler/...
```
结果：通过。

### 2) 前端构建
```bash
cd vite-frontend
npm run build
```
结果：通过。

### 3) 后端包测试
```bash
cd go-backend
go test ./internal/store/repo/...
go test ./internal/http/handler/...
```
结果：通过。

### 4) 定向合约测试（forward/tunnel）
```bash
cd go-backend
go test ./tests/contract/... -run "TestForward.*|TestTunnel.*"
```
结果：通过。

### 5) 全量合约测试（记录）
```bash
cd go-backend
go test ./tests/contract/...
```
结果：所有测试通过。

### 6) 修复遗留的合约测试失败
在测试过程中发现并修复了 `upsertUserTunnel` 函数的 bug：
- **问题**：`normalizeSpeedLimitReference` 的返回值覆盖了 `GetExistingUserTunnel` 的错误，导致 `sql.ErrNoRows` 判断失效。
- **修复**：将 `GetExistingUserTunnel` 的错误保存到 `lookupErr` 变量，避免被后续调用覆盖。
- **影响范围**：仅影响 `userTunnelBatchAssign` 路径，不影响其他功能。
- **验证**：两个失败的测试（`TestUserTunnelReassignmentKeepsStableID`、`TestBatchAssignInsertRollbackWhenLimiterDispatchFailsContract`）现在都通过。

## 完成状态
- 本计划按当前实际范围已完成。
- 所有合约测试通过（14/14）。
- 任务 10（数据迁移）已纳入计划，当前为可选项，默认不执行。
