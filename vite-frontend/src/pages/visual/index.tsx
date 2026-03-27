import { ReactFlowProvider } from "reactflow";

import VisualEditor from "./VisualEditor";

export default function VisualPage() {
  return (
    <div className="w-full h-full p-4 flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">可视化节点配置 (Beta)</h1>
        <p className="text-sm text-gray-500">拖拽节点进行配置，支持多级连线诊断</p>
      </div>
      <div className="flex-1 min-h-[600px] border border-gray-200 dark:border-gray-800 rounded-lg overflow-hidden relative shadow-inner bg-gray-50 dark:bg-zinc-950">
        <ReactFlowProvider>
          <VisualEditor />
        </ReactFlowProvider>
      </div>
    </div>
  );
}
