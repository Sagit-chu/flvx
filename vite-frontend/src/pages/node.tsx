import { useState, useEffect, useMemo, useRef } from "react";
import { Card, CardBody, CardHeader } from "@heroui/card";
import { Button } from "@heroui/button";
import { Input } from "@heroui/input";
import { Textarea } from "@heroui/input";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
} from "@heroui/modal";
import { Chip } from "@heroui/chip";
import { Switch } from "@heroui/switch";
import { Spinner } from "@heroui/spinner";
import { Alert } from "@heroui/alert";
import { Progress } from "@heroui/progress";
import { Accordion, AccordionItem } from "@heroui/accordion";
import { Checkbox } from "@heroui/checkbox";
import toast from "react-hot-toast";
import axios from "axios";
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
  createNode,
  getNodeList,
  updateNode,
  deleteNode,
  getNodeInstallCommand,
  updateNodeOrder,
  batchDeleteNodes,
} from "@/api";

interface Node {
  id: number;
  inx?: number;
  name: string;
  ip: string;
  serverIp: string;
  serverIpV4?: string;
  serverIpV6?: string;
  port: string;
  tcpListenAddr?: string;
  udpListenAddr?: string;
  version?: string;
  http?: number; // 0 关 1 开
  tls?: number; // 0 关 1 开
  socks?: number; // 0 关 1 开
  status: number;
  isRemote?: number;
  remoteUrl?: string;
  connectionStatus: "online" | "offline";
  systemInfo?: {
    cpuUsage: number;
    memoryUsage: number;
    uploadTraffic: number;
    downloadTraffic: number;
    uploadSpeed: number;
    downloadSpeed: number;
    uptime: number;
  } | null;
  copyLoading?: boolean;
}

interface NodeForm {
  id: number | null;
  name: string;
  serverHost: string;
  serverIpV4: string;
  serverIpV6: string;
  port: string;
  tcpListenAddr: string;
  udpListenAddr: string;
  interfaceName: string;
  http: number; // 0 关 1 开
  tls: number; // 0 关 1 开
  socks: number; // 0 关 1 开
}

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

export default function NodePage() {
  const [nodeList, setNodeList] = useState<Node[]>([]);
  const [nodeOrder, setNodeOrder] = useState<number[]>([]);
  const [loading, setLoading] = useState(false);
  const [wsConnected, setWsConnected] = useState(false);
  const [wsConnecting, setWsConnecting] = useState(false);
  const [dialogVisible, setDialogVisible] = useState(false);
  const [dialogTitle, setDialogTitle] = useState("");
  const [isEdit, setIsEdit] = useState(false);
  const [submitLoading, setSubmitLoading] = useState(false);
  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [nodeToDelete, setNodeToDelete] = useState<Node | null>(null);
  const [protocolDisabled, setProtocolDisabled] = useState(false);
  const [protocolDisabledReason, setProtocolDisabledReason] = useState("");
  const [form, setForm] = useState<NodeForm>({
    id: null,
    name: "",
    serverHost: "",
    serverIpV4: "",
    serverIpV6: "",
    port: "1000-65535",
    tcpListenAddr: "[::]",
    udpListenAddr: "[::]",
    interfaceName: "",
    http: 0,
    tls: 0,
    socks: 0,
  });
  const [errors, setErrors] = useState<Record<string, string>>({});

  const [selectMode, setSelectMode] = useState(false);
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  const [batchDeleteModalOpen, setBatchDeleteModalOpen] = useState(false);
  const [batchLoading, setBatchLoading] = useState(false);

  // 安装命令相关状态
  const [installCommandModal, setInstallCommandModal] = useState(false);
  const [installCommand, setInstallCommand] = useState("");
  const [currentNodeName, setCurrentNodeName] = useState("");

  const websocketRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<NodeJS.Timeout | null>(null);
  const reconnectAttemptsRef = useRef(0);
  const maxReconnectAttempts = 5;
  const offlineTimersRef = useRef<Map<number, ReturnType<typeof setTimeout>>>(
    new Map(),
  );
  const offlineDelayMs = 3000;

  const clearOfflineTimer = (nodeId: number) => {
    const timer = offlineTimersRef.current.get(nodeId);

    if (timer) {
      clearTimeout(timer);
      offlineTimersRef.current.delete(nodeId);
    }
  };

  const scheduleNodeOffline = (nodeId: number) => {
    if (offlineTimersRef.current.has(nodeId)) return;
    const timer = setTimeout(() => {
      offlineTimersRef.current.delete(nodeId);
      setNodeList((prev) =>
        prev.map((node) => {
          if (node.id !== nodeId) return node;
          if (node.connectionStatus === "offline" && node.systemInfo === null)
            return node;

          return { ...node, connectionStatus: "offline", systemInfo: null };
        }),
      );
    }, offlineDelayMs);

    offlineTimersRef.current.set(nodeId, timer);
  };

  useEffect(() => {
    loadNodes();
    initWebSocket();

    return () => {
      closeWebSocket();
    };
  }, []);

  // 加载节点列表
  const loadNodes = async () => {
    setLoading(true);
    try {
      const res = await getNodeList();

      if (res.code === 0) {
        const nodesData: Node[] = (res.data || []).map((node: any) => ({
          ...node,
          inx: node.inx ?? 0,
          connectionStatus: node.status === 1 ? "online" : "offline",
          systemInfo: null,
          copyLoading: false,
        }));

        setNodeList(nodesData);

        // 优先使用数据库中的 inx 字段进行排序，否则回退到本地排序
        const hasDbOrdering = nodesData.some(
          (n) => n.inx !== undefined && n.inx !== 0,
        );

        if (hasDbOrdering) {
          const dbOrder = [...nodesData]
            .sort((a, b) => (a.inx ?? 0) - (b.inx ?? 0))
            .map((n) => n.id);

          setNodeOrder(dbOrder);
        } else {
          try {
            const stored = localStorage.getItem("node-order");

            if (stored) {
              const parsed = JSON.parse(stored);

              if (Array.isArray(parsed)) {
                const existingIds = new Set(nodesData.map((n) => n.id));
                const validOrder = parsed
                  .map((id: any) => Number(id))
                  .filter((id: number) => existingIds.has(id));

                if (validOrder.length > 0) {
                  setNodeOrder(validOrder);
                } else {
                  setNodeOrder(nodesData.map((n) => n.id));
                }
              } else {
                setNodeOrder(nodesData.map((n) => n.id));
              }
            } else {
              setNodeOrder(nodesData.map((n) => n.id));
            }
          } catch {
            setNodeOrder(nodesData.map((n) => n.id));
          }
        }
      } else {
        toast.error(res.msg || "加载节点列表失败");
      }
    } catch {
      toast.error("网络错误，请重试");
    } finally {
      setLoading(false);
    }
  };

  // 初始化WebSocket连接
  const initWebSocket = () => {
    if (
      websocketRef.current &&
      (websocketRef.current.readyState === WebSocket.OPEN ||
        websocketRef.current.readyState === WebSocket.CONNECTING)
    ) {
      return;
    }

    if (websocketRef.current) {
      closeWebSocket();
    }

    // 构建WebSocket URL，使用axios的baseURL
    const baseUrl =
      axios.defaults.baseURL ||
      (import.meta.env.VITE_API_BASE
        ? `${import.meta.env.VITE_API_BASE}/api/v1/`
        : "/api/v1/");
    const wsUrl =
      baseUrl.replace(/^http/, "ws").replace(/\/api\/v1\/$/, "") +
      `/system-info?type=0&secret=${localStorage.getItem("token")}`;

    try {
      setWsConnecting(true);
      websocketRef.current = new WebSocket(wsUrl);

      websocketRef.current.onopen = () => {
        reconnectAttemptsRef.current = 0;
        setWsConnected(true);
        setWsConnecting(false);
      };

      websocketRef.current.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);

          handleWebSocketMessage(data);
        } catch {
          // 解析失败时不输出错误信息
        }
      };

      websocketRef.current.onerror = () => {
        // WebSocket错误时不输出错误信息
      };

      websocketRef.current.onclose = () => {
        websocketRef.current = null;
        setWsConnected(false);
        setWsConnecting(false);
        attemptReconnect();
      };
    } catch {
      setWsConnected(false);
      setWsConnecting(false);
      attemptReconnect();
    }
  };

  // 处理WebSocket消息
  const handleWebSocketMessage = (data: any) => {
    const { id, type, data: messageData } = data;
    const nodeId = Number(id);

    if (Number.isNaN(nodeId)) return;

    if (type === "status") {
      if (messageData === 1) {
        clearOfflineTimer(nodeId);
        setNodeList((prev) =>
          prev.map((node) => {
            if (node.id !== nodeId) return node;
            if (node.connectionStatus === "online") return node;

            return { ...node, connectionStatus: "online" };
          }),
        );
      } else {
        // 离线事件做延迟处理，避免短抖动导致频繁闪烁
        scheduleNodeOffline(nodeId);
      }
    } else if (type === "info") {
      clearOfflineTimer(nodeId);
      setNodeList((prev) =>
        prev.map((node) => {
          if (node.id === nodeId) {
            try {
              let systemInfo;

              if (typeof messageData === "string") {
                systemInfo = JSON.parse(messageData);
              } else {
                systemInfo = messageData;
              }

              const currentUpload = parseInt(systemInfo.bytes_transmitted) || 0;
              const currentDownload = parseInt(systemInfo.bytes_received) || 0;
              const currentUptime = parseInt(systemInfo.uptime) || 0;

              let uploadSpeed = 0;
              let downloadSpeed = 0;

              if (node.systemInfo && node.systemInfo.uptime) {
                const timeDiff = currentUptime - node.systemInfo.uptime;

                if (timeDiff > 0 && timeDiff <= 10) {
                  const lastUpload = node.systemInfo.uploadTraffic || 0;
                  const lastDownload = node.systemInfo.downloadTraffic || 0;

                  const uploadDiff = currentUpload - lastUpload;
                  const downloadDiff = currentDownload - lastDownload;

                  const uploadReset = currentUpload < lastUpload;
                  const downloadReset = currentDownload < lastDownload;

                  if (!uploadReset && uploadDiff >= 0) {
                    uploadSpeed = uploadDiff / timeDiff;
                  }

                  if (!downloadReset && downloadDiff >= 0) {
                    downloadSpeed = downloadDiff / timeDiff;
                  }
                }
              }

              return {
                ...node,
                connectionStatus: "online",
                systemInfo: {
                  cpuUsage: parseFloat(systemInfo.cpu_usage) || 0,
                  memoryUsage: parseFloat(systemInfo.memory_usage) || 0,
                  uploadTraffic: currentUpload,
                  downloadTraffic: currentDownload,
                  uploadSpeed: uploadSpeed,
                  downloadSpeed: downloadSpeed,
                  uptime: currentUptime,
                },
              };
            } catch {
              return node;
            }
          }

          return node;
        }),
      );
    }
  };

  // 尝试重新连接
  const attemptReconnect = () => {
    if (reconnectTimerRef.current) return;
    if (reconnectAttemptsRef.current < maxReconnectAttempts) {
      reconnectAttemptsRef.current++;

      reconnectTimerRef.current = setTimeout(() => {
        reconnectTimerRef.current = null;
        initWebSocket();
      }, 3000 * reconnectAttemptsRef.current);
    }
  };

  // 关闭WebSocket连接
  const closeWebSocket = () => {
    if (reconnectTimerRef.current) {
      clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }

    offlineTimersRef.current.forEach((timer) => clearTimeout(timer));
    offlineTimersRef.current.clear();

    reconnectAttemptsRef.current = 0;
    setWsConnected(false);
    setWsConnecting(false);

    if (websocketRef.current) {
      websocketRef.current.onopen = null;
      websocketRef.current.onmessage = null;
      websocketRef.current.onerror = null;
      websocketRef.current.onclose = null;

      if (
        websocketRef.current.readyState === WebSocket.OPEN ||
        websocketRef.current.readyState === WebSocket.CONNECTING
      ) {
        websocketRef.current.close();
      }

      websocketRef.current = null;
    }
  };

  // 格式化速度
  const formatSpeed = (bytesPerSecond: number): string => {
    if (bytesPerSecond === 0) return "0 B/s";

    const k = 1024;
    const sizes = ["B/s", "KB/s", "MB/s", "GB/s", "TB/s"];
    const i = Math.floor(Math.log(bytesPerSecond) / Math.log(k));

    return (
      parseFloat((bytesPerSecond / Math.pow(k, i)).toFixed(2)) + " " + sizes[i]
    );
  };

  // 格式化开机时间
  const formatUptime = (seconds: number): string => {
    if (seconds === 0) return "-";

    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);

    if (days > 0) {
      return `${days}天${hours}小时`;
    } else if (hours > 0) {
      return `${hours}小时${minutes}分钟`;
    } else {
      return `${minutes}分钟`;
    }
  };

  // 格式化流量
  const formatTraffic = (bytes: number): string => {
    if (bytes === 0) return "0 B";

    const k = 1024;
    const sizes = ["B", "KB", "MB", "GB", "TB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));

    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
  };

  // 获取进度条颜色
  const getProgressColor = (
    value: number,
    offline = false,
  ): "default" | "primary" | "secondary" | "success" | "warning" | "danger" => {
    if (offline) return "default";
    if (value <= 50) return "success";
    if (value <= 80) return "warning";

    return "danger";
  };

  // IPv4/IPv6 格式验证（仅用于判定地址族）
  const ipv4Regex =
    /^(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$/;
  const ipv6Regex =
    /^(([0-9a-fA-F]{1,4}:){7,7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,7}:|([0-9a-fA-F]{1,4}:){1,6}:[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:){1,5}(:[0-9a-fA-F]{1,4}){1,2}|([0-9a-fA-F]{1,4}:){1,4}(:[0-9a-fA-F]{1,4}){1,3}|([0-9a-fA-F]{1,4}:){1,3}(:[0-9a-fA-F]{1,4}){1,4}|([0-9a-fA-F]{1,4}:){1,2}(:[0-9a-fA-F]{1,4}){1,5}|[0-9a-fA-F]{1,4}:((:[0-9a-fA-F]{1,4}){1,6})|:((:[0-9a-fA-F]{1,4}){1,7}|:)|fe80:(:[0-9a-fA-F]{0,4}){0,4}%[0-9a-zA-Z]{1,}|::(ffff(:0{1,4}){0,1}:){0,1}((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])|([0-9a-fA-F]{1,4}:){1,4}:((25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9])\.){3,3}(25[0-5]|(2[0-4]|1{0,1}[0-9]){0,1}[0-9]))$/;

  const validateIpv4Literal = (ip: string): boolean =>
    ipv4Regex.test(ip.trim());
  const validateIpv6Literal = (ip: string): boolean =>
    ipv6Regex.test(ip.trim());

  // Hostname/domain validation (no scheme/port)
  const hostnameRegex =
    /^(?=.{1,253}$)(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)(?:\.(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?))*$/;
  const validateHostname = (host: string): boolean => {
    const v = host.trim();

    if (!v) return false;
    if (v === "localhost") return true;

    return hostnameRegex.test(v);
  };

  // 验证端口格式：支持 80,443,100-600
  const validatePort = (
    portStr: string,
  ): { valid: boolean; error?: string } => {
    if (!portStr || !portStr.trim()) {
      return { valid: false, error: "请输入端口" };
    }

    const trimmed = portStr.trim();
    const parts = trimmed
      .split(",")
      .map((p) => p.trim())
      .filter((p) => p);

    if (parts.length === 0) {
      return { valid: false, error: "请输入有效的端口" };
    }

    for (const part of parts) {
      // 检查是否是端口范围 (如 100-600)
      if (part.includes("-")) {
        const range = part.split("-").map((p) => p.trim());

        if (range.length !== 2) {
          return { valid: false, error: `端口范围格式错误: ${part}` };
        }

        const start = parseInt(range[0]);
        const end = parseInt(range[1]);

        if (isNaN(start) || isNaN(end)) {
          return { valid: false, error: `端口必须是数字: ${part}` };
        }

        if (start < 1 || start > 65535 || end < 1 || end > 65535) {
          return {
            valid: false,
            error: `端口范围必须在 1-65535 之间: ${part}`,
          };
        }

        if (start >= end) {
          return { valid: false, error: `起始端口必须小于结束端口: ${part}` };
        }
      } else {
        // 单个端口
        const port = parseInt(part);

        if (isNaN(port)) {
          return { valid: false, error: `端口必须是数字: ${part}` };
        }

        if (port < 1 || port > 65535) {
          return { valid: false, error: `端口必须在 1-65535 之间: ${part}` };
        }
      }
    }

    return { valid: true };
  };

  // 表单验证
  const validateForm = (): boolean => {
    const newErrors: Record<string, string> = {};

    if (!form.name.trim()) {
      newErrors.name = "请输入节点名称";
    } else if (form.name.trim().length < 2) {
      newErrors.name = "节点名称长度至少2位";
    } else if (form.name.trim().length > 50) {
      newErrors.name = "节点名称长度不能超过50位";
    }

    const v4 = form.serverIpV4.trim();
    const v6 = form.serverIpV6.trim();
    const host = form.serverHost.trim();

    if (!v4 && !v6 && !host) {
      const msg = "请至少填写一个 IPv4/IPv6 地址或域名";

      newErrors.serverIpV4 = msg;
      newErrors.serverIpV6 = msg;
      newErrors.serverHost = msg;
    } else {
      if (v4 && !validateIpv4Literal(v4)) {
        newErrors.serverIpV4 = "请输入有效的IPv4地址";
      }
      if (v6 && !validateIpv6Literal(v6)) {
        newErrors.serverIpV6 = "请输入有效的IPv6地址";
      }
      if (host && !validateHostname(host)) {
        newErrors.serverHost = "请输入有效的域名/主机名";
      }
    }

    const portValidation = validatePort(form.port);

    if (!portValidation.valid) {
      newErrors.port = portValidation.error || "端口格式错误";
    }

    setErrors(newErrors);

    return Object.keys(newErrors).length === 0;
  };

  // 新增节点
  const handleAdd = () => {
    setDialogTitle("新增节点");
    setIsEdit(false);
    setDialogVisible(true);
    resetForm();
    setProtocolDisabled(true);
    setProtocolDisabledReason("节点未在线，等待节点上线后再设置");
  };

  // 编辑节点
  const handleEdit = (node: Node) => {
    setDialogTitle("编辑节点");
    setIsEdit(true);

    const legacy = (node.serverIp || "").trim();
    const normalizedV4 =
      node.serverIpV4?.trim() || (validateIpv4Literal(legacy) ? legacy : "");
    const normalizedV6 =
      node.serverIpV6?.trim() || (validateIpv6Literal(legacy) ? legacy : "");
    const normalizedHost =
      !normalizedV4 && !normalizedV6 && legacy ? legacy : "";

    setForm({
      id: node.id,
      name: node.name,
      serverHost: normalizedHost,
      serverIpV4: normalizedV4,
      serverIpV6: normalizedV6,
      port: node.port || "1000-65535",
      tcpListenAddr: node.tcpListenAddr || "[::]",
      udpListenAddr: node.udpListenAddr || "[::]",
      interfaceName: (node as any).interfaceName || "",
      http: typeof node.http === "number" ? node.http : 1,
      tls: typeof node.tls === "number" ? node.tls : 1,
      socks: typeof node.socks === "number" ? node.socks : 1,
    });
    const offline = node.connectionStatus !== "online";

    setProtocolDisabled(offline);
    setProtocolDisabledReason(
      offline ? "节点未在线，等待节点上线后再设置" : "",
    );
    setDialogVisible(true);
  };

  // 删除节点
  const handleDelete = (node: Node) => {
    setNodeToDelete(node);
    setDeleteModalOpen(true);
  };

  const confirmDelete = async () => {
    if (!nodeToDelete) return;

    setDeleteLoading(true);
    try {
      const res = await deleteNode(nodeToDelete.id);

      if (res.code === 0) {
        toast.success("删除成功");
        setNodeList((prev) => prev.filter((n) => n.id !== nodeToDelete.id));
        setDeleteModalOpen(false);
        setNodeToDelete(null);
      } else {
        toast.error(res.msg || "删除失败");
      }
    } catch {
      toast.error("网络错误，请重试");
    } finally {
      setDeleteLoading(false);
    }
  };

  // 复制安装命令
  const handleCopyInstallCommand = async (node: Node) => {
    setNodeList((prev) =>
      prev.map((n) => (n.id === node.id ? { ...n, copyLoading: true } : n)),
    );

    try {
      const res = await getNodeInstallCommand(node.id);

      if (res.code === 0 && res.data) {
        try {
          await navigator.clipboard.writeText(res.data);
          toast.success("安装命令已复制到剪贴板");
        } catch {
          // 复制失败，显示安装命令模态框
          setInstallCommand(res.data);
          setCurrentNodeName(node.name);
          setInstallCommandModal(true);
        }
      } else {
        toast.error(res.msg || "获取安装命令失败");
      }
    } catch {
      toast.error("获取安装命令失败");
    } finally {
      setNodeList((prev) =>
        prev.map((n) => (n.id === node.id ? { ...n, copyLoading: false } : n)),
      );
    }
  };

  // 手动复制安装命令
  const handleManualCopy = async () => {
    try {
      await navigator.clipboard.writeText(installCommand);
      toast.success("安装命令已复制到剪贴板");
      setInstallCommandModal(false);
    } catch {
      toast.error("复制失败，请手动选择文本复制");
    }
  };

  // 提交表单
  const handleSubmit = async () => {
    if (!validateForm()) return;

    setSubmitLoading(true);

    try {
      const apiCall = isEdit ? updateNode : createNode;
      const { serverHost, ...rest } = form;
      const data = {
        ...rest,
        serverIp:
          form.serverIpV4?.trim() ||
          form.serverIpV6?.trim() ||
          serverHost?.trim() ||
          "",
      };

      const res = await apiCall(data);

      if (res.code === 0) {
        toast.success(isEdit ? "更新成功" : "创建成功");
        setDialogVisible(false);

        if (isEdit) {
          setNodeList((prev) =>
            prev.map((n) =>
              n.id === form.id
                ? {
                    ...n,
                    name: form.name,
                    serverIp:
                      form.serverIpV4?.trim() ||
                      form.serverIpV6?.trim() ||
                      form.serverHost?.trim() ||
                      "",
                    serverIpV4: form.serverIpV4,
                    serverIpV6: form.serverIpV6,
                    port: form.port,
                    tcpListenAddr: form.tcpListenAddr,
                    udpListenAddr: form.udpListenAddr,
                    interfaceName: form.interfaceName,
                    http: form.http,
                    tls: form.tls,
                    socks: form.socks,
                  }
                : n,
            ),
          );
        } else {
          loadNodes();
        }
      } else {
        toast.error(res.msg || (isEdit ? "更新失败" : "创建失败"));
      }
    } catch {
      toast.error("网络错误，请重试");
    } finally {
      setSubmitLoading(false);
    }
  };

  // 重置表单
  const resetForm = () => {
    setForm({
      id: null,
      name: "",
      serverHost: "",
      serverIpV4: "",
      serverIpV6: "",
      port: "1000-65535",
      tcpListenAddr: "[::]",
      udpListenAddr: "[::]",
      interfaceName: "",
      http: 0,
      tls: 0,
      socks: 0,
    });
    setErrors({});
  };

  // 处理拖拽结束
  const handleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event;

    if (!active || !over || active.id === over.id) return;
    if (!nodeOrder || nodeOrder.length === 0) return;

    const activeId = Number(active.id);
    const overId = Number(over.id);

    if (isNaN(activeId) || isNaN(overId)) return;

    const oldIndex = nodeOrder.indexOf(activeId);
    const newIndex = nodeOrder.indexOf(overId);

    if (oldIndex === -1 || newIndex === -1 || oldIndex === newIndex) return;

    const newOrder = arrayMove(nodeOrder, oldIndex, newIndex);

    setNodeOrder(newOrder);

    // 保存到 localStorage
    try {
      localStorage.setItem("node-order", JSON.stringify(newOrder));
    } catch {}

    // 持久化到数据库
    try {
      const nodesToUpdate = newOrder.map((id, index) => ({ id, inx: index }));
      const response = await updateNodeOrder({ nodes: nodesToUpdate });

      if (response.code === 0) {
        setNodeList((prev) =>
          prev.map((node) => {
            const updated = nodesToUpdate.find((n) => n.id === node.id);

            return updated ? { ...node, inx: updated.inx } : node;
          }),
        );
      } else {
        toast.error("保存排序失败：" + (response.msg || "未知错误"));
      }
    } catch {
      toast.error("保存排序失败，请重试");
    }
  };

  // 批量操作处理函数
  const toggleSelectMode = () => {
    setSelectMode((prev) => {
      if (prev) {
        setSelectedIds(new Set());
      }

      return !prev;
    });
  };

  const toggleSelect = (id: number) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);

      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }

      return next;
    });
  };

  const selectAll = () => {
    setSelectedIds(new Set(sortedNodes.map((n) => n.id)));
  };

  const deselectAll = () => {
    setSelectedIds(new Set());
  };

  const handleBatchDelete = async () => {
    if (selectedIds.size === 0) return;
    setBatchLoading(true);
    try {
      const res = await batchDeleteNodes(Array.from(selectedIds));

      if (res.code === 0) {
        toast.success(`成功删除 ${selectedIds.size} 个节点`);
        setNodeList((prev) => prev.filter((n) => !selectedIds.has(n.id)));
        setSelectedIds(new Set());
        setBatchDeleteModalOpen(false);
        setSelectMode(false);
      } else {
        toast.error(res.msg || "删除失败");
      }
    } catch {
      toast.error("网络错误，请重试");
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

  // 根据排序顺序获取节点列表
  const sortedNodes = useMemo((): Node[] => {
    if (!nodeList || nodeList.length === 0) return [];

    const sortedByDb = [...nodeList].sort((a, b) => {
      const aInx = a.inx ?? 0;
      const bInx = b.inx ?? 0;

      return aInx - bInx;
    });

    // 如果数据库中没有排序信息，则使用本地存储的顺序
    if (
      nodeOrder &&
      nodeOrder.length > 0 &&
      sortedByDb.every((n) => n.inx === undefined || n.inx === 0)
    ) {
      const nodeMap = new Map(nodeList.map((n) => [n.id, n] as const));
      const localSorted: Node[] = [];

      nodeOrder.forEach((id) => {
        const node = nodeMap.get(id);

        if (node) localSorted.push(node);
      });

      nodeList.forEach((node) => {
        if (!nodeOrder.includes(node.id)) {
          localSorted.push(node);
        }
      });

      return localSorted;
    }

    return sortedByDb;
  }, [nodeList, nodeOrder]);

  const sortableNodeIds = useMemo(
    () => sortedNodes.map((n) => n.id),
    [sortedNodes],
  );

  return (
    <div className="px-3 lg:px-6 py-8">
      {/* 页面头部 */}
      <div className="flex items-center justify-between mb-6">
        <div className="flex-1" />

        <div className="flex gap-2 items-center">
          <Button
            color={selectMode ? "warning" : "default"}
            size="sm"
            variant="flat"
            onPress={toggleSelectMode}
          >
            {selectMode ? "取消多选" : "多选"}
          </Button>
          <Button color="primary" size="sm" variant="flat" onPress={handleAdd}>
            新增
          </Button>
        </div>
      </div>

      {/* 批量操作浮动工具栏 */}
      {selectMode && selectedIds.size > 0 && (
        <div className="fixed bottom-7 left-1/2 z-50 w-[calc(100vw-1rem)] max-w-max -translate-x-1/2 overflow-x-auto rounded-lg border border-divider bg-content1 p-2 shadow-lg">
          <div className="flex min-w-max items-center gap-2">
            <span className="text-sm font-medium shrink-0">
              已选 {selectedIds.size} 项
            </span>
            <Button size="sm" variant="flat" onPress={selectAll}>
              全选
            </Button>
            <Button size="sm" variant="flat" onPress={deselectAll}>
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
          </div>
        </div>
      )}

      {!wsConnected && (
        <Alert
          className="mb-4"
          color="warning"
          description={
            wsConnecting ? "监控连接中..." : "监控连接已断开，正在重连..."
          }
          variant="flat"
        />
      )}

      {/* 节点列表 */}
      {loading ? (
        <div className="flex items-center justify-center h-64">
          <div className="flex items-center gap-3">
            <Spinner size="sm" />
            <span className="text-default-600">正在加载...</span>
          </div>
        </div>
      ) : nodeList.length === 0 ? (
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
                    d="M5 12h14M5 12l4-4m-4 4l4 4"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={1.5}
                  />
                </svg>
              </div>
              <div>
                <h3 className="text-lg font-semibold text-foreground">
                  暂无节点配置
                </h3>
                <p className="text-default-500 text-sm mt-1">
                  还没有创建任何节点配置，点击上方按钮开始创建
                </p>
              </div>
            </div>
          </CardBody>
        </Card>
      ) : (
        <DndContext sensors={sensors} onDragEnd={handleDragEnd}>
          <SortableContext
            items={sortableNodeIds}
            strategy={rectSortingStrategy}
          >
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 gap-4">
              {sortedNodes.map((node) => (
                <SortableItem key={node.id} id={node.id}>
                  {(listeners) => (
                    <Card
                      key={node.id}
                      className="group shadow-sm border border-divider hover:shadow-md transition-shadow duration-200"
                    >
                      <CardHeader className="pb-2">
                        <div className="flex justify-between items-start w-full">
                          <div className="flex items-center gap-2 flex-1 min-w-0">
                            {selectMode && (
                              <Checkbox
                                isSelected={selectedIds.has(node.id)}
                                onValueChange={() => toggleSelect(node.id)}
                              />
                            )}
                            <h3 className="font-semibold text-foreground truncate text-sm">
                              {node.name}
                            </h3>
                          </div>
                          <div className="flex items-center gap-1.5 ml-2">
                            {node.isRemote === 1 && (
                              <Chip
                                className="text-xs"
                                color="secondary"
                                size="sm"
                                variant="flat"
                              >
                                远程
                              </Chip>
                            )}
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
                            <Chip
                              className="text-xs"
                              color={
                                node.connectionStatus === "online"
                                  ? "success"
                                  : "danger"
                              }
                              size="sm"
                              variant="flat"
                            >
                              {node.connectionStatus === "online"
                                ? "在线"
                                : "离线"}
                            </Chip>
                          </div>
                        </div>
                      </CardHeader>

                      <CardBody className="pt-0 pb-3">
                        {/* 基础信息 */}
                        <div className="space-y-2 mb-4">
                          <div className="flex justify-between items-center text-sm min-w-0">
                            <span className="text-default-600 flex-shrink-0">
                              IP
                            </span>
                            <div className="text-right text-xs min-w-0 flex-1 ml-2">
                              {node.serverIpV4?.trim() ||
                              node.serverIpV6?.trim() ? (
                                <div className="space-y-0.5">
                                  {node.serverIpV4?.trim() && (
                                    <span
                                      className="font-mono truncate block"
                                      title={node.serverIpV4.trim()}
                                    >
                                      {node.serverIpV4.trim()}
                                    </span>
                                  )}
                                  {node.serverIpV6?.trim() && (
                                    <span
                                      className="font-mono truncate block"
                                      title={node.serverIpV6.trim()}
                                    >
                                      {node.serverIpV6.trim()}
                                    </span>
                                  )}
                                </div>
                              ) : (
                                <span
                                  className="font-mono truncate block"
                                  title={node.serverIp.trim()}
                                >
                                  {node.serverIp.trim()}
                                </span>
                              )}
                            </div>
                          </div>
                          <div className="flex justify-between text-sm">
                            <span className="text-default-600">版本</span>
                            <span className="text-xs">
                              {node.version || "未知"}
                            </span>
                          </div>
                          <div className="flex justify-between text-sm">
                            <span className="text-default-600">开机时间</span>
                            <span className="text-xs">
                              {node.connectionStatus === "online" &&
                              node.systemInfo
                                ? formatUptime(node.systemInfo.uptime)
                                : "-"}
                            </span>
                          </div>
                        </div>

                        {/* 系统监控 */}
                        <div className="space-y-3 mb-4">
                          <div className="grid grid-cols-2 gap-3">
                            <div>
                              <div className="flex justify-between text-xs mb-1">
                                <span>CPU</span>
                                <span className="font-mono">
                                  {node.connectionStatus === "online" &&
                                  node.systemInfo
                                    ? `${node.systemInfo.cpuUsage.toFixed(1)}%`
                                    : "-"}
                                </span>
                              </div>
                              <Progress
                                aria-label="CPU使用率"
                                color={getProgressColor(
                                  node.connectionStatus === "online" &&
                                    node.systemInfo
                                    ? node.systemInfo.cpuUsage
                                    : 0,
                                  node.connectionStatus !== "online",
                                )}
                                size="sm"
                                value={
                                  node.connectionStatus === "online" &&
                                  node.systemInfo
                                    ? node.systemInfo.cpuUsage
                                    : 0
                                }
                              />
                            </div>
                            <div>
                              <div className="flex justify-between text-xs mb-1">
                                <span>内存</span>
                                <span className="font-mono">
                                  {node.connectionStatus === "online" &&
                                  node.systemInfo
                                    ? `${node.systemInfo.memoryUsage.toFixed(1)}%`
                                    : "-"}
                                </span>
                              </div>
                              <Progress
                                aria-label="内存使用率"
                                color={getProgressColor(
                                  node.connectionStatus === "online" &&
                                    node.systemInfo
                                    ? node.systemInfo.memoryUsage
                                    : 0,
                                  node.connectionStatus !== "online",
                                )}
                                size="sm"
                                value={
                                  node.connectionStatus === "online" &&
                                  node.systemInfo
                                    ? node.systemInfo.memoryUsage
                                    : 0
                                }
                              />
                            </div>
                          </div>

                          <div className="grid grid-cols-2 gap-2 text-xs">
                            <div className="text-center p-2 bg-default-50 dark:bg-default-100 rounded">
                              <div className="text-default-600 mb-0.5">
                                上传
                              </div>
                              <div className="font-mono">
                                {node.connectionStatus === "online" &&
                                node.systemInfo
                                  ? formatSpeed(node.systemInfo.uploadSpeed)
                                  : "-"}
                              </div>
                            </div>
                            <div className="text-center p-2 bg-default-50 dark:bg-default-100 rounded">
                              <div className="text-default-600 mb-0.5">
                                下载
                              </div>
                              <div className="font-mono">
                                {node.connectionStatus === "online" &&
                                node.systemInfo
                                  ? formatSpeed(node.systemInfo.downloadSpeed)
                                  : "-"}
                              </div>
                            </div>
                          </div>

                          {/* 流量统计 */}
                          <div className="grid grid-cols-2 gap-2 text-xs">
                            <div className="text-center p-2 bg-primary-50 dark:bg-primary-100/20 rounded border border-primary-200 dark:border-primary-300/20">
                              <div className="text-primary-600 dark:text-primary-400 mb-0.5">
                                ↑ 上行流量
                              </div>
                              <div className="font-mono text-primary-700 dark:text-primary-300">
                                {node.connectionStatus === "online" &&
                                node.systemInfo
                                  ? formatTraffic(node.systemInfo.uploadTraffic)
                                  : "-"}
                              </div>
                            </div>
                            <div className="text-center p-2 bg-success-50 dark:bg-success-100/20 rounded border border-success-200 dark:border-success-300/20">
                              <div className="text-success-600 dark:text-success-400 mb-0.5">
                                ↓ 下行流量
                              </div>
                              <div className="font-mono text-success-700 dark:text-success-300">
                                {node.connectionStatus === "online" &&
                                node.systemInfo
                                  ? formatTraffic(
                                      node.systemInfo.downloadTraffic,
                                    )
                                  : "-"}
                              </div>
                            </div>
                          </div>
                        </div>

                        {/* 操作按钮 */}
                        <div className="space-y-1.5">
                          <div className="flex gap-1.5">
                            <Button
                              className="flex-1 min-h-8"
                              color="success"
                              isDisabled={node.isRemote === 1}
                              isLoading={node.copyLoading}
                              size="sm"
                              variant="flat"
                              onPress={() => handleCopyInstallCommand(node)}
                            >
                              安装
                            </Button>
                            <Button
                              className="flex-1 min-h-8"
                              color="primary"
                              isDisabled={node.isRemote === 1}
                              size="sm"
                              variant="flat"
                              onPress={() => handleEdit(node)}
                            >
                              编辑
                            </Button>
                            <Button
                              className="flex-1 min-h-8"
                              color="danger"
                              size="sm"
                              variant="flat"
                              onPress={() => handleDelete(node)}
                            >
                              删除
                            </Button>
                          </div>
                        </div>
                      </CardBody>
                    </Card>
                  )}
                </SortableItem>
              ))}
            </div>
          </SortableContext>
        </DndContext>
      )}

      {/* 新增/编辑节点对话框 */}
      <Modal
        backdrop="blur"
        isOpen={dialogVisible}
        placement="center"
        scrollBehavior="outside"
        size="2xl"
        onClose={() => setDialogVisible(false)}
      >
        <ModalContent>
          <ModalHeader>{dialogTitle}</ModalHeader>
          <ModalBody>
            <div className="space-y-4">
              <Input
                errorMessage={errors.name}
                isInvalid={!!errors.name}
                label="节点名称"
                placeholder="请输入节点名称"
                value={form.name}
                variant="bordered"
                onChange={(e) =>
                  setForm((prev) => ({ ...prev, name: e.target.value }))
                }
              />

              <Input
                description="可选：不带协议、不带端口。至少填写一个 IPv4/IPv6/域名"
                errorMessage={errors.serverHost}
                isInvalid={!!errors.serverHost}
                label="服务器域名/主机名"
                placeholder="例如: node.example.com"
                value={form.serverHost}
                variant="bordered"
                onChange={(e) =>
                  setForm((prev) => ({ ...prev, serverHost: e.target.value }))
                }
              />

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <Input
                  description="双栈节点组隧道时优先使用 IPv4"
                  errorMessage={errors.serverIpV4}
                  isInvalid={!!errors.serverIpV4}
                  label="服务器IPv4"
                  placeholder="例如: 203.0.113.10"
                  value={form.serverIpV4}
                  variant="bordered"
                  onChange={(e) =>
                    setForm((prev) => ({ ...prev, serverIpV4: e.target.value }))
                  }
                />

                <Input
                  description="至少填写一个 IPv4/IPv6/域名"
                  errorMessage={errors.serverIpV6}
                  isInvalid={!!errors.serverIpV6}
                  label="服务器IPv6"
                  placeholder="例如: 2001:db8::10"
                  value={form.serverIpV6}
                  variant="bordered"
                  onChange={(e) =>
                    setForm((prev) => ({ ...prev, serverIpV6: e.target.value }))
                  }
                />
              </div>

              <Input
                classNames={{
                  input: "font-mono",
                }}
                description="支持单个端口(80)、多个端口(80,443)或端口范围(1000-65535)，多个可用逗号分隔"
                errorMessage={errors.port}
                isInvalid={!!errors.port}
                label="可用端口"
                placeholder="例如: 80,443,1000-65535"
                value={form.port}
                variant="bordered"
                onChange={(e) =>
                  setForm((prev) => ({ ...prev, port: e.target.value }))
                }
              />

              {/* 高级配置 */}
              <Accordion variant="bordered">
                <AccordionItem
                  key="advanced"
                  aria-label="高级配置"
                  title="高级配置"
                >
                  <div className="space-y-4 pb-2">
                    <Input
                      description="用于多IP服务器指定使用那个IP请求远程地址，不懂的默认为空就行"
                      errorMessage={errors.interfaceName}
                      isInvalid={!!errors.interfaceName}
                      label="出口网卡名或IP"
                      placeholder="请输入出口网卡名或IP"
                      value={form.interfaceName}
                      variant="bordered"
                      onChange={(e) =>
                        setForm((prev) => ({
                          ...prev,
                          interfaceName: e.target.value,
                        }))
                      }
                    />

                    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                      <Input
                        errorMessage={errors.tcpListenAddr}
                        isInvalid={!!errors.tcpListenAddr}
                        label="TCP监听地址"
                        placeholder="请输入TCP监听地址"
                        startContent={
                          <div className="pointer-events-none flex items-center">
                            <span className="text-default-400 text-small">
                              TCP
                            </span>
                          </div>
                        }
                        value={form.tcpListenAddr}
                        variant="bordered"
                        onChange={(e) =>
                          setForm((prev) => ({
                            ...prev,
                            tcpListenAddr: e.target.value,
                          }))
                        }
                      />

                      <Input
                        errorMessage={errors.udpListenAddr}
                        isInvalid={!!errors.udpListenAddr}
                        label="UDP监听地址"
                        placeholder="请输入UDP监听地址"
                        startContent={
                          <div className="pointer-events-none flex items-center">
                            <span className="text-default-400 text-small">
                              UDP
                            </span>
                          </div>
                        }
                        value={form.udpListenAddr}
                        variant="bordered"
                        onChange={(e) =>
                          setForm((prev) => ({
                            ...prev,
                            udpListenAddr: e.target.value,
                          }))
                        }
                      />
                    </div>
                    {/* 屏蔽协议 */}
                    <div>
                      <div className="text-sm font-medium text-default-700 mb-2">
                        屏蔽协议
                      </div>
                      <div className="text-xs text-default-500 mb-2">
                        开启开关以屏蔽对应协议
                      </div>
                      {protocolDisabled && (
                        <Alert
                          className="mb-2"
                          color="warning"
                          description={
                            protocolDisabledReason || "等待节点上线后再设置"
                          }
                          variant="flat"
                        />
                      )}
                      <div
                        className={`grid grid-cols-1 sm:grid-cols-3 gap-3 bg-default-50 dark:bg-default-100 p-3 rounded-md border border-default-200 dark:border-default-100/30 ${protocolDisabled ? "opacity-70" : ""}`}
                      >
                        {/* HTTP tile */}
                        <div className="px-3 py-3 rounded-lg bg-white dark:bg-default-50 border border-default-200 dark:border-default-100/30 hover:border-primary-200 transition-colors">
                          <div className="flex items-center gap-2 mb-2">
                            <svg
                              className="w-4 h-4 text-default-500"
                              fill="none"
                              stroke="currentColor"
                              strokeLinecap="round"
                              strokeLinejoin="round"
                              strokeWidth="2"
                              viewBox="0 0 24 24"
                            >
                              <rect height="16" rx="2" width="20" x="2" y="4" />
                              <path d="M2 10h20" />
                            </svg>
                            <div className="text-sm font-medium text-default-700">
                              HTTP
                            </div>
                          </div>
                          <div className="flex items-center justify-between">
                            <div className="text-xs text-default-500">
                              禁用/启用
                            </div>
                            <Switch
                              isDisabled={protocolDisabled}
                              isSelected={form.http === 1}
                              size="sm"
                              onValueChange={(v) =>
                                setForm((prev) => ({
                                  ...prev,
                                  http: v ? 1 : 0,
                                }))
                              }
                            />
                          </div>
                          <div className="mt-1 text-xs text-default-400">
                            {form.http === 1 ? "已开启" : "已关闭"}
                          </div>
                        </div>

                        {/* TLS tile */}
                        <div className="px-3 py-3 rounded-lg bg-white dark:bg-default-50 border border-default-200 dark:border-default-100/30 hover:border-primary-200 transition-colors">
                          <div className="flex items-center gap-2 mb-2">
                            <svg
                              className="w-4 h-4 text-default-500"
                              fill="none"
                              stroke="currentColor"
                              strokeLinecap="round"
                              strokeLinejoin="round"
                              strokeWidth="2"
                              viewBox="0 0 24 24"
                            >
                              <path d="M6 10V7a6 6 0 1 1 12 0v3" />
                              <rect
                                height="10"
                                rx="2"
                                width="16"
                                x="4"
                                y="10"
                              />
                            </svg>
                            <div className="text-sm font-medium text-default-700">
                              TLS
                            </div>
                          </div>
                          <div className="flex items-center justify-between">
                            <div className="text-xs text-default-500">
                              禁用/启用
                            </div>
                            <Switch
                              isDisabled={protocolDisabled}
                              isSelected={form.tls === 1}
                              size="sm"
                              onValueChange={(v) =>
                                setForm((prev) => ({ ...prev, tls: v ? 1 : 0 }))
                              }
                            />
                          </div>
                          <div className="mt-1 text-xs text-default-400">
                            {form.tls === 1 ? "已开启" : "已关闭"}
                          </div>
                        </div>

                        {/* SOCKS tile */}
                        <div className="px-3 py-3 rounded-lg bg-white dark:bg-default-50 border border-default-200 dark:border-default-100/30 hover:border-primary-200 transition-colors">
                          <div className="flex items-center gap-2 mb-2">
                            <svg
                              className="w-4 h-4 text-default-500"
                              fill="none"
                              stroke="currentColor"
                              strokeLinecap="round"
                              strokeLinejoin="round"
                              strokeWidth="2"
                              viewBox="0 0 24 24"
                            >
                              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
                              <polyline points="7 10 12 15 17 10" />
                              <line x1="12" x2="12" y1="15" y2="3" />
                            </svg>
                            <div className="text-sm font-medium text-default-700">
                              SOCKS
                            </div>
                          </div>
                          <div className="flex items-center justify-between">
                            <div className="text-xs text-default-500">
                              禁用/启用
                            </div>
                            <Switch
                              isDisabled={protocolDisabled}
                              isSelected={form.socks === 1}
                              size="sm"
                              onValueChange={(v) =>
                                setForm((prev) => ({
                                  ...prev,
                                  socks: v ? 1 : 0,
                                }))
                              }
                            />
                          </div>
                          <div className="mt-1 text-xs text-default-400">
                            {form.socks === 1 ? "已开启" : "已关闭"}
                          </div>
                        </div>
                      </div>
                    </div>

                    <Alert
                      color="danger"
                      description="请不要在出口节点执行屏蔽协议，否则可能影响转发；屏蔽协议仅需在入口节点执行。"
                      variant="flat"
                    />
                  </div>
                </AccordionItem>
              </Accordion>

              <Alert
                className="mt-4"
                color="primary"
                description="服务器ip是你要添加的服务器的ip地址，不是面板的ip地址。"
                variant="flat"
              />
            </div>
          </ModalBody>
          <ModalFooter>
            <Button variant="flat" onPress={() => setDialogVisible(false)}>
              取消
            </Button>
            <Button
              color="primary"
              isLoading={submitLoading}
              onPress={handleSubmit}
            >
              {submitLoading ? "提交中..." : "确定"}
            </Button>
          </ModalFooter>
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
                  确定要删除节点{" "}
                  <strong>&quot;{nodeToDelete?.name}&quot;</strong> 吗？
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

      {/* 安装命令模态框 */}
      <Modal
        backdrop="blur"
        isOpen={installCommandModal}
        placement="center"
        scrollBehavior="outside"
        size="2xl"
        onClose={() => setInstallCommandModal(false)}
      >
        <ModalContent>
          <ModalHeader>安装命令 - {currentNodeName}</ModalHeader>
          <ModalBody>
            <div className="space-y-4">
              <p className="text-sm text-default-600">
                请复制以下安装命令到服务器上执行：
              </p>
              <div className="relative">
                <Textarea
                  readOnly
                  className="font-mono text-sm"
                  classNames={{
                    input: "font-mono text-sm",
                  }}
                  maxRows={10}
                  minRows={6}
                  value={installCommand}
                  variant="bordered"
                />
                <Button
                  className="absolute top-2 right-2"
                  color="primary"
                  size="sm"
                  variant="flat"
                  onPress={handleManualCopy}
                >
                  复制
                </Button>
              </div>
              <div className="text-xs text-default-500">
                💡 提示：如果复制按钮失效，请手动选择上方文本进行复制
              </div>
            </div>
          </ModalBody>
          <ModalFooter>
            <Button
              variant="flat"
              onPress={() => setInstallCommandModal(false)}
            >
              关闭
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* 批量删除确认模态框 */}
      <Modal
        backdrop="blur"
        isOpen={batchDeleteModalOpen}
        placement="center"
        scrollBehavior="outside"
        size="md"
        onOpenChange={setBatchDeleteModalOpen}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                <h2 className="text-xl font-bold">确认删除</h2>
              </ModalHeader>
              <ModalBody>
                <p>
                  确定要删除选中的 <strong>{selectedIds.size}</strong>{" "}
                  个节点吗？
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
                  isLoading={batchLoading}
                  onPress={handleBatchDelete}
                >
                  {batchLoading ? "删除中..." : "确认删除"}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </div>
  );
}
