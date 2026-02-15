# PROJECT KNOWLEDGE BASE

**Generated:** Sun Feb 15 2026
**Commit:** e5e22ba
**Branch:** main

## OVERVIEW
FLVX (formerly Flux Panel) is a traffic forwarding management system built on a forked GOST v3 stack. It ships as a Go-based admin API (SQLite) + Vite/React UI + Go forwarding agent, with optional mobile WebView wrappers.

## STRUCTURE
```
./
├── go-gost/               # Go forwarding agent (forked gost + local x/)
│   └── x/                 # Local fork of github.com/go-gost/x (replace => ./x)
├── go-backend/            # Go Admin API (SQLite, net/http)
├── vite-frontend/         # React/Vite dashboard (HeroUI + Tailwind)
├── docker-compose-v4.yml  # Panel deploy (IPv4-only bridge)
├── docker-compose-v6.yml  # Panel deploy (IPv6-enabled bridge)
├── panel_install.sh       # Panel installer/upgrader (downloads compose)
├── install.sh             # Node installer/upgrader (downloads gost binary)
└── .github/workflows/     # CI: build/push images + release artifacts
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| **Deploy (Docker)** | `docker-compose-v4.yml` | Env: `JWT_SECRET`, `BACKEND_PORT`, `FRONTEND_PORT` |
| **Deploy (IPv6)** | `docker-compose-v6.yml` | Same as v4 + IPv6-enabled bridge |
| **Panel install** | `panel_install.sh` | Picks v4/v6, generates `JWT_SECRET`, downloads compose |
| **Node install** | `install.sh` | Installs `/etc/flux_agent/flux_agent` + writes `config.json`/`gost.json` + systemd `flux_agent.service` |
| **Admin API** | `go-backend/` | Go Admin API (SQLite) |
| **Web UI** | `vite-frontend/` | React/Vite dashboard (HeroUI + Tailwind) |
| **Go Agent** | `go-gost/` | Forwarding agent (forked gost + local x/) |
| **Go Core** | `go-gost/x/` | Handlers/listeners/dialers + management API |

## CODE MAP
| Symbol | Type | Location | Role |
|--------|------|----------|------|
| `flvx` | Project | `.` | Root directory |
| `main` | Func | `go-backend/cmd/paneld/main.go` | Backend Entry |
| `App` | Component | `vite-frontend/src/App.tsx` | Frontend Entry |
| `main` | Func | `go-gost/main.go` | Agent Entry |


## CONVENTIONS
- **Auth**: `Authorization` header carries the raw JWT token (no `Bearer` prefix) between `vite-frontend/` and `go-backend/`.
- **Module Fork**: `go-gost/` uses `replace github.com/go-gost/x => ./x` and `go-gost/x/` is also its own Go module.
- **Encryption**: Agent-to-panel communication uses AES encryption with node `secret` as PSK.
- **API Envelope**: All REST responses follow `{code, msg, data, ts}` structure (code 0 = success).

## ANTI-PATTERNS (THIS PROJECT)
- **DO NOT EDIT** generated protobuf output: `go-gost/x/internal/util/grpc/proto/*.pb.go`, `go-gost/x/internal/util/grpc/proto/*_grpc.pb.go`.
- **DO NOT ADD** `Bearer` prefix to Authorization header - expects raw JWT token.
- **DO NOT MODIFY** `install.sh` or `panel_install.sh` locally - CI overwrites these on release.
- **DO NOT USE** ORM in backend - uses raw SQL with `database/sql`.
- **DO NOT ADD** frontend tests - project has no test infrastructure (Vitest/Jest not configured).

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
```

## UNIQUE STYLES
- **Flat Monorepo**: Language-prefixed dirs (`go-backend`, `go-gost`, `vite-frontend`) instead of `apps/`/`libs/`.
- **Asymmetric Go Layout**: `go-backend` follows `cmd/<app>/main.go` while `go-gost` uses `root/main.go`.
- **Frontend Hybrid Mode**: `App.tsx` detects "H5 mode" (mobile WebView) vs desktop, dictating layout strategy.

## NOTES
- LSP servers are not installed in this environment (gopls/jdtls/typescript-language-server); rely on grep-based navigation.
- `vite-frontend/vite.config.ts` sets `minify: false` and disables treeshake; expect larger bundles.
- `vite-frontend` uses `rolldown-vite` (experimental Rust bundler) instead of standard Vite.
- Install scripts (`install.sh`, `panel_install.sh`) self-delete after execution - common pattern in one-liner installs.
- CI uses UPX compression (`--best --lzma`) on Go binaries before release.
- CI dynamically injects `PINNED_VERSION` into install scripts and docker-compose files during releases.
- `panel_install.sh` auto-detects IPv6 and modifies `/etc/docker/daemon.json` to enable IPv6 bridge.
- Download proxy `https://gcode.hostcentral.cc/` used for GitHub downloads in China/restricted environments.
- Backend has contract tests in `go-backend/tests/contract/` - frontend has no test infrastructure (Vitest/Jest not configured).
- `analysis/3x-ui/` contains a separate git repo for reference/comparison - not part of FLVX core.
