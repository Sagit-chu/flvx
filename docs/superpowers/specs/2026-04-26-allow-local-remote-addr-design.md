# 允许转发到本地地址开关设计

**日期**: 2026-04-26
**状态**: 待审核
**作者**: AI Assistant

## 概述

新增一个全局设置开关，控制规则目标地址是否允许指向本地/内网地址。默认关闭，保持当前安全策略不变；开启后，规则创建和编辑时允许将目标地址设置为 `127.0.0.1`、`10.x.x.x`、`172.16-31.x.x`、`192.168.x.x` 等本地或私网地址。

## 背景

当前后端在规则创建和编辑时会调用 `IsSafeRemoteAddr()`，统一禁止目标地址指向本地/内网地址，用来降低 SSRF / 开放代理风险。这一行为是全局硬编码的，无法按部署场景调整。

有些用户需要把规则转发到本机或内网服务，因此需要一个显式、全局的开关来放宽这条限制。

## 目标

1. 在设置页提供一个全局开关控制该行为。
2. 默认关闭，不改变现有安全默认值。
3. 开启后，规则创建和编辑允许本地/内网目标地址。
4. 不影响其他安全校验和其他业务流程。

## 影响范围

### 后端
- `go-backend/internal/http/handler/security_utils.go`
- `go-backend/internal/http/handler/mutations.go`
- `go-backend/internal/http/handler/handler.go`

### 前端
- `vite-frontend/src/pages/config.tsx`

### 测试
- `go-backend/tests/contract/forward_contract_test.go` 或新增独立 contract test

## 详细设计

### 1. 配置存储

使用现有 `vite_config` 表新增一个配置项：

| name | value | 说明 |
|------|-------|------|
| `allow_local_remote_addr` | `"1"` / `"0"` | 是否允许规则目标地址指向本地/内网地址 |

约定：
- 未配置时按 `"0"` 处理
- `"1"` 表示允许
- 其他值一律按关闭处理

### 2. 后端行为

新增一个轻量辅助函数，用于读取该配置开关：

```go
func (h *Handler) allowLocalRemoteAddr() bool {
    if h == nil || h.repo == nil {
        return false
    }
    cfg, err := h.repo.GetConfigByName("allow_local_remote_addr")
    if err != nil || cfg == nil {
        return false
    }
    return strings.TrimSpace(cfg.Value) == "1"
}
```

在以下路径中应用：
- `forwardCreate`
- `forwardUpdate`

行为改为：
- 当开关关闭时，继续执行 `IsSafeRemoteAddr(remoteAddr)`
- 当开关开启时，跳过这条“本地/内网地址禁止”校验

这样可以把改动范围限定在规则创建/编辑，不改变其他依赖 `IsSafeRemoteAddr()` 的场景。

### 3. 前端设置页

在 `vite-frontend/src/pages/config.tsx` 增加一个全局开关配置项。

建议文案：

- 标签：`允许转发到本地地址`
- 描述：`开启后，规则目标地址可指向 127.0.0.1、10.x.x.x、172.16-31.x.x、192.168.x.x 等本地或内网地址。默认关闭以降低开放代理风险。`

控件类型：
- 使用现有设置页的布尔开关模式

默认显示策略：
- 不依赖其他配置项
- 直接显示在设置页的网络/安全相关区域；若现有页面没有单独分区，则先按现有配置项组织方式加入即可

### 4. 错误与兼容性

关闭开关时：
- 保持现有错误行为，继续阻止本地/内网地址

开启开关时：
- 仅放开“本地/内网地址禁止”这条限制
- 仍保留地址格式解析失败等其他错误

### 5. 测试

需要补两类后端契约测试：

1. 开关关闭时拒绝本地/内网地址
- 创建规则时使用本地/内网地址
- 断言接口返回非 0 code

2. 开关开启时允许本地/内网地址
- 先写入 `vite_config(name=allow_local_remote_addr, value=1)`
- 创建或更新规则时使用相同地址
- 断言接口成功

建议至少覆盖：
- create 路径
- update 路径
- 多目标地址输入（逗号或换行分隔）中包含本地地址时的行为

## 风险与约束

1. 该开关会降低默认安全防护，应明确标注风险。
2. 这是全局开关，不做用户级或规则级细分控制。
3. 该开关只影响规则目标地址校验，不影响其他独立的安全策略。

## 推荐实施顺序

1. 先补失败的后端契约测试
2. 实现后端配置读取与创建/更新分支控制
3. 在设置页增加开关
4. 跑后端测试与前端构建验证
