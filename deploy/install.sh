#!/usr/bin/env bash
# install.sh — coord-server installer
# Usage: curl -fsSL https://raw.githubusercontent.com/goy-co/coord-server/main/deploy/install.sh | bash
set -euo pipefail

REPO="goy-co/coord-server"
BINARY="coord-server"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/coord-server"
DATA_DIR="/var/lib/coord-server"
SERVICE_USER="coord"
SERVICE_GROUP="coord"

# ── Colours ────────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'
info()    { echo -e "${CYAN}[coord-server]${NC} $*"; }
success() { echo -e "${GREEN}[coord-server]${NC} $*"; }
warn()    { echo -e "${YELLOW}[coord-server]${NC} $*"; }
error()   { echo -e "${RED}[coord-server] ERROR:${NC} $*" >&2; exit 1; }

# ── OS / Arch detection ────────────────────────────────────────────────────────
detect_os() {
  OS="$(uname -s)"
  ARCH="$(uname -m)"

  case "$OS" in
    Linux)  OS_LOWER="linux" ;;
    Darwin) OS_LOWER="darwin" ;;
    *)      error "Unsupported OS: $OS. Only Linux and macOS are supported." ;;
  esac

  case "$ARCH" in
    x86_64|amd64) ARCH_LOWER="amd64" ;;
    aarch64|arm64) ARCH_LOWER="arm64" ;;
    *) error "Unsupported architecture: $ARCH. Only amd64 and arm64 are supported." ;;
  esac

  info "Detected: $OS / $ARCH"
}

# ── Dependency check ───────────────────────────────────────────────────────────
check_deps() {
  for cmd in curl tar; do
    command -v "$cmd" >/dev/null 2>&1 || error "Required command '$cmd' not found."
  done
}

# ── Fetch latest release version ──────────────────────────────────────────────
get_latest_version() {
  if command -v curl >/dev/null 2>&1; then
    VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
      | grep '"tag_name"' | sed 's/.*"tag_name": *"\(.*\)".*/\1/')
  else
    error "curl is required to download releases."
  fi

  [ -n "$VERSION" ] || error "Could not determine latest release version."
  info "Latest version: $VERSION"
}

# ── Download and install binary ────────────────────────────────────────────────
download_binary() {
  TARBALL="${BINARY}_${OS_LOWER}_${ARCH_LOWER}.tar.gz"
  URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"

  info "Downloading $TARBALL from GitHub Releases..."
  TMPDIR="$(mktemp -d)"
  trap 'rm -rf "$TMPDIR"' EXIT

  curl -fsSL "$URL" -o "$TMPDIR/$TARBALL" || error "Failed to download $URL"
  tar -xzf "$TMPDIR/$TARBALL" -C "$TMPDIR"

  BINARY_PATH="$TMPDIR/$BINARY"
  [ -f "$BINARY_PATH" ] || error "Binary '$BINARY' not found in tarball."

  chmod +x "$BINARY_PATH"

  if [ "$OS_LOWER" = "linux" ]; then
    sudo mv "$BINARY_PATH" "$INSTALL_DIR/$BINARY"
    success "Installed $BINARY $VERSION → $INSTALL_DIR/$BINARY"
  else
    # macOS — no sudo required if /usr/local/bin is writable
    mv "$BINARY_PATH" "$INSTALL_DIR/$BINARY" 2>/dev/null \
      || sudo mv "$BINARY_PATH" "$INSTALL_DIR/$BINARY"
    success "Installed $BINARY $VERSION → $INSTALL_DIR/$BINARY"
  fi
}

# ── Linux: create system user and directories ──────────────────────────────────
setup_linux() {
  info "Creating system user '$SERVICE_USER'..."
  if ! id "$SERVICE_USER" >/dev/null 2>&1; then
    sudo useradd --system --no-create-home --shell /usr/sbin/nologin \
      --group "$SERVICE_GROUP" "$SERVICE_USER" 2>/dev/null || \
    sudo useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER"
  else
    info "User '$SERVICE_USER' already exists."
  fi

  sudo mkdir -p "$CONFIG_DIR" "$DATA_DIR"
  sudo chown "$SERVICE_USER:$SERVICE_GROUP" "$DATA_DIR"
  sudo chmod 750 "$DATA_DIR"

  generate_config_linux
  install_systemd
}

# ── macOS: create directories (no system user needed) ─────────────────────────
setup_macos() {
  CONFIG_DIR="$HOME/.config/coord-server"
  DATA_DIR="$HOME/.local/share/coord-server"

  mkdir -p "$CONFIG_DIR" "$DATA_DIR"

  generate_config_macos
  install_launchd
}

# ── Generate config.toml (Linux) ──────────────────────────────────────────────
generate_config_linux() {
  CONFIG_FILE="$CONFIG_DIR/config.toml"
  if [ -f "$CONFIG_FILE" ]; then
    warn "Config already exists at $CONFIG_FILE — skipping generation."
    return
  fi

  sudo tee "$CONFIG_FILE" >/dev/null <<EOF
# coord-server configuration
# See: https://github.com/${REPO}#configuration

[server]
listen               = "0.0.0.0:8080"
read_timeout_seconds  = 15
write_timeout_seconds = 15

[database]
path = "${DATA_DIR}/coord-server.db"

[auth]
require_auth = true
public_paths = ["/health", "/info", "/metrics"]

[vpn]
enabled                  = false
headscale_api_url        = ""
headscale_user           = "goy-nodes"
preauth_key_expiry_hours = 24
preauth_key_reusable     = false

[rate_limit]
requests_per_minute = 60
burst               = 10
heartbeat_rpm       = 120

[registry]
relay_ttl_seconds           = 300
discovery_cache_ttl_seconds = 15
max_relays_per_response     = 100

[jobs]
cleanup_relays_interval_seconds = 60
cleanup_nodes_interval_seconds  = 300
node_inactive_threshold_hours   = 24
EOF
  sudo chown root:"$SERVICE_GROUP" "$CONFIG_FILE"
  sudo chmod 640 "$CONFIG_FILE"
  success "Config written to $CONFIG_FILE"
}

# ── Generate config.toml (macOS) ──────────────────────────────────────────────
generate_config_macos() {
  CONFIG_FILE="$CONFIG_DIR/config.toml"
  if [ -f "$CONFIG_FILE" ]; then
    warn "Config already exists at $CONFIG_FILE — skipping generation."
    return
  fi

  cat >"$CONFIG_FILE" <<EOF
# coord-server configuration
# See: https://github.com/${REPO}#configuration

[server]
listen               = "127.0.0.1:8080"
read_timeout_seconds  = 15
write_timeout_seconds = 15

[database]
path = "${DATA_DIR}/coord-server.db"

[auth]
require_auth = true
public_paths = ["/health", "/info", "/metrics"]

[vpn]
enabled = false

[rate_limit]
requests_per_minute = 60
burst               = 10
heartbeat_rpm       = 120

[registry]
relay_ttl_seconds           = 300
discovery_cache_ttl_seconds = 15
max_relays_per_response     = 100

[jobs]
cleanup_relays_interval_seconds = 60
cleanup_nodes_interval_seconds  = 300
node_inactive_threshold_hours   = 24
EOF
  success "Config written to $CONFIG_FILE"
}

# ── Install systemd unit (Linux) ───────────────────────────────────────────────
install_systemd() {
  UNIT_FILE="/etc/systemd/system/coord-server.service"
  sudo tee "$UNIT_FILE" >/dev/null <<EOF
[Unit]
Description=coord-server — Goy mesh coordination server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_GROUP}
ExecStart=${INSTALL_DIR}/${BINARY} --config ${CONFIG_DIR}/config.toml
Restart=on-failure
RestartSec=5s

# Security hardening
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ReadWritePaths=${DATA_DIR}
ProtectHome=yes

# Secrets — set these in /etc/coord-server/env or via systemd override
EnvironmentFile=-/etc/coord-server/env

[Install]
WantedBy=multi-user.target
EOF

  sudo systemctl daemon-reload
  sudo systemctl enable coord-server

  success "systemd unit installed and enabled."
  echo ""
  warn "⚠️  Before starting: set COORD_ADMIN_API_KEY in /etc/coord-server/env"
  echo ""
  echo "   sudo mkdir -p /etc/coord-server"
  echo "   echo 'COORD_ADMIN_API_KEY=your-secret-key' | sudo tee /etc/coord-server/env"
  echo "   sudo chmod 600 /etc/coord-server/env"
  echo "   sudo systemctl start coord-server"
  echo "   sudo journalctl -u coord-server -f"
}

# ── Install launchd plist (macOS) ──────────────────────────────────────────────
install_launchd() {
  PLIST_DIR="$HOME/Library/LaunchAgents"
  PLIST_FILE="$PLIST_DIR/co.goyco.coord-server.plist"
  mkdir -p "$PLIST_DIR"

  cat >"$PLIST_FILE" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>co.goyco.coord-server</string>
  <key>ProgramArguments</key>
  <array>
    <string>${INSTALL_DIR}/${BINARY}</string>
    <string>--config</string>
    <string>${CONFIG_DIR}/config.toml</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>${HOME}/Library/Logs/coord-server.log</string>
  <key>StandardErrorPath</key>
  <string>${HOME}/Library/Logs/coord-server.log</string>
</dict>
</plist>
EOF

  launchctl load "$PLIST_FILE" 2>/dev/null || true
  success "launchd plist installed: $PLIST_FILE"
  echo ""
  warn "⚠️  Before starting: export COORD_ADMIN_API_KEY in your shell or add to launchd plist"
  echo ""
  echo "   launchctl setenv COORD_ADMIN_API_KEY your-secret-key"
  echo "   launchctl start co.goyco.coord-server"
  echo "   tail -f ~/Library/Logs/coord-server.log"
}

# ── Main ───────────────────────────────────────────────────────────────────────
main() {
  echo ""
  echo "  ╔═════════════════════════════════════╗"
  echo "  ║   coord-server installer            ║"
  echo "  ║   Goy mesh coordination server      ║"
  echo "  ╚═════════════════════════════════════╝"
  echo ""

  check_deps
  detect_os
  get_latest_version
  download_binary

  if [ "$OS_LOWER" = "linux" ]; then
    setup_linux
  else
    setup_macos
  fi

  echo ""
  success "✅ coord-server $VERSION installed successfully!"
  echo ""
  echo "  Verify: $BINARY --version"
  echo "  Docs:   https://github.com/${REPO}"
  echo ""
}

main "$@"
