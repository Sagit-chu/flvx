import { useState, useEffect, useMemo } from "react";
import toast from "react-hot-toast";
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  MouseSensor,
  TouchSensor,
  useSensor,
  useSensors,
  DragEndEvent,
} from "@dnd-kit/core";
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  rectSortingStrategy,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";

import { Card, CardBody, CardHeader } from "@/shadcn-bridge/heroui/card";
import { Button } from "@/shadcn-bridge/heroui/button";
import { Input } from "@/shadcn-bridge/heroui/input";
import { Textarea } from "@/shadcn-bridge/heroui/input";
import { Select, SelectItem } from "@/shadcn-bridge/heroui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableColumn,
  TableHeader,
  TableRow,
} from "@/shadcn-bridge/heroui/table";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
} from "@/shadcn-bridge/heroui/modal";
import { Chip } from "@/shadcn-bridge/heroui/chip";
import { Spinner } from "@/shadcn-bridge/heroui/spinner";
import { Switch } from "@/shadcn-bridge/heroui/switch";
import { Alert } from "@/shadcn-bridge/heroui/alert";
import { Checkbox } from "@/shadcn-bridge/heroui/checkbox";
import {
  createForward,
  getForwardList,
  updateForward,
  deleteForward,
  forceDeleteForward,
  userTunnel,
  pauseForwardService,
  resumeForwardService,
  diagnoseForward,
  updateForwardOrder,
} from "@/api";
import {
  type ForwardAddressItem,
  formatInAddress,
  formatRemoteAddress,
  hasMultipleAddresses,
  resolveForwardAddressAction,
} from "@/pages/forward/address";
import {
  buildForwardDiagnosisFallbackResult,
  getForwardDiagnosisQualityDisplay,
  type ForwardDiagnosisResult,
} from "@/pages/forward/diagnosis";
import {
  executeForwardBatchChangeTunnel,
  executeForwardBatchDelete,
  executeForwardBatchRedeploy,
  executeForwardBatchToggleService,
} from "@/pages/forward/batch-actions";
import { buildForwardOrder, FORWARD_ORDER_KEY } from "@/pages/forward/order";
import { PageLoadingState } from "@/components/page-state";
import { useMobileBreakpoint } from "@/hooks/useMobileBreakpoint";
import { saveOrder } from "@/utils/order-storage";
import { JwtUtil } from "@/utils/jwt";

interface Forward {
  id: number;
  name: string;
  tunnelId: number;
  tunnelName: string;
  inIp: string;
  inPort: number;
  remoteAddr: string;
  interfaceName?: string;
  strategy: string;
  status: number;
  inFlow: number;
  outFlow: number;
  serviceRunning: boolean;
  createdTime: string;
  userName?: string;
  userId?: number;
  inx?: number;
}

interface Tunnel {
  id: number;
  name: string;
  inNodePortSta?: number;
  inNodePortEnd?: number;
}

interface ForwardForm {
  id?: number;
  userId?: number;
  name: string;
  tunnelId: number | null;
  inPort: number | null;
  remoteAddr: string;
  interfaceName?: string;
  strategy: string;
}

// 添加分组接口
interface UserGroup {
  userId: number | null;
  userName: string;
  tunnelGroups: TunnelGroup[];
}

interface TunnelGroup {
  tunnelId: number;
  tunnelName: string;
  forwards: Forward[];
}

export default function ForwardPage() {
  const [loading, setLoading] = useState(true);
  const [forwards, setForwards] = useState<Forward[]>([]);
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const isMobile = useMobileBreakpoint();

  // 显示模式状态 - 从localStorage读取，默认为平铺显示
  const [viewMode, setViewMode] = useState<"grouped" | "direct">(() => {
    try {
      const savedMode = localStorage.getItem("forward-view-mode");

      return (savedMode as "grouped" | "direct") || "direct";
    } catch (e) {
      return "direct";
    }
  });

  // 筛选状态
  const [filterUserId, setFilterUserId] = useState<string>("all");
  const [filterTunnelId, setFilterTunnelId] = useState<string>("all");

  // 拖拽排序相关状态
  const [forwardOrder, setForwardOrder] = useState<number[]>([]);

  // 模态框状态
  const [modalOpen, setModalOpen] = useState(false);
  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [addressModalOpen, setAddressModalOpen] = useState(false);
  const [diagnosisModalOpen, setDiagnosisModalOpen] = useState(false);
  const [isEdit, setIsEdit] = useState(false);
  const [submitLoading, setSubmitLoading] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [diagnosisLoading, setDiagnosisLoading] = useState(false);
  const [forwardToDelete, setForwardToDelete] = useState<Forward | null>(null);
  const [currentDiagnosisForward, setCurrentDiagnosisForward] =
    useState<Forward | null>(null);
  const [diagnosisResult, setDiagnosisResult] =
    useState<ForwardDiagnosisResult | null>(null);
  const [addressModalTitle, setAddressModalTitle] = useState("");
  const [addressList, setAddressList] = useState<ForwardAddressItem[]>([]);

  // 导出相关状态
  const [exportModalOpen, setExportModalOpen] = useState(false);
  const [exportData, setExportData] = useState("");
  const [exportLoading, setExportLoading] = useState(false);
  const [selectedTunnelForExport, setSelectedTunnelForExport] = useState<
    number | null
  >(null);

  // 导入相关状态
  const [importModalOpen, setImportModalOpen] = useState(false);
  const [importData, setImportData] = useState("");
  const [importLoading, setImportLoading] = useState(false);
  const [selectedTunnelForImport, setSelectedTunnelForImport] = useState<
    number | null
  >(null);
  const [importResults, setImportResults] = useState<
    Array<{
      line: string;
      success: boolean;
      message: string;
      forwardName?: string;
    }>
  >([]);

  // 表单状态
  const [form, setForm] = useState<ForwardForm>({
    name: "",
    tunnelId: null,
    inPort: null,
    remoteAddr: "",
    interfaceName: "",
    strategy: "fifo",
  });

  // 表单验证错误
  const [errors, setErrors] = useState<{ [key: string]: string }>({});

  // 批量操作相关状态
  const [selectMode, setSelectMode] = useState(false);
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  const [batchDeleteModalOpen, setBatchDeleteModalOpen] = useState(false);
  const [batchChangeTunnelModalOpen, setBatchChangeTunnelModalOpen] =
    useState(false);
  const [batchTargetTunnelId, setBatchTargetTunnelId] = useState<number | null>(
    null,
  );
  const [batchLoading, setBatchLoading] = useState(false);

  useEffect(() => {
    loadData();
  }, []);

  // 切换显示模式并保存到localStorage
  const handleViewModeChange = () => {
    const newMode = viewMode === "grouped" ? "direct" : "grouped";

    setViewMode(newMode);
    try {
      localStorage.setItem("forward-view-mode", newMode);

      // 切换到直接显示模式时，初始化拖拽排序顺序
      if (newMode === "direct") {
        const currentUserId = JwtUtil.getUserIdFromToken();
        const { order, fromDatabase } = buildForwardOrder(
          forwards,
          currentUserId,
        );

        setForwardOrder(order);

        if (fromDatabase) {
          saveOrder(FORWARD_ORDER_KEY, order);
        }
      }
    } catch { }
  };

  // 加载所有数据
  const loadData = async (lod = true) => {
    setLoading(lod);
    try {
      const [forwardsRes, tunnelsRes] = await Promise.all([
        getForwardList(),
        userTunnel(),
      ]);

      if (forwardsRes.code === 0) {
        const forwardsData =
          forwardsRes.data?.map((forward: any) => ({
            ...forward,
            serviceRunning: forward.status === 1,
          })) || [];

        setForwards(forwardsData);

        // 初始化拖拽排序顺序
        if (viewMode === "direct") {
          const currentUserId = JwtUtil.getUserIdFromToken();
          const { order, fromDatabase } = buildForwardOrder(
            forwardsData,
            currentUserId,
          );

          setForwardOrder(order);

          if (fromDatabase) {
            saveOrder(FORWARD_ORDER_KEY, order);
          }
        }
      } else {
        toast.error(forwardsRes.msg || "获取转发列表失败");
      }

      if (tunnelsRes.code === 0) {
        setTunnels(tunnelsRes.data || []);
      } else {
      }
    } catch {
      toast.error("加载数据失败");
    } finally {
      setLoading(false);
    }
  };

  // 按用户和隧道分组转发数据
  const groupForwardsByUserAndTunnel = (): UserGroup[] => {
    const userMap = new Map<string, UserGroup>();

    // 获取排序后的转发列表
    sortedForwards.forEach((forward) => {
      const userKey = forward.userId ? forward.userId.toString() : "unknown";
      const userName = forward.userName || "未知用户";

      if (!userMap.has(userKey)) {
        userMap.set(userKey, {
          userId: forward.userId || null,
          userName,
          tunnelGroups: [],
        });
      }

      const userGroup = userMap.get(userKey)!;
      let tunnelGroup = userGroup.tunnelGroups.find(
        (tg) => tg.tunnelId === forward.tunnelId,
      );

      if (!tunnelGroup) {
        tunnelGroup = {
          tunnelId: forward.tunnelId,
          tunnelName: forward.tunnelName,
          forwards: [],
        };
        userGroup.tunnelGroups.push(tunnelGroup);
      }

      tunnelGroup.forwards.push(forward);
    });

    // 排序：先按用户名，再按隧道名
    const result = Array.from(userMap.values());

    result.sort((a, b) => a.userName.localeCompare(b.userName));
    result.forEach((userGroup) => {
      userGroup.tunnelGroups.sort((a, b) =>
        a.tunnelName.localeCompare(b.tunnelName),
      );
    });

    return result;
  };

  // 表单验证
  const validateForm = (): boolean => {
    const newErrors: { [key: string]: string } = {};

    if (!form.name.trim()) {
      newErrors.name = "请输入转发名称";
    } else if (form.name.length < 2 || form.name.length > 50) {
      newErrors.name = "转发名称长度应在2-50个字符之间";
    }

    if (!form.tunnelId) {
      newErrors.tunnelId = "请选择关联隧道";
    }

    // 验证入口端口（可选，如果填写则验证）
    if (form.inPort !== null && form.inPort !== undefined) {
      const port = Number(form.inPort);

      if (isNaN(port) || port < 1 || port > 65535) {
        newErrors.inPort = "端口必须在 1-65535 之间";
      }
    }

    if (!form.remoteAddr.trim()) {
      newErrors.remoteAddr = "请输入远程地址";
    } else {
      // 验证地址格式
      const addresses = form.remoteAddr
        .split("\n")
        .map((addr) => addr.trim())
        .filter((addr) => addr);
      const ipv4Pattern =
        /^(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?):\d+$/;
      const ipv6FullPattern =
        /^\[((([0-9a-fA-F]{1,4}:){7}([0-9a-fA-F]{1,4}|:))|(([0-9a-fA-F]{1,4}:){6}(:[0-9a-fA-F]{1,4}|((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9a-fA-F]{1,4}:){5}(((:[0-9a-fA-F]{1,4}){1,2})|:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3})|:))|(([0-9a-fA-F]{1,4}:){4}(((:[0-9a-fA-F]{1,4}){1,3})|((:[0-9a-fA-F]{1,4})?:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9a-fA-F]{1,4}:){3}(((:[0-9a-fA-F]{1,4}){1,4})|((:[0-9a-fA-F]{1,4}){0,2}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9a-fA-F]{1,4}:){2}(((:[0-9a-fA-F]{1,4}){1,5})|((:[0-9a-fA-F]{1,4}){0,3}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(([0-9a-fA-F]{1,4}:){1}(((:[0-9a-fA-F]{1,4}){1,6})|((:[0-9a-fA-F]{1,4}){0,4}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:))|(:(((:[0-9a-fA-F]{1,4}){1,7})|((:[0-9a-fA-F]{1,4}){0,5}:((25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}))|:)))\]:\d+$/;
      const domainPattern =
        /^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*:\d+$/;

      for (let i = 0; i < addresses.length; i++) {
        const addr = addresses[i];

        if (
          !ipv4Pattern.test(addr) &&
          !ipv6FullPattern.test(addr) &&
          !domainPattern.test(addr)
        ) {
          newErrors.remoteAddr = `第${i + 1}行地址格式错误`;
          break;
        }
      }
    }

    setErrors(newErrors);

    return Object.keys(newErrors).length === 0;
  };

  // 新增转发
  const handleAdd = () => {
    setIsEdit(false);
    setForm({
      name: "",
      tunnelId: null,
      inPort: null,
      remoteAddr: "",
      interfaceName: "",
      strategy: "fifo",
    });
    setErrors({});
    setModalOpen(true);
  };

  // 编辑转发
  const handleEdit = (forward: Forward) => {
    setIsEdit(true);
    setForm({
      id: forward.id,
      userId: forward.userId,
      name: forward.name,
      tunnelId: forward.tunnelId,
      inPort: forward.inPort,
      remoteAddr: forward.remoteAddr.split(",").join("\n"),
      interfaceName: forward.interfaceName || "",
      strategy: forward.strategy || "fifo",
    });
    setErrors({});
    setModalOpen(true);
  };

  // 显示删除确认
  const handleDelete = (forward: Forward) => {
    setForwardToDelete(forward);
    setDeleteModalOpen(true);
  };

  // 确认删除转发
  const confirmDelete = async () => {
    if (!forwardToDelete) return;

    setDeleteLoading(true);
    try {
      const res = await deleteForward(forwardToDelete.id);

      if (res.code === 0) {
        toast.success("删除成功");
        setDeleteModalOpen(false);
        loadData();
      } else {
        // 删除失败，询问是否强制删除
        const confirmed = window.confirm(
          `常规删除失败：${res.msg || "删除失败"}\n\n是否需要强制删除？\n\n⚠️ 注意：强制删除不会去验证节点端是否已经删除对应的转发服务。`,
        );

        if (confirmed) {
          const forceRes = await forceDeleteForward(forwardToDelete.id);

          if (forceRes.code === 0) {
            toast.success("强制删除成功");
            setDeleteModalOpen(false);
            loadData();
          } else {
            toast.error(forceRes.msg || "强制删除失败");
          }
        }
      }
    } catch {
      toast.error("删除失败");
    } finally {
      setDeleteLoading(false);
    }
  };

  // 处理隧道选择变化
  const handleTunnelChange = (tunnelId: string) => {
    setForm((prev) => ({ ...prev, tunnelId: parseInt(tunnelId) }));
  };

  // 提交表单
  const handleSubmit = async () => {
    if (!validateForm()) return;

    setSubmitLoading(true);
    try {
      const processedRemoteAddr = form.remoteAddr
        .split("\n")
        .map((addr) => addr.trim())
        .filter((addr) => addr)
        .join(",");

      const addressCount = processedRemoteAddr.split(",").length;

      let res: { code: number; msg: string };

      if (isEdit) {
        // 更新时确保包含必要字段
        const updateData = {
          id: form.id,
          userId: form.userId,
          name: form.name,
          tunnelId: form.tunnelId,
          inPort: form.inPort,
          remoteAddr: processedRemoteAddr,
          strategy: addressCount > 1 ? form.strategy : "fifo",
        };

        res = await updateForward(updateData);
      } else {
        // 创建时不需要id和userId（后端会自动设置）
        const createData = {
          name: form.name,
          tunnelId: form.tunnelId,
          inPort: form.inPort,
          remoteAddr: processedRemoteAddr,
          strategy: addressCount > 1 ? form.strategy : "fifo",
        };

        res = await createForward(createData);
      }

      if (res.code === 0) {
        toast.success(isEdit ? "修改成功" : "创建成功");
        setModalOpen(false);
        loadData();
      } else {
        toast.error(res.msg || "操作失败");
      }
    } catch {
      toast.error("操作失败");
    } finally {
      setSubmitLoading(false);
    }
  };

  // 处理服务开关
  const handleServiceToggle = async (forward: Forward) => {
    if (forward.status !== 1 && forward.status !== 0) {
      toast.error("转发状态异常，无法操作");

      return;
    }

    const targetState = !forward.serviceRunning;

    try {
      // 乐观更新UI
      setForwards((prev) =>
        prev.map((f) =>
          f.id === forward.id ? { ...f, serviceRunning: targetState } : f,
        ),
      );

      let res: { code: number; msg: string };

      if (targetState) {
        res = await resumeForwardService(forward.id);
      } else {
        res = await pauseForwardService(forward.id);
      }

      if (res.code === 0) {
        toast.success(targetState ? "服务已启动" : "服务已暂停");
        // 更新转发状态
        setForwards((prev) =>
          prev.map((f) =>
            f.id === forward.id ? { ...f, status: targetState ? 1 : 0 } : f,
          ),
        );
      } else {
        // 操作失败，恢复UI状态
        setForwards((prev) =>
          prev.map((f) =>
            f.id === forward.id ? { ...f, serviceRunning: !targetState } : f,
          ),
        );
        toast.error(res.msg || "操作失败");
      }
    } catch {
      // 操作失败，恢复UI状态
      setForwards((prev) =>
        prev.map((f) =>
          f.id === forward.id ? { ...f, serviceRunning: !targetState } : f,
        ),
      );
      toast.error("网络错误，操作失败");
    }
  };

  // 诊断转发
  const handleDiagnose = async (forward: Forward) => {
    setCurrentDiagnosisForward(forward);
    setDiagnosisModalOpen(true);
    setDiagnosisLoading(true);
    setDiagnosisResult(null);

    try {
      const response = await diagnoseForward(forward.id);

      if (response.code === 0) {
        setDiagnosisResult(response.data as ForwardDiagnosisResult);
      } else {
        toast.error(response.msg || "诊断失败");
        setDiagnosisResult(
          buildForwardDiagnosisFallbackResult({
            forwardName: forward.name,
            remoteAddr: forward.remoteAddr,
            description: "诊断失败",
            message: response.msg || "诊断过程中发生错误",
          }),
        );
      }
    } catch {
      toast.error("网络错误，请重试");
      setDiagnosisResult(
        buildForwardDiagnosisFallbackResult({
          forwardName: forward.name,
          remoteAddr: forward.remoteAddr,
          description: "网络错误",
          message: "无法连接到服务器",
        }),
      );
    } finally {
      setDiagnosisLoading(false);
    }
  };

  // 格式化流量
  const formatFlow = (value: number): string => {
    if (value === 0) return "0 B";
    if (value < 1024) return value + " B";
    if (value < 1024 * 1024) return (value / 1024).toFixed(2) + " KB";
    if (value < 1024 * 1024 * 1024)
      return (value / (1024 * 1024)).toFixed(2) + " MB";

    return (value / (1024 * 1024 * 1024)).toFixed(2) + " GB";
  };

  // 显示地址列表弹窗
  const showAddressModal = (
    addressString: string,
    port: number | null,
    title: string,
  ) => {
    const action = resolveForwardAddressAction(addressString, port, title);

    if (action.type === "none") {
      return;
    }

    if (action.type === "copy") {
      copyToClipboard(action.text, action.label);

      return;
    }

    setAddressList(action.items);
    setAddressModalTitle(action.title);
    setAddressModalOpen(true);
  };

  // 复制到剪贴板
  const copyToClipboard = async (text: string, label: string = "内容") => {
    try {
      await navigator.clipboard.writeText(text);
      toast.success(`已复制${label}`);
    } catch {
      toast.error("复制失败");
    }
  };

  // 复制地址
  const copyAddress = async (addressItem: ForwardAddressItem) => {
    try {
      setAddressList((prev) =>
        prev.map((item) =>
          item.id === addressItem.id ? { ...item, copying: true } : item,
        ),
      );
      await copyToClipboard(addressItem.address, "地址");
    } catch {
      toast.error("复制失败");
    } finally {
      setAddressList((prev) =>
        prev.map((item) =>
          item.id === addressItem.id ? { ...item, copying: false } : item,
        ),
      );
    }
  };

  // 复制所有地址
  const copyAllAddresses = async () => {
    if (addressList.length === 0) return;
    const allAddresses = addressList.map((item) => item.address).join("\n");

    await copyToClipboard(allAddresses, "所有地址");
  };

  // 导出转发数据
  const handleExport = () => {
    setSelectedTunnelForExport(null);
    setExportData("");
    setExportModalOpen(true);
  };

  // 执行导出
  const executeExport = () => {
    if (!selectedTunnelForExport) {
      toast.error("请选择要导出的隧道");

      return;
    }

    setExportLoading(true);

    try {
      // 根据当前显示模式获取要导出的转发列表
      let forwardsToExport: Forward[] = [];

      if (viewMode === "grouped") {
        // 分组模式下，获取指定隧道的转发
        const userGroups = groupForwardsByUserAndTunnel();

        forwardsToExport = userGroups.flatMap((userGroup) =>
          userGroup.tunnelGroups
            .filter(
              (tunnelGroup) => tunnelGroup.tunnelId === selectedTunnelForExport,
            )
            .flatMap((tunnelGroup) => tunnelGroup.forwards),
        );
      } else {
        // 直接显示模式下，过滤指定隧道的转发
        forwardsToExport = sortedForwards.filter(
          (forward) => forward.tunnelId === selectedTunnelForExport,
        );
      }

      if (forwardsToExport.length === 0) {
        toast.error("所选隧道没有转发数据");
        setExportLoading(false);

        return;
      }

      // 格式化导出数据：remoteAddr|name|inPort
      const exportLines = forwardsToExport.map((forward) => {
        return `${forward.remoteAddr}|${forward.name}|${forward.inPort}`;
      });

      const exportText = exportLines.join("\n");

      setExportData(exportText);
    } catch {
      toast.error("导出失败");
    } finally {
      setExportLoading(false);
    }
  };

  // 复制导出数据
  const copyExportData = async () => {
    await copyToClipboard(exportData, "转发数据");
  };

  // 导入转发数据
  const handleImport = () => {
    setImportData("");
    setImportResults([]);
    setSelectedTunnelForImport(null);
    setImportModalOpen(true);
  };

  // 执行导入
  const executeImport = async () => {
    if (!importData.trim()) {
      toast.error("请输入要导入的数据");

      return;
    }

    if (!selectedTunnelForImport) {
      toast.error("请选择要导入的隧道");

      return;
    }

    setImportLoading(true);
    setImportResults([]); // 清空之前的结果

    try {
      const lines = importData
        .trim()
        .split("\n")
        .filter((line) => line.trim());

      for (let i = 0; i < lines.length; i++) {
        const line = lines[i].trim();
        const parts = line.split("|");

        if (parts.length < 2) {
          setImportResults((prev) => [
            {
              line,
              success: false,
              message: "格式错误：需要至少包含目标地址和转发名称",
            },
            ...prev,
          ]);
          continue;
        }

        const [remoteAddr, name, inPort] = parts;

        if (!remoteAddr.trim() || !name.trim()) {
          setImportResults((prev) => [
            {
              line,
              success: false,
              message: "目标地址和转发名称不能为空",
            },
            ...prev,
          ]);
          continue;
        }

        // 验证远程地址格式 - 支持单个地址或多个地址用逗号分隔
        const addresses = remoteAddr.trim().split(",");
        const addressPattern = /^[^:]+:\d+$/;
        const isValidFormat = addresses.every((addr) =>
          addressPattern.test(addr.trim()),
        );

        if (!isValidFormat) {
          setImportResults((prev) => [
            {
              line,
              success: false,
              message:
                "目标地址格式错误，应为 地址:端口 格式，多个地址用逗号分隔",
            },
            ...prev,
          ]);
          continue;
        }

        try {
          // 处理入口端口
          let portNumber: number | null = null;

          if (inPort && inPort.trim()) {
            const port = parseInt(inPort.trim());

            if (isNaN(port) || port < 1 || port > 65535) {
              setImportResults((prev) => [
                {
                  line,
                  success: false,
                  message: "入口端口格式错误，应为1-65535之间的数字",
                },
                ...prev,
              ]);
              continue;
            }
            portNumber = port;
          }

          // 调用创建转发接口
          const response = await createForward({
            name: name.trim(),
            tunnelId: selectedTunnelForImport, // 使用用户选择的隧道
            inPort: portNumber, // 使用指定端口或自动分配
            remoteAddr: remoteAddr.trim(),
            strategy: "fifo",
          });

          if (response.code === 0) {
            setImportResults((prev) => [
              {
                line,
                success: true,
                message: "创建成功",
                forwardName: name.trim(),
              },
              ...prev,
            ]);
          } else {
            setImportResults((prev) => [
              {
                line,
                success: false,
                message: response.msg || "创建失败",
              },
              ...prev,
            ]);
          }
        } catch {
          setImportResults((prev) => [
            {
              line,
              success: false,
              message: "网络错误，创建失败",
            },
            ...prev,
          ]);
        }
      }

      toast.success(`导入执行完成`);

      // 导入完成后刷新转发列表
      await loadData(false);
    } catch {
      toast.error("导入过程中发生错误");
    } finally {
      setImportLoading(false);
    }
  };

  // 获取状态显示
  const getStatusDisplay = (status: number) => {
    switch (status) {
      case 1:
        return { color: "success", text: "正常" };
      case 0:
        return { color: "warning", text: "暂停" };
      case -1:
        return { color: "danger", text: "异常" };
      default:
        return { color: "default", text: "未知" };
    }
  };

  // 获取策略显示
  const getStrategyDisplay = (strategy: string) => {
    switch (strategy) {
      case "fifo":
        return { color: "primary", text: "主备" };
      case "round":
        return { color: "success", text: "轮询" };
      case "rand":
        return { color: "warning", text: "随机" };
      default:
        return { color: "default", text: "未知" };
    }
  };

  // 获取地址数量
  const getAddressCount = (addressString: string): number => {
    if (!addressString) return 0;
    const addresses = addressString
      .split("\n")
      .map((addr) => addr.trim())
      .filter((addr) => addr);

    return addresses.length;
  };

  // 处理拖拽结束
  const handleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event;

    if (!active || !over || active.id === over.id) return;

    // 确保 forwardOrder 存在且有效
    if (!forwardOrder || forwardOrder.length === 0) return;

    const activeId = Number(active.id);
    const overId = Number(over.id);

    // 检查 ID 是否有效
    if (isNaN(activeId) || isNaN(overId)) return;

    const oldIndex = forwardOrder.indexOf(activeId);
    const newIndex = forwardOrder.indexOf(overId);

    if (oldIndex !== -1 && newIndex !== -1 && oldIndex !== newIndex) {
      const newOrder = arrayMove(forwardOrder, oldIndex, newIndex);

      setForwardOrder(newOrder);

      saveOrder(FORWARD_ORDER_KEY, newOrder);

      // 持久化到数据库
      try {
        const forwardsToUpdate = newOrder.map((id, index) => ({
          id,
          inx: index,
        }));

        const response = await updateForwardOrder({
          forwards: forwardsToUpdate,
        });

        if (response.code === 0) {
          // 更新本地数据中的 inx 字段
          setForwards((prev) =>
            prev.map((forward) => {
              const updatedForward = forwardsToUpdate.find(
                (f) => f.id === forward.id,
              );

              if (updatedForward) {
                return { ...forward, inx: updatedForward.inx };
              }

              return forward;
            }),
          );
        } else {
          toast.error("保存排序失败：" + (response.msg || "未知错误"));
        }
      } catch {
        toast.error("保存排序失败，请重试");
      }
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
    const allIds = sortedForwards.map((f) => f.id);

    setSelectedIds(new Set(allIds));
  };

  const deselectAll = () => {
    setSelectedIds(new Set());
  };

  const handleBatchDelete = async () => {
    if (selectedIds.size === 0) return;
    setBatchLoading(true);
    try {
      const outcome = await executeForwardBatchDelete(Array.from(selectedIds));

      if (outcome.toastVariant === "success") {
        toast.success(outcome.toastMessage);
      } else {
        toast.error(outcome.toastMessage);
      }

      if (outcome.shouldRefresh) {
        setSelectedIds(new Set());
        setSelectMode(false);
        if (outcome.closeDeleteModal) {
          setBatchDeleteModalOpen(false);
        }
        loadData(false);
      }
    } finally {
      setBatchLoading(false);
    }
  };

  const handleBatchToggleService = async (enable: boolean) => {
    if (selectedIds.size === 0) return;
    setBatchLoading(true);
    try {
      const outcome = await executeForwardBatchToggleService(
        Array.from(selectedIds),
        enable,
      );

      if (outcome.toastVariant === "success") {
        toast.success(outcome.toastMessage);
      } else {
        toast.error(outcome.toastMessage);
      }

      if (outcome.shouldRefresh) {
        setSelectedIds(new Set());
        setSelectMode(false);
        loadData(false);
      }
    } finally {
      setBatchLoading(false);
    }
  };

  const handleBatchRedeploy = async () => {
    if (selectedIds.size === 0) return;
    setBatchLoading(true);
    try {
      const outcome = await executeForwardBatchRedeploy(
        Array.from(selectedIds),
      );

      if (outcome.toastVariant === "success") {
        toast.success(outcome.toastMessage);
      } else {
        toast.error(outcome.toastMessage);
      }

      if (outcome.shouldRefresh) {
        setSelectedIds(new Set());
        setSelectMode(false);
        loadData(false);
      }
    } finally {
      setBatchLoading(false);
    }
  };

  const handleBatchChangeTunnel = async () => {
    if (selectedIds.size === 0 || !batchTargetTunnelId) return;
    setBatchLoading(true);
    try {
      const outcome = await executeForwardBatchChangeTunnel(
        Array.from(selectedIds),
        batchTargetTunnelId,
      );

      if (outcome.toastVariant === "success") {
        toast.success(outcome.toastMessage);
      } else {
        toast.error(outcome.toastMessage);
      }

      if (outcome.shouldRefresh) {
        setSelectedIds(new Set());
        setSelectMode(false);
        if (outcome.closeChangeTunnelModal) {
          setBatchChangeTunnelModalOpen(false);
        }
        if (outcome.resetTargetTunnel) {
          setBatchTargetTunnelId(null);
        }
        loadData(false);
      }
    } finally {
      setBatchLoading(false);
    }
  };

  // 传感器配置 - 使用默认配置避免错误
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

  const tokenUserId = JwtUtil.getUserIdFromToken();

  // 根据排序顺序获取转发列表
  const sortedForwards = useMemo((): Forward[] => {
    // 确保 forwards 数组存在且有效
    if (!forwards || forwards.length === 0) {
      return [];
    }

    // 在平铺模式下，只显示当前用户的转发
    let filteredForwards = forwards;

    if (viewMode === "direct") {
      if (tokenUserId !== null) {
        filteredForwards = forwards.filter(
          (forward) => forward.userId === tokenUserId,
        );
      }
    } else if (viewMode === "grouped") {
      if (filterUserId !== "all") {
        const targetUserId = parseInt(filterUserId);
        filteredForwards = filteredForwards.filter(f => f.userId === targetUserId || (targetUserId === 0 && !f.userId));
      }
      if (filterTunnelId !== "all") {
        const targetTunnelId = parseInt(filterTunnelId);
        filteredForwards = filteredForwards.filter(f => f.tunnelId === targetTunnelId);
      }
    }

    // 确保过滤后的转发列表有效
    if (!filteredForwards || filteredForwards.length === 0) {
      return [];
    }

    // 优先使用数据库中的 inx 字段进行排序
    const sortedByDb = [...filteredForwards].sort((a, b) => {
      const aInx = a.inx ?? 0;
      const bInx = b.inx ?? 0;

      return aInx - bInx;
    });

    // 如果数据库中没有排序信息，则使用本地存储的顺序
    if (
      forwardOrder &&
      forwardOrder.length > 0 &&
      sortedByDb.every((f) => f.inx === undefined || f.inx === 0)
    ) {
      const forwardMap = new Map(filteredForwards.map((f) => [f.id, f]));
      const localSortedForwards: Forward[] = [];

      forwardOrder.forEach((id) => {
        const forward = forwardMap.get(id);

        if (forward) {
          localSortedForwards.push(forward);
        }
      });

      // 添加不在排序列表中的转发（新添加的）
      filteredForwards.forEach((forward) => {
        if (!forwardOrder.includes(forward.id)) {
          localSortedForwards.push(forward);
        }
      });

      return localSortedForwards;
    }

    return sortedByDb;
  }, [forwards, forwardOrder, viewMode, tokenUserId, filterUserId, filterTunnelId]);

  const sortableForwardIds = useMemo(
    () => sortedForwards.map((f) => f.id).filter((id) => id > 0),
    [sortedForwards],
  );

  // 可拖拽的转发卡片组件
  const SortableForwardCard = ({ forward }: { forward: Forward }) => {
    const {
      attributes,
      listeners,
      setNodeRef,
      transform,
      transition,
      isDragging,
    } = useSortable({ id: forward.id });

    const style = {
      transform: transform ? CSS.Transform.toString(transform) : undefined,
      transition: isDragging ? undefined : transition || undefined,
      opacity: isDragging ? 0.5 : 1,
      willChange: "transform",
    };

    return (
      <div ref={setNodeRef} style={style} {...attributes}>
        {renderForwardCard(forward, listeners)}
      </div>
    );
  };

  // 生成用作筛选项的用户和隧道列表
  const uniqueUsers = useMemo(() => {
    const userMap = new Map<number, { id: number; name: string }>();
    forwards.forEach((f) => {
      const uId = f.userId || 0;
      if (!userMap.has(uId)) {
        userMap.set(uId, { id: uId, name: f.userName || "未知用户" });
      }
    });

    return Array.from(userMap.values());
  }, [forwards]);

  // 可拖拽的表格行组件
  const SortableTableRow = ({ forward, selectMode, selectedIds, toggleSelect, getStrategyDisplay, formatInAddress, formatRemoteAddress, handleServiceToggle, handleEdit, handleDelete, handleDiagnose, showAddressModal, hasMultipleAddresses }: any) => {
    const {
      attributes,
      listeners,
      setNodeRef,
      transform,
      transition,
      isDragging,
    } = useSortable({ id: forward.id });

    const style = {
      transform: transform ? CSS.Transform.toString(transform) : undefined,
      transition: isDragging ? undefined : transition || undefined,
      opacity: isDragging ? 0.5 : 1,
      backgroundColor: isDragging ? "var(--nextui-default-100)" : undefined,
    };

    const strategyDisplay = getStrategyDisplay(forward.strategy);

    return (
      <TableRow ref={setNodeRef} style={style} key={forward.id}>
        {selectMode && (
          <TableCell>
            <Checkbox
              isSelected={selectedIds.has(forward.id)}
              onValueChange={() => toggleSelect(forward.id)}
            />
          </TableCell>
        )}
        <TableCell>
          <div
            className="cursor-grab active:cursor-grabbing p-1 text-default-400 hover:text-default-600 transition-colors"
            {...attributes}
            {...listeners}
            title="拖拽排序"
          >
            <svg aria-hidden="true" className="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
              <path d="M7 2a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 2zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 8zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 7 14zm6-8a2 2 0 1 1-.001-4.001A2 2 0 0 1 13 6zm0 2a2 2 0 1 1 .001 4.001A2 2 0 0 1 13 8zm0 6a2 2 0 1 1 .001 4.001A2 2 0 0 1 13 14z" />
            </svg>
          </div>
        </TableCell>
        <TableCell className="whitespace-nowrap">
          <span className="text-sm font-medium text-default-700">
            {forward.userName || "未知用户"}
          </span>
        </TableCell>
        <TableCell className="whitespace-nowrap">
          <Chip
            size="sm"
            color="secondary"
            className="border-none bg-secondary/10 px-2"
          >
            <span className="font-medium text-secondary-700">{forward.tunnelName}</span>
          </Chip>
        </TableCell>
        <TableCell className="whitespace-nowrap font-semibold text-foreground">
          {forward.name}
        </TableCell>
        <TableCell className="max-w-[220px]">
          <button
            className={`w-full truncate rounded-md bg-default-100/50 px-2.5 py-1.5 text-left font-mono text-xs font-medium text-default-700 transition-all ${hasMultipleAddresses(forward.inIp)
              ? "hover:bg-default-200 hover:shadow-sm"
              : ""
              }`}
            title={formatInAddress(forward.inIp, forward.inPort)}
            type="button"
            onClick={() =>
              showAddressModal(forward.inIp, forward.inPort, "入口端口")
            }
          >
            {formatInAddress(forward.inIp, forward.inPort)}
          </button>
        </TableCell>
        <TableCell className="max-w-[240px]">
          <button
            className={`w-full truncate rounded-md bg-default-100/50 px-2.5 py-1.5 text-left font-mono text-xs font-medium text-default-700 transition-all ${hasMultipleAddresses(forward.remoteAddr)
              ? "hover:bg-default-200 hover:shadow-sm"
              : ""
              }`}
            title={formatRemoteAddress(forward.remoteAddr)}
            type="button"
            onClick={() =>
              showAddressModal(forward.remoteAddr, null, "目标地址")
            }
          >
            {formatRemoteAddress(forward.remoteAddr)}
          </button>
        </TableCell>
        <TableCell>
          <Chip
            className="text-xs font-medium"
            color={strategyDisplay.color as any}
            size="sm"
            variant="flat"
          >
            {strategyDisplay.text}
          </Chip>
        </TableCell>
        <TableCell>
          <div className="flex items-center gap-2.5 whitespace-nowrap">
            <Switch
              isDisabled={forward.status !== 1 && forward.status !== 0}
              isSelected={forward.serviceRunning}
              size="sm"
              color="success"
              onValueChange={() => handleServiceToggle(forward)}
            />
          </div>
        </TableCell>
        <TableCell>
          <div className="flex justify-end gap-2">
            <Button
              isIconOnly
              className="bg-primary/10 text-primary hover:bg-primary/20"
              size="sm"
              title="编辑"
              onPress={() => handleEdit(forward)}
            >
              <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
              </svg>
            </Button>
            <Button
              isIconOnly
              className="bg-warning/10 text-warning hover:bg-warning/20"
              size="sm"
              title="诊断"
              onPress={() => handleDiagnose(forward)}
            >
              <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4" />
              </svg>
            </Button>
            <Button
              isIconOnly
              className="bg-danger/10 text-danger hover:bg-danger/20"
              size="sm"
              title="删除"
              onPress={() => handleDelete(forward)}
            >
              <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
              </svg>
            </Button>
          </div>
        </TableCell>
      </TableRow>
    );
  };

  // 渲染转发卡片
  const renderForwardCard = (forward: Forward, listeners?: any) => {
    const statusDisplay = getStatusDisplay(forward.status);
    const strategyDisplay = getStrategyDisplay(forward.strategy);

    return (
      <Card
        key={forward.id}
        className="group shadow-sm border border-divider hover:shadow-md transition-shadow duration-200 overflow-hidden"
      >
        <CardHeader className="pb-2 md:pb-2">
          <div className="flex justify-between items-start w-full">
            {selectMode && (
              <Checkbox
                className="mr-2"
                isSelected={selectedIds.has(forward.id)}
                onValueChange={() => toggleSelect(forward.id)}
              />
            )}
            <div className="flex-1 min-w-0">
              <h3 className="font-semibold text-foreground truncate text-sm">
                {forward.name}
              </h3>
              <p className="text-xs text-default-500 truncate">
                {forward.tunnelName}
              </p>
            </div>
            <div className="flex items-center gap-1.5 ml-2">
              {viewMode === "direct" && (
                <div
                  className={`cursor-grab active:cursor-grabbing p-2 text-default-400 hover:text-default-600 transition-colors touch-manipulation ${isMobile
                    ? "opacity-100" // 移动端始终显示
                    : "opacity-0 group-hover:opacity-100 sm:opacity-0 sm:group-hover:opacity-100"
                    }`}
                  {...listeners}
                  style={{ touchAction: "none" }}
                  title={isMobile ? "长按拖拽排序" : "拖拽排序"}
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
              )}
              <Switch
                isDisabled={forward.status !== 1 && forward.status !== 0}
                isSelected={forward.serviceRunning}
                size="sm"
                onValueChange={() => handleServiceToggle(forward)}
              />
              <Chip
                className="text-xs"
                color={statusDisplay.color as any}
                size="sm"
                variant="flat"
              >
                {statusDisplay.text}
              </Chip>
            </div>
          </div>
        </CardHeader>

        <CardBody className="pt-0 pb-3 md:pt-0 md:pb-3">
          <div className="space-y-2">
            {/* 地址信息 */}
            <div className="space-y-1">
              <button
                className={`cursor-pointer px-2 py-1 bg-default-50 dark:bg-default-100/50 rounded border border-default-200 dark:border-default-300 transition-colors duration-200 ${hasMultipleAddresses(forward.inIp)
                  ? "hover:bg-default-100 dark:hover:bg-default-200/50"
                  : ""
                  }`}
                title={formatInAddress(forward.inIp, forward.inPort)}
                type="button"
                onClick={() =>
                  showAddressModal(forward.inIp, forward.inPort, "入口端口")
                }
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-1.5 min-w-0 flex-1">
                    <span className="text-xs font-medium text-default-600 flex-shrink-0">
                      入口:
                    </span>
                    <code className="text-xs font-mono text-foreground truncate min-w-0">
                      {formatInAddress(forward.inIp, forward.inPort)}
                    </code>
                  </div>
                  {hasMultipleAddresses(forward.inIp) && (
                    <svg
                      aria-hidden="true"
                      className="w-3 h-3 text-default-400 flex-shrink-0"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                      />
                    </svg>
                  )}
                </div>
              </button>

              <button
                className={`cursor-pointer px-2 py-1 bg-default-50 dark:bg-default-100/50 rounded border border-default-200 dark:border-default-300 transition-colors duration-200 ${hasMultipleAddresses(forward.remoteAddr)
                  ? "hover:bg-default-100 dark:hover:bg-default-200/50"
                  : ""
                  }`}
                title={formatRemoteAddress(forward.remoteAddr)}
                type="button"
                onClick={() =>
                  showAddressModal(forward.remoteAddr, null, "目标地址")
                }
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-1.5 min-w-0 flex-1">
                    <span className="text-xs font-medium text-default-600 flex-shrink-0">
                      目标:
                    </span>
                    <code className="text-xs font-mono text-foreground truncate min-w-0">
                      {formatRemoteAddress(forward.remoteAddr)}
                    </code>
                  </div>
                  {hasMultipleAddresses(forward.remoteAddr) && (
                    <svg
                      aria-hidden="true"
                      className="w-3 h-3 text-default-400 flex-shrink-0"
                      fill="none"
                      stroke="currentColor"
                      viewBox="0 0 24 24"
                    >
                      <path
                        d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        strokeWidth={2}
                      />
                    </svg>
                  )}
                </div>
              </button>
            </div>

            {/* 统计信息 */}
            <div className="flex items-center justify-between pt-2 border-t border-divider">
              <Chip
                className="text-xs"
                color={strategyDisplay.color as any}
                size="sm"
                variant="flat"
              >
                {strategyDisplay.text}
              </Chip>
              <div className="flex items-center gap-1">
                <Chip
                  className="text-xs"
                  color="primary"
                  size="sm"
                  variant="flat"
                >
                  ↑{formatFlow(forward.inFlow || 0)}
                </Chip>
              </div>
              <Chip
                className="text-xs"
                color="success"
                size="sm"
                variant="flat"
              >
                ↓{formatFlow(forward.outFlow || 0)}
              </Chip>
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
              onPress={() => handleEdit(forward)}
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
              onPress={() => handleDiagnose(forward)}
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
              onPress={() => handleDelete(forward)}
            >
              删除
            </Button>
          </div>
        </CardBody>
      </Card>
    );
  };

  if (loading) {
    return <PageLoadingState message="正在加载..." />;
  }

  return (
    <div className="px-3 lg:px-6 py-8">
      {/* 页面头部 */}
      <div className="flex items-center justify-between mb-6 gap-2">
        <div className="flex items-center gap-2 flex-shrink-0">
          {viewMode === "grouped" && (
            <>
              <div className="w-[130px] sm:w-[150px]">
                <Select
                  aria-label="筛选用户"
                  size="sm"
                  variant="bordered"
                  selectedKeys={[filterUserId]}
                  onSelectionChange={(keys) => {
                    const key = Array.from(keys)[0] as string;
                    setFilterUserId(key || "all");
                  }}
                >
                  <SelectItem key="all">全部用户</SelectItem>
                  {uniqueUsers.map(user => (
                    <SelectItem key={user.id.toString()}>{user.name}</SelectItem>
                  ))}
                </Select>
              </div>

              <div className="w-[130px] sm:w-[150px]">
                <Select
                  aria-label="筛选隧道"
                  size="sm"
                  variant="bordered"
                  selectedKeys={[filterTunnelId]}
                  onSelectionChange={(keys) => {
                    const key = Array.from(keys)[0] as string;
                    setFilterTunnelId(key || "all");
                  }}
                >
                  <SelectItem key="all">全部隧道</SelectItem>
                  {tunnels.map(tunnel => (
                    <SelectItem key={tunnel.id.toString()}>{tunnel.name}</SelectItem>
                  ))}
                </Select>
              </div>
            </>
          )}
        </div>
        <div className="flex flex-1 flex-wrap items-center justify-end gap-2 sm:gap-3">
          {/* 显示模式切换按钮 */}
          <Button
            isIconOnly
            aria-label={
              viewMode === "grouped" ? "切换到直接显示" : "切换到分类显示"
            }
            className="text-sm"
            color="default"
            size="sm"
            title={viewMode === "grouped" ? "切换到直接显示" : "切换到分类显示"}
            variant="flat"
            onPress={handleViewModeChange}
          >
            {viewMode === "grouped" ? (
              <svg
                aria-hidden="true"
                className="w-4 h-4"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path
                  clipRule="evenodd"
                  d="M3 4a1 1 0 011-1h12a1 1 0 011 1v2a1 1 0 01-1 1H4a1 1 0 01-1-1V4zM3 10a1 1 0 011-1h12a1 1 0 011 1v2a1 1 0 01-1 1H4a1 1 0 01-1-1v-2zM3 16a1 1 0 011-1h12a1 1 0 011 1v2a1 1 0 01-1 1H4a1 1 0 01-1-1v-2z"
                  fillRule="evenodd"
                />
              </svg>
            ) : (
              <svg
                aria-hidden="true"
                className="w-4 h-4"
                fill="currentColor"
                viewBox="0 0 20 20"
              >
                <path d="M3 4a1 1 0 011-1h12a1 1 0 011 1v2a1 1 0 01-1 1H4a1 1 0 01-1-1V4zM3 10a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H4a1 1 0 01-1-1v-6zM14 9a1 1 0 00-1 1v6a1 1 0 001 1h2a1 1 0 001-1v-6a1 1 0 00-1-1h-2z" />
              </svg>
            )}
          </Button>

          {/* 导入按钮 */}
          <Button
            color="warning"
            size="sm"
            variant="flat"
            onPress={handleImport}
          >
            导入
          </Button>

          {/* 导出按钮 */}
          <Button
            color="success"
            isLoading={exportLoading}
            size="sm"
            variant="flat"
            onPress={handleExport}
          >
            导出
          </Button>

          <Button
            color={selectMode ? "secondary" : "default"}
            size="sm"
            variant={selectMode ? "solid" : "flat"}
            onPress={toggleSelectMode}
          >
            {selectMode ? "退出" : "批量"}
          </Button>

          <Button color="primary" size="sm" variant="flat" onPress={handleAdd}>
            新增
          </Button>
        </div>
      </div>

      {selectMode && selectedIds.size > 0 && (
        <div className="fixed bottom-7 left-1/2 z-50 w-[calc(100vw-1rem)] max-w-max -translate-x-1/2 overflow-x-auto rounded-lg border border-divider bg-content1 p-2 shadow-lg">
          <div className="flex min-w-max items-center gap-2">
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
              size="sm"
              variant="flat"
              onPress={() => setBatchDeleteModalOpen(true)}
            >
              删除
            </Button>
            <Button
              color="warning"
              isLoading={batchLoading}
              size="sm"
              variant="flat"
              onPress={() => handleBatchToggleService(false)}
            >
              停用
            </Button>
            <Button
              color="success"
              isLoading={batchLoading}
              size="sm"
              variant="flat"
              onPress={() => handleBatchToggleService(true)}
            >
              启用
            </Button>
            <Button
              color="primary"
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
              variant="flat"
              onPress={() => setBatchChangeTunnelModalOpen(true)}
            >
              隧道
            </Button>
          </div>
        </div>
      )}

      {/* 根据显示模式渲染不同内容 */}
      {viewMode === "grouped" ? (
        sortedForwards.length > 0 ? (
          <div className="overflow-hidden rounded-xl border border-divider bg-content1 shadow-md">
            <DndContext
              collisionDetection={closestCenter}
              sensors={sensors}
              onDragEnd={handleDragEnd}
            >
              <Table
                aria-label="全部转发列表"
                classNames={{
                  th: "bg-default-100/50 text-default-600 font-semibold text-sm border-b border-divider py-3 uppercase tracking-wider",
                  td: "py-3 border-b border-divider/50 group-data-[last=true]:border-b-0",
                  tr: "hover:bg-default-50/50 transition-colors",
                }}
              >
                <TableHeader>
                  {selectMode && <TableColumn className="w-14">选择</TableColumn>}
                  <TableColumn className="w-10 pl-4"></TableColumn>
                  <TableColumn>用户</TableColumn>
                  <TableColumn>隧道</TableColumn>
                  <TableColumn>名称</TableColumn>
                  <TableColumn>入口</TableColumn>
                  <TableColumn>目标</TableColumn>
                  <TableColumn>策略</TableColumn>
                  <TableColumn>状态</TableColumn>
                  <TableColumn className="text-right">操作</TableColumn>
                </TableHeader>
                <TableBody emptyContent="暂无转发配置" items={sortedForwards}>
                  {(forward) => (
                    <SortableContext items={sortableForwardIds} strategy={verticalListSortingStrategy} key={forward.id}>
                      <SortableTableRow
                        forward={forward}
                        selectMode={selectMode}
                        selectedIds={selectedIds}
                        toggleSelect={toggleSelect}
                        getStrategyDisplay={getStrategyDisplay}
                        formatInAddress={formatInAddress}
                        formatRemoteAddress={formatRemoteAddress}
                        handleServiceToggle={handleServiceToggle}
                        handleEdit={handleEdit}
                        handleDelete={handleDelete}
                        handleDiagnose={handleDiagnose}
                        showAddressModal={showAddressModal}
                        hasMultipleAddresses={hasMultipleAddresses}
                      />
                    </SortableContext>
                  )}
                </TableBody>
              </Table>
            </DndContext>
          </div>
        ) : (
          /* 空状态 */
          <Card className="shadow-sm border border-gray-200 dark:border-gray-700">
            <CardBody className="text-center py-16">
              <div className="flex flex-col items-center gap-4">
                <div className="w-16 h-16 bg-default-100 rounded-full flex items-center justify-center">
                  <svg
                    aria-hidden="true"
                    className="w-8 h-8 text-default-400"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      d="M8 9l4-4 4 4m0 6l-4 4-4-4"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth={1.5}
                    />
                  </svg>
                </div>
                <div>
                  <h3 className="text-lg font-semibold text-foreground">
                    暂无转发配置
                  </h3>
                  <p className="text-default-500 text-sm mt-1">
                    还没有创建任何转发配置，点击上方按钮开始创建
                  </p>
                </div>
              </div>
            </CardBody>
          </Card>
        )
      ) : /* 直接显示模式 */
        forwards.length > 0 ? (
          <DndContext
            collisionDetection={closestCenter}
            sensors={sensors}
            onDragEnd={handleDragEnd}
            onDragStart={() => { }} // 添加空的 onDragStart 处理器
          >
            <SortableContext
              items={sortableForwardIds}
              strategy={rectSortingStrategy}
            >
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 gap-4">
                {sortedForwards.map((forward) =>
                  forward && forward.id ? (
                    <SortableForwardCard key={forward.id} forward={forward} />
                  ) : null,
                )}
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
                    aria-hidden="true"
                    className="w-8 h-8 text-default-400"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      d="M8 9l4-4 4 4m0 6l-4 4-4-4"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth={1.5}
                    />
                  </svg>
                </div>
                <div>
                  <h3 className="text-lg font-semibold text-foreground">
                    暂无转发配置
                  </h3>
                  <p className="text-default-500 text-sm mt-1">
                    还没有创建任何转发配置，点击上方按钮开始创建
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
                  {isEdit ? "编辑转发" : "新增转发"}
                </h2>
                <p className="text-small text-default-500">
                  {isEdit ? "修改现有转发配置的信息" : "创建新的转发配置"}
                </p>
              </ModalHeader>
              <ModalBody>
                <div className="space-y-4 pb-4">
                  <Input
                    errorMessage={errors.name}
                    isInvalid={!!errors.name}
                    label="转发名称"
                    placeholder="请输入转发名称"
                    value={form.name}
                    variant="bordered"
                    onChange={(e) =>
                      setForm((prev) => ({ ...prev, name: e.target.value }))
                    }
                  />

                  <Select
                    description={
                      isEdit
                        ? "更改隧道将释放原端口并在新隧道分配端口"
                        : undefined
                    }
                    errorMessage={errors.tunnelId}
                    isInvalid={!!errors.tunnelId}
                    label="选择隧道"
                    placeholder="请选择关联的隧道"
                    selectedKeys={
                      form.tunnelId ? [form.tunnelId.toString()] : []
                    }
                    variant="bordered"
                    onSelectionChange={(keys) => {
                      const selectedKey = Array.from(keys)[0] as string;

                      if (selectedKey) {
                        handleTunnelChange(selectedKey);
                      }
                    }}
                  >
                    {tunnels.map((tunnel) => (
                      <SelectItem key={tunnel.id}>{tunnel.name}</SelectItem>
                    ))}
                  </Select>

                  <Input
                    description="指定入口端口，留空则从节点可用端口中自动分配"
                    errorMessage={errors.inPort}
                    isInvalid={!!errors.inPort}
                    label="入口端口"
                    placeholder="留空则自动分配可用端口"
                    type="number"
                    value={form.inPort !== null ? form.inPort.toString() : ""}
                    variant="bordered"
                    onChange={(e) => {
                      const value = e.target.value;

                      setForm((prev) => ({
                        ...prev,
                        inPort: value ? parseInt(value) : null,
                      }));
                    }}
                  />

                  <Textarea
                    description="格式: IP:端口 或 域名:端口，支持多个地址（每行一个）"
                    errorMessage={errors.remoteAddr}
                    isInvalid={!!errors.remoteAddr}
                    label="远程地址"
                    maxRows={6}
                    minRows={3}
                    placeholder="请输入远程地址，多个地址用换行分隔&#10;例如:&#10;192.168.1.100:8080&#10;example.com:3000"
                    value={form.remoteAddr}
                    variant="bordered"
                    onChange={(e) =>
                      setForm((prev) => ({
                        ...prev,
                        remoteAddr: e.target.value,
                      }))
                    }
                  />

                  {getAddressCount(form.remoteAddr) > 1 && (
                    <Select
                      description="多个目标地址的负载均衡策略"
                      label="负载策略"
                      placeholder="请选择负载均衡策略"
                      selectedKeys={[form.strategy]}
                      variant="bordered"
                      onSelectionChange={(keys) => {
                        const selectedKey = Array.from(keys)[0] as string;

                        setForm((prev) => ({ ...prev, strategy: selectedKey }));
                      }}
                    >
                      <SelectItem key="fifo">主备模式 - 自上而下</SelectItem>
                      <SelectItem key="round">轮询模式 - 依次轮换</SelectItem>
                      <SelectItem key="rand">随机模式 - 随机选择</SelectItem>
                      <SelectItem key="hash">哈希模式 - IP哈希</SelectItem>
                    </Select>
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
                  {isEdit ? "保存修改" : "创建转发"}
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
                <h2 className="text-lg font-bold text-danger">确认删除</h2>
              </ModalHeader>
              <ModalBody>
                <p className="text-default-600">
                  确定要删除转发{" "}
                  <span className="font-semibold text-foreground">
                    &quot;{forwardToDelete?.name}&quot;
                  </span>{" "}
                  吗？
                </p>
                <p className="text-small text-default-500 mt-2">
                  此操作无法撤销，删除后该转发将永久消失。
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
                  确认删除
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {/* 地址列表弹窗 */}
      <Modal
        isOpen={addressModalOpen}
        scrollBehavior="outside"
        size="lg"
        onClose={() => setAddressModalOpen(false)}
      >
        <ModalContent>
          <ModalHeader className="text-base">{addressModalTitle}</ModalHeader>
          <ModalBody className="pb-6">
            <div className="mb-4 text-right">
              <Button size="sm" onPress={copyAllAddresses}>
                复制
              </Button>
            </div>

            <div className="space-y-2 max-h-60 overflow-y-auto">
              {addressList.map((item) => (
                <div
                  key={item.id}
                  className="flex justify-between items-center p-3 border border-default-200 dark:border-default-100 rounded-lg"
                >
                  <code className="text-sm flex-1 mr-3 text-foreground">
                    {item.address}
                  </code>
                  <Button
                    isLoading={item.copying}
                    size="sm"
                    variant="light"
                    onPress={() => copyAddress(item)}
                  >
                    复制
                  </Button>
                </div>
              ))}
            </div>
          </ModalBody>
        </ModalContent>
      </Modal>

      {/* 导出数据模态框 */}
      <Modal
        backdrop="blur"
        isOpen={exportModalOpen}
        placement="center"
        scrollBehavior="outside"
        size="2xl"
        onClose={() => {
          setExportModalOpen(false);
          setSelectedTunnelForExport(null);
          setExportData("");
        }}
      >
        <ModalContent>
          <ModalHeader className="flex flex-col gap-1">
            <h2 className="text-xl font-bold">导出转发数据</h2>
            <p className="text-small text-default-500">
              格式：目标地址|转发名称|入口端口
            </p>
          </ModalHeader>
          <ModalBody className="pb-6">
            <div className="space-y-4">
              {/* 隧道选择 */}
              <div>
                <Select
                  isRequired
                  label="选择导出隧道"
                  placeholder="请选择要导出的隧道"
                  selectedKeys={
                    selectedTunnelForExport
                      ? [selectedTunnelForExport.toString()]
                      : []
                  }
                  variant="bordered"
                  onSelectionChange={(keys) => {
                    const selectedKey = Array.from(keys)[0] as string;

                    setSelectedTunnelForExport(
                      selectedKey ? parseInt(selectedKey) : null,
                    );
                  }}
                >
                  {tunnels.map((tunnel) => (
                    <SelectItem
                      key={tunnel.id.toString()}
                      textValue={tunnel.name}
                    >
                      {tunnel.name}
                    </SelectItem>
                  ))}
                </Select>
              </div>

              {/* 导出按钮和数据 */}
              {exportData && (
                <div className="flex justify-between items-center">
                  <Button
                    color="primary"
                    isDisabled={!selectedTunnelForExport}
                    isLoading={exportLoading}
                    size="sm"
                    startContent={
                      <svg
                        aria-hidden="true"
                        className="w-4 h-4"
                        fill="currentColor"
                        viewBox="0 0 20 20"
                      >
                        <path
                          clipRule="evenodd"
                          d="M3 17a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zM6.293 6.707a1 1 0 010-1.414l3-3a1 1 0 011.414 0l3 3a1 1 0 01-1.414 1.414L11 5.414V13a1 1 0 11-2 0V5.414L7.707 6.707a1 1 0 01-1.414 0z"
                          fillRule="evenodd"
                        />
                      </svg>
                    }
                    onPress={executeExport}
                  >
                    重新生成
                  </Button>
                  <Button
                    color="secondary"
                    size="sm"
                    startContent={
                      <svg
                        aria-hidden="true"
                        className="w-4 h-4"
                        fill="currentColor"
                        viewBox="0 0 20 20"
                      >
                        <path d="M8 3a1 1 0 011-1h2a1 1 0 110 2H9a1 1 0 01-1-1z" />
                        <path d="M6 3a2 2 0 00-2 2v11a2 2 0 002 2h8a2 2 0 002-2V5a2 2 0 00-2-2 3 3 0 01-3 3H9a3 3 0 01-3-3z" />
                      </svg>
                    }
                    onPress={copyExportData}
                  >
                    复制
                  </Button>
                </div>
              )}

              {/* 初始导出按钮 */}
              {!exportData && (
                <div className="text-right">
                  <Button
                    color="primary"
                    isDisabled={!selectedTunnelForExport}
                    isLoading={exportLoading}
                    size="sm"
                    startContent={
                      <svg
                        aria-hidden="true"
                        className="w-4 h-4"
                        fill="currentColor"
                        viewBox="0 0 20 20"
                      >
                        <path
                          clipRule="evenodd"
                          d="M3 17a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zM6.293 6.707a1 1 0 010-1.414l3-3a1 1 0 011.414 0l3 3a1 1 0 01-1.414 1.414L11 5.414V13a1 1 0 11-2 0V5.414L7.707 6.707a1 1 0 01-1.414 0z"
                          fillRule="evenodd"
                        />
                      </svg>
                    }
                    onPress={executeExport}
                  >
                    生成导出数据
                  </Button>
                </div>
              )}

              {/* 导出数据显示 */}
              {exportData && (
                <div className="relative">
                  <Textarea
                    readOnly
                    className="font-mono text-sm"
                    classNames={{
                      input: "font-mono text-sm",
                    }}
                    maxRows={20}
                    minRows={10}
                    placeholder="暂无数据"
                    value={exportData}
                    variant="bordered"
                  />
                </div>
              )}
            </div>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={() => setExportModalOpen(false)}>
              关闭
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* 导入数据模态框 */}
      <Modal
        backdrop="blur"
        isOpen={importModalOpen}
        placement="center"
        scrollBehavior="outside"
        size="2xl"
        onClose={() => setImportModalOpen(false)}
      >
        <ModalContent>
          <ModalHeader className="flex flex-col gap-1">
            <h2 className="text-xl font-bold">导入转发数据</h2>
            <p className="text-small text-default-500">
              格式：目标地址|转发名称|入口端口，每行一个，入口端口留空将自动分配可用端口
            </p>
            <p className="text-small text-default-400">
              目标地址支持单个地址(如：example.com:8080)或多个地址用逗号分隔(如：3.3.3.3:3,4.4.4.4:4)
            </p>
          </ModalHeader>
          <ModalBody className="pb-6">
            <div className="space-y-4">
              {/* 隧道选择 */}
              <div>
                <Select
                  isRequired
                  label="选择导入隧道"
                  placeholder="请选择要导入的隧道"
                  selectedKeys={
                    selectedTunnelForImport
                      ? [selectedTunnelForImport.toString()]
                      : []
                  }
                  variant="bordered"
                  onSelectionChange={(keys) => {
                    const selectedKey = Array.from(keys)[0] as string;

                    setSelectedTunnelForImport(
                      selectedKey ? parseInt(selectedKey) : null,
                    );
                  }}
                >
                  {tunnels.map((tunnel) => (
                    <SelectItem
                      key={tunnel.id.toString()}
                      textValue={tunnel.name}
                    >
                      {tunnel.name}
                    </SelectItem>
                  ))}
                </Select>
              </div>

              {/* 输入区域 */}
              <div>
                <Textarea
                  classNames={{
                    input: "font-mono text-sm",
                  }}
                  label="导入数据"
                  maxRows={12}
                  minRows={8}
                  placeholder="请输入要导入的转发数据，格式：目标地址|转发名称|入口端口"
                  value={importData}
                  variant="flat"
                  onChange={(e) => setImportData(e.target.value)}
                />
              </div>

              {/* 导入结果 */}
              {importResults.length > 0 && (
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <h3 className="text-base font-semibold">导入结果</h3>
                    <div className="flex items-center gap-2">
                      <span className="text-xs text-default-500">
                        成功：{importResults.filter((r) => r.success).length} /
                        总计：{importResults.length}
                      </span>
                    </div>
                  </div>

                  <div
                    className="max-h-40 overflow-y-auto space-y-1"
                    style={{
                      scrollbarWidth: "thin",
                      scrollbarColor: "rgb(156 163 175) transparent",
                    }}
                  >
                    {importResults.map((result, index) => (
                      <div
                        key={index}
                        className={`p-2 rounded border ${result.success
                          ? "bg-success-50 dark:bg-success-100/10 border-success-200 dark:border-success-300/20"
                          : "bg-danger-50 dark:bg-danger-100/10 border-danger-200 dark:border-danger-300/20"
                          }`}
                      >
                        <div className="flex items-center gap-2">
                          {result.success ? (
                            <svg
                              aria-hidden="true"
                              className="w-3 h-3 text-success-600 flex-shrink-0"
                              fill="currentColor"
                              viewBox="0 0 20 20"
                            >
                              <path
                                clipRule="evenodd"
                                d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
                                fillRule="evenodd"
                              />
                            </svg>
                          ) : (
                            <svg
                              aria-hidden="true"
                              className="w-3 h-3 text-danger-600 flex-shrink-0"
                              fill="currentColor"
                              viewBox="0 0 20 20"
                            >
                              <path
                                clipRule="evenodd"
                                d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
                                fillRule="evenodd"
                              />
                            </svg>
                          )}
                          <div className="flex-1 min-w-0">
                            <div className="flex items-center gap-2 mb-0.5">
                              <span
                                className={`text-xs font-medium ${result.success
                                  ? "text-success-700 dark:text-success-300"
                                  : "text-danger-700 dark:text-danger-300"
                                  }`}
                              >
                                {result.success ? "成功" : "失败"}
                              </span>
                              <span className="text-xs text-default-500">
                                |
                              </span>
                              <code className="text-xs font-mono text-default-600 truncate">
                                {result.line}
                              </code>
                            </div>
                            <div
                              className={`text-xs ${result.success
                                ? "text-success-600 dark:text-success-400"
                                : "text-danger-600 dark:text-danger-400"
                                }`}
                            >
                              {result.message}
                            </div>
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </ModalBody>
          <ModalFooter>
            <Button variant="light" onPress={() => setImportModalOpen(false)}>
              关闭
            </Button>
            <Button
              color="warning"
              isDisabled={!importData.trim() || !selectedTunnelForImport}
              isLoading={importLoading}
              onPress={executeImport}
            >
              开始导入
            </Button>
          </ModalFooter>
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
                <h2 className="text-xl font-bold">转发诊断结果</h2>
                {currentDiagnosisForward && (
                  <div className="flex items-center gap-2 min-w-0">
                    <span className="text-small text-default-500 truncate flex-1 min-w-0">
                      {currentDiagnosisForward.name}
                    </span>
                    <Chip
                      className="flex-shrink-0"
                      color="primary"
                      size="sm"
                      variant="flat"
                    >
                      转发服务
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
                                    const quality =
                                      getForwardDiagnosisQualityDisplay(
                                        result.averageTime,
                                        result.packetLoss,
                                      );

                                    return (
                                      <tr
                                        key={index}
                                        className={`hover:bg-default-50 dark:hover:bg-gray-700/50 ${result.success
                                          ? "bg-white dark:bg-gray-800"
                                          : "bg-danger-50 dark:bg-danger-900/30"
                                          }`}
                                      >
                                        <td className="px-3 py-2">
                                          <div className="flex items-center gap-2">
                                            <span
                                              className={`w-5 h-5 rounded-full flex items-center justify-center text-xs ${result.success
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
                                              className={`font-semibold ${(result.packetLoss || 0) > 0
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
                                const quality =
                                  getForwardDiagnosisQualityDisplay(
                                    result.averageTime,
                                    result.packetLoss,
                                  );

                                return (
                                  <div
                                    key={index}
                                    className={`border rounded-lg p-3 ${result.success
                                      ? "border-divider bg-white dark:bg-gray-800"
                                      : "border-danger-200 dark:border-danger-300/30 bg-danger-50 dark:bg-danger-900/30"
                                      }`}
                                  >
                                    <div className="flex items-start gap-2 mb-2">
                                      <span
                                        className={`w-6 h-6 rounded-full flex items-center justify-center text-xs flex-shrink-0 ${result.success
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
                                            className={`text-lg font-bold ${(result.packetLoss || 0) > 0
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
              <ModalFooter className="bg-content1 border-t border-divider">
                <Button variant="light" onPress={onClose}>
                  关闭
                </Button>
                {currentDiagnosisForward && (
                  <Button
                    color="primary"
                    isLoading={diagnosisLoading}
                    onPress={() => handleDiagnose(currentDiagnosisForward)}
                  >
                    重新诊断
                  </Button>
                )}
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {/* 批量删除确认模态框 */}
      <Modal
        isOpen={batchDeleteModalOpen}
        onOpenChange={setBatchDeleteModalOpen}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader>确认删除</ModalHeader>
              <ModalBody>
                <p>
                  确定要删除选中的 {selectedIds.size} 项转发吗？此操作不可撤销。
                </p>
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  取消
                </Button>
                <Button
                  color="danger"
                  isLoading={batchLoading}
                  onPress={handleBatchDelete}
                >
                  确认删除
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {/* 批量换隧道模态框 */}
      <Modal
        isOpen={batchChangeTunnelModalOpen}
        onOpenChange={setBatchChangeTunnelModalOpen}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader>隧道</ModalHeader>
              <ModalBody>
                <p className="mb-4">
                  将选中的 {selectedIds.size} 项转发迁移到新隧道：
                </p>
                <Select
                  label="目标隧道"
                  placeholder="请选择目标隧道"
                  selectedKeys={
                    batchTargetTunnelId ? [String(batchTargetTunnelId)] : []
                  }
                  onSelectionChange={(keys) => {
                    const selected = Array.from(keys)[0];

                    setBatchTargetTunnelId(selected ? Number(selected) : null);
                  }}
                >
                  {tunnels.map((tunnel) => (
                    <SelectItem key={String(tunnel.id)}>
                      {tunnel.name}
                    </SelectItem>
                  ))}
                </Select>
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  取消
                </Button>
                <Button
                  color="primary"
                  isDisabled={!batchTargetTunnelId}
                  isLoading={batchLoading}
                  onPress={handleBatchChangeTunnel}
                >
                  确认换隧道
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </div>
  );
}
