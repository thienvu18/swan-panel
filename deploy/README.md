# SwanPanel Production Deployment Guide

This directory contains production deployment configurations for **SwanPanel**:

1. **Docker & Docker Compose**: Automated multi-stage container deployment with strongSwan integration.
2. **Systemd Service**: Native Linux daemon deployment for Ubuntu, Debian, RHEL, Rocky Linux, and Fedora.

---

## Architecture Overview

SwanPanel compiles into a single, self-contained binary containing:
* Go 1.27 backend control plane
* Embedded React 19 SPA
* Embedded SQLite database engine (WAL mode)
* Autonomous internal PKI (X.509 RSA-4096 / ECC)
* Embedded Split-DNS server (`:53`)
* strongSwan VICI IPC integration (`/var/run/charon.vici`)

---

## 1. Docker Compose Deployment (Recommended)

### Prerequisites
* Docker 24.0+ and Docker Compose v2.20+
* Linux host with IP forwarding enabled

### Quickstart
1. Clone the repository and navigate to root:
   ```bash
   git clone https://github.com/thienvu18/swan-panel.git
   cd swan-panel
   ```

2. Generate a secure random JWT secret:
   ```bash
   export JWT_SECRET="$(head -c 32 /dev/urandom | base64 | tr -dc 'a-zA-Z0-9' | head -c 32)"
   ```

3. Start the container in background:
   ```bash
   docker compose up -d
   ```

4. Verify service status and logs:
   ```bash
   docker compose ps
   docker compose logs -f
   ```

5. Access the Web Dashboard:
   * **URL:** `http://<SERVER_IP>:8080`
   * **Default Admin Email:** `admin@swanpanel.internal`
   * **Default Password:** `admin123456`
   *(Change the default password immediately after first sign-in)*

---

## 2. Standalone Docker Run

To run without Docker Compose:

```bash
docker run -d \
  --name swan-panel \
  --restart unless-stopped \
  --cap-add=NET_ADMIN \
  --cap-add=NET_RAW \
  -p 8080:8080/tcp \
  -p 53:53/udp \
  -p 53:53/tcp \
  -p 500:500/udp \
  -p 4500:4500/udp \
  -v /var/run/charon.vici:/var/run/charon.vici \
  -v swanpanel_data:/var/lib/swan-panel \
  -v swanpanel_config:/etc/swan-panel \
  -e ENVIRONMENT=production \
  -e LOG_LEVEL=INFO \
  -e JWT_SECRET="your-secure-random-secret-here" \
  swan-panel:latest
```

> [!NOTE]
> When `/var/run/charon.vici` is not mounted from the host, the container automatically starts its internal strongSwan charon daemon in standalone mode.

---

## 3. Native Linux Deployment (Systemd)

For sysadmins deploying natively on Linux hosts (Ubuntu, Debian, RHEL, Rocky, AlmaLinux, Fedora).

### Step 1: Install Dependencies & strongSwan
* **Ubuntu / Debian:**
  ```bash
  sudo apt update
  sudo apt install -y strongswan strongswan-charon-systemd libcharon-extra-plugins iptables nftables curl
  ```
* **RHEL / Rocky / AlmaLinux / Fedora:**
  ```bash
  sudo dnf install -y epel-release
  sudo dnf install -y strongswan iptables nftables curl
  ```

### Step 2: Enable Kernel Packet Forwarding
```bash
sudo mkdir -p /etc/sysctl.d
cat <<'EOF' | sudo tee /etc/sysctl.d/99-swanpanel.conf
net.ipv4.ip_forward = 1
net.ipv4.conf.all.accept_redirects = 0
net.ipv4.conf.all.send_redirects = 0
net.ipv6.conf.all.forwarding = 1
net.ipv6.conf.all.accept_redirects = 0
EOF

sudo sysctl -p /etc/sysctl.d/99-swanpanel.conf
```

### Step 3: Install Binary
Download the precompiled binary from [GitHub Releases](https://github.com/thienvu18/swan-panel/releases):
```bash
sudo install -m 755 swan-panel /usr/local/bin/swan-panel
```

### Step 4: Configure Environment & Service
1. Create directories:
   ```bash
   sudo mkdir -p /etc/swan-panel /var/lib/swan-panel
   sudo chmod 750 /var/lib/swan-panel
   ```

2. Copy the sample environment file:
   ```bash
   sudo cp deploy/swan-panel.env.example /etc/swan-panel/swan-panel.env
   sudo chmod 600 /etc/swan-panel/swan-panel.env
   ```

3. Install the systemd unit file:
   ```bash
   sudo cp deploy/systemd/swan-panel.service /etc/systemd/system/swan-panel.service
   sudo systemctl daemon-reload
   ```

4. Enable and start services:
   ```bash
   sudo systemctl enable --now strongswan-starter || sudo systemctl enable --now strongswan
   sudo systemctl enable --now swan-panel
   ```

5. Check status and stream logs:
   ```bash
   sudo systemctl status swan-panel
   sudo journalctl -u swan-panel -f
   ```

---

## 4. Network Firewall Configuration

Ensure your VPS firewall allows the following inbound ports:

| Port | Protocol | Purpose |
| :--- | :--- | :--- |
| **8080** | TCP | SwanPanel Web Control Plane UI & REST API |
| **500** | UDP | strongSwan IKEv2 Key Exchange |
| **4500** | UDP | strongSwan IPsec NAT-Traversal |
| **53** | UDP/TCP | SwanPanel Split-DNS Resolver (internal VPN DNS) |

### UFW (Ubuntu/Debian):
```bash
sudo ufw allow 8080/tcp comment "SwanPanel Web UI"
sudo ufw allow 500/udp comment "IKEv2 VPN"
sudo ufw allow 4500/udp comment "IPsec NAT-T"
sudo ufw allow 53/udp comment "SwanPanel DNS"
```

### Firewalld (RHEL/Rocky/Fedora):
```bash
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --permanent --add-port=500/udp
sudo firewall-cmd --permanent --add-port=4500/udp
sudo firewall-cmd --permanent --add-port=53/udp
sudo firewall-cmd --reload
```
