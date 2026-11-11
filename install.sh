#!/usr/bin/env bash
# ==============================================================================
# SwanPanel - Automated One-Line Production Installer
# https://github.com/thienvu18/swan-panel
#
# Supported Distributions:
#   - Ubuntu 22.04 LTS / 24.04 LTS
#   - Debian 11 / 12
#   - Rocky Linux 8 / 9, AlmaLinux 8 / 9, RHEL 8 / 9
#   - Fedora 38+
# ==============================================================================

set -euo pipefail

# Product Branding
readonly PRODUCT_NAME="SwanPanel"
readonly GITHUB_REPO="thienvu18/swan-panel"
readonly INSTALL_BIN_PATH="/usr/local/bin/swan-panel"
readonly CONFIG_DIR="/etc/swan-panel"
readonly DATA_DIR="/var/lib/swan-panel"
readonly SYSTEMD_SERVICE="/etc/systemd/system/swan-panel.service"

# Plain-text status helpers (Zero AI Unicode Emojis)
info() {
    printf "[*] %s\n" "$*"
}

success() {
    printf "[+] %s\n" "$*"
}

warn() {
    printf "[!] %s\n" "$*"
}

error() {
    printf "[-] ERROR: %s\n" "$*" >&2
    exit 1
}

# 1. Require Root Privileges
if [[ $EUID -ne 0 ]]; then
    error "This installer must be run as root. Please run with: sudo bash install.sh"
fi

info "Starting ${PRODUCT_NAME} automated installation..."

# 2. Detect System Architecture
ARCH="$(uname -m)"
case "${ARCH}" in
    x86_64)
        BIN_ARCH="linux-amd64"
        ;;
    aarch64|arm64)
        BIN_ARCH="linux-arm64"
        ;;
    *)
        error "Unsupported system architecture: ${ARCH}. SwanPanel supports x86_64 and aarch64/arm64."
        ;;
esac
success "Detected architecture: ${ARCH} (${BIN_ARCH})"

# 3. Detect Linux Distribution and Install Dependencies
info "Detecting operating system distribution..."
if [[ -f /etc/os-release ]]; then
    . /etc/os-release
    OS_ID="${ID:-}"
    OS_LIKE="${ID_LIKE:-}"
else
    error "Cannot detect Linux distribution (/etc/os-release missing)."
fi

info "Detected OS: ${PRETTY_NAME:-$OS_ID}"

case "${OS_ID}" in
    ubuntu|debian)
        info "Installing strongSwan, VICI IPC plugins, and network utilities via apt..."
        export DEBIAN_FRONTEND=noninteractive
        apt-get update -q -y
        apt-get install -q -y \
            curl \
            tar \
            iptables \
            nftables \
            strongswan \
            strongswan-charon-systemd \
            libcharon-extra-plugins \
            libstrongswan-extra-plugins || {
                warn "strongswan-charon-systemd not available; falling back to strongswan-starter"
                apt-get install -q -y strongswan libcharon-extra-plugins
            }
        ;;
    rhel|centos|rocky|almalinux|fedora)
        info "Installing strongSwan, VICI IPC plugins, and network utilities via dnf/yum..."
        if command -v dnf >/dev/null 2>&1; then
            PKG_MGR="dnf"
        else
            PKG_MGR="yum"
        fi
        ${PKG_MGR} install -y curl tar iptables nftables
        if ! ${PKG_MGR} install -y strongswan strongswan-charon-systemd 2>/dev/null; then
            info "Enabling EPEL repository for strongSwan packages..."
            ${PKG_MGR} install -y epel-release || true
            ${PKG_MGR} install -y strongswan
        fi
        ;;
    *)
        warn "Unverified distribution family (${OS_ID}). Attempting generic package installation..."
        if command -v apt-get >/dev/null 2>&1; then
            apt-get update -y && apt-get install -y curl strongswan iptables nftables
        elif command -v dnf >/dev/null 2>&1; then
            dnf install -y curl strongswan iptables nftables
        else
            error "No supported package manager detected (apt/dnf/yum)."
        fi
        ;;
esac
success "Dependencies installed successfully."

# 4. Configure Kernel IP Packet Forwarding
info "Configuring IPv4/IPv6 packet forwarding in kernel..."
mkdir -p /etc/sysctl.d
cat <<'EOF' > /etc/sysctl.d/99-swanpanel.conf
# SwanPanel VPN Kernel Forwarding Optimization
net.ipv4.ip_forward = 1
net.ipv4.conf.all.accept_redirects = 0
net.ipv4.conf.all.send_redirects = 0
net.ipv6.conf.all.forwarding = 1
net.ipv6.conf.all.accept_redirects = 0
EOF

sysctl --system >/dev/null 2>&1 || sysctl -p /etc/sysctl.d/99-swanpanel.conf >/dev/null 2>&1
success "Kernel packet forwarding enabled."

# 5. Fetch and Install SwanPanel Binary
VERSION="${SWANPANEL_VERSION:-latest}"
info "Downloading ${PRODUCT_NAME} binary (${VERSION})..."

if [[ "${VERSION}" == "latest" ]]; then
    RELEASE_URL="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
    DOWNLOAD_URL="$(curl -fsSL "${RELEASE_URL}" | grep "browser_download_url.*swan-panel-${BIN_ARCH}" | cut -d '"' -f 4 | head -n 1 || true)"
    if [[ -z "${DOWNLOAD_URL}" ]]; then
        warn "Could not resolve latest release via GitHub API. Falling back to tag v0.0.2 download..."
        DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/v0.0.2/swan-panel-${BIN_ARCH}"
    fi
else
    DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/swan-panel-${BIN_ARCH}"
fi

TMP_BIN="/tmp/swan-panel-download"
info "Downloading from: ${DOWNLOAD_URL}"
if curl -fsSL -o "${TMP_BIN}" "${DOWNLOAD_URL}"; then
    chmod +x "${TMP_BIN}"
    mv "${TMP_BIN}" "${INSTALL_BIN_PATH}"
    success "Binary installed to ${INSTALL_BIN_PATH}"
else
    warn "Failed to download precompiled binary from GitHub."
    if [[ -f "./swan-panel" ]]; then
        info "Detected local binary './swan-panel'. Installing local build..."
        install -m 755 "./swan-panel" "${INSTALL_BIN_PATH}"
        success "Installed local binary to ${INSTALL_BIN_PATH}"
    else
        error "Could not download binary and no local binary was found."
    fi
fi

# 6. Create Configuration & Data Directories
info "Configuring environment and data directories..."
mkdir -p "${CONFIG_DIR}"
mkdir -p "${DATA_DIR}"
chmod 750 "${DATA_DIR}"

if [[ ! -f "${CONFIG_DIR}/swan-panel.env" ]]; then
    # Generate random 32-character secret for JWT signing
    JWT_SECRET="$(head -c 32 /dev/urandom | base64 | tr -dc 'a-zA-Z0-9' | head -c 32)"
    cat <<EOF > "${CONFIG_DIR}/swan-panel.env"
# SwanPanel Production Configuration
PORT=8080
DB_PATH=${DATA_DIR}/swanpanel.db
LOG_LEVEL=INFO
ENVIRONMENT=production
JWT_SECRET=${JWT_SECRET}
ENABLE_DNS=true
DNS_PORT=53
EOF
    chmod 600 "${CONFIG_DIR}/swan-panel.env"
    success "Generated new configuration file at ${CONFIG_DIR}/swan-panel.env"
else
    info "Existing configuration found at ${CONFIG_DIR}/swan-panel.env (preserving settings)."
fi

# 7. Configure systemd Service
info "Setting up systemd service unit..."
cat <<EOF > "${SYSTEMD_SERVICE}"
[Unit]
Description=SwanPanel Control Plane for strongSwan VPN
Documentation=https://github.com/${GITHUB_REPO}
After=network.target strongswan.service strongswan-starter.service
Wants=network.target

[Service]
Type=simple
EnvironmentFile=-${CONFIG_DIR}/swan-panel.env
ExecStart=${INSTALL_BIN_PATH}
Restart=always
RestartSec=3s
LimitNOFILE=65535
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

# 8. Start and Enable Services
info "Reloading systemd and enabling services..."
systemctl daemon-reload

# Start strongSwan if present
if systemctl list-unit-files | grep -q "strongswan-starter"; then
    systemctl enable strongswan-starter || true
    systemctl restart strongswan-starter || true
elif systemctl list-unit-files | grep -q "strongswan"; then
    systemctl enable strongswan || true
    systemctl restart strongswan || true
fi

# Enable and start SwanPanel
systemctl enable swan-panel
systemctl restart swan-panel
success "${PRODUCT_NAME} service started successfully."

# 9. Detect Public IP Address
SERVER_IP="$(curl -s4 --max-time 3 ifconfig.me || hostname -I | awk '{print $1}')"

# 10. Summary Banner
printf "\n"
printf "======================================================================\n"
printf "       %s Installation Completed Successfully!       \n" "${PRODUCT_NAME}"
printf "======================================================================\n"
printf "\n"
printf "  Web Dashboard:      http://%s:8080\n" "${SERVER_IP}"
printf "  Default Admin:      admin@swanpanel.internal\n"
printf "  Default Password:   admin123456\n"
printf "\n"
printf "  Important: Change the default administrator password immediately\n"
printf "  after logging in for the first time.\n"
printf "\n"
printf "  Service Status:     sudo systemctl status swan-panel\n"
printf "  Stream Live Logs:   sudo journalctl -u swan-panel -f\n"
printf "  Configuration:      %s/swan-panel.env\n" "${CONFIG_DIR}"
printf "  Database Path:      %s/swanpanel.db\n" "${DATA_DIR}"
printf "\n"
printf "======================================================================\n"
