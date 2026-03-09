export type NodeRenewalCycle = "" | "month" | "quarter" | "year";

export interface NodeRenewalSnapshot {
  cycle: NodeRenewalCycle;
  anchorTime?: number;
  nextDueTime?: number;
  diffDays?: number;
  state: "unset" | "expired" | "dueSoon" | "scheduled";
  label: string;
}

const addMonths = (timestamp: number, months: number): number => {
  const date = new Date(timestamp);
  const next = new Date(date);

  next.setMonth(next.getMonth() + months);

  return next.getTime();
};

const cycleToMonths = (cycle: NodeRenewalCycle): number => {
  switch (cycle) {
    case "month":
      return 1;
    case "quarter":
      return 3;
    case "year":
      return 12;
    default:
      return 0;
  }
};

export const getNodeRenewalCycleLabel = (cycle?: string): string => {
  switch (cycle) {
    case "month":
      return "月付";
    case "quarter":
      return "季付";
    case "year":
      return "年付";
    default:
      return "未设置";
  }
};

export const getNodeRenewalSnapshot = (
  anchorTime?: number,
  cycle?: string,
  warningDays = 7,
): NodeRenewalSnapshot => {
  const normalizedCycle =
    cycle === "month" || cycle === "quarter" || cycle === "year" ? cycle : "";

  if (!anchorTime || anchorTime <= 0 || !normalizedCycle) {
    return {
      cycle: normalizedCycle,
      anchorTime: anchorTime && anchorTime > 0 ? anchorTime : undefined,
      state: "unset",
      label: "未设置续费周期",
    };
  }

  const intervalMonths = cycleToMonths(normalizedCycle);
  let nextDueTime = anchorTime;

  while (nextDueTime < Date.now()) {
    const advanced = addMonths(nextDueTime, intervalMonths);

    if (advanced === nextDueTime) {
      break;
    }
    nextDueTime = advanced;
  }

  const diffDays = Math.ceil(
    (nextDueTime - Date.now()) / (1000 * 60 * 60 * 24),
  );

  if (diffDays <= 0) {
    return {
      cycle: normalizedCycle,
      anchorTime,
      nextDueTime,
      diffDays,
      state: "expired",
      label: "今天到期",
    };
  }

  if (diffDays <= warningDays) {
    return {
      cycle: normalizedCycle,
      anchorTime,
      nextDueTime,
      diffDays,
      state: "dueSoon",
      label: diffDays === 1 ? "明天续费" : `${diffDays}天后续费`,
    };
  }

  return {
    cycle: normalizedCycle,
    anchorTime,
    nextDueTime,
    diffDays,
    state: "scheduled",
    label: `${diffDays}天后续费`,
  };
};

export const formatNodeRenewalTime = (timestamp?: number): string => {
  if (!timestamp || timestamp <= 0) {
    return "未设置";
  }

  return new Date(timestamp).toLocaleString();
};
