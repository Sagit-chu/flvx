export const TUNNEL_QUALITY_INTERVAL_CONFIG_KEY =
  "monitor_tunnel_quality_interval_sec";
export const DEFAULT_TUNNEL_QUALITY_INTERVAL_SEC = 1;
export const MIN_TUNNEL_QUALITY_INTERVAL_SEC = 1;
export const MAX_TUNNEL_QUALITY_INTERVAL_SEC = 3600;

export const parseTunnelQualityIntervalSeconds = (value: unknown): number => {
  const seconds = Number(value);

  return Number.isInteger(seconds) &&
    seconds >= MIN_TUNNEL_QUALITY_INTERVAL_SEC &&
    seconds <= MAX_TUNNEL_QUALITY_INTERVAL_SEC
    ? seconds
    : DEFAULT_TUNNEL_QUALITY_INTERVAL_SEC;
};

export const validateTunnelQualityInterval = (value: string): string | null => {
  const normalized = value.trim();

  if (!normalized) {
    return "请输入探测间隔";
  }
  const seconds = Number(normalized);

  if (!Number.isInteger(seconds)) {
    return "探测间隔必须是整数";
  }
  if (
    seconds < MIN_TUNNEL_QUALITY_INTERVAL_SEC ||
    seconds > MAX_TUNNEL_QUALITY_INTERVAL_SEC
  ) {
    return `探测间隔必须在 ${MIN_TUNNEL_QUALITY_INTERVAL_SEC} 到 ${MAX_TUNNEL_QUALITY_INTERVAL_SEC} 秒之间`;
  }

  return null;
};

export const tunnelQualityIntervalLabel = (seconds: number): string =>
  seconds === 1 ? "每秒" : `每 ${seconds} 秒`;
