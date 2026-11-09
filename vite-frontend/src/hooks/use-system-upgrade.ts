import { useEffect, useRef, useState } from "react";

import { getSystemUpgradeStatus, runSystemUpgrade } from "@/api";
import {
  createUpgradeMonitor,
  initialUpgradeState,
  upgradeIsBlocked,
} from "@/utils/system-upgrade-monitor";

export function useSystemUpgrade(enabled: boolean) {
  const [state, setState] = useState(initialUpgradeState);
  const monitor = useRef<ReturnType<typeof createUpgradeMonitor> | null>(null);

  useEffect(() => {
    if (!enabled) return;
    const instance = createUpgradeMonitor(
      { status: getSystemUpgradeStatus, run: runSystemUpgrade },
      setState,
    );

    monitor.current = instance;
    instance.refresh();

    return () => {
      instance.dispose();
      monitor.current = null;
    };
  }, [enabled]);

  return {
    ...state,
    blocked: upgradeIsBlocked(state),
    refresh: () => monitor.current?.refresh(),
    submit: (version?: string, channel?: "stable" | "dev") =>
      monitor.current?.submit(version, channel) ?? Promise.resolve(false),
  };
}
