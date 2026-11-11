#!/usr/bin/env bash
# ==============================================================================
# SwanPanel - Docker Container Entrypoint
# https://github.com/thienvu18/swan-panel
# ==============================================================================

set -eo pipefail

log_info() {
    printf "[*] %s\n" "$*"
}

log_success() {
    printf "[+] %s\n" "$*"
}

log_warn() {
    printf "[!] %s\n" "$*"
}

log_info "Initializing SwanPanel container environment..."

# 1. Attempt kernel IP forwarding optimization if permitted by container capabilities
sysctl -w net.ipv4.ip_forward=1 >/dev/null 2>&1 || true
sysctl -w net.ipv4.conf.all.accept_redirects=0 >/dev/null 2>&1 || true
sysctl -w net.ipv4.conf.all.send_redirects=0 >/dev/null 2>&1 || true
sysctl -w net.ipv6.conf.all.forwarding=1 >/dev/null 2>&1 || true

# 2. Ensure data and config directories exist
mkdir -p /var/lib/swan-panel /etc/swan-panel /var/run

# 3. Detect or initialize strongSwan daemon
VICI_SOCKET="${VICI_SOCKET:-/var/run/charon.vici}"

if [[ -S "${VICI_SOCKET}" ]]; then
    log_info "External strongSwan daemon detected at ${VICI_SOCKET}. Skipping internal daemon startup."
else
    log_info "No external strongSwan socket found at ${VICI_SOCKET}."
    log_info "Starting internal strongSwan IPsec charon daemon..."

    if command -v ipsec >/dev/null 2>&1; then
        ipsec start >/dev/null 2>&1 || {
            log_warn "ipsec start exited with warning; checking daemon status..."
        }

        # Wait up to 10 seconds for VICI socket creation
        SOCKET_READY=0
        for i in $(seq 1 10); do
            if [[ -S "${VICI_SOCKET}" ]]; then
                SOCKET_READY=1
                break
            fi
            sleep 1
        done

        if [[ ${SOCKET_READY} -eq 1 ]]; then
            log_success "Internal strongSwan daemon started (VICI socket active at ${VICI_SOCKET})."
        else
            log_warn "VICI socket ${VICI_SOCKET} was not created within 10s. Continuing startup..."
        fi
    else
        log_warn "strongSwan ipsec binary not found in container. Running in standalone API mode."
    fi
fi

# 4. Execute primary command (SwanPanel binary)
log_info "Handing execution to SwanPanel binary..."
exec "$@"
