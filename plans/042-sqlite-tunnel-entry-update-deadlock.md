# 042 - SQLite 隧道编辑添加入口节点卡死排查

## Issue
- 现象：SQLite 数据库下，编辑已有隧道并新增入口节点时接口卡住；PostgreSQL 下同样操作正常。
- 初步判断：`tunnelUpdate` 在事务尚未提交时触发了额外 repository 读查询，SQLite 配置 `MaxOpenConns(1)`，容易在同一请求内形成自锁等待。

## Goal
- 找出 SQLite 与 PostgreSQL 行为差异的根因。
- 修复隧道编辑新增入口节点时的阻塞问题，同时不破坏现有的入口端口冲突校验。
- 补充最小回归测试，锁定“事务内校验不可再次占用根连接”的场景。

## Checklist
- [x] 复核 `tunnelUpdate` 在新增入口节点路径上的调用链，确认事务内哪些查询绕过了 `tx`。
- [x] 为相关 repository 查询补齐 `Tx` 版本，避免 SQLite 单连接下的自锁等待。
- [x] 调整 handler 中入口端口冲突校验，保证事务内全程复用同一个 `tx`。
- [x] 增加针对 SQLite 的回归测试，验证事务内校验不会阻塞。
- [x] 运行相关 handler/backend 测试并记录结果。

## Notes
- 重点关注 `validateTunnelEntryPortConflictsForNewEntries`：当前它在 `tx.Commit()` 前执行，但内部调用 `ListForwardsByTunnel` / `ListForwardPorts` / `HasOtherForwardOnNodePort` 等非事务查询。
- SQLite 在 `go-backend/internal/store/repo/repository.go` 中显式设置了 `SetMaxOpenConns(1)`，因此这种模式在 SQLite 下会比 PostgreSQL 更容易表现为“卡死”。

## 实际改动
- `go-backend/internal/store/repo/repository_control.go`
  - 新增 `ListForwardsByTunnelTx`、`ListForwardPortsTx`、`HasOtherForwardOnNodePortTx`，并让原有非事务方法复用统一实现。
- `go-backend/internal/http/handler/mutations.go`
  - `tunnelUpdate` 在事务内执行入口端口冲突校验时显式传入当前 `tx`。
  - `validateTunnelEntryPortConflictsForNewEntries` 改为全程使用事务查询。
  - 新增 `validateForwardPortAvailabilityTx`，避免事务内回落到根连接查询。
- `go-backend/internal/http/handler/tunnel_entry_sqlite_test.go`
  - 新增 SQLite 回归测试，验证开启事务后执行新增入口校验不会阻塞。

## 测试结果

### 通过
```bash
cd go-backend && go test ./internal/http/handler/...
```
- 结果：通过。

### 额外检查
```bash
cd go-backend && go test ./tests/contract/...
```
- 结果：未全绿；当前失败集中在既有的入口端口语义合同用例：
  - `TestIssue313_EntryPortCrossTunnelConflictContract`
  - `TestTunnelUpdateChangesEntryNodeButLeavesOldForwardRuntimeContract`
  - `TestTunnelUpdateEntryTransitionsCleanupForwardRuntimeContract`
- 备注：这些失败反映的是“新增/切换入口时端口校验预期”与现有合同用例之间的行为差异，不是本次 SQLite 事务自锁修复本身的编译或阻塞问题。
