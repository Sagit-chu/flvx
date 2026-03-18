# 044 - 隧道删除时的规则依赖处理设计

## Goal
- 删除隧道前识别关联规则，避免默认级联误删。
- 在隧道页提供两种处理方式：迁移规则到其他隧道，或连同规则一起删除。
- 第一阶段只覆盖单条隧道删除，先把核心交互和执行链路做稳。

## Checklist
- [x] 复核当前 `tunnel.tsx` 删除交互与 `DeleteTunnelCascade` 行为。
- [x] 梳理可复用的规则换隧道能力（`forward/batch-change-tunnel`）。
- [x] 产出前端交互、接口与执行链路设计。
- [x] 明确第一阶段范围、异常处理与回滚要求。

## Implementation Checklist
- [x] 新增后端删除预检与带规则处理的删除接口。
- [x] 在后端补齐规则迁移/失败明细/回滚链路。
- [x] 接入前端隧道删除预检、替换/删除规则弹窗与结果展示。
- [x] 运行定向测试并记录结果。

## Current State
- `vite-frontend/src/pages/tunnel.tsx` 的单条删除当前直接调用 `/tunnel/delete`，没有前置依赖检查。
- `go-backend/internal/store/repo/repository_mutations.go` 里的 `DeleteTunnelCascade` 会直接删除该隧道下的 `forward`、`forward_port`、`user_tunnel`、`chain_tunnel` 等关联数据。
- `vite-frontend/src/pages/forward.tsx` 已经有“批量换隧道”交互和 `/forward/batch-change-tunnel`，但如果前端直接串联“先换规则、再删隧道”，中间任一步失败都会留下半完成状态，不适合作为删除流程的最终方案。

## Proposed UX
- 点击隧道删除按钮后，前端先请求“删除预检”接口，不直接进入最终确认。
- 若关联规则数为 `0`，继续沿用当前简单确认弹框。
- 若关联规则数大于 `0`，改为展示“处理关联规则”弹框：
  - 顶部 `Alert` 提示：`隧道 "A" 正被 12 条规则使用`。
  - 展示前 5 条规则摘要：规则名、所属用户、入口端口；剩余规则用“还有 N 条未展开”提示即可。
  - 提供 `RadioGroup` 两个动作：
    - `替换到其他隧道`（推荐，默认）
    - `删除这些规则`
  - 选择“替换到其他隧道”时显示 `Select`：
    - 数据源直接复用当前页已经加载的 `tunnels`。
    - 仅显示 `status === 1` 且 `id !== 当前隧道` 的选项。
    - 若没有可选隧道，则禁用该选项并自动回退到“删除这些规则”。
  - 确认按钮文案动态变化：
    - `替换规则并删除隧道`
    - `删除规则并删除隧道`
- 提交时展示 loading 状态；成功后关闭弹框、刷新隧道列表并 toast 成功。
- 如果后端返回规则级失败明细，前端复用现有 `BatchActionResultModal` 展示失败项，而不是只给一个通用 toast。

## API Design
### 1. 删除预检
`POST /api/v1/tunnel/delete-preview`

Request:
```json
{ "id": 12 }
```

Response:
```json
{
  "code": 0,
  "msg": "",
  "data": {
    "tunnelId": 12,
    "tunnelName": "HK-Entry",
    "forwardCount": 12,
    "sampleForwards": [
      {
        "id": 101,
        "name": "web-1",
        "userId": 9,
        "userName": "alice",
        "inPort": 443
      }
    ]
  }
}
```

- `sampleForwards` 只需要返回前 5 条，前端用 `forwardCount` 决定是否显示“更多”提示。
- 预检只描述现状，不负责最终授权或一致性保证；真正提交删除时，后端必须再校验一次。

### 2. 带规则处理的删除
`POST /api/v1/tunnel/delete-with-forwards`

Request:
```json
{
  "id": 12,
  "action": "replace",
  "targetTunnelId": 18
}
```

或：

```json
{
  "id": 12,
  "action": "delete_forwards"
}
```

Success response:
```json
{
  "code": 0,
  "msg": "",
  "data": {
    "forwardCount": 12,
    "migratedCount": 12,
    "deletedForwardCount": 0,
    "portAdjustedCount": 2
  }
}
```

Failure response:
```json
{
  "code": -2,
  "msg": "部分规则迁移失败",
  "data": {
    "successCount": 9,
    "failCount": 3,
    "failures": [
      {
        "id": 101,
        "name": "web-1",
        "reason": "目标隧道入口节点端口 443 已占用"
      }
    ]
  }
}
```

- 保持现有 `/tunnel/delete` 不动，兼容旧调用和当前批量删除逻辑。
- `vite-frontend/src/pages/tunnel.tsx` 的单条删除新流程全部走新接口。

## Backend Execution Strategy
- `delete-preview`
  - 校验 tunnel 是否存在。
  - 查询 `forward.tunnel_id = 当前隧道` 的数量和前 5 条摘要。
- `delete-with-forwards`
  - 再次查询依赖规则，避免 preview 与提交之间状态变化导致误判。
  - `action = delete_forwards`
    - 直接复用当前级联删除逻辑。
  - `action = replace`
    - 校验 `targetTunnelId`：存在、启用、且不等于当前隧道。
    - 先对全部关联规则做一次前置校验：目标隧道入口节点、端口范围、端口冲突、监听 IP 约束。
    - 全部校验通过后，再逐条迁移规则并同步运行时。
    - 任一规则迁移失败时，返回失败明细并中止删除；对已迁移规则做回滚，保证用户感知为“要么都成功，要么都不删”。
  - 规则处理完成后，再删除隧道本身及非 `forward` 关联数据。

## Frontend Implementation Notes
- `vite-frontend/src/pages/tunnel.tsx`
  - 新增删除预检 loading、依赖摘要、处理动作等 state。
  - 复用现有 `Modal`，根据 preview 结果切换普通确认视图和依赖处理视图。
  - 复用 `Select`、`RadioGroup`、`Alert`。
  - 成功后继续沿用当前 `setTunnels`、`setTunnelOrder`、`setSelectedIds` 清理逻辑。
- `vite-frontend/src/api/index.ts`
  - 新增 `previewTunnelDelete`。
  - 新增 `deleteTunnelWithForwards`。
- 删除失败且返回批量明细时，复用 `BatchActionResultModal`，避免具体失败规则原因被 toast 吞掉。

## Scope Decision
- 第一阶段只改“隧道页单条删除”。
- `批量删除隧道` 先保持现有“级联删除规则”行为和提示文案不变。
- 如果第一阶段确认体验和后端回滚链路都稳定，再补第二阶段：批量删除时按每条隧道分别预检和处理。

## Edge Cases
- 没有可替换隧道：只允许“删除这些规则”。
- 目标隧道在提交前被禁用或删除：后端返回明确错误，前端保留当前弹框和用户选择。
- preview 时没有规则、提交时新建了规则：以后端最终校验为准，返回需要重新处理的提示。
- 迁移时若原入口端口在目标隧道不可用，沿用现有“换隧道”语义：优先保留原端口，不可用时自动分配可用端口，并通过 `portAdjustedCount` 给前端一个非阻塞提示。
- 第一阶段不自动补发目标隧道的 `user_tunnel` 授权，先与现有“批量换隧道”语义保持一致；如果后续需要让用户也获得目标隧道的新增规则权限，再单独评估自动补授权。

## Implementation Notes
- 后端新增 `/api/v1/tunnel/delete-preview` 和 `/api/v1/tunnel/delete-with-forwards`，单条隧道删除改为先预检再执行。
- 后端新增 `/api/v1/tunnel/batch-delete-preview` 和 `/api/v1/tunnel/batch-delete-with-forwards`，批量删除支持统一预检并选择“批量替换规则”或“批量删除规则”。
- 规则替换删除在后端先做端口/节点占用预检，再执行逐条迁移；执行阶段若任一规则失败，会回滚已迁移规则，并把失败明细返回给前端弹窗展示。
- 前端 `tunnel.tsx` 删除弹窗已支持三种状态：预检中、普通删除确认、有依赖规则时的“替换/删除规则”决策视图。
- 批量删除弹窗现在会汇总每条选中隧道的依赖规则数量，并在批量替换失败时按“隧道级”返回失败原因，避免整批操作信息丢失。

## Verification
- `cd go-backend && go test ./internal/http/handler/...`
- `cd go-backend && go test ./tests/contract/... -run 'TestTunnelDeletePreviewIncludesDependentRulesContract|TestTunnelDeleteWithForwardsDeleteActionRemovesTunnelAndRulesContract|TestTunnelDeleteWithForwardsReplaceReturnsFailureDetailsContract'`
- `cd vite-frontend && npm run build`
