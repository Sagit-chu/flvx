# Forward Flow Reset Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a permission-checked action that resets only one forward rule's displayed upload and download counters.

**Architecture:** A dedicated repository method updates only the selected `forward` row. A dedicated authenticated handler reuses `resolveForwardAccess`, and the React page calls the endpoint from all three rule views through one confirmation modal.

**Tech Stack:** Go `net/http`, GORM, SQLite/PostgreSQL-compatible models, React, TypeScript, shadcn bridge components, Tailwind CSS v4.

## Global Constraints

- Only `forward.in_flow`, `forward.out_flow`, and `forward.updated_time` may change during reset.
- Do not modify `user`, `user_tunnel`, quota, historical statistics, nftables counter state, or running services.
- Administrators may reset any rule; non-admin users may reset only their own rules through existing `resolveForwardAccess` behavior.
- All API responses must keep the `{code, msg, data, ts}` envelope.
- Frontend imports must use `src/shadcn-bridge/heroui/*`; do not add `@heroui/*` or `@nextui-org/*` dependencies.
- Do not add frontend test infrastructure.
- Do not edit generated protobuf files, `install.sh`, or `panel_install.sh`.

---

### Task 1: Add the repository flow-reset primitive

**Files:**
- Create: `go-backend/internal/store/repo/repository_forward_flow_reset_test.go`
- Modify: `go-backend/internal/store/repo/repository_mutations.go`

**Interfaces:**
- Consumes: `model.Forward`, the repository's GORM database handle, and an explicit Unix-millisecond timestamp.
- Produces: `func (r *Repository) ResetForwardFlow(forwardID int64, now int64) error`.

- [ ] **Step 1: Write the failing repository tests**

Create `go-backend/internal/store/repo/repository_forward_flow_reset_test.go`:

```go
package repo

import (
    "path/filepath"
    "testing"
)

func TestResetForwardFlowOnlyUpdatesSelectedForward(t *testing.T) {
    r, err := Open(filepath.Join(t.TempDir(), "forward-flow-reset.db"))
    if err != nil {
        t.Fatalf("open repo: %v", err)
    }
    defer r.Close()

    const originalUpdated int64 = 1000
    if err := r.DB().Exec(`
        INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status)
        VALUES(2, 'owner', 'pwd', 1, 0, 100, 700, 900, 0, 10, 1000, 1000, 1)
    `).Error; err != nil {
        t.Fatalf("insert user: %v", err)
    }
    if err := r.DB().Exec(`
        INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx)
        VALUES(1, 'tunnel', 1, 1, 'tls', 1, 1000, 1000, 1, NULL, 0)
    `).Error; err != nil {
        t.Fatalf("insert tunnel: %v", err)
    }
    if err := r.DB().Exec(`
        INSERT INTO user_tunnel(id, user_id, tunnel_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status)
        VALUES(10, 2, 1, 10, 100, 500, 600, 0, 0, 1)
    `).Error; err != nil {
        t.Fatalf("insert user tunnel: %v", err)
    }
    if err := r.DB().Exec(`
        INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx)
        VALUES
          (20, 2, 'owner', 'target', 1, '127.0.0.1:80', 'fifo', 111, 222, 1000, ?, 1, 0),
          (21, 2, 'owner', 'other', 1, '127.0.0.1:81', 'fifo', 333, 444, 1000, ?, 1, 1)
    `, originalUpdated, originalUpdated).Error; err != nil {
        t.Fatalf("insert forwards: %v", err)
    }

    const resetAt int64 = 2000
    if err := r.ResetForwardFlow(20, resetAt); err != nil {
        t.Fatalf("ResetForwardFlow: %v", err)
    }

    assertForwardFlowResetValue(t, r, "SELECT in_flow FROM forward WHERE id = 20", 0)
    assertForwardFlowResetValue(t, r, "SELECT out_flow FROM forward WHERE id = 20", 0)
    assertForwardFlowResetValue(t, r, "SELECT updated_time FROM forward WHERE id = 20", resetAt)
    assertForwardFlowResetValue(t, r, "SELECT in_flow FROM forward WHERE id = 21", 333)
    assertForwardFlowResetValue(t, r, "SELECT out_flow FROM forward WHERE id = 21", 444)
    assertForwardFlowResetValue(t, r, "SELECT in_flow FROM user WHERE id = 2", 700)
    assertForwardFlowResetValue(t, r, "SELECT out_flow FROM user WHERE id = 2", 900)
    assertForwardFlowResetValue(t, r, "SELECT in_flow FROM user_tunnel WHERE id = 10", 500)
    assertForwardFlowResetValue(t, r, "SELECT out_flow FROM user_tunnel WHERE id = 10", 600)
}

func TestResetForwardFlowRejectsUninitializedRepository(t *testing.T) {
    var r *Repository
    if err := r.ResetForwardFlow(20, 2000); err == nil {
        t.Fatal("expected uninitialized repository error")
    }
}

func assertForwardFlowResetValue(t *testing.T, r *Repository, query string, want int64) {
    t.Helper()
    var got int64
    if err := r.DB().Raw(query).Scan(&got).Error; err != nil {
        t.Fatalf("query %q: %v", query, err)
    }
    if got != want {
        t.Fatalf("query %q returned %d, want %d", query, got, want)
    }
}
```

- [ ] **Step 2: Run the repository tests and verify the missing method failure**

Run:

```bash
cd go-backend && go test ./internal/store/repo -run TestResetForwardFlow -count=1
```

Expected: compilation fails because `ResetForwardFlow` is undefined.

- [ ] **Step 3: Implement the minimal repository method**

Add to the flow-reset section of `go-backend/internal/store/repo/repository_mutations.go`:

```go
func (r *Repository) ResetForwardFlow(forwardID int64, now int64) error {
    if r == nil || r.db == nil {
        return errors.New("repository not initialized")
    }
    return r.db.Model(&model.Forward{}).
        Where("id = ?", forwardID).
        Updates(map[string]interface{}{
            "in_flow":     0,
            "out_flow":    0,
            "updated_time": now,
        }).Error
}
```

The file already imports `errors` and `model`; do not add a new dependency.

- [ ] **Step 4: Format and run the focused repository tests**

Run:

```bash
cd go-backend && gofmt -w internal/store/repo/repository_forward_flow_reset_test.go internal/store/repo/repository_mutations.go
go test ./internal/store/repo -run TestResetForwardFlow -count=1
```

Expected: both reset tests pass.

- [ ] **Step 5: Commit the repository change**

```bash
git add go-backend/internal/store/repo/repository_mutations.go go-backend/internal/store/repo/repository_forward_flow_reset_test.go
git commit -m "feat: add forward flow reset repository method"
```

---

### Task 2: Add the authenticated reset endpoint

**Files:**
- Create: `go-backend/internal/http/handler/forward_reset_flow_test.go`
- Modify: `go-backend/internal/http/handler/handler.go`
- Modify: `go-backend/internal/http/handler/mutations.go`

**Interfaces:**
- Consumes: `POST` JSON `{ "id": number }`, `resolveForwardAccess`, and `Repository.ResetForwardFlow` from Task 1.
- Produces: `POST /api/v1/forward/reset-flow` and `func (h *Handler) forwardResetFlow(http.ResponseWriter, *http.Request)`.

- [ ] **Step 1: Write the failing handler tests**

Create `go-backend/internal/http/handler/forward_reset_flow_test.go`:

```go
package handler

import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "strconv"
    "testing"

    "go-backend/internal/auth"
    "go-backend/internal/http/middleware"
    "go-backend/internal/store/repo"
)

func TestForwardResetFlowPermissionsAndIsolation(t *testing.T) {
    tests := []struct {
        name       string
        actorID    int64
        actorRole  int
        forwardID  int64
        wantCode   int
        wantInFlow int64
        wantOutFlow int64
    }{
        {name: "admin resets another user's rule", actorID: 1, actorRole: 0, forwardID: 20, wantCode: 0, wantInFlow: 0, wantOutFlow: 0},
        {name: "owner resets own rule", actorID: 2, actorRole: 1, forwardID: 20, wantCode: 0, wantInFlow: 0, wantOutFlow: 0},
        {name: "user cannot reset another user's rule", actorID: 3, actorRole: 1, forwardID: 20, wantCode: -1, wantInFlow: 111, wantOutFlow: 222},
        {name: "missing rule is rejected", actorID: 1, actorRole: 0, forwardID: 999, wantCode: -1, wantInFlow: 111, wantOutFlow: 222},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            h, r := setupForwardResetFlowHandler(t)
            req := newForwardResetFlowRequest(t, http.MethodPost, tt.forwardID, tt.actorID, tt.actorRole)
            res := httptest.NewRecorder()

            h.forwardResetFlow(res, req)

            if got := decodeForwardResetFlowCode(t, res); got != tt.wantCode {
                t.Fatalf("code = %d, want %d; body=%s", got, tt.wantCode, res.Body.String())
            }
            assertForwardResetFlowDBValue(t, r, "SELECT in_flow FROM forward WHERE id = 20", tt.wantInFlow)
            assertForwardResetFlowDBValue(t, r, "SELECT out_flow FROM forward WHERE id = 20", tt.wantOutFlow)
            assertForwardResetFlowDBValue(t, r, "SELECT in_flow FROM user WHERE id = 2", 700)
            assertForwardResetFlowDBValue(t, r, "SELECT out_flow FROM user_tunnel WHERE id = 10", 600)
        })
    }
}

func TestForwardResetFlowRejectsInvalidRequests(t *testing.T) {
    h, _ := setupForwardResetFlowHandler(t)

    t.Run("non post", func(t *testing.T) {
        req := newForwardResetFlowRequest(t, http.MethodGet, 20, 1, 0)
        res := httptest.NewRecorder()
        h.forwardResetFlow(res, req)
        if code := decodeForwardResetFlowCode(t, res); code != -1 {
            t.Fatalf("code = %d, want -1", code)
        }
    })

    t.Run("invalid id", func(t *testing.T) {
        req := newForwardResetFlowRequest(t, http.MethodPost, 0, 1, 0)
        res := httptest.NewRecorder()
        h.forwardResetFlow(res, req)
        if code := decodeForwardResetFlowCode(t, res); code != -1 {
            t.Fatalf("code = %d, want -1", code)
        }
    })
}

func setupForwardResetFlowHandler(t *testing.T) (*Handler, *repo.Repository) {
    t.Helper()
    r, err := repo.Open(filepath.Join(t.TempDir(), "forward-reset-handler.db"))
    if err != nil {
        t.Fatalf("open repo: %v", err)
    }
    t.Cleanup(func() { _ = r.Close() })

    statements := []string{
        `INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status) VALUES(1, 'admin', 'pwd', 0, 0, 100, 0, 0, 0, 10, 1000, 1000, 1)`,
        `INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status) VALUES(2, 'owner', 'pwd', 1, 0, 100, 700, 900, 0, 10, 1000, 1000, 1)`,
        `INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status) VALUES(3, 'other', 'pwd', 1, 0, 100, 0, 0, 0, 10, 1000, 1000, 1)`,
        `INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx) VALUES(1, 'tunnel', 1, 1, 'tls', 1, 1000, 1000, 1, NULL, 0)`,
        `INSERT INTO user_tunnel(id, user_id, tunnel_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status) VALUES(10, 2, 1, 10, 100, 500, 600, 0, 0, 1)`,
        `INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx) VALUES(20, 2, 'owner', 'target', 1, '127.0.0.1:80', 'fifo', 111, 222, 1000, 1000, 1, 0)`,
    }
    for _, statement := range statements {
        if err := r.DB().Exec(statement).Error; err != nil {
            t.Fatalf("seed database: %v", err)
        }
    }
    return New(r, "test-secret"), r
}

func newForwardResetFlowRequest(t *testing.T, method string, forwardID, actorID int64, roleID int) *http.Request {
    t.Helper()
    body, err := json.Marshal(map[string]int64{"id": forwardID})
    if err != nil {
        t.Fatalf("marshal request: %v", err)
    }
    req := httptest.NewRequest(method, "/api/v1/forward/reset-flow", bytes.NewReader(body))
    claims := auth.Claims{Sub: strconv.FormatInt(actorID, 10), RoleID: roleID}
    return req.WithContext(context.WithValue(req.Context(), middleware.ClaimsContextKey, claims))
}

func decodeForwardResetFlowCode(t *testing.T, res *httptest.ResponseRecorder) int {
    t.Helper()
    var payload struct {
        Code int `json:"code"`
    }
    if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
        t.Fatalf("decode response: %v; body=%s", err, res.Body.String())
    }
    return payload.Code
}

func assertForwardResetFlowDBValue(t *testing.T, r *repo.Repository, query string, want int64) {
    t.Helper()
    var got int64
    if err := r.DB().Raw(query).Scan(&got).Error; err != nil {
        t.Fatalf("query %q: %v", query, err)
    }
    if got != want {
        t.Fatalf("query %q returned %d, want %d", query, got, want)
    }
}
```

If the project's default error code differs from `-1`, replace the test expectation with the actual `response.ErrDefault` code after inspecting one existing handler response; do not weaken the success and database assertions.

- [ ] **Step 2: Run the handler tests and verify the missing handler failure**

Run:

```bash
cd go-backend && go test ./internal/http/handler -run TestForwardResetFlow -count=1
```

Expected: compilation fails because `forwardResetFlow` is undefined.

- [ ] **Step 3: Register and implement the endpoint**

Add this route beside the other forward routes in `go-backend/internal/http/handler/handler.go`:

```go
mux.HandleFunc("/api/v1/forward/reset-flow", h.forwardResetFlow)
```

Add this handler beside `forwardPause` and `forwardResume` in `go-backend/internal/http/handler/mutations.go`:

```go
func (h *Handler) forwardResetFlow(w http.ResponseWriter, r *http.Request) {
    id := idFromBody(r, w)
    if id <= 0 {
        return
    }
    if _, _, _, err := h.resolveForwardAccess(r, id); err != nil {
        if errors.Is(err, errForwardNotFound) {
            response.WriteJSON(w, response.ErrDefault("转发不存在"))
            return
        }
        response.WriteJSON(w, response.Err(-2, err.Error()))
        return
    }
    if err := h.repo.ResetForwardFlow(id, time.Now().UnixMilli()); err != nil {
        response.WriteJSON(w, response.Err(-2, err.Error()))
        return
    }
    response.WriteJSON(w, response.OKEmpty())
}
```

This deliberately does not call runtime service controls or nftables reconciliation.

- [ ] **Step 4: Format and run the focused handler tests**

Run:

```bash
cd go-backend && gofmt -w internal/http/handler/forward_reset_flow_test.go internal/http/handler/handler.go internal/http/handler/mutations.go
go test ./internal/http/handler -run TestForwardResetFlow -count=1
```

Expected: all reset endpoint tests pass.

- [ ] **Step 5: Run all backend tests**

Run:

```bash
cd go-backend && go test ./...
```

Expected: all backend packages and contract tests pass, excluding environment-gated PostgreSQL tests when `FLVX_POSTGRES_TEST_DSN` is unset.

- [ ] **Step 6: Commit the endpoint change**

```bash
git add go-backend/internal/http/handler/handler.go go-backend/internal/http/handler/mutations.go go-backend/internal/http/handler/forward_reset_flow_test.go
git commit -m "feat: add forward flow reset endpoint"
```

---

### Task 3: Add the rule-page reset action and confirmation modal

**Files:**
- Modify: `vite-frontend/src/api/index.ts`
- Modify: `vite-frontend/src/pages/forward.tsx`

**Interfaces:**
- Consumes: `POST /forward/reset-flow`, the page's `Forward` shape, `refreshForwardList`, toast notifications, and existing modal/button bridge components.
- Produces: `resetForwardFlow(id: number)`, a shared reset handler, disabled zero-usage actions in all rule views, and one confirmation modal.

- [ ] **Step 1: Add the frontend API wrapper**

Add beside the forward control operations in `vite-frontend/src/api/index.ts`:

```ts
export const resetForwardFlow = (forwardId: number) =>
  Network.post("/forward/reset-flow", { id: forwardId });
```

Import `resetForwardFlow` from `@/api` in `vite-frontend/src/pages/forward.tsx`.

- [ ] **Step 2: Add page state and shared reset handlers**

Add state beside the existing delete modal state:

```ts
const [resetFlowModalOpen, setResetFlowModalOpen] = useState(false);
const [resetFlowLoading, setResetFlowLoading] = useState(false);
const [forwardToResetFlow, setForwardToResetFlow] = useState<Forward | null>(null);
```

Add these handlers beside `handleDelete` and `confirmDelete`:

```ts
const handleResetFlow = (forward: Forward) => {
  if ((forward.inFlow || 0) + (forward.outFlow || 0) <= 0) return;
  setForwardToResetFlow(forward);
  setResetFlowModalOpen(true);
};

const confirmResetFlow = async () => {
  if (!forwardToResetFlow) return;

  setResetFlowLoading(true);
  try {
    const res = await resetForwardFlow(forwardToResetFlow.id);

    if (res.code !== 0) {
      toast.error(res.msg || "流量清零失败");
      return;
    }

    toast.success("规则流量已清零");
    setResetFlowModalOpen(false);
    setForwardToResetFlow(null);
    await refreshForwardList(false);
  } catch {
    toast.error("流量清零失败");
  } finally {
    setResetFlowLoading(false);
  }
};
```

- [ ] **Step 3: Add one reusable reset icon button to both table row components**

Pass `handleResetFlow` into `SortableTableRow` and `SortableCompactTableRow` at every render site. Add it to each component's destructured props.

Insert this button between diagnosis and delete in each table action cell:

```tsx
<Button
  isIconOnly
  className="bg-secondary/10 text-secondary hover:bg-secondary/20"
  isDisabled={(forward.inFlow || 0) + (forward.outFlow || 0) <= 0}
  size="sm"
  title="流量清零"
  onPress={() => handleResetFlow(forward)}
>
  <svg
    aria-hidden="true"
    className="h-4 w-4"
    fill="none"
    stroke="currentColor"
    viewBox="0 0 24 24"
  >
    <path
      d="M4 4v6h6M20 20v-6h-6M20 9a8 8 0 00-13.657-3.657L4 8m16 8-2.343 2.657A8 8 0 014 15"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth={2}
    />
  </svg>
</Button>
```

- [ ] **Step 4: Add the reset action to the card view**

Insert a fourth action button between diagnosis and delete in `renderForwardCard`:

```tsx
<Button
  className="flex-1 min-h-8"
  color="secondary"
  isDisabled={(forward.inFlow || 0) + (forward.outFlow || 0) <= 0}
  size="sm"
  startContent={
    <svg
      aria-hidden="true"
      className="w-3 h-3"
      fill="none"
      stroke="currentColor"
      viewBox="0 0 24 24"
    >
      <path
        d="M4 4v6h6M20 20v-6h-6M20 9a8 8 0 00-13.657-3.657L4 8m16 8-2.343 2.657A8 8 0 014 15"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={2}
      />
    </svg>
  }
  variant="flat"
  onPress={() => handleResetFlow(forward)}
>
  清零
</Button>
```

Change the card action container from `flex gap-1.5 mt-3` to `grid grid-cols-2 gap-1.5 mt-3` so all four actions remain readable at the smallest supported card width.

- [ ] **Step 5: Add the confirmation modal**

Add beside the delete confirmation modal:

```tsx
<Modal
  backdrop="blur"
  classNames={{
    base: "!w-[calc(100%-32px)] !mx-auto sm:!w-full rounded-2xl overflow-hidden",
  }}
  isOpen={resetFlowModalOpen}
  placement="center"
  scrollBehavior="inside"
  size="lg"
  onOpenChange={setResetFlowModalOpen}
>
  <ModalContent>
    {(onClose) => (
      <>
        <ModalHeader className="flex flex-col gap-1">
          <h2 className="text-lg font-bold text-secondary">确认流量清零</h2>
        </ModalHeader>
        <ModalBody>
          <p className="text-default-600">
            确定要清零规则{" "}
            <span className="font-semibold text-foreground">
              &quot;{forwardToResetFlow?.name}&quot;
            </span>{" "}
            当前显示的上传和下载流量吗？
          </p>
          <p className="text-small text-default-500 mt-2">
            此操作不可撤销，但不会影响用户总流量、用户隧道配额和历史统计。
          </p>
        </ModalBody>
        <ModalFooter>
          <Button isDisabled={resetFlowLoading} variant="light" onPress={onClose}>
            取消
          </Button>
          <Button
            color="secondary"
            isLoading={resetFlowLoading}
            onPress={confirmResetFlow}
          >
            确认清零
          </Button>
        </ModalFooter>
      </>
    )}
  </ModalContent>
</Modal>
```

Add this wrapper beside the other reset handlers and pass it to the modal as `onOpenChange={handleResetFlowModalOpenChange}`:

```ts
const handleResetFlowModalOpenChange = (isOpen: boolean) => {
  if (resetFlowLoading) return;
  setResetFlowModalOpen(isOpen);
  if (!isOpen) {
    setForwardToResetFlow(null);
  }
};
```

- [ ] **Step 6: Format and verify the frontend**

Run:

```bash
cd vite-frontend && pnpm exec prettier --write src/api/index.ts src/pages/forward.tsx
pnpm run build
pnpm run lint
```

Expected: TypeScript/Vite build succeeds and ESLint finishes without errors.

- [ ] **Step 7: Commit the frontend change**

```bash
git add vite-frontend/src/api/index.ts vite-frontend/src/pages/forward.tsx
git commit -m "feat: add forward flow reset action"
```

---

### Task 4: Perform integrated verification

**Files:**
- Verify only; no planned source changes.

**Interfaces:**
- Consumes: the repository method, API endpoint, and rule-page action from Tasks 1-3.
- Produces: evidence that the complete feature builds and all affected tests pass.

- [ ] **Step 1: Run the complete backend suite**

```bash
cd go-backend && go test ./...
```

Expected: all available backend tests pass.

- [ ] **Step 2: Run the complete frontend checks**

```bash
cd vite-frontend && pnpm run build && pnpm run lint
```

Expected: both commands exit successfully.

- [ ] **Step 3: Check formatting and working-tree scope**

```bash
git diff --check
git status --short
git log -4 --oneline
```

Expected: no whitespace errors; the working tree is clean; the three feature commits are visible after the design and implementation-plan commits.

- [ ] **Step 4: Manually verify the feature when a local panel is available**

1. Open the Rules page as an administrator and reset a rule with non-zero upload/download traffic.
2. Confirm the modal states that user totals, tunnel quota, and history are unaffected.
3. Confirm the rule immediately shows zero after success.
4. Confirm the user page's total traffic and user-tunnel traffic values did not change.
5. Generate new traffic and confirm the rule starts accumulating from zero.
6. Log in as a normal user and confirm the user can reset an owned rule but cannot access another user's rule through a direct API request.

Expected: all six checks match the design specification.
