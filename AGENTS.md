# PROJECT KNOWLEDGE BASE

**Generated:** Mon Feb 02 2026
**Commit:** 7ca01ab
**Branch:** beta

## OVERVIEW
Flux Panel is a traffic forwarding management system built on a forked GOST v3 stack. It ships as Dockerized Spring Boot (admin API) + Vite/React UI + Go forwarding agent, with optional mobile WebView wrappers.

## STRUCTURE
```
./
├── go-gost/               # Go forwarding agent (forked gost + local x/)
│   └── x/                 # Local fork of github.com/go-gost/x (replace => ./x)
├── springboot-backend/    # Java/Spring Boot admin API (SQLite/MyBatis)
├── vite-frontend/         # React/Vite dashboard (HeroUI + Tailwind)
├── android-app/           # Android WebView wrapper (optional)
├── ios-app/               # iOS WebView wrapper (optional)
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
| **Admin API entry** | `springboot-backend/src/main/java/com/admin/AdminApplication.java` | Spring Boot app |
| **Admin API routes** | `springboot-backend/src/main/java/com/admin/controller/` | Mostly `/api/v1/*` controllers |
| **Admin auth** | `springboot-backend/src/main/java/com/admin/common/interceptor/JwtInterceptor.java` | Checks `Authorization` header |
| **Web UI routing** | `vite-frontend/src/App.tsx` | React Router v6 + ProtectedRoute/H5 layouts |
| **Web UI API client** | `vite-frontend/src/api/network.ts` | Axios `baseURL` + `Authorization` header |
| **Go agent entry** | `go-gost/main.go` | Reads panel `config.json` + starts gost services |
| **Go x fork** | `go-gost/x/` | Handlers/listeners/dialers + management API |

## CONVENTIONS
- `Authorization` header carries the raw JWT token (no `Bearer` prefix) between `vite-frontend/` and `springboot-backend/`.
- `go-gost/` uses `replace github.com/go-gost/x => ./x` and `go-gost/x/` is also its own Go module.

## ANTI-PATTERNS (THIS PROJECT)
- Do not edit generated protobuf output: `go-gost/x/internal/util/grpc/proto/*.pb.go`, `go-gost/x/internal/util/grpc/proto/*_grpc.pb.go`.

## COMMANDS
```bash
# Panel (Docker)
docker compose -f docker-compose-v4.yml up -d
docker compose -f docker-compose-v6.yml up -d

# Release-based install scripts
./panel_install.sh
./install.sh

# Local dev (per subproject)
(cd springboot-backend && mvn clean package)
(cd vite-frontend && npm run dev)
(cd go-gost && go run .)
```

## NOTES
- LSP servers are not installed in this environment (gopls/jdtls/typescript-language-server); rely on grep-based navigation.
- `vite-frontend/vite.config.ts` sets `minify: false` and disables treeshake; expect larger bundles.
