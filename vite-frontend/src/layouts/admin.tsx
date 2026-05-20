import React, { useState, useEffect } from "react";
import { useNavigate, useLocation } from "react-router-dom";
import { toast } from "react-hot-toast";
import { AnimatePresence, motion } from "framer-motion";
import {
  ArrowLeftToLine,
  BarChart3,
  Bell,
  BookOpenCheck,
  ChevronDown,
  CreditCard,
  FileClock,
  Gauge,
  Gift,
  History,
  KeyRound,
  LayoutDashboard,
  LifeBuoy,
  Link2,
  Network,
  ReceiptText,
  Server,
  Settings,
  Share2,
  ShieldAlert,
  ShoppingBag,
  Ticket,
  UserPlus,
  Users,
  WalletCards,
  Zap,
} from "lucide-react";

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
import { isAdmin as hasAdminRole } from "@/utils/auth";

interface MenuItem {
  path: string;
  label: string;
  icon: React.ReactNode;
  scope: LayoutScope;
}

interface MenuGroup {
  key: string;
  label: string;
  items: MenuItem[];
}

type LayoutScope = "user" | "admin";

interface PasswordForm {
  newUsername: string;
  currentPassword: string;
  newPassword: string;
  confirmPassword: string;
}

export default function AdminLayout({
  children,
  scope = "user",
}: {
  children: React.ReactNode;
  scope?: LayoutScope;
}) {
  const navigate = useNavigate();
  const location = useLocation();
  const { isOpen, onOpenChange } = useDisclosure();
  const [mobileMenuVisible, setMobileMenuVisible] = useState(false);
  const [isCollapsed, setIsCollapsed] = useState(
    () => localStorage.getItem("sidebar_collapsed") === "true",
  );
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(() => {
    try {
      const stored = localStorage.getItem("sidebar_collapsed_groups");

      return new Set(stored ? (JSON.parse(stored) as string[]) : []);
    } catch {
      return new Set();
    }
  });
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

  const iconClass = "h-5 w-5";
  const menuGroups: MenuGroup[] = [
    {
      key: "user-panel",
      label: "用户面板",
      items: [
        {
          path: "/dashboard",
          label: "仪表盘",
          scope: "user",
          icon: <LayoutDashboard className={iconClass} />,
        },
        {
          path: "/forward",
          label: "规则",
          scope: "user",
          icon: <Gauge className={iconClass} />,
        },
        {
          path: "/plans/subscription",
          label: "我的套餐",
          scope: "user",
          icon: <ReceiptText className={iconClass} />,
        },
        {
          path: "/plans/store",
          label: "套餐商城",
          scope: "user",
          icon: <ShoppingBag className={iconClass} />,
        },
        {
          path: "/plans/coupon",
          label: "优惠码",
          scope: "user",
          icon: <Gift className={iconClass} />,
        },
        {
          path: "/plans/orders",
          label: "我的订单",
          scope: "user",
          icon: <ReceiptText className={iconClass} />,
        },
        {
          path: "/plans/wallet",
          label: "账户余额",
          scope: "user",
          icon: <WalletCards className={iconClass} />,
        },
        {
          path: "/plans/notifications",
          label: "站内通知",
          scope: "user",
          icon: <Bell className={iconClass} />,
        },
        {
          path: "/plans/tickets",
          label: "售后工单",
          scope: "user",
          icon: <LifeBuoy className={iconClass} />,
        },
        {
          path: "/monitor",
          label: "监控",
          scope: "user",
          icon: <BarChart3 className={iconClass} />,
        },
      ],
    },
    {
      key: "overview",
      label: "运营总览",
      items: [
        {
          path: "/admin/reports",
          label: "运营报表",
          scope: "admin",
          icon: <FileClock className={iconClass} />,
        },
        {
          path: "/admin/risk",
          label: "风控列表",
          scope: "admin",
          icon: <ShieldAlert className={iconClass} />,
        },
        {
          path: "/admin/resource-jobs",
          label: "资源任务",
          scope: "admin",
          icon: <BookOpenCheck className={iconClass} />,
        },
        {
          path: "/admin/audit",
          label: "审计日志",
          scope: "admin",
          icon: <History className={iconClass} />,
        },
      ],
    },
    {
      key: "resources",
      label: "资源与节点",
      items: [
        {
          path: "/admin/tunnels",
          label: "隧道管理",
          scope: "admin",
          icon: <Link2 className={iconClass} />,
        },
        {
          path: "/admin/nodes",
          label: "节点管理",
          scope: "admin",
          icon: <Server className={iconClass} />,
        },
        {
          path: "/admin/limits",
          label: "限速规则",
          scope: "admin",
          icon: <Zap className={iconClass} />,
        },
        {
          path: "/admin/groups",
          label: "分组管理",
          scope: "admin",
          icon: <Network className={iconClass} />,
        },
        {
          path: "/admin/panel-sharing",
          label: "面板共享",
          scope: "admin",
          icon: <Share2 className={iconClass} />,
        },
      ],
    },
    {
      key: "users",
      label: "用户与注册",
      items: [
        {
          path: "/admin/users",
          label: "用户管理",
          scope: "admin",
          icon: <Users className={iconClass} />,
        },
        {
          path: "/admin/register-settings",
          label: "注册设置",
          scope: "admin",
          icon: <UserPlus className={iconClass} />,
        },
        {
          path: "/admin/invites",
          label: "邀请码",
          scope: "admin",
          icon: <Ticket className={iconClass} />,
        },
      ],
    },
    {
      key: "transactions",
      label: "套餐与订单",
      items: [
        {
          path: "/admin/plans",
          label: "套餐管理",
          scope: "admin",
          icon: <ReceiptText className={iconClass} />,
        },
        {
          path: "/admin/orders",
          label: "订单管理",
          scope: "admin",
          icon: <ReceiptText className={iconClass} />,
        },
        {
          path: "/admin/payments",
          label: "支付流水",
          scope: "admin",
          icon: <CreditCard className={iconClass} />,
        },
        {
          path: "/admin/refunds",
          label: "退款审核",
          scope: "admin",
          icon: <FileClock className={iconClass} />,
        },
        {
          path: "/admin/wallet",
          label: "余额管理",
          scope: "admin",
          icon: <WalletCards className={iconClass} />,
        },
        {
          path: "/admin/coupons",
          label: "优惠码",
          scope: "admin",
          icon: <Gift className={iconClass} />,
        },
      ],
    },
    {
      key: "support",
      label: "客服",
      items: [
        {
          path: "/admin/tickets",
          label: "工单管理",
          scope: "admin",
          icon: <LifeBuoy className={iconClass} />,
        },
      ],
    },
    {
      key: "settings",
      label: "系统设置",
      items: [
        {
          path: "/admin/config",
          label: "网站配置",
          scope: "admin",
          icon: <Settings className={iconClass} />,
        },
        {
          path: "/admin/payment-settings",
          label: "支付配置",
          scope: "admin",
          icon: <CreditCard className={iconClass} />,
        },
        {
          path: "/admin/legal-settings",
          label: "合规条款",
          scope: "admin",
          icon: <BookOpenCheck className={iconClass} />,
        },
        {
          path: "/admin/license",
          label: "商业授权",
          scope: "admin",
          icon: <KeyRound className={iconClass} />,
        },
      ],
    },
  ];

  useEffect(() => {
    // 获取用户信息
    const adminFlag = hasAdminRole();

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
            res.data.allowed ? null : res.data.reason || null,
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

  const toggleGroup = (key: string) => {
    setCollapsedGroups((prev) => {
      const next = new Set(prev);

      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      localStorage.setItem(
        "sidebar_collapsed_groups",
        JSON.stringify(Array.from(next)),
      );

      return next;
    });
  };

  // 菜单点击处理
  const handleMenuClick = (path: string) => {
    if (path === "/monitor" && monitorAllowed !== true) {
      if (monitorAllowed == null) {
        toast("正在检查监控权限，请稍后重试");

        return;
      }

      const hint =
        monitorAccessReason === "need_active_plan"
          ? "当前没有可查看监控的有效套餐"
          : "暂无可查看监控的有效套餐";

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

  const isMenuItemActive = (path: string) =>
    location.pathname === path || location.pathname.startsWith(`${path}/`);

  // 过滤菜单项（根据权限）
  const filteredMenuGroups = menuGroups
    .map((group) => ({
      ...group,
      items: group.items.filter((item) => item.scope === scope),
    }))
    .filter((group) => group.items.length > 0);

  return (
    <div
      className={`flex ${isMobile ? "min-h-screen p-0" : "h-screen gap-4 p-4"} bg-mesh-gradient overflow-hidden`}
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
        ${isMobile ? "fixed h-screen top-0 left-0 rounded-r-[18px]" : "relative h-full rounded-[18px]"}
        ${isMobile && !mobileMenuVisible ? "-translate-x-full" : "translate-x-0"}
        ${isMobile ? "w-64" : isCollapsed ? "w-20" : "w-[264px]"}
        native-panel
        z-50
        transition-all duration-300 ease-in-out
        flex flex-col flex-shrink-0      `}
      >
        {/* Logo 区域 */}
        <div className="box-border flex items-center overflow-hidden whitespace-nowrap px-5 py-5">
          <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-[var(--radius-control)] bg-primary text-white">
            <BrandLogo size={24} />
          </div>
          <div
            className={`transition-all duration-300 overflow-hidden ${isCollapsed ? "max-w-0 opacity-0 ml-0" : "max-w-[180px] opacity-100 ml-3"}`}
          >
            <h1 className="overflow-hidden text-ellipsis whitespace-nowrap text-lg font-bold text-foreground">
              {siteConfig.name}
            </h1>
          </div>
        </div>

        {/* 菜单导航 */}
        <nav className="flex-1 overflow-y-auto overflow-x-hidden px-3 scrollbar-hide">
          <ul className="space-y-2.5">
            {filteredMenuGroups.map((group) => {
              const groupCollapsed = collapsedGroups.has(group.key);

              return (
                <li key={group.key}>
                  {!isCollapsed && (
                    <button
                      className="mb-1 flex w-full items-center justify-between rounded-[var(--radius-control)] px-3 py-2 text-xs font-semibold text-default-500 transition-colors hover:bg-default-100/70 dark:hover:bg-default-100/30"
                      type="button"
                      onClick={() => toggleGroup(group.key)}
                    >
                      <span>{group.label}</span>
                      <ChevronDown
                        className={`h-4 w-4 transition-transform ${
                          groupCollapsed ? "-rotate-90" : ""
                        }`}
                      />
                    </button>
                  )}
                  {(!groupCollapsed || isCollapsed) && (
                    <ul className="space-y-1">
                      {group.items.map((item) => {
                        const isActive = isMenuItemActive(item.path);
                        const isMonitor = item.path === "/monitor";
                        const isMonitorBlocked =
                          isMonitor && monitorAllowed !== true;

                        return (
                          <li key={`${group.key}-${item.path}`}>
                            <motion.button
                              aria-disabled={isMonitorBlocked}
                              className={`
                                w-full flex items-center px-3 py-2.5 rounded-[var(--radius-control)] text-left
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
                                  className="absolute inset-0 rounded-[var(--radius-control)] border border-primary/15 bg-primary/10 shadow-sm dark:bg-primary/15"
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
                                  className="absolute inset-0 rounded-[var(--radius-control)] bg-default-100/70 opacity-0 dark:bg-default-100/25"
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
                  )}
                </li>
              );
            })}
          </ul>
        </nav>

        {/* 底部版权信息和折叠按钮 */}
        <div className="mt-auto box-border flex flex-shrink-0 flex-col gap-3 overflow-hidden whitespace-nowrap px-4 py-4">
          {scope === "admin" && (
            <Button
              className={`min-h-10 rounded-[var(--radius-control)] bg-surface text-default-700 shadow-sm hover:bg-default-100 dark:text-default-300 ${
                isCollapsed
                  ? "mx-auto w-10 min-w-10 justify-center px-0"
                  : "w-full justify-start"
              }`}
              title="返回前台"
              variant="flat"
              onPress={() => navigate("/dashboard")}
            >
              <ArrowLeftToLine className="h-4 w-4 flex-shrink-0" />
              {!isCollapsed && <span>返回前台</span>}
            </Button>
          )}
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
            className="native-panel absolute left-4 top-4 z-40"
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
        <main className="flex-1 overflow-y-auto scrollbar-hide">
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
          base: "!w-[calc(100%-32px)] !mx-auto sm:!w-full rounded-[var(--radius-panel)] overflow-hidden",
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
