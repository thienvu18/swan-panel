# Changelog

## v1.1.0 — 2026-11-27

SwanPanel `v1.1.0` adds encrypted disaster recovery backups, tamper-evident audit logging with a persistent alert outbox, and two-factor authentication for local accounts:

- **1-Click Encrypted Backup & Disaster Recovery:** Export v2 AES-256-GCM encrypted `.tar.enc` archives (Argon2id key derivation; enforced limits of `64 MiB` encrypted upload, `256 MiB` uncompressed archive, `200 MiB` per entry, `4,096` entries) bundling an online SQLite snapshot, PKI state, and the independent persistent MFA encryption key (`secrets/mfa.key`) via `POST /api/system/backup`. Restores stage and verify all payloads before entering maintenance mode (HTTP 503), persisting a durable restore journal, triggering a controlled control-plane restart (`POST /api/system/restore` HTTP `202` `restarting` with correlated `restore_id`), capturing rollback snapshots strictly post-drain, activating DB/PKI/MFA state exclusively on startup with rollback/fail-closed protection, rotating session epochs to invalidate pre-restore JWTs, and resynchronizing strongSwan pools/CRL/nftables.
- **Two-Factor Authentication (TOTP MFA):** RFC 6238 TOTP support for local accounts with AES-256-GCM encrypted secrets protected by an independent `0600` persistent key (`MFA_KEY_FILE`, safely migrated from legacy `JWT_SECRET` on startup without bundling `JWT_SECRET` in backups), single-use step replay protection, 8 recovery codes, self-service Account Security settings, and admin MFA reset.
- **Immutable Audit Trail:** Append-only, SHA-256 hash-chained `audit_logs` protected by SQLite immutability triggers and fail-closed transactional audit persistence across security mutations, with integrity verification, severity/category/search filters, CSV (formula-neutralized) and JSONL export, and brute-force sign-in escalation.
- **Alert Webhooks & Persistent Outbox:** Configurable alert destinations for Slack, Telegram, Microsoft Teams, and HMAC-SHA256 signed generic HTTP endpoints with fail-safe public HTTPS SSRF protection (dial-time IP verification blocking reserved/non-routable/metadata ranges, redirect/proxy blocking, and explicit `WEBHOOK_ALLOWED_DESTINATIONS` private allowlist), sanitized error/redacted URL display, and a persistent SQLite retry outbox (`GET /api/webhooks/deliveries`) with bounded claim batches, exponential backoff retries, and automatic cancellation across backup restores.
- **Documentation & Verification Alignment:** Synchronized `README.md`, `README.vi.md`, `ARCHITECTURE.md`, `deploy/README.md`, `docs/SECURITY_REGRESSION_TEST_PLAN.md`, `.env.example`, Compose templates, landing/guide pages, wiki pages, and refreshed UI screenshots for `v1.1.0`.

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
