import type { MonitorNodeApiItem } from "@/api/types";

import { useCallback, useEffect, useMemo, useState } from "react";
import toast from "react-hot-toast";
import { RefreshCw } from "lucide-react";

import { AnimatedPage } from "@/components/animated-page";
import { Button } from "@/shadcn-bridge/heroui/button";
import { Card, CardBody, CardHeader } from "@/shadcn-bridge/heroui/card";
import { getMonitorNodes } from "@/api";
import { MonitorView } from "@/pages/node/monitor-view";

type MonitorNode = {
  id: number;
  name: string;
  connectionStatus: "online" | "offline";
};

export default function MonitorPage() {
  const [nodes, setNodes] = useState<MonitorNodeApiItem[]>([]);
  const [nodesLoading, setNodesLoading] = useState(false);
  const [nodesError, setNodesError] = useState<string | null>(null);

  const loadNodes = useCallback(async (options?: { silent?: boolean }) => {
    const silent = options?.silent ?? false;
    if (!silent) setNodesLoading(true);
    try {
      const response = await getMonitorNodes();

      if (response.code === 0 && Array.isArray(response.data)) {
        setNodesError(null);
        setNodes(response.data);

        return;
      }

      if (response.code === 403) {
        setNodes([]);
        setNodesError(response.msg || "暂无监控权限，请联系管理员授权");

        return;
      }

      if (!silent) toast.error(response.msg || "加载节点失败");
    } catch {
      if (!silent) toast.error("加载节点失败");
    } finally {
      if (!silent) setNodesLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadNodes();
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
      }));

    return new Map<number, MonitorNode>(list.map((n) => [n.id, n]));
  }, [nodes]);

  return (
    <AnimatedPage className="px-3 lg:px-6 py-8">
      <div className="mb-6 space-y-3">
        <div className="flex items-center justify-between gap-3">
          <div className="min-w-0">
            <h2 className="text-xl font-semibold truncate">监控</h2>
            <div className="text-xs text-default-500 truncate">
              实时节点状态 + 历史指标图表 + 隧道流量 + 服务监控（TCP/ICMP）
            </div>
          </div>
          <Button
            isLoading={nodesLoading}
            size="sm"
            variant="flat"
            onPress={() => loadNodes()}
          >
            <RefreshCw className="w-4 h-4 mr-1" />
            刷新节点
          </Button>
        </div>

        {nodesError ? (
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

      <MonitorView nodeMap={nodeMap} />
    </AnimatedPage>
  );
}
