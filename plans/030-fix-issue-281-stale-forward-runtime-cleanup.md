# 030 - Fix Issue 281 Stale Forward Runtime Cleanup

## Goal
When a tunnel's entry nodes change, remove forward runtime services from entry nodes that are no longer part of the tunnel before syncing the forward to its new entry nodes.

## Checklist
- [x] Review the tunnel update flow and identify where old/new entry node sets are available.
- [x] Add backend cleanup for forward runtimes on removed entry nodes.
- [x] Keep existing forward port rebuild and forward resync behavior intact.
- [x] Run focused contract regression tests for the issue 281 repro.

## Test Record
- Command: `cd go-backend && go test ./tests/contract/... -run 'TestTunnelUpdateChangesEntryNodeButLeavesOldForwardRuntimeContract|TestTunnelUpdateRecoversFromAddressInUseContract'`
- Result: passed.
