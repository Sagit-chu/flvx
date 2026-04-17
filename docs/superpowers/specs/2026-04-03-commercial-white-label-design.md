# FLVX 商业版白标授权功能设计方案

## 1. 目标
通过在设置面板中引入商业版激活码（License Key），允许已授权的用户去除前端页面的 FLVX 品牌标识，并使用自己的 App Name、Logo、Favicon 和隐藏版权信息，从而实现“白标”定制。

## 2. 功能范围
*   **授权校验（服务端）**：提供一个激活码输入与验证的接口。初始版本采用**在线 Mock 验证**，后续可通过替换验证服务器地址实现真实的在线发卡与吊销逻辑。
*   **配置存储（服务端）**：一旦授权成功，在数据库（如 `vite_config` 或现有的配置表）中记录授权状态（例如 `license_key`、`is_commercial` 等），并放开商业白标相关字段的写入权限（`app_name`, `app_logo`, `app_favicon`, `hide_footer_brand`）。
*   **权限拦截（服务端）**：拦截未授权用户的请求，禁止他们更新相关的品牌字段。
*   **前端 UI（客户端）**：
    *   在配置页面（或单独的“授权/个性化” Tab）提供激活码输入框。
    *   如果未激活：界面仅展示默认品牌配置，并提示“需要商业授权以解锁自定义品牌”。
    *   如果已激活：展示站名、Logo、Favicon 的上传和替换表单，提供隐藏“Powered by FLVX”脚标的开关。

## 3. 架构设计

### 3.1 数据库/配置结构
扩展配置系统中的以下字段：
*   `license_key` (String)：存储用户激活的商业版密钥。
*   `is_commercial` (String/Boolean)：标识是否为合法的商业授权状态（"true" 或 "false"）。
*   `hide_footer_brand` (String/Boolean)：是否隐藏底部的 FLVX 信息。

注意：现有的 `app_name`, `app_logo`, `app_favicon` 字段将收紧修改权限。

### 3.2 服务端 API 变更
*   **新增 API `POST /api/license/activate` (或将逻辑集成到现有配置修改接口)**：
    *   接收 `{ "license_key": "FLVX-xxxx" }`。
    *   **Mock 逻辑**：如果是 `FLVX-` 开头则视为合法。
    *   合法则更新系统配置，设置 `license_key` 并将状态标为 `is_commercial: "true"`。
*   **修改 API 权限校验（如保存系统设置的接口）**：
    *   当接收到更新 `app_name`、`app_logo`、`app_favicon`、`hide_footer_brand` 的请求时，检查当前系统中的 `is_commercial` 状态。
    *   如果未授权且尝试修改白标字段，返回错误（如 `403 Forbidden`）提示需要商业授权。

### 3.3 前端设计
*   **授权卡片**：在全局设置（Settings / Config）页加入「商业版授权」或「个性化」区块。
*   **表单按需显示**：使用配置中的 `is_commercial === "true"` 来控制相关表单组件的展示：
    *   如果未授权，白标字段（Logo、Favicon、App Name、Hide Footer）不可修改（呈 Disabled）或覆盖了一层“锁”图标。
    *   底部 Footer 组件读取 `hide_footer_brand === "true"` 决定是否渲染 `Powered by FLVX`。
*   **全局状态同步**：当用户激活或上传完 Logo 后，通过现有的 `syncLogo` / `syncFavicon` 等机制全局刷新外观。

## 4. 安全与降级
*   **本地缓存失效**：如果后台在线验证服务器（未来）判断该 key 被吊销，可以在后续获取 config 的接口中重置白标配置为空，强制回退到默认 FLVX 主题。
*   **接口防绕过**：所有跟商业字段相关的变更，必须经过后端 API 的鉴权，确保纯前端绕过是无效的。

## 5. 测试策略
1.  **输入非法激活码**，提示错误，白标设置项仍被锁定。
2.  **输入合法激活码 (`FLVX-...`)**，提示成功，白标设置项解锁。
3.  **成功后上传 Logo 和修改站名**，刷新页面，前端应正常应用新配置且没有 FLVX 标记。
4.  **接口测试**：在未授权状态下，尝试强行通过 API 更新 `app_logo`，接口应返回权限不足。
