# VITE FRONTEND KNOWLEDGE BASE

**Generated:** Mon Feb 02 2026

## OVERVIEW
Web management console for FLVX (formerly Flux Panel).
**Stack:** React 18, Vite 5, TypeScript, TailwindCSS 4, HeroUI.

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
- **API**: Default base URL is `/api/v1/`.
- **WebView**: In WebView mode, base URL is derived from selected panel address. If unset, API returns `code: -1`.
- **Routing**: URL query param `h5=true` forces mobile layout.

## COMMANDS
```bash
cd vite-frontend
npm run dev
npm run build
npm run lint
```
