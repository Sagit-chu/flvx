# 013 Forward Delete NotFound Compatibility Fix

## Checklist

- [x] Confirm forward update failure path caused by delete fallback short-circuiting on the first not-found service name.
- [x] Update forward service deletion logic to continue across all candidate runtime names until one is actually deleted or every candidate is exhausted.
- [x] Add regression tests covering mixed not-found and legacy-name delete recovery during forward control/update flows.
- [x] Run focused backend handler tests and record the result.

## Test Record

- Command: `cd go-backend && go test ./internal/http/handler/...`
- Result: passed.
