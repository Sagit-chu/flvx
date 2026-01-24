# GO-GOST SERVICE KNOWLEDGE BASE

## OVERVIEW
Core forwarding service based on GOST v3.
**Stack:** Go 1.23, GOST Core v0.3.1, GOST x (Extensions).

## STRUCTURE
```
go-gost/
├── main.go           # Entry point
├── x/                # Local extensions (REPLACES github.com/go-gost/x)
│   ├── api/          # Management API
│   ├── registry/     # Service registry
│   ├── handler/      # Protocol handlers (socks, tunnel, relay)
│   └── listener/     # Network listeners (tcp, udp, tun/tap)
└── go.mod            # Defines local replacement
```

## CONVENTIONS
- **Local Replace**: `go.mod` uses `replace github.com/go-gost/x => ./x`.
- **Extensions**: Custom logic lives in `x/`. This is the primary place for modifications.
- **Handlers**: Implements SOCKS5, Tunnel, Relay, etc.

## COMMANDS
```bash
# Run
go run .

# Build
go build .
```
