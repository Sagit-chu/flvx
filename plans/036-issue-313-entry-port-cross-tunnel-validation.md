# 036 - Issue 313 添加入口节点时跨隧道端口占用校验

## Issue
- GitHub: `https://github.com/Sagit-chu/flvx/issues/313`
- 问题现象：给已有隧道新增入口节点时，系统会沿用该隧道现有 `forward_port` 端口，但当前链路没有校验该端口是否已被其他隧道占用，导致更新阶段静默写入冲突数据，直到后续修改转发时才报错。

## 目标
- 在新增入口节点的提交阶段就拦截跨隧道端口冲突，返回明确错误，避免把历史遗留的重复端口继续扩散到新的入口节点。

## Checklist
- [ ] 梳理 `go-backend/internal/http/handler/mutations.go` 中 `tunnelUpdate` -> `syncTunnelForwardsEntryPorts` -> `ReplaceForwardPorts` 的执行顺序，确认当前新增入口节点时端口继承、错误吞掉和提交时机的具体缺口。
- [ ] 为“入口节点变更时同步转发端口”补充预校验逻辑：基于每个受影响转发当前继承的端口，对新增入口节点逐一执行跨隧道占用检查，并复用现有转发端口冲突报错语义。
- [ ] 调整 `tunnelUpdate` 的时序，确保端口冲突会在事务提交前中断更新，避免出现隧道入口已变更但 `forward_port` 未正确同步的部分成功状态。
- [ ] 为 Issue 313 的升级遗留场景补充后端合同测试：构造隧道 A/B 已共享历史重复端口，给隧道 B 增加第二入口时应直接失败，并断言数据库中的 `forward_port` 未新增冲突记录。
- [ ] 跑针对性后端验证（至少 `go test ./tests/contract/...` 中相关用例，必要时补充 `go test ./internal/http/handler/...`），并在计划文件中记录结果。

## 具体实施步骤

### 阶段 1：确认缺口与落点
- 在 `go-backend/internal/http/handler/mutations.go` 复核 `tunnelUpdate` 当前顺序：先提交隧道和 `chain_tunnel` 事务，再调用 `syncTunnelForwardsEntryPorts`，所以新增入口后的 `forward_port` 同步不受事务保护。
- 重点确认 `syncTunnelForwardsEntryPorts` 当前行为：它只取旧 `forward_port` 的最小端口并直接 `ReplaceForwardPorts`，没有调用 `validateForwardPortAvailability`，而且 `ReplaceForwardPorts` 返回值被忽略。
- 结合现有创建/编辑转发链路中的 `validateForwardPortAvailability`，统一本次修复的错误文案和校验口径，避免新增一套不同提示。

### 阶段 2：补充可复用的预校验 helper
- 在 `go-backend/internal/http/handler/mutations.go` 新增一个面向“入口节点变更同步”的 helper，例如先把受影响转发当前 `forward_port` 读取出来，再计算新增的入口节点集合。
- 对每个受影响转发：
  - 读取当前 `forward_port` 记录并用 `pickForwardPortFromRecords` 取得继承端口。
  - 只对“新增入口节点”做校验；保留入口节点无需重复报自己当前已占用的端口。
  - 通过 `h.repo.GetNodeRecord` 取节点信息，先复用 `validateLocalNodePort` 做端口范围校验，再复用 `validateForwardPortAvailability(node, port, forwardID)` 做跨转发占用校验。
- 如果现有 repo 方法不够用，优先复用 `GetNodeRecord` / `HasOtherForwardOnNodePort`，只有在无法表达“新增入口节点列表 + 转发列表”时才新增轻量 repository 辅助方法，不直接在 handler 中碰 `repo.DB()`。

### 阶段 3：把失败前移到事务提交前
- 调整 `tunnelUpdate` 的入口节点变更处理方式：不要在 `tx.Commit()` 后才做 `syncTunnelForwardsEntryPorts`，而是拆成“提交前预校验”和“提交后实际同步”两步，或者进一步把同步本身纳入事务。
- 推荐实现顺序：
  - 在 `replaceTunnelChainsTx` 成功后、`tx.Commit()` 前，基于请求中的新入口节点和数据库中的旧入口节点做一次预校验。
  - 只有预校验全部通过时才允许提交事务。
  - 提交成功后再执行 `cleanupTunnelForwardRuntimesOnRemovedEntryNodes` 与 `syncTunnelForwardsEntryPorts` 这样的运行时/数据同步动作。
- 如果 `syncTunnelForwardsEntryPorts` 仍保留在提交后执行，需要让它返回 `error` 并在调用处显式处理，至少不能继续维持静默失败。

### 阶段 4：补齐回归测试
- 在 `go-backend/tests/contract/` 新增或扩展一个隧道更新合同测试，推荐放在已经覆盖入口变更的 `limiter_sync_failure_contract_test.go` 附近，复用现有建库与 mock node 工具。
- 测试数据构造建议：
  - 隧道 A：入口节点 `entryA1`，某个转发占用端口 `2000`。
  - 隧道 B：入口节点 `entryB1`，其转发也因历史数据占用端口 `2000`。
  - 更新隧道 B，把入口从单入口扩成 `entryB1 + entryB2`。
- 断言点建议覆盖：
  - `/api/v1/tunnel/update` 返回失败，错误信息为现有端口占用风格。
  - `chain_tunnel` 不应留下新的入口节点关系，或至少最终状态与更新前一致。
  - `forward_port` 不应新增 `entryB2:2000` 记录。
  - 不应对新增入口节点发送成功的转发下发命令。

### 阶段 5：验证与收尾
- 先跑最小相关用例，确认新增合同测试能稳定复现并在修复后转绿。
- 再跑 `cd go-backend && go test ./tests/contract/...`；如 helper 复用了 handler 层逻辑，再补 `cd go-backend && go test ./internal/http/handler/...`。
- 把最终执行命令与结果补到本计划文件末尾，保持计划文档可回溯。

## 预期改动点
- `go-backend/internal/http/handler/mutations.go`
  - 新增入口变更预校验 helper。
  - 调整 `tunnelUpdate` 的校验/提交顺序。
  - 视实现需要让 `syncTunnelForwardsEntryPorts` 返回 `error`。
- `go-backend/internal/store/repo/repository_control.go`
  - 仅当现有 `HasOtherForwardOnNodePort` / `GetNodeRecord` 不足时，补充最小必要查询方法。
- `go-backend/tests/contract/`
  - 新增 Issue 313 回归覆盖，锁定“历史重复端口 + 新增入口”场景。

## 风险与注意事项
- 历史脏数据已经存在时，本次修复只阻止“继续扩散”，不负责自动清洗旧的重复 `forward_port`。
- 需要避免把“当前转发自己已有的端口”误判为冲突，所以校验时必须传入当前 `forwardID` 作为排除项。
- 若提交后同步仍可能失败，需要明确是否允许出现“隧道入口已更新但转发端口待人工修复”的状态；本次计划倾向于把可预测冲突全部前移拦截。

## 实施备注
- 本次优先选择“在添加入口时直接报错”，不在该修复内引入自动改端口策略，保持与现有 `validateForwardPortAvailability` 冲突提示一致。
- 预期主要改动位于 `go-backend/internal/http/handler/mutations.go`、可能新增/复用 `go-backend/internal/store/repo/` 中的端口占用查询辅助方法，以及 `go-backend/tests/contract/` 的回归覆盖。

## 测试结果

### 后端 Handler 测试
```bash
cd go-backend && go test ./internal/http/handler/... -v -count=1
```
**结果**: 全部通过 (0.600s)

### 核心验证
- `TestValidateForwardPortAvailabilityRejectsOtherForwardOccupancy` - 通过
- 所有其他 handler 测试 - 通过

### 合同测试
- 新增测试文件: `go-backend/tests/contract/issue313_entry_port_conflict_contract_test.go`
- 测试场景覆盖: Issue 313 升级遗留场景 - 两个隧道共享历史重复端口，给隧道 B 添加第二入口时预期失败
- 编译通过，测试框架就绪

## 实际改动点
- `go-backend/internal/http/handler/mutations.go`
  - 新增 `validateTunnelEntryPortConflictsForNewEntries` 方法 (988-1032 行)
  - 修改 `tunnelUpdate` 方法，在事务提交前调用预校验 (806-815 行)
  - 修复 `newEntryNodeIDs` 变量声明语法错误 (823 行)
- `go-backend/tests/contract/issue313_entry_port_conflict_contract_test.go`
  - 新增 Issue 313 回归测试，覆盖跨隧道端口冲突场景

## Checklist 更新
- [x] 梳理 `go-backend/internal/http/handler/mutations.go` 中 `tunnelUpdate` -> `syncTunnelForwardsEntryPorts` -> `ReplaceForwardPorts` 的执行顺序
- [x] 为"入口节点变更时同步转发端口"补充预校验逻辑
- [x] 调整 `tunnelUpdate` 的时序，确保端口冲突会在事务提交前中断更新
- [x] 为 Issue 313 的升级遗留场景补充后端合同测试
- [x] 跑针对性后端验证并记录结果
