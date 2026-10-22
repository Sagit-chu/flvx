# AGENTS

FLVX — traffic forwarding panel: Go admin API + Vite/React UI + Go agent.

## Structure

| Dir | Role | Entry |
|-----|------|-------|
| `go-backend/` | Admin API (GORM + SQLite/PG, net/http) | `cmd/paneld/main.go` |
| `go-gost/` | Forwarding agent (forked GOST) | `main.go` |
| `go-gost/x/` | Protocol handlers/dialers/listeners (own module) | — |
| `vite-frontend/` | React dashboard (shadcn bridge + Tailwind v4) | `src/App.tsx` |

`go-gost/go.mod` uses `replace github.com/go-gost/x => ./x`.

## Commands

```bash
# Backend
(cd go-backend && go run ./cmd/paneld)        # SERVER_ADDR defaults to :6365
(cd go-backend && make build)
(cd go-backend && go test ./...)

# Frontend
(cd vite-frontend && pnpm install)
(cd vite-frontend && pnpm run dev)             # host 0.0.0.0:3000
(cd vite-frontend && pnpm run build)           # tsc && vite build
(cd vite-frontend && pnpm run lint)            # eslint --fix (no typecheck command)

# Agent
(cd go-gost && go run .)
```

## Conventions

- **Auth**: raw JWT in `Authorization` header — **no `Bearer` prefix** (both frontend and backend).
- **API envelope**: all responses `{code, msg, data, ts}` (code 0 = success).
- **Frontend UI**: import from `src/shadcn-bridge/heroui/*`, never `@heroui/*` or `@nextui-org/*`.
- **Tailwind theme**: `globals.css` must import `tailwind-theme.pcss` or semantic classes break.
- **Backend DB**: handlers use Repository methods, never `repo.DB()` directly.
- **GORM models**: always define `TableName()` (GORM pluralizes by default).
- **GORM tags**: no `type:jsonb` or `type:serial` (SQLite incompatible).
- **Go versions**: all three modules require Go 1.26.10 or newer (CVE-2026-111943); CI reads the module's `go.mod`. Do not use Go 1.27.0–1.27.2, which also lack this fix.
- **Node.js versions**: `.node-version` pins 22.23.4 for CI; local builds also support patched 24.22.1+ and 26.12.1+ (CVE-2026-111497).

## Anti-patterns

- Don't edit `install.sh` or `panel_install.sh` locally (CI overwrites on release).
- Don't edit `go-gost/x/internal/util/grpc/proto/*.pb.go` (generated).
- Don't add frontend tests (no Vitest/Jest configured).
- Don't reintroduce `@heroui/*` or `@nextui-org/*` packages.

## Testing

- Backend: `(cd go-backend && go test ./...)` — includes contract tests in `tests/contract/`.
- Frontend: no test infrastructure.
- CI runs one PostgreSQL contract test: env var `FLVX_POSTGRES_TEST_DSN`.

## Build quirks

- `vite-frontend` uses `rolldown-vite` (Rust bundler), not standard Vite.
- `vite.config.ts`: `minify: false`, `treeshake: false` (debugging mode).
- CI builds `go-gost` with `CGO_ENABLED=0` then compresses with UPX `--best --lzma`.
