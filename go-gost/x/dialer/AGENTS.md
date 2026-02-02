# GO-GOST/X DIALERS KNOWLEDGE BASE

## OVERVIEW
Outbound dialers (client-side connection establishment) used by connectors/handlers.

## STRUCTURE
```
go-gost/x/dialer/
├── direct/   # Baseline dialer
├── tcp/
├── udp/
├── tls/
├── ws/
├── quic/
├── http2/
├── http3/
├── ssh/
├── wg/       # WireGuard dialer
└── ...
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Pick a dialer | `go-gost/x/dialer/` | One subdir per transport |
| TCP baseline | `go-gost/x/dialer/tcp/dialer.go` | Reference implementation |

## CONVENTIONS
- Dialer implementations typically live in `dialer.go` with a paired `metadata.go` (e.g. `go-gost/x/dialer/tcp/`).

## COMMANDS
```bash
cd go-gost/x
go test ./...
```
