# 数据库 GORM ORM 迁移计划

**创建时间:** 2026-02-15
**更新时间:** 2026-02-17 (实施：完成 P1 + P2 + P3 + P5(Repo 查询层 + schema 收尾) + 测试/构建收尾)
**分支:** main (commit e5e22ba)
**状态:** 基本完成（保留 4 处 PG 序列修复 DDL `Exec`）

---

## 一、现状分析

### 1.1 迁移前架构 (已归档)

项目原使用 `database/sql` + 手写 raw SQL，通过 `internal/store/db.go` 中的运行时 SQL 重写层实现 SQLite/PostgreSQL 双数据库兼容。

| 组件 | 行数 | 角色 | 当前状态 |
|------|------|------|----------|
| `store/db.go` | ~520 | SQL 方言重写层 | **已删除** |
| `store/sqlite/repository.go` | ~3118 | Repository 查询方法 | **已重写为 store/repo/** |
| `handler/mutations.go` | ~3748 | Handler 内直接写 raw SQL | **已迁移到 repo（生产 SQL=0）** |
| `handler/handler.go` | ~1283 | 部分方法用 `repo.DB()` | **大部分已迁移** |
| `handler/federation.go` | ~若干 | Federation 相关 SQL | **已迁移到 repo** |
| `handler/control_plane.go` | ~若干 | 控制面相关 SQL | **已迁移到 repo** |
| `handler/flow_policy.go` | ~若干 | 流量策略相关 SQL | **已迁移到 repo** |
| `handler/jobs.go` | ~若干 | 后台任务相关 SQL | **已迁移到 repo** |
| `store/postgres/` | 目录 | PostgreSQL 专用 schema/data | **已删除** |

### 1.2 痛点 (迁移目标)

1. ~~**双 Schema 维护**~~：已通过 AutoMigrate 解决
2. ~~**SQL 重写层复杂**~~：db.go 已删除
3. ~~**handler 直接写 SQL**~~：`mutations.go` 生产路径 `tx.Exec`/`tx.Raw` 已清零（测试代码除外）
4. ~~**无类型安全**~~：repo 业务查询已 GORM 化；剩余 4 处为 PG 序列修复 DDL `Exec`（设计保留）
5. ~~**模型定义分散**~~：已集中到 model/model.go

---

## 二、方案：引入 GORM ORM（全面重写）

### 2.1 方案变更说明

原计划为 **方案 D（扩展现有 DDL 重写层）**，现变更为 **方案 A（GORM 全面重写）**。

### 2.2 选择 GORM 的理由

1. Go 生态最成熟的 ORM，社区庞大，文档完善
2. 原生支持 SQLite + PostgreSQL 双数据库，自动处理方言差异
3. AutoMigrate 消除双 schema 维护，自动处理 AUTOINCREMENT ↔ SERIAL 等
4. 类型安全的模型定义，编译期检查字段映射
5. 内置事务管理（closure pattern 自动 rollback/commit）
6. 自动处理 `"user"` 保留字引号

### 2.3 GORM 驱动选择

| 数据库 | 驱动 | 包 | 备注 |
|--------|------|-----|------|
| SQLite | modernc.org/sqlite (CGO-free) | `github.com/glebarez/sqlite` | 纯 Go，无需 CGO |
| PostgreSQL | pgx/v5 | `gorm.io/driver/postgres` | 默认使用 pgx |

> **注意**：标准 `gorm.io/driver/sqlite` 依赖 CGO，必须使用 `glebarez/sqlite` 包装器。

### 2.4 核心设计原则

1. **Model 集中定义**：所有 GORM Model 在 `internal/store/model/` 包中
2. **Repository 模式保留**：Repository struct 持有 `*gorm.DB`，对外方法签名尽量不变
3. **Handler 不直接操作 DB**：所有数据库操作必须封装在 Repository 方法中
4. **AutoMigrate 替代 schema.sql**：启动时自动迁移，不再维护手写 DDL
5. **保留 PG 序列修复**：pgloader 迁移场景仍需 `ensurePostgresIDDefaults()`
6. **Package 重命名**：`store/sqlite` → `store/repo`

---

## 三、Model 设计

### 3.1 GORM 类型映射

| Go 类型 | GORM 行为 | PostgreSQL | SQLite |
|---------|-----------|------------|--------|
| `int64` + `primaryKey` | 自增主键 | `bigserial` | `INTEGER PRIMARY KEY AUTOINCREMENT` |
| `int64` | 64位整数 | `bigint` | `integer` (SQLite 自动 64位) |
| `int` | 整数 | `integer` | `integer` |
| `float64` | 浮点 | `double precision` | `real` |
| `string` + `size:100` | 变长字符 | `varchar(100)` | `varchar(100)` |
| `string` (无 size) | 文本 | `text` | `text` |
| `sql.NullInt64` | 可空整数 | `bigint NULL` | `integer NULL` |
| `sql.NullString` | 可空文本 | `text NULL` | `text NULL` |

### 3.2 表清单（21 张表）

| 表名 | Model | 特殊处理 |
|------|-------|----------|
| `user` | `User` | `TableName()` 返回 `"user"` (PG 保留字) |
| `forward` | `Forward` | |
| `forward_port` | `ForwardPort` | |
| `node` | `Node` | |
| `speed_limit` | `SpeedLimit` | |
| `statistics_flow` | `StatisticsFlow` | |
| `tunnel` | `Tunnel` | |
| `chain_tunnel` | `ChainTunnel` | |
| `user_tunnel` | `UserTunnel` | 复合唯一索引 (user_id, tunnel_id) |
| `tunnel_group` | `TunnelGroup` | |
| `user_group` | `UserGroup` | |
| `tunnel_group_tunnel` | `TunnelGroupTunnel` | 复合唯一索引 |
| `user_group_user` | `UserGroupUser` | 复合唯一索引 |
| `group_permission` | `GroupPermission` | 复合唯一索引 |
| `group_permission_grant` | `GroupPermissionGrant` | 复合唯一索引 |
| `vite_config` | `ViteConfig` | name 唯一 |
| `peer_share` | `PeerShare` | token 唯一 |
| `peer_share_runtime` | `PeerShareRuntime` | reservation_id, resource_key 唯一 |
| `federation_tunnel_binding` | `FederationTunnelBinding` | 复合唯一索引 + resource_key 唯一 |
| `announcement` | `Announcement` | |
| `schema_version` | `SchemaVersion` | |

---

## 四、详细实施步骤

### 阶段 1：基础设施 — 添加依赖 + 定义 Model ✅ 已完成

| 步骤 | 任务 | 文件 | 状态 |
|------|------|------|------|
| 1.1 | `go get gorm.io/gorm gorm.io/driver/postgres github.com/glebarez/sqlite` | `go.mod` | ✅ |
| 1.2 | 创建 `internal/store/model/model.go`，定义全部 21 个表 Model | 新文件 | ✅ |
| 1.3 | 为 `user` 表添加 `TableName()` 处理 PG 保留字 | model.go | ✅ |
| 1.4 | 为复合唯一索引的表添加 GORM 索引 tag | model.go | ✅ |
| 1.5 | 将 Backup 相关 struct 也迁移到 model/ | model.go | ✅ |
| 1.6 | 验证 `go build ./...` 编译通过 | - | ✅ |

### 阶段 2：GORM DB 初始化 ✅ 已完成

| 步骤 | 任务 | 文件 | 状态 |
|------|------|------|------|
| 2.1 | 修改 Repository struct，`*store.DB` → `*gorm.DB` | repository.go | ✅ |
| 2.2 | 重写 `Open()` — 用 `glebarez/sqlite` 打开 SQLite | repository.go | ✅ |
| 2.3 | 重写 `OpenPostgres()` — 用 `gorm.io/driver/postgres` 打开 PG | repository.go | ✅ |
| 2.4 | 用 `db.AutoMigrate()` 替代 `bootstrapSchema()` | repository.go | ✅ |
| 2.5 | 实现种子数据逻辑（FirstOrCreate 替代 data.sql） | repository.go | ✅ |
| 2.6 | 保留并适配 `ensurePostgresIDDefaults()`（用 `db.Exec()`） | repository.go | ✅ |
| 2.7 | 保留并适配 `migrateSchema()` 增量迁移 | repository.go | ✅ |
| 2.8 | `DB()` 方法返回 `*gorm.DB` | repository.go | ✅ |
| 2.9 | SQLite 连接池设置 `MaxOpenConns(1)` 防锁 | repository.go | ✅ |

### 阶段 3：重写 repository 查询方法 ⚠️ ~97% 完成

将所有 raw SQL 查询替换为 GORM 链式调用。

> **2026-02-16 审计**：基础 CRUD 查询已 GORM 化，但 mutation、JOIN 查询、import/export 仍大量使用 raw SQL。
> **2026-02-17 更新**：已完成 `repository_mutations.go`、Import、以及 `repository_federation/control/flow` 查询层 GORM 化；`repository.go` 中 Raw 已清零，当前仅保留 4 处 PG 序列修复 DDL `Exec`。

| 步骤 | 任务 | 方法数 | 状态 |
|------|------|--------|------|
| 3.1 | 用户查询：GetUserByUsername, GetUserByID, UsernameExists* 等 | ~5 | ✅ |
| 3.2 | 配置查询：GetConfigByName, ListConfigs, UpsertConfig | ~3 | ✅ |
| 3.3 | 公告查询：GetAnnouncement, UpsertAnnouncement | ~2 | ✅ |
| 3.4 | 节点查询：GetNodeBy*, ListNodes, UpdateNode* | ~6 | ✅ |
| 3.5 | 隧道查询：ListTunnels, ListTunnelGroups 等 (含 chain_tunnel 关联) | ~5 | ✅ |
| 3.6 | 转发查询：ListForwards, resolveForwardIngress | ~3 | ✅ |
| 3.7 | 用户隧道：GetUserPackageTunnels, GetUserPackageForwards | ~3 | ✅ |
| 3.8 | 统计/限速：GetStatisticsFlows, ListSpeedLimits, AddFlow | ~4 | ✅ |
| 3.9 | 分组查询：ListUserGroups, ListGroupPermissions 等 | ~4 | ✅ |
| 3.10 | PeerShare 全部方法 (CRUD + Runtime) | ~15 | ✅ |
| 3.11 | FederationTunnelBinding 全部方法 | ~4 | ✅ (Upsert 用 clause.OnConflict) |
| 3.12 | Export 全部方法 | ~10 | ✅ |
| 3.13 | Import 全部方法 | ~10 | ✅ 已全部改为 GORM `Clauses(clause.OnConflict)`（见 §9.6） |
| **3.14** | **repository_mutations.go 全部方法 (~40 个)** | **~40** | **✅ 已全量改为 GORM 链式调用（见 §9.3）** |
| **3.15** | **repository_federation.go 查询方法** | **~8** | **✅ 已全部改为 GORM 链式调用** |
| **3.16** | **repository_control.go 复杂查询** | **~5** | **✅ 已全部改为 GORM 链式调用** |
| **3.17** | **repository_flow.go 查询方法** | **~5** | **✅ 已全部改为 GORM 链式调用** |
| **3.18** | **Jobs 查询方法 (repository.go 尾部)** | **~8** | **✅ 已 GORM 化** |

### 阶段 4：消除 handler 中直接 SQL — 提取为 Repository 方法 ✅ 已完成

> **2026-02-16 审计**：handler 中的 SQL 已大部分提取到 repo 层，但这些 repo 方法本身仍使用 raw SQL（见阶段 3）。
> **2026-02-17 更新**：`mutations.go` 直接 `tx.Exec`/`tx.Raw` 已从 27 处降至 0 处（生产代码），详见 §9.4。

mutations.go 和其他 handler 文件中大量直接操作 `h.repo.DB()` 执行 raw SQL，需要：
1. 将 SQL 逻辑提取为 Repository 方法
2. Handler 只调用 Repository 方法

| 步骤 | 任务 | 文件 | 状态 |
|------|------|------|------|
| 4.1 | 用户 CRUD：userCreate, userUpdate, userDelete, userResetFlow | mutations.go | ✅ 已提取到 repo 方法 |
| 4.2 | 节点 CRUD：nodeCreate, nodeUpdate, nodeDelete, nodeBatch* | mutations.go | ✅ 已提取到 repo 方法 |
| 4.3 | 隧道 CRUD：tunnelCreate, tunnelUpdate, tunnelDelete, tunnelBatch* | mutations.go | ✅ tunnelCreate/Update 的 SQL 已下沉 repo |
| 4.4 | 转发 CRUD：forwardCreate, forwardUpdate, forwardDelete, forwardBatch* | mutations.go | ✅ 已提取到 repo (CreateForwardTx 等) |
| 4.5 | 限速 CRUD：speedLimitCreate, speedLimitUpdate, speedLimitDelete | mutations.go | ✅ 已提取到 repo 方法 |
| 4.6 | 分组 CRUD：所有 group* 方法 | mutations.go | ✅ 成员同步/权限管理 SQL 已下沉 repo |
| 4.7 | 用户隧道：userTunnelAssign, userTunnelRemove, userTunnelUpdate | mutations.go | ✅ 已提取到 repo 方法 |
| 4.8 | handler.go 中的直接 SQL (openAPISubStore 等) | handler.go | ✅ 已迁移（含 nil 检查清理） |
| 4.9 | federation.go 中的 raw SQL | federation.go | ✅ 已提取到 repo_federation.go |
| 4.10 | control_plane.go 中的 raw SQL | control_plane.go | ✅ 已提取到 repo_control.go |
| 4.11 | flow_policy.go 中的 raw SQL | flow_policy.go | ✅ 已提取到 repo_flow.go |
| 4.12 | jobs.go 中的 raw SQL | jobs.go | ✅ 已提取到 repo 方法（含 nil 检查清理） |

### 阶段 5：清理旧代码 ✅ 已完成

| 步骤 | 任务 | 文件 | 状态 |
|------|------|------|------|
| 5.1 | 删除 `internal/store/postgres/` 整个目录 | 目录删除 | ✅ |
| 5.2 | 删除 `internal/store/sqlite/sql/` 目录 | 目录删除 | ✅ |
| 5.3 | 删除 `internal/store/db.go` SQL 重写层 | 文件删除 | ✅ |
| 5.4 | 删除 `internal/store/db_test.go` | 文件删除 | ✅ |
| 5.5 | 清理 repository.go 中不再需要的 embed 指令 | 清理 | ✅ |

### 阶段 6：Package 重命名 ✅ 已完成

| 步骤 | 任务 | 文件 | 状态 |
|------|------|------|------|
| 6.1 | `internal/store/sqlite/` → `internal/store/repo/` | 目录重命名 | ✅ |
| 6.2 | 更新所有 import 路径：`store/sqlite` → `store/repo` (13处) | 全局替换 | ✅ |

### 阶段 7：测试 + 验证 ⚠️ 部分完成

| 步骤 | 任务 | 状态 |
|------|------|------|
| 7.1 | 更新所有现有测试适配 GORM | ✅ 测试已适配 (使用 repo.DB() 做数据准备) |
| 7.2 | `go test ./...` 全部通过 | ✅ 已通过（含 `internal/http/handler`、`tests/contract`） |
| 7.3 | `make build` 构建成功 | ✅ 已通过 |

### 阶段 8：文档更新 ✅ 已完成

| 步骤 | 任务 | 文件 | 状态 |
|------|------|------|------|
| 8.1 | 更新 `go-backend/AGENTS.md` — 移除 "DO NOT USE ORM"，记录 GORM 规范 | AGENTS.md | ✅ |
| 8.2 | 更新根 `AGENTS.md` | AGENTS.md | ✅ |
| 8.3 | 更新 `handler/AGENTS.md` | AGENTS.md | ✅ |

---

## 五、GORM 使用规范

### 5.1 查询模式

```go
// 单条查询 - 未找到返回 nil, nil (保持现有语义)
var user model.User
err := r.db.Where("id = ?", id).First(&user).Error
if errors.Is(err, gorm.ErrRecordNotFound) {
    return nil, nil
}

// 列表查询
var users []model.User
err := r.db.Where("role_id != ?", 0).Order("id ASC").Find(&users).Error

// 创建
err := r.db.Create(&user).Error

// 更新 (部分字段)
err := r.db.Model(&model.User{}).Where("id = ?", id).Updates(map[string]interface{}{
    "user": username, "flow": flow, "updated_time": now,
}).Error

// 事务 (closure pattern - 自动 rollback/commit)
err := r.db.Transaction(func(tx *gorm.DB) error {
    if err := tx.Where("user_id = ?", id).Delete(&model.Forward{}).Error; err != nil {
        return err
    }
    return tx.Where("id = ?", id).Delete(&model.User{}).Error
})

// 原生 SQL (仅用于复杂查询和 PG 特有操作)
r.db.Exec("SELECT setval(?::regclass, ?, ?)", seqRef, maxID, true)
```

### 5.2 关键注意事项

1. **user 保留字**：通过 `TableName()` 返回 `"user"`，GORM 自动处理引号
2. **SQLite MaxOpenConns**：必须设为 1 防止 "database locked"
3. **SQLite WAL 模式**：DSN 中配置 `_pragma=journal_mode(WAL)`
4. **不要用 `type:jsonb`**：SQLite 不支持，用 `serializer:json`
5. **不要用 `type:serial`**：让 GORM 从 `primaryKey` 自动推断
6. **AutoMigrate 在 SQLite 中使用 copy-swap-drop**：大表慎用

---

## 六、影响范围

### 需要修改的文件

| 文件 | 修改类型 | 描述 | 当前状态 |
|------|----------|------|----------|
| `go.mod` / `go.sum` | 修改 | 添加 GORM + 驱动依赖 | ✅ |
| `internal/store/model/model.go` | **新增** | 全部 21 个 GORM Model | ✅ |
| `internal/store/repo/repository.go` | **重写** | 全部查询 GORM 化 | ⚠️ 业务查询已 GORM；仅剩 PG 序列修复 DDL `Exec` 4 处 |
| `internal/store/repo/repository_mutations.go` | **重写** | Mutation helpers | ✅ 全量 GORM（Raw=0） |
| `internal/store/repo/repository_federation.go` | **重写** | Federation 查询 | ✅ 已 GORM 化（Raw=0） |
| `internal/store/repo/repository_control.go` | **重写** | 控制面查询 | ✅ 已 GORM 化（Raw=0） |
| `internal/store/repo/repository_flow.go` | **重写** | 流量/转发查询 | ✅ 已 GORM 化（Raw=0） |
| `internal/http/handler/mutations.go` | **重写** | 全部 CRUD 提取到 repo | ✅ 生产代码 `tx.Exec/tx.Raw` = 0 |
| `internal/http/handler/handler.go` | 修改 | 更新 import、移除直接 SQL | ✅ (仅剩 nil check) |
| `internal/http/handler/federation.go` | 修改 | GORM 替代 raw SQL | ✅ |
| `internal/http/handler/control_plane.go` | 修改 | GORM 替代 raw SQL | ✅ |
| `internal/http/handler/flow_policy.go` | 修改 | GORM 替代 raw SQL | ✅ |
| `internal/http/handler/jobs.go` | 修改 | GORM 替代 raw SQL | ✅ (仅剩 nil check) |
| `internal/ws/server.go` | 修改 | 更新 import | ✅ |
| `internal/app/app.go` | 修改 | 更新 import | ✅ |
| `internal/store/postgres/` | **删除** | 不再需要 | ✅ |
| `internal/store/db.go` | **删除** | GORM 自动处理方言 | ✅ |
| `internal/store/db_test.go` | **删除** | 旧重写层测试 | ✅ |
| `internal/store/sqlite/sql/` | **删除** | AutoMigrate 替代 | ✅ |
| `tests/contract/*.go` | 修改 | 适配 GORM | ✅ |
| `AGENTS.md` (3处) | 更新 | 反映新架构 | ✅ |

### 不需要修改的文件

- `internal/http/router.go` — 路由不变
- `internal/config/config.go` — 配置不变
- `internal/auth/` — 认证不变
- `internal/security/` — 加密不变
- `internal/http/middleware/` — 中间件不变
- `internal/http/response/` — 响应格式不变
- `Dockerfile`, `Makefile` — 构建不变

---

## 七、风险与缓解

| 风险 | 可能性 | 影响 | 缓解措施 |
|------|--------|------|----------|
| GORM AutoMigrate SQLite/PG 行为差异 | 中 | 高 | 先写 Model 验证双数据库 AutoMigrate |
| handler 中散落 raw SQL 遗漏 | 中 | 高 | 全局搜索 `.Exec(`, `.Query(`, `.QueryRow(` |
| 事务语义变化 | 低 | 中 | 逐方法对比旧代码事务边界 |
| 大量代码变更导致回归 | 高 | 高 | 分阶段提交，每阶段 `go test` |
| GORM 性能开销 | 低 | 低 | 此场景下可忽略 |
| SQLite "database locked" | 中 | 高 | `MaxOpenConns(1)` + WAL 模式 |

---

## 八、迁移顺序原则

1. **先 Model 后查询**：确保 AutoMigrate 双数据库通过
2. **先 Repository 后 Handler**：Handler 依赖 Repository
3. **先核心后边缘**：User → Node → Tunnel → Forward → 分组 → Federation
4. **每步编译**：每完成一组方法确保 `go build ./...` 通过
5. **最后清理**：全部重写完成后再删除旧代码和重命名 package

---

---

## 九、2026-02-16 审计发现 + 2026-02-17 进展记录

### 9.1 总体完成度

| 指标 | 数值 |
|------|------|
| 阶段完成数 | 7/8 完成 (1, 2, 4, 5, 6, 7, 8)，1/8 部分完成 (3) |
| GORM 链式调用 | ~226 处 |
| Raw SQL 调用 (`.Exec`/`.Raw`+`.Scan`) | 4 处（生产代码） |
| GORM 占比 | ~98% |
| Handler 内 `tx.Exec`/`tx.Raw` | 0 处（生产代码） |
| `last_insert_rowid()` 生产代码 | 0 处（已消灭） |

### 9.2 ✅ P0：`last_insert_rowid()`（生产代码）已清零

`last_insert_rowid()` 已从生产路径移除，创建主键统一改为 `Create(&model)` 自动回填 ID，
确保 SQLite / PostgreSQL 双数据库行为一致。

> 备注：测试代码中的历史 SQL 兼容性用例可在后续测试清理阶段单独处理。

### 9.3 ✅ P1：`repository_mutations.go` 已全量 GORM 化

本次已完成 `repository_mutations.go` 的集中清理：

1. User / Node / Tunnel / Forward / UserTunnel / SpeedLimit / Group / Permission 全部 mutation 方法改为 GORM 链式调用。
2. 事务内级联删除统一为 `tx.Where(...).Delete(&Model{})` 模式。
3. `ON CONFLICT DO NOTHING` 统一替换为 `Clauses(clause.OnConflict{DoNothing: true})`。
4. 保留原有调用语义（含 `sql.ErrNoRows` 行为兼容）并完成 `go build ./...` 验证。

> 当前 `repository_mutations.go` 中生产代码 `.Raw(`/`.Exec(` 调用已降为 0。

### 9.4 ✅ P2：Handler `mutations.go` 直接 SQL 已清零

2026-02-17 本轮静态扫描结果：`mutations.go` **0 处** `tx.Exec`/`tx.Raw`（生产代码）。

本轮完成下沉到 repo 的逻辑：

- `tunnelUpdate` 中 `UPDATE tunnel` + `DELETE chain_tunnel`
- `isRemoteNodeTx` 查询
- `pickNodePortTx` 的 node/chain_tunnel/forward_port 端口占用查询
- `replaceTunnelChainsTx` 的 chain_tunnel 写入
- 分组成员同步（`tunnel_group_tunnel` / `user_group_user`）
- 权限删除与 grant 回收（`group_permission` / `group_permission_grant` / `user_tunnel`）
- federation 绑定替换（`federation_tunnel_binding`）

### 9.5 ✅ P3（部分）：已移除 `QueryInt64List` / `QueryPairs` SQL 透传

- `repository_mutations.go` 中两个 SQL 透传入口已删除。
- Handler 已切换为语义化 repo 方法：
  - `ListUserIDsByUserGroup`
  - `ListTunnelIDsByTunnelGroup`
  - `ListGroupPermissionPairsByUserGroup`
  - `ListGroupPermissionPairsByTunnelGroup`

### 9.6 ✅ P3：Import 函数已全部 GORM 化

`repository.go` 中 Import 相关函数已完成迁移：

- `importUsers`
- `importNodes`
- `importTunnels`（含 `chain_tunnel` 子项 upsert）
- `importForwards`（含 `forward_port` 覆盖写入）
- `importUserTunnels`
- `importSpeedLimits`
- `importTunnelGroups`
- `importUserGroups`
- `importPermissions`
- `importConfigs`（原本已是 GORM）

迁移后统一采用 `Clauses(clause.OnConflict{Columns: id/name, DoUpdates: ...}).Create(&model)` 模式，
保留原 `ON CONFLICT ... DO UPDATE` 语义；Import 区段 `tx.Exec`/`tx.Raw` 已清零。

### 9.7 ✅ P4：`h.repo.DB() == nil` 检查已清理

`internal/http/handler/` 下已无 `h.repo.DB()` 直接访问；handler 仅通过语义化 repo 方法进行数据访问。

### 9.8 ✅ P5：Repository 层 Raw 已收敛（仅保留 PG 序列修复 DDL）

当前生产代码中 `.Raw()` 已清零；仅剩 `repository.go` 的 4 处 `Exec()`，全部位于 PG 序列修复 DDL：

- `CREATE SEQUENCE IF NOT EXISTS ...`
- `ALTER TABLE ... ALTER COLUMN id SET DEFAULT nextval(...)`
- `ALTER SEQUENCE ... OWNED BY ...`
- `SELECT setval(...::regclass, ?, ?)`

以上 4 处属于数据库管理 DDL/序列同步语义，当前保留，不再继续向 GORM 链式调用替换。

`repository_federation.go` / `repository_control.go` / `repository_flow.go` 已完成 GORM 化（Raw=0）。

---

## 十、后续工作优先级

| 优先级 | 任务 | 影响范围 | 工作量 |
|--------|------|----------|--------|
| **P0** | ✅ 已完成：生产代码中 `last_insert_rowid()` 清零（测试用例待单独清理） | 6 处生产（已完成） | 完成 |
| **P1** | ✅ 已完成：`repository_mutations.go` ~40 方法改为 GORM 链式调用 | 659 行（已完成） | 完成 |
| **P2** | ✅ 已完成：`mutations.go` handler 直接 SQL 全部提取为 repo 方法 | mutations.go | 完成 |
| **P3** | ✅ 已完成：移除 `QueryInt64List`/`QueryPairs` 透传，切换语义化 repo 方法 | 2 个方法 + 调用方（已完成） | 完成 |
| **P3** | ✅ 已完成：Import 函数 Raw SQL 改为 GORM `Clauses(clause.OnConflict{}).Create()` | 9 个函数（已完成） | 完成 |
| **P4** | ✅ 已完成：`h.repo.DB() == nil` 检查清理完毕 | 4 处（已完成） | 完成 |
| **P5** | ✅ 已完成：repo 查询层 Raw 清零，`repository.go` 保留 4 处 PG 序列修复 DDL `Exec`（设计保留） | repository.go | 完成 |
| **P5** | ✅ 已完成：更新 MIGRATION_PLAN.md 状态标记与收尾记录 | 本文件 | 完成 |

### 10.5 本轮执行记录（2026-02-17，P5 schema 收尾）

1. 完成 `repository.go` schema 迁移段去 Raw：
   - `normalizeStrategy` 改为 `Model(...).Where(...).Update(...)`
   - `ensurePostgresIDDefaults`/`ensurePostgresTableIDDefault` 的 information_schema 查询改为 GORM `Table+Joins+Where+Scan`
   - `syncPostgresTableIDSequence` 的 `MAX(id)` 查询改为 GORM `Table+Select+Scan`
2. 复扫结果：
   - `repository.go` `.Raw()` = 0
   - repo 生产路径剩余 `.Exec()` = 4（全部为 PG 序列修复 DDL）
3. 验证结果：
   - `go build ./...` ✅
   - `go test ./internal/store/repo/...` ✅

### 10.6 本轮执行记录（2026-02-17，测试/构建收尾）

1. 修复事务内 SQLite 连接阻塞（`MaxOpenConns(1)` 场景）：
   - 新增 `GetNodeRecordTx` 并在 `prepareTunnelCreateState` 使用事务句柄读取节点。
   - 新增 `GetNodeRemoteFieldsTx` 并在 `tunnelCreate` 事务内改用事务句柄读取远端字段。
   - `applyFederationRuntime` 改为显式接收 `localDomain`，避免事务内再次走 `repo.GetConfigByName`。
2. 修复 legacy SQLite schema 迁移契约：
   - 新增 `prepareSQLiteLegacyColumns` 预补齐 `node/tunnel` 关键列。
   - SQLite 模式下对已存在 `node/tunnel` 表跳过对应 `AutoMigrate` 重建流程，避免 `node__temp.name` 约束失败。
3. 验证结果：
   - `go test ./internal/http/handler/...` ✅
   - `go test ./tests/contract/...` ✅
   - `go test ./...` ✅
   - `go build ./...` ✅
   - `make build` ✅

### 10.1 本轮执行记录（2026-02-17，P5 查询层）

1. 完成 `repository_federation.go` 全量 GORM 化：
   - `ListRemoteNodes` / `UpdateNodeRemoteConfig`
   - `ListActiveBindingsForNode` / `GetNodeBasicInfo`
   - `ListUsedPortsOnNode` / `ListTunnelIDsByNamePrefix` / `NextIndex`
2. 完成 `repository_control.go` 全量 GORM 化：
   - `ListForwardsByTunnel` / `ListForwardPorts` / `GetTunnelOutProtocol`
   - `ResolveUserTunnelAndLimiter` / `ListChainNodesForTunnel`
3. 完成 `repository_flow.go` 全量 GORM 化：
   - `ListActiveForwardsByUser` / `ListActiveForwardsByUserTunnel`
   - `GetForwardRecord` / `GetTunnelRecord`
4. 复扫结果：
   - `repository_federation.go` Raw/Exec = 0
   - `repository_control.go` Raw/Exec = 0
   - `repository_flow.go` Raw/Exec = 0
   - repo 生产路径剩余 Raw/Exec = 9（全部在 `repository.go`）
5. 验证结果：
   - `go build ./...` ✅
   - `go test ./internal/store/repo/...` ✅

### 10.2 本轮执行记录（2026-02-17）

1. 完成 P3 Import 9 个函数的 GORM 化（`repository.go`），并保持 `ON CONFLICT` 语义一致。
2. 复扫确认：`repository.go` Import 区段 `tx.Exec`/`tx.Raw` 已清零。
3. 验证结果：
   - `go build ./...` ✅（使用显式 `GOMODCACHE/GOPATH/GOCACHE/HOME` 环境）
   - `go test ./internal/store/repo/...` ✅

### 10.3 本轮执行记录（2026-02-17，P2 部分）

1. 将 tunnel 更新/chain 重建路径 SQL 下沉到 `repository_mutations.go`：
   - 新增 `UpdateTunnelTx`
   - 新增 `DeleteChainTunnelsByTunnelTx`
   - 新增 `CreateChainTunnelTx`
2. 将 handler 内部 SQL helper 迁移到 repo：
   - 新增 `IsRemoteNodeTx`
   - 新增 `PickNodePortTx`
   - `replaceTunnelChainsTx` 改为 handler 方法并改用 repo 调用，不再直接 SQL
3. 复扫结果：`mutations.go` 直接 SQL 从 27 处降至 17 处。
4. 验证结果：
   - `go build ./...` ✅
   - `go test ./internal/store/repo/...` ✅

### 10.4 本轮执行记录（2026-02-17，P2 收尾）

1. 新增并落地事务语义化 repo 方法：
   - `ReplaceTunnelGroupMembersTx` / `ReplaceUserGroupMembersTx`
   - `ListUserIDsByUserGroupTx`
   - `GetGroupPermissionPairByIDTx` / `DeleteGroupPermissionByIDTx`
   - `RevokeGroupGrantsForRemovedUsersTx` / `RevokeGroupPermissionPairTx`
   - `ReplaceFederationTunnelBindingsTx`
2. 删除 handler 内 SQL helper（`queryInt64ListTx` / `revokeGroupGrantsForRemovedUsersTx` / `revokeGroupPermissionPairTx` / `replaceFederationTunnelBindingsTx`）。
3. 复扫确认：`mutations.go` 生产路径 `tx.Exec`/`tx.Raw` = 0。
4. 验证结果：
   - `go build ./...` ✅
   - `go test ./internal/store/repo/...` ✅

---

*本文档将随迁移进展实时更新状态标记。*
*最后审计时间：2026-02-17，审计工具：代码静态分析 (grep/AST) + go build/go test 验证*
