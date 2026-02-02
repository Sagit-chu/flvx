# GO-GOST/X CONFIG KNOWLEDGE BASE

## OVERVIEW
Config model + parsing/loading pipeline for the `go-gost/x` runtime. This is the bridge between `gost.json`/`gost.yaml` and in-memory registries/services.

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Config structs + global state | `go-gost/x/config/config.go` | `Global()`, `Set()`, `OnUpdate()` |
| Default config file search | `go-gost/x/config/config.go` | Viper `SetConfigName("gost")` + paths `/etc/gost/`, `$HOME/.gost/`, `.` |
| Registry wiring | `go-gost/x/config/loader/loader.go` | Parses config sections and registers into registries |
| Metadata keys | `go-gost/x/config/parsing/parse.go` | `MDKey*` constants used by parsers |
| Config parser behavior | `go-gost/x/config/parsing/parser/parser.go` | CLI/env overrides; loads `gost.*` when empty |

## CONVENTIONS
- Default config file is named `gost` (e.g. `gost.json`) and is discovered via viper search paths.
- Runtime config mutations should go through `config.OnUpdate(...)` so changes are applied under the global mutex.

## COMMANDS
```bash
cd go-gost/x
go test ./...
```
