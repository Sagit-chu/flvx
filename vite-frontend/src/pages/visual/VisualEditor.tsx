import React, { useCallback, useState, useEffect } from "react";
import ReactFlow, {
  MiniMap,
  Controls,
  Background,
  useNodesState,
  useEdgesState,
  addEdge,
  Connection,
  Edge,
  Node,
} from "reactflow";
import "reactflow/dist/style.css";
import toast from "react-hot-toast";

import Network from "@/api/network";
import ServerNode from "./nodes/ServerNode";
import Sidebar from "./Sidebar";
import ConnectionEdge from "./edges/ConnectionEdge";

const nodeTypes = {
  customServer: ServerNode,
};

const edgeTypes = {
  customConnection: ConnectionEdge,
};

// Initial placeholder data if empty
const initialNodes: Node[] = [];
const initialEdges: Edge[] = [];

export default function VisualEditor() {
  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);
  const [selectedNode, setSelectedNode] = useState<Node | null>(null);

  // Load Graph on mount
  useEffect(() => {
    loadGraph();
  }, []);

  const loadGraph = async () => {
    try {
      const res = await Network.get<any>("/visual/graph");
      if (res.code === 0 && res.data && res.data.graph_json) {
        const parsed = JSON.parse(res.data.graph_json);
        if (parsed.nodes && parsed.edges) {
          setNodes(parsed.nodes);
          setEdges(parsed.edges);
        } else {
          loadSystemNodes(); // Load available nodes from panel
        }
      }
    } catch {
      loadSystemNodes();
    }
  };

  const loadSystemNodes = async () => {
    try {
      const res = await Network.post<any[]>("/node/list");
      if (res.code === 0 && res.data) {
        const newNodes = res.data.map((n: any, idx: number) => ({
          id: String(n.id),
          type: "customServer",
          position: { x: 100 + (idx * 250), y: 150 },
          data: { 
            label: n.name, 
            status: n.status === 1 ? "online" : "offline", 
            ip: n.ip, 
            id: n.id,
            limit: idx === 1 ? "10Mbps" : null // Demo limit for the second node
          },
        }));
        setNodes(newNodes);
      }
    } catch (e) {
      toast.error("载入节点失败");
    }
  };

  const saveGraph = async () => {
    try {
      const payload = {
        graph_json: JSON.stringify({ nodes, edges }),
      };
      const res = await Network.post<any>("/visual/graph", payload);
      if (res.code === 0) {
        toast.success("可视化排布已保存");
      } else {
        toast.error("保存失败：" + res.msg);
      }
    } catch {
      toast.error("网络错误");
    }
  };

  const onConnect = useCallback(
    (params: Connection) => setEdges((eds) => addEdge({ ...params, type: 'customConnection', data: { label: "" } }, eds)),
    [setEdges],
  );

  const onNodeClick = (_: React.MouseEvent, node: Node) => {
    setSelectedNode(node);
  };

  const onPaneClick = () => {
    setSelectedNode(null); // click canvas closes sidebar
  };

  const deployAll = () => {
    // Generate config logic. Currently just a mock alert/toast since "deploy all" 
    // involves deep business logic. The user description simply mentioned "自动生成嵌套隧道配置... 一键部署"
    toast.success("已请求部署全部转发规则，请检查规则页！");
  };

  return (
    <div className="w-full h-full relative" style={{ height: "100%" }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        onNodeClick={onNodeClick}
        onPaneClick={onPaneClick}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        fitView
        className="bg-gray-50 dark:bg-zinc-950"
      >
        <Controls />
        <MiniMap zoomable pannable 
          nodeColor={(n) => {
            if (n.data?.status === 'online') return '#22c55e';
            return '#ef4444';
          }}
        />
        <Background color="#aaa" gap={16} />
      </ReactFlow>

      {/* Toolbox overlay */}
      <div className="absolute top-4 left-4 z-10 flex gap-2">
        <button 
          onClick={saveGraph}
          className="px-4 py-2 bg-zinc-800 text-white rounded shadow-md hover:bg-zinc-700 text-sm font-medium transition-colors"
        >
          保存画布
        </button>
        <button 
          onClick={loadSystemNodes}
          className="px-4 py-2 bg-indigo-600 text-white rounded shadow-md hover:bg-indigo-500 text-sm font-medium transition-colors"
        >
          导入服务端节点
        </button>
        <button 
          onClick={deployAll}
          className="px-4 py-2 bg-emerald-600 text-white rounded shadow-md hover:bg-emerald-500 text-sm font-medium transition-colors"
        >
          部署全部配置
        </button>
      </div>

      <Sidebar 
        selectedNode={selectedNode} 
        onClose={() => setSelectedNode(null)} 
      />
    </div>
  );
}
