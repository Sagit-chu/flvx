# Relay authentication and required-route security repair

Status: implementation, two-axis review and local integrated validation complete. Remote CI must pass before the authorized merge and release. Approved initial review base: 3549fe5; upstream rc7 is preserved.
Base inspected: 3549fe5 (existing checkout); existing investigation tests are preserved.
Workflow: installed Matt Pocock skills 1.3.2, diagnosing-bugs → TDD → code-review. The owner subsequently authorized PR submission, merge and the next release tag after review.

## Problems and evidence

1. Local and federated relay services and connectors are emitted without authentication. Native TCP, TLS, and KCP listener tests with both TCP and UDP destinations confirm unauthenticated forwarding. Existing authenticator support rejects absent/wrong credentials and permits correct credentials. TLS encryption alone does not authenticate clients.
2. A named chain missing at runtime produces a nil route, and an empty selected route also falls back to direct dialing. The loopback A–B–D → A–B–C–D test confirms that removing only B's chain permits A–B–target. A gated backend test confirms the live-service/missing-chain interval during redeployment overlaps topology editing. This is a demonstrated trigger, not proof of the reported production incident.
3. TLS client generation defaults to unverified server identity. KCP defaults are public, not unique deployment credentials. Authentication must not be advertised as protected against credential interception without addressing transport trust.

## Scope and safety

- Keep work in the existing checkout, preserve all local changes, and add no dependencies or infrastructure without asking.
- Do not mutate live production configuration or create production credentials. The owner subsequently authorized committing, pushing a PR, merging after checks, and publishing the next release tag through the existing workflow.
- Existing production topology and agent versions are unknown. All tests use temporary databases and loopback endpoints.
- Investigations characterize insecure behavior; permanent regression tests must assert desired secure behavior without deleting the historical evidence.

## Confirmed test seams

1. Panel HTTP and agent/federation command interfaces: generated authentication and strict-route policy, persistence, migration/reconnect, and serialized topology transitions. Use temporary repository storage and mock external node connections; do not test private helper implementation shapes.
2. Native GOST service/config and relay data interfaces: TCP/TLS/KCP carrying TCP/UDP, absent/wrong/correct credentials, required-route disappearance/empty selection, recovery, and legitimate exit direct dialing. Use local real sockets and public parsing/registry interfaces.

## Vertical implementation slices

1. Required-route invariant: explicit FLVX intermediate-route policy fails closed for missing or empty routes; ordinary direct exits remain valid. Do not globally redefine every GOST bypass/empty-route behavior. First write a failing behavioral test, make the minimal change, then verify recovery and transport variants.
2. Runtime lifecycle: serialize operations for the same tunnel; replace chains without delete/recreate gaps when possible. Avoid stale redeploy snapshots overwriting newer edits. Check create/update/redeploy/delete and failover owner paths. Do not hold a database transaction while awaiting node acknowledgements.
3. Authentication model and persistence: define a single runtime-auth contract usable by local and federation generation. Credentials must be purpose-separated from node management secrets, nonempty, stable across normal resync, absent from public list responses/logs, and fail safely on malformed or unavailable state.
4. Compatibility/migration: verify the exact old/new capability contract. Where staged migration is needed, teach all authorized upstream connectors before enforcing receiver auth, and expose incomplete migration rather than silently claim protection. Do not silently fall back to anonymous mode on an error. Validate restart, reconnect, edits, sharing and rollback.
5. Transport trust: use verified encryption for reusable credentials; evaluate how existing raw TCP/KCP configurations can migrate without pretending public defaults are authentication. Any unresolvable product decision or component addition must be raised before implementation.
6. Final validation and two-axis review: focused red/green evidence, race tests, relevant full module suites/build/vet, standards review and spec coverage review. Document blocked PostgreSQL/production checks separately.

## Candidate decisions requiring research

- Best credential storage granularity: tunnel listener/binding vs per-node identity; federation must carry the correct receiving identity without exposing management keys.
- Existing schema/API compatibility and capability reporting; no assumption that old agent/remote panel versions accept new required fields.
- Opt-in strict-route metadata protects FLVX-specific invariants while preserving generic GOST use; missing policy support on old agents must be visible.
- Safest no-new-component authenticated transport plan and handling of legacy unencrypted TCP.

## Completion conditions

- An unauthenticated relay client cannot use newly secured local/federated endpoints; wrong credentials are rejected and authorized forwarding works.
- Missing a required chain cannot connect directly to the ultimate target; restored chains resume the intended path.
- Editing and redeploying the same tunnel cannot leave a stale or partially rebuilt runtime presented as successful.
- No production-side actions or new external dependencies are introduced. Limitations and remaining migration steps are explicit.

## Authentication implementation decision (confirmed test seams)

The owner confirmed both seams. New relay tunnels require endpoint-scoped relay
credentials; existing rows remain explicitly `legacy` in the tunnel list API.
No reconnect automatically migrates an existing tunnel. This is an auth-only
repair against anonymous relay abuse, not protection against interception: TCP,
unverified relay TLS, public-default KCP and HTTP/WS control-plane paths retain
transport confidentiality limitations. We do not silently change those protocols.

Persist independent credentials per tunnel/receiving node/role, never reuse
management/share tokens, and fail on RNG failure. Credentials survive reconnect,
best-exit ordering and edits. Missing required credentials fail closed. New
federation provisioning requires an explicit auth-version capability and apply
acknowledgment; authenticated runtimes reject legacy reapply. Credentials are
excluded from ordinary public DTOs. Backups without the security state must not
be represented as protected after restoration; migration/credential backup and
rotation require a later explicit maintenance workflow.

Legacy live migration is not automatic in this patch. A future explicit migration
must coordinate all upstreams and receivers in a maintenance cutover, persist
progress, and never report protection before all acknowledgments. Existing legacy
tunnels remain unprotected until that work is completed. No production credentials
or configuration are generated by this local development task.

## Validation and release boundaries

Implemented required-route behavior is opt-in handler metadata `chain.strict`.
The panel emits it for tunnel entry forwards and relay intermediates; direct
exits do not set it. A new agent build is required: existing binaries can ignore
unknown metadata. A panel command acknowledgment alone is not proof that an
unknown older binary enforces this policy.

The runtime lock serializes operations within one panel process; it is not a
distributed transaction or a cross-panel lock. Updating desired topology and
applying it to multiple nodes still permits partial failure. Errors must remain
visible, and a later explicit resync may be needed. No claim is made that old
connections switch route atomically during an edit.

Before a production rollout, back up the database, deploy matching agent builds,
test a small explicitly selected tunnel, then deploy the panel using a planned
maintenance window. The local implementation task does not perform these steps.
Existing legacy tunnels require a separately designed, explicitly initiated
credential migration. Required auth is a policy, not proof of every live
receiver's current enforcement.

Validation completed so far:
- Public parsed-service missing/empty/no-chain rejection and restored A–B–C–D
  forwarding: race repeated passes.
- Native TCP/TLS/KCP carrying TCP/UDP: required missing route rejects and
  authenticated direct exit works; race repeated three times.
- Panel forward-create commands emit strict policy only for type-2 tunnel
  forwards, preserving ordinary direct forwards; race repeated three times.
- go-gost/x: complete `go test ./...` and `go vet ./...` passed.
- go-gost: complete `go test ./...` passed, including agent lifecycle tests.

PostgreSQL behavior and deployment compatibility need separate verification;
no production endpoint was used. The test TLS certificate is verified but is
not evidence that generated production TLS configurations validate peers.

### Final identity and recovery details

The stable credential key is tunnel + receiving node + role, NOT hop index:
inserting a hop ahead of an existing receiver must preserve its password. A
direct tunnel converted to a relay tunnel is a new protected relay deployment;
converting back does not erase required mode or credentials. Existing relay
legacy rows are unchanged and listed as `relayAuthMode: legacy`. Required means
configuration policy, not proof of deployment or old-agent capability.

Both consumer policy and provider authentication version are persisted separately
from private credentials. Missing credentials fail closed on update/reconnect;
ordinary reapply cannot rotate or remove established authentication. Config
exports retain the required policy but omit credentials. Config imports of
protected tunnels (or over existing protected rows) are explicitly rejected with
an explanation to use full-database recovery. Secure configuration-only credential
export/import and rotation are not implemented. Full database backups contain
private credentials and must be protected as secrets.

The federation provider advertises auth version 1 on connect/reserve and
acknowledges it on apply. Consumers preflight before reservation and before
tearing down update resources. This is panel capability negotiation; unknown
old local agents are not remotely attested by this patch. The new strict-route
metadata requires the patched agent, and an old agent can ignore it. Deployment
must upgrade the participating agents before claiming the required-route fix.

Log audit: normal agent service/node parsing logs metadata and relay usernames,
not the `auth` password fields. The backend's GORM logger is Silent. Authenticated
federation apply errors suppress peer-supplied response text to avoid reflected
credential disclosure. Private provider incoming credentials and downstream
Target JSON are excluded from whole-model JSON serialization. Authentication
still resides in desired-state database/agent configuration; this is not an
at-rest encryption feature.

Downgrading the panel to an older build is not a safe rollback for protected
tunnels: older generation code can omit authentication and overwrite receiver
configuration. Preserve the full security-aware database and deploy compatible
panel/agent versions together. Any rollback or old-tunnel migration needs an
explicit maintenance plan and validation of the live receiver, not only API
policy fields.

## Final lifecycle guarantees and limits

Creation reserves prepared topology and local ports before waiting for remote
acknowledgments. Create/update report saved-but-pending outcomes explicitly,
including a frontend warning. Obsolete runtime ownership persists until cleanup
is acknowledged, even after desired topology or tunnel deletion. Unconfirmed
receiver ports remain reserved. Re-added resources and middle-to-exit role
changes cannot lose a required chain before receiver replacement acknowledgment.

This still does not provide distributed atomic topology cutover. During pending
application, old paths or established connections can remain. Upgrade agents,
resynchronize their configurations, and verify required-route policy is active
before claiming the deployed protection.
