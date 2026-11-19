# Changelog

## v1.0.1 — Unreleased (UI/UX polish)

- **More comfortable layout:** Larger readable body text, 38px form controls, better-spaced tables, a wider content canvas and roomier dialogs without changing the SwanPanel visual language.
- **Clearer daily workflows:** Contextual Add User/Add Group action; visible mobile action legends; consistent table loading, empty and failure states; readable VPN session durations and missing telemetry.
- **Safer configuration:** Early CIDR capacity/overlap guidance, a line-numbered highlighted gateway JSON editor with field validation and change preview, confirmed JSON reset, and clearly separated read-only Root CA identity versus certificate rotation.
- **More reliable SSO setup:** Callback URL is supplied by the gateway's OIDC configuration rather than guessed from the browser origin; local-preview, HTTPS and missing-public-URL warnings are explicit.
- **Translated UI copy:** Additional EN/VI text for tooltips, errors, empty states, status labels and new guidance.

This entry describes implementation, not a published release. Automated frontend checks pass; Go compilation and browser/real-device acceptance must pass on the release source before tagging or publishing `v1.0.1`.

## v1.0.0 — 2026-11-19

SwanPanel's first stable release makes secure remote access easier to deploy and manage on your own infrastructure.

- **Native VPN setup:** Create and revoke device credentials without installing a SwanPanel client. Guided Android IKEv2 setup keeps import passwords hidden by default and reminds users to remove temporary certificate files.
- **Clear access decisions:** Preview ALLOW/DENY for a destination IP, including the matching network and rule. A preview does not guarantee live connectivity.
- **Reliable gateway changes:** Save settings, see whether the gateway has applied them, and retry pending updates when strongSwan is available again.
- **Easier daily administration:** EN/VI interface, accessible confirmations, clearer DNS validation, live dashboard and log updates, and improved light-theme contrast.
- **Self-hosted delivery:** Linux AMD64/ARM64 release binaries with embedded web UI, SQLite, internal PKI, Split-DNS and strongSwan VICI integration. The default Docker Compose deployment bundles strongSwan in one service.

Automated Go, UI, translation and release configuration checks accompany the release. Browser screenshots cover EN/VI in Light/Dark themes; Android native connectivity and gateway apply/retry were verified on physical hardware.

See [README.md](README.md), [README.vi.md](README.vi.md), and [deployment and build checks](docs/RELEASE_BUILD_TESTING.md).
