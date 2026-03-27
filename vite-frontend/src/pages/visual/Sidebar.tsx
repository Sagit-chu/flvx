import { useEffect, useState } from "react";
import { Node } from "reactflow";

import Network from "@/api/network";

interface SidebarProps {
  selectedNode: Node | null;
  onClose: () => void;
}

export default function Sidebar({ selectedNode, onClose }: SidebarProps) {
  const [loading, setLoading] = useState(false);
  const [nodeData, setNodeData] = useState<any>(null);

  useEffect(() => {
    if (!selectedNode) {
      setNodeData(null);
      return;
    }
    void loadNodeData(selectedNode.id);
  }, [selectedNode]);

  const loadNodeData = async (id: string) => {
    setLoading(true);
    try {
      const res = await Network.post<any>(`/probe/node/${id}`);
      if (res.code === 0 && res.data) {
        setNodeData(res.data);
      } else {
        setNodeData(null);
      }
    } catch {
      setNodeData(null);
    } finally {
      setLoading(false);
    }
  };

  if (!selectedNode) {
    return null;
  }

  return (
    <div className="absolute right-0 top-0 z-20 flex h-full w-80 flex-col overflow-hidden border-l border-zinc-200 bg-white/95 shadow-2xl backdrop-blur-xl transition-transform duration-300 dark:border-zinc-800 dark:bg-black/95">
      <div className="sticky top-0 z-10 flex items-center justify-between border-b border-zinc-200 bg-white/50 p-4 backdrop-blur dark:border-zinc-800 dark:bg-zinc-950/50">
        <h2 className="flex items-center gap-2 text-lg font-bold text-zinc-800 dark:text-zinc-100">
          {String(selectedNode.data?.label ?? selectedNode.id)}
          <span
            className={`h-2.5 w-2.5 rounded-full ${nodeData?.status === "online" ? "bg-emerald-500" : "bg-rose-500"}`}
          />
        </h2>
        <button
          className="text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200"
          onClick={onClose}
        >
          <svg
            className="h-5 w-5"
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
        </button>
      </div>

      <div className="custom-scrollbar flex-1 overflow-y-auto p-4">
        {loading ? (
          <div className="animate-pulse space-y-4">
            <div className="h-8 rounded bg-zinc-200 dark:bg-zinc-800"></div>
            <div className="h-24 rounded bg-zinc-200 dark:bg-zinc-800"></div>
            <div className="h-16 rounded bg-zinc-200 dark:bg-zinc-800"></div>
          </div>
        ) : nodeData ? (
          <div className="space-y-6">
            <div className="grid grid-cols-2 gap-3">
              <div className="rounded-xl border border-zinc-200 bg-zinc-100 p-3 shadow-sm dark:border-zinc-800 dark:bg-zinc-900">
                <span className="mb-1 block text-xs font-semibold text-zinc-500">
                  CPU
                </span>
                <div className="font-mono text-xl text-zinc-800 dark:text-zinc-100">
                  {nodeData.cpu_usage ? nodeData.cpu_usage.toFixed(1) : "0"}
                  <span className="ml-1 text-xs text-zinc-500">%</span>
                </div>
              </div>
              <div className="rounded-xl border border-zinc-200 bg-zinc-100 p-3 shadow-sm dark:border-zinc-800 dark:bg-zinc-900">
                <span className="mb-1 block text-xs font-semibold text-zinc-500">
                  内存
                </span>
                <div className="font-mono text-xl text-zinc-800 dark:text-zinc-100">
                  {nodeData.mem_usage ? nodeData.mem_usage.toFixed(1) : "0"}
                  <span className="ml-1 text-xs text-zinc-500">%</span>
                </div>
              </div>
              <div className="col-span-2 flex items-center justify-between rounded-xl border border-zinc-200 bg-zinc-100 p-3 shadow-sm dark:border-zinc-800 dark:bg-zinc-900">
                <span className="text-xs font-semibold text-zinc-500">
                  连接数
                </span>
                <div className="font-mono text-xl text-zinc-800 dark:text-zinc-100">
                  {nodeData.connections || 0}
                </div>
              </div>
            </div>

            <div className="rounded-xl border border-zinc-200 bg-zinc-100 p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900">
              <h3 className="mb-2 border-b border-zinc-200 pb-2 text-sm font-bold text-zinc-700 dark:border-zinc-800 dark:text-zinc-300">
                端口分配
              </h3>
              <p className="mb-2 text-xs text-zinc-500">
                可用范围: {nodeData.allowed_ports || nodeData.allowed_port || "暂无"}
              </p>
              <div className="flex flex-wrap gap-1">
                {nodeData.occupied_ports?.length ? (
                  nodeData.occupied_ports.map((port: number) => (
                    <span
                      key={port}
                      className="rounded bg-indigo-100 px-1.5 py-0.5 text-[10px] text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300"
                    >
                      {port}
                    </span>
                  ))
                ) : (
                  <span className="text-xs italic text-zinc-500">
                    无占用端口
                  </span>
                )}
              </div>
            </div>

            <div className="h-52 rounded-xl border border-zinc-200 bg-zinc-100 p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900">
              <h3 className="mb-4 text-sm font-bold text-zinc-700 dark:text-zinc-300">
                实时连接分布
              </h3>
              <div className="flex h-24 items-end justify-center gap-6">
                <div className="flex h-full flex-col items-center justify-end gap-2">
                  <div
                    className="w-12 rounded-t-sm bg-indigo-500 transition-all duration-500"
                    style={{
                      height: `${nodeData.tcp_connections ? Math.min(100, Math.max(5, nodeData.tcp_connections)) : 5}%`,
                    }}
                  />
                  <span className="text-xs text-zinc-500">
                    TCP ({nodeData.tcp_connections || 0})
                  </span>
                </div>
                <div className="flex h-full flex-col items-center justify-end gap-2">
                  <div
                    className="w-12 rounded-t-sm bg-indigo-400 transition-all duration-500"
                    style={{
                      height: `${nodeData.udp_connections ? Math.min(100, Math.max(5, nodeData.udp_connections)) : 5}%`,
                    }}
                  />
                  <span className="text-xs text-zinc-500">
                    UDP ({nodeData.udp_connections || 0})
                  </span>
                </div>
              </div>
              <p className="mt-4 text-xs text-zinc-500">
                更新时间:{" "}
                {nodeData.metric_timestamp
                  ? new Date(nodeData.metric_timestamp).toLocaleString()
                  : "暂无指标"}
              </p>
            </div>
          </div>
        ) : (
          <div className="mt-10 text-center text-sm text-zinc-500">
            加载节点详情失败
          </div>
        )}
      </div>
    </div>
  );
}
