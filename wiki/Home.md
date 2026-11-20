# Welcome to the SwanPanel Wiki

**SwanPanel** is a modern, enterprise-grade, zero-trust web control plane for **strongSwan** Remote Access VPNs.

**SwanPanel v1.0.1:** Native VPN on supported devices, clear access decisions, verified Gateway health and visible configuration apply status. [Release notes](https://github.com/thienvu18/swan-panel/blob/main/CHANGELOG.md).

![SwanPanel Dashboard](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/02_dashboard.png)

---

## Core Wiki Navigation

- [Quickstart & Installation Guide](Quickstart)
- [Administrator Operations Guide](Administrator-Guide)
- [End-User Native Client Guide](End-User-Guide)
- [Architecture & Internals](Architecture)
- [Native Client Provisioning](Client-Provisioning)
- [Split-DNS & MagicDNS Engine](DNS-and-MagicDNS)
- [Zero-Trust Access Policies](Zero-Trust-Policies)
- [REST API Reference](API-Reference)
- [Production Deployment & Hardening](Production-Deployment)
- [Troubleshooting & FAQ](Troubleshooting)

---

## Key Highlights

1. **Native VPN, simpler onboarding:** Use built-in VPN support on compatible Apple, Windows, Linux and Android devices; Android IKEv2/IPsec RSA availability varies by device.
2. **Manage access in one place:** Review effective ALLOW/DENY rules, update settings and see whether changes are active without editing VPN files manually. Current Gateway health is shown as Online, Degraded, Offline or Unknown, including on mobile.
3. **Single-Binary Delivery:** Go backend, embedded React 19 SPA, SQLite database, internal PKI, and DNS server compile into one self-contained executable.
4. **Data Sovereignty:** Free, open-source AGPL-3.0 software running on your own infrastructure without vendor lock-in or SaaS fees.
