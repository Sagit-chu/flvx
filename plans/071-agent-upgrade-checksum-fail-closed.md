# Agent upgrade checksum fail-closed

## Scope

Fix the panel-issued UpgradeAgent flow only. Do not change Alpine support, installer scripts, release artifacts, production state, or publish changes.

## Observed defect

`handleUpgradeAgent` treats checksum fetch errors, non-200 responses and body read errors as success and emits `校验通过`. An absent checksum URL bypasses verification entirely. The panel already supplies a checksum URL for individual and batch upgrades, and the release workflow generates GNU sha256sum files.

## Intended behavior

- Require a nonblank checksum URL before downloading an upgrade.
- Abort before chmod, backup, or restart on fetch/status/read/format/hash failure; remove the staged binary.
- Only emit verification success after a complete, valid SHA256 record matches the downloaded bytes.
- Accept one bare 64-hex SHA256 or a single GNU sha256sum record (text or binary marker), case-insensitive hex and surrounding whitespace/CRLF. Reject empty, truncated, nonhex, multiline and oversized payloads. Do not treat an ambiguous multi-file manifest as a checksum.
- Preserve architecture substitution and current installation flow. This is integrity checking, not a signature or an independent guarantee against a compromised download source.

## Verification plan

User-confirmed seam: actual UpgradeAgent command handling, loopback HTTP fixtures, isolated binary paths and an intercepted restart action. Assert error/response and progress, untouched old executable/backup, absence of staged bytes and no restart on every invalid checksum condition. Valid records should reach the intercepted restart with exact downloaded bytes.

Use red-green slices, then race tests, module checks, build and independent scope review. No push, PR, merge, tag or deployment is authorized for this task.

## Review scope

User confirmed review against rc8 commit `fadc2b6c122df99140e0c9cbd481a1214d17d175`, covering all changes for this task and this plan. Run independent Standards and Spec reviews after the implementation is ready.
