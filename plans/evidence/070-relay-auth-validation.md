# Relay authentication validation

Local darwin/arm64 checkout, base 3549fe5. No deploy/push/release or production
credentials. Three pre-existing investigation tests were preserved.

## Confirmed seam

Panel HTTP plus node/federation commands, using temporary SQLite repositories,
loopback HTTP/WebSocket peers, and captured command data. Native data-plane auth
matrix is recorded separately in the required-route validation evidence.

## Vertical regression evidence

- New local tunnel: observed failure because generated relay connector/receiver
  authentication was absent; implemented purpose-separated endpoint credentials.
- Required tunnel configuration export: observed missing authentication-policy
  marker; exported marker added, credentials remain excluded. Configuration
  restore of protected tunnels explicitly rejects missing private state.
- Federation reserve/apply: observed missing capability advertisement; added
  versioned contract, receiver authentication, persistent state, and no-downgrade.
- Full dual-panel middle/exit tunnel: observed zero authenticated receivers;
  threaded receiving and next-target credentials through consumer requests.
- Federation strict middle policy: observed absent chain.strict; local and
  federation middle generators now set it while exits retain direct dialing.
- Insert a new middle before an existing middle: observed rejected update with
  hop-index credentials; receiver identity changed to tunnel/node/role, preserving
  the existing credential and assigning the new receiver a distinct credential.
- Direct-to-relay conversion: observed absent relay authentication; conversion now
  requires credentials while existing legacy relay tunnels remain explicit.
- Provider private JSON loss: observed successful reapply recreating protected
  state; separately persisted auth version now rejects missing private material.

Additional command-seam checks cover local update stability, reconnect followed
by explicit public batch-redeploy (avoids the 30-second production debounce),
missing consumer private state rejection, old peer refusal before reservation,
legacy list visibility, matched per-hop credentials and secret-free list/export.

## Results

Eight authentication contract tests with race detection, repeated three times:
`go test -race ./tests/contract -run '^(TestNewRelayTunnelEmitsMatchingPrivateEndpointAuthentication|TestFederatedRelayRequiresVersionedAuthAndRejectsDowngrade|TestNewFederatedTunnelAuthenticatesEveryRelayHop|TestRelayAuthenticationSurvivesInsertingBeforeExistingMiddle|TestConvertingDirectTunnelRequiresRelayAuthentication|TestBackupImportCannotSilentlyDowngradeRequiredRelay|TestNewRelayTunnelRefusesLegacyFederationBeforeReservation|TestLegacyRelayStateRemainsExplicitInList)$' -count=3`

Earlier pass: 17.878 seconds. Final run after provider version guard and log
redaction: PASS, 19.584 seconds. Focused non-TLS generation
regression passes after narrowing its old overbroad no-metadata assertion.
`git diff --check` passes.

## Explicit limitations

- Authentication-only protection against anonymous relay clients, not credential
  confidentiality over raw TCP/unverified TLS/public-default KCP or insecure
  control-plane paths.
- Required mode is desired policy, not attestation of every participating agent.
  Old agent strict-route support requires upgrading the agent.
- Existing legacy relay tunnels remain unprotected. No automatic migration,
  credential rotation, or configuration-only credential recovery is implemented.
- SQLite/loopback verification only in this task; production topology, live
  upgrades, and PostgreSQL runtime checks are not exercised here.
