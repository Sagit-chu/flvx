# OpenRC agent upgrade and rollback validation

## Baseline and scope

User-approved review baseline: fadc2b6c122df99140e0c9cbd481a1214d17d175. Includes the pending checksum fail-closed repair (071) and manager-aware upgrade/rollback (072). No publication, deployment, or new software installation.

## Reproduction

A disposable existing Alpine 3.22.5 linux/amd64 image on Mac mini Docker reproduced both failures: actual upgrade and rollback handlers could not find systemd-run. The upgrade used a valid checksum. Source inspection confirmed install.sh supports OpenRC but the two agent handlers did not. Existing installer CI did not test those command handlers.

## Regression coverage

Actual UpgradeAgent/RollbackAgent routing and WebSocket responses cover manager selection, Alpine preference over systemctl shims, missing manager, rejected submission, backup preservation, atomic rollback staging, request IDs and all checksum cases from 071. Script execution checks stop/replace/start order, stop failure, replacement failure recovery, directory targets and no systemd legacy cleanup on OpenRC.

Linux-only integration executes the real detached helper. An rc-service double sends SIGTERM to the original test-agent's entire process group. The test verifies that the original process receives SIGTERM while the helper survives, replaces the binary with the rollback copy, preserves the backup and invokes the start command.

## Environment and checks

- Mac mini: Go 1.26.10 darwin/arm64.
- Cached Alpine 3.22.5 linux/amd64 under Docker emulation; no package installation. Tests use loopback HTTP and service-command doubles, with no host service mounts.
- Full socket tests and focused race tests: passed.
- Alpine manager/upgrade/rollback/checksum/helper-survival suite: passed.
- go-gost/x full `go test ./...`, `go vet ./...`: passed.
- go-gost `go test ./...`, `go vet ./...`, build to temporary output: passed.
- `git diff --check`: passed.

## Limitations

Real OpenRC/systemd service integration was not run. The detached process test models the installed OpenRC PID/process-group stopping behavior, not custom cgroup policies. Successful command response acknowledges helper launch or systemd submission, not completed asynchronous service restart. No automatic downgrade is attempted if starting the replaced executable fails. Live nodes were not changed. HTTP request timeouts, release/deployment, and retrofitting already installed agents are outside this local repair.

## Independent review

Standards: no documented violations or blockers. Optional one-line legacy restart-script wrapper cleanup was deferred to preserve the existing focused legacy-service regression interface.

Spec: no blocking implementation deviation. Reviewer requested explicit OpenRC process-launch-failure coverage; follow-up validation is tracked below.

Follow-up: explicit OpenRC exec-failure coverage passed for both routed commands. A Linux-only, explicitly opted-in disposable Alpine test temporarily hides only the container's /bin/sh symlink and restores it afterward. It uses production Alpine detection, executable lookup and cmd.Start; both commands reject ENOENT, preserve the live executable and expected backup, and remove staging. No runtime test abstraction was added. The test requires Alpine and Docker markers and is skipped by default.

Reproduction of the opt-in case (only a disposable container; never mount host /bin): build the socket test binary with `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go test -c -o /tmp/flvx-072-socket.test ./socket` from go-gost/x, then run `docker run --rm --pull=never --platform linux/amd64 --network none -e FLVX_TEST_DISPOSABLE_ALPINE_SHELL_FAILURE=1 -v /tmp/flvx-072-socket.test:/probe/socket.test:ro alpine:3.22 /probe/socket.test -test.run '^TestOpenRCCommandsCleanStageOnHelperExecFailure$' -test.v`.

Final host full socket race suite passed in 8.844s. Final follow-up changed only Linux test coverage; Mac socket/vet and diff checks passed again.

Independent Spec re-review confirmed the launch-failure gap is closed, with no remaining actionable spec gaps.
