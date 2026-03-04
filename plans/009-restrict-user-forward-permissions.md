# 009: 普通用户转发权限限制

## 背景

当前系统允许普通用户在创建和编辑转发时设置：
1. **限速规则** (`speedId`) - 应仅限管理员设置
2. **自定义入口端口** (`inPort`) - 应仅限管理员设置

普通用户应只能使用系统自动分配的端口和默认不限速设置。

## 实施范围

| 操作 | 普通用户 | 管理员 |
|------|----------|--------|
| 创建转发 - 设置限速 | 禁止 | 允许 |
| 创建转发 - 自定义端口 | 禁止 | 允许 |
| 编辑转发 - 修改限速 | 禁止 | 允许 |
| 编辑转发 - 修改端口 | 禁止 | 允许 |

## 修改位置

### 后端 (Go)

**文件**: `go-backend/internal/http/handler/mutations.go`

#### 1. `forwardCreate` handler (行 1147-1157)

在处理 speedId 和 inPort 之前添加权限检查：

```go
if roleID != 0 {
    if _, ok := req["speedId"]; ok {
        response.WriteJSON(w, response.Err(-1, "普通用户无法设置限速规则"))
        return
    }
    if _, ok := req["inPort"]; ok {
        response.WriteJSON(w, response.Err(-1, "普通用户无法设置自定义端口"))
        return
    }
}
```

#### 2. `forwardUpdate` handler (行 1264-1274)

在处理 speedId 和 inPort 之前添加权限检查：

```go
if actorRole != 0 {
    if _, ok := req["speedId"]; ok {
        response.WriteJSON(w, response.Err(-1, "普通用户无法修改限速规则"))
        return
    }
    if _, ok := req["inPort"]; ok {
        response.WriteJSON(w, response.Err(-1, "普通用户无法修改自定义端口"))
        return
    }
}
```

### 前端 (React/TypeScript)

**文件**: `vite-frontend/src/pages/forward.tsx`

已有变量 `isAdmin` (行 610: `const isAdmin = tokenRoleId === 0;`)

#### 1. 隐藏限速规则选择器 (行 4252-4282)

用条件渲染包裹：

```tsx
{isAdmin && (
  <Select
    label="限速规则"
    // ... 现有属性
  >
    {/* ... */}
  </Select>
)}
```

#### 2. 隐藏入口端口输入框 (行 4311-4328)

用条件渲染包裹：

```tsx
{isAdmin && (
  <Input
    description="指定入口端口，留空则从节点可用端口中自动分配"
    // ... 现有属性
  />
)}
```

## 任务清单

- [x] 后端: `forwardCreate` 添加权限检查
- [x] 后端: `forwardUpdate` 添加权限检查
- [x] 前端: 隐藏限速规则选择器 (仅管理员可见)
- [x] 前端: 隐藏入口端口输入框 (仅管理员可见)
- [x] 后端: 添加契约测试验证权限限制
- [x] 运行测试验证

## 测试验证

1. ✅ 契约测试已添加 `TestNonAdminCannotSetSpeedIdOrPort`
2. ✅ 所有测试用例通过：
   - 普通用户创建转发时设置 speedId 被拒绝
   - 普通用户创建转发时设置 inPort 被拒绝
   - 普通用户创建转发时不设置 speedId/inPort 成功
   - 普通用户更新转发时设置 speedId 被拒绝
   - 普通用户更新转发时设置 inPort 被拒绝
   - 普通用户更新转发时不设置 speedId/inPort 成功
