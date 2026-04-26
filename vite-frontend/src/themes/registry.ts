/**
 * Theme Registry
 * ==============
 * Manages the set of installed themes and the currently active theme.
 * Handles CSS variable injection, `<style>` element management, and
 * lifecycle callbacks.
 *
 * This module is framework-agnostic (no React dependency).  The React
 * integration lives in `./context.tsx`.
 */

import type {
  ThemePackage,
  ThemeTokens,
  ComponentKey,
  LayoutKey,
  PageKey,
} from "./types";

// ─── internal state ──────────────────────────────────────────────────────────

const installed = new Map<string, ThemePackage>();
let activeId: string | null = null;
let injectedStyleEl: HTMLStyleElement | null = null;

const STORAGE_KEY = "flvx:active-theme";
const MODE_KEY = "flvx:theme"; // backwards-compat with old use-theme

type ChangeListener = () => void;
const changeListeners = new Set<ChangeListener>();

function notify() {
  changeListeners.forEach((fn) => fn());
}

// ─── public API ──────────────────────────────────────────────────────────────

/**
 * Register a theme package.  Call this for every theme you want available.
 * Registering a theme with an existing id replaces the previous one.
 */
export function registerTheme(pkg: ThemePackage): void {
  installed.set(pkg.id, pkg);
  notify();
}

/** Unregister a theme by id. */
export function unregisterTheme(id: string): void {
  if (activeId === id) deactivateTheme();
  installed.delete(id);
  notify();
}

/** Get all registered themes. */
export function getRegisteredThemes(): ThemePackage[] {
  return Array.from(installed.values());
}

/** Get a specific theme by id. */
export function getTheme(id: string): ThemePackage | undefined {
  return installed.get(id);
}

/** Get the id of the currently active theme (or null). */
export function getActiveThemeId(): string | null {
  return activeId;
}

/** Get the currently active ThemePackage (or null). */
export function getActiveTheme(): ThemePackage | null {
  return activeId ? (installed.get(activeId) ?? null) : null;
}

// ─── theme mode ──────────────────────────────────────────────────────────────

export type ThemeMode = "light" | "dark" | "system";

export function resolveSystemMode(): "light" | "dark" {
  if (typeof window === "undefined") return "light";

  return window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

export function getSavedMode(): ThemeMode {
  if (typeof window === "undefined") return "system";
  const raw = localStorage.getItem(MODE_KEY);

  if (raw === "light" || raw === "dark" || raw === "system") return raw;

  return "system";
}

export function saveMode(mode: ThemeMode): void {
  localStorage.setItem(MODE_KEY, mode);
  notify();
}

export function getEffectiveMode(): "light" | "dark" {
  const mode = getSavedMode();

  return mode === "system" ? resolveSystemMode() : mode;
}

// ─── activation ──────────────────────────────────────────────────────────────

/**
 * Activate a theme by id.  This:
 * 1. Calls `onDeactivate` on the previous theme.
 * 2. Injects CSS tokens onto `document.documentElement`.
 * 3. Injects the theme's `css` string into a `<style>` element.
 * 4. Calls `onActivate` on the new theme.
 * 5. Persists the choice to localStorage.
 */
export function activateTheme(id: string): void {
  const pkg = installed.get(id);

  if (!pkg) {
    return;
  }

  // Deactivate previous
  deactivateTheme();

  activeId = id;

  // Inject tokens
  const mode = getEffectiveMode();
  const tokens = mode === "dark" ? pkg.tokens?.dark : pkg.tokens?.light;

  if (tokens) injectTokens(tokens);

  // Inject custom CSS
  if (pkg.css) {
    injectedStyleEl = document.createElement("style");
    injectedStyleEl.setAttribute("data-flvx-theme", id);
    injectedStyleEl.textContent = pkg.css;
    document.head.appendChild(injectedStyleEl);
  }

  // Update dark class
  const root = document.documentElement;

  root.classList.toggle("dark", mode === "dark");
  root.style.colorScheme = mode;

  // Lifecycle
  pkg.onActivate?.();

  // Persist
  localStorage.setItem(STORAGE_KEY, id);

  notify();
}

/** Deactivate the current theme, reverting all overrides. */
export function deactivateTheme(): void {
  const prev = activeId ? installed.get(activeId) : null;

  prev?.onDeactivate?.();

  // Remove injected tokens
  clearInjectedTokens();

  // Remove injected style element
  if (injectedStyleEl) {
    injectedStyleEl.remove();
    injectedStyleEl = null;
  }

  activeId = null;
}

/** Re-apply the active theme (e.g. after mode changes). */
export function reapplyActiveTheme(): void {
  if (activeId) {
    const id = activeId;
    // quick re-inject without full lifecycle
    const pkg = installed.get(id);

    if (!pkg) return;

    clearInjectedTokens();
    const mode = getEffectiveMode();
    const tokens = mode === "dark" ? pkg.tokens?.dark : pkg.tokens?.light;

    if (tokens) injectTokens(tokens);

    const root = document.documentElement;

    root.classList.toggle("dark", mode === "dark");
    root.style.colorScheme = mode;
  } else {
    // No theme active — just set dark class based on mode
    const mode = getEffectiveMode();
    const root = document.documentElement;

    root.classList.toggle("dark", mode === "dark");
    root.style.colorScheme = mode;
  }
  notify();
}

// ─── component resolution ────────────────────────────────────────────────────

/**
 * Resolve a component: returns the themed override if present, otherwise
 * returns the fallback (default implementation).
 */
export function resolveComponent<P = any>(
  key: ComponentKey,
  fallback: React.ComponentType<P>,
): React.ComponentType<P> {
  const pkg = activeId ? installed.get(activeId) : null;
  const override = pkg?.components?.[key];

  return (override as React.ComponentType<P>) ?? fallback;
}

/** Resolve a layout override. */
export function resolveLayout(
  key: LayoutKey,
  fallback: React.ComponentType<{ children: React.ReactNode }>,
): React.ComponentType<{ children: React.ReactNode }> {
  const pkg = activeId ? installed.get(activeId) : null;

  return pkg?.layouts?.[key] ?? fallback;
}

/** Resolve a page override. */
export function resolvePage<P = any>(
  key: PageKey,
  fallback: React.ComponentType<P>,
): React.ComponentType<P> {
  const pkg = activeId ? installed.get(activeId) : null;
  const override = pkg?.pages?.[key];

  return (override as React.ComponentType<P>) ?? fallback;
}

// ─── subscriber API (for React) ──────────────────────────────────────────────

export function subscribe(listener: ChangeListener): () => void {
  changeListeners.add(listener);

  return () => changeListeners.delete(listener);
}

// ─── initialisation ──────────────────────────────────────────────────────────

/**
 * Call once at app boot.  Restores the previously active theme from
 * localStorage (if the theme is registered).
 */
export function initThemeSystem(): void {
  const savedId = localStorage.getItem(STORAGE_KEY);

  if (savedId && installed.has(savedId)) {
    activateTheme(savedId);
  } else {
    // Just apply mode
    reapplyActiveTheme();
  }
}

// ─── internal helpers ────────────────────────────────────────────────────────

const injectedVars: string[] = [];

function injectTokens(tokens: ThemeTokens): void {
  const root = document.documentElement;

  for (const [varName, value] of Object.entries(tokens)) {
    if (value !== undefined) {
      root.style.setProperty(varName, value);
      injectedVars.push(varName);
    }
  }
}

function clearInjectedTokens(): void {
  const root = document.documentElement;

  for (const varName of injectedVars) {
    root.style.removeProperty(varName);
  }
  injectedVars.length = 0;
}
