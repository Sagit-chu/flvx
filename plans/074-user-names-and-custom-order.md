# FLVX #574 / #575: current owner names and custom order

Status: implementation. The user confirmed the three proposed test boundaries on 2026-11-10.
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

## Testing decisions (proposed boundaries)

1. **Public HTTP API**: current-name responses, all rename entry points, ownership isolation, admin-only reorder, complete-set validation, failures/conflicts, and list/create/edit/delete behavior. This catches application behavior but not browser interactions or every database upgrade path.
2. **Repository lifecycle and backup public interfaces**: fresh/legacy databases, restart persistence, old/new backup round trips and partial import behavior for SQLite and PostgreSQL. This catches persisted compatibility but not UI behavior. Observe through public reads/exports rather than implementation-coupled assertions where possible; legacy schema fixtures may require direct fixture setup.
3. **Rendered browser UI**: table/card drag, touch/keyboard affordances, search-disabled state, recovery from failed saves, repeat actions, and navigation. This catches actual UI wiring, not production connectivity or real forwarding.

Use Matt Pocock's vertical red-to-green cycles: one failing behavior test, minimum implementation, then the next behavior. Prefer existing contract fixtures and public interfaces. Review standards and requirements independently after implementation. These boundaries were confirmed by the user before implementation tests were added.

## Work graph

- **T1 Current owner names**: implement #574 with API regression coverage and historical/backup cases. Independent.
- **T2 Persistent order backend**: implement ordering models, migrations, endpoints, permissions, transaction/conflict handling and backup compatibility. Runs in an isolated worktree; integrate after T1 and resolve any shared repository-layer edits before reporting complete.
- **T3 Drag order frontend**: implement both list views and failure/search behavior against the agreed API contract. Can proceed after T2 contract is pinned; no backend edits.
- **T4 Verify and review**: integrate all slices, full backend checks, frontend checks, isolated SQLite/PG/browser verification where available, independent standards/spec reviews and fixes. Depends on T1–T3.

## Out of scope

Production access or changes, external comments/issues/spec publication, push/PR/merge/release, personal per-admin preferences, drag while searching, server pagination/cross-page drag, field-based sorting, changing actual speed-limit enforcement, agent changes, and unrelated refactors.

## Further notes

Work is isolated on `fix/user-names-custom-order`; original checkout remains untouched. Local commits are permitted by the requested implementation workflow, but no publication is authorized. Existing assessment reproductions are available in a temporary isolated checkout for reference, not as final tests. PostgreSQL currently has no supplied test DSN; any disposable local test database must be isolated from user data and reported explicitly.
