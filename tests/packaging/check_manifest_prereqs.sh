#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MANIFEST_PATH="$ROOT_DIR/release/dash-build-manifest.json"
WORKFLOW_PATH="$ROOT_DIR/.github/workflows/docker-build.yml"

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

[[ -f "$MANIFEST_PATH" ]] || fail "missing release/dash-build-manifest.json"
grep -Fq '"repo": "Sagit-chu/Dash"' "$MANIFEST_PATH" || fail "manifest missing Dash repo"
grep -Fq '"ref": "main"' "$MANIFEST_PATH" || fail "manifest missing Dash ref"
grep -Fq '"binary_name": "dash"' "$MANIFEST_PATH" || fail "manifest missing binary name"
grep -Fq '"targets": ["linux/amd64", "linux/arm64"]' "$MANIFEST_PATH" || fail "manifest missing targets"

[[ -f "$WORKFLOW_PATH" ]] || fail "missing docker-build workflow"
grep -Fq 'DASH_REPO:' "$WORKFLOW_PATH" || fail "workflow missing DASH_REPO reference"
grep -Fq 'DASH_REF:' "$WORKFLOW_PATH" || fail "workflow missing DASH_REF reference"
grep -Fq 'DASH_REPO_TOKEN:' "$WORKFLOW_PATH" || fail "workflow missing DASH_REPO_TOKEN reference"

printf 'manifest prerequisites are present\n'
