import React, { useState, useEffect } from "react";
import { useNavigate, useLocation } from "react-router-dom";
import { toast } from "react-hot-toast";
import { AnimatePresence, motion } from "framer-motion";

import { Button } from "@/shadcn-bridge/heroui/button";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  useDisclosure,
} from "@/shadcn-bridge/heroui/modal";
import { Input } from "@/shadcn-bridge/heroui/input";
import { BrandLogo } from "@/components/brand-logo";
import { VersionFooter } from "@/components/version-footer";
import { getMonitorAccess, updatePassword } from "@/api";
import { safeLogout } from "@/utils/logout";
import { siteConfig } from "@/config/site";
import { useMobileBreakpoint } from "@/hooks/useMobileBreakpoint";
import { getAdminFlag } from "@/utils/session";

interface MenuItem {
  path: string;
  label: string;
  icon: React.ReactNode;
  adminOnly?: boolean;
}

interface PasswordForm {
  newUsername: string;
  currentPassword: string;
  newPassword: string;
  confirmPassword: string;
}

export default function AdminLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const navigate = useNavigate();
  const location = useLocation();
  const { isOpen, onOpenChange } = useDisclosure();
  const [mobileMenuVisible, setMobileMenuVisible] = useState(false);
  const [isCollapsed, setIsCollapsed] = useState(
    () => localStorage.getItem("sidebar_collapsed") === "true",
  );
  const [isAdmin, setIsAdmin] = useState(() => getAdminFlag());
  const [monitorAllowed, setMonitorAllowed] = useState<boolean | null>(null);
  const [monitorAccessReason, setMonitorAccessReason] = useState<string | null>(
    null,
  );
  const [passwordLoading, setPasswordLoading] = useState(false);
  const [passwordForm, setPasswordForm] = useState<PasswordForm>({
    newUsername: "",
    currentPassword: "",
    newPassword: "",
    confirmPassword: "",
  });
  const isMobile = useMobileBreakpoint();

  // 菜单项配置
  const menuItems: MenuItem[] = [
    {
      path: "/dashboard",
      label: "仪表",
      icon: (
        <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
          <path d="M3 4a1 1 0 011-1h12a1 1 0 011 1v2a1 1 0 01-1 1H4a1 1 0 01-1-1V4zM3 10a1 1 0 011-1h6a1 1 0 011 1v6a1 1 0 01-1 1H4a1 1 0 01-1-1v-6zM14 9a1 1 0 00-1 1v6a1 1 0 001 1h2a1 1 0 001-1v-6a1 1 0 00-1-1h-2z" />
        </svg>
      ),
    },
    {
      path: "/forward",
      label: "规则",
      icon: (
        <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
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
        <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
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
        <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
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
        <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
          <path
            clipRule="evenodd"
            d="M3 3a1 1 0 000 2v11a1 1 0 001 1h13a1 1 0 100-2H5V5a1 1 0 00-1-1H3zm13.707 4.293a1 1 0 00-1.414 0L12 10.586 10.707 9.293a1 1 0 00-1.414 0L7 11.586l-1.293-1.293a1 1 0 10-1.414 1.414l2 2a1 1 0 001.414 0L10 11.414l1.293 1.293a1 1 0 001.414 0l3-3a1 1 0 000-1.414z"
            fillRule="evenodd"
          />
        </svg>
      ),
    },
    {
      path: "/limit",
      label: "限速",
      icon: (
        <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
          <path
            clipRule="evenodd"
            d="M10 18a8 8 0 100-16 8 8 0 000 16zm1-12a1 1 0 10-2 0v4a1 1 0 00.293.707l2.828 2.829a1 1 0 101.415-1.415L11 9.586V6z"
            fillRule="evenodd"
          />
        </svg>
      ),
      adminOnly: true,
    },
    {
      path: "/user",
      label: "用户",
      icon: (
        <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
          <path d="M9 6a3 3 0 11-6 0 3 3 0 016 0zM17 6a3 3 0 11-6 0 3 3 0 016 0zM12.93 17c.046-.327.07-.66.07-1a6.97 6.97 0 00-1.5-4.33A5 5 0 0119 16v1h-6.07zM6 11a5 5 0 015 5v1H1v-1a5 5 0 015-5z" />
        </svg>
      ),
      adminOnly: true,
    },
    {
      path: "/group",
      label: "分组",
      icon: (
        <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
          <path d="M10 2a3 3 0 100 6 3 3 0 000-6zM4 9a3 3 0 100 6 3 3 0 000-6zm12 0a3 3 0 100 6 3 3 0 000-6M4 16a2 2 0 00-2 2h4a2 2 0 00-2-2zm12 0a2 2 0 00-2 2h4a2 2 0 00-2-2zm-6 0a2 2 0 00-2 2h4a2 2 0 00-2-2z" />
        </svg>
      ),
      adminOnly: true,
    },
    {
      path: "/panel-sharing",
      label: "共享",
      icon: (
        <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
          <path d="M15 8a3 3 0 10-2.977-2.63l-4.94 2.47a3 3 0 100 4.319l4.94 2.47a3 3 0 10.895-1.789l-4.94-2.47a3.027 3.027 0 000-.74l4.94-2.47C13.456 7.68 14.19 8 15 8z" />
        </svg>
      ),
      adminOnly: true,
    },
    {
      path: "/config",
      label: "设置",
      icon: (
        <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 20 20">
          <path
            clipRule="evenodd"
            d="M11.49 3.17c-.38-1.56-2.6-1.56-2.98 0a1.532 1.532 0 01-2.286.948c-1.372-.836-2.942.734-2.106 2.106.54.886.061 2.042-.947 2.287-1.561.379-1.561 2.6 0 2.978a1.532 1.532 0 01.947 2.287c-.836 1.372.734 2.942 2.106 2.106a1.532 1.532 0 012.287.947c.379 1.561 2.6 1.561 2.978 0a1.533 1.533 0 012.287-.947c1.372.836 2.942-.734 2.106-2.106a1.533 1.533 0 01.947-2.287c1.561-.379 1.561-2.6 0-2.978a1.532 1.532 0 01-.947-2.287c.836-1.372-.734-2.942-2.106-2.106a1.532 1.532 0 01-2.287-.947zM10 13a3 3 0 100-6 3 3 0 000 6z"
            fillRule="evenodd"
          />
        </svg>
      ),
      adminOnly: true,
    },
  ];

  useEffect(() => {
    // 获取用户信息
    const adminFlag = getAdminFlag();

    setIsAdmin(adminFlag);

    // Monitor permission is not strictly role-based; non-admin users may be
    // granted access explicitly. Fetch a lightweight capability flag so we can
    // avoid a confusing 403 navigation.
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
        // Fail open to preserve legacy navigation behavior.
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

  useEffect(() => {
    if (!isMobile) {
      setMobileMenuVisible(false);
    }
  }, [isMobile]);

  // 退出登录
  const handleLogout = () => {
    safeLogout();
    navigate("/");
  };

  // 切换移动端菜单
  const toggleMobileMenu = () => {
    setMobileMenuVisible(!mobileMenuVisible);
  };

  // 隐藏移动端菜单
  const hideMobileMenu = () => {
    setMobileMenuVisible(false);
  };

  // 切换折叠状态
  const toggleCollapse = () => {
    const newCollapsed = !isCollapsed;

    setIsCollapsed(newCollapsed);
    localStorage.setItem("sidebar_collapsed", newCollapsed.toString());
  };

  // 菜单点击处理
  const handleMenuClick = (path: string) => {
    if (path === "/monitor" && monitorAllowed !== true) {
      if (monitorAllowed == null) {
        toast("正在检查监控权限，请稍后重试");

        return;
      }

      const hint =
        monitorAccessReason === "need_admin_grant"
          ? "暂无监控权限，请联系管理员在用户页面授予监控权限"
          : "暂无监控权限，请联系管理员授权";

      toast.error(hint);

      return;
    }

    navigate(path);
    if (isMobile) {
      hideMobileMenu();
    }
  };

  // 密码表单验证
  const validatePasswordForm = (): boolean => {
    if (!passwordForm.newUsername.trim()) {
      toast.error("请输入新用户名");

      return false;
    }
    if (passwordForm.newUsername.length < 3) {
      toast.error("用户名长度至少3位");

      return false;
    }
    if (!passwordForm.currentPassword) {
      toast.error("请输入当前密码");

      return false;
    }
    if (!passwordForm.newPassword) {
      toast.error("请输入新密码");

      return false;
    }
    if (passwordForm.newPassword.length < 6) {
      toast.error("新密码长度不能少于6位");

      return false;
    }
    if (passwordForm.newPassword !== passwordForm.confirmPassword) {
      toast.error("两次输入密码不一致");

      return false;
    }

    return true;
  };

  // 提交密码修改
  const handlePasswordSubmit = async () => {
    if (!validatePasswordForm()) return;

    setPasswordLoading(true);
    try {
      const response = await updatePassword(passwordForm);

      if (response.code === 0) {
        toast.success("密码修改成功，请重新登录");
        onOpenChange();
        handleLogout();
      } else {
        toast.error(response.msg || "密码修改失败");
      }
    } catch {
      toast.error("修改密码时发生错误");
    } finally {
      setPasswordLoading(false);
    }
  };

  // 重置密码表单
  const resetPasswordForm = () => {
    setPasswordForm({
      newUsername: "",
      currentPassword: "",
      newPassword: "",
      confirmPassword: "",
    });
  };

  // 过滤菜单项（根据权限）
  const filteredMenuItems = menuItems.filter(
    (item) => !item.adminOnly || isAdmin,
  );

  return (
    <div
      className={`flex ${isMobile ? "min-h-screen p-0" : "h-screen p-6 gap-6"} bg-mesh-gradient overflow-hidden`}
    >
      {/* 移动端遮罩层 */}
      {isMobile && mobileMenuVisible && (
        <button
          aria-label="关闭菜单"
          className="fixed inset-0 bg-black/30 backdrop-blur-sm z-40"
          type="button"
          onClick={hideMobileMenu}
        />
      )}

      {/* 左侧菜单栏 */}
      <aside
        className={`
        ${isMobile ? "fixed h-screen top-0 left-0 rounded-r-3xl" : "relative h-full rounded-3xl"}
        ${isMobile && !mobileMenuVisible ? "-translate-x-full" : "translate-x-0"}
        ${isMobile ? "w-64" : isCollapsed ? "w-20" : "w-[260px]"}
        bg-white/20 dark:bg-zinc-900/20 backdrop-blur-3xl
        shadow-[0_10px_30px_rgba(0,0,0,0.1)]
        border border-white/80 dark:border-white/10
        z-50
        transition-all duration-300 ease-in-out
        flex flex-col flex-shrink-0      `}
      >
        {/* Logo 区域 */}
        <div className="px-6 py-8 flex items-center overflow-hidden whitespace-nowrap box-border">
          <div className="flex-shrink-0 flex items-center justify-center w-10 h-10 rounded-xl bg-primary text-white">
            <BrandLogo size={24} />
          </div>
          <div
            className={`transition-all duration-300 overflow-hidden ${isCollapsed ? "max-w-0 opacity-0 ml-0" : "max-w-[180px] opacity-100 ml-3"}`}
          >
            <h1 className="text-xl font-bold text-foreground overflow-hidden whitespace-nowrap text-ellipsis">
              {siteConfig.name}
            </h1>
          </div>
        </div>

        {/* 菜单导航 */}
        <nav className="flex-1 px-4 overflow-y-auto overflow-x-hidden [scrollbar-width:none]">
          <ul className="space-y-2">
            {filteredMenuItems.map((item) => {
              const isActive = location.pathname === item.path;
              const isMonitor = item.path === "/monitor";
              const isMonitorBlocked = isMonitor && monitorAllowed !== true;

              return (
                <li key={item.path}>
                  <motion.button
                    className={`
                       w-full flex items-center p-3 rounded-2xl text-left
                       relative min-h-[48px] overflow-hidden transition-colors
                       ${isMonitorBlocked ? "opacity-60" : ""}
                       ${
                         isActive
                           ? "text-primary dark:text-primary-400 font-semibold"
                            : isMonitorBlocked
                              ? "text-gray-500 dark:text-gray-400 font-medium"
                              : "text-gray-600 dark:text-gray-300 font-medium"
                       }
                     `}
                    aria-disabled={isMonitorBlocked}
                    title={
                      isCollapsed
                        ? isMonitorBlocked
                          ? `${item.label} (无权限)`
                          : item.label
                        : undefined
                    }
                    transition={{ duration: 0.15 }}
                    onClick={() => handleMenuClick(item.path)}
                  >
                    {isActive && (
                      <motion.div
                        className="absolute inset-0 rounded-2xl bg-white/60 dark:bg-white/10 backdrop-blur-xl shadow-[0_12px_36px_rgba(0,0,0,0.18)] border border-white dark:border-white/20"
                        layoutId="sidebar-active"
                        transition={{
                          type: "spring",
                          stiffness: 380,
                          damping: 30,
                        }}
                      />
                    )}
                    {!isActive && (
                      <motion.div
                        className="absolute inset-0 rounded-2xl bg-white/40 dark:bg-white/5 opacity-0"
                        transition={{ duration: 0.15 }}
                        whileHover={{ opacity: 1 }}
                      />
                    )}
                    <div className="flex-shrink-0 w-6 h-6 flex items-center justify-center relative z-10">
                      {item.icon}
                    </div>
                    <div
                      className={`transition-all duration-300 overflow-hidden flex items-center ${isCollapsed ? "max-w-0 opacity-0 ml-0" : "max-w-[200px] opacity-100 ml-3"}`}
                    >
                      <span className="relative z-10 whitespace-nowrap">
                        {item.label}
                      </span>
                    </div>
                  </motion.button>
                </li>
              );
            })}
          </ul>
        </nav>

        {/* 底部版权信息和折叠按钮 */}
        <div className="px-5 py-6 mt-auto flex-shrink-0 flex flex-col gap-4 overflow-hidden whitespace-nowrap box-border">
          <div
            className={`transition-all duration-300 overflow-hidden flex items-center ${isCollapsed ? "max-w-0 opacity-0" : "max-w-[200px] opacity-100"}`}
          >
            <VersionFooter
              poweredClassName="text-xs text-gray-400 dark:text-gray-500"
              updateBadgeClassName="ml-2 inline-flex items-center rounded-full bg-rose-500/90 px-2 py-0.5 text-[10px] font-semibold tracking-wide text-white"
              version={siteConfig.version}
              versionClassName="text-xs text-gray-400 dark:text-gray-500"
            />
          </div>

          {/* 桌面端折叠按钮 */}
          {!isMobile && (
            <Button
              isIconOnly
              className="flex-shrink-0 text-gray-400 hover:text-gray-700 dark:text-gray-500 dark:hover:text-gray-300 min-w-0 w-10 h-10 rounded-full mx-auto"
              size="sm"
              variant="light"
              onPress={toggleCollapse}
            >
              {isCollapsed ? (
                <svg
                  className="w-5 h-5"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    d="M13 5l7 7-7 7M5 5l7 7-7 7"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                  />
                </svg>
              ) : (
                <svg
                  className="w-5 h-5"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    d="M11 19l-7-7 7-7m8 14l-7-7 7-7"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                  />
                </svg>
              )}
            </Button>
          )}
        </div>
      </aside>

      {/* 主内容区域 */}
      <div
        className={`flex flex-col flex-1 ${isMobile ? "min-h-0" : "h-full overflow-hidden"} relative`}
      >
        {/* 移动端菜单按钮 (替代Header) */}
        {isMobile && (
          <Button
            isIconOnly
            className="absolute top-4 left-4 z-40 bg-white/20 dark:bg-zinc-900/20 backdrop-blur-md shadow-sm border border-white/80 dark:border-white/10"
            variant="flat"
            onPress={toggleMobileMenu}
          >
            <svg
              className="w-5 h-5 text-foreground"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                d="M4 6h16M4 12h16M4 18h16"
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
              />
            </svg>
          </Button>
        )}

        {/* 主内容 */}
        <main className="flex-1 overflow-y-auto [scrollbar-width:none]">
          <AnimatePresence mode="wait">
            <motion.div
              key={location.pathname}
              animate={{ opacity: 1, y: 0 }}
              className="h-full"
              exit={{ opacity: 0, y: -6 }}
              initial={{ opacity: 0, y: 10 }}
              transition={{ duration: 0.22, ease: [0.25, 0.46, 0.45, 0.94] }}
            >
              {children}
            </motion.div>
          </AnimatePresence>
        </main>
      </div>

      {/* 修改密码弹窗 */}
      <Modal
        backdrop="blur"
        classNames={{
          base: "!w-[calc(100%-32px)] !mx-auto sm:!w-full rounded-2xl overflow-hidden",
        }}
        isOpen={isOpen}
        placement="center"
        scrollBehavior="inside"
        size="2xl"
        onOpenChange={() => {
          onOpenChange();
          resetPasswordForm();
        }}
      >
        <ModalContent>
          {(onClose: () => void) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                修改密码
              </ModalHeader>
              <ModalBody>
                <div className="space-y-4">
                  <Input
                    label="新用户名"
                    placeholder="请输入新用户名（至少3位）"
                    value={passwordForm.newUsername}
                    variant="bordered"
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                      setPasswordForm((prev) => ({
                        ...prev,
                        newUsername: e.target.value,
                      }))
                    }
                  />
                  <Input
                    label="当前密码"
                    placeholder="请输入当前密码"
                    type="password"
                    value={passwordForm.currentPassword}
                    variant="bordered"
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                      setPasswordForm((prev) => ({
                        ...prev,
                        currentPassword: e.target.value,
                      }))
                    }
                  />
                  <Input
                    label="新密码"
                    placeholder="请输入新密码（至少6位）"
                    type="password"
                    value={passwordForm.newPassword}
                    variant="bordered"
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                      setPasswordForm((prev) => ({
                        ...prev,
                        newPassword: e.target.value,
                      }))
                    }
                  />
                  <Input
                    label="确认密码"
                    placeholder="请再次输入新密码"
                    type="password"
                    value={passwordForm.confirmPassword}
                    variant="bordered"
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                      setPasswordForm((prev) => ({
                        ...prev,
                        confirmPassword: e.target.value,
                      }))
                    }
                  />
                </div>
              </ModalBody>
              <ModalFooter>
                <Button color="default" variant="light" onPress={onClose}>
                  取消
                </Button>
                <Button
                  color="primary"
                  isLoading={passwordLoading}
                  onPress={handlePasswordSubmit}
                >
                  确定
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </div>
  );
}
