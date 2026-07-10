# 规则流量清零设计

## 背景

Issue #523 希望“规则”页面中每条隧道规则显示的流量使用量支持手动清零。

当前规则流量保存在 `forward.in_flow` 和 `forward.out_flow`。流量上报时，同一份增量还会累计到用户总流量、用户隧道流量和相关配额统计中。因此，本功能必须将“规则展示计数器清零”与“用户或隧道配额重置”严格区分。

## 目标

为单条规则提供手动流量清零能力：

- 将所选规则的上传流量和下载流量清零。
- 管理员可以清零任意规则。
- 普通用户只能清零自己的规则。
- 清零后，新产生的流量继续从零正常累计。

## 非目标

本功能不会：

- 修改用户总流量 `user.in_flow` 或 `user.out_flow`。
- 修改用户隧道流量 `user_tunnel.in_flow` 或 `user_tunnel.out_flow`。
- 修改每日或每月配额用量。
- 修改历史流量统计。
- 重置 nftables 节点计数器或其增量计算基线。
- 重启、暂停、恢复或重新部署规则服务。
- 增加批量流量清零功能。

## 后端设计

### API

新增接口：

```text
POST /api/v1/forward/reset-flow
```

请求体：

```json
{
  "id": 123
}
```

成功响应沿用统一 envelope：

```json
{
  "code": 0,
  "msg": "success",
  "data": null,
  "ts": 0
}
```

具体 `msg`、`data` 和 `ts` 值继续由现有 response helper 生成。

### 参数与权限校验

Handler 执行以下步骤：

1. 只接受 `POST` 请求。
2. 从 JSON 请求体读取正整数规则 ID。
3. 调用现有 `resolveForwardAccess`：
   - 管理员角色可以访问任意存在的规则。
   - 普通用户仅能访问 `forward.user_id` 等于当前用户 ID 的规则。
   - 对普通用户访问他人规则的情况，沿用现有逻辑返回“转发不存在”，避免暴露规则存在性。
4. 调用 Repository 完成清零。
5. 返回统一成功响应。

### Repository

新增方法：

```go
func (r *Repository) ResetForwardFlow(forwardID int64, now int64) error
```

该方法只更新指定 `forward` 记录：

```text
in_flow = 0
out_flow = 0
updated_time = now
```

Repository 不直接操作 Handler 的身份信息，也不更新任何其他表。

### 并发与后续流量

清零使用单条 SQL `UPDATE`。agent 流量上报和 nftables 流量采集仍使用原有增量累加逻辑。清零不会重置采集基线，因此下一次采集只会把清零之后新计算出的增量加回规则计数，不会把清零前的累计值整体恢复。

若清零 SQL 与流量增量 SQL 同时执行，数据库按实际语句执行顺序决定最终值；每条更新本身保持原子性。本功能不引入暂停采集或跨节点同步流程。

## 前端设计

### API 封装

在 `vite-frontend/src/api/index.ts` 新增：

```ts
export const resetForwardFlow = (id: number) =>
  Network.post("/forward/reset-flow", { id });
```

### 入口

在规则页面所有单条规则操作入口中增加“流量清零”操作：

- 分组表格视图。
- 精简表格视图。
- 卡片视图。

按钮使用独立的清零/刷新语义图标和提示文本，不复用删除按钮样式。

当规则的 `inFlow + outFlow` 等于零时，按钮禁用，避免重复请求。

### 确认交互

点击按钮后打开确认弹窗，显示规则名称，并明确说明：

- 仅清零当前规则显示的上传和下载流量。
- 不影响用户总流量、用户隧道配额和历史统计。
- 操作不可撤销。

确认期间显示 loading 状态并阻止重复提交。

### 成功与失败

- 成功：关闭弹窗，显示成功 toast，并刷新规则列表。
- 失败：保留弹窗，显示后端错误信息或通用失败 toast。
- 刷新后，该规则上传和下载均显示为零；后续流量继续正常累计。

## 错误处理

- 非 POST 请求：返回现有通用请求失败响应。
- 请求体无法解析、ID 缺失或 ID 非正数：返回“请求参数错误”。
- 规则不存在或普通用户访问他人规则：返回“转发不存在”。
- Repository 更新失败：返回包含 Repository 错误信息的统一错误响应。
- 前端网络错误：显示“流量清零失败”。

## 测试策略

### Repository 测试

验证：

- 指定规则的 `in_flow`、`out_flow` 被清零。
- 指定规则的 `updated_time` 被更新。
- 其他规则的流量不变。
- 用户总流量不变。
- 用户隧道流量不变。
- Repository 未初始化时返回错误。

### Handler 测试

验证：

- 管理员能够清零任意存在的规则。
- 普通用户能够清零自己的规则。
- 普通用户不能清零他人的规则。
- 不存在的规则返回错误。
- 无效 ID 返回参数错误。
- 非 POST 请求返回请求失败。
- 成功请求不修改用户和用户隧道流量。

### 前端验证

项目没有配置前端测试框架，因此不新增前端单元测试。使用以下命令验证：

```bash
(cd vite-frontend && pnpm run build)
(cd vite-frontend && pnpm run lint)
```

后端使用：

```bash
(cd go-backend && go test ./...)
```

## 文件范围

预计修改：

- `go-backend/internal/http/handler/handler.go`
- `go-backend/internal/http/handler/mutations.go`
- `go-backend/internal/http/handler/*_test.go`
- `go-backend/internal/store/repo/repository_mutations.go`
- `go-backend/internal/store/repo/*_test.go`
- `vite-frontend/src/api/index.ts`
- `vite-frontend/src/pages/forward.tsx`

不需要数据库迁移或新增依赖。
