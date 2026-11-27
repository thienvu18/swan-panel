# Production Deployment & Hardening

Use HTTPS for the administration UI, restrict management access, and keep signing secrets and VPN identities in persistent protected storage.

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

# SwanPanel manages firewall and NAT for enrolled VPN addresses.
# Do not add a blanket NAT or ACCEPT rule for the full VPN subnet.
```

---

## 3. Environment Variables Reference

Docker Compose reads `JWT_SECRET` from the repository `.env` and sets service environment variables in `docker-compose.yml`. For the optional host installation, configure `/etc/swan-panel/config.env`:

| Variable | Recommended Production Setting | Description |
| :--- | :--- | :--- |
| `APP_ENV` | `production` | Enables production security optimizations |
| `PORT` | `8080` | Web UI and API port inside the service |
| `JWT_SECRET` | 32-byte cryptographically secure random string | Used for HMAC-SHA256 session token signing |
| `LOG_LEVEL` | `info` or `warn` | Minimum log severity level |
| `VICI_SOCKET` | `/var/run/charon.vici` | UNIX socket used by the bundled strongSwan daemon; host deployments require a reachable host socket |
| `ENABLE_DNS` | `true` | Runs integrated Split-DNS server |
| `DNS_PORT` | `53` (Compose) | DNS port inside the container; host mapping defaults to 53 |
| `MFA_KEY_FILE` | `DB_PATH + ".mfa-key"` | Persistent `0600` file storing the independent TOTP encryption key (migrated automatically from legacy `JWT_SECRET`, bundled in encrypted backups) |
| `WEBHOOK_ALLOWED_DESTINATIONS` | *(Empty — public HTTPS only)* | Optional comma-separated private HTTPS CIDRs, hosts, or `host:port` entries allowed for alert webhooks (no wildcards; loopback/link-local metadata remain blocked) |

Keep a restart policy enabled (`restart: unless-stopped` in Docker Compose or `Restart=always` in systemd) so encrypted backup restore can complete its controlled restart into the startup restore journal.
