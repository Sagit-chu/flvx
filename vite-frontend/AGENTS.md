# VITE FRONTEND KNOWLEDGE BASE

**Generated:** Mon Feb 02 2026

## OVERVIEW
Web management console for FLVX (formerly Flux Panel).
**Stack:** React 18, Vite 5, TypeScript, TailwindCSS 4, HeroUI.

## STRUCTURE
```
vite-frontend/
├── src/
│   ├── pages/        # Route views (some very large single-file pages)
│   ├── components/   # Reusable UI parts
│   ├── layouts/      # Admin vs H5 layouts
│   ├── api/          # API functions + axios wrapper
│   ├── config/       # Site config (title, repo, version)
│   └── utils/        # Auth/JWT + WebView helpers
├── vite.config.ts    # base '/', host 0.0.0.0:3000; build minify/treeshake disabled
├── eslint.config.mjs  # ESLint 9 flat config
└── package.json
```

## CONVENTIONS
- **Routing**: React Router v6 routes in `vite-frontend/src/App.tsx`.
- **Auth**: JWT stored as `localStorage.token`; sent as `Authorization` header (no prefix) in `vite-frontend/src/api/network.ts`.
- **Base URL**: Defaults to `/api/v1/` (or `VITE_API_BASE`); WebView mode selects a panel address via `vite-frontend/src/utils/panel.ts`.
- **UI**: HeroUI provider + theme + toast wired in `vite-frontend/src/provider.tsx`.

## COMMANDS
```bash
cd vite-frontend
npm run dev
npm run build
npm run lint
```
