# ARCHITECTURE.md — System Architecture & Technical Specifications

**Product:** SwanPanel (`swan-panel`)  
**Version:** 1.0.1
**Scope:** Zero-Trust IKEv2 / strongSwan Modern Control Plane (Enterprise Grade)  
**Standard:** Single-Binary Go Backend, Embedded React 19 SPA, 100% Native VPN Clients  

---

## 1. CORE ARCHITECTURAL PRINCIPLES

1. **100% Native VPN Client Support:**
   * No third-party client apps required. Native Apple `.mobileconfig` (macOS/iOS with embedded PKCS#12 cert), Windows PowerShell (`Add-VpnConnection`), Android IKEv2, Linux NetworkManager.
2. **Single-Binary Zero-Dependency Deployment:**
   * Single compiled executable containing Go backend, SQLite DB (WAL mode), embedded DNS server, internal X.509 PKI, and embedded React SPA (`//go:embed`).
   * Recommended gateway deployment: one Docker Compose service with bundled strongSwan in bridge networking. The opt-in host-charon override shares the host network namespace; it scopes nftables IPsec denial and MSS changes to SwanPanel-assigned per-device `/32` VIPs, and NAT to active VIPs and validated WAN egress.
3. **No Wheel Reinvention:**
   * Battle-tested libraries: Gin (~78k stars), GORM (~37k stars), govici (official strongSwan), miekg/dns (~8.3k stars), react-i18next (~8.5k stars), TanStack Query (~45k stars), Lucide React.
4. **Zero-SSH Operations:**
   * Dynamic in-memory configuration reload and session teardown via UNIX socket `/var/run/charon.vici`.
5. **Enterprise Security Practices:**
   * 100% parameterized queries (prevent SQL injection), OWASP security headers (CSP, HSTS, X-Frame-Options), token-bucket rate limiting, timing attack mitigation (`crypto/subtle.ConstantTimeCompare`), bcrypt password hashing.
6. **Multi-Language (i18n):**
   * English (`en`) and Vietnamese (`vi`) out of the box, easily extensible via `ui/src/locales/*.json`.
7. **Live Dashboard via Server-Sent Events (SSE):**
   * Real-time updates pushed from backend broker (`/api/realtime/stream`) without page refresh or polling.

---

## 2. DATABASE SCHEMA (18 TABLES)

Stored in single file `swanpanel.db` (SQLite WAL mode, `PRAGMA foreign_keys = ON`):
1. `bootstrap_states`: One-time installer setup token SHA-256 digest.
2. `oidc_providers`: Multi-IdP configuration (Google, Microsoft Entra ID, Keycloak, custom OIDC).
3. `oidc_login_attempts`: Short-lived single-use OIDC PKCE/state/nonce records.
4. `oidc_login_tickets`: Short-lived single-use post-callback session exchange tickets.
5. `mfa_challenges`: Short-lived single-use TOTP/recovery login challenges with attempt limits.
6. `users`: Identity directory (OIDC SSO + Local Admin/User accounts with bcrypt hashes and encrypted TOTP MFA state).
6. `user_groups`: Role & user grouping (DevOps, Backend, Sales).
7. `user_group_members`: Many-to-many join table mapping users to user groups.
8. `resource_groups`: Subnet CIDR allocation (Production VPC, Engineering Subnet).
9. `devices`: Company Nodes (headless servers) and User Devices with static or dynamic Virtual IP.
10. `certificate_revocations`: Persistent X.509 serial revocation ledger retained across device deletions for CRL generation.
11. `access_rules`: Zero-Trust policies (`ALLOW` group/direct policies, `DENY` exclusions).
12. `dns_settings`: MagicDNS suffix, enablement flag, and upstream DNS forwarders.
13. `dns_overrides`: Internal custom DNS override records (`A`, `CNAME`, etc.).
14. `strongswan_settings`: Data plane configuration (Re-auth lifetime, MOBIKE, NAT-T encap, DPD, Ciphers, TCP MSS clamping, advanced VICI options).
15. `runtime_apply_states`: Durable desired vs. applied gateway configuration generation and daemon boot identity tracking.
16. `audit_logs`: Append-only, SHA-256 hash-chained administrative and security audit trail protected by SQLite `BEFORE UPDATE` / `BEFORE DELETE` immutability triggers.
17. `alert_webhooks`: Outbound alert webhook destinations (Slack, Telegram, Microsoft Teams, or HMAC-signed generic HTTP) with delivery telemetry and SSRF validation.
