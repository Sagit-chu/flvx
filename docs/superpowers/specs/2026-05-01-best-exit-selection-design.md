# Best Exit Selection Design

## Goal

Add a multi-exit tunnel strategy named `best` that always sends new connections through the currently best-quality exit. The feature should prevent traffic from continuing to use an exit whose latency or packet loss has degraded while the exit is still technically online.

Existing connections must not be interrupted. Switching affects only new connections created after the runtime chain update is applied.

## Current Context

- Tunnel forwarding stores entry, chain, and exit nodes in `chain_tunnel`.
- Multi-exit runtime chains are currently rendered as one GOST hop with multiple nodes.
- GOST selectors support `fifo`, `round`, `rand`, and `hash`, plus fail filtering through `maxFails` and `failTimeout`.
- The current fail filter only reacts to dial, handshake, or transport failures. It does not react to high latency when the exit is still reachable.
- `tunnel_quality_prober` already runs panel-side TCP probes and stores tunnel quality history, but it currently probes representative nodes and does not drive runtime routing decisions.

## User Decisions

- Add a `best` option for multi-exit tunnels.
- `best` means always choose the current best exit for new connections.
- Score exits by end-to-end quality.
- Keep the existing public probe target: `www.bing.com:443`.
- Do not disrupt established connections.

## Approach

Implement `best` as a panel-driven control-plane strategy.

The database stores the user's intended strategy as `best`. When the panel renders runtime GOST config for a `best` exit group, it sends a GOST selector strategy of `fifo`. The panel dynamically sorts the candidate exits so the current best exit is first. GOST then chooses the first node for new connections.

This avoids adding active probing logic inside every GOST agent and reuses the existing panel-to-agent command path.

## Components

### Frontend

The tunnel form adds `最优` to the multi-exit load strategy selector.

- Label: `最优`
- Value: `best`
- Scope: tunnel forwarding exit groups, alongside `主备/fifo`, `轮询/round`, and `随机/rand`
- Create and edit forms must submit and restore `best` unchanged.

### Backend Data Model

No schema change is required.

The existing `chain_tunnel.strategy` column stores `best`. Repository and handler paths should preserve the value in API responses and updates.

### Runtime Chain Rendering

When building runtime chain config:

- If the configured strategy is not `best`, keep existing behavior.
- If the configured strategy is `best`, emit GOST selector strategy `fifo`.
- Sort the target nodes using the panel's latest best-exit decision before rendering the node list.
- If no quality decision exists yet, keep the saved node order.

This preserves the user's `best` intent in storage while using a GOST selector that can execute the panel's sorted decision.

### Quality Prober

Extend `tunnel_quality_prober` to evaluate all candidates in `best` exit groups.

For each chain owner node and candidate exit, measure:

- Chain owner node to candidate exit using TCP ping.
- Candidate exit to `www.bing.com:443` using TCP ping.

For direct entry-to-exit tunnels, each entry node owns its own chain decision. For tunnels with intermediate chain hops, each node in the last hop group before the exits owns its own chain decision. This allows different entry or chain nodes to choose different best exits when their path quality differs.

### Scoring

Each exit candidate gets an end-to-end score for a specific chain owner node.

- Total latency is the sum of owner-to-exit latency and exit-to-Bing latency.
- Total loss combines both legs by success probability: `1 - (1 - lossA) * (1 - lossB)`.
- Failed or unreachable candidates are sorted behind successful candidates.
- The score should heavily penalize packet loss so that low-latency but lossy exits are not selected over stable exits.

A practical scoring formula can be:

```text
score = totalLatencyMs + (totalLossPercent * lossPenaltyMsPerPercent)
```

Use `lossPenaltyMsPerPercent = 100` initially. For example, 5% loss adds 500ms to the score.

### Switching Rules

The panel should not update chains on every probe round.

Switch only when all conditions are true:

- The candidate best exit is different from the currently applied first exit.
- The candidate is successful.
- The candidate remains best for consecutive probe rounds.
- The candidate beats the current exit by a minimum advantage threshold.
- The chain owner node has passed a minimum switch cooldown.

Initial constants:

- Consecutive confirmations: 3 rounds.
- Switch cooldown: 30 seconds per chain owner node.
- Minimum advantage: the candidate score must improve by at least `max(20ms, currentScore * 0.15)`.

If all exits fail, keep the current runtime order and do not issue a destructive update.

### Runtime Update

When a `best` chain owner node changes best exit:

1. Rebuild that node's `chains_<tunnelID>` payload with the best exit first and remaining candidates sorted by quality for that node.
2. Send `UpdateChains` to that chain owner node.
3. Do not restart or update tunnel services.
4. Record success or failure in logs and in the in-memory decision state.

This affects only future connections. Existing TCP connections keep using the `net.Conn` created before the update and continue through their original exit.

### Agent Safety Improvement

The current agent `UpdateChains` path unregisters the old chain before registering the new chain. This does not kill existing connections, but it creates a small window where a new connection can fail because the chain name is temporarily absent.

Improve the update path so it parses the new chain first and only replaces the registered chain after parsing succeeds. The replacement window should be as small as possible. If parsing fails, the old chain must remain active.

## Error Handling

- If probing one candidate fails, continue scoring other candidates.
- If a chain owner node is offline or times out, skip decisions for that owner during the round instead of marking every candidate failed.
- If a candidate has no successful required probe data, mark it failed for that round.
- If `UpdateChains` fails, keep the current applied order and retry on a later round.
- If the tunnel has one exit or an incomplete config, `best` behaves like the saved order and does not trigger dynamic switching.
- If `monitor_tunnel_quality_enabled=false`, dynamic `best` switching pauses. The last applied runtime order remains in effect.

## Observability

The prober should maintain in-memory decision state per `best` tunnel and chain owner node.

Useful fields:

- Tunnel ID and chain owner node ID.
- Current applied best exit node ID.
- Candidate best exit node ID.
- Candidate scores.
- Last switch timestamp.
- Last switch result.
- Reason for not switching, such as cooldown, insufficient advantage, candidate unstable, or all exits failed.

Initial UI scope is limited to supporting create, update, and display of the `best` strategy. A later enhancement can expose current best exit and candidate scores in the tunnel monitor view.

## Testing

Backend tests:

- Score calculation orders candidates by latency and packet loss.
- Packet loss penalty prevents lossy exits from winning only because latency is low.
- All-failed candidates do not trigger a switch.
- Consecutive confirmation and cooldown prevent flapping.
- `strategy=best` persists in `chain_tunnel.strategy` and is returned by tunnel list/get APIs.
- Runtime rendering maps `best` to GOST `fifo` and places the chosen best exit first.

Agent tests:

- `UpdateChains` parse failure keeps the old chain registered.
- Successful `UpdateChains` updates the chain used by new connections.

Frontend verification:

- Tunnel form includes `最优` in the exit strategy selector.
- Existing tunnels with `strategy=best` render correctly.
- Create and update requests submit `best` unchanged.

Verification commands:

```bash
(cd go-backend && go test ./...)
(cd go-gost && go test ./...)
(cd vite-frontend && pnpm run build)
```

## Non-Goals

- Do not move existing live connections to a new exit.
- Do not add per-tunnel custom probe targets in this phase.
- Do not implement active best-exit probing inside GOST agents.
- Do not add a detailed best-exit UI dashboard in this phase.
