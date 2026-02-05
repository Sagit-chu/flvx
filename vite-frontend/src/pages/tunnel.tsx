import { useState, useEffect, useMemo } from "react";
import { Card, CardBody, CardHeader } from "@heroui/card";
import { Button } from "@heroui/button";
import { Input, Textarea } from "@heroui/input";
import { Select, SelectItem } from "@heroui/select";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
} from "@heroui/modal";
import { Chip } from "@heroui/chip";
import { Spinner } from "@heroui/spinner";
import { Divider } from "@heroui/divider";
import { Alert } from "@heroui/alert";
import { Checkbox } from "@heroui/checkbox";
import toast from "react-hot-toast";
import {
  DndContext,
  KeyboardSensor,
  MouseSensor,
  TouchSensor,
  type DragEndEvent,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  rectSortingStrategy,
  sortableKeyboardCoordinates,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";

import {
  createTunnel,
  getTunnelList,
  updateTunnel,
  deleteTunnel,
  getNodeList,
  diagnoseTunnel,
  updateTunnelOrder,
  batchDeleteTunnels,
  batchRedeployTunnels,
} from "@/api";

interface ChainTunnel {
  nodeId: number;
  protocol?: string; // 'tls' | 'wss' | 'tcp' | 'mtls' | 'mwss' | 'mtcp' - 转发链协议
  strategy?: string; // 'fifo' | 'round' | 'rand' - 仅转发链需要
  chainType?: number; // 1: 入口, 2: 转发链, 3: 出口
  inx?: number; // 转发链序号
}

interface Tunnel {
  id: number;
  inx?: number;
  name: string;
  type: number; // 1: 端口转发, 2: 隧道转发
  inNodeId: ChainTunnel[]; // 入口节点列表
  outNodeId?: ChainTunnel[]; // 出口节点列表
  chainNodes?: ChainTunnel[][]; // 转发链节点列表，二维数组
  inIp: string;
  outIp?: string;
  protocol?: string;
  flow: number; // 1: 单向, 2: 双向
  trafficRatio: number;
  status: number;
  createdTime: string;
}

interface Node {
  id: number;
  name: string;
  status: number; // 1: 在线, 0: 离线
}

interface TunnelForm {
  id?: number;
  name: string;
  type: number;
  inNodeId: ChainTunnel[];
  outNodeId?: ChainTunnel[];
  chainNodes?: ChainTunnel[][]; // 转发链节点列表，二维数组，外层是跳数，内层是该跳的节点
  flow: number;
  trafficRatio: number;
  inIp: string; // 入口IP
  status: number;
}

interface DiagnosisResult {
  tunnelName: string;
  tunnelType: string;
  timestamp: number;
  results: Array<{
    success: boolean;
    description: string;
    nodeName: string;
    nodeId: string;
    targetIp: string;
    targetPort?: number;
    message?: string;
    averageTime?: number;
    packetLoss?: number;
    fromChainType?: number; // 1: 入口, 2: 链, 3: 出口
    fromInx?: number;
    toChainType?: number;
    toInx?: number;
  }>;
}

export default function TunnelPage() {
  const [loading, setLoading] = useState(true);
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [tunnelOrder, setTunnelOrder] = useState<number[]>([]);
  const [nodes, setNodes] = useState<Node[]>([]);

  // 模态框状态
  const [modalOpen, setModalOpen] = useState(false);
  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [diagnosisModalOpen, setDiagnosisModalOpen] = useState(false);
  const [isEdit, setIsEdit] = useState(false);
  const [submitLoading, setSubmitLoading] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [diagnosisLoading, setDiagnosisLoading] = useState(false);
  const [tunnelToDelete, setTunnelToDelete] = useState<Tunnel | null>(null);
  const [currentDiagnosisTunnel, setCurrentDiagnosisTunnel] =
    useState<Tunnel | null>(null);
  const [diagnosisResult, setDiagnosisResult] =
    useState<DiagnosisResult | null>(null);

  // 表单状态
  const [form, setForm] = useState<TunnelForm>({
    name: "",
    type: 1,
    inNodeId: [],
    outNodeId: [],
    chainNodes: [],
    flow: 1,
    trafficRatio: 1.0,
    inIp: "",
    status: 1,
  });

  // 表单验证错误
  const [errors, setErrors] = useState<{ [key: string]: string }>({});

  // 批量操作相关状态
  const [selectMode, setSelectMode] = useState(false);
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  const [batchDeleteModalOpen, setBatchDeleteModalOpen] = useState(false);
  const [batchLoading, setBatchLoading] = useState(false);

  useEffect(() => {
    loadData();
  }, []);

  // 加载所有数据
  const loadData = async () => {
    setLoading(true);
    try {
      const [tunnelsRes, nodesRes] = await Promise.all([
        getTunnelList(),
        getNodeList(),
      ]);

      if (tunnelsRes.code === 0) {
        const tunnelsData: Tunnel[] = (tunnelsRes.data || []).map((t: any) => ({
          ...t,
          inx: t.inx ?? 0,
        }));

        setTunnels(tunnelsData);

        // 优先使用数据库中的 inx 字段进行排序，否则回退到本地排序
        const hasDbOrdering = tunnelsData.some(
          (t) => t.inx !== undefined && t.inx !== 0,
        );

        if (hasDbOrdering) {
          const dbOrder = [...tunnelsData]
            .sort((a, b) => (a.inx ?? 0) - (b.inx ?? 0))
            .map((t) => t.id);

          setTunnelOrder(dbOrder);
        } else {
          try {
            const stored = localStorage.getItem("tunnel-order");

            if (stored) {
              const parsed = JSON.parse(stored);

              if (Array.isArray(parsed)) {
                const existingIds = new Set(tunnelsData.map((t) => t.id));
                const validOrder = parsed
                  .map((id: any) => Number(id))
                  .filter((id: number) => existingIds.has(id));

                if (validOrder.length > 0) {
                  setTunnelOrder(validOrder);
                } else {
                  setTunnelOrder(tunnelsData.map((t) => t.id));
                }
              } else {
                setTunnelOrder(tunnelsData.map((t) => t.id));
              }
            } else {
              setTunnelOrder(tunnelsData.map((t) => t.id));
            }
          } catch {
            setTunnelOrder(tunnelsData.map((t) => t.id));
          }
        }
      } else {
        toast.error(tunnelsRes.msg || "获取隧道列表失败");
      }

      if (nodesRes.code === 0) {
        setNodes(nodesRes.data || []);
      } else {
        console.warn("获取节点列表失败:", nodesRes.msg);
      }
    } catch (error) {
      console.error("加载数据失败:", error);
      toast.error("加载数据失败");
    } finally {
      setLoading(false);
    }
  };

  // 表单验证
  const validateForm = (): boolean => {
    const newErrors: { [key: string]: string } = {};

    if (!form.name.trim()) {
      newErrors.name = "请输入隧道名称";
    } else if (form.name.length < 2 || form.name.length > 50) {
      newErrors.name = "隧道名称长度应在2-50个字符之间";
    }

    if (!form.inNodeId || form.inNodeId.length === 0) {
      newErrors.inNodeId = "请至少选择一个入口节点";
    } else {
      // 验证所有选择的节点都在线
      const offlineNodes = form.inNodeId.filter((item) => {
        const node = nodes.find((n) => n.id === item.nodeId);

        return node && node.status !== 1;
      });

      if (offlineNodes.length > 0) {
        newErrors.inNodeId = "所有入口节点必须在线";
      }
    }

    if (form.trafficRatio < 0.0 || form.trafficRatio > 100.0) {
      newErrors.trafficRatio = "流量倍率必须在0.0-100.0之间";
    }

    // 隧道转发时的验证
    if (form.type === 2) {
      if (!form.outNodeId || form.outNodeId.length === 0) {
        newErrors.outNodeId = "请至少选择一个出口节点";
      } else {
        // 验证所有选择的节点都在线
        const offlineNodes = form.outNodeId.filter((item) => {
          const node = nodes.find((n) => n.id === item.nodeId);

          return node && node.status !== 1;
        });

        if (offlineNodes.length > 0) {
          newErrors.outNodeId = "所有出口节点必须在线";
        }

        // 检查是否有重复节点
        const inNodeIds = form.inNodeId.map((item) => item.nodeId);
        const outNodeIds = form.outNodeId.map((item) => item.nodeId);
        const overlap = inNodeIds.filter((id) => outNodeIds.includes(id));

        if (overlap.length > 0) {
          newErrors.outNodeId = "隧道转发模式下，入口和出口不能有相同节点";
        }
      }
    }

    setErrors(newErrors);

    return Object.keys(newErrors).length === 0;
  };

  // 新增隧道
  const handleAdd = () => {
    setIsEdit(false);
    setForm({
      name: "",
      type: 1,
      inNodeId: [],
      outNodeId: [],
      chainNodes: [],
      flow: 1,
      trafficRatio: 1.0,
      inIp: "",
      status: 1,
    });
    setErrors({});
    setModalOpen(true);
  };

  // 编辑隧道 - 只能修改部分字段
  const handleEdit = (tunnel: Tunnel) => {
    setIsEdit(true);

    // 直接使用列表数据，getAllTunnels 已经包含完整的节点信息
    setForm({
      id: tunnel.id,
      name: tunnel.name,
      type: tunnel.type,
      inNodeId: tunnel.inNodeId || [],
      outNodeId: tunnel.outNodeId || [],
      chainNodes: tunnel.chainNodes || [],
      flow: tunnel.flow,
      trafficRatio: tunnel.trafficRatio,
      inIp: tunnel.inIp
        ? tunnel.inIp
            .split(",")
            .map((ip) => ip.trim())
            .join("\n")
        : "",
      status: tunnel.status,
    });
    setErrors({});
    setModalOpen(true);
  };

  // 删除隧道
  const handleDelete = (tunnel: Tunnel) => {
    setTunnelToDelete(tunnel);
    setDeleteModalOpen(true);
  };

  const confirmDelete = async () => {
    if (!tunnelToDelete) return;

    setDeleteLoading(true);
    try {
      const response = await deleteTunnel(tunnelToDelete.id);

      if (response.code === 0) {
        toast.success("删除成功");
        setDeleteModalOpen(false);
        setTunnelToDelete(null);
        loadData();
      } else {
        toast.error(response.msg || "删除失败");
      }
    } catch (error) {
      console.error("删除失败:", error);
      toast.error("删除失败");
    } finally {
      setDeleteLoading(false);
    }
  };

  // 隧道类型改变时的处理
  const handleTypeChange = (type: number) => {
    setForm((prev) => ({
      ...prev,
      type,
      outNodeId: type === 1 ? [] : prev.outNodeId,
      chainNodes: type === 1 ? [] : prev.chainNodes,
    }));
  };

  // 删除转发链中的某一跳（删除整个分组）
  const removeChainNode = (groupIndex: number) => {
    setForm((prev) => ({
      ...prev,
      chainNodes: (prev.chainNodes || []).filter(
        (_, index) => index !== groupIndex,
      ),
    }));
  };

  // 添加节点到指定的转发链跳数
  const addNodeToChain = (groupIndex: number, nodeId: number) => {
    setForm((prev) => {
      const chainNodes = [...(prev.chainNodes || [])];
      const group = chainNodes[groupIndex] || [];

      // 获取当前组的策略和协议
      const strategy = group.length > 0 ? group[0].strategy : "round";
      const protocol = group.length > 0 ? group[0].protocol : "tls";

      // 添加节点到该组
      chainNodes[groupIndex] = [
        ...group,
        { nodeId, chainType: 2, protocol, strategy },
      ];

      return { ...prev, chainNodes };
    });
  };

  // 从某一跳删除指定节点
  const removeNodeFromChain = (groupIndex: number, nodeId: number) => {
    setForm((prev) => {
      const chainNodes = [...(prev.chainNodes || [])];

      chainNodes[groupIndex] = (chainNodes[groupIndex] || []).filter(
        (node) => node.nodeId !== nodeId,
      );

      return { ...prev, chainNodes };
    });
  };

  // 更新某一跳的所有节点的协议
  const updateChainProtocol = (groupIndex: number, protocol: string) => {
    setForm((prev) => {
      const chainNodes = [...(prev.chainNodes || [])];

      chainNodes[groupIndex] = (chainNodes[groupIndex] || []).map((node) => ({
        ...node,
        protocol,
      }));

      return { ...prev, chainNodes };
    });
  };

  // 更新某一跳的所有节点的策略
  const updateChainStrategy = (groupIndex: number, strategy: string) => {
    setForm((prev) => {
      const chainNodes = [...(prev.chainNodes || [])];

      chainNodes[groupIndex] = (chainNodes[groupIndex] || []).map((node) => ({
        ...node,
        strategy,
      }));

      return { ...prev, chainNodes };
    });
  };

  // 获取所有转发链中已选择的节点ID列表
  const getSelectedChainNodeIds = (): number[] => {
    return (form.chainNodes || []).flatMap((group) =>
      group.map((node) => node.nodeId),
    );
  };

  // 获取转发链分组（已经是二维数组）
  const getChainGroups = (): ChainTunnel[][] => {
    return form.chainNodes || [];
  };

  // 提交表单
  const handleSubmit = async () => {
    if (!validateForm()) return;

    setSubmitLoading(true);
    try {
      // 过滤掉占位节点（nodeId === -1 的节点）
      const cleanedChainNodes = (form.chainNodes || [])
        .map((group) => group.filter((node) => node.nodeId !== -1))
        .filter((group) => group.length > 0); // 移除空组

      // 过滤掉出口节点中的占位节点
      const cleanedOutNodeId = (form.outNodeId || []).filter(
        (node) => node.nodeId !== -1,
      );

      // 将换行符分隔的IP转换为逗号分隔
      const inIpString = form.inIp
        .split("\n")
        .map((ip) => ip.trim())
        .filter((ip) => ip)
        .join(",");

      const data = {
        ...form,
        inIp: inIpString,
        outNodeId: cleanedOutNodeId,
        chainNodes: cleanedChainNodes,
      };

      const response = isEdit
        ? await updateTunnel(data)
        : await createTunnel(data);

      if (response.code === 0) {
        toast.success(isEdit ? "更新成功" : "创建成功");
        setModalOpen(false);
        loadData();
      } else {
        toast.error(response.msg || (isEdit ? "更新失败" : "创建失败"));
      }
    } catch (error) {
      console.error("提交失败:", error);
      toast.error("网络错误，请重试");
    } finally {
      setSubmitLoading(false);
    }
  };

  // 诊断隧道
  const handleDiagnose = async (tunnel: Tunnel) => {
    setCurrentDiagnosisTunnel(tunnel);
    setDiagnosisModalOpen(true);
    setDiagnosisLoading(true);
    setDiagnosisResult(null);

    try {
      const response = await diagnoseTunnel(tunnel.id);

      if (response.code === 0) {
        setDiagnosisResult(response.data);
      } else {
        toast.error(response.msg || "诊断失败");
        setDiagnosisResult({
          tunnelName: tunnel.name,
          tunnelType: tunnel.type === 1 ? "端口转发" : "隧道转发",
          timestamp: Date.now(),
          results: [
            {
              success: false,
              description: "诊断失败",
              nodeName: "-",
              nodeId: "-",
              targetIp: "-",
              targetPort: 443,
              message: response.msg || "诊断过程中发生错误",
            },
          ],
        });
      }
    } catch (error) {
      console.error("诊断失败:", error);
      toast.error("网络错误，请重试");
      setDiagnosisResult({
        tunnelName: tunnel.name,
        tunnelType: tunnel.type === 1 ? "端口转发" : "隧道转发",
        timestamp: Date.now(),
        results: [
          {
            success: false,
            description: "网络错误",
            nodeName: "-",
            nodeId: "-",
            targetIp: "-",
            targetPort: 443,
            message: "无法连接到服务器",
          },
        ],
      });
    } finally {
      setDiagnosisLoading(false);
    }
  };

  // 获取类型显示
  const getTypeDisplay = (type: number) => {
    switch (type) {
      case 1:
        return { text: "端口转发", color: "primary" };
      case 2:
        return { text: "隧道转发", color: "secondary" };
      default:
        return { text: "未知", color: "default" };
    }
  };

  // 获取流量计算显示
  const getFlowDisplay = (flow: number) => {
    switch (flow) {
      case 1:
        return "单向计算";
      case 2:
        return "双向计算";
      default:
        return "未知";
    }
  };

  // 获取连接质量
  const getQualityDisplay = (averageTime?: number, packetLoss?: number) => {
    if (averageTime === undefined || packetLoss === undefined) return null;

    if (averageTime < 30 && packetLoss === 0)
      return { text: "🚀 优秀", color: "success" };
    if (averageTime < 50 && packetLoss === 0)
      return { text: "✨ 很好", color: "success" };
    if (averageTime < 100 && packetLoss < 1)
      return { text: "👍 良好", color: "primary" };
    if (averageTime < 150 && packetLoss < 2)
      return { text: "😐 一般", color: "warning" };
    if (averageTime < 200 && packetLoss < 5)
      return { text: "😟 较差", color: "warning" };

    return { text: "😵 很差", color: "danger" };
  };

  // 处理拖拽结束
  const handleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event;

    if (!active || !over || active.id === over.id) return;
    if (!tunnelOrder || tunnelOrder.length === 0) return;

    const activeId = Number(active.id);
    const overId = Number(over.id);

    if (isNaN(activeId) || isNaN(overId)) return;

    const oldIndex = tunnelOrder.indexOf(activeId);
    const newIndex = tunnelOrder.indexOf(overId);

    if (oldIndex === -1 || newIndex === -1 || oldIndex === newIndex) return;

    const newOrder = arrayMove(tunnelOrder, oldIndex, newIndex);

    setTunnelOrder(newOrder);

    // 保存到 localStorage
    try {
      localStorage.setItem("tunnel-order", JSON.stringify(newOrder));
    } catch (error) {
      console.warn("无法保存隧道排序到localStorage:", error);
    }

    // 持久化到数据库
    try {
      const tunnelsToUpdate = newOrder.map((id, index) => ({ id, inx: index }));
      const response = await updateTunnelOrder({ tunnels: tunnelsToUpdate });

      if (response.code === 0) {
        setTunnels((prev) =>
          prev.map((tunnel) => {
            const updated = tunnelsToUpdate.find((t) => t.id === tunnel.id);

            return updated ? { ...tunnel, inx: updated.inx } : tunnel;
          }),
        );
      } else {
        toast.error("保存排序失败：" + (response.msg || "未知错误"));
      }
    } catch (error) {
      console.error("保存隧道排序到数据库失败:", error);
      toast.error("保存排序失败，请重试");
    }
  };

  const toggleSelectMode = () => {
    setSelectMode(!selectMode);
    if (selectMode) {
      setSelectedIds(new Set());
    }
  };

  const toggleSelect = (id: number) => {
    const newSet = new Set(selectedIds);
    if (newSet.has(id)) {
      newSet.delete(id);
    } else {
      newSet.add(id);
    }
    setSelectedIds(newSet);
  };

  const selectAll = () => {
    const allIds = sortedTunnels.map((t) => t.id);
    setSelectedIds(new Set(allIds));
  };

  const deselectAll = () => {
    setSelectedIds(new Set());
  };

  const handleBatchDelete = async () => {
    if (selectedIds.size === 0) return;
    setBatchLoading(true);
    try {
      const res = await batchDeleteTunnels(Array.from(selectedIds));
      if (res.code === 0) {
        const result = res.data;
        if (result.failCount === 0) {
          toast.success(`成功删除 ${result.successCount} 项`);
        } else {
          toast.error(`成功 ${result.successCount} 项，失败 ${result.failCount} 项`);
        }
        setSelectedIds(new Set());
        setSelectMode(false);
        setBatchDeleteModalOpen(false);
        loadData();
      } else {
        toast.error(res.msg || "批量删除失败");
      }
    } catch (e: any) {
      toast.error(e.message || "批量删除失败");
    } finally {
      setBatchLoading(false);
    }
  };

  const handleBatchRedeploy = async () => {
    if (selectedIds.size === 0) return;
    setBatchLoading(true);
    try {
      const res = await batchRedeployTunnels(Array.from(selectedIds));
      if (res.code === 0) {
        const result = res.data;
        if (result.failCount === 0) {
          toast.success(`成功重新下发 ${result.successCount} 项`);
        } else {
          toast.error(`成功 ${result.successCount} 项，失败 ${result.failCount} 项`);
        }
        setSelectedIds(new Set());
        setSelectMode(false);
        loadData();
      } else {
        toast.error(res.msg || "批量重新下发失败");
      }
    } catch (e: any) {
      toast.error(e.message || "批量重新下发失败");
    } finally {
      setBatchLoading(false);
    }
  };

  // 传感器配置
  const sensors = useSensors(
    useSensor(MouseSensor, {
      activationConstraint: {
        distance: 8,
      },
    }),
    useSensor(TouchSensor, {
      activationConstraint: {
        delay: 250,
        tolerance: 8,
      },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  // 根据排序顺序获取隧道列表
  const sortedTunnels = useMemo((): Tunnel[] => {
    if (!tunnels || tunnels.length === 0) return [];

    const sortedByDb = [...tunnels].sort((a, b) => {
      const aInx = a.inx ?? 0;
      const bInx = b.inx ?? 0;

      return aInx - bInx;
    });

    // 如果数据库中没有排序信息，则使用本地存储的顺序
    if (
      tunnelOrder &&
      tunnelOrder.length > 0 &&
      sortedByDb.every((t) => t.inx === undefined || t.inx === 0)
    ) {
      const tunnelMap = new Map(tunnels.map((t) => [t.id, t] as const));
      const localSorted: Tunnel[] = [];

      tunnelOrder.forEach((id) => {
        const tunnel = tunnelMap.get(id);

        if (tunnel) localSorted.push(tunnel);
      });

      tunnels.forEach((tunnel) => {
        if (!tunnelOrder.includes(tunnel.id)) {
          localSorted.push(tunnel);
        }
      });

      return localSorted;
    }

    return sortedByDb;
  }, [tunnels, tunnelOrder]);

  const sortableTunnelIds = useMemo(
    () => sortedTunnels.map((t) => t.id),
    [sortedTunnels],
  );

  const SortableItem = ({
    id,
    children,
  }: {
    id: number;
    children: (listeners: any) => any;
  }) => {
    const {
      attributes,
      listeners,
      setNodeRef,
      transform,
      transition,
      isDragging,
    } = useSortable({ id });

    const style = {
      transform: transform ? CSS.Transform.toString(transform) : undefined,
      transition: isDragging ? undefined : transition || undefined,
      opacity: isDragging ? 0.5 : 1,
      willChange: "transform",
    };

    return (
      <div ref={setNodeRef} style={style} {...attributes}>
        {children(listeners)}
      </div>
    );
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="flex items-center gap-3">
          <Spinner size="sm" />
          <span className="text-default-600">正在加载...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="px-3 lg:px-6 py-8">
      {/* 页面头部 */}
      <div className="flex items-center justify-between mb-6">
        <div className="flex-1" />

        <div className="flex items-center gap-2">
          <Button
            size="sm"
            variant={selectMode ? "solid" : "flat"}
            color={selectMode ? "warning" : "default"}
            onPress={toggleSelectMode}
          >
            {selectMode ? "退出选择" : "批量操作"}
          </Button>
          <Button color="primary" size="sm" variant="flat" onPress={handleAdd}>
            新增
          </Button>
        </div>
      </div>

      {selectMode && selectedIds.size > 0 && (
        <div className="fixed bottom-4 left-1/2 transform -translate-x-1/2 z-50 bg-content1 shadow-lg rounded-lg border border-divider p-3 flex items-center gap-3">
          <span className="text-sm text-default-600">已选择 {selectedIds.size} 项</span>
          <Button size="sm" variant="flat" onPress={selectAll}>
            全选
          </Button>
          <Button size="sm" variant="flat" onPress={deselectAll}>
            取消全选
          </Button>
          <Button
            size="sm"
            color="danger"
            variant="flat"
            onPress={() => setBatchDeleteModalOpen(true)}
          >
            批量删除
          </Button>
          <Button
            size="sm"
            color="primary"
            variant="flat"
            onPress={handleBatchRedeploy}
            isLoading={batchLoading}
          >
            批量重新下发
          </Button>
        </div>
      )}

      {/* 隧道卡片网格 */}
      {tunnels.length > 0 ? (
        <DndContext sensors={sensors} onDragEnd={handleDragEnd}>
          <SortableContext
            items={sortableTunnelIds}
            strategy={rectSortingStrategy}
          >
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 gap-4">
              {sortedTunnels.map((tunnel) => {
                const typeDisplay = getTypeDisplay(tunnel.type);

                return (
                  <SortableItem key={tunnel.id} id={tunnel.id}>
                    {(listeners) => (
                      <Card
                        key={tunnel.id}
                        className="group shadow-sm border border-divider hover:shadow-md transition-shadow duration-200"
                      >
                        <CardHeader className="pb-2">
                          <div className="flex justify-between items-start w-full">
                            {selectMode && (
                              <Checkbox
                                isSelected={selectedIds.has(tunnel.id)}
                                onValueChange={() => toggleSelect(tunnel.id)}
                                className="mr-2"
                              />
                            )}
                            <div className="flex-1 min-w-0">
                              <h3 className="font-semibold text-foreground truncate text-sm">
                                {tunnel.name}
                              </h3>
                              <div className="flex items-center gap-1.5 mt-1">
                                <Chip
                                  className="text-xs"
                                  color={typeDisplay.color as any}
                                  size="sm"
                                  variant="flat"
                                >
                                  {typeDisplay.text}
                                </Chip>
                              </div>
                            </div>
                            <div
                              className="cursor-grab active:cursor-grabbing p-2 text-default-400 hover:text-default-600 transition-colors touch-manipulation opacity-100 sm:opacity-0 sm:group-hover:opacity-100"
                              {...listeners}
                              style={{ touchAction: "none" }}
                              title="拖拽排序"
                            >
                              <svg
                                className="w-4 h-4"
                                fill="currentColor"
                                viewBox="0 0 20 20"
                              >
                                <path d="M7 2a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 2zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 8zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 14zm6-8a2 2 0 1 1-.001-4.001A2 2 0 0 1 13 6zm0 2a2 2 0 1 1 .001 4.001A2 2 0 0 1 13 8zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 13 14z" />
                              </svg>
                            </div>
                          </div>
                        </CardHeader>

                        <CardBody className="pt-0 pb-3">
                          <div className="space-y-3">
                            {/* 拓扑结构 */}
                            <div className="pt-2 border-t border-divider">
                              <div className="flex items-center justify-center gap-2 text-xs">
                                {/* 入口节点 */}
                                <div className="flex items-center gap-1 px-2 py-1 bg-primary-50 dark:bg-primary-100/20 rounded border border-primary-200 dark:border-primary-300/20">
                                  <svg
                                    className="w-3 h-3 text-primary-600"
                                    fill="currentColor"
                                    viewBox="0 0 20 20"
                                  >
                                    <path
                                      clipRule="evenodd"
                                      d="M3 4a1 1 0 011-1h12a1 1 0 011 1v12a1 1 0 01-1 1H4a1 1 0 01-1-1V4zm2 2v8h10V6H5z"
                                      fillRule="evenodd"
                                    />
                                  </svg>
                                  <span className="font-semibold text-primary-700 dark:text-primary-400">
                                    {tunnel.inNodeId?.length || 0}入口
                                  </span>
                                </div>

                                {/* 箭头 */}
                                <svg
                                  className="w-4 h-4 text-default-400"
                                  fill="none"
                                  stroke="currentColor"
                                  viewBox="0 0 24 24"
                                >
                                  <path
                                    d="M9 5l7 7-7 7"
                                    strokeLinecap="round"
                                    strokeLinejoin="round"
                                    strokeWidth={2}
                                  />
                                </svg>

                                {/* 转发链 */}
                                <div className="flex items-center gap-1 px-2 py-1 bg-secondary-50 dark:bg-secondary-100/20 rounded border border-secondary-200 dark:border-secondary-300/20">
                                  <svg
                                    className="w-3 h-3 text-secondary-600"
                                    fill="currentColor"
                                    viewBox="0 0 20 20"
                                  >
                                    <path
                                      clipRule="evenodd"
                                      d="M12.586 4.586a2 2 0 112.828 2.828l-3 3a2 2 0 01-2.828 0 1 1 0 00-1.414 1.414 4 4 0 005.656 0l3-3a4 4 0 00-5.656-5.656l-1.5 1.5a1 1 0 101.414 1.414l1.5-1.5zm-5 5a2 2 0 012.828 0 1 1 0 101.414-1.414 4 4 0 00-5.656 0l-3 3a4 4 0 105.656 5.656l1.5-1.5a1 1 0 10-1.414-1.414l-1.5 1.5a2 2 0 11-2.828-2.828l3-3z"
                                      fillRule="evenodd"
                                    />
                                  </svg>
                                  <span className="font-semibold text-secondary-700 dark:text-secondary-400">
                                    {tunnel.type === 2
                                      ? tunnel.chainNodes?.length || 0
                                      : 0}
                                    跳
                                  </span>
                                </div>

                                {/* 箭头 */}
                                <svg
                                  className="w-4 h-4 text-default-400"
                                  fill="none"
                                  stroke="currentColor"
                                  viewBox="0 0 24 24"
                                >
                                  <path
                                    d="M9 5l7 7-7 7"
                                    strokeLinecap="round"
                                    strokeLinejoin="round"
                                    strokeWidth={2}
                                  />
                                </svg>

                                {/* 出口节点 */}
                                <div className="flex items-center gap-1 px-2 py-1 bg-success-50 dark:bg-success-100/20 rounded border border-success-200 dark:border-success-300/20">
                                  <svg
                                    className="w-3 h-3 text-success-600"
                                    fill="currentColor"
                                    viewBox="0 0 20 20"
                                  >
                                    <path
                                      clipRule="evenodd"
                                      d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-8.707l-3-3a1 1 0 00-1.414 0l-3 3a1 1 0 001.414 1.414L9 9.414V13a1 1 0 102 0V9.414l1.293 1.293a1 1 0 001.414-1.414z"
                                      fillRule="evenodd"
                                    />
                                  </svg>
                                  <span className="font-semibold text-success-700 dark:text-success-400">
                                    {tunnel.type === 2
                                      ? tunnel.outNodeId?.length || 0
                                      : tunnel.inNodeId?.length || 0}
                                    出口
                                  </span>
                                </div>
                              </div>
                            </div>

                            {/* 流量配置 */}
                            <div className="grid grid-cols-2 gap-2">
                              <div className="text-center p-1.5 bg-default-50 dark:bg-default-100/30 rounded">
                                <div className="text-xs text-default-500">
                                  流量计算
                                </div>
                                <div className="text-sm font-semibold text-foreground mt-0.5">
                                  {getFlowDisplay(tunnel.flow)}
                                </div>
                              </div>
                              <div className="text-center p-1.5 bg-default-50 dark:bg-default-100/30 rounded">
                                <div className="text-xs text-default-500">
                                  流量倍率
                                </div>
                                <div className="text-sm font-semibold text-foreground mt-0.5">
                                  {tunnel.trafficRatio}x
                                </div>
                              </div>
                            </div>
                          </div>

                          <div className="flex gap-1.5 mt-3">
                            <Button
                              className="flex-1 min-h-8"
                              color="primary"
                              size="sm"
                              startContent={
                                <svg
                                  className="w-3 h-3"
                                  fill="currentColor"
                                  viewBox="0 0 20 20"
                                >
                                  <path d="M13.586 3.586a2 2 0 112.828 2.828l-.793.793-2.828-2.828.793-.793zM11.379 5.793L3 14.172V17h2.828l8.38-8.379-2.83-2.828z" />
                                </svg>
                              }
                              variant="flat"
                              onPress={() => handleEdit(tunnel)}
                            >
                              编辑
                            </Button>
                            <Button
                              className="flex-1 min-h-8"
                              color="warning"
                              size="sm"
                              startContent={
                                <svg
                                  className="w-3 h-3"
                                  fill="currentColor"
                                  viewBox="0 0 20 20"
                                >
                                  <path
                                    clipRule="evenodd"
                                    d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z"
                                    fillRule="evenodd"
                                  />
                                </svg>
                              }
                              variant="flat"
                              onPress={() => handleDiagnose(tunnel)}
                            >
                              诊断
                            </Button>
                            <Button
                              className="flex-1 min-h-8"
                              color="danger"
                              size="sm"
                              startContent={
                                <svg
                                  className="w-3 h-3"
                                  fill="currentColor"
                                  viewBox="0 0 20 20"
                                >
                                  <path
                                    clipRule="evenodd"
                                    d="M9 2a1 1 0 000 2h2a1 1 0 100-2H9z"
                                    fillRule="evenodd"
                                  />
                                  <path
                                    clipRule="evenodd"
                                    d="M10 18a8 8 0 100-16 8 8 0 000 16zM8 7a1 1 0 012 0v4a1 1 0 11-2 0V7zM12 7a1 1 0 012 0v4a1 1 0 11-2 0V7z"
                                    fillRule="evenodd"
                                  />
                                </svg>
                              }
                              variant="flat"
                              onPress={() => handleDelete(tunnel)}
                            >
                              删除
                            </Button>
                          </div>
                        </CardBody>
                      </Card>
                    )}
                  </SortableItem>
                );
              })}
            </div>
          </SortableContext>
        </DndContext>
      ) : (
        /* 空状态 */
        <Card className="shadow-sm border border-gray-200 dark:border-gray-700">
          <CardBody className="text-center py-16">
            <div className="flex flex-col items-center gap-4">
              <div className="w-16 h-16 bg-default-100 rounded-full flex items-center justify-center">
                <svg
                  className="w-8 h-8 text-default-400"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    d="M8.111 16.404a5.5 5.5 0 017.778 0M12 20h.01m-7.08-7.071c3.904-3.905 10.236-3.905 14.141 0M1.394 9.393c5.857-5.857 15.355-5.857 21.213 0"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={1.5}
                  />
                </svg>
              </div>
              <div>
                <h3 className="text-lg font-semibold text-foreground">
                  暂无隧道配置
                </h3>
                <p className="text-default-500 text-sm mt-1">
                  还没有创建任何隧道配置，点击上方按钮开始创建
                </p>
              </div>
            </div>
          </CardBody>
        </Card>
      )}

      {/* 新增/编辑模态框 */}
      <Modal
        backdrop="blur"
        isOpen={modalOpen}
        placement="center"
        scrollBehavior="outside"
        size="2xl"
        onOpenChange={setModalOpen}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                <h2 className="text-xl font-bold">
                  {isEdit ? "编辑隧道" : "新增隧道"}
                </h2>
                <p className="text-small text-default-500">
                  {isEdit
                    ? "修改节点配置会中断现有连接，隧道类型不可修改"
                    : "创建新的隧道配置"}
                </p>
              </ModalHeader>
              <ModalBody>
                <div className="space-y-4">
                  <Input
                    errorMessage={errors.name}
                    isInvalid={!!errors.name}
                    label="隧道名称"
                    placeholder="请输入隧道名称"
                    value={form.name}
                    variant="bordered"
                    onChange={(e) =>
                      setForm((prev) => ({ ...prev, name: e.target.value }))
                    }
                  />

                  <Select
                    description={isEdit ? "编辑时无法修改隧道类型" : undefined}
                    errorMessage={errors.type}
                    isDisabled={isEdit}
                    isInvalid={!!errors.type}
                    label="隧道类型"
                    placeholder="请选择隧道类型"
                    selectedKeys={[form.type.toString()]}
                    variant="bordered"
                    onSelectionChange={(keys) => {
                      const selectedKey = Array.from(keys)[0] as string;

                      if (selectedKey) {
                        handleTypeChange(parseInt(selectedKey));
                      }
                    }}
                  >
                    <SelectItem key="1">端口转发</SelectItem>
                    <SelectItem key="2">隧道转发</SelectItem>
                  </Select>

                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <Select
                      errorMessage={errors.flow}
                      isInvalid={!!errors.flow}
                      label="流量计算"
                      placeholder="请选择流量计算方式"
                      selectedKeys={[form.flow.toString()]}
                      variant="bordered"
                      onSelectionChange={(keys) => {
                        const selectedKey = Array.from(keys)[0] as string;

                        if (selectedKey) {
                          setForm((prev) => ({
                            ...prev,
                            flow: parseInt(selectedKey),
                          }));
                        }
                      }}
                    >
                      <SelectItem key="1">单向计算（仅上传）</SelectItem>
                      <SelectItem key="2">双向计算（上传+下载）</SelectItem>
                    </Select>

                    <Input
                      endContent={
                        <div className="pointer-events-none flex items-center">
                          <span className="text-default-400 text-small">x</span>
                        </div>
                      }
                      errorMessage={errors.trafficRatio}
                      isInvalid={!!errors.trafficRatio}
                      label="流量倍率"
                      placeholder="请输入流量倍率"
                      type="number"
                      value={form.trafficRatio.toString()}
                      variant="bordered"
                      onChange={(e) =>
                        setForm((prev) => ({
                          ...prev,
                          trafficRatio: parseFloat(e.target.value) || 0,
                        }))
                      }
                    />
                  </div>

                  <Textarea
                    description="支持多个IP，每行一个地址,为空时使用入口节点ip"
                    errorMessage={errors.inIp}
                    isInvalid={!!errors.inIp}
                    label="入口IP"
                    maxRows={5}
                    minRows={3}
                    placeholder="一行一个IP地址或域名，例如:&#10;192.168.1.100&#10;example.com"
                    value={form.inIp}
                    variant="bordered"
                    onChange={(e) =>
                      setForm((prev) => ({ ...prev, inIp: e.target.value }))
                    }
                  />

                  <Divider />
                  <h3 className="text-lg font-semibold">入口配置</h3>

                  <div className="space-y-2">
                    <Select
                      disabledKeys={[
                        ...nodes
                          .filter((node) => node.status !== 1)
                          .map((node) => node.id.toString()),
                        ...(form.outNodeId || []).map((ct) =>
                          ct.nodeId.toString(),
                        ),
                        ...getSelectedChainNodeIds().map((id) => id.toString()),
                      ]}
                      errorMessage={errors.inNodeId}
                      isInvalid={!!errors.inNodeId}
                      label="入口节点"
                      placeholder="请选择入口节点（可多选）"
                      selectedKeys={form.inNodeId.map((ct) =>
                        ct.nodeId.toString(),
                      )}
                      selectionMode="multiple"
                      variant="bordered"
                      onSelectionChange={(keys) => {
                        const selectedIds = Array.from(keys).map((key) =>
                          parseInt(key as string),
                        );
                        const newInNodeId: ChainTunnel[] = selectedIds.map(
                          (nodeId) => {
                            // 保留已有的端口配置
                            const existing = form.inNodeId.find(
                              (ct) => ct.nodeId === nodeId,
                            );

                            return existing || { nodeId, chainType: 1 };
                          },
                        );

                        setForm((prev) => ({ ...prev, inNodeId: newInNodeId }));
                      }}
                    >
                      {nodes.map((node) => (
                        <SelectItem key={node.id} textValue={`${node.name}`}>
                          <div className="flex items-center justify-between">
                            <span>{node.name}</span>
                            <div className="flex items-center gap-2">
                              <Chip
                                color={
                                  node.status === 1 ? "success" : "default"
                                }
                                size="sm"
                                variant="flat"
                              >
                                {node.status === 1 ? "在线" : "离线"}
                              </Chip>
                              {form.outNodeId &&
                                form.outNodeId.some(
                                  (ct) => ct.nodeId === node.id,
                                ) && (
                                  <Chip color="danger" size="sm" variant="flat">
                                    已选为出口
                                  </Chip>
                                )}
                              {getSelectedChainNodeIds().includes(node.id) && (
                                <Chip color="primary" size="sm" variant="flat">
                                  已选为转发链
                                </Chip>
                              )}
                            </div>
                          </div>
                        </SelectItem>
                      ))}
                    </Select>
                  </div>

                  {/* 隧道转发时显示转发链配置 */}
                  {form.type === 2 && (
                    <>
                      <Divider />
                      <div className="flex items-center justify-between">
                        <h3 className="text-lg font-semibold">转发链配置</h3>
                        <Button
                          color="primary"
                          size="sm"
                          startContent={
                            <svg
                              className="w-4 h-4"
                              fill="none"
                              stroke="currentColor"
                              viewBox="0 0 24 24"
                            >
                              <path
                                d="M12 4v16m8-8H4"
                                strokeLinecap="round"
                                strokeLinejoin="round"
                                strokeWidth={2}
                              />
                            </svg>
                          }
                          variant="flat"
                          onPress={() => {
                            // 添加新的一跳（一个空组，或包含占位节点）
                            setForm((prev) => ({
                              ...prev,
                              chainNodes: [
                                ...(prev.chainNodes || []),
                                [
                                  {
                                    nodeId: -1,
                                    chainType: 2,
                                    protocol: "tls",
                                    strategy: "round",
                                  },
                                ],
                              ],
                            }));
                          }}
                        >
                          添加一跳
                        </Button>
                      </div>

                      {getChainGroups().length > 0 && (
                        <div className="space-y-3">
                          {getChainGroups().map((groupNodes, groupIndex) => {
                            const protocol =
                              groupNodes.length > 0
                                ? groupNodes[0].protocol || "tls"
                                : "tls";
                            const strategy =
                              groupNodes.length > 0
                                ? groupNodes[0].strategy || "round"
                                : "round";

                            return (
                              <div
                                key={groupIndex}
                                className="border border-default-200 rounded-lg p-3"
                              >
                                <div className="flex items-center justify-between mb-2">
                                  <span className="text-sm font-medium text-default-600">
                                    第{groupIndex + 1}跳
                                  </span>
                                  <Button
                                    isIconOnly
                                    color="danger"
                                    size="sm"
                                    variant="light"
                                    onPress={() => removeChainNode(groupIndex)}
                                  >
                                    <svg
                                      className="w-4 h-4"
                                      fill="none"
                                      stroke="currentColor"
                                      viewBox="0 0 24 24"
                                    >
                                      <path
                                        d="M6 18L18 6M6 6l12 12"
                                        strokeLinecap="round"
                                        strokeLinejoin="round"
                                        strokeWidth={2}
                                      />
                                    </svg>
                                  </Button>
                                </div>

                                <div className="grid grid-cols-1 md:grid-cols-4 gap-2">
                                  {/* 节点选择 - 移动端100%，桌面端50% */}
                                  <div className="col-span-1 md:col-span-2">
                                    <Select
                                      classNames={{
                                        label: "text-xs",
                                        value: "text-sm",
                                      }}
                                      disabledKeys={[
                                        ...nodes
                                          .filter((node) => node.status !== 1)
                                          .map((node) => node.id.toString()),
                                        ...form.inNodeId.map((ct) =>
                                          ct.nodeId.toString(),
                                        ),
                                        ...(form.outNodeId || []).map((ct) =>
                                          ct.nodeId.toString(),
                                        ),
                                        // 排除其他跳数已选的节点
                                        ...(form.chainNodes || [])
                                          .flatMap((group, idx) =>
                                            idx !== groupIndex
                                              ? group.map((ct) => ct.nodeId)
                                              : [],
                                          )
                                          .filter((id) => id !== -1)
                                          .map((id) => id.toString()),
                                      ]}
                                      label="节点"
                                      placeholder="选择节点（可多选）"
                                      selectedKeys={groupNodes
                                        .filter((ct) => ct.nodeId !== -1)
                                        .map((ct) => ct.nodeId.toString())}
                                      selectionMode="multiple"
                                      size="sm"
                                      variant="bordered"
                                      onSelectionChange={(keys) => {
                                        const selectedIds = Array.from(
                                          keys,
                                        ).map((key) => parseInt(key as string));
                                        const currentNodes = groupNodes.filter(
                                          (ct) => ct.nodeId !== -1,
                                        );

                                        // 找出新增的节点
                                        const currentNodeIds = currentNodes.map(
                                          (ct) => ct.nodeId,
                                        );
                                        const addedIds = selectedIds.filter(
                                          (id) => !currentNodeIds.includes(id),
                                        );
                                        const removedIds =
                                          currentNodeIds.filter(
                                            (id) => !selectedIds.includes(id),
                                          );

                                        // 添加新节点
                                        addedIds.forEach((nodeId) =>
                                          addNodeToChain(groupIndex, nodeId),
                                        );

                                        // 删除取消选择的节点
                                        removedIds.forEach((nodeId) =>
                                          removeNodeFromChain(
                                            groupIndex,
                                            nodeId,
                                          ),
                                        );
                                      }}
                                    >
                                      {nodes.map((node) => (
                                        <SelectItem
                                          key={node.id}
                                          textValue={`${node.name}`}
                                        >
                                          <div className="flex items-center justify-between">
                                            <span className="text-sm">
                                              {node.name}
                                            </span>
                                            <div className="flex items-center gap-2">
                                              <Chip
                                                color={
                                                  node.status === 1
                                                    ? "success"
                                                    : "default"
                                                }
                                                size="sm"
                                                variant="flat"
                                              >
                                                {node.status === 1
                                                  ? "在线"
                                                  : "离线"}
                                              </Chip>
                                              {form.inNodeId.some(
                                                (ct) => ct.nodeId === node.id,
                                              ) && (
                                                <Chip
                                                  color="warning"
                                                  size="sm"
                                                  variant="flat"
                                                >
                                                  已选为入口
                                                </Chip>
                                              )}
                                              {form.outNodeId &&
                                                form.outNodeId.some(
                                                  (ct) => ct.nodeId === node.id,
                                                ) && (
                                                  <Chip
                                                    color="danger"
                                                    size="sm"
                                                    variant="flat"
                                                  >
                                                    已选为出口
                                                  </Chip>
                                                )}
                                              {/* 显示是否在其他跳数中被选择 */}
                                              {(form.chainNodes || []).some(
                                                (group, idx) =>
                                                  idx !== groupIndex &&
                                                  group.some(
                                                    (ct) =>
                                                      ct.nodeId === node.id &&
                                                      ct.nodeId !== -1,
                                                  ),
                                              ) && (
                                                <Chip
                                                  color="primary"
                                                  size="sm"
                                                  variant="flat"
                                                >
                                                  已选为其他跳
                                                </Chip>
                                              )}
                                            </div>
                                          </div>
                                        </SelectItem>
                                      ))}
                                    </Select>
                                  </div>

                                  {/* 协议选择 - 25% */}
                                  <Select
                                    classNames={{
                                      label: "text-xs",
                                      value: "text-sm",
                                    }}
                                    label="协议"
                                    placeholder="选择协议"
                                    selectedKeys={[protocol]}
                                    size="sm"
                                    variant="bordered"
                                    onSelectionChange={(keys) => {
                                      const selectedKey = Array.from(
                                        keys,
                                      )[0] as string;

                                      if (selectedKey) {
                                        updateChainProtocol(
                                          groupIndex,
                                          selectedKey,
                                        );
                                      }
                                    }}
                                  >
                                    <SelectItem key="tls">TLS</SelectItem>
                                    <SelectItem key="wss">WSS</SelectItem>
                                    <SelectItem key="tcp">TCP</SelectItem>
                                    <SelectItem key="mtls">MTLS</SelectItem>
                                    <SelectItem key="mwss">MWSS</SelectItem>
                                    <SelectItem key="mtcp">MTCP</SelectItem>
                                  </Select>

                                  {/* 负载策略 - 25% */}
                                  <Select
                                    classNames={{
                                      label: "text-xs",
                                      value: "text-sm",
                                    }}
                                    label="负载策略"
                                    placeholder="选择策略"
                                    selectedKeys={[strategy]}
                                    size="sm"
                                    variant="bordered"
                                    onSelectionChange={(keys) => {
                                      const selectedKey = Array.from(
                                        keys,
                                      )[0] as string;

                                      if (selectedKey) {
                                        updateChainStrategy(
                                          groupIndex,
                                          selectedKey,
                                        );
                                      }
                                    }}
                                  >
                                    <SelectItem key="fifo">主备</SelectItem>
                                    <SelectItem key="round">轮询</SelectItem>
                                    <SelectItem key="rand">随机</SelectItem>
                                  </Select>
                                </div>
                              </div>
                            );
                          })}
                        </div>
                      )}

                      {getChainGroups().length === 0 && (
                        <div className="text-center py-8 bg-default-50 dark:bg-default-100/50 rounded border border-dashed border-default-300">
                          <p className="text-sm text-default-500">
                            还没有添加转发链，点击上方"添加一跳"按钮开始添加
                          </p>
                        </div>
                      )}
                    </>
                  )}

                  {/* 隧道转发时显示出口配置 */}
                  {form.type === 2 && (
                    <>
                      <Divider />
                      <h3 className="text-lg font-semibold">出口配置</h3>

                      <div className="grid grid-cols-1 md:grid-cols-4 gap-2">
                        {/* 节点选择 - 移动端100%，桌面端50% */}
                        <div className="col-span-1 md:col-span-2">
                          <Select
                            classNames={{
                              label: "text-xs",
                              value: "text-sm",
                            }}
                            disabledKeys={[
                              ...nodes
                                .filter((node) => node.status !== 1)
                                .map((node) => node.id.toString()),
                              ...form.inNodeId.map((ct) =>
                                ct.nodeId.toString(),
                              ),
                              ...getSelectedChainNodeIds().map((id) =>
                                id.toString(),
                              ),
                            ]}
                            errorMessage={errors.outNodeId}
                            isInvalid={!!errors.outNodeId}
                            label="节点"
                            placeholder="请选择出口节点（可多选）"
                            selectedKeys={
                              form.outNodeId
                                ? form.outNodeId
                                    .filter((ct) => ct.nodeId !== -1)
                                    .map((ct) => ct.nodeId.toString())
                                : []
                            }
                            selectionMode="multiple"
                            variant="bordered"
                            onSelectionChange={(keys) => {
                              const selectedIds = Array.from(keys).map((key) =>
                                parseInt(key as string),
                              );
                              const currentOutNodes = form.outNodeId || [];

                              let protocol = "tls";
                              let strategy = "round";

                              if (currentOutNodes.length > 0) {
                                protocol = currentOutNodes[0].protocol || "tls";
                                strategy =
                                  currentOutNodes[0].strategy || "round";
                              }

                              const realNodes = currentOutNodes.filter(
                                (ct) => ct.nodeId !== -1,
                              );
                              const newOutNodeId: ChainTunnel[] =
                                selectedIds.map((nodeId) => {
                                  const existing = realNodes.find(
                                    (ct) => ct.nodeId === nodeId,
                                  );

                                  return (
                                    existing || {
                                      nodeId,
                                      chainType: 3,
                                      protocol,
                                      strategy,
                                    }
                                  );
                                });

                              setForm((prev) => ({
                                ...prev,
                                outNodeId: newOutNodeId,
                              }));
                            }}
                          >
                            {nodes.map((node) => (
                              <SelectItem
                                key={node.id}
                                textValue={`${node.name}`}
                              >
                                <div className="flex items-center justify-between">
                                  <span>{node.name}</span>
                                  <div className="flex items-center gap-2">
                                    <Chip
                                      color={
                                        node.status === 1
                                          ? "success"
                                          : "default"
                                      }
                                      size="sm"
                                      variant="flat"
                                    >
                                      {node.status === 1 ? "在线" : "离线"}
                                    </Chip>
                                    {form.inNodeId.some(
                                      (ct) => ct.nodeId === node.id,
                                    ) && (
                                      <Chip
                                        color="warning"
                                        size="sm"
                                        variant="flat"
                                      >
                                        已选为入口
                                      </Chip>
                                    )}
                                    {getSelectedChainNodeIds().includes(
                                      node.id,
                                    ) && (
                                      <Chip
                                        color="primary"
                                        size="sm"
                                        variant="flat"
                                      >
                                        已选为转发链
                                      </Chip>
                                    )}
                                  </div>
                                </div>
                              </SelectItem>
                            ))}
                          </Select>
                        </div>

                        {/* 协议选择 - 25% */}
                        <Select
                          classNames={{
                            label: "text-xs",
                            value: "text-sm",
                          }}
                          errorMessage={errors.protocol}
                          isInvalid={!!errors.protocol}
                          label="协议"
                          placeholder="选择协议"
                          selectedKeys={[
                            (() => {
                              if (
                                !form.outNodeId ||
                                form.outNodeId.length === 0
                              )
                                return "tls";

                              return form.outNodeId[0].protocol || "tls";
                            })(),
                          ]}
                          variant="bordered"
                          onSelectionChange={(keys) => {
                            const selectedKey = Array.from(keys)[0] as string;

                            if (selectedKey) {
                              setForm((prev) => {
                                const currentOutNodes = prev.outNodeId || [];
                                const currentStrategy =
                                  currentOutNodes.length > 0
                                    ? currentOutNodes[0].strategy || "round"
                                    : "round";

                                if (currentOutNodes.length === 0) {
                                  // 如果还没有出口节点，创建一个占位节点保存设置
                                  return {
                                    ...prev,
                                    outNodeId: [
                                      {
                                        nodeId: -1,
                                        chainType: 3,
                                        protocol: selectedKey,
                                        strategy: currentStrategy,
                                      },
                                    ],
                                  };
                                }

                                // 更新所有出口节点的协议
                                return {
                                  ...prev,
                                  outNodeId: currentOutNodes.map((ct) => ({
                                    ...ct,
                                    protocol: selectedKey,
                                  })),
                                };
                              });
                            }
                          }}
                        >
                          <SelectItem key="tls">TLS</SelectItem>
                          <SelectItem key="wss">WSS</SelectItem>
                          <SelectItem key="tcp">TCP</SelectItem>
                          <SelectItem key="mtls">MTLS</SelectItem>
                          <SelectItem key="mwss">MWSS</SelectItem>
                          <SelectItem key="mtcp">MTCP</SelectItem>
                        </Select>

                        {/* 负载策略 - 25% */}
                        <Select
                          classNames={{
                            label: "text-xs",
                            value: "text-sm",
                          }}
                          label="负载策略"
                          placeholder="选择策略"
                          selectedKeys={[
                            (() => {
                              if (
                                !form.outNodeId ||
                                form.outNodeId.length === 0
                              )
                                return "round";

                              return form.outNodeId[0].strategy || "round";
                            })(),
                          ]}
                          variant="bordered"
                          onSelectionChange={(keys) => {
                            const selectedKey = Array.from(keys)[0] as string;

                            if (selectedKey) {
                              setForm((prev) => {
                                const currentOutNodes = prev.outNodeId || [];
                                const currentProtocol =
                                  currentOutNodes.length > 0
                                    ? currentOutNodes[0].protocol || "tls"
                                    : "tls";

                                if (currentOutNodes.length === 0) {
                                  return {
                                    ...prev,
                                    outNodeId: [
                                      {
                                        nodeId: -1,
                                        chainType: 3,
                                        protocol: currentProtocol,
                                        strategy: selectedKey,
                                      },
                                    ],
                                  };
                                }

                                return {
                                  ...prev,
                                  outNodeId: currentOutNodes.map((ct) => ({
                                    ...ct,
                                    strategy: selectedKey,
                                  })),
                                };
                              });
                            }
                          }}
                        >
                          <SelectItem key="fifo">主备</SelectItem>
                          <SelectItem key="round">轮询</SelectItem>
                          <SelectItem key="rand">随机</SelectItem>
                        </Select>
                      </div>
                    </>
                  )}
                </div>
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  取消
                </Button>
                <Button
                  color="primary"
                  isLoading={submitLoading}
                  onPress={handleSubmit}
                >
                  {submitLoading
                    ? isEdit
                      ? "更新中..."
                      : "创建中..."
                    : isEdit
                      ? "更新"
                      : "创建"}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {/* 删除确认模态框 */}
      <Modal
        backdrop="blur"
        isOpen={deleteModalOpen}
        placement="center"
        scrollBehavior="outside"
        size="2xl"
        onOpenChange={setDeleteModalOpen}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                <h2 className="text-xl font-bold">确认删除</h2>
              </ModalHeader>
              <ModalBody>
                <p>
                  确定要删除隧道 <strong>"{tunnelToDelete?.name}"</strong> 吗？
                </p>
                <p className="text-small text-default-500">
                  此操作不可恢复，请谨慎操作。
                </p>
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  取消
                </Button>
                <Button
                  color="danger"
                  isLoading={deleteLoading}
                  onPress={confirmDelete}
                >
                  {deleteLoading ? "删除中..." : "确认删除"}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {/* 诊断结果模态框 */}
      <Modal
        backdrop="blur"
        classNames={{
          base: "rounded-2xl",
          header: "rounded-t-2xl",
          body: "rounded-none",
          footer: "rounded-b-2xl",
        }}
        isOpen={diagnosisModalOpen}
        placement="center"
        scrollBehavior="inside"
        size="4xl"
        onOpenChange={setDiagnosisModalOpen}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1 bg-content1 border-b border-divider">
                <h2 className="text-xl font-bold">隧道诊断结果</h2>
                {currentDiagnosisTunnel && (
                  <div className="flex items-center gap-2">
                    <span className="text-small text-default-500">
                      {currentDiagnosisTunnel.name}
                    </span>
                    <Chip
                      color={
                        currentDiagnosisTunnel.type === 1
                          ? "primary"
                          : "secondary"
                      }
                      size="sm"
                      variant="flat"
                    >
                      {currentDiagnosisTunnel.type === 1
                        ? "端口转发"
                        : "隧道转发"}
                    </Chip>
                  </div>
                )}
              </ModalHeader>
              <ModalBody className="bg-content1">
                {diagnosisLoading ? (
                  <div className="flex items-center justify-center py-16">
                    <div className="flex items-center gap-3">
                      <Spinner size="sm" />
                      <span className="text-default-600">正在诊断...</span>
                    </div>
                  </div>
                ) : diagnosisResult ? (
                  <div className="space-y-4">
                    {/* 统计摘要 */}
                    <div className="grid grid-cols-3 gap-3">
                      <div className="text-center p-3 bg-default-100 dark:bg-gray-800 rounded-lg border border-divider">
                        <div className="text-2xl font-bold text-foreground">
                          {diagnosisResult.results.length}
                        </div>
                        <div className="text-xs text-default-500 mt-1">
                          总测试数
                        </div>
                      </div>
                      <div className="text-center p-3 bg-success-50 dark:bg-success-900/20 rounded-lg border border-success-200 dark:border-success-700">
                        <div className="text-2xl font-bold text-success-600 dark:text-success-400">
                          {
                            diagnosisResult.results.filter((r) => r.success)
                              .length
                          }
                        </div>
                        <div className="text-xs text-success-600 dark:text-success-400/80 mt-1">
                          成功
                        </div>
                      </div>
                      <div className="text-center p-3 bg-danger-50 dark:bg-danger-900/20 rounded-lg border border-danger-200 dark:border-danger-700">
                        <div className="text-2xl font-bold text-danger-600 dark:text-danger-400">
                          {
                            diagnosisResult.results.filter((r) => !r.success)
                              .length
                          }
                        </div>
                        <div className="text-xs text-danger-600 dark:text-danger-400/80 mt-1">
                          失败
                        </div>
                      </div>
                    </div>

                    {/* 桌面端表格展示 */}
                    <div className="hidden md:block space-y-3">
                      {(() => {
                        // 使用后端返回的 chainType 和 inx 字段进行分组
                        const groupedResults = {
                          entry: diagnosisResult.results.filter(
                            (r) => r.fromChainType === 1,
                          ),
                          chains: {} as Record<
                            number,
                            typeof diagnosisResult.results
                          >,
                          exit: diagnosisResult.results.filter(
                            (r) => r.fromChainType === 3,
                          ),
                        };

                        // 按 inx 分组链路测试
                        diagnosisResult.results.forEach((r) => {
                          if (r.fromChainType === 2 && r.fromInx != null) {
                            if (!groupedResults.chains[r.fromInx]) {
                              groupedResults.chains[r.fromInx] = [];
                            }
                            groupedResults.chains[r.fromInx].push(r);
                          }
                        });

                        const renderTableSection = (
                          title: string,
                          results: typeof diagnosisResult.results,
                        ) => {
                          if (results.length === 0) return null;

                          return (
                            <div
                              key={title}
                              className="border border-divider rounded-lg overflow-hidden bg-white dark:bg-gray-800"
                            >
                              <div className="bg-primary/10 dark:bg-primary/20 px-3 py-2 border-b border-divider">
                                <h3 className="text-sm font-semibold text-primary">
                                  {title}
                                </h3>
                              </div>
                              <table className="w-full text-sm">
                                <thead className="bg-default-100 dark:bg-gray-700">
                                  <tr>
                                    <th className="px-3 py-2 text-left font-semibold text-xs">
                                      路径
                                    </th>
                                    <th className="px-3 py-2 text-center font-semibold text-xs w-20">
                                      状态
                                    </th>
                                    <th className="px-3 py-2 text-center font-semibold text-xs w-24">
                                      延迟(ms)
                                    </th>
                                    <th className="px-3 py-2 text-center font-semibold text-xs w-24">
                                      丢包率
                                    </th>
                                    <th className="px-3 py-2 text-center font-semibold text-xs w-20">
                                      质量
                                    </th>
                                  </tr>
                                </thead>
                                <tbody className="divide-y divide-divider bg-white dark:bg-gray-800">
                                  {results.map((result, index) => {
                                    const quality = getQualityDisplay(
                                      result.averageTime,
                                      result.packetLoss,
                                    );

                                    return (
                                      <tr
                                        key={index}
                                        className={`hover:bg-default-50 dark:hover:bg-gray-700/50 ${
                                          result.success
                                            ? "bg-white dark:bg-gray-800"
                                            : "bg-danger-50 dark:bg-danger-900/30"
                                        }`}
                                      >
                                        <td className="px-3 py-2">
                                          <div className="flex items-center gap-2">
                                            <span
                                              className={`w-5 h-5 rounded-full flex items-center justify-center text-xs ${
                                                result.success
                                                  ? "bg-success text-white"
                                                  : "bg-danger text-white"
                                              }`}
                                            >
                                              {result.success ? "✓" : "✗"}
                                            </span>
                                            <div className="flex-1 min-w-0">
                                              <div className="font-medium text-foreground truncate">
                                                {result.description}
                                              </div>
                                              <div className="text-xs text-default-500 truncate">
                                                {result.targetIp}:
                                                {result.targetPort}
                                              </div>
                                            </div>
                                          </div>
                                        </td>
                                        <td className="px-3 py-2 text-center">
                                          <Chip
                                            className="min-w-[50px]"
                                            color={
                                              result.success
                                                ? "success"
                                                : "danger"
                                            }
                                            size="sm"
                                            variant="flat"
                                          >
                                            {result.success ? "成功" : "失败"}
                                          </Chip>
                                        </td>
                                        <td className="px-3 py-2 text-center">
                                          {result.success ? (
                                            <span className="font-semibold text-primary">
                                              {result.averageTime?.toFixed(0)}
                                            </span>
                                          ) : (
                                            <span className="text-default-400">
                                              -
                                            </span>
                                          )}
                                        </td>
                                        <td className="px-3 py-2 text-center">
                                          {result.success ? (
                                            <span
                                              className={`font-semibold ${
                                                (result.packetLoss || 0) > 0
                                                  ? "text-warning"
                                                  : "text-success"
                                              }`}
                                            >
                                              {result.packetLoss?.toFixed(1)}%
                                            </span>
                                          ) : (
                                            <span className="text-default-400">
                                              -
                                            </span>
                                          )}
                                        </td>
                                        <td className="px-3 py-2 text-center">
                                          {result.success && quality ? (
                                            <Chip
                                              className="text-xs"
                                              color={quality.color as any}
                                              size="sm"
                                              variant="flat"
                                            >
                                              {quality.text}
                                            </Chip>
                                          ) : (
                                            <span className="text-default-400">
                                              -
                                            </span>
                                          )}
                                        </td>
                                      </tr>
                                    );
                                  })}
                                </tbody>
                              </table>
                            </div>
                          );
                        };

                        return (
                          <>
                            {/* 入口测试 */}
                            {renderTableSection(
                              "🚪 入口测试",
                              groupedResults.entry,
                            )}

                            {/* 链路测试（按跳数排序） */}
                            {Object.keys(groupedResults.chains)
                              .map(Number)
                              .sort((a, b) => a - b)
                              .map((hop) =>
                                renderTableSection(
                                  `🔗 转发链 - 第${hop}跳`,
                                  groupedResults.chains[hop],
                                ),
                              )}

                            {/* 出口测试 */}
                            {renderTableSection(
                              "🚀 出口测试",
                              groupedResults.exit,
                            )}
                          </>
                        );
                      })()}
                    </div>

                    {/* 移动端卡片展示 */}
                    <div className="md:hidden space-y-3">
                      {(() => {
                        // 使用后端返回的 chainType 和 inx 字段进行分组
                        const groupedResults = {
                          entry: diagnosisResult.results.filter(
                            (r) => r.fromChainType === 1,
                          ),
                          chains: {} as Record<
                            number,
                            typeof diagnosisResult.results
                          >,
                          exit: diagnosisResult.results.filter(
                            (r) => r.fromChainType === 3,
                          ),
                        };

                        // 按 inx 分组链路测试
                        diagnosisResult.results.forEach((r) => {
                          if (r.fromChainType === 2 && r.fromInx != null) {
                            if (!groupedResults.chains[r.fromInx]) {
                              groupedResults.chains[r.fromInx] = [];
                            }
                            groupedResults.chains[r.fromInx].push(r);
                          }
                        });

                        const renderCardSection = (
                          title: string,
                          results: typeof diagnosisResult.results,
                        ) => {
                          if (results.length === 0) return null;

                          return (
                            <div key={title} className="space-y-2">
                              <div className="px-2 py-1.5 bg-primary/10 dark:bg-primary/20 rounded-lg border border-primary/30">
                                <h3 className="text-sm font-semibold text-primary">
                                  {title}
                                </h3>
                              </div>
                              {results.map((result, index) => {
                                const quality = getQualityDisplay(
                                  result.averageTime,
                                  result.packetLoss,
                                );

                                return (
                                  <div
                                    key={index}
                                    className={`border rounded-lg p-3 ${
                                      result.success
                                        ? "border-divider bg-white dark:bg-gray-800"
                                        : "border-danger-200 dark:border-danger-300/30 bg-danger-50 dark:bg-danger-900/30"
                                    }`}
                                  >
                                    <div className="flex items-start gap-2 mb-2">
                                      <span
                                        className={`w-6 h-6 rounded-full flex items-center justify-center text-xs flex-shrink-0 ${
                                          result.success
                                            ? "bg-success text-white"
                                            : "bg-danger text-white"
                                        }`}
                                      >
                                        {result.success ? "✓" : "✗"}
                                      </span>
                                      <div className="flex-1 min-w-0">
                                        <div className="font-semibold text-sm text-foreground break-words">
                                          {result.description}
                                        </div>
                                        <div className="text-xs text-default-500 mt-0.5 break-all">
                                          {result.targetIp}:{result.targetPort}
                                        </div>
                                      </div>
                                      <Chip
                                        className="flex-shrink-0"
                                        color={
                                          result.success ? "success" : "danger"
                                        }
                                        size="sm"
                                        variant="flat"
                                      >
                                        {result.success ? "成功" : "失败"}
                                      </Chip>
                                    </div>

                                    {result.success ? (
                                      <div className="grid grid-cols-3 gap-2 mt-2 pt-2 border-t border-divider">
                                        <div className="text-center">
                                          <div className="text-lg font-bold text-primary">
                                            {result.averageTime?.toFixed(0)}
                                          </div>
                                          <div className="text-xs text-default-500">
                                            延迟(ms)
                                          </div>
                                        </div>
                                        <div className="text-center">
                                          <div
                                            className={`text-lg font-bold ${
                                              (result.packetLoss || 0) > 0
                                                ? "text-warning"
                                                : "text-success"
                                            }`}
                                          >
                                            {result.packetLoss?.toFixed(1)}%
                                          </div>
                                          <div className="text-xs text-default-500">
                                            丢包率
                                          </div>
                                        </div>
                                        <div className="text-center">
                                          {quality && (
                                            <>
                                              <Chip
                                                className="text-xs"
                                                color={quality.color as any}
                                                size="sm"
                                                variant="flat"
                                              >
                                                {quality.text}
                                              </Chip>
                                              <div className="text-xs text-default-500 mt-0.5">
                                                质量
                                              </div>
                                            </>
                                          )}
                                        </div>
                                      </div>
                                    ) : (
                                      <div className="mt-2 pt-2 border-t border-divider">
                                        <div className="text-xs text-danger">
                                          {result.message || "连接失败"}
                                        </div>
                                      </div>
                                    )}
                                  </div>
                                );
                              })}
                            </div>
                          );
                        };

                        return (
                          <>
                            {/* 入口测试 */}
                            {renderCardSection(
                              "🚪 入口测试",
                              groupedResults.entry,
                            )}

                            {/* 链路测试（按跳数排序） */}
                            {Object.keys(groupedResults.chains)
                              .map(Number)
                              .sort((a, b) => a - b)
                              .map((hop) =>
                                renderCardSection(
                                  `🔗 转发链 - 第${hop}跳`,
                                  groupedResults.chains[hop],
                                ),
                              )}

                            {/* 出口测试 */}
                            {renderCardSection(
                              "🚀 出口测试",
                              groupedResults.exit,
                            )}
                          </>
                        );
                      })()}
                    </div>

                    {/* 失败详情（仅桌面端显示，移动端已在卡片中显示） */}
                    {diagnosisResult.results.some((r) => !r.success) && (
                      <div className="space-y-2 hidden md:block">
                        <h4 className="text-sm font-semibold text-danger">
                          失败详情
                        </h4>
                        <div className="space-y-2">
                          {diagnosisResult.results
                            .filter((r) => !r.success)
                            .map((result, index) => (
                              <Alert
                                key={index}
                                className="text-xs"
                                color="danger"
                                description={result.message || "连接失败"}
                                title={result.description}
                                variant="flat"
                              />
                            ))}
                        </div>
                      </div>
                    )}
                  </div>
                ) : (
                  <div className="text-center py-16">
                    <div className="w-16 h-16 bg-default-100 rounded-full flex items-center justify-center mx-auto mb-4">
                      <svg
                        className="w-8 h-8 text-default-400"
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path
                          d="M9.75 9.75l4.5 4.5m0-4.5l-4.5 4.5M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          strokeWidth={1.5}
                        />
                      </svg>
                    </div>
                    <h3 className="text-lg font-semibold text-foreground">
                      暂无诊断数据
                    </h3>
                  </div>
                )}
              </ModalBody>
              <ModalFooter className="bg-content1 border-t border-divider">
                <Button variant="light" onPress={onClose}>
                  关闭
                </Button>
                {currentDiagnosisTunnel && (
                  <Button
                    color="primary"
                    isLoading={diagnosisLoading}
                    onPress={() => handleDiagnose(currentDiagnosisTunnel)}
                  >
                    重新诊断
                  </Button>
                )}
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      <Modal isOpen={batchDeleteModalOpen} onOpenChange={setBatchDeleteModalOpen}>
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader>确认批量删除</ModalHeader>
              <ModalBody>
                <p>确定要删除选中的 {selectedIds.size} 项隧道吗？此操作不可撤销，相关转发也将被删除。</p>
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  取消
                </Button>
                <Button
                  color="danger"
                  onPress={handleBatchDelete}
                  isLoading={batchLoading}
                >
                  确认删除
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </div>
  );
}
