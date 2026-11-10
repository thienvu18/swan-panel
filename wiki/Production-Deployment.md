# Production Deployment & Hardening

This guide covers best practices for running SwanPanel securely in high-traffic enterprise production environments.

---

## 1. Reverse Proxy & TLS Termination

While SwanPanel can serve direct HTTP on port 8080, production environments should place SwanPanel behind a reverse proxy (e.g. Caddy or Nginx) with automatic TLS certificates (Let's Encrypt).

### Example Caddy Configuration:
```caddyfile
vpn.yourcompany.com {
    reverse_proxy localhost:8080
}
```

### Example Nginx Configuration:
```nginx
server {
    server_name vpn.yourcompany.com;
    listen 443 ssl http2;

    ssl_certificate /etc/letsencrypt/live/vpn.yourcompany.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/vpn.yourcompany.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # SSE stream buffering disabled
        proxy_set_header Connection '';
        proxy_http_version 1.1;
        chunked_transfer_encoding off;
        proxy_buffering off;
        proxy_cache off;
    }
}
```

---

## 2. Firewall Rules (UFW / iptables)

Allow standard IPsec ports and Web ports:

```bash
# Allow IKEv2 and NAT-Traversal UDP ports
sudo ufw allow 500/udp
sudo ufw allow 4500/udp

# Allow HTTPS for Web Control Plane
sudo ufw allow 443/tcp
sudo ufw allow 80/tcp

# Enable NAT Masquerading for VPN subnets
sudo iptables -t nat -A POSTROUTING -s 10.60.0.0/16 -o eth0 -j MASQUERADE
```

---

## 3. Environment Variables Reference

Configure `/etc/swan-panel/config.env`:

| Variable | Recommended Production Setting | Description |
| :--- | :--- | :--- |
| `APP_ENV` | `production` | Enables production security optimizations |
| `SERVER_PORT` | `8080` | Internal port bound by SwanPanel |
| `SERVER_HOST` | `127.0.0.1` | Bind to localhost when using reverse proxy |
| `JWT_SECRET` | 32-byte cryptographically secure random string | Used for HMAC-SHA256 session token signing |
| `LOG_LEVEL` | `info` or `warn` | Minimum log severity level |
| `VICI_SOCKET` | `/var/run/charon.vici` | Path to strongSwan UNIX socket |
| `ENABLE_DNS` | `true` | Runs integrated Split-DNS server |
| `DNS_PORT` | `53` | Standard DNS port (requires root or `CAP_NET_BIND_SERVICE`) |
