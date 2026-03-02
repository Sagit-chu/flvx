# Connector Agent Instructions

## Build Commands

```bash
cd go-gost/x && go build ./connector/...
```

## Test Commands

```bash
cd go-gost/x && go test ./connector/...
```

## Code Style - Go

### File Pattern
Each protocol has a subdirectory with:
- `connector.go` - main implementation
- `metadata.go` - configuration metadata

### Implementation Pattern
```go
type httpConnector struct {
    options *Options
}

func (c *httpConnector) Init(md md.MD) error {
    // Initialize from metadata
}

func (c *httpConnector) Connect(ctx context.Context, conn net.Conn) (net.Conn, error) {
    // Establish protocol-level connection
}
```

## Project Structure

```
connector/
├── direct/    # Direct connection
├── forward/   # Forward proxy
├── http/      # HTTP connector
├── http2/     # HTTP/2 connector
├── relay/     # Relay protocol
├── router/    # Router connector
├── socks/     # SOCKS4/5
├── ss/        # Shadowsocks
├── sshd/      # SSH daemon
├── tcp/       # TCP connector
├── tunnel/    # Tunnel mode
└── unix/      # Unix socket
```

## Critical Conventions

### Registration
Each connector registers itself in `init()`:
```go
func init() {
    registry.RegisterConnector("http", NewConnector)
}
```

### Interface
Connectors implement the `Connector` interface from `github.com/go-gost/core`.

## Anti-Patterns

- DO NOT edit generated protobuf files