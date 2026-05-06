#!/bin/sh
set -e

REPO="jizhi0v0/163-gmail-server-wrapper"
INSTALL_DIR="/usr/local/bin"
DATA_DIR="/etc/163-wrapper"
SERVICE_FILE="/etc/systemd/system/163-wrapper.service"

# 检测架构
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  armv7l)  ARCH="arm"   ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac
OS="linux"

echo "==> Fetching latest release..."
LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\(.*\)".*/\1/')

if [ -z "$LATEST" ]; then
  echo "Failed to fetch latest release. Make sure the repo has published releases."
  exit 1
fi

BINARY_URL="https://github.com/${REPO}/releases/download/${LATEST}/163-wrapper-${OS}-${ARCH}"
echo "==> Downloading ${BINARY_URL}..."
curl -fsSL "$BINARY_URL" -o /tmp/163-wrapper
chmod +x /tmp/163-wrapper
mv /tmp/163-wrapper "${INSTALL_DIR}/163-wrapper"

# 数据目录
mkdir -p "$DATA_DIR"

# 示例配置（仅首次）
if [ ! -f "${DATA_DIR}/config.yaml" ]; then
  cat > "${DATA_DIR}/config.yaml" <<EOF
listen: "0.0.0.0:1993"
upstream: "imap.163.com:993"
upstream_tls_server_name: "imap.163.com"
log_level: "info"
EOF
  echo "==> Created ${DATA_DIR}/config.yaml — edit if needed"
fi

# systemd unit（仅在有 systemd 的系统上安装）
if command -v systemctl > /dev/null 2>&1; then
  cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=163 IMAP Proxy
After=network.target

[Service]
ExecStart=${INSTALL_DIR}/163-wrapper -c ${DATA_DIR}/config.yaml -d ${DATA_DIR}
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable 163-wrapper
  systemctl restart 163-wrapper
  echo "==> Service started. Check logs: journalctl -u 163-wrapper -f"
else
  echo "==> systemd not found. Run manually:"
  echo "    163-wrapper -c ${DATA_DIR}/config.yaml -d ${DATA_DIR}"
fi

echo ""
echo "==> Done! Cert saved to ${DATA_DIR}/cert.pem"
echo "    Copy it to your Mac and trust it:"
echo "    sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain cert.pem"
