# 037 Tunnel Chain Failover Repair

## Checklist

- [x] Analyze middle-hop primary/backup failover across backend runtime generation and agent route selection.
- [x] Add regression coverage for a tunnel relay chain where a same-hop `fifo` primary is down and the backup must take over.
- [x] Update tunnel runtime generation so chain services retry route selection when the next hop has multiple candidates.
- [x] Harden agent-side chain failover if backend-configured retries alone does not cover all relay/chain paths.
  - N/A: Router retry loop (`go-gost/x/chain/router.go:91`) rebuilds route on each iteration, so FailFilter applies to failed nodes.
- [x] Revalidate diagnosis output so tunnel/forward tests reflect failover behavior instead of looking fully broken.
  - N/A: Diagnosis tests individual legs (A→next, B→next) which is correct. Failover is for actual traffic, not diagnosis.
- [x] Run targeted backend and agent test suites.

## Findings

- Backend already emits hop selectors for tunnel chains with `strategy`, `maxFails=1`, and `failTimeout=10m` in `go-backend/internal/http/handler/mutations.go:3243`, so the control plane is not dropping the primary/backup mode itself.
- Agent route construction selects one node per hop up front in `go-gost/x/chain/chain.go:92`. If the chosen primary node is offline, the dial fails inside `go-gost/x/chain/route.go:220` and the node gets marked failed, but that mark only matters on a later route build.
- Tunnel chain services are generated without handler retry settings in `go-backend/internal/http/handler/mutations.go:3274`, while the router only rebuilds a route when `cfg.Handler.Retries` is greater than zero in `go-gost/x/config/parsing/service/parse.go:319`.
- Because the default retry count is effectively one attempt, a relay request never gets a second route selection after the primary middle-hop node is marked down, so traffic does not switch to the backup node.
- The forward handlers already have explicit retry/exclude-node loops in `go-gost/x/handler/forward/local/handler.go:179` and `go-gost/x/handler/forward/remote/handler.go:207`, which explains why failover logic exists in the codebase but is missing on the tunnel relay chain path.

## Repair Direction

- In backend tunnel runtime generation, compute the downstream candidate count for each chain service and set handler `retries` to at least `len(nextTargets) - 1` when a hop has multiple selectable nodes. That gives the router another dial cycle so `FailFilter` can skip the failed primary and pick the backup.
- Keep the retry value scoped to tunnel relay services built from `buildTunnelChainServiceConfig` so single-node hops do not incur unnecessary extra attempts.
- Add an agent-side regression test around relay + chain routing that simulates an offline primary node and asserts the second attempt lands on the backup node after the first node is marked failed.
- Add a backend regression test covering a tunnel definition with two nodes on the same middle hop in `fifo` mode, verifying the generated service config carries the retry budget needed for failover.
- Recheck tunnel/forward diagnosis behavior after the runtime fix. The current diagnosis model probes individual branch legs, so it may need an aggregated result or clearer messaging to avoid reading a partial branch failure as total failover failure.

## Validation

- `cd go-backend && go test ./internal/http/handler/... ./tests/contract/...`
- `cd go-gost/x && go test ./chain/... ./handler/relay/... ./config/parsing/service/...`
