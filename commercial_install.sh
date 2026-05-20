#!/bin/bash
set -euo pipefail

INSTALL_DIR="${FLVX_INSTALL_DIR:-/opt/flvx-panel}"
REQUESTED_BACKEND_PORT="${FLVX_BACKEND_PORT:-}"
REQUESTED_FRONTEND_PORT="${FLVX_FRONTEND_PORT:-}"
BACKEND_PORT="${REQUESTED_BACKEND_PORT:-6365}"
FRONTEND_PORT="${REQUESTED_FRONTEND_PORT:-80}"
TARGET_VERSION="${FLVX_TARGET_VERSION:-commercial-v1}"
LICENSE_DOMAIN="${FLVX_LICENSE_DOMAIN:-}"
SSL_EMAIL="${FLVX_SSL_EMAIL:-}"
CACHE_DIR="$INSTALL_DIR/.cache"
STATE_FILE="$INSTALL_DIR/.flvx-install-state"

log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

fail() {
  log "安装失败：$*"
  exit 1
}

require_env() {
  local name="$1"
  if [ -z "${!name:-}" ]; then
    fail "缺少环境变量 $name"
  fi
}

random_hex() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
    return
  fi
  head -c 48 /dev/urandom | sha256sum | awk '{print $1}'
}

normalize_domain() {
  local domain="$1"
  domain="${domain#http://}"
  domain="${domain#https://}"
  domain="${domain%%/*}"
  domain="${domain%%:*}"
  printf '%s' "$domain" | tr '[:upper:]' '[:lower:]'
}

host_port() {
  local value="$1"
  printf '%s' "${value##*:}"
}

apply_domain_defaults() {
  LICENSE_DOMAIN="$(normalize_domain "$LICENSE_DOMAIN")"
  if [ -z "$LICENSE_DOMAIN" ]; then
    return
  fi
  if ! printf '%s' "$LICENSE_DOMAIN" | grep -Eq '^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?)+$'; then
    fail "授权域名格式无效：$LICENSE_DOMAIN"
  fi
  if [ -z "$REQUESTED_BACKEND_PORT" ]; then
    BACKEND_PORT="127.0.0.1:6365"
  fi
  if [ -z "$REQUESTED_FRONTEND_PORT" ]; then
    FRONTEND_PORT="127.0.0.1:6366"
  fi
}

mark_state() {
  mkdir -p "$INSTALL_DIR"
  echo "$(date '+%Y-%m-%d %H:%M:%S') $*" >> "$STATE_FILE"
}

get_env_value() {
  local name="$1"
  if [ ! -f "$INSTALL_DIR/.env" ]; then
    return 0
  fi
  grep -E "^${name}=" "$INSTALL_DIR/.env" | tail -n 1 | cut -d= -f2-
}

upsert_env_value() {
  local name="$1"
  local value="$2"
  local env_file="$INSTALL_DIR/.env"
  local tmp_file
  tmp_file="$(mktemp /tmp/flvx-env.XXXXXX)"
  if [ -f "$env_file" ]; then
    awk -v key="$name" -v val="$value" '
      BEGIN { done = 0 }
      $0 ~ "^" key "=" { print key "=" val; done = 1; next }
      { print }
      END { if (!done) print key "=" val }
    ' "$env_file" > "$tmp_file"
  else
    echo "$name=$value" > "$tmp_file"
  fi
  mv "$tmp_file" "$env_file"
  chmod 600 "$env_file"
}

wait_apt_locks() {
  local waited=0
  while fuser /var/lib/dpkg/lock-frontend /var/lib/dpkg/lock /var/cache/apt/archives/lock /var/lib/apt/lists/lock >/dev/null 2>&1; do
    if [ "$waited" -ge 300 ]; then
      fail "等待 apt/dpkg 锁超时，请确认系统没有其他安装任务"
    fi
    log "检测到 apt/dpkg 正在运行，等待锁释放..."
    sleep 5
    waited=$((waited + 5))
  done
}

apt_update() {
  wait_apt_locks
  apt-get update
}

apt_install() {
  wait_apt_locks
  apt-get install -y "$@"
}

check_system() {
  if [ "$(id -u)" != "0" ]; then
    fail "必须使用 root 用户安装"
  fi
  if [ ! -f /etc/os-release ]; then
    fail "无法识别系统版本"
  fi
  . /etc/os-release
  if [ "${ID:-}" != "debian" ] || [ "${VERSION_ID:-}" != "12" ]; then
    fail "仅支持干净 Debian 12 系统"
  fi
}

prepare_install_dir() {
  mkdir -p "$INSTALL_DIR"
  if [ "$(find "$INSTALL_DIR" -mindepth 1 -maxdepth 1 2>/dev/null | wc -l)" -gt 0 ]; then
    if [ -f "$INSTALL_DIR/.env" ] || [ -f "$INSTALL_DIR/docker-compose.yml" ] || [ -f "$STATE_FILE" ]; then
      log "检测到上次安装目录，进入续装模式"
    else
      fail "$INSTALL_DIR 不是空目录，无法确认是否为 FLVX 安装残留"
    fi
  fi
  mkdir -p "$CACHE_DIR"
  mark_state "prepare"
}

ensure_packages() {
  export DEBIAN_FRONTEND=noninteractive
  if ! command -v curl >/dev/null 2>&1 || ! command -v sha256sum >/dev/null 2>&1; then
    apt_update
    apt_install ca-certificates curl coreutils
  fi
  if ! command -v docker >/dev/null 2>&1; then
    apt_update
    apt_install docker.io
  fi
  if ! docker compose version >/dev/null 2>&1 && ! command -v docker-compose >/dev/null 2>&1; then
    apt_update
    apt_install docker-compose-plugin || apt_install docker-compose
  fi
  if [ -n "$LICENSE_DOMAIN" ] && { ! command -v nginx >/dev/null 2>&1 || ! command -v certbot >/dev/null 2>&1; }; then
    apt_update
    apt_install nginx certbot python3-certbot-nginx
  fi
  mark_state "packages"
}

ensure_docker_ready() {
  if docker info >/dev/null 2>&1; then
    return
  fi
  if command -v systemctl >/dev/null 2>&1; then
    systemctl start docker >/dev/null 2>&1 || true
  fi
  if ! docker info >/dev/null 2>&1 && command -v service >/dev/null 2>&1; then
    service docker start >/dev/null 2>&1 || true
  fi
  for _ in $(seq 1 30); do
    if docker info >/dev/null 2>&1; then
      mark_state "docker-ready"
      return
    fi
    sleep 2
  done
  fail "Docker 服务未启动"
}

check_ipv6_support() {
  ip -6 addr show 2>/dev/null | grep -v "scope link" | grep -q "inet6"
}

select_compose() {
  if check_ipv6_support && [ -n "${FLVX_COMPOSE_V6_URL:-}" ]; then
    COMPOSE_URL="$FLVX_COMPOSE_V6_URL"
    COMPOSE_SHA256="$FLVX_COMPOSE_V6_SHA256"
    return
  fi
  COMPOSE_URL="$FLVX_COMPOSE_V4_URL"
  COMPOSE_SHA256="$FLVX_COMPOSE_V4_SHA256"
}

write_env_file() {
  INSTALL_TOKEN="$(get_env_value FLVX_INSTALL_TOKEN)"
  if [ -z "$INSTALL_TOKEN" ]; then
    INSTALL_TOKEN="$(random_hex)"
  fi
  JWT_SECRET="$(get_env_value JWT_SECRET)"
  if [ -z "$JWT_SECRET" ]; then
    JWT_SECRET="$(random_hex)"
  fi
  upsert_env_value BACKEND_PORT "$BACKEND_PORT"
  upsert_env_value FRONTEND_PORT "$FRONTEND_PORT"
  upsert_env_value DB_TYPE sqlite
  upsert_env_value JWT_SECRET "$JWT_SECRET"
  upsert_env_value FLUX_VERSION "$TARGET_VERSION"
  upsert_env_value FLVX_INSTALL_TOKEN "$INSTALL_TOKEN"
  if [ -n "$LICENSE_DOMAIN" ]; then
    upsert_env_value PANEL_DOMAIN "$LICENSE_DOMAIN"
  fi
  mark_state "env"
}

download_compose() {
  if [ -f "$INSTALL_DIR/docker-compose.yml" ] && echo "$COMPOSE_SHA256  $INSTALL_DIR/docker-compose.yml" | sha256sum -c - >/dev/null 2>&1; then
    log "compose 文件已存在且校验通过，跳过下载"
    mark_state "compose-reused"
    return
  fi
  cache_compose="$CACHE_DIR/docker-compose-$COMPOSE_SHA256.yml"
  if [ ! -f "$cache_compose" ] || ! echo "$COMPOSE_SHA256  $cache_compose" | sha256sum -c - >/dev/null 2>&1; then
    tmp_compose="$(mktemp /tmp/flvx-compose.XXXXXX.yml)"
    curl -fsSL "$COMPOSE_URL" -o "$tmp_compose"
    echo "$COMPOSE_SHA256  $tmp_compose" | sha256sum -c -
    mv "$tmp_compose" "$cache_compose"
  fi
  cp "$cache_compose" "$INSTALL_DIR/docker-compose.yml"
  mark_state "compose"
}

download_and_build_package() {
  if [ -z "${FLVX_PACKAGE_URL:-}" ]; then
    return 0
  fi
  if [ -z "${FLVX_PACKAGE_SHA256:-}" ]; then
    fail "已配置商业包下载地址，但缺少 FLVX_PACKAGE_SHA256"
  fi
  backend_image="local/flvx-panel-backend:$TARGET_VERSION"
  frontend_image="local/flvx-panel-frontend:$TARGET_VERSION"
  upsert_env_value FLVX_BACKEND_IMAGE "$backend_image"
  upsert_env_value FLVX_FRONTEND_IMAGE "$frontend_image"
  if docker image inspect "$backend_image" >/dev/null 2>&1 && docker image inspect "$frontend_image" >/dev/null 2>&1; then
    log "商业镜像已存在，跳过构建"
    mark_state "images-reused"
    return 0
  fi
  package_file="$CACHE_DIR/flvx-package-$FLVX_PACKAGE_SHA256.tar.gz"
  package_dir="$(mktemp -d /tmp/flvx-package.XXXXXX)"
  if [ ! -f "$package_file" ] || ! echo "$FLVX_PACKAGE_SHA256  $package_file" | sha256sum -c - >/dev/null 2>&1; then
    tmp_package="$(mktemp /tmp/flvx-package.XXXXXX.tar.gz)"
    curl -fsSL "$FLVX_PACKAGE_URL" -o "$tmp_package"
    echo "$FLVX_PACKAGE_SHA256  $tmp_package" | sha256sum -c -
    mv "$tmp_package" "$package_file"
  else
    log "商业包已存在且校验通过，跳过下载"
  fi
  tar -xzf "$package_file" -C "$package_dir"
  if ! docker image inspect "$backend_image" >/dev/null 2>&1; then
    docker build -t "$backend_image" "$package_dir/go-backend"
  else
    log "后端镜像已存在，跳过构建"
  fi
  if ! docker image inspect "$frontend_image" >/dev/null 2>&1; then
    docker build -t "$frontend_image" --build-arg VITE_BASE_PATH=/ "$package_dir/vite-frontend"
  else
    log "前端镜像已存在，跳过构建"
  fi
  rm -rf "$package_dir"
  mark_state "images"
}

pull_runtime_images() {
  if [ -n "${FLVX_PACKAGE_URL:-}" ]; then
    if docker image inspect postgres:16-alpine >/dev/null 2>&1; then
      log "PostgreSQL 镜像已存在，跳过拉取"
      mark_state "postgres-reused"
      return
    fi
    docker_compose pull postgres
    mark_state "postgres"
    return
  fi
  docker_compose pull
  mark_state "pull"
}

docker_compose() {
  if docker compose version >/dev/null 2>&1; then
    docker compose "$@"
  else
    docker-compose "$@"
  fi
}

wait_backend() {
  backend_port="$(host_port "$BACKEND_PORT")"
  for _ in $(seq 1 60); do
    if curl -fsS "http://127.0.0.1:$backend_port/flow/test" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  return 1
}

activate_license() {
  backend_port="$(host_port "$BACKEND_PORT")"
  status="$(curl -fsS -X POST -H "Content-Type: application/json" --data '{}' "http://127.0.0.1:$backend_port/api/v1/license/local/status" 2>/dev/null || true)"
  if echo "$status" | grep -q '"valid"[[:space:]]*:[[:space:]]*true'; then
    log "商业授权已激活，跳过激活"
    mark_state "license-reused"
    return
  fi
  payload=$(cat <<EOF
{"centerUrl":"$FLVX_OFFICIAL_URL","publicKey":"$FLVX_OFFICIAL_PUBLIC_KEY","licenseKey":"$FLVX_LICENSE_KEY","token":"$INSTALL_TOKEN"}
EOF
)
  response="$(curl -fsS -X POST -H "Content-Type: application/json" --data "$payload" "http://127.0.0.1:$backend_port/api/v1/license/local/bootstrap")"
  echo "$response" | grep -q '"code"[[:space:]]*:[[:space:]]*0' || fail "授权激活失败：$response"
  mark_state "license"
}

configure_domain_proxy() {
  if [ -z "$LICENSE_DOMAIN" ]; then
    return
  fi
  frontend_port="$(host_port "$FRONTEND_PORT")"
  cat > /etc/nginx/sites-available/flvx-panel.conf <<EOF
server {
    listen 80;
    listen [::]:80;
    server_name $LICENSE_DOMAIN;

    client_max_body_size 50m;

    location / {
        proxy_pass http://127.0.0.1:$frontend_port;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
    }
}
EOF
  rm -f /etc/nginx/sites-enabled/default
  ln -sf /etc/nginx/sites-available/flvx-panel.conf /etc/nginx/sites-enabled/flvx-panel.conf
  nginx -t
  if command -v systemctl >/dev/null 2>&1; then
    systemctl enable nginx >/dev/null 2>&1 || true
    systemctl reload nginx >/dev/null 2>&1 || systemctl restart nginx
  else
    service nginx reload >/dev/null 2>&1 || service nginx restart
  fi
  if [ -f "/etc/letsencrypt/live/$LICENSE_DOMAIN/fullchain.pem" ]; then
    log "SSL 证书已存在，跳过申请"
    mark_state "ssl-reused"
    return
  fi
  email_args="--register-unsafely-without-email"
  if [ -n "$SSL_EMAIL" ]; then
    email_args="-m $SSL_EMAIL --no-eff-email"
  fi
  certbot --nginx -d "$LICENSE_DOMAIN" --non-interactive --agree-tos $email_args --redirect
  mark_state "ssl"
}

main() {
  require_env FLVX_LICENSE_KEY
  require_env FLVX_OFFICIAL_URL
  require_env FLVX_OFFICIAL_PUBLIC_KEY
  require_env FLVX_COMPOSE_V4_URL
  require_env FLVX_COMPOSE_V4_SHA256

  log "开始安装 FLVX 商业包"
  apply_domain_defaults
  check_system
  ensure_packages
  ensure_docker_ready
  select_compose

  prepare_install_dir

  write_env_file
  download_compose
  download_and_build_package

  cd "$INSTALL_DIR"
  log "拉取并启动容器"
  pull_runtime_images
  docker_compose up -d

  log "等待后端服务启动"
  wait_backend || fail "后端服务启动超时"

  log "激活商业授权"
  activate_license

  log "配置授权域名和 HTTPS"
  configure_domain_proxy

  log "安装完成，前端端口：$FRONTEND_PORT，后端端口：$BACKEND_PORT"
  mark_state "done"
}

main "$@"
