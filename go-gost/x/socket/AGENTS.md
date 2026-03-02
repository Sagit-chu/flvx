# Socket Agent Instructions

## Build Commands

```bash
cd go-gost/x && go build ./socket/...
```

## Test Commands

```bash
cd go-gost/x && go test ./socket/...
cd go-gost/x && go test ./socket -run TestSpecificName -v
```

## Code Style - Go

### Imports
Standard library first, external packages second, local packages third.

### Error Handling
Return errors up the stack with context:
```go
if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
    return fmt.Errorf("write message: %w", err)
}
```

## Project Structure

```
socket/
├── websocket_reporter.go  # Agent-to-panel telemetry
├── service.go             # Socket service orchestration
├── socket.go              # Core socket interface
├── udp.go                 # UDP socket handling
├── packet.go              # Packet framing
└── packetconn.go          # Packet connection wrapper
```

## Critical Conventions

### Panel Reporting
`websocket_reporter.go` sends real-time system info (CPU, memory, uptime) every 2s.

### Command Handling
Processes commands like `AddService`, `UpgradeAgent`, etc.

### Encryption
All panel communication is AES-encrypted using node `secret`.

## Anti-Patterns

- DO NOT edit generated protobuf files