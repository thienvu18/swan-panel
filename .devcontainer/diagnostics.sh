#!/bin/sh
# Run inside the development container. This reports real daemon/kernel state.
set -u

echo "=== Runtime versions ==="
go version
node --version
air -v
ipsec version
echo

echo "=== Managed development services ==="
supervisorctl -c /workspace/swan-panel/.devcontainer/supervisord.conf status || true
echo

echo "=== Go HTTP health ==="
curl -fsS --max-time 4 http://127.0.0.1:8080/api/health || echo "API not ready"
echo
echo

echo "=== Vite UI ==="
curl -sS -o /dev/null -w 'Vite HTTP status: %{http_code}\n' --max-time 4 http://127.0.0.1:3000/ || true
echo

echo "=== strongSwan VICI ==="
if [ -S "${VICI_SOCKET:-/var/run/charon.vici}" ]; then
    echo "VICI socket exists"
    swanctl --stats || echo "VICI socket exists, but the daemon query failed"
else
    echo "VICI socket missing"
fi
echo

echo "=== Linux IPsec kernel support ==="
if [ -e /proc/net/xfrm_stat ]; then
    echo "XFRM statistics available:"
    sed -n '1,8p' /proc/net/xfrm_stat
else
    echo "No /proc/net/xfrm_stat; IPsec kernel support may be unavailable"
fi
ip xfrm state || echo "Cannot inspect XFRM state (possibly unsupported/denied)"
echo

echo "=== nftables ==="
nft list table inet swanpanel || echo "No SwanPanel nftables table yet (or insufficient capabilities)"
echo

echo "Test API/UI from another device using http://MAC_LAN_IP:3000."
echo "UDP :500/:4500 publication does not prove that Docker Desktop supports end-to-end IPsec."
