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

# 1. Configure forwarding in the active network namespace.
# Host mode changes host-wide settings; IPv6 forwarding is opt-in there because
# enabling it can change IPv6 Router Advertisement/default-route behavior.
if [[ "${SWANPANEL_EXTERNAL_STRONGSWAN:-0}" == "1" ]]; then
    if ! sysctl -w net.ipv4.ip_forward=1 >/dev/null 2>&1 && \
       [[ "$(sysctl -n net.ipv4.ip_forward 2>/dev/null || true)" != "1" ]]; then
        log_warn "Host IPv4 forwarding is disabled and cannot be enabled. Configure it on the host before starting."
        exit 1
    fi
    sysctl -w net.ipv4.conf.all.accept_redirects=0 >/dev/null 2>&1 || log_warn "Could not disable host ICMP redirect acceptance."
    sysctl -w net.ipv4.conf.all.send_redirects=0 >/dev/null 2>&1 || log_warn "Could not disable host ICMP redirect emission."
    if [[ "${SWANPANEL_HOST_ENABLE_IPV6_FORWARDING:-0}" == "1" ]]; then
        if ! sysctl -w net.ipv6.conf.all.forwarding=1 >/dev/null 2>&1 && \
           [[ "$(sysctl -n net.ipv6.conf.all.forwarding 2>/dev/null || true)" != "1" ]]; then
            log_warn "Host IPv6 forwarding was requested but is disabled and cannot be enabled."
            exit 1
        fi
    fi
else
    sysctl -w net.ipv4.ip_forward=1 >/dev/null 2>&1 || true
    sysctl -w net.ipv4.conf.all.accept_redirects=0 >/dev/null 2>&1 || true
    sysctl -w net.ipv4.conf.all.send_redirects=0 >/dev/null 2>&1 || true
    sysctl -w net.ipv6.conf.all.forwarding=1 >/dev/null 2>&1 || true
fi

# 2. Only the default isolated bridge mode uses legacy container masquerading.
# Host mode NAT is managed by SwanPanel's narrowly scoped nftables policy.
if [[ "${SWANPANEL_EXTERNAL_STRONGSWAN:-0}" != "1" ]]; then
    iptables -t nat -C POSTROUTING -o eth0 -j MASQUERADE >/dev/null 2>&1 || \
        iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE >/dev/null 2>&1 || true
fi

# 3. Prepare persistent state and the standard strongSwan certificate
# directories before starting charon. Go also creates them defensively when
# deploying credentials; both safeguards apply to clean production installs.
mkdir -p /var/lib/swan-panel /etc/swan-panel /var/run \
    /etc/swanctl/x509ca /etc/swanctl/x509 /etc/swanctl/rsa /etc/swanctl/x509crl /etc/swanctl/conf.d
chmod 700 /etc/swanctl/rsa
if [[ ! -f /etc/swanctl/swanctl.conf ]]; then
    printf 'include conf.d/*.conf\n' > /etc/swanctl/swanctl.conf
elif ! grep -Eq '^[[:space:]]*include[[:space:]]+conf.d/\*\.conf[[:space:]]*$' /etc/swanctl/swanctl.conf; then
    printf '\ninclude conf.d/*.conf\n' >> /etc/swanctl/swanctl.conf
fi

# 4. Detect or initialize strongSwan daemon
VICI_SOCKET="${VICI_SOCKET:-/var/run/charon.vici}"

if [[ "${SWANPANEL_EXTERNAL_STRONGSWAN:-0}" == "1" ]]; then
    # Fail closed: never remove a host daemon socket or spawn an internal charon.
    if [[ ! -S "${VICI_SOCKET}" ]] || ! swanctl --stats --uri "unix://${VICI_SOCKET}" >/dev/null 2>&1; then
        log_warn "External strongSwan VICI socket ${VICI_SOCKET} is not responding. Refusing to start."
        exit 1
    fi
    log_success "External strongSwan daemon is responsive at ${VICI_SOCKET}."
elif [[ -S "${VICI_SOCKET}" ]] && swanctl --stats --uri "unix://${VICI_SOCKET}" >/dev/null 2>&1; then
    log_info "Active strongSwan daemon detected at ${VICI_SOCKET}. Skipping internal daemon startup."
else
    # Clean up any stale or orphaned socket and pid files from unclean shutdowns
    rm -f "${VICI_SOCKET}" /var/run/charon.pid /var/run/charon.ctl >/dev/null 2>&1 || true
    log_info "Starting internal strongSwan IPsec charon daemon..."

    if command -v ipsec >/dev/null 2>&1; then
        ipsec start >/dev/null 2>&1 || {
            log_warn "ipsec start exited with warning; checking daemon status..."
        }

        # Wait up to 15 seconds for VICI socket to be responsive
        SOCKET_READY=0
        for i in $(seq 1 15); do
            if [[ -S "${VICI_SOCKET}" ]] && swanctl --stats --uri "unix://${VICI_SOCKET}" >/dev/null 2>&1; then
                SOCKET_READY=1
                break
            fi
            sleep 1
        done

        if [[ ${SOCKET_READY} -eq 1 ]]; then
            log_success "Internal strongSwan daemon started (VICI socket active and responsive at ${VICI_SOCKET})."
        else
            log_warn "VICI socket ${VICI_SOCKET} was not responsive within 15s. Continuing startup..."
        fi
    else
        log_warn "strongSwan ipsec binary not found in container. Running in standalone API mode."
    fi
fi

# Load previously persisted gateway connections after internal daemon startup.
# SwanPanel applies subsequent database changes through VICI.
if [[ "${SWANPANEL_EXTERNAL_STRONGSWAN:-0}" != "1" ]] && [[ -S "${VICI_SOCKET}" ]] && command -v swanctl >/dev/null 2>&1; then
    swanctl --load-all --noprompt --uri "unix://${VICI_SOCKET}" >/dev/null 2>&1 || log_warn "Persisted strongSwan configuration will be retried by SwanPanel."
fi

# 5. Execute primary command (SwanPanel binary)
log_info "Handing execution to SwanPanel binary..."
exec "$@"
