# Session Context

Session ID: ses_349a99cf4ffeQMqVmv7PqyI3y6
Commit Message: 普通用户添加转发的时候应该不能有权限设置限速和设置自定义端口，请检查并规划设计

## Prompts

### Prompt 1

普通用户添加转发的时候应该不能有权限设置限速和设置自定义端口，请检查并规划设计

## Summary

## 分析结果

### 当前状态
- **角色系统**: `roleID == 0` 为管理员，`roleID != 0` 为普通用户
- **问题**: 普通用户在创建/编辑转发时可设置 `speedId`(限速) 和 `inPort`(自定义端口)

### 实施方案

**后端修改** (`go-backend/internal/http/handler/mutations.go`):
- `forwardCreate`: 检查 `roleID != 0` 时拒绝 `speedId` 和 `inPort`
- `forwardUpdate`: 检查 `actorRole != 0` 时拒绝 `speedId` 和 `inPort`

**前端修改** (`vite-frontend/src/pages/forward.tsx`):
- 限速规则选择器和入口端口输入框使用 `{isAdmin && (...)}` 条件渲染

**测试**:
- 添加契约测试验证权限限制

计划文档: `plans/009-restrict-user-forward-permissions.md`
