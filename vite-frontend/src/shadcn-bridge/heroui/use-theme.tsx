import * as React from "react";

type ThemeMode = "light" | "dark";

const STORAGE_KEY = "flvx:theme";

function resolveInitialTheme(): ThemeMode {
  if (typeof window === "undefined") {
    return "light";
  }

  const fromStorage = window.localStorage.getItem(STORAGE_KEY);
  if (fromStorage === "dark" || fromStorage === "light") {
    return fromStorage;
  }

  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

let currentTheme: ThemeMode = resolveInitialTheme();
const listeners = new Set<(theme: ThemeMode) => void>();

function broadcast(theme: ThemeMode) {
  currentTheme = theme;
  if (typeof window !== "undefined") {
    window.localStorage.setItem(STORAGE_KEY, theme);
  }
  listeners.forEach((listener) => {
    listener(theme);
  });
}

export function useTheme() {
  const [theme, setThemeState] = React.useState<ThemeMode>(currentTheme);

  React.useEffect(() => {
    const listener = (nextTheme: ThemeMode) => {
      setThemeState(nextTheme);
    };

    listeners.add(listener);

    return () => {
      listeners.delete(listener);
    };
  }, []);

  const setTheme = React.useCallback((nextTheme: string) => {
    if (nextTheme !== "dark" && nextTheme !== "light") {
      return;
    }
    broadcast(nextTheme);
  }, []);

  return {
    setTheme,
    theme,
  };
}
