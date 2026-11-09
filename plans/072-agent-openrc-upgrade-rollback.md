# Agent panel upgrade and rollback on OpenRC

## Scope

Repair the agent UpgradeAgent/RollbackAgent paths on Alpine/OpenRC without regressing systemd. Preserve the pending checksum fail-closed repair. Do not change live nodes, install components, publish or deploy.

## Evidence

In a disposable cached Alpine 3.22.5 linux/amd64 container, the actual handlers fail launching `systemd-run` after successful download and verification, and also when rolling back an existing backup. Installer scripts already support OpenRC, but their CI does not exercise these command handlers.

## Safety requirements

- Prefer OpenRC on Alpine, including when a systemctl shim exists; refuse missing required manager tools.
- Keep systemd's independent transient unit behavior to survive stopping the agent cgroup.
- Launch an independent OpenRC restart helper that survives termination of the original agent, without installing extra utilities.
- Use the selected manager for stop/start; do not run systemd legacy-service cleanup on OpenRC.
- Retain validated upgrade staging and backup behavior, and do not consume the rollback backup.
- Reject setup/launch errors before claiming restart was launched. Report asynchronous-completion limits honestly.
- Preserve existing command response, request ID and progress contracts.

## Proposed tests and review

User-confirmed boundaries: real UpgradeAgent/RollbackAgent command routing, isolated files, systemd/OpenRC tools as test doubles, no-manager rejection and launch/stop/replacement failures. Use cached Alpine to execute the helper with rc-service doubles, not a real OpenRC installation. Explicitly distinguish this from service-manager integration testing.

User-confirmed review baseline is fadc2b6, covering both pending fixes and plans. No commits, push, PR, merge, release or deployment.

## Publication authorization

After the local implementation, verification and independent reviews were complete, the user authorized submitting a PR, merging if checks pass, and publishing the next tag. Earlier no-publication limits above describe the implementation phase. Deployment to live nodes remains outside scope.
