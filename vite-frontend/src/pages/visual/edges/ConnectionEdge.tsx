import { useState } from "react";
import { EdgeLabelRenderer, EdgeProps, getBezierPath } from "reactflow";
import toast from "react-hot-toast";

import { Button } from "@/shadcn-bridge/heroui/button";
import {
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  useDisclosure,
} from "@/shadcn-bridge/heroui/modal";
import Network from "@/api/network";

interface LinkTestResult {
  success: boolean;
  averageTime: number;
  packetLoss: number;
  message: string;
  ping_output: string;
  trace_output: string;
  trace_mode: string;
  simulated: boolean;
  targetHost: string;
  targetPort: number;
}

export default function ConnectionEdge({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  style = {},
  markerEnd,
  data,
  source,
  target,
}: EdgeProps) {
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
  });

  const { isOpen, onOpen, onOpenChange } = useDisclosure();
  const [hovered, setHovered] = useState(false);
  const [testResult, setTestResult] = useState<LinkTestResult | null>(null);
  const [loading, setLoading] = useState(false);

  const runLinkTest = async () => {
    const sourceNodeId = Number(source);
    const targetNodeId = Number(target);
    if (!Number.isInteger(sourceNodeId) || !Number.isInteger(targetNodeId)) {
      toast.error("此连线未连接到两个系统节点");
      return;
    }

    onOpen();
    setLoading(true);

    try {
      const res = await Network.post<LinkTestResult>("/link/test", {
        sourceNodeId,
        targetNodeId,
      });
      if (res.code === 0 && res.data) {
        setTestResult(res.data);
      } else {
        setTestResult(null);
        toast.error(res.msg || "链路测试失败");
      }
    } catch {
      setTestResult(null);
      toast.error("链路测试请求失败");
    } finally {
      setLoading(false);
    }
  };

  const onEdgeClick = (event?: { stopPropagation?: () => void }) => {
    event?.stopPropagation?.();
    void runLinkTest();
  };

  return (
    <>
      <path
        id={id}
        className={`react-flow__edge-path cursor-pointer ${hovered ? "stroke-indigo-400" : "stroke-zinc-600"}`}
        d={edgePath}
        markerEnd={markerEnd}
        onClick={onEdgeClick}
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        style={{
          ...style,
          strokeWidth: hovered ? 4 : 2,
          transition: "all 0.3s ease",
        }}
      />

      <path
        className="cursor-pointer"
        d={edgePath}
        fill="none"
        onClick={onEdgeClick}
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        stroke="transparent"
        strokeWidth={20}
      />

      <EdgeLabelRenderer>
        <div
          className="group"
          onClick={onEdgeClick}
          onMouseEnter={() => setHovered(true)}
          onMouseLeave={() => setHovered(false)}
          style={{
            position: "absolute",
            transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
            pointerEvents: "all",
          }}
        >
          <div
            className={`
              cursor-pointer select-none rounded-full border bg-zinc-900 px-3 py-1 text-[10px] font-bold uppercase tracking-wider transition-all duration-300
              ${
                hovered
                  ? "scale-110 border-indigo-500 text-indigo-400 shadow-[0_0_15px_rgba(99,102,241,0.4)]"
                  : "border-zinc-700 text-zinc-400 shadow-lg"
              }
            `}
          >
            {data?.label || "链路测试"}
          </div>
        </div>
      </EdgeLabelRenderer>

      <Modal
        backdrop="blur"
        classNames={{
          base: "rounded-2xl border border-white/10 bg-zinc-950",
          header: "border-b border-white/5",
          footer: "border-t border-white/5",
        }}
        isOpen={isOpen}
        onOpenChange={onOpenChange}
        size="2xl"
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1 p-6">
                <div className="flex items-center gap-2">
                  <div className="h-2 w-2 rounded-full bg-indigo-500 animate-pulse" />
                  <span className="bg-gradient-to-r from-zinc-100 to-zinc-400 bg-clip-text text-xl font-bold uppercase text-transparent">
                    链路诊断报告
                  </span>
                </div>
                <p className="mt-1 text-xs font-mono text-zinc-500">
                  路径: {source} <span className="mx-1 text-indigo-500">-&gt;</span> {target}
                </p>
              </ModalHeader>
              <ModalBody className="p-6">
                {loading ? (
                  <div className="flex flex-col items-center justify-center gap-4 py-12">
                    <div className="flex h-1 w-12 gap-1">
                      <div className="h-full w-full animate-bounce bg-indigo-500 [animation-delay:-0.3s]"></div>
                      <div className="h-full w-full animate-bounce bg-indigo-500 [animation-delay:-0.15s]"></div>
                      <div className="h-full w-full animate-bounce bg-indigo-500"></div>
                    </div>
                    <span className="animate-pulse text-sm font-medium text-zinc-400">
                      正在执行节点感知 TCP 诊断...
                    </span>
                  </div>
                ) : testResult ? (
                  <div className="custom-scrollbar max-h-[400px] space-y-4 overflow-y-auto pr-2">
                    <div className="group rounded-xl border border-emerald-500/20 bg-black/40 p-5 transition-colors hover:border-emerald-500/40">
                      <div className="mb-3 flex items-center justify-between">
                        <h3 className="text-xs font-black uppercase tracking-widest text-emerald-500">
                          TCP 探测
                        </h3>
                        <span className="text-[10px] font-mono text-zinc-600">
                          {testResult.success ? "成功" : "失败"} |{" "}
                          {testResult.averageTime.toFixed(2)} ms | 丢包{" "}
                          {testResult.packetLoss.toFixed(2)}%
                        </span>
                      </div>
                      <p className="mb-3 text-xs text-zinc-500">
                        目标: {testResult.targetHost}:{testResult.targetPort}
                      </p>
                      <pre className="whitespace-pre-wrap text-[11px] leading-relaxed text-zinc-400">
                        {testResult.ping_output || "未返回 TCP 探测数据"}
                      </pre>
                    </div>
                    <div className="group rounded-xl border border-amber-500/20 bg-black/40 p-5 transition-colors hover:border-amber-500/40">
                      <div className="mb-3 flex items-center justify-between">
                        <h3 className="text-xs font-black uppercase tracking-widest text-amber-500">
                          路径摘要
                        </h3>
                        <span className="text-[10px] font-mono text-zinc-600">
                          {testResult.trace_mode}
                          {testResult.simulated ? " | 模拟" : ""}
                        </span>
                      </div>
                      <p className="mb-3 text-xs text-zinc-500">
                        {testResult.message}
                      </p>
                      <pre className="whitespace-pre-wrap text-[11px] italic leading-relaxed text-zinc-400">
                        {testResult.trace_output || "未返回路径摘要"}
                      </pre>
                    </div>
                  </div>
                ) : (
                  <div className="rounded-xl border border-rose-500/20 bg-rose-500/10 py-8 text-center text-sm font-bold text-rose-500">
                    此连线无法完成诊断。
                  </div>
                )}
              </ModalBody>
              <ModalFooter className="p-4">
                <Button
                  className="border border-white/10 bg-white/5 text-zinc-300 hover:bg-white/10"
                  onPress={onClose}
                  variant="flat"
                >
                  关闭
                </Button>
                <Button
                  className="bg-indigo-600 font-bold text-white"
                  isLoading={loading}
                  onPress={() => onEdgeClick()}
                >
                  重试
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  );
}
