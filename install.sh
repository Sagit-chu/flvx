#!/bin/sh
# shellcheck shell=bash

# Alpine 默认不带 Bash。先用系统自带的 /bin/sh 安装/切换到 Bash，
# 后续主体继续使用 Bash 语法，避免要求用户手动准备运行环境。
if [ -z "${BASH_VERSION:-}" ]; then
  if command -v bash >/dev/null 2>&1; then
    exec bash "$0" "$@"
  fi

  if [ -f /etc/alpine-release ] && command -v apk >/dev/null 2>&1; then
    if [ "$(id -u)" -eq 0 ]; then
      apk add --no-cache bash
    elif command -v sudo >/dev/null 2>&1; then
      sudo apk add --no-cache bash
    elif command -v doas >/dev/null 2>&1; then
      doas apk add --no-cache bash
    else
      echo "❌ Alpine 安装需要 root 权限，或已配置 sudo/doas。" >&2
      exit 1
    fi

    exec bash "$0" "$@"
  fi

  echo "❌ 此安装脚本需要 Bash。" >&2
  exit 1
fi

# GitHub repo used for release downloads
REPO="Sagit-chu/flux-panel"

# 固定版本号（Release 构建时自动填充，留空则获取最新版）
PINNED_VERSION=""

# 获取系统架构
get_architecture() {
    ARCH=$(uname -m)
    case $ARCH in
        x86_64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        *)
            echo "amd64"  # 默认使用 amd64
            ;;
    esac
}

# 安装目录
INSTALL_DIR="/etc/flux_agent"
FLUX_AGENT_SYSTEMD_SERVICE_FILE="/etc/systemd/system/flux_agent.service"
FLUX_AGENT_OPENRC_SERVICE_FILE="/etc/init.d/flux_agent"
LEGACY_GOST_BINARY="/usr/local/bin/gost"
LEGACY_GOST_CONFIG_DIR="/etc/gost"
LEGACY_GOST_SERVICE_FILE_ETC="/etc/systemd/system/gost.service"
LEGACY_GOST_SERVICE_FILE_LIB="/lib/systemd/system/gost.service"
LEGACY_GOST_SERVICE_FILE_USR_LIB="/usr/lib/systemd/system/gost.service"
SERVICE_MANAGER="${SERVICE_MANAGER:-}"

# 镜像加速配置（可由面板传入或交互式询问）
PROXY_ENABLED="${PROXY_ENABLED:-}"
PROXY_URL="${PROXY_URL:-}"

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

  echo "❌ 无法获取最新版本号。你可以手动指定版本，例如：VERSION=<版本号> ./install.sh" >&2
  return 1
}

# 构建下载地址
build_download_url() {
    local ARCH=$(get_architecture)
    echo "https://github.com/${REPO}/releases/download/${RESOLVED_VERSION}/gost-${ARCH}"
}

ensure_download_url_initialized() {
  if [[ -n "${DOWNLOAD_URL:-}" ]]; then
    return 0
  fi

  RESOLVED_VERSION=$(resolve_version) || return 1
  DOWNLOAD_URL=$(maybe_proxy_url "$(build_download_url)")
}



# 显示菜单
show_menu() {
  echo "==============================================="
  echo "              管理脚本"
  echo "==============================================="
  echo "请选择操作："
  echo "1. 安装"
  echo "2. 更新"  
  echo "3. 卸载"
  echo "4. 退出"
  echo "==============================================="
}

# 删除脚本自身
delete_self() {
  echo ""
  echo "🗑️ 操作已完成，正在清理脚本文件..."
  SCRIPT_PATH="$(readlink -f "$0" 2>/dev/null || realpath "$0" 2>/dev/null || echo "$0")"
  sleep 1
  rm -f "$SCRIPT_PATH" && echo "✅ 脚本文件已删除" || echo "❌ 删除脚本文件失败"
}

# 检查并安装 tcpkill
check_and_install_tcpkill() {
  # 检查 tcpkill 是否已安装
  if command -v tcpkill &> /dev/null; then
    return 0
  fi
  
  # 检测操作系统类型
  OS_TYPE=$(uname -s)
  
  # 检查是否需要 sudo
  if [[ $EUID -ne 0 ]]; then
    SUDO_CMD="sudo"
  else
    SUDO_CMD=""
  fi
  
  if [[ "$OS_TYPE" == "Darwin" ]]; then
    if command -v brew &> /dev/null; then
      brew install dsniff &> /dev/null
    fi
    return 0
  fi
  
  # 检测 Linux 发行版并安装对应的包
  if [ -f /etc/os-release ]; then
    . /etc/os-release
    DISTRO=$ID
  elif [ -f /etc/redhat-release ]; then
    DISTRO="rhel"
  elif [ -f /etc/debian_version ]; then
    DISTRO="debian"
  else
    return 0
  fi
  
  case $DISTRO in
    ubuntu|debian)
      $SUDO_CMD apt update &> /dev/null
      $SUDO_CMD apt install -y dsniff &> /dev/null
      ;;
    centos|rhel|fedora)
      if command -v dnf &> /dev/null; then
        $SUDO_CMD dnf install -y dsniff &> /dev/null
      elif command -v yum &> /dev/null; then
        $SUDO_CMD yum install -y dsniff &> /dev/null
      fi
      ;;
    alpine)
      $SUDO_CMD apk add --no-cache dsniff &> /dev/null
      ;;
    arch|manjaro)
      $SUDO_CMD pacman -S --noconfirm dsniff &> /dev/null
      ;;
    opensuse*|sles)
      $SUDO_CMD zypper install -y dsniff &> /dev/null
      ;;
    gentoo)
      $SUDO_CMD emerge --ask=n net-analyzer/dsniff &> /dev/null
      ;;
    void)
      $SUDO_CMD xbps-install -Sy dsniff &> /dev/null
      ;;
  esac
  
  return 0
}

json_escape() {
  local value="$1"
  value=${value//\\/\\\\}
  value=${value//\"/\\\"}
  value=${value//$'\n'/\\n}
  value=${value//$'\r'/\\r}
  value=${value//$'\t'/\\t}
  printf '%s' "$value"
}

write_flux_agent_config() {
  local path="$1"
  printf '{\n  "addr": "%s",\n  "secret": "%s"\n}\n' \
    "$(json_escape "$SERVER_ADDR")" \
    "$(json_escape "$SECRET")" > "$path"
}

ensure_service_manager() {
  if [[ -n "$SERVICE_MANAGER" ]]; then
    case "$SERVICE_MANAGER" in
      systemd|openrc)
        return 0
        ;;
      *)
        echo "❌ 不支持的服务管理器: $SERVICE_MANAGER" >&2
        return 1
        ;;
    esac
  fi

  if command -v systemctl >/dev/null 2>&1 && [[ -d /run/systemd/system ]]; then
    SERVICE_MANAGER="systemd"
    return 0
  fi
  if command -v rc-service >/dev/null 2>&1 && command -v rc-update >/dev/null 2>&1; then
    SERVICE_MANAGER="openrc"
    return 0
  fi

  echo "❌ 未检测到受支持的服务管理器（systemd 或 OpenRC）。" >&2
  return 1
}

flux_agent_service_exists() {
  ensure_service_manager || return 1

  case "$SERVICE_MANAGER" in
    systemd)
      [[ -f "$FLUX_AGENT_SYSTEMD_SERVICE_FILE" ]] || \
        systemctl list-units --full -all 2>/dev/null | grep -Fq "flux_agent.service"
      ;;
    openrc)
      [[ -f "$FLUX_AGENT_OPENRC_SERVICE_FILE" ]]
      ;;
  esac
}

stop_flux_agent_service() {
  ensure_service_manager || return 1

  case "$SERVICE_MANAGER" in
    systemd)
      systemctl stop flux_agent 2>/dev/null || true
      ;;
    openrc)
      rc-service flux_agent stop 2>/dev/null || true
      ;;
  esac
}

disable_flux_agent_service() {
  ensure_service_manager || return 1

  case "$SERVICE_MANAGER" in
    systemd)
      systemctl disable flux_agent 2>/dev/null || true
      ;;
    openrc)
      rc-update del flux_agent default 2>/dev/null || true
      ;;
  esac
}

write_flux_agent_service() {
  ensure_service_manager || return 1

  case "$SERVICE_MANAGER" in
    systemd)
      mkdir -p "$(dirname "$FLUX_AGENT_SYSTEMD_SERVICE_FILE")"
      cat > "$FLUX_AGENT_SYSTEMD_SERVICE_FILE" <<EOF
[Unit]
Description=Flux_agent Proxy Service
After=network.target

[Service]
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/flux_agent
Restart=on-failure
StandardOutput=null
StandardError=null

[Install]
WantedBy=multi-user.target
EOF
      ;;
    openrc)
      mkdir -p "$(dirname "$FLUX_AGENT_OPENRC_SERVICE_FILE")"
      cat > "$FLUX_AGENT_OPENRC_SERVICE_FILE" <<EOF
#!/sbin/openrc-run

name="flux_agent"
description="Flux_agent Proxy Service"
command="$INSTALL_DIR/flux_agent"
directory="$INSTALL_DIR"
command_background="yes"
pidfile="/run/\${RC_SVCNAME}.pid"
output_log="/dev/null"
error_log="/dev/null"

depend() {
  need net
}
EOF
      chmod +x "$FLUX_AGENT_OPENRC_SERVICE_FILE"
      ;;
  esac
}

enable_and_start_flux_agent_service() {
  ensure_service_manager || return 1

  case "$SERVICE_MANAGER" in
    systemd)
      systemctl daemon-reload
      systemctl enable flux_agent
      systemctl start flux_agent
      ;;
    openrc)
      rc-update add flux_agent default
      rc-service flux_agent start
      ;;
  esac
}

start_flux_agent_service() {
  ensure_service_manager || return 1

  case "$SERVICE_MANAGER" in
    systemd)
      systemctl start flux_agent
      ;;
    openrc)
      rc-service flux_agent start
      ;;
  esac
}

flux_agent_service_is_active() {
  ensure_service_manager || return 1

  case "$SERVICE_MANAGER" in
    systemd)
      systemctl is-active --quiet flux_agent
      ;;
    openrc)
      rc-service flux_agent status >/dev/null 2>&1
      ;;
  esac
}

flux_agent_service_status() {
  ensure_service_manager || return 1

  case "$SERVICE_MANAGER" in
    systemd)
      systemctl is-active flux_agent 2>/dev/null || true
      ;;
    openrc)
      rc-service flux_agent status 2>/dev/null || true
      ;;
  esac
}

remove_flux_agent_service() {
  ensure_service_manager || return 1

  case "$SERVICE_MANAGER" in
    systemd)
      rm -f "$FLUX_AGENT_SYSTEMD_SERVICE_FILE"
      systemctl daemon-reload 2>/dev/null || true
      ;;
    openrc)
      rm -f "$FLUX_AGENT_OPENRC_SERVICE_FILE"
      ;;
  esac
}

flux_agent_service_status_hint() {
  ensure_service_manager || return 1

  case "$SERVICE_MANAGER" in
    systemd)
      echo "systemctl status flux_agent --no-pager"
      ;;
    openrc)
      echo "rc-service flux_agent status"
      ;;
  esac
}

cleanup_legacy_gost_installation() {
  local matched_service_files=()
  local service_file=""
  local removed_service_file="0"

  for service_file in "$LEGACY_GOST_SERVICE_FILE_ETC" "$LEGACY_GOST_SERVICE_FILE_LIB" "$LEGACY_GOST_SERVICE_FILE_USR_LIB"; do
    if [[ ! -f "$service_file" ]]; then
      continue
    fi
    if ! grep -Fq "WorkingDirectory=$LEGACY_GOST_CONFIG_DIR" "$service_file"; then
      continue
    fi
    if grep -Fq "ExecStart=$LEGACY_GOST_CONFIG_DIR/gost" "$service_file" || \
      (grep -Fq "ExecStart=$LEGACY_GOST_BINARY" "$service_file" && [[ -f "$LEGACY_GOST_CONFIG_DIR/config.json" && -f "$LEGACY_GOST_CONFIG_DIR/gost.json" ]]); then
      matched_service_files+=("$service_file")
    fi
  done

  if [[ ${#matched_service_files[@]} -eq 0 ]]; then
    return 0
  fi

  if systemctl list-units --full -all 2>/dev/null | grep -Fq "gost.service"; then
    systemctl stop gost 2>/dev/null || true
    systemctl disable gost 2>/dev/null || true
  fi

  for service_file in "${matched_service_files[@]}"; do
    if [[ -f "$service_file" ]]; then
      rm -f "$service_file"
      removed_service_file="1"
    fi
  done

  if [[ -f "$LEGACY_GOST_BINARY" ]]; then
    rm -f "$LEGACY_GOST_BINARY"
  fi
  if [[ -f "$LEGACY_GOST_CONFIG_DIR/gost" ]]; then
    rm -f "$LEGACY_GOST_CONFIG_DIR/gost"
  fi

  if [[ "$removed_service_file" == "1" ]]; then
    systemctl daemon-reload 2>/dev/null || true
  fi
}


# 获取用户输入的配置参数
get_config_params() {
  if [[ -z "$SERVER_ADDR" || -z "$SECRET" ]]; then
    echo "请输入配置参数："
    
    if [[ -z "$SERVER_ADDR" ]]; then
      read -p "服务器地址: " SERVER_ADDR
    fi
    
    if [[ -z "$SECRET" ]]; then
      read -p "密钥: " SECRET
    fi
    
    if [[ -z "$SERVER_ADDR" || -z "$SECRET" ]]; then
      echo "❌ 参数不完整，操作取消。"
      exit 1
    fi
  fi
}

# 解析命令行参数
while getopts "a:s:" opt; do
  case $opt in
    a) SERVER_ADDR="$OPTARG" ;;
    s) SECRET="$OPTARG" ;;
    *) echo "❌ 无效参数"; exit 1 ;;
  esac
done

# 安装功能
install_flux_agent() {
  echo "🚀 开始安装 flux_agent..."

  ask_proxy_config
  ensure_download_url_initialized || exit 1

  get_config_params

  # 检查并安装 tcpkill
  check_and_install_tcpkill
  ensure_service_manager || exit 1

  mkdir -p "$INSTALL_DIR"

  local tmp_binary="$INSTALL_DIR/flux_agent.new"

  # 停止并禁用已有服务
  if flux_agent_service_exists; then
    echo "🔍 检测到已存在的flux_agent服务"
    stop_flux_agent_service
    echo "🛑 停止服务"
    disable_flux_agent_service
    echo "🚫 禁用自启"
  fi

  # 下载 flux_agent
  echo "⬇️ 下载 flux_agent 中..."
  rm -f "$tmp_binary"
  curl -L "$DOWNLOAD_URL" -o "$tmp_binary"
  if [[ ! -f "$tmp_binary" || ! -s "$tmp_binary" ]]; then
    rm -f "$tmp_binary"
    echo "❌ 下载失败，请检查网络或下载链接。"
    exit 1
  fi
  cleanup_legacy_gost_installation
  mv "$tmp_binary" "$INSTALL_DIR/flux_agent"
  chmod +x "$INSTALL_DIR/flux_agent"
  echo "✅ 下载完成"

  # 打印版本
  echo "🔎 flux_agent 版本：$($INSTALL_DIR/flux_agent -V)"

  # 写入 config.json (安装时总是创建新的)
  CONFIG_FILE="$INSTALL_DIR/config.json"
  echo "📄 创建新配置: config.json"
  write_flux_agent_config "$CONFIG_FILE"

  # 写入 gost.json
  GOST_CONFIG="$INSTALL_DIR/gost.json"
  if [[ -f "$GOST_CONFIG" ]]; then
    echo "⏭️ 跳过配置文件: gost.json (已存在)"
  else
    echo "📄 创建新配置: gost.json"
    cat > "$GOST_CONFIG" <<EOF
{}
EOF
  fi

  # 加强权限
  chmod 600 "$INSTALL_DIR"/*.json

  # 创建 systemd 或 OpenRC 服务
  write_flux_agent_service

  # 启动服务
  enable_and_start_flux_agent_service

  # 检查状态
  echo "🔄 检查服务状态..."
  if flux_agent_service_is_active; then
    echo "✅ 安装完成，flux_agent服务已启动并设置为开机启动。"
    echo "📁 配置目录: $INSTALL_DIR"
    echo "🔧 服务状态: $(flux_agent_service_status)"
  else
    echo "❌ flux_agent服务启动失败，请执行以下命令查看状态："
    flux_agent_service_status_hint
  fi
}

# 更新功能
update_flux_agent() {
  echo "🔄 开始更新 flux_agent..."
  
  if [[ ! -d "$INSTALL_DIR" ]]; then
    echo "❌ flux_agent 未安装，请先选择安装。"
    return 1
  fi

  ask_proxy_config
  ensure_download_url_initialized || return 1
  
  echo "📥 使用下载地址: $DOWNLOAD_URL"
  
  # 检查并安装 tcpkill
  check_and_install_tcpkill
  ensure_service_manager || return 1
  
  # 先下载新版本
  echo "⬇️ 下载最新版本..."
  rm -f "$INSTALL_DIR/flux_agent.new"
  curl -L "$DOWNLOAD_URL" -o "$INSTALL_DIR/flux_agent.new"
  if [[ ! -f "$INSTALL_DIR/flux_agent.new" || ! -s "$INSTALL_DIR/flux_agent.new" ]]; then
    echo "❌ 下载失败。"
    return 1
  fi
  cleanup_legacy_gost_installation

  # 停止服务
  if flux_agent_service_exists; then
    echo "🛑 停止 flux_agent 服务..."
    stop_flux_agent_service
  fi

  # 替换文件
  mv "$INSTALL_DIR/flux_agent.new" "$INSTALL_DIR/flux_agent"
  chmod +x "$INSTALL_DIR/flux_agent"
  
  # 打印版本
  echo "🔎 新版本：$($INSTALL_DIR/flux_agent -V)"

  # 重启服务
  echo "🔄 重启服务..."
  start_flux_agent_service
  
  echo "✅ 更新完成，服务已重新启动。"
}

# 卸载功能
uninstall_flux_agent() {
  echo "🗑️ 开始卸载 flux_agent..."
  ensure_service_manager || return 1
  
  read -p "确认卸载 flux_agent 吗？此操作将删除所有相关文件 (y/N): " confirm
  if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
    echo "❌ 取消卸载"
    return 0
  fi

  # 停止并禁用服务
  if flux_agent_service_exists; then
    echo "🛑 停止并禁用服务..."
    stop_flux_agent_service
    disable_flux_agent_service
  fi

  # 删除服务文件
  if [[ -f "$FLUX_AGENT_SYSTEMD_SERVICE_FILE" || -f "$FLUX_AGENT_OPENRC_SERVICE_FILE" ]]; then
    remove_flux_agent_service
    echo "🧹 删除服务文件"
  fi

  # 删除安装目录
  if [[ -d "$INSTALL_DIR" ]]; then
    rm -rf "$INSTALL_DIR"
    echo "🧹 删除安装目录: $INSTALL_DIR"
  fi

  echo "✅ 卸载完成"
}

# 主逻辑
main() {
  # 如果提供了命令行参数，直接执行安装
  if [[ -n "$SERVER_ADDR" && -n "$SECRET" ]]; then
    install_flux_agent
    delete_self
    exit 0
  fi

  # 显示交互式菜单
  while true; do
    show_menu
    read -p "请输入选项 (1-4): " choice
    
    case $choice in
      1)
        install_flux_agent
        delete_self
        exit 0
        ;;
      2)
        update_flux_agent
        delete_self
        exit 0
        ;;
      3)
        uninstall_flux_agent
        delete_self
        exit 0
        ;;
      4)
        echo "👋 退出脚本"
        delete_self
        exit 0
        ;;
      *)
        echo "❌ 无效选项，请输入 1-4"
        echo ""
        ;;
    esac
  done
}

# 执行主函数
main
