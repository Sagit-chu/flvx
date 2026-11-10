import { Progress } from "@/shadcn-bridge/heroui/progress";

export interface NodeUpgradeProgressState {
  stage: string;
  percent: number;
  message: string;
}

const STAGE_LABELS: Record<string, string> = {
  downloading: "下载",
  verifying: "校验",
  installing: "安装 / 重启",
};

export function NodeUpgradeProgress({
  progress,
}: {
  progress?: NodeUpgradeProgressState;
}) {
  // The final installing event means the agent is restarting, not a durable
  // job state. Do not leave a stale "重启中 100%" banner after it reconnects.
  if (
    !progress ||
    (progress.stage === "installing" && progress.percent >= 100)
  ) {
    return null;
  }

  const percent = Number.isFinite(progress.percent)
    ? Math.min(100, Math.max(0, progress.percent))
    : 0;
  const stage = STAGE_LABELS[progress.stage];

  return (
    <div className="mt-2 min-w-40 max-w-xs break-words" role="status">
      <Progress
        showValueLabel
        aria-label={stage ? `升级${stage}进度` : "升级进度"}
        color="warning"
        label={progress.message || (stage ? `正在${stage}` : "正在升级")}
        size="sm"
        value={percent}
      />
    </div>
  );
}
