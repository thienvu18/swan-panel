# REST API Reference

SwanPanel exposes a clean, authenticated RESTful JSON API. All authenticated routes require the header:

```http
Authorization: Bearer <JWT_TOKEN>
```

> Query-string bearer tokens (such as `?token=...` on REST or SSE endpoints) are prohibited.

---

## 1. Health & Initial Setup

### `GET /api/health` (Public)
Liveness probe for the web server only (`{"status":"healthy","service":"SwanPanel"}`). It does **not** prove VPN readiness.

### `GET /api/auth/setup/status` (Public)
Reports whether initial one-time administrator setup is still required (`{"setup_required": true|false}`).

### `POST /api/auth/setup` (Public, Setup-Only)
Consumes the one-time setup token from `DB_PATH + ".setup-token"` and creates the first `ACTIVE` local administrator.

---

## 2. Authentication & Session

### `POST /api/auth/login`
Authenticates an administrator or local user. Rate-limited.
- **Request Body:**
  ```json
  {
    "email": "<YOUR_ADMIN_EMAIL>",
    "password": "<YOUR_PASSWORD>"
  }
  ```
- **Response (200 OK):**
  ```json
  {
    "token": "<JWT_TOKEN>",
    "user": {
      "id": "usr_root_admin",
      "email": "admin@swanpanel.internal",
      "name": "Administrator",
      "is_admin": true,
      "status": "ACTIVE"
    }
  }
  ```

### `GET /api/auth/me` (Authenticated)
Returns details of the currently authenticated user session.

### `PUT /api/auth/password` (Authenticated, Local Accounts)
Updates the current user's password after verifying `current_password`. New passwords must be 12–72 bytes.

### Local Two-Factor Authentication (TOTP MFA)
- `POST /api/auth/mfa/verify` (Public, Rate-Limited): Completes a two-step login by exchanging `mfa_token` + TOTP `code` (or single-use recovery code) for an application JWT.
- `GET /api/auth/mfa/status` (Authenticated): Returns `supported`, `mfa_enabled`, and `backup_codes_remaining`.
- `POST /api/auth/mfa/setup` (Authenticated, Local Accounts): Verifies `current_password` and generates an encrypted pending TOTP secret + `otpauth://` URI.
- `POST /api/auth/mfa/enable` (Authenticated, Local Accounts): Confirms setup with a 6-digit TOTP `code` and returns 8 single-use recovery codes once.
- `POST /api/auth/mfa/disable` (Authenticated, Local Accounts): Disables TOTP after verifying `current_password` and a valid TOTP/recovery `code`.
- `POST /api/auth/mfa/backup-codes` (Authenticated, Local Accounts): Regenerates 8 recovery codes after verifying `current_password` and a live TOTP `code`.
- `POST /api/users/:id/mfa/reset` (Administrator): Clears TOTP enrollment for a locked-out local user.

### OIDC SSO Flow (Public)
- `GET /api/auth/oidc/providers`: Lists active SSO providers (`id`, `name`, `provider_type`).
- `GET /api/auth/oidc/:id/start`: Starts Authorization Code + S256 PKCE login flow.
- `GET /api/auth/oidc/callback`: Verifies state cookie, PKCE, nonce, and ID token; issues a short-lived `swan_oidc_ticket` cookie.
- `POST /api/auth/oidc/exchange`: Exchanges the single-use `swan_oidc_ticket` cookie for an application JWT.

---

## 3. Dashboard, Sessions & Real-Time Telemetry

### `GET /api/dashboard/stats` (Authenticated)
Returns scoped telemetry counts (sessions, devices, users, pending approvals).

### `GET /api/gateway/health` (Authenticated)
Returns the current evidence-based Gateway health snapshot on initial load or after SSE reconnect:

```json
{
  "health": {
    "status": "online",
    "checked_at": "2026-11-20T07:30:00Z"
  }
}
```

The `status` value is `online`, `degraded`, `offline`, or `unknown`. The server verifies health every 15 seconds; snapshots older than 45 seconds expire to `unknown`.

### `GET /api/sessions` (Authenticated)
Lists active IKEv2 security associations (non-administrators see only their own non-revoked devices).

### `POST /api/sessions/terminate` (Administrator)
Terminates a live tunnel via VICI (`{"ike_name": "<connection_name>"}`).

### `GET /api/realtime/stream` (Authenticated)
Server-Sent Events (SSE) stream pushing live updates:
- `session_update`, `session_terminated`, `device_created`, `device_revoked` (sanitized for non-administrators)
- `gateway_status`: Daemon lifecycle notifications.
- `gateway_health`: Verified Gateway state updates every 15 seconds (`online`, `degraded`, `offline`, `unknown`).
- `strongswan_settings_updated`, `approval_queue_update`, `user_approved`, `log_event` (administrators only).

---

## 4. Subnets, Devices & Provisioning

### `GET /api/resource-groups` (Administrator)
Lists IPv4 VPN subnets with CIDR capacity and active node counts.

### `POST /api/resource-groups` / `DELETE /api/resource-groups/:id` (Administrator)
Creates non-overlapping IPv4 subnets (`/8`–`/30`) or deletes unreferenced subnets.

### `GET /api/devices` (Authenticated)
Lists registered devices (non-administrators see only their own devices).

### `POST /api/devices` (Authenticated)
Enrolls a new device and issues a client certificate (non-administrators may only enroll `user` devices for themselves).
- **Request Body:**
  ```json
  {
    "hostname": "work-macbook",
    "device_type": "user",
    "resource_group_id": "rg_default",
    "ip_allocation_type": "dynamic",
    "platform": "macos"
  }
  ```

### `GET /api/devices/:id/config/:platform` (Authenticated)
Downloads native configuration profile for the device (`apple`/`macos`/`ios`, `windows`, `linux`, `android-native`, or legacy `android`). Responses include `Cache-Control: no-store, private`. Treat responses as sensitive and do not log them.

### `POST /api/devices/:id/revoke` (Authenticated)
Revokes the device certificate, publishes the updated CRL, updates nftables, and terminates active tunnels.

### `DELETE /api/devices/:id` (Administrator)
Revokes and soft-deletes a device while retaining its serial in the CRL ledger.

---

## 5. Users, Groups & OIDC Providers (Administrator)

- `GET /api/users`, `POST /api/users`: List directory or create local user (`12–72` byte password).
- `PUT /api/users/:id/role`: Toggle administrator privilege (last active admin protected with HTTP 409).
- `POST /api/users/:id/approve`, `POST /api/users/:id/reject`: Approve pending SSO user (with group assignment) or reject candidate.
- `DELETE /api/users/:id`: Atomically revokes user's devices, clears group memberships, and removes user.
- `GET /api/user-groups`, `POST /api/user-groups`, `DELETE /api/user-groups/:id`: Manage user groups.
- `GET /api/oidc/providers`, `POST /api/oidc/providers`, `PUT /api/oidc/providers/:id`, `DELETE /api/oidc/providers/:id`, `POST /api/oidc/providers/:id/test`: Manage and verify OIDC providers.

---

## 6. Zero-Trust Access Rules (Administrator)

- `GET /api/access-rules`, `POST /api/access-rules`, `DELETE /api/access-rules/:id`: Manage `ALLOW` and `DENY` policies for `user_group`, `user`, or `device` subjects targeting a `resource_group` or IPv4 `cidr_ip`.
- `GET /api/access-rules/effective/:id?destination=IPv4`: Returns compiled ALLOW/DENY policy, runtime apply state, and (when `destination` is given) a predicted verdict with rule provenance. This is a policy preview, **not** a live connectivity test.

```bash
export SWANPANEL_URL='https://vpn.example.com'
export SWANPANEL_TOKEN='<YOUR_SESSION_TOKEN>'
export SWANPANEL_DEVICE_ID='<DEVICE_ID>'
curl -fsS -H "Authorization: Bearer $SWANPANEL_TOKEN" \
  "$SWANPANEL_URL/api/access-rules/effective/$SWANPANEL_DEVICE_ID?destination=10.99.1.2"
```

---

## 7. Split-DNS & Gateway Control

### Split-DNS (Administrator)
- `GET /api/settings/dns`, `PUT /api/settings/dns`: Read or update MagicDNS suffix, enablement, and upstream resolvers.
- `GET /api/dns/overrides`, `POST /api/dns/overrides`, `DELETE /api/dns/overrides/:id`: Manage custom internal DNS overrides (`A`, `CNAME`, etc.).

### strongSwan, Root CA & Daemon Control
- `GET /api/settings/strongswan`, `PUT /api/settings/strongswan` (Administrator): Read or update gateway settings. Check `/api/system/apply-status` for active enforcement.
- `GET /api/settings/ca`, `POST /api/settings/ca/rotate` (Administrator): Inspect Root CA identity or rotate before any device/revocation exists (requires `confirm: "ROTATE CA"`).
- `GET /api/system/daemon/status` (Authenticated): Queries strongSwan `charon` daemon status via VICI IPC.
- `POST /api/system/daemon` (Administrator): Controls the strongSwan daemon (`start`, `stop`, `restart`; unavailable when running against external host strongSwan).
- `GET /api/system/apply-status`, `POST /api/system/apply` (Administrator): Inspect saved vs. active generation state or retry pending synchronization.
- `GET /api/system/logs` (Administrator): Filter recent in-memory structured application logs (`scope`, `level`, `search`).
- `POST /api/system/backup` (Administrator): Streams a v2 AES-256-GCM encrypted `.tar.enc` snapshot (`64 MiB` encrypted limit, `256 MiB` uncompressed, `4,096` entries) of the SQLite database, PKI directory, and independent MFA keystore derived from the supplied passphrase (12–128 bytes) with `Cache-Control: no-store, private`.
- `POST /api/system/restore` (Administrator): Verifies the `.tar.enc` archive (`backup_file`, `passphrase`, `confirm: "RESTORE BACKUP"`), stages rollback snapshots, persists a restore journal, enters HTTP `503` maintenance mode, returns HTTP `202` (`runtime_status: "restarting"`), and triggers controlled control-plane restart so exclusive startup recovery activates DB/PKI/MFA state, rotates session epochs, and reconciles gateway policies/CRL.

---

## 8. Immutable Audit Trail & Alert Webhooks (Administrator)

- `GET /api/audit-logs`: Paginated immutable audit log query (`limit`, `offset`, `severity`, `action`, `target_type`, `actor`, `search`, `from`, `to`).
- `GET /api/audit-logs/verify`: Verifies sequential ordering and SHA-256 hash-chain linkage across all stored audit entries.
- `GET /api/audit-logs/export?format=csv|jsonl`: Exports filtered audit entries as CSV (formula-injection safe) or JSON Lines (`Cache-Control: no-store, private`).
- `GET /api/webhooks`, `POST /api/webhooks`, `PUT /api/webhooks/:id`, `DELETE /api/webhooks/:id`: Manage alert destinations (`slack`, `telegram`, `teams`, `generic`) with fail-safe public HTTPS SSRF validation (or explicit `WEBHOOK_ALLOWED_DESTINATIONS`), redacted URL/secret output, and persistent outbox counters (`pending_count`, `retrying_count`, `failed_count`).
- `GET /api/webhooks/deliveries`: Paginated inspection of persistent alert outbox delivery attempts and sanitized failure summaries (`webhook_id`, `status`, `limit`).
- `POST /api/webhooks/:id/test`: Dispatches a synthetic test alert and reports sanitized HTTP delivery status.
