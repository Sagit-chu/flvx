import type {
  TunnelRedeployRow,
  TunnelRedeployState,
} from "./use-tunnel-redeploy";

import { Button } from "@/shadcn-bridge/heroui/button";
import { Progress } from "@/shadcn-bridge/heroui/progress";
import { Spinner } from "@/shadcn-bridge/heroui/spinner";

export function TunnelRedeployStatus({ row }: { row?: TunnelRedeployRow }) {
  if (!row) return null;

  const text = {
    queued: "等待下发",
    running: "正在下发配置…",
    success: "配置下发成功",
    failed: "配置下发失败",
    unknown: "下发结果未确认",
  }[row.status];
  const color =
    row.status === "failed"
      ? "text-danger"
      : row.status === "success"
        ? "text-success"
        : row.status === "unknown"
          ? "text-warning"
          : "text-default-500";

  return (
    <div className={`mt-1 flex items-start gap-1.5 text-xs ${color}`}>
      {row.status === "running" && <Spinner size="sm" />}
      <span className="max-w-xs break-words" title={row.reason}>
        {text}
        {row.reason ? `：${row.reason}` : ""}
      </span>
    </div>
  );
}

export function TunnelRedeploySummary({
  state,
  onDismiss,
}: {
  state: TunnelRedeployState | null;
  onDismiss: () => void;
}) {
  if (!state) return null;

  const { progress, running, error, currentName } = state;
  const label = `${error ? "已确认" : "已完成"} ${progress.completed}/${progress.total} 条，成功 ${progress.success} 条，失败 ${progress.failed} 条`;
  const percent =
    progress.total > 0 ? (progress.completed / progress.total) * 100 : 0;

  return (
    <div
      className="mb-4 space-y-2 rounded-xl border border-divider bg-content1 p-3"
      role="status"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 space-y-1 break-words text-sm">
          <p className="font-medium">
            {running
              ? "正在下发隧道配置"
              : error
                ? "下发进度已中断"
                : "隧道配置下发结束"}
          </p>
          <p className="text-default-500">{label}</p>
          {currentName && <p>当前：{currentName}</p>}
          {running && (
            <p className="text-xs text-default-500">
              进度按已完成的隧道统计，当前隧道内部步骤不计百分比。请保留此页面。
            </p>
          )}
          {error && <p className="text-warning">{error}</p>}
        </div>
        {!running && (
          <Button
            aria-label="关闭下发结果"
            size="sm"
            variant="light"
            onPress={onDismiss}
          >
            关闭
          </Button>
        )}
      </div>
      <Progress
        showValueLabel
        aria-label="隧道配置下发进度"
        color={error || progress.failed > 0 ? "warning" : "primary"}
        size="sm"
        value={percent}
      />
    </div>
  );
}
