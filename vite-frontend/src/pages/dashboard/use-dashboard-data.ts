import { useEffect, useState } from "react";
import toast from "react-hot-toast";

import {
  getAnnouncement,
  getUserPackageInfo,
  type AnnouncementData,
} from "@/api";
import type { ForwardApiItem } from "@/api/types";
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

interface DashboardDataState {
  loading: boolean;
  userInfo: DashboardUserInfo;
  userTunnels: DashboardUserTunnel[];
  forwardList: DashboardForward[];
  statisticsFlows: DashboardStatisticsFlow[];
  isAdmin: boolean;
  announcement: AnnouncementData | null;
}

const checkExpirationNotifications = (
  userInfo: DashboardUserInfo,
  tunnels: DashboardUserTunnel[],
) => {
  const notificationKey = `expiration-${userInfo.expTime}-${tunnels.map((t) => t.expTime).join(",")}`;
  const lastNotified = localStorage.getItem("lastNotified");

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
    localStorage.setItem("lastNotified", notificationKey);
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

export const useDashboardData = (): DashboardDataState => {
  const [loading, setLoading] = useState(true);
  const [userInfo, setUserInfo] = useState<DashboardUserInfo>(
    {} as DashboardUserInfo,
  );
  const [userTunnels, setUserTunnels] = useState<DashboardUserTunnel[]>([]);
  const [forwardList, setForwardList] = useState<DashboardForward[]>([]);
  const [statisticsFlows, setStatisticsFlows] = useState<DashboardStatisticsFlow[]>([]);
  const [isAdmin, setIsAdmin] = useState(false);
  const [announcement, setAnnouncement] = useState<AnnouncementData | null>(null);

  useEffect(() => {
    const loadAnnouncement = async () => {
      try {
        const res = await getAnnouncement();

        if (res.code === 0 && res.data && res.data.enabled === 1) {
          setAnnouncement(res.data);
        }
      } catch (error) {
        console.error("Failed to load announcement:", error);
      }
    };

    const loadPackageData = async () => {
      setLoading(true);
      try {
        const res = await getUserPackageInfo();

        if (res.code === 0) {
          const data = res.data;
          const normalizedTunnelPermissions = normalizeTunnelPermissions(
            data.tunnelPermissions || [],
          );
          const normalizedForwards = normalizeForwards(data.forwards || []);

          setUserInfo(data.userInfo || ({} as DashboardUserInfo));
          setUserTunnels(normalizedTunnelPermissions);
          setForwardList(normalizedForwards);
          setStatisticsFlows(data.statisticsFlows || []);

          checkExpirationNotifications(data.userInfo, normalizedTunnelPermissions);
        } else {
          toast.error(res.msg || "获取套餐信息失败");
        }
      } catch {
        toast.error("获取套餐信息失败");
      } finally {
        setLoading(false);
      }
    };

    setLoading(true);
    setUserInfo({} as DashboardUserInfo);
    setUserTunnels([]);
    setForwardList([]);
    setStatisticsFlows([]);
    setIsAdmin(getAdminFlag());

    loadPackageData();
    loadAnnouncement();
    localStorage.setItem("e", "/dashboard");
  }, []);

  return {
    loading,
    userInfo,
    userTunnels,
    forwardList,
    statisticsFlows,
    isAdmin,
    announcement,
  };
};
