# 031 - Entry Transition Regression Coverage

## Goal
Expand issue #281 regression coverage to verify forward runtime cleanup and `forward_port` rebuilding across both single-entry to multi-entry and multi-entry to single-entry tunnel updates.

## Checklist
- [x] Review the current issue 281 contract repro and reuse its mock-node recording helpers.
- [x] Add a broader contract test that exercises both entry transition directions.
- [x] Assert removed entry nodes receive forward cleanup and retained/new entry nodes receive forward sync.
- [x] Run focused contract tests and record the result.

## Test Record
- Command: `cd go-backend && go test ./tests/contract/... -run 'TestTunnelUpdateChangesEntryNodeButLeavesOldForwardRuntimeContract|TestTunnelUpdateEntryTransitionsCleanupForwardRuntimeContract|TestTunnelUpdateRecoversFromAddressInUseContract'`
- Result: passed.
