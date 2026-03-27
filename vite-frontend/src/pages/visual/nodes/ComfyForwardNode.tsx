import { memo } from "react";
import { Handle, type NodeProps, Position } from "reactflow";

export interface ComfyForwardNodeData {
  draftId?: string;
  id?: number;
  inIp?: string;
  inPort: number | null;
  name: string;
  remoteAddrs: string[];
  status: number;
  strategy: string;
  tunnelId: number | null;
  tunnelName?: string;
}

const STRATEGY_LABELS: Record<string, string> = {
  fifo: "FIFO",
  hash: "Hash",
  rand: "Random",
  round: "Round Robin",
};

export default memo(function ComfyForwardNode({
  data,
  isConnectable,
  selected,
}: NodeProps<ComfyForwardNodeData>) {
  const isDraft = Boolean(data.draftId);
  const isEnabled = data.status === 1;
  const hasMultiTarget = data.remoteAddrs.length > 1;

  return (
    <div
      className={[
        "min-w-[240px] max-w-[300px] overflow-hidden rounded-xl border bg-zinc-950 shadow-2xl transition-all",
        selected
          ? "border-amber-500/60 ring-2 ring-amber-500/30 shadow-amber-500/10"
          : "border-zinc-800 hover:border-zinc-600",
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
            <path
              d="M8 7h12m0 0l-4-4m4 4l-4 4M16 17H4m0 0l4-4m-4 4l4 4"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
          <span className="truncate text-sm font-bold text-zinc-100">
            {data.name || "Unnamed Rule"}
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

      {/* Tunnel Association */}
      <div className="border-b border-zinc-800/60 px-3 py-2">
        <div className="flex items-center gap-1.5">
          <span className="h-1.5 w-1.5 rounded-full bg-sky-500" />
          <span className="text-[10px] font-medium text-zinc-500">via</span>
          <span className="truncate text-xs font-medium text-sky-400">
            {data.tunnelName || "No tunnel"}
          </span>
        </div>
      </div>

      {/* Port Mapping */}
      <div className="border-b border-zinc-800/60 px-3 py-2">
        <div className="text-[10px] font-medium uppercase tracking-wider text-zinc-600 mb-1.5">
          Port Mapping
        </div>
        <div className="flex items-center gap-2 rounded-lg bg-zinc-900 px-2.5 py-1.5">
          <span className="font-mono text-sm font-bold text-emerald-400">
            {data.inPort || "auto"}
          </span>
          <svg
            className="h-3 w-3 text-zinc-600"
            fill="none"
            stroke="currentColor"
            strokeWidth={2.5}
            viewBox="0 0 24 24"
          >
            <path
              d="M14 5l7 7m0 0l-7 7m7-7H3"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
          <div className="min-w-0 flex-1">
            {data.remoteAddrs.length > 0 ? (
              data.remoteAddrs.length === 1 ? (
                <span className="truncate font-mono text-sm font-bold text-amber-400">
                  {data.remoteAddrs[0]}
                </span>
              ) : (
                <span className="font-mono text-sm font-bold text-amber-400">
                  {data.remoteAddrs.length} targets
                </span>
              )
            ) : (
              <span className="text-xs italic text-zinc-600">
                no target
              </span>
            )}
          </div>
        </div>
      </div>

      {/* Multi-target list */}
      {hasMultiTarget && (
        <div className="border-b border-zinc-800/60 px-3 py-2">
          <div className="space-y-1">
            {data.remoteAddrs.slice(0, 5).map((addr, i) => (
              <div
                key={i}
                className="flex items-center gap-1.5 text-[10px]"
              >
                <span className="h-1 w-1 rounded-full bg-amber-500/60" />
                <span className="truncate font-mono text-zinc-400">
                  {addr}
                </span>
              </div>
            ))}
            {data.remoteAddrs.length > 5 && (
              <span className="text-[10px] text-zinc-700">
                +{data.remoteAddrs.length - 5} more
              </span>
            )}
          </div>
        </div>
      )}

      {/* Strategy & Config */}
      <div className="flex flex-wrap gap-1.5 px-3 py-2">
        {hasMultiTarget && (
          <span className="rounded bg-zinc-800/80 px-1.5 py-0.5 text-[10px] text-zinc-500">
            {STRATEGY_LABELS[data.strategy] || data.strategy}
          </span>
        )}
        {data.inIp && (
          <span className="rounded bg-zinc-800/80 px-1.5 py-0.5 text-[10px] text-zinc-500">
            Listen: {data.inIp}
          </span>
        )}
      </div>

      {/* Handles */}
      <Handle
        className="!-top-1 !h-3.5 !w-3.5 !rounded-md !border-2 !border-zinc-700 !bg-sky-600 hover:!border-sky-400 hover:!bg-sky-400"
        id="tunnel"
        isConnectable={isConnectable}
        position={Position.Top}
        type="target"
      />

      {/* Handle Label */}
      <div className="pointer-events-none absolute -top-4 left-1/2 -translate-x-1/2 -translate-y-full">
        <span className="text-[8px] font-black uppercase tracking-widest text-sky-800">
          TUNNEL
        </span>
      </div>
    </div>
  );
});
