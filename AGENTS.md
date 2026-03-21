# PROJECT KNOWLEDGE BASE

**Generated:** Sat Mar 21 2026
**Commit:** 0767c9c
**Branch:** main
**Tag:** 2.1.9-beta13

## OVERVIEW
FLVX (formerly Flux Panel) is a traffic forwarding management system built on a forked GOST v3 stack. It ships as a Go-based admin API (SQLite/PostgreSQL) + Vite/React UI + Go forwarding agent, with optional mobile WebView wrappers.

## STRUCTURE
```
./
├── go-gost/               # Go forwarding agent (forked gost + local x/)
│   └── x/                 # Local fork of github.com/go-gost/x (replace => ./x)
├── go-backend/            # Go Admin API (GORM + SQLite/PostgreSQL, net/http)
│   └── tests/contract/    # Integration/contract tests
├── vite-frontend/         # React/Vite dashboard (shadcn bridge + Tailwind v4)
│   └── src/shadcn-bridge/heroui/  # HeroUI-compatible facade
├── docker-compose-v4.yml  # Panel deploy (IPv4-only bridge)
├── docker-compose-v6.yml  # Panel deploy (IPv6-enabled bridge)
├── panel_install.sh       # Panel installer/upgrader (downloads compose)
├── install.sh             # Node installer/upgrader (downloads gost binary)
└── .github/workflows/     # CI: build/test + Docker push + release artifacts
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| **Deploy (Docker)** | `docker-compose-v4.yml` | Env: `JWT_SECRET`, `BACKEND_PORT`, `FRONTEND_PORT` |
| **Deploy (IPv6)** | `docker-compose-v6.yml` | Same as v4 + IPv6-enabled bridge |
| **Panel install** | `panel_install.sh` | Picks v4/v6, generates `JWT_SECRET`, downloads compose |
| **Node install** | `install.sh` | Installs `/etc/flux_agent/flux_agent` + writes `config.json`/`gost.json` + systemd `flux_agent.service` |
| **Admin API** | `go-backend/` | Go Admin API (SQLite/PostgreSQL) |
| **Web UI** | `vite-frontend/` | React/Vite dashboard (shadcn bridge + Tailwind v4) |
| **UI Compatibility** | `vite-frontend/src/shadcn-bridge/heroui/` | HeroUI-compatible API wrappers backed by shadcn/radix |
| **Theme Tokens** | `vite-frontend/src/styles/tailwind-theme.pcss` | Tailwind v4 `@theme inline` semantic color mapping |
| **Go Agent** | `go-gost/` | Forwarding agent (forked gost + local x/) |
| **Go Core** | `go-gost/x/` | Handlers/listeners/dialers + management API |
| **Repository Layer** | `go-backend/internal/store/repo/` | GORM data access (repository.go 83k LOC) |
| **Contract Tests** | `go-backend/tests/contract/` | Integration tests for auth, federation, tunnels |
| **CI Workflows** | `.github/workflows/` | ci-build.yml, docker-build.yml, deploy-docs.yml |

## CODE MAP
| Symbol | Type | Location | Role |
|--------|------|----------|------|
| `flvx` | Project | `.` | Root directory |
| `main` | Func | `go-backend/cmd/paneld/main.go` | Backend Entry |
| `App` | Component | `vite-frontend/src/App.tsx` | Frontend Entry |
| `main` | Func | `go-gost/main.go` | Agent Entry |
| `Repository` | Struct | `go-backend/internal/store/repo/repository.go` | Data Access Layer |
| `Handler` | Struct | `go-backend/internal/http/handler/handler.go` | HTTP Handlers |
| `websocket_reporter` | Func | `go-gost/x/socket/websocket_reporter.go` | Panel Telemetry |

## CONVENTIONS
- **Skills & MCP**: Always prefer using available skills (via `skill` tool) and MCP tools when applicable. Check for relevant skills before implementing from scratch.
- **Auth**: `Authorization` header carries the raw JWT token (no `Bearer` prefix) between `vite-frontend/` and `go-backend/`.
- **Module Fork**: `go-gost/` uses `replace github.com/go-gost/x => ./x` and `go-gost/x/` is also its own Go module.
- **Encryption**: Agent-to-panel communication uses AES encryption with node `secret` as PSK.
- **API Envelope**: All REST responses follow `{code, msg, data, ts}` structure (code 0 = success).
- **Frontend UI Layer**: Import UI primitives from `src/shadcn-bridge/heroui/*` (legacy-compatible facade), not direct `@heroui/*` packages.
- **Tailwind v4 Semantic Colors**: `src/styles/globals.css` must import `src/styles/tailwind-theme.pcss`; removing it breaks semantic classes like `bg-primary`, `text-foreground`, and `border-input`.
- **Go Versions**: `go-backend` uses Go 1.24, `go-gost` uses Go 1.23, `go-gost/x` uses Go 1.22.

## ANTI-PATTERNS (THIS PROJECT)
- **DO NOT EDIT** generated protobuf output: `go-gost/x/internal/util/grpc/proto/*.pb.go`, `go-gost/x/internal/util/grpc/proto/*_grpc.pb.go`.
- **DO NOT ADD** `Bearer` prefix to Authorization header - expects raw JWT token.
- **DO NOT MODIFY** `install.sh` or `panel_install.sh` locally - CI overwrites these on release.
- **DO NOT** let backend handlers call `repo.DB()` directly — add a Repository method instead.
- **DO NOT ADD** frontend tests - project has no test infrastructure (Vitest/Jest not configured).
- **DO NOT REINTRODUCE** `@heroui/*` or `@nextui-org/*` dependencies; migration is now shadcn bridge-based.
- **DO NOT** use `type:jsonb` or `type:serial` in GORM tags (SQLite incompatible).
- **DO NOT** omit `TableName()` on new models — GORM pluralizes by default.

## COMMANDS
```bash
# Panel (Docker)
docker compose -f docker-compose-v4.yml up -d
docker compose -f docker-compose-v6.yml up -d

# Release-based install scripts
./panel_install.sh
./install.sh

# Local dev (per subproject)
(cd go-backend && make build)
(cd vite-frontend && npm run dev)
(cd go-gost && go run .)

# Testing
(cd go-backend && go test ./...)
(cd go-backend && go test ./tests/contract/...)
```

## UNIQUE STYLES
- **Flat Monorepo**: Language-prefixed dirs (`go-backend`, `go-gost`, `vite-frontend`) instead of `apps/`/`libs/`.
- **Asymmetric Go Layout**: `go-backend` follows `cmd/<app>/main.go` while `go-gost` uses `root/main.go`.
- **Frontend Hybrid Mode**: `App.tsx` detects "H5 mode" (mobile WebView) vs desktop, dictating layout strategy.
- **Experimental Bundler**: `vite-frontend` uses `rolldown-vite` (Rust-based) instead of standard Vite.
- **Non-minified Builds**: `vite.config.ts` sets `minify: false`, `treeshake: false` for debugging.

## NOTES
- LSP servers are not installed in this environment (gopls/typescript-language-server); rely on grep-based navigation.
- `vite-frontend/vite.config.ts` sets `minify: false` and disables treeshake; expect larger bundles.
- `vite-frontend` uses `rolldown-vite` (experimental Rust bundler) instead of standard Vite.
- Install scripts (`install.sh`, `panel_install.sh`) self-delete after execution - common pattern in one-liner installs.
- CI uses UPX compression (`--best --lzma`) on Go binaries before release.
- CI dynamically injects `PINNED_VERSION` into install scripts and docker-compose files during releases.
- `panel_install.sh` auto-detects IPv6 and modifies `/etc/docker/daemon.json` to enable IPv6 bridge.
- Download proxy `https://gcode.hostcentral.cc/` used for GitHub downloads in China/restricted environments.
- Backend has contract tests in `go-backend/tests/contract/` - frontend has no test infrastructure.
- `analysis/3x-ui/` contains a separate git repo for reference/comparison - not part of FLVX core.
- CI workflows: `ci-build.yml` (build check), `docker-build.yml` (multi-arch images + release), `deploy-docs.yml` (MkDocs).
- PostgreSQL migration supported via `panel_install.sh` menu option using pgloader.
- Repository layer is large: `repository.go` (83k LOC), `repository_mutations.go` (43k LOC).
- Button visual parity relies on `vite-frontend/src/shadcn-bridge/heroui/button.tsx` color mapping + `vite-frontend/src/styles/tailwind-theme.pcss` token export.

## PLAN DOCUMENT RULE
- Every new implementation plan must have a dedicated Markdown plan document.
- Store plan documents under `plans/`.
- Use an incrementing numeric prefix and a short plan-summary name: `NNN-<plan-summary>.md` (for example, `001-auth-refactor.md`, `002-federation-api-cleanup.md`).
- The numeric prefix must increase by 1 for each new plan.
- In each plan document, keep a task checklist and mark each task as completed immediately after finishing it.
