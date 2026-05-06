#!/bin/sh
set -e

REPO="jizhi0v0/163-gmail-server-wrapper"
DATA_DIR="/etc/163-wrapper"

# ── 颜色输出 ──────────────────────────────────────────────────────────────────
info()    { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
success() { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
warn()    { printf '\033[1;33m==>\033[0m %s\n' "$*"; }
die()     { printf '\033[1;31mERROR:\033[0m %s\n' "$*" >&2; exit 1; }

# ── 交互选择安装方式 ──────────────────────────────────────────────────────────
echo ""
echo "  163 IMAP Proxy — installer"
echo ""
echo "  [1] systemd   — install binary + systemd service (recommended)"
echo "  [2] Docker    — run via docker compose"
echo ""
printf "  Choose [1/2]: "
read -r INSTALL_MODE

case "$INSTALL_MODE" in
  1) INSTALL_MODE="systemd" ;;
  2) INSTALL_MODE="docker"  ;;
  *) die "Invalid choice" ;;
esac

# ── 初始化数据目录 ─────────────────────────────────────────────────────────────
mkdir -p "$DATA_DIR"

write_config() {
  if [ ! -f "${DATA_DIR}/config.yaml" ]; then
    cat > "${DATA_DIR}/config.yaml" <<EOF
listen: "0.0.0.0:1993"
upstream: "imap.163.com:993"
upstream_tls_server_name: "imap.163.com"
log_level: "info"
EOF
    info "Created ${DATA_DIR}/config.yaml"
  else
    warn "Config already exists at ${DATA_DIR}/config.yaml — skipping"
  fi
}

# ══════════════════════════════════════════════════════════════════════════════
# 方式 1：systemd
# ══════════════════════════════════════════════════════════════════════════════
install_systemd() {
  command -v systemctl > /dev/null 2>&1 || die "systemd not found on this system"

  INSTALL_DIR="/usr/local/bin"

  # 检测架构
  ARCH=$(uname -m)
  case "$ARCH" in
    x86_64)  GOARCH="amd64" ;;
    aarch64) GOARCH="arm64" ;;
    armv7l)  GOARCH="arm"   ;;
    *) die "Unsupported architecture: $ARCH" ;;
  esac

  info "Fetching latest release..."
  LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\(.*\)".*/\1/')
  [ -n "$LATEST" ] || die "Failed to fetch latest release tag"

  BINARY_URL="https://github.com/${REPO}/releases/download/${LATEST}/163-wrapper-linux-${GOARCH}"
  info "Downloading $BINARY_URL ..."
  curl -fsSL "$BINARY_URL" -o /tmp/163-wrapper
  chmod +x /tmp/163-wrapper
  mv /tmp/163-wrapper "${INSTALL_DIR}/163-wrapper"

  write_config

  cat > /etc/systemd/system/163-wrapper.service <<EOF
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

  success "Service started via systemd"
  info "Logs: journalctl -u 163-wrapper -f"
}

# ══════════════════════════════════════════════════════════════════════════════
# 方式 2：Docker
# ══════════════════════════════════════════════════════════════════════════════
install_docker() {
  command -v docker > /dev/null 2>&1 || die "docker not found — install Docker first: https://docs.docker.com/engine/install/"

  COMPOSE_DIR="/opt/163-wrapper"
  mkdir -p "$COMPOSE_DIR"

  write_config

  # docker-compose.yml 挂载同一个 DATA_DIR
  cat > "${COMPOSE_DIR}/docker-compose.yml" <<EOF
services:
  163-wrapper:
    image: ghcr.io/jizhi0v0/163-gmail-server-wrapper:latest
    restart: unless-stopped
    ports:
      - "1993:1993"
    volumes:
      - ${DATA_DIR}:/data
EOF

  info "Pulling image..."
  docker compose -f "${COMPOSE_DIR}/docker-compose.yml" pull

  docker compose -f "${COMPOSE_DIR}/docker-compose.yml" up -d

  success "Container started via Docker"
  info "Logs: docker compose -f ${COMPOSE_DIR}/docker-compose.yml logs -f"
}

# ── 执行 ───────────────────────────────────────────────────────────────────────
case "$INSTALL_MODE" in
  systemd) install_systemd ;;
  docker)  install_docker  ;;
esac

echo ""
success "Done! Cert saved to ${DATA_DIR}/cert.pem"
info "Copy it to your Mac and trust it:"
echo ""
echo "    sudo security add-trusted-cert -d -r trustRoot \\"
echo "      -k /Library/Keychains/System.keychain cert.pem"
echo ""
