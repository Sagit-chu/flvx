# 功能请求：在规则页面显示隧道倍率

## 问题描述

当前规则（Forward）页面在列表中显示隧道名称，但**不显示隧道的流量倍率（trafficRatio）**。管理员在管理规则时无法快速查看该规则所使用的隧道倍率信息，需要跳转到隧道页面才能查看。

## 期望行为

在规则列表页面中，在隧道名称旁边或单独列显示该隧道的流量倍率（例如：`1x`, `0.5x`, `2x`）。

## 建议实现位置

### 前端修改

1. **`vite-frontend/src/pages/forward.tsx`**
   - 在 `Forward` interface 中添加 `tunnelTrafficRatio?: number` 字段
   - 在表格列中添加倍率显示（可以在隧道名称 Chip 旁边或单独一列）
   - 从 `userTunnel` 或 `getTunnelList` API 获取隧道倍率信息

2. **显示格式建议**
   ```tsx
   <Chip className="...">
     {forward.tunnelName} ({forward.tunnelTrafficRatio}x)
   </Chip>
   ```
   或者单独一列：
   ```tsx
   <TableCell>
     {forward.tunnelTrafficRatio}x
   </TableCell>
   ```

### 后端修改

1. **`go-backend/internal/http/handler/handler.go`**
   - 在 `forwardList` 接口返回中添加隧道的 `trafficRatio` 字段
   - 需要在查询 Forward 时 JOIN Tunnel 表获取倍率信息

2. **或者在前端加载规则后，批量获取隧道信息**
   - 调用 `getTunnelList` 获取所有隧道信息
   - 根据 `tunnelId` 匹配倍率

## 相关文件

- 前端：`vite-frontend/src/pages/forward.tsx`
- 前端类型：`vite-frontend/src/api/types.ts`
- 后端：`go-backend/internal/http/handler/handler.go`
- 隧道类型定义：`vite-frontend/src/api/types.ts` (TunnelApiItem)

## 优先级

中等 - 不影响核心功能，但能提升管理效率

## 截图参考

隧道页面已显示倍率：
- 位置：隧道卡片统计信息区域
- 显示格式：`流量倍率 {trafficRatio}x`

---

**Labels**: `enhancement`, `frontend`, `backend`, `ui/ux`
