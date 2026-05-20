import {
  Route,
  Routes,
  useLocation,
  useNavigate,
  Navigate,
} from "react-router-dom";
import { useEffect } from "react";
import { AnimatePresence } from "framer-motion";

import IndexPage from "@/pages/index";
import ChangePasswordPage from "@/pages/change-password";
import DashboardPage from "@/pages/dashboard";
import MonitorPage from "@/pages/monitor";
import ForwardPage from "@/pages/forward";
import PlansPage from "@/pages/plans";
import TunnelPage from "@/pages/tunnel";
import NodePage from "@/pages/node";
import UserPage from "@/pages/user";
import GroupPage from "@/pages/group";
import RegisterPage from "@/pages/register";
import LegalPage from "@/pages/legal";
import ProfilePage from "@/pages/profile";
import LimitPage from "@/pages/limit";
import ConfigPage from "@/pages/config";
import PanelSharingPage from "@/pages/panel-sharing";
import CommerceAdminPage from "@/pages/commerce-admin";
import LicensePage from "@/pages/license";
import AdminLayout from "@/layouts/admin";
import H5Layout from "@/layouts/h5";
import { isAdmin as hasAdminRole, isLoggedIn } from "@/utils/auth";
import { siteConfig, updateSiteConfig } from "@/config/site";
import { useH5Mode } from "@/hooks/useH5Mode";
import { SESSION_UPDATED_EVENT } from "@/utils/session";

const ProtectedRoute = ({
  children,
  skipLayout = false,
}: {
  children: React.ReactNode;
  skipLayout?: boolean;
}) => {
  const isH5 = useH5Mode();
  const authenticated = isLoggedIn();

  if (!authenticated) {
    return <Navigate replace to="/" />;
  }

  // 如果跳过布局，直接返回子组件
  if (skipLayout) {
    return <>{children}</>;
  }

  // 根据模式和页面类型选择布局
  const Layout = isH5 ? H5Layout : AdminLayout;

  return <Layout>{children}</Layout>;
};

const AdminRoute = ({ children }: { children: React.ReactNode }) => {
  const authenticated = isLoggedIn();

  if (!authenticated) {
    return <Navigate replace to="/" />;
  }

  if (!hasAdminRole()) {
    return <Navigate replace to="/dashboard" />;
  }

  return <AdminLayout scope="admin">{children}</AdminLayout>;
};

const LegacyAdminRoute = ({ to }: { to: string }) => (
  <AdminRoute>
    <Navigate replace to={to} />
  </AdminRoute>
);

// 登录页面路由组件 - 已登录则重定向到dashboard
const LoginRoute = () => {
  const authenticated = isLoggedIn();
  const navigate = useNavigate();

  useEffect(() => {
    if (authenticated) {
      // 使用 React Router 导航，避免无限跳转
      navigate("/dashboard", { replace: true });
    }
  }, [authenticated, navigate]);

  if (authenticated) {
    return <Navigate replace to="/dashboard" />;
  }

  return <IndexPage />;
};

const publicPaths = new Set([
  "/",
  "/register",
  "/terms",
  "/privacy",
  "/refund-policy",
  "/acceptable-use",
]);

function App() {
  const location = useLocation();
  const navigate = useNavigate();

  // 全局登录状态监听，当检测到未登录且不在首页时，跳转到首页
  useEffect(() => {
    const handleSessionUpdate = () => {
      if (!isLoggedIn() && !publicPaths.has(location.pathname)) {
        navigate("/", { replace: true });
      }
    };

    window.addEventListener(SESSION_UPDATED_EVENT, handleSessionUpdate);

    return () => {
      window.removeEventListener(SESSION_UPDATED_EVENT, handleSessionUpdate);
    };
  }, [location.pathname, navigate]);

  // 处理自定义背景图片
  useEffect(() => {
    const updateBg = () => {
      const customBg = siteConfig.app_bg_image;

      if (customBg) {
        if (customBg === "theme") {
          document.documentElement.style.removeProperty("--custom-bg-image");
          document.documentElement.style.removeProperty("--custom-bg-color");
          document.documentElement.classList.add("has-theme-bg");
          document.documentElement.classList.remove("has-custom-bg");
        } else if (
          customBg.startsWith("http") ||
          customBg.startsWith("data:") ||
          customBg.startsWith("/") ||
          customBg.startsWith("blob:")
        ) {
          document.documentElement.style.setProperty(
            "--custom-bg-image",
            `url(${customBg})`,
          );
          document.documentElement.style.setProperty(
            "--custom-bg-color",
            "transparent",
          );
          document.documentElement.classList.add("has-custom-bg");
          document.documentElement.classList.remove("has-theme-bg");
        } else {
          // Assume solid color like "#ffffff", "white", etc.
          document.documentElement.style.setProperty(
            "--custom-bg-image",
            "none",
          );
          document.documentElement.style.setProperty(
            "--custom-bg-color",
            customBg,
          );
          document.documentElement.classList.add("has-custom-bg");
          document.documentElement.classList.remove("has-theme-bg");
        }
      } else {
        document.documentElement.style.removeProperty("--custom-bg-image");
        document.documentElement.style.removeProperty("--custom-bg-color");
        document.documentElement.classList.remove("has-custom-bg");
        document.documentElement.classList.remove("has-theme-bg");
      }
    };

    updateBg();
    window.addEventListener("site-config-updated", updateBg);

    return () => {
      window.removeEventListener("site-config-updated", updateBg);
    };
  }, []);

  // 立即设置页面标题（使用已从缓存读取的配置）
  useEffect(() => {
    document.title = siteConfig.name;

    void updateSiteConfig();

    const handleConfigUpdate = () => {
      void updateSiteConfig();
    };

    window.addEventListener("configUpdated", handleConfigUpdate);

    return () => {
      window.removeEventListener("configUpdated", handleConfigUpdate);
    };
  }, []);

  return (
    <AnimatePresence mode="wait">
      <Routes key={location.pathname} location={location}>
        <Route element={<LoginRoute />} path="/" />
        <Route element={<RegisterPage />} path="/register" />
        <Route element={<LegalPage type="terms" />} path="/terms" />
        <Route element={<LegalPage type="privacy" />} path="/privacy" />
        <Route
          element={<LegalPage type="refundPolicy" />}
          path="/refund-policy"
        />
        <Route
          element={<LegalPage type="acceptableUse" />}
          path="/acceptable-use"
        />
        <Route
          element={
            <ProtectedRoute skipLayout={true}>
              <ChangePasswordPage />
            </ProtectedRoute>
          }
          path="/change-password"
        />
        <Route
          element={
            <ProtectedRoute>
              <DashboardPage />
            </ProtectedRoute>
          }
          path="/dashboard"
        />
        <Route
          element={
            <ProtectedRoute>
              <MonitorPage />
            </ProtectedRoute>
          }
          path="/monitor"
        />
        <Route
          element={
            <ProtectedRoute>
              <ForwardPage />
            </ProtectedRoute>
          }
          path="/forward"
        />
        <Route
          element={
            <ProtectedRoute>
              <PlansPage />
            </ProtectedRoute>
          }
          path="/plans"
        />
        <Route
          element={
            <ProtectedRoute>
              <PlansPage section="subscription" />
            </ProtectedRoute>
          }
          path="/plans/subscription"
        />
        <Route
          element={
            <ProtectedRoute>
              <PlansPage section="store" />
            </ProtectedRoute>
          }
          path="/plans/store"
        />
        <Route
          element={
            <ProtectedRoute>
              <PlansPage section="coupon" />
            </ProtectedRoute>
          }
          path="/plans/coupon"
        />
        <Route
          element={
            <ProtectedRoute>
              <PlansPage section="orders" />
            </ProtectedRoute>
          }
          path="/plans/orders"
        />
        <Route
          element={
            <ProtectedRoute>
              <PlansPage section="wallet" />
            </ProtectedRoute>
          }
          path="/plans/wallet"
        />
        <Route
          element={
            <ProtectedRoute>
              <PlansPage section="notifications" />
            </ProtectedRoute>
          }
          path="/plans/notifications"
        />
        <Route
          element={
            <ProtectedRoute>
              <PlansPage section="tickets" />
            </ProtectedRoute>
          }
          path="/plans/tickets"
        />
        <Route
          element={<LegacyAdminRoute to="/admin/tunnels" />}
          path="/tunnel"
        />
        <Route element={<LegacyAdminRoute to="/admin/nodes" />} path="/node" />
        <Route element={<LegacyAdminRoute to="/admin/users" />} path="/user" />
        <Route
          element={<LegacyAdminRoute to="/admin/groups" />}
          path="/group"
        />
        <Route
          element={
            <ProtectedRoute>
              <ProfilePage />
            </ProtectedRoute>
          }
          path="/profile"
        />
        <Route
          element={<LegacyAdminRoute to="/admin/limits" />}
          path="/limit"
        />
        <Route
          element={<LegacyAdminRoute to="/admin/config" />}
          path="/config"
        />
        <Route
          element={<LegacyAdminRoute to="/admin/panel-sharing" />}
          path="/panel-sharing"
        />
        <Route
          element={<LegacyAdminRoute to="/admin/orders" />}
          path="/commerce"
        />
        <Route
          element={
            <AdminRoute>
              <Navigate replace to="/admin/tunnels" />
            </AdminRoute>
          }
          path="/admin"
        />
        <Route
          element={
            <AdminRoute>
              <TunnelPage />
            </AdminRoute>
          }
          path="/admin/tunnels"
        />
        <Route
          element={
            <AdminRoute>
              <NodePage />
            </AdminRoute>
          }
          path="/admin/nodes"
        />
        <Route
          element={
            <AdminRoute>
              <UserPage />
            </AdminRoute>
          }
          path="/admin/users"
        />
        <Route
          element={
            <AdminRoute>
              <GroupPage />
            </AdminRoute>
          }
          path="/admin/groups"
        />
        <Route
          element={
            <AdminRoute>
              <LimitPage />
            </AdminRoute>
          }
          path="/admin/limits"
        />
        <Route
          element={
            <AdminRoute>
              <ConfigPage />
            </AdminRoute>
          }
          path="/admin/config"
        />
        <Route
          element={
            <AdminRoute>
              <PanelSharingPage />
            </AdminRoute>
          }
          path="/admin/panel-sharing"
        />
        <Route
          element={<LegacyAdminRoute to="/admin/orders" />}
          path="/admin/commerce"
        />
        <Route
          element={
            <AdminRoute>
              <CommerceAdminPage section="plans" />
            </AdminRoute>
          }
          path="/admin/plans"
        />
        <Route
          element={
            <AdminRoute>
              <CommerceAdminPage section="orders" />
            </AdminRoute>
          }
          path="/admin/orders"
        />
        <Route
          element={
            <AdminRoute>
              <CommerceAdminPage section="payments" />
            </AdminRoute>
          }
          path="/admin/payments"
        />
        <Route
          element={
            <AdminRoute>
              <CommerceAdminPage section="refunds" />
            </AdminRoute>
          }
          path="/admin/refunds"
        />
        <Route
          element={
            <AdminRoute>
              <CommerceAdminPage section="wallet" />
            </AdminRoute>
          }
          path="/admin/wallet"
        />
        <Route
          element={
            <AdminRoute>
              <CommerceAdminPage section="coupons" />
            </AdminRoute>
          }
          path="/admin/coupons"
        />
        <Route
          element={
            <AdminRoute>
              <CommerceAdminPage section="tickets" />
            </AdminRoute>
          }
          path="/admin/tickets"
        />
        <Route
          element={
            <AdminRoute>
              <CommerceAdminPage section="reports" />
            </AdminRoute>
          }
          path="/admin/reports"
        />
        <Route
          element={
            <AdminRoute>
              <CommerceAdminPage section="risk" />
            </AdminRoute>
          }
          path="/admin/risk"
        />
        <Route
          element={
            <AdminRoute>
              <CommerceAdminPage section="resource-jobs" />
            </AdminRoute>
          }
          path="/admin/resource-jobs"
        />
        <Route
          element={
            <AdminRoute>
              <CommerceAdminPage section="audit" />
            </AdminRoute>
          }
          path="/admin/audit"
        />
        <Route
          element={
            <AdminRoute>
              <CommerceAdminPage section="invites" />
            </AdminRoute>
          }
          path="/admin/invites"
        />
        <Route
          element={
            <AdminRoute>
              <CommerceAdminPage section="register-settings" />
            </AdminRoute>
          }
          path="/admin/register-settings"
        />
        <Route
          element={
            <AdminRoute>
              <CommerceAdminPage section="payment-settings" />
            </AdminRoute>
          }
          path="/admin/payment-settings"
        />
        <Route
          element={
            <AdminRoute>
              <CommerceAdminPage section="legal-settings" />
            </AdminRoute>
          }
          path="/admin/legal-settings"
        />
        <Route
          element={
            <AdminRoute>
              <LicensePage />
            </AdminRoute>
          }
          path="/admin/license"
        />
      </Routes>
    </AnimatePresence>
  );
}

export default App;
