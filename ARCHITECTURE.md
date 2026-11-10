# ARCHITECTURE.md — System Architecture & Technical Specifications

**Product:** SwanPanel (`swan-panel`)  
**Version:** 1.0  
**Scope:** Zero-Trust IKEv2 / strongSwan Modern Control Plane (Enterprise Grade)  
**Standard:** Single-Binary Go Backend, Embedded React 19 SPA, 100% Native VPN Clients  

---

## 1. CORE ARCHITECTURAL PRINCIPLES

1. **100% Native VPN Client Support:**
   * No third-party client apps required. Native Apple `.mobileconfig` (macOS/iOS with embedded PKCS#12 cert), Windows PowerShell (`Add-VpnConnection`), Android IKEv2, Linux NetworkManager.
2. **Single-Binary Zero-Dependency Deployment:**
   * Single compiled executable containing Go backend, SQLite DB (WAL mode), embedded DNS server, internal X.509 PKI, and embedded React SPA (`//go:embed`).
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

## 2. DATABASE SCHEMA (8 TABLES)

Stored in single file `swanpanel.db` (SQLite WAL mode):
1. `oidc_providers`: Multi-IdP configuration (Google, Microsoft Entra, Keycloak).
2. `users`: Identity directory (OIDC SSO + Local Admin/User accounts with bcrypt hashes).
3. `user_groups`: Role & user grouping (DevOps, Backend, Sales).
4. `user_group_members`: Many-to-many relationship mapping users to user groups.
5. `resource_groups`: Subnet CIDR allocation (Production VPC, Engineering Subnet).
6. `devices`: Company Nodes (headless servers) and User Devices with static or dynamic IP.
7. `access_rules`: Zero-Trust policies (`ALLOW` group policies, `DENY` user exclusions).
8. `dns_settings` & `dns_overrides`: MagicDNS `.strongnet`, upstream resolvers, internal overrides.
9. `strongswan_settings`: Data plane configuration (Re-auth lifetime, MOBIKE, NAT-T encap, DPD, Ciphers, TCP MSS clamping).
