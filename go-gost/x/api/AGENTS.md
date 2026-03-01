# API Agent Instructions

## Build Commands

```bash
cd go-gost/x && go build ./api/...
```

## Test Commands

```bash
cd go-gost/x && go test ./api/...
```

## Code Style - Go

### Route Registration
```go
func Register(r *gin.Engine, opts *Options) {
    r.GET("/config", getConfig)
    r.POST("/config/services", createService)
    // ...
}
```

## Project Structure

```
api/
├── api.go              # Route registration
├── middleware.go       # BasicAuth + interceptor
├── config.go           # Config endpoints
├── config_service.go   # Service CRUD + pause/resume
└── swagger.yaml        # OpenAPI spec (served at /docs)
```

## Critical Conventions

### Authentication
Requests without valid Basic `Authorization` header are dropped:
```go
// GlobalInterceptor drops non-BasicAuth requests
func GlobalInterceptor() gin.HandlerFunc { ... }
```

### CORS
`AllowAllOrigins: true` - see `api.go`.

### Config Mutations
Use `config.OnUpdate(...)` after starting/stopping services:
```go
config.OnUpdate(func(cfg *config.Config) error {
    // Apply changes
    return nil
})
```

### Swagger
Served at `/docs` via embedded FS.

## Anti-Patterns

- DO NOT bypass BasicAuth for management endpoints
- DO NOT edit generated protobuf files