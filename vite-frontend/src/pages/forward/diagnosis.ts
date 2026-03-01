export interface ForwardDiagnosisEntry {
  success?: boolean;
  diagnosing?: boolean;
  description: string;
  nodeName: string;
  nodeId: string;
  targetIp: string;
  targetPort?: number;
  message?: string;
  averageTime?: number;
  packetLoss?: number;
  fromChainType?: number;
  fromInx?: number;
  toChainType?: number;
  toInx?: number;
}

export interface ForwardDiagnosisResult {
  forwardName: string;
  timestamp: number;
  results: ForwardDiagnosisEntry[];
}

export interface ForwardDiagnosisFallbackInput {
  forwardName: string;
  remoteAddr: string;
  description: string;
  message: string;
}

const pickPrimaryTargetIp = (remoteAddr: string): string => {
  return remoteAddr.split(",")[0] || "-";
};

export const buildForwardDiagnosisFallbackResult = ({
  forwardName,
  remoteAddr,
  description,
  message,
}: ForwardDiagnosisFallbackInput): ForwardDiagnosisResult => {
  return {
    forwardName,
    timestamp: Date.now(),
    results: [
      {
        success: false,
        description,
        nodeName: "-",
        nodeId: "-",
        targetIp: pickPrimaryTargetIp(remoteAddr),
        message,
      },
    ],
  };
};

export const getForwardDiagnosisQualityDisplay = (
  averageTime?: number,
  packetLoss?: number,
): {
  text: string;
  color: "success" | "primary" | "warning" | "danger";
} | null => {
  if (averageTime === undefined || packetLoss === undefined) {
    return null;
  }

  if (averageTime < 30 && packetLoss === 0) {
    return { text: "🚀 优秀", color: "success" };
  }

  if (averageTime < 50 && packetLoss === 0) {
    return { text: "✨ 很好", color: "success" };
  }

  if (averageTime < 100 && packetLoss < 1) {
    return { text: "👍 良好", color: "primary" };
  }

  if (averageTime < 150 && packetLoss < 2) {
    return { text: "😐 一般", color: "warning" };
  }

  if (averageTime < 200 && packetLoss < 5) {
    return { text: "😟 较差", color: "warning" };
  }

  return { text: "😵 很差", color: "danger" };
};
