# nftables 流量统计设计

**日期**: 2026-06-06
**状态**: 待审核
**作者**: Codex

## 概述

为 FLVX 的 `nftables` 转发模式补齐流量统计。当前 nftables 模式由面板通过 SSH 全量维护 `table inet flvx`，但没有 agent，因此不能复用 WebSocket 运行时上报。新方案由面板定时通过 SSH 拉取远端 nftables counter，计算增量后写入现有流量账本。

目标是让 nftables 转发在用户可见口径上尽量接近 agent 模式：

- forward 列表显示 `inFlow` / `outFlow`。
- 用户、用户隧道、配额和流量策略继续生效。
- 隧道监控继续获得分钟级 `tunnel_metric`。
- 节点不需要安装新的 agent 或常驻进程。

## 背景

现有 agent 模式通过 `/flow/upload` 接收加密上报，handler 会把服务名解析为 `forward_id/user_id/user_tunnel_id`，再复用以下路径：

- `ApplyFlowUploadDeltasBatch` 更新 `forward`、`user`、`user_tunnel`。
- `AddUserQuotaUsageBatch` 更新用户配额窗口。
- `enforceUserQuotaIfNeeded` 和 `enforceFlowPolicies` 做约束 enforcement。
- `recordTunnelMetricsFromForwardBatch` 写入分钟级隧道监控。

nftables 模式已经有 `nft_rule_binding` 记录规则应用状态，规则 comment 里包含 `forward_id`。这给 counter 到业务实体的映射提供了稳定锚点。

## 推荐方案

采用“面板 SSH 轮询 nftables counter”的方案：

1. 渲染 nftables 规则时，为每个 forward、协议和方向写入稳定 comment 和 `counter`。
2. 后端定时扫描 `forward_mode = nftables` 的节点。
3. 对每个节点通过 SSH 执行 `nft -j list table inet flvx`。
4. 解析 JSON 规则，按 comment 得到 `forward_id/protocol/direction/bytes/packets`。
5. 用数据库中的上次采样值计算 delta。
6. 将 delta 转成现有 flow upload 内部结构，复用既有入账、配额、策略和监控逻辑。

不采用节点 crontab 或 systemd timer 回推。它会重新引入节点侧组件，削弱 nftables 模式“不安装 agent”的产品边界。

## 统计口径

正式入账使用 `forward` filter chain 的计数，不使用 NAT chain 的 DNAT 命中计数作为主口径。

原因：

- DNAT counter 表示规则命中，不一定代表后续转发成功。
- filter forward chain 更接近实际经过内核转发的数据。
- SNAT/masquerade 会改变包头，入账规则应在可稳定匹配目标服务地址和端口的位置统计。

方向定义：

| direction | nft 匹配 | 写入字段 |
|-----------|----------|----------|
| `to-target` | 外部客户端到目标服务 | `in_flow` |
| `from-target` | 目标服务返回外部客户端 | `out_flow` |

用户总用量和配额仍按 `in_flow + out_flow` 计算。隧道 `traffic_ratio` 和 `flow` 倍率继续沿用 agent 模式逻辑，保证不同运行时模式的账单口径一致。

## nftables 规则设计

继续只维护 `table inet flvx`，避免触碰用户已有规则。每条 forward 对 TCP 和 UDP 各生成一组 DNAT 和统计规则。

示例：

```nft
table inet flvx {
  chain prerouting {
    type nat hook prerouting priority dstnat; policy accept;
    tcp dport 12345 counter dnat ip to 198.51.100.20:443 comment "flvx forward:42 dnat tcp"
    udp dport 12345 counter dnat ip to 198.51.100.20:443 comment "flvx forward:42 dnat udp"
  }

  chain postrouting {
    type nat hook postrouting priority srcnat; policy accept;
    masquerade comment "flvx masquerade"
  }

  chain forward {
    type filter hook forward priority filter; policy accept;
    ip daddr 198.51.100.20 tcp dport 443 counter comment "flvx forward:42 to-target tcp"
    ip saddr 198.51.100.20 tcp sport 443 counter comment "flvx forward:42 from-target tcp"
    ip daddr 198.51.100.20 udp dport 443 counter comment "flvx forward:42 to-target udp"
    ip saddr 198.51.100.20 udp sport 443 counter comment "flvx forward:42 from-target udp"
  }
}
```

IPv6 目标使用 `ip6`：

```nft
ip6 daddr 2001:db8::20 tcp dport 443 counter comment "flvx forward:42 to-target tcp"
ip6 saddr 2001:db8::20 tcp sport 443 counter comment "flvx forward:42 from-target tcp"
```

域名目标无法在 nftables 规则中动态匹配返回方向。统计第一阶段要求 nftables forward 的 `remoteAddr` host 必须是 IP 地址；如果当前纯转发实现允许域名，开启统计时应同步收紧校验。后续若要支持域名，应在规则同步时解析并固化 IP，同时明确 DNS 变化后的重建策略。

## Comment 格式

正式统计规则使用固定格式：

```text
flvx forward:<forward_id> <direction> <protocol>
```

字段：

- `forward_id`: 十进制整数。
- `direction`: `to-target` 或 `from-target`。
- `protocol`: `tcp` 或 `udp`。

DNAT 调试规则可使用 `dnat` direction，但 collector 不入账 `dnat`。后端只依赖 comment 解析，不依赖 nft handle，因为全量重建 table 会改变 handle。

## 数据模型

新增 `nft_counter_state` 表保存上次采样基线。

| 字段 | 说明 |
|------|------|
| `id` | 主键 |
| `node_id` | nftables 节点 ID |
| `forward_id` | 转发规则 ID |
| `protocol` | `tcp` / `udp` |
| `direction` | `to-target` / `from-target` |
| `rule_hash` | 当前规则 hash |
| `bytes` | 上次采样绝对字节数 |
| `packets` | 上次采样绝对包数 |
| `collected_time` | 上次采样时间 |
| `created_time` | 创建时间 |
| `updated_time` | 更新时间 |

唯一索引：

```text
node_id, forward_id, protocol, direction
```

GORM 模型必须定义 `TableName()`，字段 tag 保持 SQLite/PostgreSQL 兼容，不使用 `jsonb`、`serial` 等数据库专属类型。

## 后端组件

扩展 `go-backend/internal/runtime/nftables`：

| 组件 | 职责 |
|------|------|
| `CounterSample` | 表达单条 nft counter 采样 |
| `Collector` | 对外提供 `Collect(ctx, cfg)` |
| `SSHRunner.ListTableJSON` | 远端执行 `nft -j list table inet flvx` |
| `ParseCounterSamples` | 解析 nft JSON 和 FLVX comment |

扩展 repository：

| 方法 | 职责 |
|------|------|
| `ListNftablesNodesForCollection` | 找到启用 nftables 且有 SSH 配置的节点 |
| `GetNftCounterStatesByNode` | 读取节点上次 counter 基线 |
| `UpsertNftCounterStates` | 批量刷新基线 |
| `DeleteNftCounterStatesByForward` | forward 删除时清理状态 |

扩展 handler/job：

- 新增 `runNftablesTrafficCollectJob(now time.Time)`。
- 默认每 60 秒运行一次。
- 对节点采集设置并发上限，建议 3 到 5。
- 单节点失败只记录日志和节点采集状态，不影响其他节点。

## 增量算法

collector 返回的是 nftables 的绝对 counter。入账前必须和上次基线做差。

规则：

- 无旧状态：只保存当前值作为基线，不入账。
- `rule_hash` 变化：只刷新基线，不入账，避免新旧规则混算。
- 新 bytes 大于等于旧 bytes：`delta = new - old`。
- 新 bytes 小于旧 bytes：认为远端 table 重建、counter reset 或系统重启，只刷新基线，不入账。
- delta 为 0：刷新采集时间，不入账。
- 样本无法映射到有效 forward：忽略并记录 debug 日志。

同一 forward 的 TCP/UDP delta 要先聚合，再转换成现有账本：

- `to-target` bytes 聚合为原始 `bytesIn`。
- `from-target` bytes 聚合为原始 `bytesOut`。
- 入账时按 `traffic_ratio` 和 `tunnel.flow` 计算 scaled `InFlow` / `OutFlow`。
- 配额使用 scaled 后的 `InFlow + OutFlow`。
- `tunnel_metric` 使用原始 `bytesIn` / `bytesOut`。

## 入账路径

新增一个 nftables 专用的 batch builder，但输出沿用现有结构：

```go
type nftTrafficDelta struct {
    ForwardID int64
    BytesIn   int64
    BytesOut  int64
}
```

处理流程：

1. 收集本轮所有 `forward_id`。
2. 调用 `GetFlowUploadForwardMetas` 获取 `user_id/user_tunnel_id/tunnel_id/traffic_ratio/tunnel_flow`。
3. 构造 `repo.FlowUploadCounterDelta`。
4. 调用 `recordTunnelMetricsFromForwardBatch` 写监控。
5. 抽出共享入账 helper，复用 `applyFlowDeltasWithFallback`、`applyQuotaUsageWithFallback`、`enforceUserQuotaIfNeeded` 和 `enforceFlowPolicies`。不要通过伪造 agent service name 去调用 agent 专用 builder。

不新增独立的 nftables 流量字段。`forward.in_flow/out_flow`、`user.in_flow/out_flow`、`user_tunnel.in_flow/out_flow` 仍是统一事实来源。

## 错误处理

采集错误分为三类：

| 类型 | 行为 |
|------|------|
| SSH 连接或认证失败 | 记录日志，保留下次继续采集 |
| 远端无 `table inet flvx` | 视为规则未应用或被清理，记录 warning，不清空账本 |
| JSON 解析失败 | 记录原始错误摘要，不入账 |

不要因为采集失败禁用 forward。流量统计失败和转发运行失败不是同一件事。

可在后续 UI 增加节点级采集状态，例如最近成功时间、最近错误。但第一步只要求后端具备日志和数据库状态即可。

## 与现有行为的关系

- agent 模式 `/flow/upload` 不变。
- nftables 模式不新增节点侧 HTTP 回调。
- 现有 `nft_rule_binding.rule_hash` 继续表示规则期望状态；counter state 用它判断采样是否跨规则版本。
- `statistics_flow` 小时统计 job 不需要改，它基于用户总流量快照自然包含 nftables 入账结果。
- 用户重置流量时不需要清空 nftables counter。重置只清业务账本；下一轮采集继续从 counter state 差值入账。

## 测试计划

后端单元测试：

- renderer 为 TCP/UDP、IPv4/IPv6 目标生成 `counter` 和稳定 comment。
- comment parser 能识别合法格式，拒绝未知 direction/protocol。
- nft JSON parser 能从 `nft -j list table` 输出中提取 bytes/packets。
- delta 算法覆盖首次基线、正常增长、counter reset、rule_hash 变化和零增量。
- batch builder 正确应用 `traffic_ratio` 和 `tunnel.flow`。

repository 测试：

- `nft_counter_state` 自动迁移。
- upsert 在 SQLite 下可重复刷新。
- forward 删除时清理 counter state。

handler/job 测试：

- 单节点采集成功会调用现有流量入账路径。
- 单节点 SSH 失败不影响其他节点。
- 无旧状态时不会误把历史 counter 入账。

验证命令：

```bash
(cd go-backend && go test ./...)
```

## 分阶段落地

第一阶段：

- 规则渲染加入 filter chain counter。
- 实现 SSH collector、JSON parser、counter state 和后台 job。
- 入账到现有账本和 tunnel metric。

第二阶段：

- UI 展示 nftables 采集状态。
- 节点详情显示最近采集时间和最近错误。
- 提供手动“采集一次”诊断按钮。

第三阶段：

- 探索域名目标的解析和重建策略。
- 优化大量节点下的采集调度、退避和超时配置。

## 开放问题

- 采集周期默认 60 秒是否满足产品预期；如果需要更实时，可以降到 30 秒，但 SSH 压力会增加。
- nftables 模式是否继续允许域名 remoteAddr。如果允许，需要先定义 DNS 固化和统计匹配规则。
- 是否要在第一阶段暴露采集状态 API。推荐后端先记录，UI 后续补齐。
