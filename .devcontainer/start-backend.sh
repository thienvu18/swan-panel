#!/bin/sh
set -eu
# Let strongSwan create its VICI socket before the initial reconciliation.
# A missing kernel XFRM feature should not prevent application development.
count=0
until [ -S "${VICI_SOCKET:-/var/run/charon.vici}" ] || [ "$count" -ge 20 ]; do
    count=$((count + 1))
    sleep 1
done
if [ ! -S "${VICI_SOCKET:-/var/run/charon.vici}" ]; then
    echo "WARNING: strongSwan VICI socket unavailable; backend will report degraded VPN status." >&2
fi
exec /usr/local/bin/air -c .devcontainer/air.toml
