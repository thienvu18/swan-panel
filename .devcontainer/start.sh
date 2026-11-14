#!/bin/sh
set -eu
umask 077

# SwanPanel writes gateway certificates/CRL into these directories at startup.
mkdir -p /etc/swanctl/x509ca /etc/swanctl/x509 /etc/swanctl/rsa \
    /etc/swanctl/x509crl /var/run /var/lib/swan-panel/pki

# Use a persistent, randomized development JWT secret rather than the static
# nonproduction fallback, because the API is intentionally LAN-accessible.
jwt_file=/var/lib/swan-panel/dev-jwt-secret
if [ ! -f "$jwt_file" ]; then
    (umask 077; openssl rand -hex 32 > "$jwt_file")
fi
export JWT_SECRET="$(cat "$jwt_file")"

# Go embed requires ui/dist to exist even though the dev browser serves Vite.
# Never use the placeholder as the user-facing dev UI.
if [ ! -f /workspace/swan-panel/ui/dist/index.html ]; then
    mkdir -p /workspace/swan-panel/ui/dist
    printf '%s\n' '<!doctype html><html><!-- SWANPANEL_DEV_UI_PLACEHOLDER --><body>Dev UI is served by Vite on port 3000.</body></html>' \
       > /workspace/swan-panel/ui/dist/index.html
fi

echo "SwanPanel dev: Vite :3000, API :8080, IKEv2 UDP :500/:4500, DNS :53"
exec /usr/bin/supervisord -c /workspace/swan-panel/.devcontainer/supervisord.conf
