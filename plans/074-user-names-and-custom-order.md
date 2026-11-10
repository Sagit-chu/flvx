# FLVX #574 / #575: current owner names and custom order

Status: implemented and verified locally on 2026-11-10. The user confirmed the three test boundaries before implementation and later authorized the PostgreSQL import repair discovered during validation.
Base: `3209ff8a59bf02287e6934d0b80b941006db239e` (main, verified 2026-11-10).
Sources: https://github.com/Sagit-chu/flvx/issues/574 and https://github.com/Sagit-chu/flvx/issues/575.

## Problem statement

Existing forwarding rules retain the username copied when they were created. Renaming an account leaves the rules page's owner filter and related displays showing the old name, including after restart. Users and speed limits have only ID-descending order, so an administrator cannot arrange them intentionally.

## Solution

Resolve a forwarding rule's current owner name by immutable user ID without recreating rules. Add administrator-managed global drag order for users and speed limits, persisted on the server across clients. Newly created entries appear first. Searching disables dragging so a partial result cannot overwrite the complete order.

## User stories

1. As an administrator, I see the current owner name on existing rules after renaming a user.
2. As a user changing my account name and password, my existing rules display the new name.
3. As an administrator, I see consistent names in rule groups, owner filters and tunnel deletion previews.
4. As an administrator restoring an old backup, I do not have to delete or recreate rules to correct owner names.
5. As a user, a different account reusing my former name cannot acquire my rules or permissions.
6. As an administrator, I can drag ordinary users into a global order in both card and table views.
7. As an administrator, I can drag speed limits into a global order in both card and table views.
8. As an administrator switching browsers or signing in again, I see the server's saved order.
9. As an administrator creating an item, I find it at the top; editing it keeps its position.
10. As an administrator searching a list, I see why dragging is unavailable and hidden items retain their positions.
11. As an administrator with more than ten users, I do not lose entries from the reorder set after creating a user.
12. As an administrator whose save fails or conflicts, I see the failure and recover the authoritative order rather than a false success.
13. As an operator upgrading an existing SQLite or PostgreSQL database, I retain the prior default order until I rearrange it.
14. As an operator restoring a new backup, I retain its order; older backups remain importable.
15. As a user, an administrative display-order change does not change my password, ownership, permissions, traffic totals, limits, or rule bindings.
16. As an operator restoring PostgreSQL data, I can immediately create new records without restarting, with existing sequence allocations preserved and failed imports rolled back.

## Implementation decisions

- Rule ownership remains numeric user ID. Display uses the current associated username, with the stored name (and existing unknown-owner UI) as a compatibility fallback when no associated user exists.
- Keep the legacy stored name field and backup format compatible. Display correction must work for stale data without a destructive rewrite or rule recreation.
- Cover all relevant presentation projections, not just the owner selector. Do not change data-plane service identity or authentication behavior.
- Add integer ordering to ordinary users and speed limits. Preserve old ID-descending order for tied/default ranks, set new items before the current first rank, and preserve rank on ordinary edits.
- Add administrator-only reorder endpoints for both resources. A request supplies the complete ordered ID set and its previously observed order so stale sets/orders can be rejected safely; return an actionable conflict/error rather than partial success.
- Validate input as a complete permutation of eligible current resources. Reject malformed IDs, duplicates, missing/extra IDs and administrator user IDs. All writes are transactional.
- Serialize client saves or disable dragging while a save is outstanding. On failure/conflict, reload the server's order. Cancel or ignore stale list requests when search/navigation changes.
- Reuse existing drag sensors and affordances. Support mouse, touch and keyboard. Keep delete/edit controls usable and prevent accidental drag activation.
- Search disables drag. User creation reloads the authoritative complete list, avoiding the existing ten-item truncation.
- Preserve existing API envelopes and raw JWT convention. Explicitly protect the new user reorder path in authorization middleware.
- User and speed-limit backup DTOs, export mappings and import updates carry order. Missing old-backup fields must not accidentally zero an existing custom order. New records imported without rank follow a documented deterministic rule.
- No new frontend test framework; use the repository's build/lint and isolated browser checks.

## Testing decisions (confirmed boundaries)

1. **Public HTTP API**: current-name responses, all rename entry points, ownership isolation, admin-only reorder, complete-set validation, failures/conflicts, and list/create/edit/delete behavior. This catches application behavior but not browser interactions or every database upgrade path.
2. **Repository lifecycle and backup public interfaces**: fresh/legacy databases, restart persistence, old/new backup round trips and partial import behavior for SQLite and PostgreSQL. This catches persisted compatibility but not UI behavior. Observe through public reads/exports rather than implementation-coupled assertions where possible; legacy schema fixtures may require direct fixture setup.
3. **Rendered browser UI**: table/card drag, touch/keyboard affordances, search-disabled state, recovery from failed saves, repeat actions, and navigation. This catches actual UI wiring, not production connectivity or real forwarding.

Use Matt Pocock's vertical red-to-green cycles: one failing behavior test, minimum implementation, then the next behavior. Prefer existing contract fixtures and public interfaces. Review standards and requirements independently after implementation. These boundaries were confirmed by the user before implementation tests were added.

## Work graph

- **T1 Current owner names**: implement #574 with API regression coverage and historical/backup cases. Independent.
- **T2 Persistent order backend**: implement ordering models, migrations, endpoints, permissions, transaction/conflict handling and backup compatibility. Runs in an isolated worktree; integrate after T1 and resolve any shared repository-layer edits before reporting complete.
- **T3 Drag order frontend**: implement both list views and failure/search behavior against the agreed API contract. Can proceed after T2 contract is pinned; no backend edits.
- **T4 Verify and review**: integrate all slices, full backend checks, frontend checks, isolated SQLite/PG/browser verification, independent standards/spec reviews and fixes. Depends on T1–T3. Complete.
- **T5 PostgreSQL import sequences**: user-authorized follow-on repair for the pre-existing explicit-ID import bug. Import takes the complete set of write-table locks with NOWAIT before any data writes, retries only pre-write lock contention within a bounded budget, and advances actual ID sequences transactionally without moving allocated IDs backward. Complete.

## Out of scope

Production access or changes, external comments/issues/spec publication, push/PR/merge/release, personal per-admin preferences, drag while searching, server pagination/cross-page drag, field-based sorting, changing actual speed-limit enforcement, agent changes, and unrelated refactors.

## Further notes

Work is isolated on `fix/user-names-custom-order`; original checkout remains untouched. Local commits are permitted by the requested implementation workflow, but no publication is authorized. Existing assessment reproductions are available in a temporary isolated checkout for reference, not as final tests. PostgreSQL testing used an isolated, disposable local PostgreSQL 16.15 container with per-test schemas. No production database or user data was used.


## Verification results

Final implementation merge: `2ad30e4b2f587b0118cb53049194c915fb32565e`.

- Full backend suite passed with PostgreSQL enabled and the cache disabled: `FLVX_POSTGRES_TEST_DSN=... go test -count=1 ./...`.
- Race-enabled owner-name, display-order and PostgreSQL backup-import contracts passed. The new PostgreSQL import contracts also passed three repeated race-enabled runs in their implementation worktree.
- `go vet ./...`, native backend build and Linux amd64/arm64 cross-builds passed using Go 1.26.10.
- Frontend `tsc --noEmit`, full `eslint .`, and production build passed using Node 24.22.1. The build still reports the existing outdated Browserslist data notice.
- SQLite and PostgreSQL contracts cover three rename paths, old-name reuse/ownership isolation, historical fallback, previews, restart and old/new backups. Order contracts cover administrator checks, complete permutations, stale order/membership, concurrent creates/reorders, new-first and edit-preserved rank.
- PostgreSQL import tests reproduce the prior 23505 failure and cover immediate public API creates without reopening the repository, all 11 explicit-ID tables, allocated/cached IDs, non-unit positive increments, unowned legacy defaults, search-path resolution, concurrent imports/creates, cascade-delete contention, bounded busy recovery, intermediate-sequence failure and commit-time rollback. Removing NOWAIT makes the contention regression fail.
- Chrome tests against synthetic localhost data passed: user and limit card keyboard reorder; user and limit table pointer reorder; 390px viewport touch reorder; refresh persistence; 13-user creation without truncation; new-first and edit-preserved order; search-disabled handles; drag cancellation; a second client's conflict followed by authoritative recovery; transport failure and successful load-error retry.
- The rendered rules page and owner filter show the current username for a historical rule whose stored snapshot still has the former name.
- Restarting the final backend binary preserved the 13-user order, four-limit order and current-name projection.
- Independent standards and requirements reviews passed after fixes. The recovered-load error is now cleared without hiding save warnings. The PostgreSQL lock-order and legacy-default findings were resolved and re-reviewed.

## Validation limits

- Linux amd64/arm64 binaries were cross-built, not run on those target machines. No live forwarding agents, production deployment, real traffic, Safari or physical mobile device were exercised. Mobile touch was tested through Chrome's emulated viewport and touch input.
- PostgreSQL identity-column fallback was reviewed but does not have a dedicated fixture. Normal repository writes and imports are covered; external SQL that reserves sequence values inside an open transaction and later acquires tables in reverse order, or concurrent manual sequence-definition changes, are outside the tested contract. Negative/non-positive ID sequences are rejected with a rolled-back import.
- Browser logs included existing React Router/dialog accessibility warnings and a password-manager-extension error. A development HMR hook-order error during the hook edit cleared on full reload; final page flows did not reproduce it.
- All changes remain on a local branch. No push, pull request, issue comment, release or production change was performed.
