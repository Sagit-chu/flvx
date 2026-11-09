# Panel self-upgrade reliability plan

## Scope

Fix the panel's Docker Compose self-upgrade flow. The report is from a third-party deployment; this work uses repository analysis and isolated tests, not access to that deployment. Base: `a784350` (`3.0.1-rc10`). No publishing or deployment is included.

## Confirmed problems

- The UI stops at helper launch. It cannot distinguish a running, successful, or failed upgrade.
- The helper resolves the Compose project and relative bind mounts from `/opt/flvx-panel`, which need not match the original host deployment.
- The HTTP mutex ends before the helper. Repeated requests can change the same configuration and overwrite logs and fixed backups.
- Active configuration is replaced before images are pulled; helper failures do not restore it.
- Successful `compose up` is not an explicit verification of both target images and frontend availability.

These are code defects and failure risks, not proof of the reported deployment's particular failure.

## Implementation plan

1. Identify and validate the existing Compose project, host deployment mount, backend service, and configuration paths before a task can mutate deployment files. Fail safely on unsupported or ambiguous deployment layouts.
2. Add a durable task with a unique ID, per-task log and immutable backups. Keep the task lock across the HTTP request, helper execution, and backend replacement. Reconcile stopped helpers conservatively; uncertainty must not permit overlapping upgrades.
3. Run a dedicated helper command in the current backend image. Validate a staged Compose configuration and pull images before replacing active files. Use atomic file replacement. Restore configuration only when failure is known to precede service replacement. A partial or uncertain service replacement requires explicit recovery rather than an unsafe automatic database/container rollback.
4. Record progress and terminal results locally, with a bounded execution deadline. Verify target services before success. Expose only safe progress messages through an admin status endpoint, never `.env` or arbitrary command output.
5. Add frontend progress, reconnect handling, stale-response protection, reload recovery, and repeat-click prevention. A launch acknowledgment is not completion.
6. Run focused fault-injection tests, the full backend checks, frontend lint/type/build checks, and UI lifecycle exercises. Document real Docker and PostgreSQL tests that cannot run locally.

## API contract

- Existing `POST /api/v1/system/upgrade`: preserve existing response fields and add `jobId`.
- New `POST /api/v1/system/upgrade/status`: accept optional `jobId`; return `currentVersion` and nullable `job`.
- A job includes `id`, `targetVersion`, `stage`, `active`, `requiresRecovery`, `message`, `startedAt`, `updatedAt`, and optional helper/backup references.
- Stages: `preparing`, `pulling`, `applying`, `restarting`, `verifying`, `succeeded`, `failed`, `recovery_required`.
- Status reads do not call GitHub. Timestamps use Unix milliseconds. A recovery-required task must not silently permit another upgrade.

## Required regression coverage

- Original project names and host paths that differ from the in-container path; invalid/missing labels and mounts; IPv4/IPv6 templates.
- Validation and pull failure without changing active config; application failure with safe restoration; partial recreation and verification failures with preserved recovery evidence.
- Duplicate requests, interrupted starts, stopped helpers, backend restart, task timeout, and preservation of older backups and logs.
- Frontend reload, transient disconnect, success, failure, recovery required, repeated clicks, unmount, and stale responses.
- No sensitive environment contents in status responses or browser-visible error messages.

## Rollout boundary

The launcher and helper are supplied by the version currently running, not by the target Compose download. Existing rc10 installations do not acquire this fix until the corrected backend image is actually installed. The first transition to the fixed release should use the established host-side upgrade process with the original deployment directory and Compose project. Do not recommend repeated retries of the old web updater or a blind rollback after partial service replacement.

## Verification record

Implementation completed in `fix/panel-upgrade-jobs`, based on `a784350`.

- Backend: full `go test ./...`, `go test -race ./internal/panelupgrade`, `go vet ./...`, native build, and CGO-disabled Linux amd64/arm64 builds passed with Go 1.26.10.
- Added an admin/non-admin/anonymous status contract, idle read with no Docker dependency, malformed job-ID rejection, and focused upgrade tests in CI.
- Fake-Docker tests cover configuration/pull/recreate/verification failures, partial configuration restoration, immutable task backups, uncertain launch, cross-manager blocking, original project/path, full service-setting preservation, unexpected database mounts, and safe public messages.
- Frontend: changed-file ESLint (zero warnings), TypeScript and production build passed with Node 24.22.1. Nine temporary lifecycle replays passed, including two-admin refresh, repeated submissions, unknown transport outcomes, stale replies, unmount, and bounded polling.
- Mocked rendered checks passed at 1280×900 and 390×844: confirmation, pulling, an injected 503 disconnect, reconnect, verified completion, reload discovery, and recovery blocking. The pre-existing Switch `classNames` React warning remains; the deliberate 503 was expected. No new framework error overlay or mobile horizontal overflow was found.
- Independent review identified and closed full-service configuration preservation, shadow database mounts, and terminal refresh discovery issues. `git diff --check` passed.

Limitations: no real Docker daemon upgrade was run; local daemon access was unavailable to this task. Fake Docker commands do not prove real Compose/Engine behavior. The PostgreSQL-specific contract was explicitly skipped because `FLVX_POSTGRES_TEST_DSN` was not set. No third-party deployment, SSH, publishing, release, or production restart was performed during implementation.

The conservative operator recovery procedure is in `go-backend/internal/panelupgrade/README.md`. Full service-setting changes, unsupported deployment layouts, or a possible partial recreation require administrator handling instead of silently changing live settings or automatically rolling back a database. Per-task logs contain safe phase summaries, not raw secret-bearing Docker output.
