# Handler Agent Instructions

## Build Commands

```bash
cd go-gost/x && go build ./handler/...
```

## Test Commands

```bash
cd go-gost/x && go test ./handler/...
```

## Code Style - Go

### File Pattern
Each protocol has a subdirectory with:
- `handler.go` - main implementation
- `metadata.go` - configuration metadata

### Implementation Pattern
```go
type httpHandler struct {
    options *Options
}

func (h *httpHandler) Init(md md.MD) error {
    // Initialize from metadata
}

func (h *httpHandler) Handle(ctx context.Context, conn net.Conn) {
    // Handle incoming connection
}
```

## Project Structure

```
handler/
├── http/      # HTTP proxy handler
├── socks/     # SOCKS v4/v5 handlers
├── tunnel/    # Tunnel forwarding
├── relay/     # Relay forwarding
├── redirect/  # TCP/UDP redirect
├── router/    # Routing entrypoints
└── ...
```

## Critical Conventions

### Registration
Each handler registers itself in `init()`:
```go
func init() {
    registry.RegisterHandler("http", NewHandler)
}
```

### Protocol Handling
Handlers implement the `Handler` interface from `github.com/go-gost/core`.

## Anti-Patterns

- DO NOT edit generated protobuf files