#!/bin/sh
set -eu

# Run only in a disposable Alpine container: this exercises real OpenRC services.
# docker run --rm -v "$PWD:/workspace:ro" alpine:3.22 sh /workspace/test-install-scripts-alpine.sh
[ -f /etc/alpine-release ] && [ "$(id -u)" = 0 ] || {
  echo "Run this test as root in a disposable Alpine container." >&2
  exit 1
}

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
INSTALL_SCRIPT=${1:-"$ROOT_DIR/install.sh"}
TEST_DIR=$(mktemp -d)
export TEST_DIR

cleanup() {
  rc-service flux_agent stop >/dev/null 2>&1 || true
  rc-update del flux_agent default >/dev/null 2>&1 || true
  rm -f /etc/init.d/flux_agent
  rm -rf "$TEST_DIR"
}
trap cleanup EXIT

# Docker supplies the network; an empty interfaces file lets OpenRC register
# that dependency without changing the container's network configuration.
apk add --no-cache openrc
mkdir -p /run/openrc /etc/network
touch /run/openrc/softlevel /etc/network/interfaces
rc-service networking start

# Fail immediately if any path accidentally calls systemd on Alpine.
mkdir -p "$TEST_DIR/bin"
cat > "$TEST_DIR/bin/systemctl" <<'EOF'
#!/bin/sh
touch "$TEST_DIR/systemctl-called"
exit 1
EOF
chmod +x "$TEST_DIR/bin/systemctl"
export PATH="$TEST_DIR/bin:$PATH"

cat > "$TEST_DIR/agent" <<'EOF'
#!/bin/sh
if [ "${1:-}" = -V ]; then
  echo "Alpine installer test agent"
  exit 0
fi
pwd > ../working-directory
exec sleep 300
EOF

# Keep the installer bootstrap, service detection and all lifecycle operations.
# Substitute only the network download and optional tcpkill dependency.
sed '/^# 执行主函数$/,$d' "$INSTALL_SCRIPT" > "$TEST_DIR/install.sh"
cat >> "$TEST_DIR/install.sh" <<'EOF'
INSTALL_DIR="$TEST_DIR/flux_agent"
ensure_download_url_initialized() {
  ensure_alpine_runtime_dependencies || return 1
  DOWNLOAD_URL="file://$TEST_DIR/agent"
}
check_and_install_tcpkill() { :; }
main
EOF
chmod +x "$TEST_DIR/install.sh"
cp "$TEST_DIR/install.sh" "$TEST_DIR/manage.sh"
PROXY_ENABLED=false "$TEST_DIR/install.sh" -a http://127.0.0.1:9 -s alpine-test

rc-service flux_agent status
test -L /etc/runlevels/default/flux_agent
test "$(cat "$TEST_DIR/working-directory")" = "$TEST_DIR/flux_agent"
test -s "$TEST_DIR/flux_agent/config.json"
rc-service flux_agent restart
rc-service flux_agent status

# Update must retain config and restart through OpenRC.
cp "$TEST_DIR/flux_agent/config.json" "$TEST_DIR/config-before-update.json"
cp "$TEST_DIR/manage.sh" "$TEST_DIR/update.sh"
printf '2\n' | PROXY_ENABLED=false "$TEST_DIR/update.sh"
cmp "$TEST_DIR/config-before-update.json" "$TEST_DIR/flux_agent/config.json"
rc-service flux_agent status

printf '3\ny\n' | "$TEST_DIR/manage.sh"
test ! -e /etc/init.d/flux_agent
test ! -e /etc/runlevels/default/flux_agent
test ! -e "$TEST_DIR/flux_agent"
test ! -e "$TEST_DIR/systemctl-called"
echo "Alpine installer lifecycle tests passed"
