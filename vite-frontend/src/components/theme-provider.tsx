import React, { useEffect } from "react";

import { useTheme } from "@/shadcn-bridge/heroui/use-theme";

interface ThemeProviderProps {
  children: React.ReactNode;
}

export const ThemeProvider: React.FC<ThemeProviderProps> = ({ children }) => {
  const { theme, setTheme } = useTheme();

  useEffect(() => {
    // 确保主题与HTML class同步
    const updateThemeClass = (currentTheme: string) => {
      if (currentTheme === "dark") {
        document.documentElement.classList.add("dark");
        document.documentElement.style.colorScheme = "dark";
      } else {
        document.documentElement.classList.remove("dark");
        document.documentElement.style.colorScheme = "light";
      }
    };

    // 始终跟随系统主题
    const systemTheme = window.matchMedia("(prefers-color-scheme: dark)")
      .matches
      ? "dark"
      : "light";

    if (systemTheme !== theme) {
      setTheme(systemTheme);
    }

    // 监听主题变化
    updateThemeClass(theme);

    // 监听系统主题变化
    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
    const handleThemeChange = (e: MediaQueryListEvent) => {
      const newTheme = e.matches ? "dark" : "light";

      setTheme(newTheme);
    };

    mediaQuery.addEventListener("change", handleThemeChange);

    return () => mediaQuery.removeEventListener("change", handleThemeChange);
  }, [theme, setTheme]);

  return <>{children}</>;
};
