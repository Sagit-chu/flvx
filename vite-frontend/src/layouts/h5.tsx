import React, { useState, useEffect } from "react";
import { useNavigate, useLocation } from "react-router-dom";
import toast from "react-hot-toast";

import { BrandLogo } from "@/components/brand-logo";
import { siteConfig } from "@/config/site";
import { getMonitorAccess } from "@/api";
import { getAdminFlag } from "@/utils/session";
import { useScrollTopOnPathChange } from "@/hooks/useScrollTopOnPathChange";

interface TabItem {
  path: string;
  label: string;
  icon: React.ReactNode;
  adminOnly?: boolean;
}

export default function H5Layout({ children }: { children: React.ReactNode }) {
  const navigate = useNavigate();
  const location = useLocation();
  const [isAdmin, setIsAdmin] = useState(() => getAdminFlag());
  const [monitorAllowed, setMonitorAllowed] = useState<boolean | null>(null);
  const [monitorAccessReason, setMonitorAccessReason] = useState<string | null>(
    null,
  );

  useScrollTopOnPathChange();

  // Tabbar配置
  const tabItems: TabItem[] = [
    {
      path: "/dashboard",
      label: "首页",
      icon: (
        <svg className="w-6 h-6" fill="currentColor" viewBox="0 0 20 20">
          <path d="M10.707 2.293a1 1 0 00-1.414 0l-7 7a1 1 0 001.414 1.414L4 10.414V17a1 1 0 001 1h2a1 1 0 001-1v-2a1 1 0 011-1h2a1 1 0 011 1v2a1 1 0 001 1h2a1 1 0 001-1v-6.586l.293.293a1 1 0 001.414-1.414l-7-7z" />
        </svg>
      ),
    },
    {
      path: "/forward",
      label: "规则",
      icon: (
        <svg className="w-6 h-6" fill="currentColor" viewBox="0 0 20 20">
          <path
            clipRule="evenodd"
            d="M3 17a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zm3.293-7.707a1 1 0 011.414 0L9 10.586V3a1 1 0 112 0v7.586l1.293-1.293a1 1 0 111.414 1.414l-3 3a1 1 0 01-1.414 0l-3-3a1 1 0 010-1.414z"
            fillRule="evenodd"
          />
        </svg>
      ),
    },
    {
      path: "/tunnel",
      label: "隧道",
      icon: (
        <svg className="w-6 h-6" fill="currentColor" viewBox="0 0 20 20">
          <path
            clipRule="evenodd"
            d="M12.586 4.586a2 2 0 112.828 2.828l-3 3a2 2 0 01-2.828 0 1 1 0 00-1.414 1.414 4 4 0 005.656 0l3-3a4 4 0 00-5.656-5.656l-1.5 1.5a1 1 0 101.414 1.414l1.5-1.5zm-5 5a2 2 0 012.828 0 1 1 0 101.414-1.414 4 4 0 00-5.656 0l-3 3a4 4 0 105.656 5.656l1.5-1.5a1 1 0 10-1.414-1.414l-1.5 1.5a2 2 0 11-2.828-2.828l3-3z"
            fillRule="evenodd"
          />
        </svg>
      ),
      adminOnly: true,
    },
    {
      path: "/node",
      label: "节点",
      icon: (
        <svg className="w-6 h-6" fill="currentColor" viewBox="0 0 20 20">
          <path
            clipRule="evenodd"
            d="M3 3a1 1 0 000 2v8a2 2 0 002 2h2.586l-1.293 1.293a1 1 0 101.414 1.414L10 15.414l2.293 2.293a1 1 0 001.414-1.414L12.414 15H15a2 2 0 002-2V5a1 1 0 100-2H3zm11.707 4.707a1 1 0 00-1.414-1.414L10 9.586 8.707 8.293a1 1 0 00-1.414 0l-2 2a1 1 0 101.414 1.414L8 10.414l1.293 1.293a1 1 0 001.414 0l4-4z"
            fillRule="evenodd"
          />
        </svg>
      ),
      adminOnly: true,
    },
    {
      path: "/monitor",
      label: "监控",
      icon: (
        <svg className="w-6 h-6" fill="currentColor" viewBox="0 0 20 20">
          <path
            clipRule="evenodd"
            d="M3 3a1 1 0 000 2v11a1 1 0 001 1h13a1 1 0 100-2H5V5a1 1 0 00-1-1H3zm13.707 4.293a1 1 0 00-1.414 0L12 10.586 10.707 9.293a1 1 0 00-1.414 0L7 11.586l-1.293-1.293a1 1 0 10-1.414 1.414l2 2a1 1 0 001.414 0L10 11.414l1.293 1.293a1 1 0 001.414 0l3-3a1 1 0 000-1.414z"
            fillRule="evenodd"
          />
        </svg>
      ),
    },
    {
      path: "/profile",
      label: "我的",
      icon: (
        <svg className="w-6 h-6" fill="currentColor" viewBox="0 0 20 20">
          <path d="M9 6a3 3 0 11-6 0 3 3 0 016 0zM17 6a3 3 0 11-6 0 3 3 0 016 0zM12.93 17c.046-.327.07-.66.07-1a6.97 6.97 0 00-1.5-4.33A5 5 0 0119 16v1h-6.07zM6 11a5 5 0 015 5v1H1v-1a5 5 0 015-5z" />
        </svg>
      ),
    },
  ];

  useEffect(() => {
    const adminFlag = getAdminFlag();

    setIsAdmin(adminFlag);
    if (adminFlag) {
      setMonitorAllowed(true);
      setMonitorAccessReason(null);

      return;
    }

    let cancelled = false;
    (async () => {
      try {
        const res = await getMonitorAccess();
        if (cancelled) return;
        if (res.code === 0 && res.data) {
          setMonitorAllowed(Boolean(res.data.allowed));
          setMonitorAccessReason(
            res.data.allowed ? null : (res.data.reason || null),
          );
          return;
        }
        setMonitorAllowed(true);
        setMonitorAccessReason(null);
      } catch {
        if (cancelled) return;
        setMonitorAllowed(true);
        setMonitorAccessReason(null);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, []);

  // Tab点击处理
  const handleTabClick = (path: string) => {
    if (path === "/monitor" && monitorAllowed !== true) {
      if (monitorAllowed == null) {
        toast("正在检查监控权限，请稍后重试");

        return;
      }

      const hint =
        monitorAccessReason === "need_admin_grant"
          ? "暂无监控权限，请联系管理员授权"
          : "暂无监控权限";

      toast.error(hint);

      return;
    }
    navigate(path);
  };

  // 过滤tab项（根据权限）
  const filteredTabItems = tabItems.filter(
    (item) => !item.adminOnly || isAdmin,
  );

  return (
    <div className="flex flex-col min-h-screen bg-mesh-gradient">
      {/* 顶部导航栏 */}
      <header className="bg-white/20 dark:bg-zinc-900/20 backdrop-blur-xl shadow-sm border-b border-white/80 dark:border-white/10 h-14 safe-top flex-shrink-0 flex items-center justify-between px-4 relative z-10">
        <div className="flex items-center gap-2">
          <BrandLogo size={20} />
          <h1 className="text-sm font-bold text-foreground">
            {siteConfig.name}
          </h1>
        </div>

        <div className="flex items-center gap-2" />
      </header>

      {/* 主内容区域 */}
      <main className="flex-1">{children}</main>

      {/* 用于给固定 Tabbar 腾出空间的占位元素 */}
      <div aria-hidden className="h-[calc(4rem+var(--safe-area-bottom))]" />

      {/* 底部Tabbar */}
      <nav className="bg-white/20 dark:bg-zinc-900/20 backdrop-blur-2xl border-t border-white/80 dark:border-white/10 h-[calc(4rem+var(--safe-area-bottom))] flex-shrink-0 flex items-center justify-around px-2 fixed bottom-0 left-0 right-0 z-30">
        {filteredTabItems.map((item) => {
          const isActive = location.pathname === item.path;
          const isMonitor = item.path === "/monitor";
          const isMonitorBlocked = isMonitor && monitorAllowed !== true;

          return (
            <button
              key={item.path}
              className={`
                flex flex-col items-center justify-center flex-1 h-full pb-[var(--safe-area-bottom)]
                transition-colors duration-200 min-h-[44px]
                ${isMonitorBlocked ? "opacity-60" : ""}
                ${
                  isActive
                    ? "text-primary-600 dark:text-primary-400"
                    : isMonitorBlocked
                      ? "text-gray-500 dark:text-gray-400"
                      : "text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200"
                }
              `}
              onClick={() => handleTabClick(item.path)}
            >
              <div className="flex-shrink-0 mb-1">{item.icon}</div>
              <span className="text-xs font-medium">{item.label}</span>
            </button>
          );
        })}
      </nav>
    </div>
  );
}
