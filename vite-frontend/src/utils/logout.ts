import { clearSession } from "@/utils/session";

/**
 * 安全退出登录函数
 * 清除登录相关数据，并强制刷新跳转到首页
 */
export const safeLogout = () => {
  clearSession();
  window.location.href = "/";
};
