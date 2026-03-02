# Handler Agent Instructions

## Build Commands

```bash
cd go-backend && go build ./internal/http/handler/...
```

## Test Commands

```bash
cd go-backend && go test ./internal/http/handler/...
```

## Code Style - Go

### Imports
Standard library first, external second, local third:
```go
import (
    "context"
    "net/http"

    "go-backend/internal/http/response"
    "go-backend/internal/store/repo"
)
```

### Handler Pattern
```go
func (h *Handler) userList(w http.ResponseWriter, r *http.Request) {
    users, err := h.repo.ListUsers()
    if err != nil {
        response.WriteJSON(w, response.ErrDefault("获取用户列表失败"))
        return
    }
    response.WriteJSON(w, response.OK(users))
}
```

### Request Structs
Define request structs at package level:
```go
type loginRequest struct {
    Username string `json:"username"`
    Password string `json:"password"`
}
```

## Project Structure

```
handler/
├── handler.go        # Main Handler struct, login/captcha
├── mutations.go      # CRUD for users, tunnels, forwards
├── control_plane.go  # Node control plane API
├── federation.go     # Federation/cluster sync
├── flow_policy.go    # Traffic policy API
├── jobs.go           # Background job management
└── upgrade.go        # System upgrade API
```

## Critical Conventions

### Repository Pattern
Never call `repo.DB()` directly. Use Repository methods:
```go
// Correct
user, err := h.repo.GetUserByID(id)

// Wrong
var user model.User
h.repo.DB().First(&user, id)
```

### Response Envelope
All responses use `response.R`:
```go
response.WriteJSON(w, response.OK(data))        // Success
response.WriteJSON(w, response.Err(401, "msg")) // Error
```

### Domain-Driven Split
One file per functional area (federation, jobs, flow policy, etc.)

## Anti-Patterns

- DO NOT call `repo.DB()` directly
- DO NOT change handler signatures without updating router.go