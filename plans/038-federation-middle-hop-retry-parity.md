# 038 Federation Middle-Hop Retry Parity

## Checklist

- [x] Reproduce and document the parity gap between local tunnel middle-hop runtime generation and federation-applied middle roles.
- [x] Update federation runtime apply logic so remote middle-hop services set handler `retries` when the next hop has multiple candidates.
- [x] Add regression coverage for federated middle-hop runtime generation or contract behavior, including multi-target `fifo` scenarios.
- [x] Verify release / cleanup paths remain correct when the federated middle service carries retry settings.
- [x] Run targeted backend tests for handler and federation contract coverage.

## Findings

- Local tunnel runtime generation now sets `handler.retries` for middle-hop services based on downstream candidate count in `go-backend/internal/http/handler/mutations.go`, which enables router-level re-selection after a failed primary node.
- Federation runtime apply still creates remote middle-hop services without `handler.retries` in `go-backend/internal/http/handler/federation.go`, even though the remote chain hop itself uses the same selector failover settings (`strategy`, `maxFails=1`, `failTimeout=10m`).
- Because `go-gost/x/config/parsing/service/parse.go` only enables router retries when `cfg.Handler.Retries > 0`, federated middle-hop services can still fail hard on the first offline primary target instead of switching to backup.
- The gap creates inconsistent behavior: identical tunnel topologies can fail over correctly on local middle nodes but not on federated / remote middle nodes.

## Repair Direction

- In `go-backend/internal/http/handler/federation.go`, compute retry budget for `req.Role == "middle"` from `len(req.Targets)` and set `service["handler"]["retries"]` to at least `len(req.Targets) - 1` when there is more than one target.
- Keep retry injection scoped to federated middle roles only; exit roles should continue to omit retries because they do not rebuild downstream chain selection.
- Add regression coverage that proves federated middle runtime application preserves local parity, ideally by asserting the generated remote service config or by exercising a dual-panel contract path with multi-target middle nodes.
- Recheck federation release behavior to ensure added retry fields do not affect idempotent cleanup, service deletion, or re-apply flows.

## Validation

- `cd go-backend && go test ./internal/http/handler/... -count=1`
- `cd go-backend && go test ./tests/contract/... -count=1`
