# GO-GOST/X LISTENERS KNOWLEDGE BASE

## OVERVIEW
Inbound listeners (transport-level accept loops) used by services defined in the GOST config.

## STRUCTURE
```
go-gost/x/listener/
├── tcp/      # listener.go + metadata.go
├── udp/
├── tls/
├── ws/
├── quic/
├── redirect/ # tcp/ + udp/
├── tun/      # TUN device listener
├── tap/      # TAP device listener
└── ...
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Listener registry | `go-gost/x/listener/` | One subdir per transport |
| TCP baseline | `go-gost/x/listener/tcp/listener.go` | Reference for other transports |
| Redirect listeners | `go-gost/x/listener/redirect/` | Per-protocol accept + redirect |
| TUN/TAP | `go-gost/x/listener/tun/`, `go-gost/x/listener/tap/` | Virtual interface listeners |

## CONVENTIONS
- Listener implementations typically live in `listener.go` with a paired `metadata.go` (e.g. `go-gost/x/listener/tcp/`).

## COMMANDS
```bash
cd go-gost/x
go test ./...
```
