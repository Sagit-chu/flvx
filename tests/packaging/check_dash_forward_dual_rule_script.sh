#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HELPER="$ROOT_DIR/tests/packaging/flvx_dash_forward_dual_rule_cleanroom.py"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

[[ -f "$HELPER" ]] || fail "missing helper script"

HELP_OUTPUT="$(python3 "$HELPER" --help)" || fail "helper --help failed"

[[ "$HELP_OUTPUT" == *"--remote-host"* ]] || fail "helper help missing --remote-host"
[[ "$HELP_OUTPUT" == *"--dry-run"* ]] || fail "helper help missing --dry-run"
[[ "$HELP_OUTPUT" == *"--ssh-port"* ]] || fail "helper help missing --ssh-port"
[[ "$HELP_OUTPUT" != *"node-runtime"* ]] || fail "helper help still exposes fake node-runtime mode"

if python3 "$HELPER" >/dev/null 2>&1; then
  fail "helper should require --remote-host"
fi

OUTPUT="$(python3 "$HELPER" --remote-host dash-lab.example --remote-user tester --dry-run)" || fail "helper dry-run failed"

python3 - "$OUTPUT" <<'PY'
import json
import sys

payload = json.loads(sys.argv[1])

assert payload["mode"] == "dry-run", payload
assert payload["remote"]["host"] == "dash-lab.example", payload
assert payload["remote"]["user"] == "tester", payload
assert payload["release"]["repo"] == "Sagit-chu/Dash", payload
assert payload["release"]["binary"] == "dash", payload
assert payload["plan"]["backend"]["mode"] == "remote-linux", payload
assert payload["plan"]["dryRunNoTouch"] is True, payload
assert payload["plan"]["nodes"] == ["entry", "exit"], payload
assert payload["plan"]["trafficChecks"] == ["tcp", "udp"], payload
assert "upload_backend" in payload["plan"]["steps"], payload
assert "install_dash_bundle" in payload["plan"]["steps"], payload
assert "verify_tcp_udp" in payload["plan"]["steps"], payload
PY

printf 'dash forward dual rule helper contract passed\n'
