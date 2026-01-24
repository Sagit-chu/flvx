# VITE FRONTEND KNOWLEDGE BASE

## OVERVIEW
Web management console for Flux Panel.
**Stack:** React 18, Vite 5, TypeScript, TailwindCSS 4, HeroUI (NextUI).

## STRUCTURE
```
vite-frontend/
├── src/
│   ├── pages/        # Route views
│   ├── components/   # Reusable UI parts
│   ├── layouts/      # Page wrappers
│   ├── api/          # Axios wrappers
│   ├── config/       # App settings
│   └── utils/        # Helpers
├── vite.config.ts    # Vite config (Base: '/')
└── package.json
```

## CONVENTIONS
- **UI Lib**: HeroUI (formerly NextUI) + Tailwind CSS 4.
- **Routing**: React Router DOM 6.
- **State**: Check `provider.tsx` or local state.
- **Build**: Output to `dist/`.

## COMMANDS
```bash
# Dev
npm run dev

# Build
npm run build
```
