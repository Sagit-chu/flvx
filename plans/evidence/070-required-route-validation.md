# Required-route repair validation

Local darwin/arm64 checkout, baseline 3549fe5; 2026-11-08 UTC.

## Red / green
- New parsed-service regression failed before implementation with:
  "required chain is missing, but relay accepted a direct connection".
- Added opt-in handler metadata chain.strict and a router required-chain guard.
  The same test passed with race detection repeated three times.
- New panel command contract failed before generation change:
  "tunnel forward must require chain, got map[chain:chains_1 type:tcp]".
- Emitting strict metadata for type-2 forwards fixed it; type-1 direct forwards
  remain unrestricted. Race detection repeated three times passed.

## Native data evidence
- Real loopback A–B–D, then A–B–C–D, refuses traffic while B's chain is missing;
  C/D counters remain unchanged on the rejected request; restoring B's chain
  restores the complete path.
- Parsed services reject missing chain names, empty candidate selection and an
  absent chain configuration with strict policy; TCP and UDP requests covered.
- Authenticated native TCP/TLS/KCP relay transports carrying TCP/UDP targets
  reject a required missing route and continue to support legitimate direct exit
  services. TLS certificates are verified in the fixture.
- All TestRequiredRelay tests: race detection, count=3 passed, 1.836s.
- All go-gost/x tests: ordinary and race passed; go vet passed.
- Agent module tests (including lifecycle), build and go vet passed.

## Limits
The guard prevents direct destination dialing when the required selected route
is nil or empty. It does not promise zero DNS activity before rejection, nor
change generic GOST behavior without the strict option. FLVX's per-node chain
has one next-hop selection; arbitrary partially populated multi-hop GOST routes
are not claimed to be validated by this change. Existing established connections
are not rerouted by an edit. Older agents must be upgraded to enforce the new
metadata; ordinary command acknowledgment cannot prove unknown-binary support.

Additional composed regression: each A/B/C/D receiver has independent synthetic
credentials, and every upstream connector uses the next receiver's credential.
The authenticated A–B–D → A–B–C–D transition, missing-B-chain rejection and
recovery all pass on real TCP loopback sockets. All required-route regressions,
including this composition: race repeated three times, PASS, 1.912s.
