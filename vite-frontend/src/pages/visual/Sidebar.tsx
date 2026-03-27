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
    if (selectedNode) {
      loadNodeData(selectedNode.id);
    } else {
      setNodeData(null);
    }
  }, [selectedNode]);

  const loadNodeData = async (id: string) => {
    setLoading(true);
    try {
      const res = await Network.post<any>(`/visual/probe/${id}`);
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

  if (!selectedNode) return null;

  return (
    <div className="absolute top-0 right-0 h-full w-80 bg-white/95 dark:bg-black/95 backdrop-blur-xl shadow-2xl border-l border-zinc-200 dark:border-zinc-800 flex flex-col z-20 transition-transform duration-300 transform translate-x-0 overflow-hidden">
      <div className="flex items-center justify-between p-4 border-b border-zinc-200 dark:border-zinc-800 sticky top-0 bg-white/50 dark:bg-zinc-950/50 backdrop-blur z-10">
        <h2 className="text-lg font-bold text-zinc-800 dark:text-zinc-100 flex items-center gap-2">
          {selectedNode.data.label}
          <span className={`w-2.5 h-2.5 rounded-full ${nodeData?.status === 'online' ? 'bg-emerald-500' : 'bg-rose-500'}`} />
        </h2>
        <button onClick={onClose} className="text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200">
          <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>

      <div className="p-4 overflow-y-auto flex-1 custom-scrollbar">
        {loading ? (
          <div className="animate-pulse space-y-4">
            <div className="h-8 bg-zinc-200 dark:bg-zinc-800 rounded"></div>
            <div className="h-24 bg-zinc-200 dark:bg-zinc-800 rounded"></div>
            <div className="h-16 bg-zinc-200 dark:bg-zinc-800 rounded"></div>
          </div>
        ) : nodeData ? (
          <div className="space-y-6">
            
            {/* Health Stats Grid */}
            <div className="grid grid-cols-2 gap-3">
              <div className="bg-zinc-100 dark:bg-zinc-900 rounded-xl p-3 border border-zinc-200 dark:border-zinc-800 shadow-sm">
                <span className="text-xs text-zinc-500 font-semibold mb-1 block">CPU</span>
                <div className="text-xl text-zinc-800 dark:text-zinc-100 font-mono">
                  {nodeData.cpu_usage ? nodeData.cpu_usage.toFixed(1) : "0"} <span className="text-xs text-zinc-500">%</span>
                </div>
              </div>
              <div className="bg-zinc-100 dark:bg-zinc-900 rounded-xl p-3 border border-zinc-200 dark:border-zinc-800 shadow-sm">
                <span className="text-xs text-zinc-500 font-semibold mb-1 block">Memory</span>
                <div className="text-xl text-zinc-800 dark:text-zinc-100 font-mono">
                  {nodeData.mem_usage ? nodeData.mem_usage.toFixed(1) : "0"} <span className="text-xs text-zinc-500">%</span>
                </div>
              </div>
              <div className="bg-zinc-100 dark:bg-zinc-900 rounded-xl p-3 border border-zinc-200 dark:border-zinc-800 shadow-sm col-span-2 flex justify-between items-center">
                <span className="text-xs text-zinc-500 font-semibold inline-block">Connections</span>
                <div className="text-xl text-zinc-800 dark:text-zinc-100 font-mono inline-block">
                  {nodeData.connections || 0}
                </div>
              </div>
            </div>

            {/* Ports Info */}
            <div className="bg-zinc-100 dark:bg-zinc-900 rounded-xl border border-zinc-200 dark:border-zinc-800 p-4 shadow-sm">
              <h3 className="text-sm font-bold text-zinc-700 dark:text-zinc-300 mb-2 border-b border-zinc-200 dark:border-zinc-800 pb-2">探针 - 端口分配</h3>
              <p className="text-xs text-zinc-500 mb-2">已占用端口列表</p>
              <div className="flex flex-wrap gap-1">
                {nodeData.occupied_ports?.length ? (
                  nodeData.occupied_ports.map((p: number, i: number) => (
                    <span key={i} className="px-1.5 py-0.5 bg-indigo-100 text-indigo-700 dark:bg-indigo-900/40 dark:text-indigo-300 text-[10px] font-mono rounded">
                      {p}
                    </span>
                  ))
                ) : (
                  <span className="text-xs text-zinc-500 italic">空闲无占用</span>
                )}
              </div>
            </div>

            {/* Traffic Pseudo Chart */}
            <div className="bg-zinc-100 dark:bg-zinc-900 rounded-xl border border-zinc-200 dark:border-zinc-800 p-4 shadow-sm h-48">
              <h3 className="text-sm font-bold text-zinc-700 dark:text-zinc-300 mb-4">实时连接分布</h3>
              <div className="flex h-24 items-end gap-6 justify-center">
                <div className="flex flex-col items-center gap-2 h-full justify-end">
                  <div 
                    className="w-12 bg-indigo-500 rounded-t-sm transition-all duration-500" 
                    style={{ height: `${nodeData.connections ? Math.min(100, Math.max(5, nodeData.connections)) : 5}%` }} 
                  />
                  <span className="text-xs text-zinc-500">TCP ({nodeData.connections || 0})</span>
                </div>
                <div className="flex flex-col items-center gap-2 h-full justify-end">
                  <div 
                    className="w-12 bg-indigo-400 rounded-t-sm transition-all duration-500" 
                    style={{ height: '5%' }} 
                  />
                  <span className="text-xs text-zinc-500">UDP (0)</span>
                </div>
              </div>
            </div>

          </div>
        ) : (
          <div className="text-center text-sm text-zinc-500 mt-10">
            无法完全加载节点详情状态
          </div>
        )}
      </div>
    </div>
  );
}
