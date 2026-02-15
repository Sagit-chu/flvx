# VITE FRONTEND KNOWLEDGE BASE

**Generated:** Sun Feb 15 2026

## OVERVIEW
Web management console for FLVX.
**Stack:** React 18, Vite 5 (rolldown-vite), TypeScript, TailwindCSS 4, HeroUI.

## STRUCTURE
```
vite-frontend/
├── src/
│   ├── api/          # Axios wrapper + typed endpoint helpers
│   ├── components/   # Shared UI components (HeroUI based)
│   ├── config/       # Site config (title, repo, version)
│   ├── layouts/      # Admin vs H5 page chrome
│   ├── pages/        # Route views (many large single-file pages)
│   ├── utils/        # Auth/JWT + WebView helpers
│   ├── App.tsx       # Routes + ProtectedRoute + H5 layout selection
│   ├── main.tsx      # ReactDOM + Providers (HeroUI, Theme, Toast)
│   └── provider.tsx  # Context provider wrapper
├── vite.config.ts    # base '/', host 0.0.0.0:3000; build minify/treeshake disabled
├── eslint.config.mjs  # ESLint 9 flat config
└── package.json
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| **Route definitions** | `src/App.tsx` | React Router v6; H5 detection logic |
| **API Client** | `src/api/network.ts` | Sets `Authorization` header (raw token) |
| **Endpoint Calls** | `src/api/index.ts` | Thin `Network.post` wrappers |
| **Login Flow** | `src/pages/index.tsx` | Calls `login()`, stores `localStorage.token` |
| **Auth Logic** | `src/utils/auth.ts` | `isAdmin()` checks `role_id == 0` |
| **Token Decoding** | `src/utils/jwt.ts` | Checks `exp` vs current time |
| **WebView Logic** | `src/utils/panel.ts` | Handles panel address selection in app mode |

## CONVENTIONS
- **Auth**: JWT stored as `localStorage.token`. Sent in `Authorization` header (no "Bearer" prefix).
- **API**: Default base URL is `/api/v1/`. Responses follow `{code, msg, data, ts}` structure.
- **WebView**: In WebView mode, base URL is derived from selected panel address. If unset, API returns `code: -1`.
- **Routing**: URL query param `h5=true` forces mobile layout.
- **Build**: `minify: false`, `treeshake: false` - unoptimized production bundles for debugging.
- **ESLint**: `react-hooks/exhaustive-deps` disabled, unused vars starting with `_` ignored.
- **Large Pages**: `forward.tsx` (3263 LOC), `tunnel.tsx` (2552 LOC), `node.tsx` (2194 LOC).

## ANTI-PATTERNS
- **DO NOT ADD** tests - no test infrastructure (Vitest/Jest not configured).

## NOTES
- Uses `rolldown-vite` (experimental Rust bundler) instead of standard Vite.
- ESLint Flat Config format with custom import ordering rules.

## COMMANDS
```bash
cd vite-frontend
npm run dev
npm run build
npm run lint
```
