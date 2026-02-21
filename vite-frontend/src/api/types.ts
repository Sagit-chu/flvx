export interface NodeApiItem {
  id: number;
  name: string;
  status: number;
  inx?: number;
  syncError?: string;
  [key: string]: unknown;
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
  inIp?: string;
  inPort?: number;
  remoteAddr?: string;
  inFlow?: number;
  outFlow?: number;
  userId?: number;
  tunnelId?: number;
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
  tunnelId: number;
  speed: number;
  status: number;
  tunnelName: string;
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
  tunnelFlow?: number;
}

export interface NodeMutationPayload {
  id?: number | null;
  name?: string;
  status?: number;
  inx?: number;
  serverIp?: string;
  serverIpV4?: string;
  serverIpV6?: string;
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
}

export interface SpeedLimitMutationPayload {
  id?: number;
  name?: string;
  speed?: number;
  status?: number;
  tunnelId?: number | null;
  tunnelName?: string;
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
