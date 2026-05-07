#!/bin/bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

fail() {
  echo "FAIL: $1" >&2
  exit 1
}

assert_equals() {
  local expected="$1"
  local actual="$2"
  local message="$3"

  if [[ "$actual" != "$expected" ]]; then
    fail "$message (expected: $expected, actual: $actual)"
  fi
}

load_script_without_main() {
  local script_path="$1"
  local temp_file

  temp_file=$(mktemp)
  sed '/^# 执行主函数$/,$d' "$script_path" > "$temp_file"
  VERSION="v-test"
  source "$temp_file"
  rm -f "$temp_file"
}

test_install_script_respects_disabled_proxy() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/install.sh"

  PROXY_ENABLED="false"
  PROXY_URL=""

  local actual
  actual=$(maybe_proxy_url "https://github.com/example/release")

  assert_equals \
    "https://github.com/example/release" \
    "$actual" \
    "install.sh should bypass the proxy when disabled"
)

test_install_script_asks_for_proxy_config() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/install.sh"

  PROXY_ENABLED=""
  PROXY_URL=""

  ask_proxy_config >/dev/null < <(printf '\nmirror.example.com/\n')

  assert_equals "true" "$PROXY_ENABLED" "install.sh should enable proxy by default"
  assert_equals "mirror.example.com/" "$PROXY_URL" "install.sh should keep the entered proxy URL"

  local actual
  actual=$(maybe_proxy_url "https://github.com/example/release")

  assert_equals \
    "https://mirror.example.com/https://github.com/example/release" \
    "$actual" \
    "install.sh should normalize the entered proxy URL"
)

test_install_script_recomputes_download_url_after_prompt() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/install.sh"

  PROXY_ENABLED=""
  PROXY_URL=""
  DOWNLOAD_URL=""

  ask_proxy_config >/dev/null < <(printf 'n\n')
  ensure_download_url_initialized

  local expected
  expected=$(build_download_url)

  assert_equals \
    "$expected" \
    "$DOWNLOAD_URL" \
    "install.sh should build the final download URL after the interactive proxy choice"
)

test_update_flux_agent_asks_for_proxy_config() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/install.sh"

  INSTALL_DIR=$(mktemp -d)
  cat > "$INSTALL_DIR/flux_agent" <<'EOF'
#!/bin/bash
echo "old version"
EOF
  chmod +x "$INSTALL_DIR/flux_agent"

  local ask_called="0"
  local cleanup_called="0"

  ask_proxy_config() {
    ask_called="1"
    PROXY_ENABLED="false"
    DOWNLOAD_URL=""
  }

  cleanup_legacy_gost_installation() {
    cleanup_called="1"
  }

  check_and_install_tcpkill() { :; }

  systemctl() {
    return 0
  }

  curl() {
    local output=""

    while [[ $# -gt 0 ]]; do
      if [[ "$1" == "-o" ]]; then
        output="$2"
        shift 2
        continue
      fi
      shift
    done

    cat > "$output" <<'EOF'
#!/bin/bash
echo "new version"
EOF
    chmod +x "$output"
  }

  update_flux_agent >/dev/null

  assert_equals "1" "$ask_called" "update_flux_agent should ask for proxy config before downloading"
  assert_equals "1" "$cleanup_called" "update_flux_agent should clean up legacy gost before restarting the agent"
  assert_equals "$(build_download_url)" "$DOWNLOAD_URL" "update_flux_agent should honor the prompted proxy choice"
)

test_update_flux_agent_skips_proxy_prompt_when_not_installed() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/install.sh"

  INSTALL_DIR=$(mktemp -u)

  local ask_called="0"

  ask_proxy_config() {
    ask_called="1"
  }

  local rc="0"
  update_flux_agent >/dev/null || rc="$?"

  assert_equals "1" "$rc" "update_flux_agent should fail when the agent is not installed"
  assert_equals "0" "$ask_called" "update_flux_agent should not prompt for proxy config when the agent is missing"
)

test_install_flux_agent_preserves_legacy_gost_when_download_fails() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/install.sh"

  INSTALL_DIR=$(mktemp -d)
  cat > "$INSTALL_DIR/flux_agent" <<'EOF'
#!/bin/bash
echo "old version"
EOF
  chmod +x "$INSTALL_DIR/flux_agent"
  SERVER_ADDR="panel.example.com:443"
  SECRET="secret"
  DOWNLOAD_URL="https://example.com/gost"

  local cleanup_called="0"
  local rc="0"

  ask_proxy_config() { :; }
  ensure_download_url_initialized() { :; }
  get_config_params() { :; }
  check_and_install_tcpkill() { :; }
  cleanup_legacy_gost_installation() {
    cleanup_called="1"
  }
  systemctl() { return 0; }
  curl() { return 0; }

  ( install_flux_agent >/dev/null ) || rc="$?"

  assert_equals "1" "$rc" "install_flux_agent should fail when the download artifact is missing"
  assert_equals "0" "$cleanup_called" "install_flux_agent should preserve legacy gost when download fails"
  [[ -f "$INSTALL_DIR/flux_agent" ]] || fail "install_flux_agent should keep the existing flux_agent binary when download fails"
)

test_update_flux_agent_preserves_legacy_gost_when_download_fails() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/install.sh"

  INSTALL_DIR=$(mktemp -d)
  cat > "$INSTALL_DIR/flux_agent" <<'EOF'
#!/bin/bash
echo "old version"
EOF
  chmod +x "$INSTALL_DIR/flux_agent"
  cat > "$INSTALL_DIR/flux_agent.new" <<'EOF'
#!/bin/bash
echo "stale version"
EOF
  chmod +x "$INSTALL_DIR/flux_agent.new"

  local cleanup_called="0"
  local rc="0"

  ask_proxy_config() {
    PROXY_ENABLED="false"
    DOWNLOAD_URL="https://example.com/gost"
  }
  check_and_install_tcpkill() { :; }
  cleanup_legacy_gost_installation() {
    cleanup_called="1"
  }
  systemctl() { return 0; }
  curl() { return 0; }

  update_flux_agent >/dev/null || rc="$?"

  assert_equals "1" "$rc" "update_flux_agent should fail when the download artifact is missing"
  assert_equals "0" "$cleanup_called" "update_flux_agent should preserve legacy gost when download fails"
  [[ ! -f "$INSTALL_DIR/flux_agent.new" ]] || fail "update_flux_agent should remove stale download artifacts before retrying"
)

test_install_flux_agent_writes_json_safe_config() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/install.sh"

  INSTALL_DIR=$(mktemp -d)
  SERVER_ADDR='panel"addr'
  SECRET='sec\ret"1'
  DOWNLOAD_URL="https://example.com/gost"

  ask_proxy_config() { :; }
  ensure_download_url_initialized() { :; }
  get_config_params() { :; }
  check_and_install_tcpkill() { :; }
  cleanup_legacy_gost_installation() { :; }
  systemctl() { return 0; }
  curl() {
    local output=""
    while [[ $# -gt 0 ]]; do
      if [[ "$1" == "-o" ]]; then
        output="$2"
        shift 2
        continue
      fi
      shift
    done

    cat > "$output" <<'EOF'
#!/bin/bash
echo "new version"
EOF
    chmod +x "$output"
  }

  ( install_flux_agent >/dev/null 2>/dev/null ) || true

  local actual
  actual=$(<"$INSTALL_DIR/config.json")
  local expected=$'{\n  "addr": "panel\\"addr",\n  "secret": "sec\\\\ret\\"1"\n}'

  assert_equals "$expected" "$actual" "install_flux_agent should JSON-escape config values"
)

test_cleanup_legacy_gost_installation_removes_service_and_binary() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/install.sh"

  LEGACY_GOST_BINARY=$(mktemp)
  LEGACY_GOST_SERVICE_FILE_ETC=$(mktemp)
  LEGACY_GOST_SERVICE_FILE_LIB=$(mktemp -u)
  LEGACY_GOST_SERVICE_FILE_USR_LIB=$(mktemp -u)
  LEGACY_GOST_CONFIG_DIR=$(mktemp -d)
  cat > "$LEGACY_GOST_SERVICE_FILE_ETC" <<EOF
[Unit]
Description=Gost Proxy Service

[Service]
WorkingDirectory=$LEGACY_GOST_CONFIG_DIR
ExecStart=$LEGACY_GOST_CONFIG_DIR/gost
EOF
  : > "$LEGACY_GOST_CONFIG_DIR/config.json"
  : > "$LEGACY_GOST_CONFIG_DIR/gost.json"

  local systemctl_calls=""

  systemctl() {
    systemctl_calls+=$'\n'"$*"
    if [[ "$1" == "list-units" ]]; then
      printf 'gost.service loaded active running\n'
    fi
    return 0
  }

  cleanup_legacy_gost_installation >/dev/null

  if [[ -e "$LEGACY_GOST_BINARY" ]]; then
    fail "cleanup_legacy_gost_installation should remove the legacy gost binary"
  fi
  if [[ -e "$LEGACY_GOST_SERVICE_FILE_ETC" ]]; then
    fail "cleanup_legacy_gost_installation should remove the legacy gost service file"
  fi
  [[ "$systemctl_calls" == *"stop gost"* ]] || fail "cleanup_legacy_gost_installation should stop the legacy gost service"
  [[ "$systemctl_calls" == *"disable gost"* ]] || fail "cleanup_legacy_gost_installation should disable the legacy gost service"
  [[ "$systemctl_calls" == *"daemon-reload"* ]] || fail "cleanup_legacy_gost_installation should reload systemd after removing the legacy service"
)

test_cleanup_legacy_gost_installation_preserves_unrelated_gost() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/install.sh"

  LEGACY_GOST_BINARY=$(mktemp)
  LEGACY_GOST_SERVICE_FILE_ETC=$(mktemp)
  LEGACY_GOST_SERVICE_FILE_LIB=$(mktemp -u)
  LEGACY_GOST_SERVICE_FILE_USR_LIB=$(mktemp -u)
  LEGACY_GOST_CONFIG_DIR=$(mktemp -d)
  cat > "$LEGACY_GOST_SERVICE_FILE_ETC" <<'EOF'
[Unit]
Description=Unrelated Gost Service

[Service]
WorkingDirectory=/srv/custom-gost
ExecStart=/usr/local/bin/gost -C /srv/custom-gost/gost.yaml
EOF

  local systemctl_calls=""

  systemctl() {
    systemctl_calls+=$'\n'"$*"
    if [[ "$1" == "list-units" ]]; then
      printf 'gost.service loaded active running\n'
    fi
    return 0
  }

  cleanup_legacy_gost_installation >/dev/null

  [[ -e "$LEGACY_GOST_BINARY" ]] || fail "cleanup_legacy_gost_installation should preserve unrelated gost binaries"
  [[ -e "$LEGACY_GOST_SERVICE_FILE_ETC" ]] || fail "cleanup_legacy_gost_installation should preserve unrelated gost service files"
  [[ "$systemctl_calls" != *"stop gost"* ]] || fail "cleanup_legacy_gost_installation should not stop unrelated gost services"
  [[ "$systemctl_calls" != *"disable gost"* ]] || fail "cleanup_legacy_gost_installation should not disable unrelated gost services"
)

test_install_script_accepts_proxy_url_env_without_prompt() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/install.sh"

  PROXY_ENABLED=""
  PROXY_URL="mirror.example.com"

  ask_proxy_config >/dev/null < <(printf '\n\n')

  assert_equals "true" "$PROXY_ENABLED" "install.sh should treat PROXY_URL as enabling the proxy"
  assert_equals "mirror.example.com" "$PROXY_URL" "install.sh should preserve PROXY_URL when provided via env"
)

test_panel_install_script_can_disable_proxy() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/panel_install.sh"

  PROXY_ENABLED=""
  PROXY_URL=""

  ask_proxy_config >/dev/null < <(printf 'n\n')

  assert_equals "false" "$PROXY_ENABLED" "panel_install.sh should allow disabling proxy"

  local actual
  actual=$(maybe_proxy_url "https://github.com/example/release")

  assert_equals \
    "https://github.com/example/release" \
    "$actual" \
    "panel_install.sh should bypass the proxy after disabling it"
)

test_panel_install_script_recomputes_compose_urls_after_prompt() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/panel_install.sh"

  PROXY_ENABLED=""
  PROXY_URL=""
  DOCKER_COMPOSEV4_URL=""
  DOCKER_COMPOSEV6_URL=""

  ask_proxy_config >/dev/null < <(printf 'n\n')
  ensure_compose_urls_initialized

  assert_equals \
    "https://github.com/${REPO}/releases/download/${RESOLVED_VERSION}/docker-compose-v4.yml" \
    "$DOCKER_COMPOSEV4_URL" \
    "panel_install.sh should build the compose URL after the interactive proxy choice"
)

test_update_panel_asks_for_proxy_config() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/panel_install.sh"

  local ask_called="0"

  ask_proxy_config() {
    ask_called="1"
    PROXY_ENABLED="false"
    DOCKER_COMPOSEV4_URL=""
    DOCKER_COMPOSEV6_URL=""
  }

  check_docker() {
    DOCKER_CMD="true"
  }

  get_current_db_type() {
    echo "sqlite"
  }

  resolve_latest_release_tag() {
    echo "v-test"
  }

  upsert_env_var() { :; }
  check_ipv6_support() { return 1; }
  configure_docker_ipv6() { :; }
  docker() { return 0; }
  wait_for_backend_healthy() { return 0; }
  sleep() { :; }
  curl() { :; }

  update_panel >/dev/null

  assert_equals "1" "$ask_called" "update_panel should ask for proxy config before downloading"
  assert_equals \
    "https://github.com/${REPO}/releases/download/v-test/docker-compose-v4.yml" \
    "$DOCKER_COMPOSEV4_URL" \
    "update_panel should honor the prompted proxy choice"
)

test_panel_install_script_accepts_proxy_url_env_without_prompt() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/panel_install.sh"

  PROXY_ENABLED=""
  PROXY_URL="mirror.example.com"

  ask_proxy_config >/dev/null < <(printf '\n\n')

  assert_equals "true" "$PROXY_ENABLED" "panel_install.sh should treat PROXY_URL as enabling the proxy"
  assert_equals "mirror.example.com" "$PROXY_URL" "panel_install.sh should preserve PROXY_URL when provided via env"
)

test_panel_install_script_defaults_proxy_on_eof() {
  local rc="0"
  local output
  local temp_script

  temp_script=$(mktemp)
  sed '/^# 执行主函数$/,$d' "$ROOT_DIR/panel_install.sh" > "$temp_script"
  cat >> "$temp_script" <<'EOF'
PROXY_ENABLED=""
PROXY_URL=""
ask_proxy_config >/dev/null < /dev/null
printf '%s\n%s\n' "$PROXY_ENABLED" "$PROXY_URL"
EOF

  output=$(bash "$temp_script") || rc="$?"
  rm -f "$temp_script"

  assert_equals "0" "$rc" "panel_install.sh should not fail when proxy prompt receives EOF"
  assert_equals $'true\ngcode.hostcentral.cc' "$output" "panel_install.sh should fall back to the default proxy on EOF"
}

test_panel_install_script_uses_default_proxy() (
  set -euo pipefail
  load_script_without_main "$ROOT_DIR/panel_install.sh"

  PROXY_ENABLED="true"
  PROXY_URL=""

  local actual
  actual=$(maybe_proxy_url "https://github.com/example/release")

  assert_equals \
    "https://gcode.hostcentral.cc/https://github.com/example/release" \
    "$actual" \
    "panel_install.sh should keep the default proxy when enabled"
)

test_install_script_respects_disabled_proxy
test_install_script_asks_for_proxy_config
test_install_script_recomputes_download_url_after_prompt
test_update_flux_agent_asks_for_proxy_config
test_update_flux_agent_skips_proxy_prompt_when_not_installed
test_install_flux_agent_preserves_legacy_gost_when_download_fails
test_update_flux_agent_preserves_legacy_gost_when_download_fails
test_install_flux_agent_writes_json_safe_config
test_cleanup_legacy_gost_installation_removes_service_and_binary
test_cleanup_legacy_gost_installation_preserves_unrelated_gost
test_install_script_accepts_proxy_url_env_without_prompt
test_panel_install_script_can_disable_proxy
test_panel_install_script_recomputes_compose_urls_after_prompt
test_update_panel_asks_for_proxy_config
test_panel_install_script_uses_default_proxy
test_panel_install_script_accepts_proxy_url_env_without_prompt
test_panel_install_script_defaults_proxy_on_eof

echo "install script proxy tests passed"
