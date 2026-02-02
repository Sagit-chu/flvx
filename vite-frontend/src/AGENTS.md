# VITE FRONTEND (src) KNOWLEDGE BASE

## OVERVIEW
React app entry + routing + providers. This is where UI architecture decisions live.

## STRUCTURE
```
vite-frontend/src/
├── main.tsx        # ReactDOM + BrowserRouter + Provider
├── provider.tsx    # HeroUI + theme + toaster + i18n wrapper
├── App.tsx         # Routes + ProtectedRoute + H5 layout selection
├── api/            # Axios wrapper + typed endpoint helpers
├── pages/          # Route views (large)
├── layouts/        # Admin/H5 page chrome
├── components/     # Shared UI components
├── utils/          # JWT parsing + auth helpers + WebView utilities
└── styles/         # globals.css
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Route definitions | `vite-frontend/src/App.tsx` | React Router v6 |
| API client baseURL | `vite-frontend/src/api/network.ts` | `/api/v1/` + token header |
| Token decoding | `vite-frontend/src/utils/jwt.ts` | Checks `exp` vs now |
| Role checks | `vite-frontend/src/utils/auth.ts` | `isAdmin()` is `role_id == 0` |
| WebView integration | `vite-frontend/src/api/network.ts` | Panel address selection in WebView mode |

## CONVENTIONS
- Token is stored in `localStorage.token` and sent as `Authorization` header (raw token string).
- H5 mode detection is in `vite-frontend/src/App.tsx` (screen/user-agent/query param `h5=true`).

## COMMANDS
```bash
cd vite-frontend
npm run dev
npm run lint
```
