# REST API Reference

SwanPanel exposes a clean, authenticated RESTful JSON API. All authenticated routes require the header:

```http
Authorization: Bearer <JWT_TOKEN>
```

---

## 1. Authentication

### `POST /api/auth/login`
Authenticates an administrator or local user.
- **Request Body:**
  ```json
  {
    "email": "admin@swanpanel.internal",
    "password": "Admin@SwanPanel123"
  }
  ```
- **Response (200 OK):**
  ```json
  {
    "token": "eyJhbGciOiJIUzI1NiIsIn...",
    "user": {
      "id": "usr_root_admin",
      "email": "admin@swanpanel.internal",
      "is_admin": true
    }
  }
  ```

### `GET /api/auth/me`
Returns details of the currently authenticated user session.

---

## 2. Devices & Provisioning

### `GET /api/devices`
Lists all registered devices.

### `POST /api/devices`
Registers a new device and issues client certificates.
- **Request Body:**
  ```json
  {
    "hostname": "work-macbook",
    "device_type": "user",
    "resource_group_id": "rg_default",
    "platform": "apple"
  }
  ```

### `GET /api/devices/:id/config/:platform`
Downloads native configuration profile for the device.
- `:platform`: `apple`, `windows`, `linux`; use `android-native` for the Android setup JSON (encrypted `.p12`, matching import password, CA and server/client identities). Treat the response as sensitive and do not log it.

### `POST /api/devices/:id/revoke`
Revokes device certificate and terminates active tunnels.

---

## 3. Real-Time Telemetry

### `GET /api/realtime/stream`
Server-Sent Events (SSE) stream streaming live events:
- `session_update`: Active tunnel counts, connection and disconnection events.
- `gateway_status`: strongSwan daemon memory usage and SA states.
- `log_event`: Structured audit log stream.

---

## 4. System & strongSwan Control

### `GET /api/system/daemon/status`
Queries strongSwan `charon` daemon status via VICI IPC.

### `POST /api/system/daemon`
Controls the strongSwan daemon with `{"action":"restart"}` (or `start` / `stop`). This is a daemon lifecycle operation; existing tunnels may disconnect.

### `GET /api/system/apply-status` (Administrator)
Returns `status`, `desired_generation`, `applied_generation`, `last_error` and `last_applied_at`. Saved settings are not necessarily active until the generations agree and status is `applied`.

### `POST /api/system/apply` (Administrator)
Retries synchronization of the saved gateway configuration. No request body is required.

### `GET /api/access-rules/effective/:id?destination=IPv4` (Administrator)
Returns compiled ALLOW/DENY policy, runtime apply state and, when a destination IPv4 is provided, a predicted decision with rule provenance. This is a policy preview, **not** a connectivity test.

Example with an existing administrator token and enrolled device ID:

```bash
export SWANPANEL_URL='https://vpn.example.com'
export SWANPANEL_TOKEN='<YOUR_SESSION_TOKEN>'
export SWANPANEL_DEVICE_ID='<DEVICE_ID>'
curl -fsS -H "Authorization: Bearer $SWANPANEL_TOKEN" \
  "$SWANPANEL_URL/api/access-rules/effective/$SWANPANEL_DEVICE_ID?destination=10.99.1.2"
```

### `GET /api/settings/strongswan` / `PUT /api/settings/strongswan` (Administrator)
Reads or saves gateway settings. After saving, check `/api/system/apply-status` for active enforcement.
