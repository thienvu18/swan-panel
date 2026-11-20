# SwanPanel Production Deployment Guide

This directory contains production deployment configurations for **SwanPanel**:

1. **Docker & Docker Compose**: Automated multi-stage container deployment with strongSwan integration.
2. **Systemd Service**: Native Linux daemon deployment for Ubuntu, Debian, RHEL, Rocky Linux, and Fedora.

---

## Architecture Overview

SwanPanel compiles into a single, self-contained binary containing:
* Go 1.27 backend control plane
* Embedded React 19 SPA
* Native SQLite engine via the official GORM SQLite driver (CGO at build time, statically linked into Linux release binaries; WAL mode)
* Autonomous internal PKI (X.509 RSA-4096 / ECC)
* Embedded Split-DNS server (`:53`)
* strongSwan VICI IPC integration (`/var/run/charon.vici`)

---

## Release binaries and native SQLite

- Supported SwanPanel **gateway/server** platforms: Linux AMD64 and Linux ARM64.
- GitHub Release publishes `swan-panel-linux-amd64`, `swan-panel-linux-arm64` and `checksums.txt`.
- Each binary is built on a native Linux runner with `CGO_ENABLED=1` and musl static linking. GCC, musl, and libsqlite3 **are not required on the target machine**. Compilation from source requires an appropriate native C compiler.
- Local macOS builds are for development only; macOS is supported as a **VPN client**, not a SwanPanel gateway/server release target.
- To inspect a released binary: `./swan-panel-linux-amd64 --version` and `file swan-panel-linux-amd64`.
- To verify the published checksums: `sha256sum -c checksums.txt` on Linux after downloading both binaries.

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

2. On a new installation, generate and persist a secure JWT signing secret:
   ```bash
   umask 077
   printf 'JWT_SECRET=%s\n' "$(openssl rand -hex 32)" > .env
   ```
   Keep `.env` across restarts and upgrades; do not overwrite an existing signing secret.

3. Start the container in background:
   ```bash
   docker compose up -d
   ```

4. Verify service status and logs:
   ```bash
   docker compose ps
   docker compose logs -f
   ```

5. Open the Web Dashboard at `http://<SERVER_IP>:8080` (use HTTPS in production).
   * **No default administrator account exists.**
   * Read the one-time token from the server: `docker compose exec swan-panel cat /var/lib/swan-panel/swanpanel.db.setup-token`.
   * Enter it in the initial setup screen and create an administrator with your own email and password.

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
  -v swanpanel_data:/var/lib/swan-panel \
  -v swanpanel_config:/etc/swan-panel \
  -v swanpanel_swanctl:/etc/swanctl \
  -e ENVIRONMENT=production \
  -e LOG_LEVEL=INFO \
  -e JWT_SECRET="your-secure-random-secret-here" \
  swan-panel:latest
```

> [!NOTE]
> Standalone Docker Run uses the container's own strongSwan daemon and dedicated persistent swanctl volume. Do not bind a host VICI socket into a bridge-network container: XFRM, DNS VIP, nftables, and NAT would operate in different network namespaces.

---

## 2.1 Optional Docker Control Plane with Host strongSwan (advanced)

The recommended **one-command single-container Docker bridge deployment remains unchanged**.
For an intentionally pre-provisioned host `charon-systemd`, the opt-in override
`deploy/docker/docker-compose.host-swan.yml` shares the host network namespace
so IKEv2, XFRM, gateway DNS aliases, nftables, and NAT apply to the same kernel.
It requires Docker Compose 2.24.4+, exclusive host IKE UDP 500/4500,
available DNS TCP/UDP 53 **on SwanPanel gateway VIPs** and management TCP 8080,
and persistent `/etc/swanctl` credentials matched to the existing database.
Host DNS binds only `lo:swanpanel` gateway VIPs, preserving host
`systemd-resolved` on `127.0.0.53/54:53`. Host networking
has no Docker port publishing: the application binds its configured ports
directly on the host. `HOST_DNS_PORT`, `IKE_PORT`, and `IPSEC_PORT` only
control bridge-mode publishing, not host-mode listeners. A running default
container must be stopped before starting
the host daemon. Do **not** copy the default Docker PKI volume over the host
`/etc/swanctl` without backup, collision review, and an approved maintenance window.

```bash
sh scripts/test-host-swan-compose.sh
docker compose -f docker-compose.yml -f deploy/docker/docker-compose.host-swan.yml config --quiet
# After approved PKI migration, stopped default container and healthy host VICI:
docker compose -f docker-compose.yml -f deploy/docker/docker-compose.host-swan.yml up -d
```

Host mode adds no helper service. SwanPanel still manages tunnels/policies over
VICI, but intentionally disables daemon Start/Stop/Restart from the container UI
until a separately authorized host control mechanism exists. The host nftables
forward/input deny rules and MSS clamping match only SwanPanel-assigned IPv4
VIPs (including revoked devices); host NAT matches active VIPs and the selected
WAN interface. Do not overlap SwanPanel VIPs with another VPN. Host mode requires IPv4 forwarding and may enable it or disable IPv4 redirects on the host;
IPv6 forwarding is left unchanged unless `SWANPANEL_HOST_ENABLE_IPV6_FORWARDING=1`
is explicitly set (which can alter Router Advertisement/default routes).
If Docker uses its iptables backend with `FORWARD DROP`, Host mode automatically
maintains an owned `SWANPANEL-FWD` chain and one `DOCKER-USER` jump. Only enrolled
SwanPanel device VIPs (`/32`) with a Zero-Trust nftables authorization mark
can pass, with established return traffic; no manual subnet-wide `ACCEPT` is
required. Device/pool changes and Docker chain resets are reconciled automatically.
An existing Docker nftables backend or a custom `FORWARD DROP` without
`DOCKER-USER` is not supported by this integration and fails closed. Remove any
legacy broad manual VPN `DOCKER-USER ACCEPT` rules after reviewing their ownership;
SwanPanel never deletes rules it did not create.
SwanPanel may update its managed `/etc/swanctl` configuration; review existing routing,
DNS, firewall, and certificate configuration before opting in. Follow the Host-mode preflight and acceptance checklist in
[VPN Gateway Acceptance Test Plan](../docs/VPN_GATEWAY_ACCEPTANCE_TEST_PLAN.md).
For reusable build checks, see the [release build and testing guide](../docs/RELEASE_BUILD_TESTING.md).

---

## 3. Native Linux Deployment (Systemd)

For sysadmins deploying natively on Linux hosts (Ubuntu, Debian, RHEL, Rocky, AlmaLinux, Fedora).

### Step 1: Install Dependencies & strongSwan
* **Ubuntu / Debian:**
  ```bash
  sudo apt update
  sudo apt install -y charon-systemd strongswan-swanctl strongswan-libcharon libcharon-extra-plugins libstrongswan-extra-plugins libstrongswan-standard-plugins iptables nftables curl
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
# Enable IPv6 forwarding separately only if required by your network.
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
   For native `charon-systemd`, keep `SWANPANEL_EXTERNAL_STRONGSWAN=1` in this file
   so host nftables/NAT and Docker forwarding are managed in the host namespace.

3. Install the systemd unit file:
   ```bash
   sudo cp deploy/systemd/swan-panel.service /etc/systemd/system/swan-panel.service
   sudo systemctl daemon-reload
   ```

4. Enable and start services:
   ```bash
   sudo systemctl disable --now strongswan-starter 2>/dev/null || true
   sudo systemctl enable --now strongswan
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
