# SwanPanel — Administrator User Guide (Hướng Dẫn Quản Trị Viên)

> **Document Version:** 3.0.0  
> **Target Audience:** Network Engineers, DevOps / SRE Administrators, Security Operations Center (SOC)  
> **Platform Version:** SwanPanel v1.0 Enterprise  
> **Security Baseline:** NIST SP 800-207 Zero-Trust Architecture, FIPS 140-3 Cryptography, RFC 7296 (IKEv2), RFC 4555 (MOBIKE)

---

## 1. ARCHITECTURAL OVERVIEW & ZERO-TRUST MISSION

**SwanPanel** is a single-binary, zero-trust web control plane engineered specifically for high-throughput **strongSwan** Remote Access VPN gateways.

### 1.1. Core Architectural Pillars
* **Single-Binary Delivery:** The control plane encapsulates the Go backend daemon, embedded SQLite database (`gorm`), embedded React 19 SPA (`embed.FS`), internal X.509 PKI certificate authority, and an embedded UDP/TCP DNS server (`miekg/dns`) into a single self-contained executable.
* **Zero-SSH Operations:** Administrators manage the entire gateway lifecycle dynamically. Connection configurations, IP pools, cryptographic proposals, and access rules are loaded directly into strongSwan daemon RAM via the official UNIX socket protocol (`/var/run/charon.vici`) without opening SSH terminals or triggering service restarts.
* **100% Native Client Support:** SwanPanel does not require end users to install third-party, closed-source VPN client applications. It natively generates standard Apple `.mobileconfig` profiles, Windows PowerShell `Add-VpnConnection` scripts, and Linux `swanctl` / NetworkManager configurations.
* **Microsegmentation by Default:** Traffic between connected nodes and enterprise subnets is subject to strict Least Privilege evaluation. All traffic outside explicitly defined `ALLOW` policies is rejected.

```mermaid
flowchart TD
    subgraph Clients["Native Endpoints"]
        macOS["Apple macOS / iOS (.mobileconfig)"]
        Windows["Windows 11 / 10 (PowerShell)"]
        Linux["Linux (swanctl / NetworkManager)"]
        Android["Android (IKEv2 / strongSwan)"]
    end

    subgraph SwanPanelGateway["SwanPanel Gateway (Port 8080 / 500 / 4500 / 53)"]
        WebControlPlane["Web Control Plane (React 19 + Gin)"]
        PKIEngine["Internal PKI Engine (RSA 2048 / 4096 X.509)"]
        DNSEngine["MagicDNS Split-DNS Server (Port 53 UDP/TCP)"]
        VICIClient["strongSwan VICI IPC Client (Unix Socket)"]
        Daemon["strongSwan charon daemon (IPsec ESP / IKEv2)"]
    end

    subgraph Infrastructure["Corporate Protected Infrastructure"]
        VPC1["Production VPC (10.100.0.0/16)"]
        VPC2["Core Infrastructure DMZ (10.10.0.0/20)"]
        VPC3["Database Subnet (10.50.10.0/24)"]
    end

    Clients -- "IKEv2 / IPsec UDP 500, 4500" --> Daemon
    WebControlPlane -- "UNIX Socket /var/run/charon.vici" --> Daemon
    WebControlPlane -- "Issues X.509 Certificates" --> PKIEngine
    Daemon -- "Tunnel Forwarding & Policy Enforcement" --> Infrastructure
    Clients -- "Split-DNS Queries (UDP 53)" --> DNSEngine
```

---

## 2. GETTING STARTED & SYSTEM INITIALIZATION

### 2.1. Accessing the Control Plane
Navigate your browser to the gateway address: `https://<gateway-fqdn>:8080` (or `http://localhost:8080` in local testing).

![SwanPanel Login Screen (Dark Mode)](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/01_login_page.png)

### 2.2. One-Time Administrator Setup
On first startup SwanPanel creates **no default login**. It generates a 256-bit one-time setup token, writes it to `DB_PATH + ".setup-token"` with file mode `0600`, and stores only the token hash in SQLite.

![SwanPanel Initial Administrator Setup with One-Time Token](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/01b_setup_token.png)

Read the token locally on the gateway (for example, `sudo cat /var/lib/swan-panel/swanpanel.db.setup-token`), enter it in the web setup page, and choose the first administrator's name, email and a password of 12–72 bytes. The token is consumed immediately after the administrator is created. Do not paste it into chat, tickets or logs.

OIDC is configured separately after initial setup; SSO candidates must be approved by an administrator.

### 2.3. Language & Theme Customization
* **Internationalization (i18n):** Toggle between **English (EN)** and **Tiếng Việt (VI)** instantly using the globe switcher in the sidebar header.
* **Theme Switching:** Toggle between **Dark Mode** (zinc-950 background `#0b0f17`) and **Light Mode** (slate-50 background `#f8fafc`) via the moon/sun icon.

![SwanPanel Login Screen (Light Mode)](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/01_login_page.png)

---

## 3. REAL-TIME OPERATIONS DASHBOARD

The **Dashboard** serves as the mission-critical cockpit for network operations and security analysts.

![SwanPanel Dashboard (Dark Mode)](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/02_dashboard.png)

### 3.1. Telemetry Cards
1. **Active Sessions:** Number of concurrent established IPsec security associations (SAs).
2. **Total Devices:** Total enrolled endpoints with active X.509 certificates.
3. **Total Users:** Registered directory users across local database and OIDC federations.
4. **Gateway Status:** Verified VPN health: Online (running with settings applied), Degraded (running but requiring attention), Offline (confirmed stopped) or Unknown (insufficient recent evidence). The server checks every 15 seconds; information older than 45 seconds becomes Unknown. On desktop the indicator is in the sidebar footer; on mobile it is in the header. Hover or tap to see the last check time. This status does not prove end-to-end client connectivity.

### 3.2. Active Tunnels Table & Instant Actions
The table displays real-time parameters streamed directly from `charon.vici`:
* **Device Hostname:** Identified endpoint name verified against client certificate Common Name (`CN`).
* **Virtual IP:** Internal leased private IP address with a 1-click **CopyButton**.
* **Remote WAN IP:** Public IP address and NAT traversal UDP port (`172.18.0.3[4500]`).
* **Cipher Suite:** Active cryptographic parameters negotiated in RAM (e.g. `AES_GCM_16-256 / PRF_HMAC_SHA2_384 / ECP_384`).
* **Instant Disconnect (Kill-Switch):** Administrators can immediately terminate any suspicious tunnel by clicking the red trash button, sending a VICI `terminate` command without disturbing other tunnels.

### 3.3. Dual View Mode (Admin vs. User View)
Administrators can toggle between **Admin View** and **User View** via the view selector control at the top right of the dashboard:
* **Admin View:** Displays global telemetry cards, active tunnel metrics, and system-wide IPsec security associations.
* **User View:** Provides a self-service client dashboard displaying **My Personal Devices**, allowing authenticated users to view enrolled endpoints, connection status, IP addresses, and directly download native configuration profiles (Apple `.mobileconfig`, Windows PowerShell, Linux NetworkManager, Android).

![SwanPanel Dashboard User View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/02b_dashboard_user_view.png)

---

## 4. IP SUBNETS & SUBNET CAPACITY METER

SwanPanel organizes your IP address space into dedicated **Resource Groups (Subnets)**.

![Resources - Subnets View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/03_resources_subnets.png)

### 4.1. Visual Subnet Capacity Meter
To prevent IP pool exhaustion, SwanPanel features an automatic, visual capacity meter:
* **Emerald Progress Bar (< 70%):** Healthy allocation pool with ample headroom.
* **Amber Progress Bar (70% - 90%):** Elevated usage; plan expansion or reclaim inactive nodes.
* **Rose Progress Bar (> 90%):** Critical threshold; immediate CIDR block expansion recommended.

### 4.2. Creating a Subnet
1. Navigate to **Resources** > click **Resource Groups** subnav tab.
2. Click **+ Add Subnet** in the top action toolbar.
3. Fill in the required parameters:
   * **Subnet Name:** Descriptive identifier (e.g. `SecOps & Threat Intel Enclave`).
   * **CIDR Block:** Standard IPv4 CIDR notation (e.g. `10.150.0.0/24`). The backend validates syntax and rejects overlapping pools.
   * **Description:** Purpose and access classification of the network segment.
4. Click **Create Subnet**. The subnet pool is immediately active in SQLite and strongSwan daemon routing tables.

![Modal Add Subnet](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/04_modal_add_subnet.png)

---

## 5. DEVICE PROVISIONING & NATIVE CONFIGURATION PROFILES

SwanPanel treats devices as first-class cryptographic identities.

![Resources - Devices Table](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/05_resources_devices.png)

### 5.1. Provisioning a Device
1. Under **Resources** > **Devices**, click **+ Add Device**.
2. Complete the enrollment form:
   * **Hostname:** Unique node hostname (e.g. `secops-jumpbox-01`).
   * **Ownership Type:** Choose between **Company Resource** (servers, gateways, bastions) or **User Device** (employee laptops, mobile devices).
   * **Subnet:** Select the destination Resource Group from the dropdown.
   * **IP Allocation:**
     * **Dynamic:** Leased automatically on connect via RADIUS/IKEv2 configuration attributes.
     * **Static:** Reserve a fixed VIP within the assigned subnet (e.g. `10.150.0.10`).
   * **Platform:** Select destination OS (`macOS`, `Windows`, `Linux`, `iOS`, `Android`).
3. Click **Create Device**. SwanPanel's internal PKI generates a 2048-bit RSA key pair, signs the X.509 client certificate, computes its SHA-256 fingerprint, and stores the record securely. If gateway reconciliation is **pending**, verify the apply status before using the new profile.

![Modal Add Device](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/06_modal_add_device.png)

### 5.2. Faceted Filter Chips & Instant Search
* **Faceted OS Filter:** Filter by `macOS`, `Windows`, `Linux`, `iOS`, `Android`, or view all nodes with real-time count badges.
* **Ownership Filter:** Filter between `Company Resource` and `User Device`.
* **Search-as-you-type:** Instantly filter devices by hostname, assigned virtual IP, or owner email.
* **1-Click Copy:** Click the copy icon next to any hostname or virtual IP to copy it cleanly to your clipboard.

### 5.3. Certificate Revocation (Self-Revoke)
If an endpoint is lost, stolen, or decommissioned:
1. Locate the device row in the table.
2. Click the red **ShieldAlert** icon in the actions column.
3. Confirm the prompt in the **ConfirmDialog**: *"Revoke security certificate for <hostname>?"*.
4. SwanPanel sets `is_revoked = true`, adds the serial to the revocation index, and immediately terminates any active tunnels associated with that certificate.

![SwanPanel Destructive Action Confirmation Dialog](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/20_confirm_dialog.png)

### 5.4. Android Native Setup (App-Free IKEv2 / IPsec RSA)
For Android endpoints, SwanPanel eliminates the need for third-party APKs or client apps:
1. Under **Resources** > **Devices**, click the **Set up Android VPN** action on any enrolled Android device.
2. The **Android Native Setup Dialog** presents four numbered steps:
   * **Import client certificate:** Download the PKCS#12 `.p12` bundle, import via Android's VPN/app certificate settings, and use the copyable import password (masked until explicitly revealed).
   * **Create native VPN:** Under Android VPN settings, select `IKEv2/IPsec RSA` and enter the displayed server address, server certificate identity and client identity.
   * **Import CA if requested:** Some Android versions require a separately installed root CA `.cer`; the client bundle already contains the CA chain.
   * **Connect and clean up:** Save the VPN entry, connect, then permanently delete the downloaded `.p12` from Downloads/shared storage.
3. The dialog warns before download: a `.p12` contains a private key. Access credentials only via HTTPS or a trusted private network; never share or screenshot revealed secrets. OEM menus and native VPN support vary by device. No ADB, APK sideloading or third-party VPN app is required.

![SwanPanel Android Native Setup Dialog](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/19_modal_android_setup.png)

---

## 6. ZERO-TRUST ACCESS RULES & MICROSEGMENTATION

SwanPanel implements a **Default-Deny** microsegmentation engine.

![Access Rules Page](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/11_access_rules.png)

### 6.1. Policy Evaluation Hierarchy
1. **Explicit DENY:** Any matching `DENY` policy immediately drops the packet and logs a security event.
2. **Explicit ALLOW:** Packets matching an active `ALLOW` policy are permitted to transit the tunnel.
3. **Implicit Drop:** Packets matching no rules are rejected at the kernel firewall boundary.

### 6.2. Creating an Access Rule
1. Navigate to **Access Rules** in the sidebar.
2. Click **+ Add Rule**.
3. Specify:
   * **Subject Type:** Choose `user_group` or `user`.
   * **Subject Name:** Select or enter the user group (e.g. `Site Reliability Engineers`).
   * **Policy Action:** Toggle between `ALLOW` (Emerald) or `DENY` (Rose).
   * **Target Type:** Choose `resource_group` or specific `cidr_ip`.
   * **Target Value:** Enter the destination subnet or CIDR (e.g. `10.150.0.0/24`).
4. Click **Create Rule**. The rule is saved, and SwanPanel attempts to reconcile it with the VPN data plane. If the API reports `pending`, the gateway has **not** confirmed enforcement yet; inspect **Server Settings > Gateway apply status** before relying on the policy.

### 6.3. Effective Access Preview
The **Effective Access** panel at the top of **Access Rules** allows an administrator to select a non-revoked device, inspect compiled **ALLOW** and **DENY** CIDR selectors, and enter a destination IPv4 address to preview the predicted result. The verdict is shown first with the evaluated destination, **ALLOW/DENY**, matched network, and contributing rule IDs grouped by device/user/group origin. An unmatched destination is **default DENY** and does not claim a matching rule. The precedence is explicit `DENY` before `ALLOW`, followed by default `DENY`.

This is a **configuration-level preview**, not a live network probe. The panel also displays the desired and applied gateway generation state; a pending or failed application means the preview may not match currently enforced policy. Only administrators can access `GET /api/access-rules/effective/:id`.

![Modal Add Access Rule](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/12_modal_add_rule.png)

---

## 7. SPLIT-TUNNEL DNS ENGINE & MAGICDNS

SwanPanel includes an embedded DNS engine designed for split-tunnel remote access.

![DNS Engine Page](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/13_dns_engine.png)

### 7.1. MagicDNS Automatic Host Resolution
Every device provisioned in SwanPanel automatically receives an internal MagicDNS entry:
```text
<hostname>.internal  ->  <virtual_ip>
```
When a connected client queries `node-24116.internal`, the embedded DNS engine intercepts the request and resolves it to `10.0.1.12` in sub-millisecond response time.

### 7.2. Custom DNS Overrides
Administrators can define custom corporate DNS overrides for internal services:
1. Click **+ Add Override** on the DNS Engine page.
2. Configure:
   * **Hostname (FQDN):** Full domain (e.g. `gitlab.internal.acme.net` or `api.internal`).
   * **Record Type:** Choose `A`, `AAAA`, `CNAME`, or `TXT`.
   * **Target IP / Host:** Destination IP address (e.g. `10.100.1.10`).
3. Click **Add Override**.

![Modal Add DNS Override](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/14_modal_add_dns_override.png)

### 7.3. Split-DNS Forwarding
Queries outside configured internal zones (e.g. `github.com`, `google.com`) are forwarded transparently to configured upstream resolvers (`1.1.1.1, 8.8.8.8`).

---

## 8. ENTERPRISE IDENTITY & SINGLE SIGN-ON (OIDC)

Federate user authentication with enterprise Identity Providers (IdPs) via standard OpenID Connect.

![OIDC Providers Page](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/15_oidc_providers.png)

### 8.1. Supported Providers
* **Okta Workforce Identity Cloud**
* **Microsoft Entra ID (Azure AD)**
* **Google Workspace**
* **Keycloak / Authentik / Generic OIDC**

### 8.2. Configuring an OIDC Provider
1. Navigate to **OIDC Providers** > click **+ Add Provider**.
2. Configure:
   * **Provider Name:** Identifier (e.g. `Corporate Okta SSO`).
   * **Provider Type:** Select provider profile.
   * **Issuer URL:** Identity Provider Discovery endpoint (e.g. `https://acme.okta.com/oauth2/default`).
   * **Client ID:** Application ID from your IdP console.
   * **Client Secret:** Application Secret key. Use the eye icon toggle (`Eye`/`EyeOff`) to inspect or mask the secret.
   * **Redirect URI:** Copy the auto-generated callback URI (`https://<gateway>:8080/api/auth/oidc/callback`) using the 1-click **CopyButton** and paste it into your IdP's allowed redirect list.
3. Click **Save Provider**.

![Modal Add OIDC Provider](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/16_modal_add_oidc.png)

---

## 9. USER DIRECTORY & ROLE-BASED ACCESS CONTROL (RBAC)

Manage corporate staff, contractor accounts, and role privileges.

![Users Directory](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/07_users_directory.png)

### 9.1. User Directory & Approvals
* **Local Accounts:** Create direct accounts with email, password (hashed with `bcrypt`), and name.
* **Role Management:** Toggle between standard user and Administrator (`is_admin: true`) with 1 click.
* **Approval Queue:** External OIDC registrations can be staged as `PENDING` until approved by an administrator.

![Modal Add User](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/08_modal_add_user.png)

### 9.2. User Groups
Group users logically (e.g. `DevOps Engineers`, `External Auditors`, `Finance Team`) to apply Access Rules in bulk.

![User Groups Sub-Tab](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/09_user_groups.png)

![Modal Add User Group](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/10_modal_add_group.png)

---

## 10. STRONGSWAN ENGINE & SERVER SETTINGS

Fine-tune kernel IPsec parameters, strongSwan daemon runtime settings, and manage cryptographic authority without editing configuration files on disk.

![Server Settings Page](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/17_server_settings.png)

### 10.1. Gateway Reconciliation Status Banner
At the top of Server Settings, a live status banner continuously monitors policy sync between the control plane and the VPN gateway data plane:
* **Applied (Emerald):** Desired configuration generation matches the applied generation (`desired_generation == applied_generation`). All rules and pools are fully enforced in kernel memory.
* **Pending / Applying (Amber):** Changes have been saved to SQLite and strongSwan configuration fragments, but runtime synchronization through VICI is in progress or pending.
* **Failed (Rose):** Daemon reported an error applying configuration. The banner displays the last error and a **Retry Apply** button to trigger immediate reconciliation.

### 10.2. Cryptographic Proposal Presets
* **Modern (High Security):**
  * IKE: `aes256gcm16-prfsha384-ecp384,chacha20poly1305-prfsha256-curve25519`
  * ESP: `aes256gcm16,chacha20poly1305`
* **Balanced (Recommended):**
  * IKE: `aes256gcm16-prfsha384-ecp384,aes256-sha256-modp2048`
  * ESP: `aes256gcm16,aes128gcm16,chacha20poly1305`
* **Custom:** Freeform string for specialized hardware accelerators or compliance standards.

### 10.3. Network Roaming & Traversal
* **MOBIKE (RFC 4555):** Enabled by default. Clients switching networks (e.g. office Wi-Fi to cellular LTE) seamlessly update their tunnel IP address without dropping active TCP connections or re-authenticating.
* **Force UDP Encapsulation (NAT-T):** Enforces UDP port 4500 encapsulation for traversing strict carrier-grade NATs.
* **MTU MSS Clamping:** Automatically clamped to `1360` bytes to eliminate IP fragmentation across lower MTU carrier networks.

### 10.4. Advanced JSON Gateway Configuration
For complex enterprise environments requiring fine-grained strongSwan parameters beyond standard form controls, switch to the **Advanced JSON** tab:
* **JSON Syntax Validation:** Built-in live validation displays a real-time status badge indicating whether the payload is valid JSON before submission.
* **Direct Parameter Control:** Fine-tune advanced strongSwan daemon parameters, custom rekeying margins, dead-peer-detection (DPD) timing, and specialized plugin options.
* **Editor Controls:** Provides **Format JSON** to re-indent and beautify configurations, along with **Reset** to revert uncommitted edits.

![Advanced JSON Configuration Tab](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/17c_server_settings_advanced.png)

### 10.5. Dynamic Zero-Downtime Reload
When changes are made, click **Save & apply**. SwanPanel renders one managed configuration from the database, writes it atomically to the strongSwan configuration include directory, and attempts to apply the same connections through VICI. The UI shows whether the desired generation was successfully applied or is still pending; a saved configuration does not necessarily mean the gateway is enforcing it yet. It updates its dedicated nftables rules in a single transaction without a routine deny-all interruption. Existing tunnels normally stay established during policy edits; a true daemon restart always terminates existing SAs, after which clients reconnect using the persisted configuration.

### 10.6. Internal Root CA Management & Rotation
Under the **Certificate Authority (CA)** tab, administrators can inspect and manage the embedded X.509 Root CA:
* **Subject Parameters:** Common Name (`CN`), Organization (`O`), Country (`C`), State/Province (`ST`), Locality (`L`).
* **Cryptographic Identity:** Certificate Serial Number, SHA-256 Fingerprint, and Expiration Date (`Not After`).
* **Safe CA Rotation:** Rotating the Root CA creates a new 4096-bit authority and archives existing private key material to `.backup-*`. Because rotation invalidates existing client trust chains until new profiles are distributed, SwanPanel enforces a multi-step safety confirmation requiring the administrator to explicitly type `"ROTATE CA"` before executing.

![Certificate Authority Management Tab](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/17b_server_settings_ca.png)

![CA Rotation Confirmation Dialog](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/21_ca_rotate_confirm.png)

---

## 11. REAL-TIME OBSERVABILITY & SYSTEM LOGS TERMINAL

Monitor low-level daemon events and debug client connectivity in real time.

![System Logs Page](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/18_system_logs.png)

### 11.1. SwanPanel Application Log Stream
The terminal displays SwanPanel's in-memory structured application logs (up to the most recent 1,000 entries), delivered through authenticated HTTP and refreshed on **SSE log notifications**. No browser polling is used. This view is **not** a raw strongSwan/charon journal or a durable audit archive; use the gateway's system journal for full daemon diagnostics.
* **Timestamp & Severity:** `INFO`, `WARN`, `ERROR`.
* **Search & Scope:** Filter by the available device/user tags or search text.
* **Connection State:** The UI distinguishes live SSE connectivity from reconnecting; manually refresh if disconnected.
* **Pause/Clear:** Pause freezes the current view. Clear hides displayed entries but does not erase server logs.

### 11.2. Smart Auto-Scroll Control
* **Auto-Scroll Active:** Terminal automatically scrolls to the bottom as new lines arrive.
* **Auto-Scroll Paused:** When an administrator scrolls up to inspect an error line, auto-scroll pauses and displays an animated floating badge: *"Auto-scroll paused (Click to resume)"*.
* **Resume:** Click the floating pill or scroll back to the bottom to resume live trailing.

### 11.3. Faceted Severity Filters
Filter the live stream by clicking the chips: `ALL`, `INFO`, `WARN`, or `ERROR` to eliminate noise during security audits.

---

## 12. ADMINISTRATIVE TROUBLESHOOTING & FAQ

### 12.1. Gateway Health Check
First check whether the SwanPanel web service is reachable. The unauthenticated endpoint below checks the application, **not** VPN readiness:
```bash
curl -i http://localhost:8080/api/health
# Expected: HTTP 200 {"service":"SwanPanel","status":"healthy"}
```

Sign in to SwanPanel to check the Gateway indicator and its last verified time. If it is **Degraded**, inspect **Server Settings > Gateway apply status** and retry a pending apply when appropriate. If it is **Unknown**, check connectivity and wait for a fresh verification rather than assuming the VPN is stopped. **Offline** means the VPN service has been confirmed stopped. The authenticated `GET /api/gateway/health` endpoint provides the same evidence-based state for diagnostics.

### 12.2. Checking strongSwan Daemon Status via VICI
Verify that the strongSwan daemon is active inside the container or host:
```bash
docker exec swan-panel swanctl --stats
```

### 12.3. Emergency Database Backup
All configuration, certificates, and user records are stored in the single SQLite database:
```bash
# Live backup of SQLite database
sqlite3 /data/swanpanel.db ".backup '/backup/swanpanel-backup-$(date +%F).db'"
```

---

## 13. UI THEME & LANGUAGE MATRIX

SwanPanel provides 100% parity across both languages and color schemes:

| Feature Route | English Light | English Dark | Vietnamese Light | Vietnamese Dark |
| :--- | :---: | :---: | :---: | :---: |
| **Setup Token Screen** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/01b_setup_token.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/01b_setup_token.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/01b_setup_token.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/01b_setup_token.png) |
| **Login Screen** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/01_login_page.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/01_login_page.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/01_login_page.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/01_login_page.png) |
| **Dashboard** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/02_dashboard.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/02_dashboard.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/02_dashboard.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/02_dashboard.png) |
| **Dashboard (User View)** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/02b_dashboard_user_view.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/02b_dashboard_user_view.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/02b_dashboard_user_view.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/02b_dashboard_user_view.png) |
| **Subnets** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/03_resources_subnets.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/03_resources_subnets.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/03_resources_subnets.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/03_resources_subnets.png) |
| **Add Subnet Modal** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/04_modal_add_subnet.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/04_modal_add_subnet.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/04_modal_add_subnet.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/04_modal_add_subnet.png) |
| **Devices** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/05_resources_devices.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/05_resources_devices.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/05_resources_devices.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/05_resources_devices.png) |
| **Add Device Modal** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/06_modal_add_device.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/06_modal_add_device.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/06_modal_add_device.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/06_modal_add_device.png) |
| **Android Native Setup** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/19_modal_android_setup.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/19_modal_android_setup.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/19_modal_android_setup.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/19_modal_android_setup.png) |
| **Confirm Action Dialog** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/20_confirm_dialog.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/20_confirm_dialog.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/20_confirm_dialog.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/20_confirm_dialog.png) |
| **Users Directory** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/07_users_directory.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/07_users_directory.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/07_users_directory.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/07_users_directory.png) |
| **Add User Modal** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/08_modal_add_user.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/08_modal_add_user.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/08_modal_add_user.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/08_modal_add_user.png) |
| **User Groups** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/09_user_groups.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/09_user_groups.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/09_user_groups.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/09_user_groups.png) |
| **Add Group Modal** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/10_modal_add_group.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/10_modal_add_group.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/10_modal_add_group.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/10_modal_add_group.png) |
| **Access Rules & Effective Access** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/11_access_rules.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/11_access_rules.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/11_access_rules.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/11_access_rules.png) |
| **Add Rule Modal** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/12_modal_add_rule.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/12_modal_add_rule.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/12_modal_add_rule.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/12_modal_add_rule.png) |
| **DNS Engine** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/13_dns_engine.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/13_dns_engine.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/13_dns_engine.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/13_dns_engine.png) |
| **Add DNS Override Modal** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/14_modal_add_dns_override.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/14_modal_add_dns_override.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/14_modal_add_dns_override.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/14_modal_add_dns_override.png) |
| **OIDC Providers** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/15_oidc_providers.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/15_oidc_providers.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/15_oidc_providers.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/15_oidc_providers.png) |
| **Add OIDC Provider Modal** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/16_modal_add_oidc.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/16_modal_add_oidc.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/16_modal_add_oidc.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/16_modal_add_oidc.png) |
| **Server Settings** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/17_server_settings.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/17_server_settings.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/17_server_settings.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/17_server_settings.png) |
| **Advanced JSON Settings** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/17c_server_settings_advanced.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/17c_server_settings_advanced.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/17c_server_settings_advanced.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/17c_server_settings_advanced.png) |
| **Certificate Authority (CA)** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/17b_server_settings_ca.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/17b_server_settings_ca.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/17b_server_settings_ca.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/17b_server_settings_ca.png) |
| **CA Rotate Confirmation** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/21_ca_rotate_confirm.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/21_ca_rotate_confirm.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/21_ca_rotate_confirm.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/21_ca_rotate_confirm.png) |
| **System Logs** | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/light/18_system_logs.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/en/dark/18_system_logs.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/light/18_system_logs.png) | [View](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-guide/vi/dark/18_system_logs.png) |

---

## 14. ADVANCED ADMINISTRATIVE WORKFLOWS: INTERACTIVE ACTIONS AUDIT

To verify and document operations as an actual network security administrator, SwanPanel underwent extensive interactive browser execution covering search-as-you-type, faceted filter chips, instant kill-switches, and end-to-end CRUD resource lifecycles.

```text
[ Interactive Evidence Directory ]
└── https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/
    ├── 01_dashboard_live_tunnel_active.png          (Active Session with VIP & Cipher)
    ├── 02_dashboard_after_terminate_empty_state.png  (Kill-Switch Instant Disconnect)
    ├── 03_search_devices_typing_keyword.png         (Real-Time Device Search)
    ├── 04_search_devices_empty_state_with_reset.png (Non-matching Search & Reset)
    ├── 05_filter_devices_by_os_linux.png            (Faceted Filter by OS: Linux)
    ├── 06_filter_devices_by_type_company.png        (Faceted Filter by Type: Company)
    ├── 07_search_subnets_by_cidr.png                (Subnet Search by CIDR: 10.100)
    ├── 08_subnet_created_before_delete.png          (Subnet Enclave Created)
    ├── 09_subnet_deleted_success.png                (Subnet Deleted via Trash Button)
    ├── 10_search_access_rules_by_name.png           (Access Rules Search: database)
    ├── 11_filter_access_rules_allow.png             (Faceted Filter: ALLOW Policies)
    ├── 12_filter_access_rules_deny.png              (Faceted Filter: DENY Policies)
    ├── 13_access_rule_created_before_delete.png     (Access Rule Policy Created)
    ├── 14_delete_access_rule_action.png             (Access Rule Policy Deleted)
    ├── 15_search_dns_by_fqdn.png                    (DNS FQDN Search: vault)
    ├── 16_filter_dns_by_type_a.png                  (Faceted Filter: Record Type A)
    ├── 17_filter_dns_by_type_cname.png              (Faceted Filter: Record Type CNAME)
    ├── 18_dns_override_created_before_delete.png    (DNS Override Created)
    ├── 19_delete_dns_override_action.png            (DNS Override Deleted)
    ├── 20_search_oidc_by_name.png                   (OIDC IdP Search: Microsoft)
    ├── 21_oidc_provider_created_before_delete.png   (OIDC IdP Created)
    ├── 22_delete_oidc_provider_action.png           (OIDC IdP Deleted)
    ├── 23_search_users_by_name.png                  (User Search by Name: Elena)
    ├── 24_users_tab_user_groups.png                 (Switch to User Groups Subtab)
    ├── 25_group_created_before_delete.png           (User Group Created)
    ├── 27_user_created_success.png                  (Directory User Created)
    ├── 29_system_logs_severity_info.png             (Severity Filter: INFO)
    ├── 30_system_logs_severity_warn.png             (Severity Filter: WARN)
    ├── 31_system_logs_severity_error.png            (Severity Filter: ERROR)
    ├── 32_system_logs_scope_filter.png              (Scope Filter Dropdown)
    └── 33_system_logs_stream_paused.png             (Terminal Stream Paused)
```

### 14.1. Dashboard Kill-Switch Disconnect & Session Termination

SwanPanel provides zero-delay tunnel kill-switch capabilities via `/api/sessions/terminate`.

![Active Tunnel on Dashboard](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/01_dashboard_live_tunnel_active.png)

1. **Active Tunnel Row:** Displays verified endpoint identity (`CN`), internal VIP with 1-click **CopyButton**, remote WAN address (`172.18.0.3[4500]`), cryptographic proposal (`AES_CBC-256`), and red trash action button.
2. **Instant Kill-Switch:** Clicking the disconnect button dispatches a VICI IPC terminate command to strongSwan daemon RAM, immediately purging the security association without restarting charon.
3. **EmptyState Transition:** The session table instantaneously refreshes via SSE to display the `<EmptyState>` component with the Activity icon.

![Empty State After Terminate](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/02_dashboard_after_terminate_empty_state.png)

### 14.2. Enrolled Devices: Real-Time Hostname Search & Faceted Filtering

Manage thousands of enterprise endpoints through combined search and multi-dimensional filters.

![Search Devices by Keyword](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/03_search_devices_typing_keyword.png)

* **Search-as-you-type:** Typing `swan-client` filters the device table instantly without full page reloads.
* **Non-Matching Search Recovery:** When a query yields 0 results, an interactive EmptyState appears with a primary **Clear Filters** button that resets all search parameters with 1 click.

![Search Devices Empty State](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/04_search_devices_empty_state_with_reset.png)

* **Faceted OS Filter:** Clicking the `LINUX` chip filters the roster to Linux endpoints with real-time node count badges.
* **Ownership Filter:** Clicking `Company` isolates corporate bastions and managed gateways from BYOD user devices.

![Filter Devices by Linux OS](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/05_filter_devices_by_os_linux.png)

![Filter Devices by Company Ownership](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/06_filter_devices_by_type_company.png)

### 14.3. Resource Groups & Subnets: CIDR Filtering & Subnet Deletion

Audit corporate IP spaces with real-time CIDR search and dynamic lifecycle operations.

![Search Subnets by CIDR](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/07_search_subnets_by_cidr.png)

* **CIDR Prefix Search:** Typing `10.100` isolates the Production VPC subnet and its capacity meter.
* **Subnet Lifecycle (Create & Delete):** Create temporary subnets (e.g. `Temporary Staging Enclave`) and immediately prune them via the red trash button. The capacity bar and routing tables update instantaneously.

![Subnet Created](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/08_subnet_created_before_delete.png)

![Subnet Deleted](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/09_subnet_deleted_success.png)

### 14.4. Zero-Trust Access Rules: Policy Search, ALLOW/DENY Filtering & Deletion

SwanPanel's Zero-Trust engine supports rapid policy verification.

![Search Access Rules](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/10_search_access_rules_by_name.png)

* **Policy Search:** Search by target subnet or rule classification (`database`).
* **Faceted Action Chips:** Switch between `ALLOW` (green shield) and `DENY` (red shield) rules with real-time policy counters.
* **Policy Deletion:** Click the trash action icon on any rule row to immediately revoke network transit permission.

![Filter Access Rules ALLOW](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/11_filter_access_rules_allow.png)

![Filter Access Rules DENY](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/12_filter_access_rules_deny.png)

### 14.5. Split-DNS Engine: FQDN Search, Record Type Filtering & Deletion

Manage custom internal domain overrides and authoritative zone mappings.

![Search DNS by FQDN](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/15_search_dns_by_fqdn.png)

* **FQDN Search:** Real-time lookup of internal hostnames (e.g. `vault`).
* **Record Type Filtering:** Toggle between `A` (IPv4 address) and `CNAME` (canonical alias) records with real-time record count badges.
* **Override Lifecycle:** Add dynamic internal overrides (e.g. `temp-api.internal -> 10.222.1.99`) and delete them with 1 click.

![Filter DNS Record Type A](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/16_filter_dns_by_type_a.png)

![Filter DNS Record Type CNAME](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/17_filter_dns_by_type_cname.png)

![Delete DNS Override Action](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/19_delete_dns_override_action.png)

### 14.6. Enterprise OIDC SSO: Provider Search & Lifecycle Management

Manage external identity provider federations with real-time provider filtering and credentials management.

![Search OIDC Provider](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/20_search_oidc_by_name.png)

* **IdP Search:** Instantly locate identity providers by name (e.g. `Microsoft`).
* **Provider Lifecycle:** Add staging SSO profiles and delete outdated provider integrations.

![Delete OIDC Provider](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/22_delete_oidc_provider_action.png)

### 14.7. Directory & User Groups: User Search & Group Lifecycle

Organize human identities into logical security groups.

![Search Users by Name](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/23_search_users_by_name.png)

* **Directory Search:** Real-time search by user name or email (`Elena`).
* **Subtab Switching:** Instant transition between **Directory & Approvals** and **User Groups**.
* **Group Lifecycle:** Provision and delete custom access groups (e.g. `Temp Security Auditors`).

![User Groups Subtab](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/24_users_tab_user_groups.png)

![User Created](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/27_user_created_success.png)

### 14.8. System Observability: Multi-Tier Severity Filtering & Stream Controls

Eliminate noise and isolate critical security anomalies within the live socket console.

* **Severity Filtering:** Filter logs by `INFO` (blue), `WARN` (amber), or `ERROR` (rose).
* **Target Scope Filter:** Narrow the stream to specific endpoints (`swan-client`) or directory accounts (`Elena Rostova`).
* **Stream Controls:** Use **Pause / Resume** to freeze the live terminal output during forensic analysis.

![Logs Severity INFO](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/29_system_logs_severity_info.png)

![Logs Severity WARN](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/30_system_logs_severity_warn.png)

![Logs Severity ERROR](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/31_system_logs_severity_error.png)

![Logs Scope Filter](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/32_system_logs_scope_filter.png)

![Logs Stream Paused](https://raw.githubusercontent.com/thienvu18/swan-panel/main/docs/assets/images/user-actions-evidence/33_system_logs_stream_paused.png)

---

## Gateway Settings & Advanced Operations

The **Server Settings** page has three tabs:

- **Basic settings:** VPN display name (used in newly generated Apple, Windows and strongSwan Android profiles), endpoint IP/FQDN, server certificate identity, cryptography compatibility preset, MOBIKE and forced NAT-T. Previously installed client profiles are not renamed automatically.
- **Advanced JSON:** edit the complete supported gateway settings object. Standard fields such as `reauth_time`, `rekey_time_ike`, `rekey_time_child`, `inactivity_timeout`, `fragmentation`, `dpd_delay`, `dpd_action`, `ike_proposals`, `esp_proposals` and `mss_clamping` are available here. Additional safe VICI connection and CHILD_SA primitive/list options can be supplied in `advanced_connection` and `advanced_child`. Invalid JSON, unknown top-level keys and protected VICI keys controlling identity, authentication, IP pools or traffic selectors are rejected. Admins can use **Format JSON** and **Reset to saved**, then the single contextual **Save & apply** action in the page header. Advanced edits require a confirmation before saving.
- **Certificate Authority:** displays the *actual issued X.509 Root CA* subject, serial and expiration. Before the first device is issued, an administrator can explicitly issue a new CA subject after typing `ROTATE CA`; existing key material is backed up. Once any device or historical revocation exists, the CA subject is immutable in this interface. Modifying the VPN display name does **not** require CA rotation. For fresh installs, `CA_COMMON_NAME`, `CA_ORGANIZATION`, `CA_COUNTRY`, `CA_PROVINCE`, and `CA_LOCALITY` may set the initial CA subject before starting SwanPanel for the first time.

### Data-plane apply status

An administrator can see a distinct unsaved-edit warning, the **saved revision** and **active Gateway revision**, plus localized `applied`, `applying`, `pending` or `failed` states. **Save & apply** stores the desired settings and attempts synchronization; a saved response does not guarantee enforcement. **Retry apply** retries the already saved configuration without resubmitting the edit. API endpoints: `GET /api/system/apply-status` and `POST /api/system/apply` (admin only). The reconciler persists desired state and checks both the on-disk configuration checksum and the live VICI socket identity every 30 seconds. A daemon restart triggers reapplication even if database settings are unchanged. **Retry apply** also redeploys gateway certificates and the signed CRL. The nftables table is atomically replaced without a blanket deny-all during normal updates; removed grants are closed before the VICI connection update. An enforcement failure remains visible as pending/failed and must be investigated.

### Operational notes

- **Recommended deployment:** Docker Compose all-in-one, with its own strongSwan and persistent `/etc/swanctl` volume. Save `JWT_SECRET` in `.env` before `docker compose up -d`; keep that file and the database/PKI volumes across upgrades.
- **Optional host strongSwan:** An explicit Compose override uses host networking so VICI, XFRM, DNS gateway VIPs, nftables and NAT operate in one namespace. The firewall only drops/clamps IPsec traffic from enrolled SwanPanel VIPs (including revoked identities), not every host IPsec packet. Do not overlap another VPN's VIP ranges, and check host DNS/API/IKE ports, kernel forwarding and `/etc/swanctl` before switching. When Docker uses the iptables backend with `FORWARD DROP`, SwanPanel maintains an owned `SWANPANEL-FWD` chain and `DOCKER-USER` jump. Only enrolled device VIPs with an authorized Zero-Trust mark may pass; do not add broad subnet-wide `ACCEPT` rules. This integration fails closed on unsupported Docker firewall configurations. Host daemon Start/Stop/Restart remains unavailable in the container UI; live connection configuration and session actions continue through VICI.
- Use **Go 1.27.4** for building. The Docker and devcontainer build images are pinned to that patch version.
- `STRONGSWAN_SYSTEMD_UNIT` can override the default `strongswan` unit name on distributions using another systemd unit.
- SwanPanel writes its connection/pool fragment to `/etc/swanctl/conf.d/swanpanel-gateway.conf` (override with `SWANPANEL_SWANCTL_FILE` only if the daemon is configured to include that location). This file and strongSwan credentials must be accessible by the same daemon. The Linux installer installs a `strongswan.service` restart hook to load all persisted connections. The official Docker Compose stack persists `/etc/swanctl` in its own named volume. The host-daemon override bind-mounts the host `/etc/swanctl` and the live `/run` directory (read-only) at `/host-run`; bind-mounting just the VICI socket file would retain a stale socket inode after an external daemon restart.
- IKEv2 supports `dpd_delay`; it does **not** expose a separately configurable `dpd_timeout` via `swanctl.conf`. The legacy `dpd_timeout` field remains readable for compatibility but cannot be edited.
- The first address within each VPN subnet is reserved for a gateway, alongside network and broadcast addresses. A device's VIP can be reused after its identity has been revoked and the device deleted. Do not recycle VIPs while the original connection is still active.
- Database-backed API/PKI tests use `APP_ENV=test`, which deliberately skips live firewall and VICI operations. These tests do **not** replace end-to-end VPN connectivity, nftables syntax validation, CRL reload, actual IP allocation and daemon restart checks on a Linux gateway.
- Rotating the Root CA can invalidate existing trust. Never delete the `.backup-*` private key material without an explicit key-retention and recovery plan; keep it restricted and backed up.

### Isolated Docker IKEv2 integration test

The original scripts/test_e2e_integration.sh has been replaced with a safe
wrapper around scripts/test_docker_integration.sh, because its former test
payloads and assumptions no longer matched the application.

On a Linux host with Docker Engine, functioning IPsec/XFRM kernel support,
NET_ADMIN inside containers, curl and jq, run:

    bash scripts/test_docker_integration.sh

The test uses tests/docker/compose.yml, builds an actual gateway image and
strongSwan Linux client image, and creates disposable credentials/data in a
separate Compose project. It checks live VICI readiness, PKI and per-device
connection configuration, nftables default deny, persistent gateway files,
independent strongSwan restart and automatic VICI/pool recovery, client tunnel
and assigned VIP, and revocation blocking reconnect. It binds the test API only to
127.0.0.1:18080, cleans up containers and volumes, and must never be run
against production data.

You may also start the Docker IKEv2 Integration GitHub Actions workflow
manually. Docker Desktop on macOS runs a LinuxKit kernel; support for the
necessary XFRM/ESP operations must be verified on the actual host. A passing
Go unit test or docker compose config does not prove real VPN connectivity.

### Safe controls in the administration UI

SwanPanel uses one consistent native-select appearance in dark and light themes.
Deleting users, groups, subnets, DNS records, access rules and identity providers;
revoking VPN certificates; terminating sessions; assigning or removing system
administrators; changing live gateway state or the Root CA; and applying
advanced VICI settings require an explicit confirmation dialog showing the
affected entity and consequences. Cancel never invokes the operation. If an
API request fails, the dialog stays open and displays the server error.
