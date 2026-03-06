# 015 Forward Runtime Port Residual Cleanup

## Checklist

- [x] Confirm 2.1.6 used service names with `_0` runtime base while later versions may target resolved `user_tunnel_id`, leaving old runtime services behind after direct upgrade.
- [x] Extend self-occupy recovery to clean residual candidate service names and retry update/add when the port is only occupied by self-owned legacy runtime services.
- [x] Add regression tests covering address-in-use recovery with legacy `_0` runtime residue.
- [x] Run focused backend handler tests and record the result.

## Test Record

- Command: `cd go-backend && go test ./internal/http/handler/...`
- Result: passed.
