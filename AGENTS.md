# FLVX Agent Instructions

## Build Commands

```bash
# Go Backend (go 1.24)
cd go-backend && go build ./cmd/paneld
cd go-backend && go run ./cmd/paneld          # SERVER_ADDR=:6365

# Go Agent (go 1.23)
cd go-gost && go build .
cd go-gost && go run .

# Frontend
cd vite-frontend && npm run dev
cd vite-frontend && npm run build
```

## Test Commands

```bash
# All backend tests
cd go-backend && go test ./...

# Contract/integration tests
cd go-backend && go test ./tests/contract/...

# Single test (by name)
cd go-backend && go test ./tests/contract -run TestJWTMiddlewareContracts -v

# Single test (by file pattern)
cd go-backend && go test ./tests/contract -run TestAuthContract -v

# Go-gost tests
cd go-gost && go test ./...
```

## Lint Commands

```bash
# Frontend (ESLint + Prettier)
cd vite-frontend && npm run lint

# Go (no golangci-lint configured - use go vet)
cd go-backend && go vet ./...
cd go-gost && go vet ./...
```

## Code Style - Go

### Imports
Standard library first, then external packages, then local packages. Group with blank lines:
```go
import (
    "context"
    "net/http"

    "gorm.io/gorm"

    "go-backend/internal/store/model"
)
```

### Error Handling
Return errors up the stack. Use `fmt.Errorf("context: %w", err)` for wrapping:
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

## Code Style - TypeScript/React

### Import Order (enforced by ESLint)
Types first, then builtins, external, internal, parent/sibling/index. Blank lines between groups.

### Component Style
Functional components with hooks. No prop-types (TypeScript handles this):
```tsx
export function MyComponent({ id, onSave }: Props) {
  const [data, setData] = useState<Data | null>(null)
  // ...
}
```

### Naming
- Components: PascalCase files and names (`UserList.tsx` → `UserList`)
- Hooks: camelCase with `use` prefix (`useAuth.ts`)
- Utils: camelCase files and functions

### UI Components
Import from `@/shadcn-bridge/heroui/*`, NOT from `@heroui/*`:
```tsx
import { Button } from "@/shadcn-bridge/heroui/button"
```

## Critical Conventions

### Authentication Header
Raw JWT token, NO "Bearer" prefix:
```typescript
// Frontend (network.ts)
axios.defaults.headers.common["Authorization"] = token

// Go middleware extracts raw header
token := r.Header.Get("Authorization")
```

### SQLite Compatibility
- No `type:jsonb` or `type:serial` in GORM tags
- Use `sql.NullString` / `sql.NullInt64` for nullable fields
- SQLite requires `MaxOpenConns(1)` (see repository.go)

### Frontend Tests
DO NOT ADD frontend tests. No Vitest/Jest infrastructure exists.

## Project Structure

```
go-backend/           # Admin API (GORM + SQLite/PostgreSQL)
├── cmd/paneld/       # Entry point
├── internal/
│   ├── http/         # HTTP layer (router, handlers, middleware)
│   ├── store/        # Data layer (model, repo)
│   └── auth/         # JWT logic
└── tests/contract/   # Integration tests

go-gost/              # Forwarding agent (forked GOST)
├── main.go           # Entry
├── config.go         # Panel integration config
└── x/                # Local fork of github.com/go-gost/x

vite-frontend/        # React dashboard
├── src/
│   ├── api/          # Axios wrapper
│   ├── pages/        # Route views
│   ├── components/   # UI components
│   └── shadcn-bridge/heroui/  # HeroUI-compatible facade
└── vite.config.ts    # rolldown-vite, minify: false
```

## Anti-Patterns

- DO NOT add `Bearer` prefix to Authorization header
- DO NOT let handlers call `repo.DB()` directly
- DO NOT reintroduce `@heroui/*` or `@nextui-org/*` dependencies
- DO NOT remove `tailwind-theme.pcss` import from `globals.css`
- DO NOT edit generated protobuf files in `go-gost/x/internal/util/grpc/proto/`
- DO NOT omit `TableName()` on new GORM models
- DO NOT modify `install.sh` or `panel_install.sh` (CI overwrites them)