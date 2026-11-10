import { useEffect, useRef, useState } from "react";

import {
  batchRedeployTunnelsStream,
  type TunnelRedeployProgress,
} from "@/api/tunnel-redeploy-stream";
import { extractApiErrorMessage } from "@/api/error-message";

export interface TunnelRedeployRow {
  status: "queued" | "running" | "success" | "failed" | "unknown";
  reason?: string;
}

export interface TunnelRedeployState {
  running: boolean;
  progress: TunnelRedeployProgress;
  rows: Record<number, TunnelRedeployRow>;
  currentName?: string;
  error?: string;
}

export function useTunnelRedeploy() {
  const [state, setState] = useState<TunnelRedeployState | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => () => abortRef.current?.abort(), []);

  const run = async (ids: number[]) => {
    // Guard synchronously, before React has rendered a disabled button.
    if (abortRef.current || ids.length === 0) return null;

    const controller = new AbortController();

    abortRef.current = controller;
    setState({
      running: true,
      progress: { total: ids.length, completed: 0, success: 0, failed: 0 },
      rows: Object.fromEntries(ids.map((id) => [id, { status: "queued" }])),
    });
    try {
      const result = await batchRedeployTunnelsStream(
        ids,
        {
          onStart: (progress) => {
            setState((prev) => prev && { ...prev, progress });
          },
          onProcessing: ({ id, name, progress }) => {
            setState(
              (prev) =>
                prev && {
                  ...prev,
                  progress,
                  currentName: name || `隧道 #${id}`,
                  rows: { ...prev.rows, [id]: { status: "running" } },
                },
            );
          },
          onItem: ({ result: item, progress }) => {
            setState(
              (prev) =>
                prev && {
                  ...prev,
                  progress,
                  currentName: undefined,
                  rows: {
                    ...prev.rows,
                    [item.id]: {
                      status: item.success ? "success" : "failed",
                      reason: item.reason,
                    },
                  },
                },
            );
          },
        },
        controller.signal,
      );

      if (controller.signal.aborted) return null;
      setState(
        (prev) => prev && { ...prev, running: false, currentName: undefined },
      );

      return result;
    } catch (error) {
      if (controller.signal.aborted) return null;
      setState(
        (prev) =>
          prev && {
            ...prev,
            running: false,
            currentName: undefined,
            error: extractApiErrorMessage(
              error,
              "下发进度中断，请检查未确认的结果后再重试",
            ),
            rows: Object.fromEntries(
              Object.entries(prev.rows).map(([id, row]) => [
                id,
                row.status === "queued" || row.status === "running"
                  ? { status: "unknown" }
                  : row,
              ]),
            ),
          },
      );
      throw error;
    } finally {
      if (abortRef.current === controller) abortRef.current = null;
    }
  };

  const clear = () => {
    if (!abortRef.current) setState(null);
  };

  return { state, run, clear };
}
