# Go-Gost/X Agent Instructions

## Build Commands

```bash
cd go-gost/x && go build ./...
```

## Test Commands

```bash
# All tests
cd go-gost/x && go test ./...

# Single test by name
cd go-gost/x && go test ./socket -run TestSpecificName -v

# Test specific package
cd go-gost/x && go test ./handler/...
```

## Lint Commands

```bash
cd go-gost/x && go vet ./...
```

## Code Style - Go

### Imports
Standard library first, external packages second, local packages third. Blank lines between groups.

### File Patterns
Handlers/listeners/dialers follow consistent pattern:
- `{type}.go` - main implementation
- `metadata.go` - configuration metadata

### OS-Specific Code
Use `name_[os].go` suffix:
- `tun_linux.go`
- `tun_darwin.go`
- `tun_windows.go`

## Project Structure

```
go-gost/x/
├── api/        # Gin management API + swagger docs
├── config/     # Config model + parsing/load/reload
├── connector/  # Outbound connect implementations
├── dialer/     # Outbound dialers (tcp/tls/ws/quic/...)
├── handler/    # Protocol handlers (socks/http/tunnel/...)
├── listener/   # Inbound listeners (tcp/udp/tun/tap/...)
├── limiter/    # Traffic/rate/conn limiters
├── registry/   # Registries for pluggable components
├── service/    # Service wrappers + reporting hooks
├── socket/     # WebSocket reporter / panel integration
└── internal/   # Shared internals (grpc proto, net utils)
```

## Critical Conventions

### Module Independence
`go-gost/x/` is a standalone Go module with its own `go.mod`.

### Component Registration
Components register via `registry.Register{Type}(name, creator)`:
```go
registry.RegisterHandler("http", NewHandler)
registry.RegisterListener("tcp", NewListener)
```

### Panel Reporting
`x/socket/websocket_reporter.go` handles agent-to-panel telemetry.

## Anti-Patterns

- DO NOT edit generated protobuf in `internal/util/grpc/proto/`
- DO NOT edit `*.pb.go` or `*_grpc.pb.go` files