# Quickstart & Installation

SwanPanel can be deployed in under 60 seconds on any supported Linux server.

---

## 1. Automated 1-Line Installer

The recommended installation method uses the official automated installer script:

```bash
curl -sSL https://raw.githubusercontent.com/thienvu18/swan-panel/main/install.sh | sudo bash
```

### What the installer does:
1. Detects OS distribution (Ubuntu, Debian, CentOS, Rocky Linux, AlmaLinux, Fedora).
2. Installs required system packages: `strongswan`, `strongswan-charon-plugin-vici` / `strongswan-swanctl`, and `iptables`.
3. Enables Linux kernel IPv4/IPv6 packet forwarding (`/etc/sysctl.d/99-swanpanel.conf`).
4. Downloads the matching static binary from GitHub Releases to `/usr/local/bin/swan-panel`.
5. Creates configuration file `/etc/swan-panel/config.env` with a cryptographically secure random `JWT_SECRET`.
6. Installs and starts the systemd service (`swan-panel.service`).

---

## 2. Accessing the Web Control Plane

Once the installer finishes, navigate to:

```text
http://<YOUR_SERVER_IP>:8080
```

- **Default Administrator Email:** `admin@swanpanel.internal`
- **Default Password:** `Admin@SwanPanel123`

You will be prompted to change the administrative password and configure your external server IP / hostname in **Server Settings**.

---

## 3. Service Management

Manage the background daemon using standard systemd commands:

```bash
# Check status
sudo systemctl status swan-panel

# View live application logs
sudo journalctl -u swan-panel -f

# Restart service
sudo systemctl restart swan-panel
```
