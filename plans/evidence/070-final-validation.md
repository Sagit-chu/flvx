# Final local validation

Local darwin/arm64 checkout, baseline 3549fe5; 2026-11-08.
Go toolchain inside all modules: 1.26.10. No dependencies installed.
No production configuration, commit, push, deployment or release.

## Final commands
- go-backend: go test -race ./... -skip '^TestCaptchaVerifyLoginContract$'
  PASS. Handler 132.684s; contract 103.615s; repository 30.830s.
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
Formal Matt Pocock two-axis Standards/Spec review is pending owner confirmation
of the fixed review base (suggested 3549fe5 plus all local uncommitted changes).
The originating task and local plan are the proposed spec.

This local patch does not secure existing production listeners by itself.
Existing legacy tunnels remain unauthenticated until a separately initiated
migration is implemented and performed. Required is a policy, not runtime
attestation. Agents must support chain.strict, protected tunnels require full
database recovery rather than credential-free configuration restore, and older
panel rollback can reintroduce anonymous receivers. The patch does not provide
cross-node transactional rollback or fix transport confidentiality.
