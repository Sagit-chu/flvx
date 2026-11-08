# Two-axis review

Fixed base: 3549fe5ad8a6e9b863b3e4f7b9bf738facff87ee plus all task working-tree
changes, including new files. The owner approved this scope and subsequently
authorized PR submission, merge and the next release tag. Initial review was
performed as independent Standards and Spec passes.

## Standards

No new hard violations found against applicable AGENTS.md rules. Repository
access, explicit table names, migrations, response envelopes and agent extension
boundaries comply.

Two non-blocking judgement calls:
1. Possible duplicated code: required_relay_transport_test.go repeats the native
   TCP/TLS/KCP fixture in relay_auth_matrix_investigation_test.go. The historical
   investigation fixture is deliberately retained separately in this patch, to
   preserve the original evidence; helper extraction is deferred.
2. Possible speculative generality: buildFederationServiceConfig accepted a
   variadic credential argument but only used its first item. Fixed by accepting
   one nullable credential pointer, updating legacy callers explicitly. Focused
   generator and federation authentication tests pass.

Summary: zero hard violations; two low-priority smells, one corrected and one
explicitly deferred.

## Spec

Three partial requirements found:
1. Create did not take the lifecycle lock, committing a visible tunnel before
   runtime application. Concurrent delete/edit/reconnect could interleave with
   stale creation. This violated the create/update/redeploy/delete scope.
2. Federated create held an open database transaction across provider/node
   acknowledgments, contrary to the no-network-wait-in-transaction requirement.
3. Offline/timeouts and forward synchronization failures could be swallowed as
   success, contrary to the requirement to expose partial application.

These were inherited behaviors within the explicitly promised repair scope,
not regressions introduced by the patch. No additional confirmed auth or
strict-route defect, and no unjustified scope creep found. Explicit legacy,
transport, old-agent and distributed-rollback limits remain documented.

Summary: three lifecycle completion gaps, all assigned for correction before
publication. Follow-up verification will be appended after the fixes.

## Review-driven recovery additions

Follow-up inspection found two additional lifecycle issues during the transaction
and pending-state changes: local port reservations must be published before the
remote acknowledgment wait, and pending updates must still clean obsolete
runtime and reconcile forwards. Both have focused regressions.

A cleanup failure must not discard the removed runtime's ownership. Persistent
cleanup records are being added in the existing database so retries can identify
old endpoints even after desired topology changes. Records are removed only after
acknowledgment and are canceled if current desired topology needs the resource
again. This is targeted cleanup recovery, not distributed transaction rollback.

## Final follow-up

Final code commit: 89a1a7b, integrated on rc7 (46bfb86).
All reported correctness/completion gaps were corrected. Durable cleanup keeps
port reservations, survives topology/tunnel deletion, and is retried through
existing maintenance and reconnect paths. Re-adding a receiver cannot cause a
delayed deletion, and changing its port does not free the old port before ACK.
The additional middle-to-exit role transition preserves the old chain until
receiver replacement is acknowledged, for both same and changed ports.

Standards: no hard violations remain; one fixture-duplication smell is deferred.
Spec: no remaining release blocker found in the reviewed scope.
The final full backend race run passed (excluding the documented external
Cloudflare test). Frontend build/typecheck, GOST x race/vet, agent tests/build/vet
and backend build/vet passed. Publication remains conditional on remote CI.
