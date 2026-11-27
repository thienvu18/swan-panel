# Architecture & Internals

SwanPanel integrates five primary subsystem modules into a single self-contained Go binary:

```text
+------------------------------------------------------------------------+
|                          SwanPanel Single Binary                       |
|                                                                        |
|  +--------------------+  +--------------------+  +------------------+  |
|  |   React 19 SPA     |  |   Gin REST API     |  |  Internal PKI    |  |
|  |   (embed.FS)       |  |   & SSE Broker     |  |  (X.509 & PKCS12)|  |
|  +--------------------+  +--------------------+  +------------------+  |
|            |                       |                      |            |
|  +--------------------+  +--------------------+  +------------------+  |
|  |   SQLite Database  |  |   VICI IPC Driver  |  |  Split-DNS Server|  |
|  |   (Native CGO GORM)   |  |   (govici UNIX)    |  |  (miekg/dns :53) |  |
|  +--------------------+  +--------------------+  +------------------+  |
+------------------------------------|-----------------------------------+
                                     | UNIX Socket
                                     v
                       +---------------------------+
                       |   strongSwan charon       |
                       |   IPsec Linux Kernel SA   |
                       +---------------------------+
```

---

## 1. Zero-SSH VICI Control Plane
- SwanPanel communicates directly with strongSwan's `charon` daemon via UNIX domain socket `/var/run/charon.vici` using the binary `govici` protocol.
- SwanPanel persists managed connection configuration and reconciles it through VICI (`load-conn`); it queries associations with `list-sas` and can terminate sessions. Ordinary policy updates aim to preserve existing tunnels, while a daemon restart disconnects them. Saved and active gateway generations are tracked separately.

## 2. Embedded PKI (Public Key Infrastructure)
- SwanPanel acts as an autonomous internal X.509 Certificate Authority.
- On initialization, it generates a 4096-bit RSA Root CA certificate (`SwanPanel Internal Root CA`).
- When a new device is registered, SwanPanel signs a client certificate, packs the client key, certificate, and Root CA chain into a PKCS#12 (`.p12`) archive, and embeds it into native operating system profiles.

## 3. High-Performance Split-DNS & MagicDNS
- Powered by `github.com/miekg/dns`, listening on UDP/TCP port 53.
- Resolves internal device hostnames dynamically (e.g. `laptop-alice.strongnet -> 10.60.0.10`).
- Supports user-defined Custom Overrides (`api.internal -> 10.60.0.50`).
- Non-internal queries forward recursively to upstream public or corporate resolvers (Cloudflare, Google, internal bind9).

## 4. Dynamic Policy Compiler
- Evaluates relationships between User Groups, Users, Devices, and Resource Groups.
- Computes granular CIDR routing policies and exclusion rules (`DROP` filters) for fine-grained zero-trust segmentation.

## 5. Disaster Recovery, Audit Trail & Local MFA Hardening
- **Crash-Safe Encrypted Backups:** Online SQLite snapshots, Root CA/PKI material, and the independent persistent MFA encryption key (`secrets/mfa.key`) are archived using Argon2id + AES-256-GCM. Restores validate archives into staging, enter HTTP `503` maintenance mode to drain active requests, capture rollback snapshots post-drain, activate DB/PKI/MFA exclusively on startup via `journal.json`, rotate `session_epoch`, and expose correlated restore outcomes on `/api/health`.
- **Immutable Audit Trail & Outbox:** `audit_logs` records form an append-only SHA-256 hash chain guarded by SQLite triggers and transactional persistence. Alert webhooks dispatch through the `webhook_deliveries` SQLite outbox with strict public HTTPS SSRF checks and explicit `WEBHOOK_ALLOWED_DESTINATIONS` allowlisting.
- **Local Account MFA / TOTP:** RFC 6238 TOTP secrets are encrypted at rest with AES-256-GCM using an independent `0600` keystore (`MFA_KEY_FILE`) rather than deriving storage keys from `JWT_SECRET`.
