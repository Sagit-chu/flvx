import type {
  NodeMetricApiItem,
  ServiceMonitorApiItem,
  ServiceMonitorResultApiItem,
  ServiceMonitorMutationPayload,
  ServiceMonitorLimitsApiData,
} from "@/api/types";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import {
  MoreVertical,
  Plus,
  RefreshCw,
  Trash2,
  Edit,
  Activity,
  Play,
  Server,
  Clock,
  ArrowLeft,
  ArrowUp,
  ArrowDown,
  Eye,

} from "lucide-react";
import toast from "react-hot-toast";
import { DistroIcon, parseDistroFromVersion, getDistroColor } from "@/components/distro-icon";

import {
  getNodeMetrics,
  getServiceMonitorList,
  getServiceMonitorLimits,
  getServiceMonitorResults,
  getServiceMonitorLatestResults,
  createServiceMonitor,
  updateServiceMonitor,
  deleteServiceMonitor,
  runServiceMonitor,
} from "@/api";
import { Button } from "@/shadcn-bridge/heroui/button";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
} from "@/shadcn-bridge/heroui/modal";
import { Input } from "@/shadcn-bridge/heroui/input";
import { Switch } from "@/shadcn-bridge/heroui/switch";
import { Select, SelectItem } from "@/shadcn-bridge/heroui/select";
import {
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
} from "@/shadcn-bridge/heroui/table";
import { Chip } from "@/shadcn-bridge/heroui/chip";
import {
  Dropdown,
  DropdownTrigger,
  DropdownMenu,
  DropdownItem,
} from "@/shadcn-bridge/heroui/dropdown";
import { Card, CardBody, CardHeader } from "@/shadcn-bridge/heroui/card";
import { Progress } from "@/shadcn-bridge/heroui/progress";
import { useNodeRealtime } from "@/pages/node/use-node-realtime";

interface MonitorViewProps {
  nodeMap: Map<number, { id: number; name: string; connectionStatus: string; version?: string }>;
  viewMode?: "list" | "grid";
}

type RealtimeNodeMetric = {
  receivedAt: number;
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
};

const isSupportedMonitorType = (t: string): t is "tcp" | "icmp" =>
  t === "tcp" || t === "icmp";

const formatTimestamp = (ts: number, rangeMs?: number): string => {
  const date = new Date(ts);
  const includeDate = (rangeMs ?? 0) >= 24 * 60 * 60 * 1000;

  if (includeDate) {
    return date.toLocaleString("zh-CN", {
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    });
  }

  return date.toLocaleTimeString("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
  });
};

const formatDateTime = (ts: number): string => {
  return new Date(ts).toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
};

const formatBytes = (bytes: number): string => {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";

  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));

  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`;
};

const formatBytesPerSecond = (bytesPerSecond: number): string => {
  if (!Number.isFinite(bytesPerSecond) || bytesPerSecond <= 0) return "0 B/s";

  const k = 1024;
  const sizes = ["B/s", "KB/s", "MB/s", "GB/s", "TB/s"];
  const i = Math.floor(Math.log(bytesPerSecond) / Math.log(k));

  return `${parseFloat((bytesPerSecond / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`;
};

const formatUptime = (seconds: number) => {
  if (!seconds) return "-";
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  if (days > 0) return `${days} 天 ${hours} 小时`;
  return `${hours} 小时`;
};

const getColorByUsage = (usage?: number) => {
  if (usage === undefined || usage === null) return "default";
  if (usage >= 90) return "danger";
  if (usage >= 75) return "warning";
  if (usage >= 50) return "primary";
  return "success";
};

function ServerCard({ node, metric, onPress }: { node: any; metric: RealtimeNodeMetric | null; onPress?: () => void }) {
  const isOnline = node.connectionStatus === "online";
  const distro = parseDistroFromVersion(node.version);
  const distroColor = getDistroColor(distro);
  
  return (
    <Card
      className="group relative hover:-translate-y-1 hover:shadow-[0_20px_40px_rgba(0,0,0,0.15)] transition-all duration-300 h-full flex flex-col cursor-pointer"
      onClick={onPress}
    >
      {/* Dynamic top gradient bar based on status */}
      <div className={`absolute top-0 left-0 right-0 h-1 ${isOnline ? "bg-success" : "bg-danger"}`} />
      
      {/* Decorative background glow */}
      <div className={`absolute -right-8 -top-8 w-24 h-24 rounded-full blur-2xl opacity-10 transition-opacity group-hover:opacity-20 ${isOnline ? "bg-success" : "bg-danger"}`} />

      <CardHeader className="pb-2 pt-5 px-5 flex flex-row justify-between items-start gap-4">
        <div className="flex items-center gap-3 min-w-0">
          <div className="relative flex-shrink-0">
            <div className="w-10 h-10 rounded-xl bg-default-100 dark:bg-default-50/10 flex items-center justify-center border border-divider">
              <DistroIcon distro={distro} className="w-5 h-5" style={{ color: isOnline ? distroColor : undefined }} />
            </div>
            <span className={`absolute -bottom-0.5 -right-0.5 w-3 h-3 rounded-full border-2 border-background ${isOnline ? "bg-success" : "bg-danger"}`} />
          </div>
          <div className="flex flex-col min-w-0">
            <h3 className="font-semibold text-foreground text-sm truncate">{node.name}</h3>
            <div className="flex items-center gap-1.5 text-[11px] text-default-500 mt-0.5">
              <span className="font-mono">{isOnline ? "在线" : "离线"}</span>
            </div>
          </div>
        </div>
      </CardHeader>
      
      <CardBody className="py-3 px-5 flex-1 flex flex-col justify-end gap-4 z-10 w-full overflow-hidden">
        <div className="grid grid-cols-2 gap-5 w-full">
          <div className="space-y-1.5 min-w-0">
            <div className="flex justify-between items-center text-xs">
              <span className="text-default-500">CPU</span>
              <span className="font-mono font-medium">{isOnline && metric ? `${metric.cpuUsage.toFixed(1)}%` : "-"}</span>
            </div>
            <Progress
              className="h-1"
              color={getColorByUsage(metric?.cpuUsage)}
              value={isOnline && metric ? metric.cpuUsage : 0}
            />
          </div>
          <div className="space-y-1.5 min-w-0">
            <div className="flex justify-between items-center text-xs">
              <span className="text-default-500">内存</span>
              <span className="font-mono font-medium">{isOnline && metric ? `${metric.memoryUsage.toFixed(1)}%` : "-"}</span>
            </div>
            <Progress
              className="h-1"
              color={getColorByUsage(metric?.memoryUsage)}
              size="sm"
              value={isOnline && metric ? metric.memoryUsage : 0}
            />
          </div>
        </div>

        <div className="flex items-center justify-between p-2.5 rounded-lg bg-default-50 dark:bg-default-100/30 border border-divider mt-1 w-full">
          <div className="flex flex-col gap-0.5 min-w-0 flex-1">
            <div className="flex items-center gap-1 text-[10px] text-default-500">
              <span className="text-primary-500 inline-block font-bold">↑</span>
              <span>上传</span>
            </div>
            <span className="font-mono text-xs font-semibold truncate">{isOnline && metric ? formatBytesPerSecond(metric.netOutSpeed) : "-"}</span>
          </div>
          <div className="w-px h-6 bg-divider mx-2" />
          <div className="flex flex-col gap-0.5 items-end min-w-0 flex-1">
            <div className="flex items-center gap-1 text-[10px] text-default-500">
              <span>下载</span>
              <span className="text-success-500 inline-block font-bold">↓</span>
            </div>
            <span className="font-mono text-xs font-semibold truncate">{isOnline && metric ? formatBytesPerSecond(metric.netInSpeed) : "-"}</span>
          </div>
        </div>

        <div className="flex justify-between items-center mt-1 pt-3 border-t border-divider/50 w-full overflow-hidden">
          <div className="flex items-center gap-1.5 text-[11px] text-default-500 min-w-0">
            <Clock className="w-3.5 h-3.5 text-default-400 shrink-0" />
            <span className="font-mono truncate">{metric ? formatUptime(metric.uptime) : "-"}</span>
          </div>
          <div className="flex items-center gap-1.5 text-[11px] text-default-500 shrink-0 ml-2">
            <Activity className="w-3.5 h-3.5 text-default-400" />
            <span className="font-mono">Load: {metric ? metric.load1.toFixed(2) : "-"}</span>
          </div>
        </div>
      </CardBody>
    </Card>
  );
}

type MetricType =
  | "cpu"
  | "memory"
  | "disk"
  | "network"
  | "load"
  | "connections";

const METRICS_MAX_ROWS = 5000;

/* ─── Memoized Node Metrics Chart sub-component ─────────────────── */

interface NodeMetricsChartCardProps {
  rangeMs: number;
  onRangeChange: (v: number) => void;
  activeMetricType: MetricType;
  onMetricTypeChange: (t: MetricType) => void;
  loading: boolean;
  error: string | null;
  truncated: boolean;
  maxRows: number;
  data: Array<Record<string, unknown>>;
  nodeId: number | null;
  onRefresh: (id: number) => void;
}

const METRIC_TYPE_BUTTONS: { key: MetricType; label: string }[] = [
  { key: "cpu", label: "CPU" },
  { key: "memory", label: "内存" },
  { key: "disk", label: "磁盘" },
  { key: "network", label: "网络" },
  { key: "load", label: "负载" },
  { key: "connections", label: "连接" },
];

const NodeMetricsChartCard = React.memo(function NodeMetricsChartCard({
  rangeMs, onRangeChange, activeMetricType, onMetricTypeChange,
  loading, error, truncated, maxRows, data, nodeId, onRefresh,
}: NodeMetricsChartCardProps) {
  const chartConfig = (() => {
    switch (activeMetricType) {
      case "cpu":    return { lines: [{ dataKey: "cpu", color: "#3b82f6", name: "CPU %" }], yAxisLabel: "使用率 (%)" };
      case "memory": return { lines: [{ dataKey: "memory", color: "#8b5cf6", name: "内存 %" }], yAxisLabel: "使用率 (%)" };
      case "disk":   return { lines: [{ dataKey: "disk", color: "#f59e0b", name: "磁盘 %" }], yAxisLabel: "使用率 (%)" };
      case "network": return { lines: [{ dataKey: "netIn", color: "#10b981", name: "入站速度" }, { dataKey: "netOut", color: "#ef4444", name: "出站速度" }], yAxisLabel: "速度 (bytes/s)" };
      case "load":   return { lines: [{ dataKey: "load1", color: "#3b82f6", name: "负载 1m" }, { dataKey: "load5", color: "#8b5cf6", name: "负载 5m" }, { dataKey: "load15", color: "#f59e0b", name: "负载 15m" }], yAxisLabel: "负载值" };
      case "connections": return { lines: [{ dataKey: "tcp", color: "#3b82f6", name: "TCP 连接" }, { dataKey: "udp", color: "#10b981", name: "UDP 连接" }], yAxisLabel: "连接数" };
    }
  })();

  const yAxisTickFormatter = (value: unknown) => {
    const n = Number(value);
    if (!Number.isFinite(n)) return "";
    switch (activeMetricType) {
      case "network": return formatBytesPerSecond(n);
      case "cpu": case "memory": case "disk": return `${n.toFixed(0)}%`;
      case "load": return n.toFixed(1);
      case "connections": return String(Math.round(n));
    }
  };

  const tooltipFormatter = (value: unknown) => {
    const n = Number(value);
    if (!Number.isFinite(n)) return "-";
    switch (activeMetricType) {
      case "network": return formatBytesPerSecond(n);
      case "cpu": case "memory": case "disk": return `${n.toFixed(1)}%`;
      case "load": return n.toFixed(2);
      case "connections": return String(Math.round(n));
    }
  };

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <h3 className="text-lg font-semibold">节点指标图表</h3>
        <div className="flex items-center gap-2">
          <Select
            className="w-36"
            selectedKeys={[String(rangeMs)]}
            onSelectionChange={(keys) => {
              const v = Number(Array.from(keys)[0]);
              if (v > 0) onRangeChange(v);
            }}
          >
            <SelectItem key={String(15 * 60 * 1000)}>15分钟</SelectItem>
            <SelectItem key={String(60 * 60 * 1000)}>1小时</SelectItem>
            <SelectItem key={String(6 * 60 * 60 * 1000)}>6小时</SelectItem>
            <SelectItem key={String(24 * 60 * 60 * 1000)}>24小时</SelectItem>
          </Select>
          <Button isLoading={loading} size="sm" variant="flat" onPress={() => nodeId && onRefresh(nodeId)}>
            <RefreshCw className="w-4 h-4 mr-1" />
            刷新
          </Button>
        </div>
      </CardHeader>
      <CardBody className="space-y-4">
        <div className="flex flex-wrap gap-2">
          {METRIC_TYPE_BUTTONS.map((item) => (
            <Button
              key={item.key}
              color={activeMetricType === item.key ? "primary" : "default"}
              size="sm"
              variant={activeMetricType === item.key ? "solid" : "flat"}
              onPress={() => onMetricTypeChange(item.key)}
            >
              {item.label}
            </Button>
          ))}
        </div>

        {loading ? (
          <div className="flex justify-center py-8"><RefreshCw className="w-6 h-6 animate-spin" /></div>
        ) : error ? (
          <div className="text-center py-8 text-danger text-sm">{error}</div>
        ) : data.length > 0 ? (
          <>
            <div className="h-64">
              <ResponsiveContainer height="100%" width="100%">
                <LineChart data={data}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="time" fontSize={12} />
                  <YAxis fontSize={12} tickFormatter={yAxisTickFormatter} />
                  <Tooltip
                    contentStyle={{ backgroundColor: "rgba(0,0,0,0.8)", border: "none", borderRadius: "8px" }}
                    labelStyle={{ color: "#fff" }}
                    formatter={tooltipFormatter}
                  />
                  {chartConfig.lines.map((line) => (
                    <Line key={line.dataKey} dataKey={line.dataKey} dot={false} name={line.name} stroke={line.color} strokeWidth={2} type="monotone" />
                  ))}
                </LineChart>
              </ResponsiveContainer>
            </div>
            {truncated && (
              <div className="text-xs text-default-500">数据点过多，已截断为最近 {maxRows} 条，建议缩小时间范围。</div>
            )}
          </>
        ) : (
          <div className="text-center py-8 text-default-500">暂无指标数据</div>
        )}
      </CardBody>
    </Card>
  );
});


const DEFAULT_SERVICE_MONITOR_LIMITS: ServiceMonitorLimitsApiData = {
  checkerScanIntervalSec: 1,
  minIntervalSec: 1,
  defaultIntervalSec: 1,
  minTimeoutSec: 1,
  defaultTimeoutSec: 5,
  maxTimeoutSec: 60,
};

export function MonitorView({ nodeMap, viewMode = "grid" }: MonitorViewProps) {
  const [detailNodeId, setDetailNodeId] = useState<number | null>(null);
  const [selectedNodeId, setSelectedNodeId] = useState<number | null>(null);
  const [metrics, setMetrics] = useState<NodeMetricApiItem[]>([]);
  const [metricsLoading, setMetricsLoading] = useState(false);
  const [metricsTruncated, setMetricsTruncated] = useState(false);
  const [metricsError, setMetricsError] = useState<string | null>(null);
  const [activeMetricType, setActiveMetricType] = useState<MetricType>("cpu");

  const [metricsRangeMs, setMetricsRangeMs] = useState(60 * 60 * 1000);



  const [serviceMonitors, setServiceMonitors] = useState<
    ServiceMonitorApiItem[]
  >([]);
  const [monitorsLoading, setMonitorsLoading] = useState(false);
  const [, setMonitorsError] = useState<string | null>(null);
  const [, setLatestResultsError] = useState<string | null>(
    null,
  );
  const [monitorResults, setMonitorResults] = useState<
    Record<number, ServiceMonitorResultApiItem[]>
  >({});

  const [serviceMonitorLimits, setServiceMonitorLimits] =
    useState<ServiceMonitorLimitsApiData | null>(null);

  const [editModalOpen, setEditModalOpen] = useState(false);
  const [editingMonitor, setEditingMonitor] =
    useState<ServiceMonitorApiItem | null>(null);
  const [monitorForm, setMonitorForm] = useState({
    name: "",
    type: "tcp" as "tcp" | "icmp",
    target: "",
    intervalSec: DEFAULT_SERVICE_MONITOR_LIMITS.defaultIntervalSec,
    timeoutSec: DEFAULT_SERVICE_MONITOR_LIMITS.defaultTimeoutSec,
    nodeId: 0,
    enabled: true,
  });
  const [submitLoading, setSubmitLoading] = useState(false);
  const [activeServiceMonitorId, setActiveServiceMonitorId] = useState<number | null>(null);
  const [serviceMonitorRangeMs, setServiceMonitorRangeMs] = useState(60 * 60 * 1000);

  const [accessDenied, setAccessDenied] = useState<string | null>(null);
  const [resultsModalOpen, setResultsModalOpen] = useState(false);
  const [resultsMonitorId, setResultsMonitorId] = useState<number | null>(null);
  const [resultsLimit, setResultsLimit] = useState(50);
  const [resultsLoading, setResultsLoading] = useState(false);

  const [realtimeNodeStatus, setRealtimeNodeStatus] = useState<
    Record<number, "online" | "offline">
  >({});
  const [realtimeNodeMetrics, setRealtimeNodeMetrics] = useState<
    Record<number, RealtimeNodeMetric>
  >({});

  const nodes = useMemo(() => {
    return Array.from(nodeMap.values()).map((n) => {
      const status = realtimeNodeStatus[n.id];

      if (!status) {
        return n;
      }

      return {
        ...n,
        connectionStatus: status,
      };
    });
  }, [nodeMap, realtimeNodeStatus]);

  const onlineNodes = nodes.filter((n) => n.connectionStatus === "online");
  const preferredNodeId = onlineNodes[0]?.id ?? nodes[0]?.id ?? 0;

  const resolvedServiceMonitorLimits =
    serviceMonitorLimits ?? DEFAULT_SERVICE_MONITOR_LIMITS;

  const handleRealtimeMessage = useCallback((message: any) => {
    const nodeId = Number(message?.id ?? 0);

    if (!nodeId || Number.isNaN(nodeId)) {
      return;
    }

    const type = String(message?.type ?? "");
    const payload = message?.data;

    if (type === "status") {
      const status = Number(payload);

      setRealtimeNodeStatus((prev) => ({
        ...prev,
        [nodeId]: status === 1 ? "online" : "offline",
      }));

      return;
    }

    if (type === "metric") {
      let raw = payload;
      if (typeof raw === "string") {
        try {
          raw = JSON.parse(raw);
        } catch {
          return;
        }
      }
      if (!raw || typeof raw !== "object") {
        return;
      }

      const metric = raw as Record<string, unknown>;
      const receivedAt = Date.now();
      const incomingUptime = Number(metric.uptime ?? 0);

      setRealtimeNodeMetrics((prev) => {
        const normalized: RealtimeNodeMetric = {
          receivedAt,
          cpuUsage: Number(metric.cpuUsage ?? metric.cpu_usage ?? 0),
          memoryUsage: Number(metric.memoryUsage ?? metric.memory_usage ?? 0),
          diskUsage: Number(metric.diskUsage ?? metric.disk_usage ?? 0),
          netInBytes: Number(metric.netInBytes ?? metric.bytes_received ?? 0),
          netOutBytes: Number(metric.netOutBytes ?? metric.bytes_transmitted ?? 0),
          netInSpeed: Number(metric.netInSpeed ?? metric.net_in_speed ?? 0),
          netOutSpeed: Number(metric.netOutSpeed ?? metric.net_out_speed ?? 0),
          load1: Number(metric.load1 ?? 0),
          load5: Number(metric.load5 ?? 0),
          load15: Number(metric.load15 ?? 0),
          tcpConns: Number(metric.tcpConns ?? metric.tcp_conns ?? 0),
          udpConns: Number(metric.udpConns ?? metric.udp_conns ?? 0),
          uptime: incomingUptime || prev[nodeId]?.uptime || 0,
        };

        return {
          ...prev,
          [nodeId]: normalized,
        };
      });
      setRealtimeNodeStatus((prev) => ({
        ...prev,
        [nodeId]: "online",
      }));

      return;
    }

    if (type === "info") {
      setRealtimeNodeStatus((prev) => ({
        ...prev,
        [nodeId]: "online",
      }));
    }
  }, []);

  const { wsConnected, wsConnecting } = useNodeRealtime({
    onMessage: handleRealtimeMessage,
    enabled: !accessDenied,
  });

  useEffect(() => {
    if (!selectedNodeId && preferredNodeId > 0) {
      setSelectedNodeId(preferredNodeId);
    }
  }, [preferredNodeId, selectedNodeId]);



  const loadMetrics = useCallback(
    async (nodeId: number, options?: { silent?: boolean }) => {
      const silent = options?.silent ?? false;
      if (!silent) setMetricsLoading(true);
      try {
        const end = Date.now();
        const start = end - metricsRangeMs;
        const response = await getNodeMetrics(nodeId, start, end);

        if (response.code === 0 && Array.isArray(response.data)) {
          setAccessDenied(null);
          setMetricsError(null);
          setMetricsTruncated(response.data.length >= METRICS_MAX_ROWS);
          const ordered = [...response.data].sort(
            (a, b) => a.timestamp - b.timestamp,
          );

          setMetrics(ordered);

          return;
        }
        if (response.code === 403) {
          setAccessDenied(response.msg || "暂无监控权限，请联系管理员授权");
          setMetricsTruncated(false);
          setMetricsError(null);

          return;
        }
        setMetricsTruncated(false);
        const msg = response.msg || "加载指标失败";
        const isTimeout = msg.toLowerCase().includes("timeout");
        const friendlyMsg = isTimeout ? "加载指标超时，请缩小时间范围后重试" : msg;
        setMetricsError(friendlyMsg);
        if (!silent) toast.error(friendlyMsg);
      } catch {
        setMetricsTruncated(false);
        if (!silent) setMetricsError("加载指标失败");
      } finally {
        if (!silent) setMetricsLoading(false);
      }
    },
    [metricsRangeMs],
  );

  const loadServiceMonitors = useCallback(async (options?: { silent?: boolean }) => {
    const silent = options?.silent ?? false;
    if (!silent) setMonitorsLoading(true);
    try {
      const response = await getServiceMonitorList();

      if (response.code === 0 && response.data) {
        setAccessDenied(null);
        setMonitorsError(null);
        setServiceMonitors(response.data);

        return;
      }
      if (response.code === 403) {
        setAccessDenied(response.msg || "暂无监控权限，请联系管理员授权");
        setMonitorsError(null);

        return;
      }
      setMonitorsError(response.msg || "加载服务监控失败");
      if (!silent) toast.error(response.msg || "加载服务监控失败");
    } catch {
      if (!silent) setMonitorsError("加载服务监控失败");
    } finally {
      if (!silent) setMonitorsLoading(false);
    }
  }, []);

  const loadServiceMonitorLimits = useCallback(async () => {
    try {
      const response = await getServiceMonitorLimits();

      if (response.code === 0 && response.data) {
        setAccessDenied(null);
        setServiceMonitorLimits(response.data);

        return;
      }
      if (response.code === 403) {
        setAccessDenied(response.msg || "暂无监控权限，请联系管理员授权");

        return;
      }
    } catch {
      toast.error("加载服务监控限制失败，已使用默认值");
    }
  }, []);

  const loadMonitorResults = useCallback(
    async (monitorId: number, options?: { rangeMs?: number; limit?: number }) => {
      try {
        const apiOptions = options?.rangeMs != null
          ? { start: Date.now() - options.rangeMs, end: Date.now() }
          : { limit: options?.limit ?? 100 };
        const response = await getServiceMonitorResults(monitorId, apiOptions);

        if (response.code === 0 && response.data) {
          setMonitorResults((prev) => ({
            ...prev,
            [monitorId]: response.data,
          }));

          return;
        }
        if (response.code === 403) {
          setAccessDenied(response.msg || "暂无监控权限，请联系管理员授权");
        }
      } catch {
        toast.error("加载监控记录失败");
      }
    },
    [],
  );

  const loadLatestMonitorResults = useCallback(async () => {
    try {
      const response = await getServiceMonitorLatestResults();

      if (response.code === 0 && Array.isArray(response.data)) {
        setAccessDenied(null);
        setLatestResultsError(null);
        setMonitorResults((prev) => {
          const next: Record<number, ServiceMonitorResultApiItem[]> = {
            ...prev,
          };

          response.data.forEach((r) => {
            const monitorId = Number(r?.monitorId ?? 0);

            if (monitorId <= 0) return;

            const existing = next[monitorId];

            if (existing && existing.length > 1) {
              const rest = existing.filter((x) => x.id !== r.id);

              next[monitorId] = [r, ...rest];

              return;
            }

            next[monitorId] = [r];
          });

          return next;
        });

        return;
      }
      if (response.code === 403) {
        setAccessDenied(response.msg || "暂无监控权限，请联系管理员授权");
        setLatestResultsError(null);

        return;
      }

      setLatestResultsError(response.msg || "加载最新监控结果失败");
    } catch {
      setLatestResultsError("加载最新监控结果失败");
    }
  }, []);

  const loadResultsForModal = useCallback(async () => {
    if (!resultsMonitorId) return;
    setResultsLoading(true);
    try {
      await loadMonitorResults(resultsMonitorId, { limit: resultsLimit });
    } finally {
      setResultsLoading(false);
    }
  }, [loadMonitorResults, resultsLimit, resultsMonitorId]);

  useEffect(() => {
    void loadServiceMonitors();
    void loadServiceMonitorLimits();
    void loadLatestMonitorResults();
  }, [loadLatestMonitorResults, loadServiceMonitorLimits, loadServiceMonitors]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      void loadServiceMonitors({ silent: true });
      void loadLatestMonitorResults();
    }, 5_000);

    return () => window.clearInterval(timer);
  }, [loadLatestMonitorResults, loadServiceMonitors]);

  useEffect(() => {
    if (selectedNodeId) {
      loadMetrics(selectedNodeId);
    }
  }, [selectedNodeId, loadMetrics]);

  useEffect(() => {
    if (!selectedNodeId) return;
    const timer = window.setInterval(() => {
      void loadMetrics(selectedNodeId, { silent: true });
    }, 15_000);

    return () => window.clearInterval(timer);
  }, [selectedNodeId, loadMetrics]);

  useEffect(() => {
    if (!resultsModalOpen || !resultsMonitorId) return;
    void loadResultsForModal();
  }, [resultsModalOpen, resultsMonitorId, resultsLimit, loadResultsForModal]);

  // Auto-load results for the resolved default monitor when entering detail view
  useEffect(() => {
    if (!detailNodeId) return;
    if (activeServiceMonitorId) return; // user already selected one
    // Find the first monitor belonging to this node (or panel-level)
    const firstMonitor = serviceMonitors.find(
      (m) => m.nodeId === detailNodeId || m.nodeId === 0,
    );
    if (firstMonitor && (!monitorResults[firstMonitor.id] || monitorResults[firstMonitor.id].length <= 1)) {
      void loadMonitorResults(firstMonitor.id, { rangeMs: serviceMonitorRangeMs });
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [detailNodeId, serviceMonitors]);

  // Reload results for the active service monitor chart when time range changes
  useEffect(() => {
    if (!activeServiceMonitorId) return;
    void loadMonitorResults(activeServiceMonitorId, { rangeMs: serviceMonitorRangeMs });
  }, [activeServiceMonitorId, serviceMonitorRangeMs, loadMonitorResults]);

  const chartData = useMemo(() => metrics.map((m) => ({
    time: formatTimestamp(m.timestamp, metricsRangeMs),
    cpu: m.cpuUsage,
    memory: m.memoryUsage,
    disk: m.diskUsage,
    netIn: m.netInSpeed,
    netOut: m.netOutSpeed,
    load1: m.load1,
    load5: m.load5,
    load15: m.load15,
    tcp: m.tcpConns,
    udp: m.udpConns,
  })), [metrics, metricsRangeMs]);



  const handleOpenEditModal = (monitor?: ServiceMonitorApiItem) => {
    if (monitor) {
      if (!isSupportedMonitorType(monitor.type)) {
        toast.error("该监控类型已不支持，仅支持删除");

        return;
      }
      setEditingMonitor(monitor);
      setMonitorForm({
        name: monitor.name,
        type: monitor.type as "tcp" | "icmp",
        target: monitor.target,
        intervalSec: monitor.intervalSec,
        timeoutSec: monitor.timeoutSec,
        nodeId: monitor.nodeId,
        enabled: monitor.enabled === 1,
      });
    } else {
      setEditingMonitor(null);
      setMonitorForm({
        name: "",
        type: "tcp",
        target: "",
        intervalSec: resolvedServiceMonitorLimits.defaultIntervalSec,
        timeoutSec: resolvedServiceMonitorLimits.defaultTimeoutSec,
        nodeId: 0,
        enabled: true,
      });
    }
    setEditModalOpen(true);
  };

  const handleSubmitMonitor = async () => {
    if (!monitorForm.name || !monitorForm.target) {
      toast.error("请填写完整信息");

      return;
    }

    if (
      monitorForm.type === "icmp" &&
      (!monitorForm.nodeId || monitorForm.nodeId <= 0)
    ) {
      toast.error("ICMP 监控必须选择执行节点");

      return;
    }

    if (monitorForm.intervalSec < resolvedServiceMonitorLimits.minIntervalSec) {
      toast.error(
        `检查间隔不能小于 ${resolvedServiceMonitorLimits.minIntervalSec}s`,
      );

      return;
    }

    if (
      monitorForm.timeoutSec < resolvedServiceMonitorLimits.minTimeoutSec ||
      monitorForm.timeoutSec > resolvedServiceMonitorLimits.maxTimeoutSec
    ) {
      toast.error(
        `超时时间需在 ${resolvedServiceMonitorLimits.minTimeoutSec}-${resolvedServiceMonitorLimits.maxTimeoutSec}s 范围内`,
      );

      return;
    }

    setSubmitLoading(true);
    try {
      const payload: ServiceMonitorMutationPayload = {
        name: monitorForm.name,
        type: monitorForm.type,
        target: monitorForm.target,
        intervalSec: monitorForm.intervalSec,
        timeoutSec: monitorForm.timeoutSec,
        nodeId: monitorForm.nodeId,
        enabled: monitorForm.enabled ? 1 : 0,
      };

      if (editingMonitor) {
        const response = await updateServiceMonitor({
          ...payload,
          id: editingMonitor.id,
        });

        if (response.code === 0) {
          toast.success("更新成功");
        } else {
          toast.error(response.msg || "更新失败");
        }
      } else {
        const response = await createServiceMonitor(payload);

        if (response.code === 0) {
          toast.success("创建成功");
        } else {
          toast.error(response.msg || "创建失败");
        }
      }

      setEditModalOpen(false);
      void loadServiceMonitors();
      void loadLatestMonitorResults();
    } catch {
      toast.error("操作失败");
    } finally {
      setSubmitLoading(false);
    }
  };

  const handleDeleteMonitor = async (id: number) => {
    if (!confirm("确定删除该监控项?")) return;
    try {
      const response = await deleteServiceMonitor(id);

      if (response.code === 0) {
        toast.success("删除成功");
        void loadServiceMonitors();
        void loadLatestMonitorResults();
      } else {
        toast.error(response.msg || "删除失败");
      }
    } catch {
      toast.error("删除失败");
    }
  };

  const handleRunMonitor = async (id: number) => {
    try {
      const response = await runServiceMonitor(id);

      if (response.code === 0) {
        const ok = response.data?.success === 1;

        if (ok) {
          toast.success("检查成功");
        } else {
          toast.error(response.data?.errorMessage || "检查失败");
        }

        if (response.data) {
          const latest = response.data;

          setMonitorResults((prev) => {
            const next = { ...prev };
            const existing = next[id];

            if (existing && existing.length > 1) {
              next[id] = [
                latest,
                ...existing.filter((x) => x.id !== latest.id),
              ];
            } else {
              next[id] = [latest];
            }

            return next;
          });
        }

        void loadLatestMonitorResults();
        if (resultsMonitorId === id && resultsModalOpen) {
          void loadResultsForModal();
        }
      } else if (response.code === 403) {
        setAccessDenied(response.msg || "暂无监控权限，请联系管理员授权");
      } else {
        toast.error(response.msg || "执行失败");
      }
    } catch {
      toast.error("执行失败");
    }
  };

  const openResultsModal = (monitorId: number) => {
    setResultsMonitorId(monitorId);
    setResultsModalOpen(true);
  };

  const getLatestResult = (
    monitorId: number,
  ): ServiceMonitorResultApiItem | null => {
    const results = monitorResults[monitorId];

    return results && results.length > 0 ? results[0] : null;
  };

  const resultsMonitor =
    resultsMonitorId != null
      ? serviceMonitors.find((m) => m.id === resultsMonitorId) || null
      : null;
  const modalResults =
    resultsMonitorId != null ? monitorResults[resultsMonitorId] || [] : [];

  const resolveMonitorIntervalSec = useCallback(
    (monitor: ServiceMonitorApiItem) => {
      let interval = Number(monitor.intervalSec ?? 0);

      if (!interval || interval <= 0) {
        interval = resolvedServiceMonitorLimits.defaultIntervalSec;
      }

      return Math.max(interval, resolvedServiceMonitorLimits.minIntervalSec);
    },
    [resolvedServiceMonitorLimits.defaultIntervalSec, resolvedServiceMonitorLimits.minIntervalSec],
  );

  // Backend batches DB writes every 30s, but latest API reads from in-memory cache.
  // The cache timestamp reflects the real check time (every ~1s), so stale detection
  // should still allow for the batch report interval + scan jitter.
  const SERVICE_MONITOR_REPORT_INTERVAL_MS = 30_000; // matches backend serviceMonitorReportInterval

  const isResultStale = useCallback(
    (monitor: ServiceMonitorApiItem, latestResult: ServiceMonitorResultApiItem | null) => {
      if (monitor.enabled !== 1) {
        return false;
      }
	  if (!latestResult || !latestResult.timestamp) {
		  return false;
	  }

      const intervalMs = resolveMonitorIntervalSec(monitor) * 1000;
      // Budget = batch report interval + one check interval + scan jitter + grace
      const budgetMs =
        SERVICE_MONITOR_REPORT_INTERVAL_MS + intervalMs + resolvedServiceMonitorLimits.checkerScanIntervalSec * 1000 + 5000;

      return Date.now() - latestResult.timestamp > budgetMs;
    },
    [resolveMonitorIntervalSec, resolvedServiceMonitorLimits.checkerScanIntervalSec],
  );

  const monitorSummary = useMemo(() => {
    let disabled = 0;
    let ok = 0;
    let fail = 0;
    let unknown = 0;
    let stale = 0;

    serviceMonitors.forEach((m) => {
      if (m.enabled !== 1) {
        disabled += 1;

        return;
      }

      const latest = getLatestResult(m.id);
	  if (!latest) {
		  unknown += 1;
		  return;
	  }

      if (isResultStale(m, latest)) {
        stale += 1;
      }
      if (latest.success === 1) {
        ok += 1;
      } else {
        fail += 1;
      }
    });

    return { disabled, ok, fail, unknown, stale };
  }, [getLatestResult, isResultStale, serviceMonitors]);



  const detailNode = detailNodeId != null ? nodes.find((n) => n.id === detailNodeId) : null;
  const detailRealtimeMetric = detailNodeId != null ? realtimeNodeMetrics[detailNodeId] || null : null;

  // Service monitors filtered for the detail node (0 = panel-executed, show all in that case)
  const detailServiceMonitors = detailNodeId != null
    ? serviceMonitors.filter((m) => m.nodeId === detailNodeId || m.nodeId === 0)
    : serviceMonitors;

  return (
    <div className="space-y-6">
      {accessDenied && (
        <Card>
          <CardHeader className="flex flex-row items-center gap-2">
            <Activity className="w-5 h-5 text-warning" />
            <h3 className="text-lg font-semibold">监控权限</h3>
          </CardHeader>
          <CardBody>
            <div className="text-sm text-default-600">{accessDenied}</div>
            <div className="text-xs text-default-500 mt-2">
              如需使用监控功能，请联系管理员在用户页面授予监控权限。
            </div>
          </CardBody>
        </Card>
      )}

      {/* ====== GRID/LIST VIEW ====== */}
      {!accessDenied && !detailNodeId && (
        <>
          <div className="flex flex-wrap items-center gap-3 mb-1">
            <div className="flex items-center gap-2 text-xs text-default-500">
              <div className={`w-2 h-2 rounded-full ${wsConnected ? "bg-success" : wsConnecting ? "bg-warning" : "bg-default-300"}`} />
              <span>{wsConnected ? "实时已连接" : wsConnecting ? "实时连接中" : "实时未连接"}</span>
            </div>
            <Chip color="primary" size="sm" variant="flat">节点在线 {onlineNodes.length}/{nodes.length}</Chip>
            <Chip color="success" size="sm" variant="flat">监控 成功 {monitorSummary.ok} / 失败 {monitorSummary.fail}</Chip>
            {monitorSummary.stale > 0 && <Chip color="warning" size="sm" variant="flat">陈旧 {monitorSummary.stale}</Chip>}
          </div>

          {viewMode === "grid" ? (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
              {nodes.map((node) => {
                const metric = realtimeNodeMetrics[node.id] || null;
                return (
                  <ServerCard
                    key={node.id}
                    node={node}
                    metric={metric}
                    onPress={() => {
                      setDetailNodeId(node.id);
                      setSelectedNodeId(node.id);
                    }}
                  />
                );
              })}
            </div>
          ) : (
            <Card className="w-full">
              <Table
                aria-label="节点列表"
                className="overflow-x-auto min-w-full"
                classNames={{
                  wrapper: "bg-transparent p-0 shadow-none border-none overflow-hidden rounded-2xl",
                  th: "bg-transparent text-default-600 font-semibold text-sm border-b border-white/20 dark:border-white/10 py-3 uppercase tracking-wider first:rounded-tl-[24px] last:rounded-tr-[24px]",
                  td: "py-3 border-b border-divider/50 group-data-[last=true]:border-b-0",
                  tr: "hover:bg-white/40 dark:hover:bg-white/10 transition-colors",
                }}
              >
                <TableHeader>
                  <TableColumn>状态</TableColumn>
                  <TableColumn>名称</TableColumn>
                  <TableColumn>速率</TableColumn>
                  <TableColumn>流量</TableColumn>
                  <TableColumn>开机时长</TableColumn>
                  <TableColumn>连接数</TableColumn>
                  <TableColumn>CPU</TableColumn>
                  <TableColumn>RAM</TableColumn>
                  <TableColumn>存储</TableColumn>
                  <TableColumn align="center">操作</TableColumn>
                </TableHeader>
                <TableBody emptyContent="暂无节点">
                  {nodes.map((node) => {
                    const metric = realtimeNodeMetrics[node.id] || null;
                    const isOnline = node.connectionStatus === "online";

                    return (
                      <TableRow key={node.id}>
                        <TableCell>
                          <div className={`w-2 h-2 rounded-full ml-1 ${isOnline ? "bg-success" : "bg-danger"}`} />
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <DistroIcon distro={parseDistroFromVersion(node.version)} className="w-4 h-4 flex-shrink-0" style={{ color: isOnline ? getDistroColor(parseDistroFromVersion(node.version)) : undefined }} />
                            <span className="font-semibold text-sm whitespace-nowrap">{node.name}</span>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex flex-col gap-2 py-1 text-xs whitespace-nowrap">
                            <div className="flex items-center gap-1.5 font-mono text-success-500">
                              <span className="w-[86px] text-right inline-block">{isOnline && metric ? formatBytesPerSecond(metric.netOutSpeed) : "-"}</span>
                              <div className="flex items-center justify-center p-[3px] rounded-full bg-success-50 dark:bg-success-500/10 text-success-500">
                                <ArrowUp className="w-3 h-3" strokeWidth={2.5} />
                              </div>
                            </div>
                            <div className="flex items-center gap-1.5 font-mono text-primary-500">
                              <span className="w-[86px] text-right inline-block">{isOnline && metric ? formatBytesPerSecond(metric.netInSpeed) : "-"}</span>
                              <div className="flex items-center justify-center p-[3px] rounded-full bg-primary-50 dark:bg-primary-500/10 text-primary-500">
                                <ArrowDown className="w-3 h-3" strokeWidth={2.5} />
                              </div>
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex flex-col gap-2 py-1 text-xs whitespace-nowrap">
                            <div className="flex items-center gap-1.5 font-mono text-default-600">
                              <span className="w-[86px] text-right inline-block">{isOnline && metric ? formatBytes(metric.netOutBytes) : "-"}</span>
                              <div className="flex items-center justify-center p-[3px] rounded-full bg-default-100 text-default-500 dark:bg-default-100/50">
                                <ArrowUp className="w-3 h-3" strokeWidth={2.5} />
                              </div>
                            </div>
                            <div className="flex items-center gap-1.5 font-mono text-default-600">
                              <span className="w-[86px] text-right inline-block">{isOnline && metric ? formatBytes(metric.netInBytes) : "-"}</span>
                              <div className="flex items-center justify-center p-[3px] rounded-full bg-default-100 text-default-500 dark:bg-default-100/50">
                                <ArrowDown className="w-3 h-3" strokeWidth={2.5} />
                              </div>
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>
                          <span className="font-mono text-xs text-default-500 whitespace-nowrap">
                            {isOnline && metric ? formatUptime(metric.uptime) : "-"}
                          </span>
                        </TableCell>
                        <TableCell>
                          <div className="flex flex-col gap-1.5 text-xs font-mono text-default-500">
                            <div>TCP {isOnline && metric ? metric.tcpConns : "-"}</div>
                            <div>UDP {isOnline && metric ? metric.udpConns : "-"}</div>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            {isOnline && metric ? (
                              <Progress className="w-[40px] md:w-[60px]" color={getColorByUsage(metric.cpuUsage)} size="sm" value={metric.cpuUsage} />
                            ) : (
                              <div className="w-[40px] md:w-[60px] h-2 rounded-full bg-default-100" />
                            )}
                            <span className="text-xs font-mono w-9 text-right text-default-500">{isOnline && metric ? `${metric.cpuUsage.toFixed(1)}%` : "-"}</span>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            {isOnline && metric ? (
                              <Progress className="w-[40px] md:w-[60px]" color={getColorByUsage(metric.memoryUsage)} size="sm" value={metric.memoryUsage} />
                            ) : (
                              <div className="w-[40px] md:w-[60px] h-2 rounded-full bg-default-100" />
                            )}
                            <span className="text-xs font-mono w-9 text-right text-default-500">{isOnline && metric ? `${metric.memoryUsage.toFixed(1)}%` : "-"}</span>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            {isOnline && metric ? (
                              <Progress className="w-[40px] md:w-[60px]" color={getColorByUsage(metric.diskUsage)} size="sm" value={metric.diskUsage} />
                            ) : (
                              <div className="w-[40px] md:w-[60px] h-2 rounded-full bg-default-100" />
                            )}
                            <span className="text-xs font-mono w-9 text-right text-default-500">{isOnline && metric ? `${metric.diskUsage.toFixed(1)}%` : "-"}</span>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex justify-center">
                            <Button
                              isIconOnly
                              size="sm"
                              variant="light"
                              onPress={() => {
                                setDetailNodeId(node.id);
                                setSelectedNodeId(node.id);
                              }}
                            >
                              <Eye className="w-4 h-4 text-default-500" />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </Card>
          )}
        </>
      )}

      {/* ====== DETAIL VIEW ====== */}
      {!accessDenied && detailNodeId && (
        <>
          {/* Header */}
          <div className="flex items-center gap-3 flex-wrap">
            <Button size="sm" variant="flat" onPress={() => setDetailNodeId(null)}>
              <ArrowLeft className="w-4 h-4 mr-1" />
              返回总览
            </Button>
            <div className="flex items-center gap-2">
              <Server className={`w-5 h-5 ${detailNode?.connectionStatus === "online" ? "text-success" : "text-default-400"}`} />
              <h3 className="text-lg font-semibold">{detailNode?.name || `节点 #${detailNodeId}`}</h3>
              <Chip size="sm" color={detailNode?.connectionStatus === "online" ? "success" : "danger"} variant="flat">
                {detailNode?.connectionStatus === "online" ? "在线" : "离线"}
              </Chip>
            </div>
            <div className="flex items-center gap-2 text-xs text-default-500 ml-auto">
              <div className={`w-2 h-2 rounded-full ${wsConnected ? "bg-success" : wsConnecting ? "bg-warning" : "bg-default-300"}`} />
              <span>{wsConnected ? "实时已连接" : wsConnecting ? "实时连接中" : "实时未连接"}</span>
            </div>
          </div>

          {/* Realtime KPI cards */}
          {detailRealtimeMetric && (
            <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-3 pt-2">
              {[
                { label: "CPU", value: `${detailRealtimeMetric.cpuUsage.toFixed(1)}%`, color: getColorByUsage(detailRealtimeMetric.cpuUsage) },
                { label: "内存", value: `${detailRealtimeMetric.memoryUsage.toFixed(1)}%`, color: getColorByUsage(detailRealtimeMetric.memoryUsage) },
                { label: "磁盘", value: `${detailRealtimeMetric.diskUsage.toFixed(1)}%`, color: getColorByUsage(detailRealtimeMetric.diskUsage) },
                { label: "↓ 下行速度", value: formatBytesPerSecond(detailRealtimeMetric.netInSpeed), color: "success" as const },
                { label: "↑ 上行速度", value: formatBytesPerSecond(detailRealtimeMetric.netOutSpeed), color: "primary" as const },
                { label: "运行时间", value: formatUptime(detailRealtimeMetric.uptime), color: "default" as const },
              ].map((item) => (
                <Card key={item.label} className="hover:shadow-[0_15px_40px_rgba(0,0,0,0.15)] transition-shadow">
                  <CardBody className="py-3 px-4 flex flex-col items-center justify-center min-h-[5rem]">
                    <span className="text-[11px] text-default-500 mb-1.5">{item.label}</span>
                    <span className={`text-sm font-semibold font-mono ${item.color === 'danger' ? 'text-danger' : item.color === 'warning' ? 'text-warning' : item.color === 'success' ? 'text-success' : item.color === 'primary' ? 'text-primary' : ''}`}>
                      {item.value}
                    </span>
                  </CardBody>
                </Card>
              ))}
            </div>
          )}

          {/* Node metrics chart */}
          <NodeMetricsChartCard
            rangeMs={metricsRangeMs}
            onRangeChange={setMetricsRangeMs}
            activeMetricType={activeMetricType}
            onMetricTypeChange={setActiveMetricType}
            loading={metricsLoading}
            error={metricsError}
            truncated={metricsTruncated}
            maxRows={METRICS_MAX_ROWS}
            data={chartData}
            nodeId={selectedNodeId}
            onRefresh={loadMetrics}
          />


          {/* Service monitors chart – same style as node metrics */}
          {(() => {
            // Resolve which monitor is active
            const resolvedActiveMonitor =
              detailServiceMonitors.find((m) => m.id === activeServiceMonitorId) ||
              detailServiceMonitors[0] ||
              null;
            const resolvedActiveMonitorId = resolvedActiveMonitor?.id ?? null;
            const activeLatestResult = resolvedActiveMonitorId != null ? getLatestResult(resolvedActiveMonitorId) : null;
            const activeStale = resolvedActiveMonitor ? isResultStale(resolvedActiveMonitor, activeLatestResult) : false;
            const activeResults = resolvedActiveMonitorId != null ? (monitorResults[resolvedActiveMonitorId] || []) : [];
            const activeLatencyData = [...activeResults]
              .map((r) => ({
                time: formatTimestamp(r.timestamp, serviceMonitorRangeMs),
                latency: r.success === 1 ? r.latencyMs : null,
                success: r.success,
              }));

            return (
              <Card>
                <CardHeader className="flex flex-row items-center justify-between">
                  <h3 className="text-lg font-semibold">服务监控图表</h3>
                  <div className="flex items-center gap-2">
                    <Select
                      className="w-36"
                      selectedKeys={[String(serviceMonitorRangeMs)]}
                      onSelectionChange={(keys) => {
                        const v = Number(Array.from(keys)[0]);
                        if (v > 0) setServiceMonitorRangeMs(v);
                      }}
                    >
                      <SelectItem key={String(60 * 60 * 1000)}>1小时</SelectItem>
                      <SelectItem key={String(6 * 60 * 60 * 1000)}>6小时</SelectItem>
                      <SelectItem key={String(24 * 60 * 60 * 1000)}>24小时</SelectItem>
                    </Select>
                    {resolvedActiveMonitorId != null && (
                      <Button
                        size="sm"
                        variant="flat"
                        onPress={() => {
                          void loadMonitorResults(resolvedActiveMonitorId, { rangeMs: serviceMonitorRangeMs });
                        }}
                      >
                        <RefreshCw className="w-4 h-4 mr-1" />
                        刷新
                      </Button>
                    )}
                    <Button color="primary" size="sm" variant="flat" onPress={() => handleOpenEditModal()}>
                      <Plus className="w-4 h-4 mr-1" />
                      添加监控
                    </Button>
                  </div>
                </CardHeader>
                <CardBody className="space-y-4">
                  {monitorsLoading ? (
                    <div className="flex justify-center py-8"><RefreshCw className="w-6 h-6 animate-spin" /></div>
                  ) : detailServiceMonitors.length > 0 ? (
                    <>
                      {/* Monitor switcher buttons – like metric type tabs */}
                      <div className="flex flex-wrap gap-2">
                        {detailServiceMonitors.map((monitor) => {
                          const lr = getLatestResult(monitor.id);
                          const statusColor =
                            monitor.enabled !== 1 ? "default" :
                            !lr ? "default" :
                            lr.success === 1 ? "success" : "danger";
                          const isActive = monitor.id === resolvedActiveMonitorId;

                          return (
                            <Button
                              key={monitor.id}
                              color={isActive ? "primary" : statusColor as any}
                              size="sm"
                              variant={isActive ? "solid" : "flat"}
                              onPress={() => {
                                setActiveServiceMonitorId(monitor.id);
                                // Load results for this monitor if not loaded
                                if (!monitorResults[monitor.id] || monitorResults[monitor.id].length <= 1) {
                                  void loadMonitorResults(monitor.id, { rangeMs: serviceMonitorRangeMs });
                                }
                              }}
                            >
                              <div className={`w-2 h-2 rounded-full shrink-0 mr-1 ${
                                monitor.enabled !== 1 ? "bg-default-300" :
                                !lr ? "bg-default-400" :
                                lr.success === 1 ? (isActive ? "bg-white" : "bg-success") : (isActive ? "bg-white" : "bg-danger")
                              }`} />
                              {monitor.name}
                            </Button>
                          );
                        })}
                      </div>

                      {/* Active monitor info bar */}
                      {resolvedActiveMonitor && (
                        <div className="flex items-center justify-between gap-3 p-3 rounded-2xl bg-white/40 dark:bg-black/40 backdrop-blur-md border border-white/50 dark:border-white/10 shadow-[0_4px_12px_rgba(0,0,0,0.05)]">
                          <div className="flex items-center gap-3 min-w-0 flex-wrap">
                            <Chip size="sm" color="primary" variant="flat">{resolvedActiveMonitor.type.toUpperCase()}</Chip>
                            <span className="font-mono text-xs text-default-500">{resolvedActiveMonitor.target}</span>
                            <span className="text-xs text-default-500">每秒测试，30秒上报</span>
                            {activeLatestResult && Number.isFinite(activeLatestResult.latencyMs) ? (
                              <span className="font-mono text-xs font-semibold text-success">{activeLatestResult.latencyMs.toFixed(0)}ms</span>
                            ) : null}
                            {activeStale ? <Chip size="sm" color="warning" variant="flat">陈旧</Chip> : null}
                            {resolvedActiveMonitor.enabled !== 1 ? <Chip size="sm" color="default" variant="flat">已禁用</Chip> : null}
                          </div>
                          <Dropdown>
                            <DropdownTrigger>
                              <Button isIconOnly size="sm" variant="light"><MoreVertical className="w-4 h-4" /></Button>
                            </DropdownTrigger>
                            <DropdownMenu>
                              <DropdownItem startContent={<Play className="w-4 h-4" />} onPress={() => resolvedActiveMonitorId != null && handleRunMonitor(resolvedActiveMonitorId)}>立即检查</DropdownItem>
                              <DropdownItem startContent={<Activity className="w-4 h-4" />} onPress={() => resolvedActiveMonitorId != null && openResultsModal(resolvedActiveMonitorId)}>查看记录</DropdownItem>
                              <DropdownItem startContent={<Edit className="w-4 h-4" />} onPress={() => resolvedActiveMonitor && handleOpenEditModal(resolvedActiveMonitor)}>编辑</DropdownItem>
                              <DropdownItem className="text-danger" color="danger" startContent={<Trash2 className="w-4 h-4" />} onPress={() => resolvedActiveMonitorId != null && handleDeleteMonitor(resolvedActiveMonitorId)}>删除</DropdownItem>
                            </DropdownMenu>
                          </Dropdown>
                        </div>
                      )}

                      {/* Chart area – same h-64 as node metrics */}
                      {activeLatencyData.length > 0 ? (
                        <div className="h-64">
                          <ResponsiveContainer height="100%" width="100%">
                            <LineChart data={activeLatencyData}>
                              <CartesianGrid strokeDasharray="3 3" />
                              <XAxis dataKey="time" fontSize={12} />
                              <YAxis fontSize={12} tickFormatter={(v: number) => `${Math.round(v)}ms`} />
                              <Tooltip
                                contentStyle={{ backgroundColor: "rgba(0,0,0,0.8)", border: "none", borderRadius: "8px" }}
                                labelStyle={{ color: "#fff" }}
                                formatter={(value: unknown) => [`${Number(value).toFixed(0)}ms`, "延迟"]}
                              />
                              <Line dataKey="latency" dot={false} stroke="#10b981" strokeWidth={2} type="monotone" connectNulls={false} name="延迟" />
                            </LineChart>
                          </ResponsiveContainer>
                        </div>
                      ) : (
                        <div className="text-center py-8 text-default-500">暂无检查记录</div>
                      )}
                    </>
                  ) : (
                    <div className="text-center py-8 text-default-500">暂无服务监控，点击&quot;添加监控&quot;创建</div>
                  )}
                </CardBody>
              </Card>
            );
          })()}
        </>
      )}

      <Modal
        isOpen={resultsModalOpen}
        onClose={() => {
          setResultsModalOpen(false);
          setResultsMonitorId(null);
        }}
      >
        <ModalContent>
          <ModalHeader className="flex flex-row items-center justify-between gap-3">
            <div className="min-w-0">
              <div className="text-base font-semibold truncate">
                监控记录
                {resultsMonitor?.name ? ` - ${resultsMonitor.name}` : ""}
              </div>
              {resultsMonitor?.nodeId && resultsMonitor.nodeId > 0 ? (
                <div className="text-xs text-default-500 truncate">
                  节点:{" "}
                  {nodeMap.get(resultsMonitor.nodeId)?.name ||
                    resultsMonitor.nodeId}
                </div>
              ) : (
                <div className="text-xs text-default-500 truncate">
                  面板执行
                </div>
              )}

              {modalResults.length > 0 ? (
                <div className="text-xs text-default-500 truncate">
                  最新检查:{" "}
                  <span className="font-mono">
                    {formatDateTime(modalResults[0].timestamp)}
                  </span>
                </div>
              ) : null}
            </div>
            {resultsMonitorId != null ? (
              <div className="flex items-center gap-2 shrink-0">
                <Select
                  className="w-28"
                  selectedKeys={[String(resultsLimit)]}
                  onSelectionChange={(keys) => {
                    const v = Number(Array.from(keys)[0]);

                    if (v > 0) setResultsLimit(v);
                  }}
                >
                  <SelectItem key="20">20条</SelectItem>
                  <SelectItem key="50">50条</SelectItem>
                  <SelectItem key="100">100条</SelectItem>
                  <SelectItem key="200">200条</SelectItem>
                </Select>
                <Button
                  size="sm"
                  variant="flat"
                  onPress={() =>
                    resultsMonitorId != null &&
                    handleRunMonitor(resultsMonitorId)
                  }
                >
                  <Play className="w-4 h-4 mr-1" />
                  检查
                </Button>
                <Button
                  isLoading={resultsLoading}
                  size="sm"
                  variant="flat"
                  onPress={() => void loadResultsForModal()}
                >
                  <RefreshCw className="w-4 h-4 mr-1" />
                  刷新
                </Button>
              </div>
            ) : null}
          </ModalHeader>
          <ModalBody>
            {resultsLoading ? (
              <div className="flex justify-center py-10">
                <RefreshCw className="w-6 h-6 animate-spin" />
              </div>
            ) : modalResults.length > 0 ? (
              <Table
                aria-label="监控记录"
                className="w-full overflow-x-auto"
                classNames={{
                  wrapper: "bg-transparent p-0 shadow-none border-none overflow-hidden rounded-2xl",
                  th: "bg-transparent text-default-600 font-semibold text-sm border-b border-white/20 dark:border-white/10 py-3 uppercase tracking-wider first:rounded-tl-[24px] last:rounded-tr-[24px]",
                  td: "py-3 border-b border-divider/50 group-data-[last=true]:border-b-0",
                  tr: "hover:bg-white/40 dark:hover:bg-white/10 transition-colors",
                }}
              >
                <TableHeader>
                  <TableColumn>时间</TableColumn>
                  <TableColumn>结果</TableColumn>
                  <TableColumn>延迟</TableColumn>
                  <TableColumn>错误</TableColumn>
                </TableHeader>
                <TableBody>
                  {modalResults.map((r) => (
                    <TableRow key={r.id}>
                      <TableCell className="text-sm">
                        <span className="font-mono">
                          {formatDateTime(r.timestamp)}
                        </span>
                      </TableCell>
                      <TableCell>
                        {r.success === 1 ? (
                          <Chip color="success" size="sm" variant="flat">
                            成功
                          </Chip>
                        ) : (
                          <Chip color="danger" size="sm" variant="flat">
                            失败
                          </Chip>
                        )}
                      </TableCell>
                      <TableCell>
                        <span className="font-mono text-sm">
                          {Number.isFinite(r.latencyMs)
                            ? `${r.latencyMs.toFixed(0)}ms`
                            : "-"}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span
                          className={
                            r.errorMessage
                              ? "text-danger text-xs"
                              : "text-default-400 text-xs"
                          }
                        >
                          {r.errorMessage || "-"}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            ) : (
              <div className="text-center py-10 text-default-500">
                暂无监控记录
              </div>
            )}
          </ModalBody>
          <ModalFooter>
            <Button
              variant="flat"
              onPress={() => {
                setResultsModalOpen(false);
                setResultsMonitorId(null);
              }}
            >
              关闭
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      <Modal isOpen={editModalOpen} onClose={() => setEditModalOpen(false)}>
        <ModalContent>
          <ModalHeader>{editingMonitor ? "编辑监控" : "添加监控"}</ModalHeader>
          <ModalBody className="space-y-4">
            <Input
              label="名称"
              placeholder="例如: Google DNS"
              value={monitorForm.name}
              onChange={(e) =>
                setMonitorForm((f) => ({ ...f, name: e.target.value }))
              }
            />
            <Select
              label="类型"
              selectedKeys={[monitorForm.type]}
              onSelectionChange={(keys) => {
                const nextType = Array.from(keys)[0] as "tcp" | "icmp";

                setMonitorForm((f) => {
                  let nextNodeId = f.nodeId;

                  if (nextType === "icmp" && (!nextNodeId || nextNodeId <= 0)) {
                    nextNodeId = onlineNodes.length > 0 ? onlineNodes[0].id : 0;
                  }

                  return {
                    ...f,
                    type: nextType,
                    nodeId: nextNodeId,
                  };
                });
              }}
            >
              <SelectItem key="tcp">TCP</SelectItem>
              <SelectItem key="icmp">ICMP Ping</SelectItem>
            </Select>
            <Input
              label="目标"
              placeholder={
                monitorForm.type === "icmp"
                  ? "例如: 8.8.8.8 或 1.1.1.1"
                  : "例如: 8.8.8.8:53"
              }
              value={monitorForm.target}
              onChange={(e) =>
                setMonitorForm((f) => ({ ...f, target: e.target.value }))
              }
            />
            <div className="grid grid-cols-2 gap-4">
              <Input
                description={`最小 ${resolvedServiceMonitorLimits.minIntervalSec}s（扫描周期 ${resolvedServiceMonitorLimits.checkerScanIntervalSec}s）`}
                errorMessage={
                  monitorForm.intervalSec <
                  resolvedServiceMonitorLimits.minIntervalSec
                    ? `不能小于 ${resolvedServiceMonitorLimits.minIntervalSec}s`
                    : undefined
                }
                isInvalid={
                  monitorForm.intervalSec <
                  resolvedServiceMonitorLimits.minIntervalSec
                }
                label="检查间隔(秒)"
                type="number"
                value={String(monitorForm.intervalSec)}
                onChange={(e) =>
                  setMonitorForm((f) => ({
                    ...f,
                    intervalSec:
                      Number(e.target.value) ||
                      resolvedServiceMonitorLimits.defaultIntervalSec,
                  }))
                }
              />
              <Input
                description={`范围 ${resolvedServiceMonitorLimits.minTimeoutSec}-${resolvedServiceMonitorLimits.maxTimeoutSec}s`}
                errorMessage={
                  monitorForm.timeoutSec <
                    resolvedServiceMonitorLimits.minTimeoutSec ||
                  monitorForm.timeoutSec >
                    resolvedServiceMonitorLimits.maxTimeoutSec
                    ? `需在 ${resolvedServiceMonitorLimits.minTimeoutSec}-${resolvedServiceMonitorLimits.maxTimeoutSec}s 范围内`
                    : undefined
                }
                isInvalid={
                  monitorForm.timeoutSec <
                    resolvedServiceMonitorLimits.minTimeoutSec ||
                  monitorForm.timeoutSec >
                    resolvedServiceMonitorLimits.maxTimeoutSec
                }
                label="超时时间(秒)"
                type="number"
                value={String(monitorForm.timeoutSec)}
                onChange={(e) =>
                  setMonitorForm((f) => ({
                    ...f,
                    timeoutSec:
                      Number(e.target.value) ||
                      resolvedServiceMonitorLimits.defaultTimeoutSec,
                  }))
                }
              />
            </div>
            <Select
              label="执行节点"
              selectedKeys={
                monitorForm.type === "icmp"
                  ? monitorForm.nodeId
                    ? [String(monitorForm.nodeId)]
                    : []
                  : monitorForm.nodeId
                    ? [String(monitorForm.nodeId)]
                    : ["0"]
              }
              onSelectionChange={(keys) =>
                setMonitorForm((f) => ({
                  ...f,
                  nodeId: Number(Array.from(keys)[0]),
                }))
              }
            >
              {monitorForm.type !== "icmp" && (
                <SelectItem key="0">面板执行</SelectItem>
              )}
              {onlineNodes.map((node) => (
                <SelectItem key={String(node.id)}>{node.name}</SelectItem>
              ))}
            </Select>
            <div className="flex items-center gap-2">
              <Switch
                isSelected={monitorForm.enabled}
                onValueChange={(v) =>
                  setMonitorForm((f) => ({ ...f, enabled: v }))
                }
              />
              <span className="text-sm">启用</span>
            </div>
          </ModalBody>
          <ModalFooter>
            <Button variant="flat" onPress={() => setEditModalOpen(false)}>
              取消
            </Button>
            <Button
              color="primary"
              isLoading={submitLoading}
              onPress={handleSubmitMonitor}
            >
              {editingMonitor ? "更新" : "创建"}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
