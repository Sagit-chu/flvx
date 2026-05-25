#!/bin/bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${1:-${FLUX_VERSION:-commercial-v1}}"
OUT_DIR="${ROOT_DIR}/release/${VERSION}"
PACKAGE_NAME="flvx-commercial-${VERSION}.tar.gz"

mkdir -p "$OUT_DIR"

sha256_file() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    sha256sum "$1" | awk '{print $1}'
  fi
}

write_sha() {
  local file="$1"
  sha256_file "$file" > "${file}.sha256"
}

cp "${ROOT_DIR}/commercial_install.sh" "${OUT_DIR}/commercial_install.sh"
cp "${ROOT_DIR}/docker-compose-v4.yml" "${OUT_DIR}/docker-compose-v4.yml"
cp "${ROOT_DIR}/docker-compose-v6.yml" "${OUT_DIR}/docker-compose-v6.yml"

chmod 0755 "${OUT_DIR}/commercial_install.sh"
write_sha "${OUT_DIR}/commercial_install.sh"
write_sha "${OUT_DIR}/docker-compose-v4.yml"
write_sha "${OUT_DIR}/docker-compose-v6.yml"

tar \
  --exclude ".git" \
  --exclude ".github" \
  --exclude ".DS_Store" \
  --exclude ".env" \
  --exclude ".env.*" \
  --exclude "*/.env" \
  --exclude "*/.env.*" \
  --exclude ".vscode" \
  --exclude ".idea" \
  --exclude "release" \
  --exclude "vite-frontend/dist" \
  --exclude "vite-frontend/node_modules" \
  --exclude "go-backend/tmp" \
  -czf "${OUT_DIR}/${PACKAGE_NAME}" \
  -C "$ROOT_DIR" .
write_sha "${OUT_DIR}/${PACKAGE_NAME}"

cat > "${OUT_DIR}/release-artifacts.txt" <<EOF
版本：${VERSION}

安装脚本：
文件：${OUT_DIR}/commercial_install.sh
SHA256：$(cat "${OUT_DIR}/commercial_install.sh.sha256")
后台类型：installer

IPv4 Compose：
文件：${OUT_DIR}/docker-compose-v4.yml
SHA256：$(cat "${OUT_DIR}/docker-compose-v4.yml.sha256")
后台类型：compose-v4

IPv6 Compose：
文件：${OUT_DIR}/docker-compose-v6.yml
SHA256：$(cat "${OUT_DIR}/docker-compose-v6.yml.sha256")
后台类型：compose-v6

商业包归档：
文件：${OUT_DIR}/${PACKAGE_NAME}
SHA256：$(cat "${OUT_DIR}/${PACKAGE_NAME}.sha256")
后台类型：package
EOF

echo "商业包发布文件已生成：${OUT_DIR}"
