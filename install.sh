#!/bin/bash

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

build_dash_download_url() {
    local ARCH=$(get_architecture)
    echo "https://github.com/${REPO}/releases/download/${RESOLVED_VERSION}/dash-${ARCH}"
}

ensure_download_url_initialized() {
  if [[ -n "${DOWNLOAD_URL:-}" ]]; then
    return 0
  fi

  RESOLVED_VERSION=$(resolve_version) || return 1
  DOWNLOAD_URL=$(maybe_proxy_url "$(build_download_url)")
  DASH_DOWNLOAD_URL=$(maybe_proxy_url "$(build_dash_download_url)")
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
  

  mkdir -p "$INSTALL_DIR"

  # 停止并禁用已有服务
  if systemctl list-units --full -all | grep -Fq "flux_agent.service"; then
    echo "🔍 检测到已存在的flux_agent服务"
    systemctl stop flux_agent 2>/dev/null && echo "🛑 停止服务"
    systemctl disable flux_agent 2>/dev/null && echo "🚫 禁用自启"
  fi

  # 删除旧文件
  [[ -f "$INSTALL_DIR/flux_agent" ]] && echo "🧹 删除旧文件 flux_agent" && rm -f "$INSTALL_DIR/flux_agent"

  # 下载 flux_agent
  echo "⬇️ 下载 flux_agent 中..."
  curl -L "$DOWNLOAD_URL" -o "$INSTALL_DIR/flux_agent"
  if [[ ! -f "$INSTALL_DIR/flux_agent" || ! -s "$INSTALL_DIR/flux_agent" ]]; then
    echo "❌ 下载失败"
    return 1
  fi
  chmod +x "$INSTALL_DIR/flux_agent"

  # 下载 dash 内核
  echo "⬇️ 下载 dash 内核中..."
  curl -L "$DASH_DOWNLOAD_URL" -o "$INSTALL_DIR/dash"
  if [[ ! -f "$INSTALL_DIR/dash" || ! -s "$INSTALL_DIR/dash" ]]; then
    echo "⚠️ 下载 dash 失败，非致命错误"
  else
    chmod +x "$INSTALL_DIR/dash"
  fi
  echo "✅ 下载完成"

  # 打印版本
  echo "🔎 flux_agent 版本：$($INSTALL_DIR/flux_agent -V)"

  # 写入 config.json (安装时总是创建新的)
  CONFIG_FILE="$INSTALL_DIR/config.json"
  echo "📄 创建新配置: config.json"
  cat > "$CONFIG_FILE" <<EOF
{
  "addr": "$SERVER_ADDR",
  "secret": "$SECRET"
}
EOF

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

  # 创建 systemd 服务
  SERVICE_FILE="/etc/systemd/system/flux_agent.service"
  cat > "$SERVICE_FILE" <<EOF
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

  DASH_SERVICE_FILE="/etc/systemd/system/dash.service"
  cat > "$DASH_SERVICE_FILE" <<EOF
[Unit]
Description=Dash Proxy Kernel
After=network.target

[Service]
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/dash
Restart=on-failure
StandardOutput=null
StandardError=null

[Install]
WantedBy=multi-user.target
EOF

  # 启动服务
  systemctl daemon-reload
  systemctl enable flux_agent
  systemctl start flux_agent

  if [[ -f "$INSTALL_DIR/dash" && -s "$INSTALL_DIR/dash" ]]; then
    systemctl enable dash
    systemctl start dash
  fi

  # 检查状态
  echo "🔄 检查服务状态..."
  if systemctl is-active --quiet flux_agent; then
    echo "✅ 安装完成，flux_agent服务已启动并设置为开机启动。"
    if [[ -f "$INSTALL_DIR/dash" && -s "$INSTALL_DIR/dash" ]]; then
      if systemctl is-active --quiet dash; then
        echo "✅ dash 服务已启动并设置为开机启动。"
      else
        echo "⚠️ dash 服务启动失败，但这不影响 gost 内核的正常运行。"
      fi
    fi
    echo "📁 配置目录: $INSTALL_DIR"
    echo "🔧 服务状态: $(systemctl is-active flux_agent)"
  else
    echo "❌ flux_agent服务启动失败，请执行以下命令查看状态："
    echo "systemctl status flux_agent --no-pager"
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
  
  # 先下载新版本
  echo "⬇️ 下载最新版本..."
  echo "⬇️ 下载新版本 flux_agent 中..."
  curl -L "$DOWNLOAD_URL" -o "$INSTALL_DIR/flux_agent.new"
  if [[ ! -f "$INSTALL_DIR/flux_agent.new" || ! -s "$INSTALL_DIR/flux_agent.new" ]]; then
    echo "❌ 下载 flux_agent 失败"
    return 1
  fi

  echo "⬇️ 下载新版本 dash 内核中..."
  curl -L "$DASH_DOWNLOAD_URL" -o "$INSTALL_DIR/dash.new"
  if [[ ! -f "$INSTALL_DIR/dash.new" || ! -s "$INSTALL_DIR/dash.new" ]]; then
    echo "⚠️ 下载 dash 失败，跳过更新 dash 内核"
    rm -f "$INSTALL_DIR/dash.new"
  fi

  if systemctl list-units --full -all | grep -Fq "flux_agent.service"; then
    echo "🛑 停止 flux_agent 服务..."
    systemctl stop flux_agent
  fi

  echo "🔄 替换文件..."
  mv "$INSTALL_DIR/flux_agent.new" "$INSTALL_DIR/flux_agent"
  chmod +x "$INSTALL_DIR/flux_agent"

  if [[ -f "$INSTALL_DIR/dash.new" ]]; then
    mv "$INSTALL_DIR/dash.new" "$INSTALL_DIR/dash"
    chmod +x "$INSTALL_DIR/dash"
  fi
  
  # 打印版本
  echo "🔎 新版本：$($INSTALL_DIR/flux_agent -V)"

  # 重启服务
  echo "🔄 重启服务..."
  systemctl start flux_agent
  
  echo "✅ 更新完成，服务已重新启动。"
}

# 卸载功能
uninstall_flux_agent() {
  echo "🗑️ 开始卸载 flux_agent..."
  
  read -p "确认卸载 flux_agent 吗？此操作将删除所有相关文件 (y/N): " confirm
  if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
    echo "❌ 取消卸载"
    return 0
  fi

  # 停止并禁用服务
  if systemctl list-units --full -all | grep -Fq "flux_agent.service"; then
    echo "🛑 停止并禁用相关服务"
    systemctl stop flux_agent 2>/dev/null
    systemctl disable flux_agent 2>/dev/null
    systemctl stop dash 2>/dev/null
    systemctl disable dash 2>/dev/null
  fi

  if [[ -f "/etc/systemd/system/flux_agent.service" ]]; then
    rm -f "/etc/systemd/system/flux_agent.service"
    echo "🧹 删除 flux_agent 服务文件"
  fi

  if [[ -f "/etc/systemd/system/dash.service" ]]; then
    rm -f "/etc/systemd/system/dash.service"
    echo "🧹 删除 dash 服务文件"
  fi
  fi

  # 删除安装目录
  if [[ -d "$INSTALL_DIR" ]]; then
    rm -rf "$INSTALL_DIR"
    echo "🧹 删除安装目录: $INSTALL_DIR"
  fi

  # 重载 systemd
  systemctl daemon-reload

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
