# nftables Traffic Stats Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add traffic accounting for `nftables` forwarding mode by collecting nftables counters over SSH and writing deltas into the existing FLVX flow, quota, policy, and tunnel metric paths.

**Architecture:** The backend remains the only control plane for nftables nodes. Rule rendering adds stable `counter` comments, a collector parses `nft -j list table inet flvx`, repository state stores last absolute counters, and a scheduled job converts deltas into existing flow ledger updates.

**Tech Stack:** Go 1.23-compatible code, net/http handlers, GORM with SQLite/PostgreSQL-compatible tags, existing `internal/runtime/nftables` SSH runner, existing repository and handler flow ingestion helpers.

---

## File Structure

- Modify `go-backend/internal/runtime/nftables/types.go`: add counter constants and sample structs.
- Modify `go-backend/internal/runtime/nftables/renderer.go`: add DNAT counters and forward-chain accounting rules.
- Modify `go-backend/internal/runtime/nftables/renderer_test.go`: cover IPv4/IPv6 accounting rule rendering.
- Create `go-backend/internal/runtime/nftables/collector.go`: parse FLVX counter comments and nft JSON output.
- Create `go-backend/internal/runtime/nftables/collector_test.go`: cover comment parsing and nft JSON parsing.
- Modify `go-backend/internal/runtime/nftables/runner.go`: add `ListTableJSON` support to the runner interface and SSH runner.
- Modify `go-backend/internal/runtime/nftables/runner_test.go`: cover command construction through a fake runner where applicable.
- Modify `go-backend/internal/store/model/model.go`: add `NftCounterState` model.
- Modify `go-backend/internal/store/repo/repository.go`: include `NftCounterState` in auto migration.
- Create `go-backend/internal/store/repo/repository_nft_counter.go`: repository APIs for collection node listing and counter state upsert/read/delete.
- Create `go-backend/internal/store/repo/repository_nft_counter_test.go`: SQLite tests for migration and upsert behavior.
- Modify `go-backend/internal/store/repo/repository_flow.go`: include `user_id` and `user_tunnel_id` in `FlowUploadForwardMeta` for nftables ingestion.
- Modify `go-backend/internal/store/repo/repository_flow_batch_test.go` or add a focused repo test: verify flow upload metadata includes user and user tunnel IDs.
- Create `go-backend/internal/http/handler/nftables_traffic.go`: delta calculation, batch building, and job orchestration.
- Create `go-backend/internal/http/handler/nftables_traffic_test.go`: unit tests for delta rules and batch conversion.
- Modify `go-backend/internal/http/handler/jobs.go`: call the nftables collection job once per minute.
- Modify forward-delete cleanup path in `go-backend/internal/store/repo/repository_mutations.go` or the existing delete handler path: delete counter state for removed forwards.

---

### Task 1: Render Stable nftables Counters

**Files:**
- Modify: `go-backend/internal/runtime/nftables/types.go`
- Modify: `go-backend/internal/runtime/nftables/renderer.go`
- Test: `go-backend/internal/runtime/nftables/renderer_test.go`

- [ ] **Step 1: Add failing renderer tests**

Add tests that assert both DNAT debug counters and forward-chain accounting counters are rendered:

```go
func TestRenderTableIncludesForwardAccountingCounters(t *testing.T) {
	plan := NodePlan{
		NodeID: 7,
		Rules: []Rule{{
			ForwardID:  42,
			InPort:     12345,
			TargetHost: "198.51.100.20",
			TargetPort: 443,
			Protocols:  []string{"tcp", "udp"},
		}},
	}

	got := RenderTable(plan)
	wantLines := []string{
		`tcp dport 12345 counter dnat ip to 198.51.100.20:443 comment "flvx forward:42 dnat tcp"`,
		`udp dport 12345 counter dnat ip to 198.51.100.20:443 comment "flvx forward:42 dnat udp"`,
		`ip daddr 198.51.100.20 tcp dport 443 counter comment "flvx forward:42 to-target tcp"`,
		`ip saddr 198.51.100.20 tcp sport 443 counter comment "flvx forward:42 from-target tcp"`,
		`ip daddr 198.51.100.20 udp dport 443 counter comment "flvx forward:42 to-target udp"`,
		`ip saddr 198.51.100.20 udp sport 443 counter comment "flvx forward:42 from-target udp"`,
	}
	for _, want := range wantLines {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderTable() missing %q\n%s", want, got)
		}
	}
}

func TestRenderTableIncludesIPv6ForwardAccountingCounters(t *testing.T) {
	plan := NodePlan{
		NodeID: 7,
		Rules: []Rule{{
			ForwardID:  43,
			InPort:     12346,
			TargetHost: "2001:db8::20",
			TargetPort: 8443,
			Protocols:  []string{"tcp"},
		}},
	}

	got := RenderTable(plan)
	wantLines := []string{
		`tcp dport 12346 counter dnat ip6 to [2001:db8::20]:8443 comment "flvx forward:43 dnat tcp"`,
		`ip6 daddr 2001:db8::20 tcp dport 8443 counter comment "flvx forward:43 to-target tcp"`,
		`ip6 saddr 2001:db8::20 tcp sport 8443 counter comment "flvx forward:43 from-target tcp"`,
	}
	for _, want := range wantLines {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderTable() missing %q\n%s", want, got)
		}
	}
}
```

- [ ] **Step 2: Run renderer tests and verify failure**

Run: `cd go-backend && go test ./internal/runtime/nftables -run 'TestRenderTableIncludes.*AccountingCounters'`

Expected: FAIL because rendered rules do not yet include `counter` and forward-chain accounting rules.

- [ ] **Step 3: Add direction constants and rendering helpers**

Add to `types.go`:

```go
const (
	CounterDirectionDNAT       = "dnat"
	CounterDirectionToTarget   = "to-target"
	CounterDirectionFromTarget = "from-target"
)
```

Add helpers to `renderer.go`:

```go
func counterComment(forwardID int64, direction, protocol string) string {
	return fmt.Sprintf("flvx forward:%d %s %s", forwardID, direction, protocol)
}

func nftAddressFamily(host string) string {
	trimmed := strings.Trim(strings.TrimSpace(host), "[]")
	ip := net.ParseIP(trimmed)
	if ip != nil && ip.To4() == nil {
		return "ip6"
	}
	return "ip"
}
```

- [ ] **Step 4: Update DNAT rendering**

Change the DNAT line in `RenderTable` to include `counter` and the new comment format:

```go
b.WriteString(fmt.Sprintf("    %s dport %d counter dnat %s to %s comment %q\n",
	protocol,
	rule.InPort,
	nftAddressFamily(rule.TargetHost),
	formatDNATTarget(rule.TargetHost, rule.TargetPort),
	counterComment(rule.ForwardID, CounterDirectionDNAT, protocol),
))
```

- [ ] **Step 5: Add forward-chain accounting rendering**

In the `chain forward` block, iterate sorted rules and normalized protocols:

```go
for _, rule := range sortedRules(plan.Rules) {
	family := nftAddressFamily(rule.TargetHost)
	targetHost := strings.Trim(strings.TrimSpace(rule.TargetHost), "[]")
	for _, protocol := range normalizedProtocols(rule.Protocols) {
		b.WriteString(fmt.Sprintf("    %s daddr %s %s dport %d counter comment %q\n",
			family,
			targetHost,
			protocol,
			rule.TargetPort,
			counterComment(rule.ForwardID, CounterDirectionToTarget, protocol),
		))
		b.WriteString(fmt.Sprintf("    %s saddr %s %s sport %d counter comment %q\n",
			family,
			targetHost,
			protocol,
			rule.TargetPort,
			counterComment(rule.ForwardID, CounterDirectionFromTarget, protocol),
		))
	}
}
```

- [ ] **Step 6: Run renderer tests and full nftables package tests**

Run: `cd go-backend && go test ./internal/runtime/nftables`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add go-backend/internal/runtime/nftables/types.go go-backend/internal/runtime/nftables/renderer.go go-backend/internal/runtime/nftables/renderer_test.go
git commit -m "feat(nftables): render traffic counters"
```

---

### Task 2: Parse nftables Counter Samples

**Files:**
- Create: `go-backend/internal/runtime/nftables/collector.go`
- Test: `go-backend/internal/runtime/nftables/collector_test.go`

- [ ] **Step 1: Write failing parser tests**

Create `collector_test.go` with tests:

```go
func TestParseCounterComment(t *testing.T) {
	sample, ok := ParseCounterComment(`flvx forward:42 to-target tcp`)
	if !ok {
		t.Fatal("expected comment to parse")
	}
	if sample.ForwardID != 42 || sample.Direction != CounterDirectionToTarget || sample.Protocol != "tcp" {
		t.Fatalf("unexpected sample: %#v", sample)
	}
}

func TestParseCounterCommentRejectsUnknownDirection(t *testing.T) {
	if _, ok := ParseCounterComment(`flvx forward:42 dnat tcp`); ok {
		t.Fatal("dnat counters must not be treated as billable samples")
	}
}

func TestParseCounterSamples(t *testing.T) {
	raw := []byte(`{"nftables":[
		{"metainfo":{"json_schema_version":1}},
		{"rule":{"family":"inet","table":"flvx","chain":"forward","expr":[
			{"match":{"left":{"payload":{"protocol":"ip","field":"daddr"}},"op":"==","right":"198.51.100.20"}},
			{"counter":{"packets":3,"bytes":1200}},
			{"comment":"flvx forward:42 to-target tcp"}
		]}},
		{"rule":{"family":"inet","table":"flvx","chain":"forward","expr":[
			{"counter":{"packets":5,"bytes":3400}},
			{"comment":"flvx forward:42 from-target tcp"}
		]}},
		{"rule":{"family":"inet","table":"flvx","chain":"prerouting","expr":[
			{"counter":{"packets":9,"bytes":9999}},
			{"comment":"flvx forward:42 dnat tcp"}
		]}}
	]}`)

	samples, err := ParseCounterSamples(raw)
	if err != nil {
		t.Fatalf("ParseCounterSamples returned error: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("expected 2 billable samples, got %d: %#v", len(samples), samples)
	}
	if samples[0].Bytes != 1200 || samples[0].Packets != 3 {
		t.Fatalf("unexpected first sample: %#v", samples[0])
	}
}
```

- [ ] **Step 2: Run parser tests and verify failure**

Run: `cd go-backend && go test ./internal/runtime/nftables -run 'TestParseCounter'`

Expected: FAIL because parser code does not exist.

- [ ] **Step 3: Implement `CounterSample` and comment parsing**

Create `collector.go`:

```go
package nftables

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

type CounterSample struct {
	ForwardID int64
	Direction string
	Protocol  string
	Bytes     uint64
	Packets   uint64
}

var flvxCounterCommentRE = regexp.MustCompile(`^flvx forward:(\d+) (to-target|from-target) (tcp|udp)$`)

func ParseCounterComment(comment string) (CounterSample, bool) {
	matches := flvxCounterCommentRE.FindStringSubmatch(strings.TrimSpace(comment))
	if len(matches) != 4 {
		return CounterSample{}, false
	}
	forwardID, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil || forwardID <= 0 {
		return CounterSample{}, false
	}
	return CounterSample{
		ForwardID: forwardID,
		Direction: matches[2],
		Protocol:  matches[3],
	}, true
}
```

- [ ] **Step 4: Implement nft JSON parsing**

Add to `collector.go`:

```go
type nftListTable struct {
	Nftables []map[string]json.RawMessage `json:"nftables"`
}

type nftRule struct {
	Family string          `json:"family"`
	Table  string          `json:"table"`
	Chain  string          `json:"chain"`
	Expr   []nftExpression `json:"expr"`
}

type nftExpression struct {
	Counter *struct {
		Packets uint64 `json:"packets"`
		Bytes   uint64 `json:"bytes"`
	} `json:"counter,omitempty"`
	Comment *string `json:"comment,omitempty"`
}

func ParseCounterSamples(raw []byte) ([]CounterSample, error) {
	var table nftListTable
	if err := json.Unmarshal(raw, &table); err != nil {
		return nil, err
	}
	samples := make([]CounterSample, 0)
	for _, item := range table.Nftables {
		rawRule, ok := item["rule"]
		if !ok {
			continue
		}
		var rule nftRule
		if err := json.Unmarshal(rawRule, &rule); err != nil {
			return nil, err
		}
		if rule.Table != "flvx" || rule.Chain != "forward" {
			continue
		}
		var (
			counter *struct {
				Packets uint64 `json:"packets"`
				Bytes   uint64 `json:"bytes"`
			}
			comment string
		)
		for i := range rule.Expr {
			if rule.Expr[i].Counter != nil {
				counter = rule.Expr[i].Counter
			}
			if rule.Expr[i].Comment != nil {
				comment = *rule.Expr[i].Comment
			}
		}
		if counter == nil {
			continue
		}
		sample, ok := ParseCounterComment(comment)
		if !ok {
			continue
		}
		sample.Bytes = counter.Bytes
		sample.Packets = counter.Packets
		samples = append(samples, sample)
	}
	return samples, nil
}
```

- [ ] **Step 5: Run parser tests**

Run: `cd go-backend && go test ./internal/runtime/nftables -run 'TestParseCounter'`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go-backend/internal/runtime/nftables/collector.go go-backend/internal/runtime/nftables/collector_test.go
git commit -m "feat(nftables): parse traffic counters"
```

---

### Task 3: Add SSH Collection API

**Files:**
- Modify: `go-backend/internal/runtime/nftables/runner.go`
- Modify: `go-backend/internal/runtime/nftables/manager.go`
- Test: `go-backend/internal/runtime/nftables/manager_test.go`

- [ ] **Step 1: Add failing manager test**

Add a fake runner test:

```go
type listTableRunner struct {
	script string
	raw    []byte
}

func (r *listTableRunner) ApplyScript(context.Context, SSHConfig, string) error { return nil }
func (r *listTableRunner) Test(context.Context, SSHConfig) error { return nil }
func (r *listTableRunner) ListTableJSON(context.Context, SSHConfig) ([]byte, error) {
	return r.raw, nil
}

func TestManagerCollectCounters(t *testing.T) {
	raw := []byte(`{"nftables":[{"rule":{"family":"inet","table":"flvx","chain":"forward","expr":[{"counter":{"packets":1,"bytes":2}},{"comment":"flvx forward:42 to-target tcp"}]}}]}`)
	manager := NewManager(&listTableRunner{raw: raw})
	samples, err := manager.CollectCounters(context.Background(), SSHConfig{Host: "127.0.0.1", Username: "root", PrivateKey: "unused"})
	if err != nil {
		t.Fatalf("CollectCounters returned error: %v", err)
	}
	if len(samples) != 1 || samples[0].ForwardID != 42 || samples[0].Bytes != 2 {
		t.Fatalf("unexpected samples: %#v", samples)
	}
}
```

- [ ] **Step 2: Run test and verify failure**

Run: `cd go-backend && go test ./internal/runtime/nftables -run TestManagerCollectCounters`

Expected: FAIL because `Runner` lacks `ListTableJSON` and manager lacks `CollectCounters`.

- [ ] **Step 3: Extend `Runner` interface and SSH runner**

Change `Runner` in `runner.go`:

```go
type Runner interface {
	ApplyScript(ctx context.Context, cfg SSHConfig, script string) error
	Test(ctx context.Context, cfg SSHConfig) error
	ListTableJSON(ctx context.Context, cfg SSHConfig) ([]byte, error)
}
```

Add output-capable run helper:

```go
func (r *SSHRunner) ListTableJSON(ctx context.Context, cfg SSHConfig) ([]byte, error) {
	return r.runOutput(ctx, cfg, nftBinary(cfg)+" -j list table inet flvx")
}
```

Refactor `run` so `runOutput` shares SSH setup and captures stdout. Keep existing error messages in Chinese style:

```go
func (r *SSHRunner) runOutput(ctx context.Context, cfg SSHConfig, command string) ([]byte, error) {
	// Same timeout, SSH config, dial, auth, session flow as run.
	// Set session.Stdout to bytes.Buffer and return stdout.Bytes() on success.
}
```

- [ ] **Step 4: Add manager method**

Add to `manager.go`:

```go
func (m *Manager) CollectCounters(ctx context.Context, cfg SSHConfig) ([]CounterSample, error) {
	if err := m.ensureInitialized(); err != nil {
		return nil, err
	}
	raw, err := m.runner.ListTableJSON(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return ParseCounterSamples(raw)
}
```

- [ ] **Step 5: Run nftables package tests**

Run: `cd go-backend && go test ./internal/runtime/nftables`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go-backend/internal/runtime/nftables/runner.go go-backend/internal/runtime/nftables/manager.go go-backend/internal/runtime/nftables/manager_test.go
git commit -m "feat(nftables): collect counters over ssh"
```

---

### Task 4: Persist Counter State

**Files:**
- Modify: `go-backend/internal/store/model/model.go`
- Modify: `go-backend/internal/store/repo/repository.go`
- Create: `go-backend/internal/store/repo/repository_nft_counter.go`
- Test: `go-backend/internal/store/repo/repository_nft_counter_test.go`

- [ ] **Step 1: Write failing repository tests**

Create tests:

```go
func TestNftCounterStateUpsertAndList(t *testing.T) {
	r := openTestRepository(t)
	now := time.Now().UnixMilli()
	input := NftCounterStateInput{
		NodeID:        1,
		ForwardID:     42,
		Protocol:      "tcp",
		Direction:     "to-target",
		RuleHash:      "hash-a",
		Bytes:         100,
		Packets:       5,
		CollectedTime: now,
	}
	if err := r.UpsertNftCounterStates([]NftCounterStateInput{input}, now); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	input.Bytes = 250
	input.Packets = 7
	input.RuleHash = "hash-b"
	if err := r.UpsertNftCounterStates([]NftCounterStateInput{input}, now+1000); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}
	rows, err := r.GetNftCounterStatesByNode(1)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Bytes != 250 || rows[0].RuleHash != "hash-b" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
}

func TestDeleteNftCounterStatesByForward(t *testing.T) {
	r := openTestRepository(t)
	now := time.Now().UnixMilli()
	err := r.UpsertNftCounterStates([]NftCounterStateInput{
		{NodeID: 1, ForwardID: 42, Protocol: "tcp", Direction: "to-target", Bytes: 10, CollectedTime: now},
		{NodeID: 1, ForwardID: 43, Protocol: "tcp", Direction: "to-target", Bytes: 20, CollectedTime: now},
	}, now)
	if err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	if err := r.DeleteNftCounterStatesByForward(42); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	rows, err := r.GetNftCounterStatesByNode(1)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(rows) != 1 || rows[0].ForwardID != 43 {
		t.Fatalf("unexpected rows after delete: %#v", rows)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run: `cd go-backend && go test ./internal/store/repo -run TestNftCounter`

Expected: FAIL because model and methods do not exist.

- [ ] **Step 3: Add model**

Add to `model.go` near `NftRuleBinding`:

```go
type NftCounterState struct {
	ID            int64  `gorm:"primaryKey;autoIncrement"`
	NodeID        int64  `gorm:"column:node_id;not null;uniqueIndex:idx_nft_counter_state_key;index"`
	ForwardID     int64  `gorm:"column:forward_id;not null;uniqueIndex:idx_nft_counter_state_key;index"`
	Protocol      string `gorm:"type:varchar(10);not null;uniqueIndex:idx_nft_counter_state_key"`
	Direction     string `gorm:"type:varchar(20);not null;uniqueIndex:idx_nft_counter_state_key"`
	RuleHash      string `gorm:"column:rule_hash;type:varchar(128);not null;default:''"`
	Bytes         uint64 `gorm:"not null;default:0"`
	Packets       uint64 `gorm:"not null;default:0"`
	CollectedTime int64  `gorm:"column:collected_time;not null;default:0"`
	CreatedTime   int64  `gorm:"column:created_time;not null"`
	UpdatedTime   int64  `gorm:"column:updated_time;not null"`
}

func (NftCounterState) TableName() string { return "nft_counter_state" }
```

- [ ] **Step 4: Add model to auto migration**

In `repository.go` auto migration list, add:

```go
&model.NftCounterState{},
```

- [ ] **Step 5: Implement repository methods**

Create `repository_nft_counter.go`:

```go
package repo

import (
	"errors"
	"strings"

	"go-backend/internal/store/model"

	"gorm.io/gorm/clause"
)

type NftCounterStateInput struct {
	NodeID        int64
	ForwardID     int64
	Protocol      string
	Direction     string
	RuleHash      string
	Bytes         uint64
	Packets       uint64
	CollectedTime int64
}

func (r *Repository) GetNftCounterStatesByNode(nodeID int64) ([]model.NftCounterState, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var rows []model.NftCounterState
	err := r.db.Where("node_id = ?", nodeID).
		Order("forward_id ASC, protocol ASC, direction ASC").
		Find(&rows).Error
	return rows, err
}

func (r *Repository) UpsertNftCounterStates(inputs []NftCounterStateInput, now int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	if len(inputs) == 0 {
		return nil
	}
	rows := make([]model.NftCounterState, 0, len(inputs))
	for _, input := range inputs {
		if input.NodeID <= 0 || input.ForwardID <= 0 {
			continue
		}
		protocol := strings.ToLower(strings.TrimSpace(input.Protocol))
		direction := strings.ToLower(strings.TrimSpace(input.Direction))
		if protocol == "" || direction == "" {
			continue
		}
		rows = append(rows, model.NftCounterState{
			NodeID:        input.NodeID,
			ForwardID:     input.ForwardID,
			Protocol:      protocol,
			Direction:     direction,
			RuleHash:      strings.TrimSpace(input.RuleHash),
			Bytes:         input.Bytes,
			Packets:       input.Packets,
			CollectedTime: input.CollectedTime,
			CreatedTime:   now,
			UpdatedTime:   now,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "node_id"},
			{Name: "forward_id"},
			{Name: "protocol"},
			{Name: "direction"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"rule_hash":      clause.Expr{SQL: "excluded.rule_hash"},
			"bytes":          clause.Expr{SQL: "excluded.bytes"},
			"packets":        clause.Expr{SQL: "excluded.packets"},
			"collected_time": clause.Expr{SQL: "excluded.collected_time"},
			"updated_time":   clause.Expr{SQL: "excluded.updated_time"},
		}),
	}).Create(&rows).Error
}

func (r *Repository) DeleteNftCounterStatesByForward(forwardID int64) error {
	if r == nil || r.db == nil {
		return errors.New("repository not initialized")
	}
	return r.db.Where("forward_id = ?", forwardID).Delete(&model.NftCounterState{}).Error
}
```

If `excluded.column` is not accepted by SQLite through GORM in this repo, replace the `DoUpdates` assignments with direct values from a per-row single upsert loop.

- [ ] **Step 6: Run repository tests**

Run: `cd go-backend && go test ./internal/store/repo -run TestNftCounter`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add go-backend/internal/store/model/model.go go-backend/internal/store/repo/repository.go go-backend/internal/store/repo/repository_nft_counter.go go-backend/internal/store/repo/repository_nft_counter_test.go
git commit -m "feat(nftables): persist counter state"
```

---

### Task 5: Extend Flow Metadata for nftables Ingestion

**Files:**
- Modify: `go-backend/internal/store/repo/repository_flow.go`
- Test: `go-backend/internal/store/repo/repository_flow_batch_test.go` or `go-backend/internal/store/repo/repository_flow_test.go`

- [ ] **Step 1: Write failing metadata test**

Add a focused test that creates a user tunnel, a forward, and then checks `GetFlowUploadForwardMetas` returns `UserID` and `UserTunnelID`:

```go
func TestGetFlowUploadForwardMetasIncludesUserAndUserTunnel(t *testing.T) {
	r := openTestRepository(t)
	now := time.Now().UnixMilli()
	if err := r.db.Create(&model.User{
		ID: 1, User: "meta-user", Pwd: "x", RoleID: 0, ExpTime: now + 86400000,
		Flow: 1000, FlowResetTime: now, Num: 1, CreatedTime: now, Status: 1,
	}).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := r.db.Create(&model.Tunnel{
		ID: 2, Name: "meta-tunnel", Type: 1, Protocol: "tcp", Flow: 2,
		TrafficRatio: 1.5, CreatedTime: now, UpdatedTime: now, Status: 1,
	}).Error; err != nil {
		t.Fatalf("insert tunnel: %v", err)
	}
	if err := r.db.Create(&model.UserTunnel{
		ID: 3, UserID: 1, TunnelID: 2, Num: 1, Flow: 1000,
		FlowResetTime: now, ExpTime: now + 86400000, Status: 1,
	}).Error; err != nil {
		t.Fatalf("insert user tunnel: %v", err)
	}
	if err := r.db.Create(&model.Forward{
		ID: 4, UserID: 1, UserName: "meta-user", Name: "meta-forward",
		TunnelID: 2, RemoteAddr: "198.51.100.20:443", Strategy: "fifo",
		CreatedTime: now, UpdatedTime: now, Status: 1,
	}).Error; err != nil {
		t.Fatalf("insert forward: %v", err)
	}

	metas, err := r.GetFlowUploadForwardMetas([]int64{4})
	if err != nil {
		t.Fatalf("GetFlowUploadForwardMetas failed: %v", err)
	}
	meta := metas[4]
	if meta.UserID != 1 || meta.UserTunnelID != 3 || meta.TunnelID != 2 {
		t.Fatalf("unexpected meta: %#v", meta)
	}
	if meta.TrafficRatio != 1.5 || meta.TunnelFlow != 2 {
		t.Fatalf("unexpected ratio/flow: %#v", meta)
	}
}
```

- [ ] **Step 2: Run metadata test and verify failure**

Run the specific test added in Step 1.

Expected: FAIL because `FlowUploadForwardMeta` does not yet expose `UserID` or `UserTunnelID`.

- [ ] **Step 3: Extend `FlowUploadForwardMeta`**

In `repository_flow.go`, change the struct:

```go
type FlowUploadForwardMeta struct {
	ForwardID    int64
	UserID       int64
	UserTunnelID int64
	TunnelID     int64
	TrafficRatio float64
	TunnelFlow   int64
}
```

- [ ] **Step 4: Extend metadata query**

In `GetFlowUploadForwardMetas`, update the local row type and query:

```go
type row struct {
	ForwardID    int64   `gorm:"column:forward_id"`
	UserID       int64   `gorm:"column:user_id"`
	UserTunnelID int64   `gorm:"column:user_tunnel_id"`
	TunnelID     int64   `gorm:"column:tunnel_id"`
	TrafficRatio float64 `gorm:"column:traffic_ratio"`
	TunnelFlow   int64   `gorm:"column:tunnel_flow"`
}
```

Use this query shape:

```go
err := r.db.Table("forward AS f").
	Select("f.id AS forward_id, f.user_id AS user_id, COALESCE(ut.id, 0) AS user_tunnel_id, f.tunnel_id AS tunnel_id, t.traffic_ratio AS traffic_ratio, t.flow AS tunnel_flow").
	Joins("LEFT JOIN tunnel t ON t.id = f.tunnel_id").
	Joins("LEFT JOIN user_tunnel ut ON ut.user_id = f.user_id AND ut.tunnel_id = f.tunnel_id").
	Where("f.id IN ?", chunk).
	Scan(&rows).Error
```

When assigning `out[row.ForwardID]`, include:

```go
UserID:       row.UserID,
UserTunnelID: row.UserTunnelID,
```

- [ ] **Step 5: Run metadata and existing flow upload tests**

Run: `cd go-backend && go test ./internal/store/repo -run 'FlowUpload|GetFlowUploadForwardMetas'`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go-backend/internal/store/repo/repository_flow.go go-backend/internal/store/repo/repository_flow_batch_test.go go-backend/internal/store/repo/repository_flow_test.go
git commit -m "feat(flow): expose forward owner metadata"
```

Stage only the test file that actually changed.

---

### Task 6: Calculate Deltas and Build Flow Batches

**Files:**
- Create: `go-backend/internal/http/handler/nftables_traffic.go`
- Test: `go-backend/internal/http/handler/nftables_traffic_test.go`

- [ ] **Step 1: Write failing delta tests**

Create tests for first baseline, normal growth, reset, and rule hash change:

```go
func TestBuildNftCounterDeltasSkipsFirstBaseline(t *testing.T) {
	samples := []nftables.CounterSample{{ForwardID: 42, Protocol: "tcp", Direction: nftables.CounterDirectionToTarget, Bytes: 100, Packets: 5}}
	deltas, states := buildNftCounterDeltas(1, samples, nil, map[int64]string{42: "hash-a"}, 1000)
	if len(deltas) != 0 {
		t.Fatalf("expected no deltas for first baseline, got %#v", deltas)
	}
	if len(states) != 1 || states[0].Bytes != 100 || states[0].RuleHash != "hash-a" {
		t.Fatalf("unexpected states: %#v", states)
	}
}

func TestBuildNftCounterDeltasNormalGrowth(t *testing.T) {
	old := []model.NftCounterState{{NodeID: 1, ForwardID: 42, Protocol: "tcp", Direction: nftables.CounterDirectionToTarget, RuleHash: "hash-a", Bytes: 100, Packets: 5}}
	samples := []nftables.CounterSample{{ForwardID: 42, Protocol: "tcp", Direction: nftables.CounterDirectionToTarget, Bytes: 175, Packets: 9}}
	deltas, _ := buildNftCounterDeltas(1, samples, old, map[int64]string{42: "hash-a"}, 2000)
	if len(deltas) != 1 || deltas[0].BytesIn != 75 || deltas[0].BytesOut != 0 {
		t.Fatalf("unexpected deltas: %#v", deltas)
	}
}

func TestBuildNftCounterDeltasResetRefreshesBaseline(t *testing.T) {
	old := []model.NftCounterState{{NodeID: 1, ForwardID: 42, Protocol: "tcp", Direction: nftables.CounterDirectionFromTarget, RuleHash: "hash-a", Bytes: 500, Packets: 10}}
	samples := []nftables.CounterSample{{ForwardID: 42, Protocol: "tcp", Direction: nftables.CounterDirectionFromTarget, Bytes: 20, Packets: 1}}
	deltas, states := buildNftCounterDeltas(1, samples, old, map[int64]string{42: "hash-a"}, 3000)
	if len(deltas) != 0 {
		t.Fatalf("expected no deltas after reset, got %#v", deltas)
	}
	if len(states) != 1 || states[0].Bytes != 20 {
		t.Fatalf("expected refreshed baseline, got %#v", states)
	}
}

func TestBuildNftCounterDeltasRuleHashChangeRefreshesBaseline(t *testing.T) {
	old := []model.NftCounterState{{NodeID: 1, ForwardID: 42, Protocol: "udp", Direction: nftables.CounterDirectionToTarget, RuleHash: "hash-a", Bytes: 100}}
	samples := []nftables.CounterSample{{ForwardID: 42, Protocol: "udp", Direction: nftables.CounterDirectionToTarget, Bytes: 1000}}
	deltas, states := buildNftCounterDeltas(1, samples, old, map[int64]string{42: "hash-b"}, 4000)
	if len(deltas) != 0 {
		t.Fatalf("expected no deltas on hash change, got %#v", deltas)
	}
	if len(states) != 1 || states[0].RuleHash != "hash-b" || states[0].Bytes != 1000 {
		t.Fatalf("unexpected states: %#v", states)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run: `cd go-backend && go test ./internal/http/handler -run TestBuildNftCounterDeltas`

Expected: FAIL because helper does not exist.

- [ ] **Step 3: Implement delta types and helper**

Create `nftables_traffic.go`:

```go
package handler

import (
	"fmt"
	"log"
	"time"

	runtimenft "go-backend/internal/runtime/nftables"
	"go-backend/internal/store/model"
	"go-backend/internal/store/repo"
)

type nftTrafficDelta struct {
	ForwardID int64
	BytesIn   int64
	BytesOut  int64
}

type nftCounterKey struct {
	ForwardID int64
	Protocol  string
	Direction string
}

func nftCounterStateKey(forwardID int64, protocol, direction string) nftCounterKey {
	return nftCounterKey{ForwardID: forwardID, Protocol: protocol, Direction: direction}
}
```

Implement:

```go
func buildNftCounterDeltas(nodeID int64, samples []runtimenft.CounterSample, oldStates []model.NftCounterState, hashes map[int64]string, nowMs int64) ([]nftTrafficDelta, []repo.NftCounterStateInput) {
	oldByKey := make(map[nftCounterKey]model.NftCounterState, len(oldStates))
	for _, state := range oldStates {
		oldByKey[nftCounterStateKey(state.ForwardID, state.Protocol, state.Direction)] = state
	}

	deltaByForward := make(map[int64]nftTrafficDelta)
	newStates := make([]repo.NftCounterStateInput, 0, len(samples))
	for _, sample := range samples {
		if sample.ForwardID <= 0 {
			continue
		}
		hash := hashes[sample.ForwardID]
		key := nftCounterStateKey(sample.ForwardID, sample.Protocol, sample.Direction)
		old, hasOld := oldByKey[key]
		newStates = append(newStates, repo.NftCounterStateInput{
			NodeID:        nodeID,
			ForwardID:     sample.ForwardID,
			Protocol:      sample.Protocol,
			Direction:     sample.Direction,
			RuleHash:      hash,
			Bytes:         sample.Bytes,
			Packets:       sample.Packets,
			CollectedTime: nowMs,
		})
		if !hasOld || old.RuleHash != hash || sample.Bytes < old.Bytes {
			continue
		}
		deltaBytes := sample.Bytes - old.Bytes
		if deltaBytes == 0 {
			continue
		}
		current := deltaByForward[sample.ForwardID]
		current.ForwardID = sample.ForwardID
		switch sample.Direction {
		case runtimenft.CounterDirectionToTarget:
			current.BytesIn += int64(deltaBytes)
		case runtimenft.CounterDirectionFromTarget:
			current.BytesOut += int64(deltaBytes)
		default:
			continue
		}
		deltaByForward[sample.ForwardID] = current
	}

	deltas := make([]nftTrafficDelta, 0, len(deltaByForward))
	for _, delta := range deltaByForward {
		deltas = append(deltas, delta)
	}
	return deltas, newStates
}
```

- [ ] **Step 4: Add batch conversion tests**

Test scaling and tunnel metric raw bytes:

```go
func TestBuildNftFlowUploadBatchScalesFlow(t *testing.T) {
	deltas := []nftTrafficDelta{{ForwardID: 42, BytesIn: 100, BytesOut: 50}}
	metas := map[int64]repo.FlowUploadForwardMeta{
		42: {ForwardID: 42, TunnelID: 8, TrafficRatio: 1.5, TunnelFlow: 2},
	}
	batch := buildNftFlowUploadBatch(deltas, metas)
	if len(batch.flowDeltas) != 1 {
		t.Fatalf("expected one flow delta, got %#v", batch.flowDeltas)
	}
	if batch.flowDeltas[0].InFlow != 300 || batch.flowDeltas[0].OutFlow != 150 {
		t.Fatalf("unexpected scaled flow: %#v", batch.flowDeltas[0])
	}
	if batch.forwardTraffic[42].bytesIn != 100 || batch.forwardTraffic[42].bytesOut != 50 {
		t.Fatalf("unexpected raw tunnel traffic: %#v", batch.forwardTraffic[42])
	}
}
```

- [ ] **Step 5: Implement `buildNftFlowUploadBatch`**

Add:

```go
func buildNftFlowUploadBatch(deltas []nftTrafficDelta, metas map[int64]repo.FlowUploadForwardMeta) flowUploadBatch {
	batch := flowUploadBatch{
		quotaUsage:            make(map[int64]int64),
		forwardTraffic:        make(map[int64]tunnelTrafficDelta),
		orphanServices:        make(map[string]struct{}),
		peerShareForwardItems: make(map[string]flowItem),
		peerShareRuntimeItems: make(map[int64]flowItem),
	}
	policySeen := map[flowPolicyTarget]struct{}{}
	for _, delta := range deltas {
		meta, ok := metas[delta.ForwardID]
		if !ok {
			continue
		}
		raw := batch.forwardTraffic[delta.ForwardID]
		raw.bytesIn += delta.BytesIn
		raw.bytesOut += delta.BytesOut
		batch.forwardTraffic[delta.ForwardID] = raw

		scaledIn := int64(float64(delta.BytesIn)*meta.TrafficRatio) * meta.TunnelFlow
		scaledOut := int64(float64(delta.BytesOut)*meta.TrafficRatio) * meta.TunnelFlow
		batch.flowDeltas = append(batch.flowDeltas, repo.FlowUploadCounterDelta{
			ForwardID:    delta.ForwardID,
			UserID:       meta.UserID,
			UserTunnelID: meta.UserTunnelID,
			InFlow:       scaledIn,
			OutFlow:      scaledOut,
		})
		batch.quotaUsage[meta.UserID] += scaledIn + scaledOut
		target := flowPolicyTarget{UserID: meta.UserID, UserTunnelID: meta.UserTunnelID}
		if _, ok := policySeen[target]; !ok {
			policySeen[target] = struct{}{}
			batch.policyTargets = append(batch.policyTargets, target)
		}
	}
	return batch
}
```

- [ ] **Step 6: Run handler tests**

Run: `cd go-backend && go test ./internal/http/handler -run 'TestBuildNft'`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add go-backend/internal/http/handler/nftables_traffic.go go-backend/internal/http/handler/nftables_traffic_test.go
git commit -m "feat(nftables): calculate traffic deltas"
```

---

### Task 7: Wire Scheduled Collection Job

**Files:**
- Modify: `go-backend/internal/http/handler/nftables_runtime.go`
- Modify: `go-backend/internal/http/handler/handler.go`
- Modify: `go-backend/internal/http/handler/jobs.go`
- Modify: `go-backend/internal/http/handler/nftables_traffic.go`
- Modify: `go-backend/internal/store/repo/repository_nft_counter.go`
- Test: `go-backend/internal/http/handler/nftables_traffic_test.go`

- [ ] **Step 1: Extend handler manager interface**

In `nftables_runtime.go`, add to `nftablesRuntimeManager`:

```go
CollectCounters(ctx context.Context, cfg runtimenft.SSHConfig) ([]runtimenft.CounterSample, error)
```

Update tests with fake managers to implement the method:

```go
func (m *fakeNftablesManager) CollectCounters(context.Context, runtimenft.SSHConfig) ([]runtimenft.CounterSample, error) {
	return m.samples, m.collectErr
}
```

- [ ] **Step 2: Add repository node listing**

Add to `repository_nft_counter.go`:

```go
type NftablesCollectionNode struct {
	NodeID int64
	Config model.NodeSSHConfig
}

func (r *Repository) ListNftablesNodesForCollection() ([]NftablesCollectionNode, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("repository not initialized")
	}
	var rows []struct {
		NodeID int64 `gorm:"column:node_id"`
		model.NodeSSHConfig
	}
	err := r.db.Table("node").
		Select("node.id AS node_id, node_ssh_config.*").
		Joins("JOIN node_ssh_config ON node_ssh_config.node_id = node.id").
		Where("LOWER(TRIM(node.forward_mode)) = ?", "nftables").
		Where("node.status = 1").
		Order("node.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]NftablesCollectionNode, 0, len(rows))
	for _, row := range rows {
		out = append(out, NftablesCollectionNode{NodeID: row.NodeID, Config: row.NodeSSHConfig})
	}
	return out, nil
}
```

- [ ] **Step 3: Add collection job implementation**

Add to `nftables_traffic.go`:

```go
func (h *Handler) runNftablesTrafficCollectJob(now time.Time) {
	if h == nil || h.repo == nil || h.nftablesManager == nil {
		return
	}
	nodes, err := h.repo.ListNftablesNodesForCollection()
	if err != nil {
		log.Printf("nftables traffic collect list failed err=%v", err)
		return
	}
	for _, node := range nodes {
		h.collectNftablesNodeTraffic(node.NodeID, &node.Config, now)
	}
}

func (h *Handler) collectNftablesNodeTraffic(nodeID int64, cfgModel *model.NodeSSHConfig, now time.Time) {
	sshCfg, err := sshConfigFromModel(cfgModel)
	if err != nil {
		log.Printf("nftables traffic collect ssh config invalid node_id=%d err=%v", nodeID, err)
		return
	}
	samples, err := h.nftablesManager.CollectCounters(context.Background(), sshCfg)
	if err != nil {
		log.Printf("nftables traffic collect failed node_id=%d err=%v", nodeID, err)
		return
	}
	oldStates, err := h.repo.GetNftCounterStatesByNode(nodeID)
	if err != nil {
		log.Printf("nftables traffic state load failed node_id=%d err=%v", nodeID, err)
		return
	}
	bindings, err := h.repo.ListNftRuleBindingsByNode(nodeID)
	if err != nil {
		log.Printf("nftables binding load failed node_id=%d err=%v", nodeID, err)
		return
	}
	hashes := make(map[int64]string, len(bindings))
	for _, binding := range bindings {
		hashes[binding.ForwardID] = binding.RuleHash
	}
	nowMs := now.UnixMilli()
	deltas, newStates := buildNftCounterDeltas(nodeID, samples, oldStates, hashes, nowMs)
	if err := h.repo.UpsertNftCounterStates(newStates, nowMs); err != nil {
		log.Printf("nftables traffic state save failed node_id=%d err=%v", nodeID, err)
		return
	}
	if len(deltas) == 0 {
		return
	}
	forwardIDs := make([]int64, 0, len(deltas))
	for _, delta := range deltas {
		forwardIDs = append(forwardIDs, delta.ForwardID)
	}
	metas, err := h.repo.GetFlowUploadForwardMetas(forwardIDs)
	if err != nil {
		log.Printf("nftables traffic metadata lookup failed node_id=%d err=%v", nodeID, err)
		return
	}
	batch := buildNftFlowUploadBatch(deltas, metas)
	h.recordTunnelMetricsFromForwardBatch(nodeID, batch.forwardTraffic, metas, nowMs)
	h.applyFlowUploadBatch(nodeID, batch, now)
}
```

Run sequential collection first. Add worker concurrency only after the sequential version is verified; this keeps first implementation deterministic.

- [ ] **Step 4: Call job once per minute**

In `jobs.go`, find the ticker loop where `runStatisticsFlowJob` is called. Add:

```go
h.runNftablesTrafficCollectJob(time.Now())
```

Use the same minute cadence as statistics or monitoring jobs. Do not call it on every request.

- [ ] **Step 5: Run handler and repo focused tests**

Run: `cd go-backend && go test ./internal/http/handler ./internal/store/repo -run 'Nft|TestBuildNft'`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go-backend/internal/http/handler/nftables_runtime.go go-backend/internal/http/handler/handler.go go-backend/internal/http/handler/jobs.go go-backend/internal/http/handler/nftables_traffic.go go-backend/internal/http/handler/nftables_traffic_test.go go-backend/internal/store/repo/repository_nft_counter.go
git commit -m "feat(nftables): ingest traffic counters"
```

---

### Task 8: Cleanup Counter State on Forward Delete

**Files:**
- Modify: existing forward delete path in `go-backend/internal/store/repo/repository_mutations.go` or `go-backend/internal/http/handler/mutations.go`
- Test: relevant existing mutation test file or `go-backend/internal/store/repo/repository_nft_counter_test.go`

- [ ] **Step 1: Locate forward delete implementation**

Run: `cd go-backend && rg -n "Delete.*Forward|forwardDelete|DeleteForward|DeleteNftRuleBindingsByForward" internal/http/handler internal/store/repo`

Expected: find the method that deletes forward rows and existing nft rule bindings.

- [ ] **Step 2: Add failing cleanup test**

In the closest existing forward delete test, create a forward, insert a `nft_counter_state` row for it, delete the forward through the same repo/handler path, and assert state is gone:

```go
states, err := r.GetNftCounterStatesByNode(nodeID)
if err != nil {
	t.Fatalf("list counter states: %v", err)
}
for _, state := range states {
	if state.ForwardID == forwardID {
		t.Fatalf("counter state for deleted forward still exists: %#v", state)
	}
}
```

- [ ] **Step 3: Run test and verify failure**

Run the specific test from Step 2.

Expected: FAIL because delete path does not clean `nft_counter_state`.

- [ ] **Step 4: Add cleanup call**

Where `DeleteNftRuleBindingsByForward(forwardID)` or forward-row deletion currently happens, add:

```go
if err := r.DeleteNftCounterStatesByForward(forwardID); err != nil {
	return err
}
```

If the delete path is in handler and only has `h.repo`, add:

```go
if err := h.repo.DeleteNftCounterStatesByForward(forwardID); err != nil {
	response.WriteJSON(w, response.ErrDefault(err.Error()))
	return
}
```

Prefer repository transaction cleanup if the existing delete path already uses a transaction.

- [ ] **Step 5: Run cleanup test**

Run the specific test from Step 2.

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go-backend/internal/store/repo/repository_mutations.go go-backend/internal/http/handler/mutations.go go-backend/internal/store/repo/repository_nft_counter_test.go
git commit -m "fix(nftables): clean counter state on forward delete"
```

Stage only files that actually changed.

---

### Task 9: Full Verification

**Files:**
- All changed Go files

- [ ] **Step 1: Run full backend test suite**

Run: `cd go-backend && go test ./...`

Expected: PASS.

- [ ] **Step 2: Inspect git diff**

Run: `git diff --stat HEAD`

Expected: only nftables traffic stats implementation files are changed.

- [ ] **Step 3: Review generated nftables script manually**

Run focused renderer test with verbose output if needed:

```bash
cd go-backend && go test ./internal/runtime/nftables -run TestRenderTableIncludesForwardAccountingCounters -v
```

Expected: PASS. Confirm rendered strings include `counter comment "flvx forward:<id> to-target <protocol>"` and `from-target`.

- [ ] **Step 4: Final commit if any verification-only fixes were needed**

```bash
git add <changed-files>
git commit -m "test(nftables): verify traffic stats"
```

Only commit if Step 1-3 required additional changes.

---

## Self-Review Notes

- Spec coverage: rule rendering, comment parsing, SSH collection, counter state, delta handling, flow/quota/policy/tunnel metric integration, delete cleanup, and tests are all covered.
- Scope intentionally excludes UI采集状态、manual collect button、域名目标 DNS 固化和采集并发优化；这些 are second/third phase items from the spec.
- The plan starts sequential collection for determinism. If node count creates visible SSH pressure, add bounded concurrency in a follow-up after tests pass.
