# GOST SOCKET KNOWLEDGE BASE

**Generated:** Fri Feb 13 2026

## OVERVIEW
Socket utilities and wrappers for GOST forwarding.
**Stack:** Go, GOST core.

## STRUCTURE
```
socket/
├── socket.go      # Core socket interface
├── udp.go         # UDP socket handling
├── packet.go      # Packet framing
├── packetconn.go  # Packet connection wrapper
└── ...            # Additional socket utilities
```

## CONVENTIONS
- Inherits from parent `go-gost/x/` conventions.
- Low-level network primitives.

## ANTI-PATTERNS
- DO NOT EDIT generated protobuf.

## COMMANDS
```bash
cd go-gost
go test ./x/socket/...
```
