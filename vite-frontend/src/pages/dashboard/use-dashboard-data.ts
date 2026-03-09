import type { ForwardApiItem, NodeApiItem } from "@/api/types";

import { useCallback, useEffect, useRef, useState } from "react";
import toast from "react-hot-toast";

import {
  getAnnouncement,
  getDashboardNodeExpiryList,
  getUserPackageInfo,
  type AnnouncementData,
} from "@/api";
import { getNodeRenewalSnapshot } from "@/pages/node/renewal";
import { getAdminFlag } from "@/utils/session";

export interface DashboardUserInfo {
  flow: number;
  inFlow: number;
  outFlow: number;
  num: number;
  expTime?: string | number;
  flowResetTime?: number;
}

export interface DashboardUserTunnel {
  id: number;
  tunnelId: number;
  tunnelName: string;
  flow: number;
  inFlow: number;
  outFlow: number;
  num: number;
  expTime?: number;
  flowResetTime?: number;
  tunnelFlow?: number;
}

export interface DashboardForward {
  id: number;
  name: string;
  tunnelId: number;
  tunnelName: string;
  inIp: string;
  inPort: number;
  remoteAddr: string;
  inFlow: number;
  outFlow: number;
}

export interface DashboardStatisticsFlow {
  id: number;
  userId: number;
  flow: number;
  totalFlow: number;
  time: string;
}

export interface DashboardNodeExpiryItem {
  id: number;
  name: string;
  remark?: string;
  expiryTime?: number;
  renewalCycle?: "month" | "quarter" | "year" | "";
}

const normalizeDashboardRenewalCycle = (
  value: unknown,
): DashboardNodeExpiryItem["renewalCycle"] => {
  return value === "month" || value === "quarter" || value === "year"
    ? value
    : "";
};

const DASHBOARD_POLL_INTERVAL_MS = 5000;
const EXPIRATION_NOTIFICATION_STORAGE_KEY =
  "dashboard:last-expiration-notification";

const buildExpirationNotificationKey = (
  userInfo: DashboardUserInfo,
  tunnels: DashboardUserTunnel[],
) => {
  const userExpTime = userInfo.expTime ?? "permanent";
  const tunnelExpirationKey = [...tunnels]
    .map((tunnel) => `${tunnel.tunnelId}:${tunnel.expTime ?? "permanent"}`)
    .sort()
    .join("|");

  return `user:${userExpTime};tunnels:${tunnelExpirationKey}`;
};

interface DashboardDataState {
  loading: boolean;
  userInfo: DashboardUserInfo;
  userTunnels: DashboardUserTunnel[];
  forwardList: DashboardForward[];
  statisticsFlows: DashboardStatisticsFlow[];
  nodeExpiryReminders: DashboardNodeExpiryItem[];
  isAdmin: boolean;
  announcement: AnnouncementData | null;
}

const checkExpirationNotifications = (
  userInfo: DashboardUserInfo,
  tunnels: DashboardUserTunnel[],
) => {
  const notificationKey = buildExpirationNotificationKey(userInfo, tunnels);
  const lastNotified = localStorage.getItem(
    EXPIRATION_NOTIFICATION_STORAGE_KEY,
  );

  if (lastNotified === notificationKey) {
    return;
  }

  let hasNotification = false;

  if (userInfo.expTime) {
    const expDate = new Date(userInfo.expTime);
    const now = new Date();

    if (!isNaN(expDate.getTime()) && expDate > now) {
      const diffTime = expDate.getTime() - now.getTime();
      const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24));

      if (diffDays <= 7 && diffDays > 0) {
        hasNotification = true;
        if (diffDays === 1) {
          toast("账户将于明天过期，请及时续费", {
            icon: "⚠️",
            duration: 6000,
            style: { background: "#f59e0b", color: "#fff" },
          });
        } else {
          toast(`账户将于${diffDays}天后过期，请及时续费`, {
            icon: "⚠️",
            duration: 6000,
            style: { background: "#f59e0b", color: "#fff" },
          });
        }
      } else if (diffDays <= 0) {
        hasNotification = true;
        toast("账户已过期，请立即续费", {
          icon: "⚠️",
          duration: 8000,
          style: { background: "#ef4444", color: "#fff" },
        });
      }
    }
  }

  tunnels.forEach((tunnel) => {
    if (!tunnel.expTime) {
      return;
    }

    const expDate = new Date(tunnel.expTime);
    const now = new Date();

    if (!isNaN(expDate.getTime()) && expDate > now) {
      const diffTime = expDate.getTime() - now.getTime();
      const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24));

      if (diffDays <= 7 && diffDays > 0) {
        hasNotification = true;
        if (diffDays === 1) {
          toast(`隧道"${tunnel.tunnelName}"将于明天过期`, {
            icon: "⚠️",
            duration: 5000,
            style: { background: "#f59e0b", color: "#fff" },
          });
        } else {
          toast(`隧道"${tunnel.tunnelName}"将于${diffDays}天后过期`, {
            icon: "⚠️",
            duration: 5000,
            style: { background: "#f59e0b", color: "#fff" },
          });
        }
      } else if (diffDays <= 0) {
        hasNotification = true;
        toast(`隧道"${tunnel.tunnelName}"已过期`, {
          icon: "⚠️",
          duration: 6000,
          style: { background: "#ef4444", color: "#fff" },
        });
      }
    }
  });

  if (hasNotification) {
    localStorage.setItem(EXPIRATION_NOTIFICATION_STORAGE_KEY, notificationKey);
  }
};

const normalizeForwards = (items: ForwardApiItem[]) => {
  return (items || []).map((item) => ({
    ...item,
    name: item.name || "",
    tunnelId: item.tunnelId ?? 0,
    tunnelName: item.tunnelName || "",
    inIp: item.inIp || "",
    inPort: item.inPort || 0,
    remoteAddr: item.remoteAddr || "",
    inFlow: item.inFlow || 0,
    outFlow: item.outFlow || 0,
  }));
};

const normalizeTunnelPermissions = (items: DashboardUserTunnel[]) => {
  return (items || []).map((item) => ({
    ...item,
    inFlow: item.inFlow ?? 0,
    outFlow: item.outFlow ?? 0,
  }));
};

const normalizeNodeExpiryReminders = (items: NodeApiItem[]) => {
  const now = Date.now();
  const warningWindowMs = 7 * 24 * 60 * 60 * 1000;

  return (items || [])
    .map((item) => ({
      id: item.id,
      name: item.name || "",
      remark: typeof item.remark === "string" ? item.remark : "",
      renewalCycle: normalizeDashboardRenewalCycle(item.renewalCycle),
      expiryTime:
        typeof item.expiryTime === "number" && item.expiryTime > 0
          ? item.expiryTime
          : undefined,
    }))
    .filter((item) => {
      if (!item.expiryTime || !item.renewalCycle) return false;
      const snapshot = getNodeRenewalSnapshot(
        item.expiryTime,
        item.renewalCycle,
      );

      if (!snapshot.nextDueTime) return false;

      return snapshot.nextDueTime <= now + warningWindowMs;
    })
    .sort((a, b) => {
      const aDue =
        getNodeRenewalSnapshot(a.expiryTime, a.renewalCycle).nextDueTime || 0;
      const bDue =
        getNodeRenewalSnapshot(b.expiryTime, b.renewalCycle).nextDueTime || 0;

      return aDue - bDue;
    });
};

export const useDashboardData = (): DashboardDataState => {
  const [loading, setLoading] = useState(true);
  const [userInfo, setUserInfo] = useState<DashboardUserInfo>(
    {} as DashboardUserInfo,
  );
  const [userTunnels, setUserTunnels] = useState<DashboardUserTunnel[]>([]);
  const [forwardList, setForwardList] = useState<DashboardForward[]>([]);
  const [statisticsFlows, setStatisticsFlows] = useState<
    DashboardStatisticsFlow[]
  >([]);
  const [nodeExpiryReminders, setNodeExpiryReminders] = useState<
    DashboardNodeExpiryItem[]
  >([]);
  const [isAdmin, setIsAdmin] = useState(false);
  const [announcement, setAnnouncement] = useState<AnnouncementData | null>(
    null,
  );
  const isMountedRef = useRef(true);
  const packageRequestInFlightRef = useRef(false);
  const nodeExpiryRequestInFlightRef = useRef(false);

  const applyPackageData = useCallback(
    (data: {
      userInfo?: DashboardUserInfo;
      tunnelPermissions?: DashboardUserTunnel[];
      forwards?: ForwardApiItem[];
      statisticsFlows?: DashboardStatisticsFlow[];
    }) => {
      const normalizedTunnelPermissions = normalizeTunnelPermissions(
        data.tunnelPermissions || [],
      );
      const normalizedForwards = normalizeForwards(data.forwards || []);

      if (!isMountedRef.current) {
        return;
      }

      setUserInfo(data.userInfo || ({} as DashboardUserInfo));
      setUserTunnels(normalizedTunnelPermissions);
      setForwardList(normalizedForwards);
      setStatisticsFlows(data.statisticsFlows || []);

      checkExpirationNotifications(
        data.userInfo || ({} as DashboardUserInfo),
        normalizedTunnelPermissions,
      );
    },
    [],
  );

  const loadPackageData = useCallback(
    async ({ silent = false, notifyOnError = false } = {}) => {
      if (packageRequestInFlightRef.current) {
        return;
      }

      packageRequestInFlightRef.current = true;

      if (!silent && isMountedRef.current) {
        setLoading(true);
      }

      try {
        const res = await getUserPackageInfo();

        if (res.code === 0) {
          applyPackageData(res.data || {});
        } else if (notifyOnError) {
          toast.error(res.msg || "获取套餐信息失败");
        }
      } catch {
        if (notifyOnError) {
          toast.error("获取套餐信息失败");
        }
      } finally {
        packageRequestInFlightRef.current = false;

        if (!silent && isMountedRef.current) {
          setLoading(false);
        }
      }
    },
    [applyPackageData],
  );

  const loadAnnouncement = useCallback(async () => {
    try {
      const res = await getAnnouncement();

      if (!isMountedRef.current) {
        return;
      }

      if (res.code === 0 && res.data && res.data.enabled === 1) {
        setAnnouncement(res.data);
      } else {
        setAnnouncement(null);
      }
    } catch {
      if (isMountedRef.current) {
        setAnnouncement(null);
      }
    }
  }, []);

  const loadNodeExpiryData = useCallback(async () => {
    if (nodeExpiryRequestInFlightRef.current) {
      return;
    }

    nodeExpiryRequestInFlightRef.current = true;

    try {
      const res = await getDashboardNodeExpiryList();

      if (!isMountedRef.current) {
        return;
      }

      if (res.code === 0 && Array.isArray(res.data)) {
        setNodeExpiryReminders(normalizeNodeExpiryReminders(res.data));
      }
    } catch {
    } finally {
      nodeExpiryRequestInFlightRef.current = false;
    }
  }, []);

  useEffect(() => {
    isMountedRef.current = true;
    const adminFlag = getAdminFlag();

    setIsAdmin(adminFlag);

    void loadPackageData({ notifyOnError: true });
    void loadAnnouncement();
    if (adminFlag) {
      void loadNodeExpiryData();
    }
    localStorage.setItem("e", "/dashboard");

    return () => {
      isMountedRef.current = false;
    };
  }, [loadAnnouncement, loadNodeExpiryData, loadPackageData]);

  useEffect(() => {
    if (typeof document === "undefined") {
      return;
    }

    const handleVisibilityChange = () => {
      if (document.visibilityState === "visible") {
        void loadPackageData({ silent: true });
        if (isAdmin) {
          void loadNodeExpiryData();
        }
      }
    };

    const interval = window.setInterval(() => {
      if (document.visibilityState !== "visible") {
        return;
      }

      void loadPackageData({ silent: true });
      if (isAdmin) {
        void loadNodeExpiryData();
      }
    }, DASHBOARD_POLL_INTERVAL_MS);

    document.addEventListener("visibilitychange", handleVisibilityChange);

    return () => {
      window.clearInterval(interval);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    };
  }, [isAdmin, loadNodeExpiryData, loadPackageData]);

  return {
    loading,
    userInfo,
    userTunnels,
    forwardList,
    statisticsFlows,
    nodeExpiryReminders,
    isAdmin,
    announcement,
  };
};
