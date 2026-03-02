# Dialer Agent Instructions

## Build Commands

```bash
cd go-gost/x && go build ./dialer/...
```

## Test Commands

```bash
cd go-gost/x && go test ./dialer/...
```

## Code Style - Go

### File Pattern
Each transport has a subdirectory with:
- `dialer.go` - main implementation
- `metadata.go` - configuration metadata

### Implementation Pattern
```go
type tcpDialer struct {
    options *Options
}

func (d *tcpDialer) Init(md md.MD) error {
    // Initialize from metadata
}

func (d *tcpDialer) Dial(ctx context.Context, addr string) (net.Conn, error) {
    // Establish outbound connection
}
```

## Project Structure

```
dialer/
├── direct/    # Direct connection
├── tcp/       # TCP dialer
├── udp/       # UDP dialer
├── tls/       # TLS dialer
├── ws/        # WebSocket dialer
├── quic/      # QUIC dialer
├── http2/     # HTTP/2 dialer
├── http3/     # HTTP/3 dialer
├── ssh/       # SSH dialer
├── wg/        # WireGuard dialer
└── ...
```

## Critical Conventions

### Registration
Each dialer registers itself in `init()`:
```go
func init() {
    registry.RegisterDialer("tcp", NewDialer)
}
```

### Interface
Dialers implement the `Dialer` interface from `github.com/go-gost/core`.

## Anti-Patterns

- DO NOT edit generated protobuf files