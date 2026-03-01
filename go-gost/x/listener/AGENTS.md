# Listener Agent Instructions

## Build Commands

```bash
cd go-gost/x && go build ./listener/...
```

## Test Commands

```bash
cd go-gost/x && go test ./listener/...
```

## Code Style - Go

### File Pattern
Each transport has a subdirectory with:
- `listener.go` - main implementation
- `metadata.go` - configuration metadata

### Implementation Pattern
```go
type tcpListener struct {
    addr net.Addr
    // ...
}

func (l *tcpListener) Init(md md.MD) error {
    // Initialize from metadata
}

func (l *tcpListener) Accept() (net.Conn, error) {
    // Accept incoming connections
}
```

## Project Structure

```
listener/
├── tcp/       # TCP listener
├── udp/       # UDP listener
├── tls/       # TLS listener
├── ws/        # WebSocket listener
├── quic/      # QUIC listener
├── redirect/  # Transparent redirect
├── tun/       # TUN device
├── tap/       # TAP device
└── ...
```

## Critical Conventions

### Registration
Each listener registers itself in `init()`:
```go
func init() {
    registry.RegisterListener("tcp", NewListener)
}
```

### OS-Specific Code
Use build tags for OS-specific implementations:
- `tun_linux.go`
- `tun_darwin.go`

## Anti-Patterns

- DO NOT edit generated protobuf files