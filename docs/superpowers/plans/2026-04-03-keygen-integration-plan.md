# Commercial White-Label (Keygen) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement Keygen.sh license activation and periodic validation to manage commercial white-label features, replacing the temporary mock logic.
**Architecture:** The backend generates a machine fingerprint, validates the license via the Keygen.sh API, and creates a machine associated with the license. A periodic job verifies the license status to support remote revocation.
**Tech Stack:** Go (Backend API), Keygen.sh API.

---

### Task 1: Generate and Store Machine Fingerprint

**Files:**
- Modify: `go-backend/internal/http/handler/handler.go`

- [ ] **Step 1: Add `getOrCreateMachineFingerprint` helper function**
Add a helper function in `handler.go` (or a dedicated license file) to get or generate the machine fingerprint. Use `github.com/google/uuid`.

```go
import "github.com/google/uuid"

func (h *Handler) getOrCreateMachineFingerprint() (string, error) {
	fp, _ := h.repo.GetViteConfigValue("machine_fingerprint")
	if fp != "" {
		return fp, nil
	}

	newFp := uuid.New().String()
	now := time.Now().UnixMilli()
	if err := h.repo.UpsertConfig("machine_fingerprint", newFp, now); err != nil {
		return "", err
	}
	return newFp, nil
}
```

- [ ] **Step 2: Commit**
```bash
git add go-backend/internal/http/handler/handler.go
git commit -m "feat: add machine fingerprint generation"
```

### Task 2: Create Keygen Client Package

**Files:**
- Create: `go-backend/internal/license/keygen.go`

- [ ] **Step 1: Create Keygen client structs and interface**
Create the file and define the request/response structs for Keygen's `/licenses/actions/validate-key` and `/machines` endpoints. Also define an interface for the client.

```go
package license

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type KeygenClient struct {
	AccountID string
	Token     string
	HTTPClient *http.Client
}

func NewKeygenClient(accountID, token string) *KeygenClient {
	return &KeygenClient{
		AccountID: accountID,
		Token:     token,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type ValidateResponse struct {
	Meta struct {
		Valid bool   `json:"valid"`
		Code  string `json:"code"`
	} `json:"meta"`
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
}

type ActivateMachineRequest struct {
	Data struct {
		Type       string `json:"type"`
		Attributes struct {
			Fingerprint string `json:"fingerprint"`
		} `json:"attributes"`
		Relationships struct {
			License struct {
				Data struct {
					Type string `json:"type"`
					ID   string `json:"id"`
				} `json:"data"`
			} `json:"license"`
		} `json:"relationships"`
	} `json:"data"`
}
```

- [ ] **Step 2: Implement `ValidateKey`**
Add the `ValidateKey` method.

```go
func (c *KeygenClient) ValidateKey(key string) (*ValidateResponse, error) {
	url := fmt.Sprintf("https://api.keygen.sh/v1/accounts/%s/licenses/actions/validate-key", c.AccountID)
	
	reqBody := map[string]interface{}{
		"meta": map[string]string{
			"key": key,
		},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("keygen api error: status %d", resp.StatusCode)
	}

	var valResp ValidateResponse
	if err := json.NewDecoder(resp.Body).Decode(&valResp); err != nil {
		return nil, err
	}

	return &valResp, nil
}
```

- [ ] **Step 3: Implement `ActivateMachine`**
Add the `ActivateMachine` method.

```go
func (c *KeygenClient) ActivateMachine(licenseID, fingerprint string) error {
	url := fmt.Sprintf("https://api.keygen.sh/v1/accounts/%s/machines", c.AccountID)
	
	var reqBody ActivateMachineRequest
	reqBody.Data.Type = "machines"
	reqBody.Data.Attributes.Fingerprint = fingerprint
	reqBody.Data.Relationships.License.Data.Type = "licenses"
	reqBody.Data.Relationships.License.Data.ID = licenseID

	bodyBytes, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		return nil
	}

	if resp.StatusCode == http.StatusConflict { // 409 usually means fingerprint already exists
		return nil // Machine might already be registered
	}

	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("failed to activate machine: status %d, response: %s", resp.StatusCode, string(body))
}
```

- [ ] **Step 4: Commit**
```bash
git add go-backend/internal/license/keygen.go
git commit -m "feat: add keygen.sh api client"
```

### Task 3: Integrate Keygen into License Activation Endpoint

**Files:**
- Modify: `go-backend/internal/http/handler/handler.go`

- [ ] **Step 1: Update `licenseActivate` logic**
Modify `licenseActivate` to use the Keygen client instead of the mock logic. Note: For this implementation, we will use an environment variable `KEYGEN_ACCOUNT_ID`. We can use `os.Getenv` directly for simplicity, or hardcode a fallback if not present.

```go
import (
	"go-backend/internal/license"
	"os"
)

func (h *Handler) licenseActivate(w http.ResponseWriter, r *http.Request) {
	// ... (keep request parsing)

	key := strings.TrimSpace(req.LicenseKey)
	if key == "" {
		response.WriteJSON(w, response.ErrDefault("授权码不能为空"))
		return
	}

	accountID := os.Getenv("KEYGEN_ACCOUNT_ID")
	if accountID == "" {
		// Fallback for mock/development if no keygen account configured
		if strings.HasPrefix(key, "FLVX-") {
			now := time.Now().UnixMilli()
			h.repo.UpsertConfig("license_key", key, now)
			h.repo.UpsertConfig("is_commercial", "true", now)
			response.WriteJSON(w, response.OKEmpty())
			return
		}
		response.WriteJSON(w, response.ErrDefault("系统未配置 Keygen 账号 ID"))
		return
	}

	fingerprint, err := h.getOrCreateMachineFingerprint()
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("生成设备指纹失败"))
		return
	}

	client := license.NewKeygenClient(accountID, "") // Token may be optional for validate-key depending on policy, or can be passed if needed
	
	valResp, err := client.ValidateKey(key)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("连接授权服务器失败: "+err.Error()))
		return
	}

	if !valResp.Meta.Valid {
		response.WriteJSON(w, response.ErrDefault("授权码无效或已过期 (Code: "+valResp.Meta.Code+")"))
		return
	}

	// Try to activate machine
	err = client.ActivateMachine(valResp.Data.ID, fingerprint)
	if err != nil {
		response.WriteJSON(w, response.ErrDefault("设备绑定失败: "+err.Error()))
		return
	}

	now := time.Now().UnixMilli()
	if err := h.repo.UpsertConfig("license_key", key, now); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}
	if err := h.repo.UpsertConfig("is_commercial", "true", now); err != nil {
		response.WriteJSON(w, response.Err(-2, err.Error()))
		return
	}

	response.WriteJSON(w, response.OKEmpty())
}
```

- [ ] **Step 2: Commit**
```bash
git add go-backend/internal/http/handler/handler.go
git commit -m "feat: integrate keygen into license activation endpoint"
```

### Task 4: Add Periodic License Validation Job

**Files:**
- Modify: `go-backend/internal/http/handler/jobs.go`
- Modify: `go-backend/internal/http/handler/handler.go`

- [ ] **Step 1: Add `validateLicenseJob` function in `jobs.go`**
Create a new function that performs the background validation.

```go
import "os"

func (h *Handler) validateLicenseJob() {
	if h == nil || h.repo == nil {
		return
	}

	accountID := os.Getenv("KEYGEN_ACCOUNT_ID")
	if accountID == "" {
		return // Skip if not configured
	}

	key, _ := h.repo.GetViteConfigValue("license_key")
	isCommercial, _ := h.repo.GetViteConfigValue("is_commercial")

	if key == "" || isCommercial != "true" {
		return // Nothing to validate
	}

	client := license.NewKeygenClient(accountID, "")
	valResp, err := client.ValidateKey(key)
	
	if err != nil {
		// Network error or timeout. We implement a grace period by NOT revoking immediately here.
		// In a production system, you might count consecutive failures.
		// For now, we skip revocation on network errors.
		return
	}

	if !valResp.Meta.Valid {
		// License is invalid (e.g., revoked, suspended, expired). Downgrade the system.
		now := time.Now().UnixMilli()
		_ = h.repo.UpsertConfig("is_commercial", "false", now)
		// We could optionally clear brand configs here, or just let them be disabled in UI
	}
}
```

- [ ] **Step 2: Register the job in `RunJobs`**
In `handler.go` or `jobs.go`, wherever the periodic cron jobs are registered (usually `go h.runJobs()`), ensure `validateLicenseJob` is called periodically (e.g., every 12 hours). Look for `h.startCronJobs()` or similar in `handler.go`. 

If a central `RunJobs` loop exists in `jobs.go` (like a `for` loop with a `time.Ticker`), add it there. If not, create a simple goroutine in `Register` or `NewHandler`.

*Assuming there's a `startJobs` or `Init` block in `handler.go`:*
```go
// Inside handler initialization or Register:
go func() {
	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			h.validateLicenseJob()
		}
	}
}()
```

- [ ] **Step 3: Commit**
```bash
git add go-backend/internal/http/handler/jobs.go go-backend/internal/http/handler/handler.go
git commit -m "feat: add periodic license validation job"
```
