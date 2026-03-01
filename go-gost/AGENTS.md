# Go-Gost Agent Instructions

## Build Commands

```bash
cd go-gost && go build .
cd go-gost && go run .
```

## Test Commands

```bash
# All tests
cd go-gost && go test ./...

# Single test by name
cd go-gost && go test . -run TestSpecificName -v

# Test specific package
cd go-gost && go test ./x/socket/...
```

## Lint Commands

```bash
cd go-gost && go vet ./...
```

## Code Style - Go

### Imports
Standard library first, external packages second, local packages third. Blank lines between groups.

### Error Handling
Return errors up the stack. Wrap with context using `fmt.Errorf`:
```go
if err := svc.Start(); err != nil {
    return fmt.Errorf("start service: %w", err)
}
```

### Config Files
- Panel integration: `config.json` (address, secret, ports)
- GOST services: `gost.json` or `gost.yaml` (via viper)

## Project Structure

```
go-gost/
├── main.go           # Entry; reads config.json, starts svc.Run
├── config.go         # Panel config loader
├── program.go        # GOST runtime: parse config, run/reload
├── go.mod            # replace github.com/go-gost/x => ./x
└── x/                # Local fork of github.com/go-gost/x
```

## Critical Conventions

### Module Replacement
`go.mod` uses `replace github.com/go-gost/x => ./x`. The `x/` directory is its own Go module.

### Panel Communication
- WebSocket for real-time commands
- HTTP for batch traffic reports
- AES encryption with node `secret` as PSK

### Config Reload
SIGHUP triggers config reload via `program.go`.

### CI Build
`CGO_ENABLED=0` for static binaries, UPX compression.

## Anti-Patterns

- DO NOT edit generated protobuf in `x/internal/util/grpc/proto/`
- DO NOT edit vendored dependencies in `x/`