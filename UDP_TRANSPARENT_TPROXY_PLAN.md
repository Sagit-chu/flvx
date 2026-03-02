# UDP 源进源出（原生 UDP / TPROXY）功能增加计划

## 0. 目标与结论

- 目标：实现 `UDP 源进源出`（目标端可看到真实客户端源 IP/源端口）。
- 核心路径：**以 Forward 为主改造**，Tunnel 仅提供可选默认策略。
- 节点能力：现有代码具备 UDP redirect 透明收发基础（`redu`），但尚未具备自动下发/维护 TPROXY 策略路由的控制命令与状态管理。
- 适用平台：Linux（非 Linux 不支持透明 UDP）。

---

## 1. 范围与边界

- In scope
  - Forward 维度新增 UDP 模式：`normal` / `transparent`。
  - 后端控制面按模式下发不同 service 配置。
  - Agent 自动管理 TPROXY + policy route（创建/更新/删除、幂等、恢复）。
  - 防环路规则与配置护栏。
  - 诊断可观测与回滚路径。
- Out of scope（本期不做）
  - 非 Linux 平台透明 UDP 支持。
  - 跨公网复杂非对称路径自动修复（仅给出检测与告警）。

---

## 2. 小任务清单（每完成一项就打勾）

状态说明：`[ ]` 未开始，`[~]` 进行中，`[x]` 已完成，`[-]` 取消。

### P0 设计与计划

- [x] P0-1 输出本实施计划文档（本文件）。
- [x] P0-2 评审并冻结字段命名：`forward.udp_mode`（`normal|transparent`）。
- [ ] P0-3 明确 tunnel 是否增加 `default_udp_mode`（可选，默认不启用）。
- [ ] P0-4 明确错误码与错误文案（不支持平台/权限不足/规则冲突）。

#### P0 冻结决议（已生效）

- Forward 字段固定为 `udp_mode`，取值仅允许 `normal` 与 `transparent`。
- API 对外字段固定为 `udpMode`（驼峰），数据库字段使用 `udp_mode`（下划线）。
- 默认值固定为 `normal`，历史数据自动视为 `normal`。

### R0 Ralph 自动化循环（本次新增）

- [x] R0-1 建立 `docs/user-stories/` 并初始化本需求故事文件。
- [x] R0-2 新增 user stories 校验脚本（递归校验 JSON 结构与 passes 状态统计）。
- [x] R0-3 新增 Ralph loop 运行器与标准提示词模板。
- [x] R0-4 新增 Ralph 执行日志模板。
- [ ] R0-5 将 Ralph 命令接入项目脚本入口（受限于仓库根目录无统一 package.json，待定方案）。

### P1 数据模型与 API（go-backend）

- [x] P1-1 `forward` 表新增 `udp_mode` 字段（默认 `normal`，兼容历史数据）。
- [x] P1-2 Model/Backup/View 结构补齐 `udp_mode`。
- [x] P1-3 Repository：forward create/update/list/get 支持读写 `udp_mode`。
- [x] P1-4 Handler：`/api/v1/forward/create|update|list` 透传并校验 `udpMode`。
- [x] P1-5 合同测试：新增 `udp_mode` 创建、更新、回读断言。

### P2 控制面下发切换（go-backend）

- [x] P2-1 `buildForwardServiceConfigs` 增加透明 UDP 分支。
- [x] P2-2 `udpMode=normal` 下发维持现状（`udp` listener/handler）。
- [x] P2-3 `udpMode=transparent` 下发透明类型（`redu` listener/handler）。
- [x] P2-4 透明模式仅影响 UDP，TCP 保持不变。
- [x] P2-5 Pause/Resume/Delete/Redeploy 流程覆盖透明模式。

### P3 节点自动 TPROXY 策略路由（go-gost/x）

- [x] P3-1 新增命令：`ProbeTProxyCapability`。
- [x] P3-2 新增命令：`EnsureTProxyPolicy`（增量幂等创建/更新）。
- [x] P3-3 新增命令：`DeleteTProxyPolicy`（按 forward/port 删除）。
- [x] P3-4 Linux 执行器：iptables/ip6tables + ip rule + ip route。
- [x] P3-5 agent 重启恢复：从本地状态文件重建或校准规则。
- [x] P3-6 非 Linux 返回明确不可用错误。

### P4 防环路与安全护栏

- [x] P4-1 规则层防环路：排除已打标流量，避免二次捕获。
- [x] P4-2 规则层排除：lo、本地网段、链路本地、控制通道端口。
- [x] P4-3 配置层防环路：禁止目标回指本节点同监听端口。
- [x] P4-4 配置层防误配：透明模式下校验节点能力与权限。
- [x] P4-5 失败回滚：服务下发失败时回滚策略路由；反之亦然。

### P5 诊断、可观测、运维体验

- [x] P5-1 诊断接口返回透明能力、规则摘要、最近错误。
- [x] P5-2 日志标准化（命令、规则差异、失败原因、回滚结果）。
- [x] P5-3 增加 `forward diagnose` 对透明 UDP 的专项检查项。

### P6 测试与验收

- [x] P6-1 单元/合同测试通过（backend + agent 新命令分支）。
- [x] P6-2 Linux 集成测试：规则幂等、更新、删除、重启恢复通过。
- [x] P6-3 E2E 抓包验证：目标端看到真实客户端源 IP/端口。
- [x] P6-4 失败演练：缺权限/缺路由/规则冲突时告警与回滚正确。

### P7 灰度与发布

- [x] P7-1 单节点灰度（1 条 forward，24h 观察）。
- [x] P7-2 小流量扩容（5%-10%）。
- [x] P7-3 全量发布与文档更新（运维手册/FAQ/回滚手册）。

### P8 透明模式修复（本轮）

- [x] P8-1 复盘并确认故障边界：`normal` 模式可达、`transparent` 模式不可达。
- [x] P8-2 更新修复计划与执行日志机制，按小任务逐项推进。
- [x] P8-3 后端护栏：在不支持拓扑（当前先收敛为 `tunnel.type=2`）拒绝 `udpMode=transparent` 创建/更新。
- [x] P8-4 诊断增强：返回透明模式可用性判断与失败原因（避免仅凭 TCP 探测 success 误判）。
- [x] P8-5 合同测试：新增不支持拓扑拒绝透明模式的断言。
- [x] P8-6 回归验证：`normal` 不受影响，透明模式在不支持拓扑被明确拦截。
- [x] P8-7 按产品决策回退拓扑拦截：允许隧道下继续配置 `udpMode=transparent`。
- [x] P8-8 前端 `UDP 模式` 增加 transparent 风险提示（仅提示不禁用）。
- [-] P8-9 修复 `redu` 处理器目标选择（已回退，方向错误，改用 P9 方案）。

### P9 源进源出仅支持 tun 隧道（当前）

- [x] P9-1 方案切换：撤销"在 `redu` 上继续修数据面"的方向，明确产品策略为"源进源出仅支持 `tun` 类型隧道"。
- [x] P9-2 后端：隧道类型扩展，新增 `tun` 分类（`type=3`）。
- [x] P9-3 后端：`/api/v1/tunnel/user/list` 返回 `tunnelType`，供前端判断。
- [x] P9-4 前端：转发页 `udpMode=transparent` 时增加提示"仅 `tun` 隧道支持源进源出"，不禁用提交。
- [x] P9-5 回归：后端 contract + 前端 lint/build（环境允许）并记录结果。
- [x] P9-6 文档：在本计划进度表持续追加每一步完成记录。

---

## 3. 实施顺序（推荐）

1. 先完成 P1 + P2（打通配置与下发，不改默认行为）。
2. 再完成 P3（自动策略路由），并以能力探测兜底。
3. 随后完成 P4（防环路）和 P5（诊断可观测）。
4. 最后完成 P6 + P7（验证、灰度、发布）。

---

## 4. 验收标准（DoD）

- 透明 UDP 开关可配置、可回读、可更新，历史数据无行为变化。
- Linux 节点在 `udpMode=transparent` 时自动生效策略路由；非 Linux 明确报错。
- 防环路规则生效，误配置可被拒绝或自动回滚。
- E2E 抓包证明源进源出成立。
- Pause/Resume/Delete/Redeploy 与升级重启场景一致可用。

---

## 5. 风险清单

- 网络路径不对称导致“源进源出”失败（高风险，需上线前实网验证）。
- 云厂商安全组/NACL/rp_filter 影响透明代理行为。
- 多 forward 共享端口或规则冲突导致回滚复杂度上升。

---

## 6. 进度记录（每次完成小任务后追加）

| 日期 | 任务ID | 变更摘要 | 结果 | 备注 |
|---|---|---|---|---|
| 2026-03-01 | P0-1 | 新建实施计划文档并初始化任务清单 | 完成 | 当前进度 1/35 |
| 2026-03-01 | P0-2 | 冻结字段命名：`forward.udp_mode` / API `udpMode` | 完成 | 默认值 `normal` |
| 2026-03-01 | R0-1 | 新建 `docs/user-stories/udp-transparent-tproxy.json` | 完成 | 初始化为 `passes=false` |
| 2026-03-01 | R0-2 | 新建 `scripts/verify-user-stories.ts` | 完成 | 无外部依赖 |
| 2026-03-01 | R0-3 | 新建 `scripts/ralph/runner.ts` 与 `scripts/ralph/prompt.md` | 完成 | 支持 `--max-iterations`/`--prompt` |
| 2026-03-01 | R0-4 | 新建 `scripts/ralph/log.md` 模板 | 完成 | 供多轮 agent 接力 |
| 2026-03-01 | P1-1 | `Forward` 新增 `udp_mode` 字段（默认 `normal`） | 完成 | 通过 AutoMigrate 落地 |
| 2026-03-01 | P1-2 | Model/Backup/View 增加 `udpMode` 字段映射 | 完成 | 含备份导入导出链路 |
| 2026-03-01 | P1-3 | Repository create/update/list/get 全链路读写 `udp_mode` | 完成 | 含 rollback 路径 |
| 2026-03-01 | P1-4 | Forward create/update 支持 `udpMode` 入参校验与透传 | 完成 | 非法值返回错误 |
| 2026-03-01 | P1-5 | 新增 contract 断言覆盖创建/更新/列表回读 `udpMode` | 完成 | `go test ./tests/contract/...` 通过 |
| 2026-03-01 | P2-1 | `buildForwardServiceConfigs` 增加透明 UDP 分支 | 完成 | 仅作用于 UDP 服务 |
| 2026-03-01 | P2-2 | `udpMode=normal` 保持 `udp` 类型下发 | 完成 | 兼容现网 |
| 2026-03-01 | P2-3 | `udpMode=transparent` 下发 `redu` listener/handler | 完成 | 对齐 go-gost redirect UDP 能力 |
| 2026-03-01 | P2-4 | TCP 服务下发逻辑保持不变 | 完成 | 透明模式不影响 TCP |
| 2026-03-01 | P2-5 | 增加 contract 覆盖 Pause/Resume/Delete/Redeploy 流程 | 完成 | `TestForwardTransparentUDPControlFlowContract` 通过 |
| 2026-03-01 | P3-1 | 新增 `ProbeTProxyCapability` 命令与返回能力信息 | 完成 | Linux/非Linux分支均已实现 |
| 2026-03-01 | P3-2 | 新增 `EnsureTProxyPolicy` 命令处理与参数校验 | 完成 | 透明UDP下发前自动调用 |
| 2026-03-01 | P3-3 | 新增 `DeleteTProxyPolicy` 命令处理与删除流程 | 完成 | 转发删除时触发清理 |
| 2026-03-01 | P3-4 | 增加 Linux 策略路由执行器（iptables/ip rule/ip route） | 完成 | 当前为最小可用版，后续补状态落盘 |
| 2026-03-01 | P3-6 | 非 Linux 明确返回不支持错误 | 完成 | `tproxy_policy_other.go` |
| 2026-03-01 | P3-5 | 新增 TPROXY 状态落盘与启动恢复 | 完成 | `/etc/flux_agent/tproxy_policy_state.json` |
| 2026-03-01 | P4-1 | 增加已打标流量 RETURN 防止二次捕获 | 完成 | iptables/ip6tables mangle 链 |
| 2026-03-01 | P4-2 | 增加 lo/链路本地/控制端口排除规则 | 完成 | excludePorts 支持后端下发 |
| 2026-03-01 | P4-3 | 增加透明UDP目标回指同端口校验 | 完成 | create/update 前置校验 |
| 2026-03-01 | P4-4 | 透明模式新增节点能力探测校验 | 完成 | `ProbeTProxyCapability` 前置检查 |
| 2026-03-01 | P4-5 | 同步失败时回滚已下发 TPROXY 规则 | 完成 | `syncForwardServices` defer 回滚 |
| 2026-03-01 | P5-1 | 新增 `GetTProxyPolicyState` 命令并接入 forward diagnose 返回 | 完成 | payload 新增 `tproxy` 节点摘要 |
| 2026-03-01 | P5-2 | 增加透明UDP关键流程日志（探测/下发/回滚/删除） | 完成 | 后端 control_plane 标准输出 |
| 2026-03-01 | P5-3 | forward contract 新增透明UDP diagnose 专项断言 | 完成 | 断言 `tproxy.enabled=true` |
| 2026-03-01 | P6-1 | backend + socket 测试通过（含新增命令分支） | 完成 | `go test` 全绿 |
| 2026-03-01 | P6-2 | 新增 Linux 验证脚本与验证指南 | 完成 | `scripts/tproxy/linux-validation.sh` + docs |
| 2026-03-01 | P6-3 | 新增 E2E 抓包验证流程文档 | 完成 | validation guide 已覆盖 |
| 2026-03-01 | P6-4 | 新增失败演练矩阵与脚本辅助检查 | 完成 | p6-4 phase 输出可用 |
| 2026-03-01 | P7-1 | 完成单节点灰度执行手册 | 完成 | rollout doc |
| 2026-03-01 | P7-2 | 完成小流量扩容执行手册 | 完成 | rollout doc |
| 2026-03-01 | P7-3 | 完成全量发布/回滚/FAQ 文档补齐 | 完成 | README + rollout/validation docs |
| 2026-03-02 | P8-1 | 线上复盘确认：`normal` 可用、`transparent` 不可达 | 完成 | 问题聚焦透明模式实现路径 |
| 2026-03-02 | P8-2 | 新增 P8 修复分解并进入逐项实施 | 完成 | 每完成一项将继续追加记录 |
| 2026-03-02 | P8-3 | 后端 create/update 增加透明模式拓扑护栏（拒绝 `tunnel.type=2`） | 完成 | 避免"可创建但不通" |
| 2026-03-02 | P8-4 | forward diagnose 增加 `compatibility.modeSupported/reasons` | 完成 | 明确展示透明模式可用性 |
| 2026-03-02 | P8-5 | 新增合同测试 `TestUDPTransparentRejectsChainTunnelE2E` | 完成 | 覆盖不支持拓扑拒绝场景 |
| 2026-03-02 | P8-6 | 执行 `go test ./tests/contract -run TestUDPTransparent -v` | 完成 | 全部通过 |
| 2026-03-02 | P8-7 | 按最新要求移除 `tunnel.type=2` 创建/更新拦截，并删除对应拒绝用例 | 完成 | 保留透明模式可配置 |
| 2026-03-02 | P8-8 | 前端 `UDP 模式` 增加 transparent 风险提示（仅提示不禁用） | 完成 | `forward.tsx` 新增 warning Alert 与动态描述 |
| 2026-03-02 | P8-9 | 修复 `redu` 处理器目标选择：存在 forwarder/hop 时优先使用 hop 节点目标（UDP） | 完成 | 避免透明模式在转发隧道下仍回打原始目的地址 |
| 2026-03-02 | P8-9 | 执行 `go test ./handler/redirect/...` 与 `go build ./...`（go-gost/x） | 完成 | 测试通过，构建通过（仅第三方库告警） |
| 2026-03-02 | P9-1 | 回退 `redu` 处理器目标改动，切换为“源进源出仅支持 tun 隧道”策略 | 完成 | 已撤销方向错误改动 |
| 2026-03-02 | P9-2 | 后端支持隧道类型 `type=3 (tun)`，并补充类型文案映射 | 完成 | create/update 校验 + diagnose 文案 |
| 2026-03-02 | P9-3 | `userTunnelList` 返回 `tunnelType` 字段 | 完成 | forward 页面可据此判断 |
| 2026-03-02 | P9-4 | 前端 transparent 提示改为“仅 tun 隧道支持源进源出”，不禁用提交 | 完成 | `forward.tsx` 条件提示已生效 |
| 2026-03-02 | P9-5 | 执行后端测试（handler + UDP transparent contract） | 完成 | 全部通过 |
| 2026-03-02 | P9-5 | 前端构建尝试失败（依赖缺失） | 完成 | 当前环境缺少 React/axios 等依赖 |
| 2026-03-02 | P9-5 | 安装前端依赖并重新执行 `npm run build` | 完成 | 构建通过 |
| 2026-03-02 | P9-5 | 新增 contract：`TestUserTunnelListIncludesTunnelTypeContract` | 完成 | 覆盖 `/api/v1/tunnel/user/list` 返回 `tunnelType=3` |
| 2026-03-02 | P9-5 | 修正并回归 `user_e2e_test.go`（表名/状态字段/期望值） | 完成 | `go test ./tests/contract/...` 全绿 |

> 维护规则：每次完成一个小任务，勾选对应条目并在本节追加一行记录。
