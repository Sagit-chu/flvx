import { memo } from 'react';
import { Handle, Position, NodeProps } from 'reactflow';

export default memo(({ data, isConnectable }: NodeProps) => {
  const isOnline = data.status === 'online';

  return (
    <div className={`
      relative min-w-[200px] p-0 rounded-2xl overflow-hidden
      bg-zinc-900/90 backdrop-blur-xl 
      border border-white/10 shadow-[0_8px_32px_rgba(0,0,0,0.4)]
      transition-all duration-300 hover:shadow-indigo-500/20 hover:border-indigo-500/30
    `}>
      {/* Header with Breathing Light */}
      <div className="px-4 py-3 border-b border-white/5 flex items-center justify-between">
        <div className="flex items-center gap-2.5 overflow-hidden">
          <div className="relative flex items-center justify-center">
            <span className={`w-2.5 h-2.5 rounded-full ${isOnline ? 'bg-emerald-500' : 'bg-rose-500'} relative z-10`} />
            {isOnline && (
              <span className="absolute w-2.5 h-2.5 rounded-full bg-emerald-500 animate-ping opacity-75" />
            )}
          </div>
          <h3 className="text-sm font-bold text-zinc-100 truncate tracking-tight uppercase">
            {data.label || 'SERVER NODE'}
          </h3>
        </div>
        <div className="text-[10px] font-mono text-zinc-500 bg-black/40 px-1.5 py-0.5 rounded border border-white/5">
          ID: {data.id || 'N/A'}
        </div>
      </div>

      {/* Body Content */}
      <div className="p-4 space-y-3">
        {/* TCP Flow Capsule */}
        <div className="bg-white/5 border border-white/5 rounded-lg px-3 py-2 flex items-center justify-center gap-2">
          <span className="text-[10px] font-bold text-indigo-400">TCP</span>
          <span className="text-[11px] text-zinc-300">入口</span>
          <svg className="w-3 h-3 text-zinc-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14 5l7 7m0 0l-7 7m7-7H3" />
          </svg>
          <span className="text-[11px] text-zinc-300">出口</span>
        </div>

        <div className="flex flex-col gap-1">
          <div className="text-[10px] text-zinc-500 flex justify-between">
            <span>NETWORK ADDR</span>
            <span className="text-zinc-300 font-mono">{data.ip || '0.0.0.0'}</span>
          </div>
        </div>

        {/* Speed Limit Indicator (Conditional) */}
        {(data.limit || data.speedLimit) && (
          <div className="mt-2 bg-amber-500/10 border border-amber-500/20 rounded-lg px-2.5 py-1.5 flex items-center gap-2">
            <svg className="w-3.5 h-3.5 text-amber-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
            </svg>
            <span className="text-[10px] font-bold text-amber-500 uppercase tracking-wider">
              限速 {data.limit || data.speedLimit}
            </span>
          </div>
        )}
      </div>

      {/* Connection Slots Labels & Handles */}
      <div className="absolute inset-y-0 -left-1 flex items-center">
        <div className="flex flex-col items-center">
          <Handle
            type="target"
            position={Position.Left}
            className="!w-3 !h-3 !bg-zinc-700 !border-2 !border-zinc-300 hover:!scale-125 transition-transform !static"
            isConnectable={isConnectable}
          />
          <span className="text-[8px] font-black text-zinc-600 mt-1 select-none">IN</span>
        </div>
      </div>

      <div className="absolute inset-y-0 -right-1 flex items-center">
        <div className="flex flex-col items-center">
          <Handle
            type="source"
            position={Position.Right}
            className="!w-3 !h-3 !bg-indigo-500 !border-2 !border-zinc-300 hover:!scale-125 transition-transform !static"
            isConnectable={isConnectable}
          />
          <span className="text-[8px] font-black text-zinc-600 mt-1 select-none">OUT</span>
        </div>
      </div>
    </div>
  );
});

