# Allow Local Remote Address Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a global settings toggle that allows non-admin forward rules to target local/private addresses when explicitly enabled.

**Architecture:** Keep the existing remote-address safety validator as the default path for non-admin rule changes, but gate its use behind a single backend config lookup in forward create/update handlers. Surface the toggle through the existing `vite_config` settings page and prove behavior with backend contract tests first.

**Tech Stack:** Go `net/http` + GORM backend, React + TypeScript frontend settings page, Go contract tests.

---

### Task 1: Backend Contract Coverage

**Files:**
- Modify: `go-backend/tests/contract/forward_contract_test.go`

- [ ] **Step 1: Write the failing tests**

Add contract tests that prove the desired behavior:

```go
t.Run("local remote address is rejected when toggle is off", func(t *testing.T) {
	createPayload := map[string]interface{}{
		"name":       "deny-local-remote",
		"tunnelId":   tunnelID,
		"remoteAddr": "127.0.0.1:8080",
		"strategy":   "fifo",
	}
	createBody, _ := json.Marshal(createPayload)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	router.ServeHTTP(createRes, createReq)

	var out response.R
	_ = json.NewDecoder(createRes.Body).Decode(&out)
	if out.Code == 0 {
		t.Fatalf("expected local remote address to be rejected when toggle is off")
	}
})

t.Run("local remote address is allowed when toggle is on", func(t *testing.T) {
	if err := repo.DB().Exec(`
		INSERT INTO vite_config(name, value, time)
		VALUES(?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET value = excluded.value, time = excluded.time
	`, "allow_local_remote_addr", "1", time.Now().UnixMilli()).Error; err != nil {
		t.Fatalf("enable allow_local_remote_addr: %v", err)
	}

	createPayload := map[string]interface{}{
		"name":       "allow-local-remote",
		"tunnelId":   tunnelID,
		"remoteAddr": "127.0.0.1:8080",
		"strategy":   "fifo",
	}
	createBody, _ := json.Marshal(createPayload)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/forward/create", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	router.ServeHTTP(createRes, createReq)
	assertCode(t, createRes, 0)
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./tests/contract/... -run 'TestForwardContracts|local remote address'`
Expected: FAIL because backend still rejects local/private addresses unconditionally.

- [ ] **Step 3: Commit**

Do not commit yet; combine with Task 2 after implementation passes.

### Task 2: Backend Toggle Implementation

**Files:**
- Modify: `go-backend/internal/http/handler/mutations.go`

- [ ] **Step 1: Add a tiny config helper**

Add a helper near other handler helpers:

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

- [ ] **Step 2: Gate create/update validation behind the helper**

Replace the unconditional checks with:

```go
if !h.allowLocalRemoteAddr() {
	if err := IsSafeRemoteAddr(remoteAddr); err != nil {
		response.WriteJSON(w, response.Err(403, err.Error()))
		return
	}
}
```

- [ ] **Step 3: Run contract tests to verify they pass**

Run: `go test ./tests/contract/... -run 'TestForwardContracts|local remote address'`
Expected: PASS

- [ ] **Step 4: Run full backend tests**

Run: `go test ./...`
Expected: PASS

### Task 3: Settings Page Toggle

**Files:**
- Modify: `vite-frontend/src/pages/config.tsx`

- [ ] **Step 1: Add the config item to the settings schema**

Add a switch-style item for `allow_local_remote_addr` with warning copy about reduced safety.

- [ ] **Step 2: Ensure the key is included in config loading/saving paths**

Add `allow_local_remote_addr` anywhere the page enumerates config keys or groups persisted config values.

- [ ] **Step 3: Run frontend build**

Run: `pnpm run build`
Expected: PASS

- [ ] **Step 4: Run frontend lint**

Run: `pnpm run lint`
Expected: 0 errors; existing warnings may remain.

### Task 4: Final Verification

**Files:**
- Verify only

- [ ] **Step 1: Re-run backend contracts for the toggle**

Run: `go test ./tests/contract/... -run 'TestForwardContracts|local remote address'`
Expected: PASS

- [ ] **Step 2: Re-run full backend tests**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 3: Re-run frontend build/lint**

Run: `pnpm run build && pnpm run lint`
Expected: Build passes, lint has no errors.

- [ ] **Step 4: Commit**

```bash
git add go-backend/internal/http/handler/mutations.go go-backend/tests/contract/forward_contract_test.go vite-frontend/src/pages/config.tsx docs/superpowers/specs/2026-04-26-allow-local-remote-addr-design.md docs/superpowers/plans/2026-04-26-allow-local-remote-addr.md
git commit -m "feat: add allow-local-remote-address toggle"
```
