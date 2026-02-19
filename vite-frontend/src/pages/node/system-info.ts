export interface NodeSystemInfo {
  cpuUsage: number;
  memoryUsage: number;
  uploadTraffic: number;
  downloadTraffic: number;
  uploadSpeed: number;
  downloadSpeed: number;
  uptime: number;
}

type RawSystemInfo = Record<string, string | number | undefined>;

const toInteger = (value: string | number | undefined): number => {
  return Number.parseInt(String(value ?? 0), 10) || 0;
};

const toFloat = (value: string | number | undefined): number => {
  return Number.parseFloat(String(value ?? 0)) || 0;
};

const parseRawSystemInfo = (messageData: unknown): RawSystemInfo | null => {
  if (typeof messageData === "string") {
    try {
      const parsed = JSON.parse(messageData);

      if (parsed && typeof parsed === "object") {
        return parsed as RawSystemInfo;
      }

      return null;
    } catch {
      return null;
    }
  }

  if (messageData && typeof messageData === "object") {
    return messageData as RawSystemInfo;
  }

  return null;
};

export const buildNodeSystemInfo = (
  messageData: unknown,
  previous: NodeSystemInfo | null | undefined,
): NodeSystemInfo | null => {
  const raw = parseRawSystemInfo(messageData);

  if (!raw) {
    return null;
  }

  const uploadTraffic = toInteger(raw.bytes_transmitted);
  const downloadTraffic = toInteger(raw.bytes_received);
  const uptime = toInteger(raw.uptime);

  let uploadSpeed = 0;
  let downloadSpeed = 0;

  if (previous && previous.uptime) {
    const timeDiff = uptime - previous.uptime;

    if (timeDiff > 0 && timeDiff <= 10) {
      const uploadDiff = uploadTraffic - previous.uploadTraffic;
      const downloadDiff = downloadTraffic - previous.downloadTraffic;

      if (uploadTraffic >= previous.uploadTraffic && uploadDiff >= 0) {
        uploadSpeed = uploadDiff / timeDiff;
      }

      if (downloadTraffic >= previous.downloadTraffic && downloadDiff >= 0) {
        downloadSpeed = downloadDiff / timeDiff;
      }
    }
  }

  return {
    cpuUsage: toFloat(raw.cpu_usage),
    memoryUsage: toFloat(raw.memory_usage),
    uploadTraffic,
    downloadTraffic,
    uploadSpeed,
    downloadSpeed,
    uptime,
  };
};
