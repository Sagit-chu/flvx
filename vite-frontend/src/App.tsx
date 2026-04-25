import {
  Route,
  Routes,
  useLocation,
  useNavigate,
  Navigate,
} from "react-router-dom";
import { useEffect, useState } from "react";
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
import AdminLayout from "@/layouts/admin";
import H5Layout from "@/layouts/h5";
import H5SimpleLayout from "@/layouts/h5-simple";
import { isLoggedIn } from "@/utils/auth";
import { siteConfig, updateSiteConfig } from "@/config/site";
import { useH5Mode } from "@/hooks/useH5Mode";
import { SESSION_UPDATED_EVENT } from "@/utils/session";

const ProtectedRoute = ({
  children,
  useSimpleLayout = false,
  skipLayout = false,
}: {
  children: React.ReactNode;
  useSimpleLayout?: boolean;
  skipLayout?: boolean;
}) => {
  const isH5 = useH5Mode();
  const navigate = useNavigate();
  const [authenticated, setAuthenticated] = useState(() => isLoggedIn());

  useEffect(() => {
    const handleSessionChange = () => {
      const loggedIn = isLoggedIn();

      setAuthenticated(loggedIn);

      if (!loggedIn) {
        navigate("/", { replace: true });
      }
    };

    window.addEventListener(SESSION_UPDATED_EVENT, handleSessionChange);

    return () => {
      window.removeEventListener(SESSION_UPDATED_EVENT, handleSessionChange);
    };
  }, [navigate]);

  useEffect(() => {
    if (!authenticated) {
      navigate("/", { replace: true });
    }
  }, [authenticated, navigate]);

  if (!authenticated) {
    return <Navigate replace to="/" />;
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
    return <Navigate replace to="/dashboard" />;
  }

  return <IndexPage />;
};

function App() {
  const location = useLocation();

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
      </Routes>
    </AnimatePresence>
  );
}

export default App;
