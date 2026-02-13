import { useState, useEffect, useCallback } from "react";
import { Button } from "@heroui/button";
import { Card, CardBody, CardHeader } from "@heroui/card";
import { Tabs, Tab } from "@heroui/tabs";
import { Input } from "@heroui/input";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
} from "@heroui/modal";
import { Select, SelectItem } from "@heroui/select";
import { toast } from "react-hot-toast";
import {
  getNodeList,
  createPeerShare,
  getPeerShareList,
  deletePeerShare,
  resetPeerShareFlow,
  getPeerRemoteUsageList,
  importRemoteNode,
  updatePeerShare,
} from "@/api";

interface Node {
  id: number;
  name: string;
  isRemote?: number;
}

interface PeerShare {
  id: number;
  name: string;
  token: string;
  maxBandwidth: number;
  currentFlow: number;
  expiryTime: number;
  portRangeStart: number;
  portRangeEnd: number;
  isActive: number;
  allowedDomains?: string;
  allowedIps?: string;
  usedPorts?: number[];
  usedPortDetails?: Array<{
    runtimeId: number;
    port: number;
    role: string;
    protocol: string;
    resourceKey: string;
    applied: number;
    updatedTime: number;
  }>;
  activeRuntimeNum?: number;
}

interface RemoteUsageBinding {
  bindingId: number;
  tunnelId: number;
  tunnelName: string;
  chainType: number;
  hopInx: number;
  allocatedPort: number;
  resourceKey: string;
  remoteBindingId: string;
  updatedTime: number;
}

interface RemoteUsageNode {
  nodeId: number;
  nodeName: string;
  remoteUrl: string;
  shareId: number;
  portRangeStart: number;
  portRangeEnd: number;
  maxBandwidth: number;
  currentFlow: number;
  usedPorts: number[];
  bindings: RemoteUsageBinding[];
  activeBindingNum: number;
  syncError?: string;
}

export default function PanelSharingPage() {
  const [selectedTab, setSelectedTab] = useState("my-shares");
  const [shares, setShares] = useState<PeerShare[]>([]);
  const [remoteUsageNodes, setRemoteUsageNodes] = useState<RemoteUsageNode[]>(
    [],
  );
  const [nodes, setNodes] = useState<Node[]>([]);
  const [loading, setLoading] = useState(false);
  const [remoteUsageLoading, setRemoteUsageLoading] = useState(false);

  // Modals
  const [createShareOpen, setCreateShareOpen] = useState(false);
  const [editShareOpen, setEditShareOpen] = useState(false);
  const [importNodeOpen, setImportNodeOpen] = useState(false);

  // Forms
  const [shareForm, setShareForm] = useState({
    name: "",
    nodeId: "",
    maxBandwidth: 0,
    expiryDays: 30,
    portRangeStart: 10000,
    portRangeEnd: 20000,
    allowedDomains: "",
    allowedIps: "",
  });

  const [importForm, setImportForm] = useState({
    remoteUrl: "",
    token: "",
  });

  const [editForm, setEditForm] = useState({
    id: 0,
    name: "",
    maxBandwidth: 0,
    expiryTime: 0,
    portRangeStart: 10000,
    portRangeEnd: 20000,
    allowedDomains: "",
    allowedIps: "",
  });

  const loadShares = useCallback(async () => {
    setLoading(true);
    try {
      const res = await getPeerShareList();
      if (res.code === 0) {
        setShares(res.data || []);
      } else {
        toast.error(res.msg || "加载分享列表失败");
      }
    } finally {
      setLoading(false);
    }
  }, []);

  const loadNodes = useCallback(async () => {
    try {
      const res = await getNodeList();
      if (res.code === 0) {
        const localNodes: Node[] = (res.data || []).filter(
          (node: Node) => (node?.isRemote ?? 0) !== 1,
        );
        setNodes(localNodes);
        setShareForm((prev) => {
          if (!prev.nodeId) {
            return prev;
          }
          const hasSelectedNode = localNodes.some(
            (node: Node) => String(node.id) === prev.nodeId,
          );
          return hasSelectedNode ? prev : { ...prev, nodeId: "" };
        });
      }
    } catch {
      // ignore
    }
  }, []);

  const loadRemoteUsage = useCallback(async () => {
    setRemoteUsageLoading(true);
    try {
      const res = await getPeerRemoteUsageList();
      if (res.code === 0) {
        setRemoteUsageNodes(res.data || []);
      } else {
        toast.error(res.msg || "加载远程占用端口失败");
      }
    } finally {
      setRemoteUsageLoading(false);
    }
  }, []);

  useEffect(() => {
    if (selectedTab === "my-shares") {
      loadShares();
      loadNodes();
      return;
    }
    if (selectedTab === "remote-nodes") {
      loadRemoteUsage();
    }
  }, [selectedTab, loadShares, loadNodes, loadRemoteUsage]);

  const handleCreateShare = async () => {
    if (!shareForm.name || !shareForm.nodeId) {
      toast.error("请填写必要信息");
      return;
    }
    const nodeId = parseInt(shareForm.nodeId, 10);
    if (Number.isNaN(nodeId) || !nodes.some((node) => node.id === nodeId)) {
      toast.error("仅可选择本地节点");
      return;
    }
    if (shareForm.maxBandwidth < 0) {
      toast.error("流量上限不能为负数");
      return;
    }
    try {
      const expiryTime =
        Date.now() + shareForm.expiryDays * 24 * 60 * 60 * 1000;
      const res = await createPeerShare({
        name: shareForm.name,
        nodeId,
        maxBandwidth: Math.max(0, shareForm.maxBandwidth) * 1024 * 1024 * 1024,
        expiryTime: shareForm.expiryDays === 0 ? 0 : expiryTime,
        portRangeStart: shareForm.portRangeStart,
        portRangeEnd: shareForm.portRangeEnd,
        allowedDomains: shareForm.allowedDomains,
        allowedIps: shareForm.allowedIps,
      });
      if (res.code === 0) {
        toast.success("创建成功");
        setCreateShareOpen(false);
        loadShares();
      } else {
        toast.error(res.msg || "创建失败");
      }
    } catch {
      toast.error("网络错误");
    }
  };

  const handleDeleteShare = async (id: number) => {
    try {
      const res = await deletePeerShare(id);
      if (res.code === 0) {
        toast.success("删除成功");
        loadShares();
      } else {
        toast.error(res.msg || "删除失败");
      }
    } catch {
      toast.error("网络错误");
    }
  };

  const handleResetShareFlow = async (id: number) => {
    try {
      const res = await resetPeerShareFlow(id);
      if (res.code === 0) {
        toast.success("共享流量已重置");
        loadShares();
      } else {
        toast.error(res.msg || "重置流量失败");
      }
    } catch {
      toast.error("网络错误");
    }
  };

  const openEditShare = (share: PeerShare) => {
    setEditForm({
      id: share.id,
      name: share.name,
      maxBandwidth: share.maxBandwidth > 0 ? Math.round(share.maxBandwidth / (1024 * 1024 * 1024)) : 0,
      expiryTime: share.expiryTime,
      portRangeStart: share.portRangeStart,
      portRangeEnd: share.portRangeEnd,
      allowedDomains: share.allowedDomains || "",
      allowedIps: share.allowedIps || "",
    });
    setEditShareOpen(true);
  };

  const handleEditShare = async () => {
    if (!editForm.name) {
      toast.error("名称不能为空");
      return;
    }
    if (editForm.maxBandwidth < 0) {
      toast.error("流量上限不能为负数");
      return;
    }
    try {
      const res = await updatePeerShare({
        id: editForm.id,
        name: editForm.name,
        maxBandwidth: Math.max(0, editForm.maxBandwidth) * 1024 * 1024 * 1024,
        expiryTime: editForm.expiryTime,
        portRangeStart: editForm.portRangeStart,
        portRangeEnd: editForm.portRangeEnd,
        allowedDomains: editForm.allowedDomains,
        allowedIps: editForm.allowedIps,
      });
      if (res.code === 0) {
        toast.success("编辑成功");
        setEditShareOpen(false);
        loadShares();
      } else {
        toast.error(res.msg || "编辑失败");
      }
    } catch {
      toast.error("网络错误");
    }
  };

  const handleImportNode = async () => {
    if (!importForm.remoteUrl || !importForm.token) {
      toast.error("请填写完整信息");
      return;
    }
    try {
      // Automatically add http/https if missing
      let url = importForm.remoteUrl.trim();
      if (!url.startsWith("http")) {
        url = "http://" + url;
      }
      
      const res = await importRemoteNode({
        remoteUrl: url,
        token: importForm.token.trim(),
      });
      if (res.code === 0) {
        toast.success("导入成功，请前往节点列表查看");
        setImportNodeOpen(false);
        setImportForm({ remoteUrl: "", token: "" });
        loadRemoteUsage();
      } else {
        toast.error(res.msg || "导入失败");
      }
    } catch {
      toast.error("网络错误");
    }
  };

  const copyToken = (token: string) => {
    navigator.clipboard.writeText(token);
    toast.success("Token已复制");
  };

  const formatFlowGB = (bytes: number) => {
    if (!Number.isFinite(bytes) || bytes <= 0) {
      return "0 B";
    }
    if (bytes < 1024) return bytes + " B";
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(2) + " KB";
    if (bytes < 1024 * 1024 * 1024)
      return (bytes / (1024 * 1024)).toFixed(2) + " MB";
    return (bytes / (1024 * 1024 * 1024)).toFixed(2) + " GB";
  };

  const formatChainType = (chainType: number, hopInx: number) => {
    if (chainType === 2) {
      return `中继跳点 #${hopInx}`;
    }
    if (chainType === 3) {
      return "出口节点";
    }
    return "未知链路";
  };

  return (
    <div className="p-4 md:p-6 space-y-6">
      <div className="flex justify-between items-center">
        <h1 className="text-2xl font-bold">面板共享 (Panel Peering)</h1>
      </div>

      <Tabs
        disableCursorAnimation
        aria-label="Options"
        selectedKey={selectedTab}
        onSelectionChange={(k) => setSelectedTab(k as string)}
      >
        <Tab key="my-shares" title="我分享的 (Provider)">
          <Card>
            <CardBody>
              <div className="mb-4">
                <Button color="primary" onPress={() => setCreateShareOpen(true)}>
                  创建分享
                </Button>
              </div>
              
              {loading ? (
                <div className="text-center py-10 text-gray-500">加载中...</div>
              ) : shares.length === 0 ? (
                <div className="text-center py-10 text-gray-500">暂无分享</div>
              ) : (
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                  {shares.map((share) => (
                    <Card key={share.id} className="border border-divider shadow-sm">
                      <CardHeader className="flex justify-between">
                        <h3 className="font-bold">{share.name}</h3>
                        <div className="flex gap-2">
                          <Button
                            size="sm"
                            variant="flat"
                            onPress={() => openEditShare(share)}
                          >
                            编辑
                          </Button>
                          <Button
                            size="sm"
                            variant="flat"
                            onPress={() => handleResetShareFlow(share.id)}
                          >
                            重置流量
                          </Button>
                          <Button size="sm" color="danger" variant="flat" onPress={() => handleDeleteShare(share.id)}>删除</Button>
                        </div>
                      </CardHeader>
                      <CardBody className="text-sm space-y-2">
                        <p>端口范围: {share.portRangeStart} - {share.portRangeEnd}</p>
                        <p>流量上限: {share.maxBandwidth > 0 ? formatFlowGB(share.maxBandwidth) : "不限制"}</p>
                        <p>当前流量: {formatFlowGB(share.currentFlow || 0)}</p>
                        <p>远程占用端口: {share.usedPorts && share.usedPorts.length > 0 ? share.usedPorts.join(", ") : "暂无"}</p>
                        {share.usedPortDetails && share.usedPortDetails.length > 0 && (
                          <div className="flex flex-wrap gap-2">
                            {share.usedPortDetails.map((item) => (
                              <span key={item.runtimeId} className="text-xs rounded-full px-2 py-1 bg-default-100">
                                {item.port} / {item.role || "reserved"}
                              </span>
                            ))}
                          </div>
                        )}
                        {share.allowedDomains && <p>允许域名: {share.allowedDomains}</p>}
                        {share.allowedIps && <p>允许API IP: {share.allowedIps}</p>}
                        <p>过期时间: {share.expiryTime === 0 ? "永久" : new Date(share.expiryTime).toLocaleDateString()}</p>
                        <div className="flex gap-2">
                          <Input readOnly size="sm" value={share.token} />
                          <Button size="sm" onPress={() => copyToken(share.token)}>复制</Button>
                        </div>
                      </CardBody>
                    </Card>
                  ))}
                </div>
              )}
            </CardBody>
          </Card>
        </Tab>
        <Tab key="remote-nodes" title="远程节点 (Consumer)">
          <Card>
            <CardBody>
              <div className="mb-4">
                <Button color="secondary" onPress={() => setImportNodeOpen(true)}>
                  导入远程节点
                </Button>
              </div>

              {remoteUsageLoading ? (
                <div className="text-center py-10 text-gray-500">加载中...</div>
              ) : remoteUsageNodes.length === 0 ? (
                <div className="text-center py-10 text-gray-500">
                  <p>暂无远程节点占用记录。</p>
                  <p className="mt-2">导入远程节点并创建隧道后，这里会显示远端端口占用情况。</p>
                </div>
              ) : (
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  {remoteUsageNodes.map((node) => (
                    <Card key={node.nodeId} className="border border-divider shadow-sm">
                      <CardHeader className="flex justify-between">
                        <h3 className="font-bold">{node.nodeName}</h3>
                        <span className="text-xs text-default-500">绑定 {node.activeBindingNum || 0}</span>
                      </CardHeader>
                      <CardBody className="text-sm space-y-2">
                        {node.syncError && (
                          <div className="px-2 py-1.5 rounded-md bg-warning-50 dark:bg-warning-100/10 text-warning-700 dark:text-warning-400 text-xs">
                            {node.syncError === "provider_share_deleted"
                              ? "提供方已删除该分享"
                              : node.syncError === "provider_share_disabled"
                                ? "提供方已禁用该分享"
                                : node.syncError === "provider_share_expired"
                                  ? "提供方分享已过期"
                                  : `远程同步失败: ${node.syncError}`}
                          </div>
                        )}
                        {node.remoteUrl && <p>远程地址: {node.remoteUrl}</p>}
                        <p>共享ID: {node.shareId || "-"}</p>
                        <p>端口范围: {node.portRangeStart > 0 && node.portRangeEnd > 0 ? `${node.portRangeStart} - ${node.portRangeEnd}` : "-"}</p>
                        <p>共享流量: {node.maxBandwidth > 0 ? `${formatFlowGB(node.currentFlow || 0)} / ${formatFlowGB(node.maxBandwidth)}` : `${formatFlowGB(node.currentFlow || 0)} / 不限制`}</p>
                        <p>远端占用端口: {node.usedPorts && node.usedPorts.length > 0 ? node.usedPorts.join(", ") : "暂无"}</p>
                        {node.bindings && node.bindings.length > 0 && (
                          <div className="space-y-1 pt-1">
                            {node.bindings.map((binding) => (
                              <p key={binding.bindingId} className="text-xs text-default-600">
                                隧道 {binding.tunnelName || `#${binding.tunnelId}`}
                                {" · "}
                                端口 {binding.allocatedPort}
                                {" · "}
                                {formatChainType(binding.chainType, binding.hopInx)}
                              </p>
                            ))}
                          </div>
                        )}
                      </CardBody>
                    </Card>
                  ))}
                </div>
              )}
            </CardBody>
          </Card>
        </Tab>
      </Tabs>

      {/* Create Share Modal */}
      <Modal isOpen={createShareOpen} onClose={() => setCreateShareOpen(false)}>
        <ModalContent>
          <ModalHeader>创建分享</ModalHeader>
          <ModalBody>
            <Input
              label="名称"
              placeholder="备注名称"
              value={shareForm.name}
              onChange={(e) => setShareForm({ ...shareForm, name: e.target.value })}
            />
            <Select
              label="选择节点"
              placeholder="选择要分享的本地节点"
              selectedKeys={shareForm.nodeId ? [shareForm.nodeId] : []}
              onChange={(e) => setShareForm({ ...shareForm, nodeId: e.target.value })}
            >
              {nodes.map((node) => (
                <SelectItem key={node.id} textValue={node.name}>
                  {node.name}
                </SelectItem>
              ))}
            </Select>
            <div className="flex gap-4">
              <Input
                label="起始端口"
                type="number"
                value={shareForm.portRangeStart.toString()}
                onChange={(e) => setShareForm({ ...shareForm, portRangeStart: parseInt(e.target.value) })}
              />
              <Input
                label="结束端口"
                type="number"
                value={shareForm.portRangeEnd.toString()}
                onChange={(e) => setShareForm({ ...shareForm, portRangeEnd: parseInt(e.target.value) })}
              />
            </div>
            <Input
              label="有效期 (天)"
              type="number"
              description="0 表示永久"
              value={shareForm.expiryDays.toString()}
              onChange={(e) => setShareForm({ ...shareForm, expiryDays: parseInt(e.target.value) })}
            />
            <Input
              label="流量上限 (GB)"
              type="number"
              description="0 表示不限流量"
              value={shareForm.maxBandwidth.toString()}
              onChange={(e) => setShareForm({ ...shareForm, maxBandwidth: parseInt(e.target.value, 10) || 0 })}
            />
            <Input
              label="允许的域名 (可选)"
              placeholder="example.com, panel.test.com"
              description="限制使用此Token的来源面板域名，多个域名用逗号分隔，留空不限制"
              value={shareForm.allowedDomains}
              onChange={(e) => setShareForm({ ...shareForm, allowedDomains: e.target.value })}
            />
            <Input
              label="允许的API IP (可选)"
              placeholder="203.0.113.10, 2001:db8::10, 198.51.100.0/24"
              description="仅白名单IP可导入此分享，支持IPv4/IPv6/CIDR，多个用逗号分隔"
              value={shareForm.allowedIps}
              onChange={(e) => setShareForm({ ...shareForm, allowedIps: e.target.value })}
            />
          </ModalBody>
          <ModalFooter>
            <Button onPress={() => setCreateShareOpen(false)}>取消</Button>
            <Button color="primary" onPress={handleCreateShare}>创建</Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Edit Share Modal */}
      <Modal isOpen={editShareOpen} onClose={() => setEditShareOpen(false)}>
        <ModalContent>
          <ModalHeader>编辑分享</ModalHeader>
          <ModalBody>
            <Input
              label="名称"
              placeholder="备注名称"
              value={editForm.name}
              onChange={(e) => setEditForm({ ...editForm, name: e.target.value })}
            />
            <div className="flex gap-4">
              <Input
                label="起始端口"
                type="number"
                value={editForm.portRangeStart.toString()}
                onChange={(e) => setEditForm({ ...editForm, portRangeStart: parseInt(e.target.value) || 0 })}
              />
              <Input
                label="结束端口"
                type="number"
                value={editForm.portRangeEnd.toString()}
                onChange={(e) => setEditForm({ ...editForm, portRangeEnd: parseInt(e.target.value) || 0 })}
              />
            </div>
            <Input
              label="流量上限 (GB)"
              type="number"
              description="0 表示不限流量"
              value={editForm.maxBandwidth.toString()}
              onChange={(e) => setEditForm({ ...editForm, maxBandwidth: parseInt(e.target.value, 10) || 0 })}
            />
            <Input
              label="过期时间"
              type="datetime-local"
              description="留空或清除表示永久有效"
              value={editForm.expiryTime > 0 ? new Date(editForm.expiryTime).toISOString().slice(0, 16) : ""}
              onChange={(e) => setEditForm({ ...editForm, expiryTime: e.target.value ? new Date(e.target.value).getTime() : 0 })}
            />
            <Input
              label="允许的域名 (可选)"
              placeholder="example.com, panel.test.com"
              description="限制使用此Token的来源面板域名，多个域名用逗号分隔，留空不限制"
              value={editForm.allowedDomains}
              onChange={(e) => setEditForm({ ...editForm, allowedDomains: e.target.value })}
            />
            <Input
              label="允许的API IP (可选)"
              placeholder="203.0.113.10, 2001:db8::10, 198.51.100.0/24"
              description="仅白名单IP可导入此分享，支持IPv4/IPv6/CIDR，多个用逗号分隔"
              value={editForm.allowedIps}
              onChange={(e) => setEditForm({ ...editForm, allowedIps: e.target.value })}
            />
          </ModalBody>
          <ModalFooter>
            <Button onPress={() => setEditShareOpen(false)}>取消</Button>
            <Button color="primary" onPress={handleEditShare}>保存</Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Import Node Modal */}
      <Modal isOpen={importNodeOpen} onClose={() => setImportNodeOpen(false)}>
        <ModalContent>
          <ModalHeader>导入远程节点</ModalHeader>
          <ModalBody>
            <Input
              label="远程面板地址"
              placeholder="http://panel.example.com:8088"
              value={importForm.remoteUrl}
              onChange={(e) => setImportForm({ ...importForm, remoteUrl: e.target.value })}
            />
            <Input
              label="Token"
              placeholder="Bearer Token"
              value={importForm.token}
              onChange={(e) => setImportForm({ ...importForm, token: e.target.value })}
            />
          </ModalBody>
          <ModalFooter>
            <Button onPress={() => setImportNodeOpen(false)}>取消</Button>
            <Button color="secondary" onPress={handleImportNode}>导入</Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
