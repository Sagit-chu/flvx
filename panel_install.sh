#!/bin/bash
set -e

# 解决 macOS 下 tr 可能出现的非法字节序列问题
export LANG=en_US.UTF-8
export LC_ALL=C



# GitHub repo used for release downloads
REPO="Sagit-chu/flux-panel"

# 固定版本号（Release 构建时自动填充，留空则获取最新版）
PINNED_VERSION=""

# 镜像加速配置（可由面板传入或交互式询问）
PROXY_ENABLED="${PROXY_ENABLED:-}"
PROXY_URL="${PROXY_URL:-}"
DEFAULT_PANEL_BACKEND_CONTAINER="flux-panel-backend"
DEFAULT_PANEL_POSTGRES_CONTAINER="flux-panel-postgres"
PANEL_SCRIPT_PATH="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"

# 镜像加速
maybe_proxy_url() {
  local url="$1"

  if [[ "$PROXY_ENABLED" == "false" ]]; then
    echo "$url"
    return
  fi

  local proxy="${PROXY_URL:-gcode.hostcentral.cc}"

  if [[ "$proxy" == https://* || "$proxy" == http://* ]]; then
    proxy="${proxy%/}"
  else
    proxy="https://${proxy%/}"
  fi

  echo "${proxy}/${url}"
}

ask_proxy_config() {
  if [[ -n "$PROXY_ENABLED" ]]; then
    return
  fi

  if [[ -n "$PROXY_URL" ]]; then
    PROXY_ENABLED="true"
    return
  fi

  echo ""
  echo "==============================================="
  echo "           GitHub 加速配置"
  echo "==============================================="
  if ! read -r -p "是否开启 GitHub 加速? (Y/n): " proxy_choice; then
    proxy_choice=""
  fi
  case "$proxy_choice" in
    n|N)
      PROXY_ENABLED="false"
      echo "已关闭加速，将直连 GitHub"
      ;;
    *)
      PROXY_ENABLED="true"
      if ! read -r -p "加速地址 (默认 gcode.hostcentral.cc): " input_url; then
        input_url=""
      fi
      PROXY_URL="${input_url:-gcode.hostcentral.cc}"
      echo "已开启加速: $PROXY_URL"
      ;;
  esac
  echo "==============================================="
}

resolve_latest_release_tag() {
  local effective_url tag api_tag latest_url api_url

  latest_url="https://github.com/${REPO}/releases/latest"
  api_url="https://api.github.com/repos/${REPO}/releases/latest"

  effective_url=$(curl -fsSL -o /dev/null -w '%{url_effective}' -L "$(maybe_proxy_url "$latest_url")" 2>/dev/null || true)
  tag="${effective_url##*/}"
  if [[ -n "$tag" && "$tag" != "latest" ]]; then
    echo "$tag"
    return 0
  fi

  api_tag=$(curl -fsSL "$(maybe_proxy_url "$api_url")" 2>/dev/null | grep -m1 '"tag_name"' | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/' || true)
  if [[ -n "$api_tag" ]]; then
    echo "$api_tag"
    return 0
  fi

  return 1
}

resolve_version() {
  if [[ -n "${VERSION:-}" ]]; then
    echo "$VERSION"
    return 0
  fi
  if [[ -n "${FLUX_VERSION:-}" ]]; then
    echo "$FLUX_VERSION"
    return 0
  fi
  if [[ -n "${PINNED_VERSION:-}" ]]; then
    echo "$PINNED_VERSION"
    return 0
  fi

  if resolve_latest_release_tag; then
    return 0
  fi

  echo "❌ 无法获取最新版本号。你可以手动指定版本，例如：VERSION=<版本号> ./panel_install.sh" >&2
  return 1
}

# 根据版本号设置 compose 下载地址
set_compose_urls_by_version() {
  local version="$1"
  DOCKER_COMPOSEV4_URL=$(maybe_proxy_url "https://github.com/${REPO}/releases/download/${version}/docker-compose-v4.yml")
  DOCKER_COMPOSEV6_URL=$(maybe_proxy_url "https://github.com/${REPO}/releases/download/${version}/docker-compose-v6.yml")
}

ensure_compose_urls_initialized() {
  if [[ -n "${DOCKER_COMPOSEV4_URL:-}" && -n "${DOCKER_COMPOSEV6_URL:-}" ]]; then
    return 0
  fi

  RESOLVED_VERSION=$(resolve_version) || return 1
  set_compose_urls_by_version "$RESOLVED_VERSION"
}



# 根据IPv6支持情况选择docker-compose URL
get_docker_compose_url() {
  if check_ipv6_support > /dev/null 2>&1; then
    echo "$DOCKER_COMPOSEV6_URL"
  else
    echo "$DOCKER_COMPOSEV4_URL"
  fi
}

# 检查 docker-compose 或 docker compose 命令
check_docker() {
  if command -v docker-compose &> /dev/null; then
    DOCKER_CMD="docker-compose"
  elif command -v docker &> /dev/null; then
    if docker compose version &> /dev/null; then
      DOCKER_CMD="docker compose"
    else
      echo "错误：检测到 docker，但不支持 'docker compose' 命令。请安装 docker-compose 或更新 docker 版本。"
      exit 1
    fi
  else
    echo "错误：未检测到 docker 或 docker-compose 命令。请先安装 Docker。"
    exit 1
  fi
  echo "检测到 Docker 命令：$DOCKER_CMD"
}

# 检测系统是否支持 IPv6
check_ipv6_support() {
  echo "🔍 检测 IPv6 支持..."

  # 检查是否有 IPv6 地址（排除 link-local 地址）
  if ip -6 addr show | grep -v "scope link" | grep -q "inet6"; then
    echo "✅ 检测到系统支持 IPv6"
    return 0
  elif ifconfig 2>/dev/null | grep -v "fe80:" | grep -q "inet6"; then
    echo "✅ 检测到系统支持 IPv6"
    return 0
  else
    echo "⚠️ 未检测到 IPv6 支持"
    return 1
  fi
}



# 配置 Docker 启用 IPv6
configure_docker_ipv6() {
  echo "🔧 配置 Docker IPv6 支持..."

  # 检查操作系统类型
  OS_TYPE=$(uname -s)

  if [[ "$OS_TYPE" == "Darwin" ]]; then
    # macOS 上 Docker Desktop 已默认支持 IPv6
    echo "✅ macOS Docker Desktop 默认支持 IPv6"
    return 0
  fi

  # Docker daemon 配置文件路径
  DOCKER_CONFIG="/etc/docker/daemon.json"

  # 检查是否需要 sudo
  if [[ $EUID -ne 0 ]]; then
    SUDO_CMD="sudo"
  else
    SUDO_CMD=""
  fi

  # 检查 Docker 配置文件
  if [ -f "$DOCKER_CONFIG" ]; then
    # 检查是否已经配置了 IPv6
    if grep -q '"ipv6"' "$DOCKER_CONFIG"; then
      echo "✅ Docker 已配置 IPv6 支持"
    else
      echo "📝 更新 Docker 配置以启用 IPv6..."
      # 备份原配置
      $SUDO_CMD cp "$DOCKER_CONFIG" "${DOCKER_CONFIG}.backup"

      # 使用 jq 或 sed 添加 IPv6 配置
      if command -v jq &> /dev/null; then
        $SUDO_CMD jq '. + {"ipv6": true, "fixed-cidr-v6": "fd00::/80"}' "$DOCKER_CONFIG" > /tmp/daemon.json && $SUDO_CMD mv /tmp/daemon.json "$DOCKER_CONFIG"
      else
        # 如果没有 jq，使用 sed
        $SUDO_CMD sed -i 's/^{$/{\n  "ipv6": true,\n  "fixed-cidr-v6": "fd00::\/80",/' "$DOCKER_CONFIG"
      fi

      echo "🔄 重启 Docker 服务..."
      if command -v systemctl &> /dev/null; then
        $SUDO_CMD systemctl restart docker
      elif command -v service &> /dev/null; then
        $SUDO_CMD service docker restart
      else
        echo "⚠️ 请手动重启 Docker 服务"
      fi
      sleep 5
    fi
  else
    # 创建新的配置文件
    echo "📝 创建 Docker 配置文件..."
    $SUDO_CMD mkdir -p /etc/docker
    echo '{
  "ipv6": true,
  "fixed-cidr-v6": "fd00::/80"
}' | $SUDO_CMD tee "$DOCKER_CONFIG" > /dev/null

    echo "🔄 重启 Docker 服务..."
    if command -v systemctl &> /dev/null; then
      $SUDO_CMD systemctl restart docker
    elif command -v service &> /dev/null; then
      $SUDO_CMD service docker restart
    else
      echo "⚠️ 请手动重启 Docker 服务"
    fi
    sleep 5
  fi
}

# 显示菜单
show_menu() {
  echo "==============================================="
  echo "          面板管理脚本"
  echo "==============================================="
  echo "请选择操作："
  echo "1. 安装面板"
  echo "2. 更新面板"
  echo "3. 卸载面板"
  echo "4. 迁移到 PostgreSQL"
  echo "5. 退出"
  echo "==============================================="
}

generate_random() {
  LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c16
}

upsert_env_var() {
  local file="$1"
  local key="$2"
  local value="$3"
  local tmp_file

  tmp_file=$(mktemp)
  if [ -f "$file" ]; then
    awk -v k="$key" -v v="$value" '
      BEGIN { found=0 }
      $0 ~ ("^" k "=") { print k "=" v; found=1; next }
      { print }
      END { if (!found) print k "=" v }
    ' "$file" > "$tmp_file"
  else
    printf '%s=%s\n' "$key" "$value" > "$tmp_file"
  fi

  mv "$tmp_file" "$file"
}

get_env_var() {
  local key="$1"
  local file="${2:-.env}"

  if [[ ! -f "$file" ]]; then
    return 0
  fi

  grep -m1 "^${key}=" "$file" | cut -d= -f2-
}

get_current_db_type() {
  local db_type database_url

  db_type=$(get_env_var "DB_TYPE" || true)
  database_url=$(get_env_var "DATABASE_URL" || true)

  if [[ "$db_type" == "sqlite" ]]; then
    echo "sqlite"
  elif [[ "$db_type" == "postgres" || "$database_url" == postgres://* || "$database_url" == postgresql://* ]]; then
    echo "postgres"
  else
    echo "sqlite"
  fi
}

get_container_compose_label() {
  local container="$1"
  local label="$2"
  local value

  value=$(docker inspect -f "{{ index .Config.Labels \"$label\" }}" "$container" 2>/dev/null || true)
  if [[ "$value" == "<no value>" ]]; then
    value=""
  fi
  printf '%s' "$value"
}

resolve_panel_deployment() {
  local backend_container="${PANEL_BACKEND_CONTAINER:-$DEFAULT_PANEL_BACKEND_CONTAINER}"
  local requested_dir="${PANEL_DEPLOY_DIR:-}"
  local label_dir label_project deploy_dir project_name

  label_dir=$(get_container_compose_label "$backend_container" "com.docker.compose.project.working_dir")
  label_project=$(get_container_compose_label "$backend_container" "com.docker.compose.project")

  if [[ -n "$requested_dir" ]]; then
    deploy_dir="$requested_dir"
  elif [[ -n "$label_dir" ]]; then
    deploy_dir="$label_dir"
  elif [[ -f ".env" && -f "docker-compose.yml" ]]; then
    deploy_dir=$(pwd -P)
  else
    echo "❌ 无法识别面板部署目录：未找到容器 Compose 标签，当前目录也没有完整部署配置"
    return 1
  fi

  if [[ ! -d "$deploy_dir" ]]; then
    echo "❌ 面板部署目录不存在：$deploy_dir"
    return 1
  fi
  deploy_dir=$(cd "$deploy_dir" && pwd -P)
  if [[ -n "$label_dir" && -d "$label_dir" ]]; then
    label_dir=$(cd "$label_dir" && pwd -P)
  fi
  if [[ -n "$requested_dir" && -n "$label_dir" && "$deploy_dir" != "$label_dir" ]]; then
    echo "❌ 指定部署目录与运行中容器标签不一致：$deploy_dir != $label_dir"
    return 1
  fi

  project_name="${COMPOSE_PROJECT_NAME:-}"
  if [[ -z "$project_name" && -n "$label_project" ]]; then
    project_name="$label_project"
  fi
  if [[ -n "$project_name" && ! "$project_name" =~ ^[a-z0-9][a-z0-9_-]*$ ]]; then
    echo "❌ Compose 项目名不合法：$project_name"
    return 1
  fi

  PANEL_DEPLOY_DIR="$deploy_dir"
  PANEL_BACKEND_CONTAINER="$backend_container"
  PANEL_POSTGRES_CONTAINER="${PANEL_POSTGRES_CONTAINER:-$DEFAULT_PANEL_POSTGRES_CONTAINER}"
  PANEL_COMPOSE_PROJECT="$project_name"
  export PANEL_DEPLOY_DIR PANEL_BACKEND_CONTAINER PANEL_POSTGRES_CONTAINER PANEL_COMPOSE_PROJECT
  if [[ -n "$project_name" ]]; then
    COMPOSE_PROJECT_NAME="$project_name"
    export COMPOSE_PROJECT_NAME
  fi

  cd "$PANEL_DEPLOY_DIR"
  echo "📁 部署目录：$PANEL_DEPLOY_DIR"
  if [[ -n "$PANEL_COMPOSE_PROJECT" ]]; then
    echo "📦 Compose 项目：$PANEL_COMPOSE_PROJECT"
  fi
}

run_panel_compose() {
  $DOCKER_CMD "$@"
}

validate_panel_update_environment() {
  local key value backend_port frontend_port configured_db_type db_type database_url postgres_password

  if [[ ! -f ".env" ]]; then
    echo "❌ 部署目录缺少 .env，更新终止"
    return 1
  fi
  if [[ ! -f "docker-compose.yml" ]]; then
    echo "❌ 部署目录缺少 docker-compose.yml，更新终止"
    return 1
  fi

  for key in JWT_SECRET BACKEND_PORT FRONTEND_PORT; do
    value=$(get_env_var "$key" || true)
    if [[ -z "${value//[[:space:]]/}" ]]; then
      echo "❌ .env 中 $key 缺失或为空，更新终止"
      return 1
    fi
  done

  backend_port=$(get_env_var "BACKEND_PORT" || true)
  frontend_port=$(get_env_var "FRONTEND_PORT" || true)
  if [[ ! "$backend_port" =~ ^[0-9]+$ || "$backend_port" -lt 1 || "$backend_port" -gt 65535 ]]; then
    echo "❌ BACKEND_PORT 不是有效端口：$backend_port"
    return 1
  fi
  if [[ ! "$frontend_port" =~ ^[0-9]+$ || "$frontend_port" -lt 1 || "$frontend_port" -gt 65535 ]]; then
    echo "❌ FRONTEND_PORT 不是有效端口：$frontend_port"
    return 1
  fi

  configured_db_type=$(get_env_var "DB_TYPE" || true)
  if [[ -n "$configured_db_type" && "$configured_db_type" != "sqlite" && "$configured_db_type" != "postgres" ]]; then
    echo "❌ DB_TYPE 仅支持 sqlite 或 postgres：$configured_db_type"
    return 1
  fi
  db_type=$(get_current_db_type)
  if [[ "$db_type" == "postgres" ]]; then
    database_url=$(get_env_var "DATABASE_URL" || true)
    postgres_password=$(get_env_var "POSTGRES_PASSWORD" || true)
    if [[ -z "${database_url//[[:space:]]/}" || -z "${postgres_password//[[:space:]]/}" ]]; then
      echo "❌ PostgreSQL 模式要求 DATABASE_URL 和 POSTGRES_PASSWORD 均非空"
      return 1
    fi
  fi

  if ! run_panel_compose -f docker-compose.yml config -q; then
    echo "❌ 当前 Compose 配置校验失败，更新终止"
    return 1
  fi
}

download_panel_update_compose() {
  local compose_url="$1"

  UPDATE_COMPOSE_CANDIDATE=$(mktemp "$PANEL_DEPLOY_DIR/.docker-compose.yml.update.XXXXXX")
  if ! curl -fL -o "$UPDATE_COMPOSE_CANDIDATE" "$compose_url"; then
    rm -f "$UPDATE_COMPOSE_CANDIDATE"
    UPDATE_COMPOSE_CANDIDATE=""
    echo "❌ 下载最新 Compose 配置失败，更新终止"
    return 1
  fi
  if [[ ! -s "$UPDATE_COMPOSE_CANDIDATE" ]]; then
    rm -f "$UPDATE_COMPOSE_CANDIDATE"
    UPDATE_COMPOSE_CANDIDATE=""
    echo "❌ 下载的 Compose 配置为空，更新终止"
    return 1
  fi
}

validate_panel_update_compose() {
  if ! FLUX_VERSION="$LATEST_VERSION" run_panel_compose -f "$UPDATE_COMPOSE_CANDIDATE" config -q; then
    echo "❌ 新 Compose 配置校验失败，更新终止"
    return 1
  fi
}

backup_sqlite_for_update() (
  local destination="$1"
  local running paused="false"

  cleanup_paused_backend() {
    if [[ "$paused" == "true" ]]; then
      docker unpause "$PANEL_BACKEND_CONTAINER" >/dev/null 2>&1 || true
    fi
  }
  trap cleanup_paused_backend EXIT

  running=$(docker inspect -f '{{.State.Running}}' "$PANEL_BACKEND_CONTAINER" 2>/dev/null || true)
  if [[ "$running" == "true" ]]; then
    if ! docker pause "$PANEL_BACKEND_CONTAINER" >/dev/null; then
      echo "❌ 暂停后端以创建一致性 SQLite 备份失败"
      return 1
    fi
    paused="true"
  fi

  mkdir -p "$destination"
  if ! docker cp "$PANEL_BACKEND_CONTAINER:/app/data/." "$destination"; then
    echo "❌ SQLite 数据备份失败"
    return 1
  fi

  if [[ "$paused" == "true" ]] && ! docker unpause "$PANEL_BACKEND_CONTAINER" >/dev/null; then
    echo "❌ SQLite 备份完成，但恢复后端运行失败"
    return 1
  fi
  paused="false"
)

backup_postgres_for_update() {
  local destination="$1"
  local postgres_db postgres_user

  postgres_db=$(get_env_var "POSTGRES_DB" || true)
  postgres_user=$(get_env_var "POSTGRES_USER" || true)
  postgres_db=${postgres_db:-flux_panel}
  postgres_user=${postgres_user:-flux_panel}

  if ! docker exec "$PANEL_POSTGRES_CONTAINER" pg_dump -U "$postgres_user" "$postgres_db" > "$destination"; then
    rm -f "$destination"
    echo "❌ PostgreSQL 数据备份失败"
    return 1
  fi
}

create_panel_update_backup() {
  local db_type="$1"
  local timestamp

  timestamp=$(date '+%Y%m%d-%H%M%S')
  if ! (umask 077 && mkdir -p "$PANEL_DEPLOY_DIR/backups"); then
    echo "❌ 创建更新备份目录失败"
    return 1
  fi
  UPDATE_BACKUP_DIR=$(umask 077 && mktemp -d "$PANEL_DEPLOY_DIR/backups/panel-update-$timestamp.XXXXXX") || {
    echo "❌ 创建更新备份目录失败"
    return 1
  }
  if ! cp -p .env docker-compose.yml "$UPDATE_BACKUP_DIR/"; then
    echo "❌ 备份面板配置失败"
    return 1
  fi

  if [[ "$db_type" == "postgres" ]]; then
    backup_postgres_for_update "$UPDATE_BACKUP_DIR/postgres.sql" || return 1
  else
    backup_sqlite_for_update "$UPDATE_BACKUP_DIR/sqlite" || return 1
  fi
  echo "💾 更新备份：$UPDATE_BACKUP_DIR"
}

pull_panel_update_images() {
  local db_type="$1"

  if [[ "$db_type" == "postgres" ]]; then
    FLUX_VERSION="$LATEST_VERSION" run_panel_compose -f "$UPDATE_COMPOSE_CANDIDATE" pull backend frontend postgres
  else
    FLUX_VERSION="$LATEST_VERSION" run_panel_compose -f "$UPDATE_COMPOSE_CANDIDATE" pull backend frontend
  fi
}

activate_panel_update_files() {
  if ! chmod 0644 "$UPDATE_COMPOSE_CANDIDATE" || ! mv "$UPDATE_COMPOSE_CANDIDATE" docker-compose.yml; then
    echo "❌ 替换 Compose 配置失败"
    return 1
  fi
  UPDATE_COMPOSE_CANDIDATE=""
  if ! upsert_env_var ".env" "FLUX_VERSION" "$LATEST_VERSION"; then
    echo "❌ 更新版本配置失败"
    return 1
  fi
}

start_panel_after_update() {
  local db_type="$1"

  if [[ "$db_type" == "postgres" ]]; then
    run_panel_compose up -d postgres || return 1
    wait_for_postgres_healthy || return 1
  fi
  run_panel_compose up -d --force-recreate --remove-orphans backend frontend || return 1
  wait_for_backend_healthy
}

rollback_panel_update() {
  local db_type="$1"

  echo "↩️ 正在恢复更新前配置..."
  if ! cp -p "$UPDATE_BACKUP_DIR/.env" .env || ! cp -p "$UPDATE_BACKUP_DIR/docker-compose.yml" docker-compose.yml; then
    echo "❌ 配置回滚失败，请从 $UPDATE_BACKUP_DIR 手动恢复"
    return 1
  fi
  if [[ "$db_type" == "postgres" ]]; then
    run_panel_compose up -d postgres || return 1
    wait_for_postgres_healthy || return 1
  fi
  run_panel_compose up -d --force-recreate --remove-orphans backend frontend || return 1
  wait_for_backend_healthy
}

wait_for_postgres_healthy() {
  local pg_health
  local postgres_container="${PANEL_POSTGRES_CONTAINER:-$DEFAULT_PANEL_POSTGRES_CONTAINER}"

  echo "🔍 检查 PostgreSQL 服务状态..."
  for i in {1..90}; do
    if docker ps --format "{{.Names}}" | grep -Fxq "$postgres_container"; then
      pg_health=$(docker inspect -f '{{.State.Health.Status}}' "$postgres_container" 2>/dev/null || echo "unknown")
      if [[ "$pg_health" == "healthy" ]]; then
        echo "✅ PostgreSQL 服务健康检查通过"
        return 0
      elif [[ "$pg_health" == "unhealthy" ]]; then
        echo "⚠️ PostgreSQL 健康状态：$pg_health"
      fi
    else
      pg_health="not_running"
    fi

    if [ $i -eq 90 ]; then
      echo "❌ PostgreSQL 启动超时（90秒）"
      echo "🔍 当前状态：$(docker inspect -f '{{.State.Health.Status}}' "$postgres_container" 2>/dev/null || echo '容器不存在')"
      return 1
    fi

    if [ $((i % 15)) -eq 1 ]; then
      echo "⏳ 等待 PostgreSQL 启动... ($i/90) 状态：${pg_health:-unknown}"
    fi
    sleep 1
  done
}

wait_for_backend_healthy() {
  local backend_health
  local backend_container="${PANEL_BACKEND_CONTAINER:-$DEFAULT_PANEL_BACKEND_CONTAINER}"

  echo "🔍 检查后端服务状态..."
  for i in {1..90}; do
    if docker ps --format "{{.Names}}" | grep -Fxq "$backend_container"; then
      backend_health=$(docker inspect -f '{{.State.Health.Status}}' "$backend_container" 2>/dev/null || echo "unknown")
      if [[ "$backend_health" == "healthy" ]]; then
        echo "✅ 后端服务健康检查通过"
        return 0
      elif [[ "$backend_health" == "unhealthy" ]]; then
        echo "⚠️ 后端健康状态：$backend_health"
      fi
    else
      backend_health="not_running"
    fi

    if [ $i -eq 90 ]; then
      echo "❌ 后端服务启动超时（90秒）"
      echo "🔍 当前状态：$(docker inspect -f '{{.State.Health.Status}}' "$backend_container" 2>/dev/null || echo '容器不存在')"
      return 1
    fi

    if [ $((i % 15)) -eq 1 ]; then
      echo "⏳ 等待后端服务启动... ($i/90) 状态：${backend_health:-unknown}"
    fi
    sleep 1
  done
}

# 删除脚本自身
delete_self() {
  echo ""
  echo "🗑️ 操作已完成，正在清理脚本文件..."
  sleep 1
  rm -f "$PANEL_SCRIPT_PATH" && echo "✅ 脚本文件已删除" || echo "❌ 删除脚本文件失败"
}



# 获取用户输入的配置参数
get_config_params() {
  echo "🔧 请输入配置参数："

  read -p "前端端口（默认 6366）: " FRONTEND_PORT
  FRONTEND_PORT=${FRONTEND_PORT:-6366}

  read -p "后端端口（默认 6365）: " BACKEND_PORT
  BACKEND_PORT=${BACKEND_PORT:-6365}

  echo "请选择数据库类型："
  echo "1. SQLite（默认）"
  echo "2. PostgreSQL"
  read -p "数据库类型（1/2，默认 1）: " DB_CHOICE
  case "$DB_CHOICE" in
    2)
      DB_TYPE="postgres"
      ;;
    ""|1)
      DB_TYPE="sqlite"
      ;;
    *)
      echo "⚠️ 输入无效，默认使用 SQLite"
      DB_TYPE="sqlite"
      ;;
  esac

  POSTGRES_DB="flux_panel"
  POSTGRES_USER="flux_panel"
  POSTGRES_PASSWORD=$(generate_random)

  if [[ "$DB_TYPE" == "postgres" ]]; then
    DATABASE_URL="postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}?sslmode=disable"
  else
    DATABASE_URL=""
  fi

  # 生成JWT密钥
  JWT_SECRET=$(generate_random)
}

# 安装功能
install_panel() {
  echo "🚀 开始安装面板..."

  ask_proxy_config
  ensure_compose_urls_initialized || return 1

  check_docker
  get_config_params

  echo "🔽 下载必要文件..."
  DOCKER_COMPOSE_URL=$(get_docker_compose_url)
  echo "📡 选择配置文件：$(basename "$DOCKER_COMPOSE_URL")"
  curl -L -o docker-compose.yml "$DOCKER_COMPOSE_URL"
  echo "✅ 文件准备完成"

  # 自动检测并配置 IPv6 支持
  if check_ipv6_support; then
    echo "🚀 系统支持 IPv6，自动启用 IPv6 配置..."
    configure_docker_ipv6
  fi

  cat > .env <<EOF
JWT_SECRET=$JWT_SECRET
FRONTEND_PORT=$FRONTEND_PORT
BACKEND_PORT=$BACKEND_PORT
FLUX_VERSION=$RESOLVED_VERSION

DB_TYPE=$DB_TYPE
DATABASE_URL=$DATABASE_URL

POSTGRES_DB=$POSTGRES_DB
POSTGRES_USER=$POSTGRES_USER
POSTGRES_PASSWORD=$POSTGRES_PASSWORD
EOF

  echo "🚀 启动 docker 服务..."
  if [[ "$DB_TYPE" == "postgres" ]]; then
    $DOCKER_CMD up -d postgres
    wait_for_postgres_healthy
    $DOCKER_CMD up -d backend frontend
  else
    $DOCKER_CMD up -d backend frontend
  fi

  echo "🎉 部署完成"
  echo "🌐 访问地址: http://服务器IP:$FRONTEND_PORT"
  echo "📖 部署完成后请阅读下使用文档，求求了啊，不要上去就是一顿操作"
  echo "📚 文档地址: https://tes.cc/guide.html"
  echo "💡 默认管理员账号: admin_user / admin_user"
  echo "⚠️  登录后请立即修改默认密码！"


}

# 更新功能
update_panel() {
  echo "🔄 开始更新面板..."
  check_docker
  resolve_panel_deployment || return 1
  ask_proxy_config
  validate_panel_update_environment || return 1

  CURRENT_DB_TYPE=$(get_current_db_type)
  echo "🗄️ 当前数据库类型：$CURRENT_DB_TYPE"

  echo "🔍 获取最新版本号..."
  LATEST_VERSION=$(resolve_latest_release_tag) || {
    echo "❌ 无法获取最新版本号，更新终止"
    return 1
  }
  echo "🆕 最新版本：$LATEST_VERSION"
  set_compose_urls_by_version "$LATEST_VERSION"

  echo "🔽 下载最新配置文件..."
  DOCKER_COMPOSE_URL=$(get_docker_compose_url)
  echo "📡 选择配置文件：$(basename "$DOCKER_COMPOSE_URL")"
  download_panel_update_compose "$DOCKER_COMPOSE_URL" || return 1
  if ! validate_panel_update_compose; then
    rm -f "$UPDATE_COMPOSE_CANDIDATE"
    UPDATE_COMPOSE_CANDIDATE=""
    return 1
  fi

  echo "💾 备份当前配置和数据库..."
  if ! create_panel_update_backup "$CURRENT_DB_TYPE"; then
    rm -f "$UPDATE_COMPOSE_CANDIDATE"
    UPDATE_COMPOSE_CANDIDATE=""
    return 1
  fi

  echo "⬇️ 拉取最新镜像..."
  if ! pull_panel_update_images "$CURRENT_DB_TYPE"; then
    rm -f "$UPDATE_COMPOSE_CANDIDATE"
    UPDATE_COMPOSE_CANDIDATE=""
    echo "❌ 镜像拉取失败，现有服务保持运行"
    return 1
  fi

  if ! activate_panel_update_files; then
    rollback_panel_update "$CURRENT_DB_TYPE" || true
    return 1
  fi

  echo "🚀 原地重建更新后的服务..."
  if ! start_panel_after_update "$CURRENT_DB_TYPE"; then
    echo "🛑 新版本启动失败"
    if rollback_panel_update "$CURRENT_DB_TYPE"; then
      echo "✅ 已恢复更新前版本"
    else
      echo "❌ 自动回滚失败，请从 $UPDATE_BACKUP_DIR 手动恢复"
    fi
    return 1
  fi

  echo "✅ 更新完成"
}


migrate_to_postgres() {
  local current_db_type postgres_db postgres_user postgres_password database_url

  echo "🔄 开始迁移 SQLite -> PostgreSQL..."
  check_docker

  if [[ ! -f ".env" ]]; then
    echo "❌ 未找到 .env 文件，请先安装面板"
    return 1
  fi

  if [[ ! -f "docker-compose.yml" ]]; then
    echo "⚠️ 未找到 docker-compose.yml 文件，正在下载..."
    ask_proxy_config
    ensure_compose_urls_initialized || return 1
    DOCKER_COMPOSE_URL=$(get_docker_compose_url)
    echo "📡 选择配置文件：$(basename "$DOCKER_COMPOSE_URL")"
    curl -L -o docker-compose.yml "$DOCKER_COMPOSE_URL"
    echo "✅ docker-compose.yml 下载完成"
  fi

  current_db_type=$(get_current_db_type)
  if [[ "$current_db_type" == "postgres" ]]; then
    echo "ℹ️ 当前已使用 PostgreSQL，无需迁移"
    return 0
  fi

  postgres_db=$(get_env_var "POSTGRES_DB")
  postgres_user=$(get_env_var "POSTGRES_USER")
  postgres_password=$(get_env_var "POSTGRES_PASSWORD")

  postgres_db=${postgres_db:-flux_panel}
  postgres_user=${postgres_user:-flux_panel}
  postgres_password=${postgres_password:-$(generate_random)}

  upsert_env_var ".env" "POSTGRES_DB" "$postgres_db"
  upsert_env_var ".env" "POSTGRES_USER" "$postgres_user"
  upsert_env_var ".env" "POSTGRES_PASSWORD" "$postgres_password"

  echo "🛑 停止当前服务..."
  docker stop -t 30 flux-panel-backend 2>/dev/null || true
  docker stop -t 10 vite-frontend 2>/dev/null || true
  echo "⏳ 等待数据同步..."
  sleep 5
  $DOCKER_CMD down

  echo "💾 备份 SQLite 数据到当前目录..."
  if ! docker run --rm -v sqlite_data:/data -v "$(pwd)":/backup alpine sh -c "cp /data/gost.db /backup/gost.db.bak"; then
    echo "❌ SQLite 备份失败，迁移终止"
    return 1
  fi

  echo "🚀 启动 PostgreSQL..."
  $DOCKER_CMD up -d postgres
  if ! wait_for_postgres_healthy; then
    echo "🛑 PostgreSQL 未就绪，迁移终止"
    return 1
  fi

  echo "🔄 执行 pgloader 迁移..."
  if ! docker run --rm --network gost-network -v sqlite_data:/sqlite dimitri/pgloader:latest pgloader /sqlite/gost.db "postgresql://${postgres_user}:${postgres_password}@postgres:5432/${postgres_db}"; then
    echo "❌ pgloader 迁移失败，迁移终止（如报 28P01，可执行 docker volume rm postgres_data 后重试）"
    return 1
  fi

  database_url="postgresql://${postgres_user}:${postgres_password}@postgres:5432/${postgres_db}?sslmode=disable"
  upsert_env_var ".env" "DB_TYPE" "postgres"
  upsert_env_var ".env" "DATABASE_URL" "$database_url"

  echo "🚀 启动迁移后的服务..."
  $DOCKER_CMD up -d postgres backend frontend

  echo "⏳ 等待服务启动..."
  if ! wait_for_backend_healthy; then
    echo "🛑 迁移后服务启动失败"
    return 1
  fi

  echo "✅ SQLite -> PostgreSQL 迁移完成"
}



# 卸载功能
uninstall_panel() {
  echo "🗑️ 开始卸载面板..."
  check_docker

  if [[ ! -f "docker-compose.yml" ]]; then
    echo "⚠️ 未找到 docker-compose.yml 文件，正在下载以完成卸载..."
    ask_proxy_config
    ensure_compose_urls_initialized || return 1
    DOCKER_COMPOSE_URL=$(get_docker_compose_url)
    echo "📡 选择配置文件：$(basename "$DOCKER_COMPOSE_URL")"
    curl -L -o docker-compose.yml "$DOCKER_COMPOSE_URL"
    echo "✅ docker-compose.yml 下载完成"
  fi

  read -p "确认卸载面板吗？此操作将停止并删除所有容器和数据 (y/N): " confirm
  if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
    echo "❌ 取消卸载"
    return 0
  fi

  echo "🛑 停止并删除容器、镜像、卷..."
  $DOCKER_CMD down --rmi all --volumes --remove-orphans
  echo "🧹 删除配置文件..."
  rm -f docker-compose.yml .env
  echo "✅ 卸载完成"
}

# 主逻辑
main() {

  # 显示交互式菜单
  while true; do
    show_menu
    read -p "请输入选项 (1-5): " choice

    case $choice in
      1)
        install_panel
        delete_self
        exit 0
        ;;
      2)
        update_panel
        delete_self
        exit 0
        ;;
      3)
        uninstall_panel
        delete_self
        exit 0
        ;;
      4)
        migrate_to_postgres
        delete_self
        exit 0
        ;;
      5)
        echo "👋 退出脚本"
        delete_self
        exit 0
        ;;
      *)
        echo "❌ 无效选项，请输入 1-5"
        echo ""
        ;;
    esac
  done
}

# 执行主函数
main
