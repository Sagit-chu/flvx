# GO-GOST SERVICE KNOWLEDGE BASE

**Generated:** Sun Feb 15 2026

## OVERVIEW
Forwarding agent built on GOST v3 with a local fork of `github.com/go-gost/x` under `x/`.
**Stack:** Go 1.23, github.com/go-gost/core v0.3.1, local `go-gost/x` module.

## STRUCTURE
```
go-gost/
├── main.go           # Entry; reads panel config.json; starts svc.Run(program)
├── config.go         # Panel config.json loader (addr/secret + ports)
├── program.go        # GOST runtime: parse config, run/reload services
├── x/                # Local fork of github.com/go-gost/x (has its own go.mod)
└── go.mod            # replace github.com/go-gost/x => ./x
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Panel integration config | `go-gost/config.go` | Expects `config.json` in cwd by default |
| Service lifecycle/reload | `go-gost/program.go` | Parses config; handles SIGHUP reload |
| WebSocket reporting | `go-gost/main.go` | Starts reporter + sets HTTP report URL |
| Protocol behaviors | `go-gost/x/` | Handlers/listeners/dialers live here |

## CONVENTIONS
- Two configs exist: panel integration uses `config.json`; forwarding services use GOST config (defaults to `gost.{json,yaml}` via viper search paths).
- `go-gost/x/` is the primary extension surface; avoid editing vendored deps.
- Agent communicates with panel via WebSocket (real-time commands) + HTTP (batch traffic reports).
- All panel communication uses AES encryption with node `secret` as PSK.

## ANTI-PATTERNS
- **DO NOT EDIT** generated protobuf in `x/internal/util/grpc/proto/`.

## COMMANDS
```bash
cd go-gost
go run .
go test ./...
go build .
```
