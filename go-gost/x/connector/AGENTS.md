# GOST CONNECTOR KNOWLEDGE BASE

**Generated:** Fri Feb 13 2026

## OVERVIEW
Connection initiators (clients) for various protocols in GOST forwarding.
**Stack:** Go, GOST core.

## STRUCTURE
```
connector/
├── direct/    # Direct connection
├── forward/   # Forward proxy
├── http/      # HTTP connector
├── http2/     # HTTP/2 connector
├── relay/     # Relay protocol
├── router/    # Router connector
├── serial/    # Serial port
├── sni/       # SNI routing
├── socks/     # SOCKS4/5
├── ss/        # Shadowsocks
├── sshd/      # SSH daemon
├── tcp/       # TCP connector
├── tunnel/    # Tunnel mode
└── unix/      # Unix socket
```

## CONVENTIONS
- Inherits from parent `go-gost/x/` conventions.
- Each subdir implements `Connector` interface from GOST core.

## ANTI-PATTERNS
- DO NOT EDIT generated protobuf in `go-gost/x/internal/util/grpc/proto/`.

## COMMANDS
```bash
cd go-gost
go test ./x/connector/...
```
