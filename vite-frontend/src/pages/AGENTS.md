# VITE FRONTEND (pages) KNOWLEDGE BASE

## OVERVIEW
Route views rendered by `vite-frontend/src/App.tsx`. Several pages are large, single-file screens.

## STRUCTURE
```
vite-frontend/src/pages/
├── index.tsx            # Login + captcha flow
├── dashboard.tsx
├── forward.tsx          # Large
├── tunnel.tsx           # Large
├── node.tsx             # Large
├── user.tsx             # Large
├── config.tsx
├── limit.tsx
├── profile.tsx
├── settings.tsx
└── change-password.tsx
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Login flow | `vite-frontend/src/pages/index.tsx` | Calls `login()` and stores `localStorage.token` |
| API calls | `vite-frontend/src/api/index.ts` | Thin wrappers around `Network.post` |
| Token expiration behavior | `vite-frontend/src/api/network.ts` | Clears localStorage + redirects on 401 |

## CONVENTIONS
- Pages call API wrappers from `vite-frontend/src/api/index.ts` (most endpoints are POST).
