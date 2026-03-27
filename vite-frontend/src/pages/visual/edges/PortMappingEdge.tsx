import { useState } from "react";
import {
  type EdgeProps,
  EdgeLabelRenderer,
  getSmoothStepPath,
} from "reactflow";
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
  averageTime: number;
  message: string;
  packetLoss: number;
  ping_output: string;
  simulated: boolean;
  success: boolean;
  targetHost: string;
  targetPort: number;
  trace_mode: string;
  trace_output: string;
}

export interface PortMappingEdgeData {
  color?: string;
  diagnosticSourceId?: number;
  diagnosticTargetId?: number;
  edgeType?: "entry" | "exit" | "hop" | "rule";
  label: string;
  portMapping?: string;
  speedLimit?: string;
}

const EDGE_COLORS: Record<string, string> = {
  entry: "#22c55e",
  exit: "#0d9488",
  hop: "#0284c7",
  rule: "#d97706",
};

export default function PortMappingEdge({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  data,
  markerEnd,
}: EdgeProps<PortMappingEdgeData>) {
  const [edgePath, labelX, labelY] = getSmoothStepPath({
    sourcePosition,
    sourceX,
    sourceY,
    targetPosition,
    targetX,
    targetY,
  });

  const { isOpen, onOpen, onOpenChange } = useDisclosure();
  const [hovered, setHovered] = useState(false);
  const [testResult, setTestResult] = useState<LinkTestResult | null>(null);
  const [loading, setLoading] = useState(false);

  const edgeType = data?.edgeType || "entry";
  const color = data?.color || EDGE_COLORS[edgeType] || "#71717a";
  const canTest =
    typeof data?.diagnosticSourceId === "number" &&
    typeof data?.diagnosticTargetId === "number";

  const runLinkTest = async () => {
    if (!canTest) {
      toast.error("此连线不支持链路测试");
      return;
    }

    onOpen();
    setLoading(true);

    try {
      const res = await Network.post<LinkTestResult>("/link/test", {
        sourceNodeId: data!.diagnosticSourceId,
        targetNodeId: data!.diagnosticTargetId,
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

  const handleEdgeClick = (e?: { stopPropagation?: () => void }) => {
    e?.stopPropagation?.();
    if (canTest) {
      void runLinkTest();
    }
  };

  const isAnimated = edgeType === "rule";

  return (
    <>
      {/* Main edge path */}
      <path
        className="react-flow__edge-path"
        d={edgePath}
        id={id}
        markerEnd={markerEnd}
        style={{
          stroke: hovered ? "#818cf8" : color,
          strokeDasharray: isAnimated ? "6 3" : undefined,
          strokeWidth: hovered ? 3 : 2,
          transition: "all 0.2s ease",
        }}
      />

      {/* Wider invisible path for easier interaction */}
      <path
        className="cursor-pointer"
        d={edgePath}
        fill="none"
        onClick={handleEdgeClick}
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        stroke="transparent"
        strokeWidth={24}
      />

      {/* Animated flow particles */}
      {isAnimated && (
        <circle r={3} fill={color}>
          <animateMotion dur="2s" path={edgePath} repeatCount="indefinite" />
        </circle>
      )}

      {/* Label */}
      <EdgeLabelRenderer>
        <div
          className="nodrag nopan"
          onClick={handleEdgeClick}
          onMouseEnter={() => setHovered(true)}
          onMouseLeave={() => setHovered(false)}
          style={{
            pointerEvents: "all",
            position: "absolute",
            transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
          }}
        >
          <div
            className={[
              "flex cursor-pointer select-none flex-col items-center gap-0.5 rounded-lg border px-2.5 py-1 text-center transition-all duration-200",
              hovered
                ? "scale-110 border-indigo-500/60 bg-zinc-900 shadow-lg shadow-indigo-500/20"
                : "border-zinc-800 bg-zinc-950/95 shadow-md",
            ].join(" ")}
          >
            {/* Main label */}
            <span
              className="text-[10px] font-bold uppercase tracking-wider"
              style={{ color: hovered ? "#818cf8" : color }}
            >
              {data?.label || edgeType}
            </span>

            {/* Port mapping */}
            {data?.portMapping && (
              <span className="font-mono text-[9px] text-zinc-400">
                {data.portMapping}
              </span>
            )}

            {/* Speed limit */}
            {data?.speedLimit && (
              <span className="text-[9px] text-amber-500">
                {data.speedLimit}
              </span>
            )}

            {/* Test hint */}
            {canTest && hovered && (
              <span className="text-[8px] text-indigo-400">
                click to test
              </span>
            )}
          </div>
        </div>
      </EdgeLabelRenderer>

      {/* Diagnostic Modal */}
      <Modal
        backdrop="blur"
        classNames={{
          base: "rounded-2xl border border-zinc-800 bg-zinc-950",
          footer: "border-t border-zinc-800",
          header: "border-b border-zinc-800",
        }}
        isOpen={isOpen}
        onOpenChange={onOpenChange}
        size="2xl"
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="p-5">
                <div>
                  <div className="flex items-center gap-2">
                    <div className="h-2 w-2 animate-pulse rounded-full bg-indigo-500" />
                    <span className="text-lg font-bold text-zinc-100">
                      Link Diagnostics
                    </span>
                  </div>
                  <p className="mt-1 font-mono text-xs text-zinc-600">
                    {data?.diagnosticSourceId}{" "}
                    <span className="text-indigo-500">→</span>{" "}
                    {data?.diagnosticTargetId}
                    {data?.portMapping && (
                      <span className="ml-2 text-zinc-500">
                        ({data.portMapping})
                      </span>
                    )}
                  </p>
                </div>
              </ModalHeader>
              <ModalBody className="p-5">
                {loading ? (
                  <div className="flex flex-col items-center gap-4 py-12">
                    <div className="flex gap-1">
                      <div className="h-2 w-2 animate-bounce rounded-full bg-indigo-500 [animation-delay:-0.3s]" />
                      <div className="h-2 w-2 animate-bounce rounded-full bg-indigo-500 [animation-delay:-0.15s]" />
                      <div className="h-2 w-2 animate-bounce rounded-full bg-indigo-500" />
                    </div>
                    <span className="animate-pulse text-sm text-zinc-500">
                      Running TCP diagnostics...
                    </span>
                  </div>
                ) : testResult ? (
                  <div className="space-y-4">
                    {/* Summary Cards */}
                    <div className="grid grid-cols-3 gap-3">
                      <div className="rounded-lg border border-zinc-800 bg-zinc-900/50 p-3 text-center">
                        <div className="text-[10px] font-medium uppercase text-zinc-600">
                          Status
                        </div>
                        <div
                          className={[
                            "mt-1 text-lg font-bold",
                            testResult.success
                              ? "text-emerald-400"
                              : "text-red-400",
                          ].join(" ")}
                        >
                          {testResult.success ? "OK" : "FAIL"}
                        </div>
                      </div>
                      <div className="rounded-lg border border-zinc-800 bg-zinc-900/50 p-3 text-center">
                        <div className="text-[10px] font-medium uppercase text-zinc-600">
                          RTT
                        </div>
                        <div className="mt-1 text-lg font-bold text-sky-400">
                          {testResult.averageTime.toFixed(2)}
                          <span className="text-xs text-zinc-600"> ms</span>
                        </div>
                      </div>
                      <div className="rounded-lg border border-zinc-800 bg-zinc-900/50 p-3 text-center">
                        <div className="text-[10px] font-medium uppercase text-zinc-600">
                          Packet Loss
                        </div>
                        <div
                          className={[
                            "mt-1 text-lg font-bold",
                            testResult.packetLoss > 5
                              ? "text-red-400"
                              : testResult.packetLoss > 0
                                ? "text-amber-400"
                                : "text-emerald-400",
                          ].join(" ")}
                        >
                          {testResult.packetLoss.toFixed(1)}%
                        </div>
                      </div>
                    </div>

                    {/* TCP Probe */}
                    <div className="rounded-lg border border-zinc-800 bg-zinc-900/30 p-4">
                      <div className="mb-2 flex items-center justify-between">
                        <span className="text-xs font-bold uppercase tracking-wider text-emerald-500">
                          TCP Probe
                        </span>
                        <span className="font-mono text-[10px] text-zinc-700">
                          {testResult.targetHost}:{testResult.targetPort}
                        </span>
                      </div>
                      <pre className="whitespace-pre-wrap text-[11px] leading-relaxed text-zinc-500">
                        {testResult.ping_output || "No TCP probe data"}
                      </pre>
                    </div>

                    {/* Traceroute */}
                    <div className="rounded-lg border border-zinc-800 bg-zinc-900/30 p-4">
                      <div className="mb-2 flex items-center justify-between">
                        <span className="text-xs font-bold uppercase tracking-wider text-amber-500">
                          Route Trace
                        </span>
                        <span className="font-mono text-[10px] text-zinc-700">
                          {testResult.trace_mode}
                          {testResult.simulated ? " | simulated" : ""}
                        </span>
                      </div>
                      <p className="mb-2 text-[11px] text-zinc-600">
                        {testResult.message}
                      </p>
                      <pre className="whitespace-pre-wrap text-[11px] leading-relaxed text-zinc-500">
                        {testResult.trace_output || "No route data"}
                      </pre>
                    </div>
                  </div>
                ) : (
                  <div className="rounded-lg border border-red-500/20 bg-red-500/10 py-8 text-center text-sm font-bold text-red-400">
                    Diagnostics unavailable for this link.
                  </div>
                )}
              </ModalBody>
              <ModalFooter className="p-4">
                <Button
                  className="border-zinc-800 text-zinc-400"
                  onPress={onClose}
                  variant="bordered"
                >
                  Close
                </Button>
                {canTest && (
                  <Button
                    className="bg-indigo-600 font-semibold text-white"
                    isLoading={loading}
                    onPress={() => handleEdgeClick()}
                  >
                    Retry Test
                  </Button>
                )}
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  );
}
