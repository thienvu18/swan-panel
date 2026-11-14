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
        # SwanPanel uses the modern VICI/swanctl backend, not ipsec.conf/starter.
        # charon-systemd and strongswan-swanctl are separate packages here.
        apt-get install -q -y \
            curl \
            tar \
            iptables \
            nftables \
            charon-systemd \
            strongswan-swanctl \
            libcharon-extra-plugins \
            libstrongswan-extra-plugins
        ;;
    rhel|centos|rocky|almalinux|fedora)
        info "Installing strongSwan, VICI IPC plugins, and network utilities via dnf/yum..."
        if command -v dnf >/dev/null 2>&1; then
            PKG_MGR="dnf"
        else
            PKG_MGR="yum"
        fi
        ${PKG_MGR} install -y curl tar iptables nftables
        # Fedora/EPEL ships charon-systemd, swanctl and VICI in strongswan.
        if ! ${PKG_MGR} install -y strongswan; then
            info "Enabling EPEL repository for strongSwan packages..."
            ${PKG_MGR} install -y epel-release || error "Cannot enable EPEL repository."
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
# A legacy starter-only installation is not enough for SwanPanel's VICI API.
command -v swanctl >/dev/null 2>&1 || error "strongSwan swanctl is missing; cannot run the SwanPanel VPN backend."
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

# 5. Download and verify a native, statically linked Linux release binary.
VERSION="${SWANPANEL_VERSION:-latest}"
info "Downloading ${PRODUCT_NAME} binary (${VERSION})..."

if [[ "${VERSION}" == "latest" ]]; then
    RELEASE_URL="https://api.github.com/repos/${GITHUB_REPO}/releases/latest"
    DOWNLOAD_URL="$(curl -fsSL "${RELEASE_URL}" | grep -E "browser_download_url.*swan-panel-${BIN_ARCH}\"" | cut -d '"' -f 4 | head -n 1 || true)"
    [[ -n "${DOWNLOAD_URL}" ]] || error "Could not resolve a matching latest Linux release asset; refusing to install an older fallback binary."
else
    DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/swan-panel-${BIN_ARCH}"
fi

CHECKSUM_URL="${DOWNLOAD_URL%/*}/checksums.txt"
TMP_BIN="$(mktemp /tmp/swan-panel-download.XXXXXX)"
TMP_CHECKSUMS="$(mktemp /tmp/swan-panel-checksums.XXXXXX)"
trap 'rm -f "${TMP_BIN}" "${TMP_CHECKSUMS}"' EXIT

info "Downloading from: ${DOWNLOAD_URL}"
curl -fsSL -o "${TMP_BIN}" "${DOWNLOAD_URL}" || error "Binary download failed."
curl -fsSL -o "${TMP_CHECKSUMS}" "${CHECKSUM_URL}" || error "Release checksum manifest download failed."

EXPECTED_HASH="$(awk -v name="swan-panel-${BIN_ARCH}" '$2 == name {print $1}' "${TMP_CHECKSUMS}")"
[[ "$(printf '%s\n' "${EXPECTED_HASH}" | wc -l | tr -d ' ')" == "1" ]] || error "Release manifest has missing or duplicate SHA256 entries."
[[ "${EXPECTED_HASH}" =~ ^[[:xdigit:]]{64}$ ]] || error "Missing or invalid SHA256 for swan-panel-${BIN_ARCH}."
ACTUAL_HASH="$(sha256sum "${TMP_BIN}" | awk '{print $1}')"
[[ "${ACTUAL_HASH}" == "${EXPECTED_HASH}" ]] || error "Release SHA256 verification failed; refusing to install."

chmod +x "${TMP_BIN}"
"${TMP_BIN}" --version || error "Downloaded Linux binary cannot execute on this host."
install -m 755 "${TMP_BIN}" "${INSTALL_BIN_PATH}"
success "Verified Linux binary installed to ${INSTALL_BIN_PATH}"

# 6. Create Configuration & Data Directories
info "Configuring environment and data directories..."
mkdir -p "${CONFIG_DIR}"
mkdir -p "${DATA_DIR}"
chmod 750 "${DATA_DIR}"

if [[ ! -f "${CONFIG_DIR}/swan-panel.env" ]]; then
    # Generate a 256-bit secret for JWT signing (64 hex characters)
    # 32 random bytes encoded as hex. No head-pipeline SIGPIPE under pipefail.
    JWT_SECRET="$(od -An -v -N32 -tx1 /dev/urandom | tr -d ' \n')"
    [[ "${#JWT_SECRET}" -eq 64 ]] || error "Failed to generate strong JWT secret."
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

# 8. Prepare strongSwan's credential directories before daemon startup.
mkdir -p /etc/swanctl/x509ca /etc/swanctl/x509 /etc/swanctl/rsa /etc/swanctl/x509crl
chmod 700 /etc/swanctl/rsa

# 9. Start and Enable Services
info "Reloading systemd and enabling services..."
systemctl daemon-reload

# Prefer the VICI-enabled charon-systemd daemon. Do not start two different
# strongSwan daemons: their shared IKE UDP ports would conflict.
if systemctl list-unit-files --no-legend | grep -q '^strongswan.service'; then
    systemctl disable --now strongswan-starter.service >/dev/null 2>&1 || true
    systemctl enable strongswan.service
    systemctl restart strongswan.service
else
    error "VICI-enabled strongswan.service (charon-systemd) is not installed."
fi

# Wait for the actual daemon API, not just a successful systemctl exit.
# SwanPanel requires VICI to load certificates, CRLs and tunnel configuration.
VICI_READY=0
for attempt in $(seq 1 15); do
    if swanctl --stats >/dev/null 2>&1; then
        VICI_READY=1
        break
    fi
    sleep 1
done
[[ "${VICI_READY}" -eq 1 ]] || error "strongSwan did not expose a working VICI socket."

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
printf "  Initial Setup:      Required (no default admin/password)\n"
printf "  Setup Token File:   %s/swanpanel.db.setup-token\n" "${DATA_DIR}"
printf "\n"
printf "  Read the one-time token locally on this server, then create\n"
printf "  your first administrator through the web setup form.\n"
printf "\n"
printf "  Service Status:     sudo systemctl status swan-panel\n"
printf "  Stream Live Logs:   sudo journalctl -u swan-panel -f\n"
printf "  Configuration:      %s/swan-panel.env\n" "${CONFIG_DIR}"
printf "  Database Path:      %s/swanpanel.db\n" "${DATA_DIR}"
printf "\n"
printf "======================================================================\n"
