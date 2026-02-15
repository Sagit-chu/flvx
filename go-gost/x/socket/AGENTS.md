# GOST SOCKET KNOWLEDGE BASE

**Generated:** Sun Feb 15 2026

## OVERVIEW
WebSocket reporter and socket utilities for panel integration.
**Stack:** Go, GOST core, gorilla/websocket.

## STRUCTURE
```
socket/
├── websocket_reporter.go  # Agent-to-panel telemetry (1504 LOC)
├── service.go             # Socket service orchestration (534 LOC)
├── socket.go              # Core socket interface
├── udp.go                 # UDP socket handling
├── packet.go              # Packet framing
└── packetconn.go          # Packet connection wrapper
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| **Panel Reporting** | `websocket_reporter.go` | Real-time system info (CPU, mem, uptime) every 2s |
| **Command Handling** | `websocket_reporter.go` | Processes `AddService`, `UpgradeAgent`, etc. |

## CONVENTIONS
- Inherits from parent `go-gost/x/` conventions.
- Low-level network primitives.
- All panel communication is AES-encrypted using node `secret`.

## ANTI-PATTERNS
- DO NOT EDIT generated protobuf.

## COMMANDS
```bash
cd go-gost
go test ./x/socket/...
```
