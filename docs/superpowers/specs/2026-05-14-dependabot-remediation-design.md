# FLVX Dependabot 告警修复设计

**日期**: 2026-05-14
**状态**: 待审核
**范围**: 仅处理 GitHub Dependabot 依赖告警

## 概述

本设计针对 `Sagit-chu/flvx` 当前 open Dependabot alerts 制定依赖修复方案。目标是在不混入业务安全逻辑改造的前提下，消除或最大限度降低依赖漏洞告警，并通过各模块现有构建和测试命令验证兼容性。

当前告警共 21 条：

- Critical: 1
- High: 6
- Medium: 13
- Low: 1

按生态划分：

- Go: 14
- npm: 7

Go 告警中有一部分因为 `go-gost/go.mod` 和 `go-gost/x/go.mod` 同时被扫描而重复出现；实际修复应按依赖和模块关系聚合处理，而不是按 alert 数逐条机械修改。

## 目标

1. 修复 `go-backend` 中 `github.com/jackc/pgx/v5` 的 critical 和 low 告警。
2. 修复 `vite-frontend` 中 npm 直接依赖、开发依赖和 lockfile 传递依赖告警。
3. 修复 `go-gost` 与 `go-gost/x` 中可升级到 patched version 的 Go 依赖告警。
4. 对 Dependabot 未给出 patched version 的依赖进行单独确认，避免盲目大版本升级。
5. 保持改动最小化，便于回滚和定位 CI 失败。

## 非目标

1. 不处理既有认证、配置读取、备份导出、JWT 失效等业务安全逻辑问题。
2. 不合并或修改 `2026-05-13-security-remediation` 相关设计和计划。
3. 不进行 `go get -u ./...` 或 `pnpm update` 级别的大范围依赖升级。
4. 不引入前端测试框架。
5. 不编辑 `install.sh`、`panel_install.sh` 或 generated `.pb.go` 文件。

## 影响范围

### Backend

- `go-backend/go.mod`
- `go-backend/go.sum`

### Frontend

- `vite-frontend/package.json`
- `vite-frontend/pnpm-lock.yaml`

### Agent

- `go-gost/go.mod`
- `go-gost/go.sum`
- `go-gost/x/go.mod`
- `go-gost/x/go.sum`

`go-gost/go.mod` 使用：

```go
replace github.com/go-gost/x => ./x
```

因此 `go-gost/x` 的依赖修复应先完成，再验证 `go-gost` 主模块。

## 修复策略

采用“分模块、最小安全升级”策略。

### 1. go-backend

Dependabot alerts:

- `github.com/jackc/pgx/v5 < 5.9.0`
  - severity: critical
  - summary: memory-safety vulnerability
- `github.com/jackc/pgx/v5 < 5.9.2`
  - severity: low
  - summary: SQL injection via placeholder confusion with dollar quoted string literals

当前版本：

- `github.com/jackc/pgx/v5 v5.7.3`

目标版本：

- `github.com/jackc/pgx/v5 v5.9.2`

设计说明：

- 直接升到 `v5.9.2`，同时覆盖 `v5.9.0` 和 `v5.9.2` 的修复要求。
- 不调整 GORM PostgreSQL driver，除非 `go mod tidy` 或测试显示必须联动升级。
- 验证以 backend 全量测试为准。

验证命令：

```bash
(cd go-backend && go test ./...)
```

### 2. vite-frontend

Dependabot alerts:

- `postcss < 8.5.10`
  - appears in `vite-frontend/package.json`
  - appears in `vite-frontend/pnpm-lock.yaml`
- `serialize-javascript <= 7.0.2` and `< 7.0.5`
  - lockfile includes `serialize-javascript@6.0.2`
  - package override currently pins `serialize-javascript` to `7.0.3`
- `fast-uri <= 3.1.1`
  - lockfile currently includes `fast-uri@3.1.0`
- `@babel/plugin-transform-modules-systemjs <= 7.29.3`
  - lockfile currently includes `7.29.0`

目标版本：

- `postcss >= 8.5.10`
- `serialize-javascript >= 7.0.5`
- `fast-uri >= 3.1.2`
- `@babel/plugin-transform-modules-systemjs >= 7.29.4`

设计说明：

- 对直接声明的 `postcss` 更新 `package.json`。
- 将 `overrides.serialize-javascript` 从 `7.0.3` 更新到 `7.0.5`。
- 对只出现在 lockfile 的传递依赖，优先通过 `pnpm install` 重新解析 lockfile，让上游范围自然选择 patched version。
- 如果 lockfile 仍保留 vulnerable 版本，再添加精确 `pnpm.overrides` 或现有 `overrides` 条目，避免无关依赖大升级。
- 不引入前端测试框架，验证使用现有 build。

验证命令：

```bash
(cd vite-frontend && pnpm run build)
```

可选补充检查：

```bash
(cd vite-frontend && pnpm why postcss serialize-javascript fast-uri @babel/plugin-transform-modules-systemjs)
```

### 3. go-gost/x

Dependabot alerts:

- `github.com/sirupsen/logrus < 1.8.3`
  - severity: high
  - current: `v1.8.1`
  - target: at least `v1.8.3`
- `github.com/quic-go/quic-go < 0.57.0`
  - severity: medium
  - current: `v0.49.1`
  - target: `v0.57.0`
- `github.com/quic-go/webtransport-go <= 0.9.0`
  - severity: medium
  - current: `v0.8.1-0.20241018022711-4ac2c9250e66`
  - target: `v0.10.0`
- `github.com/pion/dtls/v2 <= 2.2.12`
  - severity: medium
  - current: `v2.2.6`
  - target: no patched version provided by Dependabot

设计说明：

- 先处理 `go-gost/x`，因为它是 `go-gost` 通过 `replace` 使用的本地模块。
- 将 `logrus`、`quic-go` 和 `webtransport-go` 升级到 Dependabot 标出的 patched version。
- 单独处理 `pion/dtls/v2`，因为 Dependabot 没有提供 `first_patched_version`。
- 对 `pion/dtls/v2`，先查询可用 module versions 和 advisory 详情。如果存在 patched `v2` release，使用最小安全修复版本；如果不存在 patched version，则记录残留告警，避免在没有兼容性评估的情况下强行做高风险大版本迁移。
- 升级完成后，在 `go-gost/x` 中运行 `go mod tidy` 并编译/测试该模块。

验证命令:

```bash
(cd go-gost/x && go test ./...)
```

### 4. go-gost

Dependabot reports the same vulnerable Go dependencies in `go-gost/go.mod`.

设计说明：

- `go-gost/x` 修复后，再更新 `go-gost` 的 module requirements，使主模块也解析到 patched versions。
- 保留 `replace github.com/go-gost/x => ./x`。
- 使用针对具体漏洞依赖的 `go get` 命令，不使用宽泛的 `go get -u`。
- 定向升级后运行 `go mod tidy`。

验证命令:

```bash
(cd go-gost && go test ./...)
(cd go-gost && go build .)
```

## 处理顺序

1. 修复 `go-backend` 的 `pgx/v5`。
2. 修复 `vite-frontend` 的 npm dependencies 和 lockfile。
3. 修复 `go-gost/x` 的 Go dependencies。
4. 同步并验证 `go-gost`。
5. 再次查询 Dependabot alerts，确认 alert 数量下降，或记录有意保留的未解决告警。

这个顺序可以降低耦合：backend 和 frontend 能独立验证，而 `go-gost/x` 因本地 module replacement 必须先于 `go-gost` 处理。

## 错误处理与回退

如果定向依赖升级无法解析：

1. 使用 `go mod why`、`go mod graph` 或 `pnpm why` 检查依赖链。
2. 优先添加最小显式 requirement 或 override，以强制解析到 patched version。
3. 除非定向解析不可行，否则避免宽泛升级。
4. 如果 patched version 不可用，记录准确 advisory、受影响依赖、当前暴露面，以及保留 open 状态的原因。

如果验证失败：

1. 将失败范围限制在当前升级的模块内。
2. 先分析编译错误或测试失败，再决定是否调整版本。
3. 优先选择能通过测试和构建的最低 patched version。
4. 不通过删除测试或修改无关应用代码来掩盖失败。

## 成功标准

1. `pgx/v5` 升级后，`go-backend` 测试通过。
2. npm dependency 和 lockfile 更新后，frontend production build 通过。
3. 定向升级后，`go-gost/x` 测试通过。
4. 同步 module requirements 后，`go-gost` 测试和构建通过。
5. 最终 Dependabot API 查询显示所有可修复告警已关闭或数量明确下降。
6. 任何剩余告警都有明确记录；尤其是 `github.com/pion/dtls/v2` 如果不存在 patched version，需要记录原因和下一步动作。
