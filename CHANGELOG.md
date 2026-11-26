# Changelog

## Unreleased

- **1-Click Encrypted Backup & Disaster Recovery:** Export AES-256-GCM encrypted `.tar.enc` archives (Argon2id key derivation) bundling an online SQLite snapshot and PKI state via `POST /api/system/backup`, with verified atomic restore, rollback safety, gateway resynchronization, and Server Settings UI (`POST /api/system/restore`).
- **Two-Factor Authentication (TOTP MFA):** RFC 6238 TOTP support for local accounts with AES-256-GCM encrypted secrets, single-use replay protection, 8 recovery codes, self-service Account Security settings, and admin MFA reset.
- **Immutable Audit Trail:** Append-only, SHA-256 hash-chained `audit_logs` protected by SQLite immutability triggers, with integrity verification, severity/category/search filters, CSV (formula-neutralized) and JSONL export, and brute-force sign-in escalation.
- **Alert Webhooks:** Configurable alert destinations for Slack, Telegram, Microsoft Teams, and HMAC-SHA256 signed generic HTTP endpoints with SSRF validation, redacted URL/secret display, and test delivery.
- **Documentation Alignment:** Updated `ARCHITECTURE.md`, `wiki/API-Reference.md`, `wiki/Client-Provisioning.md`, `wiki/DNS-and-MagicDNS.md`, and admin/user guides to match the current schema, routes, and `v1.0.1` baseline.

## v1.0.1

- **Easier to read:** Clearer forms, tables, dialogs and page layouts, including on smaller screens.
- **Clearer daily workflows:** Contextual Add User/Add Group action; visible mobile action legends; consistent table loading, empty and failure states; readable VPN session durations and missing telemetry.
- **Safer configuration:** Early CIDR capacity/overlap guidance, a line-numbered highlighted gateway JSON editor with field validation and change preview, confirmed JSON reset, and clearly separated read-only Root CA identity versus certificate rotation.
- **Safer SSO setup:** Use the correct sign-in callback address with clear warnings when public HTTPS settings need attention.
- **Trustworthy Gateway status:** Verified updates every 15 seconds, automatic expiry of outdated information, and clear Online, Degraded, Offline or Unknown labels. A restarted Gateway is no longer shown as healthy until its configuration is confirmed.
- **Gateway visibility on mobile:** Gateway status moves into the mobile header, with tap-to-view last-check details and no duplicate status in the navigation drawer.
- **Focused layout improvements:** Cleaner connection and DNS settings with expandable advanced options, readable guidance and table spacing without oversized badges.
- **Clearer EN/VI guidance:** Consistent status labels, helpful messages and action descriptions across both languages.

## v1.0.0 — 2026-11-19

SwanPanel's first stable release makes secure remote access easier to deploy and manage on your own infrastructure.

- **Native VPN setup:** Create and revoke device credentials without installing a SwanPanel client. Guided Android IKEv2 setup keeps import passwords hidden by default and reminds users to remove temporary certificate files.
- **Clear access decisions:** Preview ALLOW/DENY for a destination IP, including the matching network and rule. A preview does not guarantee live connectivity.
- **Reliable gateway changes:** Save settings, see whether the gateway has applied them, and retry pending updates when strongSwan is available again.
- **Easier daily administration:** EN/VI interface, accessible confirmations, clearer DNS validation, live dashboard and log updates, and improved light-theme contrast.
- **Self-hosted delivery:** Linux AMD64/ARM64 release binaries with embedded web UI, SQLite, internal PKI, Split-DNS and strongSwan VICI integration. The default Docker Compose deployment bundles strongSwan in one service.

See [README.md](README.md), [README.vi.md](README.vi.md), and [deployment and build checks](docs/RELEASE_BUILD_TESTING.md).
