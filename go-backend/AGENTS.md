# Go Backend Agent Instructions

## Build Commands

```bash
cd go-backend && go build ./cmd/paneld
cd go-backend && go run ./cmd/paneld          # SERVER_ADDR=:6365
cd go-backend && make build
cd go-backend && make run
```

## Test Commands

```bash
# All tests
cd go-backend && go test ./...

# Contract tests only
cd go-backend && go test ./tests/contract/...

# Single test by name
cd go-backend && go test ./tests/contract -run TestJWTMiddlewareContracts -v

# Single test by pattern
cd go-backend && go test ./tests/contract -run TestAuthContract -v

# Run with coverage
cd go-backend && go test -cover ./...
```

## Lint Commands

```bash
cd go-backend && go vet ./...
```

## Code Style - Go

### Imports
Standard library first, external packages second, local packages third. Blank lines between groups:
```go
import (
    "context"
    "net/http"

    "gorm.io/gorm"

    "go-backend/internal/store/model"
)
```

### Error Handling
Return errors up the stack. Wrap with context using `fmt.Errorf`:
```go
if err := db.Save(&item); err != nil {
    return fmt.Errorf("save item: %w", err)
}
```

### GORM Models
Always define `TableName()` returning singular snake_case:
```go
type User struct {
    ID   int64  `gorm:"primaryKey;autoIncrement"`
    Name string `gorm:"type:varchar(100);not null"`
}

func (User) TableName() string { return "user" }
```

Use `sql.NullString` / `sql.NullInt64` for nullable fields.

### Repository Pattern
Handlers NEVER call `repo.DB()` directly. Add Repository methods:
```go
// In repository.go
func (r *Repository) GetUserByID(id int64) (*User, error) { ... }

// In handler
user, err := h.repo.GetUserByID(id)
```

### API Response Envelope
All responses use `response.R{code, msg, data, ts}`:
```go
response.WriteJSON(w, response.OK(data))        // Success (code 0)
response.WriteJSON(w, response.Err(401, "msg")) // Error
```

## Project Structure

```
go-backend/
├── cmd/paneld/main.go        # Entry point
├── internal/
│   ├── http/
│   │   ├── router.go         # Routes (NewServeMux)
│   │   ├── handler/          # API handlers
│   │   ├── middleware/       # JWT, CORS, Logging
│   │   └── response/         # JSON helpers (r.go)
│   ├── store/
│   │   ├── model/model.go    # GORM structs
│   │   └── repo/             # Repository pattern
│   └── auth/                 # JWT logic
└── tests/contract/           # Integration tests
```

## Critical Conventions

### Authentication Header
Raw JWT token, NO "Bearer" prefix:
```go
// Middleware extracts raw header
token := r.Header.Get("Authorization")
```

### SQLite Constraints
- `MaxOpenConns(1)` required
- WAL mode, busy_timeout=5000
- No `type:jsonb` or `type:serial` in GORM tags

### PostgreSQL Support
Set `DB_TYPE=postgres` and `DATABASE_URL` env vars.

## Anti-Patterns

- DO NOT let handlers call `repo.DB()` directly
- DO NOT add `Bearer` prefix to Authorization header
- DO NOT omit `TableName()` on new models
- DO NOT use `type:jsonb` or `type:serial` in GORM tags
- DO NOT change handler signatures without updating router.go