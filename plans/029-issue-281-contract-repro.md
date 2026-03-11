# 029 - Issue 281 Contract Repro

## Goal
Add a contract test that reproduces issue #281: after changing a tunnel's entry node, forward runtime cleanup does not remove the stale service from the old entry node.

## Checklist
- [x] Review existing contract test helpers for mock node command recording.
- [x] Add a contract test that updates a tunnel entry node while a forward is bound to the tunnel.
- [x] Assert the new entry node receives forward sync commands and the old entry node does not receive forward cleanup, reproducing the bug.
- [x] Run the focused contract test and capture the failure.

## Test Record
- Command: `cd go-backend && go test ./tests/contract/... -run TestTunnelUpdateChangesEntryNodeButLeavesOldForwardRuntimeContract`
- Result: failed as expected with `expected old entry node to receive forward DeleteService cleanup for 1_2_281, got none`.
