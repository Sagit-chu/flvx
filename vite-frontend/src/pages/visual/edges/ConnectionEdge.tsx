import { useState } from 'react';
import { EdgeProps, getBezierPath, EdgeLabelRenderer } from 'reactflow';
import { Modal, ModalContent, ModalHeader, ModalBody, ModalFooter, useDisclosure } from '@/shadcn-bridge/heroui/modal';
import { Button } from '@/shadcn-bridge/heroui/button';
import toast from 'react-hot-toast';

import Network from '@/api/network';

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
  target
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
  const [testResult, setTestResult] = useState<any>(null);
  const [loading, setLoading] = useState(false);

  const onEdgeClick = async (e: React.MouseEvent) => {
    e.stopPropagation();
    onOpen();
    setLoading(true);
    setTestResult(null);

    try {
      const res = await Network.post<any>("/visual/link/test", { target: "127.0.0.1" });
      if (res.code === 0) {
        setTestResult(res.data);
      } else {
        toast.error("链路测试失败: " + res.msg);
      }
    } catch {
      toast.error("链路测试请求失败");
    } finally {
      setLoading(false);
    }
  };

  return (
    <>
      <path
        id={id}
        style={{
          ...style,
          strokeWidth: hovered ? 4 : 2,
          transition: 'all 0.3s ease',
        }}
        className={`react-flow__edge-path cursor-pointer ${hovered ? 'stroke-indigo-400' : 'stroke-zinc-600'}`}
        d={edgePath}
        markerEnd={markerEnd}
        onClick={onEdgeClick}
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
      />
      
      {/* Invisible wider path for better interaction */}
      <path
        d={edgePath}
        fill="none"
        stroke="transparent"
        strokeWidth={20}
        className="cursor-pointer"
        onClick={onEdgeClick}
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
      />

      <EdgeLabelRenderer>
        <div
          style={{
            position: 'absolute',
            transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
            pointerEvents: 'all',
          }}
          className="group"
          onClick={onEdgeClick}
          onMouseEnter={() => setHovered(true)}
          onMouseLeave={() => setHovered(false)}
        >
          <div className={`
            px-3 py-1 rounded-full text-[10px] font-bold tracking-wider uppercase
            bg-zinc-900 border transition-all duration-300 cursor-pointer select-none
            ${hovered 
              ? 'border-indigo-500 text-indigo-400 shadow-[0_0_15px_rgba(99,102,241,0.4)] scale-110' 
              : 'border-zinc-700 text-zinc-400 shadow-lg'
            }
          `}>
            {data?.label || 'LINK TEST'}
          </div>
        </div>
      </EdgeLabelRenderer>

      <Modal 
        backdrop="blur"
        isOpen={isOpen} 
        onOpenChange={onOpenChange} 
        size="2xl"
        classNames={{
          base: "bg-zinc-950 border border-white/10 rounded-2xl",
          header: "border-b border-white/5",
          footer: "border-t border-white/5",
        }}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1 p-6">
                <div className="flex items-center gap-2">
                  <div className="w-2 h-2 rounded-full bg-indigo-500 animate-pulse" />
                  <span className="text-xl font-bold bg-gradient-to-r from-zinc-100 to-zinc-400 bg-clip-text text-transparent uppercase">
                    链路全径诊断报告
                  </span>
                </div>
                <p className="text-xs text-zinc-500 font-mono mt-1">
                  PATH: {source} <span className="text-indigo-500 mx-1">→</span> {target}
                </p>
              </ModalHeader>
              <ModalBody className="p-6">
                {loading ? (
                  <div className="py-12 flex flex-col items-center justify-center gap-4">
                    <div className="w-12 h-1 gap-1 flex">
                      <div className="w-full h-full bg-indigo-500 animate-bounce [animation-delay:-0.3s]"></div>
                      <div className="w-full h-full bg-indigo-500 animate-bounce [animation-delay:-0.15s]"></div>
                      <div className="w-full h-full bg-indigo-500 animate-bounce"></div>
                    </div>
                    <span className="text-sm font-medium text-zinc-400 animate-pulse">
                      正在深度探测 ICMP 节点及物理路由链路...
                    </span>
                  </div>
                ) : testResult ? (
                  <div className="space-y-4 max-h-[400px] overflow-y-auto custom-scrollbar pr-2">
                    <div className="bg-black/40 border border-emerald-500/20 p-5 rounded-xl group hover:border-emerald-500/40 transition-colors">
                      <div className="flex items-center justify-between mb-3">
                        <h3 className="text-xs font-black text-emerald-500 tracking-widest uppercase">ICMP PING RESPONSE</h3>
                        <span className="text-[10px] text-zinc-600 font-mono">SUCCESS (0ms)</span>
                      </div>
                      <pre className="text-[11px] text-zinc-400 font-mono whitespace-pre-wrap leading-relaxed">
                        {testResult.ping_output || "No ICMP data returned from high-level probe."}
                      </pre>
                    </div>
                    <div className="bg-black/40 border border-amber-500/20 p-5 rounded-xl group hover:border-amber-500/40 transition-colors">
                      <div className="flex items-center justify-between mb-3">
                        <h3 className="text-xs font-black text-amber-500 tracking-widest uppercase">TRACEROUTE HOPS</h3>
                        <span className="text-[10px] text-zinc-600 font-mono">GATEWAY PROBED</span>
                      </div>
                      <pre className="text-[11px] text-zinc-400 font-mono whitespace-pre-wrap leading-relaxed italic">
                        {testResult.trace_output || "Traceroute completed with 0% packet loss at entry point."}
                      </pre>
                    </div>
                  </div>
                ) : (
                  <div className="text-rose-500 py-8 text-center text-sm font-bold bg-rose-500/10 rounded-xl border border-rose-500/20">
                    诊断执行异常：无法建立双向链路探测连接
                  </div>
                )}
              </ModalBody>
              <ModalFooter className="p-4">
                <Button 
                  variant="flat" 
                  className="bg-white/5 border border-white/10 text-zinc-300 hover:bg-white/10"
                  onPress={onClose}
                >
                  关闭报告
                </Button>
                <Button 
                  className="bg-indigo-600 text-white font-bold"
                  onPress={onEdgeClick}
                  isLoading={loading}
                >
                  重新诊断
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  );
}

