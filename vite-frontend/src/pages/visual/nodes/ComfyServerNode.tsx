import { memo, useState } from "react";
import { Handle, type NodeProps, Position } from "reactflow";

export interface ComfyServerNodeData {
  allowedPorts?: string;
  connections?: number;
  cpuUsage?: number;
  id: number;
  ip: string;
  ipV4?: string;
  ipV6?: string;
  memUsage?: number;
  name: string;
  occupiedPorts?: number[];
  online: boolean;
}

const STATUS_COLORS: Record<string, string> = {
  high: "bg-red-500",
  low: "bg-emerald-500",
  medium: "bg-amber-400",
};

const getUsageColor = (value: number) => {
  if (value >= 80) return STATUS_COLORS.high;
  if (value >= 50) return STATUS_COLORS.medium;
  return STATUS_COLORS.low;
};

const MetricBar = ({
  label,
  value,
}: {
  label: string;
  value: number;
}) => (
  <div className="flex items-center gap-2">
    <span className="w-8 text-[10px] font-medium text-zinc-500">{label}</span>
    <div className="relative h-1.5 flex-1 overflow-hidden rounded-full bg-zinc-800">
      <div
        className={`absolute inset-y-0 left-0 rounded-full transition-all duration-500 ${getUsageColor(value)}`}
        style={{ width: `${Math.min(100, value)}%` }}
      />
    </div>
    <span className="w-10 text-right font-mono text-[10px] text-zinc-400">
      {value.toFixed(1)}%
    </span>
  </div>
);

export default memo(function ComfyServerNode({
  data,
  isConnectable,
  selected,
}: NodeProps<ComfyServerNodeData>) {
  const [expanded, setExpanded] = useState(false);
  const occupiedPorts = data.occupiedPorts || [];
  const displayPorts = expanded ? occupiedPorts : occupiedPorts.slice(0, 12);
  const hasMore = occupiedPorts.length > 12;

  return (
    <div
      className={[
        "min-w-[260px] max-w-[300px] overflow-hidden rounded-xl border bg-zinc-950 shadow-2xl transition-all",
        selected
          ? "border-emerald-500/60 ring-2 ring-emerald-500/30 shadow-emerald-500/10"
          : "border-zinc-800 hover:border-zinc-600",
      ].join(" ")}
    >
      {/* Header */}
      <div className="flex items-center justify-between border-b border-zinc-800 px-3 py-2">
        <div className="flex items-center gap-2 min-w-0">
          <div className="relative flex-shrink-0">
            <span
              className={[
                "block h-2.5 w-2.5 rounded-full",
                data.online ? "bg-emerald-500" : "bg-red-500",
              ].join(" ")}
            />
            {data.online && (
              <span className="absolute inset-0 animate-ping rounded-full bg-emerald-500 opacity-40" />
            )}
          </div>
          <span className="truncate text-sm font-bold tracking-tight text-zinc-100">
            {data.name}
          </span>
        </div>
        <span
          className={[
            "flex-shrink-0 rounded-md px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider",
            data.online
              ? "bg-emerald-500/15 text-emerald-400"
              : "bg-red-500/15 text-red-400",
          ].join(" ")}
        >
          {data.online ? "Online" : "Offline"}
        </span>
      </div>

      {/* IP Section */}
      <div className="border-b border-zinc-800/60 px-3 py-2">
        <div className="flex items-center justify-between">
          <span className="text-[10px] font-medium uppercase tracking-wider text-zinc-600">
            Network
          </span>
          <span className="font-mono text-xs text-zinc-300">
            {data.ip || data.ipV4 || "0.0.0.0"}
          </span>
        </div>
        {data.ipV6 && (
          <div className="mt-1 truncate font-mono text-[10px] text-zinc-600">
            {data.ipV6}
          </div>
        )}
      </div>

      {/* Metrics */}
      <div className="space-y-1.5 border-b border-zinc-800/60 px-3 py-2">
        <MetricBar label="CPU" value={data.cpuUsage || 0} />
        <MetricBar label="MEM" value={data.memUsage || 0} />
        <div className="flex items-center justify-between">
          <span className="text-[10px] font-medium text-zinc-500">
            Connections
          </span>
          <span className="font-mono text-xs font-semibold text-zinc-300">
            {data.connections || 0}
          </span>
        </div>
      </div>

      {/* Port Grid */}
      <div className="px-3 py-2">
        <div className="mb-1.5 flex items-center justify-between">
          <span className="text-[10px] font-medium uppercase tracking-wider text-zinc-600">
            Ports
          </span>
          <span className="text-[10px] text-zinc-600">
            {occupiedPorts.length} occupied
          </span>
        </div>
        {occupiedPorts.length > 0 ? (
          <>
            <div className="flex flex-wrap gap-1">
              {displayPorts.map((port) => (
                <span
                  key={port}
                  className="inline-flex items-center gap-1 rounded bg-zinc-800/80 px-1.5 py-0.5 font-mono text-[10px] text-zinc-400"
                >
                  <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
                  {port}
                </span>
              ))}
            </div>
            {hasMore && (
              <button
                className="mt-1 text-[10px] text-zinc-600 hover:text-zinc-400"
                onClick={(e) => {
                  e.stopPropagation();
                  setExpanded(!expanded);
                }}
              >
                {expanded
                  ? "collapse"
                  : `+${occupiedPorts.length - 12} more`}
              </button>
            )}
          </>
        ) : (
          <span className="text-[10px] italic text-zinc-700">
            No ports in use
          </span>
        )}
        {data.allowedPorts && (
          <div className="mt-1 text-[10px] text-zinc-700">
            Range: {data.allowedPorts}
          </div>
        )}
      </div>

      {/* Handles */}
      <Handle
        className="!-left-1 !h-3.5 !w-3.5 !rounded-md !border-2 !border-zinc-700 !bg-zinc-500 hover:!border-emerald-500 hover:!bg-emerald-500"
        id="in"
        isConnectable={isConnectable}
        position={Position.Left}
        type="target"
      />
      <Handle
        className="!-right-1 !h-3.5 !w-3.5 !rounded-md !border-2 !border-zinc-700 !bg-zinc-500 hover:!border-sky-500 hover:!bg-sky-500"
        id="out"
        isConnectable={isConnectable}
        position={Position.Right}
        type="source"
      />

      {/* Handle Labels */}
      <div className="pointer-events-none absolute -left-3 top-1/2 -translate-x-full -translate-y-1/2">
        <span className="text-[8px] font-black uppercase tracking-widest text-zinc-700">
          IN
        </span>
      </div>
      <div className="pointer-events-none absolute -right-3 top-1/2 -translate-y-1/2 translate-x-full">
        <span className="text-[8px] font-black uppercase tracking-widest text-zinc-700">
          OUT
        </span>
      </div>
    </div>
  );
});
