# Commercial White-Label Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow users with a valid license key to activate commercial white-label features, enabling them to remove FLVX branding and use their own app name, logos, and footer.
**Architecture:** Backend API handles license validation and stores state (`is_commercial`). Both frontend and backend check this state to conditionally render or allow modifications to brand config.
**Tech Stack:** Go (Backend API), React + Vite (Frontend UI).

---

### Task 1: Backend License Activation Endpoint

**Files:**
- Modify: `go-backend/internal/http/handler/handler.go`

- [ ] **Step 1: Add license request struct**
Add the `licenseActivateRequest` struct in `handler.go`.

```go
type licenseActivateRequest struct {
	LicenseKey string `json:"license_key"`
}
```

- [ ] **Step 2: Add `licenseActivate` handler method**
Add the method to validate the key in `handler.go`.

```go
func (h *Handler) licenseActivate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.WriteJSON(w, response.ErrDefault("请求失败"))
		return
	}

	var req licenseActivateRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		response.WriteJSON(w, response.ErrDefault("授权码不能为空"))
		return
	}
	
	key := strings.TrimSpace(req.LicenseKey)
	if !strings.HasPrefix(key, "FLVX-") {
		response.WriteJSON(w, response.ErrDefault("无效的商业授权码"))
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

- [ ] **Step 3: Register the route**
In `handler.go` inside `Register(mux *http.ServeMux)`, add the route.

```go
	mux.HandleFunc("/api/v1/license/activate", h.licenseActivate)
```

- [ ] **Step 4: Commit**
```bash
git add go-backend/internal/http/handler/handler.go
git commit -m "feat: add license activation endpoint"
```

### Task 2: Backend Config Update Validation

**Files:**
- Modify: `go-backend/internal/http/handler/handler.go`

- [ ] **Step 1: Add permission check in `updateConfigs`**
In `updateConfigs`, fetch `isCommercial := h.repo.GetConfig("is_commercial")`. Inside the loop, check if the user is trying to update protected keys.

```go
	isCommercial, _ := h.repo.GetConfig("is_commercial")
	protectedKeys := map[string]bool{
		"app_name":          true,
		"app_logo":          true,
		"app_favicon":       true,
		"hide_footer_brand": true,
	}
```
Inside `for k, v := range payload`:
```go
		if protectedKeys[key] && isCommercial.Value != "true" {
			response.WriteJSON(w, response.ErrDefault("需要商业版授权"))
			return
		}
```

- [ ] **Step 2: Add permission check in `updateSingleConfig`**
In `updateSingleConfig`, do the same check before calling `normalizeAndValidateConfigValue`.

```go
	isCommercial, _ := h.repo.GetConfig("is_commercial")
	if (name == "app_name" || name == "app_logo" || name == "app_favicon" || name == "hide_footer_brand") && isCommercial.Value != "true" {
		response.WriteJSON(w, response.ErrDefault("需要商业版授权"))
		return
	}
```

- [ ] **Step 3: Commit**
```bash
git add go-backend/internal/http/handler/handler.go
git commit -m "feat: add authorization check for commercial config keys"
```

### Task 3: Frontend API & Site Config Update

**Files:**
- Modify: `vite-frontend/src/api/index.ts`
- Modify: `vite-frontend/src/config/site.ts`

- [ ] **Step 1: Add `activateLicense` API**
In `vite-frontend/src/api/index.ts`:

```typescript
export const activateLicense = (licenseKey: string) =>
  Network.post("/license/activate", { license_key: licenseKey });
```

- [ ] **Step 2: Update `siteConfig` defaults**
In `vite-frontend/src/config/site.ts`, inside `getInitialConfig()`, add properties.

```typescript
    app_logo: cachedAppLogo,
    app_favicon: cachedAppFavicon,
    is_commercial: configCache.get("is_commercial") === "true",
    hide_footer_brand: configCache.get("hide_footer_brand") === "true",
```

- [ ] **Step 3: Update `updateSiteConfig`**
In `updateSiteConfig` inside `site.ts`, extract and update `is_commercial` and `hide_footer_brand`.

```typescript
  const isCommercial = resolvedConfigMap.is_commercial === "true";
  const hideFooterBrand = resolvedConfigMap.hide_footer_brand === "true";
  siteConfig.is_commercial = isCommercial;
  siteConfig.hide_footer_brand = hideFooterBrand;
```

- [ ] **Step 4: Commit**
```bash
git add vite-frontend/src/api/index.ts vite-frontend/src/config/site.ts
git commit -m "feat: add frontend api and update site config state for license"
```

### Task 4: Frontend Footer Component Update

**Files:**
- Modify: `vite-frontend/src/components/version-footer.tsx`

- [ ] **Step 1: Conditionally hide "Powered by FLVX"**
In the render block, wrap the `Powered by FLVX` text.

```tsx
      {siteConfig.hide_footer_brand !== true && (
        <p className={poweredClassName}>
          Powered by{" "}
          <a
            className="text-gray-500 dark:text-gray-400 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
            href={siteConfig.github_repo}
            rel="noopener noreferrer"
            target="_blank"
          >
            FLVX
          </a>
        </p>
      )}
```

- [ ] **Step 2: Commit**
```bash
git add vite-frontend/src/components/version-footer.tsx
git commit -m "feat: conditionally hide flvx footer brand"
```

### Task 5: Frontend Settings Page UI Update

**Files:**
- Modify: `vite-frontend/src/pages/config.tsx`

- [ ] **Step 1: Add config keys to initialization**
In `getInitialConfigs`, add `"is_commercial"` and `"hide_footer_brand"` to `configKeys`.

- [ ] **Step 2: Add `hide_footer_brand` switch field**
Add it to the `CONFIG_ITEMS` array.

```typescript
  {
    key: "hide_footer_brand",
    label: "隐藏页面底部 FLVX 版权信息",
    description: "需商业版授权才能生效",
    type: "switch",
  },
```

- [ ] **Step 3: Add license activation UI**
Above the System Config Card (near `value="configs"`), add a new `Card` for "商业版授权". You will need a local state `licenseKey` and an `handleActivateLicense` function that calls `activateLicense(licenseKey)` and refetches configs on success. 

- [ ] **Step 4: Disable brand settings when not commercial**
In `renderConfigItem`, compute `isDisabled` and pass it to the `<Input>`, `<Switch>`, and `BrandUploading` UI. Update the logic to disable modifications and add a lock icon or a tooltip explaining that a commercial license is required.

```typescript
const isCommercialDisabled = ["app_name", "app_logo", "app_favicon", "hide_footer_brand"].includes(item.key) && configs.is_commercial !== "true";
```

- [ ] **Step 5: Commit**
```bash
git add vite-frontend/src/pages/config.tsx
git commit -m "feat: ui settings for commercial white-label and license activation"
```
