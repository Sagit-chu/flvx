import { useCallback, useEffect, useRef } from "react";

interface UseNodeOfflineTimersOptions {
  delayMs?: number;
  onNodeOffline: (nodeId: number) => void;
}

export const useNodeOfflineTimers = ({
  delayMs = 3000,
  onNodeOffline,
}: UseNodeOfflineTimersOptions) => {
  const timersRef = useRef<Map<number, ReturnType<typeof setTimeout>>>(
    new Map(),
  );

  const clearOfflineTimer = useCallback((nodeId: number) => {
    const timer = timersRef.current.get(nodeId);

    if (!timer) {
      return;
    }

    clearTimeout(timer);
    timersRef.current.delete(nodeId);
  }, []);

  const clearAllOfflineTimers = useCallback(() => {
    timersRef.current.forEach((timer) => {
      clearTimeout(timer);
    });
    timersRef.current.clear();
  }, []);

  const scheduleNodeOffline = useCallback(
    (nodeId: number) => {
      if (timersRef.current.has(nodeId)) {
        return;
      }

      const timer = setTimeout(() => {
        timersRef.current.delete(nodeId);
        onNodeOffline(nodeId);
      }, delayMs);

      timersRef.current.set(nodeId, timer);
    },
    [delayMs, onNodeOffline],
  );

  useEffect(() => {
    return () => {
      clearAllOfflineTimers();
    };
  }, [clearAllOfflineTimers]);

  return {
    clearOfflineTimer,
    scheduleNodeOffline,
    clearAllOfflineTimers,
  };
};
