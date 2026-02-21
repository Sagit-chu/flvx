# FLVX MCP Service

This directory hosts the standalone MCP service for full FLVX panel control.

- Design plan: `docs/PLAN.md`
- Planned entrypoint: `cmd/flvx-mcp/main.go`

## Phase 1 (current)

- MCP server skeleton based on `github.com/modelcontextprotocol/go-sdk`
- Transports: `stdio` and `http` (`/mcp`)
- Health endpoint: `/healthz` (configurable)
- Implemented read-only tools:
  - `node.list`
  - `user.list`

## Phase 2 (in progress)

- Added tools:
  - `auth.login`
  - `auth.user_package`
  - `tunnel.list`
  - `forward.list`
  - `forward.delete` (dangerous op)
  - `forward.pause` (dangerous op)
  - `forward.resume` (dangerous op)
  - `user.delete` (dangerous op)
  - `node.delete` (dangerous op)
  - `tunnel.delete` (dangerous op)
- Added dangerous operation guard:
  - `MCP_CONFIRM_TOKEN` (fail-closed when unset)
- Added idempotency protection for dangerous mutations:
  - `idempotency_key` is required on all dangerous tools
  - `MCP_IDEMPOTENCY_TTL_SECONDS` controls dedupe retention window
- Added audit logging:
  - `MCP_AUDIT_ENABLED` (default `true`)

## Run

```bash
go build ./...
```

```bash
# stdio mode (default)
MCP_TRANSPORT=stdio PANEL_BASE_URL=http://127.0.0.1:6365 MCP_CONFIRM_TOKEN=change-me go run ./cmd/flvx-mcp
```

```bash
# http mode
MCP_TRANSPORT=http MCP_HTTP_ADDR=:8088 PANEL_BASE_URL=http://127.0.0.1:6365 MCP_CONFIRM_TOKEN=change-me go run ./cmd/flvx-mcp
```
