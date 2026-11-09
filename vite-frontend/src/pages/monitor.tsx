import type { MonitorNodeApiItem } from "@/api/types";
import type { RealtimeNodeMetric } from "@/pages/node/monitor-view";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import toast from "react-hot-toast";
import {
  RefreshCw,
  LayoutGrid,
  List,
  Server,
  ArrowRightLeft,
} from "lucide-react";

import { AnimatedPage } from "@/components/animated-page";
import { Button } from "@/shadcn-bridge/heroui/button";
import { Card, CardBody, CardHeader } from "@/shadcn-bridge/heroui/card";
import { getMonitorNodes } from "@/api";
import { MonitorView } from "@/pages/node/monitor-view";
import { TunnelMonitorView } from "@/pages/node/tunnel-monitor-view";
import { useNodeRealtime } from "@/pages/node/use-node-realtime";

type MonitorNode = {
  id: number;
  name: string;
  connectionStatus: "online" | "offline";
  version?: string;
};

type MonitorTab = "nodes" | "tunnels";

const formatBytesPerSecond = (bytesPerSecond: number): string => {
  if (!Number.isFinite(bytesPerSecond) || bytesPerSecond <= 0) return "0 B/s";

  const k = 1024;
  const sizes = ["B/s", "KB/s", "MB/s", "GB/s", "TB/s"];
  const i = Math.floor(Math.log(bytesPerSecond) / Math.log(k));

  return `${parseFloat((bytesPerSecond / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`;
};

export default function MonitorPage() {
  const [nodes, setNodes] = useState<MonitorNodeApiItem[]>([]);
  const [nodesLoading, setNodesLoading] = useState(false);
  const [nodesError, setNodesError] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<"list" | "grid">("list");
  const [activeTab, setActiveTab] = useState<MonitorTab>("nodes");

  const realtimeNodeStatusRef = useRef(new Map<number, { status: number }>());
  const nodesRequestIdRef = useRef(0);
  const [realtimeNodeMetrics, setRealtimeNodeMetrics] = useState<
    Record<number, RealtimeNodeMetric>
  >({});

  const updateNodeStatus = useCallback((nodeId: number, status: number) => {
    realtimeNodeStatusRef.current.set(nodeId, { status });
    setNodes((prev) =>
      prev.map((node) =>
        Number(node.id) === nodeId ? { ...node, status } : node,
      ),
    );
  }, []);

  const handleRealtimeMessage = useCallback(
    (message: any) => {
      const nodeId = Number(message?.id ?? 0);

      if (!nodeId || Number.isNaN(nodeId)) return;

      const type = String(message?.type ?? "");
      const payload = message?.data;

      if (type === "status") {
        updateNodeStatus(nodeId, Number(payload) === 1 ? 1 : 0);

        return;
      }

      if (type === "info") {
        updateNodeStatus(nodeId, 1);

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
        if (!raw || typeof raw !== "object") return;
        const metric = raw as Record<string, unknown>;
        const receivedAt = Date.now();
        const incomingUptime = Number(metric.uptime ?? 0);

        setRealtimeNodeMetrics((prev) => ({
          ...prev,
          [nodeId]: {
            receivedAt,
            cpuUsage: Number(metric.cpuUsage ?? metric.cpu_usage ?? 0),
            memoryUsage: Number(metric.memoryUsage ?? metric.memory_usage ?? 0),
            diskUsage: Number(metric.diskUsage ?? metric.disk_usage ?? 0),
            netInBytes: Number(metric.netInBytes ?? metric.bytes_received ?? 0),
            netOutBytes: Number(
              metric.netOutBytes ?? metric.bytes_transmitted ?? 0,
            ),
            netInSpeed: Number(metric.netInSpeed ?? metric.net_in_speed ?? 0),
            netOutSpeed: Number(
              metric.netOutSpeed ?? metric.net_out_speed ?? 0,
            ),
            load1: Number(metric.load1 ?? 0),
            load5: Number(metric.load5 ?? 0),
            load15: Number(metric.load15 ?? 0),
            tcpConns: Number(metric.tcpConns ?? metric.tcp_conns ?? 0),
            udpConns: Number(metric.udpConns ?? metric.udp_conns ?? 0),
            uptime: incomingUptime || prev[nodeId]?.uptime || 0,
          },
        }));
        updateNodeStatus(nodeId, 1);
      }
    },
    [updateNodeStatus],
  );

  const { wsConnected, wsConnecting } = useNodeRealtime({
    onMessage: handleRealtimeMessage,
    enabled: true,
  });

  const loadNodes = useCallback(async (options?: { silent?: boolean }) => {
    const silent = options?.silent ?? false;
    const requestId = ++nodesRequestIdRef.current;
    const statusAtRequest = new Map(realtimeNodeStatusRef.current);

    if (!silent) setNodesLoading(true);
    try {
      const response = await getMonitorNodes();

      // A newer refresh or poll owns the page state, even if it finishes first.
      if (requestId !== nodesRequestIdRef.current) return;

      if (response.code === 0 && Array.isArray(response.data)) {
        setNodesError(null);
        // A fresh snapshot repairs missed WS events. Events received while the
        // request was in flight are newer than that snapshot and must win.
        setNodes(
          response.data.map((node) => {
            const nodeId = Number(node.id);
            const latest = realtimeNodeStatusRef.current.get(nodeId);

            return latest && latest !== statusAtRequest.get(nodeId)
              ? { ...node, status: latest.status }
              : node;
          }),
        );

        return;
      }

      if (response.code === 403) {
        setNodes([]);
        setNodesError(response.msg || "暂无监控权限，请联系管理员授权");

        return;
      }

      if (!silent) toast.error(response.msg || "加载节点失败");
    } catch {
      if (requestId === nodesRequestIdRef.current && !silent) {
        toast.error("加载节点失败");
      }
    } finally {
      // A silent poll may supersede a foreground refresh that showed loading.
      if (requestId === nodesRequestIdRef.current) setNodesLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadNodes();

    return () => {
      nodesRequestIdRef.current += 1;
    };
  }, [loadNodes]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      void loadNodes({ silent: true });
    }, 30_000);

    return () => window.clearInterval(timer);
  }, [loadNodes]);

  const nodeMap = useMemo(() => {
    const list: MonitorNode[] = nodes
      .filter((n) => Number(n.id) > 0)
      .map((n) => ({
        id: Number(n.id),
        name: String(n.name ?? ""),
        connectionStatus: n.status === 1 ? "online" : "offline",
        version: n.version,
      }));

    return new Map<number, MonitorNode>(list.map((n) => [n.id, n]));
  }, [nodes]);

  const aggregateMetrics = useMemo(() => {
    let totalCpu = 0;
    let totalConns = 0;
    let totalNetIn = 0;
    let totalNetOut = 0;
    let onlineCount = 0;

    nodeMap.forEach((node) => {
      if (node.connectionStatus === "online") {
        onlineCount++;
        const metric = realtimeNodeMetrics[node.id];

        if (metric) {
          totalCpu += metric.cpuUsage;
          totalConns += metric.tcpConns + metric.udpConns;
          totalNetIn += metric.netInSpeed;
          totalNetOut += metric.netOutSpeed;
        }
      }
    });

    const avgCpu = onlineCount > 0 ? totalCpu / onlineCount : 0;

    return {
      avgCpu,
      totalConns,
      totalBandwidth: totalNetIn + totalNetOut,
    };
  }, [nodeMap, realtimeNodeMetrics]);

  return (
    <AnimatedPage className="px-3 lg:px-6 py-8">
      {/* 顶部英雄数据指标 (Hero Metrics) */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-8">
        <div className="rounded-3xl border border-white/80 dark:border-white/10 bg-white/20 dark:bg-zinc-900/20 backdrop-blur-3xl shadow-[0_10px_30px_rgba(0,0,0,0.1)] p-6 relative overflow-hidden flex flex-col justify-between h-40">
          <div className="flex justify-between items-center z-10 relative">
            <span className="text-default-600 font-medium text-sm">
              System Load
            </span>
          </div>
          <div className="z-10 relative">
            <span className="text-4xl font-bold text-foreground">
              {aggregateMetrics.avgCpu.toFixed(1)}%
            </span>
          </div>
          <div className="absolute bottom-0 left-0 right-0 h-12 flex items-end gap-1 px-6 pb-4 opacity-50 z-0">
            <div className="w-full bg-primary/40 h-2 rounded-t-sm" />
            <div className="w-full bg-primary/40 h-3 rounded-t-sm" />
            <div className="w-full bg-primary/40 h-1.5 rounded-t-sm" />
            <div className="w-full bg-primary/40 h-4 rounded-t-sm" />
            <div className="w-full bg-primary/40 h-2.5 rounded-t-sm" />
            <div className="w-full bg-primary h-5 rounded-t-sm shadow-[0_0_10px_rgba(0,122,255,0.5)]" />
          </div>
        </div>

        <div className="rounded-3xl border border-white/80 dark:border-white/10 bg-white/20 dark:bg-zinc-900/20 backdrop-blur-3xl shadow-[0_10px_30px_rgba(0,0,0,0.1)] p-6 relative overflow-hidden flex flex-col justify-between h-40">
          <div className="flex justify-between items-center z-10 relative">
            <span className="text-default-600 font-medium text-sm">
              Active Connections
            </span>
          </div>
          <div className="z-10 relative">
            <span className="text-4xl font-bold text-foreground">
              {aggregateMetrics.totalConns}
            </span>
          </div>
          <div className="absolute bottom-0 left-0 right-0 h-12 flex items-end gap-1 px-6 pb-4 opacity-50 z-0">
            <div className="w-full bg-success/40 h-3.5 rounded-t-sm" />
            <div className="w-full bg-success/40 h-4.5 rounded-t-sm" />
            <div className="w-full bg-success/40 h-2.5 rounded-t-sm" />
            <div className="w-full bg-success/40 h-6 rounded-t-sm" />
            <div className="w-full bg-success/40 h-5.5 rounded-t-sm" />
            <div className="w-full bg-success h-7 rounded-t-sm shadow-[0_0_10px_rgba(52,199,89,0.5)]" />
          </div>
        </div>

        <div className="rounded-3xl border border-white/80 dark:border-white/10 bg-white/20 dark:bg-zinc-900/20 backdrop-blur-3xl shadow-[0_10px_30px_rgba(0,0,0,0.1)] p-6 relative overflow-hidden flex flex-col justify-between h-40">
          <div className="flex justify-between items-center z-10 relative">
            <span className="text-default-600 font-medium text-sm">
              Bandwidth
            </span>
          </div>
          <div className="z-10 relative">
            <span className="text-4xl font-bold text-foreground">
              {formatBytesPerSecond(aggregateMetrics.totalBandwidth)}
            </span>
          </div>
          <div className="absolute bottom-0 left-0 right-0 h-12 flex items-end gap-1 px-6 pb-4 opacity-50 z-0">
            <div className="w-full bg-secondary/40 h-2 rounded-t-sm" />
            <div className="w-full bg-secondary/40 h-3 rounded-t-sm" />
            <div className="w-full bg-secondary/40 h-5 rounded-t-sm" />
            <div className="w-full bg-secondary/40 h-8 rounded-t-sm" />
            <div className="w-full bg-secondary/40 h-7 rounded-t-sm" />
            <div className="w-full bg-secondary h-10 rounded-t-sm shadow-[0_0_10px_rgba(175,82,222,0.5)]" />
          </div>
        </div>
      </div>

      <div className="mb-6 space-y-3">
        <div className="flex items-center justify-between gap-3">
          <div className="min-w-0">
            <h2 className="text-xl font-semibold truncate">监控面板</h2>
          </div>
          <div className="flex items-center gap-2">
            <Button
              isIconOnly
              size="sm"
              variant="flat"
              onPress={() => setViewMode(viewMode === "list" ? "grid" : "list")}
            >
              {viewMode === "list" ? (
                <LayoutGrid className="w-4 h-4" />
              ) : (
                <List className="w-4 h-4" />
              )}
            </Button>
            {activeTab === "nodes" && (
              <Button
                isLoading={nodesLoading}
                size="sm"
                variant="flat"
                onPress={() => loadNodes()}
              >
                <RefreshCw className="w-4 h-4 mr-1" />
                刷新节点
              </Button>
            )}
          </div>
        </div>

        {/* Tab Switcher */}
        <div className="flex items-center gap-1 p-1 rounded-2xl bg-white/20 dark:bg-black/20 backdrop-blur-3xl border border-white/50 dark:border-white/10 w-fit shadow-sm">
          <button
            className={`flex items-center gap-1.5 px-5 py-2 rounded-xl text-sm font-semibold transition-all duration-200 ${
              activeTab === "nodes"
                ? "bg-white dark:bg-zinc-800 shadow-[0_2px_8px_rgba(0,0,0,0.08)] text-foreground"
                : "text-default-500 hover:text-foreground"
            }`}
            onClick={() => setActiveTab("nodes")}
          >
            <Server className="w-4 h-4" />
            节点
          </button>
          <button
            className={`flex items-center gap-1.5 px-5 py-2 rounded-xl text-sm font-semibold transition-all duration-200 ${
              activeTab === "tunnels"
                ? "bg-white dark:bg-zinc-800 shadow-[0_2px_8px_rgba(0,0,0,0.08)] text-foreground"
                : "text-default-500 hover:text-foreground"
            }`}
            onClick={() => setActiveTab("tunnels")}
          >
            <ArrowRightLeft className="w-4 h-4" />
            隧道
          </button>
        </div>

        {nodesError && activeTab === "nodes" ? (
          <Card>
            <CardHeader>
              <h3 className="text-sm font-semibold">节点列表</h3>
            </CardHeader>
            <CardBody>
              <div className="text-sm text-default-600">{nodesError}</div>
            </CardBody>
          </Card>
        ) : null}
      </div>

      {activeTab === "nodes" ? (
        <MonitorView
          nodeMap={nodeMap}
          realtimeNodeMetrics={realtimeNodeMetrics}
          viewMode={viewMode}
          wsConnected={wsConnected}
          wsConnecting={wsConnecting}
        />
      ) : (
        <TunnelMonitorView viewMode={viewMode} />
      )}
    </AnimatedPage>
  );
}
