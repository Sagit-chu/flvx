# Tunnel lifecycle repair validation

Local darwin/arm64 checkout, baseline 3549fe5; 2026-11-08 UTC. No deployment, push, commit, dependency, or real node used.

## Agreed seams

Panel HTTP requests, temporary repository storage, and mock external agent/federation command interfaces. Tests observe runtime command payloads and API results. No private lifecycle helper tests.

## Red → green evidence

1. `go test ./tests/contract -run '^TestTunnelRedeployPreservesLiveRoutes$' -count=1`
   - Red: `unexpected chain deletion on A: {"chain":"chains_1"}`.
   - Green after replacing cleanup + create with existing chain/service upsert: 1.620s.
2. `go test ./tests/contract -run '^TestTunnelEditWaitsForRedeploy$' -count=1`
   - Red: `edit changed runtime before redeploy finished` while the redeploy's exit service command was gated.
   - Green after per-tunnel locking over snapshot, persistence and runtime application. Combined first two tests: 2.879s.
3. `go test ./tests/contract -run '^TestTunnelFailedRedeployPreservesLiveRoutes$' -count=1`
   - Red: `failed redeploy removed existing runtime: DeleteService on B` on an injected bind failure.
   - Green after removing destructive cleanup retry for upsert. Existing add/create retry behavior remains unchanged. Combined local tests: 3.922s.
4. `go test ./tests/contract -run '^TestFederationRedeployPreservesExistingBindings$' -count=1`
   - Red: `redeploy destroyed existing federation runtime: [DeleteService DeleteService DeleteChains]`.
   - Green: 0.653s after reusing stable resource keys and excluding existing bindings from failure cleanup.

Additional verification covers failed federation apply, concurrent delete waiting for redeploy, and final middle-hop target after concurrent edit. Full six-regression race invocation:

`go test -race ./tests/contract -run 'Test(Tunnel(RedeployPreservesLiveRoutes|FailedRedeployPreservesLiveRoutes|EditWaitsForRedeploy|DeleteWaitsForRedeploy)|Federation(Failed)?RedeployPreservesExistingBindings)' -count=3`

Result: PASS, 32.626s. The gated concurrency observation lasts 150ms; successful completion and the final B→C target are also asserted.

## Scope and limitations

- One process's Handler owns the locks. Different tunnels remain independent; migration locks source and target in sorted order. Update, redeploy/reconnect (through the shared redeploy path), ordinary/batch delete, and delete-with-forward migration participate. Best-exit probes remain unlocked while measuring, then revalidate owner/exit topology under the lifecycle lock before applying.
- Existing PR #550 agent mutation queue and service rollback were not modified. Panel upserts defer failure rollback to the agent rather than deleting a preserved listener.
- This is not a distributed multi-node transaction. Existing tunnel edits still commit desired state before node application; partial apply can leave some nodes ahead of others, and full atomic rollback was not introduced. Offline/deferred application behavior is inherited and is not proof of complete deployment.
- Forward CRUD and provider-side federation ownership are not newly serialized by this per-consumer-tunnel lock.
- Original bug-characterization source is preserved verbatim at `070-tunnel-insert-middle-hop-investigation.go.txt`; only its old-bug gated test is skipped in the active test file because waiting for the removed insecure event is no longer a passing regression.

## Release review follow-up (2026-11-08 UTC)

Review findings were repaired before publication:

- `TestTunnelDeleteWaitsForCreateRuntime`: red `delete removed runtime while create was still applying`; green after claiming the new ID's lifecycle lock before its first commit. Claim is nonblocking so it never waits for a lifecycle owner while holding a database transaction.
- `TestFederationCreateDoesNotHoldDatabaseDuringAcknowledgment`: red `federation create held database while waiting for acknowledgment`; green after splitting local desired-state persistence and final provider-binding persistence into short transactions around network work. Initial topology and allocated local ports are persisted with credentials before waiting for the provider; unallocated remote ports are NULL.
- `TestConcurrentCreatesReserveLocalPortsBeforeFederationAcknowledgment`: mixed local/remote topology with one available local receiver port. The second tunnel is rejected while the first awaits provider acknowledgment. Mutation proof disabling only initial prepared-chain persistence produced red `second create did not reject the reserved local port before provider acknowledgment`; restoring persistence produced green.
- `TestOfflineTunnelCreateReportsPendingRuntime`: red response was only `code:0,msg:操作成功`. Create/update now retain code-0 accepted-configuration compatibility but include explicit pending message and `runtimeStatus:pending`; batch redeploy reports pending in its failure count. Desired configuration/authentication and previously applied runtime are retained.
- `TestPendingTunnelUpdateStillRemovesObsoleteRelay`: red `pending update left removed relay live: map[]`; pending results now continue obsolete cleanup and required forward reconciliation before responding.
- `TestFailedObsoleteRelayRemovalPreservesItsChain`: red deleted both listener and chain despite rejected listener deletion. Cleanup now removes service before chain, preserves the chain on unacknowledged listener deletion, and reports cleanup errors in the pending outcome.

The original statement that offline application is silently inherited no longer applies to create/update/redeploy outcomes. This remains an asynchronous, multi-node workflow rather than a distributed transaction. Failed obsolete cleanup is reported; full durable cross-node rollback was not introduced.

After these changes, handler and contract suites passed in 11.951s and 16.189s respectively (prior to the final isolated cleanup-error regression/hardening, which also passed focused validation).

### Durable obsolete-runtime cleanup

A later review identified that a pending message alone loses obsolete endpoint ownership once topology rows are replaced. The repair adds `tunnel_runtime_cleanup` in the existing database (no external component). Each row contains tunnel ID, node ID, runtime kind and old receiver port. Topology updates queue ownership in the same transaction that replaces topology; tunnel deletion queues current runtime before deleting its desired rows. Cleanup records are not cascaded with the tunnel or authentication rows.

Reconciliation uses existing maintenance, reconnect, and redeploy paths under the tunnel lifecycle lock. Service deletion must be acknowledged before its chain is removed. Successful removal clears only acknowledged records. Re-desired roles cancel obsolete deletion safely; changed receiver ports remain reserved until an acknowledged service replacement. Multiple unresolved ports can coexist for the same named receiver. Automatic allocation, explicit relay/forward port validation, and federation port discovery include these reservations.

Public-seam regressions:
- Extended failed-cleanup test first failed `redeploy lost obsolete cleanup ownership after failure: map[]`; now a subsequent successful redeploy removes both obsolete resources and a third pass sends no repeated deletion.
- `TestRedesiredRelayKeepsOldPortReservedUntilReplacementAcknowledged` re-adds a removed node on a different receiver port with a simulated timeout, rejects another tunnel claiming the unresolved old port, then permits that port only after a successful replacement ACK. No delayed deletion reaches the re-desired listener.
- `TestDeletedTunnelCleanupSurvivesHandlerReplacement` deletes desired state despite a rejected listener deletion, verifies old-port reservation, then replaces the Handler and reconnects node sessions. Persisted ownership permits cleanup/retry and releases the port only after ACK. Subsequent retry does not remove the replacement tunnel.

The full repository, handler and contract suites passed (2.965s, 12.463s, 16.974s) before the final reconnect-specific test and forward-port reservation check. Focused race validation results are reported with the integration checkpoint.

### Receiver role-transition acknowledgment barrier

Final review on rebased baseline `7f01774` found that middle→exit on the same node could remove the old chain after a rejected/timed-out receiver replacement. `TestMiddleToExitKeepsChainUntilReceiverReplacementAcknowledged` reproduced this in all four combinations of same/changed receiver port and timeout/hard rejection (`deletes=1 unsafe=true`). The fix queues a service-replacement barrier on receiver role changes, including same-port changes. A still-desired service record is retained and protects chain cleanup until successful service upsert acknowledgment; redeploy drains obsolete chains after that acknowledgment. Focused green result: PASS, 0.764s. Red evidence: `/tmp/flvx-role-transition-red.log`; race validation is reported in the final integration checkpoint.
