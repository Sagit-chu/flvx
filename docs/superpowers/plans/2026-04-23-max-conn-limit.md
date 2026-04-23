# 最大连接数限制实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 FLVX 中实现基于用户的全局最大连接数限制和基于单条规则的独立最大连接数限制功能。前端输入框为 0 或空时表示不限制。

**Architecture:** 采用“覆盖逻辑”（方案二）。
1. 数据库层面：在 `user` 和 `forward` 表中各增加一个整型字段 `max_conn`，默认值为 0（表示不限制）。
2. 后端接口层面：提供 API 更新该字段，在组装下发给 GOST 的配置时，判断规则的 `max_conn` 是否大于 0：
   - 如果规则 `max_conn > 0`，则为此规则动态生成一个唯一的连接限制器配置，并在下发服务的 `climiter` 字段中引用该限制器。
   - 如果规则 `max_conn == 0`，则检查该规则所属用户的 `max_conn`。
   - 如果用户 `max_conn > 0`，则引用以用户维度的连接限制器配置（如 `user_conn_limit_<user_id>`）。
   - 否则不下发 `climiter`。
3. 后端服务控制平面：需要在下发服务前，将需要的连接限制器（Rule 或 User 维度）推送到节点上。
   - **重要发现：** 当前 `go-gost` 的 WebSocket Reporter (`go-gost/x/socket/websocket_reporter.go`) 仅支持 `TrafficLimiter` 的动态增删（如 `AddLimiters` 等），**不支持** `ConnLimiter`（即 `CLimiters`）。
   - **计划修改：** 我们需要先在 `go-gost` 侧（`go-gost/x/socket`）添加针对 `CLimiters` 的 WebSocket 指令（`AddCLimiters`, `UpdateCLimiters`, `DeleteCLimiters`）以及对应的处理函数（参考 `AddLimiters` 等的实现，调用现有的针对 `ConnLimiterRegistry` 的相关接口和配置存储逻辑，具体需要实现类似 `createLimiter` 到 `createConnLimiter` 的逻辑）。
   - 完成底层修改后，`go-backend` 再通过这些新增加的 WebSocket 指令，在 `ensureLimiterOnNode` 时下发最大连接数限制规则。
4. 前端层面：在用户管理和规则管理页面增加输入框组件。

**Tech Stack:** Go, GORM, SQLite/PostgreSQL, React, Vite, TypeScript, TailwindCSS.

---

### Task 1: 扩展 go-gost WebSocket 接口以支持 CLimiters

**Files:**
- Modify: `go-gost/x/socket/limiter.go`
- Modify: `go-gost/x/socket/websocket_reporter.go`

- [ ] **Step 1: 实现 `createConnLimiter` 等功能**

在 `go-gost/x/socket/limiter.go` 中参考现有 `createLimiter` 添加对 `CLimiters` 的支持：

```go
func createConnLimiter(req createLimiterRequest) error {
	name := strings.TrimSpace(req.Data.Name)
	if name == "" {
		return errors.New("limiter name is required")
	}
	req.Data.Name = name

	if registry.ConnLimiterRegistry().IsRegistered(name) {
		return errors.New("conn limiter " + name + " already exists")
	}

	v := parser.ParseConnLimiter(&req.Data)

	if err := registry.ConnLimiterRegistry().Register(name, v); err != nil {
		return errors.New("conn limiter " + name + " already exists")
	}

	if c := config.Global(); c != nil {
		c.CLimiters = append(c.CLimiters, &req.Data)
	}
	return nil
}

func updateConnLimiter(req updateLimiterRequest) error {
	name := strings.TrimSpace(req.Limiter)
	req.Data.Name = name
	if registry.ConnLimiterRegistry().IsRegistered(name) {
		registry.ConnLimiterRegistry().Unregister(name)
	}

	v := parser.ParseConnLimiter(&req.Data)

	if err := registry.ConnLimiterRegistry().Register(name, v); err != nil {
		return errors.New("conn limiter " + name + " already exists")
	}

	if c := config.Global(); c != nil {
		for i := range c.CLimiters {
			if c.CLimiters[i].Name == name {
				c.CLimiters[i] = &req.Data
				return nil
			}
		}
		c.CLimiters = append(c.CLimiters, &req.Data)
	}
	return nil
}

func deleteConnLimiter(req deleteLimiterRequest) error {
	name := strings.TrimSpace(req.Limiter)

	if registry.ConnLimiterRegistry().IsRegistered(name) {
		registry.ConnLimiterRegistry().Unregister(name)
	}

	if c := config.Global(); c != nil {
		limiteres := c.CLimiters
		c.CLimiters = nil
		for _, s := range limiteres {
			if s.Name == name {
				continue
			}
			c.CLimiters = append(c.CLimiters, s)
		}
	}
	return nil
}
```

- [ ] **Step 2: 在 `WebSocketReporter` 注册命令**

在 `go-gost/x/socket/websocket_reporter.go` 的 `ProcessCommand` 中添加 case：

```go
	case "AddCLimiters":
		err = w.handleAddCLimiter(cmd.Data)
		response.Type = "AddCLimitersResponse"
		needSaveConfig = true
	case "UpdateCLimiters":
		err = w.handleUpdateCLimiter(cmd.Data)
		response.Type = "UpdateCLimitersResponse"
		needSaveConfig = true
	case "DeleteCLimiters":
		err = w.handleDeleteCLimiter(cmd.Data)
		response.Type = "DeleteCLimitersResponse"
		needSaveConfig = true
```

- [ ] **Step 3: 实现 Handler 方法**

在 `go-gost/x/socket/websocket_reporter.go` 中添加：

```go
func (w *WebSocketReporter) handleAddCLimiter(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化数据失败: %v", err)
	}

	var limiterConfig config.LimiterConfig
	if err := json.Unmarshal(jsonData, &limiterConfig); err != nil {
		return fmt.Errorf("解析限流器配置失败: %v", err)
	}

	req := createLimiterRequest{Data: limiterConfig}
	return createConnLimiter(req)
}

func (w *WebSocketReporter) handleUpdateCLimiter(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化数据失败: %v", err)
	}

	var updateReq struct {
		Limiter string               `json:"limiter"`
		Data    config.LimiterConfig `json:"data"`
	}

	if err := json.Unmarshal(jsonData, &updateReq); err != nil {
		var limiterConfig config.LimiterConfig
		if err := json.Unmarshal(jsonData, &limiterConfig); err != nil {
			return fmt.Errorf("解析更新请求失败: %v", err)
		}
		updateReq.Limiter = limiterConfig.Name
		updateReq.Data = limiterConfig
	}

	req := updateLimiterRequest{
		Limiter: updateReq.Limiter,
		Data:    updateReq.Data,
	}
	return updateConnLimiter(req)
}

func (w *WebSocketReporter) handleDeleteCLimiter(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化数据失败: %v", err)
	}

	var deleteReq deleteLimiterRequest

	if err := json.Unmarshal(jsonData, &deleteReq); err != nil {
		var limiterName string
		if err := json.Unmarshal(jsonData, &limiterName); err != nil {
			return fmt.Errorf("解析删除请求失败: %v", err)
		}
		deleteReq.Limiter = limiterName
	}

	return deleteConnLimiter(deleteReq)
}
```

- [ ] **Step 4: Commit**

```bash
cd go-gost
git add x/socket/limiter.go x/socket/websocket_reporter.go
git commit -m "feat: add CLimiters support for websocket reporter"
cd ..
```

---

### Task 2: 数据库迁移与模型更新

**Files:**
- Modify: `go-backend/internal/store/model/model.go`
- Modify: `go-backend/internal/store/repo/repository.go`

- [ ] **Step 1: 更新数据库模型**

在 `go-backend/internal/store/model/model.go` 的 `User` 和 `Forward` 结构体中添加 `MaxConn` 字段。

```go
// 在 User 结构体中
type User struct {
	// ...
	MaxConn       int           `gorm:"column:max_conn;not null;default:0"`
	// ...
}

// 在 Forward 结构体中
type Forward struct {
	// ...
	MaxConn       int           `gorm:"column:max_conn;not null;default:0"`
	// ...
}
```

- [ ] **Step 2: 编写数据库迁移**

在 `go-backend/internal/store/repo/repository.go` 的 `AutoMigrate` 逻辑前（如果有自定义迁移）或利用 gorm 自动迁移机制，由于这是 autoMigrate，添加字段只要 `db.AutoMigrate(&model.User{}, &model.Forward{})` 被调用就能自动加上。确认已执行迁移。由于 `FLVX` 通常会自动执行迁移，只需修改模型即可。我们需要处理默认值，由于使用了 `default:0`，GORM 会处理新增字段的默认值，但为了安全起见，如果在旧环境中，可能直接 alter table。

```go
// 无需手动编写 SQL，依赖现有的 gorm AutoMigrate 即可。
```

- [ ] **Step 3: 运行并验证迁移通过**

Run: `make build` (在 go-backend 中)，或者运行一个相关的存储单元测试。

- [ ] **Step 4: Commit**

```bash
cd go-backend
git add internal/store/model/model.go
git commit -m "feat: add max_conn field to user and forward models"
cd ..
```

---

### Task 3: 后端控制平面 - 连接数限制器的组装与下发

**Files:**
- Modify: `go-backend/internal/http/handler/control_plane.go`
- Modify: `go-backend/internal/store/repo/repository_control.go`

- [ ] **Step 1: 更新存储层以获取 User 的 MaxConn**

在 `go-backend/internal/store/repo/repository_control.go` 中：

需要一个方法获取 User，或者如果已经有，确保可以拿到 `MaxConn`。

- [ ] **Step 2: 编写下发 CLimiter 到节点的辅助函数**

在 `go-backend/internal/http/handler/control_plane.go`，参考 `ensureLimiterOnNode` 和 `upsertLimiterOnNode`：

```go
func (h *Handler) ensureConnLimiterOnNode(nodeID int64, limiterName string, maxConn int) error {
	limitStr := fmt.Sprintf("$ %d", maxConn)
	
	payload := map[string]interface{}{
		"name":   limiterName,
		"limits": []string{limitStr},
	}
	
	if _, err := h.sendNodeCommand(nodeID, "AddCLimiters", payload, false, false); err != nil {
		if !isAlreadyExistsMessage(err.Error()) {
			return fmt.Errorf("连接限制器下发失败: %w", err)
		}
		updatePayload := map[string]interface{}{
			"limiter": limiterName,
			"data":    payload,
		}
		if _, updateErr := h.sendNodeCommand(nodeID, "UpdateCLimiters", updatePayload, false, false); updateErr != nil {
			return fmt.Errorf("连接限制器更新失败: %w", updateErr)
		}
	}
	return nil
}
```

- [ ] **Step 3: 更新组装配置逻辑以绑定 `climiter`**

在 `control_plane.go` 的 `syncForwardServicesWithWarnings` 及其辅助函数 `buildForwardServiceConfigs` 附近：

修改 `buildForwardServiceConfigs` 的签名，传入 `maxConn int` 和对应的 `cLimiterName string`。

```go
func buildForwardServiceConfigs(baseName string, forward *model.Forward, tunnel *model.Tunnel, node *model.Node, port int, bindIP string, limiterID *int64, cLimiterName string) []map[string]interface{} {
	// ... 现有逻辑
	// 在服务配置生成的部分增加：
	if cLimiterName != "" {
		service["climiter"] = cLimiterName
	}
	// ...
}
```

- [ ] **Step 4: 在转发服务同步主流程中决定并下发 `climiter`**

在 `syncForwardServicesWithWarnings` (可能在多个重载/处理入口处，如 `ensureForwardServices`)，查出转发所属 user 的 `MaxConn`，以及转发本身的 `MaxConn`。

```go
	// 获取 User
	user, err := h.repo.GetUser(forward.UserID)
	if err != nil {
		return nil, err
	}

	var cLimiterName string
	var maxConnToSet int

	if forward.MaxConn > 0 {
		maxConnToSet = forward.MaxConn
		cLimiterName = fmt.Sprintf("rule_conn_limit_%d", forward.ID)
	} else if user != nil && user.MaxConn > 0 {
		maxConnToSet = user.MaxConn
		cLimiterName = fmt.Sprintf("user_conn_limit_%d", user.ID)
	}

	if cLimiterName != "" {
		for _, fp := range ports {
			if err := h.ensureConnLimiterOnNode(fp.NodeID, cLimiterName, maxConnToSet); err != nil {
				warnings = append(warnings, fmt.Sprintf("节点 %d 连接限制器下发失败: %v", fp.NodeID, err))
			}
		}
	}

	// 传递给 buildForwardServiceConfigs
	// ...
```
*(注意：需要确保更新涉及 `buildForwardServiceConfigs` 的所有调用点)*

- [ ] **Step 5: Commit**

```bash
cd go-backend
git add internal/http/handler/control_plane.go internal/store/repo/repository_control.go
git commit -m "feat: implement max conn limiter dispatching"
cd ..
```

---

### Task 4: 后端接口 - 用户和规则的 CRUD 支持

**Files:**
- Modify: `go-backend/internal/http/handler/admin_user.go`
- Modify: `go-backend/internal/http/handler/forward.go`

- [ ] **Step 1: 用户接口更新**

在 `go-backend/internal/http/handler/admin_user.go`，修改用户创建和更新请求的结构体（如果有），接收 `MaxConn`，并在保存到数据库时赋值。

```go
type CreateUserReq struct {
	// ...
	MaxConn *int `json:"maxConn"`
}
// 接收后：
if req.MaxConn != nil {
	user.MaxConn = *req.MaxConn
}
```

在获取用户列表时，确保 `MaxConn` 返回给前端。

- [ ] **Step 2: 规则接口更新**

在 `go-backend/internal/http/handler/forward.go` 中，更新 `CreateForwardReq` 和 `UpdateForwardReq` 结构体，增加 `MaxConn`，并在创建/更新 Forward 时保存到数据库。

如果转发规则的 `MaxConn` 或相关信息改变，触发节点上的规则重载（重新下发服务）。这一步由于更改了数据库，复用现有的 `syncForwardServices` 就会带上最新的配置。

- [ ] **Step 3: 测试接口**

Run: 可以启动后使用 curl 测试。

- [ ] **Step 4: Commit**

```bash
cd go-backend
git add internal/http/handler/admin_user.go internal/http/handler/forward.go
git commit -m "feat: add maxConn to user and forward CRUD API"
cd ..
```

---

### Task 5: 前端 - 用户管理页面集成

**Files:**
- Modify: `vite-frontend/src/api/types.ts`
- Modify: `vite-frontend/src/api/index.ts`
- Modify: `vite-frontend/src/pages/users.tsx` (或者对应的用户管理页面文件)

- [ ] **Step 1: 类型更新**

在 `vite-frontend/src/api/types.ts` 中：
为 `UserApiItem` 和相关的 mutation payload 增加 `maxConn?: number` 属性。

- [ ] **Step 2: UI 修改**

在用户创建/编辑弹窗中，增加“最大连接数”输入框：
(假设使用 `@nextui-org/react` 的 `Input`)

```tsx
<Input
  type="number"
  label="最大并发连接数"
  placeholder="0 或空表示不限制"
  value={formData.maxConn === 0 ? "" : String(formData.maxConn || "")}
  onValueChange={(val) => {
	const num = parseInt(val, 10);
	setFormData({ ...formData, maxConn: isNaN(num) ? 0 : num });
  }}
/>
```
并在用户的表格列中展示 `最大连接数`（值为 0 显示“不限制”）。

- [ ] **Step 3: 运行 Vite 进行验证**

- [ ] **Step 4: Commit**

```bash
cd vite-frontend
git add src/api/types.ts src/api/index.ts src/pages/users.tsx
git commit -m "feat: add max conn UI to user management"
cd ..
```

---

### Task 6: 前端 - 转发规则页面集成

**Files:**
- Modify: `vite-frontend/src/pages/forward.tsx`

- [ ] **Step 1: 类型更新**

在 `api/types.ts` 中 `ForwardMutationPayload` 和 `ForwardApiItem` 中增加 `maxConn?: number`。

- [ ] **Step 2: UI 修改**

在 `vite-frontend/src/pages/forward.tsx` 的创建/编辑规则弹窗（在 "规则限速" 附近）增加“最大连接数”输入框：

```tsx
<Input
  type="number"
  label="最大并发连接数"
  placeholder="0 或空表示不限制"
  value={formData.maxConn === 0 ? "" : String(formData.maxConn || "")}
  onValueChange={(val) => {
	const num = parseInt(val, 10);
	setFormData({ ...formData, maxConn: isNaN(num) ? 0 : num });
  }}
  description="此设置优先于用户的全局连接数限制。0 表示不限制（或使用用户的全局限制）。"
/>
```

如果是在列表/卡片中展示，可以增加一个小标签或者 Tooltip 显示其最大连接数设置。

- [ ] **Step 3: 验证**

在前端验证该功能能正确读写规则的连接限制字段。

- [ ] **Step 4: Commit**

```bash
cd vite-frontend
git add src/pages/forward.tsx src/api/types.ts
git commit -m "feat: add max conn UI to forward rules"
cd ..
```