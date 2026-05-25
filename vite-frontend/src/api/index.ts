import type {
  BatchOperationResult,
  ForwardDiagnosisApiData,
  ForwardApiItem,
  GroupPermissionApiItem,
  NodeReleaseApiItem,
  NodeApiItem,
  SpeedLimitApiItem,
  TunnelBatchDeletePreviewApiData,
  TunnelBatchDeleteWithForwardsApiData,
  TunnelDeletePreviewApiData,
  TunnelDeleteWithForwardsApiData,
  TunnelDiagnosisApiData,
  TunnelGroupApiItem,
  UserApiItem,
  UserGroupApiItem,
  UserListQuery,
  UserPackageInfoApiData,
  UserTunnelPermissionApiItem,
  TunnelApiItem,
  UserTunnelApiItem,
  UserMutationPayload,
  NodeMutationPayload,
  TunnelMutationPayload,
  UserQuotaResetPayload,
  UserTunnelAssignPayload,
  UserTunnelListQuery,
  UserTunnelRemovePayload,
  ForwardMutationPayload,
  SpeedLimitMutationPayload,
  UpdatePasswordPayload,
  BackupImportPayload,
  NodeMetricApiItem,
  TunnelMetricApiItem,
  ServiceMonitorApiItem,
  ServiceMonitorResultApiItem,
  ServiceMonitorLimitsApiData,
  ServiceMonitorMutationPayload,
  MonitorNodeApiItem,
  MonitorTunnelApiItem,
  MonitorPermissionApiItem,
  MonitorAccessApiData,
  TunnelQualityApiItem,
  StorageSummaryApiData,
  SystemUpgradeVersionApiData,
} from "./types";

import axios from "axios";

import Network from "./network";

export type ReleaseChannel = "stable" | "dev";

// 登陆相关接口
export interface LoginData {
  username: string;
  password: string;
  captchaId: string;
}

export interface LoginResponse {
  token: string;
  role_id: number;
  name: string;
  requirePasswordChange?: boolean;
}

export interface RegisterData {
  username: string;
  password: string;
  inviteCode?: string;
  captchaId?: string;
}

export interface CommerceSettings {
  registrationEnabled?: boolean;
  inviteRequired?: boolean;
  captchaEnabled?: boolean;
  epayEnabled?: boolean;
  usdtEnabled?: boolean;
  [key: string]: unknown;
}

export interface LegalPagesApiData {
  terms: string;
  privacy: string;
  refundPolicy: string;
  acceptableUse: string;
}

export interface PlanApiItem {
  id: number;
  name: string;
  description: string;
  category?: string;
  scopeKey?: string;
  priceCents: number;
  resetFlowPriceCents: number;
  currency: string;
  durationDays: number;
  flow: number;
  dailyQuotaGB: number;
  monthlyQuotaGB: number;
  num: number;
  maxConn: number;
  speedId?: number | null;
  sort: number;
  status: number;
  tunnelIds: number[];
  tunnelGroupIds: number[];
  tunnelNames?: string[];
  tunnelGroupNames?: string[];
  tunnels?: Array<{ id: number; name: string; trafficRatio: number }>;
}

export interface UserSubscriptionApiItem {
  id: number;
  userId: number;
  planId: number;
  orderId: number;
  status: string;
  startsAt: number;
  expiresAt: number;
  planName?: string;
  planCategory?: string;
  planScopeKey?: string;
  planPriceCents?: number;
  planFlow?: number;
  planNum?: number;
  planMaxConn?: number;
  planTunnels?: Array<{ id: number; name: string; trafficRatio: number }>;
  resetFlowPriceCents?: number;
  resetFlowName?: string;
}

export interface InviteCodeApiItem {
  id: number;
  code: string;
  maxUses: number;
  usedCount: number;
  expTime: number;
  status: number;
  createdTime: number;
}

export interface CommerceOrderApiItem {
  id: number;
  orderNo: string;
  userId: number;
  planId: number;
  amountCents: number;
  currency: string;
  status: string;
  paymentStatus?: string;
  fulfillmentStatus?: string;
  refundStatus?: string;
  refundAmountCents?: number;
  refundReason?: string;
  orderType?: string;
  paymentProvider: string;
  providerTradeNo: string;
  planName?: string;
  paidTime: number;
  provisionedTime: number;
  createdTime: number;
  updatedTime?: number;
  canPay?: boolean;
  canCancel?: boolean;
  canRefund?: boolean;
}

export interface CommercePaymentApiItem {
  id: number;
  orderId: number;
  orderNo: string;
  provider: string;
  providerTradeNo: string;
  amountCents: number;
  status: string;
  rawPayload?: string;
  createdTime: number;
}

export interface CommerceAuditApiItem {
  id: number;
  actorId: number;
  actorName: string;
  action: string;
  targetType: string;
  targetId: number;
  summary: string;
  payload: string;
  createdTime: number;
}

export interface RefundRequestApiItem {
  id: number;
  orderId: number;
  orderNo: string;
  userId: number;
  amountCents: number;
  reason: string;
  adminNote: string;
  status: string;
  createdTime: number;
}

export interface SupportTicketApiItem {
  id: number;
  userId: number;
  title: string;
  category?: string;
  status: string;
  priority: string;
  internalNote?: string;
  createdTime: number;
  updatedTime: number;
}

export interface SupportTicketMessageApiItem {
  id: number;
  ticketId: number;
  userId: number;
  isAdmin: number;
  content: string;
  attachmentUrl?: string;
  createdTime: number;
}

export interface NotificationApiItem {
  id: number;
  userId: number;
  title: string;
  content: string;
  level: string;
  readTime: number;
  createdTime: number;
}

export interface CouponApiItem {
  id: number;
  code: string;
  name: string;
  discountType: "fixed" | "percent";
  discountValue: number;
  planId: number;
  category?: string;
  minAmountCents?: number;
  perUserLimit?: number;
  maxUses: number;
  usedCount: number;
  expTime: number;
  status: number;
}

export interface WalletLedgerApiItem {
  id: number;
  userId: number;
  amountCents: number;
  balanceAfterCents: number;
  type: string;
  refType: string;
  refId: number;
  note: string;
  createdTime: number;
}

export interface CommerceReportSummaryApiData {
  paidTotalCents: number;
  paidMonthCents: number;
  refundTotalCents: number;
  walletBalanceCents: number;
  ordersToday: number;
  pendingOrders: number;
  activeSubscriptions: number;
  openTickets: number;
  pendingRefunds: number;
}

export interface CommerceRiskApiItem {
  userId: number;
  type: string;
  level: string;
  summary: string;
  count: number;
}

export interface CommerceResourceJobApiItem {
  id: number;
  jobType: string;
  userId: number;
  orderId: number;
  status: string;
  attempts: number;
  maxAttempts: number;
  nextRunAt: number;
  lastError: string;
  payload: string;
  createdTime: number;
  updatedTime: number;
  finishedTime: number;
}

export interface LocalLicenseStatusApiData {
  valid: boolean;
  state: string;
  reason: string;
  keyPrefix: string;
  product: string;
  edition: string;
  versionChannel: string;
  features?: Record<string, unknown>;
  expiresAt: number;
  issuedAt: number;
  nextHeartbeatAt: number;
  graceUntil: number;
  configured: boolean;
  centerUrl: string;
  instanceId: string;
  lastError: string;
  updatedAt: number;
  publicKeySet: boolean;
}

export interface UpdateManifestAssetApiItem {
  id: number;
  type: string;
  file: string;
  sizeBytes: number;
  sha256: string;
  url: string;
}

export interface LocalLicenseUpdateApiData {
  currentVersion: string;
  latestVersion: string;
  hasUpdate: boolean;
  channel: string;
  force: boolean;
  allowRollback: boolean;
  releaseNotes: string;
  manifestUrl: string;
  reason?: string;
  capability?: {
    capable: boolean;
    reasons: string[];
    deployDir: string;
    backendContainer: string;
  };
  artifacts?: UpdateManifestAssetApiItem[];
}

export interface LocalLicenseUpdateRunApiData {
  version: string;
  channel: string;
  composeAsset: string;
  helperContainer: string;
  backendImageId: string;
  message: string;
}

export interface LocalLicenseUpdateLogApiData {
  log: string;
  deployDir: string;
  logPath: string;
}

export interface CommerceListFilter {
  keyword?: string;
  orderNo?: string;
  status?: string;
  orderType?: string;
  provider?: string;
  action?: string;
  userId?: number;
  dateFrom?: number;
  dateTo?: number;
  page?: number;
  pageSize?: number;
}

export interface PaginatedApiData<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
}

export const login = (data: LoginData) =>
  Network.post<LoginResponse>("/user/login", data);
export const registerUser = (data: RegisterData) =>
  Network.post<LoginResponse>("/user/register", data);

// 用户CRUD操作 - 全部使用POST请求
export const createUser = (data: UserMutationPayload) =>
  Network.post("/user/create", data);
export const getAllUsers = (pageData: UserListQuery = {}) =>
  Network.post<UserApiItem[]>("/user/list", pageData);
export const updateUser = (data: UserMutationPayload) =>
  Network.post("/user/update", data);
export const deleteUser = (id: number) => Network.post("/user/delete", { id });
export const getUserPackageInfo = () =>
  Network.post<UserPackageInfoApiData>("/user/package");

export const getPublicCommerceSettings = () =>
  Network.post<CommerceSettings>("/commerce/public/settings");
export const getLegalPages = () =>
  Network.post<LegalPagesApiData>("/commerce/legal");
export const getPublicPlans = () =>
  Network.post<PlanApiItem[]>("/commerce/plans/public");
export const createCommerceOrder = (data: {
  planId: number;
  type?: string;
  action?: "new" | "upgrade" | "renew";
  couponCode?: string;
}) =>
  Network.post<{ order: CommerceOrderApiItem; payUrl: string }>(
    "/commerce/order/create",
    data,
  );
export const getMyCommerceOrders = (filter: CommerceListFilter = {}) =>
  Network.post<PaginatedApiData<CommerceOrderApiItem>>(
    "/commerce/order/list",
    filter,
  );
export const payCommerceOrder = (id: number) =>
  Network.post<{ order: CommerceOrderApiItem; payUrl: string }>(
    "/commerce/order/pay",
    { id },
  );
export const payCommerceOrderWithProvider = (id: number, type: string) =>
  Network.post<{ order: CommerceOrderApiItem; payUrl: string }>(
    "/commerce/order/pay",
    { id, type },
  );
export const payCommerceOrderWithBalance = (id: number) =>
  Network.post<{ order: CommerceOrderApiItem; payUrl: string }>(
    "/commerce/order/pay-balance",
    { id },
  );
export const cancelCommerceOrder = (id: number) =>
  Network.post("/commerce/order/cancel", { id });
export const requestOrderRefund = (id: number, reason: string) =>
  Network.post<RefundRequestApiItem>("/commerce/order/refund", { id, reason });
export const getMySubscription = () =>
  Network.post<
    | (UserSubscriptionApiItem & {
        subscriptions?: UserSubscriptionApiItem[];
        resetFlowEnabled?: boolean;
      })
    | null
  >("/commerce/subscription/me");
export const resetMySubscriptionFlow = (data?: {
  type?: string;
  subscriptionId?: number;
}) =>
  Network.post<{ order: CommerceOrderApiItem; payUrl: string }>(
    "/commerce/subscription/reset-flow",
    data || {},
  );
export const getLocalLicenseStatus = () =>
  Network.post<LocalLicenseStatusApiData>("/license/local/status");
export const activateLocalLicense = (data: {
  centerUrl: string;
  publicKey: string;
  licenseKey: string;
}) => Network.post<LocalLicenseStatusApiData>("/license/local/activate", data);
export const heartbeatLocalLicense = () =>
  Network.post<LocalLicenseStatusApiData>("/license/local/heartbeat");
export const checkLocalLicenseUpdate = () =>
  Network.post<LocalLicenseUpdateApiData>("/license/local/update/check");
export const runLocalLicenseUpdate = (data?: {
  version?: string;
  channel?: string;
}) =>
  Network.post<LocalLicenseUpdateRunApiData>(
    "/license/local/update/run",
    data || {},
    { timeout: 15 * 60 * 1000 },
  );
export const getLocalLicenseUpdateLog = () =>
  Network.post<LocalLicenseUpdateLogApiData>("/license/local/update/log");
export const getAdminCommerceSettings = () =>
  Network.post<Record<string, string>>("/admin/commerce/settings");
export const updateAdminCommerceSettings = (data: Record<string, string>) =>
  Network.post("/admin/commerce/settings/update", data);
export const getAdminPlans = () =>
  Network.post<PlanApiItem[]>("/admin/commerce/plan/list");
export const saveAdminPlan = (data: Partial<PlanApiItem>) =>
  Network.post("/admin/commerce/plan/save", data);
export const deleteAdminPlan = (id: number) =>
  Network.post("/admin/commerce/plan/delete", { id });
export const getAdminCommerceOrders = (filter: CommerceListFilter = {}) =>
  Network.post<PaginatedApiData<CommerceOrderApiItem>>(
    "/admin/commerce/order/list",
    filter,
  );
export const confirmAdminCommerceOrder = (id: number) =>
  Network.post("/admin/commerce/order/confirm", { id });
export const getAdminPayments = (filter: CommerceListFilter = {}) =>
  Network.post<PaginatedApiData<CommercePaymentApiItem>>(
    "/admin/commerce/payment/list",
    filter,
  );
export const getAdminAuditLogs = (filter: CommerceListFilter = {}) =>
  Network.post<PaginatedApiData<CommerceAuditApiItem>>(
    "/admin/commerce/audit/list",
    filter,
  );
export const getAdminRefunds = (filter: CommerceListFilter = {}) =>
  Network.post<PaginatedApiData<RefundRequestApiItem>>(
    "/admin/commerce/refund/list",
    filter,
  );
export const handleAdminRefund = (
  id: number,
  decision: "approved" | "rejected",
  adminNote = "",
) => Network.post("/admin/commerce/refund/handle", { id, decision, adminNote });
export const getMyNotifications = (filter: CommerceListFilter = {}) =>
  Network.post<PaginatedApiData<NotificationApiItem>>(
    "/commerce/notification/list",
    filter,
  );
export const markNotificationRead = (id: number) =>
  Network.post("/commerce/notification/read", { id });
export const markAllNotificationsRead = () =>
  Network.post("/commerce/notification/read-all");
export const getMyWallet = (filter: CommerceListFilter = {}) =>
  Network.post<
    PaginatedApiData<WalletLedgerApiItem> & { balanceCents: number }
  >("/commerce/wallet/me", filter);
export const rechargeMyWallet = (data: {
  amountCents: number;
  type?: string;
}) =>
  Network.post<{ order: CommerceOrderApiItem; payUrl: string }>(
    "/commerce/wallet/recharge",
    data,
  );
export const getMyTickets = (filter: CommerceListFilter = {}) =>
  Network.post<PaginatedApiData<SupportTicketApiItem>>(
    "/commerce/ticket/list",
    filter,
  );
export const getMyTicketMessages = (id: number) =>
  Network.post<{
    ticket: SupportTicketApiItem;
    messages: SupportTicketMessageApiItem[];
  }>("/commerce/ticket/messages", { id });
export const createMyTicket = (data: {
  title: string;
  category?: string;
  content: string;
  attachmentUrl?: string;
}) => Network.post<SupportTicketApiItem>("/commerce/ticket/create", data);
export const replyMyTicket = (
  id: number,
  content: string,
  attachmentUrl = "",
) => Network.post("/commerce/ticket/reply", { id, content, attachmentUrl });
export const closeMyTicket = (id: number) =>
  Network.post("/commerce/ticket/close", { id });
export const getAdminTickets = (filter: CommerceListFilter = {}) =>
  Network.post<PaginatedApiData<SupportTicketApiItem>>(
    "/admin/commerce/ticket/list",
    filter,
  );
export const getAdminTicketMessages = (id: number) =>
  Network.post<{
    ticket: SupportTicketApiItem;
    messages: SupportTicketMessageApiItem[];
  }>("/admin/commerce/ticket/messages", { id });
export const updateAdminTicket = (data: {
  id: number;
  category?: string;
  priority?: string;
  internalNote?: string;
}) => Network.post("/admin/commerce/ticket/update", data);
export const replyAdminTicket = (
  id: number,
  content: string,
  attachmentUrl = "",
) =>
  Network.post("/admin/commerce/ticket/reply", { id, content, attachmentUrl });
export const closeAdminTicket = (id: number) =>
  Network.post("/admin/commerce/ticket/close", { id });
export const getAdminCoupons = (filter: CommerceListFilter = {}) =>
  Network.post<PaginatedApiData<CouponApiItem>>(
    "/admin/commerce/coupon/list",
    filter,
  );
export const saveAdminCoupon = (data: Partial<CouponApiItem>) =>
  Network.post<CouponApiItem>("/admin/commerce/coupon/save", data);
export const deleteAdminCoupon = (id: number) =>
  Network.post("/admin/commerce/coupon/delete", { id });
export const getAdminWalletLedger = (filter: CommerceListFilter = {}) =>
  Network.post<PaginatedApiData<WalletLedgerApiItem>>(
    "/admin/commerce/wallet/list",
    filter,
  );
export const adjustAdminWallet = (data: {
  userId: number;
  amountCents: number;
  note?: string;
}) => Network.post<WalletLedgerApiItem>("/admin/commerce/wallet/adjust", data);
export const syncAdminUserResources = (userId: number) =>
  Network.post("/admin/commerce/user/resources/sync", { userId });
export const getAdminCommerceReportSummary = () =>
  Network.post<CommerceReportSummaryApiData>("/admin/commerce/report/summary");
export const getAdminCommerceRisks = (filter: CommerceListFilter = {}) =>
  Network.post<PaginatedApiData<CommerceRiskApiItem>>(
    "/admin/commerce/risk/list",
    filter,
  );
export const getAdminResourceJobs = (filter: CommerceListFilter = {}) =>
  Network.post<PaginatedApiData<CommerceResourceJobApiItem>>(
    "/admin/commerce/resource-job/list",
    filter,
  );
export const retryAdminResourceJob = (id: number) =>
  Network.post("/admin/commerce/resource-job/retry", { id });
export const getInviteCodes = (filter: CommerceListFilter = {}) =>
  Network.post<PaginatedApiData<InviteCodeApiItem>>(
    "/admin/commerce/invite/list",
    filter,
  );
export const saveInviteCode = (data: Partial<InviteCodeApiItem>) =>
  Network.post<InviteCodeApiItem>("/admin/commerce/invite/save", data);
export const deleteInviteCode = (id: number) =>
  Network.post("/admin/commerce/invite/delete", { id });

// 节点CRUD操作 - 全部使用POST请求
export const createNode = (data: NodeMutationPayload) =>
  Network.post("/node/create", data);
export const getNodeList = () => Network.post<NodeApiItem[]>("/node/list");
export const getDashboardNodeExpiryList = () =>
  Network.post<NodeApiItem[]>("/node/list", {});
export const updateNode = (data: NodeMutationPayload) =>
  Network.post("/node/update", data);
export const deleteNode = (id: number) => Network.post("/node/delete", { id });
export const getNodeInstallCommand = (
  id: number,
  channel: ReleaseChannel = "stable",
) => Network.post<string>("/node/install", { id, channel });
export const updateNodeOrder = (data: {
  nodes: Array<{ id: number; inx: number }>;
}) => Network.post("/node/update-order", data);
export const dismissNodeExpiryReminder = (id: number) =>
  Network.post("/node/dismiss-expiry-reminder", { id });
export const checkNodeStatus = (nodeId?: number) => {
  const params = nodeId ? { nodeId } : {};

  return Network.post("/node/check-status", params);
};

export const upgradeNode = (
  id: number,
  version?: string,
  channel: ReleaseChannel = "stable",
) =>
  Network.post(
    "/node/upgrade",
    { id, version: version || "", channel },
    { timeout: 5 * 60 * 1000 },
  );
export const batchUpgradeNodes = (
  ids: number[],
  version?: string,
  channel: ReleaseChannel = "stable",
) =>
  Network.post(
    "/node/batch-upgrade",
    { ids, version: version || "", channel },
    { timeout: 15 * 60 * 1000 },
  );
export const getNodeReleases = (channel: ReleaseChannel = "stable") =>
  Network.post<NodeReleaseApiItem[]>("/node/releases", { channel });
export const rollbackNode = (id: number) =>
  Network.post("/node/rollback", { id });

// 隧道CRUD操作 - 全部使用POST请求
export const createTunnel = (data: TunnelMutationPayload) =>
  Network.post("/tunnel/create", data);
export const getTunnelList = () =>
  Network.post<TunnelApiItem[]>("/tunnel/list");
export const getTunnelById = (id: number) =>
  Network.post<TunnelApiItem>("/tunnel/get", { id });
export const updateTunnel = (data: TunnelMutationPayload) =>
  Network.post("/tunnel/update", data, { timeout: 120_000 });
export const deleteTunnel = (id: number) =>
  Network.post("/tunnel/delete", { id });
export const previewTunnelDelete = (id: number) =>
  Network.post<TunnelDeletePreviewApiData>("/tunnel/delete-preview", { id });
export const deleteTunnelWithForwards = (data: {
  id: number;
  action: "replace" | "delete_forwards";
  targetTunnelId?: number;
}) =>
  Network.post<TunnelDeleteWithForwardsApiData | BatchOperationResult>(
    "/tunnel/delete-with-forwards",
    data,
  );
export const previewBatchTunnelDelete = (ids: number[]) =>
  Network.post<TunnelBatchDeletePreviewApiData>(
    "/tunnel/batch-delete-preview",
    {
      ids,
    },
  );
export const batchDeleteTunnelsWithForwards = (data: {
  ids: number[];
  action: "replace" | "delete_forwards";
  targetTunnelId?: number;
}) =>
  Network.post<TunnelBatchDeleteWithForwardsApiData>(
    "/tunnel/batch-delete-with-forwards",
    data,
  );
export const diagnoseTunnel = (tunnelId: number) =>
  Network.post<TunnelDiagnosisApiData>(
    "/tunnel/diagnose",
    { tunnelId },
    { timeout: 120 * 1000 },
  );
export const updateTunnelOrder = (data: {
  tunnels: Array<{ id: number; inx: number }>;
}) => Network.post("/tunnel/update-order", data);

// 用户隧道权限管理操作 - 全部使用POST请求
export const assignUserTunnel = (data: UserTunnelAssignPayload) =>
  Network.post("/tunnel/user/assign", data);
export const batchAssignUserTunnel = (data: {
  userId: number;
  tunnels: Array<{ tunnelId: number; speedId?: number | null }>;
}) => Network.post("/tunnel/user/batch-assign", data);
export const getUserTunnelList = (queryData: UserTunnelListQuery = {}) =>
  Network.post<UserTunnelPermissionApiItem[]>("/tunnel/user/list", queryData);
export const removeUserTunnel = (params: UserTunnelRemovePayload) =>
  Network.post("/tunnel/user/remove", params);
export const updateUserTunnel = (data: UserTunnelAssignPayload) =>
  Network.post("/tunnel/user/update", data);
export const userTunnel = () =>
  Network.post<UserTunnelApiItem[]>("/tunnel/user/tunnel");

// 转发CRUD操作 - 全部使用POST请求
export const createForward = (data: ForwardMutationPayload) =>
  Network.post("/forward/create", data);
export const getForwardList = () =>
  Network.post<ForwardApiItem[]>("/forward/list");
export const updateForward = (data: ForwardMutationPayload) =>
  Network.post("/forward/update", data);
export const deleteForward = (id: number) =>
  Network.post("/forward/delete", { id });
export const forceDeleteForward = (id: number) =>
  Network.post("/forward/force-delete", { id });

// 转发服务控制操作 - 通过Java后端接口
export const pauseForwardService = (forwardId: number) =>
  Network.post("/forward/pause", { id: forwardId });
export const resumeForwardService = (forwardId: number) =>
  Network.post("/forward/resume", { id: forwardId });

// 转发诊断操作
export const diagnoseForward = (forwardId: number) =>
  Network.post<ForwardDiagnosisApiData>(
    "/forward/diagnose",
    { forwardId },
    { timeout: 120 * 1000 },
  );

// 转发排序操作
export const updateForwardOrder = (data: {
  forwards: Array<{ id: number; inx: number }>;
}) => Network.post("/forward/update-order", data);

// 限速规则CRUD操作 - 全部使用POST请求
export const createSpeedLimit = (data: SpeedLimitMutationPayload) =>
  Network.post("/speed-limit/create", data);
export const getSpeedLimitList = () =>
  Network.post<SpeedLimitApiItem[]>("/speed-limit/list");
export const updateSpeedLimit = (data: SpeedLimitMutationPayload) =>
  Network.post("/speed-limit/update", data);
export const deleteSpeedLimit = (id: number) =>
  Network.post("/speed-limit/delete", { id });

// 修改密码接口
export const updatePassword = (data: UpdatePasswordPayload) =>
  Network.post("/user/updatePassword", data);

// 重置流量接口
export const resetUserFlow = (data: { id: number; type: number }) =>
  Network.post("/user/reset", data);
export const resetUserQuota = (data: UserQuotaResetPayload) =>
  Network.post("/user/quota/reset", data);

export const getUserGroups = (id: number) =>
  Network.post<number[]>("/user/groups", { id });

// 网站配置相关接口
export const getConfigs = () =>
  Network.post<Record<string, string>>("/config/list");
export const getConfigByName = (name: string) =>
  Network.post<{ name: string; value: string }>("/config/get", { name });
export const getPublicConfigByName = (name: string) =>
  Network.post<{ name: string; value: string }>("/public/config/get", { name });
export const updateConfigs = (configMap: Record<string, string>) =>
  Network.post("/config/update", configMap);
export const updateConfig = (name: string, value: string) =>
  Network.post("/config/update-single", { name, value });

export const getStorageSummary = () =>
  Network.get<StorageSummaryApiData>("/system/storage");

export const getSystemUpgradeVersion = () =>
  Network.post<SystemUpgradeVersionApiData>("/system/version");

export const exportBackupData = () => Network.post("/backup/export");
export const importBackupData = (data: BackupImportPayload) =>
  Network.post("/backup/import", data);
export const restoreBackupData = (data: BackupImportPayload) =>
  Network.post("/backup/restore", data);

// 验证码相关接口
export const checkCaptcha = () => Network.post("/captcha/check");
export const verifyCaptcha = (data: { captchaId: string; trackData: string }) =>
  Network.post("/captcha/verify", data);

// 批量操作接口
export const batchDeleteForwards = (ids: number[]) =>
  Network.post<BatchOperationResult>("/forward/batch-delete", { ids });
export const batchPauseForwards = (ids: number[]) =>
  Network.post<BatchOperationResult>("/forward/batch-pause", { ids });
export const batchResumeForwards = (ids: number[]) =>
  Network.post<BatchOperationResult>("/forward/batch-resume", { ids });
export const batchDeleteTunnels = (ids: number[]) =>
  Network.post<BatchOperationResult>("/tunnel/batch-delete", { ids });
export const batchDeleteNodes = (ids: number[]) =>
  Network.post<BatchOperationResult>("/node/batch-delete", { ids });
export const batchRedeployForwards = (ids: number[]) =>
  Network.post<BatchOperationResult>("/forward/batch-redeploy", { ids });
export const batchRedeployTunnels = (ids: number[]) =>
  Network.post<BatchOperationResult>("/tunnel/batch-redeploy", { ids });
export const batchChangeTunnel = (data: {
  forwardIds: number[];
  targetTunnelId: number;
}) => Network.post<BatchOperationResult>("/forward/batch-change-tunnel", data);

// 分组与权限分配接口
export const getTunnelGroupList = () =>
  Network.post<TunnelGroupApiItem[]>("/group/tunnel/list");
export const createTunnelGroup = (data: { name: string; status?: number }) =>
  Network.post("/group/tunnel/create", data);
export const updateTunnelGroup = (data: {
  id: number;
  name: string;
  status?: number;
}) => Network.post("/group/tunnel/update", data);
export const deleteTunnelGroup = (id: number) =>
  Network.post("/group/tunnel/delete", { id });
export const assignTunnelsToGroup = (data: {
  groupId: number;
  tunnelIds: number[];
}) => Network.post("/group/tunnel/assign", data);

export const getUserGroupList = () =>
  Network.post<UserGroupApiItem[]>("/group/user/list");
export const createUserGroup = (data: { name: string; status?: number }) =>
  Network.post("/group/user/create", data);
export const updateUserGroup = (data: {
  id: number;
  name: string;
  status?: number;
}) => Network.post("/group/user/update", data);
export const deleteUserGroup = (id: number) =>
  Network.post("/group/user/delete", { id });
export const assignUsersToGroup = (data: {
  groupId: number;
  userIds: number[];
}) => Network.post("/group/user/assign", data);

export const getGroupPermissionList = () =>
  Network.post<GroupPermissionApiItem[]>("/group/permission/list");
export const assignGroupPermission = (data: {
  userGroupId: number;
  tunnelGroupId: number;
}) => Network.post("/group/permission/assign", data);
export const removeGroupPermission = (id: number) =>
  Network.post("/group/permission/remove", { id });

// 面板共享 (Federation) 接口
export const getPeerShareList = () =>
  Network.post<Array<Record<string, unknown>>>("/federation/share/list");
export const createPeerShare = (data: {
  name: string;
  nodeId: number;
  maxBandwidth?: number;
  expiryTime?: number;
  portRangeStart?: number;
  portRangeEnd?: number;
  allowedDomains?: string;
  allowedIps?: string;
}) => Network.post("/federation/share/create", data);
export const updatePeerShare = (data: {
  id: number;
  name: string;
  maxBandwidth: number;
  expiryTime: number;
  portRangeStart: number;
  portRangeEnd: number;
  allowedDomains: string;
  allowedIps: string;
}) => Network.post("/federation/share/update", data);
export const deletePeerShare = (id: number) =>
  Network.post("/federation/share/delete", { id });
export const resetPeerShareFlow = (id: number) =>
  Network.post("/federation/share/reset-flow", { id });
export const getPeerRemoteUsageList = () =>
  Network.post<Array<Record<string, unknown>>>(
    "/federation/share/remote-usage/list",
  );
export const importRemoteNode = (data: { remoteUrl: string; token: string }) =>
  Network.post("/federation/node/import", data);

export interface BackupTypes {
  users?: boolean;
  nodes?: boolean;
  tunnels?: boolean;
  forwards?: boolean;
  userTunnels?: boolean;
  speedLimits?: boolean;
  tunnelGroups?: boolean;
  userGroups?: boolean;
  permissions?: boolean;
  configs?: boolean;
}

export const exportBackup = async (types: string[] = []) => {
  const token = window.localStorage.getItem("token");
  const baseURL = axios.defaults.baseURL || "/api/v1/";

  const response = await axios.post(
    `${baseURL}/backup/export`,
    { types },
    {
      headers: {
        Authorization: token,
        "Content-Type": "application/json",
      },
      responseType: "blob",
    },
  );

  const url = window.URL.createObjectURL(new Blob([response.data]));
  const link = document.createElement("a");

  link.href = url;
  const timestamp = new Date().toISOString().slice(0, 19).replace(/[:-]/g, "");

  link.setAttribute("download", `backup_${timestamp}.json`);
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  window.URL.revokeObjectURL(url);
};

export const importBackup = (data: BackupImportPayload) =>
  Network.post("/backup/import", data);

export interface AnnouncementData {
  content: string;
  enabled: number;
  update_time?: number;
}

export const getAnnouncement = () =>
  Network.get<AnnouncementData>("/announcement/get");
export const updateAnnouncement = ({
  content,
  enabled,
}: Pick<AnnouncementData, "content" | "enabled">) =>
  Network.post("/announcement/update", { content, enabled });

export const getNodeMetrics = (
  nodeId: number,
  start?: number,
  end?: number,
) => {
  const params: Record<string, string> = {};

  if (start) params.start = String(start);
  if (end) params.end = String(end);

  return Network.get<NodeMetricApiItem[]>(
    `/monitor/nodes/${nodeId}/metrics`,
    params,
    { timeout: 60_000 },
  );
};

export const getNodeMetricsLatest = (nodeId: number) =>
  Network.get<NodeMetricApiItem>(`/monitor/nodes/${nodeId}/metrics/latest`);

export const getTunnelMetrics = (
  tunnelId: number,
  start?: number,
  end?: number,
) => {
  const params: Record<string, string> = {};

  if (start) params.start = String(start);
  if (end) params.end = String(end);

  return Network.get<TunnelMetricApiItem[]>(
    `/monitor/tunnels/${tunnelId}/metrics`,
    params,
  );
};

export const getMonitorTunnels = () =>
  Network.get<MonitorTunnelApiItem[]>("/monitor/tunnels");

export const getMonitorTunnelQuality = () =>
  Network.get<TunnelQualityApiItem[]>("/monitor/tunnels/quality");

export const getMonitorTunnelQualityHistory = (
  tunnelId: number,
  start?: number,
  end?: number,
) => {
  const params: Record<string, string> = {};

  if (start) params.start = String(start);
  if (end) params.end = String(end);

  return Network.get<TunnelQualityApiItem[]>(
    `/monitor/tunnels/${tunnelId}/quality`,
    params,
  );
};

export const getServiceMonitorList = () =>
  Network.get<ServiceMonitorApiItem[]>("/monitor/services");

export const getServiceMonitorLimits = () =>
  Network.get<ServiceMonitorLimitsApiData>("/monitor/services/limits");

export const createServiceMonitor = (data: ServiceMonitorMutationPayload) =>
  Network.post<ServiceMonitorApiItem>("/monitor/services/create", data);

export const updateServiceMonitor = (data: ServiceMonitorMutationPayload) =>
  Network.post<ServiceMonitorApiItem>("/monitor/services/update", data);

export const deleteServiceMonitor = (id: number) =>
  Network.post("/monitor/services/delete", { id });

export const getServiceMonitorResults = (
  monitorId: number,
  options?: { limit?: number; start?: number; end?: number },
) => {
  const params: Record<string, string> = {};

  if (options?.start != null && options?.end != null) {
    params.start = String(options.start);
    params.end = String(options.end);
  } else if (options?.limit != null) {
    params.limit = String(options.limit);
  }

  return Network.get<ServiceMonitorResultApiItem[]>(
    `/monitor/services/${monitorId}/results`,
    params,
  );
};

export const getServiceMonitorLatestResults = () =>
  Network.get<ServiceMonitorResultApiItem[]>(
    "/monitor/services/latest-results",
  );

export const runServiceMonitor = (id: number) =>
  Network.post<ServiceMonitorResultApiItem>("/monitor/services/run", { id });

export const getMonitorNodes = () =>
  Network.get<MonitorNodeApiItem[]>("/monitor/nodes");

export const getMonitorAccess = () =>
  Network.get<MonitorAccessApiData>("/monitor/access");

export const getMonitorPermissionList = () =>
  Network.get<MonitorPermissionApiItem[]>("/monitor/permission/list");

export const assignMonitorPermission = (userId: number) =>
  Network.post("/monitor/permission/assign", { userId });

export const removeMonitorPermission = (userId: number) =>
  Network.post("/monitor/permission/remove", { userId });
