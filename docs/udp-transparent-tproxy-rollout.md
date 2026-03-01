# UDP Transparent TPROXY Rollout and Operations

This document covers gradual rollout (P7), rollback, and operations guidance for `udpMode=transparent`.

## Rollout Plan

### Stage 1: Single Node Canary

- Select one Linux node and one low-risk forward
- Enable `udpMode=transparent` on fixed port
- Observe for 24 hours:
  - Forward diagnose `tproxy` payload status
  - Agent logs for ensure/delete/rollback actions
  - Packet loss and latency trends

Success gate:

- No loop guard failures
- No repeated policy apply failures
- Target sees client source tuple as expected

### Stage 2: Small Traffic Expansion (5%-10%)

- Expand to a limited set of users/tunnels
- Keep same mark/table defaults (`11` / `111`)
- Run failure drills at least once during window

Success gate:

- Rule state remains consistent after node restarts
- No uncontrolled growth of policy entries

### Stage 3: Full Rollout

- Enable for remaining approved forwards
- Keep dashboard and diagnosis checks as standard SOP

## Rollback Playbook

### Single Forward Rollback

1. Update forward `udpMode` from `transparent` to `normal`
2. Redeploy forward
3. Verify `DeleteTProxyPolicy` has been executed
4. Confirm no matching port rule remains in `FLVX_TPROXY`

### Node-Level Emergency Rollback

1. Pause affected forwards
2. Delete related `FLVX_TPROXY` port rules
3. Remove mark rule/table route if needed
4. Resume forwards in `normal` mode

## Runtime Diagnostics SOP

For each transparent forward issue:

1. Call `/api/v1/forward/diagnose`
2. Inspect `data.tproxy.nodes[*]`:
   - `supported`
   - `capabilityError`
   - `state` summary
3. Run Linux validation script phase aligned to symptom:
   - state/rules: `--phase p6-2`
   - source tuple: `--phase p6-3`
   - failure drill: `--phase p6-4`

## FAQ

### Why does transparent mode fail on non-Linux?

Transparent UDP mode requires Linux kernel TPROXY primitives. Non-Linux returns unsupported by design.

### Why can transparent mode be rejected during create/update?

The system blocks loop-prone targets that point to local node listen address and same port.

### What if node restarts?

Agent restores policy entries from `/etc/flux_agent/tproxy_policy_state.json` at startup.
