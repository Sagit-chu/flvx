/**
 * ThemeSettings — theme picker card for the Settings page
 * ========================================================
 * Shows:
 *  • Mode toggle (light / dark / system)
 *  • Grid of registered themes with preview dots
 *  • "Reset to default" option
 */

import React from "react";
import toast from "react-hot-toast";

import { Card, CardBody } from "@/shadcn-bridge/heroui/card";
import { Button } from "@/shadcn-bridge/heroui/button";
import { useThemeContext } from "@/themes/context";
import type { ThemeMode } from "@/themes/registry";

// ─── Constants ──────────────────────────────────────────────────────────────

const MODE_OPTIONS: Array<{ value: ThemeMode; label: string; icon: string }> = [
  { value: "light", label: "亮色", icon: "☀️" },
  { value: "dark", label: "暗色", icon: "🌙" },
  { value: "system", label: "跟随系统", icon: "🖥️" },
];

// ─── Component ──────────────────────────────────────────────────────────────

export const ThemeSettings: React.FC = () => {
  const {
    themes,
    activeThemeId,
    mode,
    effectiveMode,
    switchTheme,
    resetTheme,
    setMode,
  } = useThemeContext();

  const handleModeChange = (m: ThemeMode) => {
    setMode(m);
    const label = m === "light" ? "亮色" : m === "dark" ? "暗色" : "跟随系统";
    toast.success(`已切换为${label}模式`);
  };

  const handleThemeSelect = (id: string) => {
    switchTheme(id);
    const theme = themes.find((t) => t.id === id);
    toast.success(`已切换主题「${theme?.name ?? id}」`);
  };

  const handleReset = () => {
    resetTheme();
    toast.success("已恢复默认主题");
  };

  return (
    <Card className="shadow-md">
      <CardBody className="p-6">
        <h2 className="text-lg font-medium text-gray-900 dark:text-white mb-5">
          主题设置
        </h2>

        {/* ── Mode toggle ────────────────────────────────────── */}
        <div className="mb-6">
          <p className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
            外观模式
          </p>
          <div className="inline-flex rounded-lg border border-gray-200 dark:border-gray-600 p-1 gap-1">
            {MODE_OPTIONS.map((opt) => (
              <button
                key={opt.value}
                className={`px-4 py-2 rounded-md text-sm font-medium transition-all duration-200 ${
                  mode === opt.value
                    ? "bg-primary text-white shadow-sm"
                    : "text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700/50"
                }`}
                type="button"
                onClick={() => handleModeChange(opt.value)}
              >
                <span className="mr-1.5">{opt.icon}</span>
                {opt.label}
              </button>
            ))}
          </div>
        </div>

        {/* ── Theme grid ─────────────────────────────────────── */}
        <div className="mb-4">
          <p className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
            选择主题
            <span className="ml-2 text-xs text-gray-400 dark:text-gray-500 font-normal">
              共 {themes.length} 个可用主题
            </span>
          </p>

          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
            {themes.map((theme) => {
              const isActive = activeThemeId === theme.id;
              // Pick the right token set for preview
              const previewTokens =
                effectiveMode === "dark" && theme.tokens?.dark
                  ? theme.tokens.dark
                  : theme.tokens?.light;

              const primary = previewTokens?.["--primary"] ?? "#2563eb";
              const secondary = previewTokens?.["--secondary"] ?? "#6366f1";
              const success = previewTokens?.["--success"] ?? "#16a34a";
              const danger = previewTokens?.["--danger"] ?? "#dc2626";
              const bg = previewTokens?.["--background"] ?? "#ffffff";

              return (
                <button
                  key={theme.id}
                  className={`relative rounded-xl text-left transition-all duration-200 border-2 overflow-hidden ${
                    isActive
                      ? "border-primary shadow-lg shadow-primary/15 scale-[1.01]"
                      : "border-gray-200 dark:border-gray-600 hover:border-gray-300 dark:hover:border-gray-500 hover:shadow-md"
                  }`}
                  type="button"
                  onClick={() => handleThemeSelect(theme.id)}
                >
                  {/* Colour strip preview */}
                  <div className="flex h-8">
                    <div className="flex-1" style={{ background: primary }} />
                    <div className="flex-1" style={{ background: secondary }} />
                    <div className="flex-1" style={{ background: success }} />
                    <div className="flex-1" style={{ background: danger }} />
                    <div className="flex-1" style={{ background: bg, borderLeft: "1px solid rgba(0,0,0,0.06)" }} />
                  </div>

                  {/* Info */}
                  <div className="p-3">
                    <div className="flex items-center gap-2">
                      <p className="text-sm font-semibold text-gray-900 dark:text-white truncate">
                        {theme.name}
                      </p>
                      {isActive && (
                        <span className="shrink-0 px-1.5 py-0.5 rounded text-[10px] font-bold bg-primary/15 text-primary">
                          当前
                        </span>
                      )}
                    </div>
                    {theme.description && (
                      <p className="text-xs text-gray-500 dark:text-gray-400 mt-0.5 truncate">
                        {theme.description}
                      </p>
                    )}
                    <p className="text-[10px] text-gray-400 dark:text-gray-500 mt-1">
                      {theme.author} · v{theme.version}
                    </p>
                  </div>

                  {/* Active indicator dot */}
                  {isActive && (
                    <span className="absolute top-2 right-2 w-2.5 h-2.5 rounded-full bg-primary shadow-sm shadow-primary/50 animate-pulse" />
                  )}
                </button>
              );
            })}
          </div>
        </div>

        {/* ── Reset ──────────────────────────────────────────── */}
        {activeThemeId && activeThemeId !== "default" && (
          <div className="pt-2">
            <Button
              className="text-gray-500 dark:text-gray-400"
              size="sm"
              variant="light"
              onPress={handleReset}
            >
              ↩ 恢复默认主题
            </Button>
          </div>
        )}
      </CardBody>
    </Card>
  );
};
