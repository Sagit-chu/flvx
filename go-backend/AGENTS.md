# GO BACKEND KNOWLEDGE BASE

## OVERVIEW
Go-based Admin API for FLVX. Replaced legacy Spring Boot backend.
**Stack:** Go 1.23, net/http (std lib), SQLite/PostgreSQL (modernc.org/sqlite - CGO-free).

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
│   ├── store/sqlite/         # Data Access Layer (Repository pattern)
│   │   ├── repository.go     # SQL queries & Struct definitions
│   │   └── sql/              # Embedded schema.sql & data.sql
│   └── auth/                 # Auth logic
├── tests/                    # Integration/Contract tests
├── Dockerfile                # Multi-stage build (alpine)
└── Makefile                  # Build commands
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| **API Routes** | `go-backend/internal/http/router.go` | Registers handlers to `http.ServeMux` |
| **DB Schema** | `go-backend/internal/store/sqlite/sql/schema.sql` | Embedded in binary |
| **SQL Queries** | `go-backend/internal/store/sqlite/repository.go` | Raw SQL, no ORM |
| **Auth Middleware** | `go-backend/internal/http/middleware/jwt.go` | Extracts `Authorization` header |
| **WebSocket** | `go-backend/internal/ws/` | Real-time updates (traffic, status) |

## CONVENTIONS
- **No ORM**: Uses raw SQL with `database/sql` and `modernc.org/sqlite`.
- **CGO-Free SQLite**: `modernc.org/sqlite` instead of `mattn/go-sqlite3` - builds without CGO.
- **Standard Lib**: Uses `net/http` for routing (Go 1.22+ patterns).
- **Auth**: Expects raw JWT in `Authorization` header (no `Bearer` prefix).
- **API Envelope**: All responses use `response.R{code, msg, data, ts}` structure.
- **Config**: Loaded from environment variables (see `cmd/paneld/main.go`).
- **SQL Idempotency**: Prefer `ON CONFLICT DO NOTHING` for inserts in migrations/sync.

## ANTI-PATTERNS
- **DO NOT USE** ORM - uses raw SQL throughout.
- **DO NOT CHANGE** handler signatures without updating `router.go`.

## COMMANDS
```bash
cd go-backend
go run ./cmd/paneld       # Default: SERVER_ADDR=:6365
go test ./...
make build
```
