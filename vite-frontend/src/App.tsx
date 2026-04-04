import { Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { useEffect } from "react";
import { AnimatePresence } from "framer-motion";

import IndexPage from "@/pages/index";
import ChangePasswordPage from "@/pages/change-password";
import DashboardPage from "@/pages/dashboard";
import MonitorPage from "@/pages/monitor";
import ForwardPage from "@/pages/forward";
import TunnelPage from "@/pages/tunnel";
import NodePage from "@/pages/node";
import UserPage from "@/pages/user";
import GroupPage from "@/pages/group";
import ProfilePage from "@/pages/profile";
import LimitPage from "@/pages/limit";
import ConfigPage from "@/pages/config";
import PanelSharingPage from "@/pages/panel-sharing";
import { SettingsPage } from "@/pages/settings";
import AdminLayout from "@/layouts/admin";
import H5Layout from "@/layouts/h5";
import H5SimpleLayout from "@/layouts/h5-simple";
import { isLoggedIn } from "@/utils/auth";
import { siteConfig, updateSiteConfig } from "@/config/site";
import { useH5Mode } from "@/hooks/useH5Mode";

// 简化的路由保护组件 - 使用 React Router 导航避免循环
const ProtectedRoute = ({
  children,
  useSimpleLayout = false,
  skipLayout = false,
}: {
  children: React.ReactNode;
  useSimpleLayout?: boolean;
  skipLayout?: boolean;
}) => {
  const authenticated = isLoggedIn();
  const isH5 = useH5Mode();
  const navigate = useNavigate();

  useEffect(() => {
    if (!authenticated) {
      // 使用 React Router 导航，避免无限跳转
      navigate("/", { replace: true });
    }
  }, [authenticated, navigate]);

  if (!authenticated) {
    return (
      <div className="flex items-center justify-center min-h-screen bg-mesh-gradient">
        <div className="text-lg text-gray-700 dark:text-gray-200" />
      </div>
    );
  }

  // 如果跳过布局，直接返回子组件
  if (skipLayout) {
    return <>{children}</>;
  }

  // 根据模式和页面类型选择布局
  const Layout =
    isH5 && useSimpleLayout ? H5SimpleLayout : isH5 ? H5Layout : AdminLayout;

  return <Layout>{children}</Layout>;
};

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
    return (
      <div className="flex items-center justify-center min-h-screen bg-mesh-gradient">
        <div className="text-lg text-gray-700 dark:text-gray-200" />
      </div>
    );
  }

  return <IndexPage />;
};

function App() {
  const location = useLocation();

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
              <TunnelPage />
            </ProtectedRoute>
          }
          path="/tunnel"
        />
        <Route
          element={
            <ProtectedRoute>
              <NodePage />
            </ProtectedRoute>
          }
          path="/node"
        />
        <Route
          element={
            <ProtectedRoute useSimpleLayout={true}>
              <UserPage />
            </ProtectedRoute>
          }
          path="/user"
        />
        <Route
          element={
            <ProtectedRoute useSimpleLayout={true}>
              <GroupPage />
            </ProtectedRoute>
          }
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
          element={
            <ProtectedRoute useSimpleLayout={true}>
              <LimitPage />
            </ProtectedRoute>
          }
          path="/limit"
        />
        <Route
          element={
            <ProtectedRoute>
              <ConfigPage />
            </ProtectedRoute>
          }
          path="/config"
        />
        <Route
          element={
            <ProtectedRoute useSimpleLayout={true}>
              <PanelSharingPage />
            </ProtectedRoute>
          }
          path="/panel-sharing"
        />
        <Route element={<SettingsPage />} path="/settings" />
      </Routes>
    </AnimatePresence>
  );
}

export default App;
