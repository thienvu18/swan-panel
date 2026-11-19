# Changelog

## v1.0.0 — 2026-11-19

SwanPanel's first stable release makes secure remote access easier to deploy and manage on your own infrastructure.

- **Native VPN setup:** Create and revoke device credentials without installing a SwanPanel client. Guided Android IKEv2 setup keeps import passwords hidden by default and reminds users to remove temporary certificate files.
- **Clear access decisions:** Preview ALLOW/DENY for a destination IP, including the matching network and rule. A preview does not guarantee live connectivity.
- **Reliable gateway changes:** Save settings, see whether the gateway has applied them, and retry pending updates when strongSwan is available again.
- **Easier daily administration:** EN/VI interface, accessible confirmations, clearer DNS validation, live dashboard and log updates, and improved light-theme contrast.
- **Self-hosted delivery:** Linux AMD64/ARM64 release binaries with embedded web UI, SQLite, internal PKI, Split-DNS and strongSwan VICI integration. The default Docker Compose deployment bundles strongSwan in one service.

Automated Go, UI, translation and release configuration checks accompany the release. Browser screenshots cover EN/VI in Light/Dark themes; Android native connectivity and gateway apply/retry were verified on physical hardware.

See [README.md](README.md), [README.vi.md](README.vi.md), and [deployment and build checks](docs/RELEASE_BUILD_TESTING.md).
