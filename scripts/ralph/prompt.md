# Ralph Agent Task

Implement features from user stories until all are complete.

## Workflow Per Iteration

1. Read `scripts/ralph/log.md` to understand previous progress.
2. Search `docs/user-stories/` for entries with `"passes": false`.
3. If no failing entries remain, output `<promise>FINISHED</promise>`.
4. Pick exactly one story entry with highest implementation priority.
5. Implement and verify acceptance criteria.
6. Run relevant verification commands for changed components.
7. If checks fail, fix and rerun until passing.
8. Mark the implemented story entry `passes: true`.
9. Append concise notes to `scripts/ralph/log.md`.

## Project-Specific Constraints

- Backend: run tests via `go test` in `go-backend`.
- Frontend: do not add new test frameworks.
- Keep API response envelope unchanged.
- Authorization header must remain raw JWT token (no Bearer prefix).

## Completion Signal

When all stories pass:

`<promise>FINISHED</promise>`
