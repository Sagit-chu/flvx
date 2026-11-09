# Agent checksum upgrade validation

## Scope and baseline

Reviewed local changes against `fadc2b6c122df99140e0c9cbd481a1214d17d175` (rc8), as confirmed by the user. Tests use the actual UpgradeAgent command router and WebSocket response path, loopback HTTP, temporary executable/backup paths, and an intercepted restart action. No real installer or service process is launched.

## Reproduction and fix

Four red-to-green slices reproduced missing URL, checksum HTTP 404, ambiguous multiple records, and oversized checksum acceptance. Initially the command returned success, emitted verification/install progress, and reached the restart substitute. The corrected flow returns an unsuccessful UpgradeAgentResponse before installation and removes staged bytes.

Additional cases cover blank URL, invalid request URL, connection interruption, truncated HTTP body despite a valid-looking hash, empty/whitespace checksum, short/nonhex/mismatched hash, invalid GNU marker, absent filename, garbage, NUL and embedded newline. Valid fixtures cover bare digest, GNU text and binary records, uppercase hex, CRLF and surrounding whitespace. Both URL architecture placeholders and response request IDs are checked. Failed cases preserve old executable and backup; valid cases reach the intercepted restart with exact `abc` bytes and the independent known SHA256 fixture.

## Checks

- Initial socket baseline: passed.
- UpgradeAgent targeted tests and race tests: passed.
- Socket package race tests, three runs: passed (2.009s after cleanup).
- go-gost/x: `go test ./...` and `go vet ./...`: passed.
- go-gost: `go test ./...`, `go vet ./...`, and build to a temporary output: passed.
- `git diff --check`: passed.
- All listed module checks and the build were rerun after cleanup and passed.
- Go toolchain: 1.26.10 darwin/arm64.

## Independent review

Standards: no hard violations or blockers. One optional duplicate cleanup observation was addressed by consolidating staging removal in the defer.

Spec: no actionable deviations found. Missing checksum, fetch/status/read/format/hash errors stop before success and installation; supported SHA256 formats and architecture substitution are retained. No unrelated installer, Alpine, release or deployment changes.

## Limits

This verifies the agent command and integrity gate, not actual systemd installation or live-node deployment. The fix must be installed on an agent before that agent gains the new behavior; updating the panel alone cannot retrofit old agents. Hash comparison is not a signature and does not protect against a compromised source supplying both artifact and matching checksum. No push, PR, merge, tag or deployment was performed. HTTP request timeout and installer/platform redesign remain outside this repair.
