/**
 * Theme Loader — auto-registers all built-in themes
 * ==================================================
 * Import this module once at app startup (in provider.tsx or App.tsx).
 *
 * To add a new theme:
 * 1. Create a folder under `src/themes/` (e.g. `src/themes/my-theme/`)
 * 2. Export a `ThemePackage` as the default export from `index.ts`
 * 3. Import and register it below
 */

import { registerTheme } from "./registry";
// ── Built-in themes ──────────────────────────────────────────────────────────
import defaultTheme from "./default";
import cyberpunkTheme from "./example-cyberpunk";

// Register all themes
registerTheme(defaultTheme);
registerTheme(cyberpunkTheme);

/*
 * ── ADDING YOUR OWN THEME ──────────────────────────────────────────────────
 *
 * 1. Create your theme folder:
 *      src/themes/my-awesome-theme/
 *        ├── index.ts          ← exports ThemePackage
 *        ├── components/       ← optional component overrides
 *        └── ...
 *
 * 2. Import and register here:
 *      import myTheme from "./my-awesome-theme";
 *      registerTheme(myTheme);
 *
 * That's it! The theme will appear in the theme picker.
 */
