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
- `:platform`: `apple`, `windows`, `linux`, `android`.

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

### `POST /api/system/daemon/restart`
Initiates controlled reload of strongSwan connection parameters.
