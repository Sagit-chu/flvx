import Network from "./network";

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

export const login = (data: LoginData) =>
  Network.post<LoginResponse>("/user/login", data);

// 用户CRUD操作 - 全部使用POST请求
export const createUser = (data: any) => Network.post("/user/create", data);
export const getAllUsers = (pageData: any = {}) =>
  Network.post("/user/list", pageData);
export const updateUser = (data: any) => Network.post("/user/update", data);
export const deleteUser = (id: number) => Network.post("/user/delete", { id });
export const getUserPackageInfo = () => Network.post("/user/package");

// 节点CRUD操作 - 全部使用POST请求
export const createNode = (data: any) => Network.post("/node/create", data);
export const getNodeList = () => Network.post("/node/list");
export const updateNode = (data: any) => Network.post("/node/update", data);
export const deleteNode = (id: number) => Network.post("/node/delete", { id });
export const getNodeInstallCommand = (id: number) =>
  Network.post("/node/install", { id });
export const updateNodeOrder = (data: {
  nodes: Array<{ id: number; inx: number }>;
}) => Network.post("/node/update-order", data);
export const checkNodeStatus = (nodeId?: number) => {
  const params = nodeId ? { nodeId } : {};

  return Network.post("/node/check-status", params);
};

export const upgradeNode = (id: number, version?: string) =>
  Network.post("/node/upgrade", { id, version: version || "" }, { timeout: 5 * 60 * 1000 });
export const batchUpgradeNodes = (ids: number[], version?: string) =>
  Network.post("/node/batch-upgrade", { ids, version: version || "" }, { timeout: 15 * 60 * 1000 });
export const getNodeReleases = () => Network.post("/node/releases");
export const rollbackNode = (id: number) =>
  Network.post("/node/rollback", { id });

// 隧道CRUD操作 - 全部使用POST请求
export const createTunnel = (data: any) => Network.post("/tunnel/create", data);
export const getTunnelList = () => Network.post("/tunnel/list");
export const getTunnelById = (id: number) =>
  Network.post("/tunnel/get", { id });
export const updateTunnel = (data: any) => Network.post("/tunnel/update", data);
export const deleteTunnel = (id: number) =>
  Network.post("/tunnel/delete", { id });
export const diagnoseTunnel = (tunnelId: number) =>
  Network.post("/tunnel/diagnose", { tunnelId });
export const updateTunnelOrder = (data: {
  tunnels: Array<{ id: number; inx: number }>;
}) => Network.post("/tunnel/update-order", data);

// 用户隧道权限管理操作 - 全部使用POST请求
export const assignUserTunnel = (data: any) =>
  Network.post("/tunnel/user/assign", data);
export const batchAssignUserTunnel = (data: {
  userId: number;
  tunnels: Array<{ tunnelId: number; speedId?: number | null }>;
}) => Network.post("/tunnel/user/batch-assign", data);
export const getUserTunnelList = (queryData: any = {}) =>
  Network.post("/tunnel/user/list", queryData);
export const removeUserTunnel = (params: any) =>
  Network.post("/tunnel/user/remove", params);
export const updateUserTunnel = (data: any) =>
  Network.post("/tunnel/user/update", data);
export const userTunnel = () => Network.post("/tunnel/user/tunnel");

// 转发CRUD操作 - 全部使用POST请求
export const createForward = (data: any) =>
  Network.post("/forward/create", data);
export const getForwardList = () => Network.post("/forward/list");
export const updateForward = (data: any) =>
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
  Network.post("/forward/diagnose", { forwardId });

// 转发排序操作
export const updateForwardOrder = (data: {
  forwards: Array<{ id: number; inx: number }>;
}) => Network.post("/forward/update-order", data);

// 限速规则CRUD操作 - 全部使用POST请求
export const createSpeedLimit = (data: any) =>
  Network.post("/speed-limit/create", data);
export const getSpeedLimitList = () => Network.post("/speed-limit/list");
export const updateSpeedLimit = (data: any) =>
  Network.post("/speed-limit/update", data);
export const deleteSpeedLimit = (id: number) =>
  Network.post("/speed-limit/delete", { id });

// 修改密码接口
export const updatePassword = (data: any) =>
  Network.post("/user/updatePassword", data);

// 重置流量接口
export const resetUserFlow = (data: { id: number; type: number }) =>
  Network.post("/user/reset", data);

// 网站配置相关接口
export const getConfigs = () => Network.post("/config/list");
export const getConfigByName = (name: string) =>
  Network.post("/config/get", { name });
export const updateConfigs = (configMap: Record<string, string>) =>
  Network.post("/config/update", configMap);
export const updateConfig = (name: string, value: string) =>
  Network.post("/config/update-single", { name, value });

// 验证码相关接口
export const checkCaptcha = () => Network.post("/captcha/check");
export const generateCaptcha = () => Network.post(`/captcha/generate`);
export const verifyCaptcha = (data: { captchaId: string; trackData: string }) =>
  Network.post("/captcha/verify", data);

// 批量操作接口
export const batchDeleteForwards = (ids: number[]) =>
  Network.post("/forward/batch-delete", { ids });
export const batchPauseForwards = (ids: number[]) =>
  Network.post("/forward/batch-pause", { ids });
export const batchResumeForwards = (ids: number[]) =>
  Network.post("/forward/batch-resume", { ids });
export const batchDeleteTunnels = (ids: number[]) =>
  Network.post("/tunnel/batch-delete", { ids });
export const batchDeleteNodes = (ids: number[]) =>
  Network.post("/node/batch-delete", { ids });
export const batchRedeployForwards = (ids: number[]) =>
  Network.post("/forward/batch-redeploy", { ids });
export const batchRedeployTunnels = (ids: number[]) =>
  Network.post("/tunnel/batch-redeploy", { ids });
export const batchChangeTunnel = (data: {
  forwardIds: number[];
  targetTunnelId: number;
}) => Network.post("/forward/batch-change-tunnel", data);

// 分组与权限分配接口
export const getTunnelGroupList = () => Network.post("/group/tunnel/list");
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

export const getUserGroupList = () => Network.post("/group/user/list");
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
  Network.post("/group/permission/list");
export const assignGroupPermission = (data: {
  userGroupId: number;
  tunnelGroupId: number;
}) => Network.post("/group/permission/assign", data);
export const removeGroupPermission = (id: number) =>
  Network.post("/group/permission/remove", { id });

// 面板共享 (Federation) 接口
export const getPeerShareList = () => Network.post("/federation/share/list");
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
  Network.post("/federation/share/remote-usage/list");
export const importRemoteNode = (data: {
  remoteUrl: string;
  token: string;
}) => Network.post("/federation/node/import", data);
