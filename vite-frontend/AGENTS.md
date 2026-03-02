# Vite Frontend Agent Instructions

## Build Commands

```bash
cd vite-frontend && npm run dev
cd vite-frontend && npm run build
cd vite-frontend && npm run preview
```

## Lint Commands

```bash
cd vite-frontend && npm run lint
```

## Code Style - TypeScript/React

### Import Order (ESLint enforced)
Types first, then builtins, external, internal, parent/sibling/index. Blank lines between groups.

### Component Style
Functional components with hooks. No prop-types:
```tsx
export function MyComponent({ id, onSave }: Props) {
  const [data, setData] = useState<Data | null>(null)
  // ...
}
```

### Naming Conventions
- Components: PascalCase files and names (`UserList.tsx` → `UserList`)
- Hooks: camelCase with `use` prefix (`useAuth.ts`)
- Utils: camelCase files and functions

### UI Components
Import from `@/shadcn-bridge/heroui/*`, NOT from `@heroui/*`:
```tsx
import { Button } from "@/shadcn-bridge/heroui/button"
import { Input } from "@/shadcn-bridge/heroui/input"
```

### TypeScript Config
- Path alias: `@/*` → `./src/*`
- Strict mode enabled
- `noUnusedLocals: true`, `noUnusedParameters: true`

## Project Structure

```
vite-frontend/
├── src/
│   ├── api/                      # Axios wrapper
│   ├── components/ui/            # shadcn/radix primitives
│   ├── shadcn-bridge/heroui/     # HeroUI-compatible facade
│   ├── pages/                    # Route views
│   ├── hooks/                    # Custom hooks
│   ├── styles/
│   │   ├── globals.css           # Must import tailwind-theme.pcss
│   │   └── tailwind-theme.pcss   # Semantic color tokens
│   ├── App.tsx                   # Routes + ProtectedRoute
│   └── main.tsx                  # Entry point
├── tsconfig.json
└── vite.config.ts                # rolldown-vite, minify: false
```

## Critical Conventions

### Authentication Header
Raw JWT token, NO "Bearer" prefix:
```typescript
// network.ts
axios.defaults.headers.common["Authorization"] = token
```

### API Response Envelope
All responses follow `{code, msg, data, ts}`. Code 0 = success.

### Theme Tokens
`globals.css` must import `./tailwind-theme.pcss` or semantic classes like `bg-primary` break.

### Build Profile
- Uses `rolldown-vite` (Rust bundler)
- `minify: false`, `treeshake: false` for debugging

## Anti-Patterns

- DO NOT add `Bearer` prefix to auth header
- DO NOT import from `@heroui/*` or `@nextui-org/*`
- DO NOT remove `tailwind-theme.pcss` import from `globals.css`
- DO NOT add frontend tests (no Vitest/Jest setup)