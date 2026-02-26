# GO BACKEND KNOWLEDGE BASE

## OVERVIEW
Go-based Admin API for FLVX. Replaced legacy Spring Boot backend.
**Stack:** Go 1.24, net/http (std lib), GORM + SQLite/PostgreSQL (glebarez/sqlite - CGO-free).

## STRUCTURE
```
go-backend/
├── cmd/paneld/main.go        # Entry point; starts HTTP server + WebSocket
├── internal/
│   ├── http/                 # HTTP layer
│   │   ├── router.go         # Routes (NewServeMux) + Middleware chain
│   │   ├── handler/          # API Handlers (User, Tunnel, Node, etc.)
│   │   ├── middleware/       # JWT, CORS, Logging, Recover
│   │   └── response/         # JSON response helpers
│   ├── store/
│   │   ├── model/model.go    # GORM model structs (single source of truth)
│   │   └── repo/             # Data Access Layer (Repository pattern, GORM)
│   │       ├── repository.go           # Core queries, Open/OpenPostgres, AutoMigrate (83k LOC)
│   │       ├── repository_mutations.go # Mutation helpers (user/node/tunnel/forward CRUD, 43k LOC)
│   │       ├── repository_federation.go # Federation-specific queries
│   │       ├── repository_flow.go      # Flow/forward status queries
│   │       ├── repository_control.go   # Control plane queries
│   │       └── repository_groups.go    # Group management queries
│   └── auth/                 # Auth logic
├── tests/contract/            # Integration/contract tests (14 tests)
├── Dockerfile                # Multi-stage build (golang:1.24-bookworm → debian:bookworm-slim)
└── Makefile                  # Build commands
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| **API Routes** | `go-backend/internal/http/router.go` | Registers handlers to `http.ServeMux` |
| **DB Models** | `go-backend/internal/store/model/model.go` | GORM structs with `TableName()` methods |
| **Repository** | `go-backend/internal/store/repo/` | GORM-based queries, all DB ops encapsulated |
| **Auth Middleware** | `go-backend/internal/http/middleware/jwt.go` | Extracts `Authorization` header |
| **WebSocket** | `go-backend/internal/ws/` | Real-time updates (traffic, status) |
| **Contract Tests** | `go-backend/tests/contract/` | Integration tests for auth, federation, tunnels |

## CONVENTIONS
- **GORM ORM**: Uses GORM with `glebarez/sqlite` (CGO-free) and `gorm.io/driver/postgres`.
- **AutoMigrate**: Schema created at startup via `autoMigrateAll()` — no hand-written DDL.
- **TableName()**: All models define explicit `TableName()` returning singular snake_case names.
- **Repository Pattern**: Handlers never access `*gorm.DB` directly — all queries go through `repo.Repository` methods.
- **Standard Lib**: Uses `net/http` for routing (Go 1.22+ patterns).
- **Auth**: Expects raw JWT in `Authorization` header (no `Bearer` prefix).
- **API Envelope**: All responses use `response.R{code, msg, data, ts}` structure.
- **Config**: Loaded from environment variables (see `cmd/paneld/main.go`).
- **SQLite Constraints**: `MaxOpenConns(1)`, WAL mode, busy_timeout=5000.
- **PostgreSQL**: Supported via `DB_TYPE=postgres` and `DATABASE_URL` env vars.

## ANTI-PATTERNS
- **DO NOT** let handlers call `repo.DB()` directly — add a Repository method instead.
- **DO NOT CHANGE** handler signatures without updating `router.go`.
- **DO NOT** use `type:jsonb` or `type:serial` in GORM tags (SQLite incompatible).
- **DO NOT** omit `TableName()` on new models — GORM pluralizes by default.

## COMMANDS
```bash
cd go-backend
go run ./cmd/paneld       # Default: SERVER_ADDR=:6365
go test ./...             # Unit tests
go test ./tests/contract/... # Contract tests
make build
```
