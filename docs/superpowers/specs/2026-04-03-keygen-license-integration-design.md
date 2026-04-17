# FLVX 商业版 Keygen.sh 授权集成设计方案

## 1. 目标
使用 [Keygen.sh](https://keygen.sh/) 替换当前 FLVX 中基于 Mock 的商业版授权验证逻辑。通过接入 Keygen.sh，实现安全、可控的许可证分发、设备绑定（防止一码多用）、定期验证以及远程吊销功能，为 FLVX 的商业化白标功能提供生产级支持。

## 2. Keygen.sh 核心概念映射
*   **Account (账户)**：您在 Keygen 注册的商户账号。
*   **Product (产品)**：在 Keygen 中创建一个名为 `FLVX Panel` 的产品。
*   **Policy (策略)**：定义授权规则。例如，创建一个 `White-Label Policy`，限制每个 License 只能绑定 **1 个 Machine**（即一个 FLVX 面板实例），并可配置有效期（如按年订阅或永久有效）。
*   **License (许可证)**：发给客户的激活码（Key），格式可自定义（如 `FLVX-XXXX-XXXX`）。
*   **Machine (机器/设备)**：运行 FLVX 的具体服务器或面板实例。为了防止一码多开，FLVX 激活时需要向 Keygen 注册一台 Machine。

## 3. 架构设计与集成流程

### 3.1 唯一设备标识 (Machine Fingerprint)
为了在 Keygen 中标识不同的 FLVX 面板，FLVX 后端需要生成并持久化一个唯一的机器指纹（Fingerprint）。
*   **生成时机**：FLVX 首次启动或首次激活时，生成一个 UUID v4。
*   **存储**：保存在数据库 `vite_config` 表中，键名为 `machine_fingerprint`。

### 3.2 激活流程 (License Activation)
当用户在前端输入激活码并点击“激活”时：
1.  **FLVX 后端验证 Key**：调用 Keygen API `POST /v1/accounts/{account}/licenses/actions/validate-key`，传入 `key`。
2.  **检查 License 状态**：如果返回 `valid: true`，说明 License 合法且未过期。
3.  **激活 Machine (设备绑定)**：
    *   调用 Keygen API `POST /v1/accounts/{account}/machines`。
    *   关联刚才验证的 `licenseId`，并传入 FLVX 的 `machine_fingerprint`。
    *   *异常处理*：如果该 License 已绑定了其他 Machine（达到 Policy 上限），Keygen 会报错，FLVX 后端需返回“该授权码已在其他设备使用”。
4.  **持久化状态**：激活成功后，在本地数据库保存 `license_key`、`is_commercial: "true"`，以及从 Keygen 返回的额外信息（如过期时间 `license_expiry`）。

### 3.3 定期心跳与验证 (Periodic Validation)
为了防止用户激活后断网或通过修改数据库绕过，以及实现**远程吊销**：
*   **定时任务**：FLVX 后端增加一个后台协程（如每天运行一次，或每 12 小时运行一次）。
*   **验证逻辑**：调用 Keygen API 验证当前的 `license_key` 和 `machine_fingerprint`。
*   **吊销/过期处理**：如果 Keygen 明确返回 License 已吊销（Suspended/Revoked/Banned）或已过期，或者当前 Machine 不再属于该 License，FLVX 后端需将 `is_commercial` 强制设为 `"false"`，并清空本地缓存，恢复官方品牌展示。
*   **宽限期 (Grace Period)**：考虑到用户服务器可能偶尔网络不通，如果请求 Keygen 超时或失败，不应立刻吊销。可设置一个宽限期（如连续 3 天请求失败才降级）。

## 4. 后端 API 改造计划 (`go-backend`)

### 4.1 新增环境变量/配置
*   `KEYGEN_ACCOUNT_ID`: 您的 Keygen 账户 ID（打包时可硬编码，或作为全局环境变量）。
*   （可选）`KEYGEN_PRODUCT_TOKEN` 或仅使用 License Key 进行验证（取决于 Keygen 验证方式的选择，推荐直接使用 License Key 进行无状态验证）。

### 4.2 改造 `/api/v1/license/activate`
*   引入 HTTP 客户端向 `api.keygen.sh` 发起请求。
*   实现上述提到的 Validate Key 和 Activate Machine 两步走逻辑。
*   返回具体的错误信息给前端（例如：“授权码不存在”、“授权码已过期”、“激活设备数达上限”）。

## 5. 前端改造计划 (`vite-frontend`)
前端在目前的 UI 基础上几乎不需要大改，只需配合后端的增强：
1.  **展示过期时间**：如果后端返回了 `license_expiry`，可以在“商业版授权”卡片中展示“授权有效期至：YYYY-MM-DD”。
2.  **错误提示优化**：透传后端返回的 Keygen 验证错误，给予用户明确的指引。
3.  **解绑/停用功能（可选）**：未来可增加“停用授权”按钮，调用后端接口在 Keygen 中删除 Machine 绑定，以便用户将 License 迁移到新的服务器。

## 6. 实施步骤建议
1.  在 Keygen.sh 注册账号，创建 Product 和 Policy，生成测试用的 License Key。
2.  在 FLVX 的 `go-backend` 中新建一个 `pkg/keygen` 或 `internal/license` 包，封装 Keygen API 的调用（Validate, Activate Machine）。
3.  修改现有的 `licenseActivate` 接口，接入真正的验证逻辑。
4.  添加定期验证的 Cron Job。
5.  测试激活、吊销、过期、断网等各种场景。