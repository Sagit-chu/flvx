# Registry Agent Instructions

## Build Commands

```bash
cd go-gost/x && go build ./registry/...
```

## Test Commands

```bash
cd go-gost/x && go test ./registry/...
```

## Code Style - Go

### Registration Pattern
```go
// Register a new component
registry.RegisterHandler("myhandler", NewMyHandler)
registry.RegisterListener("mylistener", NewMyListener)
registry.RegisterDialer("mydialer", NewMyDialer)
```

### Lookup Pattern
```go
// Get a registered component
creator := registry.GetHandler("http")
if creator == nil {
    return errors.New("handler not found")
}
```

## Project Structure

```
registry/
├── handler.go    # RegisterHandler, GetHandler
├── listener.go   # RegisterListener, GetListener
├── dialer.go     # RegisterDialer, GetDialer
├── connector.go  # RegisterConnector, GetConnector
├── auth.go       # Auther registry
├── bypass.go     # Bypass registry
├── admission.go  # Admission registry
└── ...
```

## Critical Conventions

### Thread Safety
Registries use thread-safe maps for storage.

### Naming
Names are case-sensitive (usually lowercase).

### Initialization Order
Components must be registered before config parser runs (via `import _ "..."` in `main.go`).

## Anti-Patterns

- DO NOT register components after config parsing