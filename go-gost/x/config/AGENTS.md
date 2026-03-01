# Config Agent Instructions

## Build Commands

```bash
cd go-gost/x && go build ./config/...
```

## Test Commands

```bash
cd go-gost/x && go test ./config/...
```

## Code Style - Go

### Config Access
```go
// Get global config
cfg := config.Global()

// Set config
config.Set(cfg)

// Register update callback
config.OnUpdate(func(cfg *config.Config) error {
    // Handle config change
    return nil
})
```

## Project Structure

```
config/
├── config.go         # Config structs + Global()/Set()/OnUpdate()
├── loader/
│   └── loader.go     # Parses config sections into registries
└── parsing/
    └── parse.go      # MDKey* constants, parser behavior
```

## Critical Conventions

### Default Config File
Named `gost` (e.g., `gost.json`), discovered via viper search paths:
- `/etc/gost/`
- `$HOME/.gost/`
- `.` (current directory)

### Runtime Mutations
Use `config.OnUpdate(...)` for thread-safe changes:
```go
config.OnUpdate(func(cfg *Config) error {
    cfg.Services = append(cfg.Services, newService)
    return nil
})
```

### Metadata Keys
Use `MDKey*` constants from `parsing/parse.go` for consistency.

## Anti-Patterns

- DO NOT mutate config directly without `OnUpdate`
- DO NOT edit generated protobuf files