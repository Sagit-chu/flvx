import { memo } from "react";
import { Handle, type NodeProps, Position } from "reactflow";

export interface ComfyLandingNodeData {
  addresses: string[];
  forwardId?: number;
  forwardName?: string;
  inPort?: number | null;
}

export default memo(function ComfyLandingNode({
  data,
  isConnectable,
  selected,
}: NodeProps<ComfyLandingNodeData>) {
  return (
    <div
      className={[
        "min-w-[200px] max-w-[260px] overflow-hidden rounded-xl border bg-zinc-950 shadow-2xl transition-all",
        selected
          ? "border-rose-500/60 ring-2 ring-rose-500/30 shadow-rose-500/10"
          : "border-zinc-800 hover:border-zinc-600",
      ].join(" ")}
    >
      {/* Header */}
      <div className="flex items-center justify-between border-b border-zinc-800 px-3 py-2">
        <div className="flex items-center gap-2 min-w-0">
          <svg
            className="h-4 w-4 flex-shrink-0 text-rose-500"
            fill="none"
            stroke="currentColor"
            strokeWidth={2}
            viewBox="0 0 24 24"
          >
            <path
              d="M5 12h14M12 5l7 7-7 7"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
          <span className="truncate text-sm font-bold text-zinc-100">
            落地目标
          </span>
        </div>
        <span className="flex-shrink-0 rounded-md border border-rose-500/30 bg-rose-500/15 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-rose-400">
          Landing
        </span>
      </div>

      {/* Source info */}
      {data.forwardName && (
        <div className="border-b border-zinc-800/60 px-3 py-1.5">
          <div className="flex items-center gap-1.5 text-[10px]">
            <span className="text-zinc-600">from</span>
            <span className="font-medium text-amber-400 truncate">
              {data.forwardName}
            </span>
            {data.inPort && (
              <span className="font-mono text-zinc-500">:{data.inPort}</span>
            )}
          </div>
        </div>
      )}

      {/* Address list */}
      <div className="px-3 py-2 space-y-1">
        <div className="text-[10px] font-medium uppercase tracking-wider text-zinc-600 mb-1">
          Target Addresses
        </div>
        {data.addresses.length > 0 ? (
          data.addresses.slice(0, 6).map((addr, i) => (
            <div
              key={i}
              className="flex items-center gap-1.5 rounded bg-zinc-900 px-2 py-1"
            >
              <span className="h-1.5 w-1.5 rounded-full bg-rose-500" />
              <span className="truncate font-mono text-xs text-rose-300">
                {addr}
              </span>
            </div>
          ))
        ) : (
          <div className="text-[10px] italic text-zinc-700">No targets</div>
        )}
        {data.addresses.length > 6 && (
          <span className="text-[10px] text-zinc-700">
            +{data.addresses.length - 6} more
          </span>
        )}
      </div>

      {/* Handle */}
      <Handle
        className="!-left-1 !h-3.5 !w-3.5 !rounded-md !border-2 !border-zinc-700 !bg-rose-600 hover:!border-rose-400 hover:!bg-rose-400"
        id="in"
        isConnectable={isConnectable}
        position={Position.Left}
        type="target"
      />
      <div className="pointer-events-none absolute -left-3 top-1/2 -translate-x-full -translate-y-1/2">
        <span className="text-[8px] font-black uppercase tracking-widest text-rose-900">
          IN
        </span>
      </div>
    </div>
  );
});
