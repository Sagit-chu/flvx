# VITE FRONTEND KNOWLEDGE BASE

**Generated:** Thu Feb 19 2026
**Commit:** 137c34e
**Branch:** main
**Tag:** 2.1.4-rc2

## OVERVIEW
Web management console for FLVX.
**Stack:** React 18, rolldown-vite, TypeScript, Tailwind CSS v4, shadcn/radix primitives with HeroUI-compatible bridge.

## STRUCTURE
```
vite-frontend/
├── src/
│   ├── api/                      # Axios wrapper + typed endpoint helpers
│   ├── components/ui/            # shadcn/radix primitive components
│   ├── shadcn-bridge/heroui/     # HeroUI-compatible facade used by pages/layouts
│   ├── pages/                    # Route views + page modules (forward/node/tunnel split helpers)
│   ├── hooks/                    # H5/WebView/mobile hooks
│   ├── styles/
│   │   ├── globals.css           # Base styles + imports tailwind-theme.pcss
│   │   └── tailwind-theme.pcss   # Tailwind v4 @theme inline semantic token mapping
│   ├── App.tsx                   # Routes + ProtectedRoute + H5 layout selection
│   ├── main.tsx                  # ReactDOM + BrowserRouter + Provider
│   └── provider.tsx              # Toast/theme/provider composition
├── components.json               # shadcn/ui config
├── tailwind.config.js            # Compatibility config still used by migration scaffolding
├── vite.config.ts                # base '/', host 0.0.0.0:3000; build minify/treeshake disabled
└── package.json
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| **Route definitions** | `src/App.tsx` | React Router v6 + ProtectedRoute |
| **API Client/Auth header** | `src/api/network.ts` | Sends raw JWT in `Authorization` header |
| **Login Flow** | `src/pages/index.tsx` | Calls `login()`, stores `localStorage.token` |
| **Auth helpers** | `src/utils/auth.ts`, `src/utils/jwt.ts` | Role checks + token expiration parsing |
| **UI bridge usage** | `src/shadcn-bridge/heroui/` | Import from bridge, not `@heroui/*` |
| **Button parity mapping** | `src/shadcn-bridge/heroui/button.tsx` | Legacy `color`/`variant` mapped to shadcn classes |
| **Semantic theme tokens** | `src/styles/tailwind-theme.pcss` | Restores classes like `bg-primary`, `border-input` |
| **Theme wiring** | `src/styles/globals.css` | Must import `./tailwind-theme.pcss` |

## CONVENTIONS
- **Auth Header**: Use raw JWT token (no `Bearer` prefix).
- **API Envelope**: Responses follow `{code, msg, data, ts}`.
- **UI Imports**: Use `src/shadcn-bridge/heroui/*` in app pages/layouts for compatibility.
- **Semantic Colors**: Keep `globals.css -> tailwind-theme.pcss` import intact or semantic classes break.
- **Build profile**: `minify: false`, `treeshake: false` for easier debugging.
- **Layout mode**: H5/mobile mode still controlled by existing route/query and hook logic.

## ANTI-PATTERNS
- **DO NOT ADD** `Bearer` to auth header in frontend requests.
- **DO NOT REINTRODUCE** `@heroui/*` or `@nextui-org/*` dependencies.
- **DO NOT REMOVE** `src/styles/tailwind-theme.pcss` import from `src/styles/globals.css`.
- **DO NOT ADD** frontend tests; no Vitest/Jest setup exists.

## NOTES
- PR `#144` (shadcn migration) and PR `#142` (user-group binding) are merged in `main`.
- Release tag `2.1.4-rc2` points to commit `137c34e`.
- Button border/color parity depends on both bridge mapping and semantic Tailwind token export.
- Uses `rolldown-vite` (experimental Rust bundler) instead of standard Vite.

## COMMANDS
```bash
cd vite-frontend
npm run dev
npm run build
npm run lint
```
