# GO-GOST/X KNOWLEDGE BASE

## OVERVIEW
Local fork of `github.com/go-gost/x` used by `go-gost/` via `replace github.com/go-gost/x => ./x`. Most protocol/runtime behavior changes happen here. 30+ top-level packages - framework-style layout.

## STRUCTURE
```
go-gost/x/
├── api/        # Gin management API + embedded swagger docs
├── config/     # Config model + parsing/load/reload
├── connector/  # Outbound connect implementations
├── dialer/     # Outbound dialers (tcp/tls/ws/quic/...) 
├── handler/    # Protocol handlers (socks/http/tunnel/relay/...)
├── listener/   # Inbound listeners (tcp/udp/tun/tap/redirect/...)
├── limiter/    # Traffic/rate/conn limiters
├── registry/   # Registries for services/handlers/listeners/etc
├── service/    # Service wrappers + reporting hooks
├── socket/     # WebSocket reporter / panel integration
└── internal/   # Shared internals (grpc proto, net utils, sniffing, tls, ...)
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Management API routes/auth | `go-gost/x/api/api.go` | `/docs`, `/config/*`; BasicAuth + interceptor |
| Service config parsing | `go-gost/x/config/parsing/` | Converts config to running services |
| Add a handler | `go-gost/x/handler/` | Per-protocol subdirs |
| Add a listener/dialer | `go-gost/x/listener/`, `go-gost/x/dialer/` | Transport variants |
| Panel reporting | `go-gost/x/socket/` | WebSocket + HTTP report URL hooks |

## CONVENTIONS
- `go-gost/x/` is a standalone Go module (`go-gost/x/go.mod`); run go tooling from this dir when debugging module resolution.
- Generated gRPC/proto code lives under `go-gost/x/internal/util/grpc/proto/`.
- Handlers/listeners/dialers follow consistent pattern: `{type}.go` + `metadata.go` per protocol.
- OS-specific code uses `name_[os].go` suffix (e.g., `tun_linux.go`, `tun_darwin.go`).

## ANTI-PATTERNS
- Do not edit generated files in `go-gost/x/internal/util/grpc/proto/` (`*.pb.go`, `*_grpc.pb.go`).

## COMMANDS
```bash
cd go-gost/x
go test ./...
```
