import type {
  BatchOperationFailure,
  TunnelBatchDeletePreviewApiData,
  TunnelDeletePreviewApiData,
} from "@/api/types";

import { useState, useEffect, useMemo, useRef, useCallback } from "react";
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

import { SearchBar } from "@/components/search-bar";
import { AnimatedPage } from "@/components/animated-page";
import { BatchActionResultModal } from "@/components/batch-action-result-modal";
import { Card, CardBody, CardHeader } from "@/shadcn-bridge/heroui/card";
import { Button } from "@/shadcn-bridge/heroui/button";
import { Input, Textarea } from "@/shadcn-bridge/heroui/input";
import { Select, SelectItem } from "@/shadcn-bridge/heroui/select";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
} from "@/shadcn-bridge/heroui/modal";
import { Chip } from "@/shadcn-bridge/heroui/chip";
import { Spinner } from "@/shadcn-bridge/heroui/spinner";
import { Divider } from "@/shadcn-bridge/heroui/divider";
import { Alert } from "@/shadcn-bridge/heroui/alert";
import { Checkbox } from "@/shadcn-bridge/heroui/checkbox";
import { Progress } from "@/shadcn-bridge/heroui/progress";
import { Radio, RadioGroup } from "@/shadcn-bridge/heroui/radio";
import { LayoutGrid, List } from "lucide-react";
import {
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
} from "@/shadcn-bridge/heroui/table";
import {
  createTunnel,
  batchDeleteTunnelsWithForwards,
  getTunnelList,
  updateTunnel,
  deleteTunnelWithForwards,
  getNodeList,
  diagnoseTunnel,
  updateTunnelOrder,
  batchRedeployTunnels,
  previewBatchTunnelDelete,
  previewTunnelDelete,
} from "@/api";
import { PageLoadingState } from "@/components/page-state";
import {
  buildDiagnosisFallbackResult,
  getDiagnosisQualityDisplay,
  type DiagnosisResult,
} from "@/pages/tunnel/diagnosis";
import { diagnoseTunnelStream } from "@/api/diagnosis-stream";
import {
  createTunnelFormDefaults,
  getTunnelFlowDisplay,
  getTunnelTypeDisplay,
  validateTunnelForm,
} from "@/pages/tunnel/form";
import { useLocalStorageState } from "@/hooks/use-local-storage-state";
import { loadStoredOrder, saveOrder } from "@/utils/order-storage";
import {
  buildBatchFailureMessage,
  extractBatchFailures,
  extractApiErrorMessage,
} from "@/api/error-message";

interface ChainTunnel {
  nodeId: number;
  protocol?: string; // 'tls' | 'wss' | 'tcp' | 'mtls' | 'mwss' | 'mtcp' - 转发链协议
  strategy?: string; // 'fifo' | 'round' | 'rand' - 仅转发链需要
  chainType?: number; // 1: 入口, 2: 转发链, 3: 出口
  inx?: number; // 转发链序号
  connectIp?: string; // 连接IP（多IP节点指定连接地址）
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
  ipPreference?: string;
  status: number;
  createdTime: string;
}

interface Node {
  id: number;
  name: string;
  status: number; // 1: 在线, 0: 离线
  serverIp?: string;
  serverIpV4?: string;
  serverIpV6?: string;
  extraIPs?: string;
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
  ipPreference: string;
  status: number;
}

interface BatchProgressState {
  active: boolean;
  label: string;
  percent: number;
}

interface BatchResultModalState {
  failures: BatchOperationFailure[];
  open: boolean;
  summary: string;
  title: string;
}

type TunnelDeleteAction = "replace" | "delete_forwards";

const EMPTY_BATCH_RESULT_MODAL_STATE: BatchResultModalState = {
  failures: [],
  open: false,
  summary: "",
  title: "",
};

const DEFAULT_TUNNEL_DELETE_ACTION: TunnelDeleteAction = "replace";

const TUNNEL_ORDER_KEY = "tunnel-order";

const mapTunnelApiItems = (items: any[]): Tunnel[] => {
  return (items || []).map((tunnel) => ({
    ...tunnel,
    inx: tunnel.inx ?? 0,
    inNodeId: Array.isArray(tunnel.inNodeId) ? tunnel.inNodeId : [],
    outNodeId: Array.isArray(tunnel.outNodeId) ? tunnel.outNodeId : [],
    chainNodes: Array.isArray(tunnel.chainNodes) ? tunnel.chainNodes : [],
    inIp: tunnel.inIp || "",
    flow: tunnel.flow ?? 1,
    trafficRatio: tunnel.trafficRatio ?? 1,
    status: typeof tunnel.status === "number" ? tunnel.status : 0,
    createdTime: tunnel.createdTime || "",
  }));
};

export default function TunnelPage() {
  const [loading, setLoading] = useState(true);
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [tunnelOrder, setTunnelOrder] = useState<number[]>([]);
  const [nodes, setNodes] = useState<Node[]>([]);
  const [searchKeyword, setSearchKeyword] = useLocalStorageState(
    "tunnel-search-keyword",
    "",
  );
  const [isSearchVisible, setIsSearchVisible] = useState(false);
  const [viewMode, setViewMode] = useLocalStorageState<"list" | "grid">(
    "tunnel-view-mode",
    "grid",
  );

  // 模态框状态
  const [modalOpen, setModalOpen] = useState(false);
  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [diagnosisModalOpen, setDiagnosisModalOpen] = useState(false);
  const [isEdit, setIsEdit] = useState(false);
  const [submitLoading, setSubmitLoading] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [deletePreviewLoading, setDeletePreviewLoading] = useState(false);
  const [diagnosisLoading, setDiagnosisLoading] = useState(false);
  const [tunnelToDelete, setTunnelToDelete] = useState<Tunnel | null>(null);
  const [tunnelDeletePreview, setTunnelDeletePreview] =
    useState<TunnelDeletePreviewApiData | null>(null);
  const [deleteAction, setDeleteAction] =
    useState<TunnelDeleteAction>(DEFAULT_TUNNEL_DELETE_ACTION);
  const [deleteTargetTunnelId, setDeleteTargetTunnelId] = useState<number | null>(
    null,
  );
  const [currentDiagnosisTunnel, setCurrentDiagnosisTunnel] =
    useState<Tunnel | null>(null);
  const [diagnosisResult, setDiagnosisResult] =
    useState<DiagnosisResult | null>(null);
  const [diagnosisProgress, setDiagnosisProgress] = useState({
    total: 0,
    completed: 0,
    success: 0,
    failed: 0,
    timedOut: false,
  });
  const diagnosisAbortRef = useRef<AbortController | null>(null);

  const getNodeIpOptions = (nodeId: number): string[] => {
    const node = nodes.find((item) => item.id === nodeId);

    if (!node) {
      return [];
    }

    const values: string[] = [];
    const push = (value?: string) => {
      const trimmed = (value || "").trim();

      if (trimmed) {
        values.push(trimmed);
      }
    };

    push(node.serverIpV4);
    push(node.serverIpV6);
    push(node.serverIp);

    (node.extraIPs || "")
      .split(",")
      .map((v) => v.trim())
      .filter((v) => v)
      .forEach((v) => values.push(v));

    return Array.from(new Set(values));
  };

  const getCommonIpOptions = (nodeIds: number[]): string[] => {
    if (nodeIds.length === 0) {
      return [];
    }

    const optionSets = nodeIds.map(
      (nodeId) => new Set(getNodeIpOptions(nodeId)),
    );
    const base = optionSets[0];

    return Array.from(base).filter((ip) =>
      optionSets.every((set) => set.has(ip)),
    );
  };

  // 表单状态
  const [form, setForm] = useState<TunnelForm>(createTunnelFormDefaults());

  // 表单验证错误
  const [errors, setErrors] = useState<{ [key: string]: string }>({});

  // 批量操作相关状态
  const [selectMode, setSelectMode] = useState(false);
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  const [batchDeleteModalOpen, setBatchDeleteModalOpen] = useState(false);
  const [batchDeletePreviewLoading, setBatchDeletePreviewLoading] =
    useState(false);
  const [batchDeletePreview, setBatchDeletePreview] =
    useState<TunnelBatchDeletePreviewApiData | null>(null);
  const [batchDeleteAction, setBatchDeleteAction] =
    useState<TunnelDeleteAction>(DEFAULT_TUNNEL_DELETE_ACTION);
  const [batchDeleteTargetTunnelId, setBatchDeleteTargetTunnelId] = useState<
    number | null
  >(null);
  const [batchLoading, setBatchLoading] = useState(false);
  const [batchProgress, setBatchProgress] = useState<BatchProgressState>({
    active: false,
    label: "",
    percent: 0,
  });
  const [batchResultModal, setBatchResultModal] =
    useState<BatchResultModalState>(EMPTY_BATCH_RESULT_MODAL_STATE);

  useEffect(() => {
    return () => {
      diagnosisAbortRef.current?.abort();
      diagnosisAbortRef.current = null;
    };
  }, []);

  const applyTunnelList = useCallback((items: Tunnel[]) => {
    setTunnels(items);

    const hasDbOrdering = items.some(
      (tunnel) => tunnel.inx !== undefined && tunnel.inx !== 0,
    );

    if (hasDbOrdering) {
      const dbOrder = [...items]
        .sort((a, b) => (a.inx ?? 0) - (b.inx ?? 0))
        .map((tunnel) => tunnel.id);

      setTunnelOrder(dbOrder);

      return;
    }

    setTunnelOrder(
      loadStoredOrder(
        TUNNEL_ORDER_KEY,
        items.map((tunnel) => tunnel.id),
      ),
    );
  }, []);

  const refreshTunnelList = useCallback(
    async (withLoading = true) => {
      if (withLoading) {
        setLoading(true);
      }

      try {
        const tunnelsRes = await getTunnelList();

        if (tunnelsRes.code === 0) {
          applyTunnelList(mapTunnelApiItems(tunnelsRes.data || []));
        } else {
          toast.error(tunnelsRes.msg || "获取隧道列表失败");
        }
      } catch {
        toast.error("获取隧道列表失败");
      } finally {
        if (withLoading) {
          setLoading(false);
        }
      }
    },
    [applyTunnelList],
  );

  const refreshNodes = useCallback(async () => {
    try {
      const nodesRes = await getNodeList();

      if (nodesRes.code === 0) {
        setNodes(nodesRes.data || []);
      }
    } catch {}
  }, []);

  // 加载所有数据
  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      await Promise.all([refreshTunnelList(false), refreshNodes()]);
    } catch {
      toast.error("加载数据失败");
    } finally {
      setLoading(false);
    }
  }, [refreshNodes, refreshTunnelList]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const resetDeleteState = useCallback(() => {
    setDeleteLoading(false);
    setDeletePreviewLoading(false);
    setTunnelToDelete(null);
    setTunnelDeletePreview(null);
    setDeleteAction(DEFAULT_TUNNEL_DELETE_ACTION);
    setDeleteTargetTunnelId(null);
  }, []);

  const handleDeleteModalOpenChange = useCallback(
    (open: boolean) => {
      setDeleteModalOpen(open);
      if (!open) {
        resetDeleteState();
      }
    },
    [resetDeleteState],
  );

  const resetBatchDeleteState = useCallback(() => {
    setBatchDeletePreviewLoading(false);
    setBatchDeletePreview(null);
    setBatchDeleteAction(DEFAULT_TUNNEL_DELETE_ACTION);
    setBatchDeleteTargetTunnelId(null);
  }, []);

  const handleBatchDeleteModalOpenChange = useCallback(
    (open: boolean) => {
      setBatchDeleteModalOpen(open);
      if (!open) {
        resetBatchDeleteState();
      }
    },
    [resetBatchDeleteState],
  );

  // 表单验证
  const validateForm = (): boolean => {
    const newErrors = validateTunnelForm(form, nodes, isEdit);

    setErrors(newErrors);

    return Object.keys(newErrors).length === 0;
  };

  // 新增隧道
  const handleAdd = () => {
    setIsEdit(false);
    setForm(createTunnelFormDefaults());
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
            .map((ip: string) => ip.trim())
            .join("\n")
        : "",
      ipPreference: tunnel.ipPreference || "",
      status: tunnel.status,
    });
    setErrors({});
    setModalOpen(true);
  };

  // 删除隧道
  const handleDelete = async (tunnel: Tunnel) => {
    setTunnelToDelete(tunnel);
    setDeleteModalOpen(true);

    setDeletePreviewLoading(true);
    setTunnelDeletePreview(null);
    setDeleteAction(DEFAULT_TUNNEL_DELETE_ACTION);
    setDeleteTargetTunnelId(null);

    try {
      const response = await previewTunnelDelete(tunnel.id);

      if (response.code !== 0 || !response.data) {
        toast.error(response.msg || "获取删除依赖失败");
        setDeleteModalOpen(false);
        resetDeleteState();

        return;
      }

      setTunnelDeletePreview(response.data);
    } catch (error) {
      toast.error(extractApiErrorMessage(error, "获取删除依赖失败"));
      setDeleteModalOpen(false);
      resetDeleteState();
    } finally {
      setDeletePreviewLoading(false);
    }
  };

  const confirmDelete = async () => {
    if (!tunnelToDelete) return;

    const forwardCount = tunnelDeletePreview?.forwardCount ?? 0;
    const action: TunnelDeleteAction =
      forwardCount > 0 ? deleteAction : "delete_forwards";

    if (
      action === "replace" &&
      forwardCount > 0 &&
      (!deleteTargetTunnelId ||
        !deleteReplacementTunnels.some(
          (tunnel) => tunnel.id === deleteTargetTunnelId,
        ))
    ) {
      toast.error("请选择替换规则的目标隧道");

      return;
    }

    setDeleteLoading(true);
    try {
      const response = await deleteTunnelWithForwards({
        id: tunnelToDelete.id,
        action,
        targetTunnelId:
          action === "replace" ? deleteTargetTunnelId ?? undefined : undefined,
      });

      if (response.code === 0) {
        const deleteResult = (response.data || null) as {
          warnings?: string[];
        } | null;

        if ((deleteResult?.warnings?.length ?? 0) > 0) {
          toast.success(
            `删除成功，另有 ${deleteResult?.warnings?.length ?? 0} 条节点清理提示`,
          );
        } else {
          toast.success("删除成功");
        }
        setDeleteModalOpen(false);
        setTunnels((prev) =>
          prev.filter((tunnel) => tunnel.id !== tunnelToDelete.id),
        );
        setTunnelOrder((prev) => {
          const next = prev.filter((id) => id !== tunnelToDelete.id);

          saveOrder(TUNNEL_ORDER_KEY, next);

          return next;
        });
        setSelectedIds((prev) => {
          const next = new Set(prev);

          next.delete(tunnelToDelete.id);

          return next;
        });
        resetDeleteState();
      } else if (
        response.data &&
        typeof response.data === "object" &&
        Number((response.data as { failCount?: number }).failCount ?? 0) > 0
      ) {
        const result = response.data as {
          failCount?: number;
          successCount?: number;
        };
        const failures = extractBatchFailures(response.data);

        if (failures.length > 0) {
          setBatchResultModal({
            failures,
            open: true,
            summary: `成功 ${Number(result.successCount ?? 0)} 项，失败 ${Number(result.failCount ?? failures.length)} 项`,
            title: "规则处理失败",
          });
        }
        toast.error(response.msg || "删除失败");
      } else {
        toast.error(response.msg || "删除失败");
      }
    } catch (error) {
      toast.error(extractApiErrorMessage(error, "删除失败"));
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

  const toSelectedNodeIds = (keys: Iterable<unknown>): number[] => {
    return Array.from(keys)
      .map((key) => Number.parseInt(String(key), 10))
      .filter((nodeId) => Number.isFinite(nodeId));
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

  // 更新某一跳的所有节点的连接IP
  const updateChainConnectIp = (groupIndex: number, connectIp: string) => {
    setForm((prev) => {
      const chainNodes = [...(prev.chainNodes || [])];

      chainNodes[groupIndex] = (chainNodes[groupIndex] || []).map((node) => ({
        ...node,
        connectIp,
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

  const mergeOrderedNodes = (
    currentNodes: ChainTunnel[],
    selectedNodeIds: number[],
    buildDefault: (nodeId: number) => ChainTunnel,
  ): ChainTunnel[] => {
    const selectedSet = new Set(selectedNodeIds);
    const kept = currentNodes.filter((node) => selectedSet.has(node.nodeId));
    const keptIds = new Set(kept.map((node) => node.nodeId));
    const added = selectedNodeIds
      .filter((nodeId) => !keptIds.has(nodeId))
      .map((nodeId) => buildDefault(nodeId));

    return [...kept, ...added];
  };

  const syncChainGroupNodes = (
    groupIndex: number,
    selectedNodeIds: number[],
  ) => {
    setForm((prev) => {
      const chainNodes = [...(prev.chainNodes || [])];
      const currentGroup = chainNodes[groupIndex] || [];
      const protocol = currentGroup[0]?.protocol || "tls";
      const strategy = currentGroup[0]?.strategy || "round";
      const realNodes = currentGroup.filter((node) => node.nodeId !== -1);
      const mergedNodes = mergeOrderedNodes(
        realNodes,
        selectedNodeIds,
        (nodeId) => ({
          nodeId,
          chainType: 2,
          protocol,
          strategy,
        }),
      );

      chainNodes[groupIndex] =
        mergedNodes.length > 0
          ? mergedNodes
          : [{ nodeId: -1, chainType: 2, protocol, strategy }];

      return { ...prev, chainNodes };
    });
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
        await refreshTunnelList(false);
      } else {
        toast.error(response.msg || (isEdit ? "更新失败" : "创建失败"));
      }
    } catch {
      toast.error("网络错误，请重试");
    } finally {
      setSubmitLoading(false);
    }
  };

  // 诊断隧道
  const handleDiagnose = async (tunnel: Tunnel) => {
    diagnosisAbortRef.current?.abort();
    const abortController = new AbortController();

    diagnosisAbortRef.current = abortController;

    setCurrentDiagnosisTunnel(tunnel);
    setDiagnosisModalOpen(true);
    setDiagnosisLoading(true);
    setDiagnosisProgress({
      total: 0,
      completed: 0,
      success: 0,
      failed: 0,
      timedOut: false,
    });
    setDiagnosisResult({
      tunnelName: tunnel.name,
      tunnelType: tunnel.type === 1 ? "端口转发" : "隧道转发",
      timestamp: Date.now(),
      results: [],
    });

    try {
      let streamErrorMessage = "";
      const streamResult = await diagnoseTunnelStream(
        tunnel.id,
        {
          onStart: (payload) => {
            const startTunnelName =
              typeof payload.tunnelName === "string" &&
              payload.tunnelName.trim() !== ""
                ? payload.tunnelName
                : tunnel.name;
            const startTunnelType =
              typeof payload.tunnelType === "string" &&
              payload.tunnelType.trim() !== ""
                ? payload.tunnelType
                : tunnel.type === 1
                  ? "端口转发"
                  : "隧道转发";
            const startTotal = Number(payload.total);
            const startItems = Array.isArray(payload.items)
              ? (payload.items as DiagnosisResult["results"])
              : [];

            setDiagnosisResult((prev) => ({
              tunnelName: startTunnelName,
              tunnelType: startTunnelType,
              timestamp: Date.now(),
              results: startItems.length > 0 ? startItems : prev?.results || [],
            }));
            if (Number.isFinite(startTotal) && startTotal >= 0) {
              setDiagnosisProgress((prev) => ({
                ...prev,
                total: startTotal,
              }));
            }
          },
          onItem: ({ result, progress }) => {
            setDiagnosisResult((prev) => {
              const base: DiagnosisResult = prev || {
                tunnelName: tunnel.name,
                tunnelType: tunnel.type === 1 ? "端口转发" : "隧道转发",
                timestamp: Date.now(),
                results: [],
              };
              const nextResults = [...base.results];
              const existingIndex = nextResults.findIndex(
                (item) =>
                  item.description === result.description &&
                  item.nodeId === result.nodeId &&
                  item.targetIp === result.targetIp &&
                  item.targetPort === result.targetPort,
              );

              if (existingIndex >= 0) {
                nextResults[existingIndex] = {
                  ...result,
                  diagnosing: false,
                };
              } else {
                nextResults.push({
                  ...result,
                  diagnosing: false,
                });
              }

              return {
                ...base,
                timestamp: Date.now(),
                results: nextResults,
              };
            });
            setDiagnosisProgress({
              total: progress.total,
              completed: progress.completed,
              success: progress.success,
              failed: progress.failed,
              timedOut: Boolean(progress.timedOut),
            });
          },
          onDone: (progress) => {
            setDiagnosisProgress({
              total: progress.total,
              completed: progress.completed,
              success: progress.success,
              failed: progress.failed,
              timedOut: Boolean(progress.timedOut),
            });
          },
          onError: (message) => {
            streamErrorMessage = message;
          },
        },
        abortController.signal,
      );

      if (streamResult.fallback) {
        const response = await diagnoseTunnel(tunnel.id);

        if (response.code === 0) {
          const resultData = response.data as DiagnosisResult;
          const successCount = resultData.results.filter(
            (r) => r.success,
          ).length;
          const failedCount = resultData.results.length - successCount;

          setDiagnosisResult(resultData);
          setDiagnosisProgress({
            total: resultData.results.length,
            completed: resultData.results.length,
            success: successCount,
            failed: failedCount,
            timedOut: false,
          });
        } else {
          toast.error(response.msg || "诊断失败");
          setDiagnosisResult(
            buildDiagnosisFallbackResult({
              tunnelName: tunnel.name,
              tunnelType: tunnel.type,
              description: "诊断失败",
              message: response.msg || "诊断过程中发生错误",
            }),
          );
          setDiagnosisProgress({
            total: 1,
            completed: 1,
            success: 0,
            failed: 1,
            timedOut: false,
          });
        }

        return;
      }

      if (streamErrorMessage) {
        toast.error(streamErrorMessage);
      }
      if (streamResult.timedOut) {
        toast.error("诊断超时（单条30秒 / 整体2分钟），已返回当前结果");
      }
    } catch {
      if (abortController.signal.aborted) {
        return;
      }
      toast.error("网络错误，请重试");
      setDiagnosisResult(
        buildDiagnosisFallbackResult({
          tunnelName: tunnel.name,
          tunnelType: tunnel.type,
          description: "网络错误",
          message: "无法连接到服务器",
        }),
      );
      setDiagnosisProgress({
        total: 1,
        completed: 1,
        success: 0,
        failed: 1,
        timedOut: false,
      });
    } finally {
      if (diagnosisAbortRef.current === abortController) {
        diagnosisAbortRef.current = null;
      }
      setDiagnosisLoading(false);
    }
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

    saveOrder(TUNNEL_ORDER_KEY, newOrder);

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
    } catch {
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

  const openBatchResultModal = useCallback(
    (title: string, summary: string, failures: BatchOperationFailure[]) => {
      setBatchResultModal({
        failures,
        open: true,
        summary,
        title,
      });
    },
    [],
  );

  const handleOpenBatchDeleteModal = async () => {
    if (selectedIds.size === 0) return;

    setBatchDeleteModalOpen(true);
    setBatchDeletePreviewLoading(true);
    setBatchDeletePreview(null);
    setBatchDeleteAction(DEFAULT_TUNNEL_DELETE_ACTION);
    setBatchDeleteTargetTunnelId(null);

    try {
      const response = await previewBatchTunnelDelete(selectedTunnelIdList);

      if (response.code !== 0 || !response.data) {
        toast.error(response.msg || "获取批量删除依赖失败");
        setBatchDeleteModalOpen(false);
        resetBatchDeleteState();

        return;
      }

      setBatchDeletePreview(response.data);
    } catch (error) {
      toast.error(extractApiErrorMessage(error, "获取批量删除依赖失败"));
      setBatchDeleteModalOpen(false);
      resetBatchDeleteState();
    } finally {
      setBatchDeletePreviewLoading(false);
    }
  };

  const handleBatchDelete = async () => {
    if (selectedIds.size === 0) return;
    if (
      batchDeleteHasForwardDependencies &&
      batchDeleteAction === "replace" &&
      (!batchDeleteTargetTunnelId || batchDeleteReplaceUnavailable)
    ) {
      toast.error("请选择替换规则的目标隧道");

      return;
    }

    setBatchLoading(true);
    setBatchProgress({
      active: true,
      label: `正在删除 ${selectedIds.size} 条隧道...`,
      percent: 30,
    });
    try {
      const res = await batchDeleteTunnelsWithForwards({
        ids: selectedTunnelIdList,
        action: batchDeleteHasForwardDependencies
          ? batchDeleteAction
          : "delete_forwards",
        targetTunnelId:
          batchDeleteHasForwardDependencies && batchDeleteAction === "replace"
            ? batchDeleteTargetTunnelId ?? undefined
            : undefined,
      });

      if (res.code === 0) {
        const result = (res.data || {
          successCount: 0,
          failCount: 0,
          warnings: [],
        }) as {
          successCount: number;
          failCount: number;
          warnings?: string[];
        };
        const warningCount = result?.warnings?.length ?? 0;

        if (result.failCount === 0) {
          toast.success(
            warningCount > 0
              ? `成功删除 ${result.successCount} 项，另有 ${warningCount} 条节点清理提示`
              : `成功删除 ${result.successCount} 项`,
          );
          setBatchProgress({
            active: true,
            label: `删除完成：成功 ${result.successCount} 项`,
            percent: 100,
          });
        } else {
          const failures = extractBatchFailures(result);

          if (failures.length > 0) {
            openBatchResultModal(
              "批量删除结果",
              `成功 ${result.successCount} 项，失败 ${result.failCount} 项`,
              failures,
            );
          } else {
            toast.error(
              `成功 ${result.successCount} 项，失败 ${result.failCount} 项`,
            );
          }
          setBatchProgress({
            active: true,
            label: `部分完成：成功 ${result.successCount} 项，正在刷新列表...`,
            percent: 75,
          });
        }
        await refreshTunnelList(false);
        setSelectedIds(new Set());
        setSelectMode(false);
        setBatchDeleteModalOpen(false);
        resetBatchDeleteState();
      } else {
        toast.error(res.msg || "删除失败");
      }
    } catch (error) {
      toast.error(extractApiErrorMessage(error, "删除失败"));
    } finally {
      setBatchProgress({ active: false, label: "", percent: 0 });
      setBatchLoading(false);
    }
  };

  const handleBatchRedeploy = async () => {
    if (selectedIds.size === 0) return;
    setBatchLoading(true);
    setBatchProgress({
      active: true,
      label: `正在重新下发 ${selectedIds.size} 条隧道...`,
      percent: 30,
    });
    try {
      const res = await batchRedeployTunnels(Array.from(selectedIds));

      if (res.code === 0) {
        const result = res.data;

        if (result.failCount === 0) {
          toast.success(`成功重新下发 ${result.successCount} 项`);
        } else {
          const failures = extractBatchFailures(result);

          if (failures.length > 0) {
            openBatchResultModal(
              "批量下发结果",
              `成功 ${result.successCount} 项，失败 ${result.failCount} 项`,
              failures,
            );
          } else {
            toast.error(
              buildBatchFailureMessage(
                result,
                `成功 ${result.successCount} 项，失败 ${result.failCount} 项`,
              ),
            );
          }
        }
        setSelectedIds(new Set());
        setSelectMode(false);
        setBatchProgress({
          active: true,
          label: `重新下发完成：成功 ${result.successCount} 项，正在刷新列表...`,
          percent: 100,
        });
        await refreshTunnelList(false);
      } else {
        toast.error(res.msg || "下发失败");
      }
    } catch (error) {
      toast.error(extractApiErrorMessage(error, "下发失败"));
    } finally {
      setBatchProgress({ active: false, label: "", percent: 0 });
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

    let filteredTunnels = tunnels;

    if (searchKeyword.trim()) {
      const lowerKeyword = searchKeyword.toLowerCase();

      filteredTunnels = filteredTunnels.filter(
        (t) =>
          (t.name && t.name.toLowerCase().includes(lowerKeyword)) ||
          (t.inIp && t.inIp.toLowerCase().includes(lowerKeyword)),
      );
    }

    const sortedByDb = [...filteredTunnels].sort((a, b) => {
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
      const tunnelMap = new Map(filteredTunnels.map((t) => [t.id, t] as const));
      const localSorted: Tunnel[] = [];

      tunnelOrder.forEach((id) => {
        const tunnel = tunnelMap.get(id);

        if (tunnel) localSorted.push(tunnel);
      });

      filteredTunnels.forEach((tunnel) => {
        if (!tunnelOrder.includes(tunnel.id)) {
          localSorted.push(tunnel);
        }
      });

      return localSorted;
    }

    return sortedByDb;
  }, [tunnels, tunnelOrder, searchKeyword]);

  const sortableTunnelIds = useMemo(
    () => sortedTunnels.map((t) => t.id),
    [sortedTunnels],
  );

  const deleteReplacementTunnels = useMemo(() => {
    if (!tunnelToDelete) {
      return [] as Tunnel[];
    }

    return tunnels
      .filter((tunnel) => tunnel.id !== tunnelToDelete.id && tunnel.status === 1)
      .sort((a, b) => {
        const aInx = a.inx ?? 0;
        const bInx = b.inx ?? 0;

        return aInx - bInx;
      });
  }, [tunnelToDelete, tunnels]);

  useEffect(() => {
    if (!deleteModalOpen) {
      return;
    }

    if ((tunnelDeletePreview?.forwardCount ?? 0) <= 0) {
      return;
    }

    if (deleteReplacementTunnels.length === 0) {
      setDeleteAction("delete_forwards");
      setDeleteTargetTunnelId(null);

      return;
    }

    if (deleteAction !== "replace") {
      return;
    }

    setDeleteTargetTunnelId((prev) => {
      if (prev && deleteReplacementTunnels.some((tunnel) => tunnel.id === prev)) {
        return prev;
      }

      return deleteReplacementTunnels[0]?.id ?? null;
    });
  }, [
    deleteAction,
    deleteModalOpen,
    deleteReplacementTunnels,
    tunnelDeletePreview?.forwardCount,
  ]);

  const deletePreviewForwardCount = tunnelDeletePreview?.forwardCount ?? 0;
  const deleteHasForwardDependencies = deletePreviewForwardCount > 0;
  const deleteReplaceUnavailable =
    deleteHasForwardDependencies && deleteReplacementTunnels.length === 0;
  const deleteConfirmLabel = deleteHasForwardDependencies
    ? deleteAction === "replace"
      ? "迁移规则后删除该隧道"
      : "删除规则并删除该隧道"
    : "删除该隧道";

  const selectedTunnelIdList = useMemo(
    () => Array.from(selectedIds),
    [selectedIds],
  );
  const batchDeleteReplacementTunnels = useMemo(() => {
    if (selectedIds.size === 0) {
      return [] as Tunnel[];
    }

    return tunnels
      .filter((tunnel) => !selectedIds.has(tunnel.id) && tunnel.status === 1)
      .sort((a, b) => {
        const aInx = a.inx ?? 0;
        const bInx = b.inx ?? 0;

        return aInx - bInx;
      });
  }, [selectedIds, tunnels]);

  useEffect(() => {
    if (!batchDeleteModalOpen) {
      return;
    }

    if ((batchDeletePreview?.totalForwardCount ?? 0) <= 0) {
      return;
    }

    if (batchDeleteReplacementTunnels.length === 0) {
      setBatchDeleteAction("delete_forwards");
      setBatchDeleteTargetTunnelId(null);

      return;
    }

    if (batchDeleteAction !== "replace") {
      return;
    }

    setBatchDeleteTargetTunnelId((prev) => {
      if (
        prev &&
        batchDeleteReplacementTunnels.some((tunnel) => tunnel.id === prev)
      ) {
        return prev;
      }

      return batchDeleteReplacementTunnels[0]?.id ?? null;
    });
  }, [
    batchDeleteAction,
    batchDeleteModalOpen,
    batchDeletePreview?.totalForwardCount,
    batchDeleteReplacementTunnels,
  ]);

  const batchDeleteTotalForwardCount = batchDeletePreview?.totalForwardCount ?? 0;
  const batchDeleteHasForwardDependencies = batchDeleteTotalForwardCount > 0;
  const batchDeleteDependentTunnelCount =
    batchDeletePreview?.items?.filter((item) => item.forwardCount > 0).length ?? 0;
  const batchDeleteDirectDeleteTunnelCount = Math.max(
    selectedTunnelIdList.length - batchDeleteDependentTunnelCount,
    0,
  );
  const batchDeletePreviewItems = useMemo(() => {
    return [...(batchDeletePreview?.items ?? [])].sort((a, b) => {
      if ((a.forwardCount > 0) === (b.forwardCount > 0)) {
        return a.tunnelName.localeCompare(b.tunnelName, "zh-CN");
      }

      return a.forwardCount > 0 ? -1 : 1;
    });
  }, [batchDeletePreview?.items]);
  const batchDeleteDependentItems = useMemo(
    () => batchDeletePreviewItems.filter((item) => item.forwardCount > 0),
    [batchDeletePreviewItems],
  );
  const batchDeleteReplaceUnavailable =
    batchDeleteHasForwardDependencies && batchDeleteReplacementTunnels.length === 0;
  const batchDeleteConfirmLabel = batchDeleteHasForwardDependencies
    ? batchDeleteAction === "replace"
      ? `迁移规则后删除这 ${selectedTunnelIdList.length} 条隧道`
      : `删除规则并删除 ${selectedTunnelIdList.length} 条隧道`
    : `删除这 ${selectedTunnelIdList.length} 条隧道`;

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

    const style: React.CSSProperties = {
      transform: transform
        ? CSS.Transform.toString({
            ...transform,
            x: Math.round(transform.x),
            y: Math.round(transform.y),
          })
        : undefined,
      transition: isDragging ? undefined : transition || undefined,
      opacity: isDragging ? 0.5 : 1,
      willChange: isDragging ? "transform" : undefined,
    };

    return (
      <div ref={setNodeRef} style={style} {...attributes}>
        {children(listeners)}
      </div>
    );
  };

  if (loading) {
    return <PageLoadingState message="正在加载..." />;
  }

  return (
    <AnimatedPage className="px-3 lg:px-6 py-8">
      <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between mb-6 gap-3">
        <div className="flex-1 max-w-sm flex items-center gap-2">
          <SearchBar
            isVisible={isSearchVisible}
            placeholder="搜索隧道名称或IP"
            value={searchKeyword}
            onChange={setSearchKeyword}
            onClose={() => setIsSearchVisible(false)}
            onOpen={() => setIsSearchVisible(true)}
          />
        </div>

        <div className="min-h-9 min-w-0 max-w-full overflow-x-auto touch-pan-x">
          <div className="flex min-h-9 w-max min-w-full items-center justify-end gap-2 whitespace-nowrap [&>*]:shrink-0">
            {selectMode ? (
              <>
                <span className="text-sm text-default-600 shrink-0">
                  已选择 {selectedIds.size} 项
                </span>
                <Button
                  color="primary"
                  size="sm"
                  variant="flat"
                  onPress={selectAll}
                >
                  全选
                </Button>
                <Button
                  color="secondary"
                  size="sm"
                  variant="flat"
                  onPress={deselectAll}
                >
                  清空
                </Button>
                <Button
                  color="danger"
                  isDisabled={selectedIds.size === 0}
                  size="sm"
                  variant="flat"
                  onPress={handleOpenBatchDeleteModal}
                >
                  删除
                </Button>
                <Button
                  color="primary"
                  isDisabled={selectedIds.size === 0}
                  isLoading={batchLoading}
                  size="sm"
                  variant="flat"
                  onPress={handleBatchRedeploy}
                >
                  下发
                </Button>
                <Button
                  color="secondary"
                  size="sm"
                  variant="solid"
                  onPress={toggleSelectMode}
                >
                  退出
                </Button>
              </>
            ) : (
              <>
                <Button
                  className="bg-sky-100 text-sky-700 hover:bg-sky-200 dark:bg-sky-900/30 dark:text-sky-300 dark:hover:bg-sky-900/45"
                  color="default"
                  size="sm"
                  variant="flat"
                  onPress={toggleSelectMode}
                >
                  批量
                </Button>
                <Button
                  isIconOnly
                  size="sm"
                  variant="flat"
                  onPress={() => setViewMode(viewMode === "list" ? "grid" : "list")}
                >
                  {viewMode === "list" ? <LayoutGrid className="w-4 h-4" /> : <List className="w-4 h-4" />}
                </Button>
                <Button
                  color="primary"
                  size="sm"
                  variant="flat"
                  onPress={handleAdd}
                >
                  新增
                </Button>
              </>
            )}
          </div>
        </div>
      </div>

      {batchProgress.active && (
        <div className="mb-4">
          <Alert
            color="primary"
            description={batchProgress.label}
            variant="flat"
          />
          <Progress
            aria-label={batchProgress.label}
            className="mt-3"
            color="primary"
            size="sm"
            value={batchProgress.percent}
          />
        </div>
      )}

      {/* 隧道卡片网格 */}
      {tunnels.length > 0 ? (
        viewMode === "list" ? (
          <Card className="bg-white/20 dark:bg-zinc-900/20 backdrop-blur-3xl border border-white/80 dark:border-white/10 shadow-[0_15px_35px_rgba(0,0,0,0.1)]">
          <Table
            aria-label="隧道列表"
            className="overflow-x-auto min-w-full"
            classNames={{
              wrapper: "bg-transparent p-0 shadow-none border-none overflow-hidden rounded-[24px]",
              th: "bg-transparent text-default-600 font-semibold text-sm border-b border-white/20 dark:border-white/10 py-3 uppercase tracking-wider first:rounded-tl-[24px] last:rounded-tr-[24px]",
              td: "py-3 border-b border-divider/50 group-data-[last=true]:border-b-0",
              tr: "hover:bg-white/10 dark:hover:bg-white/5 transition-colors",
            }}
          >
            <TableHeader>
              {selectMode ? <TableColumn className="w-12 px-4 whitespace-nowrap overflow-hidden">
                <Checkbox
                  isSelected={selectedIds.size === sortedTunnels.length && sortedTunnels.length > 0}
                  onValueChange={(checked) => checked ? selectAll() : deselectAll()}
                />
              </TableColumn> : <TableColumn className="w-0 p-0 overflow-hidden text-[0px]"></TableColumn>}
              <TableColumn>隧道名称</TableColumn>
              <TableColumn>类型</TableColumn>
              <TableColumn>拓扑</TableColumn>
              <TableColumn>流量统计</TableColumn>
              <TableColumn>操作</TableColumn>
            </TableHeader>
            <TableBody items={sortedTunnels}>
              {(tunnel) => {
                const typeDisplay = getTunnelTypeDisplay(tunnel.type);
                const tunnelTypeChipClassName =
                  tunnel.type === 1
                    ? "text-[10px] h-5 bg-primary-100 text-primary-800 border-primary-300 dark:bg-primary-900/45 dark:text-primary-200 dark:border-primary-700"
                    : "text-[10px] h-5 bg-success-100 text-success-800 border-success-300 dark:bg-success-900/35 dark:text-success-200 dark:border-success-700";
                    
                return (
                  <TableRow key={tunnel.id}>
                    {selectMode ? <TableCell className="px-4">
                      <Checkbox
                        isSelected={selectedIds.has(tunnel.id)}
                        onValueChange={() => toggleSelect(tunnel.id)}
                      />
                    </TableCell> : <TableCell className="w-0 p-0 overflow-hidden text-[0px]"></TableCell>}
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <div
                          className={`shrink-0 w-2 h-2 rounded-full ${
                            tunnel.status === 1 ? "bg-success" : "bg-danger"
                          }`}
                        />
                        <span className="font-medium text-foreground text-sm">
                          {tunnel.name}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell>
                      <Chip
                        className={tunnelTypeChipClassName}
                        color={typeDisplay.color as any}
                        size="sm"
                        variant="flat"
                      >
                        {typeDisplay.text}
                      </Chip>
                    </TableCell>
                    <TableCell>
                       <div className="flex items-center gap-1.5 text-xs">
                          <span className="font-semibold text-primary-700 dark:text-primary-400">
                            {tunnel.inNodeId?.length || 0}入口
                          </span>
                          <span className="text-default-400">→</span>
                          <span className="font-semibold text-secondary-700 dark:text-secondary-400">
                            {tunnel.type === 2 ? tunnel.chainNodes?.length || 0 : 0}跳
                          </span>
                          <span className="text-default-400">→</span>
                          <span className="font-semibold text-success-700 dark:text-success-400">
                            {tunnel.type === 2 ? tunnel.outNodeId?.length || 0 : tunnel.inNodeId?.length || 0}出口
                          </span>
                       </div>
                    </TableCell>
                    <TableCell>
                       <div className="flex items-center gap-2 text-xs">
                         <span className="text-default-500">{getTunnelFlowDisplay(tunnel.flow)}</span>
                         <span className="text-default-300">|</span>
                         <span className="text-default-500">{tunnel.trafficRatio}x</span>
                         {tunnel.type === 2 && tunnel.ipPreference && (
                           <>
                             <span className="text-default-300">|</span>
                             <span className="text-default-500">{tunnel.ipPreference === "v4" ? "IPv4" : "IPv6"}</span>
                           </>
                         )}
                       </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap items-center gap-1.5 min-w-max">
                        <Button
                          className="h-6 px-2 min-w-0 text-xs bg-indigo-50 text-indigo-600 hover:bg-indigo-100 dark:bg-indigo-950/30 dark:text-indigo-400"
                          size="sm"
                          variant="flat"
                          onPress={() => handleEdit(tunnel)}
                        >
                          编辑
                        </Button>
                        <Button
                          className="h-6 px-2 min-w-0 text-xs bg-amber-50 text-amber-600 hover:bg-amber-100 dark:bg-amber-950/30 dark:text-amber-400"
                          size="sm"
                          variant="flat"
                          onPress={() => handleDiagnose(tunnel)}
                        >
                          诊断
                        </Button>
                        <Button
                          className="h-6 px-2 min-w-0 text-xs bg-rose-50 text-rose-600 hover:bg-rose-100 dark:bg-rose-950/30 dark:text-rose-400"
                          size="sm"
                          variant="flat"
                          onPress={() => handleDelete(tunnel)}
                        >
                          删除
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                );
              }}
            </TableBody>
          </Table>
          </Card>
        ) : (
        <DndContext sensors={sensors} onDragEnd={handleDragEnd}>
          <SortableContext
            items={sortableTunnelIds}
            strategy={rectSortingStrategy}
          >
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 gap-4">
              {sortedTunnels.map((tunnel) => {
                const typeDisplay = getTunnelTypeDisplay(tunnel.type);
                const tunnelTypeChipClassName =
                  tunnel.type === 1
                    ? "text-xs bg-primary-100 text-primary-800 border-primary-300 dark:bg-primary-900/45 dark:text-primary-200 dark:border-primary-700"
                    : "text-xs bg-success-100 text-success-800 border-success-300 dark:bg-success-900/35 dark:text-success-200 dark:border-success-700";

                return (
                  <SortableItem key={tunnel.id} id={tunnel.id}>
                    {(listeners) => (
                      <Card
                        key={tunnel.id}
                        className="group overflow-hidden bg-white/20 dark:bg-zinc-900/20 backdrop-blur-3xl border border-white/80 dark:border-white/10 shadow-[0_15px_35px_rgba(0,0,0,0.1)]"
                      >
                        <CardHeader className="pb-2 md:pb-2">
                          <div className="flex justify-between items-start w-full">
                            {selectMode && (
                              <Checkbox
                                className="mr-2"
                                isSelected={selectedIds.has(tunnel.id)}
                                onValueChange={() => toggleSelect(tunnel.id)}
                              />
                            )}
                            <div className="flex-1 min-w-0">
                              <h3 className="font-semibold text-foreground truncate text-sm">
                                {tunnel.name}
                              </h3>
                              <div className="flex items-center gap-1.5 mt-1">
                                <Chip
                                  className={tunnelTypeChipClassName}
                                  color={typeDisplay.color as any}
                                  size="sm"
                                  variant="flat"
                                >
                                  {typeDisplay.text}
                                </Chip>
                              </div>
                            </div>
                            <div
                              className="cursor-grab active:cursor-grabbing p-1 -mr-1 text-default-400 hover:text-default-600 transition-colors touch-manipulation flex-shrink-0"
                              {...listeners}
                              style={{ touchAction: "none" }}
                              title="拖拽排序"
                            >
                              <svg
                                aria-hidden="true"
                                className="w-4 h-4"
                                fill="currentColor"
                                viewBox="0 0 20 20"
                              >
                                <path d="M7 2a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 2zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 8zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 14zm6-8a2 2 0 1 1-.001-4.001A2 2 0 0 1 13 6zm0 2a2 2 0 1 1 .001 4.001A2 2 0 0 1 13 8zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 13 14z" />
                              </svg>
                            </div>
                          </div>
                        </CardHeader>

                        <CardBody className="pt-0 pb-3 md:pt-0 md:pb-3">
                          <div className="space-y-3">
                            {/* 拓扑结构 */}
                            <div className="pt-2 border-t border-divider">
                              <div className="flex items-center justify-center gap-2 text-xs">
                                {/* 入口节点 */}
                                <div className="flex items-center gap-1 px-2 py-1 bg-primary-50/30 dark:bg-primary-100/20 backdrop-blur-md rounded border border-primary-200/50 dark:border-primary-300/20">
                                  <svg
                                    aria-hidden="true"
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
                                  aria-hidden="true"
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
                                <div className="flex items-center gap-1 px-2 py-1 bg-secondary-50/30 dark:bg-secondary-100/20 backdrop-blur-md rounded border border-secondary-200/50 dark:border-secondary-300/20">
                                  <svg
                                    aria-hidden="true"
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
                                  aria-hidden="true"
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
                                <div className="flex items-center gap-1 px-2 py-1 bg-success-50/30 dark:bg-success-100/20 backdrop-blur-md rounded border border-success-200/50 dark:border-success-300/20">
                                  <svg
                                    aria-hidden="true"
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
                            <div
                              className={`grid gap-2 ${tunnel.type === 2 && tunnel.ipPreference ? "grid-cols-3" : "grid-cols-2"}`}
                            >
                              <div className="text-center p-1.5 bg-white/5 dark:bg-black/5 backdrop-blur-3xl rounded-lg border border-divider">
                                <div className="text-xs text-default-500">
                                  流量计算
                                </div>
                                <div className="text-sm font-semibold text-foreground mt-0.5">
                                  {getTunnelFlowDisplay(tunnel.flow)}
                                </div>
                              </div>
                              <div className="text-center p-1.5 bg-white/5 dark:bg-black/5 backdrop-blur-3xl rounded-lg border border-divider">
                                <div className="text-xs text-default-500">
                                  流量倍率
                                </div>
                                <div className="text-sm font-semibold text-foreground mt-0.5">
                                  {tunnel.trafficRatio}x
                                </div>
                              </div>
                              {tunnel.type === 2 && tunnel.ipPreference && (
                                <div className="text-center p-1.5 bg-white/5 dark:bg-black/5 backdrop-blur-3xl rounded-lg border border-divider">
                                  <div className="text-xs text-default-500">
                                    连接偏好
                                  </div>
                                  <div className="text-sm font-semibold text-foreground mt-0.5">
                                    {tunnel.ipPreference === "v4"
                                      ? "IPv4"
                                      : "IPv6"}
                                  </div>
                                </div>
                              )}
                            </div>
                          </div>

                          <div className="flex gap-1.5 mt-3">
                            <Button
                              className="flex-1 min-h-8"
                              color="primary"
                              size="sm"
                              startContent={
                                <svg
                                  aria-hidden="true"
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
                                  aria-hidden="true"
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
                                  aria-hidden="true"
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
        )
      ) : (
        /* 空状态 */
        <Card className="bg-white/20 dark:bg-zinc-900/20 backdrop-blur-3xl border border-white/80 dark:border-white/10 shadow-[0_15px_35px_rgba(0,0,0,0.1)]">
          <CardBody className="text-center py-20 flex flex-col items-center justify-center min-h-[240px]">
            <h3 className="text-xl font-medium text-foreground tracking-tight mb-2">
              暂无隧道配置
            </h3>
            <p className="text-default-500 text-sm max-w-xs mx-auto leading-relaxed">
              还没有创建任何隧道配置，点击上方按钮开始创建
            </p>
          </CardBody>
        </Card>
      )}

      {/* 新增/编辑模态框 */}
      <Modal
        backdrop="blur"
        classNames={{
          base: "!w-[calc(100%-32px)] !mx-auto sm:!w-full rounded-2xl overflow-hidden",
        }}
        isOpen={modalOpen}
        placement="center"
        scrollBehavior="inside"
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
                      errorMessage={errors.trafficRatio}
                      isInvalid={!!errors.trafficRatio}
                      label="流量倍率"
                      max={100}
                      min={0.01}
                      placeholder="例如：0.5 或 1 或 2"
                      step="any"
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
                    description="入口IP由系统自动从入口节点采集，无需手动填写。支持多个IP，每行一个地址，留空则使用入口节点IP"
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

                  {form.type === 2 && (
                    <Select
                      description="当节点同时拥有IPv4和IPv6地址时，选择隧道连接使用的地址类型"
                      label="隧道连接地址偏好"
                      placeholder="自动选择"
                      selectedKeys={[form.ipPreference || ""]}
                      variant="bordered"
                      onSelectionChange={(keys) => {
                        const selectedKey = Array.from(keys)[0] as string;

                        setForm((prev) => ({
                          ...prev,
                          ipPreference: selectedKey || "",
                        }));
                      }}
                    >
                      <SelectItem key="v4">优先IPv4</SelectItem>
                      <SelectItem key="v6">优先IPv6</SelectItem>
                    </Select>
                  )}

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
                        const selectedIds = toSelectedNodeIds(keys);

                        setForm((prev) => ({
                          ...prev,
                          inNodeId: mergeOrderedNodes(
                            prev.inNodeId,
                            selectedIds,
                            (nodeId) => ({ nodeId, chainType: 1 }),
                          ),
                        }));
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
                              aria-hidden="true"
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
                            const groupSelectedNodeIds = groupNodes
                              .filter((ct) => ct.nodeId !== -1)
                              .map((ct) => ct.nodeId);
                            const groupIpOptions =
                              getCommonIpOptions(groupSelectedNodeIds);
                            const isMultiNodeGroup =
                              groupSelectedNodeIds.length > 1;
                            const selectedGroupConnectIp =
                              groupNodes.length > 0
                                ? groupNodes[0].connectIp || ""
                                : "";

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
                                    aria-label={`删除第${groupIndex + 1}跳`}
                                    color="danger"
                                    size="sm"
                                    variant="light"
                                    onPress={() => removeChainNode(groupIndex)}
                                  >
                                    <svg
                                      aria-hidden="true"
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
                                      dropdownPlacement="top"
                                      label="节点"
                                      placeholder="选择节点（可多选）"
                                      selectedKeys={groupNodes
                                        .filter((ct) => ct.nodeId !== -1)
                                        .map((ct) => ct.nodeId.toString())}
                                      selectionMode="multiple"
                                      size="sm"
                                      variant="bordered"
                                      onSelectionChange={(keys) => {
                                        syncChainGroupNodes(
                                          groupIndex,
                                          toSelectedNodeIds(keys),
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

                                {/* 连接IP - 转发链节点 */}
                                <Select
                                  classNames={{
                                    label: "text-xs",
                                    value: "text-sm",
                                  }}
                                  description={
                                    isMultiNodeGroup
                                      ? "多节点跳不支持设置自定义连接IP，使用各节点默认IP"
                                      : "按当前跳所选节点的共有IP进行选择，留空使用默认"
                                  }
                                  isDisabled={
                                    groupSelectedNodeIds.length === 0 ||
                                    groupIpOptions.length === 0 ||
                                    isMultiNodeGroup
                                  }
                                  label="连接IP"
                                  placeholder={
                                    isMultiNodeGroup
                                      ? "多节点跳使用节点默认IP"
                                      : groupSelectedNodeIds.length === 0
                                        ? "请先选择节点"
                                        : groupIpOptions.length > 0
                                          ? "选择连接IP"
                                          : "所选节点无共同可选IP"
                                  }
                                  selectedKeys={[
                                    selectedGroupConnectIp || "__default__",
                                  ]}
                                  size="sm"
                                  variant="bordered"
                                  onSelectionChange={(keys) => {
                                    const selectedKey = Array.from(
                                      keys,
                                    )[0] as string;

                                    updateChainConnectIp(
                                      groupIndex,
                                      selectedKey === "__default__"
                                        ? ""
                                        : selectedKey,
                                    );
                                  }}
                                >
                                  <SelectItem key="__default__">
                                    默认连接IP
                                  </SelectItem>
                                  {groupIpOptions.map((ip) => (
                                    <SelectItem key={ip}>{ip}</SelectItem>
                                  ))}
                                </Select>
                              </div>
                            );
                          })}
                        </div>
                      )}

                      {getChainGroups().length === 0 && (
                        <div className="text-center py-8 bg-default-50 dark:bg-default-100/50 rounded border border-dashed border-default-300">
                          <p className="text-sm text-default-500">
                            还没有添加转发链，点击上方&quot;添加一跳&quot;按钮开始添加
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

                      {(() => {
                        const selectedOutNodeIds = (form.outNodeId || [])
                          .filter((ct) => ct.nodeId !== -1)
                          .map((ct) => ct.nodeId);
                        const isMultiExit = selectedOutNodeIds.length > 1;
                        const commonOutIpOptions =
                          getCommonIpOptions(selectedOutNodeIds);

                        return (
                          <>
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
                                  dropdownPlacement="top"
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
                                    const selectedIds = toSelectedNodeIds(keys);

                                    setForm((prev) => {
                                      const currentOutNodes =
                                        prev.outNodeId || [];
                                      const protocol =
                                        currentOutNodes[0]?.protocol || "tls";
                                      const strategy =
                                        currentOutNodes[0]?.strategy || "round";
                                      const realNodes = currentOutNodes.filter(
                                        (ct) => ct.nodeId !== -1,
                                      );

                                      return {
                                        ...prev,
                                        outNodeId: mergeOrderedNodes(
                                          realNodes,
                                          selectedIds,
                                          (nodeId) => ({
                                            nodeId,
                                            chainType: 3,
                                            protocol,
                                            strategy,
                                          }),
                                        ),
                                      };
                                    });
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
                                  const selectedKey = Array.from(
                                    keys,
                                  )[0] as string;

                                  if (selectedKey) {
                                    setForm((prev) => {
                                      const currentOutNodes =
                                        prev.outNodeId || [];
                                      const currentStrategy =
                                        currentOutNodes.length > 0
                                          ? currentOutNodes[0].strategy ||
                                            "round"
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
                                        outNodeId: currentOutNodes.map(
                                          (ct) => ({
                                            ...ct,
                                            protocol: selectedKey,
                                          }),
                                        ),
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

                                    return (
                                      form.outNodeId[0].strategy || "round"
                                    );
                                  })(),
                                ]}
                                variant="bordered"
                                onSelectionChange={(keys) => {
                                  const selectedKey = Array.from(
                                    keys,
                                  )[0] as string;

                                  if (selectedKey) {
                                    setForm((prev) => {
                                      const currentOutNodes =
                                        prev.outNodeId || [];
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
                                        outNodeId: currentOutNodes.map(
                                          (ct) => ({
                                            ...ct,
                                            strategy: selectedKey,
                                          }),
                                        ),
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

                            {/* 连接IP - 出口节点 */}
                            <Select
                              classNames={{
                                label: "text-xs",
                                value: "text-sm",
                              }}
                              description={
                                isMultiExit
                                  ? "多出口隧道不支持设置自定义连接IP，使用各节点默认IP"
                                  : "按出口节点共同可用IP选择，留空使用默认"
                              }
                              isDisabled={
                                selectedOutNodeIds.length === 0 ||
                                commonOutIpOptions.length === 0 ||
                                isMultiExit
                              }
                              label="连接IP"
                              placeholder={
                                isMultiExit
                                  ? "多出口隧道使用节点默认IP"
                                  : selectedOutNodeIds.length === 0
                                    ? "请先选择出口节点"
                                    : commonOutIpOptions.length > 0
                                      ? "选择连接IP"
                                      : "所选节点无共同可选IP"
                              }
                              selectedKeys={[
                                form.outNodeId && form.outNodeId.length > 0
                                  ? form.outNodeId[0].connectIp || "__default__"
                                  : "__default__",
                              ]}
                              size="sm"
                              variant="bordered"
                              onSelectionChange={(keys) => {
                                const selectedKey = Array.from(
                                  keys,
                                )[0] as string;
                                const value =
                                  selectedKey === "__default__"
                                    ? ""
                                    : selectedKey;

                                setForm((prev) => {
                                  const currentOutNodes = prev.outNodeId || [];

                                  if (currentOutNodes.length === 0) {
                                    return {
                                      ...prev,
                                      outNodeId: [
                                        {
                                          nodeId: -1,
                                          chainType: 3,
                                          protocol: "tls",
                                          strategy: "round",
                                          connectIp: value,
                                        },
                                      ],
                                    };
                                  }

                                  return {
                                    ...prev,
                                    outNodeId: currentOutNodes.map((ct) => ({
                                      ...ct,
                                      connectIp: value,
                                    })),
                                  };
                                });
                              }}
                            >
                              <SelectItem key="__default__">
                                默认连接IP
                              </SelectItem>
                              {commonOutIpOptions.map((ip) => (
                                <SelectItem key={ip}>{ip}</SelectItem>
                              ))}
                            </Select>
                          </>
                        );
                      })()}
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
        classNames={{
          base: "!w-[calc(100%-32px)] !mx-auto sm:!w-full rounded-2xl overflow-hidden",
        }}
        isOpen={deleteModalOpen}
        placement="center"
        scrollBehavior="inside"
        size="2xl"
        onOpenChange={handleDeleteModalOpenChange}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                <h2 className="text-lg font-bold sm:text-xl">删除隧道</h2>
                <p className="text-xs font-normal leading-5 text-default-500 sm:text-sm">
                  {tunnelDeletePreview?.tunnelName || tunnelToDelete?.name
                    ? `即将删除“${tunnelDeletePreview?.tunnelName || tunnelToDelete?.name}”，删除前会先检查是否有关联规则。`
                    : "删除前会先检查是否有关联规则。"}
                </p>
              </ModalHeader>
              <ModalBody className="space-y-3 sm:space-y-4">
                {deletePreviewLoading ? (
                  <div className="flex items-center gap-3 rounded-xl border border-divider bg-content2/40 px-3 py-5 text-sm text-default-600 sm:px-4 sm:py-6">
                    <Spinner size="sm" />
                    正在检查是否有规则正在使用该隧道...
                  </div>
                ) : deleteHasForwardDependencies ? (
                  <>
                    <Alert
                      color="warning"
                      description={`隧道 \"${tunnelDeletePreview?.tunnelName || tunnelToDelete?.name || ""}\" 当前被 ${deletePreviewForwardCount} 条规则使用。删除前需要先处理这些规则。`}
                      title="发现关联规则"
                      variant="flat"
                    />

                    {(tunnelDeletePreview?.sampleForwards?.length ?? 0) > 0 ? (
                      <div className="space-y-3 rounded-xl border border-divider bg-content2/40 p-3 sm:p-4">
                        <div className="flex items-center justify-between gap-3">
                          <h3 className="text-sm font-semibold text-foreground">
                            关联规则预览
                          </h3>
                          <span className="text-xs text-default-500">
                            前 {tunnelDeletePreview?.sampleForwards?.length ?? 0} 条
                          </span>
                        </div>
                        <div className="space-y-2">
                          {tunnelDeletePreview?.sampleForwards?.map((forward) => (
                            <div
                              key={forward.id}
                              className="rounded-lg border border-divider/70 bg-background/80 px-2.5 py-2 sm:px-3"
                            >
                              <div className="flex items-center justify-between gap-3">
                                <span className="truncate text-sm font-medium text-foreground">
                                  {forward.name}
                                </span>
                                <span className="shrink-0 font-mono text-xs text-default-500">
                                  :{forward.inPort || 0}
                                </span>
                              </div>
                              <p className="mt-1 text-xs text-default-500">
                                用户：{forward.userName || `#${forward.userId}`}
                              </p>
                            </div>
                          ))}
                        </div>
                        {deletePreviewForwardCount >
                        (tunnelDeletePreview?.sampleForwards?.length ?? 0) ? (
                          <p className="text-xs text-default-500">
                            还有 {deletePreviewForwardCount - (tunnelDeletePreview?.sampleForwards?.length ?? 0)} 条规则未展开显示。
                          </p>
                        ) : null}
                      </div>
                    ) : null}

                    <RadioGroup
                      label="处理方式"
                      value={deleteAction}
                      onValueChange={(value) => {
                        const nextAction = value as TunnelDeleteAction;

                        setDeleteAction(nextAction);
                        if (nextAction !== "replace") {
                          setDeleteTargetTunnelId(null);
                          return;
                        }

                        setDeleteTargetTunnelId(
                          deleteReplacementTunnels[0]?.id ?? null,
                        );
                      }}
                    >
                      <Radio value="replace">
                        保留规则，迁移到其他隧道{deleteReplaceUnavailable ? "（当前无可用目标）" : "（推荐）"}
                      </Radio>
                      <Radio value="delete_forwards">直接删除这些关联规则</Radio>
                    </RadioGroup>

                    {deleteReplaceUnavailable ? (
                      <Alert
                        color="warning"
                        description="当前没有其他启用中的隧道可用于承接这些规则，只能删除关联规则后再删除该隧道。"
                        variant="flat"
                      />
                    ) : null}

                    {deleteAction === "replace" && !deleteReplaceUnavailable ? (
                      <div className="space-y-2">
                        <Select
                          label="目标隧道"
                          placeholder="请选择目标隧道"
                          selectedKeys={
                            deleteTargetTunnelId
                              ? [String(deleteTargetTunnelId)]
                              : []
                          }
                          variant="bordered"
                          onSelectionChange={(keys) => {
                            const selected = Array.from(keys)[0];

                            setDeleteTargetTunnelId(
                              selected ? Number(selected) : null,
                            );
                          }}
                        >
                          {deleteReplacementTunnels.map((tunnel) => (
                            <SelectItem key={String(tunnel.id)}>
                              {tunnel.name}
                            </SelectItem>
                          ))}
                        </Select>
                        <p className="text-xs text-default-500">
                          关联规则会迁移到这里，当前要删除的隧道不会出现在可选项里。
                        </p>
                      </div>
                    ) : null}
                  </>
                ) : (
                  <Alert
                    color="warning"
                    description={`当前未发现关联规则。确认后将直接删除“${tunnelToDelete?.name || "该隧道"}”，此操作不可撤销。`}
                    title="可以直接删除"
                    variant="flat"
                  />
                )}
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  取消
                </Button>
                <Button
                  color="danger"
                  isDisabled={
                    deletePreviewLoading ||
                    (deleteHasForwardDependencies &&
                      deleteAction === "replace" &&
                      (!deleteTargetTunnelId || deleteReplaceUnavailable))
                  }
                  isLoading={deleteLoading}
                  onPress={confirmDelete}
                >
                  {deleteLoading ? "删除中..." : deleteConfirmLabel}
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
          base: "!w-[calc(100%-32px)] !mx-auto sm:!w-full rounded-2xl overflow-hidden",
        }}
        isOpen={diagnosisModalOpen}
        placement="center"
        scrollBehavior="inside"
        size="4xl"
        onOpenChange={(open) => {
          setDiagnosisModalOpen(open);
          if (!open) {
            diagnosisAbortRef.current?.abort();
            diagnosisAbortRef.current = null;
            setDiagnosisLoading(false);
          }
        }}
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
              <ModalBody className="bg-transparent">
                {diagnosisResult ? (
                  <div className="space-y-4">
                    {diagnosisLoading && (
                      <div className="flex items-center justify-between rounded-lg border border-primary/20 bg-primary/5 px-3 py-2">
                        <div className="flex items-center gap-2 text-sm text-primary">
                          <Spinner size="sm" />
                          <span>
                            正在诊断 {diagnosisProgress.completed}/
                            {diagnosisProgress.total > 0
                              ? diagnosisProgress.total
                              : "?"}
                          </span>
                        </div>
                        <Chip color="primary" size="sm" variant="flat">
                          流式更新中
                        </Chip>
                      </div>
                    )}

                    {diagnosisProgress.timedOut && (
                      <Alert
                        color="warning"
                        description="诊断超时（单条30秒 / 整体2分钟），以下为当前已完成结果。"
                        title="诊断超时"
                        variant="flat"
                      />
                    )}

                    {/* 统计摘要 */}
                    <div className="grid grid-cols-3 gap-3">
                      <div className="text-center p-3 bg-default-100 dark:bg-gray-800 rounded-lg border border-divider">
                        <div className="text-2xl font-bold text-foreground">
                          {diagnosisProgress.total > 0
                            ? diagnosisProgress.total
                            : diagnosisResult.results.length}
                        </div>
                        <div className="text-xs text-default-500 mt-1">
                          总测试数
                        </div>
                      </div>
                      <div className="text-center p-3 bg-success-50 dark:bg-success-900/20 rounded-lg border border-success-200 dark:border-success-700">
                        <div className="text-2xl font-bold text-success-600 dark:text-success-400">
                          {diagnosisProgress.completed > 0 ||
                          diagnosisProgress.total > 0
                            ? diagnosisProgress.success
                            : diagnosisResult.results.filter((r) => r.success)
                                .length}
                        </div>
                        <div className="text-xs text-success-600 dark:text-success-400/80 mt-1">
                          成功
                        </div>
                      </div>
                      <div className="text-center p-3 bg-danger-50 dark:bg-danger-900/20 rounded-lg border border-danger-200 dark:border-danger-700">
                        <div className="text-2xl font-bold text-danger-600 dark:text-danger-400">
                          {diagnosisProgress.completed > 0 ||
                          diagnosisProgress.total > 0
                            ? diagnosisProgress.failed
                            : diagnosisResult.results.filter((r) => !r.success)
                                .length}
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
                                    const isDiagnosing = Boolean(
                                      result.diagnosing,
                                    );
                                    const isSuccess = result.success === true;
                                    const quality = getDiagnosisQualityDisplay(
                                      result.averageTime,
                                      result.packetLoss,
                                    );

                                    return (
                                      <tr
                                        key={index}
                                        className={`hover:bg-default-50 dark:hover:bg-gray-700/50 ${
                                          isDiagnosing
                                            ? "bg-warning-50 dark:bg-warning-900/20"
                                            : isSuccess
                                              ? "bg-white dark:bg-gray-800"
                                              : "bg-danger-50 dark:bg-danger-900/30"
                                        }`}
                                      >
                                        <td className="px-3 py-2">
                                          <div className="flex items-center gap-2">
                                            {isDiagnosing ? (
                                              <Spinner size="sm" />
                                            ) : (
                                              <span
                                                className={`w-5 h-5 rounded-full flex items-center justify-center text-xs ${
                                                  isSuccess
                                                    ? "bg-success text-white"
                                                    : "bg-danger text-white"
                                                }`}
                                              >
                                                {isSuccess ? "✓" : "✗"}
                                              </span>
                                            )}
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
                                            color={
                                              isDiagnosing
                                                ? "warning"
                                                : isSuccess
                                                  ? "success"
                                                  : "danger"
                                            }
                                            size="sm"
                                            variant="flat"
                                          >
                                            {isDiagnosing
                                              ? "诊断中"
                                              : isSuccess
                                                ? "成功"
                                                : "失败"}
                                          </Chip>
                                        </td>
                                        <td className="px-3 py-2 text-center">
                                          {isSuccess ? (
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
                                          {isSuccess ? (
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
                                          {isSuccess && quality ? (
                                            <Chip
                                              className="text-xs whitespace-nowrap"
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
                                const isDiagnosing = Boolean(result.diagnosing);
                                const isSuccess = result.success === true;
                                const quality = getDiagnosisQualityDisplay(
                                  result.averageTime,
                                  result.packetLoss,
                                );

                                return (
                                  <div
                                    key={index}
                                    className={`border rounded-lg p-3 ${
                                      isDiagnosing
                                        ? "border-warning-200 dark:border-warning-300/30 bg-warning-50 dark:bg-warning-900/20"
                                        : isSuccess
                                          ? "border-divider bg-white dark:bg-gray-800"
                                          : "border-danger-200 dark:border-danger-300/30 bg-danger-50 dark:bg-danger-900/30"
                                    }`}
                                  >
                                    <div className="flex items-start gap-2 mb-2">
                                      {isDiagnosing ? (
                                        <Spinner size="sm" />
                                      ) : (
                                        <span
                                          className={`w-6 h-6 rounded-full flex items-center justify-center text-xs flex-shrink-0 ${
                                            isSuccess
                                              ? "bg-success text-white"
                                              : "bg-danger text-white"
                                          }`}
                                        >
                                          {isSuccess ? "✓" : "✗"}
                                        </span>
                                      )}
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
                                          isDiagnosing
                                            ? "warning"
                                            : isSuccess
                                              ? "success"
                                              : "danger"
                                        }
                                        size="sm"
                                        variant="flat"
                                      >
                                        {isDiagnosing
                                          ? "诊断中"
                                          : isSuccess
                                            ? "成功"
                                            : "失败"}
                                      </Chip>
                                    </div>

                                    {isSuccess ? (
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
                                                className="text-xs whitespace-nowrap"
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
                                        <div
                                          className={`text-xs ${
                                            isDiagnosing
                                              ? "text-warning"
                                              : "text-danger"
                                          }`}
                                        >
                                          {isDiagnosing
                                            ? result.message || "诊断中..."
                                            : result.message || "连接失败"}
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
                    {diagnosisResult.results.some(
                      (r) => r.success === false && !r.diagnosing,
                    ) && (
                      <div className="space-y-2 hidden md:block">
                        <h4 className="text-sm font-semibold text-danger">
                          失败详情
                        </h4>
                        <div className="space-y-2">
                          {diagnosisResult.results
                            .filter((r) => r.success === false && !r.diagnosing)
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
                        aria-hidden="true"
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
              <ModalFooter className="bg-transparent border-t border-white/20 dark:border-white/10">
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

      <Modal
        backdrop="blur"
        classNames={{
          base: "!w-[calc(100%-32px)] !mx-auto sm:!w-full rounded-2xl overflow-hidden",
        }}
        isOpen={batchDeleteModalOpen}
        onOpenChange={handleBatchDeleteModalOpenChange}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                <h2 className="text-lg font-bold sm:text-xl">批量删除隧道</h2>
                <p className="text-xs font-normal leading-5 text-default-500 sm:text-sm">
                  即将删除这 {selectedTunnelIdList.length} 条隧道，删除前会先检查是否有关联规则。
                </p>
              </ModalHeader>
              <ModalBody className="space-y-3 sm:space-y-4">
                {batchDeletePreviewLoading ? (
                  <div className="flex items-center gap-3 rounded-xl border border-divider bg-content2/40 px-3 py-5 text-sm text-default-600 sm:px-4 sm:py-6">
                    <Spinner size="sm" />
                    正在检查选中隧道是否有关联规则...
                  </div>
                ) : batchDeleteHasForwardDependencies ? (
                  <>
                    <Alert
                      color="warning"
                      description={`已选 ${selectedTunnelIdList.length} 条隧道，其中 ${batchDeleteDependentTunnelCount} 条仍被规则使用，共 ${batchDeleteTotalForwardCount} 条规则待处理。${batchDeleteDirectDeleteTunnelCount > 0 ? `其余 ${batchDeleteDirectDeleteTunnelCount} 条会直接删除。` : ""}`}
                      title="发现关联规则"
                      variant="flat"
                    />

                    <div className="max-h-64 space-y-3 overflow-y-auto rounded-xl border border-divider bg-content2/40 p-3 sm:max-h-72 sm:p-4">
                      {batchDeleteDependentItems.map((item) => (
                        <div
                          key={item.tunnelId}
                          className="rounded-lg border border-divider/70 bg-background/80 p-2.5 sm:p-3"
                        >
                          <div className="flex items-center justify-between gap-3">
                            <div className="min-w-0">
                              <p className="truncate text-sm font-medium text-foreground">
                                {item.tunnelName}
                              </p>
                              <p className="mt-1 text-xs text-default-500">
                                {item.forwardCount} 条规则依赖
                              </p>
                            </div>
                            <Chip color="warning" size="sm" variant="flat">
                              有关联
                            </Chip>
                          </div>

                          {item.sampleForwards.length > 0 ? (
                            <div className="mt-3 space-y-2">
                              {item.sampleForwards.map((forward) => (
                                <div
                                  key={forward.id}
                                  className="rounded-md bg-content1/70 px-2.5 py-2 sm:px-3"
                                >
                                  <div className="flex items-center justify-between gap-3">
                                    <span className="truncate text-xs font-medium text-foreground">
                                      {forward.name}
                                    </span>
                                    <span className="shrink-0 font-mono text-[11px] text-default-500">
                                      :{forward.inPort || 0}
                                    </span>
                                  </div>
                                  <p className="mt-1 text-[11px] text-default-500">
                                    用户：{forward.userName || `#${forward.userId}`}
                                  </p>
                                </div>
                              ))}
                              {item.forwardCount > item.sampleForwards.length ? (
                                <p className="text-[11px] text-default-500">
                                  还有 {item.forwardCount - item.sampleForwards.length} 条规则未展开显示。
                                </p>
                              ) : null}
                            </div>
                          ) : null}
                        </div>
                      ))}
                    </div>

                    <RadioGroup
                      label="处理方式"
                      value={batchDeleteAction}
                      onValueChange={(value) => {
                        const nextAction = value as TunnelDeleteAction;

                        setBatchDeleteAction(nextAction);
                        if (nextAction !== "replace") {
                          setBatchDeleteTargetTunnelId(null);
                          return;
                        }

                        setBatchDeleteTargetTunnelId(
                          batchDeleteReplacementTunnels[0]?.id ?? null,
                        );
                      }}
                    >
                      <Radio value="replace">
                        保留规则，统一迁移到其他隧道{batchDeleteReplaceUnavailable ? "（当前无可用目标）" : "（推荐）"}
                      </Radio>
                      <Radio value="delete_forwards">直接删除这些关联规则</Radio>
                    </RadioGroup>

                    {batchDeleteReplaceUnavailable ? (
                      <Alert
                        color="warning"
                        description="当前没有可承接这些规则的启用隧道，只能删除关联规则后再删除所选隧道。"
                        variant="flat"
                      />
                    ) : null}

                    {batchDeleteAction === "replace" &&
                    !batchDeleteReplaceUnavailable ? (
                      <div className="space-y-2">
                        <Select
                          label="目标隧道"
                          placeholder="请选择目标隧道"
                          selectedKeys={
                            batchDeleteTargetTunnelId
                              ? [String(batchDeleteTargetTunnelId)]
                              : []
                          }
                          variant="bordered"
                          onSelectionChange={(keys) => {
                            const selected = Array.from(keys)[0];

                            setBatchDeleteTargetTunnelId(
                              selected ? Number(selected) : null,
                            );
                          }}
                        >
                          {batchDeleteReplacementTunnels.map((tunnel) => (
                            <SelectItem key={String(tunnel.id)}>
                              {tunnel.name}
                            </SelectItem>
                          ))}
                        </Select>
                        <p className="text-xs text-default-500">
                          所有关联规则都会迁移到这里，删除列表中的隧道不会出现在可选项里。
                        </p>
                      </div>
                    ) : null}
                  </>
                ) : (
                  <Alert
                    color="warning"
                    description={`已选 ${selectedTunnelIdList.length} 条隧道，当前未发现关联规则。确认后将直接删除这些隧道，此操作不可撤销。`}
                    title="可以直接删除"
                    variant="flat"
                  />
                )}
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  取消
                </Button>
                <Button
                  color="danger"
                  isDisabled={
                    batchDeletePreviewLoading ||
                    (batchDeleteHasForwardDependencies &&
                      batchDeleteAction === "replace" &&
                      (!batchDeleteTargetTunnelId ||
                        batchDeleteReplaceUnavailable))
                  }
                  isLoading={batchLoading}
                  onPress={handleBatchDelete}
                >
                  {batchLoading ? "删除中..." : batchDeleteConfirmLabel}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      <BatchActionResultModal
        failures={batchResultModal.failures}
        isOpen={batchResultModal.open}
        summary={batchResultModal.summary}
        title={batchResultModal.title}
        onOpenChange={(open) => {
          if (open) {
            setBatchResultModal((prev) => ({ ...prev, open: true }));

            return;
          }
          setBatchResultModal(EMPTY_BATCH_RESULT_MODAL_STATE);
        }}
      />
    </AnimatedPage>
  );
}
