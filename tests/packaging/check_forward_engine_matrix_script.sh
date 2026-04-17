#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
HELPER="$ROOT_DIR/tests/packaging/flvx_forward_engine_matrix_summary.sh"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

[[ -f "$HELPER" ]] || fail "missing helper script"

OUTPUT="$(bash "$HELPER")" || fail "helper execution failed"

EXPECTED=$'gost tcp\ngost udp\ndash tcp\ndash udp'

[[ "$OUTPUT" == "$EXPECTED" ]] || fail "unexpected forward engine matrix output"

printf 'forward engine matrix helper contract passed\n'
