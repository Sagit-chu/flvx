export interface NodeApiItem {
  id: number;
  name: string;
  status: number;
  inx?: number;
  remark?: string;
  expiryTime?: number;
  renewalCycle?: "month" | "quarter" | "year" | "";
  expiryReminderDismissed?: number;
  syncError?: string;
  [key: string]: unknown;
}

export type RuntimeEngine = "gost" | "dash";

export type RuntimeSwitchStatus = "idle" | "switching" | "failed";

export type RuntimeProgressState =
  | "pending"
  | "running"
  | "succeeded"
  | "failed"
  | "switching";

export interface RuntimeNodeSummaryApiData {
  total: number;
  completed: number;
  ready: number;
  pending: number;
  failed: number;
}

export interface RuntimeProgressApiData {
  engine: RuntimeEngine;
  state: RuntimeProgressState;
  phase?: string;
  message?: string;
}

export interface RuntimeNodeProgressApiItem {
  nodeId: number;
  nodeName: string;
  engine: RuntimeEngine;
  ready: boolean;
  progress: RuntimeProgressState;
  message?: string;
}

export interface RuntimeEngineSettingsApiData {
  currentEngine: RuntimeEngine;
  switchStatus: RuntimeSwitchStatus;
  generation: number;
  lastError: string;
  nodeSummary: RuntimeNodeSummaryApiData;
  runtimeProgress: RuntimeProgressApiData;
  rebuildProgress: RuntimeProgressApiData;
  warnings: string[];
  nodes: RuntimeNodeProgressApiItem[];
}

export interface RuntimeEngineUpdatePayload {
  engine: RuntimeEngine;
}

export interface UserApiItem {
  id: number;
  user: string;
  name?: string;
  status: number;
  flow: number;
  num: number;
  expTime?: number;
  flowResetTime?: number;
  inFlow?: number;
  outFlow?: number;
  dailyQuotaGB?: number;
  monthlyQuotaGB?: number;
  dailyUsedBytes?: number;
  monthlyUsedBytes?: number;
  disabledByQuota?: number;
  quotaDisabledAt?: number;
  [key: string]: unknown;
}

export interface UserListQuery {
  current?: number;
  size?: number;
  keyword?: string;
  [key: string]: unknown;
}

export interface TunnelApiItem {
  id: number;
  name: string;
  type: number;
  status: number;
  flow?: number;
  trafficRatio?: number;
  inIp?: string;
  ipPreference?: string;
  inNodeId?: TunnelChainNodePayload[];
  outNodeId?: TunnelChainNodePayload[];
  chainNodes?: TunnelChainNodePayload[][];
  entryNodeId: number;
  exitNodeId: number;
  inx?: number;
  [key: string]: unknown;
}

export interface ForwardApiItem {
  id: number;
  name: string;
  status: number;
  tunnelName?: string;
  tunnelTrafficRatio?: number;
  inIp?: string;
  inPort?: number;
  remoteAddr?: string;
  inFlow?: number;
  outFlow?: number;
  userId?: number;
  tunnelId?: number;
  speedId?: number | null;
  inx?: number;
  [key: string]: unknown;
}

export interface UserTunnelApiItem {
  id: number;
  name: string;
  tunnelId?: number;
  tunnelName?: string;
  inNodePortSta?: number;
  inNodePortEnd?: number;
  speedId?: number | null;
  [key: string]: unknown;
}

export interface UserTunnelPermissionApiItem {
  id: number;
  userId: number;
  tunnelId: number;
  tunnelName: string;
  status: number;
  flow: number;
  num: number;
  expTime: number;
  flowResetTime: number;
  speedId?: number | null;
  speedLimitName?: string;
  inFlow: number;
  outFlow: number;
  tunnelFlow?: number;
  [key: string]: unknown;
}

export interface StatisticsFlowApiItem {
  id: number;
  userId: number;
  flow: number;
  totalFlow: number;
  time: string;
  [key: string]: unknown;
}

export interface SpeedLimitApiItem {
  id: number;
  name: string;
  speed: number;
  status: number;
  createdTime: string;
  updatedTime: string;
  uploadSpeed?: number;
  downloadSpeed?: number;
  [key: string]: unknown;
}

export interface TunnelGroupApiItem {
  id: number;
  name: string;
  status: number;
  tunnelIds: number[];
  tunnelNames: string[];
  createdTime: number;
  [key: string]: unknown;
}

export interface UserGroupApiItem {
  id: number;
  name: string;
  status: number;
  userIds: number[];
  userNames: string[];
  createdTime: number;
  [key: string]: unknown;
}

export interface GroupPermissionApiItem {
  id: number;
  userGroupId: number;
  userGroupName: string;
  tunnelGroupId: number;
  tunnelGroupName: string;
  createdTime: number;
  [key: string]: unknown;
}

export interface TunnelDiagnosisApiItem {
  success: boolean;
  description: string;
  nodeName: string;
  nodeId: string;
  targetIp: string;
  targetPort?: number;
  message?: string;
  averageTime?: number;
  packetLoss?: number;
  fromChainType?: number;
  fromInx?: number;
  toChainType?: number;
  toInx?: number;
  [key: string]: unknown;
}

export interface TunnelDiagnosisApiData {
  tunnelName: string;
  tunnelType: string;
  timestamp: number;
  results: TunnelDiagnosisApiItem[];
}

export interface ForwardDiagnosisApiData {
  forwardName: string;
  timestamp: number;
  results: TunnelDiagnosisApiItem[];
}

export interface NodeReleaseApiItem {
  version: string;
  name: string;
  publishedAt: string;
  prerelease: boolean;
  channel: "stable" | "dev";
}

export interface UserPackageInfoApiData {
  userInfo: {
    flow: number;
    inFlow: number;
    outFlow: number;
    num: number;
    expTime?: string;
    flowResetTime?: number;
    [key: string]: unknown;
  };
  tunnelPermissions: UserTunnelPermissionApiItem[];
  forwards: ForwardApiItem[];
  statisticsFlows: StatisticsFlowApiItem[];
  [key: string]: unknown;
}

export interface BatchOperationResult {
  successCount: number;
  failCount: number;
  failures?: BatchOperationFailure[];
  [key: string]: unknown;
}

export interface BatchOperationFailure {
  id?: number;
  name?: string;
  reason?: string;
  [key: string]: unknown;
}

export interface TunnelDeletePreviewForwardApiItem {
  id: number;
  name: string;
  userId: number;
  userName: string;
  inPort: number;
  [key: string]: unknown;
}

export interface TunnelDeletePreviewApiData {
  tunnelId: number;
  tunnelName: string;
  forwardCount: number;
  sampleForwards: TunnelDeletePreviewForwardApiItem[];
  [key: string]: unknown;
}

export interface TunnelBatchDeletePreviewApiData {
  tunnelCount: number;
  totalForwardCount: number;
  items: TunnelDeletePreviewApiData[];
  [key: string]: unknown;
}

export interface TunnelDeleteWithForwardsApiData {
  forwardCount: number;
  migratedCount: number;
  deletedForwardCount: number;
  portAdjustedCount: number;
  warnings?: string[];
  [key: string]: unknown;
}

export interface TunnelBatchDeleteWithForwardsApiData {
  successCount: number;
  failCount: number;
  failures?: BatchOperationFailure[];
  deletedForwardCount: number;
  migratedCount: number;
  portAdjustedCount: number;
  warnings?: string[];
  [key: string]: unknown;
}

export interface UserMutationPayload {
  id?: number;
  user?: string;
  name?: string;
  password?: string;
  status?: number;
  flow?: number;
  num?: number;
  expTime?: number | string;
  flowResetTime?: number;
  dailyQuotaGB?: number;
  monthlyQuotaGB?: number;
  tunnelFlow?: number;
}

export interface NodeMutationPayload {
  id?: number | null;
  name?: string;
  status?: number;
  inx?: number;
  remark?: string;
  expiryTime?: number;
  renewalCycle?: "month" | "quarter" | "year" | "";
  serverIp?: string;
  serverIpV4?: string;
  serverIpV6?: string;
  extraIPs?: string;
  port?: string;
  tcpListenAddr?: string;
  udpListenAddr?: string;
  interfaceName?: string;
  http?: number;
  tls?: number;
  socks?: number;
}

export interface TunnelChainNodePayload {
  nodeId: number;
  protocol?: string;
  strategy?: string;
  connectIp?: string;
  chainType?: number;
  inx?: number;
}

export interface TunnelMutationPayload {
  id?: number;
  name?: string;
  type?: number;
  status?: number;
  flow?: number;
  trafficRatio?: number;
  inIp?: string;
  ipPreference?: string;
  inNodeId?: TunnelChainNodePayload[];
  outNodeId?: TunnelChainNodePayload[];
  chainNodes?: TunnelChainNodePayload[][];
}

export interface UserQuotaResetPayload {
  userId: number;
  scope?: "daily" | "monthly" | "all";
}

export interface UserTunnelAssignPayload {
  userId?: number;
  id?: number;
  tunnelId?: number;
  flow?: number;
  num?: number;
  expTime?: number;
  flowResetTime?: number;
  status?: number;
  speedId?: number | null;
  tunnels?: Array<{ tunnelId: number; speedId?: number | null }>;
}

export interface UserTunnelListQuery {
  userId?: number;
  tunnelId?: number;
  current?: number;
  size?: number;
}

export interface UserTunnelRemovePayload {
  id?: number;
  userId?: number;
  tunnelId?: number;
}

export interface ForwardMutationPayload {
  id?: number;
  name?: string;
  status?: number;
  tunnelId?: number | null;
  inIp?: string;
  inPort?: number | null;
  remoteAddr?: string;
  strategy?: string;
  speedId?: number | null;
}

export interface SpeedLimitMutationPayload {
  id?: number;
  name?: string;
  speed?: number;
  status?: number;
}

export interface UpdatePasswordPayload {
  currentPassword: string;
  newPassword: string;
  newUsername?: string;
}

export interface BackupImportPayload {
  types: string[];
  [key: string]: unknown;
}

export interface NodeMetricApiItem {
  id: number;
  nodeId: number;
  timestamp: number;
  cpuUsage: number;
  memoryUsage: number;
  diskUsage: number;
  netInBytes: number;
  netOutBytes: number;
  netInSpeed: number;
  netOutSpeed: number;
  load1: number;
  load5: number;
  load15: number;
  tcpConns: number;
  udpConns: number;
  uptime: number;
}

export interface TunnelMetricApiItem {
  id: number;
  tunnelId: number;
  nodeId: number;
  timestamp: number;
  bytesIn: number;
  bytesOut: number;
  connections: number;
  errors: number;
  avgLatencyMs: number;
}

export interface ServiceMonitorApiItem {
  id: number;
  name: string;
  // Keep as string for forward-compatibility.
  type: string;
  target: string;
  intervalSec: number;
  timeoutSec: number;
  nodeId: number;
  enabled: number;
  createdTime: number;
  updatedTime: number;
}

export interface ServiceMonitorResultApiItem {
  id: number;
  monitorId: number;
  timestamp: number;
  success: number;
  latencyMs: number;
  statusCode: number;
  errorMessage: string;
}

export interface ServiceMonitorMutationPayload {
  id?: number;
  name: string;
  type: "tcp" | "icmp";
  target: string;
  intervalSec?: number;
  timeoutSec?: number;
  nodeId?: number;
  enabled?: number;
}

export interface ServiceMonitorLimitsApiData {
  checkerScanIntervalSec: number;
  minIntervalSec: number;
  defaultIntervalSec: number;
  minTimeoutSec: number;
  defaultTimeoutSec: number;
  maxTimeoutSec: number;
}

export interface MonitorNodeApiItem {
  id: number;
  inx: number;
  name: string;
  status: number;
  version?: string;
  updatedTime: number;
}

export interface MonitorTunnelApiItem {
  id: number;
  inx: number;
  name: string;
  status: number;
  updatedTime: number;
}

export interface MonitorPermissionApiItem {
  id: number;
  userId: number;
  createdTime: number;
}

export interface MonitorAccessApiData {
  allowed: boolean;
  reason?: string;
}

export interface TunnelQualityHopApiItem {
  fromNodeId: number;
  fromNodeName: string;
  toNodeId: number;
  toNodeName: string;
  latency: number;
  loss: number;
  targetIp?: string;
  targetPort?: number;
}

export interface TunnelQualityApiItem {
  tunnelId: number;
  entryToExitLatency: number;
  exitToBingLatency: number;
  entryToExitLoss: number;
  exitToBingLoss: number;
  success: boolean;
  errorMessage?: string;
  timestamp: number;
  chainDetails?: string;
}
