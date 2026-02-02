# GO-GOST/X API KNOWLEDGE BASE

## OVERVIEW
Gin-based management API for reading/writing config and controlling services at runtime.

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Route registration | `go-gost/x/api/api.go` | `Register(*gin.Engine, *Options)` |
| Auth gating | `go-gost/x/api/middleware.go` | Drops non-BasicAuth requests; optional auther check |
| Service CRUD + pause/resume | `go-gost/x/api/config_service.go` | Uses registry + `config.OnUpdate(...)` |
| Swagger spec | `go-gost/x/api/swagger.yaml` | Served at `/docs` via embedded FS |

## CONVENTIONS
- CORS is `AllowAllOrigins: true` (see `go-gost/x/api/api.go`).
- Requests without a valid Basic `Authorization` header are silently dropped (connection hijack + close) by `GlobalInterceptor()`.
- Many operations mutate the in-memory config via `config.OnUpdate(...)` after starting/stopping services.

## COMMANDS
```bash
cd go-gost/x
go test ./...
```
