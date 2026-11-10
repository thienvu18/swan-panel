<div align="center">

# SwanPanel

**Modern web control plane for strongSwan Remote Access VPN with 100% native client provisioning**

[![GitHub Release](https://img.shields.io/github/v/release/thienvu18/swan-panel?style=flat-square)](https://github.com/thienvu18/swan-panel/releases)
[![CI](https://github.com/thienvu18/swan-panel/actions/workflows/ci.yml/badge.svg)](https://github.com/thienvu18/swan-panel/actions/workflows/ci.yml)
[![Pages](https://img.shields.io/badge/docs-GitHub_Pages-blue?style=flat-square)](https://thienvu18.github.io/swan-panel/)
[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square)](https://golang.org)
[![strongSwan](https://img.shields.io/badge/strongSwan-VICI_IPC-crimson?style=flat-square)](https://github.com/strongswan/strongswan)
[![License](https://img.shields.io/badge/License-AGPL--3.0-gray?style=flat-square)](LICENSE)

[Website & Docs](https://thienvu18.github.io/swan-panel/) • [English](README.md) • [Tiếng Việt](README.vi.md) • [Wiki](https://github.com/thienvu18/swan-panel/wiki)

</div>

---

## Overview

**SwanPanel** is a lightweight, single-binary web control plane designed for [strongSwan](https://github.com/strongswan/strongswan), the industry-standard open-source IPsec/IKEv2 VPN daemon.

While strongSwan provides high-performance, kernel-accelerated IPsec encryption and native compatibility across all major operating systems, administrating it via CLI requires terminal SSH access, editing complex text files (`swanctl.conf`, `ipsec.conf`), and generating PKI certificates manually. SwanPanel wraps strongSwan into an intuitive zero-trust control plane: managing connections dynamically in memory, issuing client certificates, and provisioning native OS VPN profiles with one click.

---

## Key Features

### 1. 100% Native Client Provisioning (Zero 3rd-Party Apps)
* **Apple Ecosystem (macOS & iOS):** Generates signed `.mobileconfig` profiles with embedded PKCS#12 client certificates and CA trust chain for 1-click installation in System Settings.
* **Windows 10 & 11:** Auto-generates idempotent PowerShell scripts (`Add-VpnConnection`) configuring split-tunnel routing and modern IKEv2 cipher suites.
* **Linux:** Generates NetworkManager connection files (`.nmconnection`) or standard `swanctl.conf` templates.
* **Android:** Native IKEv2/IPsec RSA certificate and MSCHAPv2 configuration profiles.

### 2. Zero-SSH strongSwan Control via VICI IPC
* **Live In-Memory Reloads:** Communicates directly with the `charon` daemon via strongSwan's official UNIX domain socket (`/var/run/charon.vici`) using the `govici` protocol. Reloads connections in RAM without dropping active tunnels.
* **Real-Time Session Management:** Terminate active sessions immediately, enforce re-authentication lifetimes, and monitor tunnel status directly from the web interface.
* **Data Plane Configuration:** Configure MOBIKE roaming, force UDP NAT-Traversal encapsulation, dead peer detection (DPD) keepalive intervals, fragmentation, and TCP MSS clamping.

### 3. Automated Internal PKI (X.509)
* **Zero-Touch Certificate Authority:** Automatically initializes and manages the root CA, server certificates, and client certificates.
* **Automated Packaging:** Generates password-protected PKCS#12 (.p12) bundles directly for native client import without requiring OpenSSL or CFSSL command-line tools.

### 4. Configurable Split-DNS & Custom DNS Overrides
* **Fully Customizable MagicDNS:** Embedded high-concurrency DNS server (`github.com/miekg/dns`). The internal domain suffix is completely configurable (e.g. `.internal`, `.corp`, `.vpn`, `.strongnet`) and resolves directly to assigned virtual device IPs.
* **Custom DNS Overrides:** Manually define custom DNS records (e.g. mapping `gitlab.company.internal` or `api.internal` to specific target IPs) with instant propagation across connected VPN clients.
* **Upstream Forwarding:** Seamlessly forwards non-internal DNS queries to configurable corporate or public resolvers (1.1.1.1, 8.8.8.8).

### 5. Live Operations & Real-Time Dashboard
* **Server-Sent Events (SSE):** Real-time event broker pushes active session updates, bandwidth metrics, and log streams to connected browsers without client-side polling loops.
* **Centralized Log Streaming:** Inspect and filter strongSwan daemon events by user, device, and log severity levels.

### 6. Zero-Trust Access Control & Identity Directory
* **Multi-IdP SSO & Local Directory:** Authenticate admins and users via OIDC providers (Google, Microsoft Azure AD, Okta, Keycloak) or local credentials.
* **Granular Access Policies:** Group-based routing rules and CIDR access permissions (Production VPC, Staging, Engineering Subnets).

### 7. Single-Binary, Zero-Dependency Architecture
* **Self-Contained Executable:** Go backend, embedded pure Go SQLite database (in high-performance WAL mode), embedded React 19 SPA, PKI, and DNS engine compile into a single executable.
* **No External Database Required:** Runs without PostgreSQL, MySQL, Redis, or container orchestration dependencies.

---

## Why SwanPanel?

### Compared to Raw strongSwan (CLI Only)

| Feature | Raw strongSwan (CLI) | SwanPanel |
| :--- | :--- | :--- |
| **Daemon Administration** | Manual SSH + editing `swanctl.conf` / `ipsec.conf` | Web UI via `/var/run/charon.vici` socket |
| **Config Reloading** | Restarting daemon risks dropping tunnels | RAM hot-reload (`load-conn`) with zero downtime |
| **Certificate Management**| Manual OpenSSL / CFSSL command line | Automated internal CA & PKCS#12 bundle generation |
| **Client Provisioning** | Manual guides for importing certs & setting routes | 1-click profiles (Apple `.mobileconfig`, Windows PowerShell) |
| **Internal DNS** | Requires external BIND / Unbound server setup | Built-in Split-DNS with custom domain overrides |
| **Monitoring** | Running `swanctl --list-sas` in terminal | Live real-time SSE dashboard with bandwidth stats |

### Compared to Other VPNs (OpenVPN, WireGuard, Tailscale)

* **No Third-Party Client Software:**
  OpenVPN, WireGuard, and Tailscale require every user to install, configure, and maintain third-party client apps on their laptops and phones. strongSwan uses native **IKEv2/IPsec**, which is already built directly into the OS kernel of iOS, macOS, Windows 10/11, Android, and Linux.
* **Single-Binary Zero-Dependency Deployment:**
  Unlike multi-service VPN platforms requiring Docker Compose, PostgreSQL, and Redis, SwanPanel compiles into **one single executable**.
* **100% Self-Hosted & Sovereign:**
  Unlike proprietary zero-trust clouds (Tailscale, Cloudflare Zero Trust), your control plane, data plane, cryptographic keys, and metadata remain strictly on your own hardware with zero vendor lock-in.

---

## Architecture

```mermaid
flowchart TD
    subgraph Clients["Native OS Clients (No 3rd-Party App)"]
        Apple["Apple (iOS / macOS .mobileconfig)"]
        Win["Windows (PowerShell Add-VpnConnection)"]
        Android["Android (Native IKEv2)"]
        Linux["Linux (NetworkManager / swanctl)"]
    end

    subgraph Core["SwanPanel (Single Binary)"]
        UI["Embedded React 19 SPA\n(Tailwind CSS + TanStack Query)"]
        API["Go Core REST API & SSE Broker"]
        PKI["Internal X.509 PKI"]
        DNS["Custom Split-DNS & Overrides"]
        DB[("SQLite WAL Engine")]
    end

    subgraph DataPlane["strongSwan Data Plane"]
        Socket["UNIX Socket\n/var/run/charon.vici"]
        Daemon["strongSwan charon Daemon\n(IPsec / IKEv2 Gateway)"]
    end

    Apple -->|Native IKEv2 / IPsec| Daemon
    Win -->|Native IKEv2 / IPsec| Daemon
    Android -->|Native IKEv2 / IPsec| Daemon
    Linux -->|Native IKEv2 / IPsec| Daemon

    UI <-->|HTTP / SSE Stream| API
    API <--> DB
    API <--> PKI
    API <--> DNS
    API <-->|govici IPC| Socket
    Socket <--> Daemon
```

---

## Quick Start

### 1. One-Line Automated Production Install (Recommended)
Run the automated installation script on any clean Ubuntu, Debian, Rocky Linux, or AlmaLinux server:
```bash
curl -fsSL https://raw.githubusercontent.com/thienvu18/swan-panel/main/install.sh | sudo bash
```

### 2. Build from Source
```bash
git clone https://github.com/thienvu18/swan-panel.git
cd swan-panel

# Compiles embedded React 19 UI and single Go binary
cd ui && npm install && npm run build && cd ..
go build -o swan-panel ./cmd/server

# Run SwanPanel
./swan-panel
```
Access the control plane at `http://localhost:8080`.

### 3. Default Admin Credentials
* **Email:** `admin@swanpanel.internal`
* **Password:** `admin123456`

*(Remember to change the default password immediately in production).*

---

## Production Deployment (systemd)

For a production Linux server running strongSwan, deploy SwanPanel as a managed `systemd` service:

1. Move the binary to `/usr/local/bin/swan-panel`:
   ```bash
   sudo cp swan-panel /usr/local/bin/swan-panel
   sudo chmod +x /usr/local/bin/swan-panel
   ```

2. Create `/etc/systemd/system/swan-panel.service`:
   ```ini
   [Unit]
   Description=SwanPanel strongSwan Web Control Plane
   After=network.target strongswan.service strongswan-starter.service
   Requires=strongswan.service

   [Service]
   Type=simple
   User=root
   WorkingDirectory=/var/lib/swan-panel
   ExecStart=/usr/local/bin/swan-panel
   Restart=always
   RestartSec=5s
   Environment="PORT=8080"
   Environment="DB_PATH=/var/lib/swan-panel/swanpanel.db"
   Environment="JWT_SECRET=your-strong-random-production-secret"
   Environment="LOG_LEVEL=info"
   Environment="VICI_SOCKET=/var/run/charon.vici"
   Environment="DNS_PORT=53"
   Environment="APP_ENV=production"

   [Install]
   WantedBy=multi-user.target
   ```

3. Enable and start the service:
   ```bash
   sudo mkdir -p /var/lib/swan-panel
   sudo systemctl daemon-reload
   sudo systemctl enable --now swan-panel
   ```

---

## Configuration Reference

Configure SwanPanel via environment variables or a `.env` file:

| Variable | Default | Description |
| :--- | :--- | :--- |
| `PORT` | `8080` | HTTP port for web control plane and REST API |
| `DB_PATH` | `swanpanel.db` | SQLite database file location |
| `JWT_SECRET` | *(Random string)* | Key for signing JWT session tokens (**change in production**) |
| `LOG_LEVEL` | `info` | Logging verbosity: `debug`, `info`, `warn`, `error` |
| `VICI_SOCKET` | `/var/run/charon.vici` | Path to strongSwan charon UNIX domain socket |
| `ENABLE_DNS` | `true` | Enable built-in Split-DNS engine |
| `DNS_PORT` | `5353` | Port for the DNS server (`53` for root/production) |
| `APP_ENV` | `development` | Environment mode (`development` or `production`) |

---

## Engineering & Security Standards

SwanPanel adheres to strict enterprise software standards:
* **No SQL Injection:** 100% parameterized queries via GORM ORM; dynamic SQL string concatenations are strictly forbidden.
* **OWASP Hardened:** Standard HTTP security headers configured out of the box (CSP, HSTS, `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`).
* **Timing-Safe Cryptography:** Authentication tokens, API keys, and hashes are compared in constant time (`crypto/subtle.ConstantTimeCompare`).
* **Rate Limiting:** In-memory token-bucket rate limiters on all public authentication routes.

---

## Project Philosophy

SwanPanel was created around three foundational principles:
1. **Embrace Native Capabilities:** Modern operating systems already ship with robust, hardware-accelerated IKEv2/IPsec stacks. Users should not be forced to install third-party background applications just to connect to their private infrastructure.
2. **Minimalist Architecture:** No bloated orchestration stacks. A single self-contained binary running alongside strongSwan provides everything required for production operations.
3. **True Network Sovereignty:** Your credentials, keys, access rules, and network traffic belong entirely to you, free from SaaS subscriptions and cloud vendor lock-in.

---

## Community & Contributing

SwanPanel is a community-driven open-source project. We warmly welcome contributions:

### How to Contribute:
* **Report Bugs & Suggest Features:** Open an issue on GitHub detailing the scenario, steps to reproduce, or workflow proposal.
* **Submit Pull Requests:** Check existing issues, follow the code guidelines in [AGENTS.md](AGENTS.md), and ensure all tests pass (`go test -v ./...`).
* **Add Translations (i18n):**
  1. Duplicate `ui/src/locales/en.json` to `ui/src/locales/<language-code>.json`.
  2. Translate the values while preserving JSON keys.
  3. Register the new locale in `ui/src/i18n.ts` and open a PR!

---

## Authors & Contributors

### Creator & Lead Maintainer
* **Thái Thiên Vũ** ([@thienvu18](https://github.com/thienvu18))

<a href="https://github.com/thienvu18">
  <img src="https://github.com/thienvu18.png?size=80" width="80" height="80" alt="Thái Thiên Vũ" style="border-radius: 50%;" />
</a>

### Contributors
We thank all contributors who help improve SwanPanel!

<a href="https://github.com/thienvu18/swan-panel/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=thienvu18/swan-panel" alt="Contributors" />
</a>

---

## Documentation Links

* [System Architecture & DB Specs](ARCHITECTURE.md) — GORM relational schema, REST endpoints, and VICI workflow.
* [UI/UX Design Specification](DESIGN.md) — Google Labs `design.md` compliance, design tokens, and components.
* [AI Agent Guidelines](AGENTS.md) — Coding conventions, security mandates, and build instructions.

---

## License

SwanPanel is licensed under the **GNU Affero General Public License v3.0 (AGPL-3.0)**.

* **Commercial Use:** Permitted for free internal corporate use to manage private VPN infrastructure.
* **Strong Copyleft:** Any modifications made to SwanPanel that are served over a network must be open-sourced under the same AGPL-3.0 license terms.
* **Anti-SaaS Clause:** Third parties and cloud providers may not offer SwanPanel as a commercial hosted service/SaaS to customers without releasing their entire management stack under AGPL-3.0 or obtaining an explicit commercial license.
