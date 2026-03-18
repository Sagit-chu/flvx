# 043 - Entry Transition Contract Semantics Alignment

## Issue
- SQLite 阻塞修复完成后，`go test ./tests/contract/...` 暴露出 3 个入口变更相关合同用例失败。
- 失败原因分成两类：
  - Issue 313 用例的数据构造没有真正制造“新增入口节点已被其他转发占用”的冲突。
  - Issue 281 回归用例在后续引入“入口端口必须落在节点端口范围内”后，仍沿用旧的非重叠端口范围数据，和当前产品语义不一致。

## Goal
- 对齐这 3 个合同测试与当前后端语义。
- 保持 SQLite 自锁修复不回退。
- 让入口节点切换/新增相关合同测试重新稳定通过。

## Checklist
- [x] 复核 3 个失败用例的测试数据与当前后端校验语义差异。
- [x] 调整 Issue 313 用例，确保新增入口节点确实命中“其他转发已占用同节点同端口”。
- [x] 调整 Issue 281 两个回归用例，使入口切换场景使用与保留端口兼容的节点端口范围。
- [x] 运行相关合同测试与 handler 测试并记录结果。

## Notes
- `validateTunnelEntryPortConflictsForNewEntries` 的占用判定复用 `HasOtherForwardOnNodePort`，语义是“同节点 + 同端口 + 其他转发”，不是全局无节点维度的端口唯一性。
- 入口切换测试当前更关注 `forward_port` 重建和旧节点运行时清理，因此测试数据应避免被端口范围校验提前拦截。

## 实际调整
- `go-backend/tests/contract/issue313_entry_port_conflict_contract_test.go`
  - 将隧道 A 的入口节点改为复用即将添加到隧道 B 的 `entryB2`，确保新增入口时真正命中“同节点同端口已被其他转发占用”。
  - 修正错误消息断言，直接检查 `out.Msg`，避免 JSON 解码后再读 `res.Body` 导致误判。
- `go-backend/tests/contract/limiter_sync_failure_contract_test.go`
  - 将 issue 281 的新入口节点端口范围调整为包含原有保留端口，保持测试关注点在运行时清理/同步，而不是被后续引入的端口范围校验拦截。

## 测试结果

### 定向回归
```bash
cd go-backend && go test ./tests/contract/... -run 'TestIssue313_EntryPortCrossTunnelConflictContract|TestTunnelUpdateChangesEntryNodeButLeavesOldForwardRuntimeContract|TestTunnelUpdateEntryTransitionsCleanupForwardRuntimeContract' -v
```
- 结果：3/3 通过。

### Handler
```bash
cd go-backend && go test ./internal/http/handler/...
```
- 结果：通过。

### 全量合同测试
```bash
cd go-backend && go test ./tests/contract/...
```
- 结果：通过。
