import { memo } from "react";
import { Handle, type NodeProps, Position } from "reactflow";

export interface ComfyTunnelNodeData {
  chainCount: number;
  draftId?: string;
  entryCount: number;
  exitCount: number;
  exitProtocol?: string;
  exitStrategy?: string;
  flow: number;
  id?: number;
  name: string;
  relatedForwards: number;
  status: number;
  trafficRatio: number;
  type: number;
}

const TYPE_STYLES = {
  1: {
    badge: "bg-violet-500/15 text-violet-400 border-violet-500/30",
    border: "border-violet-500/40",
    glow: "shadow-violet-500/10",
    label: "Port Forward",
    ring: "ring-violet-500/30",
  },
  2: {
    badge: "bg-sky-500/15 text-sky-400 border-sky-500/30",
    border: "border-sky-500/40",
    glow: "shadow-sky-500/10",
    label: "Tunnel Forward",
    ring: "ring-sky-500/30",
  },
} as const;

export default memo(function ComfyTunnelNode({
  data,
  isConnectable,
  selected,
}: NodeProps<ComfyTunnelNodeData>) {
  const style = TYPE_STYLES[data.type as 1 | 2] || TYPE_STYLES[1];
  const isDraft = Boolean(data.draftId);
  const isEnabled = data.status === 1;
  const isTunnelForward = data.type === 2;

  return (
    <div
      className={[
        "min-w-[240px] max-w-[280px] overflow-hidden rounded-xl border bg-zinc-950 shadow-2xl transition-all",
        selected ? `${style.border} ring-2 ${style.ring} ${style.glow}` : "border-zinc-800 hover:border-zinc-600",
      ].join(" ")}
    >
      {/* Header */}
      <div className="flex items-center justify-between border-b border-zinc-800 px-3 py-2">
        <div className="flex items-center gap-2 min-w-0">
          <svg
            className="h-4 w-4 flex-shrink-0 text-zinc-500"
            fill="none"
            stroke="currentColor"
            strokeWidth={2}
            viewBox="0 0 24 24"
          >
            {isTunnelForward ? (
              <path
                d="M13 10V3L4 14h7v7l9-11h-7z"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            ) : (
              <path
                d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            )}
          </svg>
          <span className="truncate text-sm font-bold text-zinc-100">
            {data.name || "Unnamed Tunnel"}
          </span>
        </div>
        <span
          className={[
            "flex-shrink-0 rounded-md border px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider",
            isDraft
              ? "border-amber-500/30 bg-amber-500/15 text-amber-400"
              : isEnabled
                ? "border-emerald-500/30 bg-emerald-500/15 text-emerald-400"
                : "border-zinc-700 bg-zinc-800 text-zinc-500",
          ].join(" ")}
        >
          {isDraft ? "Draft" : isEnabled ? "Active" : "Disabled"}
        </span>
      </div>

      {/* Type Badge */}
      <div className="border-b border-zinc-800/60 px-3 py-2">
        <span
          className={[
            "inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider",
            style.badge,
          ].join(" ")}
        >
          {style.label}
        </span>
      </div>

      {/* Topology Info */}
      <div className="space-y-1.5 border-b border-zinc-800/60 px-3 py-2">
        <div className="flex items-center justify-between text-[11px]">
          <span className="text-zinc-600">Entry Nodes</span>
          <span className="font-mono font-semibold text-emerald-400">
            {data.entryCount}
          </span>
        </div>
        {isTunnelForward && (
          <>
            <div className="flex items-center justify-between text-[11px]">
              <span className="text-zinc-600">Chain Hops</span>
              <span className="font-mono font-semibold text-sky-400">
                {data.chainCount}
              </span>
            </div>
            <div className="flex items-center justify-between text-[11px]">
              <span className="text-zinc-600">Exit Nodes</span>
              <span className="font-mono font-semibold text-teal-400">
                {data.exitCount}
              </span>
            </div>
          </>
        )}
        <div className="flex items-center justify-between text-[11px]">
          <span className="text-zinc-600">Rules</span>
          <span className="font-mono font-semibold text-amber-400">
            {data.relatedForwards}
          </span>
        </div>
      </div>

      {/* Config Row */}
      <div className="flex flex-wrap gap-1.5 px-3 py-2">
        <span className="rounded bg-zinc-800/80 px-1.5 py-0.5 text-[10px] text-zinc-500">
          {data.trafficRatio || 1}x rate
        </span>
        <span className="rounded bg-zinc-800/80 px-1.5 py-0.5 text-[10px] text-zinc-500">
          {data.flow === 2 ? "Bi-dir" : "One-way"}
        </span>
        {isTunnelForward && data.exitProtocol && (
          <span className="rounded bg-zinc-800/80 px-1.5 py-0.5 text-[10px] text-zinc-500">
            {data.exitProtocol.toUpperCase()}
          </span>
        )}
        {isTunnelForward && data.exitStrategy && (
          <span className="rounded bg-zinc-800/80 px-1.5 py-0.5 text-[10px] text-zinc-500">
            {data.exitStrategy}
          </span>
        )}
      </div>

      {/* Visual Chain Indicator for Tunnel Forwarding */}
      {isTunnelForward && data.chainCount > 0 && (
        <div className="border-t border-zinc-800/60 px-3 py-2">
          <div className="flex items-center gap-1">
            <span className="h-2 w-2 rounded-full bg-emerald-500" />
            {Array.from({ length: data.chainCount }).map((_, i) => (
              <span key={i} className="flex items-center gap-1">
                <span className="h-px w-3 bg-zinc-700" />
                <span className="h-2 w-2 rounded-full bg-sky-500" />
              </span>
            ))}
            <span className="h-px w-3 bg-zinc-700" />
            <span className="h-2 w-2 rounded-full bg-teal-500" />
          </div>
          <div className="mt-1 flex items-center justify-between text-[9px] text-zinc-700">
            <span>ENTRY</span>
            <span>EXIT</span>
          </div>
        </div>
      )}

      {/* Handles */}
      <Handle
        className="!-left-1 !h-3.5 !w-3.5 !rounded-md !border-2 !border-zinc-700 !bg-emerald-600 hover:!border-emerald-400 hover:!bg-emerald-400"
        id="entry"
        isConnectable={isConnectable}
        position={Position.Left}
        type="target"
      />
      <Handle
        className="!-right-1 !h-3.5 !w-3.5 !rounded-md !border-2 !border-zinc-700 !bg-teal-600 hover:!border-teal-400 hover:!bg-teal-400"
        id="exit"
        isConnectable={isConnectable}
        position={Position.Right}
        type="source"
      />
      {/* Bottom handle for rule connections */}
      <Handle
        className="!-bottom-1 !h-3.5 !w-3.5 !rounded-md !border-2 !border-zinc-700 !bg-amber-600 hover:!border-amber-400 hover:!bg-amber-400"
        id="rules"
        isConnectable={isConnectable}
        position={Position.Bottom}
        type="source"
      />

      {/* Handle Labels */}
      <div className="pointer-events-none absolute -left-3 top-1/2 -translate-x-full -translate-y-1/2">
        <span className="text-[8px] font-black uppercase tracking-widest text-emerald-800">
          ENTRY
        </span>
      </div>
      <div className="pointer-events-none absolute -right-3 top-1/2 -translate-y-1/2 translate-x-full">
        <span className="text-[8px] font-black uppercase tracking-widest text-teal-800">
          EXIT
        </span>
      </div>
      <div className="pointer-events-none absolute -bottom-4 left-1/2 -translate-x-1/2 translate-y-full">
        <span className="text-[8px] font-black uppercase tracking-widest text-amber-800">
          RULES
        </span>
      </div>
    </div>
  );
});
