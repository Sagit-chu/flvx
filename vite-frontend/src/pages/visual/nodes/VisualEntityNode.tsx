import { memo } from "react";
import { Handle, NodeProps, Position } from "reactflow";

export interface VisualEntityNodeData {
  description?: string;
  kind: "system" | "tunnel" | "forward";
  meta?: string[];
  statusLabel?: string;
  subtitle?: string;
  title: string;
}

const KIND_STYLES: Record<
  VisualEntityNodeData["kind"],
  {
    border: string;
    dot: string;
    pill: string;
    title: string;
  }
> = {
  system: {
    border: "border-emerald-500/30",
    dot: "bg-emerald-500",
    pill: "bg-emerald-500/10 text-emerald-600 dark:text-emerald-300",
    title: "text-emerald-700 dark:text-emerald-200",
  },
  tunnel: {
    border: "border-sky-500/30",
    dot: "bg-sky-500",
    pill: "bg-sky-500/10 text-sky-700 dark:text-sky-300",
    title: "text-sky-700 dark:text-sky-200",
  },
  forward: {
    border: "border-amber-500/30",
    dot: "bg-amber-500",
    pill: "bg-amber-500/10 text-amber-700 dark:text-amber-300",
    title: "text-amber-700 dark:text-amber-200",
  },
};

export default memo(function VisualEntityNode({
  data,
  isConnectable,
  selected,
}: NodeProps<VisualEntityNodeData>) {
  const style = KIND_STYLES[data.kind];

  return (
    <div
      className={[
        "min-w-[220px] rounded-2xl border bg-white/95 p-4 shadow-lg backdrop-blur-sm transition-all dark:bg-zinc-950/95",
        style.border,
        selected ? "ring-2 ring-primary/60 shadow-xl" : "",
      ].join(" ")}
    >
      <Handle
        className="!h-3 !w-3 !border-2 !border-white !bg-zinc-500"
        isConnectable={isConnectable}
        position={Position.Left}
        type="target"
      />
      <Handle
        className="!h-3 !w-3 !border-2 !border-white !bg-zinc-700"
        isConnectable={isConnectable}
        position={Position.Right}
        type="source"
      />

      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="mb-2 flex items-center gap-2">
            <span className={["h-2.5 w-2.5 rounded-full", style.dot].join(" ")} />
            <span className="text-[11px] font-medium uppercase tracking-[0.18em] text-default-500">
              {data.kind}
            </span>
          </div>
          <div className={["truncate text-sm font-semibold", style.title].join(" ")}>
            {data.title}
          </div>
          {data.subtitle ? (
            <div className="mt-1 text-xs text-default-500">{data.subtitle}</div>
          ) : null}
        </div>
        {data.statusLabel ? (
          <div className={["rounded-full px-2 py-1 text-[10px] font-medium", style.pill].join(" ")}>
            {data.statusLabel}
          </div>
        ) : null}
      </div>

      {data.description ? (
        <div className="mt-3 line-clamp-2 text-xs text-default-600">
          {data.description}
        </div>
      ) : null}

      {Array.isArray(data.meta) && data.meta.length > 0 ? (
        <div className="mt-3 flex flex-wrap gap-2">
          {data.meta.map((item) => (
            <span
              key={item}
              className="rounded-full bg-default-100 px-2 py-1 text-[10px] text-default-600 dark:bg-zinc-900"
            >
              {item}
            </span>
          ))}
        </div>
      ) : null}
    </div>
  );
});
