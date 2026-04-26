# flow/upload Batch Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce `POST /flow/upload` database pressure by converting the hot path from per-item queries and per-item transactions to per-request aggregation, batched metadata reads, and batched writes, while preserving immediate quota disable / forward pause behavior inside the same upload.

**Architecture:** Parse one upload into a batch object in the handler layer, fetch one shared `forward+tunnel` metadata map, then reuse that map for flow accounting and tunnel metric aggregation. Replace `AddFlow` and `AddUserQuotaUsage` per-item transactions with one batched flow transaction and one batched quota transaction; run policy enforcement, orphan cleanup, and peer-share flow handling once per affected target instead of once per item.

**Tech Stack:** Go, net/http, GORM, SQLite/PostgreSQL, existing backend contract tests.

---

## File Map

- Create: `go-backend/internal/http/handler/flow_upload_batch.go`
  Responsibility: request-scoped parsing, aggregation, and application of one `/flow/upload` batch.
- Create: `go-backend/internal/http/handler/flow_upload_batch_test.go`
  Responsibility: unit coverage for batch aggregation semantics.
- Create: `go-backend/internal/store/repo/repository_flow_batch_test.go`
  Responsibility: unit coverage for batched flow and quota persistence.
- Create: `go-backend/tests/contract/flow_upload_batch_contract_test.go`
  Responsibility: contract coverage that repeated items still accumulate correctly and still disable quota immediately.
- Modify: `go-backend/internal/http/handler/handler.go`
  Responsibility: switch `/flow/upload` entrypoint to the new batch pipeline.
- Modify: `go-backend/internal/http/handler/tunnel_metrics_ingestion.go`
  Responsibility: accept pre-aggregated forward deltas plus shared forward metadata instead of reparsing the raw items.
- Modify: `go-backend/internal/store/repo/repository.go`
  Responsibility: add batched flow persistence primitives near the existing flow update code.
- Modify: `go-backend/internal/store/repo/repository_flow.go`
  Responsibility: add shared flow-upload metadata query helpers.
- Modify: `go-backend/internal/store/repo/repository_user_quota.go`
  Responsibility: add batched quota usage persistence that still returns normalized quota views for immediate enforcement.

---

### Task 1: Add Failing Tests For Batched flow/upload Semantics

**Files:**
- Create: `go-backend/internal/http/handler/flow_upload_batch_test.go`
- Create: `go-backend/tests/contract/flow_upload_batch_contract_test.go`

- [ ] **Step 1: Write the failing handler unit test**

Create `go-backend/internal/http/handler/flow_upload_batch_test.go` with a unit test that locks in the new aggregation contract.

```go
package handler

import (
	"testing"

	"go-backend/internal/store/repo"
)

func TestBuildFlowUploadBatchAggregatesForwardQuotaPeerShareAndCleanupTargets(t *testing.T) {
	h := &Handler{}
	metas := map[int64]repo.FlowUploadForwardMeta{
		20: {
			ForwardID:     20,
			TunnelID:      1,
			TrafficRatio:  2,
			TunnelFlow:    3,
		},
	}

	batch := h.buildFlowUploadBatch([]flowItem{
		{N: "20_2_10", U: 70, D: 50},
		{N: "20_2_10_tcp", U: 40, D: 30},
		{N: "99_2_10", U: 12, D: 8},
		{N: "fed_svc_17", U: 9, D: 1},
	}, metas)

	if len(batch.flowDeltas) != 1 {
		t.Fatalf("expected 1 flow delta, got %d", len(batch.flowDeltas))
	}
	delta := batch.flowDeltas[0]
	if delta.ForwardID != 20 || delta.UserID != 2 || delta.UserTunnelID != 10 {
		t.Fatalf("unexpected flow delta identity: %#v", delta)
	}
	if delta.InFlow != 480 || delta.OutFlow != 660 {
		t.Fatalf("expected scaled flow in=480 out=660, got in=%d out=%d", delta.InFlow, delta.OutFlow)
	}
	if batch.quotaUsage[2] != 1140 {
		t.Fatalf("expected quota usage 1140, got %d", batch.quotaUsage[2])
	}
	if len(batch.policyTargets) != 1 {
		t.Fatalf("expected 1 policy target, got %d", len(batch.policyTargets))
	}
	if batch.policyTargets[0].UserID != 2 || batch.policyTargets[0].UserTunnelID != 10 {
		t.Fatalf("unexpected policy target: %#v", batch.policyTargets[0])
	}
	traffic := batch.forwardTraffic[20]
	if traffic.bytesIn != 80 || traffic.bytesOut != 110 {
		t.Fatalf("expected raw traffic in=80 out=110, got in=%d out=%d", traffic.bytesIn, traffic.bytesOut)
	}
	if _, ok := batch.orphanServices["99_2_10"]; !ok {
		t.Fatalf("expected orphan service cleanup target for 99_2_10")
	}
	if item, ok := batch.peerShareForwardItems["20_2_10"]; !ok || item.U != 110 || item.D != 80 {
		t.Fatalf("expected merged peer-share forward item, got %#v ok=%v", item, ok)
	}
	if item, ok := batch.peerShareRuntimeItems[17]; !ok || item.U != 9 || item.D != 1 {
		t.Fatalf("expected merged peer-share runtime item, got %#v ok=%v", item, ok)
	}
}
```

- [ ] **Step 2: Run the handler unit test to verify RED**

Run:

```bash
go test ./internal/http/handler -run TestBuildFlowUploadBatchAggregatesForwardQuotaPeerShareAndCleanupTargets -v
```

Expected: FAIL because `FlowUploadForwardMeta`, `buildFlowUploadBatch`, and the new batch fields do not exist yet.

- [ ] **Step 3: Write the contract test that guards current behavior**

Create `go-backend/tests/contract/flow_upload_batch_contract_test.go` so the optimization cannot weaken same-request quota enforcement.

```go
package contract_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go-backend/internal/store/model"
)

func TestFlowUploadAggregatesRepeatedItemsAndDisablesQuotaImmediately(t *testing.T) {
	secret := "monitoring-jwt-secret"
	router, repo := setupContractRouter(t, secret)
	now := time.Now()
	nowMs := now.UnixMilli()
	dayKey := int64(now.Year()*10000 + int(now.Month())*100 + now.Day())
	monthKey := int64(now.Year()*100 + int(now.Month()))
	const bytesPerGB = int64(1024 * 1024 * 1024)

	node := &model.Node{Name: "node-1", Secret: "node-secret", ServerIP: "127.0.0.1", Port: "10000-10010", TCPListenAddr: "[::]", UDPListenAddr: "[::]", CreatedTime: nowMs, Status: 1}
	if err := repo.DB().Create(node).Error; err != nil {
		t.Fatalf("seed node: %v", err)
	}
	if err := repo.DB().Exec(`INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status) VALUES(2, 'flow_user', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)`, nowMs, nowMs).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	tunnel := &model.Tunnel{Name: "tunnel-1", TrafficRatio: 1.0, Type: 1, Protocol: "tls", Flow: 1, CreatedTime: nowMs, UpdatedTime: nowMs, Status: 1}
	if err := repo.DB().Create(tunnel).Error; err != nil {
		t.Fatalf("seed tunnel: %v", err)
	}
	if err := repo.DB().Exec(`INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status) VALUES(10, 2, ?, NULL, 99999, 99999, 0, 0, 1, 2727251700000, 1)`, tunnel.ID).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}
	forward := &model.Forward{ID: 20, UserID: 2, UserName: "flow_user", Name: "forward-20", TunnelID: tunnel.ID, RemoteAddr: "1.1.1.1:80", Strategy: "fifo", CreatedTime: nowMs, UpdatedTime: nowMs, Status: 1}
	if err := repo.DB().Create(forward).Error; err != nil {
		t.Fatalf("seed forward: %v", err)
	}
	if err := repo.DB().Exec(`INSERT INTO user_quota(user_id, daily_limit_gb, monthly_limit_gb, daily_used_bytes, monthly_used_bytes, day_key, month_key, disabled_by_quota, disabled_at, paused_forward_ids, created_time, updated_time) VALUES(2, 1, 0, ?, ?, ?, ?, 0, 0, '', ?, ?)`, bytesPerGB-100, bytesPerGB-100, dayKey, monthKey, nowMs, nowMs).Error; err != nil {
		t.Fatalf("insert user_quota: %v", err)
	}

	body, err := json.Marshal([]map[string]interface{}{
		{"n": "20_2_10", "u": 70, "d": 50},
		{"n": "20_2_10_tcp", "u": 40, "d": 30},
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/flow/upload?secret="+node.Secret, bytes.NewReader(body))
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}
	if got := mustQueryInt(t, repo, `SELECT status FROM forward WHERE id = 20`); got != 0 {
		t.Fatalf("expected forward paused immediately, got status=%d", got)
	}
	if got := mustQueryInt(t, repo, `SELECT disabled_by_quota FROM user_quota WHERE user_id = 2`); got != 1 {
		t.Fatalf("expected quota disabled flag=1, got %d", got)
	}
	if got := mustQueryInt(t, repo, `SELECT in_flow FROM forward WHERE id = 20`); got != 80 {
		t.Fatalf("expected forward in_flow=80, got %d", got)
	}
	if got := mustQueryInt(t, repo, `SELECT out_flow FROM forward WHERE id = 20`); got != 110 {
		t.Fatalf("expected forward out_flow=110, got %d", got)
	}
	metrics, err := repo.GetTunnelMetrics(tunnel.ID, 0, nowMs+60_000)
	if err != nil {
		t.Fatalf("get tunnel metrics: %v", err)
	}
	if len(metrics) != 1 || metrics[0].BytesIn != 80 || metrics[0].BytesOut != 110 {
		t.Fatalf("expected one aggregated metric row, got %#v", metrics)
	}
}
```

- [ ] **Step 4: Run the contract test to verify the same-request guard stays green or reveals an existing regression**

Run:

```bash
go test ./tests/contract/... -run TestFlowUploadAggregatesRepeatedItemsAndDisablesQuotaImmediately -v
```

Expected: this test may already PASS before the refactor because it locks in existing external behavior. Keep it either way; it is the guardrail for the optimization.

- [ ] **Step 5: Optional commit if the user explicitly requested commits**

```bash
git add go-backend/internal/http/handler/flow_upload_batch_test.go go-backend/tests/contract/flow_upload_batch_contract_test.go
git commit -m "test: cover flow upload batch semantics"
```

---

### Task 2: Add Batched Repository Primitives

**Files:**
- Modify: `go-backend/internal/store/repo/repository_flow.go`
- Modify: `go-backend/internal/store/repo/repository.go`
- Modify: `go-backend/internal/store/repo/repository_user_quota.go`
- Create: `go-backend/internal/store/repo/repository_flow_batch_test.go`

- [ ] **Step 1: Write the failing repository tests**

Create `go-backend/internal/store/repo/repository_flow_batch_test.go` with coverage for both the shared metadata query and the batched counter/quota writes.

```go
package repo

import (
	"path/filepath"
	"testing"
	"time"
)

func TestGetFlowUploadForwardMetasAndApplyFlowUploadDeltasBatch(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "flow-batch.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now().UnixMilli()
	if err := r.DB().Exec(`INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status) VALUES(2, 'u2', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)`, now, now).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := r.DB().Exec(`INSERT INTO tunnel(id, name, traffic_ratio, type, protocol, flow, created_time, updated_time, status, in_ip, inx) VALUES(1, 't1', 2.0, 1, 'tls', 3, ?, ?, 1, NULL, 0)`, now, now).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	if err := r.DB().Exec(`INSERT INTO user_tunnel(id, user_id, tunnel_id, speed_id, num, flow, in_flow, out_flow, flow_reset_time, exp_time, status) VALUES(10, 2, 1, NULL, 99999, 99999, 0, 0, 1, 2727251700000, 1)`).Error; err != nil {
		t.Fatalf("insert user_tunnel: %v", err)
	}
	if err := r.DB().Exec(`INSERT INTO forward(id, user_id, user_name, name, tunnel_id, remote_addr, strategy, in_flow, out_flow, created_time, updated_time, status, inx) VALUES(20, 2, 'u2', 'f20', 1, '1.1.1.1:80', 'fifo', 0, 0, ?, ?, 1, 0)`, now, now).Error; err != nil {
		t.Fatalf("insert forward: %v", err)
	}

	metas, err := r.GetFlowUploadForwardMetas([]int64{20, 99})
	if err != nil {
		t.Fatalf("get metas: %v", err)
	}
	if metas[20].TunnelID != 1 || metas[20].TrafficRatio != 2 || metas[20].TunnelFlow != 3 {
		t.Fatalf("unexpected meta for forward 20: %#v", metas[20])
	}
	if _, ok := metas[99]; ok {
		t.Fatalf("did not expect meta for missing forward 99")
	}

	err = r.ApplyFlowUploadDeltasBatch([]FlowUploadCounterDelta{{ForwardID: 20, UserID: 2, UserTunnelID: 10, InFlow: 480, OutFlow: 660}})
	if err != nil {
		t.Fatalf("apply flow batch: %v", err)
	}
	if got := mustFlowBatchCount(t, r, `SELECT in_flow FROM forward WHERE id = 20`); got != 480 {
		t.Fatalf("expected forward in_flow=480, got %d", got)
	}
	if got := mustFlowBatchCount(t, r, `SELECT out_flow FROM user WHERE id = 2`); got != 660 {
		t.Fatalf("expected user out_flow=660, got %d", got)
	}
	if got := mustFlowBatchCount(t, r, `SELECT in_flow FROM user_tunnel WHERE id = 10`); got != 480 {
		t.Fatalf("expected user_tunnel in_flow=480, got %d", got)
	}
}

func TestAddUserQuotaUsageBatchReturnsNormalizedViews(t *testing.T) {
	r, err := Open(filepath.Join(t.TempDir(), "quota-batch.db"))
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer r.Close()

	now := time.Now()
	nowMs := now.UnixMilli()
	if err := r.DB().Exec(`INSERT INTO user(id, user, pwd, role_id, exp_time, flow, in_flow, out_flow, flow_reset_time, num, created_time, updated_time, status) VALUES(2, 'u2', 'pwd', 1, 2727251700000, 99999, 0, 0, 1, 99999, ?, ?, 1)`, nowMs, nowMs).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	views, err := r.AddUserQuotaUsageBatch(map[int64]int64{2: 1140}, now)
	if err != nil {
		t.Fatalf("batch quota update: %v", err)
	}
	if views[2] == nil || views[2].DailyUsedBytes != 1140 || views[2].MonthlyUsedBytes != 1140 {
		t.Fatalf("unexpected quota view: %#v", views[2])
	}
}

func mustFlowBatchCount(t *testing.T, r *Repository, query string, args ...interface{}) int64 {
	t.Helper()
	var value int64
	if err := r.DB().Raw(query, args...).Row().Scan(&value); err != nil {
		t.Fatalf("query %q failed: %v", query, err)
	}
	return value
}
```

- [ ] **Step 2: Run the repository tests to verify RED**

Run:

```bash
go test ./internal/store/repo -run 'TestGetFlowUploadForwardMetasAndApplyFlowUploadDeltasBatch|TestAddUserQuotaUsageBatchReturnsNormalizedViews' -v
```

Expected: FAIL because `GetFlowUploadForwardMetas`, `ApplyFlowUploadDeltasBatch`, `FlowUploadCounterDelta`, and `AddUserQuotaUsageBatch` do not exist yet.

- [ ] **Step 3: Implement shared flow-upload metadata and batched persistence**

Update `go-backend/internal/store/repo/repository_flow.go`, `repository.go`, and `repository_user_quota.go` with the following concrete APIs. Add `sort` to the `repository_user_quota.go` import list.

```go
// repository_flow.go
type FlowUploadForwardMeta struct {
	ForwardID    int64
	TunnelID     int64
	TrafficRatio float64
	TunnelFlow   int64
}

func (r *Repository) GetFlowUploadForwardMetas(forwardIDs []int64) (map[int64]FlowUploadForwardMeta, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	if len(forwardIDs) == 0 {
		return map[int64]FlowUploadForwardMeta{}, nil
	}
	ids := make([]int64, 0, len(forwardIDs))
	seen := make(map[int64]struct{}, len(forwardIDs))
	for _, id := range forwardIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	type row struct {
		ForwardID    int64   `gorm:"column:forward_id"`
		TunnelID     int64   `gorm:"column:tunnel_id"`
		TrafficRatio float64 `gorm:"column:traffic_ratio"`
		TunnelFlow   int64   `gorm:"column:tunnel_flow"`
	}
	var rows []row
	err := r.db.Table("forward AS f").
		Select("f.id AS forward_id, f.tunnel_id AS tunnel_id, t.traffic_ratio AS traffic_ratio, t.flow AS tunnel_flow").
		Joins("JOIN tunnel t ON t.id = f.tunnel_id").
		Where("f.id IN ?", ids).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[int64]FlowUploadForwardMeta, len(rows))
	for _, row := range rows {
		if row.TunnelFlow <= 0 {
			row.TunnelFlow = 1
		}
		if row.TrafficRatio <= 0 {
			row.TrafficRatio = 1
		}
		out[row.ForwardID] = FlowUploadForwardMeta{ForwardID: row.ForwardID, TunnelID: row.TunnelID, TrafficRatio: row.TrafficRatio, TunnelFlow: row.TunnelFlow}
	}
	return out, nil
}
```

```go
// repository.go
type FlowUploadCounterDelta struct {
	ForwardID    int64
	UserID       int64
	UserTunnelID int64
	InFlow       int64
	OutFlow      int64
}

func (r *Repository) ApplyFlowUploadDeltasBatch(deltas []FlowUploadCounterDelta) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if len(deltas) == 0 {
		return nil
	}
	forwardTotals := make(map[int64][2]int64, len(deltas))
	userTotals := make(map[int64][2]int64, len(deltas))
	userTunnelTotals := make(map[int64][2]int64, len(deltas))
	for _, delta := range deltas {
		if delta.ForwardID > 0 {
			current := forwardTotals[delta.ForwardID]
			current[0] += delta.InFlow
			current[1] += delta.OutFlow
			forwardTotals[delta.ForwardID] = current
		}
		if delta.UserID > 0 {
			current := userTotals[delta.UserID]
			current[0] += delta.InFlow
			current[1] += delta.OutFlow
			userTotals[delta.UserID] = current
		}
		if delta.UserTunnelID > 0 {
			current := userTunnelTotals[delta.UserTunnelID]
			current[0] += delta.InFlow
			current[1] += delta.OutFlow
			userTunnelTotals[delta.UserTunnelID] = current
		}
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		for forwardID, total := range forwardTotals {
			if err := tx.Model(&model.Forward{}).Where("id = ?", forwardID).UpdateColumns(map[string]interface{}{"in_flow": gorm.Expr("in_flow + ?", total[0]), "out_flow": gorm.Expr("out_flow + ?", total[1])}).Error; err != nil {
				return err
			}
		}
		for userID, total := range userTotals {
			if err := tx.Model(&model.User{}).Where("id = ?", userID).UpdateColumns(map[string]interface{}{"in_flow": gorm.Expr("in_flow + ?", total[0]), "out_flow": gorm.Expr("out_flow + ?", total[1])}).Error; err != nil {
				return err
			}
		}
		for userTunnelID, total := range userTunnelTotals {
			if err := tx.Model(&model.UserTunnel{}).Where("id = ?", userTunnelID).UpdateColumns(map[string]interface{}{"in_flow": gorm.Expr("in_flow + ?", total[0]), "out_flow": gorm.Expr("out_flow + ?", total[1])}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
```

```go
// repository_user_quota.go
func (r *Repository) AddUserQuotaUsageBatch(usages map[int64]int64, now time.Time) (map[int64]*model.UserQuotaView, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	if len(usages) == 0 {
		return map[int64]*model.UserQuotaView{}, nil
	}
	result := make(map[int64]*model.UserQuotaView, len(usages))
	err := r.db.Transaction(func(tx *gorm.DB) error {
		userIDs := make([]int64, 0, len(usages))
		for userID := range usages {
			if userID > 0 {
				userIDs = append(userIDs, userID)
			}
		}
		sort.Slice(userIDs, func(i, j int) bool { return userIDs[i] < userIDs[j] })
		for _, userID := range userIDs {
			q, err := r.loadOrCreateUserQuotaTx(tx, userID, now)
			if err != nil {
				return err
			}
			applyUserQuotaWindowRoll(q, now)
			if usages[userID] > 0 {
				q.DailyUsedBytes += usages[userID]
				q.MonthlyUsedBytes += usages[userID]
			}
			q.UpdatedTime = now.UnixMilli()
			if err := tx.Model(&model.UserQuota{}).Where("user_id = ?", userID).Updates(map[string]interface{}{"daily_used_bytes": q.DailyUsedBytes, "monthly_used_bytes": q.MonthlyUsedBytes, "day_key": q.DayKey, "month_key": q.MonthKey, "updated_time": q.UpdatedTime}).Error; err != nil {
				return err
			}
			result[userID] = normalizeUserQuotaView(cloneUserQuotaView(*q), now)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
```

- [ ] **Step 4: Run the repository tests to verify GREEN**

Run:

```bash
go test ./internal/store/repo -run 'TestGetFlowUploadForwardMetasAndApplyFlowUploadDeltasBatch|TestAddUserQuotaUsageBatchReturnsNormalizedViews' -v
```

Expected: PASS.

- [ ] **Step 5: Optional commit if the user explicitly requested commits**

```bash
git add go-backend/internal/store/repo/repository.go go-backend/internal/store/repo/repository_flow.go go-backend/internal/store/repo/repository_user_quota.go go-backend/internal/store/repo/repository_flow_batch_test.go
git commit -m "refactor: batch flow upload persistence"
```

---

### Task 3: Refactor flow/upload To Use One Parsed Batch

**Files:**
- Create: `go-backend/internal/http/handler/flow_upload_batch.go`
- Modify: `go-backend/internal/http/handler/handler.go`
- Modify: `go-backend/internal/http/handler/tunnel_metrics_ingestion.go`
- Modify: `go-backend/internal/http/handler/flow_upload_batch_test.go`
- Modify: `go-backend/tests/contract/flow_upload_batch_contract_test.go`

- [ ] **Step 1: Write the new handler batch implementation**

Create `go-backend/internal/http/handler/flow_upload_batch.go` and move the request-scoped aggregation there.

```go
package handler

import (
	"log"
	"sort"
	"strings"
	"time"

	"go-backend/internal/store/repo"
)

type flowPolicyTarget struct {
	UserID       int64
	UserTunnelID int64
}

type flowUploadBatch struct {
	flowDeltas           []repo.FlowUploadCounterDelta
	quotaUsage           map[int64]int64
	policyTargets        []flowPolicyTarget
	forwardTraffic       map[int64]tunnelTrafficDelta
	orphanServices       map[string]struct{}
	peerShareForwardItems map[string]flowItem
	peerShareRuntimeItems map[int64]flowItem
}

func (h *Handler) buildFlowUploadBatch(items []flowItem, metas map[int64]repo.FlowUploadForwardMeta) flowUploadBatch {
	batch := flowUploadBatch{
		quotaUsage:            make(map[int64]int64),
		forwardTraffic:        make(map[int64]tunnelTrafficDelta),
		orphanServices:        make(map[string]struct{}),
		peerShareForwardItems: make(map[string]flowItem),
		peerShareRuntimeItems: make(map[int64]flowItem),
	}
	policySeen := map[flowPolicyTarget]struct{}{}
	flowSeen := map[int64]int{}

	for _, item := range items {
		serviceName := strings.TrimSpace(item.N)
		if serviceName == "" || serviceName == "web_api" {
			continue
		}
		if runtimeID, ok := parsePeerShareRuntimeServiceID(serviceName); ok {
			merged := batch.peerShareRuntimeItems[runtimeID]
			merged.N = serviceName
			merged.U += item.U
			merged.D += item.D
			batch.peerShareRuntimeItems[runtimeID] = merged
			continue
		}
		forwardID, userID, userTunnelID, ok := parseFlowServiceIDs(serviceName)
		if !ok {
			continue
		}
		meta, exists := metas[forwardID]
		if !exists {
			batch.orphanServices[serviceName] = struct{}{}
			continue
		}
		raw := batch.forwardTraffic[forwardID]
		raw.bytesIn += item.D
		raw.bytesOut += item.U
		batch.forwardTraffic[forwardID] = raw

		scaledIn := int64(float64(item.D)*meta.TrafficRatio) * meta.TunnelFlow
		scaledOut := int64(float64(item.U)*meta.TrafficRatio) * meta.TunnelFlow
		if idx, ok := flowSeen[forwardID]; ok {
			batch.flowDeltas[idx].InFlow += scaledIn
			batch.flowDeltas[idx].OutFlow += scaledOut
		} else {
			flowSeen[forwardID] = len(batch.flowDeltas)
			batch.flowDeltas = append(batch.flowDeltas, repo.FlowUploadCounterDelta{ForwardID: forwardID, UserID: userID, UserTunnelID: userTunnelID, InFlow: scaledIn, OutFlow: scaledOut})
		}
		batch.quotaUsage[userID] += scaledIn + scaledOut
		target := flowPolicyTarget{UserID: userID, UserTunnelID: userTunnelID}
		if _, seen := policySeen[target]; !seen {
			policySeen[target] = struct{}{}
			batch.policyTargets = append(batch.policyTargets, target)
		}
		merged := batch.peerShareForwardItems[normalizeForwardRuntimeServiceName(serviceName)]
		merged.N = normalizeForwardRuntimeServiceName(serviceName)
		merged.U += item.U
		merged.D += item.D
		batch.peerShareForwardItems[normalizeForwardRuntimeServiceName(serviceName)] = merged
	}

	sort.Slice(batch.policyTargets, func(i, j int) bool {
		if batch.policyTargets[i].UserID == batch.policyTargets[j].UserID {
			return batch.policyTargets[i].UserTunnelID < batch.policyTargets[j].UserTunnelID
		}
		return batch.policyTargets[i].UserID < batch.policyTargets[j].UserID
	})
	return batch
}

func (h *Handler) applyFlowUploadBatch(nodeID int64, batch flowUploadBatch, now time.Time) {
	if h == nil || h.repo == nil {
		return
	}
	if err := h.repo.ApplyFlowUploadDeltasBatch(batch.flowDeltas); err != nil {
		log.Printf("flow upload write failed op=flow.batch_apply node_id=%d err=%v", nodeID, err)
		return
	}
	quotaViews, err := h.repo.AddUserQuotaUsageBatch(batch.quotaUsage, now)
	if err != nil {
		log.Printf("flow upload write failed op=quota.batch_apply node_id=%d err=%v", nodeID, err)
		return
	}
	for userID, quota := range quotaViews {
		h.enforceUserQuotaIfNeeded(userID, quota)
	}
	for _, target := range batch.policyTargets {
		if target.UserID <= 0 || target.UserTunnelID <= 0 {
			continue
		}
		h.enforceFlowPolicies(target.UserID, target.UserTunnelID)
	}
	for serviceName := range batch.orphanServices {
		h.sendDeleteOrphanedForwardService(nodeID, serviceName)
	}
	for serviceName, item := range batch.peerShareForwardItems {
		forwardID, _, _, ok := parseFlowServiceIDs(serviceName)
		if ok {
			h.processPeerShareFlowFromForward(forwardID, nodeID, serviceName, item)
		}
	}
	for runtimeID, item := range batch.peerShareRuntimeItems {
		h.processPeerShareFlow(runtimeID, item)
	}
}
```

- [ ] **Step 2: Switch the `/flow/upload` entrypoint and tunnel metric ingestion to the shared batch**

Modify `handler.go` and `tunnel_metrics_ingestion.go` so the raw JSON is parsed once and the same forward metadata powers both flow counters and tunnel metrics.

```go
// handler.go
func (h *Handler) flowUpload(w http.ResponseWriter, r *http.Request) {
	secret := r.URL.Query().Get("secret")
	node, _ := h.repo.GetNodeBySecret(secret)
	if node == nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
		return
	}

	raw, err := readAndDecryptFlowBody(r.Body, secret)
	if err == nil && strings.TrimSpace(raw) != "" {
		var items []flowItem
		if json.Unmarshal([]byte(raw), &items) == nil {
			now := time.Now()
			forwardIDs := collectFlowUploadForwardIDs(items)
			metas, metaErr := h.repo.GetFlowUploadForwardMetas(forwardIDs)
			if metaErr != nil {
				log.Printf("flow upload metadata lookup failed node_id=%d err=%v", node.ID, metaErr)
				metas = map[int64]repo.FlowUploadForwardMeta{}
			}
			batch := h.buildFlowUploadBatch(items, metas)
			h.recordTunnelMetricsFromForwardBatch(node.ID, batch.forwardTraffic, metas, now.UnixMilli())
			h.applyFlowUploadBatch(node.ID, batch, now)
		}
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}
```

```go
// tunnel_metrics_ingestion.go
func collectFlowUploadForwardIDs(items []flowItem) []int64 {
	ids := make([]int64, 0, len(items))
	seen := make(map[int64]struct{}, len(items))
	for _, item := range items {
		forwardID, _, _, ok := parseFlowServiceIDs(strings.TrimSpace(item.N))
		if !ok || forwardID <= 0 {
			continue
		}
		if _, exists := seen[forwardID]; exists {
			continue
		}
		seen[forwardID] = struct{}{}
		ids = append(ids, forwardID)
	}
	return ids
}

func (h *Handler) recordTunnelMetricsFromForwardBatch(nodeID int64, forwardDeltas map[int64]tunnelTrafficDelta, metas map[int64]repo.FlowUploadForwardMeta, nowMs int64) {
	if h == nil || h.repo == nil || nodeID <= 0 || len(forwardDeltas) == 0 {
		return
	}
	bucketTs := unixMilliBucketMinute(nowMs)
	if bucketTs <= 0 {
		return
	}
	tunnelAgg := make(map[int64]tunnelTrafficDelta)
	for forwardID, delta := range forwardDeltas {
		meta, ok := metas[forwardID]
		if !ok || meta.TunnelID <= 0 {
			continue
		}
		current := tunnelAgg[meta.TunnelID]
		current.bytesIn += delta.bytesIn
		current.bytesOut += delta.bytesOut
		tunnelAgg[meta.TunnelID] = current
	}
	metrics := make([]*model.TunnelMetric, 0, len(tunnelAgg))
	for tunnelID, delta := range tunnelAgg {
		if delta.bytesIn == 0 && delta.bytesOut == 0 {
			continue
		}
		metrics = append(metrics, &model.TunnelMetric{TunnelID: tunnelID, NodeID: nodeID, Timestamp: bucketTs, BytesIn: delta.bytesIn, BytesOut: delta.bytesOut})
	}
	if len(metrics) == 0 {
		return
	}
	if err := h.repo.UpsertTunnelMetricBuckets(metrics); err != nil {
		log.Printf("monitoring write failed op=tunnel_metric.upsert_buckets node_id=%d bucket_ts=%d count=%d err=%v", nodeID, bucketTs, len(metrics), err)
		return
	}
	log.Printf("monitoring ok op=tunnel_metric.upsert_buckets node_id=%d bucket_ts=%d count=%d", nodeID, bucketTs, len(metrics))
}
```

- [ ] **Step 3: Run focused handler and contract tests to verify GREEN**

Run:

```bash
go test ./internal/http/handler -run TestBuildFlowUploadBatchAggregatesForwardQuotaPeerShareAndCleanupTargets -v
go test ./tests/contract/... -run TestFlowUploadAggregatesRepeatedItemsAndDisablesQuotaImmediately -v
```

Expected: PASS.

- [ ] **Step 4: Run the full backend suite**

Run:

```bash
go test ./...
```

Expected: PASS across the backend module.

- [ ] **Step 5: Optional commit if the user explicitly requested commits**

```bash
git add go-backend/internal/http/handler/handler.go go-backend/internal/http/handler/tunnel_metrics_ingestion.go go-backend/internal/http/handler/flow_upload_batch.go go-backend/internal/http/handler/flow_upload_batch_test.go go-backend/tests/contract/flow_upload_batch_contract_test.go
git commit -m "refactor: batch flow upload processing"
```

---

### Task 4: Final Verification And Performance Sanity Check

**Files:**
- Modify: `go-backend/tests/contract/flow_upload_batch_contract_test.go`

- [ ] **Step 1: Add a same-batch duplicate-item stress assertion**

Extend the contract test with a second request that repeats the same service name multiple times and assert the counters advance by exactly the summed amount.

```go
body, err = json.Marshal([]map[string]interface{}{
	{"n": "20_2_10", "u": 10, "d": 20},
	{"n": "20_2_10", "u": 10, "d": 20},
	{"n": "20_2_10_tcp", "u": 10, "d": 20},
})
if err != nil {
	t.Fatalf("marshal body: %v", err)
}
req = httptest.NewRequest(http.MethodPost, "/flow/upload?secret="+node.Secret, bytes.NewReader(body))
res = httptest.NewRecorder()
router.ServeHTTP(res, req)

if got := mustQueryInt(t, repo, `SELECT in_flow FROM forward WHERE id = 20`); got != 140 {
	t.Fatalf("expected forward in_flow=140 after second request, got %d", got)
}
if got := mustQueryInt(t, repo, `SELECT out_flow FROM forward WHERE id = 20`); got != 140 {
	t.Fatalf("expected forward out_flow=140 after second request, got %d", got)
}
```

- [ ] **Step 2: Run the targeted contract test again**

Run:

```bash
go test ./tests/contract/... -run TestFlowUploadAggregatesRepeatedItemsAndDisablesQuotaImmediately -v
```

Expected: PASS.

- [ ] **Step 3: Re-run the full backend suite before claiming completion**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 4: Optional local profiling sanity check**

Run a short local comparison before and after the change with the same repeated flow payload.

```bash
go test ./tests/contract/... -run TestFlowUploadAggregatesRepeatedItemsAndDisablesQuotaImmediately -count=10
```

Expected: the test remains stable across repeated runs and does not introduce flakiness.

- [ ] **Step 5: Optional commit if the user explicitly requested commits**

```bash
git add go-backend/tests/contract/flow_upload_batch_contract_test.go
git commit -m "test: harden flow upload batch regression coverage"
```
