# Final local validation

Local darwin/arm64 checkout, initial baseline 3549fe5; final integration includes upstream rc7 (46bfb86); 2026-11-08.
Go toolchain inside all modules: 1.26.10. No dependencies installed.
No production configuration was changed. Publication is authorized after review and CI.

## Final commands
- go-backend: go test -race ./... -skip '^TestCaptchaVerifyLoginContract$'
  PASS on the final code at 89a1a7b. Handler 140.273s; contract 110.907s. This includes upstream user-disable regressions.
- go-backend: go vet ./... and go build ./cmd/paneld
  PASS; executable output was directed to /tmp, not the repository.
- go-gost/x: go test -race ./... and go vet ./...
  PASS, including authenticated A–B–C–D transition/rejection/recovery.
- go-gost: go test ./..., go vet ./..., go build .
  PASS; executable output was directed to /tmp.
- git diff --check: PASS.

## Test-environment adjustments
Two pre-existing federation cache-fallback tests formerly relied on external
unreachable.invalid and peer.example requests failing. They now use loopback
httptest providers returning HTTP 503, preserving the fallback assertions.
The existing TestCaptchaVerifyLoginContract makes a real Cloudflare verification
request; it was excluded, not reported as passing. No external access restriction
was bypassed.

The broad race run exposed a pre-existing test-fixture GORM Update callback
registration racing WebSocket teardown's node-status update. Test setup now
registers those callbacks before WebSocket startup, with atomic flags for failure
injection. Two matching fixture tests passed with race detection repeated three
times, followed by the passing full backend race run above. Production behavior
was not changed for this fixture repair.

FLVX_POSTGRES_TEST_DSN is absent; PostgreSQL execution was not verified.
Three original investigation test files remain. The old bug-expecting gated test
is skipped and its complete original source preserved in the evidence directory.

## Review status and operational limits
The approved two-axis review is complete. Standards found no hard violations;
one fixture-duplication judgement call remains explicitly deferred. Spec
follow-up at 89a1a7b found no remaining blocker in the reviewed scope after the
create, pending, reservation, durable-cleanup and role-transition fixes.
Review details are in 070-code-review.md.

This local patch does not secure existing production listeners by itself.
Existing legacy tunnels remain unauthenticated until a separately initiated
migration is implemented and performed. Required is a policy, not runtime
attestation. Agents must support chain.strict, protected tunnels require full
database recovery rather than credential-free configuration restore, and older
panel rollback can reintroduce anonymous receivers. The patch does not provide
cross-node transactional rollback or fix transport confidentiality.

Frontend TypeScript and production/PWA build passed after rc7 integration using
the existing Node 24.22.1 runtime. No dependency install or engine bypass.
