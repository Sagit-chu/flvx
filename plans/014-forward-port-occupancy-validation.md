# 014 Forward Port Occupancy Validation

## Checklist

- [x] Confirm current forward create/update only validates node port range and misses DB-backed occupancy checks for local nodes.
- [x] Add shared forward port occupancy validation for create/update paths before runtime dispatch.
- [x] Add focused tests covering create/update validation when another forward already uses the same node+port.
- [x] Run focused backend handler tests and record the result.

## Test Record

- Command: `cd go-backend && go test ./internal/http/handler/...`
- Result: passed.
