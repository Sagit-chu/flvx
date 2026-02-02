# GO-GOST/X HANDLERS KNOWLEDGE BASE

## OVERVIEW
Protocol handlers (server-side request handling) used by services defined in the GOST config.

## STRUCTURE
```
go-gost/x/handler/
├── http/     # handler.go + metadata.go (+ udp.go)
├── socks/    # SOCKS variants
├── tunnel/   # Tunnel forwarding
├── relay/    # Relay forwarding
├── redirect/ # TCP/UDP redirect handlers
├── router/   # Routing/association entrypoints
└── ...
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Find a protocol handler | `go-gost/x/handler/` | Subdir per protocol (`http`, `socks`, `tunnel`, ...) |
| HTTP specifics | `go-gost/x/handler/http/handler.go` | Implements HTTP proxy behavior |
| SOCKS specifics | `go-gost/x/handler/socks/` | v4/v5 implementations |

## CONVENTIONS
- Handler implementations typically live in `handler.go` with a paired `metadata.go` (e.g. `go-gost/x/handler/http/`).

## COMMANDS
```bash
cd go-gost/x
go test ./...
```
